//go:build e2e && enterprise

// groupsshcerts_ee_test.go exercises the group SSH certificates
// meta-tool against a live GitLab EE Ultimate instance. Group SSH
// certificates are Premium/Ultimate-only. The test creates a
// real cert via the meta-tool, reads it back, lists, and deletes
// it via the per-test ledger.
//
// CANNOT parallelize: shares the meta session with the rest of the
// EE suite; tolerates EE-only feature flag activation timing.
package suite

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupsshcerts"
)

// TestMeta_GroupSSHCerts exercises group SSH certificate lifecycle
// (create/list/delete) via the gitlab_group meta-tool.
func TestMeta_GroupSSHCerts(t *testing.T) {
	if !sess.enterprise {
		t.Skip("group SSH certificates require GitLab Premium/Ultimate")
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx := context.Background()
	groupName := uniqueName("e2e-ssh-cert")
	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, groupName)

	var certID int64

	t.Run("Meta/SSHCert/Create", func(t *testing.T) {
		out, err := callToolOn[groupsshcerts.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ssh_cert_create",
			"params": map[string]any{
				"group_id": grp.gidStr(),
				"key":      generateED25519AuthorizedKey(t, "e2e-group-ssh-cert"),
				"title":    uniqueName("group-ssh-cert"),
			},
		})
		requireNoError(t, err, "ssh_cert_create")
		requireTruef(t, out.ID > 0, "expected SSH certificate ID > 0")
		certID = out.ID
		t.Logf("Created group SSH certificate %d", certID)
	})

	t.Run("Meta/SSHCert/List", func(t *testing.T) {
		requireTruef(t, certID > 0, "certID not set (Create must run first)")
		out, err := callToolOn[groupsshcerts.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ssh_cert_list",
			"params": map[string]any{"group_id": grp.gidStr()},
		})
		requireNoError(t, err, "ssh_cert_list")
		found := false
		for _, c := range out.Certificates {
			if c.ID == certID {
				found = true
				break
			}
		}
		requireTruef(t, found, "created SSH cert ID=%d not in list", certID)
		t.Logf("Group %s has %d SSH cert(s) (created cert present)", groupName, len(out.Certificates))
	})

	t.Run("Meta/SSHCert/Delete", func(t *testing.T) {
		requireTruef(t, certID > 0, "certID not set (Create must run first)")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ssh_cert_delete",
			"params": map[string]any{
				"group_id":       grp.gidStr(),
				"certificate_id": certID,
			},
		})
		requireNoError(t, err, "ssh_cert_delete")
		// Verify the certificate is actually gone. Without this check,
		// a delete endpoint that silently no-ops would pass.
		listed, listErr := callToolOn[groupsshcerts.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ssh_cert_list",
			"params": map[string]any{"group_id": grp.gidStr()},
		})
		requireNoError(t, listErr, "ssh_cert_list after delete")
		for _, c := range listed.Certificates {
			requireTruef(t, c.ID != certID, "deleted SSH certificate %d still present in ssh_cert_list", certID)
		}
		certID = 0
		t.Logf("Deleted SSH certificate %d and verified absence from list", certID)
	})
}
