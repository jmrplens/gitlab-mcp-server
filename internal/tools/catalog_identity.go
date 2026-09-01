package tools

import (
	"encoding/json"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/mcpotel"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
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
	return newCallIdentifier(catalog.Actions(), surface)
}

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
	switch surface {
	case config.ToolSurfaceIndividual:
		return individualIdentifier(actions)
	case config.ToolSurfaceMeta:
		return metaIdentifier(actions)
	default:
		return dynamicIdentifier(actions)
	}
}

// individualIdentifier resolves a declared tool name to its action.
//
// Nothing here decodes arguments, and that is worth stating: this is the
// surface with roughly a thousand tools, and its tools carry no action field at
// all, so a decode would be pure waste on every call.
func individualIdentifier(actions []actioncatalog.Action) mcpotel.CallIdentifier {
	byTool := make(map[string]mcpotel.Identity, len(actions))
	for _, action := range actions {
		if name := action.IndividualTool.Name; name != "" {
			byTool[name] = mcpotel.Identity{ActionID: string(action.ID), Domain: action.Domain}
		}
	}
	return mcpotel.IdentifierFunc(func(toolName string, _ any) (mcpotel.Identity, bool) {
		identity, ok := byTool[toolName]
		return identity, ok
	})
}

// metaIdentifier resolves a domain tool plus its action argument.
//
// The canonical id is the pair and neither half is enough: gitlab_issue says
// which domain, "list" says which operation, and only together do they name an
// action the catalog knows.
func metaIdentifier(actions []actioncatalog.Action) mcpotel.CallIdentifier {
	domains := make(map[string]string, len(actions))
	byID := make(map[string]mcpotel.Identity, len(actions))
	for _, action := range actions {
		if action.ToolName != "" && action.Domain != "" {
			domains[action.ToolName] = action.Domain
		}
		byID[string(action.ID)] = mcpotel.Identity{ActionID: string(action.ID), Domain: action.Domain}
	}

	return mcpotel.IdentifierFunc(func(toolName string, arguments any) (mcpotel.Identity, bool) {
		domain, known := domains[toolName]
		if !known {
			// A standalone tool: gitlab_discover_project, an interactive
			// elicitation flow. These belong to no catalog action, and saying
			// so is the right answer rather than a failure.
			return mcpotel.Identity{}, false
		}
		action := actionArgument(arguments)
		if action == "" {
			return mcpotel.Identity{Domain: domain}, true
		}
		if identity, found := byID[domain+"."+action]; found {
			return identity, true
		}
		// An action the catalog does not have, which happens whenever a model
		// invents one. The domain is still true and still worth recording, so
		// it is returned without an id rather than discarded.
		return mcpotel.Identity{Domain: domain}, true
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
