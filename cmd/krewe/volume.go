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
	"strings"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/display"
	"github.com/atlantic-blue/quay-krewe/internal/workspace"
)

// The verb for the files a volume holds.
//
// The problem it answers: a volume is the directory a session reads, and every level of it on disk is
// a generated identifier. `krewe where` names the directory and stops there. `krewe read` answers for
// a session and for nothing else. So nothing said what a workspace's shared folder held, and nothing
// put a file in one.
//
// The bytes are read on the machine this runs on. The tool and the volume are on one machine today,
// so the path the system hands back is a path this process can open. Where they are not, this call
// becomes a stream through the control plane. The address a person types does not change.

// volumeUsage names the address form, because the scheme is the part nobody guesses.
const volumeUsage = "usage: krewe volume list <address>" +
	"\n       krewe volume cp <file> <address> [" + flagReplace + "]" +
	"\n\nan address is krewe://<workspace>[/<project>[/<session>]], and a name after that is a file in it" +
	"\n\n  krewe volume list krewe://itv/vast" +
	"\n  krewe volume cp ./explore.txt krewe://itv/vast"

// flagReplace says the caller means to write over a name that is already there.
const flagReplace = "--replace"

func runVolume(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(volumeUsage)
	}
	switch args[0] {
	case "list":
		return runVolumeList(ctx, client, args[1:], out)
	case "cp":
		return runVolumeCopy(ctx, hostVolume{client: client}, client, args[1:], out)
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

// volumeTransport carries bytes to a volume.
//
// It is an interface because of where the bytes are. The tool and the sandboxes run on one machine
// today, so a copy is a copy on that machine. Under Kubernetes they do not, and the same command has
// to become a stream through the control plane. Everything above this works in addresses and never in
// host paths, so that change replaces one implementation and alters nothing a person types.
type volumeTransport interface {
	// Put writes body at the address, and answers with the path a session reads the file at.
	//
	// The name is what the file is called on the machine it came from. It is used where the address
	// names a directory rather than a file, which is how `cp ./log.txt krewe://itv/vast` keeps the
	// name it had.
	//
	// A replace of false refuses a name that is already there. A copy that quietly writes over the
	// last one is how the work in it is lost.
	Put(ctx context.Context, to workspace.VolumeLocation, name string, body io.Reader, replace bool) (string, error)
}

// runVolumeCopy puts a file on this machine in front of every session that reads the address.
//
// It prints one path and nothing else: the path a session reads the file at. A person types that
// path into the message they send the session. So it goes on its own line, the way `krewe where`
// prints a directory.
func runVolumeCopy(ctx context.Context, carry volumeTransport, client quaycrewv1.ControlPlaneServiceClient, args []string, out io.Writer) error {
	replace := false
	named := make([]string, 0, 2)
	for _, arg := range args {
		if arg == flagReplace {
			replace = true
			continue
		}
		named = append(named, arg)
	}
	if len(named) != 2 {
		return errors.New(volumeUsage)
	}
	source, destination := named[0], named[1]

	// The scheme is what tells the two arguments apart. `itv/vast` is a good relative path and a good
	// address, so without it nothing here can say which of the two somebody meant.
	if strings.HasPrefix(source, workspace.Scheme) {
		return fmt.Errorf("this copies a file on this machine into a volume, and %q is an address"+
			"\n\n%s", source, volumeUsage)
	}
	if !strings.HasPrefix(destination, workspace.Scheme) {
		return fmt.Errorf("%q is a path on this machine, and a copy needs an address to put the file at"+
			"\n\n%s", destination, volumeUsage)
	}

	address, err := workspace.ParseVolumePath(destination)
	if err != nil {
		return err
	}
	found, err := workspace.ResolveVolume(ctx, client, address)
	if err != nil {
		return err
	}

	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	// A whole directory is out of scope. A copy that took the first file in one would be worse than a
	// refusal, because nobody would know the other files never went.
	if info.IsDir() {
		return fmt.Errorf("%s is a directory, and this copies one file: name a file in it", source)
	}

	at, err := carry.Put(ctx, found, filepath.Base(source), file, replace)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, at)
	return nil
}

// hostVolume is the transport for a volume on the machine this runs on.
//
// It asks the control plane where the address is and copies the bytes there itself. That works
// because the two are one machine. Where they are not, the implementation behind the interface
// changes and this file does not.
type hostVolume struct {
	client quaycrewv1.ControlPlaneServiceClient
}

func (h hostVolume) Put(ctx context.Context, to workspace.VolumeLocation, name string, body io.Reader, replace bool) (string, error) {
	found, err := h.client.LocateDirectory(ctx, &quaycrewv1.LocateDirectoryRequest{
		Workspace: to.Where.WorkspaceID,
		Project:   to.Where.ProjectID,
		Session:   to.Where.SessionID,
	})
	if err != nil {
		return "", err
	}
	key := volumeKeyFor(found.GetHost(), to.Address.Key, name)
	inside := inVolume(found.GetHost(), key)

	switch _, err := os.Stat(inside); {
	case err == nil && !replace:
		taken := to.Address
		taken.Key = key
		return "", fmt.Errorf("%s is already there: say %s to write over it", taken, flagReplace)
	case err != nil && !os.IsNotExist(err):
		return "", err
	}
	if err := writeWholeFile(inside, body); err != nil {
		return "", err
	}
	return path.Join(found.GetSandbox(), key), nil
}

// volumeKeyFor is the name the file lands under, held inside the volume.
//
// A key that names a directory keeps the file's own name inside it, the way copying a file into a
// folder does everywhere else. An empty key is the directory the address itself named, which is the
// same case. `krewe volume cp ./log.txt krewe://itv/vast` writes log.txt into the vast folder.
//
// It answers with the held key rather than with the one it was given, because the caller prints where
// the file went. A key that climbs, written inside the volume and reported at the path it asked for,
// names a file that is not there.
func volumeKeyFor(root, key, name string) string {
	held := volumeKey(key)
	if info, err := os.Stat(inVolume(root, held)); err == nil && info.IsDir() {
		return volumeKey(path.Join(held, name))
	}
	return held
}

// writeWholeFile puts the bytes there in one move.
//
// They go to a temporary name in the same directory. The rename onto the real one is one operation,
// so a session reading the folder sees the whole file or no file at all. A file of a megabyte takes
// long enough to copy that a session can read half of one. Half a file reads as a broken file rather
// than as a copy still under way.
//
// The mode is set after the write. A temporary file is made readable by its owner alone, and the
// session that has to read this runs in a container as somebody else.
func writeWholeFile(at string, body io.Reader) error {
	partial, err := os.CreateTemp(filepath.Dir(at), "."+filepath.Base(at)+".")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(partial.Name()) }()
	if _, err := io.Copy(partial, body); err != nil {
		_ = partial.Close()
		return err
	}
	if err := partial.Close(); err != nil {
		return err
	}
	if err := os.Chmod(partial.Name(), 0o666); err != nil {
		return err
	}
	return os.Rename(partial.Name(), at)
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

// inVolume is where a key becomes a path on disk. This is the point of use, and a key that came from
// anywhere else is held here.
func inVolume(root, key string) string {
	return filepath.Join(root, filepath.FromSlash(volumeKey(key)))
}

// volumeKey holds a key inside the directory it names.
//
// The leading slash makes a key that climbs harmless. It resolves against the root of nothing, so
// ../../etc/passwd becomes /etc/passwd, and then a name inside the volume. The parser cleans a key it
// read the same way, and this is the guard under it.
func volumeKey(key string) string {
	return strings.TrimPrefix(path.Clean(workspace.Separator+key), workspace.Separator)
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
