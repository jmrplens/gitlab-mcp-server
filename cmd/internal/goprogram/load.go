package goprogram

import (
	"fmt"
	"strings"

	"golang.org/x/tools/go/packages"
)

// ModulePath is this repository's module path, the prefix every gate trims off
// an import path so a report names a package the way the repository does, and
// the prefix it resolves the shared packages under.
//
// It is spelled here once. Two gates used to carry their own copy, one built
// from a module constant and one written out in full, and a module path that
// moves (it did, at v3) is a change that has to land in every copy at once or
// a gate goes on resolving a package that no longer exists.
const ModulePath = "github.com/jmrplens/gitlab-mcp-server/v3"

// ToolutilPath is the import path of the package that owns the escaping
// helpers, ActionSpec, the route constructors and the shared GraphQL
// executors: the one package every gate resolves calls into. Resolution keys
// on the path rather than on the package name so an import alias cannot fool
// it.
const ToolutilPath = ModulePath + "/internal/toolutil"

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

// Options is what a load may ask for beyond what every gate shares.
//
// The four gates that existed first load production source with no build tag
// and no test files, and [Load] still gives them exactly that. The e2e
// coverage gate reads test packages that only exist behind the e2e tag, which
// is a different load and not a different wording of the same one, so it asks
// for both here rather than every caller gaining two parameters it passes as
// zero.
type Options struct {
	// Tests includes each package's test variants, type-checked from source,
	// so a gate over _test.go files sees them as the test binary would.
	Tests bool
	// BuildTags are the build constraints the load satisfies, as -tags would.
	// Empty means the default constraints, which is what every earlier caller
	// loads under.
	BuildTags []string
	// Overlay supplies source that is not on disk, as [Load]'s parameter
	// does.
	Overlay map[string][]byte
}

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
	return LoadWith(dir, patterns, Options{Overlay: overlay})
}

// LoadWith is [Load] with the options a caller states: test variants, build
// tags, an overlay. Everything [Load] refuses, this refuses on the same terms.
func LoadWith(dir string, patterns []string, opts Options) ([]*packages.Package, error) {
	cfg := &packages.Config{Mode: LoadMode, Dir: dir, Tests: opts.Tests, Overlay: opts.Overlay}
	if len(opts.BuildTags) > 0 {
		cfg.BuildFlags = []string{"-tags=" + strings.Join(opts.BuildTags, ",")}
	}
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
