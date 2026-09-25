//go:build !unix

// download_output_other.go stands in for the directory sync on platforms
// where the os package cannot perform one.

package toolutil

// syncDirectory does nothing here. On Windows, (*os.File).Sync is
// FlushFileBuffers, which needs a handle opened for writing, and the os
// package opens a directory for reading only, so no directory can be flushed
// through it; a rename there is as durable as the filesystem makes its own
// metadata.
func syncDirectory(string) error {
	return nil
}
