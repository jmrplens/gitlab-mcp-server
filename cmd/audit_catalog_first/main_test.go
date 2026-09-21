// main_test.go covers the audit_action_spec_coverage command, which
// validates the catalog-first migration by walking internal/tools and
// reporting the per-domain coverage of the action spec system.
//
// Tests rely on the live repository (via cmdutil.RepositoryRoot) so the
// production state is exercised end-to-end. A small set of unit tests
// target the individual helpers that classify stale AI-context lines,
// surface classifications, and catalog invariants.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/docgen"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncompat"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// cachedCoverageReport builds the repository coverage report once and shares
// it across the test functions of this package: buildCoverageReport is a pure
// analysis over the working tree (~3s per run), so re-running it per test only
// multiplied CPU time. Tests must treat the returned report as read-only.
var coverageReportOnce sync.Once

var (
	cachedReport    coverageReport
	errCachedReport error
)

func cachedCoverageReport(t *testing.T) coverageReport {
	t.Helper()
	coverageReportOnce.Do(func() {
		root, err := cmdutil.RepositoryRoot("../..")
		if err != nil {
			errCachedReport = err
			return
		}
		cachedReport, errCachedReport = buildCoverageReport(root)
	})
	if errCachedReport != nil {
		t.Fatalf("buildCoverageReport() error = %v", errCachedReport)
	}
	return cachedReport
}

// TestBuildCoverageReport_ClassifiesKeyDomains verifies BuildCoverageReport classifies key domains.
func TestBuildCoverageReport_ClassifiesKeyDomains(t *testing.T) {
	report := cachedCoverageReport(t)
	if report.SchemaVersion != schemaVersion {
		t.Fatalf("SchemaVersion = %d, want %d", report.SchemaVersion, schemaVersion)
	}
	if report.Summary.DomainCount == 0 {
		t.Fatal("expected discovered domains")
	}
	assertArchitectureCoverage(t, report)
	assertDomainCoverage(t, report)
}

// assertArchitectureCoverage holds the architecture section to what each of
// its fields says rather than to "not empty" and "not zero".
//
// Five strings naming five layers, and three alias counts, are exactly the
// shape where two fields trading places is invisible: every earlier assertion
// here passed on any permutation of them, so a report telling a reader the
// meta surface is registered the way the individual one is, or that there are
// as many action aliases as parameter aliases, read as a complete report.
func assertArchitectureCoverage(t *testing.T, report coverageReport) {
	t.Helper()
	sources := []struct {
		field string
		got   string
		want  string
	}{
		{field: "catalog_source", got: report.Architecture.CatalogSource, want: "ActionSpec groups collected from action_specs_manifest_gen.go"},
		{field: "manifest_source", got: report.Architecture.ManifestSource, want: "internal/tools/action_specs_manifest_gen.go"},
		{field: "meta_registration_source", got: report.Architecture.MetaRegistrationSource, want: "catalog projection via RegisterMetaCatalog"},
		{field: "individual_registration_source", got: report.Architecture.IndividualRegistrationSource, want: "catalog projection via RegisterIndividualCatalogTools"},
		{field: "dynamic_alias_source", got: report.Architecture.DynamicAliasSource, want: "actioncompat compatibility policy projected into ActionSpec metadata and Dynamic normalization"},
	}
	for _, source := range sources {
		t.Run(source.field, func(t *testing.T) {
			if source.got != source.want {
				t.Errorf("architecture %s = %q, want %q", source.field, source.got, source.want)
			}
		})
	}
	if report.Architecture.SurfaceSpecCount != report.Summary.SurfaceSpecCount {
		t.Fatalf("architecture surface specs = %d, summary = %d", report.Architecture.SurfaceSpecCount, report.Summary.SurfaceSpecCount)
	}
	if report.Architecture.LegacyBridgeCount != 0 || len(report.Architecture.LegacyBridges) != 0 {
		t.Fatalf("architecture legacy bridges = %+v, want zero", report.Architecture.LegacyBridges)
	}
	if want := len(actioncompat.ActionAliases()); report.Architecture.DynamicActionAliasCount != want {
		t.Errorf("architecture action aliases = %d, want %d", report.Architecture.DynamicActionAliasCount, want)
	}
	if want := len(actioncompat.ParameterAliases()); report.Architecture.DynamicParameterAliasCount != want {
		t.Errorf("architecture parameter aliases = %d, want %d", report.Architecture.DynamicParameterAliasCount, want)
	}
	// The spec-metadata aliases are the ones the ActionSpec carries, a proper
	// subset of the parameter aliases: stating that is what keeps the third
	// count from being satisfied by either of the other two.
	specMetadata := report.Architecture.DynamicSpecMetadataParameterAliasCount
	if specMetadata <= 0 || specMetadata >= report.Architecture.DynamicParameterAliasCount {
		t.Errorf("architecture spec-metadata aliases = %d, want a non-empty proper subset of the %d parameter aliases", specMetadata, report.Architecture.DynamicParameterAliasCount)
	}
}

func assertDomainCoverage(t *testing.T, report coverageReport) {
	t.Helper()
	projects := requireDomain(t, report, "projects")
	if !projects.HasIndividualTools || !projects.HasMetaSpecs || !projects.HasDynamicCatalogEntries {
		t.Fatalf("projects coverage missing expected surfaces: %+v", projects)
	}
	if projects.SurfaceClassification != "spec-backed" {
		t.Fatalf("projects classification = %q, want spec-backed", projects.SurfaceClassification)
	}

	dynamic := requireDomain(t, report, "dynamic")
	if dynamic.SurfaceClassification != "dynamic-controller-surface" || !dynamic.HasSurfaceSpecs || dynamic.SurfaceSpecCount != 2 {
		t.Fatalf("dynamic coverage missing controller surface specs: %+v", dynamic)
	}
}

// TestAuditCatalogFirstSource_CurrentProductionCodePasses verifies AuditCatalogFirstSource when current production code passes.
func TestAuditCatalogFirstSource_CurrentProductionCodePasses(t *testing.T) {
	root, err := cmdutil.RepositoryRoot("../..")
	if err != nil {
		t.Fatalf("cmdutil.RepositoryRoot() error = %v", err)
	}
	if auditErr := auditCatalogFirstSource(root); auditErr != nil {
		t.Fatalf("auditCatalogFirstSource() error = %v", auditErr)
	}
}

// TestAssertActionSpecManifestCurrent_DetectsStaleManifest verifies AssertActionSpecManifestCurrent detects stale manifest.
func TestAssertActionSpecManifestCurrent_DetectsStaleManifest(t *testing.T) {
	root := t.TempDir()
	toolsDir := filepath.Join(root, "internal", "tools")
	if err := os.MkdirAll(toolsDir, 0o750); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeAuditTestFile(t, filepath.Join(toolsDir, "action_specs.go"), `package tools

func buildAlphaActionSpecs() {}
func buildBetaActionSpecs() {}
`)
	writeAuditTestFile(t, filepath.Join(toolsDir, "action_specs_manifest_gen.go"), `package tools

func actionSpecGroupBuilders() []actionSpecGroupBuilder {
	return []actionSpecGroupBuilder{
		buildAlphaActionSpecs,
	}
}
`)

	if err := assertActionSpecManifestCurrent(root); err == nil {
		t.Fatal("assertActionSpecManifestCurrent() error = nil, want stale manifest error")
	}
}

// TestLegacyBridgeFindingsInContent_DetectsForbiddenReferences verifies LegacyBridgeFindingsInContent detects forbidden references.
func TestLegacyBridgeFindingsInContent_DetectsForbiddenReferences(t *testing.T) {
	findings := legacyBridgeFindingsInContent("runtime.go", "package tools\nfunc f(){ registerAllLegacy() }", []string{"registerAllLegacy"})
	if len(findings) != 1 || findings[0] != "runtime.go contains \"registerAllLegacy\"" {
		t.Fatalf("legacyBridgeFindingsInContent() = %+v, want registerAllLegacy finding", findings)
	}
}

// TestStaleAIContextLine_ClassifiesLegacyRegistrationGuidance covers StaleAIContextLine with table-driven subtests for classifies legacy registration guidance.
func TestStaleAIContextLine_ClassifiesLegacyRegistrationGuidance(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{name: "legacy create register tools", line: "4. Create `register.go` with `RegisterTools(server, client)`", want: true},
		{name: "legacy compatibility register tools", line: "Existing package-local `RegisterTools` files may remain for compatibility.", want: true},
		{name: "legacy subpackage delegation", line: "register.go # RegisterAll() — delegates to sub-package RegisterTools()", want: true},
		{name: "legacy register meta function", line: "func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {", want: true},
		{name: "negative guidance allowed", line: "Do not add package-level `RegisterMeta` calls for ordinary GitLab API actions.", want: false},
		// The second escape hatch beside "do not": a line saying a retired
		// shape cannot come back quotes that shape to name it, and is the one
		// sentence about it a reader most wants to keep.
		{name: "regression guidance allowed", line: "The catalog cannot regress to `func RegisterMeta(server` in a sub-package.", want: false},
		{name: "empty line allowed", line: "   ", want: false},
		{name: "catalog guidance allowed", line: "Add or update domain-local `ActionSpecs` and the audited catalog aggregation path.", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := staleAIContextLine(tt.line); got != tt.want {
				t.Fatalf("staleAIContextLine(%q) = %t, want %t", tt.line, got, tt.want)
			}
		})
	}
}

// TestAssertCoverageInvariants_DetectsPackageLocalRegisterTools verifies AssertCoverageInvariants detects package local register tools.
func TestAssertCoverageInvariants_DetectsPackageLocalRegisterTools(t *testing.T) {
	err := assertCoverageInvariants([]domainCoverage{{
		Package:          "example",
		HasRegisterTools: true,
		HasMetaSpecs:     true,
	}})
	if err == nil {
		t.Fatal("assertCoverageInvariants() error = nil, want package-local RegisterTools error")
	}
}

// TestAssertCoverageInvariants_DetectsIndividualOnlyPackage verifies AssertCoverageInvariants detects individual only package.
func TestAssertCoverageInvariants_DetectsIndividualOnlyPackage(t *testing.T) {
	err := assertCoverageInvariants([]domainCoverage{{
		Package:               "example",
		HasIndividualTools:    true,
		HasMetaSpecs:          false,
		SurfaceClassification: "individual-only",
	}})
	if err == nil {
		t.Fatal("assertCoverageInvariants() error = nil, want missing ActionSpec error")
	}
}

// TestCatalogActionsMissingIndividualProjectionPolicy verifies CatalogActionsMissingIndividualProjectionPolicy.
func TestCatalogActionsMissingIndividualProjectionPolicy(t *testing.T) {
	catalog := actioncatalog.NewCatalog()
	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_example"})
	group.SetAction(actioncatalog.Action{ID: "example.get", Name: "get"})
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}

	missing := catalogActionsMissingIndividualProjectionPolicy(catalog)
	if len(missing) != 1 || missing[0] != "example.get" {
		t.Fatalf("catalogActionsMissingIndividualProjectionPolicy() = %+v, want example.get", missing)
	}
}

// TestCatalogActionsMissingIndividualProjectionPolicy_Exemptions_AreAccepted
// verifies the projection check on the cases around a plain gap: a nil
// catalog reports nothing, an action whose ID is a documented meta-only alias
// is exempt, and an action carrying an individual tool name passes.
func TestCatalogActionsMissingIndividualProjectionPolicy_Exemptions_AreAccepted(t *testing.T) {
	tests := []struct {
		name    string
		group   string
		action  actioncatalog.Action
		nilCase bool
		want    []string
	}{
		{name: "nil catalog", nilCase: true},
		{name: "meta-only alias is exempt", group: "gitlab_server", action: actioncatalog.Action{ID: "server.health_check", Name: "health_check"}},
		// An action added with no ID of its own is still named in the finding,
		// by the ID the catalog derived for it when the group was added. That
		// is also what keeps the fallback below it — the group tool name
		// joined to the action name — unreachable: no action a catalog hands
		// back carries an empty ID.
		{
			name:   "an action added without an ID is named by the derived one",
			group:  "gitlab_example",
			action: actioncatalog.Action{Name: "get"},
			want:   []string{"example.get"},
		},
		{
			name:   "projected action passes",
			group:  "gitlab_example",
			action: actioncatalog.Action{ID: "example.get", Name: "get", IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_example_get"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var catalog *actioncatalog.Catalog
			if !tt.nilCase {
				catalog = actioncatalog.NewCatalog()
				group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: tt.group})
				group.SetAction(tt.action)
				if err := catalog.AddGroup(group); err != nil {
					t.Fatalf("AddGroup() error = %v", err)
				}
			}
			got := catalogActionsMissingIndividualProjectionPolicy(catalog)
			if len(got) != len(tt.want) {
				t.Fatalf("catalogActionsMissingIndividualProjectionPolicy() = %v, want %v", got, tt.want)
			}
			for i, want := range tt.want {
				if got[i] != want {
					t.Errorf("missing[%d] = %q, want %q", i, got[i], want)
				}
			}
		})
	}
}

// TestBuildCoverageReport_CoreSourceDomainsAreSpecBacked verifies BuildCoverageReport when core source domains are spec backed.
func TestBuildCoverageReport_CoreSourceDomainsAreSpecBacked(t *testing.T) {
	report := cachedCoverageReport(t)

	assertSpecBackedDomains(t, report, []string{
		"branches",
		"commits",
		"files",
		"groups",
		"issues",
		"mergerequests",
		"projects",
		"releaselinks",
		"releases",
		"repository",
		"tags",
		"wikis",
	})
}

// TestBuildCoverageReport_CICDDomainsAreSpecBacked verifies BuildCoverageReport when cicd domains are spec backed.
func TestBuildCoverageReport_CICDDomainsAreSpecBacked(t *testing.T) {
	report := cachedCoverageReport(t)

	assertSpecBackedDomains(t, report, []string{
		"cicatalog",
		"cilint",
		"civariables",
		"ciyamltemplates",
		"freezeperiods",
		"jobs",
		"jobtokenscope",
		"pipelines",
		"pipelineschedules",
		"pipelinetriggers",
		"runnercontrollers",
		"runnercontrollerscopes",
		"runnercontrollertokens",
		"runners",
	})
}

// TestBuildCoverageReport_CollaborationDomainsAreSpecBacked verifies BuildCoverageReport when collaboration domains are spec backed.
func TestBuildCoverageReport_CollaborationDomainsAreSpecBacked(t *testing.T) {
	report := cachedCoverageReport(t)

	assertSpecBackedDomains(t, report, []string{
		"boards",
		"events",
		"groupboards",
		"grouplabels",
		"groupmembers",
		"groupmilestones",
		"invites",
		"labels",
		"members",
		"milestones",
		"notifications",
		"resourceevents",
		"todos",
	})
}

// TestBuildCoverageReport_NoteAndDiscussionDomainsAreSpecBacked verifies BuildCoverageReport when note and discussion domains are spec backed.
func TestBuildCoverageReport_NoteAndDiscussionDomainsAreSpecBacked(t *testing.T) {
	report := cachedCoverageReport(t)

	assertSpecBackedDomains(t, report, []string{
		"commitdiscussions",
		"epicdiscussions",
		"epicnotes",
		"issuediscussions",
		"issuenotes",
		"mrapprovals",
		"mrapprovalsettings",
		"mrchanges",
		"mrcontextcommits",
		"mrdiscussions",
		"mrdraftnotes",
		"mrnotes",
		"snippetdiscussions",
		"snippetnotes",
	})
}

// TestBuildCoverageReport_AccessAndSecurityDomainsAreSpecBacked verifies BuildCoverageReport when access and security domains are spec backed.
func TestBuildCoverageReport_AccessAndSecurityDomainsAreSpecBacked(t *testing.T) {
	report := cachedCoverageReport(t)

	assertSpecBackedDomains(t, report, []string{
		"accessrequests",
		"accesstokens",
		"attestations",
		"compliancepolicy",
		"deploykeys",
		"deploytokens",
		"groupcredentials",
		"groupsshcerts",
		"impersonationtokens",
		"keys",
		"securityfindings",
		"securitysettings",
		"useremails",
		"usergpgkeys",
		"vulnerabilities",
	})
}

// TestBuildCoverageReport_AdminPlatformDomainsAreSpecBacked verifies BuildCoverageReport when admin platform domains are spec backed.
func TestBuildCoverageReport_AdminPlatformDomainsAreSpecBacked(t *testing.T) {
	report := cachedCoverageReport(t)

	assertSourceSpecBackedDomains(t, report, []string{
		"applications",
		"appearance",
		"appstatistics",
		"broadcastmessages",
		"bulkimports",
		"clusteragents",
		"customattributes",
		"dbmigrations",
		"features",
		"health",
		"license",
		"metadata",
		"namespaces",
		"planlimits",
		"settings",
		"sidekiq",
		"systemhooks",
		"topics",
		"usagedata",
	})
}

// TestBuildCoverageReport_PackageDeploymentStorageDomainsAreSpecBacked verifies BuildCoverageReport when package deployment storage domains are spec backed.
func TestBuildCoverageReport_PackageDeploymentStorageDomainsAreSpecBacked(t *testing.T) {
	report := cachedCoverageReport(t)

	assertSpecBackedDomains(t, report, []string{
		"containerregistry",
		"dependencies",
		"deploymentmergerequests",
		"deployments",
		"environments",
		"externalstatuschecks",
		"groupstoragemoves",
		"packages",
		"pages",
		"projectstoragemoves",
		"protectedenvs",
		"protectedpackages",
		"snippetstoragemoves",
		"uploads",
	})
	assertSourceSpecBackedDomains(t, report, []string{
		"dependencyproxy",
		"errortracking",
		"securefiles",
		"terraformstates",
	})
}

// TestBuildCoverageReport_GroupProjectEnterpriseDomainsAreSpecBacked verifies BuildCoverageReport when group project enterprise domains are spec backed.
func TestBuildCoverageReport_GroupProjectEnterpriseDomainsAreSpecBacked(t *testing.T) {
	report := cachedCoverageReport(t)

	assertSpecBackedDomains(t, report, []string{
		"epicissues",
		"epics",
		"groupepicboards",
		"groupiterations",
		"groupldap",
		"groupprotectedbranches",
		"groupprotectedenvs",
		"groupreleases",
		"groupsaml",
		"groupscim",
		"groupserviceaccounts",
		"groupwikis",
		"mergetrains",
		"projectaliases",
		"projectiterations",
		"projectmirrors",
		"projecttemplates",
	})
}

// TestBuildCoverageReport_UtilityTemplateDomainsAreSpecBacked verifies BuildCoverageReport when utility template domains are spec backed.
func TestBuildCoverageReport_UtilityTemplateDomainsAreSpecBacked(t *testing.T) {
	report := cachedCoverageReport(t)

	assertSpecBackedDomains(t, report, []string{
		"avatar",
		"awardemoji",
		"badges",
		"customemoji",
		"dockerfiletemplates",
		"gitignoretemplates",
		"licensetemplates",
		"markdown",
		"modelregistry",
	})
	assertSurfaceBackedDomain(t, report, "elicitationtools", "interactive-utility", 4)
	assertSurfaceBackedDomain(t, report, "projectdiscovery", "runtime-utility", 1)
}

// TestMarshalReport_WrittenFile_RoundTripsThroughJSON verifies the marshaled
// coverage report survives a write and a decode unchanged.
func TestMarshalReport_WrittenFile_RoundTripsThroughJSON(t *testing.T) {
	report := coverageReport{SchemaVersion: schemaVersion, Summary: coverageSummary{DomainCount: 1}, Domains: []domainCoverage{{Package: "example"}}}
	content, err := marshalReport(report)
	if err != nil {
		t.Fatalf("marshalReport() error = %v", err)
	}

	outputPath := filepath.Join(t.TempDir(), "coverage.json")
	writeErr := docgen.WriteReport(outputPath, content)
	if writeErr != nil {
		t.Fatalf("WriteReport() error = %v", writeErr)
	}

	written, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var decoded coverageReport
	unmarshalErr := json.Unmarshal(written, &decoded)
	if unmarshalErr != nil {
		t.Fatalf("Unmarshal() error = %v", unmarshalErr)
	}
	if decoded.SchemaVersion != schemaVersion || len(decoded.Domains) != 1 || decoded.Domains[0].Package != "example" {
		t.Fatalf("decoded report = %+v", decoded)
	}
}

// requireDomain returns domain test data or fails the test.
func requireDomain(t *testing.T, report coverageReport, packageName string) domainCoverage {
	t.Helper()
	for _, domain := range report.Domains {
		if domain.Package == packageName {
			return domain
		}
	}
	t.Fatalf("domain %q not found", packageName)
	return domainCoverage{}
}

// assertSpecBackedDomains checks spec backed domains invariants for tests.
func assertSpecBackedDomains(t *testing.T, report coverageReport, packageNames []string) {
	t.Helper()
	for _, packageName := range packageNames {
		domain := requireDomain(t, report, packageName)
		if domain.SurfaceClassification != "spec-backed" {
			t.Fatalf("%s classification = %q, want spec-backed", packageName, domain.SurfaceClassification)
		}
		if !domain.HasIndividualTools || !domain.HasMetaSpecs || !domain.HasDynamicCatalogEntries {
			t.Fatalf("%s coverage missing required surfaces: %+v", packageName, domain)
		}
		if domain.ActionSpecCount == 0 || domain.DynamicCatalogActionCount == 0 {
			t.Fatalf("%s coverage missing action counts: %+v", packageName, domain)
		}
	}
}

// assertSourceSpecBackedDomains checks source spec backed domains invariants for tests.
func assertSourceSpecBackedDomains(t *testing.T, report coverageReport, packageNames []string) {
	t.Helper()
	for _, packageName := range packageNames {
		domain := requireDomain(t, report, packageName)
		if domain.SurfaceClassification != "spec-backed" {
			t.Fatalf("%s classification = %q, want spec-backed", packageName, domain.SurfaceClassification)
		}
		if !domain.HasIndividualTools || !domain.HasMetaSpecs {
			t.Fatalf("%s coverage missing individual/source spec surfaces: %+v", packageName, domain)
		}
	}
}

// assertSurfaceBackedDomain checks surface backed domain invariants for tests.
func assertSurfaceBackedDomain(t *testing.T, report coverageReport, packageName, surfaceKind string, expectedUtilityActions int) {
	t.Helper()
	domain := requireDomain(t, report, packageName)
	if domain.SurfaceClassification != "surface-backed" {
		t.Fatalf("%s classification = %q, want surface-backed", packageName, domain.SurfaceClassification)
	}
	if domain.UtilitySurfaceActionCount != expectedUtilityActions {
		t.Fatalf("%s utility action count = %d, want %d: %+v", packageName, domain.UtilitySurfaceActionCount, expectedUtilityActions, domain)
	}
	if domain.SurfaceKindCounts[surfaceKind] != expectedUtilityActions {
		t.Fatalf("%s surface kind %q count = %d, want %d: %+v", packageName, surfaceKind, domain.SurfaceKindCounts[surfaceKind], expectedUtilityActions, domain)
	}
}

// writeAuditTestFile writes audit test file fixture data for tests.
func writeAuditTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

// TestIsGitLabClientType_RecognizesGitLabClient verifies the GitLab client
// type heuristic accepts names that contain both "gitlab" and "Client"
// substrings and rejects names missing either token.
//
// This helper underpins the productionFileCallsSelector classification
// logic; verifying it independently keeps the heuristic honest when the
// wider integration tests do not exercise the matching branch.
func TestIsGitLabClientType_RecognizesGitLabClient(t *testing.T) {
	tests := []struct {
		name     string
		typeName string
		want     bool
	}{
		{name: "concrete pointer", typeName: "*gitlabclient.Client", want: true},
		{name: "concrete value", typeName: "gitlabclient.Client", want: true},
		{name: "missing client token", typeName: "gitlabclient.Connection", want: false},
		{name: "missing gitlab token", typeName: "*internal.Client", want: false},
		{name: "empty", typeName: "", want: false},
		{name: "unrelated", typeName: "*http.Client", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isGitLabClientType(tt.typeName); got != tt.want {
				t.Fatalf("isGitLabClientType(%q) = %t, want %t", tt.typeName, got, tt.want)
			}
		})
	}
}

// TestNormalizedSurfaceKind_DefaultsToMetaGroup verifies the empty kind
// normalizes to the meta-group surface kind so downstream counters bucket
// legacy actions into a known category.
func TestNormalizedSurfaceKind_DefaultsToMetaGroup(t *testing.T) {
	if got := normalizedSurfaceKind(""); got != actioncatalog.SurfaceKindMetaGroup {
		t.Fatalf("normalizedSurfaceKind(\"\") = %q, want %q", got, actioncatalog.SurfaceKindMetaGroup)
	}
	if got := normalizedSurfaceKind(actioncatalog.SurfaceKindGitLabAction); got != actioncatalog.SurfaceKindGitLabAction {
		t.Fatalf("normalizedSurfaceKind preserves concrete kinds; got %q", got)
	}
}

// TestIsOrdinaryGitLabActionKind_Cases verifies the kind switch recognizes
// ordinary GitLab actions and meta-groups as ordinary, while utility and
// controller kinds are excluded.
func TestIsOrdinaryGitLabActionKind_Cases(t *testing.T) {
	tests := []struct {
		name string
		kind actioncatalog.SurfaceKind
		want bool
	}{
		{name: "gitlab action", kind: actioncatalog.SurfaceKindGitLabAction, want: true},
		{name: "meta group", kind: actioncatalog.SurfaceKindMetaGroup, want: true},
		{name: "empty falls back to meta group", kind: "", want: true},
		{name: "utility", kind: actioncatalog.SurfaceKindRuntimeUtility, want: false},
		{name: "controller", kind: actioncatalog.SurfaceKindDynamicController, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isOrdinaryGitLabActionKind(tt.kind); got != tt.want {
				t.Fatalf("isOrdinaryGitLabActionKind(%q) = %t, want %t", tt.kind, got, tt.want)
			}
		})
	}
}

// TestActionOwnerPackage_PrefersOwnerOverDomain verifies action owner lookup
// prefers the explicit OwnerPackage and falls back to the domain when the
// owner is missing.
func TestActionOwnerPackage_PrefersOwnerOverDomain(t *testing.T) {
	ownerOnly := actioncatalog.Action{OwnerPackage: "  ownerpkg  ", Domain: "dom"}
	if got := actionOwnerPackage(ownerOnly); got != "ownerpkg" {
		t.Fatalf("actionOwnerPackage(ownerOnly) = %q, want ownerpkg", got)
	}
	domainOnly := actioncatalog.Action{Domain: "  dompkg  "}
	if got := actionOwnerPackage(domainOnly); got != "dompkg" {
		t.Fatalf("actionOwnerPackage(domainOnly) = %q, want dompkg", got)
	}
	both := actioncatalog.Action{OwnerPackage: "owner", Domain: "domain"}
	if got := actionOwnerPackage(both); got != "owner" {
		t.Fatalf("actionOwnerPackage(both) = %q, want owner", got)
	}
	if got := actionOwnerPackage(actioncatalog.Action{}); got != "" {
		t.Fatalf("actionOwnerPackage(empty) = %q, want empty", got)
	}
}

// TestExprString_FormatsASTNodes verifies exprString renders Go AST
// expressions using format.Node and returns empty on format errors.
func TestExprString_FormatsASTNodes(t *testing.T) {
	fileSet := token.NewFileSet()
	expr, err := parser.ParseExpr("*gitlabclient.Client")
	if err != nil {
		t.Fatalf("parser.ParseExpr() error = %v", err)
	}
	if got := exprString(fileSet, expr); got != "*gitlabclient.Client" {
		t.Fatalf("exprString() = %q, want *gitlabclient.Client", got)
	}
}

// TestRegisterToolsClientType_ReturnsNonServerParam verifies the helper
// extracts the first non-*mcp.Server parameter type name from a RegisterTools
// function declaration.
func TestRegisterToolsClientType_ReturnsNonServerParam(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "single client param",
			src:  "package x\nfunc RegisterTools(s *mcp.Server, c *gitlabclient.Client) {}\n",
			want: "*gitlabclient.Client",
		},
		{
			name: "no params",
			src:  "package x\nfunc RegisterTools() {}\n",
			want: "",
		},
		{
			name: "only server param",
			src:  "package x\nfunc RegisterTools(s *mcp.Server) {}\n",
			want: "",
		},
		{
			name: "first param is server",
			src:  "package x\nfunc RegisterTools(s *mcp.Server, c *gitlabclient.Client) {}\n",
			want: "*gitlabclient.Client",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileSet := token.NewFileSet()
			file, err := parser.ParseFile(fileSet, "", tt.src, 0)
			if err != nil {
				t.Fatalf("parser.ParseFile() error = %v", err)
			}
			var fn *ast.FuncDecl
			for _, decl := range file.Decls {
				if f, ok := decl.(*ast.FuncDecl); ok {
					fn = f
					break
				}
			}
			if fn == nil {
				t.Fatalf("no function declaration in %q", tt.src)
			}
			if got := registerToolsClientType(fileSet, fn); got != tt.want {
				t.Fatalf("registerToolsClientType() = %q, want %q", got, tt.want)
			}
		})
	}
}

// catalogFirstFixtureFiles is the smallest repository layout every source
// assertion of buildCoverageReport accepts: the production files the
// legacy-bridge scan reads, the dynamic register.go, one builder source plus
// a manifest naming it, the AI-context files and directories, and one domain
// package carrying a markdown formatter and a test. Paths are slash-separated
// and relative to the fixture root.
func catalogFirstFixtureFiles() map[string]string {
	return map[string]string{
		"internal/tools/action_catalog.go":            "package tools\n",
		"internal/tools/register_meta.go":             "package tools\n",
		"internal/tools/register.go":                  "package tools\n",
		"internal/toolutil/meta_tool.go":              "package toolutil\n",
		"internal/tools/dynamic/register.go":          "package dynamic\n",
		"internal/tools/action_specs.go":              "package tools\n\nfunc buildAlphaActionSpecs() {}\n",
		"internal/tools/action_specs_manifest_gen.go": "package tools\n\nfunc actionSpecGroupBuilders() []actionSpecGroupBuilder {\n\treturn []actionSpecGroupBuilder{\n\t\tbuildAlphaActionSpecs,\n\t}\n}\n",
		"internal/tools/alpha/alpha.go":               "package alpha\n",
		"internal/tools/alpha/markdown.go":            "package alpha\n",
		"internal/tools/alpha/alpha_test.go":          "package alpha\n",
		".github/copilot-instructions.md":             "# Copilot\n",
		"AGENTS.md":                                   "# Agents\n",
		"CLAUDE.md":                                   "# Claude\n",
		".github/agents/agent.md":                     "# Agent\n",
		".github/agents/notes.txt":                    "not markdown\n",
		".github/skills/skill.md":                     "# Skill\n",
		".github/instructions/go.md":                  "# Go\n",
	}
}

// writeCatalogFirstFixture materializes files under a fresh temporary root
// and returns that root.
func writeCatalogFirstFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		writeAuditTestFile(t, path, content)
	}
	return root
}

// TestBuildCoverageReport_FixtureRoot_ReportsSourceOnlyDomains verifies the
// full report over a synthetic repository: a domain that contributes no
// catalog action is classified as having no GitLab surface while its
// markdown formatter and test file are still recorded, and the architecture
// section reports no legacy bridge.
func TestBuildCoverageReport_FixtureRoot_ReportsSourceOnlyDomains(t *testing.T) {
	root := writeCatalogFirstFixture(t, catalogFirstFixtureFiles())

	report, err := buildCoverageReport(root)
	if err != nil {
		t.Fatalf("buildCoverageReport() error = %v", err)
	}
	alpha := requireDomain(t, report, "alpha")
	if alpha.SurfaceClassification != noGitLabSurface || !alpha.HasMarkdown || !alpha.HasTests || alpha.HasRegisterTools {
		t.Errorf("alpha coverage = %+v, want no-surface domain with markdown and tests", alpha)
	}
	if report.Summary.DomainCount != 2 || report.Summary.NoGitLabActionSurfaceCount < 1 {
		t.Errorf("summary = %+v, want 2 domains with at least one no-surface domain", report.Summary)
	}
	if report.Architecture.LegacyBridgeCount != 0 || len(report.Architecture.LegacyBridges) != 0 {
		t.Errorf("architecture reports legacy bridges: %+v", report.Architecture)
	}
}

// TestBuildCoverageReport_NestedWorktrees_AreNotReadAsThisTree verifies that
// the source audits read this repository and nothing else that merely lives
// inside the checkout.
//
// The parallel-agent tooling puts a complete worktree of this repository under
// .claude/worktrees/ per running agent, and a developer may put one anywhere
// with `git worktree add`. Each case below plants one carrying a production
// file that calls the forbidden selector: the file is real, it parses, and it
// would fail the audit outright if the walk read it. Asserting only that the
// walk does not crash would be the weaker test, because the crash was the
// benign outcome; what must hold is that the files inside are not counted, so
// a gate's verdict cannot depend on whether an agent happened to be running.
//
// Both rules are exercised. The .claude case is caught by name, the ordinary
// name by the .git marker a worktree carries, which is the case a skip list
// naming today's directory would have folded in without a word.
func TestBuildCoverageReport_NestedWorktrees_AreNotReadAsThisTree(t *testing.T) {
	const callsForbiddenSelector = "package beta\n\nfunc f() { toolutil.CaptureMetaToolDefinitions() }\n"

	cases := []struct {
		name     string
		worktree string
	}{
		{name: "under the agent tooling's directory", worktree: ".claude/worktrees/agent-1"},
		{name: "under an ordinary name", worktree: "scratch"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := catalogFirstFixtureFiles()
			files[tc.worktree+"/.git"] = "gitdir: /elsewhere/.git/worktrees/agent-1\n"
			files[tc.worktree+"/internal/tools/beta/beta.go"] = callsForbiddenSelector
			root := writeCatalogFirstFixture(t, files)

			report, err := buildCoverageReport(root)
			if err != nil {
				t.Fatalf("buildCoverageReport() error = %v, want the worktree left unread", err)
			}
			if report.Summary.DomainCount != 2 {
				t.Errorf("summary domain count = %d, want 2: the worktree's domains are not ours", report.Summary.DomainCount)
			}
			for _, domain := range report.Domains {
				if domain.Package == "beta" {
					t.Errorf("report counts %q, a domain that exists only inside the worktree", domain.Package)
				}
			}
		})
	}
}

// TestBuildCoverageReport_BrokenFixtures_ReportsFirstFailingAssertion
// verifies each source assertion and invariant of buildCoverageReport on a
// synthetic repository broken in exactly one way, checking that the error
// names the failing assertion.
func TestBuildCoverageReport_BrokenFixtures_ReportsFirstFailingAssertion(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(files map[string]string)
		wantErr string
	}{
		{
			name: "production source calls the forbidden selector",
			mutate: func(f map[string]string) {
				f["internal/tools/beta/beta.go"] = "package beta\n\nfunc f() { toolutil.CaptureMetaToolDefinitions() }\n"
			},
			wantErr: "calls toolutil.CaptureMetaToolDefinitions",
		},
		{
			name:    "production source does not parse",
			mutate:  func(f map[string]string) { f["internal/tools/beta/beta.go"] = "package beta\n\nfunc {\n" },
			wantErr: "parse ",
		},
		{
			name:    "action_catalog.go missing",
			mutate:  func(f map[string]string) { delete(f, "internal/tools/action_catalog.go") },
			wantErr: "read ",
		},
		{
			name: "action_catalog.go references legacy meta registration",
			mutate: func(f map[string]string) {
				f["internal/tools/action_catalog.go"] = "package tools\n\n// registerAllMetaGroups(\n"
			},
			wantErr: "must not depend on legacy meta registration",
		},
		{
			name:    "register.go keeps a legacy bridge",
			mutate:  func(f map[string]string) { f["internal/tools/register.go"] = "package tools\n\n// registerAllLegacy\n" },
			wantErr: "production legacy bridge count = 1",
		},
		{
			name:    "meta_tool.go missing",
			mutate:  func(f map[string]string) { delete(f, "internal/toolutil/meta_tool.go") },
			wantErr: "read ",
		},
		{
			name:    "dynamic register.go missing",
			mutate:  func(f map[string]string) { delete(f, "internal/tools/dynamic/register.go") },
			wantErr: "read ",
		},
		{
			name: "dynamic register.go owns compatibility policy",
			mutate: func(f map[string]string) {
				f["internal/tools/dynamic/register.go"] = "package dynamic\n\nfunc boolStringValue(v bool) string { return \"\" }\n"
			},
			wantErr: "owns compatibility policy",
		},
		{
			name:    "AI context file missing",
			mutate:  func(f map[string]string) { delete(f, "CLAUDE.md") },
			wantErr: "read ",
		},
		{
			name:    "AI context carries stale guidance",
			mutate:  func(f map[string]string) { f["CLAUDE.md"] = "# Claude\n\nCreate `register.go` with `RegisterTools`.\n" },
			wantErr: "AI context audit failed",
		},
		{
			name: "AI context directory missing",
			mutate: func(f map[string]string) {
				delete(f, ".github/agents/agent.md")
				delete(f, ".github/agents/notes.txt")
			},
			wantErr: "walk AI context",
		},
		{
			name:    "no builder in the tools source",
			mutate:  func(f map[string]string) { f["internal/tools/action_specs.go"] = "package tools\n" },
			wantErr: "no action spec group builders found",
		},
		{
			name:    "manifest missing",
			mutate:  func(f map[string]string) { delete(f, "internal/tools/action_specs_manifest_gen.go") },
			wantErr: "parse ",
		},
		{
			name:    "manifest without the builders function",
			mutate:  func(f map[string]string) { f["internal/tools/action_specs_manifest_gen.go"] = "package tools\n" },
			wantErr: "does not define actionSpecGroupBuilders",
		},
		{
			name: "domain keeps package-level RegisterMeta",
			mutate: func(f map[string]string) {
				f["internal/tools/alpha/alpha.go"] = "package alpha\n\nfunc RegisterMeta() {}\n"
			},
			wantErr: "still defines package-level RegisterMeta",
		},
		{
			name: "domain keeps GitLab-client RegisterTools",
			mutate: func(f map[string]string) {
				f["internal/tools/alpha/alpha.go"] = "package alpha\n\nfunc RegisterTools(server *mcp.Server, client *gitlabclient.Client) {}\n"
			},
			wantErr: "still defines package-local RegisterTools",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := catalogFirstFixtureFiles()
			tt.mutate(files)
			root := writeCatalogFirstFixture(t, files)

			_, err := buildCoverageReport(root)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("buildCoverageReport() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

// TestAssertNoLegacyRuntimeBridges_Scenarios_ReadsFixedFiles verifies the
// bridge scan reports a missing production file, a reference to a retired
// bridge, and a clean tree, and that buildArchitectureReport shares the
// missing-file failure.
func TestAssertNoLegacyRuntimeBridges_Scenarios_ReadsFixedFiles(t *testing.T) {
	clean := map[string]string{
		"internal/tools/action_catalog.go": "package tools\n",
		"internal/tools/register_meta.go":  "package tools\n",
		"internal/tools/register.go":       "package tools\n",
		"internal/toolutil/meta_tool.go":   "package toolutil\n",
	}
	bridged := maps.Clone(clean)
	bridged["internal/tools/register_meta.go"] = "package tools\n\n// domain.RegisterMeta(server)\n"

	tests := []struct {
		name    string
		files   map[string]string
		wantErr string
	}{
		{name: "missing production file", files: map[string]string{}, wantErr: "read "},
		{name: "retired bridge referenced", files: bridged, wantErr: "production legacy bridge count = 1"},
		{name: "clean tree", files: clean},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeCatalogFirstFixture(t, tt.files)
			err := assertNoLegacyRuntimeBridges(root)
			_, architectureErr := buildArchitectureReport(root, coverageSummary{SurfaceSpecCount: 3})
			if tt.wantErr == "" {
				if err != nil || architectureErr != nil {
					t.Fatalf("clean tree errors = %v / %v, want nil", err, architectureErr)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("assertNoLegacyRuntimeBridges() error = %v, want containing %q", err, tt.wantErr)
			}
			if tt.files["internal/tools/register.go"] == "" && architectureErr == nil {
				t.Error("buildArchitectureReport() error = nil on a tree without production files")
			}
		})
	}
}

// TestBuildArchitectureReport_CleanFixture_MirrorsSummaryAndAliases verifies
// the architecture section carries the summary's surface spec count, no
// legacy bridge, and the compatibility alias counts of the real actioncompat
// policy.
func TestBuildArchitectureReport_CleanFixture_MirrorsSummaryAndAliases(t *testing.T) {
	root := writeCatalogFirstFixture(t, catalogFirstFixtureFiles())

	architecture, err := buildArchitectureReport(root, coverageSummary{SurfaceSpecCount: 7})
	if err != nil {
		t.Fatalf("buildArchitectureReport() error = %v", err)
	}
	if architecture.SurfaceSpecCount != 7 || architecture.LegacyBridgeCount != 0 || len(architecture.LegacyBridges) != 0 {
		t.Errorf("architecture = %+v, want 7 surface specs and no bridge", architecture)
	}
	if architecture.DynamicActionAliasCount == 0 || architecture.DynamicParameterAliasCount < architecture.DynamicSpecMetadataParameterAliasCount {
		t.Errorf("alias counts = %+v, want the real policy sizes", architecture)
	}
}

// TestAIContextFiles_Fixture_ListsMarkdownSorted verifies the AI-context
// inventory holds the three fixed files plus every .md under the three
// .github directories, sorted, and skips non-Markdown entries.
func TestAIContextFiles_Fixture_ListsMarkdownSorted(t *testing.T) {
	root := writeCatalogFirstFixture(t, catalogFirstFixtureFiles())

	files, err := aiContextFiles(root)
	if err != nil {
		t.Fatalf("aiContextFiles() error = %v", err)
	}
	want := []string{
		filepath.Join(root, ".github", "agents", "agent.md"),
		filepath.Join(root, ".github", "copilot-instructions.md"),
		filepath.Join(root, ".github", "instructions", "go.md"),
		filepath.Join(root, ".github", "skills", "skill.md"),
		filepath.Join(root, "AGENTS.md"),
		filepath.Join(root, "CLAUDE.md"),
	}
	if strings.Join(files, "\n") != strings.Join(want, "\n") {
		t.Errorf("aiContextFiles() = %v, want %v", files, want)
	}
}

// TestSkipSelectorAuditEntry_WalkError_IsReturned verifies a walk error is
// handed back unchanged instead of being skipped.
func TestSkipSelectorAuditEntry_WalkError_IsReturned(t *testing.T) {
	walkErr := errors.New("walk failed")
	skip, err := skipSelectorAuditEntry(false, "", nil, walkErr)
	if skip || !errors.Is(err, walkErr) {
		t.Fatalf("skipSelectorAuditEntry() = %v, %v; want false and the walk error", skip, err)
	}
}

// TestReadManifestActionSpecGroupBuilders_Scenarios_ParsesReturnLiteral
// verifies the manifest reader lists the identifiers of the returned
// composite literal, returns nothing for a function that returns nil, and
// fails when the file does not parse or lacks the function.
func TestReadManifestActionSpecGroupBuilders_Scenarios_ParsesReturnLiteral(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		want    []string
		wantErr string
	}{
		{
			name:   "identifiers in declaration order",
			source: "package tools\n\nfunc helper() {}\n\nfunc actionSpecGroupBuilders() []b {\n\treturn []b{\n\t\tbuildZetaActionSpecs,\n\t\tbuildAlphaActionSpecs,\n\t}\n}\n",
			want:   []string{"buildZetaActionSpecs", "buildAlphaActionSpecs"},
		},
		{name: "nil return carries no names", source: "package tools\n\nfunc actionSpecGroupBuilders() []b { return nil }\n"},
		// The three shapes the walk has to step over rather than read: a
		// declaration that is not a function at all, a return carrying no
		// result, and an element of the literal that is not a plain name. Each
		// used to be a branch nothing took, so a walk that stopped at the
		// first of them would have reported an empty manifest and made every
		// builder read as stale.
		{
			name:   "non-function declarations are stepped over",
			source: "package tools\n\nimport \"fmt\"\n\nvar _ = fmt.Sprint\n\nfunc actionSpecGroupBuilders() []b {\n\treturn []b{\n\t\tbuildAlphaActionSpecs,\n\t}\n}\n",
			want:   []string{"buildAlphaActionSpecs"},
		},
		{
			name:   "a resultless return is stepped over",
			source: "package tools\n\nfunc actionSpecGroupBuilders() (builders []b) {\n\tif builders != nil {\n\t\treturn\n\t}\n\treturn []b{\n\t\tbuildAlphaActionSpecs,\n\t}\n}\n",
			want:   []string{"buildAlphaActionSpecs"},
		},
		{
			name:   "an element that is not a name is left out",
			source: "package tools\n\nfunc actionSpecGroupBuilders() []b {\n\treturn []b{\n\t\tbuildAlphaActionSpecs,\n\t\tother.BuildBetaActionSpecs,\n\t}\n}\n",
			want:   []string{"buildAlphaActionSpecs"},
		},
		{name: "unparsable manifest", source: "package tools\n\nfunc {\n", wantErr: "parse "},
		{name: "function missing", source: "package tools\n\nfunc other() {}\n", wantErr: "does not define actionSpecGroupBuilders"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "action_specs_manifest_gen.go")
			writeAuditTestFile(t, path, tt.source)
			got, err := readManifestActionSpecGroupBuilders(path)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("readManifestActionSpecGroupBuilders() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("readManifestActionSpecGroupBuilders() error = %v", err)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("builders = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDiscoverDomainSources_UnusableTree_ReturnsError verifies the domain
// walk reports a missing internal/tools directory, a domain directory that
// vanished before inspection, and a domain file that does not parse.
func TestDiscoverDomainSources_UnusableTree_ReturnsError(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		inspect string
		wantErr string
		// wantIs is asserted with errors.Is where the text is the operating
		// system's rather than this tool's: Windows spells a missing path
		// differently from Linux, and the error wraps fs.ErrNotExist on both.
		wantIs error
	}{
		{name: "tools directory missing", files: map[string]string{}, wantErr: "read tools directory"},
		{name: "domain file does not parse", files: map[string]string{"internal/tools/alpha/alpha.go": "package alpha\n\nfunc {\n"}, wantErr: "parse "},
		{name: "domain directory missing", files: map[string]string{}, inspect: "absent", wantIs: fs.ErrNotExist},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeCatalogFirstFixture(t, tt.files)
			var err error
			if tt.inspect != "" {
				_, err = inspectDomainSource(filepath.Join(root, tt.inspect), tt.inspect)
			} else {
				_, err = discoverDomainSources(root)
			}
			if tt.wantIs != nil {
				if !errors.Is(err, tt.wantIs) {
					t.Fatalf("error = %v, want %v", err, tt.wantIs)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

// TestRegisterToolsClientType_NoParameterList_ReturnsEmpty verifies a
// declaration without a parameter list yields no client type.
func TestRegisterToolsClientType_NoParameterList_ReturnsEmpty(t *testing.T) {
	if got := registerToolsClientType(token.NewFileSet(), &ast.FuncDecl{Type: &ast.FuncType{}}); got != "" {
		t.Fatalf("registerToolsClientType() = %q, want empty", got)
	}
}

// TestExprString_UnprintableNode_ReturnsEmpty verifies a node the printer
// rejects renders as the empty string instead of aborting the scan.
func TestExprString_UnprintableNode_ReturnsEmpty(t *testing.T) {
	if got := exprString(token.NewFileSet(), nil); got != "" {
		t.Fatalf("exprString(nil) = %q, want empty", got)
	}
}

// TestReferencedPackages_Scenarios_CollectsQualifiers verifies the selector
// scan records the package qualifier of every <pkg>.RegisterTools reference,
// ignores nested selectors, bare calls and other selectors, and reports a
// file that does not parse.
func TestReferencedPackages_Scenarios_CollectsQualifiers(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		want    []string
		wantErr string
	}{
		{
			name:   "qualifier references",
			source: "package tools\n\nfunc f() {\n\talpha.RegisterTools(nil, nil)\n\tnested.pkg.RegisterTools(nil, nil)\n\tbeta.Other()\n\tRegisterTools()\n\tgamma.RegisterTools(nil, nil)\n}\n",
			want:   []string{"alpha", "gamma"},
		},
		{name: "unparsable file", source: "package tools\n\nfunc {\n", wantErr: "parse "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), registerGoFile)
			writeAuditTestFile(t, path, tt.source)
			got, err := referencedPackages(path, "RegisterTools")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("referencedPackages() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("referencedPackages() error = %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("referencedPackages() = %v, want %v", got, tt.want)
			}
			for _, name := range tt.want {
				if !got[name] {
					t.Errorf("referencedPackages() lacks %q: %v", name, got)
				}
			}
		})
	}
}

// TestRecordSurfaceSpecs_OwnerlessSpec_IsSkipped verifies specs without an
// owner package contribute nothing while owned specs are counted under
// their owner with their surface kind and group.
func TestRecordSurfaceSpecs_OwnerlessSpec_IsSkipped(t *testing.T) {
	coverage := map[string]packageActionCoverage{}
	recordSurfaceSpecs(coverage, []actioncatalog.SurfaceToolSpec{
		{OwnerPackage: "  ", GroupToolName: "gitlab_ignored"},
		{OwnerPackage: "owner", GroupToolName: "gitlab_owned", SurfaceKind: actioncatalog.SurfaceKindDynamicController},
	})
	if len(coverage) != 1 {
		t.Fatalf("coverage = %v, want the owned spec only", coverage)
	}
	owned := coverage["owner"]
	if owned.SurfaceSpecCount != 1 || owned.UtilitySurfaceActionCount != 1 || owned.OrdinaryGitLabActionCount != 0 {
		t.Errorf("owned coverage = %+v, want one utility surface spec", owned)
	}
	if _, ok := owned.MetaGroups["gitlab_owned"]; !ok {
		t.Errorf("meta groups = %v, want gitlab_owned", owned.MetaGroups)
	}
}

// TestRecordActionSpecGroups_Scenarios_CountSpecsUnderTheirOwner verifies a
// group's actions are counted under the package that owns each of them, with
// the group's tool name and its surface kind, and that an action naming no
// owner is skipped.
//
// The ownerless case is the one this repository has never been in: every spec
// compiled in names an owner, so an action that lost its owner would leave the
// report silently smaller with no test noticing.
func TestRecordActionSpecGroups_Scenarios_CountSpecsUnderTheirOwner(t *testing.T) {
	coverage := map[string]packageActionCoverage{}
	recordActionSpecGroups(coverage, []tools.ActionSpecGroup{{
		ToolName:    "gitlab_owned",
		SurfaceKind: actioncatalog.SurfaceKindGitLabAction,
		Actions: []toolutil.ActionSpec{
			{Name: "get", OwnerPackage: "owner"},
			{Name: "list", OwnerPackage: "owner"},
			{Name: "orphaned", OwnerPackage: "  "},
		},
	}})

	if len(coverage) != 1 {
		t.Fatalf("coverage = %v, want the owned package only", coverage)
	}
	owned := coverage["owner"]
	if owned.ActionSpecCount != 2 || owned.OrdinaryGitLabActionCount != 2 || owned.DynamicCatalogActionCount != 0 {
		t.Errorf("owned coverage = %+v, want 2 spec actions and no catalog actions", owned)
	}
	if _, ok := owned.MetaGroups["gitlab_owned"]; !ok || len(owned.MetaGroups) != 1 {
		t.Errorf("meta groups = %v, want gitlab_owned alone", owned.MetaGroups)
	}
}

// TestRecordCatalogActions_Scenarios_CountActionsUnderTheirOwner verifies the
// assembled catalog is counted into the row's own field, under the owner each
// action names and with the tool name the action carries, and that a nil
// catalog contributes nothing.
//
// The two counters this and [recordActionSpecGroups] fill answer different
// questions — what a package contributes and what the catalog serves — so this
// asserts the whole row rather than the one number it raises.
//
// The fallback case is also the property that makes the ownerless skip beside
// it unobservable: an action a catalog hands back always names a domain, since
// normalizing a group derives one from the group's tool name where the action
// declares none, so the owner this resolves is never empty.
func TestRecordCatalogActions_Scenarios_CountActionsUnderTheirOwner(t *testing.T) {
	catalog := actioncatalog.NewCatalog()
	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_example"})
	group.SetAction(actioncatalog.Action{ID: "example.get", Name: "get", ToolName: "gitlab_example", OwnerPackage: "owner"})
	group.SetAction(actioncatalog.Action{ID: "fallback.list", Name: "list", ToolName: "gitlab_example", Domain: "fallback"})
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}

	coverage := map[string]packageActionCoverage{}
	recordCatalogActions(coverage, catalog)

	owned := coverage["owner"]
	if owned.DynamicCatalogActionCount != 1 || owned.ActionSpecCount != 0 || owned.SurfaceSpecCount != 0 {
		t.Errorf("owned coverage = %+v, want one catalog action and nothing else", owned)
	}
	if _, ok := owned.MetaGroups["gitlab_example"]; !ok {
		t.Errorf("meta groups = %v, want gitlab_example", owned.MetaGroups)
	}
	if fallback := coverage["fallback"]; fallback.DynamicCatalogActionCount != 1 {
		t.Errorf("fallback coverage = %+v, want the domain used where no owner is named", fallback)
	}

	empty := map[string]packageActionCoverage{}
	recordCatalogActions(empty, nil)
	if len(empty) != 0 {
		t.Errorf("recordCatalogActions(nil) filled %v, want nothing", empty)
	}
}

// TestRecordSurfaceKind_ZeroValue_AllocatesCounts verifies the kind counter
// allocates its map on first use.
func TestRecordSurfaceKind_ZeroValue_AllocatesCounts(t *testing.T) {
	var coverage packageActionCoverage
	coverage.recordSurfaceKind(actioncatalog.SurfaceKindGitLabAction)
	if coverage.SurfaceKindCounts[string(actioncatalog.SurfaceKindGitLabAction)] != 1 {
		t.Fatalf("SurfaceKindCounts = %v, want one gitlab-action", coverage.SurfaceKindCounts)
	}
}

// TestClassifySurface_Scenarios_OrdersRules verifies each classification
// rule in precedence order against synthetic source and coverage facts.
func TestClassifySurface_Scenarios_OrdersRules(t *testing.T) {
	tests := []struct {
		name     string
		source   domainSource
		coverage domainCoverage
		want     string
	}{
		{name: "dynamic controller surface", source: domainSource{HasDynamicCatalogRegistration: true}, coverage: domainCoverage{HasSurfaceSpecs: true}, want: "dynamic-controller-surface"},
		{name: "surface backed by utility actions", coverage: domainCoverage{UtilitySurfaceActionCount: 1}, want: "surface-backed"},
		{name: "dynamic catalog surface", source: domainSource{HasDynamicCatalogRegistration: true}, want: "dynamic-catalog-surface"},
		{name: "spec backed", coverage: domainCoverage{HasIndividualTools: true, HasMetaSpecs: true}, want: "spec-backed"},
		// The second operand of each two-way case, which the case above it
		// short-circuits past: a domain backed by the dynamic catalog alone is
		// spec-backed and catalog-only on the same terms as one backed by
		// ActionSpecs, and nothing said so until these two.
		{name: "spec backed by dynamic catalog entries", coverage: domainCoverage{HasIndividualTools: true, HasDynamicCatalogEntries: true}, want: "spec-backed"},
		{name: "individual only", coverage: domainCoverage{HasIndividualTools: true}, want: "individual-only"},
		{name: "standalone meta", source: domainSource{HasRegisterMeta: true}, coverage: domainCoverage{HasDynamicCatalogEntries: true}, want: "standalone-meta"},
		{name: "catalog only", coverage: domainCoverage{HasMetaSpecs: true}, want: "catalog-only"},
		{name: "catalog only by dynamic catalog entries", coverage: domainCoverage{HasDynamicCatalogEntries: true}, want: "catalog-only"},
		{name: "standalone only", coverage: domainCoverage{HasStandaloneOnlyTools: true}, want: "standalone-only"},
		{name: "register meta without catalog entries", source: domainSource{HasRegisterMeta: true}, want: "standalone-only"},
		{name: "nothing discovered", want: noGitLabSurface},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifySurface(tt.source, tt.coverage); got != tt.want {
				t.Errorf("classifySurface() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestCoverageNotes_Scenarios_ExplainsRegistrationState verifies the notes
// name an unreferenced RegisterTools, a delegated RegisterMeta, and a domain
// without any surface, and that a RegisterTools the root file does reference
// is worth no note at all.
//
// That last case is what the note is for: it reports the mismatch, so a
// package register.go names must produce silence, or the note says nothing
// about which of the two states a reader is looking at.
func TestCoverageNotes_Scenarios_ExplainsRegistrationState(t *testing.T) {
	tests := []struct {
		name     string
		source   domainSource
		coverage domainCoverage
		want     []string
	}{
		{name: "unreferenced RegisterTools", source: domainSource{HasRegisterTools: true}, want: []string{"RegisterTools is not referenced from internal/tools/register.go"}},
		{name: "referenced RegisterTools", source: domainSource{HasRegisterTools: true}, coverage: domainCoverage{RegisteredInRegisterAll: true, SurfaceClassification: "spec-backed"}},
		{name: "delegated RegisterMeta", source: domainSource{HasRegisterMeta: true}, coverage: domainCoverage{DelegatedMeta: true}, want: []string{"delegated RegisterMeta is referenced from internal/tools/register_meta.go"}},
		{name: "no surface", coverage: domainCoverage{SurfaceClassification: noGitLabSurface}, want: []string{"no GitLab action surface discovered from source or catalog metadata"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notes := coverageNotes(tt.source, tt.coverage)
			if !slices.Equal(notes, tt.want) {
				t.Errorf("coverageNotes() = %v, want %v", notes, tt.want)
			}
		})
	}
}

// TestSummarizeCoverage_NestedDomains_CountEachFormApart verifies every
// counter of the summary against a set of domains that tells it from all the
// others.
//
// The summary is eight counters raised by eight one-line branches and three
// running totals, which is the shape where a branch that counts into its
// neighbour's field is invisible: a fixture in which each of them reads 1 is
// satisfied by any permutation of the eight. The flags are therefore nested,
// so each counter ends on a different number, the three totals are given
// values that share no sum, and the whole summary is compared at once.
func TestSummarizeCoverage_NestedDomains_CountEachFormApart(t *testing.T) {
	summary := summarizeCoverage([]domainCoverage{
		{
			Package: "a", HasRegisterTools: true, HasRegisterMeta: true, HasMetaSpecs: true,
			HasDynamicCatalogEntries: true, HasSurfaceSpecs: true, HasStandaloneOnlyTools: true,
			SurfaceClassification: noGitLabSurface, SurfaceSpecCount: 2,
			OrdinaryGitLabActionCount: 1, UtilitySurfaceActionCount: 4,
			SurfaceKindCounts: map[string]int{"gitlab-action": 2},
		},
		{
			Package: "b", HasRegisterTools: true, HasRegisterMeta: true, HasMetaSpecs: true,
			HasDynamicCatalogEntries: true, HasSurfaceSpecs: true, HasStandaloneOnlyTools: true,
			SurfaceClassification: "standalone-only", SurfaceSpecCount: 15,
			SurfaceKindCounts: map[string]int{"gitlab-action": 1, "meta-group": 1},
		},
		{
			Package: "c", HasRegisterTools: true, HasRegisterMeta: true, HasMetaSpecs: true,
			HasDynamicCatalogEntries: true, HasSurfaceSpecs: true,
			SurfaceClassification: "surface-backed", OrdinaryGitLabActionCount: 10, UtilitySurfaceActionCount: 9,
		},
		{Package: "d", HasRegisterTools: true, HasRegisterMeta: true, HasMetaSpecs: true, HasDynamicCatalogEntries: true, SurfaceClassification: "spec-backed"},
		{Package: "e", HasRegisterTools: true, HasRegisterMeta: true, HasMetaSpecs: true, SurfaceClassification: "catalog-only"},
		{Package: "f", HasRegisterTools: true, HasRegisterMeta: true, SurfaceClassification: "standalone-meta"},
		{Package: "g", HasRegisterTools: true, SurfaceClassification: "individual-only"},
	})

	want := coverageSummary{
		DomainCount:                7,
		RegisterToolsCount:         7,
		RegisterMetaCount:          6,
		ActionSpecDomainCount:      5,
		DynamicCatalogDomainCount:  4,
		SurfaceSpecDomainCount:     3,
		StandaloneOnlyDomainCount:  2,
		NoGitLabActionSurfaceCount: 1,
		OrdinaryGitLabActionCount:  11,
		UtilitySurfaceActionCount:  13,
		SurfaceSpecCount:           17,
		SurfaceClassificationCounts: map[string]int{
			noGitLabSurface:   1,
			"standalone-only": 1,
			"surface-backed":  1,
			"spec-backed":     1,
			"catalog-only":    1,
			"standalone-meta": 1,
			"individual-only": 1,
		},
		SurfaceKindCounts: map[string]int{"gitlab-action": 3, "meta-group": 1},
	}
	if !reflect.DeepEqual(summary, want) {
		t.Errorf("summarizeCoverage() = %+v, want %+v", summary, want)
	}
}

// TestJoinSortedSet_Scenarios_JoinsSortedOrEmpty verifies an empty set joins
// to the empty string and a populated set joins its sorted members.
func TestJoinSortedSet_Scenarios_JoinsSortedOrEmpty(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]struct{}
		want   string
	}{
		{name: "empty", values: nil, want: ""},
		{name: "sorted members", values: map[string]struct{}{"gitlab_b": {}, "gitlab_a": {}}, want: "gitlab_a,gitlab_b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := joinSortedSet(tt.values); got != tt.want {
				t.Errorf("joinSortedSet() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestProductionFileCallsSelector_FindsCallsAndParsesErrors verifies the file
// scanner matches a call only when both halves of the selector agree, and
// surfaces parse errors with the expected prefix.
//
// The two near misses are what the rule stands or falls on: a call to the same
// method name through another package is not the forbidden call, and neither
// is one reached through a value rather than a package name, where the
// qualifier the rule compares is not an identifier at all. Both used to be
// branches no file took, so a scan that answered "found" for either would have
// condemned production source that is fine.
func TestProductionFileCallsSelector_FindsCallsAndParsesErrors(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		selector string
		want     bool
	}{
		{name: "matching call", source: "package x\nfunc f() { tools.RegisterTools(s, c) }\n", selector: "RegisterTools", want: true},
		{name: "another selector on the same qualifier", source: "package x\nfunc f() { tools.RegisterTools(s, c) }\n", selector: "RegisterMeta"},
		{name: "same selector on another qualifier", source: "package x\nfunc f() { other.RegisterTools(s, c) }\n", selector: "RegisterTools"},
		{name: "qualifier is not a package name", source: "package x\nfunc f() { nested.pkg.RegisterTools(s, c) }\n", selector: "RegisterTools"},
		{name: "selector is not called", source: "package x\nvar f = tools.RegisterTools\n", selector: "RegisterTools"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "calls.go")
			writeAuditTestFile(t, path, tt.source)
			found, err := productionFileCallsSelector(token.NewFileSet(), path, "tools", tt.selector)
			if err != nil {
				t.Fatalf("productionFileCallsSelector() error = %v", err)
			}
			if found != tt.want {
				t.Errorf("productionFileCallsSelector() = %t, want %t", found, tt.want)
			}
		})
	}

	// A file that is not there surfaces a parse-prefixed error.
	_, err := productionFileCallsSelector(token.NewFileSet(), filepath.Join(t.TempDir(), "missing.go"), "tools", "RegisterTools")
	if err == nil {
		t.Fatal("productionFileCallsSelector(missing) error = nil, want parse failure")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Fatalf("productionFileCallsSelector(missing) error = %v, want parse prefix", err)
	}
}

// TestSurfaceKinds_Scenarios_DropsTheEmptyKind verifies the kind list is
// sorted, that an empty map carries no list at all, and that a kind with no
// name is left out rather than rendered as an empty entry.
//
// A report listing "" beside the real kinds would read as a surface this
// repository has no name for, and nothing had ever put an unnamed kind in.
func TestSurfaceKinds_Scenarios_DropsTheEmptyKind(t *testing.T) {
	tests := []struct {
		name   string
		counts map[string]int
		want   []string
	}{
		{name: "no counts carry no kinds", counts: nil},
		{name: "kinds are sorted", counts: map[string]int{"meta-group": 2, "gitlab-action": 1}, want: []string{"gitlab-action", "meta-group"}},
		{name: "the unnamed kind is left out", counts: map[string]int{"": 3, "gitlab-action": 1}, want: []string{"gitlab-action"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := surfaceKinds(tt.counts); !slices.Equal(got, tt.want) {
				t.Errorf("surfaceKinds(%v) = %v, want %v", tt.counts, got, tt.want)
			}
		})
	}
}

// TestProjectionPolicyError_Scenarios_NamesTheUnprojectedActions verifies the
// verdict half of the projection rule: a catalog whose actions all project is
// no finding, and one carrying an action with no individual tool name is
// reported with that action named.
//
// The catalog the rule runs against in production is the one compiled into
// this binary, where nothing is missing, so this is the only place the
// emitting branch is taken at all: without it the rule could stop reporting
// and stay green.
func TestProjectionPolicyError_Scenarios_NamesTheUnprojectedActions(t *testing.T) {
	catalogWith := func(action actioncatalog.Action) *actioncatalog.Catalog {
		t.Helper()
		catalog := actioncatalog.NewCatalog()
		group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_example"})
		group.SetAction(action)
		if err := catalog.AddGroup(group); err != nil {
			t.Fatalf("AddGroup() error = %v", err)
		}
		return catalog
	}

	projected := catalogWith(actioncatalog.Action{ID: "example.get", Name: "get", IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_example_get"}})
	if err := projectionPolicyError(projected); err != nil {
		t.Errorf("projectionPolicyError(projected) = %v, want nil", err)
	}

	err := projectionPolicyError(catalogWith(actioncatalog.Action{ID: "example.get", Name: "get"}))
	if err == nil {
		t.Fatal("projectionPolicyError(unprojected) = nil, want the action named")
	}
	if !strings.Contains(err.Error(), "example.get") {
		t.Errorf("projectionPolicyError() = %v, want it to name example.get", err)
	}
}

// TestDomainCoverageFor_DistinctInputs_LandOnTheirOwnFields verifies that each
// count, each set and each of the two registration maps reaches the field of
// the row that names it.
//
// The row carries five counts, three string-shaped fields and nine flags
// assigned one after another, which is the shape where a line reading its
// neighbour's value is invisible: every count here is a different number and
// the two registration maps name different packages, so a pair that traded
// places changes the record this compares. The whole row is compared rather
// than a field at a time for the same reason.
func TestDomainCoverageFor_DistinctInputs_LandOnTheirOwnFields(t *testing.T) {
	source := domainSource{
		Package:     "alpha",
		HasMarkdown: true,
		HasTests:    true,
		ClientType:  "*gitlabclient.Client",
	}
	actionCoverage := map[string]packageActionCoverage{
		"alpha": {
			ActionSpecCount:           2,
			OrdinaryGitLabActionCount: 3,
			UtilitySurfaceActionCount: 5,
			DynamicCatalogActionCount: 7,
			SurfaceSpecCount:          11,
			SurfaceKindCounts:         map[string]int{"gitlab-action": 3, "dynamic-controller": 5},
			MetaGroups:                map[string]struct{}{"gitlab_zeta": {}, "gitlab_alpha": {}},
		},
		"beta": {ActionSpecCount: 97},
	}

	got := domainCoverageFor(source, actionCoverage, map[string]bool{"alpha": true}, map[string]bool{"beta": true})

	want := domainCoverage{
		Package:                   "alpha",
		HasMarkdown:               true,
		HasTests:                  true,
		SurfaceClassification:     "surface-backed",
		ClientType:                "*gitlabclient.Client",
		MetaGroup:                 "gitlab_alpha,gitlab_zeta",
		RegisteredInRegisterAll:   true,
		HasMetaSpecs:              true,
		HasIndividualTools:        true,
		HasDynamicCatalogEntries:  true,
		HasSurfaceSpecs:           true,
		ActionSpecCount:           2,
		OrdinaryGitLabActionCount: 3,
		UtilitySurfaceActionCount: 5,
		DynamicCatalogActionCount: 7,
		SurfaceSpecCount:          11,
		SurfaceKinds:              []string{"dynamic-controller", "gitlab-action"},
		SurfaceKindCounts:         map[string]int{"gitlab-action": 3, "dynamic-controller": 5},
		Notes: []string{
			"11 explicit surface specs: dynamic-controller,gitlab-action",
			"5 utility/controller actions are outside ordinary GitLab API action counting",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("domainCoverageFor() = %+v, want %+v", got, want)
	}
}

// TestDomainCoverageFor_SourceFlags_AreCarriedOneAtATime verifies each source
// flag on its own, since a struct of booleans has no fixture in which no two
// values agree: setting several together is what hides one flag reading
// another's value.
//
// It is also where the client-type half of the legacy classification is
// stated. A RegisterTools taking a GitLab client is an individual-only domain
// and one taking anything else is a standalone surface, and the whole
// difference between those two verdicts is [isGitLabClientType], which no test
// had seen answer false at this call site.
func TestDomainCoverageFor_SourceFlags_AreCarriedOneAtATime(t *testing.T) {
	const noSurfaceNote = "no GitLab action surface discovered from source or catalog metadata"
	tests := []struct {
		name   string
		source domainSource
		want   domainCoverage
	}{
		{
			name:   "nothing discovered",
			source: domainSource{Package: "alpha"},
			want:   domainCoverage{Package: "alpha", SurfaceClassification: noGitLabSurface, Notes: []string{noSurfaceNote}},
		},
		{
			name:   "markdown only",
			source: domainSource{Package: "alpha", HasMarkdown: true},
			want:   domainCoverage{Package: "alpha", HasMarkdown: true, SurfaceClassification: noGitLabSurface, Notes: []string{noSurfaceNote}},
		},
		{
			name:   "tests only",
			source: domainSource{Package: "alpha", HasTests: true},
			want:   domainCoverage{Package: "alpha", HasTests: true, SurfaceClassification: noGitLabSurface, Notes: []string{noSurfaceNote}},
		},
		{
			name:   "ActionSpecs only",
			source: domainSource{Package: "alpha", HasActionSpecsFunction: true},
			want: domainCoverage{
				Package:               "alpha",
				SurfaceClassification: "spec-backed",
				HasMetaSpecs:          true,
				HasIndividualTools:    true,
				Notes:                 []string{},
			},
		},
		{
			name:   "dynamic catalog registration only",
			source: domainSource{Package: "alpha", HasDynamicCatalogRegistration: true},
			want: domainCoverage{
				Package:               "alpha",
				SurfaceClassification: "dynamic-catalog-surface",
				Notes:                 []string{"dynamic controller surface registered from the canonical action catalog"},
			},
		},
		{
			name:   "RegisterMeta only",
			source: domainSource{Package: "alpha", HasRegisterMeta: true},
			want: domainCoverage{
				Package:               "alpha",
				HasRegisterMeta:       true,
				SurfaceClassification: "standalone-only",
				Notes:                 []string{},
			},
		},
		{
			name:   "RegisterTools taking a GitLab client",
			source: domainSource{Package: "alpha", HasRegisterTools: true, ClientType: "*gitlabclient.Client"},
			want: domainCoverage{
				Package:               "alpha",
				HasRegisterTools:      true,
				ClientType:            "*gitlabclient.Client",
				SurfaceClassification: "individual-only",
				HasIndividualTools:    true,
				Notes:                 []string{"RegisterTools is not referenced from internal/tools/register.go"},
			},
		},
		{
			name:   "RegisterTools taking anything else",
			source: domainSource{Package: "alpha", HasRegisterTools: true, ClientType: "*mcp.Server"},
			want: domainCoverage{
				Package:                "alpha",
				HasRegisterTools:       true,
				ClientType:             "*mcp.Server",
				SurfaceClassification:  "standalone-only",
				HasStandaloneOnlyTools: true,
				Notes: []string{
					"RegisterTools does not use a GitLab client constructor",
					"RegisterTools is not referenced from internal/tools/register.go",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domainCoverageFor(tt.source, nil, nil, nil)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("domainCoverageFor() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestBuildCoverageReport_Domains_AreSortedByPackage states the property the
// report's sort holds, and which is also what makes its comparator
// unobservable: os.ReadDir hands the domain walk its entries already in
// filename order and a domain's package name is its directory name, so the
// rows arrive sorted and the comparator never reorders a pair.
//
// The sort stays because ordered rows are the report's contract rather than an
// accident of how the walk happens to read a directory, and this is the
// assertion a reader can check that contract against.
func TestBuildCoverageReport_Domains_AreSortedByPackage(t *testing.T) {
	files := catalogFirstFixtureFiles()
	for _, name := range []string{"zeta", "kappa", "beta"} {
		files["internal/tools/"+name+"/"+name+".go"] = "package " + name + "\n"
	}
	root := writeCatalogFirstFixture(t, files)

	report, err := buildCoverageReport(root)
	if err != nil {
		t.Fatalf("buildCoverageReport() error = %v", err)
	}
	packages := make([]string, 0, len(report.Domains))
	for _, domain := range report.Domains {
		packages = append(packages, domain.Package)
	}
	if !slices.IsSorted(packages) {
		t.Errorf("report domains = %v, want them sorted by package", packages)
	}
	if !slices.Contains(packages, "kappa") {
		t.Errorf("report domains = %v, want the planted domains among them", packages)
	}
}

// TestBuildCoverageReport_GeneratedDirectories_AreNotProductionSource verifies
// a directory named dist or site is left unread by the selector audit wherever
// it sits.
//
// They hold generated output and an Astro build rather than production Go, so
// a file inside one that calls the forbidden selector must not condemn the
// tree. Nothing planted either before, so the arm that skips them was never
// taken and a list that stopped naming them would have failed nothing.
func TestBuildCoverageReport_GeneratedDirectories_AreNotProductionSource(t *testing.T) {
	const callsForbiddenSelector = "func f() { toolutil.CaptureMetaToolDefinitions() }\n"

	files := catalogFirstFixtureFiles()
	files["dist/generated.go"] = "package dist\n\n" + callsForbiddenSelector
	files["site/embedded.go"] = "package site\n\n" + callsForbiddenSelector
	root := writeCatalogFirstFixture(t, files)

	report, err := buildCoverageReport(root)
	if err != nil {
		t.Fatalf("buildCoverageReport() error = %v, want the generated directories left unread", err)
	}
	if report.Summary.DomainCount != 2 {
		t.Errorf("summary domain count = %d, want the 2 domains under internal/tools", report.Summary.DomainCount)
	}
}

// TestDiscoverDomainSources_DomainTheSelectorAuditSkipped_IsStillParsed
// verifies the report refuses a domain directory whose source does not parse
// even where the selector audit passed over it.
//
// The two walks do not read the same set: the selector audit skips any
// directory called dist wherever it sits, while the domain walk inspects every
// directory under internal/tools by name. A domain that lands on that name is
// therefore the one input that reaches the domain walk unparsed, and it is the
// only way the report's own parse failure is observable at all.
func TestDiscoverDomainSources_DomainTheSelectorAuditSkipped_IsStillParsed(t *testing.T) {
	files := catalogFirstFixtureFiles()
	files["internal/tools/dist/broken.go"] = "package dist\n\nfunc {\n"
	root := writeCatalogFirstFixture(t, files)

	if err := assertNoProductionSelectorCall(root, "toolutil", "CaptureMetaToolDefinitions"); err != nil {
		t.Fatalf("assertNoProductionSelectorCall() error = %v, want the dist directory skipped", err)
	}
	_, err := buildCoverageReport(root)
	if err == nil || !strings.Contains(err.Error(), "broken.go") {
		t.Fatalf("buildCoverageReport() error = %v, want it to name broken.go", err)
	}
}

// TestAuditCatalogFirstSource_UnparsableRegistrationFile_IsRefusedFirst
// verifies the source audit refuses a registration file that does not parse.
//
// It is the property that makes the report's own two parse branches
// unreachable: [buildCoverageReport] reads register.go and register_meta.go
// again through [referencedPackages], and by the time it does, this audit has
// already parsed both as production source. Those two error arms are therefore
// kept for the values they return rather than for a state a tree can be in,
// and this is what has to keep holding for that to stay true.
func TestAuditCatalogFirstSource_UnparsableRegistrationFile_IsRefusedFirst(t *testing.T) {
	for _, name := range []string{registerGoFile, "register_meta.go"} {
		t.Run(name, func(t *testing.T) {
			files := catalogFirstFixtureFiles()
			files["internal/tools/"+name] = "package tools\n\nfunc {\n"
			root := writeCatalogFirstFixture(t, files)

			err := auditCatalogFirstSource(root)
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("auditCatalogFirstSource() error = %v, want it to name %s", err, name)
			}
		})
	}
}

// catalogFirstModuleFixture is [catalogFirstFixtureFiles] plus the two things
// that make the fixture a module go/packages can load: a go.mod carrying this
// repository's own module path, which is what the aggregation rule matches a
// package against, and the builder type the generated manifest names.
func catalogFirstModuleFixture() map[string]string {
	files := catalogFirstFixtureFiles()
	files["go.mod"] = "module " + strings.TrimSuffix(toolsPathPrefix, "/internal/tools/") + "\n\ngo 1.27\n"
	files["internal/tools/action_specs_manifest_gen.go"] = "package tools\n\ntype actionSpecGroupBuilder func()\n\nfunc actionSpecGroupBuilders() []actionSpecGroupBuilder {\n\treturn []actionSpecGroupBuilder{\n\t\tbuildAlphaActionSpecs,\n\t}\n}\n"
	return files
}

// TestRunMain_Scenarios_ReportsTheExitCodeAndTheRefusal drives the command
// itself over a planted repository: every argument it accepts and refuses,
// every state it exits 1 for, and the report it writes when the tree passes.
//
// Until runMain existed each of these ended in cmdutil.Fatalf, so the only way
// to observe one was to start a process and read its status. A run that exited
// 0 with the aggregation rule failing, that wrote the report somewhere other
// than where -output named, or that reported a refusal nobody could read,
// failed no test in this package.
func TestRunMain_Scenarios_ReportsTheExitCodeAndTheRefusal(t *testing.T) {
	tests := []struct {
		name string
		args []string
		// mutate breaks the planted module in one way; nil leaves it clean.
		mutate func(files map[string]string)
		// outsideModule runs from a directory no go.mod sits above, which is
		// what the arguments are judged before reaching.
		outsideModule bool
		// blockedOutput points -output below a regular file, so the report has
		// nowhere to go.
		blockedOutput bool
		want          int
		wantStderr    string
		wantReport    bool
	}{
		{name: "unknown flag", args: []string{"-nope"}, outsideModule: true, want: 2, wantStderr: "flag provided but not defined"},
		{name: "help", args: []string{"-h"}, outsideModule: true, want: 0},
		{name: "outside a module", outsideModule: true, want: 1, wantStderr: "find repository root"},
		{
			name: "specs no production file aggregates",
			mutate: func(f map[string]string) {
				f["internal/tools/alpha/specs.go"] = "package alpha\n\nfunc ActionSpecs() {}\n"
			},
			want:       1,
			wantStderr: "declares an exported ActionSpecs",
		},
		{
			name:       "the source audit refuses",
			mutate:     func(f map[string]string) { f["CLAUDE.md"] = "# Claude\n\nCreate `register.go` with `RegisterTools`.\n" },
			want:       1,
			wantStderr: "build coverage report: AI context audit failed",
		},
		{name: "report cannot be written", blockedOutput: true, want: 1, wantStderr: "write coverage report"},
		{name: "clean tree", want: 0, wantReport: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(runMainWorkingDirectory(t, tt.outsideModule, tt.mutate))
			outputPath := runMainOutputPath(t, tt.blockedOutput)
			args := tt.args
			if args == nil {
				args = []string{"-output", outputPath}
			}

			var stderr bytes.Buffer
			got := runMain(args, &stderr)

			if got != tt.want {
				t.Errorf("runMain(%v) = %d, want %d (stderr: %s)", args, got, tt.want, stderr.String())
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("runMain(%v) stderr = %q, want it to contain %q", args, stderr.String(), tt.wantStderr)
			}
			if tt.wantReport {
				assertWrittenCoverageReport(t, outputPath)
			}
		})
	}
}

// runMainWorkingDirectory returns the directory a runMain case runs from: a
// planted module, broken in the one way the case asks for, or a bare
// temporary directory for the cases decided before any module is looked for.
func runMainWorkingDirectory(t *testing.T, outsideModule bool, mutate func(files map[string]string)) string {
	t.Helper()
	if outsideModule {
		return t.TempDir()
	}
	files := catalogFirstModuleFixture()
	if mutate != nil {
		mutate(files)
	}
	return writeCatalogFirstFixture(t, files)
}

// runMainOutputPath returns where a runMain case asks for its report: a fresh
// file, or a path below a regular file so the write has nowhere to go.
func runMainOutputPath(t *testing.T, blocked bool) string {
	t.Helper()
	if !blocked {
		return filepath.Join(t.TempDir(), "coverage.json")
	}
	blocker := filepath.Join(t.TempDir(), "blocker")
	writeAuditTestFile(t, blocker, "a regular file, not a directory\n")
	return filepath.Join(blocker, "coverage.json")
}

// assertWrittenCoverageReport reads back the file runMain wrote and holds it
// to being the report rather than merely a file that exists.
func assertWrittenCoverageReport(t *testing.T, path string) {
	t.Helper()
	content, err := os.ReadFile(path) // #nosec G304 -- the path is this test's own temp directory.
	if err != nil {
		t.Fatalf("reading the written report: %v", err)
	}
	var report coverageReport
	if unmarshalErr := json.Unmarshal(content, &report); unmarshalErr != nil {
		t.Fatalf("the written report does not parse: %v", unmarshalErr)
	}
	if report.SchemaVersion != schemaVersion || report.Summary.DomainCount == 0 {
		t.Errorf("written report = %+v, want schema %d and the planted domains", report.Summary, schemaVersion)
	}
}

// TestMain_ExitsWithTheCodeRunMainDecided verifies the entry point hands the
// process exactly what runMain returned, which is the one line of the command
// no other test crosses.
func TestMain_ExitsWithTheCodeRunMainDecided(t *testing.T) {
	previous := exitProcess
	t.Cleanup(func() { exitProcess = previous })

	codes := []int{}
	exitProcess = func(code int) { codes = append(codes, code) }
	previousArgs := os.Args
	t.Cleanup(func() { os.Args = previousArgs })
	os.Args = []string{"audit_catalog_first", "-nope"}
	t.Chdir(t.TempDir())

	main()

	if !slices.Equal(codes, []int{2}) {
		t.Errorf("exit codes = %v, want the 2 runMain returned for an unknown flag", codes)
	}
}
