//go:build e2e

// assert_helpers_test.go holds the collection-membership assertions the domain
// suites use to observe what a mutation actually did, rather than only that it
// returned without an error.
//
// Build tag: e2e.
package suite

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/accessrequests"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/accesstokens"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/awardemoji"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/badges"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/clusteragents"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/customemoji"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/deploytokens"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupboards"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupmarkdownuploads"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/integrations"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/labels"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mrdraftnotes"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/resourceevents"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/usergpgkeys"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/users"
)

// membershipWait bounds how long a membership assertion re-reads a collection
// before giving up. GitLab applies the mutations these assertions follow
// synchronously, so the budget exists only to absorb the replication lag an
// ephemeral instance under parallel load occasionally shows; a correct server
// must not be failed over timing.
const membershipWait = 20 * time.Second

// membershipPollInterval is how often a membership assertion re-reads while it
// waits for that lag to clear.
const membershipPollInterval = 500 * time.Millisecond

// The time-tracking suites send human durations ("2h", "30m") and GitLab
// answers in seconds, so the assertions name both halves once here.
const (
	oneHourSeconds       = 60 * 60
	twoHoursSeconds      = 2 * 60 * 60
	threeHoursSeconds    = 3 * 60 * 60
	thirtyMinutesSeconds = 30 * 60
)

// requireListedOn re-reads a collection through the named tool and fails unless
// want is among the identifiers it reports.
//
// It exists because a mutation that answered without an error has not
// necessarily had an effect: a handler that turns a request GitLab refused into
// an empty result satisfies requireNoError and changes nothing, and only
// reading the collection back tells the two apart.
func requireListedOn[O any, K comparable](ctx context.Context, t *testing.T, session *mcp.ClientSession, label, tool string, input any, ids func(O) []K, want K) {
	t.Helper()
	requireMembership(ctx, t, session, label, tool, input, ids, want, true)
}

// requireNotListedOn is the counterpart of [requireListedOn] for a removal: it
// fails while want is still among the identifiers the collection reports.
func requireNotListedOn[O any, K comparable](ctx context.Context, t *testing.T, session *mcp.ClientSession, label, tool string, input any, ids func(O) []K, want K) {
	t.Helper()
	requireMembership(ctx, t, session, label, tool, input, ids, want, false)
}

// requireMembership polls the named list tool until want's presence among the
// identifiers matches present, and fails the test with the last observed
// collection when it never does.
//
// A failed re-read is retried rather than raised, because the budget exists to
// absorb exactly this: a rate limit or a 5xx from an ephemeral instance under
// parallel load says nothing about the mutation, and giving up on the first one
// would fail the test for a reason that is not the code. The last error is kept
// so a re-read that never recovers is still reported as itself.
func requireMembership[O any, K comparable](ctx context.Context, t *testing.T, session *mcp.ClientSession, label, tool string, input any, ids func(O) []K, want K, present bool) {
	t.Helper()
	var (
		last    []K
		callErr error
	)
	pollErr := Poll(ctx, membershipPollInterval, membershipWait, func() (bool, string, error) {
		out, err := callToolOn[O](ctx, session, tool, input)
		if err != nil {
			callErr = err
			return false, fmt.Sprintf("re-read failed: %v", err), nil
		}
		callErr = nil
		last = ids(out)
		return slices.Contains(last, want) == present, fmt.Sprintf("%v", last), nil
	})
	switch {
	case pollErr == nil:
		return
	case callErr != nil:
		t.Fatalf("%s: re-reading the collection still failed after %s: %v", label, membershipWait, callErr)
	case present:
		t.Fatalf("%s: %v is not listed after the mutation; collection holds %v", label, want, last)
	default:
		t.Fatalf("%s: %v is still listed after the mutation; collection holds %v", label, want, last)
	}
}

// requireGoneOn re-reads a single object through the named tool and fails while
// that read still succeeds. It is the counterpart of [requireNotListedOn] for a
// delete whose collection has no list action worth calling, and it insists the
// read fail as a not-found: any other error means the re-read itself broke,
// which proves nothing about the delete. Such an error is retried for the same
// budget as a membership assertion, since a rate limit or a 5xx from a loaded
// instance is not the answer being waited for either.
func requireGoneOn(ctx context.Context, t *testing.T, session *mcp.ClientSession, label, tool string, input any) {
	t.Helper()
	var lastErr error
	pollErr := Poll(ctx, membershipPollInterval, membershipWait, func() (bool, string, error) {
		_, err := callToolWithRetry(ctx, session, tool, input)
		lastErr = err
		switch {
		case err == nil:
			return false, "the object is still readable", nil
		case isNotFoundError(err):
			return true, "not found", nil
		default:
			return false, fmt.Sprintf("re-read failed: %v", err), nil
		}
	})
	switch {
	case pollErr == nil:
		return
	case lastErr == nil:
		t.Fatalf("%s: the object is still readable after the delete", label)
	default:
		t.Fatalf("%s: the re-read still failed with something other than a not-found after %s: %v", label, membershipWait, lastErr)
	}
}

// isNotFoundError reports whether err is GitLab saying the object is not there,
// either as an HTTP 404 or as one of the informational not-found results the
// get handlers return in its place.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	if isHTTPStatus(err, http.StatusNotFound) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") ||
		strings.Contains(msg, "doesn't exist") ||
		strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "no longer exists")
}

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

// groupBoardIDs maps a group issue board listing to the board IDs it holds.
func groupBoardIDs(out groupboards.ListGroupBoardsOutput) []int64 {
	ids := make([]int64, 0, len(out.Boards))
	for _, b := range out.Boards {
		ids = append(ids, b.ID)
	}
	return ids
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
