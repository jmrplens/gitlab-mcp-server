//go:build e2e && enterprise

// groupextras_ee_test.go covers the remaining Premium/Ultimate group-domain
// actions the e2e gap audit reported as unexercised: group push rules
// (push_rule_add/get/edit/delete), billable members
// (group_billable_members_list, group_billable_member_memberships_list,
// group_billable_member_remove), provisioned users (list_provisioned_users),
// and epic resource label events (event_epic_label_list/get). All flows run
// through the gitlab_group meta-tool against a live GitLab EE instance.
//
// Build tag: e2e && enterprise.
package suite

import (
	"context"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/grouplabels"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupmembers"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/resourceevents"
)

// geExtrasCreateUser creates a disposable user through the raw admin API and
// registers a best-effort deletion cleanup. Billable-member coverage needs a
// second, removable user because the group creator is the last Owner and
// GitLab refuses to remove the last Owner from the billable list.
func geExtrasCreateUser(ctx context.Context, t *testing.T, prefix string) *gl.User {
	t.Helper()
	uname := uniqueName(prefix)
	opts := &gl.CreateUserOptions{
		Email:            new(uname + "@e2e-test.local"),
		Name:             new("E2E " + uname),
		Username:         new(uname),
		Password:         new("E2eT!Gx9K#p2mNq$8BcZ"),
		SkipConfirmation: new(true),
	}
	user, _, err := sess.glClient.GL().Users.CreateUser(opts, gl.WithContext(ctx))
	requireNoError(t, err, "create disposable user for billable members")
	t.Cleanup(func() { //nolint:contextcheck // Cleanup runs after the test context is canceled and owns its own timeout.
		cctx, cancel := cleanupContext(defaultCleanupTimeout)
		defer cancel()
		_, _ = sess.glClient.GL().Users.DeleteUser(user.ID, gl.WithContext(cctx))
	})
	return user
}

// geExtrasBillableUnavailable reports whether err represents the documented
// self-managed degradation of the billable-members API (the endpoint rejects
// namespaces without subscription-style seat accounting with 400/403/404).
func geExtrasBillableUnavailable(err error) bool {
	return isHTTPStatus(err, 400) || isHTTPStatus(err, 403) || isHTTPStatus(err, 404)
}

// TestMeta_GroupPushRules exercises group push rule CRUD through the
// gitlab_group meta-tool against a live GitLab Premium/Ultimate instance.
//
// The test creates a top-level group fixture and walks push_rule_add,
// push_rule_get, push_rule_edit, and push_rule_delete. Assertions verify the
// singleton rule round-trips max_file_size through the GitLab API and that
// the edit updates the persisted value.
//
// Build tag: e2e && enterprise. Mode: EE. Surface: meta.
func TestMeta_GroupPushRules(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx := context.Background()
	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, "ge-pushrule")

	t.Run("Meta/GroupPushRule/Add", func(t *testing.T) {
		out, err := callToolOn[groups.PushRuleOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "push_rule_add",
			"params": map[string]any{
				"group_id":             grp.gidStr(),
				"commit_message_regex": "^[A-Z].*",
				"max_file_size":        25,
			},
		})
		requireNoError(t, err, "meta group push rule add")
		requireTruef(t, out.ID > 0, "group push rule ID should be positive, got %d", out.ID)
		t.Logf("Added group push rule %d", out.ID)
	})

	t.Run("Meta/GroupPushRule/Get", func(t *testing.T) {
		out, err := callToolOn[groups.PushRuleOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "push_rule_get",
			"params": map[string]any{
				"group_id": grp.gidStr(),
			},
		})
		requireNoError(t, err, "meta group push rule get")
		requireTruef(t, out.MaxFileSize == 25, "expected max_file_size=25, got %d", out.MaxFileSize)
		t.Logf("Got group push rules: max_file_size=%d", out.MaxFileSize)
	})

	t.Run("Meta/GroupPushRule/Edit", func(t *testing.T) {
		out, err := callToolOn[groups.PushRuleOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "push_rule_edit",
			"params": map[string]any{
				"group_id":      grp.gidStr(),
				"max_file_size": 75,
			},
		})
		requireNoError(t, err, "meta group push rule edit")
		requireTruef(t, out.MaxFileSize == 75, "expected max_file_size=75 after edit, got %d", out.MaxFileSize)
		t.Logf("Edited group push rules: max_file_size=%d", out.MaxFileSize)
	})

	t.Run("Meta/GroupPushRule/Delete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "push_rule_delete",
			"params": map[string]any{
				"group_id": grp.gidStr(),
			},
		})
		requireNoError(t, err, "meta group push rule delete")
		t.Log("Deleted group push rules")
	})
}

// TestMeta_GroupBillableMembers exercises the billable-member actions through
// the gitlab_group meta-tool against a live GitLab Premium/Ultimate instance.
//
// The test creates a top-level group, probes group_billable_members_list, and
// when the API is available adds a disposable user as Developer, verifies it
// appears in the billable list, lists that member's memberships, and removes
// it. When the endpoint rejects self-managed namespaces without
// subscription-style seat accounting (400/403/404), each action is still
// invoked and the clean error path is asserted instead.
//
// Build tag: e2e && enterprise. Mode: EE. Surface: meta. Admin token required.
func TestMeta_GroupBillableMembers(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin}, func(e2e *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		defer cancel()

		grp := CreateGroupMeta(ctx, e2e, sess.meta, "ge-billable")

		probeOut, probeErr := callToolOn[groupmembers.BillableMembersOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_billable_members_list",
			"params": map[string]any{
				"group_id": grp.gidStr(),
			},
		})
		if probeErr != nil {
			if !geExtrasBillableUnavailable(probeErr) {
				t.Fatalf("group_billable_members_list: unexpected error: %v", probeErr)
			}
			t.Logf("billable members API unavailable on this instance (documented self-managed degradation): %v", probeErr)

			// The sibling endpoints share the same availability gate; assert
			// they degrade with the same clean error instead of succeeding.
			_, memErr := callToolOn[groupmembers.BillableMembershipsOutput](ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "group_billable_member_memberships_list",
				"params": map[string]any{
					"group_id": grp.gidStr(),
					"user_id":  int64(1),
				},
			})
			requireTruef(t, memErr != nil && geExtrasBillableUnavailable(memErr),
				"memberships_list should degrade like the list probe, got %v", memErr)

			remErr := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "group_billable_member_remove",
				"params": map[string]any{
					"group_id": grp.gidStr(),
					"user_id":  int64(1),
				},
			})
			requireTruef(t, remErr != nil && geExtrasBillableUnavailable(remErr),
				"billable member remove should degrade like the list probe, got %v", remErr)
			return
		}
		requireTruef(t, len(probeOut.Members) >= 1, "expected at least the group owner in billable members, got %d", len(probeOut.Members))
		t.Logf("Billable members before add: %d", len(probeOut.Members))

		user := geExtrasCreateUser(ctx, t, "ge-billable-user")
		_, addErr := callToolOn[groupmembers.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_member_add",
			"params": map[string]any{
				"group_id":     grp.gidStr(),
				"user_id":      user.ID,
				"access_level": 30,
			},
		})
		requireNoError(t, addErr, "add disposable user as group Developer")

		t.Run("Meta/Billable/ListIncludesMember", func(t *testing.T) {
			out, err := retryWithBackoff(ctx, t, "billable members list", 5, func(int) (groupmembers.BillableMembersOutput, bool, string, error) {
				out, err := callToolOn[groupmembers.BillableMembersOutput](ctx, sess.meta, "gitlab_group", map[string]any{
					"action": "group_billable_members_list",
					"params": map[string]any{
						"group_id": grp.gidStr(),
						"search":   user.Username,
					},
				})
				if err != nil {
					return out, isRetryableError(err), "transient billable list error", err
				}
				if len(out.Members) == 0 {
					return out, true, "billable membership not visible yet", errBillableMemberNotVisible
				}
				return out, false, "", nil
			})
			requireNoError(t, err, "billable members list after add")
			requireTruef(t, out.Members[0].Username == user.Username,
				"billable member username = %q, want %q", out.Members[0].Username, user.Username)
			t.Logf("Billable member visible: %s (removable=%v)", out.Members[0].Username, out.Members[0].Removable)
		})

		t.Run("Meta/Billable/Memberships", func(t *testing.T) {
			out, err := callToolOn[groupmembers.BillableMembershipsOutput](ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "group_billable_member_memberships_list",
				"params": map[string]any{
					"group_id": grp.gidStr(),
					"user_id":  user.ID,
				},
			})
			requireNoError(t, err, "billable member memberships list")
			requireTruef(t, len(out.Memberships) >= 1, "expected at least 1 membership for billable member, got %d", len(out.Memberships))
			t.Logf("Billable member %s has %d membership(s), first source: %s",
				user.Username, len(out.Memberships), out.Memberships[0].SourceFullName)
		})

		t.Run("Meta/Billable/Remove", func(t *testing.T) {
			err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "group_billable_member_remove",
				"params": map[string]any{
					"group_id": grp.gidStr(),
					"user_id":  user.ID,
				},
			})
			requireNoError(t, err, "billable member remove")
			t.Logf("Removed billable member %s", user.Username)
		})
	})
}

// errBillableMemberNotVisible marks the retryable "membership not indexed
// yet" state while polling the billable member list.
var errBillableMemberNotVisible = geExtrasError("billable member not visible yet")

// geExtrasError is a trivial error string type so retry loops in this file
// have sentinels without pulling in fmt for constant messages.
type geExtrasError string

func (e geExtrasError) Error() string { return string(e) }

// TestMeta_GroupProvisionedUsers exercises list_provisioned_users through the
// gitlab_group meta-tool against a live GitLab Premium/Ultimate instance.
//
// The Docker EE fixture has no SAML/SCIM provider, so the endpoint is
// expected to succeed with an empty list; an empty result is a valid
// exercise of the action per the gap-audit plan. Filter parameters are
// passed to exercise option mapping.
//
// Build tag: e2e && enterprise. Mode: EE. Surface: meta.
func TestMeta_GroupProvisionedUsers(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx := context.Background()
	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, "ge-provisioned")

	out, err := callToolOn[groups.ProvisionedUsersListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "list_provisioned_users",
		"params": map[string]any{
			"group_id": grp.gidStr(),
			"order_by": "id",
			"sort":     "asc",
		},
	})
	requireNoError(t, err, "list provisioned users")
	// No SCIM/SAML on the ephemeral instance: an empty list proves the route
	// and option mapping without needing an identity provider fixture.
	t.Logf("Provisioned users in group %s: %d", grp.Path, len(out.Users))
}

// TestMeta_GroupEpicLabelEvents exercises event_epic_label_list and
// event_epic_label_get through the gitlab_group meta-tool against a live
// GitLab Premium/Ultimate instance.
//
// The test creates a group, a group label, and an epic, then adds and
// removes the label on the epic to generate resource label events. It lists
// the events with retry (event writes can lag the GraphQL label mutation)
// and fetches one event by ID. When the pinned GitLab version has removed
// the legacy epic resource-label-events REST endpoint (404), the test
// asserts the clean error path on both actions and skips with the
// version-gated reason.
//
// Build tag: e2e && enterprise. Mode: EE. Surface: meta.
func TestMeta_GroupEpicLabelEvents(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, "ge-epiclabel")

	labelOut, labelErr := callToolOn[grouplabels.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "group_label_create",
		"params": map[string]any{
			"group_id": grp.gidStr(),
			"name":     uniqueName("ge-epic-label"),
			"color":    "#FF0000",
		},
	})
	requireNoError(t, labelErr, "create group label for epic label events")

	epicIID := createEpicInGroup(ctx, t, e2e, grp.Path, "ge-epiclabel-epic")

	// Add then remove the label so at least two label events exist.
	_, updErr := callToolOn[map[string]any](ctx, sess.meta, "gitlab_epic", map[string]any{
		"action": "epic_update",
		"params": map[string]any{
			"full_path":     grp.Path,
			"epic_iid":      epicIID,
			"add_label_ids": []int64{labelOut.ID},
		},
	})
	requireNoError(t, updErr, "add label to epic")
	_, updErr = callToolOn[map[string]any](ctx, sess.meta, "gitlab_epic", map[string]any{
		"action": "epic_update",
		"params": map[string]any{
			"full_path":        grp.Path,
			"epic_iid":         epicIID,
			"remove_label_ids": []int64{labelOut.ID},
		},
	})
	requireNoError(t, updErr, "remove label from epic")

	var eventID int64
	var endpointGone bool

	t.Run("Meta/EpicLabelEvent/List", func(t *testing.T) {
		out, err := retryWithBackoff(ctx, t, "epic label events list", 6, func(int) (resourceevents.ListLabelEventsOutput, bool, string, error) {
			out, err := callToolOn[resourceevents.ListLabelEventsOutput](ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "event_epic_label_list",
				"params": map[string]any{
					"group_id": grp.gidStr(),
					"epic_iid": epicIID,
				},
			})
			if err != nil {
				if isHTTPStatus(err, 404) {
					// Legacy epic REST endpoints are being removed as epics
					// migrate to work items; do not burn retries on 404.
					return out, false, "", err
				}
				return out, isRetryableError(err), "transient epic label event error", err
			}
			if len(out.Events) == 0 {
				return out, true, "label events not recorded yet", geExtrasError("epic label events empty")
			}
			return out, false, "", nil
		})
		if err != nil {
			if isHTTPStatus(err, 404) {
				endpointGone = true
				t.Logf("epic resource label events endpoint returned 404 (legacy epic REST removed on this GitLab version): %v", err)
				return
			}
			t.Fatalf("event_epic_label_list: %v", err)
		}
		eventID = out.Events[0].ID
		t.Logf("Epic label events: %d (first ID %d, action %s)", len(out.Events), eventID, out.Events[0].Action)
	})

	t.Run("Meta/EpicLabelEvent/Get", func(t *testing.T) {
		if endpointGone {
			// Exercise the get action's clean error path so the sibling
			// route is validated even without the legacy endpoint.
			_, err := callToolOn[resourceevents.LabelEventOutput](ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "event_epic_label_get",
				"params": map[string]any{
					"group_id":       grp.gidStr(),
					"epic_iid":       epicIID,
					"label_event_id": int64(999999999),
				},
			})
			requireTruef(t, err != nil, "event_epic_label_get should fail when the endpoint is unavailable")
			t.Skipf("epic label events unavailable on this GitLab version; get error path validated: %v", err)
		}
		requireTruef(t, eventID > 0, "eventID not set by the list subtest")
		out, err := callToolOn[resourceevents.LabelEventOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "event_epic_label_get",
			"params": map[string]any{
				"group_id":       grp.gidStr(),
				"epic_iid":       epicIID,
				"label_event_id": eventID,
			},
		})
		requireNoError(t, err, "event_epic_label_get")
		requireTruef(t, out.ID == eventID, "label event ID = %d, want %d", out.ID, eventID)
		t.Logf("Got epic label event %d (%s)", out.ID, out.Action)
	})
}
