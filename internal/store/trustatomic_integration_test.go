//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/atlantic-blue/quay-krewe/internal/store"
)

// No reader ever sees a closed step whose counters did not move. That is the invariant behind moving
// the step and the design in one transaction, and a sequential test cannot see it: two writes that
// both land read the same afterwards however they were split.
//
// So this test reads inside the window. A second connection holds the design row, which stops the
// finish at the counters, and the step is read while the finish is stopped there. In one transaction
// the step is still taken, because nothing of the finish has landed. In two the step is already
// closed and its count is not there, which is the reader this invariant forbids.
//
// The memory store does the two under one hold of its lock, so the same window does not exist there.
func TestAClosedStepIsNeverReadableBeforeItsCountersMove(t *testing.T) {
	ctx := context.Background()
	pool, ownURL := databaseOfItsOwn(t, "trustatomic0075")

	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, statement := range []string{
		`insert into workspaces (id, name) values ('w1', 'acme')`,
		`insert into projects (id, workspace, name) values ('p1', 'w1', 'house-bills')`,
		`insert into features (id, project, number, title) values ('f1', 'p1', 1, 'the bills')`,
		`insert into feature_steps (feature, number, title, state, proof_state, proof_scenarios_run,
			proof_ran_at) values ('f1', 1, 'the first', 'taken', 'passing', 1, now())`,
		`insert into project_designs (project, body) values ('p1', 'the design, whole')`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("seed with %q: %v", statement, err)
		}
	}

	opened, err := store.NewPostgres(ctx, ownURL)
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(opened.Close)

	// The design row, held by somebody else. Any write to the counters waits for this to end.
	holding, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin the holding transaction: %v", err)
	}
	defer func() { _ = holding.Rollback(ctx) }()
	var run int32
	if err := holding.QueryRow(ctx,
		`select trust_run from project_designs where project = 'p1' for update`).Scan(&run); err != nil {
		t.Fatalf("hold the design row: %v", err)
	}

	finished := make(chan error, 1)
	go func() {
		_, _, err := opened.FinishStep(context.Background(), "f1", 1, store.Finish{
			State: store.StepDone, Result: "shipped", ClosedBy: "operator",
		})
		finished <- err
	}()

	// While the finish is stopped at the counters, the step may not read as closed. The read goes
	// through the same store the operator reads through, and it never blocks: a row somebody is
	// writing reads as it stood before that write.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-finished:
			t.Fatalf("the finish landed while the design row was held: %v", err)
		default:
		}
		read, err := opened.GetStep(ctx, "f1", 1)
		if err != nil {
			t.Fatalf("GetStep while the finish waits: %v", err)
		}
		if read.GetState() == store.StepDone {
			design, err := opened.GetDesign(ctx, "p1")
			if err != nil {
				t.Fatalf("GetDesign after reading a closed step: %v", err)
			}
			t.Fatalf("the step reads as done while the record counts %d agreements: "+
				"the two did not move together", design.GetTrustAgreements())
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Let go, and the finish lands whole.
	if err := holding.Rollback(ctx); err != nil {
		t.Fatalf("let go of the design row: %v", err)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("FinishStep: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the finish never landed after the design row was let go")
	}
	read, err := opened.GetStep(ctx, "f1", 1)
	if err != nil {
		t.Fatalf("GetStep after the finish: %v", err)
	}
	if read.GetState() != store.StepDone || read.GetOperatorAgreed() != store.AgreedYes {
		t.Fatalf("the step reads %q, agreed %q", read.GetState(), read.GetOperatorAgreed())
	}
	design, err := opened.GetDesign(ctx, "p1")
	if err != nil {
		t.Fatalf("GetDesign after the finish: %v", err)
	}
	if design.GetTrustRun() != 1 || design.GetTrustAgreements() != 1 {
		t.Fatalf("the record reads a run of %d and %d agreements after one finish",
			design.GetTrustRun(), design.GetTrustAgreements())
	}
}
