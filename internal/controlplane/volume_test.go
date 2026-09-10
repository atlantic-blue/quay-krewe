package controlplane

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/model"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/secrets"
	"github.com/atlantic-blue/quay-krewe/internal/store"
)

// What a directory in a volume holds, answered by the end that holds it.
//
// The command line proves the listing a person reads. These prove the two things it cannot: a key
// arriving on the wire rather than through the tool's own parser, and the shape of the answer where
// the directory is this process's to read.
//
// The key is the reason this call exists at all. The tool cannot open the volume, so the tool cannot
// be the end that holds a key inside it, and a request reaching here is not one this system built.

// aVolumeServer is a control plane with a volume on disk, and it answers with the workspace the
// levels of a request name.
func aVolumeServer(t *testing.T) (*Server, string) {
	t.Helper()
	data := t.TempDir()
	server := NewServer(Config{
		Store:    store.NewMemory(),
		Runner:   &model.FakeRunner{Reply: "done"},
		Provider: &sandbox.FakeProvider{},
		Secrets:  secrets.NewMemory(),
		Storage:  sandbox.Storage{Dir: data, Host: data},
	})
	made, err := server.CreateWorkspace(context.Background(), &quaycrewv1.CreateWorkspaceRequest{Name: "itv"})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	return server, made.GetWorkspace().GetId()
}

// theSharedFolder is the directory a workspace address lands on, taken from the system's own answer
// rather than assembled here, so a test cannot pass against a path nothing mounts.
func theSharedFolder(t *testing.T, server *Server, workspace string) string {
	t.Helper()
	found, err := server.LocateDirectory(context.Background(), &quaycrewv1.LocateDirectoryRequest{
		Workspace: workspace,
	})
	if err != nil {
		t.Fatalf("LocateDirectory: %v", err)
	}
	return found.GetHost()
}

// The guard that matters most, on the road it now travels. The tool cleans what it parses, so a key
// that climbs cannot reach here through the tool at all: this is a request built the way anything
// else on the wire can build one.
func TestListVolumeHoldsAKeyThatClimbsInsideTheVolume(t *testing.T) {
	server, workspace := aVolumeServer(t)
	shared := theSharedFolder(t, server, workspace)
	if err := os.WriteFile(filepath.Join(filepath.Dir(shared), "passwd"), []byte("a secret"), 0o666); err != nil {
		t.Fatalf("write the file above the volume: %v", err)
	}

	said, err := server.ListVolume(context.Background(), &quaycrewv1.ListVolumeRequest{
		Workspace: workspace, Key: "../passwd",
	})

	if err == nil {
		t.Fatalf("a key that climbs out of the volume was listed as %v", said.GetEntries())
	}
	if !strings.Contains(err.Error(), shared) {
		t.Errorf("the refusal read somewhere other than the volume: %s", err)
	}
	// The key it names is the one it read, which is the climbing key held inside the volume.
	if !strings.Contains(err.Error(), `"passwd"`) {
		t.Errorf("the key was not cleaned onto the volume: %s", err)
	}
}

// A key that climbs onto a name the volume does hold reads that one, rather than the one above it.
// The refusal above says nothing about which file was opened, and this is the half that does.
func TestListVolumeReadsTheNameInsideTheVolumeAndNotTheOneAboveIt(t *testing.T) {
	server, workspace := aVolumeServer(t)
	shared := theSharedFolder(t, server, workspace)
	if err := os.WriteFile(filepath.Join(filepath.Dir(shared), "notes.txt"), []byte("the one above"), 0o666); err != nil {
		t.Fatalf("write the file above the volume: %v", err)
	}
	if err := os.WriteFile(filepath.Join(shared, "notes.txt"), []byte("mine"), 0o666); err != nil {
		t.Fatalf("write the file in the volume: %v", err)
	}

	said, err := server.ListVolume(context.Background(), &quaycrewv1.ListVolumeRequest{
		Workspace: workspace, Key: "../notes.txt",
	})

	if err != nil {
		t.Fatalf("ListVolume: %v", err)
	}
	if len(said.GetEntries()) != 1 || said.GetEntries()[0].GetSize() != int64(len("mine")) {
		t.Fatalf("the listing read %v, want the file inside the volume", said.GetEntries())
	}
}

// The names in a directory, sorted, with a size on each file and a folder marked as one.
func TestListVolumeAnswersWithWhatTheDirectoryHolds(t *testing.T) {
	server, workspace := aVolumeServer(t)
	shared := theSharedFolder(t, server, workspace)
	if err := os.WriteFile(filepath.Join(shared, "beta.txt"), []byte("bbbb"), 0o666); err != nil {
		t.Fatalf("write beta.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(shared, "alpha.txt"), []byte("aaaaa"), 0o666); err != nil {
		t.Fatalf("write alpha.txt: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(shared, "logs"), 0o777); err != nil {
		t.Fatalf("make logs: %v", err)
	}

	said, err := server.ListVolume(context.Background(), &quaycrewv1.ListVolumeRequest{Workspace: workspace})

	if err != nil {
		t.Fatalf("ListVolume: %v", err)
	}
	if said.GetHost() != shared {
		t.Errorf("the answer names %q, want the directory %q", said.GetHost(), shared)
	}
	want := []*quaycrewv1.VolumeEntry{
		{Name: "alpha.txt", Size: 5},
		{Name: "beta.txt", Size: 4},
		{Name: "logs", Directory: true},
	}
	if len(said.GetEntries()) != len(want) {
		t.Fatalf("the answer holds %d names, want %d: %v", len(said.GetEntries()), len(want), said.GetEntries())
	}
	for at, entry := range want {
		got := said.GetEntries()[at]
		if got.GetName() != entry.GetName() || got.GetSize() != entry.GetSize() ||
			got.GetDirectory() != entry.GetDirectory() {
			t.Errorf("name %d reads %v, want %v", at, got, entry)
		}
	}
}

// A key that names one file answers with that one file, so no names at all means an empty directory
// and nothing else. That is what lets the tool print a sentence rather than a table with no rows.
func TestListVolumeOnOneFileAnswersWithThatFile(t *testing.T) {
	server, workspace := aVolumeServer(t)
	shared := theSharedFolder(t, server, workspace)
	if err := os.WriteFile(filepath.Join(shared, "explore.txt"), []byte("a picture"), 0o666); err != nil {
		t.Fatalf("write explore.txt: %v", err)
	}

	said, err := server.ListVolume(context.Background(), &quaycrewv1.ListVolumeRequest{
		Workspace: workspace, Key: "explore.txt",
	})

	if err != nil {
		t.Fatalf("ListVolume: %v", err)
	}
	if said.GetHost() != filepath.Join(shared, "explore.txt") {
		t.Errorf("the answer names %q, want the file itself", said.GetHost())
	}
	if len(said.GetEntries()) != 1 || said.GetEntries()[0].GetName() != "explore.txt" {
		t.Fatalf("the answer holds %v, want the one file", said.GetEntries())
	}
}

// An empty directory answers with no names, which is a different thing from a directory that is not
// there. A volume the caller cannot open used to read as the first and was the second.
func TestListVolumeOnAnEmptyDirectoryAnswersWithNoNames(t *testing.T) {
	server, workspace := aVolumeServer(t)

	said, err := server.ListVolume(context.Background(), &quaycrewv1.ListVolumeRequest{Workspace: workspace})

	if err != nil {
		t.Fatalf("ListVolume: %v", err)
	}
	if len(said.GetEntries()) != 0 {
		t.Fatalf("an empty directory holds %v", said.GetEntries())
	}
}

// A name that is not there names the directory it read, because a name that is missing and a name in
// another folder read the same.
func TestListVolumeOnANameThatIsNotThereNamesTheDirectoryItRead(t *testing.T) {
	server, workspace := aVolumeServer(t)
	shared := theSharedFolder(t, server, workspace)

	_, err := server.ListVolume(context.Background(), &quaycrewv1.ListVolumeRequest{
		Workspace: workspace, Key: "nothing.txt",
	})

	if err == nil {
		t.Fatal("a name that is not there was listed as though it were")
	}
	if !strings.Contains(err.Error(), shared) {
		t.Errorf("the refusal does not name the directory it read: %s", err)
	}
	if !strings.Contains(err.Error(), "nothing.txt") {
		t.Errorf("the refusal does not name what was asked for: %s", err)
	}
}

// A request with no workspace on it says which one to name, rather than reading the top of the data
// directory, which holds the system's own credentials.
func TestListVolumeWithNoWorkspaceSaysToNameOne(t *testing.T) {
	server, _ := aVolumeServer(t)

	_, err := server.ListVolume(context.Background(), &quaycrewv1.ListVolumeRequest{})

	if err == nil {
		t.Fatal("a request naming no workspace was answered")
	}
	if !strings.Contains(err.Error(), "workspace") {
		t.Errorf("the refusal does not say to name a workspace: %s", err)
	}
}
