// Command audit_md_cards judges the shape of every Markdown response this
// server can produce: whether a one-object card is written as the bulleted
// field list the rule calls for, whether a table is a real table, and whether
// any response mixes the two into a block that renders as neither.
//
// It renders rather than reads. Every formatter is registered against the type
// it formats, so the audit builds a sample value of each registered type and
// asks the registry for its Markdown, exactly as a tool call does; what it
// then judges is the finished string. A gate that read the source instead
// would have to model conditionals, loops and the helpers a card is assembled
// out of across three packages, and it would still answer worse: the defect
// this audit exists for — a pipe row in a bulleted card, a card field inside a
// table — is a property of the lines that come out, and the lines that come
// out are what the model on the other end reads.
//
// Two samples are rendered for each type: one with every field zero, which is
// the response GitLab produces for a sparsely populated resource and the one
// most formatters have a branch for, and one with every field populated, which
// is the response that writes every conditional line. A type whose formatter
// panics on either is reported as unrenderable rather than as a failure, the
// way its sibling audit reports a value it cannot follow: a gate's own blind
// spot is worth listing and is not evidence of a defect.
//
// Usage:
//
//	go run ./cmd/audit_md_cards/                # report
//	go run ./cmd/audit_md_cards/ -check         # CI gate, non-zero on a finding
//	go run ./cmd/audit_md_cards/ -census        # the table/card census by package
//	go run ./cmd/audit_md_cards/ -json out.json # the work list as JSON
//
// Exit codes follow cmd/audit_md_escaping: 0 clean, 1 the gate found
// something, 2 the audit could not do its job. A gate that cannot run must not
// read as a gate that passed.
package main
