package sandboxtest

import (
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
)

// The contract needs a runtime to run against, so these two hold the parts of it that do not.
//
// Both guard the same failure: a suite that runs and proves nothing. It reports success either way,
// and a backend then lands with a green tick over cases that never ran or never matched.

// TestTheContractHoldsABackendToSomething. An empty map is a passing suite.
func TestTheContractHoldsABackendToSomething(t *testing.T) {
	if len(cases()) == 0 {
		t.Fatal("the contract holds a backend to no cases, so every backend keeps it")
	}
}

// TestEverySessionTheContractMakesLooksLikeASession.
//
// A provider finds a stranded sandbox by the exact shape of a session identifier: 24 hexadecimal
// characters behind the prefix. A case that used any other shape would create a container, find it in
// no listing, and pass while proving that the listing is empty.
func TestEverySessionTheContractMakesLooksLikeASession(t *testing.T) {
	for _, last := range []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "b1"} {
		id := sessionID(last)
		if len(id) != sandbox.SessionIDLength {
			t.Errorf("%q is %d characters, and a session identifier is %d", id, len(id), sandbox.SessionIDLength)
		}
		of, isSandbox := sandbox.SessionOf(sandbox.ContainerName(id))
		if !isSandbox {
			t.Errorf("a container named for %q is not read as a sandbox, so no listing would ever hold it", id)
			continue
		}
		if of != id {
			t.Errorf("a container named for %q belongs to session %q", id, of)
		}
		if retired, isSandbox := sandbox.SessionOf(sandbox.RetiredContainerPrefix + id); !isSandbox || retired != id {
			t.Errorf("a container named %q from before the rename is not read as session %q", retired, id)
		}
	}
}

// TestEveryConfigurationTheContractCreatesIsOneAProviderWithStorageAccepts.
//
// State is kept per workspace and per project, so a provider built with storage refuses a
// configuration that carries an identifier and nothing else. A case that named neither would pass
// against a provider with no storage, and fail against the one the control plane builds, and the
// failure would land only where a runtime is.
func TestEveryConfigurationTheContractCreatesIsOneAProviderWithStorageAccepts(t *testing.T) {
	data := t.TempDir()
	storage := sandbox.Storage{Dir: data, Host: data}

	made := []sandbox.Config{
		forSession(sessionID("a1")),
		{ID: sessionID("a3"), Workspace: sharedWorkspace, Project: sharedProject},
	}
	for _, cfg := range made {
		mounts, err := storage.Prepare(cfg)
		if err != nil {
			t.Errorf("a session the contract creates is refused before any container exists: %v", err)
			continue
		}
		if len(mounts) == 0 {
			t.Errorf("%+v is given no directories, so nothing it writes outlives its container", cfg)
		}
	}
}
