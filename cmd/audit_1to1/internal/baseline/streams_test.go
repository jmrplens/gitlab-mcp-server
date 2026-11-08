// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package baseline

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// fixtureReports are the seven reports, as maps a case can edit before they
// are encoded, each listing what its summary counts. Every stream holds at
// least one finding, the structs input stream excepted when a case empties
// it, and every one is listed out of order, so a key set that is not sorted
// shows.
func fixtureReports() map[string]map[string]any {
	return map[string]map[string]any{
		"structs": {
			"summary": map[string]any{"missing_input_fields": 1, "missing_output_fields": 2, "extra_output_fields": 1, "type_mismatches": 2},
			"packages": []any{
				map[string]any{"package": "beta", "gaps": []any{map[string]any{
					"kind": "output", "mcp_type": "Output", "sdk_type": "v3.Beta",
					"missing_fields":  []any{map[string]any{"tag": "name", "sdk_type": "string"}, map[string]any{"tag": "id", "sdk_type": "int64"}},
					"extra_fields":    []any{map[string]any{"tag": "web_url", "mcp_type": "string"}},
					"type_mismatches": []any{map[string]any{"tag": "state", "mcp_type": "string", "sdk_type": "*v3.State"}},
				}}},
				map[string]any{"package": "alpha", "gaps": []any{map[string]any{
					"kind": "input", "mcp_type": "ListInput", "sdk_type": "v3.ListAlphaOptions", "via": "caller",
					"missing_fields":  []any{map[string]any{"tag": "sort", "sdk_type": "*string"}},
					"type_mismatches": []any{map[string]any{"tag": "order_by", "mcp_type": "string", "sdk_type": "*string"}},
				}}},
				map[string]any{"package": "clean", "gaps": nil},
			},
		},
		"actions": {
			"summary": map[string]any{"missing_methods": 3},
			"services": []any{
				map[string]any{"service": "BetaServiceInterface", "missing_methods": []any{"Two", "One"}},
				map[string]any{"service": "AlphaServiceInterface", "missing_methods": []any{"Three"}},
				map[string]any{"service": "CleanServiceInterface"},
			},
		},
		"params": {
			"summary": map[string]any{"unmodeled": 2, "not_offered": 1, "offered_unrecorded": 1, "unjudged": 1, "declared": 9},
			"findings": []any{
				map[string]any{"route": "GET /b", "param": "x", "class": "unmodeled"},
				map[string]any{"route": "GET /a", "param": "y", "class": "unmodeled"},
				map[string]any{"route": "PUT /a", "param": "z", "class": "not_offered"},
				map[string]any{"route": "POST /a", "param": "w", "class": "offered_unrecorded"},
				map[string]any{"route": "POST /f", "param": "endpoint", "class": "unjudged"},
			},
		},
		"paths": {
			"summary": map[string]any{"unpublished_fields": 1, "typed_nested_unpublished_fields": 1, "typed_nested_unsurfaced_fields": 2, "always_sent_but_optional": 1, "actions_observed_grain": "package"},
			"shapes": map[string]any{
				"ran":         true,
				"unpublished": []any{map[string]any{"grain": "package", "package": "internal/tools/alpha", "type": "ListOutput", "field": "items"}},
				"typed": map[string]any{
					"ran":                true,
					"nested_unpublished": []any{map[string]any{"package": "internal/tools/alpha", "type": "UserOutput", "under": "user", "field": "created_at", "sdk_type": "Alpha"}},
					// One finding listed twice, as two parents holding the
					// same nested type under the same key list it.
					"nested_unsurfaced": []any{
						map[string]any{"package": "internal/tools/beta", "type": "toolutil.UserOutput", "under": "author", "field": "locked", "sdk_type": "Beta"},
						map[string]any{"package": "internal/tools/beta", "type": "toolutil.UserOutput", "under": "author", "field": "locked", "sdk_type": "Beta"},
					},
				},
			},
			"always_sent": map[string]any{"ran": true, "optional_but_always_sent": []any{
				map[string]any{"package": "internal/tools/gamma", "method": "POST", "path": "/x/:id", "option_type": "CreateXOptions", "field": "Kind"},
			}},
			"identifier_values": map[string]any{"ran": true, "leads": []any{
				map[string]any{"package": "internal/tools/delta", "method": "GET", "path": "/y/:y_id", "placeholder": ":y_id"},
			}},
		},
		"required": {
			"summary": map[string]any{"fields_unplaced": 3, "fields_unjudged": 1},
			"unplaced": []any{
				map[string]any{"action": "beta.get", "fields": []any{"b", "a"}},
				map[string]any{"action": "alpha.list", "fields": []any{"page"}},
			},
			"unjudged": []any{map[string]any{"action": "gamma.set", "fields": []any{"value"}}},
		},
		"grants": {
			// Every term differs from zero, so a sign the difference is taken
			// with that flips moves the count off the three and two listed.
			"summary": map[string]any{
				"inventory_rest_rows": 10, "inventory_rest_rows_derived": 8, "inventory_graphql_rows": 5, "inventory_graphql_rows_derived": 4,
				"derived_rest_requests": 7, "derived_rest_requests_recorded": 6, "derived_graphql_requests": 3, "derived_graphql_requests_recorded": 2,
			},
			"inventory_check": map[string]any{
				"rows_not_derived": []any{
					map[string]any{"package": "internal/tools/beta", "request": "GET /b", "why": "no action the package owns derives it"},
					map[string]any{"package": "internal/tools/alpha", "request": "GET /a", "why": "no route of the live record meets it"},
					map[string]any{"package": "internal/tools/alpha", "request": "query alphas", "why": "no action the package owns derives it"},
				},
				"derived_not_recorded": []any{
					map[string]any{"package": "internal/tools/beta", "request": "PUT /b", "actions": []any{"beta.update"}},
					map[string]any{"package": "internal/tools/alpha", "request": "mutation alphaCreate", "actions": []any{"alpha.create"}},
				},
			},
		},
		"enums": {
			"summary": map[string]any{"unsurfaced_output_fields": 1},
			"packages": []any{map[string]any{"package": "alpha", "findings": []any{
				map[string]any{"action": "alpha.get", "kind": "output", "mcp_type": "Output", "field": "state", "source": "none"},
				// An output whose schema names its values, and an input that
				// names none, are not what the stream counts.
				map[string]any{"action": "alpha.get", "kind": "output", "mcp_type": "Output", "field": "visibility", "source": "enum"},
				map[string]any{"action": "alpha.list", "kind": "input", "mcp_type": "ListInput", "field": "state", "source": "none"},
			}}},
		},
	}
}

// encodeReports encodes the fixture maps into the reports Build reads.
func encodeReports(t *testing.T, fixtures map[string]map[string]any) Reports {
	t.Helper()
	encoded := map[string][]byte{}
	for scope, report := range fixtures {
		content, err := json.Marshal(report)
		if err != nil {
			t.Fatalf("encode the %s fixture: %v", scope, err)
		}
		encoded[scope] = content
	}
	return Reports{
		Structs: encoded["structs"], Actions: encoded["actions"], Params: encoded["params"], Paths: encoded["paths"],
		Required: encoded["required"], Grants: encoded["grants"], Enums: encoded["enums"],
	}
}

// summaryOf is a fixture's summary, for a case to edit.
func summaryOf(fixtures map[string]map[string]any, scope string) map[string]any {
	return fixtures[scope]["summary"].(map[string]any)
}

// TestBuild_EveryStream_ReadsItsKeysSortedAndOnce reads all nineteen streams
// out of reports listing each one's findings out of order, and holds each to
// the keys it must spell: the fields that name a finding, joined, sorted,
// and a finding listed twice written once. What only describes a finding
// (a why, a via, the actions behind a request, a field's own type when it is
// missing) is in no key.
func TestBuild_EveryStream_ReadsItsKeysSortedAndOnce(t *testing.T) {
	got, err := Build(encodeReports(t, fixtureReports()))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	want := map[string][]string{
		StructsMissingInputs:  {"alpha | ListInput | v3.ListAlphaOptions | sort"},
		StructsMissingOutputs: {"beta | Output | v3.Beta | id", "beta | Output | v3.Beta | name"},
		StructsExtraOutputs:   {"beta | Output | v3.Beta | web_url"},
		StructsTypeMismatches: {
			"alpha | input | ListInput | v3.ListAlphaOptions | order_by | string | *string",
			"beta | output | Output | v3.Beta | state | string | *v3.State",
		},
		ActionsMissingMethods:   {"AlphaServiceInterface | Three", "BetaServiceInterface | One", "BetaServiceInterface | Two"},
		ParamsUnmodeled:         {"GET /a | y", "GET /b | x"},
		ParamsNotOffered:        {"PUT /a | z"},
		ParamsOfferedUnrecorded: {"POST /a | w"},
		ParamsUnjudged:          {"POST /f | endpoint"},
		PathsUnpublished:        {"internal/tools/alpha | ListOutput | items"},
		PathsNestedUnpublished:  {"internal/tools/alpha | UserOutput | user | created_at | Alpha"},
		PathsNestedUnsurfaced:   {"internal/tools/beta | toolutil.UserOutput | author | locked | Beta"},
		PathsAlwaysSent:         {"internal/tools/gamma | POST /x/:id | CreateXOptions | Kind"},
		PathsIdentifierLeads:    {"internal/tools/delta | GET /y/:y_id | :y_id"},
		RequiredUnplaced:        {"alpha.list | page", "beta.get | a", "beta.get | b"},
		RequiredUnjudged:        {"gamma.set | value"},
		GrantsRowsNotDerived:    {"internal/tools/alpha | GET /a", "internal/tools/alpha | query alphas", "internal/tools/beta | GET /b"},
		GrantsNotRecorded:       {"internal/tools/alpha | mutation alphaCreate", "internal/tools/beta | PUT /b"},
		EnumsUnsurfaced:         {"alpha.get | Output | state"},
	}
	if got.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", got.SchemaVersion, SchemaVersion)
	}
	if len(got.Streams) != len(want) {
		t.Errorf("Build read %d streams, want %d: %v", len(got.Streams), len(want), got.Streams)
	}
	for name, keys := range want {
		t.Run(name, func(t *testing.T) {
			if !reflect.DeepEqual(got.Streams[name], keys) {
				t.Errorf("Streams[%q] =\n%q\nwant\n%q", name, got.Streams[name], keys)
			}
		})
	}
}

// TestBuild_AStreamWithNoFinding_IsAnEmptyListAndNotNone holds a stream the
// tree has no finding in to an empty list, which the file writes as [] and a
// reader sees as a stream with nothing in it rather than a stream missing.
func TestBuild_AStreamWithNoFinding_IsAnEmptyListAndNotNone(t *testing.T) {
	fixtures := fixtureReports()
	fixtures["required"] = map[string]any{"summary": map[string]any{"fields_unplaced": 0, "fields_unjudged": 0}}
	got, err := Build(encodeReports(t, fixtures))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, name := range []string{RequiredUnplaced, RequiredUnjudged} {
		t.Run(name, func(t *testing.T) {
			if keys, ok := got.Streams[name]; !ok || keys == nil || len(keys) != 0 {
				t.Errorf("Streams[%q] = %#v (present %t), want an empty list", name, keys, ok)
			}
		})
	}
	if text := string(got.Marshal()); !strings.Contains(text, `"`+RequiredUnplaced+`": []`) {
		t.Errorf("the marshaled baseline writes the empty stream as something else than []:\n%s", text)
	}
}

// TestBuild_AListShorterOrLongerThanItsCounter_IsAnError moves each counter a
// stream is held to off what its list holds, one at a time, and holds Build
// to refusing with the scope, the stream and both figures: the list a
// renamed field or -gaps-only leaves empty or short would otherwise become a
// baseline that holds less than the tree. The grants counts are differences,
// so moving any of their terms moves them.
func TestBuild_AListShorterOrLongerThanItsCounter_IsAnError(t *testing.T) {
	cases := []struct {
		scope, counter, stream string
		by                     int
	}{
		{scope: "structs", counter: "missing_input_fields", stream: StructsMissingInputs, by: 1},
		{scope: "structs", counter: "missing_output_fields", stream: StructsMissingOutputs, by: 1},
		{scope: "structs", counter: "extra_output_fields", stream: StructsExtraOutputs, by: -1},
		{scope: "structs", counter: "type_mismatches", stream: StructsTypeMismatches, by: 1},
		{scope: "actions", counter: "missing_methods", stream: ActionsMissingMethods, by: 1},
		{scope: "params", counter: "unmodeled", stream: ParamsUnmodeled, by: 1},
		{scope: "params", counter: "not_offered", stream: ParamsNotOffered, by: 1},
		{scope: "params", counter: "offered_unrecorded", stream: ParamsOfferedUnrecorded, by: 1},
		{scope: "params", counter: "unjudged", stream: ParamsUnjudged, by: 1},
		{scope: "paths", counter: "unpublished_fields", stream: PathsUnpublished, by: 1},
		{scope: "paths", counter: "typed_nested_unpublished_fields", stream: PathsNestedUnpublished, by: 1},
		{scope: "paths", counter: "typed_nested_unsurfaced_fields", stream: PathsNestedUnsurfaced, by: 1},
		{scope: "paths", counter: "always_sent_but_optional", stream: PathsAlwaysSent, by: 1},
		{scope: "required", counter: "fields_unplaced", stream: RequiredUnplaced, by: 1},
		{scope: "required", counter: "fields_unjudged", stream: RequiredUnjudged, by: 1},
		{scope: "grants", counter: "inventory_rest_rows", stream: GrantsRowsNotDerived, by: 1},
		{scope: "grants", counter: "inventory_rest_rows_derived", stream: GrantsRowsNotDerived, by: 1},
		{scope: "grants", counter: "inventory_graphql_rows", stream: GrantsRowsNotDerived, by: 1},
		{scope: "grants", counter: "inventory_graphql_rows_derived", stream: GrantsRowsNotDerived, by: 1},
		{scope: "grants", counter: "derived_rest_requests", stream: GrantsNotRecorded, by: 1},
		{scope: "grants", counter: "derived_rest_requests_recorded", stream: GrantsNotRecorded, by: 1},
		{scope: "grants", counter: "derived_graphql_requests", stream: GrantsNotRecorded, by: 1},
		{scope: "grants", counter: "derived_graphql_requests_recorded", stream: GrantsNotRecorded, by: 1},
		{scope: "enums", counter: "unsurfaced_output_fields", stream: EnumsUnsurfaced, by: 1},
	}
	for _, tc := range cases {
		t.Run(tc.scope+"/"+tc.counter, func(t *testing.T) {
			fixtures := fixtureReports()
			summary := summaryOf(fixtures, tc.scope)
			summary[tc.counter] = summary[tc.counter].(int) + tc.by
			_, err := Build(encodeReports(t, fixtures))
			if err == nil {
				t.Fatalf("Build accepted a %s report whose %s is off by %d", tc.scope, tc.counter, tc.by)
			}
			for _, part := range []string{"the " + tc.scope + " report lists", "findings of " + tc.stream + " where its summary counts"} {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("Build error %q does not say %q", err, part)
				}
			}
		})
	}
}

// TestBuild_AReportItCannotRead_IsAnErrorNamingTheScope feeds each reader a
// report it cannot read, in each way a report can be unreadable: not JSON, a
// list where a list of findings belongs, a counter its summary does not
// carry, and a counter that is not a count. Every one is an error that names
// the scope, and none is a stream.
func TestBuild_AReportItCannotRead_IsAnErrorNamingTheScope(t *testing.T) {
	lists := map[string]string{
		"structs": "packages", "actions": "services", "params": "findings", "paths": "shapes",
		"required": "unplaced", "grants": "inventory_check", "enums": "packages",
	}
	counters := map[string]string{
		"structs": "type_mismatches", "actions": "missing_methods", "params": "unjudged", "paths": "always_sent_but_optional",
		"required": "fields_unjudged", "grants": "derived_graphql_requests_recorded", "enums": "unsurfaced_output_fields",
	}
	for scope, list := range lists {
		cases := []struct {
			name    string
			edit    func(fixtures map[string]map[string]any) []byte
			wantErr string
		}{
			{
				name:    "not_json",
				edit:    func(map[string]map[string]any) []byte { return []byte("{") },
				wantErr: "unexpected end of JSON input",
			},
			{
				name: "summary_not_an_object",
				edit: func(fixtures map[string]map[string]any) []byte {
					fixtures[scope]["summary"] = 7
					return nil
				},
				wantErr: "cannot unmarshal number",
			},
			{
				name: "list_of_another_type",
				edit: func(fixtures map[string]map[string]any) []byte {
					fixtures[scope][list] = 7
					return nil
				},
				wantErr: "cannot unmarshal number",
			},
			{
				name: "counter_missing",
				edit: func(fixtures map[string]map[string]any) []byte {
					delete(summaryOf(fixtures, scope), counters[scope])
					return nil
				},
				wantErr: "its summary carries no " + counters[scope],
			},
			{
				name: "counter_not_a_count",
				edit: func(fixtures map[string]map[string]any) []byte {
					summaryOf(fixtures, scope)[counters[scope]] = "many"
					return nil
				},
				wantErr: "its summary's " + counters[scope] + " is not a count",
			},
		}
		for _, tc := range cases {
			t.Run(scope+"/"+tc.name, func(t *testing.T) {
				fixtures := fixtureReports()
				raw := tc.edit(fixtures)
				reports := encodeReports(t, fixtures)
				if raw != nil {
					reports = replaced(reports, scope, raw)
				}
				got, err := Build(reports)
				if err == nil || !strings.Contains(err.Error(), "the "+scope+" report: ") || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Build error = %v, want one naming the %s report and saying %q", err, scope, tc.wantErr)
				}
				if got.Streams != nil {
					t.Errorf("Build returned streams beside its error: %v", got.Streams)
				}
			})
		}
	}
}

// replaced returns reports with scope's content replaced.
func replaced(reports Reports, scope string, content []byte) Reports {
	switch scope {
	case "structs":
		reports.Structs = content
	case "actions":
		reports.Actions = content
	case "params":
		reports.Params = content
	case "paths":
		reports.Paths = content
	case "required":
		reports.Required = content
	case "grants":
		reports.Grants = content
	default:
		reports.Enums = content
	}
	return reports
}

// TestBuild_AParamsClassNoStreamHolds_IsAnError holds the params reader to
// refusing a finding whose class is none of the four, since its findings
// would be in no stream, and to naming the class.
func TestBuild_AParamsClassNoStreamHolds_IsAnError(t *testing.T) {
	fixtures := fixtureReports()
	fixtures["params"]["findings"] = append(fixtures["params"]["findings"].([]any),
		map[string]any{"route": "GET /c", "param": "q", "class": "renamed_class"})
	_, err := Build(encodeReports(t, fixtures))
	if err == nil || !strings.Contains(err.Error(), `a finding of class "renamed_class", which no stream holds`) {
		t.Fatalf("Build error = %v, want the class no stream holds named", err)
	}
}

// TestBuild_APathsSectionThatDidNotRun_IsAnError turns each of the four paths
// sections the streams come from off, one at a time. A section that did not
// run lists nothing and counts nothing, which agree, so only its own flag
// tells it from a section with no finding.
func TestBuild_APathsSectionThatDidNotRun_IsAnError(t *testing.T) {
	cases := map[string]func(report map[string]any){
		"shapes": func(report map[string]any) { report["shapes"].(map[string]any)["ran"] = false },
		"shapes.typed": func(report map[string]any) {
			report["shapes"].(map[string]any)["typed"].(map[string]any)["ran"] = false
		},
		"always_sent":       func(report map[string]any) { report["always_sent"].(map[string]any)["ran"] = false },
		"identifier_values": func(report map[string]any) { report["identifier_values"].(map[string]any)["ran"] = false },
	}
	for section, turnOff := range cases {
		t.Run(section, func(t *testing.T) {
			fixtures := fixtureReports()
			turnOff(fixtures["paths"])
			_, err := Build(encodeReports(t, fixtures))
			if err == nil || !strings.Contains(err.Error(), "its "+section+" section did not run") {
				t.Fatalf("Build error = %v, want the %s section named as not run", err, section)
			}
		})
	}
}
