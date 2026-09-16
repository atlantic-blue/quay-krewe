// Command faketart stands in for tart, so the macOS backend can be driven end to end on a machine
// that is not an Apple one.
//
// It is a double, and a double that is looser than the thing it stands for manufactures a green
// suite. So every answer here is taken from tart's own source at commit 9bb2af24, 9 September 2026:
// the words of each failure, the shape of the listing, and which commands refuse what. Where the two
// could differ, this one is the stricter.
//
// What it does not stand for is the virtual machine. A command runs on this host rather than in a
// guest, nothing boots, and no image is pulled. So it proves the backend asks the right questions and
// reads the answers, and it proves nothing at all about Apple's Virtualization framework.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// HomeEnv is where the machines this stands in for are kept, one file each.
const HomeEnv = "FAKE_TART_HOME"

// LogEnv is a file every command is written to, one line each, so a test can read what the backend
// asked for rather than only what came back. Unset writes nothing.
const LogEnv = "FAKE_TART_LOG"

// machine is one virtual machine as this holds it.
type machine struct {
	Name    string
	Running bool
	// Pid is the process holding the machine up, which is what stop signals.
	Pid       int
	Processor int
	Memory    int64
}

// listed is one machine in the shape `tart list --format json` prints.
type listed struct {
	Source   string
	Name     string
	Disk     string
	Size     string
	Accessed string
	Running  bool
	State    string
}

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 {
		return refuse(errors.New("missing command"))
	}
	home := os.Getenv(HomeEnv)
	if home == "" {
		return refuse(fmt.Errorf("%s is not set, so this fake has nowhere to keep its machines", HomeEnv))
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return refuse(err)
	}
	record(args)

	switch args[0] {
	case "clone":
		return report(clone(home, args[1:]))
	case "set":
		return report(set(home, args[1:]))
	case "run":
		return report(start(home, args[1:]))
	case "exec":
		return execute(home, args[1:])
	case "list":
		return report(list(home))
	case "stop":
		return report(stop(home, args[1:]))
	case "delete":
		return report(remove(home, args[1:]))
	default:
		return refuse(fmt.Errorf("unknown command %q", args[0]))
	}
}

// record writes down one command, so a test can read what the backend asked the runtime for.
func record(args []string) {
	at := os.Getenv(LogEnv)
	if at == "" {
		return
	}
	file, err := os.OpenFile(at, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	_, _ = fmt.Fprintln(file, strings.Join(args, " "))
}

func report(err error) int {
	if err != nil {
		return refuse(err)
	}
	return 0
}

func refuse(err error) int {
	fmt.Fprintln(os.Stderr, "Error:", err)
	return 1
}

// The failures, in tart's own words. The backend reads these to tell a machine that is not there
// from one that is there and stopped, so the words are the contract.
func doesNotExist(name string) error {
	return fmt.Errorf("the specified VM %q does not exist", name)
}

func notRunning(name string) error { return fmt.Errorf("VM %q is not running", name) }

func clone(home string, args []string) error {
	positional := withoutFlags(args)
	if len(positional) != 2 {
		return errors.New("clone takes a source and a new name")
	}
	if _, err := read(home, positional[1]); err == nil {
		return errors.New("VM directory is already initialized, preventing overwrite")
	}
	return write(home, machine{Name: positional[1]})
}

func set(home string, args []string) error {
	positional := withoutFlags(args)
	if len(positional) != 1 {
		return errors.New("set takes one name")
	}
	held, err := read(home, positional[0])
	if err != nil {
		return err
	}
	for i := 0; i < len(args)-1; i++ {
		switch args[i] {
		case "--cpu":
			held.Processor = number(args[i+1])
		case "--memory":
			held.Memory = int64(number(args[i+1]))
		}
	}
	return write(home, held)
}

// start holds the machine up until it is signalled, the way `tart run` is the virtual machine's own
// process rather than a command that returns.
func start(home string, args []string) error {
	positional := withoutFlags(args)
	if len(positional) != 1 {
		return errors.New("run takes one name")
	}
	held, err := read(home, positional[0])
	if err != nil {
		return err
	}
	if held.Running {
		return fmt.Errorf("VM %q is already running", held.Name)
	}
	held.Running, held.Pid = true, os.Getpid()
	if err := write(home, held); err != nil {
		return err
	}

	down := make(chan os.Signal, 1)
	signal.Notify(down, syscall.SIGTERM, syscall.SIGINT)
	<-down

	held.Running, held.Pid = false, 0
	// The machine may have been deleted while it was up, which tart allows, and a write would then
	// put it back.
	if _, err := read(home, held.Name); err == nil {
		return write(home, held)
	}
	return nil
}

// execute runs the command on this host and answers with its exit code, standing in for a command
// run inside the guest.
func execute(home string, args []string) int {
	if len(args) < 2 {
		return refuse(errors.New("exec takes a name and a command"))
	}
	held, err := read(home, args[0])
	if err != nil {
		return refuse(err)
	}
	if !held.Running {
		return refuse(notRunning(held.Name))
	}
	cmd := exec.Command(args[1], args[2:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var exited *exec.ExitError
		if errors.As(err, &exited) {
			return exited.ExitCode()
		}
		return refuse(err)
	}
	return 0
}

func list(home string) error {
	entries, err := os.ReadDir(home)
	if err != nil {
		return err
	}
	machines := make([]listed, 0, len(entries))
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name(), ".json")
		if name == entry.Name() {
			continue
		}
		held, err := read(home, name)
		if err != nil {
			continue
		}
		state := "stopped"
		if held.Running {
			state = "running"
		}
		machines = append(machines, listed{
			Source: "local", Name: held.Name, Disk: "50", Size: "1",
			Accessed: time.Time{}.Format(time.RFC3339), Running: held.Running, State: state,
		})
	}
	said, err := json.Marshal(machines)
	if err != nil {
		return err
	}
	fmt.Println(string(said))
	return nil
}

func stop(home string, args []string) error {
	positional := withoutFlags(args)
	if len(positional) != 1 {
		return errors.New("stop takes one name")
	}
	held, err := read(home, positional[0])
	if err != nil {
		return err
	}
	if !held.Running {
		return notRunning(held.Name)
	}
	return takeDown(home, held)
}

func remove(home string, args []string) error {
	for _, name := range withoutFlags(args) {
		held, err := read(home, name)
		if err != nil {
			return err
		}
		// tart removes the directory whether or not the machine is up, so this does too, and the
		// process holding it is signalled rather than left behind.
		if held.Running {
			if err := takeDown(home, held); err != nil {
				return err
			}
		}
		if err := os.Remove(at(home, name)); err != nil {
			return err
		}
	}
	return nil
}

// takeDown signals the process holding the machine up and waits for it to put the machine down.
func takeDown(home string, held machine) error {
	if held.Pid > 0 {
		if err := syscall.Kill(held.Pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
	}
	for range 100 {
		now, err := read(home, held.Name)
		if err != nil || !now.Running {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	// The process is gone without saying so, so the machine is down whatever the file says.
	held.Running, held.Pid = false, 0
	return write(home, held)
}

// number reads a figure tart was given, and answers zero for anything that is not one.
func number(text string) int {
	read, err := strconv.Atoi(text)
	if err != nil {
		return 0
	}
	return read
}

func at(home, name string) string { return filepath.Join(home, name+".json") }

func read(home, name string) (machine, error) {
	body, err := os.ReadFile(at(home, name))
	if err != nil {
		return machine{}, doesNotExist(name)
	}
	var held machine
	if err := json.Unmarshal(body, &held); err != nil {
		return machine{}, err
	}
	return held, nil
}

func write(home string, held machine) error {
	body, err := json.Marshal(held)
	if err != nil {
		return err
	}
	return os.WriteFile(at(home, held.Name), body, 0o644)
}

// withoutFlags is the arguments that are not flags, so a command reads its names whatever options
// were passed with them.
func withoutFlags(args []string) []string {
	var names []string
	for i := 0; i < len(args); i++ {
		if !strings.HasPrefix(args[i], "-") {
			names = append(names, args[i])
			continue
		}
		// An option that takes its value as the next argument, the way tart spells --cpu 4.
		if !strings.Contains(args[i], "=") && takesValue(args[i]) {
			i++
		}
	}
	return names
}

func takesValue(flag string) bool {
	switch flag {
	case "--cpu", "--memory", "--source", "--format", "--dir", "--disk-size":
		return true
	}
	return false
}
