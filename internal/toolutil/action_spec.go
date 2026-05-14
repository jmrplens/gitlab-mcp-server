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
	ParameterGuidance      map[string]ParameterGuidance
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
	RuntimeValidationNotes []string
}

// IndividualToolSpec carries compatibility metadata for the individual-tool surface.
type IndividualToolSpec struct {
	Name        string
	Title       string
	Description string
}

// ActionSpecOptions contains optional metadata for NewActionSpec.
type ActionSpecOptions struct {
	Aliases                []string
	Tags                   []string
	Usage                  string
	RelatedActions         []string
	ParameterGuidance      map[string]ParameterGuidance
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
	RuntimeValidationNotes []string
}

// NewActionSpec creates a defensive canonical action specification.
func NewActionSpec(name string, route ActionRoute, opts ActionSpecOptions) ActionSpec {
	route = cloneActionRoute(route)
	return ActionSpec{
		Name:                   strings.TrimSpace(name),
		Route:                  route,
		Aliases:                mergeActionSpecStrings(route.Aliases, opts.Aliases),
		Tags:                   mergeActionSpecStrings(route.Tags, opts.Tags),
		Usage:                  firstNonEmptyString(opts.Usage, route.Usage),
		RelatedActions:         mergeActionSpecStrings(route.RelatedActions, opts.RelatedActions),
		ParameterGuidance:      cloneParameterGuidanceMap(opts.ParameterGuidance),
		ReadOnly:               opts.ReadOnly,
		Destructive:            route.Destructive || opts.Destructive,
		Idempotent:             opts.Idempotent,
		OpenWorld:              opts.OpenWorld,
		Edition:                strings.TrimSpace(opts.Edition),
		GitLabDotComOnly:       opts.GitLabDotComOnly,
		OwnerPackage:           strings.TrimSpace(opts.OwnerPackage),
		IndividualTool:         opts.IndividualTool,
		ContentKind:            strings.TrimSpace(opts.ContentKind),
		NotFoundPolicy:         strings.TrimSpace(opts.NotFoundPolicy),
		EmbeddedResourcePolicy: strings.TrimSpace(opts.EmbeddedResourcePolicy),
		RichResultPolicy:       strings.TrimSpace(opts.RichResultPolicy),
		RuntimeValidationNotes: normalizeActionSpecStrings(opts.RuntimeValidationNotes),
	}
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
	if err := validateActionSpecGuidance(spec); err != nil {
		return err
	}
	if err := validateActionSpecAliases(spec); err != nil {
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
			existing.CommonConfusions = append(existing.CommonConfusions, item.CommonConfusions...)
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
