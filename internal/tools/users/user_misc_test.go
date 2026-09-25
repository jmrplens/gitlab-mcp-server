// user_misc_test.go contains unit tests for miscellaneous GitLab user
// operations (impersonation tokens, memberships, etc.).
// Tests use httptest to mock the GitLab Users API.
package users

import (
	"context"
	"encoding/base64"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
	if len(out.NextSteps) != 1 || !strings.Contains(out.NextSteps[0], "user.current") {
		t.Errorf("NextSteps = %v, want one hint pointing at user.current", out.NextSteps)
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

// TestUploadCurrentUserAvatar_RefusedImageCarriesTheFormatHint verifies that
// both statuses GitLab refuses an image with reach the format-and-size hint,
// and that another failure does not. Asserting only that an error came back
// would pass with the two operands collapsed into one, which is what would
// send a 400 to the credentials hint instead.
func TestUploadCurrentUserAvatar_RefusedImageCarriesTheFormatHint(t *testing.T) {
	const formatHint = "avatar must be JPG/PNG/GIF"

	tests := []struct {
		name     string
		status   int
		wantHint bool
	}{
		{"unprocessable", http.StatusUnprocessableEntity, true},
		{"bad request", http.StatusBadRequest, true},
		{"unauthorized", http.StatusUnauthorized, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tt.status, `{"message":"refused"}`)
			}))
			content := base64.StdEncoding.EncodeToString([]byte("fake"))

			_, err := UploadCurrentUserAvatar(context.Background(), client, UploadCurrentUserAvatarInput{
				Filename: "avatar.png", ContentBase64: content,
			})
			if err == nil {
				t.Fatal("expected API error, got nil")
			}
			if got := strings.Contains(err.Error(), formatHint); got != tt.wantHint {
				t.Errorf("error %q carries the format hint = %v, want %v", err, got, tt.wantHint)
			}
		})
	}
}

// TestUploadCurrentUserAvatar_ACapturedFieldTheTypeCannotHold_IsReported
// verifies the failure the captured response adds here too: the upload reads
// the instance-user keys beside the SDK's decode, and a body those keys cannot
// hold is reported rather than answered with a user missing them.
func TestUploadCurrentUserAvatar_ACapturedFieldTheTypeCannotHold_IsReported(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":7,"username":"alice","followers":"not-a-number"}`)
	}))
	content := base64.StdEncoding.EncodeToString([]byte("fake"))

	_, err := UploadCurrentUserAvatar(context.Background(), client, UploadCurrentUserAvatarInput{
		Filename: "avatar.png", ContentBase64: content,
	})
	if err == nil || !strings.Contains(err.Error(), "decode the captured response") {
		t.Errorf("UploadCurrentUserAvatar() error = %v, want the capture's decode failure", err)
	}
}

// TestUploadCurrentUserAvatar_AnAnswerWithNeitherIdentityNorAvatarGetsNoHint
// verifies the recovery hint is attached to the answer it describes and not to
// any response that happens to lack an identifier: GitLab 19 sends avatar_url
// alone, and an answer carrying nothing at all is not that shape.
func TestUploadCurrentUserAvatar_AnAnswerWithNeitherIdentityNorAvatarGetsNoHint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	content := base64.StdEncoding.EncodeToString([]byte("fake"))

	out, err := UploadCurrentUserAvatar(context.Background(), client, UploadCurrentUserAvatarInput{
		Filename: "avatar.png", ContentBase64: content,
	})
	if err != nil {
		t.Fatalf("UploadCurrentUserAvatar() unexpected error: %v", err)
	}
	if len(out.NextSteps) != 0 {
		t.Errorf("NextSteps = %v, want none when GitLab sent no avatar URL", out.NextSteps)
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

// TestCreateUserRunner_EachOptionReachesTheRequest drives one runner option at
// a time and compares the whole body against the required runner_type plus
// that one. Three of the options are flags, so a case that sets them all at
// once cannot tell paused from run_untagged; one at a time can, and the empty
// case holds the tag list to being absent rather than sent as an empty array.
//
// The test this replaced set every option and asserted only the returned ID,
// while its own mock discarded the body: the comment claimed the fields were
// passed and nothing checked that any of them left the process.
func TestCreateUserRunner_EachOptionReachesTheRequest(t *testing.T) {
	five := int64(5)
	ten := int64(10)
	yes := true
	no := false
	timeout := int64(3600)

	tests := []struct {
		name  string
		input CreateUserRunnerInput
		want  map[string]any
	}{
		{"group_id", CreateUserRunnerInput{GroupID: &five}, map[string]any{"group_id": float64(5)}},
		{"project_id", CreateUserRunnerInput{ProjectID: &ten}, map[string]any{"project_id": float64(10)}},
		{"description", CreateUserRunnerInput{Description: "build runner"}, map[string]any{"description": "build runner"}},
		{"paused", CreateUserRunnerInput{Paused: &yes}, map[string]any{"paused": true}},
		{"locked", CreateUserRunnerInput{Locked: &no}, map[string]any{"locked": false}},
		{"run_untagged", CreateUserRunnerInput{RunUntagged: &yes}, map[string]any{"run_untagged": true}},
		{"tag_list", CreateUserRunnerInput{TagList: []string{"docker", "linux"}}, map[string]any{"tag_list": []any{"docker", "linux"}}},
		{"access_level", CreateUserRunnerInput{AccessLevel: "ref_protected"}, map[string]any{"access_level": "ref_protected"}},
		{"maximum_timeout", CreateUserRunnerInput{MaximumTimeout: &timeout}, map[string]any{"maximum_timeout": float64(3600)}},
		{"maintenance_note", CreateUserRunnerInput{MaintenanceNote: "quarterly"}, map[string]any{"maintenance_note": "quarterly"}},
		{"nothing", CreateUserRunnerInput{}, map[string]any{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got capturedRequest
			client := testutil.NewTestClient(t, recordRequest(t, "/api/v4/user/runners", &got, http.StatusCreated,
				`{"id":102,"token":"runner-token-xyz"}`))

			input := tt.input
			input.RunnerType = "group_type"
			out, err := CreateUserRunner(context.Background(), client, input)
			if err != nil {
				t.Fatalf("CreateUserRunner() unexpected error: %v", err)
			}
			if out.ID != 102 {
				t.Errorf("ID = %d, want 102", out.ID)
			}

			want := map[string]any{"runner_type": "group_type"}
			maps.Copy(want, tt.want)
			assertSent(t, "runner body", got.body, want)
		})
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

// TestGetUserActivities_TheFromFilterReachesTheQuery verifies that the
// from-date and the ordering are asked of GitLab, and that an unset filter is
// not invented. The fixture answers the same list either way, so the query the
// handler built is the only place the answer can be read: the test this
// replaced asserted the response and would have passed with the filter
// dropped, which is what its comment claimed to check.
func TestGetUserActivities_TheFromFilterReachesTheQuery(t *testing.T) {
	tests := []struct {
		name  string
		input GetUserActivitiesInput
		want  url.Values
	}{
		{"from", GetUserActivitiesInput{From: "2026-01-01"}, url.Values{"from": {"2026-01-01"}}},
		{"order_by and sort", GetUserActivitiesInput{OrderBy: "id", Sort: "desc"}, url.Values{"order_by": {"id"}, "sort": {"desc"}}},
		{"nothing", GetUserActivitiesInput{}, url.Values{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got capturedRequest
			client := testutil.NewTestClient(t, recordRequest(t, "/api/v4/user/activities", &got, http.StatusOK,
				`[{"username":"user1","last_activity_on":"2026-06-15"}]`))

			out, err := GetUserActivities(context.Background(), client, tt.input)
			if err != nil {
				t.Fatalf("GetUserActivities() unexpected error: %v", err)
			}
			if len(out.Activities) != 1 || out.Activities[0].LastActivityOn != "2026-06-15" {
				t.Errorf("activities = %+v, want the one fixture entry", out.Activities)
			}
			assertSent(t, "activities query", got.query, tt.want)
		})
	}
}

// TestGetUserActivities_AnActivityWithoutADateLeavesItEmpty verifies the
// absent side of the timestamp guard: last_activity_on is the whole point of
// this endpoint, and an entry without one must publish nothing rather than the
// zero day as the date a user was last seen.
func TestGetUserActivities_AnActivityWithoutADateLeavesItEmpty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"username":"user1"}]`)
	}))

	out, err := GetUserActivities(context.Background(), client, GetUserActivitiesInput{})
	if err != nil {
		t.Fatalf("GetUserActivities() unexpected error: %v", err)
	}
	if len(out.Activities) != 1 {
		t.Fatalf("got %d activities, want 1", len(out.Activities))
	}
	if out.Activities[0].LastActivityOn != "" {
		t.Errorf("LastActivityOn = %q, want empty when GitLab sent none", out.Activities[0].LastActivityOn)
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

// TestGetUserMemberships_TheTypeFilterReachesTheQuery verifies that the
// membership type and the ordering are asked of GitLab. The fixture answers
// with a Namespace membership whether or not the filter was sent, so reading
// the response (which is what the test this replaced did under a comment
// claiming otherwise) proves nothing about the request.
func TestGetUserMemberships_TheTypeFilterReachesTheQuery(t *testing.T) {
	tests := []struct {
		name  string
		input GetUserMembershipsInput
		want  url.Values
	}{
		{"type", GetUserMembershipsInput{Type: "Namespace"}, url.Values{"type": {"Namespace"}}},
		{"order_by and sort", GetUserMembershipsInput{OrderBy: "id", Sort: "asc"}, url.Values{"order_by": {"id"}, "sort": {"asc"}}},
		{"nothing", GetUserMembershipsInput{}, url.Values{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got capturedRequest
			client := testutil.NewTestClient(t, recordRequest(t, "/api/v4/users/42/memberships", &got, http.StatusOK,
				`[{"source_id":1,"source_name":"grp","source_type":"Namespace","access_level":40}]`))

			input := tt.input
			input.UserID = 42
			out, err := GetUserMemberships(context.Background(), client, input)
			if err != nil {
				t.Fatalf("GetUserMemberships() unexpected error: %v", err)
			}
			if len(out.Memberships) != 1 || out.Memberships[0].SourceType != "Namespace" {
				t.Errorf("memberships = %+v, want the one fixture entry", out.Memberships)
			}
			assertSent(t, "memberships query", got.query, tt.want)
		})
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

// TestFormatUserActivitiesMarkdownString_WithData verifies the whole list
// render: the dates go through the display form rather than reaching the
// reader as the ISO strings GitLab sent, and the footer names no
// preserve-links instruction over a table that carries no link.
func TestFormatUserActivitiesMarkdownString_WithData(t *testing.T) {
	out := UserActivitiesOutput{
		Activities: []UserActivityOutput{
			{Username: "alice", LastActivityOn: "2026-06-15"},
			{Username: "bob", LastActivityOn: "2026-06-14"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}

	assertMarkdown(t, FormatUserActivitiesMarkdownString(out),
		"## User Activities (2)\n\n"+
			"| Username | Last Activity |\n"+
			"| --- | --- |\n"+
			"| alice | 15 Jun 2026 |\n"+
			"| bob | 14 Jun 2026 |\n"+
			"\nPage 1 of 1 | 2 items total | 20 per page\n"+
			"\n---\n💡 **Next steps:**\n"+
			"- Use action 'user.get' to view full details for a user\n")
}

// TestFormatUserActivitiesMarkdownString_Empty verifies that a list with
// nothing in it is the one sentence and nothing else.
func TestFormatUserActivitiesMarkdownString_Empty(t *testing.T) {
	assertMarkdown(t, FormatUserActivitiesMarkdownString(UserActivitiesOutput{}), "No user activities found.\n")
}

// TestFormatUserMembershipsMarkdownString_WithData verifies the whole list
// render. The access level is the name GitLab gives it rather than the bare
// number that used to reach the reader as "30".
func TestFormatUserMembershipsMarkdownString_WithData(t *testing.T) {
	out := UserMembershipsOutput{
		Memberships: []UserMembershipOutput{
			{SourceID: 1, SourceName: "my-project", SourceType: "Project", AccessLevel: 30},
			{SourceID: 2, SourceName: "my-group", SourceType: "Namespace", AccessLevel: 50},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}

	assertMarkdown(t, FormatUserMembershipsMarkdownString(out),
		"## User Memberships (2)\n\n"+
			"| Source ID | Source Name | Source Type | Access Level |\n"+
			"| --- | --- | --- | --- |\n"+
			"| 1 | my-project | Project | Developer |\n"+
			"| 2 | my-group | Namespace | Owner |\n"+
			"\nPage 1 of 1 | 2 items total | 20 per page\n"+
			"\n---\n💡 **Next steps:**\n"+
			"- Use action 'user.get' to view the user's profile\n")
}

// TestFormatUserMembershipsMarkdownString_Empty verifies that a list with
// nothing in it is the one sentence and nothing else.
func TestFormatUserMembershipsMarkdownString_Empty(t *testing.T) {
	assertMarkdown(t, FormatUserMembershipsMarkdownString(UserMembershipsOutput{}), "No memberships found.\n")
}

// TestFormatUserRunnerMarkdownString verifies the whole runner card. The token
// is a value GitLab shows once, so the card writes it as a secret and closes
// with the sentence that says so.
func TestFormatUserRunnerMarkdownString(t *testing.T) {
	out := UserRunnerOutput{
		ID: 101, Token: "glrt-abc123", TokenExpiresAt: "2026-06-01T00:00:00Z",
	}

	assertMarkdown(t, FormatUserRunnerMarkdownString(out),
		"## User Runner Created\n\n"+
			"- **ID**: 101\n"+
			"- **Token**: `glrt-abc123`\n"+
			"- **Token Expires At**: 1 Jun 2026 00:00 UTC\n"+
			"\n---\n💡 **Next steps:**\n"+
			"- Store the token securely. It cannot be retrieved later\n")
}

// TestFormatUserRunnerMarkdownString_NoExpiry verifies that an absent expiry
// writes no row.
func TestFormatUserRunnerMarkdownString_NoExpiry(t *testing.T) {
	assertMarkdown(t, FormatUserRunnerMarkdownString(UserRunnerOutput{ID: 1, Token: "tok"}),
		"## User Runner Created\n\n"+
			"- **ID**: 1\n"+
			"- **Token**: `tok`\n"+
			"\n---\n💡 **Next steps:**\n"+
			"- Store the token securely. It cannot be retrieved later\n")
}

// TestFormatUserRunnerMarkdownString_TokenShapes_AreCodeSpannedAndGuarded
// verifies that the created runner's token is written inside a code span and
// that a card with no token announces none, and carries no hint about storing
// a credential it never showed.
//
// A token written as bare Markdown is read as Markdown, so an underscore pair
// in it is eaten as emphasis and the value copied back is not the one GitLab
// minted.
func TestFormatUserRunnerMarkdownString_TokenShapes_AreCodeSpannedAndGuarded(t *testing.T) {
	t.Run("token is inside a code span", func(t *testing.T) {
		assertMarkdown(t, FormatUserRunnerMarkdownString(UserRunnerOutput{ID: 101, Token: "glrt-a_b_c"}),
			"## User Runner Created\n\n"+
				"- **ID**: 101\n"+
				"- **Token**: `glrt-a_b_c`\n"+
				"\n---\n💡 **Next steps:**\n"+
				"- Store the token securely. It cannot be retrieved later\n")
	})
	t.Run("empty token writes no line and no hint", func(t *testing.T) {
		assertMarkdown(t, FormatUserRunnerMarkdownString(UserRunnerOutput{ID: 101, TokenExpiresAt: "2026-06-01T00:00:00Z"}),
			"## User Runner Created\n\n"+
				"- **ID**: 101\n"+
				"- **Token Expires At**: 1 Jun 2026 00:00 UTC\n")
	})
}

// TestFormatDeleteUserIdentityMarkdownString verifies the whole card for both
// outcomes: the deletion flag is the tick or the cross rather than a success
// glyph printed beside the word "false".
func TestFormatDeleteUserIdentityMarkdownString(t *testing.T) {
	tests := []struct {
		name  string
		input DeleteUserIdentityOutput
		want  string
	}{
		{
			name:  "the identity was deleted",
			input: DeleteUserIdentityOutput{UserID: 42, Provider: "saml", Deleted: true},
			want:  "## User Identity Deleted\n\n- **ID**: 42\n- **Provider**: saml\n- **Deleted**: ✅\n",
		},
		{
			name:  "the identity was not deleted",
			input: DeleteUserIdentityOutput{UserID: 42, Provider: "saml", Deleted: false},
			want:  "## User Identity Deleted\n\n- **ID**: 42\n- **Provider**: saml\n- **Deleted**: ❌\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertMarkdown(t, FormatDeleteUserIdentityMarkdownString(tt.input), tt.want)
		})
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
