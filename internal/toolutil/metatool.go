package toolutil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/url"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// maxInt is the maximum int value; used for overflow-safe capacity calculations.
const maxInt = int(math.MaxInt)

const (
	minInt64AsFloat                      = -float64(1 << 63)
	maxInt64AsFloat                      = float64(1<<63 - 1)
	invalidActionParamsError             = "invalid params for this action: %w"
	actionProjectMemberDelete            = "project.member_delete"
	actionProjectMemberEdit              = "project.member_edit"
	actionExternalStatusCheckListProject = "external_status_check.list_project"
	actionFeatureFlagUserListList        = "feature_flags.ff_user_list_list"
)

// MetaToolInput is the common input for all meta-tools.
// The LLM sends an action name and a params object; the dispatcher
// routes to the underlying handler function and deserializes params
// into the action-specific input struct.
type MetaToolInput struct {
	Action string         `json:"action" jsonschema:"Action to perform. See the tool description for available actions and their parameters."`
	Params map[string]any `json:"params,omitempty" jsonschema:"Action-specific parameters as a JSON object. See the tool description for required/optional fields per action."`
}

// ActionFunc is a handler that receives raw params and returns a result or error.
type ActionFunc func(ctx context.Context, params map[string]any) (any, error)

// ActionRoute pairs an action handler with metadata about its behavior.
// Used by meta-tools to carry per-route destructive classification
// without string parsing. OutputSchema holds the JSON Schema for the
// action's typed output. InputSchema holds the JSON Schema for the action's
// typed params (nil for routes constructed via the untyped Route and
// DestructiveRoute constructors).
type ActionRoute struct {
	Handler           ActionFunc
	Destructive       bool
	InputType         reflect.Type
	InputSchema       map[string]any
	OutputSchema      map[string]any
	ParameterGuidance map[string]ParameterGuidance
	Aliases           []string
	Tags              []string
	Usage             string
	RelatedActions    []string
}

// ParameterGuidance carries compact model-facing hints for parameters that are
// easy to confuse across similar GitLab actions.
type ParameterGuidance struct {
	SemanticRole     string   `json:"semantic_role,omitempty"`
	ValueSource      string   `json:"value_source,omitempty"`
	CommonConfusions []string `json:"common_confusions,omitempty"`
	ExampleBinding   string   `json:"example_binding,omitempty"`
}

// WithParameterGuidance returns a copy of route with merged parameter guidance.
func (route ActionRoute) WithParameterGuidance(guidance map[string]ParameterGuidance) ActionRoute {
	route.ParameterGuidance = mergeActionSpecGuidance(route.ParameterGuidance, guidance)
	return route
}

// WithAliases returns a copy of route with additional search aliases.
func (route ActionRoute) WithAliases(aliases ...string) ActionRoute {
	route.Aliases = appendNormalizedRouteStrings(route.Aliases, aliases...)
	return route
}

// WithTags returns a copy of route with additional search tags.
func (route ActionRoute) WithTags(tags ...string) ActionRoute {
	route.Tags = appendNormalizedRouteStrings(route.Tags, tags...)
	return route
}

// WithUsage returns a copy of route with a short model-facing usage hint.
func (route ActionRoute) WithUsage(usage string) ActionRoute {
	route.Usage = strings.TrimSpace(usage)
	return route
}

// WithRelatedActions returns a copy of route with related canonical action IDs.
func (route ActionRoute) WithRelatedActions(actions ...string) ActionRoute {
	route.RelatedActions = appendNormalizedRouteStrings(route.RelatedActions, actions...)
	return route
}

// ActionMap maps action names to their route definitions (handler + metadata).
type ActionMap map[string]ActionRoute

// #nosec G101 -- Alias keys and values are MCP action route names, not credentials.
var commonActionAliases = map[string]string{
	"badge.add":                "project.badge_add",
	"hook.add":                 "project.hook_add",
	"milestone.create":         "project.milestone_create",
	"milestone.delete":         "project.milestone_delete",
	"milestone.get":            "project.milestone_get",
	"milestone.list":           "project.milestone_list",
	"milestone.update":         "project.milestone_update",
	"group.custom_emoji_list":  "custom_emoji.list",
	"me":                       "current",
	"broadcast_message.create": "admin.broadcast_message_create",
	"broadcast_message.delete": "admin.broadcast_message_delete",
	"ci_job_token_scope.inbound_allowlist.list":  "job.token_scope_list_inbound",
	"deploy_key.create":                          "access.deploy_key_add",
	"deploy_key.list":                            "access.deploy_key_list_project",
	"deploy_token.create":                        "access.deploy_token_create_project",
	"deploy_token.delete":                        "access.deploy_token_delete_project",
	"deploy_token.get":                           "access.deploy_token_get_project",
	"deploy_token.list":                          "access.deploy_token_list_project",
	"branch.update_protection":                   "branch.update_protected",
	"merge_request.changes":                      "mr_review.changes_get",
	"merge_request.emoji_award_create":           "merge_request.emoji_mr_create",
	"merge_request.emoji_award_delete":           "merge_request.emoji_mr_delete",
	"merge_request.emoji_mr_award_create":        "merge_request.emoji_mr_create",
	"merge_request.emoji_mr_award_delete":        "merge_request.emoji_mr_delete",
	"merge_request_note.create":                  "mr_review.note_create",
	"merge_request_note.delete":                  "mr_review.note_delete",
	"merge_request_note.get":                     "mr_review.note_get",
	"merge_request_note.update":                  "mr_review.note_update",
	"mr_review.draft_notes_publish":              "mr_review.draft_note_publish_all",
	"mr_review.publish":                          "mr_review.draft_note_publish_all",
	"package.files":                              "package.file_list",
	"package.list_generic":                       "package.list",
	"project.releases.list":                      "release.list",
	"project.hooks.list":                         "project.hook_list",
	"project.member_remove":                      actionProjectMemberDelete,
	"project.member_update":                      actionProjectMemberEdit,
	"project.schedule_storage_move":              "storage_move.schedule_project",
	"project.status_check_list":                  actionExternalStatusCheckListProject,
	"project.status_checks.list":                 actionExternalStatusCheckListProject,
	"project_member.add":                         "project.member_add",
	"project_member.delete":                      actionProjectMemberDelete,
	"project_member.edit":                        actionProjectMemberEdit,
	"project_member.get":                         "project.member_get",
	"project_member.remove":                      actionProjectMemberDelete,
	"project_member.update":                      actionProjectMemberEdit,
	"external_status_check.list_project_checks":  actionExternalStatusCheckListProject,
	"feature_flag_user_list.create":              "feature_flags.ff_user_list_create",
	"feature_flag_user_list.delete":              "feature_flags.ff_user_list_delete",
	"feature_flag_user_list.get":                 "feature_flags.ff_user_list_get",
	"feature_flag_user_list.list":                actionFeatureFlagUserListList,
	"feature_flag_user_list.update":              "feature_flags.ff_user_list_update",
	"feature_flags.feature_flag_user_list":       actionFeatureFlagUserListList,
	"feature_flags.feature_flag_user_list_list":  actionFeatureFlagUserListList,
	"feature_flags.feature_flag_user_lists_list": actionFeatureFlagUserListList,
	"gitlab_issue.create":                        "issue.create",
	"gitlab_issue.delete":                        "issue.delete",
	"gitlab_server.health_check":                 "server.health_check",
	"group.audit_events":                         "audit_event.list_group",
	"issue.link":                                 "issue.link_create",
	"issue.note.create":                          "issue.note_create",
	"issue.note.delete":                          "issue.note_delete",
	"issue.note.get":                             "issue.note_get",
	"issue.note.list":                            "issue.note_list",
	"issue.note.update":                          "issue.note_update",
	"job.artifact_download":                      "job.download_single_artifact",
	"group.variable.create":                      "ci_variable.group_create",
	"merge_request.add_spent_time":               "merge_request.spent_time_add",
	"merge_request.set_time_estimate":            "merge_request.time_estimate_set",
	"merge_request.time_estimate":                "merge_request.time_estimate_set",
	"merge_request.time_spent_add":               "merge_request.spent_time_add",
	"release.asset_link.create":                  "release.link_create",
	"release.create_link":                        "release.link_create",
	"release_link.link_list":                     "release.link_list",
	"release.generate_notes":                     "analyze.release_notes",
	"repository_tree":                            "repository.tree",
	"repository_tree.list":                       "repository.tree",
	"repository_file.get":                        "repository.file_get",
	"repository_file.read":                       "repository.file_get",
	"repository_files.get_raw_file":              "repository.file_raw",
	"pipeline.schedule_variable_create":          "pipeline.schedule_create_variable",
	"pipeline.schedule_variable_delete":          "pipeline.schedule_delete_variable",
	"pipeline.schedule_variable_update":          "pipeline.schedule_edit_variable",
	"project.badge_update":                       "project.badge_edit",
	"merge_request.time_spent_reset":             "merge_request.spent_time_reset",
	"generic_package.list":                       "package.list",
	"issue_note.create":                          "issue.note_create",
	"issue_note.delete":                          "issue.note_delete",
	"issue_note.get":                             "issue.note_get",
	"issue_note.list":                            "issue.note_list",
	"issue_note.update":                          "issue.note_update",
	"gitlab_interactive_issue.create":            "interactive.issue_create",
	"group_board_list":                           "epic_board_list",
	"epic_discussion_note_update":                "epic_discussion_update_note",
	"epic_discussion_note_delete":                "epic_discussion_delete_note",
	"variable.create":                            "ci_variable.create",
	"webhook.add":                                "project.hook_add",
}

// NormalizeActionAlias returns the canonical action name for common shortened
// action spellings when the canonical action exists on the target meta-tool.
func NormalizeActionAlias(action string, routes ActionMap) string {
	if action == "" {
		return action
	}
	if canonical, ok := commonActionAliases[action]; ok {
		if _, exists := routes[canonical]; exists {
			return canonical
		}
	}
	return action
}

// NormalizeActionAliasForParams returns the canonical action name for aliases
// that depend on the submitted parameter shape.
func NormalizeActionAliasForParams(toolName, action string, params map[string]any, routes ActionMap) string {
	return normalizeActionAliasForParams(toolName, action, params, routes)
}

func normalizeActionAliasForParams(toolName, action string, params map[string]any, routes ActionMap) string {
	if toolName != "gitlab_environment" || action != "get" {
		return action
	}
	if _, exists := routes["protected_get"]; !exists || !hasProtectedEnvironmentNameParam(params) {
		return action
	}
	return "protected_get"
}

func hasProtectedEnvironmentNameParam(params map[string]any) bool {
	if len(params) == 0 {
		return false
	}
	if _, ok := params["environment"]; ok {
		return true
	}
	value, ok := params["environment_id"].(string)
	if !ok {
		return false
	}
	_, err := integerFromString(strings.TrimSpace(value))
	return err != nil
}

// ParamValidationError marks parameter decoding failures that should be
// surfaced as recoverable tool errors instead of protocol errors.
type ParamValidationError struct {
	Err error
}

// Error returns the underlying validation error message.
func (e *ParamValidationError) Error() string {
	if e == nil || e.Err == nil {
		return "invalid params"
	}
	return e.Err.Error()
}

// Unwrap returns the underlying validation error.
func (e *ParamValidationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func cloneRouteStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func appendNormalizedRouteStrings(existing []string, values ...string) []string {
	merged := make([]string, 0, len(existing)+len(values))
	seen := make(map[string]struct{}, len(existing)+len(values))
	for _, value := range append(cloneRouteStrings(existing), values...) {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		merged = append(merged, value)
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func cloneParameterGuidanceMap(guidance map[string]ParameterGuidance) map[string]ParameterGuidance {
	if len(guidance) == 0 {
		return nil
	}
	out := make(map[string]ParameterGuidance, len(guidance))
	for name, item := range guidance {
		item.CommonConfusions = append([]string(nil), item.CommonConfusions...)
		out[name] = item
	}
	return out
}

// Route creates a non-destructive ActionRoute without an output schema.
func Route(fn ActionFunc) ActionRoute {
	return ActionRoute{Handler: fn, Destructive: false}
}

// DestructiveRoute creates a destructive ActionRoute without an output schema.
func DestructiveRoute(fn ActionFunc) ActionRoute {
	return ActionRoute{Handler: fn, Destructive: true}
}

// RouteFunc wraps a typed function as a non-destructive ActionRoute without a
// GitLab client dependency and attaches input and output schemas.
func RouteFunc[T, R any](fn func(ctx context.Context, input T) (R, error)) ActionRoute {
	inputType := reflect.TypeFor[T]()
	return ActionRoute{
		Handler: func(ctx context.Context, params map[string]any) (any, error) {
			input, err := UnmarshalParams[T](params)
			if err != nil {
				var zero R
				return zero, err
			}
			return fn(ctx, input)
		},
		Destructive:  false,
		InputType:    inputType,
		InputSchema:  inputSchemaForType(inputType),
		OutputSchema: schemaForType(reflect.TypeFor[R]()),
	}
}

// RouteRequestFunc wraps a typed request-aware function as a non-destructive
// ActionRoute without a GitLab client dependency and attaches schemas.
func RouteRequestFunc[T, R any](fn func(ctx context.Context, req *mcp.CallToolRequest, input T) (R, error)) ActionRoute {
	inputType := reflect.TypeFor[T]()
	return ActionRoute{
		Handler: func(ctx context.Context, params map[string]any) (any, error) {
			input, err := UnmarshalParams[T](params)
			if err != nil {
				var zero R
				return zero, err
			}
			return fn(ctx, RequestFromContext(ctx), input)
		},
		Destructive:  false,
		InputType:    inputType,
		InputSchema:  inputSchemaForType(inputType),
		OutputSchema: schemaForType(reflect.TypeFor[R]()),
	}
}

// DestructiveFunc wraps a typed function as a destructive ActionRoute without a
// GitLab client dependency and attaches input and output schemas.
func DestructiveFunc[T, R any](fn func(ctx context.Context, input T) (R, error)) ActionRoute {
	route := RouteFunc(fn)
	route.Destructive = true
	return route
}

// outputSchemaCache stores reflected output schemas by Go type to avoid
// regenerating identical JSON Schemas for every route registration.
var (
	outputSchemaCache sync.Map // reflect.Type → map[string]any
	inputSchemaCache  sync.Map // reflect.Type → map[string]any
)

// schemaForType generates a JSON Schema map for the given reflect.Type
// and caches the result. Returns nil on error (best-effort).
func schemaForType(rt reflect.Type) map[string]any {
	if rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	if cached, ok := outputSchemaCache.Load(rt); ok {
		m, _ := cached.(map[string]any)
		return m
	}
	schema, err := jsonschema.ForType(rt, nil)
	if err != nil {
		return nil
	}
	data, marshalErr := json.Marshal(schema)
	if marshalErr != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(data, &m) != nil {
		return nil
	}
	normalizeSchemaDescriptions(m)
	outputSchemaCache.Store(rt, m)
	return m
}

func normalizeSchemaDescriptions(node any) {
	switch typed := node.(type) {
	case map[string]any:
		if description, ok := typed["description"].(string); ok {
			typed["description"] = strings.TrimSuffix(description, ",required")
		}
		for _, value := range typed {
			normalizeSchemaDescriptions(value)
		}
	case []any:
		for _, value := range typed {
			normalizeSchemaDescriptions(value)
		}
	}
}

func inputSchemaForType(rt reflect.Type) map[string]any {
	if rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	if cached, ok := inputSchemaCache.Load(rt); ok {
		m, _ := cached.(map[string]any)
		return m
	}
	baseSchema := schemaForType(rt)
	if baseSchema == nil {
		return nil
	}
	schema := cloneSchemaMap(baseSchema)
	markWriteOnlySecretFields(schema, rt)
	if required := requiredJSONFieldNames(rt); len(required) > 0 {
		schema["required"] = required
	} else {
		delete(schema, "required")
	}
	inputSchemaCache.Store(rt, schema)
	return schema
}

func markWriteOnlySecretFields(schema map[string]any, rt reflect.Type) {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return
	}
	for _, name := range secretJSONFieldNames(rt) {
		if prop, isPropertySchema := props[name].(map[string]any); isPropertySchema {
			prop["writeOnly"] = true
		}
	}
}

func secretJSONFieldNames(rt reflect.Type) []string {
	if rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	if rt.Kind() != reflect.Struct {
		return nil
	}
	var names []string
	for field := range rt.Fields() {
		if field.PkgPath != "" && !field.Anonymous {
			continue
		}
		jsonName := jsonFieldName(field)
		if jsonName == "-" {
			continue
		}
		if field.Anonymous && jsonName == "" {
			names = append(names, secretJSONFieldNames(field.Type)...)
			continue
		}
		if jsonName == "token" || jsonName == "signing_token" {
			names = append(names, jsonName)
		}
	}
	return names
}

func requiredJSONFieldNames(rt reflect.Type) []string {
	if rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	if rt.Kind() != reflect.Struct {
		return nil
	}
	var names []string
	for field := range rt.Fields() {
		if field.PkgPath != "" && !field.Anonymous {
			continue
		}
		jsonName := jsonFieldName(field)
		if jsonName == "-" {
			continue
		}
		if field.Anonymous && jsonName == "" {
			names = append(names, requiredJSONFieldNames(field.Type)...)
			continue
		}
		if jsonName != "" && jsonSchemaTagHasRequired(field.Tag.Get("jsonschema")) {
			names = append(names, jsonName)
		}
	}
	sort.Strings(names)
	return names
}

func jsonFieldName(field reflect.StructField) string {
	name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	if name != "" {
		return name
	}
	if field.Anonymous {
		return ""
	}
	return field.Name
}

func jsonSchemaTagHasRequired(tag string) bool {
	for part := range strings.SplitSeq(tag, ",") {
		if strings.TrimSpace(part) == "required" {
			return true
		}
	}
	return false
}

// SchemaForRoute returns the cached output schema for type R.
// Exported for use by gen_llms and audit tools.
func SchemaForRoute[R any]() map[string]any {
	return schemaForType(reflect.TypeFor[R]())
}

// requestContextKey is the context key for storing the MCP request in
// action handler contexts. Used by WrapActionWithRequest to pass the
// CallToolRequest to handlers that need it (e.g., for progress tracking).
type requestContextKey struct{}

// ContextWithRequest returns a derived context carrying the MCP request.
func ContextWithRequest(ctx context.Context, req *mcp.CallToolRequest) context.Context {
	return context.WithValue(ctx, requestContextKey{}, req)
}

// RequestFromContext extracts the MCP request from a context, or nil if absent.
func RequestFromContext(ctx context.Context) *mcp.CallToolRequest {
	req, _ := ctx.Value(requestContextKey{}).(*mcp.CallToolRequest)
	return req
}

// reservedParamKeys lists meta-protocol keys that callers may set on the
// params map but which never map to a field on the typed action input. They
// are stripped before strict unmarshalling so they do not trigger the
// "unknown field" rejection added by [strictUnmarshal].
var reservedParamKeys = map[string]struct{}{
	"confirm": {}, // bypass destructive-action elicitation prompt
}

// stripReservedKeys returns a shallow copy of params with all meta-protocol
// keys (see [reservedParamKeys]) removed. The original map is not mutated.
// Returns the input map unchanged when no reserved keys are present so the
// common path stays allocation-free.
func stripReservedKeys(params map[string]any) map[string]any {
	hasReserved := false
	for k := range params {
		if _, ok := reservedParamKeys[k]; ok {
			hasReserved = true
			break
		}
	}
	if !hasReserved {
		return params
	}
	out := make(map[string]any, len(params))
	for k, v := range params {
		if _, ok := reservedParamKeys[k]; ok {
			continue
		}
		out[k] = v
	}
	return out
}

var commonParamAliases = []struct {
	Alias     string
	Canonical string
}{
	{Alias: "search", Canonical: "query"},
	{Alias: "mr_iid", Canonical: "merge_request_iid"},
	{Alias: "merge_request_id", Canonical: "merge_request_iid"},
	{Alias: "pipeline_schedule_id", Canonical: "schedule_id"},
	{Alias: "emoji_id", Canonical: "award_id"},
	{Alias: "key_id", Canonical: "deploy_key_id"},
	{Alias: "token_id", Canonical: "deploy_token_id"},
	{Alias: "commit_id", Canonical: "commit_sha"},
	{Alias: "dependency_export_id", Canonical: "export_id"},
	{Alias: "url", Canonical: "external_url"},
	{Alias: "rule_id", Canonical: "check_id"},
	{Alias: "group_path", Canonical: "full_path"},
	{Alias: "group_id", Canonical: "full_path"},
	{Alias: "include_descendant_groups", Canonical: "include_descendants"},
	{Alias: "project_path", Canonical: "project_id"},
	{Alias: "project_id", Canonical: "project_path"},
	{Alias: "group_path", Canonical: "group_id"},
	{Alias: "group_id", Canonical: "group_path"},
	{Alias: "link", Canonical: "link_url"},
	{Alias: "image", Canonical: "image_url"},
	{Alias: "content", Canonical: "body"},
	{Alias: "description", Canonical: "body"},
	{Alias: "note", Canonical: "body"},
	{Alias: "body", Canonical: "note"},
	{Alias: "body", Canonical: "message"},
	{Alias: "content", Canonical: "message"},
	{Alias: "shard", Canonical: "destination_storage_name"},
	{Alias: "storage_shard", Canonical: "destination_storage_name"},
	{Alias: "scope", Canonical: "scopes"},
	{Alias: "scope", Canonical: "environment_scope"},
	{Alias: "environment", Canonical: "environment_scope"},
	{Alias: "expires", Canonical: "expires_at"},
	{Alias: "expiry", Canonical: "expires_at"},
	{Alias: "expiration", Canonical: "expires_at"},
	{Alias: "merge_when_pipeline_succeeds", Canonical: "auto_merge"},
	{Alias: "branch", Canonical: "branch_name"},
	{Alias: "branch", Canonical: "ref"},
	{Alias: "branch_name", Canonical: "ref"},
	{Alias: "ref", Canonical: "content_ref"},
	{Alias: "from_ref", Canonical: "from"},
	{Alias: "to_ref", Canonical: "to"},
	{Alias: "target_branch", Canonical: "to"},
	{Alias: "feature_flag_name", Canonical: "name"},
	{Alias: "emoji_name", Canonical: "name"},
	{Alias: "award_emoji", Canonical: "name"},
	{Alias: "award", Canonical: "name"},
	{Alias: "time_estimate", Canonical: "duration"},
	{Alias: "name", Canonical: "environment"},
}

// ParamAliasExplanation describes a compatibility parameter normalization.
// It intentionally records parameter names only, never parameter values.
type ParamAliasExplanation struct {
	Alias     string `json:"alias"`
	Canonical string `json:"canonical"`
	Source    string `json:"source"`
	Notes     string `json:"notes,omitempty"`
}

func normalizeIDAlias(params map[string]any, fields map[string]struct{}, accepts func(string) bool, clone func() map[string]any) {
	value, hasID := params["id"]
	if !hasID || accepts("id") {
		return
	}
	canonical := ""
	for name := range fields {
		if name == "id" || !strings.HasSuffix(name, "_id") {
			continue
		}
		if canonical != "" {
			return
		}
		canonical = name
	}
	if canonical == "" {
		return
	}
	updated := clone()
	if _, hasCanonical := params[canonical]; !hasCanonical {
		updated[canonical] = value
	}
	delete(updated, "id")
}

// normalizeParamAliases accepts common LLM-generated parameter aliases only
// when the target input type has the canonical field and not the alias field.
func normalizeParamAliases(params map[string]any, target reflect.Type) map[string]any {
	if len(params) == 0 {
		return params
	}
	fields := jsonFieldNames(target)
	if len(fields) == 0 {
		return params
	}
	return normalizeParamAliasesWithFields(params, fields)
}

// NormalizeParamAliasesForSchema applies the same compatibility aliases used
// by UnmarshalParams, driven by a JSON Schema properties map instead of a Go
// struct type. It is used by evaluation code that validates simulated calls.
func NormalizeParamAliasesForSchema(params, schema map[string]any) map[string]any {
	if len(params) == 0 {
		return params
	}
	fields := schemaPropertyNames(schema)
	if len(fields) == 0 {
		return params
	}
	normalized := normalizeParamAliasesWithFields(params, fields)
	normalized = coerceSchemaParamTypes(normalized, schema)
	normalized = coerceSingleStringArraysForSchema(normalized, schema)
	return coerceStringListParamsForSchema(normalized, schema)
}

// NormalizeParamAliasesForSchemaWithExplanation returns the normalized params
// and name-only metadata describing compatibility aliases that were applied.
func NormalizeParamAliasesForSchemaWithExplanation(params, schema map[string]any) (map[string]any, []ParamAliasExplanation) {
	normalized := NormalizeParamAliasesForSchema(params, schema)
	return normalized, explainSchemaParamAliases(params, schema)
}

func explainSchemaParamAliases(params, schema map[string]any) []ParamAliasExplanation {
	fields := schemaPropertyNames(schema)
	if len(params) == 0 || len(fields) == 0 {
		return nil
	}
	accepts := func(name string) bool {
		_, ok := fields[name]
		return ok
	}
	explanations := make([]ParamAliasExplanation, 0)
	for _, pair := range commonParamAliases {
		_, hasAlias := params[pair.Alias]
		_, hasCanonical := params[pair.Canonical]
		if !hasAlias || hasCanonical || !accepts(pair.Canonical) || accepts(pair.Alias) {
			continue
		}
		explanations = append(explanations, ParamAliasExplanation{
			Alias:     pair.Alias,
			Canonical: pair.Canonical,
			Source:    "schema_common",
		})
	}
	if explanation, ok := explainIDParamAlias(params, fields, accepts); ok {
		explanations = append(explanations, explanation)
	}
	if explanation, ok := explainIIDParamAlias(params, fields, accepts); ok {
		explanations = append(explanations, explanation)
	}
	if explanation, ok := explainEnvironmentIDParamAlias(params, accepts); ok {
		explanations = append(explanations, explanation)
	}
	return explanations
}

func explainIDParamAlias(params map[string]any, fields map[string]struct{}, accepts func(string) bool) (ParamAliasExplanation, bool) {
	if _, hasID := params["id"]; !hasID || accepts("id") {
		return ParamAliasExplanation{}, false
	}
	canonical := ""
	for name := range fields {
		if name == "id" || !strings.HasSuffix(name, "_id") {
			continue
		}
		if canonical != "" {
			return ParamAliasExplanation{}, false
		}
		canonical = name
	}
	if canonical == "" {
		return ParamAliasExplanation{}, false
	}
	return ParamAliasExplanation{Alias: "id", Canonical: canonical, Source: "schema_common"}, true
}

func explainIIDParamAlias(params map[string]any, fields map[string]struct{}, accepts func(string) bool) (ParamAliasExplanation, bool) {
	if _, hasIID := params["iid"]; !hasIID || accepts("iid") {
		return ParamAliasExplanation{}, false
	}
	canonical := ""
	for name := range fields {
		if name == "iid" || !strings.HasSuffix(name, "_iid") {
			continue
		}
		if canonical != "" {
			return ParamAliasExplanation{}, false
		}
		canonical = name
	}
	if canonical == "" {
		return ParamAliasExplanation{}, false
	}
	return ParamAliasExplanation{Alias: "iid", Canonical: canonical, Source: "schema_common"}, true
}

func explainEnvironmentIDParamAlias(params map[string]any, accepts func(string) bool) (ParamAliasExplanation, bool) {
	if _, hasEnvironmentID := params["environment_id"]; !hasEnvironmentID || !accepts("environment") || accepts("environment_id") {
		return ParamAliasExplanation{}, false
	}
	return ParamAliasExplanation{Alias: "environment_id", Canonical: "environment", Source: "schema_common"}, true
}

func normalizeParamAliasesWithFields(params map[string]any, fields map[string]struct{}) map[string]any {
	out := params
	cloned := false
	clone := func() map[string]any {
		if !cloned {
			out = maps.Clone(params)
			cloned = true
		}
		return out
	}
	accepts := func(name string) bool {
		_, ok := fields[name]
		return ok
	}
	for _, pair := range commonParamAliases {
		value, hasAlias := out[pair.Alias]
		_, hasCanonical := out[pair.Canonical]
		if !hasAlias || !accepts(pair.Canonical) || accepts(pair.Alias) {
			continue
		}
		updated := clone()
		if !hasCanonical {
			updated[pair.Canonical] = value
		}
		delete(updated, pair.Alias)
	}
	normalizeIDAlias(out, fields, accepts, clone)
	normalizeIIDAlias(out, fields, accepts, clone)
	normalizeActiveAlias(out, accepts, clone)
	normalizeFilePathAlias(out, accepts, clone)
	normalizeBranchAliases(out, accepts, clone)
	normalizeEnvironmentNameAlias(out, accepts, clone)
	normalizeEnvironmentIDAlias(out, accepts, clone)
	normalizeEncodedPathIdentifiers(out, accepts, clone)
	removeContextOnlyDiscussionID(out, accepts, clone)
	return out
}

func normalizeEnvironmentNameAlias(out map[string]any, accepts func(string) bool, clone func() map[string]any) {
	value, hasEnvironment := out["environment"]
	if !hasEnvironment || !accepts("name") || accepts("environment") || accepts("environment_scope") {
		return
	}
	if _, hasName := out["name"]; hasName {
		delete(clone(), "environment")
		return
	}
	updated := clone()
	updated["name"] = value
	delete(updated, "environment")
}

func normalizeEnvironmentIDAlias(out map[string]any, accepts func(string) bool, clone func() map[string]any) {
	value, hasEnvironmentID := out["environment_id"]
	if !hasEnvironmentID || !accepts("environment") || accepts("environment_id") {
		return
	}
	updated := clone()
	if _, hasEnvironment := out["environment"]; !hasEnvironment {
		updated["environment"] = value
	}
	delete(updated, "environment_id")
}

func normalizeEncodedPathIdentifiers(out map[string]any, accepts func(string) bool, clone func() map[string]any) {
	for _, name := range []string{"project_id", "project_path", "group_id", "group_path", "full_path", "child_project_path"} {
		if !accepts(name) {
			continue
		}
		value, ok := out[name].(string)
		if !ok {
			continue
		}
		decoded, changed := decodeEncodedPathIdentifier(value)
		if !changed {
			continue
		}
		clone()[name] = decoded
	}
}

func decodeEncodedPathIdentifier(value string) (string, bool) {
	if !strings.Contains(strings.ToLower(value), "%2f") {
		return "", false
	}
	decoded, err := url.PathUnescape(value)
	if err != nil || decoded == value || !strings.Contains(decoded, "/") {
		return "", false
	}
	return decoded, true
}

func normalizeIIDAlias(params map[string]any, fields map[string]struct{}, accepts func(string) bool, clone func() map[string]any) {
	value, hasIID := params["iid"]
	if !hasIID || accepts("iid") {
		return
	}
	canonical := ""
	for name := range fields {
		if name == "iid" || !strings.HasSuffix(name, "_iid") {
			continue
		}
		if canonical != "" {
			return
		}
		canonical = name
	}
	if canonical == "" {
		return
	}
	updated := clone()
	if _, hasCanonical := params[canonical]; !hasCanonical {
		updated[canonical] = value
	}
	delete(updated, "iid")
}

func removeContextOnlyDiscussionID(params map[string]any, accepts func(string) bool, clone func() map[string]any) {
	if _, hasDiscussionID := params["discussion_id"]; !hasDiscussionID || accepts("discussion_id") || !accepts("note_id") {
		return
	}
	if _, hasNoteID := params["note_id"]; !hasNoteID {
		return
	}
	delete(clone(), "discussion_id")
}

func normalizeActiveAlias(out map[string]any, accepts func(string) bool, clone func() map[string]any) {
	value, hasActive := out["active"]
	if !hasActive || !accepts("paused") || accepts("active") {
		return
	}
	if _, hasPaused := out["paused"]; hasPaused {
		return
	}
	active, ok := value.(bool)
	if !ok {
		return
	}
	updated := clone()
	updated["paused"] = !active
	delete(updated, "active")
}

func normalizeFilePathAlias(out map[string]any, accepts func(string) bool, clone func() map[string]any) {
	value, hasFilePath := out["file_path"]
	if !hasFilePath || !accepts("path") || !accepts("filename") || accepts("file_path") {
		return
	}
	if _, hasPath := out["path"]; hasPath {
		return
	}
	if _, hasFilename := out["filename"]; hasFilename {
		return
	}
	filePath, ok := value.(string)
	if !ok || filePath == "" {
		return
	}
	path, filename := splitPackageFilePath(filePath)
	updated := clone()
	updated["path"] = path
	updated["filename"] = filename
	delete(updated, "file_path")
}

func normalizeBranchAliases(out map[string]any, accepts func(string) bool, clone func() map[string]any) {
	if !accepts("source_branch") || !accepts("target_branch") {
		return
	}
	if _, hasSource := out["source_branch"]; !hasSource {
		copyBranchAlias(out, accepts, clone, "source_branch", []string{"ref", "branch", "from"})
	}
	if _, hasTarget := out["target_branch"]; !hasTarget {
		copyBranchAlias(out, accepts, clone, "target_branch", []string{"to", "base"})
	}
}

func copyBranchAlias(out map[string]any, accepts func(string) bool, clone func() map[string]any, target string, aliases []string) {
	for _, key := range aliases {
		value, ok := out[key]
		if !ok || !nonEmptyStringValue(value) || accepts(key) {
			continue
		}
		updated := clone()
		updated[target] = value
		delete(updated, key)
		return
	}
}

func nonEmptyStringValue(value any) bool {
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) != ""
}

func splitPackageFilePath(filePath string) (dir, filename string) {
	filePath = strings.Trim(filePath, "/")
	if filePath == "" {
		return ".", ""
	}
	idx := strings.LastIndex(filePath, "/")
	if idx < 0 {
		return ".", filePath
	}
	return filePath[:idx], filePath[idx+1:]
}

func jsonFieldNames(target reflect.Type) map[string]struct{} {
	if target == nil {
		return nil
	}
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	if target.Kind() != reflect.Struct {
		return nil
	}
	fields := make(map[string]struct{})
	collectJSONFieldNames(target, fields)
	return fields
}

func schemaPropertyNames(schema map[string]any) map[string]struct{} {
	properties, ok := schema["properties"].(map[string]any)
	if !ok || len(properties) == 0 {
		return nil
	}
	fields := make(map[string]struct{}, len(properties))
	for name := range properties {
		fields[name] = struct{}{}
	}
	return fields
}

func collectJSONFieldNames(target reflect.Type, fields map[string]struct{}) {
	for field := range target.Fields() {
		if field.PkgPath != "" && !field.Anonymous {
			continue
		}
		name := jsonFieldName(field)
		if name == "-" {
			continue
		}
		if field.Anonymous && name == "" {
			fieldType := field.Type
			for fieldType.Kind() == reflect.Pointer {
				fieldType = fieldType.Elem()
			}
			if fieldType.Kind() == reflect.Struct {
				collectJSONFieldNames(fieldType, fields)
				continue
			}
		}
		if name == "" {
			name = field.Name
		}
		fields[name] = struct{}{}
	}
}

// UnmarshalParams re-serializes params map to JSON and deserializes into T.
// LLMs frequently send numeric values as JSON strings (e.g. "17" instead of 17).
// When standard unmarshalling fails, this function retries after coercing
// string values that look like integers or floats into actual numbers.
//
// Unknown keys in params (i.e. fields that do not exist on T) are rejected
// with an actionable error so that LLMs receive a clear diagnostic when they
// mistype a parameter name (e.g. "iid" instead of "snippet_id"). This mirrors
// the JSON Schema lockdown applied to tools/list responses (see
// LockdownInputSchemas) and the MCP guidance to surface validation errors as
// recoverable tool results so the model can self-correct. Meta-protocol keys
// (see [reservedParamKeys]) are stripped before unmarshalling.
func UnmarshalParams[T any](params map[string]any) (T, error) {
	var input T
	target := reflect.TypeFor[T]()
	normalized := normalizeParamAliases(stripReservedKeys(params), target)
	cleaned := coerceStringIDNumbers(coerceStringListParams(coerceSingleStringSlices(coerceStructuredParams(normalized, target), target), target), target)
	cleaned, coerceErr := coerceNumericParams(cleaned, target)
	if coerceErr != nil {
		return input, newParamValidationError(invalidActionParamsError, coerceErr)
	}
	data, err := json.Marshal(cleaned)
	if err != nil {
		return input, newParamValidationError("invalid params: %w", err)
	}
	if err = strictUnmarshal(data, &input); err != nil {
		// Retry with legacy broad numeric string coercion for routes whose
		// target type cannot be fully reflected, while preserving the original
		// error if the retry still fails.
		coerced := coerceNumericStrings(cleaned)
		data2, marshalErr := json.Marshal(coerced)
		if marshalErr != nil {
			return input, newParamValidationError(invalidActionParamsError, err)
		}
		if strictUnmarshal(data2, &input) != nil {
			// Return the original error for a clearer message.
			return input, newParamValidationError(invalidActionParamsError, err)
		}
		return input, nil
	}
	return input, nil
}

func coerceStructuredParams(params map[string]any, target reflect.Type) map[string]any {
	if len(params) == 0 {
		return params
	}
	fields := jsonFieldTypes(target)
	if len(fields) == 0 {
		return params
	}
	var out map[string]any
	for name, value := range params {
		fieldType, ok := fields[name]
		if !ok {
			continue
		}
		coerced, changed := coerceStructuredValue(name, value, fieldType)
		if !changed {
			continue
		}
		if out == nil {
			out = maps.Clone(params)
		}
		out[name] = coerced
	}
	if out != nil {
		return out
	}
	return params
}

func coerceStructuredValue(name string, value any, target reflect.Type) (any, bool) {
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	if coerced, ok := coercePaginationBoolean(name, value, target); ok {
		return coerced, true
	}
	if target.Kind() != reflect.Slice {
		return normalizeAccessLevelScalar(name, value, target)
	}
	elem := target.Elem()
	for elem.Kind() == reflect.Pointer {
		elem = elem.Elem()
	}
	if elem.Kind() != reflect.Struct {
		return value, false
	}
	if item, ok := value.(map[string]any); ok {
		return []any{normalizeStructuredObjectFields(item, elem)}, true
	}
	if accessLevel, ok := gitLabRoleAccessLevel(value); ok && structHasJSONField(elem, "access_level") {
		return []any{map[string]any{"access_level": accessLevel}}, true
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
			updatedItem := normalizeStructuredObjectFields(typed, elem)
			updatedItems[index] = updatedItem
			changed = changed || !reflect.DeepEqual(typed, updatedItem)
		default:
			accessLevel, accessLevelOK := gitLabRoleAccessLevel(typed)
			if !accessLevelOK || !structHasJSONField(elem, "access_level") {
				updatedItems[index] = item
				continue
			}
			updatedItems[index] = map[string]any{"access_level": accessLevel}
			changed = true
		}
	}
	if !changed {
		return value, false
	}
	return updatedItems, true
}

func normalizeStructuredObjectFields(value map[string]any, target reflect.Type) map[string]any {
	fields := jsonFieldTypes(target)
	if len(fields) == 0 {
		return value
	}
	var out map[string]any
	clone := func() map[string]any {
		if out == nil {
			out = maps.Clone(value)
		}
		return out
	}
	if acceptsJSONField(fields, "required_approvals") {
		for _, alias := range []string{"required_approval_count", "approval_count", "approvals_required"} {
			aliasValue, ok := value[alias]
			if !ok {
				continue
			}
			updated := clone()
			if _, hasCanonical := updated["required_approvals"]; !hasCanonical {
				updated["required_approvals"] = aliasValue
			}
			delete(updated, alias)
		}
	}
	if acceptsJSONField(fields, "required_approvals") && acceptsJSONField(fields, "access_level") && hasStructuredApprovalCount(value) && !hasStructuredApprovalPrincipal(value) {
		clone()["access_level"] = 40
	}
	if acceptsJSONField(fields, "access_level") {
		updated := cloneAccessLevelAliases(value, clone)
		if accessLevel, ok := gitLabRoleAccessLevel(updated["access_level"]); ok {
			clone()["access_level"] = accessLevel
		}
	}
	for fieldName, fieldType := range fields {
		fieldValue, ok := value[fieldName]
		if !ok {
			continue
		}
		coerced, changed := coerceStructuredValue(fieldName, fieldValue, fieldType)
		if changed {
			clone()[fieldName] = coerced
		}
	}
	if out != nil {
		return out
	}
	return value
}

func hasStructuredApprovalCount(value map[string]any) bool {
	_, ok := value["required_approvals"]
	if ok {
		return true
	}
	for _, alias := range []string{"required_approval_count", "approval_count", "approvals_required"} {
		if _, aliasOK := value[alias]; aliasOK {
			return true
		}
	}
	return false
}

func hasStructuredApprovalPrincipal(value map[string]any) bool {
	for _, name := range []string{"access_level", "user_id", "group_id"} {
		if _, ok := value[name]; ok {
			return true
		}
	}
	for _, alias := range []string{"deploy_access_level", "group_access_level", "project_access_level", "machine_user_access_level"} {
		if _, ok := value[alias]; ok {
			return true
		}
	}
	return false
}

func coercePaginationBoolean(name string, value any, target reflect.Type) (any, bool) {
	if !isPaginationLimitParam(name) || !isNumericKind(target.Kind()) {
		return value, false
	}
	boolValue, ok := value.(bool)
	if !ok || !boolValue {
		return value, false
	}
	if name == "page" {
		return 1, true
	}
	return 100, true
}

func isPaginationLimitParam(name string) bool {
	switch name {
	case "first", "last", "per_page", "page":
		return true
	default:
		return false
	}
}

func cloneAccessLevelAliases(value map[string]any, clone func() map[string]any) map[string]any {
	if _, hasAccessLevel := value["access_level"]; hasAccessLevel {
		return value
	}
	for _, alias := range []string{"deploy_access_level", "group_access_level", "project_access_level", "machine_user_access_level"} {
		aliasValue, ok := value[alias]
		if !ok {
			continue
		}
		updated := clone()
		updated["access_level"] = aliasValue
		delete(updated, alias)
		return updated
	}
	return value
}

func normalizeAccessLevelScalar(name string, value any, target reflect.Type) (any, bool) {
	if !isAccessLevelParamName(name) {
		return value, false
	}
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	if !isNumericKind(target.Kind()) {
		return value, false
	}
	accessLevel, ok := gitLabRoleAccessLevel(value)
	if !ok {
		return value, false
	}
	return accessLevel, true
}

func acceptsJSONField(fields map[string]reflect.Type, name string) bool {
	_, ok := fields[name]
	return ok
}

func structHasJSONField(target reflect.Type, name string) bool {
	return acceptsJSONField(jsonFieldTypes(target), name)
}

func isAccessLevelParamName(name string) bool {
	return name == "access_level" || strings.HasSuffix(name, "_access_level")
}

func gitLabRoleAccessLevel(value any) (int, bool) {
	if text, ok := value.(string); ok {
		normalized := strings.ToLower(strings.TrimSpace(strings.NewReplacer("_", " ", "-", " ").Replace(text)))
		if integer, err := integerFromString(normalized); err == nil {
			return validGitLabRoleAccessLevelInt64(integer)
		}
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
	integer, ok := numericRoleAccessLevel(value)
	if !ok {
		return 0, false
	}
	return validGitLabRoleAccessLevel(integer)
}

func numericRoleAccessLevel(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return validGitLabRoleAccessLevelInt64(typed)
	case float64:
		return validGitLabRoleAccessLevelFloat64(typed)
	default:
		return 0, false
	}
}

func validGitLabRoleAccessLevelInt64(value int64) (int, bool) {
	// Guard against overflow before narrowing: valid access levels are all small
	// non-negative integers (0–60), so reject anything outside int range first.
	if value < 0 || value > math.MaxInt32 {
		return 0, false
	}
	switch value {
	case 0, 10, 20, 30, 40, 50, 60:
		return int(value), true
	default:
		return 0, false
	}
}

func validGitLabRoleAccessLevelFloat64(value float64) (int, bool) {
	switch value {
	case 0, 10, 20, 30, 40, 50, 60:
		return int(value), true
	default:
		return 0, false
	}
}

func validGitLabRoleAccessLevel(value int) (int, bool) {
	switch value {
	case 0, 10, 20, 30, 40, 50, 60:
		return value, true
	default:
		return 0, false
	}
}

func newParamValidationError(format string, args ...any) error {
	return &ParamValidationError{Err: fmt.Errorf(format, args...)}
}

func coerceSingleStringSlices(params map[string]any, target reflect.Type) map[string]any {
	if len(params) == 0 {
		return params
	}
	fields := jsonFieldTypes(target)
	if len(fields) == 0 {
		return params
	}
	var out map[string]any
	for name, value := range params {
		fieldType, ok := fields[name]
		if !ok || !isStringSliceType(fieldType) {
			continue
		}
		text, ok := value.(string)
		if !ok {
			continue
		}
		if out == nil {
			out = maps.Clone(params)
		}
		out[name] = []string{text}
	}
	if out != nil {
		return out
	}
	return params
}

func coerceStringListParams(params map[string]any, target reflect.Type) map[string]any {
	if len(params) == 0 {
		return params
	}
	fields := jsonFieldTypes(target)
	if len(fields) == 0 {
		return params
	}
	var out map[string]any
	for name, value := range params {
		if !isCommaStringParam(name) || !isStringType(fields[name]) {
			continue
		}
		if csv, ok := stringListToCSV(value); ok {
			if out == nil {
				out = maps.Clone(params)
			}
			out[name] = csv
		}
	}
	if out != nil {
		return out
	}
	return params
}

func coerceStringIDNumbers(params map[string]any, target reflect.Type) map[string]any {
	if len(params) == 0 {
		return params
	}
	fields := jsonFieldTypes(target)
	if len(fields) == 0 {
		return params
	}
	var out map[string]any
	for name, value := range params {
		fieldType, ok := fields[name]
		if !ok || !isStringIDParam(name) || !isStringType(fieldType) {
			continue
		}
		text, ok := numericIDString(value)
		if !ok {
			continue
		}
		if out == nil {
			out = maps.Clone(params)
		}
		out[name] = text
	}
	if out != nil {
		return out
	}
	return params
}

func isStringIDParam(name string) bool {
	return name == "id" || name == "iid" || name == "project_path" || name == "group_path" || name == "full_path" || strings.HasSuffix(name, "_id") || strings.HasSuffix(name, "_iid")
}

func numericIDString(value any) (string, bool) {
	switch n := value.(type) {
	case int:
		return strconv.Itoa(n), true
	case int8:
		return strconv.FormatInt(int64(n), 10), true
	case int16:
		return strconv.FormatInt(int64(n), 10), true
	case int32:
		return strconv.FormatInt(int64(n), 10), true
	case int64:
		return strconv.FormatInt(n, 10), true
	case uint:
		return strconv.FormatUint(uint64(n), 10), true
	case uint8:
		return strconv.FormatUint(uint64(n), 10), true
	case uint16:
		return strconv.FormatUint(uint64(n), 10), true
	case uint32:
		return strconv.FormatUint(uint64(n), 10), true
	case uint64:
		return strconv.FormatUint(n, 10), true
	case json.Number:
		if i, err := integerFromString(n.String()); err == nil {
			return strconv.FormatInt(i, 10), true
		}
		return "", false
	case float32:
		return integerFloatString(float64(n))
	case float64:
		return integerFloatString(n)
	default:
		return "", false
	}
}

func integerFloatString(value float64) (string, bool) {
	if math.Trunc(value) != value || value < minInt64AsFloat || value > maxInt64AsFloat {
		return "", false
	}
	return strconv.FormatInt(int64(value), 10), true
}

func coerceNumericParams(params map[string]any, target reflect.Type) (map[string]any, error) {
	if len(params) == 0 {
		return params, nil
	}
	fields := jsonFieldTypes(target)
	if len(fields) == 0 {
		return params, nil
	}
	var out map[string]any
	for name, value := range params {
		fieldType, ok := fields[name]
		if !ok {
			continue
		}
		coerced, changed, err := coerceValueForTargetType(name, value, fieldType)
		if err != nil {
			return nil, err
		}
		if !changed {
			continue
		}
		if out == nil {
			out = maps.Clone(params)
		}
		out[name] = coerced
	}
	if out != nil {
		return out, nil
	}
	return params, nil
}

func coerceValueForTargetType(name string, value any, target reflect.Type) (coerced any, changed bool, err error) {
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	switch target.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return coerceSignedIntegerValue(name, value)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return coerceUnsignedIntegerValue(name, value)
	case reflect.Float32, reflect.Float64:
		return coerceFloatValue(name, value)
	case reflect.Slice:
		return coerceSliceValueForTargetType(name, value, target.Elem())
	default:
		return value, false, nil
	}
}

func coerceSignedIntegerValue(name string, value any) (coerced any, changed bool, err error) {
	text, ok := value.(string)
	if !ok {
		return value, false, nil
	}
	integer, err := integerFromString(text)
	if err != nil {
		return nil, false, fmt.Errorf("parameter %q must be an integer, got %q", name, text)
	}
	return integer, true, nil
}

func coerceUnsignedIntegerValue(name string, value any) (coerced any, changed bool, err error) {
	text, ok := value.(string)
	if !ok {
		return value, false, nil
	}
	integer, err := integerFromString(text)
	if err != nil || integer < 0 {
		return nil, false, fmt.Errorf("parameter %q must be a non-negative integer, got %q", name, text)
	}
	return uint64(integer), true, nil
}

func coerceFloatValue(name string, value any) (coerced any, changed bool, err error) {
	text, ok := value.(string)
	if !ok {
		return value, false, nil
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return nil, false, fmt.Errorf("parameter %q must be a number, got %q", name, text)
	}
	return f, true, nil
}

func coerceSliceValueForTargetType(name string, value any, elem reflect.Type) (coerced any, changed bool, err error) {
	for elem.Kind() == reflect.Pointer {
		elem = elem.Elem()
	}
	if !isNumericKind(elem.Kind()) {
		return value, false, nil
	}
	items, ok := sliceItems(value)
	if !ok {
		return value, false, nil
	}
	out := make([]any, len(items))
	for i, item := range items {
		itemCoerced, itemChanged, itemErr := coerceValueForTargetType(fmt.Sprintf("%s[%d]", name, i), item, elem)
		if itemErr != nil {
			return nil, false, itemErr
		}
		out[i] = itemCoerced
		changed = changed || itemChanged
	}
	if !changed {
		return value, false, nil
	}
	return out, true, nil
}

func sliceItems(value any) ([]any, bool) {
	switch items := value.(type) {
	case []any:
		return items, true
	case []string:
		out := make([]any, len(items))
		for i, item := range items {
			out[i] = item
		}
		return out, true
	default:
		return nil, false
	}
}

func isNumericKind(kind reflect.Kind) bool {
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

func coerceSchemaParamTypes(params, schema map[string]any) map[string]any {
	properties, hasProperties := schema["properties"].(map[string]any)
	if !hasProperties || len(properties) == 0 {
		return params
	}
	var out map[string]any
	for name, value := range params {
		property := properties[name]
		coerced, changed := coerceSchemaParamValue(name, value, property)
		if !changed {
			continue
		}
		if out == nil {
			out = maps.Clone(params)
		}
		out[name] = coerced
	}
	if out != nil {
		return out
	}
	return params
}

func coerceSchemaParamValue(name string, value, property any) (any, bool) {
	if schemaPropertyHasType(property, "integer") {
		if text, ok := value.(string); ok {
			if integer, err := integerFromString(text); err == nil {
				return integer, true
			}
		}
		return value, false
	}
	if schemaPropertyHasType(property, "number") {
		if text, ok := value.(string); ok {
			if f, err := strconv.ParseFloat(strings.TrimSpace(text), 64); err == nil {
				return f, true
			}
		}
		return value, false
	}
	if schemaPropertyHasType(property, "string") && isStringIDParam(name) {
		if text, ok := numericIDString(value); ok {
			return text, true
		}
	}
	if schemaPropertyHasType(property, "array") {
		return coerceSchemaArrayValue(value, property)
	}
	return value, false
}

func coerceSchemaArrayValue(value, property any) (any, bool) {
	prop, ok := property.(map[string]any)
	if !ok {
		return value, false
	}
	items, ok := prop["items"].(map[string]any)
	if !ok {
		return value, false
	}
	if !schemaPropertyHasType(items, "integer") && !schemaPropertyHasType(items, "number") {
		return value, false
	}
	values, ok := sliceItems(value)
	if !ok {
		return value, false
	}
	changed := false
	out := make([]any, len(values))
	for i, item := range values {
		coerced, itemChanged := coerceSchemaParamValue("", item, items)
		out[i] = coerced
		changed = changed || itemChanged
	}
	return out, changed
}

func schemaPropertyHasType(property any, expected string) bool {
	prop, isObject := property.(map[string]any)
	if !isObject {
		return false
	}
	switch kind := prop["type"].(type) {
	case string:
		return kind == expected
	case []any:
		return slices.Contains(kind, any(expected))
	case []string:
		return slices.Contains(kind, expected)
	}
	return false
}

func integerFromString(text string) (int64, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0, errors.New("empty integer")
	}
	if integer, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return integer, nil
	}
	f, err := strconv.ParseFloat(trimmed, 64)
	if err != nil || math.Trunc(f) != f || f < minInt64AsFloat || f > maxInt64AsFloat {
		return 0, fmt.Errorf("invalid integer %q", text)
	}
	return int64(f), nil
}

func isCommaStringParam(name string) bool {
	switch name {
	case "labels", "add_labels", "remove_labels":
		return true
	default:
		return false
	}
}

func stringListToCSV(value any) (string, bool) {
	switch labels := value.(type) {
	case []string:
		return strings.Join(labels, ","), true
	case []any:
		parts := make([]string, 0, len(labels))
		for _, item := range labels {
			text, ok := item.(string)
			if !ok {
				return "", false
			}
			parts = append(parts, text)
		}
		return strings.Join(parts, ","), true
	default:
		return "", false
	}
}

func coerceSingleStringArraysForSchema(params, schema map[string]any) map[string]any {
	properties, hasProperties := schema["properties"].(map[string]any)
	if !hasProperties || len(properties) == 0 {
		return params
	}
	var out map[string]any
	for name, value := range params {
		if !schemaPropertyIsStringArray(properties[name]) {
			continue
		}
		text, ok := value.(string)
		if !ok {
			continue
		}
		if out == nil {
			out = maps.Clone(params)
		}
		out[name] = []string{text}
	}
	if out != nil {
		return out
	}
	return params
}

func coerceStringListParamsForSchema(params, schema map[string]any) map[string]any {
	properties, hasProperties := schema["properties"].(map[string]any)
	if !hasProperties || len(properties) == 0 {
		return params
	}
	var out map[string]any
	for name, value := range params {
		if !isCommaStringParam(name) || !schemaPropertyIsString(properties[name]) {
			continue
		}
		if csv, ok := stringListToCSV(value); ok {
			if out == nil {
				out = maps.Clone(params)
			}
			out[name] = csv
		}
	}
	if out != nil {
		return out
	}
	return params
}

func schemaPropertyIsStringArray(property any) bool {
	prop, isObject := property.(map[string]any)
	if !isObject {
		return false
	}
	if prop["type"] != "array" {
		return false
	}
	items, hasItems := prop["items"].(map[string]any)
	if !hasItems {
		return false
	}
	return items["type"] == "string"
}

func schemaPropertyIsString(property any) bool {
	prop, isObject := property.(map[string]any)
	if !isObject {
		return false
	}
	return prop["type"] == "string"
}

func jsonFieldTypes(target reflect.Type) map[string]reflect.Type {
	if target == nil {
		return nil
	}
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	if target.Kind() != reflect.Struct {
		return nil
	}
	fields := make(map[string]reflect.Type)
	collectJSONFieldTypes(target, fields)
	return fields
}

func collectJSONFieldTypes(target reflect.Type, fields map[string]reflect.Type) {
	for field := range target.Fields() {
		if field.PkgPath != "" && !field.Anonymous {
			continue
		}
		name := jsonFieldName(field)
		if name == "-" {
			continue
		}
		fieldType := field.Type
		if field.Anonymous && name == "" {
			for fieldType.Kind() == reflect.Pointer {
				fieldType = fieldType.Elem()
			}
			if fieldType.Kind() == reflect.Struct {
				collectJSONFieldTypes(fieldType, fields)
				continue
			}
		}
		if name == "" {
			name = field.Name
		}
		fields[name] = field.Type
	}
}

func isStringSliceType(target reflect.Type) bool {
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	return target.Kind() == reflect.Slice && target.Elem().Kind() == reflect.String
}

func isStringType(target reflect.Type) bool {
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	return target.Kind() == reflect.String
}

// strictUnmarshal decodes JSON bytes into v while rejecting any keys that do
// not map to a field on the target type. This produces actionable errors of
// the form `json: unknown field "foo"` instead of silently dropping unknown
// keys, which is critical for LLM self-correction when mistyping argument
// names.
func strictUnmarshal(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// coerceNumericStrings returns a shallow copy of params where string values
// that parse as int64 or float64 are replaced with their numeric equivalents.
// This handles the common LLM behavior of sending numbers as JSON strings.
func coerceNumericStrings(params map[string]any) map[string]any {
	result := make(map[string]any, len(params))
	for k, v := range params {
		s, ok := v.(string)
		if !ok {
			result[k] = v
			continue
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			result[k] = n
			continue
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			result[k] = f
			continue
		}
		result[k] = v
	}
	return result
}

// WrapAction wraps a typed handler (input T -> output R) into a generic ActionFunc.
func WrapAction[T, R any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) (R, error)) ActionFunc {
	return func(ctx context.Context, params map[string]any) (any, error) {
		input, err := UnmarshalParams[T](params)
		if err != nil {
			return nil, err
		}
		return fn(ctx, client, input)
	}
}

// WrapVoidAction wraps a typed handler that returns only error.
func WrapVoidAction[T any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) error) ActionFunc {
	return func(ctx context.Context, params map[string]any) (any, error) {
		input, err := UnmarshalParams[T](params)
		if err != nil {
			return nil, err
		}
		return nil, fn(ctx, client, input)
	}
}

// WrapActionWithRequest wraps a handler that also requires the MCP request
// (e.g., for progress tracking). The request is extracted from context via
// RequestFromContext; if absent, nil is passed.
func WrapActionWithRequest[T, R any](client *gitlabclient.Client, fn func(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input T) (R, error)) ActionFunc {
	return func(ctx context.Context, params map[string]any) (any, error) {
		input, err := UnmarshalParams[T](params)
		if err != nil {
			return nil, err
		}
		return fn(ctx, RequestFromContext(ctx), client, input)
	}
}

// msgActionCompleted is the standard confirmation message returned by void
// and destructive void meta-tool routes on success.
const msgActionCompleted = "Action completed successfully."

// WrapVoidActionWithRequest wraps a void handler that also requires the MCP
// request. The request is extracted from context via RequestFromContext; if
// absent, nil is passed.
func WrapVoidActionWithRequest[T any](client *gitlabclient.Client, fn func(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input T) error) ActionFunc {
	return func(ctx context.Context, params map[string]any) (any, error) {
		input, err := UnmarshalParams[T](params)
		if err != nil {
			return nil, err
		}
		return nil, fn(ctx, RequestFromContext(ctx), client, input)
	}
}

// withVoidOutput wraps inner so that a nil result is replaced by successOutput.
// Errors from inner are propagated unchanged. This lets void handlers (which
// return nil) emit a typed confirmation value without duplicating the
// UnmarshalParams + call + return pattern in every route constructor.
func withVoidOutput(inner ActionFunc, successOutput any) ActionFunc {
	return func(ctx context.Context, params map[string]any) (any, error) {
		result, err := inner(ctx, params)
		if err != nil {
			return nil, err
		}
		if result == nil {
			return successOutput, nil
		}
		return result, nil
	}
}

// RouteAction wraps a typed function as a non-destructive ActionRoute
// and attaches the JSON Schema for the input type T and output type R.
func RouteAction[T, R any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) (R, error)) ActionRoute {
	inputType := reflect.TypeFor[T]()
	return ActionRoute{
		Handler:      WrapAction(client, fn),
		Destructive:  false,
		InputType:    inputType,
		InputSchema:  inputSchemaForType(inputType),
		OutputSchema: schemaForType(reflect.TypeFor[R]()),
	}
}

// RouteVoidAction wraps a typed void function as a non-destructive ActionRoute.
// The handler returns a typed VoidOutput confirmation so meta-tool routes
// expose structured output instead of nil content.
func RouteVoidAction[T any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) error) ActionRoute {
	inputType := reflect.TypeFor[T]()
	return ActionRoute{
		Handler:      withVoidOutput(WrapVoidAction(client, fn), VoidOutput{Status: "success", Message: msgActionCompleted}),
		Destructive:  false,
		InputType:    inputType,
		InputSchema:  inputSchemaForType(inputType),
		OutputSchema: schemaForType(reflect.TypeFor[VoidOutput]()),
	}
}

// RouteActionWithRequest wraps a typed function that needs the MCP request
// as a non-destructive ActionRoute and attaches input/output schemas.
func RouteActionWithRequest[T, R any](client *gitlabclient.Client, fn func(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input T) (R, error)) ActionRoute {
	inputType := reflect.TypeFor[T]()
	return ActionRoute{
		Handler:      WrapActionWithRequest(client, fn),
		Destructive:  false,
		InputType:    inputType,
		InputSchema:  inputSchemaForType(inputType),
		OutputSchema: schemaForType(reflect.TypeFor[R]()),
	}
}

// DestructiveAction wraps a typed function as a destructive ActionRoute
// and attaches input/output schemas.
func DestructiveAction[T, R any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) (R, error)) ActionRoute {
	inputType := reflect.TypeFor[T]()
	return ActionRoute{
		Handler:      WrapAction(client, fn),
		Destructive:  true,
		InputType:    inputType,
		InputSchema:  inputSchemaForType(inputType),
		OutputSchema: schemaForType(reflect.TypeFor[R]()),
	}
}

// DestructiveVoidAction wraps a typed void function as a destructive ActionRoute.
// The handler returns a typed DeleteOutput confirmation so meta-tool routes
// expose structured output instead of nil content.
func DestructiveVoidAction[T any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) error) ActionRoute {
	inputType := reflect.TypeFor[T]()
	return ActionRoute{
		Handler:      withVoidOutput(WrapVoidAction(client, fn), DeleteOutput{Status: "success", Message: msgActionCompleted}),
		Destructive:  true,
		InputType:    inputType,
		InputSchema:  inputSchemaForType(inputType),
		OutputSchema: schemaForType(reflect.TypeFor[DeleteOutput]()),
	}
}

// DestructiveActionWithRequest wraps a typed function that needs the MCP request
// as a destructive ActionRoute and attaches input/output schemas.
func DestructiveActionWithRequest[T, R any](client *gitlabclient.Client, fn func(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input T) (R, error)) ActionRoute {
	inputType := reflect.TypeFor[T]()
	return ActionRoute{
		Handler:      WrapActionWithRequest(client, fn),
		Destructive:  true,
		InputType:    inputType,
		InputSchema:  inputSchemaForType(inputType),
		OutputSchema: schemaForType(reflect.TypeFor[R]()),
	}
}

// DestructiveVoidActionWithRequest wraps a request-aware void function as a
// destructive ActionRoute with typed DeleteOutput confirmation, reusing
// WrapVoidActionWithRequest so the request-extraction logic is not duplicated.
func DestructiveVoidActionWithRequest[T any](client *gitlabclient.Client, fn func(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input T) error) ActionRoute {
	inputType := reflect.TypeFor[T]()
	return ActionRoute{
		Handler:      withVoidOutput(WrapVoidActionWithRequest(client, fn), DeleteOutput{Status: "success", Message: msgActionCompleted}),
		Destructive:  true,
		InputType:    inputType,
		InputSchema:  inputSchemaForType(inputType),
		OutputSchema: schemaForType(reflect.TypeFor[DeleteOutput]()),
	}
}

// FormatResultFunc converts an action result into an MCP call tool result.
type FormatResultFunc func(any) *mcp.CallToolResult

// AddMetaTool registers an action-dispatched meta-tool with route-derived
// annotations. Use it for meta-tools that may include mutating or destructive
// actions; if any route is destructive, the tool receives DestructiveHint=true.
func AddMetaTool(server *mcp.Server, name, desc string, routes ActionMap, icons []mcp.Icon, formatResult FormatResultFunc) {
	if server == nil {
		return
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:         name,
		Title:        TitleFromName(name),
		Description:  MetaToolDescriptionPrefix(name, routes) + desc,
		Annotations:  DeriveAnnotationsWithTitle(name, routes),
		Icons:        icons,
		InputSchema:  MetaToolSchema(routes),
		OutputSchema: MetaToolOutputSchema(),
	}, MakeMetaHandler(name, routes, formatResult))
}

// AddReadOnlyMetaTool registers an action-dispatched meta-tool whose actions
// are all read-only list/get/search-style operations.
func AddReadOnlyMetaTool(server *mcp.Server, name, desc string, routes ActionMap, icons []mcp.Icon, formatResult FormatResultFunc) {
	if server == nil {
		return
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:         name,
		Title:        TitleFromName(name),
		Description:  MetaToolDescriptionPrefix(name, routes) + desc,
		Annotations:  ReadOnlyMetaAnnotationsWithTitle(name),
		Icons:        icons,
		InputSchema:  MetaToolSchema(routes),
		OutputSchema: MetaToolOutputSchema(),
	}, MakeMetaHandler(name, routes, formatResult))
}

// MakeMetaHandler creates a generic MCP tool handler that dispatches to action routes.
// The formatResult function converts the action result into an MCP response.
// If formatResult is nil, a default JSON formatter is used.
//
// Destructive actions (delete, remove, revoke, unprotect, etc.) are automatically
// intercepted with a user confirmation prompt via MCP elicitation before execution.
// Confirmation can be bypassed with YOLO_MODE/AUTOPILOT env vars or by passing
// "confirm": true in the action params.
func MakeMetaHandler(toolName string, routes ActionMap, formatResult FormatResultFunc) func(ctx context.Context, req *mcp.CallToolRequest, input MetaToolInput) (*mcp.CallToolResult, any, error) {
	if formatResult == nil {
		formatResult = defaultFormatResult
	}
	return func(ctx context.Context, req *mcp.CallToolRequest, input MetaToolInput) (*mcp.CallToolResult, any, error) {
		route, validationResult := validateMetaToolInput(toolName, routes, &input)
		if validationResult != nil {
			return validationResult, nil, nil
		}

		// Confirm destructive actions before execution using route metadata.
		if route.Destructive {
			msg := fmt.Sprintf("Confirm %s/%s? This action may be irreversible.", toolName, input.Action)
			if result := ConfirmDestructiveAction(ctx, req, input.Params, msg); result != nil {
				return result, nil, nil
			}
		}

		// Store the request in context so WrapActionWithRequest handlers can access it.
		actionCtx := ContextWithRequest(ctx, req)

		start := time.Now()
		result, err := route.Handler(actionCtx, input.Params)
		LogToolCallAll(ctx, req, fmt.Sprintf("%s/%s", toolName, input.Action), start, err)

		if err != nil {
			if validationErr, matched := errors.AsType[*ParamValidationError](err); matched {
				return ErrorResult(fmt.Sprintf("%s/%s: %s", toolName, input.Action, validationErr.Error())), nil, nil
			}
			return nil, nil, err
		}
		callResult := formatResult(result)
		if callResult == nil {
			callResult = defaultFormatResult(result)
		}
		if callResult.IsError {
			return callResult, nil, nil
		}
		return callResult, enrichWithHints(result, callResult), nil
	}
}

func validateMetaToolInput(toolName string, routes ActionMap, input *MetaToolInput) (ActionRoute, *mcp.CallToolResult) {
	if input.Action == "" {
		return ActionRoute{}, ErrorResult(fmt.Sprintf("%s: 'action' is required. Valid actions: %s", toolName, ValidActionsString(routes)))
	}
	input.Action = NormalizeActionAlias(input.Action, routes)
	input.Action = normalizeActionAliasForParams(toolName, input.Action, input.Params, routes)
	route, ok := routes[input.Action]
	if !ok {
		return ActionRoute{}, ErrorResult(fmt.Sprintf("%s: unknown action %q. Valid actions: %s", toolName, input.Action, ValidActionsString(routes)))
	}
	if result := validateMetaToolParams(toolName, route, input); result != nil {
		return ActionRoute{}, result
	}
	return route, nil
}

func validateMetaToolParams(toolName string, route ActionRoute, input *MetaToolInput) *mcp.CallToolResult {
	if input.Params == nil {
		required := requiredParamNames(route.InputSchema)
		if len(required) > 0 {
			return ErrorResult(fmt.Sprintf("%s/%s: 'params' is required for this action. Required params: %s.", toolName, input.Action, strings.Join(required, ", ")))
		}
	}
	if missing := missingRequiredParamNames(route.InputSchema, input.Params); len(missing) > 0 && !hasUnknownParamNames(route.InputSchema, input.Params) {
		return ErrorResult(fmt.Sprintf("%s/%s: missing required params: %s. Put action-specific fields under params.", toolName, input.Action, strings.Join(missing, ", ")))
	}
	return nil
}

func requiredParamNames(schema map[string]any) []string {
	if schema == nil {
		return nil
	}
	rawRequired, ok := schema["required"]
	if !ok {
		return nil
	}
	var names []string
	switch values := rawRequired.(type) {
	case []any:
		for _, value := range values {
			if name, isString := value.(string); isString && name != "" {
				names = append(names, name)
			}
		}
	case []string:
		for _, name := range values {
			if name != "" {
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	return names
}

func missingRequiredParamNames(schema, params map[string]any) []string {
	if len(params) == 0 {
		return requiredParamNames(schema)
	}
	var missing []string
	for _, name := range requiredParamNames(schema) {
		if _, ok := params[name]; !ok {
			missing = append(missing, name)
		}
	}
	return missing
}

func hasUnknownParamNames(schema, params map[string]any) bool {
	if len(params) == 0 {
		return false
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok || len(properties) == 0 {
		return false
	}
	for name := range params {
		if _, exists := properties[name]; !exists {
			return true
		}
	}
	return false
}

// enrichWithHints extracts next-step hints from the Markdown content in
// callResult and merges them into the JSON result as a "next_steps" field.
// The returned json.RawMessage places next_steps as the first JSON field
// so that LLMs see actionable guidance before reading the full payload.
// If no hints exist, result is returned unchanged.
func enrichWithHints(result any, callResult *mcp.CallToolResult) any {
	if result == nil || callResult == nil {
		return result
	}
	var hints []string
	for _, c := range callResult.Content {
		tc, ok := c.(*mcp.TextContent)
		if !ok {
			continue
		}
		if h := ExtractHints(tc.Text); len(h) > 0 {
			hints = h
			break
		}
	}
	if len(hints) == 0 {
		return result
	}
	data, err := json.Marshal(result)
	if err != nil {
		return result
	}
	if len(data) == 0 || data[0] != '{' {
		return result
	}
	hintsData, err := json.Marshal(hints)
	if err != nil {
		return result
	}
	// Build JSON with next_steps as the first field so LLMs see guidance early.
	overhead := len(`"next_steps":,`)
	if len(data) > maxInt-overhead {
		return result
	}
	capacity := overhead + len(data)
	if len(hintsData) > maxInt-capacity {
		return result
	}
	capacity += len(hintsData)
	buf := make([]byte, 0, capacity)
	buf = append(buf, '{')
	buf = append(buf, `"next_steps":`...)
	buf = append(buf, hintsData...)
	if len(data) > 2 {
		buf = append(buf, ',')
		buf = append(buf, data[1:]...)
	} else {
		buf = append(buf, '}')
	}
	return json.RawMessage(buf)
}

// defaultFormatResult serializes the action result as JSON text content.
func defaultFormatResult(result any) *mcp.CallToolResult {
	if result == nil {
		return SuccessResult("ok")
	}
	data, err := json.Marshal(result)
	if err != nil {
		return SuccessResult(fmt.Sprintf("%v", result))
	}
	return SuccessResult(string(data))
}

// ValidActionsString returns a sorted, comma-separated list of action names.
func ValidActionsString(routes ActionMap) string {
	actions := make([]string, 0, len(routes))
	for k := range routes {
		actions = append(actions, k)
	}
	sort.Strings(actions)
	return strings.Join(actions, ", ")
}

// MetaToolSchema builds a JSON Schema for a meta-tool with the action field
// constrained to an enum of valid action names extracted from the routes map.
// Setting this as Tool.InputSchema ensures the LLM sees the exact list of
// valid actions in the schema, enabling first-try action selection.
//
// The strategy used (opaque, compact, full) is read from the package-level
// mode set via [SetMetaParamSchemaMode]. Default is opaque. Callers that
// always want the opaque envelope regardless of global configuration should
// invoke [BuildMetaToolSchema] directly.
func MetaToolSchema(routes ActionMap) map[string]any {
	return BuildMetaToolSchema(routes, currentMetaParamSchemaMode())
}

// Meta-tool param schema mode constants. Mirrors the values accepted by the
// META_PARAM_SCHEMA env var and --meta-param-schema CLI flag in package
// config. Duplicated here to avoid an import cycle (config → toolutil → mcp).
const (
	MetaParamSchemaOpaque  = "opaque"
	MetaParamSchemaCompact = "compact"
	MetaParamSchemaFull    = "full"
)

// metaParamSchemaMode is the package-level mode consulted by MetaToolSchema.
// It is intended to be set exactly once at startup (before any meta-tool is
// registered) via SetMetaParamSchemaMode. Reads/writes are guarded by a
// mutex purely to satisfy the race detector during concurrent test setups —
// the production lifecycle is single-writer-then-many-readers.
var (
	metaParamSchemaMu   sync.RWMutex
	metaParamSchemaMode = MetaParamSchemaOpaque
)

// SetMetaParamSchemaMode selects the meta-tool input schema strategy used by
// [MetaToolSchema]. Accepts "opaque" (default), "compact", or "full". Any
// other value is coerced to opaque so that misconfiguration cannot break the
// tools/list payload. Must be called before meta-tools are registered; later
// calls only affect schemas built after the call returns.
func SetMetaParamSchemaMode(mode string) {
	metaParamSchemaMu.Lock()
	defer metaParamSchemaMu.Unlock()
	setMetaParamSchemaModeLocked(mode)
}

// SetMetaParamSchemaModeScoped selects the meta-tool input schema strategy and
// returns a restore function for tests that temporarily override the global mode.
func SetMetaParamSchemaModeScoped(mode string) func() {
	metaParamSchemaMu.Lock()
	previous := metaParamSchemaMode
	setMetaParamSchemaModeLocked(mode)
	metaParamSchemaMu.Unlock()
	return func() {
		metaParamSchemaMu.Lock()
		defer metaParamSchemaMu.Unlock()
		metaParamSchemaMode = previous
	}
}

func setMetaParamSchemaModeLocked(mode string) {
	switch mode {
	case MetaParamSchemaOpaque, MetaParamSchemaCompact, MetaParamSchemaFull:
		metaParamSchemaMode = mode
	default:
		metaParamSchemaMode = MetaParamSchemaOpaque
	}
}

// currentMetaParamSchemaMode returns the active mode for MetaToolSchema.
func currentMetaParamSchemaMode() string {
	metaParamSchemaMu.RLock()
	defer metaParamSchemaMu.RUnlock()
	return metaParamSchemaMode
}

const (
	// paramsResourceHint is appended to the description of the params property
	// in every meta-tool input schema, regardless of mode. It points the LLM at
	// the per-action detail available via the gitlab://tools resource.
	paramsResourceHint = " For the JSON Schema of a specific action's `params`, read the MCP resource `gitlab://tools/{tool}.{action}` (replace placeholders with the tool name and the chosen action)."

	// metaToolParamsDescription is the canonical description for the params
	// property generated in every meta-tool input schema.
	metaToolParamsDescription = "Action-specific parameters as a JSON object. Required and optional fields differ per action. This envelope schema stays broad; runtime validation applies the chosen action's schema after reserved meta keys like `confirm` are stripped." + paramsResourceHint
)

// BuildMetaToolSchema returns the input schema for a meta-tool given the
// chosen mode. Unknown modes silently fall back to MetaParamSchemaOpaque so
// that callers cannot break the tools/list payload by passing a typo.
//
//   - opaque:  legacy {action, params:any} envelope (default).
//   - full:    discriminated oneOf with full per-action params schemas.
//   - compact: discriminated oneOf with descriptions and $defs stripped.
func BuildMetaToolSchema(routes ActionMap, mode string) map[string]any {
	actions := make([]string, 0, len(routes))
	for name := range routes {
		actions = append(actions, name)
	}
	sort.Strings(actions)

	base := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        actions,
				"description": "Action to perform. Pick exactly one of the values in `enum`. Each action expects its own `params` object — see the tool description for the per-action parameter list.",
			},
			"params": map[string]any{
				"type":                 "object",
				"description":          metaToolParamsDescription,
				"additionalProperties": true,
			},
		},
		"required":             []any{"action"},
		"additionalProperties": false,
	}

	switch mode {
	case MetaParamSchemaFull:
		base["oneOf"] = buildMetaOneOf(routes, actions, false)
	case MetaParamSchemaCompact:
		base["oneOf"] = buildMetaOneOf(routes, actions, true)
	default:
		// MetaParamSchemaOpaque or unknown — return the envelope unchanged.
	}
	return base
}

// MetaToolDescriptionPrefix builds a fixed-format header that should be
// prepended to a meta-tool's user-supplied description. The header gives
// LLMs a literal JSON usage example based on a representative action and
// points at the gitlab://tools resource for per-action params schemas.
// Returns an empty string when routes is empty so callers degrade gracefully
// rather than emit a malformed header.
func MetaToolDescriptionPrefix(toolName string, routes ActionMap) string {
	if len(routes) == 0 {
		return ""
	}
	actions := make([]string, 0, len(routes))
	for name := range routes {
		actions = append(actions, name)
	}
	sort.Strings(actions)
	exampleAction := metaToolExampleAction(actions)
	guidance := metaToolActionGuidanceSummary(routes, actions) + metaToolParameterGuidanceSummary(routes, actions)
	return fmt.Sprintf(
		"Use {\"action\":%q,\"params\":{...}}; only top-level keys are action and params.\nAction params schema: gitlab://tools/%s.<action>.%s\n\n",
		exampleAction, toolName,
		guidance,
	)
}

func metaToolExampleAction(sortedActions []string) string {
	for _, candidate := range []string{"list", "get", "search", "create", "update"} {
		if slices.Contains(sortedActions, candidate) {
			return candidate
		}
	}
	return sortedActions[0]
}

func metaToolActionGuidanceSummary(routes ActionMap, actionNames []string) string {
	var lines []string
	for _, action := range actionNames {
		usage := strings.TrimSpace(routes[action].Usage)
		if usage == "" {
			continue
		}
		usage = strings.Join(strings.Fields(usage), " ")
		lines = append(lines, fmt.Sprintf("- %s: %s", action, usage))
	}
	if len(lines) == 0 {
		return ""
	}
	return "\nAction guidance:\n" + strings.Join(lines, "\n")
}

func metaToolParameterGuidanceSummary(routes ActionMap, actionNames []string) string {
	var lines []string
	for _, action := range actionNames {
		guidance := routes[action].ParameterGuidance
		if len(guidance) == 0 {
			continue
		}
		params := make([]string, 0, len(guidance))
		for name := range guidance {
			params = append(params, name)
		}
		sort.Strings(params)
		for _, name := range params {
			item := guidance[name]
			if item.SemanticRole == "" && item.ValueSource == "" && len(item.CommonConfusions) == 0 {
				continue
			}
			line := fmt.Sprintf("- %s.%s", action, name)
			if item.SemanticRole != "" {
				line += ": " + item.SemanticRole
			}
			if item.ValueSource != "" {
				line += "; source: " + item.ValueSource
			}
			if len(item.CommonConfusions) > 0 {
				line += "; avoid: " + strings.Join(item.CommonConfusions, ", ")
			}
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "\nParameter guidance:\n" + strings.Join(lines, "\n")
}

// StripMetaToolDescriptionPrefix removes the generated meta-tool usage header
// added by MetaToolDescriptionPrefix while preserving standalone descriptions
// that happen to start with an example.
func StripMetaToolDescriptionPrefix(description string) string {
	lines := strings.Split(description, "\n")
	if len(lines) < 2 {
		return description
	}

	firstLine := strings.TrimSpace(lines[0])
	secondLine := strings.TrimSpace(lines[1])
	hasUsageExample := strings.Contains(firstLine, `Use {"action":`) || strings.Contains(firstLine, `Example: {"action":`)
	hasSchemaHint := strings.HasPrefix(secondLine, "Action params schema:") || strings.HasPrefix(secondLine, "For the params schema of any action")
	if !hasUsageExample || !hasSchemaHint {
		return description
	}

	start := 2
	for start < len(lines) {
		section := strings.TrimSpace(lines[start])
		if section != "Action guidance:" && section != "Parameter guidance:" {
			break
		}
		start++
		for start < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[start]), "- ") {
			start++
		}
	}
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	if start >= len(lines) {
		return description
	}
	return strings.Join(lines[start:], "\n")
}

// buildMetaOneOf constructs the oneOf branch list for full/compact modes.
// Each branch pins `action` to a const and replaces `params` with the captured
// per-action InputSchema (optionally compacted).
func buildMetaOneOf(routes ActionMap, sortedActions []string, compact bool) []any {
	branches := make([]any, 0, len(sortedActions))
	for _, action := range sortedActions {
		route := routes[action]
		params := route.InputSchema
		if compact && params != nil {
			params = compactParamsSchema(params)
		}
		if params == nil {
			params = map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			}
		}
		branches = append(branches, map[string]any{
			"properties": map[string]any{
				"action": map[string]any{"const": action},
				"params": params,
			},
			// Require both `action` and `params`. Without `params` in the
			// required list, the per-action params schema is only checked
			// when callers happen to include it, which silently bypasses
			// per-action required-field validation. Actions that need no
			// arguments still accept `{}` as a valid params value.
			"required": []any{"action", "params"},
		})
	}
	return branches
}

// compactParamsSchema returns a reduced copy of a per-action params schema
// containing only property names with their declared type and (when present)
// enum values. Descriptions, required, and $defs are dropped. A top-level
// $ref is resolved against the schema's own $defs once, then $defs is
// discarded. Best-effort: shapes we don't recognize are replaced with an
// open object schema rather than panicking.
func compactParamsSchema(s map[string]any) map[string]any {
	if s == nil {
		return nil
	}
	resolved := resolveTopLevelRef(s)
	props, _ := resolved["properties"].(map[string]any)
	if props == nil {
		return map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		}
	}
	compact := make(map[string]any, len(props))
	for k, v := range props {
		pm, ok := v.(map[string]any)
		if !ok {
			compact[k] = map[string]any{}
			continue
		}
		entry := map[string]any{}
		if t, has := pm["type"]; has {
			entry["type"] = t
		}
		if e, has := pm["enum"]; has {
			entry["enum"] = e
		}
		compact[k] = entry
	}
	return map[string]any{
		"type":                 "object",
		"properties":           compact,
		"additionalProperties": true,
	}
}

// resolveTopLevelRef returns the schema with a top-level "$ref" replaced by
// the referenced $defs entry. If no top-level $ref is present, returns s.
func resolveTopLevelRef(s map[string]any) map[string]any {
	ref, _ := s["$ref"].(string)
	if ref == "" {
		return s
	}
	defs, _ := s["$defs"].(map[string]any)
	if defs == nil {
		return s
	}
	const prefix = "#/$defs/"
	if !strings.HasPrefix(ref, prefix) {
		return s
	}
	target, _ := defs[ref[len(prefix):]].(map[string]any)
	if target == nil {
		return s
	}
	return target
}

// ActionDispatchOutputSchema returns a permissive JSON Schema for tools whose
// exact structured result depends on the selected catalog action.
func ActionDispatchOutputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"description":          "Result envelope. Top-level shape varies per action and matches the chosen action's typed output. Includes optional cross-cutting fields documented below.",
		"additionalProperties": true,
		"properties": map[string]any{
			"next_steps": map[string]any{
				"type":        "array",
				"description": "Optional. Suggested follow-up actions or tool calls for the LLM, contextual to the result.",
				"items":       map[string]any{"type": "string"},
			},
			"pagination": map[string]any{
				"type":                 "object",
				"description":          "Present on list actions. Use `has_more` and `next_page` to paginate through results.",
				"additionalProperties": true,
				"properties": map[string]any{
					"page":        map[string]any{"type": "integer", "description": "Current 1-based page index."},
					"per_page":    map[string]any{"type": "integer", "description": "Items per page."},
					"total":       map[string]any{"type": "integer", "description": "Total item count when known (some endpoints omit it for performance)."},
					"total_pages": map[string]any{"type": "integer", "description": "Total page count when known."},
					"next_page":   map[string]any{"type": "integer", "description": "Next page index when `has_more` is true."},
					"prev_page":   map[string]any{"type": "integer", "description": "Previous page index when applicable."},
					"has_more":    map[string]any{"type": "boolean", "description": "True when more pages are available after the current one."},
				},
			},
		},
	}
}

// MetaToolOutputSchema returns the shared action-dispatch output schema used by
// meta-tools.
func MetaToolOutputSchema() map[string]any {
	return ActionDispatchOutputSchema()
}

// DeriveAnnotations computes tool-level MCP annotations from the route map.
// If any route is destructive, returns a copy of MetaAnnotations (DestructiveHint: true).
// If all routes are non-destructive, returns a copy of NonDestructiveMetaAnnotations.
// Each call returns a fresh copy to avoid aliasing the shared singletons.
func DeriveAnnotations(routes ActionMap) *mcp.ToolAnnotations {
	for _, r := range routes {
		if r.Destructive {
			cp := *MetaAnnotations
			return &cp
		}
	}
	cp := *NonDestructiveMetaAnnotations
	return &cp
}

// DeriveAnnotationsWithTitle returns route-derived annotations with Title set from the tool name.
func DeriveAnnotationsWithTitle(name string, routes ActionMap) *mcp.ToolAnnotations {
	a := DeriveAnnotations(routes)
	a.Title = TitleFromName(name)
	return a
}

// ReadOnlyMetaAnnotationsWithTitle returns a copy of ReadOnlyMetaAnnotations with Title set.
func ReadOnlyMetaAnnotationsWithTitle(name string) *mcp.ToolAnnotations {
	a := *ReadOnlyMetaAnnotations
	a.Title = TitleFromName(name)
	return &a
}
