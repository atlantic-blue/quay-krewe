package controlplane

import (
	"bytes"
	"fmt"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// A design stage is written as markdown, and the site hands the page html.
//
// The rendering is the safety as well as the reading. The library is left at its default, which does
// not pass raw html through: an element a session wrote arrives as a comment saying it was left out,
// and a link whose address is a program arrives with no address at all. So a body is a document the
// operator reads and never a program the operator's browser runs, and nothing here has to recognise
// the ways of writing one.
//
// A table is the one addition. A body that lines its columns up reads as one long paragraph without
// it, and a design stage is where a table belongs. Fenced code needs no addition: it is part of the
// markdown the library already reads.

// mermaidLanguage is the word a session writes above a diagram.
const mermaidLanguage = "mermaid"

// mermaidClass is what the page looks for when it draws the diagrams. The library writes every fenced
// block as code, so this shape is written here rather than taken.
const mermaidClass = `<pre class="mermaid">`

// site renders a stage body once, and is reused: the library is built to be built once and read from
// many goroutines.
var site = goldmark.New(
	goldmark.WithExtensions(extension.Table),
	goldmark.WithRendererOptions(
		renderer.WithNodeRenderers(util.Prioritized(fencedBlocks{}, 100)),
	),
)

// siteHTML is one markdown body as the html a page shows. A body with nothing in it renders as
// nothing, so the page can tell a stage that was written from one that was not.
func siteHTML(body string) (string, error) {
	if body == "" {
		return "", nil
	}
	var drawn bytes.Buffer
	if err := site.Convert([]byte(body), &drawn); err != nil {
		return "", fmt.Errorf("rendering a body: %w", err)
	}
	return drawn.String(), nil
}

// fencedBlocks draws a fenced block, and it exists for one of them: a diagram.
//
// The library writes a fenced block as a pre holding a code element, and the drawing library the page
// loads looks for a pre carrying the class instead. Every other block is written the way the library
// writes it, so a block that names a language keeps its language and a reader can tell one kind of
// code from another.
type fencedBlocks struct{}

func (fencedBlocks) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, renderFencedBlock)
}

// renderFencedBlock writes one block, opening it on the way in and closing it on the way out.
//
// The source is written escaped, which is what holds the element closed: a diagram whose text spells
// the end of a pre and then a script of its own is text inside the element rather than the end of it.
func renderFencedBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	block, held := node.(*ast.FencedCodeBlock)
	if !held {
		return ast.WalkContinue, nil
	}
	diagram := string(block.Language(source)) == mermaidLanguage

	if !entering {
		if diagram {
			_, _ = w.WriteString("</pre>\n")
			return ast.WalkContinue, nil
		}
		_, _ = w.WriteString("</code></pre>\n")
		return ast.WalkContinue, nil
	}

	switch language := block.Language(source); {
	case diagram:
		_, _ = w.WriteString(mermaidClass)
	case len(language) > 0:
		_, _ = w.WriteString(`<pre><code class="language-`)
		_, _ = w.Write(util.EscapeHTML(language))
		_, _ = w.WriteString(`">`)
	default:
		_, _ = w.WriteString("<pre><code>")
	}

	lines := block.Lines()
	for at := 0; at < lines.Len(); at++ {
		line := lines.At(at)
		_, _ = w.Write(util.EscapeHTML(line.Value(source)))
	}
	return ast.WalkContinue, nil
}
