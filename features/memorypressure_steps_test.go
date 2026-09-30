package features_test

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/cucumber/godog"
	"gopkg.in/yaml.v3"
)

// Steps for the stack under memory pressure.
//
// What the kernel does when a machine runs out of memory is a property of a running daemon, and
// there is no daemon in this suite. What decides the outcome is in the file the operator's stack
// starts from, so these steps read that file. The containers job in continuous integration boots the
// stack for real.
//
// The file is parsed rather than matched as text, so a rule that moves inside the file still passes
// and a rule that is deleted still fails.

// memoryPressureKey carries the compose file a scenario read.
type memoryPressureKey struct{}

// stackUnderPressure is as much of the compose file as these steps read: what the kernel is told
// about each service, and what the daemon is told to do with it after a restart.
type stackUnderPressure struct {
	Services map[string]struct {
		// KillPriority is oom_score_adj, which the kernel adds to a process's badness score before
		// it chooses what to kill. A pointer, because zero is the value every service carried on the
		// day this went wrong, so absent and zero have to read differently.
		KillPriority *int   `yaml:"oom_score_adj"`
		Restart      string `yaml:"restart"`
	} `yaml:"services"`
}

func stackFrom(ctx context.Context) (stackUnderPressure, error) {
	held, read := ctx.Value(memoryPressureKey{}).(stackUnderPressure)
	if !read {
		return stackUnderPressure{}, fmt.Errorf("no compose file was read")
	}
	// A file with no services would pass every check below by having nothing to check, which reads
	// exactly like a stack that is protected.
	if len(held.Services) == 0 {
		return stackUnderPressure{}, fmt.Errorf("the compose file declares no services, so this scenario proves nothing")
	}
	return held, nil
}

func initializeMemoryPressureSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the operator reads the compose file the stack starts from$`, func(ctx context.Context) (context.Context, error) {
		// The same file the site scenario reads, which is the file the operator runs.
		contents, err := os.ReadFile(composeFile)
		if err != nil {
			return ctx, fmt.Errorf("reading the compose stack: %w", err)
		}
		var held stackUnderPressure
		if err := yaml.Unmarshal(contents, &held); err != nil {
			return ctx, fmt.Errorf("parsing the compose stack: %w", err)
		}
		return context.WithValue(ctx, memoryPressureKey{}, held), nil
	})

	sc.Step(`^the "([^"]*)" service carries a kill priority of (-?\d+), so the kernel takes a session first$`,
		func(ctx context.Context, name string, want int) error {
			stack, err := stackFrom(ctx)
			if err != nil {
				return err
			}
			service, declared := stack.Services[name]
			if !declared {
				return fmt.Errorf("the compose file has no %s service, so there is nothing here to protect", name)
			}
			if service.KillPriority == nil {
				return fmt.Errorf("the %s service is left at the priority every container starts with, "+
					"so the kernel weighs it against a session by size alone and it is the smaller of the two", name)
			}
			if got := *service.KillPriority; got != want {
				return fmt.Errorf("the %s service carries a kill priority of %d, want %d", name, got, want)
			}
			return nil
		})

	sc.Step(`^every service starts again by itself after the machine restarts$`, func(ctx context.Context) error {
		stack, err := stackFrom(ctx)
		if err != nil {
			return err
		}
		const policy = "unless-stopped"
		var stayDown []string
		for name, service := range stack.Services {
			if service.Restart != policy {
				stayDown = append(stayDown, name)
			}
		}
		sort.Strings(stayDown)
		if len(stayDown) > 0 {
			return fmt.Errorf("%d of %d services stay down until somebody starts them by hand: %v",
				len(stayDown), len(stack.Services), stayDown)
		}
		return nil
	})
}
