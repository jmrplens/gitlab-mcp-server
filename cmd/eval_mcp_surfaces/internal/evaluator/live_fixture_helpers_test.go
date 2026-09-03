// live_fixture_helpers_test.go contains unit tests for the live-target
// helpers: temporary project creation with name-collision retry, award-emoji
// fallback candidates, remote-mirror target URLs, and TLS-verify parsing.
package evaluator

import (
	"context"
	"encoding/json"
	"fmt"
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
	// The handler runs on the server goroutine, so it records what it saw and
	// the assertions run on the test goroutine after the call returns. Asserting
	// inside would call FailNow off-test and abandon the request mid-flight.
	var (
		gotAction any
		gotParams map[string]any
		called    bool
	)
	mcp.AddTool(server, &mcp.Tool{Name: "gitlab_branch", Description: "branch meta-tool"}, func(_ context.Context, _ *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, any, error) {
		called = true
		gotAction = input["action"]
		gotParams, _ = input["params"].(map[string]any)
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
	if gotAction != "create" {
		t.Errorf("action = %v, want create", gotAction)
	}
	if gotParams["project_id"] != "my-org/tools/gitlab-mcp-server" {
		t.Errorf("params = %+v, want project_id my-org/tools/gitlab-mcp-server", gotParams)
	}
}

// TestLiveMergeRequestApprovalsSatisfied_ReadsConfigurationThenRules verifies
// the approval check accepts a merge request whose configuration reports no
// outstanding approvals, falls back to the approval rules when the
// configuration call fails, and reports false for an outstanding rule.
func TestLiveMergeRequestApprovalsSatisfied_ReadsConfigurationThenRules(t *testing.T) {
	cases := []struct {
		name              string
		configurationBody string
		configurationCode int
		rulesBody         string
		want              bool
	}{
		{name: "configuration satisfied", configurationBody: `{"approvals_required":0,"approvals_left":0}`, configurationCode: http.StatusOK, want: true},
		{name: "configuration outstanding", configurationBody: `{"approvals_required":1,"approvals_left":1}`, configurationCode: http.StatusOK},
		{name: "rules satisfied", configurationCode: http.StatusNotFound, rulesBody: `{"rules":[{"id":1,"approvals_required":1,"approved":true}]}`, want: true},
		{name: "rules outstanding", configurationCode: http.StatusNotFound, rulesBody: `{"rules":[{"id":1,"approvals_required":1,"approved":false}]}`},
		{name: "rules unavailable", configurationCode: http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.EscapedPath(), "/approval_state") {
					if tc.rulesBody == "" {
						w.WriteHeader(http.StatusNotFound)
						fmt.Fprint(w, `{"message":"404 Not Found"}`)
						return
					}
					fmt.Fprint(w, tc.rulesBody)
					return
				}
				w.WriteHeader(tc.configurationCode)
				if tc.configurationBody == "" {
					fmt.Fprint(w, `{"message":"404 Not Found"}`)
					return
				}
				fmt.Fprint(w, tc.configurationBody)
			}))
			defer server.Close()
			if got := liveMergeRequestApprovalsSatisfied(t.Context(), newFixtureTestClient(t, server.URL), "my-org/app", 7); got != tc.want {
				t.Fatalf("liveMergeRequestApprovalsSatisfied() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestEnsureLiveBranchExists_CreatesOnlyWhenAbsent verifies the branch helper
// skips creation when the branch is present, creates it after a 404, tolerates
// a concurrent creation, and reports an unexpected lookup failure.
func TestEnsureLiveBranchExists_CreatesOnlyWhenAbsent(t *testing.T) {
	cases := []struct {
		name       string
		getCode    int
		createCode int
		wantCreate bool
		wantErr    bool
	}{
		{name: "branch exists", getCode: http.StatusOK},
		{name: "creates after 404", getCode: http.StatusNotFound, createCode: http.StatusOK, wantCreate: true},
		{name: "tolerates concurrent creation", getCode: http.StatusNotFound, createCode: http.StatusConflict, wantCreate: true},
		{name: "reports create failure", getCode: http.StatusNotFound, createCode: http.StatusInternalServerError, wantCreate: true, wantErr: true},
		{name: "reports lookup failure", getCode: http.StatusInternalServerError, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			created := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost {
					created = true
					w.WriteHeader(tc.createCode)
					fmt.Fprint(w, `{"name":"feature/x"}`)
					return
				}
				w.WriteHeader(tc.getCode)
				fmt.Fprint(w, `{"name":"feature/x"}`)
			}))
			defer server.Close()
			err := ensureLiveBranchExists(t.Context(), newFixtureTestClient(t, server.URL), "my-org/app", "feature/x", "main")
			if (err != nil) != tc.wantErr {
				t.Fatalf("ensureLiveBranchExists() error = %v, want error = %t", err, tc.wantErr)
			}
			if created != tc.wantCreate {
				t.Fatalf("created = %t, want %t", created, tc.wantCreate)
			}
		})
	}
}

// TestCreateLiveAwardEmoji_FallsThroughCandidateNames verifies award creation
// walks its candidate emoji when GitLab rejects one as already awarded, and
// reports a non-conflict failure immediately.
func TestCreateLiveAwardEmoji_FallsThroughCandidateNames(t *testing.T) {
	t.Run("second candidate succeeds", func(t *testing.T) {
		var attempts int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			attempts++
			w.Header().Set("Content-Type", "application/json")
			if attempts == 1 {
				w.WriteHeader(http.StatusConflict)
				fmt.Fprint(w, `{"message":"already awarded"}`)
				return
			}
			fmt.Fprint(w, `{"id":42,"name":"thumbsdown"}`)
		}))
		defer server.Close()
		client := newFixtureTestClient(t, server.URL)
		id, err := createLiveMRAwardEmoji(t.Context(), client, "my-org/app", 7)
		if err != nil || id != 42 {
			t.Fatalf("createLiveMRAwardEmoji() = %d, %v; want the second candidate", id, err)
		}
		attempts = 0
		id, err = createLiveIssueAwardEmoji(t.Context(), client, "my-org/app", 3)
		if err != nil || id != 42 {
			t.Fatalf("createLiveIssueAwardEmoji() = %d, %v; want the second candidate", id, err)
		}
	})
	t.Run("non conflict failure aborts", func(t *testing.T) {
		var attempts int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			attempts++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"403 Forbidden"}`)
		}))
		defer server.Close()
		if _, err := createLiveMRAwardEmoji(t.Context(), newFixtureTestClient(t, server.URL), "my-org/app", 7); err == nil {
			t.Fatal("createLiveMRAwardEmoji() error = nil, want the forbidden failure")
		}
		if attempts != 1 {
			t.Fatalf("attempts = %d, want a single attempt", attempts)
		}
	})
}

// TestWaitForLiveMergeRequestReady_TerminalStatuses verifies the merge-request
// wait returns once GitLab reports the merge request mergeable, accepts an
// approvals-syncing status whose approvals are already satisfied, and reports
// a status that will never become mergeable.
func TestWaitForLiveMergeRequestReady_TerminalStatuses(t *testing.T) {
	cases := []struct {
		name       string
		status     string
		approvals  string
		wantErr    string
		wantNoWait bool
	}{
		{name: "mergeable", status: "mergeable", wantNoWait: true},
		{name: "approvals syncing but satisfied", status: "approvals_syncing", approvals: `{"approvals_required":0,"approvals_left":0}`, wantNoWait: true},
		{name: "terminal status", status: "conflict", wantErr: "is not mergeable: conflict"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := r.URL.EscapedPath()
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(path, "/approvals") {
					fmt.Fprint(w, tc.approvals)
					return
				}
				fmt.Fprintf(w, `[{"id":1,"iid":7,"detailed_merge_status":%q}]`, tc.status)
			}))
			defer server.Close()
			err := waitForLiveMergeRequestReady(t.Context(), newFixtureTestClient(t, server.URL), "my-org/app", 7)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("waitForLiveMergeRequestReady() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("waitForLiveMergeRequestReady() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

// TestGetLiveMergeRequestWithStatusRecheck_EmptyListing_ReportsMissingMR
// verifies the status recheck reports a merge request GitLab does not list
// rather than dereferencing an empty result.
func TestGetLiveMergeRequestWithStatusRecheck_EmptyListing_ReportsMissingMR(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	defer server.Close()
	_, err := getLiveMergeRequestWithStatusRecheck(t.Context(), newFixtureTestClient(t, server.URL), "my-org/app", 7)
	if err == nil || !strings.Contains(err.Error(), "merge request !7 not found") {
		t.Fatalf("getLiveMergeRequestWithStatusRecheck() error = %v, want missing merge request", err)
	}
}

// TestWaitForPipelineJobStatus_ListingError_ReportsContext verifies the shared
// job wait reports the listing failure under the caller's context label.
func TestWaitForPipelineJobStatus_ListingError_ReportsContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"message":"500"}`)
	}))
	defer server.Close()
	_, err := waitForFailedJob(t.Context(), newFixtureTestClient(t, server.URL), "my-org/app", 7)
	if err == nil || !strings.Contains(err.Error(), "prepare failed-job fixture jobs") {
		t.Fatalf("waitForFailedJob() error = %v, want the listing context", err)
	}
}

// TestEnsureLiveProjectActive_NilClientAndLookupFailure verifies the guard is
// a no-op without a client and reports a lookup failure with the project path.
func TestEnsureLiveProjectActive_NilClientAndLookupFailure(t *testing.T) {
	if err := ensureLiveProjectActive(t.Context(), nil); err != nil {
		t.Fatalf("ensureLiveProjectActive(nil) error = %v, want nil", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"message":"500"}`)
	}))
	defer server.Close()
	err := ensureLiveProjectActive(t.Context(), newFixtureTestClient(t, server.URL))
	if err == nil || !strings.Contains(err.Error(), "get project "+liveFixtureProjectPath) {
		t.Fatalf("ensureLiveProjectActive() error = %v, want lookup failure", err)
	}
}

// TestSplitFixtureSetupAction_RejectsMalformedActionIDs verifies the meta-tool
// fallback only splits a canonical domain.action identifier.
func TestSplitFixtureSetupAction_RejectsMalformedActionIDs(t *testing.T) {
	cases := []struct {
		action     string
		wantTool   string
		wantAction string
		wantOK     bool
	}{
		{action: "branch.create", wantTool: "gitlab_branch", wantAction: "create", wantOK: true},
		{action: "mr_review.discussion.resolve", wantTool: "gitlab_mr_review", wantAction: "discussion_resolve", wantOK: true},
		{action: "nodot"},
		{action: ".create"},
		{action: "branch."},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			tool, action, ok := splitFixtureSetupAction(tc.action)
			if ok != tc.wantOK || tool != tc.wantTool || action != tc.wantAction {
				t.Fatalf("splitFixtureSetupAction(%q) = %q, %q, %t; want %q, %q, %t", tc.action, tool, action, ok, tc.wantTool, tc.wantAction, tc.wantOK)
			}
		})
	}
}

// TestCallFixtureSetupTool_IgnoredErrors_AreTreatedAsSuccess verifies a
// fixture setup call whose error text matches an ignored substring is accepted
// and any other error is reported with the action name.
func TestCallFixtureSetupTool_IgnoredErrors_AreTreatedAsSuccess(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "fixture-ignored", Version: "0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: dynamicExecuteActionTool, Description: "dynamic execute"}, func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Branch already exists"}}}, nil, nil
	})
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(t.Context(), serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "fixture-ignored-client", Version: "0"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	if ignoredErr := callFixtureSetupTool(t.Context(), session, config.ToolSurfaceDynamic, "branch.create", nil, "already exists"); ignoredErr != nil {
		t.Fatalf("callFixtureSetupTool(ignored) error = %v, want nil", ignoredErr)
	}
	reportedErr := callFixtureSetupTool(t.Context(), session, config.ToolSurfaceDynamic, "branch.create", nil)
	if reportedErr == nil || !strings.Contains(reportedErr.Error(), "prepare fixture branch.create: Branch already exists") {
		t.Fatalf("callFixtureSetupTool() error = %v, want the reported failure", reportedErr)
	}
}

// TestTerraformStateUnlockProjectID_PrefersInProjectMarker verifies the
// Terraform unlock project identifier is read from the "in project" marker and
// falls back to the generic project marker.
func TestTerraformStateUnlockProjectID_PrefersInProjectMarker(t *testing.T) {
	cases := []struct {
		name   string
		prompt string
		want   string
		wantOK bool
	}{
		{name: "in project marker", prompt: "Unlock Terraform state `prod` in project `my-org/app`.", want: "my-org/app", wantOK: true},
		{name: "generic project marker", prompt: "Unlock state for project `my-org/other`.", want: "my-org/other", wantOK: true},
		{name: "no marker", prompt: "Unlock the state."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := terraformStateUnlockProjectID(tc.prompt)
			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("terraformStateUnlockProjectID(%q) = %q, %t; want %q, %t", tc.prompt, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// TestLiveGitLabHTTPClient_HonorsSkipTLSVerify verifies the helper returns the
// default client when TLS verification stays on, a custom transport when it is
// skipped, and an error for an unparseable setting.
func TestLiveGitLabHTTPClient_HonorsSkipTLSVerify(t *testing.T) {
	cases := []struct {
		name        string
		value       string
		wantDefault bool
		wantErr     bool
	}{
		{name: "unset", value: "", wantDefault: true},
		{name: "false", value: "false", wantDefault: true},
		{name: "true", value: "true"},
		{name: "invalid", value: "maybe", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITLAB_SKIP_TLS_VERIFY", tc.value)
			client, err := liveGitLabHTTPClient()
			if (err != nil) != tc.wantErr {
				t.Fatalf("liveGitLabHTTPClient() error = %v, want error = %t", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			if (client == http.DefaultClient) != tc.wantDefault {
				t.Fatalf("liveGitLabHTTPClient() default client = %t, want %t", client == http.DefaultClient, tc.wantDefault)
			}
		})
	}
}
