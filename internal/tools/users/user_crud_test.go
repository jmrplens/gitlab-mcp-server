// user_crud_test.go contains unit tests for GitLab user create, read, update,
// and delete operations. Tests use httptest to mock the GitLab Users API.

package users

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const userJSON = `{
	"id":42,"username":"testuser","email":"test@example.com",
	"name":"Test User","state":"active","web_url":"https://gitlab.example.com/testuser",
	"is_admin":false
}`

// TestCreateUser_Success verifies Create returns the new user when POST /users
// responds 201 Created with a user JSON body.
func TestCreateUser_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/users" {
			testutil.RespondJSON(w, http.StatusCreated, userJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		Email: "test@example.com", Name: "Test User", Username: "testuser", Password: "pa$$w0rd",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.ID != 42 {
		t.Errorf("out.ID = %d, want 42", out.ID)
	}
}

// TestCreateUser_MissingEmail verifies Create returns an input-validation error
// when the email field is empty, without hitting the API.
func TestCreateUser_MissingEmail(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{Name: "Test", Username: "test"})
	if err == nil {
		t.Fatal("expected error for missing email, got nil")
	}
}

// TestModifyUser_Success verifies Modify returns the updated user when
// PUT /users/:id responds 200 OK.
func TestModifyUser_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/users/42" {
			testutil.RespondJSON(w, http.StatusOK, userJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Modify(context.Background(), client, ModifyInput{UserID: 42, Bio: "Updated bio"})
	if err != nil {
		t.Fatalf("Modify() unexpected error: %v", err)
	}
	if out.ID != 42 {
		t.Errorf("out.ID = %d, want 42", out.ID)
	}
}

// TestModifyUser_InvalidUserID verifies Modify returns a validation error when
// user_id=0, without hitting the API.
func TestModifyUser_InvalidUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Modify(context.Background(), client, ModifyInput{UserID: 0})
	if err == nil {
		t.Fatal("expected error for invalid user_id, got nil")
	}
}

// TestDeleteUser_Success verifies Delete reports Deleted=true when
// DELETE /users/:id responds 204 No Content.
func TestDeleteUser_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/users/42" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Delete(context.Background(), client, DeleteInput{UserID: 42})
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
	if !out.Deleted {
		t.Error("out.Deleted = false, want true")
	}
}
