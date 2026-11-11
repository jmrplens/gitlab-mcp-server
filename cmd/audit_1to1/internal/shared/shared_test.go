// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package shared tests the memoized typed-package loader the analyzers
// share, the tool packages and the capability packages beside them, against
// small throwaway modules so the tests state exactly which packages resolve
// and which errors abort a run.
package shared

import (
	"fmt"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	_ "github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/serialtypecheck" // serial type-checking under -race, golang/go#81122
)

// writeModule creates a Go module under a temp dir with the given files
// (paths relative to the module root) and returns the root.
func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	files["go.mod"] = "module example.com/fixture\n\ngo 1.27\n"
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return root
}

// TestLoadToolPackages_Module_ReturnsTypedToolPackagesOnce verifies the
// loader resolves every package under internal/tools with type information,
// leaves out both a package outside that tree and the root tools package
// itself (whose path lacks the /internal/tools/ infix, as the real
// internal/tools orchestration package does), and memoizes the result per
// root so a second call hands back the same slice instead of loading again.
func TestLoadToolPackages_Module_ReturnsTypedToolPackagesOnce(t *testing.T) {
	root := writeModule(t, map[string]string{
		"internal/tools/tools.go":       "package tools\n\n// Register is the root orchestration package, outside the audited set.\nfunc Register() {}\n",
		"internal/tools/alpha/alpha.go": "package alpha\n\n// A is a typed symbol.\nfunc A() int { return 1 }\n",
		"internal/tools/beta/beta.go":   "package beta\n\nimport \"strings\"\n\n// B uses an import so the loader resolves dependencies.\nfunc B() string { return strings.ToUpper(\"b\") }\n",
		"internal/other/other.go":       "package other\n\nfunc O() {}\n",
	})

	first, err := LoadToolPackages(root)
	if err != nil {
		t.Fatalf("LoadToolPackages: %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("loaded %d packages, want the 2 sub-packages under internal/tools", len(first))
	}
	seen := map[string]bool{}
	for _, pkg := range first {
		seen[pkg.PkgPath] = true
		if !strings.Contains(pkg.PkgPath, ToolsPkgInfix) {
			t.Errorf("%s is outside %s", pkg.PkgPath, ToolsPkgInfix)
		}
		if pkg.Types == nil || pkg.TypesInfo == nil || len(pkg.Syntax) == 0 {
			t.Errorf("%s loaded without types or syntax", pkg.PkgPath)
		}
	}
	for _, want := range []string{"example.com/fixture/internal/tools/alpha", "example.com/fixture/internal/tools/beta"} {
		t.Run(want, func(t *testing.T) {
			if !seen[want] {
				t.Errorf("%s not loaded (got %v)", want, seen)
			}
		})
	}

	second, err := LoadToolPackages(root)
	if err != nil {
		t.Fatalf("second LoadToolPackages: %v", err)
	}
	if len(second) != len(first) || &second[0] != &first[0] {
		t.Error("second call did not return the memoized slice")
	}
	tools, capabilities, err := LoadToolAndCapabilityPackages(root)
	if err != nil {
		t.Fatalf("LoadToolAndCapabilityPackages: %v", err)
	}
	if len(tools) != len(first) || &tools[0] != &first[0] {
		t.Error("LoadToolAndCapabilityPackages did not return the memoized tool packages")
	}
	if len(capabilities) != 0 {
		t.Errorf("capabilities = %v, want none in a tree without them, the root tools package included", capabilities)
	}
}

// TestLoadToolAndCapabilityPackages_TheResourceAndPromptPackages_AreLoadedFromTheirSource
// verifies that internal/resources and internal/prompts are loaded beside
// the tool packages in the same pass and handed over apart from them (issue
// 1601): with their syntax and type information, their unexported
// declarations in their scope, and compiled against the same toolutil
// package object the tool packages see, so a type they share is one object.
// A package outside the two (internal/completions here) is not loaded, and a
// second call hands back the memoized slices.
func TestLoadToolAndCapabilityPackages_TheResourceAndPromptPackages_AreLoadedFromTheirSource(t *testing.T) {
	const shape = "package %s\n\nimport \"example.com/fixture/internal/toolutil\"\n\ntype row struct{ ID int }\n\n// Read is exported, row is not.\nfunc Read() toolutil.Shape { return toolutil.Shape{ID: len([]row{})} }\n"
	root := writeModule(t, map[string]string{
		"internal/toolutil/toolutil.go":       "package toolutil\n\n// Shape is shared.\ntype Shape struct{ ID int }\n",
		"internal/tools/alpha/alpha.go":       fmt.Sprintf(shape, "alpha"),
		"internal/resources/resources.go":     fmt.Sprintf(shape, "resources"),
		"internal/prompts/prompts.go":         fmt.Sprintf(shape, "prompts"),
		"internal/completions/completions.go": fmt.Sprintf(shape, "completions"),
	})
	tools, capabilities, err := LoadToolAndCapabilityPackages(root)
	if err != nil {
		t.Fatalf("LoadToolAndCapabilityPackages: %v", err)
	}
	if len(tools) != 1 || tools[0].PkgPath != "example.com/fixture/internal/tools/alpha" {
		t.Fatalf("tools = %v, want the one tool package", tools)
	}
	const toolutilPath = "example.com/fixture/internal/toolutil"
	toolutil := tools[0].Imports[toolutilPath].Types
	var paths []string
	for _, pkg := range capabilities {
		paths = append(paths, pkg.PkgPath)
		if len(pkg.Syntax) == 0 || pkg.TypesInfo == nil || pkg.Types.Scope().Lookup("row") == nil {
			t.Errorf("%s = %+v, want it loaded from its source", pkg.PkgPath, pkg)
		}
		if imported := pkg.Imports[toolutilPath]; imported == nil || imported.Types != toolutil {
			t.Errorf("%s compiles against %v, want the toolutil package the tool packages see", pkg.PkgPath, imported)
		}
	}
	slices.Sort(paths)
	if want := []string{"example.com/fixture/internal/prompts", "example.com/fixture/internal/resources"}; !slices.Equal(paths, want) {
		t.Errorf("capabilities = %q, want %q", paths, want)
	}
	again, againCapabilities, err := LoadToolAndCapabilityPackages(root)
	if err != nil {
		t.Fatalf("second LoadToolAndCapabilityPackages: %v", err)
	}
	if &again[0] != &tools[0] || len(againCapabilities) != len(capabilities) || &againCapabilities[0] != &capabilities[0] {
		t.Error("second call did not return the memoized slices")
	}
}

// TestLoadToolPackages_TheSharedToolutilPackage_IsLoadedFromItsSource
// verifies that the shared internal/toolutil package is loaded beside the
// tool packages and left out of the result: a tool package's Imports hand it
// over with its syntax and type information, and its unexported declarations,
// which export data leaves out, are in its scope, the same package object the
// tool package compiles against.
func TestLoadToolPackages_TheSharedToolutilPackage_IsLoadedFromItsSource(t *testing.T) {
	root := writeModule(t, map[string]string{
		"internal/toolutil/toolutil.go": "package toolutil\n\ntype row struct{ ID int }\n\n// Rows is exported, row is not.\nfunc Rows() int { return len([]row{}) }\n",
		"internal/tools/alpha/alpha.go": "package alpha\n\nimport \"example.com/fixture/internal/toolutil\"\n\n// A calls the shared package.\nfunc A() int { return toolutil.Rows() }\n",
	})
	pkgs, err := LoadToolPackages(root)
	if err != nil {
		t.Fatalf("LoadToolPackages: %v", err)
	}
	if len(pkgs) != 1 || pkgs[0].PkgPath != "example.com/fixture/internal/tools/alpha" {
		t.Fatalf("loaded %v, want the one tool package", pkgs)
	}
	toolutil := pkgs[0].Imports["example.com/fixture/internal/toolutil"]
	if toolutil == nil || len(toolutil.Syntax) == 0 || toolutil.TypesInfo == nil || toolutil.Types.Scope().Lookup("row") == nil {
		t.Fatalf("toolutil = %+v, want it loaded from its source", toolutil)
	}
	if used := pkgs[0].Types.Imports()[0]; used != toolutil.Types {
		t.Errorf("alpha compiles against %v, want the toolutil package the load hands over", used)
	}
}

// TestLoadToolPackages_UntypedPackage_IsLeftOut verifies the filter's last
// guard: a package under internal/tools or a capability package that the
// loader hands back with no type information is dropped rather than passed
// to an analyzer that would dereference it. The real loader always attaches
// types when asked to, so the case is reached through the loader seam.
func TestLoadToolPackages_UntypedPackage_IsLeftOut(t *testing.T) {
	original := loadPackages
	t.Cleanup(func() { loadPackages = original })
	loadPackages = func(*packages.Config, ...string) ([]*packages.Package, error) {
		return []*packages.Package{
			{PkgPath: "example.com/untyped/internal/tools/alpha"},
			{PkgPath: "example.com/untyped/internal/tools/beta", Types: types.NewPackage("example.com/untyped/internal/tools/beta", "beta"), TypesInfo: &types.Info{}},
			{PkgPath: "example.com/untyped/internal/resources"},
			{PkgPath: "example.com/untyped/internal/prompts", Types: types.NewPackage("example.com/untyped/internal/prompts", "prompts"), TypesInfo: &types.Info{}},
		}, nil
	}

	pkgs, capabilities, err := LoadToolAndCapabilityPackages(filepath.Join(t.TempDir(), "untyped"))
	if err != nil {
		t.Fatalf("LoadToolAndCapabilityPackages: %v", err)
	}
	if len(pkgs) != 1 || pkgs[0].PkgPath != "example.com/untyped/internal/tools/beta" {
		t.Errorf("loaded %v, want only the typed beta package", pkgs)
	}
	if len(capabilities) != 1 || capabilities[0].PkgPath != "example.com/untyped/internal/prompts" {
		t.Errorf("capabilities = %v, want only the typed prompts package", capabilities)
	}
}

// TestLoadToolPackages_HalfTypedPackage_IsLeftOut verifies the same guard for
// the two halves of the type information separately. A package carrying one
// and not the other is as unusable to an analyzer as one carrying neither:
// the structs pass walks TypesInfo for the object an identifier names, and the
// actions pass reads Types for the declared scope, so keeping such a package
// would hand one of them a nil to dereference. The real loader never produces
// this shape, so it is reached through the loader seam, and each case uses a
// root of its own because the result is memoized per root.
func TestLoadToolPackages_HalfTypedPackage_IsLeftOut(t *testing.T) {
	const typedPath = "example.com/half/internal/tools/typed"
	typed := func() *packages.Package {
		return &packages.Package{
			PkgPath:   typedPath,
			Types:     types.NewPackage(typedPath, "typed"),
			TypesInfo: &types.Info{},
		}
	}
	cases := []struct {
		name string
		half *packages.Package
	}{
		{
			name: "types_without_types_info",
			half: &packages.Package{
				PkgPath: "example.com/half/internal/tools/notypesinfo",
				Types:   types.NewPackage("example.com/half/internal/tools/notypesinfo", "notypesinfo"),
			},
		},
		{
			name: "types_info_without_types",
			half: &packages.Package{
				PkgPath:   "example.com/half/internal/tools/notypes",
				TypesInfo: &types.Info{},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original := loadPackages
			t.Cleanup(func() { loadPackages = original })
			loadPackages = func(*packages.Config, ...string) ([]*packages.Package, error) {
				return []*packages.Package{tc.half, typed()}, nil
			}

			pkgs, err := LoadToolPackages(filepath.Join(t.TempDir(), tc.name))
			if err != nil {
				t.Fatalf("LoadToolPackages: %v", err)
			}
			if len(pkgs) != 1 {
				t.Fatalf("loaded %d packages, want only the fully typed one", len(pkgs))
			}
			if pkgs[0].PkgPath != typedPath {
				t.Errorf("loaded %q, want %q", pkgs[0].PkgPath, typedPath)
			}
		})
	}
}

// TestLoadToolPackages_Failures_AbortTheRun verifies both failure classes
// surface as errors rather than a partial package set: a root the go tool
// cannot enter, and a tool package or a capability package that does not
// type-check.
func TestLoadToolPackages_Failures_AbortTheRun(t *testing.T) {
	cases := []struct {
		name    string
		root    func(t *testing.T) string
		wantErr string
	}{
		{
			name: "missing_root",
			root: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "absent")
			},
			wantErr: "load packages: ",
		},
		{
			name: "type_error_in_a_tool_package",
			root: func(t *testing.T) string {
				t.Helper()
				return writeModule(t, map[string]string{
					"internal/tools/broken/broken.go": "package broken\n\nfunc B() int { return \"not an int\" }\n",
				})
			},
			wantErr: "package load errors:",
		},
		{
			name: "type_error_in_a_capability_package",
			root: func(t *testing.T) string {
				t.Helper()
				return writeModule(t, map[string]string{
					"internal/tools/alpha/alpha.go":   "package alpha\n\nfunc A() int { return 1 }\n",
					"internal/prompts/broken.go":      "package prompts\n\nfunc P() int { return \"not an int\" }\n",
					"internal/resources/resources.go": "package resources\n\nfunc R() int { return 1 }\n",
				})
			},
			wantErr: "package load errors:",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := tc.root(t)
			if _, capabilities, err := LoadToolAndCapabilityPackages(root); err == nil || capabilities != nil {
				t.Errorf("capabilities = %v, error = %v, want none and an error", capabilities, err)
			}
			pkgs, err := LoadToolPackages(root)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("LoadToolPackages error = %v, want it to contain %q", err, tc.wantErr)
			}
			if pkgs != nil {
				t.Errorf("packages = %d on failure, want none", len(pkgs))
			}
			if _, again := LoadToolPackages(root); again == nil || again.Error() != err.Error() {
				t.Errorf("memoized failure = %v, want the same error again", again)
			}
		})
	}
}
