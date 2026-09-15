package sandbox

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A session identifier is twenty four hexadecimal characters, and a listing only reads a container as
// a sandbox when its name has exactly that shape.
const containerdSession = "0d4f2a1b3c5e7f9a0b1c2d3e"

// runtimeDouble is a script standing in for nerdctl, which is all the runtime is from this
// provider's side: a command line that answers. It records every command line it was given.
type runtimeDouble struct {
	// binary is what the provider drives.
	binary string
	// asked is the file each command line is appended to.
	asked string
}

// aRuntimeThat writes one double. The body is a shell script that answers the verbs a scenario needs,
// and whatever it does not answer succeeds saying nothing, which is what a create does.
func aRuntimeThat(t *testing.T, answers string) runtimeDouble {
	t.Helper()
	dir := t.TempDir()
	double := runtimeDouble{binary: filepath.Join(dir, "nerdctl"), asked: filepath.Join(dir, "asked")}
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> " + double.asked + "\n" +
		answers + "\nexit 0\n"
	if err := os.WriteFile(double.binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return double
}

// lines is every command line the double was given, one per entry.
func (r runtimeDouble) lines(t *testing.T) []string {
	t.Helper()
	said, err := os.ReadFile(r.asked)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(said)), "\n")
}

// heardVerb is the whole command line starting with this verb, and empty where the double never
// heard it.
func (r runtimeDouble) heardVerb(t *testing.T, verb string) string {
	t.Helper()
	for _, line := range r.lines(t) {
		if strings.HasPrefix(line, verb+" ") || line == verb {
			return line
		}
	}
	return ""
}

// nothingIsThere is a runtime holding no container at all: inspect fails the way it fails for a name
// nobody has.
const nothingIsThere = `case "$1" in inspect) exit 1;; esac`

// TestTheTwoRuntimesAreAskedForTheSameContainer. This is what stops the two backends drifting. A
// session under containerd joins the network the Docker one joins, is held to the same memory, gets
// the same mounts and the same secrets directory, because both argument vectors are built by one
// function. A flag added for one runtime reaches the other, and this fails the moment it does not.
func TestTheTwoRuntimesAreAskedForTheSameContainer(t *testing.T) {
	options := Options{
		Image:          "img",
		Mounts:         []string{"/opt/tools:/opt/tools:ro"},
		Network:        "quaycrew_default",
		SessionNetwork: "quaycrew_sessions",
		DriverMounts:   []string{"/hub:/hub:ro"},
		Memory:         "4g",
	}
	kept := []Mount{{Source: "/data/ws", Target: ConversationPath}}

	for _, cfg := range []Config{
		{ID: containerdSession, Env: []string{"QC_TOKEN=abc"}},
		{ID: containerdSession, Driver: true},
	} {
		docker := strings.Join(DockerProvider(options).runArgs("krewe-"+cfg.ID, cfg, kept), " ")
		containerd := strings.Join(ContainerdProvider{Options: options}.compatible().
			runArgs("krewe-"+cfg.ID, cfg, kept), " ")
		if docker != containerd {
			t.Fatalf("the two runtimes are asked for different containers (driver=%v):\ndocker:     %s\ncontainerd: %s",
				cfg.Driver, docker, containerd)
		}
	}
}

// TestTheContainerdBackendDrivesNerdctl. Nothing configured must reach containerd through the tool
// this provider was written against, rather than through whatever else happens to be on the path.
func TestTheContainerdBackendDrivesNerdctl(t *testing.T) {
	if got := (ContainerdProvider{}).command(); got != "nerdctl" {
		t.Fatalf("the containerd backend drives %q, want nerdctl", got)
	}
	if got := (ContainerdProvider{Binary: "nerdctl.lima"}).command(); got != "nerdctl.lima" {
		t.Fatalf("an operator naming their own command line got %q", got)
	}
}

// TestASessionsContainerIsCreatedOnContainerd: the verbs and the flags a session's container is
// created with, sent to the command line this provider drives.
func TestASessionsContainerIsCreatedOnContainerd(t *testing.T) {
	double := aRuntimeThat(t, nothingIsThere)
	provider := ContainerdProvider{
		Options: Options{Image: "img", SessionNetwork: "quaycrew_sessions"},
		Binary:  double.binary,
	}

	box, err := provider.Create(context.Background(), Config{ID: containerdSession})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	created := double.heardVerb(t, "run")
	for _, want := range []string{
		"--detach", "--name krewe-" + containerdSession, "--network quaycrew_sessions",
		"img sleep infinity",
	} {
		if !strings.Contains(created, want) {
			t.Fatalf("the container was created without %q:\n%s", want, created)
		}
	}
	named, ok := box.(Named)
	if !ok || named.Name() != "krewe-"+containerdSession {
		t.Fatalf("the sandbox does not carry the name the runtime holds it under: %#v", box)
	}
}

// TestAContainerAlreadyCarryingTheSessionsNameIsAdopted. A session's name is deterministic, so a
// control plane that has forgotten its sandboxes would otherwise never start that session again: the
// runtime refuses the name and the session is undispatchable until somebody removes the container by
// hand.
func TestAContainerAlreadyCarryingTheSessionsNameIsAdopted(t *testing.T) {
	double := aRuntimeThat(t, `case "$1" in inspect) echo true;; esac`)
	provider := ContainerdProvider{Options: Options{Image: "img"}, Binary: double.binary}

	if _, err := provider.Create(context.Background(), Config{ID: containerdSession}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if created := double.heardVerb(t, "run"); created != "" {
		t.Fatalf("a second container was created over a session that already had one: %s", created)
	}
	// And it is not started either. A container that is already running is reached into as it is.
	if started := double.heardVerb(t, "start"); started != "" {
		t.Fatalf("a running container was started again: %s", started)
	}
}

// TestAStoppedContainerIsStartedRatherThanReplaced. A stopped container still holds the session's
// files and the credential helper that reaches its remote, so it is started rather than thrown away.
func TestAStoppedContainerIsStartedRatherThanReplaced(t *testing.T) {
	double := aRuntimeThat(t, `case "$1" in inspect) echo false;; esac`)
	provider := ContainerdProvider{Options: Options{Image: "img"}, Binary: double.binary}

	if _, err := provider.Create(context.Background(), Config{ID: containerdSession}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if started := double.heardVerb(t, "start"); !strings.Contains(started, "krewe-"+containerdSession) {
		t.Fatalf("a stopped container was not started: %v", double.lines(t))
	}
	if created := double.heardVerb(t, "run"); created != "" {
		t.Fatalf("a stopped container was replaced rather than started: %s", created)
	}
}

// TestAnExecRunsInsideTheSessionsOwnContainer. The double prints what it was asked, so this reads the
// exec back through the process the runner reads.
func TestAnExecRunsInsideTheSessionsOwnContainer(t *testing.T) {
	double := aRuntimeThat(t, `case "$1" in inspect) exit 1;; exec) echo "$*";; esac`)
	provider := ContainerdProvider{Options: Options{Image: "img"}, Binary: double.binary}

	box, err := provider.Create(context.Background(), Config{ID: containerdSession})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	proc, err := box.Exec(context.Background(), Spec{
		Argv: []string{"echo", "hi"}, Workdir: "/home/agent/workspace", Env: []string{"K=v"},
	})
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

	want := "exec -i -w /home/agent/workspace -e K=v krewe-" + containerdSession + " echo hi"
	if strings.TrimSpace(string(out)) != want {
		t.Fatalf("the exec was %q, want %q", strings.TrimSpace(string(out)), want)
	}
}

// TestRemovingASandboxThatIsNotThereIsSuccess. A remove that already happened is not a failure, and
// nerdctl spells the absence in lower case where Docker spells it with a capital.
func TestRemovingASandboxThatIsNotThereIsSuccess(t *testing.T) {
	double := aRuntimeThat(t, `case "$1" in rm) echo "no such container: $3" >&2; exit 1;; esac`)
	provider := ContainerdProvider{Options: Options{Image: "img"}, Binary: double.binary}

	if err := provider.Remove(context.Background(), containerdSession); err != nil {
		t.Fatalf("removing a sandbox that is not there: %v", err)
	}
	// Both names, because a sandbox that started before the rename carries the retired one, and a
	// remove that only knew the new name would leave it running and holding its memory.
	for _, name := range ContainerNames(containerdSession) {
		if !strings.Contains(strings.Join(double.lines(t), "\n"), name) {
			t.Fatalf("the remove never asked about %s: %v", name, double.lines(t))
		}
	}
}

// TestARemovalThatFailedForAnotherReasonIsReported. Reading every failure as absence would report a
// session stopped while its container kept running.
func TestARemovalThatFailedForAnotherReasonIsReported(t *testing.T) {
	double := aRuntimeThat(t, `case "$1" in rm) echo "permission denied" >&2; exit 1;; esac`)
	provider := ContainerdProvider{Options: Options{Image: "img"}, Binary: double.binary}

	err := provider.Remove(context.Background(), containerdSession)
	if err == nil {
		t.Fatal("a removal the runtime refused was reported as done")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("the error does not carry what the runtime said: %v", err)
	}
}

// TestStrandedHoldsSessionsAndNothingElseTheRuntimeRuns. The stack's own services are containers
// too, and nobody stops one of them to make room for a session.
func TestStrandedHoldsSessionsAndNothingElseTheRuntimeRuns(t *testing.T) {
	listing := "krewe-" + containerdSession + "\nqcci-postgres-1\nquaycrew-redpanda-1"
	double := aRuntimeThat(t, `case "$1" in ps) printf '%s\n' "`+listing+`";; esac`)
	provider := ContainerdProvider{Options: Options{Image: "img"}, Binary: double.binary}

	stranded, err := provider.Stranded(context.Background())
	if err != nil {
		t.Fatalf("Stranded: %v", err)
	}
	if len(stranded) != 1 || stranded[0] != containerdSession {
		t.Fatalf("stranded = %v, want only the session", stranded)
	}
	if asked := double.heardVerb(t, "ps"); !strings.Contains(asked, "--all") {
		t.Fatalf("the listing leaves out the containers that have stopped: %q", asked)
	}
}

// TestAConversationSomebodyIsTypingIntoIsNotReadAsEmpty. Attached is what holds a drain, a restart or
// a reclaim off a session somebody is in.
func TestAConversationSomebodyIsTypingIntoIsNotReadAsEmpty(t *testing.T) {
	double := aRuntimeThat(t, `case "$1" in exec) echo "/dev/ttys004";; esac`)
	provider := ContainerdProvider{Options: Options{Image: "img"}, Binary: double.binary}

	attached, err := provider.Attached(context.Background(), containerdSession)
	if err != nil {
		t.Fatalf("Attached: %v", err)
	}
	if !attached {
		t.Fatal("a conversation with a client on it reads as nobody being there")
	}
	if asked := double.heardVerb(t, "exec"); !strings.Contains(asked, "tmux list-clients -t "+AttachedSessionName) {
		t.Fatalf("the question asked was %q", asked)
	}
}

// TestASandboxWithNobodyInItSaysSo. A non zero exit is no tmux server, no such conversation or no
// such container, and nobody is typing into any of those.
func TestASandboxWithNobodyInItSaysSo(t *testing.T) {
	double := aRuntimeThat(t, `case "$1" in exec) exit 1;; esac`)
	provider := ContainerdProvider{Options: Options{Image: "img"}, Binary: double.binary}

	attached, err := provider.Attached(context.Background(), containerdSession)
	if err != nil {
		t.Fatalf("Attached: %v", err)
	}
	if attached {
		t.Fatal("an empty sandbox reads as having somebody in it")
	}
}

// TestARunningConversationIsReadFromTheProcessTable. The state the system could not see: a
// conversation somebody detached from leaves the runtime answering with nobody watching it.
func TestARunningConversationIsReadFromTheProcessTable(t *testing.T) {
	double := aRuntimeThat(t, `case "$1" in exec) echo "/usr/local/bin/`+RuntimeBinary+` --resume";; esac`)
	provider := ContainerdProvider{Options: Options{Image: "img"}, Binary: double.binary}

	running, err := provider.RuntimeRunning(context.Background(), containerdSession)
	if err != nil {
		t.Fatalf("RuntimeRunning: %v", err)
	}
	if !running {
		t.Fatal("a sandbox holding a conversation reads as empty, which invites a reclaim over the top of it")
	}
}

// TestARuntimeThatCannotBeReachedIsNotAnEmptySandbox. nerdctl is not on the path here, so the command
// cannot be started at all, which is what an unreachable runtime looks like. The answer must be an
// error: a caller reads false as licence to close a container.
func TestARuntimeThatCannotBeReachedIsNotAnEmptySandbox(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	running, err := ContainerdProvider{Options: Options{Image: "img"}}.
		RuntimeRunning(context.Background(), containerdSession)
	if err == nil {
		t.Fatal("the runtime could not be reached and the system answered anyway")
	}
	if running {
		t.Fatal("a failure came back as a running runtime")
	}
	if !strings.Contains(err.Error(), ContainerName(containerdSession)) {
		t.Fatalf("the error does not name the container: %v", err)
	}
}

// TestAnAbsentContainerIsReadUnderEitherSpelling. The two runtimes differ by one capital letter, and
// a reader that knew one spelling would report a remove as failed under the other.
func TestAnAbsentContainerIsReadUnderEitherSpelling(t *testing.T) {
	for _, said := range []string{
		"Error: No such container: krewe-1",
		"FATA[0000] no such container: krewe-1",
	} {
		if !absentContainer(said) {
			t.Errorf("%q was not read as the container being absent", said)
		}
	}
	if absentContainer("FATA[0000] permission denied") {
		t.Error("a refusal was read as the container being absent")
	}
}
