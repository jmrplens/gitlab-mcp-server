// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package hints collects the canonical action IDs a package's Markdown
// invites a model to call, by rendering the package's own formatters rather
// than by reading its constants: a hint spelled as a literal at the call site
// is published exactly like one behind a constant, and a test that reads only
// the constants cannot see it.
//
// It sits beside [testutil] rather than inside it because it needs
// [toolutil], and internal/toolutil's own tests use testutil. A non-test
// import of toolutil from testutil closes that loop and every package in it
// fails to build its tests with "import cycle not allowed in test", which is
// what happened the first time this helper was written.
package hints
