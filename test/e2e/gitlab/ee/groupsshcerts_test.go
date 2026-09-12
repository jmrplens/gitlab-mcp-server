//go:build e2e

// groupsshcerts_test.go covers a group's SSH certificates: a fresh group
// has none, a certificate minted from a fresh key is listed once it is
// created, and deleted it is listed no more.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupsshcerts"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// sshCertIDs lists the ids of a certificate listing.
func sshCertIDs(certificates []groupsshcerts.Output) []int64 {
	ids := make([]int64, 0, len(certificates))
	for _, certificate := range certificates {
		ids = append(ids, certificate.ID)
	}
	return ids
}

// TestGroupSSHCerts_Lifecycle_CreateListDelete creates one certificate per
// surface in a group of the surface's own, so the empty listing before and
// the one-entry listing after are exact, and deletes it.
//
// Replaces: TestMeta_GroupSSHCerts, TestEE_MetaGroupEnterpriseOperations
func TestGroupSSHCerts_Lifecycle_CreateListDelete(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("sshcert"))
		params := map[string]any{"group_id": group.IDParam()}

		empty := harness.Do[groupsshcerts.ListOutput](s, actionGroupSSHCertList, params)
		if len(empty.Certificates) != 0 {
			e.T.Errorf("a fresh group lists %d SSH certificate(s): %v", len(empty.Certificates), sshCertIDs(empty.Certificates))
		}

		key, err := fixture.SSHPublicKey()
		if err != nil {
			e.T.Fatal(err)
		}
		title := e.Name("cert")
		created := harness.Do[groupsshcerts.Output](s, actionGroupSSHCertCreate, withParams(params, map[string]any{"key": key, "title": title}))
		if created.ID == 0 || created.Title != title {
			e.T.Fatalf("ssh_cert_create answered %+v, want a certificate titled %q with an ID", created, title)
		}

		listed := harness.Do[groupsshcerts.ListOutput](s, actionGroupSSHCertList, params)
		if ids := sshCertIDs(listed.Certificates); len(ids) != 1 || ids[0] != created.ID {
			e.T.Errorf("the group lists the certificates %v, want exactly the created %d", ids, created.ID)
		}

		harness.DoVoid(s, actionGroupSSHCertDelete, withParams(params, map[string]any{"certificate_id": created.ID}))
		after := harness.Do[groupsshcerts.ListOutput](s, actionGroupSSHCertList, params)
		if containsID(sshCertIDs(after.Certificates), created.ID) {
			e.T.Errorf("the group still lists certificate %d after its delete", created.ID)
		}
	})
}
