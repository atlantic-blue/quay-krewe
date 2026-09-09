package features_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cucumber/godog"
)

// Steps for the verb that puts a file in a volume.
//
// They run the real tool in its own process, because what is specified is what a caller gets: one
// path on standard output, a refusal on standard error, and an exit status.
//
// What arrived is then read through the mount the container runtime is given, rather than at a path
// this file builds. A copy into a directory nobody binds would pass against a path assembled
// correctly and read by nothing.

type copiedKey struct{}

// copiedWorld is the file a scenario made on the machine, and the bytes in it.
//
// The bytes are kept so an assertion is about the file arriving whole. A file of one repeated byte is
// identical to the file that lost half of itself and got padded, so they are random.
type copiedWorld struct {
	at   string
	body []byte
}

func copiedFrom(ctx context.Context) *copiedWorld {
	c, _ := ctx.Value(copiedKey{}).(*copiedWorld)
	return c
}

func initializeVolumeCopySteps(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, copiedKey{}, &copiedWorld{}), nil
	})

	sc.Step(`^a file of (\d+) bytes on this machine called "([^"]*)"$`,
		func(ctx context.Context, size int, name string) error {
			folder, err := os.MkdirTemp("", "krewe-copy-")
			if err != nil {
				return err
			}
			sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
				return ctx, os.RemoveAll(folder)
			})
			body := make([]byte, size)
			if _, err := rand.Read(body); err != nil {
				return fmt.Errorf("make %d bytes: %w", size, err)
			}
			at := filepath.Join(folder, name)
			if err := os.WriteFile(at, body, 0o666); err != nil {
				return err
			}
			held := copiedFrom(ctx)
			held.at, held.body = at, body
			return nil
		})

	sc.Step(`^the caller copies that file to "([^"]*)"$`, func(ctx context.Context, address string) error {
		return runTool(ctx, "volume", "cp", copiedFrom(ctx).at, address)
	})

	// The same copy as a setup step. A scenario about the second copy then reads as two copies, rather
	// than as one copy and a file that appeared from somewhere.
	sc.Step(`^that file is copied to "([^"]*)"$`, func(ctx context.Context, address string) error {
		if err := runTool(ctx, "volume", "cp", copiedFrom(ctx).at, address); err != nil {
			return err
		}
		if code := toolFrom(ctx).exitCode; code != 0 {
			return fmt.Errorf("the first copy exited %d, saying %q", code, toolFrom(ctx).stderr)
		}
		return nil
	})

	sc.Step(`^the caller copies that file to "([^"]*)" and means to replace it$`,
		func(ctx context.Context, address string) error {
			return runTool(ctx, "volume", "cp", copiedFrom(ctx).at, address, "--replace")
		})

	sc.Step(`^the caller copies the folder that file is in to "([^"]*)"$`,
		func(ctx context.Context, address string) error {
			return runTool(ctx, "volume", "cp", filepath.Dir(copiedFrom(ctx).at), address)
		})

	// One path and a newline is the whole of standard output, because that path goes into the message
	// somebody sends the session. Anything sharing the line has to be edited out by hand.
	sc.Step(`^standard output is the one path "([^"]*)"$`, func(ctx context.Context, want string) error {
		if got := toolFrom(ctx).stdout; got != want+"\n" {
			return fmt.Errorf("standard output is %q, want the one path %q and a newline", got, want)
		}
		return nil
	})

	sc.Step(`^the bytes a sandbox reads at "([^"]*)" are the ones that were copied$`,
		func(ctx context.Context, at string) error {
			read, err := whatASandboxReadsAt(ctx, at)
			if err != nil {
				return err
			}
			arrived, err := os.ReadFile(read)
			if err != nil {
				return err
			}
			sent := copiedFrom(ctx).body
			if len(arrived) != len(sent) {
				return fmt.Errorf("the file arrived at %d bytes, want the %d that were copied",
					len(arrived), len(sent))
			}
			if !bytes.Equal(arrived, sent) {
				return fmt.Errorf("the file arrived at the right size, and the bytes are not the ones that were copied")
			}
			return nil
		})

	sc.Step(`^the file a sandbox reads at "([^"]*)" is (\d+) bytes$`,
		func(ctx context.Context, at string, size int) error {
			read, err := whatASandboxReadsAt(ctx, at)
			if err != nil {
				return err
			}
			info, err := os.Stat(read)
			if err != nil {
				return err
			}
			if info.Size() != int64(size) {
				return fmt.Errorf("the file is %d bytes, want %d", info.Size(), size)
			}
			return nil
		})
}
