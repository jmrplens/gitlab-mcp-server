// Package apiexposes reads GitLab's Grape entities out of its Ruby source and
// records, for every field an entity exposes, the condition under which
// GitLab sends it.
//
// The OpenAPI record (cmd/internal/apishapes) is an upper bound: it lists a
// field an entity declares with `expose :x, if: ->(...) { ... }` as if it were
// always sent, because the generated document cannot see the condition. The
// licensed run showed what that hides: nine approval-configuration fields
// the record listed that a live GitLab never sends, and Enterprise fields a
// Community instance never sends. This package reads the source the document
// was generated from, so a reader can tell "never sent" from "sent only when",
// and a condition naming a licensed feature is resolved to the tier that
// feature belongs to through GitLab's own feature table.
//
// # What it reads
//
// Three directories of gitlab-org/gitlab: lib/api/entities (the Community
// entities), ee/lib/api/entities (entities only Enterprise declares) and
// ee/lib/ee/api/entities (the Enterprise modules prepended into Community
// entities, whose `prepended do` blocks add fields to them), plus
// ee/app/models/gitlab_subscriptions/features.rb, which lists every licensed
// feature symbol under the tier that unlocks it.
//
// # What it understands
//
// The Grape DSL as GitLab writes it: `expose :a, :b, as:, if:, unless:,
// using:, with:, merge:, documentation:` with the statement continued over as
// many lines as it takes; `expose :x do ... end` blocks that nest fields under
// x, told apart from `expose :x do |obj| ... end` blocks that compute x;
// `with_options if: ... do ... end` scopes; class inheritance, whose parent's
// fields come first; and `merge: true`, which inlines the used entity's
// fields. Method bodies and value blocks are skipped statement by statement,
// with every `do`, `if`, `def` and `end` tracked so the skip ends where Ruby
// ends it. This is a scanner over rubocop-formatted source, not a Ruby
// parser: a construct it does not know is left where it is, never guessed
// at, and the counts a regeneration prints are what say whether the source
// still looks like the one this was written against.
//
// # What it does not judge
//
// A condition is recorded as written, and only the licensed feature symbols
// in it (`feature_available?(:x)`, `licensed_feature_available?(:x)`,
// `License.feature_available?(:x)`) are read further, into the tier the
// feature table puts them under. Everything else a condition depends on, an
// option the caller passed, a permission, an instance setting, stays prose
// for a reviewer to read.
package apiexposes
