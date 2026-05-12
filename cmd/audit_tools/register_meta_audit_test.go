package main

import (
	"os"
	"path/filepath"
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
