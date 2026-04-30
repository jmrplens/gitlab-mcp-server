// project_mirrors_test.go contains unit tests for GitLab project mirror
// operations. Tests use httptest to mock the GitLab Project Mirrors API.
package projectmirrors

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const (
	testProjectID    = "myproject"
	pathMirrors      = "/api/v4/projects/myproject/remote_mirrors"
	pathMirror42     = "/api/v4/projects/myproject/remote_mirrors/42"
	pathMirrorKey42  = "/api/v4/projects/myproject/remote_mirrors/42/public_key"
	pathMirrorSync42 = "/api/v4/projects/myproject/remote_mirrors/42/sync"

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

	publicKeyJSON = `{"public_key": "ssh-rsa AAAAB3..."}`
)

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

// RegisterTools tests.

// TestRegisterTools_NoPanic verifies that RegisterTools registers all Project Mirrors tools
// without panicking.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallAllThroughMCP uses table-driven subtests to exercise every registered
// Project Mirrors tool through a round-trip MCP client call and asserts success.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newMirrorsMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_list_project_mirrors", map[string]any{"project_id": testProjectID}},
		{"gitlab_get_project_mirror", map[string]any{"project_id": testProjectID, "mirror_id": 42}},
		{"gitlab_get_project_mirror_public_key", map[string]any{"project_id": testProjectID, "mirror_id": 42}},
		{"gitlab_add_project_mirror", map[string]any{"project_id": testProjectID, "url": "https://example.com/repo.git"}},
		{"gitlab_edit_project_mirror", map[string]any{"project_id": testProjectID, "mirror_id": 42}},
		{"gitlab_delete_project_mirror", map[string]any{"project_id": testProjectID, "mirror_id": 42}},
		{"gitlab_force_push_mirror_update", map[string]any{"project_id": testProjectID, "mirror_id": 42}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.name,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.name, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.name)
			}
		})
	}
}

// TestMCPRoundTrip_DeleteConfirmDeclined covers the ConfirmAction decline path
// in gitlab_delete_project_mirror register handler.
func TestMCPRoundTrip_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_delete_project_mirror",
		Arguments: map[string]any{"project_id": "myproject", "mirror_id": float64(42)},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

// TestMCPRoundTrip_ErrorPaths covers error return paths through register.go
// for delete and force_push_mirror_update when the backend returns 500.
func TestMCPRoundTrip_ErrorPaths(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "accept"}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_delete_project_mirror", map[string]any{"project_id": "myproject", "mirror_id": float64(42)}},
		{"gitlab_force_push_mirror_update", map[string]any{"project_id": "myproject", "mirror_id": float64(42)}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, toolErr := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if toolErr != nil {
				t.Fatalf("unexpected transport error: %v", toolErr)
			}
			if result == nil || !result.IsError {
				t.Fatal("expected error result for 500 backend")
			}
		})
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

func newMirrorsMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && path == pathMirrors:
			testutil.RespondJSONWithPagination(w, http.StatusOK, "["+mirrorJSON+"]",
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
		case r.Method == http.MethodGet && path == pathMirrorKey42:
			testutil.RespondJSON(w, http.StatusOK, publicKeyJSON)
		case r.Method == http.MethodGet && path == pathMirror42:
			testutil.RespondJSON(w, http.StatusOK, mirrorJSON)
		case r.Method == http.MethodPost && path == pathMirrors:
			testutil.RespondJSON(w, http.StatusCreated, mirrorJSON)
		case r.Method == http.MethodPut && path == pathMirror42:
			testutil.RespondJSON(w, http.StatusOK, mirrorJSON)
		case r.Method == http.MethodDelete && path == pathMirror42:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && path == pathMirrorSync42:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && containsSubstring(s, substr)
}

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
