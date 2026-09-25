package controlplane_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/controlplane"
	"github.com/atlantic-blue/quay-krewe/internal/model"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/secrets"
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

// A stage written again. The version moves, so the word no longer stands, and the version it was
// given to stays. Both numbers travel for that reason: a document that answered one of them twice
// would draw every written stage as an approved one, and one that dropped the second could not tell
// a stage nobody agreed to from a stage that changed after the word was given.
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
	if held.ApprovedVersion != 1 {
		t.Errorf("the discovery stage is approved at version %d, want the version the word was given to",
			held.ApprovedVersion)
	}
	if held.Body != "four bills, and three of them move" {
		t.Errorf("the discovery stage reads %q, want what was written second", held.Body)
	}
}

// SITE-7. The library that draws a diagram is served by the site, and nothing the site hands over
// reaches off the machine.
//
// A design is read on a machine that may have no network at all, and a private design must not
// announce itself to anybody while it is read. So the page loads its drawing library from the site
// and from nowhere else, and every file the site serves is checked for an address that leaves the
// machine.

// pageLoads is every file the page loads: its stylesheet, its script and its drawing library.
var pageLoads = regexp.MustCompile(`(?:src|href)="([^"]+)"`)

// libraryName is the address the page loads the drawing library from. The version is in the file
// name, so a reader of the page sees which library is pinned without reading any Go.
var libraryName = regexp.MustCompile(`^/assets/mermaid-\d+\.\d+\.\d+\.min\.js$`)

// theDiagramLibrary is the address the page names for its drawing library, read off the page the site
// serves rather than written down here, so the page and the file it loads cannot drift apart.
func theDiagramLibrary(t *testing.T, s *controlplane.Server) string {
	t.Helper()
	status, page, _ := readSite(t, s, "/p/acme/house-bills/")
	if status != http.StatusOK {
		t.Fatalf("the page answered %d saying %q, want 200", status, page)
	}
	for _, found := range pageLoads.FindAllStringSubmatch(page, -1) {
		if strings.Contains(found[1], "mermaid") {
			return found[1]
		}
	}
	t.Fatalf("the page loads no drawing library, so a diagram stays as text: %s", page)
	return ""
}

// The page names the library, at an address on this site, with the version in the file name.
func TestThePageLoadsTheDiagramLibraryFromTheSite(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, projectID := newProject(t, s)
	settleSiteStage(t, s, projectID, store.StageDiscovery)

	held := theDiagramLibrary(t, s)
	if !libraryName.MatchString(held) {
		t.Fatalf("the page loads its drawing library from %q, want an address on this site with the "+
			"version in the file name", held)
	}
}

// The address the page names answers the library itself: a script, and the library that draws.
func TestTheSiteServesTheDiagramLibrary(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, projectID := newProject(t, s)
	settleSiteStage(t, s, projectID, store.StageDiscovery)

	status, body, header := readSite(t, s, theDiagramLibrary(t, s))
	if status != http.StatusOK {
		t.Fatalf("the drawing library answered %d, want 200", status)
	}
	if kind := header.Get("Content-Type"); !strings.Contains(kind, "javascript") {
		t.Errorf("the drawing library answered content type %q, and a browser runs a script", kind)
	}
	if !strings.Contains(body, `globalThis["mermaid"]`) {
		t.Errorf("the file at that address is %d bytes and never names mermaid, so it is not the library",
			len(body))
	}
}

// Nothing the operator is handed reaches off the machine. The page is read, every file it loads is
// read, and each one is held to it.
//
// The library is read for its status and skipped for its text. It carries the addresses of the
// projects it was built from in its own source, and an address written in a comment is not a call.
func TestNoFileTheSiteServesReachesOffTheMachine(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, projectID := newProject(t, s)
	settleSiteStage(t, s, projectID, store.StageDiscovery)
	library := theDiagramLibrary(t, s)

	_, page, _ := readSite(t, s, "/p/acme/house-bills/")
	read := []struct{ what, body string }{{"the page", page}}
	loaded := pageLoads.FindAllStringSubmatch(page, -1)
	if len(loaded) == 0 {
		t.Fatalf("the page loads no file at all: %s", page)
	}
	for _, found := range loaded {
		address := found[1]
		if !strings.HasPrefix(address, "/") {
			t.Fatalf("the page loads %q, which is not an address on this site", address)
		}
		status, body, _ := readSite(t, s, address)
		if status != http.StatusOK {
			t.Fatalf("the site answered %d for %s, which the page loads", status, address)
		}
		if address == library {
			continue
		}
		read = append(read, struct{ what, body string }{address, body})
	}

	offTheMachine := regexp.MustCompile(`https?://`)
	for _, one := range read {
		if found := offTheMachine.FindString(one.body); found != "" {
			t.Errorf("%s reaches %q, so opening a project would leave the machine", one.what, found)
		}
	}
}

// siteDesignSystem is the shape design-system.json promises. The page that draws the screens holds
// this and nothing else, so the field names are the contract.
type siteDesignSystem struct {
	Approved bool            `json:"approved"`
	Version  int             `json:"version"`
	Tokens   json.RawMessage `json:"tokens"`
	Assets   json.RawMessage `json:"assets"`
	CSS      string          `json:"css"`
}

// SYSTEM-2. The design system at one address, so the page that draws the screens reads the tokens,
// the font files and the stylesheet the project's screens are drawn in. The word the operator gave
// travels beside them, because a design system written again since then no longer carries it.
func TestTheSiteAnswersTheDesignSystemAProjectApproved(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, projectID := newProject(t, s)
	designedUpToTheDesignSystem(t, s, projectID)
	written := aDesignSystem(systemTokens, systemAssets, systemCSS)
	if _, err := writeDesignSystemStage(s, projectID, written); err != nil {
		t.Fatalf("writing the design system: %v", err)
	}
	if _, err := s.ApproveDesignStage(context.Background(), &quaycrewv1.ApproveDesignStageRequest{
		Project: projectID, Stage: store.StageDesignSystem,
	}); err != nil {
		t.Fatalf("ApproveDesignStage: %v", err)
	}

	status, body, header := readSite(t, s, "/p/acme/house-bills/design-system.json")
	if status != http.StatusOK {
		t.Fatalf("the site answered %d saying %q, want 200", status, body)
	}
	if kind := header.Get("Content-Type"); !strings.Contains(kind, "application/json") {
		t.Errorf("the design system came back as content type %q, and a page reads json", kind)
	}

	answer := readDesignSystem(t, body)
	if !answer.Approved {
		t.Errorf("the site says the design system is not approved, and the operator approved it")
	}
	if answer.Version != 1 {
		t.Errorf("the site answers version %d, want the version of the row, 1", answer.Version)
	}
	wrote := map[string]any{}
	if err := json.Unmarshal([]byte(written), &wrote); err != nil {
		t.Fatalf("the design system this test writes is not json: %v", err)
	}
	for name, held := range map[string]json.RawMessage{"tokens": answer.Tokens, "assets": answer.Assets} {
		if !sameJSON(t, held, wrote[name]) {
			t.Errorf("the site answers %s of %s, and the session wrote something else", name, string(held))
		}
	}
	if answer.CSS != wrote["css"] {
		t.Errorf("the site answers the stylesheet %q, and the session wrote %q", answer.CSS, wrote["css"])
	}

	// Written again, and the word no longer stands. A page draws a design system nobody agreed to
	// differently, so the field moves with the row.
	if _, err := writeDesignSystemStage(s, projectID, written); err != nil {
		t.Fatalf("writing the design system again: %v", err)
	}
	_, body, _ = readSite(t, s, "/p/acme/house-bills/design-system.json")
	answer = readDesignSystem(t, body)
	if answer.Approved {
		t.Errorf("the site still says the design system is approved, and it was written again since")
	}
	if answer.Version != 2 {
		t.Errorf("the site answers version %d, want 2", answer.Version)
	}
}

// SYSTEM-2. A design system that names no file and no stylesheet is still a design system: the
// screens are drawn in the tokens and in the font of the machine. The page draws that state rather
// than reading a refusal.
func TestTheSiteAnswersADesignSystemOfTokensAlone(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, projectID := newProject(t, s)
	designedUpToTheDesignSystem(t, s, projectID)
	if _, err := writeDesignSystemStage(s, projectID, aDesignSystem(systemTokens)); err != nil {
		t.Fatalf("writing the design system: %v", err)
	}

	status, body, _ := readSite(t, s, "/p/acme/house-bills/design-system.json")
	if status != http.StatusOK {
		t.Fatalf("the site answered %d saying %q, want 200", status, body)
	}
	answer := readDesignSystem(t, body)
	if answer.Approved {
		t.Errorf("the site says a design system nobody approved is approved")
	}
	if string(answer.Assets) != "{}" {
		t.Errorf("the site answers assets of %s, want an empty object, so the page reads a list of none",
			string(answer.Assets))
	}
	if answer.CSS != "" {
		t.Errorf("the site answers the stylesheet %q, and the session wrote none", answer.CSS)
	}
}

// SYSTEM-2. The three states that are not a design system. Each one is a 404 with one sentence
// naming the project and the stage, so the page says there is no design system yet rather than
// looking broken.
func TestTheSiteSaysWhenAProjectHasNoDesignSystemToAnswer(t *testing.T) {
	// A design system written before the stage had a shape carries whatever json it carries, so the
	// unreadable one goes in through the store, which is where that project holds it.
	held := store.NewMemory()
	s := controlplane.NewServer(controlplane.Config{
		Store: held, Runner: &model.FakeRunner{},
		Provider: &sandbox.FakeProvider{}, Secrets: secrets.NewMemory(),
	})
	_, projectID := newProject(t, s)
	designedUpToTheDesignSystem(t, s, projectID)

	status, body, _ := readSite(t, s, "/p/acme/house-bills/design-system.json")
	saysTheStageIsMissing(t, "a project that wrote no design system", status, body)

	if _, err := s.SetDesignStage(context.Background(), &quaycrewv1.SetDesignStageRequest{
		Project: projectID, Stage: store.StageDesignSystem, Body: "the design system body",
	}); err != nil {
		t.Fatalf("SetDesignStage: %v", err)
	}
	status, body, _ = readSite(t, s, "/p/acme/house-bills/design-system.json")
	saysTheStageIsMissing(t, "a design system written as prose alone", status, body)

	if _, err := held.SetDesignStage(context.Background(), projectID, store.DesignStageWrite{
		Stage: store.StageDesignSystem, Body: "the design system body",
		Artifact: `["a design system written before the stage had a shape"]`,
	}); err != nil {
		t.Fatalf("writing an older design system: %v", err)
	}
	status, body, _ = readSite(t, s, "/p/acme/house-bills/design-system.json")
	saysTheStageIsMissing(t, "a design system a page cannot read", status, body)
}

// saysTheStageIsMissing holds one refusal to the sentence stageArtifact writes: a 404 naming the
// project and the stage, and never a 500, because the page tells the two apart by the status.
func saysTheStageIsMissing(t *testing.T, state string, status int, body string) {
	t.Helper()
	if status != http.StatusNotFound {
		t.Fatalf("%s answered %d saying %q, want 404", state, status, body)
	}
	for _, want := range []string{"house-bills", store.StageDesignSystem} {
		if !strings.Contains(body, want) {
			t.Errorf("%s answered %q, and the sentence never names %q", state, body, want)
		}
	}
}

// readDesignSystem is the document a page holds, and a failure to read it is the contract broken.
func readDesignSystem(t *testing.T, body string) siteDesignSystem {
	t.Helper()
	var answer siteDesignSystem
	if err := json.Unmarshal([]byte(body), &answer); err != nil {
		t.Fatalf("the site answered %q, which is not a design system a page reads: %v", body, err)
	}
	return answer
}

// sameJSON compares what the site answered against what the session wrote, as documents rather than
// as text, because the two are the same document whatever order their names come back in.
func sameJSON(t *testing.T, answered json.RawMessage, written any) bool {
	t.Helper()
	var read any
	if err := json.Unmarshal(answered, &read); err != nil {
		t.Fatalf("the site answered %s, which a page cannot read: %v", string(answered), err)
	}
	return reflect.DeepEqual(read, written)
}
