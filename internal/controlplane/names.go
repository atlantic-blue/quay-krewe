package controlplane

import (
	"context"
	"log/slog"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/display"
	"github.com/atlantic-blue/quay-krewe/internal/name"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
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
