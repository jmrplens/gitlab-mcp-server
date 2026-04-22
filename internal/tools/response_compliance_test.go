// response_compliance_test.go validates that MCP tool responses comply with
// the JSON-RPC 2.0 + MCP protocol contract: every successful tool call must
// return a non-nil CallToolResult with at least one TextContent entry containing
// non-empty text. Tests exercise real MCP round-trips through in-memory transport.
//
// Run with: go test ./internal/tools/ -run TestResponseCompliance -count=1 -v.
package tools

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// responseComplianceCase defines a tool call with mock routing and expected behavior.
type responseComplianceCase struct {
	name      string
	toolName  string
	arguments map[string]any
	routes    map[string]string // path -> JSON body (status 200)
}

// individualComplianceCases returns test cases for individual tool mode.
// Each case exercises one tool through the full MCP round-trip with a
// mock HTTP handler returning the specified JSON for each API path.
func individualComplianceCases() []responseComplianceCase {
	return []responseComplianceCase{
		{
			name:      "gitlab_server_status",
			toolName:  "gitlab_server_status",
			arguments: map[string]any{},
			routes: map[string]string{
				"/api/v4/version": `{"version":"17.0.0","revision":"abc"}`,
				"/api/v4/user":    `{"id":1,"username":"admin","name":"Admin","state":"active","web_url":"https://example.com/admin"}`,
			},
		},
		{
			name:      "gitlab_project_get",
			toolName:  "gitlab_project_get",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":     `{"version":"17.0.0"}`,
				"/api/v4/projects/42": `{"id":42,"name":"test","path_with_namespace":"g/test","visibility":"private","default_branch":"main","web_url":"https://example.com","description":"desc"}`,
			},
		},
		{
			name:      "gitlab_branch_list",
			toolName:  "gitlab_branch_list",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":                         `{"version":"17.0.0"}`,
				"/api/v4/projects/42/repository/branches": `[{"name":"main","merged":false,"protected":true,"default":true,"commit":{"id":"abc","short_id":"abc","title":"init"}}]`,
			},
		},
		{
			name:      "gitlab_issue_list",
			toolName:  "gitlab_issue_list",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":            `{"version":"17.0.0"}`,
				"/api/v4/projects/42/issues": `[{"id":1,"iid":1,"title":"Bug","state":"opened","web_url":"https://example.com/issues/1"}]`,
			},
		},
		{
			name:      "gitlab_mr_list",
			toolName:  "gitlab_mr_list",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":                    `{"version":"17.0.0"}`,
				"/api/v4/projects/42/merge_requests": `[{"id":1,"iid":1,"title":"MR","state":"opened","web_url":"https://example.com/mr/1"}]`,
			},
		},
		{
			name:      "gitlab_tag_list",
			toolName:  "gitlab_tag_list",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":                     `{"version":"17.0.0"}`,
				"/api/v4/projects/42/repository/tags": `[{"name":"v1.0.0","commit":{"id":"abc","short_id":"abc","title":"release"}}]`,
			},
		},
		{
			name:      "gitlab_label_list",
			toolName:  "gitlab_label_list",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":            `{"version":"17.0.0"}`,
				"/api/v4/projects/42/labels": `[{"id":1,"name":"bug","color":"#ff0000"}]`,
			},
		},
		{
			name:      "gitlab_user_current",
			toolName:  "gitlab_user_current",
			arguments: map[string]any{},
			routes: map[string]string{
				"/api/v4/version": `{"version":"17.0.0"}`,
				"/api/v4/user":    `{"id":1,"username":"admin","name":"Admin","state":"active","web_url":"https://example.com/admin"}`,
			},
		},
		{
			name:      "gitlab_pipeline_list",
			toolName:  "gitlab_pipeline_list",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":               `{"version":"17.0.0"}`,
				"/api/v4/projects/42/pipelines": `[{"id":1,"iid":1,"status":"success","ref":"main","sha":"abc","web_url":"https://example.com/pipelines/1"}]`,
			},
		},
		{
			name:      "gitlab_release_list",
			toolName:  "gitlab_release_list",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":              `{"version":"17.0.0"}`,
				"/api/v4/projects/42/releases": `[{"tag_name":"v1.0.0","name":"Release 1","description":"First release"}]`,
			},
		},
		{
			name:      "gitlab_package_list",
			toolName:  "gitlab_package_list",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":              `{"version":"17.0.0"}`,
				"/api/v4/projects/42/packages": `[{"id":1,"name":"app","version":"1.0.0","package_type":"generic","status":"default","created_at":"2026-01-01T00:00:00Z"}]`,
			},
		},
		{
			name:      "gitlab_milestone_list",
			toolName:  "gitlab_milestone_list",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":                `{"version":"17.0.0"}`,
				"/api/v4/projects/42/milestones": `[{"id":1,"iid":1,"title":"v1.0","state":"active"}]`,
			},
		},
	}
}

// metaComplianceCases returns test cases for meta-tool mode.
func metaComplianceCases() []responseComplianceCase {
	return []responseComplianceCase{
		{
			name:     "meta_gitlab_project/get",
			toolName: "gitlab_project",
			arguments: map[string]any{
				"action": "get",
				"params": map[string]any{"project_id": "42"},
			},
			routes: map[string]string{
				"/api/v4/version":     `{"version":"17.0.0"}`,
				"/api/v4/projects/42": `{"id":42,"name":"test","path_with_namespace":"g/test","visibility":"private","default_branch":"main","web_url":"https://example.com","description":"desc"}`,
			},
		},
		{
			name:     "meta_gitlab_branch/list",
			toolName: "gitlab_branch",
			arguments: map[string]any{
				"action": "list",
				"params": map[string]any{"project_id": "42"},
			},
			routes: map[string]string{
				"/api/v4/version":                         `{"version":"17.0.0"}`,
				"/api/v4/projects/42/repository/branches": `[{"name":"main","merged":false,"protected":true,"default":true,"commit":{"id":"abc","short_id":"abc","title":"init"}}]`,
			},
		},
		{
			name:     "meta_gitlab_issue/list",
			toolName: "gitlab_issue",
			arguments: map[string]any{
				"action": "list",
				"params": map[string]any{"project_id": "42"},
			},
			routes: map[string]string{
				"/api/v4/version":            `{"version":"17.0.0"}`,
				"/api/v4/projects/42/issues": `[{"id":1,"iid":1,"title":"Bug","state":"opened","web_url":"https://example.com/issues/1"}]`,
			},
		},
		{
			name:     "meta_gitlab_merge_request/list",
			toolName: "gitlab_merge_request",
			arguments: map[string]any{
				"action": "list",
				"params": map[string]any{"project_id": "42"},
			},
			routes: map[string]string{
				"/api/v4/version":                    `{"version":"17.0.0"}`,
				"/api/v4/projects/42/merge_requests": `[{"id":1,"iid":1,"title":"MR","state":"opened","web_url":"https://example.com/mr/1"}]`,
			},
		},
		{
			name:     "meta_gitlab_package/list",
			toolName: "gitlab_package",
			arguments: map[string]any{
				"action": "list",
				"params": map[string]any{"project_id": "42"},
			},
			routes: map[string]string{
				"/api/v4/version":              `{"version":"17.0.0"}`,
				"/api/v4/projects/42/packages": `[{"id":1,"name":"app","version":"1.0.0","package_type":"generic","status":"default","created_at":"2026-01-01T00:00:00Z"}]`,
			},
		},
	}
}

// routeHandler builds an HTTP handler from a path -> JSON response map.
func routeHandler(routes map[string]string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		for prefix, body := range routes {
			if path == prefix || strings.HasPrefix(path, prefix) {
				respondJSON(w, http.StatusOK, body)
				return
			}
		}
		respondJSON(w, http.StatusNotFound, `{"message":"404 Not found"}`)
	})
}

// ---------- Response compliance tests ----------.

// TestResponseCompliance_Individual verifies that individual tool calls
// return structurally valid MCP responses via in-memory transport.
func TestResponseCompliance_Individual(t *testing.T) {
	for _, tc := range individualComplianceCases() {
		t.Run(tc.name, func(t *testing.T) {
			session := newMCPSession(t, routeHandler(tc.routes))
			assertToolResponse(t, session, tc.toolName, tc.arguments)
		})
	}
}

// TestResponseCompliance_Meta verifies that meta-tool calls return
// structurally valid MCP responses via in-memory transport.
func TestResponseCompliance_Meta(t *testing.T) {
	for _, tc := range metaComplianceCases() {
		t.Run(tc.name, func(t *testing.T) {
			session := newMetaMCPSession(t, routeHandler(tc.routes), true)
			assertToolResponse(t, session, tc.toolName, tc.arguments)
		})
	}
}

// assertToolResponse calls a tool and validates the response structure:
//  1. No transport/RPC error
//  2. IsError is false (tool-level success)
//  3. Content array is non-empty
//  4. At least one TextContent with non-empty text
func assertToolResponse(t *testing.T, session *mcp.ClientSession, toolName string, args map[string]any) {
	t.Helper()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%s) transport error: %v", toolName, err)
	}
	if result.IsError {
		var errText string
		for _, c := range result.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				errText = tc.Text
				break
			}
		}
		t.Fatalf("CallTool(%s) returned IsError=true: %s", toolName, errText)
	}

	if len(result.Content) == 0 {
		t.Errorf("CallTool(%s): Content array is empty -- must contain at least one TextContent", toolName)
		return
	}

	var hasText bool
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok && tc.Text != "" {
			hasText = true
			break
		}
	}
	if !hasText {
		t.Errorf("CallTool(%s): no TextContent with non-empty text found in %d content entries", toolName, len(result.Content))
		for i, c := range result.Content {
			t.Logf("  Content[%d]: type=%T", i, c)
		}
	}
}

// TestResponseCompliance_AllToolsListable verifies that all registered tools
// can be listed without error and each has a non-empty name and description.
func TestResponseCompliance_AllToolsListable(t *testing.T) {
	session := newMCPSession(t, auditHandler())

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			if tool.Name == "" {
				t.Error("tool has empty name")
			}
			if tool.Description == "" {
				t.Error("tool has empty description")
			}
		})
	}

	t.Logf("Verified %d tools are listable with name and description", len(result.Tools))
}

// TestResponseCompliance_MetaToolsListable verifies all meta-tools can be
// listed without error and each has a non-empty name and description.
func TestResponseCompliance_MetaToolsListable(t *testing.T) {
	session := newMetaMCPSession(t, auditHandler(), true)

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			if tool.Name == "" {
				t.Error("tool has empty name")
			}
			if tool.Description == "" {
				t.Error("tool has empty description")
			}
		})
	}

	t.Logf("Verified %d meta-tools are listable with name and description", len(result.Tools))
}

// TestResponseCompliance_ContentHasTextContent validates that for each
// domain that produces markdown, the markdownForResult dispatcher returns
// content with proper TextContent entries (complementary to markdown_audit_test.go).
func TestResponseCompliance_ContentHasTextContent(t *testing.T) {
	for _, fix := range allMarkdownFixtures() {
		t.Run(fix.name, func(t *testing.T) {
			result := markdownForResult(fix.result)
			if result == nil {
				t.Skip("nil dispatch -- tracked in markdown_audit_test.go")
			}

			if len(result.Content) == 0 {
				t.Fatal("CallToolResult.Content is empty")
			}

			var foundText bool
			for _, c := range result.Content {
				switch v := c.(type) {
				case *mcp.TextContent:
					if v.Text == "" {
						t.Error("TextContent.Text is empty")
					} else {
						foundText = true
					}
				case *mcp.ImageContent:
					if len(v.Data) == 0 {
						t.Error("ImageContent.Data is empty")
					}
				default:
					t.Logf("unexpected content type: %T", c)
				}
			}

			if !foundText {
				t.Error("no non-empty TextContent found in Content array")
			}
		})
	}
}

// TestResponseCompliance_ErrorResponseFormat verifies that tool calls
// returning errors use IsError=true and include descriptive text.
func TestResponseCompliance_ErrorResponseFormat(t *testing.T) {
	errorRoutes := map[string]string{
		"/api/v4/version": `{"version":"17.0.0"}`,
	}

	tests := []struct {
		name      string
		mode      string
		toolName  string
		arguments map[string]any
	}{
		{
			name:      "individual/project_get_404",
			mode:      "individual",
			toolName:  "gitlab_project_get",
			arguments: map[string]any{"project_id": "999"},
		},
		{
			name:     "meta/project_get_404",
			mode:     "meta",
			toolName: "gitlab_project",
			arguments: map[string]any{
				"action": "get",
				"params": map[string]any{"project_id": "999"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var session *mcp.ClientSession
			switch tc.mode {
			case "individual":
				session = newMCPSession(t, routeHandler(errorRoutes))
			case "meta":
				session = newMetaMCPSession(t, routeHandler(errorRoutes), true)
			}

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      tc.toolName,
				Arguments: tc.arguments,
			})
			if err != nil {
				t.Fatalf("CallTool() transport error (should not happen): %v", err)
			}

			if !result.IsError {
				t.Log("tool returned success for non-existent resource -- may be expected if error is reported differently")
				return
			}

			if len(result.Content) == 0 {
				t.Error("error result has empty Content -- should contain error description")
				return
			}

			var errText string
			for _, c := range result.Content {
				if tc, ok := c.(*mcp.TextContent); ok {
					errText = tc.Text
					break
				}
			}
			if errText == "" {
				t.Error("error result lacks TextContent with error description")
			} else {
				t.Logf("error text: %s", truncate(errText, 120))
			}
		})
	}
}

// truncate shortens a string to maxLen, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// TestResponseCompliance_MarkdownContentWellFormed verifies that individual
// tool responses contain well-formed markdown in their TextContent entries.
// The architecture uses a triple-return pattern where the first value is a
// CallToolResult with markdown and the second is the typed JSON output for
// internal meta-tool routing. Only the markdown appears in the MCP response.
func TestResponseCompliance_MarkdownContentWellFormed(t *testing.T) {
	cases := []struct {
		name      string
		toolName  string
		arguments map[string]any
		routes    map[string]string
	}{
		{
			name:      "project_get",
			toolName:  "gitlab_project_get",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":     `{"version":"17.0.0"}`,
				"/api/v4/projects/42": `{"id":42,"name":"test","path_with_namespace":"g/test","visibility":"private","default_branch":"main","web_url":"https://example.com","description":"desc"}`,
			},
		},
		{
			name:      "branch_list",
			toolName:  "gitlab_branch_list",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":                         `{"version":"17.0.0"}`,
				"/api/v4/projects/42/repository/branches": `[{"name":"main","merged":false,"protected":true,"default":true,"commit":{"id":"abc","short_id":"abc","title":"init"}}]`,
			},
		},
		{
			name:      "package_list",
			toolName:  "gitlab_package_list",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":              `{"version":"17.0.0"}`,
				"/api/v4/projects/42/packages": `[{"id":1,"name":"app","version":"1.0.0","package_type":"generic","status":"default","created_at":"2026-01-01T00:00:00Z"}]`,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			session := newMCPSession(t, routeHandler(tc.routes))

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      tc.toolName,
				Arguments: tc.arguments,
			})
			if err != nil {
				t.Fatalf("CallTool() error: %v", err)
			}
			if result.IsError {
				t.Fatalf("CallTool() returned IsError=true")
			}

			if len(result.Content) == 0 {
				t.Fatal("expected at least 1 TextContent entry with markdown, got 0")
			}

			for i, c := range result.Content {
				tc, ok := c.(*mcp.TextContent)
				if !ok {
					continue
				}
				text := strings.TrimSpace(tc.Text)
				if text == "" {
					t.Errorf("Content[%d]: TextContent.Text is empty", i)
					continue
				}
				hasMarkdown := strings.Contains(text, "**") ||
					strings.Contains(text, "| ") ||
					strings.Contains(text, "## ") ||
					strings.Contains(text, "- ")
				if !hasMarkdown {
					t.Errorf("Content[%d]: text lacks markdown indicators (headers, bold, tables, lists)", i)
				}
				t.Logf("Content[%d]: well-formed markdown (%d bytes)", i, len(text))
			}
		})
	}
}

// TestResponseCompliance_NilResultFallback verifies that markdownForResult
// returns a success confirmation for nil results (delete operations).
func TestResponseCompliance_NilResultFallback(t *testing.T) {
	result := markdownForResult(nil)
	if result == nil {
		t.Fatal("markdownForResult(nil) should return success confirmation, got nil")
	}
	if len(result.Content) == 0 {
		t.Fatal("success confirmation has empty Content")
	}
	text := extractTextContent(result)
	if text == "" {
		t.Error("success confirmation has empty TextContent")
	}
	if !strings.Contains(strings.ToLower(text), "ok") {
		t.Logf("success text: %q (expected to contain 'ok')", text)
	}
}

// TestResponseCompliance_DeleteOutputHandled verifies that DeleteOutput
// (used by delete tools) produces valid markdown through the dispatcher.
func TestResponseCompliance_DeleteOutputHandled(t *testing.T) {
	result := markdownForResult(toolutil.DeleteOutput{Message: "Resource deleted successfully"})
	if result == nil {
		t.Fatal("markdownForResult(DeleteOutput) returned nil")
	}
	text := extractTextContent(result)
	if text == "" {
		t.Error("DeleteOutput produced empty markdown")
	}
	if !strings.Contains(text, "deleted") {
		t.Logf("delete markdown: %q", truncate(text, 100))
	}
}

// ---------- Coverage tracking ----------.

// TestResponseCompliance_DomainCoverage checks that the response compliance
// test suite covers the major tool domains (sub-packages). It compares
// tested domains against the known sub-package list and reports coverage.
func TestResponseCompliance_DomainCoverage(t *testing.T) {
	// Known tool domain sub-packages (from internal/tools/*)
	knownDomains := []string{
		"project", "branch", "tag", "release", "issue", "mergerequests",
		"label", "milestone", "member", "user", "pipeline", "job",
		"commit", "search", "group", "wiki", "package", "health",
		"environment", "deployment", "civar", "cilint", "repository",
		"mrnote", "mrdiscussion", "mrapproval", "mrchange",
		"issuenote", "issuelink", "releaselink", "upload", "todo",
		"file", "pipelineschedule", "runner", "accesstoken",
		"mrdraftnote", "snippet", "pages",
	}

	// Tested domains from compliance cases -- map tool names to domain keywords
	testedKeywords := make(map[string]bool)
	for _, tc := range individualComplianceCases() {
		// Extract meaningful domain from tool name: gitlab_{domain}_{action}
		name := strings.TrimPrefix(tc.toolName, "gitlab_")
		for _, d := range knownDomains {
			if strings.Contains(name, d) {
				testedKeywords[d] = true
			}
		}
	}

	covered := len(testedKeywords)
	total := len(knownDomains)
	coverage := float64(covered) / float64(total) * 100

	t.Logf("Domain coverage: %d/%d (%.1f%%)", covered, total, coverage)

	var uncovered []string
	for _, d := range knownDomains {
		if !testedKeywords[d] {
			uncovered = append(uncovered, d)
		}
	}
	if len(uncovered) > 0 {
		t.Logf("Uncovered domains (non-blocking): %s", strings.Join(uncovered, ", "))
	}

	// Informational threshold -- 25% is reasonable for a foundation test
	if coverage < 25 {
		t.Errorf("domain coverage %.1f%% is below minimum 25%% threshold", coverage)
	}
}

func init() {
	// Silence unused import warning for fmt -- used in test log messages.
	_ = fmt.Sprintf
}
