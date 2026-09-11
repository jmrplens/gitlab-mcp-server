// user_gpg_keys_test.go contains unit tests for GitLab user GPG key operations.
// Tests use httptest to mock the GitLab User GPG Keys API.
package usergpgkeys

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
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

// assertMarkdown compares a rendered result with the whole document it is
// meant to be. A substring assertion is what let a card open a table and then
// write list rows into it in two packages of this tree: every row the test
// named was present in the string and none of them rendered as a row, so the
// rule here is the whole document or nothing.
func assertMarkdown(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("markdown mismatch\n--- got ---\n%s\n--- want ---\n%s\n--- got (quoted) ---\n%q", got, want, got)
	}
}

// The armored keys the preview tests read. One carries a body short enough to
// show whole; the other's body is longer than the preview, so what is shown is
// its tail. Both carry the armor lines, the header line and the checksum that
// every armored key shares and that no preview should be made of.
const (
	shortArmoredKey = "-----BEGIN PGP PUBLIC KEY BLOCK-----\n" +
		"Version: GnuPG v1\n" +
		"\n" +
		"mQENBFoneKEY\n" +
		"=Zm9v\n" +
		"-----END PGP PUBLIC KEY BLOCK-----\n"
	longArmoredKey = "-----BEGIN PGP PUBLIC KEY BLOCK-----\n" +
		"Version: GnuPG v1\n" +
		"\n" +
		"HEADPARTHEADPARTHEADPART\n" +
		"TAILTAILTAILTAILTAILTAIL\n" +
		"=Zm9v\n" +
		"-----END PGP PUBLIC KEY BLOCK-----\n"
)

// TestFormatListMarkdownString_Empty verifies that a list with nothing in it
// is the one sentence and nothing else.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	assertMarkdown(t, FormatListMarkdownString(ListOutput{}), "No GPG keys found.\n")
}

// TestFormatMarkdownString verifies the whole card: the ID, the key preview in
// a code span, and the creation date in the display form.
func TestFormatMarkdownString(t *testing.T) {
	assertMarkdown(t, FormatMarkdownString(Output{ID: 1, Key: "pgp-key", CreatedAt: "2026-01-15"}),
		"## GPG Key\n\n"+
			"- **ID**: 1\n"+
			"- **Key**: `pgp-key`\n"+
			"- **Created**: 15 Jan 2026\n")
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

// TestFormatDeleteMarkdownString verifies the whole deletion confirmation for
// both deletion states.
func TestFormatDeleteMarkdownString(t *testing.T) {
	tests := []struct {
		name  string
		input DeleteOutput
		want  string
	}{
		{
			name:  "deleted true",
			input: DeleteOutput{KeyID: 42, Deleted: true},
			want:  "## GPG Key Deleted\n\n- **Key ID**: 42\n- **Deleted**: ✅\n",
		},
		{
			name:  "deleted false",
			input: DeleteOutput{KeyID: 7, Deleted: false},
			want:  "## GPG Key Deleted\n\n- **Key ID**: 7\n- **Deleted**: ❌\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertMarkdown(t, FormatDeleteMarkdownString(tt.input), tt.want)
		})
	}
}

// TestFormatListMarkdownString_WithKeys verifies the whole list render. The
// preview column is what the change here is about: every armored key opens
// with the same header line and the same first base64 characters, so a preview
// cut from the front printed one identical string for every key on the
// account. It is the tail of the body now, with the armor, the header line and
// the checksum dropped.
func TestFormatListMarkdownString_WithKeys(t *testing.T) {
	out := ListOutput{Keys: []Output{
		{ID: 1, Key: shortArmoredKey, CreatedAt: "2026-01-15T10:00:00Z"},
		{ID: 2, Key: longArmoredKey, CreatedAt: ""},
	}}

	assertMarkdown(t, FormatListMarkdownString(out),
		"## GPG Keys (2)\n\n"+
			"| ID | Key (truncated) | Created At |\n"+
			"| --- | --- | --- |\n"+
			"| 1 | `mQENBFoneKEY` | 15 Jan 2026 10:00 UTC |\n"+
			"| 2 | `...TAILTAILTAILTAILTAILTAIL` |  |\n"+
			"\n---\n💡 **Next steps:**\n"+
			"- Use action 'user.get_gpg_key' to view full key details\n")
}

// TestFormatMarkdownString_LongKey verifies that a body longer than the card's
// preview is shown as its tail, and that the armor never reaches the card.
func TestFormatMarkdownString_LongKey(t *testing.T) {
	assertMarkdown(t, FormatMarkdownString(Output{ID: 5, Key: longArmoredKey}),
		"## GPG Key\n\n"+
			"- **ID**: 5\n"+
			"- **Key**: `HEADPARTHEADPARTHEADPARTTAILTAILTAILTAILTAILTAIL`\n")
}

// TestFormatMarkdownString_NoCreatedAt verifies that an absent instant writes
// no row rather than a label with nothing after it.
func TestFormatMarkdownString_NoCreatedAt(t *testing.T) {
	assertMarkdown(t, FormatMarkdownString(Output{ID: 3, Key: "short"}),
		"## GPG Key\n\n"+
			"- **ID**: 3\n"+
			"- **Key**: `short`\n")
}

// TestKeyPreview_TailIsWhatDistinguishesTwoKeys is the property the preview
// exists for: two keys from the same generator share their armor, their
// headers and their leading base64, and only their tails differ, so two
// previews must differ too.
func TestKeyPreview_TailIsWhatDistinguishesTwoKeys(t *testing.T) {
	const head = "-----BEGIN PGP PUBLIC KEY BLOCK-----\nVersion: GnuPG v1\n\n" + "mQENBFoAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\n"
	one := keyPreview(head+"ONEONEONEONEONEONEONEONE\n=Zm9v\n-----END PGP PUBLIC KEY BLOCK-----\n", listPreviewRunes)
	two := keyPreview(head+"TWOTWOTWOTWOTWOTWOTWOTWO\n=Zm9v\n-----END PGP PUBLIC KEY BLOCK-----\n", listPreviewRunes)
	if one == two {
		t.Errorf("two keys share the preview %q", one)
	}
	if one != "...ONEONEONEONEONEONEONEONE" {
		t.Errorf("preview = %q, want the tail of the body", one)
	}
}

// TestArmoredBody_ValueWithNoArmorIsItsOwnBody verifies the fallback: a value
// carrying no armor at all, which is what a truncated or hand-entered key
// looks like, is previewed as itself rather than as nothing.
func TestArmoredBody_ValueWithNoArmorIsItsOwnBody(t *testing.T) {
	if got := armoredBody("  ssh-style-value  "); got != "ssh-style-value" {
		t.Errorf("armoredBody = %q, want %q", got, "ssh-style-value")
	}
	if got := armoredBody("-----BEGIN PGP PUBLIC KEY BLOCK-----\n-----END PGP PUBLIC KEY BLOCK-----"); got == "" {
		t.Error("a value that is only armor must still preview as something")
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
