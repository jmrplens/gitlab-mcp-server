package actioncompat

import (
	"maps"
	"reflect"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const parameterAliasExplanationSource = "dynamic_action_scoped"

const (
	reasonNormalizeAccessLevel           = "normalized GitLab access level name to numeric level"
	reasonProtectedEnvAccessLevels       = "protected environment access levels must be arrays of objects with numeric access_level"
	reasonProtectedEnvApprovalCount      = "protected environment approval counts belong in approval_rules.required_approvals"
	reasonProtectedEnvApprovalRules      = "protected environment approval rules must be arrays of objects with numeric access_level and required_approvals"
	reasonPaginationBoolean              = "pagination limit parameters use numeric values; true selects the default page size"
	reasonProtectedBranchName            = "group protected branch protect uses name for the branch or wildcard to protect"
	reasonProjectPushRuleUnsigned        = "project push rules use reject_unsigned_commits for unsigned commit rejection"
	reasonPipelineScheduleDescription    = "pipeline schedules use description as the display name"
	reasonReleaseCreateMessage           = "release creation uses description for release notes"
	reasonReleaseLinkParentTagName       = "release link actions use tag_name for the parent release"
	reasonReleaseLinkBatchURL            = "batch release link entries use url for the link target"
	reasonReleaseLinkBatchUnsupported    = "batch release link entries do not accept direct asset path fields"
	reasonIssueLinkSourceIssueIID        = "issue.link_create uses issue_iid for the source issue"
	reasonIssueLinkTargetIssueIID        = "issue.link_create uses target_issue_iid for the linked issue"
	reasonIssueLinkRelation              = "issue.link_create uses link_type for the link relationship"
	reasonIssueSpentTimeSummary          = "issue.spent_time_add uses summary for the time log note"
	reasonIssueTimeEstimateDuration      = "issue.time_estimate_set uses duration for the estimate value"
	reasonMergeRequestEmojiName          = "merge request emoji creation uses name for the emoji identifier"
	reasonMergeRequestEmojiUnsupported   = "merge request emoji creation does not accept stale time-tracking or awardable metadata params"
	reasonSnippetProjectCreateFiles      = "project snippet creation uses files entries in dynamic mode"
	reasonSnippetProjectCreateFilePath   = "snippet file entries use file_path"
	reasonSnippetProjectCreateNoAction   = "project snippet creation file entries do not include an action field"
	reasonFeatureFlagUserListNameRemoved = "feature flag user-list listing is project-scoped and does not accept a feature flag name"
	reasonTerraformStateName             = "Terraform state actions use name for the state identifier"
)

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

type paramNormalization struct {
	params       map[string]any
	fields       map[string]any
	out          map[string]any
	cloned       bool
	explanations []toolutil.ParamAliasExplanation
}

type paramNormalizer func(*paramNormalization)

var actionParamNormalizers = map[string]paramNormalizer{
	actionJobList: func(state *paramNormalization) {
		state.moveParam("status", "scope", "job.list uses scope for job status filtering")
	},
	actionRepositoryFileGet: func(state *paramNormalization) {
		state.moveParam("branch", "ref", "repository.file_get reads file content at a ref")
	},
	actionIssueLinkCreate:             normalizeIssueLinkCreateParams,
	actionIssueSpentTimeAdd:           func(state *paramNormalization) { state.moveParam("note", "summary", reasonIssueSpentTimeSummary) },
	actionIssueTimeEstimateSet:        func(state *paramNormalization) { state.moveParam("time", "duration", reasonIssueTimeEstimateDuration) },
	actionIssueUpdate:                 normalizeIssueUpdateParams,
	actionMergeRequestEmojiMRCreate:   normalizeMergeRequestEmojiCreateParams,
	actionPipelineScheduleCreate:      normalizePipelineScheduleParams,
	actionPipelineScheduleUpdate:      normalizePipelineScheduleParams,
	actionBranchProtect:               normalizeBranchProtectParams,
	actionGroupProtectedBranchProtect: normalizeGroupProtectedBranchProtectParams,
	actionGroupProtectedEnvProtect:    normalizeProtectedEnvironmentParams,
	actionGroupProtectedEnvUpdate:     normalizeProtectedEnvironmentParams,
	actionProjectProtectedEnvProtect:  normalizeProtectedEnvironmentParams,
	actionProjectProtectedEnvUpdate:   normalizeProtectedEnvironmentParams,
	actionProjectPushRuleAdd:          normalizeProjectPushRuleParams,
	actionProjectPushRuleEdit:         normalizeProjectPushRuleParams,
	actionFeatureFlagCreate: func(state *paramNormalization) {
		state.moveParam("new_version_flag", "version", "feature flag creation uses version for the flag API version")
	},
	actionFeatureFlagUserListList: func(state *paramNormalization) {
		state.removeRejectedParam("name", "removed", reasonFeatureFlagUserListNameRemoved)
	},
	actionGroupLabelUpdate:          normalizeGroupLabelUpdateParams,
	actionProjectMemberAdd:          normalizeProjectMemberAccessLevelParams,
	actionProjectMemberEdit:         normalizeProjectMemberAccessLevelParams,
	actionReleaseCreate:             func(state *paramNormalization) { state.moveParam("message", "description", reasonReleaseCreateMessage) },
	actionReleaseLinkCreate:         normalizeReleaseLinkTagNameParams,
	actionReleaseLinkDelete:         normalizeReleaseLinkTagNameParams,
	actionReleaseLinkGet:            normalizeReleaseLinkTagNameParams,
	actionReleaseLinkList:           normalizeReleaseLinkTagNameParams,
	actionReleaseLinkUpdate:         normalizeReleaseLinkTagNameParams,
	actionReleaseLinkCreateBatch:    normalizeReleaseLinkCreateBatchParams,
	actionRunnerUpdate:              normalizeRunnerUpdateParams,
	actionSnippetProjectCreate:      normalizeSnippetProjectCreateParams,
	actionAdminTerraformStateUnlock: normalizeTerraformStateNameParams,
}

func newParamNormalization(params, schema map[string]any) *paramNormalization {
	return &paramNormalization{params: params, fields: actionSchemaProperties(schema), out: params}
}

func (state *paramNormalization) clone() map[string]any {
	if !state.cloned {
		state.out = maps.Clone(state.params)
		state.cloned = true
	}
	return state.out
}

func (state *paramNormalization) accepts(name string) bool {
	_, ok := state.fields[name]
	return ok
}

func (state *paramNormalization) record(alias, target, reason string) {
	state.explanations = append(state.explanations, toolutil.ParamAliasExplanation{Alias: alias, Canonical: target, Source: parameterAliasExplanationSource, Notes: reason})
}

func (state *paramNormalization) moveParam(alias, target, reason string) {
	state.moveParamWithOptions(alias, target, reason, false)
}

func (state *paramNormalization) moveAcceptedAliasParam(alias, target, reason string) {
	state.moveParamWithOptions(alias, target, reason, true)
}

func (state *paramNormalization) moveParamWithOptions(alias, target, reason string, allowAcceptedAlias bool) {
	value, ok := state.out[alias]
	if !ok || !state.accepts(target) || (!allowAcceptedAlias && state.accepts(alias)) {
		return
	}
	if _, hasTarget := state.out[target]; hasTarget {
		if allowAcceptedAlias && alias != target {
			delete(state.clone(), alias)
			state.record(alias, target, reason)
		}
		return
	}
	updated := state.clone()
	updated[target] = value
	delete(updated, alias)
	state.record(alias, target, reason)
}

func (state *paramNormalization) copyParam(alias, target, reason string) {
	value, ok := state.out[alias]
	if !ok || !state.accepts(target) {
		return
	}
	if _, hasTarget := state.out[target]; hasTarget {
		return
	}
	state.clone()[target] = value
	state.record(alias, target, reason)
}

func (state *paramNormalization) removeRejectedParam(alias, target, reason string) {
	if _, ok := state.out[alias]; !ok || state.accepts(alias) {
		return
	}
	delete(state.clone(), alias)
	state.record(alias, target, reason)
}

// ParameterAliases returns historical action-scoped parameter aliases and
// coercion policies used by Dynamic execute compatibility normalization.
func ParameterAliases() []ParameterAlias {
	return cloneParameterAliases(defaultParameterAliases())
}

func defaultParameterAliases() []ParameterAlias {
	return []ParameterAlias{
		parameterAlias(actionJobList, "status", "scope", "job.list uses scope for job status filtering"),
		parameterAlias(actionRepositoryFileGet, "branch", "ref", "repository.file_get reads file content at a ref"),
		parameterAlias(actionIssueLinkCreate, "source_issue_iid", "issue_iid", reasonIssueLinkSourceIssueIID),
		parameterAlias(actionIssueLinkCreate, "linked_issue_iid", "target_issue_iid", reasonIssueLinkTargetIssueIID),
		parameterAlias(actionIssueLinkCreate, "project_id", "target_project_id", "same-project issue links reuse project_id as target_project_id"),
		parameterAlias(actionIssueLinkCreate, "relation", "link_type", reasonIssueLinkRelation),
		parameterAlias(actionIssueLinkCreate, "type", "link_type", reasonIssueLinkRelation),
		parameterAlias(actionIssueSpentTimeAdd, "note", "summary", reasonIssueSpentTimeSummary),
		parameterAlias(actionIssueTimeEstimateSet, "time", "duration", reasonIssueTimeEstimateDuration),
		parameterAlias(actionIssueUpdate, "state_event", "state_event", "normalized issue state event value"),
		parameterAlias(actionMergeRequestEmojiMRCreate, "emoji", "name", reasonMergeRequestEmojiName),
		normalizerOnlyParameterAlias(actionMergeRequestEmojiMRCreate, "duration", "removed", reasonMergeRequestEmojiUnsupported),
		normalizerOnlyParameterAlias(actionMergeRequestEmojiMRCreate, "awardable_type", "removed", reasonMergeRequestEmojiUnsupported),
		parameterAlias(actionPipelineScheduleCreate, "name", "description", reasonPipelineScheduleDescription),
		parameterAlias(actionPipelineScheduleUpdate, "name", "description", reasonPipelineScheduleDescription),
		parameterAlias(actionBranchProtect, "push_access_level", "push_access_level", reasonNormalizeAccessLevel),
		parameterAlias(actionBranchProtect, "merge_access_level", "merge_access_level", reasonNormalizeAccessLevel),
		parameterAlias(actionGroupProtectedBranchProtect, "branch", "name", reasonProtectedBranchName),
		parameterAlias(actionGroupProtectedBranchProtect, "push_access_level", "push_access_level", reasonNormalizeAccessLevel),
		parameterAlias(actionGroupProtectedBranchProtect, "merge_access_level", "merge_access_level", reasonNormalizeAccessLevel),
		parameterAlias(actionGroupProtectedEnvProtect, "deploy_access_levels", "deploy_access_levels", reasonProtectedEnvAccessLevels),
		parameterAlias(actionGroupProtectedEnvProtect, "approval_rules", "approval_rules", reasonProtectedEnvApprovalRules),
		normalizerOnlyParameterAlias(actionGroupProtectedEnvProtect, "required_approval_count", "approval_rules", reasonProtectedEnvApprovalCount),
		parameterAlias(actionGroupProtectedEnvUpdate, "deploy_access_levels", "deploy_access_levels", reasonProtectedEnvAccessLevels),
		parameterAlias(actionGroupProtectedEnvUpdate, "approval_rules", "approval_rules", reasonProtectedEnvApprovalRules),
		normalizerOnlyParameterAlias(actionGroupProtectedEnvUpdate, "required_approval_count", "approval_rules", reasonProtectedEnvApprovalCount),
		parameterAlias(actionProjectProtectedEnvProtect, "deploy_access_levels", "deploy_access_levels", reasonProtectedEnvAccessLevels),
		parameterAlias(actionProjectProtectedEnvProtect, "approval_rules", "approval_rules", reasonProtectedEnvApprovalRules),
		normalizerOnlyParameterAlias(actionProjectProtectedEnvProtect, "required_approval_count", "approval_rules", reasonProtectedEnvApprovalCount),
		parameterAlias(actionProjectProtectedEnvUpdate, "deploy_access_levels", "deploy_access_levels", reasonProtectedEnvAccessLevels),
		parameterAlias(actionProjectProtectedEnvUpdate, "approval_rules", "approval_rules", reasonProtectedEnvApprovalRules),
		normalizerOnlyParameterAlias(actionProjectProtectedEnvUpdate, "required_approval_count", "approval_rules", reasonProtectedEnvApprovalCount),
		parameterAlias(actionProjectPushRuleAdd, "deny_unsigned_commits", "reject_unsigned_commits", reasonProjectPushRuleUnsigned),
		parameterAlias(actionProjectPushRuleEdit, "deny_unsigned_commits", "reject_unsigned_commits", reasonProjectPushRuleUnsigned),
		parameterAlias(actionFeatureFlagCreate, "new_version_flag", "version", "feature flag creation uses version for the flag API version"),
		normalizerOnlyParameterAlias(actionFeatureFlagUserListList, "name", "removed", reasonFeatureFlagUserListNameRemoved),
		parameterAlias(actionGroupLabelUpdate, "name", "new_name", "group label update renames labels with new_name"),
		parameterAlias(actionProjectMemberAdd, "access_level", "access_level", reasonNormalizeAccessLevel),
		parameterAlias(actionProjectMemberEdit, "access_level", "access_level", reasonNormalizeAccessLevel),
		parameterAlias(actionReleaseCreate, "message", "description", reasonReleaseCreateMessage),
		parameterAlias(actionReleaseLinkCreate, "release_tag_name", "tag_name", reasonReleaseLinkParentTagName),
		parameterAlias(actionReleaseLinkCreateBatch, "links.link_url", "links.url", reasonReleaseLinkBatchURL),
		normalizerOnlyParameterAlias(actionReleaseLinkCreateBatch, "links.filepath", "links", reasonReleaseLinkBatchUnsupported),
		normalizerOnlyParameterAlias(actionReleaseLinkCreateBatch, "links.direct_asset_path", "links", reasonReleaseLinkBatchUnsupported),
		parameterAlias(actionReleaseLinkDelete, "release_tag_name", "tag_name", reasonReleaseLinkParentTagName),
		parameterAlias(actionReleaseLinkGet, "release_tag_name", "tag_name", reasonReleaseLinkParentTagName),
		parameterAlias(actionReleaseLinkList, "release_tag_name", "tag_name", reasonReleaseLinkParentTagName),
		parameterAlias(actionReleaseLinkUpdate, "release_tag_name", "tag_name", reasonReleaseLinkParentTagName),
		parameterAlias(actionRunnerUpdate, "paused", "paused", "normalized string boolean to bool"),
		parameterAlias(actionSnippetProjectCreate, "file_name/content", "files", reasonSnippetProjectCreateFiles),
		parameterAlias(actionSnippetProjectCreate, "files.file_name", "files.file_path", reasonSnippetProjectCreateFilePath),
		parameterAlias(actionSnippetProjectCreate, "files.action", "files", reasonSnippetProjectCreateNoAction),
		parameterAlias(actionAdminTerraformStateUnlock, "id", "name", reasonTerraformStateName),
		parameterAlias(actionAdminTerraformStateUnlock, "state", "name", reasonTerraformStateName),
		parameterAlias(actionAdminTerraformStateUnlock, "state_name", "name", reasonTerraformStateName),
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
	state := newParamNormalization(params, schema)
	if normalizer := actionParamNormalizers[actionID]; normalizer != nil {
		normalizer(state)
	}
	normalizeCommonParams(state)
	return state.out, state.explanations
}

func normalizeCommonParams(state *paramNormalization) {
	for _, name := range []string{"first", "last", "per_page", "page"} {
		normalizePaginationBooleanParam(state, name)
	}
}

func normalizePaginationBooleanParam(state *paramNormalization, name string) {
	value, ok := state.out[name]
	if !ok || !state.accepts(name) {
		return
	}
	boolValue, ok := value.(bool)
	if !ok {
		return
	}
	updated := state.clone()
	if !boolValue {
		delete(updated, name)
		state.record(name, name, reasonPaginationBoolean)
		return
	}
	updated[name] = defaultPaginationValue(name)
	state.record(name, name, reasonPaginationBoolean)
}

func defaultPaginationValue(name string) int {
	if name == "page" {
		return 1
	}
	return 100
}

func normalizeIssueLinkCreateParams(state *paramNormalization) {
	state.moveParam("source_issue_iid", "issue_iid", reasonIssueLinkSourceIssueIID)
	state.moveParam("linked_issue_iid", "target_issue_iid", reasonIssueLinkTargetIssueIID)
	state.copyParam("project_id", "target_project_id", "same-project issue links reuse project_id as target_project_id")
	state.moveParam("relation", "link_type", reasonIssueLinkRelation)
	state.moveParam("type", "link_type", reasonIssueLinkRelation)
}

func normalizeIssueUpdateParams(state *paramNormalization) {
	value, ok := state.out["state_event"]
	if !ok || !state.accepts("state_event") {
		return
	}
	stateEvent, converted := issueStateEventValue(value)
	if !converted {
		return
	}
	state.clone()["state_event"] = stateEvent
	state.record("state_event", "state_event", "normalized issue state event value")
}

func normalizeMergeRequestEmojiCreateParams(state *paramNormalization) {
	state.moveParam("emoji", "name", reasonMergeRequestEmojiName)
	state.removeRejectedParam("duration", "removed", reasonMergeRequestEmojiUnsupported)
	state.removeRejectedParam("awardable_type", "removed", reasonMergeRequestEmojiUnsupported)
}

func normalizePipelineScheduleParams(state *paramNormalization) {
	state.moveAcceptedAliasParam("name", "description", reasonPipelineScheduleDescription)
}

func normalizeBranchProtectParams(state *paramNormalization) {
	for _, name := range []string{"push_access_level", "merge_access_level"} {
		normalizeAccessLevelParamWith(state, name, gitLabBranchProtectionAccessLevelValue)
	}
}

func normalizeGroupProtectedBranchProtectParams(state *paramNormalization) {
	state.moveParam("branch", "name", reasonProtectedBranchName)
	for _, name := range []string{"push_access_level", "merge_access_level"} {
		normalizeAccessLevelParamWith(state, name, gitLabBranchProtectionAccessLevelValue)
	}
}

func normalizeProjectPushRuleParams(state *paramNormalization) {
	state.moveParam("deny_unsigned_commits", "reject_unsigned_commits", reasonProjectPushRuleUnsigned)
}

func normalizeProtectedEnvironmentParams(state *paramNormalization) {
	if state.accepts("deploy_access_levels") && normalizeProtectedEnvironmentAccessEntries(state, "deploy_access_levels", reasonProtectedEnvAccessLevels) {
		state.record("deploy_access_levels", "deploy_access_levels", reasonProtectedEnvAccessLevels)
	}
	if state.accepts("approval_rules") && normalizeProtectedEnvironmentAccessEntries(state, "approval_rules", reasonProtectedEnvApprovalRules) {
		state.record("approval_rules", "approval_rules", reasonProtectedEnvApprovalRules)
	}
	if state.accepts("approval_rules") {
		normalizeProtectedEnvironmentApprovalCount(state)
	}
}

func normalizeProtectedEnvironmentApprovalCount(state *paramNormalization) {
	value, ok := state.out["required_approval_count"]
	if !ok {
		return
	}
	if _, hasRules := state.out["approval_rules"]; hasRules {
		updated := state.clone()
		delete(updated, "required_approval_count")
		state.record("required_approval_count", "approval_rules", reasonProtectedEnvApprovalCount)
		return
	}
	count, converted := integerValue(value)
	if !converted {
		return
	}
	updated := state.clone()
	delete(updated, "required_approval_count")
	updated["approval_rules"] = []any{map[string]any{"access_level": 40, "required_approvals": count}}
	state.record("required_approval_count", "approval_rules", reasonProtectedEnvApprovalCount)
}

func normalizeProtectedEnvironmentAccessEntries(state *paramNormalization, name, _ string) bool {
	value, ok := state.out[name]
	if !ok {
		return false
	}
	normalized, changed := protectedEnvironmentAccessEntries(value, name == "approval_rules")
	if !changed {
		return false
	}
	state.clone()[name] = normalized
	return true
}

func protectedEnvironmentAccessEntries(value any, approvalRules bool) (any, bool) {
	if entry, ok := protectedEnvironmentPrimitiveAccessEntry(value); ok {
		return []any{entry}, true
	}
	if entry, ok := value.(map[string]any); ok {
		return []any{normalizeProtectedEnvironmentAccessEntry(entry, approvalRules)}, true
	}
	items, ok := value.([]any)
	if !ok {
		return value, false
	}
	updatedItems := make([]any, len(items))
	changed := false
	for index, item := range items {
		switch typed := item.(type) {
		case map[string]any:
			updatedItem := normalizeProtectedEnvironmentAccessEntry(typed, approvalRules)
			updatedItems[index] = updatedItem
			changed = changed || !reflect.DeepEqual(typed, updatedItem)
		default:
			entry, entryOK := protectedEnvironmentPrimitiveAccessEntry(typed)
			if !entryOK {
				updatedItems[index] = item
				continue
			}
			updatedItems[index] = entry
			changed = true
		}
	}
	if !changed {
		return value, false
	}
	return updatedItems, true
}

func protectedEnvironmentPrimitiveAccessEntry(value any) (map[string]any, bool) {
	accessLevel, ok := environmentAccessLevelValue(value)
	if !ok {
		return nil, false
	}
	return map[string]any{"access_level": accessLevel}, true
}

func normalizeProtectedEnvironmentAccessEntry(entry map[string]any, approvalRules bool) map[string]any {
	updated := maps.Clone(entry)
	if _, hasAccessLevel := updated["access_level"]; !hasAccessLevel {
		for _, alias := range []string{"deploy_access_level", "group_access_level", "project_access_level", "machine_user_access_level"} {
			value, ok := updated[alias]
			if !ok {
				continue
			}
			if accessLevel, converted := environmentAccessLevelValue(value); converted {
				updated["access_level"] = accessLevel
			}
			delete(updated, alias)
			break
		}
	} else if accessLevel, converted := environmentAccessLevelValue(updated["access_level"]); converted {
		updated["access_level"] = accessLevel
	}
	if approvalRules {
		for _, alias := range []string{"required_approval_count", "approval_count", "approvals_required"} {
			value, ok := updated[alias]
			if !ok {
				continue
			}
			if _, hasRequired := updated["required_approvals"]; !hasRequired {
				updated["required_approvals"] = value
			}
			delete(updated, alias)
		}
		if _, hasRequired := updated["required_approvals"]; hasRequired && !protectedEnvironmentAccessEntryHasPrincipal(updated) {
			updated["access_level"] = 40
		}
	}
	return updated
}

func protectedEnvironmentAccessEntryHasPrincipal(entry map[string]any) bool {
	for _, name := range []string{"access_level", "user_id", "group_id"} {
		if _, ok := entry[name]; ok {
			return true
		}
	}
	return false
}

func normalizeProjectMemberAccessLevelParams(state *paramNormalization) {
	normalizeAccessLevelParam(state, "access_level")
}

func normalizeAccessLevelParam(state *paramNormalization, name string) {
	normalizeAccessLevelParamWith(state, name, gitlabAccessLevelValue)
}

func normalizeAccessLevelParamWith(state *paramNormalization, name string, convert func(any) (int, bool)) {
	value, ok := state.out[name]
	if !ok || !state.accepts(name) {
		return
	}
	accessLevel, converted := convert(value)
	if !converted {
		return
	}
	state.clone()[name] = accessLevel
	state.record(name, name, reasonNormalizeAccessLevel)
}

func normalizeTerraformStateNameParams(state *paramNormalization) {
	state.moveParam("state_name", "name", reasonTerraformStateName)
	state.moveParam("state", "name", reasonTerraformStateName)
	state.moveParam("id", "name", reasonTerraformStateName)
}

func normalizeGroupLabelUpdateParams(state *paramNormalization) {
	state.moveWithoutSchemaCheck("name", "new_name", "group label update renames labels with new_name")
}

func (state *paramNormalization) moveWithoutSchemaCheck(alias, target, reason string) {
	value, ok := state.out[alias]
	if !ok {
		return
	}
	if _, hasTarget := state.out[target]; hasTarget {
		return
	}
	updated := state.clone()
	updated[target] = value
	delete(updated, alias)
	state.record(alias, target, reason)
}

func normalizeReleaseLinkTagNameParams(state *paramNormalization) {
	state.moveParam("release_tag_name", "tag_name", reasonReleaseLinkParentTagName)
}

func normalizeReleaseLinkCreateBatchParams(state *paramNormalization) {
	if state.accepts("links") {
		normalizeReleaseLinkBatchEntries(state.clone, state.out, state.record)
	}
}

func normalizeRunnerUpdateParams(state *paramNormalization) {
	value, ok := state.out["paused"]
	if !ok || !state.accepts("paused") {
		return
	}
	paused, converted := boolStringValue(value)
	if !converted {
		return
	}
	state.clone()["paused"] = paused
	state.record("paused", "paused", "normalized string boolean to bool")
}

func normalizeSnippetProjectCreateParams(state *paramNormalization) {
	if !state.accepts("files") {
		return
	}
	if (!state.accepts("file_name") || !state.accepts("content")) && buildSnippetCreateFilesFromSingleFileParams(state.clone, state.out) {
		state.record("file_name/content", "files", reasonSnippetProjectCreateFiles)
	}
	if normalizeSnippetFileNameFields(state.clone, state.out) {
		state.record("files.file_name", "files.file_path", reasonSnippetProjectCreateFilePath)
	}
	if stripSnippetCreateFileActions(state.clone, state.out) {
		state.record("files.action", "files", reasonSnippetProjectCreateNoAction)
	}
}

func normalizeReleaseLinkBatchEntries(clone func() map[string]any, params map[string]any, record func(alias, target, reason string)) bool {
	links, ok := params["links"].([]any)
	if !ok || len(links) == 0 {
		return false
	}
	var updatedLinks []any
	changed := false
	recorded := make(map[string]bool)
	recordOnce := func(alias, target, reason string) {
		key := alias + "->" + target
		if recorded[key] {
			return
		}
		recorded[key] = true
		record(alias, target, reason)
	}
	for index, link := range links {
		linkMap, mapOK := link.(map[string]any)
		if !mapOK {
			continue
		}
		updatedLink := maps.Clone(linkMap)
		linkChanged := false
		if value, hasLinkURL := updatedLink["link_url"]; hasLinkURL {
			if _, hasURL := updatedLink["url"]; !hasURL {
				updatedLink["url"] = value
			}
			delete(updatedLink, "link_url")
			linkChanged = true
			recordOnce("links.link_url", "links.url", reasonReleaseLinkBatchURL)
		}
		for _, unsupported := range []string{"filepath", "direct_asset_path"} {
			if _, hasUnsupported := updatedLink[unsupported]; hasUnsupported {
				delete(updatedLink, unsupported)
				linkChanged = true
				recordOnce("links."+unsupported, "links", reasonReleaseLinkBatchUnsupported)
			}
		}
		if !linkChanged {
			continue
		}
		if updatedLinks == nil {
			updatedLinks = append([]any(nil), links...)
		}
		updatedLinks[index] = updatedLink
		changed = true
	}
	if changed {
		clone()["links"] = updatedLinks
	}
	return changed
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
	case "guest", "guests":
		return 10, true
	case "reporter", "reporters":
		return 20, true
	case "developer", "developers":
		return 30, true
	case "maintainer", "maintainers":
		return 40, true
	case "owner", "owners":
		return 50, true
	default:
		return 0, false
	}
}

func environmentAccessLevelValue(value any) (int, bool) {
	if accessLevel, ok := integerValue(value); ok {
		switch accessLevel {
		case 0, 10, 20, 30, 40, 50, 60:
			return accessLevel, true
		default:
			return 0, false
		}
	}
	text, ok := value.(string)
	if !ok {
		return 0, false
	}
	normalized := strings.ToLower(strings.TrimSpace(strings.NewReplacer("_", " ", "-", " ").Replace(text)))
	switch normalized {
	case "no access", "no one", "nobody", "none":
		return 0, true
	case "guest", "guests":
		return 10, true
	case "reporter", "reporters":
		return 20, true
	case "developer", "developers":
		return 30, true
	case "maintainer", "maintainers":
		return 40, true
	case "owner", "owners":
		return 50, true
	case "admin", "admins", "administrator", "administrators":
		return 60, true
	default:
		return 0, false
	}
}

func integerValue(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		integer := int(typed)
		return integer, typed == float64(integer)
	case string:
		integer, err := strconv.Atoi(strings.TrimSpace(typed))
		return integer, err == nil
	default:
		return 0, false
	}
}

func gitLabBranchProtectionAccessLevelValue(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return validGitLabBranchProtectionAccessLevel(typed)
	case int64:
		return validGitLabBranchProtectionAccessLevel(int(typed))
	case float64:
		accessLevel := int(typed)
		if typed == float64(accessLevel) {
			return validGitLabBranchProtectionAccessLevel(accessLevel)
		}
		return 0, false
	}
	text, ok := value.(string)
	if !ok {
		return 0, false
	}
	normalized := strings.ToLower(strings.TrimSpace(text))
	if accessLevel, err := strconv.Atoi(normalized); err == nil {
		return validGitLabBranchProtectionAccessLevel(accessLevel)
	}
	normalized = strings.NewReplacer("_", " ", "-", " ").Replace(normalized)
	switch normalized {
	case "developer", "developers":
		return 30, true
	case "maintainer", "maintainers":
		return 40, true
	case "no access", "no one", "nobody", "none":
		return 0, true
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

func validGitLabBranchProtectionAccessLevel(accessLevel int) (int, bool) {
	switch accessLevel {
	case 0, 30, 40:
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
