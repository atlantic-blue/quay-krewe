package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ContainerdBinary is the command line this provider drives when nothing names another one.
//
// nerdctl carries Docker's verbs and Docker's flags over containerd, so what a session's container
// is asked for is built once, by the Docker backend, and sent to this command instead. The parts
// that differ are the parts that read an answer back, and they are here.
const ContainerdBinary = "nerdctl"

// ContainerdProvider gives each session its own long lived container on containerd.
//
// containerd is the runtime the Docker daemon already runs containers with, and it is what
// Kubernetes moved to when it dropped the Docker shim in version 1.24. Running it directly drops the
// daemon and the build daemon above it, from the same images. On a Linux host that removes both. On
// a Mac containerd still needs a Linux virtual machine under it, through Lima or Colima, so there it
// removes the daemon and keeps the virtual machine.
//
// Everything a session gets is the Docker backend's to decide: the network, the memory, the mounts
// and the secrets directory are built by DockerProvider.runArgs and handed to this command line. So
// the two runtimes cannot be given different containers, and a flag added for one reaches the other.
type ContainerdProvider struct {
	// Options are what every sandbox of this provider is created with, the same set the Docker
	// backend takes.
	Options
	// Binary is the command line to drive, empty meaning nerdctl on the path. An operator whose
	// nerdctl is wrapped names the wrapper here: on a Mac through Lima it is `nerdctl.lima`.
	Binary string
}

var _ Provider = ContainerdProvider{}

// command is the command line this provider drives.
func (c ContainerdProvider) command() string {
	if c.Binary == "" {
		return ContainerdBinary
	}
	return c.Binary
}

// compatible is the Docker backend over the same options, which is where the arguments for a
// session's container are built. They are one function so the two runtimes cannot drift: a session
// under containerd joins the network the Docker one joins, gets the mounts it gets, and is held to
// the memory it is held to.
func (c ContainerdProvider) compatible() DockerProvider { return DockerProvider(c.Options) }

// Create starts a detached container for the session and returns a sandbox that execs into it.
//
// A container already carrying the session's name is adopted rather than refused, and started if it
// had stopped, for the reason the Docker backend adopts one: a session's name is deterministic, so a
// control plane that has forgotten its sandboxes could otherwise never start that session again.
func (c ContainerdProvider) Create(ctx context.Context, cfg Config) (Sandbox, error) {
	if c.Image == "" {
		return nil, fmt.Errorf("sandbox: containerd image is required")
	}
	kept, err := c.Storage.Prepare(cfg)
	if err != nil {
		return nil, err
	}

	// Either name, because a sandbox that started before the rename carries the retired one.
	for _, existing := range ContainerNames(cfg.ID) {
		adopted, err := c.adopt(ctx, existing)
		if err != nil {
			return nil, err
		}
		if adopted != nil {
			return adopted, nil
		}
	}

	name := ContainerName(cfg.ID)
	args := c.compatible().runArgs(name, cfg, kept)

	if out, err := exec.CommandContext(ctx, c.command(), args...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("sandbox: create container: %w: %s", err, out)
	}
	return &containerdSandbox{binary: c.command(), name: name}, nil
}

// adopt returns a sandbox over an existing container, starting it when it had stopped, or nil when
// there is no container by that name to adopt.
//
// nerdctl answers inspect in Docker's own shape, so the state is read at the same place: its
// ContainerState carries Running, under State.
func (c ContainerdProvider) adopt(ctx context.Context, name string) (Sandbox, error) {
	out, err := exec.CommandContext(ctx, c.command(), "inspect", "--format", "{{.State.Running}}", name).Output()
	if err != nil {
		// Nothing by that name. Anything else the runtime has to say about it will be said again, and
		// more usefully, by the create above.
		return nil, nil
	}
	if strings.TrimSpace(string(out)) == "true" {
		return &containerdSandbox{binary: c.command(), name: name}, nil
	}
	if out, err := exec.CommandContext(ctx, c.command(), "start", name).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("sandbox: start existing container %s: %w: %s", name, err, out)
	}
	return &containerdSandbox{binary: c.command(), name: name}, nil
}

type containerdSandbox struct {
	binary string
	name   string
}

var _ Sandbox = (*containerdSandbox)(nil)
var _ Named = (*containerdSandbox)(nil)

// Name is what the runtime calls this container, which is not always what ContainerName would build:
// a sandbox adopted from before the rename answers to the retired name.
func (s *containerdSandbox) Name() string { return s.name }

// Exec runs spec.Argv inside the session's container and streams its stdout.
func (s *containerdSandbox) Exec(ctx context.Context, spec Spec) (Process, error) {
	if len(spec.Argv) == 0 {
		return nil, fmt.Errorf("sandbox: empty argv")
	}
	args := []string{"exec", "-i"}
	if spec.Workdir != "" {
		args = append(args, "-w", spec.Workdir)
	}
	for _, env := range spec.Env {
		args = append(args, "-e", env)
	}
	args = append(args, s.name)
	args = append(args, spec.Argv...)

	cmd := exec.CommandContext(ctx, s.binary, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("sandbox: stdout pipe: %w", err)
	}
	proc := newCmdProcess(cmd, stdout)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("sandbox: containerd exec: %w", err)
	}
	return proc, nil
}

// Close removes the session's container.
func (s *containerdSandbox) Close(ctx context.Context) error {
	if err := exec.CommandContext(ctx, s.binary, "rm", "-f", s.name).Run(); err != nil {
		return fmt.Errorf("sandbox: remove container: %w", err)
	}
	return nil
}

// Remove tears down the container carrying this session's name, held or not. Absent is success.
//
// Both names, and neither absence is a failure, for the reason the Docker backend takes both: a
// session whose sandbox started before the rename is stopped by an operator who has upgraded since,
// and a remove that only knew the new name would report the session stopped while its container kept
// running and kept its memory.
func (c ContainerdProvider) Remove(ctx context.Context, sessionID string) error {
	for _, name := range ContainerNames(sessionID) {
		out, err := exec.CommandContext(ctx, c.command(), "rm", "-f", name).CombinedOutput()
		if err == nil || absentContainer(string(out)) {
			continue
		}
		return fmt.Errorf("sandbox: remove container: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Stranded lists the sessions whose containers the runtime still holds, running or not.
func (c ContainerdProvider) Stranded(ctx context.Context) ([]string, error) {
	out, err := exec.CommandContext(ctx, c.command(), "ps", "--all", "--format", "{{.Names}}").Output()
	if err != nil {
		return nil, fmt.Errorf("sandbox: list containers: %w", err)
	}
	return sessionsAmong(string(out)), nil
}

// Attached asks the container whether the operator's conversation has anybody watching it, which is
// one more exec against the tmux server inside it.
//
// The three answers mean what they mean for Docker: a client listed is somebody attached; a non zero
// exit is no tmux server, no such conversation or no such container, and nobody is typing into any of
// those; and a command that could not be run at all is the runtime being unreachable, which is
// returned rather than swallowed because a caller must not read it as nobody.
func (c ContainerdProvider) Attached(ctx context.Context, sessionID string) (bool, error) {
	name, out, err := c.execInSession(ctx, sessionID,
		"tmux", "list-clients", "-t", AttachedSessionName, "-F", "#{client_name}")
	if err != nil {
		var exited *exec.ExitError
		if errors.As(err, &exited) {
			return false, nil
		}
		return false, fmt.Errorf("sandbox: ask %s whether anybody is attached: %w", name, err)
	}
	return strings.TrimSpace(out) != "", nil
}

// RuntimeRunning asks the container's own process table whether a model runtime is up in it. The
// table is gathered by the same script the Docker backend uses and read by the same function, so a
// session holding a conversation is seen the same way under either runtime.
func (c ContainerdProvider) RuntimeRunning(ctx context.Context, sessionID string) (bool, error) {
	name, out, err := c.execInSession(ctx, sessionID, "sh", "-c", processTable)
	if err != nil {
		var exited *exec.ExitError
		if errors.As(err, &exited) {
			return false, nil
		}
		return false, fmt.Errorf("sandbox: ask %s what it is running: %w", name, err)
	}
	return runtimeAmong(out), nil
}

// Existing is a sandbox over the container this session already has, and false where the runtime
// holds none by that name. It never creates one.
func (c ContainerdProvider) Existing(ctx context.Context, sessionID string) (Sandbox, bool, error) {
	for _, name := range ContainerNames(sessionID) {
		box, err := c.adopt(ctx, name)
		if err != nil {
			return nil, false, err
		}
		if box != nil {
			return box, true, nil
		}
	}
	return nil, false, nil
}

// execInSession runs a command inside this session's container, under whichever name the runtime
// holds it, and answers with the name it reached.
//
// A sandbox read through the wrong name answers exactly like an empty one, and an empty sandbox is
// what invites a drain, a restart or a reclaim over the top of a conversation somebody is typing
// into.
func (c ContainerdProvider) execInSession(ctx context.Context, sessionID string, argv ...string) (string, string, error) {
	var name string
	var out []byte
	var err error
	for _, name = range ContainerNames(sessionID) {
		cmd := exec.CommandContext(ctx, c.command(), append([]string{"exec", name}, argv...)...)
		out, err = cmd.Output()
		if err == nil || !noSuchContainerd(err) {
			break
		}
	}
	return name, string(out), err
}

// noSuchContainerd is the runtime saying it holds nothing by that name, which is the one failure
// worth asking a second name about.
func noSuchContainerd(err error) bool {
	var exited *exec.ExitError
	if !errors.As(err, &exited) {
		return false
	}
	return absentContainer(string(exited.Stderr))
}

// absentContainer reads "there is nothing by that name" out of what the runtime said.
//
// The two runtimes spell it with different capitals: Docker writes "No such container" and nerdctl
// writes "no such container: <name>". The words are the same, so the case is folded rather than a
// second spelling being carried.
func absentContainer(said string) bool {
	return strings.Contains(strings.ToLower(said), "no such container")
}
