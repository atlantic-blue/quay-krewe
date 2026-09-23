//go:build integration

package store_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/store"
)

// A step says whether it needs a check, and migration 0080 adds the column that says so, and frees
// the steps the gates had stranded.
//
// The check and the red run both stand on a proof command: krewe runs it to reach a verdict, and a
// project that never set one has nothing for krewe to run. Read against every project, the gates left
// those steps unable to close at all. So the column reads false on every row that was already there,
// the migration clears the red run requirement off the steps in flight of a project that proves
// nothing, and a take from now on writes what the project asks for.
//
// A step in flight of a project that does prove its steps keeps its red run requirement, because that
// step can still meet it.
//
// The down migration runs here as well, because a down migration nobody ran is a down migration that
// does not work.
func TestAStepOfAProjectWithNoProofCommandFinishesWithNoCheck(t *testing.T) {
	ctx := context.Background()
	pool, ownURL := databaseOfItsOwn(t, "step0080")

	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`insert into workspaces (id, name) values ('w1', 'acme')`); err != nil {
		t.Fatalf("seed the workspace: %v", err)
	}
	// Two projects: one that says how a scenario of it is run, and one that says nothing. The second
	// is the shape the two projects this system was building were in when the gates arrived.
	if _, err := pool.Exec(ctx,
		`insert into projects (id, workspace, name)
		 values ('p1', 'w1', 'house-bills'), ('p2', 'w1', 'house-rent')`); err != nil {
		t.Fatalf("seed the projects: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`insert into project_designs (project, body, proof_command)
		 values ('p1', '# Bills', 'go test ./features/... -run ''{scenario}''')`); err != nil {
		t.Fatalf("seed the design that proves its steps: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`insert into features (id, project, number, title)
		 values ('f1', 'p1', 1, 'the bills'), ('f2', 'p2', 1, 'the rent')`); err != nil {
		t.Fatalf("seed the features: %v", err)
	}
	// The step the gates stranded: a session is holding it, its pull request is open, and its project
	// has no proof command, so nothing can check it and it cannot close.
	if _, err := pool.Exec(ctx, `
		insert into feature_steps (feature, number, title, proof_scenario, state, session,
			restatement, restated_at, red_run_required)
		values ('f2', 1, 'the store holds a brief', 'a project carries a brief', 'taken', 's2',
			'what I understood', now(), true)`); err != nil {
		t.Fatalf("seed the stranded step: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		insert into feature_steps (feature, number, title, proof_scenario, state)
		values ('f2', 2, 'the store holds a design', 'a project carries a design', 'ready')`); err != nil {
		t.Fatalf("seed the step nobody took: %v", err)
	}
	// The step of the project that does prove its steps. It is in flight under the red run rule, and
	// it can still meet it, so the migration leaves that requirement where it is.
	if _, err := pool.Exec(ctx, `
		insert into feature_steps (feature, number, title, proof_scenario, state, session,
			restatement, restated_at, restatement_approved, restatement_approved_at,
			red_run_required)
		values ('f1', 1, 'the store holds a brief', 'a project carries a brief', 'taken', 's1',
			'what I understood', now(), true, now(), true)`); err != nil {
		t.Fatalf("seed the step of the project that proves its steps: %v", err)
	}

	// Down, which is the state a system is in the moment before it takes this migration, and the
	// state an operator rolling back is left in. The shipped file itself, read off disk and run the
	// way an operator runs it.
	down, err := os.ReadFile("migrations/0080_a_step_says_whether_it_needs_a_check.down.sql")
	if err != nil {
		t.Fatalf("read the down migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("take the requirement away: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`delete from schema_migrations where version = $1`,
		"0080_a_step_says_whether_it_needs_a_check"); err != nil {
		t.Fatalf("forget the migration: %v", err)
	}
	var there bool
	if err := pool.QueryRow(ctx, `
		select exists (
			select 1 from information_schema.columns
			where table_name = 'feature_steps' and column_name = 'check_required')`).Scan(&there); err != nil {
		t.Fatalf("read the schema: %v", err)
	}
	if there {
		t.Fatal("the down migration left check_required on the table")
	}
	// The rest of the row is untouched by the drop. The column says which gates a step was started
	// under and never what the step is, so an operator who rolls back keeps the step, its session and
	// what the session wrote.
	var state, session, restatement string
	if err := pool.QueryRow(ctx,
		`select state, session, restatement from feature_steps
		 where feature = 'f2' and number = 1`).Scan(&state, &session, &restatement); err != nil {
		t.Fatalf("read the step after the drop: %v", err)
	}
	if state != "taken" || session != "s2" || restatement != "what I understood" {
		t.Fatalf("after the drop the step reads %q, held by %q, restated %q", state, session, restatement)
	}

	// And up again, over the rows that were written without the column.
	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate over the old shape: %v", err)
	}
	var needsCheck, needsRedRun bool
	if err := pool.QueryRow(ctx,
		`select check_required, red_run_required from feature_steps where feature = 'f2' and number = 1`).
		Scan(&needsCheck, &needsRedRun); err != nil {
		t.Fatalf("read the requirements of the stranded step: %v", err)
	}
	if needsCheck || needsRedRun {
		t.Errorf("the step of a project that proves nothing came back needing a check %v and a red run %v",
			needsCheck, needsRedRun)
	}
	// The step of the project that proves its steps keeps the rule it was taken under.
	if err := pool.QueryRow(ctx,
		`select red_run_required from feature_steps where feature = 'f1' and number = 1`).
		Scan(&needsRedRun); err != nil {
		t.Fatalf("read the requirement of the step in a project that proves its steps: %v", err)
	}
	if !needsRedRun {
		t.Error("the migration took the red run rule off a step that can still meet it")
	}

	opened, err := store.NewPostgres(ctx, ownURL)
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(opened.Close)

	// The step the gates stranded closes, with nothing checked and nothing seen to fail. This is the
	// state the build was in: the work is merged and the step cannot be closed.
	closed, _, err := opened.FinishStep(ctx, "f2", 1, store.Finish{
		State: store.StepDone, Result: "shipped", ClosedBy: "operator",
	})
	if err != nil {
		t.Fatalf("finishing the step of a project that proves nothing: %v", err)
	}
	if closed.GetState() != store.StepDone {
		t.Fatalf("the step came back in state %q, want %q", closed.GetState(), store.StepDone)
	}

	// The step of the project that proves its steps is refused, as it was before this migration.
	if _, err := opened.RecordProof(ctx, "f1", 1, store.ProofResult{
		State: store.ProofPassing, ScenariosRun: 1, Output: "1 scenarios (1 passed)"}); err != nil {
		t.Fatalf("RecordProof on the step of the project that proves its steps: %v", err)
	}
	if _, _, err := opened.FinishStep(ctx, "f1", 1, store.Finish{
		State: store.StepDone, Result: "shipped", ClosedBy: "operator",
	}); !errors.Is(err, store.ErrNoRedRun) {
		t.Fatalf("finishing the step of the project that proves its steps answered %v, want ErrNoRedRun", err)
	}

	// A take after the migration reads the project it is in. The project that proves nothing binds
	// nothing, so its next step closes the way this one did.
	taken, _, err := opened.TakeStep(ctx, "f2", 2, "s3")
	if err != nil {
		t.Fatalf("taking the next step of the project that proves nothing: %v", err)
	}
	if taken.GetCheckRequired() || taken.GetRedRunRequired() {
		t.Errorf("a take in a project that proves nothing bound the step to a check %v and a red run %v",
			taken.GetCheckRequired(), taken.GetRedRunRequired())
	}
}
