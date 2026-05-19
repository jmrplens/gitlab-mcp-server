// Package docgen contains helpers for generated project documentation.
//
// The package renders source-readable Markdown tables and normalizes existing
// GitHub-flavored Markdown pipe tables while preserving fenced code blocks,
// escaped pipe characters, inline code spans, Unicode cell widths, and original
// line endings. Command packages use these helpers when refreshing README and
// docs content so generated files stay stable in diffs and easy to review as
// plain text.
package docgen
