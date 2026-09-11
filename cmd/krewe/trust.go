package main

import (
	"context"
	"fmt"
	"io"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
)

// The trust record: how often the operator agreed with what krewe's own run of a scenario reported.
//
// The word done starts with the operator and moves to krewe as krewe earns it. This word reads the
// record and writes nothing, so the operator sees what krewe earned before anything is offered.

const trustUsage = "usage: krewe trust [<address>]"

// runTrust prints where the word done sits, the run of agreements behind it, and the whole record.
//
// It reads through GetDesign, because the trust record lives on the design row. It records nothing:
// reading how far krewe got is a question the operator asks as often as they want to.
//
// It says what each level means, one line each, so the level is a thing the operator can decide about
// from this output alone rather than from the design document.
func runTrust(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient, args []string, out io.Writer) error {
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
	return nil
}

// whatTheLevelsMean says what each level is, one line each.
//
// Both lines print at both levels. The operator reading this is deciding whether to move between
// them, and a line that only appeared at the level you are not on is the line you cannot read.
func whatTheLevelsMean() string {
	return "level 0: krewe checks a step, and you say done\n" +
		"level 1: krewe closes a step its own check passed"
}
