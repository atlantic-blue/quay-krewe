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
// put a file in one or took one out.
//
// The bytes are read on the machine this runs on. The tool and the volume are on one machine today,
// so the path the system hands back is a path this process can open. Where they are not, this call
// becomes a stream through the control plane. The address a person types does not change.

// volumeUsage names the address form, because the scheme is the part nobody guesses.
const volumeUsage = "usage: krewe volume list <address>" +
	"\n       krewe volume cp <file> <address> [" + flagReplace + "]" +
	"\n       krewe volume cp <address> <file> [" + flagReplace + "]" +
	"\n       krewe volume delete <address>" +
	"\n\nan address is krewe://<workspace>[/<project>[/<session>]], and a name after that is a file in it" +
	"\n\n  krewe volume list krewe://itv/vast" +
	"\n  krewe volume cp ./explore.txt krewe://itv/vast" +
	"\n  krewe volume cp krewe://itv/vast/explore.txt ~/Downloads" +
	"\n  krewe volume delete krewe://itv/vast/explore.txt"

// flagReplace says the caller means to write over a name that is already there.
const flagReplace = "--replace"

// volumeFileMode is the mode a file takes in a volume. A session reads it from a container as
// somebody else, so its owner alone is not enough.
const volumeFileMode = 0o666

// machineFileMode is the mode a file takes on the machine the tool runs on. The person who typed the
// command reads it, so it is the ordinary mode of a file rather than the one a container needs.
const machineFileMode = 0o644

func runVolume(ctx context.Context, client quaycrewv1.ControlPlaneServiceClient, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(volumeUsage)
	}
	switch args[0] {
	case "list":
		return runVolumeList(ctx, client, args[1:], out)
	case "cp":
		return runVolumeCopy(ctx, hostVolume{client: client}, client, args[1:], out)
	case "delete":
		return runVolumeDelete(ctx, hostVolume{client: client}, client, args[1:], out)
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

// volumeTransport carries bytes to a volume and back from one.
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

	// Get reads the file the address names. The caller closes what comes back.
	//
	// A reader rather than the bytes themselves, because step 11 makes this a stream: a file above the
	// message ceiling arrives in pieces, and holding a whole one in memory to hand it over would put
	// the ceiling back in a different place.
	//
	// An address that names a directory is refused here. A whole directory is out of scope, and
	// copying the first file in one loses the rest without saying so.
	Get(ctx context.Context, from workspace.VolumeLocation) (io.ReadCloser, error)

	// Delete removes the file the address names, and answers with the path it was at.
	//
	// One file. A directory is refused, because a recursive delete is out of scope and there is no
	// way back from one. A name that is not there is refused too, and the refusal says what the
	// directory does hold.
	Delete(ctx context.Context, at workspace.VolumeLocation) (string, error)
}

// runVolumeDelete removes one file from a volume, so a volume does not only ever grow.
//
// It prints the path the file was at, on its own line. The levels of an address are read against what
// the system holds, so `krewe://itv/notes` is a project or a file in the shared folder. A delete that
// printed nothing would leave the person to work out which of the two went.
func runVolumeDelete(ctx context.Context, carry volumeTransport, client quaycrewv1.ControlPlaneServiceClient, args []string, out io.Writer) error {
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
	// An address with no name on the end of it is the volume itself. Emptying one is out of scope, and
	// a delete of everything in a folder is not something a person can take back.
	if !found.Address.HasKey() {
		return fmt.Errorf("%s is a volume, and this deletes one file: name a file in it", found.Address)
	}
	at, err := carry.Delete(ctx, found)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, at)
	return nil
}

// runVolumeCopy carries one file between this machine and a volume.
//
// The scheme is what tells the two arguments apart. `itv/vast` is a good relative path and a good
// address, so without it nothing here can say which of the two somebody meant. One argument of each
// kind is a direction. Two of a kind is neither, and both ways round are refused.
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

	fromVolume := strings.HasPrefix(source, workspace.Scheme)
	toVolume := strings.HasPrefix(destination, workspace.Scheme)
	switch {
	case fromVolume && toVolume:
		return fmt.Errorf("%q and %q are both addresses, and this copies between a volume and this machine:"+
			" name a path on this machine as one of the two\n\n%s", source, destination, volumeUsage)
	case !fromVolume && !toVolume:
		return fmt.Errorf("%q and %q are both paths on this machine, and a copy needs an address to say"+
			" which volume\n\n%s", source, destination, volumeUsage)
	case fromVolume:
		return copyOutOfVolume(ctx, carry, client, source, destination, replace, out)
	default:
		return copyIntoVolume(ctx, carry, client, source, destination, replace, out)
	}
}

// copyIntoVolume puts a file on this machine in front of every session that reads the address.
//
// It prints one path and nothing else: the path a session reads the file at. A person types that
// path into the message they send the session. So it goes on its own line, the way `krewe where`
// prints a directory.
func copyIntoVolume(ctx context.Context, carry volumeTransport, client quaycrewv1.ControlPlaneServiceClient, source, destination string, replace bool, out io.Writer) error {
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

// copyOutOfVolume brings a file a session wrote back to this machine.
//
// It prints one path and nothing else, the way the other direction does: the path on this machine the
// file landed at. That path goes straight into whatever reads the file next.
func copyOutOfVolume(ctx context.Context, carry volumeTransport, client quaycrewv1.ControlPlaneServiceClient, source, destination string, replace bool, out io.Writer) error {
	address, err := workspace.ParseVolumePath(source)
	if err != nil {
		return err
	}
	found, err := workspace.ResolveVolume(ctx, client, address)
	if err != nil {
		return err
	}
	// An address with no name on the end of it is the directory itself, and a whole directory is out
	// of scope. Taking the first file in one would lose the rest without saying so.
	if !found.Address.HasKey() {
		return fmt.Errorf("%s is a volume, and this copies one file: name a file in it", found.Address)
	}

	at := onThisMachine(destination, path.Base(found.Address.Key))
	// The refusal comes before the read, so a copy that was never going to be allowed does not carry a
	// megabyte first. What it must not do is check here and write later without checking again, so the
	// write below is the one that holds the file.
	if err := refuseNameAlreadyThere(at, replace); err != nil {
		return err
	}

	body, err := carry.Get(ctx, found)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()
	if err := refuseNameAlreadyThere(at, replace); err != nil {
		return err
	}
	if err := writeWholeFile(at, body, machineFileMode); err != nil {
		return err
	}
	fmt.Fprintln(out, at)
	return nil
}

// onThisMachine is the path the file lands at.
//
// A destination that is a directory keeps the file's own name inside it, the way copying a file into
// a folder does everywhere else. Anything else is the name the file takes.
func onThisMachine(destination, name string) string {
	if info, err := os.Stat(destination); err == nil && info.IsDir() {
		return filepath.Join(destination, name)
	}
	return destination
}

// refuseNameAlreadyThere stops a copy destroying the file that is already at that name.
func refuseNameAlreadyThere(at string, replace bool) error {
	if replace {
		return nil
	}
	switch _, err := os.Stat(at); {
	case err == nil:
		return fmt.Errorf("%s is already there: say %s to write over it", at, flagReplace)
	case !os.IsNotExist(err):
		return err
	}
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
	if err := writeWholeFile(inside, body, volumeFileMode); err != nil {
		return "", err
	}
	return path.Join(found.GetSandbox(), key), nil
}

// Get opens the file the address names, on the machine this runs on.
//
// The key is held inside the volume here, the way the write road holds it. This is the point of use,
// and a key that reached the transport from anywhere else is cleaned here or nowhere.
func (h hostVolume) Get(ctx context.Context, from workspace.VolumeLocation) (io.ReadCloser, error) {
	found, err := h.client.LocateDirectory(ctx, &quaycrewv1.LocateDirectoryRequest{
		Workspace: from.Where.WorkspaceID,
		Project:   from.Where.ProjectID,
		Session:   from.Where.SessionID,
	})
	if err != nil {
		return nil, err
	}
	inside := inVolume(found.GetHost(), from.Address.Key)
	info, err := os.Stat(inside)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%s holds nothing called %q", found.GetHost(), volumeKey(from.Address.Key))
		}
		return nil, err
	}
	// A whole directory is out of scope on this road too, and a directory opened as a file reads as an
	// error from the operating system rather than as an answer to what was typed.
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a folder, and this copies one file: name a file in it", from.Address)
	}
	return os.Open(inside)
}

// Delete removes the file the address names, on the machine this runs on.
//
// The key is held inside the volume here, the way both copy roads hold it. This is the point of use,
// and a key that reached the transport from anywhere else is cleaned here or nowhere.
func (h hostVolume) Delete(ctx context.Context, at workspace.VolumeLocation) (string, error) {
	found, err := h.client.LocateDirectory(ctx, &quaycrewv1.LocateDirectoryRequest{
		Workspace: at.Where.WorkspaceID,
		Project:   at.Where.ProjectID,
		Session:   at.Where.SessionID,
	})
	if err != nil {
		return "", err
	}
	key := volumeKey(at.Address.Key)
	inside := inVolume(found.GetHost(), key)
	info, err := os.Stat(inside)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nothingCalledThat(filepath.Dir(inside), path.Base(key))
		}
		return "", err
	}
	// A folder is refused rather than emptied. Removing one takes every file under it, and nothing
	// here brings any of them back.
	if info.IsDir() {
		return "", fmt.Errorf("%s is a folder, and this deletes one file: name a file in it", at.Address)
	}
	if err := os.Remove(inside); err != nil {
		return "", err
	}
	return path.Join(found.GetSandbox(), key), nil
}

// nothingCalledThat is the refusal for a name the directory does not hold.
//
// It names the directory it read and the names in it. A delete is typed from memory, so the file is
// usually there under another name, and a message that only said the name was missing would send
// somebody to list the directory themselves.
func nothingCalledThat(dir, name string) error {
	return fmt.Errorf("%s holds nothing called %q: it holds %s", dir, name, whatItHolds(dir))
}

// whatItHolds is the names in a directory, in the order a listing prints them. A directory it cannot
// read holds nothing as far as the refusal goes: the refusal is about the missing name, and a second
// failure inside the message answers a question nobody asked.
func whatItHolds(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		return "nothing"
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, volumeName(entry))
	}
	return strings.Join(names, ", ")
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
// so anything reading the folder sees the whole file or no file at all. A file of a megabyte takes
// long enough to copy that a session can read half of one. Half a file reads as a broken file rather
// than as a copy still under way.
//
// The mode is set after the write, and the caller says what it is. A temporary file is made readable
// by its owner alone, which is right for neither end of this: a session in a container is somebody
// else, and a file on this machine is shared with the tools the operator runs next.
func writeWholeFile(at string, body io.Reader, mode os.FileMode) error {
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
	if err := os.Chmod(partial.Name(), mode); err != nil {
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
