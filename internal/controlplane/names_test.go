package controlplane

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/model"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/secrets"
	"github.com/atlantic-blue/quay-krewe/internal/store"
)

// The tree of names against the store, through the calls an operator makes.
//
// The tree is a view, so what matters is that it says what the store says after each of the three
// things that move a name: a label changes, a session is put away, a workspace is deleted. The
// package below proves what one sweep does to a directory; these prove the system runs one.

// namingServer is a control plane that keeps a tree of names, with the tree's directory beside it.
func namingServer(t *testing.T) (*Server, string) {
	t.Helper()
	home := t.TempDir()
	data := filepath.Join(home, "data")
	tree := filepath.Join(home, "at")
	if err := os.MkdirAll(data, 0o777); err != nil {
		t.Fatalf("the data directory: %v", err)
	}
	return NewServer(Config{
		Store:    store.NewMemory(),
		Runner:   &model.FakeRunner{Reply: "done"},
		Provider: &sandbox.FakeProvider{},
		Secrets:  secrets.NewMemory(),
		Storage:  sandbox.Storage{Dir: data, Host: data, NameTree: tree},
	}), tree
}

// namedSession is a workspace called itv, a project called vast, and one session in it dispatched
// with a title, which is what it is called until somebody labels it.
func namedSession(t *testing.T, server *Server, title string) (workspace, session string) {
	t.Helper()
	ctx := context.Background()
	made, err := server.CreateWorkspace(ctx, &quaycrewv1.CreateWorkspaceRequest{Name: "itv"})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	project, err := server.CreateProject(ctx, &quaycrewv1.CreateProjectRequest{
		Workspace: made.GetWorkspace().GetId(), Name: "vast",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	dispatched, err := server.Dispatch(ctx, &quaycrewv1.DispatchRequest{
		Project: project.GetProject().GetId(), Text: "look at the login", Title: title,
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	return made.GetWorkspace().GetId(), dispatched.GetId()
}

// sessionNamed is where one session's name sits in the tree.
func sessionNamed(tree, called string) string {
	return filepath.Join(tree, "itv"+sandbox.SessionsSuffix, "vast", called)
}

// The operator renames a conversation, and the name in the tree is the name in the listing. A tree
// that kept the old one would file the session twice and send a file to whichever the person read
// first.
func TestLabellingASessionMovesItsNameInTheTree(t *testing.T) {
	server, tree := namingServer(t)
	_, session := namedSession(t, server, "the login that times out")
	if _, err := os.Readlink(sessionNamed(tree, "the-login-that-times-out")); err != nil {
		t.Fatalf("the session was never named: %v", err)
	}

	if _, err := server.SetSessionLabel(context.Background(), &quaycrewv1.SetSessionLabelRequest{
		Id: session, Label: "the checkout that times out",
	}); err != nil {
		t.Fatalf("SetSessionLabel: %v", err)
	}

	if _, err := os.Readlink(sessionNamed(tree, "the-checkout-that-times-out")); err != nil {
		t.Errorf("the session is not under the name it was given: %v", err)
	}
	if _, err := os.Lstat(sessionNamed(tree, "the-login-that-times-out")); !errors.Is(err, os.ErrNotExist) {
		t.Error("the session is still under the name it had, so the tree holds it twice")
	}
}

// Archiving hides a session from the listing, and the tree is the same view of the same store.
// Restoring brings it back, because archiving deletes nothing.
func TestArchivingASessionTakesItsNameAndRestoringBringsItBack(t *testing.T) {
	server, tree := namingServer(t)
	_, session := namedSession(t, server, "the login that times out")
	ctx := context.Background()

	if _, err := server.ArchiveSession(ctx, &quaycrewv1.ArchiveSessionRequest{Id: session}); err != nil {
		t.Fatalf("ArchiveSession: %v", err)
	}
	if _, err := os.Lstat(sessionNamed(tree, "the-login-that-times-out")); !errors.Is(err, os.ErrNotExist) {
		t.Error("a session that was put away is still in the tree")
	}

	if _, err := server.RestoreSession(ctx, &quaycrewv1.RestoreSessionRequest{Id: session}); err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}
	if _, err := os.Readlink(sessionNamed(tree, "the-login-that-times-out")); err != nil {
		t.Errorf("a session brought back has no name: %v", err)
	}
}

// The scenario this step is measured by, at the layer that runs the sweep: a workspace is deleted,
// and the tree holds nothing pointing at it.
func TestDeletingAWorkspaceLeavesNothingInTheTree(t *testing.T) {
	server, tree := namingServer(t)
	workspace, _ := namedSession(t, server, "the login that times out")

	if _, err := server.DeleteWorkspace(context.Background(), &quaycrewv1.DeleteWorkspaceRequest{
		Id: workspace,
	}); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}

	held, err := os.ReadDir(tree)
	if err != nil {
		t.Fatalf("read the tree: %v", err)
	}
	if len(held) != 0 {
		t.Errorf("the tree still holds %v", named(held))
	}
}

// Deleting a project takes the names of the sessions in it, and leaves the workspace where it is.
func TestDeletingAProjectTakesTheNamesOfTheSessionsInIt(t *testing.T) {
	server, tree := namingServer(t)
	ctx := context.Background()
	workspace, session := namedSession(t, server, "the login that times out")
	held, err := server.GetSession(ctx, &quaycrewv1.GetSessionRequest{Id: session})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	if _, err := server.DeleteProject(ctx, &quaycrewv1.DeleteProjectRequest{
		Id: held.GetSession().GetProject(),
	}); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	if _, err := os.Lstat(sessionNamed(tree, "the-login-that-times-out")); !errors.Is(err, os.ErrNotExist) {
		t.Error("a session of a deleted project is still in the tree")
	}
	if _, err := os.Readlink(filepath.Join(tree, "itv")); err != nil {
		t.Errorf("the workspace lost its name when one of its projects was deleted: %v", err)
	}
	if workspace == "" {
		t.Error("the workspace has no identifier")
	}
}

// The tree is built from the store rather than added to as things are made, so a system that comes up
// to a tree somebody deleted, or to workspaces made before any of this shipped, writes the names on
// the way up.
func TestASweepWritesTheNamesOfEverythingTheStoreAlreadyHeld(t *testing.T) {
	server, tree := namingServer(t)
	namedSession(t, server, "the login that times out")
	if err := os.RemoveAll(tree); err != nil {
		t.Fatalf("take the tree away: %v", err)
	}

	server.SweepNames(context.Background())

	if _, err := os.Readlink(filepath.Join(tree, "itv")); err != nil {
		t.Errorf("the workspace has no name after the sweep: %v", err)
	}
	if _, err := os.Readlink(sessionNamed(tree, "the-login-that-times-out")); err != nil {
		t.Errorf("the session has no name after the sweep: %v", err)
	}
}

// A session nobody has called anything has only its identifier left, and the tree holds names, so the
// sweep leaves it out rather than filing it under one.
func TestASweepLeavesOutASessionWithNoNameOfItsOwn(t *testing.T) {
	server, tree := namingServer(t)
	namedSession(t, server, "")

	server.SweepNames(context.Background())

	if _, err := os.Lstat(filepath.Join(tree, "itv"+sandbox.SessionsSuffix)); !errors.Is(err, os.ErrNotExist) {
		t.Error("a session with no name of its own was filed in the tree")
	}
}

// named is what a directory holds, for a failure that has to say what it found.
func named(entries []os.DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Name())
	}
	return out
}

// A session nobody labelled is called by the line the system wrote about it, so writing that line
// renames it. The tree follows that rename like any other, or the name a listing shows and the name
// on the filesystem are two different words for one session.
func TestDescribingASessionMovesItsNameInTheTree(t *testing.T) {
	home := t.TempDir()
	data := filepath.Join(home, "data")
	tree := filepath.Join(home, "at")
	if err := os.MkdirAll(data, 0o777); err != nil {
		t.Fatalf("the data directory: %v", err)
	}
	runner := &describingRunner{Says: "the login that times out"}
	server := NewServer(Config{
		Store: store.NewMemory(), Runner: runner,
		Provider: &sandbox.FakeProvider{}, Secrets: secrets.NewMemory(),
		Storage:       sandbox.Storage{Dir: data, Host: data, NameTree: tree},
		DescribeEvery: 10,
	})
	ctx := context.Background()
	workspace, err := server.CreateWorkspace(ctx, &quaycrewv1.CreateWorkspaceRequest{Name: "itv"})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	project, err := server.CreateProject(ctx, &quaycrewv1.CreateProjectRequest{
		Workspace: workspace.GetWorkspace().GetId(), Name: "vast",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	if _, err := server.Dispatch(ctx, &quaycrewv1.DispatchRequest{
		Project: project.GetProject().GetId(), Text: "look at the login",
	}); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	// Describing runs behind the answer, so a case that read the tree now would be racing it.
	server.describing.Wait()

	if runner.Described == 0 {
		t.Fatal("the session was never described, so this proves nothing about the name")
	}
	if _, err := os.Readlink(sessionNamed(tree, "the-login-that-times-out")); err != nil {
		t.Errorf("the session is not under the line the system wrote about it: %v", err)
	}
}

// Putting a whole project's sessions away is the same view of the same store as putting one away, so
// the names go with them.
func TestArchivingAProjectsSessionsTakesTheirNames(t *testing.T) {
	server, tree := namingServer(t)
	ctx := context.Background()
	_, session := namedSession(t, server, "the login that times out")
	held, err := server.GetSession(ctx, &quaycrewv1.GetSessionRequest{Id: session})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	// The sweep takes the sessions that hold no container, so this one is put down first.
	if _, err := server.StopSession(ctx, &quaycrewv1.StopSessionRequest{Id: session}); err != nil {
		t.Fatalf("StopSession: %v", err)
	}

	swept, err := server.ArchiveProjectSessions(ctx, &quaycrewv1.ArchiveProjectSessionsRequest{
		Project: held.GetSession().GetProject(),
	})
	if err != nil {
		t.Fatalf("ArchiveProjectSessions: %v", err)
	}
	if len(swept.GetArchived()) != 1 {
		t.Fatalf("the sweep put %d sessions away, so this proves nothing about a name", len(swept.GetArchived()))
	}

	if _, err := os.Lstat(sessionNamed(tree, "the-login-that-times-out")); !errors.Is(err, os.ErrNotExist) {
		t.Error("a session the sweep put away is still in the tree")
	}
}
