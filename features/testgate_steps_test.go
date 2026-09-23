package features_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

// The test gate, run the way the model runtime runs it: the entry point this build ships, fed a
// PreToolUse payload on standard input, answering with an exit code.
//
// It runs the real binary rather than calling the hook's own code, because the hook is a separate
// module and the system cannot import it. The gate reads the session's own directory for the mark,
// so every firing here is pointed at the directory the control plane wrote, and nothing is told
// whether the session is building.

// theMarkFile is where the system says a session is building, under the session's own directory. The
// hook holds the same two names, and a change to either leaves this failing rather than passing over
// a file nobody writes.
const theMarkFile = ".krewe/building"

func initializeTestGateSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the session holding the step is under the test gate$`, func(ctx context.Context) error {
		return theMarkIs(ctx, true)
	})

	sc.Step(`^the session holding the step is not under the test gate$`, func(ctx context.Context) error {
		return theMarkIs(ctx, false)
	})

	sc.Step(`^a session holding no step is not under the test gate$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		if err := w.dispatch(ctx, w.projectID, "", "read the brief"); err != nil {
			return err
		}
		if w.lastErr != nil {
			return w.lastErr
		}
		dir, err := sessionWorkingDir(ctx)
		if err != nil {
			return err
		}
		if _, err := os.Stat(filepath.Join(dir, theMarkFile)); err == nil {
			return fmt.Errorf("a session holding no step carries the mark at %s, "+
				"so the stage that writes the tests is refused as well", dir)
		}
		if err := fireTestGate(ctx, dir, "Write",
			map[string]string{"file_path": "features/brief_test.go"}); err != nil {
			return err
		}
		return theTestGateRefused(ctx, false)
	})

	sc.Step(`^the test gate refuses that session a write to "([^"]*)"$`,
		func(ctx context.Context, where string) error {
			if err := theGateReads(ctx, "Write", map[string]string{"file_path": where}); err != nil {
				return err
			}
			return theTestGateRefused(ctx, true)
		})

	sc.Step(`^the test gate allows that session a write to "([^"]*)"$`,
		func(ctx context.Context, where string) error {
			if err := theGateReads(ctx, "Write", map[string]string{"file_path": where}); err != nil {
				return err
			}
			return theTestGateRefused(ctx, false)
		})

	sc.Step(`^the test gate refuses that session the command "([^"]*)"$`,
		func(ctx context.Context, command string) error {
			if err := theGateReads(ctx, "Bash", map[string]string{"command": command}); err != nil {
				return err
			}
			return theTestGateRefused(ctx, true)
		})

	sc.Step(`^the test gate allows that session the command "([^"]*)"$`,
		func(ctx context.Context, command string) error {
			if err := theGateReads(ctx, "Bash", map[string]string{"command": command}); err != nil {
				return err
			}
			return theTestGateRefused(ctx, false)
		})

	sc.Step(`^the refusal says to answer that the test is wrong instead$`, func(ctx context.Context) error {
		said := worldFrom(ctx).testGate.said
		for _, needed := range []string{"say so in your answer", "name the file"} {
			if !strings.Contains(said, needed) {
				return fmt.Errorf("the refusal never says %q, so the session tries the next spelling "+
					"of the same thing:\n%s", needed, said)
			}
		}
		return nil
	})
}

// theStepSessionDir is the working directory of the session that took the step, as this process sees
// it. It is read from what the take answered rather than from the last exec, because a scenario that
// starts a second session moves the last exec and never moves the step.
func theStepSessionDir(ctx context.Context) (string, error) {
	w, p := worldFrom(ctx), pathFrom(ctx)
	if p.take == nil {
		return "", errors.New("no step was taken, so no session holds one")
	}
	return filepath.Join(w.storage.Dir, "workspaces", w.workspaceID,
		"projects", w.projectID, "sessions", p.take.GetSession().GetId(), "workspace"), nil
}

// theMarkIs says whether the session holding the step carries the mark, read off the disk where the
// sandbox would read it.
func theMarkIs(ctx context.Context, want bool) error {
	dir, err := theStepSessionDir(ctx)
	if err != nil {
		return err
	}
	at := filepath.Join(dir, theMarkFile)
	_, err = os.Stat(at)
	switch {
	case err == nil && want:
		return nil
	case err == nil:
		return fmt.Errorf("the session carries the mark at %s, so it is under the gate and "+
			"cannot write the tests it was asked for", at)
	case os.IsNotExist(err) && !want:
		return nil
	case os.IsNotExist(err):
		return fmt.Errorf("nothing marks %s, so the session may weaken the tests it watched fail", at)
	default:
		return err
	}
}

// theGateReads fires the shipped gate over one tool call, with the session holding the step as the
// directory the session is working in.
func theGateReads(ctx context.Context, tool string, input map[string]string) error {
	dir, err := theStepSessionDir(ctx)
	if err != nil {
		return err
	}
	return fireTestGate(ctx, dir, tool, input)
}

// fireTestGate runs the shipped entry point over one payload and records what it said.
//
// The entry point is made absolute because the run happens in the session's own directory, which is
// what the gate reads to find the mark, and the loader answers with a path relative to this package.
func fireTestGate(ctx context.Context, dir, tool string, input map[string]string) error {
	entry, err := shippedEntry("test-gate")
	if err != nil {
		return err
	}
	if entry, err = filepath.Abs(entry); err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{
		"tool_name": tool, "cwd": dir, "tool_input": input,
	})
	if err != nil {
		return err
	}
	run := exec.CommandContext(ctx, entry)
	run.Dir = dir
	run.Stdin = strings.NewReader(string(payload))
	// The variable the gate still reads is cleared, so nothing here can pass on the old way on.
	run.Env = append(os.Environ(), "KREWE_BUILDING=")
	var said strings.Builder
	run.Stderr = &said

	answer := gateAnswer{}
	switch err := run.Run(); {
	case err == nil:
	case isExit(err, 2):
		answer.refused = true
	default:
		return fmt.Errorf("running %s: %w\n%s", entry, err, said.String())
	}
	answer.said = said.String()
	worldFrom(ctx).testGate = answer
	return nil
}

// theTestGateRefused holds the last firing to what the scenario says it should have answered.
func theTestGateRefused(ctx context.Context, want bool) error {
	answer := worldFrom(ctx).testGate
	switch {
	case answer.refused == want:
		return nil
	case want:
		return errors.New("the gate let it through, and the suite is the only thing holding the requirement")
	default:
		return fmt.Errorf("the gate refused work this session was asked to do: %s", answer.said)
	}
}
