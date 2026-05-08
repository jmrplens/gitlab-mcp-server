// Package dynamic registers the low-token dynamic toolset for GitLab actions.
package dynamic

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actionregistry"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const (
	searchToolName   = "gitlab_search_tools"
	describeToolName = "gitlab_describe_tools"
	findToolName     = "gitlab_find_action"
	executeToolName  = "gitlab_execute_tool"
	defaultLimit     = 20
	maxLimit         = 50
	minSegmentTerms  = 3
	maxSegmentTerms  = 6
)

// SearchInput is the input for gitlab_search_tools.
type SearchInput struct {
	Query string `json:"query" jsonschema:"Search terms for GitLab actions, such as project create, merge request approve, pipeline retry, or ci variable."`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum number of matches to return. Defaults to 20 and is capped at 50."`
}

// SearchResult is one matching GitLab catalog action.
type SearchResult struct {
	ID             string   `json:"id" jsonschema:"Canonical action ID to pass to gitlab_describe_tools or gitlab_execute_tool."`
	Tool           string   `json:"tool" jsonschema:"Backing meta-tool name."`
	Domain         string   `json:"domain" jsonschema:"Canonical action domain."`
	Action         string   `json:"action" jsonschema:"Action name inside the catalog group."`
	SchemaURI      string   `json:"schema_uri" jsonschema:"MCP resource URI for the action parameter schema."`
	Destructive    bool     `json:"destructive" jsonschema:"Whether this action is marked destructive and requires explicit confirmation."`
	RequiredParams []string `json:"required_params,omitempty" jsonschema:"Required parameter names captured from the action input schema."`
	Score          int      `json:"score" jsonschema:"Lexical relevance score for the query."`
}

// SearchOutput is the structured output for gitlab_search_tools.
type SearchOutput struct {
	Query   string         `json:"query" jsonschema:"Original search query."`
	Count   int            `json:"count" jsonschema:"Number of returned matches."`
	Results []SearchResult `json:"results" jsonschema:"Matching GitLab catalog actions."`
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
	Query string `json:"query" jsonschema:"Search terms for GitLab actions, such as project create, merge request approve, pipeline retry, or ci variable."`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum number of matches to return. Defaults to 20 and is capped at 50."`
}

// FindResult is a matching catalog action with schema details and an execute example.
type FindResult struct {
	ID             string         `json:"id" jsonschema:"Canonical action ID to pass to gitlab_execute_tool."`
	Tool           string         `json:"tool" jsonschema:"Backing meta-tool name."`
	Domain         string         `json:"domain" jsonschema:"Canonical action domain."`
	Action         string         `json:"action" jsonschema:"Action name inside the catalog group."`
	SchemaURI      string         `json:"schema_uri" jsonschema:"MCP resource URI for the action parameter schema."`
	Destructive    bool           `json:"destructive" jsonschema:"Whether this action requires explicit confirmation."`
	RequiredParams []string       `json:"required_params,omitempty" jsonschema:"Required parameter names captured from the input schema."`
	Score          int            `json:"score" jsonschema:"Lexical relevance score for the query."`
	InputSchema    map[string]any `json:"input_schema" jsonschema:"Exact JSON Schema for action-specific params."`
	OutputSchema   map[string]any `json:"output_schema,omitempty" jsonschema:"Best-effort JSON Schema for the action result."`
	Example        ActionExample  `json:"example" jsonschema:"Example gitlab_execute_tool call."`
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
	Params  map[string]any `json:"params,omitempty" jsonschema:"Action-specific parameters validated by the selected action schema."`
	Confirm bool           `json:"confirm,omitempty" jsonschema:"Set true to explicitly confirm destructive actions."`
}

type scoredActionEntry struct {
	entry actionEntry
	score int
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
}

// RegisterCatalogTools registers the dynamic search, describe, and execute
// tools from the canonical action catalog.
func RegisterCatalogTools(server *mcp.Server, catalog *actionregistry.Catalog) {
	registry := NewRegistryFromCatalog(catalog)
	addSearchTool(server, registry)
	addDescribeTool(server, registry)
	addExecuteTool(server, registry)
}

// RegisterCatalogFindExecuteTools registers the experimental two-tool dynamic
// catalog from the canonical action catalog.
func RegisterCatalogFindExecuteTools(server *mcp.Server, catalog *actionregistry.Catalog) {
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
	mcp.AddTool(server, &mcp.Tool{
		Name:        executeToolName,
		Title:       "GitLab Execute Tool",
		Description: "Execute one GitLab catalog action by canonical action ID (e.g. domain.action). For the 3-tool catalog, use gitlab_search_tools and gitlab_describe_tools first unless the exact action ID and all required param names are already known. For the 2-tool catalog, use gitlab_find_action first. Do NOT guess or invent action IDs. Include ONLY the exact param names from the action schema; do NOT invent extra params. Destructive actions require confirm=true.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "GitLab Execute Tool",
			DestructiveHint: toolutil.BoolPtr(true),
			OpenWorldHint:   toolutil.BoolPtr(true),
		},
		Icons: toolutil.IconServer,
	}, registry.Execute)
}

// NewRegistry builds a deterministic action registry from visible meta routes.
func NewRegistry(routes map[string]toolutil.ActionMap) *Registry {
	return NewRegistryFromCatalog(actionregistry.FromActionMaps(routes))
}

func newRegistry(routes map[string]toolutil.ActionMap, aliases []actionAlias) *Registry {
	return newRegistryFromCatalog(actionregistry.FromActionMaps(routes), aliases)
}

// NewRegistryFromCatalog builds a deterministic dynamic action index from the
// canonical action catalog.
func NewRegistryFromCatalog(catalog *actionregistry.Catalog) *Registry {
	return newRegistryFromCatalog(catalog, actionAliases())
}

func newRegistryFromCatalog(catalog *actionregistry.Catalog, aliases []actionAlias) *Registry {
	if catalog == nil {
		catalog = actionregistry.NewCatalog()
	}
	registry := &Registry{
		byID:             make(map[string]actionEntry),
		aliases:          make(map[string]string),
		ambiguousAliases: make(map[string][]string),
		handlers:         make(map[string]toolHandler),
	}
	aliasTargets := make(map[string][]string)

	for _, group := range catalog.Groups() {
		actions := group.ActionMap()
		registry.handlers[group.ToolName] = toolutil.MakeMetaHandler(group.ToolName, actions, toolutil.MarkdownForResult)

		for _, action := range group.ActionsInOrder() {
			route := action.Route
			domain := action.Domain
			id := string(action.ID)
			entryAliases := dedupeStrings(append(action.Aliases, aliasesForCanonicalAction(id, aliases)...))
			tags := dedupeStrings(append(action.Tags, actionTags(id, domain, action.Name, route.InputSchema)...))
			schemaURI := action.SchemaURI
			searchText := buildSearchText(id, group.ToolName, domain, action.Name, entryAliases, tags, route.InputSchema)
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
				SearchText:     searchText,
				SearchTokens:   buildSearchTokens(searchText),
				Route:          route,
			}
			registry.entries = append(registry.entries, entry)
			registry.byID[id] = entry
			for _, alias := range entryAliases {
				aliasTargets[alias] = append(aliasTargets[alias], id)
			}
		}
	}
	registry.indexAliases(aliasTargets)

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

	matches := r.searchMatches(query, input.Limit)

	results := make([]SearchResult, 0, len(matches))
	for _, match := range matches {
		entry := match.entry
		results = append(results, SearchResult{
			ID:             entry.ID,
			Tool:           entry.Tool,
			Domain:         entry.Domain,
			Action:         entry.Action,
			SchemaURI:      entry.SchemaURI,
			Destructive:    entry.Destructive,
			RequiredParams: append([]string(nil), entry.RequiredParams...),
			Score:          match.score,
		})
	}

	output := SearchOutput{Query: query, Count: len(results), Results: results}
	return toolutil.ToolResultAnnotated(formatSearchOutput(output), toolutil.ContentList), output, nil
}

// Describe returns schemas and execution metadata for GitLab catalog actions.
func (r *Registry) Describe(_ context.Context, _ *mcp.CallToolRequest, input DescribeInput) (*mcp.CallToolResult, DescribeOutput, error) {
	ids := normalizeDescribeIDs(input)
	if len(ids) == 0 {
		return toolutil.ErrorResult("gitlab_describe_tools: provide action or actions with canonical IDs from gitlab_search_tools or gitlab_find_action."), DescribeOutput{}, nil
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

	matches := r.searchMatches(query, input.Limit)
	results := make([]FindResult, 0, len(matches))
	for _, match := range matches {
		description := describeEntry(match.entry)
		results = append(results, FindResult{
			ID:             description.ID,
			Tool:           description.Tool,
			Domain:         description.Domain,
			Action:         description.Action,
			SchemaURI:      description.SchemaURI,
			Destructive:    description.Destructive,
			RequiredParams: append([]string(nil), description.RequiredParams...),
			Score:          match.score,
			InputSchema:    description.InputSchema,
			OutputSchema:   description.OutputSchema,
			Example:        description.Example,
		})
	}

	output := FindOutput{Query: query, Count: len(results), Results: results}
	return toolutil.ToolResultAnnotated(formatFindOutput(output), toolutil.ContentDetail), output, nil
}

// Execute dispatches one catalog action through the existing meta-tool handler.
func (r *Registry) Execute(ctx context.Context, req *mcp.CallToolRequest, input ExecuteInput) (*mcp.CallToolResult, any, error) {
	id := strings.ToLower(strings.TrimSpace(input.Action))
	if id == "" {
		return toolutil.ErrorResult("gitlab_execute_tool: action is required. Use gitlab_search_tools or gitlab_find_action to find a canonical action ID."), nil, nil
	}
	entry, ok := r.resolveAction(id)
	if !ok {
		return toolutil.ErrorResult(r.unknownActionMessage("gitlab_execute_tool", input.Action)), nil, nil
	}

	params := maps.Clone(input.Params)
	if params == nil {
		params = map[string]any{}
	}
	params = toolutil.NormalizeParamAliasesForSchema(params, entry.Route.InputSchema)
	if input.Confirm {
		params["confirm"] = true
	}
	if entry.Destructive && !hasExplicitConfirm(params) {
		return toolutil.ErrorResult(fmt.Sprintf("gitlab_execute_tool: action %q is destructive. Re-send with confirm=true only after the user explicitly approves this operation.", entry.ID)), nil, nil
	}

	handler := r.handlers[entry.Tool]
	return handler(ctx, req, toolutil.MetaToolInput{Action: entry.Action, Params: params})
}

func annotationsWithTitle(base *mcp.ToolAnnotations, title string) *mcp.ToolAnnotations {
	if base == nil {
		return &mcp.ToolAnnotations{Title: title}
	}
	annotation := *base
	annotation.Title = title
	return &annotation
}

func buildSearchText(id, tool, domain, action string, aliases, tags []string, schema map[string]any) string {
	parts := []string{id, strings.ReplaceAll(id, ".", " "), tool, domain, strings.ReplaceAll(domain, "_", " "), action, strings.ReplaceAll(action, "_", " ")}
	for _, alias := range aliases {
		parts = append(parts, alias, strings.ReplaceAll(alias, ".", " "), strings.ReplaceAll(alias, "_", " "))
	}
	parts = append(parts, tags...)
	parts = append(parts, requiredParams(schema)...)
	if properties, ok := schema["properties"].(map[string]any); ok {
		for name := range properties {
			parts = append(parts, name, strings.ReplaceAll(name, "_", " "))
		}
	}
	return strings.ToLower(strings.Join(parts, " "))
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

func searchStopWords() map[string]struct{} {
	return map[string]struct{}{
		"a": {}, "an": {}, "and": {}, "as": {}, "at": {}, "by": {}, "for": {}, "from": {}, "in": {},
		"of": {}, "on": {}, "or": {}, "please": {}, "the": {}, "to": {}, "using": {}, "via": {}, "with": {},
	}
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

func searchSynonyms() map[string][]string {
	return map[string][]string{
		"access":     {"token", "deploy", "member"},
		"approve":    {"approval", "review", "feedback"},
		"approved":   {"approval", "review", "approved"},
		"artifact":   {"job", "download"},
		"assigned":   {"assignee", "assignee_username", "assignee_id", "list"},
		"assignee":   {"assigned", "assign", "delegate", "list"},
		"author":     {"creator", "created_by", "owner", "list"},
		"authored":   {"author", "author_username", "creator", "created_by", "owner", "list"},
		"ci":         {"pipeline", "job", "variable", "lint"},
		"closed":     {"close", "list", "filter"},
		"comment":    {"note", "discussion", "reply"},
		"current":    {"current_user", "self", "me", "author", "author_username", "assignee", "assignee_username"},
		"deploy":     {"deployment", "environment", "key"},
		"deployment": {"deploy", "environment"},
		"details":    {"get"},
		"discussion": {"comment", "thread", "note"},
		"draft":      {"wip", "work_in_progress", "proposal"},
		"env":        {"environment"},
		"file":       {"repository", "blob", "content"},
		"filter":     {"search", "query", "find", "list"},
		"info":       {"get"},
		"label":      {"tag", "category", "list"},
		"merged":     {"merge", "integrated", "list"},
		"metadata":   {"get", "details"},
		"me":         {"author", "author_username", "assignee", "assignee_username", "current_user", "self", "list"},
		"milestone":  {"sprint", "release", "deadline", "list"},
		"mine":       {"my", "owned", "owner", "author", "list"},
		"mr":         {"merge", "request", "merge_request"},
		"my":         {"owned", "owner", "author", "assignee", "list"},
		"note":       {"comment", "discussion", "reply"},
		"open":       {"active", "unresolved", "status_open", "list"},
		"owned":      {"my", "personal", "mine", "owner", "list"},
		"pending":    {"list", "filter", "todo"},
		"read":       {"get", "file", "content"},
		"review":     {"approval", "feedback", "assessment"},
		"repo":       {"repository", "file", "tree", "branch", "tag"},
		"secret":     {"variable", "ci_variable", "token", "password"},
		"show":       {"get"},
		"state":      {"status", "condition", "filter", "list"},
		"unresolved": {"open", "active", "list"},
		"user":       {"username", "user_id", "author_username", "assignee_username", "current_user", "member"},
		"users":      {"username", "user_id", "author_username", "assignee_username", "current_user", "member"},
		"verify":     {"get"},
		"webhook":    {"hook"},
		"webhooks":   {"hook"},
		"yaml":       {"ci", "lint", "template"},
		"yml":        {"ci", "lint", "template"},
	}
}

func verbSynonyms() map[string][]string {
	return map[string][]string{
		"add":       {"create", "enable", "register"},
		"cancel":    {"stop"},
		"close":     {"update", "state_event", "closed"},
		"disable":   {"delete", "remove", "stop"},
		"download":  {"artifact", "trace", "raw", "content"},
		"destroy":   {"delete", "remove"},
		"enable":    {"add", "create", "register"},
		"lock":      {"protect"},
		"remove":    {"delete"},
		"rerun":     {"retry"},
		"revoke":    {"delete", "remove"},
		"run":       {"play", "create", "trigger"},
		"unlock":    {"unprotect"},
		"unapprove": {"reset", "approval"},
	}
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
		add("webhook", "web hook", "project webhook")
	case strings.Contains(id, "deploy_key"):
		add("deploy key", "ssh key", "access key")
	case domain == "discover_project":
		add("discover", "project", "remote", "url", "lookup", "resolve", "project discovery", "git remote", "remote url", "resolve project")
	case domain == "interactive":
		add("guided", "elicitation", "wizard", strings.ReplaceAll(action, "_", " "))
	case strings.Contains(id, "token_project") || strings.Contains(id, "token_group") || strings.Contains(id, "token_personal"):
		add("access token", "project access token", "personal access token")
	case domain == "repository" && strings.HasPrefix(action, "file_"):
		add("repository file", "repo file", "file content")
	case domain == "merge_request":
		add("mr", "merge request")
	case domain == "ci_variable":
		add("ci variable", "secret", "environment variable")
	case domain == "environment":
		add("env", "deployment")
	case domain == "job":
		add("ci job", "pipeline job")
	case domain == "pipeline":
		add("ci pipeline")
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
				add("webhook", "link")
			}
		}
	}

	return dedupeStrings(tags)
}

func (r *Registry) searchMatches(query string, limit int) []scoredActionEntry {
	limit = normalizedLimit(limit)
	terms := normalizeSearchTerms(query)
	matches := r.scoredMatches(terms, scoreEntry)
	if len(matches) == 0 {
		matches = r.scoredMatches(terms, fuzzyScoreEntry)
	}
	if segmented := r.segmentedSearchMatches(terms); len(segmented) > 0 {
		matches = mergeBestMatches(matches, segmented)
	}
	return sortAndLimitMatches(matches, limit)
}

type searchScorer func(actionEntry, []searchTerm) int

func (r *Registry) scoredMatches(terms []searchTerm, scorer searchScorer) []scoredActionEntry {
	matches := make([]scoredActionEntry, 0)
	for _, entry := range r.entries {
		score := scorer(entry, terms)
		if score > 0 {
			matches = append(matches, scoredActionEntry{entry: entry, score: score})
		}
	}
	return matches
}

func (r *Registry) segmentedSearchMatches(terms []searchTerm) []scoredActionEntry {
	if len(terms) < minSegmentTerms {
		return nil
	}

	bestByID := make(map[string]scoredActionEntry)
	maxWindow := min(maxSegmentTerms, len(terms))
	for windowSize := maxWindow; windowSize >= minSegmentTerms; windowSize-- {
		for start := 0; start+windowSize <= len(terms); start++ {
			window := terms[start : start+windowSize]
			for _, match := range r.scoredMatches(window, scoreEntry) {
				match.score += windowSize * 60
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
		InputSchema:    inputSchema,
		Example:        exampleFor(entry, inputSchema),
	}
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
		return fmt.Sprintf("%s: unknown action %q. Use gitlab_search_tools or gitlab_find_action to find canonical action IDs.", toolName, action)
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

func aliasesForCanonicalAction(id string, allAliases []actionAlias) []string {
	var aliases []string
	for _, actionAlias := range allAliases {
		if actionAlias.Canonical == id {
			aliases = append(aliases, actionAlias.Alias)
		}
	}
	return dedupeSortedStrings(aliases)
}

type actionAlias struct {
	Alias     string
	Canonical string
}

func actionAliases() []actionAlias {
	return []actionAlias{
		{Alias: "badge.create", Canonical: "project.badge_add"},
		{Alias: "badge.delete", Canonical: "project.badge_delete"},
		{Alias: "broadcast_message.create", Canonical: "admin.broadcast_message_create"},
		{Alias: "broadcast_message.delete", Canonical: "admin.broadcast_message_delete"},
		{Alias: "ci_catalog.resource_list", Canonical: "ci_catalog.list"},
		{Alias: "ci_job_token_scope.inbound_allowlist.list", Canonical: "job.token_scope_list_inbound"},
		{Alias: "deploy_key.create", Canonical: "access.deploy_key_add"},
		{Alias: "deploy_key.list", Canonical: "access.deploy_key_list_project"},
		{Alias: "deploy_token.create", Canonical: "access.deploy_token_create_project"},
		{Alias: "deploy_token.delete", Canonical: "access.deploy_token_delete_project"},
		{Alias: "deploy_token.get", Canonical: "access.deploy_token_get_project"},
		{Alias: "deploy_token.list", Canonical: "access.deploy_token_list_project"},
		{Alias: "enterprise_user.group_list", Canonical: "enterprise_user.list"},
		{Alias: "external_status_check.list_project_checks", Canonical: "external_status_check.list_project"},
		{Alias: "feature_flag.list", Canonical: "feature_flags.feature_flag_list"},
		{Alias: "geo.node_list", Canonical: "geo.list"},
		{Alias: "gitlab_server.health_check", Canonical: "server.health_check"},
		{Alias: "group.custom_member_roles_list", Canonical: "member_role.list_group"},
		{Alias: "group.ldap_link_delete", Canonical: "group.ldap_link_delete_for_provider"},
		{Alias: "issue.notes", Canonical: "issue.note_list"},
		{Alias: "issue.notes.list", Canonical: "issue.note_list"},
		{Alias: "job.download_single_artifact", Canonical: "job.artifact_download"},
		{Alias: "pipeline.jobs", Canonical: "job.list"},
		{Alias: "merge_train.list", Canonical: "merge_train.list_project"},
		{Alias: "merge_request.accept", Canonical: "merge_request.merge"},
		{Alias: "merge_request.changes", Canonical: "mr_review.changes_get"},
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
		{Alias: "repository_file.create", Canonical: "repository.file_create"},
		{Alias: "repository_file.delete", Canonical: "repository.file_delete"},
		{Alias: "repository_file.get", Canonical: "repository.file_get"},
		{Alias: "release.create_link", Canonical: "release.link_create"},
		{Alias: "release.asset_link.create", Canonical: "release.link_create"},
		{Alias: "release.generate_notes", Canonical: "analyze.release_notes"},
		{Alias: "package.list_project", Canonical: "package.list"},
		{Alias: "variable.create", Canonical: "ci_variable.create"},
		{Alias: "group.variable.create", Canonical: "ci_variable.group_create"},
		{Alias: "group.audit_events", Canonical: "audit_event.list_group"},
		{Alias: "gitlab_discover_project", Canonical: "discover_project.resolve"},
		{Alias: "interactive_issue.create", Canonical: "interactive.issue_create"},
		{Alias: "interactive_issue_create", Canonical: "interactive.issue_create"},
		{Alias: "gitlab_interactive_issue_create", Canonical: "interactive.issue_create"},
		{Alias: "gitlab_interactive_mr_create", Canonical: "interactive.mr_create"},
		{Alias: "gitlab_interactive_project_create", Canonical: "interactive.project_create"},
		{Alias: "gitlab_interactive_release_create", Canonical: "interactive.release_create"},
		{Alias: "runner.delete", Canonical: "runner.remove"},
		{Alias: "webhook.add", Canonical: "project.hook_add"},
		{Alias: "webhook.create", Canonical: "project.hook_add"},
		{Alias: "webhook.delete", Canonical: "project.hook_delete"},
	}
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
	// For short queries (1–2 terms) all terms must match.
	// For longer queries allow at most one unmatched term so that incidental
	// words like state values ("open") or prepositions don't suppress results.
	minRequired := len(terms)
	if len(terms) > 2 {
		minRequired = len(terms) - 1
	}
	if matchedCount < minRequired {
		return 0
	}
	// Scale the total score by the match ratio so fully-matched entries rank
	// above partial matches.
	return totalScore * matchedCount / len(terms)
}

func scoreSearchAlternative(entry actionEntry, raw, alternative string) int {
	switch {
	case entry.ID == alternative:
		return 120
	case stringInSlice(entry.Aliases, alternative):
		return 100
	case entry.Action == alternative || entry.Domain == alternative:
		return 80
	case stringInSlice(entry.Tags, alternative):
		return 90
	case strings.Contains(entry.ID, alternative):
		return 55
	case strings.Contains(entry.Domain, alternative) || strings.Contains(entry.Action, alternative):
		return 45
	case strings.Contains(entry.SearchText, alternative):
		if raw == alternative {
			return 25
		}
		return 18
	default:
		return 0
	}
}

func stringInSlice(values []string, needle string) bool {
	return slices.Contains(values, needle)
}

func requiredParams(schema map[string]any) []string {
	if schema == nil {
		return nil
	}
	raw, ok := schema["required"]
	if !ok {
		return nil
	}
	var names []string
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
	sort.Strings(names)
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
	if strings.HasSuffix(name, "_id") || name == "id" || strings.HasSuffix(name, "iid") {
		return 123
	}
	if strings.Contains(name, "date") {
		return "2026-05-07"
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
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "y":
			return true
		}
	}
	return false
}

func cloneSchema(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}
	clone := make(map[string]any, len(schema))
	for key, value := range schema {
		clone[key] = cloneSchemaValue(value)
	}
	return clone
}

func cloneSchemaValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneSchema(typed)
	case []any:
		clone := make([]any, len(typed))
		for i, item := range typed {
			clone[i] = cloneSchemaValue(item)
		}
		return clone
	case []string:
		return append([]string(nil), typed...)
	default:
		return typed
	}
}

func formatSearchOutput(output SearchOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## GitLab Action Search\n\n")
	if output.Count == 0 {
		fmt.Fprintf(&b, "No catalog actions matched %q. Try broader terms such as project, issue, merge request, pipeline, branch, or user.\n", output.Query)
		return b.String()
	}
	fmt.Fprintf(&b, "Query: `%s`\n\n", output.Query)
	b.WriteString("| Action ID | Destructive | Required Params |\n")
	b.WriteString("| --- | --- | --- |\n")
	for _, result := range output.Results {
		required := "-"
		if len(result.RequiredParams) > 0 {
			required = strings.Join(result.RequiredParams, ", ")
		}
		fmt.Fprintf(&b, "| `%s` | %t | %s |\n", result.ID, result.Destructive, required)
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
		fmt.Fprintf(&b, "- **Schema URI**: `%s`\n\n", action.SchemaURI)
	}
	return b.String()
}

func formatFindOutput(output FindOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## GitLab Action Finder\n\n")
	if output.Count == 0 {
		fmt.Fprintf(&b, "No catalog actions matched %q. Try broader terms such as project, issue, merge request, pipeline, branch, or user.\n", output.Query)
		return b.String()
	}
	fmt.Fprintf(&b, "Query: `%s`\n\n", output.Query)
	b.WriteString("| Action ID | Score | Destructive | Required Params |\n")
	b.WriteString("| --- | ---: | --- | --- |\n")
	for _, result := range output.Results {
		required := "-"
		if len(result.RequiredParams) > 0 {
			required = strings.Join(result.RequiredParams, ", ")
		}
		fmt.Fprintf(&b, "| `%s` | %d | %t | %s |\n", result.ID, result.Score, result.Destructive, required)
	}
	b.WriteString("\nStructured results include exact `input_schema` values and `gitlab_execute_tool` examples for each action.\n")
	return b.String()
}
