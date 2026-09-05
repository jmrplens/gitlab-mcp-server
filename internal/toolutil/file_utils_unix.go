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
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0) //#nosec G304 -- the caller resolves the path through symlinks and confines it to the allowed directories
}

// createLeafNoFollow creates or truncates path for writing and refuses a
// symlink at the leaf, for the same reason [openLeafNoFollow] refuses one on
// the way in.
//
// Truncating an existing regular file is still allowed: overwriting a
// destination the caller named is what a download does.
func createLeafNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, 0o600) //#nosec G304 -- the caller resolves the path through symlinks and confines it to the allowed directories
}
