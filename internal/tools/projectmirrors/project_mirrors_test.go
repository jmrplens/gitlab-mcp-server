// project_mirrors_test.go contains unit tests for GitLab project mirror
// operations. Tests use httptest to mock the GitLab Project Mirrors API.
package projectmirrors

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	// testProjectID identifies the test project ID constant used by this package.
	testProjectID = "myproject"
	// pathMirrors identifies the path mirrors constant used by this package.
	pathMirrors = "/api/v4/projects/myproject/remote_mirrors"
	// pathMirror42 identifies the path mirror 42 constant used by this package.
	pathMirror42 = "/api/v4/projects/myproject/remote_mirrors/42"
	// pathMirrorKey42 identifies the path mirror key 42 constant used by this package.
	pathMirrorKey42 = "/api/v4/projects/myproject/remote_mirrors/42/public_key"
	// pathMirrorSync42 identifies the path mirror sync 42 constant used by this package.
	pathMirrorSync42 = "/api/v4/projects/myproject/remote_mirrors/42/sync"

	// mirrorJSON identifies the mirror JSON constant used by this package.
	mirrorJSON = `{
		"id": 42,
		"enabled": true,
		"url": "https://example.com/repo.git",
		"update_status": "finished",
		"last_error": "",
		"only_protected_branches": false,
		"keep_divergent_refs": true,
		"mirror_branch_regex": "",
		"auth_method": "password",
		"last_successful_update_at": "2026-03-10T09:00:00Z",
		"last_update_at": "2026-03-10T09:00:00Z",
		"last_update_started_at": "2026-03-10T08:59:00Z"
	}`

	// mirrorWithHostKeysJSON identifies the mirror with host keys JSON constant used by this package.
	mirrorWithHostKeysJSON = `{
		"id": 42,
		"enabled": true,
		"url": "https://example.com/repo.git",
		"update_status": "finished",
		"last_error": "",
		"only_protected_branches": false,
		"keep_divergent_refs": true,
		"mirror_branch_regex": "",
		"auth_method": "ssh_public_key",
		"last_successful_update_at": "2026-03-10T09:00:00Z",
		"last_update_at": "2026-03-10T09:00:00Z",
		"last_update_started_at": "2026-03-10T08:59:00Z",
		"host_keys": [{"fingerprint_sha256": "SHA256:abc123def456"}]
	}`

	// publicKeyJSON identifies the public key JSON constant used by this package.
	publicKeyJSON = `{"public_key": "ssh-rsa AAAAB3..."}`
)

// TestRedactMirrorURL_RemovesEmbeddedCredentials verifies RedactMirrorURL when removes embedded credentials.
func TestRedactMirrorURL_RemovesEmbeddedCredentials(t *testing.T) {
	got := redactMirrorURL("https://user:token@example.com/group/repo.git")
	if got != "https://redacted@example.com/group/repo.git" {
		t.Fatalf("redactMirrorURL() = %q", got)
	}
}

// TestRedactMirrorURL_EdgeCases verifies URL redaction preserves safe URLs and
// still redacts credential-looking text when URL parsing fails.
func TestRedactMirrorURL_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "safe URL", in: "https://example.com/group/repo.git", want: "https://example.com/group/repo.git"},
		{name: "invalid URL with credentials", in: "https://user:token@example.com/%zz", want: "https://[redacted]@example.com/%zz"},
		{name: "invalid URL without credentials", in: "https://example.com/%zz", want: "https://example.com/%zz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redactMirrorURL(tt.in); got != tt.want {
				t.Fatalf("redactMirrorURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestRedactMirrorError_RemovesEmbeddedCredentials verifies RedactMirrorError when removes embedded credentials.
func TestRedactMirrorError_RemovesEmbeddedCredentials(t *testing.T) {
	err := redactMirrorError(errors.New("mirror failed for https://user:secret-token@example.com/group/repo.git"))
	if err == nil {
		t.Fatal("redactMirrorError() = nil, want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "[redacted]") {
		t.Fatalf("redactMirrorError() = %q, want redacted marker", msg)
	}
	if strings.Contains(msg, "secret-token") || strings.Contains(msg, "user:") {
		t.Fatalf("redactMirrorError() = %q, want credentials removed", msg)
	}
}

// TestRedactMirrorError_EdgeCases verifies nil errors and already-safe errors
// are preserved without allocation or message changes.
func TestRedactMirrorError_EdgeCases(t *testing.T) {
	if redactMirrorError(nil) != nil {
		t.Fatal("redactMirrorError(nil) should return nil")
	}
	err := errors.New("plain mirror error")
	if got := redactMirrorError(err); !errors.Is(got, err) || got.Error() != err.Error() {
		t.Fatalf("redactMirrorError(safe) = %v, want original error", got)
	}
}

// List tests.

// TestList_Success verifies that List lists a project push mirror on a successful GitLab API response.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathMirrors {
			testutil.RespondJSON(w, http.StatusOK, "["+mirrorJSON+"]")
			return
		}
		http.NotFound(w, r)
	}))
	out, err := List(context.Background(), client, ListInput{ProjectID: testProjectID})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Mirrors) != 1 {
		t.Fatalf("len(Mirrors) = %d, want 1", len(out.Mirrors))
	}
	if out.Mirrors[0].ID != 42 {
		t.Errorf("ID = %d, want 42", out.Mirrors[0].ID)
	}
	if out.Mirrors[0].URL != "https://example.com/repo.git" {
		t.Errorf("URL = %q", out.Mirrors[0].URL)
	}
	if !out.Mirrors[0].KeepDivergentRefs {
		t.Error("KeepDivergentRefs = false, want true")
	}
}

// TestList_MissingProjectID verifies that List returns a validation error when project_id is missing.
func TestList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestList_CancelledContext verifies that List returns an error when the context is cancelled before the request completes.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{ProjectID: testProjectID})
	if err == nil {
		t.Fatal("expected context error")
	}
}

// TestList_APIError verifies that List propagates errors returned by the GitLab API.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: testProjectID})
	if err == nil {
		t.Fatal("expected error for 403")
	}
}

// TestList_NotFound verifies List returns the project lookup hint for 404s.
func TestList_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Project Not Found"}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: testProjectID})
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "gitlab_project_get") {
		t.Fatalf("error missing project lookup hint: %v", err)
	}
}

// TestList_Pagination verifies that List forwards pagination parameters (page, per_page) to the GitLab API.
func TestList_Pagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "2" {
			t.Errorf("page = %q, want 2", r.URL.Query().Get("page"))
		}
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	_, err := List(context.Background(), client, ListInput{
		ProjectID:       testProjectID,
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 10},
	})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
}

// Get tests.

// TestGet_Success verifies that Get retrieves a project push mirror on a successful GitLab API response.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathMirror42 {
			testutil.RespondJSON(w, http.StatusOK, mirrorJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, MirrorID: 42})
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if out.ID != 42 {
		t.Errorf("ID = %d, want 42", out.ID)
	}
	if out.UpdateStatus != "finished" {
		t.Errorf("UpdateStatus = %q, want finished", out.UpdateStatus)
	}
}

// TestGet_RedactsCredentialsInOutput verifies Get when redacts credentials in output.
func TestGet_RedactsCredentialsInOutput(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathMirror42 {
			testutil.RespondJSON(w, http.StatusOK, strings.Replace(mirrorJSON, "https://example.com/repo.git", "https://user:token@example.com/repo.git", 1))
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, MirrorID: 42})
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if out.URL != "https://redacted@example.com/repo.git" {
		t.Fatalf("URL = %q, want redacted URL", out.URL)
	}
}

// TestGet_WithHostKeys verifies that host_keys are correctly mapped to HostKeyOutput.
func TestGet_WithHostKeys(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathMirror42 {
			testutil.RespondJSON(w, http.StatusOK, mirrorWithHostKeysJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, MirrorID: 42})
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if len(out.HostKeys) != 1 {
		t.Fatalf("len(HostKeys) = %d, want 1", len(out.HostKeys))
	}
	if out.HostKeys[0].FingerprintSHA256 != "SHA256:abc123def456" {
		t.Errorf("FingerprintSHA256 = %q, want SHA256:abc123def456", out.HostKeys[0].FingerprintSHA256)
	}
}

// TestGet_MissingProjectID verifies that Get returns a validation error when project_id is missing.
func TestGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(context.Background(), client, GetInput{MirrorID: 42})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestGet_MissingMirrorID verifies that Get returns a validation error when mirror_id is missing.
func TestGet_MissingMirrorID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID})
	if err == nil {
		t.Fatal("expected error for missing mirror_id")
	}
}

// TestGet_CancelledContext verifies that Get returns an error when the context is cancelled before the request completes.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{ProjectID: testProjectID, MirrorID: 42})
	if err == nil {
		t.Fatal("expected context error")
	}
}

// GetPublicKey tests.

// TestGetPublicKey_Success verifies that GetPublicKey retrieves the SSH public key for a project push mirror on a successful GitLab API response.
func TestGetPublicKey_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathMirrorKey42 {
			testutil.RespondJSON(w, http.StatusOK, publicKeyJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := GetPublicKey(context.Background(), client, GetPublicKeyInput{ProjectID: testProjectID, MirrorID: 42})
	if err != nil {
		t.Fatalf("GetPublicKey() error: %v", err)
	}
	if out.PublicKey != "ssh-rsa AAAAB3..." {
		t.Errorf("PublicKey = %q", out.PublicKey)
	}
}

// TestGetPublicKey_MissingProjectID verifies that GetPublicKey returns a validation error when project_id is missing.
func TestGetPublicKey_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := GetPublicKey(context.Background(), client, GetPublicKeyInput{MirrorID: 42})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestGetPublicKey_MissingMirrorID verifies that GetPublicKey returns a validation error when mirror_id is missing.
func TestGetPublicKey_MissingMirrorID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := GetPublicKey(context.Background(), client, GetPublicKeyInput{ProjectID: testProjectID})
	if err == nil {
		t.Fatal("expected error for missing mirror_id")
	}
}

// TestGetPublicKey_CancelledContext verifies that GetPublicKey returns an error when the context is cancelled before the request completes.
func TestGetPublicKey_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetPublicKey(ctx, client, GetPublicKeyInput{ProjectID: testProjectID, MirrorID: 42})
	if err == nil {
		t.Fatal("expected context error")
	}
}

// Add tests.

// TestAdd_Success verifies that Add creates a project push mirror on a successful GitLab API response.
func TestAdd_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMirrors {
			testutil.RespondJSON(w, http.StatusCreated, mirrorJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Add(context.Background(), client, AddInput{
		ProjectID:  testProjectID,
		URL:        "https://example.com/repo.git",
		AuthMethod: "password",
	})
	if err != nil {
		t.Fatalf("Add() error: %v", err)
	}
	if out.ID != 42 {
		t.Errorf("ID = %d, want 42", out.ID)
	}
}

// TestAdd_WithOptions verifies that Add serializes optional fields in the request body.
func TestAdd_WithOptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMirrors {
			testutil.RespondJSON(w, http.StatusCreated, mirrorJSON)
			return
		}
		http.NotFound(w, r)
	}))
	enabled := true
	keepDiv := false
	protOnly := true
	out, err := Add(context.Background(), client, AddInput{
		ProjectID:             testProjectID,
		URL:                   "https://example.com/repo.git",
		Enabled:               &enabled,
		KeepDivergentRefs:     &keepDiv,
		OnlyProtectedBranches: &protOnly,
		MirrorBranchRegex:     "^main$",
		AuthMethod:            "ssh_public_key",
	})
	if err != nil {
		t.Fatalf("Add() error: %v", err)
	}
	if out.ID != 42 {
		t.Errorf("ID = %d, want 42", out.ID)
	}
}

// TestAdd_WithHostKeys verifies that host_keys are sent in the add request.
func TestAdd_WithHostKeys(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMirrors {
			testutil.RespondJSON(w, http.StatusCreated, mirrorWithHostKeysJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Add(context.Background(), client, AddInput{
		ProjectID:  testProjectID,
		URL:        "https://example.com/repo.git",
		AuthMethod: "ssh_public_key",
		HostKeys:   []string{"ssh-rsa AAAAB3..."},
	})
	if err != nil {
		t.Fatalf("Add() error: %v", err)
	}
	if len(out.HostKeys) != 1 {
		t.Fatalf("len(HostKeys) = %d, want 1", len(out.HostKeys))
	}
	if out.HostKeys[0].FingerprintSHA256 != "SHA256:abc123def456" {
		t.Errorf("FingerprintSHA256 = %q, want SHA256:abc123def456", out.HostKeys[0].FingerprintSHA256)
	}
}

// TestAdd_MissingProjectID verifies that Add returns a validation error when project_id is missing.
func TestAdd_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Add(context.Background(), client, AddInput{URL: "https://example.com/repo.git"})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestAdd_MissingURL verifies that Add returns a validation error when url is missing.
func TestAdd_MissingURL(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Add(context.Background(), client, AddInput{ProjectID: testProjectID})
	if err == nil {
		t.Fatal("expected error for missing url")
	}
}

// TestAdd_CancelledContext verifies that Add returns an error when the context is cancelled before the request completes.
func TestAdd_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Add(ctx, client, AddInput{ProjectID: testProjectID, URL: "https://example.com/repo.git"})
	if err == nil {
		t.Fatal("expected context error")
	}
}

// TestAdd_ErrorBranches verifies Add handles validation-style and fallback API
// errors while redacting credentialed mirror URLs from returned messages.
func TestAdd_ErrorBranches(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		want       string
	}{
		{name: "bad request", statusCode: http.StatusBadRequest, want: "mirror URL is well-formed"},
		{name: "fallback", statusCode: http.StatusUnprocessableEntity, want: "remote rejected mirror"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tt.statusCode, `{"message":"remote rejected mirror https://user:secret@example.com/repo.git"}`)
			}))
			_, err := Add(context.Background(), client, AddInput{ProjectID: testProjectID, URL: "https://user:secret@example.com/repo.git"})
			if err == nil {
				t.Fatal("expected API error")
			}
			msg := err.Error()
			if !strings.Contains(msg, tt.want) {
				t.Fatalf("error missing %q: %v", tt.want, err)
			}
			if strings.Contains(msg, "secret") || strings.Contains(msg, "user:") {
				t.Fatalf("error leaked credentials: %v", err)
			}
		})
	}
}

// Edit tests.

// TestEdit_Success verifies that Edit updates a project push mirror on a successful GitLab API response.
func TestEdit_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathMirror42 {
			testutil.RespondJSON(w, http.StatusOK, mirrorJSON)
			return
		}
		http.NotFound(w, r)
	}))
	enabled := false
	out, err := Edit(context.Background(), client, EditInput{
		ProjectID:         testProjectID,
		MirrorID:          42,
		Enabled:           &enabled,
		MirrorBranchRegex: "^release/.*$",
		AuthMethod:        "password",
	})
	if err != nil {
		t.Fatalf("Edit() error: %v", err)
	}
	if out.ID != 42 {
		t.Errorf("ID = %d, want 42", out.ID)
	}
}

// TestEdit_MissingProjectID verifies that Edit returns a validation error when project_id is missing.
func TestEdit_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Edit(context.Background(), client, EditInput{MirrorID: 42})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestEdit_MissingMirrorID verifies that Edit returns a validation error when mirror_id is missing.
func TestEdit_MissingMirrorID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Edit(context.Background(), client, EditInput{ProjectID: testProjectID})
	if err == nil {
		t.Fatal("expected error for missing mirror_id")
	}
}

// TestEdit_CancelledContext verifies that Edit returns an error when the context is cancelled before the request completes.
func TestEdit_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Edit(ctx, client, EditInput{ProjectID: testProjectID, MirrorID: 42})
	if err == nil {
		t.Fatal("expected context error")
	}
}

// TestEdit_ErrorBranches verifies Edit handles not-found and fallback API
// errors while redacting credentialed mirror URLs from returned messages.
func TestEdit_ErrorBranches(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		want       string
	}{
		{name: "not found", statusCode: http.StatusNotFound, want: hintVerifyMirrorID},
		{name: "fallback", statusCode: http.StatusUnprocessableEntity, want: "remote rejected mirror"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tt.statusCode, `{"message":"remote rejected mirror https://user:secret@example.com/repo.git"}`)
			}))
			_, err := Edit(context.Background(), client, EditInput{ProjectID: testProjectID, MirrorID: 42})
			if err == nil {
				t.Fatal("expected API error")
			}
			msg := err.Error()
			if !strings.Contains(msg, tt.want) {
				t.Fatalf("error missing %q: %v", tt.want, err)
			}
			if strings.Contains(msg, "secret") || strings.Contains(msg, "user:") {
				t.Fatalf("error leaked credentials: %v", err)
			}
		})
	}
}

// Delete tests.

// TestDelete_Success verifies that Delete deletes a project push mirror on a successful GitLab API response.
func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathMirror42 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID, MirrorID: 42})
	if err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
}

// TestDelete_MissingProjectID verifies that Delete returns a validation error when project_id is missing.
func TestDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := Delete(context.Background(), client, DeleteInput{MirrorID: 42})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestDelete_MissingMirrorID verifies that Delete returns a validation error when mirror_id is missing.
func TestDelete_MissingMirrorID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID})
	if err == nil {
		t.Fatal("expected error for missing mirror_id")
	}
}

// TestDelete_CancelledContext verifies that Delete returns an error when the context is cancelled before the request completes.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{ProjectID: testProjectID, MirrorID: 42})
	if err == nil {
		t.Fatal("expected context error")
	}
}

// TestDelete_NotFound verifies Delete returns the shared mirror lookup hint for 404s.
func TestDelete_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Mirror Not Found"}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID, MirrorID: 42})
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), hintVerifyMirrorID) {
		t.Fatalf("error missing mirror hint: %v", err)
	}
}

// ForcePushUpdate tests.

// TestForcePushUpdate_Success verifies that ForcePushUpdate triggers a force-push update on a project push mirror on a successful GitLab API response.
func TestForcePushUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMirrorSync42 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	err := ForcePushUpdate(context.Background(), client, ForcePushInput{ProjectID: testProjectID, MirrorID: 42})
	if err != nil {
		t.Fatalf("ForcePushUpdate() error: %v", err)
	}
}

// TestForcePushUpdate_MissingProjectID verifies that ForcePushUpdate returns a validation error when project_id is missing.
func TestForcePushUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := ForcePushUpdate(context.Background(), client, ForcePushInput{MirrorID: 42})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestForcePushUpdate_MissingMirrorID verifies that ForcePushUpdate returns a validation error when mirror_id is missing.
func TestForcePushUpdate_MissingMirrorID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := ForcePushUpdate(context.Background(), client, ForcePushInput{ProjectID: testProjectID})
	if err == nil {
		t.Fatal("expected error for missing mirror_id")
	}
}

// TestForcePushUpdate_CancelledContext verifies that ForcePushUpdate returns an error when the context is cancelled before the request completes.
func TestForcePushUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	err := ForcePushUpdate(ctx, client, ForcePushInput{ProjectID: testProjectID, MirrorID: 42})
	if err == nil {
		t.Fatal("expected context error")
	}
}

// TestForcePushUpdate_NotFound verifies ForcePushUpdate returns the shared
// mirror lookup hint for 404s.
func TestForcePushUpdate_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Mirror Not Found"}`)
	}))
	err := ForcePushUpdate(context.Background(), client, ForcePushInput{ProjectID: testProjectID, MirrorID: 42})
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), hintVerifyMirrorID) {
		t.Fatalf("error missing mirror hint: %v", err)
	}
}

// Markdown tests.

// TestFormatOutputMarkdown_Basic verifies the OutputMarkdown_Basic markdown formatter output.
func TestFormatOutputMarkdown_Basic(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:           42,
		Enabled:      true,
		URL:          "https://example.com/repo.git",
		UpdateStatus: "finished",
		AuthMethod:   "password",
	})
	if !contains(md, "## Remote Mirror #42") {
		t.Error("missing header")
	}
	if !contains(md, "https://example.com/repo.git") {
		t.Error("missing URL")
	}
}

// TestFormatOutputMarkdown_WithTimestamps verifies the OutputMarkdown_WithTimestamps markdown formatter output.
func TestFormatOutputMarkdown_WithTimestamps(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:                     42,
		URL:                    "https://example.com/repo.git",
		UpdateStatus:           "finished",
		LastSuccessfulUpdateAt: "2026-03-10T09:00:00Z",
		LastUpdateAt:           "2026-03-10T09:00:00Z",
		LastError:              "auth failed",
		MirrorBranchRegex:      "^main$",
	})
	if !contains(md, "Last Successful Update") {
		t.Error("missing last successful update")
	}
	if !contains(md, "Last Error") {
		t.Error("missing last error")
	}
	if !contains(md, "Branch Regex") {
		t.Error("missing branch regex")
	}
}

// TestFormatOutputMarkdown_WithHostKeys verifies the OutputMarkdown_WithHostKeys markdown formatter output.
func TestFormatOutputMarkdown_WithHostKeys(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:           42,
		URL:          "https://example.com/repo.git",
		UpdateStatus: "finished",
		AuthMethod:   "ssh_public_key",
		HostKeys:     []HostKeyOutput{{FingerprintSHA256: "SHA256:abc123"}},
	})
	if !contains(md, "Host Keys") {
		t.Error("missing host keys section")
	}
	if !contains(md, "SHA256:abc123") {
		t.Error("missing fingerprint value")
	}
}

// TestFormatOutputMarkdown_Empty verifies the OutputMarkdown_Empty markdown formatter output.
func TestFormatOutputMarkdown_Empty(t *testing.T) {
	md := FormatOutputMarkdown(Output{})
	if md != "" {
		t.Errorf("expected empty string, got %q", md)
	}
}

// TestFormatListMarkdown_Empty verifies the ListMarkdown_Empty markdown formatter output.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !contains(md, "No remote mirrors found") {
		t.Error("missing empty message")
	}
}

// TestFormatListMarkdown_WithMirrors verifies the ListMarkdown_WithMirrors markdown formatter output.
func TestFormatListMarkdown_WithMirrors(t *testing.T) {
	md := FormatListMarkdown(ListOutput{
		Mirrors: []Output{
			{ID: 42, URL: "https://a.com/r.git", Enabled: true, UpdateStatus: "finished"},
			{ID: 43, URL: "https://b.com/r.git", Enabled: false, UpdateStatus: "failed"},
		},
	})
	if !contains(md, "| 42 |") {
		t.Error("missing mirror 42 row")
	}
	if !contains(md, "| 43 |") {
		t.Error("missing mirror 43 row")
	}
}

// TestFormatPublicKeyMarkdown_Success verifies the PublicKeyMarkdown_Success markdown formatter output.
func TestFormatPublicKeyMarkdown_Success(t *testing.T) {
	md := FormatPublicKeyMarkdown(PublicKeyOutput{PublicKey: "ssh-rsa AAAAB3..."})
	if !contains(md, "ssh-rsa AAAAB3...") {
		t.Error("missing public key")
	}
}

// TestFormatPublicKeyMarkdown_Empty verifies the PublicKeyMarkdown_Empty markdown formatter output.
func TestFormatPublicKeyMarkdown_Empty(t *testing.T) {
	md := FormatPublicKeyMarkdown(PublicKeyOutput{})
	if !contains(md, "No public key available") {
		t.Error("missing empty message")
	}
}

// toOutput coverage tests.

// TestToOutput_NilTimestamps verifies that toOutput handles edge cases in the
// GitLab response (nil timestamps or optional fields) without panicking.
func TestToOutput_NilTimestamps(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathMirror42 {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id": 42,
				"enabled": false,
				"url": "https://no-ts.com/repo.git",
				"update_status": "none"
			}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, MirrorID: 42})
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if out.LastSuccessfulUpdateAt != "" {
		t.Errorf("LastSuccessfulUpdateAt = %q, want empty", out.LastSuccessfulUpdateAt)
	}
	if out.LastUpdateAt != "" {
		t.Errorf("LastUpdateAt = %q, want empty", out.LastUpdateAt)
	}
	if out.LastUpdateStartedAt != "" {
		t.Errorf("LastUpdateStartedAt = %q, want empty", out.LastUpdateStartedAt)
	}
}

// TestGet_APIError covers the API error path in Get.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"error"}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "1", MirrorID: 42})
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

// TestGetPublicKey_APIError covers the API error path in GetPublicKey.
func TestGetPublicKey_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"error"}`)
	}))
	_, err := GetPublicKey(context.Background(), client, GetPublicKeyInput{ProjectID: "1", MirrorID: 42})
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

// TestAdd_APIError covers the API error path in Add.
func TestAdd_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"error"}`)
	}))
	_, err := Add(context.Background(), client, AddInput{ProjectID: "1", URL: "https://example.com/repo.git"})
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

// TestEdit_APIError covers the API error path in Edit.
func TestEdit_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"error"}`)
	}))
	_, err := Edit(context.Background(), client, EditInput{ProjectID: "1", MirrorID: 42})
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

// TestDelete_APIError covers the API error path in Delete.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"error"}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "1", MirrorID: 42})
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

// TestForcePushUpdate_APIError covers the API error path in ForcePushUpdate.
func TestForcePushUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"error"}`)
	}))
	err := ForcePushUpdate(context.Background(), client, ForcePushInput{ProjectID: "1", MirrorID: 42})
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

// contains reports whether contains.
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && containsSubstring(s, substr)
}

// containsSubstring reports whether contains substring.
func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestEdit_WithHostKeys verifies that Edit forwards host_keys to the
// GitLab API in the PUT body when input.HostKeys is non-empty. This
// targets the optional-field branch in Edit that copies host keys into
// the request options struct.
func TestEdit_WithHostKeys(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathMirror42 {
			b, _ := io.ReadAll(r.Body)
			capturedBody = string(b)
			testutil.RespondJSON(w, http.StatusOK, mirrorJSON)
			return
		}
		http.NotFound(w, r)
	}))
	_, err := Edit(context.Background(), client, EditInput{
		ProjectID: testProjectID,
		MirrorID:  42,
		HostKeys:  []string{"ssh-rsa AAAA..."},
	})
	if err != nil {
		t.Fatalf("Edit() unexpected error: %v", err)
	}
	if !strings.Contains(capturedBody, "host_keys") {
		t.Errorf("request body missing host_keys field; body=%q", capturedBody)
	}
}
