package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
func aStepToCheck(t *testing.T, answers sandbox.Reply) (
	quaycrewv1.ControlPlaneServiceClient, *sandbox.FakeProvider, *controlplane.Server) {
	t.Helper()
	held := store.NewMemory()
	provider := &sandbox.FakeProvider{Replies: []sandbox.Reply{answers}}
	server := controlplane.NewServer(controlplane.Config{
		Store: held, Runner: &model.FakeRunner{Reply: "ok"},
		Provider: provider, Secrets: secrets.NewMemory(),
	})
	client := testClientFor(t, server)
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
	theTakeHasLanded(t, server)
	return client, provider, server
}

// theTakeHasLanded waits for the exec the take started, so the session holding the step has the
// container that exec made before anything checks it.
//
// The take dispatches and lets go, so without this the check races the container: sometimes it finds
// one and sometimes it starts one, and a test reading what the check printed would read a different
// screen each run.
func theTakeHasLanded(t *testing.T, server *controlplane.Server) {
	t.Helper()
	waiting, giveUp := context.WithTimeout(context.Background(), 10*time.Second)
	defer giveUp()
	server.WaitForExecs(waiting)
	if waiting.Err() != nil {
		t.Fatal("the exec the take started never landed")
	}
}

// theSessionHoldingTheStep is the session the take gave step one to, read the way a caller reads it.
func theSessionHoldingTheStep(t *testing.T, client quaycrewv1.ControlPlaneServiceClient) *quaycrewv1.Session {
	t.Helper()
	ctx := context.Background()
	projects, err := client.ListProjects(ctx, &quaycrewv1.ListProjectsRequest{})
	if err != nil || len(projects.GetProjects()) != 1 {
		t.Fatalf("the system holds %d projects: %v", len(projects.GetProjects()), err)
	}
	project := projects.GetProjects()[0].GetId()
	features, err := client.ListFeatures(ctx, &quaycrewv1.ListFeaturesRequest{Project: project})
	if err != nil || len(features.GetFeatures()) != 1 {
		t.Fatalf("the project holds %d features: %v", len(features.GetFeatures()), err)
	}
	read, err := client.GetStep(ctx, &quaycrewv1.GetStepRequest{
		Feature: features.GetFeatures()[0].GetId(), Number: 1})
	if err != nil {
		t.Fatalf("GetStep: %v", err)
	}
	sessions, err := client.ListSessions(ctx, &quaycrewv1.ListSessionsRequest{Project: project})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	held := read.GetStep().GetSession()
	for _, session := range sessions.GetSessions() {
		if session.GetHandle() == held || session.GetId() == held {
			return session
		}
	}
	t.Fatalf("no session holds step 1, which reads %q", held)
	return nil
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
	client, _, _ := aStepToCheck(t, sandbox.Reply{Match: "-run", Out: "1 scenarios (1 passed)"})

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
	client, _, _ := aStepToCheck(t, sandbox.Reply{
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

// The way off the refusal this slice removes. A check on a session with no container used to be
// refused, which left a step nobody could move: the session is gone, so the step cannot be checked,
// so it cannot be closed. Now krewe starts the container, and the operator reads that it is doing so
// before the wait rather than after it.
func TestACheckOnAReclaimedSessionStartsAContainerAndSaysSoFirst(t *testing.T) {
	client, provider, _ := aStepToCheck(t, sandbox.Reply{Match: "-run", Out: "1 scenarios (1 passed)"})
	session := theSessionHoldingTheStep(t, client)
	ctx := context.Background()
	if _, err := client.ReclaimSession(ctx, &quaycrewv1.ReclaimSessionRequest{Id: session.GetId()}); err != nil {
		t.Fatalf("reclaiming the session holding the step: %v", err)
	}
	// Read back, because a scenario about a session with no container is worth nothing while it has
	// one.
	if _, running, err := provider.Existing(ctx, session.GetId()); err != nil || running {
		t.Fatalf("the session still has a container after the reclaim: running=%v, %v", running, err)
	}
	made := containersMadeFor(provider, session.GetId())

	printed := mustRun(t, client, "step", "check", "1.1")

	for _, want := range []string{
		"krewe starts a container",
		"verdict: passing, 1 scenario ran",
		"krewe started a container",
	} {
		if !strings.Contains(printed, want) {
			t.Errorf("the check printed %q, want it to carry %q", printed, want)
		}
	}
	// Before the wait, because the whole reason the line exists is that the operator is about to wait
	// longer than a check takes and should read why while it is happening.
	if strings.Index(printed, "krewe starts a container") > strings.Index(printed, "verdict:") {
		t.Errorf("the verdict is printed above the container line: %q", printed)
	}
	if got := containersMadeFor(provider, session.GetId()) - made; got != 1 {
		t.Errorf("krewe made %d containers for the session, want 1", got)
	}
}

// containersMadeFor is how many containers this provider actually made for one session. A container
// it adopted is not one it made, which is what lets a test say a check made exactly one.
func containersMadeFor(provider *sandbox.FakeProvider, session string) int {
	made := 0
	for _, cfg := range provider.Configurations() {
		if cfg.ID == session {
			made++
		}
	}
	return made
}
