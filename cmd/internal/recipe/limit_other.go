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

// startLimited starts cmd. Outside Linux no caller asks for a limit, and one
// that did would be refused rather than told it held.
func startLimited(cmd *exec.Cmd, maxBytes uint64) error {
	if maxBytes != 0 {
		return errors.ErrUnsupported
	}
	return cmd.Start()
}
