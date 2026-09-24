package toolutil

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

const (
	// partialDownloadPrefix and partialDownloadSuffix frame the name a
	// download is written under until it is complete. The name carries
	// nothing of the destination's, so it is the same length whatever the
	// caller named and cannot push a path component past the filesystem's
	// limit; the prefix is what identifies a leftover that a process killed
	// mid-download had no chance to remove.
	partialDownloadPrefix = ".gitlab-mcp-server-download-"
	partialDownloadSuffix = ".partial"
)

// mkdirAll is [os.MkdirAll], replaceable in tests. What happens between the
// directory being made and the file being created in it is a race with
// another local principal, and this is the one point a test can stage it.
var mkdirAll = os.MkdirAll

// syncDownloadDirectory is [syncDirectory], replaceable in tests: no
// directory a test can make refuses to be synced, and a download has to be
// seen reporting success when one does.
var syncDownloadDirectory = syncDirectory

// WriteDownloadOutputFile writes a download to the destination a caller
// named, and only once write has produced all of it. It returns the number of
// bytes written.
//
// The destination is resolved and confined by [canonicalDownloadOutputPath]
// before anything is created, so one outside the allowed roots leaves no
// directory behind either. It is resolved again once its directory exists:
// the first pass could only vouch for the ancestors that were there at the
// time, and a symlink planted under a directory this call just made would
// otherwise decide where the bytes land.
//
// write receives a temporary file in the destination's directory, never the
// destination. When write returns nil the file is synced, closed and renamed
// over the destination, and on Unix the directory is synced after the
// rename; when write returns an error, or the file cannot be put in place,
// the temporary file is removed and the error returned, so the destination
// holds exactly what it held before, or is still absent. That is the point
// of the indirection: a download cut off by an error answer, a dropped
// connection or a cancelled call used to leave an empty or truncated file at
// the destination, which the next reader took for the download. The
// directories made on the way stay, since they are empty and a retry needs
// them. A process killed outright leaves its temporary file, recognizable by
// [partialDownloadPrefix], and still never a partial destination.
//
// The rename is also what closes the race [canonicalDownloadOutputPath]
// cannot: it resolves a symlink already at the destination and confines what
// that link names, but a local principal who can write in an allowed root
// could plant one after the check. A rename replaces the directory entry at
// the destination and does not follow it, so a symlink planted there is
// replaced rather than written through, on every platform. On Unix the
// replacement is one rename(2) and atomic. On Windows it is MoveFileEx with
// MOVEFILE_REPLACE_EXISTING, which Go does not promise to be atomic and which
// fails while another process holds the destination open without sharing its
// deletion, a running program for one, or when the destination is marked
// read-only; it then keeps its previous content and the error says so.
//
// A replaced destination is a new file, and a hard link to the old file keeps
// the old content. On Unix the new file is readable and writable by the owner
// alone (created with mode 0600), like every file this creates, whatever the
// file it replaces allowed; on Windows it takes the access the directory
// grants to new files, not the old file's own ACL, since Go creates it with
// no security descriptor of its own and the rename carries the one it
// inherited. On Unix, whether it may be replaced at all is decided by the
// directory's permissions, as for any rename, rather than by the old file's
// own.
func WriteDownloadOutputFile(path string, write func(io.Writer) error) (_ int64, err error) {
	out, err := createPartialDownload(path)
	if err != nil {
		return 0, err
	}
	// Deferred rather than called on each error path, so a write that panics
	// takes its temporary file with it as well.
	defer func() {
		if !out.committed {
			err = out.discard(err)
		}
	}()

	if writeErr := write(out); writeErr != nil {
		return 0, writeErr
	}
	return out.commit()
}

// createPartialDownload confines the caller's destination, makes its
// directory and opens the temporary file a download is written to.
func createPartialDownload(path string) (*partialDownload, error) {
	destination, err := canonicalDownloadOutputPath(path)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(destination)
	if err = mkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create output directory %s: %w", dir, err)
	}
	destination, err = canonicalDownloadOutputPath(destination)
	if err != nil {
		return nil, err
	}

	// The name is random, so nobody can plant anything at it ahead of time,
	// and it is created exclusively, so anything planted there anyway is
	// refused rather than opened.
	name := filepath.Join(filepath.Dir(destination), partialDownloadPrefix+rand.Text()+partialDownloadSuffix)
	file, err := createNewLeafNoFollow(name)
	if err != nil {
		return nil, fmt.Errorf("create output file %s: %w", name, err)
	}
	return &partialDownload{file: file, destination: destination}, nil
}

// partialDownload is a download on its way to its destination: the open
// temporary file, the destination it will be renamed over, and the bytes
// written so far.
type partialDownload struct {
	file        *os.File
	destination string
	size        int64
	committed   bool
}

// Write appends p to the temporary file, counting what was written so the
// size is known without asking the filesystem for it.
func (d *partialDownload) Write(p []byte) (int, error) {
	n, err := d.file.Write(p)
	d.size += int64(n)
	return n, err
}

// commit puts the temporary file in place of the destination.
//
// The file is synced and then closed, both always: the sync is what makes the
// bytes durable before any name points at them, and Windows refuses to
// rename a file that is still open.
//
// On Unix the directory is synced after the rename, because syncing the file
// makes its bytes durable and says nothing about the name now pointing at
// them: until the directory reaches the disk, a crash can undo the rename and
// leave the file it replaced, or no file at all, after the tool has reported
// the new one with its hash. A directory that cannot be synced does not fail
// the download, since the file is already in place and correct and some
// filesystems refuse to sync a directory at all; it is logged, as the one
// sign that the replacement may not survive a crash.
func (d *partialDownload) commit() (int64, error) {
	if err := errors.Join(d.file.Sync(), d.file.Close()); err != nil {
		return 0, fmt.Errorf("write output file %s: %w", d.destination, err)
	}
	if err := os.Rename(d.file.Name(), d.destination); err != nil {
		return 0, fmt.Errorf("move the download into place at %s: %w", d.destination, err)
	}
	d.committed = true
	dir := filepath.Dir(d.destination)
	if err := syncDownloadDirectory(dir); err != nil {
		slog.Debug("download is in place but its directory could not be synced, so a crash soon after may bring back the file it replaced",
			"directory", dir, "error", err)
	}
	return d.size, nil
}

// discard removes the temporary file and returns cause, joined with the
// removal's own error when that failed. A failed removal is the one outcome
// that leaves something behind, so it is the one a caller has to hear about;
// a file already gone is not one.
func (d *partialDownload) discard(cause error) error {
	_ = d.file.Close() // already closed when commit got as far as the rename, and nothing written here is kept either way
	if err := os.Remove(d.file.Name()); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return errors.Join(cause, fmt.Errorf("remove the partial download: %w", err))
	}
	return cause
}
