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

// envRow is a row naming variables, carrying the findings given.
func envRow(id string, findings []string, envs ...string) tenancy.Decision {
	d := row(id)
	d.Envs = envs
	d.Findings = findings
	return d
}

// TestCheckConfig_SettingsReadThroughTheConfigurationPackage_Pass: a variable
// on the list that nothing reads directly passes, and so does a row that
// reads one another way and carries the finding that records it.
func TestCheckConfig_SettingsReadThroughTheConfigurationPackage_Pass(t *testing.T) {
	report := fixture{
		files: map[string]string{"site/site.go": configSource, "site/direct.go": directSource},
		rows: []tenancy.Decision{
			envRow("RTC-001", nil, "GITLAB_MCP_RATE_LIMIT_RPS"),
			envRow("HLD-001", []string{"F-13"}, "GITLAB_MCP_MAX_LISTEN_STREAMS"),
			row("ROW-003"),
		},
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

// TestCheckConfig_AListTheGateCannotFold_IsAFinding: a list that is not a
// composite literal, one holding a value that does not fold, and one that
// does not exist cannot be checked against, and the gate says so rather than
// passing every setting.
func TestCheckConfig_AListTheGateCannotFold_IsAFinding(t *testing.T) {
	for name, source := range map[string]string{
		"not a literal":  siteHeader + "\nvar prefixedNames = names()\n\nfunc names() []string { return nil }\n",
		"not a constant": siteHeader + "\nvar prefixedNames = []string{name()}\n\nfunc name() string { return \"\" }\n",
		"not declared":   siteHeader,
		"a function":     siteHeader + "\nfunc prefixedNames() {}\n",
	} {
		t.Run(name, func(t *testing.T) {
			report := fixture{
				files: map[string]string{"site/site.go": source},
				rows:  []tenancy.Decision{envRow("ROW-001", nil, "GITLAB_MCP_RATE_LIMIT_RPS")},
			}.run(t)
			assertFindings(t, report, "G14",
				siteDir+":prefixedNames: is not a package-level list of string constants, so no setting can be checked against it")
		})
	}
}
