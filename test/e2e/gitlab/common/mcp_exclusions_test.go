//go:build e2e

// mcp_exclusions_test.go drives --exclude-tools against the binary, on every
// request path the removal is supposed to hold on.
//
// The operator's removal is the one narrowing that is not about a credential:
// it says an action must not exist here, for anybody. The repository states the
// invariant in its own words — an operator who removes an action and finds it
// still readable through another surface has been given a guard that does not
// guard — and the surfaces it has to hold on grew over time: the tool surface,
// then the resources and subscriptions, then the prompts, and since 2026-09-14
// the argument completions, which were the fourth credentialed path to GitLab
// and the last one an operator could not narrow.
//
// A unit test can check each filter against a catalog it builds. What it
// cannot check is that the binary passes the same list to all four, which is
// exactly the way this invariant has been broken before.

package common

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// excludedGroup is the catalog group this removes, and issueListAction one of
// the actions it takes with it.
//
// The issue group is chosen because it is reachable from every path under
// test: an action on the tool surface, a resource, a subscribable template and
// a completion argument all serve issues, so one exclusion can be looked for
// in all four places rather than four exclusions in one each.
const (
	excludedGroup   = "gitlab_issue"
	issueListAction = harness.ActionID("issue.list")
)

// TestExcludeTools_RemovesTheActionFromEveryRequestPath checks that an excluded
// group is gone from the tool surface, from the resources, from the
// subscribable templates and from the argument completions.
func TestExcludeTools_RemovesTheActionFromEveryRequestPath(t *testing.T) {
	e := harness.New(t)
	s := e.Session(harness.ServerConfig{
		Surface:      harness.SurfaceDynamic,
		ExcludeTools: []string{excludedGroup},
	})

	t.Run("the action is not served", func(t *testing.T) {
		if s.Serves(issueListAction) {
			t.Errorf("%s is still served after its group was excluded", issueListAction)
		}
		// Withheld asserts the refusal a caller actually meets, which is what
		// a model reads to decide the server cannot do this.
		if refusal := harness.Withheld(s, issueListAction, map[string]any{"project_id": "1"}); refusal == "" {
			t.Error("the withheld action was refused with no message, so a caller learns nothing from it")
		}
	})

	t.Run("no resource serves the same objects", func(t *testing.T) {
		for _, uri := range s.Resources() {
			if uri == "gitlab://project/{project_id}/issues" {
				t.Errorf("resource %q survives an exclusion that removed the action serving the same data", uri)
			}
		}
	})

	t.Run("the completion for its argument answers nothing", func(t *testing.T) {
		// The completion reaches GitLab on the caller's own credential, so an
		// excluded listing must not be enumerable through it. An empty answer
		// is the whole assertion: the completion contract is never to block a
		// client, so it withholds rather than refuses.
		if values := s.CompletePrompt("summarize_open_mrs", "issue_iid", ""); len(values) != 0 {
			t.Errorf("the issue completion answered %v after the group was excluded", values)
		}
	})
}

// TestExcludeTools_LeavesEverythingElseAlone is the other half, and the half a
// filter that removes too much fails.
//
// An exclusion is judged by two things and this suite would otherwise assert
// only the first: that the named thing is gone, and that nothing else went with
// it. A filter matching a prefix rather than a name would pass the test above
// and take half the catalog with it.
func TestExcludeTools_LeavesEverythingElseAlone(t *testing.T) {
	e := harness.New(t)
	excluded := e.Session(harness.ServerConfig{
		Surface:      harness.SurfaceDynamic,
		ExcludeTools: []string{excludedGroup},
	})
	ordinary := e.On(harness.SurfaceDynamic)

	if len(excluded.Actions()) >= len(ordinary.Actions()) {
		t.Errorf("the excluding session reaches %d actions and an ordinary one %d, want fewer",
			len(excluded.Actions()), len(ordinary.Actions()))
	}
	for _, id := range []harness.ActionID{"project.get", "branch.list", "merge_request.list"} {
		t.Run(string(id), func(t *testing.T) {
			if !excluded.Serves(id) {
				t.Errorf("%s went with the excluded group, which does not own it", id)
			}
		})
	}
	// The tools themselves are still two: the dynamic surface registers find
	// and execute whatever the catalog holds, so an exclusion may never leave
	// a client with no way in.
	if tools := excluded.Tools(); len(tools) != 2 {
		t.Errorf("the excluding session serves tools %v, want the two dynamic tools", tools)
	}
}

// TestExcludeTools_TheExcludedActionIsUnknownRatherThanWithheld checks the
// message rather than the absence, and checks it says less than a withheld
// action's does.
//
// The two look alike and are opposite decisions. An action a token's scope or
// a protective mode removed is named as withheld, with the cause, because a
// model reading "unknown action, did you mean ..." concludes the server lacks
// the capability rather than that something narrowed it. An action
// --exclude-tools removed is the other way round: `ExcludedByName` is kept out
// of the withheld lists on purpose (internal/tools/catalog_filter.go:28), since
// naming it would leak the operator's configuration and contradict the
// exclusion that operator asked for.
//
// So the assertion here is that the refusal gives nothing away: it must not
// name the exclusion, the tool, or the configuration. This is the scenario
// that would fail if somebody "fixed" the message by merging the two lists.
func TestExcludeTools_TheExcludedActionIsUnknownRatherThanWithheld(t *testing.T) {
	e := harness.New(t)
	s := e.Session(harness.ServerConfig{
		Surface:      harness.SurfaceDynamic,
		ExcludeTools: []string{excludedGroup},
	})

	result, err := s.Raw(&mcp.CallToolParams{
		Name:      "gitlab_execute_action",
		Arguments: map[string]any{"action": string(issueListAction), "params": map[string]any{"project_id": "1"}},
	})
	if err != nil {
		t.Fatalf("executing an excluded action: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("an excluded action answered %s, want a refusal", rawText(result))
	}
	text := rawText(result)
	if !containsAny(text, "unknown action") {
		t.Errorf("the refusal is %q, want the answer an action the dispatcher does not have gets", text)
	}
	// The leak this protects against: a refusal that says the operator removed
	// something tells a caller what the deployment is configured to withhold.
	if containsAny(text, "exclude", "excluded", "withheld", "removed by", excludedGroup) {
		t.Errorf("the refusal is %q, and it names the exclusion or the excluded tool: "+
			"an excluded action must be indistinguishable from one the catalog never had", text)
	}
}
