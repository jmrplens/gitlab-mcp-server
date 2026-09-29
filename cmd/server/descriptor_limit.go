//go:build linux || darwin

// descriptor_limit.go reads the number of file descriptors the process may
// open, which the held-request ceiling is sized from (register row HLD-011).

package main

import "syscall"

// descriptorLimit returns the descriptors this process may open, and whether
// the platform said.
func descriptorLimit() (uint64, bool) {
	return descriptorLimitFrom(syscall.Getrlimit)
}

// descriptorLimitFrom reads the soft RLIMIT_NOFILE through getrlimit.
//
// The soft limit is the one that refuses an open, and by the time anything
// here runs it is no longer the one the process was started with: the Go
// runtime raises it to the hard limit before main (go.dev/issue/46279), so
// what this reads is the limit the process actually works under. A read that
// fails says nothing, and the caller falls back to the register's figure.
func descriptorLimitFrom(getrlimit func(int, *syscall.Rlimit) error) (uint64, bool) {
	var limit syscall.Rlimit
	if err := getrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		return 0, false
	}
	return limit.Cur, true
}
