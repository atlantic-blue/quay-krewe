package features_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/display"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/cucumber/godog"
)

// Steps for the tree of names.
//
// Each one walks from a name to the mount a container is given, rather than from a name to a path
// this file builds. A link assembled correctly and pointing at a directory nothing binds is exactly
// the failure the tree exists to prevent, and a step that read the link would pass against it.

func initializeNameSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a file called "([^"]*)" is dropped into the name "([^"]*)"$`,
		func(ctx context.Context, file, named string) error {
			w := worldFrom(ctx)
			at := filepath.Join(w.storage.NameTree, filepath.FromSlash(named))
			if err := os.WriteFile(filepath.Join(at, file), []byte("what the session needs"), 0o666); err != nil {
				return fmt.Errorf("drop a file into %q: %w", at, err)
			}
			return nil
		})

	sc.Step(`^the name "([^"]*)" holds the folder "([^"]*)"$`,
		func(ctx context.Context, named, folder string) error {
			w := worldFrom(ctx)
			at := filepath.Join(w.storage.NameTree, filepath.FromSlash(named), folder)
			info, err := os.Stat(at)
			if err != nil {
				return fmt.Errorf("%q is not there: %w", at, err)
			}
			if !info.IsDir() {
				return fmt.Errorf("%q is not a directory", at)
			}
			return nil
		})

	sc.Step(`^a session is dispatched with the title "([^"]*)"$`, func(ctx context.Context, title string) error {
		w := worldFrom(ctx)
		resp, err := w.client.Dispatch(ctx, &quaycrewv1.DispatchRequest{
			Project: w.projectID, Text: "look at the login", Title: title,
		})
		if err != nil {
			return fmt.Errorf("dispatch: %w", err)
		}
		w.execs = append(w.execs, dispatched{
			sessionID: resp.GetId(), handle: resp.GetHandle(), reply: resp.GetReply(),
		})
		return nil
	})

	// The half a link cannot prove about itself: that the directory it points at is the one this
	// session's own container is given, at the mount point a session is told to read.
	sc.Step(`^a sandbox of that session reads that file at "([^"]*)"$`, func(ctx context.Context, at string) error {
		w := worldFrom(ctx)
		current, err := w.lastExec()
		if err != nil {
			return err
		}
		session, err := w.client.GetSession(ctx, &quaycrewv1.GetSessionRequest{Id: current.sessionID})
		if err != nil {
			return fmt.Errorf("read the session back: %w", err)
		}
		mounts, err := w.storage.Prepare(sandbox.Config{
			ID:        session.GetSession().GetId(),
			Workspace: session.GetSession().GetWorkspace(),
			Project:   session.GetSession().GetProject(),
		})
		if err != nil {
			return fmt.Errorf("what a sandbox is given: %w", err)
		}
		for _, one := range mounts {
			rest, inside := strings.CutPrefix(at, one.Target+"/")
			if !inside {
				continue
			}
			read := filepath.Join(one.Source, filepath.FromSlash(rest))
			if _, err := os.Stat(read); err != nil {
				return fmt.Errorf("a sandbox reads %q at %q, and there is no file at %q: %w",
					one.Source, one.Target, read, err)
			}
			return nil
		}
		return fmt.Errorf("no mount of that session holds %q, and it is given %v", at, sources(mounts))
	})

	// A generated identifier is what the tree exists to keep off the screen, so this refuses one
	// anywhere in it. An empty tree fails as well: a name that was never written and a tree with no
	// identifier in it read the same otherwise.
	sc.Step(`^the tree of names holds no identifier$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		var held int
		err := filepath.WalkDir(w.storage.NameTree, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if path == w.storage.NameTree {
				return nil
			}
			held++
			if display.LooksLikeIdentifier(entry.Name()) {
				return fmt.Errorf("the tree holds %q, which is an identifier", path)
			}
			// A name is a link, and following it would walk the whole data directory.
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			return nil
		})
		if err != nil {
			return err
		}
		if held == 0 {
			return fmt.Errorf("the tree at %q is empty, so it says nothing about identifiers", w.storage.NameTree)
		}
		return nil
	})
}
