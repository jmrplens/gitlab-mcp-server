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

// TestListDynamicTools_ExposesFindAndExecute verifies dynamic llms generation
// exposes only the find and execute tools in deterministic order.
//
// The test builds a mock GitLab-backed client, lists dynamic tools, and checks
// the execute input schema for action, params, and confirm fields. This protects
// the low-token dynamic contract consumed by generated LLM discovery files.
func TestListDynamicTools_ExposesFindAndExecute(t *testing.T) {
	tools, err := listDynamicTools(newGenLLMSClient(t))
	if err != nil {
		t.Fatalf("listDynamicTools() error: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("len(listDynamicTools()) = %d, want 2", len(tools))
	}
	names := []string{tools[0].Name, tools[1].Name}
	if names[0] != dynamicFindToolName || names[1] != dynamicExecuteActionToolName {
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

// TestValidateDynamicToolContract_RejectsDrift verifies the generated dynamic
// tool contract fails when the expected find/execute pair changes.
//
// The happy-path assertion accepts the canonical two-tool list, while the drift
// assertion removes find and expects validation to fail. This keeps accidental
// dynamic surface changes visible during llms generation.
func TestValidateDynamicToolContract_RejectsDrift(t *testing.T) {
	if err := validateDynamicToolContract([]*mcp.Tool{{Name: dynamicFindToolName}, {Name: dynamicExecuteActionToolName}}); err != nil {
		t.Fatalf("validateDynamicToolContract() error = %v", err)
	}
	if err := validateDynamicToolContract([]*mcp.Tool{{Name: dynamicExecuteActionToolName}}); err == nil {
		t.Fatal("validateDynamicToolContract() error = nil, want error")
	}
}

// TestReadVersion_UsesProjectRoot verifies readVersion reads VERSION from the
// supplied project root and trims trailing whitespace.
//
// The test writes a temporary VERSION file and expects the exact semantic
// version string. This prevents generation from depending on the process working
// directory when a root is supplied.
func TestReadVersion_UsesProjectRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte("2.1.0\n"), 0o600); err != nil {
		t.Fatalf("write VERSION: %v", err)
	}

	if got := readVersion(dir); got != "2.1.0" {
		t.Fatalf("readVersion() = %q, want 2.1.0", got)
	}
}

// TestListResources_IncludesToolManifestTemplate verifies llms generation sees
// the unified tool manifest template alongside regular resources.
func TestListResources_IncludesToolManifestTemplate(t *testing.T) {
	resources, templates, err := listResources(newGenLLMSClient(t))
	if err != nil {
		t.Fatalf("listResources() error: %v", err)
	}
	if len(resources) == 0 {
		t.Fatal("listResources() returned no static resources")
	}
	wantTemplates := map[string]bool{
		"gitlab://tools/{id}": false,
	}
	for _, template := range templates {
		if template.URITemplate == "gitlab://schema/meta/{tool}/{action}" || template.URITemplate == "gitlab://schema/dynamic/{action}" {
			t.Fatalf("listResources() exposed legacy schema template %s: %v", template.URITemplate, templates)
		}
		if _, ok := wantTemplates[template.URITemplate]; ok {
			wantTemplates[template.URITemplate] = true
		}
	}
	for uri, found := range wantTemplates {
		if !found {
			t.Fatalf("listResources() templates missing %s: %v", uri, templates)
		}
	}
}

// TestValidateLLMSTxt_AcceptsSpecFileListSections verifies llms.txt validation
// accepts H2 sections made of Markdown file-list entries.
//
// The content includes prose before the generated sections and both linked docs
// with and without descriptions. The expected result is no error, matching the
// public llms.txt shape documented by the generator.
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

// TestValidateLLMSTxt_RejectsNonLinkH2Content verifies llms.txt H2 sections must
// contain file-list link entries rather than arbitrary prose.
//
// The test places plain text under a Docs section and expects validation to
// fail, keeping generated discovery files machine-readable for model consumers.
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

// TestValidateLLMSTxt_RejectsEmptyFileListLinkLabel verifies llms.txt validation
// rejects Markdown links without visible labels.
//
// Empty labels produce poor LLM context and broken human navigation, so the test
// expects a validation error for a file-list entry using [](...).
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

// TestValidateLLMSFullTxt_RequiresGeneratedSections verifies llms-full.txt
// validation requires all generated catalog sections.
//
// The first fixture includes Dynamic Toolset, Meta-Tools, Individual Tools,
// Resources, and Prompts and should pass. The second fixture omits most sections
// and should fail so partial generated files are caught before writing.
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

// TestWriteGeneratedFile_RejectsUnexpectedFileName verifies generated llms files
// can only be written to the supported top-level artifact names.
//
// The test attempts README.md, a parent-directory escape, and a docs path in
// check mode. Each should fail to prevent accidental writes outside the intended
// llms.txt and llms-full.txt outputs.
func TestWriteGeneratedFile_RejectsUnexpectedFileName(t *testing.T) {
	for _, name := range []string{"README.md", "../llms.txt", "docs/llms.txt"} {
		t.Run(name, func(t *testing.T) {
			if err := writeGeneratedFile(name, "content", true); err == nil {
				t.Fatal("writeGeneratedFile() error = nil, want error")
			}
		})
	}
}

// TestWriteGeneratedFile_CheckModeAcceptsCRLFLineEndings verifies check mode
// treats CRLF and LF generated files as equivalent.
//
// The test writes llms.txt with Windows line endings, then checks the same
// content with LF endings. A nil error prevents cross-platform line ending
// differences from causing false generation drift.
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

// TestSchemaTypeLabel_ArrayAndNullableTypes verifies schemaTypeLabel summarizes
// nullable, array, nested-array, object, and untyped schemas.
//
// The table covers common JSON Schema shapes emitted for tool inputs. Expected
// labels are human-readable phrases used in generated llms-full.txt parameter
// references.
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
