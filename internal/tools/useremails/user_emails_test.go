// user_emails_test.go contains unit tests for GitLab user email operations.
// Tests use httptest to mock the GitLab User Emails API.
package useremails

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	errExpAPIFailure   = "expected error for API failure, got nil"
	errExpValidation   = "expected validation error, got nil"
	pathEmailsForUser  = "/api/v4/users/42/emails"
	pathGetEmail       = "/api/v4/user/emails/1"
	pathAddEmail       = "/api/v4/user/emails"
	pathAddEmailUser   = "/api/v4/users/42/emails"
	pathDeleteEmail    = "/api/v4/user/emails/1"
	pathDeleteEmailUsr = "/api/v4/users/42/emails/1"
	emailJSON          = `{"id":1,"email":"test@example.com","confirmed_at":"2026-01-15T10:00:00Z"}`
	emailListJSON      = `[{"id":1,"email":"test@example.com","confirmed_at":"2026-01-15T10:00:00Z"},{"id":2,"email":"dev@example.com"}]`
)

// TestListForUser_Success verifies that ListForUser returns the expected output when the GitLab API responds successfully.
func TestListForUser_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathEmailsForUser {
			testutil.RespondJSON(w, http.StatusOK, emailListJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListForUser(context.Background(), client, ListForUserInput{UserID: 42})
	if err != nil {
		t.Fatalf("ListForUser() unexpected error: %v", err)
	}
	if len(out.Emails) != 2 {
		t.Fatalf("len(out.Emails) = %d, want 2", len(out.Emails))
	}
	if out.Emails[0].Email != "test@example.com" {
		t.Errorf("out.Emails[0].Email = %q, want %q", out.Emails[0].Email, "test@example.com")
	}
}

// TestListForUser_InvalidUserID verifies that ListForUser returns a validation error when user_id is invalid.
func TestListForUser_InvalidUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := ListForUser(context.Background(), client, ListForUserInput{UserID: 0})
	if err == nil {
		t.Fatal("expected error for invalid user_id, got nil")
	}
}

// TestGet_Success verifies that Get returns the expected output when the GitLab API responds successfully.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGetEmail {
			testutil.RespondJSON(w, http.StatusOK, emailJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{EmailID: 1})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("out.ID = %d, want 1", out.ID)
	}
	if out.Email != "test@example.com" {
		t.Errorf("out.Email = %q, want %q", out.Email, "test@example.com")
	}
}

// TestGet_InvalidEmailID verifies that Get returns a validation error when email_id is invalid.
func TestGet_InvalidEmailID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(context.Background(), client, GetInput{EmailID: 0})
	if err == nil {
		t.Fatal("expected error for invalid email_id, got nil")
	}
}

// TestGet_APIError verifies that Get returns an error when the GitLab API responds with a failure status.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := Get(context.Background(), client, GetInput{EmailID: 999})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestAdd_Success verifies that Add returns the expected output when the GitLab API responds successfully.
func TestAdd_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathAddEmail {
			testutil.RespondJSON(w, http.StatusCreated, emailJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Add(context.Background(), client, AddInput{Email: "test@example.com"})
	if err != nil {
		t.Fatalf("Add() unexpected error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("out.ID = %d, want 1", out.ID)
	}
}

// TestAdd_EmptyEmail verifies that Add returns a validation error when email is empty.
func TestAdd_EmptyEmail(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Add(context.Background(), client, AddInput{Email: ""})
	if err == nil {
		t.Fatal("expected error for empty email, got nil")
	}
}

// TestAddForUser_Success verifies that AddForUser returns the expected output when the GitLab API responds successfully.
func TestAddForUser_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathAddEmailUser {
			testutil.RespondJSON(w, http.StatusCreated, emailJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := AddForUser(context.Background(), client, AddForUserInput{UserID: 42, Email: "test@example.com"})
	if err != nil {
		t.Fatalf("AddForUser() unexpected error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("out.ID = %d, want 1", out.ID)
	}
}

// TestAddForUser_InvalidUserID verifies that AddForUser returns a validation error when user_id is invalid.
func TestAddForUser_InvalidUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := AddForUser(context.Background(), client, AddForUserInput{UserID: 0, Email: "test@example.com"})
	if err == nil {
		t.Fatal("expected error for invalid user_id, got nil")
	}
}

// TestDelete_Success verifies that Delete returns the expected output when the GitLab API responds successfully.
func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathDeleteEmail {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Delete(context.Background(), client, DeleteInput{EmailID: 1})
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
	if !out.Deleted {
		t.Error("out.Deleted = false, want true")
	}
}

// TestDelete_InvalidEmailID verifies that Delete returns a validation error when email_id is invalid.
func TestDelete_InvalidEmailID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Delete(context.Background(), client, DeleteInput{EmailID: 0})
	if err == nil {
		t.Fatal("expected error for invalid email_id, got nil")
	}
}

// TestDeleteForUser_Success verifies that DeleteForUser returns the expected output when the GitLab API responds successfully.
func TestDeleteForUser_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathDeleteEmailUsr {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := DeleteForUser(context.Background(), client, DeleteForUserInput{UserID: 42, EmailID: 1})
	if err != nil {
		t.Fatalf("DeleteForUser() unexpected error: %v", err)
	}
	if !out.Deleted {
		t.Error("out.Deleted = false, want true")
	}
}

// TestFormatMarkdownString verifies the card rendered from a date-only
// confirmation, which GitLab sends on the list-for-user route.
func TestFormatMarkdownString(t *testing.T) {
	assertMarkdown(t, FormatMarkdownString(Output{ID: 1, Email: "test@example.com", ConfirmedAt: "2026-01-15"}),
		"## Email\n\n"+
			"- **ID**: 1\n"+
			"- **Email**: test@example.com\n"+
			"- **Confirmed**: ✅ 15 Jan 2026\n")
}

// assertQuery fails the test if any expected query parameter is missing or
// does not match the value observed on the inbound request.
func assertQuery(t *testing.T, r *http.Request, want map[string]string) {
	t.Helper()
	for key, value := range want {
		if got := r.URL.Query().Get(key); got != value {
			t.Errorf("query %q = %q, want %q", key, got, value)
		}
	}
}

// TestListForUser_TableDriven validates ListForUser across pagination parameters,
// negative user IDs, and API failure scenarios.
func TestListForUser_TableDriven(t *testing.T) {
	tests := []struct {
		name       string
		input      ListForUserInput
		mockStatus int
		mockBody   string
		wantQuery  map[string]string
		wantErr    bool
		validate   func(t *testing.T, out ListOutput)
	}{
		{
			name: "passes pagination parameters to API",
			input: ListForUserInput{
				UserID:          42,
				PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 10},
			},
			mockStatus: http.StatusOK,
			mockBody:   `[{"id":3,"email":"page2@example.com"}]`,
			wantQuery:  map[string]string{"page": "2", "per_page": "10"},
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Emails) != 1 {
					t.Fatalf("len(Emails) = %d, want 1", len(out.Emails))
				}
				if out.Emails[0].Email != "page2@example.com" {
					t.Errorf("Email = %q, want %q", out.Emails[0].Email, "page2@example.com")
				}
			},
		},
		{
			name: "passes keyset, order_by and sort parameters to API",
			input: ListForUserInput{
				UserID:                42,
				KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "100"},
				OrderBy:               "id",
				Sort:                  "desc",
			},
			mockStatus: http.StatusOK,
			mockBody:   `[{"id":3,"email":"keyset@example.com"}]`,
			wantQuery: map[string]string{
				"pagination": "keyset", "page_token": "100", "order_by": "id", "sort": "desc",
			},
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Emails) != 1 {
					t.Fatalf("len(Emails) = %d, want 1", len(out.Emails))
				}
			},
		},
		{
			name:    "returns error for negative user_id",
			input:   ListForUserInput{UserID: -1},
			wantErr: true,
		},
		{
			name:       "returns error on 500 API failure",
			input:      ListForUserInput{UserID: 42},
			mockStatus: http.StatusForbidden,
			mockBody:   `{"message":"server error"}`,
			wantErr:    true,
		},
		{
			name:       "handles empty email list",
			input:      ListForUserInput{UserID: 42},
			mockStatus: http.StatusOK,
			mockBody:   `[]`,
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Emails) != 0 {
					t.Fatalf("len(Emails) = %d, want 0", len(out.Emails))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assertQuery(t, r, tt.wantQuery)
				if tt.mockStatus > 0 {
					testutil.RespondJSON(w, tt.mockStatus, tt.mockBody)
					return
				}
				http.NotFound(w, r)
			}))

			out, err := ListForUser(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ListForUser() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestAdd_TableDriven validates Add across skip_confirmation flag, API errors,
// and various input combinations.
func TestAdd_TableDriven(t *testing.T) {
	tests := []struct {
		name       string
		input      AddInput
		mockStatus int
		mockBody   string
		wantErr    bool
		validate   func(t *testing.T, out Output)
	}{
		{
			name:       "adds email with skip_confirmation",
			input:      AddInput{Email: "admin@example.com", SkipConfirmation: true},
			mockStatus: http.StatusCreated,
			mockBody:   `{"id":5,"email":"admin@example.com","confirmed_at":"2026-06-01T12:00:00Z"}`,
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if out.ID != 5 {
					t.Errorf("ID = %d, want 5", out.ID)
				}
				if out.ConfirmedAt == "" {
					t.Error("ConfirmedAt should not be empty when skip_confirmation is used")
				}
			},
		},
		{
			name:       "returns error on 422 unprocessable entity",
			input:      AddInput{Email: "invalid"},
			mockStatus: http.StatusUnprocessableEntity,
			mockBody:   `{"message":"Email is invalid"}`,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPost)
				testutil.RespondJSON(w, tt.mockStatus, tt.mockBody)
			}))

			out, err := Add(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Add() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestAddForUser_TableDriven validates AddForUser covering empty email, skip_confirmation,
// and API failure paths.
func TestAddForUser_TableDriven(t *testing.T) {
	tests := []struct {
		name       string
		input      AddForUserInput
		mockStatus int
		mockBody   string
		wantErr    bool
		validate   func(t *testing.T, out Output)
	}{
		{
			name:    "returns error for empty email",
			input:   AddForUserInput{UserID: 42, Email: ""},
			wantErr: true,
		},
		{
			name:       "adds email with skip_confirmation for user",
			input:      AddForUserInput{UserID: 42, Email: "skip@example.com", SkipConfirmation: true},
			mockStatus: http.StatusCreated,
			mockBody:   `{"id":10,"email":"skip@example.com","confirmed_at":"2026-03-01T08:00:00Z"}`,
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if out.ID != 10 {
					t.Errorf("ID = %d, want 10", out.ID)
				}
				if out.Email != "skip@example.com" {
					t.Errorf("Email = %q, want %q", out.Email, "skip@example.com")
				}
			},
		},
		{
			name:       "returns error on 403 forbidden",
			input:      AddForUserInput{UserID: 42, Email: "test@example.com"},
			mockStatus: http.StatusForbidden,
			mockBody:   `{"message":"403 Forbidden"}`,
			wantErr:    true,
		},
		{
			name:    "returns error for negative user_id",
			input:   AddForUserInput{UserID: -5, Email: "test@example.com"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.mockStatus > 0 {
					testutil.RespondJSON(w, tt.mockStatus, tt.mockBody)
					return
				}
				http.NotFound(w, r)
			}))

			out, err := AddForUser(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("AddForUser() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestDelete_APIError verifies Delete returns an error when the GitLab API responds
// with a server error status code.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := Delete(context.Background(), client, DeleteInput{EmailID: 1})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestDeleteForUser_TableDriven validates DeleteForUser covering invalid user_id,
// invalid email_id, and API failure scenarios.
func TestDeleteForUser_TableDriven(t *testing.T) {
	tests := []struct {
		name       string
		input      DeleteForUserInput
		mockStatus int
		mockBody   string
		wantErr    bool
	}{
		{
			name:    "returns error for zero user_id",
			input:   DeleteForUserInput{UserID: 0, EmailID: 1},
			wantErr: true,
		},
		{
			name:    "returns error for negative user_id",
			input:   DeleteForUserInput{UserID: -1, EmailID: 1},
			wantErr: true,
		},
		{
			name:    "returns error for zero email_id",
			input:   DeleteForUserInput{UserID: 42, EmailID: 0},
			wantErr: true,
		},
		{
			name:    "returns error for negative email_id",
			input:   DeleteForUserInput{UserID: 42, EmailID: -3},
			wantErr: true,
		},
		{
			name:       "returns error on 404 not found",
			input:      DeleteForUserInput{UserID: 42, EmailID: 999},
			mockStatus: http.StatusNotFound,
			mockBody:   `{"message":"404 Not Found"}`,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.mockStatus > 0 {
					testutil.RespondJSON(w, tt.mockStatus, tt.mockBody)
					return
				}
				http.NotFound(w, r)
			}))

			_, err := DeleteForUser(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("DeleteForUser() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

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

// TestFormatListMarkdownString_WithEmails verifies the whole list render: the
// heading and its count, the three columns, the confirmation state of each
// address in the words a reader needs, and the guidance section naming the
// canonical action ID.
func TestFormatListMarkdownString_WithEmails(t *testing.T) {
	out := ListOutput{
		Emails: []Output{
			{ID: 1, Email: "confirmed@example.com", ConfirmedAt: "2026-01-15T10:00:00Z"},
			{ID: 2, Email: "unconfirmed@example.com"},
		},
	}

	assertMarkdown(t, FormatListMarkdownString(out),
		"## Emails (2)\n\n"+
			"| ID | Email | Confirmed |\n"+
			"| --- | --- | --- |\n"+
			"| 1 | confirmed@example.com | ✅ 15 Jan 2026 10:00 UTC |\n"+
			"| 2 | unconfirmed@example.com | ❌ awaiting confirmation |\n"+
			"\n---\n💡 **Next steps:**\n"+
			"- Use action 'user.get_email' to read one address\n")
}

// TestFormatListMarkdownString_Empty verifies that a list with nothing in it
// is the one sentence and nothing else: no heading counting zero above it.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	assertMarkdown(t, FormatListMarkdownString(ListOutput{}), "No emails found.\n")
}

// TestFormatMarkdownString_WithoutConfirmedAt verifies that an address GitLab
// has not confirmed says so. The card used to write no row at all for it,
// which reads as an address whose state GitLab did not report rather than as
// one waiting for its confirmation mail.
func TestFormatMarkdownString_WithoutConfirmedAt(t *testing.T) {
	assertMarkdown(t, FormatMarkdownString(Output{ID: 3, Email: "noconfirm@example.com"}),
		"## Email\n\n"+
			"- **ID**: 3\n"+
			"- **Email**: noconfirm@example.com\n"+
			"- **Confirmed**: ❌ awaiting confirmation\n")
}

// TestFormatMarkdownString_Confirmed verifies the confirmed card: the instant
// in the display form, behind the tick that says the address is usable.
func TestFormatMarkdownString_Confirmed(t *testing.T) {
	assertMarkdown(t, FormatMarkdownString(Output{ID: 4, Email: "ok@example.com", ConfirmedAt: "2026-01-15T10:00:00Z"}),
		"## Email\n\n"+
			"- **ID**: 4\n"+
			"- **Email**: ok@example.com\n"+
			"- **Confirmed**: ✅ 15 Jan 2026 10:00 UTC\n")
}

// TestFormatDeleteMarkdownString validates the deletion confirmation for both
// deletion states.
func TestFormatDeleteMarkdownString(t *testing.T) {
	tests := []struct {
		name  string
		input DeleteOutput
		want  string
	}{
		{
			name:  "successful deletion",
			input: DeleteOutput{EmailID: 7, Deleted: true},
			want:  "## Email Deleted\n\n- **Email ID**: 7\n- **Deleted**: ✅\n",
		},
		{
			name:  "failed deletion",
			input: DeleteOutput{EmailID: 0, Deleted: false},
			want:  "## Email Deleted\n\n- **Email ID**: 0\n- **Deleted**: ❌\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertMarkdown(t, FormatDeleteMarkdownString(tt.input), tt.want)
		})
	}
}
