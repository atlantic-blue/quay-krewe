package controlplane

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/atlantic-blue/quay-krewe/internal/store"
	flowmap "github.com/atlantic-blue/quay-krewe/skills/flow-map"
	"github.com/santhosh-tekuri/jsonschema/v6"
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
	if named.empty() {
		return []string{"the approved design_system stage names no colour and no font, so the mockup was " +
			"kept without that check: write its tokens to have the mockups held to them"}, nil
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
		return designTokens{
			colour: valueSet(asObject(groups["colour"])),
			font:   valueSet(asObject(groups["font"])),
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
