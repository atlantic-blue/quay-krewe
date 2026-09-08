package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/display"
)

// The path a design was broken into: reading it, and writing it from a file.
//
// The tool sends the document and never parses it. One grammar, in one place, so the console and the
// command line cannot drift on what a step heading looks like.

const pathUsage = "usage: krewe path [<address>] [<feature>]" +
	"\n       krewe path set [<address>] <feature> --file <path>"

// runPath prints one feature's path, or the path of every open feature of the project.
//
// A path belongs to a feature, so the argument is a feature number. It is a bare number and not a
// token, because a feature address has one part.
//
// A closed feature is left out of the whole listing and printed when its number is named, so the
// record of finished work stays readable without filling the listing with it.
func runPath(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient, args []string, out io.Writer) error {
	if len(args) > 0 && args[0] == "set" {
		return runPathSet(ctx, client, args[1:], out)
	}
	if len(args) > 2 {
		return fmt.Errorf("%s", pathUsage)
	}
	typed, said := "", ""
	switch len(args) {
	case 1:
		// One argument is the feature number when it reads as one, and the address otherwise, which
		// is the shape krewe design brief already has.
		if _, err := strconv.Atoi(args[0]); err == nil {
			said = args[0]
		} else {
			typed = args[0]
		}
	case 2:
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
	if len(features) == 0 {
		fmt.Fprintf(out, "%s has no feature yet\n\n", located.Path.Project)
		fmt.Fprintf(out, "add one: krewe feature add %s \"...\"\n", typed)
		return nil
	}
	if said != "" {
		held, err := featureNumbered(features, said, located.Path.Project)
		if err != nil {
			return err
		}
		steps, next, err := printPath(ctx, client, held, typed, out)
		if err != nil {
			return err
		}
		// What is next is a question about one path, so the line is printed for the feature that was
		// named and never under a listing of several.
		if len(steps) > 0 {
			fmt.Fprintf(out, "%s\n", nextLine(next))
		}
		fmt.Fprintln(out)
		return nil
	}
	every := make([]*quaycrewv1.Step, 0)
	for _, feature := range features {
		if feature.GetState() != "open" {
			continue
		}
		steps, _, err := printPath(ctx, client, feature, typed, out)
		if err != nil {
			return err
		}
		every = append(every, steps...)
		fmt.Fprintln(out)
	}
	if len(every) > 0 {
		fmt.Fprintf(out, "the project holds %s\n\n", countOf(every))
	}
	return nil
}

// printPath draws one feature's path under a heading naming the feature, grouped under the milestones
// the feature is delivered in, and hands back the steps it drew and the step that is next.
//
// The heading is there whichever way the command was called, so a listing of several features and a
// listing of one say the same thing about which path is on the screen.
func printPath(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient,
	feature *quaycrewv1.Feature, typed string, out io.Writer) ([]*quaycrewv1.Step, int32, error) {
	resp, err := client.ListSteps(ctx, &quaycrewv1.ListStepsRequest{Feature: feature.GetId()})
	if err != nil {
		return nil, 0, err
	}
	fmt.Fprintf(out, "feature %d: %s\n\n", feature.GetNumber(), feature.GetTitle())
	steps := resp.GetSteps()
	if len(steps) == 0 {
		fmt.Fprintf(out, "this feature has no path yet\n\n")
		fmt.Fprintf(out, "write one: krewe path set %s %d %s path.md\n",
			typed, feature.GetNumber(), flagFile)
		return nil, 0, nil
	}
	drawPath(out, groupPath(resp.GetMilestones(), steps))
	fmt.Fprintf(out, "\n%s\n", countOf(steps))
	return steps, resp.GetNext(), nil
}

// pathGroup is one heading of the listing and the steps that print under it.
type pathGroup struct {
	heading string
	steps   []*quaycrewv1.Step
}

// groupPath puts every step under the milestone it belongs to.
//
// The order is the control plane's throughout. The milestones arrive in number order and the steps
// arrive in number order, and nothing here sorts either of them: two surfaces draw this path, and an
// order worked out in one of them lets the two disagree in front of somebody.
//
// The steps under no milestone print first, because a milestone of 0 is below every milestone number
// and the listing is in number order between milestones as well as inside one. A step whose milestone
// matches no milestone of the feature prints there too. A step that falls off the listing is the
// worst thing this command can do, because what is left still reads as the whole path.
func groupPath(milestones []*quaycrewv1.Milestone, steps []*quaycrewv1.Step) []pathGroup {
	under := func(wanted func(*quaycrewv1.Step) bool) []*quaycrewv1.Step {
		held := make([]*quaycrewv1.Step, 0, len(steps))
		for _, step := range steps {
			if wanted(step) {
				held = append(held, step)
			}
		}
		return held
	}
	named := make(map[int32]bool, len(milestones))
	for _, milestone := range milestones {
		named[milestone.GetNumber()] = true
	}

	grouped := make([]pathGroup, 0, len(milestones)+1)
	loose := under(func(step *quaycrewv1.Step) bool { return !named[step.GetMilestone()] })
	if len(loose) > 0 {
		grouped = append(grouped, pathGroup{heading: "no milestone", steps: loose})
	}
	for _, milestone := range milestones {
		number, title := milestone.GetNumber(), milestone.GetTitle()
		grouped = append(grouped, pathGroup{
			heading: fmt.Sprintf("%d. %s", number, title),
			steps:   under(func(step *quaycrewv1.Step) bool { return step.GetMilestone() == number }),
		})
	}
	return grouped
}

// drawPath writes each group under a heading carrying its count, and the steps under it.
//
// The columns are as wide as the widest cell of the whole feature rather than of one group, so a path
// cut into milestones still reads as one table.
func drawPath(out io.Writer, grouped []pathGroup) {
	widths := make([]int, stepCells)
	rows := make([][][]string, len(grouped))
	for at, group := range grouped {
		for _, step := range group.steps {
			row := stepRow(step)
			for index, cell := range row {
				if len(cell) > widths[index] {
					widths[index] = len(cell)
				}
			}
			rows[at] = append(rows[at], row)
		}
	}
	for at, group := range grouped {
		fmt.Fprintf(out, "%s (%d steps, %d done)\n", group.heading, len(group.steps), doneIn(group.steps))
		for _, row := range rows[at] {
			fmt.Fprintf(out, "   %s\n", padded(row, widths))
		}
	}
}

// stepCells is how many cells a step row carries: the number, the title, the state, the session
// holding it, and how long ago it was taken.
const stepCells = 5

// stepRow is one step as the listing prints it.
func stepRow(step *quaycrewv1.Step) []string {
	return []string{
		strconv.FormatInt(int64(step.GetNumber()), 10),
		step.GetTitle(),
		step.GetState(),
		sessionOn(step),
		display.Age(step.GetTakenAt()),
	}
}

// padded renders one row into aligned columns. Nothing is written after the last cell, because
// trailing spaces are invisible and get copied.
func padded(row []string, widths []int) string {
	var line strings.Builder
	for index, cell := range row {
		if index > 0 {
			line.WriteString("  ")
		}
		line.WriteString(cell)
		if index < len(row)-1 {
			line.WriteString(strings.Repeat(" ", widths[index]-len(cell)))
		}
	}
	return line.String()
}

// stepDone, stepTaken and stepReady are the states a step is counted under. The words are the store's
// and they are read off the wire, so they are named here rather than compared inline: a count line
// that quietly missed a word would read as a path with fewer steps in it.
const (
	stepDone  = "done"
	stepTaken = "taken"
	stepReady = "ready"
)

// countedStates is the order the count line names the states in: what is finished, what is in flight,
// then what is not started yet.
var countedStates = []string{stepDone, stepTaken, stepReady}

// doneIn counts the steps of a group that are finished.
func doneIn(steps []*quaycrewv1.Step) int {
	done := 0
	for _, step := range steps {
		if step.GetState() == stepDone {
			done++
		}
	}
	return done
}

// countOf counts the steps by state.
//
// It names each state that holds at least one step, so the line says what is there rather than
// printing a zero beside every word the system knows. A state the system grows later is named after
// the three below, so nothing goes uncounted.
func countOf(steps []*quaycrewv1.Step) string {
	counted := make(map[string]int, len(countedStates))
	for _, step := range steps {
		counted[step.GetState()]++
	}
	said := []string{fmt.Sprintf("%d steps", len(steps))}
	for _, state := range countedStates {
		if counted[state] > 0 {
			said = append(said, fmt.Sprintf("%d %s", counted[state], state))
		}
		delete(counted, state)
	}
	rest := make([]string, 0, len(counted))
	for state := range counted {
		rest = append(rest, state)
	}
	sort.Strings(rest)
	for _, state := range rest {
		said = append(said, fmt.Sprintf("%d %s", counted[state], state))
	}
	return strings.Join(said, ", ") + "."
}

// nextLine says which step may be taken now, from the number the control plane answered with.
//
// The rule behind the number lives in the control plane, so a second surface that draws this path
// cannot disagree with this one about it. The line is a sentence and nothing else: it names a command
// to type, and printing it starts no session and takes no step.
//
// A number of 0 is an answer rather than a missing one. Every step is taken, or every ready step
// waits for a step nobody finished, and the line says so instead of naming a step nobody may take.
func nextLine(next int32) string {
	if next == 0 {
		return "next: nothing, every step is taken or waiting"
	}
	return fmt.Sprintf("next: step %d", next)
}

// sessionOn is the session holding a step, and a dash where nobody holds it. A dash rather than an
// empty cell, because an empty cell in the middle of a row reads as a column that failed to render.
func sessionOn(step *quaycrewv1.Step) string {
	if step.GetSession() == "" {
		return "-"
	}
	return display.ShortID(step.GetSession())
}

// runPathSet writes one feature's path from a file.
//
// The file goes to the control plane whole. A refused document changes nothing, and the refusal
// names the line, so it is printed as it came rather than rewritten here.
//
// The output names the feature, so nobody reads it as the project's whole path: the other features
// of the project keep the paths they had.
func runPathSet(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient, args []string, out io.Writer) error {
	typed, said, path, err := fileFeatureAndAddressFor("krewe path set", args)
	if err != nil {
		return err
	}
	located, err := designProject(ctx, client, typed)
	if err != nil {
		return err
	}
	features, err := featuresOf(ctx, client, located.ProjectID)
	if err != nil {
		return err
	}
	held, err := featureNumbered(features, said, located.Path.Project)
	if err != nil {
		return err
	}
	document, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading the path from %s: %w", path, err)
	}
	resp, err := client.SetPath(ctx, &quaycrewv1.SetPathRequest{
		Feature: held.GetId(), Document: string(document),
	})
	if err != nil {
		return fmt.Errorf("%w\n\nnothing was written", err)
	}
	steps := resp.GetSteps()
	fmt.Fprintf(out, "feature %d of %s has a path of %d steps\n\n",
		held.GetNumber(), located.Path.Project, len(steps))
	for _, step := range steps {
		fmt.Fprintf(out, "  %d. %s\n", step.GetNumber(), step.GetTitle())
	}
	sayWarnings(out, resp.GetWarnings())
	return nil
}

// fileFeatureAndAddressFor reads --file out of the arguments and hands back the feature number and
// the address in front of it. The command names itself, so the usage a refusal prints is the command
// the operator typed.
//
// With one argument left the argument is the feature number, and with two the first is the address.
func fileFeatureAndAddressFor(command string, args []string) (typed, feature, path string, err error) {
	usage := fmt.Sprintf("usage: %s [<address>] <feature> %s <path>", command, flagFile)
	rest, path, err := fileOutOf(args, usage)
	if err != nil {
		return "", "", "", err
	}
	if path == "" || len(rest) == 0 || len(rest) > 2 {
		return "", "", "", fmt.Errorf("%s", usage)
	}
	// One argument left is the feature number when it reads as one, and the address otherwise, which
	// then leaves no feature named at all.
	feature = rest[0]
	if len(rest) == 2 {
		typed, feature = rest[0], rest[1]
	}
	if _, err := strconv.Atoi(feature); err != nil {
		return "", "", "", fmt.Errorf("%s", usage)
	}
	return typed, feature, path, nil
}

// fileAndAddressFor reads --file out of the arguments and hands back the address in front of it. The
// command names itself, so the usage a refusal prints is the command the operator typed.
func fileAndAddressFor(command string, args []string) (typed, path string, err error) {
	usage := fmt.Sprintf("usage: %s [<address>] %s <path>", command, flagFile)
	rest, path, err := fileOutOf(args, usage)
	if err != nil {
		return "", "", err
	}
	if path == "" || len(rest) > 1 {
		return "", "", fmt.Errorf("%s", usage)
	}
	if len(rest) == 1 {
		typed = rest[0]
	}
	return typed, path, nil
}

// fileOutOf takes --file and its path out of the arguments, and hands back everything else in the
// order it was typed.
func fileOutOf(args []string, usage string) (rest []string, path string, err error) {
	rest = make([]string, 0, len(args))
	for at := 0; at < len(args); at++ {
		if args[at] != flagFile {
			rest = append(rest, args[at])
			continue
		}
		if at+1 >= len(args) {
			return nil, "", fmt.Errorf("%s needs a path\n\n%s", flagFile, usage)
		}
		path = args[at+1]
		at++
	}
	return rest, path, nil
}
