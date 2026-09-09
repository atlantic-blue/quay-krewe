package sandbox

import (
	"fmt"
	"path"
	"path/filepath"
)

// Where an address is on the machine, so a person can put a file in front of a session by hand.
//
// The reason this exists: every level on disk is a generated identifier, none of the names is on the
// filesystem, and the only way to learn which directory a sandbox binds where was to run the container
// runtime against a container that happened to be up. With nothing running there was no way to learn
// it from the machine at all.
//
// It reads the same layout the mounts come from, so the two cannot drift into describing different
// directories, and it starts nothing: a settled workspace answers as readily as a busy one.

// DirectoryKind is which directory an address landed on.
type DirectoryKind string

const (
	// KindShared is the workspace's shared folder, which every session in it reads.
	KindShared DirectoryKind = "shared"
	// KindProject is a project's own folder inside that shared one.
	KindProject DirectoryKind = "project"
	// KindWorking is one session's own working directory.
	KindWorking DirectoryKind = "working"
	// KindWorkingTree is the working tree one session took in its workspace's volume. A session that
	// took one works there, and its own directory stays empty.
	KindWorkingTree DirectoryKind = "working tree"
)

// Directory is one place a person can put a file, in the two views that matter to them.
type Directory struct {
	// Host is the directory on the machine running the sandboxes, which is where the file goes.
	Host string
	// Sandbox is where the same directory appears inside a container, which is what a session calls
	// the file once it is in there.
	Sandbox string
	// Kind says which one this is, so an answer can name it.
	Kind DirectoryKind
}

// SharedDirectory is a workspace's shared folder, made if it is not there.
//
// Made rather than reported missing, because the volume is only created when a sandbox starts and the
// question is asked by somebody holding a file: a path that does not exist yet is not somewhere they
// can copy into, and the answer "it will be there once something runs" is the answer they already had.
// Creating it early changes nothing the system does with it, since starting a sandbox creates the same
// directory through the same call.
func (s Storage) SharedDirectory(workspace string) (Directory, error) {
	if s.Dir == "" {
		return Directory{}, ErrNoDirectories
	}
	if err := usableAsPath("workspace", workspace); err != nil {
		return Directory{}, err
	}
	parts := []string{"workspaces", workspace, "volume"}
	if err := makeWritableDir(filepath.Join(append([]string{s.Dir}, parts...)...)); err != nil {
		return Directory{}, err
	}
	return Directory{
		Host:    path.Join(append([]string{s.hostRoot()}, parts...)...),
		Sandbox: SharedPath,
		Kind:    KindShared,
	}, nil
}

// ProjectDirectory is a project's own folder inside its workspace's shared folder, made if it is not
// there, for the reason SharedDirectory is made.
//
// It is named after the project rather than after the project's identifier. This is the one
// directory in the layout a person reads out loud: they type an address and copy a file into what
// comes back, and a session is told to open it by name. Twenty four characters of hexadecimal under
// /home/agent/shared is the identifier this whole feature exists to stop anybody reading.
//
// The names the system already writes at this level are refused at project creation, so the folder
// cannot land on top of a session's working tree or the shared clone of a repository.
func (s Storage) ProjectDirectory(workspace, project string) (Directory, error) {
	if s.Dir == "" {
		return Directory{}, ErrNoDirectories
	}
	for _, part := range []struct{ kind, value string }{
		{"workspace", workspace}, {"project", project},
	} {
		if err := usableAsPath(part.kind, part.value); err != nil {
			return Directory{}, err
		}
	}
	parts := []string{"workspaces", workspace, "volume", project}
	if err := makeWritableDir(filepath.Join(append([]string{s.Dir}, parts...)...)); err != nil {
		return Directory{}, err
	}
	return Directory{
		Host:    path.Join(append([]string{s.hostRoot()}, parts...)...),
		Sandbox: path.Join(SharedPath, project),
		Kind:    KindProject,
	}, nil
}

// WorkingDirectory is one session's own working directory, made if it is not there, for the same
// reason: a session that has never run has no directory yet, and that is exactly when somebody wants
// to leave it something to read.
func (s Storage) WorkingDirectory(cfg Config) (Directory, error) {
	if s.Dir == "" {
		return Directory{}, ErrNoDirectories
	}
	for _, part := range []struct{ kind, value string }{
		{"workspace", cfg.Workspace}, {"project", cfg.Project}, {"session", cfg.ID},
	} {
		if err := usableAsPath(part.kind, part.value); err != nil {
			return Directory{}, err
		}
	}
	parts := []string{"workspaces", cfg.Workspace, "projects", cfg.Project, "sessions", cfg.ID, "workspace"}
	if err := makeWritableDir(filepath.Join(append([]string{s.Dir}, parts...)...)); err != nil {
		return Directory{}, err
	}
	return Directory{
		Host:    path.Join(append([]string{s.hostRoot()}, parts...)...),
		Sandbox: WorkingPath,
		Kind:    KindWorking,
	}, nil
}

// SessionDirectory is where one session's work is: the working tree it took, and its own working
// directory where it took none.
//
// Two roots rather than one, because the git skill teaches two shapes and both are in use. A session
// told to take a working tree writes into the workspace's volume under its own identifier, and its
// own directory stays empty. An address that answered with the empty one would send a person to copy
// a file into a directory the session is not working in, and a listing of it would say the session
// made nothing.
//
// It looks in the same two roots `krewe read` looks in, and answers a different thing inside one of
// them. Read roots at the checkout, because it is handing a file back. This names the directory the
// checkout is in, because the answer is somewhere to put a file, and a file dropped inside a checkout
// is a file in somebody's git status.
func (s Storage) SessionDirectory(cfg Config) (Directory, error) {
	if tree, held := WorkingTree(s.WorkPlaces(cfg)); held {
		return Directory{Host: tree.Host, Sandbox: tree.Sandbox, Kind: tree.Kind}, nil
	}
	return s.WorkingDirectory(cfg)
}

// hostRoot is the data directory as the machine running the sandboxes sees it, falling back to this
// process's own view where the two are the same. A path handed to a person has to be the host's: the
// control plane may be in a container, and its own view of a directory means nothing to anybody typing
// at the machine.
func (s Storage) hostRoot() string {
	if s.Host != "" {
		return s.Host
	}
	return s.Dir
}

// ErrNoDirectories says this system keeps nothing on disk, so there is no directory to name. It is a
// configuration, not a failure: state then lives in the container and dies with it.
var ErrNoDirectories = fmt.Errorf("sandbox: this system keeps no directories on disk")
