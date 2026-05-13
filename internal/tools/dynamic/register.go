package dynamic

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

var searchStopWordsMap = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "as": {}, "at": {}, "by": {}, "for": {}, "from": {}, "in": {},
	"of": {}, "on": {}, "or": {}, "please": {}, "the": {}, "to": {}, "using": {}, "via": {}, "with": {},
}

const (
	searchToolName   = "gitlab_search_tools"
	describeToolName = "gitlab_describe_tools"
	findToolName     = "gitlab_find_action"
	executeToolName  = "gitlab_execute_tool"

	actionFeatureFlagsUserListList = "feature_flags.ff_user_list_list"

	defaultLimit     = 20
	maxLimit         = 50
	minSegmentTerms  = 3
	maxSegmentTerms  = 6
	segmentTermBoost = 90
)

// SearchInput is the input for gitlab_search_tools.
type SearchInput struct {
	Query   string `json:"query" jsonschema:"Search terms for GitLab actions, such as project create, merge request approve, pipeline retry, or ci variable."`
	Limit   int    `json:"limit,omitempty" jsonschema:"Maximum number of matches to return. Defaults to 20 and is capped at 50."`
	Explain bool   `json:"explain,omitempty" jsonschema:"When true, include deterministic scoring reasons for each returned action. Defaults to false to keep responses compact."`
}

// SearchResult is one matching GitLab catalog action.
type SearchResult struct {
	ID             string              `json:"id" jsonschema:"Canonical action ID to pass to gitlab_describe_tools or gitlab_execute_tool."`
	Tool           string              `json:"tool" jsonschema:"Backing meta-tool name."`
	Domain         string              `json:"domain" jsonschema:"Canonical action domain."`
	Action         string              `json:"action" jsonschema:"Action name inside the catalog group."`
	SchemaURI      string              `json:"schema_uri" jsonschema:"MCP resource URI for the action parameter schema."`
	Destructive    bool                `json:"destructive" jsonschema:"Whether this action is marked destructive and requires explicit confirmation."`
	RequiredParams []string            `json:"required_params,omitempty" jsonschema:"Required parameter names captured from the action input schema."`
	Usage          string              `json:"usage,omitempty" jsonschema:"Short disambiguation note for commonly confused actions."`
	RelatedActions []string            `json:"related_actions,omitempty" jsonschema:"Curated nearby action IDs for workflows where ordering matters."`
	Score          int                 `json:"score" jsonschema:"Lexical relevance score for the query."`
	Explanation    *ScoringExplanation `json:"explanation,omitempty" jsonschema:"Optional scoring explanation returned only when explain is true."`
	LowConfidence  bool                `json:"low_confidence,omitempty" jsonschema:"Whether the top result is below the high-confidence score or margin threshold."`
	AmbiguousWith  []string            `json:"ambiguous_with,omitempty" jsonschema:"Other canonical action IDs that share the exact ambiguous alias used in the query."`
}

// SearchOutput is the structured output for gitlab_search_tools.
type SearchOutput struct {
	Query       string         `json:"query" jsonschema:"Original search query."`
	Count       int            `json:"count" jsonschema:"Number of returned matches."`
	Results     []SearchResult `json:"results" jsonschema:"Matching GitLab catalog actions."`
	Suggestions []string       `json:"suggestions,omitempty" jsonschema:"Small set of nearby tokens or common domains to try when no results matched."`
}

// DescribeInput is the input for gitlab_describe_tools.
type DescribeInput struct {
	Action  string   `json:"action,omitempty" jsonschema:"Canonical action ID to describe, such as project.create. Use either action or actions."`
	Actions []string `json:"actions,omitempty" jsonschema:"Canonical action IDs to describe in one call."`
}

// ActionExample shows how to call gitlab_execute_tool for an action.
type ActionExample struct {
	Tool      string         `json:"tool" jsonschema:"Tool to call for execution."`
	Arguments map[string]any `json:"arguments" jsonschema:"Example arguments for gitlab_execute_tool."`
}

// ActionDescription describes one GitLab catalog action.
type ActionDescription struct {
	ID             string         `json:"id" jsonschema:"Canonical action ID."`
	Tool           string         `json:"tool" jsonschema:"Backing meta-tool name."`
	Domain         string         `json:"domain" jsonschema:"Canonical action domain."`
	Action         string         `json:"action" jsonschema:"Action name inside the catalog group."`
	SchemaURI      string         `json:"schema_uri" jsonschema:"MCP resource URI for the action parameter schema."`
	Destructive    bool           `json:"destructive" jsonschema:"Whether this action requires explicit confirmation."`
	RequiredParams []string       `json:"required_params,omitempty" jsonschema:"Required parameter names captured from the input schema."`
	Usage          string         `json:"usage,omitempty" jsonschema:"Short disambiguation note for commonly confused actions."`
	RelatedActions []string       `json:"related_actions,omitempty" jsonschema:"Curated nearby action IDs for workflows where ordering matters."`
	InputSchema    map[string]any `json:"input_schema" jsonschema:"Exact JSON Schema for action-specific params."`
	OutputSchema   map[string]any `json:"output_schema,omitempty" jsonschema:"Best-effort JSON Schema for the action result."`
	Example        ActionExample  `json:"example" jsonschema:"Example gitlab_execute_tool call."`
}

// DescribeOutput is the structured output for gitlab_describe_tools.
type DescribeOutput struct {
	Count   int                 `json:"count" jsonschema:"Number of described actions."`
	Actions []ActionDescription `json:"actions" jsonschema:"Detailed action descriptions."`
}

// FindInput is the input for gitlab_find_action.
type FindInput struct {
	Query   string `json:"query" jsonschema:"Search terms for GitLab actions, such as project create, merge request approve, pipeline retry, or ci variable."`
	Limit   int    `json:"limit,omitempty" jsonschema:"Maximum number of matches to return. Defaults to 20 and is capped at 50."`
	Explain bool   `json:"explain,omitempty" jsonschema:"When true, include deterministic scoring reasons for each returned action. Defaults to false to keep responses compact."`
}

// FindResult is a matching catalog action with schema details and an execute example.
type FindResult struct {
	ID             string              `json:"id" jsonschema:"Canonical action ID to pass to gitlab_execute_tool."`
	Tool           string              `json:"tool" jsonschema:"Backing meta-tool name."`
	Domain         string              `json:"domain" jsonschema:"Canonical action domain."`
	Action         string              `json:"action" jsonschema:"Action name inside the catalog group."`
	SchemaURI      string              `json:"schema_uri" jsonschema:"MCP resource URI for the action parameter schema."`
	Destructive    bool                `json:"destructive" jsonschema:"Whether this action requires explicit confirmation."`
	RequiredParams []string            `json:"required_params,omitempty" jsonschema:"Required parameter names captured from the input schema."`
	Usage          string              `json:"usage,omitempty" jsonschema:"Short disambiguation note for commonly confused actions."`
	RelatedActions []string            `json:"related_actions,omitempty" jsonschema:"Curated nearby action IDs for workflows where ordering matters."`
	Score          int                 `json:"score" jsonschema:"Lexical relevance score for the query."`
	Explanation    *ScoringExplanation `json:"explanation,omitempty" jsonschema:"Optional scoring explanation returned only when explain is true."`
	LowConfidence  bool                `json:"low_confidence,omitempty" jsonschema:"Whether the top result is below the high-confidence score or margin threshold."`
	AmbiguousWith  []string            `json:"ambiguous_with,omitempty" jsonschema:"Other canonical action IDs that share the exact ambiguous alias used in the query."`
	InputSchema    map[string]any      `json:"input_schema" jsonschema:"Exact JSON Schema for action-specific params."`
	OutputSchema   map[string]any      `json:"output_schema,omitempty" jsonschema:"Best-effort JSON Schema for the action result."`
	Example        ActionExample       `json:"example" jsonschema:"Example gitlab_execute_tool call."`
}

// FindOutput is the structured output for gitlab_find_action.
type FindOutput struct {
	Query   string       `json:"query" jsonschema:"Original search query."`
	Count   int          `json:"count" jsonschema:"Number of returned matches."`
	Results []FindResult `json:"results" jsonschema:"Matching GitLab catalog actions with schemas and execute examples."`
}

// ExecuteInput is the input for gitlab_execute_tool.
type ExecuteInput struct {
	Action  string         `json:"action" jsonschema:"Canonical action ID returned by gitlab_search_tools, gitlab_describe_tools, or gitlab_find_action, such as project.list."`
	Params  map[string]any `json:"params" jsonschema:"Required action-specific parameters object validated by the selected action schema. Use an empty object for actions with no parameters."`
	Confirm bool           `json:"confirm,omitempty" jsonschema:"Set true to explicitly confirm destructive actions."`
}

type scoredActionEntry struct {
	entry         actionEntry
	score         int
	explanation   ScoringExplanation
	lowConfidence bool
	ambiguousWith []string
}

type actionEntry struct {
	ID             string
	Tool           string
	Domain         string
	Action         string
	Aliases        []string
	Tags           []string
	SchemaURI      string
	Destructive    bool
	RequiredParams []string
	Document       searchDocument
	SearchText     string
	SearchTokens   []string
	Route          toolutil.ActionRoute
}

type toolHandler func(context.Context, *mcp.CallToolRequest, toolutil.MetaToolInput) (*mcp.CallToolResult, any, error)

// Registry holds a deterministic action index and dispatch handlers.
type Registry struct {
	entries          []actionEntry
	byID             map[string]actionEntry
	aliases          map[string]string
	ambiguousAliases map[string][]string
	handlers         map[string]toolHandler
	SearchIndex      searchIndex
}

// RegisterCatalogTools registers the dynamic search, describe, and execute
// tools from the canonical action catalog.
func RegisterCatalogTools(server *mcp.Server, catalog *actioncatalog.Catalog) {
	registry := NewRegistryFromCatalog(catalog)
	addSearchTool(server, registry)
	addDescribeTool(server, registry)
	addExecuteTool(server, registry)
}

// RegisterCatalogFindExecuteTools registers the experimental two-tool dynamic
// catalog from the canonical action catalog.
func RegisterCatalogFindExecuteTools(server *mcp.Server, catalog *actioncatalog.Catalog) {
	registry := NewRegistryFromCatalog(catalog)
	addFindTool(server, registry)
	addExecuteTool(server, registry)
}

func addSearchTool(server *mcp.Server, registry *Registry) {
	mcp.AddTool(server, &mcp.Tool{
		Name:         searchToolName,
		Title:        "GitLab Search Tools",
		Description:  "Search the canonical GitLab action catalog for the exact action ID. Use this first whenever the exact action ID is not already known. Search with task keywords such as 'merge request merge', 'discover project from remote url', 'issue notes list', or 'pipeline job list'. Then pass the returned canonical domain.action ID to gitlab_describe_tools or gitlab_execute_tool. Do NOT invent IDs like merge_request.accept, issue.notes, or pipeline.jobs.",
		Annotations:  annotationsWithTitle(toolutil.ReadAnnotations, "GitLab Search Tools"),
		Icons:        toolutil.IconSearch,
		OutputSchema: nil,
	}, registry.Search)
}

func addDescribeTool(server *mcp.Server, registry *Registry) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        describeToolName,
		Title:       "GitLab Describe Tools",
		Description: "Describe one or more GitLab catalog actions by canonical action ID and return the exact params schema, required params, safety metadata, and an execute example. Use this before gitlab_execute_tool whenever params are not already exact. Rely on the returned schema and example for param names. Do NOT invent alias params or unsupported params.",
		Annotations: annotationsWithTitle(toolutil.ReadAnnotations, "GitLab Describe Tools"),
		Icons:       toolutil.IconConfig,
	}, registry.Describe)
}

func addFindTool(server *mcp.Server, registry *Registry) {
	mcp.AddTool(server, &mcp.Tool{
		Name:         findToolName,
		Title:        "GitLab Find Action",
		Description:  "Find GitLab catalog actions by searching with domain keywords (e.g. 'project create', 'merge request approve', 'pipeline retry', 'issue delete', 'ci variable'). Returns exact schemas, required params, safety metadata, and execute examples. ALWAYS use this before gitlab_execute_tool when the canonical action ID or params schema is not already known; do NOT invent action IDs.",
		Annotations:  annotationsWithTitle(toolutil.ReadAnnotations, "GitLab Find Action"),
		Icons:        toolutil.IconSearch,
		OutputSchema: nil,
	}, registry.Find)
}

func addExecuteTool(server *mcp.Server, registry *Registry) {
	destructiveHint := true
	openWorldHint := true
	mcp.AddTool(server, &mcp.Tool{
		Name:        executeToolName,
		Title:       "GitLab Execute Tool",
		Description: "Execute one GitLab catalog action by canonical action ID (e.g. domain.action). Always include params as an object: {\"action\":\"domain.action\",\"params\":{...}}; use params:{} only for actions with no parameters. For the 3-tool catalog, use gitlab_search_tools and gitlab_describe_tools first unless the exact action ID and all required param names are already known. For the 2-tool catalog, use gitlab_find_action first. Do NOT guess or invent action IDs. Include ONLY the exact param names from the action schema; do NOT invent extra params. Destructive actions require confirm=true.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "GitLab Execute Tool",
			DestructiveHint: &destructiveHint,
			OpenWorldHint:   &openWorldHint,
		},
		Icons: toolutil.IconServer,
	}, registry.Execute)
}

// NewRegistry builds a deterministic action registry from visible meta routes.
func NewRegistry(routes map[string]toolutil.ActionMap) *Registry {
	return NewRegistryFromCatalog(actioncatalog.FromActionMaps(routes))
}

func newRegistry(routes map[string]toolutil.ActionMap, aliases []actionAlias) *Registry {
	return newRegistryFromCatalog(actioncatalog.FromActionMaps(routes), aliases)
}

// NewRegistryFromCatalog builds a deterministic dynamic action index from the
// canonical action catalog.
func NewRegistryFromCatalog(catalog *actioncatalog.Catalog) *Registry {
	return newRegistryFromCatalog(catalog, actionAliases())
}

func newRegistryFromCatalog(catalog *actioncatalog.Catalog, aliases []actionAlias) *Registry {
	if catalog == nil {
		catalog = actioncatalog.NewCatalog()
	}
	compatibilityAliasesByCanonical := aliasesByCanonical(aliases)
	registry := &Registry{
		byID:             make(map[string]actionEntry),
		aliases:          make(map[string]string),
		ambiguousAliases: make(map[string][]string),
		handlers:         make(map[string]toolHandler),
	}
	aliasTargets := make(map[string][]string)

	for _, group := range catalog.Groups() {
		actions := group.ActionMap()
		formatResult := group.FormatResult
		if formatResult == nil {
			formatResult = toolutil.MarkdownForResult
		}
		registry.handlers[group.ToolName] = toolutil.MakeMetaHandler(group.ToolName, actions, formatResult)

		for _, action := range group.ActionsInOrder() {
			route := action.Route
			domain := action.Domain
			id := string(action.ID)
			compatibilityAliases := compatibilityAliasesByCanonical[id]
			entryAliases := dedupeStrings(append(action.Aliases, searchableAliasNames(compatibilityAliases)...))
			canonicalAliases := dedupeStrings(append(action.Aliases, aliasNames(compatibilityAliases)...))
			tags := dedupeStrings(append(action.Tags, actionTags(id, domain, action.Name, route.InputSchema)...))
			schemaURI := action.SchemaURI
			document := buildSearchDocument(id, group.ToolName, domain, action.Name, entryAliases, tags, route.InputSchema)
			entry := actionEntry{
				ID:             id,
				Tool:           group.ToolName,
				Domain:         domain,
				Action:         action.Name,
				Aliases:        entryAliases,
				Tags:           tags,
				SchemaURI:      schemaURI,
				Destructive:    route.Destructive,
				RequiredParams: requiredParams(route.InputSchema),
				Document:       document,
				SearchText:     document.FlatText,
				SearchTokens:   buildSearchTokens(document.FlatText),
				Route:          route,
			}
			registry.entries = append(registry.entries, entry)
			registry.byID[id] = entry
			for _, alias := range canonicalAliases {
				aliasTargets[alias] = append(aliasTargets[alias], id)
			}
		}
	}
	registry.indexAliases(aliasTargets)
	registry.SearchIndex = buildSearchIndex(registry.entries)

	return registry
}

func (r *Registry) indexAliases(aliasTargets map[string][]string) {
	for alias, targets := range aliasTargets {
		targets = dedupeStrings(targets)
		sort.Strings(targets)
		if len(targets) == 1 {
			r.aliases[alias] = targets[0]
			continue
		}
		r.ambiguousAliases[alias] = targets
	}
}

// Search finds GitLab catalog actions by lexical matching over action metadata.
func (r *Registry) Search(_ context.Context, _ *mcp.CallToolRequest, input SearchInput) (*mcp.CallToolResult, SearchOutput, error) {
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return toolutil.ErrorResult("gitlab_search_tools: query is required. Try terms like project create, merge request approve, pipeline retry, or ci variable."), SearchOutput{}, nil
	}

	matches := r.searchMatches(query, input.Limit, input.Explain)

	results := make([]SearchResult, 0, len(matches))
	for _, match := range matches {
		entry := match.entry
		result := SearchResult{
			ID:             entry.ID,
			Tool:           entry.Tool,
			Domain:         entry.Domain,
			Action:         entry.Action,
			SchemaURI:      entry.SchemaURI,
			Destructive:    entry.Destructive,
			RequiredParams: append([]string(nil), entry.RequiredParams...),
			Usage:          usageHintForEntry(entry),
			RelatedActions: relatedActionsForEntry(entry),
			Score:          match.score,
			LowConfidence:  match.lowConfidence,
			AmbiguousWith:  append([]string(nil), match.ambiguousWith...),
		}
		if input.Explain {
			explanation := match.explanation
			result.Explanation = &explanation
		}
		results = append(results, result)
	}

	output := SearchOutput{Query: query, Count: len(results), Results: results}
	if len(results) == 0 {
		output.Suggestions = r.suggestSearchTokens(query, 6)
	}
	return toolutil.ToolResultAnnotated(formatSearchOutput(output), toolutil.ContentList), output, nil
}

// Describe returns schemas and execution metadata for GitLab catalog actions.
func (r *Registry) Describe(_ context.Context, _ *mcp.CallToolRequest, input DescribeInput) (*mcp.CallToolResult, DescribeOutput, error) {
	ids := normalizeDescribeIDs(input)
	if len(ids) == 0 {
		return toolutil.ErrorResult("gitlab_describe_tools: provide action or actions with canonical IDs returned by the registered discovery tool for this surface."), DescribeOutput{}, nil
	}

	descriptions := make([]ActionDescription, 0, len(ids))
	for _, id := range ids {
		entry, ok := r.resolveAction(id)
		if !ok {
			return toolutil.ErrorResult(r.unknownActionMessage("gitlab_describe_tools", id)), DescribeOutput{}, nil
		}
		descriptions = append(descriptions, describeEntry(entry))
	}

	output := DescribeOutput{Count: len(descriptions), Actions: descriptions}
	return toolutil.ToolResultAnnotated(formatDescribeOutput(output), toolutil.ContentDetail), output, nil
}

// Find searches GitLab catalog actions and includes exact schemas for matches.
func (r *Registry) Find(_ context.Context, _ *mcp.CallToolRequest, input FindInput) (*mcp.CallToolResult, FindOutput, error) {
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return toolutil.ErrorResult("gitlab_find_action: query is required. Try terms like project create, merge request approve, pipeline retry, or ci variable."), FindOutput{}, nil
	}

	matches := r.searchMatches(query, input.Limit, input.Explain)
	results := make([]FindResult, 0, len(matches))
	for _, match := range matches {
		description := describeEntry(match.entry)
		result := FindResult{
			ID:             description.ID,
			Tool:           description.Tool,
			Domain:         description.Domain,
			Action:         description.Action,
			SchemaURI:      description.SchemaURI,
			Destructive:    description.Destructive,
			RequiredParams: append([]string(nil), description.RequiredParams...),
			Usage:          description.Usage,
			RelatedActions: append([]string(nil), description.RelatedActions...),
			Score:          match.score,
			LowConfidence:  match.lowConfidence,
			AmbiguousWith:  append([]string(nil), match.ambiguousWith...),
			InputSchema:    description.InputSchema,
			OutputSchema:   description.OutputSchema,
			Example:        description.Example,
		}
		if input.Explain {
			explanation := match.explanation
			result.Explanation = &explanation
		}
		results = append(results, result)
	}

	output := FindOutput{Query: query, Count: len(results), Results: results}
	return toolutil.ToolResultAnnotated(formatFindOutput(output), toolutil.ContentDetail), output, nil
}

// Execute dispatches one catalog action through the existing meta-tool handler.
func (r *Registry) Execute(ctx context.Context, req *mcp.CallToolRequest, input ExecuteInput) (*mcp.CallToolResult, any, error) {
	id := strings.ToLower(strings.TrimSpace(input.Action))
	if id == "" {
		return toolutil.ErrorResult("gitlab_execute_tool: action is required. Use the registered discovery tool for this surface to find a canonical action ID."), nil, nil
	}
	requestedActionID := id
	entry, ok := r.resolveAction(id)
	if !ok {
		return toolutil.ErrorResult(r.unknownActionMessage("gitlab_execute_tool", input.Action)), nil, nil
	}

	params := maps.Clone(input.Params)
	if params == nil {
		params = map[string]any{}
	}
	params, commonParamExplanations := toolutil.NormalizeParamAliasesForSchemaWithExplanation(params, entry.Route.InputSchema)
	params, actionParamExplanations := NormalizeActionScopedParamsWithExplanation(entry.ID, params, entry.Route.InputSchema)
	if stateEvent, lifecycleAlias := issueLifecycleAliasStateEvent(requestedActionID); lifecycleAlias && entry.ID == "issue.update" {
		if existing, hasStateEvent := params["state_event"]; hasStateEvent {
			if existingStateEvent, converted := issueStateEventValue(existing); converted && existingStateEvent != stateEvent {
				return toolutil.ErrorResult(fmt.Sprintf("gitlab_execute_tool: action %q implies state_event=%q, but params.state_event was %q. Use the canonical issue.update action for explicit state_event control.", requestedActionID, stateEvent, existingStateEvent)), nil, nil
			}
		} else {
			params["state_event"] = stateEvent
			actionParamExplanations = append(actionParamExplanations, toolutil.ParamAliasExplanation{Alias: requestedActionID, Canonical: "state_event", Source: "dynamic_action_alias", Notes: "issue lifecycle aliases execute issue.update with the matching state_event"})
		}
	}
	if len(commonParamExplanations)+len(actionParamExplanations) > 0 {
		slog.Debug("normalized dynamic action params", "action", entry.ID, "normalizations", len(commonParamExplanations)+len(actionParamExplanations))
	}
	if result := validateDynamicExecuteParams(entry, params); result != nil {
		return result, nil, nil
	}
	if input.Confirm {
		params["confirm"] = true
	}
	if entry.Destructive && !hasExplicitConfirm(params) {
		slog.Warn("blocked destructive dynamic action without explicit confirmation", "action", entry.ID)
		return toolutil.ErrorResult(fmt.Sprintf("gitlab_execute_tool: action %q is destructive. Re-send with confirm=true only after the user explicitly approves this operation.", entry.ID)), nil, nil
	}

	handler := r.handlers[entry.Tool]
	return handler(ctx, req, toolutil.MetaToolInput{Action: entry.Action, Params: params})
}

// NormalizeActionScopedParams applies compatibility aliases that are safe only
// for a specific dynamic catalog action.
func NormalizeActionScopedParams(actionID string, params, schema map[string]any) map[string]any {
	normalized, _ := NormalizeActionScopedParamsWithExplanation(actionID, params, schema)
	return normalized
}

type actionScopedParamAlias struct {
	ActionID  string
	Alias     string
	Canonical string
	Notes     string
}

func actionScopedParamAliases() []actionScopedParamAlias {
	return []actionScopedParamAlias{
		{ActionID: "job.list", Alias: "status", Canonical: "scope", Notes: "job.list uses scope for job status filtering"},
		{ActionID: "repository.file_get", Alias: "branch", Canonical: "ref", Notes: "repository.file_get reads file content at a ref"},
		{ActionID: "issue.link_create", Alias: "linked_issue_iid", Canonical: "target_issue_iid", Notes: "issue.link_create uses target_issue_iid for the linked issue"},
		{ActionID: "issue.link_create", Alias: "project_id", Canonical: "target_project_id", Notes: "same-project issue links reuse project_id as target_project_id"},
		{ActionID: "issue.update", Alias: "state_event", Canonical: "state_event", Notes: "normalized issue state event value"},
		{ActionID: "pipeline.schedule_create", Alias: "name", Canonical: "description", Notes: "pipeline schedules use description as the display name"},
		{ActionID: "pipeline.schedule_update", Alias: "name", Canonical: "description", Notes: "pipeline schedules use description as the display name"},
		{ActionID: "branch.protect", Alias: "push_access_level", Canonical: "push_access_level", Notes: "normalized GitLab access level name to numeric level"},
		{ActionID: "branch.protect", Alias: "merge_access_level", Canonical: "merge_access_level", Notes: "normalized GitLab access level name to numeric level"},
		{ActionID: "feature_flags.feature_flag_create", Alias: "new_version_flag", Canonical: "version", Notes: "feature flag creation uses version for the flag API version"},
		{ActionID: actionFeatureFlagsUserListList, Alias: "name", Canonical: "removed", Notes: "feature flag user-list listing is project-scoped and does not accept a feature flag name"},
		{ActionID: "group.group_label_update", Alias: "name", Canonical: "new_name", Notes: "group label update renames labels with new_name"},
		{ActionID: "project.member_add", Alias: "access_level", Canonical: "access_level", Notes: "normalized GitLab access level name to numeric level"},
		{ActionID: "project.member_edit", Alias: "access_level", Canonical: "access_level", Notes: "normalized GitLab access level name to numeric level"},
		{ActionID: "release.link_create", Alias: "release_tag_name", Canonical: "tag_name", Notes: "release link actions use tag_name for the parent release"},
		{ActionID: "release.link_delete", Alias: "release_tag_name", Canonical: "tag_name", Notes: "release link actions use tag_name for the parent release"},
		{ActionID: "release.link_get", Alias: "release_tag_name", Canonical: "tag_name", Notes: "release link actions use tag_name for the parent release"},
		{ActionID: "release.link_list", Alias: "release_tag_name", Canonical: "tag_name", Notes: "release link actions use tag_name for the parent release"},
		{ActionID: "release.link_update", Alias: "release_tag_name", Canonical: "tag_name", Notes: "release link actions use tag_name for the parent release"},
		{ActionID: "runner.update", Alias: "paused", Canonical: "paused", Notes: "normalized string boolean to bool"},
		{ActionID: "snippet.project_create", Alias: "file_name/content", Canonical: "files", Notes: "project snippet creation uses files entries in dynamic mode"},
		{ActionID: "snippet.project_create", Alias: "files.file_name", Canonical: "files.file_path", Notes: "snippet file entries use file_path"},
		{ActionID: "snippet.project_create", Alias: "files.action", Canonical: "files", Notes: "project snippet creation file entries do not include an action field"},
	}
}

// NormalizeActionScopedParamsWithExplanation returns normalized params plus
// name-only metadata for action-scoped compatibility aliases and coercions.
func NormalizeActionScopedParamsWithExplanation(actionID string, params, schema map[string]any) (map[string]any, []toolutil.ParamAliasExplanation) {
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
	record := func(alias, canonical, notes string) {
		explanations = append(explanations, toolutil.ParamAliasExplanation{Alias: alias, Canonical: canonical, Source: "dynamic_action_scoped", Notes: notes})
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
	case actionFeatureFlagsUserListList:
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

func issueLifecycleAliasStateEvent(actionID string) (string, bool) {
	switch actionID {
	case "issue.close":
		return "close", true
	case "issue.reopen":
		return "reopen", true
	default:
		return "", false
	}
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

func validateDynamicExecuteParams(entry actionEntry, params map[string]any) *mcp.CallToolResult {
	validParams := dynamicSchemaParamNames(entry.Route.InputSchema)
	if len(validParams) == 0 {
		return nil
	}
	unknown := unknownDynamicParamNames(params, validParams)
	missing := missingDynamicRequiredParams(entry.Route.InputSchema, params)
	if len(unknown) == 0 && len(missing) == 0 {
		return nil
	}
	parts := []string{fmt.Sprintf("gitlab_execute_tool/%s: invalid params.", entry.ID)}
	if len(unknown) > 0 {
		parts = append(parts, fmt.Sprintf("Unknown params: %s.", strings.Join(unknown, ", ")))
		if suggestions := unknownParamSuggestions(unknown, validParams); len(suggestions) > 0 {
			parts = append(parts, fmt.Sprintf("Did you mean %s?", strings.Join(suggestions, ", ")))
		}
	}
	if len(missing) > 0 {
		parts = append(parts, fmt.Sprintf("Missing required params: %s.", strings.Join(missing, ", ")))
	}
	parts = append(parts, fmt.Sprintf("Valid params: %s.", strings.Join(validParams, ", ")))
	return toolutil.ErrorResult(strings.Join(parts, " "))
}

func dynamicSchemaParamNames(schema map[string]any) []string {
	properties := actionSchemaProperties(schema)
	if len(properties) == 0 {
		return nil
	}
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	return dedupeSortedStrings(names)
}

func unknownDynamicParamNames(params map[string]any, validParams []string) []string {
	if len(params) == 0 || len(validParams) == 0 {
		return nil
	}
	valid := make(map[string]struct{}, len(validParams))
	for _, name := range validParams {
		valid[name] = struct{}{}
	}
	unknown := make([]string, 0)
	for name := range params {
		if name == "confirm" {
			continue
		}
		if _, ok := valid[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	return dedupeSortedStrings(unknown)
}

func missingDynamicRequiredParams(schema, params map[string]any) []string {
	missing := make([]string, 0)
	for _, name := range rootRequiredParams(schema) {
		if _, ok := params[name]; !ok {
			missing = append(missing, name)
		}
	}
	missing = append(missing, missingAlternativeRequiredParams(schema, params)...)
	return dedupeSortedStrings(missing)
}

func rootRequiredParams(schema map[string]any) []string {
	if schema == nil {
		return nil
	}
	return appendRequiredParamNames(nil, schema["required"])
}

func missingAlternativeRequiredParams(schema, params map[string]any) []string {
	groups := alternativeRequiredParamGroups(schema)
	if len(groups) == 0 {
		return nil
	}
	bestMissing := make([]string, 0)
	for index, group := range groups {
		missing := make([]string, 0)
		for _, name := range group {
			if _, ok := params[name]; !ok {
				missing = append(missing, name)
			}
		}
		if len(missing) == 0 {
			return nil
		}
		if index == 0 || len(missing) < len(bestMissing) {
			bestMissing = missing
		}
	}
	return bestMissing
}

func alternativeRequiredParamGroups(schema map[string]any) [][]string {
	if schema == nil {
		return nil
	}
	for _, keyword := range []string{"anyOf", "oneOf"} {
		alternatives, ok := schema[keyword].([]any)
		if !ok || len(alternatives) == 0 {
			continue
		}
		groups := make([][]string, 0, len(alternatives))
		for _, raw := range alternatives {
			alternative, isObject := raw.(map[string]any)
			if !isObject {
				continue
			}
			if required := appendRequiredParamNames(nil, alternative["required"]); len(required) > 0 {
				groups = append(groups, required)
			}
		}
		return groups
	}
	return nil
}

func unknownParamSuggestions(unknown, validParams []string) []string {
	suggestions := make([]string, 0, len(unknown))
	for _, name := range unknown {
		if suggestion := closestDynamicParamName(name, validParams); suggestion != "" {
			suggestions = append(suggestions, fmt.Sprintf("%s -> %s", name, suggestion))
		}
	}
	return suggestions
}

func closestDynamicParamName(name string, validParams []string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	best := ""
	bestDistance := 4
	for _, candidate := range validParams {
		distance, ok := boundedLevenshtein(name, candidate, 3)
		if ok && distance < bestDistance {
			best = candidate
			bestDistance = distance
			continue
		}
		if best == "" && strings.Contains(candidate, name) {
			best = candidate
		}
	}
	return best
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

func annotationsWithTitle(base *mcp.ToolAnnotations, title string) *mcp.ToolAnnotations {
	if base == nil {
		return &mcp.ToolAnnotations{Title: title}
	}
	annotation := *base
	annotation.Title = title
	return &annotation
}

func buildSearchDocument(id, tool, domain, action string, aliases, tags []string, schema map[string]any) searchDocument {
	document := searchDocument{
		Backend:          "gitlab",
		Capability:       inferCapability(domain, action),
		Resource:         strings.ToLower(strings.TrimSpace(domain)),
		Operation:        strings.ToLower(strings.TrimSpace(action)),
		Scope:            inferActionScope(domain, schema),
		CanonicalID:      strings.ToLower(strings.TrimSpace(id)),
		IDWords:          splitSearchFieldWords(id),
		Tool:             strings.ToLower(strings.TrimSpace(tool)),
		Domain:           strings.ToLower(strings.TrimSpace(domain)),
		DomainWords:      splitSearchFieldWords(domain),
		Action:           strings.ToLower(strings.TrimSpace(action)),
		ActionWords:      splitSearchFieldWords(action),
		Aliases:          dedupeStrings(aliases),
		Tags:             dedupeStrings(tags),
		RequiredParams:   requiredParams(schema),
		OptionalParams:   optionalParams(schema),
		SchemaProperties: schemaPropertyNames(schema),
		SchemaEnums:      schemaPropertyEnumValues(schema),
		SchemaDescTerms:  schemaPropertyDescriptions(schema),
	}

	parts := []string{
		document.Backend,
		document.Capability,
		document.Resource,
		document.Operation,
		document.Scope,
		document.CanonicalID,
		strings.Join(document.IDWords, " "),
		document.Tool,
		document.Domain,
		strings.Join(document.DomainWords, " "),
		document.Action,
		strings.Join(document.ActionWords, " "),
	}
	for _, alias := range document.Aliases {
		parts = append(parts, alias, strings.Join(splitSearchFieldWords(alias), " "))
	}
	parts = append(parts, document.Tags...)
	parts = append(parts, document.RequiredParams...)
	parts = append(parts, document.OptionalParams...)
	for _, name := range document.SchemaProperties {
		parts = append(parts, name, strings.Join(splitSearchFieldWords(name), " "))
	}
	parts = append(parts, document.SchemaEnums...)
	parts = append(parts, document.SchemaDescTerms...)
	document.FlatText = strings.ToLower(strings.Join(parts, " "))
	return document
}

func inferCapability(domain, action string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	action = strings.ToLower(strings.TrimSpace(action))
	switch {
	case domain == "merge_request" || domain == "mr_review" || strings.HasPrefix(domain, "mr_"):
		return "code_review"
	case domain == "issue" || strings.Contains(action, "issue"):
		return "work_item"
	case domain == "pipeline" || domain == "job" || strings.HasPrefix(domain, "ci_"):
		return "ci_cd"
	case domain == "repository" || domain == "branch" || domain == "tag" || domain == "commit":
		return "source_control"
	case domain == "release" || domain == "package":
		return "delivery"
	case domain == "project" || domain == "group" || domain == "user":
		return "collaboration"
	default:
		return domain
	}
}

func inferActionScope(domain string, schema map[string]any) string {
	properties := actionSchemaProperties(schema)
	if _, ok := properties["project_id"]; ok {
		return "project"
	}
	if _, ok := properties["group_id"]; ok {
		return "group"
	}
	switch strings.ToLower(strings.TrimSpace(domain)) {
	case "admin", "server":
		return "instance"
	case "user", "users":
		return "user"
	default:
		return "gitlab"
	}
}

func splitSearchFieldWords(value string) []string {
	fields := strings.Fields(strings.ToLower(strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(value)))
	return dedupeStrings(fields)
}

func schemaPropertyNames(schema map[string]any) []string {
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	return dedupeSortedStrings(names)
}

func optionalParams(schema map[string]any) []string {
	properties := schemaPropertyNames(schema)
	if len(properties) == 0 {
		return nil
	}
	required := make(map[string]struct{}, len(properties))
	for _, name := range requiredParams(schema) {
		required[name] = struct{}{}
	}
	optional := make([]string, 0, len(properties))
	for _, name := range properties {
		if _, ok := required[name]; !ok {
			optional = append(optional, name)
		}
	}
	return dedupeSortedStrings(optional)
}

func schemaPropertyDescriptions(schema map[string]any) []string {
	properties := actionSchemaProperties(schema)
	if len(properties) == 0 {
		return nil
	}
	values := make([]string, 0, len(properties))
	for _, raw := range properties {
		property, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		description, ok := property["description"].(string)
		if !ok || strings.TrimSpace(description) == "" {
			continue
		}
		values = append(values, strings.Join(splitSearchFieldWords(description), " "))
	}
	return dedupeSortedStrings(values)
}

func schemaPropertyEnumValues(schema map[string]any) []string {
	properties := actionSchemaProperties(schema)
	if len(properties) == 0 {
		return nil
	}
	values := make([]string, 0)
	for _, raw := range properties {
		property, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		values = appendSchemaEnumValues(values, property["enum"])
	}
	return dedupeSortedStrings(values)
}

func appendSchemaEnumValues(values []string, raw any) []string {
	switch typed := raw.(type) {
	case []any:
		for _, value := range typed {
			values = appendSchemaEnumValue(values, value)
		}
	case []string:
		values = append(values, typed...)
	}
	return values
}

func appendSchemaEnumValue(values []string, value any) []string {
	switch typed := value.(type) {
	case string:
		return append(values, typed)
	case fmt.Stringer:
		return append(values, typed.String())
	case int, int64, float64, bool:
		return append(values, fmt.Sprint(typed))
	default:
		return values
	}
}

type searchTerm struct {
	Raw          string
	Alternatives []string
}

func normalizeSearchTerms(query string) []searchTerm {
	fields := strings.Fields(strings.ToLower(strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(query)))
	terms := make([]searchTerm, 0, len(fields))
	for _, field := range fields {
		if _, stop := searchStopWords()[field]; stop {
			continue
		}
		alternatives := []string{field}
		if synonyms, ok := searchSynonyms()[field]; ok {
			alternatives = append(alternatives, synonyms...)
		}
		if verbs, ok := verbSynonyms()[field]; ok {
			alternatives = append(alternatives, verbs...)
		}
		terms = append(terms, searchTerm{Raw: field, Alternatives: dedupeStrings(alternatives)})
	}
	return terms
}

func (r *Registry) suggestSearchTokens(query string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	terms := normalizeSearchTerms(query)
	candidates := make([]string, 0, len(r.SearchIndex.byToken))
	for token := range r.SearchIndex.byToken {
		if len(token) < 3 {
			continue
		}
		candidates = append(candidates, token)
	}
	sort.Strings(candidates)
	type suggestion struct {
		value    string
		distance int
	}
	near := make([]suggestion, 0, limit)
	for _, candidate := range candidates {
		bestDistance := 4
		for _, term := range terms {
			distance, ok := boundedLevenshtein(term.Raw, candidate, 3)
			if ok && distance < bestDistance {
				bestDistance = distance
			}
		}
		if bestDistance <= 2 {
			near = append(near, suggestion{value: candidate, distance: bestDistance})
		}
	}
	slices.SortStableFunc(near, func(a, b suggestion) int {
		if a.distance != b.distance {
			return a.distance - b.distance
		}
		return strings.Compare(a.value, b.value)
	})
	values := make([]string, 0, limit)
	seen := make(map[string]struct{}, limit)
	for _, item := range near {
		if len(values) >= limit {
			break
		}
		seen[item.value] = struct{}{}
		values = append(values, item.value)
	}
	for _, fallback := range []string{"project", "issue", "merge request", "pipeline", "branch", "user"} {
		if len(values) >= limit {
			break
		}
		if _, ok := seen[fallback]; ok {
			continue
		}
		seen[fallback] = struct{}{}
		values = append(values, fallback)
	}
	return values
}

func searchStopWords() map[string]struct{} {
	return searchStopWordsMap
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func dedupeSortedStrings(values []string) []string {
	out := dedupeStrings(values)
	sort.Strings(out)
	return out
}

var searchSynonymsMap = map[string][]string{
	"access":        {"token", "deploy", "member"},
	"approve":       {"approval", "review", "feedback"},
	"approved":      {"approval", "review", "approved"},
	"artifact":      {"job", "download"},
	"archive":       {"artifacts", "download"},
	"asset":         {"release", "link", "package", "artifact"},
	"assigned":      {"assignee", "assignee_username", "assignee_id", "list"},
	"assignee":      {"assigned", "assign", "delegate", "list"},
	"author":        {"creator", "created_by", "owner", "list"},
	"authored":      {"author", "author_username", "creator", "created_by", "owner", "list"},
	"ci":            {"pipeline", "job", "variable", "lint"},
	"closed":        {"close", "list", "filter"},
	"comment":       {"note", "discussion", "reply"},
	"container":     {"registry", "package", "image"},
	"current":       {"current_user", "self", "me", "author", "author_username", "assignee", "assignee_username", "settings"},
	"deploy":        {"deployment", "environment", "key"},
	"deployment":    {"deploy", "environment"},
	"deployments":   {"deployment", "deploy", "environment"},
	"details":       {"get"},
	"discussion":    {"comment", "thread", "note"},
	"draft":         {"wip", "work_in_progress", "proposal"},
	"env":           {"environment"},
	"file":          {"repository", "blob", "content"},
	"filter":        {"search", "query", "find", "list"},
	"github":        {"repository", "repo", "issue", "pull_request", "pr"},
	"gitlab":        {"project", "group", "repository", "issue", "merge_request", "pipeline"},
	"info":          {"get"},
	"jira":          {"issue", "ticket", "work_item"},
	"jobs":          {"job", "list"},
	"label":         {"tag", "category", "list"},
	"merged":        {"merge", "integrated", "list"},
	"metadata":      {"get", "details", "settings"},
	"me":            {"author", "author_username", "assignee", "assignee_username", "current_user", "self", "list"},
	"milestone":     {"sprint", "release", "deadline", "list"},
	"mine":          {"my", "owned", "owner", "author", "list"},
	"mr":            {"merge", "request", "merge_request"},
	"merge_request": {"merge", "request", "mr", "pull_request", "pr"},
	"my":            {"owned", "owner", "author", "assignee", "list"},
	"note":          {"comment", "discussion", "reply"},
	"open":          {"active", "unresolved", "status_open", "list"},
	"owned":         {"my", "personal", "mine", "owner", "list"},
	"pending":       {"list", "filter", "todo"},
	"package":       {"registry", "generic_package", "container", "artifact"},
	"pr":            {"merge", "request", "merge_request", "pull_request", "mr"},
	"pull_request":  {"merge", "request", "merge_request", "mr", "pr"},
	"read":          {"get", "file", "content", "settings"},
	"registry":      {"package", "container", "image"},
	"release":       {"tag", "asset", "link", "notes"},
	"releases":      {"release", "list"},
	"remote":        {"url", "git", "origin", "repository", "discover", "resolve"},
	"refs":          {"ref", "compare", "repository"},
	"review":        {"approval", "feedback", "assessment"},
	"repo":          {"repository", "file", "tree", "branch", "tag"},
	"runner":        {"job", "ci", "pipeline"},
	"secret":        {"variable", "ci_variable", "token", "password"},
	"credential":    {"credentials", "token"},
	"credentials":   {"credential", "token"},
	"show":          {"get"},
	"state":         {"status", "condition", "filter", "list"},
	"ticket":        {"issue", "work_item"},
	"unresolved":    {"open", "active", "list"},
	"user":          {"username", "user_id", "author_username", "assignee_username", "current_user", "member"},
	"users":         {"username", "user_id", "author_username", "assignee_username", "current_user", "member"},
	"tokens":        {"token"},
	"verify":        {"get", "exists"},
	"webhook":       {"hook"},
	"webhooks":      {"hook"},
	"yaml":          {"ci", "lint", "template"},
	"yml":           {"ci", "lint", "template"},
}

func searchSynonyms() map[string][]string {
	return searchSynonymsMap
}

var verbSynonymsMap = map[string][]string{
	"add":       {"create", "enable", "register"},
	"cancel":    {"stop"},
	"close":     {"update", "state_event", "closed"},
	"create":    {"add", "new", "register"},
	"debug":     {"diagnose", "trace", "log", "status"},
	"diagnose":  {"debug", "trace", "log", "status"},
	"disable":   {"delete", "remove", "stop"},
	"download":  {"artifact", "trace", "raw", "content", "single"},
	"destroy":   {"delete", "remove"},
	"edit":      {"update", "set"},
	"enable":    {"add", "create", "register"},
	"fetch":     {"get", "list", "read"},
	"find":      {"search", "list", "get"},
	"inspect":   {"get", "show", "status"},
	"lock":      {"protect"},
	"logs":      {"log", "trace", "job"},
	"new":       {"create", "add"},
	"play":      {"run", "trigger"},
	"remove":    {"delete"},
	"rerun":     {"retry"},
	"revoke":    {"delete", "remove"},
	"run":       {"play", "create", "trigger"},
	"search":    {"find", "list"},
	"set":       {"update", "edit"},
	"show":      {"get", "list", "read"},
	"start":     {"run", "play", "trigger"},
	"status":    {"state", "get", "latest"},
	"trigger":   {"run", "play", "create"},
	"unlock":    {"unprotect"},
	"unapprove": {"reset", "approval"},
	"update":    {"edit", "set"},
}

func verbSynonyms() map[string][]string {
	return verbSynonymsMap
}

func actionTags(id, domain, action string, schema map[string]any) []string {
	var tags []string
	add := func(values ...string) {
		for _, value := range values {
			value = strings.TrimSpace(strings.ToLower(value))
			if value != "" {
				tags = append(tags, value)
			}
		}
	}

	switch {
	case strings.Contains(id, "hook_"):
		add("webhook", "web hook", "project webhook", "webhook create", "webhook add", "project hook add", "hook add")
	case strings.Contains(id, "deploy_key"):
		add("deploy key", "ssh key", "access key")
	case strings.Contains(id, "deploy_token"):
		add("deploy token", "deploy tokens", "project deploy token", "project deploy tokens", "deployment token", "credential", "credentials", "token list", "deploy token list")
	case strings.Contains(id, "member_") && domain == "project":
		add("project member", "project membership")
	case strings.Contains(id, "member_") && domain == "group":
		add("group member", "group membership")
	case domain == "discover_project":
		add("discover", "project", "remote", "url", "lookup", "resolve", "project discovery", "git remote", "remote url", "resolve project")
	case domain == "interactive":
		add("guided", "elicitation", "wizard", strings.ReplaceAll(action, "_", " "))
		switch action {
		case "project_create":
			add("project", "create", "creation", "flow", "start", "guided project creation", "guided project creation flow", "project creation flow", "project wizard", "start guided project creation")
		case "issue_create":
			add("issue", "create", "creation", "flow", "start", "guided issue creation", "guided issue creation flow", "issue creation flow", "issue wizard", "start guided issue creation")
		}
	case strings.Contains(id, "token_project") || strings.Contains(id, "token_group") || strings.Contains(id, "token_personal"):
		add("access token", "project access token", "personal access token")
	case domain == "user" && action == "current":
		add("current", "authenticated", "me", "whoami", "profile", "current user", "authenticated user", "current authenticated user", "show current user", "my profile")
	case domain == "repository" && strings.HasPrefix(action, "file_"):
		add("repository file", "repo file", "file content")
	case domain == "repository" && action == "tree":
		add("repository tree", "repository tree list", "repo tree", "list repository tree", "browse repository tree", "repository_tree", "tree list", "ref", "main")
	case domain == "merge_request":
		add("mr", "merge request")
	case domain == "ci_variable":
		add("ci variable", "ci secret", "secret", "environment variable")
	case domain == "environment":
		add("env", "deployment")
		switch {
		case strings.HasPrefix(action, "protected_"):
			add("protected environment", "environment protection", "protected environment get", "protected environment list")
		case strings.HasPrefix(action, "deployment_"):
			add("environment deployment", "deployment list", "deployment approval", "deployment approve", "deployment reject")
		}
	case domain == "feature_flags" && strings.HasPrefix(action, "ff_user_list_"):
		add("feature flag user list", "user list", "user_list_iid", "feature flag users")
	case domain == "job":
		add("ci job", "pipeline job")
		switch action {
		case "download_single_artifact":
			add("single artifact", "single file artifact", "artifact path", "artifact_path", "numeric job id", "job_id", "coverage report", "coverage/report.xml")
		case "artifacts":
			add("whole artifact archive", "archive by job id", "job_id")
		case "download_artifacts":
			add("whole artifact archive", "archive by ref", "ref_name", "job name")
		case "download_single_artifact_by_ref":
			add("single artifact", "single file artifact", "artifact_path", "ref_name", "job name")
		}
	case domain == "pipeline":
		add("ci pipeline")
		if strings.HasPrefix(action, "trigger_") {
			add("pipeline trigger")
		}
		if action == "trigger_create" {
			add("pipeline trigger create", "create trigger", "run trigger")
		}
		if strings.Contains(action, "schedule") && strings.Contains(action, "variable") {
			add("pipeline schedule variable", "schedule variable")
		}
	case domain == "admin":
		switch action {
		case "settings_get":
			add("instance settings", "application settings", "current instance settings", "read settings", "settings get")
		case "broadcast_message_list":
			add("broadcast messages", "existing broadcast messages", "message list")
		case "broadcast_message_create":
			add("create broadcast message", "maintenance banner", "broadcast banner")
		case "broadcast_message_delete":
			add("delete broadcast message", "remove maintenance banner")
		}
	case domain == "tag":
		if action == "get" {
			add("verify tag", "tag exists", "tag lookup", "release cleanup first step")
		}
	case domain == "release":
		switch action {
		case "get":
			add("verify release", "release exists", "release by tag", "tag_name")
		case "link_list":
			add("release link", "release asset link", "release asset links", "list release links", "asset link list", "tag_name")
		case "link_create", "link_update", "link_delete":
			add("release link", "release asset link", "release asset", "tag_name")
		case "delete":
			add("delete release", "remove release", "preserve tag")
		case "list":
			add("releases", "list releases", "release inventory", "release notes")
		}
	case domain == "repository" && action == "compare":
		add("compare refs", "compare branches", "compare tags", "diff between refs", "from ref", "to ref", "from", "to", "release notes", "release compare")
	case domain == "analyze" && action == "release_notes":
		add("release notes", "generate release notes", "from ref", "to ref", "from", "to")
	case domain == "package":
		switch action {
		case "list":
			add("generic packages", "package registry packages", "list packages", "package registry")
		case "delete":
			add("package delete", "package remove", "remove package", "delete package")
		case "registry_list_project":
			add("container registry", "container images", "image repositories")
		}
	case domain == "runner":
		switch action {
		case "remove":
			add("remove runner", "delete runner by id", "runner_id")
		case "delete_registered":
			add("delete runner by token", "runner authentication token")
		}
	case domain == "issue":
		switch action {
		case "note_create":
			add("issue note", "issue comment", "create note", "create comment")
		case "note_get":
			add("issue note", "issue comment", "get note", "note_id", "read one note")
		case "note_list":
			add("issue notes", "issue comments", "list notes", "list comments")
		case "note_update":
			add("issue note", "issue comment", "update note", "edit comment", "note_id")
		case "note_delete":
			add("issue note", "issue comment", "delete note", "remove comment", "note_id")
		case "time_estimate_set":
			add("issue time tracking", "set estimate", "time estimate", "estimate", "2h")
		case "spent_time_add":
			add("issue time tracking", "add spent time", "spent time", "30m", "summary")
		case "spent_time_reset":
			add("issue time tracking", "reset spent time", "clear spent time")
		case "time_estimate_reset":
			add("issue time tracking", "reset estimate", "clear estimate")
		}
	case domain == "branch" && (action == "protect" || action == "get_protected" || action == "update_protected" || action == "unprotect"):
		add("protected branch", "branch protection")
	case strings.Contains(id, "protected_env") || strings.Contains(id, "protected_environment"):
		add("protected environment", "environment protection")
	case strings.Contains(id, "member_role"):
		add("custom role", "member role")
	}

	if properties, ok := schema["properties"].(map[string]any); ok {
		for name := range properties {
			switch name {
			case "state_event":
				add("close", "reopen", "state")
			case "ref":
				add("branch", "tag", "commit")
			case "file_path":
				add("repository file", "path")
			case "url":
				add("url")
			}
		}
	}

	return dedupeStrings(tags)
}

func (r *Registry) searchMatches(query string, limit int, explain bool) []scoredActionEntry {
	limit = normalizedLimit(limit)
	terms := normalizeSearchTerms(query)
	searchScorer := scoreEntryWithoutExplanation
	fuzzyScorer := fuzzyScoreEntryWithoutExplanation
	if explain {
		searchScorer = scoreEntryWithExplanation
		fuzzyScorer = fuzzyScoreEntryWithExplanation
	}
	matches := r.scoredMatches(terms, searchScorer)
	fuzzyUsed := false
	destructiveFuzzySuppressions := 0
	if fuzzyModeForMatches(matches, limit) != fuzzyDisabled {
		fuzzyMatches := r.scoredMatches(terms, fuzzyScorer)
		beforeFilter := len(fuzzyMatches)
		fuzzyMatches = filterUnsafeFuzzyMatches(terms, fuzzyMatches)
		destructiveFuzzySuppressions = beforeFilter - len(fuzzyMatches)
		fuzzyUsed = len(fuzzyMatches) > 0
		matches = mergeBestMatches(matches, fuzzyMatches)
	}
	if segmented := r.segmentedSearchMatchesWithScorer(terms, limit, searchScorer); len(segmented) > 0 {
		matches = mergeBestMatches(matches, segmented)
	}
	matches = sortAndLimitMatches(matches, limit)
	matches = computeConfidence(matches)
	lowConfidence := len(matches) > 0 && matches[0].lowConfidence
	ambiguousAlias := len(r.ambiguousAliasTargets(query)) > 0
	matches = r.annotateAmbiguousMatches(query, matches)
	recordSearchRuntimeMetrics(len(matches), fuzzyUsed, ambiguousAlias, lowConfidence, destructiveFuzzySuppressions)
	logDynamicSearch(query, matches, fuzzyUsed, ambiguousAlias, destructiveFuzzySuppressions)
	return matches
}

func logDynamicSearch(query string, matches []scoredActionEntry, fuzzyUsed, ambiguousAlias bool, destructiveFuzzySuppressions int) {
	topAction := ""
	lowConfidence := false
	if len(matches) > 0 {
		topAction = matches[0].entry.ID
		lowConfidence = matches[0].lowConfidence
	}
	slog.Debug("dynamic search completed",
		"query_len", len(query),
		"result_count", len(matches),
		"fuzzy_used", fuzzyUsed,
		"low_confidence", lowConfidence,
		"ambiguous_alias", ambiguousAlias,
		"destructive_fuzzy_suppressions", destructiveFuzzySuppressions,
		"top_action", topAction,
	)
}

type searchScorer func(actionEntry, []searchTerm) (int, ScoringExplanation)

func scoreEntryWithoutExplanation(entry actionEntry, terms []searchTerm) (int, ScoringExplanation) {
	return scoreEntry(entry, terms), ScoringExplanation{}
}

func (r *Registry) scoredMatches(terms []searchTerm, scorer searchScorer) []scoredActionEntry {
	matches := make([]scoredActionEntry, 0)
	for _, entryIndex := range r.SearchIndex.candidateEntryIndexes(terms) {
		if entryIndex < 0 || entryIndex >= len(r.entries) {
			continue
		}
		entry := r.entries[entryIndex]
		score, explanation := scorer(entry, terms)
		if score > 0 {
			matches = append(matches, scoredActionEntry{entry: entry, score: score, explanation: explanation})
		}
	}
	return matches
}

func fuzzyModeForMatches(matches []scoredActionEntry, limit int) fuzzyCandidateMode {
	if len(matches) == 0 {
		return fuzzyZeroResults
	}
	preview := append([]scoredActionEntry(nil), matches...)
	preview = sortAndLimitMatches(preview, limit)
	preview = computeConfidence(preview)
	if len(preview) > 0 && preview[0].lowConfidence {
		return fuzzyLowConfidence
	}
	return fuzzyDisabled
}

func filterUnsafeFuzzyMatches(terms []searchTerm, matches []scoredActionEntry) []scoredActionEntry {
	if len(matches) == 0 {
		return nil
	}
	filtered := make([]scoredActionEntry, 0, len(matches))
	for _, match := range matches {
		if match.entry.Destructive && !allowsDestructiveFuzzyMatch(terms, match.entry) {
			continue
		}
		filtered = append(filtered, match)
	}
	return filtered
}

func allowsDestructiveFuzzyMatch(terms []searchTerm, entry actionEntry) bool {
	if !hasExactDestructiveVerb(terms) {
		return false
	}
	document := documentForEntry(entry)
	for _, term := range terms {
		if termMatchesResourceSignal(term.Raw, document) {
			return true
		}
	}
	return false
}

func hasExactDestructiveVerb(terms []searchTerm) bool {
	for _, term := range terms {
		switch term.Raw {
		case "delete", "destroy", "remove", "revoke", "purge":
			return true
		}
	}
	return false
}

func termMatchesResourceSignal(term string, document searchDocument) bool {
	if term == document.Domain || slices.Contains(document.DomainWords, term) {
		return true
	}
	if term == document.Action || slices.Contains(document.ActionWords, term) {
		return true
	}
	if slices.Contains(document.Tags, term) {
		return true
	}
	return false
}

func (r *Registry) segmentedSearchMatches(terms []searchTerm) []scoredActionEntry {
	return r.segmentedSearchMatchesWithScorer(terms, defaultLimit, scoreEntryWithoutExplanation)
}

func (r *Registry) segmentedSearchMatchesWithScorer(terms []searchTerm, limit int, scorer searchScorer) []scoredActionEntry {
	if !shouldRunSegmentedSearch(terms, limit) {
		return nil
	}

	bestByID := make(map[string]scoredActionEntry)
	maxWindow := min(maxSegmentTerms, len(terms))
	for windowSize := maxWindow; windowSize >= minSegmentTerms; windowSize-- {
		for start := 0; start+windowSize <= len(terms); start++ {
			window := terms[start : start+windowSize]
			for _, match := range r.scoredMatches(window, scorer) {
				match.score += windowSize * segmentTermBoost
				match.explanation.TotalScore = match.score
				current, ok := bestByID[match.entry.ID]
				if !ok || match.score > current.score {
					bestByID[match.entry.ID] = match
				}
			}
		}
	}

	matches := make([]scoredActionEntry, 0, len(bestByID))
	for _, match := range bestByID {
		matches = append(matches, match)
	}
	return matches
}

func shouldRunSegmentedSearch(terms []searchTerm, limit int) bool {
	if len(terms) < minSegmentTerms {
		return false
	}
	if len(terms) > maxSegmentTerms {
		return true
	}
	return limit <= 10 && len(terms) >= 5
}

func computeConfidence(matches []scoredActionEntry) []scoredActionEntry {
	if len(matches) == 0 {
		return matches
	}
	margin := matches[0].score
	if len(matches) > 1 {
		margin = matches[0].score - matches[1].score
	}
	lowConfidence := matches[0].score < minimumHighConfidenceScore || margin < minimumHighConfidenceMargin
	matches[0].lowConfidence = lowConfidence
	matches[0].explanation.LowConfidence = lowConfidence
	matches[0].explanation.MarginToNext = margin
	return matches
}

func (r *Registry) annotateAmbiguousMatches(query string, matches []scoredActionEntry) []scoredActionEntry {
	targets := r.ambiguousAliasTargets(query)
	if len(targets) == 0 {
		return matches
	}
	for index := range matches {
		if slices.Contains(targets, matches[index].entry.ID) {
			matches[index].ambiguousWith = append([]string(nil), targets...)
		}
	}
	return matches
}

func sortAndLimitMatches(matches []scoredActionEntry, limit int) []scoredActionEntry {
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].entry.ID < matches[j].entry.ID
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

func mergeBestMatches(groups ...[]scoredActionEntry) []scoredActionEntry {
	bestByID := make(map[string]scoredActionEntry)
	for _, group := range groups {
		for _, match := range group {
			current, ok := bestByID[match.entry.ID]
			if !ok || match.score > current.score {
				bestByID[match.entry.ID] = match
			}
		}
	}
	matches := make([]scoredActionEntry, 0, len(bestByID))
	for _, match := range bestByID {
		matches = append(matches, match)
	}
	return matches
}

func normalizedLimit(limit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

func describeEntry(entry actionEntry) ActionDescription {
	inputSchema, _ := toolutil.LookupMetaActionSchema(map[string]toolutil.ActionMap{entry.Tool: {entry.Action: entry.Route}}, entry.Tool, entry.Action)
	return ActionDescription{
		ID:             entry.ID,
		Tool:           entry.Tool,
		Domain:         entry.Domain,
		Action:         entry.Action,
		SchemaURI:      entry.SchemaURI,
		Destructive:    entry.Destructive,
		RequiredParams: append([]string(nil), entry.RequiredParams...),
		Usage:          usageHintForEntry(entry),
		RelatedActions: relatedActionsForEntry(entry),
		InputSchema:    inputSchema,
		OutputSchema:   maps.Clone(entry.Route.OutputSchema),
		Example:        exampleFor(entry, inputSchema),
	}
}

type actionUXMetadata struct {
	Usage          string
	RelatedActions []string
}

var actionUXMetadataByID = map[string]actionUXMetadata{
	"job.download_single_artifact": {
		Usage:          "Use for one artifact file path from a known numeric job_id, for example coverage/report.xml; do not use job.artifacts or job.download_artifacts for this case.",
		RelatedActions: []string{"job.artifacts", "job.download_artifacts"},
	},
	"job.artifacts": {
		Usage:          "Downloads the whole artifact archive for a known numeric job_id; use job.download_single_artifact when one artifact_path is requested.",
		RelatedActions: []string{"job.download_single_artifact"},
	},
	"job.download_artifacts": {
		Usage:          "Downloads the whole artifact archive by ref_name and job name; do not use with numeric job_id.",
		RelatedActions: []string{"job.download_single_artifact_by_ref"},
	},
	"job.download_single_artifact_by_ref": {
		Usage:          "Downloads one artifact file by ref_name and job name; use job.download_single_artifact when the prompt gives numeric job_id.",
		RelatedActions: []string{"job.download_single_artifact"},
	},
	"admin.settings_get": {
		Usage:          "Use for current instance/application settings; broadcast_message_list only lists existing broadcast messages.",
		RelatedActions: []string{"admin.settings_update", "admin.broadcast_message_list"},
	},
	"admin.broadcast_message_list": {
		Usage:          "Lists existing broadcast messages only; it does not read current instance settings.",
		RelatedActions: []string{"admin.settings_get", "admin.broadcast_message_create"},
	},
	"admin.broadcast_message_create": {
		Usage:          "Creates a broadcast message after any requested settings read; message text goes in params.message.",
		RelatedActions: []string{"admin.settings_get", "admin.broadcast_message_list"},
	},
	"access.deploy_key_list_project": {
		Usage:          "Lists deploy keys, not deploy tokens; use access.deploy_token_list_project when credentials/tokens are requested.",
		RelatedActions: []string{"access.deploy_token_list_project"},
	},
	"access.deploy_token_list_project": {
		Usage:          "Lists deploy tokens/credentials for a project; use access.deploy_key_list_project for SSH deploy keys.",
		RelatedActions: []string{"access.deploy_key_list_project"},
	},
	"environment.protected_get": {
		Usage:          "Gets one protected environment by params.name; environment.get reads a normal environment by environment_id.",
		RelatedActions: []string{"environment.protected_list", "environment.deployment_list"},
	},
	"environment.deployment_list": {
		Usage:          "Lists deployments for an environment/project; use after environment.list or protected environment lookup when deployment approval context is needed.",
		RelatedActions: []string{"environment.list", "environment.deployment_approve_or_reject"},
	},
	"environment.deployment_approve_or_reject": {
		Usage:          "Approves or rejects a deployment and requires params.deployment_id plus params.status.",
		RelatedActions: []string{"environment.deployment_list"},
	},
	"feature_flags.ff_user_list_get": {
		Usage:          "Gets one feature flag user list by params.user_list_iid; ff_user_list_list lists all user lists and does not accept user_list_iid.",
		RelatedActions: []string{"feature_flags.ff_user_list_list", "feature_flags.ff_user_list_update"},
	},
	"feature_flags.ff_user_list_list": {
		Usage:          "Lists feature flag user lists for a project; use ff_user_list_get when a specific user_list_iid is known.",
		RelatedActions: []string{"feature_flags.ff_user_list_get"},
	},
	"feature_flags.ff_user_list_update": {
		Usage:          "Updates one feature flag user list and requires params.user_list_iid.",
		RelatedActions: []string{"feature_flags.ff_user_list_get", "feature_flags.ff_user_list_list"},
	},
	"feature_flags.ff_user_list_delete": {
		Usage:          "Deletes one feature flag user list and requires params.user_list_iid.",
		RelatedActions: []string{"feature_flags.ff_user_list_get", "feature_flags.ff_user_list_list"},
	},
	"issue.note_create": {
		Usage:          "Creates a note/comment on an issue; subsequent get/update/delete steps need the returned note_id.",
		RelatedActions: []string{"issue.note_get", "issue.note_update", "issue.note_delete"},
	},
	"issue.note_get": {
		Usage:          "Gets one issue note by params.note_id; issue.note_list lists notes and does not fetch a specific note.",
		RelatedActions: []string{"issue.note_list", "issue.note_update", "issue.note_delete"},
	},
	"issue.note_list": {
		Usage:          "Lists issue notes/comments; use issue.note_get when a specific note_id is known.",
		RelatedActions: []string{"issue.note_get"},
	},
	"issue.note_update": {
		Usage:          "Updates one issue note/comment and requires params.note_id.",
		RelatedActions: []string{"issue.note_get", "issue.note_delete"},
	},
	"issue.note_delete": {
		Usage:          "Deletes one issue note/comment and requires params.note_id.",
		RelatedActions: []string{"issue.note_get", "issue.note_update"},
	},
	"mr_review.draft_note_publish_all": {
		Usage:          "Publishes all pending draft MR review notes; use draft_note_create first when adding draft comments.",
		RelatedActions: []string{"mr_review.draft_note_create", "mr_review.draft_note_list"},
	},
	"project.get": {
		RelatedActions: []string{"project.archive", "project.delete", "project.update"},
	},
	"tag.get": {
		Usage:          "Use to verify that a tag exists before release cleanup or tag deletion workflows.",
		RelatedActions: []string{"release.get", "release.link_list", "release.delete", "tag.delete"},
	},
	"release.get": {
		Usage:          "Use to verify a release for a tag after tag.get when the workflow asks to verify both.",
		RelatedActions: []string{"tag.get", "release.link_list", "release.delete"},
	},
	"release.link_list": {
		Usage:          "Lists asset links for an existing release tag; it is not a release existence check.",
		RelatedActions: []string{"release.get", "release.link_create", "release.link_delete"},
	},
	"release.link_get": {
		Usage:          "Gets one release asset link by link_id; use release.link_list to discover link IDs for a tag.",
		RelatedActions: []string{"release.link_list", "release.link_update", "release.link_delete"},
	},
	"release.link_update": {
		Usage:          "Updates one release asset link by link_id; use release.link_list or release.link_get before editing when the ID is unknown.",
		RelatedActions: []string{"release.link_get", "release.link_list", "release.link_delete"},
	},
	"release.link_delete": {
		Usage:          "Deletes one release asset link by link_id; use release.link_list before deletion when the ID is unknown.",
		RelatedActions: []string{"release.link_get", "release.link_list"},
	},
	"repository.compare": {
		Usage:          "Compares two refs using params.from and params.to; use before analyze.release_notes when the task asks to inspect the diff.",
		RelatedActions: []string{"analyze.release_notes", "release.list", "tag.list"},
	},
	"analyze.release_notes": {
		Usage:          "Generates release notes with params.project_id, params.from, and params.to; call after requested release/compare prerequisite steps.",
		RelatedActions: []string{"repository.compare", "release.list", "tag.list"},
	},
	"package.list": {
		Usage:          "Lists GitLab package registry packages; use package.registry_list_project only for container registry image repositories.",
		RelatedActions: []string{"package.registry_list_project"},
	},
	"package.registry_list_project": {
		Usage:          "Lists container registry image repositories, not generic package registry packages.",
		RelatedActions: []string{"package.list"},
	},
	"runner.remove": {
		Usage:          "Removes a runner by numeric runner_id; runner.delete_registered is for deleting by runner authentication token.",
		RelatedActions: []string{"runner.delete_registered"},
	},
	"runner.delete_registered": {
		Usage:          "Deletes a registered runner by authentication token; use runner.remove when the prompt gives numeric runner_id.",
		RelatedActions: []string{"runner.remove"},
	},
}

func usageHintForEntry(entry actionEntry) string {
	return actionUXMetadataByID[entry.ID].Usage
}

func relatedActionsForEntry(entry actionEntry) []string {
	return append([]string(nil), actionUXMetadataByID[entry.ID].RelatedActions...)
}

func (r *Registry) resolveAction(id string) (actionEntry, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	if entry, ok := r.byID[id]; ok {
		return entry, true
	}
	if _, ambiguous := r.ambiguousAliases[id]; ambiguous {
		return actionEntry{}, false
	}
	canonical, ok := r.aliases[id]
	if !ok {
		return actionEntry{}, false
	}
	entry, ok := r.byID[canonical]
	return entry, ok
}

func (r *Registry) unknownActionMessage(toolName, action string) string {
	if targets := r.ambiguousAliasTargets(action); len(targets) > 0 {
		return fmt.Sprintf("%s: action alias %q is ambiguous. Use one canonical action ID explicitly: %s.", toolName, action, strings.Join(backtickStrings(targets), ", "))
	}
	suggestions := r.suggestActionIDs(action, 5)
	if len(suggestions) == 0 {
		return fmt.Sprintf("%s: unknown action %q. Use the registered discovery tool for this surface to find canonical action IDs.", toolName, action)
	}
	return fmt.Sprintf("%s: unknown action %q. Did you mean %s? Use canonical action IDs with gitlab_execute_tool.", toolName, action, strings.Join(suggestions, ", "))
}

func (r *Registry) ambiguousAliasTargets(action string) []string {
	action = strings.ToLower(strings.TrimSpace(action))
	targets := append([]string(nil), r.ambiguousAliases[action]...)
	return targets
}

func (r *Registry) suggestActionIDs(query string, limit int) []string {
	terms := normalizeSearchTerms(query)
	if len(terms) == 0 {
		return nil
	}
	type scoredEntry struct {
		id    string
		score int
	}
	scored := make([]scoredEntry, 0)
	for _, entry := range r.entries {
		score := 0
		for _, term := range terms {
			best := 0
			for _, alternative := range term.Alternatives {
				candidate := scoreSearchAlternative(entry, term.Raw, alternative)
				if candidate > best {
					best = candidate
				}
			}
			if best > 0 {
				score += best
			}
		}
		if score > 0 {
			scored = append(scored, scoredEntry{id: entry.ID, score: score})
		}
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].id < scored[j].id
	})
	if len(scored) > limit {
		scored = scored[:limit]
	}
	suggestions := make([]string, 0, len(scored))
	for _, entry := range scored {
		suggestions = append(suggestions, backtickString(entry.id))
	}
	return suggestions
}

func backtickStrings(values []string) []string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, backtickString(value))
	}
	return quoted
}

func backtickString(value string) string {
	return "`" + value + "`"
}

func aliasesByCanonical(aliases []actionAlias) map[string][]actionAlias {
	grouped := make(map[string][]actionAlias)
	for _, alias := range dedupeActionAliases(aliases) {
		grouped[alias.Canonical] = append(grouped[alias.Canonical], alias)
	}
	for canonical := range grouped {
		sort.Slice(grouped[canonical], func(i, j int) bool {
			return grouped[canonical][i].Alias < grouped[canonical][j].Alias
		})
	}
	return grouped
}

func aliasNames(aliases []actionAlias) []string {
	names := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		names = append(names, alias.Alias)
	}
	return dedupeSortedStrings(names)
}

func searchableAliasNames(aliases []actionAlias) []string {
	names := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		if alias.searchable() {
			names = append(names, alias.Alias)
		}
	}
	return dedupeSortedStrings(names)
}

func dedupeActionAliases(aliases []actionAlias) []actionAlias {
	seen := make(map[string]struct{}, len(aliases))
	out := make([]actionAlias, 0, len(aliases))
	for _, alias := range aliases {
		alias.Alias = strings.TrimSpace(strings.ToLower(alias.Alias))
		alias.Canonical = strings.TrimSpace(strings.ToLower(alias.Canonical))
		if alias.Alias == "" || alias.Canonical == "" {
			continue
		}
		key := alias.Alias + "\x00" + alias.Canonical
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, alias)
	}
	return out
}

type actionAlias struct {
	Alias      string
	Canonical  string
	Source     aliasSource
	Searchable bool
	Notes      string
}

type aliasSource string

const (
	aliasSourceCatalog          aliasSource = "catalog"
	aliasSourceCompatibility    aliasSource = "compatibility"
	aliasSourceProviderObserved aliasSource = "provider_observed"
	aliasSourceStandalone       aliasSource = "standalone"
	aliasSourceDeprecated       aliasSource = "deprecated"
)

func (actionAlias actionAlias) searchable() bool {
	return actionAlias.Searchable || actionAlias.Source == ""
}

func actionAliases() []actionAlias {
	return annotateCompatibilityAliases([]actionAlias{
		{Alias: "badge.create", Canonical: "project.badge_add"},
		{Alias: "badge.delete", Canonical: "project.badge_delete"},
		{Alias: "broadcast_message.create", Canonical: "admin.broadcast_message_create"},
		{Alias: "broadcast_message.delete", Canonical: "admin.broadcast_message_delete"},
		{Alias: "ci_catalog.resource_list", Canonical: "ci_catalog.list"},
		{Alias: "ci_job_token_scope.inbound_allowlist.list", Canonical: "job.token_scope_list_inbound"},
		{Alias: "deploy_key.create", Canonical: "access.deploy_key_add"},
		{Alias: "deploy_key.delete", Canonical: "access.deploy_key_delete"},
		{Alias: "deploy_key.get", Canonical: "access.deploy_key_get"},
		{Alias: "deploy_key.list", Canonical: "access.deploy_key_list_project"},
		{Alias: "deploy_key.update", Canonical: "access.deploy_key_update"},
		{Alias: "deploy_token.create", Canonical: "access.deploy_token_create_project"},
		{Alias: "deploy_token.delete", Canonical: "access.deploy_token_delete_project"},
		{Alias: "deploy_token.get", Canonical: "access.deploy_token_get_project"},
		{Alias: "deploy_token.list", Canonical: "access.deploy_token_list_project"},
		{Alias: "branch.protected_list", Canonical: "branch.get_protected"},
		{Alias: "branch.update_protection", Canonical: "branch.update_protected"},
		{Alias: "enterprise_user.group_list", Canonical: "enterprise_user.list"},
		{Alias: "external_status_check.list_project_checks", Canonical: "external_status_check.list_project"},
		{Alias: "feature_flag.list", Canonical: "feature_flags.feature_flag_list"},
		{Alias: "geo.node_list", Canonical: "geo.list"},
		{Alias: "gitlab_server.health_check", Canonical: "server.health_check"},
		{Alias: "feature_flag_user_list.create", Canonical: "feature_flags.ff_user_list_create"},
		{Alias: "feature_flag_user_list.delete", Canonical: "feature_flags.ff_user_list_delete"},
		{Alias: "feature_flag_user_list.get", Canonical: "feature_flags.ff_user_list_get"},
		{Alias: "feature_flag_user_list.list", Canonical: actionFeatureFlagsUserListList},
		{Alias: "feature_flag_user_list.update", Canonical: "feature_flags.ff_user_list_update"},
		{Alias: "feature_flags.feature_flag_user_list", Canonical: actionFeatureFlagsUserListList},
		{Alias: "feature_flags.feature_flag_user_list_list", Canonical: actionFeatureFlagsUserListList},
		{Alias: "feature_flags.feature_flag_user_lists_list", Canonical: actionFeatureFlagsUserListList},
		{Alias: "gitlab_issue.create", Canonical: "issue.create"},
		{Alias: "gitlab_issue.delete", Canonical: "issue.delete"},
		{Alias: "group.custom_member_roles_list", Canonical: "member_role.list_group"},
		{Alias: "group.ldap_link_delete", Canonical: "group.ldap_link_delete_for_provider"},
		{Alias: "issue.note.create", Canonical: "issue.note_create"},
		{Alias: "issue.note.delete", Canonical: "issue.note_delete"},
		{Alias: "issue.note.get", Canonical: "issue.note_get"},
		{Alias: "issue.note.list", Canonical: "issue.note_list"},
		{Alias: "issue.note.update", Canonical: "issue.note_update"},
		{Alias: "issue.close", Canonical: "issue.update"},
		{Alias: "issue_note.get", Canonical: "issue.note_get"},
		{Alias: "issue_note.list", Canonical: "issue.note_list"},
		{Alias: "issue_note.delete", Canonical: "issue.note_delete"},
		{Alias: "issue_note.update", Canonical: "issue.note_update"},
		{Alias: "issue.notes", Canonical: "issue.note_list"},
		{Alias: "issue.notes.list", Canonical: "issue.note_list"},
		{Alias: "issue.reopen", Canonical: "issue.update"},
		{Alias: "job.artifact_download", Canonical: "job.download_single_artifact"},
		{Alias: "pipeline.jobs", Canonical: "job.list"},
		{Alias: "merge_train.list", Canonical: "merge_train.list_project"},
		{Alias: "merge_request.accept", Canonical: "merge_request.merge"},
		{Alias: "merge_request.changes", Canonical: "mr_review.changes_get"},
		{Alias: "merge_request.emoji_award_create", Canonical: "merge_request.emoji_mr_create"},
		{Alias: "merge_request.emoji_award_delete", Canonical: "merge_request.emoji_mr_delete"},
		{Alias: "merge_request.emoji_mr_award_create", Canonical: "merge_request.emoji_mr_create"},
		{Alias: "merge_request.emoji_mr_award_delete", Canonical: "merge_request.emoji_mr_delete"},
		{Alias: "merge_request_note.create", Canonical: "mr_review.note_create"},
		{Alias: "merge_request_note.delete", Canonical: "mr_review.note_delete"},
		{Alias: "merge_request_note.get", Canonical: "mr_review.note_get"},
		{Alias: "merge_request_note.update", Canonical: "mr_review.note_update"},
		{Alias: "merge_request.add_spent_time", Canonical: "merge_request.spent_time_add"},
		{Alias: "merge_request.set_time_estimate", Canonical: "merge_request.time_estimate_set"},
		{Alias: "merge_request.time_estimate", Canonical: "merge_request.time_estimate_set"},
		{Alias: "merge_request.time_spent_add", Canonical: "merge_request.spent_time_add"},
		{Alias: "mr_review.draft_notes_publish", Canonical: "mr_review.draft_note_publish_all"},
		{Alias: "mr_review.publish", Canonical: "mr_review.draft_note_publish_all"},
		{Alias: "package.files", Canonical: "package.file_list"},
		{Alias: "package.list_generic", Canonical: "package.list"},
		{Alias: "personal_snippet.raw", Canonical: "snippet.content"},
		{Alias: "project.releases.list", Canonical: "release.list"},
		{Alias: "project.hooks.list", Canonical: "project.hook_list"},
		{Alias: "project.member_remove", Canonical: "project.member_delete"},
		{Alias: "project.member_update", Canonical: "project.member_edit"},
		{Alias: "project.schedule_storage_move", Canonical: "storage_move.schedule_project"},
		{Alias: "project.status_check_list", Canonical: "external_status_check.list_project"},
		{Alias: "project.status_checks.list", Canonical: "external_status_check.list_project"},
		{Alias: "project_member.add", Canonical: "project.member_add"},
		{Alias: "project_member.delete", Canonical: "project.member_delete"},
		{Alias: "project_member.edit", Canonical: "project.member_edit"},
		{Alias: "project_member.get", Canonical: "project.member_get"},
		{Alias: "project_member.remove", Canonical: "project.member_delete"},
		{Alias: "project_member.update", Canonical: "project.member_edit"},
		{Alias: "project_access_token.create", Canonical: "access.token_project_create"},
		{Alias: "project_access_token.revoke", Canonical: "access.token_project_revoke"},
		{Alias: "repository_tree", Canonical: "repository.tree", Source: aliasSourceCompatibility, Searchable: false, Notes: "Canonicalization compatibility alias; omitted from search to avoid over-ranking repository.tree."},
		{Alias: "repository_tree.list", Canonical: "repository.tree", Source: aliasSourceCompatibility, Searchable: false, Notes: "Canonicalization compatibility alias; omitted from search to avoid over-ranking repository.tree."},
		{Alias: "repository_file.create", Canonical: "repository.file_create"},
		{Alias: "repository_file.delete", Canonical: "repository.file_delete"},
		{Alias: "repository_file.get", Canonical: "repository.file_get"},
		{Alias: "repository_file.read", Canonical: "repository.file_get"},
		{Alias: "repository_files.get_raw_file", Canonical: "repository.file_raw"},
		{Alias: "issue.link", Canonical: "issue.link_create"},
		{Alias: "pipeline.schedule_variable_create", Canonical: "pipeline.schedule_create_variable"},
		{Alias: "pipeline.schedule_variable_delete", Canonical: "pipeline.schedule_delete_variable"},
		{Alias: "pipeline.schedule_variable_update", Canonical: "pipeline.schedule_edit_variable"},
		{Alias: "project.badge_update", Canonical: "project.badge_edit"},
		{Alias: "merge_request.time_spent_reset", Canonical: "merge_request.spent_time_reset"},
		{Alias: "generic_package.list", Canonical: "package.list"},
		{Alias: "issue_note.create", Canonical: "issue.note_create"},
		{Alias: "release.create_link", Canonical: "release.link_create"},
		{Alias: "release.asset_link.create", Canonical: "release.link_create"},
		{Alias: "release.asset_link.delete", Canonical: "release.link_delete"},
		{Alias: "release.asset_link.get", Canonical: "release.link_get"},
		{Alias: "release.asset_link.list", Canonical: "release.link_list"},
		{Alias: "release.asset_link.update", Canonical: "release.link_update"},
		{Alias: "release_link.link_list", Canonical: "release.link_list"},
		{Alias: "release.generate_notes", Canonical: "analyze.release_notes"},
		{Alias: "package.list_project", Canonical: "package.list"},
		{Alias: "package.list_project_packages", Canonical: "package.list"},
		{Alias: "variable.create", Canonical: "ci_variable.create"},
		{Alias: "group.variable.create", Canonical: "ci_variable.group_create"},
		{Alias: "group.audit_events", Canonical: "audit_event.list_group"},
		{Alias: "gitlab_discover_project", Canonical: "discover_project.resolve"},
		{Alias: "interactive_issue.create", Canonical: "interactive.issue_create"},
		{Alias: "interactive_issue_create", Canonical: "interactive.issue_create"},
		{Alias: "gitlab_interactive_issue.create", Canonical: "interactive.issue_create"},
		{Alias: "gitlab_interactive_issue_create", Canonical: "interactive.issue_create"},
		{Alias: "gitlab_interactive_mr_create", Canonical: "interactive.mr_create"},
		{Alias: "gitlab_interactive_project_create", Canonical: "interactive.project_create"},
		{Alias: "gitlab_interactive_release_create", Canonical: "interactive.release_create"},
		{Alias: "job.token_scope_remove_inbound", Canonical: "job.token_scope_remove_project"},
		{Alias: "mr_review.draft_notes_publish_all", Canonical: "mr_review.draft_note_publish_all"},
		{Alias: "repository.tag.delete", Canonical: "tag.delete"},
		{Alias: "runner.delete", Canonical: "runner.remove"},
		{Alias: "wiki.show", Canonical: "wiki.get"},
		{Alias: "webhook.add", Canonical: "project.hook_add"},
		{Alias: "webhook.create", Canonical: "project.hook_add"},
		{Alias: "webhook.delete", Canonical: "project.hook_delete"},
	})
}

// NormalizeCompatibilityActionAlias canonicalizes an unambiguous built-in
// dynamic compatibility alias without requiring a registry instance.
func NormalizeCompatibilityActionAlias(actionID string) (string, bool) {
	actionID = strings.ToLower(strings.TrimSpace(actionID))
	if actionID == "" {
		return "", false
	}
	matches := compatibilityAliasTargetIndex()[actionID]
	if len(matches) != 1 {
		return actionID, false
	}
	return matches[0], true
}

var (
	compatibilityAliasTargetIndexOnce sync.Once
	compatibilityAliasTargets         map[string][]string
)

func compatibilityAliasTargetIndex() map[string][]string {
	compatibilityAliasTargetIndexOnce.Do(func() {
		targets := make(map[string][]string)
		for _, alias := range actionAliases() {
			targets[alias.Alias] = append(targets[alias.Alias], alias.Canonical)
		}
		for alias, matches := range targets {
			targets[alias] = dedupeSortedStrings(matches)
		}
		compatibilityAliasTargets = targets
	})
	return compatibilityAliasTargets
}

func annotateCompatibilityAliases(aliases []actionAlias) []actionAlias {
	for index := range aliases {
		if aliases[index].Source == "" {
			aliases[index].Source = aliasSourceCompatibility
		}
		if aliases[index].Source != aliasSourceDeprecated && aliases[index].Notes == "" && !aliases[index].Searchable {
			aliases[index].Searchable = true
		}
	}
	return aliases
}

func scoreEntry(entry actionEntry, terms []searchTerm) int {
	if len(terms) == 0 {
		return 0
	}
	totalScore := 0
	matchedCount := 0
	for _, term := range terms {
		best := 0
		for _, alternative := range term.Alternatives {
			candidateScore := scoreSearchAlternative(entry, term.Raw, alternative)
			if candidateScore > best {
				best = candidateScore
			}
		}
		if best > 0 {
			matchedCount++
			totalScore += best
		}
	}
	if matchedCount == 0 {
		return 0
	}
	minRequired := len(terms)
	if len(terms) > 2 {
		minRequired = len(terms) - 1
	}
	if matchedCount < minRequired {
		return 0
	}
	score := totalScore * matchedCount / len(terms)
	score += scoreVerbIntentValue(entry, terms)
	score += scoreRequiredParamSignalValue(entry, terms)
	score += scoreCompoundTagSignalValue(entry, terms)
	score += scoreActionSpecificityValue(entry, terms)
	if score <= 0 {
		return 0
	}
	return score
}

func scoreEntryWithExplanation(entry actionEntry, terms []searchTerm) (int, ScoringExplanation) {
	if len(terms) == 0 {
		return 0, ScoringExplanation{}
	}
	totalScore := 0
	matchedCount := 0
	reasons := make([]MatchReason, 0, len(terms))
	for _, term := range terms {
		best := 0
		var bestReason MatchReason
		for _, alternative := range term.Alternatives {
			candidateScore, reason := scoreSearchAlternativeWithReason(entry, term.Raw, alternative)
			if candidateScore > best {
				best = candidateScore
				bestReason = reason
			}
		}
		if best > 0 {
			matchedCount++
			totalScore += best
			reasons = append(reasons, bestReason)
		}
	}
	if matchedCount == 0 {
		return 0, ScoringExplanation{}
	}
	// For short queries (1–2 terms) all terms must match.
	// For longer queries allow at most one unmatched term so that incidental
	// words like state values ("open") or prepositions don't suppress results.
	minRequired := len(terms)
	if len(terms) > 2 {
		minRequired = len(terms) - 1
	}
	if matchedCount < minRequired {
		return 0, ScoringExplanation{}
	}
	// Scale the total score by the match ratio so fully-matched entries rank
	// above partial matches.
	score := totalScore * matchedCount / len(terms)
	if adjustment, reason := scoreVerbIntent(entry, terms); adjustment != 0 {
		score += adjustment
		reasons = append(reasons, reason)
	}
	if adjustment, paramReasons := scoreRequiredParamSignals(entry, terms); adjustment != 0 {
		score += adjustment
		reasons = append(reasons, paramReasons...)
	}
	if adjustment, tagReasons := scoreCompoundTagSignals(entry, terms); adjustment != 0 {
		score += adjustment
		reasons = append(reasons, tagReasons...)
	}
	if adjustment, reason := scoreActionSpecificity(entry, terms); adjustment != 0 {
		score += adjustment
		reasons = append(reasons, reason)
	}
	if score <= 0 {
		return 0, ScoringExplanation{}
	}
	return score, ScoringExplanation{
		TotalScore:    score,
		MatchedTerms:  matchedCount,
		RequiredTerms: minRequired,
		Reasons:       reasons,
	}
}

func scoreVerbIntent(entry actionEntry, terms []searchTerm) (int, MatchReason) {
	return scoreVerbIntentFor(entry, queryVerbIntent(terms), terms)
}

func scoreVerbIntentValue(entry actionEntry, terms []searchTerm) int {
	adjustment, _ := scoreVerbIntentFor(entry, queryVerbIntent(terms), terms)
	return adjustment
}

func scoreVerbIntentFor(entry actionEntry, intent verbIntent, terms []searchTerm) (int, MatchReason) {
	if intent == "" {
		return 0, MatchReason{}
	}
	document := documentForEntry(entry)
	adjustment := 0
	switch intent {
	case verbIntentRead:
		if entry.Destructive {
			adjustment = scoreVerbIntentPenalty
		} else if isReadAction(document.Action) {
			adjustment = scoreVerbIntentBoost
		}
	case verbIntentWrite:
		if isWriteAction(document.Action) {
			adjustment = scoreVerbIntentBoost
		}
	case verbIntentDestructive:
		if isDestructiveActionName(document.Action) || entry.Destructive {
			if queryHasResourceSignal(terms, document) {
				adjustment = scoreVerbIntentBoost
				if document.Action == "delete" || document.Action == "remove" || document.Action == "revoke" {
					adjustment = scoreVerbIntentBoost * 3
				}
			} else {
				adjustment = scoreVerbIntentPenalty
			}
		}
	case verbIntentWorkflow:
		if isWorkflowAction(document.Action) {
			adjustment = scoreVerbIntentBoost
		}
	case verbIntentDiagnostic:
		if isDiagnosticAction(document.Action) || isReadAction(document.Action) {
			adjustment = scoreVerbIntentBoost
		}
	}
	if adjustment == 0 {
		return 0, MatchReason{}
	}
	return adjustment, MatchReason{Field: searchFieldVerbIntent, QueryTerm: string(intent), MatchedValue: document.Action, Score: adjustment}
}

func scoreRequiredParamSignalValue(entry actionEntry, terms []searchTerm) int {
	document := documentForEntry(entry)
	total := 0
	for _, term := range terms {
		for _, alternative := range term.Alternatives {
			if matchedSearchValue(document.RequiredParams, alternative) != "" {
				total += scoreRequiredParamBoost
				break
			}
			if matchedSearchValue(document.OptionalParams, alternative) != "" {
				total += scoreRequiredParamBoost / 2
				break
			}
		}
	}
	return total
}

func scoreRequiredParamSignals(entry actionEntry, terms []searchTerm) (int, []MatchReason) {
	document := documentForEntry(entry)
	reasons := make([]MatchReason, 0)
	for _, term := range terms {
		for _, alternative := range term.Alternatives {
			matched := matchedSearchValue(document.RequiredParams, alternative)
			field := searchFieldRequiredParam
			boost := scoreRequiredParamBoost
			if matched == "" {
				matched = matchedSearchValue(document.OptionalParams, alternative)
				field = searchFieldOptionalParam
				boost = scoreRequiredParamBoost / 2
			}
			if matched == "" {
				continue
			}
			reason := MatchReason{Field: field, QueryTerm: term.Raw, MatchedValue: matched, Score: boost}
			if term.Raw != alternative {
				reason.Alternative = alternative
			}
			reasons = append(reasons, reason)
			break
		}
	}
	total := 0
	for _, reason := range reasons {
		total += reason.Score
	}
	return total, reasons
}

func scoreCompoundTagSignals(entry actionEntry, terms []searchTerm) (int, []MatchReason) {
	document := documentForEntry(entry)
	termSet := searchTermAlternativeSet(terms)
	reasons := make([]MatchReason, 0)
	for _, tag := range document.Tags {
		words := splitSearchFieldWords(tag)
		if len(words) < 2 {
			continue
		}
		matched := true
		for _, word := range words {
			if _, ok := termSet[word]; !ok {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		reasons = append(reasons, MatchReason{Field: searchFieldTag, QueryTerm: strings.Join(words, " "), MatchedValue: tag, Score: scoreCompoundTagBoost})
	}
	return len(reasons) * scoreCompoundTagBoost, reasons
}

func scoreCompoundTagSignalValue(entry actionEntry, terms []searchTerm) int {
	document := documentForEntry(entry)
	matches := 0
	for _, tag := range document.Tags {
		words := splitSearchFieldWords(tag)
		if len(words) < 2 {
			continue
		}
		matched := true
		for _, word := range words {
			if !searchTermsContainWord(terms, word) {
				matched = false
				break
			}
		}
		if matched {
			matches++
		}
	}
	return matches * scoreCompoundTagBoost
}

func scoreActionSpecificity(entry actionEntry, terms []searchTerm) (int, MatchReason) {
	document := documentForEntry(entry)
	if len(document.ActionWords) <= 1 {
		return 0, MatchReason{}
	}
	termSet := searchTermAlternativeSet(terms)
	unmatched := 0
	for _, word := range document.ActionWords {
		if _, ok := termSet[word]; ok {
			continue
		}
		unmatched++
	}
	if unmatched == 0 {
		return 0, MatchReason{}
	}
	adjustment := unmatched * scoreUnmatchedActionWord
	return adjustment, MatchReason{Field: searchFieldSpecificity, QueryTerm: "action_words", MatchedValue: document.Action, Score: adjustment}
}

func scoreActionSpecificityValue(entry actionEntry, terms []searchTerm) int {
	document := documentForEntry(entry)
	if len(document.ActionWords) <= 1 {
		return 0
	}
	unmatched := 0
	for _, word := range document.ActionWords {
		if searchTermsContainWord(terms, word) {
			continue
		}
		unmatched++
	}
	if unmatched == 0 {
		return 0
	}
	return unmatched * scoreUnmatchedActionWord
}

func searchTermsContainWord(terms []searchTerm, word string) bool {
	for _, term := range terms {
		if term.Raw == word {
			return true
		}
		for _, alternative := range term.Alternatives {
			if alternative == word {
				return true
			}
			if slices.Contains(splitSearchFieldWords(alternative), word) {
				return true
			}
		}
	}
	return false
}

func searchTermAlternativeSet(terms []searchTerm) map[string]struct{} {
	termSet := make(map[string]struct{})
	for _, term := range terms {
		termSet[term.Raw] = struct{}{}
		for _, alternative := range term.Alternatives {
			termSet[alternative] = struct{}{}
			for _, word := range splitSearchFieldWords(alternative) {
				termSet[word] = struct{}{}
			}
		}
	}
	return termSet
}

func queryVerbIntent(terms []searchTerm) verbIntent {
	selected := verbIntent("")
	for _, term := range terms {
		intent := classifyVerbIntent(term.Raw)
		if intentPrecedence(intent) > intentPrecedence(selected) {
			selected = intent
		}
	}
	return selected
}

func classifyVerbIntent(term string) verbIntent {
	switch term {
	case "get", "list", "read", "show", "fetch", "find", "search", "download":
		return verbIntentRead
	case "create", "add", "new", "update", "edit", "set", "enable", "register":
		return verbIntentWrite
	case "delete", "destroy", "remove", "revoke", "purge":
		return verbIntentDestructive
	case "run", "rerun", "retry", "trigger", "play", "start", "cancel", "stop", "merge", "protect", "unprotect":
		return verbIntentWorkflow
	case "debug", "diagnose", "inspect", "status", "log", "logs", "trace", "lint", "test":
		return verbIntentDiagnostic
	default:
		return ""
	}
}

func intentPrecedence(intent verbIntent) int {
	switch intent {
	case verbIntentDestructive:
		return 5
	case verbIntentDiagnostic:
		return 4
	case verbIntentWorkflow:
		return 3
	case verbIntentWrite:
		return 2
	case verbIntentRead:
		return 1
	default:
		return 0
	}
}

func queryHasResourceSignal(terms []searchTerm, document searchDocument) bool {
	for _, term := range terms {
		if termMatchesResourceSignal(term.Raw, document) {
			return true
		}
	}
	return false
}

func isReadAction(action string) bool {
	return action == "get" || action == "list" || strings.HasPrefix(action, "get_") || strings.HasPrefix(action, "list_") || strings.Contains(action, "status") || strings.Contains(action, "log") || strings.Contains(action, "report") || strings.Contains(action, "raw") || strings.Contains(action, "content")
}

func isWriteAction(action string) bool {
	return strings.Contains(action, "create") || strings.Contains(action, "add") || strings.Contains(action, "update") || strings.Contains(action, "edit") || strings.Contains(action, "set") || strings.Contains(action, "enable") || strings.Contains(action, "register") || strings.Contains(action, "approve")
}

func isDestructiveActionName(action string) bool {
	return strings.Contains(action, "delete") || strings.Contains(action, "remove") || strings.Contains(action, "revoke") || strings.Contains(action, "destroy")
}

func isWorkflowAction(action string) bool {
	return strings.Contains(action, "retry") || strings.Contains(action, "trigger") || strings.Contains(action, "play") || strings.Contains(action, "run") || strings.Contains(action, "merge") || strings.Contains(action, "protect") || strings.Contains(action, "cancel")
}

func isDiagnosticAction(action string) bool {
	return strings.Contains(action, "status") || strings.Contains(action, "log") || strings.Contains(action, "trace") || strings.Contains(action, "lint") || strings.Contains(action, "test") || strings.Contains(action, "health")
}

func scoreSearchAlternative(entry actionEntry, raw, alternative string) int {
	document := documentForEntry(entry)
	switch {
	case document.CanonicalID == alternative:
		return scoreCanonicalExact
	case stringInSlice(document.Aliases, alternative):
		return scoreAliasExact
	case stringInSlice(document.Tags, alternative):
		return scoreTagExact
	case document.Action == alternative || document.Domain == alternative:
		return scoreDomainActionExact
	case slices.Contains(document.ActionWords, alternative) || slices.Contains(document.DomainWords, alternative):
		return scoreDomainActionWord
	case strings.Contains(document.CanonicalID, alternative):
		return scoreIDContains
	case containsAnySearchValue(document.DomainWords, alternative) || containsAnySearchValue(document.ActionWords, alternative):
		return scoreDomainActionContains
	case strings.Contains(document.Tool, alternative):
		return scoreFieldContainsFor(raw, alternative)
	case containsAnySearchValue(document.RequiredParams, alternative):
		return scoreParamContainsFor(raw, alternative, scoreRequiredParamMatch)
	case containsAnySearchValue(document.OptionalParams, alternative):
		return scoreParamContainsFor(raw, alternative, scoreOptionalParamMatch)
	case containsAnySearchValue(document.SchemaEnums, alternative):
		return scoreParamContainsFor(raw, alternative, scoreSchemaEnumMatch)
	case containsAnySearchValue(document.SchemaDescTerms, alternative):
		return scoreParamContainsFor(raw, alternative, scoreSchemaDescMatch)
	case containsAnySearchValue(document.SchemaProperties, alternative):
		return scoreFieldContainsFor(raw, alternative)
	case strings.Contains(document.FlatText, alternative):
		if raw == alternative {
			return scoreFieldContains
		}
		return scoreSynonymContains
	default:
		return 0
	}
}

func scoreSearchAlternativeWithReason(entry actionEntry, raw, alternative string) (int, MatchReason) {
	document := documentForEntry(entry)
	reason := func(field, matchedValue string, score int) (int, MatchReason) {
		match := MatchReason{
			Field:        field,
			QueryTerm:    raw,
			MatchedValue: matchedValue,
			Score:        score,
		}
		if raw != alternative {
			match.Alternative = alternative
		}
		return score, match
	}

	switch {
	case document.CanonicalID == alternative:
		return reason(searchFieldCanonicalID, document.CanonicalID, scoreCanonicalExact)
	case stringInSlice(document.Aliases, alternative):
		return reason(searchFieldAlias, alternative, scoreAliasExact)
	case stringInSlice(document.Tags, alternative):
		return reason(searchFieldTag, alternative, scoreTagExact)
	case document.Action == alternative:
		return reason(searchFieldAction, document.Action, scoreDomainActionExact)
	case document.Domain == alternative:
		return reason(searchFieldDomain, document.Domain, scoreDomainActionExact)
	case slices.Contains(document.ActionWords, alternative):
		return reason(searchFieldAction, alternative, scoreDomainActionWord)
	case slices.Contains(document.DomainWords, alternative):
		return reason(searchFieldDomain, alternative, scoreDomainActionWord)
	case strings.Contains(document.CanonicalID, alternative):
		return reason(searchFieldIDContains, document.CanonicalID, scoreIDContains)
	case containsAnySearchValue(document.DomainWords, alternative):
		return reason(searchFieldDomainContains, document.Domain, scoreDomainActionContains)
	case containsAnySearchValue(document.ActionWords, alternative):
		return reason(searchFieldActionContains, document.Action, scoreDomainActionContains)
	case strings.Contains(document.Tool, alternative):
		return reason(searchFieldTool, document.Tool, scoreFieldContainsFor(raw, alternative))
	case containsAnySearchValue(document.RequiredParams, alternative):
		return reason(searchFieldRequiredParam, matchedSearchValue(document.RequiredParams, alternative), scoreParamContainsFor(raw, alternative, scoreRequiredParamMatch))
	case containsAnySearchValue(document.OptionalParams, alternative):
		return reason(searchFieldOptionalParam, matchedSearchValue(document.OptionalParams, alternative), scoreParamContainsFor(raw, alternative, scoreOptionalParamMatch))
	case containsAnySearchValue(document.SchemaEnums, alternative):
		return reason(searchFieldSchemaEnum, matchedSearchValue(document.SchemaEnums, alternative), scoreParamContainsFor(raw, alternative, scoreSchemaEnumMatch))
	case containsAnySearchValue(document.SchemaDescTerms, alternative):
		return reason(searchFieldSchemaDesc, matchedSearchValue(document.SchemaDescTerms, alternative), scoreParamContainsFor(raw, alternative, scoreSchemaDescMatch))
	case containsAnySearchValue(document.SchemaProperties, alternative):
		return reason(searchFieldSchemaProperty, matchedSearchValue(document.SchemaProperties, alternative), scoreFieldContainsFor(raw, alternative))
	case strings.Contains(document.FlatText, alternative):
		if raw == alternative {
			return reason(searchFieldFlatText, alternative, scoreFieldContains)
		}
		return reason(searchFieldFlatText, alternative, scoreSynonymContains)
	default:
		return 0, MatchReason{}
	}
}

func documentForEntry(entry actionEntry) searchDocument {
	if entry.Document.CanonicalID != "" || entry.Document.FlatText != "" {
		return entry.Document
	}
	return searchDocument{
		CanonicalID:      strings.ToLower(strings.TrimSpace(entry.ID)),
		IDWords:          splitSearchFieldWords(entry.ID),
		Tool:             strings.ToLower(strings.TrimSpace(entry.Tool)),
		Domain:           strings.ToLower(strings.TrimSpace(entry.Domain)),
		DomainWords:      splitSearchFieldWords(entry.Domain),
		Action:           strings.ToLower(strings.TrimSpace(entry.Action)),
		ActionWords:      splitSearchFieldWords(entry.Action),
		Aliases:          dedupeStrings(entry.Aliases),
		Tags:             dedupeStrings(entry.Tags),
		RequiredParams:   dedupeStrings(entry.RequiredParams),
		OptionalParams:   nil,
		SchemaProperties: nil,
		SchemaEnums:      nil,
		SchemaDescTerms:  nil,
		FlatText:         strings.ToLower(entry.SearchText),
	}
}

func containsAnySearchValue(values []string, alternative string) bool {
	return matchedSearchValue(values, alternative) != ""
}

func matchedSearchValue(values []string, alternative string) string {
	for _, value := range values {
		if value == alternative || strings.Contains(value, alternative) || strings.Contains(strings.Join(splitSearchFieldWords(value), " "), alternative) {
			return value
		}
	}
	return ""
}

func scoreFieldContainsFor(raw, alternative string) int {
	if raw == alternative {
		return scoreFieldContains
	}
	return scoreSynonymContains
}

func scoreParamContainsFor(raw, alternative string, exactScore int) int {
	if raw == alternative {
		return exactScore
	}
	if exactScore <= scoreSynonymContains {
		return exactScore
	}
	return scoreSynonymContains
}

func stringInSlice(values []string, needle string) bool {
	return slices.Contains(values, needle)
}

func requiredParams(schema map[string]any) []string {
	if schema == nil {
		return nil
	}
	var names []string
	names = appendRequiredParamNames(names, schema["required"])
	names = appendPreferredAlternativeRequiredParams(names, schema)
	names = dedupeStrings(names)
	sort.Strings(names)
	return names
}

func appendRequiredParamNames(names []string, raw any) []string {
	switch values := raw.(type) {
	case []any:
		for _, value := range values {
			if name, isString := value.(string); isString && name != "" {
				names = append(names, name)
			}
		}
	case []string:
		names = append(names, values...)
	}
	return names
}

func appendPreferredAlternativeRequiredParams(names []string, schema map[string]any) []string {
	for _, keyword := range []string{"anyOf", "oneOf"} {
		alternatives, ok := schema[keyword].([]any)
		if !ok || len(alternatives) == 0 {
			continue
		}
		for _, raw := range alternatives {
			alternative, isObject := raw.(map[string]any)
			if !isObject {
				continue
			}
			names = appendRequiredParamNames(names, alternative["required"])
		}
		return names
	}
	return names
}

func normalizeDescribeIDs(input DescribeInput) []string {
	seen := make(map[string]struct{})
	var ids []string
	appendID := func(id string) {
		id = strings.ToLower(strings.TrimSpace(id))
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	appendID(input.Action)
	for _, id := range input.Actions {
		appendID(id)
	}
	return ids
}

func exampleFor(entry actionEntry, schema map[string]any) ActionExample {
	params := make(map[string]any)
	for _, name := range requiredParams(schema) {
		params[name] = placeholderForParam(name)
	}
	arguments := map[string]any{
		"action": entry.ID,
		"params": params,
	}
	if entry.Destructive {
		arguments["confirm"] = true
	}
	return ActionExample{
		Tool:      executeToolName,
		Arguments: arguments,
	}
}

func placeholderForParam(name string) any {
	switch name {
	case "project_id", "target_project_id":
		return "group/project"
	case "group_id", "namespace_id":
		return "group/subgroup"
	case "file_path", "artifact_path":
		return "path/to/file"
	case "ref", "branch", "branch_name", "target_branch", "source_branch":
		return "main"
	case "url", "remote_url", "external_url", "web_url":
		return "https://example.com"
	}
	if strings.HasSuffix(name, "_id") || name == "id" || strings.HasSuffix(name, "iid") {
		return 123
	}
	if strings.Contains(name, "date") {
		return "YYYY-MM-DD"
	}
	return "value"
}

func hasExplicitConfirm(params map[string]any) bool {
	value, ok := params["confirm"]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	}
	return false
}

func formatSearchOutput(output SearchOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## GitLab Action Search\n\n")
	if output.Count == 0 {
		fmt.Fprintf(&b, "No catalog actions matched %q.", output.Query)
		if len(output.Suggestions) > 0 {
			fmt.Fprintf(&b, " Try: %s.\n", strings.Join(backtickStrings(output.Suggestions), ", "))
		} else {
			b.WriteString(" Try broader terms such as project, issue, merge request, pipeline, branch, or user.\n")
		}
		return b.String()
	}
	fmt.Fprintf(&b, "Query: `%s`\n\n", output.Query)
	if targets := ambiguousTargetsFromSearchResults(output.Results); len(targets) > 0 {
		fmt.Fprintf(&b, "Use one canonical action ID explicitly: %s.\n\n", strings.Join(backtickStrings(targets), ", "))
	}
	withExplanations := hasSearchExplanations(output.Results)
	if withExplanations {
		b.WriteString("| Action ID | Destructive | Required Params | Why |\n")
		b.WriteString("| --- | --- | --- | --- |\n")
	} else {
		b.WriteString("| Action ID | Destructive | Required Params |\n")
		b.WriteString("| --- | --- | --- |\n")
	}
	for _, result := range output.Results {
		required := "-"
		if len(result.RequiredParams) > 0 {
			required = strings.Join(result.RequiredParams, ", ")
		}
		if withExplanations {
			fmt.Fprintf(&b, "| `%s` | %t | %s | %s |\n", result.ID, result.Destructive, required, explanationSummary(result.Explanation))
		} else {
			fmt.Fprintf(&b, "| `%s` | %t | %s |\n", result.ID, result.Destructive, required)
		}
	}
	b.WriteString("\nCall `gitlab_describe_tools` with an action ID before executing it.\n")
	return b.String()
}

func formatDescribeOutput(output DescribeOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## GitLab Action Description\n\n")
	for _, action := range output.Actions {
		fmt.Fprintf(&b, "### `%s`\n\n", action.ID)
		fmt.Fprintf(&b, "- **Tool**: `%s`\n", action.Tool)
		fmt.Fprintf(&b, "- **Action**: `%s`\n", action.Action)
		fmt.Fprintf(&b, "- **Destructive**: %t\n", action.Destructive)
		if len(action.RequiredParams) > 0 {
			fmt.Fprintf(&b, "- **Required params**: `%s`\n", strings.Join(action.RequiredParams, "`, `"))
		}
		if len(action.RelatedActions) > 0 {
			fmt.Fprintf(&b, "- **Related actions**: `%s`\n", strings.Join(action.RelatedActions, "`, `"))
		}
		fmt.Fprintf(&b, "- **Schema URI**: `%s`\n", action.SchemaURI)
		if schemaJSON := compactSchemaJSON(action.InputSchema); schemaJSON != "" {
			b.WriteString("- **Input schema**:\n\n")
			b.WriteString("```json\n")
			b.WriteString(schemaJSON)
			b.WriteString("\n```\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func compactSchemaJSON(schema map[string]any) string {
	if len(schema) == 0 {
		return ""
	}
	encoded, err := json.Marshal(schema)
	if err != nil {
		slog.Debug("dynamic action schema marshal failed", "error", err)
		return ""
	}
	return string(encoded)
}

func formatFindOutput(output FindOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## GitLab Action Finder\n\n")
	if output.Count == 0 {
		fmt.Fprintf(&b, "No catalog actions matched %q. Try broader terms such as project, issue, merge request, pipeline, branch, or user.\n", output.Query)
		return b.String()
	}
	fmt.Fprintf(&b, "Query: `%s`\n\n", output.Query)
	withExplanations := hasFindExplanations(output.Results)
	if withExplanations {
		b.WriteString("| Action ID | Score | Destructive | Required Params | Why |\n")
		b.WriteString("| --- | ---: | --- | --- | --- |\n")
	} else {
		b.WriteString("| Action ID | Score | Destructive | Required Params |\n")
		b.WriteString("| --- | ---: | --- | --- |\n")
	}
	for _, result := range output.Results {
		required := "-"
		if len(result.RequiredParams) > 0 {
			required = strings.Join(result.RequiredParams, ", ")
		}
		if withExplanations {
			fmt.Fprintf(&b, "| `%s` | %d | %t | %s | %s |\n", result.ID, result.Score, result.Destructive, required, explanationSummary(result.Explanation))
		} else {
			fmt.Fprintf(&b, "| `%s` | %d | %t | %s |\n", result.ID, result.Score, result.Destructive, required)
		}
	}
	b.WriteString("\nStructured results include exact `input_schema` values and `gitlab_execute_tool` examples for each action.\n")
	return b.String()
}
