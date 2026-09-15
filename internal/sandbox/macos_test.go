package sandbox_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/atlantic-blue/quay-krewe/internal/capacity"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox/sandboxtest"
)

// The macOS backend, driven end to end against a fake tart.
//
// What this proves: the backend asks tart the right questions, reads its listing, tells a machine
// that is not there from one that is stopped, carries a working directory and an environment into a
// command, and hands out no more guests than Apple's licence permits.
//
// What it does not prove: that any of it works on an Apple machine. Nothing boots here, no image is
// pulled, and a command runs on this host rather than in a guest. The same suite runs against a real
// tart in macos_integration_test.go, which needs an Apple machine to run at all.

// TestMacOSProviderConformance holds the macOS backend to the same contract the container backend
// keeps, so the two cannot drift.
func TestMacOSProviderConformance(t *testing.T) {
	tart := tartBinary(t)
	sandboxtest.RunConformance(t, func(t *testing.T) sandbox.Provider {
		tartHome(t)
		return &sandbox.MacOSProvider{Image: "macos-base", Tart: tart, Guests: 2}
	})
}

// TestAMacOSGuestIsClonedFromTheImageAndBootedWithoutAScreen reads the command the runtime is
// actually given. A guest that opens a window needs somebody sitting at the machine.
func TestAMacOSGuestIsClonedFromTheImageAndBootedWithoutAScreen(t *testing.T) {
	tart, said := buildFakeTart(t), tartLog(t)
	provider := &sandbox.MacOSProvider{Image: "ghcr.io/cirruslabs/macos-tahoe-xcode:latest", Tart: tart}
	ctx, done := context.WithTimeout(context.Background(), time.Minute)
	defer done()

	session := "d0cf0f0a4ce0000000000001"
	if _, err := provider.Create(ctx, sandbox.Config{
		ID:      session,
		Request: capacity.Request{Processor: 4 * capacity.OneProcessor, Memory: 8 << 30},
		Mounts:  []sandbox.Mount{{Source: "/host/repos", Target: "/home/agent/shared", ReadOnly: true}},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer func() { _ = provider.Remove(context.Background(), session) }()

	asked := said()
	want := []string{
		"clone ghcr.io/cirruslabs/macos-tahoe-xcode:latest krewe-" + session,
		"set krewe-" + session + " --cpu 4 --memory 8192",
		"run --no-graphics --dir=shared:/host/repos:ro krewe-" + session,
	}
	for _, one := range want {
		if !slices.Contains(asked, one) {
			t.Fatalf("the runtime was never asked to %q.\nIt was asked:\n  %s", one, strings.Join(asked, "\n  "))
		}
	}
}

// TestAMacOSGuestIsAskedWhatItIsRunningThroughPs. macOS has no /proc, so the reader the container
// backend uses finds nothing in a guest, and every session would read as empty while a conversation
// was mid answer.
func TestAMacOSGuestIsAskedWhatItIsRunningThroughPs(t *testing.T) {
	tart, said := buildFakeTart(t), tartLog(t)
	provider := &sandbox.MacOSProvider{Image: "macos-base", Tart: tart}
	ctx, done := context.WithTimeout(context.Background(), time.Minute)
	defer done()

	if _, err := provider.RuntimeRunning(ctx, "d0cf0f0a4ce0000000000002"); err != nil {
		t.Fatalf("RuntimeRunning: %v", err)
	}
	asked := strings.Join(said(), "\n")
	if strings.Contains(asked, "/proc") {
		t.Fatalf("the guest was asked to read /proc, which macOS does not have:\n%s", asked)
	}
	if !strings.Contains(asked, "ps -Ao args=") {
		t.Fatalf("the guest was never asked for its process table:\n%s", asked)
	}
}

// TestOnlyTwoMacOSGuestsRunAtOnce is the constraint that shapes this backend. Apple licenses two
// instances per host and the Virtualization framework refuses the third, so a third session waits
// for a guest rather than being handed one that cannot boot.
func TestOnlyTwoMacOSGuestsRunAtOnce(t *testing.T) {
	tart := buildFakeTart(t)
	provider := &sandbox.MacOSProvider{Image: "macos-base", Tart: tart}
	ctx, done := context.WithTimeout(context.Background(), 2*time.Minute)
	defer done()

	held := []string{"e0cf0f0a4ce0000000000001", "e0cf0f0a4ce0000000000002"}
	for _, session := range held {
		if _, err := provider.Create(ctx, sandbox.Config{ID: session}); err != nil {
			t.Fatalf("create a guest for %s: %v", session, err)
		}
		defer func() { _ = provider.Remove(context.Background(), session) }()
	}

	third := "e0cf0f0a4ce0000000000003"
	waiting, stop := context.WithTimeout(ctx, 3*time.Second)
	defer stop()
	if _, err := provider.Create(waiting, sandbox.Config{ID: third}); err == nil {
		t.Fatal("a third guest was handed out, and the framework refuses to boot it")
	} else if !strings.Contains(err.Error(), "2 macOS guests") {
		t.Fatalf("the refusal says %q, and it has to say the host is full", err)
	}

	// And the wait ends the moment one is given back.
	if err := provider.Remove(ctx, held[0]); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	box, err := provider.Create(ctx, sandbox.Config{ID: third})
	if err != nil {
		t.Fatalf("create a guest once one came free: %v", err)
	}
	defer func() { _ = provider.Remove(context.Background(), third) }()
	if box == nil {
		t.Fatal("no sandbox came back")
	}
}

// TestASessionWaitingForAGuestIsAdmittedWhenOneIsGivenBack proves the queue rather than the refusal:
// a session already waiting is let in without asking again.
func TestASessionWaitingForAGuestIsAdmittedWhenOneIsGivenBack(t *testing.T) {
	tart := buildFakeTart(t)
	provider := &sandbox.MacOSProvider{Image: "macos-base", Tart: tart}
	ctx, done := context.WithTimeout(context.Background(), 2*time.Minute)
	defer done()

	first, second := "f0cf0f0a4ce0000000000001", "f0cf0f0a4ce0000000000002"
	for _, session := range []string{first, second} {
		if _, err := provider.Create(ctx, sandbox.Config{ID: session}); err != nil {
			t.Fatalf("create a guest for %s: %v", session, err)
		}
		defer func() { _ = provider.Remove(context.Background(), session) }()
	}

	third := "f0cf0f0a4ce0000000000003"
	admitted := make(chan error, 1)
	go func() {
		_, err := provider.Create(ctx, sandbox.Config{ID: third})
		admitted <- err
	}()

	select {
	case err := <-admitted:
		t.Fatalf("the third session was not made to wait: %v", err)
	case <-time.After(time.Second):
	}

	if err := provider.Remove(ctx, second); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	select {
	case err := <-admitted:
		if err != nil {
			t.Fatalf("the waiting session was refused after a guest came free: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("a guest came free and the session waiting for it was never admitted")
	}
	_ = provider.Remove(context.Background(), third)
}

// TestAnAdoptedGuestTakesNoSecondPlace. Create adopts the guest a session already has, and a pool
// that counted each adoption would fill the host up with one session.
func TestAnAdoptedGuestTakesNoSecondPlace(t *testing.T) {
	tart := buildFakeTart(t)
	provider := &sandbox.MacOSProvider{Image: "macos-base", Tart: tart}
	ctx, done := context.WithTimeout(context.Background(), 2*time.Minute)
	defer done()

	session := "a1cf0f0a4ce0000000000001"
	for range 3 {
		if _, err := provider.Create(ctx, sandbox.Config{ID: session}); err != nil {
			t.Fatalf("create for a session that already has a guest: %v", err)
		}
	}
	defer func() { _ = provider.Remove(context.Background(), session) }()

	other := "a1cf0f0a4ce0000000000002"
	if _, err := provider.Create(ctx, sandbox.Config{ID: other}); err != nil {
		t.Fatalf("a second session was refused a guest, so one session took the whole host: %v", err)
	}
	_ = provider.Remove(context.Background(), other)
}

// TestTheMacOSKindIsBuiltAndTheDefaultIsStillDocker. The kind is one more case beside docker and
// local: an unknown one is still refused, and a system that names nothing still gets a container.
func TestTheMacOSKindIsBuiltAndTheDefaultIsStillDocker(t *testing.T) {
	for _, kind := range []string{"", sandbox.KindDocker} {
		resolved, err := sandbox.ResolveKind(kind)
		if err != nil || resolved != sandbox.KindDocker {
			t.Fatalf("ResolveKind(%q) = %q, %v, want the container backend", kind, resolved, err)
		}
		built, err := sandbox.NewProvider(kind, sandbox.Options{Image: "img"})
		if err != nil {
			t.Fatalf("NewProvider(%q): %v", kind, err)
		}
		if _, isDocker := built.(sandbox.DockerProvider); !isDocker {
			t.Fatalf("NewProvider(%q) built %T, want the container backend", kind, built)
		}
	}

	resolved, err := sandbox.ResolveKind(sandbox.KindMacOS)
	if err != nil || resolved != sandbox.KindMacOS {
		t.Fatalf("ResolveKind(%q) = %q, %v", sandbox.KindMacOS, resolved, err)
	}
	built, err := sandbox.NewProvider(sandbox.KindMacOS, sandbox.Options{Image: "macos-base"})
	if err != nil {
		t.Fatalf("NewProvider(macos): %v", err)
	}
	guests, isMacOS := built.(*sandbox.MacOSProvider)
	if !isMacOS {
		t.Fatalf("NewProvider(macos) built %T, want the macOS backend", built)
	}
	if guests.Image != "macos-base" {
		t.Fatalf("the backend was built with the image %q, want the one it was configured with", guests.Image)
	}

	if _, err := sandbox.ResolveKind("macosx"); err == nil {
		t.Fatal("an unknown kind resolved, and a typo then silently gets a container")
	}
	if _, err := sandbox.NewProvider("macosx", sandbox.Options{Image: "img"}); err == nil {
		t.Fatal("an unknown kind was built, and a typo then silently gets a container")
	}
}

// TestAMacOSBackendWithNoImageRefusesToCreate: a guest is cloned from an image, and a backend with
// none would otherwise fail somewhere inside the runtime.
func TestAMacOSBackendWithNoImageRefusesToCreate(t *testing.T) {
	provider := &sandbox.MacOSProvider{Tart: buildFakeTart(t)}
	ctx, done := context.WithTimeout(context.Background(), time.Minute)
	defer done()
	if _, err := provider.Create(ctx, sandbox.Config{ID: "b1cf0f0a4ce0000000000001"}); err == nil {
		t.Fatal("a backend with no image made a guest")
	}
}

// The stand in is a program, so it is built once for the whole package rather than per test, and it
// outlives any one test's temporary directory.
var fakeTart struct {
	once sync.Once
	dir  string
	path string
	err  error
	said string
}

// TestMain builds the stand in before anything asks for it, and takes it away after.
func TestMain(m *testing.M) {
	code := m.Run()
	if fakeTart.dir != "" {
		_ = os.RemoveAll(fakeTart.dir)
	}
	os.Exit(code)
}

// tartBinary is the stand in, built on the first ask.
func tartBinary(t *testing.T) string {
	t.Helper()
	fakeTart.once.Do(func() {
		fakeTart.dir, fakeTart.err = os.MkdirTemp("", "faketart")
		if fakeTart.err != nil {
			return
		}
		fakeTart.path = filepath.Join(fakeTart.dir, "tart")
		out, err := exec.Command("go", "build", "-o", fakeTart.path,
			"github.com/atlantic-blue/quay-krewe/internal/sandbox/sandboxtest/faketart").CombinedOutput()
		fakeTart.err, fakeTart.said = err, string(out)
	})
	if fakeTart.err != nil {
		t.Fatalf("building the fake tart: %v: %s", fakeTart.err, fakeTart.said)
	}
	return fakeTart.path
}

// tartHome points the stand in at machines of this test's own, so two tests cannot see each
// other's. The fake reads the directory from the environment, which a child process inherits.
func tartHome(t *testing.T) {
	t.Helper()
	t.Setenv("FAKE_TART_HOME", filepath.Join(t.TempDir(), "machines"))
}

// buildFakeTart is the stand in, pointed at machines of this test's own.
func buildFakeTart(t *testing.T) string {
	t.Helper()
	path := tartBinary(t)
	tartHome(t)
	return path
}

// tartLog makes the stand in write down every command it is given, and answers with them. It is how
// a test reads what the backend asked the runtime for, rather than only what came back.
func tartLog(t *testing.T) func() []string {
	t.Helper()
	at := filepath.Join(t.TempDir(), "asked")
	t.Setenv("FAKE_TART_LOG", at)
	return func() []string {
		body, err := os.ReadFile(at)
		if err != nil {
			return nil
		}
		return strings.Split(strings.TrimSpace(string(body)), "\n")
	}
}
