package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
)

// aProjectWithADesign is a project holding one body, which is the state every edit starts from.
func aProjectWithADesign(t *testing.T, body string) quaycrewv1.ControlPlaneServiceClient {
	t.Helper()
	client := testClient(t)
	mustRun(t, client, "workspace", "create", "acme")
	mustRun(t, client, "project", "create", "house-bills")
	file := filepath.Join(t.TempDir(), "design.md")
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	mustRun(t, client, "design", "set", flagFile, file)
	return client
}

// editorDouble is a script standing in for the operator's editor, which is all an editor is from the
// tool's side: a command run on a file. It records the file it was given and what that file held,
// then saves whatever the test told it to save.
type editorDouble struct{ dir string }

// anEditorThat writes one double and points VISUAL at it. saves is what it writes into the file it
// was opened on, and status is what it stops with.
func anEditorThat(t *testing.T, saves string, saved bool, status int) editorDouble {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"echo \"$1\" > " + filepath.Join(dir, "opened") + "\n" +
		"cp \"$1\" " + filepath.Join(dir, "seen") + "\n"
	if saved {
		body := filepath.Join(dir, "saves.md")
		if err := os.WriteFile(body, []byte(saves), 0o600); err != nil {
			t.Fatal(err)
		}
		script += "cp " + body + " \"$1\"\n"
	}
	script += "exit " + strconv.Itoa(status) + "\n"
	at := filepath.Join(dir, "editor")
	if err := os.WriteFile(at, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", at)
	t.Setenv("EDITOR", "")
	return editorDouble{dir: dir}
}

// opened is the draft krewe put in front of the operator, read back from where the double recorded
// it.
func (e editorDouble) opened(t *testing.T) string {
	t.Helper()
	at, err := os.ReadFile(filepath.Join(e.dir, "opened"))
	if err != nil {
		t.Fatalf("no editor was opened on anything: %v", err)
	}
	return strings.TrimSpace(string(at))
}

// seen is what that draft held when it opened. The draft itself is gone by the time a test asks, so
// the double keeps a copy.
func (e editorDouble) seen(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(e.dir, "seen"))
	if err != nil {
		t.Fatalf("no editor was opened on anything: %v", err)
	}
	return string(body)
}

// designOf is what the store holds, printed the way an operator reads it.
func designOf(t *testing.T, client quaycrewv1.ControlPlaneServiceClient) string {
	t.Helper()
	return mustRun(t, client, "design")
}

// The whole point of the command. A design somebody edited and saved has to reach the store, or they
// have written a note on one machine.
func TestEditingADesignSendsBackWhatWasSaved(t *testing.T) {
	client := aProjectWithADesign(t, "# Bills\n")
	anEditorThat(t, "# Bills, again\n", true, 0)

	mustRun(t, client, "design", "edit")

	if got := designOf(t, client); !strings.Contains(got, "# Bills, again") {
		t.Fatalf("the store holds %q, and the editor saved \"# Bills, again\"", got)
	}
}

// An editor opened on an empty file loses the design the moment somebody saves, and every other test
// here passes just the same.
func TestTheEditorIsOpenedOnTheDesignAsItStands(t *testing.T) {
	body := "# Bills\n\nPay the water bill first.\n"
	client := aProjectWithADesign(t, body)
	editor := anEditorThat(t, "", false, 0)

	mustRun(t, client, "design", "edit")

	if got := editor.seen(t); got != body {
		t.Fatalf("the editor was opened on %q, want %q", got, body)
	}
}

// A draft left behind is a second copy of the design on the machine, and the next reader cannot tell
// which of the two krewe holds.
func TestTheDraftIsRemovedAfterwards(t *testing.T) {
	client := aProjectWithADesign(t, "# Bills\n")
	editor := anEditorThat(t, "# Bills, again\n", true, 0)

	printed := mustRun(t, client, "design", "edit")

	draft := editor.opened(t)
	if held, err := os.ReadFile(draft); err == nil {
		t.Fatalf("%s is still there, saying %q", draft, held)
	}
	// The path is printed, so somebody watching an editor open knows which file it is.
	if !strings.Contains(printed, draft) {
		t.Errorf("the command never named the file it opened: %q", printed)
	}
}

// Approval is the operator's word about a text. Nothing here can tell a text nobody touched from a
// rewritten one that reads the same, so it writes either way and says the word went.
func TestLeavingTheEditorUnchangedStillWritesAndSaysTheApprovalWent(t *testing.T) {
	client := aProjectWithADesign(t, "# Bills\n")
	mustRun(t, client, "design", "approve")
	anEditorThat(t, "", false, 0)

	printed := mustRun(t, client, "design", "edit")

	if !strings.Contains(printed, "the approval is cleared") {
		t.Fatalf("an edit that changed nothing said nothing about the approval: %q", printed)
	}
	held := designOf(t, client)
	if !strings.Contains(held, "approval: not approved") {
		t.Fatalf("the design still reads as approved after an edit: %q", held)
	}
	if !strings.Contains(held, "# Bills") {
		t.Fatalf("the body did not survive an edit that changed nothing: %q", held)
	}
}

// Quitting the editor is a thing people do. Writing the draft back anyway would take the approval
// away for an edit nobody made.
func TestAnEditorThatStopsWithAnErrorWritesNothing(t *testing.T) {
	client := aProjectWithADesign(t, "# Bills\n")
	mustRun(t, client, "design", "approve")
	editor := anEditorThat(t, "# Bills, again\n", true, 1)

	err := refused(t, client, "design", "edit")

	if !strings.Contains(err.Error(), "nothing was written") {
		t.Errorf("the refusal does not say the store was left alone: %s", err)
	}
	held := designOf(t, client)
	if !strings.Contains(held, "# Bills") || strings.Contains(held, "again") {
		t.Fatalf("the store holds %q, and the edit was abandoned", held)
	}
	if !strings.Contains(held, "approval: approved") {
		t.Fatalf("an abandoned edit took the approval away: %q", held)
	}
	// The draft goes whichever way the editor ended, or a quit leaves a copy of the design behind.
	if _, err := os.Stat(editor.opened(t)); !os.IsNotExist(err) {
		t.Errorf("%s is still there after the editor stopped", editor.opened(t))
	}
}

// One address, because a second one is a typed mistake rather than a second edit, and an editor
// opened on it would edit a project nobody named.
func TestEditingMoreThanOneAddressIsRefused(t *testing.T) {
	client := aProjectWithADesign(t, "# Bills\n")
	anEditorThat(t, "# Bills, again\n", true, 0)

	err := refused(t, client, "design", "edit", "acme/house-bills", "acme/house-bills")

	if !strings.Contains(err.Error(), "usage: krewe design edit [<address>]") {
		t.Errorf("the refusal does not say how to type it: %s", err)
	}
	if held := designOf(t, client); strings.Contains(held, "again") {
		t.Fatalf("a refused command edited the design anyway: %q", held)
	}
}

// VISUAL, then EDITOR, then vi: the order git and crontab use. The last one is why this works on a
// machine where neither variable is set, which is most machines.
func TestTheEditorIsVisualThenEditorThenVi(t *testing.T) {
	tests := []struct {
		name           string
		visual, editor bool
		want           string
	}{
		{"VISUAL wins", true, true, "# from VISUAL"},
		{"EDITOR when VISUAL says nothing", false, true, "# from EDITOR"},
		{"vi when neither says anything", false, false, "# from vi"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := aProjectWithADesign(t, "# Bills\n")
			t.Setenv("VISUAL", "")
			t.Setenv("EDITOR", "")
			if test.visual {
				t.Setenv("VISUAL", anEditorSaving(t, "editor", "# from VISUAL\n"))
			}
			if test.editor {
				t.Setenv("EDITOR", anEditorSaving(t, "editor", "# from EDITOR\n"))
			}
			if !test.visual && !test.editor {
				// A vi of our own in front of the machine's, because the real one opens a screen and
				// waits for somebody to quit it.
				at := anEditorSaving(t, "vi", "# from vi\n")
				t.Setenv("PATH", filepath.Dir(at)+string(os.PathListSeparator)+os.Getenv("PATH"))
			}

			mustRun(t, client, "design", "edit")

			if got := designOf(t, client); !strings.Contains(got, test.want) {
				t.Fatalf("the store holds %q, want what the editor saying %q wrote", got, test.want)
			}
		})
	}
}

// anEditorSaving is an editor that saves one text and does nothing else. The name is its own,
// because the fallback is proved by a command called vi and the two others by whatever a variable
// points at.
func anEditorSaving(t *testing.T, name, saves string) string {
	t.Helper()
	dir := t.TempDir()
	body := filepath.Join(dir, "saves.md")
	if err := os.WriteFile(body, []byte(saves), 0o600); err != nil {
		t.Fatal(err)
	}
	at := filepath.Join(dir, name)
	if err := os.WriteFile(at, []byte("#!/bin/sh\ncp "+body+" \"$1\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return at
}

// The one argument form of krewe design proof. A person standing in a project types the command
// alone, and a person naming the project they mean types the address alone, so the word has to tell
// the two apart from what was typed. A shell command carries a space or the scenario token; an
// address is names joined by slashes and carries neither.
//
// The scenarios in features/design.feature always name the address, because the tool they run reads
// where it is standing from the machine it runs on. This is the other branch.
func TestOneArgumentIsTheProofCommandOrTheAddress(t *testing.T) {
	for _, test := range []struct {
		name  string
		typed string
		wrote string
	}{
		{
			name:  "a command carrying a space",
			typed: "go test ./features/... -run '{scenario}'",
			wrote: "go test ./features/... -run '{scenario}'",
		},
		{
			// No space anywhere in it, so the token is the only thing saying this is a command.
			name:  "a command carrying only the token",
			typed: "./proof.sh{scenario}",
			wrote: "./proof.sh{scenario}",
		},
		{
			// An address, so nothing is written and the project reads back as proving nothing.
			name:  "an address",
			typed: "acme/house-bills",
			wrote: "",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := testClient(t)
			mustRun(t, client, "workspace", "create", "acme")
			mustRun(t, client, "project", "create", "house-bills")

			mustRun(t, client, "design", "proof", test.typed)

			design, err := client.GetDesign(context.Background(), &quaycrewv1.GetDesignRequest{
				Project: projectNamed(t, client, "house-bills")})
			if err != nil {
				t.Fatalf("GetDesign: %v", err)
			}
			if got := design.GetDesign().GetProofCommand(); got != test.wrote {
				t.Fatalf("the project proves one scenario with %q, want %q", got, test.wrote)
			}
		})
	}
}

// The pattern and the budget are flags, and both of them reach the store together with the command.
// A flag with no command is refused rather than ignored: a caller who meant to change the pattern
// would otherwise read the old one back and believe the write landed.
func TestTheProofFlagsReachTheStoreAndNeedACommand(t *testing.T) {
	client := testClient(t)
	mustRun(t, client, "workspace", "create", "acme")
	mustRun(t, client, "project", "create", "house-bills")

	mustRun(t, client, "design", "proof", "make one {scenario}", "--pattern", "ran ([0-9]+)", "--timeout", "120")

	design, err := client.GetDesign(context.Background(), &quaycrewv1.GetDesignRequest{
		Project: projectNamed(t, client, "house-bills")})
	if err != nil {
		t.Fatalf("GetDesign: %v", err)
	}
	if got := design.GetDesign().GetProofCountPattern(); got != "ran ([0-9]+)" {
		t.Errorf("the project reads the count with %q", got)
	}
	if got := design.GetDesign().GetProofTimeoutSeconds(); got != 120 {
		t.Errorf("one run has %d seconds, want 120", got)
	}

	var out bytes.Buffer
	err = run(context.Background(), client, []string{"design", "proof", "--pattern", "ran ([0-9]+)"}, &out, "")
	if err == nil {
		t.Fatal("a pattern with no command was accepted, so a caller reads the old one back and believes it landed")
	}
	if !strings.Contains(err.Error(), "belongs to") {
		t.Errorf("the refusal is %q, and never says the pattern goes with a command", err)
	}
}

// projectNamed is the identifier of one project of the system under test, which the tool never shows
// and every design call needs.
func projectNamed(t *testing.T, client quaycrewv1.ControlPlaneServiceClient, name string) string {
	t.Helper()
	resp, err := client.ListProjects(context.Background(), &quaycrewv1.ListProjectsRequest{})
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	for _, project := range resp.GetProjects() {
		if project.GetName() == name {
			return project.GetId()
		}
	}
	t.Fatalf("no project called %s", name)
	return ""
}
