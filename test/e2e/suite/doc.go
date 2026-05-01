// Package suite contains end-to-end tests for the GitLab MCP server.
//
// The suite exercises individual tools, meta-tools, resources, prompts, and
// MCP capabilities against a real or Docker-managed GitLab instance using an
// in-memory MCP transport. Tests create isolated GitLab projects, groups,
// branches, issues, merge requests, pipelines, packages, and other resources,
// then clean them up through shared fixture and ledger helpers.
//
// Resource names are generated with deterministic, GitLab-safe identifiers so
// parallel runs can isolate projects, groups, and branches. When E2E_RUN_ID is
// set, the suite uses it as the run scope; otherwise it derives a fresh run ID
// from the current time, process ID, and an atomic counter.
//
// These tests are built with the e2e build tag and are normally run through
// `make test-e2e` or `make test-e2e-docker`.
package suite
