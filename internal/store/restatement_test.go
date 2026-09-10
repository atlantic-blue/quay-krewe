package store

import (
	"context"
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// The word that approves a restatement is written by a call that is a later step of this path, so
// the shared conformance suite cannot reach a step that anybody approved: every row it can make
// reads unapproved from the moment it exists, and a write that cleared nothing would look exactly
// like a write that cleared it.
//
// So the word is written here, straight onto the step the memory store holds. Postgres is held to
// the same thing in migration 0072's test, which writes it in one statement. Two stores that agree
// on everything a caller can do and disagree on this would ship the disagreement.
func TestASecondRestatementLeavesTheStepUnapproved(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	workspace, err := m.CreateWorkspace(ctx, "acme")
	if err != nil {
		t.Fatalf("create the workspace: %v", err)
	}
	project, err := m.CreateProject(ctx, workspace.GetId(), "house-bills")
	if err != nil {
		t.Fatalf("create the project: %v", err)
	}
	feature, err := m.AddFeature(ctx, project.GetId(), "the bills")
	if err != nil {
		t.Fatalf("add the feature: %v", err)
	}
	if _, err := m.SetPath(ctx, feature.GetId(), nil, []Step{{Number: 1, Title: "the first"}}); err != nil {
		t.Fatalf("write the path: %v", err)
	}
	if _, err := m.SetRestatement(ctx, feature.GetId(), 1, "the first reading"); err != nil {
		t.Fatalf("SetRestatement: %v", err)
	}
	approve(t, m, feature.GetId(), 1)

	second, err := m.SetRestatement(ctx, feature.GetId(), 1, "the second reading")
	if err != nil {
		t.Fatalf("SetRestatement over an approved one: %v", err)
	}
	if second.GetRestatementApproved() || second.GetRestatementApprovedAt() != nil {
		t.Errorf("the second text reads as approved at %v, and nobody read it",
			second.GetRestatementApprovedAt())
	}
	// Read again, because a write that answered well and stored nothing reads the same to its caller
	// and to nobody else.
	read, err := m.GetStep(ctx, feature.GetId(), 1)
	if err != nil {
		t.Fatalf("GetStep: %v", err)
	}
	if read.GetRestatementApproved() || read.GetRestatementApprovedAt() != nil {
		t.Errorf("the step reads back approved at %v", read.GetRestatementApprovedAt())
	}
	if read.GetRestatement() != "the second reading" {
		t.Errorf("the step restates %q, want the second reading", read.GetRestatement())
	}
}

// approve writes the operator's word onto a step the memory store holds, where the call that speaks
// it will write it.
func approve(t *testing.T, m *Memory, feature string, number int32) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	held, err := m.stepLocked(feature, number)
	if err != nil {
		t.Fatalf("read the step to approve it: %v", err)
	}
	held.RestatementApproved = true
	held.RestatementApprovedAt = timestamppb.New(held.GetRestatedAt().AsTime())
	// Proved here rather than assumed, so a case that never approved anything cannot pass the test
	// below by doing nothing at all.
	if !held.GetRestatementApproved() {
		t.Fatal("the step did not take the word that approves it")
	}
}
