// main_test.go contains focused tests for llms.txt generation helpers. Tests
// use a local GitLab version mock so resource and template discovery can run
// through an in-memory MCP server.
//
// Coverage focuses on the dynamic two-tool contract, llms.txt and
// llms-full.txt structural validation, generated file naming rules, and
// the schema-type label formatter used in the long-form reference.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

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
		"- [Guide](https://example.test/docs/guide.md): Short guide",
		"- [Reference](https://example.test/docs/reference.md)",
		"",
		"## Optional",
		"",
		"- [Full reference](https://example.test/llms-full.txt): Expanded context",
		"",
	}, "\n")

	if err := validateLLMSTxt(content); err != nil {
		t.Fatalf("validateLLMSTxt() error: %v", err)
	}
}

// TestValidateLLMSTxt_RejectsRelativeFileListTarget guards the GEO regression
// where every llms.txt link was a repository-relative path. llms.txt is served
// from the docs domain, so "docs/getting-started.md" resolved against that host
// and 404'd — 17 of 18 links were dead. Only absolute URLs work for every
// consumer, so generation must fail rather than publish dead links again.
func TestValidateLLMSTxt_RejectsRelativeFileListTarget(t *testing.T) {
	content := strings.Join([]string{
		"# Example",
		"",
		"> Short project summary.",
		"",
		"## Docs",
		"",
		"- [Guide](docs/guide.md): Short guide",
		"",
	}, "\n")

	err := validateLLMSTxt(content)
	if err == nil {
		t.Fatal("validateLLMSTxt() error = nil, want error for a relative link target")
	}
	if !strings.Contains(err.Error(), "absolute URL") {
		t.Fatalf("validateLLMSTxt() error = %v, want it to mention the absolute-URL requirement", err)
	}
}

// TestAbsoluteLLMSTarget_ResolvesRepoRelativeAndPreservesAbsolute verifies both
// branches of the link resolver: repository-relative documentation paths gain
// the blob prefix, while already-absolute targets are emitted untouched.
func TestAbsoluteLLMSTarget_ResolvesRepoRelativeAndPreservesAbsolute(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   string
	}{
		{
			name:   "repo relative doc path gains the blob prefix",
			target: "docs/getting-started.md",
			want:   repoBlobBaseURL + "docs/getting-started.md",
		},
		{
			name:   "repo root file gains the blob prefix",
			target: "PRIVACY.md",
			want:   repoBlobBaseURL + "PRIVACY.md",
		},
		{
			name:   "absolute https target is preserved",
			target: siteBaseURL + "llms-full.txt",
			want:   siteBaseURL + "llms-full.txt",
		},
		{
			name:   "absolute http target is preserved",
			target: "http://example.test/a.md",
			want:   "http://example.test/a.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := absoluteLLMSTarget(tt.target); got != tt.want {
				t.Errorf("absoluteLLMSTarget(%q) = %q, want %q", tt.target, got, tt.want)
			}
		})
	}
}

// TestValidateLLMSTxt_AcceptsPreambleCodeBlock verifies that a fenced code block
// in the preamble (before the first H2) is accepted. The generated llms.txt uses
// this for the headless AI-assistant install snippet (the mcpServers JSON), so a
// stricter validator that rejected code fences would silently break that section.
func TestValidateLLMSTxt_AcceptsPreambleCodeBlock(t *testing.T) {
	content := strings.Join([]string{
		"# Example",
		"",
		"> Short project summary.",
		"",
		"Installing for an AI assistant (headless, no wizard):",
		"",
		"```json",
		"{",
		"  \"mcpServers\": {",
		"    \"gitlab\": { \"command\": \"docker\" }",
		"  }",
		"}",
		"```",
		"",
		"## Docs",
		"",
		"- [Guide](https://example.test/docs/guide.md): Short guide",
		"",
	}, "\n")

	if err := validateLLMSTxt(content); err != nil {
		t.Fatalf("validateLLMSTxt() rejected a preamble code block: %v", err)
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

// TestIsGeneratedLLMSFile_CoversEveryCompanion pins the write allowlist. The
// companion files exist because llms-full.txt is ~750K tokens — larger than any
// production context window — so a new companion silently failing to write
// would quietly undo that fix.
func TestIsGeneratedLLMSFile_CoversEveryCompanion(t *testing.T) {
	for _, name := range []string{
		llmsFileName, llmsFullFileName, llmsMediumFileName,
		llmsFullMetaFileName, llmsFullIndividualFileName, llmsFullCapabilityFileName,
	} {
		t.Run(name, func(t *testing.T) {
			if !isGeneratedLLMSFile(name) {
				t.Errorf("isGeneratedLLMSFile(%q) = false, want true", name)
			}
		})
	}
	for _, name := range []string{"README.md", "llms.txt.bak", "../llms.txt", ""} {
		t.Run(name, func(t *testing.T) {
			if isGeneratedLLMSFile(name) {
				t.Errorf("isGeneratedLLMSFile(%q) = true, want false", name)
			}
		})
	}
}

// TestWriteLLMSMediumMetaTools_OmitsSchemas verifies the medium reference keeps
// the action inventory but drops the per-action JSON schemas. Those schemas are
// what make the full file unloadable, so their absence here is the whole point.
func TestWriteLLMSMediumMetaTools_OmitsSchemas(t *testing.T) {
	catalog := llmsCatalog{
		MetaBase: []*mcp.Tool{{
			Name:        "gitlab_issue",
			Title:       "Issue",
			Description: "Manage issues. Use {\"action\":\"list\"}.",
		}},
		MetaRoutes: map[string]toolutil.ActionMap{
			"gitlab_issue": {"list": {}, "create": {}, "get": {}},
		},
	}

	var b strings.Builder
	writeLLMSMediumMetaTools(&b, catalog)
	got := b.String()

	for _, want := range []string{"### gitlab_issue", "**Issue**", "Actions (3):", "create, get, list"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(got, want) {
				t.Errorf("medium meta-tools output missing %q\ngot:\n%s", want, got)
			}
		})
	}
	// Actions must be sorted so regeneration is deterministic.
	if strings.Index(got, "create") > strings.Index(got, "get") {
		t.Error("actions are not sorted alphabetically")
	}
	for _, unwanted := range []string{"inputSchema", "\"properties\"", "Input Schema"} {
		t.Run(unwanted, func(t *testing.T) {
			if strings.Contains(got, unwanted) {
				t.Errorf("medium meta-tools output must not embed schemas, found %q", unwanted)
			}
		})
	}
}
