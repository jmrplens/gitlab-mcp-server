//go:build unix

// file_utils_unix.go opens caller-supplied local paths without following a
// symlink at the leaf, which is the containment the surrounding checks in
// file_utils.go describe but cannot enforce on their own.

package toolutil

import (
	"os"
	"syscall"
)

// openLeafNoFollow opens path for reading and refuses a symlink at the leaf.
//
// The Lstat that precedes it proves what the path named a moment ago, not what
// os.Open would open now: os.Open follows symlinks, so a local principal able
// to write in an allowed root (the OS temp directory is always one, and /tmp is
// world-writable) can swap the leaf between the two syscalls and redirect the
// read to any file this process can read. O_NOFOLLOW moves the refusal into the
// open itself, where the kernel resolves the last component, so the check and
// the use are one operation.
func openLeafNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0) // the caller resolves the path through symlinks and confines it to the allowed directories
}

// createNewLeafNoFollow creates path for writing, readable by the owner alone,
// and refuses anything already there, a symlink at the leaf included.
//
// O_EXCL alone refuses a symlink whatever it points at, which POSIX
// specifies; O_NOFOLLOW is kept beside it so the refusal does not rest on one
// flag's reading of the standard. Nothing is ever truncated here: a download
// replaces its destination by renaming a file this created over it.
func createNewLeafNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600) // the caller resolves the directory through symlinks and confines it to the allowed directories
}
