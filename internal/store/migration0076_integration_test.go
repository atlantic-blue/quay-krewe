//go:build integration

package store_test

import (
	"context"
	"os"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/store"
)

// Why a session left the listing, and migration 0076 adds the column that holds it.
//
// Every session this system archived before today was archived by a person or by the project sweep,
// and no row says which. So the migration backfills nothing, and this is the test of that: a session
// put away before the column existed comes back reading the empty string rather than a word somebody
// guessed on its behalf. A guess would be a made up fact in a record nobody could ever correct, and
// the record exists to be audited.
//
// The down migration runs here as well, because a down migration nobody ran is a down migration that
// does not work. It goes down, every session stays archived with its stamp, and they come back up
// reading no reason at all.
func TestASessionArchivedBeforeTheReasonExistedReadsNoReason(t *testing.T) {
	ctx := context.Background()
	pool, ownURL := databaseOfItsOwn(t, "archivereason0076")

	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, statement := range []string{
		`insert into workspaces (id, name) values ('w1', 'acme')`,
		`insert into projects (id, workspace, name) values ('p1', 'w1', 'house-bills')`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("seed with %q: %v", statement, err)
		}
	}

	// Down, which is the shape the sessions table had the moment before this migration ran, and the
	// shape an operator rolling back is left in.
	//
	// The shipped file itself, read off disk and run the way an operator runs it. A copy of its
	// statements written out here would prove a down migration nobody ships.
	down, err := os.ReadFile("migrations/0076_the_archive_says_who_put_a_session_away.down.sql")
	if err != nil {
		t.Fatalf("read the down migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("take the reason away: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`delete from schema_migrations where version = $1`,
		"0076_the_archive_says_who_put_a_session_away"); err != nil {
		t.Fatalf("forget the migration: %v", err)
	}
	var there bool
	if err := pool.QueryRow(ctx, `
		select exists (
			select 1 from information_schema.columns
			where table_name = 'sessions' and column_name = 'archived_reason')`).Scan(&there); err != nil {
		t.Fatalf("read the schema: %v", err)
	}
	if there {
		t.Fatal("the down migration left archived_reason on sessions")
	}
	// archived_at belongs to migration 0004 and stays, so a session that was put away is still put
	// away after this one is rolled back.
	var stampThere bool
	if err := pool.QueryRow(ctx, `
		select exists (
			select 1 from information_schema.columns
			where table_name = 'sessions' and column_name = 'archived_at')`).Scan(&stampThere); err != nil {
		t.Fatalf("read the schema for archived_at: %v", err)
	}
	if !stampThere {
		t.Fatal("the down migration took archived_at, which migration 0004 owns")
	}

	// Two sessions written by a system that had nowhere to record a reason: one a person archived,
	// one the sweep took. Nothing on either row says which, which is the whole point.
	for _, statement := range []string{
		`insert into sessions (id, workspace, project, handle, status, archived_at, updated_at)
			values ('s1', 'w1', 'p1', 'put-away-by-somebody', 'stopped', now(), now())`,
		`insert into sessions (id, workspace, project, handle, status, archived_at, updated_at)
			values ('s2', 'w1', 'p1', 'swept-up', 'stopped', now(), now())`,
		`insert into sessions (id, workspace, project, handle, status, updated_at)
			values ('s3', 'w1', 'p1', 'still-working', 'idle', now())`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("seed with %q: %v", statement, err)
		}
	}

	// And up again, over the rows that were written without the column.
	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate over the old shape: %v", err)
	}

	opened, err := store.NewPostgres(ctx, ownURL)
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(opened.Close)
	for _, id := range []string{"s1", "s2"} {
		held, err := opened.GetSession(ctx, id)
		if err != nil {
			t.Fatalf("read session %s back: %v", id, err)
		}
		if got := held.GetArchivedReason(); got != store.ArchivedByNobodyRecorded {
			t.Errorf("session %s reads %q, and nothing in its row ever said why it went", id, got)
		}
		if held.GetArchivedAt() == nil {
			t.Errorf("session %s came back live, and the migration moves no session", id)
		}
	}
	// The archived listing carries the same answer, because that listing is where a person reads it.
	archived, err := opened.ListSessions(ctx, store.SessionFilter{Project: "p1", Archived: true})
	if err != nil {
		t.Fatalf("list the archived sessions: %v", err)
	}
	if len(archived) != 2 {
		t.Fatalf("the archived listing holds %d sessions, want the two that were put away", len(archived))
	}
	for _, one := range archived {
		if got := one.GetArchivedReason(); got != store.ArchivedByNobodyRecorded {
			t.Errorf("the archived listing says session %s went by %q, want nothing at all",
				one.GetId(), got)
		}
	}

	// The column the migration added takes the record from here on, so the empty answer above is a
	// missing record rather than a column nothing can write.
	if err := opened.ArchiveSession(ctx, "s3"); err != nil {
		t.Fatalf("ArchiveSession after the migration: %v", err)
	}
	named, err := opened.GetSession(ctx, "s3")
	if err != nil {
		t.Fatalf("read the newly archived session back: %v", err)
	}
	if got := named.GetArchivedReason(); got != store.ArchivedByHand {
		t.Fatalf("the session a person archived reads %q, want %q", got, store.ArchivedByHand)
	}
}
