package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// appleBinary is Apple's container tool, as it is installed on a macOS machine. It talks to a
// service on that machine rather than to a socket, so there is nothing to hand a container: a
// control plane that runs inside the compose stack cannot reach it, and one that runs on the host
// can.
const appleBinary = "container"

// AppleProvider gives each session its own container under Apple's container tool. That tool puts
// every container in a light virtual machine of its own, through the Containerization framework, so
// a session costs one small virtual machine instead of a share of one large one, and the machine
// needs no Docker Desktop.
//
// It is the Docker backend's shape over a different command line. The two tools spell most of a run
// the same way, and each difference that matters is written at the place it applies.
//
// The tool runs Linux guests. A session under it gets no macOS, and it cannot build a darwin
// application. Issue 827 owns that.
//
// Two facts decide whether this kind is usable on a machine, and both sit outside this code:
//
//   - Container to container networking arrived in macOS 26. On macOS 15 a session joins no network
//     that reaches the control plane, so every call it makes fails to resolve the name.
//   - `container system start` is the operator's to run. Every command here fails while that service
//     is down, and the failure is returned rather than read as an empty machine.
//
// The fields are the Docker backend's fields, in the same order, so one set of options builds either
// one.
type AppleProvider struct {
	// Image is the container image sessions run in.
	Image string
	// Mounts are extra bind mounts for every sandbox, each "host:container[:ro]".
	Mounts []string
	// Storage keeps the workspace's conversation store and the project's files on the host.
	Storage Storage
	// Network is the system's own network, and the driver is the only sandbox on it.
	Network string
	// SessionNetwork is the network every sandbox joins to reach the control plane.
	SessionNetwork string
	// DriverMounts are host paths the driver gets and an ordinary session does not, each
	// "host:container[:ro]".
	DriverMounts []string
	// Memory is how much memory one session may take, for example "4g". The tool reads the same
	// spellings the Docker daemon reads, and it reads them as binary sizes, so one figure means one
	// thing under both backends.
	//
	// Empty is not "no limit" here, which is where the two backends part. The tool gives a container
	// 1024 mebibytes of its own accord, so a session left unset under this backend is a small session
	// rather than an unbounded one.
	Memory string
}

var _ Provider = AppleProvider{}

// Create starts a detached container for the session and answers with a sandbox that execs into it.
//
// A container already carrying the session's name is adopted rather than refused, and started if it
// stopped, for the reason the Docker backend gives: a session's name is deterministic, so a control
// plane that forgot its sandboxes could otherwise never start that session again.
func (a AppleProvider) Create(ctx context.Context, cfg Config) (Sandbox, error) {
	if a.Image == "" {
		return nil, fmt.Errorf("sandbox: apple container image is required")
	}
	kept, err := a.Storage.Prepare(cfg)
	if err != nil {
		return nil, err
	}

	adopted, err := a.adopt(ctx, cfg.ID)
	if err != nil {
		return nil, err
	}
	if adopted != nil {
		return adopted, nil
	}

	name := ContainerName(cfg.ID)
	args := a.runArgs(name, cfg, kept)
	if out, err := exec.CommandContext(ctx, appleBinary, args...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("sandbox: create container: %w: %s", err, out)
	}
	return &appleSandbox{name: name}, nil
}

// adopt answers with a sandbox over the container this session already holds, under either name, and
// nil where the tool holds none. A container that stopped is started first, because it still holds
// the session's files and the credential helper that reaches its remote.
func (a AppleProvider) adopt(ctx context.Context, sessionID string) (Sandbox, error) {
	holds, err := a.holdings(ctx)
	if err != nil {
		return nil, err
	}
	for _, name := range ContainerNames(sessionID) {
		running, held := holds[name]
		if !held {
			continue
		}
		if !running {
			if out, err := exec.CommandContext(ctx, appleBinary, "start", name).CombinedOutput(); err != nil {
				return nil, fmt.Errorf("sandbox: start existing container %s: %w: %s", name, err, out)
			}
		}
		return &appleSandbox{name: name}, nil
	}
	return nil, nil
}

// holdings is every container the tool holds, and whether each one runs.
//
// It reads two quiet listings rather than one inspect. The tool prints a document from `inspect`
// whose shape moved between its releases, and a reader tied to one shape reports every container as
// absent under the other. A quiet listing is one identifier per line in every release, and the
// identifier is the name the create gave.
func (a AppleProvider) holdings(ctx context.Context) (map[string]bool, error) {
	all, err := a.listing(ctx, "--all", "--quiet")
	if err != nil {
		return nil, err
	}
	running, err := a.listing(ctx, "--quiet")
	if err != nil {
		return nil, err
	}
	holds := map[string]bool{}
	for _, name := range strings.Fields(all) {
		holds[name] = false
	}
	for _, name := range strings.Fields(running) {
		holds[name] = true
	}
	return holds, nil
}

// listing asks the tool for containers. Without --all it names only the ones that run.
func (a AppleProvider) listing(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, appleBinary, append([]string{"list"}, args...)...).Output()
	if err != nil {
		return "", fmt.Errorf("sandbox: list containers: %w", err)
	}
	return string(out), nil
}

// Existing is a sandbox over the container this session already has, and false where the tool holds
// none by that name. It never creates one.
func (a AppleProvider) Existing(ctx context.Context, sessionID string) (Sandbox, bool, error) {
	box, err := a.adopt(ctx, sessionID)
	if err != nil {
		return nil, false, err
	}
	if box == nil {
		return nil, false, nil
	}
	return box, true, nil
}

// Remove tears down the container carrying this session's name, held or not, under either name.
//
// A name the tool does not list is a remove that already happened, so the listing is what decides
// rather than the words a failed delete prints. A tool that cannot be reached at all is a failure and
// is returned, because a system that reported the session stopped would leave the container running
// and holding its memory.
func (a AppleProvider) Remove(ctx context.Context, sessionID string) error {
	holds, err := a.holdings(ctx)
	if err != nil {
		return err
	}
	for _, name := range ContainerNames(sessionID) {
		if _, held := holds[name]; !held {
			continue
		}
		out, err := exec.CommandContext(ctx, appleBinary, "delete", "--force", name).CombinedOutput()
		if err != nil {
			return fmt.Errorf("sandbox: remove container: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// Stranded lists the sessions whose containers the tool still holds, running or not.
//
// SessionOf takes either name and holds both to the exact shape of a session identifier, so a
// container that is not a sandbox stays out of the listing and nobody reaps it.
func (a AppleProvider) Stranded(ctx context.Context) ([]string, error) {
	out, err := a.listing(ctx, "--all", "--quiet")
	if err != nil {
		return nil, err
	}
	return sessionsAmong(out), nil
}

// Attached asks the container whether the operator's conversation has anybody watching it. It is the
// Docker backend's question through this tool, and it means what that one means:
//
//   - a client listed, so somebody is attached.
//   - the command ran and exited non zero, so there is no tmux server or no such conversation. Nobody
//     is typing into it either way.
//   - no container at all, so nobody is attached to it.
//   - the tool could not run, so the system cannot tell. The error goes back rather than a false,
//     because a caller must not read it as nobody.
func (a AppleProvider) Attached(ctx context.Context, sessionID string) (bool, error) {
	name, out, err := a.execInSession(ctx, sessionID,
		"tmux", "list-clients", "-t", AttachedSessionName, "-F", "#{client_name}")
	if err != nil {
		if quiet, decided := emptySandbox(err); decided {
			return quiet, nil
		}
		return false, fmt.Errorf("sandbox: ask %s whether anybody is attached: %w", name, err)
	}
	return strings.TrimSpace(out) != "", nil
}

// RuntimeRunning asks the container's own process table whether a model runtime is up in it. The
// answers mean what Attached's mean, and for the same reasons.
func (a AppleProvider) RuntimeRunning(ctx context.Context, sessionID string) (bool, error) {
	name, out, err := a.execInSession(ctx, sessionID, "sh", "-c", processTable)
	if err != nil {
		if quiet, decided := emptySandbox(err); decided {
			return quiet, nil
		}
		return false, fmt.Errorf("sandbox: ask %s what it is running: %w", name, err)
	}
	return runtimeAmong(out), nil
}

// errNoSandbox is the tool holding no container for this session. A session with no container has
// nobody in it and runs nothing, which is an answer rather than a failure.
var errNoSandbox = errors.New("sandbox: this session has no container")

// emptySandbox reads a failed question as an answer where it is one, and says whether it decided.
// Absent and refused both mean an empty sandbox. Anything else is the system being unable to tell.
func emptySandbox(err error) (bool, bool) {
	if errors.Is(err, errNoSandbox) {
		return false, true
	}
	var exited *exec.ExitError
	if errors.As(err, &exited) {
		return false, true
	}
	return false, false
}

// execInSession runs a command inside this session's container, under whichever name the tool holds
// it, and answers with the name it reached.
//
// The name comes from the listing rather than from a failed attempt under each name in turn. A
// sandbox read through the wrong name answers exactly like an empty one, and an empty sandbox is what
// invites a drain, a restart or a reclaim over a conversation somebody is typing into.
func (a AppleProvider) execInSession(ctx context.Context, sessionID string, argv ...string) (string, string, error) {
	holds, err := a.holdings(ctx)
	if err != nil {
		return "", "", err
	}
	for _, name := range ContainerNames(sessionID) {
		if _, held := holds[name]; !held {
			continue
		}
		out, err := exec.CommandContext(ctx, appleBinary, append([]string{"exec", name}, argv...)...).Output()
		return name, string(out), err
	}
	return "", "", errNoSandbox
}

// runArgs is the whole `container run` for a session's sandbox. It is a function of its own so a test
// can read what the tool would be asked for without the tool: whether the sandbox joins a network at
// all is the difference between a session that can drive the system and one that cannot.
func (a AppleProvider) runArgs(name string, cfg Config, kept []Mount) []string {
	args := []string{"run", "--detach", "--name", name, "--tmpfs", secretsMount()}
	// The processor half of a request is dropped here, and the drop is the decision.
	//
	// A request is not a limit: a sandbox alone on an idle machine may take more than it asked for,
	// and the Docker backend says that with a share, which binds only while the machine is contended.
	// This tool offers no share. It offers --cpus, which allocates that many processors to the virtual
	// machine and caps the sandbox at them. Passing the request there would turn it into the limit the
	// system says it is not, so the tool's own allocation stands and the system's admission control
	// keeps counting requests.
	if a.Memory != "" {
		// No swap flag beside it, as there is under Docker. A guest of this tool has no swap to cap,
		// so the figure is already the whole of what a session may take.
		args = append(args, "--memory", a.Memory)
	}
	if network := a.networkFor(cfg); network != "" {
		args = append(args, "--network", network)
	}
	if cfg.Driver {
		for _, mount := range a.DriverMounts {
			args = append(args, "--volume", mount)
		}
	}
	for _, entry := range cfg.Env {
		args = append(args, "--env", entry)
	}
	for _, mount := range append(kept, cfg.Mounts...) {
		if mount.ReadOnly {
			args = append(args, "--volume", mount.Source+":"+mount.Target+":ro")
			continue
		}
		args = append(args, "--volume", mount.Source+":"+mount.Target)
	}
	for _, mount := range a.Mounts {
		args = append(args, "--volume", mount)
	}
	return append(args, a.Image, "sleep", "infinity")
}

// networkFor is the one network this sandbox joins, and it is the rule the Docker backend follows: a
// session joins the session network, and the driver joins the system's own network where the operator
// named one. There is no promotion, because a container that runs already joins nothing later.
func (a AppleProvider) networkFor(cfg Config) string {
	if cfg.Driver && a.Network != "" {
		return a.Network
	}
	return a.SessionNetwork
}

type appleSandbox struct {
	name string
}

var _ Sandbox = (*appleSandbox)(nil)

// Name is what the tool calls this container, which is not always what ContainerName would build: a
// sandbox adopted from before the rename answers to the retired name.
func (s *appleSandbox) Name() string { return s.name }

// Exec runs spec.Argv inside the session's container and streams its stdout.
func (s *appleSandbox) Exec(ctx context.Context, spec Spec) (Process, error) {
	if len(spec.Argv) == 0 {
		return nil, fmt.Errorf("sandbox: empty argv")
	}
	cmd := exec.CommandContext(ctx, appleBinary, appleExecArgs(s.name, spec)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("sandbox: stdout pipe: %w", err)
	}
	proc := newCmdProcess(cmd, stdout)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("sandbox: apple container exec: %w", err)
	}
	return proc, nil
}

// appleExecArgs is the whole `container exec` for one command, as a function so a test can read it
// without the tool. The long names are deliberate: this tool spells the working directory --workdir
// and --cwd, and one of those is not the Docker word at all.
func appleExecArgs(name string, spec Spec) []string {
	args := []string{"exec", "--interactive"}
	if spec.Workdir != "" {
		args = append(args, "--workdir", spec.Workdir)
	}
	for _, env := range spec.Env {
		args = append(args, "--env", env)
	}
	args = append(args, name)
	return append(args, spec.Argv...)
}

// Close removes the session's container.
func (s *appleSandbox) Close(ctx context.Context) error {
	if err := exec.CommandContext(ctx, appleBinary, "delete", "--force", s.name).Run(); err != nil {
		return fmt.Errorf("sandbox: remove container: %w", err)
	}
	return nil
}
