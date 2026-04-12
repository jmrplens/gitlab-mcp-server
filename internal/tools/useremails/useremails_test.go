package useremails

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
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
	emailJSON          = `{"id":1,"email":"test@example.com","confirmed_at":"2024-01-15T10:00:00Z"}`
	emailListJSON      = `[{"id":1,"email":"test@example.com","confirmed_at":"2024-01-15T10:00:00Z"},{"id":2,"email":"dev@example.com"}]`
)

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

func TestListForUser_InvalidUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := ListForUser(context.Background(), client, ListForUserInput{UserID: 0})
	if err == nil {
		t.Fatal("expected error for invalid user_id, got nil")
	}
}

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

func TestGet_InvalidEmailID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(context.Background(), client, GetInput{EmailID: 0})
	if err == nil {
		t.Fatal("expected error for invalid email_id, got nil")
	}
}

func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := Get(context.Background(), client, GetInput{EmailID: 999})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

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

func TestAdd_EmptyEmail(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Add(context.Background(), client, AddInput{Email: ""})
	if err == nil {
		t.Fatal("expected error for empty email, got nil")
	}
}

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

func TestAddForUser_InvalidUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := AddForUser(context.Background(), client, AddForUserInput{UserID: 0, Email: "test@example.com"})
	if err == nil {
		t.Fatal("expected error for invalid user_id, got nil")
	}
}

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

func TestDelete_InvalidEmailID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Delete(context.Background(), client, DeleteInput{EmailID: 0})
	if err == nil {
		t.Fatal("expected error for invalid email_id, got nil")
	}
}

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

func TestFormatListMarkdownString_Empty(t *testing.T) {
	md := FormatListMarkdownString(ListOutput{})
	if md == "" {
		t.Fatal("expected non-empty markdown for empty list")
	}
}

func TestFormatMarkdownString(t *testing.T) {
	md := FormatMarkdownString(Output{ID: 1, Email: "test@example.com", ConfirmedAt: "2024-01-15"})
	if md == "" {
		t.Fatal("expected non-empty markdown")
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
		wantErr    bool
		validate   func(t *testing.T, out ListOutput)
	}{
		{
			name:       "passes pagination parameters to API",
			input:      ListForUserInput{UserID: 42, Page: 2, PerPage: 10},
			mockStatus: http.StatusOK,
			mockBody:   `[{"id":3,"email":"page2@example.com"}]`,
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
			name:    "returns error for negative user_id",
			input:   ListForUserInput{UserID: -1},
			wantErr: true,
		},
		{
			name:       "returns error on 500 API failure",
			input:      ListForUserInput{UserID: 42},
			mockStatus: http.StatusInternalServerError,
			mockBody:   `{"message":"500 Internal Server Error"}`,
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
			mockBody:   `{"id":5,"email":"admin@example.com","confirmed_at":"2024-06-01T12:00:00Z"}`,
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
			mockBody:   `{"id":10,"email":"skip@example.com","confirmed_at":"2024-03-01T08:00:00Z"}`,
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
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)
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

// TestFormatListMarkdownString_WithEmails verifies the Markdown table renders correctly
// for lists with confirmed and unconfirmed emails, including header and row content.
func TestFormatListMarkdownString_WithEmails(t *testing.T) {
	out := ListOutput{
		Emails: []Output{
			{ID: 1, Email: "confirmed@example.com", ConfirmedAt: "2024-01-15T10:00:00Z"},
			{ID: 2, Email: "unconfirmed@example.com"},
		},
	}

	md := FormatListMarkdownString(out)

	checks := []struct {
		label    string
		contains string
	}{
		{"header count", "## Emails (2)"},
		{"table header", "| ID | Email | Confirmed At |"},
		{"confirmed email row", "| 1 | confirmed@example.com | 2024-01-15T10:00:00Z |"},
		{"unconfirmed email dash", "| 2 | unconfirmed@example.com | - |"},
	}
	for _, c := range checks {
		if !strings.Contains(md, c.contains) {
			t.Errorf("%s: markdown missing %q\ngot:\n%s", c.label, c.contains, md)
		}
	}
}

// TestFormatMarkdownString_WithoutConfirmedAt verifies the Markdown output omits
// the Confirmed At line when the field is empty.
func TestFormatMarkdownString_WithoutConfirmedAt(t *testing.T) {
	md := FormatMarkdownString(Output{ID: 3, Email: "noconfirm@example.com"})

	if !strings.Contains(md, "noconfirm@example.com") {
		t.Errorf("expected email in markdown, got:\n%s", md)
	}
	if strings.Contains(md, "Confirmed At") {
		t.Errorf("Confirmed At should be omitted when empty, got:\n%s", md)
	}
}

// TestFormatDeleteMarkdownString validates the deletion confirmation Markdown output
// for both true and false deletion states.
func TestFormatDeleteMarkdownString(t *testing.T) {
	tests := []struct {
		name     string
		input    DeleteOutput
		contains []string
	}{
		{
			name:  "successful deletion",
			input: DeleteOutput{EmailID: 7, Deleted: true},
			contains: []string{
				"## Email Deleted",
				"**Email ID**: 7",
			},
		},
		{
			name:  "failed deletion",
			input: DeleteOutput{EmailID: 0, Deleted: false},
			contains: []string{
				"## Email Deleted",
				"**Email ID**: 0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := FormatDeleteMarkdownString(tt.input)
			for _, want := range tt.contains {
				if !strings.Contains(md, want) {
					t.Errorf("markdown missing %q\ngot:\n%s", want, md)
				}
			}
		})
	}
}
