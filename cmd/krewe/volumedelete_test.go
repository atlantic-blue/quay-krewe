package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/workspace"
)

// krewe volume delete: a file leaves a volume.
//
// The failure it answers: a volume only ever grew. Every other verb put something in one or read what
// was in it, so a file copied in by mistake, or one that is finished with, stayed where every session
// of the workspace reads it.
//
// Two things are proved here rather than anywhere else. One named file goes and nothing beside it
// does. And a folder is refused, because a recursive delete is out of scope and there is no way back
// from one.

// The step this whole feature exists for. One file goes, and the file beside it is untouched.
func TestVolumeDeleteRemovesTheNamedFileAndNothingBesideIt(t *testing.T) {
	client, folder := aVolumeToCopyInto(t)
	going, _ := aFileOf(t, "explore.txt", 64)
	staying, kept := aFileOf(t, "kept.txt", 128)
	mustRun(t, client, "volume", "cp", going, "krewe://acme/house-bills")
	mustRun(t, client, "volume", "cp", staying, "krewe://acme/house-bills")

	said := mustRun(t, client, "volume", "delete", "krewe://acme/house-bills/explore.txt")

	if _, err := os.Stat(filepath.Join(folder, "explore.txt")); !os.IsNotExist(err) {
		t.Fatalf("the file the address named is still in the volume: %v", err)
	}
	beside, err := os.ReadFile(filepath.Join(folder, "kept.txt"))
	if err != nil {
		t.Fatalf("the file beside it went too: %v", err)
	}
	if !bytes.Equal(beside, kept) {
		t.Fatal("the file beside it is there with the wrong bytes")
	}
	if got := firstLineOf(said); got != sandbox.SharedPath+"/house-bills/explore.txt" {
		t.Fatalf("the delete printed %q, want the path the file was at", got)
	}
}

// The scenario the step names. A delete is typed from memory, so the name is usually there under
// another spelling, and the refusal carries the names that are.
func TestVolumeDeleteOfANameThatIsNotThereSaysWhatTheDirectoryHolds(t *testing.T) {
	client, folder := aVolumeToCopyInto(t)
	source, _ := aFileOf(t, "explore.txt", 8)
	mustRun(t, client, "volume", "cp", source, "krewe://acme/house-bills")

	_, err := asked(t, client, "volume", "delete", "krewe://acme/house-bills/Explore-logs.txt")

	if err == nil {
		t.Fatal("a name that is not in the volume was deleted")
	}
	if !strings.Contains(err.Error(), "Explore-logs.txt") {
		t.Errorf("the refusal does not name what it looked for: %s", err)
	}
	if !strings.Contains(err.Error(), folder) {
		t.Errorf("the refusal does not name the directory it read: %s", err)
	}
	if !strings.Contains(err.Error(), "explore.txt") {
		t.Errorf("the refusal does not say what the directory holds: %s", err)
	}
}

// The same refusal where the directory holds nothing at all. The empty list is a sentence rather than
// a message that stops after the colon.
func TestVolumeDeleteFromAnEmptyVolumeSaysItHoldsNothing(t *testing.T) {
	client, _ := aVolumeToCopyInto(t)

	_, err := asked(t, client, "volume", "delete", "krewe://acme/house-bills/explore.txt")

	if err == nil {
		t.Fatal("a name in an empty volume was deleted")
	}
	if !strings.Contains(err.Error(), "holds nothing") {
		t.Errorf("the refusal does not say the directory is empty: %s", err)
	}
}

// A folder is refused rather than emptied. Taking one takes every file under it, and nothing here
// brings any of them back.
func TestVolumeDeleteOfAFolderInTheVolumeIsRefused(t *testing.T) {
	client, folder := aVolumeToCopyInto(t)
	if err := os.MkdirAll(filepath.Join(folder, "logs"), 0o777); err != nil {
		t.Fatalf("make the folder: %v", err)
	}
	inside := filepath.Join(folder, "logs", "explore.txt")
	if err := os.WriteFile(inside, []byte("what is in the folder"), 0o666); err != nil {
		t.Fatalf("write the file in the folder: %v", err)
	}

	_, err := asked(t, client, "volume", "delete", "krewe://acme/house-bills/logs")

	if err == nil {
		t.Fatal("a folder in the volume was deleted as though it were a file")
	}
	if !strings.Contains(err.Error(), "is a folder") {
		t.Errorf("the refusal does not say the address names a folder: %s", err)
	}
	if !strings.Contains(err.Error(), "one file") {
		t.Errorf("the refusal does not say what this deletes: %s", err)
	}
	if _, err := os.Stat(inside); err != nil {
		t.Fatalf("the refusal came back and the folder was emptied anyway: %v", err)
	}
}

// An address with no name on the end of it is the volume itself. Emptying one is out of scope, and
// this is the shape somebody types by leaving the file name off.
func TestVolumeDeleteOfTheVolumeItselfIsRefused(t *testing.T) {
	client, folder := aVolumeToCopyInto(t)
	source, _ := aFileOf(t, "explore.txt", 8)
	mustRun(t, client, "volume", "cp", source, "krewe://acme/house-bills")

	_, err := asked(t, client, "volume", "delete", "krewe://acme/house-bills")

	if err == nil {
		t.Fatal("a whole volume was deleted as though it were a file")
	}
	if !strings.Contains(err.Error(), "is a volume") {
		t.Errorf("the refusal does not say the address names the volume itself: %s", err)
	}
	if !strings.Contains(err.Error(), "one file") {
		t.Errorf("the refusal does not say what this deletes: %s", err)
	}
	if _, err := os.Stat(filepath.Join(folder, "explore.txt")); err != nil {
		t.Fatalf("the refusal came back and the volume was emptied anyway: %v", err)
	}
}

// One address, and the refusal carries the form. A second argument is the copy's shape typed at the
// wrong verb, and a delete that read the first of the two would take a file nobody named.
func TestVolumeDeleteSaysWhatTheVerbTakes(t *testing.T) {
	client, _ := aVolumeToCopyInto(t)

	for _, args := range [][]string{
		{"volume", "delete"},
		{"volume", "delete", "krewe://acme/house-bills/explore.txt", "krewe://acme/house-bills/kept.txt"},
		{"volume", "delete", "krewe://acme/house-bills/explore.txt", flagReplace},
	} {
		_, err := asked(t, client, args...)
		if err == nil {
			t.Errorf("krewe %s was accepted", strings.Join(args, " "))
			continue
		}
		if !strings.Contains(err.Error(), "krewe volume delete <address>") {
			t.Errorf("krewe %s does not say what the verb takes: %s", strings.Join(args, " "), err)
		}
	}
}

// Nothing above the transport knows a host path on this road either. The delete is driven with a
// transport that removes nothing, and the file in the volume is still there afterwards.
func TestVolumeDeleteGoesThroughTheTransportAndNotThroughAPath(t *testing.T) {
	client, folder := aVolumeToCopyInto(t)
	source, _ := aFileOf(t, "explore.txt", 512)
	mustRun(t, client, "volume", "cp", source, "krewe://acme/house-bills")
	carried := &recordingVolume{at: "/somewhere/else/explore.txt"}

	var said bytes.Buffer
	err := runVolumeDelete(context.Background(), carried, client,
		[]string{"krewe://acme/house-bills/explore.txt"}, &said)

	if err != nil {
		t.Fatalf("the delete was refused: %v", err)
	}
	if said.String() != carried.at+"\n" {
		t.Fatalf("the delete printed %q, and the transport answered %q", said.String(), carried.at)
	}
	if carried.removed != "krewe://acme/house-bills/explore.txt" {
		t.Errorf("the transport was asked for %q, want the address that was typed", carried.removed)
	}
	if _, err := os.Stat(filepath.Join(folder, "explore.txt")); err != nil {
		t.Error("the file was removed from the folder as well, so something above the transport knows a path")
	}
}

// hostVolume is where a key becomes a path on disk on the road that deletes, so it is where a key
// that climbs has to be held. The parser cleans what it reads, which is why this drives the transport
// itself: a key that reached it from anywhere else takes a file inside the volume or none at all.
func TestDeletingAKeyThatClimbsStaysInsideTheDirectory(t *testing.T) {
	client, folder := aVolumeToCopyInto(t)
	// One level up is where a single .. lands, so this is the file the guard has to leave alone.
	above := filepath.Dir(folder)
	if err := os.WriteFile(filepath.Join(above, "passwd"), []byte("the one above"), 0o666); err != nil {
		t.Fatalf("write the file above the volume: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folder, "passwd"), []byte("the one inside"), 0o666); err != nil {
		t.Fatalf("write the file inside the volume: %v", err)
	}
	address, err := workspace.ParseVolumePath("krewe://acme/house-bills")
	if err != nil {
		t.Fatalf("read the address: %v", err)
	}
	found, err := workspace.ResolveVolume(context.Background(), client, address)
	if err != nil {
		t.Fatalf("resolve the address: %v", err)
	}
	found.Address.Key = "../passwd"

	at, err := hostVolume{client: client}.Delete(context.Background(), found)

	if err != nil {
		t.Fatalf("the delete was refused: %v", err)
	}
	if _, err := os.Stat(filepath.Join(above, "passwd")); err != nil {
		t.Fatalf("the transport deleted the file above the volume: %v", err)
	}
	if _, err := os.Stat(filepath.Join(folder, "passwd")); !os.IsNotExist(err) {
		t.Fatalf("the key was not held inside the volume, so the file inside it is still there: %v", err)
	}
	if !strings.HasSuffix(at, "/passwd") || strings.Contains(at, "..") {
		t.Fatalf("the delete answered %q, want the held path the file was at", at)
	}
}
