package features_test

import (
	"context"
	"fmt"

	"github.com/atlantic-blue/quay-krewe/internal/store"
	"github.com/cucumber/godog"
)

// The vocabulary the red run gate needs, beyond the one the path already has.
//
// The path's own line says krewe checked a step, and gives the verdict. It stands a whole flow up,
// because a passing verdict in a real project always follows a run that failed. These scenarios are
// about what one single run records, so they write one run at a time and say so.

func initializeRedRunSteps(sc *godog.ScenarioContext) {
	sc.Step(`^krewe ran step (\d+)'s scenario, and it (passed|failed)$`,
		func(ctx context.Context, number int, said string) error {
			state := store.ProofPassing
			if said == "failed" {
				state = store.ProofFailing
			}
			return recordOneRun(ctx, int32(number), state, 1)
		})

	// A run that reported no scenario at all: a name filter that matched nothing, or a command the
	// shell could not start. It failed, and it ran no test, so it is not a red run.
	sc.Step(`^krewe ran step (\d+)'s scenario, and it failed with no scenario in it$`,
		func(ctx context.Context, number int) error {
			return recordOneRun(ctx, int32(number), store.ProofFailing, 0)
		})

	// A step whose row says nothing about a red run, which is what every row said when the rule
	// arrived.
	//
	// The take writes the requirement and nothing else does, so a step the path wrote and no take
	// reached carries none. That is the row a session was holding at the upgrade. This suite keeps
	// its rows in memory and runs no migration, so it is the only way to stand one up here, and the
	// migration that adds the column proves the same row in Postgres, where those rows live.
	//
	// The document carries step 1 as well, because the path the background wrote holds it and a
	// document that drops a step a session is holding is refused.
	sc.Step(`^the path holds step (\d+), whose row carries no red run requirement$`,
		func(ctx context.Context, number int) error {
			held, err := theFeature(ctx)
			if err != nil {
				return err
			}
			return setPathOf(ctx, held.GetId(), fmt.Sprintf(
				"## 1. The store holds a project's brief\n\n## %d. The store holds a project's design\n",
				number))
		})
}

// recordOneRun writes one run onto a step of the feature a scenario means when it names none.
func recordOneRun(ctx context.Context, number int32, state string, scenarios int32) error {
	held, err := theFeature(ctx)
	if err != nil {
		return err
	}
	return writeOneRun(ctx, held.GetId(), number, state, scenarios)
}
