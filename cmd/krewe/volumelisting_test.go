package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// krewe volume list: the listing comes from the control plane.
//
// The failure it answers: the listing asked where the volume was and then opened that path itself.
// That works while the tool and the sandboxes share one machine. Under Kubernetes they do not, the
// volume sits beside the control plane, and the path the system hands back names nothing here. So the
// listing answered that the directory held nothing called "", and a person read that as an empty
// volume.
//
// The volume in these tests is somewhere this process cannot open, which is what aVolumeSomewhereElse
// stands up. A listing that reached for a path instead of asking reads nothing at all.

// The step this change exists for: the volume is somewhere this process cannot open, and the listing
// still names what is in it.
func TestVolumeListReachesAVolumeThisMachineCannotOpen(t *testing.T) {
	client, dir, elsewhere := aVolumeSomewhereElse(t)
	source, _ := aFileOf(t, "Explore-logs.txt", 64)
	mustRun(t, client, "volume", "cp", source, "krewe://acme/house-bills")

	said := mustRun(t, client, "volume", "list", "krewe://acme/house-bills")

	if rows := rowsOf(said); len(rows) != 2 || rows[1] != "Explore-logs.txt 64" {
		t.Fatalf("the listing reads %v, want the file that was copied in:\n%s", rows, said)
	}
	// The directory it names is the one on the machine running the sandboxes, which is the path a
	// person walks to. It is not a path this process can open, and that is the point of it.
	if got := firstLineOf(said); !strings.HasPrefix(got, elsewhere) {
		t.Fatalf("the listing opens with %q, want a directory under %q", got, elsewhere)
	}
	if _, err := os.Stat(elsewhere); !os.IsNotExist(err) {
		t.Fatalf("%s was opened, so the listing reached for a host path: %v", elsewhere, err)
	}
	// And the bytes are where the control plane put them, rather than anywhere this process wrote.
	if _, err := os.Stat(filepath.Join(theProjectFolder(dir, theWorkspaceID(t, dir)), "Explore-logs.txt")); err != nil {
		t.Fatalf("the file is not in the directory a session reads: %v", err)
	}
}

// A folder with nothing in it says so, wherever the folder is. An empty table reads as a command that
// broke, and a listing that could not open the directory used to read exactly like this one.
func TestVolumeListOnAnEmptyVolumeThisMachineCannotOpenSaysSo(t *testing.T) {
	client, _, _ := aVolumeSomewhereElse(t)

	said := mustRun(t, client, "volume", "list", "krewe://acme/house-bills")

	if !strings.Contains(said, "nothing in it") {
		t.Fatalf("an empty folder does not say so:\n%s", said)
	}
	if strings.Contains(said, "NAME") {
		t.Fatalf("an empty folder printed the heading of a table with no rows:\n%s", said)
	}
}

// A key naming one file lists that file, which is the command somebody types to check a copy arrived.
func TestVolumeListOnOneFileInAVolumeThisMachineCannotOpen(t *testing.T) {
	client, _, _ := aVolumeSomewhereElse(t)
	source, _ := aFileOf(t, "explore.txt", 9)
	mustRun(t, client, "volume", "cp", source, "krewe://acme/house-bills")

	said := mustRun(t, client, "volume", "list", "krewe://acme/house-bills/explore.txt")

	if rows := rowsOf(said); len(rows) != 2 || rows[1] != "explore.txt 9" {
		t.Fatalf("listing one file reads %v:\n%s", rows, said)
	}
}

// The refusal for a name that is not there comes from the end that read the directory, and it still
// names both the directory and what was asked for.
func TestVolumeListOnANameThatIsNotThereInAVolumeThisMachineCannotOpen(t *testing.T) {
	client, _, elsewhere := aVolumeSomewhereElse(t)

	_, err := asked(t, client, "volume", "list", "krewe://acme/house-bills/nothing.txt")

	if err == nil {
		t.Fatal("a name that is not there was listed as though it were")
	}
	if !strings.Contains(err.Error(), elsewhere) {
		t.Errorf("the refusal does not name the directory it read: %s", err)
	}
	if !strings.Contains(err.Error(), "nothing.txt") {
		t.Errorf("the refusal does not name what was asked for: %s", err)
	}
}
