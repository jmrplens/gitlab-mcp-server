// action_specs_test.go contains integration tests for the vulnerability tool
// closures in ActionSpecs routes with a mock GitLab API.
package vulnerabilities

import (
	"net/http"
	"slices"
	"strings"
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

// TestActionSpecs_DiscoveryMetadata verifies every vulnerability action spec
// carries non-generic discovery metadata: an action-specific Usage, aliases
// beyond the tool name, canonical RelatedActions cross-links, and an
// individual-tool description in the "Returns: … See also: …" form.
func TestActionSpecs_DiscoveryMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, spec := range ActionSpecs(client) {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			assertDiscoveryMetadata(t, spec)
		})
	}
}

// assertDiscoveryMetadata fails the test if spec lacks non-generic Usage,
// aliases beyond the tool name, canonical RelatedActions, or a
// "Returns: … See also: …" individual-tool description.
func assertDiscoveryMetadata(t *testing.T, spec toolutil.ActionSpec) {
	t.Helper()
	const genericUsage = "Use to execute vulnerabilities domain action."
	tool := spec.IndividualTool.Name
	if spec.Usage == "" || spec.Usage == genericUsage {
		t.Fatalf("%s: generic or empty Usage: %q", tool, spec.Usage)
	}
	if len(spec.Aliases) == 0 {
		t.Fatalf("%s: no Aliases", tool)
	}
	for _, alias := range spec.Aliases {
		if alias == tool {
			t.Fatalf("%s: alias equals tool name", tool)
		}
	}
	if len(spec.RelatedActions) == 0 {
		t.Fatalf("%s: empty RelatedActions", tool)
	}
	for _, related := range spec.RelatedActions {
		if !strings.Contains(related, ".") {
			t.Fatalf("%s: related action %q is not a canonical {domain}.{action} id", tool, related)
		}
	}
	desc := spec.IndividualTool.Description
	if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
		t.Fatalf("%s: description missing Returns/See also: %q", tool, desc)
	}
}

// TestActionSpecs_DismissalReasonEnum verifies that dismiss constrains
// dismissal_reason to the VulnerabilityDismissalReason enum, and that no other
// vulnerability action carries a dismissal_reason override.
func TestActionSpecs_DismissalReasonEnum(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	want := []any{"ACCEPTABLE_RISK", "FALSE_POSITIVE", "MITIGATING_CONTROL", "NOT_APPLICABLE", "USED_IN_TESTS"}
	for _, spec := range ActionSpecs(client) {
		wantOverride := spec.IndividualTool.Name == "gitlab_dismiss_vulnerability"
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			has := false
			for _, override := range spec.InputSchemaOverrides {
				if override.PropertyPath != "dismissal_reason" {
					continue
				}
				has = true
				if enum, ok := override.Values["enum"].([]any); !ok || !slices.Equal(enum, want) {
					t.Errorf("dismissal_reason enum = %v, want %v", override.Values["enum"], want)
				}
			}
			if has != wantOverride {
				t.Errorf("dismissal_reason override present = %v, want %v", has, wantOverride)
			}
		})
	}
}

// TestDecorateVulnerabilityMeta_UnknownToolIsNoOp verifies that decorating an
// individual tool with no metadata entry leaves the generic options untouched.
func TestDecorateVulnerabilityMeta_UnknownToolIsNoOp(t *testing.T) {
	options := vulnerabilityOptions("gitlab_unknown_vulnerability_tool")
	before := options
	decorateVulnerabilityMeta(&options, "gitlab_unknown_vulnerability_tool")
	if options.Usage != before.Usage || options.IndividualTool.Description != before.IndividualTool.Description {
		t.Fatalf("decorateVulnerabilityMeta mutated options for unknown tool: %+v", options)
	}
	if len(options.RelatedActions) != 0 {
		t.Fatalf("expected no RelatedActions for unknown tool, got %v", options.RelatedActions)
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
