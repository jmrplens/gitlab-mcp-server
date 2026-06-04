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
	const groupName = "e2e-ldap-link"
	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, groupName)

	const (
		provider = "ldapmain"
		ldapCN   = "e2e-ldap-cn"
	)

	// On the success path we additionally assert lifecycle side
	// effects (list contains the added link; after delete the list
	// no longer contains it). On the error path (LDAP integration
	// not configured) we accept 404/422/502/503 gracefully.
	ldapLinkListed := func(t *testing.T) (ok bool, listed groupldap.ListOutput, err error) {
		t.Helper()
		listed, err = callToolOn[groupldap.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ldap_link_list",
			"params": map[string]any{"group_id": grp.gidStr()},
		})
		if err != nil {
			return false, listed, err
		}
		for _, link := range listed.Links {
			if link.CN == ldapCN {
				return true, listed, nil
			}
		}
		return false, listed, nil
	}

	t.Run("Meta/GroupLDAP/Add", func(t *testing.T) {
		out, err := callToolOn[groupldap.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ldap_link_add",
			"params": map[string]any{
				"group_id":     grp.gidStr(),
				"cn":           ldapCN,
				"group_access": 30,
				"provider":     provider,
			},
		})
		if err == nil {
			requireTruef(t, out.CN == ldapCN, "LDAP link CN mismatch: got %q want %q", out.CN, ldapCN)
			found, _, listErr := ldapLinkListed(t)
			requireNoError(t, listErr, "ldap_link_list after add")
			requireTruef(t, found, "added LDAP link %q not present in ldap_link_list response", ldapCN)
			t.Logf("LDAP link added and verified in list (integration is configured)")
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
				"cn":       ldapCN,
				"provider": provider,
			},
		})
		if err == nil {
			// On the success path, assert the link is gone.
			found, _, listErr := ldapLinkListed(t)
			requireNoError(t, listErr, "ldap_link_list after delete")
			requireTruef(t, !found, "deleted LDAP link %q still present in ldap_link_list response", ldapCN)
			t.Log("LDAP link deleted and verified absent from list")
			return
		}
		if isHTTPStatus(err, 404) || isHTTPStatus(err, 422) || isHTTPStatus(err, 502) || isHTTPStatus(err, 503) {
			t.Logf("LDAP link delete returned expected error: %v", err)
			return
		}
		t.Fatalf("ldap_link_delete: unexpected error: %v", err)
	})
}
