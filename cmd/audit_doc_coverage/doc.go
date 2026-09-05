// Command audit_doc_coverage reports per-doc-file gaps between
// docs/reference/tools/<doc>.md and the canonical action catalog. It produces
// plan/docs-tools-backlog.json (gitignored) listing, per file:
//
//   - missing: catalog tools expected in the doc but not documented
//   - orphan: tool headings/rows in the doc that do not belong there
//   - tier_mismatch: best-effort detection of catalog tier vs doc tier badge
//   - count_doc vs count_catalog vs readme_count for triple-cross-check
//
// The auditor exits non-zero under -check when any file has missing,
// orphan, or tier_mismatch findings, providing a CI gate that mirrors
// audit-discovery-check / audit-action-spec-coverage.
//
// Usage:
//
//	go run ./cmd/audit_doc_coverage/                     # full report to stdout
//	go run ./cmd/audit_doc_coverage/ -gaps-only          # only files with findings
//	go run ./cmd/audit_doc_coverage/ -output plan/docs-tools-backlog.json
//	go run ./cmd/audit_doc_coverage/ -check              # CI gate
package main
