package tools

import (
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/surfaces"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// StandaloneSurfaceToolSpecs returns visible utility tools that remain
// outside ordinary GitLab API meta-tool dispatchers. The list currently
// includes the interactive elicitation tools (gitlab_interactive_*) and
// the project discovery helper (gitlab_discover_project).
func StandaloneSurfaceToolSpecs(client *gitlabclient.Client) []actioncatalog.SurfaceToolSpec {
	return surfaces.StandaloneToolSpecs(client)
}

// ExcludedStandaloneTools resolves an --exclude-tools list against the
// standalone utilities, and returns the tool names it removes, sorted,
// together with the entries that named none of them, in the operator's order.
//
// The standalone utilities are the one part of every surface the action
// catalog does not hold: the meta and individual surfaces register them as
// tools of their own and narrow them after registration, and the dynamic
// surface adds them to its catalog after that catalog was filtered. So the
// catalog's exclusion rule never reached them, and each path kept its own
// copy, which is how the canonical action ID came to remove a guided flow on
// no surface at all. Resolving through [surfaces.ExcludedToolSpecs] puts
// them under the one rule: a group name (gitlab_interactive), a tool name
// (gitlab_interactive_issue_create) and a canonical action ID
// (interactive.issue_create) remove the same tools everywhere.
//
// Three callers ask it: the pass over registered tools on the meta and
// individual surfaces, the warning about entries that named nothing, which
// must not report an entry a standalone utility answers, and the end-to-end
// harness, which has to expect what the binary serves rather than a copy of
// its rule. The resolution runs on every call, over a spec list built once
// per process (see standaloneSpecs).
//
// It panics when the specs cannot be assembled, which only a malformed
// declared spec can cause: those tools are part of the declared surface, and
// [RegisterSurfaceTools] stops startup over the same spec the same way.
func ExcludedStandaloneTools(excludeTools []string) (toolNames, unmatched []string) {
	excluded, unmatched, err := excludedStandaloneSpecs(standaloneSpecs(), excludeTools)
	if err != nil {
		panic(fmt.Errorf("resolve --exclude-tools against the standalone utilities: %w", err))
	}
	return slices.Sorted(maps.Keys(excluded)), unmatched
}

// standaloneSpecs is the standalone utilities' spec list, built once per
// process with the credential-less client a shared catalog is built with: an
// exclusion reads their names and IDs, and neither depends on a credential or
// on the instance class.
var standaloneSpecs = sync.OnceValue(func() []actioncatalog.SurfaceToolSpec {
	return StandaloneSurfaceToolSpecs(UnboundClient(false))
})

// excludedStandaloneSpecs is the resolver [ExcludedStandaloneTools] asks. The
// specs are compiled in, so nothing a deployment configures can make it
// fail, and the branch reporting that it did would otherwise never run.
var excludedStandaloneSpecs = surfaces.ExcludedToolSpecs // test seam

// RegisterSurfaceTools registers visible tools from canonical surface
// specs. Each spec is projected through [toolutil.RegisterSurfaceToolFromSpec]
// and panics on projection failure because surface tools are part of the
// declared MCP surface and a malformed spec should fail startup.
func RegisterSurfaceTools(server *mcp.Server, specs []actioncatalog.SurfaceToolSpec) {
	for _, spec := range specs {
		actionSpec, err := spec.ActionSpec()
		if err != nil {
			panic(fmt.Errorf("project surface tool %s: %w", spec.Name, err))
		}
		toolutil.RegisterSurfaceToolFromSpec(server, actionSpec, toolutil.SurfaceToolRegisterOptions{
			Icons:        spec.Icons,
			FormatResult: spec.FormatResult,
		})
	}
}
