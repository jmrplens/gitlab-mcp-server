package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apidocs"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// boardsPage is a cut-down doc/api/boards.md: a Free page whose board update
// marks its four scope parameters "Premium and Ultimate only" and whose list
// create marks the iteration list type "Ultimate only" (the real page says
// Premium; Ultimate is what lets one fixture hold both comparisons). The
// response attribute table of the update carries a Premium row of its own,
// which is what GitLab sends rather than what a caller may ask for, and a
// shell comment in the request example stands where a heading would split the
// section.
const boardsPage = "{{< details >}}\n\n- Tier: Free, Premium, Ultimate\n\n{{< /details >}}\n\n" +
	"## Update an issue board\n\n" +
	"```plaintext\nPUT /projects/:id/boards/:board_id\n```\n\n" +
	"```shell\n# the board to update\ncurl --request PUT \"https://gitlab.example.com/api/v4/projects/5/boards/1\"\n```\n\n" +
	"| Attribute | Type | Required | Description |\n" +
	"| --- | --- | --- | --- |\n" +
	"| `id` | integer or string | yes | The ID of the project. |\n" +
	"| `name` | string | no | The new name of the board. |\n" +
	"| `assignee_id` | integer | no | The assignee the board should be scoped to. Premium and Ultimate only. |\n" +
	"| `labels` | string | no | Comma-separated list of label names. Premium and Ultimate only. |\n" +
	"| `weight` | integer | no | The weight range from 0 to 9. Premium and Ultimate only. |\n\n" +
	"| Attribute | Type | Description |\n" +
	"| --- | --- | --- |\n" +
	"| `name` | string | Premium and Ultimate only. |\n\n" +
	"## Create a board list\n\n" +
	"```plaintext\nPOST /projects/:id/boards/:board_id/lists\n```\n\n" +
	"| Attribute | Type | Required | Description |\n" +
	"| --- | --- | --- | --- |\n" +
	"| `label_id` | integer | no | The ID of a label. |\n" +
	"| `iteration_id` | integer | no | The ID of an iteration. Ultimate only. |\n"

// TestParseParamSections_BoardsPage_KeepsEachSectionsEndpointsAndTieredRequestRows
// reads a page shaped like GitLab's boards page and holds what the parameter
// pass is built on: one section per heading, each with the endpoint its code
// block spells and the request rows it marks for a paid tier, with the
// untiered rows, the response attribute table and the shell comment leaving no
// trace.
func TestParseParamSections_BoardsPage_KeepsEachSectionsEndpointsAndTieredRequestRows(t *testing.T) {
	want := []docSection{
		{
			endpoints: []docEndpoint{{method: "PUT", segments: []string{"projects", ":id", "boards", ":board_id"}}},
			params: []docParam{
				{name: "assignee_id", tier: tierPremium},
				{name: "labels", tier: tierPremium},
				{name: "weight", tier: tierPremium},
			},
		},
		{
			endpoints: []docEndpoint{{method: "POST", segments: []string{"projects", ":id", "boards", ":board_id", "lists"}}},
			params:    []docParam{{name: "iteration_id", tier: tierUltimate}},
		},
	}
	if got := parseParamSections(boardsPage); !reflect.DeepEqual(got, want) {
		t.Errorf("parseParamSections(boards) =\n %+v\nwant\n %+v", got, want)
	}
}

// TestParseParamSections_SectionShapes_KeepOnlyAnEndpointBesideATieredRow holds
// the section rule case by case: a section is kept only when it spells an
// endpoint and marks a request row, a heading closes the section before it, a
// sentence opening with a method name is not an endpoint, and a table is a
// request table only when its header has a Required column.
func TestParseParamSections_SectionShapes_KeepOnlyAnEndpointBesideATieredRow(t *testing.T) {
	const (
		fencePut  = "```plaintext\nPUT /things/:id\n```\n"
		reqHeader = "| Attribute | Type | Required | Description |\n| --- | --- | --- | --- |\n"
		tieredRow = "| `weight` | integer | no | Premium and Ultimate only. |\n"
		putThings = "PUT"
	)
	thing := docSection{
		endpoints: []docEndpoint{{method: putThings, segments: []string{"things", ":id"}}},
		params:    []docParam{{name: "weight", tier: tierPremium}},
	}
	cases := []struct {
		name string
		page string
		want []docSection
	}{
		{name: "endpoint_and_tiered_row_kept", page: "## A\n" + fencePut + reqHeader + tieredRow, want: []docSection{thing}},
		{name: "no_endpoint_dropped", page: "## A\n" + reqHeader + tieredRow},
		{name: "no_tiered_row_dropped", page: "## A\n" + fencePut + reqHeader + "| `name` | string | no | The name. |\n"},
		{name: "heading_splits_endpoint_from_rows", page: "## A\n" + fencePut + "### B\n" + reqHeader + tieredRow},
		{name: "response_table_ignored", page: "## A\n" + fencePut + "| Attribute | Type | Description |\n| --- | --- | --- |\n" + tieredRow},
		{name: "method_sentence_is_no_endpoint", page: "## A\n```plaintext\nPUT is the method this takes\n```\n" + reqHeader + tieredRow},
		{name: "text_line_ends_the_table", page: "## A\n" + fencePut + reqHeader + "Supported attributes:\n" + tieredRow},
		{
			name: "two_sections_each_kept",
			page: "## A\n" + fencePut + reqHeader + tieredRow + "\n## B\n```plaintext\nGET /api/v4/things/:id/items?page=1\n```\n" + reqHeader +
				"| `assignee_ids[]` | array | no | Ultimate only. |\n",
			want: []docSection{thing, {
				endpoints: []docEndpoint{{method: "GET", segments: []string{"things", ":id", "items"}}},
				params:    []docParam{{name: "assignee_ids", tier: tierUltimate}},
			}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseParamSections(tc.page); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseParamSections =\n %+v\nwant\n %+v", got, tc.want)
			}
		})
	}
}

// TestParseParamRow_Rows_ReadTheNameAndTheTierTheRowMarks holds the row reader:
// a backticked first cell is the parameter, a nested name is read as the
// parameter it belongs to, and a row that marks no tier, or a row too short to
// carry one, is not a parameter row.
func TestParseParamRow_Rows_ReadTheNameAndTheTierTheRowMarks(t *testing.T) {
	cases := []struct {
		name   string
		row    string
		want   docParam
		wantOK bool
	}{
		{name: "premium_row", row: "| `weight` | integer | no | Premium and Ultimate only. |", want: docParam{name: "weight", tier: tierPremium}, wantOK: true},
		{name: "nested_name", row: "| `position[base_sha]` | string | yes | Premium and Ultimate only. |", want: docParam{name: "position", tier: tierPremium}, wantOK: true},
		{name: "untiered_row", row: "| `name` | string | no | The name. |"},
		{name: "unquoted_name", row: "| name | string | no | Premium and Ultimate only. |"},
		{name: "one_cell", row: "| `weight` "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseParamRow(tc.row)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("parseParamRow(%q) = %+v, %v; want %+v, %v", tc.row, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// TestParamRowTier_Phrasings_MapToTheMinimumTierTheRowNames holds the tier a
// row's text marks its parameter for against every phrasing GitLab's pages
// use, and the three ways a row names a tier that is not the parameter's: a
// tier whose subject is another key of the value, one the Free tier is also
// served, and none at all.
func TestParamRowTier_Phrasings_MapToTheMinimumTierTheRowNames(t *testing.T) {
	cases := []struct {
		name   string
		param  string
		rest   string
		want   tier
		wantOK bool
	}{
		{name: "premium_and_ultimate_only", param: "weight", rest: " integer | no | Premium and Ultimate only. |", want: tierPremium, wantOK: true},
		{name: "ultimate_only", param: "member_role_id", rest: " integer | no | Ultimate only. The ID of a custom role. |", want: tierUltimate, wantOK: true},
		{name: "premium_only", param: "x", rest: " no | Premium only. |", want: tierPremium, wantOK: true},
		{name: "premium_all_badge", param: "token_expires_at", rest: " datetime | no | **(PREMIUM ALL)** |", want: tierPremium, wantOK: true},
		{name: "ultimate_all_badge", param: "x", rest: " no | **(ULTIMATE ALL)** |", want: tierUltimate, wantOK: true},
		{name: "in_the_tier", param: "new_epic", rest: " boolean | No | When a new epic is created (in the Premium and Ultimate tier) |", want: tierPremium, wantOK: true},
		{name: "self_managed_qualifier", param: "x", rest: " no | GitLab Self-Managed, Premium and Ultimate only. |", want: tierPremium, wantOK: true},
		{name: "subject_is_the_parameter", param: "member_role_id", rest: " integer | no | `member_role_id` is Ultimate only. |", want: tierUltimate, wantOK: true},
		{name: "subject_is_another_key", param: "allowed_to_merge", rest: " array | no | `member_role_id` is Ultimate only. |"},
		// Every subject is read, not the first one alone: a row naming the
		// parameter and then another key grades nothing.
		{name: "second_subject_is_another_key", param: "member_role_id", rest: " integer | no | `member_role_id` is Ultimate only. `access_level` is Ultimate only. |"},
		{name: "free_on_gitlab_com_too", param: "duo", rest: " boolean | Premium and Ultimate. Also available on the Free tier on GitLab.com with GitLab Credits. |"},
		{name: "no_tier", param: "name", rest: " string | no | The name. |"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := paramRowTier(tc.param, tc.rest)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("paramRowTier(%q, %q) = %v, %v; want %v, %v", tc.param, tc.rest, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// TestDocSection_Documents_MatchesTheMethodAndThePathShape holds the join from
// a recorded route to a documented endpoint: the method has to agree, the path
// has to have as many segments, and a segment matches when it is the same
// literal or a placeholder on both sides, whichever way each side spells it.
func TestDocSection_Documents_MatchesTheMethodAndThePathShape(t *testing.T) {
	section := docSection{endpoints: []docEndpoint{
		{method: "PUT", segments: []string{"projects", ":id", "boards", "<board_id>"}},
		{method: "GET", segments: []string{"projects", ":id", "files", "*path"}},
		{method: "GET", segments: []string{"projects", ":id", "search"}},
	}}
	cases := []struct {
		name  string
		route string
		want  bool
	}{
		{name: "same_shape", route: "PUT /projects/:id/boards/:board_id", want: true},
		{name: "glob_against_placeholder", route: "GET /projects/:id/files/:file_path", want: true},
		{name: "optional_segment_left_out", route: "GET /projects/:id/(-/)search", want: true},
		{name: "optional_segment_on_another_path", route: "GET /groups/:id/(-/)search"},
		{name: "other_method", route: "POST /projects/:id/boards/:board_id"},
		{name: "other_literal", route: "PUT /groups/:id/boards/:board_id"},
		{name: "literal_against_placeholder", route: "PUT /projects/:id/boards/lists"},
		{name: "longer_path", route: "PUT /projects/:id/boards/:board_id/lists"},
		{name: "no_path", route: "PUT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := section.documents(tc.route); got != tc.want {
				t.Errorf("documents(%q) = %v, want %v", tc.route, got, tc.want)
			}
		})
	}
}

// TestPathSegments_WrittenPaths_SplitWithoutPrefixQueryOrEdgeSlashes holds the
// path normalization both sides of the join go through.
func TestPathSegments_WrittenPaths_SplitWithoutPrefixQueryOrEdgeSlashes(t *testing.T) {
	cases := []struct {
		name string
		path string
		want []string
	}{
		{name: "plain", path: "/projects/:id", want: []string{"projects", ":id"}},
		{name: "api_prefix", path: "/api/v4/projects/:id", want: []string{"projects", ":id"}},
		{name: "query", path: "/projects/:id?page=2", want: []string{"projects", ":id"}},
		{name: "trailing_slash", path: "projects/:id/", want: []string{"projects", ":id"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pathSegments(tc.path); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("pathSegments(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

// TestOptionalForms_GrapeGroups_ExpandToThePathWithAndWithoutEach holds the
// expansion of the optional groups Grape writes into a recorded route, "(-/)"
// in front of a scoped search or "(ref/:ref/)" in a trigger, into every path
// the route answers at: split as written, "(-" and ")search" are segments no
// documented endpoint has, so the route joined nothing.
func TestOptionalForms_GrapeGroups_ExpandToThePathWithAndWithoutEach(t *testing.T) {
	cases := []struct {
		name string
		path string
		want []string
	}{
		{name: "no_group", path: "/projects/:id/search", want: []string{"/projects/:id/search"}},
		{name: "one_group", path: "/projects/:id/(-/)search", want: []string{"/projects/:id/-/search", "/projects/:id/search"}},
		{
			name: "two_groups", path: "/releases/permalink/latest(/)(*suffix_path)",
			want: []string{
				"/releases/permalink/latest/*suffix_path", "/releases/permalink/latest/",
				"/releases/permalink/latest*suffix_path", "/releases/permalink/latest",
			},
		},
		{name: "nested_group", path: "/a/((b/)c/)d", want: []string{"/a/b/c/d", "/a/c/d", "/a/d"}},
		{name: "group_first", path: "(a/)b", want: []string{"a/b", "b"}},
		{name: "unclosed_group", path: "/a/(b/c", want: []string{"/a/(b/c"}},
		{name: "unclosed_after_a_group", path: "/a/(b/)c(d", want: []string{"/a/b/c(d", "/a/c(d"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := optionalForms(tc.path); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("optionalForms(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

// seedRecord writes an action request record holding actions under root, at
// the path the committed one lives at.
func seedRecord(t *testing.T, root string, actions ...actionrequests.RecordAction) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(actionrequests.RecordPath))
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		t.Fatalf("create record directory: %v", err)
	}
	if err := os.WriteFile(target, actionrequests.RenderRecord(actions), 0o600); err != nil {
		t.Fatalf("write record: %v", err)
	}
}

// TestReadActionRoutes_Record_KeysEachActionsRESTRoutesByID holds the reader:
// the REST routes of each action, in record order, and nothing for a GraphQL
// request, which carries no route to join a page to.
func TestReadActionRoutes_Record_KeysEachActionsRESTRoutesByID(t *testing.T) {
	root := t.TempDir()
	seedRecord(t, root,
		actionrequests.RecordAction{ID: "project.board_update", Requests: []actionrequests.RecordRequest{
			{Kind: actionrequests.KindREST, Route: "GET /projects/:id/boards/:board_id"},
			{Kind: actionrequests.KindREST, Route: "PUT /projects/:id/boards/:board_id"},
		}},
		actionrequests.RecordAction{ID: "vulnerability.list", Requests: []actionrequests.RecordRequest{
			{Kind: actionrequests.KindGraphQL, Operation: "vulnerabilities"},
		}},
	)
	got, err := readActionRoutes(root)
	if err != nil {
		t.Fatalf("readActionRoutes: %v", err)
	}
	want := map[string][]string{"project.board_update": {"GET /projects/:id/boards/:board_id", "PUT /projects/:id/boards/:board_id"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("readActionRoutes = %v, want %v", got, want)
	}
}

// TestReadActionRoutes_NoRecord_IsAnError holds that a checkout without the
// record is refused rather than graded as though no action sent anything,
// which would report no parameter finding for a reason nobody can see.
func TestReadActionRoutes_NoRecord_IsAnError(t *testing.T) {
	got, err := readActionRoutes(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "read the action request record") {
		t.Fatalf("readActionRoutes on an empty root = %v, %v; want the record read refused", got, err)
	}
}

// schemaAction is one hand-built action carrying an input schema with the
// named properties.
func schemaAction(domain, name, owner string, props ...string) actioncatalog.Action {
	properties := map[string]any{}
	for _, prop := range props {
		properties[prop] = map[string]any{"type": "string"}
	}
	return actioncatalog.Action{
		Name: name, Domain: domain, OwnerPackage: owner,
		Route: toolutil.ActionRoute{InputSchema: map[string]any{"type": "object", "properties": properties}},
	}
}

// schemaCatalog builds a catalog holding the given actions in one group.
func schemaCatalog(t *testing.T, domain string, actions ...actioncatalog.Action) *actioncatalog.Catalog {
	t.Helper()
	group := actioncatalog.Group{ToolName: "gitlab_" + domain, BaseDomain: domain, Actions: map[string]actioncatalog.Action{}}
	for _, action := range actions {
		group.ActionOrder = append(group.ActionOrder, action.Name)
		group.Actions[action.Name] = action
	}
	cat := actioncatalog.NewCatalog()
	if err := cat.AddGroup(group); err != nil {
		t.Fatalf("AddGroup: %v", err)
	}
	return cat
}

// stubTierCatalogs points the buildCatalog seam at one catalog per tier: the
// Enterprise build is Ultimate, the Premium tier build is Premium, and the
// rest is Free. The real seam is restored when the test ends.
func stubTierCatalogs(t *testing.T, free, premium, ultimate *actioncatalog.Catalog) {
	t.Helper()
	buildCatalog = func(_ *gitlabclient.Client, opts tools.ActionCatalogOptions) (*actioncatalog.Catalog, error) {
		switch {
		case opts.Enterprise:
			return ultimate, nil
		case opts.Tier == edition.Premium:
			return premium, nil
		default:
			return free, nil
		}
	}
	t.Cleanup(func() { buildCatalog = tools.BuildActionCatalog })
}

// TestOfferedTiers_ThreeCatalogs_RecordTheLowestTierOfferingEachParameter holds
// the schema half of the join: a parameter is offered at the lowest tier whose
// input schema carries it, an action only a higher tier serves takes that
// tier for every parameter, and an action carrying no properties offers none.
func TestOfferedTiers_ThreeCatalogs_RecordTheLowestTierOfferingEachParameter(t *testing.T) {
	free := schemaCatalog(t, "project", schemaAction("project", "board_update", "boards", "name"))
	premium := schemaCatalog(t, "project",
		schemaAction("project", "board_update", "boards", "name", "weight"),
		schemaAction("project", "epic_list", "epics", "state"))
	ultimate := schemaCatalog(t, "project",
		schemaAction("project", "board_update", "boards", "name", "weight", "health"),
		schemaAction("project", "epic_list", "epics", "state"),
		actioncatalog.Action{Name: "bare", Domain: "project"})
	want := map[string]map[string]tier{
		"project.board_update": {"name": tierFree, "weight": tierPremium, "health": tierUltimate},
		"project.epic_list":    {"state": tierPremium},
	}
	if got := offeredTiers(free, premium, ultimate); !reflect.DeepEqual(got, want) {
		t.Errorf("offeredTiers = %v, want %v", got, want)
	}
}

// boardsFixture is the catalog, record and page of the parameter pass's
// end-to-end test: a board update whose Free schema still offers the Premium
// labels and weight but not the Premium assignee, and a list create whose
// Premium schema offers the Ultimate iteration list. The group update is a
// second domain whose page marks nothing, so it is graded and finds nothing.
func boardsFixture(t *testing.T) *docResolver {
	t.Helper()
	update := func(props ...string) actioncatalog.Action {
		return schemaAction("project", "board_update", "boards", append([]string{"project_id", "board_id", "name"}, props...)...)
	}
	listCreate := func(props ...string) actioncatalog.Action {
		return schemaAction("project", "board_list_create", "boards", append([]string{"label_id"}, props...)...)
	}
	groupUpdate := schemaAction("project", "group_board_update", "groupboards", "name", "weight")
	stubTierCatalogs(t,
		schemaCatalog(t, "project", update("labels", "weight"), listCreate(), groupUpdate),
		schemaCatalog(t, "project", update("labels", "weight", "assignee_id"), listCreate("iteration_id"), groupUpdate),
		schemaCatalog(t, "project", update("labels", "weight", "assignee_id"), listCreate("iteration_id"), groupUpdate),
	)
	dir := t.TempDir()
	seedDoc(t, dir, "boards", boardsPage)
	seedDoc(t, dir, "group_boards", freeBadgeDoc)
	res := newOfflineResolver(t, dir)
	res.routes = map[string][]string{
		"project.board_update":       {"PUT /projects/:id/boards/:board_id", "PUT /projects/:id/boards/:board_id"},
		"project.board_list_create":  {"POST /projects/:id/boards/:board_id/lists"},
		"project.group_board_update": {"PUT /groups/:id/boards/:board_id"},
	}
	return res
}

// TestBuildReport_BoardsPage_ReportsEachTieredParameterALowerTierIsOffered is
// the pass end to end, and the reason it exists (issue 1233): a board input
// GitLab marks Premium that the Free schema offers is a finding naming the
// action, the route, the page and both tiers, as is an Ultimate list type the
// Premium schema offers. A Premium parameter the Free schema already prunes is
// joined and not reported, a route recorded twice reports once, the domain
// needs work for its findings although its page tier is green, and the summary
// counts every joined row and every finding.
func TestBuildReport_BoardsPage_ReportsEachTieredParameterALowerTierIsOffered(t *testing.T) {
	rep, err := buildReport(context.Background(), boardsFixture(t))
	if err != nil {
		t.Fatalf("buildReport: %v", err)
	}
	boards := findDomain(t, rep, "boards")
	const update, doc = "PUT /projects/:id/boards/:board_id", "doc/api/boards.md"
	// Actions are graded in ID order, which puts the list create first.
	want := []paramFinding{
		{
			Action: "project.board_list_create", Route: "POST /projects/:id/boards/:board_id/lists", Param: "iteration_id",
			Documented: "ultimate", OfferedAt: "premium", Doc: doc,
		},
		{Action: "project.board_update", Route: update, Param: "labels", Documented: "premium", OfferedAt: "free", Doc: doc},
		{Action: "project.board_update", Route: update, Param: "weight", Documented: "premium", OfferedAt: "free", Doc: doc},
	}
	if !reflect.DeepEqual(boards.ParamFindings, want) {
		t.Errorf("boards findings =\n %+v\nwant\n %+v", boards.ParamFindings, want)
	}
	// Three update rows on each of two recorded routes, and one list row.
	if boards.ParamRowsJoined != 7 {
		t.Errorf("boards joined %d rows, want 7", boards.ParamRowsJoined)
	}
	if boards.Classification != "green" || !boards.NeedsWork {
		t.Errorf("boards = class %q, needs work %v; want a green page that needs work for its findings", boards.Classification, boards.NeedsWork)
	}
	groups := findDomain(t, rep, "groupboards")
	if groups.ParamFindings != nil || groups.ParamRowsJoined != 0 || groups.NeedsWork {
		t.Errorf("groupboards = findings %+v, joined %d, needs work %v; want a page marking nothing to report nothing",
			groups.ParamFindings, groups.ParamRowsJoined, groups.NeedsWork)
	}
	if rep.Summary.ParamFindings != 3 || rep.Summary.ParamRowsJoined != 7 {
		t.Errorf("summary = %d findings over %d rows, want 3 over 7", rep.Summary.ParamFindings, rep.Summary.ParamRowsJoined)
	}
	data, err := json.Marshal(boards)
	if err != nil {
		t.Fatalf("marshal boards: %v", err)
	}
	if !strings.Contains(string(data), `"param_rows_joined":7,"param_findings":[{"action":"project.board_list_create"`) {
		t.Errorf("boards JSON = %s, want the findings under param_findings", data)
	}
}

// TestGradeParams_OverridePage_IsReadBesideTheOwnerPage holds that an action
// redirected to another page is graded against that page's rows too, and that
// an override page that cannot be fetched grades the owner page alone rather
// than failing the domain.
func TestGradeParams_OverridePage_IsReadBesideTheOwnerPage(t *testing.T) {
	const hookPage = "## Edit a hook\n```plaintext\nPUT /groups/:id/hooks/:hook_id\n```\n" +
		"| Attribute | Type | Required | Description |\n| --- | --- | --- | --- |\n" +
		"| `vulnerability_events` | boolean | no | Ultimate only. |\n"
	offered := map[string]map[string]tier{"group.hook_edit": {"vulnerability_events": tierFree}}
	actions := []actionDetail{{ID: "group.hook_edit"}}
	cases := []struct {
		name string
		seed bool
		want []paramFinding
	}{
		{name: "override_page_read", seed: true, want: []paramFinding{{
			Action: "group.hook_edit", Route: "PUT /groups/:id/hooks/:hook_id", Param: "vulnerability_events",
			Documented: "ultimate", OfferedAt: "free", Doc: "doc/api/group_webhooks.md",
		}}},
		{name: "override_page_missing", seed: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.seed {
				seedDoc(t, dir, "group_webhooks", hookPage)
			}
			res := newOfflineResolver(t, dir)
			res.routes = map[string][]string{"group.hook_edit": {"PUT /groups/:id/hooks/:hook_id"}}
			owner := pageSections{ref: docRef{area: "groups"}}
			got, joined := res.gradeParams(context.Background(), owner, actions, offered)
			if !reflect.DeepEqual(got, tc.want) || joined != len(tc.want) {
				t.Errorf("gradeParams = %+v over %d rows, want %+v over %d", got, joined, tc.want, len(tc.want))
			}
		})
	}
}

// TestRunMain_NoActionRecord_ExitsOneNamingIt holds the entry point to the
// reader's refusal: a module root without the record stops before any page is
// read or any catalog built.
func TestRunMain_NoActionRecord_ExitsOneNamingIt(t *testing.T) {
	calls := stubCatalogs(t, actioncatalog.NewCatalog(), actioncatalog.NewCatalog(), nil)
	root := fakeModuleRoot(t)
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(actionrequests.RecordPath))); err != nil {
		t.Fatalf("remove the seeded record: %v", err)
	}
	t.Chdir(root)

	var stdout, stderr strings.Builder
	if got := runMain([]string{"audit_edition_tier", "-offline"}, &stdout, &stderr); got != 1 {
		t.Errorf("runMain without a record = %d, want 1", got)
	}
	if !strings.HasPrefix(stderr.String(), "read the action request record: ") {
		t.Errorf("stderr = %q, want it to name the record read", stderr.String())
	}
	if stdout.Len() != 0 || *calls != 0 {
		t.Errorf("stdout = %q and %d catalog builds, want neither without a record", stdout.String(), *calls)
	}
}

// TestRunMain_FakeRoot_JoinsTheRecordsRoutesToThePage drives the entry point
// over a module root whose record names the board update's route and whose
// cache holds the boards page, so the routes runMain reads are the ones the
// report joins: the real catalog's Free board update offers no Premium scope
// parameter, and the page's rows are joined to it.
func TestRunMain_FakeRoot_JoinsTheRecordsRoutesToThePage(t *testing.T) {
	root := fakeModuleRoot(t)
	seedRecord(t, root, actionrequests.RecordAction{ID: "project.board_update", Requests: []actionrequests.RecordRequest{
		{Kind: actionrequests.KindREST, Route: "PUT /projects/:id/boards/:board_id"},
	}})
	seedDoc(t, apidocs.CacheDir(root), "boards", boardsPage)
	t.Chdir(root)

	rep := runReport(t, "-offline")
	boards := findDomain(t, rep, "boards")
	if boards.ParamRowsJoined != 3 || boards.ParamFindings != nil {
		t.Errorf("boards = %d rows joined, findings %+v; want the three update rows joined and none offered below Premium",
			boards.ParamRowsJoined, boards.ParamFindings)
	}
}

// TestBuildReport_PremiumCatalogBuildFailure_IsNamed holds the third catalog
// build to the same rule as the other two: a failure says which tier failed.
func TestBuildReport_PremiumCatalogBuildFailure_IsNamed(t *testing.T) {
	cause := errors.New("catalog is broken")
	buildCatalog = func(_ *gitlabclient.Client, opts tools.ActionCatalogOptions) (*actioncatalog.Catalog, error) {
		if opts.Tier == edition.Premium {
			return nil, cause
		}
		return actioncatalog.NewCatalog(), nil
	}
	t.Cleanup(func() { buildCatalog = tools.BuildActionCatalog })

	rep, err := buildReport(context.Background(), newOfflineResolver(t, t.TempDir()))
	if rep != nil || err == nil || !strings.HasPrefix(err.Error(), "build Premium catalog: ") || !errors.Is(err, cause) {
		t.Fatalf("buildReport = %+v, %v; want the Premium build named and the cause wrapped", rep, err)
	}
}
