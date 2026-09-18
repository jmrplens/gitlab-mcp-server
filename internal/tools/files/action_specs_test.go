// action_specs_test.go contains canonical-route tests for repository file actions.
package files

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestActionSpecs_CallRoutes validates the CallRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallRoutes(t *testing.T) {
	byTool := fileSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(fileMockHandler))))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_file_get", map[string]any{"project_id": "42", "file_path": "main.go"}},
		{"gitlab_file_create", map[string]any{"project_id": "42", "file_path": "new.txt", "branch": "main", "content": "x", "commit_message": "add"}},
		{"gitlab_file_update", map[string]any{"project_id": "42", "file_path": "main.go", "branch": "main", "content": "y", "commit_message": "up"}},
		{"gitlab_file_delete", map[string]any{"project_id": "42", "file_path": "main.go", "branch": "main", "commit_message": "del"}},
		{"gitlab_file_blame", map[string]any{"project_id": "42", "file_path": "main.go"}},
		{"gitlab_file_metadata", map[string]any{"project_id": "42", "file_path": "main.go"}},
		{"gitlab_file_raw", map[string]any{"project_id": "42", "file_path": "main.go"}},
		{"gitlab_file_raw_metadata", map[string]any{"project_id": "42", "file_path": "main.go"}},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// TestActionSpecs_DeleteOutput validates the DeleteOutput route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := fileSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(fileMockHandler))))

	result, err := byTool["gitlab_file_delete"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "file_path": "main.go", "branch": "main", "commit_message": "del"})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_file_delete) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_file_delete) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted file \"main.go\" from project 42." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestActionSpecs_DeleteErrorPropagates validates the DeleteErrorPropagates route through the catalog surface.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_DeleteErrorPropagates(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		http.NotFound(w, r)
	}))
	byTool := fileSpecsByTool(t, ActionSpecs(client))

	_, err := byTool["gitlab_file_delete"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "file_path": "main.go", "branch": "main", "commit_message": "del"})
	if err == nil {
		t.Fatal("Route.Handler(gitlab_file_delete) expected error")
	}
}

// TestActionSpecs_GetNotFound validates the GetNotFound route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_GetNotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 File Not Found"}`)
	}))
	byTool := fileSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_file_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "file_path": "missing.go"})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_file_get) error: %v", err)
	}
	out, ok := result.(fileNotFoundOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_file_get) returned %T, want fileNotFoundOutput", result)
	}
	if out.Identifier != `"missing.go" in project 42` {
		t.Fatalf("identifier = %q", out.Identifier)
	}
}

// TestActionSpecs_GetFailureOtherThanNotFound_StaysAnError asserts that the
// file-get route turns a 404 into the not-found card and leaves every other
// failure an error.
//
// The route's wrapper reaches the card only when both halves hold: there was
// an error, and it was a 404. Without a failure that is not a 404, nothing
// distinguishes that from "there was an error" — and a wrapper widened to the
// looser reading would answer a 403 from a token without repository access, or
// a 500 from an instance in trouble, with "File Not Found", telling the model
// the file does not exist and sending it off to create one.
func TestActionSpecs_GetFailureOtherThanNotFound_StaysAnError(t *testing.T) {
	cases := []struct {
		name   string
		status int
	}{
		{name: "forbidden", status: http.StatusForbidden},
		{name: "server error", status: http.StatusInternalServerError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tc.status, `{"message":"denied"}`)
			}))
			byTool := fileSpecsByTool(t, ActionSpecs(client))

			result, err := byTool["gitlab_file_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "file_path": "main.go"})
			if err == nil {
				t.Fatalf("Route.Handler(gitlab_file_get) returned %#v, want an error for status %d", result, tc.status)
			}
			if _, ok := result.(fileNotFoundOutput); ok {
				t.Fatalf("Route.Handler(gitlab_file_get) reported the file missing for status %d", tc.status)
			}
		})
	}
}

// TestFileOptions_DiscoveryMetadata_CoversEveryRegisteredToolAndFallsBack
// asserts that every individual tool this domain registers carries its own
// discovery metadata, and that a name the table does not know still yields a
// usable spec.
//
// The two halves are one property seen from both sides. The metadata is keyed
// by the individual tool name, so renaming a tool in ActionSpecs without
// renaming its key silently drops that tool's aliases, usage guidance and
// description — the surface a model searches by — while leaving a spec that
// registers and runs. The fallback is what makes the loss silent, so it is
// asserted here beside the guard that would otherwise hide a rename.
func TestFileOptions_DiscoveryMetadata_CoversEveryRegisteredToolAndFallsBack(t *testing.T) {
	t.Run("every registered tool", func(t *testing.T) {
		byTool := fileSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(fileMockHandler))))
		for toolName := range byTool {
			t.Run(toolName, func(t *testing.T) {
				options := fileOptions(toolName)
				if options.IndividualTool.Description == "" {
					t.Errorf("%s has no individual-tool description", toolName)
				}
				if len(options.Aliases) < 2 {
					t.Errorf("%s aliases = %v, want the natural-language set rather than the fallback", toolName, options.Aliases)
				}
			})
		}
	})

	t.Run("tool the metadata table does not know", func(t *testing.T) {
		options := fileOptions("gitlab_file_not_in_the_table")
		if options.IndividualTool.Name != "gitlab_file_not_in_the_table" {
			t.Errorf("IndividualTool.Name = %q, want the name it was given", options.IndividualTool.Name)
		}
		if len(options.Aliases) != 1 || options.Aliases[0] != "gitlab_file_not_in_the_table" {
			t.Errorf("Aliases = %v, want just the tool name", options.Aliases)
		}
		if options.Usage == "" {
			t.Error("Usage is empty, want the generic sentence")
		}
		if options.IndividualTool.Description != "" {
			t.Errorf("Description = %q, want empty for a tool the table does not describe", options.IndividualTool.Description)
		}
	})
}

// TestCatalogSurface_DeleteConfirmDeclined verifies the CatalogSurface_DeleteConfirmDeclined handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	byTool := fileSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_file_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test file destructive confirmation.",
		Icons:       toolutil.IconFile,
	})

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
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	result, callErr := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_file_delete",
		Arguments: map[string]any{"project_id": "42", "file_path": "main.go", "branch": "main", "commit_message": "del"},
	})
	if callErr != nil {
		t.Fatalf("CallTool error: %v", callErr)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

// TestMarkdownForResult_FileRichContent verifies the MarkdownForResult_FileRichContent handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestMarkdownForResult_FileRichContent(t *testing.T) {
	tests := []struct {
		name   string
		result any
	}{
		{
			name: "file get image",
			result: Output{
				FilePath:        "logo.png",
				ContentCategory: "image",
				ImageData:       []byte{0x89, 0x50},
				ImageMIMEType:   "image/png",
			},
		},
		{
			name: "raw image",
			result: RawOutput{
				FilePath:        "logo.png",
				ContentCategory: "image",
				ImageData:       []byte{0x89, 0x50},
				ImageMIMEType:   "image/png",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callResult := toolutil.MarkdownForResult(tt.result)
			if callResult == nil {
				t.Fatal("MarkdownForResult returned nil")
			}
			if len(callResult.Content) != 2 {
				t.Fatalf("content items = %d, want 2", len(callResult.Content))
			}
			if _, ok := callResult.Content[0].(*mcp.TextContent); !ok {
				t.Fatalf("content[0] = %T, want *mcp.TextContent", callResult.Content[0])
			}
			image, ok := callResult.Content[1].(*mcp.ImageContent)
			if !ok {
				t.Fatalf("content[1] = %T, want *mcp.ImageContent", callResult.Content[1])
			}
			if image.MIMEType != "image/png" {
				t.Fatalf("image MIMEType = %q", image.MIMEType)
			}
		})
	}
}

// TestMarkdownForResult_FileTextBinaryAndNotFound verifies the file Markdown
// registry covers non-image file outputs and the canonical not-found output.
func TestMarkdownForResult_FileTextBinaryAndNotFound(t *testing.T) {
	tests := []struct {
		name        string
		result      any
		want        string
		wantIsError bool
	}{
		{
			name:   "file get text",
			result: Output{FilePath: "main.go", Ref: "main", Encoding: "base64", BlobID: "blob", Size: 13},
			want:   "## File: main.go",
		},
		{
			name:   "file get binary",
			result: Output{FilePath: "archive.zip", ContentCategory: "binary", Ref: "main", BlobID: "blob", Size: 42},
			want:   "content omitted",
		},
		{
			name:   "raw text",
			result: RawOutput{FilePath: "main.go", Size: 13, Content: "package main\n"},
			want:   "## Raw File: main.go",
		},
		{
			name:   "raw binary",
			result: RawOutput{FilePath: "archive.zip", ContentCategory: "binary", Size: 42},
			want:   "## Binary File: archive.zip",
		},
		{
			name:        "not found",
			result:      fileNotFoundOutput{Identifier: "src/missing.go@main"},
			want:        "File Not Found",
			wantIsError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callResult := toolutil.MarkdownForResult(tt.result)
			if callResult == nil {
				t.Fatal("MarkdownForResult returned nil")
			}
			if callResult.IsError != tt.wantIsError {
				t.Fatalf("IsError = %v, want %v", callResult.IsError, tt.wantIsError)
			}
			if len(callResult.Content) != 1 {
				t.Fatalf("content items = %d, want 1", len(callResult.Content))
			}
			content, ok := callResult.Content[0].(*mcp.TextContent)
			if !ok {
				t.Fatalf("content[0] = %T, want *mcp.TextContent", callResult.Content[0])
			}
			if !strings.Contains(content.Text, tt.want) {
				t.Fatalf("markdown missing %q:\n%s", tt.want, content.Text)
			}
		})
	}
}

func fileMockHandler(w http.ResponseWriter, r *http.Request) {
	content := base64.StdEncoding.EncodeToString([]byte("package main\n"))
	path := r.URL.Path

	if handleRepositoryFileContent(w, r, path, content) {
		return
	}
	if handleRepositoryFileMetadata(w, r, path) {
		return
	}
	http.NotFound(w, r)
}

func handleRepositoryFileContent(w http.ResponseWriter, r *http.Request, path, content string) bool {
	switch {
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/repository/files/main.go") && !strings.Contains(path, "/raw") && !strings.Contains(path, "/blame"):
		testutil.RespondJSON(w, http.StatusOK, `{
			"file_name":"main.go","file_path":"main.go","size":13,
			"encoding":"base64","content":"`+content+`",
			"ref":"main","blob_id":"b1","commit_id":"c1","last_commit_id":"c1"
		}`)
	case r.Method == http.MethodPost && strings.Contains(path, "/repository/files/"):
		testutil.RespondJSON(w, http.StatusCreated, `{"file_path":"new.txt","branch":"main"}`)
	case r.Method == http.MethodPut && strings.Contains(path, "/repository/files/"):
		testutil.RespondJSON(w, http.StatusOK, `{"file_path":"main.go","branch":"main"}`)
	case r.Method == http.MethodDelete && strings.Contains(path, "/repository/files/"):
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/blame"):
		testutil.RespondJSON(w, http.StatusOK, `[{"commit":{"id":"abc12345","message":"init","author_name":"A","author_email":"a@t.com"},"lines":["line1"]}]`)
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/raw"):
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("raw content"))
	default:
		return false
	}
	return true
}

func handleRepositoryFileMetadata(w http.ResponseWriter, r *http.Request, path string) bool {
	switch {
	case r.Method == http.MethodHead && strings.Contains(path, "/repository/files/") && !strings.HasSuffix(path, "/raw"):
		writeFileMetadataHeaders(w, fileMetadataHeaders{name: "main.go", path: "main.go", size: "13", blobID: "b1", commitID: "c1", sha: "sha", encoding: "base64", executable: "false"})
	case r.Method == http.MethodHead && strings.HasSuffix(path, "/raw"):
		writeFileMetadataHeaders(w, fileMetadataHeaders{name: "raw.go", path: "raw.go", size: "42", blobID: "b2", commitID: "c2", sha: "sha-raw", encoding: "text", executable: "true"})
	default:
		return false
	}
	return true
}

type fileMetadataHeaders struct {
	name       string
	path       string
	size       string
	blobID     string
	commitID   string
	sha        string
	encoding   string
	executable string
}

func writeFileMetadataHeaders(w http.ResponseWriter, headers fileMetadataHeaders) {
	w.Header().Set("X-Gitlab-File-Name", headers.name)
	w.Header().Set("X-Gitlab-File-Path", headers.path)
	w.Header().Set("X-Gitlab-Size", headers.size)
	w.Header().Set("X-Gitlab-Blob-Id", headers.blobID)
	w.Header().Set("X-Gitlab-Commit-Id", headers.commitID)
	w.Header().Set("X-Gitlab-Last-Commit-Id", headers.commitID)
	w.Header().Set("X-Gitlab-Content-Sha256", headers.sha)
	w.Header().Set("X-Gitlab-Encoding", headers.encoding)
	w.Header().Set("X-Gitlab-Ref", "main")
	w.Header().Set("X-Gitlab-Execute-Filemode", headers.executable)
	w.WriteHeader(http.StatusOK)
}

func fileSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		toolName := spec.IndividualTool.Name
		if toolName == "" {
			t.Fatalf("spec %s missing IndividualTool.Name", spec.Name)
		}
		if _, exists := byTool[toolName]; exists {
			t.Fatalf("duplicate individual tool %q", toolName)
		}
		byTool[toolName] = spec
	}
	return byTool
}
