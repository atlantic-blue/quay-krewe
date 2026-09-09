package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/atlantic-blue/quay-krewe/internal/name"
)

// The names of this system's directories, on the filesystem, so a person opens one by what it is
// called.
//
// Every level on disk is a generated identifier. A workspace's shared folder is
// workspaces/<24 hexadecimal characters>/volume, and a session's own directory is three of those
// deep. The names are in the store and nowhere else, so the only way to open the right folder was to
// ask the tool for a path and paste it back.
//
// The tree is a view of the store. Nothing reads a name back out of it, and no other part of the
// system is told where a link is, so a tree that is deleted costs the names and no work at all.

// SessionsSuffix is what a workspace's directory of session names is called: the workspace's name,
// then this.
//
// A workspace's name is a link to its shared folder, so nothing can hang under it without writing
// into the folder every sandbox binds. A link there points at a path on the host, which is a broken
// name inside every container that reads the folder. The session names go beside the workspace
// instead. No workspace can take the same name, because a name is lowercase letters, digits and
// hyphens, and holds no dot.
const SessionsSuffix = ".sessions"

// Names is one address as a person says it: what the workspace, the project and the session are
// called, rather than the identifiers their directories are named after.
type Names struct {
	Workspace string
	Project   string
	Session   string
}

// NameWorkspace puts a workspace's name in the tree, pointing at its shared folder.
//
// The link is at the workspace and not at the project, and it stays there. A project's folder is
// already named after the project, inside that shared folder, so `<tree>/itv/vast` is the vast
// project's folder through this one link. The path a person types is the same either way, and one
// link per workspace is one name to repair rather than one per project. The tree also cannot then
// disagree with the volume about what a project is called.
func (s Storage) NameWorkspace(id, workspace string) error {
	if s.NameTree == "" || s.Dir == "" {
		return nil
	}
	if err := usableAsName("workspace", workspace); err != nil {
		return err
	}
	shared, err := s.SharedDirectory(id)
	if err != nil {
		return err
	}
	if err := makeWritableDir(s.NameTree); err != nil {
		return err
	}
	return pointAt(filepath.Join(s.NameTree, workspace), shared.Host)
}

// NameProject makes the folder a project's name reaches through its workspace's link, so a project
// nobody has worked in yet is somewhere a file can go the moment it exists.
//
// The folder is the name. Nothing is linked at this level, because a link beside a folder of the
// same name is two paths for one project and one of them would go stale on its own.
func (s Storage) NameProject(workspace, project string) error {
	if s.Dir == "" {
		return nil
	}
	_, err := s.ProjectDirectory(workspace, project)
	return err
}

// NameSession puts a session's name in the tree, pointing at its own working directory.
//
// Under the workspace's session directory and then the project, because two projects in one
// workspace can each hold a session called the same thing.
func (s Storage) NameSession(cfg Config, named Names) error {
	if s.NameTree == "" || s.Dir == "" {
		return nil
	}
	if err := usableAsName("workspace", named.Workspace); err != nil {
		return err
	}
	for _, part := range []struct{ kind, value string }{
		{"project", named.Project}, {"session", named.Session},
	} {
		if err := usableAsPath(part.kind, part.value); err != nil {
			return err
		}
	}
	working, err := s.WorkingDirectory(cfg)
	if err != nil {
		return err
	}
	under := filepath.Join(s.NameTree, named.Workspace+SessionsSuffix, named.Project)
	if err := makeWritableDir(under); err != nil {
		return err
	}
	return pointAt(filepath.Join(under, named.Session), working.Host)
}

// pointAt makes one name in the tree.
//
// A name already pointing at this directory is left alone, so creating the same thing twice writes
// nothing and says nothing. A name pointing somewhere else is refused, and the refusal says where it
// points: two workspaces may hold one name, and taking the name from the first would send a file
// dropped for it into the second one's folder. Repairing a name that went stale is a job for the
// sweep at start up, which reads the store and knows which of the two is right.
//
// A real file or directory under the name is refused for the same reason and one more: this did not
// write it, so removing it is not this view's business.
func pointAt(from, to string) error {
	existing, err := os.Lstat(from)
	switch {
	case err == nil && existing.Mode()&os.ModeSymlink == 0:
		return fmt.Errorf("sandbox: %s is already there and is not a name this wrote", from)
	case err == nil:
		held, err := os.Readlink(from)
		if err != nil {
			return fmt.Errorf("sandbox: read the name %s: %w", from, err)
		}
		if held != to {
			return fmt.Errorf("sandbox: %s is the name of %s already", from, held)
		}
		return nil
	case !os.IsNotExist(err):
		return fmt.Errorf("sandbox: read %s: %w", from, err)
	}
	if err := os.Symlink(to, from); err != nil {
		return fmt.Errorf("sandbox: name %s after %s: %w", from, to, err)
	}
	return nil
}

// usableAsName is usableAsPath plus the one word the top of the tree may never hold.
//
// The system's own directory holds the tokens and the sealing key. No workspace can be called that
// word, so nothing reaches here holding it today, and a tree of names that offered a road to those
// files would be the one place in the system saying they are somewhere to put a file.
func usableAsName(what, value string) error {
	if err := usableAsPath(what, value); err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(value), name.System) {
		return fmt.Errorf("sandbox: %q names the directory holding the tokens and the sealing key, so it is not a name in the tree", value)
	}
	return nil
}
