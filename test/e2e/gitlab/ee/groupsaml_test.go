//go:build e2e

// groupsaml_test.go covers a group's SAML links and its SAML users on an
// instance with no SSO provider, which is what the Docker stack is: the
// four link actions are refused as unauthorized, and each refusal is held
// to the hint that says what the group lacks, while the user listing
// answers with nobody, since SAML provisioned no one.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupsaml"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// samlLinkAccessLevel is the access a link would grant: Developer.
const samlLinkAccessLevel = int64(30)

// TestGroupSAMLLinks_NoSSO_EveryActionIsRefusedWithTheSSOHint drives the
// list, add, get and delete of a SAML link on a fresh group on every
// surface and checks each is refused, naming SSO, the tier and the access.
//
// Replaces: TestMeta_GroupSAML, TestEE_MetaGroupEnterpriseOperations
func TestGroupSAMLLinks_NoSSO_EveryActionIsRefusedWithTheSSOHint(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("saml"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)
		params := map[string]any{"group_id": group.IDParam()}
		named := withParams(params, map[string]any{"saml_group_name": e.Name("saml")})
		hint := []string{"group SAML SSO", "Premium/Ultimate", "Owner access", "401 or 404"}

		refused := harness.Refused(s, actionGroupSAMLLinkList, params, harness.FailureForbidden)
		assertMentions(e, "the SAML link listing", refused, hint...)
		refused = harness.Refused(s, actionGroupSAMLLinkAdd, withParams(named, map[string]any{"access_level": samlLinkAccessLevel}), harness.FailureForbidden)
		assertMentions(e, "the SAML link add", refused, hint...)
		refused = harness.Refused(s, actionGroupSAMLLinkGet, named, harness.FailureForbidden)
		assertMentions(e, "the SAML link read", refused, hint...)
		refused = harness.Refused(s, actionGroupSAMLLinkDelete, named, harness.FailureForbidden)
		assertMentions(e, "the SAML link delete", refused, hint...)
	})
}

// TestGroupSAMLUsers_NoSSO_ListsNobody lists the SAML users of a fresh
// group on every surface with every filter the action offers, and checks
// the answer names nobody.
//
// Replaces: TestMeta_GroupSAMLUsers
func TestGroupSAMLUsers_NoSSO_ListsNobody(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("samlusers"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)
		listed := harness.Do[groupsaml.SAMLUsersListOutput](s, actionGroupSAMLUsersList, map[string]any{
			"group_id": group.IDParam(), "search": "nobody", "active": true,
			"created_after": "2026-01-01T00:00:00Z", "created_before": "2026-12-31T23:59:59Z",
		})
		if len(listed.Users) != 0 {
			e.T.Errorf("a group with no SSO lists %d SAML user(s): %+v", len(listed.Users), listed.Users)
		}
	})
}
