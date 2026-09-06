package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/mcpsurface"
)

// Default output locations, all relative to the module root. The record lives
// under the site's data directory because the site imports it directly, the
// same single-sourcing stats.json and token-footprint.json already use. The
// profiles do not: they are binary, hundreds of kilobytes per step, and stay
// beside the record on the machine that measured, summarized into the
// analysis rather than committed.
const (
	defaultRecord     = "site/src/data/resource-benchmark.json"
	defaultDocCharts  = "docs/reference/benchmarks"
	defaultSiteCharts = "site/public/benchmarks"
	defaultDocPage    = "docs/reference/resource-benchmark.md"
	defaultSitePageEN = "site/src/content/docs/performance/resource-benchmark.mdx"
	defaultSitePageES = "site/src/content/docs/es/performance/resource-benchmark.mdx"
	defaultProfiles   = "bench/profiles"
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
	// clients, stepDuration, memoryBudget and profiles steer the concurrency
	// series; noRender stops after the record is written.
	clients      string
	stepDuration time.Duration
	memoryBudget int
	profiles     string
	noRender     bool
	// recordSet records whether -json was given, which is what lets a partial
	// matrix be refused before it overwrites the published record; the other
	// two remember flags whose default -quick overrides unless they were
	// typed.
	recordSet       bool
	clientsSet      bool
	stepDurationSet bool
}

// exitProcess is the exit main takes on failure, so a test can drive main
// through it and read the status back instead of having the test binary
// end. main returns after calling it for the same reason: os.Exit never
// returns, and a test's replacement does.
var exitProcess = os.Exit

func main() {
	opts := parseFlags()
	if err := execute(opts); err != nil {
		fmt.Fprintf(os.Stderr, "bench_resources: %v\n", err)
		exitProcess(1)
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
	flag.StringVar(&opts.clients, "clients", "", "comma-separated credential counts for the concurrency series, ascending; empty uses 1,2,5,10,20,50,100,200,500,1000 (1,2,5 with -quick)")
	flag.DurationVar(&opts.stepDuration, "step-duration", defaultStepDuration, "steady phase per series step (2s with -quick unless given)")
	flag.IntVar(&opts.memoryBudget, "memory-budget", 0, "resident set, in MiB, beyond which a series step is not started; 0 takes 80% of the host's available memory")
	flag.StringVar(&opts.profiles, "profiles", defaultProfiles, "directory the series writes its CPU and heap profiles under; empty writes none")
	flag.BoolVar(&opts.noRender, "no-render", false, "measure and write the record and profiles, then stop: for a host with no repository to render into")
	flag.Parse()

	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "json":
			opts.recordSet = true
		case "clients":
			opts.clientsSet = true
		case "step-duration":
			opts.stepDurationSet = true
		}
	})
	return opts
}

// validate refuses the flag values that would produce a record rather than an
// error.
//
// All are only checked for a run that measures. -render and -check read the
// committed record and never reach a ticker or a round, so refusing them there
// would reject a redraw over a number it does not use.
func (o options) validate() error {
	if o.render || o.check {
		return nil
	}
	if o.rounds <= 0 {
		return fmt.Errorf("-rounds must be positive, got %d", o.rounds)
	}
	// time.NewTicker panics below one nanosecond, and the value reaches it
	// through measure and runScenario, several minutes of measurement later.
	if o.sampleInterval <= 0 {
		return fmt.Errorf("-sample-interval must be positive, got %s", o.sampleInterval)
	}
	if o.stepDuration <= 0 {
		return fmt.Errorf("-step-duration must be positive, got %s", o.stepDuration)
	}
	if o.memoryBudget < 0 {
		return fmt.Errorf("-memory-budget must not be negative, got %d", o.memoryBudget)
	}
	if o.clientsSet {
		if _, err := parseSteps(o.clients); err != nil {
			return err
		}
	}
	return nil
}

// getwd is the working directory, a seam so the one failure os.Getwd has
// (a directory removed underneath the process) can be driven.
var getwd = os.Getwd

// locateRoot finds the module root, or the working directory when the run
// needs no repository.
//
// A measure-only run with a prebuilt binary reads nothing from the checkout:
// not VERSION, not the docs, not the stylesheet. That is the shape the series
// runs in on a host with no Go toolchain, so the absence of a go.mod above
// the working directory is not an error there; relative paths resolve against
// the working directory instead. Every other run does need the root, and a
// missing one is reported as before.
func locateRoot(opts options) (string, error) {
	root, err := mcpsurface.ProjectRoot()
	if err == nil {
		return root, nil
	}
	if opts.noRender && opts.binary != "" {
		cwd, wdErr := getwd()
		if wdErr != nil {
			return "", fmt.Errorf("get working directory: %w", wdErr)
		}
		return cwd, nil
	}
	return "", err
}

// execute runs the command: measure unless asked not to, then render unless
// asked not to.
func execute(opts options) error {
	if err := opts.validate(); err != nil {
		return err
	}
	root, err := locateRoot(opts)
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
		if opts.noRender {
			return nil
		}
	}

	run, err := readRun(recordPath)
	if err != nil {
		return err
	}
	return renderAll(opts, root, run)
}

// measure runs the matrix against a freshly built binary, or the one given.
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

	profilesDir := ""
	if opts.profiles != "" {
		profilesDir = resolve(root, opts.profiles)
	}
	r := &runner{
		binary:         binary,
		stub:           stub,
		otlp:           sink,
		sampleInterval: opts.sampleInterval,
		progress:       progressFunc(opts.verbose),
		report:         progressFunc(true),
		profilesDir:    profilesDir,
		budgetMiB:      memoryBudgetMiB(opts.memoryBudget),
	}

	run := &Run{
		Schema:      resultSchema,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Host:        hostInfo(),
		Settings: Settings{
			// Taken from the matrix rather than from the flag: the smoke
			// matrix pins its own rounds, and a record that reported the flag
			// would describe a run that did not happen.
			Rounds:           pointRounds(plans),
			SampleIntervalMs: int(opts.sampleInterval / time.Millisecond),
			Quick:            opts.quick,
		},
	}

	ctx := context.Background()
	started := time.Now()
	for i, plan := range plans {
		fmt.Printf("[%d/%d] %s: %s\n", i+1, len(plans), plan.ID, plan.describe())
		scenarioStarted := time.Now()
		if plan.isSeries() {
			series, runErr := r.runSeries(ctx, plan)
			if runErr != nil {
				return nil, fmt.Errorf("series %s: %w", plan.ID, runErr)
			}
			run.Series = append(run.Series, series)
			fmt.Printf("      %s\n", series.summary(time.Since(scenarioStarted)))
			continue
		}
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

// pointRounds is the rounds the point scenarios ran, for the record's
// settings; zero when the matrix holds only series, which have no rounds.
func pointRounds(plans []scenarioPlan) int {
	for _, plan := range plans {
		if !plan.isSeries() {
			return plan.Rounds
		}
	}
	return 0
}

// memoryBudgetMiB resolves the series budget: the flag when given, else
// eighty percent of what the host says is available right now. Eighty rather
// than all of it because the driver, the stand-ins and the kernel's own
// needs share the same memory, and a series that stops exactly at the last
// free page has already started swapping.
func memoryBudgetMiB(flagMiB int) float64 {
	if flagMiB > 0 {
		return float64(flagMiB)
	}
	return round(hostAvailableMemoryMiB() * defaultBudgetFraction)
}

// hostAvailableMemoryMiB is what the default budget is a share of. It is a
// variable so a test can pin the host's answer: the kernel's figure moves by a
// few kibibytes between two reads on a busy machine, and a test that read it
// once itself and once through memoryBudgetMiB compared two different hosts.
var hostAvailableMemoryMiB = availableMemoryMiB //nolint:gochecknoglobals // test seam

// defaultBudgetFraction is the share of available memory the series may
// plan up to when no budget is given.
const defaultBudgetFraction = 0.8

// matrixFor picks the matrix and refuses to overwrite the published record
// with a partial one.
//
// A -quick, -scenarios or -clients run is a development aid: writing it to
// the default path would replace measured, published numbers with a smoke
// test or a truncated series, and the page would keep claiming those were
// the figures.
func matrixFor(opts options) ([]scenarioPlan, error) {
	settings := matrixSettings{rounds: opts.rounds, steps: defaultSeriesSteps, stepDuration: opts.stepDuration}
	partial := false
	if opts.quick {
		settings.steps = quickSeriesSteps
		if !opts.stepDurationSet {
			settings.stepDuration = quickStepDuration
		}
		partial = true
	}
	if opts.clientsSet {
		steps, err := parseSteps(opts.clients)
		if err != nil {
			return nil, err
		}
		settings.steps = steps
		partial = true
	}
	plans := publishedMatrix(settings)
	if opts.quick {
		plans = quickMatrix(settings)
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
		return func(string, ...any) {
			// Empty on purpose: the quiet reporter drops the message, so no
			// call site has to test for a nil function before reporting.
		}
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
// inside it, spelled the way the repository spells it: with slashes, on
// every platform. A path outside the root is printed whole: a record written
// to /tmp is better named as /tmp than as a climb of "../" from wherever the
// checkout is.
func rel(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return path
	}
	return filepath.ToSlash(relative)
}
