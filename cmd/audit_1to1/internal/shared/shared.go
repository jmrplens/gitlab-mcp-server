// Package shared holds constants and helpers used across multiple audit_1to1
// sub-packages so they cannot drift.
package shared

import (
	"fmt"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"
)

// loadCache memoizes LoadToolPackages per root. Loading the typed package
// set for ./internal/tools/... costs 10-20s, and every analyzer plus every
// test that calls buildReport twice used to pay it again. The audits treat
// the loaded packages as read-only type information, which is what makes a
// process-lifetime shared result safe; the CLIs are one-shot anyway.
var loadCache sync.Map // root -> *loadResult

type loadResult struct {
	once sync.Once
	pkgs []*packages.Package
	err  error
}

const (
	ClientGoPkgPath = "gitlab.com/gitlab-org/api/client-go"
	ToolsPkgInfix   = "/internal/tools/"
	SchemaVersion   = 1
)

// LoadToolPackages loads every package under ./internal/tools/... rooted at root
// with full type information, returning only the tool sub-packages that resolved
// types successfully. It is shared by the structs and actions analyzers, which
// both need the same typed package set. A package-load error aborts the run.
// The result is memoized per root and must be treated as read-only.
func LoadToolPackages(root string) ([]*packages.Package, error) {
	entry, _ := loadCache.LoadOrStore(root, &loadResult{})
	result, _ := entry.(*loadResult)
	result.once.Do(func() {
		result.pkgs, result.err = loadToolPackages(root)
	})
	return result.pkgs, result.err
}

// loadToolPackages performs the actual load; see LoadToolPackages.
func loadToolPackages(root string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports,
		Dir: root,
	}
	loaded, err := packages.Load(cfg, "./internal/tools/...")
	if err != nil {
		return nil, fmt.Errorf("load packages: %w", err)
	}
	var fatal []string
	out := make([]*packages.Package, 0, len(loaded))
	for _, pkg := range loaded {
		for _, perr := range pkg.Errors {
			fatal = append(fatal, perr.Error())
		}
		if !strings.Contains(pkg.PkgPath, ToolsPkgInfix) {
			continue
		}
		if pkg.Types == nil || pkg.TypesInfo == nil {
			continue
		}
		out = append(out, pkg)
	}
	if len(fatal) > 0 {
		return nil, fmt.Errorf("package load errors:\n%s", strings.Join(fatal, "\n"))
	}
	return out, nil
}
