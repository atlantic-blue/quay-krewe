package features_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/display"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/store"
	"github.com/cucumber/godog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// pathWorld is the last path written or read, the warnings that came with the write, and the file a
// path was written from.
type pathWorld struct {
	steps    []*quaycrewv1.Step
	warnings []string
	file     string
	// take is what came back from the last take: the step, the session that took it, and the text
	// that session was given. Kept so an assertion reads the answer the caller got.
	take *quaycrewv1.TakeStepResponse
	// feature is the one the scenarios that name no feature are about. A path belongs to a feature,
	// and most of these scenarios are about the path rather than about which feature holds it.
	feature *quaycrewv1.Feature
	// milestones are what the last read of the path answered with. They travel with the steps, so a
	// scenario that asserts a grouping reads one answer rather than asking twice.
	milestones []*quaycrewv1.Milestone
	// next is the step the last read said may be taken now, and 0 where it said none may. It is the
	// answer the caller got rather than a number worked out here: the rule is the control plane's.
	next int32
	// read is what the last read of one step answered: the step, and what it warned about. Kept so
	// the assertions read the answer the operator got rather than asking the store a second time.
	read *quaycrewv1.GetStepResponse
	// approvals is what each approval of a restatement answered, in the order the scenario made them:
	// the step, the session dispatched, and the text that session was given. Every one is kept rather
	// than the last, because the scenario about approving twice holds the two stamps to each other.
	approvals []*quaycrewv1.ApproveRestatementResponse
	// checked is what the last check of a step answered, and askedBefore is how many things the model
	// had been asked when that check started. A check asks it nothing, and a count taken before is the
	// only way to say so in a scenario whose setup already asked it three times.
	checked     *quaycrewv1.CheckStepResponse
	askedBefore int
	// checks is what every check of a step answered, in the order the scenario made them. The
	// scenario about a second check reads the second answer rather than the last, because what it is
	// about is the difference between the two: the first says krewe started a container and the
	// second says nothing, which is how a scenario states that the container was reused.
	checks []*quaycrewv1.CheckStepResponse
	// containersBefore is how many containers the session holding the step had been made when the
	// scenario reclaimed it, so a count afterwards is the containers the check made and not the one
	// the take made.
	containersBefore int
	// recorded is the path as it stood before a scenario closed the feature, so a later read is
	// compared against what was there rather than against what the scenario meant to write. A step
	// somebody took has already moved, and this is what says closing the feature moved nothing more.
	recorded []*quaycrewv1.Step
}

// theFeature is the feature a scenario means when it names none.
//
// The first step that needs one adds it, rather than the background adding it to every scenario:
// the scenarios about features count from one, and a feature added under all of them would move
// every number they assert.
func theFeature(ctx context.Context) (*quaycrewv1.Feature, error) {
	w, p := worldFrom(ctx), pathFrom(ctx)
	if p.feature != nil {
		return p.feature, nil
	}
	resp, err := w.client.AddFeature(ctx, &quaycrewv1.AddFeatureRequest{
		Project: w.projectID, Title: "the bills"})
	if err != nil {
		return nil, err
	}
	p.feature = resp.GetFeature()
	return p.feature, nil
}

// featureOfProject is the feature of a number, read from the project now rather than from whatever
// listing a scenario last asked for.
func featureOfProject(ctx context.Context, number int32) (*quaycrewv1.Feature, error) {
	w := worldFrom(ctx)
	resp, err := w.client.ListFeatures(ctx, &quaycrewv1.ListFeaturesRequest{Project: w.projectID})
	if err != nil {
		return nil, err
	}
	for _, feature := range resp.GetFeatures() {
		if feature.GetNumber() == number {
			return feature, nil
		}
	}
	return nil, fmt.Errorf("the project has no feature %d", number)
}

type pathKey struct{}

func pathFrom(ctx context.Context) *pathWorld {
	p, _ := ctx.Value(pathKey{}).(*pathWorld)
	return p
}

// stepNumbered finds one step of the path last read. It is the read the assertions go through, so a
// missing step reports which numbers are there rather than an index out of range.
func stepNumbered(ctx context.Context, number int32) (*quaycrewv1.Step, error) {
	p := pathFrom(ctx)
	for _, step := range p.steps {
		if step.GetNumber() == number {
			return step, nil
		}
	}
	var held []string
	for _, step := range p.steps {
		held = append(held, fmt.Sprintf("%d", step.GetNumber()))
	}
	return nil, fmt.Errorf("the path has no step %d, it holds %s", number, strings.Join(held, ", "))
}

// milestoneNumbered finds one milestone of the path last read. It reports which numbers are there,
// for the reason stepNumbered does.
func milestoneNumbered(ctx context.Context, number int32) (*quaycrewv1.Milestone, error) {
	p := pathFrom(ctx)
	for _, milestone := range p.milestones {
		if milestone.GetNumber() == number {
			return milestone, nil
		}
	}
	var held []string
	for _, milestone := range p.milestones {
		held = append(held, fmt.Sprintf("%d", milestone.GetNumber()))
	}
	return nil, fmt.Errorf("the path has no milestone %d, it holds %s", number, strings.Join(held, ", "))
}

func initializePathSteps(sc *godog.ScenarioContext) {
	initializeFeatureSteps(sc)
	initializeProofRunSteps(sc)
	initializeReclaimedSessionSteps(sc)
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, pathKey{}, &pathWorld{}), nil
	})

	sc.Step(`^the operator sets the path to:$`, func(ctx context.Context, document *godog.DocString) error {
		return setPath(ctx, document.Content)
	})

	sc.Step(`^the project's path is:$`, func(ctx context.Context, document *godog.DocString) error {
		return setPath(ctx, document.Content)
	})

	// The scenarios that are about two features name them, so the feature a path is written to is
	// read from the project rather than assumed.
	sc.Step(`^the operator sets the path of feature (\d+) to:$`,
		func(ctx context.Context, number int, document *godog.DocString) error {
			held, err := featureOfProject(ctx, int32(number))
			if err != nil {
				return err
			}
			return setPathOf(ctx, held.GetId(), document.Content)
		})

	// The driver's own token, which is what a session inside a sandbox presents. A design session is
	// what writes a path, so this call is not one the operator keeps.
	sc.Step(`^the driver sets the path to:$`, func(ctx context.Context, document *godog.DocString) error {
		held, err := theFeature(ctx)
		if err != nil {
			return err
		}
		return asDriver(ctx, func(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient) error {
			_, err := client.SetPath(ctx, &quaycrewv1.SetPathRequest{
				Feature: held.GetId(), Document: document.Content})
			return err
		})
	})

	sc.Step(`^the operator sets a path without saying which feature$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		_, w.lastErr = w.client.SetPath(ctx, &quaycrewv1.SetPathRequest{
			Document: "## 1. The store holds a project's brief"})
		return nil
	})

	sc.Step(`^the operator reads the path$`, func(ctx context.Context) error {
		held, err := theFeature(ctx)
		if err != nil {
			return err
		}
		return readPath(ctx, held.GetId())
	})

	sc.Step(`^the operator reads the path of feature (\d+)$`, func(ctx context.Context, number int) error {
		held, err := featureOfProject(ctx, int32(number))
		if err != nil {
			return err
		}
		return readPath(ctx, held.GetId())
	})

	// The empty identifier is what lets a caller count the steps of every feature in one call.
	sc.Step(`^the operator reads the path of every feature$`, func(ctx context.Context) error {
		if _, err := theFeature(ctx); err != nil {
			return err
		}
		return readPath(ctx, "")
	})

	sc.Step(`^the operator reads the path of a feature that does not exist$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		_, w.lastErr = w.client.ListSteps(ctx, &quaycrewv1.ListStepsRequest{Feature: "no-such-feature"})
		return nil
	})

	sc.Step(`^the path holds (\d+) steps$`, func(ctx context.Context, want int) error {
		if got := len(pathFrom(ctx).steps); got != want {
			return fmt.Errorf("the path holds %d steps, want %d", got, want)
		}
		return nil
	})

	// Order is what the control plane promises, so it is asserted as an order and not as a set: a
	// check that every number is present passes against a path drawn in the wrong order.
	sc.Step(`^the path reads ([\d, ]+) in that order$`, func(ctx context.Context, wanted string) error {
		var got []string
		for _, step := range pathFrom(ctx).steps {
			got = append(got, fmt.Sprintf("%d", step.GetNumber()))
		}
		if reads := strings.Join(got, ", "); reads != wanted {
			return fmt.Errorf("the path reads %s, want %s", reads, wanted)
		}
		return nil
	})

	sc.Step(`^the project has no path$`, func(ctx context.Context) error {
		held, err := theFeature(ctx)
		if err != nil {
			return err
		}
		w := worldFrom(ctx)
		resp, err := w.client.ListSteps(ctx, &quaycrewv1.ListStepsRequest{Feature: held.GetId()})
		if err != nil {
			return err
		}
		if got := len(resp.GetSteps()); got != 0 {
			return fmt.Errorf("the feature holds %d steps, and the document was refused", got)
		}
		return nil
	})

	sc.Step(`^step (\d+) is titled "([^"]*)"$`, func(ctx context.Context, number int, want string) error {
		step, err := stepNumbered(ctx, int32(number))
		if err != nil {
			return err
		}
		if got := step.GetTitle(); got != want {
			return fmt.Errorf("step %d is titled %q, want %q", number, got, want)
		}
		return nil
	})

	sc.Step(`^step (\d+) says its intention is "([^"]*)"$`, func(ctx context.Context, number int, want string) error {
		step, err := stepNumbered(ctx, int32(number))
		if err != nil {
			return err
		}
		if got := step.GetIntention(); got != unescape(want) {
			return fmt.Errorf("step %d says its intention is %q, want %q", number, got, unescape(want))
		}
		return nil
	})

	// Line breaks and all, because the take reads this field line by line and a field joined into one
	// line would name one file nobody has.
	sc.Step(`^step (\d+) touches "([^"]*)"$`, func(ctx context.Context, number int, want string) error {
		step, err := stepNumbered(ctx, int32(number))
		if err != nil {
			return err
		}
		if got := step.GetTouches(); got != unescape(want) {
			return fmt.Errorf("step %d touches %q, want %q", number, got, unescape(want))
		}
		return nil
	})

	sc.Step(`^step (\d+) says its proof is "([^"]*)"$`, func(ctx context.Context, number int, want string) error {
		step, err := stepNumbered(ctx, int32(number))
		if err != nil {
			return err
		}
		if got := step.GetProof(); got != unescape(want) {
			return fmt.Errorf("step %d says its proof is %q, want %q", number, got, unescape(want))
		}
		return nil
	})

	sc.Step(`^step (\d+) names the scenario "([^"]*)"$`, func(ctx context.Context, number int, want string) error {
		step, err := stepNumbered(ctx, int32(number))
		if err != nil {
			return err
		}
		if got := step.GetProofScenario(); got != want {
			return fmt.Errorf("step %d names the scenario %q, want %q", number, got, want)
		}
		return nil
	})

	// Line breaks and all, for the reason the touches are asserted with them: the take text reads the
	// contracts one identifier per line, and a field joined into one line would name one contract
	// nobody wrote.
	sc.Step(`^step (\d+) builds the contracts "([^"]*)"$`,
		func(ctx context.Context, number int, want string) error {
			step, err := stepNumbered(ctx, int32(number))
			if err != nil {
				return err
			}
			if got := step.GetContracts(); got != unescape(want) {
				return fmt.Errorf("step %d builds %q, want %q", number, got, unescape(want))
			}
			return nil
		})

	sc.Step(`^step (\d+) says the scope of each contract is "([^"]*)"$`,
		func(ctx context.Context, number int, want string) error {
			step, err := stepNumbered(ctx, int32(number))
			if err != nil {
				return err
			}
			if got := step.GetContractScope(); got != unescape(want) {
				return fmt.Errorf("step %d scopes its contracts as %q, want %q", number, got, unescape(want))
			}
			return nil
		})

	sc.Step(`^step (\d+) waits for step (\d+)$`, func(ctx context.Context, number, want int) error {
		step, err := stepNumbered(ctx, int32(number))
		if err != nil {
			return err
		}
		if got := step.GetAfter(); got != int32(want) {
			return fmt.Errorf("step %d waits for step %d, want %d", number, got, want)
		}
		return nil
	})

	// What the read said may be taken now. The number is asserted whole, so a rule that answered with
	// a lower step it should have skipped is a failure rather than a near miss.
	sc.Step(`^the path says step (\d+) is next$`, func(ctx context.Context, want int) error {
		if got := pathFrom(ctx).next; got != int32(want) {
			return fmt.Errorf("the path says step %d is next, want step %d", got, want)
		}
		return nil
	})

	// Nothing is next reads as 0 on the wire, and 0 is an answer: every step is taken, or every ready
	// step waits for a step nobody finished.
	sc.Step(`^the path says nothing is next$`, func(ctx context.Context) error {
		if got := pathFrom(ctx).next; got != 0 {
			return fmt.Errorf("the path says step %d is next, and no step may be taken", got)
		}
		return nil
	})

	// Several steps at once, and the cap that says how many.

	// Two takes are two sessions. One session builds one step, so a fan out that gave two steps to
	// one session would be a wider session rather than a second one.
	sc.Step(`^step (\d+) and step (\d+) name different sessions$`,
		func(ctx context.Context, first, second int) error {
			held, err := theFeature(ctx)
			if err != nil {
				return err
			}
			if err := readPath(ctx, held.GetId()); err != nil {
				return err
			}
			one, err := stepNumbered(ctx, int32(first))
			if err != nil {
				return err
			}
			other, err := stepNumbered(ctx, int32(second))
			if err != nil {
				return err
			}
			if one.GetSession() == "" || other.GetSession() == "" {
				return fmt.Errorf("step %d names session %q and step %d names session %q, and a taken step names one",
					first, one.GetSession(), second, other.GetSession())
			}
			if one.GetSession() == other.GetSession() {
				return fmt.Errorf("step %d and step %d both name session %q",
					first, second, one.GetSession())
			}
			return nil
		})

	// The cap refusal is its own code, because it is not a state the caller got wrong. The work is
	// allowed and there is no room for it yet.
	sc.Step(`^the control plane refuses it as too many steps at once$`, func(ctx context.Context) error {
		return refused(worldFrom(ctx), codes.ResourceExhausted)
	})

	// The file refusal is the wrong state rather than a full machine: the take is not allowed at all
	// while that step runs, and no amount of room would let it through.
	sc.Step(`^the control plane refuses it as a file two steps write$`, func(ctx context.Context) error {
		return refused(worldFrom(ctx), codes.FailedPrecondition)
	})

	// The count the take answered with, and never one worked out here. It is the count the write
	// made, so a take that dispatched without counting is a failure rather than a number that
	// happens to agree.
	sc.Step(`^the take says (\d+) of (\d+) steps are in flight$`,
		func(ctx context.Context, flying, want int) error {
			p := pathFrom(ctx)
			if p.take == nil {
				return fmt.Errorf("no step was taken, so nothing counted what is in flight")
			}
			if got := p.take.GetInFlight(); got != int32(flying) {
				return fmt.Errorf("the take says %d steps are in flight, want %d", got, flying)
			}
			if got := p.take.GetStepsInFlightCap(); got != int32(want) {
				return fmt.Errorf("the take says the cap is %d, want %d", got, want)
			}
			return nil
		})

	sc.Step(`^the operator caps the steps in flight at (\d+)$`, func(ctx context.Context, atOnce int) error {
		w := worldFrom(ctx)
		_, err := w.client.SetStepsInFlightCap(ctx, &quaycrewv1.SetStepsInFlightCapRequest{
			Project: w.projectID, StepsInFlightCap: int32(atOnce)})
		w.lastErr = err
		return nil
	})

	// Read back off the design rather than off what the write answered, so a refused write that
	// wrote anyway is a failure here.
	sc.Step(`^the cap on steps in flight is (\d+)$`, func(ctx context.Context, want int) error {
		w := worldFrom(ctx)
		resp, err := w.client.GetDesign(ctx, &quaycrewv1.GetDesignRequest{Project: w.projectID})
		if err != nil {
			return err
		}
		if got := resp.GetDesign().GetStepsInFlightCap(); got != int32(want) {
			return fmt.Errorf("the project caps the steps in flight at %d, want %d", got, want)
		}
		return nil
	})

	sc.Step(`^the driver asks to cap the steps in flight$`, func(ctx context.Context) error {
		return asDriver(ctx, func(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient) error {
			_, err := client.SetStepsInFlightCap(ctx, &quaycrewv1.SetStepsInFlightCapRequest{
				Project: worldFrom(ctx).projectID, StepsInFlightCap: 20})
			return err
		})
	})

	// The guard on the fan out, read as the whole path rather than as one step. A step that started
	// because another one ended would name a session, and counting the steps that name one is what
	// catches it wherever it happened.
	sc.Step(`^no step but step (\d+) names a session$`, func(ctx context.Context, number int) error {
		held, err := theFeature(ctx)
		if err != nil {
			return err
		}
		if err := readPath(ctx, held.GetId()); err != nil {
			return err
		}
		for _, step := range pathFrom(ctx).steps {
			if step.GetNumber() == int32(number) || step.GetSession() == "" {
				continue
			}
			return fmt.Errorf("step %d names session %q, and only step %d took one",
				step.GetNumber(), step.GetSession(), number)
		}
		return nil
	})

	// The refusal is read against the steps that are actually in flight, rather than against a
	// sentence written here, so a refusal that named two of three passes nothing. The count is
	// asserted first: a loop over an empty list names nothing and would report success.
	sc.Step(`^the refusal names the steps in flight with the feature each one sits in$`,
		func(ctx context.Context) error {
			w := worldFrom(ctx)
			if w.lastErr == nil {
				return fmt.Errorf("nothing was refused")
			}
			features, err := w.client.ListFeatures(ctx, &quaycrewv1.ListFeaturesRequest{
				Project: w.projectID})
			if err != nil {
				return err
			}
			named := 0
			for _, feature := range features.GetFeatures() {
				listed, err := w.client.ListSteps(ctx, &quaycrewv1.ListStepsRequest{
					Feature: feature.GetId()})
				if err != nil {
					return err
				}
				for _, step := range listed.GetSteps() {
					if step.GetState() != "taken" {
						continue
					}
					named++
					want := fmt.Sprintf("step %d.%d %s",
						feature.GetNumber(), step.GetNumber(), feature.GetTitle())
					if !strings.Contains(w.lastErr.Error(), want) {
						return fmt.Errorf("the refusal is %q, and it never names %q",
							w.lastErr.Error(), want)
					}
				}
			}
			if named == 0 {
				return fmt.Errorf("no step is in flight, so this proves nothing about the refusal")
			}
			return nil
		})
	sc.Step(`^the caller reads the cap on steps in flight$`, func(ctx context.Context) error {
		return runTool(ctx, "path", "cap", whereTheProjectIs(ctx))
	})

	sc.Step(`^the caller caps the steps in flight at "([^"]*)"$`, func(ctx context.Context, said string) error {
		return runTool(ctx, "path", "cap", whereTheProjectIs(ctx), said)
	})

	sc.Step(`^the path holds (\d+) milestones$`, func(ctx context.Context, want int) error {
		if got := len(pathFrom(ctx).milestones); got != want {
			return fmt.Errorf("the path holds %d milestones, want %d", got, want)
		}
		return nil
	})

	// Order is what the control plane promises, so it is asserted as an order and not as a set,
	// for the reason the steps are.
	sc.Step(`^the milestones read ([\d, ]+) in that order$`, func(ctx context.Context, wanted string) error {
		var got []string
		for _, milestone := range pathFrom(ctx).milestones {
			got = append(got, fmt.Sprintf("%d", milestone.GetNumber()))
		}
		if reads := strings.Join(got, ", "); reads != wanted {
			return fmt.Errorf("the milestones read %s, want %s", reads, wanted)
		}
		return nil
	})

	sc.Step(`^milestone (\d+) is titled "([^"]*)"$`, func(ctx context.Context, number int, want string) error {
		milestone, err := milestoneNumbered(ctx, int32(number))
		if err != nil {
			return err
		}
		if got := milestone.GetTitle(); got != want {
			return fmt.Errorf("milestone %d is titled %q, want %q", number, got, want)
		}
		return nil
	})

	sc.Step(`^milestone (\d+) says its intention is "([^"]*)"$`,
		func(ctx context.Context, number int, want string) error {
			milestone, err := milestoneNumbered(ctx, int32(number))
			if err != nil {
				return err
			}
			if got := milestone.GetIntention(); got != unescape(want) {
				return fmt.Errorf("milestone %d says its intention is %q, want %q", number, got, unescape(want))
			}
			return nil
		})

	// Zero is the answer for a step under no milestone, so it is asserted like any other number
	// rather than as an absence.
	sc.Step(`^step (\d+) is in milestone (\d+)$`, func(ctx context.Context, number, want int) error {
		step, err := stepNumbered(ctx, int32(number))
		if err != nil {
			return err
		}
		if got := step.GetMilestone(); got != int32(want) {
			return fmt.Errorf("step %d is in milestone %d, want %d", number, got, want)
		}
		return nil
	})

	// The setup for the scenarios about the path write, which protects three states. It closes the
	// step through the call that closes one, so the record a rewrite has to keep is the record the
	// system writes.
	//
	// A step going to done carries a verdict first, because the word is refused until somebody read
	// one. These scenarios are about what a rewrite keeps, so the run they record passed.
	sc.Step(`^step (\d+) is recorded as (done|stopped)$`,
		func(ctx context.Context, number int, state string) error {
			if state == "done" {
				if err := recordAVerdict(ctx, int32(number), store.ProofPassing); err != nil {
					return err
				}
			}
			return finishStep(ctx, int32(number), state, "what came of it")
		})

	// The same setup where the scenario runs two features, so the step it closes is the step of the
	// feature it names rather than the one a scenario means when it names none.
	sc.Step(`^step (\d+) of feature (\d+) is recorded as (done|stopped)$`,
		func(ctx context.Context, number, feature int, state string) error {
			held, err := featureOfProject(ctx, int32(feature))
			if err != nil {
				return err
			}
			if state == "done" {
				if err := recordAVerdictOf(ctx, held.GetId(), int32(number), store.ProofPassing); err != nil {
					return err
				}
			}
			return finishStepOf(ctx, held.GetId(), int32(number), state, "what came of it")
		})

	// A verdict on a step, written where krewe's own check writes one.
	//
	// It goes straight into the store, past the control plane, because a real check needs the session
	// that holds the step, a container under it and a proof command on the project. The scenarios
	// that use this line are about the word that closes a step, and the ones about the run itself
	// take the whole way there and check for real.
	sc.Step(`^krewe checked step (\d+), and it (passed|failed)$`,
		func(ctx context.Context, number int, said string) error {
			if said == "failed" {
				return recordAVerdict(ctx, int32(number), store.ProofFailing)
			}
			return recordAVerdict(ctx, int32(number), store.ProofPassing)
		})
	// Read from the control plane rather than from what the last write answered, because what a
	// refused write left behind is the question every one of these scenarios asks.
	sc.Step(`^step (\d+) is still (ready|taken|done|stopped)$`,
		func(ctx context.Context, number int, want string) error {
			held, err := theFeature(ctx)
			if err != nil {
				return err
			}
			if err := readPath(ctx, held.GetId()); err != nil {
				return err
			}
			step, err := stepNumbered(ctx, int32(number))
			if err != nil {
				return err
			}
			if got := step.GetState(); got != want {
				return fmt.Errorf("step %d reads as %q, want %q", number, got, want)
			}
			return nil
		})

	sc.Step(`^step (\d+) is ready$`, func(ctx context.Context, number int) error {
		step, err := stepNumbered(ctx, int32(number))
		if err != nil {
			return err
		}
		if got := step.GetState(); got != "ready" {
			return fmt.Errorf("step %d reads as %q, and nobody has taken it", number, got)
		}
		if step.GetSession() != "" || step.GetTakenAt() != nil {
			return fmt.Errorf("step %d names session %q, taken at %v, and nobody has taken it",
				number, step.GetSession(), step.GetTakenAt())
		}
		return nil
	})

	// Taking a step. The take dispatches and lets go, so every one of these waits for the exec to
	// land before it reads what the session was given.

	sc.Step(`^the operator (?:takes|took) step (\d+)$`, func(ctx context.Context, number int) error {
		held, err := theFeature(ctx)
		if err != nil {
			return err
		}
		return takeStep(ctx, held.GetId(), int32(number))
	})

	sc.Step(`^the operator (?:takes|took) step (\d+) of feature (\d+)$`,
		func(ctx context.Context, number, feature int) error {
			held, err := featureOfProject(ctx, int32(feature))
			if err != nil {
				return err
			}
			return takeStep(ctx, held.GetId(), int32(number))
		})

	// Recorded from the store rather than from what the scenario wrote, because a step somebody took
	// has already moved. What changes after this is what closing the feature did, and nothing else.
	sc.Step(`^the path as it stands is written down$`, func(ctx context.Context) error {
		held, err := theFeature(ctx)
		if err != nil {
			return err
		}
		if err := readPath(ctx, held.GetId()); err != nil {
			return err
		}
		p := pathFrom(ctx)
		p.recorded = p.steps
		if len(p.recorded) == 0 {
			return fmt.Errorf("the feature holds no step, so a later comparison would prove nothing")
		}
		return nil
	})

	// Field by field, because the claim is that closing a feature touched no step. A count that
	// matched would pass against a path whose states all moved.
	sc.Step(`^the path reads back as it was written down$`, func(ctx context.Context) error {
		p := pathFrom(ctx)
		if len(p.recorded) == 0 {
			return fmt.Errorf("no path was written down, so there is nothing to compare against")
		}
		if len(p.steps) != len(p.recorded) {
			return fmt.Errorf("the path reads back %d steps, want the %d it held", len(p.steps), len(p.recorded))
		}
		for at, step := range p.steps {
			was := p.recorded[at]
			if step.GetNumber() != was.GetNumber() || step.GetTitle() != was.GetTitle() ||
				step.GetState() != was.GetState() || step.GetSession() != was.GetSession() ||
				step.GetMilestone() != was.GetMilestone() || step.GetIntention() != was.GetIntention() {
				return fmt.Errorf("step %d reads %q in state %q held by %q, want %q in state %q held by %q",
					step.GetNumber(), step.GetTitle(), step.GetState(), step.GetSession(),
					was.GetTitle(), was.GetState(), was.GetSession())
			}
		}
		return nil
	})

	sc.Step(`^the operator takes a step of a feature that does not exist$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		_, w.lastErr = w.client.TakeStep(ctx, &quaycrewv1.TakeStepRequest{
			Feature: "no-such-feature", Number: 1})
		return nil
	})

	sc.Step(`^the step text carries "([^"]*)"$`, func(ctx context.Context, want string) error {
		text, err := takenText(ctx)
		if err != nil {
			return err
		}
		if !strings.Contains(text, unescape(want)) {
			return fmt.Errorf("the step text does not carry %q: %q", unescape(want), text)
		}
		return nil
	})

	sc.Step(`^the step text does not carry "([^"]*)"$`, func(ctx context.Context, unwanted string) error {
		text, err := takenText(ctx)
		if err != nil {
			return err
		}
		if strings.Contains(text, unescape(unwanted)) {
			return fmt.Errorf("the step text carries %q, and it should not: %q", unescape(unwanted), text)
		}
		return nil
	})

	// Read off the model rather than off the answer, because what the take composed and what the
	// session was actually asked are two different claims, and only the second one is the feature.
	sc.Step(`^the session was asked exactly what the take composed$`, func(ctx context.Context) error {
		text, err := takenText(ctx)
		if err != nil {
			return err
		}
		if asked := worldFrom(ctx).runner.lastRequest().Text; asked != text {
			return fmt.Errorf("the session was asked %q, and the take answered with %q", asked, text)
		}
		return nil
	})

	// The step names the session in the same write that moved its state, so a take that dispatched
	// and recorded nobody leaves a step nothing can be read back from.
	sc.Step(`^step (\d+) is held by that session$`, func(ctx context.Context, number int) error {
		held, err := theFeature(ctx)
		if err != nil {
			return err
		}
		return stepHeldByTheTake(ctx, held.GetId(), int32(number))
	})

	// The scenarios about two features name which one they mean. The feature a scenario means when it
	// names none is the first, and the second one is the whole point of these.
	sc.Step(`^step (\d+) of feature (\d+) is held by that session$`,
		func(ctx context.Context, number, feature int) error {
			held, err := featureOfProject(ctx, int32(feature))
			if err != nil {
				return err
			}
			return stepHeldByTheTake(ctx, held.GetId(), int32(number))
		})

	sc.Step(`^the refusal names the session holding step (\d+)$`, func(ctx context.Context, number int) error {
		w, p := worldFrom(ctx), pathFrom(ctx)
		if w.lastErr == nil {
			return fmt.Errorf("nothing was refused")
		}
		if p.take == nil {
			return fmt.Errorf("no step was taken, so no session holds one")
		}
		held := display.ShortID(p.take.GetSession().GetHandle())
		if !strings.Contains(w.lastErr.Error(), held) {
			return fmt.Errorf("the refusal is %q, and it names neither step %d's session %q nor where to find it",
				w.lastErr.Error(), number, held)
		}
		return nil
	})

	// Counted rather than looked for, because a refusal that started a session anyway is exactly what
	// gate 1 exists to stop, and a check that one session exists passes against two.
	sc.Step(`^(\d+) sessions? (?:was|were) started$`, func(ctx context.Context, want int) error {
		w := worldFrom(ctx)
		listed, err := w.client.ListSessions(ctx, &quaycrewv1.ListSessionsRequest{Workspace: w.workspaceID})
		if err != nil {
			return err
		}
		if got := len(listed.GetSessions()); got != want {
			return fmt.Errorf("the workspace holds %d sessions, want %d", got, want)
		}
		return nil
	})

	// What a session says about the step it holds, before it builds anything. It writes the section
	// itself, into its own memory file, and the next exec reads it back.

	sc.Step(`^the session writes its restatement:$`, func(ctx context.Context, text *godog.DocString) error {
		return writeRestatement(ctx, text.Content)
	})

	// One long text, of the length the scenario names, so the warning and the answer are held to the
	// same number. It is one repeated letter because what is counted is characters and never what
	// they say.
	sc.Step(`^the session writes a restatement of (\d+) characters$`, func(ctx context.Context, count int) error {
		return writeRestatement(ctx, strings.Repeat("a", count))
	})

	// Read out of the store rather than out of the file the session wrote, because the claim is that
	// the text reached the step. A check against the file would pass against a read back that did
	// nothing at all.
	sc.Step(`^step (\d+) reads back the restatement "([^"]*)"$`,
		func(ctx context.Context, number int, want string) error {
			step, err := stepAsItStands(ctx, int32(number))
			if err != nil {
				return err
			}
			if !strings.Contains(step.GetRestatement(), unescape(want)) {
				return fmt.Errorf("step %d restates %q, want it to carry %q",
					number, step.GetRestatement(), unescape(want))
			}
			if step.GetRestatedAt() == nil {
				return fmt.Errorf("step %d carries a restatement and no moment it was written", number)
			}
			return nil
		})

	sc.Step(`^step (\d+) does not read back the restatement "([^"]*)"$`,
		func(ctx context.Context, number int, unwanted string) error {
			step, err := stepAsItStands(ctx, int32(number))
			if err != nil {
				return err
			}
			if strings.Contains(step.GetRestatement(), unescape(unwanted)) {
				return fmt.Errorf("step %d still restates %q: %q",
					number, unescape(unwanted), step.GetRestatement())
			}
			return nil
		})

	// Both the word and its moment, because a write that cleared one and left the other says the
	// step was approved at a time nobody approved it.
	// Read out of the store, for the reason the assertions above it are: the claim is about the row
	// the next session is judged against, and not about a file.
	sc.Step(`^step (\d+) reads back no restatement$`, func(ctx context.Context, number int) error {
		step, err := stepAsItStands(ctx, int32(number))
		if err != nil {
			return err
		}
		if step.GetRestatement() != "" || step.GetRestatedAt() != nil {
			return fmt.Errorf("step %d restates %q, written at %v",
				number, step.GetRestatement(), step.GetRestatedAt())
		}
		return nil
	})

	// Every proof column, because one left behind is a verdict the new attempt did not earn. The
	// moment is what gate 3 reads, so a step carrying one could be marked done with nothing run.
	sc.Step(`^step (\d+) has no verdict on it$`, func(ctx context.Context, number int) error {
		step, err := stepAsItStands(ctx, int32(number))
		if err != nil {
			return err
		}
		if step.GetProofState() != store.ProofUnproven {
			return fmt.Errorf("step %d reads %q, want %q",
				number, step.GetProofState(), store.ProofUnproven)
		}
		if step.GetProofRanAt() != nil || step.GetProofScenariosRun() != 0 ||
			step.GetProofOutput() != "" {
			return fmt.Errorf("step %d carries a run at %v, %d scenarios and the output %q",
				number, step.GetProofRanAt(), step.GetProofScenariosRun(), step.GetProofOutput())
		}
		return nil
	})

	sc.Step(`^nobody has approved step (\d+)'s restatement$`, func(ctx context.Context, number int) error {
		step, err := stepAsItStands(ctx, int32(number))
		if err != nil {
			return err
		}
		if step.GetRestatementApproved() || step.GetRestatementApprovedAt() != nil {
			return fmt.Errorf("step %d reads as approved at %v",
				number, step.GetRestatementApprovedAt())
		}
		return nil
	})

	// The restatement is a section and never a level. Swept into the session's own context, it is
	// stored as though the operator had typed it and rendered again underneath itself on every exec
	// from then on.
	sc.Step(`^the session's context does not carry "([^"]*)"$`, func(ctx context.Context, unwanted string) error {
		w := worldFrom(ctx)
		current, err := w.lastExec()
		if err != nil {
			return err
		}
		got, err := w.store.GetContext(ctx, store.ContextSession, current.sessionID)
		if err != nil {
			return err
		}
		if strings.Contains(got, unescape(unwanted)) {
			return fmt.Errorf("the session's context carries %q: %q", unescape(unwanted), got)
		}
		return nil
	})

	// Reading a step on demand. The read refreshes first, so what an assertion sees is what the
	// session's own file says now rather than what the store held before the call.

	sc.Step(`^the operator reads step (\d+)$`, func(ctx context.Context, number int) error {
		w, p := worldFrom(ctx), pathFrom(ctx)
		held, err := theFeature(ctx)
		if err != nil {
			return err
		}
		p.read, w.lastErr = w.client.GetStep(ctx, &quaycrewv1.GetStepRequest{
			Feature: held.GetId(), Number: int32(number),
		})
		return nil
	})

	// Asserted against the answer the read gave, and never against the store, because what this
	// proves is what the operator is looking at.
	sc.Step(`^the restatement read carries "([^"]*)"$`, func(ctx context.Context, want string) error {
		read, err := stepRead(ctx)
		if err != nil {
			return err
		}
		if !strings.Contains(read.GetStep().GetRestatement(), unescape(want)) {
			return fmt.Errorf("the read answered with %q, want it to carry %q",
				read.GetStep().GetRestatement(), unescape(want))
		}
		return nil
	})

	sc.Step(`^the restatement read does not carry "([^"]*)"$`, func(ctx context.Context, unwanted string) error {
		read, err := stepRead(ctx)
		if err != nil {
			return err
		}
		if strings.Contains(read.GetStep().GetRestatement(), unescape(unwanted)) {
			return fmt.Errorf("the read answered with %q, and it still carries %q",
				read.GetStep().GetRestatement(), unescape(unwanted))
		}
		return nil
	})

	sc.Step(`^the restatement read is (\d+) characters$`, func(ctx context.Context, want int) error {
		read, err := stepRead(ctx)
		if err != nil {
			return err
		}
		if got := len(read.GetStep().GetRestatement()); got != want {
			return fmt.Errorf("the read answered with %d characters, want %d", got, want)
		}
		return nil
	})

	sc.Step(`^the read warns "([^"]*)"$`, func(ctx context.Context, want string) error {
		read, err := stepRead(ctx)
		if err != nil {
			return err
		}
		for _, warning := range read.GetWarnings() {
			if strings.Contains(warning, unescape(want)) {
				return nil
			}
		}
		return fmt.Errorf("the read warned %q, and none of it says %q",
			read.GetWarnings(), unescape(want))
	})

	sc.Step(`^the read warns about nothing$`, func(ctx context.Context) error {
		read, err := stepRead(ctx)
		if err != nil {
			return err
		}
		if warnings := read.GetWarnings(); len(warnings) != 0 {
			return fmt.Errorf("the read warned %q about a session whose file it could read", warnings)
		}
		return nil
	})

	// The whole directory, because that is what a session that has gone leaves behind: not a file
	// somebody emptied, but nothing at all where the sandbox used to look.
	sc.Step(`^the session's own directory is gone$`, func(ctx context.Context) error {
		dir, err := sessionWorkingDir(ctx)
		if err != nil {
			return err
		}
		return os.RemoveAll(dir)
	})

	// Approving what the session wrote, which is what starts the build. The exec it dispatches is
	// waited for, because the call lets go of it and an assertion about what the session was asked
	// would otherwise run while that exec was still starting.
	//
	// A refusal is kept the way every other refusal here is kept, so a scenario about a refused
	// approval reads what the write did not change.
	sc.Step(`^the operator approves step (\d+)'s restatement$`, func(ctx context.Context, number int) error {
		w, p := worldFrom(ctx), pathFrom(ctx)
		held, err := theFeature(ctx)
		if err != nil {
			return err
		}
		resp, err := w.client.ApproveRestatement(ctx, &quaycrewv1.ApproveRestatementRequest{
			Feature: held.GetId(), Number: int32(number),
		})
		w.lastErr = err
		if err != nil {
			return nil
		}
		p.approvals = append(p.approvals, resp)
		return w.settled(ctx)
	})

	// The call carries the driver's token, which is what a session inside a sandbox presents. The
	// scenario reads the step again afterwards, because a refusal that still wrote the row would
	// leave the gate looking closed and standing open.
	sc.Step(`^the driver asks to approve step (\d+)'s restatement$`, func(ctx context.Context, number int) error {
		held, err := theFeature(ctx)
		if err != nil {
			return err
		}
		return asDriver(ctx, func(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient) error {
			_, err := client.ApproveRestatement(ctx, &quaycrewv1.ApproveRestatementRequest{
				Feature: held.GetId(), Number: int32(number),
			})
			return err
		})
	})

	// Both the word and its moment, because a write that set one and left the other says the step was
	// approved at no time at all.
	sc.Step(`^step (\d+)'s restatement is approved$`, func(ctx context.Context, number int) error {
		step, err := stepAsItStands(ctx, int32(number))
		if err != nil {
			return err
		}
		if !step.GetRestatementApproved() {
			return fmt.Errorf("step %d reads as unapproved", number)
		}
		if step.GetRestatementApprovedAt() == nil {
			return fmt.Errorf("step %d reads as approved and carries no moment it was approved", number)
		}
		return nil
	})

	// The session that restated the step is the one that builds it: it already holds the
	// conversation, so nothing repeats the step body to it.
	//
	// The exec is read as well as the session, because an approval that answered with the right
	// session and dispatched a different one, or dispatched nothing at all, reads the same here
	// otherwise.
	sc.Step(`^the session that took step (\d+) was asked to build it$`, func(ctx context.Context, number int) error {
		w, p := worldFrom(ctx), pathFrom(ctx)
		approved, err := lastApproval(ctx)
		if err != nil {
			return err
		}
		if p.take == nil {
			return fmt.Errorf("no step was taken, so no session holds step %d", number)
		}
		took := p.take.GetSession()
		if approved.GetSession().GetId() != took.GetId() {
			return fmt.Errorf("the approval dispatched session %q, and step %d was taken by %q",
				approved.GetSession().GetId(), number, took.GetId())
		}
		if asked := w.runner.lastRequest().Text; asked != approved.GetText() {
			return fmt.Errorf("the session was asked %q, and the approval answered with %q",
				asked, approved.GetText())
		}
		return nil
	})

	sc.Step(`^the build text carries "([^"]*)"$`, func(ctx context.Context, want string) error {
		approved, err := lastApproval(ctx)
		if err != nil {
			return err
		}
		if !strings.Contains(approved.GetText(), unescape(want)) {
			return fmt.Errorf("the build text is %q, want it to carry %q",
				approved.GetText(), unescape(want))
		}
		return nil
	})

	sc.Step(`^the build text does not carry "([^"]*)"$`, func(ctx context.Context, unwanted string) error {
		approved, err := lastApproval(ctx)
		if err != nil {
			return err
		}
		if strings.Contains(approved.GetText(), unescape(unwanted)) {
			return fmt.Errorf("the build text is %q, and it carries %q",
				approved.GetText(), unescape(unwanted))
		}
		return nil
	})

	// Held to the moment the first approval answered with rather than to a read of the step, because
	// what this proves is that the second write moved the stamp rather than left the first one there.
	sc.Step(`^the second approval is later than the first$`, func(ctx context.Context) error {
		p := pathFrom(ctx)
		if len(p.approvals) < 2 {
			return fmt.Errorf("this scenario approved %d times, so there is no second approval",
				len(p.approvals))
		}
		first := p.approvals[0].GetStep().GetRestatementApprovedAt()
		second := p.approvals[1].GetStep().GetRestatementApprovedAt()
		if first == nil || second == nil {
			return fmt.Errorf("an approval answered with no moment on it: %v and %v", first, second)
		}
		if !second.AsTime().After(first.AsTime()) {
			return fmt.Errorf("the second approval reads %v, and the first reads %v",
				second.AsTime(), first.AsTime())
		}
		return nil
	})

	sc.Step(`^the caller approves the restatement of step "([^"]*)"$`, func(ctx context.Context, said string) error {
		return runTool(ctx, "step", "approve", whereTheProjectIs(ctx), said)
	})

	sc.Step(`^the caller reads the restatement of step "([^"]*)"$`, func(ctx context.Context, said string) error {
		return runTool(ctx, "step", "restatement", whereTheProjectIs(ctx), said)
	})

	// Finishing a step. The result is the point of the write: nothing can see inside a container, so
	// what somebody wrote is what the next session reads.

	sc.Step(`^the operator finishes step (\d+) with "([^"]*)"$`,
		func(ctx context.Context, number int, result string) error {
			return finishStep(ctx, int32(number), "done", result)
		})

	sc.Step(`^the operator stops step (\d+) with "([^"]*)"$`,
		func(ctx context.Context, number int, reason string) error {
			return finishStep(ctx, int32(number), "stopped", reason)
		})

	// The word as a person typed it, so the refusal is about the word and not about a state the
	// system already knows.
	sc.Step(`^the operator finishes step (\d+) with the word "([^"]*)" and "([^"]*)"$`,
		func(ctx context.Context, number int, word, result string) error {
			return finishStep(ctx, int32(number), word, result)
		})

	sc.Step(`^the operator finishes step (\d+) with no result$`,
		func(ctx context.Context, number int) error {
			return finishStep(ctx, int32(number), "done", "")
		})

	sc.Step(`^the operator finishes step (\d+) of feature (\d+) with "([^"]*)"$`,
		func(ctx context.Context, number, feature int, result string) error {
			held, err := featureOfProject(ctx, int32(feature))
			if err != nil {
				return err
			}
			return finishStepOf(ctx, held.GetId(), int32(number), "done", result)
		})

	sc.Step(`^the operator finishes a step the path does not hold$`, func(ctx context.Context) error {
		return finishStep(ctx, 7, "done", "shipped")
	})

	// Taking a step back off krewe. It is the way down from a close krewe made, and the level pays
	// for it.

	sc.Step(`^the operator reopens step (\d+) saying "([^"]*)"$`,
		func(ctx context.Context, number int, why string) error {
			return reopenStep(ctx, int32(number), why)
		})

	sc.Step(`^the operator reopens step (\d+) saying nothing$`,
		func(ctx context.Context, number int) error {
			return reopenStep(ctx, int32(number), "")
		})

	sc.Step(`^step (\d+) says its result is "([^"]*)"$`,
		func(ctx context.Context, number int, want string) error {
			step, err := stepNumbered(ctx, int32(number))
			if err != nil {
				return err
			}
			if got := step.GetResult(); got != want {
				return fmt.Errorf("step %d says its result is %q, want %q", number, got, want)
			}
			return nil
		})

	sc.Step(`^step (\d+) says nothing came of it$`, func(ctx context.Context, number int) error {
		step, err := stepNumbered(ctx, int32(number))
		if err != nil {
			return err
		}
		if got := step.GetResult(); got != "" {
			return fmt.Errorf("step %d says its result is %q, and nobody closed it", number, got)
		}
		if step.GetFinishedAt() != nil {
			return fmt.Errorf("step %d carries the moment %v, and nobody closed it",
				number, step.GetFinishedAt())
		}
		return nil
	})

	// Who spoke the word, and when. The two are stamped in the write that closes the step, so a step
	// that reads as done and says neither is a record nobody can read.
	sc.Step(`^step (\d+) says "([^"]*)" closed it$`,
		func(ctx context.Context, number int, want string) error {
			step, err := stepNumbered(ctx, int32(number))
			if err != nil {
				return err
			}
			if got := step.GetClosedBy(); got != want {
				return fmt.Errorf("step %d says %q closed it, want %q", number, got, want)
			}
			if step.GetFinishedAt() == nil {
				return fmt.Errorf("step %d carries no moment, so nothing says when it finished", number)
			}
			return nil
		})

	// The two columns a reopen clears. A step back in state taken still naming a closer, or still
	// carrying the moment it finished, is a row that says two things at once.
	sc.Step(`^step (\d+) says nobody closed it$`, func(ctx context.Context, number int) error {
		step, err := stepAsItStands(ctx, int32(number))
		if err != nil {
			return err
		}
		if got := step.GetClosedBy(); got != "" {
			return fmt.Errorf("step %d still says %q closed it", number, got)
		}
		if step.GetFinishedAt() != nil {
			return fmt.Errorf("step %d still carries the moment %v it finished",
				number, step.GetFinishedAt())
		}
		return nil
	})

	// The step and the session are separate records. A finish that cleared either of these would take
	// away the record of who did the work.
	sc.Step(`^step (\d+) still names the session that took it$`, func(ctx context.Context, number int) error {
		p := pathFrom(ctx)
		if p.take == nil {
			return fmt.Errorf("no step was taken, so nothing holds one")
		}
		step, err := stepNumbered(ctx, int32(number))
		if err != nil {
			return err
		}
		started := p.take.GetSession()
		if step.GetSession() != started.GetHandle() && step.GetSession() != started.GetId() {
			return fmt.Errorf("step %d names session %q, and the session that took it is %q",
				number, step.GetSession(), started.GetId())
		}
		if step.GetTakenAt() == nil {
			return fmt.Errorf("step %d carries no take stamp, so nothing says when it was taken", number)
		}
		if was := p.take.GetStep().GetTakenAt().AsTime(); !step.GetTakenAt().AsTime().Equal(was) {
			return fmt.Errorf("step %d was taken at %v and now reads %v", number, was, step.GetTakenAt())
		}
		return nil
	})

	// Nothing dispatches, stops or reclaims a session as a consequence of the word. The session the
	// take started is read back whole.
	sc.Step(`^the session that took it is untouched$`, func(ctx context.Context) error {
		w, p := worldFrom(ctx), pathFrom(ctx)
		if p.take == nil {
			return fmt.Errorf("no step was taken, so no session holds one")
		}
		started := p.take.GetSession()
		read, err := w.client.GetSession(ctx, &quaycrewv1.GetSessionRequest{Id: started.GetId()})
		if err != nil {
			return err
		}
		session := read.GetSession()
		// The status is not compared against the one the take answered with, because the exec the take
		// started settles on its own and moves it. What the word may never do is put the session down
		// or take its container back.
		if session.GetStatus() == "stopped" {
			return fmt.Errorf("the session reads as stopped, and nothing stopped it")
		}
		if session.GetReclaimedAt() != nil {
			return fmt.Errorf("the session was reclaimed at %v", session.GetReclaimedAt())
		}
		if session.GetHandle() != started.GetHandle() {
			return fmt.Errorf("the session reads handle %q, and it took the step as %q",
				session.GetHandle(), started.GetHandle())
		}
		return nil
	})

	// Counted rather than looked at, because a second exec is what a call that dispatched something
	// would leave behind, and a check that one exec happened passes against two.
	sc.Step(`^the model was asked (\d+) things? in all$`, func(ctx context.Context, want int) error {
		if got := worldFrom(ctx).runner.count(); got != want {
			return fmt.Errorf("the model was asked %d things, want %d", got, want)
		}
		return nil
	})

	// The trust record. Every one of these reads the design back through the control plane rather
	// than out of what a write answered, because the question each scenario asks is what the record
	// holds after the write and not what one call said about it.

	sc.Step(`^step (\d+) says the operator agreed$`, func(ctx context.Context, number int) error {
		return theStepAgreement(ctx, int32(number), store.AgreedYes)
	})

	sc.Step(`^step (\d+) says the operator did not agree$`, func(ctx context.Context, number int) error {
		return theStepAgreement(ctx, int32(number), store.AgreedNo)
	})

	sc.Step(`^the run of agreements is (\d+)$`, func(ctx context.Context, want int) error {
		design, err := theTrustRecord(ctx)
		if err != nil {
			return err
		}
		if got := design.GetTrustRun(); got != int32(want) {
			return fmt.Errorf("the run of agreements is %d, want %d", got, want)
		}
		return nil
	})

	sc.Step(`^the trust level is (\d+)$`, func(ctx context.Context, want int) error {
		design, err := theTrustRecord(ctx)
		if err != nil {
			return err
		}
		if got := design.GetTrustLevel(); got != int32(want) {
			return fmt.Errorf("the trust level is %d, want %d", got, want)
		}
		return nil
	})

	// Both totals in one line, because a scenario that asserted one of them would pass against a
	// write that counted the wrong one.
	sc.Step(`^the trust record counts (\d+) agreements? and (\d+) disagreements?$`,
		func(ctx context.Context, agreements, disagreements int) error {
			design, err := theTrustRecord(ctx)
			if err != nil {
				return err
			}
			if got := design.GetTrustAgreements(); got != int32(agreements) {
				return fmt.Errorf("the record counts %d agreements, want %d", got, agreements)
			}
			if got := design.GetTrustDisagreements(); got != int32(disagreements) {
				return fmt.Errorf("the record counts %d disagreements, want %d", got, disagreements)
			}
			return nil
		})

	sc.Step(`^the caller reads the trust record$`, func(ctx context.Context) error {
		return runTool(ctx, "trust", whereTheProjectIs(ctx))
	})

	// The offer, and the raise. Krewe earns the next level by agreeing with the operator over and
	// over, and it offers rather than takes: nothing in this block moves a level except the word the
	// operator types.

	sc.Step(`^the project's trust threshold is (\d+)$`, func(ctx context.Context, number int) error {
		return setTheTrustThreshold(ctx, int32(number))
	})

	sc.Step(`^the operator sets the trust threshold to (-?\d+)$`, func(ctx context.Context, number int) error {
		return setTheTrustThreshold(ctx, int32(number))
	})

	sc.Step(`^the trust threshold is (\d+)$`, func(ctx context.Context, want int) error {
		design, err := theTrustRecord(ctx)
		if err != nil {
			return err
		}
		if got := design.GetTrustThreshold(); got != int32(want) {
			return fmt.Errorf("the trust threshold is %d, want %d", got, want)
		}
		return nil
	})

	// Read off the row rather than off what a write answered, because an offer that lived only in one
	// response is an offer no later command could find.
	sc.Step(`^krewe is offered the next level$`, func(ctx context.Context) error {
		return theStandingOffer(ctx, true)
	})

	sc.Step(`^krewe is offered nothing$`, func(ctx context.Context) error {
		return theStandingOffer(ctx, false)
	})

	sc.Step(`^the operator accepts the offer$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		_, err := w.client.RaiseTrust(ctx, &quaycrewv1.RaiseTrustRequest{Project: w.projectID})
		w.lastErr = err
		return nil
	})

	sc.Step(`^the caller accepts the offer$`, func(ctx context.Context) error {
		return runTool(ctx, "trust", "raise", whereTheProjectIs(ctx))
	})

	sc.Step(`^the caller sets the trust threshold to "([^"]*)"$`, func(ctx context.Context, said string) error {
		return runTool(ctx, "trust", "threshold", whereTheProjectIs(ctx), said)
	})

	// The two calls that move the word done, asked for with the driver's own token. A session that
	// could raise its own level would be handing itself the word it was supposed to earn, and one
	// that could lower the threshold would be writing its own offer.
	sc.Step(`^the driver asks to raise the trust level$`, func(ctx context.Context) error {
		return asDriver(ctx, func(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient) error {
			_, err := client.RaiseTrust(ctx, &quaycrewv1.RaiseTrustRequest{
				Project: worldFrom(ctx).projectID})
			return err
		})
	})

	sc.Step(`^the driver asks to set the trust threshold$`, func(ctx context.Context) error {
		return asDriver(ctx, func(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient) error {
			_, err := client.SetTrustThreshold(ctx, &quaycrewv1.SetTrustThresholdRequest{
				Project: worldFrom(ctx).projectID, Threshold: 1})
			return err
		})
	})

	sc.Step(`^the caller marks step "([^"]*)" done with "([^"]*)"$`,
		func(ctx context.Context, said, result string) error {
			return runTool(ctx, "step", "done", whereTheProjectIs(ctx), said, result)
		})

	sc.Step(`^the caller stops step "([^"]*)" with "([^"]*)"$`,
		func(ctx context.Context, said, reason string) error {
			return runTool(ctx, "step", "stop", whereTheProjectIs(ctx), said, reason)
		})

	sc.Step(`^the caller marks step "([^"]*)" done without saying what came of it$`,
		func(ctx context.Context, said string) error {
			return runTool(ctx, "step", "done", said)
		})

	// The token as a person types it, whatever is in it, because the refusals are about the shape of
	// what somebody typed.
	sc.Step(`^the caller takes step "([^"]*)"$`, func(ctx context.Context, said string) error {
		return runTool(ctx, "step", "take", whereTheProjectIs(ctx), said)
	})

	sc.Step(`^the caller takes a step without saying which one$`, func(ctx context.Context) error {
		return runTool(ctx, "step", "take")
	})

	// The warnings. None of them refuses a document, so every one of these reads the path back too.

	sc.Step(`^the path write warns that step (\d+) says nothing under "([^"]*)"$`,
		func(ctx context.Context, number int, label string) error {
			p := pathFrom(ctx)
			want := fmt.Sprintf("step %d says nothing under", number)
			for _, warning := range p.warnings {
				if strings.Contains(warning, want) && strings.Contains(warning, label) {
					return nil
				}
			}
			return fmt.Errorf("the write warned %q, and none of it says step %d left %q empty",
				p.warnings, number, label)
		})

	sc.Step(`^the path write warns "([^"]*)"$`, func(ctx context.Context, want string) error {
		p := pathFrom(ctx)
		for _, warning := range p.warnings {
			if strings.Contains(warning, want) {
				return nil
			}
		}
		return fmt.Errorf("the write warned %q, and none of it says %q", p.warnings, want)
	})

	sc.Step(`^the path write warns about nothing$`, func(ctx context.Context) error {
		if warnings := pathFrom(ctx).warnings; len(warnings) != 0 {
			return fmt.Errorf("the write warned %q about a path that says everything", warnings)
		}
		return nil
	})

	// The refusals. Each one names the line, because a document is a file somebody has to go and fix.

	sc.Step(`^the refusal names line (\d+)$`, func(ctx context.Context, line int) error {
		return refusalNamesLines(ctx, line)
	})

	sc.Step(`^the refusal names lines (\d+) and (\d+)$`, func(ctx context.Context, first, second int) error {
		return refusalNamesLines(ctx, first, second)
	})

	// The steps that drive the real command line tool, as a caller runs it.

	sc.Step(`^a path file saying:$`, func(ctx context.Context, document *godog.DocString) error {
		p := pathFrom(ctx)
		dir, err := os.MkdirTemp("", "krewe-path-")
		if err != nil {
			return err
		}
		p.file = filepath.Join(dir, "path.md")
		return os.WriteFile(p.file, []byte(document.Content), 0o600)
	})

	sc.Step(`^the caller (?:writes|wrote) the path from that file$`, func(ctx context.Context) error {
		held, err := theFeature(ctx)
		if err != nil {
			return err
		}
		return runTool(ctx, "path", "set", whereTheProjectIs(ctx),
			strconv.FormatInt(int64(held.GetNumber()), 10), "--file", pathFrom(ctx).file)
	})

	sc.Step(`^the caller (?:writes|wrote) the path of feature (\d+) from that file$`,
		func(ctx context.Context, number int) error {
			return runTool(ctx, "path", "set", whereTheProjectIs(ctx),
				strconv.Itoa(number), "--file", pathFrom(ctx).file)
		})

	sc.Step(`^the caller writes the path without naming a file$`, func(ctx context.Context) error {
		return runTool(ctx, "path", "set", whereTheProjectIs(ctx), "1")
	})

	sc.Step(`^the caller writes the path without saying which feature$`, func(ctx context.Context) error {
		return runTool(ctx, "path", "set", whereTheProjectIs(ctx), "--file", pathFrom(ctx).file)
	})

	sc.Step(`^the caller reads the path$`, func(ctx context.Context) error {
		return runTool(ctx, "path", whereTheProjectIs(ctx))
	})

	sc.Step(`^the caller reads the path of feature (\d+)$`, func(ctx context.Context, number int) error {
		return runTool(ctx, "path", whereTheProjectIs(ctx), strconv.Itoa(number))
	})

	// One step whole, which is the read the listing above cannot answer: a row holds a line, and an
	// intention, a list of files and the end of a failed run do not fit on one.
	sc.Step(`^the caller reopens step "([^"]*)" with "([^"]*)"$`,
		func(ctx context.Context, said, why string) error {
			return runTool(ctx, "step", "reopen", whereTheProjectIs(ctx), said, why)
		})

	sc.Step(`^the caller shows step "([^"]*)"$`, func(ctx context.Context, said string) error {
		return runTool(ctx, "step", "show", whereTheProjectIs(ctx), said)
	})

	// Counted off the printed lines rather than asked of the system again, because what this proves
	// is what the operator is looking at.
	sc.Step(`^standard output lists (\d+) steps in number order$`, func(ctx context.Context, want int) error {
		numbers := stepLinesOf(toolFrom(ctx).stdout)
		if len(numbers) != want {
			return fmt.Errorf("standard output lists %d steps, want %d: %q",
				len(numbers), want, toolFrom(ctx).stdout)
		}
		for at := 1; at < len(numbers); at++ {
			if numbers[at-1] >= numbers[at] {
				return fmt.Errorf("the listing reads %s, which is not number order", strings.Join(numbers, ", "))
			}
		}
		return nil
	})

	// The count alone, for a listing grouped under milestones: the steps of one path read in number
	// order inside a group and the groups are in number order, so the whole listing is not one run of
	// ascending numbers.
	sc.Step(`^standard output lists (\d+) step lines$`, func(ctx context.Context, want int) error {
		if got := stepLinesOf(toolFrom(ctx).stdout); len(got) != want {
			return fmt.Errorf("standard output lists %d step lines, want %d: %q",
				len(got), want, toolFrom(ctx).stdout)
		}
		return nil
	})

	// Order asserted as an order and not as a set, for the reason the steps are: a check that both
	// headings are somewhere on the screen passes against a listing drawn upside down.
	sc.Step(`^the heading "([^"]*)" prints before the heading "([^"]*)"$`,
		func(ctx context.Context, first, second string) error {
			printed := toolFrom(ctx).stdout
			above, below := strings.Index(printed, first), strings.Index(printed, second)
			if above < 0 {
				return fmt.Errorf("standard output carries no heading %q: %q", first, printed)
			}
			if below < 0 {
				return fmt.Errorf("standard output carries no heading %q: %q", second, printed)
			}
			if above > below {
				return fmt.Errorf("%q prints after %q: %q", first, second, printed)
			}
			return nil
		})

	// What a read is allowed to cost. A listing that took a step, or moved one, would leave a path
	// somebody has to put back.
	sc.Step(`^every step of the path is ready and held by nobody$`, func(ctx context.Context) error {
		held, err := theFeature(ctx)
		if err != nil {
			return err
		}
		resp, err := worldFrom(ctx).client.ListSteps(ctx, &quaycrewv1.ListStepsRequest{Feature: held.GetId()})
		if err != nil {
			return err
		}
		for _, step := range resp.GetSteps() {
			if step.GetState() != "ready" || step.GetSession() != "" {
				return fmt.Errorf("step %d reads as %q held by %q, and reading the path moves nothing",
					step.GetNumber(), step.GetState(), step.GetSession())
			}
		}
		return nil
	})
}

// stepLinesOf reads the step numbers off a printed listing.
//
// A step line is indented under the heading it sits below, and it opens with the step's number. That
// is what tells it apart from the headings and from the counts, and it is one definition rather than
// a filter in each assertion, so the two cannot disagree about what a step line is.
func stepLinesOf(printed string) []string {
	var numbers []string
	for _, line := range strings.Split(printed, "\n") {
		if !strings.HasPrefix(line, " ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if _, err := strconv.Atoi(fields[0]); err != nil {
			continue
		}
		numbers = append(numbers, fields[0])
	}
	return numbers
}

// takeStep gives one step to a session and waits for that session's exec to land, because the take
// lets go of it: an assertion about what the session was given, or about the file it reads, would
// otherwise run while the exec was still starting.
func takeStep(ctx context.Context, feature string, number int32) error {
	w, p := worldFrom(ctx), pathFrom(ctx)
	resp, err := w.client.TakeStep(ctx, &quaycrewv1.TakeStepRequest{
		Feature: feature, Number: number,
	})
	w.lastErr = err
	if err != nil {
		return nil
	}
	p.take = resp
	// Written down the way a dispatch is, so the steps that read the session's own working directory
	// find the session the take started.
	w.execs = append(w.execs, dispatched{
		sessionID: resp.GetSession().GetId(), handle: resp.GetSession().GetHandle()})
	return w.settled(ctx)
}

// stepHeldByTheTake reads one step back from the control plane and says whether the last take is what
// holds it. It reads the store rather than what the take answered, because a take that answered with a
// session and wrote none is exactly what this is for.
func stepHeldByTheTake(ctx context.Context, feature string, number int32) error {
	w, p := worldFrom(ctx), pathFrom(ctx)
	if p.take == nil {
		return fmt.Errorf("no step was taken, so nothing holds one")
	}
	resp, err := w.client.ListSteps(ctx, &quaycrewv1.ListStepsRequest{Feature: feature})
	if err != nil {
		return err
	}
	for _, step := range resp.GetSteps() {
		if step.GetNumber() != number {
			continue
		}
		if step.GetState() != "taken" {
			return fmt.Errorf("step %d reads as %q after a take", number, step.GetState())
		}
		started := p.take.GetSession()
		if step.GetSession() != started.GetHandle() && step.GetSession() != started.GetId() {
			return fmt.Errorf("step %d names session %q, and the session that took it is %q",
				number, step.GetSession(), started.GetId())
		}
		return nil
	}
	return fmt.Errorf("the path has no step %d", number)
}

// stepAsItStands reads one step of the feature a scenario means when it names none, from the control
// plane now. The assertions about a restatement go through it because the text arrives on an exec
// rather than in the answer to any call, so a read the scenario made earlier says nothing about it.
func stepAsItStands(ctx context.Context, number int32) (*quaycrewv1.Step, error) {
	held, err := theFeature(ctx)
	if err != nil {
		return nil, err
	}
	if err := readPath(ctx, held.GetId()); err != nil {
		return nil, err
	}
	return stepNumbered(ctx, number)
}

// writeRestatement puts a text into the session's own memory file, under the mark, the way a session
// writes into its own memory: the directory is mounted in, so this process and that container are
// looking at one place. The section it already carries is taken out first, because a session that
// restates a second time edits its own section rather than writing a second one under the same mark.
//
// The mark is written out here rather than read from the package, so a constant that comes back under
// another name does not take the check with it.
func writeRestatement(ctx context.Context, text string) error {
	dir, err := sessionWorkingDir(ctx)
	if err != nil {
		return err
	}
	existing, _ := sandbox.ReadMemory(dir)
	kept, _ := sandbox.WithoutSection(existing, "restatement")
	return sandbox.WriteMemory(dir,
		strings.TrimRight(kept, "\n")+"\n\n<!-- quay:restatement -->\n"+text+"\n")
}

// stepRead is what the last read of one step answered, and a refusal to assert on nothing when no
// read landed. A scenario whose read was refused has an error to say so, and asserting against an
// empty answer would read as a step that carries no restatement.
func stepRead(ctx context.Context) (*quaycrewv1.GetStepResponse, error) {
	w, p := worldFrom(ctx), pathFrom(ctx)
	if w.lastErr != nil {
		return nil, fmt.Errorf("the read was refused: %w", w.lastErr)
	}
	if p.read == nil {
		return nil, fmt.Errorf("no step was read, so nothing was answered")
	}
	return p.read, nil
}

// lastApproval is what the last approval of a restatement answered, and a refusal to assert on
// nothing when none landed. A scenario whose approval was refused has an error to say so, and
// asserting against an empty answer would read as an approval that dispatched a session with no text.
func lastApproval(ctx context.Context) (*quaycrewv1.ApproveRestatementResponse, error) {
	w, p := worldFrom(ctx), pathFrom(ctx)
	if w.lastErr != nil {
		return nil, fmt.Errorf("the approval was refused: %w", w.lastErr)
	}
	if len(p.approvals) == 0 {
		return nil, fmt.Errorf("nothing was approved, so no session was given any text")
	}
	return p.approvals[len(p.approvals)-1], nil
}

// takenText is what the last take composed, and a refusal to assert on nothing when no take landed.
func takenText(ctx context.Context) (string, error) {
	p := pathFrom(ctx)
	if p.take == nil {
		return "", fmt.Errorf("no step was taken, so no session was given any text")
	}
	return p.take.GetText(), nil
}

// setPath writes the document to the feature a scenario means when it names none.
func setPath(ctx context.Context, document string) error {
	held, err := theFeature(ctx)
	if err != nil {
		return err
	}
	return setPathOf(ctx, held.GetId(), document)
}

// setPathOf writes the document to one feature and keeps what came back, so a later step reads the
// same answer the caller got rather than asking again.
func setPathOf(ctx context.Context, feature, document string) error {
	w, p := worldFrom(ctx), pathFrom(ctx)
	resp, err := w.client.SetPath(ctx, &quaycrewv1.SetPathRequest{
		Feature: feature, Document: document,
	})
	w.lastErr = err
	if err != nil {
		return nil
	}
	p.steps, p.warnings = resp.GetSteps(), resp.GetWarnings()
	return nil
}

// recordAVerdict writes what one run of a step's scenario reported, on the feature a scenario means
// when it names none.
//
// The output reads like the runner's, because the path a session reads carries it and a scenario
// about that file would otherwise carry a line nothing ever wrote.
func recordAVerdict(ctx context.Context, number int32, state string) error {
	held, err := theFeature(ctx)
	if err != nil {
		return err
	}
	return recordAVerdictOf(ctx, held.GetId(), number, state)
}

// recordAVerdictOf writes one run's verdict onto a step of the feature it names, which is what the
// scenarios running two features need.
func recordAVerdictOf(ctx context.Context, feature string, number int32, state string) error {
	output := "1 scenarios (1 passed)"
	if state == store.ProofFailing {
		output = "1 scenarios (0 passed, 1 failed)"
	}
	_, err := worldFrom(ctx).store.RecordProof(ctx, feature, number, store.ProofResult{
		State: state, ScenariosRun: 1, Output: output,
	})
	return err
}

// theStepAgreement reads one step back and holds its record of the operator's word to what the
// scenario says. The step is read from the path the last write left, which is what every other
// assertion here reads.
func theStepAgreement(ctx context.Context, number int32, want string) error {
	step, err := stepNumbered(ctx, number)
	if err != nil {
		return err
	}
	if got := step.GetOperatorAgreed(); got != want {
		return fmt.Errorf("step %d says the operator agreed %q, want %q", number, got, want)
	}
	return nil
}

// theTrustRecord reads the project's design back, which is where the trust record lives. It is read
// through the control plane rather than out of what the finish answered, because what these
// scenarios ask is what the record holds now.
func theTrustRecord(ctx context.Context) (*quaycrewv1.Design, error) {
	w := worldFrom(ctx)
	resp, err := w.client.GetDesign(ctx, &quaycrewv1.GetDesignRequest{Project: w.projectID})
	if err != nil {
		return nil, fmt.Errorf("read the trust record: %w", err)
	}
	return resp.GetDesign(), nil
}

// setTheTrustThreshold records how many agreements in a row earn an offer, keeping what came back so
// a Then step reads the refusal as well as the design.
func setTheTrustThreshold(ctx context.Context, threshold int32) error {
	w := worldFrom(ctx)
	_, err := w.client.SetTrustThreshold(ctx, &quaycrewv1.SetTrustThresholdRequest{
		Project: w.projectID, Threshold: threshold,
	})
	w.lastErr = err
	return nil
}

// theStandingOffer reads the design back and holds whether krewe is asking for the next level to what
// the scenario says.
//
// It reads the row rather than what a write answered, because an offer stands until the operator
// answers it: what these scenarios ask is what a later command would find.
func theStandingOffer(ctx context.Context, want bool) error {
	design, err := theTrustRecord(ctx)
	if err != nil {
		return err
	}
	if got := design.GetTrustOffered(); got != want {
		return fmt.Errorf("krewe is offered the next level: %v, want %v (the run is %d against a threshold of %d, at level %d)",
			got, want, design.GetTrustRun(), design.GetTrustThreshold(), design.GetTrustLevel())
	}
	return nil
}

// finishStep closes a step of the feature a scenario means when it names none.
func finishStep(ctx context.Context, number int32, state, result string) error {
	held, err := theFeature(ctx)
	if err != nil {
		return err
	}
	return finishStepOf(ctx, held.GetId(), number, state, result)
}

// finishStepOf closes one step of the feature it names, and reads the path back, so an assertion
// reads the record rather than the answer the call gave.
//
// A refusal is kept the way every other refusal here is kept, and the path is left as it stood, so a
// scenario about a refused finish reads what the write did not change.
func finishStepOf(ctx context.Context, feature string, number int32, state, result string) error {
	w := worldFrom(ctx)
	_, err := w.client.FinishStep(ctx, &quaycrewv1.FinishStepRequest{
		Feature: feature, Number: number, State: state, Result: result,
	})
	w.lastErr = err
	if err != nil {
		return nil
	}
	return readPath(ctx, feature)
}

// reopenStep takes one step back off krewe, on the feature a scenario means when it names none, and
// reads the path back so an assertion reads the record rather than the answer the call gave.
//
// A refusal is kept the way every other refusal here is kept, and the path is left as it stood, so a
// scenario about a refused reopen reads what the write did not change.
func reopenStep(ctx context.Context, number int32, why string) error {
	w := worldFrom(ctx)
	held, err := theFeature(ctx)
	if err != nil {
		return err
	}
	_, err = w.client.ReopenStep(ctx, &quaycrewv1.ReopenStepRequest{
		Feature: held.GetId(), Number: number, Why: why,
	})
	w.lastErr = err
	if err != nil {
		return nil
	}
	return readPath(ctx, held.GetId())
}

// readPath reads one feature's path into the world the assertions go through.
func readPath(ctx context.Context, feature string) error {
	w, p := worldFrom(ctx), pathFrom(ctx)
	resp, err := w.client.ListSteps(ctx, &quaycrewv1.ListStepsRequest{Feature: feature})
	w.lastErr = err
	if err != nil {
		return nil
	}
	p.steps, p.milestones, p.next = resp.GetSteps(), resp.GetMilestones(), resp.GetNext()
	return nil
}

// refusalNamesLines holds a refusal to naming every line it has to name. A refusal that named one of
// two duplicate numbers would send a person to the line that is fine.
func refusalNamesLines(ctx context.Context, lines ...int) error {
	err := worldFrom(ctx).lastErr
	if err == nil {
		return fmt.Errorf("nothing was refused")
	}
	for _, line := range lines {
		if !strings.Contains(err.Error(), fmt.Sprintf("line %d", line)) {
			return fmt.Errorf("the refusal is %q, want it to name line %d", err.Error(), line)
		}
	}
	return nil
}

// The steps about what reaches the session: the path document in its working directory.

// pathFileAt is where the path sits in a session's working directory, as this process sees it on the
// host.
func pathFileAt(ctx context.Context) (string, error) {
	dir, err := sessionWorkingDir(ctx)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ".krewe", "path.md"), nil
}

func sessionPathFile(ctx context.Context) (string, error) {
	at, err := pathFileAt(ctx)
	if err != nil {
		return "", err
	}
	body, err := os.ReadFile(at)
	if err != nil {
		return "", fmt.Errorf("the session has no path at %s: %w", at, err)
	}
	return string(body), nil
}

// pathHeading matches a step's own line in the rendered document. The document reads the grammar the
// operator writes and hangs it under a feature heading, so a step sits a level lower here than in the
// document that declared it.
var pathHeading = regexp.MustCompile(`(?m)^###\s+(\d+)\.`)

func initializePathRenderSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the session's path file carries "([^"]*)"$`, func(ctx context.Context, want string) error {
		body, err := sessionPathFile(ctx)
		if err != nil {
			return err
		}
		if !strings.Contains(body, unescape(want)) {
			return fmt.Errorf("the path file does not carry %q: %q", unescape(want), body)
		}
		return nil
	})

	sc.Step(`^the session's path file does not carry "([^"]*)"$`, func(ctx context.Context, unwanted string) error {
		body, err := sessionPathFile(ctx)
		if err != nil {
			return err
		}
		if strings.Contains(body, unescape(unwanted)) {
			return fmt.Errorf("the path file carries %q, and it should not: %q", unescape(unwanted), body)
		}
		return nil
	})

	// Read as the list of headings rather than as a search for each number, because the failure this
	// guards against is the right steps in the wrong order, which a search for the text passes.
	sc.Step(`^the session's path file lists steps ([\d, ]+) in that order$`,
		func(ctx context.Context, want string) error {
			body, err := sessionPathFile(ctx)
			if err != nil {
				return err
			}
			var numbers []string
			for _, found := range pathHeading.FindAllStringSubmatch(body, -1) {
				numbers = append(numbers, found[1])
			}
			if got := strings.Join(numbers, ", "); got != strings.TrimSpace(want) {
				return fmt.Errorf("the path file lists steps %s, want %s: %q", got, want, body)
			}
			return nil
		})

	sc.Step(`^the session has no path file$`, func(ctx context.Context) error {
		at, err := pathFileAt(ctx)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(at)
		if err == nil {
			return fmt.Errorf("the session has a path at %s saying %q, and the store holds none", at, body)
		}
		if !os.IsNotExist(err) {
			return err
		}
		return nil
	})

	// The write is made to fail the way a filesystem fails one: the file the render writes is a
	// directory, so the write is refused whoever runs the suite. A mode the test takes away is not
	// enough, because root writes through it and the scenario then proves nothing.
	sc.Step(`^the path document cannot be written$`, func(ctx context.Context) error {
		at, err := pathFileAt(ctx)
		if err != nil {
			return err
		}
		if err := os.Remove(at); err != nil && !os.IsNotExist(err) {
			return err
		}
		return os.Mkdir(at, 0o755)
	})

	// What the model was given, which is what says the exec ran at all. A render that failed and took
	// the exec with it leaves the memory file from the exec before, so an assertion on that file alone
	// passes either way.
	sc.Step(`^the session was asked "([^"]*)"$`, func(ctx context.Context, want string) error {
		if err := worldFrom(ctx).lastErr; err != nil {
			return fmt.Errorf("the exec was refused: %w", err)
		}
		if asked := worldFrom(ctx).runner.lastRequest().Text; asked != want {
			return fmt.Errorf("the session was asked %q, want %q", asked, want)
		}
		return nil
	})
}

// The narrowed parts of a project, which the path will hang off once the step key moves down to
// them. They live beside the path's own steps because a feature is what a path belongs to.

// featureWorld is the features last read, the warnings that came with the last write, the feature the
// last add answered with, and the second project a numbering scenario needs.
type featureWorld struct {
	features []*quaycrewv1.Feature
	warnings []string
	added    *quaycrewv1.Feature
	beside   string
}

type featureKey struct{}

func featuresFrom(ctx context.Context) *featureWorld {
	f, _ := ctx.Value(featureKey{}).(*featureWorld)
	return f
}

// featureNumbered finds one feature of the listing last read. It is the read the assertions go
// through, so a missing feature reports which numbers are there rather than an index out of range.
func featureNumbered(ctx context.Context, number int32) (*quaycrewv1.Feature, error) {
	f := featuresFrom(ctx)
	for _, feature := range f.features {
		if feature.GetNumber() == number {
			return feature, nil
		}
	}
	var held []string
	for _, feature := range f.features {
		held = append(held, fmt.Sprintf("%d", feature.GetNumber()))
	}
	return nil, fmt.Errorf("the project has no feature %d, it holds %s", number, strings.Join(held, ", "))
}

// aLineOf is a line of text of a stated length, for the scenarios about what a length warns about.
func aLineOf(length int) string {
	return strings.Repeat("a", length)
}

func initializeFeatureSteps(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, featureKey{}, &featureWorld{}), nil
	})

	sc.Step(`^the operator adds the feature "([^"]*)"$`, func(ctx context.Context, title string) error {
		return addFeature(ctx, worldFrom(ctx).projectID, title)
	})

	sc.Step(`^the project's feature "([^"]*)"$`, func(ctx context.Context, title string) error {
		if err := addFeature(ctx, worldFrom(ctx).projectID, title); err != nil {
			return err
		}
		return worldFrom(ctx).lastErr
	})

	sc.Step(`^the operator adds a feature with no title$`, func(ctx context.Context) error {
		return addFeature(ctx, worldFrom(ctx).projectID, "")
	})

	// The driver's own token, which is what a session inside a sandbox presents. A design session
	// names the features it is about to write paths for.
	sc.Step(`^the driver adds the feature "([^"]*)"$`, func(ctx context.Context, title string) error {
		w := worldFrom(ctx)
		return asDriver(ctx, func(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient) error {
			_, err := client.AddFeature(ctx, &quaycrewv1.AddFeatureRequest{
				Project: w.projectID, Title: title})
			return err
		})
	})

	sc.Step(`^a project named "([^"]*)" beside it$`, func(ctx context.Context, name string) error {
		w, f := worldFrom(ctx), featuresFrom(ctx)
		resp, err := w.client.CreateProject(ctx, &quaycrewv1.CreateProjectRequest{
			Workspace: w.workspaceID, Name: name})
		if err != nil {
			return err
		}
		// The background's project is the one the other steps mean, so this one is kept apart.
		f.beside = resp.GetProject().GetId()
		return nil
	})

	sc.Step(`^the operator adds the feature "([^"]*)" to the project beside it$`,
		func(ctx context.Context, title string) error {
			f := featuresFrom(ctx)
			if f.beside == "" {
				return fmt.Errorf("no project was made beside it")
			}
			return addFeature(ctx, f.beside, title)
		})

	sc.Step(`^the operator reads the features$`, func(ctx context.Context) error {
		return readFeatures(ctx, worldFrom(ctx).projectID)
	})

	sc.Step(`^the operator reads the features of the project beside it$`, func(ctx context.Context) error {
		f := featuresFrom(ctx)
		if f.beside == "" {
			return fmt.Errorf("no project was made beside it")
		}
		return readFeatures(ctx, f.beside)
	})

	sc.Step(`^the operator reads the features of a project that does not exist$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		_, w.lastErr = w.client.ListFeatures(ctx, &quaycrewv1.ListFeaturesRequest{Project: "no-such-project"})
		return nil
	})

	// Order is what the control plane promises, so it is asserted as an order and not as a set: a
	// check that every number is present passes against a listing drawn in the wrong order.
	sc.Step(`^the features read ([\d, ]+) in that order$`, func(ctx context.Context, wanted string) error {
		var got []string
		for _, feature := range featuresFrom(ctx).features {
			got = append(got, fmt.Sprintf("%d", feature.GetNumber()))
		}
		if reads := strings.Join(got, ", "); reads != wanted {
			return fmt.Errorf("the features read %s, want %s", reads, wanted)
		}
		return nil
	})

	sc.Step(`^the project holds no feature$`, func(ctx context.Context) error {
		if got := len(featuresFrom(ctx).features); got != 0 {
			return fmt.Errorf("the project holds %d features, and the add was refused", got)
		}
		return nil
	})

	sc.Step(`^feature (\d+) is titled "([^"]*)"$`, func(ctx context.Context, number int, want string) error {
		feature, err := featureNumbered(ctx, int32(number))
		if err != nil {
			return err
		}
		if got := feature.GetTitle(); got != want {
			return fmt.Errorf("feature %d is titled %q, want %q", number, got, want)
		}
		return nil
	})

	sc.Step(`^feature (\d+) is open$`, func(ctx context.Context, number int) error {
		feature, err := featureNumbered(ctx, int32(number))
		if err != nil {
			return err
		}
		if got := feature.GetState(); got != "open" {
			return fmt.Errorf("feature %d reads as %q, and nobody has closed it", number, got)
		}
		return nil
	})

	sc.Step(`^feature (\d+) narrows to "([^"]*)"$`, func(ctx context.Context, number int, want string) error {
		feature, err := featureNumbered(ctx, int32(number))
		if err != nil {
			return err
		}
		if got := feature.GetIntention(); got != unescape(want) {
			return fmt.Errorf("feature %d narrows to %q, want %q", number, got, unescape(want))
		}
		return nil
	})

	sc.Step(`^feature (\d+)'s intention is (\d+) characters$`, func(ctx context.Context, number, want int) error {
		feature, err := featureNumbered(ctx, int32(number))
		if err != nil {
			return err
		}
		if got := utf8.RuneCountInString(feature.GetIntention()); got != want {
			return fmt.Errorf("feature %d's intention is %d characters, want %d kept whole", number, got, want)
		}
		return nil
	})

	// The number is the system's to give, so a scenario reads it off the answer to the add rather
	// than off a later listing: what the caller was told is the claim being made.
	sc.Step(`^the feature that was added took number (\d+)$`, func(ctx context.Context, want int) error {
		f := featuresFrom(ctx)
		if f.added == nil {
			return fmt.Errorf("no feature was added, so nothing took a number")
		}
		if got := f.added.GetNumber(); got != int32(want) {
			return fmt.Errorf("the add answered with number %d, want %d", got, want)
		}
		return nil
	})

	sc.Step(`^the feature that was added carries an identifier of its own$`, func(ctx context.Context) error {
		f := featuresFrom(ctx)
		if f.added == nil {
			return fmt.Errorf("no feature was added, so nothing carries an identifier")
		}
		if f.added.GetId() == "" || f.added.GetId() == f.added.GetProject() {
			return fmt.Errorf("the feature is identified %q, and its project is %q",
				f.added.GetId(), f.added.GetProject())
		}
		return nil
	})

	sc.Step(`^the operator sets feature (\d+)'s intention to "([^"]*)"$`,
		func(ctx context.Context, number int, text string) error {
			return setFeatureIntention(ctx, int32(number), unescape(text))
		})

	sc.Step(`^the operator sets feature (\d+)'s intention to (\d+) characters$`,
		func(ctx context.Context, number, length int) error {
			return setFeatureIntention(ctx, int32(number), aLineOf(length))
		})

	// Closing a feature, stopping it, and opening it again. The number is resolved against the project
	// rather than against whatever listing a scenario last read, so a scenario that closed a feature
	// and then read the listing means the same feature both times.
	sc.Step(`^the operator closes feature (\d+)$`, func(ctx context.Context, number int) error {
		return finishFeature(ctx, int32(number), "done")
	})

	sc.Step(`^the operator stops feature (\d+)$`, func(ctx context.Context, number int) error {
		return finishFeature(ctx, int32(number), "stopped")
	})

	sc.Step(`^the operator opens feature (\d+) again$`, func(ctx context.Context, number int) error {
		return finishFeature(ctx, int32(number), "open")
	})

	// The word goes through as it is typed, because what this proves is the refusal of a word nobody
	// can write through the tool.
	sc.Step(`^the operator sets feature (\d+)'s state to "([^"]*)"$`,
		func(ctx context.Context, number int, state string) error {
			return finishFeature(ctx, int32(number), state)
		})

	sc.Step(`^feature (\d+) reads as "([^"]*)"$`, func(ctx context.Context, number int, want string) error {
		feature, err := featureNumbered(ctx, int32(number))
		if err != nil {
			return err
		}
		if got := feature.GetState(); got != want {
			return fmt.Errorf("feature %d reads as %q, want %q", number, got, want)
		}
		return nil
	})

	// Reopening costs nothing and starts nothing, so a warning on it would be noise on the one call
	// that takes nothing away.
	sc.Step(`^the feature write warns nothing$`, func(ctx context.Context) error {
		if warned := featuresFrom(ctx).warnings; len(warned) != 0 {
			return fmt.Errorf("the write warned %q, and it takes nothing away", warned)
		}
		return nil
	})

	sc.Step(`^the feature write warns "([^"]*)"$`, func(ctx context.Context, want string) error {
		f := featuresFrom(ctx)
		for _, warning := range f.warnings {
			if strings.Contains(warning, want) {
				return nil
			}
		}
		return fmt.Errorf("the write warned %q, and none of it says %q", f.warnings, want)
	})

	// Deleting a project takes its features with it. A project here is deleted by a stamp, so the
	// check is that nothing answers with them rather than that a row went.
	sc.Step(`^no feature of that project is left anywhere$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		resp, err := w.client.ListFeatures(ctx, &quaycrewv1.ListFeaturesRequest{})
		if err != nil {
			return err
		}
		for _, feature := range resp.GetFeatures() {
			if feature.GetProject() == w.projectID {
				return fmt.Errorf("feature %d of the deleted project is still listed", feature.GetNumber())
			}
		}
		return nil
	})

	sc.Step(`^reading its features is refused as not found$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		_, err := w.client.ListFeatures(ctx, &quaycrewv1.ListFeaturesRequest{Project: w.projectID})
		if status.Code(err) != codes.NotFound {
			return fmt.Errorf("reading a deleted project's features answered %v, want not found", err)
		}
		return nil
	})

	// The steps that drive the real command line tool, as a caller runs it.

	sc.Step(`^the caller reads the features$`, func(ctx context.Context) error {
		return runTool(ctx, "feature", whereTheProjectIs(ctx))
	})

	sc.Step(`^the caller adds the feature "([^"]*)"$`, func(ctx context.Context, title string) error {
		return runTool(ctx, "feature", "add", whereTheProjectIs(ctx), title)
	})

	sc.Step(`^the caller sets feature (\d+)'s intention to "([^"]*)"$`,
		func(ctx context.Context, number int, text string) error {
			return runTool(ctx, "feature", "intention", whereTheProjectIs(ctx),
				strconv.Itoa(number), text)
		})

	sc.Step(`^the caller sets an intention without saying which feature$`, func(ctx context.Context) error {
		return runTool(ctx, "feature", "intention", whereTheProjectIs(ctx))
	})

	sc.Step(`^the caller closes feature (\d+)$`, func(ctx context.Context, number int) error {
		return runTool(ctx, "feature", "done", whereTheProjectIs(ctx), strconv.Itoa(number))
	})

	sc.Step(`^the caller stops feature (\d+) saying "([^"]*)"$`,
		func(ctx context.Context, number int, reason string) error {
			return runTool(ctx, "feature", "stop", whereTheProjectIs(ctx), strconv.Itoa(number), reason)
		})

	sc.Step(`^the caller opens feature (\d+) again$`, func(ctx context.Context, number int) error {
		return runTool(ctx, "feature", "open", whereTheProjectIs(ctx), strconv.Itoa(number))
	})

	sc.Step(`^the caller closes a feature without saying which one$`, func(ctx context.Context) error {
		return runTool(ctx, "feature", "done")
	})

	// The feature and nothing else, which is what a person standing in a project types. With two
	// arguments the second is the reason, so the shape that says nothing more is this one.
	sc.Step(`^the caller stops feature (\d+) and says nothing more$`, func(ctx context.Context, number int) error {
		return runTool(ctx, "feature", "stop", strconv.Itoa(number))
	})

	// Counted off the printed lines rather than asked of the system again, because what this proves
	// is what the operator is looking at.
	sc.Step(`^standard output lists (\d+) features in number order$`, func(ctx context.Context, want int) error {
		var numbers []string
		for _, line := range strings.Split(toolFrom(ctx).stdout, "\n") {
			fields := strings.Fields(line)
			if len(fields) == 0 || fields[0] == "FEATURE" {
				continue
			}
			numbers = append(numbers, fields[0])
		}
		if len(numbers) != want {
			return fmt.Errorf("standard output lists %d features, want %d: %q",
				len(numbers), want, toolFrom(ctx).stdout)
		}
		for at := 1; at < len(numbers); at++ {
			if numbers[at-1] >= numbers[at] {
				return fmt.Errorf("the listing reads %s, which is not number order", strings.Join(numbers, ", "))
			}
		}
		return nil
	})
}

// addFeature adds one narrowed part to a project and keeps what came back, so a later step reads the
// answer the caller got rather than asking again.
func addFeature(ctx context.Context, project, title string) error {
	w, f := worldFrom(ctx), featuresFrom(ctx)
	resp, err := w.client.AddFeature(ctx, &quaycrewv1.AddFeatureRequest{Project: project, Title: title})
	w.lastErr = err
	if err != nil {
		return nil
	}
	f.added = resp.GetFeature()
	return nil
}

// readFeatures reads one project's features into the world the assertions go through.
func readFeatures(ctx context.Context, project string) error {
	w, f := worldFrom(ctx), featuresFrom(ctx)
	resp, err := w.client.ListFeatures(ctx, &quaycrewv1.ListFeaturesRequest{Project: project})
	w.lastErr = err
	if err != nil {
		return nil
	}
	f.features = resp.GetFeatures()
	return nil
}

// finishFeature says a feature finished, stopped, or is open again, and keeps the warnings so a later
// step reads the answer the caller got. The number is resolved against the project, for the reason
// setFeatureIntention resolves it: the call takes the feature's identifier and the number is what a
// person types.
func finishFeature(ctx context.Context, number int32, state string) error {
	w, f := worldFrom(ctx), featuresFrom(ctx)
	held, err := featureOfProject(ctx, number)
	if err != nil {
		return err
	}
	resp, err := w.client.FinishFeature(ctx, &quaycrewv1.FinishFeatureRequest{
		Feature: held.GetId(), State: state,
	})
	w.lastErr = err
	if err != nil {
		return nil
	}
	f.warnings = resp.GetWarnings()
	return nil
}

// setFeatureIntention says what one feature narrows to. The number is resolved against the project's
// listing, because the call takes the feature's identifier and the number is what a person types.
func setFeatureIntention(ctx context.Context, number int32, text string) error {
	w, f := worldFrom(ctx), featuresFrom(ctx)
	if err := readFeatures(ctx, w.projectID); err != nil {
		return err
	}
	held, err := featureNumbered(ctx, number)
	if err != nil {
		return err
	}
	resp, err := w.client.SetFeatureIntention(ctx, &quaycrewv1.SetFeatureIntentionRequest{
		Feature: held.GetId(), Intention: text,
	})
	w.lastErr = err
	if err != nil {
		return nil
	}
	f.warnings = resp.GetWarnings()
	return nil
}

// What krewe's own run of a step's scenario reported. The run goes through the sandbox the session
// already has, so these scenarios set what one command answers on that sandbox and then read the
// verdict off the step.
//
// Nothing here asks a model. That is the property the scenarios are about as much as the verdict is:
// a check is one command in a container that already exists, and a scenario that woke a model would
// be specifying something else.
func initializeProofRunSteps(sc *godog.ScenarioContext) {
	// The whole way to a step that may be checked, in one line, because nine scenarios need it and
	// the eight lines it takes are about getting there rather than about the check.
	sc.Step(`^a step taken, restated and approved, naming the scenario "([^"]*)"$`,
		func(ctx context.Context, scenario string) error {
			return aStepReadyToCheck(ctx, scenario, true)
		})

	sc.Step(`^a step taken and restated, naming the scenario "([^"]*)"$`,
		func(ctx context.Context, scenario string) error {
			return aStepReadyToCheck(ctx, scenario, false)
		})

	// The same road at the top of the ladder, for the scenarios about krewe closing a step itself. The
	// step under the check is step 2, because step 1 is what bought the level.
	sc.Step(`^krewe is at trust level 1, with step 2 taken, restated and approved, `+
		`naming the scenario "([^"]*)"$`,
		func(ctx context.Context, scenario string) error {
			return aStepReadyToCheckAtLevelOne(ctx, scenario)
		})

	sc.Step(`^the project's proof command is "([^"]*)" inside (\d+) seconds?$`,
		func(ctx context.Context, command string, seconds int) error {
			return setProof(ctx, command, "", int32(seconds))
		})

	// What the command inside the sandbox answers. It is set on the sandbox the session already has,
	// rather than on the provider, because the take made that sandbox before this step runs.
	sc.Step(`^the run answers "([^"]*)" and exits (\d+)$`,
		func(ctx context.Context, output string, code int) error {
			return theRunAnswers(ctx, sandbox.Reply{
				Match: theProofCommandRuns, Out: unescape(output), Err: exitedWith(code)})
		})

	// One letter repeated, with a line at the end nothing else carries, so a run that kept the front
	// rather than the end fails here rather than passing on a length that matched.
	sc.Step(`^the run answers (\d+) characters and exits (\d+)$`,
		func(ctx context.Context, length, code int) error {
			// The count is in the ending, so the run reports one scenario and krewe adds nothing under
			// the output. A note appended to it would move the number of characters that were cut, and
			// the scenario names that number.
			ending := "\n1 scenarios (0 passed, 1 failed)\nthe end of the run\n"
			return theRunAnswers(ctx, sandbox.Reply{
				Match: theProofCommandRuns,
				Out:   strings.Repeat("a", length-len(ending)) + ending,
				Err:   exitedWith(code),
			})
		})

	// A command that runs on past the budget it was given. The delay is longer than any budget a
	// scenario sets, so what ends the run is the budget and never the clock on this machine.
	sc.Step(`^the run never answers$`, func(ctx context.Context) error {
		return theRunAnswers(ctx, sandbox.Reply{
			Match: theProofCommandRuns, Out: "still going", Delay: time.Minute})
	})

	sc.Step(`^the operator checks step (\d+)$`, func(ctx context.Context, number int) error {
		w, p := worldFrom(ctx), pathFrom(ctx)
		held, err := theFeature(ctx)
		if err != nil {
			return err
		}
		p.askedBefore = w.runner.count()
		resp, err := w.client.CheckStep(ctx, &quaycrewv1.CheckStepRequest{
			Feature: held.GetId(), Number: int32(number),
		})
		w.lastErr = err
		if err != nil {
			return nil
		}
		p.checked = resp
		p.checks = append(p.checks, resp)
		return nil
	})

	sc.Step(`^the caller checks step "([^"]*)"$`, func(ctx context.Context, said string) error {
		return runTool(ctx, "step", "check", whereTheProjectIs(ctx), said)
	})

	// Read off the command the sandbox was actually given, because the substitution is the whole of
	// what this call composes: a run of the template proves nothing about one step, and a run of a
	// command with the wrong name in it finds nothing and reports it as a failure.
	sc.Step(`^the run was given "([^"]*)"$`, func(ctx context.Context, want string) error {
		ran, err := whatTheSandboxRan(ctx)
		if err != nil {
			return err
		}
		if !strings.Contains(ran, want) {
			return fmt.Errorf("the sandbox was given %q, want it to carry %q", ran, want)
		}
		return nil
	})

	// The proof command and nothing else. A session`s sandbox is given its own setup when it starts,
	// so a step that read every command ever run in there would fail on work that has nothing to do
	// with the check.
	sc.Step(`^nothing was run$`, func(ctx context.Context) error {
		ran, err := whatTheSandboxRan(ctx)
		if err == nil && strings.Contains(ran, theProofCommandRuns) {
			return fmt.Errorf("the sandbox was given %q, and no scenario was meant to run", ran)
		}
		return nil
	})

	// Counted from before the check, because the setup already asked the model three times: taking the
	// step, reading the restatement and approving it each dispatch one exec.
	sc.Step(`^the check asked no model anything$`, func(ctx context.Context) error {
		w, p := worldFrom(ctx), pathFrom(ctx)
		if p.askedBefore == 0 {
			return fmt.Errorf("no check was run, so there is nothing to count against")
		}
		if got := w.runner.count(); got != p.askedBefore {
			return fmt.Errorf("the model was asked %d things, and %d of them were before the check",
				got, p.askedBefore)
		}
		return nil
	})

	// The word that closes a step is the operator`s until the trust ladder exists, so a check states a
	// verdict and closes nothing whatever the verdict says.
	sc.Step(`^krewe closed nothing$`, func(ctx context.Context) error {
		p := pathFrom(ctx)
		if p.checked == nil {
			return fmt.Errorf("no check was run, so nothing answered")
		}
		if p.checked.GetClosedByKrewe() {
			return fmt.Errorf("the check answered that krewe closed the step")
		}
		step, err := stepAsItStands(ctx, p.checked.GetStep().GetNumber())
		if err != nil {
			return err
		}
		if got := step.GetState(); got != "taken" {
			return fmt.Errorf("the step reads %q after a check, and the check closes nothing", got)
		}
		return nil
	})

	// The answer and the record held to each other, because a response saying krewe closed the step
	// while the row still reads taken is exactly the failure this is here to catch. Every part of the
	// close is read: the word, the closer and the moment.
	sc.Step(`^krewe closed step (\d+)$`, func(ctx context.Context, number int) error {
		p := pathFrom(ctx)
		if p.checked == nil {
			return fmt.Errorf("no check was run, so nothing answered")
		}
		if !p.checked.GetClosedByKrewe() {
			return fmt.Errorf("the check answered that krewe closed nothing")
		}
		step, err := stepAsItStands(ctx, int32(number))
		if err != nil {
			return err
		}
		if got := step.GetState(); got != store.StepDone {
			return fmt.Errorf("step %d reads %q after a check krewe closed", number, got)
		}
		if got := step.GetClosedBy(); got != closedByKrewe {
			return fmt.Errorf("step %d says %q closed it, want %q", number, got, closedByKrewe)
		}
		if step.GetFinishedAt() == nil {
			return fmt.Errorf("step %d carries no moment, so nothing says when krewe closed it", number)
		}
		return nil
	})

	// Carries rather than reads exactly, because the sentence krewe writes is the control plane's and
	// a scenario asks what is in it: the name of the scenario, and the count the run reported.
	sc.Step(`^step (\d+)'s result carries "([^"]*)"$`,
		func(ctx context.Context, number int, want string) error {
			step, err := stepAsItStands(ctx, int32(number))
			if err != nil {
				return err
			}
			if !strings.Contains(step.GetResult(), want) {
				return fmt.Errorf("step %d's result is %q, want it to carry %q",
					number, step.GetResult(), want)
			}
			return nil
		})

	// Read out of the store rather than off the answer to the check, because a call that answered
	// with a verdict and wrote nothing reads the same to its caller and to nobody else.
	sc.Step(`^step (\d+) reads back as (passing|failing|unproven)$`,
		func(ctx context.Context, number int, want string) error {
			step, err := stepAsItStands(ctx, int32(number))
			if err != nil {
				return err
			}
			if got := step.GetProofState(); got != want {
				return fmt.Errorf("step %d reads back as %q, want %q", number, got, want)
			}
			return nil
		})

	sc.Step(`^step (\d+) ran (\d+) scenarios?$`, func(ctx context.Context, number, want int) error {
		step, err := stepAsItStands(ctx, int32(number))
		if err != nil {
			return err
		}
		if got := step.GetProofScenariosRun(); got != int32(want) {
			return fmt.Errorf("step %d ran %d scenarios, want %d", number, got, want)
		}
		return nil
	})

	sc.Step(`^step (\d+)'s output carries "([^"]*)"$`, func(ctx context.Context, number int, want string) error {
		step, err := stepAsItStands(ctx, int32(number))
		if err != nil {
			return err
		}
		if !strings.Contains(step.GetProofOutput(), unescape(want)) {
			return fmt.Errorf("step %d's output is %q, want it to carry %q",
				number, lastOf(step.GetProofOutput()), unescape(want))
		}
		return nil
	})

	// The run itself, not the whole of what is stored: the line saying what was cut sits above it and
	// is not part of the output the run printed.
	sc.Step(`^step (\d+)'s output keeps (\d+) characters of the run$`,
		func(ctx context.Context, number, want int) error {
			step, err := stepAsItStands(ctx, int32(number))
			if err != nil {
				return err
			}
			_, kept, found := strings.Cut(step.GetProofOutput(), "\n")
			if !found {
				return fmt.Errorf("step %d's output carries no line above it: %q",
					number, lastOf(step.GetProofOutput()))
			}
			if len(kept) != want {
				return fmt.Errorf("step %d keeps %d characters of the run, want %d", number, len(kept), want)
			}
			return nil
		})

	sc.Step(`^step (\d+) carries the moment it was checked$`, func(ctx context.Context, number int) error {
		step, err := stepAsItStands(ctx, int32(number))
		if err != nil {
			return err
		}
		if step.GetProofRanAt() == nil {
			return fmt.Errorf("step %d carries no moment, so nothing records that a run happened", number)
		}
		return nil
	})
}

// theProofCommandRuns is the fragment every proof command in these scenarios carries, which is what
// the sandbox matches its answer on. The commands differ in what follows it, and a scenario should
// not have to repeat the whole command line to say what the run printed.
const theProofCommandRuns = "-run"

// closedByKrewe is the word the row carries when krewe closed a step its own check passed. It is the
// control plane's and it is read off the wire, so it is named here rather than compared inline.
const closedByKrewe = "krewe"

// exitedWith is a command that failed, and nothing at all for one that exited zero. A shell reports a
// failure as an exit status, and the double answers the same way.
func exitedWith(code int) error {
	if code == 0 {
		return nil
	}
	return fmt.Errorf("exit status %d", code)
}

// aStepReadyToCheck is a project with an approved design, a path of one step naming a scenario, and a
// session holding that step having restated it.
//
// The restatement is written into the session's own memory file and read back by an exec, which is
// how a restatement reaches a step: it travels through a file because a model writes files and cannot
// make a call.
func aStepReadyToCheck(ctx context.Context, scenario string, approve bool) error {
	if err := anApprovedDesign(ctx); err != nil {
		return err
	}
	if err := setPath(ctx, aPathOf(1, scenario)); err != nil {
		return err
	}
	held, err := theFeature(ctx)
	if err != nil {
		return err
	}
	return takeRestateAndApprove(ctx, held.GetId(), 1, approve)
}

// aStepReadyToCheckAtLevelOne is the same road with the ladder already climbed: a path of two steps,
// the first closed on a verdict krewe agreed with, the level raised on the offer that earned, and the
// second step taken, restated and approved. The step a scenario then checks is the second one.
//
// It climbs the ladder the way an operator does, rather than writing the level onto the row. Only a
// finish makes an offer and only the operator accepts one, so a setup that set the column would be
// proving the close against a state nothing can reach.
//
// The threshold goes to one so the path needs two steps rather than six. What the number is proves
// nothing here, and the scenarios that prove the offer set their own.
func aStepReadyToCheckAtLevelOne(ctx context.Context, scenario string) error {
	w := worldFrom(ctx)
	if err := anApprovedDesign(ctx); err != nil {
		return err
	}
	if err := setPath(ctx, aPathOf(2, scenario)); err != nil {
		return err
	}
	held, err := theFeature(ctx)
	if err != nil {
		return err
	}
	if err := setTheTrustThreshold(ctx, 1); err != nil {
		return err
	}
	if err := recordAVerdictOf(ctx, held.GetId(), 1, store.ProofPassing); err != nil {
		return err
	}
	if err := finishStepOf(ctx, held.GetId(), 1, store.StepDone, "shipped as pull request 736"); err != nil {
		return err
	}
	if w.lastErr != nil {
		return fmt.Errorf("the finish that earns the offer was refused: %w", w.lastErr)
	}
	if _, err := w.client.RaiseTrust(ctx, &quaycrewv1.RaiseTrustRequest{
		Project: w.projectID}); err != nil {
		return fmt.Errorf("accept the offer krewe earned: %w", err)
	}
	return takeRestateAndApprove(ctx, held.GetId(), 2, true)
}

// anApprovedDesign is the design a path hangs off. A step cannot be taken under one nobody approved,
// so every road to a checkable step starts here.
func anApprovedDesign(ctx context.Context) error {
	w := worldFrom(ctx)
	if _, err := w.client.SetDesign(ctx, &quaycrewv1.SetDesignRequest{
		Project: w.projectID, Body: "# Bills\n"}); err != nil {
		return err
	}
	_, err := w.client.ApproveDesign(ctx, &quaycrewv1.ApproveDesignRequest{Project: w.projectID})
	return err
}

// theStepHeadings are the titles these paths are written from, in order. A path of two reads the same
// as the two step paths the trust scenarios already write out by hand.
var theStepHeadings = []string{
	"The store holds a project's brief",
	"The store holds a project's design",
}

// aPathOf is a path document of that many steps, naming the scenario under the last of them.
//
// The scenario goes on the last step because that is the one a check runs. The steps above it are
// there to be closed, which is how a project reaches a trust level at all.
func aPathOf(steps int, scenario string) string {
	document := ""
	for number := 1; number <= steps; number++ {
		document += fmt.Sprintf("## %d. %s\n", number, theStepHeadings[number-1])
		if number == steps && scenario != "" {
			document += "\nThe scenario that proves it\n" + scenario + "\n"
		}
	}
	return document
}

// takeRestateAndApprove puts a session on one step and takes it as far as a check may go: taken,
// restated, and the restatement approved where the caller asks for it.
func takeRestateAndApprove(ctx context.Context, feature string, number int32, approve bool) error {
	w := worldFrom(ctx)
	if err := takeStep(ctx, feature, number); err != nil {
		return err
	}
	if w.lastErr != nil {
		return fmt.Errorf("the take was refused: %w", w.lastErr)
	}
	said := "What this step changes\n" + theStepHeadings[number-1] + "."
	if err := writeRestatement(ctx, said); err != nil {
		return err
	}
	// The exec is what reads the section out of the session's file and onto the step. Without it the
	// text sits in a file nothing has read, and the approval below would have nothing to approve.
	previous, err := w.lastExec()
	if err != nil {
		return err
	}
	if err := w.dispatch(ctx, w.projectID, previous.handle, "and again"); err != nil {
		return err
	}
	if !approve {
		return nil
	}
	resp, err := w.client.ApproveRestatement(ctx, &quaycrewv1.ApproveRestatementRequest{
		Feature: feature, Number: number})
	if err != nil {
		return err
	}
	pathFrom(ctx).approvals = append(pathFrom(ctx).approvals, resp)
	return w.settled(ctx)
}

// theRunAnswers makes one command answer this way inside the sandbox the session holding the step
// already has.
//
// It is set on that sandbox rather than on the provider, because the take made the sandbox before
// this step runs and a provider set afterwards would hand its answers to the next one made.
func theRunAnswers(ctx context.Context, reply sandbox.Reply) error {
	box, err := theSandboxOfTheStep(ctx)
	if err != nil {
		return err
	}
	box.Replies = append(box.Replies, reply)
	return nil
}

// whatTheSandboxRan is every command that sandbox was given, joined, so a scenario reads what ran and
// one that says nothing ran reads the same list empty.
func whatTheSandboxRan(ctx context.Context) (string, error) {
	box, err := theSandboxOfTheStep(ctx)
	if err != nil {
		return "", err
	}
	lines := make([]string, 0, len(box.Ran))
	for _, spec := range box.Ran {
		lines = append(lines, strings.Join(spec.Argv, " "))
	}
	return strings.Join(lines, "\n"), nil
}

// theSandboxOfTheStep is the sandbox of the session holding the step the last take started. It is
// asked of the provider by name, the way the control plane asks for it, so a scenario and the call it
// is about are looking at one container.
func theSandboxOfTheStep(ctx context.Context) (*sandbox.FakeSandbox, error) {
	w, p := worldFrom(ctx), pathFrom(ctx)
	if p.take == nil {
		return nil, fmt.Errorf("no step was taken, so no session holds one")
	}
	box, running, err := w.provider.Existing(ctx, p.take.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	if !running {
		return nil, fmt.Errorf("the session that took the step has no sandbox")
	}
	held, is := box.(*sandbox.FakeSandbox)
	if !is {
		return nil, fmt.Errorf("the session's sandbox is a %T rather than the double", box)
	}
	return held, nil
}

// lastOf is the end of a long text, for a failure message: a run of twelve thousand characters
// printed whole says nothing a reader can find the fault in.
func lastOf(text string) string {
	if len(text) <= 200 {
		return text
	}
	return "..." + text[len(text)-200:]
}

// What a check does when the session holding the step no longer has a container.
//
// A session the system reclaimed keeps everything except its container: the conversation, the step it
// holds and every file it wrote are where they were. So the check starts a container for it rather
// than refusing, and these steps are about the three things that decide whether that works. The
// container is the session's own, so a second check reuses it. A working tree that cannot be restored
// stops the run. The operator is told before the wait rather than after it.
func initializeReclaimedSessionSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the session holding the step was reclaimed$`, func(ctx context.Context) error {
		w, p := worldFrom(ctx), pathFrom(ctx)
		if p.take == nil {
			return fmt.Errorf("no step was taken, so no session holds one")
		}
		held := p.take.GetSession().GetId()
		// Counted before the reclaim, so an assertion after the check counts the containers this
		// slice made and not the one the take made.
		p.containersBefore = containersMadeFor(w, held)
		if _, err := w.client.ReclaimSession(ctx, &quaycrewv1.ReclaimSessionRequest{Id: held}); err != nil {
			return fmt.Errorf("reclaiming the session that took the step: %w", err)
		}
		// The reclaim is the setup rather than the scenario, so it is read back here: a scenario about
		// what a check does with no container is worth nothing if the container is still there.
		if _, running, err := w.provider.Existing(ctx, held); err != nil || running {
			return fmt.Errorf("the session still has a container after the reclaim: running=%v, %v", running, err)
		}
		return nil
	})

	// Set on the provider rather than on a sandbox, because the sandbox this answer is for does not
	// exist yet: the check is what makes it. The provider hands its replies to every sandbox it makes.
	sc.Step(`^the run in the container krewe starts answers "([^"]*)" and exits (\d+)$`,
		func(ctx context.Context, output string, code int) error {
			w := worldFrom(ctx)
			w.provider.Replies = append(w.provider.Replies, sandbox.Reply{
				Match: theProofCommandRuns, Out: unescape(output), Err: exitedWith(code)})
			return nil
		})

	sc.Step(`^the session's working directory cannot be made$`, func(ctx context.Context) error {
		w, p := worldFrom(ctx), pathFrom(ctx)
		if p.take == nil {
			return fmt.Errorf("no step was taken, so no session holds one")
		}
		session := p.take.GetSession()
		dir, kept := w.storage.WorkingDir(sandbox.Config{
			ID: session.GetId(), Workspace: w.workspaceID, Project: w.projectID})
		if !kept {
			return fmt.Errorf("this system keeps no directories, so there is no working tree to break")
		}
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		// A file where the directory belongs, which is a mount source that cannot be made and cannot
		// be mounted. Nothing can restore a working tree onto it.
		return os.WriteFile(dir, []byte("not a directory"), 0o600)
	})

	sc.Step(`^no container can be started, saying "([^"]*)"$`, func(ctx context.Context, said string) error {
		worldFrom(ctx).provider.CreateErr = errors.New(said)
		return nil
	})

	// Counted from the moment the session was reclaimed, so this is the containers the check made.
	sc.Step(`^krewe made the session (\d+) containers?$`, func(ctx context.Context, want int) error {
		return containersMade(ctx, want)
	})

	sc.Step(`^krewe made the session no container$`, func(ctx context.Context) error {
		return containersMade(ctx, 0)
	})

	sc.Step(`^the check warns "([^"]*)"$`, func(ctx context.Context, want string) error {
		p := pathFrom(ctx)
		if p.checked == nil {
			return fmt.Errorf("no check was run, so nothing answered")
		}
		for _, warning := range p.checked.GetWarnings() {
			if strings.Contains(warning, want) {
				return nil
			}
		}
		return fmt.Errorf("the check warned %q, want one of them to carry %q", p.checked.GetWarnings(), want)
	})

	// The second answer rather than the last, because what this says is that the two differ: the
	// first check started a container and the second found it already there.
	sc.Step(`^the second check warns nothing$`, func(ctx context.Context) error {
		p := pathFrom(ctx)
		if len(p.checks) < 2 {
			return fmt.Errorf("%d checks were run, and this is about the second", len(p.checks))
		}
		if warnings := p.checks[1].GetWarnings(); len(warnings) != 0 {
			return fmt.Errorf("the second check warned %q, and it started no container", warnings)
		}
		return nil
	})

	sc.Step(`^the control plane refuses it as a fault of its own$`, func(ctx context.Context) error {
		return refused(worldFrom(ctx), codes.Internal)
	})

	// Read out of the store rather than off the answer, because a call that answered with the right
	// session and moved the row underneath reads the same to its caller.
	sc.Step(`^step (\d+) is still held by the same session$`, func(ctx context.Context, number int) error {
		p := pathFrom(ctx)
		if p.take == nil {
			return fmt.Errorf("no step was taken, so no session holds one")
		}
		step, err := stepAsItStands(ctx, int32(number))
		if err != nil {
			return err
		}
		if got := step.GetSession(); got != p.take.GetStep().GetSession() {
			return fmt.Errorf("step %d is held by %q, and %q took it",
				number, got, p.take.GetStep().GetSession())
		}
		if got := step.GetState(); got != "taken" {
			return fmt.Errorf("step %d reads %q, and making a container closes nothing", number, got)
		}
		return nil
	})

	// The word on the session is the system's record of taking its container back, and a check is not
	// a dispatch. Nothing about the session moves because krewe ran one command in it.
	sc.Step(`^the session holding the step still reads as reclaimed$`, func(ctx context.Context) error {
		w, p := worldFrom(ctx), pathFrom(ctx)
		if p.take == nil {
			return fmt.Errorf("no step was taken, so no session holds one")
		}
		read, err := w.client.GetSession(ctx, &quaycrewv1.GetSessionRequest{Id: p.take.GetSession().GetId()})
		if err != nil {
			return err
		}
		if got := read.GetSession().GetStatus(); got != "reclaimed" {
			return fmt.Errorf("the session reads %q, and making a container moves nothing about it", got)
		}
		return nil
	})

	sc.Step(`^step (\d+)'s restatement is still approved$`, func(ctx context.Context, number int) error {
		step, err := stepAsItStands(ctx, int32(number))
		if err != nil {
			return err
		}
		if !step.GetRestatementApproved() {
			return fmt.Errorf("step %d's restatement reads as unapproved, and a container cleared nothing", number)
		}
		return nil
	})

	// Where one line sits against another, because the whole point of the container line is that it
	// arrives while the operator is waiting rather than with the verdict.
	sc.Step(`^standard output says "([^"]*)" above "([^"]*)"$`,
		func(ctx context.Context, first, second string) error {
			printed := toolFrom(ctx).stdout
			above, below := strings.Index(printed, first), strings.Index(printed, second)
			if above < 0 {
				return fmt.Errorf("standard output is %q, and it does not carry %q", printed, first)
			}
			if below < 0 {
				return fmt.Errorf("standard output is %q, and it does not carry %q", printed, second)
			}
			if above > below {
				return fmt.Errorf("standard output carries %q below %q: %q", first, second, printed)
			}
			return nil
		})
}

// containersMade says how many containers were made for the session holding the step since it was
// reclaimed.
func containersMade(ctx context.Context, want int) error {
	w, p := worldFrom(ctx), pathFrom(ctx)
	if p.take == nil {
		return fmt.Errorf("no step was taken, so no session holds one")
	}
	got := containersMadeFor(w, p.take.GetSession().GetId()) - p.containersBefore
	if got != want {
		return fmt.Errorf("krewe made %d containers for the session, want %d", got, want)
	}
	return nil
}

// containersMadeFor is how many containers this provider has actually made for one session. A
// container it adopted is not one it made, which is what lets a scenario say a second check made none.
func containersMadeFor(w *world, session string) int {
	made := 0
	for _, cfg := range w.provider.Configurations() {
		if cfg.ID == session {
			made++
		}
	}
	return made
}
