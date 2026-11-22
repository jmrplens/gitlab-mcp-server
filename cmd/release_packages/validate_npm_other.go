// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build !linux

package main

import (
	"fmt"
	"runtime"
)

// stopUnderNpx is a Linux check: it finds the server process under /proc,
// which no other system has.
func (n *npmValidation) stopUnderNpx(string, releasePlatform) {
	fmt.Fprintf(n.stdout, "stop: skipped on %s, which has no /proc to find the server process in\n", runtime.GOOS)
}
