package resources

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const toolsManifestURI = "gitlab://tools"

const toolsManifestTemplateURI = "gitlab://tools/{id}"

const toolsManifestDetailPrefix = "gitlab://tools/"

const (
	toolSurfaceDynamic    = "dynamic"
	toolSurfaceMeta       = "meta"
	toolSurfaceIndividual = "individual"

	toolManifestKindDynamicAction  = "dynamic_action"
	toolManifestKindMetaAction     = "meta_action"
	toolManifestKindIndividualTool = "individual_tool"
	toolManifestKindVisibleTool    = "visible_tool"
)

// ToolSurfaceResourceOptions captures the active server tool surface
// for the unified tool manifest resources ([RegisterToolSurfaceResources]).
// All three slices are projected differently depending on the surface
// (see [newToolSurfaceSnapshot]).
type ToolSurfaceResourceOptions struct {
	Surface    string
	Tools      []*mcp.Tool
	Catalog    *actioncatalog.Catalog
	MetaRoutes map[string]toolutil.ActionMap
	// SubscribableURITemplates lists the resource URI templates that accept
	// resources/subscribe on this server. Empty means subscriptions are not
	// offered (CAPABILITY_SURFACE=minimal), and the manifest then omits the
	// section rather than advertising a capability the server would refuse.
	SubscribableURITemplates []string
}

// ToolSurfaceVisibleTool summarizes one MCP tool currently advertised
// through tools/list, surfaced in the [ToolSurfaceManifest]'s
// VisibleTools list.
type ToolSurfaceVisibleTool struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	DetailURI   string `json:"detail_uri"`
	ReadOnly    bool   `json:"read_only"`
	Destructive bool   `json:"destructive"`
}

// ToolSurfaceEntry describes one executable unit in the active tool
// surface. Entries can be dynamic actions (gitlab_execute_action
// surface), meta actions (gitlab_<tool>.<action> surface), or
// individual tools (one MCP tool per GitLab action).
type ToolSurfaceEntry struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Tool          string `json:"tool"`
	Action        string `json:"action,omitempty"`
	Domain        string `json:"domain,omitempty"`
	BackingTool   string `json:"backing_tool,omitempty"`
	BackingAction string `json:"backing_action,omitempty"`
	Title         string `json:"title,omitempty"`
	Description   string `json:"description,omitempty"`
	// AliasOf names the primary entry when this action is a deliberate
	// alias: a second canonical ID projected from the same route and the
	// same individual tool, kept for discovery (user.me for user.current,
	// repository.file_history for repository.commit_list). Clients that
	// dedupe should keep the primary and treat this entry as a pointer.
	AliasOf        string                     `json:"alias_of,omitempty"`
	DetailURI      string                     `json:"detail_uri"`
	Destructive    bool                       `json:"destructive"`
	ReadOnly       bool                       `json:"read_only"`
	RequiredParams []ToolSurfaceRequiredParam `json:"required_params,omitempty"`
}

// ToolSurfaceRequiredParam names one required parameter of a manifest
// entry together with its flat JSON-Schema type ("integer", "string",
// "boolean", "array", …; a multi-typed parameter joins them as
// "integer|string"). Only the name and the plain type live here — a
// static consumer of the aggregate manifest should not need 851 detail
// reads to label a parameter — while descriptions, enums, and optional
// parameters stay in the per-entry input schema at gitlab://tools/{id}.
type ToolSurfaceRequiredParam struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

// manifestRequiredParams pairs a schema's required parameter names with
// their flat types.
func manifestRequiredParams(schema map[string]any) []ToolSurfaceRequiredParam {
	names := dynamicRequiredParams(schema)
	if len(names) == 0 {
		return nil
	}
	properties, _ := schema["properties"].(map[string]any)
	params := make([]ToolSurfaceRequiredParam, 0, len(names))
	for _, name := range names {
		params = append(params, ToolSurfaceRequiredParam{
			Name: name,
			Type: flatSchemaType(properties, name),
		})
	}
	return params
}

// flatSchemaType reads the plain "type" of one property, joining a
// multi-type list ("project_id accepts integer or string") with "|".
// "null" is dropped from multi-type lists — the SDK types nullable Go
// slices as ["null","array"], and for a required parameter the nullable
// half is schema plumbing, not something a reader passes. Properties
// without a stated type — $ref, oneOf-only, absent — return "" and the
// field is omitted rather than guessed.
func flatSchemaType(properties map[string]any, name string) string {
	property, _ := properties[name].(map[string]any)
	switch value := property["type"].(type) {
	case string:
		return value
	case []any:
		parts := make([]string, 0, len(value))
		for _, entry := range value {
			if part, ok := entry.(string); ok && part != "" && part != "null" {
				parts = append(parts, part)
			}
		}
		return strings.Join(parts, "|")
	}
	return ""
}

// ToolSurfaceManifest is the JSON payload returned by the
// "gitlab://tools" resource. It summarizes the active tool surface
// and lists every executable entry the surface exposes.
type ToolSurfaceManifest struct {
	Surface          string `json:"surface"`
	URITemplate      string `json:"uri_template"`
	VisibleToolCount int    `json:"visible_tool_count"`
	EntryCount       int    `json:"entry_count"`
	// Subscriptions describes the resources/subscribe support this server
	// offers, or is omitted when it offers none. It lives in the manifest —
	// the one resource kept even on the minimal capability surface — so a
	// machine consumer has a single place to learn the watchable set.
	Subscriptions *ToolSurfaceSubscriptions `json:"subscriptions,omitempty"`
	VisibleTools  []ToolSurfaceVisibleTool  `json:"visible_tools"`
	Entries       []ToolSurfaceEntry        `json:"entries"`
}

// ToolSurfaceSubscriptions advertises resources/subscribe support in the
// gitlab://tools manifest.
type ToolSurfaceSubscriptions struct {
	Supported bool `json:"supported"`
	// SubscribableURITemplates are the single-object resource URI templates
	// a subscription is accepted for; anything else is refused. Sourced
	// from the enforcement whitelist itself, never copied.
	SubscribableURITemplates []string `json:"subscribable_uri_templates"`
	Notification             string   `json:"notification"`
}

// ToolSurfaceCallShape describes how to invoke one manifest entry.
// ActionLocation and ConfirmLocation are populated only for surfaces
// where those fields apply (dynamic, meta); they are empty in
// individual mode.
type ToolSurfaceCallShape struct {
	Tool            string `json:"tool"`
	Action          string `json:"action,omitempty"`
	ActionLocation  string `json:"action_location,omitempty"`
	ParamsLocation  string `json:"params_location"`
	ConfirmLocation string `json:"confirm_location,omitempty"`
}

// ToolSurfaceDetail is the JSON payload returned by the
// "gitlab://tools/{id}" resource. It embeds the matching
// [ToolSurfaceEntry] and adds the per-entry call shape and input
// schema.
type ToolSurfaceDetail struct {
	ToolSurfaceEntry
	Call        ToolSurfaceCallShape `json:"call"`
	InputSchema any                  `json:"input_schema,omitempty"`
}

type toolSurfaceSnapshot struct {
	manifest ToolSurfaceManifest
	details  map[string]ToolSurfaceDetail
}

type toolSnapshot struct {
	Name        string
	Title       string
	Description string
	InputSchema any
	ReadOnly    bool
	Destructive bool
}

// RegisterToolSurfaceResources wires a surface-aware tool manifest
// into the MCP server. Two resources are registered:
//
//   - The static "gitlab://tools" resource, which lists the active
//     surface ("dynamic", "meta", or "individual") and every
//     executable entry that surface exposes.
//   - The "gitlab://tools/{id}" template resource, which returns the
//     accepted call shape and input schema for one entry.
//
// Use [ToolSurfaceResourceOptions] to pass the active tool surface;
// see [newToolSurfaceSnapshot] for the projection rules.
func RegisterToolSurfaceResources(server *mcp.Server, opts ToolSurfaceResourceOptions) {
	snapshot := newToolSurfaceSnapshot(opts)
	registerToolManifestIndex(server, snapshot)
	registerToolManifestTemplate(server, snapshot)
}

// registerToolManifestIndex registers the static catalog resource that
// lists the active surface and every executable entry.
func registerToolManifestIndex(server *mcp.Server, snapshot toolSurfaceSnapshot) {
	server.AddResource(&mcp.Resource{
		URI:         toolsManifestURI,
		Name:        "tool_manifest",
		Title:       "Tool Manifest",
		MIMEType:    mimeJSON,
		Description: "Surface-aware manifest of the tools and executable actions available in this server instance. Use gitlab://tools/{id} to fetch one entry's accepted call shape and input schema.",
		Annotations: toolutil.ResourceMachineList,
		Icons:       toolutil.IconConfig,
	}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return marshalResourceJSON(snapshot.manifest)
	})
}

// registerToolManifestTemplate registers the URI-template resource that
// returns the call shape and input schema for one entry from the
// surface manifest.
func registerToolManifestTemplate(server *mcp.Server, snapshot toolSurfaceSnapshot) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: toolsManifestTemplateURI,
		Name:        "tool_detail",
		Title:       "Tool Detail",
		MIMEType:    mimeJSON,
		Description: "Accepted call shape and input schema for one entry from gitlab://tools. Replace {id} with an entry ID from the active surface, such as project.get in dynamic mode, gitlab_project.get in meta mode, or gitlab_get_project in individual mode.",
		Annotations: toolutil.ResourceMachineDetail,
		Icons:       toolutil.IconConfig,
	}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		id := parseToolManifestURI(req.Params.URI)
		if id == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		detail, ok := snapshot.details[id]
		if !ok {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		return marshalResourceJSON(detail)
	})
}

func newToolSurfaceSnapshot(opts ToolSurfaceResourceOptions) toolSurfaceSnapshot {
	visibleTools, toolDetails := visibleToolSnapshots(opts.Tools)
	snapshot := toolSurfaceSnapshot{
		manifest: ToolSurfaceManifest{
			Surface:          normalizeToolSurface(opts.Surface),
			URITemplate:      toolsManifestTemplateURI,
			VisibleToolCount: len(visibleTools),
			VisibleTools:     visibleTools,
		},
		details: make(map[string]ToolSurfaceDetail, len(toolDetails)),
	}
	if len(opts.SubscribableURITemplates) > 0 {
		snapshot.manifest.Subscriptions = &ToolSurfaceSubscriptions{
			Supported:                true,
			SubscribableURITemplates: opts.SubscribableURITemplates,
			Notification:             "notifications/resources/updated, sent when the watched content changes (server polls GitLab)",
		}
	}
	for _, tool := range toolDetails {
		snapshot.addDirectToolDetail(tool, toolManifestKindVisibleTool)
	}

	switch snapshot.manifest.Surface {
	case toolSurfaceDynamic:
		snapshot.addDynamicActions(opts.Catalog)
	case toolSurfaceMeta:
		snapshot.addMetaActions(opts.Catalog, opts.MetaRoutes)
	default:
		snapshot.manifest.Surface = toolSurfaceIndividual
		for _, tool := range toolDetails {
			snapshot.addDirectToolEntry(tool, toolManifestKindIndividualTool)
		}
	}
	snapshot.addUncoveredDirectTools(toolDetails)
	sort.Slice(snapshot.manifest.Entries, func(i, j int) bool {
		return snapshot.manifest.Entries[i].ID < snapshot.manifest.Entries[j].ID
	})
	snapshot.manifest.EntryCount = len(snapshot.manifest.Entries)
	return snapshot
}

func normalizeToolSurface(surface string) string {
	switch strings.ToLower(strings.TrimSpace(surface)) {
	case toolSurfaceDynamic:
		return toolSurfaceDynamic
	case toolSurfaceMeta:
		return toolSurfaceMeta
	case toolSurfaceIndividual:
		return toolSurfaceIndividual
	default:
		return toolSurfaceIndividual
	}
}

func visibleToolSnapshots(tools []*mcp.Tool) ([]ToolSurfaceVisibleTool, []toolSnapshot) {
	details := make([]toolSnapshot, 0, len(tools))
	for _, tool := range tools {
		if tool == nil || tool.Name == "" {
			continue
		}
		details = append(details, toolSnapshot{
			Name:        tool.Name,
			Title:       tool.Title,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
			ReadOnly:    tool.Annotations != nil && tool.Annotations.ReadOnlyHint,
			Destructive: tool.Annotations != nil && tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint,
		})
	}
	sort.Slice(details, func(i, j int) bool { return details[i].Name < details[j].Name })
	visible := make([]ToolSurfaceVisibleTool, 0, len(details))
	for _, tool := range details {
		visible = append(visible, ToolSurfaceVisibleTool{
			Name:        tool.Name,
			Title:       tool.Title,
			DetailURI:   toolManifestDetailURI(tool.Name),
			ReadOnly:    tool.ReadOnly,
			Destructive: tool.Destructive,
		})
	}
	return visible, details
}

func (snapshot *toolSurfaceSnapshot) addDynamicActions(catalog *actioncatalog.Catalog) {
	if catalog == nil || !snapshot.hasVisibleTool("gitlab_execute_action") {
		return
	}
	resolve := dynamicSeeAlso(newSeeAlsoIndex(catalog))
	aliases := aliasPrimaries(catalog)
	for _, action := range catalog.Actions() {
		entry := ToolSurfaceEntry{
			ID:             string(action.ID),
			Kind:           toolManifestKindDynamicAction,
			Tool:           "gitlab_execute_action",
			Action:         string(action.ID),
			Domain:         action.Domain,
			BackingTool:    action.ToolName,
			BackingAction:  action.Name,
			Title:          actionTitle(action),
			Description:    actionDescription(action, resolve),
			Destructive:    action.Route.Destructive,
			ReadOnly:       action.ReadOnly,
			RequiredParams: manifestRequiredParams(action.Route.InputSchema),
		}
		if primary, ok := aliases[string(action.ID)]; ok {
			entry.AliasOf = string(primary.ID)
		}
		call := ToolSurfaceCallShape{
			Tool:           "gitlab_execute_action",
			Action:         string(action.ID),
			ActionLocation: "action",
			ParamsLocation: "params",
		}
		if entry.Destructive {
			call.ConfirmLocation = "confirm"
		}
		snapshot.addEntry(entry, call, dynamicActionSchema(action))
	}
}

func (snapshot *toolSurfaceSnapshot) hasVisibleTool(name string) bool {
	for _, tool := range snapshot.manifest.VisibleTools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func (snapshot *toolSurfaceSnapshot) addMetaActions(catalog *actioncatalog.Catalog, routes map[string]toolutil.ActionMap) {
	routeSnapshot := cloneMetaSchemaRoutes(routes)
	seen := make(map[string]struct{})
	if catalog != nil {
		resolve := metaSeeAlso(newSeeAlsoIndex(catalog))
		aliases := aliasPrimaries(catalog)
		for _, action := range catalog.Actions() {
			if !metaRouteVisible(routeSnapshot, action.ToolName, action.Name) {
				continue
			}
			snapshot.addMetaAction(action, routeSnapshot, resolve, aliases)
			seen[metaManifestID(action.ToolName, action.Name)] = struct{}{}
		}
	}
	for _, toolName := range sortedActionMapKeys(routeSnapshot) {
		for _, actionName := range sortedRouteNames(routeSnapshot[toolName]) {
			id := metaManifestID(toolName, actionName)
			if _, ok := seen[id]; ok {
				continue
			}
			route := routeSnapshot[toolName][actionName]
			entry := ToolSurfaceEntry{
				ID:             id,
				Kind:           toolManifestKindMetaAction,
				Tool:           toolName,
				Action:         actionName,
				DetailURI:      toolManifestDetailURI(id),
				Destructive:    route.Destructive,
				RequiredParams: manifestRequiredParams(route.InputSchema),
			}
			snapshot.addMetaEntry(entry, routeSnapshot)
		}
	}
}

func (snapshot *toolSurfaceSnapshot) addMetaAction(action actioncatalog.Action, routes map[string]toolutil.ActionMap, resolve seeAlsoResolver, aliases map[string]actioncatalog.Action) {
	entry := ToolSurfaceEntry{
		ID:             metaManifestID(action.ToolName, action.Name),
		Kind:           toolManifestKindMetaAction,
		Tool:           action.ToolName,
		Action:         action.Name,
		Domain:         action.Domain,
		Title:          actionTitle(action),
		Description:    actionDescription(action, resolve),
		Destructive:    action.Route.Destructive,
		ReadOnly:       action.ReadOnly,
		RequiredParams: manifestRequiredParams(action.Route.InputSchema),
	}
	if primary, ok := aliases[string(action.ID)]; ok {
		entry.AliasOf = metaManifestID(primary.ToolName, primary.Name)
	}
	snapshot.addMetaEntry(entry, routes)
}

func (snapshot *toolSurfaceSnapshot) addMetaEntry(entry ToolSurfaceEntry, routes map[string]toolutil.ActionMap) {
	call := ToolSurfaceCallShape{
		Tool:           entry.Tool,
		Action:         entry.Action,
		ActionLocation: "action",
		ParamsLocation: "params",
	}
	if entry.Destructive {
		call.ConfirmLocation = "params.confirm"
	}
	schema, _ := lookupMetaActionSchema(routes, entry.Tool, entry.Action)
	snapshot.addEntry(entry, call, schema)
}

// addUncoveredDirectTools gives an entry to every visible tool the
// surface-specific pass left unaccounted for.
//
// The manifest promises every executable entry, but the dynamic and meta
// passes only enumerate actions reached through a dispatcher. Standalone
// utilities — project discovery, the interactive creation flows — are called
// directly under their own name, so they belong to no dispatcher and were
// listed in visible_tools while missing from entries: a model enumerating
// entries to learn what it could call never saw them. On the individual
// surface every tool is already its own entry, so this pass is a no-op there.
//
// A tool counts as covered when some entry names it as the tool to call,
// which is how gitlab_execute_action and the meta dispatchers are represented.
func (snapshot *toolSurfaceSnapshot) addUncoveredDirectTools(tools []toolSnapshot) {
	covered := make(map[string]struct{}, len(snapshot.manifest.Entries))
	for _, entry := range snapshot.manifest.Entries {
		covered[entry.Tool] = struct{}{}
	}
	for _, tool := range tools {
		if _, ok := covered[tool.Name]; ok {
			continue
		}
		snapshot.addDirectToolEntry(tool, toolManifestKindVisibleTool)
	}
}

func (snapshot *toolSurfaceSnapshot) addDirectToolEntry(tool toolSnapshot, kind string) {
	entry := directToolEntry(tool, kind)
	snapshot.manifest.Entries = append(snapshot.manifest.Entries, entry)
	snapshot.details[entry.ID] = directToolDetail(entry, tool)
}

func (snapshot *toolSurfaceSnapshot) addDirectToolDetail(tool toolSnapshot, kind string) {
	entry := directToolEntry(tool, kind)
	snapshot.details[entry.ID] = directToolDetail(entry, tool)
}

func (snapshot *toolSurfaceSnapshot) addEntry(entry ToolSurfaceEntry, call ToolSurfaceCallShape, inputSchema any) {
	entry.DetailURI = toolManifestDetailURI(entry.ID)
	snapshot.manifest.Entries = append(snapshot.manifest.Entries, entry)
	snapshot.details[entry.ID] = ToolSurfaceDetail{
		ToolSurfaceEntry: entry,
		Call:             call,
		InputSchema:      inputSchema,
	}
}

func directToolEntry(tool toolSnapshot, kind string) ToolSurfaceEntry {
	return ToolSurfaceEntry{
		ID:             tool.Name,
		Kind:           kind,
		Tool:           tool.Name,
		Title:          tool.Title,
		Description:    tool.Description,
		DetailURI:      toolManifestDetailURI(tool.Name),
		Destructive:    tool.Destructive,
		ReadOnly:       tool.ReadOnly,
		RequiredParams: requiredParamsFromInputSchema(tool.InputSchema),
	}
}

func directToolDetail(entry ToolSurfaceEntry, tool toolSnapshot) ToolSurfaceDetail {
	call := ToolSurfaceCallShape{
		Tool:           entry.Tool,
		ParamsLocation: "arguments",
	}
	if entry.Destructive {
		call.ConfirmLocation = "arguments.confirm"
	}
	return ToolSurfaceDetail{
		ToolSurfaceEntry: entry,
		Call:             call,
		InputSchema:      tool.InputSchema,
	}
}

func requiredParamsFromInputSchema(inputSchema any) []ToolSurfaceRequiredParam {
	schema, ok := inputSchema.(map[string]any)
	if !ok {
		return nil
	}
	return manifestRequiredParams(schema)
}

func actionTitle(action actioncatalog.Action) string {
	if action.IndividualTool.Title != "" {
		return action.IndividualTool.Title
	}
	if action.ToolName != "" && action.Name != "" {
		return toolutil.TitleFromName(action.ToolName + "_" + action.Name)
	}
	return ""
}

func actionDescription(action actioncatalog.Action, resolve seeAlsoResolver) string {
	description := action.IndividualTool.Description
	if description == "" {
		return action.Usage
	}
	return rewriteSeeAlso(description, resolve)
}

// seeAlsoResolver maps an individual-surface tool name to the identifier
// the active surface's entries are invoked by, reporting whether the name
// is known. A nil resolver leaves descriptions untouched (the individual
// surface, whose namespace the hand-written clauses already use).
type seeAlsoResolver func(individualName string) (string, bool)

// seeAlsoClause matches the trailing cross-reference sentence the action
// specs write for the individual surface — "See also: gitlab_a, gitlab_b."
// — and, because the character class admits dots, also a clause already
// projected into entry IDs ("See also: widget.create, gitlab_widget.get."),
// which is what lets the guard test parse the rewritten output with the
// same pattern. The terminating literal dot still matches: the greedy
// class backtracks one character off the final name.
var seeAlsoClause = regexp.MustCompile(`See also: ([a-z0-9_.]+(?:, [a-z0-9_.]+)*)\.`)

// rewriteSeeAlso projects the "See also:" clause of an individual-surface
// description into the active surface's identifier namespace.
//
// The specs hand-write these clauses once, in individual-tool names; on the
// dynamic and meta surfaces those names are not invocable, and the manifest
// instructions tell the model to pass entry IDs — so emitting the
// individual names there contradicts the same document two lines later.
//
// A name the resolver does not know is dropped, not passed through: on this
// instance the catalog is tier-filtered, so a Free-tier server legitimately
// cannot resolve a reference to a Premium action — and a name that resolves
// to nothing on the whole instance is not a reference, it is noise. Stale
// names cannot hide behind this: the guard test checks the hand-written
// clauses against the full unfiltered catalog, where only a genuinely wrong
// name fails. A clause left empty is removed whole.
func rewriteSeeAlso(description string, resolve seeAlsoResolver) string {
	if resolve == nil {
		return description
	}
	rewritten := seeAlsoClause.ReplaceAllStringFunc(description, func(clause string) string {
		names := strings.Split(strings.TrimSuffix(strings.TrimPrefix(clause, "See also: "), "."), ", ")
		kept := names[:0]
		for _, name := range names {
			if id, ok := resolve(name); ok {
				kept = append(kept, id)
			}
		}
		if len(kept) == 0 {
			return ""
		}
		return "See also: " + strings.Join(kept, ", ") + "."
	})
	return strings.TrimRight(rewritten, " \n")
}

// aliasPrimaries maps each action that shares its individual tool with an
// earlier catalog action to that earlier (primary) action. Sharing the
// individual name is what makes a pair a deliberate alias: both project to
// one tool on the individual surface, so on the other surfaces the second
// canonical ID is a discovery pointer, not a distinct operation.
func aliasPrimaries(catalog *actioncatalog.Catalog) map[string]actioncatalog.Action {
	first := make(map[string]actioncatalog.Action)
	aliases := make(map[string]actioncatalog.Action)
	for _, action := range catalog.Actions() {
		name := action.IndividualTool.Name
		if name == "" {
			continue
		}
		if primary, ok := first[name]; ok {
			aliases[string(action.ID)] = primary
		} else {
			first[name] = action
		}
	}
	return aliases
}

// newSeeAlsoIndex indexes a catalog's actions by their individual-surface
// tool name, the namespace the hand-written clauses are addressed in.
func newSeeAlsoIndex(catalog *actioncatalog.Catalog) map[string]actioncatalog.Action {
	if catalog == nil {
		return nil
	}
	index := make(map[string]actioncatalog.Action)
	for _, action := range catalog.Actions() {
		if action.IndividualTool.Name != "" {
			index[action.IndividualTool.Name] = action
		}
	}
	return index
}

// dynamicSeeAlso resolves to canonical action IDs — the only identifier
// gitlab_execute_action accepts.
func dynamicSeeAlso(index map[string]actioncatalog.Action) seeAlsoResolver {
	return func(name string) (string, bool) {
		action, ok := index[name]
		if !ok {
			return "", false
		}
		return string(action.ID), true
	}
}

// metaSeeAlso resolves to the meta surface's entry IDs
// (gitlab_<tool>.<action>), matching the manifest's own ID scheme there.
func metaSeeAlso(index map[string]actioncatalog.Action) seeAlsoResolver {
	return func(name string) (string, bool) {
		action, ok := index[name]
		if !ok {
			return "", false
		}
		return metaManifestID(action.ToolName, action.Name), true
	}
}

func metaRouteVisible(routes map[string]toolutil.ActionMap, toolName, actionName string) bool {
	actions, ok := routes[toolName]
	if !ok {
		return false
	}
	_, ok = actions[actionName]
	return ok
}

func sortedActionMapKeys(routes map[string]toolutil.ActionMap) []string {
	keys := make([]string, 0, len(routes))
	for key := range routes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedRouteNames(routes toolutil.ActionMap) []string {
	names := make([]string, 0, len(routes))
	for name := range routes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func metaManifestID(toolName, actionName string) string {
	return toolName + "." + actionName
}

func toolManifestDetailURI(id string) string {
	return toolsManifestDetailPrefix + id
}

func parseToolManifestURI(uri string) string {
	rest := strings.TrimPrefix(uri, toolsManifestDetailPrefix)
	if rest == uri || rest == "" || strings.Contains(rest, "/") {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(rest))
}
