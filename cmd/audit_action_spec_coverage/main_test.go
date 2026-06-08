package main

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
)

// TestBuildCoverageReport_ClassifiesKeyDomains verifies BuildCoverageReport classifies key domains.
func TestBuildCoverageReport_ClassifiesKeyDomains(t *testing.T) {
	root, err := cmdutil.RepositoryRoot("../..")
	if err != nil {
		t.Fatalf("cmdutil.RepositoryRoot() error = %v", err)
	}
	report, err := buildCoverageReport(root)
	if err != nil {
		t.Fatalf("buildCoverageReport() error = %v", err)
	}
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

	serverUpdate := requireDomain(t, report, "serverupdate")
	if !serverUpdate.HasStandaloneOnlyTools || serverUpdate.SurfaceClassification != "surface-backed" || serverUpdate.SurfaceSpecCount != 2 {
		t.Fatalf("serverupdate coverage missing server maintenance surface specs: %+v", serverUpdate)
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

// TestBuildCoverageReport_CoreSourceDomainsAreSpecBacked verifies BuildCoverageReport when core source domains are spec backed.
func TestBuildCoverageReport_CoreSourceDomainsAreSpecBacked(t *testing.T) {
	root, err := cmdutil.RepositoryRoot("../..")
	if err != nil {
		t.Fatalf("cmdutil.RepositoryRoot() error = %v", err)
	}
	report, err := buildCoverageReport(root)
	if err != nil {
		t.Fatalf("buildCoverageReport() error = %v", err)
	}

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
	root, err := cmdutil.RepositoryRoot("../..")
	if err != nil {
		t.Fatalf("cmdutil.RepositoryRoot() error = %v", err)
	}
	report, err := buildCoverageReport(root)
	if err != nil {
		t.Fatalf("buildCoverageReport() error = %v", err)
	}

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
	root, err := cmdutil.RepositoryRoot("../..")
	if err != nil {
		t.Fatalf("cmdutil.RepositoryRoot() error = %v", err)
	}
	report, err := buildCoverageReport(root)
	if err != nil {
		t.Fatalf("buildCoverageReport() error = %v", err)
	}

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
	root, err := cmdutil.RepositoryRoot("../..")
	if err != nil {
		t.Fatalf("cmdutil.RepositoryRoot() error = %v", err)
	}
	report, err := buildCoverageReport(root)
	if err != nil {
		t.Fatalf("buildCoverageReport() error = %v", err)
	}

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
	root, err := cmdutil.RepositoryRoot("../..")
	if err != nil {
		t.Fatalf("cmdutil.RepositoryRoot() error = %v", err)
	}
	report, err := buildCoverageReport(root)
	if err != nil {
		t.Fatalf("buildCoverageReport() error = %v", err)
	}

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
	root, err := cmdutil.RepositoryRoot("../..")
	if err != nil {
		t.Fatalf("cmdutil.RepositoryRoot() error = %v", err)
	}
	report, err := buildCoverageReport(root)
	if err != nil {
		t.Fatalf("buildCoverageReport() error = %v", err)
	}

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
	root, err := cmdutil.RepositoryRoot("../..")
	if err != nil {
		t.Fatalf("cmdutil.RepositoryRoot() error = %v", err)
	}
	report, err := buildCoverageReport(root)
	if err != nil {
		t.Fatalf("buildCoverageReport() error = %v", err)
	}

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
	root, err := cmdutil.RepositoryRoot("../..")
	if err != nil {
		t.Fatalf("cmdutil.RepositoryRoot() error = %v", err)
	}
	report, err := buildCoverageReport(root)
	if err != nil {
		t.Fatalf("buildCoverageReport() error = %v", err)
	}

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
	root, err := cmdutil.RepositoryRoot("../..")
	if err != nil {
		t.Fatalf("cmdutil.RepositoryRoot() error = %v", err)
	}
	report, err := buildCoverageReport(root)
	if err != nil {
		t.Fatalf("buildCoverageReport() error = %v", err)
	}

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
	assertSurfaceBackedDomain(t, report, "samplingtools", "sampling-utility", 11)
	assertSurfaceBackedDomain(t, report, "elicitationtools", "interactive-utility", 4)
	assertSurfaceBackedDomain(t, report, "projectdiscovery", "runtime-utility", 1)
	assertSurfaceBackedDomain(t, report, "serverupdate", "server-maintenance", 2)
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
