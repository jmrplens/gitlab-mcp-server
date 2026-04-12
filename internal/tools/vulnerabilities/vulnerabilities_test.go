package vulnerabilities

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// Sample GraphQL response payloads.

const sampleVulnNode = `{
  "id": "gid://gitlab/Vulnerability/42",
  "title": "SQL Injection in login",
  "severity": "CRITICAL",
  "state": "DETECTED",
  "reportType": "SAST",
  "detectedAt": "2024-01-15T10:00:00Z",
  "dismissedAt": null,
  "resolvedAt": null,
  "confirmedAt": null,
  "primaryIdentifier": {
    "name": "CWE-89",
    "externalType": "cwe",
    "externalId": "89",
    "url": "https://cwe.mitre.org/data/definitions/89.html"
  },
  "scanner": {
    "name": "semgrep",
    "vendor": "GitLab"
  },
  "location": {
    "file": "app/controllers/sessions_controller.rb",
    "startLine": 42,
    "endLine": 42,
    "blobPath": "/project/-/blob/main/app/controllers/sessions_controller.rb"
  }
}`

const sampleVulnGetNode = `{
  "id": "gid://gitlab/Vulnerability/42",
  "title": "SQL Injection in login",
  "severity": "CRITICAL",
  "state": "DETECTED",
  "description": "User input is concatenated into SQL query without sanitization.",
  "reportType": "SAST",
  "detectedAt": "2024-01-15T10:00:00Z",
  "dismissedAt": null,
  "resolvedAt": null,
  "confirmedAt": null,
  "solution": "Use parameterized queries.",
  "hasSolutions": true,
  "dismissalReason": null,
  "primaryIdentifier": {
    "name": "CWE-89",
    "externalType": "cwe",
    "externalId": "89",
    "url": "https://cwe.mitre.org/data/definitions/89.html"
  },
  "identifiers": [
    {"name": "CWE-89", "externalType": "cwe", "externalId": "89", "url": "https://cwe.mitre.org/data/definitions/89.html"},
    {"name": "CVE-2024-1234", "externalType": "cve", "externalId": "CVE-2024-1234", "url": ""}
  ],
  "scanner": {
    "name": "semgrep",
    "vendor": "GitLab"
  },
  "location": {
    "file": "app/controllers/sessions_controller.rb",
    "startLine": 42,
    "endLine": 42,
    "blobPath": "/project/-/blob/main/app/controllers/sessions_controller.rb"
  },
  "project": {
    "id": "gid://gitlab/Project/1",
    "name": "my-project",
    "fullPath": "my-group/my-project"
  },
  "issueLinks": {"nodes": [{"id": "gid://gitlab/Vulnerabilities::IssueLink/1"}]},
  "mergeRequest": {"iid": "5"}
}`

const sampleMutationVuln = `{
  "id": "gid://gitlab/Vulnerability/42",
  "title": "SQL Injection in login",
  "severity": "CRITICAL",
  "state": "DISMISSED",
  "reportType": "SAST",
  "detectedAt": "2024-01-15T10:00:00Z",
  "dismissedAt": "2024-02-01T12:00:00Z",
  "resolvedAt": null,
  "confirmedAt": null,
  "dismissalReason": "FALSE_POSITIVE",
  "primaryIdentifier": {
    "name": "CWE-89",
    "externalType": "cwe",
    "externalId": "89",
    "url": ""
  },
  "scanner": {
    "name": "semgrep",
    "vendor": "GitLab"
  }
}`

// Test helpers.

// graphqlMux returns an [http.Handler] that routes GraphQL requests to the
// appropriate handler based on the query operation name.
func graphqlMux(handlers map[string]http.HandlerFunc) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/graphql", testutil.GraphQLHandler(handlers))
	return mux
}

// List tests.

// TestList_Success verifies that listing vulnerabilities returns the expected
// items when the GraphQL API responds with valid vulnerability data.
func TestList_Success(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"vulnerabilities": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"project": {
					"vulnerabilities": {
						"nodes": [`+sampleVulnNode+`],
						"pageInfo": {
							"hasNextPage": true,
							"hasPreviousPage": false,
							"endCursor": "cursor123",
							"startCursor": ""
						}
					}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := List(context.Background(), client, ListInput{
		ProjectPath: "my-group/my-project",
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(out.Vulnerabilities) != 1 {
		t.Fatalf("expected 1 vulnerability, got %d", len(out.Vulnerabilities))
	}
	v := out.Vulnerabilities[0]
	if v.ID != "gid://gitlab/Vulnerability/42" {
		t.Errorf("ID = %q, want %q", v.ID, "gid://gitlab/Vulnerability/42")
	}
	if v.Severity != "CRITICAL" {
		t.Errorf("Severity = %q, want CRITICAL", v.Severity)
	}
	if v.State != "DETECTED" {
		t.Errorf("State = %q, want DETECTED", v.State)
	}
	if v.Scanner == nil || v.Scanner.Name != "semgrep" {
		t.Errorf("Scanner.Name = %v, want semgrep", v.Scanner)
	}
	if v.Location == nil || v.Location.File != "app/controllers/sessions_controller.rb" {
		t.Errorf("Location.File = %v, want app/controllers/sessions_controller.rb", v.Location)
	}
	if v.PrimaryID == nil || v.PrimaryID.Name != "CWE-89" {
		t.Errorf("PrimaryID.Name = %v, want CWE-89", v.PrimaryID)
	}
	if v.Title != "SQL Injection in login" {
		t.Errorf("Title = %q, want %q", v.Title, "SQL Injection in login")
	}
	if v.ReportType != "SAST" {
		t.Errorf("ReportType = %q, want %q", v.ReportType, "SAST")
	}
	if v.DetectedAt != "2024-01-15T10:00:00Z" {
		t.Errorf("DetectedAt = %q, want %q", v.DetectedAt, "2024-01-15T10:00:00Z")
	}
	if !out.Pagination.HasNextPage {
		t.Error("expected HasNextPage=true")
	}
	if out.Pagination.EndCursor != "cursor123" {
		t.Errorf("EndCursor = %q, want cursor123", out.Pagination.EndCursor)
	}
}

// TestList_EmptyProjectPath verifies that listing vulnerabilities returns
// a validation error when the required project_path parameter is missing.
func TestList_EmptyProjectPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for empty project_path, got nil")
	}
}

// TestList_WithFilters verifies that severity and state filters are
// correctly forwarded to the GraphQL API when listing vulnerabilities.
func TestList_WithFilters(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"vulnerabilities": func(w http.ResponseWriter, r *http.Request) {
			vars, err := testutil.ParseGraphQLVariables(r)
			if err != nil {
				t.Fatalf("ParseGraphQLVariables error: %v", err)
			}
			if vars["projectPath"] != "my-group/my-project" {
				t.Errorf("projectPath = %v, want my-group/my-project", vars["projectPath"])
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"project": {
					"vulnerabilities": {
						"nodes": [],
						"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": "", "startCursor": ""}
					}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := List(context.Background(), client, ListInput{
		ProjectPath: "my-group/my-project",
		Severity:    []string{"CRITICAL", "HIGH"},
		State:       []string{"DETECTED"},
		ReportType:  []string{"SAST"},
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(out.Vulnerabilities) != 0 {
		t.Errorf("expected 0 vulnerabilities, got %d", len(out.Vulnerabilities))
	}
}

// Get tests.

// TestGet_Success verifies that retrieving a single vulnerability by ID
// returns the expected detail including identifiers, scanner, and location.
func TestGet_Success(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"vulnerability(id": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"vulnerability": `+sampleVulnGetNode+`
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Get(context.Background(), client, GetInput{
		ID: "gid://gitlab/Vulnerability/42",
	})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	v := out.Vulnerability
	if v.ID != "gid://gitlab/Vulnerability/42" {
		t.Errorf("ID = %q", v.ID)
	}
	if v.Description != "User input is concatenated into SQL query without sanitization." {
		t.Errorf("Description mismatch")
	}
	if v.Solution != "Use parameterized queries." {
		t.Errorf("Solution mismatch")
	}
	if !v.HasSolutions {
		t.Error("expected HasSolutions=true")
	}
	if !v.HasIssues {
		t.Error("expected HasIssues=true (issueLinks present)")
	}
	if !v.HasMR {
		t.Error("expected HasMR=true (mergeRequest present)")
	}
	if v.Project == nil || v.Project.FullPath != "my-group/my-project" {
		t.Errorf("Project.FullPath = %v", v.Project)
	}
	if len(v.Identifiers) != 2 {
		t.Errorf("expected 2 identifiers, got %d", len(v.Identifiers))
	}
}

// TestGet_EmptyID verifies that retrieving a vulnerability returns
// a validation error when the required id parameter is missing.
func TestGet_EmptyID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Get(context.Background(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error for empty id, got nil")
	}
}

// TestGet_NotFound verifies that retrieving a non-existent vulnerability
// returns an error.
func TestGet_NotFound(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"vulnerability(id": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"vulnerability": {"id": ""}}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	_, err := Get(context.Background(), client, GetInput{
		ID: "gid://gitlab/Vulnerability/999",
	})
	if err == nil {
		t.Fatal("expected error for not-found vulnerability")
	}
}

// Dismiss tests.

// TestDismiss_Success verifies that dismissing a vulnerability via the
// GraphQL mutation returns the updated vulnerability with dismissed state.
func TestDismiss_Success(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"vulnerabilityDismiss": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"vulnerabilityDismiss": {
					"vulnerability": `+sampleMutationVuln+`,
					"errors": []
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Dismiss(context.Background(), client, DismissInput{
		ID:              "gid://gitlab/Vulnerability/42",
		Comment:         "False positive confirmed by security team",
		DismissalReason: "FALSE_POSITIVE",
	})
	if err != nil {
		t.Fatalf("Dismiss() error = %v", err)
	}

	if out.Vulnerability.State != "DISMISSED" {
		t.Errorf("State = %q, want DISMISSED", out.Vulnerability.State)
	}
	if out.Vulnerability.DismissalReason != "FALSE_POSITIVE" {
		t.Errorf("DismissalReason = %q, want FALSE_POSITIVE", out.Vulnerability.DismissalReason)
	}
	if out.Vulnerability.Title != "SQL Injection in login" {
		t.Errorf("Title = %q, want %q", out.Vulnerability.Title, "SQL Injection in login")
	}
	if out.Vulnerability.Severity != "CRITICAL" {
		t.Errorf("Severity = %q, want %q", out.Vulnerability.Severity, "CRITICAL")
	}
	if out.Vulnerability.DismissedAt != "2024-02-01T12:00:00Z" {
		t.Errorf("DismissedAt = %q, want %q", out.Vulnerability.DismissedAt, "2024-02-01T12:00:00Z")
	}
}

// TestDismiss_EmptyID verifies that dismissing a vulnerability returns
// a validation error when the required id parameter is missing.
func TestDismiss_EmptyID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Dismiss(context.Background(), client, DismissInput{})
	if err == nil {
		t.Fatal("expected error for empty id, got nil")
	}
}

// TestDismiss_ServerError verifies that dismissing a vulnerability
// propagates errors when the GraphQL API returns a server error.
func TestDismiss_ServerError(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"vulnerabilityDismiss": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"vulnerabilityDismiss": {
					"vulnerability": null,
					"errors": ["Vulnerability cannot be dismissed"]
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	_, err := Dismiss(context.Background(), client, DismissInput{
		ID: "gid://gitlab/Vulnerability/42",
	})
	if err == nil {
		t.Fatal("expected server error, got nil")
	}
}

// Confirm tests.

// TestConfirm_Success verifies that confirming a vulnerability via the
// GraphQL mutation returns the updated vulnerability with confirmed state.
func TestConfirm_Success(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"vulnerabilityConfirm": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"vulnerabilityConfirm": {
					"vulnerability": {
						"id": "gid://gitlab/Vulnerability/42",
						"title": "SQL Injection",
						"severity": "CRITICAL",
						"state": "CONFIRMED",
						"reportType": "SAST",
						"detectedAt": "2024-01-15T10:00:00Z",
						"dismissedAt": null,
						"resolvedAt": null,
						"confirmedAt": "2024-02-01T14:00:00Z",
						"dismissalReason": null,
						"primaryIdentifier": null,
						"scanner": {"name": "semgrep", "vendor": "GitLab"}
					},
					"errors": []
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Confirm(context.Background(), client, ConfirmInput{
		ID: "gid://gitlab/Vulnerability/42",
	})
	if err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}
	if out.Vulnerability.State != "CONFIRMED" {
		t.Errorf("State = %q, want CONFIRMED", out.Vulnerability.State)
	}
	if out.Vulnerability.Title != "SQL Injection" {
		t.Errorf("Title = %q, want %q", out.Vulnerability.Title, "SQL Injection")
	}
	if out.Vulnerability.Severity != "CRITICAL" {
		t.Errorf("Severity = %q, want %q", out.Vulnerability.Severity, "CRITICAL")
	}
	if out.Vulnerability.ConfirmedAt != "2024-02-01T14:00:00Z" {
		t.Errorf("ConfirmedAt = %q, want %q", out.Vulnerability.ConfirmedAt, "2024-02-01T14:00:00Z")
	}
}

// TestConfirm_EmptyID verifies that confirming a vulnerability returns
// a validation error when the required id parameter is missing.
func TestConfirm_EmptyID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Confirm(context.Background(), client, ConfirmInput{})
	if err == nil {
		t.Fatal("expected error for empty id, got nil")
	}
}

// Resolve tests.

// TestResolve_Success verifies that resolving a vulnerability via the
// GraphQL mutation returns the updated vulnerability with resolved state.
func TestResolve_Success(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"vulnerabilityResolve": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"vulnerabilityResolve": {
					"vulnerability": {
						"id": "gid://gitlab/Vulnerability/42",
						"title": "SQL Injection",
						"severity": "CRITICAL",
						"state": "RESOLVED",
						"reportType": "SAST",
						"detectedAt": "2024-01-15T10:00:00Z",
						"dismissedAt": null,
						"resolvedAt": "2024-02-02T10:00:00Z",
						"confirmedAt": null,
						"dismissalReason": null,
						"primaryIdentifier": null,
						"scanner": null
					},
					"errors": []
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Resolve(context.Background(), client, ResolveInput{
		ID: "gid://gitlab/Vulnerability/42",
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if out.Vulnerability.State != "RESOLVED" {
		t.Errorf("State = %q, want RESOLVED", out.Vulnerability.State)
	}
	if out.Vulnerability.Title != "SQL Injection" {
		t.Errorf("Title = %q, want %q", out.Vulnerability.Title, "SQL Injection")
	}
	if out.Vulnerability.Severity != "CRITICAL" {
		t.Errorf("Severity = %q, want %q", out.Vulnerability.Severity, "CRITICAL")
	}
	if out.Vulnerability.ResolvedAt != "2024-02-02T10:00:00Z" {
		t.Errorf("ResolvedAt = %q, want %q", out.Vulnerability.ResolvedAt, "2024-02-02T10:00:00Z")
	}
}

// TestResolve_EmptyID verifies that resolving a vulnerability returns
// a validation error when the required id parameter is missing.
func TestResolve_EmptyID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Resolve(context.Background(), client, ResolveInput{})
	if err == nil {
		t.Fatal("expected error for empty id, got nil")
	}
}

// Revert tests.

// TestRevert_Success verifies that reverting a vulnerability via the
// GraphQL mutation returns the updated vulnerability with detected state.
func TestRevert_Success(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"vulnerabilityRevertToDetected": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"vulnerabilityRevertToDetected": {
					"vulnerability": {
						"id": "gid://gitlab/Vulnerability/42",
						"title": "SQL Injection",
						"severity": "CRITICAL",
						"state": "DETECTED",
						"reportType": "SAST",
						"detectedAt": "2024-01-15T10:00:00Z",
						"dismissedAt": null,
						"resolvedAt": null,
						"confirmedAt": null,
						"dismissalReason": null,
						"primaryIdentifier": null,
						"scanner": null
					},
					"errors": []
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Revert(context.Background(), client, RevertInput{
		ID: "gid://gitlab/Vulnerability/42",
	})
	if err != nil {
		t.Fatalf("Revert() error = %v", err)
	}
	if out.Vulnerability.State != "DETECTED" {
		t.Errorf("State = %q, want DETECTED", out.Vulnerability.State)
	}
	if out.Vulnerability.Title != "SQL Injection" {
		t.Errorf("Title = %q, want %q", out.Vulnerability.Title, "SQL Injection")
	}
	if out.Vulnerability.Severity != "CRITICAL" {
		t.Errorf("Severity = %q, want %q", out.Vulnerability.Severity, "CRITICAL")
	}
}

// TestRevert_EmptyID verifies that reverting a vulnerability returns
// a validation error when the required id parameter is missing.
func TestRevert_EmptyID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Revert(context.Background(), client, RevertInput{})
	if err == nil {
		t.Fatal("expected error for empty id, got nil")
	}
}

// Markdown tests.

// TestFormatListMarkdown_Empty verifies that formatting an empty
// vulnerability list produces the expected no-results Markdown message.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if md == "" {
		t.Fatal("expected non-empty markdown")
	}
	if !contains(md, "No vulnerabilities found") {
		t.Error("expected 'No vulnerabilities found' in markdown")
	}
}

// TestFormatListMarkdown_WithItems verifies that formatting vulnerabilities
// produces a Markdown table with severity, state, scanner, and identifiers.
func TestFormatListMarkdown_WithItems(t *testing.T) {
	out := ListOutput{
		Vulnerabilities: []Item{
			{
				ID:         "gid://gitlab/Vulnerability/1",
				Title:      "SQL Injection",
				Severity:   "CRITICAL",
				State:      "DETECTED",
				ReportType: "SAST",
				DetectedAt: "2024-01-15T10:00:00Z",
				Scanner:    &ScannerItem{Name: "semgrep"},
				PrimaryID:  &IdentifierItem{Name: "CWE-89"},
			},
		},
	}
	md := FormatListMarkdown(out)
	if !contains(md, "CRITICAL") {
		t.Error("expected CRITICAL in markdown")
	}
	if !contains(md, "SQL Injection") {
		t.Error("expected title in markdown")
	}
	if !contains(md, "semgrep") {
		t.Error("expected scanner name in markdown")
	}
}

// TestFormatGetMarkdown verifies that formatting a vulnerability detail
// produces a Markdown block with all fields including location and solution.
func TestFormatGetMarkdown(t *testing.T) {
	out := GetOutput{
		Vulnerability: Item{
			ID:          "gid://gitlab/Vulnerability/42",
			Title:       "SQL Injection",
			Severity:    "HIGH",
			State:       "CONFIRMED",
			Description: "A serious vulnerability",
			ReportType:  "SAST",
			Scanner:     &ScannerItem{Name: "semgrep", Vendor: "GitLab"},
			Location:    &LocationItem{File: "main.go", StartLine: 10, EndLine: 20},
			PrimaryID:   &IdentifierItem{Name: "CWE-89", URL: "https://cwe.mitre.org/89"},
			Identifiers: []IdentifierItem{
				{Name: "CWE-89", ExternalType: "cwe", ExternalID: "89"},
			},
			Project:  &ProjectItem{FullPath: "my-group/my-project"},
			Solution: "Use prepared statements",
		},
	}
	md := FormatGetMarkdown(out)
	if !contains(md, "SQL Injection") {
		t.Error("expected title")
	}
	if !contains(md, "HIGH") {
		t.Error("expected severity")
	}
	if !contains(md, "main.go:10-20") {
		t.Error("expected location with line range")
	}
	if !contains(md, "Identifiers") {
		t.Error("expected identifiers section")
	}
	if !contains(md, "Description") {
		t.Error("expected description section")
	}
}

// TestFormatMutationMarkdown verifies that formatting a vulnerability
// mutation result produces the expected state-change confirmation Markdown.
func TestFormatMutationMarkdown(t *testing.T) {
	out := MutationOutput{
		Vulnerability: Item{
			ID:              "gid://gitlab/Vulnerability/42",
			Title:           "Test Vuln",
			Severity:        "MEDIUM",
			State:           "DISMISSED",
			DismissalReason: "FALSE_POSITIVE",
		},
	}
	md := FormatMutationMarkdown(out, "dismissed")
	if !contains(md, "dismissed") {
		t.Error("expected action in markdown")
	}
	if !contains(md, "DISMISSED") {
		t.Error("expected state in markdown")
	}
	if !contains(md, "FALSE_POSITIVE") {
		t.Error("expected dismissal reason")
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
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := severityBadge(tt.input)
			if got != tt.want {
				t.Errorf("severityBadge(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// contains reports whether the given slice includes the specified integer.
func contains(s, sub string) bool {
	return len(s) > 0 && len(sub) > 0 && (s == sub || len(s) >= len(sub) && containsStr(s, sub))
}

// containsStr reports whether the given slice includes the specified string.
func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
