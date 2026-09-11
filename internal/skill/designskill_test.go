package skill

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The design skill is the one a session reads before anybody builds anything. It is prose and
// nothing else: no controller runs because it exists, and no gate reads it. So the brief is the
// whole of the capability, and what it fails to say is a thing the design will not carry.
//
// It declares the krewe binary because every instruction in it is a krewe command. A session in an
// image without the tool is refused with a sentence rather than told to type what it cannot run.
func TestTheShippedDesignSkillLoads(t *testing.T) {
	design := shippedSkill(t, "design")

	if design.Version != 1 {
		t.Errorf("the design skill is version %d, and a session is pinned to the one it started with", design.Version)
	}
	if !slices.Contains(design.Binaries, "krewe") {
		t.Errorf("the design skill declares the binaries %v, and every instruction in its brief is a krewe command", design.Binaries)
	}
	if len(design.Secrets) != 0 {
		t.Errorf("the design skill names the secrets %v, so a workspace that has not set them is a workspace where the skill is left out of every session", design.Secrets)
	}
	if design.HasSetup {
		t.Error("the design skill runs a setup script, and prose has nothing to set up")
	}
	// The summary is the line every session holding the skill reads on every conversation, so it says
	// when to reach for the brief rather than what the skill is called.
	summary := strings.ToLower(design.Summary)
	for _, said := range []string{"design", "before"} {
		if !strings.Contains(summary, said) {
			t.Errorf("the summary %q never says %q, so nothing tells a session about to design something to open the brief", design.Summary, said)
		}
	}
}

// The six parts of a restatement, which is what a session writes before it builds the step it took.
// A session that reads five of them writes five, and the operator reads a restatement that is
// missing the part they would have argued with.
func TestTheDesignBriefStatesTheSixPartsOfARestatement(t *testing.T) {
	brief := flowed(shippedSkill(t, "design").Brief)

	for part, said := range map[string]string{
		"what the step changes":        "changes, in your own words",
		"what it will not touch":       "will not touch",
		"what it assumed":              "you assumed",
		"what it does not know":        "what you do not know",
		"the scenario it will write":   "the scenario you will write, by name",
		"how sure it is, and what for": "as a percentage",
	} {
		if !strings.Contains(brief, said) {
			t.Errorf("the brief never asks for %s, so a session writes a restatement without it: it never says %q", part, said)
		}
	}
}

// What a step promises, and what krewe does with each promise. The scenario is the one krewe can
// check, and the touches are the one krewe compares before it lets two sessions run at once.
func TestTheDesignBriefSaysWhatEveryStepMustName(t *testing.T) {
	brief := flowed(shippedSkill(t, "design").Brief)

	for what, said := range map[string]string{
		"that a step names the scenario that proves it": "the scenario that proves it",
		"that an unnamed scenario cannot be checked":    "names no scenario cannot be checked",
		"what proves it states the value delivered":     "states the value the step delivers",
	} {
		if !strings.Contains(brief, said) {
			t.Errorf("the brief never says %s, and a step with no scenario is a step krewe cannot check: it never says %q", what, said)
		}
	}

	// The take compares these lines, so a file the step only reads, written down here, refuses a
	// second step that has to write it. A step that names nothing collides with nothing, which is the
	// failure nobody sees until two sessions write over each other.
	for what, said := range map[string]string{
		"that it names every file the step writes": "names every file the step writes",
		"that it names no file it only reads":      "names no file the step only reads",
		"what an empty list costs":                 "names no file collides with nothing",
	} {
		if !strings.Contains(brief, said) {
			t.Errorf("the brief never says %s about what this touches: it never says %q", what, said)
		}
	}
}

// The one line the whole gate rests on. A session writes the design, and a session that approved its
// own design would be agreeing with itself about the thing nobody else read yet.
func TestTheDesignBriefNeverApprovesTheDesign(t *testing.T) {
	brief := flowed(shippedSkill(t, "design").Brief)

	for _, said := range []string{"never approve the design", "only the operator approves"} {
		if !strings.Contains(brief, said) {
			t.Errorf("the brief never says %q, so a session that wrote a design will go and approve it", said)
		}
	}
}

// Five words for things this system removed. A session reading one of them goes looking for
// machinery that is not there, and writes a design around it.
func TestTheDesignSkillHoldsNoneOfTheRemovedWords(t *testing.T) {
	design := shippedSkill(t, "design")

	for _, gone := range []string{"stage", "job", "flow", "role", "controller"} {
		for where, body := range everyFileOf(t, design) {
			if strings.Contains(strings.ToLower(body), gone) {
				t.Errorf("%s says %q, which names a thing this system removed: a session reading it goes looking for machinery that is not there",
					where, gone)
			}
		}
	}
}

// A brief that tells a session to type a command the tool does not have is worse than no brief: the
// session reads the refusal as its own mistake and works around it. The commands are read out of the
// build in this checkout rather than out of a list kept here, which would go stale the same way.
func TestEveryCommandTheDesignBriefNamesExists(t *testing.T) {
	named := commandsNamedIn(shippedSkill(t, "design").Brief)
	if len(named) == 0 {
		t.Fatal("the brief names no krewe command at all, so this test proves nothing and the brief tells a session to run nothing")
	}

	held := kreweCommands(t)
	for _, command := range named {
		if !held[command] {
			t.Errorf("the brief tells a session to run `krewe %s`, and the tool has no such command", command)
		}
	}
	t.Logf("checked %d commands the brief names: %v", len(named), named)
}

// everyFileOf reads the skill's whole directory, because the words it must not carry are a rule about
// the skill rather than about its brief. Files is empty for a skill read from disk, which is how this
// one reaches a session, so the directory is what there is to read.
func everyFileOf(t *testing.T, s Skill) map[string]string {
	t.Helper()

	if s.Dir == "" {
		t.Fatal("the shipped design skill has no directory, so there is nothing to read")
	}
	bodies := map[string]string{}
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		t.Fatalf("reading the design skill's directory: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		at := filepath.Join(s.Dir, entry.Name())
		body, err := os.ReadFile(at)
		if err != nil {
			t.Fatalf("reading %s: %v", at, err)
		}
		bodies[entry.Name()] = string(body)
	}
	if len(bodies) == 0 {
		t.Fatal("the design skill's directory holds no files, so this test proves nothing")
	}
	return bodies
}

// wordInACommand is a plain lowercase word, which is what the words of a krewe command are. An
// argument is written `<feature>`, `[<address>]` or `--file`, and none of those matches, so the
// command stops where its arguments start.
var wordInACommand = regexp.MustCompile(`^[a-z][a-z-]*$`)

// commandsNamedIn reads every `krewe <command>` out of a brief, as the phrase without its arguments.
// "write it with `krewe path set <feature> --file <path>`" answers "path set".
func commandsNamedIn(brief string) []string {
	var named []string
	for _, line := range strings.Split(brief, "\n") {
		words := strings.Fields(strings.NewReplacer("`", " ", "(", " ", ")", " ").Replace(line))
		for at, word := range words {
			if word != "krewe" {
				continue
			}
			var phrase []string
			for _, next := range words[at+1:] {
				if !wordInACommand.MatchString(strings.Trim(next, ".,;:\"")) {
					break
				}
				phrase = append(phrase, strings.Trim(next, ".,;:\""))
			}
			if len(phrase) == 0 {
				continue
			}
			if command := strings.Join(phrase, " "); !slices.Contains(named, command) {
				named = append(named, command)
			}
		}
	}
	return named
}

// kreweCommands is every command the real tool lists, read from the build in this checkout. It reads
// the whole phrase rather than the first word, because `design` and `design set` are two commands
// and a brief can name the second while the tool has only the first.
func kreweCommands(t *testing.T) map[string]bool {
	t.Helper()

	built := filepath.Join(t.TempDir(), "krewe")
	if out, err := exec.Command("go", "build", "-o", built, "../../cmd/krewe").CombinedOutput(); err != nil {
		t.Fatalf("building the tool: %v\n%s", err, out)
	}
	out, err := exec.Command(built, "help").CombinedOutput()
	if err != nil {
		t.Fatalf("krewe help: %v\n%s", err, out)
	}

	// The command column starts each entry at a two space indent, and a continuation line is indented
	// further, so the first words of a two space indented line are a command and nothing else is.
	commands := map[string]bool{}
	listing := false
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "commands:") {
			listing = true
			continue
		}
		if !listing || !strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "   ") {
			continue
		}
		var phrase []string
		for _, word := range strings.Fields(line) {
			if !wordInACommand.MatchString(word) {
				break
			}
			phrase = append(phrase, word)
		}
		// Every leading part of an entry is a command in its own right where the tool has one, and
		// `design set` is listed while `design` is listed separately above it.
		for at := range phrase {
			commands[strings.Join(phrase[:at+1], " ")] = true
		}
	}
	if len(commands) == 0 {
		t.Fatalf("no commands were read out of krewe help, so this test proves nothing:\n%s", out)
	}
	return commands
}
