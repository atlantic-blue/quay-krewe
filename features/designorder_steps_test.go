package features_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/atlantic-blue/quay-krewe/internal/skill"
	"github.com/atlantic-blue/quay-krewe/internal/store"
	"github.com/cucumber/godog"
)

// Steps for the order the design command walks the six stages in.
//
// Where the design starts is read by running the command the file names, against a real project with
// real stages on it. A scenario that only read the prose would pass on a file that names the listing
// and then walks an order of its own, which is the break this slice exists to stop.

// designSkillDir is where the skills this build ships live, as the suite sees them. The same
// directory the image carries, so a brief that stopped saying what a session must read fails here
// rather than on somebody's first design.
const designSkillDir = "../skills"

// addressPlaceholder is what a command file writes where the operator's address goes, and stagePart
// is what it writes where the stage it read goes. Both are filled in before anything runs.
const (
	addressPlaceholder = "<workspace>/<project>"
	stagePart          = "<stage>"
)

// designOrderWorld is the design command as the install wrote it, and what the command it names
// answered about this project.
type designOrderWorld struct {
	command string
	reading string
	held    *skill.Skill
}

type designOrderKey struct{}

func designOrderFrom(ctx context.Context) *designOrderWorld {
	d, _ := ctx.Value(designOrderKey{}).(*designOrderWorld)
	return d
}

func initializeDesignOrderSteps(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, designOrderKey{}, &designOrderWorld{}), nil
	})

	sc.Step(`^the operator reads the design command$`, func(ctx context.Context) error {
		return readDesignCommand(ctx)
	})

	// The tool is run with the words the file writes out, and not with words this step chose. A file
	// that reads the design body instead of the stage listing answers with a brief, which names no
	// stage, so every assertion under it fails.
	sc.Step(`^the operator reads where the design starts, the way the design command reads it$`,
		func(ctx context.Context) error {
			if err := readDesignCommand(ctx); err != nil {
				return err
			}
			words, err := stageListingWords(designOrderFrom(ctx).command)
			if err != nil {
				return err
			}
			for at, word := range words {
				if word == addressPlaceholder {
					words[at] = whereTheProjectIs(ctx)
				}
			}
			if err := runTool(ctx, words...); err != nil {
				return err
			}
			designOrderFrom(ctx).reading = toolFrom(ctx).stdout
			return nil
		})

	sc.Step(`^the design starts at "([^"]*)"$`, func(ctx context.Context, stage string) error {
		offered, err := theStageOffered(ctx)
		if err != nil {
			return err
		}
		if !strings.Contains(offered, stage) {
			return fmt.Errorf("the reading offers %q, and the design starts at %s", offered, stage)
		}
		return nil
	})

	// One stage and not two. A line offering the stories and the data model together leaves the
	// operator to pick, which is the state this slice takes away.
	sc.Step(`^the design starts at no other stage$`, func(ctx context.Context) error {
		offered, err := theStageOffered(ctx)
		if err != nil {
			return err
		}
		if named := stagesNamedIn(offered); len(named) != 1 {
			return fmt.Errorf("the reading offers %v, want one stage: %q", named, offered)
		}
		return nil
	})

	sc.Step(`^the reading names no stage of the six$`, func(ctx context.Context) error {
		offered, err := theStageOffered(ctx)
		if err != nil {
			return err
		}
		if named := stagesNamedIn(offered); len(named) != 0 {
			return fmt.Errorf("the reading still offers %v: %q", named, offered)
		}
		return nil
	})

	sc.Step(`^the reading says the project is ready to build$`, func(ctx context.Context) error {
		return says("the reading", designOrderFrom(ctx).reading, "ready to build")
	})

	sc.Step(`^the reading offers that stage for approval$`, func(ctx context.Context) error {
		offered, err := theStageOffered(ctx)
		if err != nil {
			return err
		}
		if !strings.Contains(offered, "krewe stage approve") {
			return fmt.Errorf("the reading says %q, and a written stage is offered for approval", offered)
		}
		return nil
	})

	// The dispatch is where an order of the file's own would show up. A file naming a stage in the
	// text it sends is a file that decided which stage this run is about, and the listing is what
	// decides that.
	sc.Step(`^the design command dispatches a session for the stage it read, and for no stage of its own$`,
		func(ctx context.Context) error {
			body := designOrderFrom(ctx).command
			if body == "" {
				return fmt.Errorf("the design command was never read")
			}
			dispatched := dispatchLines(body)
			if len(dispatched) == 0 {
				return fmt.Errorf("design.md dispatches nothing, so the operator's own session writes the stage")
			}
			carried := false
			for _, line := range dispatched {
				for _, stage := range store.DesignStages() {
					if strings.Contains(line, stage) {
						return fmt.Errorf("design.md dispatches for %s by name: %q", stage, line)
					}
				}
				if strings.Contains(line, stagePart) {
					carried = true
				}
			}
			if !carried {
				return fmt.Errorf("no dispatch carries %s, so nothing tells the session which stage it writes",
					stagePart)
			}
			return nil
		})

	sc.Step(`^the design command reads the stage listing before it runs anything else$`,
		func(ctx context.Context) error {
			body := designOrderFrom(ctx).command
			if body == "" {
				return fmt.Errorf("the design command was never read")
			}
			_, err := stageListingWords(body)
			return err
		})

	sc.Step(`^the operator reads the design skill$`, func(ctx context.Context) error {
		held, err := skill.Load(designSkillDir)
		if err != nil {
			return fmt.Errorf("loading the skills this build ships: %w", err)
		}
		for at := range held {
			if held[at].Name == "design" {
				designOrderFrom(ctx).held = &held[at]
				return nil
			}
		}
		return fmt.Errorf("%s holds no design skill", designSkillDir)
	})

	sc.Step(`^the design skill says "([^"]*)"$`, func(ctx context.Context, said string) error {
		held := designOrderFrom(ctx).held
		if held == nil {
			return fmt.Errorf("the design skill was never read")
		}
		if !strings.Contains(held.Brief, said) {
			return fmt.Errorf("the brief never says %q, so a session designs a project without it", said)
		}
		return nil
	})
}

// readDesignCommand is the design command as the install wrote it onto the machine, which is the file
// the operator's agent opens.
func readDesignCommand(ctx context.Context) error {
	body, err := installedCommand(ctx, "design")
	if err != nil {
		return err
	}
	designOrderFrom(ctx).command = body
	return nil
}

// stageListingWords is the command the design command file reads the stage listing with, as words the
// tool takes. The word krewe is dropped, because the suite runs the binary itself.
//
// The first command the file writes out, and not a stage show anywhere in it. Where the design starts
// is the first thing this command needs, so a file that runs something else first decided something
// before it read the listing.
//
// The address has to be in the line. A listing read with no address answers for wherever the operator
// last stood, and the design command asks for the address in its first step.
func stageListingWords(body string) ([]string, error) {
	lines := runLine.FindAllStringSubmatch(body, -1)
	if len(lines) == 0 {
		return nil, fmt.Errorf("design.md writes out no command at all, so nothing reads where the project stands")
	}
	typed := strings.TrimSpace(lines[0][1])
	if !strings.HasPrefix(typed, "krewe stage show") {
		return nil, fmt.Errorf("the first command design.md runs is %q, and the stage listing is what says where the design starts",
			typed)
	}
	if !strings.Contains(typed, addressPlaceholder) {
		return nil, fmt.Errorf("design.md runs %q, and that reads whichever project the operator last stood in",
			typed)
	}
	return strings.Fields(typed)[1:], nil
}

// dispatchLines is every dispatch the file writes out to be typed.
func dispatchLines(body string) []string {
	var found []string
	for _, line := range runLine.FindAllStringSubmatch(body, -1) {
		typed := strings.TrimSpace(line[1])
		if strings.HasPrefix(typed, "krewe exec --dispatch") {
			found = append(found, typed)
		}
	}
	return found
}

// theStageOffered is the line under the listing, which is the one that says what to do next. The rows
// above it name all six whatever state the project is in, so a reading of the whole answer would
// report every stage as offered.
func theStageOffered(ctx context.Context) (string, error) {
	read := designOrderFrom(ctx).reading
	if strings.TrimSpace(read) == "" {
		return "", fmt.Errorf("the reading said nothing at all")
	}
	lines := strings.Split(strings.TrimRight(read, "\n"), "\n")
	for at := len(lines) - 1; at >= 0; at-- {
		if strings.TrimSpace(lines[at]) != "" {
			return strings.TrimSpace(lines[at]), nil
		}
	}
	return "", fmt.Errorf("the reading is blank lines: %q", read)
}

// stagesNamedIn is which of the six a line names.
func stagesNamedIn(line string) []string {
	var named []string
	for _, stage := range store.DesignStages() {
		if strings.Contains(line, stage) {
			named = append(named, stage)
		}
	}
	return named
}
