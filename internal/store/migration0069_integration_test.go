//go:build integration

package store_test

import (
	"context"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/store"
)

// A step names the contracts it builds now, and migration 0069 adds the two columns that hold them.
//
// Every path this project wrote before today was written without them. So the risk is not the schema
// half, which every other test proves by running: it is a step written before the columns existed
// coming back short of what it held. A default that failed to apply, or a read that lost a column to
// the two new ones, reads as a path that survived and is not one.
func TestAStepWrittenBeforeTheContractsColumnsReadsBackWhole(t *testing.T) {
	ctx := context.Background()
	pool, ownURL := databaseOfItsOwn(t, "step0069")

	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Back to the shape a system had before this change. This is what an operator's database looks
	// like at the moment they upgrade.
	for _, statement := range []string{
		`alter table feature_steps drop column contracts`,
		`alter table feature_steps drop column contract_scope`,
		`delete from schema_migrations where version = '0069_a_step_names_the_contracts_it_builds'`,
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
	// Every column a caller may set, and the two the system writes when somebody takes a step. A
	// migration that added the columns and dropped a word would read as a path that survived.
	if _, err := pool.Exec(ctx, `
		insert into feature_steps
			(feature, number, title, intention, touches, proof, proof_scenario, after, milestone,
			 state, session)
		values
			('f1', 1, 'the store holds a brief', 'The design has nowhere to live.',
			 'internal/store/store.go', 'It reads back.', 'a project carries a brief', 0, 4,
			 'taken', 'session-one')`,
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
	if len(steps) != 1 {
		t.Fatalf("the path reads back as %d steps, want the 1 that was written", len(steps))
	}
	step := steps[0]
	// The two new columns say nothing, which is how a step says the session finds its own contracts.
	if step.GetContracts() != "" || step.GetContractScope() != "" {
		t.Errorf("the step reads back building %q scoped %q, and it was written before either column",
			step.GetContracts(), step.GetContractScope())
	}
	if step.GetNumber() != 1 || step.GetTitle() != "the store holds a brief" {
		t.Fatalf("the step reads number %d titled %q", step.GetNumber(), step.GetTitle())
	}
	if step.GetIntention() != "The design has nowhere to live." ||
		step.GetTouches() != "internal/store/store.go" {
		t.Errorf("the step reads intention %q and touches %q", step.GetIntention(), step.GetTouches())
	}
	if step.GetProof() != "It reads back." || step.GetProofScenario() != "a project carries a brief" {
		t.Errorf("the step reads proof %q and scenario %q", step.GetProof(), step.GetProofScenario())
	}
	if step.GetMilestone() != 4 || step.GetAfter() != 0 {
		t.Errorf("the step reads milestone %d waiting for %d, want milestone 4 waiting for nobody",
			step.GetMilestone(), step.GetAfter())
	}
	if step.GetState() != store.StepTaken || step.GetSession() != "session-one" {
		t.Errorf("the step reads as %q held by %q, want the state and session it was written with",
			step.GetState(), step.GetSession())
	}
}
