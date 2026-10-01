package actionfixture

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRoot_FindsTheModuleAboveTheStart verifies the walk up to the module
// root, from the root itself and from a directory below it, and the error a
// start with no go.mod above it ends in.
func TestRoot_FindsTheModuleAboveTheStart(t *testing.T) {
	module := t.TempDir()
	if err := os.WriteFile(filepath.Join(module, "go.mod"), []byte("module example.com/fixture\n"), 0o600); err != nil {
		t.Fatalf("prepare the module: %v", err)
	}
	below := filepath.Join(module, "cmd", "tool")
	if err := os.MkdirAll(below, 0o750); err != nil {
		t.Fatalf("prepare the module: %v", err)
	}

	for name, start := range map[string]string{"the root": module, "a directory below it": below} {
		t.Run(name, func(t *testing.T) {
			if root, err := Root(start); err != nil || root != module {
				t.Errorf("Root(%s) = %q, %v; want %q", start, root, err, module)
			}
		})
	}
	// A walk that cannot climb from a directory with no go.mod is one that
	// may not stop at the filesystem root either, so the last case runs only
	// when the climb works.
	if t.Failed() {
		return
	}
	if root, err := Root(string(filepath.Separator)); !errors.Is(err, ErrNoModule) || root != "" {
		t.Errorf("Root(/) = %q, %v; want ErrNoModule", root, err)
	}
}

// TestOverlay_OneFilePerPackageWithItsBackticks verifies each source becomes
// one file in its own directory under Dir, with every placeholder made the
// backtick it stands for.
func TestOverlay_OneFilePerPackageWithItsBackticks(t *testing.T) {
	overlay := Overlay("/repo", map[string]string{"alpha": "package alpha\n\nconst q = @@query@@\n"})

	want := filepath.Join("/repo", filepath.FromSlash(Dir), "alpha", "alpha.go")
	content, ok := overlay[want]
	if len(overlay) != 1 || !ok {
		t.Fatalf("Overlay() = %v, want one file at %s", overlay, want)
	}
	if got := string(content); got != "package alpha\n\nconst q = `query`\n" || strings.Contains(got, Backtick) {
		t.Errorf("the overlay file reads %q, want the placeholders made backticks", got)
	}
}

// TestPattern_MatchesEveryPackageUnderDir verifies the loader pattern names
// the directory the overlay places the packages in, and everything below it.
func TestPattern_MatchesEveryPackageUnderDir(t *testing.T) {
	if want := "./" + Dir + "/..."; Pattern != want {
		t.Errorf("Pattern = %q, want %q", Pattern, want)
	}
}

// TestMain_IsTheResolutionSet verifies the package set the resolution tests
// load holds the four packages they read.
func TestMain_IsTheResolutionSet(t *testing.T) {
	sources := Main()
	for name, want := range map[string]string{"vuln": Vuln, "shapes": Shapes, "vars": Vars, "other": Other} {
		t.Run(name, func(t *testing.T) {
			if sources[name] != want {
				t.Errorf("Main()[%s] is not the %s fixture", name, name)
			}
		})
	}
	if len(sources) != 4 {
		t.Errorf("Main() holds %d packages, want 4", len(sources))
	}
}
