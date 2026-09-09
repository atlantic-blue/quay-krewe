package controlplane

import (
	"context"
	"log/slog"
	"sort"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/display"
	"github.com/atlantic-blue/quay-krewe/internal/name"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/store"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// The names an address has on the filesystem, written where a workspace, a project or a session is
// made.
//
// The tree is a view of the store, so it is written from what the store just returned and never from
// what the request said: a request carries identifiers, and a name built from one of those is the
// thing this whole feature exists to stop anybody reading.
//
// Nothing here fails a creation. A workspace that exists with no name on disk is a workspace, and
// refusing to make it because a link could not be written would take the system down over a view.
// The reason goes to the log instead, because a name that quietly did not appear is a person opening
// a folder that is not there.

// nameWorkspace puts a workspace's name in the tree.
func (s *Server) nameWorkspace(ctx context.Context, workspace *quaycrewv1.Workspace) {
	if err := s.storage.NameWorkspace(workspace.GetId(), workspace.GetName()); err != nil {
		slog.WarnContext(ctx, "this workspace has no name on the filesystem",
			"workspace", workspace.GetName(), "error", err)
	}
}

// nameProject makes the folder a project's name reaches, inside its workspace's shared folder.
func (s *Server) nameProject(ctx context.Context, project *quaycrewv1.Project) {
	if err := s.storage.NameProject(project.GetWorkspace(), project.GetName()); err != nil {
		slog.WarnContext(ctx, "this project has no folder of its own yet",
			"project", project.GetName(), "error", err)
	}
}

// nameSession puts a session's name in the tree, under the names of the workspace and the project it
// belongs to.
//
// A session is called what a person calls it: the label, then the title it was dispatched with, then
// the line the system wrote about it. A session nobody has called anything is left out of the tree,
// because the only name left is its identifier and the tree holds names.
func (s *Server) nameSession(ctx context.Context, session *quaycrewv1.Session) {
	called := name.Slugify(display.SessionLabel(session))
	if called == "" {
		return
	}
	workspace, err := s.store.GetWorkspace(ctx, session.GetWorkspace())
	if err != nil {
		slog.WarnContext(ctx, "this session has no name on the filesystem",
			"session", session.GetId(), "error", err)
		return
	}
	project, err := s.store.GetProject(ctx, session.GetProject())
	if err != nil {
		slog.WarnContext(ctx, "this session has no name on the filesystem",
			"session", session.GetId(), "error", err)
		return
	}
	if err := s.storage.NameSession(boxOf(session), sandbox.Names{
		Workspace: workspace.GetName(), Project: project.GetName(), Session: called,
	}); err != nil {
		slog.WarnContext(ctx, "this session has no name on the filesystem",
			"session", session.GetId(), "name", called, "error", err)
	}
}

// SweepNames makes the tree of names say what the store says, and nothing else.
//
// A label changes, a session is put away and a workspace is deleted, and a tree written once then
// points at directories nobody is working in. So it runs at start up, which repairs whatever moved
// while the system was down and names the workspaces that existed before any of this shipped, and it
// runs again after each of those changes, which keeps the tree right in between.
//
// It fails nothing. A name that could not be written is a view that is behind, not a workspace that
// does not exist, and the reason goes to the log so a person opening a folder that is not there can
// read why.
func (s *Server) SweepNames(ctx context.Context) {
	if err := s.storage.WriteNames(s.heldNames(ctx)); err != nil {
		slog.WarnContext(ctx, "the tree of names is not what the system holds", "error", err)
	}
}

// heldNames is every name the store holds, read through the live listings so a deleted workspace and
// the projects under it drop out on their own.
//
// Oldest first at each level, because two workspaces may hold one name and so may two sessions of one
// project. The tree gives the name to the first it is handed, so an order that is the store's map
// iteration would send a file to one workspace this morning and the other one this afternoon.
func (s *Server) heldNames(ctx context.Context) sandbox.Held {
	var held sandbox.Held
	workspaces, err := s.store.ListWorkspaces(ctx)
	if err != nil {
		slog.WarnContext(ctx, "the tree of names could not be read from the store", "error", err)
		return held
	}
	oldestFirst(workspaces, func(w *quaycrewv1.Workspace) (*timestamppb.Timestamp, string) {
		return w.GetCreatedAt(), w.GetId()
	})
	for _, workspace := range workspaces {
		held.Workspaces = append(held.Workspaces, sandbox.WorkspaceName{
			ID: workspace.GetId(), Name: workspace.GetName(),
		})
		held.Sessions = append(held.Sessions, s.heldSessionNames(ctx, workspace)...)
	}
	return held
}

// heldSessionNames is every live session of one workspace, under the name of the project it is in.
//
// A session nobody has called anything is left out, for the reason a new one is: the only name it has
// left is its identifier, and the tree holds names.
func (s *Server) heldSessionNames(ctx context.Context, workspace *quaycrewv1.Workspace) []sandbox.SessionName {
	projects, err := s.store.ListProjects(ctx, workspace.GetId())
	if err != nil {
		slog.WarnContext(ctx, "a workspace's projects are not in the tree of names",
			"workspace", workspace.GetName(), "error", err)
		return nil
	}
	oldestFirst(projects, func(p *quaycrewv1.Project) (*timestamppb.Timestamp, string) {
		return p.GetCreatedAt(), p.GetId()
	})
	var held []sandbox.SessionName
	for _, project := range projects {
		sessions, err := s.store.ListSessions(ctx, store.SessionFilter{Project: project.GetId()})
		if err != nil {
			slog.WarnContext(ctx, "a project's sessions are not in the tree of names",
				"project", project.GetName(), "error", err)
			continue
		}
		oldestFirst(sessions, func(one *quaycrewv1.Session) (*timestamppb.Timestamp, string) {
			return one.GetCreatedAt(), one.GetId()
		})
		for _, session := range sessions {
			called := name.Slugify(display.SessionLabel(session))
			if called == "" {
				continue
			}
			held = append(held, sandbox.SessionName{Config: boxOf(session), Names: sandbox.Names{
				Workspace: workspace.GetName(), Project: project.GetName(), Session: called,
			}})
		}
	}
	return held
}

// oldestFirst puts a listing in the order the tree hands out names in, so the same two rows holding
// one name resolve the same way on every sweep. The identifier breaks a tie, because two rows written
// in the same moment carry the same timestamp.
func oldestFirst[T any](rows []T, at func(T) (*timestamppb.Timestamp, string)) {
	sort.SliceStable(rows, func(i, j int) bool {
		firstAt, firstID := at(rows[i])
		secondAt, secondID := at(rows[j])
		if !firstAt.AsTime().Equal(secondAt.AsTime()) {
			return firstAt.AsTime().Before(secondAt.AsTime())
		}
		return firstID < secondID
	})
}
