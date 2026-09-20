// user_emails_test.go contains unit tests for GitLab user email operations.
// Tests use httptest to mock the GitLab User Emails API.
package useremails

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	errExpAPIFailure   = "expected error for API failure, got nil"
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

// TestListForUser_InvalidUserID verifies that ListForUser refuses user_id 0
// itself. The mock forbids every request, so the refusal asserted here is the
// handler's and not a 404 GitLab would have answered: with a mock that replies
// at all, relaxing the guard to `< 0` leaves the test green because zero
// reaches GitLab and comes back an error anyway.
func TestListForUser_InvalidUserID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := ListForUser(context.Background(), client, ListForUserInput{UserID: 0})
	assertRefusedWith(t, err, errUserIDPositive)
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
	if want := "2026-01-15T10:00:00Z"; out.ConfirmedAt != want {
		t.Errorf("out.ConfirmedAt = %q, want %q", out.ConfirmedAt, want)
	}
}

// TestGet_InvalidEmailID verifies that Get refuses email_id 0 itself, before
// any request leaves for GitLab.
func TestGet_InvalidEmailID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Get(context.Background(), client, GetInput{EmailID: 0})
	assertRefusedWith(t, err, errEmailIDPositive)
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

// TestGet_ConfirmedAtFromAnInstanceOutsideUTC verifies that a confirmation an
// instance timestamps in its own zone is published as the instant it names.
// The layout this replaced ended in a literal "Z" and converted nothing, so
// GitLab's 10:00:00+01:00 was republished as 10:00:00Z: an hour away from the
// instant meant, under a label saying otherwise. Every fixture in this file
// was already UTC, so nothing here could see it.
func TestGet_ConfirmedAtFromAnInstanceOutsideUTC(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"email":"zoned@example.com","confirmed_at":"2026-01-15T10:00:00+01:00"}`)
	}))

	out, err := Get(context.Background(), client, GetInput{EmailID: 1})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if want := "2026-01-15T09:00:00Z"; out.ConfirmedAt != want {
		t.Errorf("out.ConfirmedAt = %q, want %q", out.ConfirmedAt, want)
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

// TestAdd_EmptyEmail verifies that Add refuses an empty email itself rather
// than posting a body without one and letting GitLab answer.
func TestAdd_EmptyEmail(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Add(context.Background(), client, AddInput{Email: ""})
	assertRefusedWith(t, err, "email is required")
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

// decodeEmailRequestBody reads the JSON body the handler built. It decodes
// into a map rather than a struct because half of what these tests assert is
// that a key is absent: skip_confirmation is a pointer with omitempty, so a
// flag the caller left false and a flag the handler forgot to send are the
// same struct and differ only by whether the key is on the wire.
func decodeEmailRequestBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	body := map[string]any{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Errorf("decode request body: %v", err)
		return nil
	}
	return body
}

// assertEmailBody holds a POST body to the address and the flag the caller
// named, absence of the flag included.
func assertEmailBody(t *testing.T, body map[string]any, wantEmail string, wantSkip bool) {
	t.Helper()
	if got := body["email"]; got != wantEmail {
		t.Errorf("email sent = %v, want %q", got, wantEmail)
	}
	got, present := body["skip_confirmation"]
	switch {
	case wantSkip && got != true:
		t.Errorf("skip_confirmation sent = %v (present %t), want true", got, present)
	case !wantSkip && present:
		t.Errorf("skip_confirmation sent = %v; a caller who did not ask for it sends no key", got)
	}
}

// TestAdd_SendsTheEmailAndTheConfirmationFlag verifies that what Add puts on
// the wire is what the caller asked for. No test in this package read the
// request body, so a handler posting an empty object passed every one of them:
// emptying the options struct leaves the whole suite green.
func TestAdd_SendsTheEmailAndTheConfirmationFlag(t *testing.T) {
	tests := []struct {
		name  string
		input AddInput
	}{
		{name: "with skip_confirmation", input: AddInput{Email: "admin@example.com", SkipConfirmation: true}},
		{name: "without skip_confirmation", input: AddInput{Email: "plain@example.com"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPost)
				testutil.AssertRequestPath(t, r, pathAddEmail)
				assertEmailBody(t, decodeEmailRequestBody(t, r), tt.input.Email, tt.input.SkipConfirmation)
				testutil.RespondJSON(w, http.StatusCreated, emailJSON)
			}))

			if _, err := Add(context.Background(), client, tt.input); err != nil {
				t.Fatalf("Add() unexpected error: %v", err)
			}
		})
	}
}

// TestAddForUser_SendsTheEmailAndTheConfirmationFlag is the admin half of the
// same property: the address and the flag reach the user's own collection, at
// the path naming that user.
func TestAddForUser_SendsTheEmailAndTheConfirmationFlag(t *testing.T) {
	tests := []struct {
		name  string
		input AddForUserInput
	}{
		{name: "with skip_confirmation", input: AddForUserInput{UserID: 42, Email: "skip@example.com", SkipConfirmation: true}},
		{name: "without skip_confirmation", input: AddForUserInput{UserID: 42, Email: "plain@example.com"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPost)
				testutil.AssertRequestPath(t, r, pathAddEmailUser)
				assertEmailBody(t, decodeEmailRequestBody(t, r), tt.input.Email, tt.input.SkipConfirmation)
				testutil.RespondJSON(w, http.StatusCreated, emailJSON)
			}))

			if _, err := AddForUser(context.Background(), client, tt.input); err != nil {
				t.Fatalf("AddForUser() unexpected error: %v", err)
			}
		})
	}
}

// TestAddForUser_InvalidUserID verifies that AddForUser refuses user_id 0
// itself, so a valid email is never posted to /users/0/emails.
func TestAddForUser_InvalidUserID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := AddForUser(context.Background(), client, AddForUserInput{UserID: 0, Email: "test@example.com"})
	assertRefusedWith(t, err, errUserIDPositive)
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

// TestDelete_InvalidEmailID verifies that Delete refuses email_id 0 itself.
// A DELETE that reaches GitLab is the one refusal that cannot be taken back,
// so the mock forbids every request rather than answering one.
func TestDelete_InvalidEmailID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Delete(context.Background(), client, DeleteInput{EmailID: 0})
	assertRefusedWith(t, err, errEmailIDPositive)
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
// confirmation, which is FormatTime's second parse and the shape a caller who
// built the output itself may hand the formatter. No handler here produces
// one: toOutput formats every confirmation as RFC 3339 in UTC.
func TestFormatMarkdownString(t *testing.T) {
	assertMarkdown(t, FormatMarkdownString(Output{ID: 1, Email: "test@example.com", ConfirmedAt: "2026-01-15"}),
		"## Email\n\n"+
			"- **ID**: 1\n"+
			"- **Email**: test@example.com\n"+
			"- **Confirmed**: ✅ 15 Jan 2026\n")
}

// assertRefusedWith holds a refusal to the message the handler itself
// produces. Asserting only that some error came back cannot tell the handler's
// own validation from a status the mock happened to answer, which is what let
// every `<= 0` guard here be relaxed to `< 0` with the suite still green.
func assertRefusedWith(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected the handler to refuse with %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to carry %q", err.Error(), want)
	}
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
			name:       "returns error on 403 API failure",
			input:      ListForUserInput{UserID: 42},
			mockStatus: http.StatusForbidden,
			mockBody:   `{"message":"403 Forbidden"}`,
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
			// A case that names no status is one the handler has to refuse on
			// its own, so the mock forbids every request instead of answering
			// a 404 the assertion could not tell from the handler's refusal.
			handler := testutil.ForbiddenHandler(t)
			if tt.mockStatus > 0 {
				handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assertQuery(t, r, tt.wantQuery)
					testutil.RespondJSON(w, tt.mockStatus, tt.mockBody)
				})
			}
			client := testutil.NewTestClient(t, handler)

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

// TestAdd_TableDriven validates what Add returns: the created email on a 201,
// and an error on a 422 GitLab refused. It asserts nothing about the request
// body, which the mock here discards; what reaches GitLab is held by
// TestAdd_SendsTheEmailAndTheConfirmationFlag.
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
			name:       "returns the created email when skip_confirmation is set",
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

// TestAddForUser_TableDriven validates what AddForUser returns and what it
// refuses: an empty email and a non-positive user_id before any request, the
// created email on a 201, and an error on a 403. The request body is held by
// TestAddForUser_SendsTheEmailAndTheConfirmationFlag instead.
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
			name:       "returns the created email when skip_confirmation is set",
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
			// As in TestListForUser_TableDriven: a case with no status is one
			// the handler must refuse before it builds a request.
			handler := testutil.ForbiddenHandler(t)
			if tt.mockStatus > 0 {
				handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondJSON(w, tt.mockStatus, tt.mockBody)
				})
			}
			client := testutil.NewTestClient(t, handler)

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

// TestDelete_APIError verifies Delete returns an error when GitLab refuses the
// deletion with a 403, which is what a token without the rights to the address
// is answered.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
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
			// The four identifier cases name no status: a DELETE that leaves
			// for GitLab with a zero or negative id is the defect, so the mock
			// forbids every request rather than answering one.
			handler := testutil.ForbiddenHandler(t)
			if tt.mockStatus > 0 {
				handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondJSON(w, tt.mockStatus, tt.mockBody)
				})
			}
			client := testutil.NewTestClient(t, handler)

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
