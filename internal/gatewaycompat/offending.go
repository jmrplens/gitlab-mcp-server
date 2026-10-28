// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package gatewaycompat

import (
	"slices"
	"unicode"
)

// offendingASCII lists the ASCII characters gateway validators are known to
// reject in served text. Anything above U+007F is rejected as a class, not
// listed here: the served-prose policy is pure ASCII, because a validator
// that refuses "unsafe characters" usually matches a character class rather
// than a codepoint list, and a class is a property the next gateway cannot
// surprise.
var offendingASCII = []rune{';'}

// OffendingRune reports whether r is a character the served-prose policy
// refuses: one of the ASCII characters gateway validators are known to reject,
// or any rune above U+007F.
//
// It is the one statement of that policy. cmd/audit_gateway_chars judges every
// listed text with it, and the message catalog's character policy applies it
// to the English the server shows a person, so the two cannot drift apart.
func OffendingRune(r rune) bool {
	return r > unicode.MaxASCII || slices.Contains(offendingASCII, r)
}
