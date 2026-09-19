// Command audit_doc_tool_names checks every name the documentation teaches a
// reader to call against what the server really serves: the `gitlab_*` tool
// names against the names it registers, and the `domain.action` IDs against
// the catalog it builds.
//
// cmd/audit_doc_coverage already audits the docs against the canonical action
// catalog, but it asks which actions are documented rather than whether the
// documented ones exist, so a page could name a tool no surface has ever
// registered and audit clean. That is exactly how `gitlab_list_issues`
// survived in guides and examples: the individual surface projects
// domain-first names (`gitlab_issue_list`), so every copy-pasted verb-first
// example answered `unknown tool` at runtime.
//
// # Both halves of one sentence
//
// The ID rule was added because the tool rule alone left the other half of
// every such sentence unjudged. A page teaching a call names the tool on the
// individual surface and the canonical ID on the dynamic one, and the tool
// regex cannot see an ID at all: it matches `gitlab_[a-z0-9_]+` and an ID
// carries no such prefix. Eight IDs across five pages were wrong that way,
// among them a whole `pipeline_schedule.` family the CI/CD page asserted in
// both languages while the catalog spells it `pipeline.schedule_*`; each of
// those pages paired the wrong ID with the right tool name, so this command
// was green over all of them.
//
// # What counts as an ID
//
// The rule is cmd/internal/actionids, shared with `cmd/audit_action_ids`, so a
// spelling cannot be a cross-link in the code and prose in the docs. A dotted
// token is offered as an ID when either half is one the catalog uses, which
// turns away github.com and go.mod on its own. Three further shapes pass that
// test and are not IDs, and only the last is a table of instances: a file name
// whose stem is a catalog domain (issue.rb, pipeline.svg), a meta-surface
// manifest entry, whose left half the tool rule already checks
// (gitlab_merge_request.create), and the declarations in ids.go.
//
// # Two limits
//
// It answers whether a name resolves, never whether it is the right one: a
// page that tells a reader to call `issue.get` when it means `issue.list` is
// silent here.
//
// And it reads `.md` and `.mdx` under the roots in docRoots, so the generated
// `site/public/llms-*.txt` are outside it, and it skips
// `docs/development/adr/`, where an ADR's examples are a record of what was
// decided rather than instructions to follow.
//
// Usage:
//
//	go run ./cmd/audit_doc_tool_names/           # report
//	go run ./cmd/audit_doc_tool_names/ --check   # non-zero exit on any finding
package main
