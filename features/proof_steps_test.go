package features_test

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/cucumber/godog"
)

// The steps about the word krewe hands the shell. A scenario name holding a quote cannot be written
// between quotes, so the name and the command come as a block rather than as a phrase.
func initializeProofCommandSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a step taken, restated and approved, naming the scenario:$`,
		func(ctx context.Context, name *godog.DocString) error {
			return aStepReadyToCheck(ctx, name.Content, true)
		})

	sc.Step(`^the operator sets the project's proof command to:$`,
		func(ctx context.Context, command *godog.DocString) error {
			return setProof(ctx, command.Content, "", 0)
		})

	// The command these scenarios prove with prints its arguments rather than running a test, so the
	// sandbox matches its answer on the program rather than on a flag of a runner.
	sc.Step(`^the run reports one scenario$`, func(ctx context.Context) error {
		return theRunAnswers(ctx, sandbox.Reply{Match: theWordPrinter, Out: "1 scenarios (1 passed)"})
	})

	// A real shell, because the shell is what reads a name as code. The scenario runs the command krewe
	// composed, exactly as krewe handed it to the sandbox, and reads the name back out of it.
	sc.Step(`^a shell given what the run was given prints the scenario name as one word$`,
		func(ctx context.Context) error {
			spec, err := theRunOfTheProof(ctx)
			if err != nil {
				return err
			}
			name := pathFrom(ctx).checked.GetStep().GetProofScenario()
			if name == "" {
				return fmt.Errorf("the step names no scenario, so there is no name to read back")
			}
			out, err := exec.Command(spec.Argv[0], spec.Argv[1:]...).Output()
			if err != nil {
				return fmt.Errorf("the shell refused %q: %w", strings.Join(spec.Argv, " "), err)
			}
			// One pair of brackets around each argument the shell handed the program. A name that
			// arrived as one word prints in one pair, and a name the shell split prints in several.
			if want := "[" + name + "]"; string(out) != want {
				return fmt.Errorf("the shell read %q out of %q, want %q",
					out, strings.Join(spec.Argv, " "), want)
			}
			return nil
		})
}

// theWordPrinter is the program the proof command of these scenarios runs. It prints its arguments, so
// what the shell made of the name is readable from outside the shell.
const theWordPrinter = "printf"

// theRunOfTheProof is the exec krewe ran to check the step, as krewe built it. A session's sandbox is
// given its own setup when it starts, so the proof run is picked out by the program it runs rather than
// by being the last thing in the list.
func theRunOfTheProof(ctx context.Context) (sandbox.Spec, error) {
	box, err := theSandboxOfTheStep(ctx)
	if err != nil {
		return sandbox.Spec{}, err
	}
	for _, spec := range box.Ran {
		if strings.Contains(strings.Join(spec.Argv, " "), theWordPrinter) {
			return spec, nil
		}
	}
	return sandbox.Spec{}, fmt.Errorf("no run of the proof command is in %d commands the sandbox was given",
		len(box.Ran))
}
