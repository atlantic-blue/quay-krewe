package render_test

import (
	"encoding/json"
	"fmt"
	"regexp"
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
