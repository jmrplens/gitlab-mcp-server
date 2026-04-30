// group_ssh_certs_test.go contains unit tests for GitLab group SSH certificate
// operations. Tests use httptest to mock the GitLab Group SSH Certificates API.
package groupsshcerts

import (
	"context"
	"net/http"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestToOutput_NilInput verifies that toOutput returns a zero-value Output
// when given a nil GroupSSHCertificate pointer.
func TestToOutput_NilInput(t *testing.T) {
	out := toOutput(nil)
	if out.ID != 0 || out.Title != "" || out.Key != "" || out.CreatedAt != "" {
		t.Errorf("expected zero Output for nil input, got %+v", out)
	}
}

// TestToOutput_NilCreatedAt verifies that toOutput leaves CreatedAt empty
// when the source certificate has a nil CreatedAt field.
func TestToOutput_NilCreatedAt(t *testing.T) {
	cert := &gl.GroupSSHCertificate{
		ID:    42,
		Title: "test-cert",
		Key:   "ssh-rsa AAAA",
	}
	out := toOutput(cert)
	if out.ID != 42 {
		t.Errorf("ID = %d, want 42", out.ID)
	}
	if out.Title != "test-cert" {
		t.Errorf("Title = %q, want %q", out.Title, "test-cert")
	}
	if out.CreatedAt != "" {
		t.Errorf("CreatedAt = %q, want empty for nil time", out.CreatedAt)
	}
}

// TestList_EmptyResults verifies that List returns an empty certificates slice
// when the API returns an empty array, rather than nil.
func TestList_EmptyResults(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/empty-group/ssh_certificates" {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		GroupID: toolutil.StringOrInt("empty-group"),
	})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if out.Certificates == nil {
		t.Fatal("expected non-nil certificates slice, got nil")
	}
	if len(out.Certificates) != 0 {
		t.Errorf("expected 0 certificates, got %d", len(out.Certificates))
	}
}

// TestList_Success verifies that List returns the expected output when the GitLab API responds successfully.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/mygroup/ssh_certificates" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":1,"title":"cert-1","key":"ssh-rsa AAAA1","created_at":"2026-01-01T00:00:00Z"},
				{"id":2,"title":"cert-2","key":"ssh-rsa AAAA2","created_at":"2026-02-01T00:00:00Z"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		GroupID: toolutil.StringOrInt("mygroup"),
	})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Certificates) != 2 {
		t.Fatalf("expected 2 certificates, got %d", len(out.Certificates))
	}
	if out.Certificates[0].Title != "cert-1" {
		t.Errorf("expected title cert-1, got %s", out.Certificates[0].Title)
	}
	if out.Certificates[1].ID != 2 {
		t.Errorf("expected id 2, got %d", out.Certificates[1].ID)
	}
}

// TestList_MissingGroupID verifies that List returns a validation error when group_id is missing.
func TestList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestList_CancelledContext verifies that List returns an error when the context is already cancelled.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := List(ctx, client, ListInput{GroupID: toolutil.StringOrInt("mygroup")})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestList_APIError verifies that List returns an error when the GitLab API responds with a failure status.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/ssh_certificates" {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{
		GroupID: toolutil.StringOrInt("mygroup"),
	})
	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}
}

// TestCreate_Success verifies that Create returns the expected output when the GitLab API responds successfully.
func TestCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/groups/mygroup/ssh_certificates" {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":10,"title":"new-cert","key":"ssh-rsa NEWKEY","created_at":"2026-03-01T00:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		Key:     "ssh-rsa NEWKEY",
		Title:   "new-cert",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if out.ID != 10 {
		t.Errorf("expected id 10, got %d", out.ID)
	}
	if out.Title != "new-cert" {
		t.Errorf("expected title new-cert, got %s", out.Title)
	}
}

// TestCreate_MissingGroupID verifies that Create returns a validation error when group_id is missing.
func TestCreate_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Create(context.Background(), client, CreateInput{Key: "ssh-rsa K", Title: "t"})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestCreate_MissingKey verifies that Create returns a validation error when key is missing.
func TestCreate_MissingKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		Title:   "t",
	})
	if err == nil {
		t.Fatal("expected error for empty key, got nil")
	}
}

// TestCreate_MissingTitle verifies that Create returns a validation error when title is missing.
func TestCreate_MissingTitle(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		Key:     "ssh-rsa K",
	})
	if err == nil {
		t.Fatal("expected error for empty title, got nil")
	}
}

// TestCreate_CancelledContext verifies that Create returns an error when the context is already cancelled.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Create(ctx, client, CreateInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		Key:     "ssh-rsa K",
		Title:   "t",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestCreate_APIError verifies that Create returns an error when the GitLab API responds with a failure status.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/ssh_certificates" {
			testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"Bad Request"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		Key:     "ssh-rsa K",
		Title:   "t",
	})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

// TestDelete_Success verifies that Delete returns the expected output when the GitLab API responds successfully.
func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/groups/mygroup/ssh_certificates/10" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		GroupID:       toolutil.StringOrInt("mygroup"),
		CertificateID: 10,
	})
	if err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
}

// TestDelete_MissingGroupID verifies that Delete returns a validation error when group_id is missing.
func TestDelete_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Delete(context.Background(), client, DeleteInput{CertificateID: 10})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestDelete_MissingCertificateID verifies that Delete returns a validation error when certificate_id is missing.
func TestDelete_MissingCertificateID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Delete(context.Background(), client, DeleteInput{GroupID: toolutil.StringOrInt("mygroup")})
	if err == nil {
		t.Fatal("expected error for zero certificate_id, got nil")
	}
}

// TestDelete_CancelledContext verifies that Delete returns an error when the context is already cancelled.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	err := Delete(ctx, client, DeleteInput{
		GroupID:       toolutil.StringOrInt("mygroup"),
		CertificateID: 10,
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestDelete_APIError verifies that Delete returns an error when the GitLab API responds with a failure status.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/ssh_certificates/10" {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		GroupID:       toolutil.StringOrInt("mygroup"),
		CertificateID: 10,
	})
	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}
}
