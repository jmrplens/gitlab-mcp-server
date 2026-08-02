//go:build e2e && !enterprise

// modes_ce_test.go verifies the two protective modes against a live GitLab
// instance: read-only mode (GITLAB_READ_ONLY) must remove every mutating
// operation while leaving reads working, and safe mode (GITLAB_SAFE_MODE) must
// intercept mutating operations with a preview while leaving reads working.
//
// Both modes are covered on the individual surface and on the dynamic
// dispatcher surface, because they are enforced differently there: individual
// tools map one to one onto actions and are filtered or wrapped per tool, while
// gitlab_execute_action covers many actions at once and is therefore handled in
// the action catalog. Regression coverage for both policies previously taking
// down the reads served by the same dispatcher tool.
package suite

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/issues"
)

// modeCallText calls a tool on session and returns its concatenated text
// content. A blocked or rejected call is a result to assert on, so only
// transport errors fail the test.
func modeCallText(ctx context.Context, t *testing.T, session *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	requireNoError(t, err, "call "+name)
	var text strings.Builder
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			text.WriteString(textContent.Text)
		}
	}
	return text.String()
}

// modeSafePreview parses text as a safe-mode preview, reporting whether it is
// one and what operation it named.
func modeSafePreview(text string) (tools.SafeModePreview, bool) {
	var preview tools.SafeModePreview
	if err := json.Unmarshal([]byte(text), &preview); err != nil {
		return tools.SafeModePreview{}, false
	}
	return preview, preview.Status == "blocked" && preview.Mode == "safe"
}

// TestReadOnlyMode exercises GITLAB_READ_ONLY against a live GitLab CE
// instance on both the individual and dynamic surfaces.
//
// A project and an issue are created through the unrestricted individual
// session so the read paths have something real to return. The read-only
// sessions must then serve reads and must not expose or route any mutating
// operation, and the issue count taken afterwards through the unrestricted
// session proves nothing was written.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: read-only.
func TestReadOnlyMode(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)
	_, err := callToolOn[issues.Output](ctx, sess.individual, "gitlab_issue_create", issues.CreateInput{
		ProjectID: proj.pidOf(),
		Title:     "read-only mode fixture issue",
	})
	requireNoError(t, err, "create fixture issue")

	t.Run("IndividualExposesNoMutatingTools", func(t *testing.T) {
		listed, listErr := sess.readOnly.ListTools(ctx, nil)
		requireNoError(t, listErr, "list read-only tools")
		requireTruef(t, len(listed.Tools) > 0, "read-only session exposes no tools at all")
		var mutating []string
		for _, tool := range listed.Tools {
			if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
				mutating = append(mutating, tool.Name)
			}
		}
		requireTruef(t, len(mutating) == 0, "read-only session exposes mutating tools: %v", mutating)
	})

	t.Run("IndividualReadStillWorks", func(t *testing.T) {
		out, listErr := callToolOn[issues.ListOutput](ctx, sess.readOnly, "gitlab_issue_list", issues.ListInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, listErr, "list issues through the read-only session")
		requireTruef(t, len(out.Issues) == 1, "expected the fixture issue through read-only, got %d", len(out.Issues))
	})

	t.Run("IndividualMutatingToolIsGone", func(t *testing.T) {
		_, callErr := callToolOn[issues.Output](ctx, sess.readOnly, "gitlab_issue_create", issues.CreateInput{
			ProjectID: proj.pidOf(),
			Title:     "must not be created",
		})
		requireTruef(t, callErr != nil, "gitlab_issue_create is callable in read-only mode")
	})

	t.Run("DynamicKeepsExecuteForReads", func(t *testing.T) {
		listed, listErr := sess.readOnlyDyn.ListTools(ctx, nil)
		requireNoError(t, listErr, "list read-only dynamic tools")
		names := map[string]bool{}
		for _, tool := range listed.Tools {
			names[tool.Name] = true
		}
		requireTruef(t, names["gitlab_find_action"], "read-only dynamic surface lost gitlab_find_action")
		requireTruef(t, names["gitlab_execute_action"], "read-only dynamic surface lost gitlab_execute_action: reads become unreachable")

		text := modeCallText(ctx, t, sess.readOnlyDyn, "gitlab_execute_action", map[string]any{
			"action": "issue.list",
			"params": map[string]any{"project_id": proj.pidOf()},
		})
		requireTruef(t, strings.Contains(text, "read-only mode fixture issue"),
			"read-only dynamic surface could not list issues: %s", text)
	})

	t.Run("DynamicRejectsMutatingAction", func(t *testing.T) {
		text := modeCallText(ctx, t, sess.readOnlyDyn, "gitlab_execute_action", map[string]any{
			"action": "issue.create",
			"params": map[string]any{"project_id": proj.pidOf(), "title": "must not be created"},
		})
		_, isPreview := modeSafePreview(text)
		requireTruef(t, !isPreview, "read-only must remove mutating actions, not preview them: %s", text)
		requireTruef(t, strings.Contains(strings.ToLower(text), "unknown action"),
			"mutating action was routable in read-only mode: %s", text)
	})

	t.Run("NothingWasWritten", func(t *testing.T) {
		out, listErr := callToolOn[issues.ListOutput](ctx, sess.individual, "gitlab_issue_list", issues.ListInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, listErr, "list issues after read-only attempts")
		requireTruef(t, len(out.Issues) == 1, "read-only session wrote to GitLab: expected 1 issue, got %d", len(out.Issues))
	})
}

// TestSafeModeDynamicSurface exercises GITLAB_SAFE_MODE on the dynamic
// dispatcher surface against a live GitLab CE instance.
//
// Mutating actions must return a preview naming the canonical action, read
// actions routed through the same gitlab_execute_action tool must execute
// normally, and the project must be left untouched.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: dynamic safe-mode.
func TestSafeModeDynamicSurface(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)

	t.Run("MutatingActionReturnsPreviewNamingTheAction", func(t *testing.T) {
		text := modeCallText(ctx, t, sess.safeModeDyn, "gitlab_execute_action", map[string]any{
			"action": "issue.create",
			"params": map[string]any{"project_id": proj.pidOf(), "title": "safe mode dynamic issue"},
		})
		preview, isPreview := modeSafePreview(text)
		requireTruef(t, isPreview, "mutating dynamic action was not previewed: %s", text)
		requireTruef(t, preview.Tool == "issue.create",
			"preview names %q, want the canonical action issue.create", preview.Tool)
		requireTruef(t, strings.Contains(string(preview.Params), "safe mode dynamic issue"),
			"preview does not echo the call arguments: %s", preview.Params)
		requireTruef(t, preview.Hint != "", "preview carries no hint for disabling safe mode")
	})

	t.Run("ReadActionStillExecutes", func(t *testing.T) {
		text := modeCallText(ctx, t, sess.safeModeDyn, "gitlab_execute_action", map[string]any{
			"action": "project.get",
			"params": map[string]any{"project_id": proj.pidOf()},
		})
		_, isPreview := modeSafePreview(text)
		requireTruef(t, !isPreview, "read action was blocked by safe mode: %s", text)
		requireTruef(t, strings.Contains(text, proj.Path),
			"read action did not return the real project: %s", text)
	})

	t.Run("DestructiveActionIsPreviewedNotExecuted", func(t *testing.T) {
		text := modeCallText(ctx, t, sess.safeModeDyn, "gitlab_execute_action", map[string]any{
			"action":  "project.delete",
			"params":  map[string]any{"project_id": proj.pidOf()},
			"confirm": true,
		})
		preview, isPreview := modeSafePreview(text)
		requireTruef(t, isPreview, "destructive dynamic action was not previewed: %s", text)
		requireTruef(t, preview.Tool == "project.delete",
			"preview names %q, want project.delete", preview.Tool)
	})

	t.Run("NothingWasWritten", func(t *testing.T) {
		out, listErr := callToolOn[issues.ListOutput](ctx, sess.individual, "gitlab_issue_list", issues.ListInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, listErr, "list issues after safe-mode attempts")
		requireTruef(t, len(out.Issues) == 0, "safe mode created an issue: got %d", len(out.Issues))

		_, getErr := callToolOn[map[string]any](ctx, sess.individual, "gitlab_project_get", map[string]any{
			"project_id": proj.pidOf(),
		})
		requireNoError(t, getErr, "project must still exist after a previewed delete")
	})
}
