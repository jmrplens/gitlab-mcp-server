package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditRegisterMetaDefinitions_ClassifiesCentralReferences(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "internal/tools/register_meta.go", `package tools

func registerAllMetaGroups() {
	search.RegisterMeta(nil, nil)
	orbit.RegisterMeta(nil, nil)
}
`)
	writeTestFile(t, root, "internal/tools/search/register.go", `package search

func RegisterMeta() {
	_ = struct{Name string}{Name: "gitlab_search"}
}
`)
	writeTestFile(t, root, "internal/tools/legacy/register.go", `package legacy

func RegisterMeta() {
	_ = struct{Name string}{Name: "gitlab_legacy"}
	_ = struct{Name string}{Name: "gitlab_legacy_extra"}
}
`)

	definitions, err := auditRegisterMetaDefinitions(root)
	if err != nil {
		t.Fatalf("auditRegisterMetaDefinitions() error = %v", err)
	}
	if len(definitions) != 2 {
		t.Fatalf("len(definitions) = %d, want 2", len(definitions))
	}

	byPackage := make(map[string]registerMetaDefinition, len(definitions))
	for _, definition := range definitions {
		byPackage[definition.Package] = definition
	}

	if !byPackage["search"].Referenced {
		t.Fatal("search RegisterMeta was not marked referenced")
	}
	if byPackage["legacy"].Referenced {
		t.Fatal("legacy RegisterMeta was marked referenced")
	}
	if got := byPackage["legacy"].ToolNames; len(got) != 2 || got[0] != "gitlab_legacy" || got[1] != "gitlab_legacy_extra" {
		t.Fatalf("legacy tool names = %#v, want gitlab_legacy and gitlab_legacy_extra", got)
	}
}

func TestPrintRegisterMetaDefinitions_WritesInventorySummary(t *testing.T) {
	output := captureStdout(t, func() {
		printRegisterMetaDefinitions([]registerMetaDefinition{
			{
				Package:    "search",
				File:       "internal/tools/search/register.go",
				ToolNames:  nil,
				Referenced: true,
			},
			{
				Package:    "legacy",
				File:       "internal/tools/legacy/register.go",
				ToolNames:  []string{"gitlab_legacy"},
				Referenced: false,
			},
		})
	})

	expectedFragments := []string{
		"## RegisterMeta Definition Inventory",
		"| Package-level RegisterMeta definitions | 2 |",
		"| Referenced from central meta hub | 1 |",
		"| Unreferenced cleanup candidates | 1 |",
		"| referenced | `search` | `internal/tools/search/register.go` | `-` |",
		"| unreferenced | `legacy` | `internal/tools/legacy/register.go` | `gitlab_legacy` |",
	}
	for _, expected := range expectedFragments {
		if !strings.Contains(output, expected) {
			t.Fatalf("output missing %q:\n%s", expected, output)
		}
	}
}

func TestPrintRegisterMetaDefinitions_EmptyDefinitionsWritesNothing(t *testing.T) {
	output := captureStdout(t, func() {
		printRegisterMetaDefinitions(nil)
	})
	if output != "" {
		t.Fatalf("output = %q, want empty string", output)
	}
}

func TestRepositoryRoot_FindsNearestGoMod(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module example.com/test\n")
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", nested, err)
	}

	foundRoot, err := repositoryRoot(nested)
	if err != nil {
		t.Fatalf("repositoryRoot() error = %v", err)
	}
	if foundRoot != root {
		t.Fatalf("repositoryRoot() = %q, want %q", foundRoot, root)
	}
}

func TestRepositoryRoot_MissingGoModReturnsError(t *testing.T) {
	_, err := repositoryRoot(t.TempDir())
	if err == nil {
		t.Fatal("repositoryRoot() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "go.mod not found") {
		t.Fatalf("repositoryRoot() error = %q, want go.mod not found", err)
	}
}

func writeTestFile(t *testing.T, root string, name string, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func captureStdout(t *testing.T, action func()) string {
	t.Helper()
	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe() error = %v", err)
	}
	os.Stdout = writer

	action()

	os.Stdout = originalStdout
	if closeErr := writer.Close(); closeErr != nil {
		t.Fatalf("Close() writer error = %v", closeErr)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if closeErr := reader.Close(); closeErr != nil {
		t.Fatalf("Close() reader error = %v", closeErr)
	}
	return string(output)
}
