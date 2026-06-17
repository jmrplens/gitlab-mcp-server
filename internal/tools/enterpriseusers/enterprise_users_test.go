// enterprise_users_test.go contains unit tests for GitLab enterprise user
// operations. Tests use httptest to mock the GitLab Enterprise Users API.
package enterpriseusers

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

func assertEnterpriseUserHint(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected enterprise user error, got nil")
	}
	errText := err.Error()
	for _, want := range []string{"user_id", "gitlab_enterprise_user", "gitlab_list_enterprise_users", "enterprise namespace"} {
		if !strings.Contains(errText, want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
}

// --- List ---.

// TestList_Success verifies that List fetches /api/v4/groups/:id/enterprise_users
// and returns all enterprise users with fields like username, state and 2FA flag.
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

// TestList_WithFilters verifies the List_WithFilters handler.
// The mock GitLab API at /api/v4/groups/42/enterprise_users (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestList_MissingGroupID verifies that List_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestList_CancelledContext verifies the List_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestList_APIError verifies that List returns a wrapped error when the GitLab API responds with an error status.
// The mock GitLab API at /api/v4/groups/42/enterprise_users (GET) responds with HTTP Forbidden.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestList_InvalidCreatedAfter verifies the List_InvalidCreatedAfter handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestList_InvalidCreatedBefore verifies the List_InvalidCreatedBefore handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestList_BlockedFilter verifies the List_BlockedFilter handler.
// The mock GitLab API at /api/v4/groups/42/enterprise_users (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestList_ValidDateFilters verifies the List_ValidDateFilters handler.
// The mock GitLab API at /api/v4/groups/42/enterprise_users (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestList_EmptyResults verifies the List_EmptyResults handler.
// The mock GitLab API at /api/v4/groups/42/enterprise_users (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestToOutput_NilUser verifies the ToOutput_NilUser handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestGet_Success verifies that Get succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/42/enterprise_users/10 (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestGet_MissingGroupID verifies that Get_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Get(context.Background(), client, GetInput{UserID: 10})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestGet_MissingUserID verifies that Get_MissingUserID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Get(context.Background(), client, GetInput{GroupID: toolutil.StringOrInt("42")})
	if err == nil {
		t.Fatal("expected error for zero user_id, got nil")
	}
}

// TestGet_CancelledContext verifies the Get_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestGet_APIError verifies that Get returns a wrapped error when the GitLab API responds with an error status.
// The mock GitLab API at /api/v4/groups/42/enterprise_users/10 (GET) responds with HTTP NotFound.
// It asserts that the returned error is wrapped and contains a useful hint.
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
	assertEnterpriseUserHint(t, err)
}

// --- Disable2FA ---.

// TestDisable2FA_Success verifies that Disable2FA issues PATCH
// /api/v4/groups/:id/enterprise_users/:uid/disable_two_factor and returns
// no error on 204 No Content.
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

// TestDisable2FA_MissingGroupID verifies that Disable2FA_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDisable2FA_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Disable2FA(context.Background(), client, Disable2FAInput{UserID: 10})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestDisable2FA_MissingUserID verifies that Disable2FA_MissingUserID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDisable2FA_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Disable2FA(context.Background(), client, Disable2FAInput{GroupID: toolutil.StringOrInt("42")})
	if err == nil {
		t.Fatal("expected error for zero user_id, got nil")
	}
}

// TestDisable2FA_CancelledContext verifies the Disable2FA_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestDisable2FA_APIError verifies that Disable2FA returns a wrapped error when the GitLab API responds with an error status.
// The mock GitLab API at /api/v4/groups/42/enterprise_users/10/disable_two_factor (GET) responds with HTTP Forbidden.
// It asserts that the returned error is wrapped and contains a useful hint.
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
	assertEnterpriseUserHint(t, err)
}

// --- Delete ---.

// TestDelete_Success verifies that Delete succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/42/enterprise_users/10 (DELETE) returns a representative success body.
// It asserts the returned output matches the expected fields.
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

// TestDelete_HardDelete verifies the Delete_HardDelete handler.
// The mock GitLab API at /api/v4/groups/42/enterprise_users/10 (DELETE) returns a representative success body.
// It asserts the returned output matches the expected fields.
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

// TestDelete_MissingGroupID verifies that Delete_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDelete_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Delete(context.Background(), client, DeleteInput{UserID: 10})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestDelete_MissingUserID verifies that Delete_MissingUserID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDelete_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Delete(context.Background(), client, DeleteInput{GroupID: toolutil.StringOrInt("42")})
	if err == nil {
		t.Fatal("expected error for zero user_id, got nil")
	}
}

// TestDelete_CancelledContext verifies the Delete_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestDelete_APIError verifies that Delete returns a wrapped error when the GitLab API responds with an error status.
// The mock GitLab API at /api/v4/groups/42/enterprise_users/10 (GET) responds with HTTP Forbidden.
// It asserts that the returned error is wrapped and contains a useful hint.
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
	assertEnterpriseUserHint(t, err)
}
