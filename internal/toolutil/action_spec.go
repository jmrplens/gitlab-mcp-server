package toolutil

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// ActionSpec is the canonical metadata contract for one GitLab action.
type ActionSpec struct {
	Name                   string
	Route                  ActionRoute
	Aliases                []string
	Tags                   []string
	Usage                  string
	RelatedActions         []string
	Compatibility          CompatibilityPolicy
	ParameterGuidance      map[string]ParameterGuidance
	InputSchemaOverrides   []InputSchemaOverride
	ReadOnly               bool
	Destructive            bool
	Idempotent             bool
	OpenWorld              bool
	Edition                string
	GitLabDotComOnly       bool
	OwnerPackage           string
	IndividualTool         IndividualToolSpec
	ContentKind            string
	NotFoundPolicy         string
	EmbeddedResourcePolicy string
	RichResultPolicy       string
	SchemaValidationNotes  []string
	RuntimeValidationNotes []string
}

// ActionAliasSpec describes a compatibility alias that resolves to a canonical
// action owned by an ActionSpec.
type ActionAliasSpec struct {
	Alias          string
	Target         string
	Source         string
	Searchable     bool
	Deprecated     bool
	RemovalVersion string
	Reason         string
}

// ParameterAliasSpec describes a compatibility alias for one action parameter.
type ParameterAliasSpec struct {
	Alias          string
	Target         string
	Source         string
	Searchable     bool
	Deprecated     bool
	RemovalVersion string
	Reason         string
}

// CompatibilityPolicy carries compatibility aliases and their ownership policy.
type CompatibilityPolicy struct {
	ActionAliases    []ActionAliasSpec
	ParameterAliases []ParameterAliasSpec
}

// InputSchemaOverride describes a deterministic JSON Schema patch for an
// action input schema. PropertyPath is a dot-separated input property path; an
// empty path applies Values at the schema root. Array properties automatically
// traverse through their items schema for nested paths.
type InputSchemaOverride struct {
	PropertyPath string
	Values       map[string]any
}

// IndividualToolSpec carries compatibility metadata for the individual-tool surface.
type IndividualToolSpec struct {
	Name                string
	Title               string
	Description         string
	AnnotationOverrides IndividualToolAnnotationOverrides
}

// IndividualToolAnnotationOverrides carries compatibility overrides for
// historical individual-tool annotations that intentionally differ from the
// canonical action semantics.
type IndividualToolAnnotationOverrides struct {
	ReadOnly    *bool
	Destructive *bool
	Idempotent  *bool
	OpenWorld   *bool
}

const (
	ActionSpecContentList      = "list"
	ActionSpecContentDetail    = "detail"
	ActionSpecContentMutate    = "mutate"
	ActionSpecContentAssistant = "assistant"
	ActionSpecContentImage     = "image"

	ActionSpecNotFoundNone      = "none"
	ActionSpecNotFoundResult    = "not_found_result"
	ActionSpecNotFoundPropagate = "propagate_error"

	ActionSpecEmbeddedNone     = "none"
	ActionSpecEmbeddedOptional = "optional"
	ActionSpecEmbeddedAlways   = "always"

	ActionSpecRichStandard     = "standard"
	ActionSpecRichImage        = "image"
	ActionSpecRichResourceLink = "resource_link"
	ActionSpecRichMixed        = "mixed"
)

// ActionSpecOptions contains optional metadata for NewActionSpec.
type ActionSpecOptions struct {
	Aliases                []string
	Tags                   []string
	Usage                  string
	RelatedActions         []string
	Compatibility          CompatibilityPolicy
	ParameterGuidance      map[string]ParameterGuidance
	InputSchemaOverrides   []InputSchemaOverride
	ReadOnly               bool
	Destructive            bool
	Idempotent             bool
	OpenWorld              bool
	Edition                string
	GitLabDotComOnly       bool
	OwnerPackage           string
	IndividualTool         IndividualToolSpec
	ContentKind            string
	NotFoundPolicy         string
	EmbeddedResourcePolicy string
	RichResultPolicy       string
	SchemaValidationNotes  []string
	RuntimeValidationNotes []string
}

// NewActionSpec creates a defensive canonical action specification.
func NewActionSpec(name string, route ActionRoute, opts ActionSpecOptions) ActionSpec {
	route = cloneActionRoute(route)
	if opts.Destructive {
		route.Destructive = true
	}
	inputSchemaOverrides := cloneInputSchemaOverrides(opts.InputSchemaOverrides)
	applyInputSchemaOverrides(route.InputSchema, inputSchemaOverrides)
	return ActionSpec{
		Name:                   strings.TrimSpace(name),
		Route:                  route,
		Aliases:                mergeActionSpecStrings(route.Aliases, opts.Aliases),
		Tags:                   mergeActionSpecStrings(route.Tags, opts.Tags),
		Usage:                  firstNonEmptyString(opts.Usage, route.Usage),
		RelatedActions:         mergeActionSpecStrings(route.RelatedActions, opts.RelatedActions),
		Compatibility:          CloneCompatibilityPolicy(opts.Compatibility),
		ParameterGuidance:      cloneParameterGuidanceMap(opts.ParameterGuidance),
		InputSchemaOverrides:   inputSchemaOverrides,
		ReadOnly:               opts.ReadOnly,
		Destructive:            route.Destructive,
		Idempotent:             opts.Idempotent,
		OpenWorld:              opts.OpenWorld,
		Edition:                strings.TrimSpace(opts.Edition),
		GitLabDotComOnly:       opts.GitLabDotComOnly,
		OwnerPackage:           strings.TrimSpace(opts.OwnerPackage),
		IndividualTool:         CloneIndividualToolSpec(opts.IndividualTool),
		ContentKind:            strings.TrimSpace(opts.ContentKind),
		NotFoundPolicy:         strings.TrimSpace(opts.NotFoundPolicy),
		EmbeddedResourcePolicy: strings.TrimSpace(opts.EmbeddedResourcePolicy),
		RichResultPolicy:       strings.TrimSpace(opts.RichResultPolicy),
		SchemaValidationNotes:  normalizeActionSpecNotes(opts.SchemaValidationNotes),
		RuntimeValidationNotes: normalizeActionSpecNotes(opts.RuntimeValidationNotes),
	}
}

// NewReadActionSpec creates a read-only, idempotent action specification.
func NewReadActionSpec(name string, route ActionRoute, opts ActionSpecOptions) ActionSpec {
	opts.ReadOnly = true
	opts.Idempotent = true
	return NewActionSpec(name, route, opts)
}

// NewCreateActionSpec creates a mutating, non-idempotent action specification.
func NewCreateActionSpec(name string, route ActionRoute, opts ActionSpecOptions) ActionSpec {
	return NewActionSpec(name, route, opts)
}

// NewUpdateActionSpec creates a mutating, idempotent action specification.
func NewUpdateActionSpec(name string, route ActionRoute, opts ActionSpecOptions) ActionSpec {
	opts.Idempotent = true
	return NewActionSpec(name, route, opts)
}

// NewDeleteActionSpec creates a destructive, idempotent action specification.
func NewDeleteActionSpec(name string, route ActionRoute, opts ActionSpecOptions) ActionSpec {
	opts.Destructive = true
	opts.Idempotent = true
	return NewActionSpec(name, route, opts)
}

// CloneActionSpec returns a defensive copy of spec and all mutable metadata it owns.
func CloneActionSpec(spec ActionSpec) ActionSpec {
	return NewActionSpec(spec.Name, spec.Route, actionSpecOptionsFromSpec(spec))
}

// CloneActionSpecs returns defensive copies of specs in their original order.
func CloneActionSpecs(specs []ActionSpec) []ActionSpec {
	if len(specs) == 0 {
		return nil
	}
	out := make([]ActionSpec, 0, len(specs))
	for _, spec := range specs {
		out = append(out, CloneActionSpec(spec))
	}
	return out
}

func actionSpecOptionsFromSpec(spec ActionSpec) ActionSpecOptions {
	return ActionSpecOptions{
		Aliases:                spec.Aliases,
		Tags:                   spec.Tags,
		Usage:                  spec.Usage,
		RelatedActions:         spec.RelatedActions,
		Compatibility:          spec.Compatibility,
		ParameterGuidance:      spec.ParameterGuidance,
		InputSchemaOverrides:   spec.InputSchemaOverrides,
		ReadOnly:               spec.ReadOnly,
		Destructive:            spec.Destructive,
		Idempotent:             spec.Idempotent,
		OpenWorld:              spec.OpenWorld,
		Edition:                spec.Edition,
		GitLabDotComOnly:       spec.GitLabDotComOnly,
		OwnerPackage:           spec.OwnerPackage,
		IndividualTool:         spec.IndividualTool,
		ContentKind:            spec.ContentKind,
		NotFoundPolicy:         spec.NotFoundPolicy,
		EmbeddedResourcePolicy: spec.EmbeddedResourcePolicy,
		RichResultPolicy:       spec.RichResultPolicy,
		SchemaValidationNotes:  spec.SchemaValidationNotes,
		RuntimeValidationNotes: spec.RuntimeValidationNotes,
	}
}

// CloneCompatibilityPolicy returns a defensive copy of compatibility metadata.
func CloneCompatibilityPolicy(policy CompatibilityPolicy) CompatibilityPolicy {
	policy.ActionAliases = cloneActionAliasSpecs(policy.ActionAliases)
	policy.ParameterAliases = cloneParameterAliasSpecs(policy.ParameterAliases)
	return policy
}

func cloneActionAliasSpecs(aliases []ActionAliasSpec) []ActionAliasSpec {
	if len(aliases) == 0 {
		return nil
	}
	out := make([]ActionAliasSpec, 0, len(aliases))
	for _, alias := range aliases {
		alias.Alias = strings.TrimSpace(strings.ToLower(alias.Alias))
		alias.Target = strings.TrimSpace(strings.ToLower(alias.Target))
		alias.Source = strings.TrimSpace(alias.Source)
		alias.RemovalVersion = strings.TrimSpace(alias.RemovalVersion)
		alias.Reason = strings.TrimSpace(alias.Reason)
		out = append(out, alias)
	}
	return out
}

func cloneParameterAliasSpecs(aliases []ParameterAliasSpec) []ParameterAliasSpec {
	if len(aliases) == 0 {
		return nil
	}
	out := make([]ParameterAliasSpec, 0, len(aliases))
	for _, alias := range aliases {
		alias.Alias = strings.TrimSpace(strings.ToLower(alias.Alias))
		alias.Target = strings.TrimSpace(alias.Target)
		alias.Source = strings.TrimSpace(alias.Source)
		alias.RemovalVersion = strings.TrimSpace(alias.RemovalVersion)
		alias.Reason = strings.TrimSpace(alias.Reason)
		out = append(out, alias)
	}
	return out
}

// CloneIndividualToolSpec returns a defensive copy of individual-tool metadata.
func CloneIndividualToolSpec(spec IndividualToolSpec) IndividualToolSpec {
	spec.AnnotationOverrides.ReadOnly = cloneBoolPointer(spec.AnnotationOverrides.ReadOnly)
	spec.AnnotationOverrides.Destructive = cloneBoolPointer(spec.AnnotationOverrides.Destructive)
	spec.AnnotationOverrides.Idempotent = cloneBoolPointer(spec.AnnotationOverrides.Idempotent)
	spec.AnnotationOverrides.OpenWorld = cloneBoolPointer(spec.AnnotationOverrides.OpenWorld)
	return spec
}

func cloneBoolPointer(value *bool) *bool {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

// Validate verifies invariants that must hold before projecting a spec.
func (spec ActionSpec) Validate() error {
	if strings.TrimSpace(spec.Name) == "" {
		return errors.New("action spec name is required")
	}
	if spec.Route.Destructive != spec.Destructive {
		return fmt.Errorf("action spec %q destructive flag %t does not match route %t", spec.Name, spec.Destructive, spec.Route.Destructive)
	}
	if spec.ReadOnly && spec.Destructive {
		return fmt.Errorf("action spec %q cannot be read-only and destructive", spec.Name)
	}
	if err := validateActionSpecPolicies(spec); err != nil {
		return err
	}
	if err := validateInputSchemaOverrides(spec); err != nil {
		return err
	}
	if err := validateActionSpecGuidance(spec); err != nil {
		return err
	}
	if err := validateActionSpecAliases(spec); err != nil {
		return err
	}
	if err := validateActionSpecCompatibility(spec); err != nil {
		return err
	}
	for _, tag := range spec.Tags {
		if tag != strings.ToLower(strings.TrimSpace(tag)) || strings.ContainsAny(tag, " \t\n\r") {
			return fmt.Errorf("action spec %q has non-normalized tag %q", spec.Name, tag)
		}
	}
	return nil
}

// ActionSpecsToMap converts canonical action specs to a legacy ActionMap.
func ActionSpecsToMap(specs []ActionSpec) ActionMap {
	routes, err := ActionSpecsToMapWithError(specs)
	if err != nil {
		panic(fmt.Errorf("ActionSpecsToMap: %w", err))
	}
	return routes
}

// ActionSpecsToMapWithError converts canonical action specs to a legacy ActionMap.
func ActionSpecsToMapWithError(specs []ActionSpec) (ActionMap, error) {
	routes := make(ActionMap, len(specs))
	var errs []error
	canonicalNames := actionSpecCanonicalNames(specs)
	for _, spec := range specs {
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			errs = append(errs, errors.New("action spec name is required"))
			continue
		}
		if _, exists := routes[name]; exists {
			errs = append(errs, fmt.Errorf("duplicate action spec %q", name))
			continue
		}
		if err := spec.Validate(); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := validateActionSpecAliasesAgainstNames(spec, canonicalNames); err != nil {
			errs = append(errs, err)
			continue
		}
		route := cloneActionRoute(spec.Route)
		route.Aliases = mergeActionSpecStrings(route.Aliases, spec.Aliases)
		route.Tags = mergeActionSpecStrings(route.Tags, spec.Tags)
		route.Usage = firstNonEmptyString(spec.Usage, route.Usage)
		route.RelatedActions = mergeActionSpecStrings(route.RelatedActions, spec.RelatedActions)
		route.ParameterGuidance = mergeActionSpecGuidance(route.ParameterGuidance, spec.ParameterGuidance)
		routes[name] = route
	}
	return routes, errors.Join(errs...)
}

func actionSpecCanonicalNames(specs []ActionSpec) map[string]struct{} {
	names := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		if name := strings.ToLower(strings.TrimSpace(spec.Name)); name != "" {
			names[name] = struct{}{}
		}
	}
	return names
}

func cloneActionRoute(route ActionRoute) ActionRoute {
	routes := CloneMetaSchemaRoutes(map[string]ActionMap{"_": {"_": route}})
	return routes["_"]["_"]
}

func mergeActionSpecGuidance(routeGuidance, specGuidance map[string]ParameterGuidance) map[string]ParameterGuidance {
	merged := cloneParameterGuidanceMap(routeGuidance)
	if len(specGuidance) == 0 {
		return merged
	}
	if merged == nil {
		merged = make(map[string]ParameterGuidance, len(specGuidance))
	}
	keys := make([]string, 0, len(specGuidance))
	for key := range specGuidance {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		item := cloneParameterGuidance(specGuidance[key])
		if existing, ok := merged[key]; ok {
			if existing.SemanticRole == "" {
				existing.SemanticRole = item.SemanticRole
			}
			if existing.ValueSource == "" {
				existing.ValueSource = item.ValueSource
			}
			if existing.ExampleBinding == "" {
				existing.ExampleBinding = item.ExampleBinding
			}
			existing.CommonConfusions = mergeActionSpecNotes(existing.CommonConfusions, item.CommonConfusions)
			merged[key] = existing
			continue
		}
		merged[key] = item
	}
	return merged
}

func cloneParameterGuidance(item ParameterGuidance) ParameterGuidance {
	item.CommonConfusions = append([]string(nil), item.CommonConfusions...)
	return item
}

func normalizeActionSpecStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
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

func normalizeActionSpecNotes(values []string) []string {
	return mergeActionSpecNotes(nil, values)
}

func mergeActionSpecNotes(left, right []string) []string {
	if len(left)+len(right) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(left)+len(right))
	out := make([]string, 0, len(left)+len(right))
	for _, value := range append(append([]string(nil), left...), right...) {
		value = strings.TrimSpace(value)
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

func mergeActionSpecStrings(left, right []string) []string {
	return normalizeActionSpecStrings(append(cloneRouteStrings(left), right...))
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func validateActionSpecPolicies(spec ActionSpec) error {
	if err := validateOptionalActionSpecPolicy(spec.Name, "content kind", spec.ContentKind, actionSpecContentKinds()); err != nil {
		return err
	}
	if err := validateOptionalActionSpecPolicy(spec.Name, "not-found policy", spec.NotFoundPolicy, actionSpecNotFoundPolicies()); err != nil {
		return err
	}
	if err := validateOptionalActionSpecPolicy(spec.Name, "embedded resource policy", spec.EmbeddedResourcePolicy, actionSpecEmbeddedResourcePolicies()); err != nil {
		return err
	}
	if err := validateOptionalActionSpecPolicy(spec.Name, "rich result policy", spec.RichResultPolicy, actionSpecRichResultPolicies()); err != nil {
		return err
	}
	return nil
}

func validateOptionalActionSpecPolicy(actionName, fieldName, value string, allowed map[string]struct{}) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if _, ok := allowed[value]; ok {
		return nil
	}
	return fmt.Errorf("action spec %q has unsupported %s %q", actionName, fieldName, value)
}

func actionSpecContentKinds() map[string]struct{} {
	return map[string]struct{}{
		ActionSpecContentList:      {},
		ActionSpecContentDetail:    {},
		ActionSpecContentMutate:    {},
		ActionSpecContentAssistant: {},
		ActionSpecContentImage:     {},
	}
}

func actionSpecNotFoundPolicies() map[string]struct{} {
	return map[string]struct{}{
		ActionSpecNotFoundNone:      {},
		ActionSpecNotFoundResult:    {},
		ActionSpecNotFoundPropagate: {},
	}
}

func actionSpecEmbeddedResourcePolicies() map[string]struct{} {
	return map[string]struct{}{
		ActionSpecEmbeddedNone:     {},
		ActionSpecEmbeddedOptional: {},
		ActionSpecEmbeddedAlways:   {},
	}
}

func actionSpecRichResultPolicies() map[string]struct{} {
	return map[string]struct{}{
		ActionSpecRichStandard:     {},
		ActionSpecRichImage:        {},
		ActionSpecRichResourceLink: {},
		ActionSpecRichMixed:        {},
	}
}

func validateActionSpecGuidance(spec ActionSpec) error {
	guidance := mergeActionSpecGuidance(spec.Route.ParameterGuidance, spec.ParameterGuidance)
	if len(guidance) == 0 {
		return nil
	}
	fields := schemaPropertyNames(spec.Route.InputSchema)
	if len(fields) == 0 {
		return fmt.Errorf("action spec %q has parameter guidance without an input schema", spec.Name)
	}
	for key := range guidance {
		if _, ok := fields[key]; !ok {
			return fmt.Errorf("action spec %q has guidance for unknown parameter %q", spec.Name, key)
		}
	}
	return nil
}

func validateActionSpecAliases(spec ActionSpec) error {
	canonicalName := strings.ToLower(strings.TrimSpace(spec.Name))
	for _, alias := range spec.Aliases {
		if alias == canonicalName {
			return fmt.Errorf("action spec %q alias duplicates its action name", spec.Name)
		}
		if slices.Contains(spec.RelatedActions, alias) {
			return fmt.Errorf("action spec %q alias %q also appears in related actions", spec.Name, alias)
		}
	}
	return nil
}

func validateActionSpecAliasesAgainstNames(spec ActionSpec, canonicalNames map[string]struct{}) error {
	canonicalName := strings.ToLower(strings.TrimSpace(spec.Name))
	for _, alias := range spec.Aliases {
		if alias == canonicalName {
			continue
		}
		if _, ok := canonicalNames[alias]; ok {
			return fmt.Errorf("action spec %q alias %q duplicates canonical action name", spec.Name, alias)
		}
	}
	return nil
}

func validateActionSpecCompatibility(spec ActionSpec) error {
	if err := validateActionAliasSpecs(spec.Name, spec.Compatibility.ActionAliases); err != nil {
		return err
	}
	return validateParameterAliasSpecs(spec.Name, spec.Route.InputSchema, spec.Compatibility.ParameterAliases)
}

func validateActionAliasSpecs(actionName string, aliases []ActionAliasSpec) error {
	canonicalName := strings.ToLower(strings.TrimSpace(actionName))
	seen := make(map[string]string, len(aliases))
	for _, alias := range aliases {
		aliasName := strings.TrimSpace(strings.ToLower(alias.Alias))
		target := strings.TrimSpace(strings.ToLower(alias.Target))
		if aliasName == "" {
			return fmt.Errorf("action spec %q has compatibility action alias without alias", actionName)
		}
		if target == "" {
			return fmt.Errorf("action spec %q compatibility action alias %q has no target", actionName, aliasName)
		}
		if target != canonicalName {
			return fmt.Errorf("action spec %q compatibility action alias %q targets %q", actionName, aliasName, target)
		}
		if strings.TrimSpace(alias.Source) == "" {
			return fmt.Errorf("action spec %q compatibility action alias %q has no source", actionName, aliasName)
		}
		if strings.TrimSpace(alias.Reason) == "" {
			return fmt.Errorf("action spec %q compatibility action alias %q has no reason", actionName, aliasName)
		}
		if alias.Deprecated && strings.TrimSpace(alias.RemovalVersion) == "" {
			return fmt.Errorf("action spec %q deprecated compatibility action alias %q has no removal version", actionName, aliasName)
		}
		if existingTarget, ok := seen[aliasName]; ok && existingTarget != target {
			return fmt.Errorf("action spec %q compatibility action alias %q targets both %q and %q", actionName, aliasName, existingTarget, target)
		}
		seen[aliasName] = target
	}
	return nil
}

func validateParameterAliasSpecs(actionName string, inputSchema map[string]any, aliases []ParameterAliasSpec) error {
	seen := make(map[string]string, len(aliases))
	for _, alias := range aliases {
		aliasName := strings.TrimSpace(strings.ToLower(alias.Alias))
		target := strings.TrimSpace(alias.Target)
		if aliasName == "" {
			return fmt.Errorf("action spec %q has compatibility parameter alias without alias", actionName)
		}
		if target == "" {
			return fmt.Errorf("action spec %q compatibility parameter alias %q has no target", actionName, aliasName)
		}
		if !schemaHasPropertyPath(inputSchema, target) {
			return fmt.Errorf("action spec %q compatibility parameter alias %q targets unknown parameter %q", actionName, aliasName, target)
		}
		if strings.TrimSpace(alias.Source) == "" {
			return fmt.Errorf("action spec %q compatibility parameter alias %q has no source", actionName, aliasName)
		}
		if strings.TrimSpace(alias.Reason) == "" {
			return fmt.Errorf("action spec %q compatibility parameter alias %q has no reason", actionName, aliasName)
		}
		if alias.Deprecated && strings.TrimSpace(alias.RemovalVersion) == "" {
			return fmt.Errorf("action spec %q deprecated compatibility parameter alias %q has no removal version", actionName, aliasName)
		}
		if existingTarget, ok := seen[aliasName]; ok && existingTarget != target {
			return fmt.Errorf("action spec %q compatibility parameter alias %q targets both %q and %q", actionName, aliasName, existingTarget, target)
		}
		seen[aliasName] = target
	}
	return nil
}

func schemaHasPropertyPath(schema map[string]any, target string) bool {
	parts := strings.Split(strings.TrimSpace(target), ".")
	if len(parts) == 0 || parts[0] == "" {
		return false
	}
	return schemaHasPropertyPathFrom(schema, schema, parts)
}

func schemaHasPropertyPathFrom(root, schema map[string]any, parts []string) bool {
	if len(parts) == 0 {
		return false
	}
	schema = resolveSchemaRef(root, schema)
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return false
	}
	child, ok := properties[parts[0]].(map[string]any)
	if !ok {
		return false
	}
	if len(parts) == 1 {
		return true
	}
	child = resolveSchemaRef(root, child)
	if child == nil {
		return false
	}
	if items, hasItems := child["items"].(map[string]any); hasItems {
		child = resolveSchemaRef(root, items)
	}
	return schemaHasPropertyPathFrom(root, child, parts[1:])
}

func resolveSchemaRef(root, schema map[string]any) map[string]any {
	ref, ok := schema["$ref"].(string)
	if !ok || !strings.HasPrefix(ref, "#/$defs/") {
		return schema
	}
	defs, ok := root["$defs"].(map[string]any)
	if !ok {
		return schema
	}
	definition, ok := defs[strings.TrimPrefix(ref, "#/$defs/")].(map[string]any)
	if !ok {
		return schema
	}
	return definition
}
