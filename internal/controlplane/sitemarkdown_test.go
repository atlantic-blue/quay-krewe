package controlplane

import (
	"regexp"
	"strings"
	"testing"
)

// SITE-5: a body holding a heading, a list and a fenced code block comes back as a document, with a
// heading element, a list element and a pre holding the code.
//
// SITE-6: that html never carries a script element or an event attribute, whatever the body holds.
//
// Both are read off the html itself rather than off the library's settings, because a setting proves
// what was asked for and the html is what the operator's browser is handed.

// eventAttribute is an attribute a browser runs: on, a word, and an equals sign, inside a tag.
var eventAttribute = regexp.MustCompile(`(?i)<[^>]*\son[a-z]+\s*=`)

// aDocument is a body of the kind a design stage is written in.
const aDocument = "# Four bills\n\n" +
	"Two of them move every month.\n\n" +
	"- the rent moves\n" +
	"- the water moves\n\n" +
	"```go\nfmt.Println(\"the rent\")\n```\n"

func TestABodyReadsAsADocument(t *testing.T) {
	drawn, err := siteHTML(aDocument)
	if err != nil {
		t.Fatalf("rendering the body: %v", err)
	}

	for _, want := range []struct{ what, mark string }{
		{"heading", "<h1>Four bills</h1>"},
		{"list", "<ul>"},
		{"second line of the list", "<li>the water moves</li>"},
		{"code in a block of its own", "<pre>"},
	} {
		if !strings.Contains(drawn, want.mark) {
			t.Errorf("the document carries no %s: it reads\n%s", want.what, drawn)
		}
	}
	if !strings.Contains(drawn, "fmt.Println(&quot;the rent&quot;)") {
		t.Errorf("the code block lost the code it held: it reads\n%s", drawn)
	}
}

// A table is read as a table. It is the one part of this the library does not do on its own, so a
// body that lines its columns up in the terminal would otherwise read as one long paragraph.
func TestATableInABodyReadsAsATable(t *testing.T) {
	drawn, err := siteHTML("| bill | moves |\n| --- | --- |\n| rent | yes |\n")
	if err != nil {
		t.Fatalf("rendering the body: %v", err)
	}
	for _, want := range []string{"<table>", "<th>bill</th>", "<td>rent</td>"} {
		if !strings.Contains(drawn, want) {
			t.Errorf("the table carries no %s: it reads\n%s", want, drawn)
		}
	}
}

// The body a session writes is text the operator reads, and never a program the operator runs. Four
// ways in: an element, an attribute on an element, an attribute on an image that fails, and a link
// whose address is a program.
func TestAScriptWrittenIntoABodyNeverReachesThePage(t *testing.T) {
	drawn, err := siteHTML("# Four bills\n\n" +
		"<script>alert(1)</script>\n\n" +
		"<div onclick=\"alert(2)\">the rent</div>\n\n" +
		"<img src=\"x\" onerror=\"alert(3)\">\n\n" +
		"[the rent](javascript:alert(4))\n")
	if err != nil {
		t.Fatalf("rendering the body: %v", err)
	}

	if strings.Contains(strings.ToLower(drawn), "<script") {
		t.Errorf("the document carries a script element, so what a session wrote would run:\n%s", drawn)
	}
	if found := eventAttribute.FindString(drawn); found != "" {
		t.Errorf("the document carries the event attribute %q, so a click would run it:\n%s", found, drawn)
	}
	if strings.Contains(strings.ToLower(drawn), "javascript:") {
		t.Errorf("the document carries a link that runs a program:\n%s", drawn)
	}
	if !strings.Contains(drawn, "<h1>Four bills</h1>") {
		t.Errorf("the document lost the part that was not a script:\n%s", drawn)
	}
}

// The heading of a stage is written by a session too, so the same rule holds for the words inside an
// element as for the elements themselves.
func TestAScriptInsideASentenceNeverReachesThePage(t *testing.T) {
	drawn, err := siteHTML("The rent is <script>alert(1)</script> late.\n")
	if err != nil {
		t.Fatalf("rendering the body: %v", err)
	}
	if strings.Contains(strings.ToLower(drawn), "<script") {
		t.Errorf("the document carries a script element inside a sentence:\n%s", drawn)
	}
	if !strings.Contains(drawn, "The rent is") || !strings.Contains(drawn, "late.") {
		t.Errorf("the sentence around it is gone too, so the operator reads less than was written:\n%s", drawn)
	}
}

// A mermaid block is left for the page to draw, with its source as text inside it. The library
// writes every fenced block the same way, so this is the one shape the site asks for by name.
func TestAMermaidBlockIsLeftForThePageToDraw(t *testing.T) {
	drawn, err := siteHTML("```mermaid\nflowchart TD\n  A[Rent] --> B[Paid]\n```\n")
	if err != nil {
		t.Fatalf("rendering the body: %v", err)
	}
	if !strings.Contains(drawn, `<pre class="mermaid">`) {
		t.Errorf("the mermaid block is not left for the page to draw: it reads\n%s", drawn)
	}
	if strings.Contains(drawn, "language-mermaid") {
		t.Errorf("the mermaid block comes back as code rather than as a diagram to draw:\n%s", drawn)
	}
	if !strings.Contains(drawn, "flowchart TD") {
		t.Errorf("the mermaid block lost the diagram it held:\n%s", drawn)
	}
	if !strings.Contains(drawn, "A[Rent] --&gt; B[Paid]") {
		t.Errorf("the mermaid source is not held as text: it reads\n%s", drawn)
	}
}

// A block whose language is not one the site knows keeps its language, so a reader of the page can
// tell one kind of code from another.
func TestAFencedBlockKeepsTheLanguageItNames(t *testing.T) {
	drawn, err := siteHTML("```go\nfmt.Println(1)\n```\n")
	if err != nil {
		t.Fatalf("rendering the body: %v", err)
	}
	if !strings.Contains(drawn, `<code class="language-go">`) {
		t.Errorf("the block lost the language it names: it reads\n%s", drawn)
	}
}

// The way out of a mermaid block, which is the one element the site writes itself: source that spells
// the end of the element and then a script of its own.
func TestAMermaidBlockCannotEndItsOwnElement(t *testing.T) {
	drawn, err := siteHTML("```mermaid\n</pre><script>alert(1)</script>\n```\n")
	if err != nil {
		t.Fatalf("rendering the body: %v", err)
	}
	if strings.Contains(strings.ToLower(drawn), "<script") {
		t.Errorf("a mermaid block ended its own element and ran a script:\n%s", drawn)
	}
	if strings.Count(drawn, "</pre>") != 1 {
		t.Errorf("the mermaid element does not end once: it reads\n%s", drawn)
	}
}

// A stage nobody wrote reads as nothing, rather than as an empty paragraph the page then has to
// decide what to do with.
func TestABodyWithNothingInItReadsAsNothing(t *testing.T) {
	drawn, err := siteHTML("")
	if err != nil {
		t.Fatalf("rendering an empty body: %v", err)
	}
	if drawn != "" {
		t.Errorf("an empty body reads as %q, want nothing at all", drawn)
	}
}
