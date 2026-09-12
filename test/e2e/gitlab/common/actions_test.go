//go:build e2e

// actions_test.go names every catalog action this package calls, once.
//
// They are typed constants rather than string literals at the call sites
// because the push-time static gate reads constants of the harness's
// ActionID type out of the type checker's record, follows them through
// helper parameters, and checks each against the catalog and against the
// package's tier. A literal would be invisible to it. Keeping them together
// also means a renamed action fails to compile in one place.

package common

import "github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"

// The Free actions the MCP-layer tests drive.
const (
	// actionServerStatus is the diagnostics read every surface serves.
	actionServerStatus harness.ActionID = "server.status"
	// actionServerHealthCheck is its twin that declares no individual tool.
	actionServerHealthCheck harness.ActionID = "server.health_check"
	// actionUserCurrent reads the authenticated user, which is how a session
	// running with another credential proves whose it is.
	actionUserCurrent harness.ActionID = "user.current"
	// actionIssueList is the read the protective modes are shown to keep.
	actionIssueList harness.ActionID = "issue.list"
	// actionIssueCreate is the write the protective modes are shown to stop.
	actionIssueCreate harness.ActionID = "issue.create"
	// actionProjectGet is a read of the World's project.
	actionProjectGet harness.ActionID = "project.get"
	// actionProjectDelete is the destructive action safe mode previews.
	actionProjectDelete harness.ActionID = "project.delete"
	// actionAdminMetadataGet belongs to the admin group, which a token
	// without admin_mode is not served.
	actionAdminMetadataGet harness.ActionID = "admin.metadata_get"
)
