package main

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	bound := slices.Sorted(maps.Keys(flagFieldsBoundToHTTPConfig(t)))
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

// flagFieldsBoundToHTTPConfig reads every flag this package's non-test files
// register with a flag.XxxVar call whose destination is a field of the
// httpConfig main parses into, spelled &hcfg.field, and returns the field
// each flag writes, keyed by the flag's name. Every file is read rather than
// main.go alone, so a registration moved elsewhere is still seen.
func flagFieldsBoundToHTTPConfig(t *testing.T) map[string]string {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}
	fset := token.NewFileSet()
	fields := map[string]string{}
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
			if name, field, ok := hcfgFlagRegistration(t, n); ok {
				fields[name] = field
			}
			return true
		})
	}
	return fields
}

// hcfgFlagRegistration reports the flag name n registers, and the field of
// hcfg it writes, when n is a flag.XxxVar call whose first argument is
// &hcfg.<field>.
func hcfgFlagRegistration(t *testing.T, n ast.Node) (name, field string, ok bool) {
	t.Helper()
	call, isCall := n.(*ast.CallExpr)
	if !isCall || len(call.Args) < 2 {
		return "", "", false
	}
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if !isSel || !strings.HasSuffix(sel.Sel.Name, "Var") {
		return "", "", false
	}
	if pkg, isIdent := sel.X.(*ast.Ident); !isIdent || pkg.Name != "flag" {
		return "", "", false
	}
	addr, isAddr := call.Args[0].(*ast.UnaryExpr)
	if !isAddr || addr.Op != token.AND {
		return "", "", false
	}
	target, isField := addr.X.(*ast.SelectorExpr)
	if !isField {
		return "", "", false
	}
	if owner, isIdent := target.X.(*ast.Ident); !isIdent || owner.Name != "hcfg" {
		return "", "", false
	}
	lit, isLit := call.Args[1].(*ast.BasicLit)
	if !isLit || lit.Kind != token.STRING {
		t.Errorf("a flag registered into hcfg.%s is not named by a plain string literal", target.Sel.Name)
		return "", "", false
	}
	unquoted, err := strconv.Unquote(lit.Value)
	if err != nil {
		t.Errorf("unquoting the flag name %s: %v", lit.Value, err)
		return "", "", false
	}
	return unquoted, target.Sel.Name, true
}

// stdioVariableValues gives every variable [httpOnlyFlags] names two values
// that configure different things, so a variable a reader honors moves what
// it reads between the two.
var stdioVariableValues = map[string][2]string{
	"GITLAB_URL":                            {"https://gitlab.example.test", "https://other.example.test"},
	config.EnvPrefix + "SKIP_TLS_VERIFY":    {"true", "false"},
	config.EnvPrefix + "TIER":               {"premium", "ultimate"},
	config.EnvPrefix + "IGNORE_SCOPES":      {"true", "false"},
	config.EnvPrefix + "TOOL_SURFACE":       {config.ToolSurfaceMeta, config.ToolSurfaceIndividual},
	config.EnvPrefix + "CAPABILITY_SURFACE": {"minimal", "full"},
	config.EnvPrefix + "META_PARAM_SCHEMA":  {config.MetaParamSchemaFull, config.MetaParamSchemaCompact},
	config.EnvPrefix + "EMBEDDED_RESOURCES": {"false", "true"},
	config.EnvPrefix + "EXCLUDE_TOOLS":      {"gitlab_admin", "gitlab_user"},
	config.EnvPrefix + "READ_ONLY":          {"true", "false"},
	config.EnvPrefix + "SAFE_MODE":          {"true", "false"},
	config.EnvPrefix + "ACTION_TIMEOUT":     {"5m", "10m"},
	config.EnvPrefix + "RATE_LIMIT_RPS":     {"5", "6"},
	config.EnvPrefix + "RATE_LIMIT_BURST":   {"7", "8"},
	stdioMaxLineBytesEnv:                    {"65536", "131072"},
}

// stdioOnlySpellings names the flags whose stdio variable HTTP mode has no
// variable for, so the HTTP overlay cannot show that the variable is the
// flag's own, with the reason each is its counterpart anyway.
var stdioOnlySpellings = map[string]string{
	"max-request-body-bytes": "stdio's line ceiling and HTTP's body ceiling bound the same thing, the largest " +
		"inbound JSON-RPC message, and share a 4 MiB default so the two transports refuse the same messages (stdio.go)",
}

// isolateStdioVariables clears every variable [stdioVariableValues] names and
// every file a loader could read one from, so a case sets exactly one.
func isolateStdioVariables(t *testing.T) {
	t.Helper()
	t.Setenv(config.EnvFileVar, "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GITLAB_TOKEN", "glpat-stdio-flags")
	for variable := range stdioVariableValues {
		t.Setenv(variable, "")
	}
}

// stdioReads is everything a stdio run reads from the environment at startup:
// what [config.Load] returns and the limits the stdio transport reads itself.
type stdioReads struct {
	cfg    config.Config
	limits stdioLimits
}

func readStdio(t *testing.T) stdioReads {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("loading stdio's configuration: %v", err)
	}
	return stdioReads{cfg: *cfg, limits: stdioLimitsFromEnv()}
}

// namedStdioVariables returns every variable the table names, keyed by the
// flag naming it.
func namedStdioVariables() map[string]string {
	named := map[string]string{}
	for name, entry := range httpOnlyFlags {
		if entry.stdioVariable != "" {
			named[name] = entry.stdioVariable
		}
	}
	return named
}

// TestHTTPOnlyFlags_NameVariablesStdioReads holds the advice the startup line
// gives, "on stdio set X", to what a stdio run actually reads.
//
// Each variable the table names is set, alone, to each of two values, and what
// stdio reads at startup, [config.Load] and the transport's own limits, has to
// differ between them. A variable misspelled here, or one stdio stopped
// reading, would send an operator to set something that changes nothing, which
// is the same silence the warning exists to end.
func TestHTTPOnlyFlags_NameVariablesStdioReads(t *testing.T) {
	named := namedStdioVariables()
	for _, name := range slices.Sorted(maps.Keys(named)) {
		variable := named[name]
		t.Run(name, func(t *testing.T) {
			values, ok := stdioVariableValues[variable]
			if !ok {
				t.Fatalf("httpOnlyFlags names %s for --%s and this test has no values for it; add two that configure different things", variable, name)
			}
			isolateStdioVariables(t)
			t.Setenv(variable, values[0])
			first := readStdio(t)
			t.Setenv(variable, values[1])
			second := readStdio(t)
			if reflect.DeepEqual(first, second) {
				t.Errorf("%s=%s and %s=%s leave stdio's configuration the same, so the startup line would send an operator to a variable stdio does not read",
					variable, values[0], variable, values[1])
			}
		})
	}
}

// overlaidHTTPConfig is the HTTP configuration a run given no flags builds
// from the environment, through the overlay HTTP mode applies.
func overlaidHTTPConfig(t *testing.T) httpConfig {
	t.Helper()
	overlay, err := config.LoadHTTPEnvOverlay()
	if err != nil {
		t.Fatalf("loading the HTTP environment overlay: %v", err)
	}
	hcfg := httpConfig{setFlags: map[string]bool{}}
	applyHTTPEnvOverlay(&hcfg, overlay)
	return hcfg
}

// differingFields names the fields of two HTTP configurations that differ.
func differingFields(a, b httpConfig) []string {
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	var names []string
	for i := range va.NumField() {
		if fmt.Sprint(va.Field(i)) != fmt.Sprint(vb.Field(i)) {
			names = append(names, va.Type().Field(i).Name)
		}
	}
	return names
}

// TestHTTPOnlyFlags_NameEachFlagsOwnVariable holds each variable the table
// names to the flag it is named for, which the stdio read above cannot: a
// table sending --tier to GITLAB_MCP_TOOL_SURFACE would pass it, since stdio
// does read that variable.
//
// HTTP mode already pairs each flag with its variable: when the flag is not
// passed, the environment overlay writes the variable into the field the flag
// writes. So each variable is set to each of its two values, the overlay is
// applied to a configuration given no flags, and the one field that moves has
// to be the field the flag is registered into. A flag whose variable HTTP mode
// has no variable for is declared in [stdioOnlySpellings] with the reason it is
// the counterpart anyway, and the declaration is held to that: the overlay
// must move nothing for it.
func TestHTTPOnlyFlags_NameEachFlagsOwnVariable(t *testing.T) {
	fields := flagFieldsBoundToHTTPConfig(t)
	named := namedStdioVariables()
	for _, name := range slices.Sorted(maps.Keys(named)) {
		variable := named[name]
		t.Run(name, func(t *testing.T) {
			values, ok := stdioVariableValues[variable]
			if !ok {
				t.Fatalf("no values for %s; TestHTTPOnlyFlags_NameVariablesStdioReads says which to add", variable)
			}
			isolateStdioVariables(t)
			t.Setenv(variable, values[0])
			first := overlaidHTTPConfig(t)
			t.Setenv(variable, values[1])
			moved := differingFields(first, overlaidHTTPConfig(t))

			if _, declared := stdioOnlySpellings[name]; declared {
				if len(moved) != 0 {
					t.Errorf("--%s is declared to have no HTTP variable, but %s moves %v through the overlay; name the flag's own variable and drop the declaration",
						name, variable, moved)
				}
				return
			}
			if want := []string{fields[name]}; !slices.Equal(moved, want) {
				t.Errorf("%s moves the fields %v in HTTP mode, want only %v, the field --%s writes; the startup line names the wrong variable for it",
					variable, moved, want, name)
			}
		})
	}
	t.Run("every declaration names a flag with a stdio variable", func(t *testing.T) {
		for _, name := range slices.Sorted(maps.Keys(stdioOnlySpellings)) {
			if _, ok := named[name]; !ok {
				t.Errorf("stdioOnlySpellings declares --%s, which httpOnlyFlags gives no stdio variable", name)
			}
		}
	})
}

// TestHTTPOnlyFlags_RefuseWhereIgnoringCostsMoreThanASetting pins which flags
// can refuse a stdio start and what each reads: the three that withhold part
// of what the server serves, register row AUT-004's --read-only, --safe-mode
// and --exclude-tools, and --gitlab-url, whose being ignored sends the token
// to an instance it did not name. Each reads its own field, asks for nothing
// at its empty value, and names the setting to write instead.
//
// Every other flag ignored on stdio costs a setting and is only reported, so
// a flag added to the set is a decision about cost, taken here.
func TestHTTPOnlyFlags_RefuseWhereIgnoringCostsMoreThanASetting(t *testing.T) {
	var refusing []string
	for _, name := range slices.Sorted(maps.Keys(httpOnlyFlags)) {
		if httpOnlyFlags[name].refusal != nil {
			refusing = append(refusing, name)
		}
	}
	if want := []string{"exclude-tools", "gitlab-url", "read-only", "safe-mode"}; !slices.Equal(refusing, want) {
		t.Fatalf("flags that can refuse a stdio start = %v, want %v", refusing, want)
	}

	cases := []struct {
		name           string
		flag           string
		hcfg           httpConfig
		wantLine       string
		wantSetInstead string
	}{
		{name: "read-only on", flag: "read-only", hcfg: httpConfig{readOnly: true}, wantLine: stdioWithheldLine, wantSetInstead: "GITLAB_MCP_READ_ONLY=true"},
		{name: "read-only reads its own field", flag: "read-only", hcfg: httpConfig{safeMode: true, excludeTools: "gitlab_admin"}},
		{name: "safe-mode on", flag: "safe-mode", hcfg: httpConfig{safeMode: true}, wantLine: stdioWithheldLine, wantSetInstead: "GITLAB_MCP_SAFE_MODE=true"},
		{name: "safe-mode reads its own field", flag: "safe-mode", hcfg: httpConfig{readOnly: true, excludeTools: "gitlab_admin"}},
		{
			name: "exclude-tools naming tools", flag: "exclude-tools", hcfg: httpConfig{excludeTools: " project.delete , gitlab_admin,"},
			wantLine: stdioWithheldLine, wantSetInstead: "GITLAB_MCP_EXCLUDE_TOOLS=project.delete,gitlab_admin",
		},
		{name: "exclude-tools of blanks and commas names nothing", flag: "exclude-tools", hcfg: httpConfig{excludeTools: " , ,"}},
		{name: "exclude-tools reads its own field", flag: "exclude-tools", hcfg: httpConfig{readOnly: true, safeMode: true}},
		{
			name: "gitlab-url naming another instance", flag: "gitlab-url", hcfg: httpConfig{gitlabURLs: repeatedFlag{"https://gitlab.corp.example.test"}},
			wantLine: stdioInstanceLine, wantSetInstead: "GITLAB_URL",
		},
		{name: "gitlab-url naming the instance stdio connects to", flag: "gitlab-url", hcfg: httpConfig{gitlabURLs: repeatedFlag{"https://gitlab.example.test"}}},
		{name: "gitlab-url naming nothing", flag: "gitlab-url", hcfg: httpConfig{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITLAB_URL", "https://gitlab.example.test")
			entry := httpOnlyFlags[tc.flag]
			line, setInstead := entry.refusal(entry.stdioVariable, &tc.hcfg)
			if line != tc.wantLine {
				t.Errorf("refusal line = %q, want %q", line, tc.wantLine)
			}
			if setInstead != tc.wantSetInstead {
				t.Errorf("setting named instead = %q, want %q", setInstead, tc.wantSetInstead)
			}
		})
	}
}

// TestNamesStdioInstance_ComparesTheCanonicalInstances covers the comparison
// behind the --gitlab-url refusal: the instance a stdio run connects to
// against the ones the flag names, in the canonical form the HTTP allow-list
// uses, with a spelling either side cannot canonicalize matching nothing.
func TestNamesStdioInstance_ComparesTheCanonicalInstances(t *testing.T) {
	cases := []struct {
		name      string
		gitlabURL string
		named     []string
		want      bool
	}{
		{name: "GITLAB_URL unset connects to the default", named: []string{config.DefaultGitLabURL}, want: true},
		{name: "GITLAB_URL unset and the flag names another", named: []string{"https://gitlab.corp.example.test"}},
		{name: "spelled differently, same instance", gitlabURL: "  https://GitLab.Corp.Example.test:443/  ", named: []string{"https://gitlab.corp.example.test"}, want: true},
		{name: "one of several", gitlabURL: "https://gitlab.corp.example.test", named: []string{"https://other.example.test", "https://gitlab.corp.example.test"}, want: true},
		{name: "none of several", gitlabURL: "https://gitlab.corp.example.test", named: []string{"https://other.example.test", "https://third.example.test"}},
		{name: "a GITLAB_URL the allow-list cannot read", gitlabURL: "ftp://gitlab.corp.example.test", named: []string{"ftp://gitlab.corp.example.test"}},
		{name: "a flag the allow-list cannot read", gitlabURL: "https://gitlab.corp.example.test", named: []string{"https://gitlab.corp.example.test", "ftp://other.example.test"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITLAB_URL", tc.gitlabURL)
			if got := namesStdioInstance(tc.named); got != tc.want {
				t.Errorf("namesStdioInstance(%v) with GITLAB_URL=%q = %v, want %v", tc.named, tc.gitlabURL, got, tc.want)
			}
		})
	}
}

// TestStdioInstance_IsTheOneConfigLoadResolves holds [stdioInstance] to the
// instance [config.Load] resolves for a stdio run, which keeps its resolution
// unexported: a refusal that compared the flag against a different instance
// than the one the token is sent to would be comparing against nothing.
func TestStdioInstance_IsTheOneConfigLoadResolves(t *testing.T) {
	for _, gitlabURL := range []string{"", "   ", " https://gitlab.corp.example.test ", "https://gitlab.corp.example.test/"} {
		t.Run(strconv.Quote(gitlabURL), func(t *testing.T) {
			isolateStdioVariables(t)
			t.Setenv("GITLAB_URL", gitlabURL)
			cfg, err := config.Load()
			if err != nil {
				t.Fatalf("config.Load: %v", err)
			}
			if got := stdioInstance(); got != cfg.GitLabURL {
				t.Errorf("stdioInstance() = %q, want %q, the instance config.Load resolves", got, cfg.GitLabURL)
			}
		})
	}
}

// TestStdioInstanceLine_NamesTheDefaultInstance holds the instance the
// --gitlab-url refusal names for an unset GITLAB_URL to the one config falls
// back to, since the line spells it out rather than building it.
func TestStdioInstanceLine_NamesTheDefaultInstance(t *testing.T) {
	if want := config.DefaultGitLabURL + " when it is unset"; !strings.Contains(stdioInstanceLine, want) {
		t.Errorf("the --gitlab-url refusal does not say %q: %q", want, stdioInstanceLine)
	}
}

// TestStdioIgnoredFlags_SortsRefusalsFromTheRest covers the split: which of
// the flags a run was given only HTTP mode reads, which of those refuse the
// start, and the order they are named in.
func TestStdioIgnoredFlags_SortsRefusalsFromTheRest(t *testing.T) {
	cases := []struct {
		name        string
		hcfg        httpConfig
		wantRefuse  []stdioRefusal
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
			wantRefuse: []stdioRefusal{{flag: "--read-only", line: stdioWithheldLine, setInstead: "GITLAB_MCP_READ_ONLY=true"}},
		},
		{
			name:        "read-only=false asks for nothing and is only named",
			hcfg:        httpConfig{setFlags: map[string]bool{"read-only": true}},
			wantIgnored: []string{"read-only"},
		},
		{
			name: "refusals of both kinds and an ignored flag together",
			hcfg: httpConfig{
				setFlags:   map[string]bool{"tool-surface": true, "safe-mode": true, "gitlab-url": true},
				safeMode:   true,
				gitlabURLs: repeatedFlag{"https://gitlab.corp.example.test"},
			},
			wantRefuse: []stdioRefusal{
				{flag: "--gitlab-url", line: stdioInstanceLine, setInstead: "GITLAB_URL"},
				{flag: "--safe-mode", line: stdioWithheldLine, setInstead: "GITLAB_MCP_SAFE_MODE=true"},
			},
			wantIgnored: []string{"tool-surface"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITLAB_URL", "https://gitlab.example.test")
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
		"rate-limit-rps":         "--rate-limit-rps (on stdio set GITLAB_MCP_RATE_LIMIT_RPS)",
		"gitlab-url":             "--gitlab-url (on stdio set GITLAB_URL)",
		"max-request-body-bytes": "--max-request-body-bytes (on stdio set GITLAB_MCP_STDIO_MAX_LINE_BYTES)",
		"http-addr":              "--http-addr (stdio has no such setting)",
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

// startupLine is one line [reportStdioIgnoredFlags] is expected to write: its
// level, its message, and the flags it names when it names a list.
type startupLine struct {
	level, msg string
	flags      []any
}

// TestReportStdioIgnoredFlags_SaysWhatARunIgnoresAndRefusesWhatItCannot drives
// the reporting through each shape of run it distinguishes, reading every
// line it writes and the verdict it leaves behind.
//
// Not parallel: it replaces the default logger.
func TestReportStdioIgnoredFlags_SaysWhatARunIgnoresAndRefusesWhatItCannot(t *testing.T) {
	stdio := transportDecision{}
	auto := transportDecision{Inference: "stdin is a pipe"}
	httpRun := transportDecision{HTTP: true}
	listener := "--http-addr (stdio has no such setting)"
	limiter := "--rate-limit-rps (on stdio set GITLAB_MCP_RATE_LIMIT_RPS)"

	cases := []struct {
		name      string
		choice    transportDecision
		hcfg      httpConfig
		wantStart bool
		wantLines []startupLine
	}{
		{name: "an HTTP run reads them all", choice: httpRun, hcfg: httpConfig{setFlags: map[string]bool{"read-only": true}, readOnly: true}, wantStart: true},
		{name: "a stdio run given none", choice: stdio, hcfg: httpConfig{setFlags: map[string]bool{"log-level": true}}, wantStart: true},
		{
			name: "a stdio run the operator chose warns about every flag", choice: stdio,
			hcfg:      httpConfig{setFlags: map[string]bool{"rate-limit-rps": true, "http-addr": true}},
			wantStart: true,
			wantLines: []startupLine{{level: "WARN", msg: stdioIgnoredFlagsLine, flags: []any{listener, limiter}}},
		},
		{
			name: "auto names a flag stdio has no setting for at INFO", choice: auto,
			hcfg:      httpConfig{setFlags: map[string]bool{"http-addr": true}},
			wantStart: true,
			wantLines: []startupLine{{level: "INFO", msg: stdioAutoIgnoredFlagsLine, flags: []any{listener}}},
		},
		{
			name: "auto still warns about a flag stdio has a variable for", choice: auto,
			hcfg:      httpConfig{setFlags: map[string]bool{"rate-limit-rps": true}},
			wantStart: true,
			wantLines: []startupLine{{level: "WARN", msg: stdioIgnoredFlagsLine, flags: []any{limiter}}},
		},
		{
			name: "auto splits the two", choice: auto,
			hcfg:      httpConfig{setFlags: map[string]bool{"rate-limit-rps": true, "http-addr": true}},
			wantStart: true,
			wantLines: []startupLine{
				{level: "INFO", msg: stdioAutoIgnoredFlagsLine, flags: []any{listener}},
				{level: "WARN", msg: stdioIgnoredFlagsLine, flags: []any{limiter}},
			},
		},
		{
			name: "a gitlab-url naming the instance stdio connects to is only named", choice: stdio,
			hcfg:      httpConfig{setFlags: map[string]bool{"gitlab-url": true}, gitlabURLs: repeatedFlag{"https://gitlab.example.test/"}},
			wantStart: true,
			wantLines: []startupLine{{level: "WARN", msg: stdioIgnoredFlagsLine, flags: []any{"--gitlab-url (on stdio set GITLAB_URL)"}}},
		},
		{
			name: "read-only is refused", choice: stdio,
			hcfg:      httpConfig{setFlags: map[string]bool{"read-only": true}, readOnly: true},
			wantLines: []startupLine{{level: "ERROR", msg: stdioWithheldLine}},
		},
		{
			name: "a gitlab-url naming another instance is refused", choice: stdio,
			hcfg:      httpConfig{setFlags: map[string]bool{"gitlab-url": true}, gitlabURLs: repeatedFlag{"https://gitlab.corp.example.test"}},
			wantLines: []startupLine{{level: "ERROR", msg: stdioInstanceLine}},
		},
		{
			name: "safe-mode is refused under auto too", choice: auto,
			hcfg: httpConfig{setFlags: map[string]bool{"safe-mode": true, "tier": true, "http-addr": true}, safeMode: true},
			wantLines: []startupLine{
				{level: "INFO", msg: stdioAutoIgnoredFlagsLine, flags: []any{listener}},
				{level: "WARN", msg: stdioIgnoredFlagsLine, flags: []any{"--tier (on stdio set GITLAB_MCP_TIER)"}},
				{level: "ERROR", msg: stdioWithheldLine},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITLAB_URL", "https://gitlab.example.test")
			records := loggedRecords(t)

			if got := reportStdioIgnoredFlags(tc.choice, &tc.hcfg); got != tc.wantStart {
				t.Errorf("reportStdioIgnoredFlags() = %v, want %v", got, tc.wantStart)
			}

			assertStartupLines(t, records(), tc.wantLines)
		})
	}
}

// assertStartupLines holds the records a report wrote to the lines expected,
// one subtest per line: its level, its message, and the flags it names when
// the expected line names a list.
func assertStartupLines(t *testing.T, logged []map[string]any, want []startupLine) {
	t.Helper()
	if len(logged) != len(want) {
		t.Fatalf("logged %d lines, want %d: %v", len(logged), len(want), logged)
	}
	for i, line := range want {
		t.Run(line.level, func(t *testing.T) {
			if level, _ := logged[i]["level"].(string); level != line.level {
				t.Errorf("line %d is at %s, want %s", i, level, line.level)
			}
			if msg, _ := logged[i]["msg"].(string); msg != line.msg {
				t.Errorf("line %d reads %q, want %q", i, msg, line.msg)
			}
			if line.flags == nil {
				return
			}
			if got, _ := logged[i]["flags"].([]any); !reflect.DeepEqual(got, line.flags) {
				t.Errorf("line %d names %v, want %v", i, got, line.flags)
			}
		})
	}
}

// TestReportStdioIgnoredFlags_RefusalNamesTheFlagAndWhatToSet reads the
// refusal lines themselves: the flag each refused, the setting to write in its
// place, and a message that says the server was not started.
func TestReportStdioIgnoredFlags_RefusalNamesTheFlagAndWhatToSet(t *testing.T) {
	t.Setenv("GITLAB_URL", "")
	records := loggedRecords(t)
	hcfg := httpConfig{
		setFlags:     map[string]bool{"read-only": true, "safe-mode": true, "exclude-tools": true, "gitlab-url": true},
		readOnly:     true,
		safeMode:     true,
		excludeTools: "project.delete",
		gitlabURLs:   repeatedFlag{"https://gitlab.corp.example.test"},
	}

	if reportStdioIgnoredFlags(transportDecision{}, &hcfg) {
		t.Fatal("reportStdioIgnoredFlags() = true for a stdio run given flags it cannot ignore, want the start refused")
	}

	logged := records()
	want := []struct{ flag, set, msg string }{
		{"--exclude-tools", "GITLAB_MCP_EXCLUDE_TOOLS=project.delete", stdioWithheldLine},
		{"--gitlab-url", "GITLAB_URL", stdioInstanceLine},
		{"--read-only", "GITLAB_MCP_READ_ONLY=true", stdioWithheldLine},
		{"--safe-mode", "GITLAB_MCP_SAFE_MODE=true", stdioWithheldLine},
	}
	if len(logged) != len(want) {
		t.Fatalf("logged %d lines, want one refusal per flag: %v", len(logged), logged)
	}
	for i, w := range want {
		t.Run(w.flag, func(t *testing.T) {
			if got := logged[i]["flag"]; got != w.flag {
				t.Errorf("flag = %v, want %q", got, w.flag)
			}
			if got := logged[i]["set_instead"]; got != w.set {
				t.Errorf("set_instead = %v, want %q", got, w.set)
			}
			msg, _ := logged[i]["msg"].(string)
			if msg != w.msg {
				t.Errorf("msg = %q, want %q", msg, w.msg)
			}
			if !strings.Contains(msg, "will not be started") {
				t.Errorf("the refusal does not say the deployment was not started: %q", msg)
			}
			if level, _ := logged[i]["level"].(string); level != "ERROR" {
				t.Errorf("the refusal is logged at %s, want ERROR", level)
			}
		})
	}
}

// TestMain_StdioRunGivenAFlagItCannotIgnore_IsRefusedBeforeServing drives the
// refusals through main, which is the only place the flags a run was given
// and the transport it chose meet: a stdio run with credentials that would
// otherwise start exits 1, having said why, and never serves.
//
// The flag is refused even when the variable it stands for already asks for
// the same thing: the refusal is about a flag that does nothing on stdio, the
// rule [config.RetiredEnvUses] applies to a retired name set beside its
// replacement, and its line is worded to hold in that case too.
//
// stdin is a closed pipe, so the run is not mistaken for a person at a
// terminal and a serve that did start would end at once rather than hang.
func TestMain_StdioRunGivenAFlagItCannotIgnore_IsRefusedBeforeServing(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		env      map[string]string
		wantFlag string
		wantSet  string
	}{
		{name: "read-only", args: []string{"-read-only"}, wantFlag: "--read-only", wantSet: "GITLAB_MCP_READ_ONLY=true"},
		{
			name: "read-only beside the variable asking the same", args: []string{"-read-only"},
			env:      map[string]string{"GITLAB_MCP_READ_ONLY": "true"},
			wantFlag: "--read-only", wantSet: "GITLAB_MCP_READ_ONLY=true",
		},
		{name: "exclude-tools", args: []string{"-exclude-tools", "project.delete"}, wantFlag: "--exclude-tools", wantSet: "GITLAB_MCP_EXCLUDE_TOOLS=project.delete"},
		{name: "gitlab-url naming another instance", args: []string{"-gitlab-url", "https://gitlab.corp.example.test"}, wantFlag: "--gitlab-url", wantSet: "GITLAB_URL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withFreshFlagSet(t)
			t.Setenv(config.EnvFileVar, "")
			t.Setenv("GITLAB_URL", "https://gitlab.example.test")
			t.Setenv("GITLAB_TOKEN", testToken)
			t.Setenv("GITLAB_MCP_LOG_LEVEL", "")
			t.Setenv("GITLAB_MCP_TELEMETRY", "")
			t.Setenv("GITLAB_MCP_READ_ONLY", "")
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			closedStdin(t)

			var exits []int
			originalArgs, originalExit := os.Args, exitProcess
			originalLogger, originalBase := slog.Default(), baseLogHandler
			os.Args = append([]string{"gitlab-mcp-server"}, tc.args...)
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
				t.Fatalf("exit codes = %v, want [1] for a stdio run given %v\nstderr: %s", exits, tc.args, logged)
			}
			for _, want := range []string{`"flag":"` + tc.wantFlag + `"`, `"set_instead":"` + tc.wantSet + `"`} {
				t.Run(want, func(t *testing.T) {
					if !strings.Contains(logged, want) {
						t.Errorf("stderr does not carry %s:\n%s", want, logged)
					}
				})
			}
			if strings.Contains(logged, "starting MCP server") {
				t.Errorf("the refused run started serving:\n%s", logged)
			}
		})
	}
}
