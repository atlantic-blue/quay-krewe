package features_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/cucumber/godog"
)

// Steps for the scarcity of a macOS guest.
//
// They ask the sandbox package how many guests one host hands out, and what a session that already
// holds one is given. The guests are driven through a stand in for tart rather than through Apple's
// Virtualization framework, which needs an Apple machine, so these scenarios prove the queue and they
// do not prove that macOS boots. Which backend a word selects is sandboxbackends.feature.

// macOSWorld is the host a scenario stood up, and what it answered.
type macOSWorld struct {
	provider *sandbox.MacOSProvider
	home     string
	held     []string
	waited   time.Duration
	refusal  error
	first    string
	again    string
}

type macOSKey struct{}

func macOSFrom(ctx context.Context) *macOSWorld {
	w, _ := ctx.Value(macOSKey{}).(*macOSWorld)
	return w
}

func initializeMacOSSandboxSteps(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, macOSKey{}, &macOSWorld{}), nil
	})

	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		w := macOSFrom(ctx)
		if w == nil || w.provider == nil {
			return ctx, nil
		}
		for _, session := range w.held {
			_ = w.provider.Remove(context.Background(), session)
		}
		_ = os.RemoveAll(w.home)
		return ctx, os.Unsetenv(fakeTartHome)
	})

	sc.Step(`^a host running the macOS sandbox$`, func(ctx context.Context) error {
		w := macOSFrom(ctx)
		tart, err := standInForTart()
		if err != nil {
			return err
		}
		home, err := os.MkdirTemp("", "krewe-guests")
		if err != nil {
			return err
		}
		if err := os.Setenv(fakeTartHome, home); err != nil {
			return err
		}
		w.home = home
		w.provider = &sandbox.MacOSProvider{Image: "macos-base", Tart: tart}
		return nil
	})

	sc.Step(`^(\d+) sessions already hold a guest$`, func(ctx context.Context, count int) error {
		for i := range count {
			if err := holdAGuest(ctx, fmt.Sprintf("00000000000000000000000%d", i)); err != nil {
				return err
			}
		}
		return nil
	})

	sc.Step(`^a session already holds a guest$`, func(ctx context.Context) error {
		w := macOSFrom(ctx)
		if err := holdAGuest(ctx, "0000000000000000000000a0"); err != nil {
			return err
		}
		w.first = nameOf(w.provider, "0000000000000000000000a0")
		return nil
	})

	sc.Step(`^a third session asks for one$`, func(ctx context.Context) error {
		w := macOSFrom(ctx)
		// Bounded, because the point is that it waits: a session refused at once is a session that
		// was never queued.
		waiting, stop := context.WithTimeout(ctx, 2*time.Second)
		defer stop()
		started := time.Now()
		_, w.refusal = w.provider.Create(waiting, sandbox.Config{ID: "0000000000000000000000f3"})
		w.waited = time.Since(started)
		return nil
	})

	sc.Step(`^it waits, and the refusal names the number this host may run$`, func(ctx context.Context) error {
		w := macOSFrom(ctx)
		if w.refusal == nil {
			return fmt.Errorf("a third guest was handed out, and Apple's framework refuses to start it")
		}
		if w.waited < time.Second {
			return fmt.Errorf("the session was turned away after %s rather than queued", w.waited)
		}
		want := fmt.Sprintf("%d macOS guests", sandbox.LicensedGuests)
		if !strings.Contains(w.refusal.Error(), want) {
			return fmt.Errorf("the refusal is %q, and it has to say the host is full", w.refusal)
		}
		return nil
	})

	sc.Step(`^that session asks for one again$`, func(ctx context.Context) error {
		w := macOSFrom(ctx)
		box, err := w.provider.Create(ctx, sandbox.Config{ID: "0000000000000000000000a0"})
		if err != nil {
			return fmt.Errorf("asking for a guest a second time: %w", err)
		}
		named, says := box.(sandbox.Named)
		if !says {
			return fmt.Errorf("the guest does not say what it is called, so an operator cannot reach it")
		}
		w.again = named.Name()
		return nil
	})

	sc.Step(`^it is given the guest it already has$`, func(ctx context.Context) error {
		w := macOSFrom(ctx)
		if w.again != w.first {
			return fmt.Errorf("it was given %q, and it already had %q", w.again, w.first)
		}
		return nil
	})

	sc.Step(`^the host still runs (\d+) guest$`, func(ctx context.Context, want int) error {
		w := macOSFrom(ctx)
		running, err := w.provider.Stranded(ctx)
		if err != nil {
			return err
		}
		if len(running) != want {
			return fmt.Errorf("the host runs %d guests (%v), want %d: a second one costs tens of gigabytes and one of two places",
				len(running), running, want)
		}
		return nil
	})
}

// holdAGuest gives one session a guest and remembers it, so the scenario gives it back afterwards.
func holdAGuest(ctx context.Context, session string) error {
	w := macOSFrom(ctx)
	if _, err := w.provider.Create(ctx, sandbox.Config{ID: session}); err != nil {
		return fmt.Errorf("giving session %s a guest: %w", session, err)
	}
	if !contains(w.held, session) {
		w.held = append(w.held, session)
	}
	return nil
}

func contains(all []string, one string) bool {
	for _, each := range all {
		if each == one {
			return true
		}
	}
	return false
}

// nameOf is what the runtime calls this session's guest.
func nameOf(provider *sandbox.MacOSProvider, session string) string {
	box, found, err := provider.Existing(context.Background(), session)
	if err != nil || !found {
		return ""
	}
	if named, says := box.(sandbox.Named); says {
		return named.Name()
	}
	return ""
}

// fakeTartHome is where the stand in keeps the machines it pretends to run.
const fakeTartHome = "FAKE_TART_HOME"

// The stand in is a program, so it is built once for the whole suite.
var standIn struct {
	once sync.Once
	path string
	err  error
}

// standInForTart builds the program that answers for tart, because tart itself needs an Apple
// machine and this suite runs on whatever the pipeline has.
func standInForTart() (string, error) {
	standIn.once.Do(func() {
		dir, err := os.MkdirTemp("", "faketart")
		if err != nil {
			standIn.err = err
			return
		}
		standIn.path = filepath.Join(dir, "tart")
		out, err := exec.Command("go", "build", "-o", standIn.path,
			"github.com/atlantic-blue/quay-krewe/internal/sandbox/sandboxtest/faketart").CombinedOutput()
		if err != nil {
			standIn.err = fmt.Errorf("building the stand in for tart: %w: %s", err, out)
		}
	})
	return standIn.path, standIn.err
}
