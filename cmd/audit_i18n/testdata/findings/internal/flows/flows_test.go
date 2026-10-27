// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package flows

import (
	"testing"

	"example.com/findings/internal/i18n"
)

// TestOnly_Reference_DoesNotCount is the one reference to TestOnly: a key a
// test alone renders is still dead to the gate, which reads no test file.
func TestOnly_Reference_DoesNotCount(t *testing.T) {
	_ = i18n.New(i18n.TestOnly)
}
