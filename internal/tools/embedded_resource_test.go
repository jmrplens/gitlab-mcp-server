package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// embeddedIssueURI is what issue.get must embed for project 42, issue 7 on
// every surface.
const embeddedIssueURI = "gitlab://project/42/issue/7"

// embeddedResourceBackend answers the one GitLab request issue.get makes.
func embeddedResourceBackend() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/projects/42/issues/7":
			respondJSON(w, http.StatusOK, `{"id":1,"iid":7,"project_id":42,"title":"Embed me","state":"opened","web_url":"https://gitlab.example.com/g/p/-/issues/7"}`)
		case "/api/v4/version":
			respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
		default:
			http.NotFound(w, r)
		}
	})
}

// embeddedResourcesOf returns the resource blocks a tool result carries.
func embeddedResourcesOf(result *mcp.CallToolResult) []*mcp.EmbeddedResource {
	var found []*mcp.EmbeddedResource
	for _, content := range result.Content {
		if embedded, ok := content.(*mcp.EmbeddedResource); ok {
			found = append(found, embedded)
		}
	}
	return found
}

// assertEmbeddedIssue checks the block issue.get embeds: one resource, the
// canonical URI, JSON, and a payload that is the issue itself.
func assertEmbeddedIssue(t *testing.T, surface string, result *mcp.CallToolResult) {
	t.Helper()
	if result.IsError {
		t.Fatalf("%s: issue.get returned an error result: %+v", surface, result.Content)
	}
	embedded := embeddedResourcesOf(result)
	if len(embedded) != 1 {
		t.Fatalf("%s: embedded resources = %d, want 1", surface, len(embedded))
	}
	res := embedded[0].Resource
	if res.URI != embeddedIssueURI || res.MIMEType != "application/json" {
		t.Errorf("%s: embedded resource = %q %q, want %q application/json", surface, res.URI, res.MIMEType, embeddedIssueURI)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(res.Text), &payload); err != nil || payload["iid"] != float64(7) {
		t.Errorf("%s: embedded payload = %q (err %v), want the issue with iid 7", surface, res.Text, err)
	}
}

// newDynamicMCPSession builds a fresh find/execute session over the same
// canonical catalog cmd/server hands the dynamic surface, with the given
// GitLab backend.
func newDynamicMCPSession(t *testing.T, handler http.Handler) *mcp.ClientSession {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client, err := gitlabclient.NewClient(&config.Config{GitLabURL: srv.URL, GitLabToken: "test-token", DisableRetries: true})
	if err != nil {
		t.Fatalf("create test gitlab client: %v", err)
	}
	catalog, err := BuildActionCatalog(client, ActionCatalogOptions{Tier: edition.Free, IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, &mcp.ServerOptions{SchemaCache: testSchemaCache})
	dynamic.RegisterCatalogFindExecuteTools(server, catalog)
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, connectErr := server.Connect(ctx, st, nil); connectErr != nil {
		t.Fatalf("server connect: %v", connectErr)
	}
	session, connectErr := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil).Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// TestEmbeddedResource_GetEmbedsOnEverySurface verifies the feature the
// setting promises: a get action whose spec declares a canonical resource
// embeds it on the individual, meta and dynamic surfaces alike, with the URI
// expanded from the call's parameters and the output as payload.
func TestEmbeddedResource_GetEmbedsOnEverySurface(t *testing.T) {
	backend := embeddedResourceBackend()
	params := map[string]any{"project_id": "42", "issue_iid": 7}

	t.Run("individual", func(t *testing.T) {
		session := newMCPSession(t, backend, false)
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "gitlab_issue_get", Arguments: params})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		assertEmbeddedIssue(t, "individual", result)
	})
	t.Run("meta", func(t *testing.T) {
		session := newMetaMCPSession(t, backend, false)
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "gitlab_issue", Arguments: map[string]any{"action": "get", "params": params}})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		assertEmbeddedIssue(t, "meta", result)
	})
	t.Run("dynamic", func(t *testing.T) {
		session := newDynamicMCPSession(t, backend)
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "gitlab_execute_action", Arguments: map[string]any{"action": "issue.get", "params": params}})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		assertEmbeddedIssue(t, "dynamic", result)
	})
}

// TestEmbeddedResource_SettingOffEmbedsNothing verifies the kill switch:
// with embedding disabled the same call carries no resource block, on every
// surface. It flips a process-wide switch, so it does not run in parallel.
func TestEmbeddedResource_SettingOffEmbedsNothing(t *testing.T) {
	toolutil.EnableEmbeddedResources(false)
	t.Cleanup(func() { toolutil.EnableEmbeddedResources(true) })
	backend := embeddedResourceBackend()
	params := map[string]any{"project_id": "42", "issue_iid": 7}

	calls := []struct {
		surface string
		call    func(t *testing.T) *mcp.CallToolResult
	}{
		{"individual", func(t *testing.T) *mcp.CallToolResult {
			t.Helper()
			result, err := newMCPSession(t, backend, false).CallTool(context.Background(), &mcp.CallToolParams{Name: "gitlab_issue_get", Arguments: params})
			if err != nil {
				t.Fatalf("CallTool: %v", err)
			}
			return result
		}},
		{"meta", func(t *testing.T) *mcp.CallToolResult {
			t.Helper()
			result, err := newMetaMCPSession(t, backend, false).CallTool(context.Background(), &mcp.CallToolParams{Name: "gitlab_issue", Arguments: map[string]any{"action": "get", "params": params}})
			if err != nil {
				t.Fatalf("CallTool: %v", err)
			}
			return result
		}},
		{"dynamic", func(t *testing.T) *mcp.CallToolResult {
			t.Helper()
			result, err := newDynamicMCPSession(t, backend).CallTool(context.Background(), &mcp.CallToolParams{Name: "gitlab_execute_action", Arguments: map[string]any{"action": "issue.get", "params": params}})
			if err != nil {
				t.Fatalf("CallTool: %v", err)
			}
			return result
		}},
	}
	for _, tc := range calls {
		t.Run(tc.surface, func(t *testing.T) {
			result := tc.call(t)
			if result.IsError {
				t.Fatalf("issue.get returned an error result: %+v", result.Content)
			}
			if embedded := embeddedResourcesOf(result); len(embedded) != 0 {
				t.Errorf("embedded resources = %d with the setting off, want none", len(embedded))
			}
		})
	}
}

// TestEmbeddedResource_EveryGetThatHasAResourceDeclaresIt pins the catalog
// side: each action whose entity has a gitlab:// resource template declares
// that template, so a new get action with a resource cannot ship silent, and
// no declared template names a parameter the action lacks (the spec
// validator refuses that, but this reads the served catalog).
func TestEmbeddedResource_EveryGetThatHasAResourceDeclaresIt(t *testing.T) {
	want := map[string]string{
		"project.get":                    "gitlab://project/{project_id}",
		"project.board_get":              "gitlab://project/{project_id}/board/{board_id}",
		"project.label_get":              "gitlab://project/{project_id}/label/{label_id}",
		"project.milestone_get":          "gitlab://project/{project_id}/milestone/{milestone_iid}",
		"group.get":                      "gitlab://group/{group_id}",
		"group.group_label_get":          "gitlab://group/{group_id}/label/{label_id}",
		"group.group_milestone_get":      "gitlab://group/{group_id}/milestone/{milestone_iid}",
		"issue.get":                      "gitlab://project/{project_id}/issue/{issue_iid}",
		"merge_request.get":              "gitlab://project/{project_id}/mr/{merge_request_iid}",
		"pipeline.get":                   "gitlab://project/{project_id}/pipeline/{pipeline_id}",
		"job.get":                        "gitlab://project/{project_id}/job/{job_id}",
		"branch.get":                     "gitlab://project/{project_id}/branch/{branch_name}",
		"tag.get":                        "gitlab://project/{project_id}/tag/{tag_name}",
		"release.get":                    "gitlab://project/{project_id}/release/{tag_name}",
		"repository.commit_get":          "gitlab://project/{project_id}/commit/{sha}",
		"environment.get":                "gitlab://project/{project_id}/environment/{environment_id}",
		"environment.deployment_get":     "gitlab://project/{project_id}/deployment/{deployment_id}",
		"feature_flags.feature_flag_get": "gitlab://project/{project_id}/feature_flag/{name}",
		"access.deploy_key_get":          "gitlab://project/{project_id}/deploy_key/{deploy_key_id}",
		"wiki.get":                       "gitlab://project/{project_id}/wiki/{slug}",
		"snippet.get":                    "gitlab://snippet/{snippet_id}",
		"snippet.project_get":            "gitlab://project/{project_id}/snippet/{snippet_id}",
	}
	catalog, err := BuildActionCatalog(nil, ActionCatalogOptions{Tier: edition.Ultimate})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	declared := map[string]string{}
	for _, group := range catalog.Groups() {
		for _, action := range group.ActionsInOrder() {
			if action.EmbeddedResource != "" {
				declared[string(action.ID)] = action.EmbeddedResource
			}
		}
	}
	for id, template := range want {
		t.Run(id, func(t *testing.T) {
			if got := declared[id]; got != template {
				t.Errorf("%s embedded resource = %q, want %q", id, got, template)
			}
		})
	}
	for _, id := range sortedKeys(declared) {
		if _, known := want[id]; !known {
			t.Errorf("%s declares %q but is not in this table; add it so the documented list stays complete", id, declared[id])
		}
	}
	if len(declared) != len(want) {
		t.Errorf("%d actions declare a canonical resource, want %d: %s", len(declared), len(want), strings.Join(sortedKeys(declared), ", "))
	}
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
