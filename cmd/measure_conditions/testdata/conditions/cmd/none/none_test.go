// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package none

import "testing"

// TestBig holds a condition, which is in a test file and so not counted.
func TestBig(t *testing.T) {
	if !Big {
		t.Fatal("Big is false")
	}
}
