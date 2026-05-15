package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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

	projects := requireDomain(t, report, "projects")
	if !projects.HasRegisterTools || !projects.HasIndividualTools || !projects.HasMetaSpecs || !projects.HasDynamicCatalogEntries {
		t.Fatalf("projects coverage missing expected surfaces: %+v", projects)
	}
	if projects.SurfaceClassification != "spec-backed" {
		t.Fatalf("projects classification = %q, want spec-backed", projects.SurfaceClassification)
	}

	dynamic := requireDomain(t, report, "dynamic")
	if dynamic.SurfaceClassification != "dynamic-catalog-surface" {
		t.Fatalf("dynamic classification = %q, want dynamic-catalog-surface", dynamic.SurfaceClassification)
	}

	serverUpdate := requireDomain(t, report, "serverupdate")
	if !serverUpdate.HasStandaloneOnlyTools || serverUpdate.SurfaceClassification != "standalone-only" {
		t.Fatalf("serverupdate coverage missing standalone classification: %+v", serverUpdate)
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
		"samplingtools",
	})
	assertSourceSpecBackedDomains(t, report, []string{
		"elicitationtools",
		"projectdiscovery",
	})
	assertStandaloneOnlyDomain(t, report, "serverupdate")
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

func assertStandaloneOnlyDomain(t *testing.T, report coverageReport, packageName string) {
	t.Helper()
	domain := requireDomain(t, report, packageName)
	if domain.SurfaceClassification != "standalone-only" {
		t.Fatalf("%s classification = %q, want standalone-only", packageName, domain.SurfaceClassification)
	}
	if domain.HasIndividualTools || domain.HasMetaSpecs || domain.HasDynamicCatalogEntries {
		t.Fatalf("%s coverage should remain outside GitLab action specs/catalog: %+v", packageName, domain)
	}
}

func writeAuditTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}
