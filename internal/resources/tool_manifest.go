package resources

import (
	"cmp"
	"context"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
	// ShareKey, when set, names the configuration the manifest describes,
	// and the snapshot built for it is cached for the process and served by
	// every server registered under the same key. The caller must include
	// in it everything the manifest depends on other than the credential:
	// the surface, the shared catalog's identity, the narrowing that
	// decides which tools are visible, the meta parameter-schema mode and
	// the capability surface. Empty builds a private snapshot.
	ShareKey string
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
	// RequiredParamsAnyOf lists alternative requirement groups: the schema
	// accepts a call satisfying at least one group ("name, description or
	// color", "file_name+content or files"). These names are NOT in
	// RequiredParams — publishing an alternative as unconditionally
	// required invites clients to send every branch at once.
	RequiredParamsAnyOf [][]ToolSurfaceRequiredParam `json:"required_params_any_of,omitempty"`
}

// ToolSurfaceRequiredParam names one required parameter of a manifest
// entry together with its flat JSON-Schema type ("integer", "string",
// "boolean", "array", …; a multi-typed parameter joins them as
// "integer|string"). Only the name and the plain type live here — a
// static consumer of the aggregate manifest should not need 851 detail
// reads to label a parameter — while descriptions, enums, and optional
// parameters stay in the per-entry input schema at gitlab://tools/{id}.
//
// An absent type is deliberate, not an omission bug: the schema states no
// single plain type for that parameter (admin.feature_set's value accepts
// boolean, string, or integer). Consumers should read it as "any" and
// consult the entry's input schema for the full shape.
type ToolSurfaceRequiredParam struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

// manifestRequiredParams pairs a schema's unconditionally required
// parameter names — the top-level "required" list only — with their flat
// types. Alternative-branch requirements (anyOf/oneOf) are published
// separately by [manifestAlternativeRequiredParams]: a branch name is not
// unconditionally required, and declaring it so invites a client to send
// every branch at once.
func manifestRequiredParams(schema map[string]any) []ToolSurfaceRequiredParam {
	if schema == nil {
		return nil
	}
	names := dedupeDynamicStrings(sortedStrings(appendDynamicRequiredParamNames(nil, schema["required"])))
	if len(names) == 0 {
		return nil
	}
	return typedParams(schema, names)
}

// manifestAlternativeRequiredParams extracts the anyOf/oneOf requirement
// groups: each group is one branch's "required" list, minus names already
// unconditionally required at the top level. A call must satisfy at least
// one group.
func manifestAlternativeRequiredParams(schema map[string]any) [][]ToolSurfaceRequiredParam {
	if schema == nil {
		return nil
	}
	top := make(map[string]bool)
	for _, name := range appendDynamicRequiredParamNames(nil, schema["required"]) {
		top[name] = true
	}
	var groups [][]ToolSurfaceRequiredParam
	for _, keyword := range []string{"anyOf", "oneOf"} {
		alternatives, ok := schema[keyword].([]any)
		if !ok {
			continue
		}
		for _, alternative := range alternatives {
			branch, isObject := alternative.(map[string]any)
			if !isObject {
				continue
			}
			var names []string
			for _, name := range appendDynamicRequiredParamNames(nil, branch["required"]) {
				if !top[name] {
					names = append(names, name)
				}
			}
			if len(names) > 0 {
				groups = append(groups, typedParams(schema, names))
			}
		}
	}
	return groups
}

// typedParams pairs parameter names with their flat schema types.
func typedParams(schema map[string]any, names []string) []ToolSurfaceRequiredParam {
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

// sortedStrings sorts a copy-in-place and returns it, for deterministic
// manifest output.
func sortedStrings(values []string) []string {
	sort.Strings(values)
	return values
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
	snapshot := toolSurfaceSnapshotFor(opts)
	registerToolManifestIndex(server, snapshot)
	registerToolManifestTemplate(server, snapshot)
}

// ToolSurfaceResourceURIs names what [RegisterToolSurfaceResources] registers:
// the manifest's static URI and the template of its per-entry detail.
//
// They are the two resources whose content the active tool surface decides.
// Every other resource is registered from the capability surface and the
// operator's exclusions alone, and reads the same whichever tool surface and
// protective mode a session runs,
// while these two list what the surface registered after the read-only and
// safe passes. A reader outside this package that has to tell them apart, which
// the e2e coverage command does to count them at that finer grain, asks here
// rather than spelling the two strings again, so a rename here reaches it.
//
// Each call returns a slice of its own, so a caller may sort or trim it.
func ToolSurfaceResourceURIs() []string {
	return []string{toolsManifestURI, toolsManifestTemplateURI}
}

// sharedToolSurfaceSnapshots holds one snapshot per share key. Single-flight,
// so a startup burst of servers under one key projects the surface once
// rather than once each and discards all but one of the snapshots.
var sharedToolSurfaceSnapshots toolutil.OnceMap[string, *toolSurfaceSnapshot]

// toolSurfaceSnapshotFor returns the snapshot for opts: the one cached under
// opts.ShareKey when a key was given, built by the first caller for it while
// every concurrent caller for that key waits, and a private one otherwise.
func toolSurfaceSnapshotFor(opts ToolSurfaceResourceOptions) *toolSurfaceSnapshot {
	if opts.ShareKey == "" {
		snapshot := newToolSurfaceSnapshot(opts)
		return &snapshot
	}
	return sharedToolSurfaceSnapshots.Load(opts.ShareKey, func() *toolSurfaceSnapshot {
		built := newToolSurfaceSnapshot(opts)
		return &built
	})
}

// registerToolManifestIndex registers the static catalog resource that
// lists the active surface and every executable entry.
func registerToolManifestIndex(server *mcp.Server, snapshot *toolSurfaceSnapshot) {
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
func registerToolManifestTemplate(server *mcp.Server, snapshot *toolSurfaceSnapshot) {
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
	// Asked of every surface rather than of the two that need it. The dynamic
	// surface files its details under the canonical ID already, so this finds
	// each one taken and does nothing, and that is worth a pass over the
	// catalog: the alternative is a rule about which surfaces alias, written
	// here, that a reader of aliasCanonicalActionIDs cannot see.
	snapshot.aliasCanonicalActionIDs(opts.Catalog)
	snapshot.addUncoveredDirectTools(toolDetails)
	slices.SortFunc(snapshot.manifest.Entries, func(a, b ToolSurfaceEntry) int { return cmp.Compare(a.ID, b.ID) })
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
	slices.SortFunc(details, func(a, b toolSnapshot) int { return cmp.Compare(a.Name, b.Name) })
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
			ID:                  string(action.ID),
			Kind:                toolManifestKindDynamicAction,
			Tool:                "gitlab_execute_action",
			Action:              string(action.ID),
			Domain:              action.Domain,
			BackingTool:         action.ToolName,
			BackingAction:       action.Name,
			Title:               actionTitle(action),
			Description:         actionDescription(action, resolve),
			Destructive:         action.Route.Destructive,
			ReadOnly:            action.ReadOnly,
			RequiredParams:      manifestRequiredParams(action.Route.InputSchema),
			RequiredParamsAnyOf: manifestAlternativeRequiredParams(action.Route.InputSchema),
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
		resolve := metaSeeAlso(newSeeAlsoIndex(catalog), routeSnapshot)
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
				ID:                  id,
				Kind:                toolManifestKindMetaAction,
				Tool:                toolName,
				Action:              actionName,
				DetailURI:           toolManifestDetailURI(id),
				Destructive:         route.Destructive,
				RequiredParams:      manifestRequiredParams(route.InputSchema),
				RequiredParamsAnyOf: manifestAlternativeRequiredParams(route.InputSchema),
			}
			snapshot.addMetaEntry(entry, routeSnapshot)
		}
	}
}

func (snapshot *toolSurfaceSnapshot) addMetaAction(action actioncatalog.Action, routes map[string]toolutil.ActionMap, resolve seeAlsoResolver, aliases map[string]actioncatalog.Action) {
	entry := ToolSurfaceEntry{
		ID:                  metaManifestID(action.ToolName, action.Name),
		Kind:                toolManifestKindMetaAction,
		Tool:                action.ToolName,
		Action:              action.Name,
		Domain:              action.Domain,
		Title:               actionTitle(action),
		Description:         actionDescription(action, resolve),
		Destructive:         action.Route.Destructive,
		ReadOnly:            action.ReadOnly,
		RequiredParams:      manifestRequiredParams(action.Route.InputSchema),
		RequiredParamsAnyOf: manifestAlternativeRequiredParams(action.Route.InputSchema),
	}
	if primary, ok := aliases[string(action.ID)]; ok {
		entry.AliasOf = metaManifestID(primary.ToolName, primary.Name)
	}
	snapshot.addMetaEntry(entry, routes)
}

// aliasCanonicalActionIDs makes gitlab://tools/{canonical action ID} resolve
// on a surface whose entries are keyed by something else.
//
// The entry keys are per surface on purpose: a listing should name the call
// the reader can make, which is the tool on the individual surface and the
// tool plus an action argument on meta. The canonical ID is what everything
// AROUND the surface names, though. It is what a card hint and an error hint
// spell, what a cross-link carries, what the documentation teaches, and what
// gitlab_find_action publishes, so a model holding one and serving any surface
// has to be able to look it up.
//
// Without this it could not, and the measurement is the argument: the meta
// entry for project.get is keyed gitlab_project.get and the individual one
// gitlab_project_get, so neither answers gitlab://tools/project.get. Deriving
// the tool name from the ID is not an alternative, because the individual name
// is declared rather than computed: 338 of 1084 actions match
// gitlab_<domain>_<action> and the other 746 do not (access.deploy_key_add is
// gitlab_deploy_key_add). Meta can be derived, all 1084 of them, and is
// aliased anyway so the two surfaces answer the same question the same way.
//
// Only the detail lookup gains a key. The entry the alias resolves to is the
// surface's own, so what comes back names the call this session can really
// make, which is the translation a model asking the question needs.
//
// An ID already filed is left as it is, which is what the dynamic surface
// meets on every run: its details are keyed by the canonical ID to begin with,
// so this walks its catalog and changes nothing. That is why the caller asks
// on every surface rather than on the two that need it, and why this guard is
// load-bearing rather than defensive.
func (snapshot *toolSurfaceSnapshot) aliasCanonicalActionIDs(catalog *actioncatalog.Catalog) {
	if catalog == nil {
		return
	}
	for _, action := range catalog.Actions() {
		id := string(action.ID)
		if _, taken := snapshot.details[id]; taken {
			continue
		}
		detail, served := snapshot.details[snapshot.surfaceKeyFor(action)]
		if !served {
			continue
		}
		snapshot.details[id] = detail
	}
}

// surfaceKeyFor is the key this surface filed one action's detail under.
func (snapshot *toolSurfaceSnapshot) surfaceKeyFor(action actioncatalog.Action) string {
	if snapshot.manifest.Surface == toolSurfaceMeta {
		return metaManifestID(action.ToolName, action.Name)
	}
	return strings.TrimSpace(action.IndividualTool.Name)
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
		ID:                  tool.Name,
		Kind:                kind,
		Tool:                tool.Name,
		Title:               tool.Title,
		Description:         tool.Description,
		DetailURI:           toolManifestDetailURI(tool.Name),
		Destructive:         tool.Destructive,
		ReadOnly:            tool.ReadOnly,
		RequiredParams:      requiredParamsFromInputSchema(tool.InputSchema),
		RequiredParamsAnyOf: alternativeRequiredParamsFromInputSchema(tool.InputSchema),
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

func alternativeRequiredParamsFromInputSchema(inputSchema any) [][]ToolSurfaceRequiredParam {
	schema, ok := inputSchema.(map[string]any)
	if !ok {
		return nil
	}
	return manifestAlternativeRequiredParams(schema)
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
// Resolution is scoped to the routes the surface actually emits: a
// restricted meta surface (excluded tools, read-only filtering) has no
// entry for a hidden action, and rewriting a reference to an ID with no
// entry would reintroduce exactly the non-invocable reference this
// projection removes.
func metaSeeAlso(index map[string]actioncatalog.Action, routes map[string]toolutil.ActionMap) seeAlsoResolver {
	return func(name string) (string, bool) {
		action, ok := index[name]
		if !ok || !metaRouteVisible(routes, action.ToolName, action.Name) {
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

// appendDynamicRequiredParamNames appends each non-empty string in raw
// to names. raw is expected to be either a []any or []string (the two
// shapes the JSON parser can return for a JSON array); any other type
// is ignored.
func appendDynamicRequiredParamNames(names []string, raw any) []string {
	switch values := raw.(type) {
	case []any:
		for _, value := range values {
			if name, ok := value.(string); ok && name != "" {
				names = append(names, name)
			}
		}
	case []string:
		names = append(names, values...)
	}
	return names
}

// dedupeDynamicStrings returns a slice of strings with consecutive
// duplicates and empty values removed. The input is expected to be
// pre-sorted by the caller (the only deduplication guarantee provided).
func dedupeDynamicStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := values[:0]
	// last starts empty and every empty value is skipped above it, so the
	// first value can never equal last: an "is this the first iteration"
	// guard here would be a branch no input can take.
	var last string
	for _, value := range values {
		if value == "" || value == last {
			continue
		}
		out = append(out, value)
		last = value
	}
	return out
}

// dynamicActionSchema returns the JSON Schema (as a generic map) for
// the params object of one dynamic action. The route's own schema is
// never edited: the enriched form is derived from it. When the action
// has no captured InputSchema a permissive fallback object schema is
// returned.
func dynamicActionSchema(action actioncatalog.Action) map[string]any {
	route := action.Route
	if route.InputSchema == nil {
		schema := map[string]any{
			"type":                 "object",
			"description":          "This dynamic action has no captured parameter schema. Send an empty params object {} unless the action description says otherwise.",
			"additionalProperties": true,
		}
		return enrichDynamicSchema(schema, action)
	}
	// Derived rather than copied and edited: for an action of a shared
	// catalog the result is built once for the process (see
	// [toolutil.DeriveSchema]), and the manifest of every server carries the
	// same map. The guidance and the destructive flag are in the transform
	// name because they are in the result.
	transform := "manifest-dynamic|destructive=" + strconv.FormatBool(route.Destructive) + "|guidance=" + toolutil.ParameterGuidanceIdentity(route.ParameterGuidance)
	derived := toolutil.DeriveSchema(route.InputSchema, transform, func() any {
		return enrichDynamicSchema(toolutil.CloneSchemaMap(route.InputSchema), action)
	})
	schema, _ := derived.(map[string]any)
	return schema
}

// cloneMetaSchemaRoutes creates a snapshot of the route maps so resource
// handlers do not observe later registration changes from other server
// builds; the schemas inside are shared and frozen. It is a thin wrapper
// around [toolutil.CloneMetaSchemaRoutes] that exists for testability.
func cloneMetaSchemaRoutes(routes map[string]toolutil.ActionMap) map[string]toolutil.ActionMap {
	return toolutil.CloneMetaSchemaRoutes(routes)
}

// lookupMetaActionSchema returns the per-action params schema for the
// given tool/action pair. It returns false when the tool or action is
// unknown. When the route exists but has no captured InputSchema, a
// permissive fallback object schema (with "additionalProperties: true"
// and a guidance description) is returned along with true, so clients
// always get a usable JSON Schema.
func lookupMetaActionSchema(routes map[string]toolutil.ActionMap, tool, action string) (map[string]any, bool) {
	return toolutil.LookupMetaActionSchema(routes, tool, action)
}

// enrichDynamicSchema adds x_parameter_guidance and (for destructive
// actions) x_destructive / x_confirmation fields to schema in place.
// The map is returned for fluent use.
func enrichDynamicSchema(schema map[string]any, action actioncatalog.Action) map[string]any {
	if guidance := dynamicParameterGuidance(action); len(guidance) > 0 {
		schema["x_parameter_guidance"] = guidance
	}
	if action.Route.Destructive {
		schema["x_destructive"] = true
		schema["x_confirmation"] = map[string]any{
			"location":    "gitlab_execute_action.confirm",
			"description": "Set top-level confirm=true on gitlab_execute_action after explicit user approval; do not put confirm inside params.",
		}
	}
	return schema
}

// dynamicParameterGuidance converts the route's
// [toolutil.ParameterGuidance] map into the JSON shape embedded under
// the schema's x_parameter_guidance key. Returns nil when there is no
// guidance to embed.
func dynamicParameterGuidance(action actioncatalog.Action) map[string]any {
	if len(action.Route.ParameterGuidance) == 0 {
		return nil
	}
	guidance := make(map[string]any, len(action.Route.ParameterGuidance))
	for name, item := range action.Route.ParameterGuidance {
		entry := dynamicParameterGuidanceEntry(item)
		if len(entry) > 0 {
			guidance[name] = entry
		}
	}
	return guidance
}

// dynamicParameterGuidanceEntry renders a single guidance item as a
// JSON map. Only the populated fields of item are included so the
// output stays compact.
func dynamicParameterGuidanceEntry(item toolutil.ParameterGuidance) map[string]any {
	entry := make(map[string]any, 4)
	if item.SemanticRole != "" {
		entry["semantic_role"] = item.SemanticRole
	}
	if item.ValueSource != "" {
		entry["value_source"] = item.ValueSource
	}
	if len(item.CommonConfusions) > 0 {
		entry["common_confusions"] = append([]string(nil), item.CommonConfusions...)
	}
	if item.ExampleBinding != "" {
		entry["example_binding"] = item.ExampleBinding
	}
	return entry
}
