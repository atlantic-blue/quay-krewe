package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/atlantic-blue/quay-krewe/internal/capacity"
)

// MacOSProvider gives each session a macOS virtual machine, so a session can build and test a darwin
// application. It drives tart, which wraps Apple's Virtualization framework and pulls a macOS image
// the way a container runtime pulls one.
//
// It answers the same five questions the container backend answers, and the cost behind each one is
// different. A container starts in about a second and an image is a few hundred megabytes; a macOS
// guest boots in tens of seconds and an image is tens of gigabytes. So Stranded and Existing matter
// more here, not less: a guest nobody reaps holds a licence slot and tens of gigabytes of disk, and a
// system that cannot see the guest a session already has boots a second one for it.
//
// The licence, not the hardware, is what makes a guest scarce. See LicensedGuests. A session that
// asks for a guest while both are taken waits in guestPool rather than failing.
//
// Two things a container backend does are not done here, because the runtime cannot do them:
//
//   - A host directory reaches the guest as a virtio share rather than as a bind mount, so the guest
//     sees it under its own mount point and not at the path the sandbox image uses. The guest mounts
//     it where it wants it.
//   - A processor request is an allocation here rather than a share. tart fixes the processor count
//     and the memory of a guest before it boots, so a guest alone on an idle machine does not take
//     the machine the way a container does.
//   - A sandbox environment is carried by each command rather than by the machine. A container is
//     created with its environment and keeps it; a guest is a full operating system that was
//     installed long before this session existed. So a guest adopted by a process that did not
//     create it runs commands without that environment until the session is created again.
type MacOSProvider struct {
	// Image is the virtual machine a session's guest is cloned from, local or in a registry, for
	// example "ghcr.io/cirruslabs/macos-tahoe-xcode:latest". The clone is copy on write, so a guest
	// costs the changes it makes rather than the whole image.
	Image string
	// Tart is the command that drives the guests. Empty means "tart" on the path.
	Tart string
	// Guests is how many guests this host may run at once. Empty means LicensedGuests, which is what
	// Apple's own agreement permits.
	Guests int
	// Directories are extra host directories every guest gets, each in tart's own form,
	// "[name:]path[:options]".
	Directories []string
	// Storage keeps the workspace's conversation store and the project's files on the host, the same
	// way the container backend does. They reach the guest as shares.
	Storage Storage

	once sync.Once
	pool *guestPool
}

var _ Provider = (*MacOSProvider)(nil)

// NewMacOSProvider builds the backend from the options a composition root holds.
func NewMacOSProvider(opts Options) *MacOSProvider {
	return &MacOSProvider{Image: opts.Image, Directories: opts.Mounts, Storage: opts.Storage}
}

// guestReady is how often a starting guest is asked whether it can run a command yet.
const guestReady = time.Second

// macOSProcessTable dumps the command line of everything running in the guest, one process per line.
//
// macOS has no /proc, so the reader the container backend uses finds nothing at all in a guest. This
// asks the same question the other way, and the answer is read by the same code.
const macOSProcessTable = "ps -Ao args="

// tart is the command this provider drives.
func (m *MacOSProvider) tart() string {
	if m.Tart == "" {
		return "tart"
	}
	return m.Tart
}

// guestPool is built once, because the pool is the thing that keeps two sessions from starting a
// third guest between them.
func (m *MacOSProvider) guestPool() *guestPool {
	m.once.Do(func() { m.pool = newGuestPool(m.Guests, m.sessionsRunning) })
	return m.pool
}

// Create hands this session a running guest it can exec in, and waits where the host is full.
//
// A guest already carrying the session's name is adopted rather than refused, and started where it
// had stopped, for the reason the container backend adopts a container: the name is derived from the
// session, so a control plane that has forgotten its sandboxes could otherwise never start that
// session again.
func (m *MacOSProvider) Create(ctx context.Context, cfg Config) (Sandbox, error) {
	if m.Image == "" {
		return nil, fmt.Errorf("sandbox: macos image is required")
	}
	kept, err := m.Storage.Prepare(cfg)
	if err != nil {
		return nil, err
	}
	// Before anything is cloned or booted, because the licence limit is on guests that run and a
	// clone that cannot be booted is tens of gigabytes nobody asked for.
	if err := m.guestPool().hold(ctx, cfg.ID); err != nil {
		return nil, err
	}

	box, err := m.create(ctx, cfg, kept)
	if err != nil {
		m.guestPool().release(cfg.ID)
		return nil, err
	}
	return box, nil
}

// create is Create with the guest already held, so one release covers every way out.
func (m *MacOSProvider) create(ctx context.Context, cfg Config, kept []Mount) (Sandbox, error) {
	// Either name, because a sandbox that started before the rename carries the retired one.
	for _, existing := range ContainerNames(cfg.ID) {
		held, found, err := m.guestNamed(ctx, existing)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		if !held.Running {
			if err := m.boot(ctx, existing, cfg, kept); err != nil {
				return nil, err
			}
		}
		return &macOSSandbox{provider: m, session: cfg.ID, name: existing, env: cfg.Env}, nil
	}

	name := ContainerName(cfg.ID)
	if out, err := m.run(ctx, "clone", m.Image, name); err != nil {
		return nil, fmt.Errorf("sandbox: clone %s into %s: %w: %s", m.Image, name, err, out)
	}
	if err := m.size(ctx, name, cfg); err != nil {
		return nil, err
	}
	if err := m.boot(ctx, name, cfg, kept); err != nil {
		return nil, err
	}
	return &macOSSandbox{provider: m, session: cfg.ID, name: name, env: cfg.Env}, nil
}

// size fixes the processor count and the memory of a guest before it boots.
//
// A guest cannot be resized while it runs, so what a session asked for has to be set here or not at
// all. A request that names neither leaves the image's own figures alone.
func (m *MacOSProvider) size(ctx context.Context, name string, cfg Config) error {
	args := []string{"set", name}
	if processors := cfg.Request.Processor / capacity.OneProcessor; processors > 0 {
		args = append(args, "--cpu", strconv.Itoa(processors))
	}
	if megabytes := cfg.Request.Memory / (1 << 20); megabytes > 0 {
		args = append(args, "--memory", strconv.FormatInt(megabytes, 10))
	}
	if len(args) == 2 {
		return nil
	}
	if out, err := m.run(ctx, args...); err != nil {
		return fmt.Errorf("sandbox: size guest %s: %w: %s", name, err, out)
	}
	return nil
}

// boot starts the guest and waits until it can run a command.
//
// The guest is the process tart starts, so it is left running rather than waited on, and it is put in
// a process group of its own. Without that, anything that signals the control plane's whole group
// takes every guest on the machine down with it, and a guest costs tens of seconds to bring back.
//
// Waiting until a command runs, rather than until the runtime says the guest is up, is what makes the
// answer mean something to the caller. A guest that is listed as running has not finished booting,
// and an exec against it fails.
func (m *MacOSProvider) boot(ctx context.Context, name string, cfg Config, kept []Mount) error {
	cmd := exec.Command(m.tart(), m.runArgs(name, cfg, kept)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	said := &tail{limit: stderrTail}
	cmd.Stderr = said
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("sandbox: start guest %s: %w", name, err)
	}
	stopped := make(chan error, 1)
	go func() { stopped <- cmd.Wait() }()

	for {
		if _, err := m.run(ctx, "exec", name, "true"); err == nil {
			return nil
		}
		select {
		case err := <-stopped:
			return fmt.Errorf("sandbox: guest %s stopped before it was ready: %w: %s", name, err, said.String())
		case <-ctx.Done():
			return fmt.Errorf("sandbox: guest %s did not become ready: %w: %s", name, ctx.Err(), said.String())
		case <-time.After(guestReady):
		}
	}
}

// runArgs is the whole `tart run` for a session's guest. It is a function of its own so a test can
// read what the runtime would be asked for without a machine to run it on.
func (m *MacOSProvider) runArgs(name string, cfg Config, kept []Mount) []string {
	args := []string{"run", "--no-graphics"}
	for _, mount := range append(kept, cfg.Mounts...) {
		args = append(args, "--dir="+share(mount))
	}
	for _, directory := range m.Directories {
		args = append(args, "--dir="+directory)
	}
	return append(args, name)
}

// share is one host directory in the form tart takes, "name:path[:ro]".
//
// The name is the last part of where the container backend would have mounted it, because that is the
// only part of the path the guest can keep: a share arrives under the runtime's own mount point
// rather than at a path the host chooses.
func share(mount Mount) string {
	name := strings.Trim(mount.Target, "/")
	if cut := strings.LastIndex(name, "/"); cut >= 0 {
		name = name[cut+1:]
	}
	one := name + ":" + mount.Source
	if mount.ReadOnly {
		one += ":ro"
	}
	return one
}

// macOSSandbox is one session's guest.
type macOSSandbox struct {
	provider *MacOSProvider
	session  string
	name     string
	// env is the sandbox own environment, applied to every command, standing in for the environment a
	// container is created with.
	env []string
}

var _ Sandbox = (*macOSSandbox)(nil)

// Name is what the runtime calls this guest, which an operator's own tart command needs.
func (s *macOSSandbox) Name() string { return s.name }

// Exec runs spec.Argv inside the session's guest and streams its stdout.
//
// tart exec takes a command and nothing else, so a working directory and an environment are carried
// by the command itself. The guest needs the tart guest agent for this, which the images the project
// publishes already carry.
func (s *macOSSandbox) Exec(ctx context.Context, spec Spec) (Process, error) {
	if len(spec.Argv) == 0 {
		return nil, fmt.Errorf("sandbox: empty argv")
	}
	args := append([]string{"exec", s.name}, inGuest(s.env, spec)...)
	cmd := exec.CommandContext(ctx, s.provider.tart(), args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("sandbox: stdout pipe: %w", err)
	}
	proc := newCmdProcess(cmd, stdout)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("sandbox: tart exec: %w", err)
	}
	return proc, nil
}

// inGuest is the command as the guest runs it, with the working directory and the environment the
// spec asked for wrapped around it.
//
// Both are flags of `docker exec` and neither is a flag of `tart exec`, so they are said in the
// command instead. The values travel as arguments to a shell rather than inside the script, so a
// value holding a quote is a value and never a second command.
func inGuest(env []string, spec Spec) []string {
	argv := spec.Argv
	// The sandbox own environment first, then the command own, so a per command value wins the way it
	// would inside a container.
	if carried := append(append([]string{}, env...), spec.Env...); len(carried) > 0 {
		argv = append(append([]string{"env"}, carried...), argv...)
	}
	if spec.Workdir == "" {
		return argv
	}
	return append([]string{"sh", "-c", `cd -- "$1" || exit 1; shift; exec "$@"`, "sh", spec.Workdir}, argv...)
}

// Close takes this session's guest down and gives its place back.
func (s *macOSSandbox) Close(ctx context.Context) error {
	return s.provider.Remove(ctx, s.session)
}

// Remove takes down the guest carrying this session's name, held or not, and gives its place back to
// whatever is waiting.
//
// Absent is success, under either name, for the reason the container backend takes both: a session
// whose guest started before the rename is stopped by an operator who has upgraded since, and a
// removal that only knew the new name would report the session stopped while its guest kept running
// and kept its place.
func (m *MacOSProvider) Remove(ctx context.Context, sessionID string) error {
	defer m.guestPool().release(sessionID)
	for _, name := range ContainerNames(sessionID) {
		if out, err := m.run(ctx, "stop", name); err != nil && !guestAbsent(err) && !guestStopped(err) {
			return fmt.Errorf("sandbox: stop guest %s: %w: %s", name, err, out)
		}
		if out, err := m.run(ctx, "delete", name); err != nil && !guestAbsent(err) {
			return fmt.Errorf("sandbox: delete guest %s: %w: %s", name, err, out)
		}
	}
	return nil
}

// Existing is a sandbox over the guest this session already has, and false where there is none. It
// never clones one: it is how the system reaches into a session that has finished rather than
// starting one that has not.
//
// A guest that has stopped is started, the way Create adopts one, because a stopped guest still holds
// the session's files.
func (m *MacOSProvider) Existing(ctx context.Context, sessionID string) (Sandbox, bool, error) {
	for _, name := range ContainerNames(sessionID) {
		held, found, err := m.guestNamed(ctx, name)
		if err != nil {
			return nil, false, err
		}
		if !found {
			continue
		}
		if held.Running {
			// No environment: this is the guest of a session this process may never have created, and
			// tart holds none on the machine to read back. See the note on MacOSProvider.
			// Held rather than admitted: the guest is already up, so this takes no new place, and a
			// later Remove has a place to give back.
			if err := m.guestPool().hold(ctx, sessionID); err != nil {
				return nil, false, err
			}
			return &macOSSandbox{provider: m, session: sessionID, name: name}, true, nil
		}
		if err := m.guestPool().hold(ctx, sessionID); err != nil {
			return nil, false, err
		}
		if err := m.boot(ctx, name, Config{ID: sessionID}, nil); err != nil {
			m.guestPool().release(sessionID)
			return nil, false, err
		}
		return &macOSSandbox{provider: m, session: sessionID, name: name}, true, nil
	}
	return nil, false, nil
}

// Stranded lists the sessions whose guests the runtime still holds, running or not.
//
// A stopped guest is listed as well as a running one, for the same reason the container backend lists
// a stopped container: it is tens of gigabytes of disk belonging to a session that may be long gone.
func (m *MacOSProvider) Stranded(ctx context.Context) ([]string, error) {
	held, err := m.guests(ctx)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, one := range held {
		if id, isSandbox := SessionOf(one.Name); isSandbox {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// Attached says whether somebody has this session's conversation open inside its guest. It is the
// same question the container backend asks, put to the same multiplexer through tart.
func (m *MacOSProvider) Attached(ctx context.Context, sessionID string) (bool, error) {
	name, out, err := m.execInSession(ctx, sessionID,
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

// RuntimeRunning asks the guest's own process table whether a model runtime is up in it.
//
// The contract is Attached's on purpose. A command that ran and failed is nothing running; a command
// that could not be run at all is the system being unable to tell, and the caller must not read that
// as an empty guest.
func (m *MacOSProvider) RuntimeRunning(ctx context.Context, sessionID string) (bool, error) {
	name, out, err := m.execInSession(ctx, sessionID, "sh", "-c", macOSProcessTable)
	if err != nil {
		var exited *exec.ExitError
		if errors.As(err, &exited) {
			return false, nil
		}
		return false, fmt.Errorf("sandbox: ask %s what it is running: %w", name, err)
	}
	return runtimeAmong(out), nil
}

// execInSession runs a command inside this session's guest, under whichever name the runtime holds
// it, and answers with the name it reached. The name this build writes is asked first, so a guest
// started since the rename costs no second lookup.
func (m *MacOSProvider) execInSession(ctx context.Context, sessionID string, argv ...string) (string, string, error) {
	var name string
	var out string
	var err error
	for _, name = range ContainerNames(sessionID) {
		out, err = m.run(ctx, append([]string{"exec", name}, argv...)...)
		if err == nil || !guestAbsent(err) {
			break
		}
	}
	return name, out, err
}

// guest is one virtual machine as the runtime lists it.
type guest struct {
	Name    string
	Running bool
	State   string
}

// guests is every guest the runtime holds locally. The images it has pulled are left out: one of
// those is not a machine anybody is running.
func (m *MacOSProvider) guests(ctx context.Context) ([]guest, error) {
	out, err := m.run(ctx, "list", "--source", "local", "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("sandbox: list guests: %w: %s", err, out)
	}
	var listed []guest
	if err := json.Unmarshal([]byte(out), &listed); err != nil {
		return nil, fmt.Errorf("sandbox: read the guest listing: %w", err)
	}
	return listed, nil
}

// guestNamed is the guest carrying this name, and false where the runtime holds none.
func (m *MacOSProvider) guestNamed(ctx context.Context, name string) (guest, bool, error) {
	held, err := m.guests(ctx)
	if err != nil {
		return guest{}, false, err
	}
	for _, one := range held {
		if one.Name == name {
			return one, true, nil
		}
	}
	return guest{}, false, nil
}

// sessionsRunning is the sessions this host is running a guest for, which is what the pool counts.
func (m *MacOSProvider) sessionsRunning(ctx context.Context) ([]string, error) {
	held, err := m.guests(ctx)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, one := range held {
		if !one.Running {
			continue
		}
		if id, isSandbox := SessionOf(one.Name); isSandbox {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// run asks the runtime one question and answers with what it said.
func (m *MacOSProvider) run(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, m.tart(), args...).Output()
	return string(out), err
}

// guestAbsent is the runtime saying it holds no machine by that name, which is the one failure worth
// asking a second name about.
func guestAbsent(err error) bool { return said(err, "does not exist") }

// guestStopped is the runtime saying the machine is there and is not running, which is what stopping
// an already stopped guest answers. It is success rather than a failure: the guest is down, which is
// what the caller asked for.
func guestStopped(err error) bool { return said(err, "is not running") }

// said reads what the runtime wrote on the error stream of a command that failed.
func said(err error, phrase string) bool {
	var exited *exec.ExitError
	if !errors.As(err, &exited) {
		return false
	}
	return strings.Contains(string(exited.Stderr), phrase)
}
