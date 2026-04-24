// read_test.go contains unit tests for the repository submodule MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.

package repositorysubmodules

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestRead_Success verifies that Read handles the success scenario correctly.
func TestRead_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// .gitmodules request
		if strings.Contains(path, "/repository/files/%2Egitmodules") || strings.Contains(path, "/repository/files/.gitmodules") {
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{
				"file_name": ".gitmodules",
				"file_path": ".gitmodules",
				"size": 100,
				"encoding": "text",
				"content": %q,
				"ref": "main"
			}`, `[submodule "libs/core-module"]
	path = libs/core-module
	url = git@gitlab.example.com:org/project.git
`))
			return
		}

		// Tree request for libs directory
		if strings.Contains(path, "/repository/tree") {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id": "abc123def456", "name": "core-module", "type": "commit", "path": "libs/core-module", "mode": "160000"}
			]`)
			return
		}

		// File request from resolved submodule project
		if strings.Contains(path, "/repository/files/") {
			ref := r.URL.Query().Get("ref")
			if ref == "abc123def456" {
				testutil.RespondJSON(w, http.StatusOK, `{
					"file_name": "main.c",
					"file_path": "src/main.c",
					"size": 42,
					"encoding": "text",
					"content": "int main() { return 0; }",
					"ref": "abc123def456",
					"blob_id": "blob1",
					"commit_id": "abc123def456",
					"last_commit_id": "abc123def456"
				}`)
				return
			}
		}

		http.NotFound(w, r)
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Read(t.Context(), client, ReadInput{
		ProjectID:     "42",
		SubmodulePath: "libs/core-module",
		FilePath:      "src/main.c",
		Ref:           "main",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.FileName != "main.c" {
		t.Errorf("expected file_name 'main.c', got %q", out.FileName)
	}
	if out.Content != "int main() { return 0; }" {
		t.Errorf("unexpected content: %q", out.Content)
	}
	if out.ResolvedProject != "org/project" {
		t.Errorf("expected resolved project 'org/project', got %q", out.ResolvedProject)
	}
	if out.CommitSHA != "abc123def456" {
		t.Errorf("expected commit SHA 'abc123def456', got %q", out.CommitSHA)
	}
	if out.SubmodulePath != "libs/core-module" {
		t.Errorf("expected submodule_path 'libs/core-module', got %q", out.SubmodulePath)
	}
}

// TestRead_SubmoduleNotFound verifies that Read handles the submodule not found scenario correctly.
func TestRead_SubmoduleNotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/") {
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{
				"file_name": ".gitmodules",
				"file_path": ".gitmodules",
				"size": 50,
				"encoding": "text",
				"content": %q,
				"ref": "main"
			}`, `[submodule "lib"]
	path = lib
	url = git@host:group/lib.git
`))
			return
		}
		http.NotFound(w, r)
	})

	client := testutil.NewTestClient(t, handler)
	_, err := Read(t.Context(), client, ReadInput{
		ProjectID:     "42",
		SubmodulePath: "nonexistent",
		FilePath:      "file.txt",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent submodule")
	}
	if !strings.Contains(err.Error(), "not found in .gitmodules") {
		t.Errorf("expected 'not found in .gitmodules' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "lib") {
		t.Errorf("expected available submodule paths in error, got: %v", err)
	}
}

// TestRead_EmptyProjectID verifies that Read handles the empty project i d scenario correctly.
func TestRead_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Read(t.Context(), client, ReadInput{SubmodulePath: "lib", FilePath: "f.txt"})
	if err == nil {
		t.Fatal("expected error for empty project_id")
	}
}

// TestRead_EmptySubmodulePath verifies that Read handles the empty submodule path scenario correctly.
func TestRead_EmptySubmodulePath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Read(t.Context(), client, ReadInput{ProjectID: "42", FilePath: "f.txt"})
	if err == nil {
		t.Fatal("expected error for empty submodule_path")
	}
}

// TestRead_EmptyFilePath verifies that Read handles the empty file path scenario correctly.
func TestRead_EmptyFilePath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Read(t.Context(), client, ReadInput{ProjectID: "42", SubmodulePath: "lib"})
	if err == nil {
		t.Fatal("expected error for empty file_path")
	}
}

// TestRead_CancelledContext verifies that Read handles the cancelled context scenario correctly.
func TestRead_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Read(ctx, client, ReadInput{ProjectID: "42", SubmodulePath: "lib", FilePath: "f.txt"})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// TestRead_Base64Content verifies that Read handles the base64 content scenario correctly.
func TestRead_Base64Content(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/%2Egitmodules") || strings.Contains(r.URL.Path, "/repository/files/.gitmodules") {
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{
				"file_name": ".gitmodules",
				"file_path": ".gitmodules",
				"size": 50,
				"encoding": "text",
				"content": %q,
				"ref": "main"
			}`, `[submodule "lib"]
	path = lib
	url = git@host:group/lib.git
`))
			return
		}
		if strings.Contains(r.URL.Path, "/repository/tree") {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id": "sha123", "name": "lib", "type": "commit", "path": "lib", "mode": "160000"}
			]`)
			return
		}
		if strings.Contains(r.URL.Path, "/repository/files/") {
			// Return base64-encoded content: "hello world"
			testutil.RespondJSON(w, http.StatusOK, `{
				"file_name": "readme.txt",
				"file_path": "readme.txt",
				"size": 11,
				"encoding": "base64",
				"content": "aGVsbG8gd29ybGQ=",
				"ref": "sha123"
			}`)
			return
		}
		http.NotFound(w, r)
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Read(t.Context(), client, ReadInput{
		ProjectID:     "42",
		SubmodulePath: "lib",
		FilePath:      "readme.txt",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Content != "hello world" {
		t.Errorf("expected decoded content 'hello world', got %q", out.Content)
	}
}

// FormatReadMarkdown tests.

// TestFormatReadMarkdown verifies the behavior of format read markdown.
func TestFormatReadMarkdown(t *testing.T) {
	out := ReadOutput{
		FileName:        "main.c",
		FilePath:        "src/main.c",
		SubmodulePath:   "libs/core-module",
		ResolvedProject: "org/project",
		CommitSHA:       "abc123def456789",
		Size:            42,
		Content:         "int main() {}",
	}
	r := FormatReadMarkdown(out)
	if r == nil {
		t.Fatal("expected non-nil result")
	}
	tc, ok := r.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if !strings.Contains(tc.Text, "libs/core-module") {
		t.Error("expected submodule path")
	}
	if !strings.Contains(tc.Text, "org/project") {
		t.Error("expected resolved project")
	}
	if !strings.Contains(tc.Text, "abc123de") {
		t.Error("expected truncated commit SHA")
	}
	if !strings.Contains(tc.Text, "int main() {}") {
		t.Error("expected file content")
	}
	if !strings.Contains(tc.Text, "```c") {
		t.Error("expected code block with extension")
	}
}

// listSubmodulePaths.

// TestList_SubmodulePaths verifies the behavior of list submodule paths.
func TestList_SubmodulePaths(t *testing.T) {
	entries := []SubmoduleEntry{
		{Path: "lib"},
		{Path: "libs/gen3"},
	}
	got := listSubmodulePaths(entries)
	if got != "lib, libs/gen3" {
		t.Errorf("unexpected result: %q", got)
	}
}

// minLen.

// TestMinLen verifies the behavior of min len.
func TestMinLen(t *testing.T) {
	if minLen(3, 5) != 3 {
		t.Error("expected 3")
	}
	if minLen(10, 2) != 2 {
		t.Error("expected 2")
	}
}

// TestRead_Base64Gitmodules verifies the Read path when .gitmodules is returned
// with base64 encoding (exercises the base64 decode branch in resolveSubmoduleProject).
func TestRead_Base64Gitmodules(t *testing.T) {
	gitmodulesContent := `[submodule "lib"]
	path = lib
	url = git@host:group/lib.git
`
	encoded := base64.StdEncoding.EncodeToString([]byte(gitmodulesContent))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/%2Egitmodules") || strings.Contains(r.URL.Path, "/repository/files/.gitmodules") {
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{
				"file_name": ".gitmodules",
				"file_path": ".gitmodules",
				"size": 50,
				"encoding": "base64",
				"content": %q,
				"ref": "main"
			}`, encoded))
			return
		}
		if strings.Contains(r.URL.Path, "/repository/tree") {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id": "sha999", "name": "lib", "type": "commit", "path": "lib", "mode": "160000"}
			]`)
			return
		}
		if strings.Contains(r.URL.Path, "/repository/files/") {
			testutil.RespondJSON(w, http.StatusOK, `{
				"file_name": "f.txt",
				"file_path": "f.txt",
				"size": 5,
				"encoding": "text",
				"content": "hello",
				"ref": "sha999"
			}`)
			return
		}
		http.NotFound(w, r)
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Read(t.Context(), client, ReadInput{
		ProjectID:     "42",
		SubmodulePath: "lib",
		FilePath:      "f.txt",
		Ref:           "main",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ResolvedProject != "group/lib" {
		t.Errorf("expected resolved project 'group/lib', got %q", out.ResolvedProject)
	}
}

// TestRead_UnresolvableSubmoduleURL verifies that Read returns an error when
// the submodule URL in .gitmodules cannot be resolved to a project path.
func TestRead_UnresolvableSubmoduleURL(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/") {
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{
				"file_name": ".gitmodules",
				"file_path": ".gitmodules",
				"size": 50,
				"encoding": "text",
				"content": %q,
				"ref": "main"
			}`, `[submodule "bad"]
	path = bad
	url = ://invalid
`))
			return
		}
		http.NotFound(w, r)
	})

	client := testutil.NewTestClient(t, handler)
	_, err := Read(t.Context(), client, ReadInput{
		ProjectID:     "42",
		SubmodulePath: "bad",
		FilePath:      "f.txt",
	})
	if err == nil {
		t.Fatal("expected error for unresolvable URL")
	}
	if !strings.Contains(err.Error(), "could not resolve project path") {
		t.Errorf("expected resolve error, got: %v", err)
	}
}

// TestRead_TreeEntryNotFound verifies that Read returns an error when the
// submodule path is not found as a "commit" entry in the repository tree.
func TestRead_TreeEntryNotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/") {
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{
				"file_name": ".gitmodules",
				"file_path": ".gitmodules",
				"size": 50,
				"encoding": "text",
				"content": %q,
				"ref": "main"
			}`, `[submodule "lib"]
	path = lib
	url = git@host:group/lib.git
`))
			return
		}
		if strings.Contains(r.URL.Path, "/repository/tree") {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id": "blobsha", "name": "other", "type": "blob", "path": "other", "mode": "100644"}
			]`)
			return
		}
		http.NotFound(w, r)
	})

	client := testutil.NewTestClient(t, handler)
	_, err := Read(t.Context(), client, ReadInput{
		ProjectID:     "42",
		SubmodulePath: "lib",
		FilePath:      "f.txt",
	})
	if err == nil {
		t.Fatal("expected error when submodule not in tree")
	}
	if !strings.Contains(err.Error(), "not found as a tree entry") {
		t.Errorf("expected tree entry error, got: %v", err)
	}
}

// TestRead_GitmodulesGetFileError verifies that Read returns an error when
// the .gitmodules file cannot be retrieved from the API.
func TestRead_GitmodulesGetFileError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		http.NotFound(w, r)
	})

	client := testutil.NewTestClient(t, handler)
	_, err := Read(t.Context(), client, ReadInput{
		ProjectID:     "42",
		SubmodulePath: "lib",
		FilePath:      "f.txt",
	})
	if err == nil {
		t.Fatal("expected error when .gitmodules not found")
	}
	if !strings.Contains(err.Error(), "could not read .gitmodules") {
		t.Errorf("expected .gitmodules error, got: %v", err)
	}
}

// TestRead_TreeListError verifies that Read returns an error when the
// tree listing API call fails.
func TestRead_TreeListError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/") {
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{
				"file_name": ".gitmodules",
				"file_path": ".gitmodules",
				"size": 50,
				"encoding": "text",
				"content": %q,
				"ref": "main"
			}`, `[submodule "lib"]
	path = lib
	url = git@host:group/lib.git
`))
			return
		}
		if strings.Contains(r.URL.Path, "/repository/tree") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		http.NotFound(w, r)
	})

	client := testutil.NewTestClient(t, handler)
	_, err := Read(t.Context(), client, ReadInput{
		ProjectID:     "42",
		SubmodulePath: "lib",
		FilePath:      "f.txt",
	})
	if err == nil {
		t.Fatal("expected error when tree listing fails")
	}
	if !strings.Contains(err.Error(), "could not list tree") {
		t.Errorf("expected tree list error, got: %v", err)
	}
}

// CallTool integration test for list.

// TestList_CallThroughMCP verifies that List handles the call through m c p scenario correctly.
func TestList_CallThroughMCP(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/") {
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{
				"file_name": ".gitmodules",
				"file_path": ".gitmodules",
				"size": 50,
				"encoding": "text",
				"content": %q,
				"ref": "main"
			}`, `[submodule "lib"]
	path = lib
	url = git@host:group/lib.git
`))
			return
		}
		if strings.Contains(r.URL.Path, "/repository/tree") {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id": "sha1", "name": "lib", "type": "commit", "path": "lib", "mode": "160000"}
			]`)
			return
		}
		http.NotFound(w, r)
	})

	client := testutil.NewTestClient(t, handler)
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
		Name:      "gitlab_list_repository_submodules",
		Arguments: map[string]any{"project_id": "42"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result.IsError {
		t.Fatal("CallTool returned IsError=true")
	}
}

// TestRead_GetFileError verifies that Read returns a wrapped error when the
// final file fetch from the resolved submodule project fails (e.g. 404).
func TestRead_GetFileError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/%2Egitmodules") || strings.Contains(r.URL.Path, "/repository/files/.gitmodules") {
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{
				"file_name": ".gitmodules",
				"file_path": ".gitmodules",
				"size": 50,
				"encoding": "text",
				"content": %q,
				"ref": "main"
			}`, `[submodule "lib"]
	path = lib
	url = git@host:group/lib.git
`))
			return
		}
		if strings.Contains(r.URL.Path, "/repository/tree") {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id": "sha123", "name": "lib", "type": "commit", "path": "lib", "mode": "160000"}
			]`)
			return
		}
		// The final file fetch returns 404
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 File Not Found"}`)
	})

	client := testutil.NewTestClient(t, handler)
	_, err := Read(t.Context(), client, ReadInput{
		ProjectID:     "42",
		SubmodulePath: "lib",
		FilePath:      "missing.txt",
	})
	if err == nil {
		t.Fatal("expected error for file not found, got nil")
	}
	if !strings.Contains(err.Error(), "missing.txt") {
		t.Errorf("expected file name in error, got: %v", err)
	}
}

// TestRead_InvalidBase64FileContent verifies that Read returns an error when
// the file content in the submodule has encoding "base64" but invalid base64 data.
func TestRead_InvalidBase64FileContent(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/%2Egitmodules") || strings.Contains(r.URL.Path, "/repository/files/.gitmodules") {
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{
				"file_name": ".gitmodules",
				"file_path": ".gitmodules",
				"size": 50,
				"encoding": "text",
				"content": %q,
				"ref": "main"
			}`, `[submodule "lib"]
	path = lib
	url = git@host:group/lib.git
`))
			return
		}
		if strings.Contains(r.URL.Path, "/repository/tree") {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id": "sha123", "name": "lib", "type": "commit", "path": "lib", "mode": "160000"}
			]`)
			return
		}
		// Return file with base64 encoding but corrupted content
		testutil.RespondJSON(w, http.StatusOK, `{
			"file_name": "readme.txt",
			"file_path": "readme.txt",
			"size": 11,
			"encoding": "base64",
			"content": "!!!not-valid-base64!!!",
			"ref": "sha123"
		}`)
	})

	client := testutil.NewTestClient(t, handler)
	_, err := Read(t.Context(), client, ReadInput{
		ProjectID:     "42",
		SubmodulePath: "lib",
		FilePath:      "readme.txt",
	})
	if err == nil {
		t.Fatal("expected error for invalid base64 file content, got nil")
	}
	if !strings.Contains(err.Error(), "decode base64 content") {
		t.Errorf("expected 'decode base64 content' in error, got: %v", err)
	}
}

// TestRead_InvalidBase64Gitmodules verifies that Read returns an error when
// .gitmodules is returned with base64 encoding but has invalid base64 data.
func TestRead_InvalidBase64Gitmodules(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/%2Egitmodules") || strings.Contains(r.URL.Path, "/repository/files/.gitmodules") {
			testutil.RespondJSON(w, http.StatusOK, `{
				"file_name": ".gitmodules",
				"file_path": ".gitmodules",
				"size": 50,
				"encoding": "base64",
				"content": "!!!corrupt-base64!!!",
				"ref": "main"
			}`)
			return
		}
		http.NotFound(w, r)
	})

	client := testutil.NewTestClient(t, handler)
	_, err := Read(t.Context(), client, ReadInput{
		ProjectID:     "42",
		SubmodulePath: "lib",
		FilePath:      "readme.txt",
	})
	if err == nil {
		t.Fatal("expected error for invalid base64 .gitmodules, got nil")
	}
	if !strings.Contains(err.Error(), "decode .gitmodules") {
		t.Errorf("expected 'decode .gitmodules' in error, got: %v", err)
	}
}

// TestRead_CallThroughMCP verifies the MCP round-trip for the
// gitlab_read_repository_submodule_file tool handler.
func TestRead_CallThroughMCP(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/%2Egitmodules") || strings.Contains(r.URL.Path, "/repository/files/.gitmodules") {
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{
				"file_name": ".gitmodules",
				"file_path": ".gitmodules",
				"size": 50,
				"encoding": "text",
				"content": %q,
				"ref": "main"
			}`, `[submodule "lib"]
	path = lib
	url = git@host:group/lib.git
`))
			return
		}
		if strings.Contains(r.URL.Path, "/repository/tree") {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id": "sha1", "name": "lib", "type": "commit", "path": "lib", "mode": "160000"}
			]`)
			return
		}
		if strings.Contains(r.URL.Path, "/repository/files/") {
			testutil.RespondJSON(w, http.StatusOK, `{
				"file_name": "f.txt",
				"file_path": "f.txt",
				"size": 5,
				"encoding": "text",
				"content": "hello",
				"ref": "sha1"
			}`)
			return
		}
		http.NotFound(w, r)
	})

	client := testutil.NewTestClient(t, handler)
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
		Name: "gitlab_read_repository_submodule_file",
		Arguments: map[string]any{
			"project_id":     "42",
			"submodule_path": "lib",
			"file_path":      "f.txt",
		},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result.IsError {
		t.Fatal("CallTool returned IsError=true")
	}
}
