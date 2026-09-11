package store

import (
	"context"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
)

// A disagreement takes the trust level back down, and level 1 is the only level it can come down
// from. Nothing in this system raises a level yet: the raise is the operator accepting an offer, and
// the offer is a later step of this path, so the shared conformance suite cannot reach a project
// above level 0 at all.
//
// So the level is written here, straight onto the design the memory store holds, and the finish is
// made through the call that closes a step. Postgres is held to the same thing in migration 0075's
// test, which writes the column in one statement and closes a step through the store. Two stores
// that agree on everything a caller can do and disagree on this would ship the disagreement.
func TestADisagreementAtLevelOneLowersTheLevel(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	feature, project := aPathToFinish(t, m)
	standAtLevelOne(t, m, project)

	if _, err := m.RecordProof(ctx, feature, 1, ProofResult{
		State: ProofFailing, ScenariosRun: 1, Output: "1 scenarios (0 passed, 1 failed)"}); err != nil {
		t.Fatalf("RecordProof: %v", err)
	}
	_, design, err := m.FinishStep(ctx, feature, 1, Finish{
		State: StepDone, Result: "the scenario is wrong, not the code", ClosedBy: "operator",
	})
	if err != nil {
		t.Fatalf("FinishStep: %v", err)
	}
	if got := design.GetTrustLevel(); got != TrustLevelChecked {
		t.Errorf("the level reads %d after one disagreement at level 1, want %d",
			got, TrustLevelChecked)
	}
	if got := design.GetTrustRun(); got != 0 {
		t.Errorf("the run reads %d after a disagreement, want 0", got)
	}
	// Read again, because a write that answered well and stored nothing reads the same to its caller
	// and to nobody else.
	read, err := m.GetDesign(ctx, project)
	if err != nil {
		t.Fatalf("GetDesign: %v", err)
	}
	if got := read.GetTrustLevel(); got != TrustLevelChecked {
		t.Errorf("the design reads back level %d, want %d", got, TrustLevelChecked)
	}
}

// A step closed with the word krewe earned is still one agreement or one disagreement. Krewe closes
// nothing yet, so this is the case a caller can make and the conformance suite cannot: the word in
// the closer column changes who spoke, and never what the row records about the verdict.
func TestTheCloserDoesNotChangeWhatTheRowRecords(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	feature, _ := aPathToFinish(t, m)

	if _, err := m.RecordProof(ctx, feature, 1, ProofResult{
		State: ProofPassing, ScenariosRun: 1, Output: "1 scenarios (1 passed)"}); err != nil {
		t.Fatalf("RecordProof: %v", err)
	}
	written, design, err := m.FinishStep(ctx, feature, 1, Finish{
		State: StepDone, Result: "shipped", ClosedBy: "krewe",
	})
	if err != nil {
		t.Fatalf("FinishStep: %v", err)
	}
	if got := written.GetClosedBy(); got != "krewe" {
		t.Errorf("the step says %q closed it, want krewe", got)
	}
	if got := written.GetOperatorAgreed(); got != AgreedYes {
		t.Errorf("the step reads %q, want %q", got, AgreedYes)
	}
	if got := design.GetTrustAgreements(); got != 1 {
		t.Errorf("the record reads %d agreements, want 1", got)
	}
}

// aPathToFinish stands up a workspace, a project, a feature and one step, and hands back the feature
// and the project they hang off.
func aPathToFinish(t *testing.T, m *Memory) (feature, project string) {
	t.Helper()
	ctx := context.Background()
	workspace, err := m.CreateWorkspace(ctx, "acme")
	if err != nil {
		t.Fatalf("create the workspace: %v", err)
	}
	held, err := m.CreateProject(ctx, workspace.GetId(), "house-bills")
	if err != nil {
		t.Fatalf("create the project: %v", err)
	}
	added, err := m.AddFeature(ctx, held.GetId(), "the bills")
	if err != nil {
		t.Fatalf("add the feature: %v", err)
	}
	if _, err := m.SetPath(ctx, added.GetId(), nil, []Step{{Number: 1, Title: "the first"}}); err != nil {
		t.Fatalf("write the path: %v", err)
	}
	return added.GetId(), held.GetId()
}

// standAtLevelOne puts a project where krewe closes a step its own check passed, which is where the
// operator accepting an offer will put it.
func standAtLevelOne(t *testing.T, m *Memory, project string) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.designs == nil {
		m.designs = make(map[string]*quaycrewv1.Design)
	}
	held, ok := m.designs[project]
	if !ok {
		held = bornDesign(project)
		m.designs[project] = held
	}
	held.TrustLevel = TrustLevelCloses
	// Proved here rather than assumed, so a case that never raised anything cannot pass the test
	// below by doing nothing at all.
	if held.GetTrustLevel() != TrustLevelCloses {
		t.Fatal("the design did not take the level the operator would have raised it to")
	}
}
