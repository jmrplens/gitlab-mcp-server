package main

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// fixtureDir is where the in-memory fixture program pretends to live. Nothing
// is written there: its packages exist only in the loader overlay, so every
// rule is exercised on real type-checked source without Go files landing in
// the repository or in the package count the README reports.
const fixtureDir = "cmd/audit_tenancy/fixture"

// The fixture's two packages: a register leaf standing in for
// internal/tenancy, and a package of sites standing in for the layers.
const (
	leafDir    = fixtureDir + "/leaf"
	siteDir    = fixtureDir + "/site"
	sitePath   = goprogram.ModulePath + "/" + siteDir
	leafImport = `"` + goprogram.ModulePath + "/" + leafDir + `"`
)

// leafValues and leafCodes are the fixture register's values: an untyped
// integer, a duration, an untyped float and a code, the four shapes the real
// register's constants take.
const (
	leafValues = `package leaf

import "time"

const (
	Limit  = 64
	Window = 30 * time.Second
	Ratio  = 0.2
)
`
	leafCodes = `package leaf

const CodeBusy = -32000
`
)

// siteHeader opens a site file: the package, the register it reads, and time,
// which the register imports and which the package standing in for the server
// therefore has to import too.
const siteHeader = `package site

import (
	"time"

	leaf ` + leafImport + `
)

var (
	_ = leaf.Limit
	_ = time.Second
)
`

// repoRoot is the module root, found from the test's working directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s", dir)
		}
		dir = parent
	}
}

// fixtureRules are the production rules pointed at the fixture: its own gate
// failure type, tool result, charge plumbing, server and settings list.
func fixtureRules() rules {
	r := productionRules()
	r.leave = nil
	r.gateType = sitePath + ".gateFailure"
	r.refusalTypes = append(r.refusalTypes[:2:2], refusalType{
		name: r.gateType, code: "code", status: "status", message: "message", header: "header",
	})
	r.toolResult = sitePath + ".result"
	r.lowerCharges = []string{sitePath + ".budget.spend"}
	r.chargeCallers = []string{siteDir + ":gate.charge"}
	r.server = siteDir
	r.envNames = tenancy.Site{Pkg: siteDir, Name: "prefixedNames"}
	// No register file is held to a reader unless a case says so, since
	// the fixture's rows rarely alias the whole leaf.
	r.valueFiles = nil
	r.ruleFiles = nil
	return r
}

// fixture is one in-memory program with the register it is held to.
type fixture struct {
	// files are the sources, keyed by their path under fixtureDir. The leaf's
	// values and codes are supplied unless a test names them.
	files   map[string]string
	rows    []tenancy.Decision
	fails   []tenancy.Failure
	derive  []derivation
	rules   *rules
	exempt  map[string]exemption
	pending []string
	// validate replaces the register's validators, which the fixture's rows
	// would never satisfy.
	validate error
}

// config builds the run a fixture describes.
func (f fixture) config(t *testing.T) auditConfig {
	t.Helper()
	root := repoRoot(t)
	files := map[string]string{"leaf/values.go": leafValues, "leaf/codes.go": leafCodes}
	maps.Copy(files, f.files)
	overlay := map[string][]byte{}
	for name, source := range files {
		overlay[filepath.Join(root, filepath.FromSlash(fixtureDir), filepath.FromSlash(name))] = []byte(source)
	}
	r := fixtureRules()
	if f.rules != nil {
		r = *f.rules
	}
	validate := f.validate
	return auditConfig{
		dir:      root,
		patterns: []string{"./" + fixtureDir + "/..."},
		overlay:  overlay,
		register: register{
			leaf:             leafDir,
			decisions:        f.rows,
			failures:         f.fails,
			derivations:      f.derive,
			validate:         func([]tenancy.Decision) error { return validate },
			validateFailures: func([]tenancy.Failure) error { return nil },
		},
		rules:   r,
		exempt:  f.exempt,
		pending: f.pending,
	}
}

// run audits the fixture and returns the report.
func (f fixture) run(t *testing.T) Report {
	t.Helper()
	report, err := audit(f.config(t))
	if err != nil {
		t.Fatalf("audit fixture: %v", err)
	}
	return report
}

// findings renders the report's findings of one rule as "subject: message",
// the way most cases hold them.
func findings(report Report, rule string) []string {
	var out []string
	for _, f := range report.Findings {
		if f.Rule == rule {
			out = append(out, f.Subject+": "+f.Message)
		}
	}
	return out
}

// assertFindings holds one rule's findings to exactly want, whatever their
// order: the report orders them by source position, which is not what a case
// is about.
func assertFindings(t *testing.T, report Report, rule string, want ...string) {
	t.Helper()
	got := findings(report, rule)
	slices.Sort(got)
	want = slices.Clone(want)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("%s findings:\n got  %q\n want %q\nall findings: %v", rule, got, want, report.Findings)
	}
}

// row is a minimal decision with the given sites.
func row(id string, sites ...tenancy.Site) tenancy.Decision {
	return tenancy.Decision{ID: id, Kind: tenancy.Rule, Disposition: tenancy.Ruled, Sites: sites}
}

// site names a declaration of the fixture's site package in a role.
func site(name string, role tenancy.Role) tenancy.Site {
	return tenancy.Site{Pkg: siteDir, Name: name, Role: role}
}

// TestRun_AFinding_IsReportedAndFailsOnlyUnderCheck: a report is worth having
// without the gate, so a plain run prints the finding and exits 0, and -check
// turns the same finding into exit 1. Nothing goes to stderr, since a finding
// is not a broken run.
func TestRun_AFinding_IsReportedAndFailsOnlyUnderCheck(t *testing.T) {
	cfg := fixture{
		files: map[string]string{"site/site.go": siteHeader},
		rows:  []tenancy.Decision{row("ROW-001", site("gone", tenancy.Enforce))},
	}.config(t)
	for _, check := range []bool{false, true} {
		t.Run(map[bool]string{false: "report", true: "check"}[check], func(t *testing.T) {
			var stdout, stderr strings.Builder
			code := run(cfg, check, false, &stdout, &stderr)
			if want := map[bool]int{false: 0, true: 1}[check]; code != want {
				t.Fatalf("exit = %d, want %d", code, want)
			}
			wantOut := "G1 ROW-001: names " + siteDir + ":gone, which matches nothing: " + siteDir + " declares nothing named gone\n" +
				"audit_tenancy: 1 rows, 0 failures and 1 declared sites over 2 packages " +
				"(0 refusal returns, 0 refusals, 0 reasons and 0 settings read); 1 findings, 0 rows pending, 0 declarations exempted\n"
			if stdout.String() != wantOut {
				t.Fatalf("stdout = %q, want %q", stdout.String(), wantOut)
			}
			if stderr.String() != "" {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

// TestRun_SourceThatDoesNotTypeCheck_FailsOnStderr: a package that did not
// type-check resolves nothing, so a report over it would be a clean answer
// about source nobody understood. The loader refuses it and the run says so
// on stderr with no report.
func TestRun_SourceThatDoesNotTypeCheck_FailsOnStderr(t *testing.T) {
	cfg := fixture{files: map[string]string{"site/site.go": "package site\n\nvar x = undefinedSymbol\n"}}.config(t)
	var stdout, stderr strings.Builder
	if code := run(cfg, false, false, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.HasPrefix(stderr.String(), toolName+": ") || stdout.String() != "" {
		t.Fatalf("stdout = %q, stderr = %q, want only the refusal on stderr", stdout.String(), stderr.String())
	}
}

// TestAudit_RootThatCannotBeResolved_FailsRatherThanLoading: the root is made
// absolute first, so a run whose working directory is gone stops there
// instead of naming files against wherever the process happens to be. Only
// Linux can be made to fail that way, and the case is skipped elsewhere.
func TestAudit_RootThatCannotBeResolved_FailsRatherThanLoading(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "gone")
	if err := os.Mkdir(gone, 0o750); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	t.Chdir(gone)
	if err := os.Remove(gone); err != nil {
		t.Skipf("this platform will not remove the working directory: %v", err)
	}
	if _, err := os.Getwd(); err == nil {
		t.Skip("this platform's getcwd still answers after the working directory is removed")
	}
	if _, err := audit(auditConfig{dir: ".", patterns: []string{"./..."}}); err == nil {
		t.Fatal("audit error = nil, want the unresolved root")
	}
}

// TestRunMain_ParsesTheCommandLine holds what each argument does: an unknown
// flag and a stray argument are usage errors, a help request is not a
// failure, and -compare-binaries hands its arguments to the comparison.
func TestRunMain_ParsesTheCommandLine(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStderr string
	}{
		{name: "unknown flag", args: []string{"-nope"}, wantCode: 2, wantStderr: "flag provided but not defined"},
		{name: "help", args: []string{"-h"}, wantCode: 0, wantStderr: "Usage of audit_tenancy"},
		{name: "stray argument", args: []string{"./internal/..."}, wantCode: 2, wantStderr: "unexpected arguments"},
		{name: "compare with one binary", args: []string{"-compare-binaries", "a"}, wantCode: 2, wantStderr: "takes two binaries"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			if code := runMain(tt.args, &stdout, &stderr); code != tt.wantCode {
				t.Fatalf("exit = %d, want %d; stderr = %q", code, tt.wantCode, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Fatalf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

// TestRunMain_TheTree_PassesTheGate is the gate on this repository, the run
// `make check-tenancy` makes: every row, failure and derivation of the real
// register against the real server and internal/, with the real exemption
// table and pending list. It is what says the tables in declarations.go
// describe the tree today, and the summary line is held so a rule that stops
// reading anything is a failure rather than a quiet pass. Every value layer
// has moved its rows, so the list is empty and no row is deferred.
//
// It runs with the environment naming Windows on arm64, which is what a
// Windows host's toolchain would be told: the verdict must not depend on the
// host, and a load that followed the environment would leave out every file
// constrained away from Windows, a declaration the exemption table names
// among them.
func TestRunMain_TheTree_PassesTheGate(t *testing.T) {
	t.Setenv("GOOS", "windows")
	t.Setenv("GOARCH", "arm64")
	var stdout, stderr strings.Builder
	if code := runMain([]string{"-dir", repoRoot(t), "-check", "-v"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	for name, want := range map[string]string{
		"exempted literal":    "cmd/server:readinessGate.abandoned: not a decision (literals, server-state): ",
		"exempted name":       "internal/toolutil:PollMaxTimeout: not a decision (names, tool-argument): ",
		"verdict":             "; 0 findings, 0 rows pending, 51 declarations exempted\n",
		"what the rules read": "(21 refusal returns, 70 refusals, 10 reasons and 31 settings read)",
	} {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(out, want) {
				t.Fatalf("stdout does not contain %q:\n%s", want, out)
			}
		})
	}
	t.Run("no pending rows", func(t *testing.T) {
		if strings.Contains(out, "pending (") {
			t.Fatalf("stdout lists pending rows, and every value layer has moved its own:\n%s", out)
		}
	})
}

// sortedPending is the pending list in the order the report prints it.
func sortedPending() []string {
	out := slices.Clone(pending)
	slices.Sort(out)
	return out
}

// TestMain_ExitsWithTheCodeRunMainDecided: main hands runMain's code to the
// process and does nothing else.
func TestMain_ExitsWithTheCodeRunMainDecided(t *testing.T) {
	previous := exitProcess
	t.Cleanup(func() { exitProcess = previous })
	var codes []int
	exitProcess = func(code int) { codes = append(codes, code) }
	previousArgs := os.Args
	t.Cleanup(func() { os.Args = previousArgs })
	os.Args = []string{toolName, "-nope"}

	main()

	if !slices.Equal(codes, []int{2}) {
		t.Fatalf("exit codes = %v, want the 2 runMain returned for an unknown flag", codes)
	}
}

// TestProductionRegister_IsTheRegisterAsItIs: the gate reads the rows, the
// failures, every key that names a derivation and the two validators of
// internal/tenancy, and nothing of its own.
func TestProductionRegister_IsTheRegisterAsItIs(t *testing.T) {
	reg := productionRegister()
	if reg.leaf != "internal/tenancy" || len(reg.decisions) != len(tenancy.Decisions()) || len(reg.failures) != len(tenancy.Failures()) {
		t.Fatalf("register = %s with %d rows and %d failures, want internal/tenancy's", reg.leaf, len(reg.decisions), len(reg.failures))
	}
	var keys []string
	for _, d := range reg.derivations {
		keys = append(keys, d.key)
	}
	want := []string{"credential", "entry", "owner", "tenant", "verified", "application", "session", "address", "source", "refused"}
	if !slices.Equal(keys, want) {
		t.Fatalf("derivations name keys %v, want %v", keys, want)
	}
	if err := reg.validate(reg.decisions); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if err := reg.validateFailures([]tenancy.Failure{{Kind: "k", Charged: true}}); err == nil {
		t.Fatal("validateFailures accepted a charged failure nobody caused, so it is not tenancy.ValidateFailures")
	}
}

// TestDefaultPatterns_AreTheServerAndInternal states what the gate loads, so
// narrowing it is a visible change rather than a quiet one.
func TestDefaultPatterns_AreTheServerAndInternal(t *testing.T) {
	if want := []string{"./cmd/server", "./internal/..."}; !slices.Equal(defaultPatterns, want) {
		t.Fatalf("defaultPatterns = %v, want %v", defaultPatterns, want)
	}
	cfg := productionConfig("root")
	if cfg.dir != "root" || !slices.Equal(cfg.pending, pending) || len(cfg.exempt) != len(notADecision) {
		t.Fatalf("productionConfig = %+v, want the tree at root held to the real tables", cfg)
	}
}

// errFixture is a validator's refusal, for the cases that need one.
var errFixture = errors.New("ROW-001: rule (INV-000): first\nROW-002: rule (INV-000): second")
