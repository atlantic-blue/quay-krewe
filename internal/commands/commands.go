// Package commands carries the slash commands krewe puts in the operator's own terminal, and writes
// them where the agent reads them.
//
// The files next to this one are the whole mechanism. Nothing here touches the store, there is no
// table and no call: a slash command is a markdown file the operator's agent reads, and the binary
// carries the files the way features/catalog.go carries the feature files and
// internal/store/migrate.go carries the migrations.
//
// Line one of every file is a marker, and it is what makes an install safe. A file carrying the
// marker was written by krewe, so krewe writes over it. A file carrying no marker was written by
// somebody else, so krewe refuses the whole install and names it. There is no flag that goes over
// that refusal: what an operator wrote by hand stays until they remove it themselves.
package commands

import (
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed *.md
var files embed.FS

// Namespace is the directory the files are installed into, and therefore the word before the colon:
// a file named init.md installed under it is /krewe:init.
const Namespace = "krewe"

// DirectoryEnv names the directory the files are installed into, for an operator whose agent keeps
// its configuration somewhere else and for a test that must not write into the operator's own.
const DirectoryEnv = "KREWE_COMMANDS_DIR"

// agentConfigDir is the agent's own configuration directory in the operator's home, which is where it
// reads its commands from. commandsDir under it is the agent's, and Namespace under that is krewe's
// alone, so an install never sees a file belonging to anything else.
const (
	agentConfigDir = ".claude"
	commandsDir    = "commands"
)

// The marker brackets line one of every file: a markdown comment, so whatever reads the command reads
// nothing, and the build between them, so krewe commands can say whether the files are this build's.
const (
	markerPrefix = "<!-- written by krewe "
	markerSuffix = " -->"
	// markerCeiling is how much of a file the install is willing to read to find line one. It reads
	// line one and never the body: what is in the body is the operator's business, and a file they
	// are protected from losing is not a file to go reading.
	markerCeiling = 512
)

// UnknownBuild is what a file carrying no marker is listed as. It is not the same as a file that is
// not there, which is why it is a word rather than an empty column.
const UnknownBuild = "unknown"

// order is the order the listing prints, which is the order the commands are met in rather than the
// order a directory read gives back. Every slice that adds a command adds its name here, and
// TestEveryCommandHasAPlaceInTheListing refuses a file this list does not name.
var order = []string{"init", "design", "status", "trust"}

// Order is that list, so a test can hold it against the set this build carries.
func Order() []string { return append([]string(nil), order...) }

// File is one slash command as this build carries it.
type File struct {
	// Name is the file name without .md, which is the command's own name.
	Name string
	// Description is the one line out of the front matter, and it is what the listing prints.
	Description string
	// Body is the whole file as it ships, marker line and all.
	Body string
}

// Slash is what the operator types to run it.
func (f File) Slash() string { return "/" + Namespace + ":" + f.Name }

// FileName is what the file is called where it is installed.
func (f File) FileName() string { return f.Name + ".md" }

// All is every command this build carries, in the order the listing prints them.
//
// A file the order does not name is printed after the ones it does, by name, so a command added
// without a place in the order still reaches the operator. The test is what refuses that state; the
// listing quietly losing a command would be worse than an ugly order.
func All() []File {
	entries, err := files.ReadDir(".")
	if err != nil {
		return nil
	}

	held := make(map[string]File, len(entries))
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		body, err := files.ReadFile(entry.Name())
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".md")
		held[name] = File{Name: name, Description: descriptionOf(string(body)), Body: string(body)}
		names = append(names, name)
	}

	all := make([]File, 0, len(names))
	placed := make(map[string]bool, len(names))
	for _, name := range order {
		if one, carried := held[name]; carried {
			all = append(all, one)
			placed[name] = true
		}
	}
	sort.Strings(names)
	for _, name := range names {
		if !placed[name] {
			all = append(all, held[name])
		}
	}
	return all
}

// Marker is line one of a file this build writes.
func Marker(build string) string { return markerPrefix + build + markerSuffix }

// BuildOf reads the build out of line one, and says whether line one is a marker at all.
func BuildOf(line string) (string, bool) {
	line = strings.TrimRight(line, "\r\n")
	if !strings.HasPrefix(line, markerPrefix) || !strings.HasSuffix(line, markerSuffix) {
		return "", false
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, markerPrefix), markerSuffix)), true
}

// descriptionOf reads the one line the front matter describes the command with.
//
// The front matter ships in the file rather than being generated here, so what the agent reads is
// what a reviewer read, and the listing cannot say one thing while the file says another.
func descriptionOf(body string) string {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if after, found := strings.CutPrefix(trimmed, "description:"); found {
			return strings.TrimSpace(after)
		}
	}
	return ""
}

// Directory is where the files are installed: what the operator named, or the agent's own commands
// directory under their home with krewe's namespace under it.
func Directory(getenv func(string) string) (string, error) {
	if named := strings.TrimSpace(getenv(DirectoryEnv)); named != "" {
		return named, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find your home directory: %w", err)
	}
	return filepath.Join(home, agentConfigDir, commandsDir, Namespace), nil
}

// Written is one file an install put on the machine.
type Written struct {
	Name string
	Path string
}

// Install writes every file this build carries into dir, stamping each one with the build.
//
// Every file it would write is read first, and one file it did not write refuses the whole install.
// A refusal that had already written half the set would leave the operator with a directory nobody
// can reason about, so the check finishes before the first write starts.
func Install(dir, build string) ([]Written, error) {
	all := All()
	if len(all) == 0 {
		return nil, fmt.Errorf("this build carries no commands to install")
	}

	refused := make([]string, 0, len(all))
	for _, one := range all {
		at := filepath.Join(dir, one.FileName())
		line, err := firstLine(at)
		switch {
		case os.IsNotExist(err):
			continue
		case err != nil:
			return nil, fmt.Errorf("read %s: %w", at, err)
		}
		if _, marked := BuildOf(line); !marked {
			refused = append(refused, at)
		}
	}
	if len(refused) > 0 {
		return nil, fmt.Errorf("%s was not written by krewe, so nothing was written at all: "+
			"remove it and run krewe commands install again", strings.Join(refused, ", "))
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("cannot write to %s: %w", dir, err)
	}
	written := make([]Written, 0, len(all))
	for _, one := range all {
		at := filepath.Join(dir, one.FileName())
		if err := os.WriteFile(at, []byte(stamped(one.Body, build)), 0o644); err != nil {
			return nil, fmt.Errorf("cannot write to %s: %w", at, err)
		}
		written = append(written, Written{Name: one.Name, Path: at})
	}
	return written, nil
}

// Builds is which build wrote each file of the set that dir holds now, sorted, without repeats. A
// file carrying no marker counts as UnknownBuild, and a file that is not there counts as nothing.
func Builds(dir string) ([]string, error) {
	seen := make(map[string]bool)
	for _, one := range All() {
		at := filepath.Join(dir, one.FileName())
		line, err := firstLine(at)
		switch {
		case os.IsNotExist(err):
			continue
		case err != nil:
			return nil, fmt.Errorf("read %s: %w", at, err)
		}
		build, marked := BuildOf(line)
		if !marked || build == "" {
			build = UnknownBuild
		}
		seen[build] = true
	}
	found := make([]string, 0, len(seen))
	for build := range seen {
		found = append(found, build)
	}
	sort.Strings(found)
	return found, nil
}

// stamped is the file as it goes onto the machine: line one replaced with the build that wrote it,
// and the rest of the file as it ships.
func stamped(body, build string) string {
	_, rest, found := strings.Cut(body, "\n")
	if !found {
		return Marker(build) + "\n"
	}
	return Marker(build) + "\n" + rest
}

// firstLine reads line one of a file and nothing else.
func firstLine(at string) (string, error) {
	file, err := os.Open(at) //nolint:gosec // the path is the install directory and a name this build carries
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	held := make([]byte, markerCeiling)
	read, err := io.ReadFull(file, held)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", err
	}
	line, _, _ := strings.Cut(string(held[:read]), "\n")
	return line, nil
}
