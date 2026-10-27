// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package i18n

import (
	"context"
	"strconv"
	"strings"
)

// English is the language the catalog is written in, a constant of the
// package that is not a key.
const English = "en"

// Key names one text.
type Key string

// Arg fills one placeholder.
type Arg struct {
	name  string
	value string
}

// Message is a text with its arguments.
type Message struct {
	key   Key
	args  []Arg
	parts []Message
}

// Text is a rendered text.
type Text struct{ s string }

// S fills a placeholder with a text.
func S(name, value string) Arg { return Arg{name: name, value: value} }

// N fills a placeholder with an integer.
func N(name string, n int64) Arg { return Arg{name: name, value: strconv.FormatInt(n, 10)} }

// M fills a placeholder with another message.
func M(name string, m Message) Arg { return Arg{name: name, value: m.render()} }

// L fills a placeholder with a list joined by the conjunction, which this file
// references, so the key counts as used.
func L(name string, items ...Message) Arg {
	rendered := make([]string, len(items))
	for i, item := range items {
		rendered[i] = item.render()
	}
	return Arg{name: name, value: strings.Join(rendered, english[ListAnd])}
}

// New builds a message.
func New(key Key, args ...Arg) Message { return Message{key: key, args: args} }

// Concat joins messages.
func Concat(parts ...Message) Message { return Message{parts: parts} }

// T renders a key, through New with a key that is not a constant, which the
// gate leaves to this package.
func T(ctx context.Context, key Key, args ...Arg) Text { return New(key, args...).Text(ctx) }

// Text renders the message.
func (m Message) Text(context.Context) Text { return Text{s: m.render()} }

// String returns the text.
func (t Text) String() string { return t.s }

// Verbatim wraps a text for tests.
func Verbatim(s string) Text { return Text{s: s} }

// render fills the template.
func (m Message) render() string {
	var out strings.Builder
	for _, part := range m.parts {
		out.WriteString(part.render())
	}
	pairs := make([]string, 0, 2*len(m.args))
	for _, arg := range m.args {
		pairs = append(pairs, "{"+arg.name+"}", arg.value)
	}
	out.WriteString(strings.NewReplacer(pairs...).Replace(english[m.key]))
	return out.String()
}
