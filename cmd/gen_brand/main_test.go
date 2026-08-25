// main_test.go validates the brand-asset generator: every emitted SVG is
// well-formed, the shared geometry reaches each emitter, and the check
// mode detects both a missing and an edited asset.
package main

import (
	"encoding/xml"
	"os"
	"path/filepath"
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
// The banner and OG card additionally echo the branch trio as their
// backdrop, so their path count is a larger multiple of three; every
// asset keeps exactly four circles, because the echoes deliberately
// carry no nodes.
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
		if strings.Contains(canonical, forbidden) {
			t.Errorf("canonical mark contains %q; it must be painted by site CSS tokens only", forbidden)
		}
	}
	for _, class := range []string{"m-node", "m-branch", "m-tip"} {
		if !strings.Contains(canonical, class) {
			t.Errorf("canonical mark is missing the %q class", class)
		}
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

// TestRun_Check_MissingAssetFails verifies check reports assets that were
// never generated instead of passing vacuously.
func TestRun_Check_MissingAssetFails(t *testing.T) {
	if err := run(t.TempDir(), true); err == nil {
		t.Fatal("run(check) on an empty tree = nil, want a staleness error")
	}
}
