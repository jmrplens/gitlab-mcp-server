//go:build e2e && !enterprise

// groups_ce_test.go tests the group MCP tools against a live GitLab instance.
// Covers create, list, get, members, subgroups, and delete for both individual and meta-tool modes.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestIndividual_Groups exercises group CRUD using individual MCP tools.
//
// The test creates a unique top-level group fixture (deferred deletion),
// then runs subtests covering group_create, group_list, group_get, group
// members, subgroups, and group_delete through the individual
// gitlab_group_* tools. Each subtest asserts the expected ID or name
// round-trips through the GitLab API.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_Groups(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	groupPath := fmt.Sprintf("e2e-grp-%d", time.Now().UnixMilli())
	var groupID int64

	t.Cleanup(func() {
		if groupID > 0 {
			cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cleanCancel()
			_ = callToolVoidOn(cleanCtx, sess.individual, "gitlab_group_delete", groups.DeleteInput{
				GroupID: toolutil.StringOrInt(strconv.FormatInt(groupID, 10)),
			})
		}
	})

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[groups.Output](ctx, sess.individual, "gitlab_group_create", groups.CreateInput{
			Name:       groupPath,
			Path:       groupPath,
			Visibility: "public",
		})
		requireNoError(t, err, "group create")
		requireTruef(t, out.ID > 0, "group ID should be positive")
		groupID = out.ID
		t.Logf("Created group %d (%s)", out.ID, out.FullPath)
	})

	t.Run("List", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groups.ListOutput](ctx, sess.individual, "gitlab_group_list", groups.ListInput{
			Search: groupPath,
		})
		requireNoError(t, err, "group list")
		requireTruef(t, len(out.Groups) > 0, "expected at least 1 group")
		t.Logf("Found %d groups matching %q", len(out.Groups), groupPath)
	})

	t.Run("Get", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		gid := strconv.FormatInt(groupID, 10)
		out, err := callToolOn[groups.Output](ctx, sess.individual, "gitlab_group_get", groups.GetInput{
			GroupID: toolutil.StringOrInt(gid),
		})
		requireNoError(t, err, "group get")
		requireTruef(t, out.ID == groupID, "expected group ID %d, got %d", groupID, out.ID)
		t.Logf("Group %d: %s (visibility=%s)", out.ID, out.FullPath, out.Visibility)
	})

	t.Run("MembersList", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		gid := strconv.FormatInt(groupID, 10)
		out, err := callToolOn[groups.MemberListOutput](ctx, sess.individual, "gitlab_group_members_list", groups.MembersListInput{
			GroupID: toolutil.StringOrInt(gid),
		})
		requireNoError(t, err, "group members list")
		t.Logf("Group %d has %d members", groupID, len(out.Members))
	})

	t.Run("SubgroupsList", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		gid := strconv.FormatInt(groupID, 10)
		out, err := callToolOn[groups.ListOutput](ctx, sess.individual, "gitlab_subgroups_list", groups.SubgroupsListInput{
			GroupID: toolutil.StringOrInt(gid),
		})
		requireNoError(t, err, "subgroups list")
		t.Logf("Group %d has %d subgroups", groupID, len(out.Groups))
	})

	t.Run("SharedWithList", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		gid := strconv.FormatInt(groupID, 10)
		out, err := callToolOn[groups.ListOutput](ctx, sess.individual, "gitlab_group_shared_with_list", groups.SharedWithListInput{
			GroupID: toolutil.StringOrInt(gid),
		})
		requireNoError(t, err, "group shared_with list")
		t.Logf("Group %d has %d groups shared with it", groupID, len(out.Groups))
	})

	t.Run("InvitedList", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		gid := strconv.FormatInt(groupID, 10)
		out, err := callToolOn[groups.ListOutput](ctx, sess.individual, "gitlab_group_invited_list", groups.InvitedListInput{
			GroupID: toolutil.StringOrInt(gid),
		})
		requireNoError(t, err, "group invited list")
		t.Logf("Group %d has %d invited groups", groupID, len(out.Groups))
	})

	t.Run("TransferLocations", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		gid := strconv.FormatInt(groupID, 10)
		out, err := callToolOn[groups.TransferLocationsListOutput](ctx, sess.individual, "gitlab_group_transfer_locations", groups.TransferLocationsListInput{
			GroupID: toolutil.StringOrInt(gid),
		})
		requireNoError(t, err, "group transfer_locations")
		t.Logf("Group %d has %d candidate transfer locations", groupID, len(out.Locations))
	})

	t.Run("Delete", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		gid := strconv.FormatInt(groupID, 10)
		err := callToolVoidOn(ctx, sess.individual, "gitlab_group_delete", groups.DeleteInput{
			GroupID: toolutil.StringOrInt(gid),
		})
		requireNoError(t, err, "group delete")
		t.Logf("Deleted group %d", groupID)
		groupID = 0
	})
}

// TestIndividual_GroupNewV241Fields verifies the client-go v2.41.0 group
// create options and the enriched group Output fields round-trip end-to-end:
// it creates a group setting the new boolean options that are available in all
// tiers (math rendering limits, personal snippets), then reads the group back
// and asserts the new Output fields (archived, math_rendering_limits_enabled,
// duo_availability, enabled_git_access_protocol, organization_id) are accessible.
//
// The create flags that are Premium/Ultimate-only (web-based commit signing,
// the unique-project-download-limit cluster) are intentionally not set here so
// the test stays green on a CE instance.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_GroupNewV241Fields(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	path := fmt.Sprintf("e2e-grp-v241-%d", time.Now().UnixMilli())
	enabled := true
	var groupID int64
	t.Cleanup(func() {
		if groupID > 0 {
			cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cleanCancel()
			_ = callToolVoidOn(cleanCtx, sess.individual, "gitlab_group_delete", groups.DeleteInput{
				GroupID: toolutil.StringOrInt(strconv.FormatInt(groupID, 10)),
			})
		}
	})

	created, err := callToolOn[groups.Output](ctx, sess.individual, "gitlab_group_create", groups.CreateInput{
		Name:                       path,
		Path:                       path,
		Visibility:                 "public",
		MathRenderingLimitsEnabled: &enabled,
		AllowPersonalSnippets:      &enabled,
	})
	requireNoError(t, err, "group create with v2.41.0 fields")
	requireTruef(t, created.ID > 0, "created group ID should be positive")
	groupID = created.ID

	got, err := callToolOn[groups.Output](ctx, sess.individual, "gitlab_group_get", groups.GetInput{
		GroupID: toolutil.StringOrInt(strconv.FormatInt(groupID, 10)),
	})
	requireNoError(t, err, "group get after create")
	requireTruef(t, !got.Archived, "freshly created group should not be archived")
	t.Logf("Enriched group fields: archived=%v math_rendering=%v duo_availability=%q git_access_protocol=%q organization_id=%d",
		got.Archived, got.MathRenderingLimitsEnabled, got.DuoAvailability, got.EnabledGitAccessProtocol, got.OrganizationID)
}

// TestMeta_Groups exercises group operations using the gitlab_group
// meta-tool.
//
// The test mirrors [TestIndividual_Groups] but drives every step with
// {action, params} arguments through the catalog-backed gitlab_group
// tool. Each subtest asserts the same outcome and verifies the tool name
// stays constant across the lifecycle.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_Groups(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	groupPath := fmt.Sprintf("e2e-meta-grp-%d", time.Now().UnixMilli())
	var groupID int64

	t.Cleanup(func() {
		if groupID > 0 {
			cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cleanCancel()
			_ = callToolVoidOn(cleanCtx, sess.meta, "gitlab_group", map[string]any{
				"action": "delete",
				"params": map[string]any{
					"group_id": strconv.FormatInt(groupID, 10),
				},
			})
		}
	})

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "create",
			"params": map[string]any{
				"name":       groupPath,
				"path":       groupPath,
				"visibility": "public",
			},
		})
		requireNoError(t, err, "meta group create")
		requireTruef(t, out.ID > 0, "group ID should be positive")
		groupID = out.ID
		t.Logf("Created group %d via meta-tool (%s)", out.ID, out.FullPath)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[groups.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "list",
			"params": map[string]any{
				"search": groupPath,
			},
		})
		requireNoError(t, err, "meta group list")
		requireTruef(t, len(out.Groups) > 0, "expected at least 1 group")
		t.Logf("Found %d groups via meta-tool", len(out.Groups))
	})

	t.Run("Get", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "get",
			"params": map[string]any{
				"group_id": strconv.FormatInt(groupID, 10),
			},
		})
		requireNoError(t, err, "meta group get")
		requireTruef(t, out.ID == groupID, "expected group ID %d, got %d", groupID, out.ID)
		t.Logf("Group %d: %s (visibility=%s)", out.ID, out.FullPath, out.Visibility)
	})

	t.Run("MembersList", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groups.MemberListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "members",
			"params": map[string]any{
				"group_id": strconv.FormatInt(groupID, 10),
			},
		})
		requireNoError(t, err, "meta group members list")
		t.Logf("Group %d has %d members via meta-tool", groupID, len(out.Members))
	})

	t.Run("SubgroupsList", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groups.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "subgroups",
			"params": map[string]any{
				"group_id": strconv.FormatInt(groupID, 10),
			},
		})
		requireNoError(t, err, "meta subgroups list")
		t.Logf("Group %d has %d subgroups via meta-tool", groupID, len(out.Groups))
	})

	t.Run("SharedWithList", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groups.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "shared_with",
			"params": map[string]any{"group_id": strconv.FormatInt(groupID, 10)},
		})
		requireNoError(t, err, "meta group shared_with list")
		t.Logf("Group %d has %d groups shared with it via meta-tool", groupID, len(out.Groups))
	})

	t.Run("InvitedList", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groups.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "invited_groups",
			"params": map[string]any{"group_id": strconv.FormatInt(groupID, 10)},
		})
		requireNoError(t, err, "meta group invited list")
		t.Logf("Group %d has %d invited groups via meta-tool", groupID, len(out.Groups))
	})

	t.Run("TransferLocations", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groups.TransferLocationsListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "transfer_locations",
			"params": map[string]any{"group_id": strconv.FormatInt(groupID, 10)},
		})
		requireNoError(t, err, "meta group transfer_locations")
		t.Logf("Group %d has %d candidate transfer locations via meta-tool", groupID, len(out.Locations))
	})

	t.Run("Delete", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "delete",
			"params": map[string]any{
				"group_id": strconv.FormatInt(groupID, 10),
			},
		})
		requireNoError(t, err, "meta group delete")
		t.Logf("Deleted group %d via meta-tool", groupID)
		groupID = 0
	})
}
