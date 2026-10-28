// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package i18n

import (
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// placeholderPattern is the grammar a placeholder is written in, and the
// only use a template makes of a brace.
var placeholderPattern = regexp.MustCompile(`\{[a-z][a-z0-9_]*\}`)

// placeholdersOf returns the distinct placeholder names a template carries,
// sorted.
func placeholdersOf(template string) []string {
	var names []string
	for _, match := range placeholderPattern.FindAllString(template, -1) {
		if name := match[1 : len(match)-1]; !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// TestCatalogs_EverySupportedLanguageHasOne pins that a language the parser
// accepts is one the renderer has a catalog for, so an operator's choice is
// never served through the English fallback in silence.
func TestCatalogs_EverySupportedLanguageHasOne(t *testing.T) {
	for _, lang := range Supported() {
		t.Run(string(lang), func(t *testing.T) {
			if _, ok := catalogs[lang]; !ok {
				t.Errorf("%s is supported and has no catalog", lang)
			}
		})
	}
	if len(catalogs) != len(Supported()) {
		t.Errorf("%d catalogs for %d supported languages; a catalog nobody can select is dead weight", len(catalogs), len(Supported()))
	}
}

// TestCatalogs_EachHoldsExactlyTheEnglishKeys is the completeness rule:
// English is the source, and every translation carries the same keys, none
// missing and none of its own, so the fallback to English is a safety net
// that never catches anything in a build that ships.
func TestCatalogs_EachHoldsExactlyTheEnglishKeys(t *testing.T) {
	want := slices.Sorted(maps.Keys(english))
	for lang, catalog := range catalogs {
		t.Run(string(lang), func(t *testing.T) {
			if got := slices.Sorted(maps.Keys(catalog)); !slices.Equal(got, want) {
				t.Errorf("%s holds the keys %v, want the English %v", lang, got, want)
			}
		})
	}
}

// TestCatalogs_NoTemplateIsEmpty refuses an entry that would render nothing,
// which a person reads as a form with no question.
func TestCatalogs_NoTemplateIsEmpty(t *testing.T) {
	for lang, catalog := range catalogs {
		for key, template := range catalog {
			t.Run(string(lang)+"/"+string(key), func(t *testing.T) {
				if template == "" {
					t.Errorf("%s %s is empty", lang, key)
				}
			})
		}
	}
}

// TestCatalogs_BracesAreOnlyPlaceholders holds every template to the
// placeholder grammar: once each well-formed placeholder is removed, no brace
// is left, so a typo such as {Title} or {title is caught here rather than
// shown to a person as written.
func TestCatalogs_BracesAreOnlyPlaceholders(t *testing.T) {
	for lang, catalog := range catalogs {
		for key, template := range catalog {
			t.Run(string(lang)+"/"+string(key), func(t *testing.T) {
				rest := placeholderPattern.ReplaceAllString(template, "")
				if regexp.MustCompile(`[{}]`).MatchString(rest) {
					t.Errorf("%s %s = %q carries a brace that is not a placeholder", lang, key, template)
				}
			})
		}
	}
}

// TestCatalogs_EachTranslationFillsTheEnglishPlaceholders is the parity rule
// for arguments: a call passes the placeholders of the English template, so a
// translation that drops one loses a value and one that adds one shows it
// unfilled.
func TestCatalogs_EachTranslationFillsTheEnglishPlaceholders(t *testing.T) {
	for lang, catalog := range catalogs {
		for key, template := range catalog {
			t.Run(string(lang)+"/"+string(key), func(t *testing.T) {
				if got, want := placeholdersOf(template), placeholdersOf(english[key]); !slices.Equal(got, want) {
					t.Errorf("%s %s fills %v, want the English %v", lang, key, got, want)
				}
			})
		}
	}
}

// structure counts the constructions of a template the Markdown escaping
// audit tells apart: line breaks, bold markers, backticks and arrows.
type structure struct {
	lineBreaks int
	bold       int
	backticks  int
	arrows     int
}

// structureOf returns how often each construction occurs in template.
func structureOf(template string) structure {
	return structure{
		lineBreaks: strings.Count(template, "\n"),
		bold:       strings.Count(template, "**"),
		backticks:  strings.Count(template, "`"),
		arrows:     strings.Count(template, "->"),
	}
}

// TestCatalogs_EachTranslationKeepsTheEnglishStructure is the parity rule for
// layout: per key, a translation carries as many line breaks, bold markers,
// backticks and arrows as its English source, so the translated text falls
// into the same Markdown constructions the escaping audit judges in English.
func TestCatalogs_EachTranslationKeepsTheEnglishStructure(t *testing.T) {
	for lang, catalog := range catalogs {
		for key, template := range catalog {
			t.Run(string(lang)+"/"+string(key), func(t *testing.T) {
				if got, want := structureOf(template), structureOf(english[key]); got != want {
					t.Errorf("%s %s has %+v, want the English %+v", lang, key, got, want)
				}
			})
		}
	}
}

// TestStructureOf_CountsEachConstruction holds the helper the layout rule
// rests on, since a helper that counted nothing would let every catalog pass:
// each construction is counted on its own and as often as it occurs, and a
// translation that adds a line break its source does not have reads as
// departing from it.
func TestStructureOf_CountsEachConstruction(t *testing.T) {
	for _, tc := range []struct {
		name     string
		template string
		want     structure
	}{
		{name: "none", template: "Create the issue?", want: structure{}},
		{name: "line breaks", template: "a\nb\n", want: structure{lineBreaks: 2}},
		{name: "bold markers", template: "**Title**: x", want: structure{bold: 2}},
		{name: "backticks", template: "`a` or `b`", want: structure{backticks: 4}},
		{name: "arrows", template: "a -> b -> c", want: structure{arrows: 2}},
		{name: "all four", template: "**From**: `a` -> `b`\n", want: structure{lineBreaks: 1, bold: 2, backticks: 4, arrows: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := structureOf(tc.template); got != tc.want {
				t.Errorf("structureOf(%q) = %+v, want %+v", tc.template, got, tc.want)
			}
		})
	}

	withTemplates(t, "xx", map[Key]string{CommonListAnd: " and\n"})
	if structureOf(catalogs["xx"][CommonListAnd]) == structureOf(english[CommonListAnd]) {
		t.Error("a translation that adds a line break reads as keeping the English structure")
	}
}

// TestPlaceholdersOf_ReadsEachNameOnce holds the helper the parity rule rests
// on, since a helper that read nothing would let every catalog pass.
func TestPlaceholdersOf_ReadsEachNameOnce(t *testing.T) {
	got := placeholdersOf("{b} {a} {b} {Not} {c_1} {}")
	if want := []string{"a", "b", "c_1"}; !slices.Equal(got, want) {
		t.Errorf("placeholdersOf() = %v, want %v", got, want)
	}
}

// TestTemplates_HandsOutACopyOfEachCatalog pins the reader a whole catalog is
// taken through: every template of a supported language, a copy the caller
// may change without changing what the server renders, and nothing for a
// language no catalog holds.
func TestTemplates_HandsOutACopyOfEachCatalog(t *testing.T) {
	for _, lang := range Supported() {
		t.Run(string(lang), func(t *testing.T) {
			got := Templates(lang)
			if !maps.Equal(got, catalogs[lang]) {
				t.Fatalf("Templates(%s) = %v, want the catalog %v", lang, got, catalogs[lang])
			}
			got[CommonListAnd] = "changed"
			if catalogs[lang][CommonListAnd] == "changed" {
				t.Errorf("Templates(%s) handed out the catalog itself: a write changed what the server renders", lang)
			}
		})
	}
	if got := Templates("fr"); got != nil {
		t.Errorf(`Templates("fr") = %v, want nil for a language no catalog holds`, got)
	}
}

// TestEnglishTemplate_AnswersTheSourceText pins the reader the audit gates
// use: the English template of a key, and whether there is one.
func TestEnglishTemplate_AnswersTheSourceText(t *testing.T) {
	withTemplates(t, "xx", map[Key]string{CommonListAnd: " plus "})

	if got, ok := EnglishTemplate(CommonListAnd); !ok || got != " and " {
		t.Errorf("EnglishTemplate(%s) = %q, %v; want %q, true", CommonListAnd, got, ok, " and ")
	}
	if got, ok := EnglishTemplate("no.such.key"); ok || got != "" {
		t.Errorf(`EnglishTemplate("no.such.key") = %q, %v; want "", false`, got, ok)
	}
}
