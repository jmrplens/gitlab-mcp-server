package main

import (
	"fmt"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
)

// catalogActions returns every action in the canonical catalog at the widest
// tier, which is the surface this audit has to answer for: a tier below
// Ultimate only removes actions, so auditing Ultimate audits all of them.
//
// The client is nil on purpose. Building the catalog registers routes and
// schemas from the specs compiled into this binary and never calls GitLab, so
// the audit needs no instance, no token, and no network.
func catalogActions() ([]action, error) {
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{
		Tier:       edition.Ultimate,
		IncludeMCP: true,
	})
	if err != nil {
		return nil, fmt.Errorf("build action catalog: %w", err)
	}
	var actions []action
	for _, group := range catalog.Groups() {
		for _, catalogAction := range group.ActionsInOrder() {
			actions = append(actions, action{
				ID:       string(catalogAction.ID),
				Name:     catalogAction.Name,
				Owner:    catalogAction.OwnerPackage,
				ReadOnly: catalogAction.ReadOnly,
			})
		}
	}
	return actions, nil
}
