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

// Every test here runs over the whole embedded set rather than over a count of it. The set grew one
// file per slice, and what holds at every size is that every file in the set obeys the rules.

// The two places a count is the point. An embed that matched nothing would make every rule below
// hold over no files at all, and report a clean run. And the set is the four commands SLASH-1 names,
// so a fifth file shipped without a contract behind it is caught here rather than by an operator
// reading a listing.
func TestTheEmbeddedSetIsNotEmpty(t *testing.T) {
	if len(commands.All()) == 0 {
		t.Fatal("this build carries no commands, so every rule over the set proves nothing")
	}
}

func TestTheEmbeddedSetHoldsTheFourCommandsOfTheContract(t *testing.T) {
	if held := len(commands.All()); held != 4 {
		t.Fatalf("this build carries %d commands, and the contract names four", held)
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

// The order the commands are met in: a project is started, then designed, then read back. A
// directory read would give back design, init, status, which is why the order is declared rather
// than read. The list here is written out rather than taken from commands.Order, so a wrong order
// declared in the package fails here instead of agreeing with itself.
func TestTheListingNamesTheCommandsInTheOrderTheyAreMetIn(t *testing.T) {
	at := func(want string) int {
		for i, one := range commands.All() {
			if one.Name == want {
				return i
			}
		}
		return -1
	}

	met := []string{"init", "design", "status", "trust"}
	for _, name := range met {
		if at(name) < 0 {
			t.Fatalf("this build carries no %s command", name)
		}
	}
	for i := 1; i < len(met); i++ {
		if at(met[i-1]) > at(met[i]) {
			t.Errorf("the listing names %s before %s, and they are met the other way round",
				met[i], met[i-1])
		}
	}
}

// kreweVerb finds krewe and the one or two words after it, which is how a command file talks in
// prose about what it runs. Two words, because the manual's commands are one word or two.
var kreweVerb = regexp.MustCompile(`krewe ([a-z][a-z-]*)(?: ([a-z][a-z-]*))?`)

// kreweCommand finds a command where a file writes one out to be typed: an indented code line, or an
// inline code span. Prose is read loosely, because "krewe design and show what it says" is a
// sentence rather than a command. What is written out to be typed is read strictly, and a phrase
// whose first word is a real command is exactly how a verb the tool does not have gets past a loose
// read.
var kreweCommand = regexp.MustCompile("(?m)^ {4,}krewe ([^\n]*)$|`krewe ([^`]*)`")

// A command file that names a verb the tool does not have sends the operator's own session to run
// something that refuses. The manual is the tool's own command list, so this reads the list rather
// than a copy of it.
func TestEveryVerbACommandNamesIsInTheManual(t *testing.T) {
	carried := manualPhrases(t)

	for _, one := range commands.All() {
		typed := kreweCommand.FindAllStringSubmatch(one.Body, -1)
		if len(typed) == 0 {
			t.Errorf("%s writes out no krewe command at all, so it asks its questions and runs nothing",
				one.FileName())
		}
		for _, match := range typed {
			phrase := leadingWords(match[1] + match[2])
			if carried[phrase] {
				continue
			}
			t.Errorf("%s writes out krewe %s to be typed, and the manual carries no such command",
				one.FileName(), phrase)
		}

		for _, match := range kreweVerb.FindAllStringSubmatch(one.Body, -1) {
			pair := strings.TrimSpace(match[1] + " " + match[2])
			if carried[pair] || carried[match[1]] {
				continue
			}
			t.Errorf("%s names krewe %s, and the manual carries no such command", one.FileName(), pair)
		}
	}
}

// leadingWords is the command out of the front of a line, which is its first word or its first two.
// It stops at the first thing that is not a word, because everything after that is an argument.
func leadingWords(line string) string {
	word := regexp.MustCompile(`^[a-z][a-z-]*$`)

	var built []string
	for _, token := range strings.Fields(line) {
		if len(built) == 2 || !word.MatchString(token) {
			break
		}
		built = append(built, token)
	}
	return strings.Join(built, " ")
}

// The design work belongs to a session in a sandbox, where the record keeps it. A command file that
// wrote a design body or a path would be the operator's own session doing the work the record is
// supposed to hold.
func TestNoCommandWritesADesignBodyOrAPath(t *testing.T) {
	for _, one := range commands.All() {
		for _, writing := range []string{
			"krewe design set", "krewe design edit", "krewe design contracts", "krewe path set",
		} {
			if strings.Contains(one.Body, writing) {
				t.Errorf("%s runs %q, and a command never writes the design or the path", one.FileName(), writing)
			}
		}
	}
}

// writingCommands is every word of the tool that only writes. A command file that names one of them
// runs it, so this is the list a readout may not touch.
//
// krewe path cap is not here, because the same word reads: with no number after it, the manual says
// it prints the cap and writes nothing. The form that writes is the one carrying a number, and
// capWithANumber below is what refuses that.
//
// The same list is read by features/commands_steps_test.go, over the file the install put on the
// machine. Each hook and each suite reads its own copy, the way the four design writes already do.
var writingCommands = []string{
	"krewe use",
	"krewe workspace create", "krewe workspace delete",
	"krewe project create", "krewe project delete", "krewe project repository",
	"krewe target",
	"krewe exec",
	"krewe archive", "krewe unarchive", "krewe label", "krewe mode", "krewe stop", "krewe drain",
	"krewe volume cp", "krewe volume delete",
	"krewe context set", "krewe context edit", "krewe context clear",
	"krewe design brief", "krewe design set", "krewe design edit", "krewe design contracts",
	"krewe design approve", "krewe design proof",
	"krewe feature add", "krewe feature intention", "krewe feature done", "krewe feature stop",
	"krewe feature open",
	"krewe path set",
	"krewe step take", "krewe step approve", "krewe step check", "krewe step done",
	"krewe step stop", "krewe step reopen",
	"krewe trust raise", "krewe trust threshold",
	"krewe secret set", "krewe secret mount",
	"krewe skill import", "krewe skill attach", "krewe skill detach",
	"krewe hook import", "krewe hook attach", "krewe hook detach",
	"krewe commands install",
}

// capWithANumber is krewe path cap carrying a number, which is the form that writes the cap. The
// number is a digit or the placeholder a command file writes in place of one.
var capWithANumber = regexp.MustCompile(`krewe path cap(?:\s+\S+)?\s+(?:\d+|<number>)`)

// asksForAYes is the word a command file uses where it waits for the operator to agree.
var asksForAYes = regexp.MustCompile(`(?i)\byes\b`)

// The status command is a readout. It reads the path, the sessions and the design, and it prints
// what they said. A command that wrote something would take an action the operator never asked for,
// out of a word they typed to look at the project.
func TestTheStatusCommandRunsNoCommandThatWrites(t *testing.T) {
	body := statusBody(t)

	for _, writing := range writingCommands {
		if strings.Contains(body, writing) {
			t.Errorf("status.md runs %q, and the status command writes nothing", writing)
		}
	}
	if found := capWithANumber.FindString(body); found != "" {
		t.Errorf("status.md runs %q, which writes the cap", strings.TrimSpace(found))
	}
}

// No yes, because there is nothing to agree to. A readout that stopped to ask would be one more
// thing to answer, where the whole point is to type one word and read the state.
func TestTheStatusCommandAsksForNoYes(t *testing.T) {
	if found := asksForAYes.FindString(statusBody(t)); found != "" {
		t.Errorf("status.md asks for a %q, and it approves nothing", found)
	}
}

// The three commands SLASH-6 names. The readout is built out of what they printed, so a file that
// stopped naming one of them would print a part of the state from nothing.
func TestTheStatusCommandReadsThePathTheSessionsAndTheDesign(t *testing.T) {
	body := statusBody(t)

	for _, reading := range []string{
		"krewe path <workspace>/<project>",
		"krewe sessions <workspace>/<project>",
		"krewe design <workspace>/<project>",
	} {
		if !strings.Contains(body, reading) {
			t.Errorf("status.md never runs %q", reading)
		}
	}
}

// A project with no path has no readout to print, so the one line it gets names the command that
// writes one. A readout of zeros would read as a project where nothing is happening.
func TestTheStatusCommandNamesTheCommandThatWritesAPath(t *testing.T) {
	if !strings.Contains(statusBody(t), "/krewe:design") {
		t.Error("status.md never names /krewe:design, so a project with no path is left looking it up")
	}
}

// The trust command reads the record, and the raise is the operator's answer to an offer krewe made.
// A file that named only one of the two would either print a ladder nobody can climb, or raise a
// level nobody read first.
func TestTheTrustCommandReadsTheRecordAndRaisesIt(t *testing.T) {
	body := trustBody(t)

	for _, named := range []string{
		"krewe trust <workspace>/<project>",
		"krewe trust raise <workspace>/<project>",
	} {
		if !strings.Contains(body, named) {
			t.Errorf("trust.md never runs %q", named)
		}
	}
}

// Raising the level moves the word done from the operator to krewe, which is the one place in this
// system where the operator hands something over. So the yes sits in the step that raises, and in no
// other step: a yes asked earlier, for something else, is not the operator handing that over.
//
// The part before the first step is left out of the count. It says what the command does, and what
// it says there is that a yes is asked where an offer stands.
func TestTheTrustCommandAsksForAYesOnlyInTheStepThatRaises(t *testing.T) {
	steps := strings.Split(trustBody(t), "\n## ")[1:]

	asked := make([]string, 0, 1)
	for _, step := range steps {
		if asksForAYes.MatchString(step) {
			asked = append(asked, step)
		}
	}
	if len(asked) != 1 {
		t.Fatalf("trust.md asks for a yes in %d of its %d steps, want the one that raises",
			len(asked), len(steps))
	}
	if !strings.Contains(asked[0], "krewe trust raise") {
		t.Errorf("trust.md asks for a yes in a step that raises nothing:\n%s", asked[0])
	}
}

// runLine is a command a file writes out to be typed, which is an indented line: that is how every
// file in the set says run this. A command named in prose or in an inline span is one the file tells
// the operator about, and the way back down is exactly that.
var runLine = regexp.MustCompile(`(?m)^ {4,}(krewe [^\n]*)$`)

// The raise is the only write this command makes. Everything else it does is a read, so a second
// writing command in the file would act on a word the operator typed to look at the record.
func TestTheTrustCommandRunsNoCommandThatWritesButTheRaise(t *testing.T) {
	for _, line := range runLine.FindAllStringSubmatch(trustBody(t), -1) {
		typed := strings.TrimSpace(line[1])
		for _, writing := range writingCommands {
			if writing == "krewe trust raise" || !strings.HasPrefix(typed, writing) {
				continue
			}
			t.Errorf("trust.md runs %q, and the raise is the only write this command makes", typed)
		}
	}
}

// The way down is a command the operator types on a step krewe closed wrongly, and it drops a level.
// The file names it, because a person who cannot find the way back stops handing the word over at
// all. It never runs it: nothing here lowers a level, and a command file that offered to would be
// undoing the operator's own yes.
func TestTheTrustCommandNamesTheWayBackDownAndNeverRunsIt(t *testing.T) {
	body := trustBody(t)

	if !strings.Contains(body, "krewe step reopen") {
		t.Error("trust.md never names krewe step reopen, so the way back down is left to be looked up")
	}
	if !strings.Contains(body, "never lowers a level") {
		t.Error("trust.md never says that it lowers no level")
	}
	for _, line := range runLine.FindAllStringSubmatch(body, -1) {
		if strings.HasPrefix(strings.TrimSpace(line[1]), "krewe step reopen") {
			t.Errorf("trust.md runs %q, and it lowers no level", strings.TrimSpace(line[1]))
		}
	}
}

// A project with no design has taken no step, so it agreed with nothing. The one line it gets names
// the command that writes a design, rather than a ladder of zeroes that reads as a project that
// tried and earned nothing.
func TestTheTrustCommandNamesTheCommandThatWritesADesign(t *testing.T) {
	if !strings.Contains(trustBody(t), "/krewe:design") {
		t.Error("trust.md never names /krewe:design, so a project with no design is left looking it up")
	}
}

// statusBody is the status command as this build carries it.
func statusBody(t *testing.T) string {
	t.Helper()
	return commandBody(t, "status")
}

// trustBody is the trust command as this build carries it.
func trustBody(t *testing.T) string {
	t.Helper()
	return commandBody(t, "trust")
}

// commandBody is one command of the set, read out of the binary rather than off a machine.
func commandBody(t *testing.T, name string) string {
	t.Helper()
	for _, one := range commands.All() {
		if one.Name == name {
			return one.Body
		}
	}
	t.Fatalf("this build carries no %s command", name)
	return ""
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
