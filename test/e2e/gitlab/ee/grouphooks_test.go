//go:build e2e

// grouphooks_test.go covers a group's webhooks: the lifecycle of one, with
// the event flags a group hook carries beyond a project's, and the
// sub-operations on one that delivers somewhere, which is where the
// fixture service comes in: its custom headers, its URL variables, a test
// delivery and the resend of a recorded one.

package ee

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The two URLs the lifecycle hook is written with, so the edit is told
// apart from the add by what a read answers. Neither is ever delivered to.
const (
	hookInitialURL = "https://example.com/hook"
	hookEditedURL  = "https://example.com/hook-updated"
)

// The custom header and the two URL variable keys the extras scenario
// uses: one GitLab accepts, and one it refuses for the digit it carries.
const (
	hookCustomHeaderKey    = "X-E2E-Group"
	hookURLVariableKey     = "hook_token"
	hookIllegalVariableKey = "e2e_var"
)

// hookIDs lists the ids of a hook listing.
func hookIDs(hooks []groups.HookOutput) []int64 {
	ids := make([]int64, 0, len(hooks))
	for _, hook := range hooks {
		ids = append(ids, hook.ID)
	}
	return ids
}

// customHeaderKeys lists the custom header keys a hook carries; GitLab
// masks the values on read and keeps the keys.
func customHeaderKeys(hook groups.HookOutput) []string {
	keys := make([]string, 0, len(hook.CustomHeaders))
	for _, header := range hook.CustomHeaders {
		keys = append(keys, header.Key)
	}
	return keys
}

// urlVariableKeys lists the URL variable keys a hook carries, masked the
// same way.
func urlVariableKeys(hook groups.HookOutput) []string {
	keys := make([]string, 0, len(hook.URLVariables))
	for _, variable := range hook.URLVariables {
		keys = append(keys, variable.Key)
	}
	return keys
}

// TestGroupHooks_Lifecycle_AddListGetEditDelete walks one hook per surface
// through its whole life in a shared group, and checks the event flags of a
// group hook round-trip through an edit and a read: the emoji, access
// token and project events, and the branch filter with its strategy.
//
// Replaces: TestEE_MetaGroupEnterpriseOperations
func TestGroupHooks_Lifecycle_AddListGetEditDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("hooks"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)
		params := map[string]any{"group_id": group.IDParam()}

		added := harness.Do[groups.HookOutput](s, actionGroupHookAdd, withParams(params, map[string]any{"url": hookInitialURL, "push_events": true}))
		if added.ID == 0 || added.URL != hookInitialURL {
			e.T.Fatalf("hook_add answered %+v, want a hook on %s with an ID", added, hookInitialURL)
		}
		hook := withParams(params, map[string]any{"hook_id": added.ID})

		listed := harness.Do[groups.HookListOutput](s, actionGroupHookList, params)
		if !containsID(hookIDs(listed.Hooks), added.ID) {
			e.T.Errorf("the group's hooks do not hold the added hook %d: %v", added.ID, hookIDs(listed.Hooks))
		}
		got := harness.Do[groups.HookOutput](s, actionGroupHookGet, hook)
		if got.ID != added.ID || !got.PushEvents {
			e.T.Errorf("hook_get answered %+v, want hook %d with push events on", got, added.ID)
		}

		edited := harness.Do[groups.HookOutput](s, actionGroupHookEdit, withParams(hook, map[string]any{"url": hookEditedURL, "issues_events": true}))
		if edited.ID != added.ID || edited.URL != hookEditedURL || !edited.IssuesEvents {
			e.T.Errorf("hook_edit answered %+v, want hook %d on %s with issue events on", edited, added.ID, hookEditedURL)
		}

		flagged := harness.Do[groups.HookOutput](s, actionGroupHookEdit, withParams(hook, map[string]any{
			"emoji_events": true, "resource_access_token_events": true, "project_events": true,
			"push_events_branch_filter": fixture.DefaultBranch, "branch_filter_strategy": "wildcard",
		}))
		assertGroupHookFlags(e, "hook_edit", flagged)
		// The read is what proves the flags were stored rather than echoed.
		reread := harness.Do[groups.HookOutput](s, actionGroupHookGet, hook)
		assertGroupHookFlags(e, "hook_get after the edit", reread)

		harness.DoVoid(s, actionGroupHookDelete, hook)
		after := harness.Do[groups.HookListOutput](s, actionGroupHookList, params)
		if containsID(hookIDs(after.Hooks), added.ID) {
			e.T.Errorf("the group's hooks still hold hook %d after its delete", added.ID)
		}
	})
}

// assertGroupHookFlags checks the five event fields the flag edit set.
func assertGroupHookFlags(e *harness.Env, what string, hook groups.HookOutput) {
	e.T.Helper()
	if !hook.EmojiEvents || !hook.ResourceAccessTokenEvents || !hook.ProjectEvents {
		e.T.Errorf("%s answers emoji=%t access_token=%t project=%t, want all three on", what, hook.EmojiEvents, hook.ResourceAccessTokenEvents, hook.ProjectEvents)
	}
	if hook.PushEventsBranchFilter != fixture.DefaultBranch || hook.BranchFilterStrategy != "wildcard" {
		e.T.Errorf("%s answers the branch filter %q with strategy %q, want %q with wildcard", what, hook.PushEventsBranchFilter, hook.BranchFilterStrategy, fixture.DefaultBranch)
	}
}

// deliveringHookFixture is a group with a project in it, so a test push
// event has a commit to describe, and the hook is added per surface.
type deliveringHookFixture struct {
	group fixture.Group
}

// buildDeliveringHookFixture creates the group and the project.
func buildDeliveringHookFixture(e *harness.Env) deliveringHookFixture {
	group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("hookxtra"))
	fixture.NewProject(e, fixture.WithNamePrefix("hookxtra"), fixture.InGroup(group))
	return deliveringHookFixture{group: group}
}

// TestGroupHooks_Extras_HeadersVariablesTestAndResend adds a hook per
// surface that delivers to the fixture service, sets and removes a custom
// header, refuses a URL variable whose key carries a digit and accepts one
// that does not, refuses the removal of a variable never set, fires a test
// push event and resends the delivery GitLab recorded for it.
//
// Replaces: TestMeta_GroupHookExtras
func TestGroupHooks_Extras_HeadersVariablesTestAndResend(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedFixtureService))

	harness.SurfacesWith(e, buildDeliveringHookFixture, func(e *harness.Env, surface harness.Surface, f deliveringHookFixture) {
		s := e.On(surface)
		params := map[string]any{"group_id": f.group.IDParam()}

		added := harness.Do[groups.HookOutput](s, actionGroupHookAdd, withParams(params, map[string]any{
			"url": fixture.ServiceURL(e, "/group-hook"), "push_events": true,
		}))
		if added.ID == 0 {
			e.T.Fatalf("hook_add answered %+v, want a hook with an ID", added)
		}
		hook := withParams(params, map[string]any{"hook_id": added.ID})

		harness.DoVoid(s, actionGroupHookSetCustomHeader, withParams(hook, map[string]any{"key": hookCustomHeaderKey, "value": "e2e-group-value"}))
		withHeader := harness.Do[groups.HookOutput](s, actionGroupHookGet, hook)
		if !slices.Contains(customHeaderKeys(withHeader), hookCustomHeaderKey) {
			e.T.Errorf("the hook carries the custom headers %v after the set, want %s among them", customHeaderKeys(withHeader), hookCustomHeaderKey)
		}
		harness.DoVoid(s, actionGroupHookDeleteCustomHeader, withParams(hook, map[string]any{"key": hookCustomHeaderKey}))
		withoutHeader := harness.Do[groups.HookOutput](s, actionGroupHookGet, hook)
		if slices.Contains(customHeaderKeys(withoutHeader), hookCustomHeaderKey) {
			e.T.Errorf("the hook still carries the custom header %s after its delete", hookCustomHeaderKey)
		}

		refused := harness.ExpectToolError(s, actionGroupHookSetURLVariable, withParams(hook, map[string]any{"key": hookIllegalVariableKey, "value": "e2e-value"}), "letters and underscores")
		e.T.Logf("the URL variable with a digit in its key was refused: %s", firstLine(refused))
		harness.DoVoid(s, actionGroupHookSetURLVariable, withParams(hook, map[string]any{"key": hookURLVariableKey, "value": "e2e-value"}))
		withVariable := harness.Do[groups.HookOutput](s, actionGroupHookGet, hook)
		if !slices.Contains(urlVariableKeys(withVariable), hookURLVariableKey) {
			e.T.Errorf("the hook carries the URL variables %v after the set, want %s among them", urlVariableKeys(withVariable), hookURLVariableKey)
		}
		refused = harness.Refused(s, actionGroupHookDeleteURLVariable, withParams(hook, map[string]any{"key": hookIllegalVariableKey}), harness.FailureNotFound)
		assertMentions(e, "the removal of a variable never set", refused, "not currently set")
		harness.DoVoid(s, actionGroupHookDeleteURLVariable, withParams(hook, map[string]any{"key": hookURLVariableKey}))
		withoutVariable := harness.Do[groups.HookOutput](s, actionGroupHookGet, hook)
		if slices.Contains(urlVariableKeys(withoutVariable), hookURLVariableKey) {
			e.T.Errorf("the hook still carries the URL variable %s after its delete", hookURLVariableKey)
		}

		harness.DoVoid(s, actionGroupHookTest, withParams(hook, map[string]any{"trigger": "push_events"}))
		eventID := fixture.WaitForGroupHookEvent(e, f.group, added.ID)
		harness.DoVoid(s, actionGroupHookResendEvent, withParams(hook, map[string]any{"hook_event_id": eventID}))
	})
}
