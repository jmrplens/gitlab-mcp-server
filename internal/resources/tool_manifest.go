package resources

import (
	"context"
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
	ID             string   `json:"id"`
	Kind           string   `json:"kind"`
	Tool           string   `json:"tool"`
	Action         string   `json:"action,omitempty"`
	Domain         string   `json:"domain,omitempty"`
	BackingTool    string   `json:"backing_tool,omitempty"`
	BackingAction  string   `json:"backing_action,omitempty"`
	Title          string   `json:"title,omitempty"`
	Description    string   `json:"description,omitempty"`
	DetailURI      string   `json:"detail_uri"`
	Destructive    bool     `json:"destructive"`
	ReadOnly       bool     `json:"read_only"`
	RequiredParams []string `json:"required_params,omitempty"`
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
			Description:    actionDescription(action),
			Destructive:    action.Route.Destructive,
			ReadOnly:       action.ReadOnly,
			RequiredParams: dynamicRequiredParams(action.Route.InputSchema),
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
		for _, action := range catalog.Actions() {
			if !metaRouteVisible(routeSnapshot, action.ToolName, action.Name) {
				continue
			}
			snapshot.addMetaAction(action, routeSnapshot)
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
				RequiredParams: dynamicRequiredParams(route.InputSchema),
			}
			snapshot.addMetaEntry(entry, routeSnapshot)
		}
	}
}

func (snapshot *toolSurfaceSnapshot) addMetaAction(action actioncatalog.Action, routes map[string]toolutil.ActionMap) {
	entry := ToolSurfaceEntry{
		ID:             metaManifestID(action.ToolName, action.Name),
		Kind:           toolManifestKindMetaAction,
		Tool:           action.ToolName,
		Action:         action.Name,
		Domain:         action.Domain,
		Title:          actionTitle(action),
		Description:    actionDescription(action),
		Destructive:    action.Route.Destructive,
		ReadOnly:       action.ReadOnly,
		RequiredParams: dynamicRequiredParams(action.Route.InputSchema),
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

func requiredParamsFromInputSchema(inputSchema any) []string {
	schema, ok := inputSchema.(map[string]any)
	if !ok {
		return nil
	}
	return dynamicRequiredParams(schema)
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

func actionDescription(action actioncatalog.Action) string {
	if action.IndividualTool.Description != "" {
		return action.IndividualTool.Description
	}
	return action.Usage
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
