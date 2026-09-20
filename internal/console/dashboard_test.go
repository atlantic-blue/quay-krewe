package console

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/workspace"
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

// bodyOf is what one frame says, found by the title above it: every row from that title down to the
// next one. It reads the grid rather than indexing into it, because the frames no longer hold one
// body row each and an index into a grid that grows is a test that passes by accident.
func bodyOf(drawn [][]string, title string) []string {
	said := make([]string, 0, len(drawn))
	inside := false
	for _, cells := range drawn {
		if cells[widgetCell] != "" {
			inside = cells[widgetCell] == title
			continue
		}
		if inside {
			said = append(said, cells[saysCell])
		}
	}
	return said
}

// The panel is a grid of frames, and this is the grid: one frame per widget, in the order the panel
// declares them, with what needs the operator at the top.
//
// Every frame below the two that have been built says nothing at all. A body with a word in it is a
// widget, and those widgets are the steps after this one.
func TestThePanelDrawsAFrameForEveryWidget(t *testing.T) {
	drawn := drawnBy(t, Dashboard(aSystemOf(2, 2, 1)), "")

	if got := framesDrawn(drawn); strings.Join(got, ",") != strings.Join(theFrames, ",") {
		t.Fatalf("the panel draws the frames %v, want %v", got, theFrames)
	}
	for _, title := range theFrames[2:] {
		body := bodyOf(drawn, title)
		if len(body) != 1 || body[0] != "" {
			t.Errorf("the body of the %q frame says %q, want one empty row", title, body)
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

// The two addresses the first system aSystemOf builds draws, written out rather than composed, so a
// change to how an address is put together fails here instead of agreeing with itself.
const (
	theProjectAddress = "workspace-1/project-1-1"
	theFirstSession   = "workspace-1/project-1-1/session-1-1-1"
)

// aTakenStepOf adds a step somebody holds to the first project of a system. Every case below starts
// from one of these and moves the one field it is about, so what makes a step wait is the only
// difference between a case that draws a line and a case that draws none.
func aTakenStepOf(client *dashboardClient, number int32) *quaycrewv1.Step {
	step := &quaycrewv1.Step{
		Feature: client.features[0].GetId(), Number: number, Title: "a step somebody holds",
		State: stepTaken, ProofState: proofUnproven,
	}
	client.steps = append(client.steps, step)
	return step
}

// What the frame says, case by case. Three counts, each with the address of the first thing behind
// it, and one line rather than three zeros on a system where nothing has stopped.
//
// The two step shapes are the two shapes of waitsForTheOperator, which the path view already marks on
// each row as "waiting on you", so a step this frame counts and a step that view marks are the same
// step.
func TestWhatNeedsYouCountsWhatStoppedUntilTheOperatorAnswers(t *testing.T) {
	for _, held := range []struct {
		named string
		build func(*dashboardClient)
		says  []string
	}{
		{
			named: "nothing has stopped",
			build: func(*dashboardClient) {},
			says:  []string{"nothing waits on you"},
		},
		{
			named: "one session's last exec did not land",
			build: func(client *dashboardClient) {
				client.sessions[0].Status = statusFailed
			},
			says: []string{"1 session failed        " + theFirstSession},
		},
		{
			named: "two sessions' last exec did not land",
			build: func(client *dashboardClient) {
				client.sessions[0].Status, client.sessions[1].Status = statusFailed, statusFailed
			},
			says: []string{"2 sessions failed       " + theFirstSession},
		},
		{
			// The control plane answers in whatever order it holds, and the panel draws itself again
			// every three seconds, so the line has to name the same one until that one is answered.
			named: "the sessions come back in the other order",
			build: func(client *dashboardClient) {
				client.sessions[0], client.sessions[2] = client.sessions[2], client.sessions[0]
				client.sessions[0].Status, client.sessions[2].Status = statusFailed, statusFailed
			},
			says: []string{"2 sessions failed       " + theFirstSession},
		},
		{
			named: "a session somebody stopped is not waiting on anybody",
			build: func(client *dashboardClient) {
				client.sessions[0].Status = "stopped"
			},
			says: []string{"nothing waits on you"},
		},
		{
			named: "a restatement nobody read",
			build: func(client *dashboardClient) {
				aTakenStepOf(client, 2).Restatement = "what I understood of it"
			},
			says: []string{"1 restatement to read   " + theProjectAddress + " 1.2"},
		},
		{
			named: "a restatement the operator already read",
			build: func(client *dashboardClient) {
				step := aTakenStepOf(client, 2)
				step.Restatement, step.RestatementApproved = "what I understood of it", true
			},
			says: []string{"nothing waits on you"},
		},
		{
			// Only a step somebody holds can wait. A restatement under a closed step is a record.
			named: "a restatement under a step somebody closed",
			build: func(client *dashboardClient) {
				step := aTakenStepOf(client, 2)
				step.State, step.Restatement = stepDone, "what I understood of it"
			},
			says: []string{"nothing waits on you"},
		},
		{
			named: "a check that passed with nobody's word on it",
			build: func(client *dashboardClient) {
				aTakenStepOf(client, 3).ProofState = proofPassing
			},
			says: []string{"1 check to close        " + theProjectAddress + " 1.3"},
		},
		{
			named: "a check that passed and somebody closed the step",
			build: func(client *dashboardClient) {
				step := aTakenStepOf(client, 3)
				step.ProofState, step.ClosedBy = proofPassing, "operator"
			},
			says: []string{"nothing waits on you"},
		},
		{
			// The restatement is the earlier question, so it is the one the line names.
			named: "a step waiting on both a restatement and a word",
			build: func(client *dashboardClient) {
				step := aTakenStepOf(client, 4)
				step.Restatement, step.ProofState = "what I understood of it", proofPassing
			},
			says: []string{"1 restatement to read   " + theProjectAddress + " 1.4"},
		},
		{
			named: "all three at once",
			build: func(client *dashboardClient) {
				client.sessions[0].Status, client.sessions[1].Status = statusFailed, statusFailed
				aTakenStepOf(client, 2).Restatement = "what I understood of it"
				aTakenStepOf(client, 3).ProofState = proofPassing
			},
			says: []string{
				"2 sessions failed       " + theFirstSession,
				"1 restatement to read   " + theProjectAddress + " 1.2",
				"1 check to close        " + theProjectAddress + " 1.3",
			},
		},
	} {
		t.Run(held.named, func(t *testing.T) {
			client := aSystemOf(1, 1, 3)
			held.build(client)

			got := bodyOf(drawnBy(t, Dashboard(client), ""), theFrames[0])
			if strings.Join(got, "\n") != strings.Join(held.says, "\n") {
				t.Fatalf("what needs you says\n%q\nwant\n%q", got, held.says)
			}
		})
	}
}

// A count on its own is a report, and the address is what turns it into something to do. Every
// address the frame draws is read back through the parser the command line reads an address with, so
// a line carrying an identifier, or a shortened one, fails here.
func TestEveryAddressTheFrameDrawsIsOneKreweAccepts(t *testing.T) {
	client := aSystemOf(1, 1, 3)
	client.sessions[0].Status = statusFailed
	aTakenStepOf(client, 2).Restatement = "what I understood of it"
	aTakenStepOf(client, 3).ProofState = proofPassing

	body := bodyOf(drawnBy(t, Dashboard(client), ""), theFrames[0])
	if len(body) != 3 {
		t.Fatalf("what needs you says %q, want three lines", body)
	}
	for _, line := range body {
		fields := strings.Fields(addressOn(t, line))
		parsed, err := workspace.ParsePath(fields[0])
		if err != nil {
			t.Fatalf("the line %q carries %q, which krewe does not read as an address: %v",
				line, fields[0], err)
		}
		if parsed.Workspace != "workspace-1" || parsed.Project != "project-1-1" {
			t.Errorf("the line %q addresses %q, want the workspace and the project by name", line, parsed)
		}
		// A step is named beside its project the way krewe step show takes it, and a session is the
		// third level of the address rather than a fourth word after it.
		if len(fields) == 2 && !regexp.MustCompile(`^\d+\.\d+$`).MatchString(fields[1]) {
			t.Errorf("the line %q names the step %q, want <feature>.<number>", line, fields[1])
		}
		if len(fields) > 2 {
			t.Errorf("the line %q carries %d words after the count, want the address and at most a step",
				line, len(fields))
		}
	}
	if got := addressOn(t, body[0]); got != theFirstSession {
		t.Errorf("the failed session is drawn as %q, want the handle at %q", got, theFirstSession)
	}
	// The identifiers never reach the screen, shortened or whole. A shortened identifier is what
	// every other listing prints, and pasting one into a command resolves to nothing.
	for _, identifier := range identifiersOf(client) {
		for _, line := range body {
			if strings.Contains(line, identifier[:8]) {
				t.Errorf("the line %q carries the identifier %q rather than a name", line, identifier)
			}
		}
	}
}

// identifiersOf is every identifier the system holds, for the check that none of them is drawn.
func identifiersOf(client *dashboardClient) []string {
	held := make([]string, 0, 8)
	for _, workspace := range client.workspaces {
		held = append(held, workspace.GetId())
	}
	for _, project := range client.projects {
		held = append(held, project.GetId())
	}
	for _, feature := range client.features {
		held = append(held, feature.GetId())
	}
	for _, session := range client.sessions {
		held = append(held, session.GetId())
	}
	return held
}

// addressOn is the address a line carries: everything after the gap that follows the count. The gap
// is two spaces and an address holds none, so the last one is where the count stops.
func addressOn(t *testing.T, line string) string {
	t.Helper()
	gap := strings.LastIndex(line, "  ")
	if gap < 0 {
		t.Fatalf("the line %q carries no address", line)
	}
	return strings.TrimSpace(line[gap:])
}

// Losing a read this frame counts or names with empties the frame, and every other frame is still
// drawn. A frame that guessed at the part it could not read would put a number on the screen that
// nothing behind it agrees with.
func TestLosingAReadEmptiesWhatNeedsYouAndLeavesEveryOtherFrameDrawn(t *testing.T) {
	for _, refused := range theReadsBehindWhatNeedsYou {
		t.Run(string(refused), func(t *testing.T) {
			client := aSystemOf(1, 1, 3)
			client.sessions[0].Status = statusFailed
			aTakenStepOf(client, 2).Restatement = "what I understood of it"
			client.refuses = refused

			drawn := drawnBy(t, Dashboard(client), "")
			if got := framesDrawn(drawn); strings.Join(got, ",") != strings.Join(theFrames, ",") {
				t.Fatalf("losing the %s read draws the frames %v, want all of %v", refused, got, theFrames)
			}
			if got := bodyOf(drawn, theFrames[0]); len(got) != 1 || got[0] != "" {
				t.Fatalf("losing the %s read draws %q in what needs you, want an empty body", refused, got)
			}
		})
	}
}

// The other side of that rule: a read this frame does not use costs it nothing. The spend says what
// the system has cost and never what it needs, so losing it takes the spend frame's body and no more.
func TestLosingTheSpendReadLeavesWhatNeedsYouSaying(t *testing.T) {
	client := aSystemOf(1, 1, 3)
	client.sessions[0].Status = statusFailed
	client.refuses = readSpend

	got := bodyOf(drawnBy(t, Dashboard(client), ""), theFrames[0])
	want := []string{"1 session failed        " + theFirstSession}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("losing the spend read draws %q in what needs you, want %q", got, want)
	}
}

// A line longer than the window is cut to the frame rather than running past it, and the cut takes
// the address rather than the count. At forty columns the flexible column is eighteen wide, which is
// room for the number and not for what it is about, so the operator still reads that two sessions
// want them and goes to the panel at a wider window for the rest.
func TestALineTooLongForTheWindowIsCutAndKeepsItsCount(t *testing.T) {
	client := aSystemOf(1, 1, 3)
	client.sessions[0].Status, client.sessions[1].Status = statusFailed, statusFailed

	model := newTestModel(t, Dashboard(client))
	model.width = 40
	rows, err := model.active.List(context.Background(), "")
	if err != nil {
		t.Fatalf("list dashboard: %v", err)
	}
	model, _ = update(t, model, rowsFor(model, rows...))

	for _, line := range model.bodyLines(model.visibleRows()) {
		if width := lipgloss.Width(line); width != model.innerWidth() {
			t.Fatalf("at 40 columns a panel row is %d wide and the panel is %d: %q",
				width, model.innerWidth(), line)
		}
	}
	drawn := stripped(model.View())
	if !strings.Contains(drawn, "2 sessions failed…") {
		t.Fatalf("at 40 columns the panel does not say what needs the operator, cut:\n%s", drawn)
	}
	if strings.Contains(drawn, theFirstSession) {
		t.Fatalf("at 40 columns the whole address is drawn, so the row runs past the frame:\n%s", drawn)
	}
}

// What the in flight frame says, case by case. Two counts, each with the address of the first thing
// behind it, and one line rather than two zeros on a system where nothing is moving.
//
// The sessions are counted by the colour the sessions listing already gives their status, so a
// session this frame counts and a session that listing draws as busy are the same session.
func TestWhatIsInFlightCountsWhatTheSystemIsWorkingOnNow(t *testing.T) {
	for _, held := range []struct {
		named string
		build func(*dashboardClient)
		says  []string
	}{
		{
			// The system aSystemOf builds: one step somebody holds, and every session waiting.
			named: "a step somebody holds and no exec under way",
			build: func(*dashboardClient) {},
			says:  []string{"1 step in flight        " + theProjectAddress + " 1.1"},
		},
		{
			named: "nothing is moving at all",
			build: func(client *dashboardClient) {
				client.steps[0].State = stepReady
			},
			says: []string{nothingIsInFlight},
		},
		{
			named: "one session has an exec under way",
			build: func(client *dashboardClient) {
				client.sessions[0].Status = "running"
			},
			says: []string{
				"1 session running       " + theFirstSession,
				"1 step in flight        " + theProjectAddress + " 1.1",
			},
		},
		{
			// The other word the sessions listing draws as busy. A session working under it is the
			// system working, and counting only the first word would leave it off the panel.
			named: "a working session counts beside a running one",
			build: func(client *dashboardClient) {
				client.sessions[0].Status, client.sessions[1].Status = "running", "dispatching"
			},
			says: []string{
				"2 sessions running      " + theFirstSession,
				"1 step in flight        " + theProjectAddress + " 1.1",
			},
		},
		{
			named: "a session waiting for work is not in flight",
			build: func(client *dashboardClient) {
				client.sessions[0].Status = "idle"
			},
			says: []string{"1 step in flight        " + theProjectAddress + " 1.1"},
		},
		{
			named: "a session somebody stopped is not in flight",
			build: func(client *dashboardClient) {
				client.sessions[0].Status = "stopped"
			},
			says: []string{"1 step in flight        " + theProjectAddress + " 1.1"},
		},
		{
			named: "a session whose last exec did not land is not in flight",
			build: func(client *dashboardClient) {
				client.sessions[0].Status = statusFailed
			},
			says: []string{"1 step in flight        " + theProjectAddress + " 1.1"},
		},
		{
			// The control plane answers in whatever order it holds, and the panel draws itself again
			// every three seconds, so the line has to name the same session until that one lands.
			named: "the sessions come back in the other order",
			build: func(client *dashboardClient) {
				client.sessions[0], client.sessions[2] = client.sessions[2], client.sessions[0]
				client.sessions[0].Status, client.sessions[2].Status = "running", "running"
			},
			says: []string{
				"2 sessions running      " + theFirstSession,
				"1 step in flight        " + theProjectAddress + " 1.1",
			},
		},
		{
			named: "two steps somebody holds",
			build: func(client *dashboardClient) {
				aTakenStepOf(client, 2)
			},
			says: []string{"2 steps in flight       " + theProjectAddress + " 1.1"},
		},
		{
			named: "a step somebody closed is not in flight",
			build: func(client *dashboardClient) {
				client.steps[0].State = stepDone
			},
			says: []string{nothingIsInFlight},
		},
		{
			named: "a step nobody has taken is not in flight",
			build: func(client *dashboardClient) {
				aTakenStepOf(client, 2).State = stepReady
			},
			says: []string{"1 step in flight        " + theProjectAddress + " 1.1"},
		},
		{
			// Zero of a cap is a project nobody has started, and a line about it would read as a
			// project that stalled. The frame says nothing at all about one instead.
			named: "a project with no path draws nothing rather than a count of zero",
			build: func(client *dashboardClient) {
				client.features, client.steps = nil, nil
			},
			says: []string{nothingIsInFlight},
		},
		{
			named: "a project with no path beside one with a path",
			build: func(client *dashboardClient) {
				client.projects = append(client.projects, &quaycrewv1.Project{
					Id:        fmt.Sprintf("%012d%012d", 1, 2),
					Workspace: client.workspaces[0].GetId(),
					Name:      "project-nobody-has-started",
				})
			},
			says: []string{"1 step in flight        " + theProjectAddress + " 1.1"},
		},
	} {
		t.Run(held.named, func(t *testing.T) {
			client := aSystemOf(1, 1, 3)
			held.build(client)

			got := bodyOf(drawnBy(t, Dashboard(client), ""), theFrames[1])
			if strings.Join(got, "\n") != strings.Join(held.says, "\n") {
				t.Fatalf("in flight says\n%q\nwant\n%q", got, held.says)
			}
		})
	}
}

// A project at its cap is drawn the way the projects listing draws one: plainly, with no colour and
// no mark. The refusal is what the operator reads when they take the next step, and a panel that
// shouted about a project with work in it would be shouting about the normal state of one.
//
// Ten is the cap a project nobody set one on carries, so ten steps in flight is a project at it.
func TestAProjectAtItsCapIsDrawnPlainly(t *testing.T) {
	client := aSystemOf(1, 1, 1)
	for number := int32(2); number <= 10; number++ {
		aTakenStepOf(client, number)
	}

	rows, err := Dashboard(client).List(context.Background(), "")
	if err != nil {
		t.Fatalf("the panel refused to draw: %v", err)
	}
	said := 0
	for _, row := range rows {
		if !strings.HasPrefix(row.ID, "in-flight:body:") {
			continue
		}
		said++
		if want := "10 steps in flight      " + theProjectAddress + " 1.1"; row.Cells[saysCell] != want {
			t.Errorf("the frame says %q, want %q", row.Cells[saysCell], want)
		}
		if row.State != StateUnknown {
			t.Errorf("the line %q is drawn in state %v, want no colour and no claim",
				row.Cells[saysCell], row.State)
		}
	}
	if said != 1 {
		t.Fatalf("the in flight frame drew %d body rows, want one", said)
	}
}

// A count on its own is a report, and the address is what turns it into something to act on. Every
// address the frame draws is read back through the parser the command line reads an address with, so
// a line carrying an identifier, or a shortened one, fails here.
func TestEveryAddressTheInFlightFrameDrawsIsOneKreweAccepts(t *testing.T) {
	client := aSystemOf(1, 1, 3)
	client.sessions[0].Status = "running"

	body := bodyOf(drawnBy(t, Dashboard(client), ""), theFrames[1])
	if len(body) != 2 {
		t.Fatalf("in flight says %q, want two lines", body)
	}
	for _, line := range body {
		fields := strings.Fields(addressOn(t, line))
		parsed, err := workspace.ParsePath(fields[0])
		if err != nil {
			t.Fatalf("the line %q carries %q, which krewe does not read as an address: %v",
				line, fields[0], err)
		}
		if parsed.Workspace != "workspace-1" || parsed.Project != "project-1-1" {
			t.Errorf("the line %q addresses %q, want the workspace and the project by name", line, parsed)
		}
	}
	if got := addressOn(t, body[0]); got != theFirstSession {
		t.Errorf("the running session is drawn as %q, want the handle at %q", got, theFirstSession)
	}
	for _, identifier := range identifiersOf(client) {
		for _, line := range body {
			if strings.Contains(line, identifier[:8]) {
				t.Errorf("the line %q carries the identifier %q rather than a name", line, identifier)
			}
		}
	}
}

// Losing a read this frame counts or names with empties the frame, and every other frame is still
// drawn. A frame that guessed at the part it could not read would put a number on the screen that
// nothing behind it agrees with.
func TestLosingAReadEmptiesWhatIsInFlightAndLeavesEveryOtherFrameDrawn(t *testing.T) {
	for _, refused := range theReadsBehindWhatIsInFlight {
		t.Run(string(refused), func(t *testing.T) {
			client := aSystemOf(1, 1, 3)
			client.sessions[0].Status = "running"
			client.refuses = refused

			drawn := drawnBy(t, Dashboard(client), "")
			if got := framesDrawn(drawn); strings.Join(got, ",") != strings.Join(theFrames, ",") {
				t.Fatalf("losing the %s read draws the frames %v, want all of %v", refused, got, theFrames)
			}
			if got := bodyOf(drawn, theFrames[1]); len(got) != 1 || got[0] != "" {
				t.Fatalf("losing the %s read draws %q in flight, want an empty body", refused, got)
			}
		})
	}
}

// The other side of that rule: a read this frame does not use costs it nothing. The spend says what
// the system has cost and never what it is doing.
func TestLosingTheSpendReadLeavesWhatIsInFlightSaying(t *testing.T) {
	client := aSystemOf(1, 1, 3)
	client.sessions[0].Status = "running"
	client.refuses = readSpend

	got := bodyOf(drawnBy(t, Dashboard(client), ""), theFrames[1])
	want := []string{
		"1 session running       " + theFirstSession,
		"1 step in flight        " + theProjectAddress + " 1.1",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("losing the spend read draws %q in flight, want %q", got, want)
	}
}

// The draw costs what it cost before this frame had a body, on a system where the frame has both of
// its counts to say. The panel reads the system once and hands that one reading to every frame, so a
// frame that answered from a call of its own would show up here as a seventh call.
func TestAFrameWithBothCountsStillCostsTheSixReadings(t *testing.T) {
	client := aSystemOf(3, 2, 2)
	for _, session := range client.sessions {
		session.Status = "running"
	}

	if _, err := Dashboard(client).List(context.Background(), ""); err != nil {
		t.Fatalf("the panel refused to draw: %v", err)
	}
	made := 0
	for _, times := range client.counted {
		made += times
	}
	if made != len(everyCallThePanelMakes) {
		t.Fatalf("one draw made %d calls, want %d", made, len(everyCallThePanelMakes))
	}
}
