package main

import (
	"fmt"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// configSource is the fixture's configuration package: its list of prefixed
// names, and reads of variables both ways.
const configSource = siteHeader + `
var prefixedNames = []string{
	"RATE_LIMIT_RPS",
	"MAX_LISTEN_STREAMS",
}

const listenEnv = "GITLAB_MCP_MAX_LISTEN_STREAMS"
`

// directSource reads variables around the configuration package, in a file
// of its own for its imports.
const directSource = `package site

import "os"

func readDirectly(name string) {
	_ = os.Getenv(listenEnv)
	_, _ = os.LookupEnv("GITLAB_MCP_DIRECT")
	_ = os.Getenv(name)
}
`

// syscallSource reads variables through the readers os wraps and through a
// template, in a file of its own for its imports.
const syscallSource = `package site

import (
	"os"
	"syscall"
)

func readAround(name string) {
	_, _ = syscall.Getenv("GITLAB_MCP_RATE_LIMIT_RPS")
	_ = os.ExpandEnv("${GITLAB_MCP_MAX_LISTEN_STREAMS}:$GITLAB_MCP_RATE_LIMIT_RPS")
	_ = os.ExpandEnv(name)
	_ = os.Expand("${GITLAB_MCP_UNREAD}", func(string) string { return "" })
}
`

// listSource stands in for the configuration package's readers, Getenv and
// TrimmedGetenv, which hands its parameter on to Getenv, and reads through
// them: two suffixes the list holds, a name nobody prefixed, a setting the
// list left out, through either reader, a full name carrying the prefix, and
// a name that does not fold.
const listSource = `package site

import (
	"os"
	"strings"
)

func Getenv(name string) string { return os.Getenv(name) }

func TrimmedGetenv(name string) string { return strings.TrimSpace(Getenv(name)) }

func readThroughTheList(name string) {
	_ = Getenv("RATE_LIMIT_RPS")
	_ = TrimmedGetenv("MAX_LISTEN_STREAMS")
	_ = TrimmedGetenv("GITLAB_URL")
	_ = Getenv("TELEMETRY")
	_ = TrimmedGetenv("TELEMETRY")
	_ = Getenv("GITLAB_MCP_FULL")
	_ = Getenv(name)
}
`

// envRow is a row naming variables, carrying the findings given.
func envRow(id string, findings []string, envs ...string) tenancy.Decision {
	d := row(id)
	d.Envs = envs
	d.Findings = findings
	return d
}

// aroundSource reads variables no row names: directly, through a helper that
// hands its parameter on to another that reads it, through one declared
// before the reader it hands its parameter to, through a helper that reads
// two of its parameters, directly or one of them through a reader declared
// after it, through a method, through a template a helper expands, and under
// a name a constant expression folds to. It also reads names that are not
// this project's, or that do not fold, which the gate cannot attribute to a
// setting, and hands a parameter to a reader as an argument that is not the
// name, which makes no reader of its own.
const aroundSource = `package site

import "os"

const unnamedEnv = "GITLAB_MCP_UNNAMED"

func size(name string) int { return len(name) }

func lookup(key string) string {
	value, _ := os.LookupEnv(key)
	return value
}

func setting(name string) string { return lookup(name) }

func early(name string) string { return late(name) }

func pick(first bool, a, b string) string {
	if first {
		return os.Getenv(a)
	}
	return os.Getenv(b)
}

func both(first, second string) string { return late(first) + os.Getenv(second) }

func late(name string) string { return os.Getenv(name) }

type reader struct{}

func (reader) read(_ int, name string) string { return os.Getenv(name) }

func label(prefix, name string) string { return prefix + os.Getenv(name) }

func mislabel(prefix string) string { return label(prefix, "") }

func expand(template string) string { return os.ExpandEnv(template) }

func pair() (int, string) { return 0, "" }

func readAround(name string) {
	_ = os.Getenv(unnamedEnv)
	_ = setting("GITLAB_MCP_HELPED")
	_ = early("GITLAB_MCP_EARLY")
	_ = pick(true, "GITLAB_MCP_FIRST", "GITLAB_MCP_SECOND")
	_ = both("GITLAB_MCP_THIRD", "GITLAB_MCP_FOURTH")
	_ = reader{}.read(0, "GITLAB_MCP_METHOD")
	_ = expand("$GITLAB_MCP_EXPANDED")
	_ = os.Getenv("GITLAB_MCP_" + "FOLDED")
	_ = mislabel("GITLAB_MCP_PREFIX")
	_ = setting("OTEL_SERVICE_NAME")
	_ = setting(name)
	_ = reader{}.read(size(name), name)
	_ = reader{}.read(pair())
}
`

// TestCheckConfig_SettingsReadThroughTheConfigurationPackage_Pass: a variable
// on the list that nothing reads directly passes, and so does a row that
// reads one another way and carries the finding that records it, and a
// variable no row names that a declaration answers.
func TestCheckConfig_SettingsReadThroughTheConfigurationPackage_Pass(t *testing.T) {
	declared := fixtureRules()
	declared.envDeclared = map[string]string{"GITLAB_MCP_DIRECT": "read once, before anything else is"}
	report := fixture{
		files: map[string]string{"site/site.go": configSource, "site/direct.go": directSource},
		rows: []tenancy.Decision{
			envRow("RTC-001", nil, "GITLAB_MCP_RATE_LIMIT_RPS"),
			envRow("HLD-001", []string{"F-13"}, "GITLAB_MCP_MAX_LISTEN_STREAMS"),
			row("ROW-003"),
		},
		rules: &declared,
	}.run(t)
	assertFindings(t, report, "G14")
	if report.Summary.Settings != 2 {
		t.Fatalf("settings read = %d, want 2", report.Summary.Settings)
	}
}

// TestCheckConfig_ASettingReadAroundTheConfigurationPackage_IsAFinding: a
// variable missing the prefix, one missing from the list, and one read
// directly each fail without the finding; a row carrying it while every
// variable passes has a finding that no longer describes the tree.
func TestCheckConfig_ASettingReadAroundTheConfigurationPackage_IsAFinding(t *testing.T) {
	report := fixture{
		files: map[string]string{"site/site.go": configSource, "site/direct.go": directSource},
		rows: []tenancy.Decision{
			envRow("ROW-001", nil, "RATE_LIMIT_RPS", "GITLAB_MCP_DIRECT", "GITLAB_MCP_MAX_LISTEN_STREAMS"),
			envRow("ROW-002", []string{"F-13"}, "GITLAB_MCP_RATE_LIMIT_RPS"),
		},
	}.run(t)
	listed := siteDir + ":prefixedNames"
	read := func(line string) string {
		return fmt.Sprintf("%s/site/direct.go:%d", fixtureDir, lineOf(t, directSource, line))
	}
	assertFindings(t, report, "G14",
		"ROW-001: RATE_LIMIT_RPS is not on "+listed+", and the row does not carry F-13",
		"ROW-001: GITLAB_MCP_DIRECT is not on "+listed+", and the row does not carry F-13",
		"ROW-001: GITLAB_MCP_MAX_LISTEN_STREAMS is read directly at "+read("\t_ = os.Getenv(listenEnv)")+", and the row does not carry F-13",
		"ROW-002: carries F-13, and every variable it names is read through the configuration package",
	)
}

// TestCheckConfig_ASettingReadThroughSyscallOrATemplate_IsAFinding: the
// syscall readers os wraps read a variable as directly as os does, and so does
// a template os.ExpandEnv expands, for every variable it names in either form;
// os.Expand hands its template to a function of the caller's, which reads no
// environment, and a template that does not fold names nothing the gate can
// see.
func TestCheckConfig_ASettingReadThroughSyscallOrATemplate_IsAFinding(t *testing.T) {
	report := fixture{
		files: map[string]string{"site/site.go": configSource, "site/syscall.go": syscallSource},
		rows: []tenancy.Decision{
			envRow("ROW-001", nil, "GITLAB_MCP_RATE_LIMIT_RPS", "GITLAB_MCP_MAX_LISTEN_STREAMS"),
		},
	}.run(t)
	read := func(line string) string {
		return fmt.Sprintf("%s/site/syscall.go:%d", fixtureDir, lineOf(t, syscallSource, line))
	}
	syscallRead := read("\t_, _ = syscall.Getenv(\"GITLAB_MCP_RATE_LIMIT_RPS\")")
	templateRead := read("\t_ = os.ExpandEnv(\"${GITLAB_MCP_MAX_LISTEN_STREAMS}:$GITLAB_MCP_RATE_LIMIT_RPS\")")
	assertFindings(t, report, "G14",
		"ROW-001: GITLAB_MCP_RATE_LIMIT_RPS is read directly at "+syscallRead+", "+templateRead+", and the row does not carry F-13",
		"ROW-001: GITLAB_MCP_MAX_LISTEN_STREAMS is read directly at "+templateRead+", and the row does not carry F-13",
	)
}

// TestCheckConfig_AListTheGateCannotFold_IsAFinding: a list that is not a
// composite literal, one holding a value that does not fold, and one that
// does not exist cannot be checked against, and the gate says so rather than
// passing every setting, whichever of the three things that read the list
// asks for it: a row's variable, a name the configuration package's readers
// are handed without the prefix, or a declaration of one read bare.
func TestCheckConfig_AListTheGateCannotFold_IsAFinding(t *testing.T) {
	const bareSource = `package site

import "os"

func Getenv(name string) string { return os.Getenv(name) }

func readBare() { _ = Getenv("TELEMETRY") }
`
	declared := fixtureRules()
	declared.envBare = map[string]string{"GITLAB_URL": "the instance, a name nobody prefixed"}
	for name, source := range map[string]string{
		"not a literal":  siteHeader + "\nvar prefixedNames = names()\n\nfunc names() []string { return nil }\n",
		"not a constant": siteHeader + "\nvar prefixedNames = []string{name()}\n\nfunc name() string { return \"\" }\n",
		"not declared":   siteHeader,
		"a function":     siteHeader + "\nfunc prefixedNames() {}\n",
	} {
		for asker, f := range map[string]fixture{
			"a row's variable": {
				files: map[string]string{"site/site.go": source},
				rows:  []tenancy.Decision{envRow("ROW-001", nil, "GITLAB_MCP_RATE_LIMIT_RPS")},
			},
			"a name read bare": {
				files: map[string]string{"site/site.go": source, "site/bare.go": bareSource},
				rows:  []tenancy.Decision{row("ROW-001")},
			},
			"a name declared bare": {
				files: map[string]string{"site/site.go": source},
				rows:  []tenancy.Decision{row("ROW-001")},
				rules: &declared,
			},
		} {
			t.Run(name+", "+asker, func(t *testing.T) {
				assertFindings(t, f.run(t), "G14",
					siteDir+":prefixedNames: is not a package-level list of string constants, so no setting can be checked against it")
			})
		}
	}
}

// TestCheckConfig_AVariableNoRowNames_ReadAroundTheConfigurationPackage_IsAFinding:
// every variable of this project's the code reads around the configuration
// package is reported, whether or not a row names it, at the call that names
// it: read directly, through a function that hands its parameter on to one
// that reads it, whichever of the two is declared first, through each of two
// parameters a function reads, whichever way it reads each and whichever
// reader it finds first, through a method, through a template a function
// expands, or under a name a constant expression folds to. A name that is
// not this project's is somebody else's
// variable, one that does not fold names nothing the gate can see, and a
// parameter handed to a reader as anything but the name reads nothing;
// none is reported. The rows here name no variable, so the list of prefixed
// names is never read, and its absence is no finding.
func TestCheckConfig_AVariableNoRowNames_ReadAroundTheConfigurationPackage_IsAFinding(t *testing.T) {
	report := fixture{
		files: map[string]string{"site/site.go": siteHeader, "site/around.go": aroundSource},
		rows:  []tenancy.Decision{row("ROW-001")},
	}.run(t)
	read := func(line string) string {
		return fmt.Sprintf("%s/site/around.go:%d", fixtureDir, lineOf(t, aroundSource, line))
	}
	unrecorded := ", and no row names it and no declaration answers it"
	picked := read("\t_ = pick(true, \"GITLAB_MCP_FIRST\", \"GITLAB_MCP_SECOND\")")
	both := read("\t_ = both(\"GITLAB_MCP_THIRD\", \"GITLAB_MCP_FOURTH\")")
	assertFindings(t, report, "G14",
		"GITLAB_MCP_UNNAMED: is read directly at "+read("\t_ = os.Getenv(unnamedEnv)")+unrecorded,
		"GITLAB_MCP_HELPED: is read directly at "+read("\t_ = setting(\"GITLAB_MCP_HELPED\")")+unrecorded,
		"GITLAB_MCP_EARLY: is read directly at "+read("\t_ = early(\"GITLAB_MCP_EARLY\")")+unrecorded,
		"GITLAB_MCP_FIRST: is read directly at "+picked+unrecorded,
		"GITLAB_MCP_SECOND: is read directly at "+picked+unrecorded,
		"GITLAB_MCP_THIRD: is read directly at "+both+unrecorded,
		"GITLAB_MCP_FOURTH: is read directly at "+both+unrecorded,
		"GITLAB_MCP_METHOD: is read directly at "+read("\t_ = reader{}.read(0, \"GITLAB_MCP_METHOD\")")+unrecorded,
		"GITLAB_MCP_EXPANDED: is read directly at "+read("\t_ = expand(\"$GITLAB_MCP_EXPANDED\")")+unrecorded,
		"GITLAB_MCP_FOLDED: is read directly at "+read("\t_ = os.Getenv(\"GITLAB_MCP_\" + \"FOLDED\")")+unrecorded,
	)
}

// TestCheckConfig_ADeclaredRead_IsAnsweredAndHeldToTheTree: a declaration
// answers the variable it names, wherever it is read; one whose variable
// nothing reads around the configuration package no longer describes the
// tree; and one naming a variable a row names is reported too, since a row's
// variable is the row's to answer. A variable the declarations leave out is
// still a finding.
func TestCheckConfig_ADeclaredRead_IsAnsweredAndHeldToTheTree(t *testing.T) {
	declared := fixtureRules()
	declared.envDeclared = map[string]string{
		"GITLAB_MCP_UNNAMED":        "read once, before anything else is",
		"GITLAB_MCP_HELPED":         "read where it is used, by design",
		"GITLAB_MCP_EARLY":          "read through a helper declared first",
		"GITLAB_MCP_FIRST":          "read by a helper of two names",
		"GITLAB_MCP_SECOND":         "read by a helper of two names",
		"GITLAB_MCP_THIRD":          "read by a helper of two readers",
		"GITLAB_MCP_FOURTH":         "read by a helper of two readers",
		"GITLAB_MCP_METHOD":         "read by a method of its own",
		"GITLAB_MCP_EXPANDED":       "read through a template",
		"GITLAB_MCP_GONE":           "read once, by code that has since gone",
		"GITLAB_MCP_RATE_LIMIT_RPS": "read where a row reads it",
	}
	report := fixture{
		files: map[string]string{"site/site.go": configSource, "site/around.go": aroundSource},
		rows:  []tenancy.Decision{envRow("RTC-001", nil, "GITLAB_MCP_RATE_LIMIT_RPS")},
		rules: &declared,
	}.run(t)
	folded := fmt.Sprintf("%s/site/around.go:%d", fixtureDir, lineOf(t, aroundSource, "\t_ = os.Getenv(\"GITLAB_MCP_\" + \"FOLDED\")"))
	assertFindings(t, report, "G14",
		"GITLAB_MCP_FOLDED: is read directly at "+folded+", and no row names it and no declaration answers it",
		"GITLAB_MCP_GONE: is declared as read around the configuration package, and nothing reads it that way",
		"GITLAB_MCP_RATE_LIMIT_RPS: is declared as read around the configuration package, and RTC-001 names it: a row's variable is its row's to answer",
	)
}

// TestCheckConfig_ANameTheConfigurationPackageReadsBare_IsDeclaredOrAFinding:
// the configuration package's readers read a suffix the list holds under the
// prefix, which passes, and any other name without the prefix bare, which
// passes only when a declaration says it is somebody else's. A setting of
// this project's that the list left out is read under a name no operator
// sets, so it is reported at every call that reads it, through either
// reader. A full name carrying the prefix is the direct half's to report, and
// a name that does not fold names nothing. A declaration whose name nothing
// reads bare is reported, and so is one of a suffix the list holds, which is
// read under the prefix.
func TestCheckConfig_ANameTheConfigurationPackageReadsBare_IsDeclaredOrAFinding(t *testing.T) {
	declared := fixtureRules()
	declared.envBare = map[string]string{
		"GITLAB_URL":     "the instance, a name nobody prefixed",
		"GITLAB_GONE":    "read bare by code that has since gone",
		"RATE_LIMIT_RPS": "read bare before the list held it",
	}
	report := fixture{
		files: map[string]string{"site/site.go": configSource, "site/list.go": listSource},
		rows:  []tenancy.Decision{row("ROW-001")},
		rules: &declared,
	}.run(t)
	read := func(line string) string {
		return fmt.Sprintf("%s/site/list.go:%d", fixtureDir, lineOf(t, listSource, line))
	}
	stale := ": is declared as read bare through the configuration package, and nothing reads it that way"
	assertFindings(t, report, "G14",
		"TELEMETRY: is read through the configuration package at "+
			read("\t_ = Getenv(\"TELEMETRY\")")+", "+read("\t_ = TrimmedGetenv(\"TELEMETRY\")")+
			", which reads it bare since "+siteDir+":prefixedNames does not hold it, and no declaration says it is somebody else's",
		"GITLAB_MCP_FULL: is read directly at "+read("\t_ = Getenv(\"GITLAB_MCP_FULL\")")+", and no row names it and no declaration answers it",
		"GITLAB_GONE"+stale,
		"RATE_LIMIT_RPS"+stale,
	)
}
