//go:build integration

package store_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/store"
)

// The trust record, and migration 0075 adds the seven columns that hold it: six on the design, one
// on the step.
//
// Every project and every step this system wrote before today was written without them, so the risk
// is not the schema half, which every other test proves by running. It is a design row written
// before the columns existed coming back at a level nobody set, or a step written before them coming
// back saying the operator agreed with a verdict nobody ran.
//
// The down migration runs here as well, because a down migration nobody ran is a down migration that
// does not work. It goes down, the rows survive with everything else they hold, and they come back
// up reading a level of 0 and an empty word.
func TestAProjectWrittenBeforeTheTrustColumnsReadsBackWhole(t *testing.T) {
	ctx := context.Background()
	pool, ownURL := databaseOfItsOwn(t, "trust0075")

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
	// A project that was already designed and approved, holding a step somebody was already building,
	// when the upgrade ran. That is the case this migration is about: the seven defaults have to land
	// without touching the design, the word spoken over it, or the record on the step.
	if _, err := pool.Exec(ctx, `
		insert into project_designs (project, brief, body, approved, approved_at)
		values ('p1', 'the bills of one house', 'the design, whole', true, now())`); err != nil {
		t.Fatalf("seed the design: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		insert into feature_steps (feature, number, title, proof_scenario, state, session,
			proof_state, proof_scenarios_run, proof_ran_at)
		values ('f1', 1, 'the store holds a brief', 'a project carries a brief', 'taken', 's1',
			'passing', 1, now())`); err != nil {
		t.Fatalf("seed the step: %v", err)
	}

	// Down, which is the state a system is in the moment before it takes this migration, and the
	// state an operator rolling back is left in.
	//
	// The shipped file itself, read off disk and run the way an operator runs it. A copy of its
	// statements written out here would prove a down migration nobody ships.
	down, err := os.ReadFile("migrations/0075_krewe_earns_the_word_done.down.sql")
	if err != nil {
		t.Fatalf("read the down migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("take the trust columns away: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`delete from schema_migrations where version = $1`,
		"0075_krewe_earns_the_word_done"); err != nil {
		t.Fatalf("forget the migration: %v", err)
	}
	for table, columns := range map[string][]string{
		"project_designs": {"trust_level", "trust_threshold", "trust_run", "trust_offered",
			"trust_agreements", "trust_disagreements"},
		"feature_steps": {"operator_agreed"},
	} {
		for _, column := range columns {
			var there bool
			if err := pool.QueryRow(ctx, `
				select exists (
					select 1 from information_schema.columns
					where table_name = $1 and column_name = $2)`, table, column).Scan(&there); err != nil {
				t.Fatalf("read the schema for %s.%s: %v", table, column, err)
			}
			if there {
				t.Fatalf("the down migration left %s on %s", column, table)
			}
		}
	}
	// closed_by belongs to migration 0070 and stays, so a step that was closed still says who closed
	// it after this one is rolled back.
	var closerThere bool
	if err := pool.QueryRow(ctx, `
		select exists (
			select 1 from information_schema.columns
			where table_name = 'feature_steps' and column_name = 'closed_by')`).Scan(&closerThere); err != nil {
		t.Fatalf("read the schema for closed_by: %v", err)
	}
	if !closerThere {
		t.Fatal("the down migration took closed_by, which migration 0070 owns")
	}
	// The rest of the rows are untouched by the drop. The trust record says how often the operator
	// agreed, and never what the design says or what a run reported.
	var brief string
	var approved bool
	if err := pool.QueryRow(ctx,
		`select brief, approved from project_designs where project = 'p1'`).Scan(&brief, &approved); err != nil {
		t.Fatalf("read the design after the drop: %v", err)
	}
	if brief != "the bills of one house" || !approved {
		t.Fatalf("after the drop the design reads %q, approved %v", brief, approved)
	}

	// And up again, over the rows that were written without the columns.
	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate over the old shape: %v", err)
	}

	opened, err := store.NewPostgres(ctx, ownURL)
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(opened.Close)
	design, err := opened.GetDesign(ctx, "p1")
	if err != nil {
		t.Fatalf("read the design back: %v", err)
	}
	if got := design.GetTrustLevel(); got != store.TrustLevelChecked {
		t.Errorf("the design reads level %d, want %d", got, store.TrustLevelChecked)
	}
	if got := design.GetTrustThreshold(); got != store.DefaultTrustThreshold {
		t.Errorf("the design reads a threshold of %d, want %d", got, store.DefaultTrustThreshold)
	}
	if design.GetTrustRun() != 0 || design.GetTrustAgreements() != 0 || design.GetTrustDisagreements() != 0 {
		t.Errorf("the design reads a run of %d, %d agreements and %d disagreements, and nothing was counted",
			design.GetTrustRun(), design.GetTrustAgreements(), design.GetTrustDisagreements())
	}
	if design.GetTrustOffered() {
		t.Error("the design reads as offered the next level, and nobody offered it")
	}
	if !design.GetApproved() || design.GetBrief() != "the bills of one house" {
		t.Errorf("the design reads %q, approved %v", design.GetBrief(), design.GetApproved())
	}
	held, err := opened.GetStep(ctx, "f1", 1)
	if err != nil {
		t.Fatalf("read the step back: %v", err)
	}
	if got := held.GetOperatorAgreed(); got != "" {
		t.Errorf("the step reads %q, and nobody closed it", got)
	}
	if held.GetState() != "taken" || held.GetProofState() != store.ProofPassing {
		t.Errorf("the step reads %q, proved %q", held.GetState(), held.GetProofState())
	}

	// The columns the migration added take the record, and the call that closes a step writes it.
	written, moved, err := opened.FinishStep(ctx, "f1", 1, store.Finish{
		State: store.StepDone, Result: "it reads back whole", ClosedBy: "operator",
	})
	if err != nil {
		t.Fatalf("FinishStep after the migration: %v", err)
	}
	if got := written.GetOperatorAgreed(); got != store.AgreedYes {
		t.Fatalf("the step reads %q after done on a passing check, want %q", got, store.AgreedYes)
	}
	if moved.GetTrustRun() != 1 || moved.GetTrustAgreements() != 1 {
		t.Fatalf("the design reads a run of %d and %d agreements after one finish",
			moved.GetTrustRun(), moved.GetTrustAgreements())
	}
}

// A disagreement takes the level back down, and level 1 is the only level it can come down from.
// Nothing raises a level yet: the raise is the operator accepting an offer, and the offer is a later
// step of this path, so no caller can put a project above level 0.
//
// So the column is written here in one statement, and the finish is made through the call that closes
// a step. The memory store is held to the same thing in TestADisagreementAtLevelOneLowersTheLevel.
func TestADisagreementAtLevelOneLowersTheLevelInPostgres(t *testing.T) {
	ctx := context.Background()
	pool, ownURL := databaseOfItsOwn(t, "trustlevel0075")

	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, statement := range []string{
		`insert into workspaces (id, name) values ('w1', 'acme')`,
		`insert into projects (id, workspace, name) values ('p1', 'w1', 'house-bills')`,
		`insert into features (id, project, number, title) values ('f1', 'p1', 1, 'the bills')`,
		`insert into feature_steps (feature, number, title, state, proof_state, proof_scenarios_run,
			proof_ran_at) values ('f1', 1, 'the first', 'taken', 'failing', 1, now())`,
		// Where the operator accepting an offer will put the project.
		`insert into project_designs (project, body, trust_level, trust_run, trust_agreements)
			values ('p1', 'the design, whole', 1, 0, 5)`,
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
	// Proved here rather than assumed, so a seed that never raised anything cannot pass this test by
	// doing nothing at all.
	standing, err := opened.GetDesign(ctx, "p1")
	if err != nil {
		t.Fatalf("GetDesign before the finish: %v", err)
	}
	if got := standing.GetTrustLevel(); got != store.TrustLevelCloses {
		t.Fatalf("the seeded design stands at level %d, want %d", got, store.TrustLevelCloses)
	}

	_, design, err := opened.FinishStep(ctx, "f1", 1, store.Finish{
		State: store.StepDone, Result: "the scenario is wrong, not the code", ClosedBy: "operator",
	})
	if err != nil {
		t.Fatalf("FinishStep: %v", err)
	}
	if got := design.GetTrustLevel(); got != store.TrustLevelChecked {
		t.Errorf("the level reads %d after one disagreement at level 1, want %d",
			got, store.TrustLevelChecked)
	}
	if got := design.GetTrustRun(); got != 0 {
		t.Errorf("the run reads %d after a disagreement, want 0", got)
	}
	if got := design.GetTrustAgreements(); got != 5 {
		t.Errorf("the record reads %d agreements, and a disagreement takes none away", got)
	}
	read, err := opened.GetDesign(ctx, "p1")
	if err != nil {
		t.Fatalf("GetDesign after the finish: %v", err)
	}
	if got := read.GetTrustLevel(); got != store.TrustLevelChecked {
		t.Errorf("the design reads back level %d, want %d", got, store.TrustLevelChecked)
	}
}

// The ladder has two rungs. Krewe only ever offers the level below the top, so an offer standing at
// level 1 is a row no call can produce, and the guard in the raise is what makes sure a row that
// somehow held one still never reads as level 2.
//
// So the columns are written here in one statement, and the raise is made through the call the
// operator makes. The memory store is held to the same thing in
// TestARaiseWithAnOfferStandingAtLevelOneIsRefused.
func TestARaiseAtLevelOneIsRefusedInPostgres(t *testing.T) {
	ctx := context.Background()
	pool, ownURL := databaseOfItsOwn(t, "trustraise0075")

	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, statement := range []string{
		`insert into workspaces (id, name) values ('w1', 'acme')`,
		`insert into projects (id, workspace, name) values ('p1', 'w1', 'house-bills')`,
		// A row at the top of the ladder carrying an offer of a level above it, which is the state the
		// guard exists for.
		`insert into project_designs (project, body, trust_level, trust_offered, trust_run)
			values ('p1', 'the design, whole', 1, true, 5)`,
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
	// Proved here rather than assumed, so a seed that wrote no offer cannot pass this test by leaving
	// the row with nothing on it at all.
	standing, err := opened.GetDesign(ctx, "p1")
	if err != nil {
		t.Fatalf("GetDesign before the raise: %v", err)
	}
	if !standing.GetTrustOffered() || standing.GetTrustLevel() != store.TrustLevelCloses {
		t.Fatalf("the seeded design stands at level %d with an offer of %v",
			standing.GetTrustLevel(), standing.GetTrustOffered())
	}

	if _, err := opened.RaiseTrust(ctx, "p1"); !errors.Is(err, store.ErrNoOfferStanding) {
		t.Fatalf("a raise at level 1 with an offer standing answered %v, want ErrNoOfferStanding", err)
	}
	read, err := opened.GetDesign(ctx, "p1")
	if err != nil {
		t.Fatalf("GetDesign after the refusal: %v", err)
	}
	if got := read.GetTrustLevel(); got != store.TrustLevelCloses {
		t.Errorf("the refused raise left the level at %d, want %d", got, store.TrustLevelCloses)
	}
}
