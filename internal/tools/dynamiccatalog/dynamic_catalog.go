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
//
// The catalog is assembled once per distinct configuration and shared (see
// [gitlabtools.ShareCatalog]); what is returned is that catalog bound to
// client. The standalone actions are built with the unbound client the shared
// copy is built with, and bound with everything else.
func Build(client *gitlabclient.Client, cfg *config.ServerConfig) (*actioncatalog.Catalog, gitlabtools.WithheldActions, error) {
	dotcom := client.IsGitLabDotCom()
	key := "dynamic|" + gitlabtools.BaseCatalogKey(cfg.Tier, dotcom, true) + "|" + gitlabtools.CatalogFilterKey(cfg)
	catalog, withheld, err := gitlabtools.ShareCatalog(key, func() (*actioncatalog.Catalog, gitlabtools.WithheldActions, error) {
		return build(gitlabtools.UnboundClient(dotcom), dotcom, cfg)
	})
	if err != nil {
		return nil, gitlabtools.WithheldActions{}, err
	}
	return catalog.BindTo(client), withheld, nil
}

// Test seams. None of these can fail from any input the server takes: the
// specs are compiled in, the filter adds groups the base already validated,
// and the standalone specs are fixed. The branches reporting their failure
// exist for the day that changes, and would otherwise never run.
var (
	sharedBaseCatalog    = gitlabtools.SharedBaseCatalog     //nolint:gochecknoglobals // test seam
	filterActionCatalog  = gitlabtools.FilterActionCatalog   //nolint:gochecknoglobals // test seam
	addStandaloneCatalog = dynamictools.AddStandaloneCatalog //nolint:gochecknoglobals // test seam
)

// build assembles the dynamic catalog for cfg from the shared base catalog,
// with the standalone actions bound to client.
func build(client *gitlabclient.Client, dotcom bool, cfg *config.ServerConfig) (*actioncatalog.Catalog, gitlabtools.WithheldActions, error) {
	catalog, err := sharedBaseCatalog(dotcom, gitlabtools.ActionCatalogOptions{
		Tier:       cfg.Tier,
		IncludeMCP: true,
	})
	if err != nil {
		return nil, gitlabtools.WithheldActions{}, fmt.Errorf("build action catalog: %w", err)
	}
	filterCfg := *cfg
	filterCfg.SafeMode = false
	filtered, withheld, filterErr := filterActionCatalog(catalog, &filterCfg)
	if filterErr != nil {
		return nil, gitlabtools.WithheldActions{}, fmt.Errorf("filter dynamic action catalog: %w", filterErr)
	}
	withStandalone, standaloneErr := addStandaloneCatalog(filtered, client, dynamictools.StandaloneOptions{
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
