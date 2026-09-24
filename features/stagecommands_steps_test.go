package features_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/cucumber/godog"
)

// stageCommandWorld is the file a stage is written from. The document is on the machine, which is
// what the command takes, so a scenario has to put one there before it types anything.
type stageCommandWorld struct {
	// openMark is where the open command this scenario put on the path writes the address it was
	// given, so a step reads back whether a page opened and which one.
	openMark string
	file     string
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

	// The tool opens a page by running the machine's own open command, so a scenario puts its own
	// command on the path and reads back what it was given. Both names are written, because the
	// command is open on macOS and xdg-open everywhere else, and the suite runs on both.
	sc.Step(`^a machine where opening a page leaves a mark$`, func(ctx context.Context) error {
		return aMachineWhereOpeningLeavesAMark(ctx)
	})

	sc.Step(`^nothing opened a page$`, func(ctx context.Context) error {
		opened, err := whatOpened(ctx)
		if err != nil {
			return err
		}
		if opened != "" {
			return fmt.Errorf("a page was opened at %q, and the caller asked only to be told the address", opened)
		}
		return nil
	})

	sc.Step(`^the page that opened is "([^"]*)"$`, func(ctx context.Context, want string) error {
		opened, err := whatOpened(ctx)
		if err != nil {
			return err
		}
		if opened != want {
			return fmt.Errorf("the page that opened is %q, want %q", opened, want)
		}
		return nil
	})

	// The last line, and not a line somewhere in the middle: the address sits under the listing, so a
	// person reading the six states finds it where their eye already is.
	sc.Step(`^the last line of standard output is "([^"]*)"$`, func(ctx context.Context, want string) error {
		lines := strings.Split(strings.TrimRight(toolFrom(ctx).stdout, "\n"), "\n")
		last := strings.TrimSpace(lines[len(lines)-1])
		if last != want {
			return fmt.Errorf("standard output ends with %q, want %q", last, want)
		}
		return nil
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

// aMachineWhereOpeningLeavesAMark puts a command named open, and one named xdg-open, in front of
// every other one on the path, and each of them writes down the address it was given.
//
// It is how a scenario watches the opening from outside the process. The tool runs as its own
// binary here, so there is nothing in it a test can replace, and the machine running this suite must
// not have a browser opened on it either.
func aMachineWhereOpeningLeavesAMark(ctx context.Context) error {
	dir, err := os.MkdirTemp("", "krewe-open-")
	if err != nil {
		return err
	}
	mark := filepath.Join(dir, "opened")
	stageCommandsFrom(ctx).openMark = mark
	script := "#!/bin/sh\nprintf '%s' \"$1\" > " + mark + "\n"
	for _, name := range []string{"open", "xdg-open"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o700); err != nil {
			return err
		}
	}
	t := toolFrom(ctx)
	t.env = append(t.env, "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return nil
}

// whatOpened is the address the machine was asked to open, and nothing at all when it was asked to
// open nothing.
func whatOpened(ctx context.Context) (string, error) {
	mark := stageCommandsFrom(ctx).openMark
	if mark == "" {
		return "", fmt.Errorf("this scenario never put an open command on the path, so it can prove nothing about opening")
	}
	opened, err := os.ReadFile(mark)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(opened)), nil
}
