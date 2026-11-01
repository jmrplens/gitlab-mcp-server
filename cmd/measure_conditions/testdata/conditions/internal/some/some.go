// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package some holds one condition gobco counts in either mode.
package some

// Sign is 1 for a positive x and 0 otherwise.
func Sign(x int) int {
	if x > 0 {
		return 1
	}
	return 0
}
