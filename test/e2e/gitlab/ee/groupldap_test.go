//go:build e2e

// groupldap_test.go covers a group's LDAP links and the LDAP sync. A link
// is a record GitLab keeps about a provider it will consult later, so the
// link actions answer on any licensed instance whether or not a directory
// is reachable; the sync is what needs group sync configured on the
// instance, and the Docker stack does not configure it, so it is refused
// and the refusal is asserted.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupldap"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The provider the Docker stack configures and the access a link grants.
const (
	ldapProvider    = "ldapmain"
	ldapGroupAccess = int64(30)
)

// ldapLinkCNs lists the common names of a link listing.
func ldapLinkCNs(links []groupldap.Output) []string {
	cns := make([]string, 0, len(links))
	for _, link := range links {
		cns = append(cns, link.CN)
	}
	return cns
}

// containsCN reports whether a listing holds a link by common name.
func containsCN(links []groupldap.Output, cn string) bool {
	for _, link := range links {
		if link.CN == cn {
			return true
		}
	}
	return false
}

// TestGroupLDAPLinks_Lifecycle_AddListDeleteAndDeleteForProvider adds a
// link on every surface in a group of its own, finds it in the listing,
// deletes it by common name, then adds another and deletes it by provider.
//
// Replaces: TestMeta_GroupLDAPLinks, TestEE_MetaGroupEnterpriseOperations
func TestGroupLDAPLinks_Lifecycle_AddListDeleteAndDeleteForProvider(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("ldap"))
		params := map[string]any{"group_id": group.IDParam()}
		cn := e.Name("cn")

		empty := harness.Do[groupldap.ListOutput](s, actionGroupLDAPLinkList, params)
		if len(empty.Links) != 0 {
			e.T.Errorf("a fresh group lists %d LDAP link(s): %v", len(empty.Links), ldapLinkCNs(empty.Links))
		}

		added := harness.Do[groupldap.Output](s, actionGroupLDAPLinkAdd, withParams(params, map[string]any{
			"cn": cn, "group_access": ldapGroupAccess, "provider": ldapProvider,
		}))
		if added.CN != cn || added.Provider != ldapProvider {
			e.T.Errorf("ldap_link_add answered %+v, want the link %q on %q", added, cn, ldapProvider)
		}
		listed := harness.Do[groupldap.ListOutput](s, actionGroupLDAPLinkList, params)
		if !containsCN(listed.Links, cn) {
			e.T.Errorf("the group's LDAP links do not hold %q: %v", cn, ldapLinkCNs(listed.Links))
		}

		harness.DoVoid(s, actionGroupLDAPLinkDelete, withParams(params, map[string]any{"cn": cn, "provider": ldapProvider}))
		after := harness.Do[groupldap.ListOutput](s, actionGroupLDAPLinkList, params)
		if containsCN(after.Links, cn) {
			e.T.Errorf("the group's LDAP links still hold %q after its delete", cn)
		}

		byProvider := e.Name("provider-cn")
		harness.DoVoid(s, actionGroupLDAPLinkAdd, withParams(params, map[string]any{
			"cn": byProvider, "group_access": ldapGroupAccess, "provider": ldapProvider,
		}))
		harness.DoVoid(s, actionGroupLDAPLinkDeleteForProvider, withParams(params, map[string]any{"cn": byProvider, "provider": ldapProvider}))
		final := harness.Do[groupldap.ListOutput](s, actionGroupLDAPLinkList, params)
		if containsCN(final.Links, byProvider) {
			e.T.Errorf("the group's LDAP links still hold %q after the delete by provider", byProvider)
		}
	})
}

// TestGroupLDAPSync_NoGroupSync_IsRefusedWithTheProviderHint asks a fresh
// group to sync with LDAP on every surface and checks the refusal names
// what a caller has to have for it: the provider, the tier and the access.
//
// Replaces: TestMeta_GroupLDAPSync
func TestGroupLDAPSync_NoGroupSync_IsRefusedWithTheProviderHint(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("ldapsync"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)
		refused := harness.Refused(s, actionGroupLDAPSync, map[string]any{"group_id": group.IDParam()}, harness.FailureNotFound)
		assertMentions(e, "the LDAP sync refusal", refused, "LDAP provider", "Premium/Ultimate", "Owner access")
	})
}
