// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Command orbit_fixtures provisions the Orbit live test fixtures on a GitLab
// group and waits for the Orbit indexer to reach them. It is the Go port of
// scripts/setup-orbit-fixtures.sh and of the indexer wait the make target
// orbit-wait-indexer ran in shell and Python (issue 1114, item 7), and it sits
// beside cmd/gen_orbit_record, which records what Orbit answers over the same
// fixtures.
//
// setup creates the kg-fixtures and security-fixtures projects under the
// fixture namespace from test/fixtures/orbit, and with -mirror-cli a mirror
// of gitlab-org/cli, and reconciles what already exists: every fixture file
// git tracks that a project lacks is added in one commit, with the bytes
// committed at HEAD, and each project is given the configured visibility. It
// creates the labels, milestone, issues and environments of kg-fixtures that
// are missing, and makes sure the fixture merge request from
// feature/restock-helper is merged: an open one is merged once GitLab has
// checked it, a closed one is reopened while its branch is there and opened
// again when it is gone, with none at all one is opened from the branch, a
// branch a failed run left being opened as it is, and a merge GitLab refuses
// or answers unmerged stops the run (issue 1428). A non-public visibility is refused under jmrp-oss,
// whose projects must all be public, before anything is sent and again on the
// group GitLab resolves the namespace to.
//
// wait-indexer reads the Orbit indexing state of the fixture namespace through
// GitLab's GraphQL API every 15 seconds until it is READY with no project left
// out and at least the two fixture projects indexed, or until the timeout
// allows no further read, and then proceeds with a warning, as the live suite
// skips what it cannot ask.
//
// Both read GITLAB_COM_TOKEN, ORBIT_FIXTURES_NAMESPACE (jmrp-oss/orbit-fixtures
// by default) and ORBIT_FIXTURES_GITLAB_URL (https://gitlab.com by default);
// setup also reads ORBIT_FIXTURES_VISIBILITY (public by default) and
// ORBIT_FIXTURES_MERGE_POLL_SECONDS, the seconds between two reads of the
// fixture merge request while GitLab checks it (2 by default, a fraction
// allowed), and wait-indexer ORBIT_FIXTURES_INDEXER_TIMEOUT, in seconds (600
// by default).
// make orbit-setup-fixtures and make orbit-wait-indexer pass them on from the
// make command line, the environment or .env.
//
// Usage:
//
//	go run ./cmd/orbit_fixtures/ setup [-mirror-cli] [-skip-mirror] [-root dir]
//	go run ./cmd/orbit_fixtures/ wait-indexer
//
// -root names the repository root that holds test/fixtures/orbit, the
// working directory by default.
package main
