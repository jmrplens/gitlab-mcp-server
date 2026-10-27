// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package flows

import (
	"context"

	"example.com/clean/internal/i18n"
)

// blank is a zero Text and none a zero Message, which are no literals and
// are allowed.
var (
	blank i18n.Text
	none  i18n.Message
)

// empty is a literal of another type of the catalog, neither Text nor
// Message.
var empty = i18n.Arg{}

// language names a constant of the catalog that is not a key.
var language = i18n.English

// pair is a type of this package, whose literal is no Text either.
type pair struct{ first, second string }

// Pair writes a literal of this package's own type and converts to a type
// that is not the catalog's.
func Pair(a string) []byte {
	_ = empty
	return []byte(pair{first: a}.first + language + none.Text(context.Background()).String())
}

// Greet asks a question with one value.
func Greet(ctx context.Context, name string) i18n.Text {
	return i18n.T(ctx, i18n.Greeting, i18n.S("name", name))
}

// Summarize builds a summary with an optional row.
func Summarize(thing, title, labels string) i18n.Message {
	return i18n.Concat(
		i18n.New(i18n.Summary, i18n.S("thing", thing), i18n.S("title", title)),
		i18n.New((i18n.Row), i18n.S("labels", labels)),
	)
}

// Counted fills a placeholder with an integer and one with a list.
func Counted(ctx context.Context, n int64) i18n.Text {
	return i18n.T(ctx, i18n.Count, i18n.N("count", n), i18n.L("items", i18n.New(i18n.Greeting, i18n.S("name", "a"))))
}

// Nested fills a placeholder with another message, the argument and its
// name in parentheses.
func Nested() i18n.Message {
	return i18n.New(i18n.Count, i18n.N("count", 1), (i18n.M("items", i18n.New(i18n.Greeting, i18n.S(("name"), "b")))))
}

// Describe calls a method of the universe, which belongs to no package, and a
// function literal, which is no declared function at all.
func Describe(err error) string {
	return err.Error() + blank.String() + func() string { return "" }()
}
