// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package i18n

import "context"

// Text is a rendered catalog text, the type a person-facing sink takes once
// issue 1547 converts it.
//
// Only this package makes one that is not empty, so a text a person reads
// cannot be written inline where a converted sink sends it: the call site has
// to name a key, and the key has to have a template in every catalog.
type Text struct{ s string }

// String returns the rendered text.
func (t Text) String() string {
	return t.s
}

// T renders key with args in the language ctx carries. It is [New] followed by
// [Message.Text].
func T(ctx context.Context, key Key, args ...Arg) Text {
	return New(key, args...).Text(ctx)
}

// Verbatim wraps s as a Text without a catalog entry.
//
// It exists for tests, which hand a sink a text of their own to check what
// the sink does with it. Production code never calls it: a text built this
// way is the same in every language, which is what the type exists to stop.
func Verbatim(s string) Text {
	return Text{s: s}
}
