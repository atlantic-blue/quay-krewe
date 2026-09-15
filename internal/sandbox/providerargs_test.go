package sandbox

import (
	"slices"
	"strings"
	"testing"
)

// The rules that hold whatever a session is isolated in.
//
// Two backends start a container for a session, and they do it with two different tools. What the
// session gets must not depend on which one an operator picked: the network it can address, the place
// a mounted secret lands, the environment it carries and the directories it was given are the
// sandbox, not the tool. So each rule is written once here and asked of every backend.
//
// The flags differ and that is expected, so these read the values rather than the spellings. A rule
// that can only be said in one tool's words belongs in that backend's own test.
func runUnderEveryBackend(opts Options) map[string]func(string, Config, []Mount) []string {
	return map[string]func(string, Config, []Mount) []string{
		KindDocker: DockerProvider(opts).runArgs,
		KindApple:  AppleProvider(opts).runArgs,
	}
}

// valueAfter is what the tool was given for a flag, and false where the flag is not there at all.
func valueAfter(args []string, flag string) (string, bool) {
	at := slices.Index(args, flag)
	if at < 0 || at+1 >= len(args) {
		return "", false
	}
	return args[at+1], true
}

// TestEveryBackendPutsASessionOnTheNetworkThatReachesTheSystem.
//
// A session running a job is handed the system's address and a credential minted for that job. Both
// are worthless in a container with no route to the address, and that is true of every runtime.
func TestEveryBackendPutsASessionOnTheNetworkThatReachesTheSystem(t *testing.T) {
	for kind, run := range runUnderEveryBackend(Options{Image: "img", SessionNetwork: "quaycrew_sessions"}) {
		t.Run(kind, func(t *testing.T) {
			got := run("krewe-s2", Config{ID: "s2"}, nil)
			if network, joined := valueAfter(got, "--network"); !joined || network != "quaycrew_sessions" {
				t.Fatalf("a session joins no network that reaches the system:\n%s", strings.Join(got, " "))
			}
		})
	}
}

// TestNoBackendPutsASessionOnTheSystemsOwnNetwork. That network carries the store, the broker and the
// dashboards, and a session runs model output.
func TestNoBackendPutsASessionOnTheSystemsOwnNetwork(t *testing.T) {
	opts := Options{
		Image: "img", Network: "quaycrew_default", SessionNetwork: "quaycrew_sessions",
		DriverMounts: []string{"/hub:/hub:ro"},
	}
	for kind, run := range runUnderEveryBackend(opts) {
		t.Run(kind, func(t *testing.T) {
			got := strings.Join(run("krewe-s2", Config{ID: "s2"}, nil), " ")
			if strings.Contains(got, "quaycrew_default") {
				t.Fatalf("a session joins the system's own network, where the store and the broker are:\n%s", got)
			}
			if strings.Contains(got, "/hub") {
				t.Fatalf("a session gets the driver's host paths:\n%s", got)
			}
		})
	}
}

// TestEveryBackendPutsTheDriverOnTheSystemsOwnNetworkWhenTheOperatorNamedOne. The driver is the
// deliberate widening, and it is the operator who asks for it by naming the network.
func TestEveryBackendPutsTheDriverOnTheSystemsOwnNetworkWhenTheOperatorNamedOne(t *testing.T) {
	opts := Options{
		Image: "img", Network: "quaycrew_default", SessionNetwork: "quaycrew_sessions",
		DriverMounts: []string{"/hub:/hub:ro"},
	}
	for kind, run := range runUnderEveryBackend(opts) {
		t.Run(kind, func(t *testing.T) {
			got := run("krewe-s1", Config{ID: "s1", Driver: true}, nil)
			if network, joined := valueAfter(got, "--network"); !joined || network != "quaycrew_default" {
				t.Fatalf("the driver does not join the network the operator named:\n%s", strings.Join(got, " "))
			}
			if !slices.Contains(got, "/hub:/hub:ro") {
				t.Fatalf("the driver does not get the host paths it was given:\n%s", strings.Join(got, " "))
			}
		})
	}
}

// TestUnderEveryBackendTheDriverFallsBackToTheSessionNetwork, so a system that named no wider network
// still has a driver that can drive it. Reaching the control plane is the whole of what driving needs.
func TestUnderEveryBackendTheDriverFallsBackToTheSessionNetwork(t *testing.T) {
	for kind, run := range runUnderEveryBackend(Options{Image: "img", SessionNetwork: "quaycrew_sessions"}) {
		t.Run(kind, func(t *testing.T) {
			got := run("krewe-s1", Config{ID: "s1", Driver: true}, nil)
			if network, joined := valueAfter(got, "--network"); !joined || network != "quaycrew_sessions" {
				t.Fatalf("the driver joins nothing, so it cannot reach the system it exists to drive:\n%s",
					strings.Join(got, " "))
			}
		})
	}
}

// TestUnderEveryBackendASandboxJoinsExactlyOneNetwork. A sandbox keeps what it was created with and
// there is no promotion, so this decision is made once and two network flags would be one of them
// silently ignored.
func TestUnderEveryBackendASandboxJoinsExactlyOneNetwork(t *testing.T) {
	opts := Options{Image: "img", Network: "quaycrew_default", SessionNetwork: "quaycrew_sessions"}
	for kind, run := range runUnderEveryBackend(opts) {
		t.Run(kind, func(t *testing.T) {
			for _, cfg := range []Config{{ID: "s1", Driver: true}, {ID: "s2"}} {
				got := run("krewe-"+cfg.ID, cfg, nil)
				joined := 0
				for _, arg := range got {
					if arg == "--network" {
						joined++
					}
				}
				if joined != 1 {
					t.Fatalf("the sandbox for %+v joins %d networks, want exactly 1:\n%s",
						cfg, joined, strings.Join(got, " "))
				}
			}
		})
	}
}

// TestUnderEveryBackendASandboxJoinsNothingWithNoNetworkConfigured: a system run outside the compose
// stack may have no network to put a session on, and it says so by joining none rather than by
// inventing a name.
func TestUnderEveryBackendASandboxJoinsNothingWithNoNetworkConfigured(t *testing.T) {
	for kind, run := range runUnderEveryBackend(Options{Image: "img"}) {
		t.Run(kind, func(t *testing.T) {
			for _, cfg := range []Config{{ID: "s1", Driver: true}, {ID: "s2"}} {
				got := strings.Join(run("krewe-"+cfg.ID, cfg, nil), " ")
				if strings.Contains(got, "--network") {
					t.Fatalf("the sandbox for %+v joins a network with none configured:\n%s", cfg, got)
				}
			}
		})
	}
}

// TestEveryBackendGivesASandboxSomewhereToPutAMountedSecret, whether or not its workspace mounted one.
// A mount is a create time decision, so the alternative is that the first workspace to mount a secret
// needs a fresh container before it can have the directory at all.
//
// It names the sandbox user in both. Without that the directory belongs to root, and the system writes
// into it as the sandbox's own user, so every write is refused.
func TestEveryBackendGivesASandboxSomewhereToPutAMountedSecret(t *testing.T) {
	for kind, run := range runUnderEveryBackend(Options{Image: "img"}) {
		t.Run(kind, func(t *testing.T) {
			got := run("krewe-s1", Config{ID: "s1"}, nil)
			at, given := valueAfter(got, "--tmpfs")
			if !given || at != "/run/secrets:mode=0700,uid=1001,gid=1001" {
				t.Fatalf("the sandbox has nowhere to put a mounted secret:\n%s", strings.Join(got, " "))
			}
		})
	}
}

// TestEveryBackendCarriesTheEnvironmentAndTheDirectories, so the network did not displace anything and
// a read only mount is still read only.
func TestEveryBackendCarriesTheEnvironmentAndTheDirectories(t *testing.T) {
	opts := Options{Image: "img", Network: "net", Mounts: []string{"/a:/b:ro"}}
	cfg := Config{ID: "s1", Driver: true, Env: []string{"QC_GRPC_ADDR=controlplane:50051"}}
	kept := []Mount{
		{Source: "/host", Target: ConversationPath},
		{Source: "/skills", Target: SkillsPath, ReadOnly: true},
	}
	for kind, run := range runUnderEveryBackend(opts) {
		t.Run(kind, func(t *testing.T) {
			got := run("krewe-s1", cfg, kept)
			for _, want := range []string{
				"QC_GRPC_ADDR=controlplane:50051",
				"/host:" + ConversationPath,
				"/skills:" + SkillsPath + ":ro",
				"/a:/b:ro",
			} {
				if !slices.Contains(got, want) {
					t.Fatalf("the run is missing %q:\n%s", want, strings.Join(got, " "))
				}
			}
			// The image and what it runs are the last words, so a flag added after them would reach
			// the command rather than the tool.
			if tail := got[len(got)-3:]; tail[0] != "img" || tail[1] != "sleep" || tail[2] != "infinity" {
				t.Fatalf("the run ends %q, want the image and the command it runs", tail)
			}
		})
	}
}
