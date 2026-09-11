package controlplane

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// What a session says about the step it holds, before it builds anything.
//
// The session is given the step and told to write no code. What it writes back, under the
// restatement mark in its own memory file, is the evidence that it read the step the way the step
// was meant. The operator reads that and agrees to it, or answers the session, and only then does any
// code exist to review.
//
// The text travels through the session's memory file rather than through a call, because a model
// writes files and cannot make one. It goes into the step so that one thing holds it: the file is
// rendered from the store on every exec, so a session that lost its file gets its own words back.

// restatementMark is the length past which a restatement is long enough to say so. Six named parts
// about one step fit well inside it, and a document pasted in does not.
//
// It refuses nothing. No length refuses text a person or a session wrote, and the text is kept whole
// either way.
const restatementMark = 2_000

// readRestatement takes what the session wrote under the restatement mark and puts it on the step
// that session holds.
//
// Nothing here fails an exec. A write that fails is logged, and the session still holds the text in
// its own file, so the next exec reads it again and writes it again.
func (s *Server) readRestatement(ctx context.Context, session *quaycrewv1.Session, text string) {
	if text == "" {
		return
	}
	// A restatement with no step is noise: a session an operator dispatched into the project wrote
	// under a mark that means nothing to it. The text is dropped rather than filed somewhere.
	held, _ := s.stepThisSessionHolds(ctx, session)
	if held == nil {
		return
	}
	s.putRestatement(ctx, held, text)
}

// putRestatement records what was written about a step and answers the step as it stands afterwards.
//
// Two callers write a restatement, and they find the step two ways: an exec has a session and asks
// which step it holds, and a read has the step and asks which session holds it. What they do with
// the text is one thing, and it is here.
//
// The store clears the approval on every write, because it cannot tell an unchanged text from a
// rewritten one that reads the same. So an unchanged text is not written at all: the session renders
// its own restatement back on every exec, and a call here would clear the operator's approval on each
// of them.
//
// A write that fails is logged and answers with the step as it was. The session keeps the text in its
// own file, so the next read finds it again.
func (s *Server) putRestatement(ctx context.Context, held *quaycrewv1.Step, text string) *quaycrewv1.Step {
	if text == "" || text == held.GetRestatement() {
		return held
	}
	if len(text) > restatementMark {
		slog.Warn("a session restated its step at length",
			"session", held.GetSession(), "feature", held.GetFeature(), "step", held.GetNumber(),
			"characters", len(text), "mark", restatementMark)
	}
	written, err := s.store.SetRestatement(ctx, held.GetFeature(), held.GetNumber(), text)
	if err != nil {
		slog.Warn("what the session restated was not recorded on its step",
			"session", held.GetSession(), "feature", held.GetFeature(), "step", held.GetNumber(),
			"error", err)
		return held
	}
	return written
}

// renderRestatement is the section the inner memory file carries while a session is on a step, and
// the empty string when it carries none. Compose drops an empty section.
//
// renderContext writes the whole inner file from the store on every exec, so a section it does not
// render is a section that disappears. That is what this exists for: the session reads back what it
// wrote about its step, from the store, however many execs later.
//
// It stops once the step is done or stopped. After that the work is over, and the text would be read
// again on every exec of a session that has moved on to something else. That is one condition and it
// lives in stepThisSessionHolds, which answers with a step in state taken and with nothing else, so a
// finished step is nil here rather than a second state check written out again.
func (s *Server) renderRestatement(ctx context.Context, session *quaycrewv1.Session) string {
	held, _ := s.stepThisSessionHolds(ctx, session)
	if held == nil {
		return ""
	}
	return held.GetRestatement()
}

// GetStep's read: the session's own file, put onto the step before the step is answered.

// theFileWasNotRead is what the operator is told when the session's own file could not be read. It
// is a warning and never a refusal: a session that has gone still leaves the last text it wrote
// behind it, and refusing the read would take that away as well.
const theFileWasNotRead = "the session's own file could not be read, " +
	"so this is the text the step already held"

// refreshRestatement puts what the session wrote into the step, then answers the step as it stands
// and what the operator has to be told about the answer.
//
// The read costs one file read on this machine. A session's working directory is mounted into its
// sandbox, so this process and that container look at one place, and nothing here starts a
// container, wakes a session or spends a token. That is what lets the operator read a restatement
// the moment it is written rather than at the next exec.
//
// Nothing here is an error. Every way the read can fail leaves the step as the store has it and adds
// a warning, because the text the session wrote before it went is the text the operator is about to
// read.
func (s *Server) refreshRestatement(ctx context.Context, held *quaycrewv1.Step) (*quaycrewv1.Step, []string) {
	// A step that is over is answered from the store, and its file is not read at all. The session
	// may be gone, and what a finished step was restated as no longer changes: reading the file
	// would let a session that carried on writing move the record of work that is closed.
	if held.GetState() == stepDone || held.GetState() == stepStopped {
		return held, nil
	}
	// A step nobody holds has no session and no file. That is a step in state ready, which is most of
	// a path, and it is answered from the store with nothing to say about it.
	if held.GetSession() == "" {
		return held, nil
	}
	feature, err := s.store.GetFeature(ctx, held.GetFeature())
	if err != nil {
		return held, []string{theFileWasNotRead}
	}
	session, err := s.sessionAt(ctx, "", feature.GetProject(), held.GetSession())
	if err != nil {
		return held, []string{theFileWasNotRead}
	}
	dirs := s.storage.MyDirs(boxOf(session))
	if len(dirs) != 2 {
		return held, []string{theFileWasNotRead}
	}
	body, found := sandbox.ReadMemory(dirs[innerFile])
	if !found {
		return held, []string{theFileWasNotRead}
	}
	written := sandbox.Decompose(body, marksOfTheInnerFile(session))
	return s.putRestatement(ctx, held, written[sandbox.RestatementScope]), nil
}

// marksOfTheInnerFile is what a session's own memory file is decomposed against: the sections the
// system renders into it, and then the levels of context it carries.
//
// The order is what makes it right. Text under a mark this build does not know belongs to the last
// scope given, so a level is last and the restatement never is: read with the restatement last, a
// file would answer with the whole of itself as a restatement.
func marksOfTheInnerFile(session *quaycrewv1.Session) []string {
	levels := contextFiles(session)[innerFile]
	marks := make([]string, 0, len(levels)+3)
	marks = append(marks, sandbox.SkillsScope, sandbox.DesignScope, sandbox.RestatementScope)
	for _, level := range levels {
		marks = append(marks, string(level.scope))
	}
	return marks
}

// tooLongToBeSix says a restatement is past the length six parts about one step take, and says
// nothing about one that is not.
//
// It refuses nothing and it hides nothing. The text is answered whole either way, and the warning
// prints under it, because no length refuses what a person or a session wrote.
func tooLongToBeSix(step *quaycrewv1.Step) []string {
	if len(step.GetRestatement()) <= restatementMark {
		return nil
	}
	return []string{fmt.Sprintf(
		"this restatement is %d characters, past the %d that six parts about one step take. "+
			"It is kept whole", len(step.GetRestatement()), restatementMark)}
}

// Running the scenario the step promised, and the verdict that comes of it.
//
// Nothing here starts a model and nothing spends a token. It is an exec in a container that already
// exists, and what it reads back is an exit status and a count. Whether the scenario describes the
// value the step promised is not a question krewe can answer: the operator answers it when the
// operator approves the design.

// proofOutputRead is how much of a run's output krewe reads, keeping the end. A run that prints
// without stopping cannot then take the control plane with it, and the end is where a runner prints
// its count and its reason.
//
// It is larger than what a step keeps, because the count is read out of this and the last of it is
// what the step then carries.
const proofOutputRead = 64 << 10

// CheckStep runs the scenario one step names, inside the sandbox of the session holding that step,
// and records what the run reported.
//
// The order is PROOF-1's, and every refusal in it comes before anything runs: a step that names no
// scenario, a restatement nobody approved, and a project with no proof command each cost one line of
// output and start nothing.
//
// A failing run is not one of those refusals. It is a verdict, and it comes back on the step with the
// count and the last of the output, because a caller prints a verdict rather than a refusal.
func (s *Server) CheckStep(ctx context.Context, req *quaycrewv1.CheckStepRequest) (
	*quaycrewv1.CheckStepResponse, error) {
	if req.GetFeature() == "" {
		return nil, status.Error(codes.InvalidArgument, "which feature: a step belongs to one, so say its number")
	}
	if req.GetNumber() < 1 {
		return nil, status.Error(codes.InvalidArgument, "a step number counts from one")
	}
	// The feature says which project this is, which is what carries the proof command. A step is
	// addressed by its feature, and one design belongs to the whole project.
	feature, err := s.store.GetFeature(ctx, req.GetFeature())
	if err != nil {
		return nil, storeError(err, "feature")
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
	design, err := s.store.GetDesign(ctx, feature.GetProject())
	if err != nil {
		return nil, storeError(err, "project")
	}
	if why := whyNothingCanRun(held, design); why != nil {
		return nil, refusalToRun(why, held)
	}

	result, err := s.runTheScenario(ctx, feature.GetProject(), held, design)
	if err != nil {
		return nil, err
	}
	written, err := s.store.RecordProof(ctx, req.GetFeature(), req.GetNumber(), result)
	if err != nil {
		return nil, storeError(err, "step")
	}
	// The design is read again after the write, because the trust record moves with a verdict once
	// the ladder exists and a caller reads the two together.
	return &quaycrewv1.CheckStepResponse{Step: written, Design: design}, nil
}

// whyNothingCanRun is the rule this step and this project fail, and nil where a run may go ahead.
//
// The three are sentinels rather than sentences, so one place names each rule and the sentence a
// person reads is built from it below.
func whyNothingCanRun(held *quaycrewv1.Step, design *quaycrewv1.Design) error {
	if held.GetProofScenario() == "" {
		return store.ErrNoScenarioNamed
	}
	if !held.GetRestatementApproved() {
		return store.ErrRestatementNotApproved
	}
	if design.GetProofCommand() == "" {
		return store.ErrNoProofCommand
	}
	return nil
}

// refusalToRun is the sentence for each of those rules, and what to type about it.
//
// Each one names the next move rather than only what is wrong. A step that names no scenario is
// answered in the path document, a restatement is approved with a command, and a project sets a proof
// command with another, so none of these leaves a person reading the manual to find out what to do.
func refusalToRun(why error, held *quaycrewv1.Step) error {
	switch {
	case errors.Is(why, store.ErrNoScenarioNamed):
		return status.Errorf(codes.FailedPrecondition,
			"step %d names no scenario, so there is nothing to run. "+
				"Name one under %q in the path document, then write it again with krewe path set",
			held.GetNumber(), labelScenario)
	case errors.Is(why, store.ErrRestatementNotApproved):
		return status.Errorf(codes.FailedPrecondition,
			"nobody approved step %d's restatement, so it built nothing yet. "+
				"Read what the session wrote with krewe step restatement, "+
				"and approve it with krewe step approve", held.GetNumber())
	default:
		return status.Error(codes.FailedPrecondition,
			"this project has no proof command, so krewe cannot run anything. "+
				"Set one with krewe design proof [<address>] \"<command>\"")
	}
}

// runTheScenario runs the step's scenario in the sandbox of the session that holds it, and turns what
// came back into a verdict.
//
// The sandbox is the one the session already has. Nothing is created, nothing is copied and nothing
// is cloned: the run reads the working directory the session has been writing in, which is what makes
// the check cost a container exec and nothing else.
func (s *Server) runTheScenario(ctx context.Context, project string, held *quaycrewv1.Step,
	design *quaycrewv1.Design) (store.ProofResult, error) {
	// By handle or by identifier, because the step records whichever the take wrote and a session
	// answers to both. This is the read krewe step restatement already makes to find the same session.
	session, err := s.sessionAt(ctx, "", project, held.GetSession())
	if err != nil {
		return store.ProofResult{}, err
	}
	box, running, err := s.provider.Existing(ctx, session.GetId())
	if err != nil {
		return store.ProofResult{}, status.Errorf(codes.Internal,
			"the container holding step %d could not be reached: %v", held.GetNumber(), err)
	}
	if !running {
		return store.ProofResult{}, status.Errorf(codes.Internal,
			"the session holding step %d has no container, so there is nothing to run the scenario in",
			held.GetNumber())
	}

	command := strings.ReplaceAll(design.GetProofCommand(), scenarioToken, held.GetProofScenario())
	budget := time.Duration(design.GetProofTimeoutSeconds()) * time.Second
	if budget <= 0 {
		budget = time.Duration(store.DefaultProofTimeoutSeconds) * time.Second
	}
	under, stop := context.WithTimeout(ctx, budget)
	defer stop()

	// A shell, because a proof command is a command line rather than a program and its arguments: it
	// carries quotes, pipes and a scenario name with spaces in it.
	spec := sandbox.Spec{Argv: []string{"sh", "-c", command}, Workdir: s.whereTheWorkIs(session)}
	started, err := box.Exec(under, spec)
	if err != nil {
		// A command the shell cannot start is a verdict and not an error. Nothing about the step is
		// wrong, the run said nothing, and a count of zero never passes.
		return failedRun(fmt.Sprintf("the run could not start: %v", err)), nil
	}
	printed := theEndOfTheRun(started.Stdout())
	ran := started.Wait()
	if said := started.Stderr(); said != "" {
		printed = strings.TrimRight(printed, "\n") + "\n" + said
	}
	if errors.Is(under.Err(), context.DeadlineExceeded) {
		return failedRun(fmt.Sprintf("%s\n\nthe run passed its budget of %d seconds and was stopped",
			strings.TrimRight(printed, "\n"), int(budget/time.Second))), nil
	}
	return verdictOf(ran, printed, design.GetProofCountPattern()), nil
}

// whereTheWorkIs is the directory inside the container that the run is pointed at: the repository the
// session worked in where there is one, and the session's own directory where there is not.
//
// It is the root ReadSessionWork reads from, named as the container sees it rather than as this
// process does, because the command runs in there. A system that keeps no directories falls back to
// the path a sandbox mounts its working directory at, which is where the session has been working
// whatever this process can see of it.
func (s *Server) whereTheWorkIs(session *quaycrewv1.Session) string {
	places := s.storage.WorkPlaces(boxOf(session))
	if len(places) == 0 {
		return sandbox.WorkingPath
	}
	if found, held := sandbox.Repository(places); held {
		return found.Sandbox
	}
	return places[0].Sandbox
}

// theEndOfTheRun is the last proofOutputRead characters the run printed, read to the end.
//
// Everything is read and the front is thrown away, rather than the reader being cut short: a reader
// nobody drains stops the command dead as soon as the pipe fills, and a run stopped by the thing
// watching it reports a failure that never happened.
func theEndOfTheRun(stream io.Reader) string {
	var kept strings.Builder
	buffer := make([]byte, 32<<10)
	for {
		read, err := stream.Read(buffer)
		if read > 0 {
			kept.Write(buffer[:read])
			if kept.Len() > proofOutputRead {
				held := kept.String()
				kept.Reset()
				kept.WriteString(held[len(held)-proofOutputRead:])
			}
		}
		if err != nil {
			return kept.String()
		}
	}
}

// verdictOf is what the run reported: the state, the count, and the output with anything krewe has to
// say about it underneath.
//
// Passing takes all three: the run exited zero, the pattern matched, and the count is above zero. The
// count is what makes this worth reading, because a name filter that matches nothing prints success
// in most runners, and a check that read the exit status alone would report a passing verdict on a
// scenario that never ran.
func verdictOf(ran error, printed, pattern string) store.ProofResult {
	count, said := scenariosIn(printed, pattern)
	result := store.ProofResult{State: store.ProofFailing, ScenariosRun: count, Output: printed}
	if said != "" {
		result.Output = strings.TrimRight(printed, "\n") + "\n\n" + said
	}
	if ran == nil && count > 0 {
		result.State = store.ProofPassing
	}
	return result
}

// failedRun is a verdict about a run that never reported anything: no scenarios, and one line saying
// what happened instead.
func failedRun(said string) store.ProofResult {
	return store.ProofResult{State: store.ProofFailing, Output: said}
}

// scenariosIn reads how many scenarios the run reported, and what krewe has to say where it could not
// read one.
//
// Every way of failing to read a count answers zero, and zero never passes. The sentence beside it is
// what stops that reading as a scenario that ran and failed: the run may have passed perfectly and
// printed its count in a shape this project's pattern does not match.
func scenariosIn(printed, pattern string) (int32, string) {
	if pattern == "" {
		return 0, "this project has no count pattern, so krewe read no count from this run"
	}
	reader, err := regexp.Compile(pattern)
	if err != nil {
		return 0, fmt.Sprintf("the count pattern %q does not compile, so krewe read no count: %v",
			pattern, err)
	}
	found := reader.FindStringSubmatch(printed)
	if len(found) < 2 {
		return 0, fmt.Sprintf("the pattern %q found no count in this output, "+
			"so nothing is known to have run", pattern)
	}
	count, err := strconv.Atoi(found[1])
	if err != nil {
		return 0, fmt.Sprintf("the pattern %q matched %q, which is not a number", pattern, found[1])
	}
	return int32(count), ""
}
