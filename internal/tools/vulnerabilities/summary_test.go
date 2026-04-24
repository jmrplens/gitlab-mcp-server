// summary_test.go contains unit tests for GitLab vulnerability summary
// retrieval operations. Tests use httptest to mock the GitLab Vulnerabilities API.

package vulnerabilities

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// Severity count tests.

// TestSeverityCount_Success verifies that counting vulnerability severities
// returns the expected breakdown by severity level.
func TestSeverityCount_Success(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"vulnerabilitySeveritiesCount": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"project": {
					"vulnerabilitySeveritiesCount": {
						"critical": 5,
						"high": 12,
						"medium": 23,
						"low": 8,
						"info": 3,
						"unknown": 1
					}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := SeverityCount(context.Background(), client, SeverityCountInput{
		ProjectPath: "my-group/my-project",
	})
	if err != nil {
		t.Fatalf("SeverityCount() error = %v", err)
	}

	if out.Critical != 5 {
		t.Errorf("Critical = %d, want 5", out.Critical)
	}
	if out.High != 12 {
		t.Errorf("High = %d, want 12", out.High)
	}
	if out.Medium != 23 {
		t.Errorf("Medium = %d, want 23", out.Medium)
	}
	if out.Low != 8 {
		t.Errorf("Low = %d, want 8", out.Low)
	}
	if out.Info != 3 {
		t.Errorf("Info = %d, want 3", out.Info)
	}
	if out.Unknown != 1 {
		t.Errorf("Unknown = %d, want 1", out.Unknown)
	}
	if out.Total != 52 {
		t.Errorf("Total = %d, want 52", out.Total)
	}
}

// TestSeverityCount_ZeroCounts verifies that severity counts with all-zero
// values are correctly reported.
func TestSeverityCount_ZeroCounts(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"vulnerabilitySeveritiesCount": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"project": {
					"vulnerabilitySeveritiesCount": {
						"critical": 0,
						"high": 0,
						"medium": 0,
						"low": 0,
						"info": 0,
						"unknown": 0
					}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := SeverityCount(context.Background(), client, SeverityCountInput{
		ProjectPath: "clean-project",
	})
	if err != nil {
		t.Fatalf("SeverityCount() error = %v", err)
	}

	if out.Total != 0 {
		t.Errorf("Total = %d, want 0", out.Total)
	}
}

// TestSeverityCount_MissingProjectPath verifies that counting severities
// returns a validation error when the required project_path is missing.
func TestSeverityCount_MissingProjectPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := SeverityCount(context.Background(), client, SeverityCountInput{})
	if err == nil {
		t.Fatal("expected error for empty project_path, got nil")
	}
}

// TestSeverityCount_ServerError verifies that counting severities propagates
// errors when the GraphQL API returns a server error.
func TestSeverityCount_ServerError(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"vulnerabilitySeveritiesCount": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "bad request", http.StatusBadRequest)
		},
	})

	client := testutil.NewTestClient(t, handler)
	_, err := SeverityCount(context.Background(), client, SeverityCountInput{
		ProjectPath: "my-group/my-project",
	})
	if err == nil {
		t.Fatal("expected error on server error, got nil")
	}
}

// Pipeline security summary tests.

// TestPipelineSecuritySummary_Success verifies that the pipeline security
// summary returns scanner results with vulnerability counts.
func TestPipelineSecuritySummary_Success(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"securityReportSummary": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"project": {
					"pipeline": {
						"securityReportSummary": {
							"sast": {
								"vulnerabilitiesCount": 10,
								"scannedResourcesCount": 150,
								"scannedResourcesCsvPath": "/downloads/sast.csv"
							},
							"dast": {
								"vulnerabilitiesCount": 3,
								"scannedResourcesCount": 50,
								"scannedResourcesCsvPath": ""
							},
							"dependencyScanning": {
								"vulnerabilitiesCount": 7,
								"scannedResourcesCount": 200,
								"scannedResourcesCsvPath": ""
							},
							"containerScanning": null,
							"secretDetection": {
								"vulnerabilitiesCount": 1,
								"scannedResourcesCount": 300,
								"scannedResourcesCsvPath": ""
							},
							"coverageFuzzing": null,
							"apiFuzzing": null,
							"clusterImageScanning": null
						}
					}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := PipelineSecuritySummary(context.Background(), client, PipelineSecuritySummaryInput{
		ProjectPath: "my-group/my-project",
		PipelineIID: "42",
	})
	if err != nil {
		t.Fatalf("PipelineSecuritySummary() error = %v", err)
	}

	if out.Sast == nil {
		t.Fatal("expected SAST summary, got nil")
	}
	if out.Sast.VulnerabilitiesCount != 10 {
		t.Errorf("SAST vulnerabilities = %d, want 10", out.Sast.VulnerabilitiesCount)
	}
	if out.Sast.ScannedResourcesCount != 150 {
		t.Errorf("SAST scanned resources = %d, want 150", out.Sast.ScannedResourcesCount)
	}

	if out.Dast == nil {
		t.Fatal("expected DAST summary, got nil")
	}
	if out.Dast.VulnerabilitiesCount != 3 {
		t.Errorf("DAST vulnerabilities = %d, want 3", out.Dast.VulnerabilitiesCount)
	}

	if out.DependencyScanning == nil {
		t.Fatal("expected DependencyScanning summary, got nil")
	}
	if out.DependencyScanning.VulnerabilitiesCount != 7 {
		t.Errorf("DependencyScanning vulnerabilities = %d, want 7", out.DependencyScanning.VulnerabilitiesCount)
	}

	if out.ContainerScanning != nil {
		t.Errorf("expected nil ContainerScanning, got %+v", out.ContainerScanning)
	}

	if out.SecretDetection == nil {
		t.Fatal("expected SecretDetection summary, got nil")
	}
	if out.SecretDetection.VulnerabilitiesCount != 1 {
		t.Errorf("SecretDetection vulnerabilities = %d, want 1", out.SecretDetection.VulnerabilitiesCount)
	}

	if out.TotalVulnerabilities != 21 {
		t.Errorf("TotalVulnerabilities = %d, want 21", out.TotalVulnerabilities)
	}
}

// TestPipelineSecuritySummary_AllScanners verifies that all scanner types
// (SAST, DAST, dependency scanning, container scanning, secret detection) are included.
func TestPipelineSecuritySummary_AllScanners(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"securityReportSummary": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"project": {
					"pipeline": {
						"securityReportSummary": {
							"sast": {"vulnerabilitiesCount": 2, "scannedResourcesCount": 10, "scannedResourcesCsvPath": ""},
							"dast": {"vulnerabilitiesCount": 3, "scannedResourcesCount": 20, "scannedResourcesCsvPath": ""},
							"dependencyScanning": {"vulnerabilitiesCount": 4, "scannedResourcesCount": 30, "scannedResourcesCsvPath": ""},
							"containerScanning": {"vulnerabilitiesCount": 1, "scannedResourcesCount": 5, "scannedResourcesCsvPath": ""},
							"secretDetection": {"vulnerabilitiesCount": 0, "scannedResourcesCount": 100, "scannedResourcesCsvPath": ""},
							"coverageFuzzing": {"vulnerabilitiesCount": 5, "scannedResourcesCount": 50, "scannedResourcesCsvPath": ""},
							"apiFuzzing": {"vulnerabilitiesCount": 2, "scannedResourcesCount": 15, "scannedResourcesCsvPath": ""},
							"clusterImageScanning": {"vulnerabilitiesCount": 1, "scannedResourcesCount": 8, "scannedResourcesCsvPath": ""}
						}
					}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := PipelineSecuritySummary(context.Background(), client, PipelineSecuritySummaryInput{
		ProjectPath: "my-group/my-project",
		PipelineIID: "99",
	})
	if err != nil {
		t.Fatalf("PipelineSecuritySummary() error = %v", err)
	}

	if out.Sast == nil || out.Dast == nil || out.DependencyScanning == nil ||
		out.ContainerScanning == nil || out.SecretDetection == nil ||
		out.CoverageFuzzing == nil || out.APIFuzzing == nil || out.ClusterImageScanning == nil {
		t.Fatal("expected all 8 scanners to be non-nil")
	}

	if out.TotalVulnerabilities != 18 {
		t.Errorf("TotalVulnerabilities = %d, want 18", out.TotalVulnerabilities)
	}
}

// TestPipelineSecuritySummary_PipelineNotFound verifies that the pipeline
// security summary returns an error when the specified pipeline does not exist.
func TestPipelineSecuritySummary_PipelineNotFound(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"securityReportSummary": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"project": {
					"pipeline": null
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	_, err := PipelineSecuritySummary(context.Background(), client, PipelineSecuritySummaryInput{
		ProjectPath: "my-group/my-project",
		PipelineIID: "999",
	})
	if err == nil {
		t.Fatal("expected error for pipeline not found, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain 'not found'", err.Error())
	}
}

// TestPipelineSecuritySummary_NoSecurityScans verifies that a pipeline
// without security scans returns zero scanner entries.
func TestPipelineSecuritySummary_NoSecurityScans(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"securityReportSummary": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"project": {
					"pipeline": {
						"securityReportSummary": null
					}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := PipelineSecuritySummary(context.Background(), client, PipelineSecuritySummaryInput{
		ProjectPath: "my-group/my-project",
		PipelineIID: "10",
	})
	if err != nil {
		t.Fatalf("PipelineSecuritySummary() error = %v", err)
	}
	if out.TotalVulnerabilities != 0 {
		t.Errorf("TotalVulnerabilities = %d, want 0", out.TotalVulnerabilities)
	}
	if out.Sast != nil {
		t.Errorf("expected nil SAST, got %+v", out.Sast)
	}
}

// TestPipelineSecuritySummary_MissingProjectPath verifies that the pipeline
// security summary returns a validation error when project_path is missing.
func TestPipelineSecuritySummary_MissingProjectPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := PipelineSecuritySummary(context.Background(), client, PipelineSecuritySummaryInput{
		PipelineIID: "42",
	})
	if err == nil {
		t.Fatal("expected error for empty project_path, got nil")
	}
}

// TestPipelineSecuritySummary_MissingPipelineIID verifies that the pipeline
// security summary returns a validation error when pipeline_iid is missing.
func TestPipelineSecuritySummary_MissingPipelineIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := PipelineSecuritySummary(context.Background(), client, PipelineSecuritySummaryInput{
		ProjectPath: "my-group/my-project",
	})
	if err == nil {
		t.Fatal("expected error for empty pipeline_iid, got nil")
	}
}

// TestPipelineSecuritySummary_ServerError verifies that the pipeline security
// summary propagates errors when the GraphQL API returns a server error.
func TestPipelineSecuritySummary_ServerError(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"securityReportSummary": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "bad request", http.StatusBadRequest)
		},
	})

	client := testutil.NewTestClient(t, handler)
	_, err := PipelineSecuritySummary(context.Background(), client, PipelineSecuritySummaryInput{
		ProjectPath: "my-group/my-project",
		PipelineIID: "42",
	})
	if err == nil {
		t.Fatal("expected error on server error, got nil")
	}
}

// Markdown formatter tests.

// TestFormatSeverityCountMarkdown_WithCounts verifies that formatting severity
// counts produces a Markdown block with the total and per-severity breakdown.
func TestFormatSeverityCountMarkdown_WithCounts(t *testing.T) {
	out := SeverityCountOutput{
		Critical: 5, High: 12, Medium: 23, Low: 8, Info: 3, Unknown: 1, Total: 52,
	}
	md := FormatSeverityCountMarkdown(out)
	if !strings.Contains(md, "Severity Counts") {
		t.Error("expected heading in markdown")
	}
	if !strings.Contains(md, "CRITICAL") {
		t.Error("expected CRITICAL label")
	}
	if !strings.Contains(md, "**52**") {
		t.Error("expected total count 52 in bold")
	}
}

// TestFormatSeverityCountMarkdown_AllZero verifies that formatting severity
// counts with all zeros produces the expected clean-state Markdown message.
func TestFormatSeverityCountMarkdown_AllZero(t *testing.T) {
	out := SeverityCountOutput{}
	md := FormatSeverityCountMarkdown(out)
	if !strings.Contains(md, "**0**") {
		t.Error("expected total 0 in bold")
	}
}

// TestFormatPipelineSecuritySummaryMarkdown_WithScanners verifies that formatting
// a pipeline security summary produces a Markdown table with scanner details.
func TestFormatPipelineSecuritySummaryMarkdown_WithScanners(t *testing.T) {
	out := PipelineSecuritySummaryOutput{
		Sast:                 &ScannerSummaryItem{VulnerabilitiesCount: 10, ScannedResourcesCount: 150},
		Dast:                 &ScannerSummaryItem{VulnerabilitiesCount: 3, ScannedResourcesCount: 50},
		TotalVulnerabilities: 13,
	}
	md := FormatPipelineSecuritySummaryMarkdown(out)
	if !strings.Contains(md, "SAST") {
		t.Error("expected SAST row")
	}
	if !strings.Contains(md, "DAST") {
		t.Error("expected DAST row")
	}
	if !strings.Contains(md, "13") {
		t.Error("expected total 13")
	}
}

// TestFormatPipelineSecuritySummaryMarkdown_Empty verifies that formatting
// an empty pipeline security summary produces the expected no-scanners Markdown.
func TestFormatPipelineSecuritySummaryMarkdown_Empty(t *testing.T) {
	out := PipelineSecuritySummaryOutput{}
	md := FormatPipelineSecuritySummaryMarkdown(out)
	if !strings.Contains(md, "No security scans") {
		t.Error("expected empty message")
	}
}
