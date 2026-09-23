//go:build integration

package store_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/store"
)

// A step records the run that was seen to fail, and migration 0078 adds the two columns that hold it.
//
// Every step this system wrote before today was written without them, and some of those steps are in
// flight right now. So the risk is a step a session is holding coming back from the upgrade unable to
// finish at all, or coming back having lost the verdict and the approval beside it.
//
// A step in flight that already saw its tests fail is checked once more after the upgrade, because
// the record starts empty. That is one command and it spends no model tokens.
//
// The down migration runs here as well, because a down migration nobody ran is a down migration that
// does not work. It goes down, the row survives with everything else it holds, and it comes back up
// reading no red run.
func TestAStepWrittenBeforeTheRedRunColumnsReadsBackWhole(t *testing.T) {
	ctx := context.Background()
	pool, ownURL := databaseOfItsOwn(t, "step0078")

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
	// A step a session is holding, whose restatement the operator approved and whose scenario krewe
	// already ran. That is the case this migration is about: the two defaults land on the row without
	// touching the verdict or the word spoken over the restatement.
	if _, err := pool.Exec(ctx, `
		insert into feature_steps (feature, number, title, proof_scenario, state, session,
			restatement, restated_at, restatement_approved, restatement_approved_at,
			proof_state, proof_scenarios_run, proof_output, proof_ran_at)
		values ('f1', 1, 'the store holds a brief', 'a project carries a brief', 'taken', 's1',
			'what I understood', now(), true, now(),
			'passing', 1, '1 scenarios (1 passed)', now())`); err != nil {
		t.Fatalf("seed the step: %v", err)
	}

	// Down, which is the state a system is in the moment before it takes this migration, and the
	// state an operator rolling back is left in. The shipped file itself, read off disk and run the
	// way an operator runs it: a copy of its statements written out here would prove a down migration
	// nobody ships.
	down, err := os.ReadFile("migrations/0078_a_step_records_its_red_run.down.sql")
	if err != nil {
		t.Fatalf("read the down migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("take the red run columns away: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`delete from schema_migrations where version = $1`,
		"0078_a_step_records_its_red_run"); err != nil {
		t.Fatalf("forget the migration: %v", err)
	}
	for _, column := range []string{"red_run_scenarios", "red_run_at"} {
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
	// The rest of the row is untouched by the drop. The red run says what one run reported and never
	// what the step is, so an operator who rolls back keeps the step, its session and its verdict.
	var state, proofState, restatement string
	if err := pool.QueryRow(ctx,
		`select state, proof_state, restatement from feature_steps
		 where feature = 'f1' and number = 1`).Scan(&state, &proofState, &restatement); err != nil {
		t.Fatalf("read the step after the drop: %v", err)
	}
	if state != "taken" || proofState != "passing" || restatement != "what I understood" {
		t.Fatalf("after the drop the step reads %q, proved %q, restated %q", state, proofState, restatement)
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
	if held.GetRedRunAt() != nil || held.GetRedRunScenarios() != 0 {
		t.Errorf("the step reads a red run at %v over %d scenarios, and none is recorded",
			held.GetRedRunAt(), held.GetRedRunScenarios())
	}
	if held.GetProofState() != store.ProofPassing || !held.GetRestatementApproved() {
		t.Errorf("the step reads %q, approved %v", held.GetProofState(), held.GetRestatementApproved())
	}

	// The step is in flight and nobody saw its tests fail, so the upgrade leaves it one command from
	// finishing rather than closed on a record that is not there.
	if _, _, err := opened.FinishStep(ctx, "f1", 1, store.Finish{
		State: store.StepDone, Result: "shipped", ClosedBy: "operator",
	}); !errors.Is(err, store.ErrNoRedRun) {
		t.Fatalf("finishing the migrated step answered %v, want ErrNoRedRun", err)
	}

	// The columns the migration added take a run, and the call that records one writes it.
	written, err := opened.RecordProof(ctx, "f1", 1, store.ProofResult{
		State: store.ProofFailing, ScenariosRun: 2, Output: "2 scenarios (0 passed, 2 failed)"})
	if err != nil {
		t.Fatalf("RecordProof after the migration: %v", err)
	}
	if written.GetRedRunAt() == nil {
		t.Fatal("a run that failed with two scenarios in it left no red run on the step")
	}
	if got := written.GetRedRunScenarios(); got != 2 {
		t.Fatalf("the red run reads %d scenarios, want the 2 the run reported", got)
	}
	if _, err := opened.RecordProof(ctx, "f1", 1, store.ProofResult{
		State: store.ProofPassing, ScenariosRun: 2, Output: "2 scenarios (2 passed)"}); err != nil {
		t.Fatalf("RecordProof on the run that passed: %v", err)
	}
	closed, _, err := opened.FinishStep(ctx, "f1", 1, store.Finish{
		State: store.StepDone, Result: "shipped", ClosedBy: "operator",
	})
	if err != nil {
		t.Fatalf("finishing the migrated step after a red run and a green one: %v", err)
	}
	if closed.GetState() != store.StepDone {
		t.Fatalf("the step came back in state %q, want %q", closed.GetState(), store.StepDone)
	}
}
