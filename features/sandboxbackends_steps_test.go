package features_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/cucumber/godog"
)

// The backend scenarios touch the control plane nowhere. What they prove is the choice a system makes
// when it reads its own configuration, which is a question with an answer before any container exists.
// So they ask the sandbox package the question the control plane asks it at startup.

type backendWorld struct {
	// configured is the word the operator set, and empty is a system that named none.
	configured string
	// reported is the runtime the system says it runs, and built is the backend it built.
	reported string
	built    sandbox.Provider
	err      error
}

type backendKey struct{}

func backendFrom(ctx context.Context) *backendWorld {
	b, _ := ctx.Value(backendKey{}).(*backendWorld)
	return b
}

func initializeSandboxBackendSteps(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, backendKey{}, &backendWorld{}), nil
	})

	sc.Step(`^the system names no runtime for a session$`, func(ctx context.Context) error {
		backendFrom(ctx).configured = ""
		return nil
	})

	sc.Step(`^the system is configured to isolate a session with "([^"]*)"$`, func(ctx context.Context, word string) error {
		backendFrom(ctx).configured = word
		return nil
	})

	// Both questions, because the system asks both: one names what it reports to an operator, and the
	// other builds what a session actually runs in. A scenario that asked only one would pass over a
	// system that reported one runtime and ran another.
	sc.Step(`^the system builds the backend a session runs in$`, func(ctx context.Context) error {
		b := backendFrom(ctx)
		b.reported, b.err = sandbox.ResolveKind(b.configured)
		built, err := sandbox.NewProvider(b.configured, sandbox.Options{Image: "an-image"})
		if b.err == nil {
			b.err = err
		}
		b.built = built
		return nil
	})

	sc.Step(`^it reports the runtime "([^"]*)"$`, func(ctx context.Context, want string) error {
		b := backendFrom(ctx)
		if b.err != nil {
			return fmt.Errorf("the system refused to run %q: %w", b.configured, b.err)
		}
		if b.reported != want {
			return fmt.Errorf("the system reports %q, want %q", b.reported, want)
		}
		return nil
	})

	sc.Step(`^it builds the (.+) backend$`, func(ctx context.Context, named string) error {
		b := backendFrom(ctx)
		if b.err != nil {
			return fmt.Errorf("the system built nothing: %w", b.err)
		}
		wanted := map[string]string{
			"Docker":          "sandbox.DockerProvider",
			"Apple container": "sandbox.AppleProvider",
			"host":            "sandbox.LocalProvider",
		}[named]
		if got := fmt.Sprintf("%T", b.built); got != wanted {
			return fmt.Errorf("the system built %s, want the %s backend", got, named)
		}
		return nil
	})

	sc.Step(`^it refuses to build a backend$`, func(ctx context.Context) error {
		b := backendFrom(ctx)
		if b.err == nil {
			return fmt.Errorf("the system accepted %q and built %T", b.configured, b.built)
		}
		if b.built != nil {
			return fmt.Errorf("the system refused %q and handed out %T anyway", b.configured, b.built)
		}
		return nil
	})

	sc.Step(`^the refusal names the word "([^"]*)"$`, func(ctx context.Context, word string) error {
		b := backendFrom(ctx)
		if b.err == nil {
			return fmt.Errorf("nothing was refused")
		}
		if !strings.Contains(b.err.Error(), word) {
			return fmt.Errorf("the refusal does not say which word it refused: %v", b.err)
		}
		return nil
	})

	// A refusal that still named a runtime would be read as a choice by anything that reports the
	// configuration, and the operator would be told they picked it.
	sc.Step(`^it reports no runtime at all$`, func(ctx context.Context) error {
		if reported := backendFrom(ctx).reported; reported != "" {
			return fmt.Errorf("the system reports the runtime %q for a word it refused", reported)
		}
		return nil
	})
}
