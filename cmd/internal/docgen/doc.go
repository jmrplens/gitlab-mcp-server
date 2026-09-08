// Package docgen contains helpers for generated project documentation.
//
// The package renders source-readable Markdown tables and normalizes existing
// GitHub-flavored Markdown pipe tables while preserving fenced code blocks,
// escaped pipe characters, inline code spans, Unicode cell widths, and original
// line endings. Command packages use these helpers when refreshing README and
// docs content so generated files stay stable in diffs and easy to review as
// plain text.
//
// It also owns the two ways a command puts bytes on disk, kept apart because
// they answer different questions. [WriteOrCheck] is the whole-file freshness
// convention for a committed artifact: one file mode, one directory mode, one
// line-ending-agnostic comparison, one trailing newline and one "is stale; run
// …" sentence. [WriteReport] is the "-" means stdout convention for an
// auditor's -output flag, where nothing is committed and nothing is compared.
//
// Neither absorbs [ReplaceSection] and [ComputeReplacedSection], which rewrite
// a marked region of a hand-written file rather than owning the whole of it;
// what is generated there is a section, and the rest of the file is somebody's
// prose.
package docgen
