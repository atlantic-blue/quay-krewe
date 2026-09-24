package controlplane

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
)

// CHECK-5: the working tree a step session builds in, made before that session's first command.
//
// The check reads one directory, so a session that clones anywhere else cannot be checked at all. The
// session used to make the tree itself, and three things took it elsewhere: a note that sends every
// session to /tmp, a shell that goes back to the start folder after each command, and a session that
// skips the brief. So krewe makes it, with the commands the git skill names.

// theTreeOfASession is the tree of session 9e8153f6 in a project that works in atlantic-blue/bills.
func theTreeOfASession(t *testing.T) workingTree {
	t.Helper()
	plan, named := workingTreeOf("9e8153f6", "atlantic-blue/bills")
	if !named {
		t.Fatal("a session with an identifier and a project with a repository name no tree")
	}
	return plan
}

// withNoCheckout is a sandbox that answers every read of a directory with "nothing there", which is a
// container before anything has been cloned into it.
func withNoCheckout() *sandbox.FakeSandbox {
	return &sandbox.FakeSandbox{Replies: []sandbox.Reply{
		{Match: "test -", Err: exitStatus(1)},
	}}
}

// exitStatus is a command that failed, the way a shell reports one.
func exitStatus(code int) error { return fmt.Errorf("exit status %d", code) }

// ranIn is every command that sandbox was given, one to a line.
func ranIn(box *sandbox.FakeSandbox) string {
	lines := make([]string, 0, len(box.Ran))
	for _, spec := range box.Ran {
		lines = append(lines, strings.Join(spec.Argv, " "))
	}
	return strings.Join(lines, "\n")
}

// The commands the git skill names, in the order it names them, run in the session's own container.
//
// Each one is a separate way for the work to land where krewe cannot read it: a clone of another
// repository, a stale clone, a tree under a name another session shares, or a tree that nothing in the
// session's own folder points at.
func TestTheTreeIsMadeWithTheCommandsTheGitSkillNames(t *testing.T) {
	box := withNoCheckout()
	if err := makeTheWorkingTree(context.Background(), box, nil, theTreeOfASession(t)); err != nil {
		t.Fatalf("making the tree: %v", err)
	}
	ran := ranIn(box)
	at := 0
	for _, want := range []string{
		"git clone https://github.com/atlantic-blue/bills.git /home/agent/shared/repos/bills",
		"git -C /home/agent/shared/repos/bills fetch origin",
		"git -C /home/agent/shared/repos/bills worktree add /home/agent/shared/worktrees/9e8153f6/bills " +
			"-b krewe/9e8153f6 origin/HEAD",
		"ln -s /home/agent/shared/worktrees/9e8153f6/bills /home/agent/workspace/bills",
	} {
		found := strings.Index(ran[at:], want)
		if found < 0 {
			t.Fatalf("the container was given:\n%s\nand it never ran %q in that order", ran, want)
		}
		at += found + len(want)
	}
}

// The environment of the exec, so a private clone authenticates with the same credential the session
// would have used. Without it the clone asks for a password, and a command waiting for one never ends.
func TestEveryCommandCarriesTheEnvironmentOfTheExec(t *testing.T) {
	box := withNoCheckout()
	env := []string{"GH_TOKEN=a-token", "QC_SESSION_ID=9e8153f6"}
	if err := makeTheWorkingTree(context.Background(), box, env, theTreeOfASession(t)); err != nil {
		t.Fatalf("making the tree: %v", err)
	}
	if len(box.Ran) == 0 {
		t.Fatal("the container was given no command, so this says nothing about what they carry")
	}
	for _, spec := range box.Ran {
		if strings.Join(spec.Env, " ") != strings.Join(env, " ") {
			t.Errorf("%q ran with %v, want %v", strings.Join(spec.Argv, " "), spec.Env, env)
		}
	}
}

// A later exec finds the tree the first one made and touches nothing. A second add of one path is
// refused by git, and a fetch nobody needs is a wait before every command the session runs.
func TestATreeThatIsThereIsLeftAlone(t *testing.T) {
	box := &sandbox.FakeSandbox{}
	plan := theTreeOfASession(t)
	if err := makeTheWorkingTree(context.Background(), box, nil, plan); err != nil {
		t.Fatalf("making the tree: %v", err)
	}
	if len(box.Ran) == 0 {
		t.Fatal("the container was given no command, so nothing asked whether the tree was there")
	}
	if !strings.Contains(ranIn(box), plan.tree) {
		t.Fatalf("the container never read %q, so nothing asked whether the tree was there:\n%s",
			plan.tree, ranIn(box))
	}
	for _, unwanted := range []string{"git clone", "fetch origin", "worktree add", "ln -s"} {
		if strings.Contains(ranIn(box), unwanted) {
			t.Errorf("the container was given %q, and the tree was already there:\n%s", unwanted, ranIn(box))
		}
	}
}

// One clone serves every session in the workspace, so a session whose sibling cloned it takes a tree
// out of that clone rather than cloning again. A second clone costs the whole repository twice and
// leaves two origins that drift.
func TestACloneThatIsThereIsNotClonedAgain(t *testing.T) {
	box := &sandbox.FakeSandbox{Replies: []sandbox.Reply{
		{Match: "test -e /home/agent/shared/repos/bills/.git", Out: ""},
		{Match: "test -", Err: exitStatus(1)},
	}}
	if err := makeTheWorkingTree(context.Background(), box, nil, theTreeOfASession(t)); err != nil {
		t.Fatalf("making the tree: %v", err)
	}
	ran := ranIn(box)
	if strings.Contains(ran, "git clone") {
		t.Errorf("the container cloned a repository it already had:\n%s", ran)
	}
	for _, want := range []string{"fetch origin", "worktree add"} {
		if !strings.Contains(ran, want) {
			t.Errorf("the container never ran %q, so the session has no tree:\n%s", want, ran)
		}
	}
}

// Something already at the name in the session's own folder is left where it is. Replacing it would
// take away whatever a session put there, and the link is a convenience: the tree is reachable by its
// own path either way.
func TestSomethingAlreadyAtTheLinkIsLeftAlone(t *testing.T) {
	box := &sandbox.FakeSandbox{Replies: []sandbox.Reply{
		{Match: "/home/agent/workspace/bills", Out: ""},
		{Match: "test -", Err: exitStatus(1)},
	}}
	if err := makeTheWorkingTree(context.Background(), box, nil, theTreeOfASession(t)); err != nil {
		t.Fatalf("making the tree: %v", err)
	}
	if !strings.Contains(ranIn(box), "worktree add") {
		t.Fatalf("the container never added the tree, so this says nothing about the link:\n%s", ranIn(box))
	}
	if strings.Contains(ranIn(box), "ln -s") {
		t.Errorf("the container linked over something that was already there:\n%s", ranIn(box))
	}
}

// A command that fails says which command and what it printed, because the operator's next move is to
// run that command themselves. A tree half made is worse than none: the session builds in its start
// folder and reports work krewe reads as an empty directory.
func TestAFailedCommandSaysWhichOneAndWhatItSaid(t *testing.T) {
	box := &sandbox.FakeSandbox{Replies: []sandbox.Reply{
		{Match: "worktree add", Stderr: "fatal: could not create work tree", Err: exitStatus(128)},
		{Match: "test -", Err: exitStatus(1)},
	}}
	err := makeTheWorkingTree(context.Background(), box, nil, theTreeOfASession(t))
	if err == nil {
		t.Fatal("the tree was not made and nothing said so")
	}
	for _, want := range []string{"worktree add", "fatal: could not create work tree"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the failure reads %q, want it to name %q", err, want)
		}
	}
}

// The paths, which are the whole of what this names. A tree under the wrong name is a tree the check
// does not read, and two sessions at one path take each other's tree away.
func TestTheTreeIsNamedAfterTheSessionAndTheRepository(t *testing.T) {
	plan := theTreeOfASession(t)
	for _, tc := range []struct{ what, got, want string }{
		{"the clone", plan.clone, "/home/agent/shared/repos/bills"},
		{"the tree", plan.tree, "/home/agent/shared/worktrees/9e8153f6/bills"},
		{"the link", plan.link, "/home/agent/workspace/bills"},
		{"the folder the session may work in", plan.addDir, "/home/agent/shared/worktrees/9e8153f6"},
		{"the branch", plan.branch, "krewe/9e8153f6"},
		{"the address", plan.address, "https://github.com/atlantic-blue/bills.git"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s is %q, want %q", tc.what, tc.got, tc.want)
		}
	}
}

// Where there is nothing to name a tree from, none is named and the exec runs as it always did. A
// project that never said where its work goes is the normal state of most projects.
func TestNoTreeWithoutASessionAndARepository(t *testing.T) {
	for _, tc := range []struct{ what, session, repository string }{
		{"a project that names no repository", "9e8153f6", ""},
		{"an address with no owner", "9e8153f6", "bills"},
		{"an address with nothing after the slash", "9e8153f6", "atlantic-blue/"},
		{"a session with no identifier", "", "atlantic-blue/bills"},
	} {
		if plan, named := workingTreeOf(tc.session, tc.repository); named {
			t.Errorf("%s named the tree %q", tc.what, plan.tree)
		}
	}
}
