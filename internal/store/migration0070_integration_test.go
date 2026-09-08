//go:build integration

package store_test

import (
	"context"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/store"
)

// A step records who closed it now, and migration 0070 adds the column that holds it.
//
// Every step this project wrote before today was written without it, and some of them were closed by
// hand while no call could close one. So the risk is not the schema half, which every other test
// proves by running: it is a step written before the column existed coming back short of what it
// held. A default that failed to apply, or a read that lost a column to the new one, reads as a path
// that survived and is not one.
func TestAStepWrittenBeforeTheClosedByColumnReadsBackWhole(t *testing.T) {
	ctx := context.Background()
	pool, ownURL := databaseOfItsOwn(t, "step0070")

	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Back to the shape a system had before this change. This is what an operator's database looks
	// like at the moment they upgrade.
	for _, statement := range []string{
		`alter table feature_steps drop column closed_by`,
		`delete from schema_migrations where version = '0070_a_step_records_what_came_of_it'`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("put the schema back to what it was: %s: %v", statement, err)
		}
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
	// A step that finished before anything recorded who closed it, and one nobody has touched. The
	// finished one is the case this migration is about: it holds the record of work, and a default
	// that failed to apply would take it away.
	if _, err := pool.Exec(ctx, `
		insert into feature_steps
			(feature, number, title, intention, touches, proof, proof_scenario, after, milestone,
			 state, session, result, taken_at, finished_at)
		values
			('f1', 1, 'the store holds a brief', 'The design has nowhere to live.',
			 'internal/store/store.go', 'It reads back.', 'a project carries a brief', 0, 4,
			 'done', 'session-one', 'shipped as pull request 712', now(), now()),
			('f1', 2, 'the store holds a design', '', '', '', '', 1, 4, 'ready', '', '', null, null)`,
	); err != nil {
		t.Fatalf("seed the path: %v", err)
	}

	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate over the old shape: %v", err)
	}

	opened, err := store.NewPostgres(ctx, ownURL)
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(opened.Close)
	steps, err := opened.ListSteps(ctx, "f1")
	if err != nil {
		t.Fatalf("read the path back: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("the path reads back as %d steps, want the 2 that were written", len(steps))
	}
	finished, ready := steps[0], steps[1]
	// The new column says nothing on a step closed before it existed, which is the truth: nothing
	// recorded who spoke the word.
	if finished.GetClosedBy() != "" {
		t.Errorf("the finished step says %q closed it, and it was closed before the column existed",
			finished.GetClosedBy())
	}
	if finished.GetState() != store.StepDone || finished.GetResult() != "shipped as pull request 712" {
		t.Errorf("the finished step reads as %q with the result %q",
			finished.GetState(), finished.GetResult())
	}
	if finished.GetSession() != "session-one" || finished.GetTakenAt() == nil ||
		finished.GetFinishedAt() == nil {
		t.Errorf("the finished step reads held by %q, taken at %v, finished at %v",
			finished.GetSession(), finished.GetTakenAt(), finished.GetFinishedAt())
	}
	if finished.GetTitle() != "the store holds a brief" || finished.GetMilestone() != 4 {
		t.Errorf("the finished step reads %q in milestone %d",
			finished.GetTitle(), finished.GetMilestone())
	}
	if ready.GetState() != store.StepReady || ready.GetClosedBy() != "" {
		t.Errorf("the ready step reads as %q closed by %q", ready.GetState(), ready.GetClosedBy())
	}

	// The column the migration added takes a word, and the call that closes a step writes it.
	closed, err := opened.FinishStep(ctx, "f1", 2, store.Finish{
		State: store.StepDone, Result: "it reads back whole", ClosedBy: "operator",
	})
	if err != nil {
		t.Fatalf("FinishStep after the migration: %v", err)
	}
	if closed.GetClosedBy() != "operator" || closed.GetFinishedAt() == nil {
		t.Fatalf("the step reads closed by %q at %v", closed.GetClosedBy(), closed.GetFinishedAt())
	}
}
