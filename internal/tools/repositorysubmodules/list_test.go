// list_test.go contains unit tests for the repository submodule MCP tool handlers.
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

const sampleGitmodules = `[submodule "dsp_up_spi_common_module"]
	path = dsp_up_spi_common_module
	url = git@gitlab.example.com:hardware/dsp/common-modules.git
[submodule "bootloader"]
	path = bootloader
	url = git@gitlab.example.com:embedded/cloud-bootloader.git
[submodule "MCF/mcf_gen3"]
	path = MCF/mcf_gen3
	url = git@gitlab.example.com:engineering/embedded/firmware/module-v3.git
[submodule ".gitlab/gitlab-templates"]
	path = .gitlab/gitlab-templates
	url = git@gitlab.example.com:engineering/docs/gitlab-templates.git
`

// parseGitmodules unit tests.

// TestParseGitmodules_Success validates parse gitmodules across multiple scenarios using table-driven subtests
// for the success case.
func TestParseGitmodules_Success(t *testing.T) {
	entries := parseGitmodules(sampleGitmodules)
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	tests := []struct {
		name            string
		path            string
		url             string
		resolvedProject string
	}{
		{"dsp_up_spi_common_module", "dsp_up_spi_common_module", "git@gitlab.example.com:hardware/dsp/common-modules.git", "hardware/dsp/common-modules"},
		{"bootloader", "bootloader", "git@gitlab.example.com:embedded/cloud-bootloader.git", "embedded/cloud-bootloader"},
		{"MCF/mcf_gen3", "MCF/mcf_gen3", "git@gitlab.example.com:engineering/embedded/firmware/module-v3.git", "engineering/embedded/firmware/module-v3"},
		{".gitlab/gitlab-templates", ".gitlab/gitlab-templates", "git@gitlab.example.com:engineering/docs/gitlab-templates.git", "engineering/docs/gitlab-templates"},
	}

	for i, tt := range tests {
		e := entries[i]
		if e.Name != tt.name {
			t.Errorf("entry[%d] name: got %q, want %q", i, e.Name, tt.name)
		}
		if e.Path != tt.path {
			t.Errorf("entry[%d] path: got %q, want %q", i, e.Path, tt.path)
		}
		if e.URL != tt.url {
			t.Errorf("entry[%d] url: got %q, want %q", i, e.URL, tt.url)
		}
		if e.ResolvedProject != tt.resolvedProject {
			t.Errorf("entry[%d] resolved: got %q, want %q", i, e.ResolvedProject, tt.resolvedProject)
		}
	}
}

// TestParseGitmodules_Empty verifies that ParseGitmodules handles the empty scenario correctly.
func TestParseGitmodules_Empty(t *testing.T) {
	entries := parseGitmodules("")
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

// TestParseGitmodules_HTTPSUrl verifies that ParseGitmodules handles the h t t p s url scenario correctly.
func TestParseGitmodules_HTTPSUrl(t *testing.T) {
	content := `[submodule "lib"]
	path = lib
	url = https://gitlab.com/group/project.git
`
	entries := parseGitmodules(content)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].ResolvedProject != "group/project" {
		t.Errorf("expected resolved project 'group/project', got %q", entries[0].ResolvedProject)
	}
}

// TestParseGitmodules_SSHUrl verifies that ParseGitmodules handles the s s h url scenario correctly.
func TestParseGitmodules_SSHUrl(t *testing.T) {
	content := `[submodule "lib"]
	path = lib
	url = ssh://git@gitlab.com/group/subgroup/project.git
`
	entries := parseGitmodules(content)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].ResolvedProject != "group/subgroup/project" {
		t.Errorf("expected resolved project 'group/subgroup/project', got %q", entries[0].ResolvedProject)
	}
}

// resolveProjectPath unit tests.

// TestResolveProjectPath validates resolve project path across multiple scenarios using table-driven subtests.
func TestResolveProjectPath(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"SCP-style", "git@gitlab.example.com:group/project.git", "group/project"},
		{"SCP nested", "git@gitlab.example.com:a/b/c/project.git", "a/b/c/project"},
		{"HTTPS", "https://gitlab.com/group/project.git", "group/project"},
		{"HTTPS no .git", "https://gitlab.com/group/project", "group/project"},
		{"SSH scheme", "ssh://git@gitlab.com/group/project.git", "group/project"},
		{"URL parse error", "://bad\x7f", ""},
		{"Empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveProjectPath(tt.url)
			if got != tt.want {
				t.Errorf("resolveProjectPath(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

// parentDir unit tests.

// TestParentDir validates parent dir across multiple scenarios using table-driven subtests.
func TestParentDir(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"MCF/mcf_gen3", "MCF"},
		{".gitlab/gitlab-templates", ".gitlab"},
		{"bootloader", ""},
		{"a/b/c", "a/b"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := parentDir(tt.path)
			if got != tt.want {
				t.Errorf("parentDir(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// List integration tests.

// TestList_Success verifies that List handles the success scenario correctly.
func TestList_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// .gitmodules request
		if strings.Contains(path, "/repository/files/") {
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{
				"file_name": ".gitmodules",
				"file_path": ".gitmodules",
				"size": 200,
				"encoding": "text",
				"content": %q,
				"ref": "main"
			}`, sampleGitmodules))
			return
		}

		// Tree requests: return commit-type entries
		if strings.Contains(path, "/repository/tree") {
			treePath := r.URL.Query().Get("path")
			switch treePath {
			case "MCF":
				testutil.RespondJSON(w, http.StatusOK, `[
					{"id": "aaa111", "name": "mcf_gen3", "type": "commit", "path": "MCF/mcf_gen3", "mode": "160000"}
				]`)
			case ".gitlab":
				testutil.RespondJSON(w, http.StatusOK, `[
					{"id": "bbb222", "name": "gitlab-templates", "type": "commit", "path": ".gitlab/gitlab-templates", "mode": "160000"}
				]`)
			default:
				// Root-level submodules
				testutil.RespondJSON(w, http.StatusOK, `[
					{"id": "ccc333", "name": "dsp_up_spi_common_module", "type": "commit", "path": "dsp_up_spi_common_module", "mode": "160000"},
					{"id": "ddd444", "name": "bootloader", "type": "commit", "path": "bootloader", "mode": "160000"},
					{"id": "eee555", "name": "src", "type": "tree", "path": "src", "mode": "040000"}
				]`)
			}
			return
		}

		http.NotFound(w, r)
	})

	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Count != 4 {
		t.Errorf("expected 4 submodules, got %d", out.Count)
	}

	// Verify commit SHAs were enriched
	shaMap := make(map[string]string)
	for _, s := range out.Submodules {
		shaMap[s.Path] = s.CommitSHA
	}
	if shaMap["dsp_up_spi_common_module"] != "ccc333" {
		t.Errorf("dsp_up_spi_common_module SHA: got %q", shaMap["dsp_up_spi_common_module"])
	}
	if shaMap["bootloader"] != "ddd444" {
		t.Errorf("bootloader SHA: got %q", shaMap["bootloader"])
	}
	if shaMap["MCF/mcf_gen3"] != "aaa111" {
		t.Errorf("MCF/mcf_gen3 SHA: got %q", shaMap["MCF/mcf_gen3"])
	}
	if shaMap[".gitlab/gitlab-templates"] != "bbb222" {
		t.Errorf(".gitlab/gitlab-templates SHA: got %q", shaMap[".gitlab/gitlab-templates"])
	}
}

// TestList_NoGitmodules verifies that List handles the no gitmodules scenario correctly.
func TestList_NoGitmodules(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"404 File Not Found"}`)
	})

	client := testutil.NewTestClient(t, handler)
	_, err := List(t.Context(), client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for missing .gitmodules, got nil")
	}
}

// TestList_EmptyProjectID verifies that List handles the empty project i d scenario correctly.
func TestList_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := List(t.Context(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for empty project_id")
	}
}

// TestList_CancelledContext verifies that List handles the cancelled context scenario correctly.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := List(ctx, client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// FormatListMarkdown tests.

// TestFormatListMarkdown_WithEntries verifies that FormatListMarkdown handles the with entries scenario correctly.
func TestFormatListMarkdown_WithEntries(t *testing.T) {
	out := ListOutput{
		Submodules: []SubmoduleEntry{
			{Name: "lib", Path: "lib", CommitSHA: "abc123def456", ResolvedProject: "group/lib"},
		},
		Count: 1,
	}
	r := FormatListMarkdown(out)
	if r == nil {
		t.Fatal("expected non-nil result")
	}
	tc, ok := r.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if !strings.Contains(tc.Text, "abc123de") {
		t.Error("expected truncated SHA in output")
	}
	if !strings.Contains(tc.Text, "group/lib") {
		t.Error("expected resolved project in output")
	}
}

// TestFormatListMarkdown_Empty verifies that FormatListMarkdown handles the empty scenario correctly.
func TestFormatListMarkdown_Empty(t *testing.T) {
	r := FormatListMarkdown(ListOutput{Submodules: []SubmoduleEntry{}, Count: 0})
	if r == nil {
		t.Fatal("expected non-nil result")
	}
	tc, ok := r.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if !strings.Contains(tc.Text, "No submodules found") {
		t.Error("expected empty message")
	}
}

// RegisterTools includes new tools.

// TestRegisterTools_IncludesNewTools verifies that RegisterTools handles the includes new tools scenario correctly.
func TestRegisterTools_IncludesNewTools(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
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

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools error: %v", err)
	}

	toolNames := make(map[string]bool)
	for _, tool := range tools.Tools {
		toolNames[tool.Name] = true
	}

	for _, name := range []string{
		"gitlab_list_repository_submodules",
		"gitlab_read_repository_submodule_file",
		"gitlab_update_repository_submodule",
	} {
		if !toolNames[name] {
			t.Errorf("expected tool %q to be registered", name)
		}
	}
}

// TestList_Base64EncodedWithRef verifies that [List] correctly decodes
// base64-encoded .gitmodules content and passes the Ref parameter to the
// GitLab API (covers the base64 decode + Ref branches in List).
func TestList_Base64EncodedWithRef(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(sampleGitmodules))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/") {
			// Verify ref parameter is passed
			if r.URL.Query().Get("ref") != "develop" {
				t.Errorf("expected ref=develop, got ref=%s", r.URL.Query().Get("ref"))
			}
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{
				"file_name": ".gitmodules",
				"encoding": "base64",
				"content": %q,
				"ref": "develop"
			}`, encoded))
			return
		}
		if strings.Contains(r.URL.Path, "/repository/tree") {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	})

	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{ProjectID: "42", Ref: "develop"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Count != 4 {
		t.Errorf("expected 4 submodules, got %d", out.Count)
	}
}

// TestList_EmptyGitmodulesAfterBase64 verifies that [List] returns an empty
// result when .gitmodules is base64-encoded but contains no submodule sections.
func TestList_EmptyGitmodulesAfterBase64(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("# empty file\n"))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/") {
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{
				"file_name": ".gitmodules",
				"encoding": "base64",
				"content": %q,
				"ref": "main"
			}`, encoded))
			return
		}
		http.NotFound(w, r)
	})

	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Count != 0 {
		t.Errorf("expected 0 submodules, got %d", out.Count)
	}
}

// TestList_Base64DecodeError verifies that [List] returns a meaningful error
// when the base64 content in .gitmodules is malformed.
func TestList_Base64DecodeError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/") {
			testutil.RespondJSON(w, http.StatusOK, `{
				"file_name": ".gitmodules",
				"encoding": "base64",
				"content": "!!!invalid-base64!!!",
				"ref": "main"
			}`)
			return
		}
		http.NotFound(w, r)
	})

	client := testutil.NewTestClient(t, handler)
	_, err := List(t.Context(), client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for invalid base64 content")
	}
	if !strings.Contains(err.Error(), "decode .gitmodules") {
		t.Errorf("error = %v, want 'decode .gitmodules' message", err)
	}
}

// TestList_TreeRequestError verifies that [enrichSubmoduleCommitSHAs] handles
// a failed tree request gracefully (entries still returned, just without SHAs).
func TestList_TreeRequestError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/") {
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{
				"file_name": ".gitmodules",
				"encoding": "text",
				"content": %q,
				"ref": "main"
			}`, sampleGitmodules))
			return
		}
		if strings.Contains(r.URL.Path, "/repository/tree") {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"internal error"}`)
			return
		}
		http.NotFound(w, r)
	})

	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Entries returned but without SHAs
	if out.Count != 4 {
		t.Errorf("expected 4 submodules, got %d", out.Count)
	}
}

// TestParseKeyValue_MalformedLine verifies that [parseKeyValue] returns empty
// strings for lines without '=' separator.
func TestParseKeyValue_MalformedLine(t *testing.T) {
	k, v, ok := parseKeyValue("no-equals-sign-here")
	if ok || k != "" || v != "" {
		t.Errorf("parseKeyValue(no =) = (%q, %q, %v), want empty + false", k, v, ok)
	}
}

// TestEnrichSubmoduleCommitSHAs_CancelledContext verifies that
// enrichSubmoduleCommitSHAs stops iterating directories when the
// context is cancelled, leaving CommitSHA fields empty.
func TestEnrichSubmoduleCommitSHAs_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	entries := []SubmoduleEntry{
		{Name: "a", Path: "dir1/a"},
		{Name: "b", Path: "dir2/b"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	enrichSubmoduleCommitSHAs(ctx, client, "42", "main", entries)

	for _, e := range entries {
		if e.CommitSHA != "" {
			t.Errorf("expected empty CommitSHA for entry %q after cancelled context, got %q", e.Name, e.CommitSHA)
		}
	}
}
