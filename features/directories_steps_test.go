package features_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/cucumber/godog"
)

// Steps for the command that turns an address into a directory.
//
// They run the real tool in its own process, because the shape of the answer is what is specified: a
// path alone on the first line is what makes `cd "$(krewe where acme)"` work, and a line that is one
// line inside the test process can be two on the caller's screen.
//
// The directory is then checked against what a sandbox binds, read from the same call the container
// runtime is given, rather than against a path this file builds. A path assembled correctly and
// mounted nowhere is exactly the failure this scenario exists to catch.

func initializeDirectorySteps(sc *godog.ScenarioContext) {
	sc.Step(`^the caller asks where "([^"]*)" is$`, func(ctx context.Context, address string) error {
		return runTool(ctx, "where", address)
	})

	sc.Step(`^the directory it names exists on the machine$`, func(ctx context.Context) error {
		named := theDirectoryNamed(ctx)
		info, err := os.Stat(named)
		if err != nil {
			return fmt.Errorf("the command named %q, which is not on the machine: %w", named, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("the command named %q, which is not a directory", named)
		}
		return nil
	})

	sc.Step(`^it says a session reads that directory at "([^"]*)"$`, func(ctx context.Context, mount string) error {
		return says("standard output", toolFrom(ctx).stdout, mount)
	})

	sc.Step(`^the first line is a path and nothing else$`, func(ctx context.Context) error {
		first := theDirectoryNamed(ctx)
		if first == "" {
			return fmt.Errorf("the command said nothing")
		}
		if strings.ContainsAny(first, " \t") {
			return fmt.Errorf("the first line is %q, so it cannot be typed into a shell", first)
		}
		if !strings.HasPrefix(first, "/") {
			return fmt.Errorf("the first line is %q, want an absolute path", first)
		}
		return nil
	})

	sc.Step(`^a file called "([^"]*)" is put in that directory by hand$`, func(ctx context.Context, name string) error {
		named := theDirectoryNamed(ctx)
		return os.WriteFile(filepath.Join(named, name), []byte("a picture"), 0o666)
	})

	// The half a printed path cannot prove about itself: that a sandbox binds this directory, and at
	// the mount point the answer promised.
	sc.Step(`^a sandbox of that workspace binds that directory at "([^"]*)"$`, func(ctx context.Context, mount string) error {
		w := worldFrom(ctx)
		mounts, err := w.storage.Prepare(sandbox.Config{
			ID: "a-session", Workspace: w.workspaceID, Project: "a-project",
		})
		if err != nil {
			return fmt.Errorf("what a sandbox is given: %w", err)
		}
		named := theDirectoryNamed(ctx)
		for _, one := range mounts {
			if one.Source == named {
				if one.Target != mount {
					return fmt.Errorf("a sandbox binds %q at %q, and the answer said %q", named, one.Target, mount)
				}
				return nil
			}
		}
		return fmt.Errorf("the command named %q, and a sandbox binds %v", named, sources(mounts))
	})

	sc.Step(`^the file is inside the directory that sandbox binds$`, func(ctx context.Context) error {
		named := theDirectoryNamed(ctx)
		entries, err := os.ReadDir(named)
		if err != nil {
			return fmt.Errorf("read %q: %w", named, err)
		}
		if len(entries) == 0 {
			return fmt.Errorf("%q is empty, so the file went somewhere else", named)
		}
		return nil
	})

	// A project folder is not a mount of its own, it is a name inside the shared one, so the proof
	// walks from the mount the container runtime is given down to the file. A step that checked the
	// printed path instead would pass against a folder nothing reads.
	sc.Step(`^a sandbox of that workspace reads that file at "([^"]*)"$`, func(ctx context.Context, at string) error {
		_, err := whatASandboxReadsAt(ctx, at)
		return err
	})

	sc.Step(`^the directory it names holds the folder "([^"]*)"$`, func(ctx context.Context, folder string) error {
		named := theDirectoryNamed(ctx)
		info, err := os.Stat(filepath.Join(named, folder))
		if err != nil {
			return fmt.Errorf("%q does not hold %q: %w", named, folder, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%q holds %q, which is not a directory", named, folder)
		}
		return nil
	})

	sc.Step(`^the caller makes a project called "([^"]*)"$`, func(ctx context.Context, project string) error {
		return runTool(ctx, "project", "create", worldFrom(ctx).workspaceName+"/"+project)
	})

	// The shape the git skill teaches first: the checkout goes in the workspace's volume, under the
	// session's own identifier, and its `.git` is a file rather than a directory. It is written here
	// rather than cloned for real, because a scenario about which directory an address names has no
	// business needing a network.
	sc.Step(`^that session takes a working tree holding a checkout called "([^"]*)"$`,
		func(ctx context.Context, repository string) error {
			w := worldFrom(ctx)
			current, err := w.lastExec()
			if err != nil {
				return err
			}
			volume, held := w.storage.VolumeDir(w.workspaceID)
			if !held {
				return fmt.Errorf("this system keeps no volume, so no session in it can take a working tree")
			}
			checkout := filepath.Join(volume, "worktrees", current.sessionID, repository)
			if err := os.MkdirAll(checkout, 0o777); err != nil {
				return err
			}
			gitdir := "gitdir: /home/agent/shared/repos/" + repository + "/.git/worktrees/" + current.sessionID
			return os.WriteFile(filepath.Join(checkout, ".git"), []byte(gitdir+"\n"), 0o600)
		})

	sc.Step(`^the caller asks where that session is$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		current, err := w.lastExec()
		if err != nil {
			return err
		}
		return runTool(ctx, "where", w.workspaceName+"/"+w.projectName+"/"+current.handle)
	})

	sc.Step(`^it says the directory is that session's working tree$`, func(ctx context.Context) error {
		current, err := worldFrom(ctx).lastExec()
		if err != nil {
			return err
		}
		return says("standard output", toolFrom(ctx).stdout, "the working tree of session "+current.handle)
	})

	// The half a printed path cannot prove about itself, for the working tree: that a sandbox of this
	// session binds a directory holding it, and that the mount the answer promised is where the same
	// directory turns up inside the container. A step that rebuilt the path would pass against a
	// directory nothing reads.
	sc.Step(`^that session reads the directory it names at the mount the answer promised$`,
		func(ctx context.Context) error {
			w := worldFrom(ctx)
			current, err := w.lastExec()
			if err != nil {
				return err
			}
			mounts, err := w.storage.Prepare(sandbox.Config{
				ID: current.sessionID, Workspace: w.workspaceID, Project: w.projectID,
			})
			if err != nil {
				return fmt.Errorf("what a sandbox is given: %w", err)
			}
			named := theDirectoryNamed(ctx)
			for _, one := range mounts {
				rest, inside := strings.CutPrefix(named, one.Source+string(filepath.Separator))
				if !inside {
					continue
				}
				return says("standard output", toolFrom(ctx).stdout, one.Target+"/"+filepath.ToSlash(rest))
			}
			return fmt.Errorf("no sandbox mount holds %q, and a sandbox is given %v", named, sources(mounts))
		})

	// The other half of the sentence. The directory a session address used to answer with is empty, so
	// naming it would have said this session made nothing.
	sc.Step(`^that session's own directory holds nothing$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		current, err := w.lastExec()
		if err != nil {
			return err
		}
		own, held := w.storage.WorkingDir(sandbox.Config{
			ID: current.sessionID, Workspace: w.workspaceID, Project: w.projectID,
		})
		if !held {
			return fmt.Errorf("this system keeps no working directory, so there is none to be empty")
		}
		entries, err := os.ReadDir(own)
		if err != nil {
			return fmt.Errorf("read %q: %w", own, err)
		}
		if len(entries) != 0 {
			return fmt.Errorf("the session's own directory %q holds %d entries, so it is not the empty one",
				own, len(entries))
		}
		return nil
	})
}

// theDirectoryNamed is the path the command printed, which is its whole first line.
func theDirectoryNamed(ctx context.Context) string {
	line, _, _ := strings.Cut(toolFrom(ctx).stdout, "\n")
	return strings.TrimSpace(line)
}

// sources is what a sandbox would bind, for a failure that says what was there instead.
func sources(mounts []sandbox.Mount) []string {
	out := make([]string, 0, len(mounts))
	for _, one := range mounts {
		out = append(out, one.Source+" -> "+one.Target)
	}
	return out
}

// whatASandboxReadsAt is the path on the machine behind a path inside a container.
//
// It walks from the mount the container runtime is given down to the file, rather than reading a path
// the tool printed. A step that checked the printed path instead would pass against a folder nothing
// reads. It answers with the path so a step that has to open the file can, and it fails where there
// is nothing to open.
func whatASandboxReadsAt(ctx context.Context, at string) (string, error) {
	read, err := whereASandboxReads(ctx, at)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(read); err != nil {
		return "", fmt.Errorf("a sandbox reads that path at %q, and there is no file there: %w", read, err)
	}
	return read, nil
}

// whereASandboxReads is the same walk, and it stops at the path rather than at the file.
//
// A step about a file that is gone needs the two halves apart. Reading "there is no file" off a
// failure of the whole walk would pass just as well against a mount that was never made, which is the
// one thing every scenario here is standing on.
func whereASandboxReads(ctx context.Context, at string) (string, error) {
	w := worldFrom(ctx)
	mounts, err := w.storage.Prepare(sandbox.Config{
		ID: "a-session", Workspace: w.workspaceID, Project: "a-project",
	})
	if err != nil {
		return "", fmt.Errorf("what a sandbox is given: %w", err)
	}
	for _, one := range mounts {
		rest, inside := strings.CutPrefix(at, one.Target+"/")
		if !inside {
			continue
		}
		return filepath.Join(one.Source, filepath.FromSlash(rest)), nil
	}
	return "", fmt.Errorf("no sandbox mount holds %q, and a sandbox is given %v", at, sources(mounts))
}
