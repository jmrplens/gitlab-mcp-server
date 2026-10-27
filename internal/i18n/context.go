// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package i18n

import "context"

// languageKey is the context key a request's language is stored under.
type languageKey struct{}

// WithLanguage returns a context whose texts are rendered in lang, keeping
// everything the parent carried.
func WithLanguage(ctx context.Context, lang Language) context.Context {
	return context.WithValue(ctx, languageKey{}, lang)
}

// FromContext returns the language a context's texts are rendered in: the one
// [WithLanguage] put there when a catalog answers it, and [Default] otherwise,
// for a context nothing set one on as for one set to a language no catalog
// holds.
func FromContext(ctx context.Context) Language {
	if lang, _ := ctx.Value(languageKey{}).(Language); isSupported(lang) {
		return lang
	}
	return Default
}

// Requested returns the language [WithLanguage] put on ctx, as it was put
// there, and whether one was. It is not the language to render in, which is
// what [FromContext] answers: a value no catalog holds is returned as it is,
// so the reader can tell a request carrying en from one carrying nothing,
// which a render, English for both, cannot.
func Requested(ctx context.Context) (Language, bool) {
	lang, ok := ctx.Value(languageKey{}).(Language)
	return lang, ok
}
