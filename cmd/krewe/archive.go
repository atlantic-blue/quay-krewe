package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/display"
	"github.com/atlantic-blue/quay-krewe/internal/workspace"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// archiveUsage is what both forms of the word are, in the order somebody reads them.
const archiveUsage = "usage: krewe archive [<address>] [<session>] [" + flagOlderThan + " <days>d]\n\n" +
	"a session is its id, its handle, or its address, and it is put away on its own.\n" +
	"an address naming a workspace and a project puts away every session in it that holds\n" +
	"no container and has not been touched for " + defaultAgeTyped + ", and says how many it left and why.\n\n" +
	flagOlderThan + " takes a whole number of days, written the way the age column prints one: 30d.\n" +
	"it reads a project and not a session, and " + defaultAgeTyped + " is what a project takes without it.\n\n" +
	"with nothing at all it reads where you are standing.\n\n" +
	"nothing is deleted: krewe sessions --archived lists what is put away, and\n" +
	"krewe unarchive <session> brings one back"

// flagOlderThan is how far back a sweep reaches. Everything the age column prints is a number and a
// letter, and this takes the one letter that matters at this scale, so what an operator reads in the
// listing is what they type here.
const flagOlderThan = "--older-than"

// defaultAge is how long a session is left alone before a sweep with no age given takes it, and
// defaultAgeTyped is the same age written the way somebody types it.
//
// Read on 19 September 2026 from the live listing: 465 sessions, 310 of them stopped, 276 of them
// fourteen days or older and 259 of those stopped. Nothing there had reached thirty days. That says
// what exists and not what the operator wants to keep, which is why the number is provisional and
// says so where anybody choosing one would read it.
const (
	defaultAge      = 14 * 24 * time.Hour
	defaultAgeTyped = "14d"
)

// runArchive puts a session away, or every finished session of one project.
//
// Two forms under one word, because a person archives one session and a project's sessions for the
// same reason: the finished ones bury the live ones. Measured on 5 September 2026 a system held 303
// sessions and 282 of them were stopped, so the three that were working sat at the top of a list
// nobody could read.
//
// The address decides which form runs, exactly as krewe exec and krewe sessions decide.
//
// The age reaches the project form only. A session named on its own is the operator naming that
// session, and how old it is has nothing to do with that decision, so an age given beside one is
// refused rather than ignored.
func runArchive(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient, args []string, out io.Writer) error {
	flags, rest, err := readFlags(args)
	if err != nil {
		return err
	}
	if len(rest) > 1 {
		return fmt.Errorf("%s", archiveUsage)
	}
	typed := ""
	if len(rest) == 1 {
		typed = strings.TrimSpace(rest[0])
	}
	naming, err := whatToArchive(typed)
	if err != nil {
		return err
	}
	// Before the age is read, because what to say about an age beside a session is where it belongs
	// and not what shape it takes.
	if naming.session != "" {
		if flags.has(flagOlderThan) {
			return fmt.Errorf("%s reads a project, and %q names one session\n\narchive that session "+
				"with krewe archive %s, or sweep its project with krewe archive <workspace>/<project> "+
				"%s %s", flagOlderThan, typed, typed, flagOlderThan, defaultAgeTyped)
		}
		return archiveOneSession(ctx, client, naming.session, out)
	}
	age, err := ageToSweepBy(flags)
	if err != nil {
		return err
	}
	return archiveAProject(ctx, client, naming.project, age, out)
}

// ageToSweepBy reads the one shape the flag takes: a whole number of days, written the way the age
// column prints one. 14d, 30d, 90d.
//
// One shape and not three. An age that also took 336h, or two weeks, would be three spellings of one
// value, and each of them is a thing to get wrong in a command that hides sessions.
//
// Zero and below are refused by name. A sweep is the one word here that moves hundreds of rows at
// once, and an age of nothing would move every settled session in the project, which is the slip this
// refusal exists to stop.
func ageToSweepBy(flags given) (time.Duration, error) {
	if !flags.has(flagOlderThan) {
		return defaultAge, nil
	}
	typed := flags.first(flagOlderThan)
	digits, isDays := strings.CutSuffix(typed, "d")
	days, err := strconv.Atoi(digits)
	if !isDays || err != nil {
		return 0, fmt.Errorf("%q is not an age: %s takes a whole number of days, written as %s or 30d",
			typed, flagOlderThan, defaultAgeTyped)
	}
	if days < 1 {
		return 0, fmt.Errorf("an age of %s would archive every session in the project that holds no "+
			"container, whatever it is: %s takes a whole number of days above zero, written as %s or 30d",
			typed, flagOlderThan, defaultAgeTyped)
	}
	return time.Duration(days) * 24 * time.Hour, nil
}

// typedAge writes an age back the way it was typed, so every line the sweep prints names the rule it
// ran under in the shape the operator can type again.
func typedAge(age time.Duration) string {
	return strconv.Itoa(int(age/(24*time.Hour))) + "d"
}

// archiving is which of the two forms an invocation asked for: one of the fields is set and the
// other is empty.
type archiving struct {
	session string
	project workspace.Path
}

// whatToArchive decides between the two forms. A word that names a session is one, an address that
// reaches a project is the other, and nothing at all is whichever of those the operator is standing
// in.
func whatToArchive(typed string) (archiving, error) {
	if typed != "" {
		// A bare word is a session when it is shaped like one of a session's two identifiers, which is
		// what a listing prints. Everything else is an address, and the level it stops at picks the
		// form: a session archives on its own, a project is swept.
		if !strings.Contains(typed, workspace.Separator) && display.LooksLikeIdentifier(typed) {
			return archiving{session: typed}, nil
		}
		path, err := workspace.ParsePath(typed)
		if err != nil {
			return archiving{}, err
		}
		if path.Session != "" {
			return archiving{session: typed}, nil
		}
		// Archiving a whole workspace is not a thing this word does. A workspace holds projects that
		// have nothing to do with each other, and a sweep across all of them is not one decision.
		if path.Project == "" {
			return archiving{}, fmt.Errorf("%q names a workspace, and archiving reads a project or a "+
				"session\n\n%s", typed, archiveUsage)
		}
		return archiving{project: path}, nil
	}

	where, err := currentPath()
	if err != nil {
		return archiving{}, err
	}
	switch {
	case where.Session != "":
		return archiving{session: where.String()}, nil
	case where.Project != "":
		return archiving{project: where}, nil
	default:
		return archiving{}, fmt.Errorf("you are not standing in a project or a session, so there is "+
			"nothing to archive\n\n%s", archiveUsage)
	}
}

// archiveOneSession puts one session away and says the record is kept, because a word that reads like
// a delete stops a person from using it.
func archiveOneSession(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient, typed string,
	out io.Writer) error {
	session, err := workspace.Session(ctx, client, typed)
	if err != nil {
		return err
	}
	if session.GetArchivedAt() != nil {
		return fmt.Errorf("session %s is already archived\n\nbring it back with krewe unarchive %s",
			display.ShortID(session.GetHandle()), display.ShortID(session.GetHandle()))
	}
	if _, err := client.ArchiveSession(ctx, &quaycrewv1.ArchiveSessionRequest{Id: session.GetId()}); err != nil {
		return err
	}
	where := display.ShortID(session.GetHandle())
	fmt.Fprintf(out, "archived %s: its conversation, its execs and its files are all still there\n", where)
	fmt.Fprintf(out, "what it left is a volume: krewe volume list krewe://<workspace>/<project>/%s\n", where)
	fmt.Fprintf(out, "bring it back with krewe unarchive %s\n", where)
	return nil
}

// archiveAProject sweeps one project, and says what it left as well as what it took. A sweep that
// reports only what it took reads as a sweep that took everything.
//
// Three lines, each of them a number somebody can check against the listing they just read: what
// went, what stayed and under which of the two rules, and the way back. A command that puts 259
// sessions away and prints one number is a command nobody runs twice.
func archiveAProject(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient, path workspace.Path,
	age time.Duration, out io.Writer) error {
	located, err := workspace.ResolvePath(ctx, client, path)
	if err != nil {
		return standing(path.String(), path, err)
	}
	// The instant rather than the length. The tool works out what fourteen days ago was and says it,
	// so the rule cannot shift between this process reading its clock and the system reading its own.
	resp, err := client.ArchiveProjectSessions(ctx, &quaycrewv1.ArchiveProjectSessionsRequest{
		Project:         located.ProjectID,
		LastMovedBefore: timestamppb.New(time.Now().UTC().Add(-age)),
	})
	if err != nil {
		return err
	}
	stayed := fmt.Sprintf("%s left in the listing: %d holding a container, %d younger than %s\n",
		plural(int(resp.GetSkipped()), "sessions"), resp.GetHoldingAContainer(), resp.GetYoungerThanTheAge(),
		typedAge(age))

	archived := resp.GetArchived()
	if len(archived) == 0 {
		fmt.Fprintf(out, "nothing was archived in %s: a sweep takes a session that holds no container "+
			"and has not been touched for %s\n", path.String(), typedAge(age))
		fmt.Fprint(out, stayed)
		fmt.Fprintf(out, "reach further back with krewe archive %s %s <days>d\n", path.String(), flagOlderThan)
		return nil
	}
	fmt.Fprintf(out, "archived %s in %s, none of them touched for %s\n",
		plural(len(archived), "sessions"), path.String(), typedAge(age))
	fmt.Fprint(out, stayed)
	// Said on the way out as well as in the usage, because a person who has just moved 259 rows wants
	// to read that nothing went with them without going to look for it.
	fmt.Fprintf(out, "nothing is deleted: krewe sessions %s --archived lists what went, and "+
		"krewe unarchive <session> brings one back\n", path.String())
	return nil
}

// runUnarchive brings a session back into the default listing.
//
// It ships in the same slice as the word that hides one. A wrong address hides work a person then
// cannot find, so the way back is never a later change.
func runUnarchive(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient, args []string, out io.Writer) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: krewe unarchive <session>\n\n" +
			"a session is its id, its handle, or its address. With nothing at all it reads where you\n" +
			"are standing")
	}
	typed := ""
	if len(args) == 1 {
		typed = strings.TrimSpace(args[0])
	}
	if typed == "" {
		where, err := currentPath()
		if err != nil {
			return err
		}
		if where.Session == "" {
			return fmt.Errorf("you are not standing in a session: krewe unarchive <session>")
		}
		typed = where.String()
	}

	session, err := workspace.Session(ctx, client, typed)
	if err != nil {
		return err
	}
	if _, err := client.RestoreSession(ctx, &quaycrewv1.RestoreSessionRequest{Id: session.GetId()}); err != nil {
		return err
	}
	where := display.ShortID(session.GetHandle())
	fmt.Fprintf(out, "%s is back in the listing, reading stopped\n", where)
	// Said out loud, because an operator who expects the old container back finds a session that
	// answers nothing.
	fmt.Fprintf(out, "it holds no container: the next exec builds a fresh one over the same conversation\n")
	return nil
}
