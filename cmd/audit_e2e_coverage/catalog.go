package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/mcpsurface"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamiccatalog"
)

// catalogAction is one action of the catalog a runtime serves, with the name
// each surface calls it by.
type catalogAction struct {
	// id is the canonical catalog id.
	id string
	// domain is the catalog domain.
	domain string
	// tier is the minimum licensing tier the action needs.
	tier edition.Tier
	// readOnly is whether the action mutates nothing, which decides whether a
	// read-only session serves it.
	readOnly bool
	// destructive is whether the action needs an explicit confirmation.
	destructive bool
	// metaTool is the tool the meta surface registers the action on: the
	// domain group, or the standalone tool itself.
	metaTool string
	// individualTool is the tool the individual surface registers for the
	// action, empty when it registers none for it.
	individualTool string
	// individualOwner names the action that took this action's declared
	// individual tool name, when a sibling declared it first.
	individualOwner string
	// standalone is whether the action is one of the utilities outside the
	// domain groups, registered by name on meta and individual.
	standalone bool
}

// servedCatalog is the catalog a runtime serves, built the way the server
// builds it.
type servedCatalog struct {
	// tier is the tier the catalog was built at.
	tier edition.Tier
	// actions holds every action by id.
	actions map[string]catalogAction
	// ids lists the ids in sorted order, for stable output.
	ids []string
}

// catalogBuilder is how the command obtains a catalog, as a value so a test
// can hand the classification a catalog of a few actions instead of the
// compiled-in one.
type catalogBuilder func(tier edition.Tier) (*servedCatalog, error)

// buildServedCatalog builds the catalog the server assembles for the dynamic
// surface at tier, on a self-managed instance: the shared base catalog with
// the diagnostics group, plus the standalone utilities.
//
// It is the dynamic assembly rather than the base one because the dynamic
// surface is the default and the widest: the standalone actions are catalog
// actions there and named tools on the other two surfaces, and a report that
// took the base catalog would have no row for the elicitation flows at all.
// The client is mcpsurface's stub, so the build is offline and the same on
// every machine.
func buildServedCatalog(tier edition.Tier) (*servedCatalog, error) {
	client, cleanup := mcpsurface.NewStubClient()
	defer cleanup()
	catalog, _, err := dynamiccatalog.Build(client, &config.ServerConfig{Tier: tier})
	if err != nil {
		return nil, fmt.Errorf("build the catalog at tier %s: %w", tier, err)
	}
	return catalogFrom(catalog, tier), nil
}

// catalogFrom reads what each surface calls every action of a catalog.
//
// The individual name is taken from the identifier that registration uses
// rather than from the action's own declaration, because several actions
// declare one name and registration binds it to the first: the others are
// unservable on that surface, and a report that credited a call to their
// declared name would be crediting a different action.
func catalogFrom(catalog *actioncatalog.Catalog, tier edition.Tier) *servedCatalog {
	identify := tools.NewCallIdentifier(catalog, config.ToolSurfaceIndividual)
	served := &servedCatalog{tier: tier, actions: map[string]catalogAction{}}
	for _, group := range catalog.Groups() {
		standalone := group.SurfaceKind == actioncatalog.SurfaceKindRuntimeUtility ||
			group.SurfaceKind == actioncatalog.SurfaceKindInteractiveUtility
		for _, action := range group.ActionsInOrder() {
			entry := catalogAction{
				id:          string(action.ID),
				domain:      action.Domain,
				tier:        edition.TierFromEdition(action.Edition),
				readOnly:    action.ReadOnly,
				destructive: action.Destructive,
				metaTool:    action.ToolName,
				standalone:  standalone,
			}
			if name := strings.TrimSpace(action.IndividualTool.Name); name != "" {
				identity, known := identify.Identify(name, nil)
				switch {
				case known && identity.ActionID == entry.id:
					entry.individualTool = name
				case known:
					entry.individualOwner = identity.ActionID
				}
			}
			if standalone {
				// A standalone utility is registered under its own tool name
				// on meta as well as on individual; the group's tool name is
				// the dynamic surface's grouping and no session registers it.
				entry.metaTool = entry.individualTool
			}
			served.actions[entry.id] = entry
			served.ids = append(served.ids, entry.id)
		}
	}
	sort.Strings(served.ids)
	return served
}

// toolOn names the tool a surface registers for the action, and reports false
// when the surface cannot reach it at all.
func (a catalogAction) toolOn(surface string) (string, bool) {
	switch surface {
	case config.ToolSurfaceDynamic:
		return mcpsurface.DynamicExecuteActionToolName, true
	case config.ToolSurfaceMeta:
		return a.metaTool, a.metaTool != ""
	case config.ToolSurfaceIndividual:
		return a.individualTool, a.individualTool != ""
	default:
		return "", false
	}
}

// unservableReason says why a surface in a mode cannot serve the action, and
// returns the empty string when it can.
//
// served is the set of tools the sessions of that surface and mode listed.
// A tool missing from it is withheld, by the token's scopes or by the
// operator's exclusions, and the two are not told apart here: the session
// line records what was served and not why.
func (a catalogAction) unservableReason(surface, mode string, served map[string]bool) string {
	if mode == modeReadOnly && !a.readOnly {
		return "withheld by read-only mode"
	}
	tool, reachable := a.toolOn(surface)
	if !reachable {
		if surface == config.ToolSurfaceIndividual && a.individualOwner != "" {
			return "individual tool name is registered for " + a.individualOwner
		}
		return "no " + surface + " tool"
	}
	if !served[tool] {
		return "tool " + tool + " not served"
	}
	return ""
}
