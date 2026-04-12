// markdown_test.go contains unit tests for the Markdown formatting functions
// in the markdown package.
package markdown

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestRender_Success verifies that Render handles the success scenario correctly.
func TestRender_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/markdown" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"html":"<p>Hello <strong>world</strong></p>"}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Render(t.Context(), client, RenderInput{Text: "Hello **world**"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.HTML != "<p>Hello <strong>world</strong></p>" {
		t.Errorf("unexpected HTML: %s", out.HTML)
	}
}

// TestRender_WithGFMAndProject verifies that Render handles the with g f m and project scenario correctly.
func TestRender_WithGFMAndProject(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"html":"<p>Rendered with GFM</p>"}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Render(t.Context(), client, RenderInput{
		Text:    "Hello",
		GFM:     true,
		Project: "my-group/my-project",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.HTML != "<p>Rendered with GFM</p>" {
		t.Errorf("unexpected HTML: %s", out.HTML)
	}
}

// TestRender_Error verifies that Render handles the error scenario correctly.
func TestRender_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := Render(t.Context(), client, RenderInput{Text: "test"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestFormatRenderMarkdown_Empty verifies that FormatRenderMarkdown handles the empty scenario correctly.
func TestFormatRenderMarkdown_Empty(t *testing.T) {
	result := FormatRenderMarkdown(RenderOutput{})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	text := fmt.Sprintf("%v", result.Content[0])
	if text == "" {
		t.Fatal("expected non-empty text")
	}
}

// TestFormatRenderMarkdown_WithData verifies that FormatRenderMarkdown handles the with data scenario correctly.
func TestFormatRenderMarkdown_WithData(t *testing.T) {
	result := FormatRenderMarkdown(RenderOutput{HTML: "<p>Hello</p>"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	text := fmt.Sprintf("%v", result.Content[0])
	if text == "" {
		t.Fatal("expected non-empty text")
	}
}

// TestRender_CancelledContext verifies that Render handles the cancelled context scenario correctly.
func TestRender_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"html":"<p>x</p>"}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Render(ctx, client, RenderInput{Text: "test"})
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}

// TestRegisterTools_NoPanic verifies that RegisterTools handles the no panic scenario correctly.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallThroughMCP verifies that RegisterTools handles the call through m c p scenario correctly.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/markdown" {
			testutil.RespondJSON(w, http.StatusOK, `{"html":"<p>rendered</p>"}`)
			return
		}
		http.NotFound(w, r)
	}))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_render_markdown",
		Arguments: map[string]any{"text": "Hello **world**"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result.IsError {
		t.Fatal("CallTool returned IsError=true")
	}
}
