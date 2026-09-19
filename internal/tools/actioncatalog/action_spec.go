package actioncatalog

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ActionsFromSpecs projects canonical action specs into catalog actions.
func ActionsFromSpecs(specs []toolutil.ActionSpec) ([]Action, error) {
	routes, err := toolutil.ActionSpecsToMapWithError(specs)
	if err != nil {
		return nil, err
	}
	actions := make([]Action, 0, len(specs))
	seen := make(map[string]struct{}, len(specs))
	var errs []error
	for _, spec := range specs {
		if _, exists := seen[spec.Name]; exists {
			continue
		}
		seen[spec.Name] = struct{}{}
		route, ok := routes[spec.Name]
		if !ok {
			errs = append(errs, fmt.Errorf("action spec %q was not projected to a route", spec.Name))
			continue
		}
		// The dispatchers see the route, not the spec, so the template travels
		// on it; a policy that never embeds leaves the route without one.
		if spec.EmbeddedResourcePolicy != "" && spec.EmbeddedResourcePolicy != toolutil.ActionSpecEmbeddedNone {
			route.EmbeddedResource = spec.EmbeddedResource
		}
		// The content kind travels on the route for the same reason: it is
		// what the dispatcher annotates the result with.
		route.ContentKind = spec.ContentKind
		// And so do the historical spellings, because the meta dispatcher
		// resolves one out of the group's route map and never sees the spec.
		route.CompatibilityAliases = compatibilityAliasNames(spec.Compatibility.ActionAliases)
		actions = append(actions, Action{
			Name:                   spec.Name,
			Route:                  route,
			SpecBacked:             true,
			Aliases:                append([]string(nil), spec.Aliases...),
			Tags:                   append([]string(nil), spec.Tags...),
			Usage:                  spec.Usage,
			RelatedActions:         append([]string(nil), spec.RelatedActions...),
			Compatibility:          toolutil.CloneCompatibilityPolicy(spec.Compatibility),
			ReadOnly:               spec.ReadOnly,
			Edition:                spec.Edition,
			GitLabDotComOnly:       spec.GitLabDotComOnly,
			OwnerPackage:           spec.OwnerPackage,
			IndividualTool:         toolutil.CloneIndividualToolSpec(spec.IndividualTool),
			ContentKind:            spec.ContentKind,
			NotFoundPolicy:         spec.NotFoundPolicy,
			EmbeddedResourcePolicy: spec.EmbeddedResourcePolicy,
			EmbeddedResource:       spec.EmbeddedResource,
			RichResultPolicy:       spec.RichResultPolicy,
			SchemaValidationNotes:  append([]string(nil), spec.SchemaValidationNotes...),
			RuntimeValidationNotes: append([]string(nil), spec.RuntimeValidationNotes...),
			Destructive:            spec.Destructive,
			Idempotent:             spec.Idempotent,
			OpenWorld:              spec.OpenWorld,
		})
	}
	return actions, errors.Join(errs...)
}

// compatibilityAliasNames returns the alias spellings of a compatibility
// policy, lowercased and without blanks, in declaration order.
//
// The target is dropped rather than carried: a spec's alias may only target
// the action declaring it (validateActionAliasSpecs refuses anything else), so
// the name the route is filed under is already the target.
func compatibilityAliasNames(aliases []toolutil.ActionAliasSpec) []string {
	if len(aliases) == 0 {
		return nil
	}
	names := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		name := strings.ToLower(strings.TrimSpace(alias.Alias))
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil
	}
	return names
}

// GroupFromSpecs builds a catalog group from canonical action specs.
func GroupFromSpecs(opts GroupOptions, specs []toolutil.ActionSpec) (Group, error) {
	actions, err := ActionsFromSpecs(specs)
	if err != nil {
		return Group{}, err
	}
	group := NewGroup(opts)
	for _, action := range actions {
		group.SetAction(action)
	}
	return normalizeGroup(group)
}
