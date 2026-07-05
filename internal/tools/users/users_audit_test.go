// users_audit_test.go contains coverage for the 1:1-audit field expansions:
// the full gl.User output mapping (nested identities, SCIM identities, custom
// attributes, created_by, sign-in IPs/timestamps) and the additional
// create/modify/list input fields wired onto the SDK option structs.
package users

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// decodeJSONBody reads an HTTP request's JSON body into a generic map so tests
// can assert which option fields the SDK serialized.
func decodeJSONBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	raw, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		t.Fatalf("read body: %v", readErr)
	}
	m := map[string]any{}
	if len(raw) > 0 {
		if jsonErr := json.Unmarshal(raw, &m); jsonErr != nil {
			t.Fatalf("unmarshal body %q: %v", raw, jsonErr)
		}
	}
	return m
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
	if got := toOutput(nil); got.ID != 0 {
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
		PaginationInput:       toolutil.PaginationInput{Page: 1, PerPage: 20},
		KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "100"},
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
		if query.Get(key) == "" {
			t.Errorf("query missing %q: %v", key, query.Encode())
		}
	}
}

// TestCreate_AllOptionalFields exercises every new CreateUser optional field so
// the option-wiring branches are covered.
func TestCreate_AllOptionalFields(t *testing.T) {
	var body map[string]any
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/users" {
			body = decodeJSONBody(t, r)
			testutil.RespondJSON(w, http.StatusCreated, fullUserJSON)
			return
		}
		http.NotFound(w, r)
	}))

	bt := true
	n := int64(5)
	_, err := Create(context.Background(), client, CreateInput{
		Email: "n@x.test", Name: "N", Username: "n",
		Auditor: &bt, CanCreateGroup: &bt, PrivateProfile: &bt, ViewDiffsFileByFile: &bt,
		Pronouns: "they", CommitEmail: "c@x.test", PublicEmail: "p@x.test", WebsiteURL: "https://x.test",
		Linkedin: "li", Twitter: "tw", Skype: "sk", Discord: "dc", Github: "gh",
		Provider: "ldap", ExternUID: "uid",
		GroupIDForSAML: &n, ProjectsLimit: &n, ThemeID: &n, ColorSchemeID: &n,
		SharedRunnersMinutesLimit: &n, ExtraSharedRunnersMinutesLimit: &n,
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	for _, key := range []string{
		"auditor", "can_create_group", "private_profile", "view_diffs_file_by_file",
		"pronouns", "commit_email", "public_email", "website_url", "linkedin", "twitter",
		"skype", "discord", "github", "provider", "extern_uid", "group_id_for_saml",
		"theme_id", "color_scheme_id", "shared_runners_minutes_limit", "extra_shared_runners_minutes_limit",
	} {
		if _, ok := body[key]; !ok {
			t.Errorf("create body missing %q: %v", key, body)
		}
	}
}

// TestModify_AllOptionalFields exercises every new ModifyUser optional field so
// the option-wiring branches are covered.
func TestModify_AllOptionalFields(t *testing.T) {
	var body map[string]any
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/users/42" {
			body = decodeJSONBody(t, r)
			testutil.RespondJSON(w, http.StatusOK, fullUserJSON)
			return
		}
		http.NotFound(w, r)
	}))

	bt := true
	n := int64(4)
	_, err := Modify(context.Background(), client, ModifyInput{
		UserID: 42, Auditor: &bt, ViewDiffsFileByFile: &bt, CommitEmail: "c@x.test", PublicEmail: "p@x.test",
		WebsiteURL: "https://x.test", Linkedin: "li", Twitter: "tw", Skype: "sk",
		Provider: "ldap", ExternUID: "uid", ThemeID: &n,
	})
	if err != nil {
		t.Fatalf("Modify() unexpected error: %v", err)
	}
	for _, key := range []string{
		"auditor", "view_diffs_file_by_file", "commit_email", "public_email", "website_url",
		"linkedin", "twitter", "skype", "provider", "extern_uid", "theme_id",
	} {
		if _, ok := body[key]; !ok {
			t.Errorf("modify body missing %q: %v", key, body)
		}
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
		UserID:                42,
		Scope:                 "all",
		OrderBy:               "id",
		KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "5"},
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
