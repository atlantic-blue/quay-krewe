package sandbox_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
)

func TestLocalProviderExec(t *testing.T) {
	box, err := sandbox.LocalProvider{}.Create(context.Background(), sandbox.Config{ID: "sess-1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	proc, err := box.Exec(context.Background(), sandbox.Spec{Argv: []string{"echo", "hi there"}})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	out, err := io.ReadAll(proc.Stdout())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := proc.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}
	if strings.TrimSpace(string(out)) != "hi there" {
		t.Fatalf("stdout = %q, want 'hi there'", string(out))
	}
	if err := box.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestLocalExecRejectsEmptyArgv(t *testing.T) {
	box, _ := sandbox.LocalProvider{}.Create(context.Background(), sandbox.Config{ID: "s"})
	if _, err := box.Exec(context.Background(), sandbox.Spec{}); err == nil {
		t.Fatal("Exec with empty argv = nil error, want error")
	}
}

func TestFakeProvider(t *testing.T) {
	provider := &sandbox.FakeProvider{Output: "canned"}
	box, err := provider.Create(context.Background(), sandbox.Config{ID: "sess-1", Workspace: "ws-1", Project: "prj-1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(provider.Created) != 1 || provider.Created[0].ID != "sess-1" {
		t.Fatalf("Created not recorded: %+v", provider.Created)
	}
	proc, _ := box.Exec(context.Background(), sandbox.Spec{Argv: []string{"claude"}})
	out, _ := io.ReadAll(proc.Stdout())
	if string(out) != "canned" {
		t.Fatalf("out = %q", string(out))
	}
	if err := box.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !provider.Boxes[0].Closed {
		t.Fatal("Close did not mark the sandbox closed")
	}
}

func TestNewProvider(t *testing.T) {
	if p, err := sandbox.NewProvider("", sandbox.Options{Image: "img"}); err != nil {
		t.Fatalf("default: %v", err)
	} else if _, ok := p.(sandbox.DockerProvider); !ok {
		t.Fatalf("default should be DockerProvider, got %T", p)
	}
	if p, err := sandbox.NewProvider("local", sandbox.Options{}); err != nil {
		t.Fatalf("local: %v", err)
	} else if _, ok := p.(sandbox.LocalProvider); !ok {
		t.Fatalf("local should be LocalProvider, got %T", p)
	}
	if _, err := sandbox.NewProvider("nope", sandbox.Options{}); err == nil {
		t.Fatal("unknown kind = nil error, want error")
	}
}

// TestFakeProviderAdoptsASessionsSandbox holds the double to what Docker does. It used to hand out
// two sandboxes for one session id, so the suite stayed green while the real daemon refused the
// duplicate name, which is the exact shape of false green this project keeps finding.
func TestFakeProviderAdoptsASessionsSandbox(t *testing.T) {
	provider := &sandbox.FakeProvider{Output: "hi"}

	first, err := provider.Create(context.Background(), sandbox.Config{ID: "s1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	again, err := provider.Create(context.Background(), sandbox.Config{ID: "s1"})
	if err != nil {
		t.Fatalf("Create again: %v", err)
	}
	if first != again {
		t.Fatal("the fake made a second sandbox for one session, which Docker refuses")
	}

	// Closed is gone, so the next one is genuinely new.
	if err := first.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	replacement, err := provider.Create(context.Background(), sandbox.Config{ID: "s1"})
	if err != nil {
		t.Fatalf("Create after close: %v", err)
	}
	if replacement == first {
		t.Fatal("a closed sandbox was handed back, want a fresh one")
	}
}

// TestASandboxJoinsNoNetworkByDefault. Neither network is assumed: a system run outside the compose
// stack may have no network to put a session on, and the system's own network, which carries the store
// and the broker, is a widening the operator asks for.
func TestASandboxJoinsNoNetworkByDefault(t *testing.T) {
	plain := sandbox.DockerProvider{Image: "krewe-sandbox-claude:local"}
	if plain.Network != "" || plain.SessionNetwork != "" {
		t.Fatalf("a sandbox joins %q and %q with nothing configured", plain.Network, plain.SessionNetwork)
	}
}

// TestOptionsCarryBothNetworksThroughToTheBackend: the provider is built by converting the options
// across, so a field added to one and not the other silently does nothing. Both, because a session
// that never reaches the system is exactly what this pair of fields exists to stop.
func TestOptionsCarryBothNetworksThroughToTheBackend(t *testing.T) {
	provider, err := sandbox.NewProvider(sandbox.KindDocker, sandbox.Options{
		Image: "img", Network: "quaycrew_default", SessionNetwork: "quaycrew_sessions",
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	docker, isDocker := provider.(sandbox.DockerProvider)
	if !isDocker {
		t.Fatalf("NewProvider gave %T, want the Docker backend", provider)
	}
	if docker.Network != "quaycrew_default" {
		t.Fatalf("the driver joins %q, want the network it was configured with", docker.Network)
	}
	if docker.SessionNetwork != "quaycrew_sessions" {
		t.Fatalf("a session joins %q, want the network it was configured with", docker.SessionNetwork)
	}
}

// TestResolveKindNamesTheBackendAndRefusesAnythingElse.
//
// Anything that reports the configuration reads the answer from here, so what an operator is told
// cannot drift from what the system built. A kind nobody knows is refused rather than filled in: a
// system that fell back to the default would isolate every session in something the operator did not
// choose, and would report that choice as theirs.
func TestResolveKindNamesTheBackendAndRefusesAnythingElse(t *testing.T) {
	for kind, want := range map[string]string{
		"":                 sandbox.KindDocker,
		sandbox.KindDocker: sandbox.KindDocker,
		sandbox.KindLocal:  sandbox.KindLocal,
		sandbox.KindApple:  sandbox.KindApple,
		sandbox.KindMacOS:  sandbox.KindMacOS,
	} {
		got, err := sandbox.ResolveKind(kind)
		if err != nil {
			t.Errorf("ResolveKind(%q): %v", kind, err)
			continue
		}
		if got != want {
			t.Errorf("ResolveKind(%q) = %q, want %q", kind, got, want)
		}
	}

	got, err := sandbox.ResolveKind("podman")
	if err == nil {
		t.Fatal("a kind the system cannot build was accepted")
	}
	if got != "" {
		t.Errorf("an unknown kind resolved to %q, so a system would run a backend nobody asked for", got)
	}
	if !strings.Contains(err.Error(), "podman") {
		t.Errorf("the refusal does not name the word it refused: %v", err)
	}
}

// TestNewProviderBuildsTheAppleBackendWithWhatItWasConfiguredWith. The provider is built by converting
// the options across, so a field added to one backend and not the other silently does nothing. The two
// networks and the memory are here because a session that reaches nothing, or that is given a figure
// nobody set, looks exactly like a working session until it runs.
func TestNewProviderBuildsTheAppleBackendWithWhatItWasConfiguredWith(t *testing.T) {
	provider, err := sandbox.NewProvider(sandbox.KindApple, sandbox.Options{
		Image: "img", Network: "quaycrew_default", SessionNetwork: "quaycrew_sessions", Memory: "4g",
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	apple, isApple := provider.(sandbox.AppleProvider)
	if !isApple {
		t.Fatalf("NewProvider gave %T, want the Apple container backend", provider)
	}
	if apple.Image != "img" {
		t.Errorf("the backend runs %q, want the image it was configured with", apple.Image)
	}
	if apple.Network != "quaycrew_default" {
		t.Errorf("the driver joins %q, want the network it was configured with", apple.Network)
	}
	if apple.SessionNetwork != "quaycrew_sessions" {
		t.Errorf("a session joins %q, want the network it was configured with", apple.SessionNetwork)
	}
	if apple.Memory != "4g" {
		t.Errorf("a session may take %q, want the figure it was configured with", apple.Memory)
	}
}
