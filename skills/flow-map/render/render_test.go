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
// FLOW-2: every colour and font the page draws on a screen comes from the tokens object of the
// flows.json it was given.
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

// FLOW-1. Each surface gets its own frame, and only its own. A page that drew every screen the
// same way would still pass a test that only looked for a phone.
func TestAWebScreenIsDrawnInABrowserFrameAndAMobileScreenInAPhone(t *testing.T) {
	drawn := page(t)
	flows := fixture(t)

	seen := map[string]int{}
	for id, screen := range flows.Screens {
		markup, err := drawn.Screen(flows, id)
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

	markup, err := drawn.Screen(flows, "sign-in")
	if err != nil {
		t.Fatalf("drawing a screen with no surface: %v", err)
	}
	if !strings.Contains(markup, `data-surface="mobile"`) {
		t.Errorf("a screen naming no surface was not drawn on a phone:\n%s", markup)
	}
}

// FLOW-2, in the markup. Every colour the page wrote is one the tokens name, and every colour the
// tokens name reached the frame.
func TestEveryColourOnAScreenComesFromTheTokens(t *testing.T) {
	drawn := page(t)
	flows := fixture(t)

	named := map[string]bool{}
	for _, value := range flows.TokenValues() {
		named[value] = true
	}
	if len(named) == 0 {
		t.Fatal("the fixture names no tokens, so this test proves nothing")
	}

	for id := range flows.Screens {
		markup, err := drawn.Screen(flows, id)
		if err != nil {
			t.Fatalf("drawing %s: %v", id, err)
		}
		for _, found := range colourLiteral.FindAllString(markup, -1) {
			if !named[found] {
				t.Errorf("%s was drawn with the colour %q, which the tokens do not name", id, found)
			}
		}
		for name, value := range flows.Tokens["colour"] {
			if !strings.Contains(markup, "--t-colour-"+name+":"+value) {
				t.Errorf("%s does not carry the colour token %s, so the page is not drawing it in the project's own colours", id, name)
			}
		}
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

// Every kind of shape the schema allows is a kind the page can draw. A type the schema names and
// the page does not know renders as nothing, and a screen then quietly loses a shape.
func TestThePageDrawsEveryKindOfShapeTheSchemaAllows(t *testing.T) {
	drawn := page(t)
	flows := fixture(t)

	for _, kind := range elementKinds(t) {
		one := fmt.Sprintf(`{"tokens":%s,"screens":{"one":{"name":"One","surface":"mobile","status":"designed",
			"el":[{"t":%q,"component":"Thing","v":[]}]}}}`, tokensJSON(t, flows), kind)
		markup, err := drawn.Screen(Flows{Raw: []byte(one)}, "one")
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

func tokensJSON(t *testing.T, flows Flows) string {
	t.Helper()
	body, err := json.Marshal(flows.Tokens)
	if err != nil {
		t.Fatalf("writing the tokens: %v", err)
	}
	return string(body)
}
