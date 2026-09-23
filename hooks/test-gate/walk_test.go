package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The walk is what tells `rm -rf build/` from `rm -rf internal/`, and the two read the same to any
// rule about names. So it reads the disk, and these cases are the disk.
func TestWhatADirectoryHolds(t *testing.T) {
	root := t.TempDir()
	write := func(where string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, where)), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, where), []byte("x"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	write("internal/session/build.go")
	write("internal/session/build_test.go")
	write("internal/web/page.go")
	write("features/build.feature")
	write("build/binary")
	write(".git/objects/ab/cdef")
	write("web/node_modules/somepackage/index.test.js")

	holds := []struct {
		where string
		want  bool
		why   string
	}{
		{where: "internal/session", want: true, why: "it holds a file ending in _test.go"},
		{where: "internal", want: true, why: "the test is one directory down"},
		{where: ".", want: true, why: "the working directory holds all of it"},
		{where: "features", want: true, why: "the directory is named after the tests"},
		{where: "internal/web", want: false, why: "it holds one file and that file is not a test"},
		{where: "build", want: false, why: "a directory of output holds no test"},
		{where: "web", want: false, why: "the only test under it is inside node_modules"},
		{where: "internal/session/build_test.go", want: false, why: "a file is read by its name, not walked"},
		{where: "nothing/here", want: false, why: "a command cannot take away what is not there"},
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	for _, one := range holds {
		t.Run(one.where, func(t *testing.T) {
			if got := HoldsATest(one.where); got != one.want {
				t.Fatalf("%q holds a test: %v, want %v, because %s", one.where, got, one.want, one.why)
			}
		})
	}
}

// The mark says whether this session is building, and it is read off the disk for the reason the
// walk above is: a container already running cannot be handed an environment variable, so the
// boundary arrives as a file in the session's own directory.
func TestTheMarkSaysWhetherThisSessionIsBuilding(t *testing.T) {
	root := t.TempDir()
	elsewhere := t.TempDir()
	under := filepath.Join(root, "quay-krewe", "internal", "store")
	if err := os.MkdirAll(under, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	if Marked(root) {
		t.Fatal("a session nobody marked reads as building, so every session would be under the gate")
	}
	if Marked("") {
		t.Fatal("a runtime that said where nothing is put the session under the gate")
	}

	if err := os.MkdirAll(filepath.Join(root, MarkDir), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, MarkDir, MarkFile), []byte("building"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if !Marked(root) {
		t.Fatal("the session's own directory carries the mark and the gate reads it as off")
	}
	// A session clones the repository it works on into its own directory, so most commands run one
	// or two directories below the mark. A read of the working directory alone would find nothing
	// there and turn the gate off for the whole build.
	if !Marked(under) {
		t.Fatal("a command run inside the cloned repository is outside the gate")
	}
	if Marked(elsewhere) {
		t.Fatal("another directory on the same disk reads as building")
	}
}
