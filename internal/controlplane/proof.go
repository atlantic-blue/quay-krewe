package controlplane

import (
	"context"
	"log/slog"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
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
	// The store clears the approval on every write, because it cannot tell an unchanged text from a
	// rewritten one that reads the same. So an unchanged text is not written at all: the session
	// renders its own restatement back on every exec, and a call here would clear the operator's
	// approval on each of them.
	if text == held.GetRestatement() {
		return
	}
	if len(text) > restatementMark {
		slog.Warn("a session restated its step at length",
			"session", session.GetId(), "step", held.GetNumber(),
			"characters", len(text), "long enough to say so past", restatementMark)
	}
	if _, err := s.store.SetRestatement(ctx, held.GetFeature(), held.GetNumber(), text); err != nil {
		slog.Warn("what the session restated was not recorded on its step",
			"session", session.GetId(), "step", held.GetNumber(), "error", err)
	}
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
