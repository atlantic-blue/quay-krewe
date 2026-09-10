package controlplane

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/contextsize"
	"github.com/atlantic-blue/quay-krewe/internal/display"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// What a project is for, and what was designed for it. Four calls, beside the context calls in
// server.go, because a design is the same kind of thing: text the operator writes about a project,
// kept by the system rather than in a file somebody has to find.
//
// A design is a separate table from a project because a body is the largest text in the system and
// every project listing reads the project row.

// briefMark is the length past which a brief is long enough to say so. A brief is one paragraph
// naming what the project is for, and a page of prose in that field is a design in the wrong column.
//
// bodyMark is the same for the design body. It is far larger because a design is expected to be
// long, and the number is there to catch a whole repository pasted in.
//
// Neither refuses anything. The text is kept whole either way, and the caller is told the length so
// a person decides. Refusing here would lose work that only exists in the call being made.
const (
	briefMark = 2_000
	bodyMark  = 100_000
)

// GetDesign returns what a project is for and what was designed for it.
//
// A project with no design row answers with an empty design rather than an error, because nothing
// written is the normal state. Reading records nothing.
func (s *Server) GetDesign(ctx context.Context, req *quaycrewv1.GetDesignRequest) (*quaycrewv1.GetDesignResponse, error) {
	if req.GetProject() == "" {
		return nil, status.Error(codes.InvalidArgument, "which project: say where with an address")
	}
	design, err := s.store.GetDesign(ctx, req.GetProject())
	if err != nil {
		return nil, storeError(err, "project")
	}
	return &quaycrewv1.GetDesignResponse{Design: design}, nil
}

// SetBrief records what a project is for. The brief is one paragraph, and it is kept whole however
// long it is.
func (s *Server) SetBrief(ctx context.Context, req *quaycrewv1.SetBriefRequest) (*quaycrewv1.SetBriefResponse, error) {
	if req.GetProject() == "" {
		return nil, status.Error(codes.InvalidArgument, "which project: say where with an address")
	}
	design, err := s.store.SetProjectBrief(ctx, req.GetProject(), req.GetBrief())
	if err != nil {
		return nil, storeError(err, "project")
	}
	s.renderDesignTo(ctx, req.GetProject())
	return &quaycrewv1.SetBriefResponse{
		Design:   design,
		Warnings: overMark("brief", utf8.RuneCountInString(req.GetBrief()), briefMark, "one paragraph saying what the project is for"),
	}, nil
}

// SetDesign records the design document whole, and who wrote it.
//
// written_by is what the caller claimed and the system does not check it. It grants nothing, so a
// wrong one costs a wrong name in a record rather than a capability.
func (s *Server) SetDesign(ctx context.Context, req *quaycrewv1.SetDesignRequest) (*quaycrewv1.SetDesignResponse, error) {
	if req.GetProject() == "" {
		return nil, status.Error(codes.InvalidArgument, "which project: say where with an address")
	}
	design, err := s.store.SetProjectDesign(ctx, req.GetProject(), req.GetBody(), req.GetWrittenBy())
	if err != nil {
		return nil, storeError(err, "project")
	}
	s.renderDesignTo(ctx, req.GetProject())
	return &quaycrewv1.SetDesignResponse{
		Design:   design,
		Warnings: overMark("design", utf8.RuneCountInString(req.GetBody()), bodyMark, "the design document"),
	}, nil
}

// SetContracts records the contracts a project builds against, whole.
//
// It is a second body on the design row and the approval is not touched, which is the whole rule of
// this call. SetDesign clears the approval because approval is a statement about one text; the
// contracts document is read out of the design the operator already approved, so writing one is not
// a design that changed. The command line says so on every write, because an operator who thinks a
// contracts write undid their approval approves a design nobody rewrote.
//
// Nothing parses the body. A contract a step names is never checked against it, and the deferred
// list in the design records that.
//
// written_by is what the caller claimed, exactly as SetDesign takes it, and it grants nothing.
func (s *Server) SetContracts(ctx context.Context, req *quaycrewv1.SetContractsRequest) (*quaycrewv1.SetContractsResponse, error) {
	if req.GetProject() == "" {
		return nil, status.Error(codes.InvalidArgument, "which project: say where with an address")
	}
	design, err := s.store.SetProjectContracts(ctx, req.GetProject(), req.GetBody(), req.GetWrittenBy())
	if err != nil {
		return nil, storeError(err, "project")
	}
	s.renderDesignTo(ctx, req.GetProject())
	return &quaycrewv1.SetContractsResponse{Design: design}, nil
}

// ApproveDesign records the operator's word on the design as it stands.
//
// It approves the text that is in the store now, and it asks nothing: a call that opened an editor
// or a question would be approving a text nobody named. A design with no body is refused, because
// there is nothing to agree to.
//
// DeniedToDriver refuses this call to a session, so the word reaches the store only through the
// operator's own command. That is the boundary that makes the gate real: a session that could
// approve its own design would be agreeing with itself.
func (s *Server) ApproveDesign(ctx context.Context, req *quaycrewv1.ApproveDesignRequest) (*quaycrewv1.ApproveDesignResponse, error) {
	if req.GetProject() == "" {
		return nil, status.Error(codes.InvalidArgument, "which project: say where with an address")
	}
	design, err := s.store.ApproveProjectDesign(ctx, req.GetProject())
	if errors.Is(err, store.ErrNothingToApprove) {
		return nil, status.Error(codes.FailedPrecondition,
			"this project has no design to approve: write one with krewe design set [<address>] --file <path>")
	}
	if err != nil {
		return nil, storeError(err, "project")
	}
	s.renderDesignTo(ctx, req.GetProject())
	return &quaycrewv1.ApproveDesignResponse{Design: design}, nil
}

// stepsInFlightBounds are the smallest and largest cap a person may set.
//
// A cap of zero would refuse every take, which is a project nobody can work on rather than a project
// that runs nothing. Twenty is more sessions than one operator reads, and a number typed by accident
// is far more likely than a machine that genuinely wants a hundred.
const (
	leastStepsInFlight = 1
	mostStepsInFlight  = 20
)

// SetStepsInFlightCap records how many steps of one project may be in state taken at one time.
//
// The bounds are checked here rather than in the store, the way the two words a step ends with are:
// one layer owns what a person may type, and the store keeps what it is given.
//
// It is one number on the project, counting across every feature. There is no second cap per feature
// and none is added: nothing can collide across features, because the take reads the whole project,
// so one number protects the machine and the operator's reading. A per feature cap would let a
// project with five features run five times its number while every number written down still read
// the same.
//
// DeniedToDriver refuses this call to a session. The cap is how much the operator reads at once, so a
// session that could raise its own would widen the fan out without anybody asking for it.
func (s *Server) SetStepsInFlightCap(ctx context.Context, req *quaycrewv1.SetStepsInFlightCapRequest) (
	*quaycrewv1.SetStepsInFlightCapResponse, error) {
	if req.GetProject() == "" {
		return nil, status.Error(codes.InvalidArgument, "which project: say where with an address")
	}
	if req.GetStepsInFlightCap() < leastStepsInFlight {
		return nil, status.Errorf(codes.InvalidArgument,
			"a cap of zero would refuse every take: say %d or more", leastStepsInFlight)
	}
	if req.GetStepsInFlightCap() > mostStepsInFlight {
		return nil, status.Errorf(codes.InvalidArgument,
			"a cap above %d is more sessions than one operator reads", mostStepsInFlight)
	}
	design, err := s.store.SetStepsInFlightCap(ctx, req.GetProject(), req.GetStepsInFlightCap())
	if err != nil {
		return nil, storeError(err, "project")
	}
	return &quaycrewv1.SetStepsInFlightCapResponse{Design: design}, nil
}

// overMark says how long the text is when it is past the mark, and says nothing at all when it is
// not. It is a warning and never a refusal: the text is already kept.
func overMark(what string, length, mark int, expected string) []string {
	if length <= mark {
		return nil
	}
	return []string{fmt.Sprintf("the %s is %s, over the %s mark. It is kept whole. A %s is %s.",
		what, contextsize.Characters(length), contextsize.Characters(mark), what, expected)}
}

// What the session working in a project is told about that project's design, and where the design
// itself is put so the model can open it.
//
// The summary is a section in the inner memory file, which every exec of every session in the
// project reads. That is what the cap below is for: the whole section is read again on every exec,
// so its cost is paid per exec rather than once.
const (
	// designSectionCap is the whole section, in characters. The design body is not in it: the
	// section names a file, and the file is opened by a model that decides it needs it.
	designSectionCap = 400
	// designBriefCap is how much of the brief reaches the section. The store keeps it whole.
	designBriefCap = 200
	// designDir and designFile are where the design body goes in the session's working directory. A
	// dot directory, because a repository cloned into the working directory may hold a file called
	// design.md of its own.
	designDir  = ".krewe"
	designFile = "design.md"
	// pathFile is the path document, beside the design in the same dot directory and for the same
	// reason.
	pathFile = "path.md"
	// contractsFile is the contracts document, beside the design and written the same way. It is a
	// file rather than a section in the memory file because that file is read on every exec of every
	// session in the project and a contracts document is long: this repository carries 127 contracts
	// across 44 slices. The take text names it, and a model that needs it opens it.
	contractsFile = "contracts.md"
)

// renderDesign puts the design body in the session's working directory and returns the summary that
// goes in its memory file. It returns the empty string when there is nothing to say, and Compose
// drops an empty section.
//
// The file and the summary are written together, for the reason renderSkills gives: a line telling
// the model to read a file that is not there sends it to open nothing.
//
// Nothing here fails an exec. A session with no design summary is a session that reads the project's
// context and gets on with it, which is what every session did before this existed.
func (s *Server) renderDesign(ctx context.Context, session *quaycrewv1.Session, dir string, hasPath bool) string {
	design, err := s.store.GetDesign(ctx, session.GetProject())
	if err != nil {
		return ""
	}
	if design.GetBrief() == "" && design.GetBody() == "" {
		s.clearSessionFile(dir, designFile, "design")
		return ""
	}
	project, err := s.store.GetProject(ctx, session.GetProject())
	if err != nil {
		return ""
	}
	hasBody := s.writeSessionFile(dir, designFile, "design", design.GetBody())
	on, inThePath := s.stepThisSessionHolds(ctx, session)
	return designSummary(project.GetName(), design, hasBody, hasPath, on, inThePath)
}

// renderContracts puts the project's contracts document where the model can open it.
//
// It reads the design row for itself rather than being a line inside renderDesign, because that
// render returns early for a project carrying neither a brief nor a design body, and a project can
// carry a contracts document before it carries either.
//
// A project with an empty contracts body gets no file. An empty body is a value rather than an
// absence: it is how a project says it carries no contracts document.
//
// Nothing here fails an exec, for the same reason renderDesign fails none.
func (s *Server) renderContracts(ctx context.Context, project, dir string) {
	design, err := s.store.GetDesign(ctx, project)
	if err != nil {
		return
	}
	s.writeSessionFile(dir, contractsFile, "contracts", design.GetContracts())
}

// stepThisSessionHolds is the step this session took and how many steps that step's path has, and
// nil for a session that took none. Most sessions took none: an ordinary exec into a project is not
// a step.
//
// The two reads are at two levels and that is the point. A session works in a project and its step
// may sit in any feature of it, so the search for it is across the whole project. The count beside
// it is the step's own feature's path, because a path belongs to a feature and "step 3 of 5" is a
// sentence about one path.
//
// The path is read again here rather than handed down from the render beside it, because the summary
// and the path document are written by two calls and neither owns the other's read.
func (s *Server) stepThisSessionHolds(ctx context.Context, session *quaycrewv1.Session) (*quaycrewv1.Step, int) {
	steps := s.stepsOfProject(ctx, session.GetProject())
	for _, step := range steps {
		if step.GetState() != store.StepTaken {
			continue
		}
		if step.GetSession() == session.GetHandle() || step.GetSession() == session.GetId() {
			return step, inTheSamePath(steps, step.GetFeature())
		}
	}
	return nil, 0
}

// stepsOfProject is every step of every feature of one project, in feature and then number order.
//
// A session works in a project rather than in a feature, so which step it holds is a question about
// the whole project. The read goes through the project's features and never through one of them: a
// read narrowed to a single feature is the query the new key invites and it is the wrong one, since
// it cannot see the work the project's other features are doing.
func (s *Server) stepsOfProject(ctx context.Context, project string) []*quaycrewv1.Step {
	features, err := s.store.ListFeatures(ctx, project)
	if err != nil {
		return nil
	}
	every := make([]*quaycrewv1.Step, 0)
	for _, feature := range features {
		steps, err := s.store.ListSteps(ctx, feature.GetId())
		if err != nil {
			continue
		}
		every = append(every, steps...)
	}
	return every
}

// inTheSamePath counts the steps of one feature out of a project wide listing.
func inTheSamePath(steps []*quaycrewv1.Step, feature string) int {
	held := 0
	for _, step := range steps {
		if step.GetFeature() == feature {
			held++
		}
	}
	return held
}

// writeSessionFile puts a rendered document where the model can open it, and says whether it is
// there to open. An empty document has no file: a file that exists and says nothing costs a read.
func (s *Server) writeSessionFile(dir, name, what, body string) bool {
	if body == "" {
		s.clearSessionFile(dir, name, what)
		return false
	}
	at := filepath.Join(dir, designDir)
	if err := os.MkdirAll(at, 0o755); err != nil {
		slog.Warn("the "+what+" was not written where the session can read it", "at", at, "error", err)
		return false
	}
	if err := os.WriteFile(filepath.Join(at, name), []byte(body), 0o644); err != nil {
		slog.Warn("the "+what+" was not written where the session can read it", "at", at, "error", err)
		return false
	}
	return true
}

// clearSessionFile takes the file away when the store holds nothing, so what the session reads and
// what the store holds cannot disagree. A design or a path emptied on purpose would otherwise stay
// readable for the life of the working directory.
func (s *Server) clearSessionFile(dir, name, what string) {
	if err := os.Remove(filepath.Join(dir, designDir, name)); err != nil && !os.IsNotExist(err) {
		slog.Warn("the old "+what+" was left where the session can read it", "dir", dir, "error", err)
	}
}

// designSummary is the section itself.
//
// The approval line is there whenever there is a design to approve, because a session that reads a
// design has to know whether anybody agreed to it. A project holding a brief and no design has
// nothing to say about approval, so it says nothing rather than spending a line of a capped section
// on a word about a document that does not exist.
//
// The brief is cut to fit rather than the section being allowed to grow, because the section is read
// on every exec and the brief is the only part of it whose length nobody controls. A cut line ends
// with a full stop and nothing else: an ellipsis or a note saying the text was cut would tell the
// model to go looking for the rest, and the rest is in the design.
func designSummary(project string, design *quaycrewv1.Design, hasBody, hasPath bool,
	on *quaycrewv1.Step, inThePath int) string {
	lines := []string{"This project is " + project + "."}
	if hasBody {
		lines = append(lines, approvalLine(design), readLine(hasPath))
	}
	// The step this session took, which most sessions did not. It goes above the brief's own room so
	// a long brief is cut for it rather than pushing it out of the section.
	if on != nil {
		lines = append(lines, fmt.Sprintf("You are on step %d of %d: %s",
			on.GetNumber(), inThePath, on.GetTitle()))
	}
	brief := design.GetBrief()
	if brief == "" {
		return strings.Join(lines, "\n")
	}

	// What is left for the brief once the rest of the section is written. The two is the " It is
	// for: " separator's own cost against the line it joins, counted here rather than guessed.
	spent := utf8.RuneCountInString(strings.Join(lines, "\n")) + utf8.RuneCountInString(" It is for: ")
	room := min(designBriefCap, designSectionCap-spent)
	if room <= 0 {
		return strings.Join(lines, "\n")
	}
	lines[0] += " It is for: " + cutTo(brief, room)
	return strings.Join(lines, "\n")
}

// readLine sends the session to the documents it has. The path is named only when the project has
// one, for the reason renderDesign gives: a line naming a file that is not there sends the model to
// open nothing.
func readLine(hasPath bool) string {
	line := "Read " + designDir + "/" + designFile + " before you start."
	if hasPath {
		line += " The whole path is in " + designDir + "/" + pathFile + "."
	}
	return line
}

// approvalLine says where the design stands with the operator, in the one line the session reads on
// every exec. The date and nothing finer: the session decides what to do with the design, and the
// hour it was approved changes none of that.
func approvalLine(design *quaycrewv1.Design) string {
	if !design.GetApproved() {
		return "The design is not approved yet."
	}
	return "The design is approved, on " + design.GetApprovedAt().AsTime().Format("2006-01-02") + "."
}

// cutTo shortens text to a number of characters, at a word boundary where there is one, and ends it
// with a full stop. Text already short enough is the operator's own and is left exactly as it is.
func cutTo(text string, room int) string {
	if utf8.RuneCountInString(text) <= room {
		return text
	}
	// One character of the room is the full stop this ends with.
	runes := []rune(text)[:room-1]
	cut := strings.TrimRight(string(runes), " \t\n.,;:")
	if at := strings.LastIndexAny(cut, " \t\n"); at > 0 {
		cut = strings.TrimRight(cut[:at], " \t\n.,;:")
	}
	return cut + "."
}

// renderDesignTo puts a changed design in front of every live session in the project.
//
// Without it a design only reached a session when its sandbox was built, so writing one while a
// session was running did nothing that session could see, and nobody replaces a container to deliver
// a document. It is the same reason SetContext renders, and the same call underneath.
func (s *Server) renderDesignTo(ctx context.Context, project string) {
	s.renderTo(ctx, store.ContextProject, project)
}

// The path a design was broken into: the document grammar, and the two calls that write and read it.
//
// The control plane parses the document rather than the caller. That way the command line and the
// console send the same words and cannot drift on the grammar, which is the same reason the control
// plane composes the text a session is dispatched with.

// The seven labels a step block carries. Each one sits alone on its line, and a block runs from its
// label to the next label, to the next step heading, or to the end of the document.
const (
	labelIntention = "What changes and why"
	labelTouches   = "What this touches"
	labelProof     = "What proves it"
	labelScenario  = "The scenario that proves it"
	labelContracts = "The contracts it builds"
	labelScope     = "The scope of each contract"
	labelAfter     = "After"
)

// stepHeading matches a step's own line: `## <number>. <title>`.
//
// The sign is matched so a number below one is read as a heading with a bad number rather than as
// prose. Left out, `## -1. the store` would be ignored the way a paragraph is, and the refusal about
// a number below one could only ever fire for zero.
var stepHeading = regexp.MustCompile(`^##\s+(-?\d+)\s*\.\s*(.*)$`)

// milestoneHeading matches a milestone's own line: `# <number>. <title>`. One hash, so it sits above
// the step headings, which carry two.
//
// A hash line carrying no number and no full stop matches nothing here, so a document may still
// open with a title and that title is read as the prose around it is.
//
// The sign is matched for the reason stepHeading matches it.
var milestoneHeading = regexp.MustCompile(`^#\s+(-?\d+)\s*\.\s*(.*)$`)

// pathStepMark is the count past which a path is long enough to say so. It refuses nothing: the
// path is kept whole and a person decides.
const pathStepMark = 200

// declaredStep is one step as the document declares it, with the line numbers a refusal has to name.
type declaredStep struct {
	step        store.Step
	headingLine int
	// numberText is the number as the document wrote it, so a refusal quotes what a person typed
	// rather than what it became. A number too large for the column never reaches step.Number.
	numberText string
	// afterSaid is whether the document carried an After block at all, which is a different thing
	// from one that holds nothing. A block holding nothing means zero, and no block at all means the
	// step waits for the number below it.
	afterSaid bool
	afterText string
	afterLine int
	// scenarioExtraLine is the second line of a scenario block, which holds one line and no more.
	scenarioExtraLine int
	// scope is the scope block line by line, with the line each one sits on, because every refusal
	// about a scope line names that line for somebody to go and fix.
	scope []declaredScope
}

// declaredScope is one line of a step's scope block: what it says about which contract, and where it
// is.
type declaredScope struct {
	// identifier is what the line names before the colon, trimmed. It is empty for a line that names
	// nothing, which the contracts block then does not name either.
	identifier string
	line       int
	// stopped is whether the line carries a colon after the identifier at all. A line that does not
	// is refused, because without the stop there is no telling the identifier from the sentence.
	stopped bool
}

// declaredMilestone is one milestone as the document declares it, with the line a refusal has to
// name and the lines under the heading that are its intention.
type declaredMilestone struct {
	milestone   store.Milestone
	headingLine int
	// numberText is the number as the document wrote it, for the reason declaredStep keeps one.
	numberText string
	intention  []string
}

// parsePath reads a path document and returns the milestones and the steps, each in ascending number
// order, with any warnings. Every refusal is an InvalidArgument naming the line, so a person can go
// and fix it.
func parsePath(document string) ([]store.Milestone, []store.Step, []string, error) {
	grouped, declared := readPathDocument(document)
	if len(declared) == 0 {
		return nil, nil, nil, status.Error(codes.InvalidArgument,
			"this document has no steps in it. A step starts with a line reading ## 1. <title>")
	}
	// The milestones first, because a milestone heading sits above the steps it groups and a person
	// reading the refusal reads the document from the top.
	if err := refuseBadMilestones(grouped); err != nil {
		return nil, nil, nil, err
	}
	if err := refuseBadHeadings(declared); err != nil {
		return nil, nil, nil, err
	}
	// After the headings and before the sort, so the lines a refusal names are read in the order the
	// document writes them rather than in the order the numbers happen to run.
	if err := refuseBadScope(declared); err != nil {
		return nil, nil, nil, err
	}
	sort.SliceStable(grouped, func(i, j int) bool {
		return grouped[i].milestone.Number < grouped[j].milestone.Number
	})
	sort.SliceStable(declared, func(i, j int) bool {
		return declared[i].step.Number < declared[j].step.Number
	})
	if err := resolveAfter(declared); err != nil {
		return nil, nil, nil, err
	}

	milestones := make([]store.Milestone, 0, len(grouped))
	for _, one := range grouped {
		one.milestone.Intention = joinBlock(one.intention)
		milestones = append(milestones, one.milestone)
	}
	steps := make([]store.Step, 0, len(declared))
	for _, one := range declared {
		steps = append(steps, one.step)
	}
	return milestones, steps, pathWarnings(milestones, steps), nil
}

// readPathDocument walks the document once and collects what each milestone and each step declared.
//
// Text before the first heading is ignored, so a document may carry a title and a paragraph saying
// what the path is for. Every label is optional: a step needs only its heading.
//
// A step belongs to the milestone heading above it, and to no milestone when there is none above it.
// The number is stamped on the step as it is read, so the grouping is the document's own order and
// never a second pass over it.
func readPathDocument(document string) ([]*declaredMilestone, []*declaredStep) {
	var (
		grouped  []*declaredMilestone
		declared []*declaredStep
		current  *declaredStep
		// milestone is the last milestone heading read, which every step after it belongs to.
		milestone *declaredMilestone
		// underMilestone is whether the lines being read are still the milestone's intention. A
		// heading of either kind ends it, which is what stops a step's own blocks being read as one.
		underMilestone bool
		blocks         map[string][]string
		label          string
	)
	// finish writes the blocks read so far onto the step they belong to. It runs at the next heading
	// and again at the end, because the last step's blocks have no heading after them.
	finish := func() {
		if current == nil {
			return
		}
		current.step.Intention = joinBlock(blocks[labelIntention])
		current.step.Touches = joinBlock(blocks[labelTouches])
		current.step.Proof = joinBlock(blocks[labelProof])
		current.step.ProofScenario = joinBlock(blocks[labelScenario])
		current.step.Contracts = joinBlock(blocks[labelContracts])
		current.step.ContractScope = joinBlock(blocks[labelScope])
		current.afterText = joinBlock(blocks[labelAfter])
		declared = append(declared, current)
	}

	for at, line := range strings.Split(document, "\n") {
		number := at + 1
		if found := stepHeading.FindStringSubmatch(line); found != nil {
			finish()
			underMilestone = false
			// Parsed at the width the column holds, so a number larger than that is refused by the
			// rule below rather than wrapping into a different step silently.
			read, err := strconv.ParseInt(found[1], 10, 32)
			if err != nil {
				read = 0
			}
			current = &declaredStep{
				step: store.Step{
					Number:    int32(read),
					Title:     strings.TrimSpace(found[2]),
					Milestone: milestoneAbove(milestone),
				},
				headingLine: number,
				numberText:  found[1],
			}
			blocks, label = map[string][]string{}, ""
			continue
		}
		if found := milestoneHeading.FindStringSubmatch(line); found != nil {
			// A heading of either kind ends the blocks of the step above it.
			finish()
			current, underMilestone = nil, true
			read, err := strconv.ParseInt(found[1], 10, 32)
			if err != nil {
				read = 0
			}
			milestone = &declaredMilestone{
				milestone:   store.Milestone{Number: int32(read), Title: strings.TrimSpace(found[2])},
				headingLine: number,
				numberText:  found[1],
			}
			grouped = append(grouped, milestone)
			continue
		}
		if underMilestone {
			milestone.intention = append(milestone.intention, line)
			continue
		}
		if current == nil {
			continue
		}
		if named, is := labelOn(line); is {
			label = named
			if named == labelAfter {
				current.afterSaid, current.afterLine = true, number
			}
			continue
		}
		if label == "" {
			continue
		}
		// The scenario block holds one line, so the second line with anything on it is the line a
		// person has to delete. Blank lines do not count: a block written with a line break under its
		// label still names one scenario. Recorded rather than refused here, so every refusal runs in
		// one place.
		if label == labelScenario && strings.TrimSpace(line) != "" && current.scenarioExtraLine == 0 &&
			saidSomething(blocks[labelScenario]) {
			current.scenarioExtraLine = number
		}
		// A scope line is kept with the line it sits on, because every refusal about one names that
		// line. A blank line inside the block says nothing about a contract, so it is not one.
		if label == labelScope && strings.TrimSpace(line) != "" {
			current.scope = append(current.scope, scopeLineOn(line, number))
		}
		blocks[label] = append(blocks[label], line)
	}
	finish()
	return grouped, declared
}

// scopeLineOn reads one line of a scope block: the identifier it names, and whether it stopped after
// that identifier at all.
//
// The stop is a colon, and it is what tells the identifier from the sentence. A line without one
// names no contract, so it is refused rather than read as an identifier the whole line long.
func scopeLineOn(line string, number int) declaredScope {
	identifier, _, stopped := strings.Cut(strings.TrimSpace(line), ":")
	return declaredScope{identifier: strings.TrimSpace(identifier), line: number, stopped: stopped}
}

// milestoneAbove is the number a step read now belongs to, and zero when no milestone heading has
// been read yet. Zero is never a milestone row, so it is how a step says it belongs to none.
func milestoneAbove(milestone *declaredMilestone) int32 {
	if milestone == nil {
		return 0
	}
	return milestone.milestone.Number
}

// saidSomething says whether a block already holds a line with anything on it, so a blank line
// under a label is not read as a second answer.
func saidSomething(lines []string) bool {
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			return true
		}
	}
	return false
}

// labelOn says whether a line is one of the seven labels, alone on its line.
func labelOn(line string) (string, bool) {
	switch strings.TrimSpace(line) {
	case labelIntention:
		return labelIntention, true
	case labelTouches:
		return labelTouches, true
	case labelProof:
		return labelProof, true
	case labelScenario:
		return labelScenario, true
	case labelContracts:
		return labelContracts, true
	case labelScope:
		return labelScope, true
	case labelAfter:
		return labelAfter, true
	}
	return "", false
}

// joinBlock is a block's own text: the blank lines at each end taken off, and everything between
// them kept, because `What this touches` is read line by line later.
func joinBlock(lines []string) string {
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// refuseBadMilestones holds the rules about a milestone's own line: its number and its title. They
// are the step's rules, counted apart, because a milestone number and a step number are two
// numberings and neither constrains the other.
func refuseBadMilestones(grouped []*declaredMilestone) error {
	seen := make(map[int32]int, len(grouped))
	for _, one := range grouped {
		if one.milestone.Number < 1 {
			return status.Errorf(codes.InvalidArgument,
				"line %d: this milestone is numbered %s, and milestones are numbered from one",
				one.headingLine, one.numberText)
		}
		if before, already := seen[one.milestone.Number]; already {
			return status.Errorf(codes.InvalidArgument,
				"line %d: milestone %d is already declared on line %d, and two milestones cannot share a number",
				one.headingLine, one.milestone.Number, before)
		}
		if one.milestone.Title == "" {
			return status.Errorf(codes.InvalidArgument,
				"line %d: milestone %d has no title, and a title is the one line saying what the milestone is",
				one.headingLine, one.milestone.Number)
		}
		seen[one.milestone.Number] = one.headingLine
	}
	return nil
}

// refuseBadHeadings holds the rules about a step's own line: its number and its title.
//
// A duplicate names both lines, because the person fixing it has to see the one they forgot as well
// as the one they are looking at.
func refuseBadHeadings(declared []*declaredStep) error {
	seen := make(map[int32]int, len(declared))
	for _, one := range declared {
		if one.step.Number < 1 {
			return status.Errorf(codes.InvalidArgument,
				"line %d: this step is numbered %s, and a path is numbered from one",
				one.headingLine, one.numberText)
		}
		if before, already := seen[one.step.Number]; already {
			return status.Errorf(codes.InvalidArgument,
				"line %d: step %d is already declared on line %d. Step numbers are unique across the whole "+
					"feature, and not inside a milestone, so a milestone heading does not start the numbering again",
				one.headingLine, one.step.Number, before)
		}
		if one.step.Title == "" {
			return status.Errorf(codes.InvalidArgument,
				"line %d: step %d has no title, and a title is the one line saying what the step is",
				one.headingLine, one.step.Number)
		}
		if one.scenarioExtraLine > 0 {
			return status.Errorf(codes.InvalidArgument,
				"line %d: step %d names more than one scenario, and a step names exactly one",
				one.scenarioExtraLine, one.step.Number)
		}
		seen[one.step.Number] = one.headingLine
	}
	return nil
}

// refuseBadScope holds the rules about the scope block: the form of each line, and which contracts
// those lines are allowed to name.
//
// A scope line that names a contract the step does not build is the defect this refusal exists for.
// It reads as an answer about that contract, and the take text then hands the session a scope for
// work that belongs to another step.
//
// Nothing here asks whether an identifier names a contract that exists. Krewe never reads the
// project's contracts document, and the session opens it for itself.
func refuseBadScope(declared []*declaredStep) error {
	for _, one := range declared {
		named := contractsNamed(one.step)
		seen := make(map[string]int, len(one.scope))
		for _, scope := range one.scope {
			if !scope.stopped {
				return status.Errorf(codes.InvalidArgument,
					"line %d: step %d gives a scope that does not stop after the identifier, "+
						"and the form is <identifier>: <sentence>",
					scope.line, one.step.Number)
			}
			if !named[scope.identifier] {
				return status.Errorf(codes.InvalidArgument,
					"line %d: step %d gives the scope of %s, and %q does not name it",
					scope.line, one.step.Number, scope.identifier, labelContracts)
			}
			if before, already := seen[scope.identifier]; already {
				return status.Errorf(codes.InvalidArgument,
					"line %d: the scope of %s is already given on line %d, and one contract has one scope line",
					scope.line, scope.identifier, before)
			}
			seen[scope.identifier] = scope.line
		}
	}
	return nil
}

// contractsNamed is the identifiers a step's contracts block names, one per line. A blank line names
// nothing, so it is left out.
func contractsNamed(step store.Step) map[string]bool {
	named := make(map[string]bool)
	for _, line := range strings.Split(step.Contracts, "\n") {
		if identifier := strings.TrimSpace(line); identifier != "" {
			named[identifier] = true
		}
	}
	return named
}

// scopedContracts is the identifiers a step's scope block gives a line to, which is what the warning
// about a contract nobody scoped is counted from.
func scopedContracts(step store.Step) map[string]bool {
	scoped := make(map[string]bool)
	for _, line := range strings.Split(step.ContractScope, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		identifier, _, _ := strings.Cut(strings.TrimSpace(line), ":")
		scoped[strings.TrimSpace(identifier)] = true
	}
	return scoped
}

// resolveAfter reads what each step waits for, and gives a step that says nothing the number below
// it. The steps arrive in ascending number order.
//
// A default of nothing would make the gate worthless, because every step would be ready at once. So
// a numbered path is a chain unless the document says otherwise, and saying otherwise is an After
// block holding nothing.
func resolveAfter(declared []*declaredStep) error {
	numbers := make(map[int32]bool, len(declared))
	for _, one := range declared {
		numbers[one.step.Number] = true
	}

	for at, one := range declared {
		if !one.afterSaid {
			if at > 0 {
				one.step.After = declared[at-1].step.Number
			}
			continue
		}
		// An After block holding nothing means zero, which is the way to say a step waits for nobody.
		if one.afterText == "" {
			continue
		}
		read, err := strconv.Atoi(one.afterText)
		if err != nil {
			return status.Errorf(codes.InvalidArgument,
				"line %d: step %d says it comes after %q, and After takes one step number or nothing",
				one.afterLine, one.step.Number, one.afterText)
		}
		after := int32(read)
		if after == 0 {
			continue
		}
		if !numbers[after] {
			return status.Errorf(codes.InvalidArgument,
				"line %d: step %d says it comes after step %d, and this document has no step %d",
				one.afterLine, one.step.Number, after, after)
		}
		if after >= one.step.Number {
			return status.Errorf(codes.InvalidArgument,
				"line %d: step %d says it comes after step %d, and a step waits for a lower number",
				one.afterLine, one.step.Number, after)
		}
		one.step.After = after
	}
	return nil
}

// pathWarnings is what the write says about a path it kept whole. No warning refuses a document.
func pathWarnings(milestones []store.Milestone, steps []store.Step) []string {
	var warnings []string
	under := make(map[int32]int, len(milestones))
	for _, step := range steps {
		under[step.Milestone]++
	}
	for _, milestone := range milestones {
		// A milestone nobody planned steps for is worth seeing on the listing, so it is kept and said
		// out loud rather than dropped for being empty.
		if under[milestone.Number] == 0 {
			warnings = append(warnings, fmt.Sprintf(
				"milestone %d has no step under it. It is kept as it is.", milestone.Number))
		}
		if milestone.Intention == "" {
			warnings = append(warnings, fmt.Sprintf(
				"milestone %d says nothing under its heading. It is kept as it is.", milestone.Number))
		}
	}
	if len(steps) > pathStepMark {
		warnings = append(warnings, fmt.Sprintf(
			"this path has %d steps, over the %d mark. It is kept whole.", len(steps), pathStepMark))
	}
	for _, step := range steps {
		if missing := whatTheStepDoesNotSay(step); len(missing) > 0 {
			warnings = append(warnings, fmt.Sprintf(
				"step %d says nothing under %s. It is kept as it is.",
				step.Number, strings.Join(missing, ", ")))
		}
		// Harder than the line above, because this is the one krewe itself cannot work around: it
		// runs the scenario a step names, and a step that names none is a step it cannot check.
		if step.ProofScenario == "" {
			warnings = append(warnings, fmt.Sprintf(
				"step %d names no scenario, so krewe step check will refuse this step.", step.Number))
		}
		// The take reads this block to refuse a step that writes a file a step in flight writes. A
		// step that names no file matches nothing, so it passes that check whatever it goes on to
		// write, and a second session may be given the same file.
		if step.Touches == "" {
			warnings = append(warnings, fmt.Sprintf(
				"step %d names no file under %s, so it collides with nothing and a second step may "+
					"be taken on the same file.", step.Number, labelTouches))
		}
		warnings = append(warnings, whatTheStepSaysAboutContracts(step)...)
	}
	return warnings
}

// whatTheStepSaysAboutContracts is what a path write says about the contracts a step builds. Neither
// of these refuses the document: a contract is a string the operator wrote, and the session reads
// the contracts document itself.
//
// A step naming no contract is worth saying out loud because the take then hands that session
// nothing, and finding its own contract is the work this column exists to save.
func whatTheStepSaysAboutContracts(step store.Step) []string {
	named := contractsNamed(step)
	if len(named) == 0 {
		return []string{fmt.Sprintf(
			"step %d names no contract, so krewe hands it none and the session has to find its own.",
			step.Number)}
	}
	// In the order the document names them, so two warnings about one step read down the block the
	// person is looking at. A map is read in no order at all.
	scoped := scopedContracts(step)
	var warnings []string
	for _, line := range strings.Split(step.Contracts, "\n") {
		identifier := strings.TrimSpace(line)
		if identifier == "" || scoped[identifier] {
			continue
		}
		warnings = append(warnings, fmt.Sprintf(
			"step %d builds %s and says nothing about its scope. It is kept as it is.",
			step.Number, identifier))
	}
	return warnings
}

// whatTheStepDoesNotSay names the blocks a step left empty, in the order the document writes them.
func whatTheStepDoesNotSay(step store.Step) []string {
	var missing []string
	for _, block := range []struct {
		label string
		text  string
	}{
		{labelIntention, step.Intention},
		{labelTouches, step.Touches},
		{labelProof, step.Proof},
		{labelScenario, step.ProofScenario},
	} {
		if block.text == "" {
			missing = append(missing, block.label)
		}
	}
	return missing
}

// SetPath replaces one feature's path with the steps the document declares.
//
// A refused document changes nothing, because the parse runs before the write. There is no way to
// empty a path: a document with no step heading is refused, so a wrong file path cannot take
// somebody's path away.
//
// It writes one feature's path and leaves every other feature of the project alone, which is what
// lets two features of one project be built at the same time.
func (s *Server) SetPath(ctx context.Context, req *quaycrewv1.SetPathRequest) (*quaycrewv1.SetPathResponse, error) {
	if req.GetFeature() == "" {
		return nil, status.Error(codes.InvalidArgument, "which feature: a path belongs to one, so say its number")
	}
	milestones, steps, warnings, err := parsePath(req.GetDocument())
	if err != nil {
		return nil, err
	}
	written, err := s.store.SetPath(ctx, req.GetFeature(), milestones, steps)
	var protected *store.ProtectedStepsError
	if errors.As(err, &protected) {
		return nil, status.Error(codes.FailedPrecondition, keepTheSteps(protected.Steps))
	}
	if err != nil {
		return nil, storeError(err, "feature")
	}
	return &quaycrewv1.SetPathResponse{Steps: written, Warnings: warnings}, nil
}

// What the session working in a project reads about the steps before its own: the path, written
// into its working directory beside the design.
//
// It is a file rather than a section in the memory file because a path grows with the project and
// the memory file is read on every exec. The summary names it, and a model that needs it opens it.

// renderPath puts the project's path where the model can open it, and says whether it is there to
// open. A project with no path has no file: a file that exists and says nothing costs a read.
//
// Every open feature of the project goes in, one whole path after another, because the session reads
// this as what the project is building and it works in the project rather than in one feature of it.
// Each feature's steps are written under a heading of their own rather than merged into one run of
// numbers, since two features each start at step 1 and a merged list would read as one path with the
// numbers repeating.
//
// A feature that is done or stopped is left out, which is what keeps this file from growing with
// every finished feature. The store answers with every state, so the filter is here. Reopening a
// feature brings it back on the next render.
//
// Nothing here fails an exec, for the same reason renderDesign fails none.
func (s *Server) renderPath(ctx context.Context, project, dir string) bool {
	features, err := s.store.ListFeatures(ctx, project)
	if err != nil {
		return false
	}
	paths := make([]string, 0, len(features))
	for _, feature := range features {
		if feature.GetState() != store.FeatureOpen {
			continue
		}
		steps, err := s.store.ListSteps(ctx, feature.GetId())
		if err != nil {
			continue
		}
		milestones, err := s.store.ListMilestones(ctx, feature.GetId())
		if err != nil {
			continue
		}
		if document := pathDocument(feature, milestones, steps); document != "" {
			paths = append(paths, document)
		}
	}
	return s.writeSessionFile(dir, pathFile, "path", strings.Join(paths, "\n"))
}

// pathDocument is one feature's path: a heading naming the feature, a heading for each milestone it
// is delivered in, and one block for each step under the milestone it belongs to.
//
// The order is the number's and never the store's. A session reads this file as the path, so steps
// out of order are a different path: the one thing the file is for is saying what came before this
// step.
func pathDocument(feature *quaycrewv1.Feature,
	milestones []*quaycrewv1.Milestone, steps []*quaycrewv1.Step) string {
	if len(steps) == 0 {
		return ""
	}
	ordered := make([]*quaycrewv1.Step, len(steps))
	copy(ordered, steps)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].GetNumber() < ordered[j].GetNumber()
	})

	parts := []string{featureHeading(feature)}
	for _, group := range groupPath(milestones, ordered) {
		parts = append(parts, "## "+group.heading)
		for _, step := range group.steps {
			parts = append(parts, stepBlock(step, group.named))
		}
	}
	return strings.Join(parts, "\n\n") + "\n"
}

// featureHeading is the feature's own line and what it narrows to, which is one line or none.
//
// The heading is there so a session reading two paths knows which project part each one belongs to.
// Two features of a project each hold a step 1, and without the heading the file reads as one path
// that counts up twice.
func featureHeading(feature *quaycrewv1.Feature) string {
	heading := fmt.Sprintf("# %d. %s", feature.GetNumber(), feature.GetTitle())
	if feature.GetIntention() == "" {
		return heading
	}
	return heading + "\n" + feature.GetIntention()
}

// pathGroup is one milestone of the document and the steps written under it.
type pathGroup struct {
	// heading is the milestone's own line, under two hashes.
	heading string
	// named is what the step blocks under it say their milestone is. It repeats the heading, so a
	// reader who opens the file at one step still knows where the step sits.
	named string
	steps []*quaycrewv1.Step
}

// groupPath puts every step under the milestone it belongs to.
//
// The steps under no milestone come first, because a milestone of 0 is below every milestone number
// and the document is in number order between milestones as well as inside one. A step whose
// milestone matches no milestone of the feature is written there too. A step that falls out of the
// document is the worst thing this render can do, because what is left still reads as the whole path.
func groupPath(milestones []*quaycrewv1.Milestone, steps []*quaycrewv1.Step) []pathGroup {
	under := func(wanted func(*quaycrewv1.Step) bool) []*quaycrewv1.Step {
		held := make([]*quaycrewv1.Step, 0, len(steps))
		for _, step := range steps {
			if wanted(step) {
				held = append(held, step)
			}
		}
		return held
	}
	named := make(map[int32]bool, len(milestones))
	for _, milestone := range milestones {
		named[milestone.GetNumber()] = true
	}

	grouped := make([]pathGroup, 0, len(milestones)+1)
	if loose := under(func(step *quaycrewv1.Step) bool { return !named[step.GetMilestone()] }); len(loose) > 0 {
		grouped = append(grouped, pathGroup{heading: noMilestoneHeading, named: noMilestone, steps: loose})
	}
	for _, milestone := range milestones {
		number := milestone.GetNumber()
		line := fmt.Sprintf("%d. %s", number, milestone.GetTitle())
		grouped = append(grouped, pathGroup{
			heading: line,
			named:   line,
			steps:   under(func(step *quaycrewv1.Step) bool { return step.GetMilestone() == number }),
		})
	}
	return grouped
}

// What a step with no milestone reads under, and what its own line says. The two differ by one
// capital letter, because one of them opens a heading.
const (
	noMilestoneHeading = "No milestone"
	noMilestone        = "no milestone"
)

// stepBlock is one step: its heading, where it sits, what came of it once somebody closed it, and the
// blocks the operator wrote under it, in the words they set. A block they left empty is left out
// rather than written as a bare label, because a label with nothing under it costs a read and
// answers nothing.
//
// The milestone line says what the heading above it says. A session opens this file at its own step
// and reads down, not from the top, so a block that named no milestone would leave the reader
// scrolling for the heading it sits under.
func stepBlock(step *quaycrewv1.Step, milestone string) string {
	lines := []string{
		fmt.Sprintf("### %d. %s", step.GetNumber(), step.GetTitle()),
		"milestone: " + milestone,
		"state: " + step.GetState(),
	}
	// What came of a finished step, under its state. This is what makes a session start from what is
	// true: a session on step 4 reads what steps 1 to 3 produced. Nothing can see inside a container,
	// so this line is what somebody wrote and there is nothing else to write.
	if step.GetResult() != "" {
		lines = append(lines, "result: "+step.GetResult())
	}
	for _, block := range []struct {
		label string
		text  string
	}{
		{labelIntention, step.GetIntention()},
		{labelTouches, step.GetTouches()},
		{labelProof, step.GetProof()},
		{labelScenario, step.GetProofScenario()},
		{labelAfter, strconv.Itoa(int(step.GetAfter()))},
	} {
		if block.text == "" {
			continue
		}
		lines = append(lines, "", block.label, block.text)
	}
	return strings.Join(lines, "\n")
}

// ListSteps reads a feature's path and the milestones it is grouped into, or every feature's path
// when it names none.
//
// A feature with no path answers with an empty list rather than an error, because nothing written is
// the normal state. Reading records nothing.
func (s *Server) ListSteps(ctx context.Context, req *quaycrewv1.ListStepsRequest) (*quaycrewv1.ListStepsResponse, error) {
	steps, err := s.store.ListSteps(ctx, req.GetFeature())
	if err != nil {
		return nil, storeError(err, "feature")
	}
	// Number order is this call's promise, so it is made here rather than left to the store. Two
	// surfaces draw this listing, and an order either one worked out for itself would let the two
	// disagree about one path in front of somebody.
	//
	// A request that names no feature is left as the store answered it: that order is by feature and
	// then by number, and sorting on the number alone would shuffle the features together.
	if req.GetFeature() != "" {
		sort.SliceStable(steps, func(one, two int) bool {
			return steps[one].GetNumber() < steps[two].GetNumber()
		})
	}
	answer := &quaycrewv1.ListStepsResponse{Steps: steps}
	// What is next is the operator's next command, and it is a sentence: reading it starts no
	// session, takes no step and writes nothing.
	//
	// It is left at 0 when the request names no feature, because what is next is a question about
	// one path, and a lowest number read across several paths would name a step of a path nobody
	// asked about.
	if req.GetFeature() != "" {
		answer.Next = nextStep(steps)
	}
	// The milestones travel with the steps, so a caller groups the listing without a second call.
	// They are left out when the request names no feature, because a milestone number restarts in
	// each feature and a merged list would read as one run of numbers.
	if req.GetFeature() != "" {
		milestones, err := s.store.ListMilestones(ctx, req.GetFeature())
		if err != nil {
			return nil, storeError(err, "feature")
		}
		sort.SliceStable(milestones, func(one, two int) bool {
			return milestones[one].GetNumber() < milestones[two].GetNumber()
		})
		answer.Milestones = milestones
	}
	return answer, nil
}

// nextStep is the lowest numbered step somebody may take now: a step in state ready whose predecessor
// is done, or that waits for nobody.
//
// Both halves of the rule are read. A ready step whose predecessor is unfinished is not next, however
// low its number, because taking it would build on work nobody proved. Answering with the lowest
// ready step alone is right on a path where the steps were done in order and wrong everywhere else.
//
// It answers 0 when no step qualifies, and 0 is an answer rather than an error: an empty path has
// nothing to start, and so does a path where every ready step waits on something unfinished.
//
// The order it is given does not decide the answer, so a caller that sorted nothing gets the same
// number as a caller that did.
func nextStep(steps []*quaycrewv1.Step) int32 {
	done := make(map[int32]bool, len(steps))
	for _, step := range steps {
		if step.GetState() == stepDone {
			done[step.GetNumber()] = true
		}
	}
	next := int32(0)
	for _, step := range steps {
		if step.GetState() != store.StepReady {
			continue
		}
		if step.GetAfter() != 0 && !done[step.GetAfter()] {
			continue
		}
		if next == 0 || step.GetNumber() < next {
			next = step.GetNumber()
		}
	}
	return next
}

// GetStep reads one step, whole, after the session that holds it is asked what it wrote.
//
// One call refreshes and answers. The session writes its restatement into its own memory file and
// makes no call, so a read that only looked at the store would answer with what the session
// understood at its last exec. Reading the file here is what lets the operator read the text the
// moment it is written, with no dispatch in between, which matters because the operator reads the
// restatement before agreeing to it.
//
// Nothing about the refresh can fail the read. A file that cannot be read, and a restatement long
// enough to say so, both come back as warnings beside the step rather than in place of it.
func (s *Server) GetStep(ctx context.Context, req *quaycrewv1.GetStepRequest) (*quaycrewv1.GetStepResponse, error) {
	if req.GetFeature() == "" {
		return nil, status.Error(codes.InvalidArgument, "which feature: a step belongs to one, so say its number")
	}
	if req.GetNumber() < 1 {
		return nil, status.Error(codes.InvalidArgument, "a step number counts from one")
	}
	// The whole path, so a number nobody wrote is refused with how many steps there are rather than
	// with a bare not found. It is the read the take already does, for the same reason.
	steps, err := s.store.ListSteps(ctx, req.GetFeature())
	if err != nil {
		return nil, storeError(err, "feature")
	}
	held := stepNumbered(steps, req.GetNumber())
	if held == nil {
		return nil, noSuchStep(req.GetNumber(), len(steps))
	}
	fresh, warnings := s.refreshRestatement(ctx, held)
	return &quaycrewv1.GetStepResponse{
		Step: fresh, Warnings: append(warnings, tooLongToBeSix(fresh)...),
	}, nil
}

// Taking a step: the gate the operator's own command is refused by, the write that records who holds
// it, and the text the session is dispatched with.
//
// The gate reads first and starts nothing. A refusal costs one line of output, and the point of it is
// that no code exists before the operator approves the path.

// TakeStep gives one step of a feature's path to a session, and dispatches that session with the step.
//
// The step is addressed by its feature and the design is the project's, so the feature is read first
// to say which project this is. There is one approval for a project, and a step taken in any feature
// of it reads that one.
//
// The order is the whole design of this call. The approval is read before anything else, so a project
// whose design nobody approved never reaches the store, never mints a session and never starts a
// container. The store's own write is what refuses a step somebody already holds, because a read here
// followed by a write there would let two takes of one step both pass.
func (s *Server) TakeStep(ctx context.Context, req *quaycrewv1.TakeStepRequest) (*quaycrewv1.TakeStepResponse, error) {
	if req.GetFeature() == "" {
		return nil, status.Error(codes.InvalidArgument, "which feature: a step belongs to one, so say its number")
	}
	if req.GetNumber() < 1 {
		return nil, status.Error(codes.InvalidArgument, "a step number counts from one")
	}
	feature, err := s.store.GetFeature(ctx, req.GetFeature())
	if err != nil {
		return nil, storeError(err, "feature")
	}
	design, err := s.store.GetDesign(ctx, feature.GetProject())
	if err != nil {
		return nil, storeError(err, "project")
	}
	if !design.GetApproved() {
		return nil, status.Error(codes.FailedPrecondition,
			"this project's design is not approved, so no step can be taken. "+
				"Read it with krewe design [<address>]. Approve it with krewe design approve [<address>]")
	}

	// The whole path, because the text says which step of how many this is, and because a refusal
	// about a step nobody wrote has to say how many the path holds.
	steps, err := s.store.ListSteps(ctx, req.GetFeature())
	if err != nil {
		return nil, storeError(err, "feature")
	}
	held := stepNumbered(steps, req.GetNumber())
	if held == nil {
		return nil, noSuchStep(req.GetNumber(), len(steps))
	}

	// The session is named before the take, because the store records who holds the step in the same
	// write that moves the state. The name is a handle, which is what a dispatch is addressed by, so
	// the session the store names is the session the dispatch continues.
	handle := store.NewID()
	taken, flying, err := s.store.TakeStep(ctx, req.GetFeature(), req.GetNumber(), handle)
	if errors.Is(err, store.ErrStepNotReady) {
		return nil, status.Error(codes.FailedPrecondition, whoHoldsIt(held))
	}
	// Not a state the caller got wrong. The take is allowed and there is no room for it yet, so the
	// refusal says what runs and what to do about it rather than what is wrong with the request.
	var full *store.StepsInFlightError
	if errors.As(err, &full) {
		return nil, status.Error(codes.ResourceExhausted, noRoomForIt(full))
	}
	// A state the take cannot be given yet, so it reads as the wrong state rather than as a full
	// machine. The step is refused and the feature is not: every other step of this path that names
	// another file is still there to take.
	var shared *store.SharedFileError
	if errors.As(err, &shared) {
		return nil, status.Error(codes.FailedPrecondition, somebodyElseWritesIt(shared))
	}
	if err != nil {
		return nil, storeError(err, "step")
	}

	// The design was read above for the approval, and it carries the contracts document too, so the
	// text says to read that file only when the project has one.
	text := takeText(taken, len(steps), s.projectName(ctx, feature.GetProject()),
		design.GetContracts() != "")
	dispatched, err := s.Dispatch(ctx, &quaycrewv1.DispatchRequest{
		Project: feature.GetProject(), Handle: handle, Text: text, Detach: true,
	})
	if err != nil {
		return nil, err
	}
	started, err := s.store.GetSession(ctx, dispatched.GetId())
	if err != nil {
		return nil, storeError(err, "session")
	}
	// The count is the one the write made, and the cap is the number it was counted against, so a
	// caller prints what runs now without asking again.
	return &quaycrewv1.TakeStepResponse{
		Step: taken, Session: started, Text: text,
		InFlight: flying, StepsInFlightCap: design.GetStepsInFlightCap(),
	}, nil
}

// closedByOperator is who spoke the word on this call. Krewe closes a step through its own check, and
// that call writes the other word.
const closedByOperator = "operator"

// stepFinishStates are the two ways a step ends, in the order the refusal names them.
func stepFinishStates() []string { return []string{stepDone, stepStopped} }

// FinishStep records what came of one step: the word that closes it, and what somebody wrote.
//
// The result is required, and that is the whole point of the call. Nothing can see inside a
// container, so what somebody wrote is all the next session gets: a step marked done with no result
// tells it nothing. An unknown word is refused rather than stored, the way an unknown permission mode
// already is, so one layer owns the two words a step ends with.
//
// Nothing dispatches, stops or reclaims a session as a consequence. The step and the session that
// took it are separate records, and the row still says who took it after the write.
func (s *Server) FinishStep(ctx context.Context, req *quaycrewv1.FinishStepRequest) (*quaycrewv1.FinishStepResponse, error) {
	if req.GetFeature() == "" {
		return nil, status.Error(codes.InvalidArgument, "which feature: a step belongs to one, so say its number")
	}
	if req.GetNumber() < 1 {
		return nil, status.Error(codes.InvalidArgument, "a step number counts from one")
	}
	if !slices.Contains(stepFinishStates(), req.GetState()) {
		return nil, status.Errorf(codes.InvalidArgument,
			"%q is not a way to finish a step: use %s or %s", req.GetState(), stepDone, stepStopped)
	}
	if req.GetResult() == "" {
		return nil, status.Error(codes.InvalidArgument,
			"say what came of it, because nothing can see inside the container")
	}

	// The whole path, so a number nobody wrote is refused with how many steps there are rather than
	// with a bare not found. It is the read the take already does, for the same reason.
	steps, err := s.store.ListSteps(ctx, req.GetFeature())
	if err != nil {
		return nil, storeError(err, "feature")
	}
	if stepNumbered(steps, req.GetNumber()) == nil {
		return nil, noSuchStep(req.GetNumber(), len(steps))
	}
	written, err := s.store.FinishStep(ctx, req.GetFeature(), req.GetNumber(), store.Finish{
		State: req.GetState(), Result: req.GetResult(), ClosedBy: closedByOperator,
	})
	if err != nil {
		return nil, storeError(err, "step")
	}
	return &quaycrewv1.FinishStepResponse{Step: written}, nil
}

// stepNumbered is the step of that number, and nil when the path holds none.
func stepNumbered(steps []*quaycrewv1.Step, number int32) *quaycrewv1.Step {
	for _, step := range steps {
		if step.GetNumber() == number {
			return step
		}
	}
	return nil
}

// keepTheSteps is the refusal for a document that would drop or rename a step somebody worked on.
//
// It names every protected step and the state each one is in, because the operator's next move is to
// put those numbers back under the titles they have, and a refusal naming one of two sends them back
// to write the document twice.
func keepTheSteps(steps []store.ProtectedStep) string {
	said := make([]string, 0, len(steps))
	for _, step := range steps {
		said = append(said, fmt.Sprintf("step %d is %s", step.Number, step.State))
	}
	return strings.Join(said, ", ") +
		". this document drops or renames them, and that takes away the record of the work. " +
		"keep those numbers in the document, with the titles they have."
}

// noSuchStep says how many steps the path has, because a number that is one past the end and a
// number nobody ever wrote read the same to whoever typed it.
func noSuchStep(number int32, held int) error {
	if held == 0 {
		return status.Errorf(codes.NotFound,
			"this feature has no path, so there is no step %d to take: "+
				"write one with krewe path set [<address>] <feature> --file <path>", number)
	}
	return status.Errorf(codes.NotFound, "this path has no step %d: it has %d steps", number, held)
}

// whoHoldsIt is the refusal for a step that is not ready. It names the state, and the session where
// there is one, because the way past a step somebody is already on is to go and talk to that session.
func whoHoldsIt(step *quaycrewv1.Step) string {
	said := fmt.Sprintf("step %d is %s", step.GetNumber(), step.GetState())
	if step.GetSession() == "" {
		return said
	}
	return said + ", and session " + display.ShortID(step.GetSession()) + " holds it"
}

// noRoomForIt is the refusal for a take at the cap. It names every step in flight with the feature
// each one sits in, the cap, and the two ways past it.
//
// Every step rather than the count, because the operator's next move is to finish one, and a refusal
// saying only how many sends them to the listing to work out which. The feature is on each line
// because the cap counts across the whole project: three steps numbered 1, in three features, read as
// one step named three times without it.
func noRoomForIt(full *store.StepsInFlightError) string {
	said := make([]string, 0, len(full.Steps))
	for _, step := range full.Steps {
		said = append(said, fmt.Sprintf("step %d.%d %s", step.FeatureNumber, step.Number, step.FeatureTitle))
	}
	return fmt.Sprintf(
		"%d steps of this project are already in flight, and its cap is %d: %s. "+
			"Finish one with krewe step done [<address>] <feature>.<number> \"<result>\", "+
			"or raise the cap with krewe path cap [<address>] <number>",
		len(full.Steps), full.Cap, strings.Join(said, ", "))
}

// somebodyElseWritesIt is the refusal for a take that names a file a step in flight already names.
//
// It names the file, the step and that step's feature, because the two steps sit in two paths and a
// refusal naming a number alone reads as a step of the path the operator is looking at. The way past
// it is to wait for that step or to finish it, and finishing it lets this take through with nothing
// re-planned.
//
// It reads what each step says it writes and never a diff, so a step that writes a file it did not
// name is not caught here. Section 14 of the design defers that check.
func somebodyElseWritesIt(shared *store.SharedFileError) string {
	return fmt.Sprintf(
		"step %d.%d %s writes %s too, and it is in flight. Two sessions on one file write over each "+
			"other. Wait for that step, or finish it with krewe step done [<address>] "+
			"<feature>.<number> \"<result>\"",
		shared.Step.FeatureNumber, shared.Step.Number, shared.Step.FeatureTitle, shared.File)
}

// takeText is what the session is given: the step whole, where it sits in the path, and what to do
// with it.
//
// A block the step left empty is left out with its label, because a label with nothing under it is
// text the model reads for nothing. The count is of the steps in the path and never of the highest
// number, so a path running 1, 2, 5 reads "of 3".
func takeText(step *quaycrewv1.Step, inThePath int, project string, hasContracts bool) string {
	blocks := []string{
		fmt.Sprintf("Step %d of %d on the path for %s.", step.GetNumber(), inThePath, project),
		step.GetTitle(),
	}
	if step.GetIntention() != "" {
		blocks = append(blocks, labelIntention+"\n"+step.GetIntention())
	}
	if step.GetTouches() != "" {
		blocks = append(blocks, labelTouches+"\n"+step.GetTouches())
	}
	if proof := proofBlock(step); proof != "" {
		blocks = append(blocks, proof)
	}
	// The scoping every step brief of this project carried by hand, copied out of the graph. The
	// system carries it now, so nobody types it and a step cannot be given the wrong part of a
	// contract.
	if step.GetContracts() != "" {
		blocks = append(blocks, takeContracts+"\n"+step.GetContracts())
	}
	if step.GetContractScope() != "" {
		blocks = append(blocks, labelScope+"\n"+step.GetContractScope())
	}
	blocks = append(blocks, whereToRead(hasContracts), takeDelivery, restateFirst())
	return strings.Join(blocks, "\n\n") + "\n"
}

// restateFirst is the last paragraph of the take text, and it is always there, whole. It is what
// makes the session restate the step rather than build it.
//
// It goes after the paragraph about delivering the work, because the last thing the text says is the
// thing the session does next, and the session that has just read this delivers nothing yet.
//
// The mark is read from the package that reads it back, so the section the session is asked to write
// and the section the system looks for cannot come to be two different words.
//
// The six parts are what the operator reads to decide whether the session understood the step. The
// last of them is a percentage, because a number below about 80 is the signal to answer the session
// rather than to approve it.
func restateFirst() string {
	return "Write no code. Change no file in the repository. Write what you understood into your own\n" +
		"CLAUDE.md, inside a section marked " + sandbox.Mark(sandbox.RestatementScope) +
		", with these six headings:\n" +
		"what this step changes, what it will not touch, what you assumed, what you do not know,\n" +
		"the scenario you will write and the value it describes, how sure you are and what lowers it.\n" +
		"Then stop and say you are ready."
}

// takeContracts is what the contracts block is called in the take text. The document calls it "The
// contracts it builds", because there the step is one of many on a page. The session is given one
// step, so the text says which step these contracts belong to.
const takeContracts = "The contracts this step builds"

// takeDelivery is the last block of the take text: what to do with the work once it is built.
//
// Without it a session builds the step and stops, because nothing else in the text says a pull
// request is the deliverable, and the words had to be written into each dispatch by hand.
//
// It names the git and github skills rather than restating them. Both are already in the session's
// context, and a second copy of a rule is a copy that can disagree with the first. Opening a pull
// request is the session's, ending it is the operator's, so the text refuses the merge here too.
const takeDelivery = "Deliver this step as one pull request. The git and github skills say how a " +
	"working tree, a branch, a commit and a pull request are done here." +
	"\n\nWatch what proves this step fail before you make it pass, because a test you did not see " +
	"fail proves nothing. Get every check green before you report, and fix a red check rather than " +
	"explaining it." +
	"\n\nDo not merge it. Report the address of the pull request."

// whereToRead names the files the session opens before it starts.
//
// The contracts document is named only when the project has one. A pointer to a file that is not
// there sends the model to open nothing, which is what renderContracts avoids by writing no file.
func whereToRead(hasContracts bool) string {
	design := "The design is in " + designDir + "/" + designFile + ". "
	path := "The whole path is in " + designDir + "/" + pathFile + ". "
	if !hasContracts {
		return design + path + "Read both."
	}
	return design + "The contracts are in " + designDir + "/" + contractsFile + ". " + path + "Read all three."
}

// proofBlock is what proves the step and the scenario that proves it, which are one block: the
// scenario names what the prose above it describes. Either half alone still says something, so the
// block is left out only when the step says neither.
func proofBlock(step *quaycrewv1.Step) string {
	block := ""
	if step.GetProof() != "" {
		block = labelProof + "\n" + step.GetProof()
	}
	if step.GetProofScenario() == "" {
		return block
	}
	named := "The scenario that proves it is named: " + step.GetProofScenario()
	if block == "" {
		return named
	}
	return block + "\n" + named
}

// The narrowed parts of a project: the listing, the add that gives the number, and the one line
// saying what a feature narrows to.
//
// A project delivers several features at the same time, each with its own milestones, and none of
// them waits for another. A feature carries no design and no approval of its own: those belong to
// the project, so gate 1 reads the project's design whichever feature a step sits in.

// featureTitleCap is the length past which a title is refused. A title is one line in a listing, and
// a paragraph in that column is a listing nobody can read.
//
// featureIntentionMark is the length past which an intention is long enough to say so. It refuses
// nothing: no length cap refuses text a person wrote, and the line is kept whole either way.
const (
	featureTitleCap      = 200
	featureIntentionMark = 200
)

// ListFeatures reads a project's features, or every project's when it names none.
//
// A project with no feature answers with an empty list rather than an error, because nothing written
// is the normal state. Every state comes back: filtering to the open ones is the caller's question,
// because a closed feature is what the record of the work looks like.
func (s *Server) ListFeatures(ctx context.Context, req *quaycrewv1.ListFeaturesRequest) (*quaycrewv1.ListFeaturesResponse, error) {
	features, err := s.store.ListFeatures(ctx, req.GetProject())
	if err != nil {
		return nil, storeError(err, "project")
	}
	return &quaycrewv1.ListFeaturesResponse{Features: features}, nil
}

// AddFeature gives a project one more narrowed part of itself.
//
// The number is the store's to give, in the statement that writes the row, so two adds at one moment
// cannot take the same one. Nothing here reads the highest number, because a read here followed by a
// write there is exactly the race the store's single statement exists to close.
func (s *Server) AddFeature(ctx context.Context, req *quaycrewv1.AddFeatureRequest) (*quaycrewv1.AddFeatureResponse, error) {
	if req.GetProject() == "" {
		return nil, status.Error(codes.InvalidArgument, "which project: say where with an address")
	}
	title := req.GetTitle()
	if strings.TrimSpace(title) == "" {
		return nil, status.Error(codes.InvalidArgument,
			"this feature has no title, and a feature with no title says nothing in a listing")
	}
	if length := utf8.RuneCountInString(title); length > featureTitleCap {
		return nil, status.Errorf(codes.InvalidArgument,
			"this title is %d characters, over the %d a title holds. A title is one line naming the part of the project this feature narrows to",
			length, featureTitleCap)
	}
	feature, err := s.store.AddFeature(ctx, req.GetProject(), title)
	if err != nil {
		return nil, storeError(err, "project")
	}
	return &quaycrewv1.AddFeatureResponse{Feature: feature}, nil
}

// SetFeatureIntention records which part of the project a feature narrows to.
//
// One line, so a second line is refused rather than kept: the intention is read beside the title in a
// listing, and a paragraph there is a design in the wrong column. Length is a warning and never a
// refusal, because the text only exists in the call being made.
func (s *Server) SetFeatureIntention(ctx context.Context, req *quaycrewv1.SetFeatureIntentionRequest) (*quaycrewv1.SetFeatureIntentionResponse, error) {
	if req.GetFeature() == "" {
		return nil, status.Error(codes.InvalidArgument, "which feature: say its number")
	}
	intention := req.GetIntention()
	if strings.Contains(intention, "\n") {
		return nil, status.Error(codes.InvalidArgument,
			"an intention is one line, and this one holds a line break. Say what the feature narrows to in one line, and write the rest in the design")
	}
	feature, err := s.store.SetFeatureIntention(ctx, req.GetFeature(), intention)
	if err != nil {
		return nil, storeError(err, "feature")
	}
	return &quaycrewv1.SetFeatureIntentionResponse{
		Feature: feature,
		Warnings: overMark("feature intention", utf8.RuneCountInString(intention), featureIntentionMark,
			"one line saying which part of the project the feature narrows to"),
	}, nil
}

// The three words a feature reads as. They are here rather than in the store because the store keeps
// the word it is given: one layer owns the vocabulary, so the two cannot disagree about it.
const (
	featureOpen    = "open"
	featureDone    = "done"
	featureStopped = "stopped"
)

// featureStates are the three, in the order the refusal names them.
func featureStates() []string { return []string{featureOpen, featureDone, featureStopped} }

// The two step states that mean nobody will do any more to that step. They read the same as two of
// the words above and they are a different list: a step is also ready or taken, so a step nobody
// finished is what the warning below names.
const (
	stepDone    = "done"
	stepStopped = "stopped"
)

// FinishFeature says a feature finished, or stopped, or is open again.
//
// It warns and it never refuses. A feature is closed while steps under it are ready or taken all the
// time: the work moved on, or it was abandoned, and the operator is the one who knows which. A
// refusal here would make the operator finish or stop every step of a feature nobody is working on
// before the record of it could stop growing.
//
// The warning says what closing costs, because a closed feature leaves the path document a session
// reads and a person who closes a feature and then finds a session with nothing to read would take
// the silence for a fault.
func (s *Server) FinishFeature(ctx context.Context, req *quaycrewv1.FinishFeatureRequest) (*quaycrewv1.FinishFeatureResponse, error) {
	if req.GetFeature() == "" {
		return nil, status.Error(codes.InvalidArgument, "which feature: say its number")
	}
	state := req.GetState()
	if !slices.Contains(featureStates(), state) {
		return nil, status.Errorf(codes.InvalidArgument,
			"%q is not a state a feature reads as. A feature is %s, and nothing else",
			state, strings.Join(featureStates(), ", "))
	}
	warnings := s.whatClosingCosts(ctx, req.GetFeature(), state)
	feature, err := s.store.FinishFeature(ctx, req.GetFeature(), state)
	if err != nil {
		return nil, storeError(err, "feature")
	}
	return &quaycrewv1.FinishFeatureResponse{Feature: feature, Warnings: warnings}, nil
}

// whatClosingCosts is what the operator loses by closing this feature: every step under it that
// nobody finished, and the path document the feature leaves.
//
// Reopening warns nothing. It costs nothing and it starts nothing, so a warning there would be noise
// on the one call that takes nothing away.
//
// The steps are read before the write rather than after, because the write is what the warning is
// about. A read that fails leaves the warnings empty and the write still happens: the operator asked
// for the state, and a warning that cannot be composed is not a reason to refuse one.
func (s *Server) whatClosingCosts(ctx context.Context, feature, state string) []string {
	if state == featureOpen {
		return nil
	}
	steps, err := s.store.ListSteps(ctx, feature)
	if err != nil {
		return nil
	}
	warnings := make([]string, 0, len(steps)+1)
	for _, step := range steps {
		if step.GetState() == stepDone || step.GetState() == stepStopped {
			continue
		}
		warnings = append(warnings, fmt.Sprintf("step %d is %s, and closing the feature leaves it that way: %s",
			step.GetNumber(), step.GetState(), step.GetTitle()))
	}
	return append(warnings, "this feature leaves "+designDir+"/"+pathFile+
		", so a session in this project stops reading its path. Open it again with krewe feature open.")
}
