package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/store"
)

// aStagedProject is a project with nothing written on it, which is where every stage starts.
func aStagedProject(t *testing.T) quaycrewv1.ControlPlaneServiceClient {
	t.Helper()
	client := testClient(t)
	mustRun(t, client, "workspace", "create", "acme")
	mustRun(t, client, "project", "create", "house-bills")
	return client
}

// aFileSaying writes one document for a command to read, and hands back its path.
func aFileSaying(t *testing.T, name, body string) string {
	t.Helper()
	at := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(at, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return at
}

// settle writes a stage and approves it, which is the only way past the rule that orders the six.
//
// The data model and the architecture are refused without a diagram, so the file written for those
// two holds one.
func settle(t *testing.T, client quaycrewv1.ControlPlaneServiceClient, stage string) {
	t.Helper()
	mustRun(t, client, "stage", "set", stage, flagFile, aFileSaying(t, stage+".md", stageFileFor(stage)))
	mustRun(t, client, "stage", "approve", stage)
}

// stageFileFor is what the settle helper writes into the file for one stage.
func stageFileFor(stage string) string {
	text := "the " + stage + "\n"
	if stage == store.StageDataModel || stage == store.StageArchitecture {
		return text + "\n```mermaid\nflowchart TD\n  one --> two\n```\n"
	}
	return text
}

// The listing says where the project is up to, so it names all six. A listing of the stages that
// exist would show one row on a project with five stages still to write, and the four after the one
// being written are the part a person is deciding about.
func TestTheListingNamesAllSixStagesEvenWhenNothingIsWritten(t *testing.T) {
	client := aStagedProject(t)

	printed := mustRun(t, client, "stage", "show")

	for _, stage := range store.DesignStages() {
		if !strings.Contains(printed, stage) {
			t.Errorf("the listing leaves out %s:\n%s", stage, printed)
		}
	}
	if got := strings.Count(printed, "empty"); got != 6 {
		t.Errorf("the listing says empty %d times, want 6:\n%s", got, printed)
	}
}

// The one move the project has. An operator reading six rows of empty has to know the order to work
// out which of them they may write, and the order is the thing this feature exists to hold.
func TestTheListingNamesTheStageToWriteNext(t *testing.T) {
	client := aStagedProject(t)

	printed := mustRun(t, client, "stage", "show")

	if !strings.Contains(printed, "krewe stage set discovery "+flagFile) {
		t.Errorf("the listing does not say how to write the first stage:\n%s", printed)
	}
}

// Written and approved are two states, and the move differs. A stage that is written needs reading,
// not writing again, so the line under the listing changes with it.
func TestAWrittenStageIsOfferedForApprovalRatherThanForWritingAgain(t *testing.T) {
	client := aStagedProject(t)
	mustRun(t, client, "stage", "set", "discovery", flagFile, aFileSaying(t, "d.md", "they pay four bills\n"))

	printed := mustRun(t, client, "stage", "show")

	if !strings.Contains(printed, "written") {
		t.Errorf("the listing does not say the stage is written:\n%s", printed)
	}
	if !strings.Contains(printed, "krewe stage approve discovery") {
		t.Errorf("the listing does not offer the approval:\n%s", printed)
	}
}

// The whole point of the write: a document on a machine reaches the project, whole.
func TestWritingAStageSendsTheWholeDocument(t *testing.T) {
	client := aStagedProject(t)
	body := "# Discovery\n\nThey pay four bills, and two move.\n"

	printed := mustRun(t, client, "stage", "set", "discovery", flagFile, aFileSaying(t, "d.md", body))

	held := stageHeld(t, client, "discovery")
	if held.GetBody() != body {
		t.Errorf("the project holds %q, want %q", held.GetBody(), body)
	}
	if !strings.Contains(printed, "48 characters") {
		t.Errorf("the write does not say how much it wrote:\n%s", printed)
	}
}

// A person who reads this twice learns the rule, which is that approval is a statement about one
// text. Learning it by being refused a take three days later is learning it too late.
func TestAWriteSaysTheApprovalIsCleared(t *testing.T) {
	client := aStagedProject(t)

	printed := mustRun(t, client, "stage", "set", "discovery", flagFile, aFileSaying(t, "d.md", "what we asked\n"))

	if !strings.Contains(printed, "the approval is cleared") {
		t.Errorf("the write says nothing about the approval:\n%s", printed)
	}
}

// The operator's word, recorded against the text that is there now.
func TestApprovingAStageRecordsTheWord(t *testing.T) {
	client := aStagedProject(t)
	mustRun(t, client, "stage", "set", "discovery", flagFile, aFileSaying(t, "d.md", "what we asked\n"))

	printed := mustRun(t, client, "stage", "approve", "discovery")

	if held := stageHeld(t, client, "discovery"); !held.GetApproved() {
		t.Fatalf("the discovery stage reads approved=false at version %d", held.GetVersion())
	}
	if !strings.Contains(printed, "you approved") {
		t.Errorf("the approval does not say the word was recorded:\n%s", printed)
	}
	if listed := mustRun(t, client, "stage", "show"); !strings.Contains(listed, "approved") {
		t.Errorf("the listing does not say the stage is approved:\n%s", listed)
	}
}

// The refusal the whole feature exists for, read from the command line. It names the stage to go and
// approve, and it says nothing was written, because an operator who thinks half a write landed goes
// looking for it.
func TestWritingAStageOutOfOrderIsRefusedAndNamesTheStageToApprove(t *testing.T) {
	client := aStagedProject(t)

	err := refused(t, client, "stage", "set", "data_model", flagFile, aFileSaying(t, "m.md", "one table\n"))

	for _, want := range []string{"discovery", "krewe stage approve", "nothing was written"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not say %q: %v", want, err)
		}
	}
	if held := stageHeld(t, client, "data_model"); held != nil {
		t.Errorf("the data_model stage was written anyway: %q", held.GetBody())
	}
}

// An address in front of the stage reaches a project the operator is not standing in, which is how
// every other command takes one.
func TestAStageIsWrittenAndApprovedSomewhereElse(t *testing.T) {
	client := aStagedProject(t)
	mustRun(t, client, "workspace", "create", "other")

	mustRun(t, client, "stage", "set", "acme/house-bills", "discovery",
		flagFile, aFileSaying(t, "d.md", "what we asked\n"))
	mustRun(t, client, "stage", "approve", "acme/house-bills", "discovery")

	if held := stageHeld(t, client, "discovery"); !held.GetApproved() {
		t.Fatal("the stage was not approved in the project the address named")
	}
	if printed := mustRun(t, client, "stage", "show", "acme/house-bills"); !strings.Contains(printed, "approved") {
		t.Errorf("the listing of the named project does not say the stage is approved:\n%s", printed)
	}
}

// The artifact is the structured document a stage carries beside its prose, such as the flows the
// mockups are played from. It is a second file for the reason the body is a file at all.
func TestAStageCarriesAnArtifactFromASecondFile(t *testing.T) {
	client := aStagedProject(t)
	artifact := `{"asked":["when does it move"]}`

	printed := mustRun(t, client, "stage", "set", "discovery",
		flagFile, aFileSaying(t, "d.md", "what we asked\n"),
		flagArtifact, aFileSaying(t, "asked.json", artifact))

	if held := stageHeld(t, client, "discovery"); held.GetArtifact() != artifact {
		t.Errorf("the stage carries %q, want %q", held.GetArtifact(), artifact)
	}
	if !strings.Contains(printed, "it carries an artifact") {
		t.Errorf("the write says nothing about the artifact:\n%s", printed)
	}
}

// The artifact is json, and json is not what a person approves. The mockups stage is a flows.json
// with a flow map drawn from it, so the stage carries the address of the page as well as the data,
// and the write prints it back.
func TestAStageRecordsWhereTheArtifactWasPublished(t *testing.T) {
	client := aStagedProject(t)
	published := "https://example.invalid/tide/flow-map/"

	printed := mustRun(t, client, "stage", "set", "discovery",
		flagFile, aFileSaying(t, "d.md", "what we asked\n"),
		flagArtifact, aFileSaying(t, "flows.json", `{"screens":{}}`),
		flagURL, published)

	if held := stageHeld(t, client, "discovery"); held.GetArtifactUrl() != published {
		t.Errorf("the stage was published at %q, want %q", held.GetArtifactUrl(), published)
	}
	if !strings.Contains(printed, published) {
		t.Errorf("the write does not say where the artifact was published:\n%s", printed)
	}
}

// The address may be given on its own. A discovery stage whose page is already up carries no
// second file, and a flag that needed one would send somebody to write an empty artifact.
func TestAnAddressIsKeptWithoutAnArtifact(t *testing.T) {
	client := aStagedProject(t)

	mustRun(t, client, "stage", "set", "discovery",
		flagFile, aFileSaying(t, "d.md", "what we asked\n"),
		flagURL, "https://example.invalid/tide/")

	held := stageHeld(t, client, "discovery")
	if held.GetArtifactUrl() != "https://example.invalid/tide/" {
		t.Errorf("the address was not kept: %q", held.GetArtifactUrl())
	}
	if held.GetArtifact() != "" {
		t.Errorf("an artifact appeared from nowhere: %q", held.GetArtifact())
	}
}

// The system keeps an artifact as json so a reader can open it as json. The refusal is the control
// plane's, and it has to reach the operator rather than be swallowed by the tool.
func TestAnArtifactThatIsNotJsonIsRefused(t *testing.T) {
	client := aStagedProject(t)

	err := refused(t, client, "stage", "set", "discovery",
		flagFile, aFileSaying(t, "d.md", "what we asked\n"),
		flagArtifact, aFileSaying(t, "asked.json", "the flows are over there"))

	if !strings.Contains(err.Error(), "json") {
		t.Errorf("the refusal does not say the artifact is not json: %v", err)
	}
	if held := stageHeld(t, client, "discovery"); held != nil {
		t.Errorf("the stage was written anyway: %q", held.GetBody())
	}
}

// An empty document is nothing to agree to, and a stage written empty would sit there reading
// written while it says nothing at all.
func TestAnEmptyStageFileIsRefused(t *testing.T) {
	client := aStagedProject(t)

	err := refused(t, client, "stage", "set", "discovery", flagFile, aFileSaying(t, "d.md", "   \n"))

	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("the refusal does not say the file is empty: %v", err)
	}
	if held := stageHeld(t, client, "discovery"); held != nil {
		t.Errorf("the stage was written anyway: %q", held.GetBody())
	}
}

// A file that is not there is the ordinary typing mistake, and the refusal names the path so the
// operator can see what they typed.
func TestAStageFileThatIsNotThereNamesThePath(t *testing.T) {
	client := aStagedProject(t)

	err := refused(t, client, "stage", "set", "discovery", flagFile, "/no/such/discovery.md")

	if !strings.Contains(err.Error(), "/no/such/discovery.md") {
		t.Errorf("the refusal does not name the path: %v", err)
	}
}

// Every shape the tool refuses on its own, and each one prints the three forms so the operator reads
// what they meant to type.
func TestTheShapesTheStageCommandRefuses(t *testing.T) {
	client := aStagedProject(t)
	file := aFileSaying(t, "d.md", "what we asked\n")

	for _, typed := range [][]string{
		{"stage"},
		{"stage", "wibble"},
		{"stage", "set", "discovery"},
		{"stage", "set", flagFile},
		{"stage", "set", flagArtifact, "a.json", flagFile, file, "acme/house-bills", "discovery", "extra"},
		{"stage", "set", "discovery", flagFile, file, flagURL},
		{"stage", "approve"},
		{"stage", "approve", "acme/house-bills", "discovery", "extra"},
		{"stage", "show", "acme/house-bills", "extra"},
	} {
		err := refused(t, client, typed...)
		if !strings.Contains(err.Error(), "usage: krewe stage") {
			t.Errorf("krewe %s was refused without the usage: %v", strings.Join(typed, " "), err)
		}
	}
}

// A flag the command does not take is refused by name before the command reads anything, so the
// allow list and the parser cannot disagree about which flags exist.
func TestTheStageCommandTakesItsThreeFlagsAndNoOthers(t *testing.T) {
	client := aStagedProject(t)

	err := refused(t, client, "stage", "set", "discovery", "--body", "anything")
	if !strings.Contains(err.Error(), "--body is not one") {
		t.Errorf("the refusal does not name the flag: %v", err)
	}
	for _, flag := range []string{flagFile, flagArtifact, flagURL} {
		if !takenFlags["stage"][flag] {
			t.Errorf("krewe stage does not take %s, so the command never sees it", flag)
		}
	}
}

// The listing reads every state a stage can be in, including the one nothing writes yet. A stage
// that was skipped is not a stage somebody agreed to, and a person deciding what to do next has to
// be able to tell the two apart.
func TestTheWordForEachStateAStageCanBeIn(t *testing.T) {
	for _, one := range []struct {
		what  string
		stage *quaycrewv1.DesignStage
		want  string
	}{
		{"a stage nobody wrote", nil, "empty"},
		{"a stage written and not read", &quaycrewv1.DesignStage{Version: 1}, "written"},
		{"a stage the operator approved", &quaycrewv1.DesignStage{Version: 1, Approved: true}, "approved"},
		{"a stage written over since it was approved", &quaycrewv1.DesignStage{Version: 2}, "written"},
		{"a stage that was skipped", &quaycrewv1.DesignStage{Skipped: true}, "skipped"},
	} {
		if got := stageState(one.stage); got != one.want {
			t.Errorf("%s reads %q, want %q", one.what, got, one.want)
		}
	}
}

// A skipped stage settles the rule that orders the six, so the listing must not offer it back as the
// next thing to write. Nothing writes the column yet, which is why this is read here rather than
// through the command.
func TestASkippedStageIsNotOfferedAsTheNextMove(t *testing.T) {
	held := []*quaycrewv1.DesignStage{
		{Stage: store.StageDiscovery, Skipped: true},
		{Stage: store.StageStories, Version: 1, Approved: true},
	}

	if got := stageToWriteNext(held); got != store.StageDesignSystem {
		t.Errorf("the next move is %q, want %q", got, store.StageDesignSystem)
	}
}

// Six approved stages leave nothing to write, and the listing says so rather than offering a
// seventh.
func TestAProjectWithEverySixStagesApprovedIsOfferedNothingToWrite(t *testing.T) {
	client := aStagedProject(t)
	for _, stage := range store.DesignStages() {
		settle(t, client, stage)
	}

	printed := mustRun(t, client, "stage", "show")

	if strings.Contains(printed, "krewe stage set") {
		t.Errorf("the listing still offers a stage to write:\n%s", printed)
	}
	if !strings.Contains(printed, "every stage carries your word") {
		t.Errorf("the listing does not say the project is designed:\n%s", printed)
	}
	if got := strings.Count(printed, "approved"); got != 6 {
		t.Errorf("the listing says approved %d times, want 6:\n%s", got, printed)
	}
}

// stageHeld is one stage as the project holds it, and nil for a stage nobody wrote.
func stageHeld(t *testing.T, client quaycrewv1.ControlPlaneServiceClient, stage string) *quaycrewv1.DesignStage {
	t.Helper()
	located, err := locate(context.Background(), client, "acme/house-bills")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.ListDesignStages(context.Background(),
		&quaycrewv1.ListDesignStagesRequest{Project: located.ProjectID})
	if err != nil {
		t.Fatal(err)
	}
	return stageNamed(resp.GetStages(), stage)
}
