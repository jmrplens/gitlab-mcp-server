package dynamic

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/surfaces"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// StandaloneOptions controls which standalone tools are added to the canonical
// dynamic action catalog.
type StandaloneOptions struct {
	ReadOnly     bool
	ExcludeTools []string
}

// AddStandaloneRoutes adds non-meta standalone tools to the canonical dynamic
// action catalog so dynamic mode can still execute them through
// gitlab_execute_action without increasing the visible tool count.
func AddStandaloneRoutes(routes map[string]toolutil.ActionMap, client *gitlabclient.Client, opts StandaloneOptions) (map[string]toolutil.ActionMap, error) {
	catalog, err := AddStandaloneCatalog(actioncatalog.FromActionMaps(routes), client, opts)
	if err != nil {
		return nil, err
	}
	return catalog.ActionMaps(), nil
}

// AddStandaloneCatalog adds non-meta standalone tools to the canonical dynamic
// action catalog so dynamic mode can execute them without increasing the
// visible tool count.
func AddStandaloneCatalog(catalog *actioncatalog.Catalog, client *gitlabclient.Client, opts StandaloneOptions) (*actioncatalog.Catalog, error) {
	if catalog == nil {
		catalog = actioncatalog.NewCatalog()
	} else {
		catalog = catalog.Clone()
	}
	return surfaces.AddToolCatalog(catalog, surfaces.StandaloneToolSpecs(client), surfaces.CatalogOptions{
		ReadOnlyOnly:     opts.ReadOnly,
		ExcludeToolNames: opts.ExcludeTools,
	})
}
