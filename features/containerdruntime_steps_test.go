package features_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/cucumber/godog"

	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
)

// The runtime scenarios touch the control plane nowhere. What they prove is the reading of one
// configuration value: the name of a runtime becomes the backend that makes a session's container,
// and a name the system does not have stops it starting. So they call the same function the control
// plane calls at startup.

type runtimeWorld struct {
	// told is what the operator configured.
	told string
	// backend is what the system built from it, and refused is what it said instead.
	backend sandbox.Provider
	refused error
}

type runtimeKey struct{}

func runtimeFrom(ctx context.Context) *runtimeWorld {
	world, _ := ctx.Value(runtimeKey{}).(*runtimeWorld)
	return world
}

func initializeContainerdRuntimeSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the system is told to run sessions on "([^"]*)"$`, func(ctx context.Context, told string) (context.Context, error) {
		world := &runtimeWorld{told: told}
		world.backend, world.refused = sandbox.NewProvider(told, sandbox.Options{Image: "img"})
		return context.WithValue(ctx, runtimeKey{}, world), nil
	})

	sc.Step(`^the system runs sessions on "([^"]*)"$`, func(ctx context.Context, want string) error {
		world := runtimeFrom(ctx)
		kind, err := sandbox.ResolveKind(world.told)
		if err != nil {
			return fmt.Errorf("the system refused %q: %w", world.told, err)
		}
		if kind != want {
			return fmt.Errorf("a system told %q runs sessions on %q, want %q", world.told, kind, want)
		}
		return nil
	})

	sc.Step(`^a session gets a container of its own on containerd$`, func(ctx context.Context) error {
		world := runtimeFrom(ctx)
		if world.refused != nil {
			return world.refused
		}
		if _, isContainerd := world.backend.(sandbox.ContainerdProvider); !isContainerd {
			return fmt.Errorf("the backend is %T, want the containerd one", world.backend)
		}
		return nil
	})

	sc.Step(`^a session gets a container of its own on the Docker daemon$`, func(ctx context.Context) error {
		world := runtimeFrom(ctx)
		if world.refused != nil {
			return world.refused
		}
		if _, isDocker := world.backend.(sandbox.DockerProvider); !isDocker {
			return fmt.Errorf("the backend is %T, want the Docker one", world.backend)
		}
		return nil
	})

	sc.Step(`^the system refuses the runtime$`, func(ctx context.Context) error {
		world := runtimeFrom(ctx)
		if world.refused == nil {
			return fmt.Errorf("a system told %q was given %T rather than refused", world.told, world.backend)
		}
		if world.backend != nil {
			return fmt.Errorf("a refused runtime came back with a backend: %T", world.backend)
		}
		return nil
	})

	sc.Step(`^the refusal about the runtime names "([^"]*)"$`, func(ctx context.Context, want string) error {
		world := runtimeFrom(ctx)
		if world.refused == nil {
			return fmt.Errorf("nothing was refused")
		}
		if !strings.Contains(world.refused.Error(), want) {
			return fmt.Errorf("the refusal does not name %q: %v", want, world.refused)
		}
		return nil
	})
}
