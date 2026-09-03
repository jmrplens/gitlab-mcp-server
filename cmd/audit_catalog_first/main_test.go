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
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

func assertArchitectureCoverage(t *testing.T, report coverageReport) {
	t.Helper()
	if report.Architecture.CatalogSource == "" || report.Architecture.MetaRegistrationSource == "" || report.Architecture.IndividualRegistrationSource == "" {
		t.Fatalf("architecture report missing source fields: %+v", report.Architecture)
	}
	if report.Architecture.SurfaceSpecCount != report.Summary.SurfaceSpecCount {
		t.Fatalf("architecture surface specs = %d, summary = %d", report.Architecture.SurfaceSpecCount, report.Summary.SurfaceSpecCount)
	}
	if report.Architecture.LegacyBridgeCount != 0 || len(report.Architecture.LegacyBridges) != 0 {
		t.Fatalf("architecture legacy bridges = %+v, want zero", report.Architecture.LegacyBridges)
	}
	if report.Architecture.DynamicActionAliasCount == 0 || report.Architecture.DynamicParameterAliasCount == 0 {
		t.Fatalf("architecture dynamic alias counts missing: %+v", report.Architecture)
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

// TestWriteReport_WritesJSONFile verifies WriteReport writes JSON file.
func TestWriteReport_WritesJSONFile(t *testing.T) {
	report := coverageReport{SchemaVersion: schemaVersion, Summary: coverageSummary{DomainCount: 1}, Domains: []domainCoverage{{Package: "example"}}}
	content, err := marshalReport(report)
	if err != nil {
		t.Fatalf("marshalReport() error = %v", err)
	}

	outputPath := filepath.Join(t.TempDir(), "coverage.json")
	writeErr := writeReport(outputPath, content)
	if writeErr != nil {
		t.Fatalf("writeReport() error = %v", writeErr)
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
		"internal/toolutil/metatool.go":               "package toolutil\n",
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
			name:    "metatool.go missing",
			mutate:  func(f map[string]string) { delete(f, "internal/toolutil/metatool.go") },
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
		"internal/toolutil/metatool.go":    "package toolutil\n",
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
	skip, err := skipSelectorAuditEntry(nil, walkErr)
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
	}{
		{name: "tools directory missing", files: map[string]string{}, wantErr: "read tools directory"},
		{name: "domain file does not parse", files: map[string]string{"internal/tools/alpha/alpha.go": "package alpha\n\nfunc {\n"}, wantErr: "parse "},
		{name: "domain directory missing", files: map[string]string{}, inspect: "absent", wantErr: "no such file or directory"},
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
		{name: "individual only", coverage: domainCoverage{HasIndividualTools: true}, want: "individual-only"},
		{name: "standalone meta", source: domainSource{HasRegisterMeta: true}, coverage: domainCoverage{HasDynamicCatalogEntries: true}, want: "standalone-meta"},
		{name: "catalog only", coverage: domainCoverage{HasMetaSpecs: true}, want: "catalog-only"},
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
// without any surface.
func TestCoverageNotes_Scenarios_ExplainsRegistrationState(t *testing.T) {
	tests := []struct {
		name     string
		source   domainSource
		coverage domainCoverage
		want     string
	}{
		{name: "unreferenced RegisterTools", source: domainSource{HasRegisterTools: true}, want: "RegisterTools is not referenced from internal/tools/register.go"},
		{name: "delegated RegisterMeta", source: domainSource{HasRegisterMeta: true}, coverage: domainCoverage{DelegatedMeta: true}, want: "delegated RegisterMeta is referenced from internal/tools/register_meta.go"},
		{name: "no surface", coverage: domainCoverage{SurfaceClassification: noGitLabSurface}, want: "no GitLab action surface discovered from source or catalog metadata"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notes := coverageNotes(tt.source, tt.coverage)
			if len(notes) != 1 || notes[0] != tt.want {
				t.Errorf("coverageNotes() = %v, want [%q]", notes, tt.want)
			}
		})
	}
}

// TestSummarizeCoverage_LegacyDomains_CountsRegistrationForms verifies the
// summary tallies package-local RegisterTools and RegisterMeta domains and
// merges the per-domain surface kind counts.
func TestSummarizeCoverage_LegacyDomains_CountsRegistrationForms(t *testing.T) {
	summary := summarizeCoverage([]domainCoverage{
		{Package: "a", HasRegisterTools: true, SurfaceKindCounts: map[string]int{"gitlab-action": 2}},
		{Package: "b", HasRegisterMeta: true, SurfaceKindCounts: map[string]int{"gitlab-action": 1, "meta-group": 1}},
	})
	if summary.DomainCount != 2 || summary.RegisterToolsCount != 1 || summary.RegisterMetaCount != 1 {
		t.Errorf("summary = %+v, want 2 domains with one RegisterTools and one RegisterMeta", summary)
	}
	if summary.SurfaceKindCounts["gitlab-action"] != 3 || summary.SurfaceKindCounts["meta-group"] != 1 {
		t.Errorf("SurfaceKindCounts = %v, want merged counts", summary.SurfaceKindCounts)
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

// captureStdout swaps os.Stdout for a temporary file until the test ends and
// returns a reader for what was written, so the "-" output path can be
// observed.
func captureStdout(t *testing.T) func() string {
	t.Helper()
	file, err := os.Create(filepath.Join(t.TempDir(), "stdout"))
	if err != nil {
		t.Fatalf("create stdout capture: %v", err)
	}
	previous := os.Stdout
	os.Stdout = file
	t.Cleanup(func() {
		os.Stdout = previous
		_ = file.Close()
	})
	return func() string {
		data, readErr := os.ReadFile(file.Name())
		if readErr != nil {
			t.Fatalf("read stdout capture: %v", readErr)
		}
		return string(data)
	}
}

// TestWriteReport_Scenarios_WritesStdoutOrFailsOnBlockedDirectory verifies
// the "-" sentinel writes the report to stdout and a report path whose parent
// directory cannot be created is reported as an error.
func TestWriteReport_Scenarios_WritesStdoutOrFailsOnBlockedDirectory(t *testing.T) {
	t.Run("stdout sentinel", func(t *testing.T) {
		stdout := captureStdout(t)
		if err := writeReport("-", []byte("{}\n")); err != nil {
			t.Fatalf("writeReport(-) error = %v", err)
		}
		if got := stdout(); got != "{}\n" {
			t.Errorf("stdout = %q, want the report", got)
		}
	})
	t.Run("blocked parent directory", func(t *testing.T) {
		blocker := filepath.Join(t.TempDir(), "blocker")
		writeAuditTestFile(t, blocker, "x")
		if err := writeReport(filepath.Join(blocker, "coverage.json"), []byte("{}\n")); err == nil {
			t.Fatal("writeReport() error = nil, want the directory creation failure")
		}
	})
}

// TestProductionFileCallsSelector_FindsCallsAndParsesErrors verifies the
// file scanner locates matching calls and surfaces parse errors with the
// expected prefix.
func TestProductionFileCallsSelector_FindsCallsAndParsesErrors(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "calls.go")
	writeAuditTestFile(t, goFile, "package x\nfunc f() { tools.RegisterTools(s, c) }\n")

	fileSet := token.NewFileSet()
	found, err := productionFileCallsSelector(fileSet, goFile, "tools", "RegisterTools")
	if err != nil {
		t.Fatalf("productionFileCallsSelector() error = %v", err)
	}
	if !found {
		t.Fatal("productionFileCallsSelector() = false, want true for matching call")
	}

	found, err = productionFileCallsSelector(fileSet, goFile, "tools", "RegisterMeta")
	if err != nil {
		t.Fatalf("productionFileCallsSelector(other) error = %v", err)
	}
	if found {
		t.Fatal("productionFileCallsSelector(other) = true, want false for missing call")
	}

	// Invalid file path surfaces a parse-prefixed error.
	_, err = productionFileCallsSelector(fileSet, filepath.Join(dir, "missing.go"), "tools", "RegisterTools")
	if err == nil {
		t.Fatal("productionFileCallsSelector(missing) error = nil, want parse failure")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Fatalf("productionFileCallsSelector(missing) error = %v, want parse prefix", err)
	}
}
