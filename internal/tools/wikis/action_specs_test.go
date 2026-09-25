// action_specs_test.go contains route and catalog-surface tests for behavior that
// used to live in register.go (not-found output and destructive confirmation),
// alongside the discovery metadata and input-schema assertions each spec carries.
package wikis

import (
	"context"
	"net/http"
	"os"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

	// The project is a JSON number of eight digits, which reaches the route as
	// a float64: %v named it 1.2345678e+07.
	result, err := byTool["gitlab_wiki_get"].Route.Handler(t.Context(), map[string]any{"project_id": float64(12345678), "slug": "NonExistent"})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	if notFound, ok := result.(wikiNotFoundOutput); !ok || notFound.Identifier != `slug "NonExistent" in project 12345678` {
		t.Fatalf("result = %#v, want wikiNotFoundOutput naming slug \"NonExistent\" in project 12345678", result)
	}
}

// TestActionSpecs_GetRoute_ServerErrorIsNotReportedAsNotFound covers the other
// half of the same condition: only a 404 becomes the structured not-found
// output, and every other failure is handed back as the error it is. The route
// reads the error and its status together, and a route that stopped asking for
// the status would answer a 403 with "no such wiki page", sending a model to
// look for a slug that exists and it may not read.
func TestActionSpecs_GetRoute_ServerErrorIsNotReportedAsNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := wikiSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_wiki_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "slug": "Home"})
	if err == nil {
		t.Fatalf("Route.Handler() = %#v with nil error, want the 403 passed through", result)
	}
	if _, ok := result.(wikiNotFoundOutput); ok {
		t.Errorf("403 answered with %T, want no not-found output", result)
	}
	if got := err.Error(); !contains(got, "wikiGet") {
		t.Errorf("error = %q, want it to name the wikiGet operation", got)
	}
}

// TestActionSpecs_MetadataEntriesAreComplete pins what decorateWikiMeta's
// guards rest on: every entry of wikiActionMeta fills usage, aliases, related
// and description, which is why none of those four guards is ever seen false.
// It holds each spec to the entry's own values rather than to "not empty",
// because the placeholder metadata wikiOptions builds is non-empty too, so a
// spec that quietly fell back to it would have passed either way.
func TestActionSpecs_MetadataEntriesAreComplete(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	byTool := wikiSpecsByTool(t, ActionSpecs(client))

	for name, meta := range wikiActionMeta {
		t.Run(name, func(t *testing.T) {
			spec, ok := byTool[name]
			if !ok {
				t.Fatalf("wikiActionMeta names %s, which ActionSpecs does not register", name)
			}
			if meta.usage == "" || meta.description == "" {
				t.Errorf("usage = %q, description = %q, want both filled", meta.usage, meta.description)
			}
			if len(meta.aliases) == 0 || len(meta.related) == 0 {
				t.Errorf("aliases = %v, related = %v, want both filled", meta.aliases, meta.related)
			}
			if spec.Usage != meta.usage {
				t.Errorf("Usage = %q, want the entry's own %q", spec.Usage, meta.usage)
			}
			if spec.IndividualTool.Description != meta.description {
				t.Errorf("Description = %q, want the entry's own %q", spec.IndividualTool.Description, meta.description)
			}
			if !slices.Equal(spec.Aliases, meta.aliases) {
				t.Errorf("Aliases = %v, want the entry's own %v", spec.Aliases, meta.aliases)
			}
			if !slices.Equal(spec.RelatedActions, meta.related) {
				t.Errorf("RelatedActions = %v, want the entry's own %v", spec.RelatedActions, meta.related)
			}
		})
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
// branch of the shared toolutil.ReadFileOrBase64 helper as used by the wiki attachment upload.
func TestResolveAttachmentReader_InvalidBase64(t *testing.T) {
	_, err := toolutil.ReadFileOrBase64("upload_wiki_attachment", "", "!!!not-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
	if got := err.Error(); !contains(got, "invalid base64 content") {
		t.Errorf("expected base64 error message, got: %s", got)
	}
}

// TestResolveAttachmentReader_InvalidFilePath covers the file open error
// branch of the shared toolutil.ReadFileOrBase64 helper as used by the wiki attachment upload when the file does not exist.
func TestResolveAttachmentReader_InvalidFilePath(t *testing.T) {
	_, err := toolutil.ReadFileOrBase64("upload_wiki_attachment", "/nonexistent/path/to/file.txt", "")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

// TestResolveAttachmentReader_ValidFile covers the successful file-read branch
// of the shared toolutil.ReadFileOrBase64 helper as used by the wiki
// attachment upload, and that the bytes it hands back are the file's own.
func TestResolveAttachmentReader_ValidFile(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/test.txt"
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := toolutil.ReadFileOrBase64("upload_wiki_attachment", path, "")
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

// TestActionSpecs_RichMetadata verifies that every wiki action carries
// non-generic discovery metadata (Usage, Aliases, RelatedActions) and a
// "Returns: … See also: …" individual-tool description (1:1 audit R-META),
// and that decorateWikiMeta is a no-op for an unknown tool.
func TestActionSpecs_RichMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	byTool := wikiSpecsByTool(t, ActionSpecs(client))

	wantTools := []string{
		"gitlab_wiki_list",
		"gitlab_wiki_get",
		"gitlab_wiki_create",
		"gitlab_wiki_update",
		"gitlab_wiki_delete",
		"gitlab_wiki_upload_attachment",
	}
	for _, name := range wantTools {
		t.Run(name, func(t *testing.T) {
			spec, ok := byTool[name]
			if !ok {
				t.Fatalf("missing spec for %s", name)
			}
			if spec.Usage == "" || spec.Usage == "Use to execute wikis domain action." {
				t.Errorf("%s: generic or empty Usage: %q", name, spec.Usage)
			}
			if len(spec.Aliases) == 0 || spec.Aliases[0] == name {
				t.Errorf("%s: aliases not replaced with natural-language phrases: %v", name, spec.Aliases)
			}
			if len(spec.RelatedActions) == 0 {
				t.Errorf("%s: empty RelatedActions", name)
			}
			desc := spec.IndividualTool.Description
			if !contains(desc, "Returns:") || !contains(desc, "See also:") {
				t.Errorf("%s: description missing Returns:/See also: form: %q", name, desc)
			}
		})
	}

	// decorateWikiMeta must be a no-op for a tool with no metadata entry.
	opts := wikiOptions("gitlab_wiki_unknown")
	decorateWikiMeta(&opts, "gitlab_wiki_unknown")
	if opts.Usage != "Use to execute wikis domain action." {
		t.Errorf("unknown tool Usage mutated: %q", opts.Usage)
	}
	if opts.IndividualTool.Description != "" {
		t.Errorf("unknown tool Description mutated: %q", opts.IndividualTool.Description)
	}
}

// TestActionSpecs_FormatEnum verifies that create and update constrain format
// to the four page formats the Wikis API accepts, and that no other wiki
// action carries a format override.
func TestActionSpecs_FormatEnum(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	byTool := wikiSpecsByTool(t, ActionSpecs(client))
	want := []any{"markdown", "rdoc", "asciidoc", "org"}
	for name, spec := range byTool {
		wantOverride := name == "gitlab_wiki_create" || name == "gitlab_wiki_update"
		t.Run(name, func(t *testing.T) {
			has := false
			for _, override := range spec.InputSchemaOverrides {
				if override.PropertyPath != "format" {
					continue
				}
				has = true
				if enum, ok := override.Values["enum"].([]any); !ok || !slices.Equal(enum, want) {
					t.Errorf("format enum = %v, want %v", override.Values["enum"], want)
				}
			}
			if has != wantOverride {
				t.Errorf("format override present = %v, want %v", has, wantOverride)
			}
		})
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
