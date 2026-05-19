// action_specs_test.go contains integration tests for the vulnerability tool
// closures in ActionSpecs routes with a mock GitLab API.
package vulnerabilities

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const registerVulnListGQL = `{
	"project": {
		"vulnerabilities": {
			"nodes": [{
				"id": "gid://gitlab/Vulnerability/42",
				"title": "SQL Injection",
				"severity": "CRITICAL",
				"state": "DETECTED",
				"reportType": "SAST",
				"detectedAt": "2026-01-15T10:00:00Z",
				"primaryIdentifier": {"name": "CWE-89", "externalType": "cwe", "externalId": "89", "url": ""},
				"scanner": {"name": "semgrep", "vendor": "GitLab"},
				"project": {"id": "gid://gitlab/Project/1", "name": "proj", "fullPath": "g/p"}
			}],
			"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": "", "startCursor": ""}
		}
	}
}`

const registerVulnGetGQL = `{
	"vulnerability": {
		"id": "gid://gitlab/Vulnerability/42",
		"title": "SQL Injection",
		"description": "Vulnerable to SQL injection",
		"severity": "CRITICAL",
		"state": "DETECTED",
		"reportType": "SAST",
		"detectedAt": "2026-01-15T10:00:00Z",
		"primaryIdentifier": {"name": "CWE-89", "externalType": "cwe", "externalId": "89", "url": ""},
		"identifiers": [{"name": "CWE-89", "externalType": "cwe", "externalId": "89", "url": ""}],
		"scanner": {"name": "semgrep", "vendor": "GitLab"},
		"project": {"id": "gid://gitlab/Project/1", "name": "proj", "fullPath": "g/p"},
		"location": {},
		"issueLinks": {"nodes": []},
		"mergeRequest": null
	}
}`

const registerMutationGQL = `{
	"vulnerabilityDismiss": {
		"vulnerability": {
			"id": "gid://gitlab/Vulnerability/42",
			"title": "SQL Injection",
			"severity": "CRITICAL",
			"state": "DISMISSED",
			"reportType": "SAST",
			"detectedAt": "2026-01-15T10:00:00Z",
			"primaryIdentifier": {"name": "CWE-89", "externalType": "cwe", "externalId": "89", "url": ""},
			"scanner": {"name": "semgrep", "vendor": "GitLab"}
		},
		"errors": []
	}
}`

const registerSeverityCountGQL = `{
	"project": {
		"vulnerabilitySeveritiesCount": {
			"critical": 5, "high": 12, "medium": 23, "low": 8, "info": 3, "unknown": 1
		}
	}
}`

const registerSecuritySummaryGQL = `{
	"project": {
		"pipeline": {
			"securityReportSummary": {
				"sast": {"vulnerabilitiesCount": 10, "scannedResourcesCount": 150, "scannedResourcesCsvPath": ""},
				"dast": {"vulnerabilitiesCount": 3, "scannedResourcesCount": 50, "scannedResourcesCsvPath": ""}
			}
		}
	}
}`

// TestActionSpecs_Metadata verifies vulnerability action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 8 {
		t.Fatalf("len(ActionSpecs) = %d, want 8", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "vulnerabilities" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// TestActionSpecs_CallRoutes verifies all 8 vulnerability routes execute successfully.
func TestActionSpecs_CallRoutes(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"vulnerabilities": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, registerVulnListGQL)
		},
		"vulnerability(id": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, registerVulnGetGQL)
		},
		"vulnerabilityDismiss": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, registerMutationGQL)
		},
		"vulnerabilityConfirm": func(w http.ResponseWriter, _ *http.Request) {
			resp := `{"vulnerabilityConfirm":{"vulnerability":{"id":"gid://gitlab/Vulnerability/42","title":"t","severity":"CRITICAL","state":"CONFIRMED","reportType":"SAST","detectedAt":"2026-01-15T10:00:00Z","confirmedAt":"2026-02-01T12:00:00Z","primaryIdentifier":{"name":"CWE-89","externalType":"cwe","externalId":"89","url":""},"scanner":{"name":"semgrep","vendor":"GitLab"}},"errors":[]}}`
			testutil.RespondGraphQL(w, http.StatusOK, resp)
		},
		"vulnerabilityResolve": func(w http.ResponseWriter, _ *http.Request) {
			resp := `{"vulnerabilityResolve":{"vulnerability":{"id":"gid://gitlab/Vulnerability/42","title":"t","severity":"CRITICAL","state":"RESOLVED","reportType":"SAST","detectedAt":"2026-01-15T10:00:00Z","resolvedAt":"2026-02-01T12:00:00Z","primaryIdentifier":{"name":"CWE-89","externalType":"cwe","externalId":"89","url":""},"scanner":{"name":"semgrep","vendor":"GitLab"}},"errors":[]}}`
			testutil.RespondGraphQL(w, http.StatusOK, resp)
		},
		"vulnerabilityRevertToDetected": func(w http.ResponseWriter, _ *http.Request) {
			resp := `{"vulnerabilityRevertToDetected":{"vulnerability":{"id":"gid://gitlab/Vulnerability/42","title":"t","severity":"CRITICAL","state":"DETECTED","reportType":"SAST","detectedAt":"2026-01-15T10:00:00Z","primaryIdentifier":{"name":"CWE-89","externalType":"cwe","externalId":"89","url":""},"scanner":{"name":"semgrep","vendor":"GitLab"}},"errors":[]}}`
			testutil.RespondGraphQL(w, http.StatusOK, resp)
		},
		"vulnerabilitySeveritiesCount": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, registerSeverityCountGQL)
		},
		"securityReportSummary": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, registerSecuritySummaryGQL)
		},
	})
	client := testutil.NewTestClient(t, handler)
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_list_vulnerabilities", map[string]any{"project_path": "g/p"}},
		{"gitlab_get_vulnerability", map[string]any{"id": "gid://gitlab/Vulnerability/42"}},
		{"gitlab_dismiss_vulnerability", map[string]any{"id": "gid://gitlab/Vulnerability/42"}},
		{"gitlab_confirm_vulnerability", map[string]any{"id": "gid://gitlab/Vulnerability/42"}},
		{"gitlab_resolve_vulnerability", map[string]any{"id": "gid://gitlab/Vulnerability/42"}},
		{"gitlab_revert_vulnerability", map[string]any{"id": "gid://gitlab/Vulnerability/42"}},
		{"gitlab_vulnerability_severity_count", map[string]any{"project_path": "g/p"}},
		{"gitlab_pipeline_security_summary", map[string]any{"project_path": "g/p", "pipeline_iid": "1"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.name)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.name)
			}
		})
	}
}
