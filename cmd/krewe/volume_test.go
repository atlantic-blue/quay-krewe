package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
)

// krewe volume list: what a volume holds.
//
// The failure it answers: a volume is the directory a session reads, and nothing said what was in
// one. `krewe where` named the directory and left the person to open it by hand, and `krewe read`
// answered for a session and for nothing else. So a file copied into a workspace's shared folder
// could not be checked from the tool at all. Both words are gone, and each names this one instead.

// aVolumeHolding stands a system up, puts a workspace and a project in it, and writes what the test
// names into the project's folder. It hands back the client and that directory.
//
// The directory comes from the tool's own answer rather than from a path this file builds. A listing
// that read a directory nobody copies into would pass against a path assembled correctly and mounted
// nowhere.
func aVolumeHolding(t *testing.T, files map[string]string, folders ...string) (quaycrewv1.ControlPlaneServiceClient, string) {
	t.Helper()
	client, _, _ := aSystemOnDisk(t)
	mustRun(t, client, "workspace", "create", "acme")
	mustRun(t, client, "project", "create", "house-bills")
	folder := theVolumeAt(t, client, "acme/house-bills")
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(folder, name), []byte(body), 0o666); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	for _, name := range folders {
		if err := os.MkdirAll(filepath.Join(folder, name), 0o777); err != nil {
			t.Fatalf("make %s: %v", name, err)
		}
	}
	return client, folder
}

// theVolumeAt is the directory an address is kept in on this machine, which is the listing own first
// line. Tests take it from the tool answer rather than building the path themselves, because a path
// assembled correctly and mounted nowhere would pass against every assertion under it.
func theVolumeAt(t *testing.T, client quaycrewv1.ControlPlaneServiceClient, address string) string {
	t.Helper()
	return firstLineOf(mustRun(t, client, "volume", "list", address))
}

// The listing itself: the directory on the first line, then one row for each name, sorted, with a
// size on each file and none on a folder.
func TestVolumeListNamesTheDirectoryThenWhatIsInIt(t *testing.T) {
	client, folder := aVolumeHolding(t,
		map[string]string{"beta.txt": "bbbb", "alpha.txt": "aaaaa"}, "logs")

	said := mustRun(t, client, "volume", "list", "krewe://acme/house-bills")

	if got := firstLineOf(said); got != folder {
		t.Fatalf("the listing opens with %q, want the directory %q", got, folder)
	}
	lines := rowsOf(said)
	want := []string{"NAME SIZE", "alpha.txt 5", "beta.txt 4", "logs/"}
	if len(lines) != len(want) {
		t.Fatalf("the listing has %d rows, want %d:\n%s", len(lines), len(want), said)
	}
	for at, row := range want {
		if lines[at] != row {
			t.Errorf("row %d reads %q, want %q:\n%s", at, lines[at], row, said)
		}
	}
}

// An empty folder is the answer to a copy that has not happened yet, and it is a sentence rather than
// a table: a heading over nothing reads as a command that broke.
func TestVolumeListOnAnEmptyFolderSaysSo(t *testing.T) {
	client, _ := aVolumeHolding(t, nil)

	said := mustRun(t, client, "volume", "list", "krewe://acme/house-bills")

	if !strings.Contains(said, "nothing in it") {
		t.Fatalf("an empty folder does not say so:\n%s", said)
	}
	if strings.Contains(said, "NAME") {
		t.Fatalf("an empty folder printed the heading of a table with no rows:\n%s", said)
	}
}

// A key that names a file lists that file, which is the command somebody types to check a copy
// arrived under the name they gave it.
func TestVolumeListOnOneFileListsThatFile(t *testing.T) {
	client, _ := aVolumeHolding(t, map[string]string{"explore.txt": "a picture"})

	said := mustRun(t, client, "volume", "list", "krewe://acme/house-bills/explore.txt")

	if rows := rowsOf(said); len(rows) != 2 || rows[1] != "explore.txt 9" {
		t.Fatalf("listing one file reads %v:\n%s", rows, said)
	}
}

// A name that is missing and a name in another folder read the same, so the refusal carries the
// directory it read.
func TestVolumeListOnANameThatIsNotThereNamesTheDirectoryItRead(t *testing.T) {
	client, folder := aVolumeHolding(t, map[string]string{"explore.txt": "a picture"})

	_, err := asked(t, client, "volume", "list", "krewe://acme/house-bills/nothing.txt")

	if err == nil {
		t.Fatal("a name that is not there was listed as though it were")
	}
	if !strings.Contains(err.Error(), folder) {
		t.Errorf("the refusal does not name the directory it read: %s", err)
	}
	if !strings.Contains(err.Error(), "nothing.txt") {
		t.Errorf("the refusal does not name what was asked for: %s", err)
	}
}

// The oldest way into a directory that is not yours. The key is cleaned onto the root of nothing, so
// it lands inside the volume or it lands nowhere.
func TestVolumeListHoldsAKeyThatClimbsInsideTheVolume(t *testing.T) {
	client, folder := aVolumeHolding(t, nil)

	_, err := asked(t, client, "volume", "list", "krewe://acme/house-bills/../../etc/passwd")

	if err == nil {
		t.Fatal("a key that climbs out of the volume was listed")
	}
	if !strings.Contains(err.Error(), folder) {
		t.Errorf("the refusal read somewhere other than the volume: %s", err)
	}
	if !strings.Contains(err.Error(), `"etc/passwd"`) {
		t.Errorf("the key was not cleaned onto the volume: %s", err)
	}
}

// A project name and a file name are the same class of word, so the string alone cannot say which
// this is. The system is asked, and a name that is no project is a file in the shared folder.
func TestVolumeListReadsANameThatIsNoProjectAsAFileInTheSharedFolder(t *testing.T) {
	client, _, _ := aSystemOnDisk(t)
	mustRun(t, client, "workspace", "create", "acme")
	shared := theVolumeAt(t, client, "acme")
	if err := os.WriteFile(filepath.Join(shared, "screenshot.png"), []byte("a picture"), 0o666); err != nil {
		t.Fatalf("write the file: %v", err)
	}

	said := mustRun(t, client, "volume", "list", "krewe://acme/screenshot.png")

	if rows := rowsOf(said); len(rows) != 2 || rows[1] != "screenshot.png 9" {
		t.Fatalf("a name that is no project was not read as a file: %v\n%s", rows, said)
	}
}

// The other half of that reading. A session identifier is one this system made, so the string proves
// the level and a missing one is a missing session rather than a file with an identifier for a name.
func TestVolumeListOnASessionIdentifierThatIsNotThereSaysTheSessionIsMissing(t *testing.T) {
	client, _ := aVolumeHolding(t, nil)

	_, err := asked(t, client, "volume", "list", "krewe://acme/house-bills/deadbeefdeadbeef")

	if err == nil {
		t.Fatal("a session that does not exist was read as something else")
	}
	if !strings.Contains(err.Error(), "session") {
		t.Errorf("the refusal does not say the session is missing: %s", err)
	}
}

// The third level. A session's own directory is where it works when it took no working tree, and it
// is reached by the same string as the two above it.
func TestVolumeListReachesASessionsOwnDirectory(t *testing.T) {
	client, held, _ := aSystemOnDisk(t)
	mustRun(t, client, "workspace", "create", "acme")
	mustRun(t, client, "project", "create", "house-bills")
	handle := aSessionNothingHasRunIn(t, held, "house-bills")
	own := theVolumeAt(t, client, "acme/house-bills/"+handle)
	if err := os.WriteFile(filepath.Join(own, "answer.md"), []byte("done"), 0o666); err != nil {
		t.Fatalf("write the file: %v", err)
	}

	said := mustRun(t, client, "volume", "list", "krewe://acme/house-bills/"+handle)

	if got := firstLineOf(said); got != own {
		t.Fatalf("the listing opens with %q, want the session's own directory %q", got, own)
	}
	if rows := rowsOf(said); len(rows) != 2 || rows[1] != "answer.md 4" {
		t.Fatalf("the session's directory reads %v:\n%s", rows, said)
	}
}

// A session that was put away is out of the listing, and its volume goes with it.
//
// Archiving hides a session, and hiding it means hiding what it holds. The files stay on the machine,
// because archiving deletes nothing, and the address stops reaching them until the session comes back.
func TestVolumeListDoesNotReachASessionThatWasPutAway(t *testing.T) {
	client, held, _ := aSystemOnDisk(t)
	mustRun(t, client, "workspace", "create", "acme")
	mustRun(t, client, "project", "create", "house-bills")
	handle := aSessionNothingHasRunIn(t, held, "house-bills")
	own := theVolumeAt(t, client, "acme/house-bills/"+handle)
	if err := os.WriteFile(filepath.Join(own, "answer.md"), []byte("done"), 0o666); err != nil {
		t.Fatalf("write the file: %v", err)
	}
	mustRun(t, client, "archive", handle)

	_, err := asked(t, client, "volume", "list", "krewe://acme/house-bills/"+handle)

	if err == nil {
		t.Fatal("the volume of an archived session was listed, so archiving hides the row and not the files")
	}
	if !strings.Contains(err.Error(), "session") {
		t.Errorf("the refusal is %q, which does not say the session is the part that is missing", err)
	}
	// And nothing was deleted, which is the other half of what the word promises.
	if _, err := os.Stat(filepath.Join(own, "answer.md")); err != nil {
		t.Fatalf("archiving took what the session left: %v", err)
	}
}

// The scheme is what tells an address from a path on the machine, and a copy takes one of each. A
// listing takes one address and nothing else, so it reads either spelling.
func TestVolumeListTakesAnAddressWithoutTheScheme(t *testing.T) {
	client, _ := aVolumeHolding(t, map[string]string{"explore.txt": "a picture"})

	said := mustRun(t, client, "volume", "list", "acme/house-bills")

	if !strings.Contains(said, "explore.txt") {
		t.Fatalf("an address without the scheme was not read:\n%s", said)
	}
}

// A word under volume that this does not have, and the verb with no address at all. Both say the
// form the verb takes, because a refusal that does not is a refusal nobody can act on.
func TestVolumeSaysWhatItTakes(t *testing.T) {
	client, _ := aVolumeHolding(t, nil)

	for _, args := range [][]string{
		{"volume"},
		{"volume", "sizes", "krewe://acme"},
		{"volume", "list"},
		{"volume", "list", "krewe://acme", "krewe://acme"},
	} {
		_, err := asked(t, client, args...)
		if err == nil {
			t.Errorf("krewe %s was accepted", strings.Join(args, " "))
			continue
		}
		if !strings.Contains(err.Error(), "krewe volume list <address>") {
			t.Errorf("krewe %s does not say what the verb takes: %s", strings.Join(args, " "), err)
		}
	}
}

// rowsOf is the table under the directory, each row as its cells with one space between them, so an
// assertion is about the names and the sizes rather than about how wide a column happens to be.
func rowsOf(said string) []string {
	_, under, _ := strings.Cut(said, "\n")
	rows := make([]string, 0, 4)
	for _, line := range strings.Split(under, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		rows = append(rows, strings.Join(strings.Fields(line), " "))
	}
	return rows
}
