// Package sandboxtest holds the contract every sandbox provider keeps, and runs it against one.
//
// There is more than one runtime now: Docker, and containerd through nerdctl. Both are asked for a
// container, both are asked to find one again, and both are asked what is running inside it. A
// backend that answered any of those differently would be found by an operator rather than by a
// test, so the two are held to one suite the way the two stores are.
//
// What each case needs is a real runtime, so a caller runs this from a file tagged integration. The
// image needs a shell, a cat and an echo; busybox is enough.
package sandboxtest

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
)

// Open builds the provider under test over a storage directory the case owns. The directory is the
// host path and the path this process sees, because these cases run on the host rather than inside
// the control plane.
type Open func(t *testing.T, storage sandbox.Storage) sandbox.Provider

// sessionID is a session identifier of the shape a listing reads: twenty four hexadecimal
// characters. Anything looser is not read as a sandbox at all, so a case that used a readable name
// would prove nothing about Stranded.
func sessionID(n int) string { return fmt.Sprintf("%024x", 0xc0ffee00+n) }

// RunConformance runs the whole provider contract against one backend.
func RunConformance(t *testing.T, open Open) {
	t.Helper()
	for at, one := range []struct {
		name string
		run  func(t *testing.T, provider sandbox.Provider, id string)
	}{
		{"a command runs inside the session's sandbox and its output comes back", runsACommand},
		{"what a session writes outlives its container", keepsStateAcrossContainers},
		{"a sandbox is found again by a system that forgot it", isFoundAgain},
		{"the runtime's listing holds the sessions it is holding", listsWhatItHolds},
		{"removing a sandbox that is not there is success", removingWhatIsGoneIsSuccess},
		{"a sandbox nobody has opened holds no conversation", nobodyIsInAFreshSandbox},
	} {
		t.Run(one.name, func(t *testing.T) {
			data := t.TempDir()
			provider := open(t, sandbox.Storage{Dir: data, Host: data})
			// By position rather than by name, so renaming a case cannot hand two of them one
			// identifier and one container.
			id := sessionID(at)
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				_ = provider.Remove(ctx, id)
			})
			one.run(t, provider, id)
		})
	}
}

// deadline is one case's own, long enough for the runtime to pull an image it does not have.
func deadline(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// say runs one command in a sandbox and answers with what it printed.
func say(ctx context.Context, t *testing.T, box sandbox.Sandbox, argv ...string) string {
	t.Helper()
	proc, err := box.Exec(ctx, sandbox.Spec{Argv: argv})
	if err != nil {
		t.Fatalf("exec %v: %v", argv, err)
	}
	out, err := io.ReadAll(proc.Stdout())
	if err != nil {
		t.Fatalf("read what %v said: %v", argv, err)
	}
	if err := proc.Wait(); err != nil {
		t.Fatalf("%v exited: %v: %s", argv, err, proc.Stderr())
	}
	return strings.TrimSpace(string(out))
}

func runsACommand(t *testing.T, provider sandbox.Provider, id string) {
	ctx := deadline(t)
	box, err := provider.Create(ctx, sandbox.Config{ID: id})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := say(ctx, t, box, "echo", "hi from the sandbox"); got != "hi from the sandbox" {
		t.Fatalf("the sandbox said %q", got)
	}
	if err := box.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// keepsStateAcrossContainers is the case this whole design exists for: the conversation the model
// keeps lives in the container, so a session whose state lived only there would lose it the moment
// the container was replaced.
func keepsStateAcrossContainers(t *testing.T, provider sandbox.Provider, id string) {
	ctx := deadline(t)
	cfg := sandbox.Config{ID: id, Workspace: "ws" + id, Project: "prj" + id}

	first, err := provider.Create(ctx, cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, dir := range []string{sandbox.ConversationPath, sandbox.WorkingPath} {
		say(ctx, t, first, "sh", "-c", "echo remembered > "+dir+"/note")
	}
	if err := first.Close(ctx); err != nil {
		t.Fatalf("destroy the first sandbox: %v", err)
	}

	second, err := provider.Create(ctx, cfg)
	if err != nil {
		t.Fatalf("create the replacement sandbox: %v", err)
	}
	for _, dir := range []string{sandbox.ConversationPath, sandbox.WorkingPath} {
		if got := say(ctx, t, second, "cat", dir+"/note"); got != "remembered" {
			t.Fatalf("%s/note reads %q after the container was replaced", dir, got)
		}
	}
}

// isFoundAgain is what a control plane that restarted needs. The handles are a map in one process
// and the containers are not, so after a restart the map is empty while every container runs on.
func isFoundAgain(t *testing.T, provider sandbox.Provider, id string) {
	ctx := deadline(t)
	if _, err := provider.Create(ctx, sandbox.Config{ID: id}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	found, held, err := provider.Existing(ctx, id)
	if err != nil {
		t.Fatalf("Existing: %v", err)
	}
	if !held {
		t.Fatal("a session with a container of its own was not found again")
	}
	if got := say(ctx, t, found, "echo", "still here"); got != "still here" {
		t.Fatalf("the sandbox found again said %q", got)
	}

	// And a session that never had one is not made one by asking.
	_, held, err = provider.Existing(ctx, sessionID(999))
	if err != nil {
		t.Fatalf("Existing for a session with no container: %v", err)
	}
	if held {
		t.Fatal("a session that never had a container was handed one")
	}
}

func listsWhatItHolds(t *testing.T, provider sandbox.Provider, id string) {
	ctx := deadline(t)
	if _, err := provider.Create(ctx, sandbox.Config{ID: id}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	stranded, err := provider.Stranded(ctx)
	if err != nil {
		t.Fatalf("Stranded: %v", err)
	}
	if !slices.Contains(stranded, id) {
		t.Fatalf("the listing does not hold the session this case made: %v", stranded)
	}

	if err := provider.Remove(ctx, id); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	stranded, err = provider.Stranded(ctx)
	if err != nil {
		t.Fatalf("Stranded after the remove: %v", err)
	}
	if slices.Contains(stranded, id) {
		t.Fatalf("a removed session is still listed: %v", stranded)
	}
}

// removingWhatIsGoneIsSuccess: a sandbox that is not there is a remove that already happened. A
// backend that refused would leave a session that cannot be stopped.
func removingWhatIsGoneIsSuccess(t *testing.T, provider sandbox.Provider, id string) {
	if err := provider.Remove(deadline(t), id); err != nil {
		t.Fatalf("removing a sandbox that was never made: %v", err)
	}
}

// nobodyIsInAFreshSandbox holds both readings a reclaim depends on. Reading a live conversation as
// empty invites a drain over the top of it, so the two questions have to be answered by the runtime
// rather than guessed.
func nobodyIsInAFreshSandbox(t *testing.T, provider sandbox.Provider, id string) {
	ctx := deadline(t)
	if _, err := provider.Create(ctx, sandbox.Config{ID: id}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	attached, err := provider.Attached(ctx, id)
	if err != nil {
		t.Fatalf("Attached: %v", err)
	}
	if attached {
		t.Fatal("a sandbox nobody opened says somebody is in it")
	}

	running, err := provider.RuntimeRunning(ctx, id)
	if err != nil {
		t.Fatalf("RuntimeRunning: %v", err)
	}
	if running {
		t.Fatal("a sandbox running nothing says a conversation is up in it")
	}
}
