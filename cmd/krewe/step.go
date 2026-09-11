package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/display"
	"github.com/atlantic-blue/quay-krewe/internal/workspace"
)

// The steps of a path, one at a time. Taking one starts a session on it.
//
// The tool composes none of the text a session is given. It sends the feature and the number, and
// prints what came back, so the console and the command line ask for the same words.

const stepUsage = "usage: krewe step take [<address>] <feature>.<number>" +
	"\n       krewe step restatement [<address>] <feature>.<number>" +
	"\n       krewe step approve [<address>] <feature>.<number>" +
	"\n       krewe step check [<address>] <feature>.<number>" +
	"\n       krewe step done [<address>] <feature>.<number> \"<result>\"" +
	"\n       krewe step stop [<address>] <feature>.<number> \"<reason>\""

func runStep(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient, args []string, out io.Writer) error {
	if len(args) > 0 && args[0] == "take" {
		return runStepTake(ctx, client, args[1:], out)
	}
	if len(args) > 0 && args[0] == "restatement" {
		return runStepRestatement(ctx, client, args[1:], out)
	}
	if len(args) > 0 && args[0] == "approve" {
		return runStepApprove(ctx, client, args[1:], out)
	}
	if len(args) > 0 && args[0] == "check" {
		return runStepCheck(ctx, client, args[1:], out)
	}
	if len(args) > 0 && (args[0] == "done" || args[0] == "stop") {
		return runStepFinish(ctx, client, args[0], args[1:], out)
	}
	return fmt.Errorf("%s", stepUsage)
}

// runStepTake gives one step to a session that starts now.
//
// With one argument the argument is the step, and with two the first is the address. This is the
// shape krewe design brief already has, so an operator standing in a project types the step alone.
//
// It lets go of the exec, so closing the terminal does not take the work with it. The output names
// the session, and attaching is a separate command whenever the operator wants it.
func runStepTake(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient, args []string, out io.Writer) error {
	if len(args) == 0 || len(args) > 2 {
		return fmt.Errorf("%s", stepUsage)
	}
	typed, said := "", args[0]
	if len(args) == 2 {
		typed, said = args[0], args[1]
	}
	located, err := designProject(ctx, client, typed)
	if err != nil {
		return err
	}
	features, err := featuresOf(ctx, client, located.ProjectID)
	if err != nil {
		return err
	}
	held, number, err := stepAddressed(said, features, located.Path.Project)
	if err != nil {
		return err
	}
	resp, err := client.TakeStep(ctx, &quaycrewv1.TakeStepRequest{
		Feature: held.GetId(), Number: number,
	})
	if err != nil {
		return fmt.Errorf("%w\n\nnothing was started", err)
	}
	step := resp.GetStep()
	fmt.Fprintf(out, "step %d.%d of %s is taken: %s\n",
		held.GetNumber(), step.GetNumber(), located.Path.Project, step.GetTitle())
	fmt.Fprintf(out, "(session %s, handle %s)\n\n",
		resp.GetSession().GetId(), resp.GetSession().GetHandle())
	fmt.Fprintf(out, "it was asked to:\n\n%s\n", strings.TrimRight(resp.GetText(), "\n"))
	// What the session does next, said once here, because the take text above is long and an
	// operator who reads only the first lines of it still has to know that no code is coming yet.
	fmt.Fprint(out, "\nit will restate the step and build nothing\n")
	sayWarnings(out, resp.GetWarnings())
	// Both numbers are the control plane's, read off the response. Counting the steps here would put
	// a second count of one thing in a second place, and the two would disagree the moment a take
	// landed between this call and that one.
	fmt.Fprintf(out, "\n%d of %d steps in flight\n", resp.GetInFlight(), resp.GetStepsInFlightCap())
	return nil
}

// runStepRestatement prints what the session wrote about the step it holds.
//
// The read refreshes first, in the control plane: it reads the session's own file, so what prints is
// what the session understands now rather than what it understood at its last exec. Nothing is
// dispatched, so this costs no token and starts no container, and the operator reads the text the
// moment the session writes it.
//
// The text prints last and whole, so the output can be piped. A warning prints under it and never in
// place of it: a note about the length of a text is not a reason to withhold the text.
func runStepRestatement(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient, args []string, out io.Writer) error {
	if len(args) == 0 || len(args) > 2 {
		return fmt.Errorf("usage: krewe step restatement [<address>] <feature>.<number>")
	}
	typed, said := "", args[0]
	if len(args) == 2 {
		typed, said = args[0], args[1]
	}
	located, err := designProject(ctx, client, typed)
	if err != nil {
		return err
	}
	features, err := featuresOf(ctx, client, located.ProjectID)
	if err != nil {
		return err
	}
	held, number, err := stepAddressed(said, features, located.Path.Project)
	if err != nil {
		return err
	}
	resp, err := client.GetStep(ctx, &quaycrewv1.GetStepRequest{Feature: held.GetId(), Number: number})
	if err != nil {
		return err
	}
	step := resp.GetStep()
	fmt.Fprintf(out, "step %d.%d of %s: %s\n",
		held.GetNumber(), step.GetNumber(), located.Path.Project, step.GetTitle())
	if step.GetRestatement() == "" {
		fmt.Fprintf(out, "\nthis session wrote no restatement yet\n")
		fmt.Fprintf(out, "%s\n", waitOrAskFor(located, step))
		sayWarnings(out, resp.GetWarnings())
		return nil
	}
	fmt.Fprintf(out, "written %s\n", step.GetRestatedAt().AsTime().Format(whenItWasWritten))
	fmt.Fprintf(out, "approval: %s\n", restatementApproval(step))
	fmt.Fprintf(out, "\n%s\n", strings.TrimRight(step.GetRestatement(), "\n"))
	sayWarnings(out, resp.GetWarnings())
	return nil
}

// runStepApprove says the word on what the session wrote, which is what starts the build.
//
// It approves the text as it stands. It opens no editor and asks no question, the way krewe design
// approve does not: the operator has already read the text with krewe step restatement, and a command
// that asked again would be asking somebody to agree to something twice.
//
// When the restatement is wrong the answer is not here. The operator answers that session with krewe
// exec, and what it writes back clears the approval, which is why the last line says so.
//
// The build text prints whole, under the session, so the operator reads what the session was asked to
// do without a second command.
func runStepApprove(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient, args []string, out io.Writer) error {
	if len(args) == 0 || len(args) > 2 {
		return fmt.Errorf("usage: krewe step approve [<address>] <feature>.<number>")
	}
	typed, said := "", args[0]
	if len(args) == 2 {
		typed, said = args[0], args[1]
	}
	located, err := designProject(ctx, client, typed)
	if err != nil {
		return err
	}
	features, err := featuresOf(ctx, client, located.ProjectID)
	if err != nil {
		return err
	}
	held, number, err := stepAddressed(said, features, located.Path.Project)
	if err != nil {
		return err
	}
	resp, err := client.ApproveRestatement(ctx, &quaycrewv1.ApproveRestatementRequest{
		Feature: held.GetId(), Number: number,
	})
	if err != nil {
		return fmt.Errorf("%w\n\nnothing was approved and nothing was started", err)
	}
	step := resp.GetStep()
	fmt.Fprintf(out, "the restatement of step %d.%d of %s is approved: %s\n",
		held.GetNumber(), step.GetNumber(), located.Path.Project, step.GetTitle())
	fmt.Fprintf(out, "(session %s, handle %s)\n\n",
		resp.GetSession().GetId(), resp.GetSession().GetHandle())
	fmt.Fprintf(out, "it was asked to:\n\n%s\n", strings.TrimRight(resp.GetText(), "\n"))
	// Said here because the approval is about one text. An operator who answers the session after this
	// has a step nobody has agreed to again, and nothing else would tell them.
	fmt.Fprint(out, "\na restatement written after this clears the approval\n")
	return nil
}

// runStepCheck runs the scenario the step promised and prints what the run reported.
//
// It prints the command before it waits, so the operator reads what runs rather than watching a
// command they cannot see. The line is composed here from the step and the design, the way krewe
// design proof already composes one, because a template is not what runs.
//
// A failing verdict is printed and not refused: the run happened and it said something, and the
// operator reads the count and the end of the output. The command still exits non zero, so a script
// reads the verdict without parsing the page.
//
// It waits for the run. A proof run takes as long as the project's suite takes, and this says so
// while it waits.
func runStepCheck(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient, args []string, out io.Writer) error {
	if len(args) == 0 || len(args) > 2 {
		return fmt.Errorf("usage: krewe step check [<address>] <feature>.<number>")
	}
	typed, said := "", args[0]
	if len(args) == 2 {
		typed, said = args[0], args[1]
	}
	located, err := designProject(ctx, client, typed)
	if err != nil {
		return err
	}
	features, err := featuresOf(ctx, client, located.ProjectID)
	if err != nil {
		return err
	}
	held, number, err := stepAddressed(said, features, located.Path.Project)
	if err != nil {
		return err
	}
	sayWhatWillRun(ctx, client, located.ProjectID, held.GetId(), number, out)

	resp, err := client.CheckStep(ctx, &quaycrewv1.CheckStepRequest{Feature: held.GetId(), Number: number})
	if err != nil {
		return fmt.Errorf("%w\n\nnothing was run and nothing was recorded", err)
	}
	step := resp.GetStep()
	fmt.Fprintf(out, "\nstep %d.%d of %s: %s\n",
		held.GetNumber(), step.GetNumber(), located.Path.Project, step.GetTitle())
	fmt.Fprintf(out, "verdict: %s, %s ran\n", step.GetProofState(), display.Scenarios(step.GetProofScenariosRun()))
	sayWarnings(out, resp.GetWarnings())
	if output := strings.TrimRight(step.GetProofOutput(), "\n"); output != "" {
		fmt.Fprintf(out, "\n%s\n", output)
	}
	if step.GetProofState() != proofPassing {
		// The reason is on the screen above this line, so nothing prints it again underneath. What is
		// left to say is the exit status, which is what a script reads.
		return fmt.Errorf("%w: step %d.%d did not pass", ErrSaid, held.GetNumber(), step.GetNumber())
	}
	return nil
}

// proofPassing is the one verdict that is not a failure. The word is the control plane's and it is
// read off the wire, so it is named here rather than compared inline.
const proofPassing = "passing"

// sayWhatWillRun prints the command this check is about to run, with the step's own scenario name
// where the token is.
//
// It says nothing where there is nothing to compose: a step that names no scenario, or a project with
// no proof command, is refused by the call underneath and the refusal says what to do about it. So
// this holds no gate of its own, and a rule it repeated would be a second place for the rule to live.
//
// Nothing here fails the check. A read that fails leaves the operator without the line and with the
// verdict, which is the answer they typed the command for.
func sayWhatWillRun(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient,
	project, feature string, number int32, out io.Writer) {
	design, err := client.GetDesign(ctx, &quaycrewv1.GetDesignRequest{Project: project})
	if err != nil {
		return
	}
	command := design.GetDesign().GetProofCommand()
	read, err := client.GetStep(ctx, &quaycrewv1.GetStepRequest{Feature: feature, Number: number})
	if err != nil || command == "" || read.GetStep().GetProofScenario() == "" {
		return
	}
	fmt.Fprintf(out, "the check runs:\n  %s\n",
		strings.ReplaceAll(command, scenarioToken, read.GetStep().GetProofScenario()))
	fmt.Fprintf(out, "\nthis waits for the run, and starts no model\n")
	sayWhatTheWaitCosts(ctx, client, project, read.GetStep(), out)
}

// sayWhatTheWaitCosts prints the extra the operator is about to wait for: the container krewe starts
// when the session holding the step was reclaimed.
//
// It prints before the wait rather than after it. The whole reason the line exists is that the
// operator is about to wait longer than a check usually takes, and a sentence that arrives with the
// verdict explains a wait that is already over.
//
// The control plane says the same thing in a warning underneath the verdict, because the two answer
// different moments. This line is read from the session's status before the call. That warning is
// what the run actually cost.
//
// Nothing here fails the check. A read that fails leaves the operator with one line fewer.
func sayWhatTheWaitCosts(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient,
	project string, step *quaycrewv1.Step, out io.Writer) {
	if step.GetSession() == "" {
		return
	}
	sessions, err := client.ListSessions(ctx, &quaycrewv1.ListSessionsRequest{Project: project})
	if err != nil {
		return
	}
	for _, session := range sessions.GetSessions() {
		// By handle or by identifier, because the step records whichever the take wrote. It is the
		// match the control plane makes to find the same session.
		if session.GetHandle() != step.GetSession() && session.GetId() != step.GetSession() {
			continue
		}
		if session.GetStatus() != statusReclaimed {
			return
		}
		fmt.Fprintf(out, "\nthe session holding this step was reclaimed, "+
			"so krewe starts a container for it first\nthis takes longer than the run does\n")
		return
	}
}

// statusReclaimed is a session the system took the container back from. The word is the control
// plane's and it is read off the wire, so it is named here rather than compared inline.
const statusReclaimed = "reclaimed"

// whenItWasWritten is how a moment prints here, and it is the one krewe design already prints an
// whenItWasWritten is how a moment prints here, and it is the one krewe design already prints an
// approval at: the operator reads the two on one screen.
const whenItWasWritten = "2006-01-02 15:04"

// waitOrAskFor is what to do about a step whose session has written nothing yet. A session may still
// be reading, so waiting is the first answer, and the second names the session to ask.
//
// It is a sentence with a command in it rather than a refusal, because nothing went wrong: a step
// taken a moment ago has no restatement yet, and that is the state every taken step starts in.
func waitOrAskFor(located workspace.Location, step *quaycrewv1.Step) string {
	if step.GetSession() == "" {
		return "wait until somebody takes the step, and the session it starts writes one"
	}
	return fmt.Sprintf("wait for it, or ask that session with krewe exec %s/%s/%s \"restate the step\"",
		located.Path.Workspace, located.Path.Project, step.GetSession())
}

// restatementApproval says where this exact text stands with the operator. It is a statement about
// one text and never about the step, so a step whose restatement changed reads as unapproved again.
func restatementApproval(step *quaycrewv1.Step) string {
	if !step.GetRestatementApproved() {
		return "not approved"
	}
	return "approved " + step.GetRestatementApprovedAt().AsTime().Format(whenItWasWritten)
}

// runStepFinish records what came of a step: done, or stopped, and what somebody wrote about it.
//
// The two words take the same arguments in the same order, so the two are one thing to learn. With
// two arguments they are the step and the result, and with three the first is the address, which is
// the shape krewe step take already has.
//
// The result is required, and the control plane refuses an empty one: nothing can see inside a
// container, so what somebody wrote is all the next session reads.
//
// A stop runs no check and refuses no unchecked step. It is how a step nobody will finish ends.
func runStepFinish(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient,
	word string, args []string, out io.Writer) error {
	usage := fmt.Sprintf("usage: krewe step %s [<address>] <feature>.<number> %q", word, whatItAsksFor(word))
	if len(args) < 2 || len(args) > 3 {
		return fmt.Errorf("%s", usage)
	}
	typed, said, text := "", args[0], args[1]
	if len(args) == 3 {
		typed, said, text = args[0], args[1], args[2]
	}
	located, err := designProject(ctx, client, typed)
	if err != nil {
		return err
	}
	features, err := featuresOf(ctx, client, located.ProjectID)
	if err != nil {
		return err
	}
	held, number, err := stepAddressed(said, features, located.Path.Project)
	if err != nil {
		return err
	}
	resp, err := client.FinishStep(ctx, &quaycrewv1.FinishStepRequest{
		Feature: held.GetId(), Number: number, State: stateOfStepWord(word), Result: text,
	})
	if err != nil {
		return fmt.Errorf("%w\n\nnothing was written", err)
	}
	step := resp.GetStep()
	fmt.Fprintf(out, "step %d.%d of %s is %s: %s\n",
		held.GetNumber(), step.GetNumber(), located.Path.Project, step.GetState(), step.GetResult())
	if word == "done" {
		return sayWhatIsNext(ctx, client, held, out)
	}
	return nil
}

// sayWhatIsNext prints the step the operator may take now, under the line saying this one is done.
//
// It is a sentence and never a dispatch. The read starts no session and changes no row, and the step
// it names waits for the operator to type the take.
//
// The number is the control plane's, read back after the write, so the line says what the path holds
// now rather than what it held before this step closed.
func sayWhatIsNext(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient,
	feature *quaycrewv1.Feature, out io.Writer) error {
	resp, err := client.ListSteps(ctx, &quaycrewv1.ListStepsRequest{Feature: feature.GetId()})
	if err != nil {
		return fmt.Errorf("%w\n\nthe step is recorded, and what is next was not read", err)
	}
	fmt.Fprintf(out, "%s\n", nextLine(resp.GetNext()))
	return nil
}

// stateOfStepWord is the state word the typed word writes. Done is its own word, and stop writes
// stopped, the way krewe feature stop does.
func stateOfStepWord(word string) string {
	if word == "stop" {
		return "stopped"
	}
	return word
}

// whatItAsksFor is what the last argument of each word is called in its usage line: a step that
// finished produced something, and a step that stopped has a reason.
func whatItAsksFor(word string) string {
	if word == "stop" {
		return "<reason>"
	}
	return "<result>"
}

// stepAddressed reads a step token, `<feature>.<number>`, into the feature it names and the number
// inside that feature's path.
//
// A bare number was a whole step address before a path belonged to a feature, so it is in somebody's
// notes and in their shell history. It names nothing now and it says so, rather than being guessed
// at, and it says so even when the project holds exactly one feature: a guess that is right today is
// wrong the moment a second feature is added, and it would be wrong silently.
func stepAddressed(said string, features []*quaycrewv1.Feature, project string) (*quaycrewv1.Feature, int32, error) {
	before, after, found := strings.Cut(said, ".")
	if !found {
		return nil, 0, fmt.Errorf("name a step as <feature>.<number>, for example 2.3\n\n%s",
			whatIsOpen(features, project))
	}
	number, err := strconv.Atoi(after)
	if err != nil {
		return nil, 0, fmt.Errorf("%q is not a step: the number after the full stop reads %q", said, after)
	}
	feature, err := featureNumbered(features, before, project)
	if err != nil {
		return nil, 0, err
	}
	return feature, int32(number), nil
}

// whatIsOpen lists the project's open features with their numbers, which is what somebody holding
// the old form needs in order to type the new one.
func whatIsOpen(features []*quaycrewv1.Feature, project string) string {
	open := make([]string, 0, len(features))
	for _, feature := range features {
		if feature.GetState() == "open" {
			open = append(open, fmt.Sprintf("  %d. %s", feature.GetNumber(), feature.GetTitle()))
		}
	}
	if len(open) == 0 {
		return fmt.Sprintf("%s has no open feature: add one with krewe feature add", project)
	}
	return fmt.Sprintf("%s has these open features:\n%s", project, strings.Join(open, "\n"))
}
