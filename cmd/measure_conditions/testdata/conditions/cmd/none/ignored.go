// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build ignore

package none

// Positive holds a condition in a file the go command never builds, and so
// is not counted.
func Positive(x int) bool {
	if x > 0 {
		return true
	}
	return false
}
