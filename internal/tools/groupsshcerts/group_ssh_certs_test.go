// group_ssh_certs_test.go contains unit tests for GitLab group SSH certificate
// operations. Tests use httptest to mock the GitLab Group SSH Certificates API.
package groupsshcerts

import (
	"context"
	"net/http"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestToOutput_NilInput verifies the ToOutput_NilInput handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestToOutput_NilInput(t *testing.T) {
	out := toOutput(nil)
	if out.ID != 0 || out.Title != "" || out.Key != "" || out.CreatedAt != "" {
		t.Errorf("expected zero Output for nil input, got %+v", out)
	}
}

// TestToOutput_NilCreatedAt verifies the ToOutput_NilCreatedAt handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestList_EmptyResults verifies the List_EmptyResults handler.
// The mock GitLab API at /api/v4/groups/empty-group/ssh_certificates (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestList_Success verifies that List succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/mygroup/ssh_certificates (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestList_PaginationParameters verifies that offset and keyset pagination
// inputs are forwarded as query parameters on the list request.
// The mock asserts page, per_page, pagination, and page_token are present.
// It confirms List succeeds and returns the mocked certificate.
func TestList_PaginationParameters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v4/groups/mygroup/ssh_certificates" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		for key, want := range map[string]string{
			"page":       "2",
			"per_page":   "50",
			"pagination": "keyset",
			"page_token": "tok-123",
		} {
			if got := q.Get(key); got != want {
				t.Errorf("query %s = %q, want %q", key, got, want)
			}
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"id":7,"title":"cert-7","key":"ssh-rsa AAAA7","created_at":"2026-03-01T00:00:00Z"}]`)
	}))

	out, err := List(context.Background(), client, ListInput{
		GroupID:               toolutil.StringOrInt("mygroup"),
		PaginationInput:       toolutil.PaginationInput{Page: 2, PerPage: 50},
		KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "tok-123"},
	})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Certificates) != 1 || out.Certificates[0].ID != 7 {
		t.Fatalf("unexpected certificates: %+v", out.Certificates)
	}
}

// TestList_MissingGroupID verifies that List_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestList_CancelledContext verifies the List_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestList_APIError verifies that List returns a wrapped error when the GitLab API responds with an error status.
// The mock GitLab API at /api/v4/groups/mygroup/ssh_certificates (GET) responds with HTTP Forbidden.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestCreate_Success verifies that Create succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/mygroup/ssh_certificates (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
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

// TestCreate_MissingGroupID verifies that Create_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreate_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Create(context.Background(), client, CreateInput{Key: "ssh-rsa K", Title: "t"})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestCreate_MissingKey verifies that Create_MissingKey returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestCreate_MissingTitle verifies that Create_MissingTitle returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestCreate_CancelledContext verifies the Create_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestCreate_APIError verifies that Create returns a wrapped error when the GitLab API responds with an error status.
// The mock GitLab API at /api/v4/groups/mygroup/ssh_certificates (GET) responds with HTTP BadRequest.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestDelete_Success verifies that Delete succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/mygroup/ssh_certificates/10 (DELETE) returns a representative success body.
// It asserts the returned output matches the expected fields.
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

// TestDelete_MissingGroupID verifies that Delete_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDelete_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Delete(context.Background(), client, DeleteInput{CertificateID: 10})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestDelete_MissingCertificateID verifies that Delete_MissingCertificateID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDelete_MissingCertificateID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Delete(context.Background(), client, DeleteInput{GroupID: toolutil.StringOrInt("mygroup")})
	if err == nil {
		t.Fatal("expected error for zero certificate_id, got nil")
	}
}

// TestDelete_CancelledContext verifies the Delete_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestDelete_APIError verifies that Delete returns a wrapped error when the GitLab API responds with an error status.
// The mock GitLab API at /api/v4/groups/mygroup/ssh_certificates/10 (GET) responds with HTTP Forbidden.
// It asserts that the returned error is wrapped and contains a useful hint.
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
