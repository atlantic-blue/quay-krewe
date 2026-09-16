//go:build integration

package sandbox_test

import (
	"context"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox/sandboxtest"
)

// TestTheContainerdBackendKeepsTheProviderContract runs the same contract the Docker backend runs,
// against containerd through nerdctl, so the two backends cannot drift.
//
// It skips where nerdctl is not installed, and the continuous integration runners have none, so a
// green pipeline says nothing about this backend. What it does say is that the contract itself runs,
// because Docker runs the same cases beside it.
//
// To run it, on a Linux host with containerd, or on a Mac through Lima or Colima:
//
//	go test -tags=integration -count=1 -v -run TestTheContainerdBackendKeepsTheProviderContract ./internal/sandbox/
func TestTheContainerdBackendKeepsTheProviderContract(t *testing.T) {
	sandboxtest.RunConformance(t, sandboxtest.Backend{
		Kind:  sandbox.KindContainerd,
		Image: "busybox:latest",
		New: func(opts sandbox.Options) sandbox.Provider {
			return sandbox.ContainerdProvider{Options: opts}
		},
		Available: func() (bool, string) {
			if _, err := exec.LookPath(sandbox.ContainerdBinary); err != nil {
				return false, sandbox.ContainerdBinary + " is not on the path"
			}
			// The command line talks to a containerd on the machine, and every verb fails while
			// that daemon is down. A skip that said "no containers" would read as a pass.
			if err := exec.Command(sandbox.ContainerdBinary, "ps", "--all", "--quiet").Run(); err != nil {
				return false, "containerd does not answer: start it, or start the Lima or Colima machine"
			}
			return true, ""
		},
		Stop: func(ctx context.Context, name string) error {
			return said(exec.CommandContext(ctx, sandbox.ContainerdBinary, "stop", name))
		},
		Held: func(ctx context.Context, name string) (bool, error) {
			out, err := exec.CommandContext(ctx, sandbox.ContainerdBinary,
				"ps", "--all", "--format", "{{.Names}}").Output()
			if err != nil {
				return false, err
			}
			return slices.Contains(strings.Fields(string(out)), name), nil
		},
		Start: func(ctx context.Context, name string) error {
			return said(exec.CommandContext(ctx, sandbox.ContainerdBinary, "run", "--detach",
				"--name", name, "busybox:latest", "sleep", "600"))
		},
	})
}
