package controlplane_test

import (
	"context"
	"strings"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/controlplane"
	"github.com/atlantic-blue/quay-krewe/internal/model"
	"github.com/atlantic-blue/quay-krewe/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The six stages a project is designed in, at the surface a person reaches them through. The store
// holds the order and the clearing; what is proved here is the code a refusal carries and the words
// it says, because an operator who is told no and not which stage cannot act on it.

// STAGE-1, at the wire. The refusal is FailedPrecondition rather than InvalidArgument, because
// nothing about the request is wrong: the same call goes through the moment the stage above it is
// approved.
func TestAStageIsRefusedWhileTheStageBeforeItIsNotApproved(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	ctx := context.Background()
	_, projectID := newProject(t, s)

	_, err := s.SetDesignStage(ctx, &quaycrewv1.SetDesignStageRequest{
		Project: projectID, Stage: store.StageDataModel, Body: stageBody(store.StageDataModel),
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("writing the data model first answered %v, want FailedPrecondition", err)
	}
	said := status.Convert(err).Message()
	if !strings.Contains(said, store.StageDiscovery) {
		t.Errorf("the refusal reads %q, and it has to name the stage to go and approve", said)
	}
	if !strings.Contains(said, "krewe stage approve") {
		t.Errorf("the refusal reads %q, and it has to say what to type next", said)
	}

	stages, err := s.ListDesignStages(ctx, &quaycrewv1.ListDesignStagesRequest{Project: projectID})
	if err != nil {
		t.Fatalf("ListDesignStages: %v", err)
	}
	if len(stages.GetStages()) != 0 {
		t.Fatalf("the refused write left %d stages behind", len(stages.GetStages()))
	}
}

// The order walked the whole way, which is the shape an operator works in, and the answer a project
// gives about itself at each point.
func TestTheSixStagesAreWrittenAndApprovedInOrder(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	ctx := context.Background()
	_, projectID := newProject(t, s)

	for _, stage := range store.DesignStages() {
		written, err := s.SetDesignStage(ctx, &quaycrewv1.SetDesignStageRequest{
			Project: projectID, Stage: stage, Body: stageBody(stage),
		})
		if err != nil {
			t.Fatalf("SetDesignStage %s: %v", stage, err)
		}
		if written.GetStage().GetApproved() {
			t.Errorf("%s came back approved before anybody read it", stage)
		}
		if _, err := s.ApproveDesignStage(ctx, &quaycrewv1.ApproveDesignStageRequest{
			Project: projectID, Stage: stage,
		}); err != nil {
			t.Fatalf("ApproveDesignStage %s: %v", stage, err)
		}
	}

	listed, err := s.ListDesignStages(ctx, &quaycrewv1.ListDesignStagesRequest{Project: projectID})
	if err != nil {
		t.Fatalf("ListDesignStages: %v", err)
	}
	if len(listed.GetStages()) != 6 {
		t.Fatalf("the project holds %d stages, want the six", len(listed.GetStages()))
	}
	for at, stage := range listed.GetStages() {
		if stage.GetStage() != store.DesignStages()[at] {
			t.Errorf("stage %d is %s, want %s", at, stage.GetStage(), store.DesignStages()[at])
		}
		if !stage.GetApproved() {
			t.Errorf("%s came back without the word it was given", stage.GetStage())
		}
	}
}

// STAGE-2, at the wire, and the sentence that makes the rule learnable: an operator who rewrites the
// stories is told which later stages they now have to read again, by name.
func TestWritingAStageSaysWhichApprovalsItTookAway(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	ctx := context.Background()
	_, projectID := newProject(t, s)

	for _, stage := range store.DesignStages() {
		if _, err := s.SetDesignStage(ctx, &quaycrewv1.SetDesignStageRequest{
			Project: projectID, Stage: stage, Body: stageBody(stage),
		}); err != nil {
			t.Fatalf("SetDesignStage %s: %v", stage, err)
		}
		if _, err := s.ApproveDesignStage(ctx, &quaycrewv1.ApproveDesignStageRequest{
			Project: projectID, Stage: stage,
		}); err != nil {
			t.Fatalf("ApproveDesignStage %s: %v", stage, err)
		}
	}

	again, err := s.SetDesignStage(ctx, &quaycrewv1.SetDesignStageRequest{
		Project: projectID, Stage: store.StageStories, Body: "the stories, rethought",
	})
	if err != nil {
		t.Fatalf("SetDesignStage over an approved stage: %v", err)
	}
	if again.GetStage().GetApproved() {
		t.Error("the rewritten stage kept the word on the text that just changed")
	}
	said := strings.Join(again.GetWarnings(), "\n")
	for _, later := range []string{store.StageDesignSystem, store.StageMockups, store.StageDataModel, store.StageArchitecture} {
		if !strings.Contains(said, later) {
			t.Errorf("the warnings read %q, and %s lost its approval without being named", said, later)
		}
	}
	if strings.Contains(said, store.StageDiscovery) {
		t.Errorf("the warnings read %q, and discovery sits above the write and kept its word", said)
	}

	listed, err := s.ListDesignStages(ctx, &quaycrewv1.ListDesignStagesRequest{Project: projectID})
	if err != nil {
		t.Fatalf("ListDesignStages: %v", err)
	}
	for _, stage := range listed.GetStages() {
		if stage.GetStage() == store.StageDiscovery && !stage.GetApproved() {
			t.Error("discovery lost its word, and it sits above the write")
		}
		if stage.GetPosition() >= 1 && stage.GetApproved() {
			t.Errorf("%s kept its word over a text that changed under it", stage.GetStage())
		}
	}
}

// A write that takes no word away says nothing about approvals, so the sentence means something when
// it is there.
func TestAFirstWriteWarnsAboutNoApprovals(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	ctx := context.Background()
	_, projectID := newProject(t, s)

	written, err := s.SetDesignStage(ctx, &quaycrewv1.SetDesignStageRequest{
		Project: projectID, Stage: store.StageDiscovery, Body: "what we asked",
	})
	if err != nil {
		t.Fatalf("SetDesignStage: %v", err)
	}
	for _, said := range written.GetWarnings() {
		if strings.Contains(said, "approval") {
			t.Errorf("a first write said %q, and it took no word away from anybody", said)
		}
	}
}

// The vocabulary is the control plane's, and the refusal names the six rather than only refusing the
// one that was typed.
func TestAStageOutsideTheSixIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	ctx := context.Background()
	_, projectID := newProject(t, s)

	for _, call := range []struct {
		what string
		run  func() error
	}{
		{what: "writing", run: func() error {
			_, err := s.SetDesignStage(ctx, &quaycrewv1.SetDesignStageRequest{
				Project: projectID, Stage: "wireframes", Body: "the wireframes",
			})
			return err
		}},
		{what: "approving", run: func() error {
			_, err := s.ApproveDesignStage(ctx, &quaycrewv1.ApproveDesignStageRequest{
				Project: projectID, Stage: "wireframes",
			})
			return err
		}},
	} {
		t.Run(call.what+" a stage that is not one of the six", func(t *testing.T) {
			err := call.run()
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("%s wireframes answered %v, want InvalidArgument", call.what, err)
			}
			said := status.Convert(err).Message()
			for _, stage := range store.DesignStages() {
				if !strings.Contains(said, stage) {
					t.Errorf("the refusal reads %q, and it leaves out %s", said, stage)
				}
			}
		})
	}
}

// A call that names no stage at all is the same class of mistake, and it says which words are the
// six rather than only that one is missing.
func TestACallThatNamesNoStageSaysWhichSixThereAre(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	ctx := context.Background()
	_, projectID := newProject(t, s)

	_, err := s.SetDesignStage(ctx, &quaycrewv1.SetDesignStageRequest{Project: projectID, Body: "a body"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a write naming no stage answered %v, want InvalidArgument", err)
	}
	if said := status.Convert(err).Message(); !strings.Contains(said, store.StageMockups) {
		t.Errorf("the refusal reads %q, and it has to say what the six are", said)
	}
}

// A project that has written no stage answers with an empty list rather than with six empty stages,
// which is how a reader tells a project designed in stages from one designed the way every project
// was until today.
func TestAProjectThatStagedNothingAnswersWithNothing(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	ctx := context.Background()
	_, projectID := newProject(t, s)

	listed, err := s.ListDesignStages(ctx, &quaycrewv1.ListDesignStagesRequest{Project: projectID})
	if err != nil {
		t.Fatalf("ListDesignStages: %v", err)
	}
	if len(listed.GetStages()) != 0 {
		t.Fatalf("a project nobody staged holds %d stages", len(listed.GetStages()))
	}
}

// A stage with no body is nothing to agree to, and the refusal says how to write one.
func TestApprovingAStageNobodyWroteIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	ctx := context.Background()
	_, projectID := newProject(t, s)

	_, err := s.ApproveDesignStage(ctx, &quaycrewv1.ApproveDesignStageRequest{
		Project: projectID, Stage: store.StageDiscovery,
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("approving an empty stage answered %v, want FailedPrecondition", err)
	}
	if said := status.Convert(err).Message(); !strings.Contains(said, "krewe stage set") {
		t.Errorf("the refusal reads %q, and it has to say what to type next", said)
	}
}

// The artifact is kept as json because a reader opens it as json. Prose in that field is refused
// before it reaches a column that would throw it out.
func TestAnArtifactThatIsNotJSONIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	ctx := context.Background()
	_, projectID := newProject(t, s)

	_, err := s.SetDesignStage(ctx, &quaycrewv1.SetDesignStageRequest{
		Project: projectID, Stage: store.StageDiscovery,
		Body: "what we asked", Artifact: "the flows are over there",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("an artifact that is not json answered %v, want InvalidArgument", err)
	}

	written, err := s.SetDesignStage(ctx, &quaycrewv1.SetDesignStageRequest{
		Project: projectID, Stage: store.StageDiscovery,
		Body: "what we asked", Artifact: `{"asked":["when does it move"]}`,
		ArtifactUrl: "https://example.invalid/discovery",
	})
	if err != nil {
		t.Fatalf("SetDesignStage with json: %v", err)
	}
	if !strings.Contains(written.GetStage().GetArtifact(), "when does it move") {
		t.Errorf("the artifact came back as %q", written.GetStage().GetArtifact())
	}
	if written.GetStage().GetArtifactUrl() != "https://example.invalid/discovery" {
		t.Errorf("the artifact address came back as %q", written.GetStage().GetArtifactUrl())
	}
}

// Every stage call says which project, the way every design call does.
func TestAStageCallWithNoProjectSaysSo(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	ctx := context.Background()

	for what, err := range map[string]error{
		"listing": func() error {
			_, err := s.ListDesignStages(ctx, &quaycrewv1.ListDesignStagesRequest{})
			return err
		}(),
		"writing": func() error {
			_, err := s.SetDesignStage(ctx, &quaycrewv1.SetDesignStageRequest{Stage: store.StageDiscovery})
			return err
		}(),
		"approving": func() error {
			_, err := s.ApproveDesignStage(ctx, &quaycrewv1.ApproveDesignStageRequest{Stage: store.StageDiscovery})
			return err
		}(),
	} {
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s with no project answered %v, want InvalidArgument", what, err)
		}
	}
}

// A project nobody can reach is not found, rather than a stage written into nothing.
func TestStagesOfAProjectThatDoesNotExist(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	ctx := context.Background()

	_, err := s.SetDesignStage(ctx, &quaycrewv1.SetDesignStageRequest{
		Project: "nothing", Stage: store.StageDiscovery, Body: "what we asked",
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("writing a stage of a project that does not exist answered %v, want NotFound", err)
	}
}

// A body long enough to be a whole document pasted into one stage is said out loud, and the write
// still keeps it whole. The number is the one the design body is measured against.
func TestALongStageBodyIsKeptAndSaidOutLoud(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	ctx := context.Background()
	_, projectID := newProject(t, s)

	long := strings.Repeat("a", 100_001)
	written, err := s.SetDesignStage(ctx, &quaycrewv1.SetDesignStageRequest{
		Project: projectID, Stage: store.StageDiscovery, Body: long,
	})
	if err != nil {
		t.Fatalf("SetDesignStage with a long body: %v", err)
	}
	if len(written.GetStage().GetBody()) != len(long) {
		t.Fatalf("the body came back %d characters long, want %d", len(written.GetStage().GetBody()), len(long))
	}
	if len(written.GetWarnings()) == 0 {
		t.Error("a body of 100,001 characters was written with nothing said about it")
	}
}

// The driver is what a session calls through, and this is what it may do with the stages. Writing
// and reading are open, for the reason writing a design is: a design session is what writes them.
// The approval is refused, and internal/controlplane/deny_test.go holds that half.
func TestWhatASessionMayDoWithTheStages(t *testing.T) {
	for _, method := range []string{
		quaycrewv1.ControlPlaneService_ListDesignStages_FullMethodName,
		quaycrewv1.ControlPlaneService_SetDesignStage_FullMethodName,
	} {
		if err := controlplane.DeniedToDriver(method, nil); err != nil {
			t.Errorf("%s is refused to a session, and a design session is what writes the stages: %v",
				method, err)
		}
	}
}

// SITE-10. The two stages that describe a structure carry a picture of it.
//
// A data model or an architecture written as prose alone is read a different way by every reader,
// and the operator then approves a paragraph rather than a design. So the write is refused until the
// body holds a fenced block marked mermaid, and the refusal says which stage and what to write.

// The stage the step is named for. The refusal is InvalidArgument rather than FailedPrecondition,
// because the body itself is what is wrong: no later approval makes this same call go through.
func TestAnArchitectureStageWithNoDiagramIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	ctx := context.Background()
	_, projectID := newProject(t, s)
	designedUpTo(t, s, projectID, store.StageArchitecture)

	_, err := s.SetDesignStage(ctx, &quaycrewv1.SetDesignStageRequest{
		Project: projectID, Stage: store.StageArchitecture,
		Body: "the control plane holds the store, and a session talks to the control plane",
	})

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("writing an architecture of prose answered %v, want InvalidArgument", err)
	}
	said := status.Convert(err).Message()
	if !strings.Contains(said, store.StageArchitecture) {
		t.Errorf("the refusal reads %q, and it has to name the stage it is about", said)
	}
	if !strings.Contains(said, "mermaid") {
		t.Errorf("the refusal reads %q, and it has to say what to write", said)
	}
	if !strings.Contains(said, "diagram") {
		t.Errorf("the refusal reads %q, and it has to say a diagram is what is missing", said)
	}

	if held := stagesHeld(t, s, projectID); stageInList(held, store.StageArchitecture) != nil {
		t.Error("the refused write left an architecture stage behind")
	}
}

// The data model carries the same rule, and its refusal names the data model rather than the stage
// the rule was written for.
func TestADataModelStageWithNoDiagramIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	ctx := context.Background()
	_, projectID := newProject(t, s)
	designedUpTo(t, s, projectID, store.StageDataModel)

	_, err := s.SetDesignStage(ctx, &quaycrewv1.SetDesignStageRequest{
		Project: projectID, Stage: store.StageDataModel, Body: "one table for each bill",
	})

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("writing a data model of prose answered %v, want InvalidArgument", err)
	}
	if said := status.Convert(err).Message(); !strings.Contains(said, store.StageDataModel) {
		t.Errorf("the refusal reads %q, and it has to name the stage it is about", said)
	}
	if held := stagesHeld(t, s, projectID); stageInList(held, store.StageDataModel) != nil {
		t.Error("the refused write left a data model stage behind")
	}
}

// The other half. A body with a diagram goes in, and it is kept whole, so the rule adds a condition
// and takes nothing away.
func TestAnArchitectureStageWithADiagramGoesIn(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	ctx := context.Background()
	_, projectID := newProject(t, s)
	designedUpTo(t, s, projectID, store.StageArchitecture)
	body := "the control plane holds the store\n\n```mermaid\nflowchart LR\n  session --> store\n```\n"

	written, err := s.SetDesignStage(ctx, &quaycrewv1.SetDesignStageRequest{
		Project: projectID, Stage: store.StageArchitecture, Body: body,
	})
	if err != nil {
		t.Fatalf("writing an architecture with a diagram: %v", err)
	}
	if written.GetStage().GetBody() != body {
		t.Errorf("the stage reads %q, want the body it was given", written.GetStage().GetBody())
	}
}

// An empty body holds no diagram, so it earns the same refusal rather than a second message about
// the same fault.
func TestAnEmptyArchitectureStageIsRefusedAsHavingNoDiagram(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	ctx := context.Background()
	_, projectID := newProject(t, s)
	designedUpTo(t, s, projectID, store.StageArchitecture)

	_, err := s.SetDesignStage(ctx, &quaycrewv1.SetDesignStageRequest{
		Project: projectID, Stage: store.StageArchitecture, Body: "",
	})

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("writing an empty architecture answered %v, want InvalidArgument", err)
	}
	if said := status.Convert(err).Message(); !strings.Contains(said, "mermaid") {
		t.Errorf("the refusal reads %q, and it has to say what to write", said)
	}
}

// What counts as a diagram, and what does not.
//
// The fence is what a renderer keys on, so this is the same reading a renderer does: the block
// opens with three backticks or three tildes, and the first word after it is mermaid. A body that
// only talks about mermaid holds no picture, and a language whose name starts with mermaid is
// another language.
func TestWhatCountsAsADiagramInAStage(t *testing.T) {
	for _, body := range []struct {
		what   string
		text   string
		counts bool
	}{
		{what: "a fenced block", text: "the shape\n\n```mermaid\nflowchart LR\n  a --> b\n```\n", counts: true},
		{what: "a block fenced with tildes", text: "~~~mermaid\nflowchart LR\n  a --> b\n~~~\n", counts: true},
		{what: "a fence carrying more than the language", text: "```mermaid title=bills\nflowchart LR\n  a --> b\n```\n", counts: true},
		{what: "a fence indented under a list item", text: "- the shape\n\n  ```mermaid\n  flowchart LR\n    a --> b\n  ```\n", counts: true},
		{what: "a diagram after some prose", text: "one table for each bill.\n\nthe rest:\n\n```mermaid\nerDiagram\n  BILL ||--o{ PAYMENT : has\n```", counts: true},
		{what: "prose about a diagram", text: "the mermaid diagram is in the wiki", counts: false},
		{what: "a fenced block of something else", text: "```go\nfunc main() {}\n```\n", counts: false},
		{what: "a language whose name begins with mermaid", text: "```mermaidjs\nflowchart LR\n  a --> b\n```\n", counts: false},
		{what: "a fence that names no language", text: "```\nflowchart LR\n  a --> b\n```\n", counts: false},
	} {
		t.Run(body.what, func(t *testing.T) {
			s := newServer(&model.FakeRunner{})
			_, projectID := newProject(t, s)
			designedUpTo(t, s, projectID, store.StageArchitecture)

			_, err := s.SetDesignStage(context.Background(), &quaycrewv1.SetDesignStageRequest{
				Project: projectID, Stage: store.StageArchitecture, Body: body.text,
			})

			if body.counts && err != nil {
				t.Fatalf("%s was refused: %v", body.what, err)
			}
			if !body.counts && status.Code(err) != codes.InvalidArgument {
				t.Fatalf("%s answered %v, want InvalidArgument", body.what, err)
			}
		})
	}
}

// The rule is about the two stages that describe a structure. A discovery is what somebody was
// asked and told, and a rule over all six would refuse the four stages nobody can draw.
func TestTheOtherFourStagesAreWrittenWithoutADiagram(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	ctx := context.Background()
	_, projectID := newProject(t, s)

	for _, stage := range []string{
		store.StageDiscovery, store.StageStories, store.StageDesignSystem, store.StageMockups,
	} {
		if _, err := s.SetDesignStage(ctx, &quaycrewv1.SetDesignStageRequest{
			Project: projectID, Stage: stage, Body: "the " + stage + " body",
		}); err != nil {
			t.Fatalf("writing %s as prose was refused: %v", stage, err)
		}
		if _, err := s.ApproveDesignStage(ctx, &quaycrewv1.ApproveDesignStageRequest{
			Project: projectID, Stage: stage,
		}); err != nil {
			t.Fatalf("ApproveDesignStage %s: %v", stage, err)
		}
	}
}

// stageBody is the body a test writes for one stage.
//
// The data model and the architecture carry a diagram or they are refused, so a setup that writes
// all six writes a picture into those two. Every other stage is prose.
func stageBody(stage string) string {
	body := "the " + stage + " body"
	if stage == store.StageDataModel || stage == store.StageArchitecture {
		return body + "\n\n```mermaid\nflowchart TD\n  one --> two\n```\n"
	}
	return body
}

// designedUpTo writes and approves every stage before the one named, which is the only state a
// write of that stage is read in.
func designedUpTo(t *testing.T, s *controlplane.Server, projectID, stage string) {
	t.Helper()
	for _, before := range store.DesignStagesBefore(stage) {
		writeStage(t, s, projectID, before)
		approveStage(t, s, projectID, before)
	}
}

// stagesHeld is what the project holds now, read back through the call an operator reads it with.
func stagesHeld(t *testing.T, s *controlplane.Server, projectID string) []*quaycrewv1.DesignStage {
	t.Helper()
	listed, err := s.ListDesignStages(context.Background(), &quaycrewv1.ListDesignStagesRequest{
		Project: projectID,
	})
	if err != nil {
		t.Fatalf("ListDesignStages: %v", err)
	}
	return listed.GetStages()
}

func stageInList(stages []*quaycrewv1.DesignStage, stage string) *quaycrewv1.DesignStage {
	for _, held := range stages {
		if held.GetStage() == stage {
			return held
		}
	}
	return nil
}
