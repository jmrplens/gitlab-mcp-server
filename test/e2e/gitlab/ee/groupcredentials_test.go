//go:build e2e

// groupcredentials_test.go covers the group credential inventory on an
// instance that does not expose it, which a self-managed Ultimate instance
// does not: the four actions are refused as not found, and each refusal is
// held to the hint that says the endpoints may be absent and what the
// inventory needs when they are there.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// missingCredentialID is a token or key number nothing has, for the two
// writes that name one.
const missingCredentialID = int64(999999)

// TestGroupCredentials_InventoryUnserved_EveryActionIsRefusedWithTheHint
// drives the two listings and the two revocations of the credential
// inventory on a fresh group on every surface and checks each is refused
// as not found with the hint naming the endpoints, the tier and the access.
//
// Replaces: TestEE_MetaGroupEnterpriseOperations
func TestGroupCredentials_InventoryUnserved_EveryActionIsRefusedWithTheHint(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.Tier(edition.Ultimate)))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("creds"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)
		params := map[string]any{"group_id": group.IDParam()}
		hint := []string{"group credential inventory", "/groups/:id/manage", "Ultimate", "Owner or admin"}

		refused := harness.Refused(s, actionGroupCredentialListPATs, params, harness.FailureNotFound)
		assertMentions(e, "the token inventory listing", refused, hint...)
		refused = harness.Refused(s, actionGroupCredentialListSSHKeys, params, harness.FailureNotFound)
		assertMentions(e, "the key inventory listing", refused, hint...)
		refused = harness.Refused(s, actionGroupCredentialRevokePAT, withParams(params, map[string]any{"token_id": missingCredentialID}), harness.FailureNotFound)
		assertMentions(e, "the token revocation", refused, "credential_list_pats", "group credential inventory", "Ultimate", "Owner or admin")
		refused = harness.Refused(s, actionGroupCredentialDeleteSSHKey, withParams(params, map[string]any{"key_id": missingCredentialID}), harness.FailureNotFound)
		assertMentions(e, "the key deletion", refused, "credential_list_ssh_keys", "group credential inventory", "Ultimate", "Owner or admin")
	})
}
