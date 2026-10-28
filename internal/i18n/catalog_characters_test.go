// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package i18n_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/gatewaycompat"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/i18n"
)

// spanishLetters is what a Spanish text needs beyond ASCII: the accented
// vowels, the diaeresis and the eñe in both cases, and the marks that open a
// question and an exclamation.
const spanishLetters = "áéíóúüñÁÉÍÓÚÜÑ¿¡"

// The characters a translation is most tempted to use and the policy refuses
// by name, written as their code points so that this file carries none of
// them.
const (
	enDash             = rune(0x2013)
	emDash             = rune(0x2014)
	horizontalEllipsis = rune(0x2026)
	leftSingleQuote    = rune(0x2018)
	rightSingleQuote   = rune(0x2019)
	lowSingleQuote     = rune(0x201a)
	leftDoubleQuote    = rune(0x201c)
	rightDoubleQuote   = rune(0x201d)
	lowDoubleQuote     = rune(0x201e)
	leftGuillemet      = rune(0x00ab)
	rightGuillemet     = rune(0x00bb)
	noBreakSpace       = rune(0x00a0)
)

// beyondASCII is, per language, every character its catalog may carry that
// the policy of the listed catalog refuses. English may carry none, which is
// that policy itself. A language without an entry has no policy, and its
// catalog is refused until one is written.
var beyondASCII = map[i18n.Language]string{
	i18n.English: "",
	i18n.Spanish: spanishLetters,
}

// refusedByName are refused in every language whatever it may otherwise
// carry, with what each is called in a finding. A semicolon is the character
// a strict gateway refused, dashes are the house style's, the ellipsis is
// written as three dots, and typographic quotes are not quotes a terminal or
// a form shows reliably.
var refusedByName = map[rune]string{
	';':                "a semicolon",
	enDash:             "an en dash",
	emDash:             "an em dash",
	horizontalEllipsis: "an ellipsis character, where three dots are written",
	leftSingleQuote:    "a typographic quote",
	rightSingleQuote:   "a typographic quote",
	lowSingleQuote:     "a typographic quote",
	leftDoubleQuote:    "a typographic quote",
	rightDoubleQuote:   "a typographic quote",
	lowDoubleQuote:     "a typographic quote",
	leftGuillemet:      "a typographic quote",
	rightGuillemet:     "a typographic quote",
}

// policyFinding returns what is wrong with r in a template of a language that
// may carry extra beyond ASCII, or the empty string when nothing is: a
// character refused by name, a control character other than the line break,
// or a character outside extra that the gateway policy of the listed catalog
// (gatewaycompat.OffendingRune) refuses.
func policyFinding(extra string, r rune) string {
	if name, refused := refusedByName[r]; refused {
		return name
	}
	if r == '\n' || strings.ContainsRune(extra, r) {
		return ""
	}
	if r < ' ' || r == '\u007f' {
		return "a control character"
	}
	if gatewaycompat.OffendingRune(r) {
		return "a character the gateway policy refuses"
	}
	return ""
}

// TestCatalogs_EachTemplateKeepsItsLanguagesCharacters is the character
// policy of the catalog. English is printable ASCII and the line break with no
// semicolon, the policy cmd/audit_gateway_chars holds the listed catalog to,
// judged by the same predicate. Spanish is the same with the letters and marks
// Spanish needs, and with no dash, ellipsis character or typographic quote.
//
// None of it reaches a listing a gateway validates (an elicitation and a
// progress message travel as content), so the policy is not a gateway's: it
// keeps a translation in the characters every client renders, and makes a
// stray character a test failure rather than something a person sees.
//
// It is an external test because internal/gatewaycompat imports
// internal/config, which imports this package, so an internal test importing
// it would be an import cycle. It reads every catalog through i18n.Templates
// rather than through an export_test.go, which gobco, the condition coverage
// gate, cannot resolve from an external test package.
func TestCatalogs_EachTemplateKeepsItsLanguagesCharacters(t *testing.T) {
	for _, lang := range i18n.Supported() {
		t.Run(string(lang), func(t *testing.T) {
			extra, ok := beyondASCII[lang]
			if !ok {
				t.Fatalf("%s has a catalog and no character policy", lang)
			}
			catalog := i18n.Templates(lang)
			if len(catalog) == 0 {
				t.Fatalf("%s is supported and Templates hands out no template of it", lang)
			}
			for key, template := range catalog {
				t.Run(string(key), func(t *testing.T) {
					for _, r := range template {
						if finding := policyFinding(extra, r); finding != "" {
							t.Errorf("%s %s = %q carries %U, %s", lang, key, template, r, finding)
						}
					}
				})
			}
		})
	}
}

// TestCatalogs_EveryCharacterPolicyHasACatalog is the other direction: a
// policy written for a language the server has no catalog for is dead, and
// most likely names a language whose catalog was meant to be added.
func TestCatalogs_EveryCharacterPolicyHasACatalog(t *testing.T) {
	for lang := range beyondASCII {
		t.Run(string(lang), func(t *testing.T) {
			if i18n.Templates(lang) == nil {
				t.Errorf("%s has a character policy and no catalog", lang)
			}
		})
	}
}

// TestPolicyFinding_RefusesWhatThePolicyNames holds the helper the policy
// rests on, since one that refused nothing would let every catalog pass: each
// class is refused and each allowance is honored, for English and for
// Spanish, and a character refused by name stays refused even in a language
// told it may carry it.
func TestPolicyFinding_RefusesWhatThePolicyNames(t *testing.T) {
	carriesDashAndSemicolon := string([]rune{emDash, ';'})
	for _, tc := range []struct {
		extra   string
		r       rune
		refused bool
	}{
		{extra: "", r: 'a', refused: false},
		{extra: "", r: '~', refused: false},
		{extra: "", r: ' ', refused: false},
		{extra: "", r: '\n', refused: false},
		{extra: "", r: '\t', refused: true},
		{extra: "", r: '\r', refused: true},
		{extra: "", r: '\x1f', refused: true},
		{extra: "", r: '\u007f', refused: true},
		{extra: "", r: ';', refused: true},
		{extra: "", r: '\u0080', refused: true},
		{extra: "", r: 'é', refused: true},
		{extra: "", r: '¿', refused: true},
		{extra: spanishLetters, r: 'é', refused: false},
		{extra: spanishLetters, r: 'Ñ', refused: false},
		{extra: spanishLetters, r: '¡', refused: false},
		{extra: spanishLetters, r: 'a', refused: false},
		{extra: spanishLetters, r: ';', refused: true},
		{extra: spanishLetters, r: 'ç', refused: true},
		{extra: spanishLetters, r: 'à', refused: true},
		{extra: spanishLetters, r: noBreakSpace, refused: true},
		{extra: spanishLetters, r: emDash, refused: true},
		{extra: spanishLetters, r: enDash, refused: true},
		{extra: spanishLetters, r: horizontalEllipsis, refused: true},
		{extra: spanishLetters, r: leftDoubleQuote, refused: true},
		{extra: spanishLetters, r: leftGuillemet, refused: true},
		{extra: carriesDashAndSemicolon, r: emDash, refused: true},
		{extra: carriesDashAndSemicolon, r: ';', refused: true},
	} {
		t.Run(fmt.Sprintf("%+q/%U", tc.extra, tc.r), func(t *testing.T) {
			if got := policyFinding(tc.extra, tc.r) != ""; got != tc.refused {
				t.Errorf("policyFinding(%+q, %U) refused = %v, want %v", tc.extra, tc.r, got, tc.refused)
			}
		})
	}
}
