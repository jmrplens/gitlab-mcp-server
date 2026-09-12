// users_test.go contains unit tests for GitLab user operations.
package users

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

// TestList_UsersPairsTheCapturedKeysByPosition verifies a page of users takes
// one captured extra per row in order, so the second user's profile keys are
// not read onto the first. GET /users presents UserBasic, which carries none of
// them, but the reader is the same one every route here uses and the pairing is
// what a list can get wrong.
func TestList_UsersPairsTheCapturedKeysByPosition(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathListUsers {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":1,"username":"alice","pronouns":"she/her","followers":5},
				{"id":2,"username":"bob"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Users) != 2 {
		t.Fatalf("got %d users, want 2", len(out.Users))
	}
	if out.Users[0].Pronouns != "she/her" || out.Users[0].Followers == nil || *out.Users[0].Followers != 5 {
		t.Errorf("Users[0] = %+v, want the first row's own keys", out.Users[0])
	}
	if out.Users[1].Pronouns != "" || out.Users[1].Followers != nil {
		t.Errorf("Users[1] = %+v, want the second row left empty", out.Users[1])
	}
}

// TestList_UsersACapturedFieldTheTypeCannotHold_IsReported verifies a list body
// that decodes for the SDK and not for the keys read beside it is the
// operation's error rather than a page of users with those keys empty.
func TestList_UsersACapturedFieldTheTypeCannotHold_IsReported(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"username":"alice","following":"not-a-number"}]`)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil || !strings.Contains(err.Error(), "decode the captured response") {
		t.Errorf("List() error = %v, want the capture's decode failure", err)
	}
}

// TestGet_UserReadsTheProfileKeysAndTheFollowCounts verifies GET /users/:id,
// which presents UserProfile, carries the rendered bio and the three follow
// counts to a caller allowed to read the profile, none of which client-go's
// User models.
func TestGet_UserReadsTheProfileKeysAndTheFollowCounts(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGetUser {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":42,"username":"testuser","bio_html":"<p>Developer</p>","discord":"jdoe#1",
				"github":"jdoe","work_information":"Org","pronouns":"she/her","local_time":"2:30 PM",
				"followers":12,"following":34,"is_followed":true
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{UserID: 42})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.BioHTML != "<p>Developer</p>" || out.Discord != "jdoe#1" || out.GitHub != "jdoe" {
		t.Errorf("out = %+v, want the rendered bio and the two account names", out)
	}
	if out.WorkInformation != "Org" || out.Pronouns != "she/her" || out.LocalTime != "2:30 PM" {
		t.Errorf("out = %+v, want the three profile keys", out)
	}
	if out.Followers == nil || *out.Followers != 12 || out.Following == nil || *out.Following != 34 {
		t.Errorf("counts = %v/%v, want 12 and 34", out.Followers, out.Following)
	}
	if out.IsFollowed == nil || !*out.IsFollowed {
		t.Errorf("IsFollowed = %v, want true", out.IsFollowed)
	}
}

// TestGet_UserOnAProfileTheCallerMayNotReadSendsNoCounts is the other side of
// the Ability.allowed?(:read_user_profile) condition: GitLab exposes none of
// the three, and the output must leave them absent rather than report a user
// with no followers.
func TestGet_UserOnAProfileTheCallerMayNotReadSendsNoCounts(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGetUser {
			testutil.RespondJSON(w, http.StatusOK, `{"id":42,"username":"testuser","pronouns":"she/her"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{UserID: 42})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.Followers != nil || out.Following != nil || out.IsFollowed != nil {
		t.Errorf("counts = %v/%v/%v, want all three absent", out.Followers, out.Following, out.IsFollowed)
	}
	if out.Pronouns != "she/her" {
		t.Errorf("Pronouns = %q, want the unconditional key still read", out.Pronouns)
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
		Page:       1, PerPage: 20,
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

// assertMarkdown compares a rendered result with the whole document it is
// meant to be. A substring assertion is what let a card open a table and then
// write list rows into it in two packages of this tree: every row the test
// named was present in the string and none of them rendered as a row, so the
// rule here is the whole document or nothing.
func assertMarkdown(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("markdown mismatch\n--- got ---\n%s\n--- want ---\n%s\n--- got (quoted) ---\n%q", got, want, got)
	}
}

// userCardHints is the guidance section the user card closes with.
const userCardHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'user.get_status' to check the user's current status\n" +
	"- Use action 'user.ssh_keys' to list the account's SSH keys\n"

// TestFormatMarkdownString_WithData verifies the whole user card: the identity
// rows, the three flags the card used to omit entirely (bot, external and the
// lock), the avatar as a link rather than escaped text, and the SCIM
// identities as the nested collection they are.
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

	assertMarkdown(t, FormatMarkdownString(out),
		"## GitLab User: Alice Smith\n\n"+
			"- **ID**: 1\n"+
			"- **Username**: alice\n"+
			"- **Email**: alice@example.com\n"+
			"- **State**: active\n"+
			"- **Bio**: Go developer\n"+
			"- **Admin**: ✅\n"+
			"- **Bot**: ❌\n"+
			"- **External**: ❌\n"+
			"- **URL**: [https://gitlab.example.com/alice](https://gitlab.example.com/alice)\n"+
			"- **Avatar**: [https://gitlab.example.com/alice/avatar.png](https://gitlab.example.com/alice/avatar.png)\n"+
			"\n### SCIM Identities\n\n"+
			"| Extern UID | Group ID | Active |\n"+
			"| --- | --- | --- |\n"+
			"| scim-alice | 9 | ✅ |\n"+
			userCardHints)
}

// TestFormatMarkdownString_Locked verifies that a locked account carries the
// warning sign rather than the tick BoolEmoji would give a true, which on
// "Locked" reads as success.
func TestFormatMarkdownString_Locked(t *testing.T) {
	assertMarkdown(t, FormatMarkdownString(Output{ID: 3, Username: "carol", Name: "Carol", Locked: true}),
		"## GitLab User: Carol\n\n"+
			"- **ID**: 3\n"+
			"- **Username**: carol\n"+
			"- **Admin**: ❌\n"+
			"- **Bot**: ❌\n"+
			"- **External**: ❌\n"+
			"- ⚠️ **Locked**\n"+
			userCardHints)
}

// TestFormatMarkdownString_AvatarOnly verifies the narrow answer the avatar
// upload gives on GitLab 19: an avatar URL and no identity at all. The user
// card used to print an ID of zero and an empty email for it, which reads as a
// user whose profile GitLab lost.
func TestFormatMarkdownString_AvatarOnly(t *testing.T) {
	out := Output{
		NextSteps: []string{"The avatar was updated; use gitlab_user_current to fetch the profile"},
		AvatarURL: "https://gitlab.example.com/uploads/avatar.png",
	}

	assertMarkdown(t, FormatMarkdownString(out),
		"## Avatar Updated\n\n"+
			"- **Avatar**: [https://gitlab.example.com/uploads/avatar.png](https://gitlab.example.com/uploads/avatar.png)\n"+
			"\n---\n💡 **Next steps:**\n"+
			"- The avatar was updated; use gitlab_user_current to fetch the profile\n")
}

// TestFormatMarkdownString_Empty verifies that an empty user writes no row
// that has no value: only the ID, which is an answer at zero, and the three
// flags GitLab always sends.
func TestFormatMarkdownString_Empty(t *testing.T) {
	assertMarkdown(t, FormatMarkdownString(Output{}),
		"## GitLab User: \n\n"+
			"- **ID**: 0\n"+
			"- **Admin**: ❌\n"+
			"- **Bot**: ❌\n"+
			"- **External**: ❌\n"+
			userCardHints)
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

// TestFormatListMarkdownString_WithData verifies the whole list render.
func TestFormatListMarkdownString_WithData(t *testing.T) {
	out := ListOutput{
		Users: []Output{
			{ID: 1, Username: "alice", Name: "Alice", Email: "alice@example.com", State: "active", WebURL: "https://gitlab.example.com/alice"},
			{ID: 2, Username: "bob", Name: "Bob", Email: "bob@example.com", State: "blocked", WebURL: "https://gitlab.example.com/bob"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}

	assertMarkdown(t, FormatListMarkdownString(out),
		"## GitLab Users (2)\n\n"+
			"| ID | Username | Name | Email | State |\n"+
			"| --- | --- | --- | --- | --- |\n"+
			"| 1 | [@alice](https://gitlab.example.com/alice) | Alice | alice@example.com | active |\n"+
			"| 2 | [@bob](https://gitlab.example.com/bob) | Bob | bob@example.com | blocked |\n"+
			"\nPage 1 of 1 | 2 items total | 20 per page\n"+
			"\n---\n💡 **Next steps:**\n"+
			"- When presenting these results, always include the clickable [text](url) links from the table so the user can navigate to GitLab\n"+
			"- Use action 'user.get' to see full user details\n")
}

// TestFormatListMarkdownString_HeadingCountsTheTotal verifies that the heading
// counts what GitLab reported rather than what this page holds: a heading
// counting two above a response reporting forty-five was the commonest way a
// list misled its reader.
func TestFormatListMarkdownString_HeadingCountsTheTotal(t *testing.T) {
	out := ListOutput{
		Users:      []Output{{ID: 1, Username: "alice", Name: "Alice", WebURL: "https://gitlab.example.com/alice"}},
		Pagination: toolutil.PaginationOutput{TotalItems: 45, Page: 1, PerPage: 1, TotalPages: 45},
	}

	assertMarkdown(t, FormatListMarkdownString(out),
		"## GitLab Users (45)\n\n"+
			"Showing 1 of 45 results (page 1 of 45)\n\n"+
			"| ID | Username | Name | Email | State |\n"+
			"| --- | --- | --- | --- | --- |\n"+
			"| 1 | [@alice](https://gitlab.example.com/alice) | Alice |  |  |\n"+
			"\nPage 1 of 45 | 45 items total | 1 per page\n"+
			"\n---\n💡 **Next steps:**\n"+
			"- When presenting these results, always include the clickable [text](url) links from the table so the user can navigate to GitLab\n"+
			"- Use action 'user.get' to see full user details\n")
}

// TestFormatListMarkdownString_Empty verifies that a list with nothing in it
// is the one sentence and nothing else: no heading counting zero above it, and
// no instruction to keep links a render with no table cannot have.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	assertMarkdown(t, FormatListMarkdownString(ListOutput{}), "No users found.\n")
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

// statusHints is the guidance section the status card closes with.
const statusHints = "\n---\n💡 **Next steps:**\n- Use action 'user.set_status' to update your status\n"

// TestFormatStatusMarkdownString_WithData verifies the whole status card.
func TestFormatStatusMarkdownString_WithData(t *testing.T) {
	out := StatusOutput{
		Emoji:         "coffee",
		Message:       "Taking a break",
		Availability:  "busy",
		ClearStatusAt: "2026-12-31T23:59:59Z",
	}

	assertMarkdown(t, FormatStatusMarkdownString(out),
		"## User Status\n\n"+
			"- **Emoji**: coffee\n"+
			"- **Message**: Taking a break\n"+
			"- **Availability**: busy\n"+
			"- **Clear At**: 31 Dec 2026 23:59 UTC\n"+
			statusHints)
}

// TestFormatStatusMarkdownString_Empty verifies that a status GitLab sent
// nothing for is the heading and the guidance, with no label carrying nothing.
func TestFormatStatusMarkdownString_Empty(t *testing.T) {
	assertMarkdown(t, FormatStatusMarkdownString(StatusOutput{}), "## User Status\n"+statusHints)
}

// TestFormatStatusMarkdownString_Partial verifies that the rows GitLab sent
// are written and the rest write nothing.
func TestFormatStatusMarkdownString_Partial(t *testing.T) {
	assertMarkdown(t, FormatStatusMarkdownString(StatusOutput{Emoji: "fire"}),
		"## User Status\n\n- **Emoji**: fire\n"+statusHints)
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

// TestFormatSSHKeyListMarkdownString_WithData verifies the whole list render.
// The two timestamps go through the display form rather than reaching the
// reader as the RFC 3339 strings GitLab sent.
func TestFormatSSHKeyListMarkdownString_WithData(t *testing.T) {
	out := SSHKeyListOutput{
		Keys: []SSHKeyOutput{
			{ID: 1, Title: "Work Laptop", UsageType: "auth", CreatedAt: "2026-01-01T00:00:00Z", ExpiresAt: "2026-01-01T00:00:00Z"},
			{ID: 2, Title: "Personal", UsageType: "auth_and_signing", CreatedAt: "2026-06-01T00:00:00Z"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}

	assertMarkdown(t, FormatSSHKeyListMarkdownString(out),
		"## SSH Keys (2)\n\n"+
			"| ID | Title | Usage Type | Created At | Expires At |\n"+
			"| --- | --- | --- | --- | --- |\n"+
			"| 1 | Work Laptop | auth | 1 Jan 2026 00:00 UTC | 1 Jan 2026 00:00 UTC |\n"+
			"| 2 | Personal | auth_and_signing | 1 Jun 2026 00:00 UTC |  |\n"+
			"\nPage 1 of 1 | 2 items total | 20 per page\n"+
			"\n---\n💡 **Next steps:**\n"+
			"- Use action 'user.get_ssh_key' to view one key in full\n")
}

// TestFormatSSHKeyListMarkdownString_Empty verifies that a list with nothing
// in it is the one sentence and nothing else.
func TestFormatSSHKeyListMarkdownString_Empty(t *testing.T) {
	assertMarkdown(t, FormatSSHKeyListMarkdownString(SSHKeyListOutput{}), "No SSH keys found.\n")
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

// TestFormatEmailListMarkdownString_WithData verifies the whole list render.
// An address GitLab has not confirmed says so rather than leaving the column
// empty, which reads as a state GitLab did not report.
func TestFormatEmailListMarkdownString_WithData(t *testing.T) {
	out := EmailListOutput{
		Emails: []EmailOutput{
			{ID: 1, Email: "primary@example.com", ConfirmedAt: "2026-01-01T00:00:00Z"},
			{ID: 2, Email: "alias@example.com"},
		},
	}

	assertMarkdown(t, FormatEmailListMarkdownString(out),
		"## Email Addresses (2)\n\n"+
			"| ID | Email | Confirmed |\n"+
			"| --- | --- | --- |\n"+
			"| 1 | primary@example.com | ✅ 1 Jan 2026 00:00 UTC |\n"+
			"| 2 | alias@example.com | ❌ awaiting confirmation |\n"+
			"\n---\n💡 **Next steps:**\n"+
			"- Use action 'user.current' to view your full profile\n")
}

// TestFormatEmailListMarkdownString_Empty verifies that a list with nothing in
// it is the one sentence and nothing else.
func TestFormatEmailListMarkdownString_Empty(t *testing.T) {
	assertMarkdown(t, FormatEmailListMarkdownString(EmailListOutput{}), "No email addresses found.\n")
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

// TestFormatContributionEventsMarkdownString_WithData verifies the whole list
// render for events that carry no target address: the target column is plain
// text, so the footer carries no instruction to keep links the table has none
// of.
func TestFormatContributionEventsMarkdownString_WithData(t *testing.T) {
	out := ContributionEventsOutput{
		Events: []ContributionEventOutput{
			{ID: 100, ActionName: "pushed", TargetType: "Project", TargetTitle: "main", CreatedAt: "2026-06-01T12:00:00Z"},
			{ID: 101, ActionName: "commented", TargetType: "Issue", TargetTitle: "Fix bug", CreatedAt: "2026-06-02T14:00:00Z"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}

	assertMarkdown(t, FormatContributionEventsMarkdownString(out),
		"## Contribution Events (2)\n\n"+
			"| ID | Action | Target Type | Target | Created At |\n"+
			"| --- | --- | --- | --- | --- |\n"+
			"| 100 | pushed | Project | main | 1 Jun 2026 12:00 UTC |\n"+
			"| 101 | commented | Issue | Fix bug | 2 Jun 2026 14:00 UTC |\n"+
			"\nPage 1 of 1 | 2 items total | 20 per page\n"+
			"\n---\n💡 **Next steps:**\n"+
			"- Use action 'user.get' to view the user's profile\n")
}

// TestFormatContributionEventsMarkdownString_Linked verifies that an event
// carrying a target address renders a link, and that the footer then names the
// preserve-links instruction the link is there for.
func TestFormatContributionEventsMarkdownString_Linked(t *testing.T) {
	out := ContributionEventsOutput{
		Events: []ContributionEventOutput{{
			ID:          100,
			ActionName:  "opened",
			TargetType:  "Issue",
			TargetIID:   7,
			TargetTitle: "Fix bug",
			TargetURL:   "https://gitlab.example.com/g/p/-/issues/7",
			CreatedAt:   "2026-06-01T12:00:00Z",
		}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1},
	}

	assertMarkdown(t, FormatContributionEventsMarkdownString(out),
		"## Contribution Events (1)\n\n"+
			"| ID | Action | Target Type | Target | Created At |\n"+
			"| --- | --- | --- | --- | --- |\n"+
			"| 100 | opened | Issue | [Fix bug](https://gitlab.example.com/g/p/-/issues/7) | 1 Jun 2026 12:00 UTC |\n"+
			"\nPage 1 of 1 | 1 items total | 20 per page\n"+
			"\n---\n💡 **Next steps:**\n"+
			"- When presenting these results, always include the clickable [text](url) links from the table so the user can navigate to GitLab\n"+
			"- Use action 'user.get' to view the user's profile\n")
}

// TestFormatContributionEventsMarkdownString_Empty verifies that a list with
// nothing in it is the one sentence and nothing else.
func TestFormatContributionEventsMarkdownString_Empty(t *testing.T) {
	assertMarkdown(t, FormatContributionEventsMarkdownString(ContributionEventsOutput{}), "No contribution events found.\n")
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

// associationsHints is the guidance section the associations card closes with.
const associationsHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'user.get' to view the user's profile\n" +
	"- Use action 'user.contribution_events' to see recent activity\n"

// TestFormatAssociationsCountMarkdownString_WithData verifies the whole card.
func TestFormatAssociationsCountMarkdownString_WithData(t *testing.T) {
	out := AssociationsCountOutput{
		GroupsCount:        5,
		ProjectsCount:      12,
		IssuesCount:        45,
		MergeRequestsCount: 30,
	}

	assertMarkdown(t, FormatAssociationsCountMarkdownString(out),
		"## User Associations Count\n\n"+
			"- **Groups**: 5\n"+
			"- **Projects**: 12\n"+
			"- **Issues**: 45\n"+
			"- **Merge Requests**: 30\n"+
			associationsHints)
}

// TestFormatAssociationsCountMarkdownString_Zero verifies that a zero count is
// written: here the zero is GitLab's answer rather than an absence.
func TestFormatAssociationsCountMarkdownString_Zero(t *testing.T) {
	assertMarkdown(t, FormatAssociationsCountMarkdownString(AssociationsCountOutput{}),
		"## User Associations Count\n\n"+
			"- **Groups**: 0\n"+
			"- **Projects**: 0\n"+
			"- **Issues**: 0\n"+
			"- **Merge Requests**: 0\n"+
			associationsHints)
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

	urls := toolutil.ResolveProjectWebURLs(context.Background(), client.GL().Projects, []int64{10})
	if got := urls[10]; got != "https://gitlab.example.com/group/project" {
		t.Errorf("urls[10] = %q, want %q", got, "https://gitlab.example.com/group/project")
	}
}

// TestList_CustomAttributesFilterReachesQuery verifies the user list encodes
// the custom attribute filter as custom_attributes[key]=value, distinct from
// with_custom_attributes which only controls whether attributes are returned.
func TestList_CustomAttributesFilterReachesQuery(t *testing.T) {
	var gotQuery string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	client := testutil.NewTestClient(t, handler)

	if _, err := List(t.Context(), client, ListInput{
		CustomAttributes: map[string]string{"role": "maintainer"},
	}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	decoded, err := url.QueryUnescape(gotQuery)
	if err != nil {
		t.Fatalf("unescape query: %v", err)
	}
	if !strings.Contains(decoded, "custom_attributes[role]=maintainer") {
		t.Errorf("query = %q, want custom_attributes[role]=maintainer", decoded)
	}
}

// fullUserJSON is a User payload populated with every top-level scalar plus the
// nested sub-objects (identities, scim_identities, custom_attributes,
// created_by) and the IP/timestamp fields, so toOutput's branches are all hit.
const fullUserJSON = `{
	"id":42,"username":"testuser","email":"test@example.com","name":"Test User",
	"state":"active","web_url":"https://gitlab.example.com/testuser","avatar_url":"https://gitlab.example.com/a.png",
	"is_admin":true,"is_auditor":true,"bot":false,"bio":"Dev","location":"Earth","job_title":"Eng",
	"organization":"ACME","created_at":"2026-01-01T00:00:00Z","confirmed_at":"2026-01-02T00:00:00Z",
	"public_email":"pub@example.com","skype":"sk","linkedin":"li","twitter":"tw","website_url":"https://x.test",
	"extern_uid":"euid","provider":"ldap","last_activity_on":"2026-06-01",
	"two_factor_enabled":true,"external":true,"locked":true,"private_profile":true,
	"current_sign_in_at":"2026-05-01T00:00:00Z","current_sign_in_ip":"10.0.0.1",
	"last_sign_in_at":"2026-04-01T00:00:00Z","last_sign_in_ip":"10.0.0.2",
	"projects_limit":50,"can_create_project":true,"can_create_group":true,"can_create_organization":true,
	"note":"admin note","using_license_seat":true,"theme_id":2,"color_scheme_id":3,
	"shared_runners_minutes_limit":100,"extra_shared_runners_minutes_limit":200,"namespace_id":7,
	"identities":[{"provider":"ldap","extern_uid":"u1"}],
	"scim_identities":[{"extern_uid":"s1","group_id":9,"active":true}],
	"custom_attributes":[{"key":"team","value":"core"}],
	"created_by":{"id":1,"username":"root","name":"Root","state":"active","created_at":"2025-01-01T00:00:00Z"}
}`

// TestGet_FullUserShape verifies toOutput maps every nested sub-object and the
// sign-in IP/timestamp fields from a fully populated User payload.
func TestGet_FullUserShape(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/users/42" {
			testutil.RespondJSON(w, http.StatusOK, fullUserJSON)
			return
		}
		http.NotFound(w, r)
	}))

	wca := true
	out, err := Get(context.Background(), client, GetInput{UserID: 42, WithCustomAttributes: &wca})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	assertFullUserScalars(t, out)
	assertFullUserSubObjects(t, out)
}

// assertFullUserScalars verifies the additive scalar fields toOutput maps.
func assertFullUserScalars(t *testing.T, out Output) {
	t.Helper()
	if !out.IsAuditor || !out.CanCreateOrganization {
		t.Errorf("auditor/can_create_organization not mapped: %+v", out)
	}
	if out.CurrentSignInIP != "10.0.0.1" || out.LastSignInIP != "10.0.0.2" {
		t.Errorf("sign-in IPs = %q/%q", out.CurrentSignInIP, out.LastSignInIP)
	}
	if out.ConfirmedAt == "" || out.LastSignInAt == "" || out.CurrentSignInAt == "" {
		t.Errorf("timestamps not mapped: %+v", out)
	}
	if out.ExternUID != "euid" || out.Provider != "ldap" || out.Skype != "sk" || out.Linkedin != "li" || out.Twitter != "tw" {
		t.Errorf("social/identity scalars not mapped: %+v", out)
	}
	if out.SharedRunnersMinutesLimit != 100 || out.ExtraSharedRunnersMinutesLimit != 200 || out.NamespaceID != 7 {
		t.Errorf("runner/namespace fields not mapped: %+v", out)
	}
}

// assertFullUserSubObjects verifies the nested sub-objects toOutput maps.
func assertFullUserSubObjects(t *testing.T, out Output) {
	t.Helper()
	if len(out.Identities) != 1 || out.Identities[0].ExternUID != "u1" {
		t.Errorf("identities = %+v", out.Identities)
	}
	if len(out.SCIMIdentities) != 1 || out.SCIMIdentities[0].GroupID != 9 {
		t.Errorf("scim identities = %+v", out.SCIMIdentities)
	}
	if len(out.CustomAttributes) != 1 || out.CustomAttributes[0].Key != "team" {
		t.Errorf("custom attributes = %+v", out.CustomAttributes)
	}
	if out.CreatedBy == nil || out.CreatedBy.Username != "root" || out.CreatedBy.CreatedAt == "" {
		t.Errorf("created_by = %+v", out.CreatedBy)
	}
}

// TestToOutput_NilAndEmptySubObjects covers the nil-User short-circuit and the
// empty-slice / nil-element paths of the sub-object converters.
func TestToOutput_NilAndEmptySubObjects(t *testing.T) {
	if got := toOutput(nil, toolutil.InstanceUserExtra{}); got.ID != 0 {
		t.Errorf("toOutput(nil) = %+v, want zero", got)
	}
	if got := toIdentityOutputs([]*gl.UserIdentity{}); got != nil {
		t.Errorf("toIdentityOutputs(empty) = %+v, want nil", got)
	}
	if got := toIdentityOutputs([]*gl.UserIdentity{nil}); got != nil {
		t.Errorf("toIdentityOutputs([nil]) = %+v, want nil", got)
	}
	if got := toCustomAttributeOutputs([]*gl.CustomAttribute{}); got != nil {
		t.Errorf("toCustomAttributeOutputs(empty) = %+v, want nil", got)
	}
	if got := toCustomAttributeOutputs([]*gl.CustomAttribute{nil}); got != nil {
		t.Errorf("toCustomAttributeOutputs([nil]) = %+v, want nil", got)
	}
	if got := toBasicUserOutput(nil); got != nil {
		t.Errorf("toBasicUserOutput(nil) = %+v, want nil", got)
	}
}

// TestList_AllFilters exercises every new ListUsers filter and keyset option so
// the option-wiring branches are covered, asserting the resulting query string.
func TestList_AllFilters(t *testing.T) {
	var query url.Values
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/users" {
			query = r.URL.Query()
			testutil.RespondJSON(w, http.StatusOK, `[`+fullUserJSON+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	bt := true
	_, err := List(context.Background(), client, ListInput{
		Search: "alice", Username: "alice", Active: &bt, Blocked: &bt, External: &bt,
		Admins: &bt, Humans: &bt, ExcludeActive: &bt, ExcludeExternal: &bt, ExcludeHumans: &bt,
		ExcludeInternal: &bt, WithoutProjects: &bt, WithoutProjectBots: &bt, WithCustomAttributes: &bt,
		TwoFactor: "enabled", ExternUID: "uid", Provider: "ldap", PublicEmail: "p@x.test",
		CreatedAfter: "2026-01-01T00:00:00Z", CreatedBefore: "2026-12-31T00:00:00Z",
		OrderBy: "id", Sort: "desc",
		Page: 1, PerPage: 20,
		Pagination: "keyset", PageToken: "100",
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	for _, key := range []string{
		"admins", "humans", "exclude_active", "exclude_external", "exclude_humans",
		"exclude_internal", "without_projects", "without_project_bots", "with_custom_attributes",
		"two_factor", "extern_uid", "provider", "public_email", "created_after", "created_before",
		"pagination", "page_token",
	} {
		t.Run(key, func(t *testing.T) {
			if query.Get(key) == "" {
				t.Errorf("query missing %q: %v", key, query.Encode())
			}
		})
	}
}

// TestListContributionEvents_ScopeAndKeyset covers the new scope filter and the
// keyset pagination wiring on the contribution events list.
func TestListContributionEvents_ScopeAndKeyset(t *testing.T) {
	var query url.Values
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/users/42/events" {
			query = r.URL.Query()
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := ListContributionEvents(context.Background(), client, ListContributionEventsInput{
		UserID:     42,
		Scope:      "all",
		OrderBy:    "id",
		Pagination: "keyset", PageToken: "5",
	})
	if err != nil {
		t.Fatalf("ListContributionEvents() unexpected error: %v", err)
	}
	if query.Get("scope") != "all" || query.Get("pagination") != "keyset" || query.Get("page_token") != "5" {
		t.Errorf("query = %v, want scope/keyset wiring", query.Encode())
	}
	if query.Get("order_by") != "id" {
		t.Errorf("order_by = %q, want id", query.Get("order_by"))
	}
}

// TestListSSHKeys_OrderSort covers applyOrderSort wiring (order_by + sort) on a
// list endpoint whose SDK options expose ordering through embedded ListOptions.
func TestListSSHKeys_OrderSort(t *testing.T) {
	var query url.Values
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/user/keys" {
			query = r.URL.Query()
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := ListSSHKeys(context.Background(), client, ListSSHKeysInput{OrderBy: "id", Sort: "desc"})
	if err != nil {
		t.Fatalf("ListSSHKeys() unexpected error: %v", err)
	}
	if query.Get("order_by") != "id" || query.Get("sort") != "desc" {
		t.Errorf("query = %v, want order_by=id sort=desc", query.Encode())
	}
}
