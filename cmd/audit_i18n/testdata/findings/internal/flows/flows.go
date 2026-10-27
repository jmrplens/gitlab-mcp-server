// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package flows

import (
	"context"

	"example.com/findings/internal/i18n"
)

// firstName names a placeholder with a constant rather than a literal.
const firstName = "first"

// localKey is a key constant this package declares for itself.
const localKey i18n.Key = "flow.pair"

// Unfilled leaves a placeholder of the template unfilled.
func Unfilled(ctx context.Context, a string) i18n.Text {
	return i18n.T(ctx, i18n.Pair, i18n.S("first", a))
}

// Extra fills a placeholder the template does not carry.
func Extra(a, b, c string) i18n.Message {
	return i18n.New(i18n.Pair, i18n.S("first", a), i18n.S("second", b), i18n.S("third", c))
}

// Twice fills one placeholder twice.
func Twice(a, b, c string) i18n.Message {
	return i18n.New(i18n.Pair, i18n.S("first", a), i18n.N("first", 2), i18n.S("second", c))
}

// Held passes an argument built elsewhere.
func Held(held i18n.Arg, b string) i18n.Message {
	return i18n.New(i18n.Pair, held, i18n.S("second", b))
}

// Helped makes an argument in a helper, and another through a function value.
func Helped(a, b string) i18n.Message {
	makeArg := i18n.S
	return i18n.New(i18n.Pair, firstArg(a), makeArg("second", b))
}

// Named names one placeholder with a variable and one with a constant.
func Named(name, a, b string) i18n.Message {
	return i18n.New(i18n.Pair, i18n.S(name, a), i18n.S(firstName, b))
}

// Spread hands the call a list of arguments.
func Spread(args []i18n.Arg) i18n.Message {
	return i18n.New(i18n.Pair, args...)
}

// ByLiteral names its key with a literal, which compiles by conversion.
func ByLiteral(ctx context.Context, name string) i18n.Text {
	return i18n.T(ctx, "flow.literal", i18n.S("name", name))
}

// ByLocal names its key with a constant of its own.
func ByLocal(a, b string) i18n.Message {
	return i18n.New(localKey, i18n.S("first", a), i18n.S("second", b))
}

// ByVariable names its key with a value.
func ByVariable(key i18n.Key) i18n.Message {
	return i18n.New(key)
}

// Converted converts a string to a key.
func Converted(s string) i18n.Message {
	return i18n.New(i18n.Key(s))
}

// Unknown names a key the catalog has no template for.
func Unknown(ctx context.Context) i18n.Text {
	return i18n.T(ctx, i18n.Missing)
}

// Held as a value, so the key of the call made through it cannot be read.
var render = i18n.T

// Verbatim makes a text outside the catalog, by a call and as a value.
func Verbatim() (i18n.Text, func(string) i18n.Text) {
	return i18n.Verbatim("typed inline"), i18n.Verbatim
}

// Literal writes a Text literal, and one whose type its slice gives it.
func Literal() []i18n.Text {
	return append([]i18n.Text{{}}, i18n.Text{})
}

// Ok renders through the function value it was handed, with a well-formed
// render beside it.
func Ok(ctx context.Context) i18n.Text {
	_ = render
	return i18n.T(ctx, i18n.Pair, i18n.S("first", "a"), i18n.S("second", "b"))
}

// firstArg makes an argument in a helper.
func firstArg(a string) i18n.Arg {
	return i18n.S("first", a)
}

// Results takes its context and its key from the results of one call.
func Results(ctx context.Context) i18n.Text {
	return i18n.T(pair(ctx))
}

// pair returns a context and a key together.
func pair(ctx context.Context) (context.Context, i18n.Key) {
	return ctx, i18n.Pair
}

// Empty makes the empty message as a literal and as a join of nothing.
func Empty(ctx context.Context) (i18n.Message, i18n.Text) {
	return i18n.Message{}, i18n.Concat().Text(ctx)
}
