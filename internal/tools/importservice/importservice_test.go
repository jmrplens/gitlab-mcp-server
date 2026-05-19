// importservice_test.go contains unit tests for the importservice MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package importservice

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// errExpectedErr identifies the err expected err constant used by this package.
const errExpectedErr = "expected error"

// testGHPToken identifies the test ghp token constant used by this package.
const testGHPToken = "ghp_token"

// testNamespace identifies the test namespace constant used by this package.
const testNamespace = "ns"

// testMyRepoName identifies the test my repo name constant used by this package.
const testMyRepoName = "my-repo"

// testBBSRepoName identifies the test bbs repo name constant used by this package.
const testBBSRepoName = "bbs-repo"

// TestImportFromGitHub verifies ImportFromGitHub.
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

// TestImportFromGitHub_InvalidRepoID verifies ImportFromGitHub when invalid repo ID.
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

// TestImportFromGitHub_Error verifies ImportFromGitHub when error.
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

// TestCancelGitHubImport verifies CancelGitHubImport.
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

// TestCancelGitHubImport_InvalidProjectID verifies CancelGitHubImport when invalid project ID.
func TestCancelGitHubImport_InvalidProjectID(t *testing.T) {
	_, err := CancelGitHubImport(t.Context(), nil, CancelGitHubImportInput{ProjectID: -1})
	if err == nil {
		t.Fatal("expected error for negative project_id")
	}
	if !strings.Contains(err.Error(), "project_id") {
		t.Errorf("expected error to mention 'project_id', got %q", err.Error())
	}
}

// TestCancelGitHubImport_Error verifies CancelGitHubImport when error.
func TestCancelGitHubImport_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := CancelGitHubImport(t.Context(), client, CancelGitHubImportInput{ProjectID: 999})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestImportGists verifies ImportGists.
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

// TestImportGists_Error verifies ImportGists when error.
func TestImportGists_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	err := ImportGists(t.Context(), client, ImportGistsInput{PersonalAccessToken: "bad"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestImportFromBitbucketCloud verifies ImportFromBitbucketCloud.
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

// TestImportFromBitbucketCloud_Error verifies ImportFromBitbucketCloud when error.
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

// TestImportFromBitbucketServer verifies ImportFromBitbucketServer.
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

// TestImportFromBitbucketServer_Error verifies ImportFromBitbucketServer when error.
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

// TestFormatGitHubImport verifies FormatGitHubImport.
func TestFormatGitHubImport(t *testing.T) {
	out := &GitHubImportOutput{ID: 1, Name: testMyRepoName, FullPath: "ns/my-repo", ImportStatus: "scheduled"}
	md := FormatGitHubImport(out)
	if !strings.Contains(md, testMyRepoName) {
		t.Errorf("expected markdown to contain '%s'", testMyRepoName)
	}
}

// TestFormatBitbucketServerImport verifies FormatBitbucketServerImport.
func TestFormatBitbucketServerImport(t *testing.T) {
	out := &BitbucketServerImportOutput{ID: 3, Name: testBBSRepoName, FullPath: "ns/bbs-repo"}
	md := FormatBitbucketServerImport(out)
	if !strings.Contains(md, testBBSRepoName) {
		t.Errorf("expected markdown to contain '%s'", testBBSRepoName)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// ImportFromGitHub — optional fields
// ---------------------------------------------------------------------------.

// TestImportFromGitHub_WithAllOptionalFields verifies ImportFromGitHub when with all optional fields.
func TestImportFromGitHub_WithAllOptionalFields(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/import/github" {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			capturedBody = string(body)
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
	for _, want := range []string{"personal_access_token", "repo_id", "target_namespace", "new_name", "github_hostname", "timeout_strategy"} {
		if !strings.Contains(capturedBody, want) {
			t.Errorf("request body missing field %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// CancelGitHubImport — API error (400)
// ---------------------------------------------------------------------------.

// TestCancelGitHubImport_APIError400 verifies CancelGitHubImport when API error 400.
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

// TestImportGists_APIError400 verifies ImportGists when API error 400.
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

// TestImportFromBitbucketCloud_WithOptionalFields verifies ImportFromBitbucketCloud when with optional fields.
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

// TestImportFromBitbucketServer_WithOptionalFields verifies ImportFromBitbucketServer when with optional fields.
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

// TestFormatGitHubImport_WithHumanStatus verifies FormatGitHubImport when with human status.
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

// TestFormatCancelledImport verifies FormatCancelledImport.
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

// TestFormatBitbucketCloudImport verifies FormatBitbucketCloudImport.
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

// TestFormatImportGists verifies FormatImportGists.
func TestFormatImportGists(t *testing.T) {
	md := FormatImportGists()
	if !strings.Contains(md, "gists") {
		t.Errorf("expected 'gists' in output, got %q", md)
	}
}

// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata verifies canonical metadata for import service actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)

	if len(specs) != 5 {
		t.Fatalf("len(ActionSpecs) = %d, want 5", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "importservice" {
			t.Errorf("OwnerPackage for %s = %q, want importservice", spec.Name, spec.OwnerPackage)
		}
		if spec.IndividualTool.Name == "" {
			t.Errorf("IndividualTool.Name for %s is empty", spec.Name)
		}
	}
	if !importServiceSpecsByTool(t, specs)["gitlab_cancel_github_import"].Idempotent {
		t.Error("cancel GitHub import action should be idempotent")
	}
}

// ---------------------------------------------------------------------------
// MCP round-trip — all tools
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRoutes validates all import service routes through the catalog.
func TestActionSpecs_CallRoutes(t *testing.T) {
	client := testutil.NewTestClient(t, importHandler())
	byTool := importServiceSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"import_github", "gitlab_import_from_github", map[string]any{
			"personal_access_token": "ghp_token",
			"repo_id":               int64(12345),
			"target_namespace":      "ns",
		}},
		{"cancel_github", "gitlab_cancel_github_import", map[string]any{
			"project_id": int64(1),
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
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MCP round-trip — meta tool
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// Helpers: MCP session factories
// ---------------------------------------------------------------------------.

// importHandler supports import handler assertions in importservice tests.
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

// TestActionSpecs_ErrorPaths covers error returns from import service routes.
func TestActionSpecs_ErrorPaths(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, handler)
	byTool := importServiceSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_import_from_github", map[string]any{"personal_access_token": "tok", "repo_id": int64(1), "target_namespace": "ns"}},
		{"gitlab_cancel_github_import", map[string]any{"project_id": int64(1)}},
		{"gitlab_import_github_gists", map[string]any{"personal_access_token": "tok"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			_, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatalf("Route.Handler(%s) expected error, got nil", tt.name)
			}
		})
	}
}

// importServiceSpecsByTool supports import service specs by tool assertions in importservice tests.
func importServiceSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}
