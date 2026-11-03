// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/baseline"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/grants"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/params"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/paths"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/required"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/freshness"
)

// The reports a planted tree answers with, one per scope the baseline reads.
// Every stream is empty but params.unmodeled, which holds the two findings
// the params report lists, so a case moves the tree by editing that one list.
const (
	plantedStructs  = `{"summary": {"missing_input_fields": 0, "missing_output_fields": 0, "extra_output_fields": 0, "type_mismatches": 0}, "packages": []}`
	plantedActions  = `{"summary": {"missing_methods": 0}, "services": []}`
	plantedPaths    = `{"summary": {"unpublished_fields": 0, "typed_nested_unpublished_fields": 0, "typed_nested_unsurfaced_fields": 0, "always_sent_but_optional": 0}, "shapes": {"ran": true, "typed": {"ran": true}}, "always_sent": {"ran": true}, "identifier_values": {"ran": true}}`
	plantedRequired = `{"summary": {"fields_unplaced": 0, "fields_unjudged": 0}}`
	plantedGrants   = `{"summary": {"inventory_rest_rows": 0, "inventory_rest_rows_derived": 0, "inventory_graphql_rows": 0, "inventory_graphql_rows_derived": 0, "derived_rest_requests": 0, "derived_rest_requests_recorded": 0, "derived_graphql_requests": 0, "derived_graphql_requests_recorded": 0}, "inventory_check": {}}`
	plantedEnums    = `{"summary": {"unsurfaced_output_fields": 0}, "packages": []}`
)

// plantedParams is a params report whose unmodeled findings are the routes
// and params given, as "route | param" keys spell them.
func plantedParams(t *testing.T, findings ...[2]string) string {
	t.Helper()
	listed := make([]map[string]string, 0, len(findings))
	for _, finding := range findings {
		listed = append(listed, map[string]string{"route": finding[0], "param": finding[1], "class": "unmodeled"})
	}
	content, err := json.Marshal(map[string]any{
		"summary":  map[string]int{"unmodeled": len(findings), "not_offered": 0, "offered_unrecorded": 0, "unjudged": 0},
		"findings": listed,
	})
	if err != nil {
		t.Fatalf("encode the planted params report: %v", err)
	}
	return string(content)
}

// baselineCalls records what each scope seam was asked for.
type baselineCalls struct {
	roots          []string
	structsGaps    []bool
	actionsGaps    []bool
	enumsGaps      []bool
	paramsOptions  []params.Options
	pathsOptions   []paths.Options
	requiredOpts   []required.Options
	grantsOptions  []grants.Options
	scopesAnswered int
}

// plantBaselineTree points the root at a fresh directory, writes the
// committed baseline there when committed is not nil, and replaces the seven
// scope seams with ones that answer the planted reports, params answering
// paramsReport. It returns the root and what the seams were asked.
func plantBaselineTree(t *testing.T, committed *baseline.Baseline, paramsReport string) (string, *baselineCalls) {
	t.Helper()
	originalRoot := repositoryRoot
	originalStructs, originalActions, originalEnums := structsRun, actionsRun, enumsRun
	originalParams, originalPaths, originalRequired, originalGrants := paramsRun, pathsRun, requiredRun, grantsRun
	t.Cleanup(func() {
		repositoryRoot = originalRoot
		structsRun, actionsRun, enumsRun = originalStructs, originalActions, originalEnums
		paramsRun, pathsRun, requiredRun, grantsRun = originalParams, originalPaths, originalRequired, originalGrants
	})
	root := t.TempDir()
	if committed != nil {
		writeTree(t, root, map[string]string{baseline.File: string(committed.Marshal())})
	}
	calls := &baselineCalls{}
	repositoryRoot = func(string) (string, error) { return root, nil }
	answer := func(scopeRoot, content string) []byte {
		calls.roots = append(calls.roots, scopeRoot)
		calls.scopesAnswered++
		return []byte(content)
	}
	structsRun = func(scopeRoot string, gapsOnly bool) ([]byte, error) {
		calls.structsGaps = append(calls.structsGaps, gapsOnly)
		return answer(scopeRoot, plantedStructs), nil
	}
	actionsRun = func(scopeRoot string, gapsOnly bool) ([]byte, error) {
		calls.actionsGaps = append(calls.actionsGaps, gapsOnly)
		return answer(scopeRoot, plantedActions), nil
	}
	paramsRun = func(scopeRoot string, opts params.Options) ([]byte, bool, error) {
		calls.paramsOptions = append(calls.paramsOptions, opts)
		return answer(scopeRoot, paramsReport), false, nil
	}
	pathsRun = func(scopeRoot string, opts paths.Options) ([]byte, bool, error) {
		calls.pathsOptions = append(calls.pathsOptions, opts)
		return answer(scopeRoot, plantedPaths), false, nil
	}
	requiredRun = func(scopeRoot string, opts required.Options) ([]byte, bool, error) {
		calls.requiredOpts = append(calls.requiredOpts, opts)
		return answer(scopeRoot, plantedRequired), false, nil
	}
	grantsRun = func(scopeRoot string, opts grants.Options) ([]byte, bool, error) {
		calls.grantsOptions = append(calls.grantsOptions, opts)
		return answer(scopeRoot, plantedGrants), false, nil
	}
	enumsRun = func(scopeRoot string, gapsOnly bool) ([]byte, bool, error) {
		calls.enumsGaps = append(calls.enumsGaps, gapsOnly)
		return answer(scopeRoot, plantedEnums), false, nil
	}
	return root, calls
}

// plantedBaseline is the baseline a tree whose params.unmodeled stream holds
// keys, and every other stream nothing, writes.
func plantedBaseline(keys ...string) baseline.Baseline {
	streams := map[string][]string{}
	for _, name := range []string{
		baseline.StructsMissingInputs, baseline.StructsMissingOutputs, baseline.StructsExtraOutputs, baseline.StructsTypeMismatches,
		baseline.ActionsMissingMethods, baseline.ParamsNotOffered, baseline.ParamsOfferedUnrecorded, baseline.ParamsUnjudged,
		baseline.PathsUnpublished, baseline.PathsNestedUnpublished, baseline.PathsNestedUnsurfaced, baseline.PathsAlwaysSent,
		baseline.PathsIdentifierLeads, baseline.RequiredUnplaced, baseline.RequiredUnjudged, baseline.GrantsRowsNotDerived,
		baseline.GrantsNotRecorded, baseline.EnumsUnsurfaced,
	} {
		streams[name] = []string{}
	}
	streams[baseline.ParamsUnmodeled] = append([]string{}, keys...)
	return baseline.Baseline{SchemaVersion: baseline.SchemaVersion, Streams: streams}
}

// assertScopesAskedForTheirWholeLists holds the seven seams to having been
// asked once each, at the root, in the mode whose lists hold every finding:
// structs, required and enums without -gaps-only, and actions, params, paths
// and grants with it and with nothing else switched on.
func assertScopesAskedForTheirWholeLists(t *testing.T, root string, calls *baselineCalls) {
	t.Helper()
	if calls.scopesAnswered != 7 {
		t.Fatalf("the scopes were asked %d times, want once each of 7", calls.scopesAnswered)
	}
	for _, scopeRoot := range calls.roots {
		if scopeRoot != root {
			t.Errorf("a scope was run at %q, want the repository root %q", scopeRoot, root)
		}
	}
	for _, asked := range []struct {
		name      string
		got, want any
	}{
		{name: "structs -gaps-only", got: calls.structsGaps, want: []bool{false}},
		{name: "actions -gaps-only", got: calls.actionsGaps, want: []bool{true}},
		{name: "enums -gaps-only", got: calls.enumsGaps, want: []bool{false}},
		{name: "params options", got: calls.paramsOptions, want: []params.Options{{GapsOnly: true}}},
		{name: "paths options", got: calls.pathsOptions, want: []paths.Options{{GapsOnly: true}}},
		{name: "required options", got: calls.requiredOpts, want: []required.Options{{}}},
		{name: "grants options", got: calls.grantsOptions, want: []grants.Options{{GapsOnly: true}}},
	} {
		t.Run(asked.name, func(t *testing.T) {
			if !reflect.DeepEqual(asked.got, asked.want) {
				t.Errorf("%s = %#v, want %#v", asked.name, asked.got, asked.want)
			}
		})
	}
}

// TestMain_WriteBaseline_WritesTheTreesFindingsByStream runs -write-baseline
// through main on a planted tree and holds it to writing what the seven
// reports hold, as baseline.Build reads them, to -output, having asked each
// scope once for its whole lists.
func TestMain_WriteBaseline_WritesTheTreesFindingsByStream(t *testing.T) {
	root, calls := plantBaselineTree(t, nil, plantedParams(t, [2]string{"GET /b", "y"}, [2]string{"GET /a", "x"}))
	messages := captureFatal(t)
	output := filepath.Join(t.TempDir(), "baseline.json")
	resetFlags(t, "-write-baseline", "-output", output)

	main()

	if len(*messages) != 0 {
		t.Fatalf("main reported %v, want the baseline written", *messages)
	}
	written, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read the written baseline: %v", err)
	}
	if want := plantedBaseline("GET /a | x", "GET /b | y").Marshal(); string(written) != string(want) {
		t.Errorf("-write-baseline wrote\n%s\nwant\n%s", written, want)
	}
	assertScopesAskedForTheirWholeLists(t, root, calls)
}

// TestMain_CheckBaseline_HoldsTheTreeToTheCommittedBaseline is the detection
// the baseline exists for, run through main. A tree equal to the committed
// baseline passes. A tree with one finding more fails naming the stream and
// the key, as a client-go bump or a new handler that adds one would; a
// baseline key no finding answers fails as stale, so the record cannot
// outlive what it holds. Either way the comparison is written first.
func TestMain_CheckBaseline_HoldsTheTreeToTheCommittedBaseline(t *testing.T) {
	committed := plantedBaseline("GET /a | x", "GET /b | y")
	cases := []struct {
		name      string
		tree      [][2]string
		wantFatal string
		wantNew   []baseline.Difference
		wantStale []baseline.Difference
	}{
		{
			name: "equal",
			tree: [][2]string{{"GET /a", "x"}, {"GET /b", "y"}},
		},
		{
			name:      "one_finding_more",
			tree:      [][2]string{{"GET /a", "x"}, {"GET /b", "y"}, {"GET /c", "z"}},
			wantFatal: "audit_1to1: 1 findings the baseline does not hold (params.unmodeled: GET /c | z); when the change means them, make gen-audit-1to1-baseline records the tree (see report)",
			wantNew:   []baseline.Difference{{Stream: baseline.ParamsUnmodeled, Key: "GET /c | z"}},
		},
		{
			name:      "a_key_no_finding_answers",
			tree:      [][2]string{{"GET /b", "y"}},
			wantFatal: "audit_1to1: 1 keys of the baseline no finding answers (params.unmodeled: GET /a | x)",
			wantStale: []baseline.Difference{{Stream: baseline.ParamsUnmodeled, Key: "GET /a | x"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, calls := plantBaselineTree(t, &committed, plantedParams(t, tc.tree...))
			messages := captureFatal(t)
			output := filepath.Join(t.TempDir(), "comparison.json")
			resetFlags(t, "-check-baseline", "-output", output)

			main()

			switch {
			case tc.wantFatal == "" && len(*messages) != 0:
				t.Errorf("main reported %v, want the check to pass", *messages)
			case tc.wantFatal != "" && (len(*messages) != 1 || !strings.HasPrefix((*messages)[0], tc.wantFatal)):
				t.Errorf("main reported %v, want one fatal message starting %q", *messages, tc.wantFatal)
			}
			var comparison baseline.Comparison
			content, err := os.ReadFile(output)
			if err != nil {
				t.Fatalf("read the comparison: %v", err)
			}
			if err = json.Unmarshal(content, &comparison); err != nil {
				t.Fatalf("decode the comparison %s: %v", content, err)
			}
			if !reflect.DeepEqual(comparison.New, orNone(tc.wantNew)) || !reflect.DeepEqual(comparison.Stale, orNone(tc.wantStale)) {
				t.Errorf("comparison new = %v, stale = %v; want %v and %v", comparison.New, comparison.Stale, tc.wantNew, tc.wantStale)
			}
			assertScopesAskedForTheirWholeLists(t, root, calls)
		})
	}
}

// orNone is differences, or the empty list a comparison writes for none.
func orNone(differences []baseline.Difference) []baseline.Difference {
	if differences == nil {
		return []baseline.Difference{}
	}
	return differences
}

// baselineFailure is one way a baseline mode stops before a verdict.
type baselineFailure struct {
	name string
	// arrange replaces the seam that fails, after the tree is planted.
	arrange func()
	// output names an output that cannot be written; nil writes to a fresh
	// directory.
	output func(t *testing.T) string
	want   string
	// noScope says the mode stops before asking any scope.
	noScope bool
}

// baselineFailures are the ways both modes stop before a verdict, each seam
// failing with boom: a root that cannot be found, each of the seven scopes in
// turn, reports whose lists and counts disagree, and an output that cannot be
// written.
func baselineFailures(boom error) []baselineFailure {
	failures := []baselineFailure{
		{
			name:    "no_repository_root",
			arrange: func() { repositoryRoot = func(string) (string, error) { return "", boom } },
			want:    "find repository root: boom",
			noScope: true,
		},
		{
			name: "reports_whose_lists_and_counts_disagree",
			arrange: func() {
				paramsRun = func(string, params.Options) ([]byte, bool, error) {
					return []byte(`{"summary": {"unmodeled": 1, "not_offered": 0, "offered_unrecorded": 0, "unjudged": 0}, "findings": []}`), true, nil
				}
			},
			want: "the params report lists 0 findings of params.unmodeled where its summary counts 1",
		},
		{
			name: "an_output_it_cannot_write",
			output: func(t *testing.T) string {
				t.Helper()
				file := filepath.Join(t.TempDir(), "file")
				writeTree(t, filepath.Dir(file), map[string]string{"file": ""})
				return filepath.Join(file, "out.json")
			},
			want: "write output: ",
		},
	}
	for scope, fail := range map[string]func(){
		"structs":  func() { structsRun = func(string, bool) ([]byte, error) { return nil, boom } },
		"actions":  func() { actionsRun = func(string, bool) ([]byte, error) { return nil, boom } },
		"params":   func() { paramsRun = func(string, params.Options) ([]byte, bool, error) { return nil, false, boom } },
		"paths":    func() { pathsRun = func(string, paths.Options) ([]byte, bool, error) { return nil, false, boom } },
		"required": func() { requiredRun = func(string, required.Options) ([]byte, bool, error) { return nil, false, boom } },
		"grants":   func() { grantsRun = func(string, grants.Options) ([]byte, bool, error) { return nil, false, boom } },
		"enums":    func() { enumsRun = func(string, bool) ([]byte, bool, error) { return nil, false, boom } },
	} {
		failures = append(failures, baselineFailure{name: "the_" + scope + "_scope_fails", arrange: fail, want: scope + " report: boom"})
	}
	return failures
}

// TestBaselineModes_AFailure_IsNamedAndWritesNothing drives both baseline
// modes through every way they stop before a verdict, and holds each to
// returning what stopped it, to writing nothing, and, when the root cannot be
// found, to asking no scope.
func TestBaselineModes_AFailure_IsNamedAndWritesNothing(t *testing.T) {
	committed := plantedBaseline()
	modes := map[string]func(output string) error{"write": writeBaselineFile, "check": checkBaselineFile}
	for modeName, mode := range modes {
		for _, tc := range baselineFailures(errors.New("boom")) {
			t.Run(modeName+"/"+tc.name, func(t *testing.T) {
				_, calls := plantBaselineTree(t, &committed, plantedParams(t))
				if tc.arrange != nil {
					tc.arrange()
				}
				output := filepath.Join(t.TempDir(), "out.json")
				if tc.output != nil {
					output = tc.output(t)
				}
				err := mode(output)
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("%s error = %v, want it to contain %q", modeName, err, tc.want)
				}
				if _, statErr := os.Stat(output); statErr == nil {
					t.Errorf("%s wrote %s beside its error", modeName, output)
				}
				if tc.noScope && calls.scopesAnswered != 0 {
					t.Errorf("%s asked %d scopes before stopping, want none", modeName, calls.scopesAnswered)
				}
			})
		}
	}
}

// TestCheckBaselineFile_NoCommittedBaseline_FailsBeforeAnyScope holds the
// check to reading the committed baseline first: with none there it fails
// naming the file it could not read, having run no scope and written nothing.
func TestCheckBaselineFile_NoCommittedBaseline_FailsBeforeAnyScope(t *testing.T) {
	_, calls := plantBaselineTree(t, nil, plantedParams(t))
	output := filepath.Join(t.TempDir(), "out.json")
	err := checkBaselineFile(output)
	if err == nil || !strings.Contains(err.Error(), "read the baseline") || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("checkBaselineFile error = %v, want the missing baseline named", err)
	}
	if calls.scopesAnswered != 0 {
		t.Errorf("the check ran %d scopes with no baseline to compare with, want none", calls.scopesAnswered)
	}
	if _, statErr := os.Stat(output); statErr == nil {
		t.Errorf("the check wrote %s with no baseline to compare with", output)
	}
}

// TestCheckBaselineFile_TheTree_MatchesTheCommittedBaseline is the check on
// the repository's own tree: every report-only finding of the seven scopes is
// a key of docs/development/audit-1to1-baseline.json, and every key of it is
// a finding. The baseline is a committed artifact read from the committed
// request records, so the comparison waits for the top of a stack.
func TestCheckBaselineFile_TheTree_MatchesTheCommittedBaseline(t *testing.T) {
	freshness.SkipIfDeferred(t)
	output := filepath.Join(t.TempDir(), "comparison.json")
	if err := checkBaselineFile(output); err != nil {
		t.Fatalf("%v\nWhen the change means it, make gen-audit-1to1-baseline records the tree, and the diff of the file is what it adds.", err)
	}
}

// TestMain_BaselineModes_AFailure_IsFatal holds main to ending through the
// fatal path when either baseline mode fails, with the mode's own error.
func TestMain_BaselineModes_AFailure_IsFatal(t *testing.T) {
	for _, flagName := range []string{"-write-baseline", "-check-baseline"} {
		t.Run(flagName, func(t *testing.T) {
			plantBaselineTree(t, nil, plantedParams(t))
			repositoryRoot = func(string) (string, error) { return "", errors.New("boom") }
			messages := captureFatal(t)
			resetFlags(t, flagName, "-output", filepath.Join(t.TempDir(), "out.json"))

			main()

			if len(*messages) != 1 || (*messages)[0] != "find repository root: boom" {
				t.Errorf("main reported %v, want the mode's error as the one fatal message", *messages)
			}
		})
	}
}
