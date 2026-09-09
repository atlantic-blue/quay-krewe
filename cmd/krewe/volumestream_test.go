package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/controlplane"
	"github.com/atlantic-blue/quay-krewe/internal/model"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/secrets"
	"github.com/atlantic-blue/quay-krewe/internal/store"
)

// krewe volume cp: the bytes travel through the control plane.
//
// The failure it answers: a volume is a directory on the machine that runs the sandboxes. The tool
// used to open that directory itself, which works while the two are one machine and reaches nothing
// under Kubernetes, where the volume is beside the control plane.
//
// Two things are proved here rather than anywhere else. A file above the four mebibyte message
// ceiling goes in and comes back, which is what says the pieces are real rather than one larger
// message. And the same copy works against a system whose volume this process cannot open at all.

// aboveTheMessageCeiling is a file bigger than a call may carry in one message.
//
// The default ceiling is four mebibytes, and this is one byte over it. A message of this size is
// refused by the runtime before any of this code reads it, so a copy that passes at this size is a
// copy that sent pieces.
const aboveTheMessageCeiling = 4<<20 + 1

// aVolumeSomewhereElse stands a system up whose volume this process cannot open.
//
// The data directory is real, because the control plane writes in it. The host path is not, and that
// is the whole of the difference: it is the path the machine running the sandboxes would use, and
// under Kubernetes it names nothing in this process. Every answer the tool is given carries it, so a
// tool that opens a path instead of asking reaches nothing.
//
// It answers with the data directory and the host path, so a test can say where the bytes landed and
// that nothing was written through the path that reaches nowhere.
func aVolumeSomewhereElse(t *testing.T) (quaycrewv1.ControlPlaneServiceClient, string, string) {
	t.Helper()
	dir := t.TempDir()
	elsewhere := filepath.Join(t.TempDir(), "on-another-machine")
	client := testClientWith(t, controlplane.Config{
		Store: store.NewMemory(), Runner: &model.FakeRunner{Reply: "ok"},
		Provider: &sandbox.FakeProvider{}, Secrets: secrets.NewMemory(),
		Storage: sandbox.Storage{Dir: dir, Host: elsewhere},
	})
	mustRun(t, client, "workspace", "create", "acme")
	mustRun(t, client, "project", "create", "house-bills")
	return client, dir, elsewhere
}

// theProjectFolder is where the bytes actually are, read from the data directory rather than from
// what the tool printed. The tool prints the path a session reads, and a copy that answered with the
// right words and wrote nowhere would pass against that.
func theProjectFolder(dir, workspace string) string {
	return filepath.Join(dir, "workspaces", workspace, "volume", "house-bills")
}

// theWorkspaceID is the one workspace this system holds. The folder on disk is named after the
// identifier, and nothing on the command line ever says it.
func theWorkspaceID(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, "workspaces"))
	if err != nil {
		t.Fatalf("read the workspaces on disk: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("the system holds %d workspaces on disk, want the one this test made", len(entries))
	}
	return entries[0].Name()
}

// The step this whole change exists for: the volume is somewhere this process cannot open, and the
// copy still puts the file in front of the sessions that read the address.
func TestVolumeCopyReachesAVolumeThisMachineCannotOpen(t *testing.T) {
	client, dir, elsewhere := aVolumeSomewhereElse(t)
	source, sent := aFileOf(t, "Explore-logs.txt", theLogFile)

	said := mustRun(t, client, "volume", "cp", source, "krewe://acme/house-bills")

	folder := theProjectFolder(dir, theWorkspaceID(t, dir))
	arrived, err := os.ReadFile(filepath.Join(folder, "Explore-logs.txt"))
	if err != nil {
		t.Fatalf("the file is not in the directory a session reads: %v", err)
	}
	if !bytes.Equal(arrived, sent) {
		t.Fatalf("the file arrived at %d bytes, want the %d that were sent", len(arrived), len(sent))
	}
	if got := firstLineOf(said); got != sandbox.SharedPath+"/house-bills/Explore-logs.txt" {
		t.Fatalf("the copy printed %q, want the path a session reads the file at", got)
	}
	if _, err := os.Stat(elsewhere); !os.IsNotExist(err) {
		t.Fatalf("%s was opened, so the tool reached for a host path: %v", elsewhere, err)
	}
}

// The other direction against the same system, which is the half a person uses to get a log back.
func TestVolumeCopyOutOfAVolumeThisMachineCannotOpen(t *testing.T) {
	client, _, elsewhere := aVolumeSomewhereElse(t)
	source, sent := aFileOf(t, "Explore-logs.txt", theLogFile)
	mustRun(t, client, "volume", "cp", source, "krewe://acme/house-bills")
	back := filepath.Join(t.TempDir(), "yesterday.txt")

	mustRun(t, client, "volume", "cp", "krewe://acme/house-bills/Explore-logs.txt", back)

	arrived, err := os.ReadFile(back)
	if err != nil {
		t.Fatalf("the file did not land on this machine: %v", err)
	}
	if !bytes.Equal(arrived, sent) {
		t.Fatalf("the file came back at %d bytes, want the %d that went in", len(arrived), len(sent))
	}
	if _, err := os.Stat(elsewhere); !os.IsNotExist(err) {
		t.Fatalf("%s was opened, so the tool reached for a host path: %v", elsewhere, err)
	}
}

// A file bigger than one message. It goes in and comes back byte for byte, which is what says the
// pieces are real: at this size a single message is refused before either end reads it.
func TestVolumeCopyCarriesAFileAboveTheMessageCeilingBothWays(t *testing.T) {
	client, dir, _ := aVolumeSomewhereElse(t)
	source, sent := aFileOf(t, "Explore-logs.txt", aboveTheMessageCeiling)
	back := filepath.Join(t.TempDir(), "yesterday.txt")

	mustRun(t, client, "volume", "cp", source, "krewe://acme/house-bills")
	mustRun(t, client, "volume", "cp", "krewe://acme/house-bills/Explore-logs.txt", back)

	folder := theProjectFolder(dir, theWorkspaceID(t, dir))
	inTheVolume, err := os.ReadFile(filepath.Join(folder, "Explore-logs.txt"))
	if err != nil {
		t.Fatalf("the file is not in the directory a session reads: %v", err)
	}
	if !bytes.Equal(inTheVolume, sent) {
		t.Fatalf("the file arrived at %d bytes, want the %d that were sent", len(inTheVolume), len(sent))
	}
	arrived, err := os.ReadFile(back)
	if err != nil {
		t.Fatalf("the file did not come back: %v", err)
	}
	if !bytes.Equal(arrived, sent) {
		t.Fatalf("the file came back at %d bytes, want the %d that went in", len(arrived), len(sent))
	}
}

// A file of no bytes sends no piece at all, so an empty file is the case where the stream carries
// nothing. It still has to arrive, and it still has to come back empty rather than as a refusal.
func TestVolumeCopyCarriesAFileOfNoBytes(t *testing.T) {
	client, dir, _ := aVolumeSomewhereElse(t)
	source, _ := aFileOf(t, "empty.txt", 0)
	back := filepath.Join(t.TempDir(), "empty.txt")

	mustRun(t, client, "volume", "cp", source, "krewe://acme/house-bills")
	mustRun(t, client, "volume", "cp", "krewe://acme/house-bills/empty.txt", back)

	folder := theProjectFolder(dir, theWorkspaceID(t, dir))
	info, err := os.Stat(filepath.Join(folder, "empty.txt"))
	if err != nil {
		t.Fatalf("the empty file is not in the directory a session reads: %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("the file arrived at %d bytes, want none", info.Size())
	}
	if arrived, err := os.ReadFile(back); err != nil || len(arrived) != 0 {
		t.Fatalf("the file came back as %q and %v, want an empty file", arrived, err)
	}
}
