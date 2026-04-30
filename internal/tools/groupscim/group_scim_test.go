// group_scim_test.go contains unit tests for GitLab group SCIM token
// operations. Tests use httptest to mock the GitLab Group SCIM API.
package groupscim

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

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

// TestList_MissingGroupID verifies that List returns a validation error
// when the required group_id input is empty.
func TestList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestList_CancelledContext verifies that List returns an error when
// invoked with an already-cancelled context.
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

// TestList_APIError verifies that List returns an error when the GitLab
// group SCIM endpoint responds with 403 Forbidden.
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

// TestGet_Success verifies that Get retrieves a single SCIM identity by
// group and UID and returns the expected external_uid and user_id.
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

// TestGet_MissingGroupID verifies that Get returns a validation error
// when the required group_id input is empty.
func TestGet_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Get(context.Background(), client, GetInput{UID: "uid-123"})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestGet_MissingUID verifies that Get returns a validation error when
// the required uid input is empty.
func TestGet_MissingUID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Get(context.Background(), client, GetInput{GroupID: toolutil.StringOrInt("mygroup")})
	if err == nil {
		t.Fatal("expected error for empty uid, got nil")
	}
}

// TestGet_CancelledContext verifies that Get returns an error when
// invoked with an already-cancelled context.
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

// TestGet_APIError verifies that Get returns an error when the GitLab
// SCIM endpoint responds with 400 Bad Request.
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
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

// TestUpdate_Success verifies that Update issues PATCH
// /api/v4/groups/:id/scim/:uid and returns no error on 204 No Content.
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

// TestUpdate_MissingGroupID verifies that Update returns a validation
// error when the required group_id input is empty.
func TestUpdate_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Update(context.Background(), client, UpdateInput{UID: "uid-123", ExternUID: "new"})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestUpdate_MissingUID verifies that Update returns a validation error
// when the required uid input is empty.
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

// TestUpdate_MissingExternUID verifies that Update returns a validation
// error when the required extern_uid input is empty.
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

// TestUpdate_CancelledContext verifies that Update returns an error when
// invoked with an already-cancelled context.
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

// TestUpdate_APIError verifies that Update returns an error when the
// GitLab SCIM endpoint responds with 403 Forbidden.
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
	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}
}

// TestDelete_Success verifies that Delete issues DELETE
// /api/v4/groups/:id/scim/:uid and returns no error on 204 No Content.
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

// TestDelete_MissingGroupID verifies that Delete returns a validation
// error when the required group_id input is empty.
func TestDelete_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Delete(context.Background(), client, DeleteInput{UID: "uid-123"})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestDelete_MissingUID verifies that Delete returns a validation error
// when the required uid input is empty.
func TestDelete_MissingUID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Delete(context.Background(), client, DeleteInput{GroupID: toolutil.StringOrInt("mygroup")})
	if err == nil {
		t.Fatal("expected error for empty uid, got nil")
	}
}

// TestDelete_CancelledContext verifies that Delete returns an error when
// invoked with an already-cancelled context.
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

// TestDelete_APIError verifies that Delete returns an error when the
// GitLab SCIM endpoint responds with 400 Bad Request.
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
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}
