package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The flow map skill is the page that plays a mockup, and the brief beside it is the only thing that
// tells a session how to write a screen. The page draws markup now, so a brief describing a list of
// shapes teaches every design session to write a file the control plane refuses.
//
// What the example must survive is read in features/flowmap.feature, which writes it to a real
// control plane. What is read here is the brief: the field the markup goes in, the two attributes
// that carry the contract, where the colours and the fonts come from, and the fact that nothing
// inside a screen can reach an address.
func TestTheShippedFlowMapSkillLoads(t *testing.T) {
	flowMap := shippedSkill(t, "flow-map")

	if flowMap.Version != 2 {
		t.Errorf("the flow map skill is version %d, and a session is pinned to the one it started with", flowMap.Version)
	}
	if len(flowMap.Binaries) != 0 {
		t.Errorf("the flow map skill declares the binaries %v, and a binary the sandbox image lacks leaves the skill out of every session that image runs", flowMap.Binaries)
	}
	if len(flowMap.Secrets) != 0 {
		t.Errorf("the flow map skill names the secrets %v, so a workspace that has not set them is a workspace where the skill is left out of every session", flowMap.Secrets)
	}
	if flowMap.HasSetup {
		t.Error("the flow map skill runs a setup script, and a page and its prose have nothing to set up")
	}
	// The summary is the line every session holding the skill reads on every conversation, so it says
	// when to reach for the brief rather than what the skill is called.
	summary := strings.ToLower(flowMap.Summary)
	for _, said := range []string{"screens", "play"} {
		if !strings.Contains(summary, said) {
			t.Errorf("the summary %q never says %q, so nothing tells a session about to draw a project's screens to open the brief", flowMap.Summary, said)
		}
	}
}

// A screen is a document now, so the brief has to say what a session writes into it. Each line below
// is a thing a session cannot work out from the schema, and each one was a fault measured on real
// screens: markup that named no component, a font fetched from a host, a colour nobody approved.
func TestTheFlowMapBriefSaysHowAScreenIsWritten(t *testing.T) {
	brief := flowed(shippedSkill(t, "flow-map").Brief)

	for what, said := range map[string]string{
		"which field the markup goes in":            "the markup of the body of one screen",
		"that the field is called html":             "`html`",
		"how a part names its component":            "`data-component`",
		"how a part opens another screen":           "`data-to`",
		"where the colours and the fonts come from": "the approved design_system stage",
		"that the check refuses any other value":    "refuses a colour or a font that stage does not name",
		"that a screen reaches no network":          "there is no network",
		"where a font file comes from":              "a font comes from an asset of the design system",
		"that a mark is better written inline":      "an inline `<svg>`",
		"that a stylesheet reaches one screen":      "reaches that screen only",
		"the size of a phone":                       "390 by 844",
		"the size of a browser":                     "1280 by 800",
	} {
		if !strings.Contains(brief, said) {
			t.Errorf("the brief never says %s, so a session writes a screen without it: it never says %q", what, said)
		}
	}
}

// The way off the old form. A brief still naming the shape kinds sends a session to write a field the
// schema refuses, and the session reads the refusal as its own mistake.
func TestTheFlowMapBriefTeachesNoListOfShapes(t *testing.T) {
	flowMap := shippedSkill(t, "flow-map")
	brief := flowed(flowMap.Brief)

	for _, gone := range []string{"`el`", "eyebrow", "btn2", "chips", "spacer"} {
		if strings.Contains(brief, gone) {
			t.Errorf("the brief says %q, which is the shape list this system removed: a session reading it writes a mockup the control plane refuses", gone)
		}
	}
}

// The example is the part a session copies, so the brief has to send it there, and the files have to
// be beside the brief rather than named and missing.
func TestTheFlowMapSkillShipsTheExampleItNames(t *testing.T) {
	flowMap := shippedSkill(t, "flow-map")
	if flowMap.Dir == "" {
		t.Fatal("the shipped flow map skill has no directory, so there is nothing to read")
	}

	for _, named := range []string{"example/design-system.json", "example/flows.json"} {
		if !strings.Contains(flowMap.Brief, named) {
			t.Errorf("the brief never names %s, so nothing sends a session to the worked example", named)
		}
		at := filepath.Join(flowMap.Dir, filepath.FromSlash(named))
		read, err := os.Stat(at)
		if err != nil {
			t.Errorf("the brief names %s and the skill does not ship it: %v", named, err)
			continue
		}
		if read.Size() == 0 {
			t.Errorf("%s is empty, so a session copying it copies nothing", named)
		}
	}
}

// A brief that tells a session to type a command the tool does not have is worse than no brief: the
// session reads the refusal as its own mistake and works around it. The commands are read out of the
// build in this checkout rather than out of a list kept here, which would go stale the same way.
func TestEveryCommandTheFlowMapBriefNamesExists(t *testing.T) {
	named := commandsNamedIn(shippedSkill(t, "flow-map").Brief)
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
