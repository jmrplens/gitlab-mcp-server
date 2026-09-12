package toolvisibility

import (
	"context"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Apply narrows the tools registered on server to what cfg says the
// deployment serves, in three steps: the names cfg.ExcludeTools lists are
// removed, then read-only mode removes every tool without a read-only hint,
// or safe mode wraps every mutating tool that the catalog does not already
// preview per action. Read-only mode wins over safe mode, since nothing is
// left for safe mode to wrap.
//
// It runs after registration, over every tool the server holds, which is what
// reaches the tools registered outside the catalog: the catalog filter has
// already applied the same configuration to the catalog-backed ones, per
// action, and this pass is the second mechanism the standalone flows need.
// toolSurface and surfaceCatalog decide the safe-mode exemptions (see
// catalogBackedToolNames); a surface whose exclusions were applied to the
// catalog legitimately has nothing left for the first step to remove.
//
// The token-scope filter is deliberately absent. It is applied to the
// catalog, before registration, by the three catalog assemblers, which is the
// only place it can reach the individual surface: its keys are meta-tool
// group names and that surface registers one tool per action, so a pass over
// registered names here matched nothing and left every admin tool listed for
// a token with no admin_mode. See [gitlabtools.MetaToolScopes].
func Apply(ctx context.Context, server *mcp.Server, cfg *config.ServerConfig, toolSurface string, surfaceCatalog *actioncatalog.Catalog) {
	if len(cfg.ExcludeTools) > 0 {
		removed := removeExcluded(ctx, server, cfg.ExcludeTools)
		// Named for what it counts. This pass sees registered tool names only,
		// so on a surface whose exclusions are applied to the catalog it
		// legitimately removes nothing, and a bare "excluded" reading zero
		// there said the opposite of what had happened.
		slog.InfoContext(ctx, "excluded tools by configuration", "excluded_registered_tools", removed, "patterns", cfg.ExcludeTools)
	}
	if cfg.ReadOnly {
		removed := gitlabtools.RemoveNonReadOnlyTools(ctx, server)
		slog.InfoContext(ctx, "read-only mode: removed write tools", "removed", removed)
		return
	}
	if cfg.SafeMode {
		// Catalog-backed dispatcher tools already carry per-action preview
		// handlers from the catalog filter, and wrapping them here would block
		// the reads they also serve. Everything else still needs wrapping,
		// including tools registered outside the catalog such as the
		// gitlab_interactive_* utilities.
		exempt := catalogBackedToolNames(surfaceCatalog, toolSurface)
		wrapped := gitlabtools.WrapMutatingToolsForSafeModeExcept(ctx, server, exempt)
		slog.InfoContext(ctx, "safe mode: intercepted mutating operations",
			"surface", toolSurface, "wrapped_tools", wrapped, "catalog_backed_tools", len(exempt))
	}
}

// catalogBackedToolNames returns the tools whose handlers come from the action
// catalog and therefore already enforce safe mode per action.
func catalogBackedToolNames(surfaceCatalog *actioncatalog.Catalog, toolSurface string) map[string]struct{} {
	if toolSurface == config.ToolSurfaceIndividual {
		// One tool is one action here, so tool-level wrapping is already
		// action-granular and nothing is exempt.
		return nil
	}
	exempt := map[string]struct{}{}
	if toolSurface == config.ToolSurfaceDynamic {
		exempt[dynamictools.FindActionToolName] = struct{}{}
		exempt[dynamictools.ExecuteActionToolName] = struct{}{}
		return exempt
	}
	if surfaceCatalog == nil {
		return exempt
	}
	for _, group := range surfaceCatalog.Groups() {
		exempt[group.ToolName] = struct{}{}
	}
	return exempt
}

// removeExcluded removes every registered tool whose name is in exclude,
// matched exactly, and returns how many it removed. A server that cannot be
// listed has nothing removed and the failure logged: guessing at names to
// remove would be worse than leaving an exclusion unapplied and reported.
func removeExcluded(ctx context.Context, server *mcp.Server, exclude []string) int {
	registered, err := toolutil.ListRegisteredTools(ctx, server, "exclude-filter")
	if err != nil {
		slog.ErrorContext(ctx, "exclude-tools: list registered tools failed", "error", err)
		return 0
	}
	excludeSet := make(map[string]struct{}, len(exclude))
	for _, name := range exclude {
		excludeSet[name] = struct{}{}
	}
	var toRemove []string
	for _, tool := range registered {
		if _, ok := excludeSet[tool.Name]; ok {
			toRemove = append(toRemove, tool.Name)
		}
	}
	// Called unconditionally, as [gitlabtools.RemoveNonReadOnlyTools] does:
	// RemoveTools decides per name, so an empty list removes nothing and
	// notifies nobody.
	server.RemoveTools(toRemove...)
	return len(toRemove)
}
