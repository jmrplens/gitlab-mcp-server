package shared

import (
	"fmt"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"
)

// loadCache memoizes LoadToolPackages per root. Loading the typed package
// set for ./internal/tools/... is the most expensive thing any of these
// analyzers does — it was 10-20s when it type-checked the dependency tree
// from source, and is a fraction of a second now that [loadMode] leaves that
// to export data — and every analyzer plus every test that calls buildReport
// twice used to pay it again. The audits treat the loaded packages as
// read-only type information, which is what makes a process-lifetime shared
// result safe; the CLIs are one-shot anyway.
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

// loadPackages is the go/packages loader, a variable so a test can hand the
// filter a package the real loader never produces: one under internal/tools
// with no type information at all.
var loadPackages = packages.Load

// loadMode is what the audit needs from the loader: the tool packages'
// own syntax and type information, so a handler body can be walked and an
// identifier resolved to the object it names, plus their imports, so the
// client-go package the handlers compile against is reachable through the
// graph rather than loaded a second time.
//
// NeedDeps is deliberately absent. It is what decides whether a dependency
// is type-checked from source or read from export data (go/packages sets
// needsrc on a non-root only when it is set, and asks go list for -export
// only when it is not), and with it every transitive import of the 180 tool
// packages — client-go, the MCP SDK, their trees — is re-typechecked from
// source on every run. Nothing here reads a dependency's syntax: the audit
// asks client-go for its struct fields, their json tags and its service
// method signatures, all of which export data carries, and gives each object
// the same identity the tool packages themselves see.
//
// What the two readers of the import graph need stays populated without it.
// go/packages marks the *immediate* dependencies of every source package
// needtypes regardless of NeedDeps ("Complete type information is required
// for the immediate dependencies of each source package"), and client-go is
// a direct import of the tool packages, so shared.ClientGoTypes still finds
// its *types.Package and structs.clientGoDir still finds its GoFiles.
// Measured on this tree, with the build cache warm as it is whenever a test
// or a build has run: 1.0s to 0.4s per load, and the six reports come out
// byte for byte identical.
const loadMode = packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
	packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports

// loadToolPackages performs the actual load; see LoadToolPackages.
func loadToolPackages(root string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: loadMode,
		Dir:  root,
	}
	loaded, err := loadPackages(cfg, "./internal/tools/...")
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
