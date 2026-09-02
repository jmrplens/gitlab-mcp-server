// metadata_register_meta_test.go covers the package-level RegisterMeta
// detection logic used by the metadata audit.
//
// Tests build temporary Go sources with hand-written RegisterMeta functions
// and exercise the discover / reference / classify pipeline. Each test
// asserts the resulting classification, the referenced-from-hub flag, and
// the human-readable reason returned for unexpected definitions.
package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
)

// TestAuditRegisterMetaDefinitions_ClassifiesCentralReferences verifies AuditRegisterMetaDefinitions classifies central references.
func TestAuditRegisterMetaDefinitions_ClassifiesCentralReferences(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "internal/tools/register_meta.go", `package tools

func registerAllMetaGroups() {
	search.RegisterMeta(nil, nil)
	orbit.RegisterMeta(nil, nil)
	legacyreferenced.RegisterMeta(nil, nil)
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
	writeTestFile(t, root, "internal/tools/legacyreferenced/register.go", `package legacyreferenced

func RegisterMeta() {
	_ = struct{Name string}{Name: "gitlab_legacy_referenced"}
}
`)

	definitions, err := auditRegisterMetaDefinitions(root)
	if err != nil {
		t.Fatalf("auditRegisterMetaDefinitions() error = %v", err)
	}
	if len(definitions) != 3 {
		t.Fatalf("len(definitions) = %d, want 3", len(definitions))
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
	if !byPackage["legacyreferenced"].Referenced {
		t.Fatal("legacyreferenced RegisterMeta was not marked referenced")
	}
	if got := byPackage["legacy"].ToolNames; len(got) != 2 || got[0] != "gitlab_legacy" || got[1] != "gitlab_legacy_extra" {
		t.Fatalf("legacy tool names = %#v, want gitlab_legacy and gitlab_legacy_extra", got)
	}
}

// TestUnexpectedRegisterMetaDefinitions_FlagsPackageLevelDefinitions verifies UnexpectedRegisterMetaDefinitions flags package level definitions.
func TestUnexpectedRegisterMetaDefinitions_FlagsPackageLevelDefinitions(t *testing.T) {
	definitions := []registerMetaDefinition{
		{Package: "search", File: "internal/tools/search/register.go", Referenced: true},
		{Package: "legacy", File: "internal/tools/legacy/register.go", Referenced: false},
		{Package: "legacyreferenced", File: "internal/tools/legacyreferenced/register.go", Referenced: true},
		{Package: "runners", File: "internal/tools/runners/register.go", Referenced: false},
	}

	unexpected := unexpectedRegisterMetaDefinitions(definitions)
	if len(unexpected) != 4 {
		t.Fatalf("len(unexpected) = %d, want 4", len(unexpected))
	}

	byPackage := make(map[string]unexpectedRegisterMetaDefinition, len(unexpected))
	for _, definition := range unexpected {
		byPackage[definition.Package] = definition
	}
	if !strings.Contains(byPackage["legacy"].Reason, "not an approved catalog-first runtime pattern") {
		t.Fatalf("legacy reason = %q", byPackage["legacy"].Reason)
	}
	if !strings.Contains(byPackage["legacyreferenced"].Reason, "not an approved catalog-first runtime pattern") {
		t.Fatalf("legacyreferenced reason = %q", byPackage["legacyreferenced"].Reason)
	}
	if !strings.Contains(byPackage["runners"].Reason, "not an approved catalog-first runtime pattern") {
		t.Fatalf("runners reason = %q", byPackage["runners"].Reason)
	}
}

// TestAuditRegisterMetaDefinitionViolations_ConvertsUnexpectedDefinitions verifies AuditRegisterMetaDefinitionViolations converts unexpected definitions.
func TestAuditRegisterMetaDefinitionViolations_ConvertsUnexpectedDefinitions(t *testing.T) {
	violations := auditRegisterMetaDefinitionViolations([]registerMetaDefinition{
		{Package: "legacy", File: "internal/tools/legacy/register.go", Referenced: false},
	})

	if len(violations) != 1 {
		t.Fatalf("len(violations) = %d, want 1", len(violations))
	}
	if violations[0].category != "register-meta" {
		t.Fatalf("category = %q, want register-meta", violations[0].category)
	}
	if !strings.Contains(violations[0].detail, "not an approved catalog-first runtime pattern") {
		t.Fatalf("detail = %q", violations[0].detail)
	}
}

// TestCurrentRegisterMetaDefinitions_NoneRemain verifies CurrentRegisterMetaDefinitions when none remain.
func TestCurrentRegisterMetaDefinitions_NoneRemain(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repositoryRoot() error = %v", err)
	}
	definitions, err := auditRegisterMetaDefinitions(root)
	if err != nil {
		t.Fatalf("auditRegisterMetaDefinitions() error = %v", err)
	}
	if len(definitions) != 0 {
		t.Fatalf("RegisterMeta definitions = %#v, want none", definitions)
	}
}

// TestPrintRegisterMetaDefinitions_WritesInventorySummary verifies PrintRegisterMetaDefinitions writes inventory summary.
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
			{
				Package:    "runners",
				File:       "internal/tools/runners/register.go",
				ToolNames:  nil,
				Referenced: false,
			},
		})
	})

	expectedFragments := []string{
		"## RegisterMeta Definition Inventory",
		"| Package-level RegisterMeta definitions | 3 |",
		"| Referenced from central meta hub | 1 |",
		"| Approved delegated definitions | 0 |",
		"| Unexpected definitions | 3 |",
		"| unexpected | `search` | `internal/tools/search/register.go` | `-` |",
		"| unexpected | `legacy` | `internal/tools/legacy/register.go` | `gitlab_legacy` |",
		"| unexpected | `runners` | `internal/tools/runners/register.go` | `-` |",
	}
	for _, expected := range expectedFragments {
		t.Run(expected, func(t *testing.T) {
			if !strings.Contains(output, expected) {
				t.Fatalf("output missing %q:\n%s", expected, output)
			}
		})
	}
}

// TestPrintRegisterMetaDefinitions_EmptyDefinitionsWritesNothing verifies PrintRegisterMetaDefinitions when empty definitions writes nothing.
func TestPrintRegisterMetaDefinitions_EmptyDefinitionsWritesNothing(t *testing.T) {
	output := captureStdout(t, func() {
		printRegisterMetaDefinitions(nil)
	})
	if output != "" {
		t.Fatalf("output = %q, want empty string", output)
	}
}

// TestRepositoryRoot_FindsNearestGoMod verifies RepositoryRoot when finds nearest go mod.
func TestRepositoryRoot_FindsNearestGoMod(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module example.com/test\n")
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", nested, err)
	}

	foundRoot, err := cmdutil.RepositoryRoot(nested)
	if err != nil {
		t.Fatalf("repositoryRoot() error = %v", err)
	}
	if foundRoot != root {
		t.Fatalf("repositoryRoot() = %q, want %q", foundRoot, root)
	}
}

// TestRepositoryRoot_MissingGoModReturnsError verifies RepositoryRoot when missing go mod returns error.
func TestRepositoryRoot_MissingGoModReturnsError(t *testing.T) {
	_, err := cmdutil.RepositoryRoot(t.TempDir())
	if err == nil {
		t.Fatal("repositoryRoot() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "go.mod not found") {
		t.Fatalf("repositoryRoot() error = %q, want go.mod not found", err)
	}
}

// TestRegisterMetaToolNames_CollectsOnlyQuotedGitlabNames verifies the tool
// names are read from Name:"gitlab_*" string literals only: a non-identifier
// key, a non-literal value, a non-string literal, a name outside the gitlab_
// namespace and a repeated name are all ignored, and the survivors come back
// sorted and deduplicated.
func TestRegisterMetaToolNames_CollectsOnlyQuotedGitlabNames(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "internal/tools/register_meta.go", "package tools\n")
	writeTestFile(t, root, "internal/tools/mixed/register.go", `package mixed

func RegisterMeta() {
	_ = struct{ Name string }{Name: "gitlab_zebra"}
	_ = struct{ Name string }{Name: "gitlab_alpha"}
	_ = struct{ Name string }{Name: "gitlab_alpha"}
	_ = map[string]string{"Name": "gitlab_not_an_identifier_key"}
	_ = struct{ Name string }{Name: other}
	_ = struct{ Name int }{Name: 42}
	_ = struct{ Name string }{Name: "helper_not_gitlab"}
	_ = struct{ Other string }{Other: "gitlab_wrong_key"}
}
`)

	definitions, err := auditRegisterMetaDefinitions(root)
	if err != nil {
		t.Fatalf("auditRegisterMetaDefinitions() error = %v", err)
	}
	if len(definitions) != 1 {
		t.Fatalf("len(definitions) = %d, want 1: %#v", len(definitions), definitions)
	}
	got := definitions[0].ToolNames
	if len(got) != 2 || got[0] != "gitlab_alpha" || got[1] != "gitlab_zebra" {
		t.Fatalf("ToolNames = %#v, want [gitlab_alpha gitlab_zebra]", got)
	}
}

// TestAuditRegisterMetaDefinitions_SkipsTestdataAndTestFiles verifies the
// walk ignores testdata directories and _test.go files, so a RegisterMeta
// written in either place is not reported as a package-level definition.
func TestAuditRegisterMetaDefinitions_SkipsTestdataAndTestFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "internal/tools/register_meta.go", "package tools\n")
	writeTestFile(t, root, "internal/tools/testdata/register.go", `package testdata

func RegisterMeta() {}
`)
	writeTestFile(t, root, "internal/tools/sample/register_test.go", `package sample

func RegisterMeta() {}
`)
	writeTestFile(t, root, "internal/tools/sample/notes.md", "not Go\n")

	definitions, err := auditRegisterMetaDefinitions(root)
	if err != nil {
		t.Fatalf("auditRegisterMetaDefinitions() error = %v", err)
	}
	if len(definitions) != 0 {
		t.Fatalf("definitions = %#v, want none", definitions)
	}
}

// TestAuditRegisterMetaDefinitions_ErrorPaths verifies the audit fails when
// a source file under internal/tools cannot be parsed and when the central
// register_meta.go hub is missing, rather than reporting a clean tree.
func TestAuditRegisterMetaDefinitions_ErrorPaths(t *testing.T) {
	testCases := []struct {
		name    string
		files   map[string]string
		wantErr string
	}{
		{
			name: "unparseable package file",
			files: map[string]string{
				"internal/tools/register_meta.go": "package tools\n",
				"internal/tools/broken/broken.go": "package broken\n\nfunc (\n",
			},
			wantErr: "parse ",
		},
		{
			name:    "missing central hub",
			files:   map[string]string{"internal/tools/sample/register.go": "package sample\n"},
			wantErr: "parse ",
		},
		{
			name:    "missing tools tree",
			files:   map[string]string{"go.mod": "module example.com/fixture\n"},
			wantErr: "internal",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for name, content := range tc.files {
				writeTestFile(t, root, name, content)
			}

			_, err := auditRegisterMetaDefinitions(root)
			if err == nil {
				t.Fatal("auditRegisterMetaDefinitions() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

// TestReferencedRegisterMetaPackages_IgnoresNonIdentifierReceivers verifies
// the central hub scan records plain package identifiers only, so a
// RegisterMeta reached through a nested selector or a call result is not
// mistaken for a package reference.
func TestReferencedRegisterMetaPackages_IgnoresNonIdentifierReceivers(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "register_meta.go")
	writeTestFile(t, root, "register_meta.go", `package tools

func registerAllMetaGroups() {
	search.RegisterMeta(nil, nil)
	pkg.nested.RegisterMeta(nil, nil)
	builder().RegisterMeta(nil, nil)
	other.Register(nil, nil)
}
`)

	references, err := referencedRegisterMetaPackages(path)
	if err != nil {
		t.Fatalf("referencedRegisterMetaPackages() error = %v", err)
	}
	if len(references) != 1 {
		t.Fatalf("references = %#v, want only search", references)
	}
	if _, ok := references["search"]; !ok {
		t.Fatalf("references = %#v, want search", references)
	}
}

// TestDelegatedRegisterMetaPackages_AllowListedDefinitions verifies the
// allow-list path: a delegated package that the central hub references is
// approved and reported as delegated, while a delegated package the hub
// never calls is flagged with the unreferenced reason.
func TestDelegatedRegisterMetaPackages_AllowListedDefinitions(t *testing.T) {
	delegatedRegisterMetaPackages["approved"] = struct{}{}
	delegatedRegisterMetaPackages["orphaned"] = struct{}{}
	t.Cleanup(func() {
		delete(delegatedRegisterMetaPackages, "approved")
		delete(delegatedRegisterMetaPackages, "orphaned")
	})

	definitions := []registerMetaDefinition{
		{Package: "approved", File: "internal/tools/approved/register.go", Referenced: true},
		{Package: "orphaned", File: "internal/tools/orphaned/register.go", Referenced: false},
	}

	if !isDelegatedRegisterMetaDefinition(definitions[0]) {
		t.Error("isDelegatedRegisterMetaDefinition(approved) = false, want true")
	}
	if isDelegatedRegisterMetaDefinition(definitions[1]) {
		t.Error("isDelegatedRegisterMetaDefinition(orphaned) = true, want false")
	}

	unexpected := unexpectedRegisterMetaDefinitions(definitions)
	if len(unexpected) != 1 {
		t.Fatalf("unexpected = %#v, want only the orphaned definition", unexpected)
	}
	if unexpected[0].Package != "orphaned" {
		t.Fatalf("unexpected package = %q, want orphaned", unexpected[0].Package)
	}
	if !strings.Contains(unexpected[0].Reason, "not referenced from internal/tools/register_meta.go") {
		t.Fatalf("reason = %q, want the unreferenced reason", unexpected[0].Reason)
	}

	output := captureStdout(t, func() { printRegisterMetaDefinitions(definitions) })
	for _, want := range []string{
		"| Approved delegated definitions | 1 |",
		"| Unexpected definitions | 1 |",
		"| delegated | `approved` | `internal/tools/approved/register.go` | `-` |",
		"| unexpected | `orphaned` | `internal/tools/orphaned/register.go` | `-` |",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(output, want) {
				t.Errorf("output missing %q:\n%s", want, output)
			}
		})
	}
}

// writeTestFile writes test file fixture data for tests.
func writeTestFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

// captureStdout supports capture stdout assertions in metadata tests.
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
