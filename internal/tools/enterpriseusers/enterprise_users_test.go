// enterprise_users_test.go contains unit tests for GitLab enterprise user
// operations. Tests use httptest to mock the GitLab Enterprise Users API.
package enterpriseusers

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// --- List ---.

func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/42/enterprise_users" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":1,"username":"alice","name":"Alice","email":"alice@example.com","state":"active","web_url":"https://gitlab.example.com/alice","two_factor_enabled":true,"created_at":"2026-01-01T00:00:00Z"},
				{"id":2,"username":"bob","name":"Bob","email":"bob@example.com","state":"blocked","web_url":"https://gitlab.example.com/bob","two_factor_enabled":false}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		GroupID: toolutil.StringOrInt("42"),
	})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(out.Users))
	}
	if out.Users[0].Username != "alice" {
		t.Errorf("expected username alice, got %s", out.Users[0].Username)
	}
	if !out.Users[0].TwoFactorEnabled {
		t.Error("expected alice to have 2FA enabled")
	}
	if out.Users[1].State != "blocked" {
		t.Errorf("expected bob state blocked, got %s", out.Users[1].State)
	}
}

func TestList_WithFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/42/enterprise_users" {
			testutil.AssertQueryParam(t, r, "username", "alice")
			testutil.AssertQueryParam(t, r, "search", "alice")
			testutil.AssertQueryParam(t, r, "two_factor", "enabled")
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"username":"alice","name":"Alice"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	active := true
	out, err := List(context.Background(), client, ListInput{
		GroupID:   toolutil.StringOrInt("42"),
		Username:  "alice",
		Search:    "alice",
		Active:    &active,
		TwoFactor: "enabled",
	})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(out.Users))
	}
}

func TestList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := List(ctx, client, ListInput{GroupID: toolutil.StringOrInt("42")})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/42/enterprise_users" {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{GroupID: toolutil.StringOrInt("42")})
	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}
}

func TestList_InvalidCreatedAfter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := List(context.Background(), client, ListInput{
		GroupID:      toolutil.StringOrInt("42"),
		CreatedAfter: "not-a-date",
	})
	if err == nil {
		t.Fatal("expected error for invalid created_after, got nil")
	}
}

func TestList_InvalidCreatedBefore(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := List(context.Background(), client, ListInput{
		GroupID:       toolutil.StringOrInt("42"),
		CreatedBefore: "not-a-date",
	})
	if err == nil {
		t.Fatal("expected error for invalid created_before, got nil")
	}
}

// TestList_BlockedFilter verifies the Blocked filter is passed to the API.
func TestList_BlockedFilter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/42/enterprise_users" {
			testutil.AssertQueryParam(t, r, "blocked", "true")
			testutil.RespondJSON(w, http.StatusOK, `[{"id":2,"username":"bob","state":"blocked"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	blocked := true
	out, err := List(context.Background(), client, ListInput{
		GroupID: toolutil.StringOrInt("42"),
		Blocked: &blocked,
	})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(out.Users))
	}
	if out.Users[0].State != "blocked" {
		t.Errorf("expected state blocked, got %s", out.Users[0].State)
	}
}

// TestList_ValidDateFilters verifies valid CreatedAfter and CreatedBefore are accepted.
func TestList_ValidDateFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/42/enterprise_users" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"username":"alice"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		GroupID:       toolutil.StringOrInt("42"),
		CreatedAfter:  "2026-01-01T00:00:00Z",
		CreatedBefore: "2026-12-31T23:59:59Z",
	})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(out.Users))
	}
}

// TestList_EmptyResults verifies an empty users list returns zero-length slice.
func TestList_EmptyResults(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/42/enterprise_users" {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		GroupID: toolutil.StringOrInt("42"),
	})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Users) != 0 {
		t.Fatalf("expected 0 users, got %d", len(out.Users))
	}
}

// TestToOutput_NilUser verifies that toOutput with nil returns a zero-value Output.
func TestToOutput_NilUser(t *testing.T) {
	out := toOutput(nil)
	if out.ID != 0 {
		t.Errorf("expected ID 0 for nil user, got %d", out.ID)
	}
	if out.Username != "" {
		t.Errorf("expected empty username for nil user, got %q", out.Username)
	}
}

// --- Get ---.

func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/42/enterprise_users/10" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":10,"username":"alice","name":"Alice Wonderland",
				"email":"alice@example.com","state":"active",
				"web_url":"https://gitlab.example.com/alice",
				"is_admin":false,"two_factor_enabled":true,
				"created_at":"2026-01-01T00:00:00Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{
		GroupID: toolutil.StringOrInt("42"),
		UserID:  10,
	})
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if out.ID != 10 {
		t.Errorf("expected ID 10, got %d", out.ID)
	}
	if out.Username != "alice" {
		t.Errorf("expected username alice, got %s", out.Username)
	}
	if !out.TwoFactorEnabled {
		t.Error("expected 2FA enabled")
	}
	if out.CreatedAt == "" {
		t.Error("expected created_at to be set")
	}
}

func TestGet_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Get(context.Background(), client, GetInput{UserID: 10})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

func TestGet_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Get(context.Background(), client, GetInput{GroupID: toolutil.StringOrInt("42")})
	if err == nil {
		t.Fatal("expected error for zero user_id, got nil")
	}
}

func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Get(ctx, client, GetInput{GroupID: toolutil.StringOrInt("42"), UserID: 10})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/42/enterprise_users/10" {
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Get(context.Background(), client, GetInput{
		GroupID: toolutil.StringOrInt("42"),
		UserID:  10,
	})
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

// --- Disable2FA ---.

func TestDisable2FA_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Path == "/api/v4/groups/42/enterprise_users/10/disable_two_factor" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Disable2FA(context.Background(), client, Disable2FAInput{
		GroupID: toolutil.StringOrInt("42"),
		UserID:  10,
	})
	if err != nil {
		t.Fatalf("Disable2FA() error: %v", err)
	}
}

func TestDisable2FA_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Disable2FA(context.Background(), client, Disable2FAInput{UserID: 10})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

func TestDisable2FA_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Disable2FA(context.Background(), client, Disable2FAInput{GroupID: toolutil.StringOrInt("42")})
	if err == nil {
		t.Fatal("expected error for zero user_id, got nil")
	}
}

func TestDisable2FA_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	err := Disable2FA(ctx, client, Disable2FAInput{
		GroupID: toolutil.StringOrInt("42"),
		UserID:  10,
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestDisable2FA_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/42/enterprise_users/10/disable_two_factor" {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			return
		}
		http.NotFound(w, r)
	}))

	err := Disable2FA(context.Background(), client, Disable2FAInput{
		GroupID: toolutil.StringOrInt("42"),
		UserID:  10,
	})
	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}
}

// --- Delete ---.

func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/groups/42/enterprise_users/10" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		GroupID: toolutil.StringOrInt("42"),
		UserID:  10,
	})
	if err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
}

func TestDelete_HardDelete(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/groups/42/enterprise_users/10" {
			testutil.AssertQueryParam(t, r, "hard_delete", "true")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	hardDel := true
	err := Delete(context.Background(), client, DeleteInput{
		GroupID:    toolutil.StringOrInt("42"),
		UserID:     10,
		HardDelete: &hardDel,
	})
	if err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
}

func TestDelete_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Delete(context.Background(), client, DeleteInput{UserID: 10})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

func TestDelete_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Delete(context.Background(), client, DeleteInput{GroupID: toolutil.StringOrInt("42")})
	if err == nil {
		t.Fatal("expected error for zero user_id, got nil")
	}
}

func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	err := Delete(ctx, client, DeleteInput{
		GroupID: toolutil.StringOrInt("42"),
		UserID:  10,
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/42/enterprise_users/10" {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		GroupID: toolutil.StringOrInt("42"),
		UserID:  10,
	})
	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}
}
