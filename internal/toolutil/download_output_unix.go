//go:build unix

// download_output_unix.go syncs the directory a download was renamed into,
// which is what makes the rename itself survive a crash on these platforms.

package toolutil

import (
	"errors"
	"os"
)

// syncDirectory flushes dir to stable storage.
//
// POSIX gives fsync on a file no say over the directory entry that names it,
// so a file synced and then renamed can reach the disk while the rename does
// not; syncing the directory is the only way to make the new name durable.
func syncDirectory(dir string) error {
	d, err := os.Open(dir) // the caller's destination directory, resolved and confined before anything was written into it
	if err != nil {
		return err
	}
	return errors.Join(d.Sync(), d.Close())
}
