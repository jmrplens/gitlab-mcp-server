// importservice_test.go contains unit tests for the importservice MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package importservice

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const errExpectedErr = "expected error"

const testGHPToken = "ghp_token"

const testNamespace = "ns"

const testMyRepoName = "my-repo"

const testBBSRepoName = "bbs-repo"

// TestImportFromGitHub verifies the behavior of import from git hub.
func TestImportFromGitHub(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v4/import/github" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"id":1,"name":"my-repo","full_path":"ns/my-repo","full_name":"ns / my-repo","import_source":"github.com/user/repo","import_status":"scheduled","human_import_status_name":"scheduled"}`)
	}))
	out, err := ImportFromGitHub(t.Context(), client, ImportFromGitHubInput{
		PersonalAccessToken: testGHPToken,
		RepoID:              12345,
		TargetNamespace:     testNamespace,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != testMyRepoName {
		t.Errorf("expected name '%s', got %q", testMyRepoName, out.Name)
	}
	if out.ImportStatus != "scheduled" {
		t.Errorf("expected import_status 'scheduled', got %q", out.ImportStatus)
	}
}

// TestImportFromGitHub_InvalidRepoID verifies the behavior of import from git hub invalid repo i d.
func TestImportFromGitHub_InvalidRepoID(t *testing.T) {
	_, err := ImportFromGitHub(t.Context(), nil, ImportFromGitHubInput{
		PersonalAccessToken: testGHPToken,
		RepoID:              0,
		TargetNamespace:     testNamespace,
	})
	if err == nil {
		t.Fatal("expected error for zero repo_id")
	}
	if !strings.Contains(err.Error(), "repo_id") {
		t.Errorf("expected error to mention 'repo_id', got %q", err.Error())
	}
}

// TestImportFromGitHub_Error verifies the behavior of import from git hub error.
func TestImportFromGitHub_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := ImportFromGitHub(t.Context(), client, ImportFromGitHubInput{
		PersonalAccessToken: testGHPToken,
		RepoID:              12345,
		TargetNamespace:     testNamespace,
	})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestCancelGitHubImport verifies the behavior of cancel git hub import.
func TestCancelGitHubImport(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v4/import/github/cancel" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"my-repo","full_path":"ns/my-repo","full_name":"ns / my-repo","import_source":"github.com/user/repo","import_status":"canceled","human_import_status_name":"canceled"}`)
	}))
	out, err := CancelGitHubImport(t.Context(), client, CancelGitHubImportInput{ProjectID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ImportStatus != "canceled" {
		t.Errorf("expected import_status 'canceled', got %q", out.ImportStatus)
	}
}

// TestCancelGitHubImport_InvalidProjectID verifies the behavior of cancel git hub import invalid project i d.
func TestCancelGitHubImport_InvalidProjectID(t *testing.T) {
	_, err := CancelGitHubImport(t.Context(), nil, CancelGitHubImportInput{ProjectID: -1})
	if err == nil {
		t.Fatal("expected error for negative project_id")
	}
	if !strings.Contains(err.Error(), "project_id") {
		t.Errorf("expected error to mention 'project_id', got %q", err.Error())
	}
}

// TestCancelGitHubImport_Error verifies the behavior of cancel git hub import error.
func TestCancelGitHubImport_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := CancelGitHubImport(t.Context(), client, CancelGitHubImportInput{ProjectID: 999})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestImportGists verifies the behavior of import gists.
func TestImportGists(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v4/import/github/gists" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	err := ImportGists(t.Context(), client, ImportGistsInput{PersonalAccessToken: testGHPToken})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestImportGists_Error verifies the behavior of import gists error.
func TestImportGists_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	err := ImportGists(t.Context(), client, ImportGistsInput{PersonalAccessToken: "bad"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestImportFromBitbucketCloud verifies the behavior of import from bitbucket cloud.
func TestImportFromBitbucketCloud(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v4/import/bitbucket" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"name":"bb-repo","full_path":"ns/bb-repo","full_name":"ns / bb-repo","import_source":"bitbucket.org/user/repo","import_status":"scheduled","human_import_status_name":"scheduled"}`)
	}))
	out, err := ImportFromBitbucketCloud(t.Context(), client, ImportFromBitbucketCloudInput{
		BitbucketUsername:    "user",
		BitbucketAppPassword: "pass",
		RepoPath:             "user/repo",
		TargetNamespace:      testNamespace,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "bb-repo" {
		t.Errorf("expected name 'bb-repo', got %q", out.Name)
	}
}

// TestImportFromBitbucketCloud_Error verifies the behavior of import from bitbucket cloud error.
func TestImportFromBitbucketCloud_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := ImportFromBitbucketCloud(t.Context(), client, ImportFromBitbucketCloudInput{
		BitbucketUsername:    "user",
		BitbucketAppPassword: "pass",
		RepoPath:             "user/repo",
		TargetNamespace:      testNamespace,
	})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestImportFromBitbucketServer verifies the behavior of import from bitbucket server.
func TestImportFromBitbucketServer(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v4/import/bitbucket_server" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"id":3,"name":"bbs-repo","full_path":"ns/bbs-repo","full_name":"ns / bbs-repo","refs_url":"refs"}`)
	}))
	out, err := ImportFromBitbucketServer(t.Context(), client, ImportFromBitbucketServerInput{
		BitbucketServerURL:      "https://bitbucket.example.com",
		BitbucketServerUsername: "admin",
		PersonalAccessToken:     "pat123",
		BitbucketServerProject:  "PROJ",
		BitbucketServerRepo:     "repo",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != testBBSRepoName {
		t.Errorf("expected name '%s', got %q", testBBSRepoName, out.Name)
	}
}

// TestImportFromBitbucketServer_Error verifies the behavior of import from bitbucket server error.
func TestImportFromBitbucketServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := ImportFromBitbucketServer(t.Context(), client, ImportFromBitbucketServerInput{
		BitbucketServerURL:      "https://bitbucket.example.com",
		BitbucketServerUsername: "admin",
		PersonalAccessToken:     "pat123",
		BitbucketServerProject:  "PROJ",
		BitbucketServerRepo:     "repo",
	})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestFormatGitHubImport verifies the behavior of format git hub import.
func TestFormatGitHubImport(t *testing.T) {
	out := &GitHubImportOutput{ID: 1, Name: testMyRepoName, FullPath: "ns/my-repo", ImportStatus: "scheduled"}
	md := FormatGitHubImport(out)
	if !strings.Contains(md, testMyRepoName) {
		t.Errorf("expected markdown to contain '%s'", testMyRepoName)
	}
}

// TestFormatBitbucketServerImport verifies the behavior of format bitbucket server import.
func TestFormatBitbucketServerImport(t *testing.T) {
	out := &BitbucketServerImportOutput{ID: 3, Name: testBBSRepoName, FullPath: "ns/bbs-repo"}
	md := FormatBitbucketServerImport(out)
	if !strings.Contains(md, testBBSRepoName) {
		t.Errorf("expected markdown to contain '%s'", testBBSRepoName)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// ImportFromGitHub — optional fields
// ---------------------------------------------------------------------------.

// TestImportFromGitHub_WithAllOptionalFields verifies the behavior of import from git hub with all optional fields.
func TestImportFromGitHub_WithAllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/import/github" {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":1,"name":"imported","full_path":"ns/imported","full_name":"ns / imported","import_source":"github.com/user/repo","import_status":"scheduled"}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ImportFromGitHub(t.Context(), client, ImportFromGitHubInput{
		PersonalAccessToken: "ghp_token",
		RepoID:              12345,
		TargetNamespace:     "ns",
		NewName:             "imported",
		GitHubHostname:      "github.example.com",
		TimeoutStrategy:     "optimistic",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "imported" {
		t.Errorf("expected name 'imported', got %q", out.Name)
	}
}

// ---------------------------------------------------------------------------
// CancelGitHubImport — API error (400)
// ---------------------------------------------------------------------------.

// TestCancelGitHubImport_APIError400 verifies the behavior of cancel git hub import a p i error400.
func TestCancelGitHubImport_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := CancelGitHubImport(t.Context(), client, CancelGitHubImportInput{ProjectID: 1})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// ImportGists — API error (400)
// ---------------------------------------------------------------------------.

// TestImportGists_APIError400 verifies the behavior of import gists a p i error400.
func TestImportGists_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	err := ImportGists(t.Context(), client, ImportGistsInput{PersonalAccessToken: "bad"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// ImportFromBitbucketCloud — optional fields
// ---------------------------------------------------------------------------.

// TestImportFromBitbucketCloud_WithOptionalFields verifies the behavior of import from bitbucket cloud with optional fields.
func TestImportFromBitbucketCloud_WithOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/import/bitbucket" {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"name":"bb-new","full_path":"ns/bb-new","full_name":"ns / bb-new","import_source":"bitbucket.org/user/repo","import_status":"scheduled"}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ImportFromBitbucketCloud(t.Context(), client, ImportFromBitbucketCloudInput{
		BitbucketUsername:    "user",
		BitbucketAppPassword: "pass",
		RepoPath:             "user/repo",
		TargetNamespace:      "ns",
		NewName:              "bb-new",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "bb-new" {
		t.Errorf("expected name 'bb-new', got %q", out.Name)
	}
}

// ---------------------------------------------------------------------------
// ImportFromBitbucketServer — optional fields
// ---------------------------------------------------------------------------.

// TestImportFromBitbucketServer_WithOptionalFields verifies the behavior of import from bitbucket server with optional fields.
func TestImportFromBitbucketServer_WithOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/import/bitbucket_server" {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":3,"name":"bbs-new","full_path":"ns/bbs-new","full_name":"ns / bbs-new"}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ImportFromBitbucketServer(t.Context(), client, ImportFromBitbucketServerInput{
		BitbucketServerURL:      "https://bitbucket.example.com",
		BitbucketServerUsername: "admin",
		PersonalAccessToken:     "pat123",
		BitbucketServerProject:  "PROJ",
		BitbucketServerRepo:     "repo",
		NewName:                 "bbs-new",
		NewNamespace:            "ns",
		TimeoutStrategy:         "pessimistic",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "bbs-new" {
		t.Errorf("expected name 'bbs-new', got %q", out.Name)
	}
}

// ---------------------------------------------------------------------------
// Formatters — additional branches
// ---------------------------------------------------------------------------.

// TestFormatGitHubImport_WithHumanStatus verifies the behavior of format git hub import with human status.
func TestFormatGitHubImport_WithHumanStatus(t *testing.T) {
	out := &GitHubImportOutput{
		ID: 1, Name: "my-repo", FullPath: "ns/my-repo",
		ImportSource: "github.com/user/repo", ImportStatus: "scheduled",
		HumanImportStatusName: "Importing...",
	}
	md := FormatGitHubImport(out)
	if !strings.Contains(md, "Importing...") {
		t.Errorf("expected human status name in output")
	}
}

// TestFormatCancelledImport verifies the behavior of format cancelled import.
func TestFormatCancelledImport(t *testing.T) {
	out := &CancelledImportOutput{
		ID: 1, Name: "my-repo", FullPath: "ns/my-repo",
		ImportStatus: "canceled",
	}
	md := FormatCancelledImport(out)
	if !strings.Contains(md, "canceled") {
		t.Errorf("expected 'canceled' in output")
	}
	if !strings.Contains(md, "my-repo") {
		t.Errorf("expected 'my-repo' in output")
	}
}

// TestFormatBitbucketCloudImport verifies the behavior of format bitbucket cloud import.
func TestFormatBitbucketCloudImport(t *testing.T) {
	out := &BitbucketCloudImportOutput{
		ID: 2, Name: "bb-repo", FullPath: "ns/bb-repo",
		ImportSource: "bitbucket.org/user/repo", ImportStatus: "scheduled",
	}
	md := FormatBitbucketCloudImport(out)
	if !strings.Contains(md, "bb-repo") {
		t.Errorf("expected 'bb-repo' in output")
	}
	if !strings.Contains(md, "scheduled") {
		t.Errorf("expected 'scheduled' in output")
	}
}

// TestFormatImportGists verifies the behavior of format import gists.
func TestFormatImportGists(t *testing.T) {
	md := FormatImportGists()
	if !strings.Contains(md, "gists") {
		t.Errorf("expected 'gists' in output, got %q", md)
	}
}

// ---------------------------------------------------------------------------
// RegisterTools + RegisterMeta — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterMeta_NoPanic verifies the behavior of register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// ---------------------------------------------------------------------------
// MCP round-trip — all tools
// ---------------------------------------------------------------------------.

// TestMCPRoundTrip_AllTools validates m c p round trip all tools across multiple scenarios using table-driven subtests.
func TestMCPRoundTrip_AllTools(t *testing.T) {
	session := newImportMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"import_github", "gitlab_import_from_github", map[string]any{
			"personal_access_token": "ghp_token",
			"repo_id":               float64(12345),
			"target_namespace":      "ns",
		}},
		{"cancel_github", "gitlab_cancel_github_import", map[string]any{
			"project_id": float64(1),
		}},
		{"import_gists", "gitlab_import_github_gists", map[string]any{
			"personal_access_token": "ghp_token",
		}},
		{"import_bitbucket_cloud", "gitlab_import_from_bitbucket_cloud", map[string]any{
			"bitbucket_username":     "user",
			"bitbucket_app_password": "pass",
			"repo_path":              "user/repo",
			"target_namespace":       "ns",
		}},
		{"import_bitbucket_server", "gitlab_import_from_bitbucket_server", map[string]any{
			"bitbucket_server_url":      "https://bitbucket.example.com",
			"bitbucket_server_username": "admin",
			"personal_access_token":     "pat123",
			"bitbucket_server_project":  "PROJ",
			"bitbucket_server_repo":     "repo",
		}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.tool, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.tool, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MCP round-trip — meta tool
// ---------------------------------------------------------------------------.

// TestMCPRound_TripMetaTool validates m c p round trip meta tool across multiple scenarios using table-driven subtests.
func TestMCPRound_TripMetaTool(t *testing.T) {
	session := newImportMetaMCPSession(t)
	ctx := context.Background()

	actions := []struct {
		name   string
		action string
		params map[string]any
	}{
		{"from_github", "from_github", map[string]any{
			"personal_access_token": "ghp_token",
			"repo_id":               float64(12345),
			"target_namespace":      "ns",
		}},
		{"cancel_github", "cancel_github", map[string]any{
			"project_id": float64(1),
		}},
		{"github_gists", "github_gists", map[string]any{
			"personal_access_token": "ghp_token",
		}},
		{"from_bitbucket_cloud", "from_bitbucket_cloud", map[string]any{
			"bitbucket_username":     "user",
			"bitbucket_app_password": "pass",
			"repo_path":              "user/repo",
			"target_namespace":       "ns",
		}},
		{"from_bitbucket_server", "from_bitbucket_server", map[string]any{
			"bitbucket_server_url":      "https://bitbucket.example.com",
			"bitbucket_server_username": "admin",
			"personal_access_token":     "pat123",
			"bitbucket_server_project":  "PROJ",
			"bitbucket_server_repo":     "repo",
		}},
	}

	for _, tt := range actions {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name: "gitlab_import",
				Arguments: map[string]any{
					"action": tt.action,
					"params": tt.params,
				},
			})
			if err != nil {
				t.Fatalf("CallTool(gitlab_import/%s) error: %v", tt.action, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(gitlab_import/%s) returned error: %s", tt.action, tc.Text)
					}
				}
				t.Fatalf("CallTool(gitlab_import/%s) returned IsError=true", tt.action)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helpers: MCP session factories
// ---------------------------------------------------------------------------.

// importHandler is an internal helper for the importservice package.
func importHandler() *http.ServeMux {
	handler := http.NewServeMux()

	ghJSON := `{"id":1,"name":"my-repo","full_path":"ns/my-repo","full_name":"ns / my-repo","import_source":"github.com/user/repo","import_status":"scheduled","human_import_status_name":"scheduled"}`
	cancelJSON := `{"id":1,"name":"my-repo","full_path":"ns/my-repo","full_name":"ns / my-repo","import_source":"github.com/user/repo","import_status":"canceled"}`
	bbCloudJSON := `{"id":2,"name":"bb-repo","full_path":"ns/bb-repo","full_name":"ns / bb-repo","import_source":"bitbucket.org/user/repo","import_status":"scheduled"}`
	bbServerJSON := `{"id":3,"name":"bbs-repo","full_path":"ns/bbs-repo","full_name":"ns / bbs-repo"}`

	handler.HandleFunc("POST /api/v4/import/github", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, ghJSON)
	})
	handler.HandleFunc("POST /api/v4/import/github/cancel", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, cancelJSON)
	})
	handler.HandleFunc("POST /api/v4/import/github/gists", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	handler.HandleFunc("POST /api/v4/import/bitbucket", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, bbCloudJSON)
	})
	handler.HandleFunc("POST /api/v4/import/bitbucket_server", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, bbServerJSON)
	})

	return handler
}

// TestMCPRoundTrip_ErrorPaths covers the error return paths in register.go
// handlers when the GitLab API returns an error.
func TestMCPRoundTrip_ErrorPaths(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_import_from_github", map[string]any{"personal_access_token": "tok", "repo_id": float64(1), "target_namespace": "ns"}},
		{"gitlab_cancel_github_import", map[string]any{"project_id": "1"}},
		{"gitlab_import_github_gists", map[string]any{"personal_access_token": "tok"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("unexpected transport error: %v", err)
			}
			if result == nil || !result.IsError {
				t.Fatalf("expected error result for %s with 500 backend", tt.name)
			}
		})
	}
}

// newImportMCPSession is an internal helper for the importservice package.
func newImportMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	client := testutil.NewTestClient(t, importHandler())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// newImportMetaMCPSession is an internal helper for the importservice package.
func newImportMetaMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	client := testutil.NewTestClient(t, importHandler())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
