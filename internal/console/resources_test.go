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
