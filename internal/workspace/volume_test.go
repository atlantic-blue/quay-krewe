package workspace_test

import (
	"strings"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/workspace"
)

func TestParseVolumePathReadsTheLevelsAndTheKey(t *testing.T) {
	cases := map[string]workspace.VolumePath{
		// One level: the workspace's shared folder. Nothing is in doubt, because an address must have
		// a workspace and the first segment is one.
		"krewe://itv": {Workspace: "itv", Kind: workspace.VolumeShared},

		// Two: the vast folder, or the file called vast. The string does not say which, so the level
		// is filled and the reading is marked a guess from there down.
		"krewe://itv/vast": {
			Workspace: "itv", Project: "vast",
			Kind: workspace.VolumeProject, Ambiguous: workspace.LevelProject,
		},
		"krewe://itv/a.txt": {
			Workspace: "itv", Project: "a.txt",
			Kind: workspace.VolumeProject, Ambiguous: workspace.LevelProject,
		},

		// Three, where the third is an identifier this system made: a session, and a project above it.
		"krewe://itv/vast/9e8153f6": {
			Workspace: "itv", Project: "vast", Session: "9e8153f6", Kind: workspace.VolumeWorking,
		},
		"krewe://itv/vast/9e8153f6bd7c41a0f5e2d3b4": {
			Workspace: "itv", Project: "vast", Session: "9e8153f6bd7c41a0f5e2d3b4", Kind: workspace.VolumeWorking,
		},
		"krewe://itv/vast/9e8153f6/a.txt": {
			Workspace: "itv", Project: "vast", Session: "9e8153f6", Key: "a.txt", Kind: workspace.VolumeWorking,
		},

		// Three, where the third is any other word: a session a channel named, or a file. Filled and
		// marked, both of them.
		"krewe://itv/vast/logs/explore.txt": {
			Workspace: "itv", Project: "vast", Session: "logs", Key: "explore.txt",
			Kind: workspace.VolumeWorking, Ambiguous: workspace.LevelProject,
		},
		"krewe://itv/vast/log.txt": {
			Workspace: "itv", Project: "vast", Session: "log.txt",
			Kind: workspace.VolumeWorking, Ambiguous: workspace.LevelProject,
		},

		// There are three levels at most, so the fourth segment is the key whatever it is spelled like.
		"krewe://itv/vast/9e8153f6/logs/explore.txt": {
			Workspace: "itv", Project: "vast", Session: "9e8153f6", Key: "logs/explore.txt",
			Kind: workspace.VolumeWorking,
		},
		"krewe://itv/vast/9e8153f6/logs/monday": {
			Workspace: "itv", Project: "vast", Session: "9e8153f6", Key: "logs/monday",
			Kind: workspace.VolumeWorking,
		},

		// The scheme is optional here. Whatever takes an address and a local path in one command is
		// what requires it.
		"itv": {Workspace: "itv", Kind: workspace.VolumeShared},
		"itv/vast": {
			Workspace: "itv", Project: "vast",
			Kind: workspace.VolumeProject, Ambiguous: workspace.LevelProject,
		},
		"itv/vast/9e8153f6": {
			Workspace: "itv", Project: "vast", Session: "9e8153f6", Kind: workspace.VolumeWorking,
		},

		// A directory is spelled with a trailing slash by anybody who has used the storage tools.
		"krewe://itv/vast/": {
			Workspace: "itv", Project: "vast",
			Kind: workspace.VolumeProject, Ambiguous: workspace.LevelProject,
		},
		"krewe://itv/": {Workspace: "itv", Kind: workspace.VolumeShared},

		"  krewe://itv/vast  ": {
			Workspace: "itv", Project: "vast",
			Kind: workspace.VolumeProject, Ambiguous: workspace.LevelProject,
		},

		// A name from before names were slugs is still a workspace: the first segment is the workspace
		// whatever it is spelled like, because an address with no workspace is not an address.
		"krewe://house bills": {Workspace: "house bills", Kind: workspace.VolumeShared},
	}
	for input, want := range cases {
		got, err := workspace.ParseVolumePath(input)
		if err != nil {
			t.Errorf("ParseVolumePath(%q): %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("ParseVolumePath(%q) = %#v, want %#v", input, got, want)
		}
	}
}

// The shape of a session identifier is the one thing in the string that settles a reading, so what
// counts as one is worth holding down on its own.
func TestParseVolumePathSettlesOnASessionIdentifier(t *testing.T) {
	settled := []string{
		"9e8153f6",                 // what a listing prints
		"9e8153f6bd7c",             // longer than the listing, shorter than the whole
		"9e8153f6bd7c41a0f5e2d3b4", // twelve bytes, which is what the system makes
		"00000000",
	}
	for _, segment := range settled {
		address, err := workspace.ParseVolumePath("krewe://itv/vast/" + segment)
		if err != nil {
			t.Errorf("ParseVolumePath with %q: %v", segment, err)
			continue
		}
		if !address.Settled() {
			t.Errorf("a session of %q is %s, want the reading settled", segment, address.Ambiguous)
		}
		if address.Session != segment {
			t.Errorf("the session of %q is %q, want %q", segment, address.Session, segment)
		}
	}

	guesses := map[string]string{
		"notes":  "a word that is not hexadecimal",
		"9e8153": "shorter than a listing prints",
		"9e8153f6bd7c41a0f5e2d3b49e8153f6bd7c41a0": "forty characters, which is a commit and not a session",
		"logs": "the design's own example",
	}
	for segment, why := range guesses {
		address, err := workspace.ParseVolumePath("krewe://itv/vast/" + segment)
		if err != nil {
			t.Errorf("ParseVolumePath with %q: %v", segment, err)
			continue
		}
		if address.Settled() {
			t.Errorf("a session of %q is settled, want a guess: %s", segment, why)
		}
	}
}

// The two the string cannot settle, which are the two the resolver in step 3 has to answer.
//
// Both fill the levels as deep as they go, because the deepest reading is the one the resolver walks
// back from: it takes the longest prefix of the levels that is really there, and the rest is the key.
func TestParseVolumePathMarksWhatItCannotSettle(t *testing.T) {
	// A file whose name is a plain word reads as a session.
	file, err := workspace.ParseVolumePath("krewe://itv/vast/notes")
	if err != nil {
		t.Fatalf("ParseVolumePath: %v", err)
	}
	if file.Session != "notes" || file.Kind != workspace.VolumeWorking {
		t.Errorf("the deepest reading of a plain word is %#v, want it filled as a session", file)
	}
	if file.Ambiguous != workspace.LevelProject {
		t.Errorf("a plain word is marked %q, want %q: the vast above it may be a file name too",
			file.Ambiguous, workspace.LevelProject)
	}

	// A session handle a channel chose, which is not shaped like anything this system makes.
	handle, err := workspace.ParseVolumePath("krewe://itv/vast/C0FFEE.99/notes.txt")
	if err != nil {
		t.Fatalf("ParseVolumePath: %v", err)
	}
	if handle.Session != "C0FFEE.99" {
		t.Errorf("the session is %q, want the handle: a channel names a conversation its own way", handle.Session)
	}
	if handle.Key != "notes.txt" {
		t.Errorf("the key is %q, want %q", handle.Key, "notes.txt")
	}
	if handle.Settled() {
		t.Error("a handle a channel chose is settled, want a guess: nothing in it says it is a session")
	}
}

// A key that climbs is the oldest way into a directory that is not yours. It is cleaned onto the root
// of nothing, so it lands inside the volume or it lands nowhere.
func TestParseVolumePathHoldsAKeyInsideTheVolume(t *testing.T) {
	cases := map[string]string{
		"krewe://itv/vast/../../etc/passwd":          "etc/passwd",
		"krewe://itv/vast/9e8153f6/../../etc/passwd": "etc/passwd",
		"krewe://itv/../etc/passwd":                  "etc/passwd",
		"krewe://itv/vast/9e8153f6/logs/../a.txt":    "a.txt",
		"krewe://itv/vast/9e8153f6/./log.txt":        "log.txt",
	}
	for input, want := range cases {
		got, err := workspace.ParseVolumePath(input)
		if err != nil {
			t.Errorf("ParseVolumePath(%q): %v", input, err)
			continue
		}
		if got.Key != want {
			t.Errorf("ParseVolumePath(%q).Key = %q, want %q", input, got.Key, want)
		}
		if strings.HasPrefix(got.Key, "/") || strings.HasPrefix(got.Key, "..") {
			t.Errorf("ParseVolumePath(%q).Key = %q, which reaches outside the volume", input, got.Key)
		}
		// The two path elements never fill a level either, which is what keeps them under the cleaning.
		for _, level := range []string{got.Project, got.Session} {
			if level == "." || level == ".." {
				t.Errorf("ParseVolumePath(%q) filled a level with %q, which climbs out of the volume", input, level)
			}
		}
	}
}

func TestParseVolumePathRefusesWhatIsNotAVolume(t *testing.T) {
	refused := map[string]struct{ input, says string }{
		"empty":                 {"", "an address is required"},
		"only spaces":           {"   ", "an address is required"},
		"the scheme alone":      {"krewe://", "an address is required"},
		"another scheme":        {"s3://itv/vast", "krewe://"},
		"an empty level":        {"krewe://itv//vast", "empty level"},
		"a leading slash":       {"krewe:///itv", "empty level"},
		"the system's own":      {"krewe://system", "tokens and the sealing key"},
		"however it is spelled": {"krewe://System/vast", "tokens and the sealing key"},
		"the retired word":      {"krewe://crew/vast", "not a word this takes any more"},
		"the working trees":     {"krewe://itv/worktrees", "working tree"},
		"a session under it":    {"krewe://itv/worktrees/9e8153f6", "working tree"},
		"the shared clones":     {"krewe://itv/repos", "clone of a repository"},
		"a repository under it": {"krewe://itv/repos/quay-krewe", "clone of a repository"},
	}
	for what, c := range refused {
		got, err := workspace.ParseVolumePath(c.input)
		if err == nil {
			t.Errorf("%s: ParseVolumePath(%q) = %#v, want a refusal", what, c.input, got)
			continue
		}
		if !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: ParseVolumePath(%q) said %q, want it to say %q", what, c.input, err, c.says)
		}
	}
}

// What is printed after a copy has to be typeable back, so the scheme goes on and the key stays where
// the parser put it.
func TestVolumePathRendersBackWithTheScheme(t *testing.T) {
	for _, input := range []string{
		"krewe://itv",
		"krewe://itv/vast",
		"krewe://itv/vast/9e8153f6",
		"krewe://itv/vast/9e8153f6/logs/explore.txt",
		"krewe://itv/a.txt",
	} {
		parsed, err := workspace.ParseVolumePath(input)
		if err != nil {
			t.Fatalf("ParseVolumePath(%q): %v", input, err)
		}
		if parsed.String() != input {
			t.Errorf("ParseVolumePath(%q).String() = %q, want %q", input, parsed.String(), input)
		}
		again, err := workspace.ParseVolumePath(parsed.String())
		if err != nil {
			t.Fatalf("ParseVolumePath(%q): %v", parsed.String(), err)
		}
		if again != parsed {
			t.Errorf("reading %q back gave %#v, want %#v", parsed.String(), again, parsed)
		}
	}
}

// The levels are an address like any other, so what already turns one into identifiers takes this one.
func TestVolumePathHandsBackTheAddressWithoutTheKey(t *testing.T) {
	parsed, err := workspace.ParseVolumePath("krewe://itv/vast/9e8153f6/logs/explore.txt")
	if err != nil {
		t.Fatalf("ParseVolumePath: %v", err)
	}
	want := workspace.Path{Workspace: "itv", Project: "vast", Session: "9e8153f6"}
	if parsed.Path() != want {
		t.Errorf("Path() = %#v, want %#v", parsed.Path(), want)
	}
	if !parsed.HasKey() {
		t.Error("HasKey() = false, want true: the address names a file")
	}
	shared, err := workspace.ParseVolumePath("krewe://itv")
	if err != nil {
		t.Fatalf("ParseVolumePath: %v", err)
	}
	if shared.HasKey() {
		t.Errorf("HasKey() = true for %q, want false", shared)
	}
}
