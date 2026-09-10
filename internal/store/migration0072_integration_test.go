//go:build integration

package store_test

import (
	"context"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/store"
)

// A step carries what its session restated now, and migration 0072 adds the four columns that hold
// it.
//
// Every path this project wrote before today was written without them. So the risk is not the schema
// half, which every other test proves by running: it is a step written before the columns existed
// coming back short of what it held. A default that failed to apply, or a read that lost a column to
// the four new ones, reads as a path that survived and is not one.
//
// The second half of this is the invariant nothing else can reach. The word that approves a
// restatement is written by a call that does not exist yet, so this is the only place a step can be
// approved and then restated again, which is what says the approval is about one text.
func TestAStepWrittenBeforeTheRestatementColumnsReadsBackWhole(t *testing.T) {
	ctx := context.Background()
	pool, ownURL := databaseOfItsOwn(t, "step0072")

	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Back to the shape a system had before this change. This is what an operator's database looks
	// like at the moment they upgrade, and it is the down migration read as statements.
	for _, statement := range []string{
		`alter table feature_steps drop column restatement`,
		`alter table feature_steps drop column restated_at`,
		`alter table feature_steps drop column restatement_approved`,
		`alter table feature_steps drop column restatement_approved_at`,
		`delete from schema_migrations where version = '0072_a_step_carries_a_restatement'`,
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
	// A step a session was holding when the upgrade ran. That is the case this migration is about: it
	// is the row the four defaults have to land on without touching anything the take wrote.
	if _, err := pool.Exec(ctx, `
		insert into feature_steps
			(feature, number, title, intention, touches, proof, proof_scenario, after, milestone,
			 state, session, result, taken_at)
		values
			('f1', 1, 'the store holds a brief', 'The design has nowhere to live.',
			 'internal/store/store.go', 'It reads back.', 'a project carries a brief', 0, 4,
			 'taken', 'session-one', '', now())`,
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
	held, err := opened.GetStep(ctx, "f1", 1)
	if err != nil {
		t.Fatalf("read the step back: %v", err)
	}
	// The new columns say nothing on a step taken before they existed, which is the truth: that
	// session was never asked to restate anything.
	if held.GetRestatement() != "" || held.GetRestatedAt() != nil {
		t.Errorf("the step restates %q at %v, and it was taken before the columns existed",
			held.GetRestatement(), held.GetRestatedAt())
	}
	if held.GetRestatementApproved() || held.GetRestatementApprovedAt() != nil {
		t.Errorf("the step reads as approved at %v", held.GetRestatementApprovedAt())
	}
	if held.GetState() != store.StepTaken || held.GetSession() != "session-one" {
		t.Errorf("the step reads as %q held by %q", held.GetState(), held.GetSession())
	}
	if held.GetTitle() != "the store holds a brief" || held.GetMilestone() != 4 {
		t.Errorf("the step reads %q in milestone %d", held.GetTitle(), held.GetMilestone())
	}

	// The columns the migration added take a restatement, and the call that records one writes it.
	written, err := opened.SetRestatement(ctx, "f1", 1, "the first reading")
	if err != nil {
		t.Fatalf("SetRestatement after the migration: %v", err)
	}
	if written.GetRestatement() != "the first reading" || written.GetRestatedAt() == nil {
		t.Fatalf("the step restates %q at %v", written.GetRestatement(), written.GetRestatedAt())
	}

	// The operator's word, written here because the call that speaks it is a later step of this path.
	// Without it the approval is false from the moment the row is made, and a write that cleared
	// nothing would read exactly like a write that cleared it.
	if _, err := pool.Exec(ctx, `
		update feature_steps set restatement_approved = true, restatement_approved_at = now()
		where feature = 'f1' and number = 1`); err != nil {
		t.Fatalf("approve the restatement: %v", err)
	}
	approved, err := opened.GetStep(ctx, "f1", 1)
	if err != nil {
		t.Fatalf("read the approved step: %v", err)
	}
	if !approved.GetRestatementApproved() || approved.GetRestatementApprovedAt() == nil {
		t.Fatalf("the step reads as unapproved after the word was written, at %v",
			approved.GetRestatementApprovedAt())
	}

	// A second text is a text nobody has read. The approval and its moment go with the first one.
	second, err := opened.SetRestatement(ctx, "f1", 1, "the second reading")
	if err != nil {
		t.Fatalf("SetRestatement over an approved one: %v", err)
	}
	if second.GetRestatementApproved() || second.GetRestatementApprovedAt() != nil {
		t.Errorf("the second text reads as approved at %v, and nobody read it",
			second.GetRestatementApprovedAt())
	}
	if second.GetRestatement() != "the second reading" {
		t.Errorf("the step restates %q after the second write", second.GetRestatement())
	}
}
