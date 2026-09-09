package features_test

import (
	"context"
	"fmt"
	"os"

	"github.com/cucumber/godog"
)

// Steps for the verb that takes a file out of a volume.
//
// They run the real tool in its own process, because what is specified is what a caller gets: the
// path the file was at on standard output, a refusal on standard error, and an exit status.
//
// What went is then read through the mount the container runtime is given, rather than at a path this
// file builds. A delete of a directory nobody binds would pass against a path assembled correctly and
// read by nothing.

func initializeVolumeDeleteSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the caller deletes "([^"]*)"$`, func(ctx context.Context, address string) error {
		return runTool(ctx, "volume", "delete", address)
	})

	// The path is mapped first and the file is looked for second. A step that read "there is no file"
	// off a failure of the whole walk would pass just as well against a mount that was never made.
	sc.Step(`^a sandbox of that workspace reads no file at "([^"]*)"$`, func(ctx context.Context, at string) error {
		read, err := whereASandboxReads(ctx, at)
		if err != nil {
			return err
		}
		switch _, err := os.Stat(read); {
		case err == nil:
			return fmt.Errorf("a sandbox still reads a file at %q, on this machine at %q", at, read)
		case !os.IsNotExist(err):
			return err
		}
		return nil
	})
}
