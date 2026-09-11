package tools

import (
	"encoding/json"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/mcpotel"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// actionArgumentName is the JSON field both dispatching surfaces carry the
// operation in.
//
// One constant rather than two, because the dynamic and meta surfaces genuinely
// share it: dynamic's gitlab_execute_action takes {"action": "issue.list"} and a
// meta tool takes {"action": "list"}. What differs is what the value means,
// which is why the two resolvers below do different things with the same field.
const actionArgumentName = "action"

// NewCallIdentifier builds the resolver that tells instrumentation what a
// tools/call actually invokes, for the surface this process registered.
//
// The tool name is the operation on exactly one surface, and on that one it is
// declared rather than derived:
//
//	surface     tool                    action argument   canonical id
//	dynamic     gitlab_execute_action   issue.list        the argument, resolved
//	meta        gitlab_issue            list              domain + "." + argument
//	individual  gitlab_issue_list       absent            declared, so looked up
//
// The individual row is what forces a catalog. Tool names there come from each
// ActionSpec, with a large legacy verb-first set (gitlab_list_issue_discussions,
// gitlab_add_ssh_key) beside newer domain-first ones, so no string
// transformation recovers the action. CLAUDE.md states it directly: never infer
// one from a formula.
//
// # Why the surface is a parameter
//
// It is decided before the process starts and cannot change while it runs, so
// there is nothing to discover per request. An earlier version ignored it and
// tried each shape in turn: individual lookup, then decode, then meta, then
// dynamic. That worked, and it was dishonest in two ways. It built three maps
// where one is needed, keeping roughly a thousand entries alive twice over for
// nothing. And a fallback chain reads as though it were resolving an ambiguity,
// which invites the next reader to work out what happens when two surfaces
// claim one name, when in fact only one surface is ever registered.
func NewCallIdentifier(catalog *actioncatalog.Catalog, surface string) mcpotel.CallIdentifier {
	if catalog == nil {
		return mcpotel.IdentifierFunc(noAction)
	}
	origin := catalog.SharedOrigin()
	if origin == nil {
		return newCallIdentifier(identifierActions(catalog, surface), surface)
	}
	// The identifier reads names and IDs only, so every server bound to one
	// shared catalog can use one; built from the origin, whose actions carry
	// the same names, so the maps hold nothing bound to a credential.
	key := identifierKey{origin: origin, surface: surface}
	return sharedIdentifiers.Load(key, func() mcpotel.CallIdentifier {
		return newCallIdentifier(identifierActions(origin, surface), surface)
	})
}

// identifierActions lists a catalog's actions in the order the surface's
// resolver reads them.
//
// Only the individual surface cares. Several actions can declare one
// individual tool name, registration binds it to the first of them in
// [individualRegistrationOrder], and the resolver must name that same action.
// It read them sorted by canonical ID and kept the last, which put every call to
// gitlab_commit_list under repository.file_history and every call to
// gitlab_user_current under user.me. The meta and dynamic resolvers key on
// canonical IDs, which are unique, so they keep the catalog's own order.
func identifierActions(catalog *actioncatalog.Catalog, surface string) []actioncatalog.Action {
	if surface == config.ToolSurfaceIndividual {
		return individualRegistrationOrder(catalog)
	}
	return catalog.Actions()
}

// identifierKey names one shared identifier: the shared catalog it resolves
// against and the surface that decides how.
type identifierKey struct {
	origin  *actioncatalog.Catalog
	surface string
}

// sharedIdentifiers holds one identifier per shared catalog and surface.
// Single-flight, so a startup burst of servers for one configuration indexes
// the actions once rather than once each.
var sharedIdentifiers toolutil.OnceMap[identifierKey, mcpotel.CallIdentifier]

// newCallIdentifier builds the resolver from a plain slice of actions.
//
// Split from [NewCallIdentifier] so a test can feed it a catalog that does not
// exist. The alias guard in the dynamic branch is the reason: no action in
// today's catalog declares an alias colliding with another action's canonical
// id, so a test against the real catalog exercises it vacuously and would keep
// passing after the guard was deleted.
//
// An unrecognized surface resolves as dynamic, matching the server's own
// default rather than inventing a fourth behavior for a value that cannot
// reach here from configuration.
func newCallIdentifier(actions []actioncatalog.Action, surface string) mcpotel.CallIdentifier {
	routes := newMetaRoutes(actions)
	var identify mcpotel.CallIdentifier
	switch surface {
	case config.ToolSurfaceIndividual:
		identify = individualIdentifier(actions)
	case config.ToolSurfaceMeta:
		identify = metaIdentifier(routes)
	default:
		identify = dynamicIdentifier(actions)
	}
	return catalogIdentifier{identify: identify, dispatch: routes}
}

// catalogIdentifier is the resolver [NewCallIdentifier] returns: the surface's
// own reading of a call's arguments, and the meta reading of a route a
// dispatcher reports through [mcpotel.RecordDispatch].
//
// Every surface carries the second, not only meta, because the dynamic surface
// reaches its actions through the meta handlers: gitlab_execute_action resolves
// issue.get and enters gitlab_issue's handler, which is where every rewrite
// happens and so where the route that ran is known.
type catalogIdentifier struct {
	identify mcpotel.CallIdentifier
	dispatch metaRoutes
}

// Identify reads a call the way its surface spells it.
func (c catalogIdentifier) Identify(toolName string, arguments any) (mcpotel.Identity, bool) {
	return c.identify.Identify(toolName, arguments)
}

// IdentifyDispatch names the catalog action a meta handler dispatched.
func (c catalogIdentifier) IdentifyDispatch(tool, action string) (mcpotel.Identity, bool) {
	return c.dispatch.identify(tool, action)
}

// metaRoutes maps a meta tool and one of its action names to the catalog
// action, which is how both a meta call and a meta handler's dispatch name one.
//
// The canonical id is the pair and neither half is enough: gitlab_issue says
// which domain, "list" says which operation, and only together do they name an
// action the catalog knows.
type metaRoutes struct {
	domains map[string]string
	byID    map[string]mcpotel.Identity
}

// newMetaRoutes indexes actions by their meta tool and canonical id.
func newMetaRoutes(actions []actioncatalog.Action) metaRoutes {
	routes := metaRoutes{
		domains: make(map[string]string, len(actions)),
		byID:    make(map[string]mcpotel.Identity, len(actions)),
	}
	for _, action := range actions {
		if action.ToolName != "" && action.Domain != "" {
			routes.domains[action.ToolName] = action.Domain
		}
		routes.byID[string(action.ID)] = mcpotel.Identity{ActionID: string(action.ID), Domain: action.Domain}
	}
	return routes
}

// identify resolves tool and action. A tool the catalog does not know is a
// standalone tool such as gitlab_discover_project or an interactive elicitation
// flow, which belongs to no catalog action. An action the domain does not have,
// which happens whenever a model invents one, still names the domain.
func (m metaRoutes) identify(tool, action string) (mcpotel.Identity, bool) {
	domain, known := m.domains[tool]
	if !known {
		return mcpotel.Identity{}, false
	}
	if action == "" {
		return mcpotel.Identity{Domain: domain}, true
	}
	if identity, found := m.byID[domain+"."+action]; found {
		return identity, true
	}
	return mcpotel.Identity{Domain: domain}, true
}

// individualIdentifier resolves a declared tool name to its action.
//
// Nothing here decodes arguments, and that is worth stating: this is the
// surface with roughly a thousand tools, and its tools carry no action field at
// all, so a decode would be pure waste on every call.
//
// The first action to declare a name keeps it, and the name is trimmed, both
// because that is what registration does with the same field: the actions
// arrive in registration order, and a name two of them declare belongs to the
// one whose handler was registered.
func individualIdentifier(actions []actioncatalog.Action) mcpotel.CallIdentifier {
	byTool := make(map[string]mcpotel.Identity, len(actions))
	for _, action := range actions {
		name := strings.TrimSpace(action.IndividualTool.Name)
		if name == "" {
			continue
		}
		if _, taken := byTool[name]; taken {
			continue
		}
		byTool[name] = mcpotel.Identity{ActionID: string(action.ID), Domain: action.Domain}
	}
	return mcpotel.IdentifierFunc(func(toolName string, _ any) (mcpotel.Identity, bool) {
		identity, ok := byTool[toolName]
		return identity, ok
	})
}

// metaIdentifier resolves a domain tool plus its action argument, as the
// client sent it. The action that ran can differ once dispatch rewrites it,
// and [catalogIdentifier.IdentifyDispatch] names that one.
func metaIdentifier(routes metaRoutes) mcpotel.CallIdentifier {
	return mcpotel.IdentifierFunc(func(toolName string, arguments any) (mcpotel.Identity, bool) {
		return routes.identify(toolName, actionArgument(arguments))
	})
}

// dynamicIdentifier resolves the canonical id, or an alias for one, straight
// out of the action argument.
//
// Aliases are included because gitlab_execute_action accepts them, and a
// resolver that understood only canonical ids would silently drop exactly the
// calls where a model reached for a name that used to be right. Those are the
// ones worth seeing in a trace.
func dynamicIdentifier(actions []actioncatalog.Action) mcpotel.CallIdentifier {
	byID := make(map[string]mcpotel.Identity, len(actions))
	for _, action := range actions {
		byID[string(action.ID)] = mcpotel.Identity{ActionID: string(action.ID), Domain: action.Domain}
	}
	// Aliases in a second pass, and added only when unclaimed. A canonical id
	// must never be shadowed by another action's alias; the catalog does not
	// forbid that collision, and in one pass the iteration order would decide
	// which one wins, which is a coin flip dressed as behavior.
	for _, action := range actions {
		identity := mcpotel.Identity{ActionID: string(action.ID), Domain: action.Domain}
		for _, alias := range action.Aliases {
			if _, taken := byID[alias]; !taken {
				byID[alias] = identity
			}
		}
	}

	return mcpotel.IdentifierFunc(func(_ string, arguments any) (mcpotel.Identity, bool) {
		action := actionArgument(arguments)
		if action == "" {
			return mcpotel.Identity{}, false
		}
		// Lowered like dynamic dispatch lowers before executing, or an
		// uppercase action id runs fine and loses its action attribute.
		identity, ok := byID[strings.ToLower(action)]
		return identity, ok
	})
}

// noAction is the resolver for a process with no catalog.
//
// It exists so that forgetting to wire one costs the action attribute rather
// than the process: this runs on every tool call, and a nil dereference there
// would be a crash on the happy path.
func noAction(string, any) (mcpotel.Identity, bool) { return mcpotel.Identity{}, false }

// actionArgument reads the operation out of a tool call's arguments.
//
// Two shapes, and the first is the one that matters. A call off the wire
// arrives as CallToolParamsRaw, whose Arguments field is json.RawMessage: the
// SDK deliberately leaves decoding to the tool handler. A reader that only
// understood map[string]any would compile, run, and find nothing on every real
// request. The map form is kept because an in-process caller, including this
// repository's own e2e suite, can build CallToolParams directly.
func actionArgument(arguments any) string {
	switch args := arguments.(type) {
	case json.RawMessage:
		return actionFromJSON(args)
	case []byte:
		return actionFromJSON(args)
	case map[string]any:
		value, ok := args[actionArgumentName].(string)
		if !ok {
			return ""
		}
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

// actionFromJSON pulls one field out of a raw argument blob.
//
// Decoding into a single-field struct rather than a map is what keeps this
// affordable: encoding/json skips every other key without allocating for it, so
// a tool call carrying a large body costs one string. A decode failure is not
// reported anywhere, because malformed arguments are the handler's business to
// reject and telemetry has no standing to complain about them first.
func actionFromJSON(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var envelope struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return ""
	}
	return strings.TrimSpace(envelope.Action)
}
