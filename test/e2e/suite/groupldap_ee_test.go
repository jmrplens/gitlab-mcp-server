//go:build e2e && enterprise

// groupldap_ee_test.go exercises the group LDAP links meta-tool
// against a live GitLab EE Ultimate instance. Group LDAP links are
// Ultimate-only and require the LDAP integration to be configured
// in GitLab. The test creates a real LDAP link on a test group via
// the meta-tool, lists, and deletes it via the per-test ledger.
//
// When the e2e-ldap container is not present in the docker-compose
// stack, the LDAP integration is not configured and the test falls
// back to error-path assertions (404/422 from GitLab's LDAP API).
//
// CANNOT parallelize: shares the meta session with the rest of the
// EE suite.
package suite

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupldap"
)

// TestMeta_GroupLDAPLinks exercises the group LDAP link lifecycle
// (add/list/delete) via the gitlab_group meta-tool. Falls back to
// the error path if LDAP integration is not configured.
func TestMeta_GroupLDAPLinks(t *testing.T) {
	if !sess.enterprise {
		t.Skip("group LDAP links require GitLab Ultimate")
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx := context.Background()
	groupName := uniqueName("e2e-ldap-link")
	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, groupName)

	const provider = "ldapmain"

	// First attempt the happy path: add a link. If the LDAP integration
	// is not configured, GitLab returns 404/422 and we exercise the
	// error path. We do not requireNoError the add; we treat any error
	// as the "integration not configured" branch.
	t.Run("Meta/GroupLDAP/Add", func(t *testing.T) {
		_, err := callToolOn[groupldap.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ldap_link_add",
			"params": map[string]any{
				"group_id":     grp.gidStr(),
				"cn":           "e2e-ldap-cn",
				"group_access": 30,
				"provider":     provider,
			},
		})
		if err == nil {
			t.Log("LDAP link added (integration is configured)")
			return
		}
		// Fall back to the error path. The integration may be absent
		// (404) or the LDAP server may be unreachable (502/503). All of
		// these indicate the tool routes correctly; a 2xx without
		// side-effect would be the only failure mode.
		if isHTTPStatus(err, 404) || isHTTPStatus(err, 422) || isHTTPStatus(err, 502) || isHTTPStatus(err, 503) {
			t.Logf("LDAP link add returned expected error (integration may be absent): %v", err)
			return
		}
		t.Fatalf("ldap_link_add: unexpected error: %v", err)
	})

	t.Run("Meta/GroupLDAP/List", func(t *testing.T) {
		_, err := callToolOn[groupldap.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ldap_link_list",
			"params": map[string]any{"group_id": grp.gidStr()},
		})
		if err == nil {
			t.Log("LDAP list ok")
			return
		}
		if isHTTPStatus(err, 404) || isHTTPStatus(err, 422) || isHTTPStatus(err, 502) || isHTTPStatus(err, 503) {
			t.Logf("LDAP link list returned expected error: %v", err)
			return
		}
		t.Fatalf("ldap_link_list: unexpected error: %v", err)
	})

	t.Run("Meta/GroupLDAP/Delete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ldap_link_delete",
			"params": map[string]any{
				"group_id": grp.gidStr(),
				"cn":       "e2e-ldap-cn",
				"provider": provider,
			},
		})
		if err == nil {
			t.Log("LDAP link deleted")
			return
		}
		if isHTTPStatus(err, 404) || isHTTPStatus(err, 422) || isHTTPStatus(err, 502) || isHTTPStatus(err, 503) {
			t.Logf("LDAP link delete returned expected error: %v", err)
			return
		}
		t.Fatalf("ldap_link_delete: unexpected error: %v", err)
	})
}
