package sandbox

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"
)

// LicensedGuests is how many macOS guests one host may run at once.
//
// Apple's licence sets this number, not the hardware. Section 2B(iii) of the macOS Tahoe 26 Software
// License Agreement grants the right "to install, use and run up to two (2) additional copies or
// instances of the Apple Software ... within virtual operating system environments on each
// Apple-branded computer you own or control that is already running the Apple Software", for
// software development, for testing during development, for macOS Server, or for personal use. The
// same section refuses the use of those instances "in connection with service bureau, time-sharing,
// terminal sharing, relay service or other similar types of services". So this pool is for the
// machines an operator owns, and a host that rents its guests out is outside the grant.
//
// The Virtualization framework holds the number as well. It refuses the third guest with
// VZErrorDomain code 6, "The maximum supported number of active virtual machines has been reached".
// A system that admitted a third session would therefore fail at boot rather than wait.
//
// Read from https://www.apple.com/legal/sla/docs/macOSTahoe.pdf on 15 September 2026.
const LicensedGuests = 2

// guestPoll is how often a waiting session looks again when the guests are all taken. A guest
// another process gave back sends no signal here, so the wait cannot be a signal alone.
const guestPoll = 2 * time.Second

// guestPool hands out the few guests a host may run, and makes the rest wait.
//
// A container provider needs none of this. A container costs a second to start and a few hundred
// megabytes, so every session gets one. A macOS guest costs tens of seconds and tens of gigabytes,
// and the licence above permits two, so a guest is a scarce resource and a session queues for it.
//
// Occupancy is read from the host on every attempt, not counted in memory. The guests outlive this
// process the way containers do: after a restart an in memory count reads zero while two guests are
// still up, and the system would then start a third and be refused by the framework.
type guestPool struct {
	// limit is how many guests may run at once, which is LicensedGuests unless an operator's own
	// agreement with Apple says another number.
	limit int
	// running answers which sessions the host holds a running guest for.
	running func(context.Context) ([]string, error)

	mu sync.Mutex
	// held is the sessions this process admitted. A guest admitted a moment ago is not running yet,
	// so the host alone would let a second session in through the gap.
	held map[string]bool
	// released nudges one waiting session when a guest goes back.
	released chan struct{}
}

func newGuestPool(limit int, running func(context.Context) ([]string, error)) *guestPool {
	if limit <= 0 {
		limit = LicensedGuests
	}
	return &guestPool{
		limit:    limit,
		running:  running,
		held:     map[string]bool{},
		released: make(chan struct{}, 1),
	}
}

// hold admits this session to a guest, and waits where every guest is taken.
//
// A session that already holds one is admitted again without taking a second. Create adopts the
// guest a session already has, and a pool that counted that adoption as a new guest would fill
// itself up with one session.
//
// The wait ends when the caller's context does, and the refusal then names the limit: a session
// waiting for a machine that is full has to be told that, rather than told the guest failed to boot.
func (p *guestPool) hold(ctx context.Context, session string) error {
	full := false
	for {
		admitted, err := p.tryHold(ctx, session)
		if err != nil {
			// A wait that ran out while the host was full is the host being full. Without this the
			// caller is told the listing failed, which is the one thing that did not happen.
			if full && ctx.Err() != nil {
				return p.refuse(ctx.Err())
			}
			return err
		}
		if admitted {
			return nil
		}
		full = true
		select {
		case <-ctx.Done():
			return p.refuse(ctx.Err())
		case <-p.released:
		case <-time.After(guestPoll):
		}
	}
}

// refuse is what a session waiting for a guest is told when it gives up. It names the number, because
// a session told only that its guest failed goes looking for the failure in the image.
func (p *guestPool) refuse(err error) error {
	return fmt.Errorf("sandbox: this host may run %d macOS guests at once and every one is taken: %w",
		p.limit, err)
}

// tryHold admits the session where the host has room, and answers false where it has none.
func (p *guestPool) tryHold(ctx context.Context, session string) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.held[session] {
		return true, nil
	}
	taken, err := p.occupancy(ctx, session)
	if err != nil {
		return false, err
	}
	if taken >= p.limit {
		return false, nil
	}
	p.held[session] = true
	return true, nil
}

// occupancy is how many guests are in use by anybody but this session: the ones the host runs, and
// the ones this process admitted and has not started yet.
func (p *guestPool) occupancy(ctx context.Context, session string) (int, error) {
	up, err := p.running(ctx)
	if err != nil {
		return 0, err
	}
	taken := make([]string, 0, len(up)+len(p.held))
	for _, one := range up {
		if one != session && !slices.Contains(taken, one) {
			taken = append(taken, one)
		}
	}
	for one := range p.held {
		if one != session && !slices.Contains(taken, one) {
			taken = append(taken, one)
		}
	}
	return len(taken), nil
}

// release gives this session's guest back, and wakes one session waiting for it.
func (p *guestPool) release(session string) {
	p.mu.Lock()
	held := p.held[session]
	delete(p.held, session)
	p.mu.Unlock()
	if !held {
		return
	}
	select {
	case p.released <- struct{}{}:
	default:
	}
}
