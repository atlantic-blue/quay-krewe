//go:build integration

package sandbox_test

import (
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox/sandboxtest"
)

// TestDockerProviderConformance holds the container backend to the contract every isolating backend
// keeps. It is the half of the suite that runs against a real daemon, so the macOS backend's run
// against a stand in has something true to be held beside.
func TestDockerProviderConformance(t *testing.T) {
	sandboxtest.RunConformance(t, func(*testing.T) sandbox.Provider {
		return sandbox.DockerProvider{Image: "busybox:latest"}
	})
}
