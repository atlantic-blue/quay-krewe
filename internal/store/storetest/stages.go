package storetest

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/store"
)

// The six stages a project is designed in, held against both implementations.
//
// The two rules this contract exists for are the order and the clearing. A memory store that took a
// write the real one refuses would let a project write its data model first, pass every scenario, and
// fail the moment the same call reached Postgres.

func runDesignStageConformance(t *testing.T, newDataset func(t *testing.T) Opener) {
	t.Helper()

	// Nothing written is the normal state, and it is the state every project made before the stages
	// existed is in. An error here would make every such project look broken.
	t.Run("a project that wrote no stage answers with nothing, and it is not an error", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		project := newProject(t, s, "acme", "house-bills")

		stages, err := s.ListDesignStages(ctx, project.GetId())
		if err != nil {
			t.Fatalf("ListDesignStages: %v", err)
		}
		if len(stages) != 0 {
			t.Fatalf("a project nobody staged holds %d stages: %v", len(stages), named(stages))
		}
	})

	t.Run("the stages of a project that does not exist are not found", func(t *testing.T) {
		s := newDataset(t)(t)
		if _, err := s.ListDesignStages(context.Background(), "nothing"); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("ListDesignStages on a project that does not exist: %v", err)
		}
	})

	t.Run("a stage is written and read back whole, at its own position", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		project := newProject(t, s, "acme", "house-bills")

		written, err := s.SetDesignStage(ctx, project.GetId(), store.DesignStageWrite{
			Stage:       store.StageDiscovery,
			Body:        "# What we asked\n\nThey pay four bills, and two of them move.\n",
			Artifact:    `{"asked":["when does it move"]}`,
			ArtifactURL: "https://example.invalid/discovery",
		})
		if err != nil {
			t.Fatalf("SetDesignStage: %v", err)
		}
		if written.GetId() == "" {
			t.Error("the stage came back with no identifier")
		}
		if written.GetProject() != project.GetId() {
			t.Errorf("the stage names project %q, want %q", written.GetProject(), project.GetId())
		}
		if written.GetStage() != store.StageDiscovery || written.GetPosition() != 0 {
			t.Errorf("discovery came back as %q at position %d", written.GetStage(), written.GetPosition())
		}
		if !strings.Contains(written.GetBody(), "two of them move") {
			t.Errorf("the body did not survive the round trip: %q", written.GetBody())
		}
		if written.GetArtifactUrl() != "https://example.invalid/discovery" {
			t.Errorf("the artifact address did not survive: %q", written.GetArtifactUrl())
		}
		// What it means, never how it is spelled. Postgres holds a jsonb document in its own form, so
		// the spaces move and the keys of an object sort, and a comparison of the bytes would pass in
		// memory and fail against a database.
		sameArtifact(t, written.GetArtifact(), `{"asked":["when does it move"]}`)
		if written.GetVersion() != 1 {
			t.Errorf("a first write reads version %d, want 1", written.GetVersion())
		}
		if written.GetApproved() || written.GetApprovedVersion() != 0 {
			t.Errorf("a stage nobody read came back approved at version %d", written.GetApprovedVersion())
		}
		if written.GetCreatedAt() == nil || written.GetUpdatedAt() == nil {
			t.Error("the stage came back with no stamps, so nothing says when it was written")
		}

		read, err := s.ListDesignStages(ctx, project.GetId())
		if err != nil {
			t.Fatalf("ListDesignStages: %v", err)
		}
		if len(read) != 1 || read[0].GetStage() != store.StageDiscovery {
			t.Fatalf("the listing holds %v, want discovery alone", named(read))
		}
		if read[0].GetBody() != written.GetBody() {
			t.Errorf("the listing reads body %q, and the write answered %q", read[0].GetBody(), written.GetBody())
		}
	})

	// STAGE-1. The whole reason the stages exist: the data model cannot be written before the stories
	// are agreed, so nothing is built from a shape nobody has seen.
	t.Run("a stage written before the stage above it is approved is refused, naming it", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		project := newProject(t, s, "acme", "house-bills")

		_, err := s.SetDesignStage(ctx, project.GetId(), store.DesignStageWrite{
			Stage: store.StageStories, Body: "as a person paying bills, I want to see what is due",
		})
		var blocked *store.StageNotApprovedError
		if !errors.As(err, &blocked) {
			t.Fatalf("writing the stories before discovery: %v", err)
		}
		if blocked.Stage != store.StageDiscovery {
			t.Errorf("the refusal names %q, want discovery", blocked.Stage)
		}
		if blocked.Writing != store.StageStories {
			t.Errorf("the refusal says the write was %q, want the stories", blocked.Writing)
		}
		if !errors.Is(err, store.ErrStageNotApproved) {
			t.Error("a caller asking only which rule refused the write cannot tell")
		}
		if !strings.Contains(err.Error(), store.StageDiscovery) {
			t.Errorf("the refusal reads %q, and it has to name the stage to go and approve", err)
		}

		stages, err := s.ListDesignStages(ctx, project.GetId())
		if err != nil {
			t.Fatalf("ListDesignStages: %v", err)
		}
		if len(stages) != 0 {
			t.Fatalf("a refused write left %v behind", named(stages))
		}
	})

	// A stage written and not approved is no better than a stage nobody wrote, which is the half of
	// the rule that a check for the row alone would miss.
	t.Run("a stage written above and never approved refuses the one below it", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		project := newProject(t, s, "acme", "house-bills")

		if _, err := s.SetDesignStage(ctx, project.GetId(), store.DesignStageWrite{
			Stage: store.StageDiscovery, Body: "what we asked",
		}); err != nil {
			t.Fatalf("SetDesignStage discovery: %v", err)
		}
		_, err := s.SetDesignStage(ctx, project.GetId(), store.DesignStageWrite{
			Stage: store.StageStories, Body: "the stories",
		})
		var blocked *store.StageNotApprovedError
		if !errors.As(err, &blocked) || blocked.Stage != store.StageDiscovery {
			t.Fatalf("the stories went in over an unapproved discovery: %v", err)
		}
	})

	// The refusal names the first stage without a word on it rather than the nearest one, because the
	// operator's next move is at the top of the list.
	t.Run("the refusal names the first stage without approval, not the nearest", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		project := newProject(t, s, "acme", "house-bills")

		approveThrough(t, s, project.GetId(), store.StageDiscovery)
		if _, err := s.SetDesignStage(ctx, project.GetId(), store.DesignStageWrite{
			Stage: store.StageStories, Body: "the stories",
		}); err != nil {
			t.Fatalf("SetDesignStage stories: %v", err)
		}
		_, err := s.SetDesignStage(ctx, project.GetId(), store.DesignStageWrite{
			Stage: store.StageDesignSystem, Body: "the colours",
		})
		var blocked *store.StageNotApprovedError
		if !errors.As(err, &blocked) || blocked.Stage != store.StageStories {
			t.Fatalf("the refusal named %v, want the stories, which are the first without a word", err)
		}
	})

	// The whole order walked once, which is the shape an operator actually works in.
	t.Run("the six go in one after another, each once the one above is approved", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		project := newProject(t, s, "acme", "house-bills")

		for _, stage := range store.DesignStages() {
			approveThrough(t, s, project.GetId(), stage)
		}
		stages, err := s.ListDesignStages(ctx, project.GetId())
		if err != nil {
			t.Fatalf("ListDesignStages: %v", err)
		}
		if got := named(stages); strings.Join(got, ",") != strings.Join(store.DesignStages(), ",") {
			t.Fatalf("the listing reads %v, want the six in order", got)
		}
		for at, stage := range stages {
			if stage.GetPosition() != int32(at) {
				t.Errorf("%s sits at position %d, want %d", stage.GetStage(), stage.GetPosition(), at)
			}
			if !stage.GetApproved() {
				t.Errorf("%s came back without the word that was given to it", stage.GetStage())
			}
		}
	})

	// STAGE-2, the near half: the text changed, so the word on that text is gone.
	t.Run("writing a stage clears its own approval and raises its version", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		project := newProject(t, s, "acme", "house-bills")

		approved := approveThrough(t, s, project.GetId(), store.StageDiscovery)
		if !approved.GetApproved() || approved.GetApprovedAt() == nil {
			t.Fatalf("the approval answered approved=%t at %v", approved.GetApproved(), approved.GetApprovedAt())
		}
		if approved.GetApprovedVersion() != approved.GetVersion() {
			t.Fatalf("the approval reads version %d against %d", approved.GetApprovedVersion(), approved.GetVersion())
		}

		again, err := s.SetDesignStage(ctx, project.GetId(), store.DesignStageWrite{
			Stage: store.StageDiscovery, Body: "what we asked, the second time",
		})
		if err != nil {
			t.Fatalf("SetDesignStage over an approved stage: %v", err)
		}
		if again.GetApproved() || again.GetApprovedAt() != nil || again.GetApprovedVersion() != 0 {
			t.Fatalf("a rewritten stage reads approved=%t at %v", again.GetApproved(), again.GetApprovedAt())
		}
		if again.GetVersion() != approved.GetVersion()+1 {
			t.Errorf("a rewritten stage reads version %d, want %d", again.GetVersion(), approved.GetVersion()+1)
		}
		if again.GetId() != approved.GetId() {
			t.Errorf("writing a stage again made a second row: %s then %s", approved.GetId(), again.GetId())
		}
	})

	// STAGE-2, the far half: the stages after it were agreed under the text that just moved, so the
	// word on each of them is gone too. The words themselves stay.
	t.Run("writing a stage clears the approval of every stage after it", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		project := newProject(t, s, "acme", "house-bills")

		for _, stage := range store.DesignStages() {
			approveThrough(t, s, project.GetId(), stage)
		}
		if _, err := s.SetDesignStage(ctx, project.GetId(), store.DesignStageWrite{
			Stage: store.StageDesignSystem, Body: "the colours, rethought",
		}); err != nil {
			t.Fatalf("SetDesignStage design_system: %v", err)
		}

		stages, err := s.ListDesignStages(ctx, project.GetId())
		if err != nil {
			t.Fatalf("ListDesignStages: %v", err)
		}
		for _, stage := range stages {
			switch {
			case stage.GetPosition() < 2:
				if !stage.GetApproved() {
					t.Errorf("%s sits above the write and lost its word", stage.GetStage())
				}
			default:
				if stage.GetApproved() || stage.GetApprovedAt() != nil {
					t.Errorf("%s sits at or after the write and kept its word", stage.GetStage())
				}
				if stage.GetBody() == "" {
					t.Errorf("%s lost its body, and only the word on it was supposed to go", stage.GetStage())
				}
			}
		}
	})

	// The version is what holds an approval to one text, so a stage rewritten twice and approved once
	// has to read the number it was approved at rather than the one it holds.
	t.Run("an approval is about the version it was given to", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		project := newProject(t, s, "acme", "house-bills")

		approveThrough(t, s, project.GetId(), store.StageDiscovery)
		if _, err := s.SetDesignStage(ctx, project.GetId(), store.DesignStageWrite{
			Stage: store.StageDiscovery, Body: "the third body",
		}); err != nil {
			t.Fatalf("SetDesignStage again: %v", err)
		}
		approved, err := s.ApproveDesignStage(ctx, project.GetId(), store.StageDiscovery)
		if err != nil {
			t.Fatalf("ApproveDesignStage: %v", err)
		}
		if approved.GetVersion() != 2 || approved.GetApprovedVersion() != 2 || !approved.GetApproved() {
			t.Fatalf("the second approval reads version %d approved at %d",
				approved.GetVersion(), approved.GetApprovedVersion())
		}
	})

	// Writing the same text again still clears the word, for the reason SetRestatement does: the
	// store cannot tell an unchanged text from a rewritten one that reads the same.
	t.Run("writing the same body again still clears the approval", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		project := newProject(t, s, "acme", "house-bills")

		approveThrough(t, s, project.GetId(), store.StageDiscovery)
		again, err := s.SetDesignStage(ctx, project.GetId(), store.DesignStageWrite{
			Stage: store.StageDiscovery, Body: "the " + store.StageDiscovery + " body",
		})
		if err != nil {
			t.Fatalf("SetDesignStage with the same body: %v", err)
		}
		if again.GetApproved() {
			t.Error("the same text written again kept the word, and the store cannot tell it is the same text")
		}
	})

	// An empty artifact is no artifact rather than an empty document, which is how a stage takes one
	// away.
	t.Run("a stage written without an artifact carries none", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		project := newProject(t, s, "acme", "house-bills")

		if _, err := s.SetDesignStage(ctx, project.GetId(), store.DesignStageWrite{
			Stage: store.StageDiscovery, Body: "what we asked", Artifact: `{"asked":[]}`,
		}); err != nil {
			t.Fatalf("SetDesignStage with an artifact: %v", err)
		}
		without, err := s.SetDesignStage(ctx, project.GetId(), store.DesignStageWrite{
			Stage: store.StageDiscovery, Body: "what we asked",
		})
		if err != nil {
			t.Fatalf("SetDesignStage without an artifact: %v", err)
		}
		if without.GetArtifact() != "" {
			t.Errorf("the stage kept the artifact %q after a write that carried none", without.GetArtifact())
		}
		if without.GetArtifactUrl() != "" {
			t.Errorf("the stage kept the artifact address %q", without.GetArtifactUrl())
		}
	})

	// The Postgres column is jsonb and would refuse this. A memory store that took it would make a
	// suite green over a write that fails the moment it reaches a database.
	t.Run("an artifact that is not json is refused by both stores", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		project := newProject(t, s, "acme", "house-bills")

		_, err := s.SetDesignStage(ctx, project.GetId(), store.DesignStageWrite{
			Stage: store.StageDiscovery, Body: "what we asked", Artifact: "the flows are over there",
		})
		if !errors.Is(err, store.ErrArtifactNotJSON) {
			t.Fatalf("an artifact that is not json: %v", err)
		}
		stages, err := s.ListDesignStages(ctx, project.GetId())
		if err != nil {
			t.Fatalf("ListDesignStages: %v", err)
		}
		if len(stages) != 0 {
			t.Fatalf("the refused write left %v behind", named(stages))
		}
	})

	t.Run("a stage outside the six is refused by both stores", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		project := newProject(t, s, "acme", "house-bills")

		if _, err := s.SetDesignStage(ctx, project.GetId(), store.DesignStageWrite{
			Stage: "wireframes", Body: "the wireframes",
		}); !errors.Is(err, store.ErrUnknownDesignStage) {
			t.Fatalf("writing a stage outside the six: %v", err)
		}
		if _, err := s.ApproveDesignStage(ctx, project.GetId(), "wireframes"); !errors.Is(err, store.ErrUnknownDesignStage) {
			t.Fatalf("approving a stage outside the six: %v", err)
		}
	})

	t.Run("approving a stage nobody wrote is refused", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		project := newProject(t, s, "acme", "house-bills")

		if _, err := s.ApproveDesignStage(ctx, project.GetId(), store.StageDiscovery); !errors.Is(err, store.ErrNoStageToApprove) {
			t.Fatalf("approving a stage nobody wrote: %v", err)
		}
		if _, err := s.SetDesignStage(ctx, project.GetId(), store.DesignStageWrite{
			Stage: store.StageDiscovery,
		}); err != nil {
			t.Fatalf("SetDesignStage with an empty body: %v", err)
		}
		if _, err := s.ApproveDesignStage(ctx, project.GetId(), store.StageDiscovery); !errors.Is(err, store.ErrNoStageToApprove) {
			t.Fatalf("approving a stage that says nothing: %v", err)
		}
	})

	t.Run("approving an approved stage moves the moment", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		project := newProject(t, s, "acme", "house-bills")

		first := approveThrough(t, s, project.GetId(), store.StageDiscovery)
		second, err := s.ApproveDesignStage(ctx, project.GetId(), store.StageDiscovery)
		if err != nil {
			t.Fatalf("ApproveDesignStage again: %v", err)
		}
		if second.GetApprovedAt().AsTime().Before(first.GetApprovedAt().AsTime()) {
			t.Errorf("the second word is stamped %v, before the first at %v",
				second.GetApprovedAt().AsTime(), first.GetApprovedAt().AsTime())
		}
		if second.GetVersion() != first.GetVersion() {
			t.Errorf("approving again moved the version from %d to %d", first.GetVersion(), second.GetVersion())
		}
	})

	t.Run("writing or approving a stage of a project that does not exist is not found", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()

		if _, err := s.SetDesignStage(ctx, "nothing", store.DesignStageWrite{
			Stage: store.StageDiscovery, Body: "what we asked",
		}); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("SetDesignStage on a project that does not exist: %v", err)
		}
		if _, err := s.ApproveDesignStage(ctx, "nothing", store.StageDiscovery); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("ApproveDesignStage on a project that does not exist: %v", err)
		}
	})

	// One project's stages are not another's, and a project designed in stages does not put the
	// project beside it under the same rule.
	t.Run("one project's stages are not another's", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		staged := newProject(t, s, "acme", "house-bills")
		other, err := s.CreateProject(ctx, staged.GetWorkspace(), "car-insurance")
		if err != nil {
			t.Fatalf("CreateProject: %v", err)
		}

		approveThrough(t, s, staged.GetId(), store.StageDiscovery)
		stages, err := s.ListDesignStages(ctx, other.GetId())
		if err != nil {
			t.Fatalf("ListDesignStages: %v", err)
		}
		if len(stages) != 0 {
			t.Fatalf("the project beside it holds %v", named(stages))
		}
		// And the rule reads that project's own rows, so the stage before this one being approved
		// next door refuses nothing and grants nothing.
		_, err = s.SetDesignStage(ctx, other.GetId(), store.DesignStageWrite{
			Stage: store.StageStories, Body: "the stories",
		})
		var blocked *store.StageNotApprovedError
		if !errors.As(err, &blocked) || blocked.Stage != store.StageDiscovery {
			t.Fatalf("the project beside it wrote its stories off another project's approval: %v", err)
		}
	})

	t.Run("deleting a project takes its stages with it", func(t *testing.T) {
		s := newDataset(t)(t)
		ctx := context.Background()
		project := newProject(t, s, "acme", "house-bills")

		approveThrough(t, s, project.GetId(), store.StageDiscovery)
		if err := s.DeleteProject(ctx, project.GetId()); err != nil {
			t.Fatalf("DeleteProject: %v", err)
		}
		if _, err := s.ListDesignStages(ctx, project.GetId()); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("the stages of a deleted project came back: %v", err)
		}
	})
}

// approveThrough writes a stage and approves it, which is how a test reaches the stage below it. It
// answers with the stage as the approval left it.
func approveThrough(t *testing.T, s store.Store, project, stage string) *quaycrewv1.DesignStage {
	t.Helper()
	ctx := context.Background()
	if _, err := s.SetDesignStage(ctx, project, store.DesignStageWrite{
		Stage: stage, Body: "the " + stage + " body",
	}); err != nil {
		t.Fatalf("SetDesignStage %s: %v", stage, err)
	}
	approved, err := s.ApproveDesignStage(ctx, project, stage)
	if err != nil {
		t.Fatalf("ApproveDesignStage %s: %v", stage, err)
	}
	return approved
}

// named is the stages in a listing, so a failure says which came back rather than how many.
func named(stages []*quaycrewv1.DesignStage) []string {
	out := make([]string, 0, len(stages))
	for _, stage := range stages {
		out = append(out, stage.GetStage())
	}
	return out
}

// sameArtifact says whether two json documents mean the same thing, which is the only comparison a
// caller of a stage may make. Postgres holds a jsonb document in its own form: the spaces move and
// the keys of an object sort. The memory store keeps the string it was given, so a test that compared
// the bytes would pass against one store and fail against the other, and the drift would read as a
// bug in whichever one was run second.
func sameArtifact(t *testing.T, got, want string) {
	t.Helper()
	var read, wanted any
	if err := json.Unmarshal([]byte(got), &read); err != nil {
		t.Fatalf("the artifact came back as %q, which is not json: %v", got, err)
	}
	if err := json.Unmarshal([]byte(want), &wanted); err != nil {
		t.Fatalf("the wanted artifact %q is not json: %v", want, err)
	}
	if !reflect.DeepEqual(read, wanted) {
		t.Errorf("the artifact means %v, want %v", read, wanted)
	}
}
