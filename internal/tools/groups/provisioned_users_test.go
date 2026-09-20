// provisioned_users_test.go contains unit tests for listing a group's
// provisioned users, including filters, validation, not-found handling and
// output mapping. Tests use httptest to mock the GitLab API.
package groups

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestListProvisionedUsers_FiltersAndOutput verifies the query parameters, the
// pagination headers, and the identity, nested lists and creator of the one
// user published; every field of a user is held to its value by
// [TestListProvisionedUsers_PublishesEachFieldGitLabSent].
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
		t.Run(k, func(t *testing.T) {
			if got := query.Get(k); got != want {
				t.Errorf("query %s = %q, want %q", k, got, want)
			}
		})
	}

	if len(out.Users) != 1 {
		t.Fatalf("len(Users) = %d, want 1", len(out.Users))
	}
	assertProvisionedUser(t, out.Users[0])
	if out.Pagination.TotalItems != 2 || out.Pagination.TotalPages != 1 {
		t.Fatalf("pagination = %#v, want 2 items / 1 page", out.Pagination)
	}
}

// assertProvisionedUser checks the fixture user's identity, its three nested
// lists and its creator. It does not read the name, the URL or the bot flag.
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

// TestProvisionedUserToOutput_AllFields sets every scalar and timestamp of the
// gl.User -> ProvisionedUserOutput mapping and reads back the seven times and
// addresses, the creator's date, five scalars and the three list lengths. It
// asserts nothing about the other scalars; [TestListProvisionedUsers_PublishesEachFieldGitLabSent]
// holds each of those to its value.
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
	out := ProvisionedUserToOutput(u, toolutil.UserExtra{})
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

// TestProvisionedUserToOutput_CopiesEveryCapturedKey verifies the converter
// writes all ten keys of the captured extra onto the output. A key added to
// toolutil.UserExtra and not copied here would be a schema field nothing fills.
func TestProvisionedUserToOutput_CopiesEveryCapturedKey(t *testing.T) {
	followers, following, followed := int64(3), int64(4), true
	out := ProvisionedUserToOutput(&gl.User{ID: 7, Username: "u"}, toolutil.UserExtra{
		CommitEmail: "c@x.com", Discord: "d#1", GitHub: "gh", LocalTime: "9:00 AM",
		PreferredLanguage: "es", Pronouns: "they/them", WorkInformation: "org",
		Followers: &followers, Following: &following, IsFollowed: &followed,
	})
	if out.CommitEmail != "c@x.com" || out.Discord != "d#1" || out.GitHub != "gh" {
		t.Errorf("out = %#v, want the commit address and the two account names", out)
	}
	if out.LocalTime != "9:00 AM" || out.PreferredLanguage != "es" ||
		out.Pronouns != "they/them" || out.WorkInformation != "org" {
		t.Errorf("out = %#v, want the four profile keys", out)
	}
	if out.Followers == nil || *out.Followers != 3 || out.Following == nil || *out.Following != 4 ||
		out.IsFollowed == nil || !*out.IsFollowed {
		t.Errorf("out counts = %v/%v/%v, want 3, 4 and true", out.Followers, out.Following, out.IsFollowed)
	}
}

// TestProvisionedUserToOutput_NilTimes verifies the nil-timestamp branches leave
// the corresponding string fields empty.
func TestProvisionedUserToOutput_NilTimes(t *testing.T) {
	out := ProvisionedUserToOutput(&gl.User{ID: 1, Username: "u"}, toolutil.UserExtra{})
	if out.CreatedAt != "" || out.LastActivityOn != "" || out.CurrentSignInIP != "" || out.ConfirmedAt != "" {
		t.Fatalf("expected empty timestamp/IP fields, got %#v", out)
	}
	// A caller who may not read the profile is sent no count at all, which the
	// pointers keep distinct from a user nobody follows.
	if out.Followers != nil || out.Following != nil || out.IsFollowed != nil {
		t.Fatalf("follow counts = %v/%v/%v, want all nil", out.Followers, out.Following, out.IsFollowed)
	}
	// The creator object carries a date of its own, and GitLab leaves it out
	// for a user created before it recorded one, so the nested branch has a
	// side of its own to answer for.
	nested := ProvisionedUserToOutput(&gl.User{ID: 1, Username: "u", CreatedBy: &gl.BasicUser{ID: 2, Username: "admin"}}, toolutil.UserExtra{})
	if nested.CreatedBy == nil {
		t.Fatal("created_by was dropped")
	}
	if nested.CreatedBy.CreatedAt != "" {
		t.Errorf("created_by.created_at = %q, want empty", nested.CreatedBy.CreatedAt)
	}
}

// TestListProvisionedUsers_PairsTheCapturedKeysByPosition verifies the ten keys
// lib/api/entities/user_public.rb sends that client-go's User declares on no
// field of its own reach the output, one captured extra per row in order, and
// that a row GitLab sent no follow counts on leaves them absent rather than
// zero. GitLab presents this endpoint `with: ::API::Entities::UserPublic`, so
// these ten are the whole set it can carry.
func TestListProvisionedUsers_PairsTheCapturedKeysByPosition(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/42/provisioned_users", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id":1,"username":"alice","commit_email":"c@example.com","discord":"alice#1","github":"alice",
			 "local_time":"2:30 PM","preferred_language":"en","pronouns":"she/her","work_information":"Org",
			 "followers":12,"following":34,"is_followed":true},
			{"id":2,"username":"bob"}
		]`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListProvisionedUsers(context.Background(), client, ListProvisionedUsersInput{GroupID: "42"})
	if err != nil {
		t.Fatalf("ListProvisionedUsers() unexpected error: %v", err)
	}
	if len(out.Users) != 2 {
		t.Fatalf("got %d users, want 2", len(out.Users))
	}
	first := out.Users[0]
	if first.CommitEmail != "c@example.com" || first.Discord != "alice#1" || first.GitHub != "alice" ||
		first.LocalTime != "2:30 PM" || first.PreferredLanguage != "en" || first.Pronouns != "she/her" ||
		first.WorkInformation != "Org" {
		t.Errorf("Users[0] = %+v, want the seven unconditional keys", first)
	}
	if first.Followers == nil || *first.Followers != 12 || first.Following == nil || *first.Following != 34 ||
		first.IsFollowed == nil || !*first.IsFollowed {
		t.Errorf("Users[0] counts = %v/%v/%v, want 12, 34 and true", first.Followers, first.Following, first.IsFollowed)
	}
	if out.Users[1].CommitEmail != "" || out.Users[1].Followers != nil || out.Users[1].IsFollowed != nil {
		t.Errorf("Users[1] = %+v, want the second row left empty", out.Users[1])
	}
}

// TestListProvisionedUsers_ACapturedFieldTheTypeCannotHold_IsReported verifies
// a body that decodes for the SDK and not for the keys read beside it is the
// operation's error rather than a page of users with those keys silently empty.
func TestListProvisionedUsers_ACapturedFieldTheTypeCannotHold_IsReported(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/42/provisioned_users", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"username":"alice","following":"not-a-number"}]`)
	})
	client := testutil.NewTestClient(t, mux)

	_, err := ListProvisionedUsers(context.Background(), client, ListProvisionedUsersInput{GroupID: "42"})
	if err == nil || !strings.Contains(err.Error(), "decode the captured response") {
		t.Errorf("ListProvisionedUsers() error = %v, want the capture's decode failure", err)
	}
}

// listDistinctProvisionedUsers answers the list with body and returns the one
// user it published.
func listDistinctProvisionedUsers(t *testing.T, body string) ProvisionedUserOutput {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/42/provisioned_users", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, body)
	})
	client := testutil.NewTestClient(t, mux)
	out, err := ListProvisionedUsers(context.Background(), client, ListProvisionedUsersInput{GroupID: "42"})
	if err != nil {
		t.Fatalf("ListProvisionedUsers() unexpected error: %v", err)
	}
	if len(out.Users) != 1 {
		t.Fatalf("got %d users, want 1", len(out.Users))
	}
	return out.Users[0]
}

// TestListProvisionedUsers_PublishesEachFieldGitLabSent verifies the whole
// user against an answer in which no two values agree, the captured keys and
// the nested objects included. The all-fields test above sets a value on
// every field and reads five of them back, so a converter copying one of the
// nine profile strings into a neighbor's field passed.
func TestListProvisionedUsers_PublishesEachFieldGitLabSent(t *testing.T) {
	out := listDistinctProvisionedUsers(t, `[{"id":7,"username":"scim-user","email":"scim@acme.example","name":"SCIM User",`+
		`"state":"active","web_url":"https://gl.example.com/scim-user","created_at":"2026-02-03T04:05:06Z",`+
		`"bio":"a bio","location":"Madrid","public_email":"public@acme.example","skype":"scim.skype",`+
		`"linkedin":"scim-linkedin","twitter":"scim_twitter","website_url":"https://scim.example",`+
		`"organization":"Acme","job_title":"Engineer","extern_uid":"uid-7","provider":"group_saml",`+
		`"theme_id":2,"last_activity_on":"2026-03-04","color_scheme_id":3,"avatar_url":"https://gl.example.com/uploads/7.png",`+
		`"projects_limit":50,"current_sign_in_at":"2026-04-05T06:07:08Z","current_sign_in_ip":"10.0.0.1",`+
		`"last_sign_in_at":"2026-04-04T06:07:08Z","last_sign_in_ip":"10.0.0.2","confirmed_at":"2026-02-04T04:05:06Z",`+
		`"note":"a note","identities":[{"provider":"group_saml","extern_uid":"uid-7"}],`+
		`"scim_identities":[{"extern_uid":"scim-7","group_id":42,"active":true}],`+
		`"shared_runners_minutes_limit":1500,"extra_shared_runners_minutes_limit":250,`+
		`"custom_attributes":[{"key":"dept","value":"eng"}],"namespace_id":9,`+
		`"created_by":{"id":1,"username":"admin","name":"Admin","state":"blocked","created_at":"2023-01-02T03:04:05Z",`+
		`"avatar_url":"https://gl.example.com/uploads/1.png","web_url":"https://gl.example.com/admin"},`+
		`"commit_email":"commit@acme.example","discord":"scim#1","github":"scim-gh","local_time":"2:30 PM",`+
		`"preferred_language":"es","pronouns":"they/them","work_information":"Acme Engineering",`+
		`"followers":12,"following":34,"is_followed":true}]`)
	want := ProvisionedUserOutput{
		ID: 7, Username: "scim-user", Email: "scim@acme.example", Name: "SCIM User", State: "active",
		WebURL: "https://gl.example.com/scim-user", CreatedAt: "2026-02-03T04:05:06Z", Bio: "a bio",
		Location: "Madrid", PublicEmail: "public@acme.example", Skype: "scim.skype", Linkedin: "scim-linkedin",
		Twitter: "scim_twitter", WebsiteURL: "https://scim.example", Organization: "Acme", JobTitle: "Engineer",
		ExternUID: "uid-7", Provider: "group_saml", ThemeID: 2, LastActivityOn: "2026-03-04", ColorSchemeID: 3,
		AvatarURL: "https://gl.example.com/uploads/7.png", ProjectsLimit: 50,
		CurrentSignInAt: "2026-04-05T06:07:08Z", CurrentSignInIP: "10.0.0.1",
		LastSignInAt: "2026-04-04T06:07:08Z", LastSignInIP: "10.0.0.2", ConfirmedAt: "2026-02-04T04:05:06Z",
		Note:                      "a note",
		Identities:                []ProvisionedUserIdentity{{Provider: "group_saml", ExternUID: "uid-7"}},
		SCIMIdentities:            []ProvisionedUserSCIMIdentity{{ExternUID: "scim-7", GroupID: 42, Active: true}},
		SharedRunnersMinutesLimit: 1500, ExtraSharedRunnersMinutesLimit: 250,
		CustomAttributes: []ProvisionedUserCustomAttribute{{Key: "dept", Value: "eng"}},
		NamespaceID:      9,
		CreatedBy: &ProvisionedUserBasicUser{
			ID: 1, Username: "admin", Name: "Admin", State: "blocked", CreatedAt: "2023-01-02T03:04:05Z",
			AvatarURL: "https://gl.example.com/uploads/1.png", WebURL: "https://gl.example.com/admin",
		},
		CommitEmail: "commit@acme.example", Discord: "scim#1", GitHub: "scim-gh", LocalTime: "2:30 PM",
		PreferredLanguage: "es", Pronouns: "they/them", WorkInformation: "Acme Engineering",
		Followers: new(int64(12)), Following: new(int64(34)), IsFollowed: new(true),
	}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("ListProvisionedUsers() published\n%+v\nwant\n%+v", out, want)
	}
}

// TestListProvisionedUsers_PublishesEachFlagOnItsOwn verifies each of the
// eleven booleans a user carries lands on its own field when sent alone; the
// all-fields test sets ten of them true together.
func TestListProvisionedUsers_PublishesEachFlagOnItsOwn(t *testing.T) {
	for _, tc := range []struct {
		key  string
		want ProvisionedUserOutput
	}{
		{key: "bot", want: ProvisionedUserOutput{ID: 7, Bot: true}},
		{key: "is_admin", want: ProvisionedUserOutput{ID: 7, IsAdmin: true}},
		{key: "is_auditor", want: ProvisionedUserOutput{ID: 7, IsAuditor: true}},
		{key: "can_create_group", want: ProvisionedUserOutput{ID: 7, CanCreateGroup: true}},
		{key: "can_create_project", want: ProvisionedUserOutput{ID: 7, CanCreateProject: true}},
		{key: "can_create_organization", want: ProvisionedUserOutput{ID: 7, CanCreateOrganization: true}},
		{key: "two_factor_enabled", want: ProvisionedUserOutput{ID: 7, TwoFactorEnabled: true}},
		{key: "external", want: ProvisionedUserOutput{ID: 7, External: true}},
		{key: "private_profile", want: ProvisionedUserOutput{ID: 7, PrivateProfile: true}},
		{key: "using_license_seat", want: ProvisionedUserOutput{ID: 7, UsingLicenseSeat: true}},
		{key: "locked", want: ProvisionedUserOutput{ID: 7, Locked: true}},
	} {
		t.Run(tc.key, func(t *testing.T) {
			out := listDistinctProvisionedUsers(t, `[{"id":7,"`+tc.key+`":true}]`)
			if !reflect.DeepEqual(out, tc.want) {
				t.Errorf("ListProvisionedUsers() published %+v, want %+v", out, tc.want)
			}
		})
	}
}
