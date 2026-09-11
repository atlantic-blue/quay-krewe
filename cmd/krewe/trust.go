package main

import (
	"context"
	"fmt"
	"io"
	"strconv"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
)

// The trust record: how often the operator agreed with what krewe's own run of a scenario reported.
//
// The word done starts with the operator and moves to krewe as krewe earns it. The first word reads
// the record and writes nothing, so the operator sees what krewe earned before anything is offered.
// The other two are the answer to an offer and the number the offer is earned against.

const (
	trustUsage          = "usage: krewe trust [<address>]"
	trustRaiseUsage     = "usage: krewe trust raise [<address>]"
	trustThresholdUsage = "usage: krewe trust threshold [<address>] <number>"
)

// runTrust prints where the word done sits, the run of agreements behind it, and the whole record.
//
// It reads through GetDesign, because the trust record lives on the design row. It records nothing:
// reading how far krewe got is a question the operator asks as often as they want to.
//
// It says what each level means, one line each, so the level is a thing the operator can decide about
// from this output alone rather than from the design document.
//
// A standing offer prints under the record and names the word that accepts it. It stands until the
// operator answers, so this is where they find it again after the finish that made it scrolled away.
func runTrust(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient, args []string, out io.Writer) error {
	if len(args) > 0 && args[0] == "raise" {
		return runTrustRaise(ctx, client, args[1:], out)
	}
	if len(args) > 0 && args[0] == "threshold" {
		return runTrustThreshold(ctx, client, args[1:], out)
	}
	if len(args) > 1 {
		return fmt.Errorf("%s", trustUsage)
	}
	typed := ""
	if len(args) == 1 {
		typed = args[0]
	}
	located, err := designProject(ctx, client, typed)
	if err != nil {
		return err
	}
	resp, err := client.GetDesign(ctx, &quaycrewv1.GetDesignRequest{Project: located.ProjectID})
	if err != nil {
		return err
	}
	design := resp.GetDesign()
	// A project with no design has taken no step, so it has agreed with nothing and sits at level 0.
	// The line says so rather than printing a record of zeroes, which reads as a project that tried
	// and failed.
	if design.GetBrief() == "" && design.GetBody() == "" {
		fmt.Fprintf(out, "%s has no design yet, so krewe is at trust level 0\n\n",
			located.Path.Project)
		fmt.Fprintf(out, "%s\n", whatTheLevelsMean())
		return nil
	}
	fmt.Fprintf(out, "%s: krewe is at trust level %d\n\n",
		located.Path.Project, design.GetTrustLevel())
	fmt.Fprintf(out, "%s\n", whatTheLevelsMean())
	// Counted in words rather than as bare numbers, because a reader who finds "1 agreements" in this
	// output reads the whole record as generated rather than as a record.
	fmt.Fprintf(out, "\n%s in a row, against a threshold of %d\n",
		counted(int(design.GetTrustRun()), "agreement"), design.GetTrustThreshold())
	fmt.Fprintf(out, "%s and %s in all\n",
		counted(int(design.GetTrustAgreements()), "agreement"),
		counted(int(design.GetTrustDisagreements()), "disagreement"))
	sayTheStandingOffer(design, out)
	return nil
}

// sayTheStandingOffer prints the offer krewe made, until the operator answers it.
//
// The sentence is the control plane's, and this asks the row whether one stands rather than deciding
// it from the numbers. A second rule here would print an offer a raise would then refuse.
func sayTheStandingOffer(design *quaycrewv1.Design, out io.Writer) {
	if !design.GetTrustOffered() {
		return
	}
	fmt.Fprintf(out, "\nkrewe agreed with you %s in a row, so it asks for level 1\n",
		timesOver(design.GetTrustRun()))
	fmt.Fprintf(out, "accept with krewe trust raise [<address>]\n")
}

// timesOver counts a run in words. A project may set its threshold to one, and "1 times" reads as a
// sentence a machine assembled rather than a record of what happened.
func timesOver(run int32) string {
	if run == 1 {
		return "1 time"
	}
	return fmt.Sprintf("%d times", run)
}

// whatTheLevelsMean says what each level is, one line each.
//
// Both lines print at both levels. The operator reading this is deciding whether to move between
// them, and a line that only appeared at the level you are not on is the line you cannot read.
func whatTheLevelsMean() string {
	return "level 0: krewe checks a step, and you say done\n" +
		"level 1: krewe closes a step its own check passed"
}

// runTrustRaise accepts the offer krewe made, and says what changes.
//
// It accepts an offer and never makes one. Krewe offers the next level when the run of agreements
// reaches the threshold, and this is the operator answering: a raise with no offer standing is
// refused, and the refusal names the word that reads the record.
//
// The output names krewe step reopen, because a level the operator regrets has a way back down and a
// person who cannot find it stops handing the word over at all.
func runTrustRaise(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient,
	args []string, out io.Writer) error {
	if len(args) > 1 {
		return fmt.Errorf("%s", trustRaiseUsage)
	}
	typed := ""
	if len(args) == 1 {
		typed = args[0]
	}
	located, err := designProject(ctx, client, typed)
	if err != nil {
		return err
	}
	resp, err := client.RaiseTrust(ctx, &quaycrewv1.RaiseTrustRequest{Project: located.ProjectID})
	if err != nil {
		return fmt.Errorf("%w\n\nnothing was written", err)
	}
	fmt.Fprintf(out, "%s: krewe is at trust level %d\n\n",
		located.Path.Project, resp.GetDesign().GetTrustLevel())
	fmt.Fprintf(out, "krewe now closes a step its own check passed, and says so on the check\n")
	fmt.Fprintf(out, "a failing check still closes nothing, at any level\n")
	fmt.Fprintf(out, "\ndisagree with one it closed: krewe step reopen [<address>] <feature>.<number>\n")
	return nil
}

// runTrustThreshold sets how many agreements in a row earn an offer of the next level.
//
// With one argument the argument is the number, and with two the first is the address, which is the
// shape krewe path cap already has.
//
// It makes no offer, whatever the number. The offer belongs to the finish that moves the run, so a
// threshold set below a run the project already has is a number the next finish reads.
//
// The output says where the five came from, because it came from nowhere: nothing measured it, and a
// number presented without that reads as a figure somebody arrived at.
func runTrustThreshold(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient,
	args []string, out io.Writer) error {
	if len(args) < 1 || len(args) > 2 {
		return fmt.Errorf("%s", trustThresholdUsage)
	}
	typed, said := "", args[0]
	if len(args) == 2 {
		typed, said = args[0], args[1]
	}
	run, err := strconv.Atoi(said)
	if err != nil {
		return fmt.Errorf("%s", trustThresholdUsage)
	}
	located, err := designProject(ctx, client, typed)
	if err != nil {
		return err
	}
	resp, err := client.SetTrustThreshold(ctx, &quaycrewv1.SetTrustThresholdRequest{
		Project: located.ProjectID, Threshold: int32(run),
	})
	if err != nil {
		return fmt.Errorf("%w\n\nnothing was written", err)
	}
	design := resp.GetDesign()
	fmt.Fprintf(out, "%s offers krewe level 1 after %s in a row\n",
		located.Path.Project, counted(int(design.GetTrustThreshold()), "agreement"))
	fmt.Fprintf(out, "\nthe default of 5 is a guess and nothing measured it, because krewe has closed no step yet\n")
	fmt.Fprintf(out, "the number that replaces it comes from the first project to reach ten closes\n")
	// Said only when it is true. The run is not reset and no offer is made here, so a threshold under
	// the run reads as an offer that should already have arrived, and the line says where it comes
	// from instead.
	if design.GetTrustRun() >= design.GetTrustThreshold() &&
		design.GetTrustLevel() < trustLevelCloses && !design.GetTrustOffered() {
		fmt.Fprintf(out, "\nthe run already stands at %d, and this made no offer: "+
			"krewe offers the level on the next step you finish\n", design.GetTrustRun())
	}
	return nil
}

// trustLevelCloses is the level at which krewe closes a step its own check passed. It is the store's
// number and it is read off the wire, so it is named here rather than compared inline.
const trustLevelCloses int32 = 1
