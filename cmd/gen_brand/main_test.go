// main_test.go validates the brand-asset generator: every emitted SVG is
// well-formed, the shared geometry reaches each emitter, and the check
// mode detects both a missing and an edited asset.
package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/docgen"
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
// backgrounds decode as JPEG data: a corrupted embed would otherwise ship as
// a broken base64 payload that every renderer fails on silently. The two must
// also be different images, or every claim about which card carries which
// background holds vacuously — which card inlines which is asserted by
// TestAssets_EachPathCarriesTheSurfaceItIsFor.
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
	t.Run("the wide and tall grounds are two different images", func(t *testing.T) {
		if bytes.Equal(bgWide, bgTall) {
			t.Error("both embeds carry the same bytes, so no card can be told from another by its ground")
		}
	})
}

// short trims a want string to something a failure message can carry: the
// card backgrounds are hundreds of kilobytes of base64, and printing one
// would bury the line that says which file was wrong.
func short(s string) string {
	const limit = 48
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "…"
}

// TestAssets_EachPathCarriesTheSurfaceItIsFor pins the pairing the table
// makes between a published file and the art written into it. Every emitter
// produces the same well-formed fan-out, so a table that hands the favicon's
// art to the site logo satisfies every other test in this file: only naming
// what each file is for catches it. Each row states the thing that makes its
// surface that surface — the site mark is painted by CSS tokens alone, the
// favicon brings its own ground, the Go const is Go source at a 24-unit
// viewBox, and each card carries its own canvas and its own background art.
func TestAssets_EachPathCarriesTheSurfaceItIsFor(t *testing.T) {
	tests := []struct {
		path    string
		want    []string
		notWant []string
	}{
		{
			path:    filepath.Join("site", "src", "assets", "logo.svg"),
			want:    []string{`viewBox="0 0 64 64"`, `class="m-node"`, `class="m-branch"`, `class="m-tip"`},
			notWant: []string{"#", "currentColor"},
		},
		{
			path:    filepath.Join(brandDir, "logo-mono.svg"),
			want:    []string{`viewBox="0 0 64 64"`, `stroke="currentColor"`, `fill="currentColor"`},
			notWant: []string{"#", "class="},
		},
		{
			path: filepath.Join("site", "public", "favicon.svg"),
			want: []string{
				`viewBox="0 0 64 64"`,
				fmt.Sprintf(`rx="14" fill=%q`, darkGround),
				fmt.Sprintf(`fill=%q`, darkNode),
				fmt.Sprintf(`stroke=%q`, darkBranch),
				fmt.Sprintf(`fill=%q`, darkTip),
			},
			notWant: []string{"currentColor", "class="},
		},
		{
			path:    filepath.Join("internal", "toolutil", "brandmark_gen.go"),
			want:    []string{"package toolutil", "const svgBrand", `viewBox="0 0 24 24"`, "DO NOT EDIT"},
			notWant: []string{"#", "class="},
		},
		{
			path: filepath.Join(brandDir, "banner.svg"),
			want: []string{`viewBox="0 0 1280 400"`, bgImage(bgWide, 1280, 400), "GitLab MCP Server"},
		},
		{
			path: filepath.Join(brandDir, "og.svg"),
			want: []string{`viewBox="0 0 1200 630"`, bgImage(bgTall, 1200, 630), "GitLab MCP Server"},
		},
		{
			path: filepath.Join(brandDir, "social.svg"),
			want: []string{`viewBox="0 0 1280 640"`, bgImage(bgTall, 1280, 640), "GitLab MCP Server"},
		},
	}
	emitted := make(map[string]string, len(assets()))
	for _, a := range assets() {
		emitted[a.path] = a.content
	}
	if len(emitted) != len(tests) {
		t.Fatalf("assets() emits %d files, and %d are described here", len(emitted), len(tests))
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			content, ok := emitted[tt.path]
			if !ok {
				t.Fatalf("assets() emits no %s", tt.path)
			}
			for _, want := range tt.want {
				if !strings.Contains(content, want) {
					t.Errorf("%s does not carry %q", tt.path, short(want))
				}
			}
			for _, notWant := range tt.notWant {
				if strings.Contains(content, notWant) {
					t.Errorf("%s carries %q, which belongs to another surface", tt.path, notWant)
				}
			}
		})
	}
}

// TestBgImage_HrefAndDimensionsComeFromItsOwnArguments pins the three
// parameters against each other. Every caller passes a landscape canvas and a
// JPEG, so transposing the width and the height inside bgImage — or inlining
// the other embed — still yields a card that parses, still carries a
// background, and crops the art along the wrong axis.
func TestBgImage_HrefAndDimensionsComeFromItsOwnArguments(t *testing.T) {
	got := bgImage([]byte{0xff, 0xd8, 0xff}, 7, 9)
	for _, want := range []string{"data:image/jpeg;base64,/9j/", `width="7"`, `height="9"`} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(got, want) {
				t.Errorf("bgImage(art, 7, 9) = %q, want it to carry %q", got, want)
			}
		})
	}
}

// markElement is one drawn element of the fan-out, decoded from an emitted
// fragment so a test reads the attributes a renderer reads rather than the
// string the formatter happened to build.
type markElement struct {
	XMLName     xml.Name
	D           string `xml:"d,attr"`
	CX          string `xml:"cx,attr"`
	CY          string `xml:"cy,attr"`
	R           string `xml:"r,attr"`
	StrokeWidth string `xml:"stroke-width,attr"`
	Part        string `xml:"data-part,attr"`
}

// markElements decodes the drawn elements of a mark fragment in the order the
// emitter wrote them, unwrapping a whole <svg> document when it is given one.
func markElements(t *testing.T, fragment string) []markElement {
	t.Helper()
	if open := strings.Index(fragment, "<svg"); open >= 0 {
		body := fragment[open:]
		fragment = body[strings.Index(body, ">")+1 : strings.LastIndex(body, "</svg>")]
	}
	var doc struct {
		Elements []markElement `xml:",any"`
	}
	if err := xml.Unmarshal([]byte("<g>"+fragment+"</g>"), &doc); err != nil {
		t.Fatalf("mark fragment is not well-formed XML: %v", err)
	}
	return doc.Elements
}

// TestMarkBody_EachPartCarriesItsOwnPaintAndCoordinates pins which of the
// three attribute strings reaches which element and which constant reaches
// which coordinate. Nothing else can: the branches, the tips and the source
// node are all drawn from the same constants into the same document, so a
// body that paints the source node as a tip, or writes the tip's cy into its
// cx, emits the same element and attribute counts and parses the same way.
// The expected numbers are read back from the geometry constants rather than
// spelled out, because what is asserted here is the routing and not the
// design: moving a tip is a brand decision, painting it with the node's token
// is a defect.
func TestMarkBody_EachPartCarriesItsOwnPaintAndCoordinates(t *testing.T) {
	coord := func(v float64) string { return fmt.Sprintf("%g", v) }
	want := []markElement{
		{XMLName: xml.Name{Local: "path"}, Part: "branch", D: arcPath(-tipSpan), StrokeWidth: coord(branchWidth)},
		{XMLName: xml.Name{Local: "path"}, Part: "branch", D: arcPath(0), StrokeWidth: coord(branchWidth)},
		{XMLName: xml.Name{Local: "path"}, Part: "branch", D: arcPath(tipSpan), StrokeWidth: coord(branchWidth)},
		{XMLName: xml.Name{Local: "circle"}, Part: "tip", CX: coord(tipX), CY: coord(srcY - tipSpan), R: coord(tipR)},
		{XMLName: xml.Name{Local: "circle"}, Part: "tip", CX: coord(tipX), CY: coord(srcY), R: coord(tipR)},
		{XMLName: xml.Name{Local: "circle"}, Part: "tip", CX: coord(tipX), CY: coord(srcY + tipSpan), R: coord(tipR)},
		{XMLName: xml.Name{Local: "circle"}, Part: "node", CX: coord(srcX), CY: coord(srcY), R: coord(srcR)},
	}
	got := markElements(t, markBody(`data-part="node"`, `data-part="branch"`, `data-part="tip"`))
	if len(got) != len(want) {
		t.Fatalf("markBody drew %d elements, want %d (three branches, three tips, one source node)", len(got), len(want))
	}
	for i, wantElement := range want {
		t.Run(fmt.Sprintf("%s %d", wantElement.Part, i), func(t *testing.T) {
			if got[i].XMLName.Local != wantElement.XMLName.Local {
				t.Fatalf("element %d is a <%s>, want a <%s>", i, got[i].XMLName.Local, wantElement.XMLName.Local)
			}
			got[i].XMLName = wantElement.XMLName
			if got[i] != wantElement {
				t.Errorf("element %d = %+v, want %+v", i, got[i], wantElement)
			}
		})
	}
}

// TestEmitters_EachEmbedsTheSharedMarkWithItsOwnTokens verifies every emitter
// renders the shared geometry rather than a copy of it, and hands it the
// attribute triple its own surface calls for. The favicon and the three cards
// take the same dark triple, so an emitter that swapped the source node's
// token for the tips' would still carry all three colors and still parse.
func TestEmitters_EachEmbedsTheSharedMarkWithItsOwnTokens(t *testing.T) {
	dark := markBody(
		fmt.Sprintf("fill=%q", darkNode),
		fmt.Sprintf("stroke=%q", darkBranch),
		fmt.Sprintf("fill=%q", darkTip),
	)
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"canonical", canonicalSVG(), markBody(`class="m-node"`, `class="m-branch"`, `class="m-tip"`)},
		{"mono", monoSVG(), markBody(`fill="currentColor"`, `stroke="currentColor"`, `fill="currentColor"`)},
		{"favicon", faviconSVG(), dark},
		{"banner", bannerSVG(), dark},
		{"og", ogSVG(), dark},
		{"social", socialSVG(), dark},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.got, tt.want) {
				t.Errorf("the %s emitter does not carry the shared mark painted with its own tokens:\nwant %q", tt.name, short(tt.want))
			}
		})
	}
}

// scaledCoordinate renders one canonical 64-unit coordinate the way the
// 24-unit mark renders it: through the same ratio and the same three
// significant digits. An attribute the element does not carry stays absent.
func scaledCoordinate(t *testing.T, canonicalValue string) string {
	t.Helper()
	if canonicalValue == "" {
		return ""
	}
	v, err := strconv.ParseFloat(canonicalValue, 64)
	if err != nil {
		t.Fatalf("canonical attribute %q is not a number: %v", canonicalValue, err)
	}
	return fmt.Sprintf("%.3g", v*24.0/canvas)
}

// scaledPath rewrites every number of a path's d attribute through
// scaledCoordinate, leaving the command letters and the separators where the
// canonical path put them.
func scaledPath(t *testing.T, d string) string {
	t.Helper()
	tokens := strings.Split(d, " ")
	for i, token := range tokens {
		number := strings.TrimSuffix(token, ",")
		if _, err := strconv.ParseFloat(number, 64); err != nil {
			continue
		}
		tokens[i] = scaledCoordinate(t, number) + strings.TrimPrefix(token, number)
	}
	return strings.Join(tokens, " ")
}

// TestBrandMark24_IsTheCanonicalGeometryScaled holds the in-binary mark to
// the claim its doc comment makes, that it is the canonical fan-out scaled
// rather than a second drawing of it. It is in fact a second drawing —
// brandMark24 re-derives the Bézier control points instead of calling
// arcPath — so a curve edited in one place and not the other would ship a
// site logo and an MCP client icon that no longer match, with every other
// test in this file green. Comparing element by element against the canonical
// geometry put through the same ratio is what makes the divergence fail.
func TestBrandMark24_IsTheCanonicalGeometryScaled(t *testing.T) {
	canonical := markElements(t, markBody("", "", ""))
	small := markElements(t, brandMark24())
	if len(small) != len(canonical) {
		t.Fatalf("the 24-unit mark draws %d elements, the canonical one draws %d", len(small), len(canonical))
	}
	for i, canonicalElement := range canonical {
		t.Run(fmt.Sprintf("%s %d", canonicalElement.XMLName.Local, i), func(t *testing.T) {
			want := markElement{
				XMLName:     canonicalElement.XMLName,
				D:           scaledPath(t, canonicalElement.D),
				CX:          scaledCoordinate(t, canonicalElement.CX),
				CY:          scaledCoordinate(t, canonicalElement.CY),
				R:           scaledCoordinate(t, canonicalElement.R),
				StrokeWidth: scaledCoordinate(t, canonicalElement.StrokeWidth),
			}
			got := small[i]
			if got.XMLName.Local != want.XMLName.Local {
				t.Fatalf("element %d is a <%s>, want a <%s>", i, got.XMLName.Local, want.XMLName.Local)
			}
			got.XMLName = want.XMLName
			if got != want {
				t.Errorf("element %d = %+v, want %+v (the canonical %+v scaled)", i, got, want, canonicalElement)
			}
		})
	}
}

// TestRun_Check_NamesEveryStaleAsset verifies the check pass reports all the
// assets that drifted rather than the first one it meets. The exit code alone
// is the same either way, so a run that stopped at the first would send a
// maintainer back for one more run per file while both gates and the Makefile
// read exactly the same.
func TestRun_Check_NamesEveryStaleAsset(t *testing.T) {
	reported, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatalf("create the stderr capture: %v", err)
	}
	original := os.Stderr
	t.Cleanup(func() { os.Stderr = original })
	os.Stderr = reported

	runErr := run(t.TempDir(), true)

	os.Stderr = original
	if closeErr := reported.Close(); closeErr != nil {
		t.Fatalf("close the stderr capture: %v", closeErr)
	}
	if runErr == nil {
		t.Fatal("run(check) on an empty tree = nil, want a staleness error")
	}
	lines, err := os.ReadFile(reported.Name())
	if err != nil {
		t.Fatalf("read the stderr capture: %v", err)
	}
	for _, a := range assets() {
		if !strings.Contains(string(lines), a.path) {
			t.Errorf("the check pass never names %s as stale:\n%s", a.path, lines)
		}
	}
}

// TestRun_Write_UsesTheSharedGeneratedFileMode verifies the generator creates
// its artifacts with the mode every generated artifact here carries, which is
// docgen's decision rather than this command's. Nothing else observes the
// mode: an asset written world-writable is byte-identical to one written the
// way the convention says, so check passes and the tree stays wrong.
func TestRun_Write_UsesTheSharedGeneratedFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reports 0666/0444 rather than the unix mode the file was created with")
	}
	root := t.TempDir()
	if err := run(root, false); err != nil {
		t.Fatalf("run(write) error: %v", err)
	}
	for _, a := range assets() {
		t.Run(a.path, func(t *testing.T) {
			info, err := os.Stat(filepath.Join(root, a.path))
			if err != nil {
				t.Fatalf("stat the written asset: %v", err)
			}
			if got := info.Mode().Perm(); got != docgen.GeneratedFileMode {
				t.Errorf("%s was written %04o, want %04o", a.path, got, docgen.GeneratedFileMode)
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

// chdirIntoNestedModule makes a tree holding two go.mod files, one above the
// other, the working directory a subdirectory of the inner one, and returns
// the inner root — the nearest module, and so the root a generator invoked
// there must write into.
func chdirIntoNestedModule(t *testing.T) string {
	t.Helper()
	outer := t.TempDir()
	if err := os.WriteFile(filepath.Join(outer, "go.mod"), []byte("module outer\n"), 0o600); err != nil {
		t.Fatalf("write the outer go.mod: %v", err)
	}
	inner := filepath.Join(outer, "inner")
	nested := filepath.Join(inner, "cmd", "gen_brand")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.WriteFile(filepath.Join(inner, "go.mod"), []byte("module inner\n"), 0o600); err != nil {
		t.Fatalf("write the inner go.mod: %v", err)
	}
	t.Chdir(nested)
	return inner
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
//
// Windows refuses to remove a process's working directory, and macOS keeps
// answering getcwd from the path it remembers, so on neither can the failure
// be produced this way: both skip rather than report the operating system's
// design as a defect here.
func chdirIntoRemovedDir(t *testing.T) {
	t.Helper()
	gone := filepath.Join(t.TempDir(), "gone")
	if err := os.Mkdir(gone, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Chdir(gone)
	if err := os.RemoveAll(gone); err != nil {
		t.Skipf("this platform will not remove the working directory: %v", err)
	}
	if _, err := os.Getwd(); err == nil {
		t.Skip("this platform's getcwd still answers after the working directory is removed")
	}
}

// assetsExist reports whether every asset the generator emits is present
// under root with exactly the bytes the geometry produces.
func assetsExist(t *testing.T, root string) bool {
	t.Helper()
	for _, a := range assets() {
		got, err := os.ReadFile(filepath.Join(root, a.path))
		if err != nil || string(got) != a.content {
			return false
		}
	}
	return true
}

// TestCLI_Scenarios_ReturnsTheExitCodeAndSaysWhy verifies the command layer
// main is a shim over: the default run writes every asset, -check verifies
// without writing, a tree with no go.mod and a tree the write fails in are
// each reported on stderr and exit 1, and a flag the command does not define
// exits 2 while -h exits 0.
func TestCLI_Scenarios_ReturnsTheExitCodeAndSaysWhy(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		prepare  func(t *testing.T) (root string)
		wantCode int
		wantErr  string
		// wantWritten is the state of the tree the run leaves behind:
		// true when every asset must be on disk, false when none may be.
		wantWritten bool
	}{
		{
			name:        "default run writes every asset",
			prepare:     chdirIntoFixtureRepo,
			wantWritten: true,
		},
		{
			name:    "check on an empty tree reports staleness and writes nothing",
			args:    []string{"-check"},
			prepare: chdirIntoFixtureRepo,
			// The exit code is what a Makefile gate reads, and writing
			// under -check would make the next check pass vacuously.
			wantCode: 1,
			wantErr:  "brand assets are stale",
		},
		{
			name:     "no go.mod above the working directory",
			prepare:  func(t *testing.T) string { t.Helper(); chdirIntoRootlessDir(t); return "" },
			wantCode: 1,
			wantErr:  "no go.mod found above",
		},
		{
			name: "a write that fails is reported",
			prepare: func(t *testing.T) string {
				t.Helper()
				root := chdirIntoFixtureRepo(t)
				top, _, _ := strings.Cut(assets()[0].path, string(filepath.Separator))
				if err := os.WriteFile(filepath.Join(root, top), []byte("x"), 0o600); err != nil {
					t.Fatalf("write blocker: %v", err)
				}
				return root
			},
			wantCode: 1,
			wantErr:  "create ",
		},
		{
			name:     "a flag the command does not define",
			args:     []string{"-nope"},
			prepare:  chdirIntoFixtureRepo,
			wantCode: 2,
			wantErr:  "not defined: -nope",
		},
		{
			name:    "-h prints the usage it accepts",
			args:    []string{"-h"},
			prepare: chdirIntoFixtureRepo,
			wantErr: "-check",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tt.prepare(t)
			var stderr bytes.Buffer

			code := cli(tt.args, &stderr)

			if code != tt.wantCode {
				t.Errorf("cli(%q) = %d, want %d\nstderr: %s", tt.args, code, tt.wantCode, stderr.String())
			}
			if tt.wantErr != "" && !strings.Contains(stderr.String(), tt.wantErr) {
				t.Errorf("cli(%q) stderr = %q, want containing %q", tt.args, stderr.String(), tt.wantErr)
			}
			if root == "" {
				return
			}
			if written := assetsExist(t, root); written != tt.wantWritten {
				t.Errorf("assets present under the root = %t, want %t", written, tt.wantWritten)
			}
		})
	}
}

// TestMain_CheckFlag_ExitsThroughTheSeam checks the one line main carries, so
// the command's entry point is exercised rather than assumed. -check is the
// flag to drive it with: main forwarding os.Args instead of os.Args[1:] would
// leave the flag set reading the binary name as an operand, the run would
// write instead of verify, and the exit code would be 0 rather than the 1 an
// empty tree earns.
func TestMain_CheckFlag_ExitsThroughTheSeam(t *testing.T) {
	root := chdirIntoFixtureRepo(t)
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer devNull.Close()
	originalArgs, originalExit, originalErr := os.Args, exit, os.Stderr
	t.Cleanup(func() { os.Args, exit, os.Stderr = originalArgs, originalExit, originalErr })
	code := -1
	exit = func(got int) { code = got }
	os.Args = []string{"gen_brand", "-check"}
	os.Stderr = devNull

	main()

	if code != 1 {
		t.Errorf("main exited %d, want 1 for a tree with no assets in it", code)
	}
	if assetsExist(t, root) {
		t.Error("main wrote the assets under -check, which must only verify them")
	}
}

// TestRepoRoot_Scenarios_WalksUpToGoMod verifies the root lookup returns the
// nearest ancestor holding go.mod, fails when no ancestor holds one, and
// fails when the working directory itself cannot be read because it was
// removed from under the process.
//
// The nested case is what separates "the nearest module" from "some module
// above here": with one go.mod in the fixture the two answers are the same
// file, so a walk that kept going and returned the outermost one would pass
// while writing every asset into whatever repository happens to contain the
// checkout.
func TestRepoRoot_Scenarios_WalksUpToGoMod(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T) (want string)
		wantErr string
	}{
		{name: "go.mod in an ancestor", prepare: chdirIntoFixtureRepo},
		{name: "the nearest go.mod wins over one above it", prepare: chdirIntoNestedModule},
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
