//go:build integration

package store_test

import (
	"context"
	"os"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/store"
)

// A step carries what krewe's own run of its scenario reported, and migration 0074 adds the four
// columns that hold it.
//
// Every step this system wrote before today was written without them, so the risk is not the schema
// half, which every other test proves by running. It is a step row written before the columns existed
// coming back with an empty proof state, which is a fourth word to everything that reads one, or
// coming back having lost the restatement and the approval beside it.
//
// The down migration is run here as well, because a down migration nobody ran is a down migration
// that does not work. It goes down, the row survives with everything else it holds, and it comes back
// up reading unproven.
func TestAStepWrittenBeforeTheProofColumnsReadsBackWhole(t *testing.T) {
	ctx := context.Background()
	pool, ownURL := databaseOfItsOwn(t, "step0074")

	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`insert into workspaces (id, name) values ('w1', 'acme')`); err != nil {
		t.Fatalf("seed the workspace: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`insert into projects (id, workspace, name) values ('p1', 'w1', 'house-bills')`); err != nil {
		t.Fatalf("seed the project: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`insert into features (id, project, number, title) values ('f1', 'p1', 1, 'the bills')`); err != nil {
		t.Fatalf("seed the feature: %v", err)
	}
	// A step a session was already holding, with a restatement the operator had already approved, when
	// the upgrade ran. That is the case this migration is about: the four defaults have to land on the
	// row without touching what the session wrote or the word spoken over it.
	if _, err := pool.Exec(ctx, `
		insert into feature_steps (feature, number, title, proof_scenario, state, session,
			restatement, restated_at, restatement_approved, restatement_approved_at)
		values ('f1', 1, 'the store holds a brief', 'a project carries a brief', 'taken', 's1',
			'what I understood', now(), true, now())`); err != nil {
		t.Fatalf("seed the step: %v", err)
	}

	// Down, which is the state a system is in the moment before it takes this migration, and the
	// state an operator rolling back is left in.
	// The shipped file itself, read off disk and run the way an operator runs it. A copy of its
	// statements written out here would prove a down migration nobody ships.
	down, err := os.ReadFile("migrations/0074_a_step_carries_a_proof_result.down.sql")
	if err != nil {
		t.Fatalf("read the down migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("take the proof columns away: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`delete from schema_migrations where version = $1`,
		"0074_a_step_carries_a_proof_result"); err != nil {
		t.Fatalf("forget the migration: %v", err)
	}
	for _, column := range []string{"proof_state", "proof_scenarios_run", "proof_output", "proof_ran_at"} {
		var there bool
		if err := pool.QueryRow(ctx, `
			select exists (
				select 1 from information_schema.columns
				where table_name = 'feature_steps' and column_name = $1)`, column).Scan(&there); err != nil {
			t.Fatalf("read the schema for %s: %v", column, err)
		}
		if there {
			t.Fatalf("the down migration left %s on the table", column)
		}
	}
	// The rest of the row is untouched by the drop. A verdict says what a run reported and never what
	// the step is, so an operator who rolls back keeps the step, its session and its approval.
	var state, restatement string
	var approved bool
	if err := pool.QueryRow(ctx,
		`select state, restatement, restatement_approved from feature_steps
		 where feature = 'f1' and number = 1`).Scan(&state, &restatement, &approved); err != nil {
		t.Fatalf("read the step after the drop: %v", err)
	}
	if state != "taken" || restatement != "what I understood" || !approved {
		t.Fatalf("after the drop the step reads %q, restated %q, approved %v", state, restatement, approved)
	}

	// And up again, over the row that was written without the columns.
	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate over the old shape: %v", err)
	}

	opened, err := store.NewPostgres(ctx, ownURL)
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(opened.Close)
	held, err := opened.GetStep(ctx, "f1", 1)
	if err != nil {
		t.Fatalf("read the step back: %v", err)
	}
	// Unproven, which is the truth: nobody ever ran this step's scenario. A row that read back with an
	// empty state would be a fourth word to everything that reads one.
	if got := held.GetProofState(); got != store.ProofUnproven {
		t.Errorf("the step reads %q, want %q", got, store.ProofUnproven)
	}
	if got := held.GetProofScenariosRun(); got != 0 {
		t.Errorf("it reads %d scenarios run, and nothing ran", got)
	}
	if held.GetProofOutput() != "" || held.GetProofRanAt() != nil {
		t.Errorf("it reads back output %q at %v, and nothing ran",
			held.GetProofOutput(), held.GetProofRanAt())
	}
	if held.GetState() != "taken" || !held.GetRestatementApproved() {
		t.Errorf("the step reads %q, approved %v", held.GetState(), held.GetRestatementApproved())
	}

	// The columns the migration added take a verdict, and the call that records one writes it.
	written, err := opened.RecordProof(ctx, "f1", 1, store.ProofResult{
		State: store.ProofPassing, ScenariosRun: 2, Output: "2 scenarios (2 passed)"})
	if err != nil {
		t.Fatalf("RecordProof after the migration: %v", err)
	}
	if got := written.GetProofState(); got != store.ProofPassing {
		t.Fatalf("the step reads %q after a passing run", got)
	}
	if got := written.GetProofScenariosRun(); got != 2 {
		t.Fatalf("it reads %d scenarios run, want 2", got)
	}
	if written.GetProofRanAt() == nil {
		t.Error("the run left no moment on the step")
	}
	// The restatement and the word spoken over it are a different record, and a run moves neither.
	if !written.GetRestatementApproved() {
		t.Error("recording a run cleared the operator's word on the restatement")
	}
}
