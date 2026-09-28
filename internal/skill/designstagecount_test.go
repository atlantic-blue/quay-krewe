package skill_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/skill"
	"github.com/atlantic-blue/quay-krewe/internal/store"
)

// How many of the seven a file has to name before it is read as the file that lists them. The design
// brief carries the order, and the flow-map brief names the two stages it works on without listing
// anything, so a rule that bound every mention would ask the wrong file for the whole order.
const namesTheOrder = 5

// A skill that counts the design stages says seven, because there are seven. The interview was added
// under the discovery, and a brief still saying six teaches every session an order with the first
// stage missing from it: the session reads the repository, writes what the code does, and nothing in
// its reading says what the operator wanted instead.
//
// The count is read as the word in front of the noun rather than as a number anywhere in the text,
// because a brief says "6,000 characters is refused" one line from the word stage, and the parts of a
// restatement are six and stay six. The text is read flowed, so a count the wrapping split over two
// lines is caught the same as one on a single line.
func TestNoShippedSkillCountsTheDesignStagesAsSix(t *testing.T) {
	sixStages := regexp.MustCompile(`(?i)\bsix\b[^.]{0,60}\bstages?\b`)

	read := 0
	for _, held := range shipped(t) {
		for where, body := range everyFileOfSkill(t, held) {
			read++
			for _, said := range sixStages.FindAllString(flowedBrief(body), -1) {
				t.Errorf("%s counts the design stages as six, saying %q, and there are %d of them: %v",
					where, said, len(store.DesignStages()), store.DesignStages())
			}
		}
	}
	if read == 0 {
		t.Fatal("no skill file was read, so this test proves nothing")
	}
	t.Logf("read %d files of %d shipped skills", read, len(shipped(t)))
}

// The file that lists the stages lists all of them, in the store's own order, and the interview is
// first. A list that is right about six names and silent about the seventh is the shape that reads as
// complete, so nobody goes looking for what is missing.
func TestASkillThatListsTheDesignStagesListsAllSevenInOrder(t *testing.T) {
	stages := store.DesignStages()

	listings := 0
	for _, held := range shipped(t) {
		for where, body := range everyFileOfSkill(t, held) {
			var named []string
			for _, stage := range stages {
				if at := namesStage(body, stage); at >= 0 {
					named = append(named, stage)
				}
			}
			if len(named) < namesTheOrder {
				continue
			}
			listings++

			for _, stage := range stages {
				if namesStage(body, stage) < 0 {
					t.Errorf("%s lists the design stages and never names %q, so a session reading it writes an order that is missing a stage",
						where, stage)
				}
			}
			// The interview comes first because it is the one stage the operator answers rather than
			// the repository, so a list that puts it under another stage puts a reading of the code
			// before anything says what the code is for.
			first := namesStage(body, store.StageInterview)
			for _, stage := range stages[1:] {
				at := namesStage(body, stage)
				if at >= 0 && (first < 0 || first > at) {
					t.Errorf("%s names %q before the interview, and the interview is the first of the %d stages",
						where, stage, len(stages))
				}
			}
		}
	}
	if listings == 0 {
		t.Fatal("no skill file lists the design stages, so this test proves nothing")
	}
	t.Logf("read the stage order out of %d skill files", listings)
}

// A session writing a stage reads the interview before it reads anything else. Every stage under the
// interview is written from a repository, and the repository says what the code does today rather
// than what the operator asked for, so a stage written without the page describes the wrong thing
// confidently.
//
// The two skills here are the ones a session reads before it writes a stage from a repository. The
// interview skill is not one of them: it writes the page, and it cannot read itself first.
func TestTheStageSkillsSendASessionToTheInterviewFirst(t *testing.T) {
	for _, name := range []string{"design", "discover"} {
		brief := flowedBrief(shipped(t)[name].Brief)

		if !strings.Contains(brief, store.StageInterview) {
			t.Errorf("the %s brief never names the interview, so a session writing a stage never reads the page the operator approved first", name)
		}
		if !strings.Contains(brief, "the interview first") {
			t.Errorf("the %s brief never says to read the interview first, so a session reads the repository and writes what the code does", name)
		}
	}
}

// namesStage is where a file first names one stage, as a whole word, and -1 for a file that does not.
// A whole word, because "discovery" is a word inside no other stage's name but "design_system" sits
// inside prose about the design and the match has to be the name rather than the prefix.
func namesStage(body, stage string) int {
	word := regexp.MustCompile(`(?i)(^|[^\w_])` + regexp.QuoteMeta(stage) + `($|[^\w_])`)
	at := word.FindStringIndex(body)
	if at == nil {
		return -1
	}
	return at[0]
}

// shipped is every skill in skills/ at the root of this repository, by name. The loader refuses a
// brief past its ceiling, so a skill that grew past one page fails here rather than going quietly
// missing from every session.
func shipped(t *testing.T) map[string]skill.Skill {
	t.Helper()

	held, err := skill.Load("../../skills")
	if err != nil {
		t.Fatalf("loading the shipped skills: %v", err)
	}
	if len(held) == 0 {
		t.Fatal("skills/ holds no skills, so this test proves nothing")
	}
	by := map[string]skill.Skill{}
	for _, one := range held {
		by[one.Name] = one
	}
	return by
}

// everyFileOfSkill reads the skill's whole directory, because what a brief sends a session to is part
// of what the skill says. The example a skill ships sits in a directory of its own and is read too.
func everyFileOfSkill(t *testing.T, held skill.Skill) map[string]string {
	t.Helper()

	if held.Dir == "" {
		t.Fatalf("the shipped %s skill has no directory, so there is nothing to read", held.Name)
	}
	bodies := map[string]string{}
	err := filepath.WalkDir(held.Dir, func(at string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}
		body, err := os.ReadFile(at)
		if err != nil {
			return err
		}
		bodies[at] = string(body)
		return nil
	})
	if err != nil {
		t.Fatalf("reading the %s skill's directory: %v", held.Name, err)
	}
	if len(bodies) == 0 {
		t.Fatalf("the %s skill ships no prose, so there is nothing to read", held.Name)
	}
	return bodies
}

// flowedBrief reads a brief as the prose it is rather than as the lines it is wrapped into, so a
// reflow that changes no words cannot fail a test about what the brief says. The in-package tests
// have their own copy of this; an external test package cannot reach it.
func flowedBrief(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}
