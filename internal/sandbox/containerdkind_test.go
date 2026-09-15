package sandbox_test

import (
	"strings"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
)

// TestASystemToldContainerdRunsSessionsOnContainerd. The kind is one word in the configuration, and
// what the system reports it is running on is read from the same place it is built from, so the two
// cannot drift.
func TestASystemToldContainerdRunsSessionsOnContainerd(t *testing.T) {
	kind, err := sandbox.ResolveKind(sandbox.KindContainerd)
	if err != nil {
		t.Fatalf("ResolveKind: %v", err)
	}
	if kind != "containerd" {
		t.Fatalf("a system told containerd reports %q", kind)
	}

	provider, err := sandbox.NewProvider(sandbox.KindContainerd, sandbox.Options{
		Image: "img", Network: "quaycrew_default", SessionNetwork: "quaycrew_sessions", Memory: "4g",
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	backend, isContainerd := provider.(sandbox.ContainerdProvider)
	if !isContainerd {
		t.Fatalf("NewProvider gave %T, want the containerd backend", provider)
	}
	// The options are carried across whole. A field that arrived empty is a session on no network, or
	// a session with no memory limit, and neither says anything at the time.
	if backend.Image != "img" || backend.SessionNetwork != "quaycrew_sessions" ||
		backend.Network != "quaycrew_default" || backend.Memory != "4g" {
		t.Fatalf("the backend was built with %+v, want what it was configured with", backend.Options)
	}
}

// TestDockerIsStillWhatASystemGetsWhenNobodyChoseARuntime. The new kind adds a choice and takes none
// away: a system that says nothing, and a system that says docker, run sessions the way they did.
func TestDockerIsStillWhatASystemGetsWhenNobodyChoseARuntime(t *testing.T) {
	for _, configured := range []string{"", sandbox.KindDocker} {
		kind, err := sandbox.ResolveKind(configured)
		if err != nil {
			t.Fatalf("ResolveKind(%q): %v", configured, err)
		}
		if kind != sandbox.KindDocker {
			t.Fatalf("a system configured with %q runs sessions on %q, want docker", configured, kind)
		}
		provider, err := sandbox.NewProvider(configured, sandbox.Options{Image: "img"})
		if err != nil {
			t.Fatalf("NewProvider(%q): %v", configured, err)
		}
		if _, isDocker := provider.(sandbox.DockerProvider); !isDocker {
			t.Fatalf("a system configured with %q got %T, want the Docker backend", configured, provider)
		}
	}
}

// TestARuntimeTheSystemDoesNotHaveIsRefused. A kind nobody recognises must stop the system starting
// rather than fall back to the default: a system asked for one runtime and quietly running another
// looks exactly like a system that was asked for that one.
func TestARuntimeTheSystemDoesNotHaveIsRefused(t *testing.T) {
	for _, asked := range []string{"podman", "containerD", "nerdctl", "kubernetes"} {
		if _, err := sandbox.ResolveKind(asked); err == nil {
			t.Errorf("%q was accepted as a runtime this system has", asked)
		} else if !strings.Contains(err.Error(), asked) {
			t.Errorf("the refusal does not name what was asked for: %v", err)
		}
		provider, err := sandbox.NewProvider(asked, sandbox.Options{Image: "img"})
		if err == nil {
			t.Errorf("a system asked for %q was given %T", asked, provider)
		}
		if provider != nil {
			t.Errorf("a refused kind came back with a provider: %T", provider)
		}
	}
}
