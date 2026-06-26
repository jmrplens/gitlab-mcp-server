// avatar_provisioned_test.go contains tests for the group avatar upload and
// list-provisioned-users tools.
package groups

import (
	"context"
	"encoding/base64"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

const groupAvatarRespJSON = `{"id":99,"name":"infra","path":"infra","full_path":"org/infra","visibility":"private","web_url":"https://gitlab.example.com/groups/org/infra"}`

// TestUploadAvatar_ContentBase64_Multipart verifies that an inline base64 image
// is streamed to PUT /groups/{id} as a multipart upload with the avatar field
// and filename, and that the updated group is returned.
func TestUploadAvatar_ContentBase64_Multipart(t *testing.T) {
	var gotFilename, gotField, gotContent string
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v4/groups/99", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil { //nolint:gosec // Test handler parses a small in-memory fixture body.
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		for field, files := range r.MultipartForm.File {
			gotField = field
			f, err := files[0].Open()
			if err != nil {
				t.Fatalf("open multipart file: %v", err)
			}
			defer f.Close()
			buf := make([]byte, 64)
			n, _ := f.Read(buf)
			gotContent = string(buf[:n])
			gotFilename = files[0].Filename
		}
		testutil.RespondJSON(w, http.StatusOK, groupAvatarRespJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := UploadAvatar(context.Background(), client, UploadAvatarInput{
		GroupID:       "99",
		Filename:      "logo.png",
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("avatar-bytes")),
	})
	if err != nil {
		t.Fatalf("UploadAvatar error: %v", err)
	}
	if out.ID != 99 {
		t.Fatalf("output ID = %d, want 99", out.ID)
	}
	if gotField != "avatar" {
		t.Fatalf("multipart field = %q, want avatar", gotField)
	}
	if gotFilename != "logo.png" {
		t.Fatalf("multipart filename = %q, want logo.png", gotFilename)
	}
	if gotContent != "avatar-bytes" {
		t.Fatalf("multipart content = %q, want avatar-bytes", gotContent)
	}
}

// TestUploadAvatar_FilePath verifies that file_path reads a local file and
// uploads it as multipart content.
func TestUploadAvatar_FilePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pic.png")
	if err := os.WriteFile(path, []byte("file-bytes"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	var gotContent string
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v4/groups/99", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20) //nolint:gosec // Test handler parses a small in-memory fixture body.
		for _, files := range r.MultipartForm.File {
			f, _ := files[0].Open()
			defer f.Close()
			buf := make([]byte, 64)
			n, _ := f.Read(buf)
			gotContent = string(buf[:n])
		}
		testutil.RespondJSON(w, http.StatusOK, groupAvatarRespJSON)
	})
	client := testutil.NewTestClient(t, mux)

	if _, err := UploadAvatar(context.Background(), client, UploadAvatarInput{
		GroupID:  "99",
		Filename: "pic.png",
		FilePath: path,
	}); err != nil {
		t.Fatalf("UploadAvatar error: %v", err)
	}
	if gotContent != "file-bytes" {
		t.Fatalf("multipart content = %q, want file-bytes", gotContent)
	}
}

// TestUploadAvatar_Validation covers the input validation branches.
func TestUploadAvatar_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	tests := []struct {
		name    string
		input   UploadAvatarInput
		wantErr string
	}{
		{"missing group_id", UploadAvatarInput{Filename: "a.png", ContentBase64: "YQ=="}, "group_id is required"},
		{"missing filename", UploadAvatarInput{GroupID: "99", ContentBase64: "YQ=="}, "filename is required"},
		{"both sources", UploadAvatarInput{GroupID: "99", Filename: "a.png", FilePath: "/tmp/x", ContentBase64: "YQ=="}, "not both"},
		{"no source", UploadAvatarInput{GroupID: "99", Filename: "a.png"}, "either file_path or content_base64 is required"},
		{"bad base64", UploadAvatarInput{GroupID: "99", Filename: "a.png", ContentBase64: "!!!notbase64!!!"}, "invalid base64"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := UploadAvatar(context.Background(), client, tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

// TestUploadAvatar_FilePathOpenError covers the OpenAndValidateFile error branch
// when the local file does not exist.
func TestUploadAvatar_FilePathOpenError(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := UploadAvatar(context.Background(), client, UploadAvatarInput{
		GroupID:  "99",
		Filename: "a.png",
		FilePath: filepath.Join(t.TempDir(), "does-not-exist.png"),
	})
	if err == nil || !strings.Contains(err.Error(), "groupUploadAvatar") {
		t.Fatalf("err = %v, want groupUploadAvatar file error", err)
	}
}

// TestUploadAvatar_ErrorStatuses covers the GitLab error-to-hint mapping.
func TestUploadAvatar_ErrorStatuses(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   string
	}{
		{"bad request", http.StatusBadRequest, "200 KB"},
		{"forbidden", http.StatusForbidden, "Owner role"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("PUT /api/v4/groups/99", func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tt.status, `{"message":"nope"}`)
			})
			client := testutil.NewTestClient(t, mux)
			_, err := UploadAvatar(context.Background(), client, UploadAvatarInput{
				GroupID: "99", Filename: "a.png", ContentBase64: base64.StdEncoding.EncodeToString([]byte("x")),
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want containing %q", err, tt.want)
			}
		})
	}
}

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

// TestContextCancellation covers the ctx.Err() guard in both new handlers.
func TestContextCancellation(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := UploadAvatar(ctx, client, UploadAvatarInput{GroupID: "1", Filename: "a.png", ContentBase64: "YQ=="}); err == nil {
		t.Fatal("UploadAvatar: expected context error")
	}
	if _, err := ListProvisionedUsers(ctx, client, ListProvisionedUsersInput{GroupID: "1"}); err == nil {
		t.Fatal("ListProvisionedUsers: expected context error")
	}
}

// TestFormatProvisionedUsersListMarkdown_Empty covers the empty-list branch.
func TestFormatProvisionedUsersListMarkdown_Empty(t *testing.T) {
	md := FormatProvisionedUsersListMarkdown(ProvisionedUsersListOutput{})
	if !strings.Contains(md, "No provisioned users found.") {
		t.Fatalf("markdown = %q, want empty message", md)
	}
}

// TestFormatProvisionedUsersListMarkdown_Rows covers the populated table branch.
func TestFormatProvisionedUsersListMarkdown_Rows(t *testing.T) {
	md := FormatProvisionedUsersListMarkdown(ProvisionedUsersListOutput{
		Users: []ProvisionedUserOutput{{ID: 7, Username: "scim-user", Name: "SCIM User", State: "active", Email: "s@e.com", WebURL: "https://g/scim-user"}},
	})
	if !strings.Contains(md, "scim-user") || !strings.Contains(md, "https://g/scim-user") {
		t.Fatalf("markdown = %q, want user row with link", md)
	}
}
