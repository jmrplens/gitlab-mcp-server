package evaluator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestValidateExecutionOptions_RejectsUnsafeCombinations verifies live execution
// cannot accidentally run against a non-Docker GitLab target.
func TestValidateExecutionOptions_RejectsUnsafeCombinations(t *testing.T) {
	t.Setenv("E2E_MODE", "")
	if err := validateExecutionOptions(options{Execute: true, Backend: backendMock}); err == nil || !strings.Contains(err.Error(), "backend=gitlab") {
		t.Fatalf("validateExecutionOptions(mock) error = %v, want backend=gitlab", err)
	}
	if err := validateExecutionOptions(options{Execute: true, Backend: backendGitLab}); err == nil || !strings.Contains(err.Error(), "E2E_MODE=docker") {
		t.Fatalf("validateExecutionOptions(non-docker) error = %v, want Docker guard", err)
	}
	if err := validateExecutionOptions(options{Execute: true, Backend: backendGitLab, AllowLive: true}); err != nil {
		t.Fatalf("validateExecutionOptions(allow live) error = %v", err)
	}
	if err := validateExecutionOptions(options{Execute: true, MCPCommand: "server"}); err == nil || !strings.Contains(err.Error(), "tools-file") {
		t.Fatalf("validateExecutionOptions(external without tools) error = %v, want tools-file", err)
	}
}

// TestExternalMCPEnv_LoadsOverrides verifies external MCP env files replace
// existing variables while preserving the rest of the process environment.
func TestExternalMCPEnv_LoadsOverrides(t *testing.T) {
	t.Setenv("EVAL_MCP_TEST_ENV", "old")
	envFile := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envFile, []byte("EVAL_MCP_TEST_ENV=new\nEVAL_MCP_ADDED=1\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	env, err := externalMCPEnv(options{MCPEnv: envFile})
	if err != nil {
		t.Fatalf("externalMCPEnv() error = %v", err)
	}
	joined := "\n" + strings.Join(env, "\n") + "\n"
	if !strings.Contains(joined, "\nEVAL_MCP_TEST_ENV=new\n") || !strings.Contains(joined, "\nEVAL_MCP_ADDED=1\n") {
		t.Fatalf("env = %s, want overridden and added values", joined)
	}
}

// TestDockerModeEnabled_ReadsEnvironmentAndEnvFile verifies Docker safety checks
// accept either the process environment or an explicit env file.
func TestDockerModeEnabled_ReadsEnvironmentAndEnvFile(t *testing.T) {
	t.Setenv("E2E_MODE", "docker")
	if !dockerModeEnabled("") {
		t.Fatal("dockerModeEnabled(env) = false, want true")
	}
	t.Setenv("E2E_MODE", "")
	envFile := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envFile, []byte("E2E_MODE=docker\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	if !dockerModeEnabled(envFile) || dockerModeEnabled(filepath.Join(t.TempDir(), "missing.env")) {
		t.Fatal("dockerModeEnabled(env file) did not respect present and missing files")
	}
}

// TestToolResultContent_HandlesStructuredTextAndEmptyResults verifies MCP result
// rendering prefers structured content and has stable fallbacks.
func TestToolResultContent_HandlesStructuredTextAndEmptyResults(t *testing.T) {
	if got := callToolResultText(nil); got != "empty error result" {
		t.Fatalf("callToolResultText(nil) = %q", got)
	}
	structured := &mcp.CallToolResult{StructuredContent: map[string]any{"ok": true}}
	if got := toolResultContent(structured); got != `{"ok":true}` {
		t.Fatalf("toolResultContent(structured) = %q", got)
	}
	if got := toolResultContentForTool("gitlab_project", structured); got != `{"ok":true}` {
		t.Fatalf("toolResultContentForTool(non-find) = %q", got)
	}
	text := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: " one "}, &mcp.TextContent{Text: "two"}}}
	if got := toolResultContent(text); got != " one \ntwo" {
		t.Fatalf("toolResultContent(text) = %q", got)
	}
	find := &mcp.CallToolResult{
		StructuredContent: map[string]any{"results": []any{map[string]any{"id": "project.get", "schema": strings.Repeat("x", maxToolResultLen)}}},
		Content:           []mcp.Content{&mcp.TextContent{Text: "compact result for `project.get`"}},
	}
	if got := toolResultContentForTool(dynamicFindTool, find); got != "compact result for `project.get`" {
		t.Fatalf("toolResultContentForTool(dynamic find) = %q", got)
	}
	if got := truncateToolResult(strings.Repeat("x", maxToolResultLen+1)); !strings.HasSuffix(got, "\n...[truncated]") {
		t.Fatalf("truncateToolResult() suffix = %q, want truncated marker", got[len(got)-20:])
	}
}

// TestParseToolsSnapshot_AcceptsRawAndWrappedShapes verifies snapshots can be
// loaded from both tools/list-compatible and plain array fixtures.
func TestParseToolsSnapshot_AcceptsRawAndWrappedShapes(t *testing.T) {
	for _, data := range [][]byte{
		[]byte(`[{"name":"gitlab_project","inputSchema":{"type":"object"}}]`),
		[]byte(`{"tools":[{"name":"gitlab_project","inputSchema":{"type":"object"}}]}`),
	} {
		snapshot, err := parseToolsSnapshot(data)
		if err != nil {
			t.Fatalf("parseToolsSnapshot(%s) error = %v", data, err)
		}
		if len(snapshot) != 1 || snapshot[0].Name != "gitlab_project" {
			t.Fatalf("snapshot = %#v, want gitlab_project", snapshot)
		}
	}
	if _, err := parseToolsSnapshot([]byte(`{"tools":`)); err == nil {
		t.Fatal("parseToolsSnapshot(invalid) error = nil, want error")
	}
}

// TestDynamicValidationRoutes_RewritesCatalogRoutes verifies dynamic mode routes
// are represented as gitlab_execute_action domain.action IDs.
func TestDynamicValidationRoutes_RewritesCatalogRoutes(t *testing.T) {
	routes := dynamicValidationRoutes(map[string]toolutil.ActionMap{
		"gitlab_project": {"get": toolutil.ActionRoute{}},
		"gitlab_issue":   {"create": toolutil.ActionRoute{Destructive: false}},
	})
	if _, ok := routes[dynamicExecuteActionTool]["project.get"]; !ok {
		t.Fatalf("dynamicValidationRoutes() = %#v, want project.get", routes)
	}
	if got := dynamicActionID("gitlab_issue", "create"); got != "issue.create" {
		t.Fatalf("dynamicActionID() = %q, want issue.create", got)
	}
}
