// main_test.go validates the brand-asset generator: every emitted SVG is
// well-formed, the shared geometry reaches each emitter, and the check
// mode detects both a missing and an edited asset.
package main

import (
	"bytes"
	"encoding/xml"
	"image/jpeg"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestAssets_EverySVGIsWellFormedXML verifies each emitted SVG (and the
// SVG inside the generated Go const) parses as XML.
func TestAssets_EverySVGIsWellFormedXML(t *testing.T) {
	for _, a := range assets() {
		t.Run(a.path, func(t *testing.T) {
			content := a.content
			if strings.HasSuffix(a.path, ".go") {
				start := strings.Index(content, "`<svg")
				end := strings.LastIndex(content, "</svg>`")
				if start < 0 || end < 0 {
					t.Fatal("generated Go source carries no raw-string SVG constant")
				}
				content = content[start+1 : end+len("</svg>")]
			}
			var node struct {
				XMLName xml.Name `xml:"svg"`
			}
			if err := xml.Unmarshal([]byte(content), &node); err != nil {
				t.Fatalf("emitted SVG is not well-formed XML: %v", err)
			}
		})
	}
}

// TestAssets_SharedGeometryReachesEveryEmitter verifies each variant
// renders the full fan-out: three branches, three tips, one source node.
// The multiple-of-three form is kept so a backdrop that re-emits the
// branch trio (as the retired procedural echoes did, and a future one
// may again) stays legal; the circle count stays pinned at four
// because no backdrop carries nodes.
func TestAssets_SharedGeometryReachesEveryEmitter(t *testing.T) {
	for _, a := range assets() {
		t.Run(a.path, func(t *testing.T) {
			paths := strings.Count(a.content, "<path")
			circles := strings.Count(a.content, "<circle")
			if paths < 3 || paths%3 != 0 {
				t.Errorf("emitted %d branch paths, want a positive multiple of 3 (branch trios)", paths)
			}
			if circles != 4 {
				t.Errorf("emitted %d circles, want 4 (three tips + source node)", circles)
			}
		})
	}
}

// TestAssets_CanonicalCarriesNoColor verifies the site mark stays paintable
// by CSS alone: classes only, no fill or stroke color literals.
func TestAssets_CanonicalCarriesNoColor(t *testing.T) {
	canonical := canonicalSVG()
	for _, forbidden := range []string{"#", "currentColor"} {
		t.Run(forbidden, func(t *testing.T) {
			if strings.Contains(canonical, forbidden) {
				t.Errorf("canonical mark contains %q; it must be painted by site CSS tokens only", forbidden)
			}
		})
	}
	for _, class := range []string{"m-node", "m-branch", "m-tip"} {
		t.Run(class, func(t *testing.T) {
			if !strings.Contains(canonical, class) {
				t.Errorf("canonical mark is missing the %q class", class)
			}
		})
	}
}

// TestRun_WriteThenCheck_RoundTrips verifies a fresh write passes check,
// and an edited asset fails it.
func TestRun_WriteThenCheck_RoundTrips(t *testing.T) {
	root := t.TempDir()
	if err := run(root, false); err != nil {
		t.Fatalf("run(write) error: %v", err)
	}
	if err := run(root, true); err != nil {
		t.Fatalf("run(check) right after write = %v, want nil", err)
	}
	victim := filepath.Join(root, assets()[0].path)
	if err := os.WriteFile(victim, []byte("edited"), 0o600); err != nil {
		t.Fatalf("edit asset: %v", err)
	}
	if err := run(root, true); err == nil {
		t.Fatal("run(check) after an edit = nil, want a staleness error")
	}
}

// TestEmbeddedBackgrounds_AreValidJPEGs verifies the frozen card
// backgrounds decode as JPEG data and actually reach the emitted cards:
// a corrupted embed would otherwise ship as a broken base64 payload that
// every renderer fails on silently.
func TestEmbeddedBackgrounds_AreValidJPEGs(t *testing.T) {
	for name, art := range map[string][]byte{"bg-wide": bgWide, "bg-tall": bgTall} {
		t.Run(name, func(t *testing.T) {
			if len(art) < 4 || art[0] != 0xff || art[1] != 0xd8 {
				t.Errorf("%s does not start with the JPEG SOI marker", name)
			}
			if _, err := jpeg.DecodeConfig(bytes.NewReader(art)); err != nil {
				t.Errorf("%s does not decode as JPEG: %v", name, err)
			}
		})
	}
	for _, card := range []string{bannerSVG(), ogSVG(), socialSVG()} {
		t.Run(card, func(t *testing.T) {
			if !strings.Contains(card, "data:image/jpeg;base64,") {
				t.Error("a card is missing its embedded background layer")
			}
		})
	}
}

// TestRun_Check_MissingAssetFails verifies check reports assets that were
// never generated instead of passing vacuously.
func TestRun_Check_MissingAssetFails(t *testing.T) {
	if err := run(t.TempDir(), true); err == nil {
		t.Fatal("run(check) on an empty tree = nil, want a staleness error")
	}
}

// TestRun_Write_UnwritableTree_ReturnsError verifies a write run reports the
// filesystem failure it hits instead of continuing with the next asset: a
// regular file where an asset's directory must be created, and a directory
// where the asset file itself must be written. Neither depends on permission
// bits, so both reproduce as root.
func TestRun_Write_UnwritableTree_ReturnsError(t *testing.T) {
	first := assets()[0].path
	tests := []struct {
		name    string
		block   func(t *testing.T, root string)
		wantErr []string
	}{
		{
			name: "asset directory is a file",
			block: func(t *testing.T, root string) {
				t.Helper()
				top, _, _ := strings.Cut(first, string(filepath.Separator))
				if err := os.WriteFile(filepath.Join(root, top), []byte("x"), 0o600); err != nil {
					t.Fatalf("write blocker: %v", err)
				}
			},
			wantErr: []string{"create ", filepath.Dir(first), notADirectoryText()},
		},
		{
			name: "asset path is a directory",
			block: func(t *testing.T, root string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(root, first), 0o750); err != nil {
					t.Fatalf("mkdir blocker: %v", err)
				}
			},
			wantErr: []string{"write " + first, "is a directory"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.block(t, root)
			err := run(root, false)
			if err == nil {
				t.Fatal("run(write) error = nil, want a filesystem failure")
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("run(write) error = %v, want containing %q", err, want)
				}
			}
		})
	}
}

// chdirIntoFixtureRepo makes a temporary directory holding go.mod the
// working directory (from a nested subdirectory) and returns that root, so
// repoRoot's walk has an ancestor to find.
func chdirIntoFixtureRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	nested := filepath.Join(root, "cmd", "gen_brand")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	t.Chdir(nested)
	return root
}

// chdirIntoRootlessDir makes a temporary directory with no go.mod anywhere
// above it the working directory, skipping the test when the sandbox happens
// to hold a go.mod in an ancestor.
func chdirIntoRootlessDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	for ancestor := dir; ; ancestor = filepath.Dir(ancestor) {
		if _, err := os.Stat(filepath.Join(ancestor, "go.mod")); err == nil {
			t.Skipf("%s holds a go.mod above the temp dir", ancestor)
		}
		if filepath.Dir(ancestor) == ancestor {
			break
		}
	}
	t.Chdir(dir)
}

// notADirectoryText is the operating system's wording for a directory being
// created below a regular file: ENOTDIR on unix, and on Windows the
// path-not-found error that stands in for it.
func notADirectoryText() string {
	if runtime.GOOS == "windows" {
		return "cannot find the path specified"
	}
	return "not a directory"
}

// chdirIntoRemovedDir makes a directory the working directory and then
// removes it, so os.Getwd fails with ENOENT regardless of privilege.
func chdirIntoRemovedDir(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows getcwd does not fail when the current directory is removed")
	}
	gone := filepath.Join(t.TempDir(), "gone")
	if err := os.Mkdir(gone, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Chdir(gone)
	if err := os.RemoveAll(gone); err != nil {
		t.Fatalf("remove working directory: %v", err)
	}
}

// TestRepoRoot_Scenarios_WalksUpToGoMod verifies the root lookup returns the
// nearest ancestor holding go.mod, fails when no ancestor holds one, and
// fails when the working directory itself cannot be read because it was
// removed from under the process.
func TestRepoRoot_Scenarios_WalksUpToGoMod(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T) (want string)
		wantErr string
	}{
		{name: "go.mod in an ancestor", prepare: chdirIntoFixtureRepo},
		{
			name:    "no go.mod above the working directory",
			prepare: func(t *testing.T) string { t.Helper(); chdirIntoRootlessDir(t); return "" },
			wantErr: "no go.mod found above",
		},
		{
			name:    "working directory removed",
			prepare: func(t *testing.T) string { t.Helper(); chdirIntoRemovedDir(t); return "" },
			wantErr: "get working directory",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.prepare(t)
			got, err := repoRoot()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("repoRoot() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("repoRoot() error = %v", err)
			}
			wantResolved, _ := filepath.EvalSymlinks(want)
			gotResolved, _ := filepath.EvalSymlinks(got)
			if gotResolved != wantResolved {
				t.Errorf("repoRoot() = %q, want %q", gotResolved, wantResolved)
			}
		})
	}
}
