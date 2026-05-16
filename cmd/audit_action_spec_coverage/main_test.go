package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actioncatalog"
)

func TestBuildCoverageReport_ClassifiesKeyDomains(t *testing.T) {
	root, err := repositoryRoot("../..")
	if err != nil {
		t.Fatalf("repositoryRoot() error = %v", err)
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

	projects := requireDomain(t, report, "projects")
	if !projects.HasIndividualTools || !projects.HasMetaSpecs || !projects.HasDynamicCatalogEntries {
		t.Fatalf("projects coverage missing expected surfaces: %+v", projects)
	}
	if projects.SurfaceClassification != "spec-backed" {
		t.Fatalf("projects classification = %q, want spec-backed", projects.SurfaceClassification)
	}

	dynamic := requireDomain(t, report, "dynamic")
	if dynamic.SurfaceClassification != "dynamic-controller-surface" || !dynamic.HasSurfaceSpecs || dynamic.SurfaceSpecCount != 4 {
		t.Fatalf("dynamic coverage missing controller surface specs: %+v", dynamic)
	}

	serverUpdate := requireDomain(t, report, "serverupdate")
	if !serverUpdate.HasStandaloneOnlyTools || serverUpdate.SurfaceClassification != "surface-backed" || serverUpdate.SurfaceSpecCount != 2 {
		t.Fatalf("serverupdate coverage missing server maintenance surface specs: %+v", serverUpdate)
	}
}

func TestAuditCatalogFirstSource_CurrentProductionCodePasses(t *testing.T) {
	root, err := repositoryRoot("../..")
	if err != nil {
		t.Fatalf("repositoryRoot() error = %v", err)
	}
	if auditErr := auditCatalogFirstSource(root); auditErr != nil {
		t.Fatalf("auditCatalogFirstSource() error = %v", auditErr)
	}
}

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

func TestLegacyBridgeFindingsInContent_DetectsForbiddenReferences(t *testing.T) {
	findings := legacyBridgeFindingsInContent("runtime.go", "package tools\nfunc f(){ registerAllLegacy() }", []string{"registerAllLegacy"})
	if len(findings) != 1 || findings[0] != "runtime.go contains \"registerAllLegacy\"" {
		t.Fatalf("legacyBridgeFindingsInContent() = %+v, want registerAllLegacy finding", findings)
	}
}

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

func TestBuildCoverageReport_CoreSourceDomainsAreSpecBacked(t *testing.T) {
	root, err := repositoryRoot("../..")
	if err != nil {
		t.Fatalf("repositoryRoot() error = %v", err)
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

func TestBuildCoverageReport_CICDDomainsAreSpecBacked(t *testing.T) {
	root, err := repositoryRoot("../..")
	if err != nil {
		t.Fatalf("repositoryRoot() error = %v", err)
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

func TestBuildCoverageReport_CollaborationDomainsAreSpecBacked(t *testing.T) {
	root, err := repositoryRoot("../..")
	if err != nil {
		t.Fatalf("repositoryRoot() error = %v", err)
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

func TestBuildCoverageReport_NoteAndDiscussionDomainsAreSpecBacked(t *testing.T) {
	root, err := repositoryRoot("../..")
	if err != nil {
		t.Fatalf("repositoryRoot() error = %v", err)
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

func TestBuildCoverageReport_AccessAndSecurityDomainsAreSpecBacked(t *testing.T) {
	root, err := repositoryRoot("../..")
	if err != nil {
		t.Fatalf("repositoryRoot() error = %v", err)
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

func TestBuildCoverageReport_AdminPlatformDomainsAreSpecBacked(t *testing.T) {
	root, err := repositoryRoot("../..")
	if err != nil {
		t.Fatalf("repositoryRoot() error = %v", err)
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

func TestBuildCoverageReport_PackageDeploymentStorageDomainsAreSpecBacked(t *testing.T) {
	root, err := repositoryRoot("../..")
	if err != nil {
		t.Fatalf("repositoryRoot() error = %v", err)
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

func TestBuildCoverageReport_GroupProjectEnterpriseDomainsAreSpecBacked(t *testing.T) {
	root, err := repositoryRoot("../..")
	if err != nil {
		t.Fatalf("repositoryRoot() error = %v", err)
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

func TestBuildCoverageReport_UtilityTemplateDomainsAreSpecBacked(t *testing.T) {
	root, err := repositoryRoot("../..")
	if err != nil {
		t.Fatalf("repositoryRoot() error = %v", err)
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

func writeAuditTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}
