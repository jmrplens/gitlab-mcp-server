// Command audit_doc_tool_names checks every `gitlab_*` tool name the
// documentation mentions against the names the server actually registers.
//
// cmd/audit_doc_coverage already audits the docs against the canonical action
// catalog, but it compares `domain.action` IDs — so a page can name a tool that
// no surface has ever registered and still audit clean. That is exactly how
// `gitlab_list_issues` survived in guides and examples: the individual surface
// projects domain-first names (`gitlab_issue_list`), so every copy-pasted
// verb-first example answered `unknown tool` at runtime.
//
// The name set is built in memory from the same registration paths the server
// uses, across the individual, meta and dynamic surfaces at the Ultimate tier,
// so it needs no network and cannot drift from the catalog.
//
// Usage:
//
//	go run ./cmd/audit_doc_tool_names/           # report
//	go run ./cmd/audit_doc_tool_names/ --check   # non-zero exit on any finding
package main
