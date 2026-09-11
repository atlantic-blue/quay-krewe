package commands_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/commands"
	"github.com/atlantic-blue/quay-krewe/internal/manual"
)

// Every test here runs over the whole embedded set rather than over a count of it. The set holds one
// file today and grows to four, one command per slice, so a test that asserts four fails today and a
// test that asserts one fails the moment the next command ships. What holds at every size is that
// every file in the set obeys the rules.

// The one place a count is the point: an embed that matched nothing would make every rule below hold
// over no files at all, and report a clean run.
func TestTheEmbeddedSetIsNotEmpty(t *testing.T) {
	if len(commands.All()) == 0 {
		t.Fatal("this build carries no commands, so every rule over the set proves nothing")
	}
}

// The marker is what makes an install safe, so a file shipped without one would be a file krewe
// installs and then refuses to replace on the next install.
func TestEveryCommandCarriesTheMarkerOnLineOne(t *testing.T) {
	for _, one := range commands.All() {
		line, _, _ := strings.Cut(one.Body, "\n")
		if _, marked := commands.BuildOf(line); !marked {
			t.Errorf("%s carries no marker on line one, it begins %q", one.FileName(), line)
		}
	}
}

// The second line begins the front matter, because the marker is a comment and the agent reads the
// front matter. A blank line between them would make the file front matter the agent never reads.
func TestTheSecondLineBeginsTheFrontMatter(t *testing.T) {
	for _, one := range commands.All() {
		lines := strings.Split(one.Body, "\n")
		if len(lines) < 2 || strings.TrimSpace(lines[1]) != "---" {
			t.Errorf("%s does not begin its front matter on line two", one.FileName())
		}
	}
}

// The description is what the listing prints, so it comes out of the file and cannot disagree with
// it. One line, because the listing is one line per command.
func TestEveryCommandCarriesADescriptionOfOneLine(t *testing.T) {
	for _, one := range commands.All() {
		switch {
		case one.Description == "":
			t.Errorf("%s carries no description, so the listing would name it and say nothing", one.FileName())
		case strings.Contains(one.Description, "\n"):
			t.Errorf("%s describes itself over more than one line: %q", one.FileName(), one.Description)
		}
	}
}

// The order the listing prints is declared, because a directory read gives back name order and the
// commands are met in the order a project is started in. This is both directions: a command with no
// place would print after the ones that have one, and a place with no command would be a name in a
// list that names nothing.
func TestEveryCommandHasAPlaceInTheListing(t *testing.T) {
	placed := make(map[string]bool, len(commands.Order()))
	for _, name := range commands.Order() {
		placed[name] = true
	}
	carried := make(map[string]bool, len(commands.All()))
	for _, one := range commands.All() {
		carried[one.Name] = true
		if !placed[one.Name] {
			t.Errorf("%s has no place in the listing's order, so it prints after the ones that do", one.FileName())
		}
	}
	for _, name := range commands.Order() {
		if !carried[name] {
			t.Errorf("the listing's order names %q, and this build carries no such command", name)
		}
	}
}

// kreweVerb finds krewe and the one or two words after it, which is how a command file names what it
// runs. Two words, because the manual's commands are one word or two.
var kreweVerb = regexp.MustCompile(`krewe ([a-z][a-z-]*)(?: ([a-z][a-z-]*))?`)

// A command file that names a verb the tool does not have sends the operator's own session to run
// something that refuses. The manual is the tool's own command list, so this reads the list rather
// than a copy of it.
func TestEveryVerbACommandNamesIsInTheManual(t *testing.T) {
	carried := manualPhrases(t)

	for _, one := range commands.All() {
		found := kreweVerb.FindAllStringSubmatch(one.Body, -1)
		if len(found) == 0 {
			t.Errorf("%s names no krewe verb at all, so it asks its questions and runs nothing", one.FileName())
		}
		for _, match := range found {
			pair := strings.TrimSpace(match[1] + " " + match[2])
			if carried[pair] || carried[match[1]] {
				continue
			}
			t.Errorf("%s names krewe %s, and the manual carries no such command", one.FileName(), pair)
		}
	}
}

// The design work belongs to a session in a sandbox, where the record keeps it. A command file that
// wrote a design body or a path would be the operator's own session doing the work the record is
// supposed to hold.
func TestNoCommandWritesADesignBodyOrAPath(t *testing.T) {
	for _, one := range commands.All() {
		for _, writing := range []string{"krewe design set", "krewe design edit", "krewe path set"} {
			if strings.Contains(one.Body, writing) {
				t.Errorf("%s runs %q, and a command never writes the design or the path", one.FileName(), writing)
			}
		}
	}
}

// manualPhrases is every command the tool has, read out of the usage the tool itself prints. A
// command line is indented two spaces; everything under it is indented far further.
func manualPhrases(t *testing.T) map[string]bool {
	t.Helper()
	word := regexp.MustCompile(`^[a-z][a-z-]*$`)

	phrases := make(map[string]bool)
	for _, line := range strings.Split(manual.Commands, "\n") {
		if !strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "   ") {
			continue
		}
		var built []string
		for _, token := range strings.Fields(line) {
			if !word.MatchString(token) {
				break
			}
			built = append(built, token)
			phrases[strings.Join(built, " ")] = true
		}
	}
	if len(phrases) == 0 {
		t.Fatal("no commands were read out of the manual, so every verb would pass")
	}
	return phrases
}

// The install is the whole point of the marker: what krewe wrote, krewe replaces.
func TestAnInstallWritesEveryFileAndStampsTheBuild(t *testing.T) {
	dir := t.TempDir()

	written, err := commands.Install(dir, "abc1234")
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if len(written) != len(commands.All()) {
		t.Fatalf("the install wrote %d of %d files", len(written), len(commands.All()))
	}
	for _, one := range written {
		body, err := os.ReadFile(one.Path)
		if err != nil {
			t.Fatalf("read what was written: %v", err)
		}
		line, rest, _ := strings.Cut(string(body), "\n")
		build, marked := commands.BuildOf(line)
		if !marked || build != "abc1234" {
			t.Errorf("%s begins %q, want the marker naming the build that wrote it", one.Path, line)
		}
		if !strings.HasPrefix(rest, "---") {
			t.Errorf("%s lost its front matter: %q", one.Path, rest)
		}
	}
}

// A second install is the ordinary case, after an upgrade. It replaces what krewe wrote whatever
// build the marker names, so the files are never left behind the binary.
func TestASecondInstallReplacesWhatKreweWroteAndRefusesNothing(t *testing.T) {
	dir := t.TempDir()

	if _, err := commands.Install(dir, "older11"); err != nil {
		t.Fatalf("the first install: %v", err)
	}
	written, err := commands.Install(dir, "newer22")
	if err != nil {
		t.Fatalf("the second install: %v", err)
	}
	if len(written) != len(commands.All()) {
		t.Fatalf("the second install wrote %d of %d files", len(written), len(commands.All()))
	}
	builds, err := commands.Builds(dir)
	if err != nil {
		t.Fatalf("read the builds: %v", err)
	}
	if len(builds) != 1 || builds[0] != "newer22" {
		t.Errorf("the directory holds %v, want the build the second install wrote", builds)
	}
}

// The one this slice exists for. A file krewe did not write is never written over, and a refusal
// that had already written half the set is not a refusal.
func TestAnInstallOverAFileWithNoMarkerWritesNothingAtAll(t *testing.T) {
	dir := t.TempDir()
	all := commands.All()
	byHand := filepath.Join(dir, all[len(all)-1].FileName())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(byHand, []byte("what the operator wrote themselves\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	written, err := commands.Install(dir, "abc1234")
	if err == nil {
		t.Fatalf("the install went over a file krewe did not write, and wrote %d files", len(written))
	}
	if !strings.Contains(err.Error(), byHand) {
		t.Errorf("the refusal does not name the file: %v", err)
	}
	if !strings.Contains(err.Error(), "krewe commands install") {
		t.Errorf("the refusal does not say to run the install again: %v", err)
	}

	// Nothing at all, which is what the check running before the first write is for. With one file in
	// the set this is that file; with four it is the other three as well.
	held, err := os.ReadFile(byHand)
	if err != nil {
		t.Fatal(err)
	}
	if string(held) != "what the operator wrote themselves\n" {
		t.Errorf("the file the operator wrote says %q now", held)
	}
	for _, one := range all {
		at := filepath.Join(dir, one.FileName())
		if at == byHand {
			continue
		}
		if _, err := os.Stat(at); !os.IsNotExist(err) {
			t.Errorf("%s was written even though the install was refused", at)
		}
	}
}

// A refusal decided on line one and not on the body. The body here carries a marker further down, so
// a check that read the whole file would replace this one.
func TestTheInstallReadsLineOneAndNeverTheBody(t *testing.T) {
	dir := t.TempDir()
	byHand := filepath.Join(dir, commands.All()[0].FileName())
	if err := os.WriteFile(byHand,
		[]byte("my own notes\n"+commands.Marker("abc1234")+"\nand more of them\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := commands.Install(dir, "abc1234"); err == nil {
		t.Fatal("a file whose marker is in its body was written over")
	}
}

// A file krewe never named is not krewe's business, marker or not. The directory is the agent's, and
// the operator may keep their own commands beside these.
func TestAFileTheSetDoesNotNameSurvivesAnInstall(t *testing.T) {
	dir := t.TempDir()
	theirs := filepath.Join(dir, "mine.md")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(theirs, []byte("a command of my own\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := commands.Install(dir, "abc1234"); err != nil {
		t.Fatalf("a file krewe never named refused the install: %v", err)
	}
	held, err := os.ReadFile(theirs)
	if err != nil {
		t.Fatalf("the file the operator wrote is gone: %v", err)
	}
	if string(held) != "a command of my own\n" {
		t.Errorf("the file the operator wrote says %q now", held)
	}
}

// A file with no marker is listed as unknown, which is not the same as a file that is not there.
func TestAFileWithNoMarkerIsReadAsUnknown(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, commands.All()[0].FileName()),
		[]byte("what the operator wrote themselves\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	builds, err := commands.Builds(dir)
	if err != nil {
		t.Fatalf("read the builds: %v", err)
	}
	if len(builds) != 1 || builds[0] != commands.UnknownBuild {
		t.Errorf("the directory reads as %v, want %q", builds, commands.UnknownBuild)
	}
}

// Where the files go. The variable is for an operator whose agent keeps its configuration elsewhere,
// and for every test that must not write into the operator's own directory.
func TestTheDirectoryIsWhatTheOperatorNamedOrTheAgentsOwn(t *testing.T) {
	named, err := commands.Directory(func(string) string { return "/somewhere/else" })
	if err != nil {
		t.Fatal(err)
	}
	if named != "/somewhere/else" {
		t.Errorf("the directory is %q, want what the operator named", named)
	}

	t.Setenv("HOME", "/home/somebody")
	fallback, err := commands.Directory(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("/home/somebody", ".claude", "commands", "krewe"); fallback != want {
		t.Errorf("the directory is %q, want %q", fallback, want)
	}
}

// The file name is the command name, and the directory is the namespace.
func TestTheFileNameIsTheCommandName(t *testing.T) {
	for _, one := range commands.All() {
		if one.Slash() != "/krewe:"+one.Name {
			t.Errorf("%s is typed as %q", one.FileName(), one.Slash())
		}
	}
}
