package sourcewalk

import (
	"os"
	"path/filepath"
	"strings"
)

// gitMarker is the entry git writes at the root of a checkout: a directory in
// a clone, a file naming the real git directory in a linked worktree. Either
// one answers the only question asked of it, so the kind is never inspected.
const gitMarker = ".git"

// lstat is a seam over os.Lstat, so a test can drive the branch a filesystem
// the tests own never produces: an entry that exists and cannot be stated.
var lstat = os.Lstat

// SkipDir reports whether a directory of this base name is left out of a walk
// that descends into it, by name alone.
//
// The rule is Go's own for ./...: a dot-directory holds no source this
// repository publishes. The relative names "." and ".." are exempt because
// they name a tree the caller is already in rather than one to descend into.
//
// This is the cheap half of the rule and is not the whole of it: see
// [SkipDirBelowRoot].
func SkipDir(name string) bool {
	switch name {
	case ".", "..":
		return false
	default:
		return strings.HasPrefix(name, ".")
	}
}

// IsNestedCheckout reports whether dir is the root of another git checkout: a
// clone, a submodule, or one of the linked worktrees the parallel-agent
// tooling creates. It answers false for a directory it cannot read, because a
// walk that cannot stat a path has a failure to report and this is not the
// place that reports it.
//
// It is deliberately a question about the directory rather than about its
// name. A worktree is a complete copy of this repository whatever it is
// called, and a copy folded into a walk moves that walk's answer without
// saying so.
func IsNestedCheckout(dir string) bool {
	_, err := lstat(filepath.Join(dir, gitMarker))
	return err == nil
}

// SkipDirBelowRoot reports whether a walk leaves the directory at path out.
// It is the whole rule, and either half is enough to leave it out: the base
// name is a dot-directory's, or the directory is another checkout.
//
// It is only ever asked of a directory below a walk root. The root itself is
// entered whatever it is called and whether or not it is a checkout, which is
// what keeps a scan pointed at one of these directories by name, or at a
// repository root, from walking nothing at all.
func SkipDirBelowRoot(path string) bool {
	return SkipDir(filepath.Base(path)) || IsNestedCheckout(path)
}
