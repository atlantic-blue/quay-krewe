package features_test

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	flowmap "github.com/atlantic-blue/quay-krewe/skills/flow-map"
	"github.com/cucumber/godog"
)

// The flow map scenarios touch the control plane nowhere. What they prove is a property of the
// page the skill ships: which frame a surface is drawn in, and where the colours come from. So
// they run the page's own render block over the skill's fixture and read the markup it answers
// with.

// aColour is a colour written into the markup or into a rule.
var aColour = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b|\brgba?\(|\bhsla?\(`)

// aFont is a font declaration, with whatever it was set to.
var aFont = regexp.MustCompile(`font-family\s*:\s*([^;}]+)`)

type flowMapWorld struct {
	page   flowmap.Page
	flows  flowmap.Flows
	drawn  string
	screen string
}

type flowMapKey struct{}

func flowMapFrom(ctx context.Context) *flowMapWorld {
	w, _ := ctx.Value(flowMapKey{}).(*flowMapWorld)
	return w
}

func initializeFlowMapSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the flow map and the fixture that holds both surfaces$`, func(ctx context.Context) (context.Context, error) {
		dir, err := flowmap.Dir()
		if err != nil {
			return ctx, err
		}
		page, err := flowmap.ReadPage(filepath.Join(dir, "index.html"))
		if err != nil {
			return ctx, err
		}
		flows, err := flowmap.ReadFlows(filepath.Join(dir, "fixtures", "two-surfaces.json"))
		if err != nil {
			return ctx, err
		}
		if len(flows.Screens) == 0 || len(flows.Stories) == 0 {
			return ctx, fmt.Errorf("the fixture holds %d screens and %d stories, so it proves nothing",
				len(flows.Screens), len(flows.Stories))
		}
		return context.WithValue(ctx, flowMapKey{}, &flowMapWorld{page: page, flows: flows}), nil
	})

	sc.Step(`^the operator plays the story "(.*)"$`, func(ctx context.Context, title string) error {
		w := flowMapFrom(ctx)
		story, found := w.flows.StoryCalled(title)
		if !found {
			return fmt.Errorf("no story called %q", title)
		}
		drawn, err := w.page.Screen(w.flows, story.Start)
		if err != nil {
			return err
		}
		w.screen = story.Start
		w.drawn = drawn
		return nil
	})

	sc.Step(`^the screen is drawn in a browser frame$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		for _, want := range []string{`data-surface="web"`, `class="frame frame-web"`, `class="addr"`} {
			if !strings.Contains(w.drawn, want) {
				return fmt.Errorf("%s carries no %q:\n%s", w.screen, want, w.drawn)
			}
		}
		if strings.Contains(w.drawn, "frame-mobile") {
			return fmt.Errorf("%s was drawn in a phone as well as in a browser", w.screen)
		}
		return nil
	})

	sc.Step(`^the address bar reads "(.*)"$`, func(ctx context.Context, route string) error {
		w := flowMapFrom(ctx)
		if !strings.Contains(w.drawn, `<span class="addr">`+route+`</span>`) {
			return fmt.Errorf("the address bar does not read %q:\n%s", route, w.drawn)
		}
		return nil
	})

	sc.Step(`^the screen is drawn in a phone frame$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		for _, want := range []string{`data-surface="mobile"`, `class="frame frame-mobile"`, `class="bezel"`} {
			if !strings.Contains(w.drawn, want) {
				return fmt.Errorf("%s carries no %q:\n%s", w.screen, want, w.drawn)
			}
		}
		if strings.Contains(w.drawn, "frame-web") {
			return fmt.Errorf("%s was drawn in a browser as well as on a phone", w.screen)
		}
		return nil
	})

	sc.Step(`^every colour it is drawn in is one the project's tokens name$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		named := map[string]bool{}
		for _, value := range w.flows.TokenValues() {
			named[value] = true
		}
		for _, found := range aColour.FindAllString(w.drawn, -1) {
			if !named[found] {
				return fmt.Errorf("%s was drawn with the colour %q, which the tokens do not name", w.screen, found)
			}
		}
		for name, value := range w.flows.Tokens["colour"] {
			if !strings.Contains(w.drawn, "--t-colour-"+name+":"+value) {
				return fmt.Errorf("%s does not carry the colour token %s", w.screen, name)
			}
		}
		return nil
	})

	sc.Step(`^the page's own rules are read$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		if !strings.Contains(w.page.ScreenStyles, ".screen") {
			return fmt.Errorf("the marked block holds no .screen rule, so it is not the part that paints a screen")
		}
		return nil
	})

	sc.Step(`^no rule that paints a screen carries a colour or a font of its own$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		if found := aColour.FindAllString(w.page.ScreenStyles, -1); len(found) > 0 {
			return fmt.Errorf("a rule that paints a screen holds the colours %v", found)
		}
		for _, found := range aFont.FindAllStringSubmatch(w.page.ScreenStyles, -1) {
			if !strings.HasPrefix(strings.TrimSpace(found[1]), "var(") {
				return fmt.Errorf("a rule that paints a screen sets font-family to %q", strings.TrimSpace(found[1]))
			}
		}
		return nil
	})

	sc.Step(`^no rule outside that block reaches a screen$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		for _, reserved := range []string{".screen", ".frame", ".bezel", ".chrome"} {
			if strings.Contains(w.page.OtherStyles, reserved+"{") || strings.Contains(w.page.OtherStyles, reserved+" ") {
				return fmt.Errorf("a rule outside the marked block names %s, so a screen can be painted where nothing checks the values", reserved)
			}
		}
		return nil
	})
}
