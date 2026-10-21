package goprogram

import (
	"path/filepath"
	"strings"
)

// TrimModulePath names a package the way the repository does: its import path
// with [ModulePath] taken off, so a report says internal/oauth, and a package
// of any other module keeps its path whole. The module path is matched as a
// whole path element, so a module whose path merely begins with the same
// characters is another module rather than a package of this one.
func TrimModulePath(pkgPath string) string {
	rest, found := strings.CutPrefix(pkgPath, ModulePath)
	if !found || (rest != "" && rest[0] != '/') {
		return pkgPath
	}
	return strings.TrimPrefix(rest, "/")
}

// RelativePath renders a file below the repository root, so a finding reads
// the same wherever the audit was run from. A file the root does not contain
// is named in full rather than climbed to, and so is one that cannot be made
// relative to the root at all, because one of the two is relative and the
// other is not; either way the separators are slashes. A climb is a ".." path
// element, so a file below the root whose first directory merely begins with
// two dots is still named relative to it.
func RelativePath(path, root string) string {
	if rel, err := filepath.Rel(root, path); err == nil && !climbsOut(rel) {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

// climbsOut reports whether a path filepath.Rel returned leaves the directory
// it was made relative to, which it does only through a leading ".." element.
func climbsOut(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
