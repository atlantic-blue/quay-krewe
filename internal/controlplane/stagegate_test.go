package controlplane_test

import (
	"context"
	"strings"
	"testing"
	"time"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/controlplane"
	"github.com/atlantic-blue/quay-krewe/internal/model"
	"github.com/atlantic-blue/quay-krewe/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The gate a staged project's path stands behind. The stages say what a person will see before the
// data and the architecture are written, and they mean nothing while a step can be taken over a
// mockup nobody read, so a take reads them and refuses until every one of the six carries the word.
//
// The other half is here too, and it is the half that costs something if it breaks: a project that
// holds no stage is refused nothing. Three projects are being built that way today.

// STAGE-4. Five approved and the sixth written and unread, which is the state an operator is in the
// moment before they finish. The refusal names the stage they are on and says what to type.
func TestAStagedProjectWithFiveApprovedStagesRefusesAStep(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	ctx := context.Background()
	projectID, featureID := newPathToTake(t, s)

	for _, stage := range store.DesignStages() {
		writeStage(t, s, projectID, stage)
		if stage == store.StageArchitecture {
			break
		}
		approveStage(t, s, projectID, stage)
	}

	_, err := s.TakeStep(ctx, &quaycrewv1.TakeStepRequest{Feature: featureID, Number: 1})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("taking a step of a staged project answered %v, want FailedPrecondition", err)
	}
	said := status.Convert(err).Message()
	if !strings.Contains(said, store.StageArchitecture) {
		t.Errorf("the refusal reads %q, and it has to name the stage without the word", said)
	}
	if !strings.Contains(said, "krewe stage approve") {
		t.Errorf("the refusal reads %q, and it has to say what to type next", said)
	}
	if held := stepOne(t, s, featureID); held.GetState() != store.StepReady {
		t.Errorf("the refused take left step 1 in state %q, and it starts nothing", held.GetState())
	}
}

// The first stage without the word rather than the nearest one, for the reason the refusal on a write
// names the first: an operator sent to the data model is sent to a stage they cannot write either.
func TestTheRefusalNamesTheFirstStageWithoutApproval(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	ctx := context.Background()
	projectID, featureID := newPathToTake(t, s)

	writeStage(t, s, projectID, store.StageDiscovery)
	approveStage(t, s, projectID, store.StageDiscovery)
	writeStage(t, s, projectID, store.StageStories)

	_, err := s.TakeStep(ctx, &quaycrewv1.TakeStepRequest{Feature: featureID, Number: 1})
	said := status.Convert(err).Message()
	if !strings.Contains(said, store.StageStories) {
		t.Fatalf("the refusal reads %q, and the first stage without the word is the stories", said)
	}
	if strings.Contains(said, store.StageArchitecture) {
		t.Errorf("the refusal reads %q, and it sends the operator to a stage they cannot write yet", said)
	}
}

// A stage nobody wrote and a stage nobody read are two different moves, so the refusal says which one
// this is. An operator told to approve a text that is not there goes looking for what they missed.
func TestAStageNobodyWroteIsRefusedAsUnwritten(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	ctx := context.Background()
	projectID, featureID := newPathToTake(t, s)

	writeStage(t, s, projectID, store.StageDiscovery)
	approveStage(t, s, projectID, store.StageDiscovery)

	_, err := s.TakeStep(ctx, &quaycrewv1.TakeStepRequest{Feature: featureID, Number: 1})
	said := status.Convert(err).Message()
	if !strings.Contains(said, "krewe stage set") {
		t.Fatalf("the refusal reads %q, and the stories are not there to approve yet", said)
	}
}

// The six approved, which is what the gate exists to let through.
func TestAStagedProjectWithEveryStageApprovedTakesAStep(t *testing.T) {
	s := newServer(&model.FakeRunner{Reply: "taken"})
	ctx := context.Background()
	projectID, featureID := newPathToTake(t, s)

	for _, stage := range store.DesignStages() {
		writeStage(t, s, projectID, stage)
		approveStage(t, s, projectID, stage)
	}

	taken, err := s.TakeStep(ctx, &quaycrewv1.TakeStepRequest{Feature: featureID, Number: 1})
	if err != nil {
		t.Fatalf("taking a step of a project whose six stages are approved: %v", err)
	}
	if taken.GetStep().GetState() != store.StepTaken {
		t.Fatalf("the step came back in state %q, want %q", taken.GetStep().GetState(), store.StepTaken)
	}
	settle(t, s)
}

// STAGE-5. The half that is not about the stages at all: a project that holds none is refused
// nothing, which is every project made before the stages existed and every one being built today.
func TestAProjectWithNoStagesTakesAStepAsBefore(t *testing.T) {
	s := newServer(&model.FakeRunner{Reply: "taken"})
	ctx := context.Background()
	projectID, featureID := newPathToTake(t, s)

	stages, err := s.ListDesignStages(ctx, &quaycrewv1.ListDesignStagesRequest{Project: projectID})
	if err != nil {
		t.Fatalf("ListDesignStages: %v", err)
	}
	if len(stages.GetStages()) != 0 {
		t.Fatalf("the project holds %d stages, and this is about a project that holds none",
			len(stages.GetStages()))
	}

	taken, err := s.TakeStep(ctx, &quaycrewv1.TakeStepRequest{Feature: featureID, Number: 1})
	if err != nil {
		t.Fatalf("taking a step of a project that holds no stages: %v", err)
	}
	if taken.GetSession().GetId() == "" {
		t.Fatal("the take started no session, so nothing is building the step")
	}
	settle(t, s)
}

// The order of the two gates. The design document is read first and its refusal is the one an
// unapproved project gets, whatever its stages say, because that check is the one every project has.
func TestAnUnapprovedDesignIsStillTheFirstRefusal(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	ctx := context.Background()
	_, projectID := newProject(t, s)
	featureID := newFeatureWithAStep(t, s, projectID)
	if _, err := s.SetDesign(ctx, &quaycrewv1.SetDesignRequest{
		Project: projectID, Body: "# Bills\n",
	}); err != nil {
		t.Fatalf("SetDesign: %v", err)
	}
	for _, stage := range store.DesignStages() {
		writeStage(t, s, projectID, stage)
		approveStage(t, s, projectID, stage)
	}

	_, err := s.TakeStep(ctx, &quaycrewv1.TakeStepRequest{Feature: featureID, Number: 1})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("taking a step of an unapproved project answered %v, want FailedPrecondition", err)
	}
	if said := status.Convert(err).Message(); !strings.Contains(said, "krewe design approve") {
		t.Fatalf("the refusal reads %q, and the design document is what this project is missing", said)
	}
}

// newPathToTake is a project whose design is approved and whose feature holds one ready step: the
// smallest thing a take can be refused or allowed on, with nothing about the stages in it.
func newPathToTake(t *testing.T, s *controlplane.Server) (projectID, featureID string) {
	t.Helper()
	ctx := context.Background()
	_, projectID = newProject(t, s)
	if _, err := s.SetDesign(ctx, &quaycrewv1.SetDesignRequest{
		Project: projectID, Body: "# Bills\n",
	}); err != nil {
		t.Fatalf("SetDesign: %v", err)
	}
	if _, err := s.ApproveDesign(ctx, &quaycrewv1.ApproveDesignRequest{Project: projectID}); err != nil {
		t.Fatalf("ApproveDesign: %v", err)
	}
	return projectID, newFeatureWithAStep(t, s, projectID)
}

func newFeatureWithAStep(t *testing.T, s *controlplane.Server, projectID string) string {
	t.Helper()
	ctx := context.Background()
	feature, err := s.AddFeature(ctx, &quaycrewv1.AddFeatureRequest{
		Project: projectID, Title: "The bills are listed",
	})
	if err != nil {
		t.Fatalf("AddFeature: %v", err)
	}
	if _, err := s.SetPath(ctx, &quaycrewv1.SetPathRequest{
		Feature:  feature.GetFeature().GetId(),
		Document: "## 1. The store holds a project's brief\n",
	}); err != nil {
		t.Fatalf("SetPath: %v", err)
	}
	return feature.GetFeature().GetId()
}

func writeStage(t *testing.T, s *controlplane.Server, projectID, stage string) {
	t.Helper()
	if _, err := s.SetDesignStage(context.Background(), &quaycrewv1.SetDesignStageRequest{
		Project: projectID, Stage: stage, Body: stageBody(stage),
	}); err != nil {
		t.Fatalf("SetDesignStage %s: %v", stage, err)
	}
}

func approveStage(t *testing.T, s *controlplane.Server, projectID, stage string) {
	t.Helper()
	if _, err := s.ApproveDesignStage(context.Background(), &quaycrewv1.ApproveDesignStageRequest{
		Project: projectID, Stage: stage,
	}); err != nil {
		t.Fatalf("ApproveDesignStage %s: %v", stage, err)
	}
}

func stepOne(t *testing.T, s *controlplane.Server, featureID string) *quaycrewv1.Step {
	t.Helper()
	held, err := s.ListSteps(context.Background(), &quaycrewv1.ListStepsRequest{Feature: featureID})
	if err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	if len(held.GetSteps()) != 1 {
		t.Fatalf("the feature holds %d steps, want the one the setup wrote", len(held.GetSteps()))
	}
	return held.GetSteps()[0]
}

// settle waits for the exec a take let go of, so the test does not end while a goroutine is still
// writing to the store behind it.
func settle(t *testing.T, s *controlplane.Server) {
	t.Helper()
	waiting, giveUp := context.WithTimeout(context.Background(), 10*time.Second)
	defer giveUp()
	s.WaitForExecs(waiting)
	if waiting.Err() != nil {
		t.Fatal("the exec the take started never landed")
	}
}
