//go:build e2e

// actions_B6_test.go names the catalog actions the MCP-layer batch (B6) drives
// and that actions_test.go does not already declare.
//
// They are typed harness.ActionID constants for the same reason the ones in
// actions_test.go are: the push-time static gate reads constants of that type
// out of the type checker's record and follows them through the harness verbs,
// so a literal at a call site would be invisible to it and a renamed action
// would fail to compile in one place rather than silently.

package common

import "github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"

// The project listing the dynamic find and execute workflow drives for the
// pagination block a list answer carries. The file read and the merge request
// listing it also drives are declared by the repository and merge request
// batches of this same package.
const actionProjectListB6 harness.ActionID = "project.list"

// actionDiscoverProjectResolve is the standalone utility that resolves a git
// remote URL to a project. It is registered outside the base catalog, so the
// dynamic test reaches it through [harness.ExecuteStandalone] rather than the
// projecting verbs.
const actionDiscoverProjectResolve harness.ActionID = "discover_project.resolve"

// The pipeline and job waits, and the job list the wait test reads to find a
// job to wait on. They need an instance CI runner to reach a terminal state.
const (
	actionPipelineWait harness.ActionID = "pipeline.wait"
	actionJobWait      harness.ActionID = "job.wait"
)
