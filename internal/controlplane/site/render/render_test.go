package render_test

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/controlplane/site/render"
	"github.com/atlantic-blue/quay-krewe/internal/store"
)

// SITE-4: the menu lists Design and then the six stages in position order, and each stage carries
// exactly one of the states approved, changed since approval, written, not written and skipped,
// drawn from version, approved_version and skipped.
//
// Every assertion here reads the markup the page's own renderer produces, run outside a browser, so
// what these tests read is what an operator sees.

// entry reads one menu entry out of the markup: which entry it is, and the state it carries.
var entry = regexp.MustCompile(`data-entry="([^"]*)"[^>]*data-state="([^"]*)"`)

// stateText is the word an entry shows a reader, which is the same word as its state attribute. An
// entry whose attribute and words disagree tells a person one thing and a test another.
var stateText = regexp.MustCompile(`<span class="state">([^<]*)</span>`)

func script(t *testing.T) render.Script {
	t.Helper()
	read, err := render.ReadSite()
	if err != nil {
		t.Fatalf("reading the site's script: %v", err)
	}
	return read
}

// stagesJSON is a stages.json as the site serves one: the design, and a row for each stage written.
func stagesJSON(t *testing.T, brief string, rows ...map[string]any) string {
	t.Helper()
	for at, row := range rows {
		if _, held := row["position"]; !held {
			position, known := store.DesignStagePosition(fmt.Sprint(row["stage"]))
			if !known {
				t.Fatalf("row %d names %q, which is not one of the six stages", at, row["stage"])
			}
			row["position"] = position
		}
	}
	document, err := json.Marshal(map[string]any{
		"design": map[string]any{"brief": brief, "body": ""},
		"stages": rows,
	})
	if err != nil {
		t.Fatalf("writing the stages document: %v", err)
	}
	return string(document)
}

func written(stage string, version, approved int) map[string]any {
	return map[string]any{
		"stage": stage, "body": "the " + stage + " body",
		"version": version, "approved_version": approved,
	}
}

// The menu is every stage, whether or not anybody wrote one, in the order the stages are written in.
// A menu drawn from the rows alone would list two entries for a project on its second stage, and the
// operator would have no way to see what is still to come.
func TestTheMenuListsDesignAndThenTheSixStagesInOrder(t *testing.T) {
	drawn, err := script(t).Menu(stagesJSON(t, "four bills, and two of them move",
		written(store.StageDiscovery, 1, 1)))
	if err != nil {
		t.Fatalf("drawing the menu: %v", err)
	}

	var listed []string
	for _, found := range entry.FindAllStringSubmatch(drawn, -1) {
		listed = append(listed, found[1])
	}
	want := append([]string{"design"}, store.DesignStages()...)
	if strings.Join(listed, ",") != strings.Join(want, ",") {
		t.Fatalf("the menu lists %v, want %v:\n%s", listed, want, drawn)
	}
}

// The order lives in the script because a stage nobody wrote has no row and therefore no position.
// Held against the store's own list, so the page and the table cannot come to disagree about which
// stage is second.
func TestTheScriptHoldsTheSixStagesTheStoreWrites(t *testing.T) {
	held, err := script(t).Stages()
	if err != nil {
		t.Fatalf("reading the stage order out of the script: %v", err)
	}
	if strings.Join(held, ",") != strings.Join(store.DesignStages(), ",") {
		t.Fatalf("the script holds %v and the store writes %v", held, store.DesignStages())
	}
}

// The five states, each from the numbers on the row. The one this step exists for is the fourth:
// approved at a version that is no longer the version the stage holds.
func TestEachStateIsDrawnFromTheVersionsOnTheRow(t *testing.T) {
	document := stagesJSON(t, "four bills, and two of them move",
		written(store.StageDiscovery, 2, 2),
		written(store.StageStories, 3, 2),
		written(store.StageDesignSystem, 1, 0),
		map[string]any{"stage": store.StageMockups, "body": "", "version": 0,
			"approved_version": 0, "skipped": true},
	)
	drawn, err := script(t).Menu(document)
	if err != nil {
		t.Fatalf("drawing the menu: %v", err)
	}

	for _, want := range []struct{ stage, state string }{
		{store.StageDiscovery, "approved"},
		{store.StageStories, "changed since approval"},
		{store.StageDesignSystem, "written"},
		{store.StageMockups, "skipped"},
		{store.StageDataModel, "not written"},
		{store.StageArchitecture, "not written"},
	} {
		markup := entryFor(t, drawn, want.stage)
		if state := stateOf(t, markup); state != want.state {
			t.Errorf("the %s stage reads %q, want %q:\n%s", want.stage, state, want.state, markup)
		}
	}
}

// One state and not two. An entry that carried its old word beside its new one would read as both
// approved and changed, and the operator would have to work out which of them is true.
func TestAnEntryCarriesOneStateAndNotTwo(t *testing.T) {
	drawn, err := script(t).Menu(stagesJSON(t, "", written(store.StageDiscovery, 3, 1)))
	if err != nil {
		t.Fatalf("drawing the menu: %v", err)
	}
	markup := entryFor(t, drawn, store.StageDiscovery)
	if held := stateText.FindAllStringSubmatch(markup, -1); len(held) != 1 {
		t.Fatalf("the discovery entry shows %d states, want one:\n%s", len(held), markup)
	}
}

// The body of a stage, shown as the text it was written as. A body carrying markup is text too: the
// page shows what the session wrote rather than running it.
func TestTheBodyOfAStageIsShownAsText(t *testing.T) {
	document := stagesJSON(t, "", map[string]any{
		"stage": store.StageDiscovery, "body": "<script>alert(1)</script> four bills",
		"version": 1, "approved_version": 1,
	})
	drawn, err := script(t).Body(document, store.StageDiscovery)
	if err != nil {
		t.Fatalf("drawing the body: %v", err)
	}
	if !strings.Contains(drawn, "&lt;script&gt;alert(1)&lt;/script&gt; four bills") {
		t.Errorf("the body reads %q, and a stage is shown as the text it was written as", drawn)
	}
	if strings.Contains(drawn, "<script>") {
		t.Errorf("the body carries a script element, so what a session wrote would run:\n%s", drawn)
	}
}

// entryFor is the markup of one menu entry, so an assertion about one stage cannot pass on another
// stage's words.
func entryFor(t *testing.T, drawn, stage string) string {
	t.Helper()
	mark := `data-entry="` + stage + `"`
	from := strings.Index(drawn, mark)
	if from < 0 {
		t.Fatalf("the menu carries no entry for the %s stage:\n%s", stage, drawn)
	}
	rest := drawn[from:]
	if to := strings.Index(rest, "</button>"); to >= 0 {
		return rest[:to]
	}
	t.Fatalf("the %s entry never ends, so the menu is not a list of entries:\n%s", stage, drawn)
	return ""
}

// stateOf is the word one entry shows, read from the words rather than from the attribute, because
// the words are the thing a person reads.
func stateOf(t *testing.T, markup string) string {
	t.Helper()
	found := stateText.FindStringSubmatch(markup)
	if found == nil {
		t.Fatalf("this entry shows no state at all:\n%s", markup)
	}
	return found[1]
}

// SITE-5, at the page: what the site rendered is what the operator reads. A page that drew the marks
// instead would show a heading as a hash and a list as a row of dashes.
func TestTheBodyOfAStageIsDrawnFromTheDocumentTheSiteRendered(t *testing.T) {
	document := stagesJSON(t, "", map[string]any{
		"stage":     store.StageDiscovery,
		"body":      "# Four bills\n\n- the rent moves\n",
		"body_html": "<h1>Four bills</h1>\n<ul>\n<li>the rent moves</li>\n</ul>\n",
		"version":   1, "approved_version": 1,
	})
	drawn, err := script(t).Body(document, store.StageDiscovery)
	if err != nil {
		t.Fatalf("drawing the body: %v", err)
	}
	if !strings.Contains(drawn, "<li>the rent moves</li>") {
		t.Errorf("the page draws no list, so the operator reads the marks:\n%s", drawn)
	}
	if strings.Contains(drawn, "# Four bills") {
		t.Errorf("the page draws the marks beside the document:\n%s", drawn)
	}
}

// The design is written as markdown too, so the operator reads it the same way.
func TestTheDesignIsDrawnFromTheDocumentTheSiteRendered(t *testing.T) {
	document, err := json.Marshal(map[string]any{
		"design": map[string]any{
			"brief":      "four bills, and two of them move",
			"brief_html": "<p>four bills, and two of them move</p>",
			"body":       "## The bills\n",
			"body_html":  "<h2>The bills</h2>",
		},
		"stages": []any{},
	})
	if err != nil {
		t.Fatalf("writing the stages document: %v", err)
	}
	drawn, err := script(t).Body(string(document), "design")
	if err != nil {
		t.Fatalf("drawing the design: %v", err)
	}
	for _, want := range []string{"<p>four bills, and two of them move</p>", "<h2>The bills</h2>"} {
		if !strings.Contains(drawn, want) {
			t.Errorf("the page draws no %s:\n%s", want, drawn)
		}
	}
}

// SITE-9: the design system stage draws the tokens it names. For a tokens artifact with n colours, f
// fonts and r radii, the view holds n swatches each carrying its value, f sample lines and r radius
// boxes, and no colour or font the artifact does not name.
//
// A design system is the one stage nobody approves by reading it. So these tests read the markup for
// the thing an operator looks at: the colour itself, the line set in the font, the corner the radius
// rounds and the room the space leaves, each beside its name and its value.

// tokensFile is the fixture these tests read: a tokens artifact as a session writes one.
const tokensFile = "tokens.json"

// drawnBy is the style property each group is drawn with. A colour reaches the page as the colour, a
// font as a line set in it, a radius as a rounded corner and a space as a bar of that width.
var drawnBy = map[string]string{
	"colour": "background",
	"font":   "font-family",
	"radius": "border-radius",
	"space":  "width",
}

// tokenBlock is one token as the page drew it: its group, its name, and everything drawn for it.
var tokenBlock = regexp.MustCompile(`(?s)<li class="token" data-group="([^"]*)" data-token="([^"]*)">(.*?)</li>`)

// tokenValue is the value one token shows a reader.
var tokenValue = regexp.MustCompile(`<code class="value">([^<]*)</code>`)

// drawnToken is one token of the view: what it shows, and the markup it was drawn with.
type drawnToken struct {
	value  string
	markup string
}

// tokensDrawn reads every token out of the markup, by group and by name.
func tokensDrawn(t *testing.T, drawn string) map[string]map[string]drawnToken {
	t.Helper()
	held := map[string]map[string]drawnToken{}
	for _, found := range tokenBlock.FindAllStringSubmatch(drawn, -1) {
		group, name, markup := found[1], found[2], found[3]
		if held[group] == nil {
			held[group] = map[string]drawnToken{}
		}
		value := ""
		if shown := tokenValue.FindStringSubmatch(markup); shown != nil {
			value = shown[1]
		}
		held[group][name] = drawnToken{value: value, markup: markup}
	}
	return held
}

// designSystem is a written and approved design system stage carrying one artifact.
func designSystem(artifact string) map[string]any {
	return map[string]any{
		"stage": store.StageDesignSystem, "body": "the design system body",
		"version": 1, "approved_version": 1, "artifact": artifact,
	}
}

// tokensFixture is the fixture, read twice: as the artifact a stage carries, and as the groups a test
// holds the page to.
func tokensFixture(t *testing.T) (string, map[string]map[string]string) {
	t.Helper()
	artifact, err := render.Fixture(tokensFile)
	if err != nil {
		t.Fatalf("reading the tokens fixture: %v", err)
	}
	var read struct {
		Tokens map[string]map[string]string `json:"tokens"`
	}
	if err := json.Unmarshal([]byte(artifact), &read); err != nil {
		t.Fatalf("the tokens fixture is not readable: %v", err)
	}
	if len(read.Tokens) != 4 {
		t.Fatalf("the fixture names %d groups, and a design system names colour, font, radius and space",
			len(read.Tokens))
	}
	return artifact, read.Tokens
}

// Every token the artifact names, on the page, with its value beside it and drawn as what it is. A
// page short of one colour is a page the operator approves a design system they never saw.
func TestTheDesignSystemDrawsEveryTokenItNames(t *testing.T) {
	artifact, groups := tokensFixture(t)
	drawn, err := script(t).Body(stagesJSON(t, "", designSystem(artifact)), store.StageDesignSystem)
	if err != nil {
		t.Fatalf("drawing the design system: %v", err)
	}
	held := tokensDrawn(t, drawn)

	for group, names := range groups {
		if len(held[group]) != len(names) {
			t.Errorf("the artifact names %d %s tokens and the page draws %d:\n%s",
				len(names), group, len(held[group]), drawn)
		}
		for name, value := range names {
			token, found := held[group][name]
			if !found {
				t.Errorf("the page draws no %s token called %q:\n%s", group, name, drawn)
				continue
			}
			if token.value != value {
				t.Errorf("the %s token %q shows %q, and the artifact names %q",
					group, name, token.value, value)
			}
			want := `style="` + drawnBy[group] + ":" + value + `"`
			if !strings.Contains(token.markup, want) {
				t.Errorf("the %s token %q is never drawn as %s: %s", group, name, want, token.markup)
			}
		}
	}
}

// A colour the design system does not name is a colour the operator never agreed to, so the page
// holds none of its own.
func TestTheDesignSystemDrawsNoTokenTheArtifactDoesNotName(t *testing.T) {
	artifact := `{"tokens": {"colour": {"ink": "#1d1c1a"}}}`
	drawn, err := script(t).Body(stagesJSON(t, "", designSystem(artifact)), store.StageDesignSystem)
	if err != nil {
		t.Fatalf("drawing the design system: %v", err)
	}
	held := tokensDrawn(t, drawn)

	if len(held["colour"]) != 1 {
		t.Errorf("the artifact names one colour and the page draws %d:\n%s", len(held["colour"]), drawn)
	}
	if _, found := held["colour"]["ink"]; !found {
		t.Errorf("the page draws no colour called ink:\n%s", drawn)
	}
	for _, group := range []string{"font", "radius", "space"} {
		if len(held[group]) != 0 {
			t.Errorf("the artifact names no %s and the page draws %d:\n%s", group, len(held[group]), drawn)
		}
	}
}

// A design system written in words names a colour a page cannot draw. The name and the value still
// reach the operator, because a token left out reads as a design system with one fewer colour.
func TestAColourThePageCannotDrawIsShownAsTextWithNoSwatch(t *testing.T) {
	artifact := `{"tokens": {"colour": {"ink": "the colour of a rainy pavement"}}}`
	drawn, err := script(t).Body(stagesJSON(t, "", designSystem(artifact)), store.StageDesignSystem)
	if err != nil {
		t.Fatalf("drawing the design system: %v", err)
	}
	token, found := tokensDrawn(t, drawn)["colour"]["ink"]
	if !found {
		t.Fatalf("the page draws no colour called ink, so the operator reads one colour fewer:\n%s", drawn)
	}
	if token.value != "the colour of a rainy pavement" {
		t.Errorf("the ink token shows %q, and the artifact names a sentence", token.value)
	}
	if strings.Contains(token.markup, "style=") {
		t.Errorf("a value this page cannot draw reached a style attribute: %s", token.markup)
	}
}

// A value written to close the style attribute it sits in. The whole element after it would be
// whatever the value says, so the value goes nowhere near a style attribute and reads as text.
func TestAValueThatClosesTheStyleNeverReachesAStyle(t *testing.T) {
	artifact := `{"tokens": {"colour": {"ink": "#fff\" onmouseover=\"alert(1)"}}}`
	drawn, err := script(t).Body(stagesJSON(t, "", designSystem(artifact)), store.StageDesignSystem)
	if err != nil {
		t.Fatalf("drawing the design system: %v", err)
	}
	if strings.Contains(drawn, "onmouseover") && !strings.Contains(drawn, "onmouseover=&quot;") {
		t.Errorf("the page carries an attribute the artifact wrote:\n%s", drawn)
	}
	token, found := tokensDrawn(t, drawn)["colour"]["ink"]
	if !found {
		t.Fatalf("the page draws no colour called ink:\n%s", drawn)
	}
	if strings.Contains(token.markup, "style=") {
		t.Errorf("a value carrying a quote reached a style attribute: %s", token.markup)
	}
	if !strings.Contains(token.markup, "&quot;") {
		t.Errorf("the value is shown with its quote unescaped: %s", token.markup)
	}
}

// One order, and sorted names inside it. Two reads of one design system agree, so an operator who
// approved a colour finds it in the same place the next day.
func TestTheGroupsAreDrawnInOneOrderAndTheirNamesSorted(t *testing.T) {
	artifact, groups := tokensFixture(t)
	drawn, err := script(t).Body(stagesJSON(t, "", designSystem(artifact)), store.StageDesignSystem)
	if err != nil {
		t.Fatalf("drawing the design system: %v", err)
	}

	var order, colours []string
	for _, found := range tokenBlock.FindAllStringSubmatch(drawn, -1) {
		if len(order) == 0 || order[len(order)-1] != found[1] {
			order = append(order, found[1])
		}
		if found[1] == "colour" {
			colours = append(colours, found[2])
		}
	}
	want := []string{"colour", "font", "radius", "space"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("the page draws the groups %v, want %v", order, want)
	}
	sorted := make([]string, 0, len(groups["colour"]))
	for name := range groups["colour"] {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	if strings.Join(colours, ",") != strings.Join(sorted, ",") {
		t.Errorf("the page draws the colours %v, want %v", colours, sorted)
	}
}

// The skill pins neither shape, so both read: a document with a tokens key, and the groups on their
// own. The page draws the same design system either way.
func TestTheTokensAreReadFromEitherArtifactShape(t *testing.T) {
	both := []string{
		`{"tokens": {"colour": {"ink": "#1d1c1a"}, "font": {"sans": "Inter, sans-serif"}}}`,
		`{"colour": {"ink": "#1d1c1a"}, "font": {"sans": "Inter, sans-serif"}}`,
	}
	for _, artifact := range both {
		drawn, err := script(t).Body(stagesJSON(t, "", designSystem(artifact)), store.StageDesignSystem)
		if err != nil {
			t.Fatalf("drawing the design system of %s: %v", artifact, err)
		}
		held := tokensDrawn(t, drawn)
		if held["colour"]["ink"].value != "#1d1c1a" {
			t.Errorf("the artifact %s draws the ink colour as %q", artifact, held["colour"]["ink"].value)
		}
		if held["font"]["sans"].value != "Inter, sans-serif" {
			t.Errorf("the artifact %s draws the sans font as %q", artifact, held["font"]["sans"].value)
		}
	}
}

// An artifact nothing can read leaves the stage as it reads today: its text, its state, and no tokens
// invented to fill the space. The readable artifact beside them says what the page does when it can
// read one, so this is a difference between two runs rather than a page that draws nothing at all.
func TestAnArtifactThatIsNotReadableLeavesTheStageAsItReads(t *testing.T) {
	readable, _ := tokensFixture(t)
	drawn, err := script(t).Body(stagesJSON(t, "", designSystem(readable)), store.StageDesignSystem)
	if err != nil {
		t.Fatalf("drawing the design system: %v", err)
	}
	if len(tokenBlock.FindAllStringSubmatch(drawn, -1)) == 0 {
		t.Fatalf("the fixture names tokens and the page draws none, so this test compares nothing:\n%s", drawn)
	}

	for _, artifact := range []string{"", "not json at all", `"a string"`, `{"tokens": {}}`} {
		drawn, err := script(t).Body(stagesJSON(t, "", designSystem(artifact)), store.StageDesignSystem)
		if err != nil {
			t.Fatalf("drawing the design system of %q: %v", artifact, err)
		}
		if found := tokenBlock.FindAllStringSubmatch(drawn, -1); len(found) != 0 {
			t.Errorf("the artifact %q names no token and the page draws %d:\n%s", artifact, len(found), drawn)
		}
		if !strings.Contains(drawn, "the design system body") {
			t.Errorf("the artifact %q took the stage's own text off the page:\n%s", artifact, drawn)
		}
	}
}

// Tokens belong to the design system. The same artifact on another stage draws that stage's own text
// and no token view, because a stage that drew somebody else's tokens says a thing about itself that
// is not true.
func TestOnlyTheDesignSystemStageDrawsTokens(t *testing.T) {
	artifact, _ := tokensFixture(t)
	system, err := script(t).Body(stagesJSON(t, "", designSystem(artifact)), store.StageDesignSystem)
	if err != nil {
		t.Fatalf("drawing the design system: %v", err)
	}
	if len(tokenBlock.FindAllStringSubmatch(system, -1)) == 0 {
		t.Fatalf("the design system draws no tokens, so this test compares nothing:\n%s", system)
	}

	document := stagesJSON(t, "", map[string]any{
		"stage": store.StageDiscovery, "body": "the discovery body",
		"version": 1, "approved_version": 1, "artifact": artifact,
	})
	drawn, err := script(t).Body(document, store.StageDiscovery)
	if err != nil {
		t.Fatalf("drawing the discovery stage: %v", err)
	}
	if found := tokenBlock.FindAllStringSubmatch(drawn, -1); len(found) != 0 {
		t.Errorf("the discovery stage draws %d tokens of its own:\n%s", len(found), drawn)
	}
}
