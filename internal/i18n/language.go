// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package i18n

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Language is a BCP 47 primary language subtag in lower case, the form every
// catalog is keyed by.
type Language string

// English is the source language: every key is written in it first, and it is
// what a request carrying no language is served.
const English Language = "en"

// Spanish is the first translation, written to a glossary taken from the
// Spanish half of the documentation site and held to the English source key
// by key.
const Spanish Language = "es"

// Default is the language served when the operator chose none, and the one a
// text missing from another catalog falls back to.
const Default = English

// supported lists the languages a catalog exists for, in the order a refusal
// names them. It is a variable so a test can add a language of its own.
var supported = []Language{English, Spanish}

// Supported returns the languages the server can serve, in a slice the caller
// may keep and change.
func Supported() []Language {
	return slices.Clone(supported)
}

// isSupported reports whether a catalog answers lang.
func isSupported(lang Language) bool {
	return slices.Contains(supported, lang)
}

// languageTag is the shape Parse accepts: a primary language subtag of two to
// eight letters followed by any number of hyphenated subtags of one to eight
// letters or digits, the grammar BCP 47 gives every subtag after the first.
var languageTag = regexp.MustCompile(`^([A-Za-z]{2,8})(?:-[A-Za-z0-9]{1,8})*$`)

// Parse reads the language an operator configured.
//
// Surrounding blanks are dropped, and a value that is empty after that is no
// choice at all, which is the default. Any other value is a BCP 47 tag read
// without regard to case, whose subtags after the primary language are
// accepted and dropped (en-GB is English, es-419 and es-MX are Spanish): with
// one catalog per language they choose nothing, which is the answer the RFC
// 4647 lookup would give.
//
// A tag of a language the server has no catalog for is refused, as is a value
// that is not a tag at all (a name, the POSIX form es_ES, a gettext list), and
// the error names the languages it accepts and the value as written. Serving
// such a deployment in English would hide that it asked for something else.
func Parse(value string) (Language, error) {
	tag := strings.TrimSpace(value)
	if tag == "" {
		return Default, nil
	}
	if match := languageTag.FindStringSubmatch(tag); match != nil {
		if lang := Language(strings.ToLower(match[1])); isSupported(lang) {
			return lang, nil
		}
	}
	names := make([]string, len(supported))
	for i, lang := range supported {
		names[i] = string(lang)
	}
	return "", fmt.Errorf("expected one of %s, got %q", strings.Join(names, ", "), value)
}
