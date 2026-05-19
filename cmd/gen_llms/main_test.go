// main_test.go contains focused tests for llms.txt generation helpers. Tests
// use a local GitLab version mock so resource and template discovery can run
// through an in-memory MCP server.
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// newGenLLMSClient creates a [gitlabclient.Client] backed by a mock
// /api/v4/version endpoint for gen_llms tests.
func newGenLLMSClient(t *testing.T) *gitlabclient.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"version":"17.0.0"}`)
	}))
	t.Cleanup(srv.Close)

	client, err := gitlabclient.NewClient(&config.Config{GitLabURL: srv.URL, GitLabToken: "gen-llms-token"})
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	return client
}

func TestListDynamicTools_ExposesFindAndExecute(t *testing.T) {
	tools, err := listDynamicTools(newGenLLMSClient(t))
	if err != nil {
		t.Fatalf("listDynamicTools() error: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("len(listDynamicTools()) = %d, want 2", len(tools))
	}
	names := []string{tools[0].Name, tools[1].Name}
	if names[0] != dynamicFindToolName || names[1] != dynamicExecuteToolName {
		t.Fatalf("dynamic tools = %v, want find before execute", names)
	}

	executeSchema, ok := tools[1].InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("execute InputSchema has type %T, want map[string]any", tools[1].InputSchema)
	}
	executeProperties, ok := executeSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("execute InputSchema properties has type %T, want map[string]any", executeSchema["properties"])
	}
	for _, property := range []string{"action", "params", "confirm"} {
		if _, exists := executeProperties[property]; !exists {
			t.Fatalf("execute InputSchema missing %q property: %v", property, executeProperties)
		}
	}
	required, ok := executeSchema["required"].([]any)
	if !ok {
		t.Fatalf("execute InputSchema required has type %T, want []any", executeSchema["required"])
	}
	if !slices.Contains(required, any("action")) || !slices.Contains(required, any("params")) {
		t.Fatalf("execute InputSchema required = %v, want action and params", required)
	}
}

func TestValidateDynamicToolContract_RejectsDrift(t *testing.T) {
	if err := validateDynamicToolContract([]*mcp.Tool{{Name: dynamicFindToolName}, {Name: dynamicExecuteToolName}}); err != nil {
		t.Fatalf("validateDynamicToolContract() error = %v", err)
	}
	if err := validateDynamicToolContract([]*mcp.Tool{{Name: dynamicExecuteToolName}}); err == nil {
		t.Fatal("validateDynamicToolContract() error = nil, want error")
	}
}

func TestReadVersion_UsesProjectRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte("2.1.0\n"), 0o600); err != nil {
		t.Fatalf("write VERSION: %v", err)
	}

	if got := readVersion(dir); got != "2.1.0" {
		t.Fatalf("readVersion() = %q, want 2.1.0", got)
	}
}

// TestListResources_IncludesMetaSchemaTemplate verifies llms generation sees
// the per-action meta-schema resource template alongside regular resources.
func TestListResources_IncludesMetaSchemaTemplate(t *testing.T) {
	resources, templates, err := listResources(newGenLLMSClient(t))
	if err != nil {
		t.Fatalf("listResources() error: %v", err)
	}
	if len(resources) == 0 {
		t.Fatal("listResources() returned no static resources")
	}
	for _, template := range templates {
		if template.URITemplate == "gitlab://schema/meta/{tool}/{action}" {
			return
		}
	}
	t.Fatalf("listResources() templates missing meta-schema template: %v", templates)
}

func TestValidateLLMSTxt_AcceptsSpecFileListSections(t *testing.T) {
	content := strings.Join([]string{
		"# Example",
		"",
		"> Short project summary.",
		"",
		"Details before H2 sections can use normal Markdown lists.",
		"",
		"- key: value",
		"",
		"## Docs",
		"",
		"- [Guide](docs/guide.md): Short guide",
		"- [Reference](docs/reference.md)",
		"",
		"## Optional",
		"",
		"- [Full reference](llms-full.txt): Expanded context",
		"",
	}, "\n")

	if err := validateLLMSTxt(content); err != nil {
		t.Fatalf("validateLLMSTxt() error: %v", err)
	}
}

func TestValidateLLMSTxt_RejectsNonLinkH2Content(t *testing.T) {
	content := strings.Join([]string{
		"# Example",
		"",
		"> Short project summary.",
		"",
		"## Docs",
		"",
		"Plain text is not a file-list entry.",
		"",
	}, "\n")

	if err := validateLLMSTxt(content); err == nil {
		t.Fatal("validateLLMSTxt() error = nil, want error")
	}
}

func TestValidateLLMSTxt_RejectsEmptyFileListLinkLabel(t *testing.T) {
	content := strings.Join([]string{
		"# Example",
		"",
		"> Short project summary.",
		"",
		"## Docs",
		"",
		"- [](docs/guide.md)",
		"",
	}, "\n")

	if err := validateLLMSTxt(content); err == nil {
		t.Fatal("validateLLMSTxt() error = nil, want error")
	}
}

func TestValidateLLMSFullTxt_RequiresGeneratedSections(t *testing.T) {
	content := strings.Join([]string{
		"# Example Full Reference",
		"",
		"## Dynamic Toolset",
		"",
		"## Meta-Tools",
		"",
		"## Individual Tools",
		"",
		"## Resources",
		"",
		"## Prompts",
		"",
	}, "\n")

	if err := validateLLMSFullTxt(content); err != nil {
		t.Fatalf("validateLLMSFullTxt() error: %v", err)
	}
	if err := validateLLMSFullTxt("# Example\n\n## Dynamic Toolset\n"); err == nil {
		t.Fatal("validateLLMSFullTxt() error = nil, want error")
	}
}

func TestWriteGeneratedFile_RejectsUnexpectedFileName(t *testing.T) {
	for _, name := range []string{"README.md", "../llms.txt", "docs/llms.txt"} {
		t.Run(name, func(t *testing.T) {
			if err := writeGeneratedFile(name, "content", true); err == nil {
				t.Fatal("writeGeneratedFile() error = nil, want error")
			}
		})
	}
}

func TestWriteGeneratedFile_CheckModeAcceptsCRLFLineEndings(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	content := "# Example\n\n"
	if err := os.WriteFile(filepath.Join(dir, llmsFileName), []byte("# Example\r\n\r\n"), 0o600); err != nil {
		t.Fatalf("write llms.txt: %v", err)
	}
	t.Chdir(dir)

	if err := writeGeneratedFile(llmsFileName, content, true); err != nil {
		t.Fatalf("writeGeneratedFile() error = %v", err)
	}
}

func TestSchemaTypeLabel_ArrayAndNullableTypes(t *testing.T) {
	tests := []struct {
		name   string
		schema map[string]any
		want   string
	}{
		{
			name:   "nullable string",
			schema: map[string]any{"type": []any{"null", "string"}},
			want:   "string",
		},
		{
			name: "nullable integer array",
			schema: map[string]any{
				"type":  []any{"null", "array"},
				"items": map[string]any{"type": "integer"},
			},
			want: "array of integers",
		},
		{
			name: "object array",
			schema: map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "object"},
			},
			want: "array of objects",
		},
		{
			name:   "untyped any value",
			schema: map[string]any{},
			want:   "any",
		},
		{
			name: "nested string array",
			schema: map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "string"},
				},
			},
			want: "array of arrays of strings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := schemaTypeLabel(tt.schema); got != tt.want {
				t.Fatalf("schemaTypeLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}
