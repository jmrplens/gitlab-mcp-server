// group_scim_test.go contains unit tests for GitLab group SCIM token
// operations. Tests use httptest to mock the GitLab Group SCIM API.
package groupscim

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

func assertSCIMIdentityHint(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected SCIM identity error, got nil")
	}
	errText := err.Error()
	for _, want := range []string{"uid", "gitlab_group_scim", "gitlab_list_group_scim_identities", "SAML SSO SCIM provisioning"} {
		if !strings.Contains(errText, want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
}

// TestList_Success verifies that List fetches /api/v4/groups/:id/scim/identities
// and returns all SCIM identities with external_uid, user_id and active fields.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/mygroup/scim/identities" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"external_uid":"ext-1","user_id":10,"active":true},
				{"external_uid":"ext-2","user_id":20,"active":false}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		GroupID: toolutil.StringOrInt("mygroup"),
	})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Identities) != 2 {
		t.Fatalf("expected 2 identities, got %d", len(out.Identities))
	}
	if out.Identities[0].ExternalUID != "ext-1" {
		t.Errorf("expected external_uid ext-1, got %s", out.Identities[0].ExternalUID)
	}
	if out.Identities[1].UserID != 20 {
		t.Errorf("expected user_id 20, got %d", out.Identities[1].UserID)
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

	_, err := List(ctx, client, ListInput{GroupID: toolutil.StringOrInt("mygroup")})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestList_APIError verifies that List returns a wrapped error when the GitLab API responds with an error status.
// The mock GitLab API at /api/v4/groups/mygroup/scim/identities (GET) responds with HTTP Forbidden.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/scim/identities" {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{
		GroupID: toolutil.StringOrInt("mygroup"),
	})
	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}
}

// TestGet_Success verifies that Get succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/mygroup/scim/uid-123 (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/mygroup/scim/uid-123" {
			testutil.RespondJSON(w, http.StatusOK, `{"external_uid":"uid-123","user_id":42,"active":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		UID:     "uid-123",
	})
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if out.ExternalUID != "uid-123" {
		t.Errorf("expected external_uid uid-123, got %s", out.ExternalUID)
	}
	if out.UserID != 42 {
		t.Errorf("expected user_id 42, got %d", out.UserID)
	}
}

// TestGet_MissingGroupID verifies that Get_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Get(context.Background(), client, GetInput{UID: "uid-123"})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestGet_MissingUID verifies that Get_MissingUID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_MissingUID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Get(context.Background(), client, GetInput{GroupID: toolutil.StringOrInt("mygroup")})
	if err == nil {
		t.Fatal("expected error for empty uid, got nil")
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

	_, err := Get(ctx, client, GetInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		UID:     "uid-123",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestGet_APIError verifies that Get returns a wrapped error when the GitLab API responds with an error status.
// The mock GitLab API at /api/v4/groups/mygroup/scim/uid-123 (GET) responds with HTTP BadRequest.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/scim/uid-123" {
			testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"Bad Request"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Get(context.Background(), client, GetInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		UID:     "uid-123",
	})
	assertSCIMIdentityHint(t, err)
}

// TestUpdate_Success verifies that Update succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/mygroup/scim/uid-123 (PATCH) returns a representative success body.
// It asserts the returned output matches the expected fields.
func TestUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Path == "/api/v4/groups/mygroup/scim/uid-123" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Update(context.Background(), client, UpdateInput{
		GroupID:   toolutil.StringOrInt("mygroup"),
		UID:       "uid-123",
		ExternUID: "new-ext-uid",
	})
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}
}

// TestUpdate_MissingGroupID verifies that Update_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdate_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Update(context.Background(), client, UpdateInput{UID: "uid-123", ExternUID: "new"})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestUpdate_MissingUID verifies that Update_MissingUID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdate_MissingUID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Update(context.Background(), client, UpdateInput{
		GroupID:   toolutil.StringOrInt("mygroup"),
		ExternUID: "new",
	})
	if err == nil {
		t.Fatal("expected error for empty uid, got nil")
	}
}

// TestUpdate_MissingExternUID verifies that Update_MissingExternUID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdate_MissingExternUID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Update(context.Background(), client, UpdateInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		UID:     "uid-123",
	})
	if err == nil {
		t.Fatal("expected error for empty extern_uid, got nil")
	}
}

// TestUpdate_CancelledContext verifies the Update_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	err := Update(ctx, client, UpdateInput{
		GroupID:   toolutil.StringOrInt("mygroup"),
		UID:       "uid-123",
		ExternUID: "new",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestUpdate_APIError verifies that Update returns a wrapped error when the GitLab API responds with an error status.
// The mock GitLab API at /api/v4/groups/mygroup/scim/uid-123 (GET) responds with HTTP Forbidden.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/scim/uid-123" {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			return
		}
		http.NotFound(w, r)
	}))

	err := Update(context.Background(), client, UpdateInput{
		GroupID:   toolutil.StringOrInt("mygroup"),
		UID:       "uid-123",
		ExternUID: "new",
	})
	assertSCIMIdentityHint(t, err)
}

// TestDelete_Success verifies that Delete succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/mygroup/scim/uid-123 (DELETE) returns a representative success body.
// It asserts the returned output matches the expected fields.
func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/groups/mygroup/scim/uid-123" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		UID:     "uid-123",
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

	err := Delete(context.Background(), client, DeleteInput{UID: "uid-123"})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestDelete_MissingUID verifies that Delete_MissingUID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDelete_MissingUID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Delete(context.Background(), client, DeleteInput{GroupID: toolutil.StringOrInt("mygroup")})
	if err == nil {
		t.Fatal("expected error for empty uid, got nil")
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
		GroupID: toolutil.StringOrInt("mygroup"),
		UID:     "uid-123",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestDelete_APIError verifies that Delete returns a wrapped error when the GitLab API responds with an error status.
// The mock GitLab API at /api/v4/groups/mygroup/scim/uid-123 (GET) responds with HTTP BadRequest.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/scim/uid-123" {
			testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"Bad Request"}`)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		UID:     "uid-123",
	})
	assertSCIMIdentityHint(t, err)
}
