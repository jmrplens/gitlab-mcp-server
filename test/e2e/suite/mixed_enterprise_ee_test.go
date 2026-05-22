//go:build e2e

// mixed_enterprise_ee_test.go exposes Enterprise-only selectors for tests that
// still live in broader CE/common files while they are migrated to dedicated
// *_ee_test.go files.
package suite

import (
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groups"
)

func TestEE_MetaProjectServiceAccounts(t *testing.T) {
	if !sess.enterprise {
		return
	}
	TestMeta_ProjectServiceAccounts(t)
}

func TestEE_MetaProjectSecuritySettings(t *testing.T) {
	if !sess.enterprise {
		return
	}
	TestMeta_ProjectSecuritySettings(t)
}

func TestEE_MetaProjectMirroring(t *testing.T) {
	if !sess.enterprise {
		return
	}
	TestMeta_ProjectMirroring(t)
}

func TestEE_MetaIssueWorkItems(t *testing.T) {
	if !sess.enterprise {
		return
	}
	TestMeta_IssueWorkItems(t)
}

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
