package toolvisibility

import (
	"context"
	"log/slog"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Apply narrows the tools registered on server to what cfg says the
// deployment serves, in four steps: the tools cfg.ExcludeTools names are
// removed, then, for a credential carrying read_api and not api, the
// standalone utilities whose action GitLab refuses read_api, then read-only
// mode removes every tool without a read-only hint, or safe mode wraps every
// mutating tool that the catalog does not already preview per action.
// Read-only mode wins over safe mode, since nothing is left for safe mode to
// wrap.
//
// It runs after registration, over every tool the server holds, which is what
// reaches the tools registered outside the catalog: the catalog filter has
// already applied the same configuration to the catalog-backed ones, per
// action, and this pass is the second mechanism the standalone flows need.
// The exclusions reach those flows by the catalog's own rule, so an entry
// naming one by its group name or its canonical action ID removes it here as
// its tool name does (see removeExcluded). toolSurface and surfaceCatalog
// decide the safe-mode exemptions (see catalogBackedToolNames); a surface
// whose exclusions were applied to the catalog legitimately has nothing left
// for the first step to remove.
//
// The token-scope filter is deliberately absent, the standalone share of the
// read_api narrowing aside. It is applied to the catalog, before
// registration, by the three catalog assemblers, which is the only place it
// can reach the individual surface: its keys are meta-tool group names and
// that surface registers one tool per action, so a pass over registered names
// here matched nothing and left every admin tool listed for a token with no
// admin_mode. See [gitlabtools.MetaToolScopes]. The read_api step here names
// tools by the rule the catalog applies ([gitlabtools.StandaloneToolsBeyond]),
// never by their annotation, which is the operator's read-only rule.
func Apply(ctx context.Context, server *mcp.Server, cfg *config.ServerConfig, toolSurface string, surfaceCatalog *actioncatalog.Catalog) {
	if len(cfg.ExcludeTools) > 0 {
		removed := removeExcluded(ctx, server, cfg.ExcludeTools)
		// Named for what it counts. This pass removes registered tools only,
		// so on a surface whose exclusions are applied to the catalog it
		// legitimately removes nothing, and a bare "excluded" reading zero
		// there said the opposite of what had happened.
		slog.InfoContext(ctx, "excluded tools by configuration", "excluded_registered_tools", removed, "patterns", cfg.ExcludeTools)
	}
	if cfg.ReadAPIOnly {
		// The catalog assemblers already narrowed the catalog-backed tools to
		// what read_api reaches; the standalone utilities are registered
		// outside the catalog on the meta and individual surfaces, so their
		// share of the narrowing is applied here, by the same per-action rule.
		// The dynamic surface registers none of them and removes nothing.
		removed := removeRegistered(ctx, server, "read-api", gitlabtools.StandaloneToolsBeyond(finegrained.ClassicReadAPI))
		slog.InfoContext(ctx, "read_api token: removed the standalone tools GitLab refuses read_api", "removed", removed)
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

// removeExcluded removes every registered tool an entry of exclude names, and
// returns how many it removed. A server that cannot be listed has nothing
// removed and the failure logged: guessing at names to remove would be worse
// than leaving an exclusion unapplied and reported.
//
// Two rules name a tool, and either is enough. An entry equal to a registered
// name removes that tool, which is how gitlab_find_action leaves the dynamic
// surface; the match is whole, never a prefix, since the names of the three
// surfaces nest inside one another. And an entry naming a standalone utility
// by any spelling the catalog accepts, the group name gitlab_interactive, a
// tool name or a canonical action ID such as interactive.issue_create, removes
// the tools [gitlabtools.ExcludedStandaloneTools] resolves it to. Only the
// first rule applied here until issue 911, so the other two spellings removed
// no guided flow on the meta and individual surfaces while the documentation
// offered all three.
func removeExcluded(ctx context.Context, server *mcp.Server, exclude []string) int {
	standalone, _ := gitlabtools.ExcludedStandaloneTools(exclude)
	return removeRegistered(ctx, server, "exclude-tools", append(slices.Clone(exclude), standalone...))
}

// removeRegistered removes every registered tool names holds and returns how
// many it removed, which a name no tool is registered under adds nothing to.
// A server that cannot be listed has nothing removed and the failure logged
// under the pass's label.
func removeRegistered(ctx context.Context, server *mcp.Server, label string, names []string) int {
	registered, err := toolutil.ListRegisteredTools(ctx, server, label)
	if err != nil {
		slog.ErrorContext(ctx, label+": list registered tools failed", "error", err)
		return 0
	}
	var toRemove []string
	for _, tool := range registered {
		if slices.Contains(names, tool.Name) {
			toRemove = append(toRemove, tool.Name)
		}
	}
	// Called unconditionally, as [gitlabtools.RemoveNonReadOnlyTools] does:
	// RemoveTools decides per name, so an empty list removes nothing and
	// notifies nobody.
	server.RemoveTools(toRemove...)
	return len(toRemove)
}
