// Package sandboxtest holds the contract every sandbox backend keeps, so a second backend cannot
// drift from the first.
//
// A backend is a runtime and a command line, and the system reads the same five answers out of each
// one: create a session's container and exec in it, reach into one that already exists, remove one by
// name, list the ones nobody wants, and say whether anybody is inside. Each of those failed quietly
// once, and every one of those failures looked like an empty machine rather than like a fault. So the
// cases are written once here and run against each backend against its real runtime.
//
// What a backend brings is the three things only its own tool can do: stop a container from outside
// the provider, say whether the runtime still holds one, and start one under a name the provider can
// no longer write.
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

// Backend is one runtime the contract is run against.
type Backend struct {
	// Kind is the word the system knows this backend by, and it names the subtests.
	Kind string
	// Image is a small image with a shell in it, which the runtime can pull.
	Image string
	// New builds the provider under test. The options carry the storage a case needs.
	New func(opts sandbox.Options) sandbox.Provider
	// Available says whether this runtime can be reached on this machine, and what to say when it
	// cannot. A backend that is not there skips, and the skip says which command was missing.
	Available func() (bool, string)
	// Stop stops a container by name from outside the provider. It is the state a machine is left in
	// after a restart, and the provider must start the container again rather than refuse the name.
	Stop func(ctx context.Context, name string) error
	// Held says whether the runtime still holds a container of this name. A remove is proved against
	// the runtime, never against the provider that did the removing.
	Held func(ctx context.Context, name string) (bool, error)
	// Start starts a container under a name the provider can no longer write, which is how the case
	// for a sandbox from before the rename sets its state up.
	Start func(ctx context.Context, name string) error
}

// RunConformance holds one backend to the whole contract.
func RunConformance(t *testing.T, backend Backend) {
	t.Helper()

	if available, why := backend.Available(); !available {
		t.Skipf("the %s runtime is not on this machine, so nothing here ran: %s", backend.Kind, why)
	}

	contract := cases()
	// A suite that found nothing to run reports success exactly like one that ran everything.
	if len(contract) == 0 {
		t.Fatalf("the %s backend was held to no cases at all, so this run proved nothing", backend.Kind)
	}
	for name, run := range contract {
		t.Run(name, func(t *testing.T) { run(t, backend) })
	}
}

// cases is the contract, by the sentence each one proves.
func cases() map[string]func(*testing.T, Backend) {
	return map[string]func(*testing.T, Backend){
		"a session runs a command and reads its output back":         aSessionRunsACommand,
		"a value in the environment reaches the process":             theEnvironmentTravels,
		"what a session wrote outlives its container":                stateOutlivesTheContainer,
		"a container that is already there is adopted, not refused":  anExistingContainerIsAdopted,
		"a container is removed by name, and twice is still success": removalGoesByName,
		"a sandbox from before the rename is found and removed":      theRetiredNameIsStillReached,
		"an empty sandbox holds nobody and runs nothing":             anEmptySandboxIsQuiet,
	}
}

// sessionID is a name of the exact shape a session identifier has. The listing that finds a stranded
// sandbox matches that shape and nothing looser, because the system's own containers are containers
// too and nobody stops one of them to make room for a session. An identifier of any other shape would
// make every case here pass while finding nothing.
func sessionID(last string) string {
	return strings.Repeat("0", sandbox.SessionIDLength-len(last)) + last
}

func timed(t *testing.T, seconds int) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(seconds)*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// output runs one command in the sandbox and answers with what it wrote, trimmed.
func output(t *testing.T, ctx context.Context, box sandbox.Sandbox, argv ...string) string {
	t.Helper()
	proc, err := box.Exec(ctx, sandbox.Spec{Argv: argv})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	said, err := io.ReadAll(proc.Stdout())
	if err != nil {
		t.Fatalf("read what the sandbox said: %v", err)
	}
	if err := proc.Wait(); err != nil {
		t.Fatalf("the command failed: %v: %s", err, proc.Stderr())
	}
	return strings.TrimSpace(string(said))
}

func aSessionRunsACommand(t *testing.T, backend Backend) {
	ctx := timed(t, 180)
	provider := backend.New(sandbox.Options{Image: backend.Image})
	id := sessionID("a1")

	box, err := provider.Create(ctx, sandbox.Config{ID: id})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = provider.Remove(context.Background(), id) })

	if said := output(t, ctx, box, "echo", "hi from the sandbox"); said != "hi from the sandbox" {
		t.Fatalf("the sandbox said %q", said)
	}
	if err := box.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// theEnvironmentTravels proves the mechanism the model's token rides on: a value put in Spec.Env
// reaches the process running inside the sandbox. Without it an exec runs with no credential and
// reports that the operator is logged out.
func theEnvironmentTravels(t *testing.T, backend Backend) {
	ctx := timed(t, 180)
	provider := backend.New(sandbox.Options{Image: backend.Image})
	id := sessionID("a2")

	box, err := provider.Create(ctx, sandbox.Config{ID: id})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = provider.Remove(context.Background(), id) })

	proc, err := box.Exec(ctx, sandbox.Spec{
		Argv: []string{"printenv", "CLAUDE_CODE_OAUTH_TOKEN"},
		Env:  []string{"CLAUDE_CODE_OAUTH_TOKEN=tok-from-secret"},
	})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	said, err := io.ReadAll(proc.Stdout())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := proc.Wait(); err != nil {
		t.Fatalf("wait: %v: %s", err, proc.Stderr())
	}
	if strings.TrimSpace(string(said)) != "tok-from-secret" {
		t.Fatalf("the process read %q from its environment", said)
	}
}

// stateOutlivesTheContainer is the assertion the whole storage design exists for: a session writes
// something, its container goes, a new container is made for the same session, and what it wrote is
// still there. A sibling session in the same project gets its own working directory and shares the
// conversation store, because two conversations in one directory means one of them changing a file
// under the other.
func stateOutlivesTheContainer(t *testing.T, backend Backend) {
	ctx := timed(t, 300)
	// Dir and Host are one path because this test runs on the machine the runtime is on, so the two
	// views of the directory are one directory.
	data := t.TempDir()
	provider := backend.New(sandbox.Options{
		Image:   backend.Image,
		Storage: sandbox.Storage{Dir: data, Host: data},
	})
	id, sibling := sessionID("a3"), sessionID("a4")
	config := sandbox.Config{ID: id, Workspace: "ws-durable", Project: "prj-durable"}
	t.Cleanup(func() {
		_ = provider.Remove(context.Background(), id)
		_ = provider.Remove(context.Background(), sibling)
	})

	first, err := provider.Create(ctx, config)
	if err != nil {
		t.Fatalf("create the first sandbox: %v", err)
	}
	for _, dir := range []string{sandbox.ConversationPath, sandbox.WorkingPath} {
		output(t, ctx, first, "sh", "-c", "echo remembered > "+dir+"/note")
	}
	if err := first.Close(ctx); err != nil {
		t.Fatalf("destroy the first sandbox: %v", err)
	}

	second, err := provider.Create(ctx, config)
	if err != nil {
		t.Fatalf("create the replacement sandbox: %v", err)
	}
	for _, dir := range []string{sandbox.ConversationPath, sandbox.WorkingPath} {
		if said := output(t, ctx, second, "cat", dir+"/note"); said != "remembered" {
			t.Fatalf("%s/note reads %q after the container was replaced", dir, said)
		}
	}

	beside, err := provider.Create(ctx, sandbox.Config{ID: sibling, Workspace: config.Workspace, Project: config.Project})
	if err != nil {
		t.Fatalf("create a sibling session's sandbox: %v", err)
	}
	if said := output(t, ctx, beside, "sh", "-c", "cat "+sandbox.WorkingPath+"/note 2>&1 || true"); strings.Contains(said, "remembered") {
		t.Fatalf("the sibling session reads the other one's working directory: %q", said)
	}
	if said := output(t, ctx, beside, "cat", sandbox.ConversationPath+"/note"); said != "remembered" {
		t.Fatalf("the sibling session reads %q from the conversation store, want it shared", said)
	}
}

// anExistingContainerIsAdopted is what a control plane that forgot its sandboxes runs into. A
// session's container name is deterministic, so creating again after a restart hits the runtime's name
// conflict, and that session is undispatchable until somebody removes the container by hand.
func anExistingContainerIsAdopted(t *testing.T, backend Backend) {
	ctx := timed(t, 300)
	provider := backend.New(sandbox.Options{Image: backend.Image})
	id := sessionID("a5")

	first, err := provider.Create(ctx, sandbox.Config{ID: id})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = provider.Remove(context.Background(), id) })

	again, err := provider.Create(ctx, sandbox.Config{ID: id})
	if err != nil {
		t.Fatalf("creating a second time for the same session: %v", err)
	}
	if said := output(t, ctx, again, "echo", "adopted"); said != "adopted" {
		t.Fatalf("the adopted sandbox says %q, want it usable", said)
	}
	if named, says := first.(sandbox.Named); says {
		if beside, also := again.(sandbox.Named); also && named.Name() != beside.Name() {
			t.Fatalf("a second container %s was started beside %s", beside.Name(), named.Name())
		}
	}

	// A container that stopped is started rather than left dead, which is the state a machine is in
	// after the host or the runtime restarted under it.
	if err := backend.Stop(ctx, sandbox.ContainerName(id)); err != nil {
		t.Fatalf("stopping the container: %v", err)
	}
	restarted, err := provider.Create(ctx, sandbox.Config{ID: id})
	if err != nil {
		t.Fatalf("creating over a stopped container: %v", err)
	}
	if said := output(t, ctx, restarted, "echo", "started again"); said != "started again" {
		t.Fatalf("the restarted sandbox says %q, want it running", said)
	}

	// And reaching into that session finds it rather than making a second one.
	held, there, err := provider.Existing(ctx, id)
	if err != nil {
		t.Fatalf("Existing: %v", err)
	}
	if !there {
		t.Fatal("the system reaches into a session whose container runs and finds nothing")
	}
	if said := output(t, ctx, held, "echo", "reached"); said != "reached" {
		t.Fatalf("the sandbox reached into says %q, want it usable", said)
	}
}

// removalGoesByName because stopping a session has to work from a process that never made the
// container. The handles are a map in one process and the containers are not: after a restart the map
// is empty while every container runs on.
func removalGoesByName(t *testing.T, backend Backend) {
	ctx := timed(t, 240)
	provider := backend.New(sandbox.Options{Image: backend.Image})
	id := sessionID("a6")

	if _, err := provider.Create(ctx, sandbox.Config{ID: id}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = provider.Remove(context.Background(), id) })

	stranded, err := provider.Stranded(ctx)
	if err != nil {
		t.Fatalf("Stranded: %v", err)
	}
	if !slices.Contains(stranded, id) {
		t.Fatalf("Stranded = %v, want it to hold %s, or a drain leaves that container holding the machine", stranded, id)
	}

	if err := provider.Remove(ctx, id); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	still, err := backend.Held(ctx, sandbox.ContainerName(id))
	if err != nil {
		t.Fatalf("asking the runtime what it holds: %v", err)
	}
	if still {
		t.Fatal("the container is still there after Remove")
	}
	if err := provider.Remove(ctx, id); err != nil {
		t.Fatalf("Remove of an absent container: %v, want success", err)
	}

	stranded, err = provider.Stranded(ctx)
	if err != nil {
		t.Fatalf("Stranded: %v", err)
	}
	if slices.Contains(stranded, id) {
		t.Fatalf("Stranded still lists %s after Remove", id)
	}

	// And a session that never had a container is nobody's to reach into.
	if _, there, err := provider.Existing(ctx, sessionID("a7")); err != nil {
		t.Fatalf("Existing on a session with no container: %v", err)
	} else if there {
		t.Fatal("the system found a container for a session that never had one")
	}
}

// theRetiredNameIsStillReached is the state an upgrade inherits. An operator upgrades with sessions
// up, so their containers carry the retired name, and a system that cannot see one does not drain it,
// does not remove it, and starts a second container beside it on the next exec while the first keeps
// the machine's memory.
//
// The container is made by the backend rather than by the provider, because the provider can no longer
// write that name. That is the point: this is a state this build cannot produce.
func theRetiredNameIsStillReached(t *testing.T, backend Backend) {
	ctx := timed(t, 240)
	provider := backend.New(sandbox.Options{Image: backend.Image})
	id := sessionID("a8")
	retired := sandbox.RetiredContainerPrefix + id

	if err := backend.Start(ctx, retired); err != nil {
		t.Fatalf("start a container under the retired name: %v", err)
	}
	t.Cleanup(func() { _ = provider.Remove(context.Background(), id) })

	stranded, err := provider.Stranded(ctx)
	if err != nil {
		t.Fatalf("Stranded: %v", err)
	}
	if !slices.Contains(stranded, id) {
		t.Fatalf("Stranded = %v, and %s runs, so a drain leaves it holding the machine", stranded, retired)
	}

	box, there, err := provider.Existing(ctx, id)
	if err != nil {
		t.Fatalf("Existing: %v", err)
	}
	if !there {
		t.Fatalf("the system reaches into session %s and finds nothing, while %s is up", id, retired)
	}
	if said := output(t, ctx, box, "echo", "reached"); said != "reached" {
		t.Fatalf("the adopted sandbox says %q, want it usable", said)
	}
	if named, says := box.(sandbox.Named); !says || named.Name() != retired {
		t.Fatalf("the sandbox calls itself %v, and an attach opens that name", box)
	}

	again, err := provider.Create(ctx, sandbox.Config{ID: id})
	if err != nil {
		t.Fatalf("Create over a sandbox from before the rename: %v", err)
	}
	if named, says := again.(sandbox.Named); !says || named.Name() != retired {
		t.Fatalf("a second container was started beside %s", retired)
	}

	if err := provider.Remove(ctx, id); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	still, err := backend.Held(ctx, retired)
	if err != nil {
		t.Fatalf("asking the runtime what it holds: %v", err)
	}
	if still {
		t.Fatalf("%s still runs after the session was stopped", retired)
	}
}

// anEmptySandboxIsQuiet. Both questions are asked by name from outside, and both have to answer for a
// session this process never created. A container with nobody in it answers no to each, and a session
// with no container at all answers no as well rather than failing: absent is an answer, and the system
// being unable to tell is not.
func anEmptySandboxIsQuiet(t *testing.T, backend Backend) {
	ctx := timed(t, 240)
	provider := backend.New(sandbox.Options{Image: backend.Image})
	id := sessionID("a9")

	if _, err := provider.Create(ctx, sandbox.Config{ID: id}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = provider.Remove(context.Background(), id) })

	attached, err := provider.Attached(ctx, id)
	if err != nil {
		t.Fatalf("Attached: %v", err)
	}
	if attached {
		t.Error("a container nobody opened reports somebody typing into it, so the system will not reclaim it")
	}
	running, err := provider.RuntimeRunning(ctx, id)
	if err != nil {
		t.Fatalf("RuntimeRunning: %v", err)
	}
	if running {
		t.Error("a container running nothing reports a conversation in flight")
	}

	absent := sessionID("b1")
	if attached, err := provider.Attached(ctx, absent); err != nil {
		t.Errorf("Attached for a session with no container: %v, want the answer no", err)
	} else if attached {
		t.Error("a session with no container reports somebody attached to it")
	}
	if running, err := provider.RuntimeRunning(ctx, absent); err != nil {
		t.Errorf("RuntimeRunning for a session with no container: %v, want the answer no", err)
	} else if running {
		t.Error("a session with no container reports a runtime up in it")
	}
}
