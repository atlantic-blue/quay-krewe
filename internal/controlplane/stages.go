package controlplane

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The six stages a project is designed in, before anything under it is built: discovery, stories,
// design_system, mockups, data_model, architecture.
//
// A project used to start from its design document, one text covering everything at once, so what a
// person sees and what the data looks like were written in the same breath. These calls put an order
// on it. A stage cannot be written while a stage before it carries no approval, so the data model is
// never written before the stories, and a write to a stage takes the approval off every stage after
// it, because those were agreed under a text that has just moved.
//
// The order and the clearing both live in the store, in one transaction, rather than here. A check
// here would read the stages, decide, and then write, and an approval that landed between the two
// would let through exactly the write the rule exists to refuse. What lives here is the vocabulary:
// which six names a person may type, and what a refusal says.

// stageMark is the length past which a stage body is long enough to say so. It is the same number the
// design body is measured against, because a stage is a part of the design and a page that would be
// a whole repository pasted into one is a whole repository pasted into the other.
//
// It refuses nothing. The text is kept whole either way, and the caller is told the length so a
// person decides.
const stageMark = bodyMark

// ListDesignStages returns the stages a project has written, in the order the six are written.
//
// A project that has written none answers with an empty list rather than with six empty stages.
// Nothing written is the normal state, and it is the state every project made before the stages
// existed is in, so a caller can tell a project that is designed in stages from one that is not.
func (s *Server) ListDesignStages(ctx context.Context, req *quaycrewv1.ListDesignStagesRequest) (
	*quaycrewv1.ListDesignStagesResponse, error) {
	if req.GetProject() == "" {
		return nil, status.Error(codes.InvalidArgument, "which project: say where with an address")
	}
	stages, err := s.store.ListDesignStages(ctx, req.GetProject())
	if err != nil {
		return nil, storeError(err, "project")
	}
	return &quaycrewv1.ListDesignStagesResponse{Stages: stages}, nil
}

// SetDesignStage writes one stage's prose and the artifact beside it.
//
// The refusal an unapproved earlier stage earns is the whole point of the call, and it names the
// stage to go and approve rather than only saying no, because an operator told no has to work out
// which of five stages they are missing.
func (s *Server) SetDesignStage(ctx context.Context, req *quaycrewv1.SetDesignStageRequest) (
	*quaycrewv1.SetDesignStageResponse, error) {
	if req.GetProject() == "" {
		return nil, status.Error(codes.InvalidArgument, "which project: say where with an address")
	}
	if err := checkStageName(req.GetStage()); err != nil {
		return nil, err
	}
	if err := checkStageDiagram(req.GetStage(), req.GetBody()); err != nil {
		return nil, err
	}

	// The design system is read the same way, and before the mockups are ever written. Every screen of
	// the project is drawn from this one stage, so a design system no screen could be drawn from is a
	// fault on every screen under it.
	if req.GetStage() == store.StageDesignSystem {
		if err := checkDesignSystemArtifact(req.GetArtifact()); err != nil {
			return nil, err
		}
	}

	// The mockups carry one more rule than the other five: a session has to be able to build the
	// screens from them. It is read before the write, so a mockup nobody could build from never
	// reaches the operator to approve.
	var mockupWarnings []string
	if req.GetStage() == store.StageMockups {
		said, err := s.checkMockupArtifact(ctx, req.GetProject(), req.GetArtifact())
		if err != nil {
			return nil, err
		}
		mockupWarnings = said
	}

	// What still carried the operator's word before the write, so the response can say which words
	// the write took away. Read before rather than after, because after the write they are gone and
	// nothing can say what went. A read that fails says nothing: the write is what matters and a
	// warning nobody could compose is not a reason to refuse it.
	approvedBefore := s.approvedStagesAfter(ctx, req.GetProject(), req.GetStage())

	written, err := s.store.SetDesignStage(ctx, req.GetProject(), store.DesignStageWrite{
		Stage:       req.GetStage(),
		Body:        req.GetBody(),
		Artifact:    req.GetArtifact(),
		ArtifactURL: req.GetArtifactUrl(),
	})
	var notApproved *store.StageNotApprovedError
	if errors.As(err, &notApproved) {
		return nil, status.Errorf(codes.FailedPrecondition,
			"%s comes after %s, and %s carries no approval: approve it first with krewe stage approve [<address>] %s",
			req.GetStage(), notApproved.Stage, notApproved.Stage, notApproved.Stage)
	}
	if errors.Is(err, store.ErrArtifactNotJSON) {
		return nil, status.Errorf(codes.InvalidArgument,
			"the artifact for %s is not json, and the system keeps it as json so a reader can open it",
			req.GetStage())
	}
	if err != nil {
		return nil, storeError(err, "project")
	}

	return &quaycrewv1.SetDesignStageResponse{
		Stage:    written,
		Warnings: append(stageWarnings(req.GetStage(), req.GetBody(), approvedBefore), mockupWarnings...),
	}, nil
}

// ApproveDesignStage records the operator's word on one stage as it stands.
//
// It approves the text that is in the store now, and it asks nothing, for the reason ApproveDesign
// asks nothing: a call that opened an editor would be approving a text nobody named. A stage with no
// body is refused, because there is nothing to agree to.
func (s *Server) ApproveDesignStage(ctx context.Context, req *quaycrewv1.ApproveDesignStageRequest) (
	*quaycrewv1.ApproveDesignStageResponse, error) {
	if req.GetProject() == "" {
		return nil, status.Error(codes.InvalidArgument, "which project: say where with an address")
	}
	if err := checkStageName(req.GetStage()); err != nil {
		return nil, err
	}
	approved, err := s.store.ApproveDesignStage(ctx, req.GetProject(), req.GetStage())
	if errors.Is(err, store.ErrNoStageToApprove) {
		return nil, status.Errorf(codes.FailedPrecondition,
			"the %s stage of this project is empty: write one with krewe stage set [<address>] %s --file <path>",
			req.GetStage(), req.GetStage())
	}
	if err != nil {
		return nil, storeError(err, "project")
	}
	return &quaycrewv1.ApproveDesignStageResponse{Stage: approved}, nil
}

// checkStageName refuses a name outside the six, and names the six in the refusal.
//
// The vocabulary is refused here rather than in the store, the way the two words a step ends with
// are: one layer owns what a person may type. The store refuses an unknown name too, and that is not
// the same check: it cannot work out which position the row sits at, so a row it wrote anyway would
// be a stage in no order at all.
func checkStageName(stage string) error {
	if stage == "" {
		return status.Errorf(codes.InvalidArgument,
			"which stage: one of %s", strings.Join(store.DesignStages(), ", "))
	}
	if _, known := store.DesignStagePosition(stage); !known {
		return status.Errorf(codes.InvalidArgument,
			"%q is not a design stage: the six are %s, in that order",
			stage, strings.Join(store.DesignStages(), ", "))
	}
	return nil
}

// stagesThatCarryADiagram are the two stages whose subject is a structure rather than prose about
// one. Everything else the six hold is written in sentences and is read the same way by everybody.
var stagesThatCarryADiagram = map[string]bool{
	store.StageDataModel:    true,
	store.StageArchitecture: true,
}

// aMermaidFence opens a fenced block whose language is mermaid.
//
// It is the same reading a renderer does, because the fence is what a renderer keys on: three
// backticks or three tildes, then the word mermaid, then either the end of the line or the rest of
// the information string. A fence saying mermaidjs is another language, and prose about mermaid is
// prose. The leading spaces are allowed because a fence inside a list item is indented and is still
// a diagram.
var aMermaidFence = regexp.MustCompile("(?m)^[ \t]*(?:```|~~~)[ \t]*mermaid([ \t][^\n]*)?[ \t]*\r?$")

// checkStageDiagram refuses a data model or an architecture that carries no diagram.
//
// The two stages describe a structure, and a structure written as prose alone is read a different
// way by every reader: the operator then approves a paragraph, and what gets built is whatever the
// session read into it. A picture is one text.
//
// It runs before the store is asked to write, so a refused stage leaves nothing behind. That order
// also decides which refusal an operator gets when a write breaks two rules at once, and the body is
// the one they can fix where they stand: the stage above is somebody else's to approve.
//
// A body with nothing in it holds no diagram either, so it earns this refusal rather than a second
// message about the same missing thing.
func checkStageDiagram(stage, body string) error {
	if !stagesThatCarryADiagram[stage] || aMermaidFence.MatchString(body) {
		return nil
	}
	return status.Errorf(codes.InvalidArgument,
		"the %s stage carries no diagram: write at least one mermaid diagram into it, as a fenced "+
			"block opening ```mermaid, because a structure written as prose alone is read a "+
			"different way by every reader",
		stage)
}

// approvedStagesAfter names the stages after this one that carry the operator's word now, so a write
// can say which words it is about to take away.
//
// A read that fails answers with nothing. It composes a warning and the write is what the caller
// asked for, so a refusal here would cost the operator their write to save them a sentence.
func (s *Server) approvedStagesAfter(ctx context.Context, project, stage string) []string {
	position, known := store.DesignStagePosition(stage)
	if !known {
		return nil
	}
	held, err := s.store.ListDesignStages(ctx, project)
	if err != nil {
		return nil
	}
	after := make([]string, 0, len(held))
	for _, one := range held {
		if one.GetPosition() > position && one.GetApproved() {
			after = append(after, one.GetStage())
		}
	}
	return after
}

// stageWarnings is what a write says beyond having written: the approvals it took away, and a body
// long enough to be worth saying so.
//
// The approvals are named rather than counted. An operator who wrote the stories again has to
// re approve the mockups and the data model, and a warning saying two stages lost their approval
// sends them to the listing to work out which.
func stageWarnings(stage, body string, cleared []string) []string {
	warnings := overMark("the "+stage+" stage", utf8.RuneCountInString(body), stageMark,
		"one stage of the design")
	if len(cleared) > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"writing %s took the approval off %s, because each was agreed under the text that just changed",
			stage, strings.Join(cleared, ", ")))
	}
	return warnings
}
