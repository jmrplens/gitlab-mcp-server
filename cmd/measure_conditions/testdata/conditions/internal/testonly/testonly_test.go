// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package testonly is a test file and nothing else, so the go command builds
// no file of it that gobco would instrument.
package testonly

import "testing"

// TestNothing holds a condition, in a test file.
func TestNothing(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
}
