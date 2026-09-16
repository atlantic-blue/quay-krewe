//go:build integration

package sandbox_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox/sandboxtest"
)

// TestTheAppleBackendKeepsTheProviderContract runs the same contract the Docker backend runs, against
// Apple's container tool, so the two backends cannot drift.
//
// It skips where that tool is not installed, which is every machine that is not a macOS one, and the
// continuous integration runners are Linux. So a green pipeline says nothing about this backend. What
// it does say is that the contract itself runs, because Docker runs the same cases beside it.
//
// To run it, on a macOS machine with the tool installed and `container system start` done:
//
//	go test -tags=integration -count=1 -v -run TestTheAppleBackendKeepsTheProviderContract ./internal/sandbox/
func TestTheAppleBackendKeepsTheProviderContract(t *testing.T) {
	sandboxtest.RunConformance(t, sandboxtest.Backend{
		Kind:  sandbox.KindApple,
		Image: "busybox:latest",
		New:   func(opts sandbox.Options) sandbox.Provider { return sandbox.AppleProvider(opts) },
		Available: func() (bool, string) {
			if _, err := exec.LookPath("container"); err != nil {
				return false, "container is not on the path, and it installs on macOS only"
			}
			// The tool talks to a service on the machine, and every command fails while that
			// service is down. A skip that said "no containers" would read as a pass.
			if err := exec.Command("container", "list", "--all", "--quiet").Run(); err != nil {
				return false, "the container service does not answer: run `container system start`"
			}
			return true, ""
		},
		Stop: func(ctx context.Context, name string) error {
			return said(exec.CommandContext(ctx, "container", "stop", name))
		},
		Held: func(ctx context.Context, name string) (bool, error) {
			out, err := exec.CommandContext(ctx, "container", "list", "--all", "--quiet").Output()
			if err != nil {
				return false, err
			}
			for _, held := range strings.Fields(string(out)) {
				if held == name {
					return true, nil
				}
			}
			return false, nil
		},
		Start: func(ctx context.Context, name string) error {
			return said(exec.CommandContext(ctx, "container", "run", "--detach", "--name", name,
				"busybox:latest", "sleep", "600"))
		},
	})
}
