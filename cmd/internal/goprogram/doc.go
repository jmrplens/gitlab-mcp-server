// Package goprogram is the one go/packages front end for the gates that
// type-check this repository's own source.
//
// Four of them do it: cmd/audit_md_escaping walks the calls that interpolate
// a GitLab-authored value into Markdown, cmd/audit_readonly_graphql walks the
// calls a read-only action can reach, cmd/internal/graphqldocs folds every raw
// GraphQL document to the one string GitLab would receive, and
// cmd/audit_graphql_shapes pairs each of those documents with the struct that
// decodes it. They ask four different questions of the loaded program and
// index it four different ways, which is why only the front end lives here:
// the load mode, the config, and the rule that a package which did not
// type-check stops the run.
//
// That last rule is why this package exists rather than being four tidy
// copies. Each of the four gates answers "cannot tell" for anything it cannot
// resolve, and a partially typed package resolves nothing: the escaping audit
// would classify every value as unfollowable, the read-only audit would find
// no handlers, and the document collector and the shape audit would fold no
// constants. All four would then report a clean run over source they never
// understood, which is the one failure mode a gate must not have. The refusal
// was written four times with four wordings, so a change to it was a four-file
// edit with one file easy to forget.
//
// What stays with each gate is everything above the load: its own indexers,
// its own detectors, its own question and its own binary.
//
// [github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/shared.LoadToolPackages]
// is deliberately not folded in. It loads with NeedDeps, so it pays for the
// dependency tree these four refuse to pay for, and it filters rather than
// refuses: it returns the tool packages that typed and drops the ones that did
// not. That is a different contract, not a different wording of this one.
package goprogram
