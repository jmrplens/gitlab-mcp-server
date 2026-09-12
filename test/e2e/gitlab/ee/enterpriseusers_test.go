//go:build e2e

// enterpriseusers_test.go covers the enterprise user actions on a group
// that manages no user: the listing answers empty, plain and filtered, and
// the read, the 2FA reset and the delete of a user the group does not
// manage are refused with the hint naming the listing.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/enterpriseusers"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestEnterpriseUsers_UnmanagedGroup_ListsNothingAndRefusesAMissingUser
// lists a fresh group's enterprise users on every surface, with and without
// filters, and asks about a user nothing manages through get, disable_2fa
// and delete.
//
// Replaces: TestMeta_EnterpriseUsers
func TestEnterpriseUsers_UnmanagedGroup_ListsNothingAndRefusesAMissingUser(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("entusers"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)
		hint := []string{"user_id", "gitlab_enterprise_user", "enterprise namespace"}

		listed := harness.Do[enterpriseusers.ListOutput](s, actionEnterpriseUserList, map[string]any{"group_id": group.IDParam()})
		if len(listed.Users) != 0 {
			e.T.Errorf("a group that manages nobody lists %d enterprise users: %+v", len(listed.Users), listed.Users)
		}
		filtered := harness.Do[enterpriseusers.ListOutput](s, actionEnterpriseUserList, map[string]any{
			"group_id": group.IDParam(), "search": "e2e-nonexistent-enterprise-user", "active": true, "two_factor": "disabled", "per_page": 10,
		})
		if len(filtered.Users) != 0 {
			e.T.Errorf("the filtered listing holds %d enterprise users: %+v", len(filtered.Users), filtered.Users)
		}

		refused := harness.Refused(s, actionEnterpriseUserGet, map[string]any{"group_id": group.IDParam(), "user_id": missingID}, harness.FailureNotFound)
		assertMentions(e, "the read of an unmanaged user", refused, hint...)

		refused = harness.Refused(s, actionEnterpriseUserDisable2FA, map[string]any{"group_id": group.IDParam(), "user_id": missingID}, harness.FailureNotFound)
		assertMentions(e, "the 2FA reset of an unmanaged user", refused, hint...)

		refused = harness.Refused(s, actionEnterpriseUserDelete, map[string]any{"group_id": group.IDParam(), "user_id": missingID, "hard_delete": false}, harness.FailureNotFound)
		assertMentions(e, "the delete of an unmanaged user", refused, "user_id", "gitlab_enterprise_user", "irreversible")
	})
}
