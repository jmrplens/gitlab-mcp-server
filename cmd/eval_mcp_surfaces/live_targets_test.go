package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// TestOptionalEnvironmentScopeFromPrompt_ExtractsExplicitAndProductionScopes
// verifies CI variable fixture preparation uses the intended environment scope.
func TestOptionalEnvironmentScopeFromPrompt_ExtractsExplicitAndProductionScopes(t *testing.T) {
	if got := projectVariableEnvironmentScope("update variable with environment_scope `staging`"); got != "staging" {
		t.Fatalf("projectVariableEnvironmentScope(explicit) = %q, want staging", got)
	}
	if got := projectVariableEnvironmentScope("delete variable in production scope"); got != "production" {
		t.Fatalf("projectVariableEnvironmentScope(production) = %q, want production", got)
	}
	if got := projectVariableEnvironmentScope("delete unscoped variable"); got != "*" {
		t.Fatalf("projectVariableEnvironmentScope(default) = %q, want *", got)
	}
}

// TestPromptInt64After_ParsesBacktickInteger verifies live fixture prompt
// rewriting fails loudly when numeric placeholders are absent or malformed.
func TestPromptInt64After_ParsesBacktickInteger(t *testing.T) {
	got, err := promptInt64After("delete award emoji ID `42`", promptMarkerAwardEmojiID)
	if err != nil || got != 42 {
		t.Fatalf("promptInt64After() = %d, %v; want 42", got, err)
	}
	_, malformedErr := promptInt64After("delete award emoji ID `abc`", promptMarkerAwardEmojiID)
	if malformedErr == nil {
		t.Fatal("promptInt64After(malformed) error = nil, want error")
	}
	_, missingErr := promptInt64After("delete missing value", promptMarkerAwardEmojiID)
	if missingErr == nil {
		t.Fatal("promptInt64After(missing) error = nil, want error")
	}
}

// TestEnsureLiveIssueAwardDeleteTarget_CreatesDisposableIssue verifies MT-110
// live preparation does not depend on a shared issue that previous tasks may mutate.
func TestEnsureLiveIssueAwardDeleteTarget_CreatesDisposableIssue(t *testing.T) {
	calls := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/my-org%2Ftools%2Fgitlab-mcp-server/issues":
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode issue request: %v", err)
				http.Error(w, "decode issue request", http.StatusBadRequest)
				return
			}
			if title, _ := request["title"].(string); !strings.HasPrefix(title, "Evaluation issue award delete target ") {
				t.Errorf("issue title = %q, want live award delete target", title)
				http.Error(w, "unexpected issue title", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 501, "iid": 77, "project_id": 101, "title": request["title"]})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/my-org%2Ftools%2Fgitlab-mcp-server/issues/77/award_emoji":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 901, "name": "thumbsup"})
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
	task := evalTask{ID: "MT-110", Prompt: "Remove award emoji ID `12` from issue `42` in project `my-org/tools/gitlab-mcp-server`."}

	prepared, err := ensureLiveIssueAwardDeleteTarget(t.Context(), client, task)
	if err != nil {
		t.Fatalf("ensureLiveIssueAwardDeleteTarget() error = %v", err)
	}

	if !strings.Contains(prepared.Prompt, "issue `77`") || !strings.Contains(prepared.Prompt, "award emoji ID `901`") {
		t.Fatalf("prepared prompt = %q, want disposable issue and award IDs", prepared.Prompt)
	}
	if got := strings.Join(calls, ","); got != "POST /api/v4/projects/my-org%2Ftools%2Fgitlab-mcp-server/issues,POST /api/v4/projects/my-org%2Ftools%2Fgitlab-mcp-server/issues/77/award_emoji" {
		t.Fatalf("calls = %q", got)
	}
}

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
