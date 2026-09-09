package sandbox_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
)

// A name on the filesystem for every address.
//
// The failure it answers: a person told to put a file in front of a session had to read
// workspaces/<24 hexadecimal characters>/volume, and a session's own directory is three of those
// deep. The names were in the store and nowhere on disk, so the finder showed identifiers.

// wholeTree is a data directory, the tree beside it, and the storage that writes both.
func wholeTree(t *testing.T) (sandbox.Storage, string) {
	t.Helper()
	home := t.TempDir()
	data := filepath.Join(home, "data")
	tree := filepath.Join(home, "at")
	if err := os.MkdirAll(data, 0o777); err != nil {
		t.Fatalf("the data directory: %v", err)
	}
	return sandbox.Storage{Dir: data, Host: data, NameTree: tree}, tree
}

func TestAWorkspaceNameReachesItsSharedFolder(t *testing.T) {
	storage, tree := wholeTree(t)

	if err := storage.NameWorkspace("9e8153f6d4c1", "itv"); err != nil {
		t.Fatalf("NameWorkspace: %v", err)
	}

	shared, err := storage.SharedDirectory("9e8153f6d4c1")
	if err != nil {
		t.Fatalf("SharedDirectory: %v", err)
	}
	named := filepath.Join(tree, "itv")
	points, err := os.Readlink(named)
	if err != nil {
		t.Fatalf("%q is not a name pointing anywhere: %v", named, err)
	}
	if points != shared.Host {
		t.Errorf("%q points at %q, want the shared folder %q", named, points, shared.Host)
	}
	if strings.Contains(named, "9e8153f6d4c1") {
		t.Errorf("the name is %q, and an identifier is in it", named)
	}
}

// The project level is the folder itself, inside the shared one, so one link carries the workspace
// and every project in it. A file dropped at the name has to arrive in the folder a sandbox binds.
func TestAProjectIsReachedThroughItsWorkspaceName(t *testing.T) {
	storage, tree := wholeTree(t)

	if err := storage.NameWorkspace("9e8153f6d4c1", "itv"); err != nil {
		t.Fatalf("NameWorkspace: %v", err)
	}
	if err := storage.NameProject("9e8153f6d4c1", "vast"); err != nil {
		t.Fatalf("NameProject: %v", err)
	}

	dropped := filepath.Join(tree, "itv", "vast", "explore.txt")
	if err := os.WriteFile(dropped, []byte("a log"), 0o666); err != nil {
		t.Fatalf("drop a file at the name: %v", err)
	}

	project, err := storage.ProjectDirectory("9e8153f6d4c1", "vast")
	if err != nil {
		t.Fatalf("ProjectDirectory: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(project.Host, "explore.txt"))
	if err != nil {
		t.Fatalf("the file is not in the folder a sandbox binds: %v", err)
	}
	if string(body) != "a log" {
		t.Errorf("the folder holds %q, want the bytes that were dropped", body)
	}
}

// A session's own directory is not in the volume, so its name sits beside the workspace's rather
// than under it: a link inside the shared folder points at a path on the host, and every container
// reading that folder sees a broken name.
func TestASessionNameSitsBesideItsWorkspaceAndReachesItsOwnDirectory(t *testing.T) {
	storage, tree := wholeTree(t)
	cfg := sandbox.Config{Workspace: "9e8153f6d4c1", Project: "b75f5bf62544", ID: "aa01bb02cc03"}

	if err := storage.NameSession(cfg, sandbox.Names{
		Workspace: "itv", Project: "vast", Session: "the-login-that-times-out",
	}); err != nil {
		t.Fatalf("NameSession: %v", err)
	}

	working, err := storage.WorkingDirectory(cfg)
	if err != nil {
		t.Fatalf("WorkingDirectory: %v", err)
	}
	named := filepath.Join(tree, "itv"+sandbox.SessionsSuffix, "vast", "the-login-that-times-out")
	points, err := os.Readlink(named)
	if err != nil {
		t.Fatalf("%q is not a name pointing anywhere: %v", named, err)
	}
	if points != working.Host {
		t.Errorf("%q points at %q, want the session's own directory %q", named, points, working.Host)
	}
	// The workspace's own name is a link, so a session name under it would be written inside the
	// shared folder every sandbox binds.
	if _, err := os.Lstat(filepath.Join(tree, "itv", "vast", "the-login-that-times-out")); err == nil {
		t.Error("a session name was written inside the shared folder, where every sandbox reads it")
	}
}

func TestNamingTheSameThingTwiceWritesNothingTheSecondTime(t *testing.T) {
	storage, tree := wholeTree(t)

	if err := storage.NameWorkspace("9e8153f6d4c1", "itv"); err != nil {
		t.Fatalf("the first: %v", err)
	}
	if err := storage.NameWorkspace("9e8153f6d4c1", "itv"); err != nil {
		t.Fatalf("the second: %v", err)
	}

	entries, err := os.ReadDir(tree)
	if err != nil {
		t.Fatalf("read the tree: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("the tree holds %d names, want one", len(entries))
	}
}

// Two workspaces may hold one name, so the second cannot take the first one's name: a file dropped
// for the first would land in the second one's folder, and nothing would say so.
func TestANameAlreadyPointingSomewhereElseIsRefused(t *testing.T) {
	storage, tree := wholeTree(t)

	if err := storage.NameWorkspace("9e8153f6d4c1", "itv"); err != nil {
		t.Fatalf("the first: %v", err)
	}
	err := storage.NameWorkspace("b75f5bf62544", "itv")
	if err == nil {
		t.Fatal("the second workspace took the name of the first")
	}
	if !strings.Contains(err.Error(), "9e8153f6d4c1") {
		t.Errorf("the refusal is %q, and it does not say what the name points at", err)
	}

	shared, err := storage.SharedDirectory("9e8153f6d4c1")
	if err != nil {
		t.Fatalf("SharedDirectory: %v", err)
	}
	points, err := os.Readlink(filepath.Join(tree, "itv"))
	if err != nil {
		t.Fatalf("read the name: %v", err)
	}
	if points != shared.Host {
		t.Errorf("the name points at %q, want the first workspace's folder %q", points, shared.Host)
	}
}

// A directory somebody made by hand under a name is left where it is. This did not write it, so
// removing it is not this view's business.
func TestANameSomethingElseOwnsIsLeftAsItIs(t *testing.T) {
	storage, tree := wholeTree(t)
	mine := filepath.Join(tree, "itv")
	if err := os.MkdirAll(mine, 0o777); err != nil {
		t.Fatalf("the directory somebody made: %v", err)
	}

	if err := storage.NameWorkspace("9e8153f6d4c1", "itv"); err == nil {
		t.Fatal("a directory that was already there was replaced by a name")
	}
	info, err := os.Lstat(mine)
	if err != nil {
		t.Fatalf("the directory is gone: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("%q is now a %v, want the directory that was there", mine, info.Mode())
	}
}

// The top of the data directory holds the tokens and the sealing key. No workspace can be called
// that word, and a tree of names must not be the one place in the system offering a road to them.
func TestTheSystemsOwnWordIsNeverANameInTheTree(t *testing.T) {
	storage, tree := wholeTree(t)

	for _, called := range []string{"system", "System", "SYSTEM"} {
		err := storage.NameWorkspace("9e8153f6d4c1", called)
		if err == nil {
			t.Fatalf("%q became a name in the tree", called)
		}
		if !strings.Contains(err.Error(), "sealing key") {
			t.Errorf("the refusal for %q is %q, and it does not say what is in that directory", called, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(tree, "system")); !errors.Is(err, os.ErrNotExist) {
		t.Error("the tree holds a name called system")
	}
}

// A name is one segment. A workspace whose name climbed would put the tree somewhere else entirely.
func TestANameThatClimbsIsRefused(t *testing.T) {
	storage, _ := wholeTree(t)

	for _, called := range []string{"..", ".", "../elsewhere", "itv/vast", ""} {
		if err := storage.NameWorkspace("9e8153f6d4c1", called); err == nil {
			t.Errorf("%q became a name in the tree", called)
		}
	}
	if err := storage.NameSession(
		sandbox.Config{Workspace: "9e8153f6d4c1", Project: "b75f5bf62544", ID: "aa01bb02cc03"},
		sandbox.Names{Workspace: "itv", Project: "vast", Session: "../../elsewhere"},
	); err == nil {
		t.Error("a session name that climbs became a name in the tree")
	}
}

// A system told no tree writes none, and says nothing about it: the tree is a view, and running
// without one is a way of running the system rather than a failure.
func TestNoTreeConfiguredWritesNothing(t *testing.T) {
	dir := t.TempDir()
	storage := sandbox.Storage{Dir: dir, Host: dir}

	if err := storage.NameWorkspace("9e8153f6d4c1", "itv"); err != nil {
		t.Errorf("NameWorkspace: %v", err)
	}
	if err := storage.NameSession(
		sandbox.Config{Workspace: "9e8153f6d4c1", Project: "b75f5bf62544", ID: "aa01bb02cc03"},
		sandbox.Names{Workspace: "itv", Project: "vast", Session: "the-login-that-times-out"},
	); err != nil {
		t.Errorf("NameSession: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "itv")); !errors.Is(err, os.ErrNotExist) {
		t.Error("a name was written into the data directory")
	}
}

// The tree follows the store, so a name that changed, was put away or was deleted does not leave a
// link behind pointing at a directory nobody is working in.

// A session's label is the name in the tree. Changing it has to move the link rather than add a
// second one, or the tree says the session is in two places and one of them is wrong.
func TestARenamedSessionIsUnderTheNewNameAndNotTheOldOne(t *testing.T) {
	storage, tree := wholeTree(t)
	cfg := sandbox.Config{Workspace: "9e8153f6d4c1", Project: "b75f5bf62544", ID: "aa01bb02cc03"}
	held := sandbox.Held{
		Workspaces: []sandbox.WorkspaceName{{ID: "9e8153f6d4c1", Name: "itv"}},
		Sessions: []sandbox.SessionName{{Config: cfg, Names: sandbox.Names{
			Workspace: "itv", Project: "vast", Session: "the-login-that-times-out",
		}}},
	}
	if err := storage.WriteNames(held); err != nil {
		t.Fatalf("the first: %v", err)
	}

	held.Sessions[0].Names.Session = "the-checkout-that-times-out"
	if err := storage.WriteNames(held); err != nil {
		t.Fatalf("after the rename: %v", err)
	}

	working, err := storage.WorkingDirectory(cfg)
	if err != nil {
		t.Fatalf("WorkingDirectory: %v", err)
	}
	under := filepath.Join(tree, "itv"+sandbox.SessionsSuffix, "vast")
	points, err := os.Readlink(filepath.Join(under, "the-checkout-that-times-out"))
	if err != nil {
		t.Fatalf("the new name is not there: %v", err)
	}
	if points != working.Host {
		t.Errorf("the new name points at %q, want the session's own directory %q", points, working.Host)
	}
	if _, err := os.Lstat(filepath.Join(under, "the-login-that-times-out")); !errors.Is(err, os.ErrNotExist) {
		t.Error("the old name is still in the tree, so the session is in it twice")
	}
}

// A session put away is out of the listing, and the tree is the same view. A link left behind sends
// somebody to drop a file for a session that is not running and cannot be seen.
func TestAnArchivedSessionLeavesNoNameBehind(t *testing.T) {
	storage, tree := wholeTree(t)
	cfg := sandbox.Config{Workspace: "9e8153f6d4c1", Project: "b75f5bf62544", ID: "aa01bb02cc03"}
	held := sandbox.Held{
		Workspaces: []sandbox.WorkspaceName{{ID: "9e8153f6d4c1", Name: "itv"}},
		Sessions: []sandbox.SessionName{{Config: cfg, Names: sandbox.Names{
			Workspace: "itv", Project: "vast", Session: "the-login-that-times-out",
		}}},
	}
	if err := storage.WriteNames(held); err != nil {
		t.Fatalf("the first: %v", err)
	}

	held.Sessions = nil
	if err := storage.WriteNames(held); err != nil {
		t.Fatalf("after the archive: %v", err)
	}

	named := filepath.Join(tree, "itv"+sandbox.SessionsSuffix, "vast", "the-login-that-times-out")
	if _, err := os.Lstat(named); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("%q is still in the tree", named)
	}
	// And the scaffolding under it goes too, or the tree grows a directory per project that holds
	// nothing and says a workspace has sessions when it has none.
	if _, err := os.Lstat(filepath.Join(tree, "itv"+sandbox.SessionsSuffix)); !errors.Is(err, os.ErrNotExist) {
		t.Error("the workspace's session directory is still there and holds nothing")
	}
	// The workspace itself is untouched: archiving a session is not deleting a workspace.
	if _, err := os.Readlink(filepath.Join(tree, "itv")); err != nil {
		t.Errorf("the workspace lost its name: %v", err)
	}
}

// The scenario this step is measured by: a workspace is deleted, and the tree holds nothing pointing
// at it.
func TestADeletedWorkspaceLeavesNoNameBehind(t *testing.T) {
	storage, tree := wholeTree(t)
	cfg := sandbox.Config{Workspace: "9e8153f6d4c1", Project: "b75f5bf62544", ID: "aa01bb02cc03"}
	held := sandbox.Held{
		Workspaces: []sandbox.WorkspaceName{
			{ID: "9e8153f6d4c1", Name: "itv"}, {ID: "b75f5bf62544", Name: "sky"},
		},
		Sessions: []sandbox.SessionName{{Config: cfg, Names: sandbox.Names{
			Workspace: "itv", Project: "vast", Session: "the-login-that-times-out",
		}}},
	}
	if err := storage.WriteNames(held); err != nil {
		t.Fatalf("the first: %v", err)
	}

	held.Workspaces = held.Workspaces[1:]
	held.Sessions = nil
	if err := storage.WriteNames(held); err != nil {
		t.Fatalf("after the delete: %v", err)
	}

	gone, err := storage.SharedDirectory("9e8153f6d4c1")
	if err != nil {
		t.Fatalf("SharedDirectory: %v", err)
	}
	err = filepath.WalkDir(tree, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink == 0 {
			return nil
		}
		points, err := os.Readlink(path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(points, gone.Host) {
			return fmt.Errorf("%q still points at the deleted workspace, at %q", path, points)
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}
	if _, err := os.Lstat(filepath.Join(tree, "itv")); !errors.Is(err, os.ErrNotExist) {
		t.Error("the deleted workspace still has a name in the tree")
	}
	// The workspace that was not deleted keeps its name, so the sweep takes what went and nothing else.
	if _, err := os.Readlink(filepath.Join(tree, "sky")); err != nil {
		t.Errorf("the workspace that is still there lost its name: %v", err)
	}
}

// A directory the store still holds a name for is made again rather than left pointed at by a link
// that resolves to nothing. A name that opens nothing reads exactly like a name that is wrong.
func TestANameWhoseDirectoryWentIsRepaired(t *testing.T) {
	storage, tree := wholeTree(t)
	held := sandbox.Held{Workspaces: []sandbox.WorkspaceName{{ID: "9e8153f6d4c1", Name: "itv"}}}
	if err := storage.WriteNames(held); err != nil {
		t.Fatalf("the first: %v", err)
	}
	shared, err := storage.SharedDirectory("9e8153f6d4c1")
	if err != nil {
		t.Fatalf("SharedDirectory: %v", err)
	}
	if err := os.RemoveAll(shared.Host); err != nil {
		t.Fatalf("take the directory away: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tree, "itv")); err == nil {
		t.Fatal("the name still opens something, so this proves nothing about a dangling one")
	}

	if err := storage.WriteNames(held); err != nil {
		t.Fatalf("the repair: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tree, "itv")); err != nil {
		t.Errorf("the name still points at nothing: %v", err)
	}
}

// A name whose link points somewhere else is moved onto the directory the store says, which is the
// half creating a name cannot do: creation refuses, because it does not know which of the two is
// right, and the sweep reads the store and does.
func TestANameLeftPointingAtTheWrongDirectoryIsMoved(t *testing.T) {
	storage, tree := wholeTree(t)
	if err := os.MkdirAll(tree, 0o777); err != nil {
		t.Fatalf("the tree: %v", err)
	}
	elsewhere := filepath.Join(t.TempDir(), "somewhere-else")
	if err := os.MkdirAll(elsewhere, 0o777); err != nil {
		t.Fatalf("somewhere else: %v", err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(tree, "itv")); err != nil {
		t.Fatalf("the stale name: %v", err)
	}

	if err := storage.WriteNames(sandbox.Held{
		Workspaces: []sandbox.WorkspaceName{{ID: "9e8153f6d4c1", Name: "itv"}},
	}); err != nil {
		t.Fatalf("WriteNames: %v", err)
	}

	shared, err := storage.SharedDirectory("9e8153f6d4c1")
	if err != nil {
		t.Fatalf("SharedDirectory: %v", err)
	}
	points, err := os.Readlink(filepath.Join(tree, "itv"))
	if err != nil {
		t.Fatalf("read the name: %v", err)
	}
	if points != shared.Host {
		t.Errorf("the name points at %q, want the shared folder %q", points, shared.Host)
	}
}

// Two workspaces may hold one name, and the sweep has to answer the same way every time it runs, or
// the tree sends a file to one workspace this morning and the other one this afternoon. The first
// the store gives keeps it, which is the older one.
func TestOneNameHeldByTwoWorkspacesAlwaysAnswersTheSameWay(t *testing.T) {
	storage, tree := wholeTree(t)
	held := sandbox.Held{Workspaces: []sandbox.WorkspaceName{
		{ID: "9e8153f6d4c1", Name: "itv"}, {ID: "b75f5bf62544", Name: "itv"},
	}}

	for run := range 2 {
		// The one that did not get the name is said out loud rather than dropped, because a person
		// dropping a file for it is about to put it in the other workspace's folder.
		err := storage.WriteNames(held)
		if err == nil {
			t.Fatalf("run %d: nothing was said about the workspace that did not get the name", run)
		}
		if !strings.Contains(err.Error(), "two workspaces") {
			t.Errorf("run %d: it says %q, and it does not say the name is held twice", run, err)
		}
		shared, err := storage.SharedDirectory("9e8153f6d4c1")
		if err != nil {
			t.Fatalf("SharedDirectory: %v", err)
		}
		points, err := os.Readlink(filepath.Join(tree, "itv"))
		if err != nil {
			t.Fatalf("run %d: read the name: %v", run, err)
		}
		if points != shared.Host {
			t.Errorf("run %d: the name points at %q, want the first workspace's folder %q", run, points, shared.Host)
		}
	}
}

// The sweep takes away the names it writes and nothing else. A file or a directory somebody put here
// by hand was not written by this view, so removing it is not this view's business.
func TestTheSweepLeavesWhatItDidNotWrite(t *testing.T) {
	storage, tree := wholeTree(t)
	if err := os.MkdirAll(filepath.Join(tree, "my-own-notes"), 0o777); err != nil {
		t.Fatalf("the directory somebody made: %v", err)
	}
	mine := filepath.Join(tree, "a-file-somebody-left.txt")
	if err := os.WriteFile(mine, []byte("keep me"), 0o666); err != nil {
		t.Fatalf("the file somebody left: %v", err)
	}

	if err := storage.WriteNames(sandbox.Held{
		Workspaces: []sandbox.WorkspaceName{{ID: "9e8153f6d4c1", Name: "itv"}},
	}); err != nil {
		t.Fatalf("WriteNames: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tree, "my-own-notes")); err != nil {
		t.Errorf("the directory somebody made is gone: %v", err)
	}
	body, err := os.ReadFile(mine)
	if err != nil {
		t.Errorf("the file somebody left is gone: %v", err)
	}
	if string(body) != "keep me" {
		t.Errorf("the file holds %q, want what was in it", body)
	}
}

// A system told no tree sweeps nothing, the way it writes nothing: running without a tree is a way of
// running the system rather than a failure.
func TestNoTreeConfiguredSweepsNothing(t *testing.T) {
	dir := t.TempDir()
	storage := sandbox.Storage{Dir: dir, Host: dir}

	if err := storage.WriteNames(sandbox.Held{
		Workspaces: []sandbox.WorkspaceName{{ID: "9e8153f6d4c1", Name: "itv"}},
	}); err != nil {
		t.Errorf("WriteNames: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "itv")); !errors.Is(err, os.ErrNotExist) {
		t.Error("a name was written into the data directory")
	}
}

// The tree is built from the store rather than added to as things are made, so a workspace that
// existed before any of this shipped gets its name on the next sweep.
func TestAWorkspaceThatWasAlreadyThereGetsItsNameOnASweep(t *testing.T) {
	storage, tree := wholeTree(t)

	if err := storage.WriteNames(sandbox.Held{Workspaces: []sandbox.WorkspaceName{
		{ID: "9e8153f6d4c1", Name: "itv"},
	}}); err != nil {
		t.Fatalf("WriteNames: %v", err)
	}

	shared, err := storage.SharedDirectory("9e8153f6d4c1")
	if err != nil {
		t.Fatalf("SharedDirectory: %v", err)
	}
	points, err := os.Readlink(filepath.Join(tree, "itv"))
	if err != nil {
		t.Fatalf("the workspace has no name: %v", err)
	}
	if points != shared.Host {
		t.Errorf("the name points at %q, want the shared folder %q", points, shared.Host)
	}
}

// The word the top of the data directory uses never becomes a name, on the sweep as on creation. A
// workspace cannot be called it, so nothing reaches here holding it, and a sweep that wrote it would
// be the one place in the system offering a road to the tokens and the sealing key.
func TestTheSweepNeverWritesTheSystemsOwnWord(t *testing.T) {
	storage, tree := wholeTree(t)

	err := storage.WriteNames(sandbox.Held{Workspaces: []sandbox.WorkspaceName{
		{ID: "9e8153f6d4c1", Name: "system"}, {ID: "b75f5bf62544", Name: "itv"},
	}})
	if err == nil {
		t.Fatal("the sweep said nothing about the name it would not write")
	}
	if !strings.Contains(err.Error(), "sealing key") {
		t.Errorf("it says %q, and it does not say what is in that directory", err)
	}

	if _, err := os.Lstat(filepath.Join(tree, "system")); !errors.Is(err, os.ErrNotExist) {
		t.Error("the tree holds a name called system")
	}
	// And the one beside it is still written, so a name that cannot be used costs the tree one name
	// rather than the whole sweep.
	if _, err := os.Readlink(filepath.Join(tree, "itv")); err != nil {
		t.Errorf("the workspace beside it has no name: %v", err)
	}
}

// A directory standing where a name should go is refused rather than replaced, on the sweep as on
// creation. The sweep knows which workspace the name belongs to; it does not know what is in a
// directory it never wrote, and removing it would take work with it.
func TestTheSweepRefusesANameSomethingElseOwnsRatherThanReplacingIt(t *testing.T) {
	storage, tree := wholeTree(t)
	mine := filepath.Join(tree, "itv")
	if err := os.MkdirAll(mine, 0o777); err != nil {
		t.Fatalf("the directory somebody made: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mine, "notes.md"), []byte("keep me"), 0o666); err != nil {
		t.Fatalf("what is in it: %v", err)
	}

	err := storage.WriteNames(sandbox.Held{
		Workspaces: []sandbox.WorkspaceName{{ID: "9e8153f6d4c1", Name: "itv"}},
	})

	if err == nil {
		t.Fatal("the sweep said nothing about the name it could not write")
	}
	body, readErr := os.ReadFile(filepath.Join(mine, "notes.md"))
	if readErr != nil {
		t.Fatalf("what was in the directory is gone: %v", readErr)
	}
	if string(body) != "keep me" {
		t.Errorf("the file holds %q, want what was in it", body)
	}
}
