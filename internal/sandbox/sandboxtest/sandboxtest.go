// Package sandboxtest holds the conformance suite every sandbox.Provider that isolates a session
// must pass.
//
// It exists so the backends cannot drift. A behaviour asserted here is asserted against each of
// them: the container backend against a real daemon in the integration tier, the macOS backend
// against a fake tart in the unit tier and against a real one on an Apple machine. Anything that
// passes for one and not another is a defect in that backend, not a difference the control plane is
// allowed to care about.
//
// The host backend is deliberately outside it. A local sandbox is the host, so it holds no state of
// its own, answers Existing with false and Stranded with nothing, and those are the answers it is
// meant to give. It is a stopgap rather than an isolated environment.
//
// Two sandboxes live at once here and no more. Apple's licence permits two macOS guests on one host,
// so a suite that wanted three would wait for a guest that is never coming.
package sandboxtest

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
)

// Patience is how long one behaviour has. A container starts in about a second and a macOS guest
// boots in tens of seconds, so the figure is the slow backend's.
const Patience = 5 * time.Minute

// Session identifiers, each exactly as long as a real one. A name of any other shape is read as
// nothing of the system's, so Stranded would not list it and a drain would walk past it.
const (
	first  = "c04f0f0a4ce0000000000001"
	second = "c04f0f0a4ce0000000000002"
	absent = "c04f0f0a4ce0000000000003"
)

// RunConformance runs the whole contract against a backend. newProvider must hand back a provider
// over an environment no other subtest can see.
func RunConformance(t *testing.T, newProvider func(t *testing.T) sandbox.Provider) {
	t.Helper()

	t.Run("a sandbox runs a command and gives back what it said", func(t *testing.T) {
		ctx, provider := patient(t), newProvider(t)
		box := create(ctx, t, provider, first)

		if said := output(ctx, t, box, "echo", "hi from the sandbox"); said != "hi from the sandbox" {
			t.Fatalf("the sandbox said %q, want 'hi from the sandbox'", said)
		}
	})

	t.Run("a command with no words is refused", func(t *testing.T) {
		ctx, provider := patient(t), newProvider(t)
		box := create(ctx, t, provider, first)

		if _, err := box.Exec(ctx, sandbox.Spec{}); err == nil {
			t.Fatal("a command with no words ran, and the backend has nothing to run")
		}
	})

	t.Run("a command runs in the directory it was given", func(t *testing.T) {
		ctx, provider := patient(t), newProvider(t)
		box := create(ctx, t, provider, first)

		proc, err := box.Exec(ctx, sandbox.Spec{Argv: []string{"pwd"}, Workdir: "/tmp"})
		if err != nil {
			t.Fatalf("run pwd in /tmp: %v", err)
		}
		if said := wait(t, proc); said != "/tmp" {
			t.Fatalf("the command ran in %q, want /tmp", said)
		}
	})

	t.Run("a command is given the environment it was told to carry", func(t *testing.T) {
		ctx, provider := patient(t), newProvider(t)
		box := create(ctx, t, provider, first)

		proc, err := box.Exec(ctx, sandbox.Spec{
			Argv: []string{"sh", "-c", `printf '%s' "$QC_CONFORMANCE"`},
			Env:  []string{"QC_CONFORMANCE=carried"},
		})
		if err != nil {
			t.Fatalf("run a command with an environment: %v", err)
		}
		if said := wait(t, proc); said != "carried" {
			t.Fatalf("the command read %q, want 'carried'", said)
		}
	})

	t.Run("a sandbox carries the environment it was created with", func(t *testing.T) {
		ctx, provider := patient(t), newProvider(t)
		box := createWith(ctx, t, provider, sandbox.Config{ID: first, Env: []string{"QC_CONFORMANCE_CARRIED=yes"}})

		proc, err := box.Exec(ctx, sandbox.Spec{Argv: []string{"sh", "-c", `printf '%s' "$QC_CONFORMANCE_CARRIED"`}})
		if err != nil {
			t.Fatalf("read the sandbox own environment: %v", err)
		}
		// The subscription token rides on this. A sandbox that drops it runs every command logged out.
		if said := wait(t, proc); said != "yes" {
			t.Fatalf("the sandbox read %q from the environment it was created with, want yes", said)
		}
	})

	t.Run("a second create adopts the sandbox the session already has", func(t *testing.T) {
		ctx, provider := patient(t), newProvider(t)
		create(ctx, t, provider, first)

		again, err := provider.Create(ctx, sandbox.Config{ID: first})
		if err != nil {
			t.Fatalf("create for a session that already has one: %v", err)
		}
		if said := output(ctx, t, again, "echo", "adopted"); said != "adopted" {
			t.Fatalf("the adopted sandbox said %q, want it usable", said)
		}
		if held := stranded(ctx, t, provider, first); held != 1 {
			t.Fatalf("session %s holds %d sandboxes, want 1: a second one costs a machine and finds an empty directory",
				first, held)
		}
	})

	t.Run("the system reaches into the sandbox a session already has", func(t *testing.T) {
		ctx, provider := patient(t), newProvider(t)
		create(ctx, t, provider, first)

		box, found, err := provider.Existing(ctx, first)
		if err != nil {
			t.Fatalf("Existing: %v", err)
		}
		if !found {
			t.Fatalf("the system reaches into session %s and finds nothing, while its sandbox is up", first)
		}
		if said := output(ctx, t, box, "echo", "reached"); said != "reached" {
			t.Fatalf("the sandbox reached into said %q, want it usable", said)
		}
		if named, says := box.(sandbox.Named); !says || !strings.HasSuffix(named.Name(), first) {
			t.Fatalf("the sandbox calls itself %v, and an attach opens that name", box)
		}
	})

	t.Run("the system reaches into a session with no sandbox and makes none", func(t *testing.T) {
		ctx, provider := patient(t), newProvider(t)

		box, found, err := provider.Existing(ctx, absent)
		if err != nil {
			t.Fatalf("Existing for a session with no sandbox: %v, want no error", err)
		}
		if found || box != nil {
			t.Fatalf("Existing handed back %v for a session with no sandbox", box)
		}
		if held := stranded(ctx, t, provider, absent); held != 0 {
			t.Fatalf("looking into session %s made it %d sandboxes", absent, held)
		}
	})

	t.Run("a sandbox is removed by name, and removing one that is gone is success", func(t *testing.T) {
		ctx, provider := patient(t), newProvider(t)
		create(ctx, t, provider, first)

		if held := stranded(ctx, t, provider, first); held != 1 {
			t.Fatalf("Stranded holds %d sandboxes for %s after a create, want 1", held, first)
		}
		if err := provider.Remove(ctx, first); err != nil {
			t.Fatalf("Remove: %v", err)
		}
		if held := stranded(ctx, t, provider, first); held != 0 {
			t.Fatalf("Stranded still holds %d sandboxes for %s after a remove", held, first)
		}
		if err := provider.Remove(ctx, first); err != nil {
			t.Fatalf("Remove of a sandbox that is gone: %v, want success", err)
		}
		if _, found, err := provider.Existing(ctx, first); err != nil || found {
			t.Fatalf("Existing after a remove = %v, %v, want nothing and no error", found, err)
		}
	})

	t.Run("two sessions get a sandbox each", func(t *testing.T) {
		ctx, provider := patient(t), newProvider(t)
		one, other := create(ctx, t, provider, first), create(ctx, t, provider, second)

		if said := output(ctx, t, one, "echo", "one"); said != "one" {
			t.Fatalf("the first sandbox said %q", said)
		}
		if said := output(ctx, t, other, "echo", "other"); said != "other" {
			t.Fatalf("the second sandbox said %q", said)
		}
		held, err := provider.Stranded(ctx)
		if err != nil {
			t.Fatalf("Stranded: %v", err)
		}
		if !slices.Contains(held, first) || !slices.Contains(held, second) {
			t.Fatalf("Stranded = %v, want both sessions in it", held)
		}
	})

	t.Run("a session with no sandbox has nobody attached and nothing running", func(t *testing.T) {
		ctx, provider := patient(t), newProvider(t)

		attached, err := provider.Attached(ctx, absent)
		if err != nil {
			t.Fatalf("Attached for a session with no sandbox: %v, want no error", err)
		}
		if attached {
			t.Fatalf("session %s has nothing to attach to and reads as attached, so nothing reclaims it", absent)
		}
		running, err := provider.RuntimeRunning(ctx, absent)
		if err != nil {
			t.Fatalf("RuntimeRunning for a session with no sandbox: %v, want no error", err)
		}
		if running {
			t.Fatalf("session %s has no sandbox and reads as running a model", absent)
		}
	})
}

// patient is a context with the slow backend's patience, ended when the subtest is.
func patient(t *testing.T) context.Context {
	t.Helper()
	ctx, done := context.WithTimeout(context.Background(), Patience)
	t.Cleanup(done)
	return ctx
}

// create hands the session a sandbox and takes it away again when the subtest ends, because a
// sandbox nobody removed holds a machine, and on the macOS backend it holds one of two places.
func create(ctx context.Context, t *testing.T, provider sandbox.Provider, session string) sandbox.Sandbox {
	t.Helper()
	return createWith(ctx, t, provider, sandbox.Config{ID: session})
}

// createWith is create for a subtest that needs more of the configuration than a session identifier.
func createWith(ctx context.Context, t *testing.T, provider sandbox.Provider, cfg sandbox.Config) sandbox.Sandbox {
	t.Helper()
	box, err := provider.Create(ctx, cfg)
	if err != nil {
		t.Fatalf("create a sandbox for %s: %v", cfg.ID, err)
	}
	t.Cleanup(func() {
		removal, done := context.WithTimeout(context.Background(), Patience)
		defer done()
		if err := provider.Remove(removal, cfg.ID); err != nil {
			t.Errorf("removing the sandbox of %s: %v", cfg.ID, err)
		}
	})
	return box
}

// output runs a command in the sandbox and answers with what it wrote, trimmed.
func output(ctx context.Context, t *testing.T, box sandbox.Sandbox, argv ...string) string {
	t.Helper()
	proc, err := box.Exec(ctx, sandbox.Spec{Argv: argv})
	if err != nil {
		t.Fatalf("run %v: %v", argv, err)
	}
	return wait(t, proc)
}

// wait reads everything the command wrote and then waits for it, in that order: a command nobody
// drains stops dead as soon as the pipe fills.
func wait(t *testing.T, proc sandbox.Process) string {
	t.Helper()
	said, err := io.ReadAll(proc.Stdout())
	if err != nil {
		t.Fatalf("read what the command said: %v", err)
	}
	if err := proc.Wait(); err != nil {
		t.Fatalf("the command failed: %v: %s", err, proc.Stderr())
	}
	return strings.TrimSpace(string(said))
}

// stranded is how many sandboxes this session holds, which is one after a create and none after a
// remove. More than one is a second machine holding a session's name and an empty directory.
func stranded(ctx context.Context, t *testing.T, provider sandbox.Provider, session string) int {
	t.Helper()
	held, err := provider.Stranded(ctx)
	if err != nil {
		t.Fatalf("Stranded: %v", err)
	}
	count := 0
	for _, one := range held {
		if one == session {
			count++
		}
	}
	return count
}
