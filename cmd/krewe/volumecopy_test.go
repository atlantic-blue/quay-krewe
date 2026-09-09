package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/workspace"
)

// krewe volume cp: a file reaches a session.
//
// The failure it answers: a person holding a file had `krewe where`, and no write verb at all. That
// command names a directory and leaves them to copy into it by hand. The name of that directory
// carries up to three generated identifiers, so the copy could not be typed from what is on screen.
//
// Two things are proved here rather than anywhere else. The bytes arrive whole, at a size the
// existing read ceiling of one mebibyte refuses. And a name already there is refused, because a copy
// that writes over the last one silently is how the work in it is lost.

// theLogFile is the size of the file that started this feature, in bytes. It is over the one mebibyte
// ceiling `krewe read` held a file to. So a copy cannot be that call with a writer on the end of it.
const theLogFile = 1_105_815

// aVolumeToCopyInto stands a system up with a workspace and a project, and hands back the client and
// the project's folder on disk.
//
// The folder comes from the tool's own answer rather than from a path this file builds. A copy into a
// directory nobody mounts would pass against a path assembled correctly and read by nothing.
func aVolumeToCopyInto(t *testing.T) (quaycrewv1.ControlPlaneServiceClient, string) {
	t.Helper()
	client, _, _ := aSystemOnDisk(t)
	mustRun(t, client, "workspace", "create", "acme")
	mustRun(t, client, "project", "create", "house-bills")
	return client, theVolumeAt(t, client, "acme/house-bills")
}

// aFileOf writes a file of that many bytes on the machine and hands back its path and its bytes.
//
// The bytes are random, so a copy that wrote the right number of the wrong ones is caught. A file of
// one repeated byte is identical to the file that lost half of itself and got padded.
func aFileOf(t *testing.T, name string, size int) (string, []byte) {
	t.Helper()
	body := make([]byte, size)
	if _, err := rand.Read(body); err != nil {
		t.Fatalf("make %d bytes: %v", size, err)
	}
	at := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(at, body, 0o666); err != nil {
		t.Fatalf("write %s: %v", at, err)
	}
	return at, body
}

// The step this whole feature exists for, at the size that made somebody ask for it. Byte for byte,
// in the directory a session reads, and above the ceiling the read call holds a file to.
func TestVolumeCopyPutsTheWholeFileInTheDirectoryASessionReads(t *testing.T) {
	client, folder := aVolumeToCopyInto(t)
	source, sent := aFileOf(t, "Explore-logs.txt", theLogFile)

	said := mustRun(t, client, "volume", "cp", source, "krewe://acme/house-bills")

	arrived, err := os.ReadFile(filepath.Join(folder, "Explore-logs.txt"))
	if err != nil {
		t.Fatalf("the file is not in the directory a session reads: %v", err)
	}
	if len(arrived) != theLogFile {
		t.Fatalf("the file arrived at %d bytes, want %d", len(arrived), theLogFile)
	}
	if !bytes.Equal(arrived, sent) {
		t.Fatal("the file arrived at the right size and the bytes are not the ones that were sent")
	}
	if got := firstLineOf(said); got != sandbox.SharedPath+"/house-bills/Explore-logs.txt" {
		t.Fatalf("the copy printed %q, want the path a session reads the file at", got)
	}
}

// The path is the whole of what this prints, on its own line, because it goes straight into the
// message somebody sends the session. Anything sharing that line has to be edited out by hand.
func TestVolumeCopyPrintsThePathASessionReadsAndNothingElse(t *testing.T) {
	client, _ := aVolumeToCopyInto(t)
	source, _ := aFileOf(t, "explore.txt", 9)

	said := mustRun(t, client, "volume", "cp", source, "krewe://acme/house-bills")

	if said != sandbox.SharedPath+"/house-bills/explore.txt\n" {
		t.Fatalf("the copy printed %q, want one path and a newline", said)
	}
}

// A name after the folder is the name the file takes, which is how a file is renamed on the way in.
func TestVolumeCopyTakesTheNameTheAddressGivesIt(t *testing.T) {
	client, folder := aVolumeToCopyInto(t)
	source, sent := aFileOf(t, "explore.txt", 32)

	said := mustRun(t, client, "volume", "cp", source, "krewe://acme/house-bills/yesterday.txt")

	arrived, err := os.ReadFile(filepath.Join(folder, "yesterday.txt"))
	if err != nil {
		t.Fatalf("the file did not arrive under the name the address gave it: %v", err)
	}
	if !bytes.Equal(arrived, sent) {
		t.Fatal("the file arrived under the right name with the wrong bytes")
	}
	if got := firstLineOf(said); got != sandbox.SharedPath+"/house-bills/yesterday.txt" {
		t.Fatalf("the copy printed %q, want the name the address gave it", got)
	}
}

// A key that names a folder keeps the file's own name inside it, the way copying into a folder does
// everywhere else. Writing over the folder is not something a person can mean.
func TestVolumeCopyIntoAFolderKeepsTheFilesOwnName(t *testing.T) {
	client, folder := aVolumeToCopyInto(t)
	if err := os.MkdirAll(filepath.Join(folder, "logs"), 0o777); err != nil {
		t.Fatalf("make the folder: %v", err)
	}
	source, sent := aFileOf(t, "explore.txt", 16)

	said := mustRun(t, client, "volume", "cp", source, "krewe://acme/house-bills/logs")

	arrived, err := os.ReadFile(filepath.Join(folder, "logs", "explore.txt"))
	if err != nil {
		t.Fatalf("the file did not land inside the folder: %v", err)
	}
	if !bytes.Equal(arrived, sent) {
		t.Fatal("the file landed inside the folder with the wrong bytes")
	}
	if got := firstLineOf(said); got != sandbox.SharedPath+"/house-bills/logs/explore.txt" {
		t.Fatalf("the copy printed %q, want the path inside the folder", got)
	}
}

// The refusal that stops a copy destroying the last one. It has to leave the first file exactly as it
// was, or the refusal is a message printed after the damage.
func TestVolumeCopyOntoANameAlreadyThereIsRefusedAndChangesNothing(t *testing.T) {
	client, folder := aVolumeToCopyInto(t)
	first, held := aFileOf(t, "explore.txt", 64)
	mustRun(t, client, "volume", "cp", first, "krewe://acme/house-bills")
	second, _ := aFileOf(t, "explore.txt", 128)

	_, err := asked(t, client, "volume", "cp", second, "krewe://acme/house-bills")

	if err == nil {
		t.Fatal("a copy wrote over a name that was already there")
	}
	if !strings.Contains(err.Error(), flagReplace) {
		t.Errorf("the refusal does not say what to type to mean it: %s", err)
	}
	if !strings.Contains(err.Error(), "krewe://acme/house-bills/explore.txt") {
		t.Errorf("the refusal does not name the file that is already there: %s", err)
	}
	arrived, err := os.ReadFile(filepath.Join(folder, "explore.txt"))
	if err != nil {
		t.Fatalf("read the file that was there first: %v", err)
	}
	if !bytes.Equal(arrived, held) {
		t.Fatal("the refusal came back and the first file was written over anyway")
	}
}

// Saying it is meant writes over it. The whole file, so a shorter one on top of a longer one leaves
// none of the longer one behind.
func TestVolumeCopyWithReplaceWritesOverWhatIsThere(t *testing.T) {
	client, folder := aVolumeToCopyInto(t)
	first, _ := aFileOf(t, "explore.txt", 4096)
	mustRun(t, client, "volume", "cp", first, "krewe://acme/house-bills")
	second, sent := aFileOf(t, "explore.txt", 32)

	mustRun(t, client, "volume", "cp", second, "krewe://acme/house-bills", flagReplace)

	arrived, err := os.ReadFile(filepath.Join(folder, "explore.txt"))
	if err != nil {
		t.Fatalf("read the file: %v", err)
	}
	if !bytes.Equal(arrived, sent) {
		t.Fatalf("the file is %d bytes, want the %d that replaced it", len(arrived), len(sent))
	}
}

// The bytes go to a temporary name and are renamed onto the real one. A session reading the folder
// then sees the whole file or no file. What that must not leave behind is the temporary name.
func TestVolumeCopyLeavesNothingBesideTheFile(t *testing.T) {
	client, folder := aVolumeToCopyInto(t)
	source, _ := aFileOf(t, "explore.txt", 2048)

	mustRun(t, client, "volume", "cp", source, "krewe://acme/house-bills")

	entries, err := os.ReadDir(folder)
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

// A session in a container is not the user this tool runs as. A file it cannot open is a file that
// did not arrive. A temporary file is made readable by its owner alone, which is where this goes
// wrong if the mode is left as it was made.
func TestVolumeCopyLeavesTheFileReadableBySomebodyElse(t *testing.T) {
	client, folder := aVolumeToCopyInto(t)
	source, _ := aFileOf(t, "explore.txt", 16)

	mustRun(t, client, "volume", "cp", source, "krewe://acme/house-bills")

	info, err := os.Stat(filepath.Join(folder, "explore.txt"))
	if err != nil {
		t.Fatalf("stat the file: %v", err)
	}
	if info.Mode().Perm()&0o044 == 0 {
		t.Fatalf("the file is %v, so a session running as anybody else cannot read it", info.Mode().Perm())
	}
}

// The oldest way into a directory that is not yours, on the road that writes. The key is cleaned onto
// the root of nothing, so it lands inside the volume or it lands nowhere.
func TestVolumeCopyHoldsAKeyThatClimbsInsideTheVolume(t *testing.T) {
	client, folder := aVolumeToCopyInto(t)
	source, _ := aFileOf(t, "passwd", 8)
	above := filepath.Dir(filepath.Dir(folder))

	mustRun(t, client, "volume", "cp", source, "krewe://acme/house-bills/../../passwd")

	if _, err := os.Stat(filepath.Join(above, "passwd")); err == nil {
		t.Fatal("the copy wrote above the volume")
	}
	if _, err := os.Stat(filepath.Join(folder, "passwd")); err != nil {
		t.Fatalf("the key was not held inside the volume: %v", err)
	}
}

// A whole directory is out of scope, and taking the first file in one would be worse than refusing:
// nobody would learn the other files never went.
func TestVolumeCopyOfADirectoryIsRefused(t *testing.T) {
	client, _ := aVolumeToCopyInto(t)
	source := t.TempDir()

	_, err := asked(t, client, "volume", "cp", source, "krewe://acme/house-bills")

	if err == nil {
		t.Fatal("a directory was copied as though it were a file")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Errorf("the refusal does not say what is wrong with it: %s", err)
	}
}

// The scheme is what tells the two arguments apart, because acme/house-bills is a good relative path
// and a good address. One argument of each kind is a direction. Two of a kind is neither, and both
// pairs are refused with the form on the end of the refusal.
func TestVolumeCopySaysWhichArgumentCarriesTheScheme(t *testing.T) {
	client, _ := aVolumeToCopyInto(t)
	source, _ := aFileOf(t, "explore.txt", 8)

	for _, args := range [][]string{
		{"volume", "cp", source, "acme/house-bills"},
		{"volume", "cp", "krewe://acme/house-bills/explore.txt", "krewe://acme/house-bills/kept.txt"},
		{"volume", "cp", source},
		{"volume", "cp"},
		{"volume", "cp", source, "krewe://acme/house-bills", source},
	} {
		_, err := asked(t, client, args...)
		if err == nil {
			t.Errorf("krewe %s was accepted", strings.Join(args, " "))
			continue
		}
		if !strings.Contains(err.Error(), "krewe volume cp <file> <address>") {
			t.Errorf("krewe %s does not say what the verb takes: %s", strings.Join(args, " "), err)
		}
	}
}

// A file that is not on the machine is a typo, and the message has to name the path that was read.
func TestVolumeCopyOfAFileThatIsNotThereNamesIt(t *testing.T) {
	client, _ := aVolumeToCopyInto(t)

	_, err := asked(t, client, "volume", "cp", "/tmp/no-such-file-here.txt", "krewe://acme/house-bills")

	if err == nil {
		t.Fatal("a file that is not on the machine was copied")
	}
	if !strings.Contains(err.Error(), "/tmp/no-such-file-here.txt") {
		t.Errorf("the refusal does not name the file it read: %s", err)
	}
}

// A workspace that is not there is a refusal. A volume that made the directory for the occasion
// would answer a typed workspace name with a folder no session ever reads.
func TestVolumeCopyIntoAWorkspaceThatIsNotThereIsRefused(t *testing.T) {
	client, _ := aVolumeToCopyInto(t)
	source, _ := aFileOf(t, "explore.txt", 8)

	_, err := asked(t, client, "volume", "cp", source, "krewe://nowhere/house-bills")

	if err == nil {
		t.Fatal("a copy into a workspace that does not exist succeeded")
	}
}

// Nothing above the transport knows a host path, so a copy can be driven with the bytes going
// somewhere else entirely. That is what step 11 replaces, and this is the seam it replaces at.
func TestVolumeCopyGoesThroughTheTransportAndNotThroughAPath(t *testing.T) {
	client, folder := aVolumeToCopyInto(t)
	source, sent := aFileOf(t, "explore.txt", 512)
	carried := &recordingVolume{at: "/somewhere/else/explore.txt"}

	var said bytes.Buffer
	err := runVolumeCopy(context.Background(), carried, client,
		[]string{source, "krewe://acme/house-bills"}, &said)

	if err != nil {
		t.Fatalf("the copy was refused: %v", err)
	}
	if firstLineOf(said.String()) != carried.at {
		t.Fatalf("the copy printed %q, and the transport answered %q", said.String(), carried.at)
	}
	if carried.name != "explore.txt" {
		t.Errorf("the transport was handed the name %q, want the file's own name", carried.name)
	}
	if carried.address != "krewe://acme/house-bills" {
		t.Errorf("the transport was handed %q, want the address that was typed", carried.address)
	}
	if !bytes.Equal(carried.body, sent) {
		t.Error("the transport was handed bytes that are not the ones in the file")
	}
	if _, err := os.Stat(filepath.Join(folder, "explore.txt")); err == nil {
		t.Error("the file was written to the folder as well, so something above the transport knows a path")
	}
}

// recordingVolume is a transport that keeps what it was asked to carry and writes nothing.
//
// gives is what it hands back on the way out, so a copy out can be driven with bytes that are in no
// volume at all. That is what proves nothing above the transport reads a path of its own.
type recordingVolume struct {
	at      string
	address string
	name    string
	body    []byte
	replace bool
	gives   []byte
	asked   string
	removed string
}

func (r *recordingVolume) Put(_ context.Context, to workspace.VolumeLocation, name string, body io.Reader, replace bool) (string, error) {
	read, err := io.ReadAll(body)
	if err != nil {
		return "", err
	}
	r.address, r.name, r.body, r.replace = to.Address.String(), name, read, replace
	return r.at, nil
}

func (r *recordingVolume) Get(_ context.Context, from workspace.VolumeLocation) (io.ReadCloser, error) {
	r.asked = from.Address.String()
	return io.NopCloser(bytes.NewReader(r.gives)), nil
}

func (r *recordingVolume) Delete(_ context.Context, at workspace.VolumeLocation) (string, error) {
	r.removed = at.Address.String()
	return r.at, nil
}

// The control plane is where a key becomes a path on disk on the road that writes. So it is where a
// key that climbs has to be held. The parser cleans what it reads, and this is the guard under it: a
// key that reached the transport from anywhere else lands inside the volume or it lands nowhere.
//
// The path it answers with is held too. A file written inside the volume and reported at the path the
// key asked for names a file that is not there.
func TestPuttingAKeyThatClimbsStaysInsideTheDirectory(t *testing.T) {
	client, folder := aVolumeToCopyInto(t)
	above := filepath.Dir(filepath.Dir(folder))
	address, err := workspace.ParseVolumePath("krewe://acme/house-bills")
	if err != nil {
		t.Fatalf("read the address: %v", err)
	}
	found, err := workspace.ResolveVolume(context.Background(), client, address)
	if err != nil {
		t.Fatalf("resolve the address: %v", err)
	}
	found.Address.Key = "../passwd"

	at, err := planeVolume{hostVolume{client: client}}.Put(context.Background(), found,
		"passwd", strings.NewReader("a secret"), false)

	if err != nil {
		t.Fatalf("the copy was refused: %v", err)
	}
	if _, err := os.Stat(filepath.Join(above, "passwd")); err == nil {
		t.Fatal("the transport wrote above the volume")
	}
	if _, err := os.Stat(filepath.Join(folder, "passwd")); err != nil {
		t.Fatalf("the key was not held inside the volume: %v", err)
	}
	if at != sandbox.SharedPath+"/house-bills/passwd" {
		t.Fatalf("the transport answered %q, which is not where it put the file", at)
	}
}
