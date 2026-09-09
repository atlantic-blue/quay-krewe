package sandbox

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
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
	if err := usablePart(named); err != nil {
		return err
	}
	working, err := s.WorkingDirectory(cfg)
	if err != nil {
		return err
	}
	at := filepath.Join(s.NameTree, sessionNameAt(named))
	if err := makeWritableDir(filepath.Dir(at)); err != nil {
		return err
	}
	return pointAt(at, working.Host)
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

// Held is every name the store holds right now: what each workspace is called, and what each live
// session is called under its workspace and project. It is the whole answer rather than a change,
// because the tree is a view and a view is repaired by being written again.
//
// The order matters. Two workspaces may hold one name, and so may two sessions of one project, so the
// first entry keeps the name and the ones after it are said out loud rather than written. Give them
// oldest first and the tree answers the same way every time it is built.
type Held struct {
	Workspaces []WorkspaceName
	Sessions   []SessionName
}

// WorkspaceName is one workspace: the identifier its directory is named after, and what a person
// calls it.
type WorkspaceName struct {
	ID   string
	Name string
}

// SessionName is one session: the identifiers that locate its own directory, and the names it is
// filed under.
type SessionName struct {
	Config Config
	Names  Names
}

// WriteNames makes the tree say what the store says, and nothing else.
//
// A label changes, a session is put away and a workspace is deleted, and a tree written once then
// points at directories nobody is working in. This is the repair: it runs at start up, so a system
// that was down while things moved comes up correct, and it runs after each of those changes, so the
// tree is right in between. A workspace made before any of this shipped gets its name here too,
// because the tree is built from the store rather than added to as things are made.
//
// It writes the whole tree each time rather than the one name that moved. Deleting a workspace takes
// every session name in it, so the targeted form would be the same walk with a second set of rules to
// keep in step with this one. The cost is a read of the store and a read of the tree, on an operator's
// action rather than on a request path.
//
// What it takes away is what it writes: a link. A file or a directory somebody left here was not
// written by this view, so removing it is not this view's business, and it is left where it is.
//
// A name it cannot write costs the tree that name and not the sweep. The reason comes back joined
// with every other, because a name that quietly did not appear is a person opening a folder that is
// not there.
func (s Storage) WriteNames(held Held) error {
	if s.NameTree == "" || s.Dir == "" {
		return nil
	}
	if err := makeWritableDir(s.NameTree); err != nil {
		return err
	}
	wanted, refused := s.wantedNames(held)
	trouble := []error{refused, s.takeAwayWhatWent(wanted)}
	for _, at := range slices.Sorted(maps.Keys(wanted)) {
		trouble = append(trouble, repointAt(filepath.Join(s.NameTree, at), wanted[at]))
	}
	return errors.Join(trouble...)
}

// wantedNames is every name the tree should hold, from the path inside the tree to the directory on
// the host it points at.
//
// The directories are made here, which is how a name whose directory went comes back pointing at
// something: the same call that says where a name points creates what it points at.
func (s Storage) wantedNames(held Held) (map[string]string, error) {
	wanted := make(map[string]string, len(held.Workspaces)+len(held.Sessions))
	var refused []error
	claim := func(at, to, whose string) {
		if taken, already := wanted[at]; already {
			if taken != to {
				refused = append(refused, fmt.Errorf(
					"sandbox: %s is the name of two %ss, so the tree holds the one made first, at %s",
					at, whose, taken))
			}
			return
		}
		wanted[at] = to
	}
	for _, workspace := range held.Workspaces {
		if err := usableAsName("workspace", workspace.Name); err != nil {
			refused = append(refused, err)
			continue
		}
		shared, err := s.SharedDirectory(workspace.ID)
		if err != nil {
			refused = append(refused, err)
			continue
		}
		claim(workspace.Name, shared.Host, "workspace")
	}
	for _, session := range held.Sessions {
		if err := usableAsName("workspace", session.Names.Workspace); err != nil {
			refused = append(refused, err)
			continue
		}
		if err := usablePart(session.Names); err != nil {
			refused = append(refused, err)
			continue
		}
		working, err := s.WorkingDirectory(session.Config)
		if err != nil {
			refused = append(refused, err)
			continue
		}
		claim(sessionNameAt(session.Names), working.Host, "session")
	}
	return wanted, errors.Join(refused...)
}

// takeAwayWhatWent removes every link the store no longer holds a name for, and the directories that
// held nothing but those links.
//
// A session directory left standing empty says a workspace has sessions when it has none, so it goes
// with the last name in it. It goes only when it is empty, so anything else in there keeps it.
func (s Storage) takeAwayWhatWent(wanted map[string]string) error {
	entries, err := os.ReadDir(s.NameTree)
	if err != nil {
		return fmt.Errorf("sandbox: read the tree at %s: %w", s.NameTree, err)
	}
	var trouble []error
	for _, entry := range entries {
		switch {
		case entry.Type()&os.ModeSymlink != 0:
			trouble = append(trouble, s.takeAwayUnless(entry.Name(), wanted))
		case entry.IsDir() && strings.HasSuffix(entry.Name(), SessionsSuffix):
			trouble = append(trouble, s.takeAwaySessionNames(entry.Name(), wanted))
		}
	}
	return errors.Join(trouble...)
}

// takeAwaySessionNames does the same one level down, under a workspace's session directory, where a
// name is filed by project and then by session.
func (s Storage) takeAwaySessionNames(sessions string, wanted map[string]string) error {
	projects, err := os.ReadDir(filepath.Join(s.NameTree, sessions))
	if err != nil {
		return fmt.Errorf("sandbox: read %s: %w", sessions, err)
	}
	var trouble []error
	for _, project := range projects {
		if !project.IsDir() {
			continue
		}
		under := filepath.Join(sessions, project.Name())
		named, err := os.ReadDir(filepath.Join(s.NameTree, under))
		if err != nil {
			trouble = append(trouble, fmt.Errorf("sandbox: read %s: %w", under, err))
			continue
		}
		for _, one := range named {
			if one.Type()&os.ModeSymlink == 0 {
				continue
			}
			trouble = append(trouble, s.takeAwayUnless(filepath.Join(under, one.Name()), wanted))
		}
		trouble = append(trouble, s.takeAwayIfEmpty(under))
	}
	return errors.Join(append(trouble, s.takeAwayIfEmpty(sessions))...)
}

// takeAwayUnless removes one name the store no longer holds.
func (s Storage) takeAwayUnless(at string, wanted map[string]string) error {
	if _, keep := wanted[at]; keep {
		return nil
	}
	if err := os.Remove(filepath.Join(s.NameTree, at)); err != nil {
		return fmt.Errorf("sandbox: take the name %s away: %w", at, err)
	}
	return nil
}

// takeAwayIfEmpty removes a directory this view made to file names under, once the last name in it
// has gone. A directory holding anything else is left alone.
func (s Storage) takeAwayIfEmpty(at string) error {
	held, err := os.ReadDir(filepath.Join(s.NameTree, at))
	if err != nil || len(held) > 0 {
		return nil
	}
	if err := os.Remove(filepath.Join(s.NameTree, at)); err != nil {
		return fmt.Errorf("sandbox: take the empty directory %s away: %w", at, err)
	}
	return nil
}

// sessionNameAt is where one session's name is filed, inside the tree.
func sessionNameAt(named Names) string {
	return filepath.Join(named.Workspace+SessionsSuffix, named.Project, named.Session)
}

// usablePart refuses a project or a session name that would land somewhere other than where it says.
func usablePart(named Names) error {
	for _, part := range []struct{ kind, value string }{
		{"project", named.Project}, {"session", named.Session},
	} {
		if err := usableAsPath(part.kind, part.value); err != nil {
			return err
		}
	}
	return nil
}

// repointAt is pointAt with the store's answer behind it, so a name that went stale is moved onto the
// directory the store says rather than refused.
//
// Creating a name refuses to move one, because it holds one workspace's word and cannot tell which of
// the two is right. The sweep read every name in the system before it wrote any of them, so it can.
func repointAt(from, to string) error {
	if held, err := os.Readlink(from); err == nil && held != to {
		if err := os.Remove(from); err != nil {
			return fmt.Errorf("sandbox: take the stale name %s away: %w", from, err)
		}
	}
	if err := makeWritableDir(filepath.Dir(from)); err != nil {
		return err
	}
	return pointAt(from, to)
}
