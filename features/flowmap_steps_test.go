package features_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	flowmap "github.com/atlantic-blue/quay-krewe/skills/flow-map/render"
	"github.com/cucumber/godog"
)

// The flow map scenarios touch the control plane nowhere. What they prove is a property of the
// page the skill ships: which frame a surface is drawn in, what an operator reads inside it, and
// where the colours come from. So they run the page's own render block over the skill's fixture
// and read the markup it answers with.

// aColour is a colour written into the markup or into a rule.
var aColour = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b|\brgba?\(|\bhsla?\(`)

// aFont is a font declaration, with whatever it was set to.
var aFont = regexp.MustCompile(`font-family\s*:\s*([^;}]+)`)

// theWords are what a person reads on that screen, which is what has to reach the operator.
var theWords = []string{"Today", "Two tides, both after dark.", "Log a tide", "Tap here"}

// theScript and theEventAttribute are what a session may have written before any gate read the
// artifact. The page takes both out of the markup it was given.
const (
	theScreenMarkup   = `<main data-component="Card"><h1 data-component="Heading">Today</h1><p data-component="Text">Two tides, both after dark.</p><button data-component="Button" data-to="log">Log a tide</button></main>`
	theScript         = `<script>parent.document.title='taken'</script>`
	theEventAttribute = `<p data-component="Text" onclick="steal()">Tap here</p>`
)

type flowMapWorld struct {
	page     flowmap.Page
	flows    flowmap.Flows
	system   flowmap.System
	drawn    string
	document string
	screen   string
}

type flowMapKey struct{}

func flowMapFrom(ctx context.Context) *flowMapWorld {
	w, _ := ctx.Value(flowMapKey{}).(*flowMapWorld)
	return w
}

func initializeFlowMapSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the flow map, the fixture that holds both surfaces, and the design system of the project$`,
		func(ctx context.Context) (context.Context, error) {
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
			system, err := flowmap.ReadSystem(filepath.Join(dir, "fixtures", "design-system.json"))
			if err != nil {
				return ctx, err
			}
			if len(flows.Screens) == 0 || len(flows.Stories) == 0 {
				return ctx, fmt.Errorf("the fixture holds %d screens and %d stories, so it proves nothing",
					len(flows.Screens), len(flows.Stories))
			}
			if len(system.TokenValues()) == 0 {
				return ctx, fmt.Errorf("the design system names no value, so nothing holds a screen to it")
			}
			return context.WithValue(ctx, flowMapKey{}, &flowMapWorld{page: page, flows: flows, system: system}), nil
		})

	sc.Step(`^the operator plays the story "(.*)"$`, func(ctx context.Context, title string) error {
		w := flowMapFrom(ctx)
		story, found := w.flows.StoryCalled(title)
		if !found {
			return fmt.Errorf("no story called %q", title)
		}
		return w.open(story.Start)
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

	// The markup is written here rather than read from the fixture, because what this scenario is
	// about is a screen a session wrote, and a session writes whatever it likes. It gets a script
	// and an event attribute, because an artifact stored before this feature was never read by a
	// gate and the page has to hold it anyway.
	sc.Step(`^a screen a session wrote as markup, holding a script and an event attribute$`,
		func(ctx context.Context) error {
			return flowMapFrom(ctx).write(theScreenMarkup + theEventAttribute + theScript)
		})

	sc.Step(`^the operator opens that screen$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		return w.open(w.screen)
	})

	sc.Step(`^the operator opens a screen a session wrote as markup$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		if err := w.write(theScreenMarkup); err != nil {
			return err
		}
		return w.open(w.screen)
	})

	sc.Step(`^the operator reads the words the session wrote$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		wrote := w.flows.Screens[w.screen].HTML
		read := 0
		for _, word := range theWords {
			if !strings.Contains(wrote, word) {
				continue
			}
			read++
			if !strings.Contains(w.document, word) {
				return fmt.Errorf("the screen does not read %q, which the session wrote:\n%s", word, w.document)
			}
		}
		if read == 0 {
			return fmt.Errorf("the screen holds none of the words, so this step proves nothing")
		}
		return nil
	})

	sc.Step(`^the screen is drawn at the size of the surface it was written at$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		size := map[string][2]int{"mobile": {390, 844}, "web": {1280, 800}}[w.flows.Screens[w.screen].Surface]
		for _, want := range []string{
			fmt.Sprintf(`width="%d"`, size[0]),
			fmt.Sprintf(`height="%d"`, size[1]),
		} {
			if !strings.Contains(w.drawn, want) {
				return fmt.Errorf("the frame of %s carries no %s:\n%s", w.screen, want, w.drawn)
			}
		}
		if !strings.Contains(w.document, fmt.Sprintf("width:%dpx", size[0])) {
			return fmt.Errorf("the screen is not sized to the surface it was written at:\n%s", w.document)
		}
		return nil
	})

	sc.Step(`^nothing the session wrote can run$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		for _, refuse := range []string{"<script", "onclick", "steal(", "document.title"} {
			if strings.Contains(w.document, refuse) {
				return fmt.Errorf("the screen still holds %q, which the session wrote:\n%s", refuse, w.document)
			}
		}
		if !strings.Contains(w.drawn, `sandbox="allow-scripts"`) {
			return fmt.Errorf("the frame is not sandboxed, so what it holds can reach the page:\n%s", w.drawn)
		}
		return nil
	})

	sc.Step(`^the screen can reach no address, and no other screen$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		for _, want := range []string{
			"default-src 'none'", "img-src data:", "font-src data:",
			"style-src 'unsafe-inline'", "script-src 'unsafe-inline'",
			"form-action 'none'", "base-uri 'none'",
		} {
			if !strings.Contains(w.document, want) {
				return fmt.Errorf("the screen does not refuse an address with %q:\n%s", want, w.document)
			}
		}
		if !strings.Contains(w.drawn, "<iframe") {
			return fmt.Errorf("the screen is not in a frame of its own, so a rule in it reaches another screen:\n%s", w.drawn)
		}
		return nil
	})

	sc.Step(`^every colour it is drawn in is one the design system names$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		named := map[string]bool{}
		for _, value := range w.system.TokenValues() {
			named[value] = true
		}
		read := w.drawn + w.document
		for _, found := range aColour.FindAllString(read, -1) {
			if !named[found] {
				return fmt.Errorf("%s was drawn with the colour %q, which the design system does not name", w.screen, found)
			}
		}
		for name, value := range w.system.Tokens["colour"] {
			if !strings.Contains(read, "--t-colour-"+name+":"+value) {
				return fmt.Errorf("%s does not carry the colour %s of the design system", w.screen, name)
			}
		}
		return nil
	})

	sc.Step(`^the page put no colour and no font of its own into it$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		if !strings.Contains(w.page.ScreenStyles, ".screen") {
			return fmt.Errorf("the marked block holds no .screen rule, so it is not the part that paints a screen")
		}
		if found := aColour.FindAllString(w.page.ScreenStyles, -1); len(found) > 0 {
			return fmt.Errorf("a rule that paints a screen holds the colours %v", found)
		}
		for _, found := range aFont.FindAllStringSubmatch(w.page.ScreenStyles, -1) {
			if !strings.HasPrefix(strings.TrimSpace(found[1]), "var(") {
				return fmt.Errorf("a rule that paints a screen sets font-family to %q", strings.TrimSpace(found[1]))
			}
		}
		for _, reserved := range []string{".screen", ".frame", ".bezel", ".chrome"} {
			if strings.Contains(w.page.OtherStyles, reserved+"{") || strings.Contains(w.page.OtherStyles, reserved+" ") {
				return fmt.Errorf("a rule outside the marked block names %s, so a screen can be painted where nothing checks the values", reserved)
			}
		}
		return nil
	})
}

// open draws one screen and keeps both what the page answered and the document the frame was
// given, because the operator reads the second one through the first.
func (w *flowMapWorld) open(id string) error {
	drawn, err := w.page.Screen(w.flows, w.system, id)
	if err != nil {
		return err
	}
	w.screen, w.drawn = id, drawn
	w.document, _ = flowmap.DocumentIn(drawn)
	return nil
}

// write puts markup a session wrote onto one screen of the fixture, leaving the file on disk
// alone. The screen is the one the mobile story opens on, so what is proved is the screen an
// operator actually reaches.
func (w *flowMapWorld) write(markup string) error {
	story, found := w.flows.StoryCalled("A person logs a tide on the phone")
	if !found {
		return fmt.Errorf("the fixture holds no mobile story, so there is no screen to write")
	}
	var loose map[string]any
	if err := json.Unmarshal(w.flows.Raw, &loose); err != nil {
		return fmt.Errorf("reading the fixture again: %w", err)
	}
	screens, _ := loose["screens"].(map[string]any)
	one, _ := screens[story.Start].(map[string]any)
	if one == nil {
		return fmt.Errorf("the fixture holds no screen called %s", story.Start)
	}
	one["html"] = markup
	delete(one, "el")
	raw, err := json.Marshal(loose)
	if err != nil {
		return fmt.Errorf("writing the changed fixture: %w", err)
	}
	w.flows.Raw = raw
	held := w.flows.Screens[story.Start]
	held.HTML = markup
	w.flows.Screens[story.Start] = held
	w.screen = story.Start
	return nil
}
