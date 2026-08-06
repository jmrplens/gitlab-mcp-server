// provisioned_users_test.go contains unit tests for listing a group's
// provisioned users, including filters, validation, not-found handling and
// output mapping. Tests use httptest to mock the GitLab API.
package groups

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// TestListProvisionedUsers_FiltersAndOutput verifies the query parameters, the
// pagination headers, and that the full user output is parsed 1:1.
func TestListProvisionedUsers_FiltersAndOutput(t *testing.T) {
	var query url.Values
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/org%2Finfra/provisioned_users", func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":7,"username":"scim-user","name":"SCIM User","state":"active","email":"scim@example.com","web_url":"https://gitlab.example.com/scim-user","bot":false,
			 "identities":[{"provider":"group_saml","extern_uid":"uid-7"}],
			 "scim_identities":[{"extern_uid":"scim-7","group_id":99,"active":true}],
			 "custom_attributes":[{"key":"dept","value":"eng"}],
			 "created_by":{"id":1,"username":"admin","name":"Admin","created_at":"2023-01-02T03:04:05Z"}}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
	})
	client := testutil.NewTestClient(t, mux)

	active := true
	blocked := false
	out, err := ListProvisionedUsers(context.Background(), client, ListProvisionedUsersInput{
		GroupID:       "org/infra",
		Username:      "scim-user",
		Search:        "scim",
		Active:        &active,
		Blocked:       &blocked,
		CreatedAfter:  "2024-01-02T15:04:05Z",
		CreatedBefore: "2024-12-31T23:59:59Z",
		OrderBy:       "created_at",
		Sort:          "desc",
	})
	if err != nil {
		t.Fatalf("ListProvisionedUsers error: %v", err)
	}

	for k, want := range map[string]string{
		"username":       "scim-user",
		"search":         "scim",
		"active":         "true",
		"blocked":        "false",
		"created_after":  "2024-01-02T15:04:05Z",
		"created_before": "2024-12-31T23:59:59Z",
		"order_by":       "created_at",
		"sort":           "desc",
	} {
		if got := query.Get(k); got != want {
			t.Errorf("query %s = %q, want %q", k, got, want)
		}
	}

	if len(out.Users) != 1 {
		t.Fatalf("len(Users) = %d, want 1", len(out.Users))
	}
	assertProvisionedUser(t, out.Users[0])
	if out.Pagination.TotalItems != 2 || out.Pagination.TotalPages != 1 {
		t.Fatalf("pagination = %#v, want 2 items / 1 page", out.Pagination)
	}
}

// assertProvisionedUser checks the full 1:1 mapping of the fixture user.
func assertProvisionedUser(t *testing.T, u ProvisionedUserOutput) {
	t.Helper()
	if u.ID != 7 || u.Username != "scim-user" || u.Email != "scim@example.com" || u.State != "active" {
		t.Fatalf("user output = %#v, want full scim-user fields", u)
	}
	if len(u.Identities) != 1 || u.Identities[0].Provider != "group_saml" || u.Identities[0].ExternUID != "uid-7" {
		t.Fatalf("identities = %#v, want group_saml/uid-7", u.Identities)
	}
	if len(u.SCIMIdentities) != 1 || u.SCIMIdentities[0].GroupID != 99 || !u.SCIMIdentities[0].Active {
		t.Fatalf("scim_identities = %#v, want group 99 active", u.SCIMIdentities)
	}
	if len(u.CustomAttributes) != 1 || u.CustomAttributes[0].Key != "dept" || u.CustomAttributes[0].Value != "eng" {
		t.Fatalf("custom_attributes = %#v, want dept/eng", u.CustomAttributes)
	}
	if u.CreatedBy == nil || u.CreatedBy.Username != "admin" {
		t.Fatalf("created_by = %#v, want admin", u.CreatedBy)
	}
	if u.CreatedBy.CreatedAt != "2023-01-02T03:04:05Z" {
		t.Fatalf("created_by.created_at = %q, want 2023-01-02T03:04:05Z", u.CreatedBy.CreatedAt)
	}
}

// TestListProvisionedUsers_Validation covers required-field and timestamp errors.
func TestListProvisionedUsers_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	tests := []struct {
		name    string
		input   ListProvisionedUsersInput
		wantErr string
	}{
		{"missing group_id", ListProvisionedUsersInput{}, "group_id is required"},
		{"bad created_after", ListProvisionedUsersInput{GroupID: "99", CreatedAfter: "not-a-time"}, "created_after must be an RFC3339"},
		{"bad created_before", ListProvisionedUsersInput{GroupID: "99", CreatedBefore: "not-a-time"}, "created_before must be an RFC3339"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ListProvisionedUsers(context.Background(), client, tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

// TestListProvisionedUsers_NotFound verifies the 404 hint.
func TestListProvisionedUsers_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/99/provisioned_users", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Group Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := ListProvisionedUsers(context.Background(), client, ListProvisionedUsersInput{GroupID: "99"})
	if err == nil || !strings.Contains(err.Error(), "SAML/SCIM-enabled group") {
		t.Fatalf("err = %v, want SCIM hint", err)
	}
}

// TestProvisionedUserToOutput_NilSliceElements verifies nil-element skipping in
// the nested identity slices and nil created_by handling.
func TestProvisionedUserToOutput_NilSliceElements(t *testing.T) {
	if got := provisionedUserIdentities(nil); got != nil {
		t.Fatalf("identities(nil) = %#v, want nil", got)
	}
	if got := provisionedUserSCIMIdentities(nil); got != nil {
		t.Fatalf("scim(nil) = %#v, want nil", got)
	}
	if got := provisionedUserCustomAttributes(nil); got != nil {
		t.Fatalf("attrs(nil) = %#v, want nil", got)
	}
	if got := provisionedUserBasicUser(nil); got != nil {
		t.Fatalf("basicUser(nil) = %#v, want nil", got)
	}
}

// TestProvisionedUserToOutput_AllFields exercises every scalar and timestamp
// branch of the gl.User -> ProvisionedUserOutput mapping.
func TestProvisionedUserToOutput_AllFields(t *testing.T) {
	ts := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	iso := gl.ISOTime(ts)
	ip1 := net.ParseIP("10.0.0.1")
	ip2 := net.ParseIP("10.0.0.2")
	u := &gl.User{
		ID: 7, Username: "u", Email: "e@x.com", Name: "N", State: "active", WebURL: "https://g/u",
		Bio: "bio", Bot: true, Location: "loc", PublicEmail: "p@x.com", Skype: "sk", Linkedin: "li",
		Twitter: "tw", WebsiteURL: "https://w", Organization: "org", JobTitle: "jt", ExternUID: "x",
		Provider: "saml", ThemeID: 2, ColorSchemeID: 3, IsAdmin: true, IsAuditor: true, AvatarURL: "https://a",
		CanCreateGroup: true, CanCreateProject: true, CanCreateOrganization: true, ProjectsLimit: 5,
		TwoFactorEnabled: true, Note: "note", External: true, PrivateProfile: true,
		SharedRunnersMinutesLimit: 100, ExtraSharedRunnersMinutesLimit: 50, UsingLicenseSeat: true,
		NamespaceID: 9, Locked: true,
		CreatedAt: &ts, LastActivityOn: &iso, CurrentSignInAt: &ts, CurrentSignInIP: &ip1,
		LastSignInAt: &ts, LastSignInIP: &ip2, ConfirmedAt: &ts,
		Identities:       []*gl.UserIdentity{{Provider: "saml", ExternUID: "x"}, nil},
		SCIMIdentities:   []*gl.SCIMIdentity{{ExternUID: "s", GroupID: 9, Active: true}, nil},
		CustomAttributes: []*gl.CustomAttribute{{Key: "k", Value: "v"}, nil},
		CreatedBy:        &gl.BasicUser{ID: 1, Username: "admin", Name: "Admin", CreatedAt: &ts},
	}
	out := ProvisionedUserToOutput(u)
	if out.CreatedAt == "" || out.LastActivityOn == "" || out.CurrentSignInAt == "" ||
		out.CurrentSignInIP != "10.0.0.1" || out.LastSignInAt == "" || out.LastSignInIP != "10.0.0.2" ||
		out.ConfirmedAt == "" {
		t.Fatalf("timestamp/IP fields not fully mapped: %#v", out)
	}
	if out.CreatedBy == nil || out.CreatedBy.CreatedAt == "" {
		t.Fatalf("created_by.created_at not mapped: %#v", out.CreatedBy)
	}
	if !out.Bot || !out.IsAdmin || out.JobTitle != "jt" || out.ProjectsLimit != 5 || out.NamespaceID != 9 {
		t.Fatalf("scalar fields not fully mapped: %#v", out)
	}
	if len(out.Identities) != 1 || len(out.SCIMIdentities) != 1 || len(out.CustomAttributes) != 1 {
		t.Fatalf("nil slice elements not skipped: %#v", out)
	}
}

// TestProvisionedUserToOutput_NilTimes verifies the nil-timestamp branches leave
// the corresponding string fields empty.
func TestProvisionedUserToOutput_NilTimes(t *testing.T) {
	out := ProvisionedUserToOutput(&gl.User{ID: 1, Username: "u"})
	if out.CreatedAt != "" || out.LastActivityOn != "" || out.CurrentSignInIP != "" || out.ConfirmedAt != "" {
		t.Fatalf("expected empty timestamp/IP fields, got %#v", out)
	}
}
