package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/contextsize"
	"github.com/atlantic-blue/quay-krewe/internal/store"
)

// The six stages a project is designed in: reading them, writing one, and approving one.
//
// The tool holds no rule about the order. It sends one stage and prints what came back, so the
// refusal an operator reads is the control plane's own words. The six names come from the store,
// which is where both stores read them, because a second list here would let the tool offer a name
// the system does not have.

// flagArtifact names the file a stage's artifact is read from. It is a path and not the json itself
// for the reason a design body is a path: a shell mangles a document, and the file is the thing being
// kept.
const flagArtifact = "--artifact"

const stageUsage = "usage: krewe stage show [<address>]" +
	"\n       krewe stage set [<address>] <stage> " + flagFile + " <path> [" + flagArtifact + " <path>]" +
	"\n       krewe stage approve [<address>] <stage>"

func runStage(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient, args []string, out io.Writer) error {
	if len(args) > 0 && args[0] == "show" {
		return runStageShow(ctx, client, args[1:], out)
	}
	if len(args) > 0 && args[0] == "set" {
		return runStageSet(ctx, client, args[1:], out)
	}
	if len(args) > 0 && args[0] == "approve" {
		return runStageApprove(ctx, client, args[1:], out)
	}
	return fmt.Errorf("%s", stageUsage)
}

// runStageShow prints the six stages and the state of each one.
//
// All six, including the ones nobody wrote. The listing answers "where is this project up to", and a
// listing of the two written stages says nothing about the four that come next.
func runStageShow(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient, args []string, out io.Writer) error {
	if len(args) > 1 {
		return fmt.Errorf("%s", stageUsage)
	}
	typed := ""
	if len(args) == 1 {
		typed = args[0]
	}
	located, err := designProject(ctx, client, typed)
	if err != nil {
		return err
	}
	resp, err := client.ListDesignStages(ctx, &quaycrewv1.ListDesignStagesRequest{Project: located.ProjectID})
	if err != nil {
		return err
	}
	held := resp.GetStages()
	fmt.Fprintf(out, "the six design stages of %s\n\n", located.Path.Project)
	drawStages(out, held)
	next := stageToWriteNext(held)
	if next == "" {
		fmt.Fprintf(out, "\nevery stage carries your word, so the project is ready to build\n")
		return nil
	}
	fmt.Fprintf(out, "\n%s\n", stageAdvice(next, stageNamed(held, next), typed))
	return nil
}

// drawStages writes one row for each of the six, in the order they are written.
func drawStages(out io.Writer, held []*quaycrewv1.DesignStage) {
	rows := make([][]string, 0, len(store.DesignStages()))
	widths := make([]int, stageCells)
	for _, name := range store.DesignStages() {
		row := stageRow(name, stageNamed(held, name))
		for at, cell := range row {
			if len(cell) > widths[at] {
				widths[at] = len(cell)
			}
		}
		rows = append(rows, row)
	}
	for _, row := range rows {
		fmt.Fprintf(out, "   %s\n", strings.TrimRight(padded(row, widths), " "))
	}
}

// stageCells is how many cells a stage row carries: the name, the state, and how much prose it
// holds.
const stageCells = 3

// stageRow is one stage as the listing prints it.
func stageRow(name string, held *quaycrewv1.DesignStage) []string {
	return []string{name, stageState(held), stageSize(held)}
}

// stageState is the word for where one stage stands.
//
// Written and approved are two different states, and the difference is the whole rule: a stage
// sitting there unread is no better than a stage nobody wrote, and the stage after it is refused
// either way. Skipped reads as its own word rather than as approved, because a person deciding what
// to do next wants to know that nobody wrote it.
func stageState(held *quaycrewv1.DesignStage) string {
	switch {
	case held == nil:
		return "empty"
	case held.GetSkipped():
		return "skipped"
	case held.GetApproved():
		return "approved"
	default:
		return "written"
	}
}

// stageSize is how much prose a stage holds, and nothing at all for a stage nobody wrote. The length
// is here because a stage of one line and a stage of ten pages read the same in a state column.
func stageSize(held *quaycrewv1.DesignStage) string {
	if held == nil || held.GetBody() == "" {
		return ""
	}
	return contextsize.Characters(utf8.RuneCountInString(held.GetBody()))
}

// stageToWriteNext is the first of the six that does not carry the operator's word, which is the one
// move the project has. Nothing comes back when all six are settled.
func stageToWriteNext(held []*quaycrewv1.DesignStage) string {
	for _, name := range store.DesignStages() {
		stage := stageNamed(held, name)
		if stage.GetSkipped() || stage.GetApproved() {
			continue
		}
		return name
	}
	return ""
}

// stageAdvice is the line under the listing: write the stage, or approve the one that is written.
//
// The address is left out of the command when the operator typed none, because a command printed
// with a gap where a word goes reads as a word somebody has to fill in.
func stageAdvice(next string, held *quaycrewv1.DesignStage, typed string) string {
	if held.GetBody() != "" {
		return "read it and say so: " + commandWords("krewe stage approve", typed, next)
	}
	return "write it: " + commandWords("krewe stage set", typed, next, flagFile,
		strings.ReplaceAll(next, "_", "-")+".md")
}

// commandWords joins what a command line reads as, leaving out the parts that are empty.
func commandWords(parts ...string) string {
	said := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			said = append(said, part)
		}
	}
	return strings.Join(said, " ")
}

// stageNamed is one stage out of a listing, and nil for a stage nobody wrote.
func stageNamed(held []*quaycrewv1.DesignStage, name string) *quaycrewv1.DesignStage {
	for _, stage := range held {
		if stage.GetStage() == name {
			return stage
		}
	}
	return nil
}

// runStageSet writes one stage's prose, and the artifact beside it.
//
// The body is a file for the reason a design body is one: it is a document, and a path in the command
// is what makes the write repeatable. The artifact is a second file rather than json on the command
// line, because it is a document too and usually a large one.
func runStageSet(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient, args []string, out io.Writer) error {
	rest, path, artifactPath, err := stageFilesOutOf(args)
	if err != nil {
		return err
	}
	if path == "" {
		return fmt.Errorf("%s", stageUsage)
	}
	typed, stage, err := stageAndAddress(rest)
	if err != nil {
		return err
	}
	located, err := designProject(ctx, client, typed)
	if err != nil {
		return err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading the %s stage from %s: %w", stage, path, err)
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return fmt.Errorf("%s is empty, and an empty stage is nothing to agree to", path)
	}
	artifact := ""
	if artifactPath != "" {
		read, err := os.ReadFile(artifactPath)
		if err != nil {
			return fmt.Errorf("reading the artifact for the %s stage from %s: %w", stage, artifactPath, err)
		}
		artifact = string(read)
	}
	resp, err := client.SetDesignStage(ctx, &quaycrewv1.SetDesignStageRequest{
		Project: located.ProjectID, Stage: stage, Body: string(body), Artifact: artifact,
	})
	if err != nil {
		return fmt.Errorf("%w\n\nnothing was written", err)
	}
	fmt.Fprintf(out, "%s has a %s stage: %s\n", located.Path.Project, stage,
		contextsize.Characters(utf8.RuneCountInString(string(body))))
	if artifactPath != "" {
		fmt.Fprintf(out, "it carries an artifact: %s\n",
			contextsize.Characters(utf8.RuneCountInString(artifact)))
	}
	// Said on every write, whether or not this stage carried the word before it. A person who reads
	// it twice learns the rule, which is that approval is a statement about one text.
	fmt.Fprintln(out, "the approval is cleared: a stage that changed is a stage nobody has agreed to yet")
	sayWarnings(out, resp.GetWarnings())
	return nil
}

// runStageApprove records the operator's word on one stage as it stands.
//
// A session is refused this call, so what reaches the store is the operator's own word. It asks
// nothing and opens nothing: the stage is read with krewe stage show, and this approves the text that
// read.
func runStageApprove(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient, args []string, out io.Writer) error {
	typed, stage, err := stageAndAddress(args)
	if err != nil {
		return err
	}
	located, err := designProject(ctx, client, typed)
	if err != nil {
		return err
	}
	resp, err := client.ApproveDesignStage(ctx, &quaycrewv1.ApproveDesignStageRequest{
		Project: located.ProjectID, Stage: stage,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%s has a %s stage you approved, on %s\n", located.Path.Project, stage,
		resp.GetStage().GetApprovedAt().AsTime().Format("2006-01-02 15:04"))
	return nil
}

// stageAndAddress reads which stage a command is about, and the address in front of it.
//
// With one argument the argument is the stage, and with two the first is the address. This is the
// shape krewe step take already has, so an operator standing in a project types the stage alone. The
// name is sent as it was typed: the six are the control plane's vocabulary, and a second copy of the
// refusal here would be a second place it can go stale.
func stageAndAddress(args []string) (typed, stage string, err error) {
	if len(args) == 0 || len(args) > 2 {
		return "", "", fmt.Errorf("%s", stageUsage)
	}
	if len(args) == 2 {
		return args[0], args[1], nil
	}
	return "", args[0], nil
}

// stageFilesOutOf takes the body and the artifact out of the arguments, and hands back everything
// else in the order it was typed. It is the shape fileOutOf has, for the reason that one has it: the
// address and the stage are positions, and a flag may sit anywhere among them.
func stageFilesOutOf(args []string) (rest []string, path, artifact string, err error) {
	rest = make([]string, 0, len(args))
	for at := 0; at < len(args); at++ {
		switch args[at] {
		case flagFile:
			if at+1 >= len(args) {
				return nil, "", "", fmt.Errorf("%s needs a path\n\n%s", flagFile, stageUsage)
			}
			path = args[at+1]
			at++
		case flagArtifact:
			if at+1 >= len(args) {
				return nil, "", "", fmt.Errorf("%s needs a path\n\n%s", flagArtifact, stageUsage)
			}
			artifact = args[at+1]
			at++
		default:
			rest = append(rest, args[at])
		}
	}
	return rest, path, artifact, nil
}
