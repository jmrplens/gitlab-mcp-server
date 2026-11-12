// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package committext

import "regexp"

// EmDash is U+2014, the character nothing written in this repository uses:
// not code, not a comment, not documentation, not a commit message. It is
// built from its code point, so this package does not carry the character it
// refuses.
const EmDash = string(rune(0x2014))

// SkipCommand matches the five bracketed commands GitHub documents for
// skipping every workflow a push or a pull request would trigger, when the
// commit carries one in its message. It matches anywhere in a line, without
// regard to case and with any spacing inside the brackets, which costs
// nothing and does not rely on GitHub matching only the spelling it
// documents. Backticks around one do not stop GitHub reading it, so they do
// not stop this.
var SkipCommand = regexp.MustCompile(`(?i)\[\s*(?:skip\s+ci|ci\s+skip|no\s+ci|skip\s+actions|actions\s+skip)\s*\]`)

// SkipChecksTrailer matches a line GitHub reads as a skip-checks trailer:
// at the start of the line only, indented or in backticks included, since
// GitHub reads it as a trailer and prose may name it mid-sentence; and on
// any line rather than the last, since a description is not the end of the
// squash commit's message and where GitHub honors the trailer is not
// something to rely on. Its value is the whole word true, followed by
// whitespace, a closing backtick or the end of the line: a word that only
// starts with true is not the trailer.
var SkipChecksTrailer = regexp.MustCompile("(?i)^\\s*`?skip-checks\\s*:\\s*true(?:\\s|`|$)")
