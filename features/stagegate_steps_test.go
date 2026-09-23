package features_test

import (
	"context"
	"fmt"

	"github.com/atlantic-blue/quay-krewe/internal/store"
	"github.com/cucumber/godog"
)

// The steps the gate on a take needs beyond the ones the stages themselves already have.
//
// A project part way through its six is the state this gate is about, so what is missing here is a
// way to say how far through it is. The rest of the vocabulary is the stages' own and the path's:
// this file adds one step rather than a second spelling of either.

func initializeStageGateSteps(sc *godog.ScenarioContext) {
	// Counted from the front, because the six are written in an order and a project cannot approve
	// the fifth without the four above it. The count is the number of stages that carry the word, so
	// a scenario says five and the sixth is whatever it goes on to write.
	sc.Step(`^the first (\d+) design stages are written and approved$`,
		func(ctx context.Context, count int) error {
			six := store.DesignStages()
			if count < 0 || count > len(six) {
				return fmt.Errorf("a project holds %d design stages, and this asks for %d", len(six), count)
			}
			for _, stage := range six[:count] {
				if err := settleStage(ctx, stage); err != nil {
					return err
				}
			}
			return nil
		})
}
