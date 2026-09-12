//go:build e2e

// groupscim_test.go covers the group SCIM identity actions on a group with
// no SAML provider: the listing answers empty, and the read, update and
// delete of an identity nothing provisioned are refused with the hint that
// says where identities come from.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupscim"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestGroupSCIM_UnprovisionedGroup_ListsNothingAndRefusesAMissingIdentity
// lists a fresh group's SCIM identities on every surface and asks for one
// by a uid nothing provisioned, through get, update and delete.
//
// Replaces: TestMeta_GroupSCIM
func TestGroupSCIM_UnprovisionedGroup_ListsNothingAndRefusesAMissingIdentity(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("scim"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)
		hint := []string{"uid", "gitlab_group_scim", "SCIM provisioning"}
		uid := e.Name("scim")

		listed := harness.Do[groupscim.ListOutput](s, actionGroupSCIMList, map[string]any{"group_id": group.IDParam()})
		if len(listed.Identities) != 0 {
			e.T.Errorf("a group with no SAML provider lists %d SCIM identities: %+v", len(listed.Identities), listed.Identities)
		}

		refused := harness.Refused(s, actionGroupSCIMGet, map[string]any{"group_id": group.IDParam(), "uid": uid}, harness.FailureNotFound)
		assertMentions(e, "the read of a missing identity", refused, hint...)

		refused = harness.Refused(s, actionGroupSCIMUpdate, map[string]any{"group_id": group.IDParam(), "uid": uid, "extern_uid": e.Name("extern")}, harness.FailureNotFound)
		assertMentions(e, "the update of a missing identity", refused, hint...)

		refused = harness.Refused(s, actionGroupSCIMDelete, map[string]any{"group_id": group.IDParam(), "uid": uid}, harness.FailureNotFound)
		assertMentions(e, "the delete of a missing identity", refused, hint...)
	})
}
