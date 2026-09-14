//go:build e2e

// projecthooks_test.go covers a project webhook through its whole life and
// everything that hangs off one: the custom headers GitLab masks on read,
// the URL variables it validates, and the test delivery. The receiver is
// the fixture service the Docker stack runs, so the test delivery has
// somewhere to land.

package common

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The two paths on the fixture service a hook is pointed at, before and
// after its edit.
const (
	hookReceiverPath        = "/hook"
	hookReceiverUpdatedPath = "/hook-updated"
)

// hookIDs lists the ids of a hook listing.
func hookIDs(hooks []projects.HookOutput) []int64 {
	ids := make([]int64, 0, len(hooks))
	for _, hook := range hooks {
		ids = append(ids, hook.ID)
	}
	return ids
}

// hookHeaderKeys lists the custom header keys a hook carries. GitLab masks
// the values on read and keeps the keys, so the key is what a read observes.
func hookHeaderKeys(hook projects.HookOutput) []string {
	keys := make([]string, 0, len(hook.CustomHeaders))
	for _, header := range hook.CustomHeaders {
		keys = append(keys, header.Key)
	}
	return keys
}

// hookVariableKeys lists the URL variable keys a hook carries, whose values
// GitLab masks the same way.
func hookVariableKeys(hook projects.HookOutput) []string {
	keys := make([]string, 0, len(hook.URLVariables))
	for _, variable := range hook.URLVariables {
		keys = append(keys, variable.Key)
	}
	return keys
}

// containsKey reports whether a key is among those listed, which is the
// question every listing of names here is asked.
func containsKey(keys []string, want string) bool {
	return slices.Contains(keys, want)
}

// TestProjectHooks_Lifecycle_HeadersVariablesAndDelivery adds a webhook to
// a project of each surface's own, lists and reads it, edits its URL and
// events, sets and deletes a custom header, sets and deletes a URL
// variable, shows the refusals of a variable key with a digit and of a
// delete of a variable never set, triggers a test delivery and deletes the
// hook.
//
// Replaces: TestMeta_ProjectHooks
func TestProjectHooks_Lifecycle_HeadersVariablesAndDelivery(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedFixtureService))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("hooks"))
		params := map[string]any{"project_id": project.IDParam()}
		receiver := fixture.ServiceURL(e, hookReceiverPath)
		name := e.Name("hook")

		added := harness.Do[projects.HookOutput](s, actionProjectHookAdd, withParams(params, map[string]any{
			"url": receiver, "push_events": true, "token": "e2e-hook-secret", "name": name, "description": "added by the e2e suite",
		}))
		if added.ID == 0 || added.URL != receiver || !added.PushEvents || added.Name != name {
			e.T.Fatalf("hook_add answered %+v, want a push hook named %q at %s with an ID", added, name, receiver)
		}
		hook := withParams(params, map[string]any{"hook_id": added.ID})

		listed := harness.Do[projects.ListHooksOutput](s, actionProjectHookList, params)
		if !containsID(hookIDs(listed.Hooks), added.ID) {
			e.T.Errorf("the project lists the hooks %v, want hook %d among them", hookIDs(listed.Hooks), added.ID)
		}
		got := harness.Do[projects.HookOutput](s, actionProjectHookGet, hook)
		if got.ID != added.ID || got.URL != receiver {
			e.T.Errorf("hook_get answered hook %d at %s, want %d at %s", got.ID, got.URL, added.ID, receiver)
		}

		updatedReceiver := fixture.ServiceURL(e, hookReceiverUpdatedPath)
		edited := harness.Do[projects.HookOutput](s, actionProjectHookEdit, withParams(hook, map[string]any{"url": updatedReceiver, "issues_events": true}))
		if edited.ID != added.ID || edited.URL != updatedReceiver || !edited.IssuesEvents {
			e.T.Errorf("hook_edit answered %+v, want hook %d at %s with issue events on", edited, added.ID, updatedReceiver)
		}

		assertHookCustomHeaderRoundTrips(e, s, hook)
		assertHookURLVariablesRoundTrip(e, s, hook)

		delivery := harness.Do[projects.TriggerTestHookOutput](s, actionProjectHookTest, withParams(hook, map[string]any{"event": "push_events"}))
		if delivery.Message == "" {
			e.T.Errorf("hook_test answered %+v, want a message about the delivery", delivery)
		}

		harness.DoVoid(s, actionProjectHookDelete, hook)
		remaining := harness.Do[projects.ListHooksOutput](s, actionProjectHookList, params)
		if containsID(hookIDs(remaining.Hooks), added.ID) {
			e.T.Errorf("the project still lists hook %d after its delete", added.ID)
		}
	})
}

// assertHookCustomHeaderRoundTrips sets a custom header on the hook, sees
// its key on the read, deletes it and sees it gone.
func assertHookCustomHeaderRoundTrips(e *harness.Env, s *harness.Session, hook map[string]any) {
	e.T.Helper()
	const key = "X-E2E-Header"

	set := harness.Do[toolutil.VoidOutput](s, actionProjectHookSetCustomHeader, withParams(hook, map[string]any{"key": key, "value": "e2e-value"}))
	if set.Status != voidStatusSuccess {
		e.T.Errorf("hook_set_custom_header answered %+v, want a %s status", set, voidStatusSuccess)
	}
	withHeader := harness.Do[projects.HookOutput](s, actionProjectHookGet, hook)
	if !containsKey(hookHeaderKeys(withHeader), key) {
		e.T.Errorf("the hook carries the custom headers %v after the set, want %q among them", hookHeaderKeys(withHeader), key)
	}

	harness.DoVoid(s, actionProjectHookDeleteCustomHeader, withParams(hook, map[string]any{"key": key}))
	withoutHeader := harness.Do[projects.HookOutput](s, actionProjectHookGet, hook)
	if containsKey(hookHeaderKeys(withoutHeader), key) {
		e.T.Errorf("the hook still carries the custom header %q after its delete", key)
	}
}

// assertHookURLVariablesRoundTrip shows the two refusals GitLab makes of a
// URL variable, a key carrying a digit and a delete of a variable never
// set, then sets a legal one, sees its key on the read, deletes it and
// sees it gone.
//
// The illegal key is the old suite's own e2e_var, whose digit is what
// GitLab refuses: the old test read that refusal as the hook's URL
// carrying no {template} and said so in a comment, and the group hook
// scenario the EE port already drives shows a key with no digit accepted
// on a hook whose URL templates nothing. The old comment was wrong about
// the cause, and keeping its key is what holds this to the same refusal
// its run recorded.
func assertHookURLVariablesRoundTrip(e *harness.Env, s *harness.Session, hook map[string]any) {
	e.T.Helper()
	const (
		illegalKey = "e2e_var"
		key        = "receiver_host"
	)

	refused := harness.ExpectToolError(s, actionProjectHookSetURLVariable,
		withParams(hook, map[string]any{"key": illegalKey, "value": "value"}), "letters and underscores")
	e.T.Logf("a URL variable key with a digit is refused: %s", firstLine(refused))
	refused = harness.Refused(s, actionProjectHookDeleteURLVariable, withParams(hook, map[string]any{"key": illegalKey}), harness.FailureNotFound)
	assertMentions(e, "the delete of a URL variable never set", refused, "not currently set")

	set := harness.Do[toolutil.VoidOutput](s, actionProjectHookSetURLVariable, withParams(hook, map[string]any{"key": key, "value": "e2e-value"}))
	if set.Status != voidStatusSuccess {
		e.T.Errorf("hook_set_url_variable answered %+v, want a %s status", set, voidStatusSuccess)
	}
	withVariable := harness.Do[projects.HookOutput](s, actionProjectHookGet, hook)
	if !containsKey(hookVariableKeys(withVariable), key) {
		e.T.Errorf("the hook carries the URL variables %v after the set, want %q among them", hookVariableKeys(withVariable), key)
	}

	harness.DoVoid(s, actionProjectHookDeleteURLVariable, withParams(hook, map[string]any{"key": key}))
	withoutVariable := harness.Do[projects.HookOutput](s, actionProjectHookGet, hook)
	if containsKey(hookVariableKeys(withoutVariable), key) {
		e.T.Errorf("the hook still carries the URL variable %q after its delete", key)
	}
}
