// usergpgkeys_test.go contains unit tests for GitLab user GPG key operations.
// Tests use httptest to mock the GitLab User GPG Keys API.

package usergpgkeys

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const (
	errExpAPIFailure     = "expected error for API failure, got nil"
	errExpValidation     = "expected validation error, got nil"
	errExpCtxCancel      = "expected context cancellation error, got nil"
	pathGPGKeys          = "/api/v4/user/gpg_keys"
	pathGPGKeysUser      = "/api/v4/users/42/gpg_keys"
	pathGPGKey           = "/api/v4/user/gpg_keys/1"
	pathGPGKeyUser       = "/api/v4/users/42/gpg_keys/1"
	gpgKeyJSON           = `{"id":1,"key":"-----BEGIN PGP PUBLIC KEY BLOCK-----","created_at":"2026-01-15T10:00:00Z"}`
	gpgKeyListJSON       = `[{"id":1,"key":"-----BEGIN PGP PUBLIC KEY BLOCK-----","created_at":"2026-01-15T10:00:00Z"},{"id":2,"key":"-----BEGIN PGP PUBLIC KEY BLOCK-----","created_at":"2026-02-20T12:00:00Z"}]`
	gpgKeyNilCreatedJSON = `{"id":3,"key":"-----BEGIN PGP PUBLIC KEY BLOCK-----"}`
)

// TestList_Success verifies that List lists a user GPG key on a successful GitLab API response.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGPGKeys {
			testutil.RespondJSON(w, http.StatusOK, gpgKeyListJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Keys) != 2 {
		t.Fatalf("len(out.Keys) = %d, want 2", len(out.Keys))
	}
	if out.Keys[0].ID != 1 {
		t.Errorf("out.Keys[0].ID = %d, want 1", out.Keys[0].ID)
	}
}

// TestList_APIError verifies that List propagates errors returned by the GitLab API.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnauthorized, `{"message":"401 Unauthorized"}`)
	}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestListForUser_Success verifies that ListForUser lists (admin) for a specific user a user GPG key on a successful GitLab API response.
func TestListForUser_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGPGKeysUser {
			testutil.RespondJSON(w, http.StatusOK, gpgKeyListJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListForUser(context.Background(), client, ListForUserInput{UserID: 42})
	if err != nil {
		t.Fatalf("ListForUser() unexpected error: %v", err)
	}
	if len(out.Keys) != 2 {
		t.Fatalf("len(out.Keys) = %d, want 2", len(out.Keys))
	}
}

// TestListForUser_InvalidUserID verifies that ListForUser returns a validation error when user_id is invalid.
func TestListForUser_InvalidUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := ListForUser(context.Background(), client, ListForUserInput{UserID: 0})
	if err == nil {
		t.Fatal("expected error for invalid user_id, got nil")
	}
}

// TestGet_Success verifies that Get retrieves a user GPG key on a successful GitLab API response.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGPGKey {
			testutil.RespondJSON(w, http.StatusOK, gpgKeyJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{KeyID: 1})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("out.ID = %d, want 1", out.ID)
	}
}

// TestGet_InvalidKeyID verifies that Get returns a validation error when key_id is invalid.
func TestGet_InvalidKeyID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(context.Background(), client, GetInput{KeyID: 0})
	if err == nil {
		t.Fatal("expected error for invalid key_id, got nil")
	}
}

// TestGetForUser_Success verifies that GetForUser retrieves (admin) for a specific user a user GPG key on a successful GitLab API response.
func TestGetForUser_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGPGKeyUser {
			testutil.RespondJSON(w, http.StatusOK, gpgKeyJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetForUser(context.Background(), client, GetForUserInput{UserID: 42, KeyID: 1})
	if err != nil {
		t.Fatalf("GetForUser() unexpected error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("out.ID = %d, want 1", out.ID)
	}
}

// TestAdd_Success verifies that Add creates a user GPG key on a successful GitLab API response.
func TestAdd_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathGPGKeys {
			testutil.RespondJSON(w, http.StatusCreated, gpgKeyJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Add(context.Background(), client, AddInput{Key: "-----BEGIN PGP PUBLIC KEY BLOCK-----"})
	if err != nil {
		t.Fatalf("Add() unexpected error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("out.ID = %d, want 1", out.ID)
	}
}

// TestAdd_EmptyKey verifies that Add returns a validation error when the key field is empty.
func TestAdd_EmptyKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Add(context.Background(), client, AddInput{Key: ""})
	if err == nil {
		t.Fatal("expected error for empty key, got nil")
	}
}

// TestAddForUser_Success verifies that AddForUser creates (admin) for a specific user a user GPG key on a successful GitLab API response.
func TestAddForUser_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathGPGKeysUser {
			testutil.RespondJSON(w, http.StatusCreated, gpgKeyJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := AddForUser(context.Background(), client, AddForUserInput{UserID: 42, Key: "-----BEGIN PGP PUBLIC KEY BLOCK-----"})
	if err != nil {
		t.Fatalf("AddForUser() unexpected error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("out.ID = %d, want 1", out.ID)
	}
}

// TestDelete_Success verifies that Delete deletes a user GPG key on a successful GitLab API response.
func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathGPGKey {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Delete(context.Background(), client, DeleteInput{KeyID: 1})
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
	if !out.Deleted {
		t.Error("out.Deleted = false, want true")
	}
}

// TestDelete_InvalidKeyID verifies that Delete returns a validation error when key_id is invalid.
func TestDelete_InvalidKeyID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Delete(context.Background(), client, DeleteInput{KeyID: 0})
	if err == nil {
		t.Fatal("expected error for invalid key_id, got nil")
	}
}

// TestDeleteForUser_Success verifies that DeleteForUser deletes (admin) for a specific user a user GPG key on a successful GitLab API response.
func TestDeleteForUser_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathGPGKeyUser {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := DeleteForUser(context.Background(), client, DeleteForUserInput{UserID: 42, KeyID: 1})
	if err != nil {
		t.Fatalf("DeleteForUser() unexpected error: %v", err)
	}
	if !out.Deleted {
		t.Error("out.Deleted = false, want true")
	}
}

// TestFormatListMarkdownString_Empty verifies the ListMarkdownString_Empty markdown formatter output.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	md := FormatListMarkdownString(ListOutput{})
	if md == "" {
		t.Fatal("expected non-empty markdown for empty list")
	}
}

// TestFormatMarkdownString verifies the MarkdownString markdown formatter output.
func TestFormatMarkdownString(t *testing.T) {
	md := FormatMarkdownString(Output{ID: 1, Key: "pgp-key", CreatedAt: "2026-01-15"})
	if md == "" {
		t.Fatal("expected non-empty markdown")
	}
}

// --- Context cancellation tests ---
// These tests verify every handler respects context cancellation and returns
// an error instead of proceeding with the API call.

// TestList_ContextCancelled verifies that List returns an error when the context is cancelled before the request completes.
func TestList_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestListForUser_ContextCancelled verifies that ListForUser returns an error when the context is cancelled before the request completes.
func TestListForUser_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListForUser(ctx, client, ListForUserInput{UserID: 42})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestGet_ContextCancelled verifies that Get returns an error when the context is cancelled before the request completes.
func TestGet_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, gpgKeyJSON)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{KeyID: 1})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestGetForUser_ContextCancelled verifies that GetForUser returns an error when the context is cancelled before the request completes.
func TestGetForUser_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, gpgKeyJSON)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetForUser(ctx, client, GetForUserInput{UserID: 42, KeyID: 1})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestAdd_ContextCancelled verifies that Add returns an error when the context is cancelled before the request completes.
func TestAdd_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, gpgKeyJSON)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Add(ctx, client, AddInput{Key: "-----BEGIN PGP PUBLIC KEY BLOCK-----"})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestAddForUser_ContextCancelled verifies that AddForUser returns an error when the context is cancelled before the request completes.
func TestAddForUser_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, gpgKeyJSON)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := AddForUser(ctx, client, AddForUserInput{UserID: 42, Key: "-----BEGIN PGP PUBLIC KEY BLOCK-----"})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestDelete_ContextCancelled verifies that Delete returns an error when the context is cancelled before the request completes.
func TestDelete_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Delete(ctx, client, DeleteInput{KeyID: 1})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestDeleteForUser_ContextCancelled verifies that DeleteForUser returns an error when the context is cancelled before the request completes.
func TestDeleteForUser_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := DeleteForUser(ctx, client, DeleteForUserInput{UserID: 42, KeyID: 1})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// --- Missing input validation tests ---
// These tests verify validation branches not covered by existing tests.

// TestGetForUser_InvalidUserID verifies that GetForUser returns a validation error when user_id is invalid.
func TestGetForUser_InvalidUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := GetForUser(context.Background(), client, GetForUserInput{UserID: 0, KeyID: 1})
	if err == nil {
		t.Fatal(errExpValidation)
	}
}

// TestGetForUser_InvalidKeyID verifies that GetForUser returns a validation error when key_id is invalid.
func TestGetForUser_InvalidKeyID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := GetForUser(context.Background(), client, GetForUserInput{UserID: 42, KeyID: 0})
	if err == nil {
		t.Fatal(errExpValidation)
	}
}

// TestAddForUser_InvalidUserID verifies that AddForUser returns a validation error when user_id is invalid.
func TestAddForUser_InvalidUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := AddForUser(context.Background(), client, AddForUserInput{UserID: 0, Key: "pgp-key"})
	if err == nil {
		t.Fatal(errExpValidation)
	}
}

// TestAddForUser_EmptyKey verifies that AddForUser returns a validation error when the key field is empty.
func TestAddForUser_EmptyKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := AddForUser(context.Background(), client, AddForUserInput{UserID: 42, Key: ""})
	if err == nil {
		t.Fatal(errExpValidation)
	}
}

// TestDeleteForUser_InvalidUserID verifies that DeleteForUser returns a validation error when user_id is invalid.
func TestDeleteForUser_InvalidUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := DeleteForUser(context.Background(), client, DeleteForUserInput{UserID: 0, KeyID: 1})
	if err == nil {
		t.Fatal(errExpValidation)
	}
}

// TestDeleteForUser_InvalidKeyID verifies that DeleteForUser returns a validation error when key_id is invalid.
func TestDeleteForUser_InvalidKeyID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := DeleteForUser(context.Background(), client, DeleteForUserInput{UserID: 42, KeyID: 0})
	if err == nil {
		t.Fatal(errExpValidation)
	}
}

// --- Missing API error tests ---
// These tests verify error propagation from the GitLab API for handlers
// that did not yet have API error coverage.

// TestListForUser_APIError verifies that ListForUser propagates errors returned by the GitLab API.
func TestListForUser_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := ListForUser(context.Background(), client, ListForUserInput{UserID: 42})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestGet_APIError verifies that Get propagates errors returned by the GitLab API.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := Get(context.Background(), client, GetInput{KeyID: 999})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestGetForUser_APIError verifies that GetForUser propagates errors returned by the GitLab API.
func TestGetForUser_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := GetForUser(context.Background(), client, GetForUserInput{UserID: 42, KeyID: 1})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestAdd_APIError verifies that Add propagates errors returned by the GitLab API.
func TestAdd_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"422 Unprocessable"}`)
	}))
	_, err := Add(context.Background(), client, AddInput{Key: "bad-key"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestAddForUser_APIError verifies that AddForUser propagates errors returned by the GitLab API.
func TestAddForUser_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"422 Unprocessable"}`)
	}))
	_, err := AddForUser(context.Background(), client, AddForUserInput{UserID: 42, Key: "bad-key"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestDelete_APIError verifies that Delete propagates errors returned by the GitLab API.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := Delete(context.Background(), client, DeleteInput{KeyID: 999})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestDeleteForUser_APIError verifies that DeleteForUser propagates errors returned by the GitLab API.
func TestDeleteForUser_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := DeleteForUser(context.Background(), client, DeleteForUserInput{UserID: 42, KeyID: 1})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// --- Empty result tests ---

// TestList_EmptyResult verifies List returns an empty slice (not nil) when the
// API returns an empty JSON array.
func TestList_EmptyResult(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGPGKeys {
			testutil.RespondJSON(w, http.StatusOK, "[]")
			return
		}
		http.NotFound(w, r)
	}))
	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Keys) != 0 {
		t.Errorf("len(out.Keys) = %d, want 0", len(out.Keys))
	}
}

// --- Markdown formatter tests ---
// These tests cover formatting branches for markdown renderers including
// FormatDeleteMarkdownString, non-empty lists with long keys, and long single keys.

// TestFormatDeleteMarkdownString verifies the DeleteMarkdownString markdown formatter output.
func TestFormatDeleteMarkdownString(t *testing.T) {
	tests := []struct {
		name    string
		input   DeleteOutput
		wantSub string
	}{
		{
			name:    "deleted true",
			input:   DeleteOutput{KeyID: 42, Deleted: true},
			wantSub: "42",
		},
		{
			name:    "deleted false",
			input:   DeleteOutput{KeyID: 7, Deleted: false},
			wantSub: "7",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := FormatDeleteMarkdownString(tt.input)
			if md == "" {
				t.Fatal("expected non-empty markdown")
			}
			if !strings.Contains(md, tt.wantSub) {
				t.Errorf("markdown missing %q:\n%s", tt.wantSub, md)
			}
		})
	}
}

// TestFormatListMarkdownString_WithKeys verifies the list markdown renderer
// correctly renders a non-empty key list including the truncation of long keys.
func TestFormatListMarkdownString_WithKeys(t *testing.T) {
	longKey := strings.Repeat("A", 60)
	out := ListOutput{Keys: []Output{
		{ID: 1, Key: "short-key", CreatedAt: "2026-01-15T10:00:00Z"},
		{ID: 2, Key: longKey, CreatedAt: ""},
	}}
	md := FormatListMarkdownString(out)
	if !strings.Contains(md, "(2)") {
		t.Error("markdown should contain key count (2)")
	}
	if !strings.Contains(md, "short-key") {
		t.Error("markdown should contain the short key")
	}
	if !strings.Contains(md, "...") {
		t.Error("long key should be truncated with '...'")
	}
	if strings.Contains(md, "No GPG keys found") {
		t.Error("non-empty list should not contain 'No GPG keys found'")
	}
}

// TestFormatMarkdownString_LongKey verifies FormatMarkdownString truncates
// keys longer than 80 characters.
func TestFormatMarkdownString_LongKey(t *testing.T) {
	longKey := strings.Repeat("B", 100)
	md := FormatMarkdownString(Output{ID: 5, Key: longKey})
	if !strings.Contains(md, "...") {
		t.Error("long key should be truncated with '...'")
	}
	if strings.Contains(md, longKey) {
		t.Error("full long key should not appear in markdown")
	}
}

// TestFormatMarkdownString_NoCreatedAt verifies the formatter omits the
// Created At line when it is empty.
func TestFormatMarkdownString_NoCreatedAt(t *testing.T) {
	md := FormatMarkdownString(Output{ID: 3, Key: "short"})
	if strings.Contains(md, "Created") {
		t.Error("empty CreatedAt should not produce a Created line")
	}
}

// TestGet_NilCreatedAt verifies toOutput handles a GPG key with nil CreatedAt
// by leaving CreatedAt empty in the output.
func TestGet_NilCreatedAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/user/gpg_keys/3" {
			testutil.RespondJSON(w, http.StatusOK, gpgKeyNilCreatedJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Get(context.Background(), client, GetInput{KeyID: 3})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.ID != 3 {
		t.Errorf("out.ID = %d, want 3", out.ID)
	}
	if out.CreatedAt != "" {
		t.Errorf("out.CreatedAt = %q, want empty for nil created_at", out.CreatedAt)
	}
}
