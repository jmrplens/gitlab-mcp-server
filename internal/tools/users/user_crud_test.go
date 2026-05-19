// user_crud_test.go contains unit tests for GitLab user create, read, update,
// and delete operations. Tests use httptest to mock the GitLab Users API.
package users

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
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

const crudUserJSON = `{
	"id":42,"username":"newuser","email":"new@example.com",
	"name":"New User","state":"active","web_url":"https://gitlab.example.com/newuser",
	"is_admin":false,"bio":"Tester","location":"Berlin","job_title":"Dev","organization":"ACME"
}`

// TestCreateUser_AllOptionalFields verifies Create with every optional field set,
// covering all if-branches in the Create function.
func TestCreateUser_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/users" {
			testutil.RespondJSON(w, http.StatusCreated, crudUserJSON)
			return
		}
		http.NotFound(w, r)
	}))

	resetPwd := true
	forceRandom := false
	skipConf := true
	admin := false
	external := true
	projLimit := int64(50)

	out, err := Create(context.Background(), client, CreateInput{
		Email:               "new@example.com",
		Name:                "New User",
		Username:            "newuser",
		Password:            "secureP@ss1",
		ResetPassword:       &resetPwd,
		ForceRandomPassword: &forceRandom,
		SkipConfirmation:    &skipConf,
		Admin:               &admin,
		External:            &external,
		Bio:                 "Tester",
		Location:            "Berlin",
		JobTitle:            "Dev",
		Organization:        "ACME",
		ProjectsLimit:       &projLimit,
		Note:                "Internal user",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.ID != 42 {
		t.Errorf("ID = %d, want 42", out.ID)
	}
	if out.Username != "newuser" {
		t.Errorf("Username = %q, want %q", out.Username, "newuser")
	}
}

// TestCreateUser_MissingName verifies validation error when name is empty.
func TestCreateUser_MissingName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		Email: "a@b.com", Username: "user1",
	})
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
}

// TestCreateUser_MissingUsername verifies validation error when username is empty.
func TestCreateUser_MissingUsername(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		Email: "a@b.com", Name: "User",
	})
	if err == nil {
		t.Fatal("expected error for missing username, got nil")
	}
}

// TestCreateUser_APIError verifies error handling on API failure.
func TestCreateUser_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusConflict, `{"message":"409 Conflict"}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		Email: "dup@example.com", Name: "Dup", Username: "dup",
	})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// TestCreateUser_CancelledContext verifies context cancellation.
func TestCreateUser_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Create(ctx, client, CreateInput{
		Email: "a@b.com", Name: "User", Username: "user",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestModifyUser_AllOptionalFields verifies Modify with every optional field set.
func TestModifyUser_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/users/42" {
			testutil.RespondJSON(w, http.StatusOK, crudUserJSON)
			return
		}
		http.NotFound(w, r)
	}))

	admin := true
	external := false
	skipReconf := true
	projLimit := int64(100)
	privateProf := true
	canCreateGrp := true
	locked := false

	out, err := Modify(context.Background(), client, ModifyInput{
		UserID:             42,
		Email:              "updated@example.com",
		Name:               "Updated",
		Username:           "updated-user",
		Password:           "newP@ss!",
		Admin:              &admin,
		External:           &external,
		SkipReconfirmation: &skipReconf,
		Bio:                "Updated bio",
		Location:           "London",
		JobTitle:           "Lead",
		Organization:       "NewOrg",
		ProjectsLimit:      &projLimit,
		Note:               "Updated note",
		PrivateProfile:     &privateProf,
		CanCreateGroup:     &canCreateGrp,
		Locked:             &locked,
	})
	if err != nil {
		t.Fatalf("Modify() unexpected error: %v", err)
	}
	if out.ID != 42 {
		t.Errorf("ID = %d, want 42", out.ID)
	}
}

// TestModifyUser_APIError verifies error handling on API failure.
func TestModifyUser_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := Modify(context.Background(), client, ModifyInput{UserID: 999})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// TestModifyUser_CancelledContext verifies context cancellation.
func TestModifyUser_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Modify(ctx, client, ModifyInput{UserID: 42})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestDeleteUser_InvalidUserID verifies validation for zero user_id.
func TestDeleteUser_InvalidUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Delete(context.Background(), client, DeleteInput{})
	if err == nil {
		t.Fatal("expected error for invalid user_id, got nil")
	}
}

// TestDeleteUser_APIError verifies error handling on API failure.
func TestDeleteUser_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := Delete(context.Background(), client, DeleteInput{UserID: 999})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// TestDeleteUser_CancelledContext verifies context cancellation.
func TestDeleteUser_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Delete(ctx, client, DeleteInput{UserID: 42})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}
