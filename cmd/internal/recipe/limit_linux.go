// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package recipe

import (
	"os/exec"
	"syscall"
)

// getrlimit and setrlimit are the calls the limit is read and set with,
// behind a seam so a test can have either refused.
var (
	getrlimit = syscall.Getrlimit
	setrlimit = syscall.Setrlimit
)

// AddressSpace returns the soft and hard limits on the address space of this
// process, which what it starts inherits, in bytes; [math.MaxUint64] is no
// limit.
func AddressSpace() (soft, hard uint64, err error) {
	var limit syscall.Rlimit
	err = getrlimit(syscall.RLIMIT_AS, &limit)
	return limit.Cur, limit.Max, err
}

// startLimited starts cmd under a soft limit of maxBytes on its address
// space, which everything it starts inherits. The limit is this process's own
// for the moment the program is started, since that is what a child inherits,
// and is put back before it is waited for, so nothing this process does while
// it waits, and the removal of what it staged above all, runs under a ceiling
// meant for a mutant.
func startLimited(cmd *exec.Cmd, maxBytes uint64) error {
	var held syscall.Rlimit
	if err := getrlimit(syscall.RLIMIT_AS, &held); err != nil {
		return err
	}
	if err := setrlimit(syscall.RLIMIT_AS, &syscall.Rlimit{Cur: maxBytes, Max: held.Max}); err != nil {
		return err
	}
	startErr := cmd.Start()
	if err := setrlimit(syscall.RLIMIT_AS, &held); err != nil && startErr == nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}
	return startErr
}
