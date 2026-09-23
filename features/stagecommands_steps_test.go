package features_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/cucumber/godog"
)

// stageCommandWorld is the file a stage is written from. The document is on the machine, which is
// what the command takes, so a scenario has to put one there before it types anything.
type stageCommandWorld struct {
	file string
}

type stageCommandKey struct{}

func stageCommandsFrom(ctx context.Context) *stageCommandWorld {
	s, _ := ctx.Value(stageCommandKey{}).(*stageCommandWorld)
	return s
}

func initializeStageCommandSteps(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, stageCommandKey{}, &stageCommandWorld{}), nil
	})

	sc.Step(`^a stage file saying "([^"]*)"$`, func(ctx context.Context, body string) error {
		return aStageFileSaying(ctx, unescape(body))
	})

	// A body that runs to more than one line arrives as a docstring, because a diagram is a fenced
	// block and a fenced block is three lines at the shortest.
	sc.Step(`^a stage file saying:$`, func(ctx context.Context, body *godog.DocString) error {
		return aStageFileSaying(ctx, body.Content)
	})

	// The steps that run the real command line tool, as an operator runs it.

	sc.Step(`^the caller writes the "([^"]*)" design stage from that file$`,
		func(ctx context.Context, stage string) error {
			file := stageCommandsFrom(ctx).file
			if file == "" {
				return fmt.Errorf("no stage file was written, so there is nothing to send")
			}
			return runTool(ctx, "stage", "set", whereTheProjectIs(ctx), stage, "--file", file)
		})

	sc.Step(`^the caller approves the "([^"]*)" design stage$`, func(ctx context.Context, stage string) error {
		return runTool(ctx, "stage", "approve", whereTheProjectIs(ctx), stage)
	})

	// The call carries the driver's token, which is what a session inside a sandbox presents. The
	// scenario reads the stage back afterwards, because a refusal that approved the row anyway would
	// leave the gate looking closed and standing open.
	sc.Step(`^the driver asks to approve the "([^"]*)" design stage$`, func(ctx context.Context, stage string) error {
		return asDriver(ctx, func(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient) error {
			_, err := client.ApproveDesignStage(ctx, &quaycrewv1.ApproveDesignStageRequest{
				Project: worldFrom(ctx).projectID, Stage: stage})
			return err
		})
	})
}

// aStageFileSaying is the file the operator points krewe stage set at.
func aStageFileSaying(ctx context.Context, body string) error {
	dir, err := os.MkdirTemp("", "krewe-stage-")
	if err != nil {
		return err
	}
	at := filepath.Join(dir, "stage.md")
	stageCommandsFrom(ctx).file = at
	return os.WriteFile(at, []byte(body), 0o600)
}
