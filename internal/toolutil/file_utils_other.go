//go:build !unix

// file_utils_other.go carries the leaf-open primitives for platforms with no
// O_NOFOLLOW. The read primitive relies on the surrounding Lstat and
// post-open Stat checks in file_utils.go alone; the exclusive create needs no
// such caveat, since O_EXCL refuses anything already at the name.

package toolutil

import "os"

// openLeafNoFollow opens path for reading.
//
// Windows has no O_NOFOLLOW, so the leaf swap [openLeafNoFollow] refuses on
// unix is refused here only by the caller's Lstat before the open and its Stat
// on the descriptor after it. Both still run, and both still catch a leaf that
// is not a regular file; what is missing is the guarantee that the file opened
// is the file that was checked.
func openLeafNoFollow(path string) (*os.File, error) {
	return os.Open(path) // the caller resolves the path through symlinks and confines it to the allowed directories
}

// createNewLeafNoFollow creates path for writing and refuses anything already
// there.
//
// Unlike [openLeafNoFollow], this loses nothing on Windows: O_EXCL is
// CREATE_NEW there, which fails on an existing name, a link included, so the
// file created is always a new one this process made.
func createNewLeafNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // the caller resolves the directory through symlinks and confines it to the allowed directories
}
