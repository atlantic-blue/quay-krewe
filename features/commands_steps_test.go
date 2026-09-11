package features_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/atlantic-blue/quay-krewe/internal/commands"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/cucumber/godog"
)

// Steps for the slash commands krewe installs into the operator's own terminal.
//
// They run the real tool in its own process, because what is specified is what an operator gets:
// files on their machine, a refusal on standard error, and an exit status. The directory is one this
// scenario made and named, through the variable the tool reads, so nothing here can reach the
// operator's own commands.

type commandsKey struct{}

// commandsWorld is the directory the scenario installs into, and the file it wrote by hand.
type commandsWorld struct {
	dir string
	// byHand is the path of the file the operator wrote themselves, and wrote holds what they put in
	// it, so a later step compares the file against the whole of what it said rather than its length.
	byHand string
	wrote  string
}

func commandsFrom(ctx context.Context) *commandsWorld {
	c, _ := ctx.Value(commandsKey{}).(*commandsWorld)
	return c
}

func initializeCommandSteps(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, commandsKey{}, &commandsWorld{}), nil
	})

	// The variable goes into this process's own environment, because the tool is run as a child and
	// inherits it. The scenarios run one at a time, and the value is taken back off afterwards.
	sc.Step(`^the operator's agent reads its commands from a directory of its own$`, func(ctx context.Context) error {
		// The directory itself is not made here. A machine where nothing was installed has no such
		// directory, and that is the state every operator starts in, so a scenario that made it would
		// never meet it. The install makes it, and a step writing a file by hand makes it first.
		parent, err := os.MkdirTemp("", "krewe-commands-")
		if err != nil {
			return err
		}
		dir := filepath.Join(parent, "krewe")
		if err := os.Setenv(commands.DirectoryEnv, dir); err != nil {
			return err
		}
		sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
			return ctx, errorsJoined(os.Unsetenv(commands.DirectoryEnv), os.RemoveAll(parent))
		})
		commandsFrom(ctx).dir = dir
		return nil
	})

	sc.Step(`^the operator installs the slash commands$`, func(ctx context.Context) error {
		return runTool(ctx, "commands", "install")
	})

	sc.Step(`^the operator installs the slash commands with "([^"]*)"$`, func(ctx context.Context, flag string) error {
		return runTool(ctx, "commands", "install", flag)
	})

	sc.Step(`^the operator asks where the slash commands go$`, func(ctx context.Context) error {
		return runTool(ctx, "commands")
	})

	sc.Step(`^the operator asks which slash commands this build carries$`, func(ctx context.Context) error {
		return runTool(ctx, "commands", "list")
	})

	// The install as a setup step, so a scenario about the second one reads as two installs rather
	// than as files that appeared from somewhere.
	sc.Step(`^the slash commands are installed$`, func(ctx context.Context) error {
		if err := runTool(ctx, "commands", "install"); err != nil {
			return err
		}
		if code := toolFrom(ctx).exitCode; code != 0 {
			return fmt.Errorf("the first install exited %d, saying %q", code, toolFrom(ctx).stderr)
		}
		return nil
	})

	sc.Step(`^the operator wrote "([^"]*)" in that directory by hand$`, func(ctx context.Context, name string) error {
		held := commandsFrom(ctx)
		held.wrote = "a command of my own, which krewe never wrote\n"
		held.byHand = filepath.Join(held.dir, name)
		if err := os.MkdirAll(held.dir, 0o755); err != nil {
			return err
		}
		return os.WriteFile(held.byHand, []byte(held.wrote), 0o644)
	})

	sc.Step(`^the directory holds "([^"]*)" written by build "([^"]*)"$`,
		func(ctx context.Context, name, build string) error {
			held := commandsFrom(ctx)
			if err := os.MkdirAll(held.dir, 0o755); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(held.dir, name),
				[]byte(commands.Marker(build)+"\n---\ndescription: an older build wrote this\n---\n"), 0o644)
		})

	sc.Step(`^the directory holds every command this build carries$`, func(ctx context.Context) error {
		for _, one := range commands.All() {
			at := filepath.Join(commandsFrom(ctx).dir, one.FileName())
			if _, err := os.Stat(at); err != nil {
				return fmt.Errorf("%s is not there: %w", at, err)
			}
		}
		return nil
	})

	sc.Step(`^every command in the directory names this build$`, func(ctx context.Context) error {
		builds, err := commands.Builds(commandsFrom(ctx).dir)
		if err != nil {
			return err
		}
		if len(builds) != 1 || builds[0] != toolBuild {
			return fmt.Errorf("the directory holds %v, want only %q", builds, toolBuild)
		}
		return nil
	})

	sc.Step(`^it names where every command went$`, func(ctx context.Context) error {
		said := toolFrom(ctx).stdout
		for _, one := range commands.All() {
			at := filepath.Join(commandsFrom(ctx).dir, one.FileName())
			if !strings.Contains(said, at) {
				return fmt.Errorf("the install never said where %s went: %q", one.FileName(), said)
			}
		}
		return nil
	})

	sc.Step(`^it says how many it wrote$`, func(ctx context.Context) error {
		want := fmt.Sprintf("%d command", len(commands.All()))
		return says("standard output", toolFrom(ctx).stdout, want)
	})

	sc.Step(`^it names the file the operator wrote$`, func(ctx context.Context) error {
		return says("standard error", toolFrom(ctx).stderr, commandsFrom(ctx).byHand)
	})

	sc.Step(`^it names the command directory$`, func(ctx context.Context) error {
		return says("standard error", toolFrom(ctx).stderr, commandsFrom(ctx).dir)
	})

	sc.Step(`^the file the operator wrote says what it said before$`, func(ctx context.Context) error {
		held := commandsFrom(ctx)
		body, err := os.ReadFile(held.byHand)
		if err != nil {
			return fmt.Errorf("the file the operator wrote is gone: %w", err)
		}
		if string(body) != held.wrote {
			return fmt.Errorf("the file the operator wrote says %q now, and said %q", body, held.wrote)
		}
		return nil
	})

	// Nothing at all, which is what the check running before the first write is for. The file the
	// operator wrote is left out, because the step above is what holds that one to what it said.
	sc.Step(`^nothing else was written into that directory$`, func(ctx context.Context) error {
		held := commandsFrom(ctx)
		entries, err := os.ReadDir(held.dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if filepath.Join(held.dir, entry.Name()) == held.byHand {
				continue
			}
			return fmt.Errorf("%s was written even though the install was refused", entry.Name())
		}
		return nil
	})

	// A directory that was never made holds nothing, which is the state a read command leaves behind
	// on a machine where nothing was installed.
	sc.Step(`^nothing at all was written into that directory$`, func(ctx context.Context) error {
		entries, err := os.ReadDir(commandsFrom(ctx).dir)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if len(entries) != 0 {
			return fmt.Errorf("a read command wrote %d files into %s", len(entries), commandsFrom(ctx).dir)
		}
		return nil
	})

	sc.Step(`^it prints this build twice$`, func(ctx context.Context) error {
		if count := strings.Count(toolFrom(ctx).stdout, toolBuild); count != 2 {
			return fmt.Errorf("standard output names %q %d times, want twice:\n%s",
				toolBuild, count, toolFrom(ctx).stdout)
		}
		return nil
	})

	sc.Step(`^it never says to install again$`, func(ctx context.Context) error {
		if strings.Contains(toolFrom(ctx).stdout, "Run krewe commands install") {
			return fmt.Errorf("it says to install again over its own build:\n%s", toolFrom(ctx).stdout)
		}
		return nil
	})

	sc.Step(`^it names every command this build carries, with what each one does$`, func(ctx context.Context) error {
		said := toolFrom(ctx).stdout
		for _, one := range commands.All() {
			if err := says("standard output", said, one.Slash()); err != nil {
				return err
			}
			if err := says("standard output", said, one.Description); err != nil {
				return err
			}
		}
		return nil
	})

	// What one command file says for itself, read out of the file the install put on the machine
	// rather than out of the binary. The rules that hold over every file are read in the package; a
	// command that has to say a particular thing is read here.

	sc.Step(`^the installed command "([^"]*)" carries the marker of this build$`,
		func(ctx context.Context, name string) error {
			body, err := installedCommand(ctx, name)
			if err != nil {
				return err
			}
			line, _, _ := strings.Cut(body, "\n")
			build, marked := commands.BuildOf(line)
			if !marked {
				return fmt.Errorf("%s.md begins %q, and that is no marker", name, line)
			}
			if build != toolBuild {
				return fmt.Errorf("%s.md names build %q, and this tool is %q", name, build, toolBuild)
			}
			return nil
		})

	sc.Step(`^the installed command "([^"]*)" describes itself in one line$`,
		func(ctx context.Context, name string) error {
			body, err := installedCommand(ctx, name)
			if err != nil {
				return err
			}
			for _, line := range strings.Split(body, "\n") {
				if after, found := strings.CutPrefix(strings.TrimSpace(line), "description:"); found {
					if strings.TrimSpace(after) == "" {
						break
					}
					return nil
				}
			}
			return fmt.Errorf("%s.md carries no description, so the listing would name it and say nothing", name)
		})

	sc.Step(`^the installed command "([^"]*)" names "([^"]*)"$`,
		func(ctx context.Context, name, phrase string) error {
			body, err := installedCommand(ctx, name)
			if err != nil {
				return err
			}
			if !strings.Contains(body, phrase) {
				return fmt.Errorf("%s.md never names %q", name, phrase)
			}
			return nil
		})

	// The design work belongs to a session in a sandbox, where the record keeps it. These are the
	// commands that put a design or a path into the record, and a command file naming one of them is
	// the operator's own terminal doing the work the record is supposed to hold.
	sc.Step(`^the installed command "([^"]*)" runs no command that writes a design or a path$`,
		func(ctx context.Context, name string) error {
			body, err := installedCommand(ctx, name)
			if err != nil {
				return err
			}
			for _, writing := range []string{
				"krewe design set", "krewe design edit", "krewe design contracts", "krewe path set",
			} {
				if strings.Contains(body, writing) {
					return fmt.Errorf("%s.md runs %q, and a command never writes the design or the path",
						name, writing)
				}
			}
			return nil
		})

	// The other shape of the same break: the design or the path written as prose in the file itself,
	// with no command anywhere near it. These labels are what a path document is made of.
	sc.Step(`^the installed command "([^"]*)" carries no design document of its own$`,
		func(ctx context.Context, name string) error {
			body, err := installedCommand(ctx, name)
			if err != nil {
				return err
			}
			for _, label := range []string{
				"What changes and why", "What this touches", "What proves it",
				"The scenario that proves it", "The contracts it builds", "The scope of each contract",
			} {
				if strings.Contains(body, label) {
					return fmt.Errorf("%s.md carries %q, which is a path document written where no record keeps it",
						name, label)
				}
			}
			return nil
		})

	// Beside the command, and not anywhere in the file. A yes asked three steps earlier, for
	// something else, is not the operator approving this design.
	sc.Step(`^the installed command "([^"]*)" asks for a yes where it runs "([^"]*)"$`,
		func(ctx context.Context, name, command string) error {
			body, err := installedCommand(ctx, name)
			if err != nil {
				return err
			}
			for _, section := range strings.Split(body, "\n## ") {
				if !strings.Contains(section, command) {
					continue
				}
				if !strings.Contains(section, "yes") {
					return fmt.Errorf("%s.md runs %q in a step that never asks for a yes:\n%s",
						name, command, section)
				}
				return nil
			}
			return fmt.Errorf("%s.md never names %q", name, command)
		})

	sc.Step(`^the installed command "([^"]*)" says a no leaves the design unapproved$`,
		func(ctx context.Context, name string) error {
			body, err := installedCommand(ctx, name)
			if err != nil {
				return err
			}
			if !strings.Contains(body, "stays unapproved") {
				return fmt.Errorf("%s.md never says what a no leaves behind", name)
			}
			return nil
		})

	sc.Step(`^standard output names "([^"]*)" before "([^"]*)"$`,
		func(ctx context.Context, first, second string) error {
			said := toolFrom(ctx).stdout
			at, then := strings.Index(said, first), strings.Index(said, second)
			switch {
			case at < 0:
				return fmt.Errorf("standard output never names %q:\n%s", first, said)
			case then < 0:
				return fmt.Errorf("standard output never names %q:\n%s", second, said)
			case at > then:
				return fmt.Errorf("standard output names %q before %q:\n%s", second, first, said)
			}
			return nil
		})

	// The system puts the session identifier in every sandbox it builds, so the tool reads it to know
	// it is inside one. It is cleared for every other run, because the machine running this suite may
	// be a session itself.
	sc.Step(`^the tool is running inside a session.s sandbox$`, func(ctx context.Context) error {
		toolFrom(ctx).env = append(toolFrom(ctx).env, sandbox.SessionIDEnv+"=4f2c91aa6b0e3d7c85a11b20")
		return nil
	})
}

// errorsJoined keeps the first thing that went wrong while a scenario tidies up after itself.
func errorsJoined(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// installedCommand is one command file as the install wrote it onto the machine. It reads the
// directory this scenario named, so what is read is what the operator would open.
func installedCommand(ctx context.Context, name string) (string, error) {
	at := filepath.Join(commandsFrom(ctx).dir, name+".md")
	body, err := os.ReadFile(at) //nolint:gosec // the path is this scenario's own directory
	if err != nil {
		return "", fmt.Errorf("read what the install wrote: %w", err)
	}
	return string(body), nil
}
