// user_crud_test.go contains unit tests for GitLab user create, read, update,
// and delete operations. Tests use httptest to mock the GitLab Users API.
package users

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
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

// TestModifyUser_AllOptionalFields verifies that a request carrying every
// optional field is accepted and its user decoded. It asserts nothing about
// what reached GitLab, which is what TestModify_EachOptionReachesTheRequest is
// for; the comment used to claim it verified the fields were sent while its
// own mock discarded the body.
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

// decodeJSONBody reads an HTTP request's JSON body into a generic map so tests
// can assert which option fields the SDK serialized.
//
// It is called from httptest handlers, which run on the server goroutine, so it
// reports failures with t.Errorf and returns an empty map. testing requires
// FailNow to run on the test goroutine; from a handler it would abort the
// request mid-response and the client would report a misleading transport error
// instead of the decoding failure.
func decodeJSONBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	m := map[string]any{}
	raw, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		t.Errorf("read body: %v", readErr)
		return m
	}
	if len(raw) > 0 {
		if jsonErr := json.Unmarshal(raw, &m); jsonErr != nil {
			t.Errorf("unmarshal body %q: %v", raw, jsonErr)
		}
	}
	return m
}

// capturedRequest is what a handler asked GitLab for: the query it built and
// the JSON body it wrote. A test asserting that an optional input reaches
// GitLab reads this and not the response, which is the fixture's own whatever
// was sent.
type capturedRequest struct {
	query url.Values
	body  map[string]any
}

// recordRequest answers one path with the given status and body, capturing what
// the handler sent. Every other path is a 404, so a handler that makes a second
// call of its own cannot overwrite the capture.
func recordRequest(t *testing.T, path string, got *capturedRequest, status int, response string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		got.query = r.URL.Query()
		got.body = decodeJSONBody(t, r)
		testutil.RespondJSON(w, status, response)
	}
}

// assertSent compares the whole request against what one case drove instead of
// checking that a key is present. Presence cannot tell two fields apart: an
// option builder that assigned the wrong neighbor still sends both keys. A
// case that drives one field and compares the whole map can.
func assertSent(t *testing.T, what string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v, want %#v", what, got, want)
	}
}

// TestCreate_EachOptionReachesTheRequest drives one optional field at a time
// and compares the whole body GitLab received with the three required keys plus
// that one. Driving them all at once and checking each key is present cannot
// see an option builder that assigned the wrong neighbor, and cannot tell two
// flags apart at all, since every one of them is then true.
func TestCreate_EachOptionReachesTheRequest(t *testing.T) {
	yes := true
	five := int64(5)

	tests := []struct {
		name  string
		input CreateInput
		want  map[string]any
	}{
		{"password", CreateInput{Password: "pa$$"}, map[string]any{"password": "pa$$"}},
		{"reset_password", CreateInput{ResetPassword: &yes}, map[string]any{"reset_password": true}},
		{"force_random_password", CreateInput{ForceRandomPassword: &yes}, map[string]any{"force_random_password": true}},
		{"skip_confirmation", CreateInput{SkipConfirmation: &yes}, map[string]any{"skip_confirmation": true}},
		{"admin", CreateInput{Admin: &yes}, map[string]any{"admin": true}},
		{"auditor", CreateInput{Auditor: &yes}, map[string]any{"auditor": true}},
		{"external", CreateInput{External: &yes}, map[string]any{"external": true}},
		{"can_create_group", CreateInput{CanCreateGroup: &yes}, map[string]any{"can_create_group": true}},
		{"private_profile", CreateInput{PrivateProfile: &yes}, map[string]any{"private_profile": true}},
		{"view_diffs_file_by_file", CreateInput{ViewDiffsFileByFile: &yes}, map[string]any{"view_diffs_file_by_file": true}},
		{"bio", CreateInput{Bio: "bio"}, map[string]any{"bio": "bio"}},
		{"location", CreateInput{Location: "location"}, map[string]any{"location": "location"}},
		{"job_title", CreateInput{JobTitle: "job title"}, map[string]any{"job_title": "job title"}},
		{"organization", CreateInput{Organization: "organization"}, map[string]any{"organization": "organization"}},
		{"pronouns", CreateInput{Pronouns: "pronouns"}, map[string]any{"pronouns": "pronouns"}},
		{"commit_email", CreateInput{CommitEmail: "commit@x.test"}, map[string]any{"commit_email": "commit@x.test"}},
		{"public_email", CreateInput{PublicEmail: "public@x.test"}, map[string]any{"public_email": "public@x.test"}},
		{"website_url", CreateInput{WebsiteURL: "https://site.test"}, map[string]any{"website_url": "https://site.test"}},
		{"linkedin", CreateInput{Linkedin: "linkedin"}, map[string]any{"linkedin": "linkedin"}},
		{"twitter", CreateInput{Twitter: "twitter"}, map[string]any{"twitter": "twitter"}},
		{"skype", CreateInput{Skype: "skype"}, map[string]any{"skype": "skype"}},
		{"discord", CreateInput{Discord: "discord"}, map[string]any{"discord": "discord"}},
		{"github", CreateInput{Github: "github"}, map[string]any{"github": "github"}},
		{"provider", CreateInput{Provider: "ldapmain"}, map[string]any{"provider": "ldapmain"}},
		{"extern_uid", CreateInput{ExternUID: "extern-uid"}, map[string]any{"extern_uid": "extern-uid"}},
		{"note", CreateInput{Note: "admin note"}, map[string]any{"note": "admin note"}},
		{"group_id_for_saml", CreateInput{GroupIDForSAML: &five}, map[string]any{"group_id_for_saml": float64(5)}},
		{"projects_limit", CreateInput{ProjectsLimit: &five}, map[string]any{"projects_limit": float64(5)}},
		{"theme_id", CreateInput{ThemeID: &five}, map[string]any{"theme_id": float64(5)}},
		{"color_scheme_id", CreateInput{ColorSchemeID: &five}, map[string]any{"color_scheme_id": float64(5)}},
		{"shared_runners_minutes_limit", CreateInput{SharedRunnersMinutesLimit: &five}, map[string]any{"shared_runners_minutes_limit": float64(5)}},
		{"extra_shared_runners_minutes_limit", CreateInput{ExtraSharedRunnersMinutesLimit: &five}, map[string]any{"extra_shared_runners_minutes_limit": float64(5)}},
		{"nothing", CreateInput{}, map[string]any{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got capturedRequest
			client := testutil.NewTestClient(t, recordRequest(t, pathListUsers, &got, http.StatusCreated, fullUserJSON))

			input := tt.input
			input.Email, input.Name, input.Username = "n@x.test", "N", "n"
			if _, err := Create(context.Background(), client, input); err != nil {
				t.Fatalf("Create() unexpected error: %v", err)
			}

			want := map[string]any{"email": "n@x.test", "name": "N", "username": "n"}
			maps.Copy(want, tt.want)
			assertSent(t, "create body", got.body, want)
		})
	}
}

// TestModify_EachOptionReachesTheRequest drives one optional field at a time
// and compares the whole body, for the reason its create sibling gives: an
// update sends only what the caller asked to change, so the body is exactly
// that one key and a builder writing into the wrong field is visible.
//
// A case per field is also what would have caught the phantom this sweep
// removed. A locked flag this input published reached no field of client-go's
// ModifyUserOptions, and GitLab's PUT /users/:id declares no such parameter,
// so its case here would have driven the flag and found an empty body.
func TestModify_EachOptionReachesTheRequest(t *testing.T) {
	yes := true
	four := int64(4)

	tests := []struct {
		name  string
		input ModifyInput
		want  map[string]any
	}{
		{"email", ModifyInput{Email: "moved@x.test"}, map[string]any{"email": "moved@x.test"}},
		{"name", ModifyInput{Name: "New Name"}, map[string]any{"name": "New Name"}},
		{"username", ModifyInput{Username: "new-username"}, map[string]any{"username": "new-username"}},
		{"password", ModifyInput{Password: "pa$$"}, map[string]any{"password": "pa$$"}},
		{"admin", ModifyInput{Admin: &yes}, map[string]any{"admin": true}},
		{"auditor", ModifyInput{Auditor: &yes}, map[string]any{"auditor": true}},
		{"external", ModifyInput{External: &yes}, map[string]any{"external": true}},
		{"skip_reconfirmation", ModifyInput{SkipReconfirmation: &yes}, map[string]any{"skip_reconfirmation": true}},
		{"private_profile", ModifyInput{PrivateProfile: &yes}, map[string]any{"private_profile": true}},
		{"can_create_group", ModifyInput{CanCreateGroup: &yes}, map[string]any{"can_create_group": true}},
		{"view_diffs_file_by_file", ModifyInput{ViewDiffsFileByFile: &yes}, map[string]any{"view_diffs_file_by_file": true}},
		{"bio", ModifyInput{Bio: "bio"}, map[string]any{"bio": "bio"}},
		{"location", ModifyInput{Location: "location"}, map[string]any{"location": "location"}},
		{"job_title", ModifyInput{JobTitle: "job title"}, map[string]any{"job_title": "job title"}},
		{"organization", ModifyInput{Organization: "organization"}, map[string]any{"organization": "organization"}},
		{"note", ModifyInput{Note: "admin note"}, map[string]any{"note": "admin note"}},
		{"commit_email", ModifyInput{CommitEmail: "commit@x.test"}, map[string]any{"commit_email": "commit@x.test"}},
		{"public_email", ModifyInput{PublicEmail: "public@x.test"}, map[string]any{"public_email": "public@x.test"}},
		{"website_url", ModifyInput{WebsiteURL: "https://site.test"}, map[string]any{"website_url": "https://site.test"}},
		{"linkedin", ModifyInput{Linkedin: "linkedin"}, map[string]any{"linkedin": "linkedin"}},
		{"twitter", ModifyInput{Twitter: "twitter"}, map[string]any{"twitter": "twitter"}},
		{"skype", ModifyInput{Skype: "skype"}, map[string]any{"skype": "skype"}},
		{"provider", ModifyInput{Provider: "ldapmain"}, map[string]any{"provider": "ldapmain"}},
		{"extern_uid", ModifyInput{ExternUID: "extern-uid"}, map[string]any{"extern_uid": "extern-uid"}},
		{"projects_limit", ModifyInput{ProjectsLimit: &four}, map[string]any{"projects_limit": float64(4)}},
		{"theme_id", ModifyInput{ThemeID: &four}, map[string]any{"theme_id": float64(4)}},
		{"nothing", ModifyInput{}, map[string]any{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got capturedRequest
			client := testutil.NewTestClient(t, recordRequest(t, pathGetUser, &got, http.StatusOK, fullUserJSON))

			input := tt.input
			input.UserID = 42
			if _, err := Modify(context.Background(), client, input); err != nil {
				t.Fatalf("Modify() unexpected error: %v", err)
			}
			assertSent(t, "modify body", got.body, tt.want)
		})
	}
}

// TestCreateUser_ReadsTheLicensedEnterpriseKeys verifies the three keys
// ee/lib/ee/api/entities/user_with_admin.rb exposes reach the output when the
// instance holds the license that gates them. POST /users is one of the two
// routes presenting UserWithAdmin, and client-go's User models none of the
// three, so they can only arrive through the captured response (ADR-0021).
func TestCreateUser_ReadsTheLicensedEnterpriseKeys(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/users" {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":42,"username":"testuser","commit_email":"c@example.com","local_time":"9:00 AM",
				"enterprise_group_id":33,"enterprise_group_associated_at":"2026-01-02T03:04:05Z",
				"provisioned_by_group_id":44
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{Email: "e@x", Name: "N", Username: "u"})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.EnterpriseGroupID == nil || *out.EnterpriseGroupID != 33 {
		t.Errorf("EnterpriseGroupID = %v, want 33", out.EnterpriseGroupID)
	}
	if out.EnterpriseGroupAssociatedAt != "2026-01-02T03:04:05Z" {
		t.Errorf("EnterpriseGroupAssociatedAt = %q, want the RFC3339 timestamp", out.EnterpriseGroupAssociatedAt)
	}
	if out.ProvisionedByGroupID == nil || *out.ProvisionedByGroupID != 44 {
		t.Errorf("ProvisionedByGroupID = %v, want 44", out.ProvisionedByGroupID)
	}
	if out.CommitEmail != "c@example.com" || out.LocalTime != "9:00 AM" {
		t.Errorf("out = %+v, want the two unconditional keys beside them", out)
	}
}

// TestCreateUser_OnAnUnlicensedInstanceLeavesTheEnterpriseKeysAbsent is the
// other side of that condition: License.feature_available? is false for
// domain_verification and group_saml, GitLab exposes neither key, and the
// output must say nothing rather than name group 0.
func TestCreateUser_OnAnUnlicensedInstanceLeavesTheEnterpriseKeysAbsent(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/users" {
			testutil.RespondJSON(w, http.StatusCreated, userJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{Email: "e@x", Name: "N", Username: "u"})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.EnterpriseGroupID != nil || out.ProvisionedByGroupID != nil || out.EnterpriseGroupAssociatedAt != "" {
		t.Errorf("out = %+v, want the three licensed keys left absent", out)
	}
}

// TestModify_UserACapturedFieldTheTypeCannotHold_IsReported verifies the one
// failure the captured response adds: GitLab's answer decodes for the SDK and
// not for the keys read beside it, and the handler reports it rather than
// returning a user with those keys silently empty.
func TestModify_UserACapturedFieldTheTypeCannotHold_IsReported(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":42,"username":"u","followers":"not-a-number"}`)
	}))

	_, err := Modify(context.Background(), client, ModifyInput{UserID: 42, Name: "N"})
	if err == nil || !strings.Contains(err.Error(), "decode the captured response") {
		t.Errorf("Modify() error = %v, want the capture's decode failure", err)
	}
}
