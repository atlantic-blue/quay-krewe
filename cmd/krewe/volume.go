package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/display"
	"github.com/atlantic-blue/quay-krewe/internal/workspace"
)

// The verb for the files a volume holds.
//
// The problem it answers: a volume is the directory a session reads, and every level of it on disk is
// a generated identifier. `krewe where` names the directory and stops there. `krewe read` answers for
// a session and for nothing else. So nothing said what a workspace's shared folder held.
//
// The bytes are read on the machine this runs on. The tool and the volume are on one machine today,
// so the path the system hands back is a path this process can open. Where they are not, this call
// becomes a stream through the control plane. The address a person types does not change.

// volumeUsage names the address form, because the scheme is the part nobody guesses.
const volumeUsage = "usage: krewe volume list <address>" +
	"\n\nan address is krewe://<workspace>[/<project>[/<session>]], and a name after that is a file in it" +
	"\n\n  krewe volume list krewe://itv/vast"

func runVolume(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(volumeUsage)
	}
	switch args[0] {
	case "list":
		return runVolumeList(ctx, client, args[1:], out)
	default:
		return fmt.Errorf("there is no volume %s: %s", args[0], volumeUsage)
	}
}

// runVolumeList prints what one directory in a volume holds.
func runVolumeList(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient, args []string, out io.Writer) error {
	if len(args) != 1 {
		return errors.New(volumeUsage)
	}
	address, err := workspace.ParseVolumePath(args[0])
	if err != nil {
		return err
	}
	found, err := workspace.ResolveVolume(ctx, client, address)
	if err != nil {
		return err
	}
	resp, err := client.LocateDirectory(ctx, &quaycrewv1.LocateDirectoryRequest{
		Workspace: found.Where.WorkspaceID,
		Project:   found.Where.ProjectID,
		Session:   found.Where.SessionID,
	})
	if err != nil {
		return err
	}
	return listVolume(resp.GetHost(), found.Address.Key, out)
}

// listVolume prints the directory the address landed on, then what is in it.
//
// The path goes first, on its own line. A listing is only useful beside the directory it read: a name
// that is missing and a name in another folder read the same otherwise.
func listVolume(root, key string, out io.Writer) error {
	inside := inVolume(root, key)
	info, err := os.Stat(inside)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s holds nothing called %q", root, key)
		}
		return err
	}
	fmt.Fprintln(out, inside)
	// A key that names a file lists that file, the way listing an object by its whole name does.
	// Refusing it would refuse the one command somebody types to check a copy arrived.
	if !info.IsDir() {
		fmt.Fprint(out, volumeRows([]os.DirEntry{fs.FileInfoToDirEntry(info)}))
		return nil
	}
	entries, err := os.ReadDir(inside)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprintln(out, "nothing in it")
		return nil
	}
	fmt.Fprint(out, volumeRows(entries))
	return nil
}

// inVolume is where a key becomes a path on disk, and where a key that climbs is held.
//
// The leading slash makes such a key harmless. It resolves against the root of nothing, so
// ../../etc/passwd becomes /etc/passwd, and then a name inside the volume. The parser cleans a key it
// read the same way. This is the point of use, and a key that came from anywhere else is held here.
func inVolume(root, key string) string {
	return filepath.Join(root, filepath.FromSlash(path.Clean(workspace.Separator+key)))
}

// volumeRows is the table. os.ReadDir hands its entries back sorted by name, and that order is the
// contract here: one directory read twice answers the same.
func volumeRows(entries []os.DirEntry) string {
	rows := make([][]string, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, []string{volumeName(entry), volumeSize(entry)})
	}
	return display.Rows([]string{"NAME", "SIZE"}, rows)
}

// volumeName marks a folder with a trailing slash, the way every listing of files does, so a name
// that could be either is not ambiguous.
func volumeName(entry os.DirEntry) string {
	if entry.IsDir() {
		return entry.Name() + "/"
	}
	return entry.Name()
}

// volumeSize is how big a file is, and empty for a folder. A folder's size on disk is the size of its
// own record rather than of what is in it, so printing one answers a question nobody asked.
func volumeSize(entry os.DirEntry) string {
	if entry.IsDir() {
		return ""
	}
	info, err := entry.Info()
	if err != nil {
		return ""
	}
	return strconv.FormatInt(info.Size(), 10)
}
