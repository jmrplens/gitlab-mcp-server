// Package dynamic registers the low-token dynamic toolset for GitLab actions.
package dynamic

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const (
	searchToolName   = "gitlab_search_tools"
	describeToolName = "gitlab_describe_tools"
	executeToolName  = "gitlab_execute_tool"
	defaultLimit     = 20
	maxLimit         = 50
)

// SearchInput is the input for gitlab_search_tools.
type SearchInput struct {
	Query string `json:"query" jsonschema:"Search terms for GitLab actions, such as project create, merge request approve, pipeline retry, or ci variable."`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum number of matches to return. Defaults to 20 and is capped at 50."`
}

// SearchResult is one matching hidden GitLab action.
type SearchResult struct {
	ID             string   `json:"id" jsonschema:"Canonical action ID to pass to gitlab_describe_tools or gitlab_execute_tool."`
	Tool           string   `json:"tool" jsonschema:"Underlying hidden meta-tool name."`
	Domain         string   `json:"domain" jsonschema:"Canonical action domain."`
	Action         string   `json:"action" jsonschema:"Underlying action name inside the hidden meta-tool."`
	SchemaURI      string   `json:"schema_uri" jsonschema:"MCP resource URI for the action parameter schema."`
	Destructive    bool     `json:"destructive" jsonschema:"Whether this action is marked destructive and requires explicit confirmation."`
	RequiredParams []string `json:"required_params,omitempty" jsonschema:"Required parameter names captured from the action input schema."`
	Score          int      `json:"score" jsonschema:"Lexical relevance score for the query."`
}

// SearchOutput is the structured output for gitlab_search_tools.
type SearchOutput struct {
	Query   string         `json:"query" jsonschema:"Original search query."`
	Count   int            `json:"count" jsonschema:"Number of returned matches."`
	Results []SearchResult `json:"results" jsonschema:"Matching hidden GitLab actions."`
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

// ActionDescription describes one hidden GitLab action.
type ActionDescription struct {
	ID             string         `json:"id" jsonschema:"Canonical action ID."`
	Tool           string         `json:"tool" jsonschema:"Underlying hidden meta-tool name."`
	Domain         string         `json:"domain" jsonschema:"Canonical action domain."`
	Action         string         `json:"action" jsonschema:"Underlying action name inside the hidden meta-tool."`
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

// ExecuteInput is the input for gitlab_execute_tool.
type ExecuteInput struct {
	Action  string         `json:"action" jsonschema:"Canonical action ID returned by gitlab_search_tools or gitlab_describe_tools, such as project.list."`
	Params  map[string]any `json:"params,omitempty" jsonschema:"Action-specific parameters validated by the selected action schema."`
	Confirm bool           `json:"confirm,omitempty" jsonschema:"Set true to explicitly confirm destructive actions."`
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
	Route          toolutil.ActionRoute
}

type toolHandler func(context.Context, *mcp.CallToolRequest, toolutil.MetaToolInput) (*mcp.CallToolResult, any, error)

// Registry holds a deterministic hidden action index and dispatch handlers.
type Registry struct {
	entries  []actionEntry
	byID     map[string]actionEntry
	aliases  map[string]string
	handlers map[string]toolHandler
}

// RegisterTools registers the dynamic search, describe, and execute tools.
func RegisterTools(server *mcp.Server, routes map[string]toolutil.ActionMap) {
	registry := NewRegistry(routes)

	mcp.AddTool(server, &mcp.Tool{
		Name:         searchToolName,
		Title:        "GitLab Search Tools",
		Description:  "Search the hidden GitLab action registry. Use this first when you need to find the canonical action ID for gitlab_execute_tool.",
		Annotations:  annotationsWithTitle(toolutil.ReadAnnotations, "GitLab Search Tools"),
		Icons:        toolutil.IconSearch,
		OutputSchema: nil,
	}, registry.Search)

	mcp.AddTool(server, &mcp.Tool{
		Name:        describeToolName,
		Title:       "GitLab Describe Tools",
		Description: "Describe one or more hidden GitLab actions by canonical action ID, returning exact params schema, output summary, safety metadata, and an execute example.",
		Annotations: annotationsWithTitle(toolutil.ReadAnnotations, "GitLab Describe Tools"),
		Icons:       toolutil.IconConfig,
	}, registry.Describe)

	mcp.AddTool(server, &mcp.Tool{
		Name:        executeToolName,
		Title:       "GitLab Execute Tool",
		Description: "Execute one hidden GitLab action by canonical action ID. Call gitlab_describe_tools first for the exact params schema. Destructive actions require confirm=true.",
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
	routes = toolutil.CloneMetaSchemaRoutes(routes)
	registry := &Registry{
		byID:     make(map[string]actionEntry),
		aliases:  make(map[string]string),
		handlers: make(map[string]toolHandler),
	}

	toolNames := make([]string, 0, len(routes))
	for tool := range routes {
		toolNames = append(toolNames, tool)
	}
	sort.Strings(toolNames)

	for _, tool := range toolNames {
		actions := routes[tool]
		registry.handlers[tool] = toolutil.MakeMetaHandler(tool, actions, toolutil.MarkdownForResult)

		actionNames := make([]string, 0, len(actions))
		for action := range actions {
			actionNames = append(actionNames, action)
		}
		sort.Strings(actionNames)

		for _, action := range actionNames {
			route := actions[action]
			domain := domainFromTool(tool)
			id := domain + "." + action
			aliases := aliasesForCanonicalAction(id)
			tags := actionTags(id, domain, action, route.InputSchema)
			entry := actionEntry{
				ID:             id,
				Tool:           tool,
				Domain:         domain,
				Action:         action,
				Aliases:        aliases,
				Tags:           tags,
				SchemaURI:      toolutil.MetaSchemaURI(tool, action),
				Destructive:    route.Destructive,
				RequiredParams: requiredParams(route.InputSchema),
				SearchText:     buildSearchText(id, tool, domain, action, aliases, tags, route.InputSchema),
				Route:          route,
			}
			registry.entries = append(registry.entries, entry)
			registry.byID[id] = entry
			for _, alias := range aliases {
				registry.aliases[alias] = id
			}
		}
	}

	return registry
}

// Search finds hidden GitLab actions by lexical matching over action metadata.
func (r *Registry) Search(_ context.Context, _ *mcp.CallToolRequest, input SearchInput) (*mcp.CallToolResult, SearchOutput, error) {
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return toolutil.ErrorResult("gitlab_search_tools: query is required. Try terms like project create, merge request approve, pipeline retry, or ci variable."), SearchOutput{}, nil
	}

	limit := input.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	type scoredEntry struct {
		entry actionEntry
		score int
	}
	terms := normalizeSearchTerms(query)
	matches := make([]scoredEntry, 0)
	for _, entry := range r.entries {
		score := scoreEntry(entry, terms)
		if score > 0 {
			matches = append(matches, scoredEntry{entry: entry, score: score})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].entry.ID < matches[j].entry.ID
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}

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

// Describe returns schemas and execution metadata for hidden GitLab actions.
func (r *Registry) Describe(_ context.Context, _ *mcp.CallToolRequest, input DescribeInput) (*mcp.CallToolResult, DescribeOutput, error) {
	ids := normalizeDescribeIDs(input)
	if len(ids) == 0 {
		return toolutil.ErrorResult("gitlab_describe_tools: provide action or actions with canonical IDs from gitlab_search_tools."), DescribeOutput{}, nil
	}

	descriptions := make([]ActionDescription, 0, len(ids))
	for _, id := range ids {
		entry, ok := r.resolveAction(id)
		if !ok {
			return toolutil.ErrorResult(r.unknownActionMessage("gitlab_describe_tools", id)), DescribeOutput{}, nil
		}
		inputSchema, _ := toolutil.LookupMetaActionSchema(map[string]toolutil.ActionMap{entry.Tool: toolutil.ActionMap{entry.Action: entry.Route}}, entry.Tool, entry.Action)
		descriptions = append(descriptions, ActionDescription{
			ID:             entry.ID,
			Tool:           entry.Tool,
			Domain:         entry.Domain,
			Action:         entry.Action,
			SchemaURI:      entry.SchemaURI,
			Destructive:    entry.Destructive,
			RequiredParams: append([]string(nil), entry.RequiredParams...),
			InputSchema:    inputSchema,
			OutputSchema:   cloneSchema(entry.Route.OutputSchema),
			Example:        exampleFor(entry, inputSchema),
		})
	}

	output := DescribeOutput{Count: len(descriptions), Actions: descriptions}
	return toolutil.ToolResultAnnotated(formatDescribeOutput(output), toolutil.ContentDetail), output, nil
}

// Execute dispatches one hidden action through the existing meta-tool handler.
func (r *Registry) Execute(ctx context.Context, req *mcp.CallToolRequest, input ExecuteInput) (*mcp.CallToolResult, any, error) {
	id := strings.ToLower(strings.TrimSpace(input.Action))
	if id == "" {
		return toolutil.ErrorResult("gitlab_execute_tool: action is required. Use gitlab_search_tools to find a canonical action ID."), nil, nil
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

func domainFromTool(tool string) string {
	return strings.TrimPrefix(tool, "gitlab_")
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

func searchSynonyms() map[string][]string {
	return map[string][]string{
		"access":     {"token", "deploy", "member"},
		"artifact":   {"job", "download"},
		"ci":         {"pipeline", "job", "variable", "lint"},
		"deploy":     {"deployment", "environment", "key"},
		"deployment": {"deploy", "environment"},
		"details":    {"get"},
		"env":        {"environment"},
		"file":       {"repository", "blob", "content"},
		"info":       {"get"},
		"metadata":   {"get", "details"},
		"mr":         {"merge", "request", "merge_request"},
		"read":       {"get", "file", "content"},
		"repo":       {"repository", "file", "tree", "branch", "tag"},
		"secret":     {"variable", "ci_variable", "token"},
		"show":       {"get"},
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
		add("project discovery", "git remote", "remote url", "resolve project")
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

func (r *Registry) resolveAction(id string) (actionEntry, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	if entry, ok := r.byID[id]; ok {
		return entry, true
	}
	canonical, ok := r.aliases[id]
	if !ok {
		return actionEntry{}, false
	}
	entry, ok := r.byID[canonical]
	return entry, ok
}

func (r *Registry) unknownActionMessage(toolName, action string) string {
	suggestions := r.suggestActionIDs(action, 5)
	if len(suggestions) == 0 {
		return fmt.Sprintf("%s: unknown action %q. Use gitlab_search_tools to find canonical action IDs.", toolName, action)
	}
	return fmt.Sprintf("%s: unknown action %q. Did you mean %s? Use canonical action IDs with gitlab_execute_tool.", toolName, action, strings.Join(suggestions, ", "))
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
		suggestions = append(suggestions, "`"+entry.id+"`")
	}
	return suggestions
}

func aliasesForCanonicalAction(id string) []string {
	var aliases []string
	for alias, canonical := range actionAliases() {
		if canonical == id {
			aliases = append(aliases, alias)
		}
	}
	sort.Strings(aliases)
	return aliases
}

func actionAliases() map[string]string {
	return map[string]string{
		"badge.create":                              "project.badge_add",
		"badge.delete":                              "project.badge_delete",
		"broadcast_message.create":                  "admin.broadcast_message_create",
		"broadcast_message.delete":                  "admin.broadcast_message_delete",
		"ci_catalog.resource_list":                  "ci_catalog.list",
		"deploy_key.list":                           "access.deploy_key_list_project",
		"deploy_token.create":                       "access.deploy_token_create_project",
		"deploy_token.delete":                       "access.deploy_token_delete_project",
		"deploy_token.get":                          "access.deploy_token_get_project",
		"deploy_token.list":                         "access.deploy_token_list_project",
		"enterprise_user.group_list":                "enterprise_user.list",
		"external_status_check.list_project_checks": "external_status_check.list_project",
		"feature_flag.list":                         "feature_flags.feature_flag_list",
		"geo.node_list":                             "geo.list",
		"gitlab_server.health_check":                "server.health_check",
		"group.custom_member_roles_list":            "member_role.list_group",
		"job.download_single_artifact":              "job.artifact_download",
		"merge_train.list":                          "merge_train.list_project",
		"merge_request.changes":                     "mr_review.changes_get",
		"merge_request_note.create":                 "mr_review.note_create",
		"merge_request_note.delete":                 "mr_review.note_delete",
		"merge_request_note.get":                    "mr_review.note_get",
		"merge_request_note.update":                 "mr_review.note_update",
		"merge_request.add_spent_time":              "merge_request.spent_time_add",
		"merge_request.set_time_estimate":           "merge_request.time_estimate_set",
		"mr_review.draft_notes_publish":             "mr_review.draft_note_publish_all",
		"mr_review.publish":                         "mr_review.draft_note_publish_all",
		"package.list_generic":                      "package.list",
		"personal_snippet.raw":                      "snippet.content",
		"project.hooks.list":                        "project.hook_list",
		"project.member_remove":                     "project.member_delete",
		"project.member_update":                     "project.member_edit",
		"project.schedule_storage_move":             "storage_move.schedule_project",
		"project.status_check_list":                 "external_status_check.list_project",
		"project_member.add":                        "project.member_add",
		"project_member.delete":                     "project.member_delete",
		"project_member.edit":                       "project.member_edit",
		"project_member.get":                        "project.member_get",
		"project_member.remove":                     "project.member_delete",
		"project_member.update":                     "project.member_edit",
		"project_access_token.create":               "access.token_project_create",
		"project_access_token.revoke":               "access.token_project_revoke",
		"repository_file.create":                    "repository.file_create",
		"repository_file.delete":                    "repository.file_delete",
		"repository_file.get":                       "repository.file_get",
		"gitlab_discover_project":                   "discover_project.resolve",
		"interactive_issue.create":                  "interactive.issue_create",
		"interactive_issue_create":                  "interactive.issue_create",
		"gitlab_interactive_issue_create":           "interactive.issue_create",
		"gitlab_interactive_mr_create":              "interactive.mr_create",
		"gitlab_interactive_project_create":         "interactive.project_create",
		"gitlab_interactive_release_create":         "interactive.release_create",
		"runner.delete":                             "runner.remove",
		"webhook.add":                               "project.hook_add",
		"webhook.create":                            "project.hook_add",
		"webhook.delete":                            "project.hook_delete",
	}
}

func scoreEntry(entry actionEntry, terms []searchTerm) int {
	if len(terms) == 0 {
		return 0
	}
	score := 0
	for _, term := range terms {
		matched := false
		best := 0
		for _, alternative := range term.Alternatives {
			candidateScore := scoreSearchAlternative(entry, term.Raw, alternative)
			if candidateScore > best {
				best = candidateScore
			}
			if candidateScore > 0 {
				matched = true
			}
		}
		if !matched {
			return 0
		}
		score += best
	}
	return score
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
	data, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	var clone map[string]any
	if decodeErr := json.Unmarshal(data, &clone); decodeErr != nil {
		return nil
	}
	return clone
}

func formatSearchOutput(output SearchOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## GitLab Action Search\n\n")
	if output.Count == 0 {
		fmt.Fprintf(&b, "No hidden actions matched %q. Try broader terms such as project, issue, merge request, pipeline, branch, or user.\n", output.Query)
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
