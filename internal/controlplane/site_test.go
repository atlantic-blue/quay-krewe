package controlplane_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/controlplane"
	"github.com/atlantic-blue/quay-krewe/internal/model"
	"github.com/atlantic-blue/quay-krewe/internal/store"
)

// The read only http surface, at the level a page reads it: a path, a status, and a document.
//
// What is proved here is the part a scenario cannot reach cheaply: that either identifier reaches a
// project, that a project of another workspace is not answered, and that each refusal names the part
// of the address that named nothing.

// siteStages is the shape stages.json promises. A reader of the page holds this and nothing else, so
// the field names are the contract and not an implementation detail.
type siteStages struct {
	Design struct {
		Brief string `json:"brief"`
		Body  string `json:"body"`
	} `json:"design"`
	Stages []struct {
		Stage           string `json:"stage"`
		Position        int    `json:"position"`
		Body            string `json:"body"`
		Artifact        string `json:"artifact"`
		Version         int    `json:"version"`
		ApprovedVersion int    `json:"approved_version"`
		Skipped         bool   `json:"skipped"`
	} `json:"stages"`
}

// SITE-1. The whole document, read at the address a person types: names rather than identifiers.
func TestTheSiteAnswersTheStagesAProjectHasWritten(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, projectID := newProject(t, s)
	settleSiteStage(t, s, projectID, store.StageDiscovery)
	settleSiteStage(t, s, projectID, store.StageStories)
	if _, err := s.SetBrief(context.Background(), &quaycrewv1.SetBriefRequest{
		Project: projectID, Brief: "four bills, and two of them move",
	}); err != nil {
		t.Fatalf("SetBrief: %v", err)
	}

	status, body, header := readSite(t, s, "/p/acme/house-bills/stages.json")
	if status != http.StatusOK {
		t.Fatalf("the site answered %d saying %q, want 200", status, body)
	}
	if kind := header.Get("Content-Type"); !strings.Contains(kind, "application/json") {
		t.Errorf("the site answered content type %q, and a page reads json", kind)
	}

	var answer siteStages
	if err := json.Unmarshal([]byte(body), &answer); err != nil {
		t.Fatalf("the site answered %q, which is not the stages a page reads: %v", body, err)
	}
	if answer.Design.Brief != "four bills, and two of them move" {
		t.Errorf("the answer carries the brief %q", answer.Design.Brief)
	}
	if len(answer.Stages) != 2 {
		t.Fatalf("the answer carries %d stages, want the two the project wrote: %q", len(answer.Stages), body)
	}
	for at, want := range []string{store.StageDiscovery, store.StageStories} {
		held := answer.Stages[at]
		if held.Stage != want {
			t.Fatalf("stage %d is %q, want %q, so the order is not the order they are written in",
				at, held.Stage, want)
		}
		if held.Position != at {
			t.Errorf("the %s stage sits at position %d, want %d", want, held.Position, at)
		}
		if held.Body != "the "+want+" body" {
			t.Errorf("the %s stage reads %q", want, held.Body)
		}
		if held.Version != 1 || held.ApprovedVersion != 1 {
			t.Errorf("the %s stage is at version %d approved at version %d, want 1 and 1",
				want, held.Version, held.ApprovedVersion)
		}
		if held.Skipped {
			t.Errorf("the %s stage comes back skipped", want)
		}
	}
}

// The identifier reaches the same document as the name, because a listing prints the identifier and
// both are on the operator's screen.
func TestTheSiteAnswersToTheIdentifierAsWellAsTheName(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	workspaceID, projectID := newProject(t, s)
	settleSiteStage(t, s, projectID, store.StageDiscovery)

	status, body, _ := readSite(t, s, "/p/"+workspaceID+"/"+projectID+"/stages.json")
	if status != http.StatusOK {
		t.Fatalf("the site answered %d saying %q, want 200", status, body)
	}
	var answer siteStages
	if err := json.Unmarshal([]byte(body), &answer); err != nil {
		t.Fatalf("the site answered %q: %v", body, err)
	}
	if len(answer.Stages) != 1 {
		t.Fatalf("the answer carries %d stages, want the one the project wrote", len(answer.Stages))
	}
}

// SITE-2. Each part of the address names itself when it is the part that named nothing, because an
// operator told only "not found" has three places to go and look.
func TestAnAddressThatNamesNothingSaysWhichPartNamedNothing(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, projectID := newProject(t, s)
	settleSiteStage(t, s, projectID, store.StageDiscovery)

	for _, one := range []struct {
		why  string
		path string
		said string
	}{
		{"the workspace", "/p/no-such-workspace/house-bills/stages.json", "no-such-workspace"},
		{"the project", "/p/acme/no-such-project/stages.json", "no-such-project"},
		{"a stage nobody wrote", "/p/acme/house-bills/stories/flows.json", store.StageStories},
		{"a stage that is not one of the six", "/p/acme/house-bills/kitchen/flows.json", "kitchen"},
		{"a stage carrying no artifact", "/p/acme/house-bills/discovery/flows.json", store.StageDiscovery},
	} {
		status, body, _ := readSite(t, s, one.path)
		if status != http.StatusNotFound {
			t.Errorf("%s answered %d saying %q, want 404", one.why, status, body)
		}
		if !strings.Contains(body, one.said) {
			t.Errorf("%s answered %q, and it never named %q", one.why, body, one.said)
		}
		if lines := strings.Count(strings.TrimSpace(body), "\n"); lines != 0 {
			t.Errorf("%s answered %d lines, and a refusal is one sentence: %q", one.why, lines+1, body)
		}
	}
}

// A project of another workspace is a refusal rather than a document. Answering it would put one
// workspace's design at every other workspace's address.
func TestTheSiteRefusesAProjectOfAnotherWorkspace(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, projectID := newProject(t, s)
	settleSiteStage(t, s, projectID, store.StageDiscovery)
	other, err := s.CreateWorkspace(context.Background(), &quaycrewv1.CreateWorkspaceRequest{Name: "other"})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	status, body, _ := readSite(t, s, "/p/"+other.GetWorkspace().GetName()+"/house-bills/stages.json")
	if status != http.StatusNotFound {
		t.Fatalf("reading another workspace's project answered %d saying %q, want 404", status, body)
	}
	if !strings.Contains(body, "house-bills") {
		t.Errorf("the refusal reads %q, and it has to name the project it looked for", body)
	}
}

// SITE-2, the other half. The surface has no write on it, so a method that writes is refused before
// anything is read, and the refusal says which methods the address answers.
func TestTheSiteRefusesAMethodThatWrites(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, projectID := newProject(t, s)
	settleSiteStage(t, s, projectID, store.StageDiscovery)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		recorder := httptest.NewRecorder()
		s.Site().ServeHTTP(recorder, httptest.NewRequest(method, "/p/acme/house-bills/stages.json", nil))
		if recorder.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s answered %d, want 405", method, recorder.Code)
		}
		if allowed := recorder.Header().Get("Allow"); !strings.Contains(allowed, "GET") ||
			!strings.Contains(allowed, "HEAD") {
			t.Errorf("%s was allowed %q, and a caller cannot tell what the address answers", method, allowed)
		}
	}
}

// A page asks for the head of a document to see whether it is there, and the surface is read only, so
// the same route answers it.
func TestTheSiteAnswersHead(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, projectID := newProject(t, s)
	settleSiteStage(t, s, projectID, store.StageDiscovery)

	recorder := httptest.NewRecorder()
	s.Site().ServeHTTP(recorder, httptest.NewRequest(http.MethodHead, "/p/acme/house-bills/stages.json", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("a head request answered %d, want 200", recorder.Code)
	}
}

// SITE-1, the second document. The artifact is json in the column, and it comes back as it is, so the
// flow map reads the bytes the session wrote.
func TestTheSiteAnswersAStagesArtifact(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, projectID := newProject(t, s)
	settleSiteStage(t, s, projectID, store.StageDiscovery)
	artifact := `{"screens":[{"id":"bills","kind":"web"}]}`
	if _, err := s.SetDesignStage(context.Background(), &quaycrewv1.SetDesignStageRequest{
		Project: projectID, Stage: store.StageStories, Body: "the stories body", Artifact: artifact,
	}); err != nil {
		t.Fatalf("SetDesignStage: %v", err)
	}

	status, body, header := readSite(t, s, "/p/acme/house-bills/stories/flows.json")
	if status != http.StatusOK {
		t.Fatalf("the site answered %d saying %q, want 200", status, body)
	}
	if kind := header.Get("Content-Type"); !strings.Contains(kind, "application/json") {
		t.Errorf("the artifact came back as content type %q", kind)
	}
	if strings.TrimSpace(body) != artifact {
		t.Errorf("the site answered %q, want the artifact as it was written: %q", body, artifact)
	}
}

// SITE-3. The default binds the machine the control plane runs on. A port is not a boundary here: the
// address is the whole system, and nothing on this surface asks a caller who they are.
func TestTheSiteBindsLoopbackByDefault(t *testing.T) {
	if controlplane.SiteAddr != "127.0.0.1:50052" {
		t.Fatalf("the site binds %q by default, want 127.0.0.1:50052", controlplane.SiteAddr)
	}
}

// read makes one request against the site and hands back what a page would have.
func readSite(t *testing.T, s *controlplane.Server, path string) (int, string, http.Header) {
	t.Helper()
	recorder := httptest.NewRecorder()
	s.Site().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder.Code, recorder.Body.String(), recorder.Result().Header
}

// settle writes a stage and approves it, which is the only way past the rule that orders the six.
func settleSiteStage(t *testing.T, s *controlplane.Server, project, stage string) {
	t.Helper()
	if _, err := s.SetDesignStage(context.Background(), &quaycrewv1.SetDesignStageRequest{
		Project: project, Stage: stage, Body: "the " + stage + " body",
	}); err != nil {
		t.Fatalf("SetDesignStage %s: %v", stage, err)
	}
	if _, err := s.ApproveDesignStage(context.Background(), &quaycrewv1.ApproveDesignStageRequest{
		Project: project, Stage: stage,
	}); err != nil {
		t.Fatalf("ApproveDesignStage %s: %v", stage, err)
	}
}

// A name two workspaces share names neither of them. The tree of names has the same problem and
// answers it by order, which would send the operator to one workspace this morning and the other one
// this afternoon, so the address is refused and the refusal offers both identifiers.
func TestTheSiteRefusesANameTwoWorkspacesShare(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, projectID := newProject(t, s)
	settleSiteStage(t, s, projectID, store.StageDiscovery)
	twin, err := s.CreateWorkspace(context.Background(), &quaycrewv1.CreateWorkspaceRequest{Name: "acme"})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	status, body, _ := readSite(t, s, "/p/acme/house-bills/stages.json")
	if status != http.StatusNotFound {
		t.Fatalf("a shared name answered %d saying %q, want 404", status, body)
	}
	if !strings.Contains(body, twin.GetWorkspace().GetId()) {
		t.Errorf("the refusal reads %q, and it has to offer the identifiers to pick between", body)
	}
}

// A stage written again. The version moves and the word is gone, because the operator agreed to a
// text that has just changed. Both numbers are on the row for that reason, and a document that
// answered one of them twice would draw every written stage as an approved one.
func TestTheSiteAnswersAStageWrittenAgainWithoutTheWordItHad(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, projectID := newProject(t, s)
	settleSiteStage(t, s, projectID, store.StageDiscovery)
	if _, err := s.SetDesignStage(context.Background(), &quaycrewv1.SetDesignStageRequest{
		Project: projectID, Stage: store.StageDiscovery, Body: "four bills, and three of them move",
	}); err != nil {
		t.Fatalf("SetDesignStage: %v", err)
	}

	status, body, _ := readSite(t, s, "/p/acme/house-bills/stages.json")
	if status != http.StatusOK {
		t.Fatalf("the site answered %d saying %q, want 200", status, body)
	}
	var answer siteStages
	if err := json.Unmarshal([]byte(body), &answer); err != nil {
		t.Fatalf("the site answered %q: %v", body, err)
	}
	if len(answer.Stages) != 1 {
		t.Fatalf("the answer carries %d stages, want the one the project wrote", len(answer.Stages))
	}
	held := answer.Stages[0]
	if held.Version != 2 {
		t.Errorf("the discovery stage is at version %d, want 2", held.Version)
	}
	if held.ApprovedVersion != 0 {
		t.Errorf("the discovery stage is approved at version %d, and the write took that word away",
			held.ApprovedVersion)
	}
	if held.Body != "four bills, and three of them move" {
		t.Errorf("the discovery stage reads %q, want what was written second", held.Body)
	}
}
