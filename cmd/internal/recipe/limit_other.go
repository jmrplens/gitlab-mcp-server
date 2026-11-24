// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build !linux

package recipe

import (
	"errors"
	"os/exec"
)

// AddressSpace is answered on Linux alone, the kernel the recipes rely on to
// enforce the limit.
func AddressSpace() (soft, hard uint64, err error) {
	return 0, 0, errors.ErrUnsupported
}

// TryAddressSpace answers that no limit can be set, on Linux alone as
// [AddressSpace] is.
func TryAddressSpace(uint64) error {
	return errors.ErrUnsupported
}

// startLimited refuses to start a program under a limit. Outside Linux no
// caller asks for one, and one that did would be refused rather than told it
// held.
func startLimited(*exec.Cmd, uint64) error {
	return errors.ErrUnsupported
}
