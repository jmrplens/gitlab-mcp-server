// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package i18n

import (
	"context"
	"maps"
	"testing"
)

// Keys the rendering tests install into the catalogs for themselves. None of
// them is declared in keys.go, so no test can leave one behind for the
// catalog tests to find.
//
// The second language the tests install, xx, is a pseudo-locale: its texts
// are English marked so that a render in the wrong language cannot pass for
// the right one.
const (
	testKeyGreeting Key = "test.greeting"
	testKeyPair     Key = "test.pair"
	testKeyWrapper  Key = "test.wrapper"
	testKeyWord     Key = "test.word"
	testKeyOther    Key = "test.other"
	testKeyLosses   Key = "test.losses"
	testKeyCount    Key = "test.count"
	testKeyLiteral  Key = "test.literal"
)

// withTemplates installs templates for one language for the length of one
// test, creating that language's catalog if it has none, and puts the
// catalogs back as they were when the test ends. English entries land in the
// English catalog itself, which is the one the fallback reads.
//
// Not parallel: it writes the package's catalogs.
func withTemplates(t *testing.T, lang Language, templates map[Key]string) {
	t.Helper()
	previous := maps.Clone(catalogs)
	catalog, existed := catalogs[lang]
	saved := maps.Clone(catalog)
	if !existed {
		catalog = map[Key]string{}
		catalogs[lang] = catalog
	}
	maps.Copy(catalog, templates)
	t.Cleanup(func() {
		if existed {
			clear(catalog)
			maps.Copy(catalog, saved)
		}
		catalogs = previous
	})
}

// TestMessageIn_FillsEachPlaceholderByName pins substitution by name rather
// than by position, which is what lets a translation put the values in its
// own order: the same arguments fill an English and a reordered template.
func TestMessageIn_FillsEachPlaceholderByName(t *testing.T) {
	withTemplates(t, English, map[Key]string{testKeyGreeting: "Create {kind} in {project}?"})
	withTemplates(t, "xx", map[Key]string{testKeyGreeting: "xx: in {project}, create {kind}?"})

	msg := New(testKeyGreeting, S("kind", "issue"), S("project", "group/app"))
	for lang, want := range map[Language]string{
		English: "Create issue in group/app?",
		"xx":    "xx: in group/app, create issue?",
	} {
		t.Run(string(lang), func(t *testing.T) {
			if got := msg.In(lang); got != want {
				t.Errorf("In(%q) = %q, want %q", lang, got, want)
			}
		})
	}
}

// TestMessageIn_AValueCarryingAPlaceholder_IsNotExpandedAgain pins the one
// pass: a value is inserted as it is, so a GitLab title that happens to read
// {target} is shown as written instead of being filled with another argument.
func TestMessageIn_AValueCarryingAPlaceholder_IsNotExpandedAgain(t *testing.T) {
	withTemplates(t, English, map[Key]string{testKeyPair: "{source} -> {target}"})

	got := New(testKeyPair, S("source", "{target}"), S("target", "main")).In(English)
	if want := "{target} -> main"; got != want {
		t.Errorf("In(en) = %q, want %q", got, want)
	}
}

// TestMessageIn_APlaceholderNoArgumentFills_IsLeftAsWritten pins what a
// template whose argument was not passed renders: the placeholder itself,
// visible, rather than an empty hole a reader would not notice. The audit
// gate issue 1547 adds refuses such a call before it ships; this is what the
// renderer does with one anyway.
func TestMessageIn_APlaceholderNoArgumentFills_IsLeftAsWritten(t *testing.T) {
	withTemplates(t, English, map[Key]string{testKeyPair: "{source} -> {target}"})

	if got, want := New(testKeyPair, S("source", "a")).In(English), "a -> {target}"; got != want {
		t.Errorf("In(en) = %q, want %q", got, want)
	}
	if got, want := New(testKeyPair).In(English), "{source} -> {target}"; got != want {
		t.Errorf("In(en) with no arguments = %q, want %q", got, want)
	}
}

// TestMessageIn_NumbersAreDecimalWithoutGrouping pins how N writes a number in
// every language: base ten, a minus sign where it is negative, and no
// thousands separator, which the English the server serves today never had.
func TestMessageIn_NumbersAreDecimalWithoutGrouping(t *testing.T) {
	withTemplates(t, English, map[Key]string{testKeyCount: "[{n}]"})
	withTemplates(t, "xx", map[Key]string{testKeyCount: "xx<{n}>"})

	for _, tc := range []struct {
		lang Language
		n    int64
		want string
	}{
		{lang: English, n: 0, want: "[0]"},
		{lang: English, n: 1234567, want: "[1234567]"},
		{lang: English, n: -42, want: "[-42]"},
		{lang: "xx", n: 1234567, want: "xx<1234567>"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			if got := New(testKeyCount, N("n", tc.n)).In(tc.lang); got != tc.want {
				t.Errorf("In(%q) = %q, want %q", tc.lang, got, tc.want)
			}
		})
	}
}

// TestMessageIn_ANestedMessage_IsRenderedInTheSameLanguage pins M: the inner
// message is a catalog text of its own, rendered in the language the outer one
// is, so a question that embeds a phrase never mixes two languages.
func TestMessageIn_ANestedMessage_IsRenderedInTheSameLanguage(t *testing.T) {
	withTemplates(t, English, map[Key]string{testKeyWrapper: "<{inner}>", testKeyWord: "word {n}"})
	withTemplates(t, "xx", map[Key]string{testKeyWrapper: "xx({inner})", testKeyWord: "xx-word {n}"})

	msg := New(testKeyWrapper, M("inner", New(testKeyWord, N("n", 3))))
	for lang, want := range map[Language]string{English: "<word 3>", "xx": "xx(xx-word 3)"} {
		t.Run(string(lang), func(t *testing.T) {
			if got := msg.In(lang); got != want {
				t.Errorf("In(%q) = %q, want %q", lang, got, want)
			}
		})
	}
}

// TestMessageIn_AList_IsJoinedWithTheLanguagesConjunction pins L: the items
// are joined pairwise with the conjunction of the language rendered, the way
// strings.Join would join them, so none gives nothing, one is the item alone
// and three read "a and b and c". Today's only caller passes one or two,
// which is what fixes the English byte for byte.
func TestMessageIn_AList_IsJoinedWithTheLanguagesConjunction(t *testing.T) {
	withTemplates(t, English, map[Key]string{testKeyLosses: "Remove {losses}?", testKeyWord: "a", testKeyOther: "b", testKeyPair: "c"})
	withTemplates(t, "xx", map[Key]string{
		testKeyLosses: "xx: remove {losses}?", testKeyWord: "xa", testKeyOther: "xb", testKeyPair: "xc",
		CommonListAnd: " plus ",
	})

	a, b, c := New(testKeyWord), New(testKeyOther), New(testKeyPair)
	for _, tc := range []struct {
		name  string
		lang  Language
		items []Message
		want  string
	}{
		{name: "none", lang: English, want: "Remove ?"},
		{name: "one", lang: English, items: []Message{a}, want: "Remove a?"},
		{name: "two", lang: English, items: []Message{a, b}, want: "Remove a and b?"},
		{name: "three", lang: English, items: []Message{a, b, c}, want: "Remove a and b and c?"},
		{name: "two in another language", lang: "xx", items: []Message{a, b}, want: "xx: remove xa plus xb?"},
		{name: "three in another language", lang: "xx", items: []Message{a, b, c}, want: "xx: remove xa plus xb plus xc?"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := New(testKeyLosses, L("losses", tc.items...)).In(tc.lang); got != tc.want {
				t.Errorf("In(%q) = %q, want %q", tc.lang, got, tc.want)
			}
		})
	}
}

// TestMessageIn_AMissingTranslation_FallsBackToEnglish pins that a language
// whose catalog lacks a key, or which has no catalog at all, is answered with
// the English text rather than with nothing: the completeness test refuses
// the first before it ships, and the second is a language nobody parsed.
func TestMessageIn_AMissingTranslation_FallsBackToEnglish(t *testing.T) {
	withTemplates(t, English, map[Key]string{testKeyGreeting: "Hello {name}", testKeyWord: "word"})
	withTemplates(t, "xx", map[Key]string{testKeyWord: "xx-word"})

	if got, want := New(testKeyGreeting, S("name", "Ana")).In("xx"), "Hello Ana"; got != want {
		t.Errorf("a key the catalog lacks rendered %q, want the English %q", got, want)
	}
	if got, want := New(testKeyWord).In("xx"), "xx-word"; got != want {
		t.Errorf("a key the catalog has rendered %q, want its own %q", got, want)
	}
	if got, want := New(testKeyWord).In("zz"), "word"; got != want {
		t.Errorf("a language with no catalog rendered %q, want the English %q", got, want)
	}
}

// TestMessageIn_AnUnknownKey_RendersTheKey pins the last resort: a key no
// catalog holds renders as itself, visible, rather than as an empty string a
// person would be shown a blank form for. The audit gate issue 1547 adds
// refuses such a key.
func TestMessageIn_AnUnknownKey_RendersTheKey(t *testing.T) {
	const unknown Key = "no.such.key"
	withTemplates(t, "xx", map[Key]string{testKeyWord: "xx-word"})

	for _, lang := range []Language{English, "xx"} {
		t.Run(string(lang), func(t *testing.T) {
			if got := New(unknown, S("ignored", "value")).In(lang); got != string(unknown) {
				t.Errorf("In(%q) = %q, want the key %q", lang, got, unknown)
			}
		})
	}
}

// TestConcat_JoinsItsPartsWithoutASeparator pins how a summary with optional
// rows is built: each part rendered in the one language asked for and the
// results written one after another, with nothing added between them, since
// a row carries its own line break.
func TestConcat_JoinsItsPartsWithoutASeparator(t *testing.T) {
	withTemplates(t, English, map[Key]string{testKeyGreeting: "Create {name}?", testKeyWord: "\n**Row**: {v}", testKeyOther: "!"})
	withTemplates(t, "xx", map[Key]string{testKeyGreeting: "xx: create {name}?", testKeyWord: "\n**xx-row**: {v}", testKeyOther: "#"})

	head := New(testKeyGreeting, S("name", "app"))
	row := New(testKeyWord, S("v", "1"))
	for _, tc := range []struct {
		name string
		msg  Message
		lang Language
		want string
	}{
		{name: "no parts", msg: Concat(), lang: English, want: ""},
		{name: "one part", msg: Concat(head), lang: English, want: "Create app?"},
		{name: "two parts", msg: Concat(head, row), lang: English, want: "Create app?\n**Row**: 1"},
		{name: "two parts in another language", msg: Concat(head, row), lang: "xx", want: "xx: create app?\n**xx-row**: 1"},
		{
			name: "a nested concatenation in another language",
			msg:  Concat(Concat(head, row), New(testKeyOther)),
			lang: "xx",
			want: "xx: create app?\n**xx-row**: 1#",
		},
		{name: "the zero message", msg: Message{}, lang: English, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.msg.In(tc.lang); got != tc.want {
				t.Errorf("In(%q) = %q, want %q", tc.lang, got, tc.want)
			}
		})
	}
}

// TestConcat_AConcatenationNestsAsAnArgument pins that a concatenation is a
// message like any other, so it can fill a placeholder through M and is
// rendered there in the outer message's language.
func TestConcat_AConcatenationNestsAsAnArgument(t *testing.T) {
	withTemplates(t, English, map[Key]string{testKeyWrapper: "[{inner}]", testKeyWord: "a", testKeyOther: "b"})
	withTemplates(t, "xx", map[Key]string{testKeyWrapper: "xx({inner})", testKeyWord: "x", testKeyOther: "y"})

	msg := New(testKeyWrapper, M("inner", Concat(New(testKeyWord), New(testKeyOther))))
	if got, want := msg.In("xx"), "xx(xy)"; got != want {
		t.Errorf("In(xx) = %q, want %q", got, want)
	}
	if got, want := msg.English(), "[ab]"; got != want {
		t.Errorf("English() = %q, want %q", got, want)
	}
}

// TestMessageText_RendersInTheContextLanguage pins Text, which is what a
// person-facing sink is handed: the language the request carries, and
// English for a request that carries none.
func TestMessageText_RendersInTheContextLanguage(t *testing.T) {
	withSupported(t, "xx")
	withTemplates(t, English, map[Key]string{testKeyWord: "word"})
	withTemplates(t, "xx", map[Key]string{testKeyWord: "xx-word"})

	msg := New(testKeyWord)
	if got := msg.Text(WithLanguage(context.Background(), "xx")).String(); got != "xx-word" {
		t.Errorf("Text in a context carrying xx = %q, want %q", got, "xx-word")
	}
	if got := msg.Text(context.Background()).String(); got != "word" {
		t.Errorf("Text in a context carrying nothing = %q, want %q", got, "word")
	}
}

// TestMessageEnglish_IsEnglishWhateverTheRequestCarries pins the render the
// model and the logs read. A question a person is asked is also the refusal a
// model is handed when the client cannot ask, and the model keeps reading
// English whatever language the person was asked in.
func TestMessageEnglish_IsEnglishWhateverTheRequestCarries(t *testing.T) {
	withTemplates(t, English, map[Key]string{testKeyGreeting: "Hello {name}"})
	withTemplates(t, "xx", map[Key]string{testKeyGreeting: "xx: hello {name}"})

	if got, want := New(testKeyGreeting, S("name", "Ana")).English(), "Hello Ana"; got != want {
		t.Errorf("English() = %q, want %q", got, want)
	}
}

// TestMessageIn_ALiteralTemplate_IsReturnedAsWritten pins the template with
// no placeholder, which is most of them: it is served exactly as the catalog
// holds it, whatever arguments the call passed.
func TestMessageIn_ALiteralTemplate_IsReturnedAsWritten(t *testing.T) {
	withTemplates(t, English, map[Key]string{testKeyLiteral: "Collecting issue details..."})

	if got, want := New(testKeyLiteral).In(English), "Collecting issue details..."; got != want {
		t.Errorf("In(en) = %q, want %q", got, want)
	}
	if got, want := New(testKeyLiteral, S("unused", "x")).In(English), "Collecting issue details..."; got != want {
		t.Errorf("In(en) with an unused argument = %q, want %q", got, want)
	}
}
