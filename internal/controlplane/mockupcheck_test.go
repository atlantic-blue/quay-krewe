package controlplane_test

import (
	"context"
	"fmt"
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
// Both are read at the call, because that is where an operator meets them. A mockup nobody could
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

// aMockup is a whole flows.json the schema accepts, with the shapes of its one screen written in.
func aMockup(elements string) string {
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

// aMockupWith is the same file with one token group replaced, which is how a mockup comes to be
// drawn in a colour the design system never named.
func aMockupWith(tokens, elements string) string {
	return strings.Replace(aMockup(elements), mockupTokens, tokens, 1)
}

const namedShapes = `{"t": "h", "component": "Heading", "v": "Sign in"},
	{"t": "btn", "component": "Button", "v": "Sign in", "to": "sign-in"}`

// designedUpToMockups walks a project to the point a mockup may be written: the three stages
// before the mockups are written and approved, and the design system carries the tokens above.
func designedUpToMockups(t *testing.T, s *controlplane.Server, project string) {
	t.Helper()
	ctx := context.Background()
	for _, stage := range []struct{ name, artifact string }{
		{store.StageDiscovery, ""},
		{store.StageStories, ""},
		{store.StageDesignSystem, designSystemArtifact},
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

// FLOW-3. The refusal this step exists for. It has to name the screen and the shape: a mockup runs
// to dozens of screens, and an operator told only that a component is missing has to read the
// whole file to find out where.
func TestAMockupWithAShapeThatNamesNoComponentIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	_, err := writeMockups(s, project, aMockup(
		`{"t": "h", "component": "Heading", "v": "Sign in"},
		 {"t": "btn", "v": "Sign in"}`))

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a shape with no component answered %v, want InvalidArgument", err)
	}
	said := status.Convert(err).Message()
	for _, want := range []string{"sign-in", "component", "el 1", "btn"} {
		if !strings.Contains(said, want) {
			t.Errorf("the refusal reads %q, and it has to say %q", said, want)
		}
	}
	if held := mockupsHeld(t, s, project); held != nil {
		t.Fatalf("the refused write left a mockups stage behind, holding %q", held.GetArtifact())
	}
}

// An empty component is the same fault as no component at all. A session reads the name to pick a
// component, and there is nothing to read either way.
func TestAMockupWithAnEmptyComponentIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	_, err := writeMockups(s, project, aMockup(`{"t": "btn", "component": "   ", "v": "Sign in"}`))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a shape whose component is spaces answered %v, want InvalidArgument", err)
	}
	if said := status.Convert(err).Message(); !strings.Contains(said, "sign-in") {
		t.Errorf("the refusal reads %q, and it has to name the screen", said)
	}
}

// The other half of the contract. A mockup that names a component on every shape is kept, so the
// check refuses a fault rather than refusing the stage.
func TestAMockupThatNamesEveryComponentGoesIn(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	written, err := writeMockups(s, project, aMockup(namedShapes))
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

// Two screens, both wrong. A map is read in whatever order the runtime feels like, so a refusal
// that read one would name a different screen on a different day and two operators comparing
// notes would disagree about what the file says.
func TestTheRefusalNamesTheSameShapeEveryTime(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	two := strings.Replace(aMockup(`{"t": "btn", "v": "Sign in"}`),
		`"screens": {`,
		`"screens": {
			"account": {"name": "Account", "surface": "web", "status": "designed",
				"el": [{"t": "h", "v": "Account"}]},`, 1)

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
		strings.Replace(mockupTokens, `"primary": "#1b6b57"`, `"primary": "#ff0000"`, 1), namedShapes))

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
			`"sans": "Comic Sans MS, cursive"`, 1), namedShapes))

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
	if _, err := writeMockups(s, project, aMockupWith(upper, namedShapes)); err != nil {
		t.Fatalf("the same colour in capitals was refused: %v", err)
	}
}

// A colour written onto one screen, rather than into the tokens. The refusal names that screen,
// because the fault is on it and not on the file.
func TestAColourWrittenOntoAScreenIsRefused(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	_, err := writeMockups(s, project, aMockup(
		`{"t": "btn", "component": "Button", "v": "Sign in", "style": "background: #ff0000"}`))

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a colour written onto a screen answered %v, want InvalidArgument", err)
	}
	said := status.Convert(err).Message()
	for _, want := range []string{"sign-in", "#ff0000"} {
		if !strings.Contains(said, want) {
			t.Errorf("the refusal reads %q, and it has to say %q", said, want)
		}
	}
}

// What a screen says is what a person reads. A gate that searched every string for something
// shaped like a colour would refuse the words on the screen, which is worse than missing a colour
// nobody wrote in a field meant for one.
func TestWordsOnAScreenAreNotColours(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	if _, err := writeMockups(s, project, aMockup(
		`{"t": "p", "component": "Text", "v": "Your bill reference is #dedbee and it moves on Monday"}`,
	)); err != nil {
		t.Fatalf("prose carrying a hash word was refused as a colour: %v", err)
	}
}

// A design system written as prose alone names nothing, so there is nothing to hold the mockup to.
// The write goes through and says so, because refusing every mockup would block a project whose
// design system is a paragraph.
func TestADesignSystemWithNoTokensTurnsTheColourCheckOffAndSaysSo(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	ctx := context.Background()
	_, project := newProject(t, s)
	for _, stage := range []string{store.StageDiscovery, store.StageStories, store.StageDesignSystem} {
		if _, err := s.SetDesignStage(ctx, &quaycrewv1.SetDesignStageRequest{
			Project: project, Stage: stage, Body: "one accent colour, and plenty of white space",
		}); err != nil {
			t.Fatalf("SetDesignStage %s: %v", stage, err)
		}
		if _, err := s.ApproveDesignStage(ctx, &quaycrewv1.ApproveDesignStageRequest{
			Project: project, Stage: stage,
		}); err != nil {
			t.Fatalf("ApproveDesignStage %s: %v", stage, err)
		}
	}

	written, err := writeMockups(s, project, aMockupWith(
		strings.Replace(mockupTokens, `"primary": "#1b6b57"`, `"primary": "#ff0000"`, 1), namedShapes))
	if err != nil {
		t.Fatalf("a mockup was refused against a design system that names nothing: %v", err)
	}
	said := strings.Join(written.GetWarnings(), " ")
	if !strings.Contains(said, "design_system") {
		t.Errorf("the write said %q, and it has to say the colour check did not run", said)
	}

	// The shape check is not the colour check, and it still holds.
	if _, err := writeMockups(s, project, aMockup(`{"t": "btn", "v": "Sign in"}`)); status.Code(err) !=
		codes.InvalidArgument {
		t.Errorf("a shape with no component answered %v, want it refused whatever the design system names", err)
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

// A screen written as a shape list keeps the refusal it has today, because both shapes are written
// while the format changes over.
func TestAScreenWrittenAsShapesKeepsTheShapeRefusal(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	_, err := writeMockups(s, project, aMockup(`{"t": "btn", "v": "Sign in"}`))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a shape with no component answered %v, want InvalidArgument", err)
	}
	if said := status.Convert(err).Message(); !strings.Contains(said, "shape") {
		t.Errorf("the refusal reads %q, and it has to name the shape it read", said)
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

// A screen written as a shape list is not read for addresses. The shape list is removed with the
// format it belongs to, and a rule that reached into it would refuse a stored mockup nobody can
// rewrite.
func TestAScreenWrittenAsShapesIsNotReadForAddresses(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, project := newProject(t, s)
	designedUpToMockups(t, s, project)

	if _, err := writeMockups(s, project, aMockup(
		`{"t": "image", "component": "Mark", "v": "https://cdn.example.invalid/logo.png"}`)); err != nil {
		t.Fatalf("a screen written as shapes was refused: %v", err)
	}
}
