package features_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/atlantic-blue/quay-krewe/internal/skill"
	"github.com/cucumber/godog"
)

// Steps for the interview stage: the page a session writes from the operator's own answers, and the
// skill that says how it asks.
//
// The brief is read off the directory this build ships, the way the discover steps read theirs, so a
// brief that stopped saying how the questions are asked fails here rather than on somebody's first
// project.

// interviewSkillDir is where the skills this build ships live, as the suite sees them.
const interviewSkillDir = "../skills"

// interviewWorld is the interview skill as this build ships it.
type interviewWorld struct {
	held *skill.Skill
}

type interviewKey struct{}

func interviewFrom(ctx context.Context) *interviewWorld {
	i, _ := ctx.Value(interviewKey{}).(*interviewWorld)
	return i
}

func initializeInterviewSteps(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, interviewKey{}, &interviewWorld{}), nil
	})

	sc.Step(`^the operator reads the interview skill$`, func(ctx context.Context) error {
		held, err := skill.Load(interviewSkillDir)
		if err != nil {
			return fmt.Errorf("loading the skills this build ships: %w", err)
		}
		for at := range held {
			if held[at].Name == "interview" {
				interviewFrom(ctx).held = &held[at]
				return nil
			}
		}
		return fmt.Errorf("%s holds no interview skill", interviewSkillDir)
	})

	sc.Step(`^the interview skill says "([^"]*)"$`, func(ctx context.Context, said string) error {
		held := interviewFrom(ctx).held
		if held == nil {
			return fmt.Errorf("the interview skill was never read")
		}
		if !strings.Contains(held.Brief, said) {
			return fmt.Errorf("the brief never says %q, so a session interviews an operator without it", said)
		}
		return nil
	})

	// The stage sits where the order says, and the listing is read in that order, so the first entry
	// is the first stage.
	sc.Step(`^the "([^"]*)" design stage sits first$`, func(ctx context.Context, stage string) error {
		held := stagesFrom(ctx).stages
		if len(held) == 0 {
			return fmt.Errorf("the project holds no stages at all")
		}
		if first := held[0].GetStage(); first != stage {
			return fmt.Errorf("the listing starts at %q, and %s is the first of the seven", first, stage)
		}
		if position := held[0].GetPosition(); position != 0 {
			return fmt.Errorf("%s sits at position %d, want 0", stage, position)
		}
		return nil
	})
}
