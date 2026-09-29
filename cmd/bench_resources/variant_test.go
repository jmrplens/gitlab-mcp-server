package main

import (
	"context"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// slotsVariant is a variant of the throwaway module variantModule writes.
var slotsVariant = buildVariant{
	File:        "internal/limits/limits.go",
	Declaration: "const slots = ",
	Replacement: "const slots = 1 << 20",
}

// variantModule writes a throwaway module whose cmd/server prints the
// constant slotsVariant replaces, and returns its root.
func variantModule(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeModuleFile(t, filepath.Join(root, "go.mod"), "module variant.example/server\n\ngo 1.22\n")
	writeModuleFile(t, filepath.Join(root, "internal", "limits", "limits.go"),
		"package limits\n\n// Slots is the ceiling.\nconst Slots = slots\n\nconst slots = 16 // the one line a variant replaces\n")
	writeModuleFile(t, filepath.Join(root, "cmd", "server", "main.go"),
		"package main\n\nimport (\n\t\"fmt\"\n\n\t\"variant.example/server/internal/limits\"\n)\n\nfunc main() { fmt.Println(limits.Slots) }\n")
	return root
}

// TestBuildVariant_Rewrite_ReplacesTheOneLineItNames verifies a variant
// replaces exactly the declaration it names, and refuses a file where that
// declaration is gone or ambiguous rather than building something else.
//
// A variant that replaced nothing would run the binary under test in both
// arms, which reads as a bound that changes nothing; so the refusals are the
// half of this that matters.
func TestBuildVariant_Rewrite_ReplacesTheOneLineItNames(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		want    string
		wantErr string
	}{
		{
			name: "an indented declaration, its line ending kept",
			src:  "package x\n\n\tconst slots = 16 // why\nconst other = 1\n",
			want: "package x\n\nconst slots = 1 << 20\nconst other = 1\n",
		},
		{
			name: "the file's last line, with no ending to keep",
			src:  "package x\nconst slots = 16",
			want: "package x\nconst slots = 1 << 20",
		},
		{name: "a declaration that is gone", src: "package x\nconst other = 16\n", wantErr: "no longer declares"},
		{name: "a declaration made twice", src: "const slots = 1\nconst slots = 2\n", wantErr: "more than one line"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := slotsVariant.rewrite([]byte(tc.src))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || !strings.Contains(err.Error(), slotsVariant.File) {
					t.Errorf("rewrite = %v, want an error about %q naming the file", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("rewrite: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("rewrite = %q, want %q", got, tc.want)
			}
		})
	}
	if got := slotsVariant.describe(); got != "internal/limits/limits.go: const slots = 1 << 20" {
		t.Errorf("describe = %q, want the file and the line put in", got)
	}
}

// TestBuildVariantServer_BuildsTheOverlayAndLeavesTheTreeAlone builds a
// throwaway module through the variant and runs what it built.
//
// Running it is the point: a build that silently ignored the overlay would
// still produce a binary, and it would print the constant the tree holds.
func TestBuildVariantServer_BuildsTheOverlayAndLeavesTheTreeAlone(t *testing.T) {
	root := variantModule(t)
	source := filepath.Join(root, filepath.FromSlash(slotsVariant.File))
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var built string
	var buildErr error
	captureStdout(t, func() { built, buildErr = buildVariantServer(root, slotsVariant) })
	if buildErr != nil {
		t.Fatalf("buildVariantServer: %v", buildErr)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(built)) })

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	out, runErr := exec.CommandContext(ctx, built).Output() // #nosec G204 -- this test's own build
	if runErr != nil {
		t.Fatalf("run the variant: %v", runErr)
	}
	if got := strings.TrimSpace(string(out)); got != "1048576" {
		t.Errorf("the variant printed %q, want the replaced constant 1048576", got)
	}
	after, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(after) != string(before) {
		t.Error("building the variant rewrote the tree on disk")
	}
}

// TestBuildVariantServer_ReportsWhatItCouldNotDo verifies every way a variant
// build fails is named, and that a failed build leaves no directory behind.
func TestBuildVariantServer_ReportsWhatItCouldNotDo(t *testing.T) {
	t.Run("no directory to build in", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing")
		for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
			t.Setenv(name, missing)
		}
		if _, err := buildVariantServer(variantModule(t), slotsVariant); err == nil || !strings.Contains(err.Error(), "build directory") {
			t.Errorf("buildVariantServer = %v, want the build directory failure", err)
		}
	})
	t.Run("a declaration the tree no longer holds", func(t *testing.T) {
		gone := slotsVariant
		gone.Declaration = "const gone = "
		if _, err := buildVariantServer(variantModule(t), gone); err == nil || !strings.Contains(err.Error(), "no longer declares") {
			t.Errorf("buildVariantServer = %v, want the missing declaration named", err)
		}
	})
	t.Run("a replacement that does not compile", func(t *testing.T) {
		broken := slotsVariant
		broken.Replacement = "const slots = "
		var err error
		captureStdout(t, func() { _, err = buildVariantServer(variantModule(t), broken) })
		if err == nil || !strings.Contains(err.Error(), "build ./cmd/server") {
			t.Errorf("buildVariantServer = %v, want the compiler's refusal", err)
		}
	})
}

// TestBuildVariant_Overlay_ReportsWhatItCouldNotWrite verifies the overlay's
// own failures: a file it cannot read, and a directory it cannot write the
// rewritten file or the overlay document into.
func TestBuildVariant_Overlay_ReportsWhatItCouldNotWrite(t *testing.T) {
	root := variantModule(t)
	t.Run("a file that is not there", func(t *testing.T) {
		missing := slotsVariant
		missing.File = "internal/limits/absent.go"
		if _, err := missing.overlay(root, t.TempDir()); err == nil || !strings.Contains(err.Error(), "read the file") {
			t.Errorf("overlay = %v, want the read failure", err)
		}
	})
	t.Run("a directory that is not there", func(t *testing.T) {
		if _, err := slotsVariant.overlay(root, filepath.Join(t.TempDir(), "absent")); err == nil ||
			!strings.Contains(err.Error(), "write the variant's source") {
			t.Errorf("overlay = %v, want the write failure", err)
		}
	})
	t.Run("an overlay document that cannot be written", func(t *testing.T) {
		dir := t.TempDir()
		// A directory where the document belongs: the source is written beside
		// it, and the document's write is the one that fails.
		if err := os.Mkdir(filepath.Join(dir, "overlay.json"), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if _, err := slotsVariant.overlay(root, dir); err == nil || !strings.Contains(err.Error(), "overlay.json") {
			t.Errorf("overlay = %v, want the document's write failure", err)
		}
	})
}

// TestFairnessBounds_EveryVariantReplacesOneDeclarationThatCompiles holds each
// bound's variant to the tree: its declaration is on exactly one line of the
// real file, and the package holding that file compiles with the line
// replaced.
//
// This is what catches a variant going stale in CI rather than at the start of
// a measurement: the oauth-verification bound is built by replacing a line of
// internal/oauth/verifier.go, and a refactor there that moved or renamed the
// declaration would otherwise be found by whoever next ran the benchmark.
func TestFairnessBounds_EveryVariantReplacesOneDeclarationThatCompiles(t *testing.T) {
	root, err := projectRootForTest()
	if err != nil {
		t.Skipf("not running inside the module: %v", err)
	}
	for _, bound := range fairnessBounds {
		if bound.Variant == nil {
			continue
		}
		t.Run(bound.ID, func(t *testing.T) {
			if bound.Refusals[0].TextPrefix == "" {
				t.Error("a bound measured against a variant has no refusal of its own to count")
			}
			document, overlayErr := bound.Variant.overlay(root, t.TempDir())
			if overlayErr != nil {
				t.Fatalf("overlay: %v", overlayErr)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
			defer cancel()
			pkg := "./" + path.Dir(bound.Variant.File)
			cmd := exec.CommandContext(ctx, "go", "build", "-overlay", document, "-o", os.DevNull, pkg) // #nosec G204 -- the go command on this checkout
			cmd.Dir = root
			if output, buildErr := cmd.CombinedOutput(); buildErr != nil {
				t.Errorf("%s does not compile with %s: %v\n%s", pkg, bound.Variant.describe(), buildErr, output)
			}
		})
	}
}
