package features_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/atlantic-blue/quay-krewe/internal/store"
	flowmap "github.com/atlantic-blue/quay-krewe/skills/flow-map/render"
	"github.com/cucumber/godog"
)

// Most of these scenarios touch the control plane nowhere. What they prove is a property of the
// page the skill ships: which frame a surface is drawn in, what an operator reads inside it, and
// where the colours come from. So they run the page's own render block over the skill's fixture
// and read the markup it answers with.
//
// The last one is the exception. The worked example beside the brief is written to the control
// plane, because what holds an example right is the check that reads a real mockup.

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
	theMissingMark    = `<img data-component="Logo" src="asset:missing" alt="the mark">`
	// The words sit in a part that names no component, so nothing says which component a building
	// session puts there. The gate refuses this shape now, and an artifact stored before the gate is
	// what an operator opens, so the view is where it is read.
	theWordsOutsideEveryComponent = `<section class="tides"><h1 data-component="Heading">Today</h1>` +
		`<p class="note">Two tides, both after dark.</p></section>`
)

type flowMapWorld struct {
	page      flowmap.Page
	flows     flowmap.Flows
	system    flowmap.System
	drawn     string
	document  string
	screen    string
	documents map[string]string
	courier   *flowmap.Courier
	opens     string
	posted    string
	gaps      []flowmap.GapRow
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
		for _, refuse := range []string{"onclick", "steal(", "parent.document", "document.title"} {
			if strings.Contains(w.document, refuse) {
				return fmt.Errorf("the screen still holds %q, which the session wrote:\n%s", refuse, w.document)
			}
		}
		scripts := flowmap.ScriptsIn(w.document)
		if len(scripts) != 1 {
			return fmt.Errorf("the screen carries %d scripts, and the page writes one, the courier:\n%s",
				len(scripts), w.document)
		}
		for _, want := range []string{"krewe", "press", "data-to"} {
			if !strings.Contains(scripts[0], want) {
				return fmt.Errorf("the one script in the screen holds no %q, so it is not the courier:\n%s",
					want, scripts[0])
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

	// The four things the font files and the images stand on. The fixture is held to two of them
	// first, because the green is cheap to fake: the design system writes no @font-face rule of its
	// own, and the mark is named both ways a screen can name it.
	sc.Step(`^the design system carries the font file and the mark of the project$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		if strings.Contains(w.system.CSS, "@font-face") {
			return fmt.Errorf("the design system writes its own @font-face rule, so this scenario would pass with no page behind it")
		}
		fonts, images := flowmap.AssetsByKind(w.system)
		if len(fonts) == 0 || len(images) == 0 {
			return fmt.Errorf("the design system carries %d font files and %d images, so this scenario proves nothing",
				len(fonts), len(images))
		}
		named := 0
		for _, screen := range w.flows.Screens {
			for name := range images {
				if strings.Contains(screen.HTML, "asset:"+name) {
					named++
				}
			}
		}
		if named == 0 {
			return fmt.Errorf("no screen of the project names an image in its markup, so one of the two ways is never read")
		}
		if !strings.Contains(w.system.CSS, "url(asset:") {
			return fmt.Errorf("the stylesheet of the design system names no image, so the other way is never read")
		}
		return nil
	})

	sc.Step(`^the operator opens every screen the project wrote as markup$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		w.documents = map[string]string{}
		for id, screen := range w.flows.Screens {
			if screen.HTML == "" {
				continue
			}
			if err := w.open(id); err != nil {
				return err
			}
			if w.document == "" {
				return fmt.Errorf("%s was drawn without a document of its own:\n%s", id, w.drawn)
			}
			w.documents[id] = w.document
		}
		if len(w.documents) == 0 {
			return fmt.Errorf("no screen of the project is written as markup, so nothing was opened")
		}
		return nil
	})

	sc.Step(`^each screen is drawn in the font file the design system carries$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		fonts, _ := flowmap.AssetsByKind(w.system)
		for id, document := range w.documents {
			for name, asset := range fonts {
				if rule, whyNot := flowmap.FontFaceFor(document, asset); rule == "" {
					return fmt.Errorf("%s is drawn in no rule for the font file %s: %s", id, name, whyNot)
				}
			}
		}
		return nil
	})

	sc.Step(`^the mark of the project reaches the screen, named in the markup and named in the stylesheet$`,
		func(ctx context.Context) error {
			w := flowMapFrom(ctx)
			_, images := flowmap.AssetsByKind(w.system)
			fromTheMarkup := 0
			for id, document := range w.documents {
				for name, asset := range images {
					address := flowmap.DataAddressOf(asset)
					if !strings.Contains(document, "url("+address+")") {
						return fmt.Errorf("%s does not draw the image %s from the stylesheet as a data address:\n%s",
							id, name, document)
					}
					if !strings.Contains(w.flows.Screens[id].HTML, "asset:"+name) {
						continue
					}
					fromTheMarkup++
					if !strings.Contains(document, `src="`+address+`"`) {
						return fmt.Errorf("%s names the image %s in its markup and does not draw it as a data address:\n%s",
							id, name, document)
					}
				}
			}
			if fromTheMarkup == 0 {
				return fmt.Errorf("no screen read an image named in its markup, so only one of the two ways was read")
			}
			return nil
		})

	sc.Step(`^no screen fetches anything, because every address it draws with is inside it$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		for id, document := range w.documents {
			for _, address := range flowmap.AddressesIn(document) {
				if !strings.HasPrefix(address, "data:") && !strings.HasPrefix(address, "#") {
					return fmt.Errorf("%s draws with the address %q, which is outside the document", id, address)
				}
			}
			for name := range w.system.Assets {
				if strings.Contains(document, "asset:"+name) {
					return fmt.Errorf("%s still names the asset %s by name, so the file never reached the screen:\n%s",
						id, name, document)
				}
			}
		}
		return nil
	})

	sc.Step(`^a file the design system does not carry stays named, so the operator reads what is missing$`,
		func(ctx context.Context) error {
			w := flowMapFrom(ctx)
			if err := w.write(theScreenMarkup + theMissingMark); err != nil {
				return err
			}
			if err := w.open(w.screen); err != nil {
				return err
			}
			if !strings.Contains(w.document, "asset:missing") {
				return fmt.Errorf("a mark the design system does not carry was dropped, so nothing says it is missing:\n%s",
					w.document)
			}
			return nil
		})

	// The press. The screen is one of the project's own, because what is proved is that an operator
	// playing a story gets from one screen to the next, and the screens are read in name order so
	// two runs read the same one.
	sc.Step(`^a screen a session wrote as markup, holding a part that opens another screen$`,
		func(ctx context.Context) error {
			w := flowMapFrom(ctx)
			ids := make([]string, 0, len(w.flows.Screens))
			for id := range w.flows.Screens {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			for _, id := range ids {
				found := aPartThatOpens.FindStringSubmatch(w.flows.Screens[id].HTML)
				if found == nil {
					continue
				}
				if _, held := w.flows.Screens[found[1]]; !held {
					continue
				}
				w.opens = found[1]
				return w.open(id)
			}
			return fmt.Errorf("no screen of the project holds a part that opens another screen, so this scenario proves nothing")
		})

	sc.Step(`^the operator presses that part$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		courier, err := flowmap.CourierIn(w.document)
		if err != nil {
			return fmt.Errorf("the screen %s carries no courier, so a press in it reaches nothing: %w", w.screen, err)
		}
		w.courier = courier
		posted, err := courier.Press(w.opens)
		if err != nil {
			return err
		}
		if len(posted) != 1 {
			return fmt.Errorf("a press on the part of %s that opens %s posted %d messages, and one is what it takes",
				w.screen, w.opens, len(posted))
		}
		w.posted = posted[0]
		return nil
	})

	sc.Step(`^the page opens the screen that part names$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		to, read, err := w.page.PressFrom(w.posted, true)
		if err != nil {
			return err
		}
		if !read {
			return fmt.Errorf("the page read no press in %s, which the screen it drew posted", w.posted)
		}
		if to != w.opens {
			return fmt.Errorf("the page opens %q, and the part that was pressed names %q", to, w.opens)
		}
		if _, held := w.flows.Screens[to]; !held {
			return fmt.Errorf("the page opens %q, which the project holds no screen for", to)
		}
		return nil
	})

	sc.Step(`^a press that came from anywhere else opens nothing$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		if to, read, err := w.page.PressFrom(w.posted, false); err != nil {
			return err
		} else if read {
			return fmt.Errorf("the page opened %q on a message from a window it did not draw", to)
		}
		for _, other := range []string{`{"hello":"there"}`, `{"krewe":"flash"}`, `null`} {
			to, read, err := w.page.PressFrom(other, true)
			if err != nil {
				return err
			}
			if read {
				return fmt.Errorf("the page opened %q on the message %s, which names no press", to, other)
			}
		}
		return nil
	})

	sc.Step(`^a press on a spot that opens nothing shows the operator what can be pressed$`,
		func(ctx context.Context) error {
			w := flowMapFrom(ctx)
			posted, err := w.courier.Press("")
			if err != nil {
				return err
			}
			if len(posted) != 1 {
				return fmt.Errorf("a press on a spot that opens nothing posted %d messages, and one is what it takes",
					len(posted))
			}
			to, read, err := w.page.PressFrom(posted[0], true)
			if err != nil {
				return err
			}
			if !read || to != "" {
				return fmt.Errorf("a press on a spot that opens nothing was read as %q, read=%v", to, read)
			}
			lit, cleared, err := w.courier.Flash()
			if err != nil {
				return err
			}
			if len(lit) == 0 {
				return fmt.Errorf("the screen outlined nothing, so the operator is told nothing about what can be pressed")
			}
			if !cleared {
				return fmt.Errorf("the screen outlined %v and left the outline on", lit)
			}
			return nil
		})

	// What is missing. The screen carries markup a gate never read, which is the artifact an operator
	// opens, and the view has to name the part a building session would have to guess at.
	//
	// One screen of the project is left with nothing to draw. A write asks every screen for its
	// markup now, so the only artifact carrying such a screen is one that was stored before, and that
	// is the artifact an operator opens the Gaps view on.
	sc.Step(`^a screen a session wrote as markup, holding words outside every named component$`,
		func(ctx context.Context) error {
			w := flowMapFrom(ctx)
			if err := w.write(theWordsOutsideEveryComponent); err != nil {
				return err
			}
			return w.storeAScreenNobodyDrew()
		})

	sc.Step(`^the operator reads what is missing from the project$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		rows, err := w.page.Gaps(w.flows)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return fmt.Errorf("the view answers no row at all, so an operator reads an empty page as good news")
		}
		w.gaps = rows
		return nil
	})

	sc.Step(`^the view names the part that names no component, by its tag, its class and its first words$`,
		func(ctx context.Context) error {
			w := flowMapFrom(ctx)
			row, found := theGapRow(w.gaps, w.screen, "No component")
			if !found {
				return fmt.Errorf("the view says nothing about the part of %s that names no component:\n%s",
					w.screen, whatTheViewSaid(w.gaps))
			}
			for _, want := range []string{"<p", "note", "Two tides, both after dark."} {
				if !strings.Contains(row.Words, want) {
					return fmt.Errorf("the row does not name %q, so the operator cannot find the part: %q", want, row.Words)
				}
			}
			return nil
		})

	sc.Step(`^it names the screen with nothing to draw$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		empty, err := screensDrawingNothing(w.flows)
		if err != nil {
			return err
		}
		if len(empty) == 0 {
			return fmt.Errorf("every screen of the project draws something, so this step proves nothing")
		}
		for _, id := range empty {
			if _, found := theGapRow(w.gaps, id, "No markup"); !found {
				return fmt.Errorf("%s draws nothing and the view says nothing about it:\n%s", id, whatTheViewSaid(w.gaps))
			}
		}
		return nil
	})

	sc.Step(`^it names the press that opens a screen the project does not hold$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		read := 0
		for id, screen := range w.flows.Screens {
			for _, found := range aPartThatOpens.FindAllStringSubmatch(screen.HTML, -1) {
				if _, held := w.flows.Screens[found[1]]; held {
					continue
				}
				read++
				row, there := theGapRow(w.gaps, id, "No screen")
				if !there {
					return fmt.Errorf("%s holds a press that opens %s, which the project does not hold, and the view says nothing:\n%s",
						id, found[1], whatTheViewSaid(w.gaps))
				}
				if !strings.Contains(row.Words, found[1]) {
					return fmt.Errorf("the row for %s does not name the screen %s the press opens: %q", id, found[1], row.Words)
				}
			}
		}
		if read == 0 {
			return fmt.Errorf("no press of the project opens a screen it does not hold, so this step proves nothing")
		}
		return nil
	})

	sc.Step(`^it still names every defect the project holds$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		defects, err := theDefectsOf(w.flows)
		if err != nil {
			return err
		}
		if len(defects) == 0 {
			return fmt.Errorf("the project holds no defect, so this step proves nothing")
		}
		for id, said := range defects {
			for _, one := range said {
				row, found := theGapRow(w.gaps, id, "Missing")
				if !found || !strings.Contains(row.Words, one) {
					return fmt.Errorf("the view no longer names the defect %q of %s:\n%s", one, id, whatTheViewSaid(w.gaps))
				}
			}
		}
		return nil
	})

	sc.Step(`^on the map a press reaches the node under the screen$`, func(ctx context.Context) error {
		w := flowMapFrom(ctx)
		if !strings.Contains(strings.Join(strings.Fields(w.page.ScreenStyles), ""), ".nodeiframe.screen{pointer-events:none}") {
			return fmt.Errorf("no rule keeps a press off a frame on the map, so a node under one cannot be opened")
		}
		return nil
	})

	// The worked example, held to the check that reads a real mockup. The design system goes first,
	// because a screen is refused against the colours and the fonts the operator approved, so an
	// example written against its own design system is the only one a session can copy whole.
	sc.Step(`^the design system the flow map example ships is approved$`, func(ctx context.Context) error {
		artifact, err := theFlowMapExampleFile("design-system.json")
		if err != nil {
			return err
		}
		if err := writeStage(ctx, store.StageDesignSystem, "the design system of the example", artifact); err != nil {
			return err
		}
		if w := worldFrom(ctx); w.lastErr != nil {
			return fmt.Errorf("the design system the example ships was refused: %w", w.lastErr)
		}
		if err := approveStage(ctx, store.StageDesignSystem); err != nil {
			return err
		}
		if w := worldFrom(ctx); w.lastErr != nil {
			return fmt.Errorf("approving the design system the example ships was refused: %w", w.lastErr)
		}
		return nil
	})

	sc.Step(`^a session writes the screens the flow map example ships$`, func(ctx context.Context) error {
		artifact, err := theFlowMapExampleFile("flows.json")
		if err != nil {
			return err
		}
		return writeStage(ctx, store.StageMockups, "the screens of the example", artifact)
	})

	// Both surfaces, because a session copies the one nearest what it is writing, and a project
	// designed on a phone and in a browser needs a screen of each to copy.
	sc.Step(`^the example ships a screen on a phone and a screen in a browser$`, func() error {
		flows, err := theFlowMapExample()
		if err != nil {
			return err
		}
		surfaces := map[string]bool{}
		for id, screen := range flows.Screens {
			if strings.TrimSpace(screen.HTML) == "" {
				return fmt.Errorf("the %s screen of the example carries no markup, so there is nothing to copy", id)
			}
			surfaces[screen.Surface] = true
		}
		for _, surface := range []string{"mobile", "web"} {
			if !surfaces[surface] {
				return fmt.Errorf("the example ships no %s screen: it ships %v", surface, namesOf(surfaces))
			}
		}
		return nil
	})

	// A brief that never names the example leaves a session with a page of rules and nothing to
	// copy, and the example is the part that shows what a screen looks like written out.
	sc.Step(`^the flow map brief names the example beside it$`, func() error {
		dir, err := flowmap.Dir()
		if err != nil {
			return err
		}
		brief, err := os.ReadFile(filepath.Join(dir, "SKILL.md")) //nolint:gosec // the path is the skill's own brief
		if err != nil {
			return fmt.Errorf("reading the flow map brief: %w", err)
		}
		for _, named := range []string{"example/design-system.json", "example/flows.json"} {
			if !strings.Contains(string(brief), named) {
				return fmt.Errorf("the brief never names %s, so nothing sends a session to the example", named)
			}
		}
		return nil
	})
}

// theFlowMapExampleFile reads one file of the worked example as it is written on disk, because the
// bytes a session copies are the bytes the check has to keep.
func theFlowMapExampleFile(name string) (string, error) {
	dir, err := flowmap.Dir()
	if err != nil {
		return "", err
	}
	at := filepath.Join(dir, "example", name)
	body, err := os.ReadFile(at) //nolint:gosec // the path is the skill's own example
	if err != nil {
		return "", fmt.Errorf("reading the example the flow map skill ships: %w", err)
	}
	return string(body), nil
}

// theFlowMapExample is the example's flows.json, read the way the page reads one.
func theFlowMapExample() (flowmap.Flows, error) {
	dir, err := flowmap.Dir()
	if err != nil {
		return flowmap.Flows{}, err
	}
	flows, err := flowmap.ReadFlows(filepath.Join(dir, "example", "flows.json"))
	if err != nil {
		return flowmap.Flows{}, err
	}
	if len(flows.Screens) == 0 {
		return flowmap.Flows{}, fmt.Errorf("the example ships no screen, so this step proves nothing")
	}
	return flows, nil
}

// namesOf is the keys of a set, sorted, so a refusal reads the same way twice.
func namesOf(held map[string]bool) []string {
	var names []string
	for name := range held {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// theGapRow is the first row of one kind about one screen.
func theGapRow(rows []flowmap.GapRow, screen, kind string) (flowmap.GapRow, bool) {
	for _, row := range rows {
		if row.Screen == screen && row.Kind == kind {
			return row, true
		}
	}
	return flowmap.GapRow{}, false
}

// whatTheViewSaid is every row the view answered, so a failure names what there was instead.
func whatTheViewSaid(rows []flowmap.GapRow) string {
	if len(rows) == 0 {
		return "the view answered no row at all"
	}
	said := make([]string, 0, len(rows))
	for _, row := range rows {
		said = append(said, fmt.Sprintf("%s: %s: %s", row.Screen, row.Kind, row.Words))
	}
	return strings.Join(said, "\n")
}

// screensDrawingNothing are the screens carrying no markup.
func screensDrawingNothing(flows flowmap.Flows) ([]string, error) {
	var loose struct {
		Screens map[string]struct {
			HTML string `json:"html"`
		} `json:"screens"`
	}
	if err := json.Unmarshal(flows.Raw, &loose); err != nil {
		return nil, fmt.Errorf("reading the project's screens again: %w", err)
	}
	var out []string
	for id, screen := range loose.Screens {
		if screen.HTML == "" {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out, nil
}

// storeAScreenNobodyDrew takes the markup off the screen the mobile story ends on, leaving the
// project in the state an operator meets: an artifact stored before every screen was asked for its
// markup, holding one screen nobody has drawn.
func (w *flowMapWorld) storeAScreenNobodyDrew() error {
	var loose map[string]any
	if err := json.Unmarshal(w.flows.Raw, &loose); err != nil {
		return fmt.Errorf("reading the project again: %w", err)
	}
	screens, _ := loose["screens"].(map[string]any)
	for _, id := range sortedScreenNames(screens) {
		one, _ := screens[id].(map[string]any)
		if one == nil || id == w.screen {
			continue
		}
		delete(one, "html")
		raw, err := json.Marshal(loose)
		if err != nil {
			return fmt.Errorf("writing the changed project: %w", err)
		}
		w.flows.Raw = raw
		held := w.flows.Screens[id]
		held.HTML = ""
		w.flows.Screens[id] = held
		return nil
	}
	return fmt.Errorf("the project holds no second screen, so nothing can be left undrawn")
}

// sortedScreenNames reads the screens in one order, so the same screen is left undrawn every run.
func sortedScreenNames(screens map[string]any) []string {
	out := make([]string, 0, len(screens))
	for id := range screens {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// theDefectsOf is what each screen of a project says is missing from it.
func theDefectsOf(flows flowmap.Flows) (map[string][]string, error) {
	var loose struct {
		Screens map[string]struct {
			Defects []string `json:"defects"`
		} `json:"screens"`
	}
	if err := json.Unmarshal(flows.Raw, &loose); err != nil {
		return nil, fmt.Errorf("reading the project's screens again: %w", err)
	}
	out := map[string][]string{}
	for id, screen := range loose.Screens {
		if len(screen.Defects) > 0 {
			out[id] = screen.Defects
		}
	}
	return out, nil
}

// aPartThatOpens is a part of a screen that opens another one, with the name of that screen.
var aPartThatOpens = regexp.MustCompile(`data-to="([^"]+)"`)

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
