package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/atlantic-blue/quay-krewe/internal/commands"
	"github.com/atlantic-blue/quay-krewe/internal/controlplane"
	"google.golang.org/grpc/status"
)

// runCommands is the word for krewe's own slash commands: where they go, writing them there, and
// what this build carries.
//
// It talks to nothing. The files belong to the machine rather than to a project, so none of these
// takes an address and none of them needs a system to be up.
func runCommands(args []string, out io.Writer, inASession bool) error {
	if len(args) == 0 {
		return reportCommands(out)
	}
	switch args[0] {
	case "install":
		if len(args) > 1 {
			return fmt.Errorf("krewe commands install takes nothing after it, and %q is something: "+
				"the files it writes are the ones this build carries", args[1])
		}
		return installCommands(out, inASession)
	case "list":
		if len(args) > 1 {
			return fmt.Errorf("krewe commands list takes nothing after it, and %q is something", args[1])
		}
		return listCommands(out)
	default:
		return fmt.Errorf("no such word: krewe commands %s\n\n  krewe commands\n  krewe commands install\n  krewe commands list", args[0])
	}
}

// reportCommands is the read: where the files go, which build wrote the ones there now, and which
// build this binary would write. It installs nothing, because a read command that writes is a
// command nobody can run to find out where they stand.
func reportCommands(out io.Writer) error {
	dir, err := commands.Directory(os.Getenv)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("the command directory does not exist yet: %s. "+
			"Run krewe commands install to make it", dir)
	} else if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}

	builds, err := commands.Builds(dir)
	if err != nil {
		return err
	}
	installed := "none"
	if len(builds) > 0 {
		installed = strings.Join(builds, ", ")
	}

	fmt.Fprintf(out, "directory   %s\n", dir)
	fmt.Fprintf(out, "installed   %s\n", installed)
	fmt.Fprintf(out, "this build  %s\n", version)
	if installed != version {
		fmt.Fprintf(out, "\nthe files there are not this build. Run krewe commands install\n")
	}
	return nil
}

// installCommands writes this build's files where the operator's agent reads them.
//
// inASession is whether this tool is running inside a session's sandbox, which the caller reads from
// the identifier the system puts in one. A session could take that identifier back out of its own
// environment, and one that does is working around a guard rather than tripping over it: what this
// stops is a session being handed the operator's terminal by asking politely.
func installCommands(out io.Writer, inASession bool) error {
	// A session must not write the files the operator's own terminal runs, so the one list of what a
	// session may not do is asked before anything is written. The message rather than the status,
	// because nothing here made a call and "rpc error" would name a thing that never happened.
	if inASession {
		if refused := controlplane.DeniedToDriver(controlplane.InstallCommands, nil); refused != nil {
			return errors.New(status.Convert(refused).Message())
		}
	}

	dir, err := commands.Directory(os.Getenv)
	if err != nil {
		return err
	}
	written, err := commands.Install(dir, version)
	if err != nil {
		return err
	}
	for _, one := range written {
		fmt.Fprintf(out, "%s\n", one.Path)
	}
	fmt.Fprintf(out, "%s installed in %s\n", counted(len(written), "command"), dir)
	return nil
}

// listCommands says what this build carries, read out of the files themselves, so the listing and
// the file cannot disagree. It reads nothing on the machine, so it answers before anything is
// installed.
func listCommands(out io.Writer) error {
	all := commands.All()
	if len(all) == 0 {
		return fmt.Errorf("this build carries no commands")
	}
	widest := 0
	for _, one := range all {
		if len(one.Slash()) > widest {
			widest = len(one.Slash())
		}
	}
	for _, one := range all {
		fmt.Fprintf(out, "%-*s  %s\n", widest, one.Slash(), one.Description)
	}
	return nil
}
