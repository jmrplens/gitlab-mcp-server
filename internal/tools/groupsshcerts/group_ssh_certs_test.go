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

func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/mygroup/ssh_certificates" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":1,"title":"cert-1","key":"ssh-rsa AAAA1","created_at":"2024-01-01T00:00:00Z"},
				{"id":2,"title":"cert-2","key":"ssh-rsa AAAA2","created_at":"2024-02-01T00:00:00Z"}
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

func TestList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := List(ctx, client, ListInput{GroupID: toolutil.StringOrInt("mygroup")})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

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

func TestCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/groups/mygroup/ssh_certificates" {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":10,"title":"new-cert","key":"ssh-rsa NEWKEY","created_at":"2024-03-01T00:00:00Z"}`)
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

func TestCreate_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Create(context.Background(), client, CreateInput{Key: "ssh-rsa K", Title: "t"})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

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

func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Create(ctx, client, CreateInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		Key:     "ssh-rsa K",
		Title:   "t",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

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

func TestDelete_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Delete(context.Background(), client, DeleteInput{CertificateID: 10})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

func TestDelete_MissingCertificateID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Delete(context.Background(), client, DeleteInput{GroupID: toolutil.StringOrInt("mygroup")})
	if err == nil {
		t.Fatal("expected error for zero certificate_id, got nil")
	}
}

func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Delete(ctx, client, DeleteInput{
		GroupID:       toolutil.StringOrInt("mygroup"),
		CertificateID: 10,
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

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
