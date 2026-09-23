// Package flowmap runs the flow map page's own renderer outside a browser.
//
// The page draws a screen in JavaScript, in a script block that touches no document. This package
// reads that block out of index.html, runs it over a flows.json, and answers with the markup an
// operator sees. A test can then read what was drawn rather than what the file says.
//
// It is here rather than under internal/ because it belongs to the skill. The page and the harness
// that proves the page move together, and a reader of the skill directory finds both.
package flowmap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dop251/goja"
)

// The page's own marks. The render block is the part that draws a screen, and the style block is
// every rule that paints one. Both are read by name, so moving either one breaks loudly here
// rather than quietly in a browser.
const (
	renderOpen  = `<script id="flow-map-render">`
	scriptClose = `</script>`
	styleOpen   = "/* screen:start"
	styleClose  = "/* screen:end */"
)

// cssComment is a comment in the stylesheet. A rule is read for what it paints, and a comment that
// names a selector is prose rather than a rule.
var cssComment = regexp.MustCompile(`(?s)/\*.*?\*/`)

// Dir is the skill's directory, found by walking up to the module root. The tests that use this
// run from two different working directories, and a relative path only ever suits one of them.
func Dir() (string, error) {
	at, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(at, "go.mod")); err == nil {
			return filepath.Join(at, "skills", "flow-map"), nil
		}
		up := filepath.Dir(at)
		if up == at {
			return "", fmt.Errorf("flowmap: no go.mod above %s, so the skill directory cannot be found", at)
		}
		at = up
	}
}

// A Page is index.html taken apart: the renderer, the rules that paint a screen, and the rules
// that do not.
type Page struct {
	// HTML is the whole file, as it is on disk.
	HTML string
	// Render is the script block that draws a screen.
	Render string
	// ScreenStyles is every rule between the two marks, which is where a screen is painted.
	ScreenStyles string
	// OtherStyles is the rest of the stylesheet, with the comments taken out. It is every rule that
	// may not reach a screen.
	OtherStyles string
}

// ReadPage reads index.html and takes it apart.
func ReadPage(path string) (Page, error) {
	body, err := os.ReadFile(path) //nolint:gosec // the path is the skill's own file, named by a test
	if err != nil {
		return Page{}, fmt.Errorf("flowmap: reading the page: %w", err)
	}
	html := string(body)

	render, err := between(html, renderOpen, scriptClose)
	if err != nil {
		return Page{}, fmt.Errorf("flowmap: the page has no %s block, so nothing can draw a screen", renderOpen)
	}
	screen, err := between(html, styleOpen, styleClose)
	if err != nil {
		return Page{}, fmt.Errorf("flowmap: the page has no %s mark, so nothing says which rules paint a screen", styleOpen)
	}
	sheet, err := between(html, "<style>", "</style>")
	if err != nil {
		return Page{}, fmt.Errorf("flowmap: the page has no stylesheet")
	}
	return Page{
		HTML:         html,
		Render:       render,
		ScreenStyles: screen,
		OtherStyles:  cssComment.ReplaceAllString(strings.Replace(sheet, screen, "", 1), ""),
	}, nil
}

// Screen draws one screen of a flows.json and answers with the markup, the way the page does.
func (p Page) Screen(flows Flows, id string) (string, error) {
	vm := goja.New()
	if err := vm.Set("flowsJSON", string(flows.Raw)); err != nil {
		return "", err
	}
	if err := vm.Set("screenID", id); err != nil {
		return "", err
	}
	if _, err := vm.RunString(p.Render); err != nil {
		return "", fmt.Errorf("flowmap: the render block did not run: %w", err)
	}
	drawn, err := vm.RunString(`(function () {
		var d = JSON.parse(flowsJSON);
		var s = d.screens[screenID];
		if (!s) { throw new Error("no screen called " + screenID); }
		return renderScreen(s, d.tokens);
	})()`)
	if err != nil {
		return "", fmt.Errorf("flowmap: drawing %s: %w", id, err)
	}
	return drawn.String(), nil
}

// Flows is a flows.json, with the parts a test reads named and the bytes kept whole. The page is
// given the bytes, so nothing here can quietly change what it was asked to draw.
type Flows struct {
	Raw     []byte
	Project string                       `json:"project"`
	Tokens  map[string]map[string]string `json:"tokens"`
	Screens map[string]Screen            `json:"screens"`
	Stories []Story                      `json:"stories"`
}

// A Screen is one thing a person looks at.
type Screen struct {
	Name    string `json:"name"`
	Surface string `json:"surface"`
	Status  string `json:"status"`
	Route   string `json:"route"`
}

// A Story is a walk over the screens.
type Story struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Start string `json:"start"`
}

// ReadFlows reads a flows.json.
func ReadFlows(path string) (Flows, error) {
	body, err := os.ReadFile(path) //nolint:gosec // the path is a fixture, named by a test
	if err != nil {
		return Flows{}, fmt.Errorf("flowmap: reading the flows: %w", err)
	}
	var read Flows
	if err := json.Unmarshal(body, &read); err != nil {
		return Flows{}, fmt.Errorf("flowmap: %s is not readable: %w", path, err)
	}
	read.Raw = body
	return read, nil
}

// StoryCalled finds a story by its title, which is the words an operator reads in the listing.
func (f Flows) StoryCalled(title string) (Story, bool) {
	for _, story := range f.Stories {
		if story.Title == title {
			return story, true
		}
	}
	return Story{}, false
}

// TokenValues is every value the tokens name, whatever group it is in.
func (f Flows) TokenValues() []string {
	var out []string
	for _, set := range f.Tokens {
		for _, value := range set {
			out = append(out, value)
		}
	}
	return out
}

func between(text, open, ending string) (string, error) {
	from := strings.Index(text, open)
	if from < 0 {
		return "", fmt.Errorf("no %q", open)
	}
	rest := text[from+len(open):]
	to := strings.Index(rest, ending)
	if to < 0 {
		return "", fmt.Errorf("no %q after %q", ending, open)
	}
	return rest[:to], nil
}
