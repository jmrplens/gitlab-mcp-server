// render.go writes everything the record implies: the SVG pairs, the block of
// tables in the Markdown reference, and the same block in each language of the
// documentation site.
//
// Rendering is separated from measuring so the drawing can be verified. -check
// re-renders from the committed record and compares, which turns "the chart
// matches the numbers" from a promise into a gate; -render redraws after a
// change to the figures without spending minutes re-measuring.
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/docgen"
)

// Markers for the generated block. The Markdown page uses HTML comments, the
// way every other managed section in this repository does; the site pages use
// an MDX expression comment, because MDX parses an HTML comment as markup and
// refuses the file.
const (
	docStartMark = "<!-- START BENCHMARK -->"
	docEndMark   = "<!-- END BENCHMARK -->"
	// The MDX comment carries no spaces inside its delimiters on purpose:
	// markdownlint reads "/* " as an emphasis marker followed by a space and
	// fails the page on MD037.
	siteStartMark = "{/*START BENCHMARK*/}"
	siteEndMark   = "{/*END BENCHMARK*/}"
)

// themeCSS is where the chart palette is read from.
const themeCSS = "site/src/styles/theme.css"

// renderAll draws the figures and rewrites the generated blocks.
func renderAll(opts options, root string, run *Run) error {
	palettes, err := loadPalettes(readFileString(resolve(root, themeCSS)))
	if err != nil {
		return err
	}

	english, spanish := englishLabels(), spanishLabels()
	var changed []string

	// The Markdown reference is English only, and reads its charts from a
	// directory beside it so a GitHub reader gets them without the site.
	for _, l := range []labels{english, spanish} {
		dir := filepath.Join(resolve(root, opts.siteCharts), l.Code)
		written, chartErr := writeCharts(dir, run, l, palettes, opts.check)
		if chartErr != nil {
			return chartErr
		}
		changed = append(changed, relAll(root, written)...)
	}
	written, err := writeCharts(resolve(root, opts.docCharts), run, english, palettes, opts.check)
	if err != nil {
		return err
	}
	changed = append(changed, relAll(root, written)...)

	sections := []struct {
		path       string
		start, end string
		content    string
	}{
		{resolve(root, opts.docPage), docStartMark, docEndMark, docBlock(run, english)},
		{resolve(root, opts.sitePageEN), siteStartMark, siteEndMark, siteBlock(run, english)},
		{resolve(root, opts.sitePageES), siteStartMark, siteEndMark, siteBlock(run, spanish)},
	}
	for _, section := range sections {
		sectionChanged, sectionErr := writeSection(section.path, section.start, section.end, section.content, opts.check)
		if sectionErr != nil {
			return sectionErr
		}
		if sectionChanged {
			changed = append(changed, rel(root, section.path))
		}
	}

	if opts.check {
		if len(changed) > 0 {
			return fmt.Errorf("generated benchmark artifacts are stale in %d file(s): %s",
				len(changed), strings.Join(changed, ", "))
		}
		fmt.Println("benchmark charts and tables are up to date")
		return nil
	}
	if len(changed) == 0 {
		fmt.Println("benchmark charts and tables already current")
		return nil
	}
	fmt.Printf("updated %d file(s)\n", len(changed))
	for _, file := range changed {
		fmt.Printf("- %s\n", file)
	}
	return nil
}

// writeCharts renders every figure in both schemes into one directory.
func writeCharts(dir string, run *Run, l labels, palettes map[string]palette, check bool) ([]string, error) {
	if !check {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("create %s: %w", dir, err)
		}
	}
	var changed []string
	for _, fig := range buildFigures(run, l) {
		for _, scheme := range []string{schemeLight, schemeDark} {
			path := filepath.Join(dir, fig.Name+"."+scheme+".svg")
			svg := fig.Render(palettes[scheme])
			fileChanged, err := writeFile(path, []byte(svg), check)
			if err != nil {
				return nil, err
			}
			if fileChanged {
				changed = append(changed, path)
			}
		}
	}
	return changed, nil
}

// writeFile writes content unless checking, and reports whether the file on
// disk differs.
func writeFile(path string, content []byte, check bool) (bool, error) {
	existing, err := os.ReadFile(path) // #nosec G304 -- generated output paths, from this command's flags
	if err == nil && bytes.Equal(existing, content) {
		return false, nil
	}
	if check {
		return true, nil
	}
	if writeErr := os.WriteFile(path, content, 0o600); writeErr != nil {
		return false, fmt.Errorf("write %s: %w", path, writeErr)
	}
	return true, nil
}

// writeSection rewrites one generated block, or reports that it would change.
func writeSection(path, start, end, content string, check bool) (bool, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- generated output paths, from this command's flags
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	updated, err := docgen.ComputeReplacedSection(string(data), start, end, content)
	if err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	if updated == string(data) {
		return false, nil
	}
	if check {
		return true, nil
	}
	//#nosec G703 -- the path is this command's own flag, and the file was just read from it
	if writeErr := os.WriteFile(path, []byte(updated), 0o600); writeErr != nil {
		return false, fmt.Errorf("write %s: %w", path, writeErr)
	}
	return true, nil
}

// docBlock is the generated body of the Markdown reference page.
func docBlock(run *Run, l labels) string {
	var b strings.Builder
	b.WriteString(measuredOn(run, l) + "\n")
	for _, fig := range buildFigures(run, l) {
		b.WriteString("\n<picture>\n")
		fmt.Fprintf(&b, "  <source media=\"(prefers-color-scheme: dark)\" srcset=\"benchmarks/%s.dark.svg\">\n", fig.Name)
		fmt.Fprintf(&b, "  <img alt=\"%s\" src=\"benchmarks/%s.light.svg\">\n", l.FigureAlt[fig.Name], fig.Name)
		b.WriteString("</picture>\n")
	}
	b.WriteString("\n")
	b.WriteString(tableBlocks(run, l, "###"))
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// siteBlock is the generated body of one language's site page.
func siteBlock(run *Run, l labels) string {
	var b strings.Builder
	b.WriteString(measuredOn(run, l) + "\n\n")
	for _, fig := range buildFigures(run, l) {
		fmt.Fprintf(&b, "<ChartPair name=\"%s\" lang=\"%s\" alt=\"%s\" />\n\n", fig.Name, l.Code, l.FigureAlt[fig.Name])
	}
	b.WriteString(tableBlocks(run, l, "###"))
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// measuredOn is the sentence that makes the numbers actionable: without the
// machine, the build and the method, a resident set is a rumor.
func measuredOn(run *Run, l labels) string {
	build := run.Server.Version
	if run.Server.Commit != "" {
		build += " (" + shortCommit(run.Server.Commit) + ")"
	}
	if strings.TrimSpace(build) == "" {
		build = "unknown"
	}
	return fmt.Sprintf(l.MeasuredOn,
		run.Host.describe(l), build, run.GeneratedAt, run.Settings.Rounds, run.Settings.SampleIntervalMs)
}

// shortCommit trims a commit to the width the rest of the documentation uses.
func shortCommit(commit string) string {
	if len(commit) > 8 {
		return commit[:8]
	}
	return commit
}

// tableBlocks renders the three tables with headings at the given level, so
// the same content fits under a Markdown page's H3 and a site page's H2.
func tableBlocks(run *Run, l labels, heading string) string {
	var b strings.Builder
	tables := []struct {
		key   string
		table string
	}{
		{"summary", summaryTable(run, l)},
		{"startup", startupTable(run, l)},
		{"latency", latencyTable(run, l)},
	}
	for _, entry := range tables {
		fmt.Fprintf(&b, "%s %s\n\n%s\n\n", heading, l.TableCaption[entry.key], strings.TrimRight(entry.table, "\n"))
	}
	return b.String()
}

// summaryTable is memory, goroutines and processor time per scenario.
func summaryTable(run *Run, l labels) string {
	alignments := []docgen.Alignment{
		docgen.AlignLeft, docgen.AlignRight, docgen.AlignRight, docgen.AlignRight,
		docgen.AlignRight, docgen.AlignRight, docgen.AlignRight, docgen.AlignRight,
		docgen.AlignRight,
	}
	rows := make([][]string, 0, len(run.Scenarios))
	for _, s := range run.Scenarios {
		rows = append(rows, []string{
			scenarioLabel(s),
			strconv.Itoa(s.Clients),
			mib(s.Memory.IdleMiB),
			mib(s.Memory.OneClientMiB),
			mib(s.Memory.AllClientsMiB),
			mib(s.Memory.PerExtraClientMiB),
			mib(s.Memory.PeakMiB),
			strconv.Itoa(s.Goroutines),
			fmt.Sprintf("%.0f%%", s.CPU.LoadPercent),
		})
	}
	return docgen.RenderMarkdownTable(l.SummaryHead, alignments, rows)
}

// startupTable is what a client waits for, per scenario.
func startupTable(run *Run, l labels) string {
	alignments := []docgen.Alignment{
		docgen.AlignLeft, docgen.AlignRight, docgen.AlignRight,
		docgen.AlignRight, docgen.AlignRight,
	}
	rows := make([][]string, 0, len(run.Scenarios))
	for _, s := range run.Scenarios {
		rows = append(rows, []string{
			scenarioLabel(s),
			ms(s.Startup.ProcessReadyMs),
			ms(s.Startup.FirstListMs),
			ms(s.Startup.WarmListMs),
			bytesLabel(s.ListBytes),
		})
	}
	return docgen.RenderMarkdownTable(l.StartupHead, alignments, rows)
}

// latencyTable is every method's distribution in every scenario.
func latencyTable(run *Run, l labels) string {
	alignments := []docgen.Alignment{
		docgen.AlignLeft, docgen.AlignLeft, docgen.AlignLeft,
		docgen.AlignRight, docgen.AlignRight, docgen.AlignRight, docgen.AlignRight,
	}
	var rows [][]string
	for _, s := range run.Scenarios {
		for _, latency := range s.Latency {
			rows = append(rows, []string{
				scenarioLabel(s),
				"`" + latency.Method + "`",
				latency.Detail,
				ms(latency.P50),
				ms(latency.P90),
				ms(latency.P99),
				ms(latency.Max),
			})
		}
	}
	return docgen.RenderMarkdownTable(l.LatencyHead, alignments, rows)
}

// mib renders a mebibyte figure for a table.
func mib(value float64) string {
	if value == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.0f", value)
}

// ms renders a millisecond figure for a table.
func ms(value float64) string {
	if value == 0 {
		return "n/a"
	}
	return msLabel(value)
}

// relAll shortens a list of paths for a report.
func relAll(root string, paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		out = append(out, rel(root, path))
	}
	return out
}
