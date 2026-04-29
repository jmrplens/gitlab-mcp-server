// meta_schema_modes_test.go verifies that meta-tool dispatch behavior is
// invariant across the three META_PARAM_SCHEMA modes: the InputSchema sent
// to the LLM differs (opaque envelope vs structured oneOf), but the same
// {action, params} payload must reach the same handler and produce the
// same response in every mode.

package tools

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// metaSession sets the active param-schema mode, registers meta-tools, and
// resets the mode on cleanup so other tests see the default (opaque).
func metaSessionWithMode(t *testing.T, mode string, handler http.Handler) *mcp.ClientSession {
	t.Helper()
	SetMetaParamSchema(mode)
	t.Cleanup(func() { SetMetaParamSchema("opaque") })
	return newMetaMCPSession(t, handler, false)
}

// TestMetaSchema_DispatchParity verifies that the same {action, params}
// payload reaches the same handler and returns a non-error result in
// opaque, compact, and full modes.
func TestMetaSchema_DispatchParity(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/version":
			respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
		case "/api/v4/projects/42":
			respondJSON(w, http.StatusOK, `{"id":42,"name":"test","path_with_namespace":"g/test","visibility":"private","default_branch":"main","web_url":"https://example.com","description":"d"}`)
		default:
			http.NotFound(w, r)
		}
	})

	for _, mode := range []string{"opaque", "compact", "full"} {
		t.Run(mode, func(t *testing.T) {
			session := metaSessionWithMode(t, mode, handler)
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name: "gitlab_project",
				Arguments: map[string]any{
					"action": "get",
					"params": map[string]any{"project_id": "42"},
				},
			})
			if err != nil {
				t.Fatalf("CallTool(%s): %v", mode, err)
			}
			if result.IsError {
				t.Fatalf("CallTool(%s) returned error result", mode)
			}
		})
	}
}

// TestMetaSchema_FullModeAdvertisesOneOf verifies that ListTools in full
// mode emits a structured oneOf in the meta-tool InputSchema, while opaque
// mode does not. This is the LLM-facing visible difference.
func TestMetaSchema_FullModeAdvertisesOneOf(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
	})

	cases := []struct {
		mode      string
		wantOneOf bool
	}{
		{"opaque", false},
		{"full", true},
	}
	for _, c := range cases {
		t.Run(c.mode, func(t *testing.T) {
			session := metaSessionWithMode(t, c.mode, handler)
			result, err := session.ListTools(context.Background(), nil)
			if err != nil {
				t.Fatalf("ListTools: %v", err)
			}
			var found *mcp.Tool
			for _, tool := range result.Tools {
				if tool.Name == "gitlab_project" {
					found = tool
					break
				}
			}
			if found == nil {
				t.Fatal("gitlab_project not in tools list")
			}
			if found.InputSchema == nil {
				t.Fatal("InputSchema is nil")
			}
			schema, ok := found.InputSchema.(map[string]any)
			if !ok {
				t.Fatalf("InputSchema is not a map: %T", found.InputSchema)
			}
			_, hasOneOf := schema["oneOf"]
			if hasOneOf != c.wantOneOf {
				t.Errorf("mode=%s: oneOf present = %v, want %v", c.mode, hasOneOf, c.wantOneOf)
			}
		})
	}
}
