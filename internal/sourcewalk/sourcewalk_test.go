package sourcewalk

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// TestSkipDir_Names_PruneDotDirectories verifies the cheap half of the rule:
// a dot-directory is not this repository's source, the relative names "." and
// ".." are exempt because they name the tree the caller is already in, and an
// ordinary name that merely contains a dot is kept.
func TestSkipDir_Names_PruneDotDirectories(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{name: ".claude", want: true},
		{name: ".git", want: true},
		{name: ".github", want: true},
		{name: ".", want: false},
		{name: "..", want: false},
		{name: "internal", want: false},
		{name: "go.mod", want: false},
		{name: "node_modules", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SkipDir(tc.name); got != tc.want {
				t.Errorf("SkipDir(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestIsNestedCheckout_GitMarker_IdentifiesEitherShape verifies the marker
// half of the rule answers true for both shapes git writes at the root of a
// checkout, the directory a clone has and the file a linked worktree has, and
// false for a directory that holds neither.
func TestIsNestedCheckout_GitMarker_IdentifiesEitherShape(t *testing.T) {
	base := t.TempDir()
	// sequential: setup steps building one tree, asserted by the cases below
	clone := filepath.Join(base, "clone")
	worktree := filepath.Join(base, "worktree")
	plain := filepath.Join(base, "plain")
	for _, dir := range []string{clone, worktree, plain} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", dir, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(clone, ".git"), 0o750); err != nil {
		t.Fatalf("MkdirAll(clone/.git) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: /elsewhere\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(worktree/.git) error = %v", err)
	}

	cases := []struct {
		name string
		dir  string
		want bool
	}{
		{name: "clone carries a .git directory", dir: clone, want: true},
		{name: "worktree carries a .git file", dir: worktree, want: true},
		{name: "plain directory carries neither", dir: plain, want: false},
		{name: "absent directory", dir: filepath.Join(base, "gone"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsNestedCheckout(tc.dir); got != tc.want {
				t.Errorf("IsNestedCheckout(%q) = %v, want %v", tc.dir, got, tc.want)
			}
		})
	}
}

// TestIsNestedCheckout_UnstatableMarker_IsNotACheckout verifies the branch a
// filesystem the tests own never produces: an Lstat that fails for a reason
// other than absence answers false, because a walk that cannot stat a path
// has a failure of its own to report and this is not what reports it.
func TestIsNestedCheckout_UnstatableMarker_IsNotACheckout(t *testing.T) {
	original := lstat
	t.Cleanup(func() { lstat = original })
	lstat = func(string) (os.FileInfo, error) { return nil, errors.New("permission denied") }

	if IsNestedCheckout(t.TempDir()) {
		t.Error("IsNestedCheckout() = true for an unstatable marker, want false")
	}
}

// TestSkipDirBelowRoot_NestedWorktree_IsExcludedByEitherRule verifies the
// whole rule on the two shapes that matter. The worktree the parallel-agent
// tooling creates is caught by its path, .claude/worktrees, and one a
// developer put somewhere ordinary is caught by the .git marker alone, which
// is the case a rule naming .claude would have missed in silence.
func TestSkipDirBelowRoot_NestedWorktree_IsExcludedByEitherRule(t *testing.T) {
	base := t.TempDir()
	// sequential: setup steps building one tree, asserted by the cases below
	agentWorktree := filepath.Join(base, ".claude", "worktrees", "agent-1")
	namedWorktree := filepath.Join(base, "scratch")
	source := filepath.Join(base, "internal", "tools")
	for _, dir := range []string{agentWorktree, namedWorktree, source} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", dir, err)
		}
	}
	for _, dir := range []string{agentWorktree, namedWorktree} {
		if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: /elsewhere\n"), 0o600); err != nil {
			t.Fatalf("WriteFile(%s/.git) error = %v", dir, err)
		}
	}

	cases := []struct {
		name string
		path string
		want bool
	}{
		{name: "the tooling's worktrees directory", path: filepath.Join(base, ".claude"), want: true},
		{name: "one agent's worktree", path: agentWorktree, want: true},
		{name: "a worktree under an ordinary name", path: namedWorktree, want: true},
		{name: "this repository's own source", path: source, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SkipDirBelowRoot(tc.path); got != tc.want {
				t.Errorf("SkipDirBelowRoot(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

// TestSkipDirBelowRootFS_SlashPaths_ApplyBothRules verifies the [io/fs] form
// reaches the same verdicts as the native one over slash-separated names, on
// both shapes of the marker, and that a name merely containing ".git" is not
// one.
func TestSkipDirBelowRootFS_SlashPaths_ApplyBothRules(t *testing.T) {
	fsys := fstest.MapFS{
		"internal/tools/tools.go":                &fstest.MapFile{Data: []byte("package tools\n")},
		".claude/worktrees/agent-1/.git":         &fstest.MapFile{Data: []byte("gitdir: /elsewhere\n")},
		"scratch/.git":                           &fstest.MapFile{Data: []byte("gitdir: /elsewhere\n")},
		"vendored/.git/HEAD":                     &fstest.MapFile{Data: []byte("ref: refs/heads/main\n")},
		"testdata/gitignore-fixtures/fixture.go": &fstest.MapFile{Data: []byte("package fixture\n")},
	}

	cases := []struct {
		name string
		path string
		want bool
	}{
		{name: "the tooling's dot-directory", path: ".claude", want: true},
		{name: "one agent's worktree", path: ".claude/worktrees/agent-1", want: true},
		{name: "a worktree under an ordinary name", path: "scratch", want: true},
		{name: "a clone whose marker is a directory", path: "vendored", want: true},
		{name: "this repository's own source", path: "internal/tools", want: false},
		{name: "a name that merely contains git", path: "testdata/gitignore-fixtures", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SkipDirBelowRootFS(fsys, tc.path); got != tc.want {
				t.Errorf("SkipDirBelowRootFS(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

// TestSkipDirBelowRootFS_TheRoot_IsAlwaysEntered verifies the one thing the
// [io/fs] form can check that the native form cannot.
//
// [io/fs] spells the walk root "." and nothing else, so this form recognizes
// it. That is not a nicety: a walk is rooted at the repository root, the
// repository root holds a .git of its own, and a marker rule allowed to reach
// it would prune the entire tree and report a clean sweep. A caller that got
// this wrong would see every guardrail pass, which is the worst way to be
// wrong, so it is pinned here rather than left to each caller.
func TestSkipDirBelowRootFS_TheRoot_IsAlwaysEntered(t *testing.T) {
	root := fstest.MapFS{
		".git":          &fstest.MapFile{Data: []byte("gitdir: /elsewhere\n")},
		"internal/x.go": &fstest.MapFile{Data: []byte("package x\n")},
	}
	if SkipDirBelowRootFS(root, ".") {
		t.Error(`SkipDirBelowRootFS(fsys, ".") = true, want false: the walk root holds this repository's own .git, so pruning it walks nothing and reports a clean tree`)
	}
	if !IsNestedCheckoutFS(root, ".") {
		t.Error(`IsNestedCheckoutFS(fsys, ".") = false, want true: the exemption belongs to SkipDirBelowRootFS, not to the marker rule itself`)
	}
}

// TestIsNestedCheckoutFS_AnOSRoot_AnswersLikeTheNativeForm verifies the form
// against the filesystem its callers actually pass, an [os.Root]'s, rather
// than only against an in-memory one: the marker probe has to resolve inside
// the root the caller opened, which is the whole reason this form exists.
func TestIsNestedCheckoutFS_AnOSRoot_AnswersLikeTheNativeForm(t *testing.T) {
	base := t.TempDir()
	// sequential: setup steps building one tree, asserted by the cases below
	worktree := filepath.Join(base, "scratch")
	source := filepath.Join(base, "internal")
	for _, dir := range []string{worktree, source} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: /elsewhere\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(scratch/.git) error = %v", err)
	}
	opened, err := os.OpenRoot(base)
	if err != nil {
		t.Fatalf("OpenRoot(%s) error = %v", base, err)
	}
	t.Cleanup(func() { _ = opened.Close() })
	fsys := opened.FS()

	cases := []struct {
		name   string
		path   string
		native string
		want   bool
	}{
		{name: "a worktree", path: "scratch", native: worktree, want: true},
		{name: "ordinary source", path: "internal", native: source, want: false},
		{name: "a directory that is not there", path: "gone", native: filepath.Join(base, "gone"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsNestedCheckoutFS(fsys, tc.path)
			if got != tc.want {
				t.Errorf("IsNestedCheckoutFS(%q) = %v, want %v", tc.path, got, tc.want)
			}
			if native := IsNestedCheckout(tc.native); native != got {
				t.Errorf("IsNestedCheckoutFS(%q) = %v but IsNestedCheckout(%q) = %v: the two forms disagree", tc.path, got, tc.native, native)
			}
		})
	}
}
