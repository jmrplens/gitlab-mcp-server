package tools

import (
	"encoding/json"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/mcpotel"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
)

// actionArgumentName is the JSON field both dispatching surfaces carry the
// operation in.
//
// It is one constant rather than two because the dynamic and meta surfaces
// genuinely share it: dynamic's gitlab_execute_action takes {"action":
// "issue.list"}, and a meta tool takes {"action": "list"}. What differs is what
// the value means, which is why the two branches below do different things with
// the same field.
const actionArgumentName = "action"

// NewCallIdentifier builds the resolver that tells instrumentation what a
// tools/call actually invokes.
//
// It lives here, beside the catalog, because nothing else can answer the
// question. The tool name maps to an operation differently on each surface, and
// on the individual surface not at all without a lookup:
//
//	surface     tool                    action argument   canonical id
//	dynamic     gitlab_execute_action   issue.list        the argument, resolved
//	meta        gitlab_issue            list              domain + "." + argument
//	individual  gitlab_issue_list       absent            declared, so looked up
//
// The individual row is the one that forces a catalog. Tool names there are
// declared in each ActionSpec rather than derived: a large legacy set is
// verb-first (gitlab_list_issue_discussions, gitlab_add_ssh_key) while newer
// ones are domain-first, so no string transformation recovers the action from
// the name. CLAUDE.md states the rule directly, "never infer one from a
// formula", and this is what that rule costs.
//
// Building all three maps at once, rather than one per configured surface,
// keeps the resolver correct for a process that registers a surface this
// function was not told about, and costs a few thousand map entries once at
// startup.
func NewCallIdentifier(catalog *actioncatalog.Catalog) mcpotel.CallIdentifier {
	if catalog == nil {
		return mcpotel.IdentifierFunc(func(string, any) (mcpotel.Identity, bool) {
			return mcpotel.Identity{}, false
		})
	}
	return newCallIdentifier(catalog.Actions())
}

// newCallIdentifier builds the resolver from a plain slice of actions.
//
// Split out from [NewCallIdentifier] so a test can feed it a catalog that does
// not exist yet. The alias guard below is the reason: no action in today's
// catalog declares an alias that collides with another action's canonical id,
// so a test against the real catalog exercises the guard vacuously and would go
// on passing after the guard was deleted.
func newCallIdentifier(actions []actioncatalog.Action) mcpotel.CallIdentifier {
	// individual: the declared tool name is the whole key.
	byIndividualTool := make(map[string]mcpotel.Identity)
	// meta: the group's tool name plus the action argument.
	metaDomains := make(map[string]string)
	// dynamic: the canonical id, including every alias that resolves to it,
	// because gitlab_execute_action accepts compatibility aliases and a trace
	// that only understood canonical ids would lose exactly the calls a model
	// got slightly wrong.
	byActionID := make(map[string]mcpotel.Identity)

	for _, action := range actions {
		identity := mcpotel.Identity{ActionID: string(action.ID), Domain: action.Domain}

		if name := action.IndividualTool.Name; name != "" {
			byIndividualTool[name] = identity
		}
		if action.ToolName != "" && action.Domain != "" {
			metaDomains[action.ToolName] = action.Domain
		}
		byActionID[string(action.ID)] = identity
		for _, alias := range action.Aliases {
			// Aliases are added only when unclaimed. A canonical id must never
			// be shadowed by another action's alias, and the catalog does not
			// forbid the collision, so the order of Actions() would otherwise
			// decide which one wins.
			if _, taken := byActionID[alias]; !taken {
				byActionID[alias] = identity
			}
		}
	}

	return mcpotel.IdentifierFunc(func(toolName string, arguments any) (mcpotel.Identity, bool) {
		if identity, ok := byIndividualTool[toolName]; ok {
			return identity, true
		}

		action := actionArgument(arguments)
		if action == "" {
			// A standalone tool: gitlab_discover_project, gitlab_server, an
			// interactive elicitation flow. These belong to no catalog action,
			// and saying so is the right answer rather than a failure.
			return mcpotel.Identity{}, false
		}

		// meta: the group supplies the domain the bare action name lacks.
		if domain, ok := metaDomains[toolName]; ok {
			if identity, found := byActionID[domain+"."+action]; found {
				return identity, true
			}
			// The action is unknown to the catalog, which happens whenever a
			// model invents one. The domain is still true and still worth
			// having, so it is returned without an id rather than discarded.
			return mcpotel.Identity{Domain: domain}, true
		}

		// dynamic: the argument is already a canonical id, or an alias for one.
		if identity, ok := byActionID[action]; ok {
			return identity, true
		}
		return mcpotel.Identity{}, false
	})
}

// actionArgument reads the operation out of a tool call's arguments.
//
// Two shapes, and the first one is the one that matters. A call that came off
// the wire arrives as CallToolParamsRaw, whose Arguments field is
// json.RawMessage: the SDK deliberately leaves decoding to the tool handler.
// So a reader that only understood map[string]any would compile, run, and find
// nothing on every real request, recording no action for the two surfaces that
// need it most. The map form is kept because an in-process caller, including
// this repository's own e2e suite, can build CallToolParams directly.
//
// Anything else yields the empty string rather than reflection. This runs on
// every tool call, and "no action attribute" is a far better outcome than a
// slow path or a panic.
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
// affordable: encoding/json skips every other key without allocating for it,
// so a tool call carrying a large body costs one string. A decode failure is
// not reported anywhere, because malformed arguments are the handler's business
// to reject and telemetry has no standing to complain about them first.
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
