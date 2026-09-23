package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Whether a directory a command takes whole has any test under it.
//
// The name of a directory says nothing about what is in it. `rm -rf build/` is ordinary work and
// `rm -rf internal/` takes every test in there with it, and the two read the same to any rule about
// names. So this reads the disk, which the hook can do: it runs inside the sandbox, in the session's
// own working directory, with the repository in front of it.

// entries bounds the walk. A repository this system works in is thousands of files, and a hook that
// walked an unbounded tree would hold the session's tool call open until the runtime's timeout.
const entries = 20000

// skipped are the directories a walk does not go into. What is inside them is not this repository's
// tests, and they are the largest directories on any machine that has them.
var skipped = map[string]bool{".git": true, "node_modules": true, "vendor": true, "target": true}

// HoldsATest says whether this path is a directory with a test somewhere under it.
//
// A path that is not a directory answers no, because the name rules already read a file. A path that
// does not exist answers no, because a command cannot take away what is not there. A walk that runs
// out of entries answers yes: a directory too big to read is a directory this gate cannot clear, and
// the safe answer for a boundary is the one that refuses.
func HoldsATest(where string) bool {
	info, err := os.Stat(where)
	if err != nil || !info.IsDir() {
		return false
	}
	if why, is := APath(where); is {
		_ = why
		return true
	}
	seen, full := 0, false
	_ = filepath.WalkDir(where, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() && skipped[strings.ToLower(entry.Name())] {
			return filepath.SkipDir
		}
		if seen++; seen > entries {
			full = true
			return filepath.SkipAll
		}
		if _, is := APath(path); is {
			full = true
			return filepath.SkipAll
		}
		return nil
	})
	return full
}

// The mark is how the system says this session is building.
//
// It is a file rather than an environment variable because of when the gate comes on. A session
// writes the tests first, watches them fail, and only then builds against them, so the boundary
// arrives in the middle of that session's life. A container that is already running cannot be handed
// a new variable, and it can be handed a file: the session's own directory is a bind mount, so the
// system writes into it from outside and the next tool call reads it.
const (
	// MarkDir is the directory the system already writes the design, the path and the contracts into.
	MarkDir = ".krewe"
	// MarkFile is the mark itself. What is in it is for a person to read; the gate reads whether it
	// is there.
	MarkFile = "building"
)

// Marked says whether the session working here is building against tests it has seen fail.
//
// It looks up the tree rather than in one directory. A session clones the repository it works on
// into its own directory, so most commands run one or two directories below the mark, and a read of
// the working directory alone would find nothing and turn the gate off for the whole build.
//
// A directory it cannot read answers no, for the reason a broken hook must not stop a system.
func Marked(dir string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}
	for at := filepath.Clean(dir); ; {
		if _, err := os.Stat(filepath.Join(at, MarkDir, MarkFile)); err == nil {
			return true
		}
		up := filepath.Dir(at)
		if up == at {
			return false
		}
		at = up
	}
}
