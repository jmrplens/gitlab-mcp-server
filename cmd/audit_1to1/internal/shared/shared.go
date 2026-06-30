// Package shared holds constants and helpers used across multiple audit_1to1
// sub-packages so they cannot drift.
package shared

import (
	"fmt"
	"strings"

	"golang.org/x/tools/go/packages"
)

const (
	ClientGoPkgPath = "gitlab.com/gitlab-org/api/client-go"
	ToolsPkgInfix   = "/internal/tools/"
	SchemaVersion   = 1
)

// LoadToolPackages loads every package under ./internal/tools/... rooted at root
// with full type information, returning only the tool sub-packages that resolved
// types successfully. It is shared by the structs and actions analyzers, which
// both need the same typed package set. A package-load error aborts the run.
func LoadToolPackages(root string) ([]*packages.Package, error) {
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
