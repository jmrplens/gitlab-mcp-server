// users_test.go contains unit tests for GitLab user operations.
package users

import (
	"context"
	"net/http"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	// errExpAPIFailure identifies the err exp API failure constant used by this package.
	errExpAPIFailure = "expected error for API failure, got nil"
	// pathCurrentUser identifies the path current user constant used by this package.
	pathCurrentUser = "/api/v4/user"
	// pathListUsers identifies the path list users constant used by this package.
	pathListUsers = "/api/v4/users"
	// pathGetUser identifies the path get user constant used by this package.
	pathGetUser = "/api/v4/users/42"
	// pathGetUserStatus identifies the path get user status constant used by this package.
	pathGetUserStatus = "/api/v4/users/42/status"
	// pathSetUserStatus identifies the path set user status constant used by this package.
	pathSetUserStatus = "/api/v4/user/status"
	// pathListSSHKeys identifies the path list SSH keys constant used by this package.
	pathListSSHKeys = "/api/v4/user/keys"
	// pathListEmails identifies the path list emails constant used by this package.
	pathListEmails = "/api/v4/user/emails"
	// pathContribEvents identifies the path contrib events constant used by this package.
	pathContribEvents = "/api/v4/users/42/events"
	// pathAssociationsCount identifies the path associations count constant used by this package.
	pathAssociationsCount = "/api/v4/users/42/associations_count"
)

// TestCurrent_Success verifies Current when success.
func TestCurrent_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathCurrentUser {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":1,
				"username":"testuser",
				"email":"test@example.com",
				"name":"Test User",
				"state":"active",
				"web_url":"https://gitlab.example.com/testuser",
				"avatar_url":"https://gitlab.example.com/uploads/-/system/user/avatar/1/avatar.png",
				"is_admin":false,
				"bio":"Go developer"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Current(context.Background(), client, CurrentInput{})
	if err != nil {
		t.Fatalf("Current() unexpected error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("out.ID = %d, want 1", out.ID)
	}
	if out.Username != "testuser" {
		t.Errorf("out.Username = %q, want %q", out.Username, "testuser")
	}
	if out.Email != "test@example.com" {
		t.Errorf("out.Email = %q, want %q", out.Email, "test@example.com")
	}
	if out.State != "active" {
		t.Errorf("out.State = %q, want %q", out.State, "active")
	}
	if out.IsAdmin {
		t.Error("out.IsAdmin = true, want false")
	}
	if out.Bio != "Go developer" {
		t.Errorf("out.Bio = %q, want %q", out.Bio, "Go developer")
	}
}

// TestCurrent_APIError verifies Current when API error.
func TestCurrent_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnauthorized, `{"message":"401 Unauthorized"}`)
	}))

	_, err := Current(context.Background(), client, CurrentInput{})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestCurrent_CancelledContext verifies Current when cancelled context.
func TestCurrent_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Current(ctx, client, CurrentInput{})
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}

// List Users.

// TestList_UsersSuccess verifies List when users success.
func TestList_UsersSuccess(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathListUsers {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":1,"username":"alice","name":"Alice","email":"alice@example.com","state":"active"},
				{"id":2,"username":"bob","name":"Bob","email":"bob@example.com","state":"active"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{Search: "a"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Users) != 2 {
		t.Fatalf("got %d users, want 2", len(out.Users))
	}
	if out.Users[0].Username != "alice" {
		t.Errorf("Users[0].Username = %q, want %q", out.Users[0].Username, "alice")
	}
}

// TestList_UsersAPIError verifies List when users API error.
func TestList_UsersAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// Get User.

// TestGet_UserSuccess verifies Get when user success.
func TestGet_UserSuccess(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGetUser {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":42,
				"username":"testuser",
				"name":"Test User",
				"email":"test@example.com",
				"state":"active",
				"bio":"Developer",
				"scim_identities":[{"extern_uid":"scim-user-42","group_id":7,"active":true}]
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{UserID: 42})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.ID != 42 {
		t.Errorf("out.ID = %d, want 42", out.ID)
	}
	if out.Bio != "Developer" {
		t.Errorf("out.Bio = %q, want %q", out.Bio, "Developer")
	}
	if len(out.SCIMIdentities) != 1 {
		t.Fatalf("got %d SCIM identities, want 1", len(out.SCIMIdentities))
	}
	if out.SCIMIdentities[0].ExternUID != "scim-user-42" {
		t.Errorf("SCIMIdentities[0].ExternUID = %q, want %q", out.SCIMIdentities[0].ExternUID, "scim-user-42")
	}
	if out.SCIMIdentities[0].GroupID != 7 {
		t.Errorf("SCIMIdentities[0].GroupID = %d, want 7", out.SCIMIdentities[0].GroupID)
	}
	if !out.SCIMIdentities[0].Active {
		t.Error("SCIMIdentities[0].Active = false, want true")
	}
}

// TestGet_UserValidation verifies Get when user validation.
func TestGet_UserValidation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Get(context.Background(), client, GetInput{})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestGet_UserAPIError verifies Get when user API error.
func TestGet_UserAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := Get(context.Background(), client, GetInput{UserID: 999})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestToSCIMIdentityOutputs_MixedInputs_ReturnsExpectedSlices verifies SCIM
// identity conversion handles valid, empty, and nil-only slices consistently
// for omitempty output.
func TestToSCIMIdentityOutputs_MixedInputs_ReturnsExpectedSlices(t *testing.T) {
	tests := []struct {
		name       string
		identities []*gl.SCIMIdentity
		want       []SCIMIdentityOutput
	}{
		{
			name:       "nil slice",
			identities: nil,
			want:       nil,
		},
		{
			name:       "empty slice",
			identities: []*gl.SCIMIdentity{},
			want:       nil,
		},
		{
			name:       "nil only",
			identities: []*gl.SCIMIdentity{nil},
			want:       nil,
		},
		{
			name: "filters nil identities",
			identities: []*gl.SCIMIdentity{
				nil,
				{ExternUID: "scim-user-42", GroupID: 7, Active: true},
			},
			want: []SCIMIdentityOutput{{ExternUID: "scim-user-42", GroupID: 7, Active: true}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toSCIMIdentityOutputs(tt.identities)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("got non-nil identities %+v, want nil", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d identities, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("identity[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// Get User Status.

// TestGet_UserStatusSuccess verifies Get when user status success.
func TestGet_UserStatusSuccess(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGetUserStatus {
			testutil.RespondJSON(w, http.StatusOK, `{
				"emoji":"coffee","message":"Working","availability":"busy"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetStatus(context.Background(), client, GetStatusInput{UserID: 42})
	if err != nil {
		t.Fatalf("GetStatus() unexpected error: %v", err)
	}
	if out.Emoji != "coffee" {
		t.Errorf("out.Emoji = %q, want %q", out.Emoji, "coffee")
	}
	if out.Message != "Working" {
		t.Errorf("out.Message = %q, want %q", out.Message, "Working")
	}
	if out.Availability != "busy" {
		t.Errorf("out.Availability = %q, want %q", out.Availability, "busy")
	}
}

// TestGet_UserStatusValidation verifies Get when user status validation.
func TestGet_UserStatusValidation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := GetStatus(context.Background(), client, GetStatusInput{})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// Set User Status.

// TestSetUserStatus_Success verifies SetUserStatus when success.
func TestSetUserStatus_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathSetUserStatus {
			testutil.RespondJSON(w, http.StatusOK, `{
				"emoji":"coffee","message":"On break","availability":"busy"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := SetStatus(context.Background(), client, SetStatusInput{
		Emoji:        "coffee",
		Message:      "On break",
		Availability: "busy",
	})
	if err != nil {
		t.Fatalf("SetStatus() unexpected error: %v", err)
	}
	if out.Emoji != "coffee" {
		t.Errorf("out.Emoji = %q, want %q", out.Emoji, "coffee")
	}
	if out.Message != "On break" {
		t.Errorf("out.Message = %q, want %q", out.Message, "On break")
	}
}

// TestSetUserStatus_APIError verifies SetUserStatus when API error.
func TestSetUserStatus_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := SetStatus(context.Background(), client, SetStatusInput{Emoji: "x"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// List SSH Keys.

// TestListSSHKeys_Success verifies ListSSHKeys when success.
func TestListSSHKeys_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathListSSHKeys {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":1,"title":"Work Laptop","key":"ssh-ed25519 AAAA...","usage_type":"auth_and_signing","created_at":"2026-01-01T00:00:00Z"},
				{"id":2,"title":"Personal","key":"ssh-rsa AAAA...","usage_type":"auth","created_at":"2026-06-01T00:00:00Z"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListSSHKeys(context.Background(), client, ListSSHKeysInput{})
	if err != nil {
		t.Fatalf("ListSSHKeys() unexpected error: %v", err)
	}
	if len(out.Keys) != 2 {
		t.Fatalf("got %d keys, want 2", len(out.Keys))
	}
	if out.Keys[0].Title != "Work Laptop" {
		t.Errorf("Keys[0].Title = %q, want %q", out.Keys[0].Title, "Work Laptop")
	}
}

// TestListSSHKeys_APIError verifies ListSSHKeys when API error.
func TestListSSHKeys_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnauthorized, `{"message":"401 Unauthorized"}`)
	}))

	_, err := ListSSHKeys(context.Background(), client, ListSSHKeysInput{})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// List Emails.

// TestListEmails_Success verifies ListEmails when success.
func TestListEmails_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathListEmails {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":1,"email":"primary@example.com","confirmed_at":"2026-01-01T00:00:00Z"},
				{"id":2,"email":"secondary@example.com"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListEmails(context.Background(), client, ListEmailsInput{})
	if err != nil {
		t.Fatalf("ListEmails() unexpected error: %v", err)
	}
	if len(out.Emails) != 2 {
		t.Fatalf("got %d emails, want 2", len(out.Emails))
	}
	if out.Emails[0].Email != "primary@example.com" {
		t.Errorf("Emails[0].Email = %q, want %q", out.Emails[0].Email, "primary@example.com")
	}
	if out.Emails[1].ConfirmedAt != "" {
		t.Errorf("Emails[1].ConfirmedAt = %q, want empty", out.Emails[1].ConfirmedAt)
	}
}

// TestListEmails_APIError verifies ListEmails when API error.
func TestListEmails_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnauthorized, `{"message":"401 Unauthorized"}`)
	}))

	_, err := ListEmails(context.Background(), client, ListEmailsInput{})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// Contribution Events.

// TestListContributionEvents_Success verifies ListContributionEvents when success.
func TestListContributionEvents_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathContribEvents {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":100,"project_id":10,"action_name":"pushed","target_type":"Project","target_title":"main","created_at":"2026-06-01T12:00:00Z"},
				{"id":101,"project_id":10,"action_name":"commented","target_type":"Issue","target_title":"Fix bug","created_at":"2026-06-02T14:00:00Z"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListContributionEvents(context.Background(), client, ListContributionEventsInput{UserID: 42})
	if err != nil {
		t.Fatalf("ListContributionEvents() unexpected error: %v", err)
	}
	if len(out.Events) != 2 {
		t.Fatalf("got %d events, want 2", len(out.Events))
	}
	if out.Events[0].ActionName != "pushed" {
		t.Errorf("Events[0].ActionName = %q, want %q", out.Events[0].ActionName, "pushed")
	}
}

// TestListContributionEvents_Validation verifies ListContributionEvents when validation.
func TestListContributionEvents_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := ListContributionEvents(context.Background(), client, ListContributionEventsInput{})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestListContributionEvents_APIError verifies ListContributionEvents when API error.
func TestListContributionEvents_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := ListContributionEvents(context.Background(), client, ListContributionEventsInput{UserID: 999})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// Associations Count.

// TestGetAssociationsCount_Success verifies GetAssociationsCount when success.
func TestGetAssociationsCount_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathAssociationsCount {
			testutil.RespondJSON(w, http.StatusOK, `{
				"groups_count":5,"projects_count":12,"issues_count":45,"merge_requests_count":30
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetAssociationsCount(context.Background(), client, GetAssociationsCountInput{UserID: 42})
	if err != nil {
		t.Fatalf("GetAssociationsCount() unexpected error: %v", err)
	}
	if out.GroupsCount != 5 {
		t.Errorf("out.GroupsCount = %d, want 5", out.GroupsCount)
	}
	if out.ProjectsCount != 12 {
		t.Errorf("out.ProjectsCount = %d, want 12", out.ProjectsCount)
	}
	if out.IssuesCount != 45 {
		t.Errorf("out.IssuesCount = %d, want 45", out.IssuesCount)
	}
	if out.MergeRequestsCount != 30 {
		t.Errorf("out.MergeRequestsCount = %d, want 30", out.MergeRequestsCount)
	}
}

// TestGetAssociationsCount_Validation verifies GetAssociationsCount when validation.
func TestGetAssociationsCount_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := GetAssociationsCount(context.Background(), client, GetAssociationsCountInput{})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestGetAssociationsCount_APIError verifies GetAssociationsCount when API error.
func TestGetAssociationsCount_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := GetAssociationsCount(context.Background(), client, GetAssociationsCountInput{UserID: 42})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpNonNilResult identifies the err exp non nil result constant used by this package.
const errExpNonNilResult = "expected non-nil result"

// errExpCancelledNil identifies the err exp cancelled nil constant used by this package.
const errExpCancelledNil = "expected error for canceled context, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// Current — canceled context (already in users_test.go), extra field coverage
// ---------------------------------------------------------------------------.

// TestCurrent_FullFields verifies Current when full fields.
func TestCurrent_FullFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/user" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":1,
				"username":"admin",
				"email":"admin@example.com",
				"name":"Admin User",
				"state":"active",
				"web_url":"https://gitlab.example.com/admin",
				"avatar_url":"https://gitlab.example.com/avatar.png",
				"is_admin":true,
				"bot":false,
				"bio":"Site admin",
				"location":"NYC",
				"job_title":"SRE",
				"organization":"ACME",
				"public_email":"pub@example.com",
				"website_url":"https://example.com",
				"two_factor_enabled":true,
				"external":false,
				"locked":false,
				"private_profile":true,
				"projects_limit":100,
				"can_create_project":true,
				"can_create_group":true,
				"note":"VIP",
				"using_license_seat":true,
				"theme_id":2,
				"color_scheme_id":3,
				"created_at":"2026-01-01T00:00:00Z",
				"last_activity_on":"2026-06-15",
				"current_sign_in_at":"2026-06-15T10:00:00Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Current(context.Background(), client, CurrentInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	assertCurrentUserProfileFields(t, out)
	assertCurrentUserAccessFields(t, out)
	assertCurrentUserAuditFields(t, out)
}

func assertCurrentUserProfileFields(t *testing.T, out Output) {
	t.Helper()
	if !out.IsAdmin {
		t.Error("expected IsAdmin = true")
	}
	if out.Location != "NYC" {
		t.Errorf("Location = %q, want %q", out.Location, "NYC")
	}
	if out.JobTitle != "SRE" {
		t.Errorf("JobTitle = %q, want %q", out.JobTitle, "SRE")
	}
	if out.Organization != "ACME" {
		t.Errorf("Organization = %q, want %q", out.Organization, "ACME")
	}
	if out.PublicEmail != "pub@example.com" {
		t.Errorf("PublicEmail = %q, want %q", out.PublicEmail, "pub@example.com")
	}
	if out.Note != "VIP" {
		t.Errorf("Note = %q, want %q", out.Note, "VIP")
	}
}

func assertCurrentUserAccessFields(t *testing.T, out Output) {
	t.Helper()
	if !out.TwoFactorEnabled {
		t.Error("expected TwoFactorEnabled = true")
	}
	if !out.PrivateProfile {
		t.Error("expected PrivateProfile = true")
	}
	if out.ProjectsLimit != 100 {
		t.Errorf("ProjectsLimit = %d, want 100", out.ProjectsLimit)
	}
	if !out.CanCreateProject {
		t.Error("expected CanCreateProject = true")
	}
	if !out.CanCreateGroup {
		t.Error("expected CanCreateGroup = true")
	}
	if !out.UsingLicenseSeat {
		t.Error("expected UsingLicenseSeat = true")
	}
}

func assertCurrentUserAuditFields(t *testing.T, out Output) {
	t.Helper()
	if out.ThemeID != 2 {
		t.Errorf("ThemeID = %d, want 2", out.ThemeID)
	}
	if out.ColorSchemeID != 3 {
		t.Errorf("ColorSchemeID = %d, want 3", out.ColorSchemeID)
	}
	if out.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
	if out.LastActivityOn == "" {
		t.Error("expected non-empty LastActivityOn")
	}
	if out.CurrentSignInAt == "" {
		t.Error("expected non-empty CurrentSignInAt")
	}
}

// ---------------------------------------------------------------------------
// List — canceled context, pagination, all optional filters
// ---------------------------------------------------------------------------.

// TestList_UsersCancelledContext verifies List when users cancelled context.
func TestList_UsersCancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := List(ctx, client, ListInput{})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestList_UsersWithPagination verifies List when users with pagination.
func TestList_UsersWithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/users" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":1,"username":"alice","name":"Alice","email":"alice@example.com","state":"active"}
			]`, testutil.PaginationHeaders{
				Page: "1", PerPage: "20", Total: "50", TotalPages: "3", NextPage: "2",
			})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Users) != 1 {
		t.Fatalf("got %d users, want 1", len(out.Users))
	}
	if out.Pagination.TotalItems != 50 {
		t.Errorf("Pagination.TotalItems = %d, want 50", out.Pagination.TotalItems)
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("Pagination.TotalPages = %d, want 3", out.Pagination.TotalPages)
	}
	if out.Pagination.NextPage != 2 {
		t.Errorf("Pagination.NextPage = %d, want 2", out.Pagination.NextPage)
	}
}

// TestList_UsersAllOptionalFilters verifies List when users all optional filters.
func TestList_UsersAllOptionalFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/users" {
			q := r.URL.Query()
			if q.Get("username") != "bob" {
				t.Errorf("username filter = %q, want %q", q.Get("username"), "bob")
			}
			if q.Get("order_by") != "created_at" {
				t.Errorf("order_by filter = %q, want %q", q.Get("order_by"), "created_at")
			}
			if q.Get("sort") != "desc" {
				t.Errorf("sort filter = %q, want %q", q.Get("sort"), "desc")
			}
			testutil.RespondJSON(w, http.StatusOK, `[{"id":2,"username":"bob","name":"Bob","email":"bob@example.com","state":"active"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	active := true
	blocked := false
	external := false
	out, err := List(context.Background(), client, ListInput{
		Username: "bob",
		Active:   &active,
		Blocked:  &blocked,
		External: &external,
		OrderBy:  "created_at",
		Sort:     "desc",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Users) != 1 {
		t.Fatalf("got %d users, want 1", len(out.Users))
	}
	if out.Users[0].Username != "bob" {
		t.Errorf("Username = %q, want %q", out.Users[0].Username, "bob")
	}
}

// TestList_UsersEmptyResult verifies List when users empty result.
func TestList_UsersEmptyResult(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/users" {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{Search: "nonexistent"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Users) != 0 {
		t.Fatalf("got %d users, want 0", len(out.Users))
	}
}

// ---------------------------------------------------------------------------
// Get — canceled context
// ---------------------------------------------------------------------------.

// TestGet_UserCancelledContext verifies Get when user cancelled context.
func TestGet_UserCancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Get(ctx, client, GetInput{UserID: 42})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// GetStatus — API error, canceled context
// ---------------------------------------------------------------------------.

// TestGet_UserStatusAPIError verifies Get when user status API error.
func TestGet_UserStatusAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := GetStatus(context.Background(), client, GetStatusInput{UserID: 42})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// TestGet_UserStatusCancelledContext verifies Get when user status cancelled context.
func TestGet_UserStatusCancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := GetStatus(ctx, client, GetStatusInput{UserID: 42})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestGet_UserStatusWithClearAt verifies Get when user status with clear at.
func TestGet_UserStatusWithClearAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/users/42/status" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"emoji":"palm_tree",
				"message":"On vacation",
				"availability":"not_set",
				"message_html":"<p>On vacation</p>",
				"clear_status_at":"2026-12-31T23:59:59Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetStatus(context.Background(), client, GetStatusInput{UserID: 42})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ClearStatusAt == "" {
		t.Error("expected non-empty ClearStatusAt")
	}
	if out.MessageHTML != "<p>On vacation</p>" {
		t.Errorf("MessageHTML = %q, want %q", out.MessageHTML, "<p>On vacation</p>")
	}
}

// ---------------------------------------------------------------------------
// SetStatus — canceled context, with ClearStatusAfter
// ---------------------------------------------------------------------------.

// TestSetUserStatus_CancelledContext verifies SetUserStatus when cancelled context.
func TestSetUserStatus_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := SetStatus(ctx, client, SetStatusInput{Emoji: "coffee"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestSetUserStatus_WithClearAfter verifies SetUserStatus when with clear after.
func TestSetUserStatus_WithClearAfter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/user/status" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"emoji":"coffee",
				"message":"BRB",
				"availability":"busy",
				"clear_status_at":"2026-06-15T18:00:00Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := SetStatus(context.Background(), client, SetStatusInput{
		Emoji:            "coffee",
		Message:          "BRB",
		Availability:     "busy",
		ClearStatusAfter: "3_hours",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ClearStatusAt == "" {
		t.Error("expected non-empty ClearStatusAt")
	}
}

// TestSetUserStatus_EmptyInput verifies SetUserStatus when empty input.
func TestSetUserStatus_EmptyInput(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/user/status" {
			testutil.RespondJSON(w, http.StatusOK, `{"emoji":"","message":"","availability":"not_set"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := SetStatus(context.Background(), client, SetStatusInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Emoji != "" {
		t.Errorf("Emoji = %q, want empty", out.Emoji)
	}
}

// ---------------------------------------------------------------------------
// ListSSHKeys — canceled context, pagination, empty result
// ---------------------------------------------------------------------------.

// TestListSSHKeys_CancelledContext verifies ListSSHKeys when cancelled context.
func TestListSSHKeys_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := ListSSHKeys(ctx, client, ListSSHKeysInput{})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestListSSHKeys_WithPagination verifies ListSSHKeys when with pagination.
func TestListSSHKeys_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/user/keys" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":1,"title":"Key1","key":"ssh-ed25519 AAAA...","usage_type":"auth","created_at":"2026-01-01T00:00:00Z","expires_at":"2026-01-01T00:00:00Z"}
			]`, testutil.PaginationHeaders{
				Page: "1", PerPage: "20", Total: "5", TotalPages: "1",
			})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListSSHKeys(context.Background(), client, ListSSHKeysInput{Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Keys) != 1 {
		t.Fatalf("got %d keys, want 1", len(out.Keys))
	}
	if out.Keys[0].ExpiresAt == "" {
		t.Error("expected non-empty ExpiresAt")
	}
	if out.Pagination.TotalItems != 5 {
		t.Errorf("TotalItems = %d, want 5", out.Pagination.TotalItems)
	}
}

// TestListSSHKeys_Empty verifies ListSSHKeys when empty.
func TestListSSHKeys_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/user/keys" {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListSSHKeys(context.Background(), client, ListSSHKeysInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Keys) != 0 {
		t.Fatalf("got %d keys, want 0", len(out.Keys))
	}
}

// ---------------------------------------------------------------------------
// ListEmails — canceled context, empty result
// ---------------------------------------------------------------------------.

// TestListEmails_CancelledContext verifies ListEmails when cancelled context.
func TestListEmails_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := ListEmails(ctx, client, ListEmailsInput{})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestListEmails_Empty verifies ListEmails when empty.
func TestListEmails_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/user/emails" {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListEmails(context.Background(), client, ListEmailsInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Emails) != 0 {
		t.Fatalf("got %d emails, want 0", len(out.Emails))
	}
}

// ---------------------------------------------------------------------------
// ListContributionEvents — canceled context, all optional filters
// ---------------------------------------------------------------------------.

// TestListContributionEvents_CancelledContext verifies ListContributionEvents when cancelled context.
func TestListContributionEvents_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := ListContributionEvents(ctx, client, ListContributionEventsInput{UserID: 42})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestListContributionEvents_AllFilters verifies ListContributionEvents when all filters.
func TestListContributionEvents_AllFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/users/42/events" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":200,"project_id":5,"action_name":"created","target_type":"Issue","target_title":"New feature","target_id":10,"target_iid":1,"created_at":"2026-03-15T09:00:00Z"}
			]`, testutil.PaginationHeaders{
				Page: "1", PerPage: "20", Total: "1", TotalPages: "1",
			})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListContributionEvents(context.Background(), client, ListContributionEventsInput{
		UserID:     42,
		Action:     "created",
		TargetType: "Issue",
		Before:     "2026-12-31",
		After:      "2026-01-01",
		Sort:       "desc",
		Page:       1,
		PerPage:    20,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Events) != 1 {
		t.Fatalf("got %d events, want 1", len(out.Events))
	}
	if out.Events[0].ActionName != "created" {
		t.Errorf("ActionName = %q, want %q", out.Events[0].ActionName, "created")
	}
	if out.Events[0].TargetID != 10 {
		t.Errorf("TargetID = %d, want 10", out.Events[0].TargetID)
	}
	if out.Events[0].TargetIID != 1 {
		t.Errorf("TargetIID = %d, want 1", out.Events[0].TargetIID)
	}
}

// TestListContributionEvents_InvalidDateIgnored verifies ListContributionEvents when invalid date ignored.
func TestListContributionEvents_InvalidDateIgnored(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/users/42/events" {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListContributionEvents(context.Background(), client, ListContributionEventsInput{
		UserID: 42,
		Before: "not-a-date",
		After:  "also-invalid",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Events) != 0 {
		t.Fatalf("got %d events, want 0", len(out.Events))
	}
}

// TestListContributionEvents_Empty verifies ListContributionEvents when empty.
func TestListContributionEvents_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/users/42/events" {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListContributionEvents(context.Background(), client, ListContributionEventsInput{UserID: 42})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Events) != 0 {
		t.Fatalf("got %d events, want 0", len(out.Events))
	}
}

// ---------------------------------------------------------------------------
// GetAssociationsCount — canceled context
// ---------------------------------------------------------------------------.

// TestGetAssociationsCount_CancelledContext verifies GetAssociationsCount when cancelled context.
func TestGetAssociationsCount_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := GetAssociationsCount(ctx, client, GetAssociationsCountInput{UserID: 42})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// FormatMarkdownString — with data, with bio/avatar
// ---------------------------------------------------------------------------.

// TestFormatMarkdownString_WithData verifies FormatMarkdownString when with data.
func TestFormatMarkdownString_WithData(t *testing.T) {
	out := Output{
		ID:        1,
		Username:  "alice",
		Email:     "alice@example.com",
		Name:      "Alice Smith",
		State:     "active",
		WebURL:    "https://gitlab.example.com/alice",
		AvatarURL: "https://gitlab.example.com/alice/avatar.png",
		IsAdmin:   true,
		Bio:       "Go developer",
		SCIMIdentities: []SCIMIdentityOutput{{
			ExternUID: "scim-alice",
			GroupID:   9,
			Active:    true,
		}},
	}
	md := FormatMarkdownString(out)

	for _, want := range []string{
		"## GitLab User: Alice Smith",
		"**Username**: alice",
		"**Email**: alice@example.com",
		"**State**: active",
		"**Bio**: Go developer",
		"**Admin**: true",
		"**Avatar**: https://gitlab.example.com/alice/avatar.png",
		"### SCIM Identities",
		"| scim-alice | 9 | true |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatMarkdownString_Empty verifies FormatMarkdownString when empty.
func TestFormatMarkdownString_Empty(t *testing.T) {
	md := FormatMarkdownString(Output{})
	if !strings.Contains(md, "## GitLab User:") {
		t.Errorf("expected header in empty output:\n%s", md)
	}
	if strings.Contains(md, "**Bio**") {
		t.Error("should not contain Bio when empty")
	}
	if strings.Contains(md, "**Avatar**") {
		t.Error("should not contain Avatar when empty")
	}
}

// TestFormatMarkdown_ReturnsMCPResult verifies FormatMarkdown returns MCP result.
func TestFormatMarkdown_ReturnsMCPResult(t *testing.T) {
	result := FormatMarkdown(Output{ID: 1, Name: "Test"})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	if len(result.Content) == 0 {
		t.Error("expected non-empty content")
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdownString — with data, empty
// ---------------------------------------------------------------------------.

// TestFormatListMarkdownString_WithData verifies FormatListMarkdownString when with data.
func TestFormatListMarkdownString_WithData(t *testing.T) {
	out := ListOutput{
		Users: []Output{
			{ID: 1, Username: "alice", Name: "Alice", Email: "alice@example.com", State: "active", WebURL: "https://gitlab.example.com/alice"},
			{ID: 2, Username: "bob", Name: "Bob", Email: "bob@example.com", State: "blocked", WebURL: "https://gitlab.example.com/bob"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdownString(out)

	for _, want := range []string{
		"## GitLab Users (2)",
		"| ID | Username | Name | Email | State |",
		"| 1 | [@alice](https://gitlab.example.com/alice) | Alice | alice@example.com | active |",
		"| 2 | [@bob](https://gitlab.example.com/bob) | Bob | bob@example.com | blocked |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatListMarkdownString_Empty verifies FormatListMarkdownString when empty.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	md := FormatListMarkdownString(ListOutput{})
	if !strings.Contains(md, "No users found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// TestFormatListMarkdown_ReturnsMCPResult verifies FormatListMarkdown returns MCP result.
func TestFormatListMarkdown_ReturnsMCPResult(t *testing.T) {
	result := FormatListMarkdown(ListOutput{Users: []Output{{ID: 1}}})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	if len(result.Content) == 0 {
		t.Error("expected non-empty content")
	}
}

// ---------------------------------------------------------------------------
// FormatStatusMarkdownString — with data, empty, partial
// ---------------------------------------------------------------------------.

// TestFormatStatusMarkdownString_WithData verifies FormatStatusMarkdownString when with data.
func TestFormatStatusMarkdownString_WithData(t *testing.T) {
	out := StatusOutput{
		Emoji:         "coffee",
		Message:       "Taking a break",
		Availability:  "busy",
		ClearStatusAt: "2026-12-31T23:59:59Z",
	}
	md := FormatStatusMarkdownString(out)

	for _, want := range []string{
		"## User Status",
		"**Emoji**: coffee",
		"**Message**: Taking a break",
		"**Availability**: busy",
		"**Clear At**: 31 Dec 2026 23:59 UTC",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatStatusMarkdownString_Empty verifies FormatStatusMarkdownString when empty.
func TestFormatStatusMarkdownString_Empty(t *testing.T) {
	md := FormatStatusMarkdownString(StatusOutput{})
	if !strings.Contains(md, "## User Status") {
		t.Errorf("expected header:\n%s", md)
	}
	for _, absent := range []string{"**Emoji**", "**Message**", "**Availability**", "**Clear At**"} {
		if strings.Contains(md, absent) {
			t.Errorf("should not contain %q when empty:\n%s", absent, md)
		}
	}
}

// TestFormatStatusMarkdownString_Partial verifies FormatStatusMarkdownString when partial.
func TestFormatStatusMarkdownString_Partial(t *testing.T) {
	md := FormatStatusMarkdownString(StatusOutput{Emoji: "fire"})
	if !strings.Contains(md, "**Emoji**: fire") {
		t.Errorf("missing emoji:\n%s", md)
	}
	if strings.Contains(md, "**Message**") {
		t.Error("should not contain Message when empty")
	}
}

// TestFormatStatusMarkdown_ReturnsMCPResult verifies FormatStatusMarkdown returns MCP result.
func TestFormatStatusMarkdown_ReturnsMCPResult(t *testing.T) {
	result := FormatStatusMarkdown(StatusOutput{Emoji: "wave"})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// ---------------------------------------------------------------------------
// FormatSSHKeyListMarkdownString — with data, empty
// ---------------------------------------------------------------------------.

// TestFormatSSHKeyListMarkdownString_WithData verifies FormatSSHKeyListMarkdownString when with data.
func TestFormatSSHKeyListMarkdownString_WithData(t *testing.T) {
	out := SSHKeyListOutput{
		Keys: []SSHKeyOutput{
			{ID: 1, Title: "Work Laptop", UsageType: "auth", CreatedAt: "2026-01-01T00:00:00Z", ExpiresAt: "2026-01-01T00:00:00Z"},
			{ID: 2, Title: "Personal", UsageType: "auth_and_signing", CreatedAt: "2026-06-01T00:00:00Z"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatSSHKeyListMarkdownString(out)

	for _, want := range []string{
		"## SSH Keys (2)",
		"| ID | Title | Usage Type | Created At | Expires At |",
		"| 1 | Work Laptop |",
		"| 2 | Personal |",
		"auth_and_signing",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatSSHKeyListMarkdownString_Empty verifies FormatSSHKeyListMarkdownString when empty.
func TestFormatSSHKeyListMarkdownString_Empty(t *testing.T) {
	md := FormatSSHKeyListMarkdownString(SSHKeyListOutput{})
	if !strings.Contains(md, "No SSH keys found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// TestFormatSSHKeyListMarkdown_ReturnsMCPResult verifies FormatSSHKeyListMarkdown returns MCP result.
func TestFormatSSHKeyListMarkdown_ReturnsMCPResult(t *testing.T) {
	result := FormatSSHKeyListMarkdown(SSHKeyListOutput{Keys: []SSHKeyOutput{{ID: 1, Title: "k"}}})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// ---------------------------------------------------------------------------
// FormatEmailListMarkdownString — with data, empty
// ---------------------------------------------------------------------------.

// TestFormatEmailListMarkdownString_WithData verifies FormatEmailListMarkdownString when with data.
func TestFormatEmailListMarkdownString_WithData(t *testing.T) {
	out := EmailListOutput{
		Emails: []EmailOutput{
			{ID: 1, Email: "primary@example.com", ConfirmedAt: "2026-01-01T00:00:00Z"},
			{ID: 2, Email: "alias@example.com"},
		},
	}
	md := FormatEmailListMarkdownString(out)

	for _, want := range []string{
		"## Email Addresses (2)",
		"| ID | Email | Confirmed At |",
		"| 1 | primary@example.com |",
		"| 2 | alias@example.com |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatEmailListMarkdownString_Empty verifies FormatEmailListMarkdownString when empty.
func TestFormatEmailListMarkdownString_Empty(t *testing.T) {
	md := FormatEmailListMarkdownString(EmailListOutput{})
	if !strings.Contains(md, "No email addresses found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// TestFormatEmailListMarkdown_ReturnsMCPResult verifies FormatEmailListMarkdown returns MCP result.
func TestFormatEmailListMarkdown_ReturnsMCPResult(t *testing.T) {
	result := FormatEmailListMarkdown(EmailListOutput{Emails: []EmailOutput{{ID: 1, Email: "a@b.com"}}})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// ---------------------------------------------------------------------------
// FormatContributionEventsMarkdownString — with data, empty
// ---------------------------------------------------------------------------.

// TestFormatContributionEventsMarkdownString_WithData verifies FormatContributionEventsMarkdownString when with data.
func TestFormatContributionEventsMarkdownString_WithData(t *testing.T) {
	out := ContributionEventsOutput{
		Events: []ContributionEventOutput{
			{ID: 100, ActionName: "pushed", TargetType: "Project", TargetTitle: "main", CreatedAt: "2026-06-01T12:00:00Z"},
			{ID: 101, ActionName: "commented", TargetType: "Issue", TargetTitle: "Fix bug", CreatedAt: "2026-06-02T14:00:00Z"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatContributionEventsMarkdownString(out)

	for _, want := range []string{
		"## Contribution Events (2)",
		"| ID | Action | Target Type | Target | Created At |",
		"| 100 | pushed | Project | main |",
		"| 101 | commented | Issue | Fix bug |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatContributionEventsMarkdownString_Empty verifies FormatContributionEventsMarkdownString when empty.
func TestFormatContributionEventsMarkdownString_Empty(t *testing.T) {
	md := FormatContributionEventsMarkdownString(ContributionEventsOutput{})
	if !strings.Contains(md, "No contribution events found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// TestFormatContributionEventsMarkdown_ReturnsMCPResult verifies FormatContributionEventsMarkdown returns MCP result.
func TestFormatContributionEventsMarkdown_ReturnsMCPResult(t *testing.T) {
	result := FormatContributionEventsMarkdown(ContributionEventsOutput{
		Events: []ContributionEventOutput{{ID: 1, ActionName: "pushed"}},
	})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// ---------------------------------------------------------------------------
// FormatAssociationsCountMarkdownString — with data, zero values
// ---------------------------------------------------------------------------.

// TestFormatAssociationsCountMarkdownString_WithData verifies FormatAssociationsCountMarkdownString when with data.
func TestFormatAssociationsCountMarkdownString_WithData(t *testing.T) {
	out := AssociationsCountOutput{
		GroupsCount:        5,
		ProjectsCount:      12,
		IssuesCount:        45,
		MergeRequestsCount: 30,
	}
	md := FormatAssociationsCountMarkdownString(out)

	for _, want := range []string{
		"## User Associations Count",
		"**Groups**: 5",
		"**Projects**: 12",
		"**Issues**: 45",
		"**Merge Requests**: 30",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatAssociationsCountMarkdownString_Zero verifies FormatAssociationsCountMarkdownString when zero.
func TestFormatAssociationsCountMarkdownString_Zero(t *testing.T) {
	md := FormatAssociationsCountMarkdownString(AssociationsCountOutput{})
	if !strings.Contains(md, "**Groups**: 0") {
		t.Errorf("expected Groups: 0:\n%s", md)
	}
	if !strings.Contains(md, "**Projects**: 0") {
		t.Errorf("expected Projects: 0:\n%s", md)
	}
}

// TestFormatAssociationsCountMarkdown_ReturnsMCPResult verifies FormatAssociationsCountMarkdown returns MCP result.
func TestFormatAssociationsCountMarkdown_ReturnsMCPResult(t *testing.T) {
	result := FormatAssociationsCountMarkdown(AssociationsCountOutput{GroupsCount: 1})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// TestGetStatus_NilResponse verifies that GetStatus handles a null JSON body
// from the GitLab API, covering the if-s==nil branch.
func TestGetStatus_NilResponse(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGetUserStatus {
			testutil.RespondJSON(w, http.StatusOK, `null`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetStatus(context.Background(), client, GetStatusInput{UserID: 42})
	if err != nil {
		t.Fatalf("expected no error for null, got: %v", err)
	}
	if out.Emoji != "" || out.Message != "" {
		t.Errorf("expected empty status for null response, got emoji=%q message=%q", out.Emoji, out.Message)
	}
}

// TestSetStatus_NilResponse verifies that SetStatus handles a null JSON body
// from the GitLab API, covering the if-s==nil branch.
func TestSetStatus_NilResponse(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/user/status" {
			testutil.RespondJSON(w, http.StatusOK, `null`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := SetStatus(context.Background(), client, SetStatusInput{Emoji: "coffee"})
	if err != nil {
		t.Fatalf("expected no error for null, got: %v", err)
	}
	if out.Emoji != "" || out.Message != "" {
		t.Errorf("expected empty status for null response, got emoji=%q message=%q", out.Emoji, out.Message)
	}
}

// TestResolveProjectWebURLs_Success verifies that resolveProjectWebURLs populates
// the map with project WebURLs for valid IDs, covering the success branch.
func TestResolveProjectWebURLs_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/10" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":10,"web_url":"https://gitlab.example.com/group/project"}`)
			return
		}
		http.NotFound(w, r)
	}))

	urls := resolveProjectWebURLs(context.Background(), client, []int64{10})
	if got := urls[10]; got != "https://gitlab.example.com/group/project" {
		t.Errorf("urls[10] = %q, want %q", got, "https://gitlab.example.com/group/project")
	}
}
