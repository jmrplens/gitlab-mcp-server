// securefiles_test.go contains unit tests for the secure file MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.

package securefiles

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const errExpectedErr = "expected error"

const fmtUnexpErr = "unexpected error: %v"

const testFileName = "key.pem"

// TestList verifies the behavior of list.
func TestList(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/secure_files" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"key.pem","checksum":"abc","checksum_algorithm":"sha256"}]`)
	}))
	out, err := List(t.Context(), client, ListInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Files) != 1 || out.Files[0].Name != testFileName {
		t.Errorf("unexpected files: %+v", out.Files)
	}
}

// TestList_Error verifies the behavior of list error.
func TestList_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	_, err := List(t.Context(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestShow verifies the behavior of show.
func TestShow(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/secure_files/1" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"key.pem","checksum":"abc","checksum_algorithm":"sha256"}`)
	}))
	out, err := Show(t.Context(), client, ShowInput{ProjectID: "1", FileID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != testFileName {
		t.Errorf("expected key.pem, got %s", out.Name)
	}
}

// TestShow_Error verifies the behavior of show error.
func TestShow_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	_, err := Show(t.Context(), client, ShowInput{ProjectID: "1", FileID: 999})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestShow_InvalidFileID verifies the behavior of show invalid file i d.
func TestShow_InvalidFileID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Show(t.Context(), client, ShowInput{ProjectID: "1", FileID: 0})
	if err == nil {
		t.Fatal("expected error for zero FileID")
	}
	if !strings.Contains(err.Error(), "file_id") {
		t.Errorf("expected error to mention file_id, got: %v", err)
	}
}

// TestRemove_InvalidFileID verifies the behavior of remove invalid file i d.
func TestRemove_InvalidFileID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	err := Remove(t.Context(), client, RemoveInput{ProjectID: "1", FileID: -1})
	if err == nil {
		t.Fatal("expected error for negative FileID")
	}
	if !strings.Contains(err.Error(), "file_id") {
		t.Errorf("expected error to mention file_id, got: %v", err)
	}
}

// TestCreate verifies the behavior of create.
func TestCreate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"name":"cert.pem","checksum":"def","checksum_algorithm":"sha256"}`)
	}))
	content := base64.StdEncoding.EncodeToString([]byte("cert-data"))
	out, err := Create(t.Context(), client, CreateInput{ProjectID: "1", Name: "cert.pem", ContentBase64: content})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 2 {
		t.Errorf("expected ID 2, got %d", out.ID)
	}
}

// TestCreate_Error verifies the behavior of create error.
func TestCreate_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	content := base64.StdEncoding.EncodeToString([]byte("data"))
	_, err := Create(t.Context(), client, CreateInput{ProjectID: "1", Name: "x", ContentBase64: content})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestCreate_FilePath_Success verifies create with file_path.
func TestCreate_FilePath_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"id":3,"name":"key.pem","checksum":"abc","checksum_algorithm":"sha256"}`)
	}))
	tmpFile := t.TempDir() + "/key.pem"
	if err := os.WriteFile(tmpFile, []byte("private-key-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := Create(t.Context(), client, CreateInput{ProjectID: "1", Name: "key.pem", FilePath: tmpFile})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 3 {
		t.Errorf("expected ID 3, got %d", out.ID)
	}
}

// TestCreate_FilePath_NotFound verifies create with nonexistent file_path.
func TestCreate_FilePath_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(t.Context(), client, CreateInput{ProjectID: "1", Name: "x", FilePath: "/nonexistent/file.pem"})
	if err == nil {
		t.Fatal("expected error for nonexistent file_path, got nil")
	}
}

// TestCreate_BothFilePathAndBase64 verifies error when both inputs provided.
func TestCreate_BothFilePathAndBase64(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(t.Context(), client, CreateInput{ProjectID: "1", Name: "x", FilePath: "/tmp/x", ContentBase64: "dGVzdA=="})
	if err == nil {
		t.Fatal("expected error when both file_path and content_base64 provided, got nil")
	}
}

// TestCreate_NeitherInput verifies error when neither input provided.
func TestCreate_NeitherInput(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(t.Context(), client, CreateInput{ProjectID: "1", Name: "x"})
	if err == nil {
		t.Fatal("expected error when neither file_path nor content_base64 provided, got nil")
	}
}

// TestCreate_InvalidBase64 verifies error for invalid base64.
func TestCreate_InvalidBase64(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(t.Context(), client, CreateInput{ProjectID: "1", Name: "x", ContentBase64: "!!!invalid!!!"})
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

// TestRemove verifies the behavior of remove.
func TestRemove(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/secure_files/1" || r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	err := Remove(t.Context(), client, RemoveInput{ProjectID: "1", FileID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestRemove_Error verifies the behavior of remove error.
func TestRemove_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	err := Remove(t.Context(), client, RemoveInput{ProjectID: "1", FileID: 999})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestFormatListMarkdown verifies the behavior of format list markdown.
func TestFormatListMarkdown(t *testing.T) {
	md := FormatListMarkdown(ListOutput{Files: []SecureFileItem{{ID: 1, Name: testFileName}}})
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// TestFormatShowMarkdown verifies the behavior of format show markdown.
func TestFormatShowMarkdown(t *testing.T) {
	md := FormatShowMarkdown(SecureFileItem{ID: 1, Name: testFileName})
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// List — pagination branch (Page > 0 || PerPage > 0)
// ---------------------------------------------------------------------------.

// TestList_WithPagination verifies the behavior of cov list with pagination.
func TestList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/secure_files" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":1,"name":"key.pem","checksum":"abc","checksum_algorithm":"sha256"},{"id":2,"name":"cert.pem","checksum":"def","checksum_algorithm":"sha256"}]`,
			testutil.PaginationHeaders{Page: "2", PerPage: "2", Total: "5", TotalPages: "3", NextPage: "3", PrevPage: "1"},
		)
	}))
	out, err := List(t.Context(), client, ListInput{ProjectID: "1", Page: 2, PerPage: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Files) != 2 {
		t.Fatalf("len(Files) = %d, want 2", len(out.Files))
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", out.Pagination.TotalPages)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — empty list
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_Empty verifies the behavior of cov format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No secure files found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — with pagination
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithPagination verifies the behavior of cov format list markdown with pagination.
func TestFormatListMarkdown_WithPagination(t *testing.T) {
	md := FormatListMarkdown(ListOutput{
		Files: []SecureFileItem{
			{ID: 1, Name: "key.pem", ChecksumAlgorithm: "sha256"},
			{ID: 2, Name: "cert.pem", ChecksumAlgorithm: "sha256"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 5, Page: 1, PerPage: 2, TotalPages: 3},
	})
	for _, want := range []string{"| ID |", "| 1 |", "| 2 |", "key.pem", "cert.pem"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of cov register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// RegisterTools — MCP round-trip for all 4 tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates cov register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := covNewSecureFilesMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_secure_files", map[string]any{"project_id": "1", "page": 1, "per_page": 20}},
		{"show", "gitlab_show_secure_file", map[string]any{"project_id": "1", "file_id": 1}},
		{"create", "gitlab_create_secure_file", map[string]any{"project_id": "1", "name": "cert.pem", "content_base64": "ZGF0YQ=="}},
		{"remove", "gitlab_remove_secure_file", map[string]any{"project_id": "1", "file_id": 1}},
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
// RegisterTools — MCP round-trip error paths
// ---------------------------------------------------------------------------.

// TestRegisterTools_ErrorPaths validates cov register tools error paths across multiple scenarios using table-driven subtests.
func TestRegisterTools_ErrorPaths(t *testing.T) {
	session := covNewSecureFilesErrorMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list_error", "gitlab_list_secure_files", map[string]any{"project_id": "1", "page": 1, "per_page": 20}},
		{"show_error", "gitlab_show_secure_file", map[string]any{"project_id": "1", "file_id": 1}},
		{"create_error", "gitlab_create_secure_file", map[string]any{"project_id": "1", "name": "x", "content_base64": "ZGF0YQ=="}},
		{"remove_error", "gitlab_remove_secure_file", map[string]any{"project_id": "1", "file_id": 1}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) transport error: %v", tt.tool, err)
			}
			if !result.IsError {
				t.Fatalf("CallTool(%s) expected IsError=true", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// RegisterMeta — no panic
// ---------------------------------------------------------------------------.

// TestRegisterMeta_NoPanic verifies the behavior of cov register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// ---------------------------------------------------------------------------
// RegisterMeta — MCP round-trip for all 4 actions
// ---------------------------------------------------------------------------.

// TestRegisterMeta_CallAllThroughMCP validates cov register meta call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterMeta_CallAllThroughMCP(t *testing.T) {
	session := covNewSecureFilesMetaMCPSession(t)
	ctx := context.Background()

	actions := []struct {
		name   string
		action string
		params map[string]any
	}{
		{"list", "list", map[string]any{"project_id": "1"}},
		{"show", "show", map[string]any{"project_id": "1", "file_id": 1}},
		{"create", "create", map[string]any{"project_id": "1", "name": "cert.pem", "content_base64": "ZGF0YQ=="}},
		{"remove", "remove", map[string]any{"project_id": "1", "file_id": 1}},
	}

	for _, tt := range actions {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name: "gitlab_secure_file",
				Arguments: map[string]any{
					"action": tt.action,
					"params": tt.params,
				},
			})
			if err != nil {
				t.Fatalf("CallTool(action=%s) error: %v", tt.action, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(action=%s) returned error: %s", tt.action, tc.Text)
					}
				}
				t.Fatalf("CallTool(action=%s) returned IsError=true", tt.action)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session for RegisterTools (success paths)
// ---------------------------------------------------------------------------.

// covNewSecureFilesMCPSession is an internal helper for the securefiles package.
func covNewSecureFilesMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	covFileJSON := `{"id":1,"name":"key.pem","checksum":"abc","checksum_algorithm":"sha256"}`

	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v4/projects/1/secure_files", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+covFileJSON+`]`)
	})

	handler.HandleFunc("GET /api/v4/projects/1/secure_files/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covFileJSON)
	})

	handler.HandleFunc("POST /api/v4/projects/1/secure_files", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"name":"cert.pem","checksum":"def","checksum_algorithm":"sha256"}`)
	})

	handler.HandleFunc("DELETE /api/v4/projects/1/secure_files/1", func(w http.ResponseWriter, _ *http.Request) {
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

// ---------------------------------------------------------------------------
// Helper: MCP session for RegisterTools (error paths)
// ---------------------------------------------------------------------------.

// covNewSecureFilesErrorMCPSession is an internal helper for the securefiles package.
func covNewSecureFilesErrorMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
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

// ---------------------------------------------------------------------------
// Helper: MCP session for RegisterMeta
// ---------------------------------------------------------------------------.

// covNewSecureFilesMetaMCPSession is an internal helper for the securefiles package.
func covNewSecureFilesMetaMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	covFileJSON := `{"id":1,"name":"key.pem","checksum":"abc","checksum_algorithm":"sha256"}`

	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v4/projects/1/secure_files", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+covFileJSON+`]`)
	})

	handler.HandleFunc("GET /api/v4/projects/1/secure_files/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covFileJSON)
	})

	handler.HandleFunc("POST /api/v4/projects/1/secure_files", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"name":"cert.pem","checksum":"def","checksum_algorithm":"sha256"}`)
	})

	handler.HandleFunc("DELETE /api/v4/projects/1/secure_files/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)

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

// TestRegisterTools_DeleteConfirmDeclined covers the ConfirmAction early-return
// branch in the secure file delete handler when the user declines confirmation.
func TestRegisterTools_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_remove_secure_file",
		Arguments: map[string]any{"project_id": "1", "file_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}
