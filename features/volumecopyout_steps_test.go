package features_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cucumber/godog"
)

// Steps for the verb that brings a file back out of a volume.
//
// They run the real tool in its own process, because what is specified is what a caller gets: one
// path on standard output, a refusal on standard error, and an exit status.
//
// The folder the file comes back to is made for the scenario, so a copy out never writes anywhere
// the operator keeps anything.

type copiedOutKey struct{}

// copiedOutWorld is the folder on this machine a file comes back to, and what was already in it.
//
// held is kept so a refusal can be checked against the file it was protecting. A refusal that came
// back after the file was written over is not a refusal.
type copiedOutWorld struct {
	folder string
	held   map[string][]byte
}

func copiedOutTo(ctx context.Context) *copiedOutWorld {
	c, _ := ctx.Value(copiedOutKey{}).(*copiedOutWorld)
	return c
}

func initializeVolumeCopyOutSteps(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		folder, err := os.MkdirTemp("", "krewe-copy-out-")
		if err != nil {
			return ctx, err
		}
		sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
			return ctx, os.RemoveAll(folder)
		})
		return context.WithValue(ctx, copiedOutKey{},
			&copiedOutWorld{folder: folder, held: map[string][]byte{}}), nil
	})

	sc.Step(`^that folder on this machine already holds a file called "([^"]*)"$`,
		func(ctx context.Context, name string) error {
			held := copiedOutTo(ctx)
			body := []byte("what was already on this machine")
			if err := os.WriteFile(filepath.Join(held.folder, name), body, 0o666); err != nil {
				return err
			}
			held.held[name] = body
			return nil
		})

	sc.Step(`^the caller copies "([^"]*)" out to the name "([^"]*)"$`,
		func(ctx context.Context, address, name string) error {
			return runTool(ctx, "volume", "cp", address, filepath.Join(copiedOutTo(ctx).folder, name))
		})

	sc.Step(`^the caller copies "([^"]*)" out to a folder on this machine$`,
		func(ctx context.Context, address string) error {
			return runTool(ctx, "volume", "cp", address, copiedOutTo(ctx).folder)
		})

	sc.Step(`^the caller copies "([^"]*)" out to a folder on this machine and means to replace it$`,
		func(ctx context.Context, address string) error {
			return runTool(ctx, "volume", "cp", address, copiedOutTo(ctx).folder, "--replace")
		})

	sc.Step(`^the caller copies "([^"]*)" to the address "([^"]*)"$`,
		func(ctx context.Context, from, to string) error {
			return runTool(ctx, "volume", "cp", from, to)
		})

	sc.Step(`^the file on this machine called "([^"]*)" holds the bytes that were copied$`,
		func(ctx context.Context, name string) error {
			arrived, err := os.ReadFile(filepath.Join(copiedOutTo(ctx).folder, name))
			if err != nil {
				return err
			}
			sent := copiedFrom(ctx).body
			if len(arrived) != len(sent) {
				return fmt.Errorf("the file came out at %d bytes, want the %d that went in",
					len(arrived), len(sent))
			}
			if !bytes.Equal(arrived, sent) {
				return fmt.Errorf("the file came out at the right size, and the bytes are not the ones that went in")
			}
			return nil
		})

	sc.Step(`^the file on this machine called "([^"]*)" is the one that was already there$`,
		func(ctx context.Context, name string) error {
			held := copiedOutTo(ctx)
			arrived, err := os.ReadFile(filepath.Join(held.folder, name))
			if err != nil {
				return err
			}
			if !bytes.Equal(arrived, held.held[name]) {
				return fmt.Errorf("the refusal came back and the file on this machine was written over anyway")
			}
			return nil
		})

	// One path and a newline is the whole of standard output, the way it is on the way in. The folder
	// is made for the scenario, so the path cannot be written in the feature file and is built here.
	sc.Step(`^standard output names the file on this machine called "([^"]*)"$`,
		func(ctx context.Context, name string) error {
			want := filepath.Join(copiedOutTo(ctx).folder, name) + "\n"
			if got := toolFrom(ctx).stdout; got != want {
				return fmt.Errorf("standard output is %q, want the one path %q", got, want)
			}
			return nil
		})
}
