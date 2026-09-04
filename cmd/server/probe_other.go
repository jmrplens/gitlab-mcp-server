//go:build !linux

package main

import "errors"

// peerStdinIsNull cannot see another process's file descriptors without
// procfs. The probe then assumes HTTP and lets the connection decide; a
// deployment on this platform gives --probe its target outright.
func peerStdinIsNull(int32) (bool, error) {
	return false, errors.ErrUnsupported
}
