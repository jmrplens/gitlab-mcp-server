// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// module_test.go covers reading the module path and the requirements from
// go.mod, and the image name derived from the module path.
package main

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReadGoMod_ReadsTheModuleDirective covers the directive written bare,
// quoted and with a comment after it, and this repository's own go.mod, whose
// module path the document's module product has to name.
func TestReadGoMod_ReadsTheModuleDirective(t *testing.T) {
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
			got, err := readGoMod(path)
			if err != nil || got.path != tc.want || len(got.requires) != 0 {
				t.Errorf("readGoMod = %+v, %v; want the path %q and no requirement", got, err, tc.want)
			}
		})
	}

	t.Run("this repository", func(t *testing.T) {
		t.Parallel()

		got, err := readGoMod(filepath.Join("..", "..", "go.mod"))
		if err != nil || got.path != "github.com/jmrplens/gitlab-mcp-server/v3" {
			t.Errorf("readGoMod of the repository's go.mod = %+v, %v", got, err)
		}
		// golang.org/x/crypto is what the repository's document states an
		// advisory against, and go.mod requires it for tests alone.
		if _, ok := got.requires["golang.org/x/crypto"]; !ok {
			t.Errorf("readGoMod of the repository's go.mod does not name golang.org/x/crypto among %d requirements", len(got.requires))
		}
	})
}

// TestReadGoMod_ReadsEveryRequireDirective covers a requirement written on
// one line and in a block, quoted and marked indirect, and the directives
// beside them that name modules without requiring them, which a reader of
// require lines alone would otherwise take for requirements.
func TestReadGoMod_ReadsEveryRequireDirective(t *testing.T) {
	t.Parallel()

	content := `module example.com/fixture

go 1.27

require example.com/single v1.0.0

require (
	example.com/direct v1.1.0
	"example.com/quoted" v1.2.0 // indirect

	example.com/lonely
)

replace (
	example.com/direct => example.com/fork v9.0.0
)

exclude example.com/excluded v0.1.0
`
	path := filepath.Join(t.TempDir(), "go.mod")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing go.mod: %v", err)
	}
	got, err := readGoMod(path)
	if err != nil {
		t.Fatalf("readGoMod: %v", err)
	}
	want := map[string]string{"example.com/single": "v1.0.0", "example.com/direct": "v1.1.0", "example.com/quoted": "v1.2.0"}
	if got.path != "example.com/fixture" || !maps.Equal(got.requires, want) {
		t.Errorf("readGoMod = %+v, want the path example.com/fixture and the requirements %v", got, want)
	}
}

// TestReadGoMod_AFileWithNoModule_IsAnError covers a go.mod that cannot be
// read and one that declares no module.
func TestReadGoMod_AFileWithNoModule_IsAnError(t *testing.T) {
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

			if _, err := readGoMod(tc.path); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("readGoMod(%s) error = %v, want one naming %q", tc.path, err, tc.want)
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
