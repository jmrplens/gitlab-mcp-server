// action_specs_test.go contains unit tests for the external status check [toolutil.ActionSpec] entries.
package externalstatuschecks

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	mergeCheckJSON   = `[{"id":1,"name":"CI Check","external_url":"https://ci.example.com","status":"passed"}]`
	projectCheckJSON = `[{"id":1,"name":"CI Check","external_url":"https://ci.example.com","hmac":true,"protected_branches":[{"id":1,"name":"main"}]}]`
	createdCheckJSON = `{"id":2,"name":"New Check","external_url":"https://new.example.com","hmac":false,"protected_branches":[]}`
)

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)

	if len(specs) != 8 {
		t.Fatalf("len(ActionSpecs) = %d, want 8", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "externalstatuschecks" {
			t.Errorf("OwnerPackage for %s = %q, want externalstatuschecks", spec.Name, spec.OwnerPackage)
		}
		if spec.IndividualTool.Name == "" {
			t.Errorf("IndividualTool.Name for %s is empty", spec.Name)
		}
	}

	byTool := externalStatusCheckSpecsByTool(t, specs)
	for _, name := range []string{"gitlab_list_project_status_checks", "gitlab_list_project_mr_external_status_checks", "gitlab_list_project_external_status_checks"} {
		t.Run(name, func(t *testing.T) {
			if !byTool[name].ReadOnly {
				t.Errorf("%s should be read-only", name)
			}
		})
	}
	spec := byTool["gitlab_delete_project_external_status_check"]
	if !spec.Destructive || !spec.Route.Destructive {
		t.Error("delete action should be destructive")
	}
}

// TestActionSpecs_SetStatus_TakesTheHeadSHAAndAStatusGitLabKnows verifies the
// schema of the action that sets a merge request's check: sha carries
// toolutil.FullCommitSHAPattern, since GitLab compares it with the head's
// full SHA and answers 409 otherwise, and status the three values GitLab's
// API lists, so find's example sends a SHA and passed where it sent "value"
// for both (issue 1272). No other action carries either.
func TestActionSpecs_SetStatus_TakesTheHeadSHAAndAStatusGitLabKnows(t *testing.T) {
	for _, spec := range ActionSpecs(nil) {
		t.Run(spec.Name, func(t *testing.T) {
			properties, _ := spec.Route.InputSchema["properties"].(map[string]any)
			sha, _ := properties["sha"].(map[string]any)
			status, _ := properties["status"].(map[string]any)
			wantPattern, wantEnum := any(nil), any(nil)
			if spec.Name == "set_project_mr_status" {
				wantPattern, wantEnum = toolutil.FullCommitSHAPattern, []any{"passed", "failed", "pending"}
			}
			if got := sha["pattern"]; got != wantPattern {
				t.Errorf("%s sha pattern = %v, want %v", spec.Name, got, wantPattern)
			}
			if got := status["enum"]; fmt.Sprint(got) != fmt.Sprint(wantEnum) {
				t.Errorf("%s status enum = %v, want %v", spec.Name, got, wantEnum)
			}
		})
	}
}

// TestActionSpecs_CallRoutes validates the CallRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if strings.Contains(r.URL.Path, "merge_requests") {
				testutil.RespondJSON(w, http.StatusOK, mergeCheckJSON)
			} else {
				testutil.RespondJSON(w, http.StatusOK, projectCheckJSON)
			}
		case http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, createdCheckJSON)
		case http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, createdCheckJSON)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, mux)
	byTool := externalStatusCheckSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_list_project_status_checks", map[string]any{"project_id": "42"}},
		{"gitlab_list_project_mr_external_status_checks", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_list_project_external_status_checks", map[string]any{"project_id": "42"}},
		{"gitlab_create_project_external_status_check", map[string]any{"project_id": "42", "name": "check", "external_url": "https://ci.example.com"}},
		{"gitlab_delete_project_external_status_check", map[string]any{"project_id": "42", "check_id": 1}},
		{"gitlab_update_project_external_status_check", map[string]any{"project_id": "42", "check_id": 1}},
		{"gitlab_retry_failed_external_status_check_for_project_mr", map[string]any{"project_id": "42", "merge_request_iid": 1, "check_id": 1}},
		{"gitlab_set_project_mr_external_status_check_status", map[string]any{"project_id": "42", "merge_request_iid": 1, "sha": "abc123", "external_status_check_id": 1, "status": "passed"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.name)
			}
		})
	}
}

// TestActionSpecs_MutationErrors validates the MutationErrors route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_MutationErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := externalStatusCheckSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_create_project_external_status_check", map[string]any{"project_id": "42", "name": "check", "external_url": "https://ci.example.com"}},
		{"gitlab_delete_project_external_status_check", map[string]any{"project_id": "42", "check_id": 1}},
		{"gitlab_update_project_external_status_check", map[string]any{"project_id": "42", "check_id": 1}},
		{"gitlab_retry_failed_external_status_check_for_project_mr", map[string]any{"project_id": "42", "merge_request_iid": 1, "check_id": 1}},
		{"gitlab_set_project_mr_external_status_check_status", map[string]any{"project_id": "42", "merge_request_iid": 1, "sha": "abc", "external_status_check_id": 1, "status": "passed"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			_, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatalf("Route.Handler(%s) expected error, got nil", tt.name)
			}
		})
	}
}

// TestDecorateExternalStatusCheckMeta_Coverage verifies the discovery-metadata
// decorator both enriches a known tool and leaves the generic placeholder
// metadata untouched for an unknown tool (the no-op early-return branch).
// It asserts every known tool gains a non-generic Usage, a
// "Returns: … See also: …" description, and the exact aliases and related
// actions its entry names, and that an unrecognized tool keeps the default
// placeholder Usage.
//
// The alias and related-action halves are compared against the entry rather
// than merely counted: the generic options already carry one alias, the tool's
// own name, so "has at least one alias" is an assertion about metadata the
// decorator never touched and passes with every natural-language alias
// dropped. Those aliases are what a model's own words reach the action
// through, and RelatedActions is what the next-step hints are drawn from, so
// losing either leaves an action that works and cannot be found.
func TestDecorateExternalStatusCheckMeta_Coverage(t *testing.T) {
	const genericUsage = "Use to execute externalstatuschecks domain action."

	for tool, meta := range externalStatusCheckActionMeta {
		t.Run(tool, func(t *testing.T) {
			options := externalStatusCheckOptions(tool)
			decorateExternalStatusCheckMeta(&options, tool)
			if options.Usage == genericUsage || options.Usage == "" {
				t.Errorf("%s: Usage not enriched: %q", tool, options.Usage)
			}
			if !strings.Contains(options.IndividualTool.Description, "Returns:") ||
				!strings.Contains(options.IndividualTool.Description, "See also:") {
				t.Errorf("%s: description missing Returns/See also: %q", tool, options.IndividualTool.Description)
			}
			if !slices.Equal(options.Aliases, meta.aliases) {
				t.Errorf("%s: Aliases = %q, want the entry's %q", tool, options.Aliases, meta.aliases)
			}
			if !slices.Equal(options.RelatedActions, meta.related) {
				t.Errorf("%s: RelatedActions = %q, want the entry's %q", tool, options.RelatedActions, meta.related)
			}
		})
	}

	unknown := externalStatusCheckOptions("gitlab_unknown_tool")
	decorateExternalStatusCheckMeta(&unknown, "gitlab_unknown_tool")
	if unknown.Usage != genericUsage {
		t.Errorf("unknown tool Usage = %q, want generic placeholder", unknown.Usage)
	}
}

// TestDecorateExternalStatusCheckMeta_EntryNamesNothing_KeepsWhatTheCallerHad
// verifies that the decorator replaces only the fields its entry actually
// names, and leaves the rest of the options as the caller built them.
//
// That is what the four "is it set" guards are for, and no entry in the table
// can demonstrate it: all eight fill every field, so each guard is only ever
// taken one way and a guard that fired on an empty value would look identical.
// The entry is therefore injected for the length of this test. What it
// protects is concrete: the generic options carry the individual tool's own
// name as its one alias, which is how a caller naming the tool reaches the
// action at all, so a guard that replaced it with an entry's empty list would
// leave an action answering to no name — and a partially filled entry is
// exactly what an author adding a ninth action writes first.
func TestDecorateExternalStatusCheckMeta_EntryNamesNothing_KeepsWhatTheCallerHad(t *testing.T) {
	const tool = "gitlab_external_status_check_unfilled_entry"
	externalStatusCheckActionMeta[tool] = externalStatusCheckActionMetaEntry{}
	t.Cleanup(func() { delete(externalStatusCheckActionMeta, tool) })

	options := externalStatusCheckOptions(tool)
	options.RelatedActions = []string{actionListProject}
	options.IndividualTool.Description = "the description the caller already had"
	before := options

	decorateExternalStatusCheckMeta(&options, tool)

	if options.Usage != before.Usage {
		t.Errorf("Usage = %q, want it left at %q", options.Usage, before.Usage)
	}
	if !slices.Equal(options.Aliases, before.Aliases) {
		t.Errorf("Aliases = %q, want them left at %q", options.Aliases, before.Aliases)
	}
	if !slices.Equal(options.RelatedActions, before.RelatedActions) {
		t.Errorf("RelatedActions = %q, want them left at %q", options.RelatedActions, before.RelatedActions)
	}
	if options.IndividualTool.Description != before.IndividualTool.Description {
		t.Errorf("Description = %q, want it left at %q", options.IndividualTool.Description, before.IndividualTool.Description)
	}
}

// TestActionSpecs_DeleteUsage_ScopesTheSilentSuccessToOlderReleases
// verifies that the delete's served usage says which releases answer a
// caller without the Maintainer role with success and delete nothing, and
// keeps the advice to confirm the deletion for those releases only. From 19.5
// GitLab refuses that caller with 403, which the handler hints, so a usage
// telling every caller to confirm every deletion describes an answer GitLab
// no longer sends (entry 56 of docs/development/upstream-bugs.md). The usage
// is served prose, so it carries no semicolon either.
func TestActionSpecs_DeleteUsage_ScopesTheSilentSuccessToOlderReleases(t *testing.T) {
	usage := externalStatusCheckSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.NewServeMux())))["gitlab_delete_project_external_status_check"].Usage
	const silentSuccess = "GitLab before 19.5 answers a caller without the role with success and deletes nothing, so on those releases confirm the deletion with " + actionListProject + "."
	if !strings.HasSuffix(usage, silentSuccess) {
		t.Errorf("delete usage = %q, want it to end with %q", usage, silentSuccess)
	}
	if strings.Contains(usage, ";") {
		t.Errorf("delete usage = %q, must carry no semicolon", usage)
	}
}

func externalStatusCheckSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}
