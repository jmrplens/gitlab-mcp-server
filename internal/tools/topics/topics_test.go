// topics_test.go contains unit tests for the topic MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package topics

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const errExpNonNilResult = "expected non-nil result"

const errNoReachAPI = "should not reach API"

const fmtUnexpErr = "unexpected error: %v"

const topicJSON = `{"id":1,"name":"go","title":"Go","description":"The Go programming language","total_projects_count":42,"avatar_url":"https://example.com/go.png"}`

const pathTopics = "/api/v4/topics"

const pathTopicOne = "/api/v4/topics/1"

const errExpErrZeroTopicID = "expected error for zero topic_id"

const errExpErrNegTopicID = "expected error for negative topic_id"

const testTopicID = "topic_id"

const fmtExpErrMentionTopicID = "expected error to mention topic_id, got %q"

// TestList_Success verifies the behavior of list success.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathTopics && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `[`+topicJSON+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(t.Context(), client, ListInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Topics) != 1 {
		t.Fatalf("expected 1 topic, got %d", len(out.Topics))
	}
	if out.Topics[0].Name != "go" {
		t.Errorf("expected name 'go', got %q", out.Topics[0].Name)
	}
	if out.Topics[0].TotalProjectsCount != 42 {
		t.Errorf("expected 42 projects, got %d", out.Topics[0].TotalProjectsCount)
	}
}

// TestList_WithSearch verifies the behavior of list with search.
func TestList_WithSearch(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathTopics && r.URL.Query().Get("search") == "go" {
			testutil.RespondJSON(w, http.StatusOK, `[`+topicJSON+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(t.Context(), client, ListInput{Search: "go"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Topics) != 1 {
		t.Fatalf("expected 1 topic, got %d", len(out.Topics))
	}
}

// TestList_Error verifies the behavior of list error.
func TestList_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := List(t.Context(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestGet_Success verifies the behavior of get success.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathTopicOne && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, topicJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(t.Context(), client, GetInput{TopicID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Topic.ID != 1 {
		t.Errorf("expected topic ID 1, got %d", out.Topic.ID)
	}
	if out.Topic.Title != "Go" {
		t.Errorf("expected title 'Go', got %q", out.Topic.Title)
	}
}

// TestCreate_Success verifies the behavior of create success.
func TestCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathTopics && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, topicJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(t.Context(), client, CreateInput{Name: "go", Title: "Go"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Topic.Name != "go" {
		t.Errorf("expected name 'go', got %q", out.Topic.Name)
	}
}

// TestUpdate_Success verifies the behavior of update success.
func TestUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathTopicOne && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, topicJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(t.Context(), client, UpdateInput{TopicID: 1, Title: "Golang"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Topic.ID != 1 {
		t.Errorf("expected topic ID 1, got %d", out.Topic.ID)
	}
}

// TestDelete_Success verifies the behavior of delete success.
func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathTopicOne && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(t.Context(), client, DeleteInput{TopicID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_Error verifies the behavior of delete error.
func TestDelete_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	err := Delete(t.Context(), client, DeleteInput{TopicID: 1})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestGet_InvalidTopicID verifies the behavior of get invalid topic i d.
func TestGet_InvalidTopicID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := Get(t.Context(), client, GetInput{TopicID: 0})
	if err == nil {
		t.Fatal(errExpErrZeroTopicID)
	}
	if !strings.Contains(err.Error(), testTopicID) {
		t.Errorf(fmtExpErrMentionTopicID, err)
	}
	_, err = Get(t.Context(), client, GetInput{TopicID: -1})
	if err == nil {
		t.Fatal(errExpErrNegTopicID)
	}
}

// TestUpdate_InvalidTopicID verifies the behavior of update invalid topic i d.
func TestUpdate_InvalidTopicID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := Update(t.Context(), client, UpdateInput{TopicID: 0, Title: "x"})
	if err == nil {
		t.Fatal(errExpErrZeroTopicID)
	}
	if !strings.Contains(err.Error(), testTopicID) {
		t.Errorf(fmtExpErrMentionTopicID, err)
	}
	_, err = Update(t.Context(), client, UpdateInput{TopicID: -1, Title: "x"})
	if err == nil {
		t.Fatal(errExpErrNegTopicID)
	}
}

// TestDelete_InvalidTopicID verifies the behavior of delete invalid topic i d.
func TestDelete_InvalidTopicID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	err := Delete(t.Context(), client, DeleteInput{TopicID: 0})
	if err == nil {
		t.Fatal(errExpErrZeroTopicID)
	}
	if !strings.Contains(err.Error(), testTopicID) {
		t.Errorf(fmtExpErrMentionTopicID, err)
	}
	err = Delete(t.Context(), client, DeleteInput{TopicID: -1})
	if err == nil {
		t.Fatal(errExpErrNegTopicID)
	}
}

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	result := FormatListMarkdown(ListOutput{})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "No topics") {
		t.Errorf("expected 'No topics' message, got %q", text)
	}
}

// TestFormatListMarkdown_WithData verifies the behavior of format list markdown with data.
func TestFormatListMarkdown_WithData(t *testing.T) {
	result := FormatListMarkdown(ListOutput{
		Topics: []TopicItem{
			{ID: 1, Name: "go", Title: "Go", TotalProjectsCount: 42},
		},
		Pagination: toolutil.PaginationOutput{},
	})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// TestFormatTopicMarkdown verifies the behavior of format topic markdown.
func TestFormatTopicMarkdown(t *testing.T) {
	result := FormatTopicMarkdown(TopicItem{
		ID: 1, Name: "go", Title: "Go", Description: "The Go language",
		TotalProjectsCount: 42, AvatarURL: "https://example.com/go.png",
	})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "go") {
		t.Errorf("expected topic name in output, got %q", text)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const errExpectedNil = "expected error, got nil"

// ---------------------------------------------------------------------------
// List — API error (400)
// ---------------------------------------------------------------------------.

// TestList_APIError400 verifies the behavior of list a p i error400.
func TestList_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := List(t.Context(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------------------------------------------------------------------------
// Get — API error (400)
// ---------------------------------------------------------------------------.

// TestGet_APIError400 verifies the behavior of get a p i error400.
func TestGet_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := Get(t.Context(), client, GetInput{TopicID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------------------------------------------------------------------------
// Create — API error (400), with optional fields
// ---------------------------------------------------------------------------.

// TestCreate_APIError400 verifies the behavior of create a p i error400.
func TestCreate_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := Create(t.Context(), client, CreateInput{Name: "test"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestCreate_WithAllOptionalFields verifies the behavior of create with all optional fields.
func TestCreate_WithAllOptionalFields(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			capturedBody = string(body)
			testutil.RespondJSON(w, http.StatusCreated, topicJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Create(t.Context(), client, CreateInput{
		Name: "go", Title: "Go", Description: "The Go programming language",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Topic.Name != "go" {
		t.Errorf("expected name 'go', got %q", out.Topic.Name)
	}
	for _, want := range []string{"title", "description"} {
		if !strings.Contains(capturedBody, want) {
			t.Errorf("request body missing field %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// Update — API error (400), with all optional fields
// ---------------------------------------------------------------------------.

// TestUpdate_APIError400 verifies the behavior of update a p i error400.
func TestUpdate_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := Update(t.Context(), client, UpdateInput{TopicID: 1, Name: "x"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestUpdate_WithAllOptionalFields verifies the behavior of update with all optional fields.
func TestUpdate_WithAllOptionalFields(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			body, _ := io.ReadAll(r.Body)
			capturedBody = string(body)
			testutil.RespondJSON(w, http.StatusOK, topicJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Update(t.Context(), client, UpdateInput{
		TopicID: 1, Name: "golang", Title: "Golang", Description: "Updated desc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Topic.ID != 1 {
		t.Errorf("expected topic ID 1, got %d", out.Topic.ID)
	}
	for _, want := range []string{"title", "description"} {
		if !strings.Contains(capturedBody, want) {
			t.Errorf("request body missing field %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// Delete — API error (400)
// ---------------------------------------------------------------------------.

// TestDelete_APIError400 verifies the behavior of delete a p i error400.
func TestDelete_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	err := Delete(t.Context(), client, DeleteInput{TopicID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------------------------------------------------------------------------
// Formatters — additional branches
// ---------------------------------------------------------------------------.

// TestFormatTopicMarkdown_MinimalFields verifies the behavior of format topic markdown minimal fields.
func TestFormatTopicMarkdown_MinimalFields(t *testing.T) {
	result := FormatTopicMarkdown(TopicItem{ID: 1, Name: "test"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if strings.Contains(text, "Title") {
		t.Error("should not contain Title for empty title")
	}
	if strings.Contains(text, "Description") {
		t.Error("should not contain Description for empty description")
	}
	if strings.Contains(text, "Avatar") {
		t.Error("should not contain Avatar for empty avatar URL")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// MCP round-trip — all tools
// ---------------------------------------------------------------------------.

// TestMCPRoundTrip_AllTools validates m c p round trip all tools across multiple scenarios using table-driven subtests.
func TestMCPRoundTrip_AllTools(t *testing.T) {
	session := newTopicsMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_topics", map[string]any{}},
		{"get", "gitlab_get_topic", map[string]any{"topic_id": float64(1)}},
		{"create", "gitlab_create_topic", map[string]any{"name": "go"}},
		{"update", "gitlab_update_topic", map[string]any{"topic_id": float64(1), "name": "golang"}},
		{"delete", "gitlab_delete_topic", map[string]any{"topic_id": float64(1)}},
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

// newTopicsMCPSession is an internal helper for the topics package.
func newTopicsMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v4/topics", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+topicJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/topics/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, topicJSON)
	})
	handler.HandleFunc("POST /api/v4/topics", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, topicJSON)
	})
	handler.HandleFunc("PUT /api/v4/topics/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, topicJSON)
	})
	handler.HandleFunc("DELETE /api/v4/topics/1", func(w http.ResponseWriter, _ *http.Request) {
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
