//go:build integration

package sandbox_test

import (
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox/sandboxtest"
)

// ConformanceImage is what a case in the suite runs in. It needs a shell, a cat and an echo, and
// nothing else.
const conformanceImage = "busybox:latest"

// TestDockerKeepsTheProviderContract runs the contract every backend keeps against Docker, which is
// the backend every other one is held against. It is what makes the suite evidence rather than a
// promise: this run happens in continuous integration, so a case that proves nothing here proves
// nothing anywhere.
func TestDockerKeepsTheProviderContract(t *testing.T) {
	sandboxtest.RunConformance(t, func(_ *testing.T, storage sandbox.Storage) sandbox.Provider {
		return sandbox.DockerProvider{Image: conformanceImage, Storage: storage}
	})
}
