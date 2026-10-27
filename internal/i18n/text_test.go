// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package i18n

import (
	"context"
	"testing"
)

// TestT_IsTheMessageRenderedInTheContextLanguage pins the shorthand every
// person-facing call site uses: T is New followed by Text, with the same key,
// the same arguments and the same language.
func TestT_IsTheMessageRenderedInTheContextLanguage(t *testing.T) {
	withSupported(t, "xx")
	withTemplates(t, English, map[Key]string{testKeyGreeting: "Hello {name}"})
	withTemplates(t, "xx", map[Key]string{testKeyGreeting: "xx: hello {name}"})

	for _, tc := range []struct {
		name string
		ctx  context.Context
		want string
	}{
		{name: "no language", ctx: context.Background(), want: "Hello Ana"},
		{name: "a second language", ctx: WithLanguage(context.Background(), "xx"), want: "xx: hello Ana"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := T(tc.ctx, testKeyGreeting, S("name", "Ana"))
			if got.String() != tc.want {
				t.Errorf("T() = %q, want %q", got, tc.want)
			}
			if same := New(testKeyGreeting, S("name", "Ana")).Text(tc.ctx); got != same {
				t.Errorf("T() = %q, while New(...).Text() = %q", got, same)
			}
		})
	}
}

// TestVerbatim_CarriesItsStringUnchanged pins the test-only constructor, which
// lets a test of a sink hand it a text without a catalog entry of its own.
func TestVerbatim_CarriesItsStringUnchanged(t *testing.T) {
	const s = "Enter {not a placeholder} as written"
	if got := Verbatim(s).String(); got != s {
		t.Errorf("Verbatim(%q).String() = %q", s, got)
	}
}

// TestText_TheZeroValue_IsEmpty pins what a sink receives from a Text nobody
// built: the empty string, which the audit gate issue 1547 adds refuses
// outside this package.
func TestText_TheZeroValue_IsEmpty(t *testing.T) {
	if got := (Text{}).String(); got != "" {
		t.Errorf("Text{}.String() = %q, want empty", got)
	}
}
