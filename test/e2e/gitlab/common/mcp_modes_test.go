//go:build e2e

// mcp_modes_test.go covers the two protective modes on every surface:
// read-only mode removes every mutating operation and leaves the reads
// working, and safe mode answers every mutating operation with a preview of
// itself and leaves the reads working.
//
// The modes are enforced differently per surface, which is why one scenario
// runs on all three: the individual surface filters or wraps whole tools, and
// the two dispatchers do it per action inside a tool that also serves reads.
// The gitlab_interactive_* flows are the one thing registered outside the
// catalog on every surface, and so the one thing each mode has to reach by a
// second mechanism; they are called by name here, through Raw, because that
// is the only way to name a tool the projection does not know.

package common

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The interactive issue flow, spelled as each surface reaches it: the
// standalone tool the meta and individual surfaces register, and the action
// gitlab_execute_action runs it as. Both are outside the catalog the
// projection reads, which is why they are named here and called through Raw.
const (
	interactiveIssueTool   = "gitlab_interactive_issue_create"
	interactiveIssueAction = "interactive.issue_create"
	executeActionTool      = "gitlab_execute_action"
)

// The names a safe-mode preview carries for issue.create on each surface: the
// canonical action on the two dispatchers, which preview per action, and the
// registered tool on the individual surface, which previews per tool.
var previewNamesIssueCreate = map[harness.Surface]string{
	harness.SurfaceDynamic:    string(actionIssueCreate),
	harness.SurfaceMeta:       string(actionIssueCreate),
	harness.SurfaceIndividual: "gitlab_issue_create",
}

// modeFixture is what the mode tests stand on: a project of their own, so a
// write that got through would be visible as a change to something nobody
// else reads, with one issue for the reads to find.
type modeFixture struct {
	project fixture.Project
	issue   fixture.Issue
}

// buildModeFixture creates the fixture once on the parent Env, so the three
// surfaces share one project rather than costing GitLab three.
func buildModeFixture(e *harness.Env) modeFixture {
	project := fixture.NewProject(e, fixture.WithNamePrefix("modes"))
	return modeFixture{project: project, issue: fixture.NewIssue(e, project, "mode fixture issue")}
}

// TestModes_ReadOnly checks GITLAB_MCP_READ_ONLY on every surface: the read
// finds the fixture issue, the write is declined in the surface's own shape,
// the interactive flow is withdrawn, and the project has exactly the issue
// it started with.
//
// Replaces: TestReadOnlyMode
func TestModes_ReadOnly(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, buildModeFixture, func(e *harness.Env, surface harness.Surface, f modeFixture) {
		s := e.Session(harness.ServerConfig{Surface: surface, Mode: harness.ModeReadOnly})

		listed := harness.Do[issues.ListOutput](s, actionIssueList, map[string]any{"project_id": f.project.IDParam()})
		if len(listed.Issues) != 1 || listed.Issues[0].IID != f.issue.IID {
			e.T.Errorf("the read-only session listed %d issues, want the one fixture issue #%d: %+v", len(listed.Issues), f.issue.IID, listed.Issues)
		}

		if s.Serves(actionIssueCreate) {
			e.T.Errorf("the read-only %s session serves %s", surface, actionIssueCreate)
		}
		declined := harness.Withheld(s, actionIssueCreate, map[string]any{"project_id": f.project.IDParam(), "title": "must not be created"})
		if _, isPreview := toolutil.ParseSafeModePreview(declined); isPreview {
			e.T.Errorf("read-only mode must remove a write, not preview it: %q", declined)
		}
		if surface == harness.SurfaceDynamic && !strings.Contains(declined, "configured to withhold") {
			// Not "unknown action": a withheld write is reported as existing
			// but unavailable, naming the decision that removed it, so a
			// model does not record the capability as missing.
			e.T.Errorf("the dynamic surface declined %s without naming the deployment's decision: %q", actionIssueCreate, declined)
		}

		assertInteractiveWithdrawn(e, s, f)
		assertIssueCount(e, f.project, 1)
	})
}

// TestModes_Safe checks GITLAB_MCP_SAFE_MODE on every surface: the read
// answers with the real project, a write and a destructive action are
// previewed under the name the surface spells them, the interactive flow is
// previewed too, and nothing was written or deleted.
//
// Replaces: TestSafeMode, TestSafeModeDynamicSurface
func TestModes_Safe(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, buildModeFixture, func(e *harness.Env, surface harness.Surface, f modeFixture) {
		s := e.Session(harness.ServerConfig{Surface: surface, Mode: harness.ModeSafe})

		got := harness.Do[projects.Output](s, actionProjectGet, map[string]any{"project_id": f.project.IDParam()})
		if got.ID != f.project.ID || got.PathWithNamespace != f.project.Path {
			e.T.Errorf("the safe-mode read answered project %d (%s), want %d (%s)", got.ID, got.PathWithNamespace, f.project.ID, f.project.Path)
		}

		preview := harness.Refused(s, actionIssueCreate, map[string]any{"project_id": f.project.IDParam(), "title": "safe mode issue"}, harness.FailureSafeMode)
		if want := previewNamesIssueCreate[surface]; !strings.HasSuffix(preview, want) {
			e.T.Errorf("the preview of %s on %s is %q, want it to name %s", actionIssueCreate, surface, preview, want)
		}
		// Confirmed, so the confirmation guard is not what stops it: the
		// preview must be what a destructive call an operator approved gets.
		preview = harness.Refused(s, actionProjectDelete, map[string]any{"project_id": f.project.IDParam()}, harness.FailureSafeMode)
		if !strings.Contains(preview, "delete") {
			e.T.Errorf("the preview of %s does not name the action: %q", actionProjectDelete, preview)
		}

		assertInteractivePreviewed(e, s, f)
		assertIssueCount(e, f.project, 1)
		assertProjectExists(e, f.project)
	})
}

// interactiveCall spells the raw call each surface takes for the interactive
// issue flow: the standalone tool on meta and individual, and the execute
// tool carrying the action on dynamic.
func interactiveCall(surface harness.Surface, f modeFixture) *mcp.CallToolParams {
	if surface == harness.SurfaceDynamic {
		return &mcp.CallToolParams{Name: executeActionTool, Arguments: map[string]any{
			"action": interactiveIssueAction,
			"params": map[string]any{"project_id": f.project.IDParam()},
		}}
	}
	return &mcp.CallToolParams{Name: interactiveIssueTool, Arguments: map[string]any{"project_id": f.project.IDParam()}}
}

// assertInteractiveWithdrawn checks that read-only mode took the interactive
// flow away: the tool is not listed and a call to it is refused, as an
// unregistered tool on the surfaces that register it and as an unknown
// action on the dynamic one.
func assertInteractiveWithdrawn(e *harness.Env, s *harness.Session, f modeFixture) {
	e.T.Helper()

	result, err := s.Raw(interactiveCall(s.Surface(), f))
	if s.Surface() == harness.SurfaceDynamic {
		if err != nil {
			e.T.Fatalf("calling %s through %s: %v", interactiveIssueAction, executeActionTool, err)
		}
		if !result.IsError || !strings.Contains(rawText(result), "unknown action") {
			e.T.Errorf("read-only mode left %s reachable through %s: %q", interactiveIssueAction, executeActionTool, rawText(result))
		}
		return
	}

	for _, tool := range s.Tools() {
		if tool == interactiveIssueTool {
			e.T.Errorf("the read-only %s session lists %s", s.Surface(), interactiveIssueTool)
		}
	}
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		e.T.Errorf("read-only mode left %s registered on %s: err=%v result=%q", interactiveIssueTool, s.Surface(), err, rawText(result))
	}
}

// assertInteractivePreviewed checks that safe mode previews the interactive
// flow instead of eliciting: the answer is a preview naming the flow as the
// surface spells it, and no elicitation was needed to get it, since the
// session's client advertises none.
func assertInteractivePreviewed(e *harness.Env, s *harness.Session, f modeFixture) {
	e.T.Helper()

	result, err := s.Raw(interactiveCall(s.Surface(), f))
	if err != nil {
		e.T.Fatalf("calling the interactive flow on %s in safe mode: %v", s.Surface(), err)
	}
	text := rawText(result)
	preview, isPreview := toolutil.ParseSafeModePreview(text)
	if !isPreview {
		e.T.Fatalf("safe mode did not preview the interactive flow on %s: %q", s.Surface(), text)
	}
	want := interactiveIssueTool
	if s.Surface() == harness.SurfaceDynamic {
		want = interactiveIssueAction
	}
	if preview.Tool != want {
		e.T.Errorf("the preview names %q, want %q", preview.Tool, want)
	}
	if preview.Hint == "" {
		e.T.Error("the preview carries no hint for turning safe mode off")
	}
}

// rawText concatenates the text blocks of a raw result.
func rawText(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	var b strings.Builder
	for _, content := range result.Content {
		if text, isText := content.(*mcp.TextContent); isText {
			b.WriteString(text.Text)
		}
	}
	return b.String()
}

// assertIssueCount reads the project's issues through client-go, beside the
// server under test, and holds the count to what the fixture created: a write
// the mode let through would show here whatever the session answered.
func assertIssueCount(e *harness.Env, project fixture.Project, want int) {
	e.T.Helper()

	listed, _, err := e.Client().GL().Issues.ListProjectIssues(project.ID, &gl.ListProjectIssuesOptions{}, gl.WithContext(e.Ctx))
	if err != nil {
		e.T.Fatalf("listing the issues of project %d: %v", project.ID, err)
	}
	if len(listed) != want {
		e.T.Errorf("project %d has %d issues, want %d: the mode let a write through", project.ID, len(listed), want)
	}
}

// assertProjectExists reads the project back through client-go, so a delete
// that a preview should have stopped is caught whatever the session said.
func assertProjectExists(e *harness.Env, project fixture.Project) {
	e.T.Helper()

	got, _, err := e.Client().GL().Projects.GetProject(project.ID, nil, gl.WithContext(e.Ctx))
	if err != nil {
		e.T.Fatalf("project %d cannot be read back after a previewed delete: %v", project.ID, err)
	}
	if got.MarkedForDeletionOn != nil {
		e.T.Errorf("project %d is marked for deletion: the preview did not stop the delete", project.ID)
	}
}
