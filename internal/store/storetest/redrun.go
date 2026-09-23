package storetest

import (
	"context"
	"errors"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/store"
)

// The record of a run that was seen to fail, held against both implementations.
//
// The rule is one sentence: the word done is refused until a run of this step's scenario failed with
// at least one scenario in it. A memory store that took a finish the real one refuses would let a
// whole path close on tests nobody ever saw fail, pass every scenario, and refuse the same finish the
// moment the call reached Postgres.

func runRedRunConformance(t *testing.T, newDataset func(t *testing.T) Opener) {
	t.Helper()

	// The state the gate exists for: the code passed its tests on the first run, so nothing was ever
	// seen to fail and the tests could be asserting nothing at all.
	t.Run("a step whose tests nobody saw fail is refused the word done", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		feature := aFeatureHoldingOneStep(t, s)
		oneRun(t, s, feature, store.ProofPassing, 1)

		if _, _, err := s.FinishStep(ctx, feature, 1, store.Finish{
			State: store.StepDone, Result: "shipped", ClosedBy: "operator",
		}); !errors.Is(err, store.ErrNoRedRun) {
			t.Fatalf("finishing a step nobody saw fail answered %v, want ErrNoRedRun", err)
		}
		read, err := s.GetStep(ctx, feature, 1)
		if err != nil {
			t.Fatalf("GetStep after the refusal: %v", err)
		}
		if read.GetState() != store.StepTaken {
			t.Fatalf("the refused finish left the step in state %q", read.GetState())
		}
	})

	// The flow the gate is for, and the record it leaves: the tests go in and the run goes red, the
	// code goes in and the run goes green.
	t.Run("a step seen to fail and then to pass carries both runs and finishes", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		feature := aFeatureHoldingOneStep(t, s)
		red := oneRun(t, s, feature, store.ProofFailing, 2)
		if red.GetRedRunAt() == nil {
			t.Fatal("a run that failed with two scenarios in it left no red run on the step")
		}
		if got := red.GetRedRunScenarios(); got != 2 {
			t.Errorf("the red run reads %d scenarios, want the 2 the run reported", got)
		}

		green := oneRun(t, s, feature, store.ProofPassing, 2)
		if green.GetRedRunAt() == nil {
			t.Fatal("the passing run took the red run off the step")
		}
		written, _, err := s.FinishStep(ctx, feature, 1, store.Finish{
			State: store.StepDone, Result: "shipped", ClosedBy: "operator",
		})
		if err != nil {
			t.Fatalf("finishing a step seen to fail and then to pass: %v", err)
		}
		if written.GetState() != store.StepDone {
			t.Fatalf("the step came back in state %q, want %q", written.GetState(), store.StepDone)
		}
		if written.GetRedRunAt() == nil {
			t.Error("the finished step carries no red run, and the gate it went through reads one")
		}
	})

	// Zero scenarios never passes, and it never fails either. A name filter that matches nothing, and
	// a command the shell cannot start, both report a failure that executed no test.
	t.Run("a run that failed with no scenario in it is no red run", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		feature := aFeatureHoldingOneStep(t, s)
		empty := oneRun(t, s, feature, store.ProofFailing, 0)
		if empty.GetRedRunAt() != nil {
			t.Fatalf("a failing run of no scenario left a red run at %v", empty.GetRedRunAt())
		}
		oneRun(t, s, feature, store.ProofPassing, 1)

		if _, _, err := s.FinishStep(ctx, feature, 1, store.Finish{
			State: store.StepDone, Result: "shipped", ClosedBy: "operator",
		}); !errors.Is(err, store.ErrNoRedRun) {
			t.Fatalf("finishing on a failing run of no scenario answered %v, want ErrNoRedRun", err)
		}
	})

	// Two states and two refusals. A step nobody checked is a different move for the operator from a
	// step whose tests nobody saw fail, so the two never answer with one error.
	t.Run("a step nobody checked is refused as unchecked rather than as unseen", func(t *testing.T) {
		s := newDataset(t)(t)
		feature := aFeatureHoldingOneStep(t, s)

		_, _, err := s.FinishStep(context.Background(), feature, 1, store.Finish{
			State: store.StepDone, Result: "shipped", ClosedBy: "operator",
		})
		if !errors.Is(err, store.ErrNotChecked) {
			t.Fatalf("finishing a step nobody checked answered %v, want ErrNotChecked", err)
		}
		if errors.Is(err, store.ErrNoRedRun) {
			t.Fatal("a step nobody checked was refused for the run after the one it is missing")
		}
	})

	// The word that ends a step nobody will finish promises nothing about its tests.
	t.Run("a step is stopped with no red run on it", func(t *testing.T) {
		s := newDataset(t)(t)
		feature := aFeatureHoldingOneStep(t, s)

		written, _, err := s.FinishStep(context.Background(), feature, 1, store.Finish{
			State: store.StepStopped, Result: "the approach was wrong", ClosedBy: "operator",
		})
		if err != nil {
			t.Fatalf("stopping a step nobody checked: %v", err)
		}
		if written.GetState() != store.StepStopped {
			t.Fatalf("the step came back in state %q, want %q", written.GetState(), store.StepStopped)
		}
	})

	// A second attempt proves itself again, the way it starts again with no restatement and no
	// approval. A red run carried over would let the next session write its code first.
	t.Run("taking a step again clears the red run", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		feature := aFeatureHoldingOneStep(t, s)
		oneRun(t, s, feature, store.ProofFailing, 1)
		oneRun(t, s, feature, store.ProofPassing, 1)
		if _, _, err := s.FinishStep(ctx, feature, 1, store.Finish{
			State: store.StepStopped, Result: "the approach was wrong", ClosedBy: "operator",
		}); err != nil {
			t.Fatalf("stopping the first attempt: %v", err)
		}
		taken, _, err := s.TakeStep(ctx, feature, 1, "session-two")
		if err != nil {
			t.Fatalf("taking the step again: %v", err)
		}
		if taken.GetRedRunAt() != nil || taken.GetRedRunScenarios() != 0 {
			t.Fatalf("the retaken step reads a red run at %v over %d scenarios",
				taken.GetRedRunAt(), taken.GetRedRunScenarios())
		}

		oneRun(t, s, feature, store.ProofPassing, 1)
		if _, _, err := s.FinishStep(ctx, feature, 1, store.Finish{
			State: store.StepDone, Result: "shipped", ClosedBy: "operator",
		}); !errors.Is(err, store.ErrNoRedRun) {
			t.Fatalf("finishing the second attempt answered %v, want ErrNoRedRun", err)
		}
	})

	// A path rewrite keeps what the system owns. The red run is the record of a run that happened, so
	// a document nobody can write it from must not take it away.
	t.Run("a path rewrite keeps the red run", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		feature := aFeatureHoldingOneStep(t, s)
		oneRun(t, s, feature, store.ProofFailing, 1)
		oneRun(t, s, feature, store.ProofPassing, 1)

		writePath(t, s, feature, store.Step{Number: 1, Title: "the first", Intention: "said again"})
		read, err := s.GetStep(ctx, feature, 1)
		if err != nil {
			t.Fatalf("GetStep after the rewrite: %v", err)
		}
		if read.GetRedRunAt() == nil || read.GetRedRunScenarios() != 1 {
			t.Fatalf("the rewritten step reads a red run at %v over %d scenarios, and one ran",
				read.GetRedRunAt(), read.GetRedRunScenarios())
		}
	})
}

// aFeatureHoldingOneStep is a project with one feature, one step, and a session holding that step,
// which is the state every finish is spoken over.
//
// The project says how one scenario is run before the take, because that is what binds a step to
// these gates. A project that says nothing is left as it was before they existed, and the cases
// about that scope stand their own project up.
func aFeatureHoldingOneStep(t *testing.T, s store.Store) string {
	t.Helper()
	project := newProject(t, s, "acme", "house-bills")
	provesItsSteps(t, s, project.GetId())
	feature := newFeature(t, s, project, "the bills")
	writePath(t, s, feature.GetId(), store.Step{Number: 1, Title: "the first"})
	if _, _, err := s.TakeStep(context.Background(), feature.GetId(), 1, "session-one"); err != nil {
		t.Fatalf("TakeStep: %v", err)
	}
	return feature.GetId()
}

// provesItsSteps records what one scenario run looks like in this project, which is what turns the
// gates on for every step taken after it.
func provesItsSteps(t *testing.T, s store.Store, project string) {
	t.Helper()
	if _, err := s.SetProofCommand(context.Background(), project, store.ProofSettings{
		Command: "go test ./features/... -run '{scenario}'",
	}); err != nil {
		t.Fatalf("SetProofCommand: %v", err)
	}
}

// oneRun writes what one run of the step's scenario reported, with the count the case names, and
// answers the step as the write left it.
func oneRun(t *testing.T, s store.Store, feature, state string, scenarios int32) *quaycrewv1.Step {
	t.Helper()
	written, err := s.RecordProof(context.Background(), feature, 1, store.ProofResult{
		State: state, ScenariosRun: scenarios, Output: "what the run printed",
	})
	if err != nil {
		t.Fatalf("RecordProof: %v", err)
	}
	return written
}

// runRedRunScopeConformance holds both stores to which steps the rule binds.
//
// The rule arrived after work had started. A step a session was holding then has its code written
// and its tests passing, so no run of it can go red any more, and a rule that bound every row would
// leave that step unable to close at all. So the row says whether the take that started it happened
// under the rule, and the word done reads that.
//
// A row that carries no requirement is one no take under the rule reached. These cases stand one up
// the way the upgrade leaves one: the path writes the step and nothing takes it.
func runRedRunScopeConformance(t *testing.T, newDataset func(t *testing.T) Opener) {
	t.Helper()

	// The case the scope exists for. The step is checked, nothing was ever seen to fail, and the word
	// done is given rather than refused.
	t.Run("a step the rule never bound finishes with no red run", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		feature := aFeatureHoldingOneStep(t, s)
		writePath(t, s, feature,
			store.Step{Number: 1, Title: "the first"},
			store.Step{Number: 2, Title: "the second"})
		if _, err := s.RecordProof(ctx, feature, 2, store.ProofResult{
			State: store.ProofPassing, ScenariosRun: 1, Output: "what the run printed",
		}); err != nil {
			t.Fatalf("RecordProof: %v", err)
		}

		written, _, err := s.FinishStep(ctx, feature, 2, store.Finish{
			State: store.StepDone, Result: "shipped", ClosedBy: "operator",
		})
		if err != nil {
			t.Fatalf("finishing a step the rule never bound: %v", err)
		}
		if written.GetState() != store.StepDone {
			t.Fatalf("the step came back in state %q, want %q", written.GetState(), store.StepDone)
		}
	})

	// The other gate on the same step, read the same way. A row no take reached carries neither
	// requirement, so the word done is given over it, and the step beside it that a take did reach
	// is refused for both in turn.
	t.Run("a step the rule never bound is refused neither gate", func(t *testing.T) {
		s := newDataset(t)(t)
		feature := aFeatureHoldingOneStep(t, s)
		writePath(t, s, feature,
			store.Step{Number: 1, Title: "the first"},
			store.Step{Number: 2, Title: "the second"})

		written, _, err := s.FinishStep(context.Background(), feature, 2, store.Finish{
			State: store.StepDone, Result: "shipped", ClosedBy: "operator",
		})
		if err != nil {
			t.Fatalf("finishing a step no take reached: %v", err)
		}
		if written.GetState() != store.StepDone {
			t.Fatalf("the step came back in state %q, want %q", written.GetState(), store.StepDone)
		}
	})

	// The two steps sit in one path, so a step the rule bound is refused beside a step it did not.
	// Read per step rather than per project, because the rule arrives while one path is part built.
	t.Run("a step the take bound is refused beside a step the rule never bound", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		feature := aFeatureHoldingOneStep(t, s)
		writePath(t, s, feature,
			store.Step{Number: 1, Title: "the first"},
			store.Step{Number: 2, Title: "the second"})
		oneRun(t, s, feature, store.ProofPassing, 1)

		if _, _, err := s.FinishStep(ctx, feature, 1, store.Finish{
			State: store.StepDone, Result: "shipped", ClosedBy: "operator",
		}); !errors.Is(err, store.ErrNoRedRun) {
			t.Fatalf("finishing the taken step answered %v, want ErrNoRedRun", err)
		}
	})

	// A path rewrite keeps what the system owns, and the requirement is the system's: a document a
	// session writes could otherwise take a step out of the rule by saying its title again.
	t.Run("a path rewrite keeps the requirement the take wrote", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		feature := aFeatureHoldingOneStep(t, s)
		oneRun(t, s, feature, store.ProofPassing, 1)

		writePath(t, s, feature, store.Step{Number: 1, Title: "the first", Intention: "said again"})
		if _, _, err := s.FinishStep(ctx, feature, 1, store.Finish{
			State: store.StepDone, Result: "shipped", ClosedBy: "operator",
		}); !errors.Is(err, store.ErrNoRedRun) {
			t.Fatalf("finishing after the rewrite answered %v, want ErrNoRedRun", err)
		}
	})

	// A second attempt is a take, so it binds a step the rule never reached. The row the upgrade left
	// is the record of one attempt, and the attempt that starts now builds under the rule.
	t.Run("taking a step the rule never bound binds it", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		feature := aFeatureHoldingOneStep(t, s)
		writePath(t, s, feature,
			store.Step{Number: 1, Title: "the first"},
			store.Step{Number: 2, Title: "the second"})
		if _, _, err := s.TakeStep(ctx, feature, 2, "session-two"); err != nil {
			t.Fatalf("taking the second step: %v", err)
		}
		if _, err := s.RecordProof(ctx, feature, 2, store.ProofResult{
			State: store.ProofPassing, ScenariosRun: 1, Output: "what the run printed",
		}); err != nil {
			t.Fatalf("RecordProof: %v", err)
		}

		if _, _, err := s.FinishStep(ctx, feature, 2, store.Finish{
			State: store.StepDone, Result: "shipped", ClosedBy: "operator",
		}); !errors.Is(err, store.ErrNoRedRun) {
			t.Fatalf("finishing the step this take bound answered %v, want ErrNoRedRun", err)
		}
	})
}

// runProofCommandScopeConformance holds both stores to which projects these gates bind at all.
//
// The check and the red run both stand on a proof command: krewe runs it to reach a verdict, and
// without one there is nothing for it to run. A project that never set one could not pass the check,
// so its steps could not close, and the two projects this system was building at the time were both
// in that state. So the take reads the project. A project that says how one scenario is run gets the
// gates, and a project that says nothing works the way it did before them.
//
// It is read at the take and never at the finish, for the reason the red run rule gives: a step
// already in flight has no way to meet a gate it was not taken under.
func runProofCommandScopeConformance(t *testing.T, newDataset func(t *testing.T) Opener) {
	t.Helper()

	// The state this scope exists for. Nobody checked the step and nobody saw its tests fail, and the
	// word done is given rather than refused.
	t.Run("a step taken in a project with no proof command finishes with no check", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		feature := aStepTakenInAProjectThatProvesNothing(t, s)

		written, _, err := s.FinishStep(ctx, feature, 1, store.Finish{
			State: store.StepDone, Result: "shipped", ClosedBy: "operator",
		})
		if err != nil {
			t.Fatalf("finishing a step of a project with no proof command: %v", err)
		}
		if written.GetState() != store.StepDone {
			t.Fatalf("the step came back in state %q, want %q", written.GetState(), store.StepDone)
		}
	})

	// The flags the take wrote, read off the step, so a finish that passed for some other reason is
	// not read as the scope working.
	t.Run("a take in a project with no proof command binds neither gate", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		feature := aStepTakenInAProjectThatProvesNothing(t, s)

		read, err := s.GetStep(ctx, feature, 1)
		if err != nil {
			t.Fatalf("GetStep: %v", err)
		}
		if read.GetCheckRequired() {
			t.Error("a take in a project with no proof command bound the step to the check")
		}
		if read.GetRedRunRequired() {
			t.Error("a take in a project with no proof command bound the step to the red run")
		}
	})

	t.Run("a take in a project that proves its steps binds both gates", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		feature := aFeatureHoldingOneStep(t, s)

		read, err := s.GetStep(ctx, feature, 1)
		if err != nil {
			t.Fatalf("GetStep: %v", err)
		}
		if !read.GetCheckRequired() {
			t.Error("a take in a project that proves its steps left the check off")
		}
		if !read.GetRedRunRequired() {
			t.Error("a take in a project that proves its steps left the red run off")
		}
		if _, _, err := s.FinishStep(ctx, feature, 1, store.Finish{
			State: store.StepDone, Result: "shipped", ClosedBy: "operator",
		}); !errors.Is(err, store.ErrNotChecked) {
			t.Fatalf("finishing a step nobody checked answered %v, want ErrNotChecked", err)
		}
	})

	// The proof command arriving later binds the steps taken after it, and leaves the one in flight
	// where it is. Read at the finish instead, a project that turned the gates on would strand the
	// step a session was already holding, which is the fault this whole scope answers.
	t.Run("a proof command set after the take leaves that step unbound", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		feature := aStepTakenInAProjectThatProvesNothing(t, s)
		project, err := s.GetFeature(ctx, feature)
		if err != nil {
			t.Fatalf("GetFeature: %v", err)
		}
		provesItsSteps(t, s, project.GetProject())

		written, _, err := s.FinishStep(ctx, feature, 1, store.Finish{
			State: store.StepDone, Result: "shipped", ClosedBy: "operator",
		})
		if err != nil {
			t.Fatalf("finishing the step the proof command arrived after: %v", err)
		}
		if written.GetState() != store.StepDone {
			t.Fatalf("the step came back in state %q, want %q", written.GetState(), store.StepDone)
		}
	})

	// A path rewrite keeps what the take bound, the way it keeps the red run requirement: a document
	// a session writes could otherwise take a step out of the check by saying its title again.
	t.Run("a path rewrite keeps the check the take bound", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		feature := aFeatureHoldingOneStep(t, s)

		writePath(t, s, feature, store.Step{Number: 1, Title: "the first", Intention: "said again"})
		read, err := s.GetStep(ctx, feature, 1)
		if err != nil {
			t.Fatalf("GetStep after the rewrite: %v", err)
		}
		if !read.GetCheckRequired() {
			t.Error("the rewritten step reads as one the check never bound")
		}
	})
}

// aStepTakenInAProjectThatProvesNothing is the state the two projects building at the time were in:
// a path the operator approved, a session holding step 1, and nothing saying how one scenario runs.
func aStepTakenInAProjectThatProvesNothing(t *testing.T, s store.Store) string {
	t.Helper()
	feature := newFeature(t, s, newProject(t, s, "acme", "house-rent"), "the rent")
	writePath(t, s, feature.GetId(), store.Step{Number: 1, Title: "the first"})
	if _, _, err := s.TakeStep(context.Background(), feature.GetId(), 1, "session-one"); err != nil {
		t.Fatalf("TakeStep: %v", err)
	}
	return feature.GetId()
}
