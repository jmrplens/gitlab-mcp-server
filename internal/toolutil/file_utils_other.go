//go:build !unix

// file_utils_other.go carries the leaf-open primitives for platforms with no
// O_NOFOLLOW, where the surrounding Lstat and post-open Stat checks in
// file_utils.go are all the containment there is.

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
	return os.Open(path) //#nosec G304 -- the caller resolves the path through symlinks and confines it to the allowed directories
}

// createLeafNoFollow creates or truncates path for writing, with the same
// caveat [openLeafNoFollow] carries on this platform.
func createLeafNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //#nosec G304 -- the caller resolves the path through symlinks and confines it to the allowed directories
}
