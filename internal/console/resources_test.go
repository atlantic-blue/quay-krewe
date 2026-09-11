package console

import (
	"context"
	"fmt"
	"strings"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/grpc"
)

// pathClient answers the two calls the path view makes and nothing else. It embeds the generated
// interface, so a call this view grows and this double does not answer panics loudly rather than
// being quietly satisfied.
//
// It refuses a feature it does not hold, the way the control plane does, so a view that asked for a
// project's steps by naming the project cannot pass here.
type pathClient struct {
	quaycrewv1.ControlPlaneServiceClient

	features []*quaycrewv1.Feature
	steps    map[string][]*quaycrewv1.Step
	listErr  error
	// asked is the feature each ListSteps call named, in order, so a test can say the view read the
	// paths it drew.
	asked []string
}

func (p *pathClient) ListFeatures(_ context.Context, req *quaycrewv1.ListFeaturesRequest,
	_ ...grpc.CallOption) (*quaycrewv1.ListFeaturesResponse, error) {
	if p.listErr != nil {
		return nil, p.listErr
	}
	if req.GetProject() == "" {
		return &quaycrewv1.ListFeaturesResponse{Features: p.features}, nil
	}
	matched := make([]*quaycrewv1.Feature, 0, len(p.features))
	for _, feature := range p.features {
		if feature.GetProject() == req.GetProject() {
			matched = append(matched, feature)
		}
	}
	return &quaycrewv1.ListFeaturesResponse{Features: matched}, nil
}

func (p *pathClient) ListSteps(_ context.Context, req *quaycrewv1.ListStepsRequest,
	_ ...grpc.CallOption) (*quaycrewv1.ListStepsResponse, error) {
	if p.listErr != nil {
		return nil, p.listErr
	}
	p.asked = append(p.asked, req.GetFeature())
	held, known := p.steps[req.GetFeature()]
	if !known {
		return nil, fmt.Errorf("no feature %q", req.GetFeature())
	}
	return &quaycrewv1.ListStepsResponse{Steps: held}, nil
}

// theProject and theFeature are the one project and the one feature almost every case here is about.
const (
	theProject = "5f1a77c0e1a24c1f9a0b2d3e"
	theFeature = "b2c3d4e5f60718293a4b5c6d"
)

// aPathOf is a control plane holding one project, one feature, and the steps handed to it. The steps
// arrive in number order, which is the order the control plane answers in.
func aPathOf(steps ...*quaycrewv1.Step) *pathClient {
	return &pathClient{
		features: []*quaycrewv1.Feature{{Id: theFeature, Project: theProject, Number: 1, Title: "the bills"}},
		steps:    map[string][]*quaycrewv1.Step{theFeature: steps},
	}
}

// aStep is one row of a path, ready and unproven, which is what every step is born as.
func aStep(number int32, title string) *quaycrewv1.Step {
	return &quaycrewv1.Step{
		Feature: theFeature, Number: number, Title: title,
		State: stepReady, ProofState: proofUnproven,
	}
}

// drawnBy drives a resource's own List the way a refresh does, and hands back the cells of each row
// in the order the table draws them, which is what the operator is left looking at.
func drawnBy(t *testing.T, resource Resource, parent string) [][]string {
	t.Helper()
	rows, err := resource.List(context.Background(), parent)
	if err != nil {
		t.Fatalf("list %s: %v", resource.Name, err)
	}
	model := newTestModel(t, resource)
	model, _ = update(t, model, rowsFor(model, rows...))

	visible := model.visibleRows()
	drawn := make([][]string, 0, len(visible))
	for _, row := range visible {
		drawn = append(drawn, row.Cells)
	}
	return drawn
}

// cellsOf is the column of one cell down the whole listing, for an assertion about what a column says
// rather than about one row.
func cellsOf(drawn [][]string, column int) []string {
	read := make([]string, 0, len(drawn))
	for _, cells := range drawn {
		if column < len(cells) {
			read = append(read, cells[column])
		}
	}
	return read
}

// onlyStepDrawn is the single row of a path of one step, which is the shape of every case about what
// one cell says.
func onlyStepDrawn(t *testing.T, step *quaycrewv1.Step) []string {
	t.Helper()
	drawn := drawnBy(t, Path(aPathOf(step)), theProject)
	if len(drawn) != 1 {
		t.Fatalf("the view drew %d rows, want 1: %v", len(drawn), drawn)
	}
	return drawn[0]
}

// Where each cell of a step row sits, named so an assertion reads as the column it is about.
const (
	numberCell   = 0
	titleCell    = 1
	stateCell    = 2
	proofColumn  = 3
	closedByCell = 4
	sessionCell  = 5
)

// The columns and their widths are the contract's, in the contract's order, and the title takes what
// is left. A column added in the wrong place moves every cell an action reads back out of a row.
func TestThePathViewDrawsTheColumnsTheContractNames(t *testing.T) {
	wanted := []struct {
		title string
		width int
	}{
		{"number", 6}, {"title", 0}, {"state", 12}, {"proof", 10},
		{"closed by", 10}, {"session", 10}, {"age", 10},
	}
	columns := Path(aPathOf()).Columns
	if len(columns) != len(wanted) {
		t.Fatalf("the path view draws %d columns, want %d", len(columns), len(wanted))
	}
	for at, want := range wanted {
		if columns[at].Title != want.title {
			t.Errorf("column %d is headed %q, want %q", at, columns[at].Title, want.title)
		}
		if columns[at].Width != want.width {
			t.Errorf("the %s column is %d wide, want %d", want.title, columns[at].Width, want.width)
		}
	}
	// A width of zero is the column that takes what is left over, and the title is the one cell here
	// that is a sentence.
	if columns[titleCell].Width != 0 {
		t.Errorf("the title column is fixed at %d, so a title is cut to it", columns[titleCell].Width)
	}
}

// The path is drawn in number order, which is the order the control plane answers in. These cells are
// rendered text, so a view that ordered them itself would compare "10" against "2" as words.
//
// Eleven steps rather than nine: nine steps sort the same either way, and the defect is only visible
// once a number has two digits.
func TestAPathOfElevenStepsDrawsStepTwoAboveStepTen(t *testing.T) {
	steps := make([]*quaycrewv1.Step, 0, 11)
	for number := int32(1); number <= 11; number++ {
		steps = append(steps, aStep(number, fmt.Sprintf("the step numbered %d", number)))
	}
	drawn := drawnBy(t, Path(aPathOf(steps...)), theProject)

	want := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"}
	if got := cellsOf(drawn, numberCell); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("the view draws the steps in the order %v, want %v", got, want)
	}
}

// The view reads the project's features and then each feature's path, because a path belongs to a
// feature and ListSteps names one. Two features of one project both belong on the screen.
func TestThePathViewDrawsEveryFeatureOfTheProject(t *testing.T) {
	second := "c3d4e5f60718293a4b5c6d7e"
	client := aPathOf(aStep(1, "the store holds a brief"))
	client.features = append(client.features,
		&quaycrewv1.Feature{Id: second, Project: theProject, Number: 2, Title: "the second part"})
	client.steps[second] = []*quaycrewv1.Step{
		{Feature: second, Number: 1, Title: "the second feature's own first step",
			State: stepReady, ProofState: proofUnproven},
	}

	drawn := drawnBy(t, Path(client), theProject)
	if len(drawn) != 2 {
		t.Fatalf("the view drew %d rows over two features, want 2: %v", len(drawn), drawn)
	}
	if strings.Join(client.asked, ",") != theFeature+","+second {
		t.Fatalf("the view read the paths of %v, want both features of the project", client.asked)
	}
}

// A project the operator has not drilled into is not a project, so the listing is scoped to the one
// they did.
func TestThePathViewLeavesOutAnotherProjectsFeature(t *testing.T) {
	elsewhere := "d4e5f60718293a4b5c6d7e8f"
	client := aPathOf(aStep(1, "the store holds a brief"))
	client.features = append(client.features,
		&quaycrewv1.Feature{Id: elsewhere, Project: "another project", Number: 1, Title: "somebody else's"})
	client.steps[elsewhere] = []*quaycrewv1.Step{
		{Feature: elsewhere, Number: 1, Title: "a step of another project",
			State: stepReady, ProofState: proofUnproven},
	}

	drawn := drawnBy(t, Path(client), theProject)
	if len(drawn) != 1 {
		t.Fatalf("the view drew %d rows, want the one step of this project: %v", len(drawn), drawn)
	}
}

// A restatement is written by the session holding the step and then waits for a person to read it.
// Nothing on the wire says so, so the cell is worked out from the row.
func TestAStepWhoseRestatementNobodyReadWaitsOnTheOperator(t *testing.T) {
	step := aStep(3, "the design reaches the session")
	step.State, step.Session = stepTaken, "9ab30e1744c1f9a0b2d3e4f5"
	step.Restatement = "what this step changes, in my own words"

	if got := onlyStepDrawn(t, step)[stateCell]; got != waitingOnYou {
		t.Fatalf("a step whose restatement nobody read draws %q, want %q", got, waitingOnYou)
	}
}

// The other half of the same cell: the scenario passed, and until somebody speaks the word the step
// is not closed. Both are the operator's move and neither is a state the step is in.
func TestAStepWhoseCheckPassedWithNoWordWaitsOnTheOperator(t *testing.T) {
	step := aStep(4, "a design carries an approval")
	step.State, step.Session = stepTaken, "77e1a0b944c1f9a0b2d3e4f5"
	step.Restatement, step.RestatementApproved = "read and approved", true
	step.ProofState, step.ProofScenariosRun = proofPassing, 1

	if got := onlyStepDrawn(t, step)[stateCell]; got != waitingOnYou {
		t.Fatalf("a step whose check passed and whose word is missing draws %q, want %q", got, waitingOnYou)
	}
}

// An approved restatement is a step that is moving again, so the cell goes back to saying where the
// step is. Without this the view would say the operator owes something to every step in flight.
func TestAStepWhoseRestatementWasReadDrawsItsOwnState(t *testing.T) {
	step := aStep(5, "the operator edits the design body")
	step.State, step.Session = stepTaken, "31a5c8d244c1f9a0b2d3e4f5"
	step.Restatement, step.RestatementApproved = "what this step changes", true

	if got := onlyStepDrawn(t, step)[stateCell]; got != stepTaken {
		t.Fatalf("a step whose restatement was read draws %q, want %q", got, stepTaken)
	}
}

// The four words the control plane holds are drawn as they are. Only the two shapes above are
// derived, and a view that derived more would be answering for the control plane.
func TestTheStateCellDrawsTheWordTheControlPlaneHolds(t *testing.T) {
	for _, state := range []string{stepReady, stepDone, stepStopped} {
		step := aStep(1, "the store holds a project brief")
		step.State = state
		if state != stepReady {
			step.ClosedBy, step.ProofState = "operator", proofPassing
		}
		if got := onlyStepDrawn(t, step)[stateCell]; got != state {
			t.Fatalf("a step in state %q draws %q", state, got)
		}
	}
}

// What krewe's own run of the scenario reported, in the three words it can report.
func TestTheProofCellDrawsWhatTheRunReported(t *testing.T) {
	for _, state := range []string{proofUnproven, proofPassing, proofFailing} {
		step := aStep(1, "the store holds a project brief")
		step.State, step.ProofState = stepDone, state
		step.ClosedBy = "operator"
		if got := onlyStepDrawn(t, step)[proofColumn]; got != state {
			t.Fatalf("a step whose run reported %q draws %q", state, got)
		}
	}
}

// Who spoke the word that closed the step, and nothing at all where nobody has. Nothing there is not
// the word operator: a step krewe closed itself was read by nobody, and that is the difference the
// column exists to carry.
func TestTheClosedByCellDrawsWhoSpokeTheWord(t *testing.T) {
	for _, closer := range []string{"krewe", "operator", ""} {
		step := aStep(1, "the store holds a project brief")
		step.State, step.ProofState, step.ClosedBy = stepDone, proofPassing, closer
		if closer == "" {
			step.State = stepReady
			step.ProofState = proofUnproven
		}
		if got := onlyStepDrawn(t, step)[closedByCell]; got != closer {
			t.Fatalf("a step closed by %q draws %q in the closed by cell", closer, got)
		}
	}
}

// Enter on a step nobody took has nowhere to go, and the refusal names the step rather than opening
// the project's sessions on a row that asked for one session.
func TestEnterOnAStepNobodyTookRefusesAndStaysWhereItIs(t *testing.T) {
	model := pathAt(t, aPathOf(aStep(3, "the riskiest assumption is measured")))
	model, _ = update(t, model, tea.KeyMsg{Type: tea.KeyEnter})

	if model.active.Name != "path" {
		t.Fatalf("the console moved to %q, and there was nothing to open", model.active.Name)
	}
	want := "nobody took step 3, so there is no session to open"
	if model.err == nil || model.err.Error() != want {
		t.Fatalf("the console says %v, want %q", model.err, want)
	}
}

// Enter on a step that names a session opens the project's sessions. Landing on that one row needs a
// mechanism the console does not have, and the contract defers it.
func TestEnterOnATakenStepOpensTheProjectsSessions(t *testing.T) {
	step := aStep(2, "the design reaches the session")
	step.State, step.Session = stepTaken, "9ab30e1744c1f9a0b2d3e4f5"
	model := pathAt(t, aPathOf(step))
	model, _ = update(t, model, tea.KeyMsg{Type: tea.KeyEnter})

	if model.err != nil {
		t.Fatalf("enter on a taken step says %v", model.err)
	}
	if model.active.Name != "sessions" {
		t.Fatalf("enter opened %q, want the sessions view", model.active.Name)
	}
	if model.parent != theProject {
		t.Fatalf("the sessions view is scoped to %q, want the step's project %q", model.parent, theProject)
	}
	// The line under the list says where the operator is standing, and every other view says it in
	// words a person could type. A step is typed as <feature number>.<step number>.
	if got := model.Position(); got != "1.2" {
		t.Fatalf("the console says the operator is standing at %q, want the step they came from", got)
	}
}

// A call that fails leaves the view empty and hands back the reason, the way every other view does.
func TestThePathViewHandsBackWhyItCouldNotRead(t *testing.T) {
	client := aPathOf(aStep(1, "the store holds a project brief"))
	client.listErr = fmt.Errorf("the control plane is not answering")

	rows, err := Path(client).List(context.Background(), theProject)
	if err == nil {
		t.Fatal("a failed call drew a listing rather than saying what went wrong")
	}
	if len(rows) != 0 {
		t.Fatalf("a failed call drew %d rows", len(rows))
	}
	if !strings.Contains(err.Error(), "not answering") {
		t.Fatalf("the view says %v, and loses what the control plane said", err)
	}
}

// The view is reachable by its own name and by the word an operator's fingers reach for, and the
// letters p and s still open what they always opened.
func TestThePathViewOpensByNameAndByItsAlias(t *testing.T) {
	registry, err := NewDefaultRegistry(&fakeClient{})
	if err != nil {
		t.Fatalf("NewDefaultRegistry: %v", err)
	}
	for _, token := range []string{"path", "steps"} {
		resource, found := registry.Resolve(token)
		if !found {
			t.Fatalf("Resolve(%q): the console cannot open it", token)
		}
		if resource.Name != "path" {
			t.Fatalf("Resolve(%q) = %q, want path", token, resource.Name)
		}
	}
	for token, want := range map[string]string{"p": "projects", "s": "sessions"} {
		resource, found := registry.Resolve(token)
		if !found {
			t.Fatalf("Resolve(%q): not found", token)
		}
		if resource.Name != want {
			t.Fatalf("Resolve(%q) = %q, want %q", token, resource.Name, want)
		}
	}
}

// pathAt stands the console up on the path of the project, with its rows loaded and the cursor on the
// first of them, which is where an operator is when they press a key.
func pathAt(t *testing.T, client *pathClient) Model {
	t.Helper()
	path := Path(client)
	model := newTestModel(t, path, Sessions(client))
	model.parent = theProject
	rows, err := path.List(context.Background(), theProject)
	if err != nil {
		t.Fatalf("list path: %v", err)
	}
	model, _ = update(t, model, rowsFor(model, rows...))
	if len(model.visibleRows()) == 0 {
		t.Fatal("the console has no row to press a key on")
	}
	return model
}

// ---------- the projects view counts the path, the trust and the flight ----------

// projectsClient answers the five calls the projects view makes, and counts each one, so a test can
// say what the view read as well as what it drew. It embeds the generated interface, so a call this
// view grows and this double does not answer panics loudly rather than being quietly satisfied.
//
// It answers ListSteps and ListFeatures the way the control plane does: a request naming nothing
// answers for everything, and a request naming a feature it does not hold is refused. A view that
// went row by row would work against a looser double and fail against the real thing.
type projectsClient struct {
	quaycrewv1.ControlPlaneServiceClient

	workspaces []*quaycrewv1.Workspace
	projects   []*quaycrewv1.Project
	features   []*quaycrewv1.Feature
	steps      []*quaycrewv1.Step
	designs    map[string]*quaycrewv1.Design

	stepsErr  error
	designErr error

	// counted is how many times each call was made, by the name of the call.
	counted map[string]int
}

func (p *projectsClient) count(call string) {
	if p.counted == nil {
		p.counted = map[string]int{}
	}
	p.counted[call]++
}

func (p *projectsClient) ListWorkspaces(context.Context, *quaycrewv1.ListWorkspacesRequest,
	...grpc.CallOption) (*quaycrewv1.ListWorkspacesResponse, error) {
	p.count("ListWorkspaces")
	return &quaycrewv1.ListWorkspacesResponse{Workspaces: p.workspaces}, nil
}

func (p *projectsClient) ListProjects(_ context.Context, req *quaycrewv1.ListProjectsRequest,
	_ ...grpc.CallOption) (*quaycrewv1.ListProjectsResponse, error) {
	p.count("ListProjects")
	matched := make([]*quaycrewv1.Project, 0, len(p.projects))
	for _, project := range p.projects {
		if req.GetWorkspace() == "" || project.GetWorkspace() == req.GetWorkspace() {
			matched = append(matched, project)
		}
	}
	return &quaycrewv1.ListProjectsResponse{Projects: matched}, nil
}

func (p *projectsClient) ListFeatures(_ context.Context, req *quaycrewv1.ListFeaturesRequest,
	_ ...grpc.CallOption) (*quaycrewv1.ListFeaturesResponse, error) {
	p.count("ListFeatures")
	if p.stepsErr != nil {
		return nil, p.stepsErr
	}
	if req.GetProject() == "" {
		return &quaycrewv1.ListFeaturesResponse{Features: p.features}, nil
	}
	matched := make([]*quaycrewv1.Feature, 0, len(p.features))
	for _, feature := range p.features {
		if feature.GetProject() == req.GetProject() {
			matched = append(matched, feature)
		}
	}
	return &quaycrewv1.ListFeaturesResponse{Features: matched}, nil
}

func (p *projectsClient) ListSteps(_ context.Context, req *quaycrewv1.ListStepsRequest,
	_ ...grpc.CallOption) (*quaycrewv1.ListStepsResponse, error) {
	p.count("ListSteps")
	if p.stepsErr != nil {
		return nil, p.stepsErr
	}
	if req.GetFeature() == "" {
		return &quaycrewv1.ListStepsResponse{Steps: p.steps}, nil
	}
	held := make([]*quaycrewv1.Step, 0, len(p.steps))
	for _, step := range p.steps {
		if step.GetFeature() == req.GetFeature() {
			held = append(held, step)
		}
	}
	if len(held) == 0 {
		return nil, fmt.Errorf("no feature %q", req.GetFeature())
	}
	return &quaycrewv1.ListStepsResponse{Steps: held}, nil
}

func (p *projectsClient) GetDesign(_ context.Context, req *quaycrewv1.GetDesignRequest,
	_ ...grpc.CallOption) (*quaycrewv1.GetDesignResponse, error) {
	p.count("GetDesign")
	if p.designErr != nil {
		return nil, p.designErr
	}
	if held, known := p.designs[req.GetProject()]; known {
		return &quaycrewv1.GetDesignResponse{Design: held}, nil
	}
	return &quaycrewv1.GetDesignResponse{Design: bornDesign(req.GetProject())}, nil
}

// theWorkspace is the one workspace every case about the projects view is drilled into.
const theWorkspace = "7b6c5d4e3f2a1b0c9d8e7f6a"

// aProjectOf is a control plane holding one workspace, one project, one feature, and the steps handed
// to it. It is the shape of every case about what one cell of a project row says.
func aProjectOf(steps ...*quaycrewv1.Step) *projectsClient {
	return &projectsClient{
		workspaces: []*quaycrewv1.Workspace{{Id: theWorkspace, Name: "acme"}},
		projects: []*quaycrewv1.Project{
			{Id: theProject, Workspace: theWorkspace, Name: "house-bills"},
		},
		features: []*quaycrewv1.Feature{
			{Id: theFeature, Project: theProject, Number: 1, Title: "the bills"},
		},
		steps: steps,
	}
}

// aStepIn is one step of the one feature, in the state handed to it.
func aStepIn(number int32, state string) *quaycrewv1.Step {
	step := aStep(number, fmt.Sprintf("the step numbered %d", number))
	step.State = state
	return step
}

// designedWith is the project's design row, carrying a body so the project counts as designed, and
// whatever trust record the case is about.
func designedWith(design *quaycrewv1.Design) map[string]*quaycrewv1.Design {
	design.Project = theProject
	if design.GetBody() == "" {
		design.Body = "the design, as the operator approved it"
	}
	if design.GetStepsInFlightCap() == 0 {
		design.StepsInFlightCap = 10
	}
	return map[string]*quaycrewv1.Design{theProject: design}
}

// Where each cell of a project row sits, named so an assertion reads as the column it is about.
const (
	projectIDCell     = 0
	projectNameCell   = 1
	deploysToCell     = 3
	projectPathCell   = 4
	projectTrustCell  = 5
	projectFlightCell = 6
)

// onlyProjectDrawn is the single row of a listing of one project, which is the shape of every case
// about what one cell says.
func onlyProjectDrawn(t *testing.T, client *projectsClient) []string {
	t.Helper()
	drawn := drawnBy(t, Projects(client), theWorkspace)
	if len(drawn) != 1 {
		t.Fatalf("the view drew %d rows, want 1: %v", len(drawn), drawn)
	}
	return drawn[0]
}

// The columns and their widths are the contract's, in the contract's order. A column added in the
// wrong place moves every cell a test and an action read back out of a row.
func TestTheProjectsViewDrawsTheColumnsTheContractNames(t *testing.T) {
	wanted := []struct {
		title string
		width int
	}{
		{"id", 10}, {"name", 24}, {"workspace", 18}, {"deploys to", 26},
		{"path", 6}, {"trust", 15}, {"flight", 6}, {"age", 0},
	}
	columns := Projects(aProjectOf()).Columns
	if len(columns) != len(wanted) {
		t.Fatalf("the projects view draws %d columns, want %d", len(columns), len(wanted))
	}
	for at, want := range wanted {
		if columns[at].Title != want.title {
			t.Errorf("column %d is headed %q, want %q", at, columns[at].Title, want.title)
		}
		if columns[at].Width != want.width {
			t.Errorf("the %s column is %d wide, want %d", want.title, columns[at].Width, want.width)
		}
	}
	// The whole record a standing offer draws, which is the longest thing the trust column holds.
	// A column narrower than it cuts the word the offer is made of.
	if columns[projectTrustCell].Width < len("0 (5/5) offered") {
		t.Errorf("the trust column is %d wide, and a standing offer is %d characters",
			columns[projectTrustCell].Width, len("0 (5/5) offered"))
	}
}

// How far the path got, out of how long it is, on the listing the operator already reads.
func TestAProjectWithSevenStepsThreeOfThemDoneDrawsThreeOfSeven(t *testing.T) {
	steps := make([]*quaycrewv1.Step, 0, 7)
	for number := int32(1); number <= 7; number++ {
		state := stepReady
		if number <= 3 {
			state = stepDone
		}
		steps = append(steps, aStepIn(number, state))
	}

	if got := onlyProjectDrawn(t, aProjectOf(steps...))[projectPathCell]; got != "3/7" {
		t.Fatalf("a project with seven steps and three done draws %q, want %q", got, "3/7")
	}
}

// Nothing there is not a count of zero. A project nobody wrote a path for draws an empty cell, and
// never 0/0: 0/0 reads as a path that exists and has not started, and the two are different
// questions.
func TestAProjectWithNoPathDrawsAnEmptyPathCellAndNeverZeroOfZero(t *testing.T) {
	client := aProjectOf()
	client.features = nil

	got := onlyProjectDrawn(t, client)[projectPathCell]
	if got == "0/0" {
		t.Fatalf("a project with no path draws %q, and a path nobody wrote is not a path of no steps", got)
	}
	if got != "" {
		t.Fatalf("a project with no path draws %q, want an empty cell", got)
	}
}

// The level with the run behind it, and the threshold that run is counted against. The number alone
// says where krewe stands and not how close it is to the next question.
func TestTheTrustCellDrawsTheLevelWithTheRunBehindIt(t *testing.T) {
	client := aProjectOf()
	client.designs = designedWith(&quaycrewv1.Design{
		TrustLevel: 0, TrustRun: 2, TrustThreshold: 5, TrustAgreements: 2,
	})

	if got := onlyProjectDrawn(t, client)[projectTrustCell]; got != "0 (2/5)" {
		t.Fatalf("a project at level 0 with two agreements in a row draws %q, want %q", got, "0 (2/5)")
	}
}

// An offer stands until the operator answers it, so the listing says so. Without this the operator
// only finds the offer by running krewe trust on each project in turn.
func TestAProjectWithAStandingOfferSaysSoInTheTrustCell(t *testing.T) {
	client := aProjectOf()
	client.designs = designedWith(&quaycrewv1.Design{
		TrustLevel: 0, TrustRun: 5, TrustThreshold: 5, TrustOffered: true,
	})

	got := onlyProjectDrawn(t, client)[projectTrustCell]
	if !strings.Contains(got, "offered") {
		t.Fatalf("a project with a standing offer draws %q, and the offer is not in it", got)
	}
	if got != "0 (5/5) offered" {
		t.Fatalf("a project with a standing offer draws %q, want %q", got, "0 (5/5) offered")
	}
}

// A project nobody designed has taken no step, so it agreed with nothing. A record of zeroes there
// reads as a project that tried and failed, which is the reading krewe trust already refuses to give.
func TestAProjectWithNoDesignDrawsAnEmptyTrustCell(t *testing.T) {
	if got := onlyProjectDrawn(t, aProjectOf())[projectTrustCell]; got != "" {
		t.Fatalf("a project with no design draws %q in the trust cell, want an empty cell", got)
	}
}

// How much of the project is moving, out of how much may move at once. Both numbers are what the
// control plane refuses the next take against.
func TestTheFlightCellCountsTheStepsInFlightAgainstTheCap(t *testing.T) {
	client := aProjectOf(
		aStepIn(1, stepDone), aStepIn(2, stepTaken), aStepIn(3, stepTaken), aStepIn(4, stepReady))
	client.designs = designedWith(&quaycrewv1.Design{StepsInFlightCap: 3})

	if got := onlyProjectDrawn(t, client)[projectFlightCell]; got != "2/3" {
		t.Fatalf("a project with two steps taken and a cap of three draws %q, want %q", got, "2/3")
	}
}

// A project at its cap is drawn plainly. The operator reads the refusal when they take the next step,
// and a listing that marked a full project would be marking the normal state of a project with work
// in it.
func TestAProjectAtItsCapDrawsTheCountAndTheCapPlainly(t *testing.T) {
	client := aProjectOf(aStepIn(1, stepTaken), aStepIn(2, stepTaken), aStepIn(3, stepTaken))
	client.designs = designedWith(&quaycrewv1.Design{StepsInFlightCap: 3})

	got := onlyProjectDrawn(t, client)[projectFlightCell]
	if got != "3/3" {
		t.Fatalf("a project at its cap draws %q, want %q", got, "3/3")
	}
	for _, mark := range []string{"!", "*", "\x1b"} {
		if strings.Contains(got, mark) {
			t.Fatalf("a project at its cap draws %q, and the cell carries a mark", got)
		}
	}
}

// A listing that cannot count steps still has rows worth drawing, so the failure is swallowed the way
// GetUsage already is in the header. The two counted cells go empty and the row stays.
func TestAFailedStepsCallLeavesThePathAndFlightCellsEmptyAndStillDrawsTheRow(t *testing.T) {
	client := aProjectOf(aStepIn(1, stepDone), aStepIn(2, stepTaken))
	client.designs = designedWith(&quaycrewv1.Design{StepsInFlightCap: 3})
	client.stepsErr = fmt.Errorf("the control plane is not answering")

	rows, err := Projects(client).List(context.Background(), theWorkspace)
	if err != nil {
		t.Fatalf("a listing that could not count steps refused to draw at all: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("the view drew %d rows, and a project it cannot count is still a project", len(rows))
	}
	if got := rows[0].Cells[projectPathCell]; got != "" {
		t.Errorf("a failed steps call draws %q in the path cell, want an empty cell", got)
	}
	if got := rows[0].Cells[projectFlightCell]; got != "" {
		t.Errorf("a failed steps call draws %q in the flight cell, want an empty cell", got)
	}
	// The name is still there, which is what makes the row worth drawing.
	if got := rows[0].Cells[projectNameCell]; got != "house-bills" {
		t.Errorf("the row names the project as %q, want house-bills", got)
	}
}

// The same swallow on the other read. A row that cannot say where trust sits is still a row.
func TestAFailedDesignReadLeavesTheTrustAndFlightCellsEmptyAndStillDrawsTheRow(t *testing.T) {
	client := aProjectOf(aStepIn(1, stepDone), aStepIn(2, stepTaken))
	client.designErr = fmt.Errorf("the control plane is not answering")

	rows, err := Projects(client).List(context.Background(), theWorkspace)
	if err != nil {
		t.Fatalf("a listing that could not read a design refused to draw at all: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("the view drew %d rows, and a project it cannot read is still a project", len(rows))
	}
	if got := rows[0].Cells[projectTrustCell]; got != "" {
		t.Errorf("a failed design read draws %q in the trust cell, want an empty cell", got)
	}
	if got := rows[0].Cells[projectFlightCell]; got != "" {
		t.Errorf("a failed design read draws %q in the flight cell, want an empty cell", got)
	}
	// The path cell is counted from the steps, which answered, so it is still drawn.
	if got := rows[0].Cells[projectPathCell]; got != "1/2" {
		t.Errorf("a failed design read draws %q in the path cell, want %q", got, "1/2")
	}
}

// The count the listing promises. The console refreshes itself every few seconds, so a call per row
// is a call per row per refresh, and a page of forty projects would make forty of them.
//
// The steps are counted in two calls whatever the page holds: one for the features, because a step
// carries its feature and not its project, and one for the steps. The design is read once per project
// because GetDesign takes one project and there is no call that answers for several.
func TestAPageOfProjectsIsCountedInAFixedNumberOfCalls(t *testing.T) {
	client := aProjectOf()
	client.steps = nil
	for at := 2; at <= 6; at++ {
		project := fmt.Sprintf("%024d", at)
		feature := fmt.Sprintf("%024d", at*100)
		client.projects = append(client.projects, &quaycrewv1.Project{
			Id: project, Workspace: theWorkspace, Name: fmt.Sprintf("project %d", at),
		})
		client.features = append(client.features,
			&quaycrewv1.Feature{Id: feature, Project: project, Number: 1, Title: "the only part"})
		client.steps = append(client.steps, &quaycrewv1.Step{
			Feature: feature, Number: 1, Title: "the one step", State: stepTaken, ProofState: proofUnproven,
		})
	}

	drawn := drawnBy(t, Projects(client), theWorkspace)
	if len(drawn) != 6 {
		t.Fatalf("the view drew %d rows, want 6", len(drawn))
	}
	for _, call := range []string{"ListProjects", "ListWorkspaces", "ListFeatures", "ListSteps"} {
		if got := client.counted[call]; got != 1 {
			t.Errorf("drawing six projects made %d %s calls, want 1", got, call)
		}
	}
	if got := client.counted["GetDesign"]; got != len(client.projects) {
		t.Errorf("drawing six projects made %d GetDesign calls, want one per project", got)
	}
}

// One project's steps are not another's. Two projects on one page are counted apart, out of the one
// listing both were read from.
func TestTwoProjectsOnOnePageAreCountedApart(t *testing.T) {
	elsewhere, itsFeature := "d4e5f60718293a4b5c6d7e8f", "e5f60718293a4b5c6d7e8f90"
	client := aProjectOf(aStepIn(1, stepDone), aStepIn(2, stepTaken), aStepIn(3, stepReady))
	client.projects = append(client.projects,
		&quaycrewv1.Project{Id: elsewhere, Workspace: theWorkspace, Name: "gardening"})
	client.features = append(client.features,
		&quaycrewv1.Feature{Id: itsFeature, Project: elsewhere, Number: 1, Title: "somebody else's"})
	client.steps = append(client.steps, &quaycrewv1.Step{
		Feature: itsFeature, Number: 1, Title: "its only step", State: stepDone, ProofState: proofPassing,
	})

	drawn := drawnBy(t, Projects(client), theWorkspace)
	counted := map[string]string{}
	for _, cells := range drawn {
		counted[cells[projectNameCell]] = cells[projectPathCell]
	}
	if counted["house-bills"] != "1/3" {
		t.Errorf("the first project draws %q in the path cell, want %q", counted["house-bills"], "1/3")
	}
	if counted["gardening"] != "1/1" {
		t.Errorf("the second project draws %q in the path cell, want %q", counted["gardening"], "1/1")
	}
}
