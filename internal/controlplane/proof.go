package controlplane

import (
	"context"
	"fmt"
	"log/slog"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
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
