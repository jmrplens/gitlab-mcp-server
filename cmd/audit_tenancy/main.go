package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// toolName is how the report names itself.
const toolName = "audit_tenancy"

// defaultPatterns are the server and every package under internal/, which is
// every package a register site can name. The test-support packages under
// internal/ that the server never links are loaded with the rest and left out
// before any rule reads them (see [rules.leave]).
var defaultPatterns = []string{"./cmd/server", "./internal/..."}

// register is what the gate holds the code to: the rows, the failure table,
// the keys' derivations and the two validators, and where the register itself
// lives. Production takes all of it from internal/tenancy; a test hands the
// gate a register of its own over a fixture package.
type register struct {
	// leaf is the module-relative directory of the register package.
	leaf             string
	decisions        []tenancy.Decision
	failures         []tenancy.Failure
	derivations      []derivation
	validate         func([]tenancy.Decision) error
	validateFailures func([]tenancy.Failure) error
}

// derivation is the symbols that compute a value of one key.
type derivation struct {
	key   string
	sites []tenancy.Site
}

// productionRegister is internal/tenancy as it is.
func productionRegister() register {
	var derivations []derivation
	for _, k := range tenancy.Keys() {
		if sites := k.Derivation(); len(sites) > 0 {
			derivations = append(derivations, derivation{key: k.String(), sites: sites})
		}
	}
	return register{
		leaf:             "internal/tenancy",
		decisions:        tenancy.Decisions(),
		failures:         tenancy.Failures(),
		derivations:      derivations,
		validate:         tenancy.Validate,
		validateFailures: tenancy.ValidateFailures,
	}
}

// auditConfig is one configured run.
type auditConfig struct {
	dir      string
	patterns []string
	// overlay supplies source that is not on disk, which is how a test hands
	// the gate a fixture program. Production passes nil.
	overlay  map[string][]byte
	register register
	rules    rules
	// exempt is the exemption table and pending the rows whose binding
	// checks are deferred; a test replaces both.
	exempt  map[string]exemption
	pending []string
}

// productionConfig is the run `make check-tenancy` makes over the tree at dir.
func productionConfig(dir string) auditConfig {
	return auditConfig{
		dir:      dir,
		patterns: defaultPatterns,
		register: productionRegister(),
		rules:    productionRules(),
		exempt:   notADecision,
		pending:  pending,
	}
}

// exitProcess is [os.Exit] behind a seam, so the code [runMain] decided is a
// value a test can read rather than the end of the test binary.
var exitProcess = os.Exit

func main() {
	exitProcess(runMain(os.Args[1:], os.Stdout, os.Stderr))
}

// runMain parses the command line and runs what it describes, returning the
// process exit code: 2 for arguments it cannot parse.
func runMain(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(toolName, flag.ContinueOnError)
	flags.SetOutput(stderr)
	dir := flags.String("dir", ".", "repository root the packages are loaded from")
	check := flags.Bool("check", false, "exit non-zero when the register and the code disagree")
	verbose := flags.Bool("v", false, "also list what the exemption table answered")
	compare := flags.Bool("compare-binaries", false,
		"compare two ELF binaries given as arguments: every allocated section but .gopclntab must be byte-identical")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *compare {
		return runCompare(flags.Args(), stdout, stderr)
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "%s: unexpected arguments %q: the gate always loads the server and internal/\n", toolName, flags.Args())
		return 2
	}
	return run(productionConfig(*dir), *check, *verbose, stdout, stderr)
}

// run audits, reports, and returns the process exit code.
//
// Exit 1 covers two different things on purpose, told apart by which stream
// spoke: a run that could not be made at all says so on stderr, and a run that
// found something says it on stdout and only fails under -check. A report is
// worth having without a gate.
func run(cfg auditConfig, check, verbose bool, stdout, stderr io.Writer) int {
	report, err := audit(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
		return 1
	}
	report.write(stdout, verbose)
	if check && !report.ok() {
		return 1
	}
	return 0
}

// audit loads the program and holds the register to it.
//
// The root is made absolute before anything is loaded, so a run whose working
// directory is gone stops on that error instead of naming files against
// wherever the process happens to be.
func audit(cfg auditConfig) (Report, error) {
	p, err := loadProgram(cfg)
	if err != nil {
		return Report{}, err
	}
	g := &gate{
		p: p, reg: cfg.register, rules: cfg.rules, exempt: cfg.exempt,
		pending: map[string]bool{}, used: map[string]string{},
		aliases: aliasIndex(cfg.register), declared: declaredKeys(cfg.register),
	}
	for _, id := range cfg.pending {
		g.pending[id] = true
	}
	return g.run(cfg.pending), nil
}

// The platform the gate judges the program on. A file behind a build
// constraint is loaded or left out by the platform the toolchain is told, and
// a load that took the host's would give a verdict that depends on who runs
// it: a site or an exemption naming a declaration in a file constrained away
// from the host would match nothing there, and fail the gate on Windows while
// it passes on Linux. So the load names the platform the server is served
// from, and a file constrained to another one is not read.
const (
	judgedGOOS   = "linux"
	judgedGOARCH = "amd64"
)

// loadProgram loads and indexes the packages a run reads.
func loadProgram(cfg auditConfig) (*program, error) {
	root, err := filepath.Abs(cfg.dir)
	if err != nil {
		return nil, err
	}
	env := append(os.Environ(), "GOOS="+judgedGOOS, "GOARCH="+judgedGOARCH)
	loaded, err := goprogram.LoadWith(cfg.dir, cfg.patterns, goprogram.Options{Overlay: cfg.overlay, Env: env})
	if err != nil {
		return nil, err
	}
	return newProgram(root, loaded, cfg.rules.leave), nil
}

// gate is one run's state: the program, what it is held to, and which
// exemptions answered something.
type gate struct {
	p       *program
	reg     register
	rules   rules
	exempt  map[string]exemption
	pending map[string]bool
	// used records, for each exemption that answered something, which part
	// of G10 it answered.
	used map[string]string
	// leafUsed are the register's names something outside it refers to,
	// built on the first question G6 asks.
	leafUsed map[string]bool
	// aliases maps each declared Alias to the constants it carries, and
	// declared is every declaration the register names in any role.
	aliases  map[string]map[string]bool
	declared map[string]bool
	// read counts what the behavioral rules read, for the summary.
	read struct{ returns, refusals, reasons, settings int }
}

// run applies every rule and assembles the report.
func (g *gate) run(pending []string) Report {
	var found []Finding
	found = append(found, checkResolve(g.p, g.reg, g.exempt)...)
	found = append(found, g.checkAliases()...)
	found = append(found, g.checkArgs()...)
	found = append(found, g.checkPins()...)
	found = append(found, g.checkRefs()...)
	found = append(found, g.checkOrphans()...)
	found = append(found, g.checkCharges()...)
	found = append(found, g.checkRefusals()...)
	found = append(found, g.checkReasons()...)
	found = append(found, g.checkTripwire()...)
	found = append(found, g.checkValidate()...)
	found = append(found, g.checkLeaf()...)
	found = append(found, g.checkShareWords()...)
	found = append(found, g.checkConfig()...)
	found = append(found, g.checkPending()...)
	found = append(found, g.checkExemptions()...)
	found = sortFindings(found)

	sortedPending := slices.Clone(pending)
	slices.Sort(sortedPending)
	var excused []Excuse
	for _, key := range sortedKeys(g.used) {
		e := g.exempt[key]
		excused = append(excused, Excuse{Key: key, Part: g.used[key], Category: e.category, Reason: e.reason})
	}
	return Report{
		Summary: Summary{
			Rows:     len(g.reg.decisions),
			Failures: len(g.reg.failures),
			Sites:    len(g.declared),
			Packages: len(g.p.packages),
			Returns:  g.read.returns,
			Refusals: g.read.refusals,
			Reasons:  g.read.reasons,
			Settings: g.read.settings,
			Findings: len(found),
			Pending:  len(pending),
			Exempted: len(excused),
		},
		Findings: found,
		Pending:  sortedPending,
		Excused:  excused,
	}
}
