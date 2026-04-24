// users_test.go contains unit tests for GitLab user operations.

package users

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	errExpAPIFailure      = "expected error for API failure, got nil"
	pathCurrentUser       = "/api/v4/user"
	pathListUsers         = "/api/v4/users"
	pathGetUser           = "/api/v4/users/42"
	pathGetUserStatus     = "/api/v4/users/42/status"
	pathSetUserStatus     = "/api/v4/user/status"
	pathListSSHKeys       = "/api/v4/user/keys"
	pathListEmails        = "/api/v4/user/emails"
	pathContribEvents     = "/api/v4/users/42/events"
	pathAssociationsCount = "/api/v4/users/42/associations_count"
)

// TestCurrent_Success verifies the behavior of current success.
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

// TestCurrent_APIError verifies the behavior of current a p i error.
func TestCurrent_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnauthorized, `{"message":"401 Unauthorized"}`)
	}))

	_, err := Current(context.Background(), client, CurrentInput{})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestCurrent_CancelledContext verifies the behavior of current cancelled context.
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

// TestList_UsersSuccess verifies the behavior of list users success.
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

// TestList_UsersAPIError verifies the behavior of list users a p i error.
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

// TestGet_UserSuccess verifies the behavior of get user success.
func TestGet_UserSuccess(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGetUser {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":42,"username":"testuser","name":"Test User","email":"test@example.com","state":"active","bio":"Developer"
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
}

// TestGet_UserValidation verifies the behavior of get user validation.
func TestGet_UserValidation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Get(context.Background(), client, GetInput{})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestGet_UserAPIError verifies the behavior of get user a p i error.
func TestGet_UserAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := Get(context.Background(), client, GetInput{UserID: 999})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// Get User Status.

// TestGet_UserStatusSuccess verifies the behavior of get user status success.
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

// TestGet_UserStatusValidation verifies the behavior of get user status validation.
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

// TestSetUserStatus_Success verifies the behavior of set user status success.
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

// TestSetUserStatus_APIError verifies the behavior of set user status a p i error.
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

// TestListSSHKeys_Success verifies the behavior of list s s h keys success.
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

// TestListSSHKeys_APIError verifies the behavior of list s s h keys a p i error.
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

// TestListEmails_Success verifies the behavior of list emails success.
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

// TestListEmails_APIError verifies the behavior of list emails a p i error.
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

// TestListContributionEvents_Success verifies the behavior of list contribution events success.
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

// TestListContributionEvents_Validation verifies the behavior of list contribution events validation.
func TestListContributionEvents_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := ListContributionEvents(context.Background(), client, ListContributionEventsInput{})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestListContributionEvents_APIError verifies the behavior of list contribution events a p i error.
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

// TestGetAssociationsCount_Success verifies the behavior of get associations count success.
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

// TestGetAssociationsCount_Validation verifies the behavior of get associations count validation.
func TestGetAssociationsCount_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := GetAssociationsCount(context.Background(), client, GetAssociationsCountInput{})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestGetAssociationsCount_APIError verifies the behavior of get associations count a p i error.
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

const errExpNonNilResult = "expected non-nil result"

const errExpCancelledNil = "expected error for canceled context, got nil"

const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// Current — canceled context (already in users_test.go), extra field coverage
// ---------------------------------------------------------------------------.

// TestCurrent_FullFields verifies the behavior of current full fields.
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
	if out.Note != "VIP" {
		t.Errorf("Note = %q, want %q", out.Note, "VIP")
	}
	if !out.UsingLicenseSeat {
		t.Error("expected UsingLicenseSeat = true")
	}
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

// TestList_UsersCancelledContext verifies the behavior of list users cancelled context.
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

// TestList_UsersWithPagination verifies the behavior of list users with pagination.
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

// TestList_UsersAllOptionalFilters verifies the behavior of list users all optional filters.
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

// TestList_UsersEmptyResult verifies the behavior of list users empty result.
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

// TestGet_UserCancelledContext verifies the behavior of get user cancelled context.
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

// TestGet_UserStatusAPIError verifies the behavior of get user status a p i error.
func TestGet_UserStatusAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := GetStatus(context.Background(), client, GetStatusInput{UserID: 42})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// TestGet_UserStatusCancelledContext verifies the behavior of get user status cancelled context.
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

// TestGet_UserStatusWithClearAt verifies the behavior of get user status with clear at.
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

// TestSetUserStatus_CancelledContext verifies the behavior of set user status cancelled context.
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

// TestSetUserStatus_WithClearAfter verifies the behavior of set user status with clear after.
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

// TestSetUserStatus_EmptyInput verifies the behavior of set user status empty input.
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

// TestListSSHKeys_CancelledContext verifies the behavior of list s s h keys cancelled context.
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

// TestListSSHKeys_WithPagination verifies the behavior of list s s h keys with pagination.
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

// TestListSSHKeys_Empty verifies the behavior of list s s h keys empty.
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

// TestListEmails_CancelledContext verifies the behavior of list emails cancelled context.
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

// TestListEmails_Empty verifies the behavior of list emails empty.
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

// TestListContributionEvents_CancelledContext verifies the behavior of list contribution events cancelled context.
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

// TestListContributionEvents_AllFilters verifies the behavior of list contribution events all filters.
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

// TestListContributionEvents_InvalidDateIgnored verifies the behavior of list contribution events invalid date ignored.
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

// TestListContributionEvents_Empty verifies the behavior of list contribution events empty.
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

// TestGetAssociationsCount_CancelledContext verifies the behavior of get associations count cancelled context.
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

// TestFormatMarkdownString_WithData verifies the behavior of format markdown string with data.
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
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatMarkdownString_Empty verifies the behavior of format markdown string empty.
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

// TestFormatMarkdown_ReturnsMCPResult verifies the behavior of format markdown returns m c p result.
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

// TestFormatListMarkdownString_WithData verifies the behavior of format list markdown string with data.
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

// TestFormatListMarkdownString_Empty verifies the behavior of format list markdown string empty.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	md := FormatListMarkdownString(ListOutput{})
	if !strings.Contains(md, "No users found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// TestFormatListMarkdown_ReturnsMCPResult verifies the behavior of format list markdown returns m c p result.
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

// TestFormatStatusMarkdownString_WithData verifies the behavior of format status markdown string with data.
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

// TestFormatStatusMarkdownString_Empty verifies the behavior of format status markdown string empty.
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

// TestFormatStatusMarkdownString_Partial verifies the behavior of format status markdown string partial.
func TestFormatStatusMarkdownString_Partial(t *testing.T) {
	md := FormatStatusMarkdownString(StatusOutput{Emoji: "fire"})
	if !strings.Contains(md, "**Emoji**: fire") {
		t.Errorf("missing emoji:\n%s", md)
	}
	if strings.Contains(md, "**Message**") {
		t.Error("should not contain Message when empty")
	}
}

// TestFormatStatusMarkdown_ReturnsMCPResult verifies the behavior of format status markdown returns m c p result.
func TestFormatStatusMarkdown_ReturnsMCPResult(t *testing.T) {
	result := FormatStatusMarkdown(StatusOutput{Emoji: "wave"})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// ---------------------------------------------------------------------------
// FormatSSHKeyListMarkdownString — with data, empty
// ---------------------------------------------------------------------------.

// TestFormatSSHKeyListMarkdownString_WithData verifies the behavior of format s s h key list markdown string with data.
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

// TestFormatSSHKeyListMarkdownString_Empty verifies the behavior of format s s h key list markdown string empty.
func TestFormatSSHKeyListMarkdownString_Empty(t *testing.T) {
	md := FormatSSHKeyListMarkdownString(SSHKeyListOutput{})
	if !strings.Contains(md, "No SSH keys found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// TestFormatSSHKeyListMarkdown_ReturnsMCPResult verifies the behavior of format s s h key list markdown returns m c p result.
func TestFormatSSHKeyListMarkdown_ReturnsMCPResult(t *testing.T) {
	result := FormatSSHKeyListMarkdown(SSHKeyListOutput{Keys: []SSHKeyOutput{{ID: 1, Title: "k"}}})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// ---------------------------------------------------------------------------
// FormatEmailListMarkdownString — with data, empty
// ---------------------------------------------------------------------------.

// TestFormatEmailListMarkdownString_WithData verifies the behavior of format email list markdown string with data.
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

// TestFormatEmailListMarkdownString_Empty verifies the behavior of format email list markdown string empty.
func TestFormatEmailListMarkdownString_Empty(t *testing.T) {
	md := FormatEmailListMarkdownString(EmailListOutput{})
	if !strings.Contains(md, "No email addresses found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// TestFormatEmailListMarkdown_ReturnsMCPResult verifies the behavior of format email list markdown returns m c p result.
func TestFormatEmailListMarkdown_ReturnsMCPResult(t *testing.T) {
	result := FormatEmailListMarkdown(EmailListOutput{Emails: []EmailOutput{{ID: 1, Email: "a@b.com"}}})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// ---------------------------------------------------------------------------
// FormatContributionEventsMarkdownString — with data, empty
// ---------------------------------------------------------------------------.

// TestFormatContributionEventsMarkdownString_WithData verifies the behavior of format contribution events markdown string with data.
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

// TestFormatContributionEventsMarkdownString_Empty verifies the behavior of format contribution events markdown string empty.
func TestFormatContributionEventsMarkdownString_Empty(t *testing.T) {
	md := FormatContributionEventsMarkdownString(ContributionEventsOutput{})
	if !strings.Contains(md, "No contribution events found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// TestFormatContributionEventsMarkdown_ReturnsMCPResult verifies the behavior of format contribution events markdown returns m c p result.
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

// TestFormatAssociationsCountMarkdownString_WithData verifies the behavior of format associations count markdown string with data.
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

// TestFormatAssociationsCountMarkdownString_Zero verifies the behavior of format associations count markdown string zero.
func TestFormatAssociationsCountMarkdownString_Zero(t *testing.T) {
	md := FormatAssociationsCountMarkdownString(AssociationsCountOutput{})
	if !strings.Contains(md, "**Groups**: 0") {
		t.Errorf("expected Groups: 0:\n%s", md)
	}
	if !strings.Contains(md, "**Projects**: 0") {
		t.Errorf("expected Projects: 0:\n%s", md)
	}
}

// TestFormatAssociationsCountMarkdown_ReturnsMCPResult verifies the behavior of format associations count markdown returns m c p result.
func TestFormatAssociationsCountMarkdown_ReturnsMCPResult(t *testing.T) {
	result := FormatAssociationsCountMarkdown(AssociationsCountOutput{GroupsCount: 1})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// RegisterToolsCallAllThroughMCP — full MCP roundtrip for all 9 tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newUsersMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"current_user", "gitlab_user_current", map[string]any{}},
		{"list_users", "gitlab_list_users", map[string]any{}},
		{"get_user", "gitlab_get_user", map[string]any{"user_id": 42}},
		{"get_user_status", "gitlab_get_user_status", map[string]any{"user_id": 42}},
		{"set_user_status", "gitlab_set_user_status", map[string]any{"emoji": "coffee", "message": "Working"}},
		{"list_ssh_keys", "gitlab_list_ssh_keys", map[string]any{}},
		{"list_emails", "gitlab_list_emails", map[string]any{}},
		{"list_contribution_events", "gitlab_list_user_contribution_events", map[string]any{"user_id": 42}},
		{"get_associations_count", "gitlab_get_user_associations_count", map[string]any{"user_id": 42}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.tool, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.tool, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session factory
// ---------------------------------------------------------------------------.

// newUsersMCPSession is an internal helper for the users package.
func newUsersMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	userJSON := `{"id":42,"username":"testuser","email":"test@example.com","name":"Test User","state":"active","web_url":"https://gitlab.example.com/testuser","avatar_url":"https://gitlab.example.com/avatar.png","is_admin":false,"bio":"Developer"}`
	statusJSON := `{"emoji":"coffee","message":"Working","availability":"busy"}`

	handler := http.NewServeMux()

	// Current user
	handler.HandleFunc("GET /api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, userJSON)
	})

	// List users
	handler.HandleFunc("GET /api/v4/users", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+userJSON+`]`)
	})

	// Get user
	handler.HandleFunc("GET /api/v4/users/42", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, userJSON)
	})

	// Get user status
	handler.HandleFunc("GET /api/v4/users/42/status", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, statusJSON)
	})

	// Set user status
	handler.HandleFunc("PUT /api/v4/user/status", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, statusJSON)
	})

	// List SSH keys
	handler.HandleFunc("GET /api/v4/user/keys", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"title":"Work","key":"ssh-ed25519 AAAA...","usage_type":"auth","created_at":"2026-01-01T00:00:00Z"}]`)
	})

	// List emails
	handler.HandleFunc("GET /api/v4/user/emails", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"email":"test@example.com","confirmed_at":"2026-01-01T00:00:00Z"}]`)
	})

	// List contribution events
	handler.HandleFunc("GET /api/v4/users/42/events", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":100,"project_id":10,"action_name":"pushed","target_type":"Project","created_at":"2026-06-01T12:00:00Z"}]`)
	})

	// Get associations count
	handler.HandleFunc("GET /api/v4/users/42/associations_count", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"groups_count":5,"projects_count":12,"issues_count":45,"merge_requests_count":30}`)
	})

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
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

// TestRegisterTools_GetUser404 verifies the get_user handler returns a
// NotFoundResult when GitLab responds with 404, covering the register.go 404 branch.
func TestRegisterTools_GetUser404(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 User Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_get_user",
		Arguments: map[string]any{"user_id": float64(999)},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Error("expected IsError=true for 404 response")
	}
}
