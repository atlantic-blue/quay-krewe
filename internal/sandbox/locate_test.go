package sandbox_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/name"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
)

// A project's folder inside the workspace's shared folder.
//
// The failure it answers: a person holding a file was given the shared folder for `itv/vast` and the
// shared folder for `itv`, which are the same directory, so the address said one thing and the disk
// held another. Sessions had already settled on putting a project's files in a folder named after the
// project, by hand, and this makes that the thing the system does.

func TestAProjectAddressNamesAFolderInsideTheSharedOne(t *testing.T) {
	dir := t.TempDir()
	storage := sandbox.Storage{Dir: dir, Host: dir}

	shared, err := storage.SharedDirectory("a-workspace")
	if err != nil {
		t.Fatalf("SharedDirectory: %v", err)
	}
	project, err := storage.ProjectDirectory("a-workspace", "vast")
	if err != nil {
		t.Fatalf("ProjectDirectory: %v", err)
	}

	if got := filepath.Dir(project.Host); got != shared.Host {
		t.Errorf("the vast folder is at %q, which is in %q and not in the shared folder %q",
			project.Host, got, shared.Host)
	}
	if filepath.Base(project.Host) != "vast" {
		t.Errorf("the folder is %q, want it named after the project", project.Host)
	}
	if project.Sandbox != sandbox.SharedPath+"/vast" {
		t.Errorf("a session reads it at %q, want %q", project.Sandbox, sandbox.SharedPath+"/vast")
	}
	if project.Kind != sandbox.KindProject {
		t.Errorf("the answer calls it %q, want %q", project.Kind, sandbox.KindProject)
	}
}

// The old answer, kept as a test of its own so the change is visible rather than silent. A workspace
// address is still the shared folder, and a session address is still that session's own directory.
func TestAWorkspaceAddressStillNamesTheSharedFolderItself(t *testing.T) {
	dir := t.TempDir()
	storage := sandbox.Storage{Dir: dir, Host: dir}

	shared, err := storage.SharedDirectory("a-workspace")
	if err != nil {
		t.Fatalf("SharedDirectory: %v", err)
	}
	if shared.Sandbox != sandbox.SharedPath {
		t.Errorf("a session reads the shared folder at %q, want %q", shared.Sandbox, sandbox.SharedPath)
	}
	if shared.Kind != sandbox.KindShared {
		t.Errorf("the answer calls the shared folder %q, want %q", shared.Kind, sandbox.KindShared)
	}

	working, err := storage.WorkingDirectory(sandbox.Config{
		Workspace: "a-workspace", Project: "a-project", ID: "a-session",
	})
	if err != nil {
		t.Fatalf("WorkingDirectory: %v", err)
	}
	if working.Sandbox != sandbox.WorkingPath {
		t.Errorf("a session reads its own directory at %q, want %q", working.Sandbox, sandbox.WorkingPath)
	}
	if working.Kind != sandbox.KindWorking {
		t.Errorf("the answer calls a session's own directory %q, want %q", working.Kind, sandbox.KindWorking)
	}
}

// The directory is made, for the reason the shared folder is made: a project nothing has run in has
// no folder yet, and that is exactly when somebody wants to leave it a file.
func TestAProjectFolderIsMadeBeforeAnythingRuns(t *testing.T) {
	dir := t.TempDir()
	storage := sandbox.Storage{Dir: dir, Host: dir}

	project, err := storage.ProjectDirectory("a-workspace", "vast")
	if err != nil {
		t.Fatalf("ProjectDirectory: %v", err)
	}
	info, err := os.Stat(project.Host)
	if err != nil {
		t.Fatalf("the answer named %q, which is not on the machine: %v", project.Host, err)
	}
	if !info.IsDir() {
		t.Fatalf("the answer named %q, which is not a directory", project.Host)
	}
}

// A system that keeps nothing on disk has no directory to name, which is a way of running it rather
// than a fault, and the caller has to be able to tell the two apart.
func TestAProjectFolderOnASystemThatKeepsNothingSaysSo(t *testing.T) {
	if _, err := (sandbox.Storage{}).ProjectDirectory("a-workspace", "vast"); err == nil {
		t.Fatal("a storage that keeps nothing answered with a directory")
	}
}

// The class guard behind the reserved names.
//
// Naming a project's folder after the project puts it beside the folders this system writes in the
// shared volume itself. Reserving the two that exist today fixes today and nothing else, so this
// reads the constants and requires every one of them to be a name a project cannot take. A third
// folder added to the layout fails here, at the moment it is added, rather than the first time
// somebody makes a project of that name and quietly copies a file onto a session's checkout.
func TestEveryFolderTheSystemWritesInTheSharedVolumeIsRefusedAsAProjectName(t *testing.T) {
	folders := foldersUnderTheSharedPath(t)
	if len(folders) == 0 {
		t.Fatal("no constant in this package builds a path under the shared folder, so this proves nothing")
	}
	for constant, folder := range folders {
		if name.ReservedProject(folder) == "" {
			t.Errorf("%s writes %q inside the shared folder, and a project may still be called %q",
				constant, folder, folder)
		}
	}
}

// foldersUnderTheSharedPath reads the package's own source for constants of the shape
// `SharedPath + "/<folder>"`, and answers the folder each one names. It parses rather than matching
// text, so a comment that says the words does not count and a constant that says them does.
//
// Every file in the directory, tagged or not: a build tag hides a file from a package load, and a
// constant this cannot see is exactly the one nobody would think to reserve.
func foldersUnderTheSharedPath(t *testing.T) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading this package's directory: %v", err)
	}
	files := token.NewFileSet()
	found := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		parsed, err := parser.ParseFile(files, entry.Name(), nil, 0)
		if err != nil {
			t.Fatalf("reading %s: %v", entry.Name(), err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			spec, ok := node.(*ast.ValueSpec)
			if !ok {
				return true
			}
			for at, value := range spec.Values {
				if at >= len(spec.Names) {
					break
				}
				if folder, held := folderUnderTheSharedPath(value); held {
					found[spec.Names[at].Name] = folder
				}
			}
			return true
		})
	}
	return found
}

// folderUnderTheSharedPath answers the one folder `SharedPath + "/x"` names, and false for anything
// else. Only the first level counts: a path two deep is inside a folder that is already reserved.
func folderUnderTheSharedPath(value ast.Expr) (string, bool) {
	binary, ok := value.(*ast.BinaryExpr)
	if !ok || binary.Op != token.ADD {
		return "", false
	}
	left, ok := binary.X.(*ast.Ident)
	if !ok || left.Name != "SharedPath" {
		return "", false
	}
	right, ok := binary.Y.(*ast.BasicLit)
	if !ok || right.Kind != token.STRING {
		return "", false
	}
	rest, err := strconv.Unquote(right.Value)
	if err != nil {
		return "", false
	}
	trimmed := strings.TrimPrefix(path.Clean(rest), "/")
	if trimmed == "" {
		return "", false
	}
	first, _, _ := strings.Cut(trimmed, "/")
	return first, true
}
