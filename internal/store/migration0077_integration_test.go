//go:build integration

package store_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The six stages a project is designed in, and migration 0077 adds the table that holds them.
//
// A project made before today has written none, and the migration writes none for it: the rows are
// made on the first write to a stage, so an empty answer and a project designed by its design
// document alone are the same thing. That is what makes this safe to ship over projects with work in
// flight, and it is the first thing proved here.
//
// The down migration runs as well, because a down migration nobody ran is a down migration that does
// not work. It goes down, the projects and their designs stay exactly as they were, and they come
// back up holding no stage at all.
func TestAProjectMadeBeforeTheStagesExistedHoldsNone(t *testing.T) {
	ctx := context.Background()
	pool, ownURL := databaseOfItsOwn(t, "designstages0077")

	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Down, which is the shape the schema had the moment before this migration ran, and the shape an
	// operator rolling back is left in. The shipped file itself, read off disk and run the way an
	// operator runs it: a copy of its statements written out here would prove a file nobody ships.
	down, err := os.ReadFile("migrations/0077_a_project_carries_design_stages.down.sql")
	if err != nil {
		t.Fatalf("read the down migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("take the stages away: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`delete from schema_migrations where version = $1`,
		"0077_a_project_carries_design_stages"); err != nil {
		t.Fatalf("forget the migration: %v", err)
	}
	if tableThere(ctx, t, pool) {
		t.Fatal("the down migration left project_design_stages behind")
	}

	// A project and a design written by a system that had nowhere to keep a stage, which is every
	// project this system holds today.
	for _, statement := range []string{
		`insert into workspaces (id, name) values ('w1', 'acme')`,
		`insert into projects (id, workspace, name) values ('p1', 'w1', 'house-bills')`,
		`insert into project_designs (project, body, approved) values ('p1', 'the whole design', true)`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("seed with %q: %v", statement, err)
		}
	}

	// And up again, over the rows that were written without the table.
	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate over the old shape: %v", err)
	}
	if !tableThere(ctx, t, pool) {
		t.Fatal("the migration ran and made no project_design_stages")
	}

	opened, err := store.NewPostgres(ctx, ownURL)
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(opened.Close)

	stages, err := opened.ListDesignStages(ctx, "p1")
	if err != nil {
		t.Fatalf("ListDesignStages: %v", err)
	}
	if len(stages) != 0 {
		t.Fatalf("the migration wrote %d stages for a project nobody staged", len(stages))
	}
	// The design it was already working under is untouched, approval and all, so a project with work
	// in flight carries on exactly as it did.
	design, err := opened.GetDesign(ctx, "p1")
	if err != nil {
		t.Fatalf("GetDesign: %v", err)
	}
	if !design.GetApproved() || design.GetBody() != "the whole design" {
		t.Fatalf("the design reads approved=%t body=%q", design.GetApproved(), design.GetBody())
	}

	// The table the migration added takes the record from here on, so the empty answer above is a
	// project nobody staged rather than a table nothing can write.
	written, err := opened.SetDesignStage(ctx, "p1", store.DesignStageWrite{
		Stage: store.StageDiscovery, Body: "what we asked",
	})
	if err != nil {
		t.Fatalf("SetDesignStage after the migration: %v", err)
	}
	if written.GetPosition() != 0 || written.GetVersion() != 1 {
		t.Fatalf("the first stage reads position %d version %d", written.GetPosition(), written.GetVersion())
	}
}

// A stage marked as skipped settles the rule that orders the six, so the stage after it goes in with
// no approval above it.
//
// Nothing writes that column yet: the command line that skips discovery is its own step. The row is
// seeded here in SQL for that reason, because the alternative is a column the schema carries, the
// rule reads, and no test ever exercises against a database.
func TestASkippedStageSettlesTheStageAfterIt(t *testing.T) {
	ctx := context.Background()
	pool, ownURL := databaseOfItsOwn(t, "skippedstage0077")

	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, statement := range []string{
		`insert into workspaces (id, name) values ('w1', 'acme')`,
		`insert into projects (id, workspace, name) values ('p1', 'w1', 'house-bills')`,
		`insert into project_design_stages (id, project, stage, position, skipped)
			values ('ds1', 'p1', 'discovery', 0, true)`,
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

	written, err := opened.SetDesignStage(ctx, "p1", store.DesignStageWrite{
		Stage: store.StageStories, Body: "I want to see what is due",
	})
	if err != nil {
		t.Fatalf("the stories were refused over a skipped discovery: %v", err)
	}
	if written.GetStage() != store.StageStories {
		t.Fatalf("the write answered with %q", written.GetStage())
	}
	// And the stage after that still waits, because a skip settles one stage and grants nothing to
	// the rest of the list.
	_, err = opened.SetDesignStage(ctx, "p1", store.DesignStageWrite{
		Stage: store.StageDesignSystem, Body: "one accent colour",
	})
	var blocked *store.StageNotApprovedError
	if !errors.As(err, &blocked) || blocked.Stage != store.StageStories {
		t.Fatalf("the design system went in over an unapproved stories: %v", err)
	}
}

// One row per stage per project is what makes a write an upsert on the name rather than a second row
// nobody reads. The constraint is in the schema, so this asks the database rather than the store.
func TestAProjectHoldsOneRowPerStage(t *testing.T) {
	ctx := context.Background()
	pool, _ := databaseOfItsOwn(t, "onerowperstage0077")

	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, statement := range []string{
		`insert into workspaces (id, name) values ('w1', 'acme')`,
		`insert into projects (id, workspace, name) values ('p1', 'w1', 'house-bills')`,
		`insert into projects (id, workspace, name) values ('p2', 'w1', 'car-insurance')`,
		`insert into project_design_stages (id, project, stage, position)
			values ('ds1', 'p1', 'discovery', 0)`,
		// The same stage in another project is a different row, which is what the constraint has to
		// leave alone.
		`insert into project_design_stages (id, project, stage, position)
			values ('ds2', 'p2', 'discovery', 0)`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("seed with %q: %v", statement, err)
		}
	}
	if _, err := pool.Exec(ctx, `insert into project_design_stages (id, project, stage, position)
		values ('ds3', 'p1', 'discovery', 0)`); err == nil {
		t.Fatal("a project took a second discovery row, so a write could stop being an upsert")
	}

	// And a project deleted for real takes its stages with it, which is the cascade the column
	// declares.
	if _, err := pool.Exec(ctx, `delete from projects where id = 'p1'`); err != nil {
		t.Fatalf("delete the project: %v", err)
	}
	var left int
	if err := pool.QueryRow(ctx,
		`select count(*) from project_design_stages where project = 'p1'`).Scan(&left); err != nil {
		t.Fatalf("count what is left: %v", err)
	}
	if left != 0 {
		t.Fatalf("%d stages outlived the project they belong to", left)
	}
}

// tableThere says whether the table this migration owns is in the schema.
func tableThere(ctx context.Context, t *testing.T, pool *pgxpool.Pool) bool {
	t.Helper()
	var there bool
	if err := pool.QueryRow(ctx, `
		select exists (
			select 1 from information_schema.tables where table_name = 'project_design_stages')`).
		Scan(&there); err != nil {
		t.Fatalf("read the schema: %v", err)
	}
	return there
}
