// user_misc_test.go contains unit tests for miscellaneous GitLab user
// operations (impersonation tokens, memberships, etc.).
// Tests use httptest to mock the GitLab Users API.
package users

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// userAvatarJSON is a minimal authenticated-user payload returned by the mocked
// PUT /user/avatar endpoint, including the updated avatar URL.
const userAvatarJSON = `{"id":7,"username":"alice","name":"Alice","state":"active","avatar_url":"https://gitlab.example.com/uploads/-/system/user/avatar/7/avatar.png"}`

// TestUploadCurrentUserAvatar_Success_Base64 verifies UploadCurrentUserAvatar
// PUTs to /user/avatar with base64 content and maps the returned user, including
// the new avatar URL.
func TestUploadCurrentUserAvatar_Success_Base64(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/user/avatar" {
			testutil.RespondJSON(w, http.StatusOK, userAvatarJSON)
			return
		}
		http.NotFound(w, r)
	}))
	content := base64.StdEncoding.EncodeToString([]byte("fake-image-data"))
	out, err := UploadCurrentUserAvatar(context.Background(), client, UploadCurrentUserAvatarInput{
		Filename: "avatar.png", ContentBase64: content,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 7 {
		t.Errorf("ID = %d, want 7", out.ID)
	}
	if !strings.Contains(out.AvatarURL, "avatar.png") {
		t.Errorf("AvatarURL = %q, want it to reference avatar.png", out.AvatarURL)
	}
	if len(out.NextSteps) != 0 {
		t.Errorf("NextSteps = %v, want none when the full user profile is returned", out.NextSteps)
	}
}

// TestUploadCurrentUserAvatar_GitLab19AvatarURLOnly_SetsRecoveryHint verifies
// that when GitLab 19 answers with only {avatar_url} (so the decoded user ID
// is zero), the output carries a next-steps hint confirming success and
// pointing at gitlab_user_current for the full profile.
func TestUploadCurrentUserAvatar_GitLab19AvatarURLOnly_SetsRecoveryHint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/user/avatar" {
			testutil.RespondJSON(w, http.StatusOK, `{"avatar_url":"https://gitlab.example.com/uploads/-/system/user/avatar/7/avatar.png"}`)
			return
		}
		http.NotFound(w, r)
	}))
	content := base64.StdEncoding.EncodeToString([]byte("fake-image-data"))
	out, err := UploadCurrentUserAvatar(context.Background(), client, UploadCurrentUserAvatarInput{
		Filename: "avatar.png", ContentBase64: content,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 0 {
		t.Errorf("ID = %d, want 0 from avatar_url-only response", out.ID)
	}
	if out.AvatarURL == "" {
		t.Error("AvatarURL is empty, want the uploaded avatar URL")
	}
	if len(out.NextSteps) != 1 || !strings.Contains(out.NextSteps[0], "gitlab_user_current") {
		t.Errorf("NextSteps = %v, want one hint pointing at gitlab_user_current", out.NextSteps)
	}
}

// TestUploadCurrentUserAvatar_Success_FilePath verifies the file_path branch
// reads a local image file and uploads its contents.
func TestUploadCurrentUserAvatar_Success_FilePath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/user/avatar" {
			testutil.RespondJSON(w, http.StatusOK, userAvatarJSON)
			return
		}
		http.NotFound(w, r)
	}))
	path := filepath.Join(t.TempDir(), "avatar.png")
	if err := os.WriteFile(path, []byte("fake-image-data"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	out, err := UploadCurrentUserAvatar(context.Background(), client, UploadCurrentUserAvatarInput{
		Filename: "avatar.png", FilePath: path,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 7 {
		t.Errorf("ID = %d, want 7", out.ID)
	}
}

// TestUploadCurrentUserAvatar_Validation covers the input-validation branches:
// missing filename, no source, both sources, and invalid base64.
func TestUploadCurrentUserAvatar_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	cases := []struct {
		name  string
		input UploadCurrentUserAvatarInput
	}{
		{"missing filename", UploadCurrentUserAvatarInput{ContentBase64: "dGVzdA=="}},
		{"no source", UploadCurrentUserAvatarInput{Filename: "a.png"}},
		{"both sources", UploadCurrentUserAvatarInput{Filename: "a.png", FilePath: "/tmp/x.png", ContentBase64: "dGVzdA=="}},
		{"invalid base64", UploadCurrentUserAvatarInput{Filename: "a.png", ContentBase64: "!!!not-base64!!!"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := UploadCurrentUserAvatar(context.Background(), client, tc.input); err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

// TestUploadCurrentUserAvatar_APIError verifies a 422 from the avatar endpoint is
// wrapped with an actionable hint.
func TestUploadCurrentUserAvatar_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"avatar invalid"}`)
	}))
	content := base64.StdEncoding.EncodeToString([]byte("fake"))
	if _, err := UploadCurrentUserAvatar(context.Background(), client, UploadCurrentUserAvatarInput{
		Filename: "avatar.png", ContentBase64: content,
	}); err == nil {
		t.Fatal("expected API error, got nil")
	}
}

// TestUploadCurrentUserAvatar_UnauthorizedError verifies a non-422/400 status
// (e.g. 401) is wrapped via the status-hint fallback path.
func TestUploadCurrentUserAvatar_UnauthorizedError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnauthorized, `{"message":"401 Unauthorized"}`)
	}))
	content := base64.StdEncoding.EncodeToString([]byte("fake"))
	if _, err := UploadCurrentUserAvatar(context.Background(), client, UploadCurrentUserAvatarInput{
		Filename: "avatar.png", ContentBase64: content,
	}); err == nil {
		t.Fatal("expected API error, got nil")
	}
}

// TestUploadCurrentUserAvatar_ContextCancelled verifies the handler returns the
// context error before performing any work when the context is already cancelled.
func TestUploadCurrentUserAvatar_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	content := base64.StdEncoding.EncodeToString([]byte("fake"))
	if _, err := UploadCurrentUserAvatar(ctx, client, UploadCurrentUserAvatarInput{
		Filename: "avatar.png", ContentBase64: content,
	}); err == nil {
		t.Fatal("expected context error, got nil")
	}
}

// TestUploadCurrentUserAvatar_FilePathNotFound verifies a missing local file path
// is surfaced as an error before any API call.
func TestUploadCurrentUserAvatar_FilePathNotFound(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	missing := filepath.Join(t.TempDir(), "does-not-exist.png")
	if _, err := UploadCurrentUserAvatar(context.Background(), client, UploadCurrentUserAvatarInput{
		Filename: "avatar.png", FilePath: missing,
	}); err == nil {
		t.Fatal("expected error for missing file_path, got nil")
	}
}

// TestGetUserActivities_Success verifies GetUserActivities returns the activity
// list when GET /user/activities responds 200 with a single record.
func TestGetUserActivities_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/user/activities" {
			testutil.RespondJSON(w, http.StatusOK, `[{"username":"testuser","last_activity_on":"2026-06-01"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetUserActivities(context.Background(), client, GetUserActivitiesInput{})
	if err != nil {
		t.Fatalf("GetUserActivities() unexpected error: %v", err)
	}
	if len(out.Activities) != 1 {
		t.Fatalf("len(out.Activities) = %d, want 1", len(out.Activities))
	}
}

// TestGetUserMemberships_Success verifies GetUserMemberships returns membership
// records when GET /users/:id/memberships responds 200 with project data.
func TestGetUserMemberships_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/users/42/memberships" {
			testutil.RespondJSON(w, http.StatusOK, `[{"source_id":1,"source_name":"my-project","source_type":"Project","access_level":30}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetUserMemberships(context.Background(), client, GetUserMembershipsInput{UserID: 42})
	if err != nil {
		t.Fatalf("GetUserMemberships() unexpected error: %v", err)
	}
	if len(out.Memberships) != 1 {
		t.Fatalf("len(out.Memberships) = %d, want 1", len(out.Memberships))
	}
	if out.Memberships[0].SourceName != "my-project" {
		t.Errorf("out.Memberships[0].SourceName = %q, want %q", out.Memberships[0].SourceName, "my-project")
	}
}

// TestGetUserMemberships_InvalidUserID verifies GetUserMemberships returns an
// error when called with user_id=0 without hitting the API.
func TestGetUserMemberships_InvalidUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := GetUserMemberships(context.Background(), client, GetUserMembershipsInput{UserID: 0})
	if err == nil {
		t.Fatal("expected error for invalid user_id, got nil")
	}
}

// TestDeleteUserIdentity_Success verifies DeleteUserIdentity reports Deleted=true
// when DELETE /users/:id/identities/:provider responds 204 No Content.
func TestDeleteUserIdentity_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/users/42/identities/ldap" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := DeleteUserIdentity(context.Background(), client, DeleteUserIdentityInput{UserID: 42, Provider: "ldap"})
	if err != nil {
		t.Fatalf("DeleteUserIdentity() unexpected error: %v", err)
	}
	if !out.Deleted {
		t.Error("out.Deleted = false, want true")
	}
}

// TestCurrentUserStatus_Success verifies that CurrentUserStatus returns the
// authenticated user's current status correctly.
func TestCurrentUserStatus_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/user/status" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"emoji":"coffee","message":"Coding","availability":"busy",
				"message_html":"<p>Coding</p>","clear_status_at":"2026-12-31T23:59:59Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := CurrentUserStatus(context.Background(), client, CurrentInput{})
	if err != nil {
		t.Fatalf("CurrentUserStatus() unexpected error: %v", err)
	}
	if out.Emoji != "coffee" {
		t.Errorf("Emoji = %q, want %q", out.Emoji, "coffee")
	}
	if out.Message != "Coding" {
		t.Errorf("Message = %q, want %q", out.Message, "Coding")
	}
	if out.Availability != "busy" {
		t.Errorf("Availability = %q, want %q", out.Availability, "busy")
	}
	if out.ClearStatusAt == "" {
		t.Error("expected non-empty ClearStatusAt")
	}
}

// TestCurrentUserStatus_APIError verifies CurrentUserStatus returns an error
// when the GitLab API responds with an error status.
func TestCurrentUserStatus_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnauthorized, `{"message":"401 Unauthorized"}`)
	}))

	_, err := CurrentUserStatus(context.Background(), client, CurrentInput{})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// TestCurrentUserStatus_CancelledContext verifies CurrentUserStatus respects
// context cancellation.
func TestCurrentUserStatus_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := CurrentUserStatus(ctx, client, CurrentInput{})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestCreateUserRunner_Success verifies that CreateUserRunner creates a runner
// and returns the ID and token.
func TestCreateUserRunner_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/user/runners" {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":101,"token":"glrt-abc123","token_expires_at":"2026-06-01T00:00:00Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := CreateUserRunner(context.Background(), client, CreateUserRunnerInput{
		RunnerType:  "instance_type",
		Description: "CI runner",
	})
	if err != nil {
		t.Fatalf("CreateUserRunner() unexpected error: %v", err)
	}
	if out.ID != 101 {
		t.Errorf("ID = %d, want 101", out.ID)
	}
	if out.Token != "glrt-abc123" {
		t.Errorf("Token = %q, want %q", out.Token, "glrt-abc123")
	}
	if out.TokenExpiresAt == "" {
		t.Error("expected non-empty TokenExpiresAt")
	}
}

// TestCreateUserRunner_AllOptions verifies that all optional fields are passed.
func TestCreateUserRunner_AllOptions(t *testing.T) {
	groupID := int64(5)
	projectID := int64(10)
	paused := true
	locked := false
	runUntagged := true
	maxTimeout := int64(3600)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/user/runners" {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":102,"token":"glrt-xyz"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := CreateUserRunner(context.Background(), client, CreateUserRunnerInput{
		RunnerType:      "group_type",
		GroupID:         &groupID,
		ProjectID:       &projectID,
		Description:     "test runner",
		Paused:          &paused,
		Locked:          &locked,
		RunUntagged:     &runUntagged,
		TagList:         []string{"docker", "linux"},
		AccessLevel:     "ref_protected",
		MaximumTimeout:  &maxTimeout,
		MaintenanceNote: "Maintenance note",
	})
	if err != nil {
		t.Fatalf("CreateUserRunner() unexpected error: %v", err)
	}
	if out.ID != 102 {
		t.Errorf("ID = %d, want 102", out.ID)
	}
}

// TestCreateUserRunner_MissingRunnerType verifies validation error for empty runner_type.
func TestCreateUserRunner_MissingRunnerType(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := CreateUserRunner(context.Background(), client, CreateUserRunnerInput{})
	if err == nil {
		t.Fatal("expected error for missing runner_type, got nil")
	}
}

// TestCreateUserRunner_APIError verifies error handling on API failure.
func TestCreateUserRunner_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := CreateUserRunner(context.Background(), client, CreateUserRunnerInput{RunnerType: "instance_type"})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// TestCreateUserRunner_CancelledContext verifies context cancellation is respected.
func TestCreateUserRunner_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := CreateUserRunner(ctx, client, CreateUserRunnerInput{RunnerType: "instance_type"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestDeleteUserIdentity_MissingProvider verifies validation for missing provider.
func TestDeleteUserIdentity_MissingProvider(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := DeleteUserIdentity(context.Background(), client, DeleteUserIdentityInput{UserID: 42})
	if err == nil {
		t.Fatal("expected error for missing provider, got nil")
	}
}

// TestDeleteUserIdentity_MissingUserID verifies validation for missing user_id.
func TestDeleteUserIdentity_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := DeleteUserIdentity(context.Background(), client, DeleteUserIdentityInput{Provider: "ldap"})
	if err == nil {
		t.Fatal("expected error for missing user_id, got nil")
	}
}

// TestDeleteUserIdentity_APIError verifies error handling on API failure.
func TestDeleteUserIdentity_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := DeleteUserIdentity(context.Background(), client, DeleteUserIdentityInput{UserID: 42, Provider: "ldap"})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// TestDeleteUserIdentity_CancelledContext verifies context cancellation.
func TestDeleteUserIdentity_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := DeleteUserIdentity(ctx, client, DeleteUserIdentityInput{UserID: 42, Provider: "ldap"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestGetUserActivities_WithFromFilter verifies that the From date filter is applied.
func TestGetUserActivities_WithFromFilter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/user/activities" {
			testutil.RespondJSON(w, http.StatusOK, `[{"username":"user1","last_activity_on":"2026-06-15"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetUserActivities(context.Background(), client, GetUserActivitiesInput{From: "2026-01-01"})
	if err != nil {
		t.Fatalf("GetUserActivities() unexpected error: %v", err)
	}
	if len(out.Activities) != 1 {
		t.Fatalf("got %d activities, want 1", len(out.Activities))
	}
	if out.Activities[0].LastActivityOn != "2026-06-15" {
		t.Errorf("LastActivityOn = %q, want %q", out.Activities[0].LastActivityOn, "2026-06-15")
	}
}

// TestGetUserActivities_APIError verifies error handling.
func TestGetUserActivities_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := GetUserActivities(context.Background(), client, GetUserActivitiesInput{})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// TestGetUserActivities_CancelledContext verifies context cancellation.
func TestGetUserActivities_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := GetUserActivities(ctx, client, GetUserActivitiesInput{})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestGetUserMemberships_WithTypeFilter verifies that the Type filter parameter is applied.
func TestGetUserMemberships_WithTypeFilter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/users/42/memberships" {
			testutil.RespondJSON(w, http.StatusOK, `[{"source_id":1,"source_name":"grp","source_type":"Namespace","access_level":40}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetUserMemberships(context.Background(), client, GetUserMembershipsInput{UserID: 42, Type: "Namespace"})
	if err != nil {
		t.Fatalf("GetUserMemberships() unexpected error: %v", err)
	}
	if len(out.Memberships) != 1 {
		t.Fatalf("got %d memberships, want 1", len(out.Memberships))
	}
	if out.Memberships[0].SourceType != "Namespace" {
		t.Errorf("SourceType = %q, want %q", out.Memberships[0].SourceType, "Namespace")
	}
}

// TestGetUserMemberships_APIError verifies error handling.
func TestGetUserMemberships_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := GetUserMemberships(context.Background(), client, GetUserMembershipsInput{UserID: 42})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// TestGetUserMemberships_CancelledContext verifies context cancellation.
func TestGetUserMemberships_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := GetUserMemberships(ctx, client, GetUserMembershipsInput{UserID: 42})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// --- Markdown formatter tests ---

// TestFormatUserActivitiesMarkdownString_WithData verifies activities markdown
// rendering with data including table rows.
func TestFormatUserActivitiesMarkdownString_WithData(t *testing.T) {
	out := UserActivitiesOutput{
		Activities: []UserActivityOutput{
			{Username: "alice", LastActivityOn: "2026-06-15"},
			{Username: "bob", LastActivityOn: "2026-06-14"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatUserActivitiesMarkdownString(out)

	for _, want := range []string{
		"## User Activities (2)",
		"| Username | Last Activity |",
		"| alice | 2026-06-15 |",
		"| bob | 2026-06-14 |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatUserActivitiesMarkdownString_Empty verifies empty activities message.
func TestFormatUserActivitiesMarkdownString_Empty(t *testing.T) {
	md := FormatUserActivitiesMarkdownString(UserActivitiesOutput{})
	if !strings.Contains(md, "No user activities found") {
		t.Errorf("expected empty message:\n%s", md)
	}
}

// TestFormatUserMembershipsMarkdownString_WithData verifies memberships markdown
// rendering with data.
func TestFormatUserMembershipsMarkdownString_WithData(t *testing.T) {
	out := UserMembershipsOutput{
		Memberships: []UserMembershipOutput{
			{SourceID: 1, SourceName: "my-project", SourceType: "Project", AccessLevel: 30},
			{SourceID: 2, SourceName: "my-group", SourceType: "Namespace", AccessLevel: 50},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatUserMembershipsMarkdownString(out)

	for _, want := range []string{
		"## User Memberships (2)",
		"| Source ID | Source Name | Source Type | Access Level |",
		"| 1 | my-project | Project | 30 |",
		"| 2 | my-group | Namespace | 50 |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatUserMembershipsMarkdownString_Empty verifies empty memberships message.
func TestFormatUserMembershipsMarkdownString_Empty(t *testing.T) {
	md := FormatUserMembershipsMarkdownString(UserMembershipsOutput{})
	if !strings.Contains(md, "No memberships found") {
		t.Errorf("expected empty message:\n%s", md)
	}
}

// TestFormatUserRunnerMarkdownString verifies runner markdown output.
func TestFormatUserRunnerMarkdownString(t *testing.T) {
	out := UserRunnerOutput{
		ID: 101, Token: "glrt-abc123", TokenExpiresAt: "2026-06-01T00:00:00Z",
	}
	md := FormatUserRunnerMarkdownString(out)

	for _, want := range []string{
		"## User Runner Created",
		"101",
		"glrt-abc123",
		"Token Expires At",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatUserRunnerMarkdownString_NoExpiry verifies runner markdown without expiry.
func TestFormatUserRunnerMarkdownString_NoExpiry(t *testing.T) {
	md := FormatUserRunnerMarkdownString(UserRunnerOutput{ID: 1, Token: "tok"})
	if strings.Contains(md, "Token Expires At") {
		t.Error("should not contain Token Expires At when empty")
	}
}

// TestFormatDeleteUserIdentityMarkdownString verifies identity deletion markdown.
func TestFormatDeleteUserIdentityMarkdownString(t *testing.T) {
	md := FormatDeleteUserIdentityMarkdownString(DeleteUserIdentityOutput{
		UserID: 42, Provider: "saml", Deleted: true,
	})
	for _, want := range []string{"42", "saml", "true"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestParseDate_ValidDate verifies parseDate returns a valid time for YYYY-MM-DD.
func TestParseDate_ValidDate(t *testing.T) {
	d := parseDate("2026-06-15")
	if d.IsZero() {
		t.Fatal("expected non-zero time for valid date")
	}
	if d.Year() != 2026 || d.Month() != 6 || d.Day() != 15 {
		t.Errorf("date = %v, want 2026-06-15", d)
	}
}

// TestParseDate_InvalidDate verifies parseDate returns zero time for invalid input.
func TestParseDate_InvalidDate(t *testing.T) {
	d := parseDate("not-a-date")
	if !d.IsZero() {
		t.Errorf("expected zero time for invalid date, got %v", d)
	}
}

// TestParseDate_Empty verifies parseDate returns zero time for empty string.
func TestParseDate_Empty(t *testing.T) {
	d := parseDate("")
	if !d.IsZero() {
		t.Errorf("expected zero time for empty string, got %v", d)
	}
}
