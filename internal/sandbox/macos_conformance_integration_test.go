//go:build integration

package sandbox_test

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"
	"time"

	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox/sandboxtest"
)

// TestTheMacOSBackendKeepsTheProviderContract runs the same contract the Docker backend runs, against
// a real macOS guest, so the two backends cannot drift.
//
// It skips where tart is not installed, which is every machine that is not an Apple one, and the
// continuous integration runners are Linux. So a green pipeline says nothing about this backend. What
// it does say is that the contract itself runs, because Docker runs the same cases beside it, and the
// unit tier runs this backend against a stand in for tart.
//
// To run it, on an Apple machine with tart installed:
//
//	QC_TEST_MACOS_IMAGE=ghcr.io/cirruslabs/macos-tahoe-xcode:latest \
//	  go test -tags=integration -count=1 -v -run TestTheMacOSBackendKeepsTheProviderContract ./internal/sandbox/
//
// The image is named by configuration rather than chosen here, because it is tens of gigabytes and a
// test must not pull one by surprise.
func TestTheMacOSBackendKeepsTheProviderContract(t *testing.T) {
	image := macOSImage()
	sandboxtest.RunConformance(t, sandboxtest.Backend{
		Kind:  sandbox.KindMacOS,
		Image: image,
		New: func(opts sandbox.Options) sandbox.Provider {
			return sandbox.NewMacOSProvider(opts)
		},
		Available: func() (bool, string) {
			if _, err := exec.LookPath("tart"); err != nil {
				return false, "tart is not on the path, and it installs on Apple hardware only"
			}
			if image == "" {
				return false, "QC_TEST_MACOS_IMAGE names the guest to clone, and it is not set"
			}
			return true, ""
		},
		// A host directory reaches a guest as a virtio share, and a share arrives under the runtime's
		// own mount point rather than at the path a bind mount would put it. The guest would have to
		// mount it at that path itself, which is the image's job and not this backend's.
		Except: map[string]string{
			"what a session wrote outlives its container": "a guest mounts a share where the runtime " +
				"puts it, so the conversation store and the working directory are not at the paths a " +
				"container has them at",
		},
		Stop: func(ctx context.Context, name string) error {
			return said(exec.CommandContext(ctx, "tart", "stop", name))
		},
		Held: func(ctx context.Context, name string) (bool, error) {
			out, err := exec.CommandContext(ctx, "tart", "list", "--source", "local", "--format", "json").Output()
			if err != nil {
				return false, err
			}
			var listed []struct{ Name string }
			if err := json.Unmarshal(out, &listed); err != nil {
				return false, err
			}
			for _, one := range listed {
				if one.Name == name {
					return true, nil
				}
			}
			return false, nil
		},
		// tart run is the guest's own process rather than a command that returns, so it is started and
		// left, and the guest is up once it answers a command.
		Start: func(ctx context.Context, name string) error {
			if err := said(exec.CommandContext(ctx, "tart", "clone", image, name)); err != nil {
				return err
			}
			if err := exec.Command("tart", "run", "--no-graphics", name).Start(); err != nil {
				return err
			}
			for ctx.Err() == nil {
				if exec.CommandContext(ctx, "tart", "exec", name, "true").Run() == nil {
					return nil
				}
				time.Sleep(time.Second)
			}
			return ctx.Err()
		},
	})
}
