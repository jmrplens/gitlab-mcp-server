// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package committext holds what text that becomes a commit message on main,
// or a release built from one, may not carry, for the two gates that judge
// such text: cmd/audit_pr_text, over a pull request's title and body, which a
// squash merge makes the commit message, and cmd/audit_supply_chain, over the
// summary of a release, which is pasted into the pull request that moves
// VERSION and published as the release body.
//
// It exists because the two gates used to carry a copy each, and the copies
// carried the same defect: the skip-checks trailer was matched as a prefix,
// so "skip-checks: trueish" was refused as if GitHub would honor it, which
// refuses more than it must and lets nothing more through. One definition is
// one place to get that right (issue 1114, item 11).
//
// The characters are written as escapes, so this package does not carry what
// it refuses.
package committext
