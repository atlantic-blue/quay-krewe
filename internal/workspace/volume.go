package workspace

import (
	"fmt"
	"path"
	"strings"

	"github.com/atlantic-blue/quay-krewe/internal/display"
	"github.com/atlantic-blue/quay-krewe/internal/name"
)

// Scheme marks a string as a place inside this system rather than a path on the machine somebody is
// typing at. A copy takes one of each, and `itv/vast` reads as both, so this is what tells them apart.
const Scheme = "krewe://"

// longestSessionID is a session identifier at full length. The system makes one from twelve random
// bytes, so it is 24 hexadecimal characters, and a listing prints the first 8 of them. A segment
// longer than this is hexadecimal for some other reason, a commit for example, and it is a file name.
const longestSessionID = 24

// VolumeKind is which of the three directories an address names.
type VolumeKind string

const (
	// VolumeShared is a workspace's shared folder, which every session of that workspace reads.
	VolumeShared VolumeKind = "shared"
	// VolumeProject is a folder inside the shared one, named after a project.
	VolumeProject VolumeKind = "project"
	// VolumeWorking is one session's own directory, or the working tree it took.
	VolumeWorking VolumeKind = "working"
)

// VolumeLevel names one level of an address.
//
// There is no workspace here. An address must have one, so the first segment is a workspace and
// nothing else, and it is never in doubt.
type VolumeLevel string

const (
	LevelProject VolumeLevel = "project"
	LevelSession VolumeLevel = "session"
)

// VolumePath is a directory in this system, and the name of a file inside it.
//
//	krewe://itv                        the shared folder of itv
//	krewe://itv/vast                   the vast folder inside it
//	krewe://itv/vast/9e8153f6          that session's own directory, or its working tree
//	krewe://itv/vast/9e8153f6/a.txt    one file in it
//
// It says what somebody typed, not what it points at. Turning it into a directory needs the control
// plane, and that is the resolver's job rather than this one's.
type VolumePath struct {
	Workspace string
	Project   string
	Session   string
	// Key is the file inside that directory, slash separated, and always a name inside it. A key that
	// climbs is cleaned onto the root before it gets here.
	Key string
	// Kind is which of the three directories the levels name, in this reading of the address.
	Kind VolumeKind
	// Ambiguous is the shallowest level this reading filled that the string alone does not prove.
	// Empty means the whole reading is proven. See ParseVolumePath for what the resolver does with it.
	Ambiguous VolumeLevel
}

// ParseVolumePath reads an address and the name of a file out of one string.
//
// The levels come first and the key is the rest. The first segment is the workspace. The next two are
// a project and a session where they can be, and the fourth segment starts the key whatever it holds.
//
// One thing in the string is structural. A session identifier is hexadecimal and 8 to 24 characters,
// because the system makes it from twelve random bytes and a listing prints the first eight. So a
// third segment of that shape is a session, and the project above it is a project.
//
// Everything else is a guess, and the guess is marked rather than hidden. A project name and a file
// name are the same class of word: `krewe://itv/notes` is the notes project of itv, or the file called
// notes in its shared folder, and nothing in those 17 characters says which. A session handle a
// channel chose is the same again, because a channel names a conversation whatever way it likes. So
// this fills the levels as deep as they go and sets Ambiguous to the shallowest one it cannot prove.
//
// What step 3 inherits: where Ambiguous is set, resolve the levels against what the system holds and
// take the longest prefix that is really there. The rest is the key. `krewe://itv/vast/notes` is the
// session notes of the vast project where that session exists, the file notes in the vast folder where
// it does not, and the file vast/notes in the shared folder where there is no vast project either.
//
// The scheme is optional here. Whatever takes an address and a local path in one command is what
// requires it.
func ParseVolumePath(value string) (VolumePath, error) {
	trimmed := strings.TrimSpace(value)
	if rest, found := strings.CutPrefix(trimmed, Scheme); found {
		trimmed = strings.TrimSpace(rest)
	} else if at := strings.Index(trimmed, "://"); at >= 0 {
		return VolumePath{}, fmt.Errorf("workspace: %q names the scheme %q, and this reads %s, for example %sitv/vast",
			value, trimmed[:at+3], Scheme, Scheme)
	}
	// One trailing slash, which is how a person spells a directory. A second one is an empty level and
	// is refused below with every other empty level.
	trimmed = strings.TrimSuffix(trimmed, Separator)
	if trimmed == "" {
		return VolumePath{}, fmt.Errorf("workspace: an address is required, for example %sitv/vast", Scheme)
	}

	segments := strings.Split(trimmed, Separator)
	for at, segment := range segments {
		segments[at] = strings.TrimSpace(segment)
		if segments[at] == "" {
			return VolumePath{}, fmt.Errorf("workspace: %q has an empty level in it", value)
		}
	}

	parsed := VolumePath{Workspace: segments[0], Kind: VolumeShared}
	if err := refuseWorkspace(parsed.Workspace); err != nil {
		return VolumePath{}, err
	}
	rest := segments[1:]
	if len(rest) > 0 && canBeLevel(rest[0]) {
		parsed.Project, parsed.Kind, rest = rest[0], VolumeProject, rest[1:]
		parsed.Ambiguous = LevelProject
		if held := name.ReservedProject(parsed.Project); held != "" {
			return VolumePath{}, fmt.Errorf(
				"workspace: %q is where this system writes %s, so no project is called it: name a project, or reach a session at %sitv/vast/<session>",
				parsed.Project, held, Scheme)
		}
		if len(rest) > 0 && canBeLevel(rest[0]) {
			parsed.Session, parsed.Kind, rest = rest[0], VolumeWorking, rest[1:]
			// An identifier of this shape is one this system made, so the session is settled and the
			// level above a session is a project.
			if isSessionID(parsed.Session) {
				parsed.Ambiguous = ""
			}
		}
	}
	parsed.Key = cleanKey(strings.Join(rest, Separator))
	return parsed, nil
}

// refuseWorkspace answers the two words that are not a workspace, before anything reads the address
// as one. Both of them would otherwise come back saying no such workspace exists, which sends
// somebody looking for a workspace instead of telling them the one thing they need.
func refuseWorkspace(workspace string) error {
	if err := name.RefuseRetired(workspace); err != nil {
		return err
	}
	if strings.EqualFold(workspace, name.System) {
		return fmt.Errorf(
			"workspace: %q is not a volume: that directory holds the tokens and the sealing key, so name a workspace, for example %sitv",
			name.System, Scheme)
	}
	return nil
}

// canBeLevel says whether this segment could name a project or a session at all.
//
// Only the two path elements are refused, and they are refused because they name no thing: they move
// about the directory instead. Everything else could be a name, an identifier or a handle a channel
// chose, so it fills a level and the reading is marked as a guess.
//
// The refusal is what holds the traversal guard up. A key is cleaned onto the root, and a `..` sitting
// in a level would have gone round that cleaning.
func canBeLevel(segment string) bool { return segment != "." && segment != ".." }

// isSessionID says whether this segment is a session identifier rather than any other word.
//
// The lower bound is the listing's, because what is on the screen has to be typeable back, and it is
// read from the same place the command line reads it so the two cannot drift. The upper bound is
// here alone: a command line word is a session or the first word of a message, and this segment
// competes with a file name as well, so a 40 character commit is refused.
func isSessionID(segment string) bool {
	return len(segment) <= longestSessionID && display.LooksLikeIdentifier(segment)
}

// cleanKey holds a key inside the directory it was given.
//
// The leading slash is what makes a key that climbs harmless: it is resolved against the root of
// nothing, so `../../etc/passwd` becomes `/etc/passwd` and then a name inside the volume rather than
// a road out of it.
func cleanKey(key string) string {
	if key == "" {
		return ""
	}
	return strings.TrimPrefix(path.Clean("/"+key), Separator)
}

// HasKey reports whether the address names a file rather than the directory itself.
func (v VolumePath) HasKey() bool { return v.Key != "" }

// Settled reports whether the string proves this reading, so nothing has to be looked up to trust it.
func (v VolumePath) Settled() bool { return v.Ambiguous == "" }

// Path is the address without the key, which is what resolves into identifiers.
func (v VolumePath) Path() Path {
	return Path{Workspace: v.Workspace, Project: v.Project, Session: v.Session}
}

// String renders the address back with the scheme on it, so anything printing a place a file went
// prints something that can be typed back.
func (v VolumePath) String() string {
	parts := make([]string, 0, 4)
	for _, segment := range []string{v.Workspace, v.Project, v.Session} {
		if segment == "" {
			break
		}
		parts = append(parts, segment)
	}
	if v.Key != "" {
		parts = append(parts, v.Key)
	}
	return Scheme + strings.Join(parts, Separator)
}
