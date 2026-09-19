// resource_groups_test.go contains unit tests for the resource group MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package resourcegroups

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// errExpectedErr identifies the err expected err constant used by this package.
const errExpectedErr = "expected error"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// The one resource group body the handler tests decode, and the item it must
// produce. Every field carries a different value and every field is asserted,
// which is what makes a converter that drops one or reads a neighbour's key
// observable: each handler used to be judged on a single field, so the other
// two could go missing with nothing failing.
const fixtureGroupJSON = `{"id":7,"key":"production","process_mode":"oldest_first"}`

// wantFixtureGroup is fixtureGroupJSON as the handlers must convert it.
func wantFixtureGroup() ResourceGroupItem {
	return ResourceGroupItem{ID: 7, Key: "production", ProcessMode: "oldest_first"}
}

// TestListAll verifies ListAll converts every field GitLab sent, not only the
// key: the whole item is compared, because asserting one field left the
// converter free to drop the id and the process mode with nothing failing.
func TestListAll(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/resource_groups" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[`+fixtureGroupJSON+`]`)
	}))
	out, err := ListAll(t.Context(), client, ListInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	want := []ResourceGroupItem{wantFixtureGroup()}
	if !reflect.DeepEqual(out.Groups, want) {
		t.Errorf("ListAll groups = %+v, want %+v", out.Groups, want)
	}
}

// TestListAll_Error verifies ListAll when error.
func TestListAll_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	_, err := ListAll(t.Context(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestGet verifies Get converts the whole resource group. Only the process
// mode used to be asserted, so the id and the key could go missing from the
// card a model reads and the suite stayed green.
func TestGet(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/resource_groups/production" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, fixtureGroupJSON)
	}))
	out, err := Get(t.Context(), client, GetInput{ProjectID: "1", Key: "production"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if want := wantFixtureGroup(); !reflect.DeepEqual(out, want) {
		t.Errorf("Get() = %+v, want %+v", out, want)
	}
}

// TestGet_Error verifies Get when error.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	_, err := Get(t.Context(), client, GetInput{ProjectID: "1", Key: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestEdit verifies that the process mode the caller asked for is what reaches
// GitLab, and that the answer is read back rather than echoed.
//
// Both halves were unobservable. The request body was never inspected, so
// building the options without ProcessMode at all left the suite green while
// the one input this tool exists to carry never left the process; and the mock
// answered the mode that had just been asked for, so a handler returning its
// own input would have passed too. The mock therefore answers a different mode
// than was requested, which no two assertions can now confuse.
func TestEdit(t *testing.T) {
	var sentProcessMode string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/resource_groups/production" || r.Method != http.MethodPut {
			http.NotFound(w, r)
			return
		}
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			t.Errorf("reading the edit request body: %v", readErr)
			testutil.RespondJSON(w, http.StatusOK, fixtureGroupJSON)
			return
		}
		var sent struct {
			ProcessMode string `json:"process_mode"`
		}
		if decodeErr := json.Unmarshal(body, &sent); decodeErr != nil {
			t.Errorf("edit request body %q is not JSON: %v", body, decodeErr)
			testutil.RespondJSON(w, http.StatusOK, fixtureGroupJSON)
			return
		}
		sentProcessMode = sent.ProcessMode
		testutil.RespondJSON(w, http.StatusOK, `{"id":7,"key":"production","process_mode":"newest_ready_first"}`)
	}))
	out, err := Edit(t.Context(), client, EditInput{ProjectID: "1", Key: "production", ProcessMode: "newest_first"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if sentProcessMode != "newest_first" {
		t.Errorf("GitLab was sent process_mode %q, want %q", sentProcessMode, "newest_first")
	}
	want := ResourceGroupItem{ID: 7, Key: "production", ProcessMode: "newest_ready_first"}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("Edit() = %+v, want %+v", out, want)
	}
}

// TestEdit_Error verifies Edit when error.
func TestEdit_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := Edit(t.Context(), client, EditInput{ProjectID: "1", Key: "x", ProcessMode: "invalid"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestListUpcomingJobs verifies every field of a queued job survives the
// conversion. The old fixture named the job and its stage both "deploy" and
// only the name was asserted, so swapping the status and the stage, or dropping
// the id, produced exactly the same passing run.
func TestListUpcomingJobs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/resource_groups/production/upcoming_jobs" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"id":10,"name":"deploy-to-prod","status":"pending","stage":"release"}]`)
	}))
	out, err := ListUpcomingJobs(t.Context(), client, ListUpcomingJobsInput{ProjectID: "1", Key: "production"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	want := []JobItem{{ID: 10, Name: "deploy-to-prod", Status: "pending", Stage: "release"}}
	if !reflect.DeepEqual(out.Jobs, want) {
		t.Errorf("ListUpcomingJobs jobs = %+v, want %+v", out.Jobs, want)
	}
}

// TestListUpcomingJobs_Error verifies ListUpcomingJobs when error.
func TestListUpcomingJobs_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	_, err := ListUpcomingJobs(t.Context(), client, ListUpcomingJobsInput{ProjectID: "1", Key: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestFormatListMarkdown checks the whole list rendering. The two next steps
// are named separately: reading one group and changing its process mode are
// different actions, and one hint offering both named a tool that only reads.
func TestFormatListMarkdown(t *testing.T) {
	want := "## Resource Groups (1)\n\n" +
		"| ID | Key | Process Mode |\n" +
		"| --- | --- | --- |\n" +
		"| 1 | prod | unordered |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'pipeline.resource_group_get' to see one resource group in full\n" +
		"- Use action 'pipeline.resource_group_edit' to change a group's process mode\n"
	md := FormatListMarkdown(ListOutput{Groups: []ResourceGroupItem{{ID: 1, Key: "prod", ProcessMode: "unordered"}}})
	if md != want {
		t.Errorf("FormatListMarkdown()\n got %q\nwant %q", md, want)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// FormatListMarkdown — empty groups
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	const want = "No resource groups found.\n"
	if md := FormatListMarkdown(ListOutput{Groups: nil}); md != want {
		t.Errorf("FormatListMarkdown(empty)\n got %q\nwant %q", md, want)
	}
}

// ---------------------------------------------------------------------------
// FormatGroupMarkdown
// ---------------------------------------------------------------------------.

// TestFormatGroupMarkdown verifies FormatGroupMarkdown.
func TestFormatGroupMarkdown(t *testing.T) {
	md := FormatGroupMarkdown(ResourceGroupItem{ID: 42, Key: "staging", ProcessMode: "oldest_first"})
	want := "## Resource Group: staging\n\n" +
		"- **ID**: 42\n" +
		"- **Key**: staging\n" +
		"- **Process Mode**: oldest_first\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'pipeline.resource_group_upcoming_jobs' to see the jobs waiting on this group\n" +
		"- Use action 'pipeline.resource_group_edit' to change its process mode\n"
	if md != want {
		t.Errorf("FormatGroupMarkdown()\n got %q\nwant %q", md, want)
	}
}

// ---------------------------------------------------------------------------
// FormatJobsMarkdown — with data and empty
// ---------------------------------------------------------------------------.

// TestFormatJobsMarkdown_WithData verifies FormatJobsMarkdown when with data.
func TestFormatJobsMarkdown_WithData(t *testing.T) {
	md := FormatJobsMarkdown(ListUpcomingJobsOutput{
		Jobs: []JobItem{
			{ID: 10, Name: "deploy", Status: "pending", Stage: "deploy"},
			{ID: 11, Name: "build", Status: "created", Stage: "build"},
		},
	})
	// The status carries the glyph every job row in the tree shows, which this
	// table was the one place not to.
	want := "## Upcoming Jobs (2)\n\n" +
		"| ID | Name | Status | Stage |\n" +
		"| --- | --- | --- | --- |\n" +
		"| 10 | deploy | 🟡 pending | deploy |\n" +
		"| 11 | build | 🆕 created | build |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'job.get' to see one of these jobs in full\n" +
		"- Use action 'job.trace' to read a job's log\n" +
		"- Use action 'pipeline.resource_group_list' to see the other resource groups of this project\n"
	if md != want {
		t.Errorf("FormatJobsMarkdown()\n got %q\nwant %q", md, want)
	}
}

// TestFormatJobsMarkdown_Empty verifies FormatJobsMarkdown when empty.
func TestFormatJobsMarkdown_Empty(t *testing.T) {
	const want = "No upcoming jobs found.\n"
	if md := FormatJobsMarkdown(ListUpcomingJobsOutput{Jobs: nil}); md != want {
		t.Errorf("FormatJobsMarkdown(empty)\n got %q\nwant %q", md, want)
	}
}

// TestFormatJobsMarkdown_BlankStatus checks the cell a job with no status
// renders to. A queued job can reach this table before GitLab has given it one,
// and a blank cell must stay blank: pasting the glyph onto an empty string
// would put a lone status emoji in the row, which reads as a status the job
// does not have. Whitespace counts as no status for the same reason.
func TestFormatJobsMarkdown_BlankStatus(t *testing.T) {
	md := FormatJobsMarkdown(ListUpcomingJobsOutput{
		Jobs: []JobItem{{ID: 12, Name: "provision", Status: "  ", Stage: "setup"}},
	})
	want := "## Upcoming Jobs (1)\n\n" +
		"| ID | Name | Status | Stage |\n" +
		"| --- | --- | --- | --- |\n" +
		"| 12 | provision |  | setup |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'job.get' to see one of these jobs in full\n" +
		"- Use action 'job.trace' to read a job's log\n" +
		"- Use action 'pipeline.resource_group_list' to see the other resource groups of this project\n"
	if md != want {
		t.Errorf("FormatJobsMarkdown(blank status)\n got %q\nwant %q", md, want)
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs metadata
// ---------------------------------------------------------------------------.

// assertResourceGroupMetadata verifies one resource group ActionSpec carries
// non-generic discovery metadata: action-specific Usage, distinctive
// domain-specific Aliases (unique across the domain and using resource-group
// or CI-concurrency phrasing), canonical RelatedActions, and a
// "Returns: … See also: …" individual-tool description.
func assertResourceGroupMetadata(t *testing.T, spec toolutil.ActionSpec, seenAlias map[string]string) {
	t.Helper()
	if spec.OwnerPackage != "resourcegroups" || spec.IndividualTool.Name == "" {
		t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
	}
	if spec.Usage == "" {
		t.Fatalf("Usage for %s should not be empty", spec.Name)
	}
	// Aliases must carry domain-specific natural-language phrasing, not only
	// the individual tool name.
	if len(spec.Aliases) < 2 {
		t.Fatalf("Aliases for %s should include natural-language phrases, got %v", spec.Name, spec.Aliases)
	}
	hasResourceGroupPhrase := false
	for _, a := range spec.Aliases {
		if strings.Contains(a, "resource group") || strings.Contains(a, "concurrency") || strings.Contains(a, "serialization") {
			hasResourceGroupPhrase = true
		}
		// Aliases must be unique across the domain.
		if prev, ok := seenAlias[a]; ok {
			t.Fatalf("duplicate alias %q on %s and %s", a, prev, spec.Name)
		}
		seenAlias[a] = spec.Name
	}
	if !hasResourceGroupPhrase {
		t.Fatalf("Aliases for %s lack distinctive resource-group phrasing: %v", spec.Name, spec.Aliases)
	}
	if len(spec.RelatedActions) == 0 {
		t.Fatalf("RelatedActions for %s should not be empty", spec.Name)
	}
	desc := spec.IndividualTool.Description
	if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
		t.Fatalf("IndividualTool.Description for %s must use Returns:/See also: form, got %q", spec.Name, desc)
	}
}

// TestActionSpecs_Metadata verifies resource group action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)
	specByTool := resourceGroupSpecsByTool(specs)
	if len(specs) != 4 {
		t.Fatalf("len(ActionSpecs) = %d, want 4", len(specs))
	}
	seenAlias := map[string]string{}
	for _, spec := range specs {
		assertResourceGroupMetadata(t, spec, seenAlias)
	}
	if specByTool["gitlab_get_resource_group"].ParameterGuidance["key"].SemanticRole == "" {
		t.Fatal("gitlab_get_resource_group should define key parameter guidance")
	}
	if specByTool["gitlab_edit_resource_group"].ParameterGuidance["process_mode"].SemanticRole == "" {
		t.Fatal("gitlab_edit_resource_group should define process_mode parameter guidance")
	}
}

// ---------------------------------------------------------------------------
// Canonical action IDs
// ---------------------------------------------------------------------------.

// registeredActionIDs returns the canonical catalog ID of every action this
// package registers: the group domain, a dot, and the spec name.
func registeredActionIDs(t *testing.T) []string {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ids := make([]string, 0, 4)
	for _, spec := range ActionSpecs(client) {
		ids = append(ids, catalogDomain+"."+spec.Name)
	}
	slices.Sort(ids)
	return ids
}

// TestActionSpecs_CanonicalIDConstantsMatchTheRegisteredSpecs pins the four
// canonical IDs to the names ActionSpecs really registers under, so renaming an
// action cannot leave a related entry or a Markdown hint pointing at the old
// one. The constants used to be written out by hand in two files and had
// drifted apart in exactly that way.
func TestActionSpecs_CanonicalIDConstantsMatchTheRegisteredSpecs(t *testing.T) {
	want := registeredActionIDs(t)
	got := []string{
		actionResourceGroupEdit,
		actionResourceGroupGet,
		actionResourceGroupList,
		actionResourceGroupUpcomingJobs,
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("canonical ID constants = %v, registered actions = %v", got, want)
	}
}

// TestActionSpecs_RelatedActionsNameActionsThatExist holds every RelatedActions
// entry to an action something really serves. Nothing else in the tree does:
// audit_discovery_completeness only reports an empty list, so a wrong spelling
// passes every gate and answers a model "unknown action" the moment it follows
// the hint. All four entries named a resource_group.* domain the catalog has
// never had, because these actions are routes on gitlab_pipeline.
func TestActionSpecs_RelatedActionsNameActionsThatExist(t *testing.T) {
	own := registeredActionIDs(t)
	// Actions of other groups this package deliberately points at. Listing them
	// is what makes adding one a decision rather than a typo.
	foreign := []string{actionJobGet, actionJobList, actionJobTrace}

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	for _, spec := range ActionSpecs(client) {
		t.Run(spec.Name, func(t *testing.T) {
			for _, related := range spec.RelatedActions {
				if slices.Contains(own, related) || slices.Contains(foreign, related) {
					continue
				}
				t.Errorf("RelatedActions names %q, which is neither one of this package's actions %v nor a declared foreign action %v",
					related, own, foreign)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution for all 4 individual tools
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRoutes validates resource group canonical routes.
func TestActionSpecs_CallRoutes(t *testing.T) {
	specByTool := covNewResourceGroupsRouteSpecs(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_resource_groups", map[string]any{"project_id": "1"}},
		{"get", "gitlab_get_resource_group", map[string]any{"project_id": "1", "key": "production"}},
		{"edit", "gitlab_edit_resource_group", map[string]any{"project_id": "1", "key": "production", "process_mode": "newest_first"}},
		{"list_upcoming_jobs", "gitlab_list_resource_group_upcoming_jobs", map[string]any{"project_id": "1", "key": "production"}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.tool)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MCP round-trip for meta-tool (all 4 actions)
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// ActionSpec route execution — API error paths
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRouteErrors validates resource group route API errors.
func TestActionSpecs_CallRouteErrors(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	specByTool := resourceGroupSpecsByTool(ActionSpecs(client))

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list_error", "gitlab_list_resource_groups", map[string]any{"project_id": "1"}},
		{"get_error", "gitlab_get_resource_group", map[string]any{"project_id": "1", "key": "x"}},
		{"edit_error", "gitlab_edit_resource_group", map[string]any{"project_id": "1", "key": "x", "process_mode": "bad"}},
		{"jobs_error", "gitlab_list_resource_group_upcoming_jobs", map[string]any{"project_id": "1", "key": "x"}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.tool)
			}
			if _, err := spec.Route.Handler(t.Context(), tt.args); err == nil {
				t.Fatalf("Route.Handler(%s) expected error", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session factory
// ---------------------------------------------------------------------------.

// covNewResourceGroupsRouteSpecs supports cov new resource groups route specs assertions in resourcegroups tests.
func covNewResourceGroupsRouteSpecs(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	covGroupJSON := `{"id":1,"key":"production","process_mode":"unordered"}`

	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v4/projects/1/resource_groups", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+covGroupJSON+`]`)
	})

	handler.HandleFunc("GET /api/v4/projects/1/resource_groups/production", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covGroupJSON)
	})

	handler.HandleFunc("PUT /api/v4/projects/1/resource_groups/production", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"key":"production","process_mode":"newest_first"}`)
	})

	handler.HandleFunc("GET /api/v4/projects/1/resource_groups/production/upcoming_jobs", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":10,"name":"deploy","status":"pending","stage":"deploy"}]`)
	})

	client := testutil.NewTestClient(t, handler)
	return resourceGroupSpecsByTool(ActionSpecs(client))
}

// resourceGroupSpecsByTool supports resource group specs by tool assertions in resourcegroups tests.
func resourceGroupSpecsByTool(specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}
	return specByTool
}
