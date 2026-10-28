// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package i18n

import "maps"

// catalogs holds the template of every key in every supported language. A
// template names its values as {name}, a name of lower-case letters, digits
// and underscores that starts with a letter, and uses braces for nothing
// else.
var catalogs = map[Language]map[Key]string{
	English: english,
	Spanish: spanish,
}

// lookup returns the template of key in lang, falling back to English when
// that catalog lacks it and to the key itself when no catalog holds it, so a
// render is never empty.
func lookup(lang Language, key Key) string {
	if template, ok := catalogs[lang][key]; ok {
		return template
	}
	if template, ok := english[key]; ok {
		return template
	}
	return string(key)
}

// Templates returns every template of lang's catalog, keyed by key, as a copy
// the caller may change, and nil for a language no catalog holds. It is how a
// whole catalog is read from outside this package: the character policy test,
// which applies internal/gatewaycompat's predicate and so cannot be an
// internal test of a package internal/config imports, reads each language
// through it.
func Templates(lang Language) map[Key]string {
	return maps.Clone(catalogs[lang])
}

// EnglishTemplate returns the English template of key and whether there is
// one, so an audit gate can judge a call by the source text its key renders.
func EnglishTemplate(key Key) (string, bool) {
	template, ok := english[key]
	return template, ok
}
