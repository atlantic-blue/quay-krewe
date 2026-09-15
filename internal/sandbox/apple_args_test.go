package sandbox

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/capacity"
)

// What this backend does differently from Docker. The rules the two share are in
// providerargs_test.go, and these are the ones that can only be said in this tool's words.

// TestASandboxUnderAppleIsGivenTheMemoryItWasConfiguredWith, and nothing beside it.
//
// Docker is told the swap ceiling as well, because the daemon otherwise allows swap of the same size
// again and a session may take twice what the operator said. A guest of this tool has no swap, so one
// figure is the whole of what a session may take.
func TestASandboxUnderAppleIsGivenTheMemoryItWasConfiguredWith(t *testing.T) {
	got := AppleProvider{Image: "img", Memory: "4g"}.runArgs("krewe-s1", Config{ID: "s1"}, nil)

	if memory, given := valueAfter(got, "--memory"); !given || memory != "4g" {
		t.Fatalf("the sandbox is not given the memory it was configured with:\n%s", strings.Join(got, " "))
	}
	if slices.Contains(got, "--memory-swap") {
		t.Fatalf("the run caps swap, and this tool's guest has none:\n%s", strings.Join(got, " "))
	}
}

// TestASandboxUnderAppleWithNoMemoryConfiguredIsToldNoFigure. The tool then gives the container 1024
// mebibytes of its own accord, which is a small session rather than an unbounded one, so an operator
// who chooses this backend sets the figure. Inventing one here would hide that from them.
func TestASandboxUnderAppleWithNoMemoryConfiguredIsToldNoFigure(t *testing.T) {
	got := AppleProvider{Image: "img"}.runArgs("krewe-s1", Config{ID: "s1"}, nil)

	if slices.Contains(got, "--memory") {
		t.Fatalf("a sandbox is given a figure nobody configured:\n%s", strings.Join(got, " "))
	}
}

// TestASandboxUnderAppleAsksForNoProcessorAllocation, however much processor the session requested.
//
// The system reserves a request and not a limit: a sandbox alone on an idle machine may take more
// than it asked for. Docker says that with a share, which binds only while the machine is contended.
// This tool has no share. Its --cpus allocates that many processors to the virtual machine and caps
// the sandbox at them, so passing the request there would turn it into the limit the system says it
// is not.
func TestASandboxUnderAppleAsksForNoProcessorAllocation(t *testing.T) {
	cfg := Config{ID: "s1", Request: capacityOfOneProcessor()}
	got := AppleProvider{Image: "img"}.runArgs("krewe-s1", cfg, nil)

	for _, forbidden := range []string{"--cpus", "-c", "--cpu-shares"} {
		if slices.Contains(got, forbidden) {
			t.Fatalf("the run caps the processors a session may use with %s, and a request is not a limit:\n%s",
				forbidden, strings.Join(got, " "))
		}
	}
}

// TestAnAppleExecCarriesTheWorkingDirectoryAndTheEnvironment. The environment is what the model's
// token rides on, and the working directory is where the session's files are, so an exec that lost
// either runs the right command in the wrong place with no credential.
func TestAnAppleExecCarriesTheWorkingDirectoryAndTheEnvironment(t *testing.T) {
	got := appleExecArgs("krewe-s1", Spec{
		Argv:    []string{"claude", "--print"},
		Workdir: WorkingPath,
		Env:     []string{"CLAUDE_CODE_OAUTH_TOKEN=tok"},
	})

	if workdir, given := valueAfter(got, "--workdir"); !given || workdir != WorkingPath {
		t.Fatalf("the exec runs somewhere else:\n%s", strings.Join(got, " "))
	}
	if env, given := valueAfter(got, "--env"); !given || env != "CLAUDE_CODE_OAUTH_TOKEN=tok" {
		t.Fatalf("the exec carries no environment:\n%s", strings.Join(got, " "))
	}
	// Standard input stays open, because an exec is fed its prompt through it.
	if !slices.Contains(got, "--interactive") {
		t.Fatalf("the exec closes standard input, so a prompt cannot reach the model:\n%s", strings.Join(got, " "))
	}
	// The container is named before the command, and everything after it belongs to the command.
	at := slices.Index(got, "krewe-s1")
	if at < 0 || !slices.Equal(got[at+1:], []string{"claude", "--print"}) {
		t.Fatalf("the command does not follow the container:\n%s", strings.Join(got, " "))
	}
}

// TestASandboxNameIsOneAppleContainerAccepts.
//
// This tool refuses a name that is not a hostname label: it must start with a letter or a digit, hold
// only letters, digits, underscore, full stop and hyphen, and be no longer than 63 characters. Docker
// is looser, so a name that grows or gains a character would keep working there and fail here, and it
// would fail at the moment a session starts rather than in a test.
func TestASandboxNameIsOneAppleContainerAccepts(t *testing.T) {
	label := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]+$`)

	for _, name := range ContainerNames("0123456789abcdef01234567") {
		if !label.MatchString(name) {
			t.Errorf("%q is not a name this tool accepts, so a session under it never starts", name)
		}
		if len(name) > 63 {
			t.Errorf("%q is %d characters, and this tool takes 63", name, len(name))
		}
	}
}

// capacityOfOneProcessor is what a sandbox asks the machine for, so the test above states a request
// that a backend could turn into a limit.
func capacityOfOneProcessor() capacity.Request {
	return capacity.Request{Processor: capacity.OneProcessor}
}
