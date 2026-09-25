package controlplane

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	flowmap "github.com/atlantic-blue/quay-krewe/skills/flow-map"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// What the design_system stage is held to, beyond being json.
//
// Every screen of a project is drawn from this one stage, and the mockups check holds each screen to
// the colours and the fonts it names. The stage itself had no shape, so it carried whatever json a
// session wrote, and a design system written as prose named nothing at all. The mockups write then
// answered that the screen was kept without that check, and nothing held those screens to anything.
//
// So the artifact is read before the store keeps it. It carries its tokens, the font files and the
// images a screen is drawn with, and the base stylesheet every screen is given.
//
// The caps are here because the whole stage travels in one request. A file nobody can send is a file
// no screen is ever drawn in, and the write is where an operator can still do something about it.
//
// Nothing here runs for the other five stages. They carry whatever json they carry.

// The name every token and every asset is held to. The page writes a token name into a custom
// property and an asset name into an address, so a name outside this is a property or an address
// nothing can read.
var aReadableName = regexp.MustCompile(`^[a-z0-9-]+$`)

// The size one asset and every asset together may reach, once decoded.
//
// A design system travels in one request, and nothing raises the receive ceiling of 4 mebibytes.
// Base64 makes 2 mebibytes of files into about 2.7 mebibytes of text, which leaves room for the
// tokens and the stylesheet beside it.
const (
	oneAssetCeiling  = 512 << 10
	allAssetsCeiling = 2 << 20
)

// anAddress is a url in a stylesheet, whatever quoting it was written with.
var anAddress = regexp.MustCompile(`(?i)url\(\s*([^)]*?)\s*\)`)

// theAssetScheme is how a stylesheet and a screen reach a file of the design system.
const theAssetScheme = "asset:"

// systemSchema is skills/flow-map/design-system.schema.json, compiled once and lazily, for the
// reason the flow map schema is.
var systemSchema = sync.OnceValues(compileSystemSchema)

func compileSystemSchema() (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(flowmap.SystemSchemaJSON))
	if err != nil {
		return nil, fmt.Errorf("the design system schema is not json: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(flowmap.SystemSchemaID, doc); err != nil {
		return nil, fmt.Errorf("the design system schema will not load: %w", err)
	}
	return compiler.Compile(flowmap.SystemSchemaID)
}

// theSchemaFile is where a refusal sends the reader, and it is the file the skill tells a session to
// write the design system against.
const theSchemaFile = "skills/flow-map/design-system.schema.json"

// checkDesignSystemArtifact reads a design system and refuses what no screen could be drawn from.
//
// An artifact that is not json is not this check's to report: the store refuses it, with one message,
// and a second message about the same fault sends the reader two ways. A stage carrying prose alone
// carries no artifact, and there is nothing to refuse.
func checkDesignSystemArtifact(artifact string) error {
	if strings.TrimSpace(artifact) == "" {
		return nil
	}
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(artifact))
	if err != nil {
		return nil
	}
	system := asObject(doc)

	// The two things every reader of the stage needs, checked here rather than left to the schema,
	// because a refusal naming the group sends the reader somewhere and a schema tree does not.
	tokens, held := system["tokens"]
	if !held {
		return status.Errorf(codes.InvalidArgument,
			"the design system artifact carries no tokens: every screen is drawn from them, so the "+
				"document holds a \"tokens\" object with a colour group and a font group (%s).",
			theSchemaFile)
	}
	groups := asObject(tokens)
	for _, group := range []string{"colour", "font"} {
		if len(asObject(groups[group])) == 0 {
			return status.Errorf(codes.InvalidArgument,
				"the design system artifact names no %s token: every screen is drawn in the tokens, so "+
					"the %s group holds one value or more (%s).",
				group, group, theSchemaFile)
		}
	}

	if err := readableNames(groups, system); err != nil {
		return err
	}

	schema, err := systemSchema()
	if err != nil {
		return status.Errorf(codes.Internal, "the design system schema could not be read: %v", err)
	}
	if err := schema.Validate(doc); err != nil {
		return status.Errorf(codes.InvalidArgument,
			"the design system artifact does not match the design system schema: %s. "+
				"%s holds every field and every allowed value.",
			whatTheSchemaSaid(err), theSchemaFile)
	}

	if err := assetsThatTravel(asObject(system["assets"]), groups); err != nil {
		return err
	}
	return theStylesheet(asString(system["css"]), groups)
}

// readableNames refuses the first token or asset name the page could not write.
//
// The token groups come first, in name order, so two reads of one document name the same fault.
func readableNames(groups, system map[string]any) error {
	for _, group := range sortedKeys(groups) {
		for _, name := range sortedKeys(asObject(groups[group])) {
			if !aReadableName.MatchString(name) {
				return status.Errorf(codes.InvalidArgument,
					"the design system artifact names the %s token %q, and a token name matches %s, "+
						"because the page writes the name into a custom property.",
					group, name, aReadableName.String())
			}
		}
	}
	for _, name := range sortedKeys(asObject(system["assets"])) {
		if !aReadableName.MatchString(name) {
			return status.Errorf(codes.InvalidArgument,
				"the design system artifact names the asset %q, and an asset name matches %s, because "+
					"the page writes the name into an address.",
				name, aReadableName.String())
		}
	}
	return nil
}

// assetsThatTravel refuses a file that cannot be read, a file too big to send, and a font no screen
// could ask for. The assets are read in name order, so a document with two faults names the same one
// every time.
func assetsThatTravel(assets, groups map[string]any) error {
	var all int
	for _, name := range sortedKeys(assets) {
		asset := asObject(assets[name])
		file, err := base64.StdEncoding.DecodeString(asString(asset["data"]))
		if err != nil {
			return status.Errorf(codes.InvalidArgument,
				"the asset %q of the design system carries data that is not base64: %v. The data field "+
					"holds the bytes of the file as base64, with no data address in front of them.",
				name, err)
		}
		if len(file) > oneAssetCeiling {
			return status.Errorf(codes.InvalidArgument,
				"the asset %q of the design system decodes to %d bytes, and one asset holds %d bytes at "+
					"most, which is %d kibibytes, because the whole stage travels in one request.",
				name, len(file), oneAssetCeiling, oneAssetCeiling>>10)
		}
		all += len(file)
		if err := aReachableFont(name, asset, asObject(groups["font"])); err != nil {
			return err
		}
	}
	if all > allAssetsCeiling {
		return status.Errorf(codes.InvalidArgument,
			"the assets of the design system decode to %d bytes together, and they hold %d bytes at "+
				"most, which is %d mebibytes, because the whole stage travels in one request.",
			all, allAssetsCeiling, allAssetsCeiling>>20)
	}
	return nil
}

// aReachableFont refuses a font file whose family no font token names. A screen writes a family to
// reach a font, so a family nothing names is a file nobody can draw in.
func aReachableFont(name string, asset, fonts map[string]any) error {
	if asString(asset["kind"]) != "font" {
		return nil
	}
	family := asString(asset["family"])
	for _, token := range sortedKeys(fonts) {
		for _, one := range strings.Split(asString(fonts[token]), ",") {
			if normalise(strings.Trim(strings.TrimSpace(one), `"'`)) == normalise(family) {
				return nil
			}
		}
	}
	return status.Errorf(codes.InvalidArgument,
		"the asset %q of the design system is the font family %q, and no font token names it: name the "+
			"family in a font token, or write the family a token already names.",
		name, family)
}

// theStylesheet refuses a base stylesheet that reaches off the page or paints outside the tokens.
//
// It is the one file every screen is given, so a colour written into it is a second design system on
// every screen at once.
func theStylesheet(sheet string, groups map[string]any) error {
	if sheet == "" {
		return nil
	}
	if strings.Contains(strings.ToLower(sheet), "@import") {
		return status.Errorf(codes.InvalidArgument,
			"the css of the design system holds @import, and a screen reaches nothing off the page: put "+
				"the file in assets and reach it with url(%s<name>).", theAssetScheme)
	}
	for _, found := range anAddress.FindAllStringSubmatch(sheet, -1) {
		address := strings.Trim(found[1], `"'`)
		if named, held := strings.CutPrefix(address, theAssetScheme); held && aReadableName.MatchString(named) {
			continue
		}
		return status.Errorf(codes.InvalidArgument,
			"the css of the design system reaches the address %q, and a stylesheet holds url(%s<name>) "+
				"and no other address: put the file in assets and reach it by name.",
			address, theAssetScheme)
	}
	named := theColoursNamed(groups)
	for _, colour := range aColourLiteral.FindAllString(sheet, -1) {
		if !named[normalise(colour)] {
			return status.Errorf(codes.InvalidArgument,
				"the css of the design system paints in the colour %q, and the tokens do not name it: "+
					"take every colour from a token, written as var(--t-colour-<name>).", colour)
		}
	}
	return nil
}

// theColoursNamed is every colour the tokens hold, read out of the values rather than out of the
// colour group alone. A shadow or a border token carries a colour inside a longer value, and a
// stylesheet painting in that colour is painting in the design system.
func theColoursNamed(groups map[string]any) map[string]bool {
	out := map[string]bool{}
	for _, group := range sortedKeys(groups) {
		values := asObject(groups[group])
		for _, name := range sortedKeys(values) {
			for _, colour := range aColourLiteral.FindAllString(asString(values[name]), -1) {
				out[normalise(colour)] = true
			}
		}
	}
	return out
}

// The three readings both checks share. A colour is one pattern across the page, the mockups and the
// design system, and two of them normalising differently would refuse a value the third accepted.

// aColourLiteral is a colour written out rather than named. It is the pattern the flow map's own
// tests read the page's markup with, so the page and these checks agree on what a colour is.
var aColourLiteral = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b|rgba?\([^)]*\)|hsla?\([^)]*\)`)

// normalise is how two writings of one value are held to be the same value. A hexadecimal colour is
// written in either case, and a font list is written with whatever spacing suits the writer.
func normalise(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(value)), " ")
}

// sortedKeys reads a map in one order, so a refusal about a document names the same thing twice.
func sortedKeys(held map[string]any) []string {
	out := make([]string, 0, len(held))
	for key := range held {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
