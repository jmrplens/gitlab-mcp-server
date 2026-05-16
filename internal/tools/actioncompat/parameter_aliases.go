package actioncompat

import (
	"maps"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const parameterAliasExplanationSource = "dynamic_action_scoped"

// ParameterAlias describes one historical action-scoped parameter alias or
// coercion policy.
type ParameterAlias struct {
	ActionID       string
	Alias          string
	Target         string
	Source         string
	Searchable     bool
	Deprecated     bool
	RemovalVersion string
	Reason         string
	SpecMetadata   bool
}

// ParameterAliases returns historical action-scoped parameter aliases and
// coercion policies used by Dynamic execute compatibility normalization.
func ParameterAliases() []ParameterAlias {
	return cloneParameterAliases(defaultParameterAliases())
}

func defaultParameterAliases() []ParameterAlias {
	return []ParameterAlias{
		parameterAlias("job.list", "status", "scope", "job.list uses scope for job status filtering"),
		parameterAlias("repository.file_get", "branch", "ref", "repository.file_get reads file content at a ref"),
		parameterAlias("issue.link_create", "linked_issue_iid", "target_issue_iid", "issue.link_create uses target_issue_iid for the linked issue"),
		parameterAlias("issue.link_create", "project_id", "target_project_id", "same-project issue links reuse project_id as target_project_id"),
		parameterAlias("issue.update", "state_event", "state_event", "normalized issue state event value"),
		parameterAlias("pipeline.schedule_create", "name", "description", "pipeline schedules use description as the display name"),
		parameterAlias("pipeline.schedule_update", "name", "description", "pipeline schedules use description as the display name"),
		parameterAlias("branch.protect", "push_access_level", "push_access_level", "normalized GitLab access level name to numeric level"),
		parameterAlias("branch.protect", "merge_access_level", "merge_access_level", "normalized GitLab access level name to numeric level"),
		parameterAlias("feature_flags.feature_flag_create", "new_version_flag", "version", "feature flag creation uses version for the flag API version"),
		normalizerOnlyParameterAlias("feature_flags.ff_user_list_list", "name", "removed", "feature flag user-list listing is project-scoped and does not accept a feature flag name"),
		parameterAlias("group.group_label_update", "name", "new_name", "group label update renames labels with new_name"),
		parameterAlias("project.member_add", "access_level", "access_level", "normalized GitLab access level name to numeric level"),
		parameterAlias("project.member_edit", "access_level", "access_level", "normalized GitLab access level name to numeric level"),
		parameterAlias("release.link_create", "release_tag_name", "tag_name", "release link actions use tag_name for the parent release"),
		parameterAlias("release.link_delete", "release_tag_name", "tag_name", "release link actions use tag_name for the parent release"),
		parameterAlias("release.link_get", "release_tag_name", "tag_name", "release link actions use tag_name for the parent release"),
		parameterAlias("release.link_list", "release_tag_name", "tag_name", "release link actions use tag_name for the parent release"),
		parameterAlias("release.link_update", "release_tag_name", "tag_name", "release link actions use tag_name for the parent release"),
		parameterAlias("runner.update", "paused", "paused", "normalized string boolean to bool"),
		parameterAlias("snippet.project_create", "file_name/content", "files", "project snippet creation uses files entries in dynamic mode"),
		parameterAlias("snippet.project_create", "files.file_name", "files", "snippet file entries use file_path"),
		parameterAlias("snippet.project_create", "files.action", "files", "project snippet creation file entries do not include an action field"),
	}
}

func parameterAlias(actionID, alias, target, reason string) ParameterAlias {
	return ParameterAlias{ActionID: actionID, Alias: alias, Target: target, Source: SourceCompatibility, Searchable: true, Reason: reason, SpecMetadata: true}
}

func normalizerOnlyParameterAlias(actionID, alias, target, reason string) ParameterAlias {
	return ParameterAlias{ActionID: actionID, Alias: alias, Target: target, Source: SourceCompatibility, Searchable: true, Reason: reason}
}

func cloneParameterAliases(aliases []ParameterAlias) []ParameterAlias {
	out := append([]ParameterAlias(nil), aliases...)
	for index := range out {
		out[index].ActionID = strings.TrimSpace(strings.ToLower(out[index].ActionID))
		out[index].Alias = strings.TrimSpace(out[index].Alias)
		out[index].Target = strings.TrimSpace(out[index].Target)
		out[index].Source = strings.TrimSpace(out[index].Source)
		out[index].Reason = strings.TrimSpace(out[index].Reason)
	}
	return out
}

// NormalizeParamsWithExplanation applies action-scoped compatibility aliases
// and coercions for Dynamic execute.
func NormalizeParamsWithExplanation(actionID string, params, schema map[string]any) (map[string]any, []toolutil.ParamAliasExplanation) {
	if len(params) == 0 {
		return params, nil
	}
	fields := actionSchemaProperties(schema)
	out := params
	cloned := false
	explanations := make([]toolutil.ParamAliasExplanation, 0)
	clone := func() map[string]any {
		if !cloned {
			out = maps.Clone(params)
			cloned = true
		}
		return out
	}
	record := func(alias, target, reason string) {
		explanations = append(explanations, toolutil.ParamAliasExplanation{Alias: alias, Canonical: target, Source: parameterAliasExplanationSource, Notes: reason})
	}
	accepts := func(name string) bool {
		_, ok := fields[name]
		return ok
	}
	switch actionID {
	case "job.list":
		if value, ok := out["status"]; ok && accepts("scope") && !accepts("status") {
			if _, hasScope := out["scope"]; !hasScope {
				updated := clone()
				updated["scope"] = value
				delete(updated, "status")
				record("status", "scope", "job.list uses scope for job status filtering")
			}
		}
	case "repository.file_get":
		if value, ok := out["branch"]; ok && accepts("ref") && !accepts("branch") {
			if _, hasRef := out["ref"]; !hasRef {
				updated := clone()
				updated["ref"] = value
				delete(updated, "branch")
				record("branch", "ref", "repository.file_get reads file content at a ref")
			}
		}
	case "issue.link_create":
		if value, ok := out["linked_issue_iid"]; ok && accepts("target_issue_iid") && !accepts("linked_issue_iid") {
			if _, hasTargetIssueIID := out["target_issue_iid"]; !hasTargetIssueIID {
				updated := clone()
				updated["target_issue_iid"] = value
				delete(updated, "linked_issue_iid")
				record("linked_issue_iid", "target_issue_iid", "issue.link_create uses target_issue_iid for the linked issue")
			}
		}
		if value, ok := out["project_id"]; ok && accepts("target_project_id") {
			if _, hasTargetProjectID := out["target_project_id"]; !hasTargetProjectID {
				clone()["target_project_id"] = value
				record("project_id", "target_project_id", "same-project issue links reuse project_id as target_project_id")
			}
		}
	case "issue.update":
		if value, ok := out["state_event"]; ok && accepts("state_event") {
			if stateEvent, converted := issueStateEventValue(value); converted {
				clone()["state_event"] = stateEvent
				record("state_event", "state_event", "normalized issue state event value")
			}
		}
	case "pipeline.schedule_create", "pipeline.schedule_update":
		if value, ok := out["name"]; ok && accepts("description") && !accepts("name") {
			updated := clone()
			if _, hasDescription := out["description"]; !hasDescription {
				updated["description"] = value
			}
			delete(updated, "name")
			record("name", "description", "pipeline schedules use description as the display name")
		}
	case "branch.protect":
		for _, name := range []string{"push_access_level", "merge_access_level"} {
			if value, ok := out[name]; ok && accepts(name) {
				if accessLevel, converted := gitlabAccessLevelValue(value); converted {
					clone()[name] = accessLevel
					record(name, name, "normalized GitLab access level name to numeric level")
				}
			}
		}
	case "feature_flags.feature_flag_create":
		if value, ok := out["new_version_flag"]; ok && accepts("version") && !accepts("new_version_flag") {
			if _, hasVersion := out["version"]; !hasVersion {
				updated := clone()
				updated["version"] = value
				delete(updated, "new_version_flag")
				record("new_version_flag", "version", "feature flag creation uses version for the flag API version")
			}
		}
	case "feature_flags.ff_user_list_list":
		if _, ok := out["name"]; ok && !accepts("name") {
			delete(clone(), "name")
			record("name", "removed", "feature flag user-list listing is project-scoped and does not accept a feature flag name")
		}
	case "group.group_label_update":
		if value, ok := out["name"]; ok {
			if _, hasNewName := out["new_name"]; !hasNewName {
				updated := clone()
				updated["new_name"] = value
				delete(updated, "name")
				record("name", "new_name", "group label update renames labels with new_name")
			}
		}
	case "project.member_add", "project.member_edit":
		if value, ok := out["access_level"]; ok && accepts("access_level") {
			if accessLevel, converted := gitlabAccessLevelValue(value); converted {
				clone()["access_level"] = accessLevel
				record("access_level", "access_level", "normalized GitLab access level name to numeric level")
			}
		}
	case "release.link_create", "release.link_delete", "release.link_get", "release.link_list", "release.link_update":
		if value, ok := out["release_tag_name"]; ok && accepts("tag_name") && !accepts("release_tag_name") {
			if _, hasTagName := out["tag_name"]; !hasTagName {
				updated := clone()
				updated["tag_name"] = value
				delete(updated, "release_tag_name")
				record("release_tag_name", "tag_name", "release link actions use tag_name for the parent release")
			}
		}
	case "runner.update":
		if value, ok := out["paused"]; ok && accepts("paused") {
			if paused, converted := boolStringValue(value); converted {
				clone()["paused"] = paused
				record("paused", "paused", "normalized string boolean to bool")
			}
		}
	case "snippet.project_create":
		if accepts("files") && (!accepts("file_name") || !accepts("content")) && buildSnippetCreateFilesFromSingleFileParams(clone, out) {
			record("file_name/content", "files", "project snippet creation uses files entries in dynamic mode")
		}
		if accepts("files") && normalizeSnippetFileNameFields(clone, out) {
			record("files.file_name", "files.file_path", "snippet file entries use file_path")
		}
		if accepts("files") && stripSnippetCreateFileActions(clone, out) {
			record("files.action", "files", "project snippet creation file entries do not include an action field")
		}
	}
	return out, explanations
}

func buildSnippetCreateFilesFromSingleFileParams(clone func() map[string]any, params map[string]any) bool {
	if _, hasFiles := params["files"]; hasFiles {
		return false
	}
	fileName, hasFileName := nonEmptyStringParam(params, "file_name")
	content, hasContent := nonEmptyStringParam(params, "content")
	if !hasFileName || !hasContent {
		return false
	}
	updated := clone()
	updated["files"] = []any{map[string]any{"file_path": fileName, "content": content}}
	delete(updated, "file_name")
	delete(updated, "content")
	return true
}

func nonEmptyStringParam(params map[string]any, name string) (string, bool) {
	value, ok := params[name].(string)
	if !ok {
		return "", false
	}
	value = strings.TrimSpace(value)
	return value, value != ""
}

func normalizeSnippetFileNameFields(clone func() map[string]any, params map[string]any) bool {
	files, ok := params["files"].([]any)
	if !ok || len(files) == 0 {
		return false
	}
	var updatedFiles []any
	changed := false
	for index, file := range files {
		fileMap, mapOK := file.(map[string]any)
		if !mapOK {
			continue
		}
		fileName, hasFileName := nonEmptyStringParam(fileMap, "file_name")
		if !hasFileName {
			continue
		}
		if updatedFiles == nil {
			updatedFiles = append([]any(nil), files...)
		}
		updatedFile := maps.Clone(fileMap)
		if _, hasFilePath := updatedFile["file_path"]; !hasFilePath {
			updatedFile["file_path"] = fileName
		}
		delete(updatedFile, "file_name")
		updatedFiles[index] = updatedFile
		changed = true
	}
	if changed {
		clone()["files"] = updatedFiles
	}
	return changed
}

func stripSnippetCreateFileActions(clone func() map[string]any, params map[string]any) bool {
	files, ok := params["files"].([]any)
	if !ok || len(files) == 0 {
		return false
	}
	var updatedFiles []any
	changed := false
	for index, file := range files {
		fileMap, mapOK := file.(map[string]any)
		if !mapOK {
			continue
		}
		action, hasAction := fileMap["action"]
		if !hasAction || !isCreateFileAction(action) {
			continue
		}
		if updatedFiles == nil {
			updatedFiles = append([]any(nil), files...)
		}
		updatedFile := maps.Clone(fileMap)
		delete(updatedFile, "action")
		updatedFiles[index] = updatedFile
		changed = true
	}
	if changed {
		clone()["files"] = updatedFiles
	}
	return changed
}

func isCreateFileAction(value any) bool {
	text, ok := value.(string)
	return ok && strings.EqualFold(strings.TrimSpace(text), "create")
}

func issueStateEventValue(value any) (string, bool) {
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "close", "closed":
		return "close", true
	case "reopen", "open", "opened":
		return "reopen", true
	default:
		return "", false
	}
}

// IssueStateEventValue normalizes historical issue state event spellings.
func IssueStateEventValue(value any) (string, bool) {
	return issueStateEventValue(value)
}

func actionSchemaProperties(schema map[string]any) map[string]any {
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}
	return properties
}

func gitlabAccessLevelValue(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return validGitLabAccessLevel(typed)
	case int64:
		return validGitLabAccessLevel(int(typed))
	case float64:
		accessLevel := int(typed)
		if typed == float64(accessLevel) {
			return validGitLabAccessLevel(accessLevel)
		}
		return 0, false
	}
	text, ok := value.(string)
	if !ok {
		return 0, false
	}
	normalized := strings.ToLower(strings.TrimSpace(text))
	if accessLevel, err := strconv.Atoi(normalized); err == nil {
		switch accessLevel {
		case 10, 20, 30, 40, 50:
			return accessLevel, true
		default:
			return 0, false
		}
	}
	switch normalized {
	case "guest":
		return 10, true
	case "reporter":
		return 20, true
	case "developer":
		return 30, true
	case "maintainer":
		return 40, true
	case "owner":
		return 50, true
	default:
		return 0, false
	}
}

// GitLabAccessLevelValue normalizes GitLab access level labels and numbers.
func GitLabAccessLevelValue(value any) (int, bool) {
	return gitlabAccessLevelValue(value)
}

func validGitLabAccessLevel(accessLevel int) (int, bool) {
	switch accessLevel {
	case 10, 20, 30, 40, 50:
		return accessLevel, true
	default:
		return 0, false
	}
}

func boolStringValue(value any) (parsed, ok bool) {
	text, ok := value.(string)
	if !ok {
		return false, false
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(text))
	if err != nil {
		return false, false
	}
	return parsed, true
}

// BoolStringValue parses historical string booleans for bool parameters.
func BoolStringValue(value any) (parsed, ok bool) {
	return boolStringValue(value)
}
