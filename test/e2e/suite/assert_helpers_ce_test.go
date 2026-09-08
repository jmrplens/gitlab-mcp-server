//go:build e2e && !enterprise

// assert_helpers_ce_test.go holds the listing projections only the Community
// Edition suites read, split from assert_helpers_test.go so that they live
// behind the constraint of the tests that use them.
//
// They used to sit in the shared file, which both halves of the suite compile,
// and were therefore unused under the Enterprise tag: every *_ce_test.go
// carries `e2e && !enterprise`, so an Enterprise compile drops the only callers
// and keeps the helpers. Nothing noticed for as long as nothing compiled that
// half outside a manual run, which is what issue 570 changed. A projection an
// Enterprise suite also needs belongs in assert_helpers_test.go, as
// groupBoardIDs does.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/accessrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/accesstokens"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/awardemoji"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/badges"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/clusteragents"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/customemoji"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/deploytokens"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupmarkdownuploads"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/integrations"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/labels"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mrdraftnotes"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/resourceevents"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/usergpgkeys"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/users"
)

// The time-tracking suites send human durations ("2h", "30m") and GitLab
// answers in seconds, so the assertions name both halves once here. Only the
// CE issue and merge request suites read them, which is why they sit in this
// half rather than in the shared file.
const (
	oneHourSeconds       = 60 * 60
	twoHoursSeconds      = 2 * 60 * 60
	threeHoursSeconds    = 3 * 60 * 60
	thirtyMinutesSeconds = 30 * 60
)

// awardEmojiIDs maps an award emoji listing to the award IDs it holds. Every
// award emoji surface answers with the same list shape, so the delete
// assertions across the issue, note and merge request suites share it.
func awardEmojiIDs(out awardemoji.ListOutput) []int64 {
	ids := make([]int64, 0, len(out.AwardEmoji))
	for _, e := range out.AwardEmoji {
		ids = append(ids, e.ID)
	}
	return ids
}

// accessRequestUserIDs maps an access request listing to the user IDs that are
// still waiting. An access request is keyed by its requester, so the user ID is
// what an approval or a denial removes from the list.
func accessRequestUserIDs(out accessrequests.ListOutput) []int64 {
	ids := make([]int64, 0, len(out.AccessRequests))
	for _, r := range out.AccessRequests {
		ids = append(ids, r.ID)
	}
	return ids
}

// personalAccessTokenIDs maps a personal access token listing to the token IDs
// it holds.
func personalAccessTokenIDs(out accesstokens.ListOutput) []int64 {
	ids := make([]int64, 0, len(out.Tokens))
	for _, tok := range out.Tokens {
		ids = append(ids, tok.ID)
	}
	return ids
}

// projectBadgeIDs maps a project badge listing to the badge IDs it holds.
func projectBadgeIDs(out badges.ListProjectOutput) []int64 {
	ids := make([]int64, 0, len(out.Badges))
	for _, b := range out.Badges {
		ids = append(ids, b.ID)
	}
	return ids
}

// clusterAgentIDs maps a cluster agent listing to the agent IDs it holds.
func clusterAgentIDs(out clusteragents.ListAgentsOutput) []int64 {
	ids := make([]int64, 0, len(out.Agents))
	for _, a := range out.Agents {
		ids = append(ids, a.ID)
	}
	return ids
}

// customEmojiIDs maps a custom emoji listing to the emoji GIDs it holds.
func customEmojiIDs(out customemoji.ListOutput) []string {
	ids := make([]string, 0, len(out.Emoji))
	for _, e := range out.Emoji {
		ids = append(ids, e.ID)
	}
	return ids
}

// groupUploadIDs maps a group markdown upload listing to the upload IDs it
// holds.
func groupUploadIDs(out groupmarkdownuploads.ListOutput) []int64 {
	ids := make([]int64, 0, len(out.Uploads))
	for _, u := range out.Uploads {
		ids = append(ids, u.ID)
	}
	return ids
}

// labelEventNames maps a label event listing to the label names its events
// carry, skipping any event GitLab returned without one.
func labelEventNames(out resourceevents.ListLabelEventsOutput) []string {
	names := make([]string, 0, len(out.Events))
	for _, e := range out.Events {
		if e.Label != nil {
			names = append(names, e.Label.Name)
		}
	}
	return names
}

// milestoneEventTitles maps a milestone event listing to the milestone titles
// its events carry, skipping any event GitLab returned without one.
func milestoneEventTitles(out resourceevents.ListMilestoneEventsOutput) []string {
	titles := make([]string, 0, len(out.Events))
	for _, e := range out.Events {
		if e.Milestone != nil {
			titles = append(titles, e.Milestone.Title)
		}
	}
	return titles
}

// groupMemberIDs maps a group member listing to the user IDs it holds.
func groupMemberIDs(out groups.MemberListOutput) []int64 {
	ids := make([]int64, 0, len(out.Members))
	for _, m := range out.Members {
		ids = append(ids, m.ID)
	}
	return ids
}

// sharedWithGroupIDs maps a group to the IDs of the groups it is shared with.
// A share and an unshare are both observable there.
func sharedWithGroupIDs(out groups.Output) []int64 {
	ids := make([]int64, 0, len(out.SharedWithGroups))
	for _, s := range out.SharedWithGroups {
		ids = append(ids, s.GroupID)
	}
	return ids
}

// projectLabelIDs maps a project label listing to the label IDs it holds.
func projectLabelIDs(out labels.ListOutput) []int64 {
	ids := make([]int64, 0, len(out.Labels))
	for _, l := range out.Labels {
		ids = append(ids, l.ID)
	}
	return ids
}

// draftNoteIDs maps a merge request draft note listing to the note IDs it
// holds.
func draftNoteIDs(out mrdraftnotes.ListOutput) []int64 {
	ids := make([]int64, 0, len(out.DraftNotes))
	for _, n := range out.DraftNotes {
		ids = append(ids, n.ID)
	}
	return ids
}

// integrationSlugs maps a project integration listing to the slugs it holds.
// GitLab lists only active integrations, so a deleted one leaves the list.
func integrationSlugs(out integrations.ListOutput) []string {
	slugs := make([]string, 0, len(out.Integrations))
	for _, i := range out.Integrations {
		slugs = append(slugs, i.Slug)
	}
	return slugs
}

// deployTokenIDs maps a deploy token listing to the token IDs it holds. The
// project-scoped and group-scoped listings share the shape.
func deployTokenIDs(out deploytokens.ListOutput) []int64 {
	ids := make([]int64, 0, len(out.DeployTokens))
	for _, tok := range out.DeployTokens {
		ids = append(ids, tok.ID)
	}
	return ids
}

// gpgKeyIDs maps a GPG key listing to the key IDs it holds. The account-scoped
// and admin user-scoped listings share the shape.
func gpgKeyIDs(out usergpgkeys.ListOutput) []int64 {
	ids := make([]int64, 0, len(out.Keys))
	for _, k := range out.Keys {
		ids = append(ids, k.ID)
	}
	return ids
}

// sshKeyIDs maps an SSH key listing to the key IDs it holds. The account-scoped
// and admin user-scoped listings share the shape.
func sshKeyIDs(out users.SSHKeyListOutput) []int64 {
	ids := make([]int64, 0, len(out.Keys))
	for _, k := range out.Keys {
		ids = append(ids, k.ID)
	}
	return ids
}

// hookCustomHeaderKeys maps a project webhook to the custom header keys it
// carries. GitLab masks each header's value on read, so the key is the only
// part of a header a read can observe.
func hookCustomHeaderKeys(out projects.HookOutput) []string {
	keys := make([]string, 0, len(out.CustomHeaders))
	for _, h := range out.CustomHeaders {
		keys = append(keys, h.Key)
	}
	return keys
}
