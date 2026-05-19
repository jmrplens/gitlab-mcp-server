package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/autoupdate"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
)

// TestRegisterServerMaintenanceSurfaceTools_SafeModeWrapsMutatingSpec verifies RegisterServerMaintenanceSurfaceTools when safe mode wraps mutating spec.
func TestRegisterServerMaintenanceSurfaceTools_SafeModeWrapsMutatingSpec(t *testing.T) {
	updater := autoupdate.NewUpdaterWithSource(autoupdate.Config{
		Mode:           autoupdate.ModeCheck,
		Repository:     autoupdate.DefaultRepository,
		CurrentVersion: "0.0.0",
	}, nil)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterServerMaintenanceSurfaceTools(server, updater)

	toolByName := toolsByName(listToolsFromServer(t, server))
	checkTool := toolByName["gitlab_server_check_update"]
	if checkTool == nil || checkTool.Annotations == nil || !checkTool.Annotations.ReadOnlyHint {
		t.Fatalf("check update annotations = %#v, want read-only surface tool", checkTool)
	}
	applyTool := toolByName["gitlab_server_apply_update"]
	if applyTool == nil || applyTool.Annotations == nil || applyTool.Annotations.ReadOnlyHint || applyTool.Annotations.DestructiveHint == nil || !*applyTool.Annotations.DestructiveHint {
		t.Fatalf("apply update annotations = %#v, want mutating destructive surface tool", applyTool)
	}

	wrapped := WrapMutatingToolsForSafeMode(server)
	if wrapped != 1 {
		t.Fatalf("WrapMutatingToolsForSafeMode() = %d, want 1", wrapped)
	}

	session := connectServerForTools(t, server)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "gitlab_server_apply_update",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("safe mode content = %#v, want one text result", result.Content)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("safe mode content = %#v, want text content", result.Content)
	}
	var preview SafeModePreview
	if unmarshalErr := json.Unmarshal([]byte(text.Text), &preview); unmarshalErr != nil {
		t.Fatalf("safe mode preview JSON = %q: %v", text.Text, unmarshalErr)
	}
	if preview.Status != "blocked" || preview.Tool != "gitlab_server_apply_update" || !strings.Contains(preview.Hint, "GITLAB_SAFE_MODE=false") {
		t.Fatalf("safe mode preview = %+v, want blocked apply-update preview", preview)
	}
}

// TestRegisterSurfaceTools_InvalidSpecPanics verifies malformed surface tool
// specs are rejected during registration.
//
// The test passes a spec with a name but no route or schema metadata and expects
// a panic. Failing early prevents partial MCP surfaces from being exposed.
func TestRegisterSurfaceTools_InvalidSpecPanics(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	assertPanics(t, func() {
		RegisterSurfaceTools(server, []actioncatalog.SurfaceToolSpec{{Name: "gitlab_invalid_surface"}})
	})
}

// toolsByName converts the GitLab API response to the tool output format.
func toolsByName(items []*mcp.Tool) map[string]*mcp.Tool {
	out := make(map[string]*mcp.Tool, len(items))
	for _, item := range items {
		out[item.Name] = item
	}
	return out
}
