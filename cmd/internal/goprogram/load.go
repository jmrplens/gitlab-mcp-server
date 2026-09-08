package goprogram

import (
	"fmt"
	"strings"

	"golang.org/x/tools/go/packages"
)

// LoadMode is what the gates need from the loader: syntax to walk, types so an
// identifier resolves to the object it names and a constant expression folds to
// the one string it denotes, and imports so an object has one identity across
// the packages that share it.
//
// NeedDeps is deliberately absent, for one reason that holds for all four
// callers: each of them only ever reads bodies written inside the patterns it
// loads, so type-checking the dependency tree from source would cost minutes
// and change no answer. What a dependency's function returns is judged by its
// name and its signature, both of which come in through export data, which
// also gives each of its objects the same identity the packages using them see.
const LoadMode = packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
	packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports

// Load type-checks the packages named by patterns, rooted at dir, and returns
// them in the loader's order. Every returned package type-checked without
// error, so a caller may read TypesInfo without checking it first.
//
// The overlay is how a test supplies source that is not on disk: a fixture
// package written in the test file itself type-checks against the real
// packages it imports, so a gate is exercised on the shapes it has to handle
// rather than on a mock of them. Production passes nil.
//
// A pattern that matches nothing is an error rather than an empty result. A
// gate that audited no package would otherwise pass, which is the same silence
// [packageLoadError] refuses one package at a time.
func Load(dir string, patterns []string, overlay map[string][]byte) ([]*packages.Package, error) {
	cfg := &packages.Config{Mode: LoadMode, Dir: dir, Tests: false, Overlay: overlay}
	loaded, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("load packages: %w", err)
	}
	if len(loaded) == 0 {
		return nil, fmt.Errorf("no packages matched %s in %s", strings.Join(patterns, " "), dir)
	}
	for _, pkg := range loaded {
		if loadErr := packageLoadError(pkg); loadErr != nil {
			return nil, loadErr
		}
	}
	return loaded, nil
}

// packageLoadError turns a package's load errors into one reportable error.
//
// A package that did not type-check resolves nothing, and every gate loading
// through here answers "cannot tell" for what it cannot resolve, so such a
// package would turn into a clean report over source nobody understood. It is
// refused instead, naming the package and its first error.
func packageLoadError(pkg *packages.Package) error {
	if len(pkg.Errors) == 0 {
		return nil
	}
	return fmt.Errorf("load %s: %w", pkg.PkgPath, pkg.Errors[0])
}
