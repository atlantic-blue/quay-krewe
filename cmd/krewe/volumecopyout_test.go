package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/workspace"
)

// krewe volume cp, in the other direction: a file a session wrote comes back.
//
// The failure it answers: `krewe read` was the only way a file left a volume, and it held a file to
// one mebibyte. A session that wrote a log bigger than that had written it somewhere nobody could
// reach without opening a directory named in generated identifiers.
//
// The address on the left and a path on this machine on the right. The scheme is what says which way
// the bytes go, which is what the scheme was added for.

// aFileInTheVolume copies a file of that size into the project's folder and hands back the bytes that
// went in, and a folder on this machine to copy it back out to.
//
// It goes in through the tool rather than through a path this file writes, so what comes out is what
// the other direction put there.
func aFileInTheVolume(t *testing.T, client quaycrewv1.ControlPlaneServiceClient, name string, size int) ([]byte, string) {
	t.Helper()
	source, sent := aFileOf(t, name, size)
	mustRun(t, client, "volume", "cp", source, "krewe://acme/house-bills")
	return sent, t.TempDir()
}

// The scenario the step names, at the size of the file that started the feature. It is above the one
// mebibyte ceiling the read call holds a file to, so this cannot be that call with a reader on it.
func TestVolumeCopyOutBringsTheWholeFileBackUnderANewName(t *testing.T) {
	client, _ := aVolumeToCopyInto(t)
	sent, here := aFileInTheVolume(t, client, "Explore-logs.txt", theLogFile)
	at := filepath.Join(here, "yesterday.txt")

	said := mustRun(t, client, "volume", "cp", "krewe://acme/house-bills/Explore-logs.txt", at)

	arrived, err := os.ReadFile(at)
	if err != nil {
		t.Fatalf("the file did not come out of the volume: %v", err)
	}
	if len(arrived) != theLogFile {
		t.Fatalf("the file came out at %d bytes, want %d", len(arrived), theLogFile)
	}
	if !bytes.Equal(arrived, sent) {
		t.Fatal("the file came out at the right size and the bytes are not the ones that went in")
	}
	if said != at+"\n" {
		t.Fatalf("the copy printed %q, want the one path on this machine and a newline", said)
	}
}

// A destination that is a directory keeps the file's own name, the way copying into a folder does
// everywhere else. `.` is the destination somebody types most, and it is this case.
func TestVolumeCopyOutIntoADirectoryKeepsTheFilesOwnName(t *testing.T) {
	client, _ := aVolumeToCopyInto(t)
	sent, here := aFileInTheVolume(t, client, "explore.txt", 512)

	said := mustRun(t, client, "volume", "cp", "krewe://acme/house-bills/explore.txt", here)

	arrived, err := os.ReadFile(filepath.Join(here, "explore.txt"))
	if err != nil {
		t.Fatalf("the file did not land in the directory under its own name: %v", err)
	}
	if !bytes.Equal(arrived, sent) {
		t.Fatal("the file landed under the right name with the wrong bytes")
	}
	if said != filepath.Join(here, "explore.txt")+"\n" {
		t.Fatalf("the copy printed %q, want the path inside the directory", said)
	}
}

// The refusal that stops a copy destroying a file on this machine. It has to leave that file exactly
// as it was, or the refusal is a message printed after the damage.
func TestVolumeCopyOutOntoANameAlreadyThereIsRefusedAndChangesNothing(t *testing.T) {
	client, _ := aVolumeToCopyInto(t)
	_, here := aFileInTheVolume(t, client, "explore.txt", 64)
	at := filepath.Join(here, "explore.txt")
	held := []byte("what was already on this machine")
	if err := os.WriteFile(at, held, 0o666); err != nil {
		t.Fatalf("write the file that is there first: %v", err)
	}

	_, err := asked(t, client, "volume", "cp", "krewe://acme/house-bills/explore.txt", here)

	if err == nil {
		t.Fatal("a copy wrote over a file that was already on this machine")
	}
	if !strings.Contains(err.Error(), flagReplace) {
		t.Errorf("the refusal does not say what to type to mean it: %s", err)
	}
	if !strings.Contains(err.Error(), at) {
		t.Errorf("the refusal does not name the file that is already there: %s", err)
	}
	arrived, err := os.ReadFile(at)
	if err != nil {
		t.Fatalf("read the file that was there first: %v", err)
	}
	if !bytes.Equal(arrived, held) {
		t.Fatal("the refusal came back and the file on this machine was written over anyway")
	}
}

// Saying it is meant writes over it. The whole file, so a shorter one on top of a longer one leaves
// none of the longer one behind.
func TestVolumeCopyOutWithReplaceWritesOverWhatIsThere(t *testing.T) {
	client, _ := aVolumeToCopyInto(t)
	sent, here := aFileInTheVolume(t, client, "explore.txt", 32)
	at := filepath.Join(here, "explore.txt")
	if err := os.WriteFile(at, make([]byte, 4096), 0o666); err != nil {
		t.Fatalf("write the file that is there first: %v", err)
	}

	mustRun(t, client, "volume", "cp", "krewe://acme/house-bills/explore.txt", here, flagReplace)

	arrived, err := os.ReadFile(at)
	if err != nil {
		t.Fatalf("read the file: %v", err)
	}
	if !bytes.Equal(arrived, sent) {
		t.Fatalf("the file is %d bytes, want the %d that replaced it", len(arrived), len(sent))
	}
}

// The bytes go to a temporary name and are renamed onto the real one, so anything watching the
// directory sees the whole file or no file. What that must not leave behind is the temporary name.
func TestVolumeCopyOutLeavesNothingBesideTheFile(t *testing.T) {
	client, _ := aVolumeToCopyInto(t)
	_, here := aFileInTheVolume(t, client, "explore.txt", 2048)

	mustRun(t, client, "volume", "cp", "krewe://acme/house-bills/explore.txt", here)

	entries, err := os.ReadDir(here)
	if err != nil {
		t.Fatalf("read the folder: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "explore.txt" {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("the folder holds %v, want the one file", names)
	}
}

// A temporary file is made readable by its owner alone. A file that lands on this machine is read by
// whatever the operator runs next, so it takes the ordinary mode of a file instead.
func TestVolumeCopyOutLeavesTheFileReadable(t *testing.T) {
	client, _ := aVolumeToCopyInto(t)
	_, here := aFileInTheVolume(t, client, "explore.txt", 16)

	mustRun(t, client, "volume", "cp", "krewe://acme/house-bills/explore.txt", here)

	info, err := os.Stat(filepath.Join(here, "explore.txt"))
	if err != nil {
		t.Fatalf("stat the file: %v", err)
	}
	if info.Mode().Perm() != machineFileMode {
		t.Fatalf("the file is %v, want %v", info.Mode().Perm(), os.FileMode(machineFileMode))
	}
}

// A name that is not in the volume is a typo, and the message has to name the directory it read.
// Without it a name that is missing and a name in another folder read the same.
func TestVolumeCopyOutOfANameThatIsNotThereNamesTheDirectoryItRead(t *testing.T) {
	client, folder := aVolumeToCopyInto(t)

	_, err := asked(t, client, "volume", "cp", "krewe://acme/house-bills/nothing.txt", t.TempDir())

	if err == nil {
		t.Fatal("a file that is not in the volume was copied out")
	}
	if !strings.Contains(err.Error(), folder) {
		t.Errorf("the refusal does not name the directory it read: %s", err)
	}
	if !strings.Contains(err.Error(), "nothing.txt") {
		t.Errorf("the refusal does not name what it looked for: %s", err)
	}
}

// A whole directory is out of scope, both ways round. An address with no name on the end of it is the
// volume itself, and taking the first file in one would lose the rest without saying so.
func TestVolumeCopyOutOfAVolumeItselfIsRefused(t *testing.T) {
	client, _ := aVolumeToCopyInto(t)

	_, err := asked(t, client, "volume", "cp", "krewe://acme/house-bills", t.TempDir())

	if err == nil {
		t.Fatal("a whole volume was copied as though it were a file")
	}
	if !strings.Contains(err.Error(), "one file") {
		t.Errorf("the refusal does not say what this copies: %s", err)
	}
}

// The same refusal where the address does name something, and what it names is a folder in the
// volume. Opened as a file it would come back as an error from the operating system instead.
func TestVolumeCopyOutOfAFolderInTheVolumeIsRefused(t *testing.T) {
	client, folder := aVolumeToCopyInto(t)
	if err := os.MkdirAll(filepath.Join(folder, "logs"), 0o777); err != nil {
		t.Fatalf("make the folder: %v", err)
	}

	_, err := asked(t, client, "volume", "cp", "krewe://acme/house-bills/logs", t.TempDir())

	if err == nil {
		t.Fatal("a folder in the volume was copied as though it were a file")
	}
	if !strings.Contains(err.Error(), "one file") {
		t.Errorf("the refusal does not say what this copies: %s", err)
	}
}

// Nothing above the transport knows a host path, in this direction either. The bytes come from a
// transport that reads no volume at all, and they are the bytes that land on the machine.
func TestVolumeCopyOutGoesThroughTheTransportAndNotThroughAPath(t *testing.T) {
	client, _ := aVolumeToCopyInto(t)
	sent, here := aFileInTheVolume(t, client, "explore.txt", 512)
	carried := &recordingVolume{gives: []byte("bytes that are in no volume")}
	at := filepath.Join(here, "explore.txt")

	var said bytes.Buffer
	err := runVolumeCopy(context.Background(), carried, client,
		[]string{"krewe://acme/house-bills/explore.txt", at}, &said)

	if err != nil {
		t.Fatalf("the copy was refused: %v", err)
	}
	if said.String() != at+"\n" {
		t.Fatalf("the copy printed %q, want the path on this machine", said.String())
	}
	if carried.asked != "krewe://acme/house-bills/explore.txt" {
		t.Errorf("the transport was asked for %q, want the address that was typed", carried.asked)
	}
	arrived, err := os.ReadFile(at)
	if err != nil {
		t.Fatalf("the file did not land on this machine: %v", err)
	}
	if !bytes.Equal(arrived, carried.gives) {
		t.Error("the bytes on this machine are not the ones the transport handed over")
	}
	if bytes.Equal(arrived, sent) {
		t.Error("the bytes came from the volume, so something above the transport knows a path")
	}
}

// hostVolume is where a key becomes a path on disk on the road that reads, so it is where a key that
// climbs has to be held. The parser cleans what it reads, which is why this drives the transport
// itself: a key that reached it from anywhere else reads inside the volume or it reads nothing.
func TestGettingAKeyThatClimbsStaysInsideTheDirectory(t *testing.T) {
	client, folder := aVolumeToCopyInto(t)
	// One level up is where a single .. lands, so this is the file the guard has to not read.
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

	body, err := hostVolume{client: client}.Get(context.Background(), found)

	if err != nil {
		t.Fatalf("the read was refused: %v", err)
	}
	defer func() { _ = body.Close() }()
	read, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read what came back: %v", err)
	}
	if string(read) != "the one inside" {
		t.Fatalf("the transport read %q, so it climbed out of the volume", read)
	}
}
