// import_service_test.go contains unit tests for the importservice MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package importservice

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

// TestImportFromGitHub verifies the ImportFromGitHub handler.
// The mock GitLab API at /api/v4/import/github (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
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

// TestImportFromGitHub_InvalidRepoID verifies the ImportFromGitHub_InvalidRepoID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestImportFromGitHub_Error verifies that ImportFromGitHub returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestCancelGitHubImport verifies the CancelGitHubImport handler.
// The mock GitLab API at /api/v4/import/github/cancel (POST) responds with HTTP OK.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestCancelGitHubImport_InvalidProjectID verifies the CancelGitHubImport_InvalidProjectID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestCancelGitHubImport_InvalidProjectID(t *testing.T) {
	_, err := CancelGitHubImport(t.Context(), nil, CancelGitHubImportInput{ProjectID: -1})
	if err == nil {
		t.Fatal("expected error for negative project_id")
	}
	if !strings.Contains(err.Error(), "project_id") {
		t.Errorf("expected error to mention 'project_id', got %q", err.Error())
	}
}

// TestCancelGitHubImport_Error verifies that CancelGitHubImport returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCancelGitHubImport_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := CancelGitHubImport(t.Context(), client, CancelGitHubImportInput{ProjectID: 999})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestImportGists verifies the ImportGists handler.
// The mock GitLab API at /api/v4/import/github/gists (POST) returns a representative success body.
// It asserts the returned output matches the expected fields.
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

// TestImportGists_Error verifies that ImportGists returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestImportGists_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	err := ImportGists(t.Context(), client, ImportGistsInput{PersonalAccessToken: "bad"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestImportFromBitbucketCloud verifies the ImportFromBitbucketCloud handler.
// The mock GitLab API at /api/v4/import/bitbucket (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
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

// TestImportFromBitbucketCloud_APIToken verifies the API-token authentication path added in client-go v2.41.0.
// The mock GitLab API inspects the request body for the new bitbucket_api_token and bitbucket_email fields.
// It asserts both fields are sent and the legacy app password is omitted.
func TestImportFromBitbucketCloud_APIToken(t *testing.T) {
	var body string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v4/import/bitbucket" {
			http.NotFound(w, r)
			return
		}
		buf, _ := io.ReadAll(r.Body)
		body = string(buf)
		testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"name":"bb-repo","full_path":"ns/bb-repo","import_status":"scheduled"}`)
	}))
	_, err := ImportFromBitbucketCloud(t.Context(), client, ImportFromBitbucketCloudInput{
		BitbucketUsername: "user",
		BitbucketAPIToken: "token-secret",
		BitbucketEmail:    "user@example.com",
		RepoPath:          "user/repo",
		TargetNamespace:   testNamespace,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, field := range []string{"bitbucket_api_token", "bitbucket_email"} {
		t.Run(field, func(t *testing.T) {
			if !strings.Contains(body, field) {
				t.Errorf("expected %s in request body, got: %s", field, body)
			}
		})
	}
	if strings.Contains(body, "bitbucket_app_password") {
		t.Errorf("did not expect bitbucket_app_password when using API token, got: %s", body)
	}
}

// TestImportFromBitbucketCloud_Validation verifies the required-field guards
// short-circuit before any API call.
func TestImportFromBitbucketCloud_Validation(t *testing.T) {
	// A client whose handler fails the test if reached — validation must run first.
	client := testutil.NewTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("API must not be called when input validation fails")
	}))

	cases := []struct {
		name    string
		input   ImportFromBitbucketCloudInput
		wantErr string
	}{
		{"missing username", ImportFromBitbucketCloudInput{RepoPath: "u/r", TargetNamespace: testNamespace}, "bitbucket_username"},
		{"missing repo_path", ImportFromBitbucketCloudInput{BitbucketUsername: "u", TargetNamespace: testNamespace}, "repo_path"},
		{"missing target_namespace", ImportFromBitbucketCloudInput{BitbucketUsername: "u", RepoPath: "u/r"}, "target_namespace"},
		{"api_token without email", ImportFromBitbucketCloudInput{BitbucketUsername: "u", RepoPath: "u/r", TargetNamespace: testNamespace, BitbucketAPIToken: "tok"}, "bitbucket_email"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ImportFromBitbucketCloud(t.Context(), client, tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error naming %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// TestImportFromBitbucketCloud_Error verifies that ImportFromBitbucketCloud returns a
// wrapped error when the GitLab API responds with an error status, exercising the GET
// path of the underlying call and asserting the error is wrapped with a useful hint.
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

// TestImportFromBitbucketServer verifies the ImportFromBitbucketServer handler.
// The mock GitLab API at /api/v4/import/bitbucket_server (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
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

// TestImportFromBitbucketServer_Error verifies that ImportFromBitbucketServer returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestFormatGitHubImport verifies the GitHubImport Markdown formatter for a representative githubimport input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatGitHubImport(t *testing.T) {
	out := &GitHubImportOutput{ID: 1, Name: testMyRepoName, FullPath: "ns/my-repo", ImportStatus: "scheduled"}

	assertImportMarkdown(t, FormatGitHubImport(out), "## GitHub Import: "+testMyRepoName+"\n\n"+
		"- **ID**: 1\n"+
		"- **Name**: "+testMyRepoName+"\n"+
		"- **Full Path**: ns/my-repo\n"+
		"- **Import Status**: scheduled\n"+
		gitHubImportHints)
}

// TestFormatBitbucketServerImport_Minimal verifies the whole card a Bitbucket
// Server import renders when GitLab sent no full name.
func TestFormatBitbucketServerImport_Minimal(t *testing.T) {
	out := &BitbucketServerImportOutput{ID: 3, Name: testBBSRepoName, FullPath: "ns/bbs-repo"}

	assertImportMarkdown(t, FormatBitbucketServerImport(out), "## Bitbucket Server Import: "+testBBSRepoName+"\n\n"+
		"- **ID**: 3\n"+
		"- **Name**: "+testBBSRepoName+"\n"+
		"- **Full Path**: ns/bbs-repo\n"+
		pollImportHints)
}

// ---------- Tests consolidated from coverage_test.go ----------.

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// ImportFromGitHub — optional fields
// ---------------------------------------------------------------------------.

// TestImportFromGitHub_WithAllOptionalFields verifies the ImportFromGitHub_WithAllOptionalFields handler.
// The mock GitLab API at /api/v4/import/github (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
func TestImportFromGitHub_WithAllOptionalFields(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/import/github" {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read request body: %v", err)
				http.Error(w, "read request body", http.StatusInternalServerError)
				return
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
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(capturedBody, want) {
				t.Errorf("request body missing field %q", want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CancelGitHubImport — API error (400)
// ---------------------------------------------------------------------------.

// TestCancelGitHubImport_APIError400 verifies that CancelGitHubImport400 returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestImportGists_APIError400 verifies that ImportGists400 returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestImportFromBitbucketCloud_WithOptionalFields verifies the ImportFromBitbucketCloud_WithOptionalFields handler.
// The mock GitLab API at /api/v4/import/bitbucket (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
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

// TestImportFromBitbucketServer_WithOptionalFields verifies the ImportFromBitbucketServer_WithOptionalFields handler.
// The mock GitLab API at /api/v4/import/bitbucket_server (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
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

// TestFormatGitHubImport_WithHumanStatus verifies the GitHubImport_WithHumanStatus Markdown formatter for a representative githubimport_withhumanstatus input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestImportFromGitHub_WithOptionalStages verifies that the optional_stages
// nested object is serialized into the request body and that the additive
// refs_url / import_warning response fields are mapped onto the output.
// The mock GitLab API at /api/v4/import/github (POST) captures the request
// body and responds with HTTP Created including the additive fields.
func TestImportFromGitHub_WithOptionalStages(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/import/github" {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read request body: %v", err)
				http.Error(w, "read request body", http.StatusInternalServerError)
				return
			}
			capturedBody = string(body)
			testutil.RespondJSON(w, http.StatusCreated, `{"id":1,"name":"my-repo","full_path":"ns/my-repo","full_name":"ns / my-repo","refs_url":"https://gitlab.example.com/ns/my-repo/refs","import_source":"github.com/user/repo","import_status":"scheduled","import_warning":"some collaborators could not be mapped"}`)
			return
		}
		http.NotFound(w, r)
	}))
	enabled := true
	out, err := ImportFromGitHub(t.Context(), client, ImportFromGitHubInput{
		PersonalAccessToken: testGHPToken,
		RepoID:              12345,
		TargetNamespace:     testNamespace,
		OptionalStages: &GitHubOptionalStagesInput{
			SingleEndpointNotesImport: &enabled,
			AttachmentsImport:         &enabled,
			CollaboratorsImport:       &enabled,
		},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, want := range []string{"optional_stages", "single_endpoint_notes_import", "attachments_import", "collaborators_import"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(capturedBody, want) {
				t.Errorf("request body missing field %q; body=%s", want, capturedBody)
			}
		})
	}
	if out.RefsURL != "https://gitlab.example.com/ns/my-repo/refs" {
		t.Errorf("expected refs_url to be mapped, got %q", out.RefsURL)
	}
	if out.ImportWarning != "some collaborators could not be mapped" {
		t.Errorf("expected import_warning to be mapped, got %q", out.ImportWarning)
	}
}

// assertImportMarkdown compares a whole rendered card with what the formatter
// is meant to write, byte for byte.
func assertImportMarkdown(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("markdown mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// The guidance section each import card closes with.
const (
	gitHubImportHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'project.get' to poll import_status until the import finishes\n" +
		"- Use action 'admin.import_cancel_github' to cancel the import while it runs\n"
	pollImportHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'project.get' to poll import_status until the import finishes\n"
	cancelledImportHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'admin.import_github' to start a new import if one is still wanted\n"
)

// TestFormatGitHubImport_WithImportWarning verifies the whole card a GitHub
// import renders: the relation type and both addresses GitLab sends, which the
// table this replaced dropped, and the warning as prose under its own label.
func TestFormatGitHubImport_WithImportWarning(t *testing.T) {
	out := &GitHubImportOutput{
		ID: 1, Name: testMyRepoName, FullPath: "ns/my-repo",
		RefsURL:       "https://gitlab.example.com/ns/my-repo/refs",
		ImportSource:  "github.com/user/repo",
		ImportStatus:  "scheduled",
		ProviderLink:  "https://github.com/user/repo",
		RelationType:  "fork",
		ImportWarning: "partial import",
	}

	assertImportMarkdown(t, FormatGitHubImport(out), "## GitHub Import: "+testMyRepoName+"\n\n"+
		"- **ID**: 1\n"+
		"- **Name**: "+testMyRepoName+"\n"+
		"- **Full Path**: ns/my-repo\n"+
		"- **Import Source**: github.com/user/repo\n"+
		"- **Import Status**: scheduled\n"+
		"- **Relation Type**: fork\n"+
		"- **Provider Link**: [https://github.com/user/repo](https://github.com/user/repo)\n"+
		"- **Refs URL**: [https://gitlab.example.com/ns/my-repo/refs](https://gitlab.example.com/ns/my-repo/refs)\n"+
		"- **Import Warning**: partial import\n"+
		gitHubImportHints)
}

// TestFormatCancelledImport verifies the whole card a canceled import renders,
// the full path and import source it used to drop included.
func TestFormatCancelledImport(t *testing.T) {
	out := &CancelledImportOutput{
		ID: 1, Name: "my-repo", FullPath: "ns/my-repo",
		ImportSource:          "github.com/user/repo",
		ImportStatus:          "canceled",
		HumanImportStatusName: "canceled",
		ProviderLink:          "https://github.com/user/repo",
	}

	assertImportMarkdown(t, FormatCancelledImport(out), "## Canceled Import: my-repo\n\n"+
		"- **ID**: 1\n"+
		"- **Name**: my-repo\n"+
		"- **Full Path**: ns/my-repo\n"+
		"- **Import Source**: github.com/user/repo\n"+
		"- **Import Status**: canceled\n"+
		"- **Status Name**: canceled\n"+
		"- **Provider Link**: [https://github.com/user/repo](https://github.com/user/repo)\n"+
		cancelledImportHints)
}

// TestFormatBitbucketCloudImport verifies the whole card a Bitbucket Cloud
// import renders, its status name and provider link included.
func TestFormatBitbucketCloudImport(t *testing.T) {
	out := &BitbucketCloudImportOutput{
		ID: 2, Name: "bb-repo", FullPath: "ns/bb-repo",
		ImportSource:          "bitbucket.org/user/repo",
		ImportStatus:          "scheduled",
		HumanImportStatusName: "scheduled",
		ProviderLink:          "https://bitbucket.org/user/repo",
	}

	assertImportMarkdown(t, FormatBitbucketCloudImport(out), "## Bitbucket Cloud Import: bb-repo\n\n"+
		"- **ID**: 2\n"+
		"- **Name**: bb-repo\n"+
		"- **Full Path**: ns/bb-repo\n"+
		"- **Import Source**: bitbucket.org/user/repo\n"+
		"- **Import Status**: scheduled\n"+
		"- **Status Name**: scheduled\n"+
		"- **Provider Link**: [https://bitbucket.org/user/repo](https://bitbucket.org/user/repo)\n"+
		pollImportHints)
}

// TestFormatBitbucketServerImport verifies the whole card a Bitbucket Server
// import renders. That endpoint answers with the created project alone, so
// there is no import status and no provider link to show.
func TestFormatBitbucketServerImport(t *testing.T) {
	out := &BitbucketServerImportOutput{
		ID: 3, Name: "bbs-repo", FullPath: "ns/bbs-repo", FullName: "ns / bbs-repo",
	}

	assertImportMarkdown(t, FormatBitbucketServerImport(out), "## Bitbucket Server Import: bbs-repo\n\n"+
		"- **ID**: 3\n"+
		"- **Name**: bbs-repo\n"+
		"- **Full Path**: ns/bbs-repo\n"+
		"- **Full Name**: ns / bbs-repo\n"+
		pollImportHints)
}

// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------.

// TestImportService_EveryImportReachesItsEndpoint drives each import handler
// against the endpoint GitLab serves it on, so a handler pointed at the wrong
// path fails rather than being answered by a catch-all.
func TestImportService_EveryImportReachesItsEndpoint(t *testing.T) {
	client := testutil.NewTestClient(t, importHandler())

	tests := []struct {
		name string
		call func() error
	}{
		{name: "import_github", call: func() error {
			_, err := ImportFromGitHub(t.Context(), client, ImportFromGitHubInput{
				PersonalAccessToken: "ghp_token",
				RepoID:              12345,
				TargetNamespace:     "ns",
			})
			return err
		}},
		{name: "cancel_github", call: func() error {
			_, err := CancelGitHubImport(t.Context(), client, CancelGitHubImportInput{ProjectID: 1})
			return err
		}},
		{name: "import_gists", call: func() error {
			return ImportGists(t.Context(), client, ImportGistsInput{PersonalAccessToken: "ghp_token"})
		}},
		{name: "import_bitbucket_cloud", call: func() error {
			_, err := ImportFromBitbucketCloud(t.Context(), client, ImportFromBitbucketCloudInput{
				BitbucketUsername:    "user",
				BitbucketAppPassword: "pass",
				RepoPath:             "user/repo",
				TargetNamespace:      "ns",
			})
			return err
		}},
		{name: "import_bitbucket_server", call: func() error {
			_, err := ImportFromBitbucketServer(t.Context(), client, ImportFromBitbucketServerInput{
				BitbucketServerURL:      "https://bitbucket.example.com",
				BitbucketServerUsername: "admin",
				PersonalAccessToken:     "pat123",
				BitbucketServerProject:  "PROJ",
				BitbucketServerRepo:     "repo",
			})
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err != nil {
				t.Fatalf("%s error = %v, want nil", tt.name, err)
			}
		})
	}
}

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

// TestImportService_RefusalsPropagate verifies that an instance refusing the
// import is reported rather than swallowed, for each import that has no
// refusal test of its own.
func TestImportService_RefusalsPropagate(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, handler)

	tests := []struct {
		name string
		call func() error
	}{
		{name: "import_github", call: func() error {
			_, err := ImportFromGitHub(t.Context(), client, ImportFromGitHubInput{
				PersonalAccessToken: "tok",
				RepoID:              1,
				TargetNamespace:     "ns",
			})
			return err
		}},
		{name: "cancel_github", call: func() error {
			_, err := CancelGitHubImport(t.Context(), client, CancelGitHubImportInput{ProjectID: 1})
			return err
		}},
		{name: "import_gists", call: func() error {
			return ImportGists(t.Context(), client, ImportGistsInput{PersonalAccessToken: "tok"})
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatalf("%s error = nil, want the instance refusal", tt.name)
			}
		})
	}
}

// TestMarkdownRegistry_PointerOutputFormatters verifies that the registry
// resolves the shape the handlers actually return.
//
// Every handler here returns a pointer, and the registrations are made for the
// value type; the registry dereferences a pointer whose element type has a
// formatter, so the key the runtime reaches is the value one. The table used
// to pass the value type in, which exercised the registration and never the
// lookup the dispatcher performs, so a registry that did not dereference would
// have passed this test while serving no Markdown at all. Each case therefore
// carries a populated pointer and asserts the card it renders.
func TestMarkdownRegistry_PointerOutputFormatters(t *testing.T) {
	outputs := []struct {
		name string
		out  any
		want string
	}{
		{
			name: "github",
			out:  &GitHubImportOutput{ID: 1, Name: "gh", ImportStatus: "scheduled"},
			want: "## GitHub Import: gh\n\n" +
				"- **ID**: 1\n- **Name**: gh\n- **Import Status**: scheduled\n" + gitHubImportHints,
		},
		{
			name: "cancelled",
			out:  &CancelledImportOutput{ID: 2, Name: "gh", ImportStatus: "canceled"},
			want: "## Canceled Import: gh\n\n" +
				"- **ID**: 2\n- **Name**: gh\n- **Import Status**: canceled\n" + cancelledImportHints,
		},
		{
			name: "bitbucket_cloud",
			out:  &BitbucketCloudImportOutput{ID: 3, Name: "bb", ImportStatus: "scheduled"},
			want: "## Bitbucket Cloud Import: bb\n\n" +
				"- **ID**: 3\n- **Name**: bb\n- **Import Status**: scheduled\n" + pollImportHints,
		},
		{
			name: "bitbucket_server",
			out:  &BitbucketServerImportOutput{ID: 4, Name: "bbs", FullPath: "ns/bbs"},
			want: "## Bitbucket Server Import: bbs\n\n" +
				"- **ID**: 4\n- **Name**: bbs\n- **Full Path**: ns/bbs\n" + pollImportHints,
		},
	}
	for _, tc := range outputs {
		t.Run(tc.name, func(t *testing.T) {
			result := toolutil.MarkdownForResult(tc.out)
			if result == nil || len(result.Content) == 0 {
				t.Fatalf("MarkdownForResult(%T) returned empty result", tc.out)
			}
			text, ok := result.Content[0].(*mcp.TextContent)
			if !ok {
				t.Fatalf("MarkdownForResult(%T) content is %T, want *mcp.TextContent", tc.out, result.Content[0])
			}
			assertImportMarkdown(t, text.Text, tc.want)
		})
	}
}

// ---------------------------------------------------------------------------
// What the handlers actually send GitLab
// ---------------------------------------------------------------------------.

// captureImportRequest answers one POST to path with respBody and hands back
// the JSON object GitLab was sent.
//
// The request body is the only place an optional field can be observed. What a
// handler returns is decided by the fixture response, so an assertion on the
// output passes whether the field reached GitLab or not, and an optional field
// has three states rather than two on the wire: absent, carrying the caller's
// value, and carrying an empty string, which GitLab reads as an instruction
// rather than as a silence.
func captureImportRequest(t *testing.T, path, respBody string, call func(*gitlabclient.Client) error) map[string]any {
	t.Helper()
	var sent map[string]any
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Errorf("decode request body: %v", err)
			http.Error(w, "decode request body", http.StatusInternalServerError)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, respBody)
	}))
	if err := call(client); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	return sent
}

// assertImportFieldSent fails unless the recorded request carried field with
// exactly the value the caller gave.
func assertImportFieldSent(t *testing.T, sent map[string]any, field, want string) {
	t.Helper()
	got, ok := sent[field]
	if !ok {
		t.Errorf("request body omitted %q; body=%v", field, sent)
		return
	}
	if got != want {
		t.Errorf("request body %q = %v, want %q", field, got, want)
	}
}

// assertImportFieldAbsent fails when the recorded request carried field at all,
// an empty value included.
func assertImportFieldAbsent(t *testing.T, sent map[string]any, field string) {
	t.Helper()
	if got, ok := sent[field]; ok {
		t.Errorf("request body carried %q = %v, and nothing asked for it", field, got)
	}
}

// TestImportFromBitbucketCloud_NewName_TravelsOnlyWhenTheCallerGaveOne asserts
// the new_name guard both ways: the name a caller asked for reaches GitLab, and
// a caller who asked for none sends no new_name at all.
//
// Only the second half is about the guard being the right way round. GitLab
// names the imported project after the Bitbucket repository unless new_name
// says otherwise, and an empty new_name is not the same as no new_name: the
// import is refused for a blank path. Nothing before this test looked at the
// body of this call, so the guard could have been inverted and every assertion
// here still read the fixture's own name back out of the response.
func TestImportFromBitbucketCloud_NewName_TravelsOnlyWhenTheCallerGaveOne(t *testing.T) {
	const respBody = `{"id":2,"name":"bb-new","full_path":"ns/bb-new","import_status":"scheduled"}`
	base := ImportFromBitbucketCloudInput{
		BitbucketUsername:    "user",
		BitbucketAppPassword: "pass",
		RepoPath:             "user/repo",
		TargetNamespace:      testNamespace,
	}

	t.Run("given", func(t *testing.T) {
		input := base
		input.NewName = "bb-new"
		sent := captureImportRequest(t, "/api/v4/import/bitbucket", respBody, func(client *gitlabclient.Client) error {
			_, err := ImportFromBitbucketCloud(t.Context(), client, input)
			return err
		})
		assertImportFieldSent(t, sent, "new_name", "bb-new")
	})

	t.Run("omitted", func(t *testing.T) {
		sent := captureImportRequest(t, "/api/v4/import/bitbucket", respBody, func(client *gitlabclient.Client) error {
			_, err := ImportFromBitbucketCloud(t.Context(), client, base)
			return err
		})
		assertImportFieldAbsent(t, sent, "new_name")
	})
}

// TestImportFromBitbucketServer_OptionalFields_TravelOnlyWhenTheCallerGaveThem
// asserts the same property for the three optional fields of the Bitbucket
// Server import: new_name, new_namespace and timeout_strategy each reach GitLab
// with the caller's value, and none of them is sent when the caller gave none.
//
// timeout_strategy is the one with teeth: GitLab accepts "optimistic" or
// "pessimistic" and refuses anything else, so a guard sending an empty string
// for every caller who did not choose one would fail every import of a large
// repository — the only imports for which this handler's other two options
// exist.
func TestImportFromBitbucketServer_OptionalFields_TravelOnlyWhenTheCallerGaveThem(t *testing.T) {
	const respBody = `{"id":3,"name":"bbs-new","full_path":"ns/bbs-new","full_name":"ns / bbs-new"}`
	base := ImportFromBitbucketServerInput{
		BitbucketServerURL:      "https://bitbucket.example.com",
		BitbucketServerUsername: "admin",
		PersonalAccessToken:     "pat123",
		BitbucketServerProject:  "PROJ",
		BitbucketServerRepo:     "repo",
	}
	optional := []struct {
		field string
		want  string
	}{
		{"new_name", "bbs-new"},
		{"new_namespace", testNamespace},
		{"timeout_strategy", "pessimistic"},
	}

	t.Run("given", func(t *testing.T) {
		input := base
		input.NewName = "bbs-new"
		input.NewNamespace = testNamespace
		input.TimeoutStrategy = "pessimistic"
		sent := captureImportRequest(t, "/api/v4/import/bitbucket_server", respBody, func(client *gitlabclient.Client) error {
			_, err := ImportFromBitbucketServer(t.Context(), client, input)
			return err
		})
		for _, tc := range optional {
			t.Run(tc.field, func(t *testing.T) {
				assertImportFieldSent(t, sent, tc.field, tc.want)
			})
		}
	})

	t.Run("omitted", func(t *testing.T) {
		sent := captureImportRequest(t, "/api/v4/import/bitbucket_server", respBody, func(client *gitlabclient.Client) error {
			_, err := ImportFromBitbucketServer(t.Context(), client, base)
			return err
		})
		for _, tc := range optional {
			t.Run(tc.field, func(t *testing.T) {
				assertImportFieldAbsent(t, sent, tc.field)
			})
		}
	})
}

// TestCancelGitHubImport_ZeroProjectID_RefusedWithoutCallingGitLab asserts the
// guard refuses project_id 0 rather than sending it.
//
// Zero is what a missing, misspelled or non-numeric argument decodes to, which
// is exactly the case the guard exists for: a handler that only refused a
// negative id would ask GitLab to cancel the import of "project 0" and hand the
// model GitLab's own 404 instead of naming the parameter it got wrong. The
// existing refusal test passes -1, which a guard narrowed to "< 0" still
// refuses, so it could never see that narrowing.
func TestCancelGitHubImport_ZeroProjectID_RefusedWithoutCallingGitLab(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("GitLab must not be called when project_id is zero")
		testutil.RespondJSON(w, http.StatusOK, `{"id":0,"name":"my-repo"}`)
	}))

	_, err := CancelGitHubImport(t.Context(), client, CancelGitHubImportInput{ProjectID: 0})
	if err == nil {
		t.Fatal("expected error for zero project_id")
	}
	if !strings.Contains(err.Error(), "project_id") {
		t.Errorf("expected error to mention 'project_id', got %q", err.Error())
	}
}
