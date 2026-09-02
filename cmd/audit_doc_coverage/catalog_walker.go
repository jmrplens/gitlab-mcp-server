// Package main: catalog walker.
//
// loadCatalog builds the canonical action catalog using the same
// options that the production server uses for the
// self-managed-enterprise tier, then snapshots every action's
// IndividualTool.Name, owning group, tier, and destructive flag into a
// flat lookup table for the auditor's main comparison loop.
package main

import (
	"fmt"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/auditclient"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
)

// catalogTool is the auditor's flat view of one individual tool. It
// carries just enough metadata to drive the comparison: the
// IndividualTool.Name (which the doc headings reference), the owning
// group ToolName (which the README mapping keys off), the tier (for
// badge verification), the destructive flag (for downstream checks
// the auditor does not currently emit but reserves), and the owner
// package (for special-case routing like the events package).
type catalogTool struct {
	Name         string
	Group        string
	Tier         string
	Destructive  bool
	ReadOnly     bool
	OwnerPackage string
}

// catalogSnapshot is the in-memory shape produced by loadCatalog.
type catalogSnapshot struct {
	Tools map[string]catalogTool
}

// loadCatalog constructs the canonical catalog via the production
// BuildActionCatalog path, including the MCP maintenance group so the
// capabilities doc's gitlab_server_status is included.
func loadCatalog(repoRoot string) (*catalogSnapshot, error) {
	_ = repoRoot // reserved for future repo-root-aware catalog options.
	client, cleanup := auditclient.NewMock()
	defer cleanup()

	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{
		Enterprise: true,
		IncludeMCP: true,
	})
	if err != nil {
		return nil, fmt.Errorf("build action catalog: %w", err)
	}

	out := &catalogSnapshot{Tools: make(map[string]catalogTool)}
	for _, group := range catalog.Groups() {
		for _, action := range group.ActionsInOrder() {
			name := action.IndividualTool.Name
			if name == "" {
				continue
			}
			// Some IndividualTool.Name values collide across groups
			// (e.g. compatibility aliases). The first registration
			// wins; surface a stable, deterministic projection by
			// only overwriting empty-tier entries when a richer
			// record arrives, or vice-versa, so the auditor's view
			// does not silently flip on import order.
			if existing, ok := out.Tools[name]; ok && existing.Group != "" && existing.Group != group.ToolName {
				continue
			}
			out.Tools[name] = catalogTool{
				Name:         name,
				Group:        group.ToolName,
				Tier:         action.Edition,
				Destructive:  action.Destructive,
				ReadOnly:     action.ReadOnly,
				OwnerPackage: action.OwnerPackage,
			}
		}
	}
	return out, nil
}
