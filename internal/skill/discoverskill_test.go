package skill

import (
	"slices"
	"strings"
	"testing"
)

// The discover skill is what a session reads before it reads somebody's repository. Like the design
// skill it is prose and nothing else: no controller runs because it exists, and no gate reads it. So
// the brief is the whole of the capability.
//
// What the brief must say is read in features/discover.feature, over the skill this build ships. The
// two rules here are the ones a scenario cannot see: the manifest a session is given the skill by,
// and whether the commands it tells a session to type are commands the tool has.
func TestTheShippedDiscoverSkillLoads(t *testing.T) {
	discover := shippedSkill(t, "discover")

	if discover.Version != 2 {
		t.Errorf("the discover skill is version %d, and a session is pinned to the one it started with", discover.Version)
	}
	if !slices.Contains(discover.Binaries, "krewe") {
		t.Errorf("the discover skill declares the binaries %v, and it writes what it found with the tool", discover.Binaries)
	}
	if len(discover.Secrets) != 0 {
		t.Errorf("the discover skill names the secrets %v, so a workspace that has not set them is a workspace where the skill is left out of every session", discover.Secrets)
	}
	if discover.HasSetup {
		t.Error("the discover skill runs a setup script, and prose has nothing to set up")
	}
	// The summary is the line every session holding the skill reads on every conversation, so it says
	// when to reach for the brief rather than what the skill is called.
	summary := strings.ToLower(discover.Summary)
	for _, said := range []string{"repository", "already"} {
		if !strings.Contains(summary, said) {
			t.Errorf("the summary %q never says %q, so nothing tells a session taking on an existing repository to open the brief", discover.Summary, said)
		}
	}
}

// A brief that tells a session to type a command the tool does not have is worse than no brief: the
// session reads the refusal as its own mistake and works around it. The commands are read out of the
// build in this checkout rather than out of a list kept here, which would go stale the same way.
func TestEveryCommandTheDiscoverBriefNamesExists(t *testing.T) {
	named := commandsNamedIn(shippedSkill(t, "discover").Brief)
	if len(named) == 0 {
		t.Fatal("the brief names no krewe command at all, so this test proves nothing and nothing it says reaches the record")
	}

	held := kreweCommands(t)
	for _, command := range named {
		if !held[command] {
			t.Errorf("the brief tells a session to run `krewe %s`, and the tool has no such command", command)
		}
	}
	t.Logf("checked %d commands the brief names: %v", len(named), named)
}
