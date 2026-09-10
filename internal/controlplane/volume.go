package controlplane

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/workspace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// One file in a volume: its bytes in and out, and the names a directory holds.
//
// The problem these answer: a volume is a directory on the machine that runs the sandboxes. Where the
// tool runs on that machine it opens the directory itself. Under Kubernetes it does not, the directory
// is beside the control plane, and a path the tool is handed reaches nothing at all. So the work is
// done here, and the tool works the same way in both places.
//
// The two that carry bytes stream, because a message has a ceiling. The default is four mebibytes.
// The file that started this work is 1,105,815 bytes, over the one mebibyte ceiling ReadSessionWork
// holds a file to, and a call that must carry a whole file in one message only moves that ceiling.

// VolumeChunk is how much of a file one message carries.
//
// It is far below the four mebibyte message ceiling, so the framing around a chunk cannot reach it,
// and small enough that neither end holds the file. A larger chunk buys throughput this does not
// need: a copy of one file is not the hot path of anything.
const VolumeChunk = 64 << 10

// volumeFileMode is the mode a file takes in a volume. A session reads it from a container as
// somebody else, so its owner alone is not enough.
const volumeFileMode = 0o666

// PutVolumeFile writes one file into a volume, from the pieces the caller sends.
//
// The first message says where the file goes. Every message after it is bytes, in order. The caller
// works in addresses and never in paths, so every decision about the filesystem is made here: which
// name the file takes, whether that name is already there, and where the bytes land.
func (s *Server) PutVolumeFile(stream quaycrewv1.ControlPlaneService_PutVolumeFileServer) error {
	first, err := stream.Recv()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return status.Error(codes.InvalidArgument,
				"a copy into a volume begins by saying where the file goes, and this one carried nothing")
		}
		return err
	}
	start := first.GetStart()
	if start == nil {
		return status.Error(codes.InvalidArgument,
			"a copy into a volume begins by saying where the file goes, and this one began with bytes")
	}
	found, err := s.directoryFor(stream.Context(), start.GetWorkspace(), start.GetProject(), start.GetSession())
	if err != nil {
		return err
	}

	key := volumeKeyFor(found.Dir, start.GetKey(), start.GetName())
	inside := workspace.InVolume(found.Dir, key)
	switch _, err := os.Stat(inside); {
	case err == nil && !start.GetReplace():
		// The address rather than the path, because the caller typed the address and a refusal that
		// named a directory of identifiers names a thing they never saw.
		return status.Errorf(codes.AlreadyExists, "%s is already there", addressOf(start.GetAddress(), key))
	case err != nil && !os.IsNotExist(err):
		return status.Errorf(codes.Internal, "%v", err)
	}
	if err := writeVolumeFile(inside, &volumeChunks{stream: stream}); err != nil {
		return err
	}
	return stream.SendAndClose(&quaycrewv1.PutVolumeFileResponse{Sandbox: path.Join(found.Sandbox, key)})
}

// GetVolumeFile reads one file out of a volume, in pieces.
//
// A file of no bytes sends no message, which is the whole of an empty file. A folder is refused: a
// whole folder is out of scope, and sending the first file in one would lose the rest without saying
// so.
func (s *Server) GetVolumeFile(req *quaycrewv1.GetVolumeFileRequest, stream quaycrewv1.ControlPlaneService_GetVolumeFileServer) error {
	found, err := s.directoryFor(stream.Context(), req.GetWorkspace(), req.GetProject(), req.GetSession())
	if err != nil {
		return err
	}
	key := workspace.HeldKey(req.GetKey())
	if key == "" {
		return status.Errorf(codes.InvalidArgument,
			"%s is a volume, and this reads one file: name a file in it", found.Host)
	}
	inside := workspace.InVolume(found.Dir, key)
	info, err := os.Stat(inside)
	if err != nil {
		if os.IsNotExist(err) {
			return status.Errorf(codes.NotFound, "%s holds nothing called %q", found.Host, key)
		}
		return status.Errorf(codes.Internal, "%v", err)
	}
	if info.IsDir() {
		return status.Errorf(codes.FailedPrecondition,
			"%s is a folder, and this copies one file: name a file in it", path.Join(found.Host, key))
	}

	file, err := os.Open(inside)
	if err != nil {
		return status.Errorf(codes.Internal, "%v", err)
	}
	defer func() { _ = file.Close() }()
	carry := make([]byte, VolumeChunk)
	for {
		read, err := file.Read(carry)
		if read > 0 {
			if err := stream.Send(&quaycrewv1.GetVolumeFileResponse{Chunk: carry[:read]}); err != nil {
				return err
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return status.Errorf(codes.Internal, "read %s: %v", key, err)
		}
	}
}

// ListVolume says what one directory in a volume holds.
//
// It reads the directory this process holds, for the reason the two calls above carry bytes. The tool
// asked where the volume was and opened that path itself, which reaches nothing where the volume is
// beside the control plane rather than beside the tool. The answer then was that the directory held
// nothing, and an empty volume and an unreachable one read the same.
//
// The names come back sorted, which is os.ReadDir's own order, so one directory read twice answers
// the same. A key that names a file answers with that one file, so no entries at all is an empty
// directory and nothing else.
func (s *Server) ListVolume(ctx context.Context, req *quaycrewv1.ListVolumeRequest) (*quaycrewv1.ListVolumeResponse, error) {
	found, err := s.directoryFor(ctx, req.GetWorkspace(), req.GetProject(), req.GetSession())
	if err != nil {
		return nil, err
	}
	// This is the point of use, and the caller can no longer be the one that holds a key inside the
	// volume: it never sees the directory. So `../../etc/passwd` is cleaned onto the root here, and
	// lands inside the volume or it lands nowhere.
	key := workspace.HeldKey(req.GetKey())
	inside := workspace.InVolume(found.Dir, key)
	info, err := os.Stat(inside)
	if err != nil {
		if os.IsNotExist(err) {
			// The directory rather than the path with the key on it, because a name that is missing
			// and a name in another folder read the same. It is the host's path: the person reading
			// the refusal is standing at that machine and not at this one.
			return nil, status.Errorf(codes.NotFound, "%s holds nothing called %q", found.Host, key)
		}
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	answer := &quaycrewv1.ListVolumeResponse{Host: workspace.InVolume(found.Host, key)}
	if !info.IsDir() {
		answer.Entries = []*quaycrewv1.VolumeEntry{{Name: info.Name(), Size: info.Size()}}
		return answer, nil
	}
	entries, err := os.ReadDir(inside)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	answer.Entries = make([]*quaycrewv1.VolumeEntry, 0, len(entries))
	for _, entry := range entries {
		answer.Entries = append(answer.Entries, volumeEntry(entry))
	}
	return answer, nil
}

// volumeEntry is one name in a directory, as the listing reads it.
//
// A folder carries no size. Its size on disk is the size of its own record rather than of what is in
// it, so answering with one answers a question nobody asked. An entry whose size cannot be read is
// answered without one for the same reason: the name is what the caller asked for, and a second
// failure inside a listing hides the names that did read.
func volumeEntry(entry os.DirEntry) *quaycrewv1.VolumeEntry {
	held := &quaycrewv1.VolumeEntry{Name: entry.Name(), Directory: entry.IsDir()}
	if held.GetDirectory() {
		return held
	}
	if info, err := entry.Info(); err == nil {
		held.Size = info.Size()
	}
	return held
}

// volumeChunks reads the pieces of a put as an ordinary reader, so the write below is one copy.
type volumeChunks struct {
	stream quaycrewv1.ControlPlaneService_PutVolumeFileServer
	held   []byte
}

// Read hands over what the last message carried, and asks for the next one when that runs out.
//
// A second start message is refused. A stream that names one destination and then another is not a
// copy of one file, and taking the first name and writing the second caller's bytes under it is the
// worst answer available.
func (v *volumeChunks) Read(into []byte) (int, error) {
	for len(v.held) == 0 {
		message, err := v.stream.Recv()
		if err != nil {
			return 0, err
		}
		if message.GetStart() != nil {
			return 0, status.Error(codes.InvalidArgument,
				"a copy into a volume says where the file goes once, and this one said it twice")
		}
		v.held = message.GetChunk()
	}
	read := copy(into, v.held)
	v.held = v.held[read:]
	return read, nil
}

// volumeKeyFor is the name the file lands under, held inside the volume.
//
// A key that names a folder keeps the file's own name inside it, the way copying a file into a folder
// does everywhere else. An empty key is the directory the address itself named, which is the same
// case. `krewe volume cp ./log.txt krewe://itv/vast` writes log.txt into the vast folder.
func volumeKeyFor(root, key, name string) string {
	held := workspace.HeldKey(key)
	if info, err := os.Stat(workspace.InVolume(root, held)); err == nil && info.IsDir() {
		return workspace.HeldKey(path.Join(held, name))
	}
	return held
}

// addressOf is the file named the way the person who typed it reads it: the directory they addressed,
// and the name the file took inside it.
func addressOf(directory, key string) string {
	if directory == "" {
		return key
	}
	return directory + workspace.Separator + key
}

// writeVolumeFile puts the bytes there in one move.
//
// They go to a temporary name in the same directory. The rename onto the real one is one operation,
// so a session reading the folder sees the whole file or no file at all. A file of a megabyte takes
// long enough to arrive that a session can read half of one, and half a file reads as a broken file
// rather than as a copy still under way.
//
// The mode is set after the write. A temporary file is readable by its owner alone, and a session in
// a container is somebody else, so a file left that way is a file that did not arrive.
func writeVolumeFile(at string, body io.Reader) error {
	partial, err := os.CreateTemp(filepath.Dir(at), "."+filepath.Base(at)+".")
	if err != nil {
		return status.Errorf(codes.Internal, "%v", err)
	}
	defer func() { _ = os.Remove(partial.Name()) }()
	if _, err := io.Copy(partial, body); err != nil {
		_ = partial.Close()
		return err
	}
	if err := partial.Close(); err != nil {
		return status.Errorf(codes.Internal, "%v", err)
	}
	if err := os.Chmod(partial.Name(), volumeFileMode); err != nil {
		return status.Errorf(codes.Internal, "%v", err)
	}
	if err := os.Rename(partial.Name(), at); err != nil {
		return status.Errorf(codes.Internal, "%v", err)
	}
	return nil
}
