package sandbox_test

import (
	"context"
	"io"
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
)

// The macOS backend, driven end to end against a stand in for tart.
//
// What this proves: the backend asks tart the right questions, reads its listing, tells a machine
// that is not there from one that is stopped, adopts the guest a session already has, carries a
// working directory and an environment into a command, and hands out no more guests than Apple's
// licence permits.
//
// What it does not prove: that any of it works on an Apple machine. Nothing boots here, no image is
// pulled, and a command runs on this host rather than in a guest. The contract in
// internal/sandbox/sandboxtest is what holds this backend beside the container one, and it runs
// against a real guest in macos_conformance_integration_test.go, which needs an Apple machine.

// TestAMacOSGuestRunsACommandAndGivesBackWhatItSaid.
func TestAMacOSGuestRunsACommandAndGivesBackWhatItSaid(t *testing.T) {
	provider, ctx := aMacOSBackend(t), patient(t)
	session := "c0cf0f0a4ce0000000000001"
	box, err := provider.Create(ctx, sandbox.Config{ID: session, Env: []string{"QC_CARRIED=yes"}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer func() { _ = provider.Remove(context.Background(), session) }()

	if said := guestSaid(t, ctx, box, []string{"echo", "hi from the guest"}, sandbox.Spec{}); said != "hi from the guest" {
		t.Fatalf("the guest said %q", said)
	}
	// The subscription token rides on the sandbox own environment. A guest that drops it runs every
	// command logged out.
	if said := guestSaid(t, ctx, box, []string{"sh", "-c", `printf '%s' "$QC_CARRIED"`}, sandbox.Spec{}); said != "yes" {
		t.Fatalf("the guest read %q from the environment it was created with, want yes", said)
	}
	// And a working directory, which tart exec has no flag for.
	if said := guestSaid(t, ctx, box, []string{"pwd"}, sandbox.Spec{Workdir: "/tmp"}); said != "/tmp" {
		t.Fatalf("the command ran in %q, want /tmp", said)
	}
	if _, err := box.Exec(ctx, sandbox.Spec{}); err == nil {
		t.Fatal("a command with no words ran, and the backend has nothing to run")
	}
}

// TestAMacOSGuestIsRemovedByNameAndTwiceIsStillSuccess. Stopping a session has to work from a process
// that never made the guest, so removal goes by name. A guest that is not there is a removal that
// already happened.
func TestAMacOSGuestIsRemovedByNameAndTwiceIsStillSuccess(t *testing.T) {
	provider, ctx := aMacOSBackend(t), patient(t)
	session := "c0cf0f0a4ce0000000000002"
	if _, err := provider.Create(ctx, sandbox.Config{ID: session}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if held, err := provider.Stranded(ctx); err != nil || !slices.Contains(held, session) {
		t.Fatalf("Stranded = %v, %v, want the session in it", held, err)
	}
	if err := provider.Remove(ctx, session); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if held, err := provider.Stranded(ctx); err != nil || slices.Contains(held, session) {
		t.Fatalf("Stranded = %v, %v after a remove", held, err)
	}
	if err := provider.Remove(ctx, session); err != nil {
		t.Fatalf("Remove of a guest that is gone: %v, want success", err)
	}
	if _, found, err := provider.Existing(ctx, session); err != nil || found {
		t.Fatalf("Existing after a remove = %v, %v, want nothing and no error", found, err)
	}
}

// TestAnEmptyMacOSSandboxHoldsNobodyAndRunsNothing. A failure to reach the runtime is the system
// being unable to tell, and a session with no guest is nobody attached and nothing running. A caller
// that read the first as the second would reclaim a conversation somebody is typing into.
func TestAnEmptyMacOSSandboxHoldsNobodyAndRunsNothing(t *testing.T) {
	provider, ctx := aMacOSBackend(t), patient(t)
	session := "c0cf0f0a4ce0000000000003"

	attached, err := provider.Attached(ctx, session)
	if err != nil {
		t.Fatalf("Attached for a session with no guest: %v, want no error", err)
	}
	if attached {
		t.Fatal("a session with no guest reads as attached, so nothing reclaims it")
	}
	running, err := provider.RuntimeRunning(ctx, session)
	if err != nil {
		t.Fatalf("RuntimeRunning for a session with no guest: %v, want no error", err)
	}
	if running {
		t.Fatal("a session with no guest reads as running a model")
	}
	if box, found, err := provider.Existing(ctx, session); err != nil || found || box != nil {
		t.Fatalf("Existing made or found %v for a session with no guest: %v", box, err)
	}
}

// TestAMacOSGuestThatIsAlreadyThereIsAdoptedNotRefused is what a control plane that forgot its
// sandboxes runs into. The guest name is derived from the session, so the runtime refuses a second
// clone under it, and that session would be undispatchable until somebody deleted the guest by hand.
func TestAMacOSGuestThatIsAlreadyThereIsAdoptedNotRefused(t *testing.T) {
	provider, ctx := aMacOSBackend(t), patient(t)
	session := "c0cf0f0a4ce0000000000004"
	first, err := provider.Create(ctx, sandbox.Config{ID: session})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer func() { _ = provider.Remove(context.Background(), session) }()

	again, err := provider.Create(ctx, sandbox.Config{ID: session})
	if err != nil {
		t.Fatalf("create for a session that already has a guest: %v", err)
	}
	if guestName(t, first) != guestName(t, again) {
		t.Fatalf("a second guest was made: %q beside %q", guestName(t, again), guestName(t, first))
	}
	held, err := provider.Stranded(ctx)
	if err != nil {
		t.Fatalf("Stranded: %v", err)
	}
	guests := 0
	for _, one := range held {
		if one == session {
			guests++
		}
	}
	if guests != 1 {
		t.Fatalf("session %s holds %d guests (%v), want 1: a second one costs tens of gigabytes and one of two places",
			session, guests, held)
	}
	if said := guestSaid(t, ctx, again, []string{"echo", "adopted"}, sandbox.Spec{}); said != "adopted" {
		t.Fatalf("the adopted guest said %q, want it usable", said)
	}
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

// aMacOSBackend is the backend over a stand in for tart, with machines of this test own.
func aMacOSBackend(t *testing.T) *sandbox.MacOSProvider {
	t.Helper()
	return &sandbox.MacOSProvider{Image: "macos-base", Tart: buildFakeTart(t)}
}

// patient is a context ended with the test, long enough for a stand in and nowhere near long enough
// for a real guest, which the integration tier handles.
func patient(t *testing.T) context.Context {
	t.Helper()
	ctx, done := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(done)
	return ctx
}

// guestSaid runs one command in the guest and answers with what it wrote, trimmed.
func guestSaid(t *testing.T, ctx context.Context, box sandbox.Sandbox, argv []string, spec sandbox.Spec) string {
	t.Helper()
	spec.Argv = argv
	proc, err := box.Exec(ctx, spec)
	if err != nil {
		t.Fatalf("run %v: %v", argv, err)
	}
	said, err := io.ReadAll(proc.Stdout())
	if err != nil {
		t.Fatalf("read what the guest said: %v", err)
	}
	if err := proc.Wait(); err != nil {
		t.Fatalf("the command failed: %v: %s", err, proc.Stderr())
	}
	return strings.TrimSpace(string(said))
}

// guestName is what the runtime calls this sandbox, which an attach and an operator own command need.
func guestName(t *testing.T, box sandbox.Sandbox) string {
	t.Helper()
	named, says := box.(sandbox.Named)
	if !says {
		t.Fatalf("%T does not say what the runtime calls it", box)
	}
	return named.Name()
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
