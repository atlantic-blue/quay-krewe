package features_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/store"
	"github.com/cucumber/godog"
)

// stageWorld is the stages last read, and what the last write said about itself.
type stageWorld struct {
	stages   []*quaycrewv1.DesignStage
	warnings []string
}

type stageKey struct{}

func stagesFrom(ctx context.Context) *stageWorld {
	s, _ := ctx.Value(stageKey{}).(*stageWorld)
	return s
}

func initializeStageSteps(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, stageKey{}, &stageWorld{}), nil
	})

	sc.Step(`^the operator reads the project's design stages$`, func(ctx context.Context) error {
		return readStages(ctx)
	})

	sc.Step(`^the operator writes the "([^"]*)" design stage as "([^"]*)"$`,
		func(ctx context.Context, stage, body string) error {
			return writeStage(ctx, stage, unescape(body), "")
		})

	// A body that runs to more than one line arrives as a docstring, because a diagram is a fenced
	// block and a fenced block is three lines at the shortest.
	sc.Step(`^the operator writes the "([^"]*)" design stage as:$`,
		func(ctx context.Context, stage string, body *godog.DocString) error {
			return writeStage(ctx, stage, body.Content, "")
		})

	// The artifact arrives as a docstring rather than inside quotes, because it is json and json is
	// mostly quotes.
	sc.Step(`^the operator writes the "([^"]*)" design stage with the artifact:$`,
		func(ctx context.Context, stage string, artifact *godog.DocString) error {
			return writeStage(ctx, stage, settledBody(stage), artifact.Content)
		})

	sc.Step(`^the operator approves the "([^"]*)" design stage$`, func(ctx context.Context, stage string) error {
		return approveStage(ctx, stage)
	})

	// The setup a scenario about a later stage needs: the stage is written and then agreed, which is
	// the only way past the rule that orders the six.
	sc.Step(`^the "([^"]*)" design stage is written and approved$`, func(ctx context.Context, stage string) error {
		return settleStage(ctx, stage)
	})

	sc.Step(`^every design stage is written and approved$`, func(ctx context.Context) error {
		for _, stage := range store.DesignStages() {
			if err := settleStage(ctx, stage); err != nil {
				return err
			}
		}
		return nil
	})

	sc.Step(`^every design stage is approved$`, func(ctx context.Context) error {
		for _, stage := range store.DesignStages() {
			held, err := stageRead(ctx, stage)
			if err != nil {
				return err
			}
			if !held.GetApproved() {
				return fmt.Errorf("the %s stage came back without the word it was given", stage)
			}
		}
		return nil
	})

	sc.Step(`^the project holds no design stages$`, func(ctx context.Context) error {
		if err := readStages(ctx); err != nil {
			return err
		}
		if held := stagesFrom(ctx).stages; len(held) != 0 {
			return fmt.Errorf("the project holds %v, and nothing was supposed to be written", stageNames(held))
		}
		return nil
	})

	// The picture itself, rather than the whole body word for word. What the rule asks for is a
	// diagram in the stage, and a scenario that repeated the body would be reading the setup back.
	sc.Step(`^the "([^"]*)" design stage carries a diagram$`, func(ctx context.Context, stage string) error {
		held, err := stageRead(ctx, stage)
		if err != nil {
			return err
		}
		if !strings.Contains(held.GetBody(), "```mermaid") {
			return fmt.Errorf("the %s stage reads %q, and it holds no mermaid block", stage, held.GetBody())
		}
		return nil
	})

	// The other half of a refusal: the write left nothing behind. A refusal that stored the text
	// anyway would leave the operator a stage they cannot see and cannot approve.
	sc.Step(`^the project holds no "([^"]*)" design stage$`, func(ctx context.Context, stage string) error {
		if err := readStages(ctx); err != nil {
			return err
		}
		if held := stageNamed(stagesFrom(ctx).stages, stage); held != nil {
			return fmt.Errorf("the project holds a %s stage reading %q, and the write was refused",
				stage, held.GetBody())
		}
		return nil
	})

	sc.Step(`^the project holds (\d+) design stages, in order$`, func(ctx context.Context, want int) error {
		held := stagesFrom(ctx).stages
		if len(held) != want {
			return fmt.Errorf("the project holds %d stages, want %d: %v", len(held), want, stageNames(held))
		}
		if got := strings.Join(stageNames(held), ","); got != strings.Join(store.DesignStages(), ",") {
			return fmt.Errorf("the stages read %s, want the six in their own order", got)
		}
		for at, stage := range held {
			if stage.GetPosition() != int32(at) {
				return fmt.Errorf("%s sits at position %d, want %d", stage.GetStage(), stage.GetPosition(), at)
			}
		}
		return nil
	})

	sc.Step(`^the "([^"]*)" design stage reads "([^"]*)"$`, func(ctx context.Context, stage, want string) error {
		held, err := stageRead(ctx, stage)
		if err != nil {
			return err
		}
		if held.GetBody() != unescape(want) {
			return fmt.Errorf("the %s stage reads %q, want %q", stage, held.GetBody(), unescape(want))
		}
		return nil
	})

	sc.Step(`^the "([^"]*)" design stage is approved$`, func(ctx context.Context, stage string) error {
		held, err := stageRead(ctx, stage)
		if err != nil {
			return err
		}
		if !held.GetApproved() {
			return fmt.Errorf("the %s stage reads approved=false at version %d, approved at version %d",
				stage, held.GetVersion(), held.GetApprovedVersion())
		}
		return nil
	})

	sc.Step(`^the "([^"]*)" design stage is not approved$`, func(ctx context.Context, stage string) error {
		held, err := stageRead(ctx, stage)
		if err != nil {
			return err
		}
		if held.GetApproved() {
			return fmt.Errorf("the %s stage still carries the word, at version %d", stage, held.GetVersion())
		}
		return nil
	})

	// A stage the operator agreed to and then wrote again keeps the version the word was given to, so
	// a reader can tell it from a stage nobody ever agreed to.
	sc.Step(`^the "([^"]*)" design stage reads approved at version (\d+), holding version (\d+)$`,
		func(ctx context.Context, stage string, approved, version int) error {
			held, err := stageRead(ctx, stage)
			if err != nil {
				return err
			}
			if int(held.GetApprovedVersion()) != approved || int(held.GetVersion()) != version {
				return fmt.Errorf("the %s stage reads approved at version %d holding version %d, want %d and %d",
					stage, held.GetApprovedVersion(), held.GetVersion(), approved, version)
			}
			return nil
		})

	// What it means, never how it is spelled. Postgres holds a jsonb document in its own form, so a
	// scenario comparing the bytes would pass here, against the memory store, and fail against a
	// database.
	sc.Step(`^the "([^"]*)" design stage carries an artifact meaning:$`,
		func(ctx context.Context, stage string, want *godog.DocString) error {
			held, err := stageRead(ctx, stage)
			if err != nil {
				return err
			}
			read, wanted := new(bytes.Buffer), new(bytes.Buffer)
			if err := json.Compact(read, []byte(held.GetArtifact())); err != nil {
				return fmt.Errorf("the %s stage carries %q, which is not json: %w", stage, held.GetArtifact(), err)
			}
			if err := json.Compact(wanted, []byte(want.Content)); err != nil {
				return fmt.Errorf("the scenario asks for %q, which is not json: %w", want.Content, err)
			}
			if read.String() != wanted.String() {
				return fmt.Errorf("the %s stage carries %s, want %s", stage, read, wanted)
			}
			return nil
		})

	sc.Step(`^the write says the approval went from "([^"]*)"$`, func(ctx context.Context, stage string) error {
		said := strings.Join(stagesFrom(ctx).warnings, "\n")
		if !strings.Contains(said, stage) {
			return fmt.Errorf("the write said %q, and %s lost its approval without being named", said, stage)
		}
		return nil
	})
}

func readStages(ctx context.Context) error {
	w, s := worldFrom(ctx), stagesFrom(ctx)
	resp, err := w.client.ListDesignStages(ctx, &quaycrewv1.ListDesignStagesRequest{Project: w.projectID})
	w.lastErr = err
	if err != nil {
		return nil
	}
	s.stages = resp.GetStages()
	return nil
}

func writeStage(ctx context.Context, stage, body, artifact string) error {
	w, s := worldFrom(ctx), stagesFrom(ctx)
	resp, err := w.client.SetDesignStage(ctx, &quaycrewv1.SetDesignStageRequest{
		Project: w.projectID, Stage: stage, Body: body, Artifact: artifact,
	})
	w.lastErr = err
	if err != nil {
		s.warnings = nil
		return nil
	}
	s.warnings = resp.GetWarnings()
	return nil
}

func approveStage(ctx context.Context, stage string) error {
	w := worldFrom(ctx)
	_, err := w.client.ApproveDesignStage(ctx, &quaycrewv1.ApproveDesignStageRequest{
		Project: w.projectID, Stage: stage,
	})
	w.lastErr = err
	return nil
}

// settleStage is the setup step: a stage is written and then agreed, and a refusal of either is the
// setup failing rather than the scenario's own refusal to read later.
func settleStage(ctx context.Context, stage string) error {
	if err := writeStage(ctx, stage, settledBody(stage), ""); err != nil {
		return err
	}
	if w := worldFrom(ctx); w.lastErr != nil {
		return fmt.Errorf("writing the %s stage was refused: %w", stage, w.lastErr)
	}
	if err := approveStage(ctx, stage); err != nil {
		return err
	}
	if w := worldFrom(ctx); w.lastErr != nil {
		return fmt.Errorf("approving the %s stage was refused: %w", stage, w.lastErr)
	}
	return nil
}

// settledBody is the body the setup writes for one stage.
//
// The data model and the architecture each describe a structure, and a structure is written as a
// picture, so a write of either carries a diagram or it is refused. A setup without one would be
// refused, and every scenario standing on it would read as a failure of its own.
func settledBody(stage string) string {
	body := "the " + stage + " body"
	if stage == store.StageDataModel || stage == store.StageArchitecture {
		return body + "\n\n```mermaid\nflowchart TD\n  one --> two\n```\n"
	}
	return body
}

// stageRead is one stage out of the last listing, and it reads the listing when no step has yet.
func stageRead(ctx context.Context, stage string) (*quaycrewv1.DesignStage, error) {
	if len(stagesFrom(ctx).stages) == 0 {
		if err := readStages(ctx); err != nil {
			return nil, err
		}
	}
	held := stageNamed(stagesFrom(ctx).stages, stage)
	if held == nil {
		return nil, fmt.Errorf("the project holds no %s stage: it holds %v",
			stage, stageNames(stagesFrom(ctx).stages))
	}
	return held, nil
}

func stageNamed(stages []*quaycrewv1.DesignStage, stage string) *quaycrewv1.DesignStage {
	for _, held := range stages {
		if held.GetStage() == stage {
			return held
		}
	}
	return nil
}

func stageNames(stages []*quaycrewv1.DesignStage) []string {
	out := make([]string, 0, len(stages))
	for _, stage := range stages {
		out = append(out, stage.GetStage())
	}
	return out
}
