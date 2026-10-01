package controlplane_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/controlplane"
	"github.com/atlantic-blue/quay-krewe/internal/model"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/secrets"
	"github.com/atlantic-blue/quay-krewe/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func newServer(runner model.Runner) *controlplane.Server {
	return controlplane.NewServer(controlplane.Config{
		Store: store.NewMemory(), Runner: runner, Provider: &sandbox.FakeProvider{}, Secrets: secrets.NewMemory(),
	})
}

func TestCreateAndListWorkspaces(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	ctx := context.Background()

	created, err := s.CreateWorkspace(ctx, &quaycrewv1.CreateWorkspaceRequest{Name: "acme"})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if created.GetWorkspace().GetId() == "" || created.GetWorkspace().GetName() != "acme" {
		t.Fatalf("bad workspace: %+v", created.GetWorkspace())
	}

	list, err := s.ListWorkspaces(ctx, &quaycrewv1.ListWorkspacesRequest{})
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	if len(list.GetWorkspaces()) != 1 {
		t.Fatalf("want 1 workspace, got %d", len(list.GetWorkspaces()))
	}
}

func TestGetWorkspaceNotFound(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, err := s.GetWorkspace(context.Background(), &quaycrewv1.GetWorkspaceRequest{Id: "nope"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("want NotFound, got %v", err)
	}
}

func TestDispatchStartsAndContinuesSession(t *testing.T) {
	runner := &model.FakeRunner{Reply: "done"}
	s := newServer(runner)
	ctx := context.Background()

	wid, pid := newProject(t, s)
	_ = wid

	first, err := s.Dispatch(ctx, &quaycrewv1.DispatchRequest{Project: pid, Text: "hello"})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if first.GetReply() != "done" || first.GetHandle() == "" {
		t.Fatalf("bad dispatch response: %+v", first)
	}
	// The system names the conversation before the exec starts, and the first exec starts it rather than
	// resuming it. Both halves matter: a first exec that carried no name left the runtime to name its
	// own conversation and tell nobody until the exec was over.
	named := runner.LastReq.ModelSessionID
	if named == "" {
		t.Fatal("the first exec carries no conversation, so the runtime names one the system cannot see")
	}
	if runner.LastReq.ConversationStarted {
		t.Fatal("the first exec resumes a conversation nothing has written, which exits saying there is none")
	}
	held, err := s.GetSession(ctx, &quaycrewv1.GetSessionRequest{Id: first.GetId()})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got := held.GetSession().GetModelSessionId(); got != named {
		t.Fatalf("the session holds conversation %q and its exec ran in %q", got, named)
	}

	// Continue the same session: the runner should be asked to resume the model session.
	second, err := s.Dispatch(ctx, &quaycrewv1.DispatchRequest{Project: pid, Handle: first.GetHandle(), Text: "more"})
	if err != nil {
		t.Fatalf("Dispatch continue: %v", err)
	}
	if second.GetId() != first.GetId() {
		t.Fatalf("continue should reuse session: %q vs %q", second.GetId(), first.GetId())
	}
	if runner.LastReq.ModelSessionID != named {
		t.Fatalf("the second exec runs in conversation %q, want the first exec's %q",
			runner.LastReq.ModelSessionID, named)
	}
	if !runner.LastReq.ConversationStarted {
		t.Fatal("the second exec starts the conversation again, which is refused as a name already in use")
	}
}

func TestDispatchUnknownWorkspace(t *testing.T) {
	s := newServer(&model.FakeRunner{})
	_, err := s.Dispatch(context.Background(), &quaycrewv1.DispatchRequest{Project: "ghost", Text: "hi"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("want NotFound, got %v", err)
	}
}

func TestDispatchInjectsTheWorkspaceSubscriptionToken(t *testing.T) {
	runner := &model.FakeRunner{Reply: "ok"}
	s := controlplane.NewServer(controlplane.Config{
		Store: store.NewMemory(), Runner: runner, Provider: &sandbox.FakeProvider{}, Secrets: secrets.NewMemory(),
	})
	ctx := context.Background()

	wid, pid := newProject(t, s)

	if _, err := s.SetSecret(ctx, &quaycrewv1.SetSecretRequest{Workspace: wid, Key: model.ClaudeCodeOAuthTokenEnv, Value: "tok-xyz"}); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}
	if _, err := s.Dispatch(ctx, &quaycrewv1.DispatchRequest{Project: pid, Text: "hello"}); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if got := runner.LastReq.Env[model.ClaudeCodeOAuthTokenEnv]; got != "tok-xyz" {
		t.Fatalf("exec env[%s] = %q, want tok-xyz", model.ClaudeCodeOAuthTokenEnv, got)
	}
}

// A session is told which session it is whatever the workspace holds, because it names its own
// working tree in the shared volume after that. Everything else here comes from a secret, so this is
// what an exec with none carries.
func TestDispatchWithoutASecretRunsWithNoExtraEnv(t *testing.T) {
	runner := &model.FakeRunner{Reply: "ok"}
	s := newServer(runner)
	ctx := context.Background()

	_, pid := newProject(t, s)
	if _, err := s.Dispatch(ctx, &quaycrewv1.DispatchRequest{Project: pid, Text: "hello"}); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if len(runner.LastReq.Env) != 1 || runner.LastReq.Env[sandbox.SessionIDEnv] == "" {
		t.Fatalf("exec env = %v, want only %s when no secret is set", runner.LastReq.Env, sandbox.SessionIDEnv)
	}
}

func TestSetSecretStoresValue(t *testing.T) {
	secretStore := secrets.NewMemory()
	s := controlplane.NewServer(controlplane.Config{
		Store: store.NewMemory(), Runner: &model.FakeRunner{}, Provider: &sandbox.FakeProvider{}, Secrets: secretStore,
	})
	ctx := context.Background()

	wid, _ := newProject(t, s)

	if _, err := s.SetSecret(ctx, &quaycrewv1.SetSecretRequest{Workspace: wid, Key: "token", Value: "s3cret"}); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}
	got, err := secretStore.Get(ctx, wid, "token")
	if err != nil || got != "s3cret" {
		t.Fatalf("secret not stored: got %q err %v", got, err)
	}
}

func TestSessionSandboxLifecycle(t *testing.T) {
	provider := &sandbox.FakeProvider{}
	s := controlplane.NewServer(controlplane.Config{
		Store: store.NewMemory(), Runner: &model.FakeRunner{Reply: "ok"}, Provider: provider, Secrets: secrets.NewMemory(),
	})
	ctx := context.Background()

	wid, pid := newProject(t, s)
	_ = wid

	// Two execs on the same session must share one sandbox (created once, not per exec).
	first, _ := s.Dispatch(ctx, &quaycrewv1.DispatchRequest{Project: pid, Text: "one"})
	if _, err := s.Dispatch(ctx, &quaycrewv1.DispatchRequest{Project: pid, Handle: first.GetHandle(), Text: "two"}); err != nil {
		t.Fatalf("second dispatch: %v", err)
	}
	if len(provider.Created) != 1 {
		t.Fatalf("expected 1 sandbox for the session, got %d (%v)", len(provider.Created), provider.Created)
	}
	if provider.Created[0].ID != first.GetId() {
		t.Fatalf("sandbox created for %q, want session %q", provider.Created[0].ID, first.GetId())
	}

	// Stopping the session tears its sandbox down.
	if _, err := s.StopSession(ctx, &quaycrewv1.StopSessionRequest{Id: first.GetId()}); err != nil {
		t.Fatalf("StopSession: %v", err)
	}
	if !provider.Boxes[0].Closed {
		t.Fatal("stopping the session did not close its sandbox")
	}
}

// TestOverGrpc exercises the full gRPC path over an in memory listener.
func TestOverGrpc(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	quaycrewv1.RegisterControlPlaneServiceServer(grpcServer, newServer(&model.FakeRunner{Reply: "ok"}))
	go func() { _ = grpcServer.Serve(lis) }()
	t.Cleanup(grpcServer.Stop)

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := quaycrewv1.NewControlPlaneServiceClient(conn)

	ctx := context.Background()
	workspace, err := client.CreateWorkspace(ctx, &quaycrewv1.CreateWorkspaceRequest{Name: "acme"})
	if err != nil {
		t.Fatalf("CreateWorkspace over grpc: %v", err)
	}
	project, err := client.CreateProject(ctx, &quaycrewv1.CreateProjectRequest{
		Workspace: workspace.GetWorkspace().GetId(), Name: "house-bills",
	})
	if err != nil {
		t.Fatalf("CreateProject over grpc: %v", err)
	}
	dispatch, err := client.Dispatch(ctx, &quaycrewv1.DispatchRequest{Project: project.GetProject().GetId(), Text: "hi"})
	if err != nil {
		t.Fatalf("Dispatch over grpc: %v", err)
	}
	if dispatch.GetReply() != "ok" {
		t.Fatalf("reply = %q, want ok", dispatch.GetReply())
	}
}

// newProject creates a workspace and a project inside it: the smallest setup a dispatch needs now
// that a session lives inside a project.
func newProject(t *testing.T, s *controlplane.Server) (workspaceID, projectID string) {
	t.Helper()
	ctx := context.Background()
	workspace, err := s.CreateWorkspace(ctx, &quaycrewv1.CreateWorkspaceRequest{Name: "acme"})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	project, err := s.CreateProject(ctx, &quaycrewv1.CreateProjectRequest{
		Workspace: workspace.GetWorkspace().GetId(), Name: "house-bills",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	return workspace.GetWorkspace().GetId(), project.GetProject().GetId()
}

// hangsUpRunner cancels the caller's context while the exec runs, which is what a client
// disconnecting after a long exec does to the dispatch path.
type hangsUpRunner struct {
	model.FakeRunner
	hangUp context.CancelFunc
}

func (r *hangsUpRunner) Run(ctx context.Context, box sandbox.Sandbox, req model.Request) (model.Response, error) {
	r.hangUp()
	return r.FakeRunner.Run(ctx, box, req)
}

// contextHonouringStore refuses writes on a dead context, the way Postgres does and the memory
// store does not. Without it this test would pass whatever the dispatch path did.
type contextHonouringStore struct {
	store.Store
}

func (s contextHonouringStore) AppendExec(ctx context.Context, exec *quaycrewv1.Exec, workspace, project, session string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.Store.AppendExec(ctx, exec, workspace, project, session)
}

// A caller hanging up after a long exec must not lose the record of the very exec they were waiting
// on: history is written on a context detached from the request's.
func TestHistorySurvivesTheCallerHangingUp(t *testing.T) {
	ctx, hangUp := context.WithCancel(context.Background())
	defer hangUp()

	runner := &hangsUpRunner{FakeRunner: model.FakeRunner{Reply: "done"}, hangUp: hangUp}
	s := controlplane.NewServer(controlplane.Config{
		Store:    contextHonouringStore{Store: store.NewMemory()},
		Runner:   runner,
		Provider: &sandbox.FakeProvider{},
		Secrets:  secrets.NewMemory(),
	})

	workspace, err := s.CreateWorkspace(context.Background(), &quaycrewv1.CreateWorkspaceRequest{Name: "acme"})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	project, err := s.CreateProject(context.Background(), &quaycrewv1.CreateProjectRequest{
		Workspace: workspace.GetWorkspace().GetId(), Name: "house-bills",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	dispatched, err := s.Dispatch(ctx, &quaycrewv1.DispatchRequest{
		Project: project.GetProject().GetId(), Text: "hello",
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	listed, err := s.ListExecs(context.Background(), &quaycrewv1.ListExecsRequest{Session: dispatched.GetId()})
	if err != nil {
		t.Fatalf("ListExecs: %v", err)
	}
	if len(listed.GetExecs()) != 1 {
		t.Fatalf("the session has %d execs after the caller hung up, want the 1 that ran", len(listed.GetExecs()))
	}
	if listed.GetExecs()[0].GetReply() != "done" {
		t.Fatalf("the recorded exec says %q, want the reply that ran", listed.GetExecs()[0].GetReply())
	}
}

// The token a second time, under a name the Claude Code command line leaves alone.
//
// The CLI removes CLAUDE_CODE_OAUTH_TOKEN from the environment of every process it starts, by that
// name and no other, so the prompt hook fired on a message held no credential and every message went
// unanalysed. Nothing said so: the hook fails open, and the only record was the word "no answer" in a
// file in /tmp. The value is the same value, and only the name is new.
func TestDispatchCarriesTheSubscriptionTokenUnderTheNameThatSurvivesIntoAHook(t *testing.T) {
	runner := &model.FakeRunner{Reply: "ok"}
	s := controlplane.NewServer(controlplane.Config{
		Store: store.NewMemory(), Runner: runner, Provider: &sandbox.FakeProvider{}, Secrets: secrets.NewMemory(),
	})
	ctx := context.Background()

	wid, pid := newProject(t, s)

	if _, err := s.SetSecret(ctx, &quaycrewv1.SetSecretRequest{
		Workspace: wid, Key: model.ClaudeCodeOAuthTokenEnv, Value: "tok-xyz"}); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}
	if _, err := s.Dispatch(ctx, &quaycrewv1.DispatchRequest{Project: pid, Text: "hello"}); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if got := runner.LastReq.Env[model.ModelTokenEnv]; got != "tok-xyz" {
		t.Fatalf("exec env[%s] = %q, want tok-xyz, and without it a hook cannot ask a model anything",
			model.ModelTokenEnv, got)
	}
	if got := runner.LastReq.Env[model.ClaudeCodeOAuthTokenEnv]; got != "tok-xyz" {
		t.Fatalf("exec env[%s] = %q, want tok-xyz: the second name is as well as, never instead of",
			model.ClaudeCodeOAuthTokenEnv, got)
	}
}

// A workspace with no subscription token carries neither name. An empty credential reads as
// configured, and the hook would report being logged out with a variable set.
func TestAWorkspaceWithNoSubscriptionTokenCarriesNeitherName(t *testing.T) {
	runner := &model.FakeRunner{Reply: "ok"}
	s := newServer(runner)
	ctx := context.Background()

	_, pid := newProject(t, s)
	if _, err := s.Dispatch(ctx, &quaycrewv1.DispatchRequest{Project: pid, Text: "hello"}); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if got, set := runner.LastReq.Env[model.ModelTokenEnv]; set {
		t.Fatalf("exec env carries %s=%q, and nobody set a token", model.ModelTokenEnv, got)
	}
}

// refusesOneWorkspace reads every workspace and refuses to list the projects of one of them, which
// is the failure a sweep over every workspace has to survive without quietly shrinking its counts.
type refusesOneWorkspace struct {
	store.Store
	refused string
}

func (s refusesOneWorkspace) ListProjects(ctx context.Context, workspace string) ([]*quaycrewv1.Project, error) {
	if workspace == s.refused {
		return nil, errors.New("the projects table is not readable")
	}
	return s.Store.ListProjects(ctx, workspace)
}

// A sweep over the system that cannot read one workspace names it rather than dropping it. Without
// this the listing falls by a number the operator cannot account for, and a workspace missing from a
// sweep reads exactly like a workspace that had nothing in it.
func TestASystemSweepNamesTheWorkspaceItCouldNotRead(t *testing.T) {
	ctx := context.Background()
	held := store.NewMemory()
	reachable, err := held.CreateWorkspace(ctx, "acme")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	unreadable, err := held.CreateWorkspace(ctx, "beside")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	s := controlplane.NewServer(controlplane.Config{
		Store:    refusesOneWorkspace{Store: held, refused: unreadable.GetId()},
		Runner:   &model.FakeRunner{},
		Provider: &sandbox.FakeProvider{},
		Secrets:  secrets.NewMemory(),
	})

	swept, err := s.ArchiveSystemSessions(ctx, &quaycrewv1.ArchiveSystemSessionsRequest{})
	if err != nil {
		t.Fatalf("ArchiveSystemSessions: %v", err)
	}
	// The one it did read is counted, so a single failure does not throw the whole answer away.
	if got := swept.GetWorkspacesRead(); got != 1 {
		t.Errorf("the sweep says it read %d workspaces, want 1 of the 2 it was given", got)
	}
	if len(swept.GetUnswept()) != 1 {
		t.Fatalf("the sweep names %d workspaces it could not read, want 1", len(swept.GetUnswept()))
	}
	named := swept.GetUnswept()[0]
	if named.GetWorkspace() != unreadable.GetId() {
		t.Errorf("the sweep names workspace %q, want %q", named.GetWorkspace(), unreadable.GetId())
	}
	// The name, because an identifier alone names nothing an operator can go and look at.
	if named.GetName() != "beside" {
		t.Errorf("the sweep calls it %q, want %q", named.GetName(), "beside")
	}
	if !strings.Contains(named.GetReason(), "the projects table is not readable") {
		t.Errorf("the sweep gives the reason %q, which does not carry what the store said", named.GetReason())
	}
	// The reason reads as a sentence about the system rather than as a wire failure.
	if strings.Contains(named.GetReason(), "rpc error") {
		t.Errorf("the reason is a wire failure rather than a sentence: %q", named.GetReason())
	}
	if reachable.GetId() == unreadable.GetId() {
		t.Fatal("both workspaces have one id, so this test proves nothing")
	}
}

// sessionReads is the store under a caller that walks away: it counts what a listing asks the store
// for one session at a time, and it is where the caller goes.
//
// The conversation store is a struct rather than an interface, so nothing can count the transcripts a
// listing reads. The store is the one per session read a double can see, and the same loop makes both,
// so counting one of them says whether the loop stopped.
type sessionReads struct {
	store.Store
	mu     sync.Mutex
	skills int
	lists  int
	// leaveAfterSkills is the session whose skills read takes the caller away, and leaveOnList the
	// listing that does. Zero is a caller that stays.
	leaveAfterSkills int
	leaveOnList      int
	leave            func()
}

func (s *sessionReads) SessionSkills(ctx context.Context, id string) (string, error) {
	born, err := s.Store.SessionSkills(ctx, id)
	s.mu.Lock()
	s.skills++
	leaving := s.leaveAfterSkills > 0 && s.skills == s.leaveAfterSkills
	s.mu.Unlock()
	if leaving {
		s.leave()
	}
	return born, err
}

func (s *sessionReads) ListSessions(ctx context.Context, filter store.SessionFilter) ([]*quaycrewv1.Session, error) {
	listed, err := s.Store.ListSessions(ctx, filter)
	s.mu.Lock()
	s.lists++
	leaving := s.leaveOnList > 0 && s.lists == s.leaveOnList
	s.mu.Unlock()
	if leaving {
		s.leave()
	}
	return listed, err
}

// leavesAfter arms the caller and forgets what setting the test up read, so the counts below belong
// to the call under test and to nothing else.
func (s *sessionReads) leavesAfter(skills, lists int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.skills, s.lists = 0, 0
	s.leaveAfterSkills, s.leaveOnList = skills, lists
}

func (s *sessionReads) counted() (skills, lists int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.skills, s.lists
}

// seedSessions puts sessions in the store without dispatching to any of them, which is the only way a
// test stands up enough of them to watch a listing stop in the middle of one.
func seedSessions(t *testing.T, held store.Store, project string, count int) {
	t.Helper()
	for i := range count {
		_, _, err := held.FindOrCreateSession(context.Background(), project,
			fmt.Sprintf("session-%d", i), store.Birth{})
		if err != nil {
			t.Fatalf("seed session %d: %v", i, err)
		}
	}
}

// A console asks for the session list every few seconds and gives up on a list that is late, so the
// calls nobody waits for are most of what a jammed control plane is busy with. This is the one that
// says a listing stops where its caller stopped.
func TestAListStopsWhereItsCallerStopped(t *testing.T) {
	const sessions = 40
	const waitedFor = 3

	ctx, gone := context.WithCancel(context.Background())
	defer gone()

	held := store.NewMemory()
	reads := &sessionReads{Store: held, leave: gone}
	s := controlplane.NewServer(controlplane.Config{
		Store: reads, Runner: &model.FakeRunner{}, Provider: &sandbox.FakeProvider{},
		Secrets: secrets.NewMemory(),
	})
	_, project := newProject(t, s)
	seedSessions(t, held, project, sessions)
	reads.leavesAfter(waitedFor, 0)

	listed, err := s.ListSessions(ctx, &quaycrewv1.ListSessionsRequest{Project: project})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("the list answered %v, want the error of the caller that went", err)
	}
	if got := len(listed.GetSessions()); got != 0 {
		t.Errorf("the list hands back %d sessions to a caller that is not there", got)
	}
	skills, lists := reads.counted()
	if skills == 0 {
		t.Fatal("the list read no session at all, so the caller never went and this test proves nothing")
	}
	if skills > waitedFor {
		t.Errorf("the list read %d sessions, and its caller went after %d", skills, waitedFor)
	}
	if lists > 1 {
		t.Errorf("the list took %d store listings, so it went on counting the hidden ones after its caller went", lists)
	}
}

// The other half of the same contract: a caller that waits gets everything it waited for.
func TestAListAnswersInFullForACallerThatWaits(t *testing.T) {
	const sessions = 40

	held := store.NewMemory()
	s := controlplane.NewServer(controlplane.Config{
		Store: held, Runner: &model.FakeRunner{}, Provider: &sandbox.FakeProvider{},
		Secrets: secrets.NewMemory(),
	})
	_, project := newProject(t, s)
	seedSessions(t, held, project, sessions)

	listed, err := s.ListSessions(context.Background(), &quaycrewv1.ListSessionsRequest{Project: project})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if got := len(listed.GetSessions()); got != sessions {
		t.Fatalf("the list holds %d sessions, want the %d in the system", got, sessions)
	}
}

// The usage call reads every transcript the system holds, live and archived, so it is the other call
// that a console can leave working for a caller that has gone.
func TestUsageStopsWhenItsCallerHasGone(t *testing.T) {
	const sessions = 40

	ctx, gone := context.WithCancel(context.Background())
	defer gone()

	held := store.NewMemory()
	reads := &sessionReads{Store: held, leave: gone}
	s := controlplane.NewServer(controlplane.Config{
		Store: reads, Runner: &model.FakeRunner{}, Provider: &sandbox.FakeProvider{},
		Secrets: secrets.NewMemory(),
	})
	_, project := newProject(t, s)
	seedSessions(t, held, project, sessions)
	reads.leavesAfter(0, 1)

	spent, err := s.GetUsage(ctx, &quaycrewv1.GetUsageRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("the usage call answered %v, want the error of the caller that went", err)
	}
	if spent.GetSessions() != 0 {
		t.Errorf("the usage call counted %d sessions for a caller that is not there", spent.GetSessions())
	}
	_, lists := reads.counted()
	if lists == 0 {
		t.Fatal("the usage call listed nothing at all, so the caller never went and this test proves nothing")
	}
	if lists > 1 {
		t.Errorf("the usage call took %d store listings, and its caller went during the first", lists)
	}
}
