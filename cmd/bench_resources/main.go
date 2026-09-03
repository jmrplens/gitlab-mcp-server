// Command bench_resources measures what this server costs to run, and draws
// the charts the documentation publishes.
//
// Everything else measured in this repository is about tokens and tool counts.
// None of it tells an operator how much memory to give a container, how long a
// client waits before the first tool call answers, or what a second credential
// adds to a shared deployment. This command answers those, from the real
// binary, on both transports, and writes one record every downstream artifact
// is rendered from.
//
// # What it needs
//
// Nothing but a Go toolchain. GitLab is stood in for by an in-process HTTP
// server on loopback, and the OTLP collector by another, so a run is offline
// and a second machine measures the same thing rather than its own network.
// The tool surface is passed to the server explicitly and never read from the
// environment, for the reason CLAUDE.md gives about generators: a developer
// machine exporting GITLAB_MCP_TOOL_SURFACE would otherwise publish different
// numbers than CI.
//
// # Usage
//
//	go run ./cmd/bench_resources/                 # measure, then render
//	go run ./cmd/bench_resources/ -render         # redraw from the record
//	go run ./cmd/bench_resources/ -check          # is the drawing current?
//	go run ./cmd/bench_resources/ -quick -json /tmp/x.json
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/mcpsurface"
)

// Default output locations, all relative to the module root. The record lives
// under the site's data directory because the site imports it directly, the
// same single-sourcing stats.json and token-footprint.json already use.
const (
	defaultRecord     = "site/src/data/resource-benchmark.json"
	defaultDocCharts  = "docs/reference/benchmarks"
	defaultSiteCharts = "site/public/benchmarks"
	defaultDocPage    = "docs/reference/resource-benchmark.md"
	defaultSitePageEN = "site/src/content/docs/operations/resource-benchmark.mdx"
	defaultSitePageES = "site/src/content/docs/es/operations/resource-benchmark.mdx"
)

// options are the command's flags.
type options struct {
	binary         string
	record         string
	docCharts      string
	siteCharts     string
	docPage        string
	sitePageEN     string
	sitePageES     string
	scenarios      string
	rounds         int
	sampleInterval time.Duration
	render         bool
	check          bool
	quick          bool
	verbose        bool
	// recordSet records whether -json was given, which is what lets a partial
	// matrix be refused before it overwrites the published record.
	recordSet bool
}

func main() {
	opts := parseFlags()
	if err := execute(opts); err != nil {
		fmt.Fprintf(os.Stderr, "bench_resources: %v\n", err)
		os.Exit(1)
	}
}

// parseFlags reads the command line.
func parseFlags() options {
	var opts options
	flag.StringVar(&opts.binary, "binary", "", "server binary to measure; empty builds ./cmd/server into a temporary directory")
	flag.StringVar(&opts.record, "json", defaultRecord, "measurement record to write, and to render from")
	flag.StringVar(&opts.docCharts, "doc-charts", defaultDocCharts, "directory for the Markdown documentation's SVG charts")
	flag.StringVar(&opts.siteCharts, "site-charts", defaultSiteCharts, "directory for the site's SVG charts")
	flag.StringVar(&opts.docPage, "doc-page", defaultDocPage, "Markdown page whose generated block is rewritten")
	flag.StringVar(&opts.sitePageEN, "site-page", defaultSitePageEN, "English site page whose generated block is rewritten")
	flag.StringVar(&opts.sitePageES, "site-page-es", defaultSitePageES, "Spanish site page whose generated block is rewritten")
	flag.StringVar(&opts.scenarios, "scenarios", "", "comma-separated scenario ids to measure; empty runs the whole matrix")
	flag.IntVar(&opts.rounds, "rounds", 3, "measured rounds per method")
	flag.DurationVar(&opts.sampleInterval, "sample-interval", 100*time.Millisecond, "how often the resident set is sampled")
	flag.BoolVar(&opts.render, "render", false, "skip measurement: redraw charts and tables from the committed record")
	flag.BoolVar(&opts.check, "check", false, "verify the committed charts and tables match the committed record; implies -render")
	flag.BoolVar(&opts.quick, "quick", false, "short smoke matrix, for verifying a change to this command")
	flag.BoolVar(&opts.verbose, "v", false, "print progress for every client and round")
	flag.Parse()

	flag.Visit(func(f *flag.Flag) {
		if f.Name == "json" {
			opts.recordSet = true
		}
	})
	return opts
}

// execute runs the command: measure unless asked not to, then render.
func execute(opts options) error {
	root, err := mcpsurface.ProjectRoot()
	if err != nil {
		return err
	}

	recordPath := resolve(root, opts.record)
	if !opts.render && !opts.check {
		run, measureErr := measure(opts, root)
		if measureErr != nil {
			return measureErr
		}
		if writeErr := writeRun(recordPath, run); writeErr != nil {
			return writeErr
		}
		fmt.Printf("wrote %s\n", rel(root, recordPath))
	}

	run, err := readRun(recordPath)
	if err != nil {
		return err
	}
	return renderAll(opts, root, run)
}

// measure runs the matrix against a freshly built binary.
func measure(opts options, root string) (*Run, error) {
	plans, err := matrixFor(opts)
	if err != nil {
		return nil, err
	}

	binary := opts.binary
	if binary == "" {
		built, buildErr := buildServer(root)
		if buildErr != nil {
			return nil, buildErr
		}
		defer func() { _ = os.RemoveAll(filepath.Dir(built)) }()
		binary = built
	}

	stub := startStubGitLab()
	defer stub.close()
	sink := startOTLPSink()
	defer sink.close()

	r := &runner{
		binary:         binary,
		stub:           stub,
		otlp:           sink,
		sampleInterval: opts.sampleInterval,
		progress:       progressFunc(opts.verbose),
	}

	run := &Run{
		Schema:      resultSchema,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Host:        hostInfo(),
		Settings: Settings{
			// Taken from the matrix rather than from the flag: the smoke
			// matrix pins its own rounds, and a record that reported the flag
			// would describe a run that did not happen.
			Rounds:           plans[0].Rounds,
			SampleIntervalMs: int(opts.sampleInterval / time.Millisecond),
			Quick:            opts.quick,
		},
	}

	ctx := context.Background()
	started := time.Now()
	for i, plan := range plans {
		fmt.Printf("[%d/%d] %s: %s\n", i+1, len(plans), plan.ID, plan.describe())
		scenarioStarted := time.Now()
		scenario, runErr := r.runScenario(ctx, plan)
		if runErr != nil {
			return nil, fmt.Errorf("scenario %s: %w", plan.ID, runErr)
		}
		run.Scenarios = append(run.Scenarios, scenario)
		fmt.Printf("      idle %.0f MiB, %d clients %.0f MiB, peak %.0f MiB, %d goroutines, %s\n",
			scenario.Memory.IdleMiB, scenario.Clients, scenario.Memory.AllClientsMiB,
			scenario.Memory.PeakMiB, scenario.Goroutines, time.Since(scenarioStarted).Round(time.Second))
	}
	run.Server = r.serverInfo
	fmt.Printf("measured %d scenarios in %s\n", len(plans), time.Since(started).Round(time.Second))
	return run, nil
}

// matrixFor picks the matrix and refuses to overwrite the published record
// with a partial one.
//
// A -quick or -scenarios run is a development aid: writing it to the default
// path would replace measured, published numbers with a smoke test, and the
// page would keep claiming those were the figures.
func matrixFor(opts options) ([]scenarioPlan, error) {
	plans := publishedMatrix(opts.rounds)
	partial := false
	if opts.quick {
		plans = quickMatrix()
		partial = true
	}
	if opts.scenarios != "" {
		partial = true
		selected, err := selectPlans(plans, opts.scenarios)
		if err != nil {
			return nil, err
		}
		plans = selected
	}
	if partial && !opts.recordSet {
		return nil, errors.New("a partial matrix must write somewhere else: pass -json with a path of your own")
	}
	return plans, nil
}

// buildServer compiles the binary under measurement and returns its path.
//
// Built rather than assumed present, and built from this checkout, so the
// numbers belong to the tree they are published from.
func buildServer(root string) (string, error) {
	dir, err := os.MkdirTemp("", "bench-resources")
	if err != nil {
		return "", fmt.Errorf("create a build directory: %w", err)
	}
	out := filepath.Join(dir, "server")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fmt.Println("building ./cmd/server")
	cmd := exec.CommandContext(ctx, "go", "build", "-o", out, "./cmd/server")
	cmd.Dir = root
	if output, runErr := cmd.CombinedOutput(); runErr != nil {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("build ./cmd/server: %w\n%s", runErr, output)
	}
	return out, nil
}

// progressFunc returns the per-client and per-round reporter, silent unless
// -v was given: a full run prints eight scenario lines, which is the right
// amount of output for a target that takes minutes.
func progressFunc(verbose bool) func(string, ...any) {
	if !verbose {
		return func(string, ...any) {}
	}
	return func(format string, args ...any) {
		fmt.Printf(format+"\n", args...)
	}
}

// resolve makes a flag path absolute against the module root.
func resolve(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

// rel renders a path for a message, relative to the module root when it is
// inside it.
func rel(root, path string) string {
	if relative, err := filepath.Rel(root, path); err == nil {
		return relative
	}
	return path
}
