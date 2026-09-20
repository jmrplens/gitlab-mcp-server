package sourcewalk

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
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
