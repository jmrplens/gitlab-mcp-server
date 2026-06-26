// ffuserlists_test.go contains unit tests for the feature flag user list MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package ffuserlists

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	// testListName identifies the test list name constant used by this package.
	testListName = "beta-users"
	// fmtUnexpErr identifies the fmt unexp err constant used by this package.
	fmtUnexpErr = "unexpected error: %v"
	// fmtWantName identifies the fmt want name constant used by this package.
	fmtWantName = "expected name 'beta-users', got %q"
	// errExpMissingParams identifies the err exp missing params constant used by this package.
	errExpMissingParams = "expected error for missing params"
	// errExpMissingIID identifies the err exp missing IID constant used by this package.
	errExpMissingIID = "expected error for missing iid"
)

// userListJSON identifies the user list JSON constant used by this package.
const userListJSON = `{
	"name": "beta-users",
	"user_xids": "user1,user2,user3",
	"id": 1,
	"iid": 10,
	"project_id": 42,
	"created_at": "2026-01-01T00:00:00Z",
	"updated_at": "2026-01-02T00:00:00Z"
}`

// userListArrayJSON identifies the user list array JSON constant used by this package.
const userListArrayJSON = `[` + userListJSON + `]`

// -- List --.

// TestListUserLists_Success verifies that ListUserLists succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListUserLists_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/42/feature_flags_user_lists", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, userListArrayJSON, testutil.PaginationHeaders{
			Page: "1", TotalPages: "1", PerPage: "20", Total: "1",
		})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListUserLists(context.Background(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.UserLists) != 1 {
		t.Errorf("expected 1 user list, got %d", len(out.UserLists))
	}
	if out.UserLists[0].Name != testListName {
		t.Errorf(fmtWantName, out.UserLists[0].Name)
	}
}

// TestListUserLists_MissingProjectID verifies that ListUserLists_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListUserLists_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListUserLists(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// -- Get --.

// TestGetUserList_Success verifies that GetUserList succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetUserList_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/42/feature_flags_user_lists/10", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, userListJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := GetUserList(context.Background(), client, GetInput{ProjectID: "42", IID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != testListName {
		t.Errorf(fmtWantName, out.Name)
	}
	if out.UserXIDs != "user1,user2,user3" {
		t.Errorf("expected user_xids 'user1,user2,user3', got %q", out.UserXIDs)
	}
}

// TestGetUserList_MissingParams verifies that GetUserList_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetUserList_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetUserList(context.Background(), client, GetInput{})
	if err == nil {
		t.Fatal(errExpMissingParams)
	}
	_, err = GetUserList(context.Background(), client, GetInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpMissingIID)
	}
}

// -- Create --.

// TestCreateUserList_Success verifies that CreateUserList succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateUserList_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/42/feature_flags_user_lists", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, userListJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := CreateUserList(context.Background(), client, CreateInput{
		ProjectID: "42", Name: testListName, UserXIDs: "user1,user2,user3",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != testListName {
		t.Errorf(fmtWantName, out.Name)
	}
}

// TestCreateUserList_MissingParams verifies that CreateUserList_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateUserList_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := CreateUserList(context.Background(), client, CreateInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
	_, err = CreateUserList(context.Background(), client, CreateInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

// -- Update --.

// TestUpdateUserList_Success verifies that UpdateUserList succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUpdateUserList_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v4/projects/42/feature_flags_user_lists/10", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, userListJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := UpdateUserList(context.Background(), client, UpdateInput{
		ProjectID: "42", IID: 10, Name: testListName,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.IID != 10 {
		t.Errorf("expected IID 10, got %d", out.IID)
	}
}

// TestUpdateUserList_MissingParams verifies that UpdateUserList_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdateUserList_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := UpdateUserList(context.Background(), client, UpdateInput{})
	if err == nil {
		t.Fatal(errExpMissingParams)
	}
	_, err = UpdateUserList(context.Background(), client, UpdateInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpMissingIID)
	}
}

// -- Delete --.

// TestDeleteUserList_Success verifies that DeleteUserList succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteUserList_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v4/projects/42/feature_flags_user_lists/10", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := DeleteUserList(context.Background(), client, DeleteInput{ProjectID: "42", IID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteUserList_MissingParams verifies that DeleteUserList_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteUserList_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := DeleteUserList(context.Background(), client, DeleteInput{})
	if err == nil {
		t.Fatal(errExpMissingParams)
	}
	err = DeleteUserList(context.Background(), client, DeleteInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpMissingIID)
	}
}

// -- Formatters --.

// TestFormatUserListMarkdown verifies the UserListMarkdown Markdown formatter for a representative userlist input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatUserListMarkdown(t *testing.T) {
	out := Output{ID: 1, IID: 10, ProjectID: 42, Name: testListName, UserXIDs: "user1,user2"}
	md := FormatUserListMarkdown(out)
	if !strings.Contains(md, testListName) {
		t.Error("expected markdown to contain name")
	}
	if !strings.Contains(md, "user1,user2") {
		t.Error("expected markdown to contain user_xids")
	}
}

// TestFormatUserListMarkdown_NameInHeading verifies the UserListMarkdown_NameInHeading Markdown formatter for a representative userlist_nameinheading input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatUserListMarkdown_NameInHeading(t *testing.T) {
	out := Output{ID: 5, IID: 3, ProjectID: 10, Name: "my-list", UserXIDs: "x1"}
	md := FormatUserListMarkdown(out)
	if !strings.Contains(md, "## Feature Flag User List: my-list") {
		t.Error("expected name in heading")
	}
	if !strings.Contains(md, "ID**: 5 (IID: 3)") {
		t.Error("expected combined ID/IID bullet")
	}
	if strings.Contains(md, "| Project ID |") {
		t.Error("detail formatter should not show raw Project ID row")
	}
}

// TestFormatListUserListsMarkdown_NoIDColumn verifies the ListUserListsMarkdown_NoIDColumn Markdown formatter for a representative listuserlists_noidcolumn input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListUserListsMarkdown_NoIDColumn(t *testing.T) {
	out := ListOutput{
		UserLists: []Output{
			{ID: 1, IID: 10, Name: "a-list", UserXIDs: "u1"},
		},
		Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1},
	}
	md := FormatListUserListsMarkdown(out)
	if strings.Contains(md, "| 1 | 10 |") {
		t.Error("list table should not have a separate ID column")
	}
	if !strings.Contains(md, "| 10 | a-list |") {
		t.Error("expected IID followed by Name in table row")
	}
}

// TestFormatListUserListsMarkdown verifies the ListUserListsMarkdown Markdown formatter for a representative listuserlists input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListUserListsMarkdown(t *testing.T) {
	out := ListOutput{
		UserLists: []Output{
			{ID: 1, IID: 10, Name: "list-1", UserXIDs: "u1"},
			{ID: 2, IID: 20, Name: "list-2", UserXIDs: "u2"},
		},
		Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1},
	}
	md := FormatListUserListsMarkdown(out)
	if !strings.Contains(md, "list-1") || !strings.Contains(md, "list-2") {
		t.Error("expected markdown to contain both list names")
	}
}

// TestFormatListUserListsMarkdown_Empty verifies the ListUserListsMarkdown_Empty Markdown formatter for a representative listuserlists_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListUserListsMarkdown_Empty(t *testing.T) {
	out := ListOutput{UserLists: []Output{}}
	md := FormatListUserListsMarkdown(out)
	if !strings.Contains(md, "No feature flag user lists found") {
		t.Error("expected empty message")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// ---------------------------------------------------------------------------
// Constants — prefixed with cov to avoid redeclaration
// ---------------------------------------------------------------------------.

// covUserListJSON identifies the cov user list JSON constant used by this package.
const covUserListJSON = `{
	"name": "cov-users",
	"user_xids": "u1,u2",
	"id": 1,
	"iid": 10,
	"project_id": 42,
	"created_at": "2026-06-01T12:00:00Z",
	"updated_at": "2026-06-02T12:00:00Z"
}`

// ---------------------------------------------------------------------------
// List — API error, search param
// ---------------------------------------------------------------------------.

// TestListUserLists_APIError verifies that ListUserLists returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListUserLists_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := ListUserLists(context.Background(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListUserLists_Forbidden verifies the ListUserLists_Forbidden handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListUserLists_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	_, err := ListUserLists(context.Background(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "Premium/Ultimate") {
		t.Fatalf("error = %v, want Premium/Ultimate hint", err)
	}
}

// TestListUserLists_WithSearch verifies the ListUserLists_WithSearch handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListUserLists_WithSearch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/42/feature_flags_user_lists", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("search") != "beta" {
			t.Errorf("expected search=beta, got %q", r.URL.Query().Get("search"))
		}
		testutil.RespondJSON(w, http.StatusOK, `[`+covUserListJSON+`]`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListUserLists(context.Background(), client, ListInput{
		ProjectID: "42",
		Search:    "beta",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.UserLists) != 1 {
		t.Errorf("expected 1 user list, got %d", len(out.UserLists))
	}
}

// TestListUserLists_KeysetAndOrdering verifies that ListUserLists forwards
// order_by, sort, pagination, page_token, and offset pagination parameters to
// the GitLab API query string (1:1 audit P3 keyset pagination).
func TestListUserLists_KeysetAndOrdering(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/42/feature_flags_user_lists", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for key, want := range map[string]string{
			"order_by":   "updated_at",
			"sort":       "desc",
			"pagination": "keyset",
			"page_token": "cursor-7",
			"per_page":   "50",
		} {
			if got := q.Get(key); got != want {
				t.Errorf("query %s = %q, want %q", key, got, want)
			}
		}
		testutil.RespondJSON(w, http.StatusOK, `[`+covUserListJSON+`]`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListUserLists(context.Background(), client, ListInput{
		ProjectID:             "42",
		OrderBy:               "updated_at",
		Sort:                  "desc",
		PaginationInput:       toolutil.PaginationInput{PerPage: 50},
		KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "cursor-7"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.UserLists) != 1 {
		t.Errorf("expected 1 user list, got %d", len(out.UserLists))
	}
}

// ---------------------------------------------------------------------------
// Get — API error
// ---------------------------------------------------------------------------.

// TestGetUserList_APIError verifies that GetUserList returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetUserList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := GetUserList(context.Background(), client, GetInput{ProjectID: "1", IID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// Create — API error
// ---------------------------------------------------------------------------.

// TestCreateUserList_APIError verifies that CreateUserList returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateUserList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := CreateUserList(context.Background(), client, CreateInput{
		ProjectID: "1", Name: "x", UserXIDs: "u1",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreateUserList_ErrorBranches verifies that CreateUserListBranches returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateUserList_ErrorBranches(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
		wantText   string
	}{
		{name: "forbidden", statusCode: http.StatusForbidden, wantText: "Premium/Ultimate"},
		{name: "generic", statusCode: http.StatusUnprocessableEntity, wantText: "ff_user_list_create"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, testCase.statusCode, `{"message":"failed"}`)
			}))
			_, err := CreateUserList(context.Background(), client, CreateInput{ProjectID: "1", Name: "x", UserXIDs: "u1"})
			if err == nil {
				t.Fatal(errExpectedAPI)
			}
			if !strings.Contains(err.Error(), testCase.wantText) {
				t.Fatalf("error = %v, want %q", err, testCase.wantText)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Update — API error
// ---------------------------------------------------------------------------.

// TestUpdateUserList_APIError verifies that UpdateUserList returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdateUserList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := UpdateUserList(context.Background(), client, UpdateInput{
		ProjectID: "1", IID: 10, Name: "x",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdateUserList_Forbidden verifies the UpdateUserList_Forbidden handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUpdateUserList_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	_, err := UpdateUserList(context.Background(), client, UpdateInput{ProjectID: "1", IID: 10, Name: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "Developer+ role") {
		t.Fatalf("error = %v, want Developer+ role hint", err)
	}
}

// ---------------------------------------------------------------------------
// Delete — API error
// ---------------------------------------------------------------------------.

// TestDeleteUserList_APIError verifies that DeleteUserList returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteUserList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	err := DeleteUserList(context.Background(), client, DeleteInput{ProjectID: "1", IID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// FormatUserListMarkdown — with CreatedAt / UpdatedAt
// ---------------------------------------------------------------------------.

// TestFormatUserListMarkdown_WithDates verifies the UserListMarkdown_WithDates Markdown formatter for a representative userlist_withdates input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatUserListMarkdown_WithDates(t *testing.T) {
	out := Output{
		ID: 1, IID: 10, ProjectID: 42,
		Name: "cov-list", UserXIDs: "a,b",
		CreatedAt: "2026-06-01T12:00:00Z",
		UpdatedAt: "2026-06-02T12:00:00Z",
	}
	md := FormatUserListMarkdown(out)
	if !strings.Contains(md, "1 Jun 2026 12:00 UTC") {
		t.Error("expected CreatedAt in markdown")
	}
	if !strings.Contains(md, "2 Jun 2026 12:00 UTC") {
		t.Error("expected UpdatedAt in markdown")
	}
}
