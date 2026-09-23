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

		writePath(t, s, feature, store.Step{Number: 1, Title: "the first, said again"})
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
func aFeatureHoldingOneStep(t *testing.T, s store.Store) string {
	t.Helper()
	feature := newFeature(t, s, newProject(t, s, "acme", "house-bills"), "the bills")
	writePath(t, s, feature.GetId(), store.Step{Number: 1, Title: "the first"})
	if _, _, err := s.TakeStep(context.Background(), feature.GetId(), 1, "session-one"); err != nil {
		t.Fatalf("TakeStep: %v", err)
	}
	return feature.GetId()
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
