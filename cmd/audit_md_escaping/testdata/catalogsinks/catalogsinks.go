// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package catalogsinks

import (
	"context"
	"strconv"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/i18n"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The fixture keys. Their templates are in the audit's test, which installs
// them in place of the shipped catalog's.
const (
	keyCell    i18n.Key = "fixture.cell"
	keyHeading i18n.Key = "fixture.heading"
	keyList    i18n.Key = "fixture.list"
	keyLink    i18n.Key = "fixture.link"
	keyFence   i18n.Key = "fixture.fence"
	keyProse   i18n.Key = "fixture.prose"
	keyPair    i18n.Key = "fixture.pair"
	keyKind    i18n.Key = "fixture.kind"
	keyInner   i18n.Key = "fixture.inner"
	keyUnknown i18n.Key = "fixture.unknown"
)

// placeholderName is a constant name for a placeholder, which binds as a
// literal does.
const placeholderName = "name"

// Issue stands in for a struct filled from a GitLab response.
type Issue struct {
	Title string
	URL   string
	Body  string
}

// Cell puts a GitLab-authored title in a table cell unescaped, beside an
// integer, which renders in decimal and is not judged.
func Cell(ctx context.Context, issue Issue) i18n.Text {
	return i18n.T(ctx, keyCell, i18n.S("name", issue.Title), i18n.N("count", 3))
}

// EscapedCell puts the same title in the cell through the escaper, under a
// name written as a constant.
func EscapedCell(ctx context.Context, issue Issue) i18n.Text {
	return i18n.T(ctx, keyCell, i18n.S(placeholderName, toolutil.EscapeMdTableCell(issue.Title)), i18n.N("count", 3))
}

// Heading puts a raw title in a heading beside a nested message, which is
// catalog text and a render of its own.
func Heading(issue Issue) i18n.Message {
	return i18n.New(keyHeading, i18n.S("title", issue.Title), i18n.M("kind", i18n.New(keyKind)))
}

// List prints a flag as a word in a list item, beside a list of catalog
// messages.
func List(ctx context.Context, done bool) i18n.Text {
	return i18n.T(ctx, keyList,
		i18n.S("flag", strconv.FormatBool(done)),
		i18n.L("items", i18n.New(keyKind), i18n.New(keyKind)))
}

// Link builds a link whose label is escaped and whose destination is not.
func Link(ctx context.Context, issue Issue) i18n.Text {
	return i18n.T(ctx, keyLink,
		(i18n.S("label", toolutil.EscapeMdLinkLabel(issue.Title))),
		i18n.S("url", issue.URL))
}

// Fence writes a GitLab-authored body inside a fence the template opens.
func Fence(ctx context.Context, issue Issue) i18n.Text {
	return i18n.T(ctx, keyFence, i18n.S("body", issue.Body))
}

// Prose puts a raw title in a paragraph, which no construct judges.
func Prose(ctx context.Context, issue Issue) i18n.Text {
	return i18n.T(ctx, keyProse, i18n.S("title", issue.Title))
}

// Repeated names {first} twice; the first argument is the one the render
// fills it with, so the raw second one lands nowhere.
func Repeated(ctx context.Context, issue Issue) i18n.Text {
	return i18n.T(ctx, keyPair,
		i18n.S("first", toolutil.EscapeMdTableCell(issue.Title)),
		i18n.S("first", issue.Title),
		i18n.S("second", toolutil.EscapeMdTableCell(issue.URL)))
}

// Missing fills {first} and leaves {second} as the template wrote it, which
// is constant text.
func Missing(ctx context.Context, issue Issue) i18n.Text {
	return i18n.T(ctx, keyPair, i18n.S("first", toolutil.EscapeMdTableCell(issue.Title)))
}

// Held passes an argument built elsewhere, which could fill either
// placeholder, before one the call names.
func Held(ctx context.Context, held i18n.Arg, issue Issue) i18n.Text {
	return i18n.T(ctx, keyPair, held, titleArg(issue), i18n.S("second", issue.URL))
}

// Spread hands the render a list of arguments assembled elsewhere.
func Spread(args []i18n.Arg) i18n.Message {
	return i18n.New(keyPair, args...)
}

// Named names a placeholder with a value rather than a constant, and makes
// the other argument through a function value.
func Named(ctx context.Context, name string, issue Issue) i18n.Text {
	makeArg := i18n.S
	return i18n.T(ctx, keyPair, i18n.S(name, issue.Title), makeArg("second", issue.URL))
}

// Dynamic renders a key that is not a constant, so no template can be read.
func Dynamic(ctx context.Context, key i18n.Key, issue Issue) i18n.Text {
	return i18n.T(ctx, key, i18n.S("name", issue.Title))
}

// Unknown renders a key with no template, which renders as the key itself.
func Unknown(ctx context.Context, issue Issue) i18n.Text {
	return i18n.T(ctx, keyUnknown, i18n.S("name", issue.Title))
}

// Joined renders two catalog messages one after the other; each is a render
// of its own.
func Joined(ctx context.Context, issue Issue) i18n.Text {
	return i18n.Concat(i18n.New(keyCell, i18n.S("name", issue.Title), i18n.N("count", 1)), i18n.New(keyKind)).Text(ctx)
}

// Nested puts a raw title in a table cell through a message nested in the
// cell's placeholder, so the title lands in the cell however the inner
// template writes it.
func Nested(ctx context.Context, issue Issue) i18n.Text {
	return i18n.T(ctx, keyCell, i18n.M("name", i18n.New(keyInner, i18n.S("text", issue.Title))), i18n.N("count", 1))
}

// Listed puts a list in a table cell: an escaped title through a message, and
// a raw body through a message joined to another inside the list.
func Listed(ctx context.Context, issue Issue) i18n.Text {
	return i18n.T(ctx, keyCell,
		i18n.L("name",
			i18n.New(keyInner, i18n.S("text", toolutil.EscapeMdTableCell(issue.Title))),
			i18n.Concat(i18n.New(keyKind), i18n.New(keyInner, i18n.S("text", issue.Body)))),
		i18n.N("count", 2))
}

// HeldMessage fills table cells with a message built elsewhere and with a
// list spread into the call, neither of which the call shows the values of.
func HeldMessage(ctx context.Context, held i18n.Message, items []i18n.Message) i18n.Text {
	return i18n.T(ctx, keyPair, i18n.M("first", held), i18n.L("second", items...))
}

// HeldInside builds the nested messages inline, one from an argument built
// elsewhere and one as a join of a list spread into it.
func HeldInside(ctx context.Context, held i18n.Arg, parts []i18n.Message) i18n.Text {
	return i18n.T(ctx, keyPair, i18n.M("first", i18n.New(keyInner, held)), i18n.M("second", i18n.Concat(parts...)))
}

// Returned fills table cells with messages a helper and a function value
// return, whose values the call does not show.
func Returned(ctx context.Context, issue Issue, build func() i18n.Message) i18n.Text {
	return i18n.T(ctx, keyPair, i18n.M("first", titleMessage(issue)), i18n.M("second", build()))
}

// Results takes the context and the key of a render from the results of one
// call, which Go accepts for a call with a variadic parameter.
func Results(ctx context.Context) i18n.Text {
	return i18n.T(renderPair(ctx))
}

// ResultsInside nests a message whose key and argument come from the results
// of one call.
func ResultsInside(ctx context.Context, issue Issue) i18n.Text {
	return i18n.T(ctx, keyPair, i18n.M("first", i18n.New(keyArg(issue))), i18n.S("second", "x"))
}

// titleArg makes an argument in a helper, which the call cannot bind.
func titleArg(issue Issue) i18n.Arg {
	return i18n.S("first", issue.Title)
}

// titleMessage builds a message in a helper, a render of its own.
func titleMessage(issue Issue) i18n.Message {
	return i18n.New(keyInner, i18n.S("text", issue.Title))
}

// renderPair returns a context and a key together.
func renderPair(ctx context.Context) (context.Context, i18n.Key) {
	return ctx, keyCell
}

// keyArg returns a key and the argument that fills its template together.
func keyArg(issue Issue) (i18n.Key, i18n.Arg) {
	return keyInner, i18n.S("text", issue.Title)
}
