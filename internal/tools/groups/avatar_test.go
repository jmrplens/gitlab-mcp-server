// avatar_test.go contains unit tests for group avatar uploads: base64 and
// file-path sources, multipart encoding, validation, and error statuses.
// Tests use httptest to mock the GitLab API.
package groups

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
			t.Errorf("ParseMultipartForm: %v", err)
			http.Error(w, "parse multipart form", http.StatusBadRequest)
			return
		}
		for field, files := range r.MultipartForm.File {
			if len(files) == 0 {
				t.Errorf("multipart field %q has no files", field)
				continue
			}
			gotField = field
			f, err := files[0].Open()
			if err != nil {
				t.Errorf("open multipart file: %v", err)
				continue
			}
			buf := make([]byte, 64)
			n, _ := f.Read(buf)
			_ = f.Close()
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

// makeDirs creates each directory or fails the test, so a containment fixture
// reads as one line instead of a loop that asserts.
func makeDirs(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		if err := os.Mkdir(dir, 0o750); err != nil {
			t.Fatalf("Mkdir(%q) error = %v", dir, err)
		}
	}
}

// TestUploadAvatar_FilePathOutsideAllowedDirs_Rejected verifies that the
// streaming upload path inherits the same containment as the buffered one: a
// file_path outside the allow-listed roots and a symlink pointing outside are
// refused, before any byte reaches the wire.
//
// This is the handler shape that leaked most: it copies to EOF rather than
// sizing its read from os.Stat, so a zero-size procfs entry such as
// /proc/self/environ streamed the server's whole environment, live token
// included, into a project the caller named.
func TestUploadAvatar_FilePathOutsideAllowedDirs_Rejected(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "workspace")
	outside := filepath.Join(root, "home")
	makeDirs(t, allowed, outside)
	secret := filepath.Join(outside, "id_rsa")
	if err := os.WriteFile(secret, []byte("PRIVATE KEY"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	link := filepath.Join(allowed, "pic.png")
	symlinked := os.Symlink(secret, link) == nil

	testutil.IsolateTempDir(t, allowed)
	t.Chdir(allowed)

	tests := []struct {
		name string
		path string
		skip bool
	}{
		{name: "absolute path outside every allowed root is refused", path: secret},
		{name: "parent traversal out of the workspace is refused", path: filepath.Join(allowed, "..", "home", "id_rsa")},
		{name: "symlink inside the workspace pointing outside is refused", path: link, skip: !symlinked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip("symlinks unsupported on this platform")
			}
			client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

			_, err := UploadAvatar(context.Background(), client, UploadAvatarInput{
				GroupID:  "99",
				Filename: "pic.png",
				FilePath: tt.path,
			})
			if err == nil {
				t.Fatalf("UploadAvatar(%q) error = nil, want refusal", tt.path)
			}
			if !strings.Contains(err.Error(), "outside allowed") {
				t.Errorf("UploadAvatar(%q) error = %q, want it to name the allow-list", tt.path, err)
			}
		})
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

// TestUploadAvatar_CanceledContext_ReturnsContextError covers the ctx.Err()
// guard in both avatar handlers.
func TestUploadAvatar_CanceledContext_ReturnsContextError(t *testing.T) {
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
