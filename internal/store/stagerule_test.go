package store

import (
	"errors"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
)

// The rule that orders the six stages, read on its own. Both stores call these two functions, so a
// case proved here is proved for Postgres and for memory at once, and the suite that holds the two to
// one behaviour cannot be green while they disagree about what settles a stage.

// A stage is settled when the operator's word stands on the text it holds now, or when the stage was
// skipped. Nothing writes the skipped column yet, and the rule reads it, so this is where that branch
// is proved: the command line that skips discovery arrives on top of a rule that already honours it.
func TestWhatSettlesADesignStage(t *testing.T) {
	for _, one := range []struct {
		what    string
		stage   *quaycrewv1.DesignStage
		settled bool
	}{
		{what: "a stage nobody wrote", stage: nil, settled: false},
		{
			what:  "a stage written and never approved",
			stage: &quaycrewv1.DesignStage{Version: 1},
		},
		{
			what:    "a stage approved at the version it holds",
			stage:   &quaycrewv1.DesignStage{Version: 2, ApprovedVersion: 2},
			settled: true,
		},
		{
			what:  "a stage approved and then written over",
			stage: &quaycrewv1.DesignStage{Version: 3, ApprovedVersion: 2},
		},
		{
			what:    "a stage that was skipped",
			stage:   &quaycrewv1.DesignStage{Version: 1, Skipped: true},
			settled: true,
		},
		{
			what:    "a stage skipped after it was written over",
			stage:   &quaycrewv1.DesignStage{Version: 4, ApprovedVersion: 2, Skipped: true},
			settled: true,
		},
	} {
		t.Run(one.what, func(t *testing.T) {
			if got := DesignStageSatisfied(one.stage); got != one.settled {
				t.Errorf("%s reads settled=%t, want %t", one.what, got, one.settled)
			}
		})
	}
}

// The refusal names the first stage without a word on it, not the nearest one. An operator writing
// the data model with nothing approved has one move, and it is at the top of the list: a refusal
// naming the mockups would send them to a stage they cannot write either.
func TestTheBlockingStageIsTheFirstOneWithoutAWord(t *testing.T) {
	approved := func(stage string) *quaycrewv1.DesignStage {
		return &quaycrewv1.DesignStage{Stage: stage, Version: 1, ApprovedVersion: 1}
	}
	written := func(stage string) *quaycrewv1.DesignStage {
		return &quaycrewv1.DesignStage{Stage: stage, Version: 1}
	}

	for _, one := range []struct {
		what     string
		writing  string
		held     []*quaycrewv1.DesignStage
		blocking string
	}{
		{
			what:    "discovery waits for nothing, so it is written into an empty project",
			writing: StageDiscovery,
		},
		{
			what:     "stories waits for discovery",
			writing:  StageStories,
			blocking: StageDiscovery,
		},
		{
			what:    "stories goes once discovery is approved",
			writing: StageStories,
			held:    []*quaycrewv1.DesignStage{approved(StageDiscovery)},
		},
		{
			what:     "the data model names the stories, and not the mockups just below it",
			writing:  StageDataModel,
			held:     []*quaycrewv1.DesignStage{approved(StageDiscovery), written(StageStories)},
			blocking: StageStories,
		},
		{
			what:     "a stage skipped at the top still lets the refusal name the next gap",
			writing:  StageMockups,
			held:     []*quaycrewv1.DesignStage{{Stage: StageDiscovery, Skipped: true}, approved(StageStories)},
			blocking: StageDesignSystem,
		},
		{
			what:    "the architecture goes once the five before it are approved",
			writing: StageArchitecture,
			held: []*quaycrewv1.DesignStage{
				approved(StageDiscovery), approved(StageStories), approved(StageDesignSystem),
				approved(StageMockups), approved(StageDataModel),
			},
		},
		{
			what:    "a stage after this one carries no weight, approved or not",
			writing: StageDiscovery,
			held:    []*quaycrewv1.DesignStage{written(StageArchitecture)},
		},
		{
			what:    "a name outside the six waits for nothing, because the store refuses it first",
			writing: "wireframes",
			held:    nil,
		},
	} {
		t.Run(one.what, func(t *testing.T) {
			if got := BlockingDesignStage(one.writing, one.held); got != one.blocking {
				t.Errorf("writing %s is blocked by %q, want %q", one.writing, got, one.blocking)
			}
		})
	}
}

// The six are a list with an order, and every caller reads the position out of it rather than
// counting for itself.
func TestTheSixStagesAreInOrder(t *testing.T) {
	want := []string{"discovery", "stories", "design_system", "mockups", "data_model", "architecture"}
	got := DesignStages()
	if len(got) != len(want) {
		t.Fatalf("the system holds %d stages: %v", len(got), got)
	}
	for at, named := range want {
		if got[at] != named {
			t.Fatalf("stage %d is %q, want %q: the whole order is %v", at, got[at], named, got)
		}
		position, known := DesignStagePosition(named)
		if !known || position != int32(at) {
			t.Errorf("%s reads position %d known=%t, want %d", named, position, known, at)
		}
	}
	if _, known := DesignStagePosition("wireframes"); known {
		t.Error("a name outside the six was given a position, so it could be written as a stage")
	}
	if before := DesignStagesBefore(StageMockups); len(before) != 3 {
		t.Errorf("the mockups wait for %v, want the three above them", before)
	}
	if before := DesignStagesBefore(StageDiscovery); len(before) != 0 {
		t.Errorf("discovery waits for %v, and nothing comes before it", before)
	}
}

// The artifact is json because the column is jsonb. Both stores ask this before writing, so the
// memory store refuses exactly what Postgres refuses.
func TestAnArtifactIsJSONOrNothing(t *testing.T) {
	for _, one := range []struct {
		what     string
		artifact string
		refused  bool
	}{
		{what: "no artifact at all", artifact: ""},
		{what: "an object", artifact: `{"screens":[]}`},
		{what: "an array", artifact: `[1,2,3]`},
		{what: "a bare string, which is a json document", artifact: `"a flow"`},
		{what: "prose", artifact: "the flows are over there", refused: true},
		{what: "an object nobody closed", artifact: `{"screens":`, refused: true},
	} {
		t.Run(one.what, func(t *testing.T) {
			err := CheckDesignStageArtifact(one.artifact)
			if one.refused && !errors.Is(err, ErrArtifactNotJSON) {
				t.Errorf("%s was taken, and the jsonb column would refuse it: %v", one.what, err)
			}
			if !one.refused && err != nil {
				t.Errorf("%s was refused: %v", one.what, err)
			}
		})
	}
}
