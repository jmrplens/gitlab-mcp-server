// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package i18n

import (
	"slices"
	"testing"
)

// withSupported adds languages to the supported list for one test, so a
// language the server does not ship can stand in for the next one a catalog
// is written for, and the paths that read the list (the refusal's wording, a
// context carrying such a language) can be held to it. The list is restored
// when the test ends.
//
// Not parallel: it replaces a package variable every reader consults.
func withSupported(t *testing.T, langs ...Language) {
	t.Helper()
	previous := supported
	supported = append(slices.Clone(previous), langs...)
	t.Cleanup(func() { supported = previous })
}

// TestParse_AcceptedValues_ReduceToTheirLanguage pins what a value an operator
// writes resolves to. A blank value is no value and gives the default; any
// other is a BCP 47 tag read without regard to case, whose subtags after the
// primary language are accepted and dropped, since a region or a script
// chooses nothing among the catalogs the server ships: es-ES, es-419 and
// es-MX are all served the one Spanish catalog. The subtag bounds are held at
// both ends: one character and eight are subtags, nine is not.
func TestParse_AcceptedValues_ReduceToTheirLanguage(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		want  Language
	}{
		{name: "unset", value: "", want: English},
		{name: "blanks", value: "   ", want: English},
		{name: "tabs and a newline", value: "\t\n", want: English},
		{name: "the tag itself", value: "en", want: English},
		{name: "surrounded by blanks", value: " en ", want: English},
		{name: "upper case", value: "EN", want: English},
		{name: "mixed case", value: "eN", want: English},
		{name: "a region", value: "en-GB", want: English},
		{name: "a region in lower case", value: "en-gb", want: English},
		{name: "a numeric region", value: "en-001", want: English},
		{name: "a region and a variant", value: "EN-GB-oxendict", want: English},
		{name: "a one-character subtag", value: "en-x-private", want: English},
		{name: "an eight-character subtag", value: "en-abcdefgh", want: English},
		{name: "spanish", value: "es", want: Spanish},
		{name: "spanish surrounded by blanks", value: " es ", want: Spanish},
		{name: "spanish in upper case", value: "ES", want: Spanish},
		{name: "spanish of spain", value: "es-ES", want: Spanish},
		{name: "latin american spanish", value: "es-419", want: Spanish},
		{name: "mexican spanish in mixed case", value: "ES-mx", want: Spanish},
		{name: "spanish with a script", value: "es-Latn-ES", want: Spanish},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.value)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tc.value, err)
			}
			if got != tc.want {
				t.Errorf("Parse(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

// TestParse_RefusedValues_NameTheLanguagesAndTheValue pins the refusal. A
// language the server does not ship is refused rather than served in English,
// because an operator who wrote fr asked for something this deployment will
// not do, and a start under another language would hide that. A value that is
// not a language tag at all (a name, the POSIX locale form, a broken subtag)
// is refused the same way, and the message names every language the server
// accepts and the value as written.
func TestParse_RefusedValues_NameTheLanguagesAndTheValue(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
	}{
		{name: "a language the server does not ship", value: "fr"},
		{name: "a region of an unshipped language", value: "fr-FR"},
		{name: "a language name", value: "spanish"},
		{name: "the spanish name in spanish", value: "espanol"},
		{name: "the english name", value: "english"},
		{name: "the POSIX locale form", value: "es_ES"},
		{name: "the POSIX form of english", value: "en_GB"},
		{name: "a gettext priority list", value: "es_ES:es"},
		{name: "a list of two tags", value: "en:es"},
		{name: "a trailing hyphen", value: "en-"},
		{name: "an empty subtag", value: "en--GB"},
		{name: "a leading hyphen", value: "-en"},
		{name: "a nine-character subtag", value: "en-abcdefghi"},
		{name: "a subtag that is not alphanumeric", value: "en-G@B"},
		{name: "a blank inside the tag", value: "en GB"},
		{name: "one letter", value: "e"},
		{name: "a digit for a language", value: "e1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.value)
			if err == nil {
				t.Fatalf("Parse(%q) = %q, want a refusal", tc.value, got)
			}
			if got != "" {
				t.Errorf("Parse(%q) returned %q beside its refusal, want nothing", tc.value, got)
			}
			want := `expected one of en, es, got "` + tc.value + `"`
			if err.Error() != want {
				t.Errorf("Parse(%q) error = %q, want %q", tc.value, err, want)
			}
		})
	}
}

// TestParse_TheRefusalListsEverySupportedLanguage holds the message to the
// list it is built from, in order, so the day a catalog is added the refusal
// names it without anybody remembering to edit a sentence.
func TestParse_TheRefusalListsEverySupportedLanguage(t *testing.T) {
	withSupported(t, "xx", "yy")

	_, err := Parse("fr")
	if err == nil {
		t.Fatal(`Parse("fr") = nil error, want a refusal`)
	}
	if want := `expected one of en, es, xx, yy, got "fr"`; err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
	if got, parseErr := Parse("XX-gb"); parseErr != nil || got != "xx" {
		t.Errorf(`Parse("XX-gb") = %q, %v; want "xx" once it is supported`, got, parseErr)
	}
}

// TestSupported_IsEnglishThenSpanishAndACopy pins the list this release
// ships, in the order a refusal names it, and that a caller cannot change it
// by writing into what it was handed.
func TestSupported_IsEnglishThenSpanishAndACopy(t *testing.T) {
	got := Supported()
	if want := []Language{English, Spanish}; !slices.Equal(got, want) {
		t.Fatalf("Supported() = %v, want %v", got, want)
	}
	got[0] = "xx"
	if again := Supported(); again[0] != English {
		t.Errorf("Supported() handed out its backing array: a write made it %v", again)
	}
}

// TestDefault_IsEnglish pins the language every reader falls back to, which
// is the promise that a deployment setting nothing is served what it was
// served before the setting existed, and the tags the two languages are
// written as, which are what an operator types.
func TestDefault_IsEnglish(t *testing.T) {
	if Default != English {
		t.Errorf("Default = %q, want %q", Default, English)
	}
	if English != "en" {
		t.Errorf("English = %q, want the BCP 47 tag en", English)
	}
	if Spanish != "es" {
		t.Errorf("Spanish = %q, want the BCP 47 tag es", Spanish)
	}
}
