package features_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/atlantic-blue/quay-krewe/internal/workspace"
	"github.com/cucumber/godog"
)

// volumeWorld is one scenario's reading of a volume address. It holds no control plane: an address
// says what somebody typed, and reading it needs nothing that is running.
type volumeWorld struct {
	address workspace.VolumePath
	err     error
}

type volumeKey struct{}

func volumeFrom(ctx context.Context) *volumeWorld {
	v, _ := ctx.Value(volumeKey{}).(*volumeWorld)
	return v
}

// read answers the address when it was read, and an error when the scenario expected one and it was
// not refused, so a step never reads the zero value as an answer.
func (v *volumeWorld) read() (workspace.VolumePath, error) {
	if v.err != nil {
		return workspace.VolumePath{}, fmt.Errorf("the address was refused: %w", v.err)
	}
	return v.address, nil
}

// initializeVolumeSteps registers the steps for reading an address into a volume.
func initializeVolumeSteps(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, volumeKey{}, &volumeWorld{}), nil
	})

	sc.Step(`^the operator reads the volume address "([^"]*)"$`, func(ctx context.Context, typed string) error {
		v := volumeFrom(ctx)
		v.address, v.err = workspace.ParseVolumePath(typed)
		return nil
	})

	sc.Step(`^the address names the workspace "([^"]*)"$`, func(ctx context.Context, want string) error {
		return volumeFrom(ctx).level("workspace", want, func(a workspace.VolumePath) string { return a.Workspace })
	})

	sc.Step(`^the address names the project "([^"]*)"$`, func(ctx context.Context, want string) error {
		return volumeFrom(ctx).level("project", want, func(a workspace.VolumePath) string { return a.Project })
	})

	sc.Step(`^the address names the session "([^"]*)"$`, func(ctx context.Context, want string) error {
		return volumeFrom(ctx).level("session", want, func(a workspace.VolumePath) string { return a.Session })
	})

	sc.Step(`^the address names the file "([^"]*)"$`, func(ctx context.Context, want string) error {
		return volumeFrom(ctx).level("file", want, func(a workspace.VolumePath) string { return a.Key })
	})

	sc.Step(`^the address names no file$`, func(ctx context.Context) error {
		address, err := volumeFrom(ctx).read()
		if err != nil {
			return err
		}
		if address.HasKey() {
			return fmt.Errorf("the address names the file %q, want none", address.Key)
		}
		return nil
	})

	sc.Step(`^the reading is settled$`, func(ctx context.Context) error {
		return volumeFrom(ctx).marked("")
	})

	sc.Step(`^the reading is a guess from the project down$`, func(ctx context.Context) error {
		return volumeFrom(ctx).marked(workspace.LevelProject)
	})

	sc.Step(`^the volume address is refused$`, func(ctx context.Context) error {
		v := volumeFrom(ctx)
		if v.err == nil {
			return fmt.Errorf("the address was read as %#v, want a refusal", v.address)
		}
		return nil
	})

	sc.Step(`^the refusal names a session address$`, func(ctx context.Context) error {
		return volumeFrom(ctx).refusalSays("<session>")
	})

	sc.Step(`^the refusal says the directory holds the tokens$`, func(ctx context.Context) error {
		return volumeFrom(ctx).refusalSays("tokens")
	})
}

// level checks one part of the address, and says which part it was reading when it did not match.
func (v *volumeWorld) level(what, want string, of func(workspace.VolumePath) string) error {
	address, err := v.read()
	if err != nil {
		return err
	}
	if got := of(address); got != want {
		return fmt.Errorf("the address names the %s %q, want %q", what, got, want)
	}
	return nil
}

// marked checks how far down the reading is proven, which is what the resolver treats differently. A
// scenario says which it expects rather than leaving it to be read off the levels.
func (v *volumeWorld) marked(want workspace.VolumeLevel) error {
	address, err := v.read()
	if err != nil {
		return err
	}
	if address.Ambiguous != want {
		return fmt.Errorf("the reading is marked %q, want %q", address.Ambiguous, want)
	}
	return nil
}

// refusalSays holds a refusal to saying what to do next, which is the whole reason a word is
// reserved rather than quietly read as something else.
func (v *volumeWorld) refusalSays(want string) error {
	if v.err == nil {
		return fmt.Errorf("the address was read as %#v, want a refusal", v.address)
	}
	if !strings.Contains(v.err.Error(), want) {
		return fmt.Errorf("the refusal says %q, want it to say %q", v.err, want)
	}
	return nil
}
