//go:build integration

package store_test

import (
	"context"
	"os"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/store"
)

// The interview becomes the first stage, and migration 0081 puts it in front of the stages a project
// already holds.
//
// A project in flight is the case this exists for. Its discovery carries the operator's word, its
// stories are being written, and the interview was never asked because nothing asked it. A row that
// simply appeared empty would refuse every write and every take on that project, so the migration
// marks it skipped. The operator runs the interview later, and until then nothing waits on it.
//
// The positions move with it. The position column is what both stores read the order out of, so a
// discovery left at position 0 would sit at the same place as the interview.
//
// The down migration runs here as well, because a down migration nobody ran is a down migration that
// does not work.
func TestAProjectInFlightReadsItsInterviewAsSkipped(t *testing.T) {
	ctx := context.Background()
	pool, ownURL := databaseOfItsOwn(t, "interview0081")

	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// The schema as it stood before this migration: seven names, and the six that existed then at the
	// positions they were written at.
	if _, err := pool.Exec(ctx, `delete from project_design_stages`); err != nil {
		t.Fatalf("clear the stages: %v", err)
	}
	for _, statement := range []string{
		`insert into workspaces (id, name) values ('w1', 'acme')`,
		`insert into projects (id, workspace, name)
			values ('p1', 'w1', 'house-bills'), ('p2', 'w1', 'house-rent')`,
		// The project in flight: its discovery is approved and its stories are written and unread.
		`insert into project_design_stages (id, project, stage, position, body, version, approved_version, approved_at)
			values ('ds1', 'p1', 'discovery', 0, 'what the repository holds', 1, 1, now())`,
		`insert into project_design_stages (id, project, stage, position, body, version)
			values ('ds2', 'p1', 'stories', 1, 'I want to see what is due', 1)`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("seed with %q: %v", statement, err)
		}
	}
	// The rows this migration lands on are written back to the state they were in before it ran, so
	// the up migration reads a database of the old shape.
	if _, err := pool.Exec(ctx,
		`delete from schema_migrations where version = $1`, migration0081); err != nil {
		t.Fatalf("wind the record back: %v", err)
	}
	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate again: %v", err)
	}

	opened, err := store.NewPostgres(ctx, ownURL)
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(opened.Close)

	held, err := opened.ListDesignStages(ctx, "p1")
	if err != nil {
		t.Fatalf("ListDesignStages: %v", err)
	}
	interview := stageNamed(held, store.StageInterview)
	if interview == nil {
		t.Fatalf("the project in flight holds %v, and the migration puts an interview in front of them",
			namesOf(held))
	}
	if !interview.GetSkipped() {
		t.Error("the interview came back unskipped, so every write and every take on this project is refused")
	}
	if interview.GetPosition() != 0 {
		t.Errorf("the interview sits at position %d, and it is the first of the seven", interview.GetPosition())
	}
	if interview.GetBody() != "" {
		t.Errorf("the migration wrote %q into the interview, and nobody asked those questions", interview.GetBody())
	}

	// The stages that were there keep their words and move down one place.
	discovery := stageNamed(held, store.StageDiscovery)
	if !discovery.GetApproved() || discovery.GetPosition() != 1 {
		t.Errorf("the discovery reads approved=%t position=%d, want approved at position 1",
			discovery.GetApproved(), discovery.GetPosition())
	}
	if body := discovery.GetBody(); body != "what the repository holds" {
		t.Errorf("the discovery holds %q, and the migration rewrites nothing", body)
	}
	if stories := stageNamed(held, store.StageStories); stories.GetPosition() != 2 {
		t.Errorf("the stories sit at position %d, want 2", stories.GetPosition())
	}

	// The rule the take and the write both read. Nothing before the stories is unsettled, so the
	// project carries on exactly where it was.
	if blocking := store.BlockingDesignStage(store.StageStories, held); blocking != "" {
		t.Errorf("writing the stories is blocked by %q, and this project was writing them yesterday", blocking)
	}

	// A project that never staged anything is left alone. It is designed the way every project was
	// before the stages existed, and a skipped row on it would say somebody decided something.
	quiet, err := opened.ListDesignStages(ctx, "p2")
	if err != nil {
		t.Fatalf("ListDesignStages for the project nobody staged: %v", err)
	}
	if len(quiet) != 0 {
		t.Errorf("the migration wrote %v onto a project nobody staged", namesOf(quiet))
	}

	// The interview is written later, which is the whole reason it is skipped rather than approved.
	// The write takes the skip off, because a stage holding a page is a stage somebody must read.
	written, err := opened.SetDesignStage(ctx, "p1", store.DesignStageWrite{
		Stage: store.StageInterview, Body: "# Goal\n\nthey pay four bills\n",
	})
	if err != nil {
		t.Fatalf("writing the interview later: %v", err)
	}
	if written.GetSkipped() {
		t.Error("the interview still reads skipped after somebody wrote it")
	}
	if written.GetPosition() != 0 {
		t.Errorf("the interview was written at position %d, want 0", written.GetPosition())
	}

	// The way back. The interview rows go, and the stages that were there sit at the positions they
	// held before, so a database rolled back reads the way it read this morning.
	down, err := os.ReadFile("migrations/" + migration0081 + ".down.sql")
	if err != nil {
		t.Fatalf("read the down migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("run the down migration: %v", err)
	}
	var rolled []string
	rows, err := pool.Query(ctx,
		`select stage from project_design_stages where project = 'p1' order by position`)
	if err != nil {
		t.Fatalf("read the stages back: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var stage string
		if err := rows.Scan(&stage); err != nil {
			t.Fatalf("scan: %v", err)
		}
		rolled = append(rolled, stage)
	}
	if len(rolled) != 2 || rolled[0] != store.StageDiscovery || rolled[1] != store.StageStories {
		t.Errorf("the rolled back project holds %v, want the discovery and the stories in that order", rolled)
	}
}

// migration0081 is the file this test is about, named once so the record and the down file cannot
// come to disagree about which migration is being wound back.
const migration0081 = "0081_the_interview_comes_before_the_discovery"

// stageNamed is one stage out of a listing, and nil for a stage the project does not hold.
func stageNamed(held []*quaycrewv1.DesignStage, name string) *quaycrewv1.DesignStage {
	for _, one := range held {
		if one.GetStage() == name {
			return one
		}
	}
	return nil
}

// namesOf is what a listing holds, for a message about a listing that holds the wrong thing.
func namesOf(held []*quaycrewv1.DesignStage) []string {
	names := make([]string, 0, len(held))
	for _, one := range held {
		names = append(names, one.GetStage())
	}
	return names
}
