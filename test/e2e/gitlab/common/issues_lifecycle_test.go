//go:build e2e

// issues_lifecycle_test.go covers the rest of an issue's own lifecycle
// through the server: the read, the listing, the close and the state
// events it records, the delete and what the read answers afterwards. The
// create and the retitle live in issues_test.go, which an earlier port
// wrote, and the get result's embedded resource is checked here on the
// individual surface, where the old suite pinned it.

package common

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/resourceevents"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The resource event waits: GitLab records a state event in the same
// transaction as the change, but a loaded Docker instance answers the
// listing a moment later.
const (
	stateEventInterval = 2 * time.Second
	stateEventWait     = 60 * time.Second
)

// issueGetTool is the individual tool of issue.get, named for the one raw
// call this file makes: the embedded resource check reads the result's
// content blocks, which the projected verbs decode past. It is the name the
// old suite pinned, and the manifest check below holds it to what the
// session serves.
const issueGetTool = "gitlab_issue_get"

// issueIIDs lists the iids of an issue listing.
func issueIIDs(listed []issues.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, issue := range listed {
		ids = append(ids, issue.IID)
	}
	return ids
}

// TestIssue_Lifecycle_GetListCloseAndDelete creates an issue on every
// surface in a shared project, reads it back, finds it among the open
// ones, closes it through an update, reads the state event the close
// recorded, deletes it and checks the read is then refused.
//
// Replaces: TestIndividual_Issues, TestMeta_Issues, TestMeta_StateEvents
func TestIssue_Lifecycle_GetListCloseAndDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("issuelife"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}
		title := e.Name("issue")

		created := harness.Do[issues.Output](s, actionIssueCreate, withParams(params, map[string]any{"title": title, "description": "created by the e2e suite"}))
		if created.IID == 0 || created.State != "opened" {
			e.T.Fatalf("issue create answered %+v, want an opened issue with an iid", created)
		}
		issueParams := withParams(params, map[string]any{"issue_iid": created.IID})

		got := harness.Do[issues.Output](s, actionIssueGet, issueParams)
		if got.IID != created.IID || got.Title != title || got.State != "opened" {
			e.T.Errorf("issue get answered %+v, want the opened issue #%d titled %q", got, created.IID, title)
		}

		listed := harness.Do[issues.ListOutput](s, actionIssueList, withParams(params, map[string]any{"state": "opened"}))
		if !containsID(issueIIDs(listed.Issues), created.IID) {
			e.T.Errorf("the open issues do not hold #%d: %v", created.IID, issueIIDs(listed.Issues))
		}

		closed := harness.Do[issues.Output](s, actionIssueUpdate, withParams(issueParams, map[string]any{"state_event": "close"}))
		if closed.IID != created.IID || closed.State != "closed" {
			e.T.Errorf("issue update answered %+v, want issue #%d closed", closed, created.IID)
		}

		events := harness.Eventually(s, actionIssueStateEventList, issueParams, stateEventInterval, stateEventWait,
			func(out resourceevents.ListStateEventsOutput) bool { return len(out.Events) > 0 })
		if !stateEventRecorded(events.Events, "closed") {
			e.T.Errorf("the issue's state events do not record the close: %+v", events.Events)
		}

		harness.DoVoid(s, actionIssueDelete, issueParams)
		refused := harness.Refused(s, actionIssueGet, issueParams, harness.FailureNotFound)
		e.T.Logf("the read of the deleted issue was refused: %s", firstLine(refused))
	})
}

// stateEventRecorded reports whether a state event listing holds one with
// the given state and an ID.
func stateEventRecorded(events []resourceevents.StateEventOutput, state string) bool {
	for _, event := range events {
		if event.ID != 0 && event.State == state {
			return true
		}
	}
	return false
}

// TestIssue_Get_EmbedsTheCanonicalResource reads an issue through the
// individual tool and checks the result carries the issue's own resource as
// an embedded content block, with the JSON media type and a body: the block
// a client can hand to resources/read without a second lookup.
//
// The call is raw because the embedded block is part of the envelope, which
// the projected verbs decode past; the tool is named as the old suite named
// it, and held to the session's own listing first so that a rename fails
// here rather than as an unknown tool.
//
// Replaces: TestIndividual_Issues
func TestIssue_Get_EmbedsTheCanonicalResource(t *testing.T) {
	e := harness.New(t)

	harness.OnSurfaces(e, "the embedded block is written by the result tail every surface shares, and only the individual "+
		"tool's envelope can be read raw without re-spelling the dispatchers' call shapes",
		[]harness.Surface{harness.SurfaceIndividual}, func(e *harness.Env, surface harness.Surface) {
			s := e.On(surface)
			if !slices.Contains(s.Tools(), issueGetTool) {
				e.T.Fatalf("the %s session does not serve %s, so the raw read below would be refused as an unknown tool", surface, issueGetTool)
			}
			project := fixture.NewProject(e, fixture.WithNamePrefix("issueembed"))
			issue := fixture.NewIssue(e, project, "embedded resource fixture")

			result, err := s.Raw(&mcp.CallToolParams{Name: issueGetTool, Arguments: map[string]any{
				"project_id": project.IDParam(), "issue_iid": issue.IID,
			}})
			if err != nil {
				e.T.Fatalf("%s: %v", issueGetTool, err)
			}
			if result.IsError {
				e.T.Fatalf("%s answered an error: %s", issueGetTool, firstLine(issueResultText(result)))
			}

			wantURI := fmt.Sprintf("gitlab://project/%d/issue/%d", project.ID, issue.IID)
			embedded := issueEmbeddedResource(result)
			switch {
			case embedded == nil || embedded.Resource == nil:
				e.T.Errorf("%s carries no embedded resource block among %d content block(s)", issueGetTool, len(result.Content))
			case embedded.Resource.URI != wantURI:
				e.T.Errorf("the embedded resource is %q, want %q", embedded.Resource.URI, wantURI)
			case embedded.Resource.MIMEType != "application/json":
				e.T.Errorf("the embedded resource is served as %q, want application/json", embedded.Resource.MIMEType)
			case embedded.Resource.Text == "":
				e.T.Errorf("the embedded resource %s carries no body", embedded.Resource.URI)
			}
		})
}

// issueEmbeddedResource returns the first embedded resource block of a
// result, or nil.
func issueEmbeddedResource(result *mcp.CallToolResult) *mcp.EmbeddedResource {
	for _, content := range result.Content {
		if block, ok := content.(*mcp.EmbeddedResource); ok {
			return block
		}
	}
	return nil
}

// issueResultText returns the first text block of a result, for a message.
func issueResultText(result *mcp.CallToolResult) string {
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			return text.Text
		}
	}
	return ""
}
