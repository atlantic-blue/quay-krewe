package controlplane

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/model"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/secrets"
	"github.com/atlantic-blue/quay-krewe/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CHECK-2: the directory a check runs in, and the refusal where there is none.
//
// A run pointed at a directory with no checkout in it reports no scenarios. Krewe records that as a
// failing verdict, and the operator then reads a fault in code that never ran. So the directory is
// read before anything starts, and a session with no checkout is refused.

// checkingServer is a control plane whose storage keeps its directories under one temporary
// directory, which is what the work places are computed from.
func checkingServer(t *testing.T) *Server {
	t.Helper()
	data := t.TempDir()
	return NewServer(Config{
		Store:    store.NewMemory(),
		Runner:   &model.FakeRunner{Reply: "done"},
		Provider: &sandbox.FakeProvider{},
		Secrets:  secrets.NewMemory(),
		Storage:  sandbox.Storage{Dir: data, Host: data},
	})
}

// aSessionOnAStep is a session of the workspace itv, in the project vast, holding step 3.
func aSessionOnAStep() (*quaycrewv1.Session, *quaycrewv1.Step) {
	return &quaycrewv1.Session{Id: "9e8153f6", Workspace: "itv", Project: "vast"},
		&quaycrewv1.Step{Number: 3}
}

// aCheckoutIn writes the name a repository is read by, so the directory reads as one.
func aCheckoutIn(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o777); err != nil {
		t.Fatalf("the checkout in %q: %v", dir, err)
	}
}

// The session cloned into its own directory, which is what the git skill teaches where a system
// keeps no volume. The run is pointed at the path that directory has inside the container.
func TestCheckTwoRunsInTheSessionsOwnCheckout(t *testing.T) {
	server := checkingServer(t)
	session, held := aSessionOnAStep()
	dir, kept := server.storage.WorkingDir(boxOf(session))
	if !kept {
		t.Fatal("this storage keeps no working directory, so there is nowhere to put a checkout")
	}
	aCheckoutIn(t, dir)

	where, err := server.whereTheWorkIs(session, held)
	if err != nil {
		t.Fatalf("the check refused a session that holds a checkout: %v", err)
	}
	if where != sandbox.WorkingPath {
		t.Fatalf("the run is pointed at %q, want %q", where, sandbox.WorkingPath)
	}
}

// The working tree the git skill names, which is the shape a session takes where the workspace keeps
// a volume. The repository sits one directory inside the tree, under its own name.
func TestCheckTwoRunsInTheWorkingTreeTheSessionTook(t *testing.T) {
	server := checkingServer(t)
	session, held := aSessionOnAStep()
	volume, kept := server.storage.VolumeDir(session.GetWorkspace())
	if !kept {
		t.Fatal("this storage keeps no volume, so no session in it can take a working tree")
	}
	aCheckoutIn(t, filepath.Join(volume, "worktrees", session.GetId(), "quay-krewe"))

	where, err := server.whereTheWorkIs(session, held)
	if err != nil {
		t.Fatalf("the check refused a session that took a working tree: %v", err)
	}
	want := sandbox.WorktreesPath + "/" + session.GetId() + "/quay-krewe"
	if where != want {
		t.Fatalf("the run is pointed at %q, want %q", where, want)
	}
}

// The refusal. It names every directory it read, because the operator's next move is to go and look
// in them, and it names the working tree to take.
func TestCheckTwoRefusesASessionWithNoCheckout(t *testing.T) {
	server := checkingServer(t)
	session, held := aSessionOnAStep()

	where, err := server.whereTheWorkIs(session, held)
	if err == nil {
		t.Fatalf("a session with no checkout was answered with %q, and nothing can run there", where)
	}
	if got := status.Code(err); got != codes.FailedPrecondition {
		t.Fatalf("the refusal is %v, want %v", got, codes.FailedPrecondition)
	}
	for _, want := range []string{
		"step 3",
		sandbox.WorkingPath,
		sandbox.WorktreesPath + "/" + session.GetId(),
		"Take the working tree the git skill names",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal is %q, want it to carry %q", err.Error(), want)
		}
	}
}

// A system that keeps nothing on disk has no directory to read at all, so the refusal cannot name
// one. It says that instead of naming an empty list.
func TestCheckTwoRefusesASystemThatKeepsNoDirectory(t *testing.T) {
	server := NewServer(Config{
		Store:    store.NewMemory(),
		Runner:   &model.FakeRunner{Reply: "done"},
		Provider: &sandbox.FakeProvider{},
		Secrets:  secrets.NewMemory(),
	})
	session, held := aSessionOnAStep()

	where, err := server.whereTheWorkIs(session, held)
	if err == nil {
		t.Fatalf("a system that keeps no directory answered with %q, and nothing can run there", where)
	}
	if got := status.Code(err); got != codes.FailedPrecondition {
		t.Fatalf("the refusal is %v, want %v", got, codes.FailedPrecondition)
	}
	if !strings.Contains(err.Error(), "keeps no directory") {
		t.Fatalf("the refusal is %q, want it to say the system keeps no directory", err.Error())
	}
}

// The first of the two cuts. A run that prints without stopping is read to the end and only its last
// 64 kibibytes are held, and that cut is by byte count, so it can fall between the bytes of one
// character. What comes out then starts with the rest of a character, which is not text any database
// takes and not text the proto wire format carries.
func TestTheEndOfARunIsCutOnACharacterBoundary(t *testing.T) {
	// The bullet a test runner prints at the head of a failure. It is three bytes, and it sits one
	// byte before the cut, so the cut falls inside it.
	const dot = "●"
	const before = 999
	ending := "\n1 scenarios (0 passed, 1 failed)\nthe end of the run\n"
	printed := strings.Repeat("a", before) + dot +
		strings.Repeat("b", proofOutputRead+1_000-before-len(dot)-len(ending)) + ending

	kept := theEndOfTheRun(strings.NewReader(printed))

	if !utf8.ValidString(kept) {
		t.Fatalf("the run was cut inside a character: what is kept starts %q", kept[:min(16, len(kept))])
	}
	if len(kept) > proofOutputRead {
		t.Errorf("the run keeps %d bytes, and the ceiling is %d", len(kept), proofOutputRead)
	}
	// Forward to the next character and never back, so at most one whole character is lost.
	if len(kept) < proofOutputRead-utf8.UTFMax {
		t.Errorf("the run keeps %d bytes, and only the character across the cut is dropped", len(kept))
	}
	if !strings.HasSuffix(printed, kept) {
		t.Error("what is kept is not the end of what the run printed")
	}
	if !strings.HasSuffix(kept, ending) {
		t.Error("the end of the run, which carries the count, was cut off")
	}
}

// A run whose output is no character at all. Nothing here is text, so there is no boundary to move
// to, and the cut still holds the ceiling rather than reading on forever.
func TestARunThatPrintsNoCharactersIsStillCutToTheCeiling(t *testing.T) {
	printed := strings.Repeat("\xff", proofOutputRead+1_000)

	kept := theEndOfTheRun(strings.NewReader(printed))

	if len(kept) > proofOutputRead {
		t.Errorf("the run keeps %d bytes, and the ceiling is %d", len(kept), proofOutputRead)
	}
}
