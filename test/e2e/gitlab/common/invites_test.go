//go:build e2e

// invites_test.go covers invitations by email to a project and to a group:
// each is sent to an address nobody holds, on a domain that delivers
// nowhere, and then found pending in the listing. An invitation to an
// unregistered address is a membership waiting for a sign-up, and it goes
// away with the project or group the fixture library deletes.

package common

import (
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/invites"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// inviteStatusSuccess is the status GitLab answers a sent invitation with.
const inviteStatusSuccess = "success"

// inviteFixture is what the invitations are sent for: a project and a
// group of the test's own.
type inviteFixture struct {
	project fixture.Project
	group   fixture.Group
}

// TestInvites_ProjectAndGroup_InviteByEmailAndListPending invites one fresh
// address per surface to the project and one to the group, as developers,
// and finds each in the pending listing of its scope.
//
// Replaces: TestMeta_Invitations, TestIndividual_GroupInvitations
func TestInvites_ProjectAndGroup_InviteByEmailAndListPending(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) inviteFixture {
		return inviteFixture{
			project: fixture.NewProject(e, fixture.WithNamePrefix("invite")),
			group:   fixture.NewGroup(e, fixture.WithGroupNamePrefix("invite")),
		}
	}, func(e *harness.Env, surface harness.Surface, f inviteFixture) {
		s := e.On(surface)

		projectEmail := uniqueAddress("pinv")
		// Named in the log because GitLab validates an invitation's address
		// and refuses it without echoing it, so a refusal here says nothing
		// about which address was refused unless the test says so itself.
		e.T.Logf("inviting %s to project %d and a second address to group %d", projectEmail, f.project.ID, f.group.ID)
		sent := harness.Do[invites.InviteResultOutput](s, actionAccessInviteProject, map[string]any{
			"project_id": f.project.IDParam(), "email": projectEmail, "access_level": int(gl.DeveloperPermissions),
		})
		if sent.Status != inviteStatusSuccess {
			e.T.Errorf("invite_project answered status %q with message %v, want %q", sent.Status, sent.Message, inviteStatusSuccess)
		}
		pending := harness.Do[invites.ListPendingInvitationsOutput](s, actionAccessInviteListProject, map[string]any{"project_id": f.project.IDParam()})
		if !invitesAddress(pending.Invitations, projectEmail) {
			e.T.Errorf("the pending invitations of project %d do not hold %s: %+v", f.project.ID, projectEmail, pending.Invitations)
		}

		groupEmail := uniqueAddress("ginv")
		sentGroup := harness.Do[invites.InviteResultOutput](s, actionAccessInviteGroup, map[string]any{
			"group_id": f.group.IDParam(), "email": groupEmail, "access_level": int(gl.DeveloperPermissions),
		})
		if sentGroup.Status != inviteStatusSuccess {
			e.T.Errorf("invite_group answered status %q with message %v, want %q", sentGroup.Status, sentGroup.Message, inviteStatusSuccess)
		}
		pendingGroup := harness.Do[invites.ListPendingInvitationsOutput](s, actionAccessInviteListGroup, map[string]any{"group_id": f.group.IDParam()})
		if !invitesAddress(pendingGroup.Invitations, groupEmail) {
			e.T.Errorf("the pending invitations of group %d do not hold %s: %+v", f.group.ID, groupEmail, pendingGroup.Invitations)
		}
	})
}

// invitesAddress reports whether a pending listing holds an invitation to
// the given address.
func invitesAddress(listed []invites.PendingInviteOutput, email string) bool {
	for _, invitation := range listed {
		if invitation.InviteEmail == email {
			return true
		}
	}
	return false
}
