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
	"\n       krewe step show [<address>] <feature>.<number>" +
	"\n       krewe step restatement [<address>] <feature>.<number>" +
	"\n       krewe step approve [<address>] <feature>.<number>" +
	"\n       krewe step check [<address>] <feature>.<number>" +
	"\n       krewe step done [<address>] <feature>.<number> \"<result>\"" +
	"\n       krewe step stop [<address>] <feature>.<number> \"<reason>\""

func runStep(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient, args []string, out io.Writer) error {
	if len(args) > 0 && args[0] == "take" {
		return runStepTake(ctx, client, args[1:], out)
	}
	if len(args) > 0 && args[0] == "show" {
		return runStepShow(ctx, client, args[1:], out)
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

// runStepShow prints one step whole: what the operator wrote under it, where it stands, and what
// krewe's last run of its scenario reported.
//
// It exists for what a row of krewe path cannot hold. That listing gives each step one line, and an
// intention, a list of files and the end of a failed run do not fit on one.
//
// It records nothing of its own and it runs no scenario. The read underneath refreshes the
// restatement from the session's own file, which is the read krewe step restatement already makes,
// and the restatement is not printed here: that command prints it.
func runStepShow(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient, args []string, out io.Writer) error {
	if len(args) == 0 || len(args) > 2 {
		return fmt.Errorf("usage: krewe step show [<address>] <feature>.<number>")
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
	// A number the path does not have is refused here, with how many steps the path has. The count is
	// the control plane's, and it is what tells the operator whether they typed the wrong number or
	// the wrong feature.
	resp, err := client.GetStep(ctx, &quaycrewv1.GetStepRequest{Feature: held.GetId(), Number: number})
	if err != nil {
		return err
	}
	step := resp.GetStep()
	fmt.Fprintf(out, "step %d.%d of %s: %s\n",
		held.GetNumber(), step.GetNumber(), located.Path.Project, step.GetTitle())
	for _, block := range whatTheStepSays(step) {
		fmt.Fprintf(out, "\n%s\n%s\n", block.label, block.text)
	}
	fmt.Fprintf(out, "\nstate: %s\n", step.GetState())
	if step.GetSession() != "" {
		fmt.Fprintf(out, "session: %s\n", step.GetSession())
	}
	fmt.Fprintf(out, "%s\n", whatTheLastRunSaid(step))
	if closer := whoClosedIt(step); closer != "" {
		fmt.Fprintf(out, "%s\n", closer)
	}
	// Under the proof line, so the operator reads why a check failed without running it again.
	if output := strings.TrimRight(step.GetProofOutput(), "\n"); output != "" {
		fmt.Fprintf(out, "\n%s\n", output)
	}
	if step.GetResult() != "" {
		fmt.Fprintf(out, "\nresult: %s\n", step.GetResult())
	}
	return nil
}

// The labels a step block carries, which are the path document's own. A step reads back in the words
// it was written in, so an operator comparing this output against the document they wrote finds the
// same headings in the same order.
const (
	labelIntention = "What changes and why"
	labelTouches   = "What this touches"
	labelProof     = "What proves it"
	labelScenario  = "The scenario that proves it"
	labelAfter     = "After"
)

// stepBlock is one labelled block of a step, as the document writes it.
type stepBlock struct {
	label string
	text  string
}

// whatTheStepSays is the blocks under this step, in the order the document writes them.
//
// A block with nothing in it is left out with its label, the way the path document a session reads
// leaves one out: a label with nothing under it is a line the reader spends a look on to learn that
// it says nothing.
//
// A step that waits for nobody carries a zero, and zero is nothing to wait for, so that block is left
// out on the same rule rather than printed as the digit.
func whatTheStepSays(step *quaycrewv1.Step) []stepBlock {
	waitsFor := ""
	if step.GetAfter() > 0 {
		waitsFor = strconv.Itoa(int(step.GetAfter()))
	}
	blocks := make([]stepBlock, 0, 5)
	for _, block := range []stepBlock{
		{labelIntention, step.GetIntention()},
		{labelTouches, step.GetTouches()},
		{labelProof, step.GetProof()},
		{labelScenario, step.GetProofScenario()},
		{labelAfter, waitsFor},
	} {
		if block.text == "" {
			continue
		}
		blocks = append(blocks, block)
	}
	return blocks
}

// whatTheLastRunSaid is the line under the state: the verdict krewe's own run of this step's scenario
// reported, the count it read out of that run, and when it ran.
//
// A step nobody ran reads unproven, and that line is printed rather than left out. A path listing
// leaves it out because a word repeated down a column says nothing, and one step on the screen is a
// question about that step: whether anybody checked it is half of what was asked.
func whatTheLastRunSaid(step *quaycrewv1.Step) string {
	state := step.GetProofState()
	if state == "" {
		state = proofUnproven
	}
	if state == proofUnproven {
		return "proof: " + proofUnproven
	}
	said := fmt.Sprintf("proof: %s, %s ran", state, display.Scenarios(step.GetProofScenariosRun()))
	if ran := step.GetProofRanAt(); ran != nil {
		said += " at " + ran.AsTime().Format(whenItWasWritten)
	}
	return said
}

// whoClosedIt is who spoke the word that finished this step, and whether that word matched krewe's
// last verdict. It is empty while the step is ready or taken, and the line is left out then, the way
// every empty block of this output is left out.
//
// The two sit on one line because they are one fact about the close. Who closed it says whether
// anybody read the step, and the agreement says what they made of what krewe reported.
func whoClosedIt(step *quaycrewv1.Step) string {
	if step.GetClosedBy() == "" {
		return ""
	}
	said := "closed by " + closerNamed(step.GetClosedBy())
	if step.GetOperatorAgreed() == "" {
		return said
	}
	return fmt.Sprintf("%s, and the row records %s", said, whatTheRowRecords(step))
}

// closerNamed reads the word the row keeps as a sentence names the closer. The operator is a person
// and takes the article, and krewe is a name and does not.
func closerNamed(closedBy string) string {
	if closedBy == closedByKrewe {
		return closedByKrewe
	}
	return "the " + closedBy
}

// closedByKrewe is the word the row carries when krewe closed a step its own check passed. It is the
// control plane's and it is read off the wire, so it is named here rather than compared inline.
const closedByKrewe = "krewe"

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
	sayKreweClosedIt(resp, out)
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

// sayKreweClosedIt prints that krewe spoke the word done on this step, and says nothing on a check
// that closed nothing.
//
// It prints under the verdict and above the output, because a run keeps thousands of characters and
// a line printed under them is a line nobody reads. A close nobody noticed is a close nobody can
// correct, and the correction is the whole safety of the ladder.
//
// So it names the way back in the same breath. A person who cannot find that word stops handing the
// word done over at all, which is what krewe trust raise already says when it hands it over.
func sayKreweClosedIt(resp *quaycrewv1.CheckStepResponse, out io.Writer) {
	if !resp.GetClosedByKrewe() {
		return
	}
	fmt.Fprintf(out, "krewe closed this step: its own check passed, and krewe is at trust level %d\n",
		resp.GetDesign().GetTrustLevel())
	fmt.Fprintf(out, "disagree with it: krewe step reopen [<address>] <feature>.<number>\n")
}

// proofPassing is the one verdict that is not a failure, and proofFailing is what a run that said no
// reads as. The words are the control plane's and they are read off the wire, so they are named here
// rather than compared inline.
//
// A step nobody checked reads as proofUnproven, which is a different thing from a step whose run
// said no. The check refuses that state before it reaches a verdict, and krewe step show prints it.
const (
	proofPassing  = "passing"
	proofFailing  = "failing"
	proofUnproven = "unproven"
)

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
	sayWhatTheRowRecords(step, out)
	// The offer krewe earned with this finish, where it earned one. It is the control plane's
	// sentence, printed as it came, so the word that accepts it is written in one place.
	if offer := resp.GetOffer(); offer != "" {
		fmt.Fprintf(out, "\n%s\n", offer)
	}
	if word == "done" {
		return sayWhatIsNext(ctx, client, held, out)
	}
	return nil
}

// sayWhatTheRowRecords says whether the operator's word matched krewe's last verdict.
//
// It prints on both words. Done after a passing check agrees with krewe, and so does a stop after a
// failing one: the operator read the verdict and did what it pointed at. The other two are
// disagreements, and the count behind them is what decides whether krewe is ever offered the word.
//
// The line is a record and never an argument. The word is the operator's and nothing refuses it, so
// this says what was written rather than telling anybody they were wrong.
func sayWhatTheRowRecords(step *quaycrewv1.Step, out io.Writer) {
	if step.GetOperatorAgreed() == "" {
		return
	}
	// A step nobody checked is closed against no verdict at all, so the line says that rather than
	// naming a state the operator never read. It is still a disagreement: krewe reported nothing to
	// agree with.
	if step.GetProofState() == "" || step.GetProofState() == proofUnproven {
		fmt.Fprintf(out, "nothing checked it, so the row records %s\n", whatTheRowRecords(step))
		return
	}
	fmt.Fprintf(out, "the check said %s, so the row records %s\n",
		step.GetProofState(), whatTheRowRecords(step))
}

// whatTheRowRecords is the two words the trust record counts in, as a sentence reads them.
func whatTheRowRecords(step *quaycrewv1.Step) string {
	if step.GetOperatorAgreed() == agreedYes {
		return "an agreement"
	}
	return "a disagreement"
}

// agreedYes is the word the row carries when the operator's word matched the verdict. It is the
// store's and it is read off the wire, so it is named here rather than compared inline.
const agreedYes = "yes"

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
