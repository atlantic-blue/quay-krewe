package controlplane

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
)

// The working tree a step session builds in, made by krewe before that session's first command.
//
// CHECK-5. The check reads one directory, the working tree the git skill names, so a session that
// built anywhere else cannot be checked at all: the run finds no checkout, reports no scenarios, and
// that reads as a fault in code which never ran. The session was the one making the tree, and three
// things took it elsewhere. The runtime puts the shell back in the folder it started in after every
// command, unless that folder is one it was given, so a session that moved into the tree was out of it
// again by its next command. A memory note sends every session to /tmp. A session that skips the git
// brief clones into its own folder.
//
// So krewe makes the tree itself, with the commands the brief names, in the session's own container and
// with the environment of the exec. The folder is then named on the command line, which is what keeps
// the shell in it.

// workingTree is where one session's checkout goes, as the container sees it. One clone serves every
// session in the workspace and each session takes a tree of its own out of it, under its own
// identifier, because a clone records where its trees are and two sessions at one path take each
// other's tree away.
type workingTree struct {
	clone   string
	tree    string
	link    string
	addDir  string
	branch  string
	address string
}

// workingTreeOf names the tree of one session in a project that works in a repository, and answers
// false where there is nothing to name one from. A project that never said where its work goes is the
// normal state of most projects, and it gets no tree rather than a refusal.
func workingTreeOf(session, repository string) (workingTree, bool) {
	owner, name, written := strings.Cut(repository, "/")
	if session == "" || !written || owner == "" || name == "" || strings.Contains(name, "/") {
		return workingTree{}, false
	}
	return workingTree{
		clone:   path.Join(sandbox.ReposPath, name),
		tree:    path.Join(sandbox.WorktreesPath, session, name),
		link:    path.Join(sandbox.WorkingPath, name),
		addDir:  path.Join(sandbox.WorktreesPath, session),
		branch:  "krewe/" + session,
		address: "https://github.com/" + owner + "/" + name + ".git",
	}, true
}

// makeTheWorkingTree runs the commands the git skill names inside the session's container.
//
// A tree that is already there is used as it is, and nothing else runs: a second add of one path is
// refused by git, and a fetch before every exec is a wait the session pays for nothing. That read comes
// first for exactly that reason, so the usual exec costs one command and no network.
//
// The reads happen in the container rather than here. The paths are the container's, and a control
// plane that is not on the machine running the sandboxes cannot see them.
func makeTheWorkingTree(ctx context.Context, box sandbox.Sandbox, env []string, plan workingTree) error {
	made, err := holdsARepository(ctx, box, env, plan.tree)
	if err != nil || made {
		return err
	}
	cloned, err := holdsARepository(ctx, box, env, plan.clone)
	if err != nil {
		return err
	}
	if !cloned {
		if err := runInTheSandbox(ctx, box, env, "git", "clone", plan.address, plan.clone); err != nil {
			return err
		}
	}
	// Before the add and only here, because the tree is cut from origin/HEAD and a clone somebody else
	// made may be a week behind. A later exec finds the tree and never reaches this line.
	if err := runInTheSandbox(ctx, box, env, "git", "-C", plan.clone, "fetch", "origin"); err != nil {
		return err
	}
	if err := runInTheSandbox(ctx, box, env, "git", "-C", plan.clone,
		"worktree", "add", plan.tree, "-b", plan.branch, "origin/HEAD"); err != nil {
		return err
	}
	// Anything already at the name is left where it is. The link is a convenience, since the tree is
	// reachable by its own path either way, and replacing what a session put there would take away work.
	taken, err := somethingIsAt(ctx, box, env, plan.link)
	if err != nil || taken {
		return err
	}
	return runInTheSandbox(ctx, box, env, "ln", "-s", plan.tree, plan.link)
}

// holdsARepository says whether that directory in the container is the top of a repository or of a
// working tree. The name is what the system reads everywhere else, and a working tree carries it as a
// file rather than a directory, which is why the read is for the name and not for a kind.
func holdsARepository(ctx context.Context, box sandbox.Sandbox, env []string, dir string) (bool, error) {
	return answersYes(ctx, box, env, "test -e "+dir+"/.git")
}

// somethingIsAt says whether anything is at that path, a broken link included: a link whose tree has
// gone is still a name in the way, and linking over it fails.
func somethingIsAt(ctx context.Context, box sandbox.Sandbox, env []string, at string) (bool, error) {
	return answersYes(ctx, box, env, "test -L "+at+" || test -e "+at)
}

// answersYes runs one test in the container and reads its exit status as the answer.
//
// A test that exits non zero is the answer no and never a fault, which is the whole difference between
// this and runInTheSandbox: a directory that is not there is what this call is for. Only a command the
// container could not start at all is an error.
func answersYes(ctx context.Context, box sandbox.Sandbox, env []string, line string) (bool, error) {
	proc, err := box.Exec(ctx, sandbox.Spec{Argv: []string{"sh", "-c", line}, Env: env})
	if err != nil {
		return false, fmt.Errorf("sh -c %s: %w", line, err)
	}
	_, _ = io.Copy(io.Discard, proc.Stdout())
	return proc.Wait() == nil, nil
}

// runInTheSandbox runs one command that has to work, and says which command failed and what it printed
// where it does not.
//
// The command is in the message because the operator's next move is to run it themselves, and a tree
// half made is worse than none: the session builds in the folder it started in, reports the work, and
// leaves the check reading a directory with nothing in it.
func runInTheSandbox(ctx context.Context, box sandbox.Sandbox, env []string, argv ...string) error {
	command := strings.Join(argv, " ")
	proc, err := box.Exec(ctx, sandbox.Spec{Argv: argv, Env: env})
	if err != nil {
		return fmt.Errorf("%s: %w", command, err)
	}
	printed, _ := io.ReadAll(proc.Stdout())
	if ran := proc.Wait(); ran != nil {
		return fmt.Errorf("%s: %v: %s", command, ran, whatItPrinted(string(printed), proc.Stderr()))
	}
	return nil
}

// whatItPrinted is what a command said, from both streams, because git puts the sentence that explains
// a failure on the error stream and the rest of the story on the other one.
func whatItPrinted(out, stderr string) string {
	said := strings.TrimSpace(strings.TrimSpace(out) + "\n" + strings.TrimSpace(stderr))
	if said == "" {
		return "it printed nothing"
	}
	return said
}

// theWorkingTreeOfAStep makes the tree for a session holding a step that is checked, and answers the
// folders that session's exec may work in.
//
// Nothing happens for any other session. The tree is for the check, so a session nobody checks has no
// use for one, and a session handed a folder it never asked for is a session that can write in
// somebody else's tree.
//
// A project that cannot be read is a session that runs as it always did rather than an exec that
// fails: the tree is worth having and it is not worth an exec for.
func (s *Server) theWorkingTreeOfAStep(ctx context.Context, session *quaycrewv1.Session,
	box sandbox.Sandbox, env map[string]string) ([]string, error) {
	held, _ := s.stepThisSessionHolds(ctx, session)
	if held == nil || !held.GetCheckRequired() {
		return nil, nil
	}
	project, err := s.store.GetProject(ctx, session.GetProject())
	if err != nil {
		return nil, nil
	}
	plan, named := workingTreeOf(session.GetId(), project.GetRepository())
	if !named {
		return nil, nil
	}
	if err := makeTheWorkingTree(ctx, box, environ(env), plan); err != nil {
		return nil, err
	}
	// On every exec and not only on the one that made the tree. The runtime is told nothing by the
	// container, so a folder named once is a folder the second exec's shell walks out of.
	return []string{plan.addDir}, nil
}
