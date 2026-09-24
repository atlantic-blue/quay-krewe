// Package render runs the design site's own renderer outside a browser.
//
// The page draws its menu in JavaScript, in a block that touches no document. This package reads
// that block out of site.js, runs it over a stages.json, and answers with the markup an operator
// sees. A test then reads what was drawn rather than what the file says.
//
// It is a package of its own because it carries a JavaScript engine. The control plane serves the
// script and never runs a line of it, so the server must not link the engine: only a test imports
// this.
package render

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dop251/goja"
)

// The marks around the part of the script that draws. Everything between them answers markup and
// reads no document, so moving either mark breaks loudly here rather than quietly in a browser.
const (
	renderOpen  = "// site:render:start"
	renderClose = "// site:render:end"
)

// Dir is the site's directory, found by walking up to the module root. The tests that use this run
// from two different working directories, and a relative path only ever suits one of them.
func Dir() (string, error) {
	at, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(at, "go.mod")); err == nil {
			return filepath.Join(at, "internal", "controlplane", "site"), nil
		}
		up := filepath.Dir(at)
		if up == at {
			return "", fmt.Errorf("site: no go.mod above %s, so the site directory cannot be found", at)
		}
		at = up
	}
}

// A Script is site.js taken apart: the whole file, and the block that draws.
type Script struct {
	// JS is the whole file, as it is on disk and as the site serves it.
	JS string
	// Render is the block between the two marks, which draws and nothing else.
	Render string
}

// ReadScript reads site.js and takes it apart.
func ReadScript(path string) (Script, error) {
	body, err := os.ReadFile(path) //nolint:gosec // the path is the site's own file, named by a test
	if err != nil {
		return Script{}, fmt.Errorf("site: reading the script: %w", err)
	}
	js := string(body)
	block, err := between(js, renderOpen, renderClose)
	if err != nil {
		return Script{}, fmt.Errorf("site: the script has no %s block, so nothing can draw the menu", renderOpen)
	}
	return Script{JS: js, Render: block}, nil
}

// ReadSite reads the script the way a test wants it: from the site's own directory.
func ReadSite() (Script, error) {
	dir, err := Dir()
	if err != nil {
		return Script{}, err
	}
	return ReadScript(filepath.Join(dir, "site.js"))
}

// Menu draws the menu of one stages.json and answers with the markup, the way the page does.
func (s Script) Menu(stages string) (string, error) {
	return s.call(stages, `renderMenu(JSON.parse(stagesJSON))`)
}

// Body draws what one entry of the menu shows when it is chosen. The design is the entry called
// design, and every other entry is one of the six stages.
func (s Script) Body(stages, entry string) (string, error) {
	return s.call(stages, fmt.Sprintf(`renderBody(JSON.parse(stagesJSON), %q)`, entry))
}

// Stages is the order the script itself holds. A stage nobody wrote has no row in stages.json, so
// the script carries the six names, and a test holds that list against the one the store writes.
func (s Script) Stages() ([]string, error) {
	drawn, err := s.call("{}", `JSON.stringify(STAGE_ORDER)`)
	if err != nil {
		return nil, err
	}
	var order []string
	if err := json.Unmarshal([]byte(drawn), &order); err != nil {
		return nil, fmt.Errorf("site: the script's stage order reads %q: %w", drawn, err)
	}
	return order, nil
}

// call runs the render block and then one expression over it, with the document handed in as text so
// nothing here can quietly change what the page was asked to draw.
func (s Script) call(stages, expression string) (string, error) {
	vm := goja.New()
	if err := vm.Set("stagesJSON", stages); err != nil {
		return "", err
	}
	if _, err := vm.RunString(s.Render); err != nil {
		return "", fmt.Errorf("site: the render block did not run: %w", err)
	}
	drawn, err := vm.RunString(expression)
	if err != nil {
		return "", fmt.Errorf("site: running %s: %w", expression, err)
	}
	return drawn.String(), nil
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

// Fixture reads one document out of the site's own testdata directory.
//
// The path is found rather than written down, for the reason Dir is: a test of this package and a
// scenario in features run from two different working directories.
func Fixture(name string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	body, err := os.ReadFile(filepath.Join(dir, "testdata", name)) //nolint:gosec // the name is a fixture, named by a test
	if err != nil {
		return "", fmt.Errorf("site: reading the fixture %s: %w", name, err)
	}
	return string(body), nil
}
