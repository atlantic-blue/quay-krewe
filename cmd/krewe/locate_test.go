package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/controlplane"
	"github.com/atlantic-blue/quay-krewe/internal/model"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/secrets"
	"github.com/atlantic-blue/quay-krewe/internal/store"
)

// An address becomes a directory, and the listing is what prints it.
//
// The failure it answers: somebody had a screenshot to put in front of a running session, and finding
// where to put it meant reading three directories named in hex and then inspecting a container that
// happened to be up. With every container down there was nothing on the machine to read at all.
//
// These drive krewe volume list, because the directory is its first line. They used to drive krewe
// where, which named the directory and stopped there. The behaviour under the two words is the same
// call, so the tests moved with the word rather than going with it.

// aSystemOnDisk stands up a system whose data directory is a real one, and hands back the client, the
// store and that directory. The store comes back because the hard case is a session that never ran,
// and the only way to make one is to put the row in without an exec touching it.
func aSystemOnDisk(t *testing.T) (quaycrewv1.ControlPlaneServiceClient, store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	held := store.NewMemory()
	client := testClientWith(t, controlplane.Config{
		Store: held, Runner: &model.FakeRunner{Reply: "ok"},
		Provider: &sandbox.FakeProvider{}, Secrets: secrets.NewMemory(),
		Storage: sandbox.Storage{Dir: dir, Host: dir},
	})
	return client, held, dir
}

// A workspace nobody has worked in yet is the first case, because the shared folder is made when a
// sandbox starts and a workspace with no sessions has never had one. Before this the answer was that
// there was no directory, which is true and useless to somebody holding a file.
func TestAWorkspaceWithNoSessionsNamesAFolderThatExists(t *testing.T) {
	client, _, _ := aSystemOnDisk(t)
	mustRun(t, client, "workspace", "create", "acme")

	said := mustRun(t, client, "volume", "list", "acme")

	path := firstLineOf(said)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the listing printed %q, which is not on disk: %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("the listing printed %q, which is not a directory", path)
	}
}

// The same for a session the system has made and nothing has run in. A job that is about to be
// dispatched is exactly when somebody wants to leave it a file to read.
func TestASessionThatHasNeverRunNamesADirectoryThatExists(t *testing.T) {
	client, held, _ := aSystemOnDisk(t)
	mustRun(t, client, "workspace", "create", "acme")
	mustRun(t, client, "project", "create", "house-bills")
	handle := aSessionNothingHasRunIn(t, held, "house-bills")

	said := mustRun(t, client, "volume", "list", "acme/house-bills/"+handle)

	path := firstLineOf(said)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the listing printed %q, which is not on disk: %v", path, err)
	}
}

// The proof that matters: the directory the command names is the one a sandbox binds, read from the
// same call the container runtime is given. Asserting the path was assembled correctly would pass
// against a layout nothing mounts.
func TestTheListingNamesTheDirectoryASandboxActuallyMounts(t *testing.T) {
	dir := t.TempDir()
	storage := sandbox.Storage{Dir: dir, Host: dir}
	held := store.NewMemory()
	client := testClientWith(t, controlplane.Config{
		Store: held, Runner: &model.FakeRunner{Reply: "ok"},
		Provider: &sandbox.FakeProvider{}, Secrets: secrets.NewMemory(), Storage: storage,
	})
	mustRun(t, client, "workspace", "create", "acme")
	mustRun(t, client, "project", "create", "house-bills")
	mustRun(t, client, "exec", "acme/house-bills", "sort the listing by the clock it shows")
	session := theOnlySession(t, client)

	shared := theVolumeAt(t, client, "acme")
	working := theVolumeAt(t, client, "acme/house-bills/"+session.GetHandle())

	mounts, err := storage.Prepare(sandbox.Config{
		ID: session.GetId(), Workspace: session.GetWorkspace(), Project: session.GetProject(),
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	bound := make(map[string]string, len(mounts))
	for _, mount := range mounts {
		bound[mount.Source] = mount.Target
	}
	if bound[shared] != sandbox.SharedPath {
		t.Fatalf("the listing says the shared folder is %q, and a sandbox binds %v", shared, bound)
	}
	if bound[working] != sandbox.WorkingPath {
		t.Fatalf("the listing says the working directory is %q, and a sandbox binds %v", working, bound)
	}
}

// A file put in by hand is in the session's directory, which is the whole sentence this serves. The
// path is followed rather than rebuilt, so a command that printed a plausible path fails here.
//
// It comes back through the copy verb, which is what replaced the word that used to print a session's
// file. A capability removed and not replaced is the failure this end of the test guards.
func TestAFilePutInTheDirectoryIsInTheSessionsOwnWork(t *testing.T) {
	client, held, _ := aSystemOnDisk(t)
	mustRun(t, client, "workspace", "create", "acme")
	mustRun(t, client, "project", "create", "house-bills")
	handle := aSessionNothingHasRunIn(t, held, "house-bills")

	path := theVolumeAt(t, client, "acme/house-bills/"+handle)
	if err := os.WriteFile(filepath.Join(path, "screenshot.png"), []byte("a picture"), 0o666); err != nil {
		t.Fatalf("put a file in %q: %v", path, err)
	}

	// Read back through the verb, which resolves the address itself rather than being told this path.
	back := filepath.Join(t.TempDir(), "screenshot.png")
	mustRun(t, client, "volume", "cp", "krewe://acme/house-bills/"+handle+"/screenshot.png", back)
	body, err := os.ReadFile(back)
	if err != nil {
		t.Fatalf("read what came back: %v", err)
	}
	if string(body) != "a picture" {
		t.Fatalf("the session's work reads %q, want the file that was put in by hand", body)
	}
}

// An address that is not there says what is, because the next move is to type one of them.
func TestAnAddressThatDoesNotExistSaysWhatThereIs(t *testing.T) {
	client, _, _ := aSystemOnDisk(t)
	mustRun(t, client, "workspace", "create", "acme")
	var out bytes.Buffer

	err := run(context.Background(), client, []string{"volume", "list", "nowhere"}, &out, "")

	if err == nil {
		t.Fatalf("an address that does not exist answered with %q", out.String())
	}
	if !strings.Contains(err.Error(), "acme") {
		t.Fatalf("the refusal is %q, want it to name the workspace there is", err)
	}
}

// A session in the wrong project is refused, at the command and again at the call behind it. A path
// assembled from what was typed would name a directory nothing mounts, and somebody would leave a
// file in it and wait for a session that never sees it.
func TestASessionThatIsNotInThatProjectIsRefused(t *testing.T) {
	client, held, _ := aSystemOnDisk(t)
	mustRun(t, client, "workspace", "create", "acme")
	mustRun(t, client, "project", "create", "house-bills")
	mustRun(t, client, "project", "create", "acme/holidays")
	handle := aSessionNothingHasRunIn(t, held, "house-bills")
	var out bytes.Buffer

	err := run(context.Background(), client, []string{"volume", "list", "acme/holidays/" + handle}, &out, "")
	if err == nil {
		t.Fatalf("a session in another project answered with %q", out.String())
	}

	// And the call itself, which another client reaches without the tool's own resolution in front of
	// it. Two guards because they fail differently: the tool cannot resolve the address at all, and the
	// system refuses to answer for a session that is not where the caller says it is.
	ctx := context.Background()
	projects, err := held.ListProjects(ctx, "")
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	for _, project := range projects {
		if project.GetName() != "holidays" {
			continue
		}
		_, err := client.LocateDirectory(ctx, &quaycrewv1.LocateDirectoryRequest{
			Workspace: project.GetWorkspace(), Project: project.GetId(), Session: handle,
		})
		if err == nil {
			t.Fatal("the call answered for a session that is in another project")
		}
	}
}

// The system's own directory is where this system keeps its credentials, so the one word that would
// name it is refused rather than answered.
func TestTheSystemsOwnDirectoryIsRefused(t *testing.T) {
	client, _, _ := aSystemOnDisk(t)
	mustRun(t, client, "workspace", "create", "acme")
	var out bytes.Buffer

	err := run(context.Background(), client, []string{"volume", "list", "system"}, &out, "")

	if err == nil {
		t.Fatalf("the system's own directory answered with %q", out.String())
	}
	if !strings.Contains(err.Error(), "the tokens and the sealing key") {
		t.Fatalf("the refusal is %q, want it to say plainly what is in there", err)
	}
	if strings.Contains(out.String(), "token") || strings.Contains(err.Error(), ".token") {
		t.Fatalf("the refusal names a credential file: %q %v", out.String(), err)
	}
}

// The path is on its own line with nothing beside it, because cd "$(krewe volume list acme)" is the
// shape this gets typed in. Anything sharing that line breaks every use of it.
func TestThePathIsAloneOnTheFirstLine(t *testing.T) {
	client, _, dir := aSystemOnDisk(t)
	mustRun(t, client, "workspace", "create", "acme")

	said := mustRun(t, client, "volume", "list", "acme")

	first := firstLineOf(said)
	if strings.ContainsAny(first, " \t") {
		t.Fatalf("the first line is %q, want the path and nothing else", first)
	}
	if !strings.HasPrefix(first, dir) {
		t.Fatalf("the first line is %q, want a path under the data directory %q", first, dir)
	}
}

// The path is the machine's, not the control plane's own view of it. Those differ the moment the
// control plane runs in a container, and the one that is any use to a person is the machine's.
//
// It asks the call rather than the listing, because the two paths differ only where the control plane
// cannot open the machine's one, and a listing reads the directory it names.
func TestTheAnswerIsTheMachinesPathNotTheControlPlanesOwn(t *testing.T) {
	held := store.NewMemory()
	client := testClientWith(t, controlplane.Config{
		Store: held, Runner: &model.FakeRunner{Reply: "ok"},
		Provider: &sandbox.FakeProvider{}, Secrets: secrets.NewMemory(),
		Storage: sandbox.Storage{Dir: t.TempDir(), Host: "/var/lib/krewe"},
	})
	mustRun(t, client, "workspace", "create", "acme")
	workspaces, err := held.ListWorkspaces(context.Background())
	if err != nil || len(workspaces) != 1 {
		t.Fatalf("ListWorkspaces: %v, %d workspaces", err, len(workspaces))
	}

	found, err := client.LocateDirectory(context.Background(), &quaycrewv1.LocateDirectoryRequest{
		Workspace: workspaces[0].GetId(),
	})
	if err != nil {
		t.Fatalf("LocateDirectory: %v", err)
	}

	if !strings.HasPrefix(found.GetHost(), "/var/lib/krewe/workspaces/") {
		t.Fatalf("the call answered %q, want the path as the machine running the sandboxes sees it",
			found.GetHost())
	}
}

// A system that keeps nothing on disk says so, rather than printing a path into a container that is
// about to be thrown away.
func TestASystemThatKeepsNothingSaysSo(t *testing.T) {
	client := testClient(t)
	mustRun(t, client, "workspace", "create", "acme")
	var out bytes.Buffer

	err := run(context.Background(), client, []string{"volume", "list", "acme"}, &out, "")

	if err == nil {
		t.Fatalf("a system with no data directory printed %q", out.String())
	}
	if !strings.Contains(err.Error(), "container") {
		t.Fatalf("the refusal is %q, want it to say where the state actually is", err)
	}
}

// The three listings each say how to get from a row to a directory, because a listing is where
// somebody looks when they are holding a file and do not know where to put it.
func TestTheListingsSayHowToReachADirectory(t *testing.T) {
	client, _, _ := aSystemOnDisk(t)
	mustRun(t, client, "workspace", "create", "acme")
	mustRun(t, client, "project", "create", "house-bills")
	mustRun(t, client, "exec", "acme/house-bills", "sort the listing")

	for _, listing := range [][]string{
		{"workspace", "list"}, {"project", "list"}, {"sessions"},
	} {
		said := mustRun(t, client, listing...)
		if !strings.Contains(said, "krewe volume list") {
			t.Fatalf("krewe %s does not say how to reach a directory:\n%s",
				strings.Join(listing, " "), said)
		}
	}
}

// aSessionNothingHasRunIn puts a session row into the named project without an exec, which is the only
// way to have a session whose directory has never been made. The project is named rather than taken
// first from the listing, because a test about the wrong project needs to know which one it is in.
func aSessionNothingHasRunIn(t *testing.T, held store.Store, project string) string {
	t.Helper()
	ctx := context.Background()
	projects, err := held.ListProjects(ctx, "")
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	for _, one := range projects {
		if one.GetName() != project {
			continue
		}
		session, _, err := held.FindOrCreateSession(ctx, one.GetId(), store.NewID(), store.Birth{})
		if err != nil {
			t.Fatalf("FindOrCreateSession: %v", err)
		}
		return session.GetHandle()
	}
	t.Fatalf("no project called %q, there are %d", project, len(projects))
	return ""
}

// theOnlySession is the session a system with one of them holds.
func theOnlySession(t *testing.T, client quaycrewv1.ControlPlaneServiceClient) *quaycrewv1.Session {
	t.Helper()
	listed, err := client.ListSessions(context.Background(), &quaycrewv1.ListSessionsRequest{})
	if err != nil || len(listed.GetSessions()) != 1 {
		t.Fatalf("ListSessions: %v, %d sessions", err, len(listed.GetSessions()))
	}
	return listed.GetSessions()[0]
}

// firstLineOf is the path an answer opens with.
func firstLineOf(said string) string {
	line, _, _ := strings.Cut(said, "\n")
	return strings.TrimSpace(line)
}

// A project address is a folder inside the shared one, which is where the file actually has to land.
// Before this it answered with the shared folder itself, so an address naming a workspace and an
// address naming a project in it named one directory between them, and the address said nothing.
func TestAProjectAddressNamesAFolderInsideTheSharedOne(t *testing.T) {
	client, _, _ := aSystemOnDisk(t)
	mustRun(t, client, "workspace", "create", "acme")
	mustRun(t, client, "project", "create", "house-bills")

	shared := theVolumeAt(t, client, "acme")
	said := mustRun(t, client, "volume", "list", "acme/house-bills")
	project := firstLineOf(said)

	if project == shared {
		t.Fatalf("the project address answers with the shared folder %q", shared)
	}
	if want := filepath.Join(shared, "house-bills"); project != want {
		t.Fatalf("the listing says the project folder is %q, want %q", project, want)
	}
	info, err := os.Stat(project)
	if err != nil {
		t.Fatalf("the listing printed %q, which is not on disk: %v", project, err)
	}
	if !info.IsDir() {
		t.Fatalf("the listing printed %q, which is not a directory", project)
	}
}

// The half a printed path cannot prove about itself: a file put in the project folder is inside the
// directory a sandbox binds, at the path the answer promised a session would read it at.
func TestAFilePutInTheProjectFolderIsWhereTheSessionWillLookForIt(t *testing.T) {
	dir := t.TempDir()
	storage := sandbox.Storage{Dir: dir, Host: dir}
	client := testClientWith(t, controlplane.Config{
		Store: store.NewMemory(), Runner: &model.FakeRunner{Reply: "ok"},
		Provider: &sandbox.FakeProvider{}, Secrets: secrets.NewMemory(), Storage: storage,
	})
	mustRun(t, client, "workspace", "create", "acme")
	mustRun(t, client, "project", "create", "house-bills")
	mustRun(t, client, "exec", "acme/house-bills", "sort the listing by the clock it shows")
	session := theOnlySession(t, client)

	project := theVolumeAt(t, client, "acme/house-bills")
	if err := os.WriteFile(filepath.Join(project, "screenshot.png"), []byte("a picture"), 0o666); err != nil {
		t.Fatalf("put a file in %q: %v", project, err)
	}

	// Read through the same call the container runtime is given, rather than a path this test builds.
	mounts, err := storage.Prepare(sandbox.Config{
		ID: session.GetId(), Workspace: session.GetWorkspace(), Project: session.GetProject(),
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	for _, mount := range mounts {
		if mount.Target != sandbox.SharedPath {
			continue
		}
		inside := filepath.Join(mount.Source, "house-bills", "screenshot.png")
		if _, err := os.Stat(inside); err != nil {
			t.Fatalf("a sandbox binds %q at %q, and the file is not at %q: %v",
				mount.Source, mount.Target, inside, err)
		}
		return
	}
	t.Fatalf("no sandbox mount lands at %s, so nothing reads the project folder", sandbox.SharedPath)
}

// A project of another workspace is a not found rather than a folder. Answering it would make a
// directory in a volume that workspace's sessions never read, and hand back a path that stays empty.
func TestLocateRefusesAProjectThatIsNotInThatWorkspace(t *testing.T) {
	client, held, _ := aSystemOnDisk(t)
	mustRun(t, client, "workspace", "create", "acme")
	mustRun(t, client, "workspace", "create", "other")
	mustRun(t, client, "project", "create", "acme/house-bills")

	ctx := context.Background()
	projects, err := held.ListProjects(ctx, "")
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	workspaces, err := held.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	other := ""
	for _, one := range workspaces {
		if one.GetName() == "other" {
			other = one.GetId()
		}
	}
	if other == "" {
		t.Fatal("the second workspace was not created, so this proves nothing")
	}
	for _, project := range projects {
		if project.GetName() != "house-bills" {
			continue
		}
		_, err := client.LocateDirectory(ctx, &quaycrewv1.LocateDirectoryRequest{
			Workspace: other, Project: project.GetId(),
		})
		if err == nil {
			t.Fatal("the call answered for a project that is in another workspace")
		}
		return
	}
	t.Fatal("the project was not created, so this proves nothing")
}

// The folder is named after the project, so a name the system already writes there is refused at
// creation. A project called worktrees would be one path naming two directories, and a file copied
// into it would land on a session's checkout.
func TestAProjectCannotBeCalledAFolderTheSystemAlreadyWrites(t *testing.T) {
	client, _, _ := aSystemOnDisk(t)
	mustRun(t, client, "workspace", "create", "acme")
	var out bytes.Buffer

	err := run(context.Background(), client, []string{"project", "create", "acme/worktrees"}, &out, "")
	if err == nil {
		t.Fatalf("a project called worktrees was created: %q", out.String())
	}
	if !strings.Contains(err.Error(), "shared folder") {
		t.Fatalf("the refusal is %q, want it to say where the collision is", err)
	}
}

// A session that took a working tree.
//
// The git skill teaches this shape first: the checkout goes in the workspace's volume under the
// session's own identifier, so the session's own directory stays empty. An address that named the
// empty one sent a person to copy a file into a directory nothing was working in, and a listing of it
// said the session had made nothing.
//
// The answer is the tree rather than the checkout inside it. A file dropped into the checkout is a
// file in somebody's git status, and the whole point of the address is somewhere to put a file.
func TestASessionThatTookAWorkingTreeNamesTheTreeAndNotItsOwnDirectory(t *testing.T) {
	dir := t.TempDir()
	storage := sandbox.Storage{Dir: dir, Host: dir}
	held := store.NewMemory()
	client := testClientWith(t, controlplane.Config{
		Store: held, Runner: &model.FakeRunner{Reply: "ok"},
		Provider: &sandbox.FakeProvider{}, Secrets: secrets.NewMemory(), Storage: storage,
	})
	mustRun(t, client, "workspace", "create", "acme")
	mustRun(t, client, "project", "create", "house-bills")
	handle := aSessionNothingHasRunIn(t, held, "house-bills")
	session := theOnlySession(t, client)
	tree := aWorkingTreeTaken(t, storage, session)

	said := mustRun(t, client, "volume", "list", "acme/house-bills/"+handle)

	if firstLineOf(said) != tree {
		t.Fatalf("the listing names %q, want the working tree at %q", firstLineOf(said), tree)
	}
	// And the listing under it is the checkout, which is what says the tree was read rather than the
	// session's own directory, since that one is empty.
	if !strings.Contains(said, "quay-krewe/") {
		t.Fatalf("the listing does not hold the checkout:\n%s", said)
	}
}

// The other shape, kept beside the first one so the answer for a session that cloned into its own
// directory is proved rather than assumed to be unchanged.
func TestASessionThatTookNoWorkingTreeStillNamesItsOwnDirectory(t *testing.T) {
	client, held, dir := aSystemOnDisk(t)
	mustRun(t, client, "workspace", "create", "acme")
	mustRun(t, client, "project", "create", "house-bills")
	handle := aSessionNothingHasRunIn(t, held, "house-bills")

	said := mustRun(t, client, "volume", "list", "acme/house-bills/"+handle)

	session := theOnlySession(t, client)
	own, keeps := sandbox.Storage{Dir: dir, Host: dir}.WorkingDir(sandbox.Config{
		ID: session.GetId(), Workspace: session.GetWorkspace(), Project: session.GetProject(),
	})
	if !keeps {
		t.Fatal("this storage keeps no working directory, so there is none to name")
	}
	if firstLineOf(said) != own {
		t.Fatalf("the listing names %q, want the session's own directory %q", firstLineOf(said), own)
	}
}

// aWorkingTreeTaken writes what a session leaves behind when it follows the git skill: a checkout in
// the workspace's volume, under this session's identifier. Its `.git` is a file, which is what git
// writes in a working tree, and it answers the directory holding the checkout.
func aWorkingTreeTaken(t *testing.T, storage sandbox.Storage, session *quaycrewv1.Session) string {
	t.Helper()
	volume, held := storage.VolumeDir(session.GetWorkspace())
	if !held {
		t.Fatal("this storage keeps no volume, so no session in it can take a working tree")
	}
	tree := filepath.Join(volume, "worktrees", session.GetId())
	checkout := filepath.Join(tree, "quay-krewe")
	if err := os.MkdirAll(checkout, 0o777); err != nil {
		t.Fatal(err)
	}
	gitdir := "gitdir: /home/agent/shared/repos/quay-krewe/.git/worktrees/" + session.GetId() + "\n"
	if err := os.WriteFile(filepath.Join(checkout, ".git"), []byte(gitdir), 0o666); err != nil {
		t.Fatal(err)
	}
	return tree
}
