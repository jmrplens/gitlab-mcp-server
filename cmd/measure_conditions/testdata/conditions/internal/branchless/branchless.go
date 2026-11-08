// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package branchless holds one condition in condition mode, a comparison it
// returns, and no branch.
package branchless

// Same reports whether a and b are equal.
func Same(a, b int) bool {
	return a == b
}
