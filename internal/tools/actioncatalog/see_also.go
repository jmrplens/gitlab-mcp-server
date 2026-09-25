package actioncatalog

import "regexp"

// SeeAlsoClause matches the cross-reference sentence an individual tool's
// Description ends with, "See also: gitlab_a, gitlab_b.", and, because the
// character class admits dots, a clause written in canonical IDs or already
// projected into a surface's entry IDs ("See also: widget.create,
// gitlab_widget.get."). The terminating literal dot still matches: the greedy
// class backtracks one character off the final name. The first submatch is
// the comma-separated list of names.
//
// It is the one definition of the clause, because two readers have to agree
// on it and disagreeing fails silently. internal/resources rewrites what it
// matches into the names of the surface a manifest serves, and
// cmd/audit_action_ids passes over what it matches when it holds a
// Description to the rule that served prose names no tool, since that rewrite
// is what makes an individual tool name right there. With a copy in each, a
// manifest pattern narrowed later would stop rewriting text the gate still
// skipped, and tool names would reach the dynamic and meta surfaces verbatim
// with the gate green.
var SeeAlsoClause = regexp.MustCompile(`See also: ([a-z0-9_.]+(?:, [a-z0-9_.]+)*)\.`)
