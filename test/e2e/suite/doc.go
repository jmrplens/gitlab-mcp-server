// Package suite contains end-to-end tests for the GitLab MCP server.
//
// Tests run against a live GitLab instance via the MCP server's tool, resource,
// and prompt surfaces using an in-memory MCP transport. Each test creates real
// GitLab resources (projects, groups, branches, issues, merge requests,
// pipelines, packages, and others) through MCP calls and verifies the expected
// outcome before tearing the resources down again.
//
// # Editions and Modes
//
// The suite is split across GitLab editions and tooling modes:
//
//   - CE (community edition) — files suffixed `_ce_test.go` exercise behavior
//     common to all GitLab installations. Selected with the build constraint
//     `e2e && !enterprise`.
//   - EE (enterprise edition) — files suffixed `_ee_test.go` exercise features
//     that require GitLab Enterprise (epics, group protected branches,
//     iterations, SAML/SCIM, compliance frameworks, security policies).
//     Selected with the build constraint `e2e && enterprise`. When the running
//     GitLab reports CE, EE tests skip instead of failing.
//
// Each domain is exercised under multiple MCP surfaces where applicable:
//
//   - dynamic — the two-tool `find` / `execute` surface (always on by default).
//   - meta — the catalog-backed single `gitlab_*` meta-tool surface.
//   - individual — the flat per-tool surface that registers every GitLab tool
//     as its own MCP tool.
//   - safe mode — the read-only surface that returns previews for mutating
//     tools instead of applying them.
//   - elicitation — an alternate client capability variant.
//
// # Execution Model
//
// The suite starts an in-memory MCP client and server pair, then drives GitLab
// operations through MCP calls instead of calling handlers directly:
//
//	E2E test
//	    |
//	    v
//	MCP client session
//	    |
//	    v
//	MCP server tools, resources, prompts, and capabilities
//	    |
//	    v
//	Real or Docker-managed GitLab instance
//
// Tests named TestIndividual_* exercise individual tools. Tests named
// TestMeta_* exercise the meta-tool catalog against the same style of GitLab
// fixtures. Tests prefixed TestCapability_*, TestSchema_*, TestSafeMode,
// TestOAuthE2E, TestElicitation, and TestIdentityE2E exercise
// MCP-layer behavior rather than a specific GitLab API.
//
// # Resource Naming and Cleanup
//
// Resource names are generated with deterministic, GitLab-safe identifiers so
// parallel runs can isolate projects, groups, and branches. When E2E_RUN_ID is
// set, the suite uses it as the run scope; otherwise it derives a fresh run ID
// from the current time, process ID, and an atomic counter. Fixtures register
// themselves with a per-test [ResourceLedger] so [ResourceLedger.CleanupAll]
// deletes every created resource when the test exits, even on failure.
//
// # Running
//
// These tests are gated by the `e2e` build tag and are normally run through
// `make test-e2e` (self-hosted, reads `.env`) or `make test-e2e-docker`
// (ephemeral GitLab CE container with fixture service). The full Docker suite
// also drives the CI runner and pipeline tools that require a registered
// runner. In self-hosted mode the suite snapshots all pre-existing groups and
// projects before the run and verifies they remain unchanged after the run
// finishes, catching tests that accidentally modify or delete resources they
// do not own.
package suite
