//go:build e2e

// actions_b7_group_test.go names the catalog actions the group family's own
// scenarios call and that no other file here already names.
//
// They are typed constants for the reason actions_test.go gives: the
// push-time static gate reads constants of the harness's ActionID type out of
// the type checker's record and holds each against the catalog and against
// this package's tier, so a string literal at a call site would be invisible
// to it. Every action named here is Free, which is what the common package
// may name.

package common

import "github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"

// A group's badges. The whole sub-family, since nothing here drove the group
// half of the badge domain before.
const (
	actionGroupBadgeAdd     harness.ActionID = "group.badge_add"
	actionGroupBadgeGet     harness.ActionID = "group.badge_get"
	actionGroupBadgeList    harness.ActionID = "group.badge_list"
	actionGroupBadgeEdit    harness.ActionID = "group.badge_edit"
	actionGroupBadgePreview harness.ActionID = "group.badge_preview"
	actionGroupBadgeDelete  harness.ActionID = "group.badge_delete"
)

// The single-label read and the subscription pair that hangs off it.
const (
	actionGroupLabelGet         harness.ActionID = "group.group_label_get"
	actionGroupLabelSubscribe   harness.ActionID = "group.group_label_subscribe"
	actionGroupLabelUnsubscribe harness.ActionID = "group.group_label_unsubscribe"
)

// A group milestone's update and the two listings scoped to it. The burndown
// listing beside them is Premium and belongs to the ee package.
const (
	actionGroupMilestoneUpdate        harness.ActionID = "group.group_milestone_update"
	actionGroupMilestoneIssues        harness.ActionID = "group.group_milestone_issues"
	actionGroupMilestoneMergeRequests harness.ActionID = "group.group_milestone_merge_requests"
)

// The two single-member reads: the direct one, which sees only what the group
// itself grants, and the inherited one, which sees what an ancestor grants.
const (
	actionGroupMemberGet          harness.ActionID = "group.group_member_get"
	actionGroupMemberGetInherited harness.ActionID = "group.group_member_get_inherited"
)

// What a group is found by and what it holds.
const (
	actionGroupSearch   harness.ActionID = "group.search"
	actionGroupProjects harness.ActionID = "group.projects"
)
