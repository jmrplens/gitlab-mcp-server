// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package i18n

import (
	"context"
	"strconv"
	"strings"
)

// Key names one text in every catalog. Keys are declared only as constants in
// keys.go, each with a stable dotted value and named for it in CamelCase
// (common.list.and is [CommonListAnd]).
type Key string

// argKind is what an [Arg] holds, and so how it is rendered.
type argKind uint8

const (
	// argText is a value inserted as it is.
	argText argKind = iota
	// argMessage is a catalog message rendered in the outer one's language.
	argMessage
	// argList is a list of catalog messages joined with the conjunction.
	argList
)

// Arg fills one placeholder of a template, named without its braces.
type Arg struct {
	name  string
	kind  argKind
	text  string
	msg   Message
	items []Message
}

// S fills the placeholder name with value, inserted as it is. A value that
// lands in Markdown is escaped by the caller for where it lands, as it is
// today, before it is passed here.
func S(name, value string) Arg {
	return Arg{name: name, kind: argText, text: value}
}

// N fills the placeholder name with n in base ten, without grouping, in every
// language.
func N(name string, n int64) Arg {
	return Arg{name: name, kind: argText, text: strconv.FormatInt(n, 10)}
}

// M fills the placeholder name with another catalog message, rendered in the
// language the outer message is rendered in.
func M(name string, m Message) Arg {
	return Arg{name: name, kind: argMessage, msg: m}
}

// L fills the placeholder name with items, each rendered in the outer
// message's language and joined pairwise with that language's conjunction
// ([CommonListAnd]) the way strings.Join joins: none renders nothing, one is
// the item alone, three read "a and b and c".
func L(name string, items ...Message) Arg {
	return Arg{name: name, kind: argList, items: items}
}

// value renders the argument in lang.
func (a Arg) value(lang Language) string {
	switch a.kind {
	case argMessage:
		return a.msg.In(lang)
	case argList:
		rendered := make([]string, len(a.items))
		for i, item := range a.items {
			rendered[i] = item.In(lang)
		}
		return strings.Join(rendered, lookup(lang, CommonListAnd))
	default:
		return a.text
	}
}

// Message is a catalog text with its arguments, not yet rendered, so the same
// message can be rendered for a person in the request's language and for the
// model and the logs in English.
type Message struct {
	key  Key
	args []Arg
	// parts is what a [Concat] joins; a message built by [New] has none.
	parts []Message
}

// New builds the message key fills in with args.
func New(key Key, args ...Arg) Message {
	return Message{key: key, args: args}
}

// Concat builds the message that renders each of parts in the one language
// asked for and writes the results one after another, with nothing between
// them. It is how a summary with optional rows is built, each row a catalog
// text of its own that carries its line break.
func Concat(parts ...Message) Message {
	return Message{parts: parts}
}

// In renders the message in lang: the template of its key in that language,
// or in English when that catalog lacks it, or the key itself when no catalog
// holds it, with every placeholder an argument names replaced in one pass, so
// a value that itself reads like a placeholder is inserted as written.
func (m Message) In(lang Language) string {
	if m.key == "" {
		var joined strings.Builder
		for _, part := range m.parts {
			joined.WriteString(part.In(lang))
		}
		return joined.String()
	}
	var pairs []string
	for _, arg := range m.args {
		pairs = append(pairs, "{"+arg.name+"}", arg.value(lang))
	}
	return strings.NewReplacer(pairs...).Replace(lookup(lang, m.key))
}

// Text renders the message in the language ctx carries, for a person.
func (m Message) Text(ctx context.Context) Text {
	return Text{s: m.In(FromContext(ctx))}
}

// English renders the message in English, for the model and the logs, which
// read English whatever language the person was asked in.
func (m Message) English() string {
	return m.In(English)
}
