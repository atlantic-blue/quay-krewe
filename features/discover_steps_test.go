package features_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/skill"
	"github.com/cucumber/godog"
)

// Steps for the discovery stage: what a session writes down about a repository that already exists,
// and the skill that tells it what to write.
//
// The discovery a scenario writes is the one the skill ships beside its brief, read off disk rather
// than made up here. A scenario that invented its own artifact would prove the record keeps json,
// and prove nothing about the shape every session is told to copy.

// discoverSkillDir is where the skills this build ships live, as the suite sees them. The same
// directory the image carries, so an example that stopped being a whole discovery fails here rather
// than on somebody's first run.
const discoverSkillDir = "../skills"

// screen is one screen of a flows.json, read for the three things discovery decides about it: that
// it exists already, which kind of surface draws it, and which file it was read from.
type screen struct {
	Name    string `json:"name"`
	Surface string `json:"surface"`
	Status  string `json:"status"`
	Source  string `json:"source"`
}

// flows is a flows.json as far as discovery writes one. The stories and the data model belong to the
// stages after this one, so nothing here reads them.
type flows struct {
	Screens map[string]screen `json:"screens"`
}

// discoverWorld is the repository a scenario stood up, and the skill it read.
type discoverWorld struct {
	// dir is the repository on disk, and files are the paths inside it the scenario named.
	dir   string
	files []string
	// held is the discover skill as this build ships it.
	held *skill.Skill
}

type discoverKey struct{}

func discoverFrom(ctx context.Context) *discoverWorld {
	d, _ := ctx.Value(discoverKey{}).(*discoverWorld)
	return d
}

func initializeDiscoverSteps(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, discoverKey{}, &discoverWorld{}), nil
	})

	// A real directory with real files in it, so a screen naming a file the repository does not hold
	// fails on the file system rather than on a list this scenario also wrote.
	sc.Step(`^a repository holding these files:$`, func(ctx context.Context, named *godog.DocString) error {
		dir, err := os.MkdirTemp("", "krewe-repository-")
		if err != nil {
			return err
		}
		sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
			return ctx, os.RemoveAll(dir)
		})
		held := discoverFrom(ctx)
		held.dir = dir
		for _, line := range strings.Split(named.Content, "\n") {
			path := strings.TrimSpace(line)
			if path == "" {
				continue
			}
			at := filepath.Join(dir, filepath.FromSlash(path))
			if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(at, []byte("the file the discovery read\n"), 0o600); err != nil {
				return err
			}
			held.files = append(held.files, path)
		}
		if len(held.files) == 0 {
			return fmt.Errorf("the scenario named no files, so a repository with nothing in it would pass")
		}
		return nil
	})

	// The driver's token, because writing a stage is the session's half of this and approving is the
	// operator's. A scenario that wrote it as the operator would leave the session's half unproved.
	sc.Step(`^a session writes the discovery that the discover skill shows$`, func(ctx context.Context) error {
		body, err := exampleFile("discovery.md")
		if err != nil {
			return err
		}
		artifact, err := exampleFile("flows.json")
		if err != nil {
			return err
		}
		if err := asDriver(ctx, func(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient) error {
			_, err := client.SetDesignStage(ctx, &quaycrewv1.SetDesignStageRequest{
				Project: worldFrom(ctx).projectID, Stage: "discovery", Body: body, Artifact: artifact,
			})
			return err
		}); err != nil {
			return err
		}
		if err := authFrom(ctx).err; err != nil {
			return fmt.Errorf("the session was refused the discovery stage: %w", err)
		}
		return nil
	})

	sc.Step(`^the discovery stage names every file of that repository$`, func(ctx context.Context) error {
		held, err := stageRead(ctx, "discovery")
		if err != nil {
			return err
		}
		for _, path := range discoverFrom(ctx).files {
			if !strings.Contains(held.GetBody(), path) {
				return fmt.Errorf("the discovery never names %s, so nobody reading it knows where that screen is", path)
			}
		}
		return nil
	})

	sc.Step(`^the discovery artifact holds (\d+) screens$`, func(ctx context.Context, want int) error {
		read, err := discoveredScreens(ctx)
		if err != nil {
			return err
		}
		if len(read) != want {
			return fmt.Errorf("the discovery artifact holds %d screens, want %d", len(read), want)
		}
		return nil
	})

	// Built, because discovery writes down what is there. A screen marked any other way is one the
	// session imagined, and the stages after this one would design it a second time.
	sc.Step(`^every screen in the discovery artifact was built already$`, func(ctx context.Context) error {
		read, err := discoveredScreens(ctx)
		if err != nil {
			return err
		}
		return everyScreenIsBuilt("the discovery artifact", read)
	})

	sc.Step(`^every screen in the discovery artifact names its surface$`, func(ctx context.Context) error {
		read, err := discoveredScreens(ctx)
		if err != nil {
			return err
		}
		for id, one := range read {
			if one.Surface != "web" && one.Surface != "mobile" {
				return fmt.Errorf("the screen %q reads surface %q, and the flow map draws web and mobile in different frames",
					id, one.Surface)
			}
		}
		return nil
	})

	sc.Step(`^every screen in the discovery artifact names a file that repository holds$`, func(ctx context.Context) error {
		read, err := discoveredScreens(ctx)
		if err != nil {
			return err
		}
		dir := discoverFrom(ctx).dir
		if dir == "" {
			return fmt.Errorf("no repository was stood up, so every source would pass")
		}
		for id, one := range read {
			if one.Source == "" {
				return fmt.Errorf("the screen %q names no file, so nobody can go and read it", id)
			}
			if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(one.Source))); err != nil {
				return fmt.Errorf("the screen %q was read from %s, and the repository holds no such file", id, one.Source)
			}
		}
		return nil
	})

	// The skill as this build ships it. It is prose and nothing else: no gate reads it, so what it
	// fails to say is a thing the discovery will not carry.
	sc.Step(`^the operator reads the discover skill$`, func(ctx context.Context) error {
		held, err := skill.Load(discoverSkillDir)
		if err != nil {
			return fmt.Errorf("loading the skills this build ships: %w", err)
		}
		for at := range held {
			if held[at].Name == "discover" {
				discoverFrom(ctx).held = &held[at]
				return nil
			}
		}
		return fmt.Errorf("%s holds no discover skill", discoverSkillDir)
	})

	sc.Step(`^the discover skill says "([^"]*)"$`, func(ctx context.Context, said string) error {
		held := discoverFrom(ctx).held
		if held == nil {
			return fmt.Errorf("the discover skill was never read")
		}
		if !strings.Contains(held.Brief, said) {
			return fmt.Errorf("the brief never says %q, so a session writes a discovery without it", said)
		}
		return nil
	})

	// The example is what a session copies, so a brief naming one that is not there teaches every
	// session to go looking for a file the image does not carry.
	sc.Step(`^the discover skill ships the example it names$`, func(ctx context.Context) error {
		held := discoverFrom(ctx).held
		if held == nil {
			return fmt.Errorf("the discover skill was never read")
		}
		for _, named := range []string{"example/discovery.md", "example/flows.json"} {
			if !strings.Contains(held.Brief, named) {
				return fmt.Errorf("the brief never names %s, so nothing points a session at the example", named)
			}
			if _, err := exampleFile(filepath.Base(named)); err != nil {
				return err
			}
		}
		return nil
	})

	sc.Step(`^the example artifact holds (\d+) screens$`, func(ctx context.Context, want int) error {
		read, err := exampleScreens()
		if err != nil {
			return err
		}
		if len(read) != want {
			return fmt.Errorf("the example holds %d screens, want %d", len(read), want)
		}
		return nil
	})

	sc.Step(`^every screen in the example was built already$`, func(ctx context.Context) error {
		read, err := exampleScreens()
		if err != nil {
			return err
		}
		return everyScreenIsBuilt("the example", read)
	})

	sc.Step(`^the installed command "([^"]*)" says it prints the discovery whole$`,
		func(ctx context.Context, name string) error {
			return installedCommandSays(ctx, name, "Print the discovery document whole")
		})

	sc.Step(`^the installed command "([^"]*)" says a no leaves the discovery unapproved$`,
		func(ctx context.Context, name string) error {
			return installedCommandSays(ctx, name, "stays unapproved")
		})
}

// discoveredScreens is the artifact the record gave back, read as the screens of a flows.json.
func discoveredScreens(ctx context.Context) (map[string]screen, error) {
	held, err := stageRead(ctx, "discovery")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(held.GetArtifact()) == "" {
		return nil, fmt.Errorf("the discovery stage carries no artifact, so there is no flows.json to read")
	}
	return screensIn([]byte(held.GetArtifact()), "the discovery stage's artifact")
}

// exampleScreens is the example beside the brief, read off disk.
func exampleScreens() (map[string]screen, error) {
	body, err := exampleFile("flows.json")
	if err != nil {
		return nil, err
	}
	return screensIn([]byte(body), "the example beside the discover brief")
}

func screensIn(body []byte, where string) (map[string]screen, error) {
	var read flows
	if err := json.Unmarshal(body, &read); err != nil {
		return nil, fmt.Errorf("%s is not json: %w", where, err)
	}
	if len(read.Screens) == 0 {
		return nil, fmt.Errorf("%s holds no screens at all, so every rule over them would pass", where)
	}
	return read.Screens, nil
}

func everyScreenIsBuilt(where string, read map[string]screen) error {
	for id, one := range read {
		if one.Status != "built" {
			return fmt.Errorf("the screen %q in %s reads status %q, and discovery writes down what exists",
				id, where, one.Status)
		}
	}
	return nil
}

// exampleFile is one file of the example the discover skill ships beside its brief.
func exampleFile(name string) (string, error) {
	at := filepath.Join(discoverSkillDir, "discover", "example", name)
	body, err := os.ReadFile(at) //nolint:gosec // the path is this build's own skills directory
	if err != nil {
		return "", fmt.Errorf("read the example the discover skill ships: %w", err)
	}
	return string(body), nil
}

// installedCommandSays reads one phrase out of the file the install put on the machine, which is
// what the operator opens.
func installedCommandSays(ctx context.Context, name, said string) error {
	body, err := installedCommand(ctx, name)
	if err != nil {
		return err
	}
	if !strings.Contains(body, said) {
		return fmt.Errorf("%s.md never says %q", name, said)
	}
	return nil
}
