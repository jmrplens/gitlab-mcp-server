// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sourcegate"
)

// writeTree writes the named files below a new temporary directory and
// returns it.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, source := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("prepare %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatalf("prepare %s: %v", name, err)
		}
	}
	return dir
}

// catalogModule is a module whose catalog package holds the given files
// beside the Key type and one key, for a catalog the reader has to refuse.
func catalogModule(t *testing.T, files map[string]string) auditConfig {
	t.Helper()
	tree := map[string]string{
		"go.mod":                  "module example.com/broken\n\ngo 1.27\n",
		"internal/i18n/keys.go":   "package i18n\n\n// Key names one text.\ntype Key string\n\n// Only is the one key.\nconst Only Key = \"only\"\n",
		"internal/flows/flows.go": "package flows\n\nimport \"example.com/broken/internal/i18n\"\n\nvar _ = i18n.Only\n",
		"cmd/tool/main.go":        "package main\n\nfunc main() {}\n",
	}
	maps.Copy(tree, files)
	return auditConfig{
		dir:      writeTree(t, tree),
		patterns: defaultPatterns,
		catalog:  "example.com/broken/internal/i18n",
		declared: map[string]sourcegate.Declaration{},
	}
}

// TestReadCatalog_PlantedCatalog_ReadsEveryEntryAndKey holds what the reader
// takes out of the catalog's source: every entry with its template folded the
// way the compiler folds it, a concatenation included, and every key constant
// the package declares, the ones the catalog lacks among them.
func TestReadCatalog_PlantedCatalog_ReadsEveryEntryAndKey(t *testing.T) {
	report := auditPlanted(t, plantedConfig(t, "clean", map[string]sourcegate.Declaration{}))
	if report.Summary.Keys != 5 {
		t.Fatalf("keys = %d, want the five entries of the clean catalog", report.Summary.Keys)
	}
	cfg := plantedConfig(t, "clean", nil)
	root, loaded, err := goprogram.LoadRooted(cfg.dir, append(slices.Clone(cfg.patterns), cfg.catalog), goprogram.Options{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cat, err := readCatalog(root, loaded, cfg.catalog)
	if err != nil {
		t.Fatalf("readCatalog: %v", err)
	}
	want := map[string]string{
		"common.list.and": " and ",
		"flow.greeting":   "Hello {name}",
		"flow.summary":    "Create {thing}?\n\n**Title**: {title}",
		"flow.row":        "\n**Labels**: {labels}",
		"flow.count":      "{count} of {items}",
	}
	if !maps.Equal(cat.templates, want) {
		t.Errorf("templates = %q, want %q", cat.templates, want)
	}
	if got := slices.Sorted(maps.Keys(cat.constants)); !slices.Equal(got, slices.Sorted(maps.Keys(want))) {
		t.Errorf("key constants = %v, want one per entry", got)
	}
	if cat.name != "i18n" {
		t.Errorf("catalog package name = %q, want i18n", cat.name)
	}
}

// TestAudit_CatalogItCannotRead_FailsRatherThanPassing holds the reader to
// refusing a catalog it cannot read, each in its own words: with no English
// catalog every rule would be asked of nothing, which reads as a clean run.
func TestAudit_CatalogItCannotRead_FailsRatherThanPassing(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			name:  "no English catalog at all",
			files: map[string]string{"internal/i18n/other.go": "package i18n\n\nvar spanish = map[Key]string{Only: \"solo\"}\n"},
			want:  "example.com/broken/internal/i18n declares no English catalog: no package-level variable named english",
		},
		{
			name:  "an English catalog declared without a value",
			files: map[string]string{"internal/i18n/catalog_en.go": "package i18n\n\nvar english map[Key]string\n"},
			want:  "the English catalog of example.com/broken/internal/i18n is not written as a map literal",
		},
		{
			name:  "an English catalog built at run time",
			files: map[string]string{"internal/i18n/catalog_en.go": "package i18n\n\nvar english = make(map[Key]string)\n"},
			want:  "the English catalog of example.com/broken/internal/i18n is not written as a map literal",
		},
		{
			name:  "a template that is not a constant",
			files: map[string]string{"internal/i18n/catalog_en.go": "package i18n\n\nimport \"strings\"\n\nvar english = map[Key]string{\n\tOnly: strings.Repeat(\"x\", 2),\n}\n"},
			want:  "the English catalog of example.com/broken/internal/i18n holds an entry at internal/i18n/catalog_en.go:6 whose key or template is not a constant",
		},
		{
			name:  "a key that is not a constant",
			files: map[string]string{"internal/i18n/catalog_en.go": "package i18n\n\nimport \"os\"\n\nvar english = map[Key]string{\n\tKey(os.Args[0]): \"x\",\n}\n"},
			want:  "the English catalog of example.com/broken/internal/i18n holds an entry at internal/i18n/catalog_en.go:6 whose key or template is not a constant",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := audit(catalogModule(t, tc.files))
			if err == nil || err.Error() != tc.want {
				t.Errorf("audit error = %v, want %q", err, tc.want)
			}
		})
	}
}

// TestAudit_CatalogFoundAmongOtherDeclarations_IsRead holds the search to
// the variable itself: a function, a type, an import and a variable declared
// beside it in the same group are passed over, and the second name of a spec
// is found as the first is.
func TestAudit_CatalogFoundAmongOtherDeclarations_IsRead(t *testing.T) {
	cfg := catalogModule(t, map[string]string{
		"internal/i18n/catalog_en.go": "package i18n\n\nimport \"strings\"\n\ntype other struct{}\n\nfunc join() string { return strings.Join(nil, \"\") }\n\nvar spare, english = 1, map[Key]string{Only: \"only one\"}\n",
	})
	report, err := audit(cfg)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if !report.OK() || report.Summary.Keys != 1 {
		t.Errorf("report = %+v, want the one entry read and nothing to answer for", report)
	}
}
