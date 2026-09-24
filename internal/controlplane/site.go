package controlplane

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/store"
)

// The read only http surface: a project's design stages at an address, so the operator approves a
// stage by looking at it rather than by reading a terminal.
//
// It carries no token. A token guards a caller and this surface has no caller to guard: it writes
// nothing, it approves nothing, and every document on it is one the operator already holds. So the
// boundary is the address it binds, which is the machine the control plane runs on and nothing wider.
//
// The routes take the names a person types rather than the identifiers a listing prints, because the
// address is meant to be typed. Either reaches the same project, the way a session address does.

// SiteAddr is where the site listens unless the operator says otherwise. Loopback, for the reason the
// gRPC address is loopback: the port is the whole system, and publishing it wider is a decision
// somebody makes with transport they own. In a container loopback is the container, so the compose
// stack overrides this and holds the host side to loopback instead.
const SiteAddr = "127.0.0.1:50052"

// siteDesign is what the project is for, and the design written for it.
type siteDesign struct {
	Brief string `json:"brief"`
	Body  string `json:"body"`
}

// siteStage is one design stage as a page reads it.
//
// The seven fields a reader acts on, and no more. The version and the approved version are both here
// because the difference between them is a state of its own: a stage that changed after the operator
// agreed to it is neither approved nor unwritten.
type siteStage struct {
	Stage           string `json:"stage"`
	Position        int32  `json:"position"`
	Body            string `json:"body"`
	Artifact        string `json:"artifact"`
	Version         int32  `json:"version"`
	ApprovedVersion int32  `json:"approved_version"`
	Skipped         bool   `json:"skipped"`
}

// siteAnswer is the whole of stages.json.
type siteAnswer struct {
	Design siteDesign  `json:"design"`
	Stages []siteStage `json:"stages"`
}

// Site is the http surface, ready to be served on a listener.
//
// It is handed back rather than served here so a test drives the same handler the operator reads,
// and so the process that owns the listener owns its shutdown too.
//
// The mux answers a method the routes do not carry: a path that matches with the wrong method is a
// 405 with the methods it does answer, which is the whole refusal this surface needs, and HEAD comes
// with GET.
func (s *Server) Site() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /p/{workspace}/{project}/{$}", s.servePage)
	mux.HandleFunc("GET /assets/{file}", serveAsset)
	mux.HandleFunc("GET /p/{workspace}/{project}/stages.json", s.serveStages)
	mux.HandleFunc("GET /p/{workspace}/{project}/{stage}/flows.json", s.serveFlows)
	return mux
}

// The page itself, and the two files it loads. They are embedded rather than read from disk because
// the control plane is installed as one binary: a page beside the executable is a page that is there
// on the machine it was built on and nowhere else.
//
// Named one by one rather than as a directory: the renderer that proves this page is a Go package in
// a directory below, and a pattern over the whole tree would carry its source into the binary.
//
//go:embed site/index.html site/site.js site/site.css
var siteFiles embed.FS

// siteAssets are the files the page loads, with what each one is. A name outside this list is not
// served: the site hands out the page and its two files, and nothing else on the machine.
var siteAssets = map[string]string{
	"site.js":  "text/javascript; charset=utf-8",
	"site.css": "text/css; charset=utf-8",
}

// servePage answers the page an operator opens.
//
// The project is resolved first, so an address with a name nobody has reads the same sentence here as
// it does on the document underneath. A page that drew itself and then said nothing would leave the
// operator looking at an empty menu with no idea which part of the address was wrong.
func (s *Server) servePage(w http.ResponseWriter, r *http.Request) {
	if _, failed := s.siteProject(r.Context(), r.PathValue("workspace"), r.PathValue("project")); failed != nil {
		failed.answer(w, r)
		return
	}
	page, err := siteFiles.ReadFile("site/index.html")
	if err != nil {
		siteBroke(w, r, "read its own page", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write(page); err != nil {
		slog.WarnContext(r.Context(), "the site could not finish writing its page",
			"path", r.URL.Path, "error", err)
	}
}

// serveAsset answers one of the files the page loads.
//
// It needs no project: the script and the stylesheet are the same for every project, so the page
// names them at one address and a reader with two projects open holds one copy of each.
func serveAsset(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	kind, served := siteAssets[name]
	if !served {
		siteMissing(fmt.Sprintf("the site serves no file called %q", name)).answer(w, r)
		return
	}
	body, err := siteFiles.ReadFile("site/" + name)
	if err != nil {
		siteBroke(w, r, "read "+name, err)
		return
	}
	w.Header().Set("Content-Type", kind)
	if _, err := w.Write(body); err != nil {
		slog.WarnContext(r.Context(), "the site could not finish writing a file",
			"path", r.URL.Path, "error", err)
	}
}

// serveStages answers the design of a project and every stage written under it.
//
// The two reads are the two the wire calls make, so the page and a caller of ListDesignStages cannot
// disagree about what a project holds.
func (s *Server) serveStages(w http.ResponseWriter, r *http.Request) {
	project, failed := s.siteProject(r.Context(), r.PathValue("workspace"), r.PathValue("project"))
	if failed != nil {
		failed.answer(w, r)
		return
	}

	design, err := s.store.GetDesign(r.Context(), project.GetId())
	if err != nil {
		siteBroke(w, r, "read the design", err)
		return
	}
	held, err := s.store.ListDesignStages(r.Context(), project.GetId())
	if err != nil {
		siteBroke(w, r, "read the design stages", err)
		return
	}

	answer := siteAnswer{
		Design: siteDesign{Brief: design.GetBrief(), Body: design.GetBody()},
		Stages: make([]siteStage, 0, len(held)),
	}
	for _, stage := range held {
		answer.Stages = append(answer.Stages, siteStage{
			Stage:           stage.GetStage(),
			Position:        stage.GetPosition(),
			Body:            stage.GetBody(),
			Artifact:        stage.GetArtifact(),
			Version:         stage.GetVersion(),
			ApprovedVersion: stage.GetApprovedVersion(),
			Skipped:         stage.GetSkipped(),
		})
	}
	siteJSON(w, r, answer)
}

// serveFlows answers the artifact one stage carries, as the bytes it was written with.
//
// The column holds json already, so it is handed over rather than parsed and rendered again: the flow
// map reads what the session wrote. A stage carrying nothing is a refusal rather than an empty
// document, because an empty body is not json a reader can parse.
func (s *Server) serveFlows(w http.ResponseWriter, r *http.Request) {
	stage := r.PathValue("stage")
	if _, known := store.DesignStagePosition(stage); !known {
		siteMissing(fmt.Sprintf("%q is not a design stage: the six are %s",
			stage, strings.Join(store.DesignStages(), ", "))).answer(w, r)
		return
	}

	project, failed := s.siteProject(r.Context(), r.PathValue("workspace"), r.PathValue("project"))
	if failed != nil {
		failed.answer(w, r)
		return
	}

	held, err := s.store.ListDesignStages(r.Context(), project.GetId())
	if err != nil {
		siteBroke(w, r, "read the design stages", err)
		return
	}
	for _, one := range held {
		if one.GetStage() != stage {
			continue
		}
		if one.GetArtifact() == "" {
			siteMissing(fmt.Sprintf("the %s stage of %q carries no artifact",
				stage, project.GetName())).answer(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(one.GetArtifact())); err != nil {
			slog.WarnContext(r.Context(), "the site could not finish writing an artifact",
				"stage", stage, "error", err)
		}
		return
	}
	siteMissing(fmt.Sprintf("%q has not written a %s stage", project.GetName(), stage)).answer(w, r)
}

// siteProject is the project an address landed on, with both names resolved the way a person types
// them.
//
// An identifier wins over a name, so a project whose name happens to be another project's identifier
// still resolves to itself. A name two workspaces share resolves to neither: answering by order would
// send the operator to one workspace this morning and the other one this afternoon.
//
// A project of another workspace is refused rather than answered, because answering it would put one
// workspace's design at every other workspace's address.
func (s *Server) siteProject(ctx context.Context, workspace, project string) (*quaycrewv1.Project, *siteRefusal) {
	workspaces, err := s.store.ListWorkspaces(ctx)
	if err != nil {
		return nil, siteBrokeReading("list the workspaces", err)
	}
	held, failed := onlyOne(workspaces, workspace, "workspace",
		func(one *quaycrewv1.Workspace) (string, string) { return one.GetId(), one.GetName() })
	if failed != nil {
		return nil, failed
	}

	projects, err := s.store.ListProjects(ctx, held.GetId())
	if err != nil {
		return nil, siteBrokeReading("list the projects", err)
	}
	found, failed := onlyOne(projects, project, "project",
		func(one *quaycrewv1.Project) (string, string) { return one.GetId(), one.GetName() })
	if failed != nil {
		return nil, failed
	}
	return found, nil
}

// onlyOne is one thing out of a listing, by identifier or by name, and a refusal when the reference
// reaches nothing or reaches more than one.
func onlyOne[T any](listing []T, reference, what string, read func(T) (id, name string)) (T, *siteRefusal) {
	var none T
	if reference == "" {
		return none, siteMissing("say which " + what)
	}
	for _, one := range listing {
		if id, _ := read(one); id == reference {
			return one, nil
		}
	}

	var matched []T
	for _, one := range listing {
		if _, name := read(one); name == reference {
			matched = append(matched, one)
		}
	}
	switch len(matched) {
	case 0:
		return none, siteMissing(fmt.Sprintf("no %s called %q", what, reference))
	case 1:
		return matched[0], nil
	default:
		ids := make([]string, 0, len(matched))
		for _, one := range matched {
			id, _ := read(one)
			ids = append(ids, id)
		}
		sort.Strings(ids)
		return none, siteMissing(fmt.Sprintf("%q is the name of %d %ss, so read one of these instead: %s",
			reference, len(matched), what, strings.Join(ids, ", ")))
	}
}

// siteRefusal is what a reader is told when the site will not answer: a status, and one sentence.
//
// One sentence, because the reader is a person looking at an address they typed and the only useful
// thing to say is which part of it named nothing.
type siteRefusal struct {
	status int
	said   string
	// because is the fault underneath, for the log rather than for the reader. A store that cannot
	// answer is the system's problem and says nothing to somebody reading a page.
	because error
	doing   string
}

func siteMissing(said string) *siteRefusal {
	return &siteRefusal{status: http.StatusNotFound, said: said}
}

func siteBrokeReading(doing string, because error) *siteRefusal {
	return &siteRefusal{
		status:  http.StatusInternalServerError,
		said:    "the system could not " + doing,
		because: because,
		doing:   doing,
	}
}

func (r *siteRefusal) answer(w http.ResponseWriter, request *http.Request) {
	if r.because != nil {
		slog.ErrorContext(request.Context(), "the site could not "+r.doing,
			"path", request.URL.Path, "error", r.because)
	}
	http.Error(w, r.said, r.status)
}

// siteBroke is the refusal a failed read answers with, written out where it happened.
func siteBroke(w http.ResponseWriter, r *http.Request, doing string, because error) {
	siteBrokeReading(doing, because).answer(w, r)
}

// siteJSON writes one document. The header goes before the body, so a reader that acts on the content
// type is never handed json under a plain text heading.
//
// A write that fails part way cannot be taken back: the status is already sent. So it is logged and
// the reader sees a document that stops, which is what a dropped connection looks like anyway.
func siteJSON(w http.ResponseWriter, r *http.Request, answer any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(answer); err != nil {
		slog.WarnContext(r.Context(), "the site could not finish writing a document",
			"path", r.URL.Path, "error", err)
	}
}
