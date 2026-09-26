package features_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	// system is the design system the scenario approved, which is where the values every screen is
	// drawn in live.
	system map[string]any
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
			system, err := theDesignSystemFixture()
			if err != nil {
				return ctx, err
			}
			ctx = context.WithValue(ctx, mockupKey{}, &mockupWorld{fixture: fixture, system: system})

			tokens, err := asJSON(system)
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

	sc.Step(`^the operator writes the mockups stage with a component on every part$`,
		func(ctx context.Context) error {
			return writeMockupFixture(ctx, func(map[string]any) error { return nil })
		})

	// The form that went, written by a session that still has it in its fingers. What it is told
	// back is the whole of this scenario: the screen, and the field to write instead.
	sc.Step(`^the operator writes the mockups stage with a screen written as a list of shapes$`,
		func(ctx context.Context) error {
			return writeMockupFixture(ctx, func(flows map[string]any) error {
				return writeAsAShapeList(flows, theScreen)
			})
		})

	// A screen written as markup carries its component names on the markup, so this takes one off a
	// part that holds the words a person reads.
	sc.Step(`^the operator writes the mockups stage with words outside every named component$`,
		func(ctx context.Context) error {
			return writeMockupFixture(ctx, func(flows map[string]any) error {
				return unnameTheComponentOf(flows, theMarkupScreen,
					`<h1 data-component="Heading">`, `<h1>`)
			})
		})

	// The shape a session writes when nothing tells it where the fonts of the project are: a font
	// host in the markup of one screen.
	sc.Step(`^the operator writes the mockups stage with a screen loading a font from the network$`,
		func(ctx context.Context) error {
			return writeMockupFixture(ctx, func(flows map[string]any) error {
				return loadAFontFromTheNetwork(flows, theMarkupScreen)
			})
		})

	// The screen is written as markup, so a colour arrives inside the stylesheet of that screen
	// rather than in a token block. It is the place a session paints from.
	sc.Step(`^the operator writes the mockups stage with the colour "([^"]*)" in the markup of a screen$`,
		func(ctx context.Context, colour string) error {
			return writeMockupFixture(ctx, func(flows map[string]any) error {
				return paintTheMarkupOf(flows, theMarkupScreen, colour)
			})
		})

	// A design system written as prose names nothing, and nothing is what the screens are then held
	// to. Writing the stage again takes the word off it, so it is approved again here.
	sc.Step(`^the design system is approved naming nothing$`, func(ctx context.Context) error {
		if err := writeStage(ctx, store.StageDesignSystem, "one accent colour, and plenty of space", ""); err != nil {
			return err
		}
		if w := worldFrom(ctx); w.lastErr != nil {
			return fmt.Errorf("writing the design system as prose was refused: %w", w.lastErr)
		}
		if err := approveStage(ctx, store.StageDesignSystem); err != nil {
			return err
		}
		if w := worldFrom(ctx); w.lastErr != nil {
			return fmt.Errorf("approving the design system was refused: %w", w.lastErr)
		}
		return nil
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

// writeAsAShapeList turns one screen back into the form that went. Every shape in it names its
// component, so what the write is refused for is the form itself and nothing else.
func writeAsAShapeList(flows map[string]any, screen string) error {
	screens, _ := flows["screens"].(map[string]any)
	held, _ := screens[screen].(map[string]any)
	if held == nil {
		return fmt.Errorf("the fixture holds no %s screen, so this scenario would prove nothing", screen)
	}
	if _, written := held["html"]; !written {
		return fmt.Errorf("the fixture's %s screen is not written as markup, "+
			"so turning it back into shapes would prove nothing", screen)
	}
	delete(held, "html")
	held["el"] = []any{
		map[string]any{"t": "h", "component": "Heading", "v": "Sign in to Tide"},
		map[string]any{"t": "btn", "component": "Button", "v": "Sign in", "to": "dashboard"},
	}
	return nil
}

// drawIn puts one value into one token, which is how a mockup comes to be drawn in something the
// design system never named.
func drawIn(flows map[string]any, group, token, value string) error {
	tokens, _ := flows["tokens"].(map[string]any)
	if tokens == nil {
		tokens = map[string]any{}
		flows["tokens"] = tokens
	}
	held, _ := tokens[group].(map[string]any)
	if held == nil {
		held = map[string]any{}
		tokens[group] = held
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

// theMarkupScreen is the screen of the fixture that is written as markup, where a component name
// sits on an element rather than on a shape.
const theMarkupScreen = "dashboard"

// unnameTheComponentOf takes the component name off one part of a screen written as markup.
//
// It refuses a screen it did not change, because a scenario that broke nothing would pass against
// the rule it was written to prove.
func unnameTheComponentOf(flows map[string]any, screen, part, without string) error {
	screens, _ := flows["screens"].(map[string]any)
	held, _ := screens[screen].(map[string]any)
	markup, _ := held["html"].(string)
	if !strings.Contains(markup, part) {
		return fmt.Errorf("the %s screen of the fixture holds no %s, so this scenario would prove nothing",
			screen, part)
	}
	held["html"] = strings.Replace(markup, part, without, 1)
	return nil
}

// theFontOnTheNetwork is how a session reaches for a typeface when it has not been told where the
// fonts of the project live. It is the form measured on the screens of a real project.
const theFontOnTheNetwork = `<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Inter">`

// loadAFontFromTheNetwork puts a font host into the markup of one screen.
//
// It refuses a screen it could not change, because a scenario that broke nothing would pass against
// the rule it was written to prove.
func loadAFontFromTheNetwork(flows map[string]any, screen string) error {
	screens, _ := flows["screens"].(map[string]any)
	held, _ := screens[screen].(map[string]any)
	markup, _ := held["html"].(string)
	if strings.TrimSpace(markup) == "" {
		return fmt.Errorf("the %s screen of the fixture is not written as markup, "+
			"so this scenario would prove nothing", screen)
	}
	if strings.Contains(markup, "fonts.googleapis.com") {
		return fmt.Errorf("the %s screen of the fixture already loads a font from the network, "+
			"so this scenario would prove nothing", screen)
	}
	held["html"] = theFontOnTheNetwork + markup
	return nil
}

// theStylesheetEnd is where the stylesheet of a screen closes. A declaration written in front of it
// is painted over every part of that screen.
const theStylesheetEnd = "</style>"

// paintTheMarkupOf writes one colour into the stylesheet of one screen.
//
// It refuses a screen it could not paint, because a scenario that broke nothing would pass against
// the rule it was written to prove.
func paintTheMarkupOf(flows map[string]any, screen, colour string) error {
	screens, _ := flows["screens"].(map[string]any)
	held, _ := screens[screen].(map[string]any)
	markup, _ := held["html"].(string)
	if !strings.Contains(markup, theStylesheetEnd) {
		return fmt.Errorf("the %s screen of the fixture carries no stylesheet, "+
			"so this scenario would prove nothing", screen)
	}
	if strings.Contains(markup, colour) {
		return fmt.Errorf("the %s screen of the fixture is already painted in %s, "+
			"so this scenario would prove nothing", screen, colour)
	}
	held["html"] = strings.Replace(markup,
		theStylesheetEnd, fmt.Sprintf("h1{color:%s}%s", colour, theStylesheetEnd), 1)
	return nil
}
