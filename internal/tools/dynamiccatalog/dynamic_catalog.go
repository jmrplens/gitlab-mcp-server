package dynamiccatalog

import (
	"fmt"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
)

// Build builds the executable catalog for low-token dynamic mode. Filters run
// before standalone tools are added so configured exclusions, token scopes,
// and read-only mode cannot leave hidden catalog actions behind, and safe-mode
// previews are applied last, over the complete catalog, so the standalone
// actions are previewed like every other write. The bookkeeping returned
// beside it is what RegisterCatalogFindExecuteTools needs through
// WithWithheldActions, so a withheld action is reported with its cause rather
// than as unknown.
//
// The ordering of safe mode is the one thing here that is not a straight move
// out of cmd/server. There the previews were applied inside the filter, before
// the standalone actions joined, and the dynamic execute tool is exempt from
// the per-tool safe-mode wrapping the individual surface gets; so in safe mode
// the standalone writes, the interactive creation flows among them, kept their
// real handlers. Read-only mode never had that gap, because the standalone
// builder takes it as an option.
func Build(client *gitlabclient.Client, cfg *config.ServerConfig) (*actioncatalog.Catalog, gitlabtools.WithheldActions, error) {
	catalog, err := gitlabtools.BuildActionCatalog(client, gitlabtools.ActionCatalogOptions{
		Tier:       cfg.Tier,
		IncludeMCP: true,
	})
	if err != nil {
		return nil, gitlabtools.WithheldActions{}, fmt.Errorf("build action catalog: %w", err)
	}
	filterCfg := *cfg
	filterCfg.SafeMode = false
	filtered, withheld, filterErr := gitlabtools.FilterActionCatalog(catalog, &filterCfg)
	if filterErr != nil {
		return nil, gitlabtools.WithheldActions{}, fmt.Errorf("filter dynamic action catalog: %w", filterErr)
	}
	withStandalone, standaloneErr := dynamictools.AddStandaloneCatalog(filtered, client, dynamictools.StandaloneOptions{
		ReadOnly:     cfg.ReadOnly,
		ExcludeTools: cfg.ExcludeTools,
	})
	if standaloneErr != nil {
		return nil, gitlabtools.WithheldActions{}, fmt.Errorf("add standalone dynamic actions: %w", standaloneErr)
	}
	if cfg.SafeMode {
		withStandalone = withStandalone.WithSafeModePreviews()
	}
	return withStandalone, withheld, nil
}
