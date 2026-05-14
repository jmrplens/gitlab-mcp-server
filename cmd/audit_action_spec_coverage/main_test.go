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
