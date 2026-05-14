package actioncatalog

import (
	"errors"
	"fmt"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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
		actions = append(actions, Action{
			Name:                   spec.Name,
			Route:                  route,
			Aliases:                append([]string(nil), spec.Aliases...),
			Tags:                   append([]string(nil), spec.Tags...),
			Usage:                  spec.Usage,
			RelatedActions:         append([]string(nil), spec.RelatedActions...),
			ReadOnly:               spec.ReadOnly,
			Edition:                spec.Edition,
			GitLabDotComOnly:       spec.GitLabDotComOnly,
			OwnerPackage:           spec.OwnerPackage,
			IndividualTool:         spec.IndividualTool,
			ContentKind:            spec.ContentKind,
			NotFoundPolicy:         spec.NotFoundPolicy,
			EmbeddedResourcePolicy: spec.EmbeddedResourcePolicy,
			RichResultPolicy:       spec.RichResultPolicy,
			RuntimeValidationNotes: append([]string(nil), spec.RuntimeValidationNotes...),
		})
	}
	return actions, errors.Join(errs...)
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
