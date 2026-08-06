// live_fixture_helpers_test.go contains unit tests for the live-target
// helpers: temporary project creation with name-collision retry, award-emoji
// fallback candidates, remote-mirror target URLs, and TLS-verify parsing.
package evaluator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// TestCreateLiveTemporaryProject_RetriesNameCollision verifies transient GitLab
// namespace collisions do not fail live evaluator fixture preparation.
func TestCreateLiveTemporaryProject_RetriesNameCollision(t *testing.T) {
	projectCreatePaths := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/groups/my-org%2Ftools":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 42, "full_path": liveFixtureToolsPath})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects":
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode project request: %v", err)
				http.Error(w, "decode project request", http.StatusBadRequest)
				return
			}
			path, _ := request["path"].(string)
			projectCreatePaths = append(projectCreatePaths, path)
			if len(projectCreatePaths) == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"path": []string{"has already been taken"}}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 501, "path": path, "path_with_namespace": liveFixtureToolsPath + "/" + path})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := gitlabclient.NewClient(&config.Config{
		GitLabURL:       server.URL,
		GitLabToken:     "eval-token",
		MetaTools:       true,
		MetaParamSchema: config.DefaultMetaParamSchema,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	project, err := createLiveTemporaryProject(t.Context(), client, "push-rule")
	if err != nil {
		t.Fatalf("createLiveTemporaryProject() error = %v", err)
	}

	if len(projectCreatePaths) != 2 {
		t.Fatalf("project create attempts = %d, want 2", len(projectCreatePaths))
	}
	if project.PathWithNamespace != liveFixtureToolsPath+"/"+projectCreatePaths[1] {
		t.Fatalf("PathWithNamespace = %q, want retried project path", project.PathWithNamespace)
	}
}

// TestLiveAwardEmojiNames_ContainsFallbackCandidates verifies award fixture
// creation has multiple deterministic names to try.
func TestLiveAwardEmojiNames_ContainsFallbackCandidates(t *testing.T) {
	names := liveAwardEmojiNames()
	if len(names) < 3 || !slices.Contains(names, "thumbsup") || !slices.Contains(names, "tada") {
		t.Fatalf("liveAwardEmojiNames() = %v, want common GitLab emoji candidates", names)
	}
	if strings.Join(names, ",") != strings.ToLower(strings.Join(names, ",")) {
		t.Fatalf("liveAwardEmojiNames() = %v, want lowercase names", names)
	}
}

// TestLiveTargetURLHelpers_ValidateEnvAndEscaping verifies live target helpers
// reject unsafe URLs and construct escaped GitLab endpoints deterministically.
func TestLiveTargetURLHelpers_ValidateEnvAndEscaping(t *testing.T) {
	t.Setenv("GITLAB_SKIP_TLS_VERIFY", "")
	client, err := liveGitLabHTTPClient()
	if err != nil || client != http.DefaultClient {
		t.Fatalf("liveGitLabHTTPClient(default) = %v, %v; want default client", client, err)
	}
	t.Setenv("GITLAB_SKIP_TLS_VERIFY", "not-bool")
	_, invalidTLSErr := liveGitLabHTTPClient()
	if invalidTLSErr == nil {
		t.Fatal("liveGitLabHTTPClient(invalid bool) error = nil, want error")
	}

	t.Setenv("GITLAB_URL", "https://gitlab.example.com/root/")
	baseURL, err := liveDockerGitLabBaseURL()
	if err != nil || baseURL.String() != "https://gitlab.example.com/root" {
		t.Fatalf("liveDockerGitLabBaseURL() = %v, %v; want trimmed URL", baseURL, err)
	}
	t.Setenv("GITLAB_URL", "ftp://gitlab.example.com")
	_, invalidURLErr := liveDockerGitLabBaseURL()
	if invalidURLErr == nil {
		t.Fatal("liveDockerGitLabBaseURL(ftp) error = nil, want unsupported scheme")
	}

	endpoint := terraformStateLockEndpoint(&url.URL{Scheme: "https", Host: "gitlab.example.com"}, "group/project", "state one")
	if !strings.Contains(endpoint, "group%2Fproject") || !strings.Contains(endpoint, "state%20one/lock") {
		t.Fatalf("terraformStateLockEndpoint() = %q, want escaped project and state", endpoint)
	}
}

// TestLiveRemoteMirrorTargetURL_EmbedsTokenAndProjectPath verifies mirror target
// URLs use the internal GitLab base and OAuth2 credentials expected by Docker.
func TestLiveRemoteMirrorTargetURL_EmbedsTokenAndProjectPath(t *testing.T) {
	t.Setenv("GITLAB_TOKEN", "token-123")
	t.Setenv("E2E_GITLAB_INTERNAL_URL", "http://gitlab-internal/root")
	got, err := liveRemoteMirrorTargetURL(&gl.Project{PathWithNamespace: "/group/project"})
	if err != nil {
		t.Fatalf("liveRemoteMirrorTargetURL() error = %v", err)
	}
	if !strings.HasPrefix(got, "http://oauth2:token-123@gitlab-internal/root/group/project.git") {
		t.Fatalf("liveRemoteMirrorTargetURL() = %q, want internal OAuth URL", got)
	}
	_, emptyPathErr := liveRemoteMirrorTargetURL(&gl.Project{})
	if emptyPathErr == nil {
		t.Fatal("liveRemoteMirrorTargetURL(empty path) error = nil, want error")
	}
}

// TestGitLabSkipTLSVerify_ParsesEnvironment verifies GitLabSkipTLSVerify parses environment.
func TestGitLabSkipTLSVerify_ParsesEnvironment(t *testing.T) {
	t.Setenv("GITLAB_SKIP_TLS_VERIFY", "true")
	got, err := gitlabSkipTLSVerify()
	if err != nil {
		t.Fatalf("gitlabSkipTLSVerify() error = %v", err)
	}
	if !got {
		t.Fatal("gitlabSkipTLSVerify() = false, want true")
	}

	t.Setenv("GITLAB_SKIP_TLS_VERIFY", "not-bool")
	if _, parseErr := gitlabSkipTLSVerify(); parseErr == nil {
		t.Fatal("gitlabSkipTLSVerify() error = nil, want invalid bool error")
	}
}

// TestReplaceAllPromptBacktickValuesAfter_ReplacesRepeatedMarkers verifies ReplaceAllPromptBacktickValuesAfter when replaces repeated markers.
func TestReplaceAllPromptBacktickValuesAfter_ReplacesRepeatedMarkers(t *testing.T) {
	prompt := "List files for package ID `55`, then delete package ID `52`."
	got, err := replaceAllPromptBacktickValuesAfter(prompt, "package ID ", 61)
	if err != nil {
		t.Fatalf("replaceAllPromptBacktickValuesAfter() error = %v", err)
	}
	want := "List files for package ID `61`, then delete package ID `61`."
	if got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}
}

// TestTerraformStateUnlockProjectID_IgnoresStateName verifies Terraform state fixture setup reads the project path, not the state name.
func TestTerraformStateUnlockProjectID_IgnoresStateName(t *testing.T) {
	got, ok := terraformStateUnlockProjectID("Unlock Terraform state `production` in project `my-org/tools/gitlab-mcp-server`.")
	if !ok {
		t.Fatal("terraformStateUnlockProjectID() ok = false, want true")
	}
	if got != "my-org/tools/gitlab-mcp-server" {
		t.Fatalf("terraformStateUnlockProjectID() = %q, want project path", got)
	}
}

// TestTerraformStateLockEndpoint_PreservesEscapedProjectPath verifies GitLab project paths are escaped exactly once.
func TestTerraformStateLockEndpoint_PreservesEscapedProjectPath(t *testing.T) {
	baseURL, err := url.Parse("http://localhost:8929")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	got := terraformStateLockEndpoint(baseURL, "my-org/tools/gitlab-mcp-server", "eval-unlock")
	want := "http://localhost:8929/api/v4/projects/my-org%2Ftools%2Fgitlab-mcp-server/terraform/state/eval-unlock/lock"
	if got != want {
		t.Fatalf("terraformStateLockEndpoint() = %q, want %q", got, want)
	}
}

// TestOptionalEnvironmentScopeFromPrompt_IgnoresBlankBacktickScope verifies blank backtick values do not mask later scope hints.
func TestOptionalEnvironmentScopeFromPrompt_IgnoresBlankBacktickScope(t *testing.T) {
	tests := []struct {
		name      string
		prompt    string
		wantScope string
		wantOK    bool
	}{
		{
			name:      "explicit scope",
			prompt:    "Delete CI variable `EVAL_TOKEN` with environment_scope `review/eval` in project `my-org/tools/gitlab-mcp-server`.",
			wantScope: "review/eval",
			wantOK:    true,
		},
		{
			name:      "blank scope falls through to production",
			prompt:    "Delete CI variable `EVAL_TOKEN` with environment_scope `` from production scope in project `my-org/tools/gitlab-mcp-server`.",
			wantScope: "production",
			wantOK:    true,
		},
		{
			name:   "whitespace scope ignored",
			prompt: "Delete CI variable `EVAL_TOKEN` with environment scope `   ` in project `my-org/tools/gitlab-mcp-server`.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotScope, gotOK := optionalEnvironmentScopeFromPrompt(tt.prompt)
			if gotScope != tt.wantScope || gotOK != tt.wantOK {
				t.Fatalf("optionalEnvironmentScopeFromPrompt() = %q, %t; want %q, %t", gotScope, gotOK, tt.wantScope, tt.wantOK)
			}
		})
	}
}

// TestFixtureSetupToolEnvelope_UsesDynamicExecuteActionTool verifies dynamic fixture setup uses the visible executor.
func TestFixtureSetupToolEnvelope_UsesDynamicExecuteActionTool(t *testing.T) {
	params := map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"}

	toolName, arguments := fixtureSetupToolEnvelope(config.ToolSurfaceDynamic, "gitlab", "branch.create", params)

	if toolName != dynamicExecuteActionTool {
		t.Fatalf("toolName = %q, want %q", toolName, dynamicExecuteActionTool)
	}
	gotParams, ok := arguments["params"].(map[string]any)
	if arguments["action"] != "branch.create" || !ok || gotParams["project_id"] != params["project_id"] {
		t.Fatalf("arguments = %#v, want dynamic action envelope", arguments)
	}
}

// TestFixtureSetupToolEnvelope_KeepsMetaDispatcher verifies meta fixture setup keeps the dispatcher envelope.
func TestFixtureSetupToolEnvelope_KeepsMetaDispatcher(t *testing.T) {
	params := map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"}

	toolName, arguments := fixtureSetupToolEnvelope(config.ToolSurfaceMeta, "gitlab", "branch.create", params)

	if toolName != "gitlab" {
		t.Fatalf("toolName = %q, want gitlab", toolName)
	}
	gotParams, ok := arguments["params"].(map[string]any)
	if arguments["action"] != "branch.create" || !ok || gotParams["project_id"] != params["project_id"] {
		t.Fatalf("arguments = %#v, want meta action envelope", arguments)
	}
}

// TestCallFixtureSetupTool_FallsBackToSplitMetaTool verifies CallFixtureSetupTool falls back to split meta tool.
func TestCallFixtureSetupTool_FallsBackToSplitMetaTool(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "fixture-test", Version: "0"}, nil)
	called := false
	mcp.AddTool(server, &mcp.Tool{Name: "gitlab_branch", Description: "branch meta-tool"}, func(_ context.Context, _ *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, any, error) {
		called = true
		if input["action"] != "create" {
			t.Fatalf("action = %v, want create", input["action"])
		}
		params, _ := input["params"].(map[string]any)
		if params["project_id"] != "my-org/tools/gitlab-mcp-server" {
			t.Fatalf("params = %+v", params)
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
	})

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(t.Context(), serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "fixture-test-client", Version: "0"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	err = callFixtureSetupTool(t.Context(), session, config.ToolSurfaceMeta, "branch.create", map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"})
	if err != nil {
		t.Fatalf("callFixtureSetupTool() error = %v", err)
	}
	if !called {
		t.Fatal("split meta-tool was not called")
	}
}
