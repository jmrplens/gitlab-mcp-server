package actioncompat

import (
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestNormalizeParamsWithExplanation_CompatibilityBranches verifies dynamic parameter compatibility rules across legacy action spellings.
func TestNormalizeParamsWithExplanation_CompatibilityBranches(t *testing.T) {
	testCases := []struct {
		name             string
		actionID         string
		params           map[string]any
		schemaProperties []string
		wantParams       map[string]any
		wantAliases      []string
	}{
		{
			name:             "job status maps to scope",
			actionID:         "job.list",
			params:           map[string]any{"status": "failed"},
			schemaProperties: []string{"scope"},
			wantParams:       map[string]any{"scope": "failed"},
			wantAliases:      []string{"status->scope"},
		},
		{
			name:             "repository branch maps to ref",
			actionID:         "repository.file_get",
			params:           map[string]any{"branch": "main", "file_path": "README.md"},
			schemaProperties: []string{"ref", "file_path"},
			wantParams:       map[string]any{"ref": "main", "file_path": "README.md"},
			wantAliases:      []string{"branch->ref"},
		},
		{
			name:             "issue link copies project and linked issue targets",
			actionID:         "issue.link_create",
			params:           map[string]any{"project_id": 42, "linked_issue_iid": 7},
			schemaProperties: []string{"target_project_id", "target_issue_iid"},
			wantParams:       map[string]any{"project_id": 42, "target_project_id": 42, "target_issue_iid": 7},
			wantAliases:      []string{"linked_issue_iid->target_issue_iid", "project_id->target_project_id"},
		},
		{
			name:             "issue link source issue maps to issue iid",
			actionID:         "issue.link_create",
			params:           map[string]any{"source_issue_iid": 6},
			schemaProperties: []string{"issue_iid"},
			wantParams:       map[string]any{"issue_iid": 6},
			wantAliases:      []string{"source_issue_iid->issue_iid"},
		},
		{
			name:             "issue link relation maps to link type",
			actionID:         "issue.link_create",
			params:           map[string]any{"project_id": 42, "issue_iid": 6, "target_project_id": 42, "target_issue_iid": 7, "relation": "relates_to"},
			schemaProperties: []string{"project_id", "issue_iid", "target_project_id", "target_issue_iid", "link_type"},
			wantParams:       map[string]any{"project_id": 42, "issue_iid": 6, "target_project_id": 42, "target_issue_iid": 7, "link_type": "relates_to"},
			wantAliases:      []string{"relation->link_type"},
		},
		{
			name:             "issue link type maps to link type",
			actionID:         "issue.link_create",
			params:           map[string]any{"project_id": 42, "issue_iid": 6, "target_project_id": 42, "target_issue_iid": 7, "type": "blocks"},
			schemaProperties: []string{"project_id", "issue_iid", "target_project_id", "target_issue_iid", "link_type"},
			wantParams:       map[string]any{"project_id": 42, "issue_iid": 6, "target_project_id": 42, "target_issue_iid": 7, "link_type": "blocks"},
			wantAliases:      []string{"type->link_type"},
		},
		{
			name:             "issue spent time note maps to summary",
			actionID:         "issue.spent_time_add",
			params:           map[string]any{"project_id": 42, "issue_iid": 6, "duration": "30m", "note": "pairing"},
			schemaProperties: []string{"project_id", "issue_iid", "duration", "summary"},
			wantParams:       map[string]any{"project_id": 42, "issue_iid": 6, "duration": "30m", "summary": "pairing"},
			wantAliases:      []string{"note->summary"},
		},
		{
			name:             "issue time estimate time maps to duration",
			actionID:         "issue.time_estimate_set",
			params:           map[string]any{"project_id": 42, "issue_iid": 6, "time": "1h"},
			schemaProperties: []string{"project_id", "issue_iid", "duration"},
			wantParams:       map[string]any{"project_id": 42, "issue_iid": 6, "duration": "1h"},
			wantAliases:      []string{"time->duration"},
		},
		{
			name:             "issue state close spelling normalizes",
			actionID:         "issue.update",
			params:           map[string]any{"state_event": "closed"},
			schemaProperties: []string{"state_event"},
			wantParams:       map[string]any{"state_event": "close"},
			wantAliases:      []string{"state_event->state_event"},
		},
		{
			name:             "merge request emoji maps emoji to name",
			actionID:         "merge_request.emoji_mr_create",
			params:           map[string]any{"project_id": 42, "merge_request_iid": 3, "emoji": "eyes"},
			schemaProperties: []string{"project_id", "merge_request_iid", "name"},
			wantParams:       map[string]any{"project_id": 42, "merge_request_iid": 3, "name": "eyes"},
			wantAliases:      []string{"emoji->name"},
		},
		{
			name:             "merge request emoji drops stale unsupported params",
			actionID:         "merge_request.emoji_mr_create",
			params:           map[string]any{"project_id": 42, "merge_request_iid": 3, "name": "eyes", "duration": "15m", "awardable_type": "MergeRequest"},
			schemaProperties: []string{"project_id", "merge_request_iid", "name"},
			wantParams:       map[string]any{"project_id": 42, "merge_request_iid": 3, "name": "eyes"},
			wantAliases:      []string{"duration->removed", "awardable_type->removed"},
		},
		{
			name:             "pipeline schedule name maps to description",
			actionID:         "pipeline.schedule_create",
			params:           map[string]any{"name": "nightly"},
			schemaProperties: []string{"description"},
			wantParams:       map[string]any{"description": "nightly"},
			wantAliases:      []string{"name->description"},
		},
		{
			name:             "branch protection access levels normalize",
			actionID:         "branch.protect",
			params:           map[string]any{"push_access_level": "maintainers", "merge_access_level": float64(30)},
			schemaProperties: []string{"push_access_level", "merge_access_level"},
			wantParams:       map[string]any{"push_access_level": 40, "merge_access_level": 30},
			wantAliases:      []string{"push_access_level->push_access_level", "merge_access_level->merge_access_level"},
		},
		{
			name:             "terraform state unlock id maps to name",
			actionID:         "admin.terraform_state_unlock",
			params:           map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "id": "eval-state"},
			schemaProperties: []string{"project_id", "name"},
			wantParams:       map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "name": "eval-state"},
			wantAliases:      []string{"id->name"},
		},
		{
			name:             "feature flag version alias maps",
			actionID:         "feature_flags.feature_flag_create",
			params:           map[string]any{"new_version_flag": true},
			schemaProperties: []string{"version"},
			wantParams:       map[string]any{"version": true},
			wantAliases:      []string{"new_version_flag->version"},
		},
		{
			name:             "feature flag user list removes name",
			actionID:         "feature_flags.ff_user_list_list",
			params:           map[string]any{"project_id": 42, "name": "beta"},
			schemaProperties: []string{"project_id"},
			wantParams:       map[string]any{"project_id": 42},
			wantAliases:      []string{"name->removed"},
		},
		{
			name:             "group label name maps to new name",
			actionID:         "group.group_label_update",
			params:           map[string]any{"name": "old"},
			schemaProperties: []string{"new_name"},
			wantParams:       map[string]any{"new_name": "old"},
			wantAliases:      []string{"name->new_name"},
		},
		{
			name:             "project member access level normalizes",
			actionID:         "project.member_add",
			params:           map[string]any{"access_level": "owner"},
			schemaProperties: []string{"access_level"},
			wantParams:       map[string]any{"access_level": 50},
			wantAliases:      []string{"access_level->access_level"},
		},
		{
			name:             "release link release tag maps to tag name",
			actionID:         "release.link_update",
			params:           map[string]any{"release_tag_name": "v1.0.0"},
			schemaProperties: []string{"tag_name"},
			wantParams:       map[string]any{"tag_name": "v1.0.0"},
			wantAliases:      []string{"release_tag_name->tag_name"},
		},
		{
			name:     "release link batch normalizes link entries",
			actionID: "release.link_create_batch",
			params: map[string]any{"links": []any{
				map[string]any{"name": "checksums.txt", "link_url": "https://example.com/checksums.txt"},
				map[string]any{"name": "binary.txt", "url": "https://example.com/binary.txt", "filepath": "binary.txt", "direct_asset_path": "/binary.txt"},
			}},
			schemaProperties: []string{"links"},
			wantParams: map[string]any{"links": []any{
				map[string]any{"name": "checksums.txt", "url": "https://example.com/checksums.txt"},
				map[string]any{"name": "binary.txt", "url": "https://example.com/binary.txt"},
			}},
			wantAliases: []string{"links.link_url->links.url", "links.filepath->links", "links.direct_asset_path->links"},
		},
		{
			name:             "runner paused string boolean normalizes",
			actionID:         "runner.update",
			params:           map[string]any{"paused": "true"},
			schemaProperties: []string{"paused"},
			wantParams:       map[string]any{"paused": true},
			wantAliases:      []string{"paused->paused"},
		},
		{
			name:             "snippet single file params build files array",
			actionID:         "snippet.project_create",
			params:           map[string]any{"file_name": "main.go", "content": "package main"},
			schemaProperties: []string{"files"},
			wantParams:       map[string]any{"files": []any{map[string]any{"file_path": "main.go", "content": "package main"}}},
			wantAliases:      []string{"file_name/content->files"},
		},
		{
			name:     "snippet file entries normalize file names and remove create actions",
			actionID: "snippet.project_create",
			params: map[string]any{"files": []any{
				map[string]any{"file_name": "old.go", "content": "one", "action": "create"},
				map[string]any{"file_path": "kept.go", "file_name": "ignored.go", "content": "two", "action": "update"},
			}},
			schemaProperties: []string{"files"},
			wantParams: map[string]any{"files": []any{
				map[string]any{"file_path": "old.go", "content": "one"},
				map[string]any{"file_path": "kept.go", "content": "two", "action": "update"},
			}},
			wantAliases: []string{"files.file_name->files.file_path", "files.action->files"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			normalized, explanations := NormalizeParamsWithExplanation(testCase.actionID, testCase.params, schemaWithProperties(testCase.schemaProperties...))
			if !reflect.DeepEqual(normalized, testCase.wantParams) {
				t.Fatalf("normalized params = %#v, want %#v", normalized, testCase.wantParams)
			}
			if gotAliases := explanationAliases(explanations); !reflect.DeepEqual(gotAliases, testCase.wantAliases) {
				t.Fatalf("explanations = %#v, want %#v", gotAliases, testCase.wantAliases)
			}
		})
	}
}

// TestNormalizeParamsWithExplanation_NoChangeScenarios verifies canonical values are not overwritten by aliases.
func TestNormalizeParamsWithExplanation_NoChangeScenarios(t *testing.T) {
	testCases := []struct {
		name     string
		actionID string
		params   map[string]any
		schema   map[string]any
	}{
		{name: "empty params", actionID: "job.list", params: nil, schema: schemaWithProperties("scope")},
		{name: "missing schema properties", actionID: "job.list", params: map[string]any{"status": "failed"}, schema: map[string]any{}},
		{name: "canonical scope wins", actionID: "job.list", params: map[string]any{"status": "failed", "scope": "success"}, schema: schemaWithProperties("scope")},
		{name: "invalid issue state is left unchanged", actionID: "issue.update", params: map[string]any{"state_event": "paused"}, schema: schemaWithProperties("state_event")},
		{name: "snippet single file requires content", actionID: "snippet.project_create", params: map[string]any{"file_name": "main.go"}, schema: schemaWithProperties("files")},
		{name: "snippet file list ignores non-map entries", actionID: "snippet.project_create", params: map[string]any{"files": []any{"not-a-file-map"}}, schema: schemaWithProperties("files")},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			normalized, explanations := NormalizeParamsWithExplanation(testCase.actionID, testCase.params, testCase.schema)
			if !reflect.DeepEqual(normalized, testCase.params) {
				t.Fatalf("normalized params = %#v, want original %#v", normalized, testCase.params)
			}
			if len(explanations) != 0 {
				t.Fatalf("explanations = %#v, want none", explanations)
			}
		})
	}
}

// TestNormalizeParamsWithExplanation_AcceptedAliasCanonicalization verifies aliases can be normalized even when the schema still accepts the legacy field.
func TestNormalizeParamsWithExplanation_AcceptedAliasCanonicalization(t *testing.T) {
	tests := []struct {
		name        string
		actionID    string
		params      map[string]any
		wantParams  map[string]any
		wantAliases []string
	}{
		{
			name:        "pipeline schedule accepted name still canonicalizes",
			actionID:    "pipeline.schedule_create",
			params:      map[string]any{"name": "nightly"},
			wantParams:  map[string]any{"description": "nightly"},
			wantAliases: []string{"name->description"},
		},
		{
			name:        "pipeline schedule description wins over accepted name",
			actionID:    "pipeline.schedule_update",
			params:      map[string]any{"name": "old", "description": "canonical"},
			wantParams:  map[string]any{"description": "canonical"},
			wantAliases: []string{"name->description"},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			normalized, explanations := NormalizeParamsWithExplanation(testCase.actionID, testCase.params, schemaWithProperties("name", "description"))
			if !reflect.DeepEqual(normalized, testCase.wantParams) {
				t.Fatalf("normalized params = %#v, want %#v", normalized, testCase.wantParams)
			}
			if gotAliases := explanationAliases(explanations); !reflect.DeepEqual(gotAliases, testCase.wantAliases) {
				t.Fatalf("explanations = %#v, want %#v", gotAliases, testCase.wantAliases)
			}
		})
	}
}

// TestParameterNormalizationHelpers_ParseValues verifies exported parser helpers used by dynamic parameter normalization.
func TestParameterNormalizationHelpers_ParseValues(t *testing.T) {
	accessLevelCases := []struct {
		name      string
		value     any
		wantLevel int
		wantOK    bool
	}{
		{name: "int", value: 30, wantLevel: 30, wantOK: true},
		{name: "int64", value: int64(40), wantLevel: 40, wantOK: true},
		{name: "numeric string", value: "20", wantLevel: 20, wantOK: true},
		{name: "invalid numeric string", value: "99", wantOK: false},
		{name: "guest label", value: "guest", wantLevel: 10, wantOK: true},
		{name: "reporter label", value: " reporter ", wantLevel: 20, wantOK: true},
		{name: "label", value: " Developer ", wantLevel: 30, wantOK: true},
		{name: "unknown label", value: "admin", wantOK: false},
		{name: "fractional float", value: 30.5, wantOK: false},
		{name: "invalid number", value: 99, wantOK: false},
		{name: "unsupported type", value: true, wantOK: false},
	}
	for _, testCase := range accessLevelCases {
		t.Run("access level "+testCase.name, func(t *testing.T) {
			gotLevel, gotOK := GitLabAccessLevelValue(testCase.value)
			if gotLevel != testCase.wantLevel || gotOK != testCase.wantOK {
				t.Fatalf("GitLabAccessLevelValue(%#v) = %d, %t; want %d, %t", testCase.value, gotLevel, gotOK, testCase.wantLevel, testCase.wantOK)
			}
		})
	}

	stateCases := []struct {
		value     any
		wantState string
		wantOK    bool
	}{
		{value: " open ", wantState: "reopen", wantOK: true},
		{value: "closed", wantState: "close", wantOK: true},
		{value: "unknown", wantOK: false},
		{value: 123, wantOK: false},
	}
	for _, testCase := range stateCases {
		t.Run("issue state", func(t *testing.T) {
			gotState, gotOK := IssueStateEventValue(testCase.value)
			if gotState != testCase.wantState || gotOK != testCase.wantOK {
				t.Fatalf("IssueStateEventValue(%#v) = %q, %t; want %q, %t", testCase.value, gotState, gotOK, testCase.wantState, testCase.wantOK)
			}
		})
	}

	boolCases := []struct {
		value    any
		wantBool bool
		wantOK   bool
	}{
		{value: "true", wantBool: true, wantOK: true},
		{value: " false ", wantBool: false, wantOK: true},
		{value: "maybe", wantOK: false},
		{value: true, wantOK: false},
	}
	for _, testCase := range boolCases {
		t.Run("bool string", func(t *testing.T) {
			gotBool, gotOK := BoolStringValue(testCase.value)
			if gotBool != testCase.wantBool || gotOK != testCase.wantOK {
				t.Fatalf("BoolStringValue(%#v) = %t, %t; want %t, %t", testCase.value, gotBool, gotOK, testCase.wantBool, testCase.wantOK)
			}
		})
	}
}

// TestParameterAliases_SnippetFileNameTargetsFilePath verifies alias metadata matches snippet file normalization output.
func TestParameterAliases_SnippetFileNameTargetsFilePath(t *testing.T) {
	for _, alias := range ParameterAliases() {
		if alias.ActionID == actionSnippetProjectCreate && alias.Alias == "files.file_name" {
			if alias.Target != "files.file_path" {
				t.Fatalf("files.file_name target = %q, want files.file_path", alias.Target)
			}
			return
		}
	}
	t.Fatal("files.file_name snippet alias not found")
}

// TestActionAliasHelpers_NormalizationAndCompaction verifies alias normalization trims, sorts, and deduplicates values.
func TestActionAliasHelpers_NormalizationAndCompaction(t *testing.T) {
	aliases := cloneActionAliases([]ActionAlias{
		{Alias: " Z.Alias ", Canonical: " z.target ", Source: " source ", Reason: " reason "},
		{Alias: " A.Alias ", Canonical: " a.target ", Source: " source ", Reason: " reason "},
	})
	if aliases[0].Alias != "a.alias" || aliases[0].Canonical != "a.target" {
		t.Fatalf("first alias = %+v, want sorted and normalized a.alias -> a.target", aliases[0])
	}
	if aliases[1].Alias != "z.alias" || aliases[1].Canonical != "z.target" {
		t.Fatalf("second alias = %+v, want sorted and normalized z.alias -> z.target", aliases[1])
	}

	compacted := compactStrings([]string{"a", "a", "b", "b", "c"})
	if !reflect.DeepEqual(compacted, []string{"a", "b", "c"}) {
		t.Fatalf("compactStrings() = %#v, want unique sorted values", compacted)
	}
	if single := compactStrings([]string{"only"}); !reflect.DeepEqual(single, []string{"only"}) {
		t.Fatalf("compactStrings(single) = %#v, want unchanged", single)
	}
}

func schemaWithProperties(names ...string) map[string]any {
	properties := make(map[string]any, len(names))
	for _, name := range names {
		properties[name] = map[string]any{}
	}
	return map[string]any{"properties": properties}
}

func explanationAliases(explanations []toolutil.ParamAliasExplanation) []string {
	out := make([]string, 0, len(explanations))
	for _, explanation := range explanations {
		out = append(out, explanation.Alias+"->"+explanation.Canonical)
	}
	return out
}
