package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/commands"
)

// commandsIn runs one word of krewe commands against a directory of its own, on a machine that is
// not a sandbox, and hands back what it printed.
func commandsIn(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	t.Setenv(commands.DirectoryEnv, dir)
	var out bytes.Buffer
	err := runCommands(args, &out, false)
	return out.String(), err
}

// Nothing installed is the state every operator starts in, so it has to say where the files go and
// what to type, rather than reading as a broken command.
func TestNothingInstalledNamesTheDirectoryAndSaysHowToInstall(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not-there")

	_, err := commandsIn(t, dir)
	if err == nil {
		t.Fatal("krewe commands answered for a directory that is not there")
	}
	for _, want := range []string{dir, "krewe commands install"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q: %v", want, err)
		}
	}
}

// After an install the two builds are the same one, which is the answer to "are my commands this
// build's".
func TestAfterAnInstallItPrintsTheSameBuildTwice(t *testing.T) {
	dir := t.TempDir()

	if _, err := commandsIn(t, dir, "install"); err != nil {
		t.Fatalf("install: %v", err)
	}
	printed, err := commandsIn(t, dir)
	if err != nil {
		t.Fatalf("krewe commands: %v", err)
	}
	if strings.Count(printed, version) != 2 {
		t.Errorf("krewe commands does not print %q twice:\n%s", version, printed)
	}
	if strings.Contains(printed, "Run krewe commands install") {
		t.Errorf("krewe commands says to install again over its own build:\n%s", printed)
	}
}

// The reason the marker names a build at all: an upgrade moves the binary and leaves the files
// behind, and nothing else would say so.
func TestADifferingBuildSaysToInstallAgain(t *testing.T) {
	dir := t.TempDir()
	writeCommandFile(t, dir, commands.All()[0].FileName(), commands.Marker("older11")+"\n---\n")

	printed, err := commandsIn(t, dir)
	if err != nil {
		t.Fatalf("krewe commands: %v", err)
	}
	for _, want := range []string{"older11", version, "Run krewe commands install"} {
		if !strings.Contains(printed, want) {
			t.Errorf("krewe commands does not say %q:\n%s", want, printed)
		}
	}
}

// A file krewe did not write is listed rather than hidden, and the word is unknown rather than a
// blank, because a blank column reads as a file that is not there.
func TestAFileWithNoMarkerIsListedAsUnknown(t *testing.T) {
	dir := t.TempDir()
	writeCommandFile(t, dir, commands.All()[0].FileName(), "what the operator wrote themselves\n")

	printed, err := commandsIn(t, dir)
	if err != nil {
		t.Fatalf("krewe commands: %v", err)
	}
	if !strings.Contains(printed, commands.UnknownBuild) {
		t.Errorf("krewe commands does not say %q:\n%s", commands.UnknownBuild, printed)
	}
}

// The listing reads the embedded files, so it answers on a machine where nothing was installed and
// says what this binary would write rather than what is there.
func TestTheListingAnswersBeforeAnythingIsInstalled(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not-there")

	printed, err := commandsIn(t, dir, "list")
	if err != nil {
		t.Fatalf("krewe commands list: %v", err)
	}
	for _, one := range commands.All() {
		if !strings.Contains(printed, one.Slash()) {
			t.Errorf("the listing does not name %s:\n%s", one.Slash(), printed)
		}
		if !strings.Contains(printed, one.Description) {
			t.Errorf("the listing does not say what %s does:\n%s", one.Slash(), printed)
		}
	}
}

// The install names every path it wrote, so the operator can open one, and ends with the count and
// the directory.
func TestTheInstallNamesEveryPathItWrote(t *testing.T) {
	dir := t.TempDir()

	printed, err := commandsIn(t, dir, "install")
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	for _, one := range commands.All() {
		if !strings.Contains(printed, filepath.Join(dir, one.FileName())) {
			t.Errorf("the install does not name where %s went:\n%s", one.FileName(), printed)
		}
	}
	if !strings.Contains(printed, dir) {
		t.Errorf("the install does not name the directory:\n%s", printed)
	}
}

// The absence that matters most in this slice. An absence proved by not writing the code is one the
// next person to think a force flag would be helpful can undo with nothing going red, so the flag
// table is read and every spelling a person reaches for is driven through the tool.
func TestNoFlagWritesOverAFileKreweDidNotWrite(t *testing.T) {
	if taken := takenFlags["commands"]; len(taken) != 0 {
		t.Fatalf("krewe commands takes flags now: %v. A file krewe did not write stays until "+
			"somebody removes it, so none of them may write over one", taken)
	}

	client := testClient(t)
	dir := t.TempDir()
	t.Setenv(commands.DirectoryEnv, dir)
	byHand := filepath.Join(dir, commands.All()[0].FileName())
	writeCommandFile(t, dir, commands.All()[0].FileName(), "what the operator wrote themselves\n")

	for _, forcing := range []string{"--force", "--replace", "--overwrite", "--anyway", "--yes"} {
		err := run(context.Background(), client, []string{"commands", "install", forcing}, io.Discard, "")
		if err == nil {
			t.Errorf("krewe commands install %s was accepted", forcing)
		}
		held, readErr := os.ReadFile(byHand)
		if readErr != nil {
			t.Fatalf("the file the operator wrote is gone after %s: %v", forcing, readErr)
		}
		if string(held) != "what the operator wrote themselves\n" {
			t.Fatalf("%s wrote over the file the operator wrote: %q", forcing, held)
		}
	}
}

// A slash command is a file the operator's own terminal runs, outside every sandbox, so a session
// that could write one could hand them anything. The tool asks the deny policy before it writes,
// which is the same list every other refusal a session meets is written in.
func TestASessionThatTriesToInstallIsRefused(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(commands.DirectoryEnv, dir)

	var out bytes.Buffer
	err := runCommands([]string{"install"}, &out, true)
	if err == nil {
		t.Fatal("a session installed the operator's own slash commands")
	}
	if !strings.Contains(err.Error(), "operator's to make") {
		t.Errorf("the refusal does not say the write is the operator's: %v", err)
	}
	// A refusal that reads as a failed call would send whoever read it looking at the system.
	if strings.Contains(err.Error(), "rpc error") {
		t.Errorf("the refusal reads as a call that failed, and nothing called anything: %v", err)
	}
	for _, one := range commands.All() {
		if _, statErr := os.Stat(filepath.Join(dir, one.FileName())); !os.IsNotExist(statErr) {
			t.Errorf("%s was written by a session", one.FileName())
		}
	}
}

// The read word installs nothing. An operator asking where they stand must not change where they
// stand by asking.
func TestTheReadWordInstallsNothing(t *testing.T) {
	dir := t.TempDir()

	if _, err := commandsIn(t, dir); err != nil {
		t.Fatalf("krewe commands: %v", err)
	}
	held, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(held) != 0 {
		t.Errorf("krewe commands wrote %d files into %s", len(held), dir)
	}
}

// A word this command does not have says what the three are, rather than reading as the tool being
// broken.
func TestAWordCommandsDoesNotHaveNamesTheOnesItDoes(t *testing.T) {
	_, err := commandsIn(t, t.TempDir(), "write")
	if err == nil {
		t.Fatal("krewe commands write was accepted")
	}
	for _, want := range []string{"krewe commands install", "krewe commands list"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q: %v", want, err)
		}
	}
}

// The manual is what a session is told with, so a command the tool has and the manual does not is a
// command nothing running in a session knows about.
func TestTheManualCarriesTheCommandsWord(t *testing.T) {
	client := testClient(t)

	printed := mustRun(t, client, "help")
	for _, want := range []string{"commands install", "commands list"} {
		if !strings.Contains(printed, want) {
			t.Errorf("the usage does not name %q", want)
		}
	}
}

func writeCommandFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
