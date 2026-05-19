// action_specs_test.go contains route and catalog-surface tests for behavior that
// used to live in register.go: not-found output and destructive confirmation.
package wikis

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestCatalogSurface_ConfirmDeclined covers generic destructive confirmation
// for wiki delete when the user declines.
func TestCatalogSurface_ConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	for _, spec := range ActionSpecs(client) {
		if spec.IndividualTool.Name == "gitlab_wiki_delete" {
			toolutil.RegisterSurfaceToolFromSpec(server, spec, toolutil.SurfaceToolRegisterOptions{Description: "Test wiki destructive confirmation.", Icons: toolutil.IconWiki})
		}
	}

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_wiki_delete",
		Arguments: map[string]any{"project_id": "42", "slug": "Home"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

// TestActionSpecs_GetNotFound covers the not-found route output when the API returns 404.
func TestActionSpecs_GetNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Wiki Page Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := wikiSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_wiki_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "slug": "NonExistent"})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	if _, ok := result.(wikiNotFoundOutput); !ok {
		t.Fatalf("result type = %T, want wikiNotFoundOutput", result)
	}
}

// TestWikiCreate_BadRequest covers the IsHTTPStatus(400) branch in Create
// that returns a hint about slug collisions or invalid content format.
func TestWikiCreate_BadRequest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"400 Bad Request"}`)
	})
	client := testutil.NewTestClient(t, mux)

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42",
		Title:     "Home",
		Content:   "hello",
	})
	if err == nil {
		t.Fatal("expected error for 400 response, got nil")
	}
	if got := err.Error(); !contains(got, "slug may already exist") {
		t.Errorf("expected hint about slug, got: %s", got)
	}
}

// TestResolveAttachmentReader_InvalidBase64 covers the base64 decode error
// branch in resolveAttachmentReader.
func TestResolveAttachmentReader_InvalidBase64(t *testing.T) {
	_, err := resolveAttachmentReader("", "!!!not-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
	if got := err.Error(); !contains(got, "invalid base64 content") {
		t.Errorf("expected base64 error message, got: %s", got)
	}
}

// TestResolveAttachmentReader_InvalidFilePath covers the file open error
// branch in resolveAttachmentReader when the file does not exist.
func TestResolveAttachmentReader_InvalidFilePath(t *testing.T) {
	_, err := resolveAttachmentReader("/nonexistent/path/to/file.txt", "")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

// TestResolveAttachmentReader_ValidFile covers the successful file-read branch
// in resolveAttachmentReader.
func TestResolveAttachmentReader_ValidFile(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/test.txt"
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := resolveAttachmentReader(path, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := make([]byte, r.Len())
	if _, readErr := r.Read(data); readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "hello" {
		t.Errorf("expected 'hello', got %q", string(data))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
