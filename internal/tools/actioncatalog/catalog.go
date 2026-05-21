package actioncatalog

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	errCatalogNil       = "action catalog is nil"
	errToolNameRequired = "tool name is required"
)

// ActionID is the stable dynamic identifier for one GitLab action.
type ActionID string

// Action describes one executable GitLab action in the canonical catalog.
type Action struct {
	ID                     ActionID
	ToolName               string
	Domain                 string
	Name                   string
	Route                  toolutil.ActionRoute
	SchemaURI              string
	Aliases                []string
	Tags                   []string
	Usage                  string
	RelatedActions         []string
	Compatibility          toolutil.CompatibilityPolicy
	ReadOnly               bool
	Edition                string
	GitLabDotComOnly       bool
	OwnerPackage           string
	IndividualTool         toolutil.IndividualToolSpec
	ContentKind            string
	NotFoundPolicy         string
	EmbeddedResourcePolicy string
	RichResultPolicy       string
	SchemaValidationNotes  []string
	RuntimeValidationNotes []string
	SpecBacked             bool
	Destructive            bool
	Idempotent             bool
	OpenWorld              bool
}

// GroupOptions contains metadata for creating a catalog group.
type GroupOptions struct {
	ToolName               string
	Title                  string
	Description            string
	Icons                  []mcp.Icon
	ReadOnly               bool
	FormatResult           toolutil.FormatResultFunc
	BaseDomain             string
	EnterpriseOnly         bool
	GitLabDotComOnly       bool
	CapabilityRequirements []string
	OwnerPackage           string
	SurfaceKind            SurfaceKind
}

// FilterOptions describes catalog-level filtering inputs.
type FilterOptions struct {
	ExcludeTools     []string
	ReadOnlyOnly     bool
	AllowedToolNames []string
}

// Group describes all actions exposed through one logical meta-tool group.
type Group struct {
	ToolName               string
	Title                  string
	Description            string
	Icons                  []mcp.Icon
	ReadOnly               bool
	FormatResult           toolutil.FormatResultFunc
	BaseDomain             string
	EnterpriseOnly         bool
	GitLabDotComOnly       bool
	CapabilityRequirements []string
	OwnerPackage           string
	SurfaceKind            SurfaceKind
	Actions                map[string]Action
	ActionOrder            []string
}

// Catalog stores deterministic groups and action lookup indexes. A Catalog is
// intended to be mutated during single-threaded initialization and then shared
// read-only; concurrent mutation is not supported.
type Catalog struct {
	groups  map[string]Group
	actions map[ActionID]Action
}

// NewCatalog creates an empty action catalog.
func NewCatalog() *Catalog {
	return &Catalog{
		groups:  make(map[string]Group),
		actions: make(map[ActionID]Action),
	}
}

// FromActionMaps converts legacy route maps into a canonical catalog.
func FromActionMaps(routes map[string]toolutil.ActionMap) *Catalog {
	catalog, err := FromActionMapsWithError(routes)
	if err != nil {
		panic(fmt.Errorf("FromActionMaps: %w", err))
	}
	return catalog
}

// FromActionMapsWithError converts legacy route maps into a canonical catalog
// and reports invalid groups instead of panicking.
func FromActionMapsWithError(routes map[string]toolutil.ActionMap) (*Catalog, error) {
	catalog := NewCatalog()
	var errs []error
	toolNames := make([]string, 0, len(routes))
	for toolName := range routes {
		toolNames = append(toolNames, toolName)
	}
	sort.Strings(toolNames)
	for _, toolName := range toolNames {
		actions := routes[toolName]
		group := NewGroup(GroupOptions{ToolName: toolName})
		actionNames := make([]string, 0, len(actions))
		for actionName := range actions {
			actionNames = append(actionNames, actionName)
		}
		sort.Strings(actionNames)
		for _, actionName := range actionNames {
			group.SetAction(Action{Name: actionName, Route: actions[actionName]})
		}
		if err := catalog.AddGroup(group); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", toolName, err))
		}
	}
	return catalog, errors.Join(errs...)
}

// ToActionMaps returns legacy route maps for compatibility with existing
// schema resources, audits, and registration paths.
func ToActionMaps(catalog *Catalog) map[string]toolutil.ActionMap {
	if catalog == nil {
		return nil
	}
	return catalog.ActionMaps()
}

// NewGroup creates an action group with initialized maps.
func NewGroup(opts GroupOptions) Group {
	surfaceKind := opts.SurfaceKind
	if surfaceKind == "" {
		surfaceKind = SurfaceKindMetaGroup
	}
	return Group{
		ToolName:               strings.TrimSpace(opts.ToolName),
		Title:                  strings.TrimSpace(opts.Title),
		Description:            opts.Description,
		Icons:                  cloneIcons(opts.Icons),
		ReadOnly:               opts.ReadOnly,
		FormatResult:           opts.FormatResult,
		BaseDomain:             strings.TrimSpace(opts.BaseDomain),
		EnterpriseOnly:         opts.EnterpriseOnly,
		GitLabDotComOnly:       opts.GitLabDotComOnly,
		CapabilityRequirements: cloneStrings(opts.CapabilityRequirements),
		OwnerPackage:           strings.TrimSpace(opts.OwnerPackage),
		SurfaceKind:            surfaceKind,
		Actions:                make(map[string]Action),
	}
}

// SetAction inserts or replaces an action in the group.
func (g *Group) SetAction(action Action) {
	if g.Actions == nil {
		g.Actions = make(map[string]Action)
	}
	actionName := strings.TrimSpace(action.Name)
	if actionName == "" {
		return
	}
	if !slices.Contains(g.ActionOrder, actionName) {
		g.ActionOrder = append(g.ActionOrder, actionName)
	}
	g.Actions[actionName] = action
}

// ActionsInOrder returns group actions in deterministic action-name order.
func (g *Group) ActionsInOrder() []Action {
	order := append([]string(nil), g.ActionOrder...)
	if len(order) == 0 {
		order = make([]string, 0, len(g.Actions))
		for actionName := range g.Actions {
			order = append(order, actionName)
		}
	}
	sort.Strings(order)
	actions := make([]Action, 0, len(order))
	seen := make(map[string]struct{}, len(order))
	for _, actionName := range order {
		if _, ok := seen[actionName]; ok {
			continue
		}
		seen[actionName] = struct{}{}
		action, ok := g.Actions[actionName]
		if !ok {
			continue
		}
		actions = append(actions, action)
	}
	return actions
}

// ActionMap returns a legacy route map for this group.
func (g *Group) ActionMap() toolutil.ActionMap {
	routes := make(toolutil.ActionMap, len(g.Actions))
	for _, action := range g.ActionsInOrder() {
		routes[action.Name] = action.Route
	}
	return toolutil.CloneMetaSchemaRoutes(map[string]toolutil.ActionMap{g.ToolName: routes})[g.ToolName]
}

// AddGroup adds a complete group to the catalog.
func (c *Catalog) AddGroup(group Group) error {
	if c == nil {
		return errors.New(errCatalogNil)
	}
	if c.groups == nil {
		c.groups = make(map[string]Group)
	}
	if c.actions == nil {
		c.actions = make(map[ActionID]Action)
	}
	normalized, err := normalizeGroup(group)
	if err != nil {
		return err
	}
	if _, exists := c.groups[normalized.ToolName]; exists {
		return fmt.Errorf("duplicate action group %q", normalized.ToolName)
	}
	seenIDs := make(map[ActionID]struct{}, len(normalized.Actions))
	for _, action := range normalized.ActionsInOrder() {
		if _, exists := seenIDs[action.ID]; exists {
			return fmt.Errorf("duplicate action id %q", action.ID)
		}
		seenIDs[action.ID] = struct{}{}
		if _, exists := c.actions[action.ID]; exists {
			return fmt.Errorf("duplicate action id %q", action.ID)
		}
	}
	c.groups[normalized.ToolName] = normalized
	for _, action := range normalized.ActionsInOrder() {
		c.actions[action.ID] = action
	}
	return nil
}

// AddAction adds one action to an existing or newly-created group. When the
// group does not exist, callers may provide GroupOptions so the synthesized
// group carries the same metadata as a normal catalog group.
func (c *Catalog) AddAction(toolName string, action Action, groupOptions ...GroupOptions) error {
	if c == nil {
		return errors.New(errCatalogNil)
	}
	if len(groupOptions) > 1 {
		return errors.New("at most one group options value is supported")
	}
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return errors.New(errToolNameRequired)
	}
	next := c.Clone()
	group, ok := next.groups[toolName]
	if !ok {
		var err error
		group, err = newAddActionGroup(toolName, groupOptions)
		if err != nil {
			return err
		}
	}
	group.SetAction(action)
	if ok {
		delete(next.groups, toolName)
		for id, existing := range next.actions {
			if existing.ToolName == toolName {
				delete(next.actions, id)
			}
		}
	}
	if err := next.AddGroup(group); err != nil {
		return err
	}
	c.groups = next.groups
	c.actions = next.actions
	return nil
}

func newAddActionGroup(toolName string, groupOptions []GroupOptions) (Group, error) {
	opts := GroupOptions{ToolName: toolName}
	if len(groupOptions) == 0 {
		return NewGroup(opts), nil
	}
	opts = groupOptions[0]
	opts.ToolName = strings.TrimSpace(opts.ToolName)
	if opts.ToolName == "" {
		opts.ToolName = toolName
	}
	if opts.ToolName != toolName {
		return Group{}, fmt.Errorf("group options tool name %q does not match %q", opts.ToolName, toolName)
	}
	return NewGroup(opts), nil
}

// Group returns a defensive copy of one group by tool name.
func (c *Catalog) Group(toolName string) (Group, bool) {
	if c == nil {
		return Group{}, false
	}
	group, ok := c.groups[toolName]
	if !ok {
		return Group{}, false
	}
	return cloneGroup(group), true
}

// Action returns a defensive copy of one action by canonical ID.
func (c *Catalog) Action(id ActionID) (Action, bool) {
	if c == nil {
		return Action{}, false
	}
	action, ok := c.actions[id]
	if !ok {
		return Action{}, false
	}
	return cloneAction(action), true
}

// Groups returns all groups sorted by tool name.
func (c *Catalog) Groups() []Group {
	if c == nil {
		return nil
	}
	names := make([]string, 0, len(c.groups))
	for name := range c.groups {
		names = append(names, name)
	}
	sort.Strings(names)
	groups := make([]Group, 0, len(names))
	for _, name := range names {
		groups = append(groups, cloneGroup(c.groups[name]))
	}
	return groups
}

// Actions returns all actions sorted by canonical ID.
func (c *Catalog) Actions() []Action {
	if c == nil {
		return nil
	}
	ids := make([]string, 0, len(c.actions))
	for id := range c.actions {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	actions := make([]Action, 0, len(ids))
	for _, id := range ids {
		actions = append(actions, cloneAction(c.actions[ActionID(id)]))
	}
	return actions
}

// ActionMaps returns a defensive legacy route snapshot keyed by tool and action.
func (c *Catalog) ActionMaps() map[string]toolutil.ActionMap {
	if c == nil {
		return nil
	}
	routes := make(map[string]toolutil.ActionMap, len(c.groups))
	for _, group := range c.Groups() {
		routes[group.ToolName] = group.ActionMap()
	}
	return toolutil.CloneMetaSchemaRoutes(routes)
}

// CountGroups returns the number of groups in the catalog.
func (c *Catalog) CountGroups() int {
	if c == nil {
		return 0
	}
	return len(c.groups)
}

// CountActions returns the number of actions in the catalog.
func (c *Catalog) CountActions() int {
	if c == nil {
		return 0
	}
	return len(c.actions)
}

// Clone returns a defensive deep copy of the catalog.
func (c *Catalog) Clone() *Catalog {
	if c == nil {
		return nil
	}
	clone := NewCatalog()
	for _, group := range c.Groups() {
		mustAddCatalogGroup(clone, group, "clone catalog")
	}
	return clone
}

func mustAddCatalogGroup(catalog *Catalog, group Group, operation string) {
	// AddGroup should not fail while cloning/filtering already-validated groups;
	// panic here so future catalog invariant drift is caught immediately.
	if err := catalog.AddGroup(group); err != nil {
		panic(fmt.Sprintf("%s: %v", operation, err))
	}
}

// Validate verifies that the catalog has a consistent, executable action index.
func (c *Catalog) Validate() error {
	if c == nil {
		return errors.New(errCatalogNil)
	}
	seenAliases := make(map[string]ActionID)
	for _, group := range c.Groups() {
		if err := validateCatalogGroup(group, seenAliases); err != nil {
			return err
		}
	}
	return nil
}

func validateCatalogGroup(group Group, seenAliases map[string]ActionID) error {
	if strings.TrimSpace(group.ToolName) == "" {
		return errors.New(errToolNameRequired)
	}
	for _, action := range group.ActionsInOrder() {
		if err := validateCatalogAction(group, action, seenAliases); err != nil {
			return err
		}
	}
	return nil
}

func validateCatalogAction(group Group, action Action, seenAliases map[string]ActionID) error {
	if strings.TrimSpace(action.Name) == "" {
		return fmt.Errorf("action name is required for tool %q", group.ToolName)
	}
	if action.Route.Handler == nil {
		return fmt.Errorf("action %q has nil handler", action.ID)
	}
	if action.Route.InputSchema == nil {
		return fmt.Errorf("action %q has nil input schema", action.ID)
	}
	if tool, actionName := toolutil.ParseMetaSchemaURI(action.SchemaURI); tool != action.ToolName || actionName != action.Name {
		return fmt.Errorf("action %q has malformed schema URI %q", action.ID, action.SchemaURI)
	}
	for _, alias := range action.Aliases {
		if err := recordCatalogAlias(seenAliases, action.ID, alias); err != nil {
			return err
		}
	}
	return nil
}

func recordCatalogAlias(seenAliases map[string]ActionID, actionID ActionID, alias string) error {
	alias = strings.TrimSpace(strings.ToLower(alias))
	if alias == "" {
		return nil
	}
	if existing, ok := seenAliases[alias]; ok && existing != actionID {
		return fmt.Errorf("alias %q maps to both %q and %q", alias, existing, actionID)
	}
	seenAliases[alias] = actionID
	return nil
}

// FilterExcludedTools returns a cloned catalog without excluded tool groups.
func (c *Catalog) FilterExcludedTools(excludeTools []string) *Catalog {
	if c == nil {
		return nil
	}
	if len(excludeTools) == 0 {
		return c.Clone()
	}
	excluded := make(map[string]struct{}, len(excludeTools))
	for _, toolName := range excludeTools {
		excluded[toolName] = struct{}{}
	}
	filtered := NewCatalog()
	for _, group := range c.Groups() {
		if _, ok := excluded[group.ToolName]; ok {
			continue
		}
		mustAddCatalogGroup(filtered, group, "filter excluded tools")
	}
	return filtered
}

// FilterReadOnlyGroups returns a cloned catalog containing only read-only groups.
func (c *Catalog) FilterReadOnlyGroups() *Catalog {
	if c == nil {
		return nil
	}
	filtered := NewCatalog()
	for _, group := range c.Groups() {
		if !group.ReadOnly {
			continue
		}
		mustAddCatalogGroup(filtered, group, "filter read-only groups")
	}
	return filtered
}

// FilterAllowedToolNames returns a cloned catalog with only explicitly allowed tools.
func (c *Catalog) FilterAllowedToolNames(toolNames []string) *Catalog {
	if c == nil {
		return nil
	}
	if len(toolNames) == 0 {
		return c.Clone()
	}
	allowed := make(map[string]struct{}, len(toolNames))
	for _, toolName := range toolNames {
		allowed[toolName] = struct{}{}
	}
	filtered := NewCatalog()
	for _, group := range c.Groups() {
		if _, ok := allowed[group.ToolName]; !ok {
			continue
		}
		mustAddCatalogGroup(filtered, group, "filter allowed tool names")
	}
	return filtered
}

// Filter applies all catalog-level filters in a deterministic order.
func (c *Catalog) Filter(opts FilterOptions) *Catalog {
	if c == nil {
		return nil
	}
	filtered := c.FilterExcludedTools(opts.ExcludeTools)
	if opts.ReadOnlyOnly {
		filtered = filtered.FilterReadOnlyGroups()
	}
	if len(opts.AllowedToolNames) > 0 {
		filtered = filtered.FilterAllowedToolNames(opts.AllowedToolNames)
	}
	return filtered
}

// DomainFromToolName returns the canonical dynamic domain for a meta-tool name.
func DomainFromToolName(toolName string) string {
	return strings.TrimPrefix(toolName, "gitlab_")
}

func normalizeGroup(group Group) (Group, error) {
	toolName := strings.TrimSpace(group.ToolName)
	if toolName == "" {
		return Group{}, errors.New(errToolNameRequired)
	}
	normalized := NewGroup(GroupOptions{
		ToolName:               toolName,
		Title:                  group.Title,
		Description:            group.Description,
		Icons:                  group.Icons,
		ReadOnly:               group.ReadOnly,
		FormatResult:           group.FormatResult,
		BaseDomain:             group.BaseDomain,
		EnterpriseOnly:         group.EnterpriseOnly,
		GitLabDotComOnly:       group.GitLabDotComOnly,
		CapabilityRequirements: group.CapabilityRequirements,
		OwnerPackage:           group.OwnerPackage,
		SurfaceKind:            group.SurfaceKind,
	})
	for _, action := range group.ActionsInOrder() {
		normalizedAction, err := normalizeAction(toolName, normalized.BaseDomain, action)
		if err != nil {
			return Group{}, err
		}
		normalized.SetAction(normalizedAction)
	}
	return normalized, nil
}

func normalizeAction(toolName, baseDomain string, action Action) (Action, error) {
	action.Name = strings.TrimSpace(action.Name)
	if action.Name == "" {
		return Action{}, fmt.Errorf("action name is required for tool %q", toolName)
	}
	action.ToolName = strings.TrimSpace(action.ToolName)
	if action.ToolName == "" {
		action.ToolName = toolName
	}
	if action.ToolName != toolName {
		return Action{}, fmt.Errorf("action %q belongs to tool %q, want %q", action.Name, action.ToolName, toolName)
	}
	action.Domain = strings.TrimSpace(action.Domain)
	if action.Domain == "" {
		action.Domain = strings.TrimSpace(baseDomain)
	}
	if action.Domain == "" {
		action.Domain = DomainFromToolName(action.ToolName)
	}
	expectedID := ActionID(action.Domain + "." + action.Name)
	if action.ID == "" {
		action.ID = expectedID
	} else if action.ID != expectedID {
		return Action{}, fmt.Errorf("action %q has id %q, want %q", action.Name, action.ID, expectedID)
	}
	if action.SchemaURI == "" {
		action.SchemaURI = toolutil.MetaSchemaURI(action.ToolName, action.Name)
	}
	if len(action.Aliases) == 0 {
		action.Aliases = action.Route.Aliases
	}
	if len(action.Tags) == 0 {
		action.Tags = action.Route.Tags
	}
	if action.Usage == "" {
		action.Usage = action.Route.Usage
	}
	if len(action.RelatedActions) == 0 {
		action.RelatedActions = action.Route.RelatedActions
	}
	action.Aliases = cloneStrings(action.Aliases)
	action.Tags = cloneStrings(action.Tags)
	action.RelatedActions = cloneStrings(action.RelatedActions)
	action.Compatibility = toolutil.CloneCompatibilityPolicy(action.Compatibility)
	action.SchemaValidationNotes = cloneStrings(action.SchemaValidationNotes)
	action.RuntimeValidationNotes = cloneStrings(action.RuntimeValidationNotes)
	return cloneAction(action), nil
}

func cloneGroup(group Group) Group {
	cloned := Group{
		ToolName:               group.ToolName,
		Title:                  group.Title,
		Description:            group.Description,
		Icons:                  cloneIcons(group.Icons),
		ReadOnly:               group.ReadOnly,
		FormatResult:           group.FormatResult,
		BaseDomain:             group.BaseDomain,
		EnterpriseOnly:         group.EnterpriseOnly,
		GitLabDotComOnly:       group.GitLabDotComOnly,
		CapabilityRequirements: cloneStrings(group.CapabilityRequirements),
		OwnerPackage:           group.OwnerPackage,
		SurfaceKind:            group.SurfaceKind,
		Actions:                make(map[string]Action, len(group.Actions)),
		ActionOrder:            cloneStrings(group.ActionOrder),
	}
	for name, action := range group.Actions {
		cloned.Actions[name] = cloneAction(action)
	}
	return cloned
}

func cloneAction(action Action) Action {
	route := action.Route
	if route.InputSchema != nil || route.OutputSchema != nil {
		routes := toolutil.CloneMetaSchemaRoutes(map[string]toolutil.ActionMap{action.ToolName: {action.Name: route}})
		route = routes[action.ToolName][action.Name]
	}
	action.Route = route
	action.IndividualTool = toolutil.CloneIndividualToolSpec(action.IndividualTool)
	action.Compatibility = toolutil.CloneCompatibilityPolicy(action.Compatibility)
	action.Aliases = cloneStrings(action.Aliases)
	action.Tags = cloneStrings(action.Tags)
	action.RelatedActions = cloneStrings(action.RelatedActions)
	action.SchemaValidationNotes = cloneStrings(action.SchemaValidationNotes)
	action.RuntimeValidationNotes = cloneStrings(action.RuntimeValidationNotes)
	return action
}

func cloneIcons(icons []mcp.Icon) []mcp.Icon {
	return append([]mcp.Icon(nil), icons...)
}

func cloneStrings(values []string) []string {
	return append([]string(nil), values...)
}
