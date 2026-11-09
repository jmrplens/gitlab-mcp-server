// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package docblocks reads the code blocks of the documentation site, so a
// test can serve the configuration a page prints instead of a copy of it.
//
// A test that embeds its own copy of a documented configuration stays green
// while the page drifts: the copy is what runs, and the page is what a reader
// pastes. The reverse proxy cases of test/e2e/http held copies of that kind,
// and two of them already differed from the pages they named (issue 1112).
// Reading the block from the page at test time makes the page the thing
// under test.
//
// A block is chosen by a key in its fence, the `e2e=<case>` option:
//
//	```nginx e2e=nginx
//
// Expressive Code passes over an option it does not know, so the key changes
// nothing on the rendered page, and site/scripts/check-i18n-code.mjs holds the
// fence options of a Spanish page to its English twin, so the key is on both
// fences or the site check fails. [Page.Block] asks the same of the Spanish
// page on its own, so a test reports a key the twin lost without waiting for
// the site check to run.
//
// Fences are found the way check-i18n-code.mjs finds them: three or more
// backticks or tildes at any indentation, closed by a run of the same
// character at least as long, each body line losing the indentation of the
// opening fence, after the page's frontmatter. One rule is stricter: a fence
// left open is an error here, where the site check runs it to the end of the
// page, because a block that silently became the rest of the page is not the
// block a test asked for.
//
// The package imports nothing but the standard library, so any test may use
// it, and cmd/server/main_test.go keeps it out of the server binary.
package docblocks
