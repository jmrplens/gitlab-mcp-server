// render_test.go covers the artifacts a record turns into: the chart files,
// the generated block in the Markdown reference and in each language of the
// site, and the -check mode that gates all of them.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/mcpsurface"
)

// projectRootForTest resolves the module root for tests that read committed
// files.
func projectRootForTest() (string, error) { return mcpsurface.ProjectRoot() }

// TestWriteCharts_WritesAPairPerFigure verifies every figure is written in
// both schemes, named so the Markdown page's <picture> can find them, and that
// the two schemes really differ.
func TestWriteCharts_WritesAPairPerFigure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "charts")
	palettes := map[string]palette{schemeLight: testPalette(), schemeDark: darkTestPalette()}

	written, err := writeCharts(dir, sampleRun(), englishLabels(), palettes, false)
	if err != nil {
		t.Fatalf("writeCharts: %v", err)
	}
	figures := buildFigures(sampleRun(), englishLabels())
	if len(written) != 2*len(figures) {
		t.Errorf("wrote %d files for %d figures, want a light and a dark one each", len(written), len(figures))
	}
	for _, fig := range figures {
		light := readFileForTest(t, filepath.Join(dir, fig.Name+".light.svg"))
		dark := readFileForTest(t, filepath.Join(dir, fig.Name+".dark.svg"))
		if light == dark {
			t.Errorf("%s renders identically in both schemes", fig.Name)
		}
		if !strings.HasPrefix(light, "<svg") {
			t.Errorf("%s is not an SVG document", fig.Name)
		}
	}
}

// TestWriteCharts_Check_ReportsStaleWithoutWriting verifies -check tells the
// truth in both directions and never touches the files, which is what makes it
// safe to run in CI.
func TestWriteCharts_Check_ReportsStaleWithoutWriting(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "charts")
	palettes := map[string]palette{schemeLight: testPalette(), schemeDark: darkTestPalette()}
	run := sampleRun()

	if _, err := writeCharts(dir, run, englishLabels(), palettes, false); err != nil {
		t.Fatalf("writeCharts: %v", err)
	}
	stale, err := writeCharts(dir, run, englishLabels(), palettes, true)
	if err != nil {
		t.Fatalf("writeCharts -check: %v", err)
	}
	if len(stale) != 0 {
		t.Errorf("-check reported %d stale files right after writing them: %v", len(stale), stale)
	}

	// A measurement that moved must be reported as stale.
	changed := sampleRun()
	changed.Scenarios[0].Memory.OneClientMiB = 999
	stale, err = writeCharts(dir, changed, englishLabels(), palettes, true)
	if err != nil {
		t.Fatalf("writeCharts -check: %v", err)
	}
	if len(stale) == 0 {
		t.Error("-check reported nothing stale after the numbers changed")
	}
	// The moved value would appear as a bar label; the palette's own hex codes
	// contain digits, so the assertion looks for the label rather than the
	// number anywhere in the document.
	if got := readFileForTest(t, filepath.Join(dir, "memory.light.svg")); strings.Contains(got, ">999<") {
		t.Error("-check rewrote a chart instead of only reporting it")
	}
}

// TestDocBlock_CarriesProvenanceFiguresAndTables verifies the Markdown block
// says what the numbers are of, embeds each figure as a theme-aware picture,
// and prints every table.
func TestDocBlock_CarriesProvenanceFiguresAndTables(t *testing.T) {
	run := sampleRun()
	l := englishLabels()
	got := docBlock(run, l)

	for _, want := range []string{
		"Test CPU",    // the machine
		"linux/amd64", // its platform
		"go1.27.1",    // the toolchain
		"2.7.6",       // the build
		"01234567",    // its commit, shortened
		"<picture>",
		"benchmarks/memory.dark.svg",
		"benchmarks/memory.light.svg",
		l.TableCaption["summary"],
		l.TableCaption["startup"],
		l.TableCaption["latency"],
		"| stdio, dynamic",
		"`tools/list`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the generated block does not contain %q", want)
		}
	}
	if strings.Contains(got, "\n\n\n") {
		t.Error("the generated block has a run of blank lines, which markdownlint refuses")
	}
}

// TestSiteBlock_UsesTheComponentAndTheRightLanguage verifies the site block
// embeds charts through the component rather than raw markup, points at the
// language's own chart directory, and carries that language's wording.
func TestSiteBlock_UsesTheComponentAndTheRightLanguage(t *testing.T) {
	run := sampleRun()

	english := siteBlock(run, englishLabels())
	spanish := siteBlock(run, spanishLabels())

	if !strings.Contains(english, `<ChartPair name="memory" lang="en"`) {
		t.Error("the English block does not embed the memory figure through ChartPair")
	}
	if !strings.Contains(spanish, `lang="es"`) {
		t.Error("the Spanish block does not point at the Spanish charts")
	}
	if !strings.Contains(spanish, spanishLabels().TableCaption["summary"]) {
		t.Error("the Spanish block does not carry Spanish table captions")
	}
	if strings.Contains(spanish, englishLabels().TableCaption["summary"]) {
		t.Error("the Spanish block carries an English caption, which the site's language parity forbids")
	}
	if strings.Contains(english, "<!--") {
		t.Error("the site block contains an HTML comment, which MDX refuses to parse")
	}
}

// TestWriteSection_RewritesBetweenMarkersAndChecks verifies the generated
// block replaces only what is between the markers, reports no change when
// nothing moved, and in -check mode reports a difference without writing.
func TestWriteSection_RewritesBetweenMarkersAndChecks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.md")
	original := "# Title\n\nprose before\n\n" + docStartMark + "\n\nold\n\n" + docEndMark + "\n\nprose after\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	changed, err := writeSection(path, docStartMark, docEndMark, "new content", false)
	if err != nil {
		t.Fatalf("writeSection: %v", err)
	}
	if !changed {
		t.Error("writeSection reported no change after replacing the block")
	}
	got := readFileForTest(t, path)
	for _, want := range []string{"prose before", "prose after", "new content"} {
		if !strings.Contains(got, want) {
			t.Errorf("the rewritten page lost %q", want)
		}
	}
	if strings.Contains(got, "old") {
		t.Error("the rewritten page kept the previous generated content")
	}

	changed, err = writeSection(path, docStartMark, docEndMark, "new content", true)
	if err != nil {
		t.Fatalf("writeSection -check: %v", err)
	}
	if changed {
		t.Error("-check reported a change when the block was already current")
	}
}

// TestWriteSection_MissingMarkers_ReturnsError verifies a page without the
// markers fails loudly instead of being silently left alone, which would leave
// a published page carrying numbers from a run nobody remembers.
func TestWriteSection_MissingMarkers_ReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.md")
	if err := os.WriteFile(path, []byte("# Title\n\nno markers here\n"), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	if _, err := writeSection(path, docStartMark, docEndMark, "content", false); err == nil {
		t.Error("writeSection accepted a page with no markers")
	}
	if _, err := writeSection(filepath.Join(t.TempDir(), "absent.md"), docStartMark, docEndMark, "x", false); err == nil {
		t.Error("writeSection accepted a page that does not exist")
	}
}

// TestTableFormatting_MatchesTheRepositoryFormatter verifies the generated
// tables are already in the shape cmd/format_md_tables normalizes to.
//
// They have to be: the formatter runs over docs/ and the site content, so a
// generator emitting a different padding would make the two gates undo each
// other on every run.
func TestTableFormatting_MatchesTheRepositoryFormatter(t *testing.T) {
	block := docBlock(sampleRun(), englishLabels())
	for line := range strings.SplitSeq(block, "\n") {
		if !strings.HasPrefix(line, "|") {
			continue
		}
		if strings.Contains(line, "| ") && strings.HasSuffix(line, " ") {
			t.Errorf("table row has trailing whitespace: %q", line)
		}
		if !strings.HasSuffix(line, "|") {
			t.Errorf("table row does not end with a pipe: %q", line)
		}
	}
}

// TestMeasuredOn_UnknownBuild_StillNamesTheMachine verifies provenance is
// written even when the build could not be read, since a run that only
// measured stdio has no /health to ask.
func TestMeasuredOn_UnknownBuild_StillNamesTheMachine(t *testing.T) {
	run := sampleRun()
	run.Server = ServerInfo{}
	got := measuredOn(run, englishLabels())
	if !strings.Contains(got, "unknown") {
		t.Errorf("measuredOn = %q, want it to admit the build is unknown", got)
	}
	if !strings.Contains(got, "Test CPU") {
		t.Errorf("measuredOn = %q, want it to name the machine anyway", got)
	}
}

// darkTestPalette is testPalette in the other scheme, so a pair of renderings
// can be compared.
func darkTestPalette() palette {
	p := testPalette()
	p.Scheme = schemeDark
	p.Page = "#000000"
	p.Text = "#ffffff"
	return p
}

// readFileForTest reads a generated artifact.
func readFileForTest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}
