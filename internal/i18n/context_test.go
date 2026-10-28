// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package i18n

import (
	"context"
	"testing"
)

// TestFromContext_WithNoLanguage_IsEnglish pins what every path the server's
// middleware does not cover reads: a test, an internal inspection, a
// goroutine started from a background context. Each is served English, which
// is what it was served before the language existed.
func TestFromContext_WithNoLanguage_IsEnglish(t *testing.T) {
	if got := FromContext(context.Background()); got != English {
		t.Errorf("FromContext(background) = %q, want %q", got, English)
	}
}

// TestFromContext_ReturnsTheLanguageTheContextCarries is the other direction:
// a supported language set on the context is the one read back, Spanish as
// shipped and one added for the test alike, and English set explicitly is
// English.
func TestFromContext_ReturnsTheLanguageTheContextCarries(t *testing.T) {
	withSupported(t, "xx")

	for _, lang := range []Language{Spanish, "xx", English} {
		t.Run(string(lang), func(t *testing.T) {
			if got := FromContext(WithLanguage(context.Background(), lang)); got != lang {
				t.Errorf("FromContext(WithLanguage(%q)) = %q", lang, got)
			}
		})
	}
}

// TestFromContext_AnUnsupportedLanguage_IsEnglish pins that a value no
// catalog answers never reaches the renderer as a language: an empty one,
// which a hand-built configuration carries, one the parser would refuse, and
// a tag the parser would have reduced (ES, es-ES) but that was stored as it
// is written, which only Parse turns into a language.
func TestFromContext_AnUnsupportedLanguage_IsEnglish(t *testing.T) {
	for _, lang := range []Language{"", "fr", "EN", "ES", "es-ES"} {
		t.Run(string(lang), func(t *testing.T) {
			if got := FromContext(WithLanguage(context.Background(), lang)); got != English {
				t.Errorf("FromContext(WithLanguage(%q)) = %q, want %q", lang, got, English)
			}
		})
	}
}

// TestRequested_ReadsWhatWithLanguageStored pins the reader that tells a
// context carrying a language from one carrying none, which FromContext
// cannot, since it answers English for both: nothing set reads as nothing, a
// value is read back as it was stored, one no catalog holds included, and the
// innermost of two is the one read.
func TestRequested_ReadsWhatWithLanguageStored(t *testing.T) {
	for _, tc := range []struct {
		name   string
		ctx    context.Context
		want   Language
		wantOK bool
	}{
		{name: "nothing set", ctx: context.Background(), want: "", wantOK: false},
		{name: "english", ctx: WithLanguage(context.Background(), English), want: English, wantOK: true},
		{name: "a language no catalog holds", ctx: WithLanguage(context.Background(), "fr"), want: "fr", wantOK: true},
		{name: "the innermost of two", ctx: WithLanguage(WithLanguage(context.Background(), "fr"), English), want: English, wantOK: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := Requested(tc.ctx); got != tc.want || ok != tc.wantOK {
				t.Errorf("Requested() = %q, %v; want %q, %v", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// TestWithLanguage_KeepsWhatTheContextAlreadyCarried pins that the language
// is added to the request's context rather than replacing it: the deadline,
// the credential and every other value set before it reach the handler as
// they were.
func TestWithLanguage_KeepsWhatTheContextAlreadyCarried(t *testing.T) {
	type key struct{}
	parent := context.WithValue(context.Background(), key{}, "credential")

	ctx := WithLanguage(parent, English)
	if got := ctx.Value(key{}); got != "credential" {
		t.Errorf("the parent's value reads %v after WithLanguage, want it kept", got)
	}
}
