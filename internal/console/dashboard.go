package console

import (
	"context"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
)

// Dashboard is the panel the console opens on: one view, scoped to nothing, that says what the whole
// system needs. Every other view answers about the rows under one thing, so finding the session
// waiting on an answer meant drilling into each workspace and each project in turn.
//
// A frame is a title row with its body under it, and the frames are drawn one after another, so the
// frame a row belongs to is the title above it. Every body is empty until the step that fills it in.
//
// Two rules decide whether this is still usable on a system of 465 sessions. One reading per draw,
// shared by every frame, and no call made once per workspace, project or session: the console draws
// itself again every three seconds, so a call per project is forty calls every three seconds on a
// system of forty. And a call that fails takes its own frame's body and nothing else, because a grid
// with a hole in it says more than a blank screen.
func Dashboard(client quaycrewv1.ControlPlaneServiceClient) Resource {
	return Resource{
		Name: "dashboard",
		// The letter and the word an operator reaches for to get back to the top. Neither was taken:
		// the views that hold single letters hold w, p, s and c. The registry refuses a spelling two
		// views share, so a clash here would be a console that will not open at all.
		Aliases: []string{"d", "home"},
		Columns: []Column{
			{Title: "widget", Width: 18, Colour: colourOfName},
			// The column that flexes. A widget line is a sentence, and a sentence cut to a fixed
			// width stops being one.
			{Title: "says", Width: 0},
		},
		// No order of its own. The frames are drawn in the order they are declared, which is what puts
		// the one that makes the operator act at the top, and an order over rendered text would sort
		// the titles into the alphabet instead.
		SortBy: -1,
		List: func(ctx context.Context, _ string) ([]Row, error) {
			return panelRows(readPanel(ctx, client)), nil
		},
	}
}

// panelWidgets are the frames, in the order they are drawn. The one that makes the operator act is
// first, where the eye goes.
var panelWidgets = []widget{
	{id: "needs-you", title: "what needs you"},
	{id: "in-flight", title: "in flight"},
	{id: "the-path", title: "the path"},
	{id: "trust", title: "trust"},
	{id: "spend", title: "spend"},
	{id: "the-pile", title: "the pile"},
}

type widget struct {
	id    string
	title string
}

// panelRead names one of the calls the panel makes, so the reading can say which of them answered.
type panelRead string

const (
	readWorkspaces panelRead = "workspaces"
	readProjects   panelRead = "projects"
	readSessions   panelRead = "sessions"
	readFeatures   panelRead = "features"
	readSteps      panelRead = "steps"
	readSpend      panelRead = "spend"
)

// panelReading is everything the panel read on one draw, and which of the reads failed. It is the
// whole of the panel's cost: each frame is built from what is in here and holds no client of its
// own, because a frame that made its own call is a panel that reads the system six times over.
type panelReading struct {
	workspaces []*quaycrewv1.Workspace
	projects   []*quaycrewv1.Project
	sessions   []*quaycrewv1.Session
	features   []*quaycrewv1.Feature
	steps      []*quaycrewv1.Step
	spent      *quaycrewv1.GetUsageResponse

	// failed is the reads that did not answer, by name, so a frame draws an empty body while the rest
	// of the panel stays on screen.
	failed map[panelRead]bool
}

// answered says whether one of the reads came back.
func (r panelReading) answered(read panelRead) bool {
	return !r.failed[read]
}

// readPanel makes the panel's calls, once each, and swallows the failures.
//
// Six calls, whatever the system holds and whatever the panel draws. Each one answers for the whole
// system rather than for a named workspace, project or session, so there is nothing here to repeat
// per row. A failure is recorded rather than returned: an operator who has lost one number still has
// five frames worth reading, which is how the header already treats a failed usage call.
func readPanel(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient) panelReading {
	read := panelReading{failed: map[panelRead]bool{}}

	if listed, err := client.ListWorkspaces(ctx, &quaycrewv1.ListWorkspacesRequest{}); err == nil {
		read.workspaces = listed.GetWorkspaces()
	} else {
		read.failed[readWorkspaces] = true
	}
	if listed, err := client.ListProjects(ctx, &quaycrewv1.ListProjectsRequest{}); err == nil {
		read.projects = listed.GetProjects()
	} else {
		read.failed[readProjects] = true
	}
	if listed, err := client.ListSessions(ctx, &quaycrewv1.ListSessionsRequest{}); err == nil {
		read.sessions = listed.GetSessions()
	} else {
		read.failed[readSessions] = true
	}
	// The features are read because a step carries its feature on the wire and not its project, so
	// they are how a step is put under the project whose path it belongs to.
	if listed, err := client.ListFeatures(ctx, &quaycrewv1.ListFeaturesRequest{}); err == nil {
		read.features = listed.GetFeatures()
	} else {
		read.failed[readFeatures] = true
	}
	if listed, err := client.ListSteps(ctx, &quaycrewv1.ListStepsRequest{}); err == nil {
		read.steps = listed.GetSteps()
	} else {
		read.failed[readSteps] = true
	}
	if spent, err := client.GetUsage(ctx, &quaycrewv1.GetUsageRequest{}); err == nil {
		read.spent = spent
	} else {
		read.failed[readSpend] = true
	}
	return read
}

// panelRows is the grid: every frame, in the order they are declared.
//
// A system with nothing in it draws no panel at all. Six frames have nothing to say about a system
// with no workspace, and an empty listing is what offers the operator the guided setup, which is a
// better first screen than a grid of empty frames.
func panelRows(read panelReading) []Row {
	if read.answered(readWorkspaces) && len(read.workspaces) == 0 {
		return nil
	}
	rows := make([]Row, 0, len(panelWidgets)*2)
	for _, drawn := range panelWidgets {
		rows = append(rows, drawn.rows()...)
	}
	return rows
}

// rows is one frame: its title, and its body under it. An empty body is a blank row rather than no
// row, so the frame keeps its shape and the hole is where the number will be.
func (w widget) rows() []Row {
	title := Row{
		ID:    w.id,
		Label: w.title,
		Cells: []string{w.title, ""},
		State: StateReady,
	}
	body := Row{
		ID: w.id + ":body",
		// Named after its frame, so anything naming this row names the frame rather than an
		// identifier nobody typed.
		Label: w.title,
		Cells: []string{"", ""},
	}
	return []Row{title, body}
}
