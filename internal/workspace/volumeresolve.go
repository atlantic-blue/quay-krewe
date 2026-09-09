package workspace

import (
	"context"
	"errors"
	"strings"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
)

// The other half of reading an address: asking the system which of the levels are really there.
//
// ParseVolumePath fills the levels as deep as they go and marks the shallowest one the string alone
// does not prove. `krewe://itv/notes` is the notes project of itv, or the file called notes in its
// shared folder, and nothing in those 17 characters says which. So the guess is taken to the system
// here: the longest prefix of the levels that exists is the directory, and the rest is the file.

// VolumeLocation is a volume address after the system answered for it.
type VolumeLocation struct {
	// Where are the identifiers the control plane works in, for the levels that exist.
	Where Location
	// Address is the reading that survived: the levels the system holds, and the key that is the rest.
	Address VolumePath
}

// ResolveVolume answers what an address names, against what the system holds.
//
// A level the string proved is looked up and a failure stays a failure: `krewe://itv/vast/9e8153f6`
// names a session this system made, so a missing one says the session is missing rather than reading
// the identifier as the name of a file. A level the string guessed at is demoted when it is not
// there, because the segment was a file name all along, and so is everything under it.
func ResolveVolume(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient, address VolumePath) (VolumeLocation, error) {
	found, err := resolveVolume(ctx, client, address)
	if err != nil {
		return VolumeLocation{}, err
	}
	found.Where.Path = found.Address.Path()
	return found, nil
}

func resolveVolume(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient, address VolumePath) (VolumeLocation, error) {
	workspaceID, err := Resolve(ctx, client, address.Workspace)
	if err != nil {
		return VolumeLocation{}, err
	}
	found := VolumeLocation{
		Where:   Location{WorkspaceID: workspaceID},
		Address: VolumePath{Workspace: address.Workspace, Kind: VolumeShared, Key: address.Key},
	}
	if address.Project == "" {
		return found, nil
	}

	projectID, err := ResolveProject(ctx, client, address.Workspace, address.Project)
	if err != nil {
		if !demotes(address, err) {
			return VolumeLocation{}, err
		}
		found.Address.Key = keyOf(address.Project, address.Session, address.Key)
		return found, nil
	}
	found.Where.ProjectID = projectID
	found.Address.Project, found.Address.Kind = address.Project, VolumeProject
	if address.Session == "" {
		return found, nil
	}

	handle, err := resolveSession(ctx, client, projectID, address.Session)
	if err != nil {
		if !demotes(address, err) {
			return VolumeLocation{}, err
		}
		found.Address.Key = keyOf(address.Session, address.Key)
		return found, nil
	}
	found.Where.SessionID = handle
	found.Address.Session, found.Address.Kind = address.Session, VolumeWorking
	return found, nil
}

// demotes says whether a level that did not resolve was a guess rather than a fact.
//
// Only a not found demotes. A name that belongs to two projects is a question for the operator, and
// reading it as a file instead would answer a different question quietly.
func demotes(address VolumePath, err error) bool {
	return !address.Settled() && errors.Is(err, ErrNotFound)
}

// keyOf joins the segments that turned out to be a file name onto the key under them.
//
// Nothing joined here can climb: a segment of . or .. never fills a level in the first place, and the
// key under them was cleaned when it was read. Whatever holds a key inside its directory does so at
// the point it becomes a path on disk, which is not here.
func keyOf(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, Separator)
}
