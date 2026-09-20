package features_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/cucumber/godog"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Steps for the age a project sweep puts a session away at.
//
// The age travels as an instant rather than as a length, and that is what these steps rest on. A
// length is read against a clock at each end, so a scenario that says "older than ten milliseconds"
// is a scenario that passes or fails on how long the call took. A moment taken once, with a wait
// either side of it, puts every session firmly on one side of the rule and leaves nothing for a busy
// machine to move.

// ageKey holds the moment one scenario swept by, which two steps apart have to agree on.
type ageKey struct{}

type sweepAge struct {
	moment time.Time
	// otherWorkspaces are the workspaces a scenario made beside the one the background makes, so the
	// system form can be asked whether it reached past the first.
	otherWorkspaces []string
	// livingElsewhere is the session left holding a container in one of those, which is the session
	// a sweep across every workspace must not take.
	livingElsewhere string
	// systemSweep is what the last sweep over every workspace answered.
	systemSweep *quaycrewv1.ArchiveSystemSessionsResponse
}

func ageFrom(ctx context.Context) *sweepAge {
	a, _ := ctx.Value(ageKey{}).(*sweepAge)
	return a
}

func initializeSessionAgeSteps(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, ageKey{}, &sweepAge{}), nil
	})

	// The wait either side is what makes the moment a real line rather than a tie. The store writes
	// its own stamps and takes none, so there is no other way to put a session on a side of it.
	sc.Step(`^a moment every session so far is older than$`, func(ctx context.Context) error {
		time.Sleep(10 * time.Millisecond)
		ageFrom(ctx).moment = time.Now().UTC()
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	sc.Step(`^(\d+) stopped sessions in the project$`, func(ctx context.Context, count int) error {
		w := worldFrom(ctx)
		for i := 0; i < count; i++ {
			if err := w.dispatch(ctx, w.projectID, "", fmt.Sprintf("a finished subject %d", i)); err != nil {
				return err
			}
			if w.lastErr != nil {
				return w.lastErr
			}
			current, err := w.lastExec()
			if err != nil {
				return err
			}
			if _, err := w.client.StopSession(ctx, &quaycrewv1.StopSessionRequest{Id: current.sessionID}); err != nil {
				return err
			}
		}
		return nil
	})

	sc.Step(`^the operator archives the project's sessions older than that moment$`, func(ctx context.Context) error {
		moment := ageFrom(ctx).moment
		if moment.IsZero() {
			return fmt.Errorf("no moment was taken, so this sweep would read every session as old")
		}
		return sweepTheProject(ctx, moment)
	})

	// A day, on sessions this scenario made seconds ago, so every one of them is younger than the age
	// by a margin nothing on the machine can close.
	sc.Step(`^the operator archives the project's sessions older than a day$`, func(ctx context.Context) error {
		return sweepTheProject(ctx, time.Now().UTC().Add(-24*time.Hour))
	})

	// Why each session stayed, which is the half of the answer a count cannot carry. A sweep that
	// leaves 17 of 276 says nothing until it says which of them were working.
	sc.Step(`^the sessions left are (\d+) holding a container and (\d+) younger than the age$`,
		func(ctx context.Context, holding, young int) error {
			sweep := worldFrom(ctx).lastSweep
			if got := int(sweep.GetHoldingAContainer()); got != holding {
				return fmt.Errorf("the sweep says %d sessions hold a container, want %d", got, holding)
			}
			if got := int(sweep.GetYoungerThanTheAge()); got != young {
				return fmt.Errorf("the sweep says %d sessions are younger than the age, want %d", got, young)
			}
			// The two reasons are the whole of what it left, or one of them is a count of nothing.
			if got := int(sweep.GetSkipped()); got != holding+young {
				return fmt.Errorf("the sweep left %d sessions and gives a reason for %d of them",
					got, holding+young)
			}
			return nil
		})

	sc.Step(`^the driver asks to archive the project's sessions older than a day$`, func(ctx context.Context) error {
		project := worldFrom(ctx).projectID
		return asDriver(ctx, func(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient) error {
			_, err := client.ArchiveProjectSessions(ctx, &quaycrewv1.ArchiveProjectSessionsRequest{
				Project:         project,
				LastMovedBefore: timestamppb.New(time.Now().UTC().Add(-24 * time.Hour)),
			})
			return err
		})
	})

	// The steps that run the real tool. What an operator does with this word is type it and read what
	// came back, so the default and the refusal are proved on the screen rather than on the wire.
	sc.Step(`^the caller archives the project's sessions$`, func(ctx context.Context) error {
		return runTool(ctx, "archive", projectAddress(ctx))
	})
	sc.Step(`^the caller archives the project's sessions older than "([^"]*)"$`,
		func(ctx context.Context, age string) error {
			return runTool(ctx, "archive", projectAddress(ctx), "--older-than", age)
		})

	sc.Step(`^the tool names (\S+) as the age it swept by$`, func(ctx context.Context, age string) error {
		return toolSaid(ctx, "not been touched for "+age)
	})
	sc.Step(`^the tool says nothing was archived$`, func(ctx context.Context) error {
		return toolSaid(ctx, "was archived")
	})
	sc.Step(`^the tool says (\d+) sessions were left, (\d+) holding a container and (\d+) younger than the age$`,
		func(ctx context.Context, left, holding, young int) error {
			return toolSaid(ctx, fmt.Sprintf("%d sessions left in the listing: %d holding a container, %d younger than",
				left, holding, young))
		})
	sc.Step(`^the tool says the way to reach further back$`, func(ctx context.Context) error {
		return toolSaid(ctx, "--older-than")
	})
	// One step for both sweeps. The refusal names what the age would have taken, and the two forms
	// differ in nothing else, so a second step here would be the same assertion written twice.
	sc.Step(`^the refusal says an age of (\S+) would take the whole (project|system)$`,
		func(ctx context.Context, age, reach string) error {
			for _, want := range []string{"an age of " + age, "every session in the " + reach} {
				if !strings.Contains(toolFrom(ctx).stderr, want) {
					return fmt.Errorf("the refusal does not say %q:\n%s", want, toolFrom(ctx).stderr)
				}
			}
			return nil
		})

	initializeSystemSweepSteps(sc)
}

// initializeSystemSweepSteps registers the steps for the sweep that reaches every workspace.
func initializeSystemSweepSteps(sc *godog.ScenarioContext) {
	sc.Step(`^another workspace holding a project with (\d+) stopped sessions$`,
		func(ctx context.Context, stopped int) error {
			return aWorkspaceBeside(ctx, stopped, false)
		})
	sc.Step(`^another workspace holding a project with (\d+) stopped sessions and a session that holds a container$`,
		func(ctx context.Context, stopped int) error {
			return aWorkspaceBeside(ctx, stopped, true)
		})
	// Three workspaces rather than two, because two cannot tell a sweep that reaches every workspace
	// from one that reaches the first and the last.
	sc.Step(`^(\d+) more workspaces, each holding a project with (\d+) stopped sessions$`,
		func(ctx context.Context, workspaces, stopped int) error {
			for i := 0; i < workspaces; i++ {
				if err := aWorkspaceBeside(ctx, stopped, false); err != nil {
					return err
				}
			}
			return nil
		})

	sc.Step(`^the operator archives the system's sessions older than that moment$`, func(ctx context.Context) error {
		moment := ageFrom(ctx).moment
		if moment.IsZero() {
			return fmt.Errorf("no moment was taken, so this sweep would read every session as old")
		}
		age := ageFrom(ctx)
		w := worldFrom(ctx)
		swept, err := w.client.ArchiveSystemSessions(ctx, &quaycrewv1.ArchiveSystemSessionsRequest{
			LastMovedBefore: timestamppb.New(moment),
		})
		age.systemSweep, w.lastErr = swept, err
		return err
	})

	sc.Step(`^the system sweep archived (\d+) sessions and left (\d+)$`,
		func(ctx context.Context, took, left int) error {
			swept := ageFrom(ctx).systemSweep
			if got := len(swept.GetArchived()); got != took {
				return fmt.Errorf("the sweep archived %d sessions, want %d", got, took)
			}
			if got := int(swept.GetSkipped()); got != left {
				return fmt.Errorf("the sweep says it left %d sessions, want %d", got, left)
			}
			return nil
		})

	// What the counts cover. A sweep that read one workspace and a sweep that read three answer with
	// the same two numbers when the other two workspaces were empty, so this is the only line that
	// says the sweep reached them at all.
	sc.Step(`^the system sweep read (\d+) workspaces$`, func(ctx context.Context, want int) error {
		swept := ageFrom(ctx).systemSweep
		if got := int(swept.GetWorkspacesRead()); got != want {
			return fmt.Errorf("the sweep read %d workspaces, want %d", got, want)
		}
		// A workspace it could not read is named rather than dropped, so a scenario that expects a
		// whole system must find nothing named here.
		if left := swept.GetUnswept(); len(left) > 0 {
			return fmt.Errorf("the sweep could not read %d workspaces, the first being %q: %s",
				len(left), left[0].GetName(), left[0].GetReason())
		}
		return nil
	})

	sc.Step(`^the sessions the system sweep left are (\d+) holding a container and (\d+) younger than the age$`,
		func(ctx context.Context, holding, young int) error {
			swept := ageFrom(ctx).systemSweep
			if got := int(swept.GetHoldingAContainer()); got != holding {
				return fmt.Errorf("the sweep says %d sessions hold a container, want %d", got, holding)
			}
			if got := int(swept.GetYoungerThanTheAge()); got != young {
				return fmt.Errorf("the sweep says %d sessions are younger than the age, want %d", got, young)
			}
			if got := int(swept.GetSkipped()); got != holding+young {
				return fmt.Errorf("the sweep left %d sessions and gives a reason for %d of them",
					got, holding+young)
			}
			return nil
		})

	// Read off the listing rather than off the answer, because the answer is the thing being tested.
	sc.Step(`^the other workspaces hold (\d+) sessions$`, func(ctx context.Context, want int) error {
		w, age := worldFrom(ctx), ageFrom(ctx)
		if len(age.otherWorkspaces) == 0 {
			return fmt.Errorf("this scenario made no workspace beside the first one")
		}
		live := 0
		for _, workspace := range age.otherWorkspaces {
			listed, err := w.client.ListSessions(ctx, &quaycrewv1.ListSessionsRequest{Workspace: workspace})
			if err != nil {
				return err
			}
			live += len(listed.GetSessions())
		}
		if live != want {
			return fmt.Errorf("%d sessions are still in the listing of the other workspaces, want %d",
				live, want)
		}
		return nil
	})

	// The rule that stops this command taking somebody's running work away, asked of the workspace a
	// sweep written as one project's loop never reaches.
	sc.Step(`^the session holding a container in another workspace is still live$`, func(ctx context.Context) error {
		w, age := worldFrom(ctx), ageFrom(ctx)
		if age.livingElsewhere == "" {
			return fmt.Errorf("no session was left holding a container in another workspace")
		}
		got, err := w.client.GetSession(ctx, &quaycrewv1.GetSessionRequest{Id: age.livingElsewhere})
		if err != nil {
			return err
		}
		if got.GetSession().GetArchivedAt() != nil {
			return fmt.Errorf("the sweep archived %s, which was holding a container", age.livingElsewhere)
		}
		return nil
	})

	sc.Step(`^the driver asks to archive the system's sessions$`, func(ctx context.Context) error {
		return asDriver(ctx, func(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient) error {
			_, err := client.ArchiveSystemSessions(ctx, &quaycrewv1.ArchiveSystemSessionsRequest{})
			return err
		})
	})

	sc.Step(`^the caller archives the system's sessions$`, func(ctx context.Context) error {
		return runTool(ctx, "archive", "system")
	})
	sc.Step(`^the caller archives the system's sessions older than "([^"]*)"$`,
		func(ctx context.Context, age string) error {
			return runTool(ctx, "archive", "system", "--older-than", age)
		})
	sc.Step(`^the tool says it read "([^"]*)"$`, func(ctx context.Context, what string) error {
		return toolSaid(ctx, "read "+what)
	})
	// Why each session left the listing, read from the archived listing itself, because that listing
	// is where a person reads it and a field that survives only a fetch by identifier reaches nobody.
	sc.Step(`^the archived listing says the session started (first|last) went by "([^"]*)"$`,
		func(ctx context.Context, which, want string) error {
			w := worldFrom(ctx)
			if len(w.execs) < 2 {
				return fmt.Errorf("fewer than two sessions were started, so first and last are the same one")
			}
			wanted := w.execs[0]
			if which == "last" {
				wanted = w.execs[len(w.execs)-1]
			}
			return archivedSessionWentBy(ctx, wanted.sessionID, want)
		})
	sc.Step(`^the archived listing says that session went by "([^"]*)"$`,
		func(ctx context.Context, want string) error {
			current, err := worldFrom(ctx).lastExec()
			if err != nil {
				return err
			}
			return archivedSessionWentBy(ctx, current.sessionID, want)
		})

	// A session nothing has put away. It is read by identifier rather than from a listing, because a
	// live session is in the other listing and the point is the word it carries, not where it is.
	sc.Step(`^that session says nothing about why it went$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		current, err := w.lastExec()
		if err != nil {
			return err
		}
		read, err := w.client.GetSession(ctx, &quaycrewv1.GetSessionRequest{Id: current.sessionID})
		if err != nil {
			return err
		}
		if got := read.GetSession().GetArchivedReason(); got != "" {
			return fmt.Errorf("the session says it went by %q, and nothing has it put away", got)
		}
		return nil
	})
}

// archivedSessionWentBy reads the archived listing and says whether the session it was asked about
// carries the word it was asked for.
//
// It fails when the session is not in that listing at all, rather than passing over an empty answer:
// a listing that lost the row would otherwise read the same as a listing that drew it correctly.
func archivedSessionWentBy(ctx context.Context, session, want string) error {
	w := worldFrom(ctx)
	listed, err := w.client.ListSessions(ctx, &quaycrewv1.ListSessionsRequest{
		Workspace: w.workspaceID, Archived: true,
	})
	if err != nil {
		return err
	}
	for _, one := range listed.GetSessions() {
		if one.GetId() != session {
			continue
		}
		if got := one.GetArchivedReason(); got != want {
			return fmt.Errorf("the archived listing says session %s went by %q, want %q", session, got, want)
		}
		return nil
	}
	return fmt.Errorf("session %s is not in the archived listing of %d sessions",
		session, len(listed.GetSessions()))
}

// aWorkspaceBeside makes a workspace the background never made, holding one project, so the system
// form can be asked to reach past the workspace every other scenario stands in.
//
// The names are numbered rather than fixed, because a scenario asking for three of them would
// otherwise ask for one name three times.
func aWorkspaceBeside(ctx context.Context, stopped int, alsoHoldingAContainer bool) error {
	w, age := worldFrom(ctx), ageFrom(ctx)
	named := fmt.Sprintf("beside-%d", len(age.otherWorkspaces)+1)
	made, err := w.client.CreateWorkspace(ctx, &quaycrewv1.CreateWorkspaceRequest{Name: named})
	if err != nil {
		return err
	}
	workspace := made.GetWorkspace().GetId()
	age.otherWorkspaces = append(age.otherWorkspaces, workspace)
	project, err := w.client.CreateProject(ctx, &quaycrewv1.CreateProjectRequest{
		Workspace: workspace, Name: "work",
	})
	if err != nil {
		return err
	}
	for i := 0; i < stopped; i++ {
		session, err := aSessionIn(ctx, project.GetProject().GetId(),
			fmt.Sprintf("a finished subject in %s %d", named, i))
		if err != nil {
			return err
		}
		if _, err := w.client.StopSession(ctx, &quaycrewv1.StopSessionRequest{Id: session}); err != nil {
			return err
		}
	}
	if !alsoHoldingAContainer {
		return nil
	}
	session, err := aSessionIn(ctx, project.GetProject().GetId(), "still working in "+named)
	if err != nil {
		return err
	}
	age.livingElsewhere = session
	return nil
}

// aSessionIn starts one session in a project the world is not standing in, and answers with its id.
func aSessionIn(ctx context.Context, project, text string) (string, error) {
	w := worldFrom(ctx)
	if err := w.dispatch(ctx, project, "", text); err != nil {
		return "", err
	}
	if w.lastErr != nil {
		return "", w.lastErr
	}
	current, err := w.lastExec()
	if err != nil {
		return "", err
	}
	return current.sessionID, nil
}

// sweepTheProject is the call all three of the age scenarios make, kept in one place so they cannot
// drift into asking for different things.
func sweepTheProject(ctx context.Context, before time.Time) error {
	w := worldFrom(ctx)
	w.lastSweep, w.lastErr = w.client.ArchiveProjectSessions(ctx, &quaycrewv1.ArchiveProjectSessionsRequest{
		Project:         w.projectID,
		LastMovedBefore: timestamppb.New(before),
	})
	return w.lastErr
}

// projectAddress is the project the scenario is standing in, written the way somebody types it.
func projectAddress(ctx context.Context) string {
	w := worldFrom(ctx)
	return w.workspaceName + "/" + w.projectName
}

// toolSaid is what the caller read on the screen, which for this word is the answer itself.
func toolSaid(ctx context.Context, want string) error {
	if printed := toolFrom(ctx).stdout; !strings.Contains(printed, want) {
		return fmt.Errorf("the tool does not say %q:\n%s", want, printed)
	}
	return nil
}
