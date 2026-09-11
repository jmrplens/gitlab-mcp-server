//go:build e2e

// project.go turns one canonical action ID into the call each surface spells
// for it.
//
// A test names an action once, as the catalog names it, and the harness works
// out what to send: gitlab_execute_action with the ID on the dynamic surface,
// the domain tool with the operation in its action argument on meta, and the
// declared tool name on individual. That last one is why this file reads the
// catalog rather than a formula: an individual tool name is declared per
// ActionSpec, a large legacy set is verb-first, and no string transformation
// recovers one from an ID.
//
// It also answers which actions a surface cannot reach at all, which matters
// more than it sounds. Several actions declare one individual tool name, and
// registration binds that name to the first of them; the others are then
// unservable on that surface, and a test that called the name anyway would
// exercise a different action and pass. The same goes for an action declaring
// no individual tool at all.

package harness

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamic"
)

// ActionID is a canonical catalog action identifier, such as issue.list.
//
// It is a named type rather than a string so that the push-time static gate
// can find every ID a test names, follow it through helper parameters, and
// check it against the catalog. A bare string literal would be invisible to
// that gate, which is the whole reason a test never writes a tool name.
type ActionID string

// String returns the identifier as the catalog spells it.
func (id ActionID) String() string { return string(id) }

// Surface is one of the three tool surfaces the binary can serve.
type Surface string

// The three surfaces, spelled as the binary's own GITLAB_MCP_TOOL_SURFACE
// values so that a session's configuration is the variable it sets.
const (
	// SurfaceDynamic is the default two-tool find and execute surface.
	SurfaceDynamic Surface = config.ToolSurfaceDynamic
	// SurfaceMeta is the domain dispatcher surface.
	SurfaceMeta Surface = config.ToolSurfaceMeta
	// SurfaceIndividual is one registered tool per action.
	SurfaceIndividual Surface = config.ToolSurfaceIndividual
)

// String returns the surface as the environment variable spells it.
func (s Surface) String() string { return string(s) }

// AllSurfaces lists the three surfaces in the order a subtest runs them:
// dynamic first, because it is what a client gets by default and so the one a
// failure matters most on.
func AllSurfaces() []Surface {
	return []Surface{SurfaceDynamic, SurfaceMeta, SurfaceIndividual}
}

// projectedAction is everything the harness needs to call one catalog action
// on any surface, plus what it needs to refuse the call before sending it.
type projectedAction struct {
	// id is the canonical catalog identifier.
	id ActionID
	// domain is the catalog domain, the half of the ID before the dot.
	domain string
	// metaTool is the domain tool the meta surface registers.
	metaTool string
	// metaAction is the operation name that tool's action argument takes.
	metaAction string
	// individualTool is the declared individual tool name, empty when this
	// action cannot be reached on that surface.
	individualTool string
	// individualOwner names the action the declared name is bound to when it
	// is bound to another one, so a refusal can say who took it.
	individualOwner string
	// destructive says the action asks for a confirmation before it runs.
	destructive bool
	// readOnly is the action's own classification, before the individual
	// surface's narrowing overrides.
	readOnly bool
	// minimumTier is the licensing tier the action needs, read from the
	// catalog's Edition annotation.
	minimumTier edition.Tier
}

// projection is one instance's reading of the catalog: every action it serves,
// projected onto the three surfaces.
type projection struct {
	tier    edition.Tier
	actions map[ActionID]projectedAction
}

// projectionKey names one built projection. The tier decides which actions
// exist and how their schemas are pruned; the instance class decides whether
// the GitLab.com-only actions are among them.
type projectionKey struct {
	tier   edition.Tier
	dotcom bool
}

// projections holds one projection per key. Building it walks the whole
// catalog, which is the expensive part of a session start, and every session
// of one run asks for the same one.
var projections sync.Map // projectionKey -> *projectionEntry

// projectionEntry is one cached projection, built by the first caller while
// every concurrent caller for the same key waits.
type projectionEntry struct {
	once sync.Once
	made *projection
	err  error
}

// newProjection returns the projection for a tier and instance class, building
// it on first use from the same shared base catalog the binary registers from.
func newProjection(tier edition.Tier, dotcom bool) (*projection, error) {
	loaded, _ := projections.LoadOrStore(projectionKey{tier: tier, dotcom: dotcom}, &projectionEntry{})
	entry, _ := loaded.(*projectionEntry)
	entry.once.Do(func() {
		entry.made, entry.err = buildProjection(tier, dotcom)
	})
	return entry.made, entry.err
}

// buildProjection reads the catalog once and records what each surface calls
// every action.
//
// IncludeMCP is set because the binary sets it: without it the gitlab_server
// diagnostics group is absent, and a test naming server.status would be told
// the catalog has no such action when the server serves it.
func buildProjection(tier edition.Tier, dotcom bool) (*projection, error) {
	catalog, err := gitlabtools.SharedBaseCatalog(dotcom, gitlabtools.ActionCatalogOptions{Tier: tier, IncludeMCP: true})
	if err != nil {
		return nil, fmt.Errorf("build the base catalog at tier %s: %w", tier, err)
	}

	// The identifier is the one reader that knows which action a declared
	// individual tool name is registered for. Asking it, rather than trusting
	// the name on each action, is what makes a shadowed sibling unservable
	// here instead of silently calling somebody else's handler.
	identify := gitlabtools.NewCallIdentifier(catalog, config.ToolSurfaceIndividual)

	made := &projection{tier: tier, actions: make(map[ActionID]projectedAction, catalog.CountActions())}
	for _, action := range catalog.Actions() {
		projected := projectedAction{
			id:          ActionID(action.ID),
			domain:      action.Domain,
			metaTool:    action.ToolName,
			metaAction:  action.Name,
			destructive: action.Destructive,
			readOnly:    action.ReadOnly,
			minimumTier: edition.TierFromEdition(action.Edition),
		}
		if name := strings.TrimSpace(action.IndividualTool.Name); name != "" {
			if identity, known := identify.Identify(name, nil); known && identity.ActionID == string(action.ID) {
				projected.individualTool = name
			} else if known {
				projected.individualOwner = identity.ActionID
			}
		}
		made.actions[projected.id] = projected
	}
	return made, nil
}

// lookup returns the projection of one action, and whether the catalog this
// run serves has it at all.
func (p *projection) lookup(id ActionID) (projectedAction, bool) {
	action, found := p.actions[id]
	return action, found
}

// toolCall is one tools/call: the tool to name and the arguments to send.
type toolCall struct {
	// tool is the registered tool name.
	tool string
	// arguments is the argument object, as the surface's own tool takes it.
	arguments map[string]any
	// argumentNames are the top-level argument names, for the record. The
	// values are deliberately left out of it: a value is a fixture, a name is
	// part of the call.
	argumentNames []string
}

// confirmArgument is the field every surface reads an explicit approval of a
// destructive action from. On dynamic it is top level, beside the action and
// its params; on the other two it travels with the action's own parameters.
const confirmArgument = "confirm"

// callOn spells the call this action takes on one surface.
//
// A destructive action always carries the confirmation, because the harness is
// not a person and the server fails closed without one: the alternative is
// GITLAB_MCP_YOLO_MODE in the child, which the launcher refuses to pass for
// the reason stated there. A test that wants to see the refusal asks for it
// explicitly instead, through Refused.
func (a projectedAction) callOn(surface Surface, params map[string]any, confirm bool) (toolCall, error) {
	arguments := make(map[string]any, len(params)+1)
	maps.Copy(arguments, params)
	if confirm && a.destructive {
		arguments[confirmArgument] = true
	}

	switch surface {
	case SurfaceDynamic:
		call := map[string]any{"action": string(a.id), "params": paramsObject(params)}
		if confirm && a.destructive {
			// Top level here, and only here: the execute tool's own schema
			// says so, and a confirm inside params is read as a parameter of
			// the action rather than as an approval.
			call[confirmArgument] = true
		}
		return toolCall{tool: dynamictools.ExecuteActionToolName, arguments: call, argumentNames: argumentNames(call)}, nil
	case SurfaceMeta:
		if a.metaTool == "" || a.metaAction == "" {
			return toolCall{}, fmt.Errorf("action %s declares no meta tool and action pair", a.id)
		}
		call := map[string]any{"action": a.metaAction}
		if len(arguments) > 0 {
			call["params"] = arguments
		}
		return toolCall{tool: a.metaTool, arguments: call, argumentNames: argumentNames(call)}, nil
	case SurfaceIndividual:
		if a.individualTool == "" {
			return toolCall{}, a.unservableOnIndividual()
		}
		return toolCall{tool: a.individualTool, arguments: arguments, argumentNames: argumentNames(arguments)}, nil
	default:
		return toolCall{}, fmt.Errorf("unknown surface %q", surface)
	}
}

// unservableOnIndividual says why this action cannot be called on the
// individual surface, naming the sibling that took its tool name when one did.
func (a projectedAction) unservableOnIndividual() error {
	if a.individualOwner != "" {
		return fmt.Errorf("action %s is not servable on the individual surface: the tool name it declares is "+
			"registered for %s, which is the first action to declare it", a.id, a.individualOwner)
	}
	return fmt.Errorf("action %s is not servable on the individual surface: it declares no individual tool", a.id)
}

// paramsObject returns the params object the dynamic execute tool takes, which
// is required and must be an object even when the action needs nothing.
func paramsObject(params map[string]any) map[string]any {
	if params == nil {
		return map[string]any{}
	}
	return params
}

// argumentNames returns the sorted top-level argument names of a call.
func argumentNames(arguments map[string]any) []string {
	return slices.Sorted(maps.Keys(arguments))
}
