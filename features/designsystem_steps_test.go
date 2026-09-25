package features_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/atlantic-blue/quay-krewe/internal/store"
	render "github.com/atlantic-blue/quay-krewe/skills/flow-map/render"
	"github.com/cucumber/godog"
)

// The design system scenarios are driven from the file the skill ships, the way the mockups
// scenarios are driven from its flows.json. A check that keeps the skill's own example is a check a
// session writing against the skill can satisfy, and each refusal then breaks one thing in a copy
// of it.

// theFontAsset is the asset the size scenario grows past the cap, and the refusal is read for its
// name.
const theFontAsset = "inter-regular"

type designSystemWorld struct {
	// fixture is the design system the skill ships, read once per scenario.
	fixture map[string]any
	// written is the artifact the scenario handed to the write, so a read back is held to it.
	written map[string]any
}

type designSystemKey struct{}

func designSystemFrom(ctx context.Context) *designSystemWorld {
	w, _ := ctx.Value(designSystemKey{}).(*designSystemWorld)
	return w
}

func initializeDesignSystemSteps(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, designSystemKey{}, &designSystemWorld{}), nil
	})

	// The state a design system may be written in: the two stages before it carry the operator's
	// word, which is the only way past the rule that orders the six.
	sc.Step(`^the stages before the design system are approved$`, func(ctx context.Context) error {
		for _, stage := range []string{store.StageDiscovery, store.StageStories} {
			if err := settleStage(ctx, stage); err != nil {
				return err
			}
		}
		return nil
	})

	sc.Step(`^the operator writes the design system the flow map skill ships$`, func(ctx context.Context) error {
		return writeDesignSystem(ctx, func(map[string]any) error { return nil })
	})

	sc.Step(`^the operator writes the design system with a font file of (\d+) kibibytes$`,
		func(ctx context.Context, size int) error {
			return writeDesignSystem(ctx, func(system map[string]any) error {
				asset, err := assetOf(system, theFontAsset)
				if err != nil {
					return err
				}
				asset["data"] = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("f"), size*1024))
				return nil
			})
		})

	sc.Step(`^the operator writes the design system with the stylesheet colour "([^"]*)"$`,
		func(ctx context.Context, colour string) error {
			return writeDesignSystem(ctx, func(system map[string]any) error {
				sheet, _ := system["css"].(string)
				if sheet == "" {
					return fmt.Errorf("the design system the skill ships carries no stylesheet to paint in")
				}
				system["css"] = sheet + "\n.notice{color:" + colour + "}\n"
				return nil
			})
		})

	sc.Step(`^the design system stage carries the tokens, the font file and the stylesheet it was given$`,
		func(ctx context.Context) error {
			held, err := designSystemHeld(ctx)
			if err != nil {
				return err
			}
			for _, group := range []string{"colour", "font"} {
				if len(namesUnder(asMap(asMap(held["tokens"])[group]))) == 0 {
					return fmt.Errorf("the stage names no %s token, so no screen knows what to draw in", group)
				}
			}
			if _, err := assetOf(held, theFontAsset); err != nil {
				return fmt.Errorf("the font file did not travel with the stage: %w", err)
			}
			if sheet, _ := held["css"].(string); strings.TrimSpace(sheet) == "" {
				return fmt.Errorf("the stage carries no stylesheet, so every screen is given nothing")
			}
			return sameDocument(held, designSystemFrom(ctx).written)
		})

	// The font file is only reachable through a family a screen can write, so the two halves of the
	// stage have to agree with each other.
	sc.Step(`^the font file the design system carries names a family one of its font tokens names$`,
		func(ctx context.Context) error {
			held, err := designSystemHeld(ctx)
			if err != nil {
				return err
			}
			asset, err := assetOf(held, theFontAsset)
			if err != nil {
				return err
			}
			family, _ := asset["family"].(string)
			if family == "" {
				return fmt.Errorf("the %s asset names no family", theFontAsset)
			}
			fonts := asMap(asMap(held["tokens"])["font"])
			for _, name := range namesUnder(fonts) {
				stack, _ := fonts[name].(string)
				for _, one := range strings.Split(stack, ",") {
					if strings.EqualFold(strings.TrimSpace(one), strings.TrimSpace(family)) {
						return nil
					}
				}
			}
			return fmt.Errorf("the %s asset is the family %q, and the font tokens read %v",
				theFontAsset, family, fonts)
		})

	sc.Step(`^the write says nothing is wrong$`, func(ctx context.Context) error {
		if w := worldFrom(ctx); w.lastErr != nil {
			return fmt.Errorf("the write was refused: %w", w.lastErr)
		}
		if said := stagesFrom(ctx).warnings; len(said) != 0 {
			return fmt.Errorf("the write warned %q, and the design system it was given is whole",
				strings.Join(said, "; "))
		}
		return nil
	})
}

// writeDesignSystem takes a fresh copy of the fixture, lets the scenario break one thing in it, and
// writes it as the design system artifact. The copy matters: a scenario that edited the fixture in
// place would hand the next one a file somebody else had already broken.
func writeDesignSystem(ctx context.Context, breakIt func(map[string]any) error) error {
	system, err := theDesignSystemFixture()
	if err != nil {
		return err
	}
	if err := breakIt(system); err != nil {
		return err
	}
	artifact, err := asJSON(system)
	if err != nil {
		return err
	}
	held := designSystemFrom(ctx)
	held.fixture, held.written = system, system
	return writeStage(ctx, store.StageDesignSystem, "the design system", artifact)
}

// theDesignSystemFixture reads the design system the skill ships, which is the file a session is
// told to write its own against.
func theDesignSystemFixture() (map[string]any, error) {
	dir, err := render.Dir()
	if err != nil {
		return nil, err
	}
	at := filepath.Join(dir, "fixtures", "design-system.json")
	body, err := os.ReadFile(at) //nolint:gosec // the path is the skill's own fixture
	if err != nil {
		return nil, fmt.Errorf("reading the design system the skill ships: %w", err)
	}
	var read map[string]any
	if err := json.Unmarshal(body, &read); err != nil {
		return nil, fmt.Errorf("%s is not readable: %w", at, err)
	}
	return read, nil
}

// designSystemHeld is the artifact the project holds, read back out of the store.
func designSystemHeld(ctx context.Context) (map[string]any, error) {
	if w := worldFrom(ctx); w.lastErr != nil {
		return nil, fmt.Errorf("the write was refused: %w", w.lastErr)
	}
	stage, err := stageRead(ctx, store.StageDesignSystem)
	if err != nil {
		return nil, err
	}
	var read map[string]any
	if err := json.Unmarshal([]byte(stage.GetArtifact()), &read); err != nil {
		return nil, fmt.Errorf("the design system stage carries %q, which is not readable: %w",
			stage.GetArtifact(), err)
	}
	return read, nil
}

// assetOf is one asset of a design system, so a scenario can grow it or read it.
func assetOf(system map[string]any, name string) (map[string]any, error) {
	asset := asMap(asMap(system["assets"])[name])
	if asset == nil {
		return nil, fmt.Errorf("the design system carries no asset named %s: it carries %v",
			name, namesUnder(asMap(system["assets"])))
	}
	return asset, nil
}

// sameDocument holds what came back to what went in, by what it means rather than how it is
// spelled. A store that keeps json in a form of its own would fail a comparison of the bytes.
func sameDocument(read, want map[string]any) error {
	got, err := asJSON(read)
	if err != nil {
		return err
	}
	wanted, err := asJSON(want)
	if err != nil {
		return err
	}
	if got != wanted {
		return fmt.Errorf("the stage carries a design system of %d characters, and it was given one of %d",
			len(got), len(wanted))
	}
	return nil
}

func asMap(value any) map[string]any {
	held, _ := value.(map[string]any)
	return held
}

// namesUnder reads a group in one order, so a message about a document names the same thing twice.
func namesUnder(held map[string]any) []string {
	out := make([]string, 0, len(held))
	for name := range held {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
