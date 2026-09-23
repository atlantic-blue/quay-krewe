package features_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	"github.com/atlantic-blue/quay-krewe/internal/controlplane"
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
type siteWorld struct {
	serving *httptest.Server
	status  int
	body    string
	header  http.Header
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
