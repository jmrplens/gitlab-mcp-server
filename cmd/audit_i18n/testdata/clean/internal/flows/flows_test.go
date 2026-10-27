// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package flows

import (
	"testing"

	"example.com/clean/internal/i18n"
)

// TestGreet_Verbatim_IsATestsOwn uses what only a test may: Verbatim, a Text
// literal and a key named by a literal. The gate does not read test files.
func TestGreet_Verbatim_IsATestsOwn(t *testing.T) {
	if i18n.Verbatim("x") == (i18n.Text{}) {
		t.Error("a verbatim text is empty")
	}
	_ = i18n.New("flow.greeting")
}
