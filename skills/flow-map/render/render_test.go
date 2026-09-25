package render

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The two contracts this page stands on.
//
// FLOW-1: a screen with surface mobile renders in a phone frame, and a screen with surface web
// renders in a browser frame.
//
// FLOW-2: every colour and font the page draws on a screen comes from the design system of the
// project, which the operator approved before the mockups were written.
//
// SCREEN-1: a screen carries the markup of its own body, and the size of the surface it is written
// at.
//
// FRAME-1: that markup is composed into a document of its own, carrying the values of the project.
//
// FRAME-2: the document runs inside a frame that cannot run the script a session wrote, cannot
// reach an address, and cannot touch the page around it.
//
// Both are read off the markup the page's own renderer produces, run here outside a browser, so
// what these tests read is what an operator sees.

// colourLiteral is a colour written into the page rather than taken from the tokens.
var colourLiteral = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b|\brgba?\(|\bhsla?\(`)

// fontFamily is a font declaration in the stylesheet, with whatever it was set to.
var fontFamily = regexp.MustCompile(`font-family\s*:\s*([^;}]+)`)

func page(t *testing.T) Page {
	t.Helper()
	dir, err := Dir()
	if err != nil {
		t.Fatalf("finding the skill: %v", err)
	}
	read, err := ReadPage(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatalf("reading the page: %v", err)
	}
	return read
}

func fixture(t *testing.T) Flows {
	t.Helper()
	dir, err := Dir()
	if err != nil {
		t.Fatalf("finding the skill: %v", err)
	}
	read, err := ReadFlows(filepath.Join(dir, "fixtures", "two-surfaces.json"))
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	if len(read.Screens) == 0 {
		t.Fatal("the fixture holds no screens, so these tests prove nothing")
	}
	return read
}

func system(t *testing.T) System {
	t.Helper()
	dir, err := Dir()
	if err != nil {
		t.Fatalf("finding the skill: %v", err)
	}
	read, err := ReadSystem(filepath.Join(dir, "fixtures", "design-system.json"))
	if err != nil {
		t.Fatalf("reading the design system: %v", err)
	}
	return read
}

// FLOW-1. Each surface gets its own frame, and only its own. A page that drew every screen the
// same way would still pass a test that only looked for a phone.
func TestAWebScreenIsDrawnInABrowserFrameAndAMobileScreenInAPhone(t *testing.T) {
	drawn := page(t)
	flows := fixture(t)
	system := system(t)

	seen := map[string]int{}
	for id, screen := range flows.Screens {
		markup, err := drawn.Screen(flows, system, id)
		if err != nil {
			t.Fatalf("drawing %s: %v", id, err)
		}
		seen[screen.Surface]++

		switch screen.Surface {
		case "web":
			for _, want := range []string{`data-surface="web"`, `class="frame frame-web"`, `class="addr"`, screen.Route} {
				if !strings.Contains(markup, want) {
					t.Errorf("%s is a web screen and its frame carries no %q:\n%s", id, want, markup)
				}
			}
			for _, refuse := range []string{"frame-mobile", "bezel", "notch"} {
				if strings.Contains(markup, refuse) {
					t.Errorf("%s is a web screen and it was drawn with %q, which belongs to a phone", id, refuse)
				}
			}
		case "mobile":
			for _, want := range []string{`data-surface="mobile"`, `class="frame frame-mobile"`, `class="bezel"`, `class="notch"`} {
				if !strings.Contains(markup, want) {
					t.Errorf("%s is a mobile screen and its frame carries no %q:\n%s", id, want, markup)
				}
			}
			for _, refuse := range []string{"frame-web", "chrome", "addr"} {
				if strings.Contains(markup, refuse) {
					t.Errorf("%s is a mobile screen and it was drawn with %q, which belongs to a browser", id, refuse)
				}
			}
		default:
			t.Errorf("%s names the surface %q, and a screen names mobile or web", id, screen.Surface)
		}
	}
	if seen["web"] == 0 || seen["mobile"] == 0 {
		t.Fatalf("the fixture holds %d web screens and %d mobile screens, so it does not prove both frames",
			seen["web"], seen["mobile"])
	}
}

// A screen that names no surface is drawn on a phone rather than refused, and the gaps view says
// so. This pins the default, because a page that fell back to a browser would quietly show a web
// frame for every screen somebody forgot to mark.
func TestAScreenThatNamesNoSurfaceIsDrawnOnAPhone(t *testing.T) {
	drawn := page(t)
	flows := fixture(t)
	system := system(t)

	var loose map[string]any
	if err := json.Unmarshal(flows.Raw, &loose); err != nil {
		t.Fatalf("reading the fixture again: %v", err)
	}
	screens, _ := loose["screens"].(map[string]any)
	one, _ := screens["sign-in"].(map[string]any)
	if one == nil {
		t.Fatal("the fixture has no sign-in screen, so this test proves nothing")
	}
	delete(one, "surface")
	raw, err := json.Marshal(loose)
	if err != nil {
		t.Fatalf("writing the changed fixture: %v", err)
	}
	flows.Raw = raw

	markup, err := drawn.Screen(flows, system, "sign-in")
	if err != nil {
		t.Fatalf("drawing a screen with no surface: %v", err)
	}
	if !strings.Contains(markup, `data-surface="mobile"`) {
		t.Errorf("a screen naming no surface was not drawn on a phone:\n%s", markup)
	}
}

// FLOW-2, in the document an operator reads. Every colour on the screen is one the design system
// names, and every colour the design system names reached the screen. The flows.json carries no
// tokens now, so a page that still read them would draw a screen with no value at all.
func TestEveryColourOnAScreenComesFromTheDesignSystem(t *testing.T) {
	drawn := page(t)
	flows := fixture(t)
	system := system(t)

	named := map[string]bool{}
	for _, value := range system.TokenValues() {
		named[value] = true
	}
	if len(named) == 0 {
		t.Fatal("the design system names no value, so this test proves nothing")
	}

	written := 0
	for id, screen := range flows.Screens {
		markup, err := drawn.Screen(flows, system, id)
		if err != nil {
			t.Fatalf("drawing %s: %v", id, err)
		}
		read := markup
		if document, found := DocumentIn(markup); found {
			read = markup + document
			written++
		}
		for _, found := range colourLiteral.FindAllString(read, -1) {
			if !named[found] {
				t.Errorf("%s was drawn with the colour %q, which the design system does not name", id, found)
			}
		}
		for name, value := range system.Tokens["colour"] {
			if !strings.Contains(read, "--t-colour-"+name+":"+value) {
				t.Errorf("%s does not carry the colour token %s, so it is not drawn in the project's own colours", id, name)
			}
		}
		if screen.HTML != "" && written == 0 {
			t.Errorf("%s is written as markup and no document reached a frame", id)
		}
	}
	if written == 0 {
		t.Fatal("no screen of the fixture is written as markup, so the document was never read")
	}
}

// FLOW-2, in the stylesheet. The markup above only carries the token values; the rules that spend
// them are in the page. A hex here would paint every project the same and nothing above would see
// it, so this reads the block the page marks as the part that paints a screen.
func TestNoRuleThatPaintsAScreenHoldsAColourOrAFontOfItsOwn(t *testing.T) {
	drawn := page(t)

	if !strings.Contains(drawn.ScreenStyles, ".screen") {
		t.Fatal("the marked block holds no .screen rule, so it is not the part that paints a screen")
	}
	for _, found := range colourLiteral.FindAllString(drawn.ScreenStyles, -1) {
		t.Errorf("a rule that paints a screen holds the colour %q, so it is this page's opinion rather than the project's", found)
	}
	for _, found := range fontFamily.FindAllStringSubmatch(drawn.ScreenStyles, -1) {
		if !strings.HasPrefix(strings.TrimSpace(found[1]), "var(") {
			t.Errorf("a rule that paints a screen sets font-family to %q rather than to a token", strings.TrimSpace(found[1]))
		}
	}
	// Nothing outside the block may reach a screen, or the two checks above are a fence with a gate
	// beside it.
	for _, reserved := range []string{".screen", ".frame", ".bezel", ".chrome"} {
		if strings.Contains(drawn.OtherStyles, reserved+"{") || strings.Contains(drawn.OtherStyles, reserved+" ") {
			t.Errorf("a rule outside the marked block names %s, so a screen can be painted where nothing checks the values", reserved)
		}
	}
}

// SCREEN-1 and FRAME-1. A screen a session wrote as markup reaches an operator as that markup, in
// a document of its own, at the size of the surface it was written at.
func TestAScreenWrittenInMarkupIsDrawnInsideAFrame(t *testing.T) {
	drawn := page(t)
	flows := fixture(t)
	system := system(t)

	for id, screen := range flows.Screens {
		if screen.HTML == "" {
			continue
		}
		markup, err := drawn.Screen(flows, system, id)
		if err != nil {
			t.Fatalf("drawing %s: %v", id, err)
		}
		for _, want := range []string{"<iframe", `class="screen"`, `loading="lazy"`, `srcdoc="`, `title="` + screen.Name} {
			if !strings.Contains(markup, want) {
				t.Errorf("%s is written as markup and its frame carries no %q:\n%s", id, want, markup)
			}
		}
		document, found := DocumentIn(markup)
		if !found {
			t.Fatalf("%s was drawn without a document:\n%s", id, markup)
		}
		if !strings.Contains(document, "<!doctype html>") {
			t.Errorf("the document of %s is not a document of its own:\n%s", id, document)
		}
		for _, want := range visibleWordsOf(screen.HTML) {
			if !strings.Contains(document, want) {
				t.Errorf("the document of %s does not hold %q, which the session wrote", id, want)
			}
		}
		// The frame keeps the size it has today, so the document is scaled into it rather than
		// pushing the bezel out.
		if !strings.Contains(markup, "transform:scale(") {
			t.Errorf("%s is not scaled into its frame, so the bezel does not keep its size:\n%s", id, markup)
		}
		return
	}
	t.Fatal("no screen of the fixture is written as markup, so this test proves nothing")
}

// FRAME-2. The markup came out of a store that no gate read, so the page cannot trust it. What an
// operator opens runs nothing the session wrote, reaches no address, and touches no other screen.
func TestTheFrameRunsNoScriptTheSessionWrote(t *testing.T) {
	drawn := page(t)
	system := system(t)

	stored := `{"screens":{"one":{"name":"One","surface":"mobile","status":"designed","html":` +
		`"<main data-component=\"Card\"><h1 onclick=\"steal()\">Today</h1>` +
		`<script>parent.document.title='taken'</script></main>"}}}`
	markup, err := drawn.Screen(Flows{Raw: []byte(stored)}, system, "one")
	if err != nil {
		t.Fatalf("drawing a stored screen: %v", err)
	}
	document, found := DocumentIn(markup)
	if !found {
		t.Fatalf("the screen was drawn without a document:\n%s", markup)
	}

	if !strings.Contains(markup, `sandbox="allow-scripts"`) {
		t.Errorf("the frame does not carry the sandbox attribute, so the document can reach the page:\n%s", markup)
	}
	for _, want := range []string{
		"default-src 'none'", "img-src data:", "font-src data:",
		"style-src 'unsafe-inline'", "script-src 'unsafe-inline'",
		"form-action 'none'", "base-uri 'none'",
	} {
		if !strings.Contains(document, want) {
			t.Errorf("the document does not refuse an address with %q:\n%s", want, document)
		}
	}
	for _, refuse := range []string{"<script", "onclick", "steal(", "document.title"} {
		if strings.Contains(document, refuse) {
			t.Errorf("the document still holds %q, which the session wrote:\n%s", refuse, document)
		}
	}
	if !strings.Contains(document, "Today") {
		t.Errorf("the words the session wrote did not survive:\n%s", document)
	}
}

// SCREEN-1. A session writes css at the real size of the surface, so the size is the surface's own
// unless the screen names another.
func TestTheViewportIsTheSizeOfTheSurface(t *testing.T) {
	drawn := page(t)
	system := system(t)

	for _, one := range []struct {
		surface, viewport string
		width, height     int
	}{
		{surface: "mobile", width: 390, height: 844},
		{surface: "web", width: 1280, height: 800},
		{surface: "mobile", viewport: `,"viewport":{"width":320,"height":568}`, width: 320, height: 568},
	} {
		stored := fmt.Sprintf(`{"screens":{"one":{"name":"One","surface":%q,"status":"designed",`+
			`"html":"<p data-component=\"Text\">a</p>"%s}}}`, one.surface, one.viewport)
		markup, err := drawn.Screen(Flows{Raw: []byte(stored)}, system, "one")
		if err != nil {
			t.Fatalf("drawing a %s screen: %v", one.surface, err)
		}
		for _, want := range []string{
			fmt.Sprintf(`width="%d"`, one.width),
			fmt.Sprintf(`height="%d"`, one.height),
		} {
			if !strings.Contains(markup, want) {
				t.Errorf("a %s screen%s carries no %s:\n%s", one.surface, ofItsOwn(one.viewport), want, markup)
			}
		}
		document, found := DocumentIn(markup)
		if !found {
			t.Fatalf("a %s screen was drawn without a document", one.surface)
		}
		if !strings.Contains(document, fmt.Sprintf("width:%dpx", one.width)) {
			t.Errorf("the document of a %s screen is not sized to its viewport:\n%s", one.surface, document)
		}
	}
}

// The discovery stage is played before any design system exists, so the route answers nothing and
// the page draws the screen with no value of its own rather than inventing one.
func TestAProjectWithNoDesignSystemDrawsWithNoValueOfThePagesOwn(t *testing.T) {
	drawn := page(t)

	stored := `{"screens":{"one":{"name":"One","surface":"mobile","status":"designed",` +
		`"html":"<p data-component=\"Text\">a</p>"}}}`
	markup, err := drawn.Screen(Flows{Raw: []byte(stored)}, System{}, "one")
	if err != nil {
		t.Fatalf("drawing a screen with no design system: %v", err)
	}
	document, found := DocumentIn(markup)
	if !found {
		t.Fatalf("the screen was drawn without a document:\n%s", markup)
	}
	if strings.Contains(document, "--t-") {
		t.Errorf("the document carries a custom property with no design system behind it:\n%s", document)
	}
	for _, found := range colourLiteral.FindAllString(document, -1) {
		t.Errorf("the page drew the screen in the colour %q, which no design system names", found)
	}
	// And the operator is told, because a screen drawn in nothing looks like a screen drawn badly.
	if !strings.Contains(drawn.HTML, "design system") {
		t.Error("the page says nothing when a project has no design system")
	}
}

// The shape list is removed at step 10, so until then a stored screen that carries one draws the
// way it does today. A page that only drew markup would empty every mockup already written.
func TestAScreenWithNoMarkupStillDrawsItsShapeList(t *testing.T) {
	drawn := page(t)
	system := system(t)

	stored := `{"screens":{"one":{"name":"One","surface":"mobile","status":"designed",` +
		`"el":[{"t":"h","component":"Heading","v":"Today"}]}}}`
	markup, err := drawn.Screen(Flows{Raw: []byte(stored)}, system, "one")
	if err != nil {
		t.Fatalf("drawing a screen with a shape list: %v", err)
	}
	if strings.Contains(markup, "<iframe") {
		t.Errorf("a screen with no markup was given a frame:\n%s", markup)
	}
	for _, want := range []string{`class="h"`, `data-component="Heading"`, "Today"} {
		if !strings.Contains(markup, want) {
			t.Errorf("a screen with a shape list carries no %q:\n%s", want, markup)
		}
	}
}

// SCREEN-1, in the file a session writes against. The markup and the size are what the schema now
// takes, and the shape list stops being asked for, so a screen can be markup alone.
func TestTheSchemaTakesMarkupAndTheSizeOfAScreen(t *testing.T) {
	read := schema(t)
	screen := definition(t, read, "screen")
	properties, _ := screen["properties"].(map[string]any)

	if _, there := properties["html"]; !there {
		t.Fatal("a screen cannot carry its markup, which is what this feature is for")
	}
	viewport, _ := properties["viewport"].(map[string]any)
	if viewport == nil {
		t.Fatal("a screen cannot say what size it was written at")
	}
	sizes, _ := viewport["properties"].(map[string]any)
	for _, one := range []struct {
		name     string
		from, to float64
	}{{name: "width", from: 240, to: 1920}, {name: "height", from: 320, to: 1600}} {
		held, _ := sizes[one.name].(map[string]any)
		if held == nil {
			t.Fatalf("the viewport has no %s", one.name)
		}
		if at, _ := held["minimum"].(float64); at != one.from {
			t.Errorf("the %s of a viewport starts at %v rather than %v", one.name, held["minimum"], one.from)
		}
		if at, _ := held["maximum"].(float64); at != one.to {
			t.Errorf("the %s of a viewport stops at %v rather than %v", one.name, held["maximum"], one.to)
		}
	}
	if required(screen, "el") {
		t.Error("the schema still asks a screen for a shape list, so a screen cannot be markup alone")
	}
	if required(read, "tokens") {
		t.Error("the schema still asks the flows for tokens, which the design system stage now holds")
	}
}

// visibleWordsOf is the words a person reads on a screen, taken out of the markup, so a test can
// look for them in the document the frame was given.
func visibleWordsOf(markup string) []string {
	var out []string
	for _, part := range strings.Split(markup, ">") {
		words := strings.TrimSpace(strings.Split(part, "<")[0])
		if len(words) > 3 && !strings.Contains(words, "=") {
			out = append(out, words)
		}
	}
	if len(out) > 3 {
		return out[:3]
	}
	return out
}

// ofItsOwn names the case in a failure, because the same check runs for a default size and for a
// size the screen asked for.
func ofItsOwn(viewport string) string {
	if viewport == "" {
		return ""
	}
	return " naming its own size"
}

// Every kind of shape the schema allows is a kind the page can draw. A type the schema names and
// the page does not know renders as nothing, and a screen then quietly loses a shape.
func TestThePageDrawsEveryKindOfShapeTheSchemaAllows(t *testing.T) {
	drawn := page(t)

	for _, kind := range elementKinds(t) {
		one := fmt.Sprintf(`{"screens":{"one":{"name":"One","surface":"mobile","status":"designed",
			"el":[{"t":%q,"component":"Thing","v":[]}]}}}`, kind)
		markup, err := drawn.Screen(Flows{Raw: []byte(one)}, system(t), "one")
		if err != nil {
			t.Fatalf("drawing a %s: %v", kind, err)
		}
		if !strings.Contains(markup, `data-component="Thing"`) {
			t.Errorf("the page draws nothing for a shape of kind %q, which the schema allows", kind)
		}
	}
}

// The schema is what step 5 refuses a mockup against, so the two rules it has to carry are pinned
// here: a screen names its surface, and every shape names the component it stands for.
func TestTheSchemaAsksForASurfaceAndAComponent(t *testing.T) {
	read := schema(t)

	screen := definition(t, read, "screen")
	if !required(screen, "surface") {
		t.Error("the schema does not ask a screen for a surface, so nothing says which frame it is drawn in")
	}
	element := definition(t, read, "element")
	if !required(element, "component") {
		t.Error("the schema does not ask a shape for a component, so a build cannot read which component goes where")
	}

	// And the fixture answers both, or the skill ships an example that its own schema refuses.
	flows := fixture(t)
	var loose struct {
		Screens map[string]struct {
			Surface string `json:"surface"`
			El      []struct {
				Component string `json:"component"`
			} `json:"el"`
		} `json:"screens"`
	}
	if err := json.Unmarshal(flows.Raw, &loose); err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	for id, one := range loose.Screens {
		if one.Surface != "mobile" && one.Surface != "web" {
			t.Errorf("the fixture screen %s names the surface %q", id, one.Surface)
		}
		for at, shape := range one.El {
			if strings.TrimSpace(shape.Component) == "" {
				t.Errorf("shape %d of the fixture screen %s names no component", at, id)
			}
		}
	}
}

func schema(t *testing.T) map[string]any {
	t.Helper()
	dir, err := Dir()
	if err != nil {
		t.Fatalf("finding the skill: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "schema.json"))
	if err != nil {
		t.Fatalf("reading the schema: %v", err)
	}
	var read map[string]any
	if err := json.Unmarshal(body, &read); err != nil {
		t.Fatalf("the schema is not readable: %v", err)
	}
	return read
}

func definition(t *testing.T, read map[string]any, name string) map[string]any {
	t.Helper()
	defs, _ := read["$defs"].(map[string]any)
	one, _ := defs[name].(map[string]any)
	if one == nil {
		t.Fatalf("the schema holds no definition called %s", name)
	}
	return one
}

func required(one map[string]any, field string) bool {
	list, _ := one["required"].([]any)
	for _, held := range list {
		if name, ok := held.(string); ok && name == field {
			return true
		}
	}
	return false
}

// elementKinds is every value the schema allows for the kind of a shape.
func elementKinds(t *testing.T) []string {
	t.Helper()
	element := definition(t, schema(t), "element")
	properties, _ := element["properties"].(map[string]any)
	kind, _ := properties["t"].(map[string]any)
	allowed, _ := kind["enum"].([]any)
	if len(allowed) == 0 {
		t.Fatal("the schema allows no kind of shape, so this test proves nothing")
	}
	out := make([]string, 0, len(allowed))
	for _, one := range allowed {
		if name, ok := one.(string); ok {
			out = append(out, name)
		}
	}
	return out
}
