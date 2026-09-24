package features_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/atlantic-blue/quay-krewe/internal/controlplane"
	"github.com/atlantic-blue/quay-krewe/internal/controlplane/site/render"
	"github.com/atlantic-blue/quay-krewe/internal/store"
	flowmaprender "github.com/atlantic-blue/quay-krewe/skills/flow-map/render"
	"github.com/cucumber/godog"
)

// Steps for the read only http surface.
//
// The site is served over the scenario's own control plane, on a local address of its own, so a step
// reads it the way a page does: one request, one status, one body. The handler comes off the same
// server every other step calls over gRPC, which is what makes the two views of a project one thing.

// composeFile is the compose stack as the suite sees it. The published port is read out of the file
// the operator runs, so a stack that starts publishing the site to the network fails here.
const composeFile = "../deploy/docker-compose.yml"

// siteStageRow is one stage as stages.json hands it over. It is written out here rather than shared
// with the code, because a reader of the page has the json and nothing else: a step that decoded the
// server's own struct would pass on a field that was renamed on the wire.
type siteStageRow struct {
	Stage           string `json:"stage"`
	Position        int    `json:"position"`
	Body            string `json:"body"`
	Artifact        string `json:"artifact"`
	Version         int    `json:"version"`
	ApprovedVersion int    `json:"approved_version"`
	Skipped         bool   `json:"skipped"`
	BodyHTML        string `json:"body_html"`
}

// siteAnswer is the whole of stages.json: what the project is for, and the stages written under it.
type siteAnswer struct {
	Design struct {
		Brief string `json:"brief"`
		Body  string `json:"body"`
	} `json:"design"`
	Stages []siteStageRow `json:"stages"`
}

// siteWorld is the address the site is served on, and what the last request answered.
//
// The page and the address it names for its drawing library are kept as well, because the steps that
// follow a page read the files the page itself loads rather than addresses of their own.
type siteWorld struct {
	serving *httptest.Server
	status  int
	body    string
	header  http.Header
	page    string
	library string
	// mapStage is the stage whose flow map was opened, so the steps after it read that stage.
	mapStage string
}

// siteFile is one file the site handed over, named by the address it came from.
type siteFile struct {
	what string
	body string
}

type siteKey struct{}

func siteFrom(ctx context.Context) *siteWorld {
	s, _ := ctx.Value(siteKey{}).(*siteWorld)
	return s
}

func initializeSiteSteps(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, siteKey{}, &siteWorld{}), nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if s := siteFrom(ctx); s != nil && s.serving != nil {
			s.serving.Close()
		}
		return ctx, nil
	})

	// httptest binds 127.0.0.1 and picks a free port, so the scenario reads the site the way a page
	// reads it, over a real connection rather than through the handler directly.
	sc.Step(`^the site is served on a local address$`, func(ctx context.Context) error {
		siteFrom(ctx).serving = httptest.NewServer(worldFrom(ctx).server.Site())
		return nil
	})

	sc.Step(`^the operator reads the site at "([^"]*)"$`, func(ctx context.Context, path string) error {
		return callSite(ctx, http.MethodGet, path)
	})

	sc.Step(`^the operator sends the site a write at "([^"]*)"$`, func(ctx context.Context, path string) error {
		return callSite(ctx, http.MethodPost, path)
	})

	sc.Step(`^the site answers (\d+)$`, func(ctx context.Context, want int) error {
		s := siteFrom(ctx)
		if s.status != want {
			return fmt.Errorf("the site answered %d saying %q, want %d", s.status, s.body, want)
		}
		return nil
	})

	sc.Step(`^the site answers json$`, func(ctx context.Context) error {
		s := siteFrom(ctx)
		if kind := s.header.Get("Content-Type"); !strings.Contains(kind, "application/json") {
			return fmt.Errorf("the site answered with content type %q, and a page reads json", kind)
		}
		if !json.Valid([]byte(s.body)) {
			return fmt.Errorf("the site answered %q, which is not json a reader can parse", s.body)
		}
		return nil
	})

	sc.Step(`^the site says it answers GET and HEAD$`, func(ctx context.Context) error {
		allowed := siteFrom(ctx).header.Get("Allow")
		if !strings.Contains(allowed, "GET") || !strings.Contains(allowed, "HEAD") {
			return fmt.Errorf("the refusal allows %q, and a caller cannot tell what the address answers", allowed)
		}
		return nil
	})

	sc.Step(`^the answer says "([^"]*)"$`, func(ctx context.Context, want string) error {
		if said := siteFrom(ctx).body; !strings.Contains(said, want) {
			return fmt.Errorf("the site said %q, and it never named %q", said, want)
		}
		return nil
	})

	sc.Step(`^the answer carries the brief "([^"]*)"$`, func(ctx context.Context, want string) error {
		answer, err := siteStages(ctx)
		if err != nil {
			return err
		}
		if answer.Design.Brief != want {
			return fmt.Errorf("the answer carries the brief %q, want %q", answer.Design.Brief, want)
		}
		return nil
	})

	// Written and agreed, which is the state the page draws as approved: a version on the row, and the
	// operator's word on that same version.
	sc.Step(`^the answer carries the "([^"]*)" stage, written and approved at the same version$`,
		func(ctx context.Context, stage string) error {
			held, err := siteStage(ctx, stage)
			if err != nil {
				return err
			}
			if held.Version == 0 {
				return fmt.Errorf("the %s stage comes back at version 0, so the answer says nobody wrote it", stage)
			}
			if held.ApprovedVersion != held.Version {
				return fmt.Errorf("the %s stage is at version %d and approved at version %d",
					stage, held.Version, held.ApprovedVersion)
			}
			return nil
		})

	sc.Step(`^the answer carries the "([^"]*)" stage at version (\d+), approved at version (\d+)$`,
		func(ctx context.Context, stage string, version, approved int) error {
			held, err := siteStage(ctx, stage)
			if err != nil {
				return err
			}
			if held.Version != version || held.ApprovedVersion != approved {
				return fmt.Errorf("the %s stage is at version %d approved at version %d, want %d and %d",
					stage, held.Version, held.ApprovedVersion, version, approved)
			}
			return nil
		})

	sc.Step(`^the answer says the "([^"]*)" stage reads "([^"]*)"$`,
		func(ctx context.Context, stage, want string) error {
			held, err := siteStage(ctx, stage)
			if err != nil {
				return err
			}
			if held.Body != want {
				return fmt.Errorf("the %s stage reads %q in the answer, want %q", stage, held.Body, want)
			}
			return nil
		})

	// What it means rather than how it is spelled, because a store is free to hand back its own
	// spacing for a json document it parsed.
	sc.Step(`^the answer is the artifact meaning:$`, func(ctx context.Context, want *godog.DocString) error {
		read, wanted := new(bytes.Buffer), new(bytes.Buffer)
		if err := json.Compact(read, []byte(siteFrom(ctx).body)); err != nil {
			return fmt.Errorf("the site answered %q, which is not json: %w", siteFrom(ctx).body, err)
		}
		if err := json.Compact(wanted, []byte(want.Content)); err != nil {
			return fmt.Errorf("the scenario asks for %q, which is not json: %w", want.Content, err)
		}
		if read.String() != wanted.String() {
			return fmt.Errorf("the site answered %s, want %s", read, wanted)
		}
		return nil
	})

	// The page itself, rather than the documents under it. A page that named a script somewhere else
	// would work on the machine that wrote it and nowhere else, so the two names are read out of the
	// markup the site served.
	sc.Step(`^the page names its stylesheet and its script$`, func(ctx context.Context) error {
		body := siteFrom(ctx).body
		for _, want := range []string{"/assets/site.css", "/assets/site.js"} {
			if !strings.Contains(body, want) {
				return fmt.Errorf("the page never names %q, so the operator gets markup and nothing else", want)
			}
		}
		return nil
	})

	// The menu, drawn by the page's own renderer over the document the site just answered. It runs
	// outside a browser, so a scenario reads the words an operator reads.
	sc.Step(`^the menu drawn from that answer reads "([^"]*)" for the "([^"]*)" stage$`,
		func(ctx context.Context, want, stage string) error {
			drawn, err := siteMenu(ctx)
			if err != nil {
				return err
			}
			held, found := menuState(drawn, stage)
			if !found {
				return fmt.Errorf("the menu carries no entry for the %s stage: %s", stage, drawn)
			}
			if held != want {
				return fmt.Errorf("the menu reads %q for the %s stage, want %q", held, stage, want)
			}
			return nil
		})

	sc.Step(`^the menu drawn from that answer lists Design and then the six stages in order$`,
		func(ctx context.Context) error {
			drawn, err := siteMenu(ctx)
			if err != nil {
				return err
			}
			var listed []string
			for _, found := range menuEntry.FindAllStringSubmatch(drawn, -1) {
				listed = append(listed, found[1])
			}
			want := append([]string{"design"}, store.DesignStages()...)
			if strings.Join(listed, ",") != strings.Join(want, ",") {
				return fmt.Errorf("the menu lists %v, want %v", listed, want)
			}
			return nil
		})

	// The body as a document. What is read is the html the site answered, because that is what the
	// operator's browser is handed: a body that reached the page as marks would read as one long line.
	sc.Step(`^the "([^"]*)" stage reads as a document, with its heading, its list and its code$`,
		func(ctx context.Context, stage string) error {
			held, err := siteStage(ctx, stage)
			if err != nil {
				return err
			}
			for _, want := range []struct {
				what string
				mark *regexp.Regexp
			}{
				{"heading", regexp.MustCompile(`<h[12][ >]`)},
				{"list", regexp.MustCompile(`<[ou]l[ >]`)},
				{"second line of the list", regexp.MustCompile(`<li>the water moves</li>`)},
				{"code in a block of its own", regexp.MustCompile(`(?s)<pre[ >].*fmt\.Println`)},
			} {
				if !want.mark.MatchString(held.BodyHTML) {
					return fmt.Errorf("the %s stage carries no %s: it reads %q", stage, want.what, held.BodyHTML)
				}
			}
			return nil
		})

	// The other half of reading a body: everything a browser would run is gone, whatever the session
	// wrote. An element that runs, and an attribute that runs when a picture fails to load.
	sc.Step(`^the "([^"]*)" stage runs nothing in the operator's browser$`,
		func(ctx context.Context, stage string) error {
			held, err := siteStage(ctx, stage)
			if err != nil {
				return err
			}
			if strings.Contains(strings.ToLower(held.BodyHTML), "<script") {
				return fmt.Errorf("the %s stage carries a script element: %q", stage, held.BodyHTML)
			}
			if found := eventAttribute.FindString(held.BodyHTML); found != "" {
				return fmt.Errorf("the %s stage carries the event attribute %q: %q", stage, found, held.BodyHTML)
			}
			return nil
		})

	// The page's own renderer, run over the document the site answered, so what this reads is what an
	// operator sees rather than what the answer holds.
	sc.Step(`^the page shows the operator that document$`, func(ctx context.Context) error {
		drawn, err := siteBody(ctx, store.StageDiscovery)
		if err != nil {
			return err
		}
		if !strings.Contains(drawn, "<li>the water moves</li>") {
			return fmt.Errorf("the page shows the marks rather than the document: %s", drawn)
		}
		if strings.Contains(strings.ToLower(drawn), "<script") {
			return fmt.Errorf("the page carries a script element, so what a session wrote would run: %s", drawn)
		}
		return nil
	})

	sc.Step(`^the site's default address is "([^"]*)"$`, func(want string) error {
		if controlplane.SiteAddr != want {
			return fmt.Errorf("the site binds %q by default, want %q", controlplane.SiteAddr, want)
		}
		return nil
	})

	sc.Step(`^the compose stack publishes the site as "([^"]*)"$`, func(want string) error {
		return composeSays(`- "` + want + `"`)
	})

	sc.Step(`^the compose stack tells the control plane to bind "([^"]*)"$`, func(want string) error {
		return composeSays(`QC_SITE_ADDR: "` + want + `"`)
	})
	// The picture, at the point the page receives it. A diagram arrives as the element the drawing
	// library looks for, holding the source a session wrote, so the page has something to draw and
	// the operator is not reading a flowchart as prose.
	sc.Step(`^the "([^"]*)" stage arrives as a diagram ready to draw$`, func(ctx context.Context, stage string) error {
		held, err := siteStage(ctx, stage)
		if err != nil {
			return err
		}
		if !strings.Contains(held.BodyHTML, diagramElement) {
			return fmt.Errorf("the %s stage carries no %s element: it reads %q", stage, diagramElement, held.BodyHTML)
		}
		if !strings.Contains(held.BodyHTML, "flowchart") {
			return fmt.Errorf("the %s stage carries an empty diagram: it reads %q", stage, held.BodyHTML)
		}
		return nil
	})

	// Where the library comes from. The address is kept, so the step after this one reads the file the
	// page itself names rather than an address a scenario made up.
	sc.Step(`^the page loads the drawing library from the site itself$`, func(ctx context.Context) error {
		s := siteFrom(ctx)
		s.page = s.body
		found := drawingLibrary.FindStringSubmatch(s.page)
		if found == nil {
			return fmt.Errorf("the page loads no drawing library, so a diagram stays as text: %s", s.page)
		}
		s.library = found[1]
		if !strings.HasPrefix(s.library, "/assets/") {
			return fmt.Errorf("the page loads its drawing library from %q, and the site has to serve it", s.library)
		}
		return nil
	})

	sc.Step(`^the operator reads the drawing library the page names$`, func(ctx context.Context) error {
		s := siteFrom(ctx)
		if s.library == "" {
			return fmt.Errorf("the page named no drawing library, so there is nothing to read")
		}
		return callSite(ctx, http.MethodGet, s.library)
	})

	sc.Step(`^the site answers a script$`, func(ctx context.Context) error {
		s := siteFrom(ctx)
		if kind := s.header.Get("Content-Type"); !strings.Contains(kind, "javascript") {
			return fmt.Errorf("the site answered content type %q, and a browser runs a script", kind)
		}
		if len(s.body) == 0 {
			return fmt.Errorf("the site answered an empty file, and an empty library draws nothing")
		}
		return nil
	})

	// The whole point of holding the library here. Every file the page loads is read, and none of them
	// reaches off the machine, so a design opened on a machine with no network draws, and a private
	// design is never announced to anybody.
	//
	// The library is read for its status and skipped for its text: it carries the addresses of the
	// projects it came from in its own source, and a comment inside a file nobody fetches is not a
	// call anywhere.
	sc.Step(`^no file the site hands the operator reaches an address off the machine$`, func(ctx context.Context) error {
		s := siteFrom(ctx)
		if s.page == "" {
			return fmt.Errorf("no page was read, so there is nothing to follow")
		}
		named := pageNames.FindAllStringSubmatch(s.page, -1)
		if len(named) == 0 {
			return fmt.Errorf("the page loads no file at all: %s", s.page)
		}
		read := []siteFile{{what: "the page", body: s.page}}
		for _, one := range named {
			address := one[1]
			if !strings.HasPrefix(address, "/") {
				return fmt.Errorf("the page loads %q, which is not an address on this site", address)
			}
			if err := callSite(ctx, http.MethodGet, address); err != nil {
				return err
			}
			if s.status != http.StatusOK {
				return fmt.Errorf("the site answered %d for %s, which the page loads", s.status, address)
			}
			if address == s.library {
				continue
			}
			read = append(read, siteFile{what: address, body: s.body})
		}
		for _, one := range read {
			if found := addressOffTheMachine.FindString(one.body); found != "" {
				return fmt.Errorf("%s reaches %q, so opening this project would leave the machine",
					one.what, found)
			}
		}
		return nil
	})

	// The flow map, opened the way an operator opens it: at the stage whose screens it plays.
	sc.Step(`^the operator opens the flow map of the "([^"]*)" stage$`,
		func(ctx context.Context, stage string) error {
			siteFrom(ctx).mapStage = stage
			return callSite(ctx, http.MethodGet, flowMapAddress(ctx, stage))
		})

	// The same address with its last slash left off, which is how a person types it. The answer is
	// read without following it, because the answer is the thing being read.
	sc.Step(`^the operator opens the flow map of the "([^"]*)" stage without its last slash$`,
		func(ctx context.Context, stage string) error {
			siteFrom(ctx).mapStage = stage
			return callSiteWithoutFollowing(ctx, strings.TrimSuffix(flowMapAddress(ctx, stage), "/"))
		})

	sc.Step(`^the site sends the operator to the flow map of the "([^"]*)" stage$`,
		func(ctx context.Context, stage string) error {
			s := siteFrom(ctx)
			if s.status < 300 || s.status > 399 {
				return fmt.Errorf("the site answered %d saying %q, and a person who left the slash off "+
					"has to be sent to the address that plays", s.status, s.body)
			}
			want := flowMapAddress(ctx, stage)
			sent := s.header.Get("Location")
			if !strings.HasSuffix(sent, want) {
				return fmt.Errorf("the site sent the operator to %q, want %q", sent, want)
			}
			return nil
		})

	// The page itself. It is the skill's own page, served out of the binary rather than copied
	// somewhere, and it carries the control a person presses to play a story.
	sc.Step(`^the page plays the project's stories$`, func(ctx context.Context) error {
		s := siteFrom(ctx)
		s.page = s.body
		held, err := theFlowMapPage()
		if err != nil {
			return err
		}
		if s.page != held {
			return fmt.Errorf("the site answered %d bytes, and the flow map the skill ships is %d bytes, "+
				"so the operator is playing something else", len(s.page), len(held))
		}
		if !strings.Contains(s.page, playControl) {
			return fmt.Errorf("the page carries no %q, so there is nothing to play a story with", playControl)
		}
		return nil
	})

	// What the page asks for, followed from the page rather than from an address a scenario made up.
	// The address is resolved against the one the page was read at, the way a browser resolves it.
	sc.Step(`^the operator reads the screens the page asks for$`, func(ctx context.Context) error {
		s := siteFrom(ctx)
		if s.page == "" {
			return fmt.Errorf("no page was read, so there is nothing to follow")
		}
		found := screensThePageAsksFor.FindStringSubmatch(s.page)
		if found == nil {
			return fmt.Errorf("the page asks for no screens at all, so it draws nothing")
		}
		at, err := url.Parse(flowMapAddress(ctx, s.mapStage))
		if err != nil {
			return err
		}
		asked, err := url.Parse(found[1])
		if err != nil {
			return fmt.Errorf("the page asks for %q, which is not an address: %w", found[1], err)
		}
		if asked.IsAbs() {
			return fmt.Errorf("the page asks for %q, which is off this machine", found[1])
		}
		return callSite(ctx, http.MethodGet, at.ResolveReference(asked).String())
	})

	// The bytes, held against what the session wrote. The stage is read over the wire, so what the
	// page is handed and what the session stored are the same screens.
	sc.Step(`^the screens are the ones the session wrote$`, func(ctx context.Context) error {
		s := siteFrom(ctx)
		if err := readStages(ctx); err != nil {
			return err
		}
		held := stageNamed(stagesFrom(ctx).stages, s.mapStage)
		if held == nil {
			return fmt.Errorf("the project holds no %s stage, so nothing was written to read back", s.mapStage)
		}
		wrote, read := new(bytes.Buffer), new(bytes.Buffer)
		if err := json.Compact(wrote, []byte(held.GetArtifact())); err != nil {
			return fmt.Errorf("the %s stage holds %q, which is not json: %w", s.mapStage, held.GetArtifact(), err)
		}
		if err := json.Compact(read, []byte(s.body)); err != nil {
			return fmt.Errorf("the page was handed %q, which is not json: %w", s.body, err)
		}
		if wrote.String() != read.String() {
			return fmt.Errorf("the page was handed %s, and the session wrote %s", read, wrote)
		}
		return nil
	})

	// The stage as the page draws it: a stage holding screens opens the flow map on them, and one
	// holding none does not, so the operator presses nothing that leads nowhere.
	sc.Step(`^the page opens the flow map on the "([^"]*)" stage$`, func(ctx context.Context, stage string) error {
		drawn, err := siteBody(ctx, stage)
		if err != nil {
			return err
		}
		found := theFlowMapFrame.FindStringSubmatch(drawn)
		if found == nil {
			return fmt.Errorf("the %s stage opens no flow map: %s", stage, drawn)
		}
		if want := stage + "/map/"; found[1] != want {
			return fmt.Errorf("the %s stage opens the flow map at %q, want %q", stage, found[1], want)
		}
		return nil
	})

	sc.Step(`^the page opens no flow map on the "([^"]*)" stage$`, func(ctx context.Context, stage string) error {
		drawn, err := siteBody(ctx, stage)
		if err != nil {
			return err
		}
		if found := theFlowMapFrame.FindStringSubmatch(drawn); found != nil {
			return fmt.Errorf("the %s stage holds no screens and opens a flow map at %q anyway",
				stage, found[1])
		}
		return nil
	})

	// The design system, as a picture rather than as a list. Every colour the session named is on the
	// page, drawn in that colour, with its name and its value beside it.
	sc.Step(`^the design system stage draws a swatch for every colour the project names, with its value$`,
		func(ctx context.Context) error {
			return theProjectDrawsItsTokens(ctx, "colour")
		})

	sc.Step(`^the design system stage draws every font, radius and space the project names, with its value$`,
		func(ctx context.Context) error {
			for _, group := range []string{"font", "radius", "space"} {
				if err := theProjectDrawsItsTokens(ctx, group); err != nil {
					return err
				}
			}
			return nil
		})

	// A colour the design system never named is a colour nobody agreed to. The page holds none of its
	// own, so what the operator approves is what the session wrote.
	sc.Step(`^the design system stage draws no colour and no font the project does not name$`,
		func(ctx context.Context) error {
			drawn, err := tokensTheSiteDrew(ctx)
			if err != nil {
				return err
			}
			for _, group := range []string{"colour", "font"} {
				named, err := theProjectsTokens(ctx, group)
				if err != nil {
					return err
				}
				for name, token := range drawn[group] {
					if _, held := named[name]; !held {
						return fmt.Errorf("the page draws a %s called %q as %q, and the design system names no such %s",
							group, name, token.value, group)
					}
				}
				if len(drawn[group]) != len(named) {
					return fmt.Errorf("the design system names %d %s tokens and the page draws %d",
						len(named), group, len(drawn[group]))
				}
			}
			return nil
		})
}

// playControl is the control a person presses to play a story. A page without it draws the screens
// and plays nothing, which is the half of the flow map the mockups stage exists for.
const playControl = `id="m-play"`

// screensThePageAsksFor reads the address the flow map asks its screens for, out of the page itself.
// The address is followed rather than written down here, so a page that starts asking somewhere else
// fails this rather than passing against an address a scenario invented.
var screensThePageAsksFor = regexp.MustCompile(`fetch\("([^"]+)"\)`)

// theFlowMapFrame is the frame one stage opens the flow map in, and the address it opens.
var theFlowMapFrame = regexp.MustCompile(`<iframe[^>]+src="([^"]*)"`)

// flowMapAddress is where a stage's flow map is read, built from the names the operator typed rather
// than from the identifiers underneath them.
func flowMapAddress(ctx context.Context, stage string) string {
	w := worldFrom(ctx)
	return "/p/" + w.workspaceName + "/" + w.projectName + "/" + stage + "/map/"
}

// theFlowMapPage is the page the skill ships, read off disk. The site serves this file, so a copy
// that drifted from it is a copy, and holding the two together is what keeps the page in one home.
func theFlowMapPage() (string, error) {
	dir, err := flowmaprender.Dir()
	if err != nil {
		return "", err
	}
	body, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		return "", fmt.Errorf("reading the flow map the skill ships: %w", err)
	}
	return string(body), nil
}

// callSiteWithoutFollowing makes one request and stops where the site sends it, because a redirect
// followed is a redirect nobody read.
func callSiteWithoutFollowing(ctx context.Context, path string) error {
	s := siteFrom(ctx)
	if s.serving == nil {
		return fmt.Errorf("the site is not being served, so there is nothing to read at %s", path)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.serving.URL+path, nil)
	if err != nil {
		return err
	}
	client := *s.serving.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	answer, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	defer func() { _ = answer.Body.Close() }()
	body, err := io.ReadAll(answer.Body)
	if err != nil {
		return fmt.Errorf("reading the body of %s: %w", path, err)
	}
	s.status, s.body, s.header = answer.StatusCode, string(body), answer.Header
	return nil
}

// callSite makes one request and keeps what came back, so the steps after it read a status, a header
// and a body rather than each making a request of its own.
func callSite(ctx context.Context, method, path string) error {
	s := siteFrom(ctx)
	if s.serving == nil {
		return fmt.Errorf("the site is not being served, so there is nothing to read at %s", path)
	}
	request, err := http.NewRequestWithContext(ctx, method, s.serving.URL+path, nil)
	if err != nil {
		return err
	}
	answer, err := s.serving.Client().Do(request)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	defer func() { _ = answer.Body.Close() }()
	body, err := io.ReadAll(answer.Body)
	if err != nil {
		return fmt.Errorf("reading the body of %s: %w", path, err)
	}
	s.status, s.body, s.header = answer.StatusCode, string(body), answer.Header
	return nil
}

func siteStages(ctx context.Context) (*siteAnswer, error) {
	var answer siteAnswer
	if err := json.Unmarshal([]byte(siteFrom(ctx).body), &answer); err != nil {
		return nil, fmt.Errorf("the site answered %q, which is not the stages a page reads: %w",
			siteFrom(ctx).body, err)
	}
	return &answer, nil
}

func siteStage(ctx context.Context, stage string) (siteStageRow, error) {
	answer, err := siteStages(ctx)
	if err != nil {
		return siteStageRow{}, err
	}
	var held []string
	for _, one := range answer.Stages {
		if one.Stage == stage {
			return one, nil
		}
		held = append(held, one.Stage)
	}
	return siteStageRow{}, fmt.Errorf("the answer carries no %s stage: it carries %v", stage, held)
}

// composeSays holds one line of the compose file, which is the file the operator runs rather than a
// description of it.
func composeSays(want string) error {
	contents, err := os.ReadFile(composeFile)
	if err != nil {
		return fmt.Errorf("reading the compose stack: %w", err)
	}
	if !strings.Contains(string(contents), want) {
		return fmt.Errorf("the compose stack never says %q", want)
	}
	return nil
}

// menuEntry reads one entry out of the drawn menu: which entry it is, and the state it carries.
var menuEntry = regexp.MustCompile(`data-entry="([^"]*)"[^>]*data-state="([^"]*)"`)

// siteMenu draws the menu over the document the site last answered, with the page's own renderer.
// The page draws it in a browser and this draws it here, and both read the same block of site.js.
func siteMenu(ctx context.Context) (string, error) {
	body := siteFrom(ctx).body
	if !json.Valid([]byte(body)) {
		return "", fmt.Errorf("the site last answered %q, and a menu is drawn over stages.json", body)
	}
	script, err := render.ReadSite()
	if err != nil {
		return "", fmt.Errorf("reading the page's renderer: %w", err)
	}
	drawn, err := script.Menu(body)
	if err != nil {
		return "", fmt.Errorf("drawing the menu: %w", err)
	}
	return drawn, nil
}

// diagramElement is what a mermaid block arrives as, and what the drawing library looks for.
const diagramElement = `<pre class="mermaid">`

// drawingLibrary is the script tag that loads the library which draws the diagrams.
var drawingLibrary = regexp.MustCompile(`<script[^>]+src="([^"]*mermaid[^"]*)"`)

// pageNames is every file the page loads: its stylesheet, its script and its drawing library.
var pageNames = regexp.MustCompile(`(?:src|href)="([^"]+)"`)

// addressOffTheMachine is a request that leaves the machine the control plane runs on.
var addressOffTheMachine = regexp.MustCompile(`https?://`)

// eventAttribute is an attribute a browser runs: on, a word and an equals sign, inside a tag.
var eventAttribute = regexp.MustCompile(`(?i)<[^>]*\son[a-z]+\s*=`)

// siteBody draws what one entry of the menu shows, with the page's own renderer, over the document
// the site last answered.
func siteBody(ctx context.Context, entry string) (string, error) {
	body := siteFrom(ctx).body
	if !json.Valid([]byte(body)) {
		return "", fmt.Errorf("the site last answered %q, and a body is drawn over stages.json", body)
	}
	script, err := render.ReadSite()
	if err != nil {
		return "", fmt.Errorf("reading the page's renderer: %w", err)
	}
	drawn, err := script.Body(body, entry)
	if err != nil {
		return "", fmt.Errorf("drawing the body: %w", err)
	}
	return drawn, nil
}

// menuState is the word one entry of the menu shows, read from the words a person reads.
func menuState(drawn, entry string) (string, bool) {
	mark := `data-entry="` + entry + `"`
	from := strings.Index(drawn, mark)
	if from < 0 {
		return "", false
	}
	rest := drawn[from:]
	if to := strings.Index(rest, "</button>"); to >= 0 {
		rest = rest[:to]
	}
	found := regexp.MustCompile(`<span class="state">([^<]*)</span>`).FindStringSubmatch(rest)
	if found == nil {
		return "", false
	}
	return found[1], true
}

// The tokens of a design system, as the page draws them and as the session wrote them.
//
// Both halves are read rather than written down here. The page is read through its own renderer, and
// the design system is read back out of the artifact the scenario wrote, so a page holding a palette
// of its own fails these rather than passing against a list a scenario invented.

// siteTokenTypes is the style property each group is drawn with. A colour reaches the operator as the
// colour, a font as a line set in it, a radius as a rounded corner, a space as a bar of that width.
var siteTokenTypes = map[string]string{
	"colour": "background",
	"font":   "font-family",
	"radius": "border-radius",
	"space":  "width",
}

// siteTokenBlock is one token as the page drew it: its group, its name, and everything drawn for it.
var siteTokenBlock = regexp.MustCompile(`(?s)<li class="token" data-group="([^"]*)" data-token="([^"]*)">(.*?)</li>`)

// siteTokenValue is the value one token shows a reader.
var siteTokenValue = regexp.MustCompile(`<code class="value">([^<]*)</code>`)

// siteToken is one token of the view: what it shows, and the markup it was drawn with.
type siteToken struct {
	value  string
	markup string
}

// tokensTheSiteDrew is every token on the design system stage, by group and by name.
func tokensTheSiteDrew(ctx context.Context) (map[string]map[string]siteToken, error) {
	drawn, err := siteBody(ctx, store.StageDesignSystem)
	if err != nil {
		return nil, err
	}
	held := map[string]map[string]siteToken{}
	for _, found := range siteTokenBlock.FindAllStringSubmatch(drawn, -1) {
		group, name, markup := found[1], found[2], found[3]
		if held[group] == nil {
			held[group] = map[string]siteToken{}
		}
		value := ""
		if shown := siteTokenValue.FindStringSubmatch(markup); shown != nil {
			value = shown[1]
		}
		held[group][name] = siteToken{value: value, markup: markup}
	}
	return held, nil
}

// theProjectsTokens is one group of the design system the scenario wrote.
func theProjectsTokens(ctx context.Context, group string) (map[string]string, error) {
	held := mockupsFrom(ctx)
	if held == nil || held.fixture == nil {
		return nil, fmt.Errorf("no design system was written, so there are no %s tokens to draw", group)
	}
	tokens, written := held.fixture["tokens"].(map[string]any)
	if !written {
		return nil, fmt.Errorf("the design system names no tokens at all")
	}
	values, named := tokens[group].(map[string]any)
	if !named || len(values) == 0 {
		return nil, fmt.Errorf("the design system names no %s, so this step would prove nothing", group)
	}
	out := make(map[string]string, len(values))
	for name, value := range values {
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("the %s called %q is not written as a value", group, name)
		}
		out[name] = text
	}
	return out, nil
}

// theProjectDrawsItsTokens holds the page to one group: every name the design system gives, with its
// value on the page, and drawn as the thing it is.
func theProjectDrawsItsTokens(ctx context.Context, group string) error {
	named, err := theProjectsTokens(ctx, group)
	if err != nil {
		return err
	}
	drawn, err := tokensTheSiteDrew(ctx)
	if err != nil {
		return err
	}
	for name, value := range named {
		token, found := drawn[group][name]
		if !found {
			return fmt.Errorf("the design system names the %s %q and the page draws no such %s",
				group, name, group)
		}
		if token.value != value {
			return fmt.Errorf("the %s %q shows %q, and the design system names %q",
				group, name, token.value, value)
		}
		want := `style="` + siteTokenTypes[group] + ":" + value + `"`
		if !strings.Contains(token.markup, want) {
			return fmt.Errorf("the %s %q is never drawn as %s, so the operator reads the value and never sees it",
				group, name, want)
		}
	}
	return nil
}
