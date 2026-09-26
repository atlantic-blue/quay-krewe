package controlplane_test

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/controlplane"
	"github.com/atlantic-blue/quay-krewe/internal/model"
	"github.com/atlantic-blue/quay-krewe/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The two contracts the mockups stage adds to a write.
//
// FLOW-3: a mockups artifact whose screen holds a shape with no component is refused, and the
// refusal names the screen and the shape.
//
// FLOW-4: a mockups artifact drawn in a colour or a font that the approved design_system stage
// does not name is refused.
//
// SCREEN-4: a screen written as markup is drawn in the colours and the fonts of the approved
// design_system stage, in its markup and in its stylesheet, and in nothing else.
//
// SYSTEM-3: a mockups artifact is refused while the approved design_system stage names no colour
// and no font, because there is nothing to hold the screens to.
//
// All are read at the call, because that is where an operator meets them. A mockup nobody could
// build from never reaches the person who would otherwise approve it.

// The colours and fonts the design system names in these tests. The mockup below is drawn in the
// same values, so a test about a component is not quietly a test about a colour.
const designSystemArtifact = `{"tokens": {
	"colour": {
		"surface": "#fdfbf7", "surface-low": "#f1ece4", "ink": "#1d1c1a", "muted": "#5f5a52",
		"line": "#ded5c8", "primary": "#1b6b57", "on-primary": "#ffffff", "frame": "#211f1c"
	},
	"font": {"sans": "Inter, system-ui, sans-serif", "mono": "JetBrains Mono, monospace"}
}}`

// mockupTokens is the tokens block of a flows.json, drawn in the values above.
const mockupTokens = `"tokens": {
	"colour": {
		"surface": "#fdfbf7", "surface-low": "#f1ece4", "ink": "#1d1c1a", "muted": "#5f5a52",
		"line": "#ded5c8", "primary": "#1b6b57", "on-primary": "#ffffff", "frame": "#211f1c"
	},
	"font": {"sans": "Inter, system-ui, sans-serif", "mono": "JetBrains Mono, monospace"},
	"radius": {"screen": "24px", "control": "12px", "card": "10px"},
	"space": {"gap": "9px", "pad": "14px"}
}`

// aMockup is a whole flows.json the schema accepts, with the markup of its one screen written in.
func aMockup(markup string) string {
	return fmt.Sprintf(`{
		"readAt": {"commit": "0000000", "date": "2026-09-23"},
		%s,
		"screens": {
			"sign-in": {"name": "Sign in", "surface": "web", "status": "designed", "html": %s}
		},
		"stories": [
			{"id": "sign-in", "title": "A person signs in", "start": "sign-in", "nodes": [["sign-in", 0, 0]]}
		]
	}`, mockupTokens, strconv.Quote(markup))
}

// aMockupWith is the same file with one token group replaced, which is how a mockup comes to be
// drawn in a colour the design system never named.
func aMockupWith(tokens, markup string) string {
	return strings.Replace(aMockup(markup), mockupTokens, tokens, 1)
}

// aShapeListMockup is the same file with its one screen written the way screens used to be
// written. Every shape in it names its component, so what a refusal reads is the form itself.
func aShapeListMockup(elements string) string {
	return fmt.Sprintf(`{
		"readAt": {"commit": "0000000", "date": "2026-09-23"},
		%s,
		"screens": {
			"sign-in": {"name": "Sign in", "surface": "web", "status": "designed", "el": [%s]}
		},
		"stories": [
			{"id": "sign-in", "title": "A person signs in", "start": "sign-in", "nodes": [["sign-in", 0, 0]]}
		]
	}`, mockupTokens, elements)
}

// theNamedScreen breaks no rule: every visible part sits under a component, and it is drawn in
// nothing at all, so a test about one rule is not quietly a test about another.
const theNamedScreen = `<main data-component="Card"><h1>Sign in</h1><p>Your tides, on every machine.</p></main>`

// namedShapes is the old form with nothing else wrong with it.
const namedShapes = `{"t": "h", "component": "Heading", "v": "Sign in"},
	{"t": "btn", "component": "Button", "v": "Sign in", "to": "sign-in"}`

// designedUpToMockups walks a project to the point a mockup may be written: the three stages
// before the mockups are written and approved, and the design system carries the tokens above.
func designedUpToMockups(t *testing.T, s *controlplane.Server, project string) {
	t.Helper()
	designedWithTheSystem(t, s, project, designSystemArtifact)
}

// designedWithTheSystem walks the same three stages, with the design system the caller wants. A
// design system carrying no artifact is a design system written as prose, which names nothing.
func designedWithTheSystem(t *testing.T, s *controlplane.Server, project, system string) {
	t.Helper()
	ctx := context.Background()
	for _, stage := range []struct{ name, artifact string }{
		{store.StageDiscovery, ""},
		{store.StageStories, ""},
		{store.StageDesignSystem, system},
	} {
		if _, err := s.SetDesignStage(ctx, &quaycrewv1.SetDesignStageRequest{
			Project: project, Stage: stage.name, Body: "the " + stage.name + " body", Artifact: stage.artifact,
		}); err != nil {
			t.Fatalf("SetDesignStage %s: %v", stage.name, err)
		}
		if _, err := s.ApproveDesignStage(ctx, &quaycrewv1.ApproveDesignStageRequest{
			Project: project, Stage: stage.name,
		}); err != nil {
			t.Fatalf("ApproveDesignStage %s: %v", stage.name, err)
		}
	}
}

// writeMockups is the call under test, with the artifact the caller wants to try.
func writeMockups(s *controlplane.Server, project, artifact string) (*quaycrewv1.SetDesignStageResponse, error) {
	return s.SetDesignStage(context.Background(), &quaycrewv1.SetDesignStageRequest{
		Project: project, Stage: store.StageMockups, Body: "the screens", Artifact: artifact,
	})
}

// mockupsHeld is what the project holds as its mockups stage, so a test can prove a refusal left
// nothing behind.
func mockupsHeld(t *testing.T, s *controlplane.Server, project string) *quaycrewv1.DesignStage {
	t.Helper()
	listed, err := s.ListDesignStages(context.Background(), &quaycrewv1.ListDesignStagesRequest{Project: project})
	if err != nil {
		t.Fatalf("ListDesignStages: %v", err)
	}
	for _, stage := range listed.GetStages() {
		if stage.GetStage() == store.StageMockups {
			return stage
		}
	}
	return nil
}

// SCREEN-5. There is one way to write a screen, and this is what a session that writes the other
// one is told. It names the screen, because a mockup runs to dozens of them, and it names the
// field to write instead, because a session told only that its file is wrong has nothing to do.
func TestAMockupWrittenAsAListOfShapesIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	_, err := writeMockups(s, project, aShapeListMockup(namedShapes))

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a screen written as a list of shapes answered %v, want InvalidArgument", err)
	}
	said := status.Convert(err).Message()
	for _, want := range []string{"sign-in", "html", "markup"} {
		if !strings.Contains(said, want) {
			t.Errorf("the refusal reads %q, and it has to say %q", said, want)
		}
	}
	if held := mockupsHeld(t, s, project); held != nil {
		t.Fatalf("the refused write left a mockups stage behind, holding %q", held.GetArtifact())
	}
}

// The refusal is the session's to act on, so it comes before the schema. A schema fault names a
// path in a document and says nothing about what to write, and it would be the first thing read.
func TestTheShapeListRefusalIsReadBeforeTheSchema(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	_, err := writeMockups(s, project, aShapeListMockup(namedShapes))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a screen written as a list of shapes answered %v, want InvalidArgument", err)
	}
	said := status.Convert(err).Message()
	if strings.Contains(said, "schema") {
		t.Errorf("the refusal reads %q, and a session reading that goes to the schema rather than to its own file", said)
	}
}

// The other half of the contract. A mockup written as markup is kept, so the check refuses a fault
// rather than refusing the stage.
func TestAMockupThatNamesEveryComponentGoesIn(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	written, err := writeMockups(s, project, aMockup(theNamedScreen))
	if err != nil {
		t.Fatalf("a mockup naming every component was refused: %v", err)
	}
	if written.GetStage().GetArtifact() == "" {
		t.Fatal("the mockups stage came back with no artifact")
	}
	if said := strings.Join(written.GetWarnings(), " "); strings.Contains(said, "design_system") {
		t.Errorf("the write warned %q, and the design system named every value it was drawn in", said)
	}
}

// Two screens, both in the old form. A map is read in whatever order the runtime feels like, so a
// refusal that read one would name a different screen on a different day, and two operators
// comparing notes would disagree about what the file says.
func TestTheShapeListRefusalNamesTheSameScreenEveryTime(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	two := strings.Replace(aShapeListMockup(namedShapes),
		`"screens": {`,
		`"screens": {
			"account": {"name": "Account", "surface": "web", "status": "designed",
				"el": [{"t": "h", "component": "Heading", "v": "Account"}]},`, 1)

	for attempt := range 8 {
		_, err := writeMockups(s, project, two)
		said := status.Convert(err).Message()
		if !strings.Contains(said, `"account"`) {
			t.Fatalf("attempt %d read %q, want the first screen in name order", attempt, said)
		}
	}
}

// The schema is the backstop. A document that is json and is not a flows.json is refused against
// the file the skill tells a session to write from, rather than being kept for somebody to find.
func TestAnArtifactThatIsNotAFlowsFileIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	_, err := writeMockups(s, project, `{"screens": {}}`)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a document that is not a flows.json answered %v, want InvalidArgument", err)
	}
	said := status.Convert(err).Message()
	if !strings.Contains(said, "schema") {
		t.Errorf("the refusal reads %q, and it has to send the reader to the schema", said)
	}
	if !strings.Contains(said, "at /") && !strings.Contains(said, "the whole file") {
		t.Errorf("the refusal reads %q, and it has to say where in the file the fault is", said)
	}
}

// FLOW-4. The tokens are what every screen is drawn in, so a colour the design system never named
// is a second design system nobody approved.
func TestAMockupDrawnInAColourTheDesignSystemDoesNotNameIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	_, err := writeMockups(s, project, aMockupWith(
		strings.Replace(mockupTokens, `"primary": "#1b6b57"`, `"primary": "#ff0000"`, 1), theNamedScreen))

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a colour outside the design system answered %v, want InvalidArgument", err)
	}
	said := status.Convert(err).Message()
	for _, want := range []string{"#ff0000", "primary", "design_system"} {
		if !strings.Contains(said, want) {
			t.Errorf("the refusal reads %q, and it has to say %q", said, want)
		}
	}
}

// FLOW-4, the font half. A font is one value rather than a pattern, so it is compared whole and
// through whatever spacing the writer used.
func TestAMockupDrawnInAFontTheDesignSystemDoesNotNameIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	_, err := writeMockups(s, project, aMockupWith(
		strings.Replace(mockupTokens, `"sans": "Inter, system-ui, sans-serif"`,
			`"sans": "Comic Sans MS, cursive"`, 1), theNamedScreen))

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a font outside the design system answered %v, want InvalidArgument", err)
	}
	if said := status.Convert(err).Message(); !strings.Contains(said, "Comic Sans MS") {
		t.Errorf("the refusal reads %q, and it has to name the font", said)
	}
}

// One value written in two ways is one value. A design system naming "#FDFBF7" and a mockup drawn
// in "#fdfbf7" agree, and a gate that refused them would be refusing a capital letter.
func TestAColourIsTheSameColourInEitherCase(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	upper := strings.Replace(mockupTokens, `"surface": "#fdfbf7"`, `"surface": "#FDFBF7"`, 1)
	if _, err := writeMockups(s, project, aMockupWith(upper, theNamedScreen)); err != nil {
		t.Fatalf("the same colour in capitals was refused: %v", err)
	}
}

// SYSTEM-3. A design system written as prose alone names nothing, so there is nothing to hold the
// screens to. That write used to go through carrying a warning, and a warning holds nothing: the
// screens of that project were drawn in whatever the session chose. It is refused now, and the
// refusal names the stage to write and the field to write in it.
func TestADesignSystemWithNoTokensRefusesTheMockupsWrite(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedWithTheSystem(t, s, project, "")

	written, err := writeMockups(s, project, aMockup(theNamedScreen))

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a mockup written against a design system that names nothing answered %v, "+
			"want InvalidArgument", err)
	}
	said := status.Convert(err).Message()
	for _, want := range []string{"design_system", "tokens"} {
		if !strings.Contains(said, want) {
			t.Errorf("the refusal reads %q, and it has to say %q", said, want)
		}
	}
	// The way off the old behaviour: the write no longer goes through with a warning on it.
	if written != nil {
		t.Errorf("the write answered %v, and a refused write writes nothing", written)
	}
	if held := mockupsHeld(t, s, project); held != nil {
		t.Fatalf("the refused write left a mockups stage behind, holding %q", held.GetArtifact())
	}
}

// Prose first and the page after is how a stage gets written, and a design system that names
// nothing is often a design system nobody has written yet. So the refusal above binds an artifact
// and leaves the prose alone.
func TestAMockupsStageWithProseAloneIsKeptWhileTheDesignSystemNamesNothing(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedWithTheSystem(t, s, project, "")

	if _, err := s.SetDesignStage(context.Background(), &quaycrewv1.SetDesignStageRequest{
		Project: project, Stage: store.StageMockups, Body: "the screens, in words",
	}); err != nil {
		t.Fatalf("a mockups stage carrying prose alone was refused: %v", err)
	}
	if held := mockupsHeld(t, s, project); held == nil {
		t.Fatal("the project holds no mockups stage, and the prose was supposed to be kept")
	}
}

// The mockups stage is the only one this reads. The other five carry whatever json they carry, and
// a check that reached them would refuse the discovery stage for not being a flows.json.
func TestAStageThatIsNotTheMockupsCarriesAnyJSON(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)

	written, err := s.SetDesignStage(context.Background(), &quaycrewv1.SetDesignStageRequest{
		Project: project, Stage: store.StageDiscovery, Body: "what we asked",
		Artifact: `{"asked": ["when does it move"]}`,
	})
	if err != nil {
		t.Fatalf("the discovery stage was refused for not being a flows.json: %v", err)
	}
	if written.GetStage().GetArtifact() == "" {
		t.Fatal("the discovery stage came back with no artifact")
	}
}

// Prose first and the page after is how a stage is written. A mockups stage with nothing to read
// has nothing to refuse.
func TestAMockupsStageWithNoArtifactIsNotRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	if _, err := writeMockups(s, project, ""); err != nil {
		t.Fatalf("a mockups stage carrying prose alone was refused: %v", err)
	}
}

// An artifact that is not json at all is the store's refusal, and it says the artifact is not
// json. Two messages about one fault send the reader two ways, so this check stands aside.
func TestAnArtifactThatIsNotJSONKeepsTheStoresOwnRefusal(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	_, err := writeMockups(s, project, "the flows are over there")
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("an artifact that is not json answered %v, want InvalidArgument", err)
	}
	if said := status.Convert(err).Message(); !strings.Contains(said, "not json") {
		t.Errorf("the refusal reads %q, want the store's own words about json", said)
	}
}

// SCREEN-2. The same contract, read in markup.
//
// A screen may be written as html, and then it has no shape list to carry a component name. The
// name moves onto the markup as data-component. Two rules keep a mockup buildable: a part that can
// be pressed names a component on itself, and every visible part sits under one.

// aMarkupMockup is a whole flows.json whose one screen is written as markup.
func aMarkupMockup(html string) string {
	return fmt.Sprintf(`{
		"readAt": {"commit": "0000000", "date": "2026-09-23"},
		"screens": {
			"sign-in": {"name": "Sign in", "surface": "web", "status": "designed", "html": %s}
		},
		"stories": [
			{"id": "sign-in", "title": "A person signs in", "start": "sign-in", "nodes": [["sign-in", 0, 0]]}
		]
	}`, strconv.Quote(html))
}

// The press is the component. A part that opens another screen and names none leaves the building
// session to guess which component it stands for, which is the fault this whole check exists for.
func TestAPressablePartThatNamesNoComponentIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	_, err := writeMockups(s, project, aMarkupMockup(
		`<main data-component="Card"><button class="go" data-to="sign-in">Continue</button></main>`))

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a part that can be pressed and names no component answered %v, want InvalidArgument", err)
	}
	said := status.Convert(err).Message()
	for _, want := range []string{"sign-in", "button", "go", "Continue", "data-component"} {
		if !strings.Contains(said, want) {
			t.Errorf("the refusal reads %q, and it has to say %q", said, want)
		}
	}
	if held := mockupsHeld(t, s, project); held != nil {
		t.Fatalf("the refused write left a mockups stage behind, holding %q", held.GetArtifact())
	}
}

// The words a person reads are the screen. Words under nothing are words no session can place.
func TestVisibleWordsOutsideEveryNamedComponentAreRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	_, err := writeMockups(s, project, aMarkupMockup(`<main><h1 class="lede">This week</h1></main>`))

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("words outside every named component answered %v, want InvalidArgument", err)
	}
	said := status.Convert(err).Message()
	for _, want := range []string{"sign-in", "h1", "lede", "This week", "data-component"} {
		if !strings.Contains(said, want) {
			t.Errorf("the refusal reads %q, and it has to say %q", said, want)
		}
	}
}

// A mark carries no words, and it is still a part somebody has to build. The four drawing elements
// are visible parts whatever they hold.
func TestAMarkOutsideEveryNamedComponentIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	_, err := writeMockups(s, project, aMarkupMockup(
		`<main data-component="Card"><p data-component="Text">Two tides today</p></main>`+
			`<img class="mark" src="asset:logo" alt="Tide">`))

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a mark outside every named component answered %v, want InvalidArgument", err)
	}
	said := status.Convert(err).Message()
	for _, want := range []string{"sign-in", "img", "mark", "data-component"} {
		if !strings.Contains(said, want) {
			t.Errorf("the refusal reads %q, and it has to say %q", said, want)
		}
	}
}

// The other half of the rule, and the half that keeps the markup readable. A card names itself
// once, and every word and mark inside it needs no attribute of its own.
func TestPartsUnderANamedComponentNeedNoNameOfTheirOwn(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	written, err := writeMockups(s, project, aMarkupMockup(
		`<h1 data-component="Heading">This week</h1>`+
			`<main data-component="Card"><p>Two tides<span>both after dark</span></p>`+
			`<svg viewBox="0 0 8 8"><path d="M0 0h8v8H0z"></path></svg>`+
			`<button data-component="Button" data-to="sign-in">Continue</button></main>`))
	if err != nil {
		t.Fatalf("a screen naming a component above every part was refused: %v", err)
	}
	if written.GetStage().GetArtifact() == "" {
		t.Fatal("the mockups stage came back with no artifact")
	}
}

// A stylesheet is not a part of the screen. Its text is read by the browser and by nobody else, so
// a rule that called it visible words would refuse every screen that carries one.
func TestAStylesheetInAScreenIsNotAVisiblePart(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	if _, err := writeMockups(s, project, aMarkupMockup(
		`<style>h1{margin:0}</style><main data-component="Card"><h1>This week</h1></main>`,
	)); err != nil {
		t.Fatalf("a screen carrying a stylesheet was refused: %v", err)
	}
}

// Spacing is not words. A part holding a space, or the space that does not break, holds nothing a
// person reads and nothing a session has to build.
func TestSpacingAloneIsNotWords(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	if _, err := writeMockups(s, project, aMarkupMockup(
		`<main data-component="Card"><h1>This week</h1></main><div class="gap"> </div><div>  </div>`,
	)); err != nil {
		t.Fatalf("a part holding spacing alone was refused: %v", err)
	}
}

// Two screens, both wrong. A map is read in whatever order the runtime feels like, so a refusal
// that read one would name a different screen on a different day.
func TestTheRefusalNamesTheSamePartEveryTime(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	two := strings.Replace(aMarkupMockup(`<main><h1>This week</h1></main>`),
		`"screens": {`,
		`"screens": {
			"account": {"name": "Account", "surface": "web", "status": "designed",
				"html": "<main><h1>Account</h1></main>"},`, 1)

	for attempt := range 8 {
		_, err := writeMockups(s, project, two)
		said := status.Convert(err).Message()
		if !strings.Contains(said, `"account"`) {
			t.Fatalf("attempt %d read %q, want the first screen in name order", attempt, said)
		}
	}
}

// The form is refused whole. A session that named a component on every shape did the old work
// well, and it still has one thing to do, so the refusal says that one thing rather than passing
// the screen and finding the next fault in it.
func TestAShapeListIsRefusedEvenWhereEveryShapeNamesItsComponent(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	_, err := writeMockups(s, project, aShapeListMockup(namedShapes))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a list of shapes that named every component answered %v, want InvalidArgument", err)
	}
	if said := status.Convert(err).Message(); !strings.Contains(said, "html") {
		t.Errorf("the refusal reads %q, and it has to name the field to write instead", said)
	}
}

// SCREEN-3. A screen is drawn, and it never runs and never calls out.
//
// A session writing markup reaches for the tools it knows: a script from a delivery network, a font
// from a font host, a handler on a button. Measured on the 18 screens of a real project, every one
// of them loaded a script from one host and 14 of them a font from another. The page contains all
// three, because it draws each screen inside a frame with no network and no script of the session's.
// This is the other half: the session is told while it writes, rather than an operator finding a
// screen that draws one thing on a machine with a network and another thing without one.

// aNamedCard is a part of a screen that breaks no other rule, so a test about an address is not
// quietly a test about a component.
const aNamedCard = `<main data-component="Card"><p>Two tides today</p></main>`

// Every place an address hides in a screen, and the addresses a session actually reaches for. The
// refusal repeats the address, because a screen runs to hundreds of lines and an operator told only
// that an address is wrong has to read all of them.
func TestAnAddressOutsideThePageIsRefused(t *testing.T) {
	for _, held := range []struct {
		name    string
		markup  string
		address string
	}{
		{
			name:    "a font in a link",
			markup:  `<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Inter">` + aNamedCard,
			address: "https://fonts.googleapis.com/css2?family=Inter",
		},
		{
			name:    "an image in a src",
			markup:  `<img data-component="Mark" src="https://cdn.example.invalid/logo.png" alt="Tide">`,
			address: "https://cdn.example.invalid/logo.png",
		},
		{
			name: "one candidate of a srcset",
			markup: `<img data-component="Mark" src="asset:logo" alt="Tide"` +
				` srcset="asset:logo 1x, https://cdn.example.invalid/logo-2x.png 2x">`,
			address: "https://cdn.example.invalid/logo-2x.png",
		},
		{
			name:    "an image in the stylesheet of the screen",
			markup:  `<style>.hero{background:url(https://cdn.example.invalid/hero.png)}</style>` + aNamedCard,
			address: "https://cdn.example.invalid/hero.png",
		},
		{
			name: "an image in a style attribute",
			markup: `<main data-component="Card" style="background:url('https://cdn.example.invalid/hero.png')">` +
				`<p>Two tides today</p></main>`,
			address: "https://cdn.example.invalid/hero.png",
		},
		{
			name: "a font the screen fetches itself",
			markup: `<style>@font-face{font-family:Inter;src:url(//cdn.example.invalid/inter.woff2)}</style>` +
				aNamedCard,
			address: "//cdn.example.invalid/inter.woff2",
		},
		{
			name:    "a file beside the page",
			markup:  `<img data-component="Mark" src="./logo.png" alt="Tide">`,
			address: "./logo.png",
		},
		{
			name:    "a data address the page did not write",
			markup:  `<img data-component="Mark" src="data:image/svg+xml;base64,PHN2Zz48L3N2Zz4=" alt="Tide">`,
			address: "data:image/svg+xml",
		},
	} {
		t.Run(held.name, func(t *testing.T) {
			s := newServer(&model.FakeRunner{})
			_, project := newProject(t, s)
			designedUpToMockups(t, s, project)

			_, err := writeMockups(s, project, aMarkupMockup(held.markup))

			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("a screen naming %s answered %v, want InvalidArgument", held.address, err)
			}
			said := status.Convert(err).Message()
			for _, want := range []string{"sign-in", held.address, "design_system"} {
				if !strings.Contains(said, want) {
					t.Errorf("the refusal reads %q, and it has to say %q", said, want)
				}
			}
			if kept := mockupsHeld(t, s, project); kept != nil {
				t.Fatalf("the refused write left a mockups stage behind, holding %q", kept.GetArtifact())
			}
		})
	}
}

// An import is an address whatever it is written as, and it is the shape two of those 18 screens
// reached the font host with. The design system check refuses one in the stylesheet it holds, and a
// screen carrying its own stylesheet is the same fault in a second place.
func TestAnImportInTheStylesheetOfAScreenIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	_, err := writeMockups(s, project, aMarkupMockup(
		`<style>@import "https://fonts.googleapis.com/css2?family=Inter";</style>`+aNamedCard))

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a screen importing a stylesheet answered %v, want InvalidArgument", err)
	}
	said := status.Convert(err).Message()
	for _, want := range []string{"sign-in", "@import", "fonts.googleapis.com", "design_system"} {
		if !strings.Contains(said, want) {
			t.Errorf("the refusal reads %q, and it has to say %q", said, want)
		}
	}
}

// A screen draws. The page gives the frame no script of the session's, so a screen that carries one
// is a screen whose author expected behaviour that is never going to happen.
func TestAScriptInAScreenIsRefused(t *testing.T) {
	for _, markup := range []string{
		`<script src="https://cdn.tailwindcss.com"></script>` + aNamedCard,
		`<script>document.title = "Tide"</script>` + aNamedCard,
	} {
		s := newServer(&model.FakeRunner{})
		_, project := newProject(t, s)
		designedUpToMockups(t, s, project)

		_, err := writeMockups(s, project, aMarkupMockup(markup))

		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("a screen holding a script answered %v, want InvalidArgument", err)
		}
		said := status.Convert(err).Message()
		for _, want := range []string{"sign-in", "script"} {
			if !strings.Contains(said, want) {
				t.Errorf("the refusal reads %q, and it has to say %q", said, want)
			}
		}
	}
}

// The other way behaviour gets written into markup. Seven of those 18 screens carried one.
func TestAnEventAttributeInAScreenIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	_, err := writeMockups(s, project, aMarkupMockup(
		`<main data-component="Card"><button data-component="Button" data-to="sign-in"`+
			` onclick="open()">Continue</button></main>`))

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a screen carrying an event attribute answered %v, want InvalidArgument", err)
	}
	said := status.Convert(err).Message()
	for _, want := range []string{"sign-in", "onclick", "button"} {
		if !strings.Contains(said, want) {
			t.Errorf("the refusal reads %q, and it has to say %q", said, want)
		}
	}
}

// The two addresses a screen may name, and the two that reach nothing. A gate that refused these
// would leave a session no way to draw a mark or to name a part of its own screen.
func TestTheAddressesAScreenMayNameAreKept(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	written, err := writeMockups(s, project, aMarkupMockup(
		`<style>.hero{background:url(asset:hero) no-repeat}</style>`+
			`<img data-component="Mark" src="asset:logo" alt="Tide"`+
			` srcset="asset:logo 1x, asset:logo-2x 2x">`+
			`<main data-component="Card" style="background:url('asset:hero')">`+
			`<a data-component="Link" href="#today">Today</a>`+
			`<a data-component="Link" href="#">Nothing yet</a>`+
			`<a data-component="Link" href="">Nothing yet</a>`+
			`<button data-component="Button" data-to="sign-in">Continue</button></main>`))
	if err != nil {
		t.Fatalf("a screen naming only its own assets and its own parts was refused: %v", err)
	}
	if written.GetStage().GetArtifact() == "" {
		t.Fatal("the mockups stage came back with no artifact")
	}
}

// A screen with both faults reads one of them, and the same one every time. The component rule is
// read first: a part nobody can build from is the fault that reaches furthest, because it survives
// the operator's approval and lands on the session that builds the screen.
func TestTheComponentRuleIsReadBeforeTheAddress(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	_, err := writeMockups(s, project, aMarkupMockup(
		`<main><img class="mark" src="https://cdn.example.invalid/logo.png" alt="Tide"></main>`))

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a screen with both faults answered %v, want InvalidArgument", err)
	}
	if said := status.Convert(err).Message(); !strings.Contains(said, "data-component") {
		t.Errorf("the refusal reads %q, want the component rule read first", said)
	}
}

// Two screens, both calling out. A map is read in whatever order the runtime feels like, so a
// refusal that read one would name a different screen on a different day.
func TestTheRefusalNamesTheFirstScreenThatCallsOut(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	calling := `<img data-component=\"Mark\" src=\"https://cdn.example.invalid/logo.png\" alt=\"Tide\">`
	two := strings.Replace(
		aMarkupMockup(`<img data-component="Mark" src="https://cdn.example.invalid/logo.png" alt="Tide">`),
		`"screens": {`,
		`"screens": {
			"account": {"name": "Account", "surface": "web", "status": "designed",
				"html": "`+calling+`"},`, 1)

	for attempt := range 8 {
		_, err := writeMockups(s, project, two)
		said := status.Convert(err).Message()
		if !strings.Contains(said, `"account"`) {
			t.Fatalf("attempt %d read %q, want the first screen in name order", attempt, said)
		}
	}
}

// A list of shapes is refused before anything inside it is read. A session holding one has one
// thing to do, and a refusal about an address inside a form that no longer exists sends it to
// repair the wrong thing.
func TestAShapeListIsRefusedBeforeAnythingInsideItIsRead(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	_, err := writeMockups(s, project, aShapeListMockup(
		`{"t": "image", "component": "Mark", "v": "https://cdn.example.invalid/logo.png"}`))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a list of shapes answered %v, want InvalidArgument", err)
	}
	said := status.Convert(err).Message()
	if !strings.Contains(said, "html") {
		t.Errorf("the refusal reads %q, and it has to name the field to write instead", said)
	}
	if strings.Contains(said, "cdn.example.invalid") {
		t.Errorf("the refusal reads %q, so it sent the session to an address inside a form that is gone", said)
	}
}

// SCREEN-4. The colours and the fonts of a screen written as markup.
//
// A screen used to carry its own token block, and the check read that. The tokens have one home now,
// the design_system stage, so a colour reaches a screen another way: the stylesheet of the screen,
// the style of one part, or the paint of a mark. Each of those is read, and a colour the operator
// never approved is refused wherever it sits.
//
// The words on a screen are not read. A bill reference of "#dedbee" in a paragraph is what a person
// reads, and a gate that refused it would be refusing words.

// aMarkupMockupOf is a flows.json whose screens are written as markup, by name.
func aMarkupMockupOf(screens map[string]string) string {
	var held []string
	for _, name := range sortedNames(screens) {
		held = append(held, fmt.Sprintf(
			`%s: {"name": %s, "surface": "web", "status": "designed", "html": %s}`,
			strconv.Quote(name), strconv.Quote(name), strconv.Quote(screens[name])))
	}
	return fmt.Sprintf(`{
		"readAt": {"commit": "0000000", "date": "2026-09-23"},
		"screens": {%s},
		"stories": [
			{"id": "walk", "title": "A person walks", "start": %s, "nodes": [[%s, 0, 0]]}
		]
	}`, strings.Join(held, ", "), strconv.Quote(sortedNames(screens)[0]), strconv.Quote(sortedNames(screens)[0]))
}

// sortedNames reads the screens of a test in one order, so a test about the first screen names the
// screen the check names.
func sortedNames(screens map[string]string) []string {
	out := make([]string, 0, len(screens))
	for name := range screens {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// The stylesheet of a screen is the strongest place to hide a colour: it is one element, it is not
// words, and every part under it is painted by it.
func TestAColourWrittenIntoTheStylesheetOfAScreenIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	_, err := writeMockups(s, project, aMarkupMockup(
		`<style>.lede{color:#ff0000}</style><main data-component="Card"><h1 class="lede">This week</h1></main>`))

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a colour in the stylesheet of a screen answered %v, want InvalidArgument", err)
	}
	said := status.Convert(err).Message()
	for _, want := range []string{"sign-in", "#ff0000", "design_system"} {
		if !strings.Contains(said, want) {
			t.Errorf("the refusal reads %q, and it has to say %q", said, want)
		}
	}
	if held := mockupsHeld(t, s, project); held != nil {
		t.Fatalf("the refused write left a mockups stage behind, holding %q", held.GetArtifact())
	}
}

// The style of one part, which is the shortest way to paint something and the easiest to miss.
func TestAColourWrittenIntoTheStyleOfAPartIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	_, err := writeMockups(s, project, aMarkupMockup(
		`<main data-component="Card"><h1 style="background:rgb(255, 0, 0)">This week</h1></main>`))

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a colour in the style of a part answered %v, want InvalidArgument", err)
	}
	said := status.Convert(err).Message()
	for _, want := range []string{"sign-in", "rgb(255, 0, 0)", "design_system"} {
		if !strings.Contains(said, want) {
			t.Errorf("the refusal reads %q, and it has to say %q", said, want)
		}
	}
}

// A mark is written as a drawing in the markup, and a drawing is painted by its own attributes
// rather than by a declaration. So the paint of a drawing is read too.
func TestAColourPaintedOnAMarkIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	_, err := writeMockups(s, project, aMarkupMockup(
		`<main data-component="Mark"><svg viewBox="0 0 8 8"><circle cx="4" cy="4" r="4" fill="#ff0000"/></svg></main>`))

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a colour painted on a mark answered %v, want InvalidArgument", err)
	}
	said := status.Convert(err).Message()
	for _, want := range []string{"sign-in", "#ff0000"} {
		if !strings.Contains(said, want) {
			t.Errorf("the refusal reads %q, and it has to say %q", said, want)
		}
	}
}

// The other half of the rule. A colour the design system names is the design system, written out
// rather than reached through a custom property, and it is the same colour.
func TestAColourTheDesignSystemNamesIsKeptInTheMarkup(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	if _, err := writeMockups(s, project, aMarkupMockup(
		`<style>.lede{color:#1B6B57;background:var(--t-colour-surface)}</style>`+
			`<main data-component="Card"><h1 class="lede">This week</h1></main>`)); err != nil {
		t.Fatalf("a colour the design system names was refused: %v", err)
	}
}

// What a screen says is what a person reads. The words are not a style field, so nothing in them is
// a colour, however much one of them looks like one.
func TestWordsInTheMarkupOfAScreenAreNotColours(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	if _, err := writeMockups(s, project, aMarkupMockup(
		`<main data-component="Card"><p>Your bill reference is #dedbee and it moves on Monday</p></main>`,
	)); err != nil {
		t.Fatalf("prose carrying a hash word was refused as a colour: %v", err)
	}
}

// The typeface half. A screen reaches a font through a token, because the font file travels in the
// design system and nothing else is on the machine that draws it.
func TestAFontFamilyOutsideTheDesignSystemIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	_, err := writeMockups(s, project, aMarkupMockup(
		`<style>h1{font-family:"Comic Sans MS", cursive}</style>`+
			`<main data-component="Card"><h1>This week</h1></main>`))

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a font outside the design system answered %v, want InvalidArgument", err)
	}
	said := status.Convert(err).Message()
	for _, want := range []string{"sign-in", "Comic Sans MS", "design_system"} {
		if !strings.Contains(said, want) {
			t.Errorf("the refusal reads %q, and it has to say %q", said, want)
		}
	}
}

// The two writings a session is told to use: the custom property the page writes for each font
// token, and the family that token names.
func TestAFontFamilyTakenFromTheDesignSystemIsKept(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	for _, family := range []string{
		"var(--t-font-sans)",
		"Inter, system-ui, sans-serif",
		"JetBrains Mono",
	} {
		if _, err := writeMockups(s, project, aMarkupMockup(
			fmt.Sprintf(`<style>h1{font-family:%s}</style>`, family)+
				`<main data-component="Card"><h1>This week</h1></main>`)); err != nil {
			t.Errorf("the font %q comes from the design system and was refused: %v", family, err)
		}
	}
}

// The shorthand puts the family last, after the size and an optional line height. The fixture the
// skill ships writes its fonts that way, so a rule that read the long form alone would read none of
// the screens somebody copies from it.
func TestAFontInTheShorthandOutsideTheDesignSystemIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	_, err := writeMockups(s, project, aMarkupMockup(
		`<style>h1{font:700 40px/1.1 "Comic Sans MS"}</style>`+
			`<main data-component="Card"><h1>This week</h1></main>`))

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a font in the shorthand outside the design system answered %v, want InvalidArgument", err)
	}
	if said := status.Convert(err).Message(); !strings.Contains(said, "Comic Sans MS") {
		t.Errorf("the refusal reads %q, and it has to name the font", said)
	}
}

// The same shorthand, written the way the shipped fixture writes it.
func TestAFontInTheShorthandTakenFromTheDesignSystemIsKept(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	if _, err := writeMockups(s, project, aMarkupMockup(
		`<style>h1{font:600 15px/1 var(--t-font-sans)}td{font:15px/1.4 var(--t-font-mono)}</style>`+
			`<main data-component="Card"><h1>This week</h1></main>`)); err != nil {
		t.Fatalf("the shorthand of the shipped fixture was refused: %v", err)
	}
}

// A part nobody can build from is the fault that reaches furthest, so a screen carrying both faults
// reads that one first. The order is the contract, because an operator fixes what the refusal names.
func TestTheComponentRuleIsReadBeforeTheColourOfTheMarkup(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	_, err := writeMockups(s, project, aMarkupMockup(
		`<style>h1{color:#ff0000}</style><main><h1>This week</h1></main>`))

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a screen carrying both faults answered %v, want InvalidArgument", err)
	}
	said := status.Convert(err).Message()
	if !strings.Contains(said, "data-component") {
		t.Errorf("the refusal reads %q, and the component rule is read first", said)
	}
	if strings.Contains(said, "#ff0000") {
		t.Errorf("the refusal reads %q, and it names one fault at a time", said)
	}
}

// Two reads of one artifact name the same screen. The screens are read in name order, so a mockup
// of twenty screens is fixed one refusal at a time rather than in whatever order a map answers in.
func TestTheRefusalNamesTheFirstScreenDrawnOutsideTheDesignSystem(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	artifact := aMarkupMockupOf(map[string]string{
		"basket":   `<main data-component="Card"><h1 style="color:#ff0000">Basket</h1></main>`,
		"checkout": `<main data-component="Card"><h1 style="color:#00ff00">Checkout</h1></main>`,
	})
	for at := 0; at < 3; at++ {
		_, err := writeMockups(s, project, artifact)
		said := status.Convert(err).Message()
		if !strings.Contains(said, "basket") || !strings.Contains(said, "#ff0000") {
			t.Fatalf("read %d refused with %q, and the first screen in name order is basket", at, said)
		}
	}
}
