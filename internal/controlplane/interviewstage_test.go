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
)

// The interview is the first of the seven stages, and the only one whose answers come from the
// operator rather than from a repository.
//
// It is held to the same shape as every other stage: seven headings, in one order, inside one page.
// That is what makes the answers usable by the stages under it, and it is read here because the
// interview is the stage a session writes from a conversation rather than from files, which is the
// one most likely to come back as a transcript.

// The stage is written into an empty project, because nothing comes before it.
func TestTheInterviewIsWrittenIntoAnEmptyProject(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	ctx := context.Background()
	_, projectID := newProject(t, s)

	written, err := s.SetDesignStage(ctx, &quaycrewv1.SetDesignStageRequest{
		Project: projectID, Stage: store.StageInterview, Body: stageBody(store.StageInterview),
	})
	if err != nil {
		t.Fatalf("writing the interview into a project that holds nothing: %v", err)
	}
	if written.GetStage().GetPosition() != 0 {
		t.Errorf("the interview sits at position %d, and it is the first of the seven",
			written.GetStage().GetPosition())
	}
}

// Every heading in turn, taken out of a whole page. A page an operator cannot read the decisions or
// the constraints out of is a page that decides nothing, and the stages under it then guess.
//
// The seven are written out here rather than read from the rule they are checked against. A list
// read from the rule shrinks with the rule, so a heading dropped from it would take its own test
// away and this would stay green.
func TestAnInterviewMissingAHeadingIsRefused(t *testing.T) {
	for _, heading := range []string{
		"Goal", "Direction", "Assumptions", "Decisions for the operator", "Done when", "Not doing",
		"Open questions",
	} {
		t.Run(heading, func(t *testing.T) {
			s := newServer(&model.FakeRunner{})
			ctx := context.Background()
			_, projectID := newProject(t, s)

			_, err := s.SetDesignStage(ctx, &quaycrewv1.SetDesignStageRequest{
				Project: projectID, Stage: store.StageInterview,
				Body: without(stageBody(store.StageInterview), heading),
			})

			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("an interview with no %q heading answered %v, want InvalidArgument", heading, err)
			}
			said := status.Convert(err).Message()
			if !strings.Contains(said, heading) {
				t.Errorf("the refusal reads %q, and it has to name the heading that is missing", said)
			}
			if !strings.Contains(said, store.StageInterview) {
				t.Errorf("the refusal reads %q, and it has to name the stage it is about", said)
			}
			if held := stagesHeld(t, s, projectID); stageInList(held, store.StageInterview) != nil {
				t.Error("the refused write left an interview stage behind")
			}
		})
	}
}

// A project in flight. Its discovery was approved before the interview stage existed, migration 0081
// wrote a skipped interview row in front of it, and the gate that reads the stages lets a take
// through: the operator runs the interview later, and nothing waits on it meanwhile.
//
// The skipped row comes from a store that answers reads the way the migrated database does. It
// replaces one approved row with a skipped one, which is a weaker state than the store wrote, so
// nothing here lets through a take the rule would refuse.
func TestAProjectWhoseInterviewIsSkippedTakesAStep(t *testing.T) {
	memory := store.NewMemory()
	s := controlplane.NewServer(controlplane.Config{
		Store:    &interviewSkipped{Store: memory},
		Runner:   &model.FakeRunner{Reply: "taken"},
		Provider: &sandbox.FakeProvider{},
		Secrets:  secrets.NewMemory(),
	})
	ctx := context.Background()
	projectID, featureID := newPathToTake(t, s)
	for _, stage := range store.DesignStages() {
		writeStage(t, s, projectID, stage)
		approveStage(t, s, projectID, stage)
	}

	held := stagesHeld(t, s, projectID)
	if found := stageInList(held, store.StageInterview); !found.GetSkipped() {
		t.Fatalf("the project reads its interview as %+v, and a project in flight reads it as skipped", found)
	}

	taken, err := s.TakeStep(ctx, &quaycrewv1.TakeStepRequest{Feature: featureID, Number: 1})
	if err != nil {
		t.Fatalf("taking a step of a project whose interview is skipped: %v", err)
	}
	if taken.GetStep().GetState() != store.StepTaken {
		t.Fatalf("the step came back in state %q, want %q", taken.GetStep().GetState(), store.StepTaken)
	}
	settle(t, s)
}

// interviewSkipped answers a stage listing the way a database migration 0081 has run over answers
// it: the interview row is there, it is skipped, and nothing else about the project changes.
type interviewSkipped struct {
	store.Store
}

func (i *interviewSkipped) ListDesignStages(ctx context.Context, project string) (
	[]*quaycrewv1.DesignStage, error) {
	held, err := i.Store.ListDesignStages(ctx, project)
	if err != nil {
		return nil, err
	}
	for at, one := range held {
		if one.GetStage() != store.StageInterview {
			continue
		}
		skipped := &quaycrewv1.DesignStage{
			Id: one.GetId(), Project: one.GetProject(), Stage: store.StageInterview,
			Position: one.GetPosition(), Skipped: true, Version: one.GetVersion(),
		}
		answered := make([]*quaycrewv1.DesignStage, len(held))
		copy(answered, held)
		answered[at] = skipped
		return answered, nil
	}
	return held, nil
}

// without is the page with one heading taken out, and the lines under it left where they were. The
// rest of the document is untouched, so what the refusal answers is the missing heading and nothing
// else.
func without(page, heading string) string {
	kept := make([]string, 0, strings.Count(page, "\n")+1)
	for _, line := range strings.Split(page, "\n") {
		if strings.EqualFold(strings.TrimSpace(line), "## "+heading) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}
