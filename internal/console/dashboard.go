package console

import (
	"context"
	"fmt"
	"slices"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/workspace"
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
	{id: "needs-you", title: "what needs you", body: whatNeedsYou},
	{id: "in-flight", title: "in flight"},
	{id: "the-path", title: "the path"},
	{id: "trust", title: "trust"},
	{id: "spend", title: "spend"},
	{id: "the-pile", title: "the pile"},
}

type widget struct {
	id    string
	title string
	// body is what this frame says, one line per row, worked out from the one reading the panel made.
	// A frame without one draws an empty body, which is every frame the steps after this one fill in.
	body func(panelReading) []string
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
		rows = append(rows, drawn.rows(read)...)
	}
	return rows
}

// rows is one frame: its title, and its body under it. An empty body is a blank row rather than no
// row, so the frame keeps its shape and the hole is where the number will be.
func (w widget) rows(read panelReading) []Row {
	said := []string{""}
	if w.body != nil {
		if lines := w.body(read); len(lines) > 0 {
			said = lines
		}
	}

	rows := make([]Row, 0, len(said)+1)
	rows = append(rows, Row{
		ID:    w.id,
		Label: w.title,
		Cells: []string{w.title, ""},
		State: StateReady,
	})
	for at, line := range said {
		rows = append(rows, Row{
			ID: fmt.Sprintf("%s:body:%d", w.id, at),
			// Named after its frame, so anything naming this row names the frame rather than an
			// identifier nobody typed.
			Label: w.title,
			// The widget column stays empty on a body row. It is the column that holds the frame
			// titles, and a word in it here would read as a seventh frame.
			Cells: []string{"", line},
		})
	}
	return rows
}

const (
	// statusFailed is the word a session's row carries when its last exec did not land. It is the one
	// status that has stopped and wants somebody: idle is where every session rests, running is under
	// way, and stopped and reclaimed were somebody's decision.
	statusFailed = "failed"
	// nothingWaits is the whole body of the frame on a system with none of the three. Three counts of
	// zero in a column read as three problems at a glance, which is the opposite of what this frame
	// is for.
	nothingWaits = "nothing waits on you"
	// theNounColumn is the room the count and its noun get before the address starts, so the three
	// lines put their addresses under each other.
	theNounColumn = 22
)

// theReadsBehindWhatNeedsYou is every read this frame uses: two to count with, and three to turn the
// identifiers the counts came from into addresses. Losing any of them empties the frame, because a
// count whose address the panel cannot name is a number nobody can act on, and acting is the whole
// job of this frame. The spend is not here: it says what the system cost and never what it needs.
var theReadsBehindWhatNeedsYou = []panelRead{
	readWorkspaces, readProjects, readSessions, readFeatures, readSteps,
}

// whatNeedsYou is the frame that makes the operator act, so it is first and it sits at the top of the
// panel. Every other frame on this panel is a report.
//
// Three lines, each one a count of things that have stopped until somebody answers, and the address
// of the first of them. An address beside a count is what makes a line actionable: a count on its own
// says a number and leaves the operator to go and find what it is about.
//
// The two step lines are the two shapes of waitsForTheOperator, which the path view already draws as
// "waiting on you" on each row. This frame is that mark counted across every project, so the panel
// and the path can never disagree about what waits.
func whatNeedsYou(read panelReading) []string {
	for _, needed := range theReadsBehindWhatNeedsYou {
		if !read.answered(needed) {
			return nil
		}
	}
	book := addressesIn(read)

	var failed, restatements, toClose waiting
	for _, session := range read.sessions {
		if session.GetStatus() == statusFailed {
			failed.add(book.session(session))
		}
	}
	for _, step := range read.steps {
		switch {
		case waitsOnARestatement(step):
			restatements.add(book.step(step))
		case waitsOnAWordToClose(step):
			toClose.add(book.step(step))
		}
	}

	counted := []struct {
		one, many string
		held      waiting
	}{
		{"session failed", "sessions failed", failed},
		{"restatement to read", "restatements to read", restatements},
		{"check to close", "checks to close", toClose},
	}
	lines := make([]string, 0, len(counted))
	for _, count := range counted {
		if line := count.held.line(count.one, count.many); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return []string{nothingWaits}
	}
	return lines
}

// waitsOnARestatement is a step whose session wrote what it understood and stopped, with nobody's
// word on the text yet.
func waitsOnARestatement(step *quaycrewv1.Step) bool {
	return waitsForTheOperator(step) &&
		step.GetRestatement() != "" && !step.GetRestatementApproved()
}

// waitsOnAWordToClose is the other shape waitsForTheOperator knows: krewe ran the step's scenario, it
// passed, and nobody has said the word that finishes the step.
//
// A step that is both is counted as a restatement. The restatement is the earlier question, and
// answering it is what the operator does next.
func waitsOnAWordToClose(step *quaycrewv1.Step) bool {
	return waitsForTheOperator(step) && !waitsOnARestatement(step)
}

// waiting is the addresses of one kind of thing that has stopped until the operator answers.
type waiting []string

// add keeps an address, and drops what has none. A line carries a count and something to type, so a
// thing the panel cannot name would put a number on the screen with nothing behind it.
func (w *waiting) add(address string) {
	if address == "" {
		return
	}
	*w = append(*w, address)
}

// line is the count, what they are, and the address of the first of them. It is empty where none
// wait, so a frame draws a line only about something that is there.
//
// The addresses are sorted, so a panel that draws itself again every three seconds names the same one
// until that one is answered. The count comes first because the flexible column is cut from the right
// on a narrow window: the number survives the cut and the address is what is lost.
func (w waiting) line(one, many string) string {
	if len(w) == 0 {
		return ""
	}
	sorted := slices.Clone([]string(w))
	slices.Sort(sorted)

	noun := many
	if len(sorted) == 1 {
		noun = one
	}
	return fmt.Sprintf("%-*s  %s", theNounColumn, fmt.Sprintf("%d %s", len(sorted), noun), sorted[0])
}

// addresses turns the identifiers the wire carries into the addresses a person types. Everywhere else
// the console prints a session's identifier shortened, and a shortened identifier pasted into a
// command resolves to nothing.
type addresses struct {
	workspaces map[string]string
	projects   map[string]*quaycrewv1.Project
	features   map[string]*quaycrewv1.Feature
}

// addressesIn indexes the reading by identifier. It is built once per draw and read many times,
// which is the same rule the reading itself follows.
func addressesIn(read panelReading) addresses {
	book := addresses{
		workspaces: make(map[string]string, len(read.workspaces)),
		projects:   make(map[string]*quaycrewv1.Project, len(read.projects)),
		features:   make(map[string]*quaycrewv1.Feature, len(read.features)),
	}
	for _, held := range read.workspaces {
		book.workspaces[held.GetId()] = held.GetName()
	}
	for _, held := range read.projects {
		book.projects[held.GetId()] = held
	}
	for _, held := range read.features {
		book.features[held.GetId()] = held
	}
	return book
}

// named is a name, falling back to the identifier it belongs to. Every level of an address is read as
// a name or as an identifier, so a name the panel could not find still leaves an address that
// resolves.
func named(name, id string) string {
	if name != "" {
		return name
	}
	return id
}

// project is <workspace>/<project>, the address every command about a project takes. It is empty for
// a project the reading does not hold.
func (a addresses) project(id string) string {
	project, found := a.projects[id]
	if !found {
		return ""
	}
	return named(a.workspaces[project.GetWorkspace()], project.GetWorkspace()) +
		workspace.Separator + named(project.GetName(), id)
}

// session is <workspace>/<project>/<handle>, which is what krewe attach takes. The handle rather than
// the identifier: the handle is the name a dispatch already uses, and it is what the operator typed.
func (a addresses) session(session *quaycrewv1.Session) string {
	project := a.project(session.GetProject())
	if project == "" {
		return ""
	}
	return project + workspace.Separator + named(session.GetHandle(), session.GetId())
}

// step is the project address and the step beside it, written <feature>.<number>, which is how krewe
// step show takes a step. A step carries its feature on the wire and the feature carries the project,
// so both reads have to have answered for this to say anything.
func (a addresses) step(step *quaycrewv1.Step) string {
	feature, found := a.features[step.GetFeature()]
	if !found {
		return ""
	}
	project := a.project(feature.GetProject())
	if project == "" {
		return ""
	}
	return fmt.Sprintf("%s %d.%d", project, feature.GetNumber(), step.GetNumber())
}
