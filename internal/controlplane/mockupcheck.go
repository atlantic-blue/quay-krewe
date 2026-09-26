package controlplane

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/atlantic-blue/quay-krewe/internal/store"
	flowmap "github.com/atlantic-blue/quay-krewe/skills/flow-map"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// What the mockups stage is held to, beyond being json.
//
// A mockup is a page a person plays, and then a session builds the screens on it. The session has
// to read which library component each shape stands for. A shape that names none is a picture
// nobody can build from, and the fault is found at the end, by the person who asked for the
// screen. So the artifact is read here, before the store keeps it.
//
// Two rules. Every shape names a component, which is the schema's own rule and is reported here
// with the screen and the shape in the message. And every colour and font a screen is drawn in is
// one the approved design_system stage names, because the design system is the stage the operator
// agreed first and a mockup that leaves it is a second design system nobody approved.
//
// Nothing here runs for the other five stages. They carry whatever json they carry.

// mockupSchema is skills/flow-map/schema.json, compiled once.
//
// The schema is the file the skill tells a session to write against, so the writer and the reader
// are held to one text. It is compiled lazily because a control plane that never takes a mockup
// should not pay for it, and once because compiling is the expensive half.
var mockupSchema = sync.OnceValues(compileMockupSchema)

func compileMockupSchema() (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(flowmap.SchemaJSON))
	if err != nil {
		return nil, fmt.Errorf("the flow map schema is not json: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(flowmap.SchemaID, doc); err != nil {
		return nil, fmt.Errorf("the flow map schema will not load: %w", err)
	}
	return compiler.Compile(flowmap.SchemaID)
}

// styleKeys are the fields a colour or a font is written into on a screen.
//
// The list is named rather than every string being searched for something that looks like a
// colour. A screen says what a person reads, and prose carrying "#abc" is prose. A gate that
// refused it would be refusing words, which is worse than missing a colour nobody wrote in a
// field meant for one.
var styleKeys = map[string]bool{
	"colour": true, "color": true, "background": true, "fill": true, "style": true,
	"font": true, "fontFamily": true, "font-family": true,
}

// fontKeys are the fields whose whole value is a font.
var fontKeys = map[string]bool{"font": true, "fontFamily": true, "font-family": true}

// checkMockupArtifact reads a mockups artifact and refuses what a session could not build from.
//
// It answers with the warnings the write should carry and, when the artifact is refused, the
// refusal. An artifact that is not json is not this check's to report: the store refuses it, with
// one message, and a second message about the same fault sends the reader two ways.
func (s *Server) checkMockupArtifact(ctx context.Context, project, artifact string) ([]string, error) {
	if strings.TrimSpace(artifact) == "" {
		return nil, nil
	}
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(artifact))
	if err != nil {
		return nil, nil
	}

	// The shape check first, then the schema. Both refuse a shape with no component, and the one
	// that names the screen is the one worth reading. The schema is what catches everything else.
	if screen, at, kind, found := aShapeWithNoComponent(doc); found {
		return nil, status.Errorf(codes.InvalidArgument,
			"the mockups artifact is refused: the %q screen has a shape at el %d%s that names no component. "+
				"Every shape names the component it stands for, for example Button, so a session building "+
				"the screen reads which component goes where.",
			screen, at, ofKind(kind))
	}

	// The markup rule, beside the shape rule, because both shapes of screen are written while the
	// format changes over. A part that names no component is a part the building session has to
	// guess at, whichever way the screen was written.
	if screen, part, found := aPartWithNoComponent(doc); found {
		if part.pressed {
			return nil, status.Errorf(codes.InvalidArgument,
				"the mockups artifact is refused: the %q screen has a part that can be pressed and "+
					"names no component: %s. The press is the component, so write "+
					"data-component=\"Button\" on that part.",
				screen, part.describe())
		}
		return nil, status.Errorf(codes.InvalidArgument,
			"the mockups artifact is refused: the %q screen holds a visible part outside every named "+
				"component: %s. Every visible part sits under an element naming the component it "+
				"stands for, so write data-component=\"Button\" on that part or on a part above it.",
			screen, part.describe())
	}

	// The containment rule, after the two component rules. A part nobody can build from is the fault
	// that reaches furthest, because it survives the operator's approval and lands on the session that
	// builds the screen, so a screen carrying both faults reads that one first.
	if screen, fault, found := aScreenThatReachesOut(doc); found {
		return nil, refusalFor(screen, fault)
	}

	schema, err := mockupSchema()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "the flow map schema could not be read: %v", err)
	}
	if err := schema.Validate(doc); err != nil {
		return nil, status.Errorf(codes.InvalidArgument,
			"the mockups artifact does not match the flow map schema: %s. "+
				"skills/flow-map/schema.json holds every field and every allowed value.",
			whatTheSchemaSaid(err))
	}

	named, err := s.approvedDesignSystemTokens(ctx, project)
	if err != nil {
		return nil, err
	}
	// A design system that names nothing used to leave the mockup kept and carrying a warning, and a
	// warning holds nothing: the screens of that project were then drawn in whatever the session
	// chose. So the write is refused, and the refusal names work somebody can do, because the stage
	// has a shape of its own to be written against.
	if named.empty() {
		return nil, status.Errorf(codes.InvalidArgument,
			"the mockups artifact is refused: the approved design_system stage names no colour and no font, "+
				"so nothing holds the screens of this mockup to anything. Write the tokens of that stage "+
				"first, with krewe stage set [<address>] design_system --artifact <file>, and approve it.")
	}
	return nil, unnamedValue(doc, named)
}

// ofKind names the shape in the refusal when the shape said what kind it is. A shape carrying
// neither a kind nor a component gets its position and nothing else, which is still enough to
// find it.
func ofKind(kind string) string {
	if kind == "" {
		return ""
	}
	return fmt.Sprintf(", of kind %q", kind)
}

// aShapeWithNoComponent finds the first shape that names no component, reading the screens in name
// order so two reads of one artifact name the same shape.
//
// It navigates rather than unmarshalling into a type, because an artifact whose screens are not
// screens at all is the schema's to refuse and this walk must find nothing rather than fail.
func aShapeWithNoComponent(doc any) (screen string, at int, kind string, found bool) {
	screens, ok := asObject(doc)["screens"]
	if !ok {
		return "", 0, "", false
	}
	held := asObject(screens)
	for _, name := range sortedKeys(held) {
		elements, ok := asArray(asObject(held[name])["el"])
		if !ok {
			continue
		}
		for at, element := range elements {
			shape, ok := element.(map[string]any)
			if !ok {
				continue
			}
			if strings.TrimSpace(asString(shape["component"])) == "" {
				return name, at, asString(shape["t"]), true
			}
		}
	}
	return "", 0, "", false
}

// The two rules a screen written as markup is held to.
//
// A screen may carry its markup instead of a shape list, and then there is no shape to hold a
// component name. The name moves onto the markup as data-component, and the same contract holds: a
// part that can be pressed names the component it stands for, and every visible part sits under one.
// So a card names itself once and the words inside it need no name of their own.

const (
	// theComponentAttribute is how a part names the component it stands for.
	theComponentAttribute = "data-component"
	// thePressAttribute is how a part opens another screen. The press is the component, so a part
	// carrying it names one on itself rather than under a card three levels up.
	thePressAttribute = "data-to"
	// theWordsShown caps how much of a part's own text a refusal repeats. The words are there to
	// find the part in a file, and a paragraph of them buries the sentence that says what to do.
	theWordsShown = 40
)

// notVisible are the elements whose text the browser reads and nobody else does. A rule that called
// their text words would refuse every screen carrying a stylesheet, which is most of them.
var notVisible = map[string]bool{
	"style": true, "script": true, "title": true, "template": true,
	"head": true, "meta": true, "link": true,
}

// drawnWithNoText are the elements that are a part of the screen whatever they hold. A mark carries
// no words and is still something a session has to build, and the markup inside a drawing is the
// drawing rather than parts of its own.
var drawnWithNoText = map[string]bool{"img": true, "svg": true, "canvas": true, "video": true}

// screenPart is one part of a screen, named the way a person would find it: the tag, the class if it
// has one, and the first words it holds.
type screenPart struct {
	tag   string
	class string
	words string
	// pressed says which rule the part broke, because the two refusals say different things to do.
	pressed bool
}

// describe names the part in a refusal, in the shape it was written in.
func (p screenPart) describe() string {
	said := "<" + p.tag + ">"
	if p.class != "" {
		said = fmt.Sprintf("<%s class=%q>", p.tag, p.class)
	}
	if p.words != "" {
		said += fmt.Sprintf(", holding %q", p.words)
	}
	return said
}

// aPartWithNoComponent finds the first part of a screen written as markup that names no component,
// reading the screens in name order so two reads of one artifact name the same part.
//
// It navigates rather than unmarshalling into a type, and markup it cannot parse is no fault of
// this walk: an artifact whose screens are not screens is the schema's to refuse.
func aPartWithNoComponent(doc any) (screen string, part screenPart, found bool) {
	held := asObject(asObject(doc)["screens"])
	for _, name := range sortedKeys(held) {
		markup := asString(asObject(held[name])["html"])
		if strings.TrimSpace(markup) == "" {
			continue
		}
		body := &html.Node{Type: html.ElementNode, DataAtom: atom.Body, Data: "body"}
		nodes, err := html.ParseFragment(strings.NewReader(markup), body)
		if err != nil {
			continue
		}
		for _, node := range nodes {
			if part, found := aPartUnder(node, false); found {
				return name, part, true
			}
		}
	}
	return "", screenPart{}, false
}

// aPartUnder reads one element and everything under it. named says whether anything above this
// element already names a component, which is what lets a card name itself once.
func aPartUnder(node *html.Node, named bool) (screenPart, bool) {
	if node.Type != html.ElementNode {
		return screenPart{}, false
	}
	names := strings.TrimSpace(attributeOf(node, theComponentAttribute)) != ""
	if strings.TrimSpace(attributeOf(node, thePressAttribute)) != "" && !names {
		return partOf(node, true), true
	}
	if notVisible[node.Data] {
		return screenPart{}, false
	}
	if drawnWithNoText[node.Data] {
		if named || names {
			return screenPart{}, false
		}
		return partOf(node, false), true
	}
	if isTheLastPart(node) && !named && !names {
		return partOf(node, false), true
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if part, found := aPartUnder(child, named || names); found {
			return part, true
		}
	}
	return screenPart{}, false
}

// isTheLastPart says whether an element is a visible leaf: it holds no element of its own, and it
// holds text that is not only spacing. Spacing is not words, so a part holding a space, or the
// space that does not break, holds nothing a person reads.
func isTheLastPart(node *html.Node) bool {
	words := false
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case html.ElementNode:
			return false
		case html.TextNode:
			if strings.TrimSpace(child.Data) != "" {
				words = true
			}
		}
	}
	return words
}

// partOf reads the part a refusal names out of the element it was found on.
func partOf(node *html.Node, pressed bool) screenPart {
	return screenPart{
		tag:     node.Data,
		class:   strings.TrimSpace(attributeOf(node, "class")),
		words:   theFirstWordsOf(node),
		pressed: pressed,
	}
}

// theFirstWordsOf is the text of one element, with its spacing collapsed and its length capped.
func theFirstWordsOf(node *html.Node) string {
	var said []string
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode {
			said = append(said, child.Data)
		}
	}
	words := strings.Join(strings.Fields(strings.Join(said, " ")), " ")
	if utf8.RuneCountInString(words) <= theWordsShown {
		return words
	}
	cut := string([]rune(words)[:theWordsShown])
	if at := strings.LastIndex(cut, " "); at > 0 {
		cut = cut[:at]
	}
	return cut
}

// attributeOf is one attribute of an element, or nothing where it carries none of that name.
func attributeOf(node *html.Node, name string) string {
	for _, held := range node.Attr {
		if held.Key == name {
			return held.Val
		}
	}
	return ""
}

// unnamedValue refuses the first colour or font the design system does not name.
//
// The tokens come first because they are what every screen is drawn in, so one wrong token is
// wrong on every screen and naming a screen for it would send the reader to the wrong place. A
// value written onto one screen is reported against that screen.
func unnamedValue(doc any, named designTokens) error {
	flows := asObject(doc)
	tokens := asObject(flows["tokens"])
	for _, group := range []struct {
		name  string
		held  map[string]bool
		kind  string
		empty bool
	}{
		{name: "colour", held: named.colour, kind: "colour", empty: len(named.colour) == 0},
		{name: "font", held: named.font, kind: "font", empty: len(named.font) == 0},
	} {
		if group.empty {
			continue
		}
		values := asObject(tokens[group.name])
		for _, token := range sortedKeys(values) {
			value := asString(values[token])
			if value == "" || group.held[normalise(value)] {
				continue
			}
			return status.Errorf(codes.InvalidArgument,
				"the mockups artifact is drawn in %s %q, under the token %q, and the approved "+
					"design_system stage does not name it: take every colour and font from that stage.",
				group.kind, value, token)
		}
	}

	screens := asObject(flows["screens"])
	for _, name := range sortedKeys(screens) {
		if kind, value, found := aValueOutside(screens[name], named); found {
			return status.Errorf(codes.InvalidArgument,
				"the %q screen of the mockups artifact uses %s %q, and the approved design_system "+
					"stage does not name it: take every colour and font from that stage.",
				name, kind, value)
		}
		if painted, found := aValuePaintedOutside(asString(asObject(screens[name])["html"]), named); found {
			return refusalForThePaint(name, painted)
		}
	}
	return nil
}

// aValueOutside walks one screen and answers the first colour or font the design system does not
// name. It reads the fields a style is written into, and no others.
func aValueOutside(screen any, named designTokens) (kind, value string, found bool) {
	switch held := screen.(type) {
	case map[string]any:
		for _, key := range sortedKeys(held) {
			if styleKeys[key] {
				if kind, value, found := unnamedIn(key, asString(held[key]), named); found {
					return kind, value, true
				}
			}
			if kind, value, found := aValueOutside(held[key], named); found {
				return kind, value, true
			}
		}
	case []any:
		for _, one := range held {
			if kind, value, found := aValueOutside(one, named); found {
				return kind, value, true
			}
		}
	}
	return "", "", false
}

// unnamedIn reads one style field. A font field is a font whole, and everything else is read for
// the colours written into it, because a style field holds a whole declaration.
func unnamedIn(key, text string, named designTokens) (kind, value string, found bool) {
	if text == "" {
		return "", "", false
	}
	if fontKeys[key] {
		if len(named.font) > 0 && !named.font[normalise(text)] {
			return "font", text, true
		}
		return "", "", false
	}
	if len(named.colour) == 0 {
		return "", "", false
	}
	for _, colour := range aColourLiteral.FindAllString(text, -1) {
		if !named.colour[normalise(colour)] {
			return "colour", colour, true
		}
	}
	return "", "", false
}

// designTokens is every colour and font value the approved design_system stage names, normalised
// so a comparison does not turn on a capital letter or a space.
type designTokens struct {
	colour map[string]bool
	font   map[string]bool
	// family is each font on its own, read out of the font values. A font token names a list, such as
	// "Inter, system-ui, sans-serif", and a screen may reach for one family of that list.
	family map[string]bool
}

func (d designTokens) empty() bool { return len(d.colour) == 0 && len(d.font) == 0 }

// approvedDesignSystemTokens reads the tokens out of the approved design_system stage.
//
// The stage is approved by the time a mockup may be written, because the order the stages are
// written in refuses a mockup otherwise. What it carries is another matter: a design system
// written as prose alone names nothing, and then this check has nothing to compare against and
// says so rather than refusing every mockup.
//
// Two shapes are read. The artifact may be a tokens document, the way the flow map writes one, or
// it may be the groups themselves. Neither is wrong and the skill pins neither.
func (s *Server) approvedDesignSystemTokens(ctx context.Context, project string) (designTokens, error) {
	held, err := s.store.ListDesignStages(ctx, project)
	if err != nil {
		return designTokens{}, storeError(err, "project")
	}
	for _, stage := range held {
		if stage.GetStage() != store.StageDesignSystem || !stage.GetApproved() {
			continue
		}
		doc, err := jsonschema.UnmarshalJSON(strings.NewReader(stage.GetArtifact()))
		if err != nil {
			return designTokens{}, nil
		}
		groups := asObject(doc)
		if tokens, ok := groups["tokens"]; ok {
			groups = asObject(tokens)
		}
		fonts := asObject(groups["font"])
		return designTokens{
			colour: valueSet(asObject(groups["colour"])),
			font:   valueSet(fonts),
			family: familySet(fonts),
		}, nil
	}
	return designTokens{}, nil
}

// valueSet is the values of one token group, normalised, ready to be asked about.
func valueSet(group map[string]any) map[string]bool {
	out := make(map[string]bool, len(group))
	for _, value := range group {
		if text := asString(value); text != "" {
			out[normalise(text)] = true
		}
	}
	return out
}

// familySet is each font a token group names, one family at a time. It is read the way the design
// system reads a font asset's family, so the two checks agree about what a token names.
func familySet(group map[string]any) map[string]bool {
	out := map[string]bool{}
	for _, value := range group {
		for _, one := range strings.Split(asString(value), ",") {
			if family := normalise(strings.Trim(strings.TrimSpace(one), `"'`)); family != "" {
				out[family] = true
			}
		}
	}
	return out
}

// whatTheSchemaSaid turns a validation failure into one line.
//
// The library's own message is a tree over several lines, which reads as a stack trace in a
// refusal. The leaves are the actual faults, so they are the part that is kept, and the first few
// of them are enough for somebody to go and fix the file.
func whatTheSchemaSaid(err error) string {
	failure, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return err.Error()
	}
	const most = 3
	var said []string
	for _, leaf := range leaves(failure.BasicOutput()) {
		said = append(said, leaf)
		if len(said) == most {
			break
		}
	}
	if len(said) == 0 {
		return strings.Join(strings.Fields(failure.Error()), " ")
	}
	return strings.Join(said, "; ")
}

// leaves is every unit of a validation output that carries a fault and holds no unit under it.
func leaves(unit *jsonschema.OutputUnit) []string {
	if unit == nil {
		return nil
	}
	var out []string
	for at := range unit.Errors {
		out = append(out, leaves(&unit.Errors[at])...)
	}
	if len(out) > 0 {
		return out
	}
	if unit.Error == nil {
		return nil
	}
	where := unit.InstanceLocation
	if where == "" {
		where = "the whole file"
	}
	return []string{fmt.Sprintf("at %s: %s", where, unit.Error)}
}

// The three readings a walk over an unknown document needs. Each answers with nothing rather than
// failing, because what this walk is reading may be any shape at all and the schema is what says
// so.

func asObject(value any) map[string]any {
	held, _ := value.(map[string]any)
	return held
}

func asArray(value any) ([]any, bool) {
	held, ok := value.([]any)
	return held, ok
}

func asString(value any) string {
	held, _ := value.(string)
	return held
}

// What a screen may not hold, beyond naming the components it stands for.
//
// A screen is drawn and it is never run. The page draws each one inside a frame with no network and
// no script of the session's, so a script, a handler and an address off the page all draw nothing.
// The containment is what holds a screen stored before this check; this is the half that tells the
// session while it writes. Measured on the 18 screens of one project: every one loaded a script from
// a delivery network, 14 named a font host in a link and 2 more reached it with @import.
//
// Two addresses stay inside the page. An asset of the approved design system, which the page
// resolves into the document as a data address, and a part of the screen itself. Every other address
// reaches a machine somewhere, so the screen draws one thing here and another thing there, and it
// tells that machine it was opened.

// theAddressAttributes are the attributes whose value is an address. A srcset holds several, so it
// is read candidate by candidate.
var theAddressAttributes = map[string]bool{"src": true, "href": true, "srcset": true}

// theAddressShown caps how much of an address a refusal repeats. An address is there to be found in
// a file, and a data address runs to hundreds of thousands of characters.
const theAddressShown = 120

// The four things a screen may not hold, which are four different things to do about it.
const (
	faultScript  = "script"
	faultEvent   = "event"
	faultImport  = "import"
	faultAddress = "address"
)

// screenFault is what one screen holds that it may not, in the words the refusal reads it out in.
type screenFault struct {
	kind string
	// said is the attribute name, the address, or the import as it was written.
	said string
	// where names the part of the screen it sits on, so an operator opens the right line.
	where string
}

// aScreenThatReachesOut finds the first screen holding a script, a handler or an address off the
// page. The screens are read in name order and each screen in document order, so two reads of one
// artifact name the same fault.
func aScreenThatReachesOut(doc any) (screen string, fault screenFault, found bool) {
	held := asObject(asObject(doc)["screens"])
	for _, name := range sortedKeys(held) {
		markup := asString(asObject(held[name])["html"])
		if strings.TrimSpace(markup) == "" {
			continue
		}
		body := &html.Node{Type: html.ElementNode, DataAtom: atom.Body, Data: "body"}
		nodes, err := html.ParseFragment(strings.NewReader(markup), body)
		if err != nil {
			continue
		}
		for _, node := range nodes {
			if fault, found := whatReachesOut(node); found {
				return name, fault, true
			}
		}
	}
	return "", screenFault{}, false
}

// whatReachesOut reads one element and everything under it, in the order the markup was written in.
func whatReachesOut(node *html.Node) (screenFault, bool) {
	if node.Type == html.ElementNode {
		if node.Data == "script" {
			return screenFault{kind: faultScript}, true
		}
		if fault, found := whatAnAttributeReaches(node); found {
			return fault, true
		}
		if node.Data == "style" {
			if fault, found := whatAStylesheetReaches(theTextOf(node)); found {
				fault.where = "the stylesheet of the screen"
				return fault, true
			}
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if fault, found := whatReachesOut(child); found {
			return fault, true
		}
	}
	return screenFault{}, false
}

// whatAnAttributeReaches reads the attributes of one element, in the order they were written.
//
// An attribute whose name starts with "on" is read whatever it holds, because the page strips the
// whole class of them rather than the handful somebody thought of.
func whatAnAttributeReaches(node *html.Node) (screenFault, bool) {
	part := partOf(node, false).describe()
	for _, held := range node.Attr {
		name := strings.ToLower(held.Key)
		switch {
		case strings.HasPrefix(name, "on"):
			return screenFault{kind: faultEvent, said: name, where: part}, true
		case theAddressAttributes[name]:
			for _, address := range theAddressesIn(name, held.Val) {
				if reachesOffThePage(address) {
					return screenFault{kind: faultAddress, said: cappedTo(address, theAddressShown),
						where: "the " + name + " of " + part}, true
				}
			}
		case name == "style":
			if fault, found := whatAStylesheetReaches(held.Val); found {
				fault.where = "the style of " + part
				return fault, true
			}
		}
	}
	return screenFault{}, false
}

// whatAStylesheetReaches reads a <style> element of a screen, or a style attribute on one of its
// parts. An @import is refused whole, the way the design system's own stylesheet refuses one: it is
// an address written in a second grammar, and a reader told only about url() writes it next.
func whatAStylesheetReaches(sheet string) (screenFault, bool) {
	if at := strings.Index(strings.ToLower(sheet), "@import"); at >= 0 {
		said := sheet[at:]
		if end := strings.Index(said, ";"); end >= 0 {
			said = said[:end+1]
		}
		return screenFault{kind: faultImport,
			said: cappedTo(strings.Join(strings.Fields(said), " "), theAddressShown)}, true
	}
	for _, found := range anAddress.FindAllStringSubmatch(sheet, -1) {
		address := strings.Trim(found[1], `"'`)
		if reachesOffThePage(address) {
			return screenFault{kind: faultAddress, said: cappedTo(address, theAddressShown)}, true
		}
	}
	return screenFault{}, false
}

// theAddressesIn is every address one attribute holds. A srcset names the same image at several
// sizes, so each candidate is read and the descriptor beside it is not an address.
func theAddressesIn(attribute, value string) []string {
	if attribute != "srcset" {
		return []string{value}
	}
	var out []string
	for _, candidate := range strings.Split(value, ",") {
		if fields := strings.Fields(candidate); len(fields) > 0 {
			out = append(out, fields[0])
		}
	}
	return out
}

// reachesOffThePage says whether an address leaves the document the page composed. An asset of the
// design system does not, because the page writes the bytes into the document. A fragment does not,
// because it names a part of the screen itself. An empty address reaches nothing at all.
func reachesOffThePage(address string) bool {
	held := strings.TrimSpace(address)
	if held == "" || strings.HasPrefix(held, "#") {
		return false
	}
	if name, asset := strings.CutPrefix(held, theAssetScheme); asset {
		return !aReadableName.MatchString(name)
	}
	return true
}

// cappedTo caps a value a refusal repeats, and says it cut it, so a long value cannot bury the sentence
// that says what to do.
func cappedTo(value string, most int) string {
	if utf8.RuneCountInString(value) <= most {
		return value
	}
	return string([]rune(value)[:most]) + "..."
}

// theTextOf is the text a <style> element holds, which is the stylesheet of one screen.
func theTextOf(node *html.Node) string {
	var said []string
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode {
			said = append(said, child.Data)
		}
	}
	return strings.Join(said, "")
}

// refusalFor says what the screen holds and what to write instead. Each of the four says a different
// thing to do, and each one names the screen, because a mockup runs to dozens of them.
func refusalFor(screen string, fault screenFault) error {
	switch fault.kind {
	case faultScript:
		return status.Errorf(codes.InvalidArgument,
			"the mockups artifact is refused: the %q screen holds a <script> element. A screen is drawn "+
				"and never run, because the page draws it in a frame that runs no script a session wrote: "+
				"write the screen as markup and css.", screen)
	case faultEvent:
		return status.Errorf(codes.InvalidArgument,
			"the mockups artifact is refused: the %q screen carries %q on %s. A screen is drawn and never "+
				"run: take the behaviour out, and write %s to say which screen a press opens.",
			screen, fault.said, fault.where, thePressAttribute)
	case faultImport:
		return status.Errorf(codes.InvalidArgument,
			"the mockups artifact is refused: the %q screen imports a stylesheet, in %q. A screen reaches "+
				"no address: the fonts and the images come from the approved design_system stage, and a "+
				"stylesheet reaches one with url(%s<name>).", screen, fault.said, theAssetScheme)
	}
	return status.Errorf(codes.InvalidArgument,
		"the mockups artifact is refused: the %q screen names the address %q, in %s. A screen reaches no "+
			"address of its own: the fonts and the images come from the approved design_system stage, "+
			"written as %s<name> under the name that stage holds them by, and a part of the screen itself "+
			"is named as #<name>.", screen, fault.said, fault.where, theAssetScheme)
}

// What a screen written as markup may be drawn in.
//
// A screen used to carry its own token block, and the check read that. The tokens have one home now,
// the design_system stage, so a value reaches a screen another way: the stylesheet of the screen, the
// style of one part, or the paint of a mark. Each of those is read here, and the screens are read in
// name order with each screen in document order, so two reads of one artifact name the same value.
//
// The words on a screen are not read. A bill reference of "#dedbee" in a paragraph is what a person
// reads, and a gate that refused it would be refusing words rather than a design system.

// thePaintAttributes are the attributes that paint rather than describe. A mark is written as a
// drawing in the markup, and a drawing carries its colours here rather than in a declaration.
var thePaintAttributes = map[string]bool{
	"fill": true, "stroke": true, "stop-color": true, "flood-color": true,
	"lighting-color": true, "color": true, "bgcolor": true,
}

// theFontProperty reads a font out of a stylesheet or out of the style of one part. The boundary in
// front of the name is what keeps font-size and font-weight out of it, and keeps the custom
// properties the page writes out of it as well.
var theFontProperty = regexp.MustCompile(`(?i)(?:^|[;{}\s])(font-family|font)\s*:\s*([^;}]+)`)

// theFamilyInTheShorthand takes the family list out of a font shorthand. The grammar puts the size
// last of the parts in front of it, with an optional line height after a slash, so the family is
// what follows the last size. The fixture the skill ships writes its fonts this way, so a reading of
// the long form alone would read none of the screens somebody copies from it.
var theFamilyInTheShorthand = regexp.MustCompile(
	`(?i)^.*\d[\d.]*(?:px|pt|pc|em|rem|ex|ch|%|vh|vw|vmin|vmax|cm|mm|in)?(?:\s*/\s*\S+)?\s+(.+)$`)

// theFontProperty names the two writings a session is told to use. A token is reached through the
// custom property the page writes for it.
const theFontPropertyPrefix = "var(--t-font-"

// paintedValue is one value a screen is drawn in that the design system does not name.
type paintedValue struct {
	// kind is "colour" or "font", because the two say different things to do.
	kind string
	// said is the value as it was written, so an operator finds it in the file.
	said string
	// where names the place it sits: the stylesheet of the screen, or the style or the paint of one
	// part.
	where string
}

// aValuePaintedOutside finds the first colour or font of one screen's markup that the approved design
// system does not name.
//
// Markup it cannot parse is no fault of this walk: an artifact whose screens are not screens is the
// schema's to refuse.
func aValuePaintedOutside(markup string, named designTokens) (paintedValue, bool) {
	if strings.TrimSpace(markup) == "" {
		return paintedValue{}, false
	}
	body := &html.Node{Type: html.ElementNode, DataAtom: atom.Body, Data: "body"}
	nodes, err := html.ParseFragment(strings.NewReader(markup), body)
	if err != nil {
		return paintedValue{}, false
	}
	for _, node := range nodes {
		if painted, found := whatIsPaintedUnder(node, named); found {
			return painted, true
		}
	}
	return paintedValue{}, false
}

// whatIsPaintedUnder reads one element and everything under it, in the order the markup was written.
func whatIsPaintedUnder(node *html.Node, named designTokens) (paintedValue, bool) {
	if node.Type == html.ElementNode {
		if node.Data == "style" {
			if painted, found := whatAStylesheetPaints(theTextOf(node), named); found {
				painted.where = "the stylesheet of the screen"
				return painted, true
			}
		}
		if painted, found := whatAnAttributePaints(node, named); found {
			return painted, true
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if painted, found := whatIsPaintedUnder(child, named); found {
			return painted, true
		}
	}
	return paintedValue{}, false
}

// whatAnAttributePaints reads the attributes of one element, in the order they were written.
func whatAnAttributePaints(node *html.Node, named designTokens) (paintedValue, bool) {
	part := partOf(node, false).describe()
	for _, held := range node.Attr {
		name := strings.ToLower(held.Key)
		switch {
		case name == "style":
			if painted, found := whatAStylesheetPaints(held.Val, named); found {
				painted.where = "the style of " + part
				return painted, true
			}
		case thePaintAttributes[name]:
			if colour, found := anUnnamedColour(held.Val, named); found {
				return paintedValue{kind: "colour", said: colour,
					where: "the " + name + " of " + part}, true
			}
		}
	}
	return paintedValue{}, false
}

// whatAStylesheetPaints reads a <style> element of a screen, or the style of one of its parts. The
// colours come first, because a colour is the thing a screen is mostly drawn in and a rule usually
// carries several of them beside one font.
func whatAStylesheetPaints(sheet string, named designTokens) (paintedValue, bool) {
	if colour, found := anUnnamedColour(sheet, named); found {
		return paintedValue{kind: "colour", said: colour}, true
	}
	for _, held := range theFontProperty.FindAllStringSubmatch(sheet, -1) {
		family := strings.TrimSpace(held[2])
		if strings.EqualFold(held[1], "font") {
			shorthand := theFamilyInTheShorthand.FindStringSubmatch(family)
			if shorthand == nil {
				continue
			}
			family = strings.TrimSpace(shorthand[1])
		}
		if !theDesignSystemNamesTheFont(family, named) {
			return paintedValue{kind: "font", said: family}, true
		}
	}
	return paintedValue{}, false
}

// anUnnamedColour is the first colour written into a value that the design system does not name.
func anUnnamedColour(value string, named designTokens) (string, bool) {
	for _, colour := range aColourLiteral.FindAllString(value, -1) {
		if !named.colour[normalise(colour)] {
			return colour, true
		}
	}
	return "", false
}

// theDesignSystemNamesTheFont says whether a screen may be drawn in one font value.
//
// Two writings are named. The custom property the page writes for a font token, which is what a
// session is told to write, and a family a font token names, because a token value is a family list
// and a screen may reach for one family of it.
func theDesignSystemNamesTheFont(value string, named designTokens) bool {
	held := normalise(value)
	if held == "" || named.font[held] {
		return true
	}
	for _, one := range strings.Split(held, ",") {
		family := strings.Trim(strings.TrimSpace(one), `"'`)
		if family == "" {
			continue
		}
		if strings.HasPrefix(family, theFontPropertyPrefix) && strings.HasSuffix(family, ")") {
			continue
		}
		if !named.family[family] {
			return false
		}
	}
	return true
}

// refusalForThePaint says what the screen is drawn in and what to write instead. Each kind says a
// different thing to do, and both name the screen, because a mockup runs to dozens of them.
func refusalForThePaint(screen string, painted paintedValue) error {
	if painted.kind == "font" {
		return status.Errorf(codes.InvalidArgument,
			"the mockups artifact is refused: the %q screen is drawn in the font %q, in %s, and the "+
				"approved design_system stage does not name it: write %s<name>), and the font file a "+
				"screen draws from travels in that stage.",
			screen, painted.said, painted.where, theFontPropertyPrefix)
	}
	return status.Errorf(codes.InvalidArgument,
		"the mockups artifact is refused: the %q screen is drawn in the colour %q, in %s, and the "+
			"approved design_system stage does not name it: take every colour from that stage, "+
			"written as var(--t-colour-<name>).",
		screen, painted.said, painted.where)
}
