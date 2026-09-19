package console

import (
	"context"
	"fmt"
	"strings"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/charmbracelet/lipgloss"
	"google.golang.org/grpc"
)

// dashboardClient answers the calls the panel makes, counts each one, and can refuse any one of them
// on its own. It embeds the generated interface, so a call the panel grows and this double does not
// answer panics loudly rather than being quietly satisfied.
type dashboardClient struct {
	quaycrewv1.ControlPlaneServiceClient

	workspaces []*quaycrewv1.Workspace
	projects   []*quaycrewv1.Project
	sessions   []*quaycrewv1.Session
	features   []*quaycrewv1.Feature
	steps      []*quaycrewv1.Step

	// refuses is the call that will not answer, by the name the panel reads it under, so a case can
	// take one number off the panel and leave the rest.
	refuses panelRead

	// counted is how many times each call was made, by the name of the call.
	counted map[string]int
}

// call records one call and says whether this case refuses it.
func (d *dashboardClient) call(name string, read panelRead) error {
	if d.counted == nil {
		d.counted = map[string]int{}
	}
	d.counted[name]++
	if d.refuses == read {
		return fmt.Errorf("the control plane is not answering")
	}
	return nil
}

func (d *dashboardClient) ListWorkspaces(context.Context, *quaycrewv1.ListWorkspacesRequest,
	...grpc.CallOption) (*quaycrewv1.ListWorkspacesResponse, error) {
	if err := d.call("ListWorkspaces", readWorkspaces); err != nil {
		return nil, err
	}
	return &quaycrewv1.ListWorkspacesResponse{Workspaces: d.workspaces}, nil
}

func (d *dashboardClient) ListProjects(_ context.Context, req *quaycrewv1.ListProjectsRequest,
	_ ...grpc.CallOption) (*quaycrewv1.ListProjectsResponse, error) {
	if err := d.call("ListProjects", readProjects); err != nil {
		return nil, err
	}
	matched := make([]*quaycrewv1.Project, 0, len(d.projects))
	for _, project := range d.projects {
		if req.GetWorkspace() == "" || project.GetWorkspace() == req.GetWorkspace() {
			matched = append(matched, project)
		}
	}
	return &quaycrewv1.ListProjectsResponse{Projects: matched}, nil
}

func (d *dashboardClient) ListSessions(_ context.Context, req *quaycrewv1.ListSessionsRequest,
	_ ...grpc.CallOption) (*quaycrewv1.ListSessionsResponse, error) {
	if err := d.call("ListSessions", readSessions); err != nil {
		return nil, err
	}
	matched := make([]*quaycrewv1.Session, 0, len(d.sessions))
	for _, session := range d.sessions {
		if req.GetWorkspace() == "" || session.GetWorkspace() == req.GetWorkspace() {
			matched = append(matched, session)
		}
	}
	return &quaycrewv1.ListSessionsResponse{Sessions: matched}, nil
}

// ListFeatures and ListSteps answer the way the control plane does: a request naming nothing answers
// for everything, and a request naming one thing is narrowed to it. A panel that read a project at a
// time would work against a looser double and cost a call a project against the real one.
func (d *dashboardClient) ListFeatures(_ context.Context, req *quaycrewv1.ListFeaturesRequest,
	_ ...grpc.CallOption) (*quaycrewv1.ListFeaturesResponse, error) {
	if err := d.call("ListFeatures", readFeatures); err != nil {
		return nil, err
	}
	if req.GetProject() == "" {
		return &quaycrewv1.ListFeaturesResponse{Features: d.features}, nil
	}
	matched := make([]*quaycrewv1.Feature, 0, len(d.features))
	for _, feature := range d.features {
		if feature.GetProject() == req.GetProject() {
			matched = append(matched, feature)
		}
	}
	return &quaycrewv1.ListFeaturesResponse{Features: matched}, nil
}

func (d *dashboardClient) ListSteps(_ context.Context, req *quaycrewv1.ListStepsRequest,
	_ ...grpc.CallOption) (*quaycrewv1.ListStepsResponse, error) {
	if err := d.call("ListSteps", readSteps); err != nil {
		return nil, err
	}
	if req.GetFeature() == "" {
		return &quaycrewv1.ListStepsResponse{Steps: d.steps}, nil
	}
	held := make([]*quaycrewv1.Step, 0, len(d.steps))
	for _, step := range d.steps {
		if step.GetFeature() == req.GetFeature() {
			held = append(held, step)
		}
	}
	return &quaycrewv1.ListStepsResponse{Steps: held}, nil
}

func (d *dashboardClient) GetUsage(context.Context, *quaycrewv1.GetUsageRequest,
	...grpc.CallOption) (*quaycrewv1.GetUsageResponse, error) {
	if err := d.call("GetUsage", readSpend); err != nil {
		return nil, err
	}
	return &quaycrewv1.GetUsageResponse{
		Total:    &quaycrewv1.Usage{Input: 1200, Output: 300},
		Sessions: int64(len(d.sessions)),
	}, nil
}

// everyCallThePanelMakes is what one draw costs, by the name of each call. The panel is held to this
// list rather than to a number, so a call quietly added is named in the failure.
var everyCallThePanelMakes = []string{
	"ListWorkspaces", "ListProjects", "ListSessions", "ListFeatures", "ListSteps", "GetUsage",
}

// Where each cell of a panel row sits, named so an assertion reads as the column it is about.
const (
	widgetCell = 0
	saysCell   = 1
)

// aSystemOf is a control plane holding the workspaces, projects and sessions a case is about, with a
// path under each project so every read the panel makes has something to answer with.
func aSystemOf(workspaces, projectsEach, sessionsEach int) *dashboardClient {
	client := &dashboardClient{}
	for w := 1; w <= workspaces; w++ {
		workspace := fmt.Sprintf("%024d", w)
		client.workspaces = append(client.workspaces,
			&quaycrewv1.Workspace{Id: workspace, Name: fmt.Sprintf("workspace-%d", w)})
		for p := 1; p <= projectsEach; p++ {
			project := fmt.Sprintf("%012d%012d", w, p)
			feature := fmt.Sprintf("%08d%08d%08d", w, p, 7)
			client.projects = append(client.projects, &quaycrewv1.Project{
				Id: project, Workspace: workspace, Name: fmt.Sprintf("project-%d-%d", w, p),
			})
			client.features = append(client.features, &quaycrewv1.Feature{
				Id: feature, Project: project, Number: 1, Title: "the only part",
			})
			client.steps = append(client.steps, &quaycrewv1.Step{
				Feature: feature, Number: 1, Title: "the one step",
				State: stepTaken, ProofState: proofUnproven,
			})
			for s := 1; s <= sessionsEach; s++ {
				client.sessions = append(client.sessions, &quaycrewv1.Session{
					Id:      fmt.Sprintf("%08d%08d%08d", w, p, s),
					Handle:  fmt.Sprintf("session-%d-%d-%d", w, p, s),
					Project: project, Workspace: workspace, Status: "idle",
				})
			}
		}
	}
	return client
}

// The titles of the six frames, in the order the panel draws them. What makes the operator act is
// first, at the top of the panel, because that is the one they open the console to find.
var theFrames = []string{
	"what needs you", "in flight", "the path", "trust", "spend", "the pile",
}

// framesDrawn is the title of each frame on the panel, in the order it was drawn.
func framesDrawn(drawn [][]string) []string {
	titles := make([]string, 0, len(theFrames))
	for _, cells := range drawn {
		if cells[widgetCell] != "" {
			titles = append(titles, cells[widgetCell])
		}
	}
	return titles
}

// The panel is a grid of frames, and this is the grid: one frame per widget, each one a title with an
// empty body under it. Nothing has content yet, which is what makes the grid reviewable on its own.
func TestThePanelDrawsAFrameForEveryWidgetWithAnEmptyBody(t *testing.T) {
	drawn := drawnBy(t, Dashboard(aSystemOf(2, 2, 1)), "")

	if got := framesDrawn(drawn); strings.Join(got, ",") != strings.Join(theFrames, ",") {
		t.Fatalf("the panel draws the frames %v, want %v", got, theFrames)
	}
	if len(drawn) != len(theFrames)*2 {
		t.Fatalf("the panel draws %d rows over %d frames, want a title and a body for each",
			len(drawn), len(theFrames))
	}
	// Every frame's body says nothing at all. A body with a word in it is a widget, and the widgets
	// are the steps after this one.
	for at, cells := range drawn {
		if at%2 == 0 {
			continue
		}
		if cells[widgetCell] != "" || cells[saysCell] != "" {
			t.Errorf("the body of the %q frame says %q, want an empty body", drawn[at-1][widgetCell], cells)
		}
	}
}

// The columns and their widths are the contract's, in the contract's order. A column added in the
// wrong place moves every cell a test and an action read back out of a row.
func TestThePanelDrawsTheColumnsTheContractNames(t *testing.T) {
	wanted := []struct {
		title string
		width int
	}{
		{"widget", 18}, {"says", 0},
	}
	columns := Dashboard(aSystemOf(1, 1, 1)).Columns
	if len(columns) != len(wanted) {
		t.Fatalf("the panel draws %d columns, want %d", len(columns), len(wanted))
	}
	for at, want := range wanted {
		if columns[at].Title != want.title {
			t.Errorf("column %d is headed %q, want %q", at, columns[at].Title, want.title)
		}
		if columns[at].Width != want.width {
			t.Errorf("the %s column is %d wide, want %d", want.title, columns[at].Width, want.width)
		}
	}
	// The widest frame title has to fit, or the panel names its own frames in cut words.
	for _, title := range theFrames {
		if len(title) > columns[widgetCell].Width {
			t.Errorf("the %q frame is %d characters and the widget column is %d wide",
				title, len(title), columns[widgetCell].Width)
		}
	}
}

// The count the panel promises, and the reason this test is written while the panel is still empty:
// it is what stops the fifth widget quietly making the panel cost five times as much.
//
// Three workspaces and six projects, so a read made once per workspace or once per project would
// show up as three or six rather than as one. The console draws itself again every three seconds, so
// a call per project is six calls every three seconds on a system this small.
func TestOneDrawOfThePanelReadsTheSystemAFixedNumberOfTimes(t *testing.T) {
	client := aSystemOf(3, 2, 2)

	drawn := drawnBy(t, Dashboard(client), "")
	if len(drawn) != len(theFrames)*2 {
		t.Fatalf("the panel drew %d rows, want %d", len(drawn), len(theFrames)*2)
	}

	for _, call := range everyCallThePanelMakes {
		if got := client.counted[call]; got != 1 {
			t.Errorf("drawing the panel over 3 workspaces and 6 projects made %d %s calls, want 1",
				got, call)
		}
	}
	for call, times := range client.counted {
		if !strings.Contains(strings.Join(everyCallThePanelMakes, ","), call) {
			t.Errorf("the panel made %d %s calls, which is not one of the reads it declares", times, call)
		}
	}
}

// Adding a frame adds no call. The panel reads the system once and hands that one reading to every
// frame, so the cost of the panel is the reading and never the number of frames on it.
func TestTheNumberOfCallsIsTheReadingsAndNeverTheNumberOfFrames(t *testing.T) {
	client := aSystemOf(2, 2, 1)
	if _, err := Dashboard(client).List(context.Background(), ""); err != nil {
		t.Fatalf("the panel refused to draw: %v", err)
	}

	made := 0
	for _, times := range client.counted {
		made += times
	}
	if made != len(everyCallThePanelMakes) {
		t.Fatalf("one draw made %d calls over %d frames, want %d",
			made, len(theFrames), len(everyCallThePanelMakes))
	}
}

// A panel that went blank because one read failed is worse than a panel with a hole in it: the
// operator loses five answers to lose one. Each read is refused in turn, and the grid is still drawn
// whole every time.
func TestAFailedCallLeavesEveryOtherFrameDrawn(t *testing.T) {
	for _, refused := range []panelRead{
		readWorkspaces, readProjects, readSessions, readFeatures, readSteps, readSpend,
	} {
		t.Run(string(refused), func(t *testing.T) {
			client := aSystemOf(2, 2, 1)
			client.refuses = refused

			rows, err := Dashboard(client).List(context.Background(), "")
			if err != nil {
				t.Fatalf("a panel that lost the %s read refused to draw at all: %v", refused, err)
			}
			drawn := make([][]string, 0, len(rows))
			for _, row := range rows {
				drawn = append(drawn, row.Cells)
			}
			if got := framesDrawn(drawn); strings.Join(got, ",") != strings.Join(theFrames, ",") {
				t.Fatalf("losing the %s read draws the frames %v, want all of %v", refused, got, theFrames)
			}
		})
	}
}

// Every read failing at once is the same rule at its end: the panel still says what it is made of,
// and the operator reads six empty frames rather than a blank screen.
func TestAPanelThatCouldReadNothingStillDrawsItsFrames(t *testing.T) {
	client := aSystemOf(1, 1, 1)
	client.refuses = readWorkspaces

	rows, err := Dashboard(client).List(context.Background(), "")
	if err != nil {
		t.Fatalf("the panel refused to draw: %v", err)
	}
	if len(rows) != len(theFrames)*2 {
		t.Fatalf("the panel drew %d rows, want %d", len(rows), len(theFrames)*2)
	}
}

// A frame wider than the window is a panel nobody can read. At forty columns the widget column and
// what is left of the flexible one still lay out inside the panel: every row is exactly the width
// inside the frame, cut rather than spilling past the border and wrapping the terminal.
func TestThePanelLaysOutInsideANarrowWindow(t *testing.T) {
	for _, columns := range []int{40, 80, 200} {
		model := newTestModel(t, Dashboard(aSystemOf(2, 2, 1)))
		model.width = columns
		rows, err := model.active.List(context.Background(), "")
		if err != nil {
			t.Fatalf("list dashboard: %v", err)
		}
		model, _ = update(t, model, rowsFor(model, rows...))

		for _, line := range model.bodyLines(model.visibleRows()) {
			if width := lipgloss.Width(line); width != model.innerWidth() {
				t.Fatalf("at %d columns a panel row is %d wide and the panel is %d: %q",
					columns, width, model.innerWidth(), line)
			}
		}
		// The frame titles are still on screen at the narrowest width, in the column that never
		// gives way, so a narrow window costs the operator the detail and never the grid.
		drawn := model.View()
		for _, title := range theFrames {
			if !strings.Contains(stripped(drawn), title) {
				t.Fatalf("at %d columns the panel does not draw the %q frame:\n%s", columns, title, drawn)
			}
		}
	}
}

// A system with nothing in it has nothing for six frames to say, and an empty listing is what offers
// the operator the guided setup. A grid of empty frames on a first run would be a screen that says
// nothing and offers nothing.
func TestASystemWithNoWorkspacesDrawsNoPanelAtAll(t *testing.T) {
	rows, err := Dashboard(&dashboardClient{}).List(context.Background(), "")
	if err != nil {
		t.Fatalf("the panel refused to draw: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("a system with no workspaces draws %d rows, want none", len(rows))
	}
}

// A system that cannot say whether it has any workspaces is not an empty system, so the panel is
// drawn rather than hidden. Without this, a control plane that stopped answering would look like a
// system nobody has set up yet.
func TestASystemThatCouldNotBeReadStillDrawsThePanel(t *testing.T) {
	client := &dashboardClient{refuses: readWorkspaces}

	rows, err := Dashboard(client).List(context.Background(), "")
	if err != nil {
		t.Fatalf("the panel refused to draw: %v", err)
	}
	if len(rows) != len(theFrames)*2 {
		t.Fatalf("a system that could not be read draws %d rows, want the whole grid", len(rows))
	}
}

// The word and the two letters an operator types to get back to the panel. They are checked against
// the whole registry, so a spelling taken by another view fails here rather than shadowing it.
func TestTheDashboardOpensByItsNameAndByBothAliases(t *testing.T) {
	registry, err := NewDefaultRegistry(&fakeClient{})
	if err != nil {
		t.Fatalf("NewDefaultRegistry: %v", err)
	}
	for _, typed := range []string{"dashboard", "d", "home"} {
		resource, found := registry.Resolve(typed)
		if !found {
			t.Fatalf("typing %q opens nothing", typed)
		}
		if resource.Name != "dashboard" {
			t.Fatalf("typing %q opens %q, want the dashboard", typed, resource.Name)
		}
	}
}

// Moving the default takes nothing away: the tree the console used to open on is one word away, and
// so is every other view.
func TestTheConsoleOpensOnThePanelAndTheTreeIsStillOneWordAway(t *testing.T) {
	if Default != "dashboard" {
		t.Fatalf("the console opens on %q, want the dashboard", Default)
	}
	registry, err := NewDefaultRegistry(&fakeClient{})
	if err != nil {
		t.Fatalf("NewDefaultRegistry: %v", err)
	}
	for _, typed := range []string{"workspaces", "w", "ws", "workspace"} {
		resource, found := registry.Resolve(typed)
		if !found {
			t.Fatalf("typing %q opens nothing, and the tree has to stay reachable", typed)
		}
		if resource.Name != "workspaces" {
			t.Fatalf("typing %q opens %q, want the workspaces", typed, resource.Name)
		}
	}
}

// The panel is drawn rather than listed, so what goes down a pipe is the tree it used to open on.
// Six frame titles in a file are no use to anybody.
func TestAPipedConsoleStillPrintsTheTree(t *testing.T) {
	client := &fakeClient{workspaces: []*quaycrewv1.Workspace{{Id: "w1", Name: "acme"}}}

	var out strings.Builder
	if err := Plain(context.Background(), client, &out); err != nil {
		t.Fatalf("Plain: %v", err)
	}
	if !strings.Contains(out.String(), "acme") {
		t.Fatalf("the piped output %q does not list the workspace", out.String())
	}
	for _, title := range theFrames {
		if strings.Contains(out.String(), title) {
			t.Fatalf("the piped output carries the %q frame:\n%s", title, out.String())
		}
	}
}
