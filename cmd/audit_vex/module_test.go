// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// module_test.go covers reading the module path from go.mod and the image
// name derived from it.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReadModulePath_ReadsTheModuleDirective covers the directive written
// bare, quoted and with a comment after it, and this repository's own go.mod,
// whose module path the document's module product has to name.
func TestReadModulePath_ReadsTheModuleDirective(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, content, want string
	}{
		{name: "bare", content: "module example.com/fixture\n\ngo 1.27\n", want: "example.com/fixture"},
		{name: "quoted", content: "// leading comment\nmodule \"example.com/quoted\" // trailing\n", want: "example.com/quoted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "go.mod")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatalf("writing go.mod: %v", err)
			}
			got, err := readModulePath(path)
			if err != nil || got != tc.want {
				t.Errorf("readModulePath = %q, %v; want %q", got, err, tc.want)
			}
		})
	}

	t.Run("this repository", func(t *testing.T) {
		t.Parallel()

		got, err := readModulePath(filepath.Join("..", "..", "go.mod"))
		if err != nil || got != "github.com/jmrplens/gitlab-mcp-server/v3" {
			t.Errorf("readModulePath of the repository's go.mod = %q, %v", got, err)
		}
	})
}

// TestReadModulePath_AFileWithNoModule_IsAnError covers a go.mod that cannot
// be read and one that declares no module.
func TestReadModulePath_AFileWithNoModule_IsAnError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	empty := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(empty, []byte("go 1.27\n"), 0o600); err != nil {
		t.Fatalf("writing go.mod: %v", err)
	}
	for _, tc := range []struct {
		name, path, want string
	}{
		{name: "absent", path: filepath.Join(dir, "absent", "go.mod"), want: "absent"},
		{name: "no directive", path: empty, want: "declares no module"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := readModulePath(tc.path); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("readModulePath(%s) error = %v, want one naming %q", tc.path, err, tc.want)
			}
		})
	}
}

// TestImageName_SetsTheMajorSuffixAside covers a module with a major version
// suffix, one without, and an element that only looks like a suffix.
func TestImageName_SetsTheMajorSuffixAside(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		module, want string
	}{
		{module: "github.com/jmrplens/gitlab-mcp-server/v3", want: "gitlab-mcp-server"},
		{module: "example.com/tool/v12", want: "tool"},
		{module: "example.com/fixture", want: "fixture"},
		{module: "example.com/tool/v1", want: "v1"},
		{module: "example.com/tool/v02", want: "v02"},
	} {
		t.Run(tc.module, func(t *testing.T) {
			t.Parallel()

			if got := imageName(tc.module); got != tc.want {
				t.Errorf("imageName(%q) = %q, want %q", tc.module, got, tc.want)
			}
		})
	}
}
