// render_test.go covers the artifacts a record turns into: the chart files,
// the generated block in the Markdown reference and in each language of the
// site, and the -check mode that gates all of them.
package main

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
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

// TestSeriesBlocks_TablesAndStopSentencesInEachLanguage verifies the
// generated block gains a concurrency section when the record holds a
// series: a subheading per series naming its settings and budget, a row per
// step, and a sentence saying where it stopped, in the page's language, and
// nothing at all for a record without one.
func TestSeriesBlocks_TablesAndStopSentencesInEachLanguage(t *testing.T) {
	if got := seriesBlocks(sampleRun(), englishLabels(), "###"); got != "" {
		t.Errorf("a record with no series produced %q", got)
	}

	english := docBlock(sampleSeriesRun(), englishLabels())
	for _, want := range []string{
		"### Concurrency series",
		"#### http, dynamic surface: 4 in flight per credential, 10 s per step, memory budget 4000 MiB",
		"#### http, meta surface",
		"| Credentials |",
		"| Settled heap | Settled resident |",
		"the peak resident set under load grows 36.61 MiB per credential",
		"the settled live heap, read with the load stopped and a collection forced, grows 512.0 KiB per credential",
		"That is a credential together with the requests it keeps in flight, not what a credential costs to hold",
		"Every planned step ran, up to 20 credentials.",
		"Stopped at 5 credentials: the next step (20) was estimated at 4300 MiB against a budget of 4000 MiB.",
		"Stopped at 5 credentials: the tools/call p99 reached 31000 ms, above the 30000 ms ceiling.",
		`<img alt="Lines on a log scale of credentials showing each surface's peak resident memory per step`,
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(english, want) {
				t.Errorf("the English block does not contain %q", want)
			}
		})
	}
	if strings.Contains(english, "\n\n\n") {
		t.Error("the generated block has a run of blank lines, which markdownlint refuses")
	}
	// The cells are padded to the column, so the row for twenty credentials
	// is matched by shape rather than by a literal. The settled pair sits
	// between the resident columns and the processor time, which is the order
	// this asserts.
	if !regexp.MustCompile(`(?m)^\|\s+20 \|\s+880 \|\s+900 \|\s+50\.0 \|\s+800 \|\s+1\.400 \|\s+8000 \|`).MatchString(english) {
		t.Error("the meta series table has no row for its twenty-credential step")
	}
	// The dynamic series measured no settled reading, so its own section
	// carries neither settled column: two columns of "n/a" would read as a
	// process holding nothing rather than as a run that did not look.
	var dynamic string
	for section := range strings.SplitSeq(english, "\n#### ") {
		if strings.HasPrefix(section, "http, dynamic surface") {
			dynamic = section
		}
	}
	if dynamic == "" {
		t.Fatal("the generated block has no section for the dynamic series")
	}
	if strings.Contains(dynamic, "Settled heap") {
		t.Error("a series with no settled reading was given settled columns")
	}
	if !strings.Contains(dynamic, "this record carries no settled reading") {
		t.Error("a series with no settled reading does not say so under its table")
	}

	spanish := siteBlock(sampleSeriesRun(), spanishLabels())
	for _, want := range []string{
		"### Serie de concurrencia",
		"presupuesto de memoria de 4000 MiB",
		"| Heap en reposo | Residente en reposo |",
		"el pico de conjunto residente bajo carga crece 36.61 MiB por credencial",
		"el heap vivo en reposo, leído con la carga detenida y una recolección forzada, crece 512.0 KiB por credencial",
		"no lo que cuesta mantener una credencial",
		"Detenida en 5 credenciales: el siguiente paso (20) se estimó en 4300 MiB frente a un presupuesto de 4000 MiB.",
		"Detenida en 5 credenciales: el p99 de tools/call alcanzó 31000 ms, por encima del techo de 30000 ms.",
		"Se ejecutaron todos los pasos previstos, hasta 20 credenciales.",
		`<ChartPair name="series-memory" lang="es"`,
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(spanish, want) {
				t.Errorf("the Spanish block does not contain %q", want)
			}
		})
	}
	for _, english := range []string{
		"Stopped at", "Every planned step", "memory budget", "Credentials |",
		"Settled heap", "Settled resident", "per credential",
	} {
		t.Run(english, func(t *testing.T) {
			if strings.Contains(spanish, english) {
				t.Errorf("the Spanish block carries the English %q", english)
			}
		})
	}
}

// TestSeriesSlopeSentence_SaysWhichGrowthItIs verifies the sentence under a
// series table reports both slopes when the record carries a settled reading,
// only the resident one otherwise, and nothing at all from steps that fix no
// line.
//
// Which of the two a figure is, is the whole point of the sentence. Published
// alone, the resident slope was read as what a pooled credential costs, and it
// is the credential and everything it keeps in flight together.
func TestSeriesSlopeSentence_SaysWhichGrowthItIs(t *testing.T) {
	l := englishLabels()
	step := func(clients int, peak, settled float64) SeriesStep {
		return SeriesStep{Clients: clients, RSSPeakMiB: peak, SettledHeapMiB: settled}
	}
	cases := []struct {
		name  string
		steps []SeriesStep
		want  string
	}{
		{
			name:  "both",
			steps: []SeriesStep{step(1, 200, 40.5), step(3, 220, 41.5)},
			want: "Fitted across these steps: the peak resident set under load grows 10.00 MiB per credential, " +
				"and the settled live heap, read with the load stopped and a collection forced, grows 512.0 KiB per credential. " +
				"The first is what a credential costs while it and every other one is calling; the second is what it costs to hold. " +
				"The settled resident set lags both, because Go returns freed pages to the operating system on its own schedule.",
		},
		{
			name:  "no settled reading",
			steps: []SeriesStep{step(1, 200, 0), step(3, 220, 0)},
			want: "Fitted across these steps, the peak resident set under load grows 10.00 MiB per credential. " +
				"That is a credential together with the requests it keeps in flight, not what a credential costs to hold: " +
				"this record carries no settled reading.",
		},
		{name: "one step", steps: []SeriesStep{step(1, 200, 40.5)}, want: ""},
		{name: "no steps", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := seriesSlopeSentence(SeriesScenario{Steps: tc.steps}, l); got != tc.want {
				t.Errorf("seriesSlopeSentence = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestMibFine_KeepsWhatWholeNumbersWouldRoundAway verifies the settled
// columns' renderer prints a small live heap at a precision that shows a
// credential's cost and a large resident set without decimals that are noise,
// and marks a figure that was not taken rather than printing it as zero.
func TestMibFine_KeepsWhatWholeNumbersWouldRoundAway(t *testing.T) {
	cases := []struct {
		name  string
		value float64
		want  string
	}{
		{name: "not taken", value: 0, want: "n/a"},
		{name: "a small heap", value: 0.384, want: "0.38"},
		{name: "under ten", value: 9.5, want: "9.50"},
		{name: "under a hundred", value: 40.53, want: "40.5"},
		{name: "a resident set", value: 2104.6, want: "2105"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mibFine(tc.value); got != tc.want {
				t.Errorf("mibFine(%v) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

// TestSeriesSentence_EveryKindOfEnding verifies the sentence under a series
// table covers every way a series ends, including a failure and a series
// that ran with no budget at all.
func TestSeriesSentence_EveryKindOfEnding(t *testing.T) {
	l := englishLabels()
	cases := []struct {
		name string
		s    SeriesScenario
		want string
	}{
		{name: "complete", s: SeriesScenario{StoppedAt: 1000}, want: "Every planned step ran, up to 1000 credentials."},
		{
			name: "budget", s: SeriesScenario{StoppedAt: 200, BudgetMiB: 48000, Stop: &SeriesStop{Kind: stopBudget, NextClients: 500, EstimateMiB: 61000}},
			want: "Stopped at 200 credentials: the next step (500) was estimated at 61000 MiB against a budget of 48000 MiB.",
		},
		{
			name: "latency", s: SeriesScenario{StoppedAt: 500, Stop: &SeriesStop{Kind: stopLatency, P99Ms: 33000}},
			want: "Stopped at 500 credentials: the tools/call p99 reached 33000 ms, above the 30000 ms ceiling.",
		},
		{
			name: "failure", s: SeriesScenario{StoppedAt: 100, Stop: &SeriesStop{Kind: stopFailure, NextClients: 200, Error: "too many open files"}},
			want: "Stopped at 100 credentials: admitting the credentials of the next step (200) failed: too many open files.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := seriesSentence(tc.s, l); got != tc.want {
				t.Errorf("seriesSentence = %q, want %q", got, tc.want)
			}
		})
	}
	t.Run("no budget in the caption", func(t *testing.T) {
		run := sampleSeriesRun()
		run.Series = run.Series[:1]
		run.Series[0].BudgetMiB = 0
		if got := seriesBlocks(run, l, "###"); !strings.Contains(got, "10 s per step, no memory budget") {
			t.Errorf("the caption does not say the series ran with no budget: %q", got)
		}
	})
}

// TestRenderAll_ReportsWhatItCannotWriteOrCheck covers the four ways the
// renderer stops with the artifact named: a site chart directory that cannot
// be created, a documentation chart directory that cannot be created, a page
// with no markers, and a -check over artifacts that were never written.
func TestRenderAll_ReportsWhatItCannotWriteOrCheck(t *testing.T) {
	block := func(t *testing.T, root, path string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatalf("create the parent of %s: %v", path, err)
		}
		//#nosec G703 -- both halves of the path are this test's own: a t.TempDir and a literal
		if err := os.WriteFile(full, []byte("in the way"), 0o600); err != nil {
			t.Fatalf("block %s: %v", path, err)
		}
	}
	cases := []struct {
		name    string
		prepare func(t *testing.T, root string, opts *options)
		want    string
	}{
		{
			name: "site charts cannot be created",
			prepare: func(t *testing.T, root string, opts *options) {
				t.Helper()
				block(t, root, opts.siteCharts)
			},
			want: "site charts (en)",
		},
		{
			name: "documentation charts cannot be created",
			prepare: func(t *testing.T, root string, opts *options) {
				t.Helper()
				block(t, root, opts.docCharts)
			},
			want: "documentation charts",
		},
		{
			name: "a page without markers",
			prepare: func(t *testing.T, root string, opts *options) {
				t.Helper()
				//#nosec G703 -- both halves of the path are this test's own: a t.TempDir and a literal
				if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(opts.docPage)), []byte("no markers\n"), 0o600); err != nil {
					t.Fatalf("rewrite the page: %v", err)
				}
			},
			want: "generated section in docs/page.md",
		},
		{
			name:    "checking before anything was written",
			prepare: func(_ *testing.T, _ string, opts *options) { opts.check = true },
			want:    "stale",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, opts := renderTree(t)
			tc.prepare(t, root, &opts)
			err := renderAll(opts, root, sampleRun())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("renderAll = %v, want an error saying %q", err, tc.want)
			}
		})
	}
}

// TestWriteCharts_ReportsWhereItCouldNotWrite covers a chart directory that
// is a file and a chart file that cannot be written.
func TestWriteCharts_ReportsWhereItCouldNotWrite(t *testing.T) {
	palettes := map[string]palette{schemeLight: testPalette(), schemeDark: darkTestPalette()}

	t.Run("directory is a file", func(t *testing.T) {
		blocker := filepath.Join(t.TempDir(), "charts")
		//#nosec G703 -- both halves of the path are this test's own: a t.TempDir and a literal
		if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
			t.Fatalf("write the blocking file: %v", err)
		}
		if _, err := writeCharts(blocker, sampleRun(), englishLabels(), palettes, false); err == nil || !strings.Contains(err.Error(), "create") {
			t.Errorf("writeCharts = %v, want the create failure", err)
		}
	})

	t.Run("a chart cannot be written", func(t *testing.T) {
		previous := writeOutput
		writeOutput = func(string, []byte, os.FileMode) error { return errors.New("disk full") }
		t.Cleanup(func() { writeOutput = previous })
		_, err := writeCharts(filepath.Join(t.TempDir(), "charts"), sampleRun(), englishLabels(), palettes, false)
		if err == nil || !strings.Contains(err.Error(), "disk full") {
			t.Errorf("writeCharts = %v, want the write failure", err)
		}
	})
}

// TestWriteSection_ChangedUnderCheck_ReportsWithoutWriting verifies -check
// reports a block that would change and leaves the file alone, and that a
// write that fails is reported as such.
func TestWriteSection_ChangedUnderCheck_ReportsWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.md")
	original := "before\n\n" + docStartMark + "\n\nold\n\n" + docEndMark + "\n\nafter\n"
	//#nosec G703 -- both halves of the path are this test's own: a t.TempDir and a literal
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	changed, err := writeSection(path, docStartMark, docEndMark, "new", true)
	if err != nil || !changed {
		t.Errorf("writeSection -check = (%v, %v), want a reported change", changed, err)
	}
	if readFileForTest(t, path) != original {
		t.Error("-check rewrote the page")
	}

	previous := writeOutput
	writeOutput = func(string, []byte, os.FileMode) error { return errors.New("read-only") }
	t.Cleanup(func() { writeOutput = previous })
	if _, writeErr := writeSection(path, docStartMark, docEndMark, "new", false); writeErr == nil || !strings.Contains(writeErr.Error(), "read-only") {
		t.Errorf("writeSection = %v, want the write failure", writeErr)
	}
}

// TestTableCells_ZeroIsNotAvailable checks the table formatters print n/a
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
		{name: "cpu per call unmeasured", render: cpuPerCallLabel, value: 0, want: "n/a"},
		{name: "cpu per call measured", render: cpuPerCallLabel, value: 1.2346, want: "1.235"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.render(tc.value); got != tc.want {
				t.Errorf("%s(%v) = %q, want %q", tc.name, tc.value, got, tc.want)
			}
		})
	}
}
