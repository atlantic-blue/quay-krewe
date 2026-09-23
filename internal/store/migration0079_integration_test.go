//go:build integration

package store_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/store"
)

// A step says whether the red run rule bound it, and migration 0079 adds the column that says so.
//
// The rule arrived after work had started. A step a session was holding then has its code written
// and its tests passing, so no run of it can go red any more, and the rule as it shipped left that
// step unable to finish at all. The column reads false on every row that was already there, so the
// work in flight closes, and a take from now on writes true.
//
// The down migration runs here as well, because a down migration nobody ran is a down migration that
// does not work. It goes down, the row survives with everything else it holds, and it comes back up
// reading a step the rule never bound.
func TestAStepWrittenBeforeTheRedRunRuleFinishesWithoutOne(t *testing.T) {
	ctx := context.Background()
	pool, ownURL := databaseOfItsOwn(t, "step0079")

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
	// Two steps. The first is the one this migration is about: a session is holding it, its
	// restatement is approved, its scenario ran and it passed, and no run of it was ever seen to
	// fail. The second is the one the same session takes next, under the rule.
	if _, err := pool.Exec(ctx, `
		insert into feature_steps (feature, number, title, proof_scenario, state, session,
			restatement, restated_at, restatement_approved, restatement_approved_at,
			proof_state, proof_scenarios_run, proof_output, proof_ran_at)
		values ('f1', 1, 'the store holds a brief', 'a project carries a brief', 'taken', 's1',
			'what I understood', now(), true, now(),
			'passing', 1, '1 scenarios (1 passed)', now())`); err != nil {
		t.Fatalf("seed the step in flight: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		insert into feature_steps (feature, number, title, proof_scenario, state)
		values ('f1', 2, 'the store holds a design', 'a project carries a design', 'ready')`); err != nil {
		t.Fatalf("seed the step nobody took: %v", err)
	}

	// Down, which is the state a system is in the moment before it takes this migration, and the
	// state an operator rolling back is left in. The shipped file itself, read off disk and run the
	// way an operator runs it: a copy of its statements written out here would prove a down migration
	// nobody ships.
	down, err := os.ReadFile("migrations/0079_a_step_says_whether_it_needs_a_red_run.down.sql")
	if err != nil {
		t.Fatalf("read the down migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("take the requirement away: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`delete from schema_migrations where version = $1`,
		"0079_a_step_says_whether_it_needs_a_red_run"); err != nil {
		t.Fatalf("forget the migration: %v", err)
	}
	var there bool
	if err := pool.QueryRow(ctx, `
		select exists (
			select 1 from information_schema.columns
			where table_name = 'feature_steps' and column_name = 'red_run_required')`).Scan(&there); err != nil {
		t.Fatalf("read the schema: %v", err)
	}
	if there {
		t.Fatal("the down migration left red_run_required on the table")
	}
	// The rest of the row is untouched by the drop. The column says which rule a step was started
	// under and never what the step is, so an operator who rolls back keeps the step, its session and
	// its verdict.
	var state, proofState, restatement string
	if err := pool.QueryRow(ctx,
		`select state, proof_state, restatement from feature_steps
		 where feature = 'f1' and number = 1`).Scan(&state, &proofState, &restatement); err != nil {
		t.Fatalf("read the step after the drop: %v", err)
	}
	if state != "taken" || proofState != "passing" || restatement != "what I understood" {
		t.Fatalf("after the drop the step reads %q, proved %q, restated %q", state, proofState, restatement)
	}

	// And up again, over the rows that were written without the column.
	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate over the old shape: %v", err)
	}
	var required bool
	if err := pool.QueryRow(ctx,
		`select red_run_required from feature_steps where feature = 'f1' and number = 1`).
		Scan(&required); err != nil {
		t.Fatalf("read the requirement back: %v", err)
	}
	if required {
		t.Error("the step in flight came back bound by a rule that arrived after it started")
	}
	opened, err := store.NewPostgres(ctx, ownURL)
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(opened.Close)

	// The step in flight closes on the check it already has. Its code is written and its tests pass,
	// so no run of it can go red any more.
	closed, _, err := opened.FinishStep(ctx, "f1", 1, store.Finish{
		State: store.StepDone, Result: "shipped", ClosedBy: "operator",
	})
	if err != nil {
		t.Fatalf("finishing the step that was in flight when the rule arrived: %v", err)
	}
	if closed.GetState() != store.StepDone {
		t.Fatalf("the step came back in state %q, want %q", closed.GetState(), store.StepDone)
	}

	// The next step is taken after the rule, so it is bound: the same passing run and no red run is
	// refused.
	if _, _, err := opened.TakeStep(ctx, "f1", 2, "s1"); err != nil {
		t.Fatalf("taking the next step: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`select red_run_required from feature_steps where feature = 'f1' and number = 2`).
		Scan(&required); err != nil {
		t.Fatalf("read the requirement of the step the take bound: %v", err)
	}
	if !required {
		t.Error("a take under the rule left the step unbound by it")
	}
	if _, err := opened.RecordProof(ctx, "f1", 2, store.ProofResult{
		State: store.ProofPassing, ScenariosRun: 1, Output: "1 scenarios (1 passed)"}); err != nil {
		t.Fatalf("RecordProof on the step the take bound: %v", err)
	}
	if _, _, err := opened.FinishStep(ctx, "f1", 2, store.Finish{
		State: store.StepDone, Result: "shipped", ClosedBy: "operator",
	}); !errors.Is(err, store.ErrNoRedRun) {
		t.Fatalf("finishing the step this take bound answered %v, want ErrNoRedRun", err)
	}
}
