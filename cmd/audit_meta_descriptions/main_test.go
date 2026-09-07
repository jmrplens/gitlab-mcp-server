// Package main tests the meta description auditor: the extraction rule for
// both blocks a description enumerates parameters in, the comparison against
// the routes' input schemas, the report and its exit codes, and one full run
// over the served surface, which is the CI gate's own assertion.
package main

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// metaPreamble is the header StripMetaToolDescriptionPrefix removes, so a test
// description reaches the enumeration rule the way a served one does.
const metaPreamble = "Use {\"action\":\"list\",\"params\":{...}}. The only top-level keys are action and params.\n" +
	"Action params schema: gitlab://tools/gitlab_widget.<action>.\n\n"

// captureStdout redirects the command's report stream into a buffer for the
// duration of the test.
func captureStdout(t *testing.T) *bytes.Buffer {
	t.Helper()
	out := &bytes.Buffer{}
	previous := stdout
	stdout = out
	t.Cleanup(func() { stdout = previous })
	return out
}

// mentionNames reduces parsed mentions to the names they carry, which is what
// most cases assert on.
func mentionNames(mentions []mention) []string {
	names := make([]string, 0, len(mentions))
	for _, m := range mentions {
		names = append(names, m.name)
	}
	return names
}

// TestParseEnumerationLine_HouseShapes_ParsedOrSkippedWhole verifies the
// extraction rule against every shape the served descriptions really use, and
// against the prose shapes it must refuse.
//
// A refusal is the important half. The rule reads a line only when the whole
// line parses, so a description sentence, a "Returns:" entry and an
// enumeration ending in a remark are all skipped rather than mined for the
// words that happen to look like parameter names: a guess here is a false
// failure, which is worse than a missed one.
func TestParseEnumerationLine_HouseShapes_ParsedOrSkippedWhole(t *testing.T) {
	cases := []struct {
		name   string
		line   string
		parsed bool
		names  []string
		values map[string][]string
	}{
		{name: "single_required_parameter", line: "- feature_delete: name*", parsed: true, names: []string{"name"}},
		{
			name:   "annotations_and_enum_values",
			line:   "- list: search, scope (ALL/NAMESPACES), first (max 100), after (cursor)",
			parsed: true,
			names:  []string{"search", "scope", "first", "after"},
			values: map[string][]string{"scope": {"ALL", "NAMESPACES"}},
		},
		{
			name:   "several_actions_and_alternatives",
			line:   "- token_project_get / token_group_get: project_id* or group_id*, token_id*",
			parsed: true,
			names:  []string{"project_id", "group_id", "token_id"},
		},
		{
			name:   "or_inside_an_annotation_is_not_an_alternative",
			line:   "- namespace_get: id* (numeric ID or full path)",
			parsed: true,
			names:  []string{"id"},
		},
		{
			name:   "comma_inside_an_annotation_stays_one_item",
			line:   "- freeze_create: project_id*, freeze_start* (cron, e.g. '0 23 * * 5')",
			parsed: true,
			names:  []string{"project_id", "freeze_start"},
		},
		{name: "leading_parenthetical_remark", line: "- list_all: (admin) type, status", parsed: true, names: []string{"type", "status"}},
		{name: "no_params_enumerates_nothing", line: "- license_get: (no params)", parsed: true},
		{
			name:   "remark_after_a_sentence_break_is_dropped",
			line:   "- create_group: group_id*, name*. Same permission booleans as create_instance.",
			parsed: true,
			names:  []string{"group_id", "name"},
		},
		{name: "wildcard_action_head", line: "- token_group_*: group_id*", parsed: true, names: []string{"group_id"}},
		{name: "trailing_prose_item_skips_the_line", line: "- hook_edit: group_id*, hook_id*, same params as hook_add"},
		{name: "returned_shape_is_not_an_enumeration", line: "- get: {id, full_path, name}."},
		{name: "guidance_sentence_is_not_an_enumeration", line: "- list: Browse the CI/CD Catalog of published component projects."},
		{name: "unbalanced_parenthetical_skips_the_line", line: "- get: (admin only"},
		{name: "not_a_bullet", line: "Returns: a page of issues."},
		{name: "slashed_parameter_names_skip_the_line", line: "- protected_branch_update: group_id*, allowed_to_push/merge"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params, ok := parseEnumerationLine(tc.line)
			if ok != tc.parsed {
				t.Fatalf("parseEnumerationLine(%q) parsed = %v, want %v", tc.line, ok, tc.parsed)
			}
			if !ok {
				return
			}
			if got := mentionNames(params); !slices.Equal(got, tc.names) {
				t.Errorf("parameter names = %v, want %v", got, tc.names)
			}
			for _, m := range params {
				want := tc.values[m.name]
				if !slices.Equal(m.values, want) {
					t.Errorf("%q enum values = %v, want %v", m.name, m.values, want)
				}
			}
		})
	}
}

// TestParseEnumerations_StripsPreambleAndReadsTheCuratedBody verifies the
// enumeration side reads only what survives StripMetaToolDescriptionPrefix,
// so the action-guidance bullets above the curated body are not mistaken for
// parameter lines.
func TestParseEnumerations_StripsPreambleAndReadsTheCuratedBody(t *testing.T) {
	description := metaPreamble + "Widget actions.\n\n- list: search, first (max 100)\n- get: {id, name}.\n"

	found := parseEnumerations(description)
	if len(found) != 1 {
		t.Fatalf("parseEnumerations() = %+v, want the one parameter line", found)
	}
	if found[0].text != "- list: search, first (max 100)" {
		t.Errorf("line = %q, want the enumeration line verbatim", found[0].text)
	}
	if got := mentionNames(found[0].params); !slices.Equal(got, []string{"search", "first"}) {
		t.Errorf("parameter names = %v, want [search first]", got)
	}
}

// TestParseGuidance_ReadsTheBlockAndStopsAtItsEnd verifies the guidance rule
// takes its lines from the "Parameter guidance:" block alone and ends at the
// first line that is not one of its bullets, so the enumeration block further
// down is left to the other rule.
func TestParseGuidance_ReadsTheBlockAndStopsAtItsEnd(t *testing.T) {
	description := "Action guidance:\n- list: Browse the catalog.\n\n" +
		"Parameter guidance:\n" +
		"- list.scope: catalog_scope. Source: ALL or NAMESPACES.\n" +
		"- get.full_path: project_path. Source: namespace/project.\n" +
		"\n- list: search, scope (ALL/NAMESPACES)\n"

	found := parseGuidance(description)
	if len(found) != 2 {
		t.Fatalf("parseGuidance() = %+v, want the two guidance lines", found)
	}
	if found[0].action != "list" || found[0].param != "scope" {
		t.Errorf("first guidance = %+v, want list.scope", found[0])
	}
	if found[1].action != "get" || found[1].param != "full_path" {
		t.Errorf("second guidance = %+v, want get.full_path", found[1])
	}
	if !strings.HasPrefix(found[0].text, "- list.scope:") {
		t.Errorf("line = %q, want the guidance line verbatim", found[0].text)
	}
}

// TestAcceptedAdd_SchemaShapes_CollectsNamesAndStringEnums verifies the
// accepted side reads top-level property names and the string enum values
// beside them, and survives every shape a schema can arrive in without one.
func TestAcceptedAdd_SchemaShapes_CollectsNamesAndStringEnums(t *testing.T) {
	cases := []struct {
		name   string
		schema map[string]any
		names  []string
		values map[string][]string
	}{
		{name: "nil_schema_adds_nothing", schema: nil},
		{name: "properties_of_the_wrong_type", schema: map[string]any{"properties": []any{"scope"}}},
		{
			name:   "property_that_is_not_an_object",
			schema: map[string]any{"properties": map[string]any{"scope": "string"}},
			names:  []string{"scope"},
		},
		{
			name:   "property_without_an_enum",
			schema: map[string]any{"properties": map[string]any{"search": map[string]any{"type": "string"}}},
			names:  []string{"search"},
		},
		{
			name: "enum_keeps_only_its_string_values",
			schema: map[string]any{"properties": map[string]any{
				"scope": map[string]any{"type": "string", "enum": []any{"ALL", 30, "NAMESPACES"}},
			}},
			names:  []string{"scope"},
			values: map[string][]string{"scope": {"ALL", "NAMESPACES"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			allowed := newAccepted()
			allowed.add(tc.schema)

			names := make([]string, 0, len(allowed.names))
			for name := range allowed.names {
				names = append(names, name)
			}
			slices.Sort(names)
			if !slices.Equal(names, tc.names) {
				t.Errorf("names = %v, want %v", names, tc.names)
			}
			for name, want := range tc.values {
				for _, value := range want {
					if !allowed.values[name][value] {
						t.Errorf("%q enum does not carry %q", name, value)
					}
				}
			}
			if len(tc.values) == 0 && len(allowed.values) != 0 {
				t.Errorf("values = %v, want none", allowed.values)
			}
		})
	}
}

// TestSchemaMap_UnreadableSchemas_DecodeToNothing verifies a served schema
// that is not a JSON object yields no accepted names rather than a panic. A
// tool with no schema and one whose schema cannot be marshaled are both
// simply nothing to compare a description against.
func TestSchemaMap_UnreadableSchemas_DecodeToNothing(t *testing.T) {
	cases := []struct {
		name   string
		schema any
		want   bool
	}{
		{name: "nil"},
		{name: "unmarshalable", schema: make(chan int)},
		{name: "not_an_object", schema: 42},
		{name: "object", schema: map[string]any{"type": "object"}, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := schemaMap(tc.schema) != nil; got != tc.want {
				t.Errorf("schemaMap(%v) != nil = %v, want %v", tc.schema, got, tc.want)
			}
		})
	}
}

// TestJudge_EnumerationLine_ReportsUnknownNamesAndValues verifies the two
// enumeration findings and the case that must stay silent: a parameter whose
// routes publish no enum at all has its spelled values left unjudged, because
// the prose is then the only description of the value set there is.
func TestJudge_EnumerationLine_ReportsUnknownNamesAndValues(t *testing.T) {
	allowed := newAccepted()
	allowed.add(map[string]any{"properties": map[string]any{
		"scope":  map[string]any{"type": "string", "enum": []any{"ALL", "NAMESPACES"}},
		"state":  map[string]any{"type": "string"},
		"search": map[string]any{"type": "string"},
	}})

	cases := []struct {
		name  string
		line  enumeration
		kinds []string
		want  []string
	}{
		{name: "known_name_and_value", line: enumeration{params: []mention{{name: "scope", values: []string{"ALL"}}}}},
		{
			name:  "unknown_name",
			line:  enumeration{params: []mention{{name: "confidence"}}},
			kinds: []string{kindParameter},
			want:  []string{"confidence"},
		},
		{
			name:  "unknown_value",
			line:  enumeration{params: []mention{{name: "scope", values: []string{"NAMESPACED"}}}},
			kinds: []string{kindEnumValue},
			want:  []string{"scope=NAMESPACED"},
		},
		{name: "value_of_a_parameter_that_publishes_no_enum", line: enumeration{params: []mention{{name: "state", values: []string{"opened", "closed"}}}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			found := judge("gitlab_widget", tc.line, allowed)
			if len(found) != len(tc.want) {
				t.Fatalf("judge() = %+v, want %d finding(s)", found, len(tc.want))
			}
			for i, f := range found {
				if f.detail != tc.want[i] || f.kind != tc.kinds[i] {
					t.Errorf("finding %d = %s/%s, want %s/%s", i, f.kind, f.detail, tc.kinds[i], tc.want[i])
				}
			}
		})
	}
}

// TestJudgeGuidance_PerAction_ReportsOnlyAKnownActionsUnknownParameter
// verifies the guidance side is judged against the action its own line names,
// and stays silent for an action the catalog does not know: the standalone
// meta tools carry no per-action schemas, and condemning them would report a
// missing lookup as a stale description.
func TestJudgeGuidance_PerAction_ReportsOnlyAKnownActionsUnknownParameter(t *testing.T) {
	perAction := newAccepted()
	perAction.add(map[string]any{"properties": map[string]any{"commit_sha": map[string]any{"type": "string"}}})
	allowed := schemas{union: perAction, byAction: map[string]accepted{"discussion_list": perAction}}

	cases := []struct {
		name      string
		mentioned guidance
		want      string
	}{
		{name: "known_action_known_parameter", mentioned: guidance{action: "discussion_list", param: "commit_sha"}},
		{name: "unknown_action", mentioned: guidance{action: "not_an_action", param: "commit_id"}},
		{name: "known_action_unknown_parameter", mentioned: guidance{action: "discussion_list", param: "commit_id"}, want: "discussion_list.commit_id"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			found := judgeGuidance("gitlab_widget", tc.mentioned, allowed)
			if tc.want == "" {
				if len(found) != 0 {
					t.Fatalf("judgeGuidance() = %+v, want no finding", found)
				}
				return
			}
			if len(found) != 1 || found[0].detail != tc.want || found[0].kind != kindGuidance {
				t.Fatalf("judgeGuidance() = %+v, want one guidance finding %q", found, tc.want)
			}
		})
	}
}

// testCatalog returns a one-group catalog whose single action accepts the
// named parameters, which is enough to drive acceptedFor and audit without
// building the real catalog.
func testCatalog(t *testing.T, toolName, actionName string, parameters ...string) *actioncatalog.Catalog {
	t.Helper()
	properties := make(map[string]any, len(parameters))
	for _, parameter := range parameters {
		properties[parameter] = map[string]any{"type": "string"}
	}
	catalog := actioncatalog.NewCatalog()
	action := actioncatalog.Action{
		Name:         actionName,
		OwnerPackage: "tools",
		Route:        toolutil.ActionRoute{InputSchema: map[string]any{"type": "object", "properties": properties}},
	}
	options := actioncatalog.GroupOptions{ToolName: toolName, OwnerPackage: "tools", SurfaceKind: actioncatalog.SurfaceKindMetaGroup}
	if err := catalog.AddAction(toolName, action, options); err != nil {
		t.Fatalf("AddAction() error: %v", err)
	}
	return catalog
}

// TestAcceptedFor_GroupOrStandalone_ReadsTheRightSchemas verifies where the
// accepted names come from on each side of the meta surface: a catalog group
// pools its routes and keeps them per action too, while a standalone tool,
// which is no group, is compared against its own input schema.
func TestAcceptedFor_GroupOrStandalone_ReadsTheRightSchemas(t *testing.T) {
	catalog := testCatalog(t, "gitlab_widget", "list", "search", "scope")

	t.Run("catalog_group", func(t *testing.T) {
		allowed := acceptedFor(&mcp.Tool{Name: "gitlab_widget"}, catalog)
		if !allowed.union.names["search"] || !allowed.union.names["scope"] {
			t.Errorf("union names = %v, want the route's parameters", allowed.union.names)
		}
		if !allowed.byAction["list"].names["search"] {
			t.Errorf("per-action names = %+v, want the route's parameters under list", allowed.byAction)
		}
	})

	t.Run("standalone_tool", func(t *testing.T) {
		tool := &mcp.Tool{Name: "gitlab_discover_project", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"remote_url": map[string]any{"type": "string"}},
		}}
		allowed := acceptedFor(tool, catalog)
		if !allowed.union.names["remote_url"] {
			t.Errorf("union names = %v, want the tool's own schema", allowed.union.names)
		}
		if len(allowed.byAction) != 0 {
			t.Errorf("byAction = %+v, want none for a tool that is no group", allowed.byAction)
		}
	})
}

// TestAudit_BothBlocks_CountsLinesAndSortsFindings verifies audit reads both
// blocks of a served description, counts every line it read, and orders the
// findings by tool, then kind, then detail.
func TestAudit_BothBlocks_CountsLinesAndSortsFindings(t *testing.T) {
	catalog := testCatalog(t, "gitlab_widget", "list", "search")
	tool := &mcp.Tool{Name: "gitlab_widget", Description: metaPreamble +
		"Parameter guidance:\n- list.query: search_term. Source: the prompt.\n\n" +
		"Widget actions.\n\n- list: search, query\n"}

	findings, lines := audit([]*mcp.Tool{tool}, catalog)
	if lines != 2 {
		t.Fatalf("lines read = %d, want the guidance line plus the enumeration line", lines)
	}
	if len(findings) != 2 {
		t.Fatalf("findings = %+v, want one per block", findings)
	}
	if findings[0].kind != kindGuidance || findings[0].detail != "list.query" {
		t.Errorf("first finding = %+v, want the guidance one, which sorts first", findings[0])
	}
	if findings[1].kind != kindParameter || findings[1].detail != "query" {
		t.Errorf("second finding = %+v, want the enumeration one", findings[1])
	}
}

// TestAudit_SortsAcrossTools verifies the tool name orders the report before
// the kind does, so a growing catalog cannot reshuffle an unchanged report.
func TestAudit_SortsAcrossTools(t *testing.T) {
	catalog := actioncatalog.NewCatalog()
	body := metaPreamble + "Widget actions.\n\n- list: query\n"
	served := []*mcp.Tool{
		{Name: "gitlab_zeta", Description: body},
		{Name: "gitlab_alpha", Description: body},
	}

	findings, _ := audit(served, catalog)
	if len(findings) != 2 || findings[0].tool != "gitlab_alpha" || findings[1].tool != "gitlab_zeta" {
		t.Fatalf("findings = %+v, want them ordered by tool name", findings)
	}
}

// TestAudit_SortsByDetailWithinAKind verifies two findings of one kind on one
// tool are ordered by what they name, which is the last tie-break and the one
// that keeps a multi-parameter line reported in a stable order.
func TestAudit_SortsByDetailWithinAKind(t *testing.T) {
	catalog := testCatalog(t, "gitlab_widget", "list", "search")
	tool := &mcp.Tool{Name: "gitlab_widget", Description: metaPreamble + "Widget actions.\n\n- list: query, filter\n"}

	findings, _ := audit([]*mcp.Tool{tool}, catalog)
	if len(findings) != 2 || findings[0].detail != "filter" || findings[1].detail != "query" {
		t.Fatalf("findings = %+v, want them ordered by the name they report", findings)
	}
}

// TestAudit_SortsByLineWhenEverythingElseTies verifies two findings that agree
// on tool, kind and detail are ordered by the line they came from: six actions
// of one tool offering one wrong parameter tie on every other part of the key,
// and without the line they printed in a different order from run to run.
func TestAudit_SortsByLineWhenEverythingElseTies(t *testing.T) {
	catalog := testCatalog(t, "gitlab_widget", "list", "search")
	tool := &mcp.Tool{Name: "gitlab_widget", Description: metaPreamble +
		"Widget actions.\n\n- search: query\n- list: query\n"}

	findings, _ := audit([]*mcp.Tool{tool}, catalog)
	if len(findings) != 2 || findings[0].line != "- list: query" || findings[1].line != "- search: query" {
		t.Fatalf("findings = %+v, want them ordered by the line each came from", findings)
	}
}

// TestReport_FindingsAndExitCodes verifies the report prints one line per
// finding plus the summary, and that only -check turns a finding into a
// non-zero exit: the plain report is meant to be readable during a fix.
func TestReport_FindingsAndExitCodes(t *testing.T) {
	found := []finding{{tool: "gitlab_widget", kind: kindParameter, detail: "confidence", line: "- list: confidence"}}

	cases := []struct {
		name     string
		findings []finding
		check    bool
		code     int
		contains string
	}{
		{name: "clean_run", contains: "7 description line(s) read, every parameter and value they offer exists"},
		{name: "findings_without_check", findings: found, contains: "7 description line(s) read, 1 disagree with the schemas"},
		{name: "findings_under_check", findings: found, check: true, code: 1, contains: "confidence"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := captureStdout(t)
			if got := report(tc.findings, 7, tc.check); got != tc.code {
				t.Errorf("report() = %d, want %d", got, tc.code)
			}
			if !strings.Contains(out.String(), tc.contains) {
				t.Errorf("stdout = %q, want it to contain %q", out.String(), tc.contains)
			}
		})
	}
}

// TestRun_ServedSurface_IsClean is the gate's own assertion: every parameter
// and enum value the served meta descriptions offer is one the actions behind
// them accept. A description that goes stale fails here rather than telling a
// model to send a parameter GitLab refuses.
func TestRun_ServedSurface_IsClean(t *testing.T) {
	out := captureStdout(t)

	if got := run(true); got != 0 {
		t.Fatalf("run(check=true) = %d, want 0\nstdout:\n%s", got, out.String())
	}
	if !strings.Contains(out.String(), "every parameter and value they offer exists") {
		t.Errorf("stdout = %q, want the all-clear summary", out.String())
	}
}
