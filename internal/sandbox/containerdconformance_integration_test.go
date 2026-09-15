//go:build integration

package sandbox_test

import (
	"os/exec"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox/sandboxtest"
)

// TestContainerdKeepsTheProviderContract runs the same contract against containerd, so the two
// backends cannot drift: a session is created, found again, listed, removed and asked what is
// running in it by one set of cases.
//
// It needs nerdctl on the path and a containerd to talk to. Neither is on a continuous integration
// runner here, so this run is the operator's, on a Linux host or on a Mac with Lima or Colima. The
// skip says so by name rather than passing quietly, because a case that did not run reads exactly
// like one that did.
func TestContainerdKeepsTheProviderContract(t *testing.T) {
	if _, err := exec.LookPath(sandbox.ContainerdBinary); err != nil {
		t.Skipf("no %s on the path, so the containerd contract did not run here: %v",
			sandbox.ContainerdBinary, err)
	}
	sandboxtest.RunConformance(t, func(_ *testing.T, storage sandbox.Storage) sandbox.Provider {
		return sandbox.ContainerdProvider{
			Options: sandbox.Options{Image: conformanceImage, Storage: storage},
		}
	})
}
