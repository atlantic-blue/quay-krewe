package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/controlplane"
	"github.com/atlantic-blue/quay-krewe/internal/model"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/secrets"
	"github.com/atlantic-blue/quay-krewe/internal/store"
)

// What `krewe step check` puts on the screen. The scenarios in features/path.feature drive the real
// binary against a real system; this holds the shape of the four lines an operator reads, which is
// what the command is for.

// aStepToCheck is a project whose design is approved, holding one step that a session took and
// restated, with the project's proof command set.
//
// The restatement and the word over it are written straight to the store, because they arrive
// through the session's own memory file and this test keeps no directories on disk. What it is about
// is what the check prints, and the gate before it has its own scenarios.
func aStepToCheck(t *testing.T, answers sandbox.Reply) (quaycrewv1.ControlPlaneServiceClient, *sandbox.FakeProvider) {
	t.Helper()
	held := store.NewMemory()
	provider := &sandbox.FakeProvider{Replies: []sandbox.Reply{answers}}
	client := testClientWith(t, controlplane.Config{
		Store: held, Runner: &model.FakeRunner{Reply: "ok"},
		Provider: provider, Secrets: secrets.NewMemory(),
	})
	mustRun(t, client, "workspace", "create", "acme")
	mustRun(t, client, "project", "create", "house-bills")

	design := filepath.Join(t.TempDir(), "design.md")
	if err := os.WriteFile(design, []byte("# Bills\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustRun(t, client, "design", "set", flagFile, design)
	mustRun(t, client, "design", "approve")
	mustRun(t, client, "design", "proof", "go test ./features/... -run '{scenario}'")
	mustRun(t, client, "feature", "add", "the bills")

	path := filepath.Join(t.TempDir(), "path.md")
	document := "## 1. The store holds a project's brief\n\n" +
		"The scenario that proves it\na project carries a brief\n"
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}
	mustRun(t, client, "path", "set", "1", flagFile, path)
	mustRun(t, client, "step", "take", "1.1")

	ctx := context.Background()
	features, err := held.ListFeatures(ctx, projectOf(t, held))
	if err != nil || len(features) != 1 {
		t.Fatalf("the project holds %d features: %v", len(features), err)
	}
	if _, err := held.SetRestatement(ctx, features[0].GetId(), 1, "what I understood"); err != nil {
		t.Fatalf("SetRestatement: %v", err)
	}
	if _, err := held.ApproveRestatement(ctx, features[0].GetId(), 1); err != nil {
		t.Fatalf("ApproveRestatement: %v", err)
	}
	theSandboxIsUp(t, held, provider, features[0].GetId())
	return client, provider
}

// theSandboxIsUp gives the provider the sandbox of the session holding the step.
//
// The take dispatches and lets go, so the container that dispatch makes may not be there yet when
// the check runs, and a check on a session with no container is refused rather than answered. That
// refusal belongs to the slice that makes a container for a reclaimed session; what these tests are
// about is a session whose container is already up.
//
// The provider adopts a sandbox it already holds, so the dispatch still in flight takes this one
// rather than making a second, and the canned answers reach it either way.
func theSandboxIsUp(t *testing.T, held store.Store, provider *sandbox.FakeProvider, feature string) {
	t.Helper()
	ctx := context.Background()
	step, err := held.GetStep(ctx, feature, 1)
	if err != nil {
		t.Fatalf("GetStep: %v", err)
	}
	sessions, err := held.ListSessions(ctx, store.SessionFilter{Project: projectOf(t, held)})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	for _, session := range sessions {
		if session.GetHandle() != step.GetSession() && session.GetId() != step.GetSession() {
			continue
		}
		if _, err := provider.Create(ctx, sandbox.Config{
			ID: session.GetId(), Workspace: session.GetWorkspace(), Project: session.GetProject(),
		}); err != nil {
			t.Fatalf("make the session a sandbox: %v", err)
		}
		return
	}
	t.Fatalf("no session holds step 1, which reads %q", step.GetSession())
}

// projectOf is the one project this system holds, which the tool made by name.
func projectOf(t *testing.T, held store.Store) string {
	t.Helper()
	projects, err := held.ListProjects(context.Background(), "")
	if err != nil || len(projects) != 1 {
		t.Fatalf("the system holds %d projects: %v", len(projects), err)
	}
	return projects[0].GetId()
}

// The command before the verdict, with the step's own scenario name in it. A template is not what
// runs, and an operator reading the template cannot see that their quoting put the name somewhere
// the runner never looks.
func TestCheckPrintsTheCommandThenTheVerdict(t *testing.T) {
	client, _ := aStepToCheck(t, sandbox.Reply{Match: "-run", Out: "1 scenarios (1 passed)"})

	printed := mustRun(t, client, "step", "check", "1.1")

	for _, want := range []string{
		"the check runs:",
		"go test ./features/... -run 'a project carries a brief'",
		"this waits for the run, and starts no model",
		"verdict: passing, 1 scenario ran",
		"1 scenarios (1 passed)",
	} {
		if !strings.Contains(printed, want) {
			t.Errorf("the check printed %q, want it to carry %q", printed, want)
		}
	}
	// The command is printed above the verdict, because the point of printing it is that the operator
	// reads what runs before the wait rather than after it.
	if strings.Index(printed, "the check runs:") > strings.Index(printed, "verdict:") {
		t.Errorf("the verdict is printed above the command: %q", printed)
	}
}

// A failing verdict is printed rather than refused: the run happened and it said something. The exit
// status is what a script reads, and ErrSaid is how this tool says the reason is already on the
// screen.
func TestAFailingCheckPrintsTheOutputAndFails(t *testing.T) {
	client, _ := aStepToCheck(t, sandbox.Reply{
		Match: "-run",
		Out:   "1 scenarios (0 passed, 1 failed)\nthe brief read back empty",
		Err:   errors.New("exit status 1"),
	})

	var out bytes.Buffer
	err := run(context.Background(), client, []string{"step", "check", "1.1"}, &out, "")
	if err == nil {
		t.Fatal("a failing check reported success, so a script cannot tell anything went wrong")
	}
	if !errors.Is(err, ErrSaid) {
		t.Errorf("the failure is %v, and the reason is already on the screen", err)
	}
	printed := out.String()
	if !strings.Contains(printed, "verdict: failing") {
		t.Errorf("the check printed %q, want it to carry the verdict", printed)
	}
	if !strings.Contains(printed, "the brief read back empty") {
		t.Errorf("the check printed %q, want it to carry the end of the run", printed)
	}
}
