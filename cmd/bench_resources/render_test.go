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
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(got, want) {
				t.Errorf("the generated block does not contain %q", want)
			}
		})
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
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(got, want) {
				t.Errorf("the rewritten page lost %q", want)
			}
		})
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

// renderTree builds a stand-in module root holding the one input renderAll
// reads, the stylesheet, and the three pages it rewrites, each with its
// markers and nothing between them.
func renderTree(t *testing.T) (root string, opts options) {
	t.Helper()
	root = t.TempDir()

	realRoot, err := projectRootForTest()
	if err != nil {
		t.Skipf("cannot locate the module root: %v", err)
	}
	styles := filepath.Join(root, "site", "src", "styles")
	if mkErr := os.MkdirAll(styles, 0o750); mkErr != nil {
		t.Fatalf("create the stylesheet directory: %v", mkErr)
	}
	// The palette is read from the site's own stylesheet rather than restated,
	// so the test uses the real one: a copy here would be the second copy the
	// arrangement exists to avoid.
	theme, err := os.ReadFile(filepath.Join(realRoot, themeCSS))
	if err != nil {
		t.Fatalf("read the real stylesheet: %v", err)
	}
	//#nosec G703 -- both halves of the path are this test's own: a t.TempDir and a literal
	if writeErr := os.WriteFile(filepath.Join(styles, "theme.css"), theme, 0o600); writeErr != nil {
		t.Fatalf("write the stylesheet: %v", writeErr)
	}

	opts = options{
		docCharts:  "docs/charts",
		siteCharts: "site/charts",
		docPage:    "docs/page.md",
		sitePageEN: "site/en.mdx",
		sitePageES: "site/es.mdx",
	}
	pages := map[string][2]string{
		opts.docPage:    {docStartMark, docEndMark},
		opts.sitePageEN: {siteStartMark, siteEndMark},
		opts.sitePageES: {siteStartMark, siteEndMark},
	}
	for page, marks := range pages {
		full := filepath.Join(root, filepath.FromSlash(page))
		if mkErr := os.MkdirAll(filepath.Dir(full), 0o750); mkErr != nil {
			t.Fatalf("create the directory for %s: %v", page, mkErr)
		}
		body := "before\n\n" + marks[0] + "\n\n" + marks[1] + "\n\nafter\n"
		if writeErr := os.WriteFile(full, []byte(body), 0o600); writeErr != nil {
			t.Fatalf("write %s: %v", page, writeErr)
		}
	}
	return root, opts
}

// assertChartsWritten checks that every figure exists in both schemes, in the
// documentation directory and in each language's site directory.
func assertChartsWritten(t *testing.T, root string, opts options, run *Run) {
	t.Helper()
	dirs := []string{
		filepath.Join(root, filepath.FromSlash(opts.docCharts)),
		filepath.Join(root, filepath.FromSlash(opts.siteCharts), englishLabels().Code),
		filepath.Join(root, filepath.FromSlash(opts.siteCharts), spanishLabels().Code),
	}
	for _, fig := range buildFigures(run, englishLabels()) {
		for _, scheme := range []string{schemeLight, schemeDark} {
			for _, dir := range dirs {
				path := filepath.Join(dir, fig.Name+"."+scheme+".svg")
				if _, err := os.Stat(path); err != nil {
					t.Errorf("missing chart %s: %v", path, err)
				}
			}
		}
	}
}

// TestRenderAll_WritesEveryArtifactAndThenAgreesWithItself verifies one call
// produces the whole published set, and that a second call in -check mode over
// what the first wrote reports nothing stale.
//
// That pairing is the real assertion. Writing and checking are separate code
// paths over the same content, so a renderer whose output did not match its own
// check would pass CI on the machine that regenerated it and fail on every
// other, which is the failure mode a generated artifact has.
func TestRenderAll_WritesEveryArtifactAndThenAgreesWithItself(t *testing.T) {
	root, opts := renderTree(t)
	run := sampleRun()

	if err := renderAll(opts, root, run); err != nil {
		t.Fatalf("renderAll: %v", err)
	}

	t.Run("charts for the documentation and both languages", func(t *testing.T) {
		assertChartsWritten(t, root, opts, run)
	})

	t.Run("each page keeps what is outside its markers", func(t *testing.T) {
		for _, page := range []string{opts.docPage, opts.sitePageEN, opts.sitePageES} {
			t.Run(page, func(t *testing.T) {
				body := readFileForTest(t, filepath.Join(root, filepath.FromSlash(page)))
				if !strings.HasPrefix(body, "before") || !strings.HasSuffix(body, "after\n") {
					t.Errorf("%s lost the text around its generated block", page)
				}
			})
		}
	})

	t.Run("the Spanish page is Spanish", func(t *testing.T) {
		body := readFileForTest(t, filepath.Join(root, filepath.FromSlash(opts.sitePageES)))
		for _, detail := range recordedCallDetails {
			if strings.Contains(body, "| "+detail+" ") {
				t.Errorf("the Spanish table carries the English description %q", detail)
			}
		}
		if !strings.Contains(body, spanishLabels().CallDetail[detailWholeSurface]) {
			t.Error("the Spanish table does not carry its own wording")
		}
	})

	t.Run("checking what was just written finds nothing stale", func(t *testing.T) {
		checking := opts
		checking.check = true
		if err := renderAll(checking, root, run); err != nil {
			t.Errorf("renderAll -check over its own output: %v", err)
		}
	})
}

// TestRenderAll_MissingStylesheet_NamesWhatItCouldNotRead verifies the failure
// says which input was missing, since renderAll reads several and an
// unqualified "no such file" would not say which.
func TestRenderAll_MissingStylesheet_NamesWhatItCouldNotRead(t *testing.T) {
	root, opts := renderTree(t)
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(themeCSS))); err != nil {
		t.Fatalf("removing the stylesheet: %v", err)
	}

	err := renderAll(opts, root, sampleRun())
	if err == nil {
		t.Fatal("renderAll succeeded without a stylesheet to read the palette from")
	}
	if !strings.Contains(err.Error(), themeCSS) {
		t.Errorf("error = %v, want it to name %s", err, themeCSS)
	}
}

// TestRelAll_ShortensEveryPathAgainstTheRoot verifies the list of rewritten
// files a run prints is relative to the module root, which is what makes the
// report readable and machine-independent.
func TestRelAll_ShortensEveryPathAgainstTheRoot(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "home", "someone", "gitlab-mcp-server")
	paths := []string{
		filepath.Join(root, "docs", "reference", "resource-benchmark.md"),
		filepath.Join(root, "site", "src", "data", "resource-benchmark.json"),
	}

	got := relAll(root, paths)
	if len(got) != len(paths) {
		t.Fatalf("relAll returned %d paths, want %d", len(got), len(paths))
	}
	for i, path := range got {
		t.Run(path, func(t *testing.T) {
			if filepath.IsAbs(path) {
				t.Errorf("relAll left %q absolute", path)
			}
			if want := rel(root, paths[i]); path != want {
				t.Errorf("relAll = %q, want %q", path, want)
			}
		})
	}

	t.Run("nothing to shorten", func(t *testing.T) {
		if empty := relAll(root, nil); len(empty) != 0 {
			t.Errorf("relAll of no paths = %v, want empty", empty)
		}
	})
}

// TestWriteFile_UnreadablePath_IsAnErrorNotAChange verifies a read failure
// that is not "the file is absent" is reported as itself.
//
// writeFile treated every os.ReadFile error as evidence that the file differs
// from the content it was given, which it is not: a path that is a directory
// answers EISDIR whatever the content is. Under -check that surfaced as a
// stale-artifact report, sending a reader to regenerate a file that was never
// the problem, and it did so without ever naming the real cause.
func TestWriteFile_UnreadablePath_IsAnErrorNotAChange(t *testing.T) {
	directory := t.TempDir()

	changed, err := writeFile(directory, []byte("content"), true)
	if err == nil {
		t.Fatalf("writeFile(%s) = (%v, nil), want an error naming the path", directory, changed)
	}
	if !strings.Contains(err.Error(), directory) {
		t.Errorf("error = %v, want it to name %s", err, directory)
	}
	if changed {
		t.Error("writeFile reported a change it could not have observed")
	}

	// An absent file is still a change, which is what makes -check able to
	// report a chart that was never written.
	absent := filepath.Join(directory, "not-written-yet.svg")
	changed, err = writeFile(absent, []byte("content"), true)
	if err != nil {
		t.Fatalf("writeFile on an absent path: %v", err)
	}
	if !changed {
		t.Error("an absent file was not reported as a change")
	}
}

// TestShortCommit_TrimsToEightCharacters covers the width the rest of the
// documentation uses, and a short value left whole.
func TestShortCommit_TrimsToEightCharacters(t *testing.T) {
	cases := []struct{ name, commit, want string }{
		{name: "full sha", commit: "0123456789abcdef", want: "01234567"},
		{name: "already short", commit: "abc", want: "abc"},
		{name: "empty", commit: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shortCommit(tc.commit); got != tc.want {
				t.Errorf("shortCommit(%q) = %q, want %q", tc.commit, got, tc.want)
			}
		})
	}
}

// TestTableCells_ZeroIsNotAvailable checks the two table formatters print n/a
// for a figure that was not measured rather than a zero a reader would take
// for a measurement.
func TestTableCells_ZeroIsNotAvailable(t *testing.T) {
	cases := []struct {
		name   string
		render func(float64) string
		value  float64
		want   string
	}{
		{name: "mib unmeasured", render: mib, value: 0, want: "n/a"},
		{name: "mib measured", render: mib, value: 63.4, want: "63"},
		{name: "ms unmeasured", render: ms, value: 0, want: "n/a"},
		{name: "ms measured", render: ms, value: 12.6, want: "13"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.render(tc.value); got != tc.want {
				t.Errorf("%s(%v) = %q, want %q", tc.name, tc.value, got, tc.want)
			}
		})
	}
}
