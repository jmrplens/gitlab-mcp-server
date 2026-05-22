// securityfindings_test.go contains unit tests for GitLab security finding
// operations. Tests use httptest to mock the GitLab Security Findings API.
package securityfindings

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// Sample GraphQL response payloads.

const sampleFindingNode = `{
  "uuid": "550e8400-e29b-41d4-a716-446655440001",
  "name": "Cross-site Scripting (XSS)",
  "title": "Potential XSS in template",
  "severity": "HIGH",
  "confidence": "MEDIUM",
  "reportType": "SAST",
  "scanner": {
    "name": "Semgrep",
    "vendor": "GitLab",
    "externalId": "semgrep-sast"
  },
  "description": "User input is rendered without escaping.",
  "solution": "Use textContent instead of innerHTML.",
  "identifiers": [
    {"name": "CWE-79", "externalType": "CWE", "externalId": "79", "url": "https://cwe.mitre.org/data/definitions/79.html"},
    {"name": "OWASP A7:2017", "externalType": "OWASP", "externalId": "A7-2017", "url": ""}
  ],
  "location": {
    "file": "src/app.js",
    "startLine": 42,
    "endLine": 42,
    "blobPath": "/src/app.js"
  },
  "state": "DETECTED",
  "evidence": "element.innerHTML = userInput;",
  "vulnerability": {
    "id": "gid://gitlab/Vulnerability/12345",
    "state": "DETECTED"
  }
}`

// graphqlMux wraps testutil.GraphQLHandler and registers it on /api/graphql.
func graphqlMux(handlers map[string]http.HandlerFunc) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/graphql", testutil.GraphQLHandler(handlers))
	return mux
}

// List tests.

// TestList_Success verifies that listing security findings returns the
// expected items when the GraphQL API responds with valid finding data.
func TestList_Success(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"securityReportFindings": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"project": {
					"pipeline": {
						"securityReportFindings": {
							"nodes": [`+sampleFindingNode+`],
							"pageInfo": {
								"hasNextPage": true,
								"hasPreviousPage": false,
								"endCursor": "cursor-abc",
								"startCursor": ""
							}
						}
					}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := List(context.Background(), client, ListInput{
		ProjectPath: "my-group/my-project",
		PipelineIID: "123",
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(out.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(out.Findings))
	}
	assertSampleSecurityFinding(t, out.Findings[0])
	assertSecurityFindingPagination(t, out)
}

func assertSampleSecurityFinding(t *testing.T, finding FindingItem) {
	t.Helper()
	if finding.UUID != "550e8400-e29b-41d4-a716-446655440001" || finding.Name != "Cross-site Scripting (XSS)" {
		t.Fatalf("finding identity = %+v, want sample XSS finding", finding)
	}
	if finding.Severity != "HIGH" || finding.Confidence != "MEDIUM" || finding.ReportType != "SAST" || finding.State != "DETECTED" {
		t.Errorf("finding classification = %+v, want HIGH/MEDIUM/SAST/DETECTED", finding)
	}
	if finding.Scanner == nil || finding.Scanner.Name != "Semgrep" || finding.Scanner.ExternalID != "semgrep-sast" {
		t.Errorf("Scanner = %+v, want Semgrep semgrep-sast", finding.Scanner)
	}
	if finding.Location == nil || finding.Location.File != "src/app.js" || finding.Location.StartLine != 42 {
		t.Errorf("Location = %+v, want src/app.js:42", finding.Location)
	}
	if len(finding.Identifiers) != 2 || finding.Identifiers[0].Name != "CWE-79" {
		t.Fatalf("Identifiers = %+v, want CWE-79 first plus second identifier", finding.Identifiers)
	}
	if finding.Evidence == nil || finding.Evidence.Data != "element.innerHTML = userInput;" {
		t.Errorf("Evidence.Data = %v, want evidence data", finding.Evidence)
	}
	if finding.VulnID != "gid://gitlab/Vulnerability/12345" {
		t.Errorf("VulnID = %q, want gid://gitlab/Vulnerability/12345", finding.VulnID)
	}
}

func assertSecurityFindingPagination(t *testing.T, out ListOutput) {
	t.Helper()
	if !out.Pagination.HasNextPage {
		t.Error("expected HasNextPage=true")
	}
	if out.Pagination.EndCursor != "cursor-abc" {
		t.Errorf("EndCursor = %q, want cursor-abc", out.Pagination.EndCursor)
	}
}

// TestList_EmptyProjectPath verifies that listing security findings returns
// a validation error when the required project_path parameter is missing.
func TestList_EmptyProjectPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := List(context.Background(), client, ListInput{
		PipelineIID: "123",
	})
	if err == nil {
		t.Fatal("expected error for empty project_path, got nil")
	}
}

// TestList_EmptyPipelineIID verifies that listing security findings returns
// a validation error when the required pipeline_iid parameter is missing.
func TestList_EmptyPipelineIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := List(context.Background(), client, ListInput{
		ProjectPath: "my-group/my-project",
	})
	if err == nil {
		t.Fatal("expected error for empty pipeline_iid, got nil")
	}
}

// TestList_WithFilters verifies that severity and report_type filters are
// correctly forwarded to the GraphQL API when listing security findings.
func TestList_WithFilters(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"securityReportFindings": func(w http.ResponseWriter, r *http.Request) {
			vars, err := testutil.ParseGraphQLVariables(r)
			if err != nil {
				t.Fatalf("ParseGraphQLVariables error: %v", err)
			}
			if vars["projectPath"] != "my-group/my-project" {
				t.Errorf("projectPath = %v, want my-group/my-project", vars["projectPath"])
			}
			if vars["pipelineIID"] != "456" {
				t.Errorf("pipelineIID = %v, want 456", vars["pipelineIID"])
			}
			for _, key := range []string{"severity", "confidence", "scanner", "reportType"} {
				if _, ok := vars[key]; !ok {
					t.Errorf("GraphQL variables missing %q", key)
				}
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"project": {
					"pipeline": {
						"securityReportFindings": {
							"nodes": [],
							"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": "", "startCursor": ""}
						}
					}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := List(context.Background(), client, ListInput{
		ProjectPath: "my-group/my-project",
		PipelineIID: "456",
		Severity:    []string{"HIGH", "CRITICAL"},
		Confidence:  []string{"CONFIRMED"},
		Scanner:     []string{"semgrep-sast"},
		ReportType:  []string{"SAST"},
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(out.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(out.Findings))
	}
}

// TestList_ProjectNilExistingProjectReturnsEmpty verifies unavailable findings are not mistaken for missing projects.
func TestList_ProjectNilExistingProjectReturnsEmpty(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/api/graphql", testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"securityReportFindings": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"project": null}`)
		},
	}))
	mux.HandleFunc("/api/v4/projects/my-group%2Fmy-project", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"path_with_namespace":"my-group/my-project"}`)
	})

	client := testutil.NewTestClient(t, mux)
	out, err := List(context.Background(), client, ListInput{ProjectPath: "my-group/my-project", PipelineIID: "456"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(out.Findings) != 0 {
		t.Fatalf("expected empty findings, got %d", len(out.Findings))
	}
}

// TestList_ProjectNilMissingProjectErrors verifies a null GraphQL project still checks the REST project identity.
func TestList_ProjectNilMissingProjectErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/api/graphql", testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"securityReportFindings": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"project": null}`)
		},
	}))
	mux.HandleFunc("/api/v4/projects/my-group%2Fmy-project", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	})

	client := testutil.NewTestClient(t, mux)
	_, err := List(context.Background(), client, ListInput{ProjectPath: "my-group/my-project", PipelineIID: "456"})
	if err == nil {
		t.Fatal("expected missing project error")
	}
	if !strings.Contains(err.Error(), "project \"my-group/my-project\" not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestList_EmptyResults verifies that listing security findings returns
// an empty result set when no findings match the query.
func TestList_EmptyResults(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"securityReportFindings": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"project": {
					"pipeline": {
						"securityReportFindings": {
							"nodes": [],
							"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": "", "startCursor": ""}
						}
					}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := List(context.Background(), client, ListInput{
		ProjectPath: "my-group/my-project",
		PipelineIID: "789",
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(out.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(out.Findings))
	}
	if out.Pagination.HasNextPage {
		t.Error("expected HasNextPage=false")
	}
}

// TestList_PipelineNotFound verifies that a null GraphQL pipeline is reported
// as an actionable pipeline_iid error instead of a false empty findings list.
func TestList_PipelineNotFound(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"securityReportFindings": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"project": {
					"pipeline": null
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	_, err := List(context.Background(), client, ListInput{
		ProjectPath: "my-group/my-project",
		PipelineIID: "999",
	})
	if err == nil {
		t.Fatal("expected error for missing pipeline_iid")
	}
	errText := err.Error()
	for _, want := range []string{"pipeline_iid", "gitlab_pipeline", "security scan report artifacts"} {
		if !strings.Contains(errText, want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
}

// TestList_ServerError verifies that listing security findings propagates
// errors when the GraphQL API returns a server error.
func TestList_ServerError(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"securityReportFindings": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "bad request", http.StatusBadRequest)
		},
	})

	client := testutil.NewTestClient(t, handler)
	_, err := List(context.Background(), client, ListInput{
		ProjectPath: "my-group/my-project",
		PipelineIID: "123",
	})
	if err == nil {
		t.Fatal("expected error from HTTP 500 response, got nil")
	}
}

// TestList_DastLocation verifies that DAST-type findings with hostname and
// path location fields are correctly mapped to the output struct.
func TestList_DastLocation(t *testing.T) {
	dastNode := `{
		"uuid": "dast-uuid-001",
		"name": "Server Information Leak",
		"title": "",
		"severity": "LOW",
		"confidence": "LOW",
		"reportType": "DAST",
		"scanner": {"name": "OWASP ZAP", "vendor": "owasp", "externalId": "zap-dast"},
		"description": "Server version exposed in headers.",
		"solution": "",
		"identifiers": [],
		"location": {"path": "/api/v1/health"},
		"state": "DETECTED",
		"evidence": null,
		"vulnerability": null
	}`

	handler := graphqlMux(map[string]http.HandlerFunc{
		"securityReportFindings": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"project": {
					"pipeline": {
						"securityReportFindings": {
							"nodes": [`+dastNode+`],
							"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": "", "startCursor": ""}
						}
					}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := List(context.Background(), client, ListInput{
		ProjectPath: "my-group/my-project",
		PipelineIID: "100",
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(out.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(out.Findings))
	}
	f := out.Findings[0]
	if f.Location == nil || f.Location.File != "/api/v1/health" {
		t.Errorf("DAST Location.File = %v, want /api/v1/health (from path)", f.Location)
	}
	if f.VulnID != "" {
		t.Errorf("VulnID = %q, want empty (no linked vulnerability)", f.VulnID)
	}
	if f.Evidence != nil {
		t.Errorf("Evidence = %v, want nil", f.Evidence)
	}
}

// TestList_ContainerScanningLocation verifies that container scanning findings
// with image and operating_system location fields are correctly mapped.
func TestList_ContainerScanningLocation(t *testing.T) {
	containerNode := `{
		"uuid": "container-uuid-001",
		"name": "CVE-2026-9999",
		"title": "Critical flaw in base image",
		"severity": "CRITICAL",
		"confidence": "CONFIRMED",
		"reportType": "CONTAINER_SCANNING",
		"scanner": {"name": "Trivy", "vendor": "aquasecurity", "externalId": "trivy"},
		"description": "",
		"solution": "Upgrade base image.",
		"identifiers": [{"name": "CVE-2026-9999", "externalType": "CVE", "externalId": "CVE-2026-9999", "url": ""}],
		"location": {"image": "registry.example.com/myapp:latest"},
		"state": "DETECTED",
		"evidence": null,
		"vulnerability": null
	}`

	handler := graphqlMux(map[string]http.HandlerFunc{
		"securityReportFindings": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"project": {
					"pipeline": {
						"securityReportFindings": {
							"nodes": [`+containerNode+`],
							"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": "", "startCursor": ""}
						}
					}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := List(context.Background(), client, ListInput{
		ProjectPath: "my-group/my-project",
		PipelineIID: "200",
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	f := out.Findings[0]
	if f.Location == nil || f.Location.File != "registry.example.com/myapp:latest" {
		t.Errorf("Container Location.File = %v, want registry.example.com/myapp:latest", f.Location)
	}
}

// Markdown formatter tests.

// TestFormatListMarkdown_Empty verifies that formatting an empty security
// findings list produces the expected no-results Markdown message.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No security findings found") {
		t.Error("expected 'No security findings found' in empty output")
	}
}

// TestFormatListMarkdown_WithItems verifies that formatting security findings
// produces a Markdown table with severity badges, scanner, and location.
func TestFormatListMarkdown_WithItems(t *testing.T) {
	out := ListOutput{
		Findings: []FindingItem{
			{
				UUID:       "uuid-1",
				Name:       "XSS Vulnerability",
				Severity:   "HIGH",
				Confidence: "MEDIUM",
				ReportType: "SAST",
				Scanner:    &ScannerItem{Name: "Semgrep"},
				Location:   &LocationItem{File: "app.js", StartLine: 10},
				State:      "DETECTED",
			},
		},
	}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "🟠 HIGH") {
		t.Error("expected severity badge in output")
	}
	if !strings.Contains(md, "XSS Vulnerability") {
		t.Error("expected finding name in output")
	}
	if !strings.Contains(md, "Semgrep") {
		t.Error("expected scanner name in output")
	}
	if !strings.Contains(md, "app.js:10") {
		t.Error("expected location in output")
	}
}

// TestFormatListMarkdown_TitleOverridesName verifies that when a finding has
// both Name and Title fields, the Title takes precedence in Markdown output.
func TestFormatListMarkdown_TitleOverridesName(t *testing.T) {
	out := ListOutput{
		Findings: []FindingItem{
			{
				UUID:       "uuid-1",
				Name:       "CWE-79",
				Title:      "Potential XSS in template",
				Severity:   "MEDIUM",
				ReportType: "SAST",
				State:      "DETECTED",
			},
		},
	}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "Potential XSS in template") {
		t.Error("expected title to be used instead of name")
	}
}

// TestSeverityBadge verifies that severityBadge returns the correct
// emoji-prefixed labels for each severity level.
func TestSeverityBadge(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"CRITICAL", "🔴 CRITICAL"},
		{"HIGH", "🟠 HIGH"},
		{"MEDIUM", "🟡 MEDIUM"},
		{"LOW", "🔵 LOW"},
		{"INFO", "ℹ️ INFO"},
		{"UNKNOWN", "UNKNOWN"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := severityBadge(tc.input)
			if got != tc.want {
				t.Errorf("severityBadge(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestFormatLocation verifies that formatLocation renders file locations
// in the expected file:line-line format for various input combinations.
func TestFormatLocation(t *testing.T) {
	tests := []struct {
		name  string
		input *LocationItem
		want  string
	}{
		{"nil", nil, ""},
		{"file only", &LocationItem{File: "main.go"}, "main.go"},
		{"file with line", &LocationItem{File: "main.go", StartLine: 5}, "main.go:5"},
		{"file with range", &LocationItem{File: "main.go", StartLine: 5, EndLine: 10}, "main.go:5-10"},
		{"same line range", &LocationItem{File: "main.go", StartLine: 5, EndLine: 5}, "main.go:5"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatLocation(tc.input)
			if got != tc.want {
				t.Errorf("formatLocation() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestActionSpecs_CallRoute verifies that the security findings canonical route
// executes successfully through ActionSpecs.
func TestActionSpecs_CallRoute(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/api/graphql", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"project":{"pipeline":{"securityReportFindings":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}}`)
	})
	client := testutil.NewTestClient(t, handler)
	specs := ActionSpecs(client)
	if len(specs) != 1 {
		t.Fatalf("len(ActionSpecs) = %d, want 1", len(specs))
	}
	spec := specs[0]
	if spec.IndividualTool.Name != "gitlab_list_security_findings" || spec.OwnerPackage != "securityfindings" {
		t.Fatalf("unexpected ActionSpec: %+v", spec)
	}
	result, err := spec.Route.Handler(t.Context(), map[string]any{
		"project_path": "group/project",
		"pipeline_iid": "42",
	})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	if result == nil {
		t.Fatal("Route.Handler returned nil")
	}
}

// TestList_APIError verifies that List wraps HTTP errors properly.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"internal server error"}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectPath: "g/p", PipelineIID: "1"})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

// TestList_WithConfidenceAndScanner verifies that confidence and scanner
// filters are correctly forwarded to the GraphQL API as query variables.
// This targets the two optional-filter branches that copy non-empty
// arrays into the variables map.
func TestList_WithConfidenceAndScanner(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"securityReportFindings": func(w http.ResponseWriter, r *http.Request) {
			vars, err := testutil.ParseGraphQLVariables(r)
			if err != nil {
				t.Fatalf("ParseGraphQLVariables error: %v", err)
			}
			if got, ok := vars["confidence"].([]any); !ok || len(got) != 1 || got[0] != "HIGH" {
				t.Errorf("confidence = %v, want [HIGH]", vars["confidence"])
			}
			if got, ok := vars["scanner"].([]any); !ok || len(got) != 1 || got[0] != "semgrep-sast" {
				t.Errorf("scanner = %v, want [semgrep-sast]", vars["scanner"])
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"project": {
					"pipeline": {
						"securityReportFindings": {
							"nodes": [],
							"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": "", "startCursor": ""}
						}
					}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := List(context.Background(), client, ListInput{
		ProjectPath: "g/p",
		PipelineIID: "1",
		Confidence:  []string{"HIGH"},
		Scanner:     []string{"semgrep-sast"},
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(out.Findings))
	}
}

// TestList_NullProject verifies that List returns a not-found error when
// the GraphQL API responds with a null project (project does not exist
// or user has no access). This targets the resp.Data.Project == nil
// branch that produces a domain-specific error message.
func TestList_NullProject(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"securityReportFindings": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"project": null}`)
		},
	})
	client := testutil.NewTestClient(t, handler)
	_, err := List(context.Background(), client, ListInput{
		ProjectPath: "missing/proj",
		PipelineIID: "1",
	})
	if err == nil {
		t.Fatal("expected error for null project, got nil")
	}
	if !strings.Contains(err.Error(), "missing/proj") {
		t.Errorf("error = %q, want contains project path", err.Error())
	}
}
