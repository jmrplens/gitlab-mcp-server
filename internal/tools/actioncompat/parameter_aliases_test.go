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
			name:             "release create message maps to description",
			actionID:         "release.create",
			params:           map[string]any{"tag_name": "v1.0.0", "name": "v1.0.0", "message": "Release notes"},
			schemaProperties: []string{"tag_name", "name", "description"},
			wantParams:       map[string]any{"tag_name": "v1.0.0", "name": "v1.0.0", "description": "Release notes"},
			wantAliases:      []string{"message->description"},
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

func TestNormalizeParamsWithExplanation_EnterpriseCompatibilityBranches(t *testing.T) {
	testCases := []struct {
		name             string
		actionID         string
		params           map[string]any
		schemaProperties []string
		wantParams       map[string]any
		wantAliases      []string
	}{
		{
			name:             "group protected branch protect maps branch and access levels",
			actionID:         "group.protected_branch_protect",
			params:           map[string]any{"branch": "release/*", "push_access_level": "Maintainer", "merge_access_level": "developer"},
			schemaProperties: []string{"name", "push_access_level", "merge_access_level"},
			wantParams:       map[string]any{"name": "release/*", "push_access_level": 40, "merge_access_level": 30},
			wantAliases:      []string{"branch->name", "push_access_level->push_access_level", "merge_access_level->merge_access_level"},
		},
		{
			name:             "project push rule unsigned alias maps",
			actionID:         "project.push_rule_edit",
			params:           map[string]any{"deny_unsigned_commits": true},
			schemaProperties: []string{"reject_unsigned_commits"},
			wantParams:       map[string]any{"reject_unsigned_commits": true},
			wantAliases:      []string{"deny_unsigned_commits->reject_unsigned_commits"},
		},
		{
			name:             "protected environment entries normalize",
			actionID:         "group.protected_env_protect",
			params:           map[string]any{"deploy_access_levels": "Maintainer", "approval_rules": map[string]any{"access_level": "Maintainer", "required_approval_count": 1}},
			schemaProperties: []string{"deploy_access_levels", "approval_rules"},
			wantParams: map[string]any{
				"deploy_access_levels": []any{map[string]any{"access_level": 40}},
				"approval_rules":       []any{map[string]any{"access_level": 40, "required_approvals": 1}},
			},
			wantAliases: []string{"deploy_access_levels->deploy_access_levels", "approval_rules->approval_rules"},
		},
		{
			name:             "protected environment approval count maps to approval rules",
			actionID:         "environment.protected_update",
			params:           map[string]any{"required_approval_count": 1},
			schemaProperties: []string{"required_approval_count", "approval_rules"},
			wantParams:       map[string]any{"approval_rules": []any{map[string]any{"access_level": 40, "required_approvals": 1}}},
			wantAliases:      []string{"required_approval_count->approval_rules"},
		},
		{
			name:             "pagination boolean maps to numeric page size",
			actionID:         "group.epic_discussion_list",
			params:           map[string]any{"first": true},
			schemaProperties: []string{"first"},
			wantParams:       map[string]any{"first": 100},
			wantAliases:      []string{"first->first"},
		},
		{
			name:             "approval rule count gets default maintainer principal",
			actionID:         "environment.protected_protect",
			params:           map[string]any{"approval_rules": []any{map[string]any{"required_approvals": 1}}},
			schemaProperties: []string{"approval_rules"},
			wantParams:       map[string]any{"approval_rules": []any{map[string]any{"access_level": 40, "required_approvals": 1}}},
			wantAliases:      []string{"approval_rules->approval_rules"},
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

// TestNormalizeCommonParams_PaginationBooleanCoercion verifies that common
// pagination boolean params (first, last, per_page, page) are coerced to their
// numeric defaults when sent as true, and removed when sent as false.
func TestNormalizeCommonParams_PaginationBooleanCoercion(t *testing.T) {
	tests := []struct {
		name        string
		params      map[string]any
		schema      []string
		wantParams  map[string]any
		wantAliases []string
	}{
		{
			name:        "first true maps to 100",
			params:      map[string]any{"first": true},
			schema:      []string{"first"},
			wantParams:  map[string]any{"first": 100},
			wantAliases: []string{"first->first"},
		},
		{
			name:        "page true maps to 1",
			params:      map[string]any{"page": true},
			schema:      []string{"page"},
			wantParams:  map[string]any{"page": 1},
			wantAliases: []string{"page->page"},
		},
		{
			name:        "per_page false is removed",
			params:      map[string]any{"per_page": false},
			schema:      []string{"per_page"},
			wantParams:  map[string]any{},
			wantAliases: []string{"per_page->per_page"},
		},
		{
			name:        "last true maps to 100",
			params:      map[string]any{"last": true},
			schema:      []string{"last"},
			wantParams:  map[string]any{"last": 100},
			wantAliases: []string{"last->last"},
		},
		{
			name:        "numeric first is unchanged",
			params:      map[string]any{"first": 20},
			schema:      []string{"first"},
			wantParams:  map[string]any{"first": 20},
			wantAliases: []string{},
		},
		{
			name:        "first not in schema is unchanged",
			params:      map[string]any{"first": true},
			schema:      []string{"query"},
			wantParams:  map[string]any{"first": true},
			wantAliases: []string{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			normalized, explanations := NormalizeParamsWithExplanation("unknown.action", tc.params, schemaWithProperties(tc.schema...))
			if !reflect.DeepEqual(normalized, tc.wantParams) {
				t.Fatalf("normalized params = %#v, want %#v", normalized, tc.wantParams)
			}
			gotAliases := explanationAliases(explanations)
			if len(gotAliases) != len(tc.wantAliases) {
				t.Fatalf("explanations = %#v, want %#v", gotAliases, tc.wantAliases)
			}
			for i, want := range tc.wantAliases {
				if gotAliases[i] != want {
					t.Fatalf("explanation[%d] = %q, want %q", i, gotAliases[i], want)
				}
			}
		})
	}
}

// TestDefaultPaginationValue_PageVsOthers verifies page defaults to 1 and all
// other pagination param names default to 100.
func TestDefaultPaginationValue_PageVsOthers(t *testing.T) {
	if got := defaultPaginationValue("page"); got != 1 {
		t.Fatalf("defaultPaginationValue(page) = %d, want 1", got)
	}
	for _, name := range []string{"first", "last", "per_page"} {
		if got := defaultPaginationValue(name); got != 100 {
			t.Fatalf("defaultPaginationValue(%q) = %d, want 100", name, got)
		}
	}
}

// TestNormalizeGroupProtectedBranchProtectParams_BranchAndAccessLevels verifies
// that the branch alias and access level normalizations are applied together.
func TestNormalizeGroupProtectedBranchProtectParams_BranchAndAccessLevels(t *testing.T) {
	tests := []struct {
		name        string
		params      map[string]any
		wantParams  map[string]any
		wantAliases []string
	}{
		{
			name:        "branch maps to name only",
			params:      map[string]any{"branch": "main"},
			wantParams:  map[string]any{"name": "main"},
			wantAliases: []string{"branch->name"},
		},
		{
			name:        "push access level string normalizes",
			params:      map[string]any{"push_access_level": "Developer"},
			wantParams:  map[string]any{"push_access_level": 30},
			wantAliases: []string{"push_access_level->push_access_level"},
		},
		{
			name:        "merge access level string normalizes",
			params:      map[string]any{"merge_access_level": "maintainer"},
			wantParams:  map[string]any{"merge_access_level": 40},
			wantAliases: []string{"merge_access_level->merge_access_level"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			normalized, explanations := NormalizeParamsWithExplanation(actionGroupProtectedBranchProtect, tc.params, schemaWithProperties("name", "push_access_level", "merge_access_level"))
			if !reflect.DeepEqual(normalized, tc.wantParams) {
				t.Fatalf("normalized params = %#v, want %#v", normalized, tc.wantParams)
			}
			if gotAliases := explanationAliases(explanations); !reflect.DeepEqual(gotAliases, tc.wantAliases) {
				t.Fatalf("explanations = %#v, want %#v", gotAliases, tc.wantAliases)
			}
		})
	}
}

// TestNormalizeProjectPushRuleParams_UnsignedCommitAlias verifies deny_unsigned_commits
// maps to reject_unsigned_commits.
func TestNormalizeProjectPushRuleParams_UnsignedCommitAlias(t *testing.T) {
	normalized, explanations := NormalizeParamsWithExplanation(
		actionProjectPushRuleAdd,
		map[string]any{"deny_unsigned_commits": false},
		schemaWithProperties("reject_unsigned_commits"),
	)
	if !reflect.DeepEqual(normalized, map[string]any{"reject_unsigned_commits": false}) {
		t.Fatalf("normalized = %#v, want reject_unsigned_commits", normalized)
	}
	if got := explanationAliases(explanations); !reflect.DeepEqual(got, []string{"deny_unsigned_commits->reject_unsigned_commits"}) {
		t.Fatalf("explanations = %#v, want deny_unsigned_commits->reject_unsigned_commits", got)
	}
}

// TestNormalizeProtectedEnvironmentParams_AccessEntriesAndApprovalCount verifies
// the full protected environment normalization pipeline including primitive access
// levels, object entries, and required_approval_count promotion.
func TestNormalizeProtectedEnvironmentParams_AccessEntriesAndApprovalCount(t *testing.T) {
	tests := []struct {
		name        string
		actionID    string
		params      map[string]any
		schema      []string
		wantParams  map[string]any
		wantAliases []string
	}{
		{
			name:     "primitive access level wraps to array",
			actionID: actionGroupProtectedEnvProtect,
			params:   map[string]any{"deploy_access_levels": float64(40)},
			schema:   []string{"deploy_access_levels"},
			wantParams: map[string]any{
				"deploy_access_levels": []any{map[string]any{"access_level": 40}},
			},
			wantAliases: []string{"deploy_access_levels->deploy_access_levels"},
		},
		{
			name:     "object entry normalizes access level string",
			actionID: actionGroupProtectedEnvProtect,
			params:   map[string]any{"deploy_access_levels": map[string]any{"access_level": "Developer"}},
			schema:   []string{"deploy_access_levels"},
			wantParams: map[string]any{
				"deploy_access_levels": []any{map[string]any{"access_level": 30}},
			},
			wantAliases: []string{"deploy_access_levels->deploy_access_levels"},
		},
		{
			name:     "required_approval_count promotes to approval_rules",
			actionID: actionProjectProtectedEnvProtect,
			params:   map[string]any{"required_approval_count": 2},
			schema:   []string{"approval_rules"},
			wantParams: map[string]any{
				"approval_rules": []any{map[string]any{"access_level": 40, "required_approvals": 2}},
			},
			wantAliases: []string{"required_approval_count->approval_rules"},
		},
		{
			name:     "required_approval_count dropped when approval_rules already present",
			actionID: actionProjectProtectedEnvProtect,
			params: map[string]any{
				"approval_rules":          []any{map[string]any{"access_level": 40, "required_approvals": 1}},
				"required_approval_count": 2,
			},
			schema: []string{"approval_rules"},
			wantParams: map[string]any{
				"approval_rules": []any{map[string]any{"access_level": 40, "required_approvals": 1}},
			},
			wantAliases: []string{"required_approval_count->approval_rules"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			normalized, explanations := NormalizeParamsWithExplanation(tc.actionID, tc.params, schemaWithProperties(tc.schema...))
			if !reflect.DeepEqual(normalized, tc.wantParams) {
				t.Fatalf("normalized = %#v, want %#v", normalized, tc.wantParams)
			}
			if gotAliases := explanationAliases(explanations); !reflect.DeepEqual(gotAliases, tc.wantAliases) {
				t.Fatalf("explanations = %#v, want %#v", gotAliases, tc.wantAliases)
			}
		})
	}
}

// TestProtectedEnvironmentAccessEntryHelpers verifies the fine-grained entry
// normalization and principal-detection helpers used by protected env params.
func TestProtectedEnvironmentAccessEntryHelpers(t *testing.T) {
	// protectedEnvironmentPrimitiveAccessEntry: numeric access level wraps to object.
	if entry, ok := protectedEnvironmentPrimitiveAccessEntry(float64(30)); !ok || entry["access_level"] != 30 {
		t.Fatalf("primitive float64(30) = %v, %v; want {access_level:30}, true", entry, ok)
	}
	if _, ok := protectedEnvironmentPrimitiveAccessEntry("unknown"); ok {
		t.Fatal("primitive(unknown string) should return false")
	}
	if _, ok := protectedEnvironmentPrimitiveAccessEntry(map[string]any{"x": 1}); ok {
		t.Fatal("primitive(map) should return false")
	}

	// normalizeProtectedEnvironmentAccessEntry: alias promotion.
	entry := normalizeProtectedEnvironmentAccessEntry(map[string]any{"deploy_access_level": "Maintainer"}, false)
	if entry["access_level"] != 40 {
		t.Fatalf("normalizeEntry alias = %#v, want access_level:40", entry)
	}

	// approvalRules=true branch with required_approval_count alias.
	approvalEntry := normalizeProtectedEnvironmentAccessEntry(map[string]any{"required_approval_count": 3}, true)
	if approvalEntry["required_approvals"] != 3 {
		t.Fatalf("approvalEntry.required_approvals = %#v, want 3", approvalEntry["required_approvals"])
	}
	// No principal → default access_level=40 is added.
	if approvalEntry["access_level"] != 40 {
		t.Fatalf("approvalEntry.access_level = %#v, want 40 (default principal)", approvalEntry["access_level"])
	}

	// protectedEnvironmentAccessEntryHasPrincipal detects presence of principal keys.
	if !protectedEnvironmentAccessEntryHasPrincipal(map[string]any{"access_level": 40}) {
		t.Fatal("hasPrincipal with access_level should return true")
	}
	if !protectedEnvironmentAccessEntryHasPrincipal(map[string]any{"user_id": 1}) {
		t.Fatal("hasPrincipal with user_id should return true")
	}
	if !protectedEnvironmentAccessEntryHasPrincipal(map[string]any{"group_id": 5}) {
		t.Fatal("hasPrincipal with group_id should return true")
	}
	if protectedEnvironmentAccessEntryHasPrincipal(map[string]any{"required_approvals": 2}) {
		t.Fatal("hasPrincipal without principal key should return false")
	}
}

// TestProtectedEnvironmentApprovalCount_NonIntegerValue verifies the
// normalizeProtectedEnvironmentApprovalCount function returns early when
// required_approval_count is present but cannot be converted to an integer.
func TestProtectedEnvironmentApprovalCount_NonIntegerValue(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{name: "non-numeric string", value: "not-a-number"},
		{name: "boolean", value: true},
		{name: "float with fraction", value: 2.5},
		{name: "nil", value: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			params := map[string]any{"required_approval_count": tc.value}
			normalized, explanations := NormalizeParamsWithExplanation(
				actionProjectProtectedEnvProtect,
				params,
				schemaWithProperties("approval_rules"),
			)
			// Function should return without recording any normalization,
			// leaving the parameter unchanged.
			if len(explanations) != 0 {
				t.Errorf("expected no explanations, got %v", explanationAliases(explanations))
			}
			if _, ok := normalized["required_approval_count"]; !ok {
				t.Errorf("required_approval_count was unexpectedly removed")
			}
		})
	}
}

// TestIssueUpdateParams_StateEventNotAccepted verifies that normalizeIssueUpdateParams
// is a no-op when the schema does not accept state_event.
func TestIssueUpdateParams_StateEventNotAccepted(t *testing.T) {
	params := map[string]any{"state_event": "close"}
	normalized, explanations := NormalizeParamsWithExplanation(
		"issue.update",
		params,
		// Empty schema: state_event is not accepted.
		schemaWithProperties(),
	)
	if len(explanations) != 0 {
		t.Errorf("expected no explanations, got %v", explanationAliases(explanations))
	}
	if normalized["state_event"] != "close" {
		t.Errorf("state_event = %v, want unchanged", normalized["state_event"])
	}
}

// TestProtectedEnvironmentAccessEntries_UnsupportedValue verifies the
// protectedEnvironmentAccessEntries helper returns the original value
// unchanged when none of the supported types (primitive, map, []any) match.
func TestProtectedEnvironmentAccessEntries_UnsupportedValue(t *testing.T) {
	// A non-supported value type (string that is not a known access label).
	v, changed := protectedEnvironmentAccessEntries("not-an-access-level", false)
	if changed {
		t.Errorf("expected changed=false for unsupported value, got true with %v", v)
	}
}

// TestProtectedEnvironmentAccessEntries_DefaultSwitchInLoop verifies the
// switch default branch (non-map, non-primitive entries inside the array).
func TestProtectedEnvironmentAccessEntries_DefaultSwitchInLoop(t *testing.T) {
	// An array of unsupported items — none of them are maps, so the
	// default case runs for each and the function returns the original
	// value with changed=false.
	v, changed := protectedEnvironmentAccessEntries([]any{123, 456}, false)
	if changed {
		t.Errorf("expected changed=false for array of primitives, got true with %v", v)
	}
}

// TestProtectedEnvironmentAccessEntries_PrimitiveInArray verifies the
// switch default branch wraps a valid primitive (int) into an object.
func TestProtectedEnvironmentAccessEntries_PrimitiveInArray(t *testing.T) {
	v, changed := protectedEnvironmentAccessEntries([]any{float64(30)}, false)
	if !changed {
		t.Errorf("expected changed=true for primitive access level in array, got false")
	}
	items, ok := v.([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected []any with 1 item, got %v", v)
	}
	entry, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", items[0])
	}
	if entry["access_level"] != 30 {
		t.Errorf("access_level = %v, want 30", entry["access_level"])
	}
}

// TestAccessLevelParam_NonConvertibleValue verifies that normalizeAccessLevelParamWith
// is a no-op when the value cannot be converted to a valid access level.
func TestAccessLevelParam_NonConvertibleValue(t *testing.T) {
	// push_access_level=99 is not a valid GitLab access level (must be 0,30,40).
	params := map[string]any{"push_access_level": 99}
	normalized, explanations := NormalizeParamsWithExplanation(
		actionBranchProtect,
		params,
		schemaWithProperties("push_access_level", "merge_access_level"),
	)
	if len(explanations) != 0 {
		t.Errorf("expected no explanations, got %v", explanationAliases(explanations))
	}
	if normalized["push_access_level"] != 99 {
		t.Errorf("push_access_level = %v, want unchanged 99", normalized["push_access_level"])
	}
}

// TestMoveWithoutSchemaCheck_AliasMissing verifies the early return when
// the alias is not present in params.
func TestMoveWithoutSchemaCheck_AliasMissing(t *testing.T) {
	// Use a group label update call without "name" in params — the
	// moveWithoutSchemaCheck("name", "new_name", ...) should be a no-op.
	params := map[string]any{"color": "#ff0000"}
	normalized, explanations := NormalizeParamsWithExplanation(
		actionGroupLabelUpdate,
		params,
		schemaWithProperties("new_name", "color"),
	)
	if len(explanations) != 0 {
		t.Errorf("expected no explanations, got %v", explanationAliases(explanations))
	}
	if _, ok := normalized["new_name"]; ok {
		t.Errorf("new_name should not be set when 'name' alias is missing")
	}
}

// TestMoveWithoutSchemaCheck_TargetPresent verifies the early return when
// the target key is already present in state.out.
func TestMoveWithoutSchemaCheck_TargetPresent(t *testing.T) {
	// Both "name" alias and "new_name" target are present — the move
	// should not happen (early return).
	params := map[string]any{"name": "old-label", "new_name": "existing-name"}
	normalized, explanations := NormalizeParamsWithExplanation(
		actionGroupLabelUpdate,
		params,
		schemaWithProperties("new_name"),
	)
	if len(explanations) != 0 {
		t.Errorf("expected no explanations, got %v", explanationAliases(explanations))
	}
	if normalized["new_name"] != "existing-name" {
		t.Errorf("new_name = %v, want unchanged 'existing-name'", normalized["new_name"])
	}
}

// TestRunnerUpdateParams_PausedNotAccepted verifies that normalizeRunnerUpdateParams
// is a no-op when the schema does not accept "paused".
func TestRunnerUpdateParams_PausedNotAccepted(t *testing.T) {
	params := map[string]any{"paused": "true"}
	normalized, explanations := NormalizeParamsWithExplanation(
		"runner.update",
		params,
		// Empty schema: paused is not accepted.
		schemaWithProperties(),
	)
	if len(explanations) != 0 {
		t.Errorf("expected no explanations, got %v", explanationAliases(explanations))
	}
	if normalized["paused"] != "true" {
		t.Errorf("paused = %v, want unchanged 'true'", normalized["paused"])
	}
}

// TestRunnerUpdateParams_NonParseableValue verifies the early return when
// the "paused" value cannot be parsed as a bool.
func TestRunnerUpdateParams_NonParseableValue(t *testing.T) {
	params := map[string]any{"paused": "maybe"}
	normalized, explanations := NormalizeParamsWithExplanation(
		"runner.update",
		params,
		schemaWithProperties("paused"),
	)
	if len(explanations) != 0 {
		t.Errorf("expected no explanations, got %v", explanationAliases(explanations))
	}
	if normalized["paused"] != "maybe" {
		t.Errorf("paused = %v, want unchanged 'maybe'", normalized["paused"])
	}
}

// TestSnippetProjectCreateParams_FilesNotAccepted verifies the early return
// when the schema does not accept "files".
func TestSnippetProjectCreateParams_FilesNotAccepted(t *testing.T) {
	params := map[string]any{"file_name": "x", "content": "y"}
	normalized, explanations := NormalizeParamsWithExplanation(
		actionSnippetProjectCreate,
		params,
		// Empty schema: files is not accepted.
		schemaWithProperties(),
	)
	if len(explanations) != 0 {
		t.Errorf("expected no explanations, got %v", explanationAliases(explanations))
	}
	if _, ok := normalized["files"]; ok {
		t.Errorf("files was unexpectedly added")
	}
	if normalized["file_name"] != "x" {
		t.Errorf("file_name = %v, want unchanged 'x'", normalized["file_name"])
	}
}

// TestReleaseLinkBatchEntries_NonArrayLinks verifies the early return when
// "links" is not a slice.
func TestReleaseLinkBatchEntries_NonArrayLinks(t *testing.T) {
	clone := func() map[string]any { return map[string]any{} }
	record := func(alias, target, reason string) {}
	params := map[string]any{"links": "not-an-array"}
	if normalizeReleaseLinkBatchEntries(clone, params, record) {
		t.Error("expected changed=false for non-array links")
	}
}

// TestReleaseLinkBatchEntries_EmptyLinks verifies the early return when
// "links" is an empty slice.
func TestReleaseLinkBatchEntries_EmptyLinks(t *testing.T) {
	clone := func() map[string]any { return map[string]any{} }
	record := func(alias, target, reason string) {}
	params := map[string]any{"links": []any{}}
	if normalizeReleaseLinkBatchEntries(clone, params, record) {
		t.Error("expected changed=false for empty links")
	}
}

// TestReleaseLinkBatchEntries_NonMapLink verifies the per-item continue
// when a link is not a map.
func TestReleaseLinkBatchEntries_NonMapLink(t *testing.T) {
	clone := func() map[string]any { return map[string]any{} }
	record := func(alias, target, reason string) {}
	params := map[string]any{"links": []any{"not-a-map"}}
	if normalizeReleaseLinkBatchEntries(clone, params, record) {
		t.Error("expected changed=false for non-map link")
	}
}

// TestReleaseLinkBatchEntries_NoChanges verifies the case where the link map
// has no aliases to normalize (so !linkChanged → continue).
func TestReleaseLinkBatchEntries_NoChanges(t *testing.T) {
	clone := func() map[string]any { return map[string]any{} }
	record := func(alias, target, reason string) {}
	params := map[string]any{"links": []any{map[string]any{"url": "https://x"}}}
	if normalizeReleaseLinkBatchEntries(clone, params, record) {
		t.Error("expected changed=false for link with no aliases to normalize")
	}
}

// TestEnvironmentAccessLevelValue_RoleLabelsAndNumericValues verifies the
// environment access level value parser accepts both label strings and numbers.
func TestEnvironmentAccessLevelValue_RoleLabelsAndNumericValues(t *testing.T) {
	cases := []struct {
		value     any
		wantLevel int
		wantOK    bool
	}{
		{value: float64(40), wantLevel: 40, wantOK: true},
		{value: int(30), wantLevel: 30, wantOK: true},
		{value: int64(20), wantLevel: 20, wantOK: true},
		{value: "developer", wantLevel: 30, wantOK: true},
		{value: "guest", wantLevel: 10, wantOK: true},
		{value: "reporter", wantLevel: 20, wantOK: true},
		{value: "owner", wantLevel: 50, wantOK: true},
		{value: "admin", wantLevel: 60, wantOK: true},
		{value: "no access", wantLevel: 0, wantOK: true},
		{value: "unknown-role", wantOK: false},
		{value: 99, wantOK: false},
		{value: true, wantOK: false},
	}
	for _, tc := range cases {
		got, ok := environmentAccessLevelValue(tc.value)
		if ok != tc.wantOK || got != tc.wantLevel {
			t.Fatalf("environmentAccessLevelValue(%#v) = %d, %v; want %d, %v", tc.value, got, ok, tc.wantLevel, tc.wantOK)
		}
	}
}

// TestGitLabBranchProtectionAccessLevelValue_Defaults verifies the
// branch-protection access level parser handles numeric, string, and
// unknown values consistently.
func TestGitLabBranchProtectionAccessLevelValue_Defaults(t *testing.T) {
	cases := []struct {
		value     any
		wantLevel int
		wantOK    bool
	}{
		{value: int(30), wantLevel: 30, wantOK: true},
		{value: int(40), wantLevel: 40, wantOK: true},
		{value: int(0), wantLevel: 0, wantOK: true},
		{value: int(99), wantOK: false},
		{value: int64(40), wantLevel: 40, wantOK: true},
		{value: float64(30.0), wantLevel: 30, wantOK: true},
		{value: float64(30.5), wantOK: false},
		{value: "developer", wantLevel: 30, wantOK: true},
		{value: "maintainer", wantLevel: 40, wantOK: true},
		{value: "no_access", wantLevel: 0, wantOK: true},
		{value: "30", wantLevel: 30, wantOK: true},
		{value: "99", wantOK: false},
		{value: "unknown", wantOK: false},
		{value: true, wantOK: false},
	}
	for _, tc := range cases {
		got, ok := gitLabBranchProtectionAccessLevelValue(tc.value)
		if ok != tc.wantOK || got != tc.wantLevel {
			t.Fatalf("gitLabBranchProtectionAccessLevelValue(%#v) = %d, %v; want %d, %v", tc.value, got, ok, tc.wantLevel, tc.wantOK)
		}
	}
}

// TestGitLabAccessLevelValue_InvalidLevel verifies that gitlabAccessLevelValue
// rejects invalid access level numbers.
func TestGitLabAccessLevelValue_InvalidLevel(t *testing.T) {
	if level, ok := gitlabAccessLevelValue(99); ok {
		t.Errorf("expected false for 99, got level=%d ok=%v", level, ok)
	}
	if level, ok := gitlabAccessLevelValue(true); ok {
		t.Errorf("expected false for bool, got level=%d ok=%v", level, ok)
	}
	if level, ok := gitlabAccessLevelValue(float64(40.5)); ok {
		t.Errorf("expected false for non-integer float, got level=%d ok=%v", level, ok)
	}
	if level, ok := gitlabAccessLevelValue("maintainer"); !ok || level != 40 {
		t.Errorf("expected maintainer → 40, got level=%d ok=%v", level, ok)
	}
	// Float64 with whole-number value should succeed.
	if level, ok := gitlabAccessLevelValue(float64(40)); !ok || level != 40 {
		t.Errorf("expected float64(40) → 40, got level=%d ok=%v", level, ok)
	}
}

// TestIntegerValue_TypeCoverage verifies integerValue handles all supported
// numeric representations and rejects unsupported ones.
func TestIntegerValue_TypeCoverage(t *testing.T) {
	cases := []struct {
		value   any
		wantInt int
		wantOK  bool
	}{
		{value: int(7), wantInt: 7, wantOK: true},
		{value: int64(42), wantInt: 42, wantOK: true},
		{value: float64(10.0), wantInt: 10, wantOK: true},
		{value: float64(10.5), wantInt: 10, wantOK: false},
		{value: "5", wantInt: 5, wantOK: true},
		{value: " 8 ", wantInt: 8, wantOK: true},
		{value: "abc", wantOK: false},
		{value: true, wantOK: false},
	}
	for _, tc := range cases {
		got, ok := integerValue(tc.value)
		if ok != tc.wantOK || got != tc.wantInt {
			t.Fatalf("integerValue(%#v) = %d, %v; want %d, %v", tc.value, got, ok, tc.wantInt, tc.wantOK)
		}
	}
}
