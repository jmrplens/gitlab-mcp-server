// Command audit_dead_consts reports every unexported constant in this
// repository that nothing reads.
//
// # Why the linter cannot
//
// staticcheck's unused, which golangci-lint runs as a gate on every push,
// treats a const group as one unit: a declaration whose first member is read
// is read, and the rest of it is never judged. Measured against the toolchain
// this repository pins, a package holding `const deadConst = "never used"` on
// its own is reported, and the same constant written as the second member of a
// group whose first member is used produces no issue at all. Nothing in the
// configuration changes that either, since golangci-lint v2's schema does not
// accept unused's constants-are-used setting.
//
// That would be a narrow gap somewhere else and is a wide one here, because
// the group is the prevailing shape: every domain under internal/tools writes
// its canonical action IDs as one const block, and about fifty of them write
// their assertion messages and fixture strings as another. A cross-link that
// was written down and never published leaves its ID in that block, reading
// like a live cross-reference to anyone who opens the file. The first pass of
// this rule found twenty-five such constants across the tree, every single one
// of them in a group the linter had already looked at and passed.
//
// # What it reads
//
// ./internal/... and ./cmd/..., loaded through cmd/internal/goprogram with the
// test variants included, which is the whole repository's own Go source. Test
// files are loaded because a constant a test reads is read: the action-ID
// blocks are exported to the catalog tests through an export_test.go, and a
// rule that skipped those files would report the live half of them as dead.
//
// A constant is judged by the type checker's Uses map rather than by a text
// search, so a name that also occurs in a comment, a string or another package
// does not make it look read.
//
// Loading the tests is also this rule's one blind spot, and it is worth
// knowing where it falls. Forty-odd packages hand their whole action-ID block
// to the catalog test through an export_test.go that lists every member, which
// makes every member read whether or not anything publishes it. One such list
// was carrying an ID the package names nowhere when this rule was written, and
// the rule could not have found it: what found it was reading the block. The
// alternative, skipping test files, would report the live half of those blocks
// as dead, which is a worse answer to a smaller question.
//
// # Exported constants are out of scope, and so are the platforms
//
// Only unexported constants are judged. An exported one may be read from
// anywhere, including the end-to-end packages this load leaves behind their
// build tags, so a run over these patterns could not tell a dead one from one
// it simply did not look at.
//
// The platforms are the other way round: the packages that carry a
// GOOS- or GOARCH-constrained file are read again under each operating system
// and architecture pair this project builds for, because a constant read only
// by the Windows half of a package is read, and so is one only its arm64 half
// reads, and a gate that failed a Linux amd64 run over either would be failing
// over code doing its job. Both halves of the pair are set on each reload,
// since setting the operating system alone keeps the host's architecture and
// leaves an `_arm64.go` file out exactly as the first load did. Only the
// packages whose files this load actually left out are re-read: two on a
// Linux host (cmd/server and internal/toolutil) and three on Windows, where
// the unix-only test in internal/tools/packages is left out as well.
//
// # Why it is its own command
//
// cmd/audit_action_ids already asks the neighboring question, whether a
// published action ID resolves to anything, and shares this loader. It stays
// separate because the two questions have different shapes: that one reads the
// four sites an ID reaches a model from, under internal/tools, and reports
// without gating because its findings are spread over packages no single
// change touches. This one reads every constant declaration in the repository
// and gates, because a constant nothing reads is deleted by the change that
// introduces it.
//
// Usage:
//
//	go run ./cmd/audit_dead_consts/          # report
//	go run ./cmd/audit_dead_consts/ -check   # fail on a finding (CI gate)
//	go run ./cmd/audit_dead_consts/ ./internal/tools/issues  # one package
package main
