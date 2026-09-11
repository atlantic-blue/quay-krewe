package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/deploy"
	"github.com/atlantic-blue/quay-krewe/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Postgres is the durable Store. Everything the control plane knows lives here, so the process holds
// nothing that a restart would lose.
type Postgres struct {
	pool *pgxpool.Pool
}

var _ Store = (*Postgres)(nil)

// NewPostgres connects, applies the migrations, and returns the store. The caller closes it.
func NewPostgres(ctx context.Context, databaseURL string) (*Postgres, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	// Every query the control plane makes is short. A connection that cannot be had quickly is a
	// failed exec, not an exec that hangs.
	config.ConnConfig.ConnectTimeout = 10 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	if err := Migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return &Postgres{pool: pool}, nil
}

// Probe writes one row and writes over it next time, so a caller can prove the store still takes a
// write. It takes a connection from the pool the way every other write does, which is the half of
// the path a read cannot speak for: a pool with nothing left to hand out answers no write at all.
func (p *Postgres) Probe(ctx context.Context) error {
	if _, err := p.pool.Exec(ctx,
		`insert into health_probe (id, written_at) values (1, now())
		 on conflict (id) do update set written_at = now()`); err != nil {
		return fmt.Errorf("store: probe write: %w", err)
	}
	return nil
}

// Close releases the connection pool.
func (p *Postgres) Close() { p.pool.Close() }

// CreateWorkspace inserts a workspace.
func (p *Postgres) CreateWorkspace(ctx context.Context, name string) (*quaycrewv1.Workspace, error) {
	var (
		id        = NewID()
		createdAt time.Time
	)
	err := p.pool.QueryRow(ctx,
		`insert into workspaces (id, name) values ($1, $2) returning created_at`, id, name,
	).Scan(&createdAt)
	if err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	return &quaycrewv1.Workspace{Id: id, Name: name, CreatedAt: timestamppb.New(createdAt)}, nil
}

// GetWorkspace returns a workspace that has not been deleted.
func (p *Postgres) GetWorkspace(ctx context.Context, id string) (*quaycrewv1.Workspace, error) {
	var (
		name      string
		createdAt time.Time
	)
	err := p.pool.QueryRow(ctx,
		`select name, created_at from workspaces where id = $1 and deleted_at is null`, id,
	).Scan(&name, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get workspace: %w", err)
	}
	return &quaycrewv1.Workspace{Id: id, Name: name, CreatedAt: timestamppb.New(createdAt)}, nil
}

// ListWorkspaces returns every workspace that has not been deleted, newest first.
func (p *Postgres) ListWorkspaces(ctx context.Context) ([]*quaycrewv1.Workspace, error) {
	rows, err := p.pool.Query(ctx,
		`select id, name, created_at from workspaces where deleted_at is null order by created_at desc, id`)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	defer rows.Close()

	out := make([]*quaycrewv1.Workspace, 0)
	for rows.Next() {
		var (
			id, name  string
			createdAt time.Time
		)
		if err := rows.Scan(&id, &name, &createdAt); err != nil {
			return nil, fmt.Errorf("scan workspace: %w", err)
		}
		out = append(out, &quaycrewv1.Workspace{Id: id, Name: name, CreatedAt: timestamppb.New(createdAt)})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	return out, nil
}

// DeleteWorkspace soft deletes a workspace, leaving its sessions intact.
func (p *Postgres) DeleteWorkspace(ctx context.Context, id string) error {
	tag, err := p.pool.Exec(ctx,
		`update workspaces set deleted_at = now(), updated_at = now() where id = $1 and deleted_at is null`, id)
	if err != nil {
		return fmt.Errorf("delete workspace: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AttachChannel records a channel against a live workspace.
func (p *Postgres) AttachChannel(ctx context.Context, workspace, id, kind string) (*quaycrewv1.Channel, error) {
	if _, err := p.GetWorkspace(ctx, workspace); err != nil {
		return nil, err
	}
	_, err := p.pool.Exec(ctx, `
		insert into channels (id, workspace, kind) values ($1, $2, $3)
		on conflict (workspace, id) do update set kind = excluded.kind, updated_at = now()`,
		id, workspace, kind)
	if err != nil {
		return nil, fmt.Errorf("attach channel: %w", err)
	}
	return &quaycrewv1.Channel{Workspace: workspace, Id: id, Kind: kind}, nil
}

// CreateProject adds a body of work to a live workspace.
func (p *Postgres) CreateProject(ctx context.Context, workspace, name string) (*quaycrewv1.Project, error) {
	if _, err := p.GetWorkspace(ctx, workspace); err != nil {
		return nil, err
	}
	var (
		id        = NewID()
		createdAt time.Time
	)
	err := p.pool.QueryRow(ctx,
		`insert into projects (id, workspace, name) values ($1, $2, $3) returning created_at`,
		id, workspace, name,
	).Scan(&createdAt)
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}
	return &quaycrewv1.Project{Id: id, Workspace: workspace, Name: name, CreatedAt: timestamppb.New(createdAt)}, nil
}

// GetProject returns a live project whose workspace is also live.
func (p *Postgres) GetProject(ctx context.Context, id string) (*quaycrewv1.Project, error) {
	var (
		workspace, name           string
		account, region, identity string
		repository, visibility    string
		createdAt                 time.Time
	)
	// The join is what stops a project outliving the workspace it belongs to.
	err := p.pool.QueryRow(ctx, `
		select p.workspace, p.name, p.created_at, p.deploy_account, p.deploy_region, p.deploy_identity,
		       p.repository, p.visibility
		from projects p join workspaces w on w.id = p.workspace
		where p.id = $1 and p.deleted_at is null and w.deleted_at is null`, id,
	).Scan(&workspace, &name, &createdAt, &account, &region, &identity, &repository, &visibility)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	return &quaycrewv1.Project{
		Id: id, Workspace: workspace, Name: name, CreatedAt: timestamppb.New(createdAt),
		DeployTarget: deployTarget(account, region, identity),
		Repository:   repository, Visibility: visibility,
	}, nil
}

// SetProjectRepository records where a project's work lands, and what kind of repository it is.
//
// The row is read first, through GetProject, so a project whose workspace has been deleted is not
// found rather than updated: a project outliving its workspace is the case that join exists for.
func (p *Postgres) SetProjectRepository(ctx context.Context, project, repository, visibility string) (*quaycrewv1.Project, error) {
	if _, err := p.GetProject(ctx, project); err != nil {
		return nil, err
	}
	if _, err := p.pool.Exec(ctx, `
		update projects set repository = $2, visibility = $3, updated_at = now()
		where id = $1 and deleted_at is null`, project, repository, visibility); err != nil {
		return nil, fmt.Errorf("set project repository: %w", err)
	}
	return p.GetProject(ctx, project)
}

// ListProjects returns live projects, filtered to one workspace when set, newest first.
func (p *Postgres) ListProjects(ctx context.Context, workspace string) ([]*quaycrewv1.Project, error) {
	rows, err := p.pool.Query(ctx, `
		select p.id, p.workspace, p.name, p.created_at, p.deploy_account, p.deploy_region, p.deploy_identity,
		       p.repository, p.visibility
		from projects p join workspaces w on w.id = p.workspace
		where p.deleted_at is null and w.deleted_at is null and ($1 = '' or p.workspace = $1)
		order by p.created_at desc, p.id`, workspace)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	out := make([]*quaycrewv1.Project, 0)
	for rows.Next() {
		var (
			id, owner, name           string
			account, region, identity string
			repository, visibility    string
			createdAt                 time.Time
		)
		if err := rows.Scan(&id, &owner, &name, &createdAt, &account, &region, &identity,
			&repository, &visibility); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		out = append(out, &quaycrewv1.Project{
			Id: id, Workspace: owner, Name: name, CreatedAt: timestamppb.New(createdAt),
			DeployTarget: deployTarget(account, region, identity),
			Repository:   repository, Visibility: visibility,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return out, nil
}

// SetDeployTarget records where a project ships, and a zero target clears it.
func (p *Postgres) SetDeployTarget(ctx context.Context, project string, target deploy.Target) error {
	if _, err := p.GetProject(ctx, project); err != nil {
		return err
	}
	if _, err := p.pool.Exec(ctx, `
		update projects
		set deploy_account = $2, deploy_region = $3, deploy_identity = $4, updated_at = now()
		where id = $1 and deleted_at is null`,
		project, target.Account, target.Region, target.Identity); err != nil {
		return fmt.Errorf("set deploy target: %w", err)
	}
	return nil
}

// DeleteProject soft deletes a project, leaving its sessions intact.
func (p *Postgres) DeleteProject(ctx context.Context, id string) error {
	if _, err := p.GetProject(ctx, id); err != nil {
		return err
	}
	if _, err := p.pool.Exec(ctx,
		`update projects set deleted_at = now(), updated_at = now() where id = $1 and deleted_at is null`, id); err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	return nil
}

// FindOrCreateSession returns the project's session for a session, creating it on first use.
//
// The insert races with any other caller dispatching to the same session, so it defers to the unique
// constraint on (workspace, handle) and reads the winner back rather than trusting a prior select.
func (p *Postgres) FindOrCreateSession(ctx context.Context, project, session string, born Birth) (*quaycrewv1.Session, bool, error) {
	owner, err := p.GetProject(ctx, project)
	if err != nil {
		return nil, false, err
	}
	// The mode is written here rather than left to the column's default, because the default is one
	// value for every system and this is the system's own choice.
	//
	// Whether a row was written is read from the insert rather than by looking first: two callers
	// racing for one handle would both find nothing and both call it a creation, and the session
	// would be announced twice.
	tag, err := p.pool.Exec(ctx, `
		insert into sessions (id, workspace, project, handle, status, permission_mode, title)
		values ($1, $2, $3, $4, 'idle', $5, $6)
		on conflict (project, handle) do nothing`,
		NewID(), owner.GetWorkspace(), project, session, model.PermissionModeBornIn(born.Mode),
		born.Title)
	if err != nil {
		return nil, false, fmt.Errorf("create session: %w", err)
	}
	found, err := p.sessionBy(ctx, `project = $1 and handle = $2`, project, session)
	if err != nil {
		return nil, false, err
	}
	return found, tag.RowsAffected() == 1, nil
}

// RecordExec stores the model conversation handle and status after an exec. An empty handle leaves
// the stored one alone, so a failed exec cannot erase the pointer to a live conversation.
func (p *Postgres) RecordExec(ctx context.Context, id, modelSessionID, status string) error {
	tag, err := p.pool.Exec(ctx, `
		update sessions
		set model_session_id = case when $2 = '' then model_session_id else $2 end,
		    status = $3,
		    -- An exec is running or has landed, so the session holds a container again and the stamp
		    -- that said the system took the last one back is no longer true. Left behind, the archive
		    -- rule would go on measuring against a reclaim that a dispatch already undid.
		    reclaimed_at = null,
		    updated_at = now()
		where id = $1`,
		id, modelSessionID, status)
	if err != nil {
		return fmt.Errorf("record exec: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetSession returns a session by id.
func (p *Postgres) GetSession(ctx context.Context, id string) (*quaycrewv1.Session, error) {
	return p.sessionBy(ctx, `id = $1`, id)
}

// sessionColumns is every field of a session, in the order scanSession reads them. One list rather
// than four copies of it: a column added to the row and forgotten in one of the four reads is a
// session that scans in three places and fails in the fourth.
const sessionColumns = `id, workspace, project, handle, status, model_session_id, created_at, ` +
	`updated_at, archived_at, reclaimed_at, permission_mode, driver, label, description, ` +
	`described_at_exec, title`

// ListSessions returns sessions, filtered to one project when set, else to one workspace when set,
// last moved first: see sortByLastMoved for the order and why it is that one.
func (p *Postgres) ListSessions(ctx context.Context, filter SessionFilter) ([]*quaycrewv1.Session, error) {
	rows, err := p.pool.Query(ctx, `
		select `+sessionColumns+`
		from sessions
		where ($2 = '' or project = $2)
		  and ($2 <> '' or $1 = '' or workspace = $1)
		  and ((archived_at is not null) = $3)
		-- The same stamp the age column shows, so the column reads in order. An archived session is
		-- measured from when it was put away, a live one from when it was last touched, and the
		-- identifier breaks a tie. sortByLastMoved is this rule in Go, and storetest holds the two to it.
		order by coalesce(archived_at, updated_at) desc, id`, filter.Workspace, filter.Project, filter.Archived)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	out := make([]*quaycrewv1.Session, 0)
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	return out, nil
}

// StopSession marks a session stopped.
func (p *Postgres) StopSession(ctx context.Context, id string) error {
	tag, err := p.pool.Exec(ctx,
		`update sessions set status = 'stopped', skills_fingerprint = '', reclaimed_at = null,
		 updated_at = now() where id = $1`, id)
	if err != nil {
		return fmt.Errorf("stop session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ReclaimSession records that the system took the session's container back.
//
// The stamp is written beside the status rather than read off updated_at, because the archive time is
// measured against how long the session has been reclaimed, and updated_at moves on every write.
//
// The skills fingerprint goes with the container, the same way stopping clears it: the next sandbox
// is born with the workspace's current set, so a reclaimed session is never stale.
func (p *Postgres) ReclaimSession(ctx context.Context, id string) error {
	tag, err := p.pool.Exec(ctx,
		`update sessions set status = 'reclaimed', skills_fingerprint = '', reclaimed_at = now(),
		 updated_at = now() where id = $1`, id)
	if err != nil {
		return fmt.Errorf("reclaim session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// IdleSandboxes is the sessions that still hold a container and nothing is holding open, oldest
// touched first.
//
// Live, not running, not already reclaimed, and named by no job in a non terminal phase. A session
// with an exec under way is not settled, and neither is one whose job is still open even though its
// own exec has landed: the job is what says the session is wanted, and the controller is about to
// send it another exec.
//
// Sessions an operator stopped are left out. A stop is somebody's decision, and filing away what
// somebody halted would overwrite it with bookkeeping.
//
// Reclaimed rows are left out too, and that split is the fault this closes. They stay settled for
// ever where no archive time is set, and their updated_at is the moment they were reclaimed, so they
// sort ahead of a sandbox that has been idle for an hour. One batch of twenty then holds twenty rows
// nothing can move, and the reclaim never reaches a container again. See issue 575.
func (p *Postgres) IdleSandboxes(ctx context.Context, limit int) ([]*quaycrewv1.Session, error) {
	return p.settled(ctx, `
		  and s.status = any($1)
		order by s.updated_at, s.id`, limit, holdingStatuses())
}

// ReclaimedSessions is the sessions whose container has already gone, longest reclaimed first.
//
// Ordered by reclaimed_at rather than updated_at, because that is what the archive time is measured
// against. A reclaim writes both stamps and only one of them says how long the session has been in
// this state.
func (p *Postgres) ReclaimedSessions(ctx context.Context, limit int) ([]*quaycrewv1.Session, error) {
	return p.settled(ctx, `
		  and s.status = $1
		order by s.reclaimed_at nulls first, s.id`, limit, StatusReclaimed)
}

// settled runs one of the two queries above. Both read the same rows through the same index and
// differ only in which statuses they take and what they order by.
func (p *Postgres) settled(ctx context.Context, where string, limit int, args ...any) (
	[]*quaycrewv1.Session, error) {
	query := `select ` + sessionColumns + ` from sessions s where s.archived_at is null` + where
	if limit > 0 {
		args = append(args, limit)
		query += fmt.Sprintf(" limit $%d", len(args))
	}
	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("settled sessions: %w", err)
	}
	defer rows.Close()

	out := make([]*quaycrewv1.Session, 0)
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("settled sessions: %w", err)
	}
	return out, nil
}

// SetSessionSkills records the skill set a session's live sandbox was born with; empty clears it.
func (p *Postgres) SetSessionSkills(ctx context.Context, id, fingerprint string) error {
	tag, err := p.pool.Exec(ctx,
		`update sessions set skills_fingerprint = $2, updated_at = now() where id = $1`, id, fingerprint)
	if err != nil {
		return fmt.Errorf("set session skills: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SessionSkills reads what skill set the session's live sandbox was born with, empty when no live
// sandbox is known.
func (p *Postgres) SessionSkills(ctx context.Context, id string) (string, error) {
	var fingerprint string
	err := p.pool.QueryRow(ctx,
		`select skills_fingerprint from sessions where id = $1`, id).Scan(&fingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("session skills: %w", err)
	}
	return fingerprint, nil
}

// RestartSession marks a session idle again. The conversation handle is left exactly as it was: it
// is the only pointer to a conversation the model keeps on its own disk.
func (p *Postgres) RestartSession(ctx context.Context, id string) error {
	tag, err := p.pool.Exec(ctx,
		`update sessions set status = 'idle', reclaimed_at = null, updated_at = now() where id = $1`, id)
	if err != nil {
		return fmt.Errorf("restart session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetPermissionMode records what a session's execs may do without asking.
func (p *Postgres) SetPermissionMode(ctx context.Context, id, mode string) error {
	tag, err := p.pool.Exec(ctx,
		`update sessions set permission_mode = $2, updated_at = now() where id = $1`, id, mode)
	if err != nil {
		return fmt.Errorf("set permission mode: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetLabel records what the operator calls a session. Empty clears it.
func (p *Postgres) SetLabel(ctx context.Context, id, label string) error {
	tag, err := p.pool.Exec(ctx,
		`update sessions set label = $2, updated_at = now() where id = $1`, id, label)
	if err != nil {
		return fmt.Errorf("set label: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetDescription records what the system observed a session to be, with the exec count it was written
// at, in one statement so the two can never disagree about how current it is.
func (p *Postgres) SetDescription(ctx context.Context, id, description string, atExec int) error {
	tag, err := p.pool.Exec(ctx,
		`update sessions set description = $2, described_at_exec = $3, updated_at = now() where id = $1`,
		id, description, atExec)
	if err != nil {
		return fmt.Errorf("set description: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CountExecs is how many execs a session has had.
func (p *Postgres) CountExecs(ctx context.Context, session string) (int, error) {
	var count int
	if err := p.pool.QueryRow(ctx,
		`select count(*) from execs where session = $1`, session).Scan(&count); err != nil {
		return 0, fmt.Errorf("count execs: %w", err)
	}
	return count, nil
}

// ArchiveSession stamps a session as put away. Nothing is deleted, which is the whole point: the row,
// the conversation handle and the files on the host are all untouched, so restoring is one update.
//
// The status is a condition on the update rather than a read before it. A read then a write lets an
// exec start between the two, and the session is then put away with an open exec in it.
func (p *Postgres) ArchiveSession(ctx context.Context, id string) error {
	tag, err := p.pool.Exec(ctx,
		`update sessions set archived_at = now(), skills_fingerprint = '', updated_at = now()
		  where id = $1 and archived_at is null and status <> $2`, id, StatusRunning)
	if err != nil {
		return fmt.Errorf("archive session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return p.whyNotArchived(ctx, id)
	}
	return nil
}

// whyNotArchived says which of the two a zero row update was. It runs only after the write refused,
// so it decides the wording rather than the outcome: nothing it reads can turn a refusal into an
// archive.
func (p *Postgres) whyNotArchived(ctx context.Context, id string) error {
	var held string
	err := p.pool.QueryRow(ctx, `select status from sessions where id = $1`, id).Scan(&held)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("archive session: %w", err)
	}
	if held == StatusRunning {
		return ErrSessionHoldsAnExec
	}
	// The row is there and was not stamped, so it was stamped already. A caller that reads the row
	// first says the plainer thing; this one has nothing left to distinguish.
	return ErrNotFound
}

// ArchiveProjectSessions puts away every session of one project that holds no container.
//
// One statement, so a dispatch landing during the sweep either runs before it and keeps its session
// out, or runs after it. The stamps it writes are all the same instant, so ordering what came back by
// the stamp and then by the identifier is the order the archived listing draws them in.
func (p *Postgres) ArchiveProjectSessions(ctx context.Context, project string) ([]string, error) {
	if _, err := p.GetProject(ctx, project); err != nil {
		return nil, err
	}
	rows, err := p.pool.Query(ctx, `
		with put_away as (
			update sessions set archived_at = now(), skills_fingerprint = '', updated_at = now()
			where project = $1 and archived_at is null and status = any($2)
			returning id, archived_at
		)
		select id from put_away order by archived_at desc, id`, project, settledStatuses())
	if err != nil {
		return nil, fmt.Errorf("archive project sessions: %w", err)
	}
	defer rows.Close()

	archived := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("archive project sessions: %w", err)
		}
		archived = append(archived, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("archive project sessions: %w", err)
	}
	return archived, nil
}

// RestoreSession clears the stamp, bringing the session back into the default listing.
func (p *Postgres) RestoreSession(ctx context.Context, id string) error {
	return p.stampArchived(ctx, id, `archived_at = null`)
}

// stampArchived is the update restoring is. The clause is a constant from the caller above and never
// carries a value, so nothing here is built from input.
func (p *Postgres) stampArchived(ctx context.Context, id, clause string) error {
	tag, err := p.pool.Exec(ctx,
		`update sessions set `+clause+`, updated_at = now() where id = $1`, id)
	if err != nil {
		return fmt.Errorf("archive session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Pool is the connection this store holds, so another durable thing can sit beside it rather than
// opening a second connection to the same database for the same system.
func (p *Postgres) Pool() *pgxpool.Pool { return p.pool }

// GetContext returns what the model should be told at a scope. Nothing written is the normal state
// and comes back empty rather than as an error.
func (p *Postgres) GetContext(ctx context.Context, scope ContextScope, owner string) (string, error) {
	var body string
	err := p.pool.QueryRow(ctx,
		`select body from contexts where scope = $1 and owner = $2`, string(scope), owner).Scan(&body)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get context: %w", err)
	}
	return body, nil
}

// SetContext records what the model should be told at a scope.
func (p *Postgres) SetContext(ctx context.Context, scope ContextScope, owner, body string) error {
	_, err := p.pool.Exec(ctx, `
		insert into contexts (scope, owner, body) values ($1, $2, $3)
		on conflict (scope, owner) do update set body = excluded.body, updated_at = now()`,
		string(scope), owner, body)
	if err != nil {
		return fmt.Errorf("set context: %w", err)
	}
	return nil
}

// designColumns is what every design read selects, in the order scanDesign reads them. The two are
// written next to each other because a column added to one and not the other reads as a zero rather
// than as a failure.
const designColumns = `project, brief, body, approved, approved_at, written_by, updated_at, contracts, ` +
	`steps_in_flight_cap, proof_command, proof_count_pattern, proof_timeout_seconds`

// scanDesign reads one design row.
func scanDesign(row pgx.Row) (*quaycrewv1.Design, error) {
	var (
		project, brief, body, writtenBy, contracts string
		approved                                   bool
		approvedAt                                 *time.Time
		updatedAt                                  time.Time
		stepsInFlightCap                           int32
		proofCommand, proofCountPattern            string
		proofTimeoutSeconds                        int32
	)
	if err := row.Scan(&project, &brief, &body, &approved, &approvedAt, &writtenBy, &updatedAt,
		&contracts, &stepsInFlightCap, &proofCommand, &proofCountPattern, &proofTimeoutSeconds); err != nil {
		return nil, err
	}
	design := &quaycrewv1.Design{
		Project:             project,
		Brief:               brief,
		Body:                body,
		Approved:            approved,
		WrittenBy:           writtenBy,
		UpdatedAt:           timestamppb.New(updatedAt),
		Contracts:           contracts,
		StepsInFlightCap:    stepsInFlightCap,
		ProofCommand:        proofCommand,
		ProofCountPattern:   proofCountPattern,
		ProofTimeoutSeconds: proofTimeoutSeconds,
	}
	if approvedAt != nil {
		design.ApprovedAt = timestamppb.New(*approvedAt)
	}
	return design, nil
}

// projectExists says whether the project is there to hang a design on. Every design call asks first,
// because an insert against a missing project fails on the foreign key with a message about a
// constraint, and the caller needs to be told the project is not there.
//
// It asks the same question GetProject asks, join and both conditions. A project here is deleted by
// a stamp rather than by removing the row, so the foreign key cascade never fires for one, and a
// check that only looked for the row would keep answering for a project nobody can reach.
func (p *Postgres) projectExists(ctx context.Context, project string) error {
	var one int
	err := p.pool.QueryRow(ctx, `
		select 1 from projects p join workspaces w on w.id = p.workspace
		where p.id = $1 and p.deleted_at is null and w.deleted_at is null`, project).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read project: %w", err)
	}
	return nil
}

// GetDesign returns the project's design. A project with no design row is the normal state and
// answers with a Design carrying its identifier and every default the row would have carried.
//
// The defaults are answered rather than left at zero, because each zero is a value a reader would act
// on: a cap of zero refuses every take, an empty pattern reads no count out of any output, and a
// budget of zero ends a run before it starts.
func (p *Postgres) GetDesign(ctx context.Context, project string) (*quaycrewv1.Design, error) {
	if err := p.projectExists(ctx, project); err != nil {
		return nil, err
	}
	design, err := scanDesign(p.pool.QueryRow(ctx,
		`select `+designColumns+` from project_designs where project = $1`, project))
	if errors.Is(err, pgx.ErrNoRows) {
		return &quaycrewv1.Design{
			Project:             project,
			StepsInFlightCap:    DefaultStepsInFlightCap,
			ProofCountPattern:   DefaultProofCountPattern,
			ProofTimeoutSeconds: DefaultProofTimeoutSeconds,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get design: %w", err)
	}
	return design, nil
}

// SetProjectBrief records what a project is for. It leaves the body and its writer alone: a brief
// says nothing about what was designed.
func (p *Postgres) SetProjectBrief(ctx context.Context, project, brief string) (*quaycrewv1.Design, error) {
	if err := p.projectExists(ctx, project); err != nil {
		return nil, err
	}
	design, err := scanDesign(p.pool.QueryRow(ctx, `
		insert into project_designs (project, brief) values ($1, $2)
		on conflict (project) do update set brief = excluded.brief, updated_at = now()
		returning `+designColumns, project, brief))
	if err != nil {
		return nil, fmt.Errorf("set brief: %w", err)
	}
	return design, nil
}

// SetProjectDesign records the design document whole, and who wrote it. It leaves the brief alone.
//
// The approval is cleared in the same statement as the body, because approval is a statement about
// one text. Two statements would leave a moment where the row says approved over a body nobody has
// read, and a reader in that moment is told the wrong thing.
func (p *Postgres) SetProjectDesign(ctx context.Context, project, body, writtenBy string) (*quaycrewv1.Design, error) {
	if err := p.projectExists(ctx, project); err != nil {
		return nil, err
	}
	design, err := scanDesign(p.pool.QueryRow(ctx, `
		insert into project_designs (project, body, written_by) values ($1, $2, $3)
		on conflict (project) do update set
			body = excluded.body, written_by = excluded.written_by,
			approved = false, approved_at = null, updated_at = now()
		returning `+designColumns, project, body, writtenBy))
	if err != nil {
		return nil, fmt.Errorf("set design: %w", err)
	}
	return design, nil
}

// SetProjectContracts records the contracts document whole, and who wrote it. It leaves the design
// body and the brief alone.
//
// The approval is not in this statement, and that is the point of the method. SetProjectDesign
// clears it because approval is a statement about one text; the contracts document is a different
// text on the same row, read out of the design the operator already approved, so a contract written
// down is not a design that changed.
//
// An empty body is written like any other. It is how a project says it carries no contracts
// document, and the render takes the file away when it reads one.
func (p *Postgres) SetProjectContracts(ctx context.Context, project, body, writtenBy string) (*quaycrewv1.Design, error) {
	if err := p.projectExists(ctx, project); err != nil {
		return nil, err
	}
	design, err := scanDesign(p.pool.QueryRow(ctx, `
		insert into project_designs (project, contracts, written_by) values ($1, $2, $3)
		on conflict (project) do update set
			contracts = excluded.contracts, written_by = excluded.written_by, updated_at = now()
		returning `+designColumns, project, body, writtenBy))
	if err != nil {
		return nil, fmt.Errorf("set contracts: %w", err)
	}
	return design, nil
}

// ApproveProjectDesign records the operator's word on the design as it stands.
//
// The body is read in the statement that writes the approval, rather than in a read before it, so a
// design emptied between the two cannot come back approved. A row that is not there, and a row with
// no body, both return no rows, and both mean there is nothing to approve.
func (p *Postgres) ApproveProjectDesign(ctx context.Context, project string) (*quaycrewv1.Design, error) {
	if err := p.projectExists(ctx, project); err != nil {
		return nil, err
	}
	design, err := scanDesign(p.pool.QueryRow(ctx, `
		update project_designs set approved = true, approved_at = now(), updated_at = now()
		where project = $1 and body <> ''
		returning `+designColumns, project))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNothingToApprove
	}
	if err != nil {
		return nil, fmt.Errorf("approve design: %w", err)
	}
	return design, nil
}

// SetStepsInFlightCap records how many steps of one project may be in state taken at one time, and
// creates the row on first use the way every other design write does.
//
// The number is kept as it is given, for the reason FinishStep keeps the word it is given: the
// control plane refuses one outside the bounds, and a second check here is a second place for the
// bounds to drift.
//
// No approval and no trust column moves. The cap governs how much runs at once, and says nothing
// about what the design body means.
func (p *Postgres) SetStepsInFlightCap(ctx context.Context, project string, atOnce int32) (
	*quaycrewv1.Design, error) {
	if err := p.projectExists(ctx, project); err != nil {
		return nil, err
	}
	design, err := scanDesign(p.pool.QueryRow(ctx, `
		insert into project_designs (project, steps_in_flight_cap) values ($1, $2)
		on conflict (project) do update set steps_in_flight_cap = excluded.steps_in_flight_cap,
			updated_at = now()
		returning `+designColumns, project, atOnce))
	if err != nil {
		return nil, fmt.Errorf("set the cap: %w", err)
	}
	return design, nil
}

// SetProofCommand records what one scenario run looks like in this project, and creates the row on
// first use the way every other design write does.
//
// An empty value leaves that setting where it is, and the choice is made inside the one statement
// rather than by reading the row first. A read and then a write would let a second caller land
// between them, and the pattern this caller meant to keep would be the one it read rather than the
// one on the row.
//
// On first use there is nothing to keep, so an empty value takes the column default. The defaults are
// passed in rather than left to the column, because this is one insert and a statement cannot ask for
// the default of one column and a value for another in the same row.
//
// The three values are kept as they are given, for the reason the cap is: the control plane refuses a
// command that names no scenario, a pattern that does not compile and a budget outside the bounds,
// and a second check here is a second place for those rules to drift.
//
// No approval and no trust column moves. A proof command says how a step is run, and nothing about
// what the design body means.
func (p *Postgres) SetProofCommand(ctx context.Context, project string, settings ProofSettings) (
	*quaycrewv1.Design, error) {
	if err := p.projectExists(ctx, project); err != nil {
		return nil, err
	}
	design, err := scanDesign(p.pool.QueryRow(ctx, `
		insert into project_designs (project, proof_command, proof_count_pattern, proof_timeout_seconds)
		values ($1, $2, coalesce(nullif($3, ''), $5), coalesce(nullif($4, 0), $6))
		on conflict (project) do update set
			proof_command = coalesce(nullif($2, ''), project_designs.proof_command),
			proof_count_pattern = coalesce(nullif($3, ''), project_designs.proof_count_pattern),
			proof_timeout_seconds = coalesce(nullif($4, 0), project_designs.proof_timeout_seconds),
			updated_at = now()
		returning `+designColumns,
		project, settings.Command, settings.CountPattern, settings.TimeoutSeconds,
		DefaultProofCountPattern, DefaultProofTimeoutSeconds))
	if err != nil {
		return nil, fmt.Errorf("set the proof command: %w", err)
	}
	return design, nil
}

// stepColumns is what every path read selects, in the order scanStep reads them. The two are written
// next to each other because a column added to one and not the other reads as a zero rather than as
// a failure.
//
// Qualified, because every read joins the feature, its project and that project's workspace to hide
// a deleted project's path, and an unqualified name would be one the join could make ambiguous
// later.
const stepColumns = `s.feature, s.number, s.title, s.intention, s.touches, s.proof, ` +
	`s.proof_scenario, s.after, s.milestone, s.contracts, s.contract_scope, ` +
	`s.state, s.session, s.result, s.closed_by, s.taken_at, s.finished_at, ` +
	`s.restatement, s.restated_at, s.restatement_approved, s.restatement_approved_at`

// stepJoins is the join every path read goes through: a step to its feature, that feature to its
// project, and that project to its workspace.
//
// A project here is deleted by a stamp rather than by removing the row, so the foreign key cascade
// never fires for one, and a read that matched on the feature alone would keep answering with the
// path of a project nobody can reach.
const stepJoins = `join features f on f.id = s.feature ` +
	`join projects p on p.id = f.project ` +
	`join workspaces w on w.id = p.workspace`

// scanStep reads one step row.
func scanStep(row pgx.Row) (*quaycrewv1.Step, error) {
	var (
		feature, title, intention, touches, proof string
		scenario, state, session, result          string
		contracts, contractScope, closedBy        string
		restatement                               string
		number, after, milestone                  int32
		approved                                  bool
		takenAt, finishedAt                       *time.Time
		restatedAt, approvedAt                    *time.Time
	)
	if err := row.Scan(&feature, &number, &title, &intention, &touches, &proof,
		&scenario, &after, &milestone, &contracts, &contractScope,
		&state, &session, &result, &closedBy, &takenAt, &finishedAt,
		&restatement, &restatedAt, &approved, &approvedAt); err != nil {
		return nil, err
	}
	step := &quaycrewv1.Step{
		Feature:       feature,
		Number:        number,
		Title:         title,
		Intention:     intention,
		Touches:       touches,
		Proof:         proof,
		ProofScenario: scenario,
		After:         after,
		Milestone:     milestone,
		Contracts:     contracts,
		ContractScope: contractScope,
		State:         state,
		Session:       session,
		Result:        result,
		ClosedBy:      closedBy,

		Restatement:         restatement,
		RestatementApproved: approved,
	}
	if takenAt != nil {
		step.TakenAt = timestamppb.New(*takenAt)
	}
	if finishedAt != nil {
		step.FinishedAt = timestamppb.New(*finishedAt)
	}
	if restatedAt != nil {
		step.RestatedAt = timestamppb.New(*restatedAt)
	}
	if approvedAt != nil {
		step.RestatementApprovedAt = timestamppb.New(*approvedAt)
	}
	return step, nil
}

// SetPath replaces one feature's path with the steps given.
//
// The delete and the inserts name the feature, so the paths of the project's other features are
// never read and never written. Keyed by the project, this call wiped every path of it.
//
// The whole write is one transaction, so a refused write changes nothing and no reader ever sees
// half a path. The rows are read back through ListSteps rather than returned from the inserts,
// because number order is what a caller is promised and the insert order is whatever the document
// happened to be written in.
func (p *Postgres) SetPath(ctx context.Context, feature string, milestones []Milestone, steps []Step) (
	[]*quaycrewv1.Step, error) {
	if err := p.featureExists(ctx, feature); err != nil {
		return nil, err
	}
	transaction, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin the path: %w", err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()

	// The rows are read and locked inside the transaction, so a take that lands while the document is
	// being written waits for it and is then seen by the refusal below. Read outside the transaction,
	// that take would be deleted by the write that follows.
	held, err := stepsForUpdate(ctx, transaction, feature)
	if err != nil {
		return nil, err
	}
	if lost := protectedSteps(held, steps); len(lost) > 0 {
		return nil, &ProtectedStepsError{Steps: lost}
	}

	// The steps the document does not carry go, and the ones it carries are written over the rows
	// that are there. A delete of the whole path would take the state, the session and the stamps of
	// a protected step with it, and put it back reading ready.
	if _, err := transaction.Exec(ctx,
		`delete from feature_steps where feature = $1 and number <> all($2::int[])`,
		feature, numbersOfSteps(steps)); err != nil {
		return nil, fmt.Errorf("clear the path: %w", err)
	}
	// The milestones are replaced whole beside the steps. A milestone this document does not carry is
	// dropped, and it loses nothing: a milestone holds no state of its own.
	if _, err := transaction.Exec(ctx,
		`delete from milestones where feature = $1`, feature); err != nil {
		return nil, fmt.Errorf("clear the milestones: %w", err)
	}
	for _, milestone := range milestones {
		if _, err := transaction.Exec(ctx, `
			insert into milestones (feature, number, title, intention)
			values ($1, $2, $3, $4)`,
			feature, milestone.Number, milestone.Title, milestone.Intention); err != nil {
			return nil, fmt.Errorf("write milestone %d: %w", milestone.Number, err)
		}
	}
	// The update names the columns the document owns and no others, so the state, the session, the
	// result and the stamps stay as the system wrote them.
	for _, step := range steps {
		if _, err := transaction.Exec(ctx, `
			insert into feature_steps
				(feature, number, title, intention, touches, proof, proof_scenario, after, milestone,
				 contracts, contract_scope)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			on conflict (feature, number) do update set
				title = excluded.title,
				intention = excluded.intention,
				touches = excluded.touches,
				proof = excluded.proof,
				proof_scenario = excluded.proof_scenario,
				after = excluded.after,
				milestone = excluded.milestone,
				contracts = excluded.contracts,
				contract_scope = excluded.contract_scope,
				updated_at = now()`,
			feature, step.Number, step.Title, step.Intention, step.Touches, step.Proof,
			step.ProofScenario, step.After, step.Milestone,
			step.Contracts, step.ContractScope); err != nil {
			return nil, fmt.Errorf("write step %d: %w", step.Number, err)
		}
	}
	if err := transaction.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit the path: %w", err)
	}
	return p.ListSteps(ctx, feature)
}

// stepsForUpdate is a feature's path as it stands, read inside a transaction and locked against a
// take landing while the document is written. Only the step rows are locked: the feature, the
// project and the workspace are joined to hide a deleted project and nothing here writes them.
func stepsForUpdate(ctx context.Context, transaction pgx.Tx, feature string) ([]*quaycrewv1.Step, error) {
	rows, err := transaction.Query(ctx, `
		select `+stepColumns+` from feature_steps s
		`+stepJoins+`
		where s.feature = $1 and p.deleted_at is null and w.deleted_at is null
		order by s.number
		for update of s`, feature)
	if err != nil {
		return nil, fmt.Errorf("read the path: %w", err)
	}
	defer rows.Close()

	held := make([]*quaycrewv1.Step, 0)
	for rows.Next() {
		step, err := scanStep(rows)
		if err != nil {
			return nil, fmt.Errorf("read step: %w", err)
		}
		held = append(held, step)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the path: %w", err)
	}
	return held, nil
}

// numbersOfSteps is what the document carries, which is what the delete keeps.
func numbersOfSteps(steps []Step) []int32 {
	numbers := make([]int32, 0, len(steps))
	for _, step := range steps {
		numbers = append(numbers, step.Number)
	}
	return numbers
}

// featureExists says whether a feature is one a caller can reach, which is the check every step call
// opens with. The conditions are projectExists's, one join further down.
func (p *Postgres) featureExists(ctx context.Context, feature string) error {
	var one int
	err := p.pool.QueryRow(ctx, `
		select 1 from features f
		join projects p on p.id = f.project
		join workspaces w on w.id = p.workspace
		where f.id = $1 and p.deleted_at is null and w.deleted_at is null`, feature).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read feature: %w", err)
	}
	return nil
}

// ListSteps returns a feature's path in number order, or every feature's when it names none.
func (p *Postgres) ListSteps(ctx context.Context, feature string) ([]*quaycrewv1.Step, error) {
	if feature != "" {
		if err := p.featureExists(ctx, feature); err != nil {
			return nil, err
		}
	}
	rows, err := p.pool.Query(ctx, `
		select `+stepColumns+` from feature_steps s
		`+stepJoins+`
		where ($1 = '' or s.feature = $1)
		  and p.deleted_at is null and w.deleted_at is null
		order by s.feature, s.number`, feature)
	if err != nil {
		return nil, fmt.Errorf("list steps: %w", err)
	}
	defer rows.Close()

	steps := make([]*quaycrewv1.Step, 0)
	for rows.Next() {
		step, err := scanStep(rows)
		if err != nil {
			return nil, fmt.Errorf("read step: %w", err)
		}
		steps = append(steps, step)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list steps: %w", err)
	}
	return steps, nil
}

// ListMilestones returns a feature's milestones in number order.
//
// The join is the one the path reads go through, so a deleted project answers about its milestones
// the same way it answers about its steps.
func (p *Postgres) ListMilestones(ctx context.Context, feature string) ([]*quaycrewv1.Milestone, error) {
	if err := p.featureExists(ctx, feature); err != nil {
		return nil, err
	}
	rows, err := p.pool.Query(ctx, `
		select m.feature, m.number, m.title, m.intention from milestones m
		join features f on f.id = m.feature
		join projects p on p.id = f.project
		join workspaces w on w.id = p.workspace
		where m.feature = $1 and p.deleted_at is null and w.deleted_at is null
		order by m.number`, feature)
	if err != nil {
		return nil, fmt.Errorf("list milestones: %w", err)
	}
	defer rows.Close()

	milestones := make([]*quaycrewv1.Milestone, 0)
	for rows.Next() {
		var (
			held             string
			number           int32
			title, intention string
		)
		if err := rows.Scan(&held, &number, &title, &intention); err != nil {
			return nil, fmt.Errorf("read milestone: %w", err)
		}
		milestones = append(milestones, &quaycrewv1.Milestone{
			Feature: held, Number: number, Title: title, Intention: intention})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list milestones: %w", err)
	}
	return milestones, nil
}

// GetStep returns one step of a feature's path.
//
// The join is the one ListSteps uses, so a deleted project answers about its path the same way
// whichever call asks.
func (p *Postgres) GetStep(ctx context.Context, feature string, number int32) (*quaycrewv1.Step, error) {
	if err := p.featureExists(ctx, feature); err != nil {
		return nil, err
	}
	step, err := scanStep(p.pool.QueryRow(ctx, `
		select `+stepColumns+` from feature_steps s
		`+stepJoins+`
		where s.feature = $1 and s.number = $2
		  and p.deleted_at is null and w.deleted_at is null`, feature, number))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get step: %w", err)
	}
	return step, nil
}

// TakeStep gives a ready step to a session, and says how many steps of the project run once it lands.
//
// The step is addressed by its feature, and step 3 of one feature is a different step from step 3 of
// another, so taking one leaves the other ready. Neither limit is addressed that way: the cap and the
// files both belong to the project, and both join the steps to the features on it. A query filtered by
// feature would let a project with five features run five times the number the operator set, and would
// let two features write one file at the same moment.
//
// The whole thing is one transaction that first locks the project row, so two takes at one moment are
// done one after the other and cannot both pass a cap with room for one of them, or both pass a file
// only one of them may write. The lock is on the project because that is what both limits are about:
// two projects each take their own steps and never wait for each other.
//
// The state is read in the statement that writes it, so two callers racing for one step cannot both
// be told they have it. A write that matched no row is then read back to say which of the two things
// happened: there is no such step, or somebody already holds it.
func (p *Postgres) TakeStep(ctx context.Context, feature string, number int32, session string) (
	*quaycrewv1.Step, int32, error) {
	if err := p.featureExists(ctx, feature); err != nil {
		return nil, 0, err
	}
	transaction, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("begin the take: %w", err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()

	var project string
	if err := transaction.QueryRow(ctx, `
		select p.id from features f
		join projects p on p.id = f.project
		where f.id = $1
		for update of p`, feature).Scan(&project); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, fmt.Errorf("hold the project: %w", err)
	}

	// The step is read and locked before anything is counted, so a step somebody already holds is
	// refused for that and never for the cap. A full project would otherwise answer a second take on
	// one step with "finish one to make room", which sends the operator to fix the wrong thing.
	var state, touches string
	err = transaction.QueryRow(ctx,
		`select s.state, s.touches from feature_steps s where s.feature = $1 and s.number = $2 for update of s`,
		feature, number).Scan(&state, &touches)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, fmt.Errorf("read the step: %w", err)
	}
	if state != StepReady {
		return nil, 0, ErrStepNotReady
	}

	flying, err := stepsInFlight(ctx, transaction, project)
	if err != nil {
		return nil, 0, err
	}
	atOnce, err := stepsInFlightCap(ctx, transaction, project)
	if err != nil {
		return nil, 0, err
	}
	if int32(len(flying)) >= atOnce {
		return nil, 0, &StepsInFlightError{Steps: flying, Cap: atOnce}
	}
	// The same rows the cap counted, read for what each one writes. The project row is locked above,
	// so a take that would share a file cannot land between this read and the write below.
	if shared := SharesAFile(touches, flying); shared != nil {
		return nil, 0, shared
	}

	step, err := scanStep(transaction.QueryRow(ctx, `
		update feature_steps s
		set state = $4, session = $3, taken_at = now(), updated_at = now()
		where s.feature = $1 and s.number = $2 and s.state = $5
		returning `+stepColumns, feature, number, session, StepTaken, StepReady))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, ErrStepNotReady
	}
	if err != nil {
		return nil, 0, fmt.Errorf("take step: %w", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return nil, 0, fmt.Errorf("commit the take: %w", err)
	}
	return step, int32(len(flying)) + 1, nil
}

// stepsInFlight is every step of one project in state taken, with the feature each one sits in and
// what each one says it writes, by feature number and then step number.
//
// The order is the store's rather than the caller's, so the refusal a person reads names the same
// three steps in the same order however the rows happen to be laid out. The file check reads this list
// too, so the order decides which collision a refused take names.
func stepsInFlight(ctx context.Context, transaction pgx.Tx, project string) ([]StepInFlight, error) {
	rows, err := transaction.Query(ctx, `
		select s.number, f.number, f.title, s.touches
		from feature_steps s
		join features f on f.id = s.feature
		where f.project = $1 and s.state = $2
		order by f.number, s.number`, project, StepTaken)
	if err != nil {
		return nil, fmt.Errorf("count the steps in flight: %w", err)
	}
	defer rows.Close()

	flying := make([]StepInFlight, 0)
	for rows.Next() {
		var held StepInFlight
		if err := rows.Scan(&held.Number, &held.FeatureNumber, &held.FeatureTitle, &held.Touches); err != nil {
			return nil, fmt.Errorf("read a step in flight: %w", err)
		}
		flying = append(flying, held)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the steps in flight: %w", err)
	}
	return flying, nil
}

// stepsInFlightCap is how many steps this project may hold in state taken at one time. A project with
// no design row has set no cap and reads the default, which is what the column would have given it.
func stepsInFlightCap(ctx context.Context, transaction pgx.Tx, project string) (int32, error) {
	var atOnce int32
	err := transaction.QueryRow(ctx,
		`select steps_in_flight_cap from project_designs where project = $1`, project).Scan(&atOnce)
	if errors.Is(err, pgx.ErrNoRows) {
		return DefaultStepsInFlightCap, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read the cap: %w", err)
	}
	return atOnce, nil
}

// FinishStep records what came of a step: the word that closes it, what somebody wrote, who spoke
// the word, and the stamp.
//
// The state and the stamp move in one statement, so a step can never read as done with no time on it.
// The word is kept as it is given, for the reason FinishFeature keeps a feature's: the control plane
// refuses a word outside the two, and a second check here is a second place for the vocabulary to
// drift.
//
// The session and the take stamp are left where they are, so the row still says who took the step. No
// session is read, stopped or reclaimed: the step and the session are separate records.
func (p *Postgres) FinishStep(ctx context.Context, feature string, number int32, finish Finish) (*quaycrewv1.Step, error) {
	if err := p.featureExists(ctx, feature); err != nil {
		return nil, err
	}
	step, err := scanStep(p.pool.QueryRow(ctx, `
		update feature_steps s
		set state = $3, result = $4, closed_by = $5, finished_at = now(), updated_at = now()
		where s.feature = $1 and s.number = $2
		returning `+stepColumns, feature, number, finish.State, finish.Result, finish.ClosedBy))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finish step: %w", err)
	}
	return step, nil
}

// SetRestatement records what the session wrote about a step before it built anything.
//
// The text, its stamp and the cleared approval move in one statement, so no reader ever sees a step
// carrying a new restatement under the approval of the text before it.
//
// The approval is cleared whatever the text says. The store cannot tell an unchanged text from a
// rewritten one that reads the same, so the caller skips the call when nothing changed and this
// clears every time it is called.
func (p *Postgres) SetRestatement(ctx context.Context, feature string, number int32, text string) (
	*quaycrewv1.Step, error) {
	if err := p.featureExists(ctx, feature); err != nil {
		return nil, err
	}
	step, err := scanStep(p.pool.QueryRow(ctx, `
		update feature_steps s
		set restatement = $3, restated_at = now(),
			restatement_approved = false, restatement_approved_at = null, updated_at = now()
		where s.feature = $1 and s.number = $2
		returning `+stepColumns, feature, number, text))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("set the restatement: %w", err)
	}
	return step, nil
}

// ApproveRestatement records the operator's word on the restatement as it stands.
//
// The text is read in the statement that writes the approval, rather than in a read before it, so a
// step restated between the two cannot come back approved under a text nobody read. The step is read
// again only to say which refusal a write of no rows earned: a step that is not there, and a step
// whose session wrote nothing, are two different things to the person who typed the command.
//
// Approving one that is already approved moves the stamp, and no proof column moves at all.
func (p *Postgres) ApproveRestatement(ctx context.Context, feature string, number int32) (
	*quaycrewv1.Step, error) {
	if err := p.featureExists(ctx, feature); err != nil {
		return nil, err
	}
	step, err := scanStep(p.pool.QueryRow(ctx, `
		update feature_steps s
		set restatement_approved = true, restatement_approved_at = now(), updated_at = now()
		where s.feature = $1 and s.number = $2 and s.restatement <> ''
		returning `+stepColumns, feature, number))
	if errors.Is(err, pgx.ErrNoRows) {
		if _, missing := p.GetStep(ctx, feature, number); missing != nil {
			return nil, missing
		}
		return nil, ErrNothingRestated
	}
	if err != nil {
		return nil, fmt.Errorf("approve the restatement: %w", err)
	}
	return step, nil
}

// The narrowed parts of a project: the listing, the add that gives the number, and the one line
// saying what a feature narrows to.

// featureColumns is what every feature read selects, in the order scanFeature reads them. The two are
// written next to each other because a column added to one and not the other reads as a zero rather
// than as a failure.
//
// Qualified, because every read joins the project and its workspace to hide a deleted project's
// features, and an unqualified name would be one the join could make ambiguous later.
const featureColumns = `f.id, f.project, f.number, f.title, f.intention, f.state, ` +
	`f.created_at, f.updated_at`

// scanFeature reads one feature row.
func scanFeature(row pgx.Row) (*quaycrewv1.Feature, error) {
	var (
		id, project, title, intention, state string
		number                               int32
		createdAt, updatedAt                 time.Time
	)
	if err := row.Scan(&id, &project, &number, &title, &intention, &state,
		&createdAt, &updatedAt); err != nil {
		return nil, err
	}
	return &quaycrewv1.Feature{
		Id:        id,
		Project:   project,
		Number:    number,
		Title:     title,
		Intention: intention,
		State:     state,
		CreatedAt: timestamppb.New(createdAt),
		UpdatedAt: timestamppb.New(updatedAt),
	}, nil
}

// GetFeature returns one feature, and ErrNotFound for one nobody can reach.
//
// The step calls take a feature, and the design and its approval belong to the project, so this is
// what the control plane reads to get from one to the other.
func (p *Postgres) GetFeature(ctx context.Context, feature string) (*quaycrewv1.Feature, error) {
	held, err := scanFeature(p.pool.QueryRow(ctx, `
		select `+featureColumns+` from features f
		join projects p on p.id = f.project
		join workspaces w on w.id = p.workspace
		where f.id = $1 and p.deleted_at is null and w.deleted_at is null`, feature))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get feature: %w", err)
	}
	return held, nil
}

// ListFeatures returns a project's features in number order, or every project's when it names none.
//
// The join is the one ListSteps uses, and it is there for the same reason: a project here is deleted
// by a stamp rather than by removing the row, so the foreign key cascade never fires for one, and a
// read that only matched on the project identifier would answer with the features of a project
// nobody can reach.
func (p *Postgres) ListFeatures(ctx context.Context, project string) ([]*quaycrewv1.Feature, error) {
	if project != "" {
		if err := p.projectExists(ctx, project); err != nil {
			return nil, err
		}
	}
	rows, err := p.pool.Query(ctx, `
		select `+featureColumns+` from features f
		join projects p on p.id = f.project
		join workspaces w on w.id = p.workspace
		where ($1 = '' or f.project = $1)
		  and p.deleted_at is null and w.deleted_at is null
		order by f.project, f.number`, project)
	if err != nil {
		return nil, fmt.Errorf("list features: %w", err)
	}
	defer rows.Close()

	features := make([]*quaycrewv1.Feature, 0)
	for rows.Next() {
		feature, err := scanFeature(rows)
		if err != nil {
			return nil, fmt.Errorf("read feature: %w", err)
		}
		features = append(features, feature)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list features: %w", err)
	}
	return features, nil
}

// AddFeature gives a project one more narrowed part of itself.
//
// The highest number is read in the statement that inserts, so two callers adding at one moment
// cannot both be handed the same number, and the unique constraint on (project, number) is what
// stands behind it. A select with an aggregate answers one row over an empty table, so the first
// feature of a project takes 1 without a branch for the empty case.
//
// The count is over every state, so a feature that stopped keeps its number and the next one is
// higher: numbers are never reused.
func (p *Postgres) AddFeature(ctx context.Context, project, title string) (*quaycrewv1.Feature, error) {
	if err := p.projectExists(ctx, project); err != nil {
		return nil, err
	}
	feature, err := scanFeature(p.pool.QueryRow(ctx, `
		insert into features as f (id, project, number, title)
		select $1, $2, coalesce(max(number), 0) + 1, $3 from features where project = $2
		returning `+featureColumns, NewID(), project, title))
	if err != nil {
		return nil, fmt.Errorf("add feature: %w", err)
	}
	return feature, nil
}

// SetFeatureIntention records which part of the project a feature narrows to.
//
// The join is the one every other feature read makes, so a feature of a deleted project is not found
// rather than written to.
func (p *Postgres) SetFeatureIntention(ctx context.Context, feature, intention string) (*quaycrewv1.Feature, error) {
	written, err := scanFeature(p.pool.QueryRow(ctx, `
		update features f set intention = $2, updated_at = now()
		from projects p join workspaces w on w.id = p.workspace
		where f.id = $1 and p.id = f.project
		  and p.deleted_at is null and w.deleted_at is null
		returning `+featureColumns, feature, intention))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("set feature intention: %w", err)
	}
	return written, nil
}

// FinishFeature writes a feature's state, and that column alone.
//
// The word is written as it is given. This store judges none of the three, because the control plane
// refuses a word outside them and a check in both places is two places to disagree.
//
// The join is the one every other feature write makes, so a feature of a deleted project is not found
// rather than written to. Nothing here reads the steps: closing a feature keeps its path whole, and
// what is still open under it is a warning the control plane composes.
func (p *Postgres) FinishFeature(ctx context.Context, feature, state string) (*quaycrewv1.Feature, error) {
	written, err := scanFeature(p.pool.QueryRow(ctx, `
		update features f set state = $2, updated_at = now()
		from projects p join workspaces w on w.id = p.workspace
		where f.id = $1 and p.id = f.project
		  and p.deleted_at is null and w.deleted_at is null
		returning `+featureColumns, feature, state))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finish feature: %w", err)
	}
	return written, nil
}

// sessionBy reads the single session matching a where clause.
func (p *Postgres) sessionBy(ctx context.Context, where string, args ...any) (*quaycrewv1.Session, error) {
	rows, err := p.pool.Query(ctx, `
		select `+sessionColumns+`
		from sessions where `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("get session: %w", err)
		}
		return nil, ErrNotFound
	}
	return scanSession(rows)
}

func scanSession(rows pgx.Rows) (*quaycrewv1.Session, error) {
	var (
		id, workspace, project, handle, status, modelSessionID string
		createdAt, updatedAt                                   time.Time
		archivedAt, reclaimedAt                                *time.Time
		permissionMode                                         string
		driver                                                 bool
		label, description, title                              string
		describedAtExec                                        int32
	)
	if err := rows.Scan(&id, &workspace, &project, &handle, &status, &modelSessionID,
		&createdAt, &updatedAt, &archivedAt, &reclaimedAt, &permissionMode, &driver, &label,
		&description, &describedAtExec, &title); err != nil {
		return nil, fmt.Errorf("scan session: %w", err)
	}
	session := &quaycrewv1.Session{
		Id:              id,
		Workspace:       workspace,
		Project:         project,
		Handle:          handle,
		Status:          status,
		ModelSessionId:  modelSessionID,
		CreatedAt:       timestamppb.New(createdAt),
		UpdatedAt:       timestamppb.New(updatedAt),
		PermissionMode:  permissionMode,
		Driver:          driver,
		Label:           label,
		Description:     description,
		DescribedAtExec: describedAtExec,
		Title:           title,
	}
	if archivedAt != nil {
		session.ArchivedAt = timestamppb.New(*archivedAt)
	}
	if reclaimedAt != nil {
		session.ReclaimedAt = timestamppb.New(*reclaimedAt)
	}
	return session, nil
}

// AppendExec records an exec. Writing the same one twice is harmless, which is what makes a consumer
// with at least once delivery safe to replay.
func (p *Postgres) AppendExec(ctx context.Context, exec *quaycrewv1.Exec, workspace, project, session string) error {
	if exec.GetId() == "" {
		return errors.New("store: an exec needs an id, so writing the same one twice leaves one exec")
	}
	_, err := p.pool.Exec(ctx, `
		insert into execs (id, session, workspace, project, handle, prompt, reply, status, failure,
			trace_id, occurred_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		on conflict (id) do nothing`,
		exec.GetId(), exec.GetSession(), workspace, project, session,
		exec.GetPrompt(), exec.GetReply(), exec.GetStatus(), exec.GetFailure(), exec.GetTraceId(),
		exec.GetOccurredAt().AsTime())
	if err != nil {
		return fmt.Errorf("append exec: %w", err)
	}
	return nil
}

// FinishExec closes the record an exec opened when it started. A row that is not there is left
// alone: the exec happened whatever the store holds.
func (p *Postgres) FinishExec(ctx context.Context, id, status, reply, failure string) error {
	if id == "" {
		return errors.New("store: an exec needs an id to be finished")
	}
	_, err := p.pool.Exec(ctx, `
		update execs set status = $2, reply = $3, failure = $4
		where id = $1`, id, status, reply, failure)
	if err != nil {
		return fmt.Errorf("finish exec: %w", err)
	}
	return nil
}

// ListExecs returns a session's execs oldest first, capped at limit.
//
// The cap takes the most recent, because the end of a conversation is the part somebody is looking
// for, and then the result is turned back the right way round so it reads as it happened.
func (p *Postgres) ListExecs(ctx context.Context, session string, limit int) ([]*quaycrewv1.Exec, error) {
	rows, err := p.pool.Query(ctx, `
		select id, session, prompt, reply, status, failure, trace_id, occurred_at
		from execs where session = $1
		order by occurred_at desc, id desc
		limit $2`, session, ExecLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("list execs: %w", err)
	}
	defer rows.Close()

	execs := make([]*quaycrewv1.Exec, 0)
	for rows.Next() {
		var exec quaycrewv1.Exec
		var occurredAt time.Time
		if err := rows.Scan(&exec.Id, &exec.Session, &exec.Prompt, &exec.Reply,
			&exec.Status, &exec.Failure, &exec.TraceId, &occurredAt); err != nil {
			return nil, fmt.Errorf("scan exec: %w", err)
		}
		exec.OccurredAt = timestamppb.New(occurredAt)
		execs = append(execs, &exec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list execs: %w", err)
	}
	// Read back newest first so the limit keeps the end of the conversation; hand it over oldest
	// first so it reads in the order it happened.
	for i, j := 0, len(execs)-1; i < j; i, j = i+1, j-1 {
		execs[i], execs[j] = execs[j], execs[i]
	}
	return execs, nil
}

// AppendSessionEvent records one thing that happened to a session. The same event written twice
// leaves one row, which is what the primary key is for.
func (p *Postgres) AppendSessionEvent(ctx context.Context, event *quaycrewv1.SessionEvent) error {
	if event.GetId() == "" {
		return errors.New("store: a session event needs an id, so writing the same one twice leaves one event")
	}
	if event.GetKind() == "" {
		return errors.New("store: a session event needs a kind, which is the field a consumer switches on")
	}
	_, err := p.pool.Exec(ctx, `
		insert into session_events (id, kind, session, workspace, project, handle, detail, occurred_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8)
		on conflict (id) do nothing`,
		event.GetId(), event.GetKind(), event.GetSession(), event.GetWorkspace(),
		event.GetProject(), event.GetHandle(), event.GetDetail(), event.GetOccurredAt().AsTime())
	if err != nil {
		return fmt.Errorf("append session event: %w", err)
	}
	return nil
}

// ListSessionEvents returns a session's lifecycle oldest first, or the whole system's when no session
// is named.
//
// Read newest first so the cap keeps the recent end, then turned back the right way round, the same
// as a history.
func (p *Postgres) ListSessionEvents(ctx context.Context, session string, limit int) ([]*quaycrewv1.SessionEvent, error) {
	rows, err := p.pool.Query(ctx, `
		select id, kind, session, workspace, project, handle, detail, occurred_at
		from session_events
		where $1 = '' or session = $1
		order by occurred_at desc, id desc
		limit $2`, session, ExecLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("list session events: %w", err)
	}
	defer rows.Close()

	events := make([]*quaycrewv1.SessionEvent, 0)
	for rows.Next() {
		var event quaycrewv1.SessionEvent
		var occurredAt time.Time
		if err := rows.Scan(&event.Id, &event.Kind, &event.Session, &event.Workspace,
			&event.Project, &event.Handle, &event.Detail, &occurredAt); err != nil {
			return nil, fmt.Errorf("scan session event: %w", err)
		}
		event.OccurredAt = timestamppb.New(occurredAt)
		events = append(events, &event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list session events: %w", err)
	}
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	return events, nil
}

// FindOrCreateDriver returns the project's driver, the session that drives the system, creating it the
// first time somebody opens it.
//
// One per project, held by a unique index rather than by reading first and writing after: two opened
// at once would otherwise each make one, and the second would be reached by nobody.
func (p *Postgres) FindOrCreateDriver(ctx context.Context, project string) (*quaycrewv1.Session, error) {
	owner, err := p.GetProject(ctx, project)
	if err != nil {
		return nil, err
	}
	// Created dangerous. The driver acts for the operator rather than doing job of its own, and a
	// driver that stops to ask before every step describes the exec instead of doing it. What bounds
	// it is the sandbox, which is the same boundary it would have either way.
	if _, err := p.pool.Exec(ctx, `
		insert into sessions (id, workspace, project, handle, status, driver, permission_mode)
		values ($1, $2, $3, $4, 'idle', true, $5)
		on conflict do nothing`,
		NewID(), owner.GetWorkspace(), project, NewID(), model.PermissionBypass); err != nil {
		return nil, fmt.Errorf("open the driver: %w", err)
	}
	rows, err := p.pool.Query(ctx, `
		select `+sessionColumns+`
		from sessions where project = $1 and driver`, project)
	if err != nil {
		return nil, fmt.Errorf("open the driver: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, ErrNotFound
	}
	return scanSession(rows)
}

// deployTarget is the three columns as a target, and nothing at all when a project has not said.
//
// A project that has not said carries no target rather than a target of three empty strings, so a
// reader asks one question instead of three.
func deployTarget(account, region, identity string) *quaycrewv1.DeployTarget {
	if account == "" && region == "" && identity == "" {
		return nil
	}
	return &quaycrewv1.DeployTarget{Account: account, Region: region, Identity: identity}
}
