// read_test.go contains unit tests for the repository submodule MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package repositorysubmodules

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// submoduleGitmodules is the one-entry .gitmodules the read fixtures below
// serve: a submodule at libs/core-module pointing at org/project.
const submoduleGitmodules = `[submodule "libs/core-module"]
	path = libs/core-module
	url = git@gitlab.example.com:org/project.git
`

// TestRead_Success verifies Read when success.
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

// TestRead_Output_CarriesEveryFieldOfTheFileGitLabSent holds the whole output
// against a response in which no two values agree.
//
// The success test above reads five of the eight fields, so the size, the path
// and the encoding could each be dropped or swapped for a neighbor and nothing
// would fail; an assignment has no branch to flip, so neither quality gate
// reaches this class at all.
func TestRead_Output_CarriesEveryFieldOfTheFileGitLabSent(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, ".gitmodules"):
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{
				"file_name": ".gitmodules", "encoding": "text", "content": %q, "ref": "main"
			}`, submoduleGitmodules))
		case strings.Contains(r.URL.Path, "/repository/tree"):
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id": "1a2b3c4d5e6f7a8b", "name": "core-module", "type": "commit", "path": "libs/core-module", "mode": "160000"}
			]`)
		default:
			testutil.RespondJSON(w, http.StatusOK, `{
				"file_name": "parser.c",
				"file_path": "src/parser.c",
				"size": 4096,
				"encoding": "text",
				"content": "int parse(void) { return 0; }",
				"ref": "1a2b3c4d5e6f7a8b"
			}`)
		}
	})

	client := testutil.NewTestClient(t, handler)
	got, err := Read(t.Context(), client, ReadInput{
		ProjectID:     "42",
		SubmodulePath: "libs/core-module",
		FilePath:      "src/parser.c",
		Ref:           "main",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := ReadOutput{
		FileName:        "parser.c",
		FilePath:        "src/parser.c",
		SubmodulePath:   "libs/core-module",
		ResolvedProject: "org/project",
		CommitSHA:       "1a2b3c4d5e6f7a8b",
		Size:            4096,
		Content:         "int parse(void) { return 0; }",
		Encoding:        "text",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("read output mismatch:\ngot:  %+v\nwant: %+v", got, want)
	}
}

// TestRead_TreeListing_AsksTheParentDirectoryAtTheRequestedRef pins the tree
// request the pinned-commit lookup builds: scoped to the submodule's parent
// directory, at the ref the caller named.
//
// The listing is not recursive, so a request that lost its path answers with
// the repository root and a submodule under libs/ is never found; one that lost
// its ref answers at the default branch, so the commit read is not the one the
// caller's ref pins and the file comes back from the wrong revision.
func TestRead_TreeListing_AsksTheParentDirectoryAtTheRequestedRef(t *testing.T) {
	var treePath, treeRef string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, ".gitmodules"):
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{
				"file_name": ".gitmodules", "encoding": "text", "content": %q, "ref": "release-3-1"
			}`, submoduleGitmodules))
		case strings.Contains(r.URL.Path, "/repository/tree"):
			treePath = r.URL.Query().Get("path")
			treeRef = r.URL.Query().Get("ref")
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id": "sha1", "name": "core-module", "type": "commit", "path": "libs/core-module", "mode": "160000"}
			]`)
		default:
			testutil.RespondJSON(w, http.StatusOK, `{"file_name": "f.txt", "encoding": "text", "content": "hi"}`)
		}
	})

	client := testutil.NewTestClient(t, handler)
	if _, err := Read(t.Context(), client, ReadInput{
		ProjectID:     "42",
		SubmodulePath: "libs/core-module",
		FilePath:      "f.txt",
		Ref:           "release-3-1",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if treePath != "libs" {
		t.Errorf("tree listing asked for path %q, want libs", treePath)
	}
	if treeRef != "release-3-1" {
		t.Errorf("tree listing asked at ref %q, want release-3-1", treeRef)
	}
}

// TestRead_TreeCarriesAnotherSubmodulesCommit_PinsOnlyItsOwn verifies that the
// commit read out of the tree is the node at the submodule's own path, and not
// the first node of type "commit" the listing happens to carry.
//
// A directory holding two submodules answers with a commit node for each, so
// the listing that resolves libs/core-module also carries libs/vendor-module;
// taking the wrong one reads the file out of a real commit of another project
// and reports it under this submodule's name, with nothing in the output saying
// so.
func TestRead_TreeCarriesAnotherSubmodulesCommit_PinsOnlyItsOwn(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, ".gitmodules"):
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{
				"file_name": ".gitmodules", "encoding": "text", "content": %q, "ref": "main"
			}`, submoduleGitmodules))
		case strings.Contains(r.URL.Path, "/repository/tree"):
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id": "0000vendorsha", "name": "vendor-module", "type": "commit", "path": "libs/vendor-module", "mode": "160000"},
				{"id": "1111ourown", "name": "core-module", "type": "commit", "path": "libs/core-module", "mode": "160000"}
			]`)
		default:
			testutil.RespondJSON(w, http.StatusOK, `{"file_name": "f.txt", "encoding": "text", "content": "hi"}`)
		}
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Read(t.Context(), client, ReadInput{
		ProjectID:     "42",
		SubmodulePath: "libs/core-module",
		FilePath:      "f.txt",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.CommitSHA != "1111ourown" {
		t.Errorf("CommitSHA = %q, want 1111ourown: the vendor module's commit is not this submodule's pointer", out.CommitSHA)
	}
}

// TestRead_TreeNodeAtTheSubmodulePathIsNotACommit_IsRefused verifies that a
// node sitting at the submodule's path but carrying any other type is not read
// as the pointer.
//
// It is the state a repository is in when a submodule has been replaced by an
// ordinary file or directory in the ref being read: the path still resolves,
// and the object id behind it names a blob or a tree rather than the commit of
// another project. Answering with it would hand the caller a SHA that cannot be
// fetched from the submodule's project, so the refusal is the right answer and
// the error says which shape was expected.
func TestRead_TreeNodeAtTheSubmodulePathIsNotACommit_IsRefused(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/tree") {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id": "b10bb10b", "name": "core-module", "type": "blob", "path": "libs/core-module", "mode": "100644"}
			]`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{
			"file_name": ".gitmodules", "encoding": "text", "content": %q, "ref": "main"
		}`, submoduleGitmodules))
	})

	client := testutil.NewTestClient(t, handler)
	_, err := Read(t.Context(), client, ReadInput{
		ProjectID:     "42",
		SubmodulePath: "libs/core-module",
		FilePath:      "f.txt",
	})
	if err == nil {
		t.Fatal("expected an error when the node at the submodule path is a blob")
	}
	if !strings.Contains(err.Error(), "not found as a tree entry") {
		t.Errorf("error = %v, want it to say no commit-type entry was found", err)
	}
	if strings.Contains(err.Error(), "b10bb10b") {
		t.Errorf("error names the blob's object id, which is not a commit this submodule is pinned to: %v", err)
	}
}

// TestRead_SubmoduleNotFound verifies Read when submodule not found.
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

// TestRead_EmptyProjectID verifies Read when empty project ID.
func TestRead_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Read(t.Context(), client, ReadInput{SubmodulePath: "lib", FilePath: "f.txt"})
	if err == nil {
		t.Fatal("expected error for empty project_id")
	}
}

// TestRead_EmptySubmodulePath verifies Read when empty submodule path.
func TestRead_EmptySubmodulePath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Read(t.Context(), client, ReadInput{ProjectID: "42", FilePath: "f.txt"})
	if err == nil {
		t.Fatal("expected error for empty submodule_path")
	}
}

// TestRead_EmptyFilePath verifies Read when empty file path.
func TestRead_EmptyFilePath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Read(t.Context(), client, ReadInput{ProjectID: "42", SubmodulePath: "lib"})
	if err == nil {
		t.Fatal("expected error for empty file_path")
	}
}

// TestRead_CancelledContext verifies Read when cancelled context.
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

// TestRead_Base64Content verifies Read when base 64 content.
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

// TestFormatReadMarkdown pins the whole card of a file read out of a
// submodule: the paths and the abbreviated commit as code spans, and the body
// fenced with the language the file's own extension names.
func TestFormatReadMarkdown(t *testing.T) {
	got := renderedText(t, FormatReadMarkdown(ReadOutput{
		FileName:        "main.c",
		FilePath:        "src/main.c",
		SubmodulePath:   "libs/core-module",
		ResolvedProject: "org/project",
		CommitSHA:       "abc123def456789",
		Size:            42,
		Content:         "int main() {}",
		Encoding:        "text",
	}))

	want := "## File from Submodule\n\n" +
		"- **Submodule**: `libs/core-module`\n" +
		"- **Resolved Project**: org/project\n" +
		"- **Commit**: `abc123de`\n" +
		"- **File**: `src/main.c`\n" +
		"- **Size (bytes)**: 42\n" +
		"- **Encoding**: text\n" +
		"\n### Content\n\n" +
		"```c\nint main() {}\n```\n" +
		submoduleReadHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// listSubmodulePaths.

// TestList_SubmodulePaths verifies List when submodule paths.
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

// TestMinLen verifies MinLen.
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

// Action route integration test for list.

// TestActionSpecs_ListRoute verifies that the list route can be called directly.
func TestActionSpecs_ListRoute(t *testing.T) {
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
	byTool := repositorySubmoduleSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_list_repository_submodules"].Route.Handler(t.Context(), map[string]any{"project_id": "42"})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	if result == nil {
		t.Fatal("Route.Handler returned nil")
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

// TestActionSpecs_ReadRoute verifies the read route can be called directly.
func TestActionSpecs_ReadRoute(t *testing.T) {
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
	byTool := repositorySubmoduleSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_read_repository_submodule_file"].Route.Handler(t.Context(), map[string]any{
		"project_id":     "42",
		"submodule_path": "lib",
		"file_path":      "f.txt",
	})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	if result == nil {
		t.Fatal("Route.Handler returned nil")
	}
}

// TestRead_DefaultsRefToHEAD verifies Read resolves .gitmodules with the HEAD
// ref alias when the input omits ref, mirroring the documented default.
func TestRead_DefaultsRefToHEAD(t *testing.T) {
	var capturedRef string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/") {
			capturedRef = r.URL.Query().Get("ref")
		}
		http.NotFound(w, r)
	})

	client := testutil.NewTestClient(t, handler)
	if _, err := Read(t.Context(), client, ReadInput{ProjectID: "42", SubmodulePath: "libs/dep", FilePath: "main.c"}); err == nil {
		t.Fatal("expected error from missing .gitmodules")
	}
	if capturedRef != "HEAD" {
		t.Errorf("GetFile ref = %q, want HEAD", capturedRef)
	}
}
