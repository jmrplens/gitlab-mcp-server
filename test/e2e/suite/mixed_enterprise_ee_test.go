//go:build e2e && enterprise

// mixed_enterprise_ee_test.go covers Enterprise workflows that share helpers
// with broader CE/common files.
//
// Build tag: e2e && enterprise.
package suite

import (
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groups"
)

// TestEE_MetaGroupEnterpriseOperations exercises Enterprise-only group
// workflows through the gitlab_group meta-tool and the shared board /
// Enterprise helper suites. The test creates a fresh group, then drives the
// EE-only board, board list, and SAML/SCIM/SSHCerts/iterations/protected
// branch helpers against it, asserting that every operation succeeds against
// the live GitLab EE instance.
//
// The test boots the meta MCP session via [sess.meta], creates a uniquely
// named group via gitlab_group{action=create}, then defers deletion through
// gitlab_group{action=delete} on the same session. Each EE helper verifies a
// distinct subdomain (boards, SAML, SCIM, iterations, protected branches,
// group SSH certificates) against the new group.
//
// The test is skipped on self-hosted runs that detected CE via
// [sess.enterprise]. Build tag: e2e. Mode: EE. Surface: meta.
func TestEE_MetaGroupEnterpriseOperations(t *testing.T) {
	if !sess.enterprise {
		return
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := e2eTimeoutContext(180*time.Second, 420*time.Second)
	defer cancel()

	groupName := uniqueName("grp-ee")
	out, err := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name": groupName,
			"path": groupName,
		},
	})
	requireNoError(t, err, "group create for Enterprise group operations")
	groupIDStr := strconv.FormatInt(out.ID, 10)
	defer func() {
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "delete",
			"params": map[string]any{"group_id": groupIDStr},
		})
	}()

	runMetaGroupBoardOperations(t, ctx, out.ID, groupIDStr)
	runMetaGroupEnterpriseOperations(t, ctx, groupName, out.ID, groupIDStr)
}
