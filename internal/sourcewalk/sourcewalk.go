package sourcewalk

import (
	"io/fs"
	"os"
	"path"
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

// SkipDirBelowRoot reports whether a walk leaves the directory at dir out.
// It is the whole rule, and either half is enough to leave it out: the base
// name is a dot-directory's, or the directory is another checkout.
//
// It is only ever asked of a directory below a walk root. The root itself is
// entered whatever it is called and whether or not it is a checkout, which is
// what keeps a scan pointed at one of these directories by name, or at a
// repository root, from walking nothing at all. The caller is what knows which
// path is its root, so the caller is what must not ask about it; the [fs.FS]
// form below needs no such care, and that is the difference between them.
func SkipDirBelowRoot(dir string) bool {
	return SkipDir(filepath.Base(dir)) || IsNestedCheckout(dir)
}

// IsNestedCheckoutFS is [IsNestedCheckout] for a walk over an [fs.FS] rather
// than over native paths: name is a slash-separated path within fsys.
//
// It exists so that a walk which deliberately scoped itself with [os.Root]
// does not have to step outside that scope to apply this rule. The native form
// would take a path built by joining the root back on, and the stat would then
// resolve through whatever the filesystem says rather than through the root,
// which is the containment the caller chose [os.Root] to get. Asking fsys
// keeps the question inside it.
func IsNestedCheckoutFS(fsys fs.FS, name string) bool {
	_, err := fs.Stat(fsys, path.Join(name, gitMarker))
	return err == nil
}

// SkipDirBelowRootFS is [SkipDirBelowRoot] for a walk over an [fs.FS]: name is
// a slash-separated path within fsys, as [fs.WalkDir] hands it to the walk
// function.
//
// Unlike the native form it needs no care from the caller about the walk root,
// because [io/fs] spells the root "." and nothing else, so this can recognize
// it and answer false. That matters more than it looks: the repository root
// holds a .git of its own, so a walker that let the marker rule reach its own
// root would prune everything and report a clean tree. The native form cannot
// make that check, since there the root is whatever path the caller passed.
func SkipDirBelowRootFS(fsys fs.FS, name string) bool {
	if name == "." {
		return false
	}
	return SkipDir(path.Base(name)) || IsNestedCheckoutFS(fsys, name)
}
