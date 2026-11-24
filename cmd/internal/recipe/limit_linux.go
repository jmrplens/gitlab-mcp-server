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

// TryAddressSpace sets the soft limit a call's AddressSpace of maxBytes would,
// puts it back, and returns why either was refused: a sandbox may let a process
// read its limits and not set them.
func TryAddressSpace(maxBytes uint64) error {
	return underLimit(maxBytes, func() error { return nil }, NoCleanup)
}

// startLimited starts cmd under a soft limit of maxBytes on its address
// space, which everything it starts inherits. The limit is this process's own
// for the moment the program is started, since that is what a child inherits,
// and is put back before it is waited for, so nothing this process does while
// it waits, and the removal of what it staged above all, runs under a ceiling
// meant for a mutant. A process may not raise its hard limit again without a
// privilege, so the hard one stays what it was, where the `ulimit -v` of the
// script this was lowered both: a program that raises its own soft limit is
// held no longer.
func startLimited(cmd *exec.Cmd, maxBytes uint64) error {
	return underLimit(maxBytes, cmd.Start, func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
}

// underLimit calls do while the soft limit on this process's address space is
// maxBytes. Where it cannot be put back after a do that went well, it calls
// undo, and that is the error.
func underLimit(maxBytes uint64, do func() error, undo func()) error {
	var held syscall.Rlimit
	if err := getrlimit(syscall.RLIMIT_AS, &held); err != nil {
		return err
	}
	if err := setrlimit(syscall.RLIMIT_AS, &syscall.Rlimit{Cur: maxBytes, Max: held.Max}); err != nil {
		return err
	}
	doErr := do()
	if err := setrlimit(syscall.RLIMIT_AS, &held); err != nil && doErr == nil {
		undo()
		return err
	}
	return doErr
}
