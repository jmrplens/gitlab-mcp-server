// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"
)

// loadCache memoizes LoadToolPackages and LoadToolAndCapabilityPackages,
// one load per root for both. Loading the typed package
// set for ./internal/tools/... is the most expensive thing any of these
// analyzers does — it was 10-20s when it type-checked the dependency tree
// from source, and is a fraction of a second now that [loadMode] leaves that
// to export data — and every analyzer plus every test that calls buildReport
// twice used to pay it again. The audits treat the loaded packages as
// read-only type information, which is what makes a process-lifetime shared
// result safe; the CLIs are one-shot anyway.
var loadCache sync.Map // root -> *loadResult

type loadResult struct {
	once         sync.Once
	pkgs         []*packages.Package
	capabilities []*packages.Package
	err          error
}

// What the 1:1 analyzers share: the import-path fragments that identify
// client-go and the tool packages, and the schema version of their reports.
const (
	ClientGoPkgPath = "gitlab.com/gitlab-org/api/client-go"
	ToolsPkgInfix   = "/internal/tools/"
	SchemaVersion   = 1
)

// LoadToolPackages loads every package under ./internal/tools/... rooted at root
// with full type information, returning only the tool sub-packages that resolved
// types successfully. It is the typed package set the structs, actions, enums
// and sdk analyzers read, and the paths and params scopes through
// structs.CollectOutputPairings and sdk.RawRequests. A package-load error
// aborts the run, one in a package under [capabilityDirs] included: the load
// that yields the tool packages is the one that yields those, so a caller that
// never reads them fails on them all the same. The result is memoized per root
// and must be treated as read-only.
//
// The shared ./internal/toolutil package, when root has one, is loaded from
// its source in the same pass and left out of the result: a tool package's
// Imports hand it over with its syntax and type information, the same
// objects the tool packages compile against, so a walk can follow a call
// into one of its bodies (the sdk scope's captures rule follows a capture
// into the reader that decodes it). So are the packages under
// [capabilityDirs], which [LoadToolAndCapabilityPackages] hands over.
func LoadToolPackages(root string) ([]*packages.Package, error) {
	pkgs, _, err := LoadToolAndCapabilityPackages(root)
	return pkgs, err
}

// LoadToolAndCapabilityPackages returns what [LoadToolPackages] returns, from
// the same memoized load, and beside it the packages under [capabilityDirs]
// that root has, with their syntax and full type information. No tool
// package imports them, so no Imports hands them over, and they are loaded
// in the same pass so that a type they share with the tool packages (a
// client-go struct, a shape of internal/toolutil) is the one object the tool
// packages see. Only the sdk scope's captures rule reads them; every other
// analyzer keeps to the tool packages. A load error in one aborts the run as
// one in a tool package does.
func LoadToolAndCapabilityPackages(root string) (tools, capabilities []*packages.Package, err error) {
	entry, _ := loadCache.LoadOrStore(root, &loadResult{})
	result, _ := entry.(*loadResult)
	result.once.Do(func() {
		result.pkgs, result.capabilities, result.err = loadToolPackages(root)
	})
	return result.pkgs, result.capabilities, result.err
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
// the same identity the tool packages themselves see. Three packages are read
// from source beside the tool packages, internal/toolutil and the two of
// [capabilityDirs], and each is a root of the load rather than a dependency,
// so their imports stay export data.
//
// What the two readers of the import graph need stays populated without it.
// go/packages marks the *immediate* dependencies of every source package
// needtypes regardless of NeedDeps ("Complete type information is required
// for the immediate dependencies of each source package"), and client-go is
// a direct import of the tool packages, so shared.ClientGoTypes still finds
// its *types.Package and structs.ClientGoDir still finds its GoFiles.
// Measured on this tree, with the build cache warm as it is whenever a test
// or a build has run: 1.0s to 0.4s per load, and the six reports come out
// byte for byte identical. Reading the two packages of [capabilityDirs] from
// source as well (issue 1601) costs nothing measurable: nine uncached loads
// each way on one machine gave medians of 0.61s without them and 0.62s with
// them, inside the spread of either.
const loadMode = packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
	packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports

// toolutilDir is the shared package the tool packages import, relative to the
// module root, which LoadToolPackages loads from its source beside them.
const toolutilDir = "internal/toolutil"

// capabilityDirs are the packages that serve the MCP resources and prompts,
// relative to the module root. They call client-go and capture its answers
// as the tool packages do (issue 1601), and LoadToolAndCapabilityPackages
// hands them over from the same pass.
var capabilityDirs = []string{"internal/resources", "internal/prompts"}

// loadToolPackages performs the actual load; see LoadToolPackages and
// LoadToolAndCapabilityPackages.
func loadToolPackages(root string) (tools, capabilities []*packages.Package, err error) {
	cfg := &packages.Config{
		Mode: loadMode,
		Dir:  root,
	}
	patterns := []string{"./internal/tools/..."}
	// A module without the shared package or a capability package, as most
	// test fixtures are, would fail the whole load on a pattern that names no
	// directory.
	for _, dir := range append([]string{toolutilDir}, capabilityDirs...) {
		if _, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(dir))); statErr == nil {
			patterns = append(patterns, "./"+dir)
		}
	}
	loaded, err := loadPackages(cfg, patterns...)
	if err != nil {
		return nil, nil, fmt.Errorf("load packages: %w", err)
	}
	var fatal []string
	tools = make([]*packages.Package, 0, len(loaded))
	capabilities = make([]*packages.Package, 0, len(capabilityDirs))
	for _, pkg := range loaded {
		for _, perr := range pkg.Errors {
			fatal = append(fatal, perr.Error())
		}
		if pkg.Types == nil || pkg.TypesInfo == nil {
			continue
		}
		if strings.Contains(pkg.PkgPath, ToolsPkgInfix) {
			tools = append(tools, pkg)
		} else if isCapabilityPackage(pkg.PkgPath) {
			capabilities = append(capabilities, pkg)
		}
	}
	if len(fatal) > 0 {
		return nil, nil, fmt.Errorf("package load errors:\n%s", strings.Join(fatal, "\n"))
	}
	return tools, capabilities, nil
}

// isCapabilityPackage reports whether pkgPath is one of the packages under
// capabilityDirs, whatever the module it is in.
func isCapabilityPackage(pkgPath string) bool {
	for _, dir := range capabilityDirs {
		if strings.HasSuffix(pkgPath, "/"+dir) {
			return true
		}
	}
	return false
}
