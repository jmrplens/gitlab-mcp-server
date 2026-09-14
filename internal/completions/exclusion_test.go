// exclusion_test.go covers the narrowing --exclude-tools applies to the
// completion surface, and the drift guards on the hand-kept table it reads.

package completions

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
)

// completionArgumentNames returns every argument name the two dispatches
// answer, read from the source through [argumentSwitchCases] rather than
// restated here, so an argument added to a switch without an entry in the
// table is caught.
func completionArgumentNames(t *testing.T) []string {
	t.Helper()
	names := argumentSwitchCases(t, "completePromptArg")
	names = append(names, argumentSwitchCases(t, "completeResourceArg")...)
	slices.Sort(names)
	return slices.Compact(names)
}

// TestCompletionBackingActions_StaysAlignedWithBothSurfaces is the drift guard
// on the hand-kept overlap table.
//
// It fails in three directions, and each one is a hole nobody would see from
// either package alone: an argument the dispatch answers with no entry escapes
// every exclusion, an entry naming an argument nothing dispatches is a dead
// string, and an action ID the catalog no longer has silently stops matching
// what an operator excluded.
func TestCompletionBackingActions_StaysAlignedWithBothSurfaces(t *testing.T) {
	catalog, err := gitlabtools.BuildActionCatalog(nil, gitlabtools.ActionCatalogOptions{Tier: edition.Ultimate, IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	dispatched := completionArgumentNames(t)

	t.Run("every completed argument is classified", func(t *testing.T) {
		for _, name := range dispatched {
			if _, ok := completionBackingActions[name]; !ok {
				t.Errorf("argument %q is completed but has no entry in completionBackingActions, so --exclude-tools cannot reach it", name)
			}
		}
	})

	t.Run("no entry describes an argument that is gone", func(t *testing.T) {
		for name := range completionBackingActions {
			if !slices.Contains(dispatched, name) {
				t.Errorf("completionBackingActions names %q, which neither dispatch completes", name)
			}
		}
	})

	t.Run("every named action exists in the catalog", func(t *testing.T) {
		for name, backing := range completionBackingActions {
			if len(backing) == 0 {
				t.Errorf("argument %q lists no backing action; give it one or state why it has none", name)
				continue
			}
			for _, id := range backing {
				if _, ok := catalog.Action(actioncatalog.ActionID(id)); !ok {
					t.Errorf("argument %q names action %q, which is not in the action catalog", name, id)
				}
			}
		}
	})
}

// TestComplete_ExcludedActionIsNotCompleted checks that an operator's
// exclusion reaches this surface.
//
// The client here refuses every request, so a completer that reached GitLab
// and one that was withheld both answer an empty list. What tells them apart
// is whether a request was made at all, which is the whole point: the objection
// is not that the data leaks past a permission but that an operator who removed
// an action still has this server enumerating it on their behalf.
func TestComplete_ExcludedActionIsNotCompleted(t *testing.T) {
	cases := []struct {
		name       string
		exclude    []string
		refType    string
		argName    string
		wantCalled bool
	}{
		{name: "excluded project listing", exclude: []string{actionProjectList}, refType: refPrompt, argName: "project_id"},
		{name: "excluded group listing", exclude: []string{actionGroupList}, refType: refPrompt, argName: "group_id"},
		{name: "excluded user listing", exclude: []string{actionUserList}, refType: refPrompt, argName: "username"},
		{
			name: "excluded project listing on a resource reference", exclude: []string{actionProjectList},
			refType: refResource, argName: "project_id",
		},
		{
			name: "an unrelated exclusion leaves it alone", exclude: []string{actionIssueList},
			refType: refPrompt, argName: "project_id", wantCalled: true,
		},
		{name: "nothing excluded", exclude: nil, refType: refPrompt, argName: "project_id", wantCalled: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			called := false
			h := NewHandler(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusNotFound)
			})))
			h.PublishPrompts([]string{"a_prompt"})
			h.PublishExcludedActions(testCase.exclude)

			req := &mcp.CompleteRequest{}
			req.Params = &mcp.CompleteParams{
				Ref:      &mcp.CompleteReference{Type: testCase.refType, Name: "a_prompt", URI: "gitlab://project/{project_id}"},
				Argument: mcp.CompleteParamsArgument{Name: testCase.argName, Value: "x"},
			}
			if _, err := h.Complete(context.Background(), req); err != nil {
				t.Fatalf(fmtUnexpectedErr, err)
			}

			if called != testCase.wantCalled {
				t.Errorf("GitLab called = %t, want %t", called, testCase.wantCalled)
			}
		})
	}
}

// TestComplete_PartlyExcludedArgument_KeepsTheHalfTheOperatorLeft pins the two
// completers that merge sources.
//
// `from` completes branches and tags together and `milestone` completes a
// project's or a group's, so withholding either whole on one exclusion would
// remove data the operator never excluded. The dispatch withholds only when
// every backing action is gone; each half is asked about on its own below it.
func TestComplete_PartlyExcludedArgument_KeepsTheHalfTheOperatorLeft(t *testing.T) {
	cases := []struct {
		name      string
		exclude   []string
		argName   string
		wantPaths []string
		noPaths   []string
	}{
		{
			name: "branches excluded, tags kept", exclude: []string{actionBranchList}, argName: "from",
			wantPaths: []string{"/repository/tags"}, noPaths: []string{"/repository/branches"},
		},
		{
			name: "tags excluded, branches kept", exclude: []string{actionTagList}, argName: "from",
			wantPaths: []string{"/repository/branches"}, noPaths: []string{"/repository/tags"},
		},
		{
			name: "both excluded", exclude: []string{actionBranchList, actionTagList}, argName: "from",
			noPaths: []string{"/repository/branches", "/repository/tags"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var seen []string
			h := NewHandler(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen = append(seen, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			})))
			h.PublishPrompts([]string{"a_prompt"})
			h.PublishExcludedActions(testCase.exclude)

			req := &mcp.CompleteRequest{}
			req.Params = &mcp.CompleteParams{
				Ref:      &mcp.CompleteReference{Type: refPrompt, Name: "a_prompt"},
				Argument: mcp.CompleteParamsArgument{Name: testCase.argName, Value: "m"},
				Context:  &mcp.CompleteContext{Arguments: map[string]string{"project_id": "group/project"}},
			}
			if _, err := h.Complete(context.Background(), req); err != nil {
				t.Fatalf(fmtUnexpectedErr, err)
			}

			for _, want := range testCase.wantPaths {
				if !slices.ContainsFunc(seen, func(path string) bool { return pathHasSuffix(path, want) }) {
					t.Errorf("the requests were %v, want one ending in %q", seen, want)
				}
			}
			for _, unwanted := range testCase.noPaths {
				if slices.ContainsFunc(seen, func(path string) bool { return pathHasSuffix(path, unwanted) }) {
					t.Errorf("the requests were %v, want none ending in %q", seen, unwanted)
				}
			}
		})
	}
}

// pathHasSuffix reports whether a request path ends in the given segment.
func pathHasSuffix(path, suffix string) bool {
	return len(path) >= len(suffix) && path[len(path)-len(suffix):] == suffix
}
