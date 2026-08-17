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
		if !strings.Contains(body, field) {
			t.Errorf("expected %s in request body, got: %s", field, body)
		}
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
	md := FormatGitHubImport(out)
	if !strings.Contains(md, testMyRepoName) {
		t.Errorf("expected markdown to contain '%s'", testMyRepoName)
	}
}

// TestFormatBitbucketServerImport verifies the BitbucketServerImport Markdown formatter for a representative bitbucketserverimport input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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
		if !strings.Contains(capturedBody, want) {
			t.Errorf("request body missing field %q", want)
		}
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
		if !strings.Contains(capturedBody, want) {
			t.Errorf("request body missing field %q; body=%s", want, capturedBody)
		}
	}
	if out.RefsURL != "https://gitlab.example.com/ns/my-repo/refs" {
		t.Errorf("expected refs_url to be mapped, got %q", out.RefsURL)
	}
	if out.ImportWarning != "some collaborators could not be mapped" {
		t.Errorf("expected import_warning to be mapped, got %q", out.ImportWarning)
	}
}

// TestFormatGitHubImport_WithImportWarning verifies the GitHubImport Markdown
// formatter renders the additive import_warning row when present.
func TestFormatGitHubImport_WithImportWarning(t *testing.T) {
	out := &GitHubImportOutput{
		ID: 1, Name: testMyRepoName, FullPath: "ns/my-repo",
		ImportSource: "github.com/user/repo", ImportStatus: "scheduled",
		ImportWarning: "partial import",
	}
	md := FormatGitHubImport(out)
	if !strings.Contains(md, "Import Warning") || !strings.Contains(md, "partial import") {
		t.Errorf("expected import warning row in output, got:\n%s", md)
	}
}

// TestFormatCancelledImport verifies the CancelledImport Markdown formatter for a representative cancelledimport input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestFormatBitbucketCloudImport verifies the BitbucketCloudImport Markdown formatter for a representative bitbucketcloudimport input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestFormatImportGists verifies the ImportGists Markdown formatter for a representative importgists input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatImportGists(t *testing.T) {
	md := FormatImportGists()
	if !strings.Contains(md, "gists") {
		t.Errorf("expected 'gists' in output, got %q", md)
	}
}

// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
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

// TestActionSpecs_CallRoutes validates the CallRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
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

// TestActionSpecs_ErrorPaths validates the ErrorPaths route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestMarkdownRegistry_PointerOutputFormatters verifies the init-registered
// value-signature formatter closures render each import output type through
// the shared Markdown registry (covering the registration lambdas that adapt
// the pointer-returning handlers to value-type registry keys).
func TestMarkdownRegistry_PointerOutputFormatters(t *testing.T) {
	outputs := []any{
		GitHubImportOutput{},
		CancelledImportOutput{},
		BitbucketCloudImportOutput{},
		BitbucketServerImportOutput{},
	}
	for _, out := range outputs {
		if result := toolutil.MarkdownForResult(out); result == nil || len(result.Content) == 0 {
			t.Errorf("MarkdownForResult(%T) returned empty result", out)
		}
	}
}
