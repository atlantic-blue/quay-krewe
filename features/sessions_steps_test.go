package features_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/cucumber/godog"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Steps for the age a project sweep puts a session away at.
//
// The age travels as an instant rather than as a length, and that is what these steps rest on. A
// length is read against a clock at each end, so a scenario that says "older than ten milliseconds"
// is a scenario that passes or fails on how long the call took. A moment taken once, with a wait
// either side of it, puts every session firmly on one side of the rule and leaves nothing for a busy
// machine to move.

// ageKey holds the moment one scenario swept by, which two steps apart have to agree on.
type ageKey struct{}

type sweepAge struct{ moment time.Time }

func ageFrom(ctx context.Context) *sweepAge {
	a, _ := ctx.Value(ageKey{}).(*sweepAge)
	return a
}

func initializeSessionAgeSteps(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, ageKey{}, &sweepAge{}), nil
	})

	// The wait either side is what makes the moment a real line rather than a tie. The store writes
	// its own stamps and takes none, so there is no other way to put a session on a side of it.
	sc.Step(`^a moment every session so far is older than$`, func(ctx context.Context) error {
		time.Sleep(10 * time.Millisecond)
		ageFrom(ctx).moment = time.Now().UTC()
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	sc.Step(`^(\d+) stopped sessions in the project$`, func(ctx context.Context, count int) error {
		w := worldFrom(ctx)
		for i := 0; i < count; i++ {
			if err := w.dispatch(ctx, w.projectID, "", fmt.Sprintf("a finished subject %d", i)); err != nil {
				return err
			}
			if w.lastErr != nil {
				return w.lastErr
			}
			current, err := w.lastExec()
			if err != nil {
				return err
			}
			if _, err := w.client.StopSession(ctx, &quaycrewv1.StopSessionRequest{Id: current.sessionID}); err != nil {
				return err
			}
		}
		return nil
	})

	sc.Step(`^the operator archives the project's sessions older than that moment$`, func(ctx context.Context) error {
		moment := ageFrom(ctx).moment
		if moment.IsZero() {
			return fmt.Errorf("no moment was taken, so this sweep would read every session as old")
		}
		return sweepTheProject(ctx, moment)
	})

	// A day, on sessions this scenario made seconds ago, so every one of them is younger than the age
	// by a margin nothing on the machine can close.
	sc.Step(`^the operator archives the project's sessions older than a day$`, func(ctx context.Context) error {
		return sweepTheProject(ctx, time.Now().UTC().Add(-24*time.Hour))
	})

	// Why each session stayed, which is the half of the answer a count cannot carry. A sweep that
	// leaves 17 of 276 says nothing until it says which of them were working.
	sc.Step(`^the sessions left are (\d+) holding a container and (\d+) younger than the age$`,
		func(ctx context.Context, holding, young int) error {
			sweep := worldFrom(ctx).lastSweep
			if got := int(sweep.GetHoldingAContainer()); got != holding {
				return fmt.Errorf("the sweep says %d sessions hold a container, want %d", got, holding)
			}
			if got := int(sweep.GetYoungerThanTheAge()); got != young {
				return fmt.Errorf("the sweep says %d sessions are younger than the age, want %d", got, young)
			}
			// The two reasons are the whole of what it left, or one of them is a count of nothing.
			if got := int(sweep.GetSkipped()); got != holding+young {
				return fmt.Errorf("the sweep left %d sessions and gives a reason for %d of them",
					got, holding+young)
			}
			return nil
		})

	sc.Step(`^the driver asks to archive the project's sessions older than a day$`, func(ctx context.Context) error {
		project := worldFrom(ctx).projectID
		return asDriver(ctx, func(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient) error {
			_, err := client.ArchiveProjectSessions(ctx, &quaycrewv1.ArchiveProjectSessionsRequest{
				Project:         project,
				LastMovedBefore: timestamppb.New(time.Now().UTC().Add(-24 * time.Hour)),
			})
			return err
		})
	})

	// The steps that run the real tool. What an operator does with this word is type it and read what
	// came back, so the default and the refusal are proved on the screen rather than on the wire.
	sc.Step(`^the caller archives the project's sessions$`, func(ctx context.Context) error {
		return runTool(ctx, "archive", projectAddress(ctx))
	})
	sc.Step(`^the caller archives the project's sessions older than "([^"]*)"$`,
		func(ctx context.Context, age string) error {
			return runTool(ctx, "archive", projectAddress(ctx), "--older-than", age)
		})

	sc.Step(`^the tool names (\S+) as the age it swept by$`, func(ctx context.Context, age string) error {
		return toolSaid(ctx, "not been touched for "+age)
	})
	sc.Step(`^the tool says nothing was archived$`, func(ctx context.Context) error {
		return toolSaid(ctx, "was archived")
	})
	sc.Step(`^the tool says (\d+) sessions were left, (\d+) holding a container and (\d+) younger than the age$`,
		func(ctx context.Context, left, holding, young int) error {
			return toolSaid(ctx, fmt.Sprintf("%d sessions left in the listing: %d holding a container, %d younger than",
				left, holding, young))
		})
	sc.Step(`^the tool says the way to reach further back$`, func(ctx context.Context) error {
		return toolSaid(ctx, "--older-than")
	})
	sc.Step(`^the refusal says an age of (\S+) would take the whole project$`,
		func(ctx context.Context, age string) error {
			for _, want := range []string{"an age of " + age, "every session in the project"} {
				if !strings.Contains(toolFrom(ctx).stderr, want) {
					return fmt.Errorf("the refusal does not say %q:\n%s", want, toolFrom(ctx).stderr)
				}
			}
			return nil
		})
}

// sweepTheProject is the call all three of the age scenarios make, kept in one place so they cannot
// drift into asking for different things.
func sweepTheProject(ctx context.Context, before time.Time) error {
	w := worldFrom(ctx)
	w.lastSweep, w.lastErr = w.client.ArchiveProjectSessions(ctx, &quaycrewv1.ArchiveProjectSessionsRequest{
		Project:         w.projectID,
		LastMovedBefore: timestamppb.New(before),
	})
	return w.lastErr
}

// projectAddress is the project the scenario is standing in, written the way somebody types it.
func projectAddress(ctx context.Context) string {
	w := worldFrom(ctx)
	return w.workspaceName + "/" + w.projectName
}

// toolSaid is what the caller read on the screen, which for this word is the answer itself.
func toolSaid(ctx context.Context, want string) error {
	if printed := toolFrom(ctx).stdout; !strings.Contains(printed, want) {
		return fmt.Errorf("the tool does not say %q:\n%s", want, printed)
	}
	return nil
}
