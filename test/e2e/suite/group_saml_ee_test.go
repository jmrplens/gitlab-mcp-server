//go:build e2e && enterprise

// group_saml_ee_test.go exercises the group SAML links meta-tool
// against a live GitLab EE Ultimate instance. Group SAML is
// Premium/Ultimate-only. Without an actual SSO provider configured
// in GitLab, the SAML endpoints return 404/422. The test asserts
// the error path is surfaced cleanly (the tool routes correctly
// even when SAML is unavailable in the test environment).
//
// CANNOT parallelize: shares the meta session with the rest of the
// EE suite.
package suite

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupsaml"
)

// TestMeta_GroupSAML exercises the group SAML actions on a fresh
// group. Without an SSO configured, the actions return 404/422;
// the test asserts the tool routes the calls correctly in both
// branches.
func TestMeta_GroupSAML(t *testing.T) {
	if !sess.enterprise {
		t.Skip("group SAML requires GitLab Premium/Ultimate")
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx := context.Background()
	groupName := uniqueName("e2e-saml")
	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, groupName)

	samlGroup := "e2e-saml-group"

	t.Run("Meta/GroupSAML/List_Graceful401", func(t *testing.T) {
		out, err := callToolOn[groupsaml.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "saml_link_list",
			"params": map[string]any{"group_id": grp.gidStr()},
		})
		if err != nil {
			// Self-managed GitLab without SAML SSO configured returns 401
			// (or 404 on some versions) from the SAML endpoints. Both are
			// acceptable error paths for the tool routing assertion.
			if isHTTPStatus(err, 401) || isHTTPStatus(err, 404) {
				t.Logf("saml_link_list returned expected error (no SSO configured): %v", err)
				return
			}
			requireNoError(t, err, "saml_link_list")
		}
		t.Logf("Group %s SAML links: %d (no SSO configured is expected to be 0)", grp.Path, len(out.Links))
	})

	t.Run("Meta/GroupSAML/Get_Graceful401", func(t *testing.T) {
		_, err := callToolOn[groupsaml.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "saml_link_get",
			"params": map[string]any{
				"group_id":        grp.gidStr(),
				"saml_group_name": samlGroup,
			},
		})
		if err == nil {
			t.Fatal("saml_link_get for non-existent SAML group returned no error; expected 401/404")
		}
		if !isHTTPStatus(err, 401) && !isHTTPStatus(err, 404) {
			t.Fatalf("saml_link_get error was not 401/404: %v", err)
		}
		t.Logf("saml_link_get error path validated: %v", err)
	})

	t.Run("Meta/GroupSAML/Add_Graceful401", func(t *testing.T) {
		_, err := callToolOn[groupsaml.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "saml_link_add",
			"params": map[string]any{
				"group_id":        grp.gidStr(),
				"saml_group_name": samlGroup,
				"access_level":    30,
			},
		})
		if err == nil {
			// When SSO is configured the call may succeed; treat as a
			// valid routing outcome.
			t.Logf("saml_link_add succeeded (SSO is configured for this group)")
			return
		}
		if !isHTTPStatus(err, 401) && !isHTTPStatus(err, 404) && !isHTTPStatus(err, 422) {
			t.Fatalf("saml_link_add error was not 401/404/422: %v", err)
		}
		t.Logf("saml_link_add error path validated: %v", err)
	})

	t.Run("Meta/GroupSAML/Delete_Graceful401", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "saml_link_delete",
			"params": map[string]any{
				"group_id":        grp.gidStr(),
				"saml_group_name": samlGroup,
			},
		})
		if err == nil {
			// When SSO is configured the call may succeed; treat as a
			// valid routing outcome.
			t.Logf("saml_link_delete succeeded (SSO is configured for this group)")
			return
		}
		if !isHTTPStatus(err, 401) && !isHTTPStatus(err, 404) {
			t.Fatalf("saml_link_delete error was not 401/404: %v", err)
		}
		t.Logf("saml_link_delete error path validated: %v", err)
	})
}

// TestMeta_GroupSAMLUsers exercises the gitlab_group saml_users_list action
// (added in client-go v2.41.0) on a fresh group. With SAML SSO configured the
// call succeeds (a fresh group has no SAML-provisioned users yet); without SSO
// the endpoint returns 401/403/404. Both indicate correct tool routing.
//
// Build tag: e2e && enterprise. Mode: EE. Surface: meta.
func TestMeta_GroupSAMLUsers(t *testing.T) {
	if !sess.enterprise {
		t.Skip("group SAML users require GitLab Premium/Ultimate")
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx := context.Background()
	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, "e2e-saml-users")

	out, err := callToolOn[groupsaml.SAMLUsersListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "saml_users_list",
		// Exercise the v2.41.0 filter parameters (incl. RFC3339 date parsing).
		"params": map[string]any{
			"group_id":       grp.gidStr(),
			"search":         "nonexistent",
			"active":         true,
			"created_after":  "2026-01-01T00:00:00Z",
			"created_before": "2026-12-31T23:59:59Z",
		},
	})
	if err == nil {
		t.Logf("saml_users_list returned %d users (SSO is configured)", len(out.Users))
		return
	}
	if !isHTTPStatus(err, 401) && !isHTTPStatus(err, 403) && !isHTTPStatus(err, 404) {
		t.Fatalf("saml_users_list error was not 401/403/404: %v", err)
	}
	t.Logf("saml_users_list error path validated: %v", err)
}
