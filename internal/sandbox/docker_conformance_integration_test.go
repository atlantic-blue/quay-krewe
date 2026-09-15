//go:build integration

package sandbox_test

import (
	"context"
	"fmt"
	"os/exec"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox/sandboxtest"
)

// TestTheDockerBackendKeepsTheProviderContract runs the whole contract against a real daemon.
//
// The cases live in sandboxtest because a second backend runs the same ones. Anything here that is
// about the sandbox image rather than about the runtime stays in docker_integration_test.go.
func TestTheDockerBackendKeepsTheProviderContract(t *testing.T) {
	sandboxtest.RunConformance(t, sandboxtest.Backend{
		Kind:  sandbox.KindDocker,
		Image: "busybox:latest",
		New:   func(opts sandbox.Options) sandbox.Provider { return sandbox.DockerProvider(opts) },
		// No skip. This tier runs where the daemon is, and a daemon that is down is a failure to
		// read rather than a reason to report a pass over tests that never ran.
		Available: func() (bool, string) { return true, "" },
		Stop: func(ctx context.Context, name string) error {
			return said(exec.CommandContext(ctx, "docker", "stop", name))
		},
		Held: func(ctx context.Context, name string) (bool, error) {
			return exec.CommandContext(ctx, "docker", "inspect", name).Run() == nil, nil
		},
		Start: func(ctx context.Context, name string) error {
			return said(exec.CommandContext(ctx, "docker", "run", "-d", "--name", name,
				"busybox:latest", "sleep", "600"))
		},
	})
}

// said runs a command and carries what it printed into the failure, because the reason a runtime
// refused is in its output rather than in the exit status.
func said(cmd *exec.Cmd) error {
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", cmd.Args[0], err, out)
	}
	return nil
}
