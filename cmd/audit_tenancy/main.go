package main

import (
	"fmt"
	"io"
	"os"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sourcegate"
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
	// exempt is the exemption table; a test replaces it.
	exempt map[string]exemption
}

// productionConfig is the run `make check-tenancy` makes over the tree at dir.
func productionConfig(dir string) auditConfig {
	return auditConfig{
		dir:      dir,
		patterns: defaultPatterns,
		register: productionRegister(),
		rules:    productionRules(),
		exempt:   notADecision,
	}
}

func main() {
	sourcegate.Main(runMain)
}

// runMain parses the command line and runs what it describes, returning the
// process exit code: 0 for a request for help, 2 for arguments it cannot
// parse or patterns it is given, and otherwise what [run] or [runCompare]
// decides.
func runMain(args []string, stdout, stderr io.Writer) int {
	cmd := sourcegate.NewCommand(toolName, sourcegate.Usage{
		Dir:     "repository root the packages are loaded from",
		Check:   "exit non-zero when the register and the code disagree",
		Verbose: "also list what the exemption table answered",
	}, stderr)
	compare := cmd.Flags.Bool("compare-binaries", false,
		"compare two ELF binaries given as arguments: every allocated section but .gopclntab must be byte-identical")
	if code, goOn := cmd.Parse(args); !goOn {
		return code
	}
	if *compare {
		return runCompare(cmd.Flags.Args(), stdout, stderr)
	}
	if cmd.Flags.NArg() > 0 {
		fmt.Fprintf(stderr, "%s: unexpected arguments %q: the gate always loads the server and internal/\n", toolName, cmd.Flags.Args())
		return 2
	}
	return run(productionConfig(cmd.Dir), cmd.Check, cmd.Verbose, stdout, stderr)
}

// run audits, reports, and returns the process exit code [sourcegate.Finish]
// decides: a run that could not be made fails on stderr, and one that found
// something says so on stdout and fails only under -check.
func run(cfg auditConfig, check, verbose bool, stdout, stderr io.Writer) int {
	report, err := audit(cfg)
	return sourcegate.Finish(toolName, report, err, check, verbose, stdout, stderr)
}

// audit loads the program and holds the register to it.
func audit(cfg auditConfig) (Report, error) {
	p, err := loadProgram(cfg)
	if err != nil {
		return Report{}, err
	}
	g := &gate{
		p: p, reg: cfg.register, rules: cfg.rules, exempt: cfg.exempt,
		used:    map[string]string{},
		aliases: aliasIndex(cfg.register), declared: declaredKeys(cfg.register),
	}
	return g.run(), nil
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

// loadProgram loads and indexes the packages a run reads, naming files below
// the root the load resolved.
func loadProgram(cfg auditConfig) (*program, error) {
	env := append(os.Environ(), "GOOS="+judgedGOOS, "GOARCH="+judgedGOARCH)
	root, loaded, err := goprogram.LoadRooted(cfg.dir, cfg.patterns, goprogram.Options{Overlay: cfg.overlay, Env: env})
	if err != nil {
		return nil, err
	}
	return newProgram(root, loaded, cfg.rules.leave), nil
}

// gate is one run's state: the program, what it is held to, and which
// exemptions answered something.
type gate struct {
	p      *program
	reg    register
	rules  rules
	exempt map[string]exemption
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
func (g *gate) run() Report {
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
	found = append(found, g.checkExemptions()...)
	found = sortFindings(found)

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
			Exempted: len(excused),
		},
		Findings: found,
		Excused:  excused,
	}
}
