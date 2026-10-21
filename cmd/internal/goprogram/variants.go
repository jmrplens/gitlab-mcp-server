package goprogram

import (
	"strings"

	"golang.org/x/tools/go/packages"
)

// PackagesUnderTest collects, from one load, the import paths the loader says
// a test executable was built for. `go list` reports a test variant as
// "p [q.test]" and fills [packages.Package.ForTest] with q for it; the
// executable itself arrives as plain "q.test" with ForTest empty, so the
// variants are where the name of the package under test can be read.
func PackagesUnderTest(loaded []*packages.Package) map[string]struct{} {
	underTest := map[string]struct{}{}
	for _, pkg := range loaded {
		if pkg.ForTest != "" {
			underTest[pkg.ForTest] = struct{}{}
		}
	}
	return underTest
}

// IsTestMain reports whether a loaded package is the test executable the go
// tool synthesizes for a package under test, "q.test", which is neither source
// of this repository nor a name a second load can be asked for. Its one
// generated file is nobody's source, so a gate passes over it.
//
// The path alone does not answer it. A directory may be named "foo.test", and
// the package in it then carries a path ending exactly as the synthesized
// executable does while being source somebody wrote; passing over it would
// drop everything it declares and report the package clean. So the path is
// only believed when the loader also produced a test variant for the package
// the suffix claims this was built for, which is a fact about the load rather
// than a guess about a name. [PackagesUnderTest] collects those, and
// [LoadMode] carries NeedForTest so a load that asks for tests has them.
func IsTestMain(pkg *packages.Package, underTest map[string]struct{}) bool {
	if pkg.Name != "main" || !strings.HasSuffix(pkg.PkgPath, ".test") {
		return false
	}
	_, built := underTest[strings.TrimSuffix(pkg.PkgPath, ".test")]
	return built
}

// VariantName is a package's import path with the test-variant decoration the
// loader adds taken off, so "p" and "p [p.test]" are one package. The suffix
// is decoration on the path rather than part of it, and the external test
// package keeps its own "_test" name because it really is a different package.
func VariantName(pkgPath string) string {
	plain, _, _ := strings.Cut(pkgPath, " [")
	return plain
}
