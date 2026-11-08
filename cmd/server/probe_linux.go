// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"strconv"
)

// peerStdinIsNull reads a peer's file descriptor 0 from procfs, which is what
// its --transport auto read to choose HTTP or stdio.
func peerStdinIsNull(pid int32) (bool, error) {
	return stdinIsNullUnder("/proc", pid)
}

// stdinIsNullUnder reports whether the process's file descriptor 0, as
// published under procRoot (/proc on Linux), is the null device.
//
// It lives in the Linux half because procfs is the only place it reads, and
// no other platform calls it: kept in probe.go, it was a function every
// platform built and only Linux and macOS tested, so its two conditions were
// never evaluated on Windows (issue 1212).
func stdinIsNullUnder(procRoot string, pid int32) (bool, error) {
	link, err := os.Readlink(filepath.Join(procRoot, strconv.Itoa(int(pid)), "fd", "0"))
	if err != nil {
		return false, err
	}
	return link == os.DevNull, nil
}
