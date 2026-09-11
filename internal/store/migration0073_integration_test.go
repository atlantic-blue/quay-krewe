//go:build integration

package store_test

import (
	"context"
	"os"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/store"
)

// A project carries what one scenario run looks like, and migration 0073 adds the three columns that
// hold it.
//
// Every project this system wrote before today was written without them, so the risk is not the
// schema half, which every other test proves by running. It is a design row written before the
// columns existed coming back short of what it held, or coming back with a pattern of nothing, which
// reads later as a run that counted no scenarios.
//
// The down migration is run here as well, because a down migration nobody ran is a down migration
// that does not work. It goes down, the row survives with everything else it holds, and it comes back
// up with the defaults on it.
func TestADesignWrittenBeforeTheProofColumnsReadsBackWhole(t *testing.T) {
	ctx := context.Background()
	pool, ownURL := databaseOfItsOwn(t, "design0073")

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
	// A design the operator had already approved when the upgrade ran. That is the case this
	// migration is about: the three defaults have to land on the row without touching the word.
	if _, err := pool.Exec(ctx, `
		insert into project_designs (project, brief, body, approved, approved_at, steps_in_flight_cap)
		values ('p1', 'keep the bills paid', '# Bills\n', true, now(), 4)`); err != nil {
		t.Fatalf("seed the design: %v", err)
	}

	// Down, which is the state a system is in the moment before it takes this migration, and the
	// state an operator rolling back is left in.
	// The shipped file itself, read off disk and run the way an operator runs it. A copy of its
	// statements written out here would prove a down migration nobody ships.
	down, err := os.ReadFile("migrations/0073_a_design_carries_a_proof_command.down.sql")
	if err != nil {
		t.Fatalf("read the down migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("take the proof columns away: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`delete from schema_migrations where version = $1`,
		"0073_a_design_carries_a_proof_command"); err != nil {
		t.Fatalf("forget the migration: %v", err)
	}
	for _, column := range []string{"proof_command", "proof_count_pattern", "proof_timeout_seconds"} {
		var there bool
		if err := pool.QueryRow(ctx, `
			select exists (
				select 1 from information_schema.columns
				where table_name = 'project_designs' and column_name = $1)`, column).Scan(&there); err != nil {
			t.Fatalf("read the schema for %s: %v", column, err)
		}
		if there {
			t.Fatalf("the down migration left %s on the table", column)
		}
	}
	// The rest of the row is untouched by the drop. The columns say how a step is run and never what
	// the design says, so an operator who rolls back keeps their design and their approval.
	var brief string
	var approved bool
	if err := pool.QueryRow(ctx,
		`select brief, approved from project_designs where project = 'p1'`).Scan(&brief, &approved); err != nil {
		t.Fatalf("read the design after the drop: %v", err)
	}
	if brief != "keep the bills paid" || !approved {
		t.Fatalf("after the drop the design reads brief %q, approved %v", brief, approved)
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
	held, err := opened.GetDesign(ctx, "p1")
	if err != nil {
		t.Fatalf("read the design back: %v", err)
	}
	// The command is empty, which is the truth: nobody ever set one. The other two carry the column
	// defaults, so nothing downstream has to treat an empty pattern or a budget of zero as a case.
	if got := held.GetProofCommand(); got != "" {
		t.Errorf("the design proves one scenario with %q, and nobody set a command", got)
	}
	if got := held.GetProofCountPattern(); got != store.DefaultProofCountPattern {
		t.Errorf("it reads the count with %q, want %q", got, store.DefaultProofCountPattern)
	}
	if got := held.GetProofTimeoutSeconds(); got != store.DefaultProofTimeoutSeconds {
		t.Errorf("one run has %d seconds, want %d", got, store.DefaultProofTimeoutSeconds)
	}
	if !held.GetApproved() || held.GetBrief() != "keep the bills paid" {
		t.Errorf("the design reads brief %q, approved %v", held.GetBrief(), held.GetApproved())
	}
	if got := held.GetStepsInFlightCap(); got != 4 {
		t.Errorf("the cap reads %d, want the 4 the row already held", got)
	}

	// The columns the migration added take a proof command, and the call that records one writes it.
	written, err := opened.SetProofCommand(ctx, "p1", store.ProofSettings{
		Command: "go test ./features/... -run '{scenario}'", TimeoutSeconds: 120})
	if err != nil {
		t.Fatalf("SetProofCommand after the migration: %v", err)
	}
	if got := written.GetProofCommand(); got != "go test ./features/... -run '{scenario}'" {
		t.Fatalf("the design proves one scenario with %q", got)
	}
	if got := written.GetProofTimeoutSeconds(); got != 120 {
		t.Fatalf("one run has %d seconds, want 120", got)
	}
	// The pattern the row was born with survives a write that said nothing about it, which is the one
	// rule this write has beyond keeping what it is given.
	if got := written.GetProofCountPattern(); got != store.DefaultProofCountPattern {
		t.Fatalf("it reads the count with %q, and the write said nothing about the pattern", got)
	}
	if !written.GetApproved() {
		t.Error("setting the proof command cleared the approval")
	}
}
