package controlplane

import (
	"context"
	"errors"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// LocateDirectory says where an address is on the machine, so a person holding a file can put it in
// front of a session without inspecting a container.
//
// Three directories and no others. The workspace's shared folder, which every session in it reads; a
// project's own folder inside that one; and one session's own working directory. The rest of the
// data directory is never named: its top holds the system's own credentials, and the conversation
// store under a workspace is a transcript rather than a place to put anything.
//
// It starts nothing and reads no container, which is the case that had no answer at all: a bind mount
// can only be read off a container that is up, and the question is usually asked once they are down.
func (s *Server) LocateDirectory(ctx context.Context, req *quaycrewv1.LocateDirectoryRequest) (*quaycrewv1.LocateDirectoryResponse, error) {
	if req.GetWorkspace() == "" {
		return nil, status.Error(codes.InvalidArgument,
			"say which workspace: krewe where <workspace>[/<project>[/<session>]]")
	}
	if _, err := s.store.GetWorkspace(ctx, req.GetWorkspace()); err != nil {
		return nil, storeError(err, "workspace")
	}

	if req.GetSession() == "" {
		if req.GetProject() == "" {
			found, err := s.storage.SharedDirectory(req.GetWorkspace())
			if err != nil {
				return nil, locateError(err)
			}
			return answerFor(found), nil
		}
		return s.projectDirectory(ctx, req)
	}

	session, err := s.sessionAt(ctx, req)
	if err != nil {
		return nil, err
	}
	// The configuration comes off the session rather than off the request, so the answer names the
	// directory that session's own sandbox binds. A session read through the wrong project would
	// otherwise be answered with a path nothing mounts.
	found, err := s.storage.WorkingDirectory(boxOf(session))
	if err != nil {
		return nil, locateError(err)
	}
	return answerFor(found), nil
}

// projectDirectory is a project's own folder inside its workspace's shared folder.
//
// The folder is named after the project, so the name comes off the stored project rather than out of
// the request, which carries the identifier. The two are not interchangeable here: a request naming
// the identifier would be answered with a directory no session is ever told to read.
//
// A project of another workspace is a not found rather than a folder. Answering it would make a
// directory in the wrong volume, which nothing mounts, and hand back a path that stays empty.
func (s *Server) projectDirectory(ctx context.Context, req *quaycrewv1.LocateDirectoryRequest) (*quaycrewv1.LocateDirectoryResponse, error) {
	project, err := s.store.GetProject(ctx, req.GetProject())
	if err != nil {
		return nil, storeError(err, "project")
	}
	if project.GetWorkspace() != req.GetWorkspace() {
		return nil, status.Error(codes.NotFound, "no such project in that workspace")
	}
	found, err := s.storage.ProjectDirectory(req.GetWorkspace(), project.GetName())
	if err != nil {
		return nil, locateError(err)
	}
	return answerFor(found), nil
}

// sessionAt finds the session an address landed on. Either identifier reaches it, because an address
// carries the handle while a listing prints the id, and both are on the operator's screen.
//
// Archived sessions are read too. Putting a session away hides it from a listing and leaves its
// directory exactly where it was, so refusing to name that directory would hide the work as well.
func (s *Server) sessionAt(ctx context.Context, req *quaycrewv1.LocateDirectoryRequest) (*quaycrewv1.Session, error) {
	filter := store.SessionFilter{Workspace: req.GetWorkspace(), Project: req.GetProject()}
	for _, archived := range []bool{false, true} {
		filter.Archived = archived
		sessions, err := s.store.ListSessions(ctx, filter)
		if err != nil {
			return nil, storeError(err, "session")
		}
		for _, session := range sessions {
			if session.GetHandle() == req.GetSession() || session.GetId() == req.GetSession() {
				return session, nil
			}
		}
	}
	return nil, status.Error(codes.NotFound, "no such session in that project")
}

// answerFor renders one directory into the wire message.
//
// An unmapped kind is left unspecified rather than folded into one of the three, because a caller
// reads the kind to say what the directory is: a wrong word there sends somebody to copy a file into
// a folder they were told belongs to something else.
func answerFor(found sandbox.Directory) *quaycrewv1.LocateDirectoryResponse {
	kinds := map[sandbox.DirectoryKind]quaycrewv1.DirectoryKind{
		sandbox.KindShared:  quaycrewv1.DirectoryKind_DIRECTORY_KIND_SHARED,
		sandbox.KindProject: quaycrewv1.DirectoryKind_DIRECTORY_KIND_PROJECT,
		sandbox.KindWorking: quaycrewv1.DirectoryKind_DIRECTORY_KIND_WORKING,
	}
	return &quaycrewv1.LocateDirectoryResponse{
		Host: found.Host, Sandbox: found.Sandbox, Kind: kinds[found.Kind],
	}
}

// locateError separates a system that keeps nothing on disk, which is a way of running it, from a
// directory that could not be made, which is a fault on the machine.
func locateError(err error) error {
	if errors.Is(err, sandbox.ErrNoDirectories) {
		return status.Error(codes.FailedPrecondition,
			"this system keeps a session's state in its container, so there is no directory on the machine to put a file in")
	}
	return status.Errorf(codes.Internal, "%v", err)
}
