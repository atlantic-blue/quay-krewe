// Package render runs the flow map page's own renderer outside a browser.
//
// The page draws a screen in JavaScript, in a script block that touches no document. This package
// reads that block out of index.html, runs it over a flows.json, and answers with the markup an
// operator sees. A test can then read what was drawn rather than what the file says.
//
// It is here rather than under internal/ because it belongs to the skill. The page and the harness
// that proves the page move together, and a reader of the skill directory finds both.
//
// It sits under the skill rather than beside schema.json because it carries a JavaScript engine.
// The control plane reads the schema and must not link the engine: the engine costs the server
// 7.8 MB and the server never runs a line of JavaScript.
package render

import (
	"encoding/json"
	"fmt"
	"html"
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

// Screen draws one screen of a flows.json and answers with the markup, the way the page does. The
// design system goes in beside the flows because the values a screen is drawn in live there.
func (p Page) Screen(flows Flows, system System, id string) (string, error) {
	vm := goja.New()
	if err := vm.Set("flowsJSON", string(flows.Raw)); err != nil {
		return "", err
	}
	if err := vm.Set("systemJSON", system.Body()); err != nil {
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
		return renderScreen(s, JSON.parse(systemJSON));
	})()`)
	if err != nil {
		return "", fmt.Errorf("flowmap: drawing %s: %w", id, err)
	}
	return drawn.String(), nil
}

// System is a design-system.json: the values every screen of the project is drawn in, the files it
// draws with, and the stylesheet every screen is given. The page reads it beside the flows, because
// the design system stage is the one the operator approved first.
type System struct {
	Raw    []byte
	Tokens map[string]map[string]string `json:"tokens"`
	Assets map[string]Asset             `json:"assets"`
	CSS    string                       `json:"css"`
}

// An Asset is a file the project draws with: a font file or an image. It travels inside the stage
// as base64, and it reaches a screen as a data address, because a screen reaches no address off the
// page.
type Asset struct {
	Kind   string `json:"kind"`
	Media  string `json:"media"`
	Data   string `json:"data"`
	Family string `json:"family"`
	Weight string `json:"weight"`
	Style  string `json:"style"`
}

// ReadSystem reads a design-system.json.
func ReadSystem(path string) (System, error) {
	body, err := os.ReadFile(path) //nolint:gosec // the path is a fixture, named by a test
	if err != nil {
		return System{}, fmt.Errorf("flowmap: reading the design system: %w", err)
	}
	var read System
	if err := json.Unmarshal(body, &read); err != nil {
		return System{}, fmt.Errorf("flowmap: %s is not readable: %w", path, err)
	}
	read.Raw = body
	return read, nil
}

// Body is the design system as the page is given it. A project with none is given null, which is
// what the site answers as a 404 and what the page draws with anyway.
func (s System) Body() string {
	if len(s.Raw) == 0 {
		return "null"
	}
	return string(s.Raw)
}

// TokenValues is every value the tokens name, whatever group it is in.
func (s System) TokenValues() []string {
	var out []string
	for _, set := range s.Tokens {
		for _, value := range set {
			out = append(out, value)
		}
	}
	return out
}

// DocumentIn reads the document a frame was given out of the markup. The page writes it into the
// srcdoc attribute, escaped, so a test that reads what an operator sees has to unescape it the way
// a browser does.
func DocumentIn(markup string) (string, bool) {
	const open = ` srcdoc="`
	from := strings.Index(markup, open)
	if from < 0 {
		return "", false
	}
	rest := markup[from+len(open):]
	to := strings.Index(rest, `"`)
	if to < 0 {
		return "", false
	}
	return html.UnescapeString(rest[:to]), true
}

// FontFacesIn is every @font-face rule the document carries, each one as the text inside its
// braces. A screen reaches a font file through one of these, so a test reads them rather than
// looking for the words anywhere in the document.
func FontFacesIn(document string) []string {
	var out []string
	rest := document
	for {
		from := strings.Index(rest, "@font-face")
		if from < 0 {
			return out
		}
		rest = rest[from+len("@font-face"):]
		open := strings.Index(rest, "{")
		shut := strings.Index(rest, "}")
		if open < 0 || shut < open {
			return out
		}
		out = append(out, rest[open+1:shut])
		rest = rest[shut+1:]
	}
}

// AddressesIn is every address the document draws with: what each url() names, and what each src
// attribute names. A screen is only as contained as the addresses in it, so a test reads all of
// them rather than the ones it thought of.
func AddressesIn(document string) []string {
	var out []string
	for _, found := range anAddressInCSS.FindAllStringSubmatch(document, -1) {
		out = append(out, strings.Trim(strings.TrimSpace(found[1]), `"'`))
	}
	for _, found := range aSourceAttribute.FindAllStringSubmatch(document, -1) {
		out = append(out, strings.Trim(strings.TrimSpace(found[1]), `"'`))
	}
	return out
}

// An address in a rule, and an address in the markup, whatever quoting each was written with.
var (
	anAddressInCSS   = regexp.MustCompile(`(?i)url\(\s*([^)]*?)\s*\)`)
	aSourceAttribute = regexp.MustCompile(`(?i)\ssrc\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
)

// FontFaceFor finds the rule that carries one font file, and says what was missing when no rule
// carries it. Every part is read, because a rule with the right family and the wrong file draws
// nothing and a rule with no weight draws the wrong face.
func FontFaceFor(document string, asset Asset) (string, string) {
	want := []string{
		`font-family:"` + asset.Family + `"`,
		"src:url(" + DataAddressOf(asset) + ")",
		"font-weight:" + asset.Weight,
		"font-style:" + asset.Style,
	}
	faces := FontFacesIn(document)
	if len(faces) == 0 {
		return "", "the document carries no @font-face rule at all"
	}
	for _, face := range faces {
		flat := strings.Join(strings.Fields(face), "")
		missing := ""
		for _, part := range want {
			if !strings.Contains(flat, part) {
				missing = part
				break
			}
		}
		if missing == "" {
			return face, ""
		}
	}
	return "", fmt.Sprintf("%d rules were read and none holds all of %v", len(faces), want)
}

// DataAddressOf is the address a screen reaches an asset at. It is the whole file, inside the
// document, because a screen reaches no address off the page.
func DataAddressOf(asset Asset) string {
	return "data:" + asset.Media + ";base64," + asset.Data
}

// AssetsByKind splits the assets of a design system into the font files and the images.
func AssetsByKind(system System) (map[string]Asset, map[string]Asset) {
	fonts, images := map[string]Asset{}, map[string]Asset{}
	for name, asset := range system.Assets {
		switch asset.Kind {
		case "font":
			fonts[name] = asset
		case "image":
			images[name] = asset
		}
	}
	return fonts, images
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

// A Screen is one thing a person looks at. HTML is the markup of its body, which a session writes
// and the page composes into a document of its own.
type Screen struct {
	Name     string   `json:"name"`
	Surface  string   `json:"surface"`
	Status   string   `json:"status"`
	Route    string   `json:"route"`
	HTML     string   `json:"html"`
	Viewport Viewport `json:"viewport"`
}

// A Viewport is the size a screen is written at. A screen that names none is drawn at the size of
// its surface.
type Viewport struct {
	Width  int `json:"width"`
	Height int `json:"height"`
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
