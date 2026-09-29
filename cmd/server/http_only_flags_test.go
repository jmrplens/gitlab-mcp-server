package main

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"maps"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
)

// TestHTTPOnlyFlags_AreTheFlagsBoundToHTTPConfig holds [httpOnlyFlags] to the
// flags this package registers into [httpConfig], in both directions.
//
// The table is what decides whether a stdio run says anything about a flag,
// and the defect it closes is a flag accepted and ignored in silence. A flag
// added to the HTTP configuration and not to the table would be exactly that
// defect again, and nothing else would notice: the flag parses, HTTP mode
// reads it, and the stdio run that ignores it passes every test. So the set is
// read from the source, where the registrations are, rather than kept as a
// second list here. The other direction matters too: an entry for a flag that
// no longer lands in the HTTP configuration would warn a stdio user about a
// flag stdio now reads.
func TestHTTPOnlyFlags_AreTheFlagsBoundToHTTPConfig(t *testing.T) {
	bound := flagsBoundToHTTPConfig(t)
	// A floor rather than a count: the read has to have found the
	// registrations at all, or the comparison below proves nothing.
	if len(bound) < 40 {
		t.Fatalf("found %d flags registered into httpConfig; the source read is not seeing main's registrations: %v", len(bound), bound)
	}

	t.Run("every bound flag is in the table", func(t *testing.T) {
		for _, name := range bound {
			if _, listed := httpOnlyFlags[name]; !listed {
				t.Errorf("-%s is registered into httpConfig, which a stdio run never reads, but httpOnlyFlags does not name it, so a stdio run ignores it in silence", name)
			}
		}
	})
	t.Run("every table entry is a bound flag", func(t *testing.T) {
		for _, name := range slices.Sorted(maps.Keys(httpOnlyFlags)) {
			if !slices.Contains(bound, name) {
				t.Errorf("httpOnlyFlags names -%s, which is not registered into httpConfig, so a stdio run would call a flag it reads ignored", name)
			}
		}
	})
}

// flagsBoundToHTTPConfig reads the name of every flag this package's non-test
// files register with a flag.XxxVar call whose destination is a field of the
// httpConfig main parses into, spelled &hcfg.field. Every file is read rather
// than main.go alone, so a registration moved elsewhere is still seen.
func flagsBoundToHTTPConfig(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}
	fset := token.NewFileSet()
	var names []string
	for _, entry := range entries {
		file := entry.Name()
		if !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, parseErr := parser.ParseFile(fset, file, nil, 0)
		if parseErr != nil {
			t.Fatalf("parsing %s: %v", file, parseErr)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			if name, ok := hcfgFlagRegistration(t, n); ok {
				names = append(names, name)
			}
			return true
		})
	}
	slices.Sort(names)
	return names
}

// hcfgFlagRegistration reports the flag name n registers when n is a
// flag.XxxVar call whose first argument is &hcfg.<field>.
func hcfgFlagRegistration(t *testing.T, n ast.Node) (string, bool) {
	t.Helper()
	call, ok := n.(*ast.CallExpr)
	if !ok || len(call.Args) < 2 {
		return "", false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !strings.HasSuffix(sel.Sel.Name, "Var") {
		return "", false
	}
	if pkg, isIdent := sel.X.(*ast.Ident); !isIdent || pkg.Name != "flag" {
		return "", false
	}
	addr, ok := call.Args[0].(*ast.UnaryExpr)
	if !ok || addr.Op != token.AND {
		return "", false
	}
	field, ok := addr.X.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	if owner, isIdent := field.X.(*ast.Ident); !isIdent || owner.Name != "hcfg" {
		return "", false
	}
	lit, ok := call.Args[1].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		t.Errorf("a flag registered into hcfg.%s is not named by a plain string literal", field.Sel.Name)
		return "", false
	}
	name, err := strconv.Unquote(lit.Value)
	if err != nil {
		t.Errorf("unquoting the flag name %s: %v", lit.Value, err)
		return "", false
	}
	return name, true
}

// TestHTTPOnlyFlags_NameVariablesStdioReads holds the advice the startup line
// gives, "on stdio set X", to the loader stdio actually runs.
//
// Each variable the table names is set, alone, to a value its default is not,
// and [config.Load] has to come back with a different configuration than it
// does without it. A variable misspelled here, or one stdio's loader stopped
// reading, would send an operator to set something that changes nothing, which
// is the same silence the warning exists to end.
func TestHTTPOnlyFlags_NameVariablesStdioReads(t *testing.T) {
	values := map[string]string{
		"GITLAB_URL":                            "https://gitlab.example.test",
		config.EnvPrefix + "SKIP_TLS_VERIFY":    "true",
		config.EnvPrefix + "TIER":               "premium",
		config.EnvPrefix + "IGNORE_SCOPES":      "true",
		config.EnvPrefix + "TOOL_SURFACE":       config.ToolSurfaceMeta,
		config.EnvPrefix + "CAPABILITY_SURFACE": "minimal",
		config.EnvPrefix + "META_PARAM_SCHEMA":  config.MetaParamSchemaFull,
		config.EnvPrefix + "EMBEDDED_RESOURCES": "false",
		config.EnvPrefix + "EXCLUDE_TOOLS":      "gitlab_admin",
		config.EnvPrefix + "READ_ONLY":          "true",
		config.EnvPrefix + "SAFE_MODE":          "true",
		config.EnvPrefix + "ACTION_TIMEOUT":     "5m",
		config.EnvPrefix + "RATE_LIMIT_RPS":     "5",
		config.EnvPrefix + "RATE_LIMIT_BURST":   "7",
	}

	named := map[string]bool{}
	for _, entry := range httpOnlyFlags {
		if entry.stdioVariable != "" {
			named[entry.stdioVariable] = true
		}
	}
	t.Run("every named variable has a value to try", func(t *testing.T) {
		for _, variable := range slices.Sorted(maps.Keys(named)) {
			if _, ok := values[variable]; !ok {
				t.Errorf("httpOnlyFlags names %s and this test has no value for it; add one its default is not", variable)
			}
		}
	})

	for _, variable := range slices.Sorted(maps.Keys(named)) {
		value, ok := values[variable]
		if !ok {
			continue
		}
		t.Run(variable, func(t *testing.T) {
			t.Setenv(config.EnvFileVar, "")
			t.Setenv("HOME", t.TempDir())
			t.Setenv("GITLAB_TOKEN", "glpat-stdio-flags")
			for other := range values {
				t.Setenv(other, "")
			}
			t.Setenv("GITLAB_URL", "https://gitlab.example.com")
			baseline, err := config.Load()
			if err != nil {
				t.Fatalf("loading the baseline configuration: %v", err)
			}
			t.Setenv(variable, value)
			changed, err := config.Load()
			if err != nil {
				t.Fatalf("loading with %s=%s: %v", variable, value, err)
			}
			if reflect.DeepEqual(baseline, changed) {
				t.Errorf("%s=%s leaves stdio's configuration as it was, so the startup line would send an operator to a variable stdio does not read", variable, value)
			}
		})
	}
}

// TestHTTPOnlyFlags_OnlyTheProtectionsWithholdWrites pins which flags refuse a
// stdio start: --read-only and --safe-mode, each reading its own field, and
// each naming the variable its refusal tells the operator to set.
//
// Those two are the settings [config.RetiredEnvUses] refuses a retired
// spelling of, and for the same reason; any other flag ignored on stdio costs
// a setting, not a capability the deployment asked to be without.
func TestHTTPOnlyFlags_OnlyTheProtectionsWithholdWrites(t *testing.T) {
	var withholding []string
	for _, name := range slices.Sorted(maps.Keys(httpOnlyFlags)) {
		if httpOnlyFlags[name].withholds != nil {
			withholding = append(withholding, name)
		}
	}
	if want := []string{"read-only", "safe-mode"}; !slices.Equal(withholding, want) {
		t.Fatalf("flags that withhold writes = %v, want %v", withholding, want)
	}

	cases := []struct {
		flag string
		own  httpConfig
		peer httpConfig
	}{
		{flag: "read-only", own: httpConfig{readOnly: true}, peer: httpConfig{safeMode: true}},
		{flag: "safe-mode", own: httpConfig{safeMode: true}, peer: httpConfig{readOnly: true}},
	}
	for _, tc := range cases {
		t.Run(tc.flag, func(t *testing.T) {
			entry := httpOnlyFlags[tc.flag]
			if !entry.withholds(&tc.own) {
				t.Errorf("--%s set does not read as withholding writes", tc.flag)
			}
			if entry.withholds(&tc.peer) {
				t.Errorf("--%s reads the other protection's field", tc.flag)
			}
			if entry.withholds(&httpConfig{}) {
				t.Errorf("--%s=false reads as withholding writes", tc.flag)
			}
			if entry.stdioVariable == "" {
				t.Errorf("--%s names no variable, so its refusal cannot say what to set instead", tc.flag)
			}
		})
	}
}

// TestStdioIgnoredFlags_SortsRefusalsFromTheRest covers the split: which of
// the flags a run was given only HTTP mode reads, which of those refuse the
// start, and the order they are named in.
func TestStdioIgnoredFlags_SortsRefusalsFromTheRest(t *testing.T) {
	cases := []struct {
		name        string
		hcfg        httpConfig
		wantRefuse  []string
		wantIgnored []string
	}{
		{name: "nothing passed", hcfg: httpConfig{}},
		{
			name: "only flags stdio reads",
			hcfg: httpConfig{setFlags: map[string]bool{"transport": true, "log-level": true, "telemetry": true}},
		},
		{
			name:        "HTTP-only flags are named in order",
			hcfg:        httpConfig{setFlags: map[string]bool{"rate-limit-rps": true, "http-addr": true, "log-level": true}},
			wantIgnored: []string{"http-addr", "rate-limit-rps"},
		},
		{
			name:       "read-only asking for reads is refused",
			hcfg:       httpConfig{setFlags: map[string]bool{"read-only": true}, readOnly: true},
			wantRefuse: []string{"read-only"},
		},
		{
			name:        "read-only=false asks for nothing and is only named",
			hcfg:        httpConfig{setFlags: map[string]bool{"read-only": true}},
			wantIgnored: []string{"read-only"},
		},
		{
			name:        "a refusal and an ignored flag together",
			hcfg:        httpConfig{setFlags: map[string]bool{"tool-surface": true, "safe-mode": true}, safeMode: true},
			wantRefuse:  []string{"safe-mode"},
			wantIgnored: []string{"tool-surface"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			refuse, ignored := stdioIgnoredFlags(&tc.hcfg)
			if !slices.Equal(refuse, tc.wantRefuse) {
				t.Errorf("refuse = %v, want %v", refuse, tc.wantRefuse)
			}
			if !slices.Equal(ignored, tc.wantIgnored) {
				t.Errorf("ignored = %v, want %v", ignored, tc.wantIgnored)
			}
		})
	}
}

// TestDescribeIgnoredFlag_NamesTheStdioVariableWhereThereIsOne covers both
// spellings of an ignored flag on the startup line.
func TestDescribeIgnoredFlag_NamesTheStdioVariableWhereThereIsOne(t *testing.T) {
	cases := map[string]string{
		"rate-limit-rps": "--rate-limit-rps (on stdio set GITLAB_MCP_RATE_LIMIT_RPS)",
		"gitlab-url":     "--gitlab-url (on stdio set GITLAB_URL)",
		"http-addr":      "--http-addr (stdio has no such setting)",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			if got := describeIgnoredFlag(name); got != want {
				t.Errorf("describeIgnoredFlag(%q) = %q, want %q", name, got, want)
			}
		})
	}
}

// loggedRecords installs a JSON logger over a buffer for the rest of the test
// and returns a function that decodes what it has recorded, one map per line.
func loggedRecords(t *testing.T) func() []map[string]any {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return func() []map[string]any {
		var records []map[string]any
		for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
			if line == "" {
				continue
			}
			var record map[string]any
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				t.Fatalf("a log line is not JSON: %q: %v", line, err)
			}
			records = append(records, record)
		}
		return records
	}
}

// TestReportStdioIgnoredFlags_SaysWhatARunIgnoresAndRefusesWhatItCannot drives
// the reporting through each shape of run it distinguishes, reading the level,
// the flags and the verdict it leaves behind.
//
// Not parallel: it replaces the default logger.
func TestReportStdioIgnoredFlags_SaysWhatARunIgnoresAndRefusesWhatItCannot(t *testing.T) {
	stdio := transportDecision{}
	auto := transportDecision{Inference: "stdin is a pipe"}
	httpRun := transportDecision{HTTP: true}

	cases := []struct {
		name      string
		choice    transportDecision
		hcfg      httpConfig
		wantStart bool
		// wantLevels is the level of every line written, in order.
		wantLevels []string
		// wantFlags is the flags attribute of the first line, when there is one.
		wantFlags []any
	}{
		{name: "an HTTP run reads them all", choice: httpRun, hcfg: httpConfig{setFlags: map[string]bool{"read-only": true}, readOnly: true}, wantStart: true},
		{name: "a stdio run given none", choice: stdio, hcfg: httpConfig{setFlags: map[string]bool{"log-level": true}}, wantStart: true},
		{
			name: "a stdio run the operator chose warns", choice: stdio,
			hcfg:      httpConfig{setFlags: map[string]bool{"rate-limit-rps": true, "http-addr": true}},
			wantStart: true, wantLevels: []string{"WARN"},
			wantFlags: []any{"--http-addr (stdio has no such setting)", "--rate-limit-rps (on stdio set GITLAB_MCP_RATE_LIMIT_RPS)"},
		},
		{
			name: "a stdio run auto chose only informs", choice: auto,
			hcfg:      httpConfig{setFlags: map[string]bool{"http-addr": true}},
			wantStart: true, wantLevels: []string{"INFO"},
			wantFlags: []any{"--http-addr (stdio has no such setting)"},
		},
		{
			name: "read-only is refused", choice: stdio,
			hcfg:       httpConfig{setFlags: map[string]bool{"read-only": true}, readOnly: true},
			wantLevels: []string{"ERROR"},
		},
		{
			name: "safe-mode is refused under auto too", choice: auto,
			hcfg:       httpConfig{setFlags: map[string]bool{"safe-mode": true, "tier": true}, safeMode: true},
			wantLevels: []string{"INFO", "ERROR"},
			wantFlags:  []any{"--tier (on stdio set GITLAB_MCP_TIER)"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			records := loggedRecords(t)

			if got := reportStdioIgnoredFlags(tc.choice, &tc.hcfg); got != tc.wantStart {
				t.Errorf("reportStdioIgnoredFlags() = %v, want %v", got, tc.wantStart)
			}

			logged := records()
			// Each level carries one message, so a line at the right level
			// with another line's text is caught as well as a wrong level.
			lineAt := map[string]string{
				"WARN":  stdioIgnoredFlagsLine,
				"INFO":  stdioAutoIgnoredFlagsLine,
				"ERROR": stdioWithheldWritesLine,
			}
			var levels []string
			for _, record := range logged {
				level, _ := record["level"].(string)
				levels = append(levels, level)
				if msg, _ := record["msg"].(string); msg != lineAt[level] {
					t.Errorf("the %s line reads %q, want %q", level, msg, lineAt[level])
				}
			}
			if !slices.Equal(levels, tc.wantLevels) {
				t.Fatalf("levels logged = %v, want %v: %v", levels, tc.wantLevels, logged)
			}
			if tc.wantFlags != nil {
				if got, _ := logged[0]["flags"].([]any); !reflect.DeepEqual(got, tc.wantFlags) {
					t.Errorf("flags = %v, want %v", got, tc.wantFlags)
				}
			}
		})
	}
}

// TestReportStdioIgnoredFlags_RefusalNamesTheFlagAndWhatToSet reads the
// refusal line itself: the flag it refused, the variable to set in its place,
// and a message that says the server was not started.
func TestReportStdioIgnoredFlags_RefusalNamesTheFlagAndWhatToSet(t *testing.T) {
	records := loggedRecords(t)
	hcfg := httpConfig{setFlags: map[string]bool{"read-only": true, "safe-mode": true}, readOnly: true, safeMode: true}

	if reportStdioIgnoredFlags(transportDecision{}, &hcfg) {
		t.Fatal("reportStdioIgnoredFlags() = true for a stdio run asked to hold back writes, want the start refused")
	}

	logged := records()
	if len(logged) != 2 {
		t.Fatalf("logged %d lines, want one refusal per flag: %v", len(logged), logged)
	}
	for i, want := range []struct{ flag, set string }{
		{"--read-only", "GITLAB_MCP_READ_ONLY=true"},
		{"--safe-mode", "GITLAB_MCP_SAFE_MODE=true"},
	} {
		t.Run(want.flag, func(t *testing.T) {
			if got := logged[i]["flag"]; got != want.flag {
				t.Errorf("flag = %v, want %q", got, want.flag)
			}
			if got := logged[i]["set_instead"]; got != want.set {
				t.Errorf("set_instead = %v, want %q", got, want.set)
			}
			if msg, _ := logged[i]["msg"].(string); !strings.Contains(msg, "will not be started") {
				t.Errorf("the refusal does not say the deployment was not started: %q", msg)
			}
			if level, _ := logged[i]["level"].(string); level != "ERROR" {
				t.Errorf("the refusal is logged at %s, want ERROR", level)
			}
		})
	}
}

// TestMain_StdioRunGivenReadOnly_IsRefusedBeforeServing drives the refusal
// through main, which is the only place the flags a run was given and the
// transport it chose meet: a stdio run with --read-only and credentials that
// would otherwise start exits 1, having said why, and never serves.
//
// stdin is a closed pipe, so the run is not mistaken for a person at a
// terminal and a serve that did start would end at once rather than hang.
func TestMain_StdioRunGivenReadOnly_IsRefusedBeforeServing(t *testing.T) {
	withFreshFlagSet(t)
	t.Setenv(config.EnvFileVar, "")
	t.Setenv("GITLAB_URL", "https://gitlab.example.test")
	t.Setenv("GITLAB_TOKEN", testToken)
	t.Setenv("GITLAB_MCP_LOG_LEVEL", "")
	t.Setenv("GITLAB_MCP_TELEMETRY", "")
	closedStdin(t)

	var exits []int
	originalArgs, originalExit := os.Args, exitProcess
	originalLogger, originalBase := slog.Default(), baseLogHandler
	os.Args = []string{"gitlab-mcp-server", "-read-only"}
	exitProcess = func(code int) { exits = append(exits, code) }
	t.Cleanup(func() {
		os.Args, exitProcess = originalArgs, originalExit
		slog.SetDefault(originalLogger)
		baseLogHandler = originalBase
	})
	stderr := captureStderr(t)

	main()

	logged := stderr()
	if !slices.Equal(exits, []int{1}) {
		t.Fatalf("exit codes = %v, want [1] for a stdio run given --read-only\nstderr: %s", exits, logged)
	}
	for _, want := range []string{`"flag":"--read-only"`, `"set_instead":"GITLAB_MCP_READ_ONLY=true"`} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(logged, want) {
				t.Errorf("stderr does not carry %s:\n%s", want, logged)
			}
		})
	}
	if strings.Contains(logged, "starting MCP server") {
		t.Errorf("the refused run started serving:\n%s", logged)
	}
}
