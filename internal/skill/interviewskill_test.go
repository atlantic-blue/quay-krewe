package skill

import (
	"slices"
	"strings"
	"testing"
)

// The interview skill is what a session reads before anybody asks the operator anything. It is prose
// and nothing else: no gate reads it, so what it fails to say is a thing the interview will not do.
//
// It declares the krewe binary because it ends in a krewe command, and a session in an image without
// the tool is refused with a sentence rather than told to type what it cannot run.
//
// What the brief must say is read in features/interview.feature, over the skill this build ships.
// What is read here is the manifest: a skill that names a secret is left out of every session of a
// workspace that has not set it, and this one must reach every session.
func TestTheShippedInterviewSkillLoads(t *testing.T) {
	interview := shippedSkill(t, "interview")

	if interview.Version != 1 {
		t.Errorf("the interview skill is version %d, and a session is pinned to the one it started with",
			interview.Version)
	}
	if !slices.Contains(interview.Binaries, "krewe") {
		t.Errorf("the interview skill declares the binaries %v, and it writes the page with the tool",
			interview.Binaries)
	}
	if len(interview.Secrets) != 0 {
		t.Errorf("the interview skill names the secrets %v, so a workspace that has not set them is a workspace where the skill is left out of every session",
			interview.Secrets)
	}
	if interview.HasSetup {
		t.Error("the interview skill runs a setup script, and prose has nothing to set up")
	}
	// The summary is the line every session holding the skill reads on every conversation, so it says
	// when to reach for the brief rather than what the skill is called.
	summary := strings.ToLower(interview.Summary)
	for _, said := range []string{"operator", "before"} {
		if !strings.Contains(summary, said) {
			t.Errorf("the summary %q never says %q, so nothing tells a session about to design something to ask first",
				interview.Summary, said)
		}
	}
}

// A brief that tells a session to type a command the tool does not have is worse than no brief: the
// session reads the refusal as its own mistake and works around it.
func TestEveryCommandTheInterviewBriefNamesExists(t *testing.T) {
	named := commandsNamedIn(shippedSkill(t, "interview").Brief)
	if len(named) == 0 {
		t.Fatal("the brief names no krewe command at all, so nothing writes the page down")
	}

	held := kreweCommands(t)
	for _, command := range named {
		if !held[command] {
			t.Errorf("the brief tells a session to run `krewe %s`, and the tool has no such command", command)
		}
	}
	t.Logf("checked %d commands the brief names: %v", len(named), named)
}

// The seven headings the control plane refuses a write without, and what the interview puts under
// each one. A brief naming six of them teaches every session to write a page the system rejects, and
// a brief that names them without saying what goes under them leaves a session to guess where the
// constraints belong.
func TestTheInterviewBriefNamesEveryHeadingTheWriteNeeds(t *testing.T) {
	brief := shippedSkill(t, "interview").Brief

	for _, heading := range []string{
		"Goal", "Direction", "Assumptions", "Decisions for the operator", "Done when", "Not doing",
		"Open questions",
	} {
		if !strings.Contains(brief, heading) {
			t.Errorf("the brief never writes out %q, and a page missing that heading is refused", heading)
		}
	}
	// The two answers with nowhere obvious to go. A session that is not told where they belong puts
	// the constraints in the direction and the scope nowhere at all.
	for what, said := range map[string]string{
		"where the constraints go": "the constraints",
		"where the scope goes":     "what is out of scope",
	} {
		if !strings.Contains(brief, said) {
			t.Errorf("the brief never says %s: it never says %q", what, said)
		}
	}
}

// The one line the whole gate rests on. A session writes the page, and a session that approved its
// own page would be agreeing with itself about what the operator wants.
func TestTheInterviewBriefNeverApprovesTheStage(t *testing.T) {
	brief := flowed(shippedSkill(t, "interview").Brief)

	for _, said := range []string{"never approve", "only the operator approves"} {
		if !strings.Contains(brief, said) {
			t.Errorf("the brief never says %q, so a session that wrote the page will go and approve it", said)
		}
	}
}
