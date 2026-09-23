package features_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/atlantic-blue/quay-krewe/internal/store"
	render "github.com/atlantic-blue/quay-krewe/skills/flow-map/render"
	"github.com/cucumber/godog"
)

// The mockups scenarios are driven from the flow map's own fixture, rather than from a flows.json
// written here. The fixture is what the skill ships and what the page is proved against, so a
// check that accepts it is a check the skill's own example passes. Each scenario then breaks one
// thing in a copy of it.

// theScreen is the screen these scenarios break something on. The fixture holds it, and the
// refusal is read for its name.
const theScreen = "sign-in"

type mockupWorld struct {
	// fixture is the flow map's own flows.json, read once per scenario.
	fixture map[string]any
}

type mockupKey struct{}

func mockupsFrom(ctx context.Context) *mockupWorld {
	w, _ := ctx.Value(mockupKey{}).(*mockupWorld)
	return w
}

func initializeMockupCheckSteps(sc *godog.ScenarioContext) {
	// The state a mockup may be written in: the three stages before it agreed, and the design
	// system naming the very colours and fonts the fixture is drawn in.
	sc.Step(`^the stages up to the design system are approved, naming the project's colours and fonts$`,
		func(ctx context.Context) (context.Context, error) {
			fixture, err := theFlowMapFixture()
			if err != nil {
				return ctx, err
			}
			ctx = context.WithValue(ctx, mockupKey{}, &mockupWorld{fixture: fixture})

			tokens, err := asJSON(map[string]any{"tokens": fixture["tokens"]})
			if err != nil {
				return ctx, err
			}
			for _, stage := range []struct{ name, artifact string }{
				{store.StageDiscovery, ""},
				{store.StageStories, ""},
				{store.StageDesignSystem, tokens},
			} {
				if err := writeStage(ctx, stage.name, "the "+stage.name+" body", stage.artifact); err != nil {
					return ctx, err
				}
				if w := worldFrom(ctx); w.lastErr != nil {
					return ctx, fmt.Errorf("writing the %s stage was refused: %w", stage.name, w.lastErr)
				}
				if err := approveStage(ctx, stage.name); err != nil {
					return ctx, err
				}
				if w := worldFrom(ctx); w.lastErr != nil {
					return ctx, fmt.Errorf("approving the %s stage was refused: %w", stage.name, w.lastErr)
				}
			}
			return ctx, nil
		})

	sc.Step(`^the operator writes the mockups stage with a component on every shape$`,
		func(ctx context.Context) error {
			return writeMockupFixture(ctx, func(map[string]any) error { return nil })
		})

	sc.Step(`^the operator writes the mockups stage with a shape that names no component$`,
		func(ctx context.Context) error {
			return writeMockupFixture(ctx, func(flows map[string]any) error {
				shape, err := firstShapeOf(flows, theScreen)
				if err != nil {
					return err
				}
				if _, named := shape["component"]; !named {
					return fmt.Errorf("the fixture's first %s shape already names no component, "+
						"so this scenario would prove nothing", theScreen)
				}
				delete(shape, "component")
				return nil
			})
		})

	sc.Step(`^the operator writes the mockups stage drawn in the colour "([^"]*)"$`,
		func(ctx context.Context, colour string) error {
			return writeMockupFixture(ctx, func(flows map[string]any) error {
				return drawIn(flows, "colour", "primary", colour)
			})
		})

	sc.Step(`^the operator writes the mockups stage drawn in the font "([^"]*)"$`,
		func(ctx context.Context, font string) error {
			return writeMockupFixture(ctx, func(flows map[string]any) error {
				return drawIn(flows, "font", "sans", font)
			})
		})

	sc.Step(`^the operator writes the mockups stage with the artifact:$`,
		func(ctx context.Context, artifact *godog.DocString) error {
			return writeStage(ctx, store.StageMockups, "the screens", artifact.Content)
		})

	sc.Step(`^the mockups stage carries the artifact it was given$`, func(ctx context.Context) error {
		if w := worldFrom(ctx); w.lastErr != nil {
			return fmt.Errorf("the write was refused: %w", w.lastErr)
		}
		if err := readStages(ctx); err != nil {
			return err
		}
		held := stageNamed(stagesFrom(ctx).stages, store.StageMockups)
		if held == nil {
			return fmt.Errorf("the project holds no mockups stage: it holds %v",
				stageNames(stagesFrom(ctx).stages))
		}
		if held.GetArtifact() == "" {
			return fmt.Errorf("the mockups stage carries no artifact")
		}
		return nil
	})

	sc.Step(`^the project holds no mockups stage$`, func(ctx context.Context) error {
		if err := readStages(ctx); err != nil {
			return err
		}
		if held := stageNamed(stagesFrom(ctx).stages, store.StageMockups); held != nil {
			return fmt.Errorf("the refused write left a mockups stage behind, carrying %q", held.GetArtifact())
		}
		return nil
	})
}

// theFlowMapFixture reads the flows.json the skill ships, which is the same file the flow map
// scenarios draw their screens from.
func theFlowMapFixture() (map[string]any, error) {
	dir, err := render.Dir()
	if err != nil {
		return nil, err
	}
	body, err := os.ReadFile(filepath.Join(dir, "fixtures", "two-surfaces.json"))
	if err != nil {
		return nil, fmt.Errorf("reading the flow map fixture: %w", err)
	}
	var read map[string]any
	if err := json.Unmarshal(body, &read); err != nil {
		return nil, fmt.Errorf("the flow map fixture is not readable: %w", err)
	}
	return read, nil
}

// writeMockupFixture takes a fresh copy of the fixture, lets the scenario break one thing in it,
// and writes it as the mockups artifact. The copy matters: a scenario that edited the fixture in
// place would hand the next step a file somebody else had already broken.
func writeMockupFixture(ctx context.Context, breakIt func(map[string]any) error) error {
	held := mockupsFrom(ctx)
	if held == nil {
		return fmt.Errorf("no flow map fixture was read, so there is no mockup to write")
	}
	flows, err := copyOf(held.fixture)
	if err != nil {
		return err
	}
	if err := breakIt(flows); err != nil {
		return err
	}
	artifact, err := asJSON(flows)
	if err != nil {
		return err
	}
	return writeStage(ctx, store.StageMockups, "the screens", artifact)
}

// firstShapeOf is the first shape of one screen, so a scenario can take something off it.
func firstShapeOf(flows map[string]any, screen string) (map[string]any, error) {
	screens, _ := flows["screens"].(map[string]any)
	held, _ := screens[screen].(map[string]any)
	elements, _ := held["el"].([]any)
	if len(elements) == 0 {
		return nil, fmt.Errorf("the fixture's %s screen holds no shapes", screen)
	}
	shape, ok := elements[0].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("the first shape of %s is not a shape", screen)
	}
	return shape, nil
}

// drawIn puts one value into one token, which is how a mockup comes to be drawn in something the
// design system never named.
func drawIn(flows map[string]any, group, token, value string) error {
	tokens, _ := flows["tokens"].(map[string]any)
	held, _ := tokens[group].(map[string]any)
	if held == nil {
		return fmt.Errorf("the fixture names no %s tokens", group)
	}
	if _, there := held[token]; !there {
		return fmt.Errorf("the fixture names no %s token called %s", group, token)
	}
	held[token] = value
	return nil
}

func copyOf(held map[string]any) (map[string]any, error) {
	body, err := json.Marshal(held)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func asJSON(held any) (string, error) {
	body, err := json.Marshal(held)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
