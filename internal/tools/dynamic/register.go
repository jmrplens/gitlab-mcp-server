package dynamic

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncompat"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

var searchStopWordsMap = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "as": {}, "at": {}, "by": {}, "for": {}, "from": {}, "in": {},
	"of": {}, "on": {}, "or": {}, "please": {}, "the": {}, "to": {}, "using": {}, "via": {}, "with": {},
}

const (
	aliasProtectedEnvironment  = "protected environment"
	aliasEnvironmentProtection = "environment protection"
	aliasMergeRequest          = "merge request"

	findToolName          = "gitlab_find_action"
	executeActionToolName = "gitlab_execute_action"

	findToolDescription          = "Search the local GitLab action catalog; read-only and no GitLab API call. Use when the action ID or params are unclear; returns schemas, hints, destructive flags, and execute examples."
	executeActionToolDescription = "Execute one GitLab catalog action by canonical ID or alias. Always pass params as an object; destructive actions require top-level confirm=true. Use find first only when action or params are unclear."
	dynamicExecuteEnvelopeHint   = "Execute matches with top-level `action` and one `params` object; every Required Params key below belongs inside `params`, not beside it. Use top-level `confirm` only for destructive actions."

	defaultLimit                 = 20
	maxLimit                     = 50
	defaultMaxParamGuidanceItems = 2
	minSegmentTerms              = 3
	maxSegmentTerms              = 6
	segmentTermBoost             = 90
	toolManifestDetailURIBase    = "gitlab://tools/"

	actionAdminBroadcastMessageList = "admin.broadcast_message_list"
	actionAdminSettingsGet          = "admin.settings_get"
	actionAnalyzeReleaseNotes       = "analyze.release_notes"
	actionEnvironmentDeploymentList = "environment.deployment_list"
	actionFeatureFlagUserListGet    = "feature_flags.ff_user_list_get"
	actionFeatureFlagUserListList   = "feature_flags.ff_user_list_list"
	actionIssueNoteDelete           = "issue.note_delete"
	actionIssueNoteGet              = "issue.note_get"
	actionIssueNoteUpdate           = "issue.note_update"
	actionJobDownloadSingleArtifact = "job.download_single_artifact"
	actionReleaseGet                = "release.get"
	actionReleaseDelete             = "release.delete"
	actionReleaseLinkDelete         = "release.link_delete"
	actionReleaseLinkGet            = "release.link_get"
	actionReleaseLinkList           = "release.link_list"
	actionReleaseLinkUpdate         = "release.link_update"
	tagIssueComment                 = "issue comment"
	tagIssueNote                    = "issue note"
	tagIssueTimeTracking            = "issue time tracking"
	tagReleaseNotes                 = "release notes"
)

// SearchInput is the input for catalog search.
type SearchInput struct {
	Query   string `json:"query" jsonschema:"Search terms for GitLab actions, such as project create, merge request approve, pipeline retry, or ci variable."`
	Limit   int    `json:"limit,omitempty" jsonschema:"Maximum number of matches to return. Defaults to 20 and is capped at 50."`
	Explain bool   `json:"explain,omitempty" jsonschema:"When true, include deterministic scoring reasons for each returned action. Defaults to false to keep responses compact."`
}

// SearchResult is one matching GitLab catalog action.
type SearchResult struct {
	ID             string              `json:"id" jsonschema:"Canonical action ID to pass to gitlab_execute_action."`
	Tool           string              `json:"tool" jsonschema:"Backing meta-tool name."`
	Domain         string              `json:"domain" jsonschema:"Canonical action domain."`
	Action         string              `json:"action" jsonschema:"Action name inside the catalog group."`
	SchemaURI      string              `json:"schema_uri" jsonschema:"MCP resource URI for the action parameter schema."`
	Destructive    bool                `json:"destructive" jsonschema:"Whether this action is marked destructive and requires explicit confirmation."`
	RequiredParams []string            `json:"required_params,omitempty" jsonschema:"Required action-specific parameter names to place inside gitlab_execute_action params."`
	Usage          string              `json:"usage,omitempty" jsonschema:"Short disambiguation note for commonly confused actions."`
	WhyThisAction  string              `json:"why_this_action,omitempty" jsonschema:"Compact reason included only for close or ambiguous alternatives."`
	RelatedActions []string            `json:"related_actions,omitempty" jsonschema:"Curated nearby action IDs for workflows where ordering matters."`
	Score          int                 `json:"score" jsonschema:"Lexical relevance score for the query."`
	Explanation    *ScoringExplanation `json:"explanation,omitempty" jsonschema:"Optional scoring explanation returned only when explain is true."`
	LowConfidence  bool                `json:"low_confidence,omitempty" jsonschema:"Whether the top result is below the high-confidence score or margin threshold."`
	AmbiguousWith  []string            `json:"ambiguous_with,omitempty" jsonschema:"Other canonical action IDs that share the exact ambiguous alias used in the query."`
}

// SearchOutput is the structured output for catalog search.
type SearchOutput struct {
	Query       string         `json:"query" jsonschema:"Original search query."`
	Count       int            `json:"count" jsonschema:"Number of returned matches."`
	Results     []SearchResult `json:"results" jsonschema:"Matching GitLab catalog actions."`
	Suggestions []string       `json:"suggestions,omitempty" jsonschema:"Small set of nearby tokens or common domains to try when no results matched."`
	NextStep    string         `json:"next_step,omitempty" jsonschema:"Compact instruction for the next action-selection step after search."`
}

// DescribeInput identifies catalog actions to describe.
type DescribeInput struct {
	Action  string   `json:"action,omitempty" jsonschema:"Canonical action ID to describe, such as project.create. Use either action or actions."`
	Actions []string `json:"actions,omitempty" jsonschema:"Canonical action IDs to describe in one call."`
}

// ActionExample shows how to call gitlab_execute_action for an action.
type ActionExample struct {
	Tool      string         `json:"tool" jsonschema:"Tool to call for execution."`
	Arguments map[string]any `json:"arguments" jsonschema:"Example arguments for gitlab_execute_action."`
}

// ActionDescription describes one GitLab catalog action.
type ActionDescription struct {
	ID             string                                `json:"id" jsonschema:"Canonical action ID."`
	Tool           string                                `json:"tool" jsonschema:"Backing meta-tool name."`
	Domain         string                                `json:"domain" jsonschema:"Canonical action domain."`
	Action         string                                `json:"action" jsonschema:"Action name inside the catalog group."`
	SchemaURI      string                                `json:"schema_uri" jsonschema:"MCP resource URI for the action parameter schema."`
	Destructive    bool                                  `json:"destructive" jsonschema:"Whether this action requires explicit confirmation."`
	RequiredParams []string                              `json:"required_params,omitempty" jsonschema:"Required action-specific parameter names to place inside gitlab_execute_action params."`
	Usage          string                                `json:"usage,omitempty" jsonschema:"Short disambiguation note for commonly confused actions."`
	RelatedActions []string                              `json:"related_actions,omitempty" jsonschema:"Curated nearby action IDs for workflows where ordering matters."`
	ParamGuidance  map[string]toolutil.ParameterGuidance `json:"parameter_guidance,omitempty" jsonschema:"Parameter binding guidance for commonly confused params."`
	InputSchema    map[string]any                        `json:"input_schema" jsonschema:"Exact JSON Schema for action-specific params."`
	OutputSchema   map[string]any                        `json:"output_schema,omitempty" jsonschema:"Best-effort JSON Schema for the action result."`
	Example        ActionExample                         `json:"example" jsonschema:"Example gitlab_execute_action call."`
}

// DescribeOutput is the structured output for catalog action descriptions.
type DescribeOutput struct {
	Count   int                 `json:"count" jsonschema:"Number of described actions."`
	Actions []ActionDescription `json:"actions" jsonschema:"Detailed action descriptions."`
}

// FindInput is the input for gitlab_find_action.
type FindInput struct {
	Query   string `json:"query" jsonschema:"Search terms combining a GitLab domain or resource with a verb, filter, or object name, such as project create, merge request approve, pipeline retry, issue delete, or ci variable."`
	Limit   int    `json:"limit,omitempty" jsonschema:"Maximum number of matches to return. Defaults to 20 and is capped at 50."`
	Explain bool   `json:"explain,omitempty" jsonschema:"When true, include deterministic scoring reasons for each returned action. Defaults to false to keep responses compact."`
}

// FindResult is a matching catalog action with schema details and an execute example.
type FindResult struct {
	ID             string                                `json:"id" jsonschema:"Canonical action ID to pass to gitlab_execute_action."`
	Tool           string                                `json:"tool" jsonschema:"Backing meta-tool name."`
	Domain         string                                `json:"domain" jsonschema:"Canonical action domain."`
	Action         string                                `json:"action" jsonschema:"Action name inside the catalog group."`
	SchemaURI      string                                `json:"schema_uri" jsonschema:"MCP resource URI for the action parameter schema."`
	Destructive    bool                                  `json:"destructive" jsonschema:"Whether this action requires explicit confirmation."`
	RequiredParams []string                              `json:"required_params,omitempty" jsonschema:"Required action-specific parameter names to place inside gitlab_execute_action params."`
	Usage          string                                `json:"usage,omitempty" jsonschema:"Short disambiguation note for commonly confused actions."`
	RelatedActions []string                              `json:"related_actions,omitempty" jsonschema:"Curated nearby action IDs for workflows where ordering matters."`
	ParamGuidance  map[string]toolutil.ParameterGuidance `json:"parameter_guidance,omitempty" jsonschema:"Parameter binding guidance for commonly confused params."`
	Score          int                                   `json:"score" jsonschema:"Lexical relevance score for the query."`
	Explanation    *ScoringExplanation                   `json:"explanation,omitempty" jsonschema:"Optional scoring explanation returned only when explain is true."`
	LowConfidence  bool                                  `json:"low_confidence,omitempty" jsonschema:"Whether the top result is below the high-confidence score or margin threshold."`
	AmbiguousWith  []string                              `json:"ambiguous_with,omitempty" jsonschema:"Other canonical action IDs that share the exact ambiguous alias used in the query."`
	InputSchema    map[string]any                        `json:"input_schema" jsonschema:"Exact JSON Schema for action-specific params."`
	OutputSchema   map[string]any                        `json:"output_schema,omitempty" jsonschema:"Best-effort JSON Schema for the action result."`
	Example        ActionExample                         `json:"example" jsonschema:"Example gitlab_execute_action call."`
}

// FindOutput is the structured output for gitlab_find_action.
type FindOutput struct {
	Query   string       `json:"query" jsonschema:"Original search query."`
	Count   int          `json:"count" jsonschema:"Number of returned matches."`
	Results []FindResult `json:"results" jsonschema:"Matching GitLab catalog actions with schemas and execute examples."`
}

// ExecuteInput is the input for gitlab_execute_action.
type ExecuteInput struct {
	Action  string         `json:"action" jsonschema:"Canonical action ID returned by gitlab_find_action, or a supported compatibility alias, such as project.list, issue.update, or issue.close."`
	Params  map[string]any `json:"params" jsonschema:"Required action-specific parameters object validated by the selected action schema. Use an empty object for actions with no parameters."`
	Confirm bool           `json:"confirm,omitempty" jsonschema:"Set top-level confirm=true to explicitly approve destructive actions; do not put confirm inside params for gitlab_execute_action."`
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
	Usage          string
	RelatedActions []string
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

// RegisterCatalogFindExecuteTools registers the dynamic find and execute tools
// from the canonical action catalog.
func RegisterCatalogFindExecuteTools(server *mcp.Server, catalog *actioncatalog.Catalog) {
	registry := NewRegistryFromCatalog(catalog)
	addFindTool(server, registry)
	addExecuteActionTool(server, registry)
}

func addFindTool(server *mcp.Server, registry *Registry) {
	mcp.AddTool(server, &mcp.Tool{
		Name:         findToolName,
		Title:        "GitLab Find Action",
		Description:  findToolDescription,
		Annotations:  annotationsWithTitle(toolutil.ReadAnnotations, "GitLab Find Action"),
		Icons:        toolutil.IconSearch,
		OutputSchema: nil,
	}, registry.Find)
}

func addExecuteActionTool(server *mcp.Server, registry *Registry) {
	destructiveHint := true
	openWorldHint := true
	mcp.AddTool(server, &mcp.Tool{
		Name:         executeActionToolName,
		Title:        "GitLab Execute Action",
		Description:  executeActionToolDescription,
		OutputSchema: toolutil.ActionDispatchOutputSchema(),
		Annotations: &mcp.ToolAnnotations{
			Title:           "GitLab Execute Action",
			DestructiveHint: &destructiveHint,
			OpenWorldHint:   &openWorldHint,
		},
		Icons: toolutil.IconServer,
	}, registry.Execute)
}

// NewRegistry builds a deterministic action registry from visible meta routes.
func NewRegistry(routes map[string]toolutil.ActionMap) *Registry {
	return newRegistry(routes, actionAliases())
}

func newRegistry(routes map[string]toolutil.ActionMap, aliases []actionAlias) *Registry {
	return newRegistryFromCatalog(actioncatalog.FromActionMaps(routes), aliases)
}

// NewRegistryFromCatalog builds a deterministic dynamic action index from the
// canonical action catalog.
func NewRegistryFromCatalog(catalog *actioncatalog.Catalog) *Registry {
	return newRegistryFromCatalog(catalog, nil)
}

func newRegistryFromCatalog(catalog *actioncatalog.Catalog, aliases []actionAlias) *Registry {
	if catalog == nil {
		catalog = actioncatalog.NewCatalog()
	}
	compatibilityAliasesByCanonical := aliasesByCanonical(append(catalogActionAliases(catalog), aliases...))
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
			schemaURI := toolDetailURIForID(id)
			document := buildSearchDocument(id, group.ToolName, domain, action.Name, entryAliases, tags, route.InputSchema)
			entry := actionEntry{
				ID:             id,
				Tool:           group.ToolName,
				Domain:         domain,
				Action:         action.Name,
				Aliases:        entryAliases,
				Tags:           tags,
				Usage:          action.Usage,
				RelatedActions: append([]string(nil), action.RelatedActions...),
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
		return toolutil.ErrorResult("catalog search: query is required. Try terms like project create, merge request approve, pipeline retry, or ci variable."), SearchOutput{}, nil
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
		if match.lowConfidence || len(match.ambiguousWith) > 0 {
			result.WhyThisAction = whyThisActionForEntry(entry)
		}
		if input.Explain {
			explanation := match.explanation
			result.Explanation = &explanation
		}
		results = append(results, result)
	}

	output := SearchOutput{Query: query, Count: len(results), Results: results, NextStep: searchNextStep(results)}
	if len(results) == 0 {
		output.Suggestions = r.suggestSearchTokens(query, 6)
	}
	return toolutil.ToolResultAnnotated(formatSearchOutput(output), toolutil.ContentList), output, nil
}

// Describe returns schemas and execution metadata for GitLab catalog actions.
func (r *Registry) Describe(_ context.Context, _ *mcp.CallToolRequest, input DescribeInput) (*mcp.CallToolResult, DescribeOutput, error) {
	ids := normalizeDescribeIDs(input)
	if len(ids) == 0 {
		return toolutil.ErrorResult("catalog describe: provide action or actions with canonical IDs returned by the registered discovery tool for this surface."), DescribeOutput{}, nil
	}

	descriptions := make([]ActionDescription, 0, len(ids))
	for _, id := range ids {
		entry, ok := r.resolveAction(id)
		if !ok {
			return toolutil.ErrorResult(r.unknownActionMessage("catalog describe", id)), DescribeOutput{}, nil
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
			ParamGuidance:  cloneParameterGuidance(description.ParamGuidance),
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
		return toolutil.ErrorResult("gitlab_execute_action: action is required. Use the registered discovery tool for this surface to find a canonical action ID."), nil, nil
	}
	requestedActionID := id
	entry, ok := r.resolveAction(id)
	if !ok {
		return toolutil.ErrorResult(r.unknownActionMessage("gitlab_execute_action", input.Action)), nil, nil
	}

	params := maps.Clone(input.Params)
	if params == nil {
		params = map[string]any{}
	}
	params, actionParamExplanations := NormalizeActionScopedParamsWithExplanation(entry.ID, params, entry.Route.InputSchema)
	params, commonParamExplanations := toolutil.NormalizeParamAliasesForSchemaWithExplanation(params, entry.Route.InputSchema)
	var postCommonActionParamExplanations []toolutil.ParamAliasExplanation
	params, postCommonActionParamExplanations = NormalizeActionScopedParamsWithExplanation(entry.ID, params, entry.Route.InputSchema)
	actionParamExplanations = append(actionParamExplanations, postCommonActionParamExplanations...)
	if stateEvent, lifecycleAlias := issueLifecycleAliasStateEvent(requestedActionID); lifecycleAlias && entry.ID == "issue.update" {
		if existing, hasStateEvent := params["state_event"]; hasStateEvent {
			if existingStateEvent, converted := actioncompat.IssueStateEventValue(existing); converted && existingStateEvent != stateEvent {
				return toolutil.ErrorResult(fmt.Sprintf("gitlab_execute_action: action %q implies state_event=%q, but params.state_event was %q. Use the canonical issue.update action for explicit state_event control.", requestedActionID, stateEvent, existingStateEvent)), nil, nil
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
		return toolutil.ErrorResult(fmt.Sprintf("gitlab_execute_action: action %q is destructive. Re-send with confirm=true only after the user explicitly approves this operation.", entry.ID)), nil, nil
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

// NormalizeActionScopedParamsWithExplanation returns normalized params plus
// name-only metadata for action-scoped compatibility aliases and coercions.
func NormalizeActionScopedParamsWithExplanation(actionID string, params, schema map[string]any) (map[string]any, []toolutil.ParamAliasExplanation) {
	return actioncompat.NormalizeParamsWithExplanation(actionID, params, schema)
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
	parts := []string{fmt.Sprintf("gitlab_execute_action/%s: invalid params.", entry.ID)}
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

func actionSchemaProperties(schema map[string]any) map[string]any {
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}
	return properties
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
	for _, fallback := range []string{"project", "issue", aliasMergeRequest, "pipeline", "branch", "user"} {
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
	"access":        {"token"},
	"alias":         {"name", "project_alias", "redirect"},
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
	"compare":       {"diff", "repository", "refs", "ref"},
	"current":       {"current_user", "self", "me", "author", "author_username", "assignee", "assignee_username", "settings"},
	"custom":        {"role", "roles", "member_role", "custom_role"},
	"deploy":        {"deployment", "environment", "key"},
	"deployment":    {"deploy", "environment"},
	"deployments":   {"deployment", "deploy", "environment"},
	"details":       {"get"},
	"diff":          {"compare", "repository", "refs", "ref"},
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
	"name":          {"alias", "project_alias", "find_by_name"},
	"note":          {"comment", "discussion", "reply"},
	"open":          {"active", "unresolved", "status_open", "list"},
	"owned":         {"my", "personal", "mine", "owner", "list"},
	"pending":       {"list", "filter", "todo"},
	"path":          {"project", "repository", "discover", "resolve", "url"},
	"package":       {"registry", "generic_package", "container", "artifact"},
	"pr":            {"merge", "request", "merge_request", "pull_request", "mr"},
	"pull_request":  {"merge", "request", "merge_request", "mr", "pr"},
	"read":          {"get", "file", "content", "settings"},
	"registry":      {"package", "container", "image"},
	"release":       {"tag", "asset", "link", "notes"},
	"releases":      {"release", "list"},
	"remote":        {"url", "git", "origin", "repository", "discover", "resolve"},
	"role":          {"roles", "custom_role", "member_role", "permission"},
	"roles":         {"role", "custom_role", "member_role", "permissions"},
	"url":           {"remote", "origin", "git", "discover", "resolve", "project", "path"},
	"refs":          {"ref", "compare", "repository"},
	"ref":           {"refs", "compare"},
	"review":        {"approval", "feedback", "assessment"},
	"repo":          {"repository", "file", "tree", "branch", "tag"},
	"repository":    {"repo", "file", "tree", "branch", "tag"},
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
	"tags":          {"tag", "refs"},
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

type (
	tagCollector func(values ...string)
	actionTagger func(tagCollector, string, string, string) bool
)

var actionTaggers = []actionTagger{
	addIDPatternTags,
	addCoreDomainTags,
	addEnvironmentAndCITags,
	addAdminReleaseTags,
	addPackageRunnerIssueTags,
	addProtectionTags,
}

func actionTags(id, domain, action string, schema map[string]any) []string {
	var tags []string
	add := tagAppender(&tags)
	for _, tagger := range actionTaggers {
		if tagger(add, id, domain, action) {
			break
		}
	}
	addSchemaPropertyTags(add, schema)
	return dedupeStrings(tags)
}

func tagAppender(tags *[]string) tagCollector {
	return func(values ...string) {
		for _, value := range values {
			value = strings.TrimSpace(strings.ToLower(value))
			if value != "" {
				*tags = append(*tags, value)
			}
		}
	}
}

func addIDPatternTags(add tagCollector, id, domain, action string) bool {
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
	case strings.Contains(id, "service_account_pat"):
		addServiceAccountPATActionTags(add, domain, action)
	case strings.Contains(id, "service_account") && (domain == "project" || domain == "group"):
		addServiceAccountActionTags(add, domain, action)
	case domain == "discover_project":
		add("discover", "project", "remote", "url", "lookup", "resolve", "project discovery", "git remote", "remote url", "resolve project")
	case domain == "interactive":
		add("guided", "elicitation", "wizard", strings.ReplaceAll(action, "_", " "))
		addInteractiveActionTags(add, action)
	case strings.Contains(id, "token_project") || strings.Contains(id, "token_group") || strings.Contains(id, "token_personal"):
		add("access token", "project access token", "personal access token")
	default:
		return false
	}
	return true
}

func addServiceAccountActionTags(add tagCollector, domain, action string) {
	scope := strings.TrimSpace(domain)
	resource := scope + " service account"
	add(resource, resource+"s")
	switch action {
	case "service_account_list":
		add(resource+" list", "list "+resource+"s")
	case "service_account_create":
		add(resource+" create", "create "+resource)
	case "service_account_update":
		add(resource+" update", "update "+resource)
	case "service_account_delete":
		add(resource+" delete", "delete "+resource)
	}
}

func addServiceAccountPATActionTags(add tagCollector, domain, action string) {
	scope := strings.TrimSpace(domain)
	resource := scope + " service account personal access token"
	add(resource, resource+"s", scope+" service account pat", scope+" service account token")
	verb := strings.TrimPrefix(action, "service_account_pat_")
	if verb == action || verb == "" {
		return
	}
	add(resource+" "+verb, verb+" "+resource, verb+" "+resource+"s", scope+" service account pat "+verb, verb+" token for "+scope+" service account")
}

func addInteractiveActionTags(add tagCollector, action string) {
	switch action {
	case "project_create":
		add("project", "create", "creation", "flow", "start", "guided project creation", "guided project creation flow", "project creation flow", "project wizard", "start guided project creation")
	case "issue_create":
		add("issue", "create", "creation", "flow", "start", "guided issue creation", "guided issue creation flow", "issue creation flow", "issue wizard", "start guided issue creation")
	case "mr_create":
		add(aliasMergeRequest, "mr", "create", "creation", "flow", "start", "merge request create", "create merge request", "mr create", "create mr", "guided merge request creation", "guided mr creation", "merge request creation flow", "mr wizard", "start guided merge request creation")
	case "release_create":
		add("release", "create", "creation", "flow", "start", "guided release creation", "guided release creation flow", "release creation flow", "release wizard", "start guided release creation")
	}
}

func addCoreDomainTags(add tagCollector, _, domain, action string) bool {
	switch {
	case domain == "user" && action == "current":
		add("current", "authenticated", "me", "whoami", "profile", "current user", "authenticated user", "current authenticated user", "show current user", "my profile")
	case domain == "project":
		addProjectActionTags(add, action)
	case domain == "repository" && strings.HasPrefix(action, "file_"):
		add("repository file", "repo file", "file content")
	case domain == "repository" && action == "tree":
		add("repository tree", "repository tree list", "repo tree", "list repository tree", "browse repository tree", "repository_tree", "tree list", "ref", "main")
	case domain == "search":
		addSearchActionTags(add, action)
	case domain == "server":
		addServerActionTags(add, action)
	case domain == "ci_catalog":
		addCICatalogActionTags(add, action)
	case domain == "merge_request":
		add("mr", aliasMergeRequest)
	case domain == "mr_review":
		addMRReviewActionTags(add, action)
	case domain == "ci_variable":
		add("ci variable", "ci secret", "secret", "environment variable")
		addCIVariableActionTags(add, action)
	default:
		return false
	}
	return true
}

func addProjectActionTags(add tagCollector, action string) {
	switch action {
	case "star":
		add("star project", "add star", "favorite project", "mark project starred", "project favorite")
	case "unstar":
		add("unstar project", "remove star", "unfavorite project", "remove project favorite")
	}
}

func addSearchActionTags(add tagCollector, action string) {
	if action == "projects" {
		add("search projects", "project search", "find projects", "find repositories", "search repositories", "project name search", "repository name search")
	}
}

func addServerActionTags(add tagCollector, action string) {
	switch action {
	case "health_check":
		add("health check", "server health check", "server diagnostics", "connectivity check", "diagnostics connectivity check", "gitlab server health", "mcp server health")
	case "status":
		add("server status", "gitlab status", "mcp status")
	}
}

func addCICatalogActionTags(add tagCollector, action string) {
	if action == "list" {
		add("ci catalog", "ci/cd catalog", "catalog resources", "catalog components", "ci catalog resources", "ci catalog components", "list catalog resources", "list catalog components")
	}
}

func addMRReviewActionTags(add tagCollector, action string) {
	if action == "changes_get" {
		add("merge request changes", "mr changes", "merge request diff", "mr diff",
			"review changes", "get merge request changes", "merge request changes analyzer",
			"inspect mr changes", "inspect merge request changes", "view mr changes",
			"list mr changes", "list merge request changes", "mr changes list",
			"mr code diff")
	}
}

func addCIVariableActionTags(add tagCollector, action string) {
	switch action {
	case "instance_create":
		add("instance ci variable", "system ci variable", "global ci variable", "admin ci variable", "create instance ci variable", "create global ci variable")
	case "create":
		add("project ci variable", "create project ci variable")
	case "group_create":
		add("group ci variable", "create group ci variable")
	}
}

func addEnvironmentAndCITags(add tagCollector, _, domain, action string) bool {
	switch {
	case domain == "environment":
		add("env", "deployment")
		addEnvironmentActionTags(add, action)
	case domain == "feature_flags" && strings.HasPrefix(action, "ff_user_list_"):
		add("feature flag user list", "user list", "user_list_iid", "feature flag users")
	case domain == "job":
		add("ci job", "pipeline job")
		addJobActionTags(add, action)
	case domain == "pipeline":
		add("ci pipeline")
		addPipelineActionTags(add, action)
	default:
		return false
	}
	return true
}

func addEnvironmentActionTags(add tagCollector, action string) {
	switch {
	case strings.HasPrefix(action, "protected_"):
		add(aliasProtectedEnvironment, aliasEnvironmentProtection)
		addProtectedEnvironmentActionTags(add, action)
	case strings.HasPrefix(action, "deployment_"):
		add("environment deployment", "deployment list", "deployment approval", "deployment approve", "deployment reject")
	}
}

func addProtectedEnvironmentActionTags(add tagCollector, action string) {
	switch action {
	case "protected_protect":
		add("protect environment", "protect project environment", "project environment protect", "project deploy access", "maintainer deploy access")
	case "protected_list":
		add("protected environment list", "list protected environments")
	case "protected_get":
		add("protected environment get", "get protected environment")
	case "protected_update":
		add("protected environment update", "update protected environment")
	case "protected_unprotect":
		add("unprotect environment", "unprotect protected environment")
	}
}

func addJobActionTags(add tagCollector, action string) {
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
}

func addPipelineActionTags(add tagCollector, action string) {
	if strings.HasPrefix(action, "trigger_") {
		add("pipeline trigger")
	}
	if action == "trigger_create" {
		add("pipeline trigger create", "create trigger", "run trigger")
	}
	if strings.Contains(action, "schedule") && strings.Contains(action, "variable") {
		add("pipeline schedule variable", "schedule variable")
	}
}

func addAdminReleaseTags(add tagCollector, _, domain, action string) bool {
	switch {
	case domain == "admin":
		addAdminActionTags(add, action)
	case domain == "tag":
		if action == "get" {
			add("verify tag", "tag exists", "tag lookup", "release cleanup first step")
		}
	case domain == "release":
		addReleaseActionTags(add, action)
	case domain == "repository" && action == "compare":
		add("compare refs", "compare branches", "compare tags", "diff between refs", "from ref", "to ref", "from", "to", tagReleaseNotes, "release compare")
	case domain == "analyze":
		addAnalyzeActionTags(add, action)
	default:
		return false
	}
	return true
}

func addAnalyzeActionTags(add tagCollector, action string) {
	switch action {
	case "release_notes":
		add(tagReleaseNotes, "generate release notes", "from ref", "to ref", "from", "to")
	case "pipeline_failure":
		add("pipeline failure", "failed pipeline", "pipeline failed", "why pipeline failed", "root cause", "failed jobs", "job trace", "failure analysis")
	case "ci_config":
		add("configuration", "config", "ci configuration", "project ci configuration", "ci configuration analysis", "project ci configuration analysis", "ci config analysis", "project ci config", "analyze .gitlab-ci.yml", "gitlab ci yaml", "pipeline config", "branch ci configuration", "best practices", "maintainability")
	case "mr_changes":
		add("merge request changes", "merge request changes analyzer", "analyze merge request changes", "mr changes analysis", "code review", "diff analysis", "review merge request changes", "review merge request diff", "llm assisted analyzer", "llm-assisted analyzer")
	case "issue_summary":
		add("issue summary", "summarize issue", "issue discussion summary", "key decisions", "issue recap")
	case "mr_security":
		add("merge request security", "security review", "mr security review", "owasp", "vulnerabilities", "review security")
	case "technical_debt":
		add("technical debt", "technical debt markers", "technical-debt markers", "find technical debt markers", "todo", "fixme", "hack", "todo fixme hack", "debt markers", "branch technical debt")
	}
}

func addAdminActionTags(add tagCollector, action string) {
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
}

func addReleaseActionTags(add tagCollector, action string) {
	switch action {
	case "create":
		add("create release", "release create", "create release from ref", "release create from ref", "generate release", "new release", "tag release", "ref")
	case "get":
		add("verify release", "release exists", "release by tag", "tag_name")
	case "link_list":
		add("release link", "release asset link", "release asset links", "list release links", "asset link list", "tag_name")
	case "link_create", "link_update", "link_delete":
		add("release link", "release asset link", "release asset", "tag_name")
	case "delete":
		add("delete release", "remove release", "preserve tag")
	case "list":
		add("releases", "list releases", "release list", "list release", "release inventory", tagReleaseNotes)
	}
}

func addPackageRunnerIssueTags(add tagCollector, _, domain, action string) bool {
	switch domain {
	case "package":
		addPackageActionTags(add, action)
	case "runner":
		addRunnerActionTags(add, action)
	case "issue":
		addIssueActionTags(add, action)
	default:
		return false
	}
	return true
}

func addPackageActionTags(add tagCollector, action string) {
	switch action {
	case "list":
		add("generic packages", "package registry packages", "list packages", "package registry")
	case "delete":
		add("package delete", "package remove", "remove package", "delete package")
	case "registry_list_project":
		add("container registry", "container images", "image repositories")
	}
}

func addRunnerActionTags(add tagCollector, action string) {
	switch action {
	case "update":
		add("update runner", "runner update", "pause runner", "paused runner", "set paused", "set paused true", "runner paused true", "paused=true", "runner_id")
	case "jobs":
		add("runner jobs", "runner jobs list", "runner job list", "jobs for runner", "list runner jobs", "inspect runner jobs", "runner job history", "runner_id")
	case "remove":
		add("remove runner", "delete runner by id", "runner_id")
	case "delete_registered":
		add("delete runner by token", "runner authentication token")
	}
}

func addIssueActionTags(add tagCollector, action string) {
	switch action {
	case "create":
		add("create issue", "new issue", "open issue", "issue create")
	case "delete":
		add("delete issue", "remove issue", "destroy issue", "issue delete")
	case "link_list":
		add("issue links", "linked issues", "list issue links", "issue relationship", "issue link list")
	case "note_create":
		add(tagIssueNote, tagIssueComment, "create note", "create comment")
	case "note_get":
		add(tagIssueNote, tagIssueComment, "get note", "note_id", "read one note")
	case "note_list":
		add("issue notes", "issue comments", "list notes", "list comments")
	case "note_update":
		add(tagIssueNote, tagIssueComment, "update note", "edit comment", "note_id")
	case "note_delete":
		add(tagIssueNote, tagIssueComment, "delete note", "remove comment", "note_id")
	case "time_estimate_set":
		add(tagIssueTimeTracking, "set estimate", "time estimate", "estimate", "2h")
	case "spent_time_add":
		add(tagIssueTimeTracking, "add spent time", "spent time", "30m", "summary")
	case "spent_time_reset":
		add(tagIssueTimeTracking, "reset spent time", "clear spent time")
	case "time_estimate_reset":
		add(tagIssueTimeTracking, "reset estimate", "clear estimate")
	}
}

func addProtectionTags(add tagCollector, id, domain, action string) bool {
	switch {
	case domain == "group" && strings.Contains(id, "protected_branch"):
		add("group protected branch", "group branch protection", "protected branch rule", "branch pattern")
		addGroupProtectedBranchActionTags(add, action)
	case domain == "group" && (strings.Contains(id, "protected_env") || strings.Contains(id, "protected_environment")):
		add("group protected environment", "group environment protection", "group deployment gate", aliasProtectedEnvironment, aliasEnvironmentProtection)
		addGroupProtectedEnvironmentActionTags(add, action)
	case domain == "branch" && (action == "protect" || action == "get_protected" || action == "update_protected" || action == "unprotect"):
		add("protected branch", "branch protection")
	case strings.Contains(id, "protected_env") || strings.Contains(id, "protected_environment"):
		add(aliasProtectedEnvironment, aliasEnvironmentProtection)
	case strings.Contains(id, "member_role"):
		add("custom role", "member role")
	default:
		return false
	}
	return true
}

func addGroupProtectedBranchActionTags(add tagCollector, action string) {
	switch action {
	case "protected_branch_protect":
		add("protect group branch", "group protected branch protect", "create group protected branch", "protect branch pattern", "maintainer push access", "maintainer merge access", "maintainer push and merge access")
	case "protected_branch_list":
		add("list group protected branches", "group protected branch list")
	case "protected_branch_get":
		add("get group protected branch", "fetch group protected branch", "group protected branch get")
	case "protected_branch_update":
		add("update group protected branch", "group protected branch update", "allow force push", "force push")
	case "protected_branch_unprotect":
		add("unprotect group branch", "remove group protected branch", "group protected branch unprotect")
	}
}

func addGroupProtectedEnvironmentActionTags(add tagCollector, action string) {
	switch action {
	case "protected_env_protect":
		add("protect group environment", "group protected environment protect", "create group protected environment", "maintainer deploy access")
	case "protected_env_list":
		add("list group protected environments", "group protected environment list")
	case "protected_env_get":
		add("get group protected environment", "fetch group protected environment", "group protected environment get")
	case "protected_env_update":
		add("update group protected environment", "group protected environment update", "require approval", "approval rules")
	case "protected_env_unprotect":
		add("unprotect group environment", "remove group protected environment", "delete group protected environment", "group protected environment unprotect")
	}
}

func addSchemaPropertyTags(add tagCollector, schema map[string]any) {
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return
	}
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
	matches = adjustServiceAccountVerbScores(matches, terms)
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
	slog.Debug(
		"dynamic search completed",
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

func adjustServiceAccountVerbScores(matches []scoredActionEntry, terms []searchTerm) []scoredActionEntry {
	if !queryHasSearchWords(terms, "service", "account") {
		return matches
	}
	queryVerb := serviceAccountQueryVerb(terms)
	if queryVerb == "" {
		return matches
	}
	for index := range matches {
		document := documentForEntry(matches[index].entry)
		if !strings.Contains(document.CanonicalID, "service_account") {
			continue
		}
		actionVerb := serviceAccountActionVerb(document.Action)
		if actionVerb == "" {
			continue
		}
		if actionVerb == queryVerb {
			matches[index].score += scoreServiceAccountBoost
		} else {
			matches[index].score -= scoreServiceAccountBoost * 2
			if matches[index].score < 0 {
				matches[index].score = 0
			}
		}
		if matches[index].explanation.TotalScore != 0 {
			matches[index].explanation.TotalScore = matches[index].score
		}
	}
	return matches
}

func serviceAccountQueryVerb(terms []searchTerm) string {
	for _, verb := range []string{"create", "list", "update", "delete", "rotate", "revoke"} {
		if searchTermsContainWord(terms, verb) {
			return verb
		}
	}
	return ""
}

func shouldRunSegmentedSearch(terms []searchTerm, _ int) bool {
	if len(terms) < minSegmentTerms {
		return false
	}
	if len(terms) > maxSegmentTerms {
		return true
	}
	return len(terms) >= 5
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
	inputSchema := dynamicInputSchema(entry)
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
		ParamGuidance:  cloneParameterGuidance(entry.Route.ParameterGuidance),
		InputSchema:    inputSchema,
		OutputSchema:   maps.Clone(entry.Route.OutputSchema),
		Example:        exampleFor(entry, inputSchema),
	}
}

func dynamicInputSchema(entry actionEntry) map[string]any {
	schema, _ := toolutil.LookupMetaActionSchema(map[string]toolutil.ActionMap{entry.Tool: {entry.Action: entry.Route}}, entry.Tool, entry.Action)
	if entry.Route.InputSchema == nil {
		schema["description"] = "This dynamic action has no captured parameter schema. Send an empty params object {} unless the action description says otherwise."
	}
	if entry.Destructive {
		if properties, ok := schema["properties"].(map[string]any); ok {
			delete(properties, "confirm")
		}
		removeDynamicRequiredConfirmParam(schema)
		schema["x_destructive"] = true
		schema["x_confirmation"] = map[string]any{
			"location":    "gitlab_execute_action.confirm",
			"description": "Set top-level confirm=true on gitlab_execute_action after explicit user approval; do not put confirm inside params.",
		}
	}
	return schema
}

func removeDynamicRequiredConfirmParam(schema map[string]any) {
	switch required := schema["required"].(type) {
	case []any:
		filtered := make([]any, 0, len(required))
		for _, raw := range required {
			if value, ok := raw.(string); ok && value == "confirm" {
				continue
			}
			filtered = append(filtered, raw)
		}
		if len(filtered) == 0 {
			delete(schema, "required")
			return
		}
		schema["required"] = filtered
	case []string:
		filtered := make([]string, 0, len(required))
		for _, value := range required {
			if value != "confirm" {
				filtered = append(filtered, value)
			}
		}
		if len(filtered) == 0 {
			delete(schema, "required")
			return
		}
		schema["required"] = filtered
	}
}

func toolDetailURIForID(id string) string {
	return toolManifestDetailURIBase + id
}

type actionUXMetadata struct {
	Usage          string
	RelatedActions []string
}

var actionUXMetadataByID = map[string]actionUXMetadata{
	actionJobDownloadSingleArtifact: {
		Usage:          "Use for one artifact file path from a known numeric job_id, for example coverage/report.xml; do not use job.artifacts or job.download_artifacts for this case.",
		RelatedActions: []string{"job.artifacts", "job.download_artifacts"},
	},
	"job.artifacts": {
		Usage:          "Downloads the whole artifact archive for a known numeric job_id; use job.download_single_artifact when one artifact_path is requested.",
		RelatedActions: []string{actionJobDownloadSingleArtifact},
	},
	"job.download_artifacts": {
		Usage:          "Downloads the whole artifact archive by ref_name and job name; do not use with numeric job_id.",
		RelatedActions: []string{"job.download_single_artifact_by_ref"},
	},
	"job.download_single_artifact_by_ref": {
		Usage:          "Downloads one artifact file by ref_name and job name; use job.download_single_artifact when the prompt gives numeric job_id.",
		RelatedActions: []string{actionJobDownloadSingleArtifact},
	},
	actionAdminSettingsGet: {
		Usage:          "Use for current instance/application settings; broadcast_message_list only lists existing broadcast messages.",
		RelatedActions: []string{"admin.settings_update", actionAdminBroadcastMessageList},
	},
	actionAdminBroadcastMessageList: {
		Usage:          "Lists existing broadcast messages only; it does not read current instance settings.",
		RelatedActions: []string{actionAdminSettingsGet, "admin.broadcast_message_create"},
	},
	"admin.broadcast_message_create": {
		Usage:          "Creates a broadcast message after any requested settings read; message text goes in params.message.",
		RelatedActions: []string{actionAdminSettingsGet, actionAdminBroadcastMessageList},
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
		RelatedActions: []string{"environment.protected_list", actionEnvironmentDeploymentList},
	},
	actionEnvironmentDeploymentList: {
		Usage:          "Lists deployments for an environment/project; use after environment.list or protected environment lookup when deployment approval context is needed.",
		RelatedActions: []string{"environment.list", "environment.deployment_approve_or_reject"},
	},
	"environment.deployment_approve_or_reject": {
		Usage:          "Approves or rejects a deployment and requires params.deployment_id plus params.status.",
		RelatedActions: []string{actionEnvironmentDeploymentList},
	},
	actionFeatureFlagUserListGet: {
		Usage:          "Gets one feature flag user list by params.user_list_iid; ff_user_list_list lists all user lists and does not accept user_list_iid.",
		RelatedActions: []string{actionFeatureFlagUserListList, "feature_flags.ff_user_list_update"},
	},
	actionFeatureFlagUserListList: {
		Usage:          "Lists feature flag user lists for a project; use ff_user_list_get when a specific user_list_iid is known.",
		RelatedActions: []string{actionFeatureFlagUserListGet},
	},
	"feature_flags.ff_user_list_update": {
		Usage:          "Updates one feature flag user list and requires params.user_list_iid.",
		RelatedActions: []string{actionFeatureFlagUserListGet, actionFeatureFlagUserListList},
	},
	"feature_flags.ff_user_list_delete": {
		Usage:          "Deletes one feature flag user list and requires params.user_list_iid.",
		RelatedActions: []string{actionFeatureFlagUserListGet, actionFeatureFlagUserListList},
	},
	"issue.note_create": {
		Usage:          "Creates a note/comment on an issue; subsequent get/update/delete steps need the returned note_id.",
		RelatedActions: []string{actionIssueNoteGet, actionIssueNoteUpdate, actionIssueNoteDelete},
	},
	actionIssueNoteGet: {
		Usage:          "Gets one issue note by params.note_id; issue.note_list lists notes and does not fetch a specific note.",
		RelatedActions: []string{"issue.note_list", actionIssueNoteUpdate, actionIssueNoteDelete},
	},
	"issue.note_list": {
		Usage:          "Lists issue notes/comments; use issue.note_get when a specific note_id is known.",
		RelatedActions: []string{actionIssueNoteGet},
	},
	actionIssueNoteUpdate: {
		Usage:          "Updates one issue note/comment and requires params.note_id.",
		RelatedActions: []string{actionIssueNoteGet, actionIssueNoteDelete},
	},
	actionIssueNoteDelete: {
		Usage:          "Deletes one issue note/comment and requires params.note_id.",
		RelatedActions: []string{actionIssueNoteGet, actionIssueNoteUpdate},
	},
	"mr_review.draft_note_publish_all": {
		Usage:          "Publishes all pending draft MR review notes; use draft_note_create first when adding draft comments.",
		RelatedActions: []string{"mr_review.draft_note_create", "mr_review.draft_note_list"},
	},
	"project.get": {
		RelatedActions: []string{"project.archive", "project.delete", "project.update"},
	},
	"vulnerability.severity_count": {
		Usage:          "Returns counts of vulnerabilities grouped by severity (critical, high, medium, low, info, unknown) for one project_path. Use this when the prompt asks for a count or summary, not for the individual vulnerability records (use vulnerability.list for that).",
		RelatedActions: []string{"vulnerability.list", "vulnerability.pipeline_security_summary"},
	},
	"vulnerability.list": {
		Usage:          "Lists vulnerability records for a project_path with optional filters (severity, state, scanner, report_type). Pagination is GraphQL-based: pass first/after, not per_page. Use vulnerability.severity_count when only a count is needed.",
		RelatedActions: []string{"vulnerability.severity_count", "vulnerability.get", "vulnerability.dismiss"},
	},
	"group.epic_list": {
		Usage:          "Lists epics for a group full_path via the Work Items GraphQL API. Pagination uses first/after — do not pass per_page. Use group.epic_get when a specific epic_iid is known.",
		RelatedActions: []string{"group.epic_get", "group.epic_create", "group.epic_update"},
	},
	"tag.get": {
		Usage:          "Use to verify that a tag exists before release cleanup or tag deletion workflows.",
		RelatedActions: []string{actionReleaseGet, actionReleaseLinkList, actionReleaseDelete, "tag.delete"},
	},
	actionReleaseGet: {
		Usage:          "Use to verify a release for a tag after tag.get when the workflow asks to verify both.",
		RelatedActions: []string{"tag.get", actionReleaseLinkList, actionReleaseDelete},
	},
	actionReleaseLinkList: {
		Usage:          "Lists asset links for an existing release tag; it is not a release existence check.",
		RelatedActions: []string{actionReleaseGet, "release.link_create", actionReleaseLinkDelete},
	},
	actionReleaseLinkGet: {
		Usage:          "Gets one release asset link by link_id; use release.link_list to discover link IDs for a tag.",
		RelatedActions: []string{actionReleaseLinkList, actionReleaseLinkUpdate, actionReleaseLinkDelete},
	},
	actionReleaseLinkUpdate: {
		Usage:          "Updates one release asset link by link_id; use release.link_list or release.link_get before editing when the ID is unknown.",
		RelatedActions: []string{actionReleaseLinkGet, actionReleaseLinkList, actionReleaseLinkDelete},
	},
	actionReleaseLinkDelete: {
		Usage:          "Deletes one release asset link by link_id; use release.link_list before deletion when the ID is unknown.",
		RelatedActions: []string{actionReleaseLinkGet, actionReleaseLinkList},
	},
	"repository.compare": {
		Usage:          "Compares two refs using params.from and params.to; use before analyze.release_notes when the task asks to inspect the diff.",
		RelatedActions: []string{actionAnalyzeReleaseNotes, "release.list", "tag.list"},
	},
	actionAnalyzeReleaseNotes: {
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
	if entry.Usage != "" {
		return entry.Usage
	}
	return actionUXMetadataByID[entry.ID].Usage
}

func whyThisActionForEntry(entry actionEntry) string {
	if usage := usageHintForEntry(entry); usage != "" {
		return usage
	}
	return fmt.Sprintf("Matches canonical action %s with required params %s.", entry.ID, strings.Join(entry.RequiredParams, ", "))
}

func relatedActionsForEntry(entry actionEntry) []string {
	if len(entry.RelatedActions) > 0 {
		return append([]string(nil), entry.RelatedActions...)
	}
	return append([]string(nil), actionUXMetadataByID[entry.ID].RelatedActions...)
}

func cloneParameterGuidance(guidance map[string]toolutil.ParameterGuidance) map[string]toolutil.ParameterGuidance {
	if len(guidance) == 0 {
		return nil
	}
	out := make(map[string]toolutil.ParameterGuidance, len(guidance))
	for name, item := range guidance {
		item.CommonConfusions = append([]string(nil), item.CommonConfusions...)
		out[name] = item
	}
	return out
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
	return fmt.Sprintf("%s: unknown action %q. Did you mean %s? Use canonical action IDs with gitlab_execute_action.", toolName, action, strings.Join(suggestions, ", "))
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

func catalogActionAliases(catalog *actioncatalog.Catalog) []actionAlias {
	if catalog == nil {
		return nil
	}
	aliases := make([]actionAlias, 0)
	for _, group := range catalog.Groups() {
		for _, action := range group.ActionsInOrder() {
			for _, alias := range action.Compatibility.ActionAliases {
				aliases = append(aliases, actionAlias{
					Alias:      alias.Alias,
					Canonical:  string(action.ID),
					Source:     sourceForCompatibilityAlias(alias.Source, alias.Deprecated),
					Searchable: alias.Searchable,
					Notes:      alias.Reason,
				})
			}
		}
	}
	return aliases
}

func actionAliases() []actionAlias {
	return actionCompatAliases(actioncompat.ActionAliases())
}

func actionCompatAliases(aliases []actioncompat.ActionAlias) []actionAlias {
	out := make([]actionAlias, 0, len(aliases))
	for _, alias := range aliases {
		out = append(out, actionAlias{
			Alias:      alias.Alias,
			Canonical:  alias.Canonical,
			Source:     sourceForCompatibilityAlias(alias.Source, alias.Deprecated),
			Searchable: alias.Searchable,
			Notes:      alias.Reason,
		})
	}
	return out
}

func sourceForCompatibilityAlias(source string, deprecated bool) aliasSource {
	if deprecated {
		return aliasSourceDeprecated
	}
	source = strings.TrimSpace(source)
	if source == "" {
		return aliasSourceCompatibility
	}
	return aliasSource(source)
}

// NormalizeCompatibilityActionAlias canonicalizes an unambiguous built-in
// dynamic compatibility alias without requiring a registry instance.
func NormalizeCompatibilityActionAlias(actionID string) (string, bool) {
	return actioncompat.NormalizeActionAlias(actionID)
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
	minRequired := minimumMatchedTermCount(entry, terms)
	if matchedCount < minRequired {
		// Exception: let through entries with an explicit high-confidence intent
		// signal (search.code, user.current) even when the query contains many
		// extra tokens (project path, symbol name) that inflate minRequired. At
		// least two terms must still match so unrelated entries are never promoted.
		if matchedCount < 2 || !qualifiesForExplicitIntentBypass(entry, terms) {
			return 0
		}
	}
	score := totalScore * matchedCount / len(terms)
	score += scoreVerbIntentValue(entry, terms)
	score += scoreRequiredParamSignalValue(entry, terms)
	score += scoreCompoundTagSignalValue(entry, terms)
	score += scoreServiceAccountIntentValue(entry, terms)
	score += scoreScopeIntentValue(entry, terms)
	score += scoreCompareRefsIntentValue(entry, terms)
	score += scoreReleaseListIntentValue(entry, terms)
	score += scoreAnalyzeReleaseNotesIntentValue(entry, terms)
	score += scoreAnalyzeMRChangesIntentValue(entry, terms)
	score += scoreMRSecurityIntentValue(entry, terms)
	score += scoreDiscoverProjectIntentValue(entry, terms)
	score += scoreProjectGetIntentValue(entry, terms)
	score += scoreSearchProjectsIntentValue(entry, terms)
	score += scoreSearchCodeIntentValue(entry, terms)
	score += scoreCurrentUserIntentValue(entry, terms)
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
	minRequired := minimumMatchedTermCount(entry, terms)
	if matchedCount < minRequired {
		// Exception: let through entries with an explicit high-confidence intent
		// signal (search.code, user.current) even when the query contains many
		// extra tokens (project path, symbol name) that inflate minRequired. At
		// least two terms must still match so unrelated entries are never promoted.
		if matchedCount < 2 || !qualifiesForExplicitIntentBypass(entry, terms) {
			return 0, ScoringExplanation{}
		}
	}
	// Scale the total score by the match ratio so fully-matched entries rank
	// above partial matches.
	score := totalScore * matchedCount / len(terms)
	score, reasons = applyIntentAdjustments(entry, terms, score, reasons)
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

// applyIntentAdjustments accumulates all specialized intent and signal scores
// into the running total and match-reason list. Extracting this loop reduces
// the cyclomatic complexity of scoreEntryWithExplanation.
func applyIntentAdjustments(entry actionEntry, terms []searchTerm, score int, reasons []MatchReason) (int, []MatchReason) {
	type intentFn func(actionEntry, []searchTerm) (int, MatchReason)
	for _, fn := range []intentFn{
		scoreVerbIntent,
		scoreServiceAccountIntent,
		scoreScopeIntent,
		scoreCompareRefsIntent,
		scoreReleaseListIntent,
		scoreAnalyzeReleaseNotesIntent,
		scoreAnalyzeMRChangesIntent,
		scoreMRSecurityIntent,
		scoreDiscoverProjectIntent,
		scoreProjectGetIntent,
		scoreSearchProjectsIntent,
		scoreSearchCodeIntent,
		scoreCurrentUserIntent,
		scoreActionSpecificity,
	} {
		if adj, r := fn(entry, terms); adj != 0 {
			score += adj
			reasons = append(reasons, r)
		}
	}
	if adj, paramReasons := scoreRequiredParamSignals(entry, terms); adj != 0 {
		score += adj
		reasons = append(reasons, paramReasons...)
	}
	if adj, tagReasons := scoreCompoundTagSignals(entry, terms); adj != 0 {
		score += adj
		reasons = append(reasons, tagReasons...)
	}
	return score, reasons
}

func minimumMatchedTermCount(entry actionEntry, terms []searchTerm) int {
	minRequired := len(terms)
	if len(terms) > 2 {
		minRequired = len(terms) - 1
	}
	if len(terms) > 3 && matchedCompoundTagCount(entry, terms) > 0 && minRequired > len(terms)-2 {
		minRequired = len(terms) - 2
	}
	if minRequired < 1 {
		return 1
	}
	return minRequired
}

func matchedCompoundTagCount(entry actionEntry, terms []searchTerm) int {
	document := documentForEntry(entry)
	termSet := searchTermAlternativeSet(terms)
	matches := 0
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
		if matched {
			matches++
		}
	}
	return matches
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
		switch {
		case entry.Destructive:
			adjustment = scoreVerbIntentPenalty
		case isWriteAction(document.Action):
			adjustment = scoreVerbIntentPenalty / 2
		case isReadAction(document.Action):
			adjustment = scoreVerbIntentBoost
		}
	case verbIntentWrite:
		if isWriteAction(document.Action) {
			adjustment = scoreVerbIntentBoost
		}
	case verbIntentDestructive:
		adjustment = scoreDestructiveVerbAdjustment(entry, terms, document)
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

// scoreDestructiveVerbAdjustment returns the score adjustment when the user
// expresses a destructive verb intent. Extracted to keep scoreVerbIntentFor
// within cyclomatic complexity limits.
func scoreDestructiveVerbAdjustment(entry actionEntry, terms []searchTerm, document searchDocument) int {
	if !isDestructiveActionName(document.Action) && !entry.Destructive {
		return 0
	}
	if !queryHasResourceSignal(terms, document) {
		return scoreVerbIntentPenalty
	}
	if document.Action == "delete" || document.Action == "remove" || document.Action == "revoke" {
		return scoreVerbIntentBoost * 3
	}
	return scoreVerbIntentBoost
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
	total := 0
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
		boost := scoreCompoundTagBoost
		if document.Domain == "repository" && document.Action == "compare" && (containsWord(words, "ref") || containsWord(words, "refs")) && containsWord(words, "compare") {
			boost += scoreRequiredParamBoost
		}
		reasons = append(reasons, MatchReason{Field: searchFieldTag, QueryTerm: strings.Join(words, " "), MatchedValue: tag, Score: boost})
		total += boost
	}
	return total, reasons
}

func scoreCompoundTagSignalValue(entry actionEntry, terms []searchTerm) int {
	document := documentForEntry(entry)
	total := 0
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
			boost := scoreCompoundTagBoost
			if document.Domain == "repository" && document.Action == "compare" && (containsWord(words, "ref") || containsWord(words, "refs")) && containsWord(words, "compare") {
				boost += scoreRequiredParamBoost
			}
			total += boost
		}
	}
	return total
}

func containsWord(words []string, target string) bool {
	return slices.Contains(words, target)
}

func scoreServiceAccountIntent(entry actionEntry, terms []searchTerm) (int, MatchReason) {
	score := scoreServiceAccountIntentValue(entry, terms)
	if score == 0 {
		return 0, MatchReason{}
	}
	document := documentForEntry(entry)
	return score, MatchReason{Field: searchFieldServiceAccount, QueryTerm: "service account", MatchedValue: document.CanonicalID, Score: score}
}

func scoreServiceAccountIntentValue(entry actionEntry, terms []searchTerm) int {
	document := documentForEntry(entry)
	if !strings.Contains(document.CanonicalID, "service_account") || !queryHasSearchWords(terms, "service", "account") {
		return 0
	}
	score := scoreServiceAccountBoost
	if document.Domain != "" && searchTermsContainWord(terms, document.Domain) {
		score += scoreServiceAccountScope
	}
	if strings.Contains(document.CanonicalID, "service_account_pat") && (queryHasSearchWords(terms, "personal", "access", "token") || searchTermsContainWord(terms, "pat")) {
		score += scoreServiceAccountBoost
	}
	if verb := serviceAccountActionVerb(document.Action); verb != "" && searchTermsContainWord(terms, verb) {
		score += scoreServiceAccountBoost
	}
	return score
}

func queryHasSearchWords(terms []searchTerm, words ...string) bool {
	for _, word := range words {
		if !searchTermsContainWord(terms, word) {
			return false
		}
	}
	return true
}

func serviceAccountActionVerb(action string) string {
	switch {
	case strings.HasSuffix(action, "_list"):
		return "list"
	case strings.HasSuffix(action, "_create"):
		return "create"
	case strings.HasSuffix(action, "_update"):
		return "update"
	case strings.HasSuffix(action, "_delete"):
		return "delete"
	case strings.HasSuffix(action, "_rotate"):
		return "rotate"
	case strings.HasSuffix(action, "_revoke"):
		return "revoke"
	default:
		return ""
	}
}

func scoreScopeIntent(entry actionEntry, terms []searchTerm) (int, MatchReason) {
	score := scoreScopeIntentValue(entry, terms)
	if score == 0 {
		return 0, MatchReason{}
	}
	document := documentForEntry(entry)
	scope := matchingQueryScope(terms)
	return score, MatchReason{Field: searchFieldScopeIntent, QueryTerm: scope, MatchedValue: document.CanonicalID, Score: score}
}

func scoreScopeIntentValue(entry actionEntry, terms []searchTerm) int {
	scope := matchingQueryScope(terms)
	if scope == "" {
		return 0
	}
	document := documentForEntry(entry)
	if document.Domain == scope || document.Scope == scope {
		return scoreScopeIntentBoost
	}
	return 0
}

func scoreCompareRefsIntent(entry actionEntry, terms []searchTerm) (int, MatchReason) {
	score := scoreCompareRefsIntentValue(entry, terms)
	if score == 0 {
		return 0, MatchReason{}
	}
	document := documentForEntry(entry)
	return score, MatchReason{Field: searchFieldCompareIntent, QueryTerm: "compare refs", MatchedValue: document.CanonicalID, Score: score}
}

func scoreCompareRefsIntentValue(entry actionEntry, terms []searchTerm) int {
	document := documentForEntry(entry)
	if document.Domain != "repository" || document.Action != "compare" {
		return 0
	}
	if !searchTermsContainWord(terms, "compare") {
		return 0
	}
	if !searchTermsContainWord(terms, "ref") && !searchTermsContainWord(terms, "refs") {
		return 0
	}
	return scoreCompareRefsIntentBoost
}

func scoreReleaseListIntent(entry actionEntry, terms []searchTerm) (int, MatchReason) {
	score := scoreReleaseListIntentValue(entry, terms)
	if score == 0 {
		return 0, MatchReason{}
	}
	document := documentForEntry(entry)
	return score, MatchReason{Field: searchFieldReleaseIntent, QueryTerm: "list releases", MatchedValue: document.CanonicalID, Score: score}
}

func scoreReleaseListIntentValue(entry actionEntry, terms []searchTerm) int {
	document := documentForEntry(entry)
	if document.Domain != "release" || document.Action != "list" {
		return 0
	}
	if !searchTermsContainWord(terms, "list") {
		return 0
	}
	if !searchTermsContainWord(terms, "release") && !searchTermsContainWord(terms, "releases") {
		return 0
	}
	return scoreReleaseListIntentBoost
}

func scoreAnalyzeMRChangesIntent(entry actionEntry, terms []searchTerm) (int, MatchReason) {
	score := scoreAnalyzeMRChangesIntentValue(entry, terms)
	if score == 0 {
		return 0, MatchReason{}
	}
	document := documentForEntry(entry)
	return score, MatchReason{Field: searchFieldAnalyzeIntent, QueryTerm: "analyze mr changes", MatchedValue: document.CanonicalID, Score: score}
}

// scoreAnalyzeMRChangesIntentValue fires for analyze.mr_changes when the query
// combines an LLM/analyzer signal with an MR context. Without this boost,
// mr_review.changes_get outranks analyze.mr_changes because it accumulates more
// match-ratio points from its broader set of "changes/review" tags.
func scoreAnalyzeMRChangesIntentValue(entry actionEntry, terms []searchTerm) int {
	document := documentForEntry(entry)
	if document.Domain != "analyze" || document.Action != "mr_changes" {
		return 0
	}
	hasLLMSignal := searchTermsContainWord(terms, "llm") ||
		searchTermsContainWord(terms, "analyzer") ||
		searchTermsContainWord(terms, "sampling")
	if !hasLLMSignal {
		return 0
	}
	hasMR := searchTermsContainWord(terms, "mr") || searchTermsContainWord(terms, "merge")
	if !hasMR {
		return 0
	}
	return scoreAnalyzeMRChangesIntentBoost
}

func scoreAnalyzeReleaseNotesIntent(entry actionEntry, terms []searchTerm) (int, MatchReason) {
	score := scoreAnalyzeReleaseNotesIntentValue(entry, terms)
	if score == 0 {
		return 0, MatchReason{}
	}
	document := documentForEntry(entry)
	return score, MatchReason{Field: searchFieldAnalyzeIntent, QueryTerm: "release notes", MatchedValue: document.CanonicalID, Score: score}
}

func scoreAnalyzeReleaseNotesIntentValue(entry actionEntry, terms []searchTerm) int {
	document := documentForEntry(entry)
	if document.Domain != "analyze" || document.Action != "release_notes" {
		return 0
	}
	if !searchTermsContainWord(terms, "release") && !searchTermsContainWord(terms, "releases") {
		return 0
	}
	if !searchTermsContainWord(terms, "notes") {
		return 0
	}
	return scoreAnalyzeNotesIntentBoost
}

func scoreMRSecurityIntent(entry actionEntry, terms []searchTerm) (int, MatchReason) {
	score := scoreMRSecurityIntentValue(entry, terms)
	if score == 0 {
		return 0, MatchReason{}
	}
	document := documentForEntry(entry)
	return score, MatchReason{Field: searchFieldSecurityIntent, QueryTerm: "mr security review", MatchedValue: document.CanonicalID, Score: score}
}

func scoreMRSecurityIntentValue(entry actionEntry, terms []searchTerm) int {
	document := documentForEntry(entry)
	if document.Domain != "analyze" || document.Action != "mr_security" {
		return 0
	}
	if !searchTermsContainWord(terms, "security") && !searchTermsContainWord(terms, "secure") {
		return 0
	}
	if !searchTermsContainWord(terms, "review") && !searchTermsContainWord(terms, "analyzer") && !searchTermsContainWord(terms, "analyze") {
		return 0
	}
	if !searchTermsContainWord(terms, "merge") && !searchTermsContainWord(terms, "mr") && !searchTermsContainWord(terms, "merge_request") {
		return 0
	}
	return scoreMRSecurityIntentBoost
}

func scoreDiscoverProjectIntent(entry actionEntry, terms []searchTerm) (int, MatchReason) {
	score := scoreDiscoverProjectIntentValue(entry, terms)
	if score == 0 {
		return 0, MatchReason{}
	}
	document := documentForEntry(entry)
	return score, MatchReason{Field: searchFieldDiscoverIntent, QueryTerm: "discover project from remote", MatchedValue: document.CanonicalID, Score: score}
}

func scoreDiscoverProjectIntentValue(entry actionEntry, terms []searchTerm) int {
	document := documentForEntry(entry)
	if document.Domain != "discover_project" || document.Action != "resolve" {
		return 0
	}
	if !searchTermsContainWord(terms, "url") && !searchTermsContainWord(terms, "remote") && !searchTermsContainWord(terms, "origin") && !searchTermsContainWord(terms, "git") {
		return 0
	}
	if !searchTermsContainWord(terms, "project") && !searchTermsContainWord(terms, "path") && !searchTermsContainWord(terms, "resolve") && !searchTermsContainWord(terms, "discover") && !searchTermsContainWord(terms, "find") {
		return 0
	}
	return scoreDiscoverIntentBoost
}

func scoreProjectGetIntent(entry actionEntry, terms []searchTerm) (int, MatchReason) {
	score := scoreProjectGetIntentValue(entry, terms)
	if score == 0 {
		return 0, MatchReason{}
	}
	document := documentForEntry(entry)
	return score, MatchReason{Field: searchFieldProjectIntent, QueryTerm: "project get by path", MatchedValue: document.CanonicalID, Score: score}
}

func scoreProjectGetIntentValue(entry actionEntry, terms []searchTerm) int {
	document := documentForEntry(entry)
	if document.Domain != "project" || document.Action != "get" {
		return 0
	}
	if !searchTermsContainWord(terms, "project") {
		return 0
	}
	if !searchTermsContainWord(terms, "get") && !searchTermsContainWord(terms, "show") && !searchTermsContainWord(terms, "find") && !searchTermsContainWord(terms, "path") && !searchTermsContainWord(terms, "id") {
		return 0
	}
	return scoreProjectGetIntentBoost
}

func scoreSearchProjectsIntent(entry actionEntry, terms []searchTerm) (int, MatchReason) {
	score := scoreSearchProjectsIntentValue(entry, terms)
	if score == 0 {
		return 0, MatchReason{}
	}
	document := documentForEntry(entry)
	return score, MatchReason{Field: searchFieldSearchIntent, QueryTerm: "search projects", MatchedValue: document.CanonicalID, Score: score}
}

func scoreSearchProjectsIntentValue(entry actionEntry, terms []searchTerm) int {
	document := documentForEntry(entry)
	if document.Domain != "search" || document.Action != "projects" {
		return 0
	}
	if !searchTermsContainWord(terms, "search") {
		return 0
	}
	if !searchTermsContainWord(terms, "project") && !searchTermsContainWord(terms, "projects") {
		return 0
	}
	if searchProjectsQueryHasConcreteNeedle(terms) {
		return scoreSearchProjectsBoost + scoreProjectGetIntentBoost + scoreCompoundTagBoost
	}
	return scoreSearchProjectsBoost
}

func searchProjectsQueryHasConcreteNeedle(terms []searchTerm) bool {
	for _, term := range terms {
		switch term.Raw {
		case "project", "projects", "search", "list", "find", "all", "show", "get", "read":
			continue
		}
		return true
	}
	return false
}

func scoreSearchCodeIntent(entry actionEntry, terms []searchTerm) (int, MatchReason) {
	score := scoreSearchCodeIntentValue(entry, terms)
	if score == 0 {
		return 0, MatchReason{}
	}
	document := documentForEntry(entry)
	return score, MatchReason{Field: searchFieldSearchIntent, QueryTerm: "search code", MatchedValue: document.CanonicalID, Score: score}
}

// scoreSearchCodeIntentValue fires for search.code when the query pairs a search
// verb ("search"/"grep") with a code/blob/source noun. Without this, long queries
// that also name a project path inflate match-ratio for search.projects far above
// search.code, causing the wrong action to rank first.
func scoreSearchCodeIntentValue(entry actionEntry, terms []searchTerm) int {
	document := documentForEntry(entry)
	if document.Domain != "search" || document.Action != "code" {
		return 0
	}
	hasSearchVerb := searchTermsContainWord(terms, "search") || searchTermsContainWord(terms, "grep")
	if !hasSearchVerb {
		return 0
	}
	// grep is a strong enough indicator on its own; "search" requires a code noun
	// to distinguish from other search actions (projects, issues, wikis).
	if !searchTermsContainWord(terms, "grep") {
		if !searchTermsContainWord(terms, "code") &&
			!searchTermsContainWord(terms, "blob") &&
			!searchTermsContainWord(terms, "blobs") &&
			!searchTermsContainWord(terms, "source") {
			return 0
		}
	}
	return scoreSearchCodeIntentBoost
}

func scoreCurrentUserIntent(entry actionEntry, terms []searchTerm) (int, MatchReason) {
	score := scoreCurrentUserIntentValue(entry, terms)
	if score == 0 {
		return 0, MatchReason{}
	}
	document := documentForEntry(entry)
	return score, MatchReason{Field: searchFieldVerbIntent, QueryTerm: "current user", MatchedValue: document.CanonicalID, Score: score}
}

// scoreCurrentUserIntentValue fires for user.current when the query pairs a self
// signal ("current"/"authenticated"/"whoami") with an identity noun. The canonical
// alias "current user" is multi-word and never reaches exact-alias score on
// word-tokenized queries, so user.get and member-get actions outrank it without
// this scorer.
func scoreCurrentUserIntentValue(entry actionEntry, terms []searchTerm) int {
	document := documentForEntry(entry)
	if document.Domain != "user" || document.Action != "current" {
		return 0
	}
	hasSelf := searchTermsContainWord(terms, "current") ||
		searchTermsContainWord(terms, "authenticated") ||
		searchTermsContainWord(terms, "whoami")
	if !hasSelf {
		return 0
	}
	// Check identity nouns in Raw terms only, not in synonym expansions.
	// "current" expands to "current_user" which splits into ["current","user"],
	// so searchTermsContainWord would match "user" via the expansion of "current"
	// itself, making every "current X" query trigger this scorer regardless of
	// what X is. Restricting to raw terms avoids that false positive.
	hasIdentity := false
	for _, tm := range terms {
		switch tm.Raw {
		case "user", "profile", "account", "identity", "me", "myself", "whoami":
			hasIdentity = true
		}
	}
	if !hasIdentity {
		return 0
	}
	return scoreCurrentUserIntentBoost
}

// qualifiesForExplicitIntentBypass reports whether an entry qualifies for the
// match-ratio filter bypass in scoreEntry/scoreEntryWithExplanation. These
// actions match only 2–3 of ~10 tokens in long queries (the rest are project
// path, symbol name, etc.), so minimumMatchedTermCount filters them out before
// any intent boost can apply. The bypass lets them through when at least 2
// terms matched and the intent signal fires.
//
// repository.compare is intentionally NOT in this list. Compare queries always
// contain "compare" + at least one of "ref/from/to" as explicit terms, so they
// pass the match-ratio filter naturally without a bypass.
func qualifiesForExplicitIntentBypass(entry actionEntry, terms []searchTerm) bool {
	return scoreSearchCodeIntentValue(entry, terms) > 0 ||
		scoreCurrentUserIntentValue(entry, terms) > 0
}

func matchingQueryScope(terms []searchTerm) string {
	switch {
	case searchTermsContainWord(terms, "group"):
		return "group"
	case searchTermsContainWord(terms, "project"):
		return "project"
	default:
		return ""
	}
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

	if score, match, ok := scoreExactSearchAlternativeWithReason(document, alternative, reason); ok {
		return score, match
	}
	switch {
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

func scoreExactSearchAlternativeWithReason(document searchDocument, alternative string, reason func(string, string, int) (int, MatchReason)) (int, MatchReason, bool) {
	switch {
	case document.CanonicalID == alternative:
		score, match := reason(searchFieldCanonicalID, document.CanonicalID, scoreCanonicalExact)
		return score, match, true
	case stringInSlice(document.Aliases, alternative):
		score, match := reason(searchFieldAlias, alternative, scoreAliasExact)
		return score, match, true
	case stringInSlice(document.Tags, alternative):
		score, match := reason(searchFieldTag, alternative, scoreTagExact)
		return score, match, true
	case document.Action == alternative:
		score, match := reason(searchFieldAction, document.Action, scoreDomainActionExact)
		return score, match, true
	case document.Domain == alternative:
		score, match := reason(searchFieldDomain, document.Domain, scoreDomainActionExact)
		return score, match, true
	case slices.Contains(document.ActionWords, alternative):
		score, match := reason(searchFieldAction, alternative, scoreDomainActionWord)
		return score, match, true
	case slices.Contains(document.DomainWords, alternative):
		score, match := reason(searchFieldDomain, alternative, scoreDomainActionWord)
		return score, match, true
	default:
		return 0, MatchReason{}, false
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
		Tool:      executeActionToolName,
		Arguments: arguments,
	}
}

func searchNextStep(results []SearchResult) string {
	if len(results) == 0 {
		return ""
	}
	top := results[0]
	if top.LowConfidence || len(top.AmbiguousWith) > 0 {
		return fmt.Sprintf("Top result %s needs confirmation; choose the intended canonical action ID before executing.", backtickString(top.ID))
	}
	if len(top.RequiredParams) == 0 {
		return fmt.Sprintf("Top result %s has no required params. Execute with params:{} unless optional params are needed.", backtickString(top.ID))
	}
	b := strings.Builder{}
	fmt.Fprintf(&b, "Top result %s is high confidence. Use its exact parameter schema before executing", backtickString(top.ID))
	fmt.Fprintf(&b, "; search only proves required params %s.", compactParamList(top.RequiredParams, 8))
	if top.Destructive {
		b.WriteString(" Because this action is destructive, execute later with top-level confirm:true only after explicit user approval.")
	}
	return b.String()
}

func compactParamList(params []string, limit int) string {
	if len(params) == 0 {
		return "none"
	}
	if limit <= 0 || len(params) <= limit {
		return strings.Join(backtickStrings(params), ", ")
	}
	shown := strings.Join(backtickStrings(params[:limit]), ", ")
	return fmt.Sprintf("%s, and %d more", shown, len(params)-limit)
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
	fmt.Fprintf(&b, "%s\n\n", dynamicExecuteEnvelopeHint)
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
	if output.NextStep != "" {
		fmt.Fprintf(&b, "\nNext step: %s\n", output.NextStep)
	} else {
		b.WriteString("\nUse `gitlab_find_action` when the chosen action's full schema is still needed.\n")
	}
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
	b.WriteString("Immediate next step: choose one row and call `gitlab_execute_action` now; do not call `gitlab_find_action` again until that execute call returns.\n\n")
	fmt.Fprintf(&b, "%s\n\n", dynamicExecuteEnvelopeHint)
	withExplanations := hasFindExplanations(output.Results)
	withGuidance := hasFindGuidance(output.Results)
	switch {
	case withExplanations && withGuidance:
		b.WriteString("| Action ID | Score | Destructive | Required Params | Guidance | Why |\n")
		b.WriteString("| --- | ---: | --- | --- | --- | --- |\n")
	case withExplanations:
		b.WriteString("| Action ID | Score | Destructive | Required Params | Why |\n")
		b.WriteString("| --- | ---: | --- | --- | --- |\n")
	case withGuidance:
		b.WriteString("| Action ID | Score | Destructive | Required Params | Guidance |\n")
		b.WriteString("| --- | ---: | --- | --- | --- |\n")
	default:
		b.WriteString("| Action ID | Score | Destructive | Required Params |\n")
		b.WriteString("| --- | ---: | --- | --- |\n")
	}
	for _, result := range output.Results {
		required := "-"
		if len(result.RequiredParams) > 0 {
			required = strings.Join(result.RequiredParams, ", ")
		}
		switch {
		case withExplanations && withGuidance:
			fmt.Fprintf(&b, "| `%s` | %d | %t | %s | %s | %s |\n", result.ID, result.Score, result.Destructive, required, compactFindGuidance(result), explanationSummary(result.Explanation))
		case withExplanations:
			fmt.Fprintf(&b, "| `%s` | %d | %t | %s | %s |\n", result.ID, result.Score, result.Destructive, required, explanationSummary(result.Explanation))
		case withGuidance:
			fmt.Fprintf(&b, "| `%s` | %d | %t | %s | %s |\n", result.ID, result.Score, result.Destructive, required, compactFindGuidance(result))
		default:
			fmt.Fprintf(&b, "| `%s` | %d | %t | %s |\n", result.ID, result.Score, result.Destructive, required)
		}
	}
	b.WriteString("\nNext step: choose one row and call `gitlab_execute_action` with that row's schema/example before starting another catalog operation.\n")
	b.WriteString("Structured results include exact `input_schema` values and `gitlab_execute_action` examples for each action.\n")
	return b.String()
}

func hasFindGuidance(results []FindResult) bool {
	return slices.ContainsFunc(results, func(result FindResult) bool {
		return result.Destructive || strings.TrimSpace(result.Usage) != "" || len(result.ParamGuidance) > 0
	})
}

func compactFindGuidance(result FindResult) string {
	parts := make([]string, 0, 3)
	if usage := strings.TrimSpace(result.Usage); usage != "" {
		parts = append(parts, usage)
	}
	if guidance := compactParameterGuidance(result.ParamGuidance, defaultMaxParamGuidanceItems, result.RequiredParams...); guidance != "" {
		parts = append(parts, guidance)
	}
	if result.Destructive {
		parts = append(parts, "Execute destructive actions with top-level `confirm:true`.")
	}
	if len(parts) == 0 {
		return "-"
	}
	return toolutil.EscapeMdTableCell(strings.Join(parts, " "))
}

func compactParameterGuidance(guidance map[string]toolutil.ParameterGuidance, limit int, requiredParams ...string) string {
	if len(guidance) == 0 || limit == 0 {
		return ""
	}
	required := make(map[string]struct{}, len(requiredParams))
	for _, name := range requiredParams {
		required[name] = struct{}{}
	}
	names := make([]string, 0, len(guidance))
	for name := range guidance {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		_, leftRequired := required[names[i]]
		_, rightRequired := required[names[j]]
		if leftRequired != rightRequired {
			return leftRequired
		}
		left := guidance[names[i]]
		right := guidance[names[j]]
		if len(left.CommonConfusions) != len(right.CommonConfusions) {
			return len(left.CommonConfusions) > len(right.CommonConfusions)
		}
		return names[i] < names[j]
	})
	truncated := 0
	if limit > 0 && len(names) > limit {
		truncated = len(names) - limit
		names = names[:limit]
	}
	parts := make([]string, 0, len(names))
	for _, name := range names {
		item := guidance[name]
		parts = append(parts, compactParameterGuidanceItem(name, item))
	}
	if truncated > 0 {
		parts = append(parts, fmt.Sprintf("...and %d more params.", truncated))
	}
	return strings.Join(parts, " ")
}

func compactParameterGuidanceItem(name string, item toolutil.ParameterGuidance) string {
	if item.ExampleBinding != "" {
		return fmt.Sprintf("`%s` example %s.", name, item.ExampleBinding)
	}
	if item.ValueSource != "" {
		return fmt.Sprintf("`%s`: %s.", name, item.ValueSource)
	}
	if item.SemanticRole != "" {
		return fmt.Sprintf("`%s`: %s.", name, item.SemanticRole)
	}
	if len(item.CommonConfusions) > 0 {
		return fmt.Sprintf("`%s`: %s", name, item.CommonConfusions[0])
	}
	return fmt.Sprintf("`%s` has action-specific guidance.", name)
}
