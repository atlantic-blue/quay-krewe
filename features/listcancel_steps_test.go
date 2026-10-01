package features_test

import (
	"context"
	"errors"
	"fmt"
	"sync"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/store"
	"github.com/cucumber/godog"
)

// sessionsAskedFor is how many sessions a caller waits for before it gives up. It is small against
// the number in the system, so a listing that read every one of them is not mistaken for one that
// stopped.
const sessionsAskedFor = 3

// countedSessionReads is the store under a scenario about a caller that goes away. It counts the
// reads a listing makes one session at a time, and the read the caller goes on.
//
// The conversation store is a struct rather than an interface, so nothing can count the transcripts a
// listing reads. The store is the one per session read a double can see, and the same loop makes
// both, so counting one of them says whether the loop stopped.
type countedSessionReads struct {
	store.Store
	mu       sync.Mutex
	sessions int
	// waitedFor is the session whose read takes the caller away. Zero is a caller that stays.
	waitedFor int
	gone      func()
}

func (c *countedSessionReads) SessionSkills(ctx context.Context, id string) (string, error) {
	born, err := c.Store.SessionSkills(ctx, id)
	c.mu.Lock()
	c.sessions++
	leaving := c.waitedFor > 0 && c.sessions == c.waitedFor && c.gone != nil
	c.mu.Unlock()
	if leaving {
		c.gone()
	}
	return born, err
}

// goesOn arms the caller to leave on the read it waited for, and forgets what the scenario read
// setting itself up.
func (c *countedSessionReads) goesOn(leave func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessions, c.gone = 0, leave
}

func (c *countedSessionReads) read() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessions
}

// Steps for the scenarios about a call whose caller has gone.
//
// They drive the control plane object rather than the world's client. A cancel sent over the
// transport reaches the server after a round trip, so a count taken on the other side of it would be
// a race, and a race here reads as the system doing the right thing about half the time.
func initializeListCancelSteps(sc *godog.ScenarioContext) {
	sc.Step(`^(\d+) sessions in the system$`, func(ctx context.Context, count int) error {
		w := worldFrom(ctx)
		for i := range count {
			_, _, err := w.store.FindOrCreateSession(ctx, w.projectID,
				fmt.Sprintf("session-%d", i), store.Birth{})
			if err != nil {
				return fmt.Errorf("put session %d in the system: %w", i, err)
			}
		}
		return nil
	})

	sc.Step(`^a caller that gives up part of the way through the list$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		w.sessionReads = &countedSessionReads{Store: w.store, waitedFor: sessionsAskedFor}
		return w.restart()
	})

	sc.Step(`^the operator asks for the session list$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		asking, gone := context.WithCancel(ctx)
		defer gone()
		if w.sessionReads != nil {
			w.sessionReads.goesOn(gone)
		}
		w.lastList, w.lastErr = w.server.ListSessions(asking,
			&quaycrewv1.ListSessionsRequest{Project: w.projectID})
		return nil
	})

	sc.Step(`^the list comes back saying its caller went$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		if !errors.Is(w.lastErr, context.Canceled) {
			return fmt.Errorf("the list answered %v, and the caller had gone", w.lastErr)
		}
		if held := len(w.lastList.GetSessions()); held != 0 {
			return fmt.Errorf("the list hands back %d sessions to a caller that is not there", held)
		}
		return nil
	})

	sc.Step(`^the system read no more sessions after the caller went$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		if w.sessionReads == nil {
			return fmt.Errorf("no caller in this scenario ever went, so nothing here is measured")
		}
		read := w.sessionReads.read()
		if read == 0 {
			return fmt.Errorf("the list read no session at all, so the caller never went")
		}
		if read > sessionsAskedFor {
			return fmt.Errorf("the list read %d sessions, and its caller went after %d", read, sessionsAskedFor)
		}
		return nil
	})

	sc.Step(`^the list holds every one of the (\d+) sessions$`, func(ctx context.Context, want int) error {
		w := worldFrom(ctx)
		if w.lastErr != nil {
			return fmt.Errorf("the list answered %v, and its caller was waiting for it", w.lastErr)
		}
		if held := len(w.lastList.GetSessions()); held != want {
			return fmt.Errorf("the list holds %d sessions, want the %d in the system", held, want)
		}
		return nil
	})
}
