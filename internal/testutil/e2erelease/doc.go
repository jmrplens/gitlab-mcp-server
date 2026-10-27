// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package e2erelease decides what an end-to-end run does on the GitLab
// release it found, measured against the release the fine-grained action
// table was recorded from (issue 1155).
//
// # Why a run is stopped at all
//
// The server judges a fine-grained token's grant only where the table applies:
// on the release it was recorded from (the grant decides the listing and the
// calls, phase B), and on the prerelease of the release right after it (the
// listing alone). On any other release it falls back to phase A. The suite
// asserts whichever phase the release decides, which is right for the server
// and wrong for the suite: once the GitLab a user installs moves past the
// table, every fine-grained scenario passes in phase A, nothing checks what a
// grant lists or may call, and the run reads exactly like one that did. So a
// run against a release newer than both judged states stops before any
// scenario reaches the instance and says how to re-record the table. A release older than the table is a
// deliberate pin rather than a stale table, and runs in phase A with one line
// saying so. A version that does not parse stops the run rather than passing
// it, since nothing about it can be judged.
//
// # Why it is a package of its own
//
// Two programs ask the question: the e2e harness, which refuses in its
// environment setup so a run against a self-hosted instance stops too, and
// cmd/audit_e2e_coverage -check-release, which test/e2e/scripts/run-docker-e2e.sh
// asks right after the instance answers its version and before go test starts.
// The rule has one implementation so the two cannot disagree, and it can only
// be shared from a package both may import: the harness is built with the e2e
// tag under test/e2e/internal, and cmd/internal is closed to it. Like
// internal/testutil/e2ecalls it is untagged, imports no testing, and must never
// reach the server binary, which cmd/server's
// TestDependencies_TestSupport_NeverReachesTheServerBinary holds by name.
//
// # Why it does not ask the server's rule
//
// It reads the release with [finegrained.Bucket], the parse the table itself is
// chosen by, and spells the prerelease after the table's release itself rather
// than calling the reading [finegrained.Judge] applies. That keeps it a second
// opinion: a Judge that let another release in, or stopped letting this one in,
// is a session the harness refuses instead of one it agrees with by
// construction.
package e2erelease
