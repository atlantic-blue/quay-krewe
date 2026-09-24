package controlplane

import (
	"strings"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
)

// The protocol a session follows to produce the two runs the word done waits for.
//
// Krewe records a red run and then a green run, and both are read out of the working tree the
// session built in. Only that session can hold the tree in the state each run needs: the tests and
// no code, and then the finished work. Nothing stopped it at either moment, so no red run was ever
// recorded, and a step whose code is already written can produce none.
//
// The session records neither run itself. A session inside a step reaches no control plane, so the
// text says where to stop and who runs the check.
//
// The text carries all of this only where the project asked to have its steps checked. A project
// that set no proof command has no check to produce runs for, so its take text is what it was.

func aCheckedStep() *quaycrewv1.Step {
	return &quaycrewv1.Step{
		Number:        3,
		Title:         "The store holds a project's brief",
		CheckRequired: true,
	}
}

func TestTheTakeTextOfACheckedStepSaysWhereToStop(t *testing.T) {
	text := takeText(aCheckedStep(), 5, "house-bills", false, 7)

	for _, want := range []string{
		// The tree krewe reads, named the way the git skill names it.
		"/home/agent/shared/worktrees/$QC_SESSION_ID/",
		// The other place krewe reads, and why it stays empty of a clone.
		"Do not clone the repository into /home/agent/workspace or into /tmp",
		// A test commit that does not compile runs nothing, and nothing is not a red run.
		"the smallest stubs the tests need to compile",
		"0 scenarios",
		// The first stop, and who runs the check at it.
		"Reply with the sha of that commit. Then stop.",
		"the operator runs krewe step check 7.3",
		// The gate that comes on the moment the red run is recorded.
		"Change no test file after the red run.",
		// The second stop.
		"Open the pull request, reply with the sha at its head, and stop again.",
		// The check refuses a step whose restatement nobody approved.
		"Build only when the operator tells you the restatement is approved",
		// What the text said before this step still stands.
		"Deliver this step as one pull request.",
		"Do not merge it. Report the address of the pull request.",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the take text of a checked step does not carry %q:\n%s", want, text)
		}
	}
	// The restatement stays the last thing the text says, because the last thing it says is the thing
	// the session does next, and this session restates before it builds.
	if !strings.HasSuffix(text, "Then stop and say you are ready.\n") {
		t.Errorf("the take text of a checked step does not end with the restatement:\n%s", text)
	}
}

// Measured on 2026-09-24: krewe step show in a session's exec answers "this session was not told
// where the system is". A text that told the session to check its own work would send it to a command
// that cannot answer, and the step would stop on that rather than on the step.
func TestTheTakeTextNeverTellsTheSessionToRunTheCheck(t *testing.T) {
	text := takeText(aCheckedStep(), 5, "house-bills", false, 7)

	if !strings.Contains(text, "Do not run krewe step check yourself") {
		t.Errorf("the take text does not refuse the session the check:\n%s", text)
	}
	for _, unwanted := range []string{
		"Run krewe step check",
		"run krewe step check " + "7.3",
		"check the step yourself",
	} {
		if strings.Contains(text, unwanted) {
			t.Errorf("the take text carries %q, which sends the session to a command it cannot reach:\n%s",
				unwanted, text)
		}
	}
}

// The address is the feature's number and the step's, so a step 3 in feature 7 sends the operator to
// 7.3. A text naming the step's number alone names another feature's third step, which is a step of
// another path and may not even be taken.
func TestTheTakeTextNamesTheStepByFeatureAndNumber(t *testing.T) {
	step := aCheckedStep()
	step.Number = 4

	text := takeText(step, 6, "house-bills", false, 12)

	if !strings.Contains(text, "krewe step check 12.4") {
		t.Errorf("the take text does not name the step as 12.4:\n%s", text)
	}
}

// The other half. A project that set no proof command is checked by nobody, so none of this is in
// its take text and the last thing it reads is still how the work is delivered.
func TestTheTakeTextOfAnUncheckedStepCarriesNoneOfIt(t *testing.T) {
	step := aCheckedStep()
	step.CheckRequired = false

	text := takeText(step, 5, "house-bills", false, 7)

	for _, unwanted := range []string{
		"/home/agent/shared/worktrees/$QC_SESSION_ID/",
		"krewe step check",
		"red run",
		"Do not clone the repository",
		"the smallest stubs the tests need to compile",
	} {
		if strings.Contains(text, unwanted) {
			t.Errorf("the take text of an unchecked step carries %q, and it should not:\n%s", unwanted, text)
		}
	}
	if !strings.HasSuffix(text, takeDelivery+"\n") {
		t.Errorf("the take text of an unchecked step does not end with the delivery paragraph:\n%s", text)
	}
}
