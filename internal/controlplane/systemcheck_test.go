package controlplane_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/controlplane"
	"github.com/atlantic-blue/quay-krewe/internal/model"
	"github.com/atlantic-blue/quay-krewe/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SYSTEM-1. What the design_system stage is held to, beyond being json.
//
// The stage had no shape, so it carried anything, and the mockups check then had nothing to hold a
// screen to. A design system is read here instead: its tokens, the font files and the images a
// screen is drawn with, and the base stylesheet every screen is given.
//
// The caps are here because everything in the stage travels in one request. A font nobody can send
// is a font no screen is drawn in, and the operator learns that at the write rather than at the
// screen.
//
// The stylesheet is held to the tokens for the reason a mockup is: a colour written beside them is a
// second design system nobody approved. A value named as a custom property is the way to reach one.

// The size of one asset and of all of them, decoded, past which a write is refused.
const (
	oneAssetCap  = 512 * 1024
	allAssetsCap = 2 * 1024 * 1024
)

// systemTokens is a whole tokens block, and the values every stylesheet below is painted from.
const systemTokens = `"tokens": {
		"colour": {"surface": "#fdfbf7", "ink": "#1d1c1a", "primary": "#1b6b57", "on-primary": "#ffffff"},
		"font": {"sans": "Inter, system-ui, sans-serif", "mono": "JetBrains Mono, monospace"},
		"radius": {"screen": "24px", "control": "12px"},
		"space": {"gap": "9px", "pad": "14px"}
	}`

// systemAssets is one font file and one image, each named the way a page writes it into an address.
var systemAssets = fmt.Sprintf(`"assets": {
		"inter-regular": {"kind": "font", "media": "font/woff2", "family": "Inter",
			"weight": "400", "style": "normal", "data": %q, "label": "Inter regular"},
		"logo": {"kind": "image", "media": "image/svg+xml", "data": %q, "label": "the mark"}
	}`, someBytes(64), someBytes(64))

// systemCSS paints from the tokens and reaches the image through the asset address, which is the
// only address a stylesheet may hold.
const systemCSS = `"css": "body{background:var(--t-colour-surface);color:var(--t-colour-ink);` +
	`font-family:var(--t-font-sans)}\n.logo{background-image:url(asset:logo)}\n"`

// someBytes is base64 for a run of bytes of one size. The check decodes an asset and never reads
// what it decoded, so a font file here is bytes rather than a font.
func someBytes(size int) string {
	return base64.StdEncoding.EncodeToString([]byte(strings.Repeat("f", size)))
}

// aDesignSystem is a whole artifact, from the three parts a caller wants to try.
func aDesignSystem(parts ...string) string {
	return "{" + strings.Join(parts, ",") + "}"
}

// designedUpToTheDesignSystem walks a project to the point a design system may be written: the two
// stages before it are written and carry the operator's word.
func designedUpToTheDesignSystem(t *testing.T, s *controlplane.Server, project string) {
	t.Helper()
	ctx := context.Background()
	for _, stage := range []string{store.StageDiscovery, store.StageStories} {
		if _, err := s.SetDesignStage(ctx, &quaycrewv1.SetDesignStageRequest{
			Project: project, Stage: stage, Body: "the " + stage + " body",
		}); err != nil {
			t.Fatalf("SetDesignStage %s: %v", stage, err)
		}
		if _, err := s.ApproveDesignStage(ctx, &quaycrewv1.ApproveDesignStageRequest{
			Project: project, Stage: stage,
		}); err != nil {
			t.Fatalf("ApproveDesignStage %s: %v", stage, err)
		}
	}
}

// writeDesignSystemStage is the call under test, with the artifact the caller wants to try.
func writeDesignSystemStage(s *controlplane.Server, project, artifact string) (
	*quaycrewv1.SetDesignStageResponse, error) {
	return s.SetDesignStage(context.Background(), &quaycrewv1.SetDesignStageRequest{
		Project: project, Stage: store.StageDesignSystem, Body: "the design system", Artifact: artifact,
	})
}

// designSystemStageHeld is what the project holds as its design system, so a test can prove a
// refusal left nothing behind.
func designSystemStageHeld(t *testing.T, s *controlplane.Server, project string) *quaycrewv1.DesignStage {
	t.Helper()
	listed, err := s.ListDesignStages(context.Background(), &quaycrewv1.ListDesignStagesRequest{Project: project})
	if err != nil {
		t.Fatalf("ListDesignStages: %v", err)
	}
	for _, stage := range listed.GetStages() {
		if stage.GetStage() == store.StageDesignSystem {
			return stage
		}
	}
	return nil
}

// refusalOf is the message a refused write left, and it fails the test when the write went through.
func refusalOf(t *testing.T, err error, what string) string {
	t.Helper()
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("%s answered %v, want InvalidArgument", what, err)
	}
	return status.Convert(err).Message()
}

// mustSay holds a refusal to naming the field and the value, so an operator reads what to change
// rather than that something is wrong.
func mustSay(t *testing.T, said string, want ...string) {
	t.Helper()
	for _, one := range want {
		if !strings.Contains(said, one) {
			t.Errorf("the refusal reads %q, and it has to say %q", said, one)
		}
	}
}

// The contract, whole. A design system with its tokens, its font file, its image and its stylesheet
// is what every screen of the project is drawn from, so it is kept as it was given.
func TestADesignSystemCarryingItsTokensItsFilesAndItsStylesheetIsKept(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToTheDesignSystem(t, s, project)

	written, err := writeDesignSystemStage(s, project, aDesignSystem(systemTokens, systemAssets, systemCSS))
	if err != nil {
		t.Fatalf("a whole design system was refused: %v", err)
	}
	held := written.GetStage().GetArtifact()
	for _, want := range []string{"inter-regular", "url(asset:logo)", "var(--t-colour-surface)"} {
		if !strings.Contains(held, want) {
			t.Errorf("the stage carries %q, and %q did not travel with it", held, want)
		}
	}
	if said := strings.Join(written.GetWarnings(), " "); said != "" {
		t.Errorf("the write warned %q, and the design system it was given is whole", said)
	}
}

// Prose first and the document after is how a stage gets written. A design system with nothing to
// read has nothing to refuse.
func TestADesignSystemStageCarryingProseAloneIsNotRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToTheDesignSystem(t, s, project)

	if _, err := writeDesignSystemStage(s, project, ""); err != nil {
		t.Fatalf("a design system written as prose alone was refused: %v", err)
	}
}

// The tokens are the part every screen reads, so a document that names no colour cannot be a design
// system. The schema is the file the skill tells a session to write against, so the refusal sends
// the reader to it.
func TestADesignSystemThatNamesNoColourIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToTheDesignSystem(t, s, project)

	_, err := writeDesignSystemStage(s, project,
		`{"tokens": {"font": {"sans": "Inter, sans-serif"}}}`)

	mustSay(t, refusalOf(t, err, "a design system naming no colour"), "colour", "schema")
	if held := designSystemStageHeld(t, s, project); held != nil {
		t.Fatalf("the refused write left a design system behind, holding %q", held.GetArtifact())
	}
}

// The groups sit under tokens, and a document of bare groups is the shape that used to go in. It is
// refused from here, and the refusal names the field that is missing.
func TestADesignSystemOfBareGroupsIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToTheDesignSystem(t, s, project)

	_, err := writeDesignSystemStage(s, project,
		`{"colour": {"ink": "#1d1c1a"}, "font": {"sans": "Inter, sans-serif"}}`)

	mustSay(t, refusalOf(t, err, "a design system of bare groups"), "tokens")
}

// A token name is written into a custom property, so a name outside the pattern is a property no
// screen can read. The refusal names the pattern, because the fault is in the name and not the
// value.
func TestATokenNameOutsideThePatternIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToTheDesignSystem(t, s, project)

	_, err := writeDesignSystemStage(s, project, aDesignSystem(
		strings.Replace(systemTokens, `"on-primary"`, `"onPrimary"`, 1), systemCSS))

	mustSay(t, refusalOf(t, err, "a token name outside the pattern"), "onPrimary", "^[a-z0-9-]+$")
}

// An asset name is written into an address, and the same rule holds for the same reason.
func TestAnAssetNameOutsideThePatternIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToTheDesignSystem(t, s, project)

	_, err := writeDesignSystemStage(s, project, aDesignSystem(systemTokens,
		strings.Replace(systemAssets, `"inter-regular"`, `"Inter_Regular"`, 1)))

	mustSay(t, refusalOf(t, err, "an asset name outside the pattern"), "Inter_Regular", "^[a-z0-9-]+$")
}

// The file itself. An asset the page cannot decode is an asset no screen is drawn in, and the fault
// is in one named field of one named asset.
func TestAnAssetWhoseDataIsNotBase64IsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToTheDesignSystem(t, s, project)

	_, err := writeDesignSystemStage(s, project, aDesignSystem(systemTokens,
		`"assets": {"inter-regular": {"kind": "font", "media": "font/woff2", "family": "Inter",
			"data": "this is not base64, it is a sentence"}}`))

	mustSay(t, refusalOf(t, err, "an asset whose data is not base64"), "inter-regular", "data", "base64")
}

// The cap on one file. Everything in the stage travels in one request, so a font past the cap is a
// design system that cannot be written at all. The message names the size as well as the cap,
// because the writer has to know how far over it is.
func TestAnAssetOverTheCapIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToTheDesignSystem(t, s, project)

	over := oneAssetCap + 1024
	_, err := writeDesignSystemStage(s, project, aDesignSystem(systemTokens, fmt.Sprintf(
		`"assets": {"inter-regular": {"kind": "font", "media": "font/woff2", "family": "Inter",
			"data": %q}}`, someBytes(over))))

	mustSay(t, refusalOf(t, err, "an asset over the cap"),
		"inter-regular", fmt.Sprint(over), fmt.Sprint(oneAssetCap))
}

// The cap on all of them. Five files each inside the cap still travel together, and together they
// are what the request carries.
func TestAllTheAssetsTogetherOverTheCapAreRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToTheDesignSystem(t, s, project)

	const each, count = 500 * 1024, 5
	var assets []string
	for at := range count {
		assets = append(assets, fmt.Sprintf(
			`"image-%d": {"kind": "image", "media": "image/png", "data": %q}`, at, someBytes(each)))
	}
	_, err := writeDesignSystemStage(s, project, aDesignSystem(systemTokens,
		`"assets": {`+strings.Join(assets, ",")+`}`))

	mustSay(t, refusalOf(t, err, "assets over the total cap"),
		fmt.Sprint(each*count), fmt.Sprint(allAssetsCap))
}

// A font is reached through its family, and a family no token names is a file nothing can ask for.
func TestAFontAssetWhoseFamilyNoFontTokenNamesIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToTheDesignSystem(t, s, project)

	_, err := writeDesignSystemStage(s, project, aDesignSystem(systemTokens,
		strings.Replace(systemAssets, `"family": "Inter"`, `"family": "Comic Sans MS"`, 1)))

	mustSay(t, refusalOf(t, err, "a font no token names"), "Comic Sans MS", "family", "font")
}

// The stylesheet reaches nothing off the page. An import is a request to a network the screen does
// not have.
func TestAStylesheetHoldingAnImportIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToTheDesignSystem(t, s, project)

	_, err := writeDesignSystemStage(s, project, aDesignSystem(systemTokens,
		`"css": "@import url(asset:other);\nbody{color:var(--t-colour-ink)}"`))

	mustSay(t, refusalOf(t, err, "a stylesheet holding an import"), "@import", "css")
}

// An address in the stylesheet is an asset of the design system or it is nothing. A font loaded from
// somewhere else is the shape a session writes unless it is told not to.
func TestAStylesheetAddressOutsideTheDesignSystemIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToTheDesignSystem(t, s, project)

	_, err := writeDesignSystemStage(s, project, aDesignSystem(systemTokens,
		`"css": "@font-face{font-family:\"Inter\";src:url(https://fonts.googleapis.com/inter.woff2)}"`))

	mustSay(t, refusalOf(t, err, "a stylesheet reaching off the page"),
		"https://fonts.googleapis.com/inter.woff2", "asset:")
}

// The tokens hold the stylesheet too. A colour painted beside them is a second design system, in the
// one file every screen is given.
func TestAStylesheetColourTheTokensDoNotNameIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToTheDesignSystem(t, s, project)

	_, err := writeDesignSystemStage(s, project, aDesignSystem(systemTokens,
		`"css": "body{color:var(--t-colour-ink)}\n.notice{background:#ff0000}"`))

	mustSay(t, refusalOf(t, err, "a stylesheet colour outside the tokens"), "#ff0000", "css")
}

// The other half of that rule. A colour the tokens do name may be written out, because a value in
// the design system is the design system wherever it is spelled.
func TestAStylesheetColourTheTokensNameIsKept(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToTheDesignSystem(t, s, project)

	if _, err := writeDesignSystemStage(s, project, aDesignSystem(systemTokens,
		`"css": "body{background:#FDFBF7;color:var(--t-colour-ink)}"`)); err != nil {
		t.Fatalf("a stylesheet painted in a colour the tokens name was refused: %v", err)
	}
}

// Two assets, both wrong. A map is read in whatever order the runtime feels like, so a refusal that
// read one would name a different asset on a different day, and two operators comparing notes would
// disagree about what the file says.
func TestTheRefusalNamesTheSameAssetEveryTime(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToTheDesignSystem(t, s, project)

	two := aDesignSystem(systemTokens, `"assets": {
		"zebra": {"kind": "image", "media": "image/png", "data": "not base64 at all"},
		"apple": {"kind": "image", "media": "image/png", "data": "not base64 at all"}}`)

	for attempt := range 8 {
		_, err := writeDesignSystemStage(s, project, two)
		said := status.Convert(err).Message()
		if !strings.Contains(said, "apple") {
			t.Fatalf("attempt %d read %q, want the first asset in name order", attempt, said)
		}
	}
}

// The check belongs to this one stage. A check that reached the other five would refuse the
// discovery stage for not being a design system.
func TestAStageThatIsNotTheDesignSystemCarriesAnyJSONItLikes(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)

	if _, err := s.SetDesignStage(context.Background(), &quaycrewv1.SetDesignStageRequest{
		Project: project, Stage: store.StageDiscovery, Body: "what it has today",
		Artifact: `{"asked": ["what does it look like"]}`,
	}); err != nil {
		t.Fatalf("a discovery artifact that is not a design system was refused: %v", err)
	}
}
