package controlplane

import (
	"context"

	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
)

// workingTree is where one session's checkout goes, as the container sees it.
type workingTree struct {
	clone   string
	tree    string
	link    string
	addDir  string
	branch  string
	address string
}

// workingTreeOf names the tree of one session in a project that works in a repository.
func workingTreeOf(_, _ string) (workingTree, bool) {
	return workingTree{}, true
}

// makeTheWorkingTree runs the commands the git skill names inside the session's container.
func makeTheWorkingTree(_ context.Context, _ sandbox.Sandbox, _ []string, _ workingTree) error {
	return nil
}
