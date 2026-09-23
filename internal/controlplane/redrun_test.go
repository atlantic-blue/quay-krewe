package controlplane_test

import (
	"context"
	"strings"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/controlplane"
	"github.com/atlantic-blue/quay-krewe/internal/model"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/secrets"
	"github.com/atlantic-blue/quay-krewe/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/prototext"
)

// The gate that holds the word done until krewe saw this step's tests fail.
//
// A test nobody saw fail proves nothing: it passes on code that is right, and it passes on an
// assertion that is empty, and the two read the same from outside. So a step carries the record of a
// run that failed with at least one scenario in it, and the finish reads that record.
//
// The other half is here too: the refusal for a step nobody checked stays what it was. They are two
// moves for the operator, so one sentence for both would send half of them to the wrong command.

// TDD-1. The code passed its tests on the first run, so nothing was ever seen to fail.
func TestAStepWhoseTestsNobodySawFailCannotFinish(t *testing.T) {
	s, held, feature := aTakenStep(t)
	ctx := context.Background()
	recordOneRun(t, held, feature, store.ProofPassing, 1)

	_, err := s.FinishStep(ctx, &quaycrewv1.FinishStepRequest{
		Feature: feature, Number: 1, State: "done", Result: "shipped",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("finishing a step nobody saw fail answered %v, want FailedPrecondition", err)
	}
	said := status.Convert(err).Message()
	if !strings.Contains(said, "nobody saw step 1's tests fail") {
		t.Errorf("the refusal reads %q, and it has to say what is missing", said)
	}
	if !strings.Contains(said, "krewe step check") {
		t.Errorf("the refusal reads %q, and it has to say what to type next", said)
	}
	if got := stepOne(t, s, feature).GetState(); got != store.StepTaken {
		t.Errorf("the refused finish left step 1 in state %q, and it closed nothing", got)
	}
}

// The flow the gate is for: the tests go in and the run goes red, the code goes in and the run goes
// green, and the word done is spoken over both.
func TestAStepSeenToFailAndThenToPassFinishes(t *testing.T) {
	s, held, feature := aTakenStep(t)
	ctx := context.Background()
	recordOneRun(t, held, feature, store.ProofFailing, 1)
	recordOneRun(t, held, feature, store.ProofPassing, 1)

	written, err := s.FinishStep(ctx, &quaycrewv1.FinishStepRequest{
		Feature: feature, Number: 1, State: "done", Result: "shipped",
	})
	if err != nil {
		t.Fatalf("finishing a step seen to fail and then to pass: %v", err)
	}
	if got := written.GetStep().GetState(); got != store.StepDone {
		t.Fatalf("the step came back in state %q, want %q", got, store.StepDone)
	}
	// Read off the message rather than through a field name, so this test compiles before the field
	// exists and fails on what it says rather than on the build.
	if !strings.Contains(prototext.Format(written.GetStep()), "red_run_at") {
		t.Error("the finished step carries no red run, and the gate it went through reads one")
	}
}

// Zero scenarios never passes, and it never fails either. A name filter that matches nothing, and a
// command the shell cannot start, both report a failure that executed no test.
func TestARunThatFailedWithNoScenarioIsNoRedRun(t *testing.T) {
	s, held, feature := aTakenStep(t)
	ctx := context.Background()
	recordOneRun(t, held, feature, store.ProofFailing, 0)
	recordOneRun(t, held, feature, store.ProofPassing, 1)

	_, err := s.FinishStep(ctx, &quaycrewv1.FinishStepRequest{
		Feature: feature, Number: 1, State: "done", Result: "shipped",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("finishing a step whose failing run ran nothing answered %v, want FailedPrecondition", err)
	}
	if said := status.Convert(err).Message(); !strings.Contains(said, "nobody saw step 1's tests fail") {
		t.Errorf("the refusal reads %q, and a run of no scenario is not a red run", said)
	}
}

// Two states and two moves. A step nobody checked is told to run the check, and it is not told to go
// and watch a test fail it has not written a run for yet.
func TestAStepNobodyCheckedIsStillToldThatNothingRan(t *testing.T) {
	s, _, feature := aTakenStep(t)

	_, err := s.FinishStep(context.Background(), &quaycrewv1.FinishStepRequest{
		Feature: feature, Number: 1, State: "done", Result: "shipped",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("finishing a step nobody checked answered %v, want FailedPrecondition", err)
	}
	said := status.Convert(err).Message()
	if !strings.Contains(said, "nothing checked step 1") {
		t.Fatalf("the refusal reads %q, and nothing ran on this step at all", said)
	}
	if strings.Contains(said, "nobody saw step 1's tests fail") {
		t.Errorf("the refusal reads %q, and it names the move after the one the operator is on", said)
	}
}

// The word that ends a step nobody will finish. It promises nothing about the tests, so it reads no
// red run.
func TestStoppingAStepReadsNoRedRun(t *testing.T) {
	s, _, feature := aTakenStep(t)

	written, err := s.FinishStep(context.Background(), &quaycrewv1.FinishStepRequest{
		Feature: feature, Number: 1, State: "stopped", Result: "the approach was wrong",
	})
	if err != nil {
		t.Fatalf("stopping a step nobody checked: %v", err)
	}
	if got := written.GetStep().GetState(); got != store.StepStopped {
		t.Fatalf("the step came back in state %q, want %q", got, store.StepStopped)
	}
}

// A second attempt proves itself again. A red run carried over from the attempt that stopped would
// let the next session write its code first and finish on the record of work it did not do.
func TestTakingAStepAgainClearsItsRedRun(t *testing.T) {
	s, held, feature := aTakenStep(t)
	ctx := context.Background()
	recordOneRun(t, held, feature, store.ProofFailing, 1)
	recordOneRun(t, held, feature, store.ProofPassing, 1)
	if _, err := s.FinishStep(ctx, &quaycrewv1.FinishStepRequest{
		Feature: feature, Number: 1, State: "stopped", Result: "the approach was wrong",
	}); err != nil {
		t.Fatalf("stopping step 1: %v", err)
	}
	if _, err := s.TakeStep(ctx, &quaycrewv1.TakeStepRequest{Feature: feature, Number: 1}); err != nil {
		t.Fatalf("taking step 1 again: %v", err)
	}
	settle(t, s)
	recordOneRun(t, held, feature, store.ProofPassing, 1)

	_, err := s.FinishStep(ctx, &quaycrewv1.FinishStepRequest{
		Feature: feature, Number: 1, State: "done", Result: "shipped",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("finishing the second attempt answered %v, want FailedPrecondition", err)
	}
	if said := status.Convert(err).Message(); !strings.Contains(said, "nobody saw step 1's tests fail") {
		t.Errorf("the refusal reads %q, and the second attempt saw nothing fail", said)
	}
}

// aTakenStep is a project whose design is approved, whose feature holds one step, and whose step a
// session holds. It answers the store as well, because a verdict is written where krewe's own check
// writes one, and a real check needs a container and a proof command this has neither of.
func aTakenStep(t *testing.T) (*controlplane.Server, store.Store, string) {
	t.Helper()
	ctx := context.Background()
	held := store.NewMemory()
	s := controlplane.NewServer(controlplane.Config{
		Store:    held,
		Runner:   &model.FakeRunner{Reply: "taken"},
		Provider: &sandbox.FakeProvider{},
		Secrets:  secrets.NewMemory(),
	})
	_, projectID := newProject(t, s)
	if _, err := s.SetDesign(ctx, &quaycrewv1.SetDesignRequest{
		Project: projectID, Body: "# Bills\n",
	}); err != nil {
		t.Fatalf("SetDesign: %v", err)
	}
	if _, err := s.ApproveDesign(ctx, &quaycrewv1.ApproveDesignRequest{Project: projectID}); err != nil {
		t.Fatalf("ApproveDesign: %v", err)
	}
	feature := newFeatureWithAStep(t, s, projectID)
	if _, err := s.TakeStep(ctx, &quaycrewv1.TakeStepRequest{Feature: feature, Number: 1}); err != nil {
		t.Fatalf("TakeStep: %v", err)
	}
	settle(t, s)
	return s, held, feature
}

// recordOneRun writes what one run of the step's scenario reported, with the count the test names.
func recordOneRun(t *testing.T, held store.Store, feature, state string, scenarios int32) {
	t.Helper()
	if _, err := held.RecordProof(context.Background(), feature, 1, store.ProofResult{
		State: state, ScenariosRun: scenarios, Output: "what the run printed",
	}); err != nil {
		t.Fatalf("RecordProof: %v", err)
	}
}
