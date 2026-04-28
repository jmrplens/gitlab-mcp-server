// ffuserlists_test.go contains unit tests for the feature flag user list MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.

package ffuserlists

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	testListName        = "beta-users"
	fmtUnexpErr         = "unexpected error: %v"
	fmtWantName         = "expected name 'beta-users', got %q"
	errExpMissingParams = "expected error for missing params"
	errExpMissingIID    = "expected error for missing iid"
)

const userListJSON = `{
	"name": "beta-users",
	"user_xids": "user1,user2,user3",
	"id": 1,
	"iid": 10,
	"project_id": 42,
	"created_at": "2026-01-01T00:00:00Z",
	"updated_at": "2026-01-02T00:00:00Z"
}`

const userListArrayJSON = `[` + userListJSON + `]`

// -- List --.

// TestListUserLists_Success verifies the behavior of list user lists success.
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

// TestListUserLists_MissingProjectID verifies the behavior of list user lists missing project i d.
func TestListUserLists_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListUserLists(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// -- Get --.

// TestGetUserList_Success verifies the behavior of get user list success.
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

// TestGetUserList_MissingParams verifies the behavior of get user list missing params.
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

// TestCreateUserList_Success verifies the behavior of create user list success.
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

// TestCreateUserList_MissingParams verifies the behavior of create user list missing params.
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

// TestUpdateUserList_Success verifies the behavior of update user list success.
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

// TestUpdateUserList_MissingParams verifies the behavior of update user list missing params.
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

// TestDeleteUserList_Success verifies the behavior of delete user list success.
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

// TestDeleteUserList_MissingParams verifies the behavior of delete user list missing params.
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

// TestFormatUserListMarkdown verifies the behavior of format user list markdown.
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

// TestFormatUserListMarkdown_NameInHeading verifies the behavior of format user list markdown name in heading.
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

// TestFormatListUserListsMarkdown_NoIDColumn verifies the behavior of format list user lists markdown no i d column.
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

// TestFormatListUserListsMarkdown verifies the behavior of format list user lists markdown.
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

// TestFormatListUserListsMarkdown_Empty verifies the behavior of format list user lists markdown empty.
func TestFormatListUserListsMarkdown_Empty(t *testing.T) {
	out := ListOutput{UserLists: []Output{}}
	md := FormatListUserListsMarkdown(out)
	if !strings.Contains(md, "No feature flag user lists found") {
		t.Error("expected empty message")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const errExpectedAPI = "expected API error, got nil"

// ---------------------------------------------------------------------------
// Constants — prefixed with cov to avoid redeclaration
// ---------------------------------------------------------------------------.

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

// TestListUserLists_APIError verifies the behavior of list user lists a p i error.
func TestListUserLists_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := ListUserLists(context.Background(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListUserLists_WithSearch verifies the behavior of list user lists with search.
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

// ---------------------------------------------------------------------------
// Get — API error
// ---------------------------------------------------------------------------.

// TestGetUserList_APIError verifies the behavior of get user list a p i error.
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

// TestCreateUserList_APIError verifies the behavior of create user list a p i error.
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

// ---------------------------------------------------------------------------
// Update — API error
// ---------------------------------------------------------------------------.

// TestUpdateUserList_APIError verifies the behavior of update user list a p i error.
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

// ---------------------------------------------------------------------------
// Delete — API error
// ---------------------------------------------------------------------------.

// TestDeleteUserList_APIError verifies the behavior of delete user list a p i error.
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

// TestFormatUserListMarkdown_WithDates verifies the behavior of format user list markdown with dates.
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

// ---------------------------------------------------------------------------
// RegisterTools — MCP round-trip
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := covNewMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_ff_user_list_list", map[string]any{"project_id": "42"}},
		{"get", "gitlab_ff_user_list_get", map[string]any{"project_id": "42", "user_list_iid": float64(10)}},
		{"create", "gitlab_ff_user_list_create", map[string]any{"project_id": "42", "name": "test", "user_xids": "u1"}},
		{"update", "gitlab_ff_user_list_update", map[string]any{"project_id": "42", "user_list_iid": float64(10), "name": "updated"}},
		{"delete", "gitlab_ff_user_list_delete", map[string]any{"project_id": "42", "user_list_iid": float64(10)}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.tool, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.tool, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// RegisterMeta — no panic
// ---------------------------------------------------------------------------.

// TestRegisterMeta_NoPanic verifies the behavior of register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// ---------------------------------------------------------------------------
// Helper: MCP session factory
// ---------------------------------------------------------------------------.

// covNewMCPSession is an internal helper for the ffuserlists package.
func covNewMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v4/projects/42/feature_flags_user_lists", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+covUserListJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/projects/42/feature_flags_user_lists/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covUserListJSON)
	})
	handler.HandleFunc("POST /api/v4/projects/42/feature_flags_user_lists", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, covUserListJSON)
	})
	handler.HandleFunc("PUT /api/v4/projects/42/feature_flags_user_lists/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covUserListJSON)
	})
	handler.HandleFunc("DELETE /api/v4/projects/42/feature_flags_user_lists/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
