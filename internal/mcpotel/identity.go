package mcpotel

// CallIdentifier answers what a tools/call actually invokes.
//
// # Why this is an interface rather than code in this package
//
// The tool name is not the operation on two of the three surfaces, and on the
// third it is not derivable from anything:
//
//   - dynamic (the default): the tool is gitlab_execute_action for every call,
//     and the operation lives in the "action" argument as a canonical catalog
//     id such as issue.list. Two tool names cover roughly a thousand
//     operations, so a trace keyed on the tool name records nothing about what
//     the server did.
//   - meta: the tool is the bare domain, gitlab_issue, and the operation is the
//     "action" argument, list. The canonical id is the pair, and neither half
//     is enough.
//   - individual: the tool is gitlab_issue_list and there is no action
//     argument, but the name is DECLARED in each ActionSpec rather than
//     derived. A large legacy set is verb-first (gitlab_list_issue_discussions)
//     while new ones are domain-first, so no formula maps a tool name back to
//     an action. Only the catalog knows.
//
// Instrumentation that tried to work this out from the arguments would be a
// second copy of dispatch, drifting from the first the moment a surface
// changes. So this package asks, and the composition root answers from the
// catalog it already built. That also keeps the import direction clean: nothing
// here knows about the catalog, and the catalog knows nothing about telemetry.
//
// # Why the identity has to be known before the span starts
//
// The span name and its attributes are fixed at creation. "Samplers can only
// consider information already present during span creation. Any changes done
// later, including updated span name, cannot change their decisions." So the
// call is identified first and the span started second, rather than starting a
// span and renaming it once dispatch has worked out what it is.
type CallIdentifier interface {
	// Identify maps a tools/call to its canonical catalog action.
	//
	// arguments is the raw tool argument value, exactly as the SDK decoded it,
	// because the shape differs per surface and only the implementation knows
	// which field to read.
	//
	// Returning false is normal rather than exceptional: a standalone tool such
	// as gitlab_discover_project or an interactive elicitation flow belongs to
	// no catalog action, and a call naming a tool that does not exist reaches
	// here before anything rejects it. Neither is worth an error, and both must
	// leave the attribute unset rather than carry a placeholder.
	Identify(toolName string, arguments any) (Identity, bool)
}

// Identity is what a call turned out to be.
type Identity struct {
	// ActionID is the canonical catalog id, such as issue.list. Empty when the
	// call is not a catalog action.
	ActionID string

	// Domain is the catalog domain, such as issue. It is carried separately
	// because it is the dimension worth grouping by when the action id itself
	// is too many values to put on a metric.
	Domain string
}

// identifierFunc adapts a plain function to [CallIdentifier], so a caller with
// one closure does not need a type.
type identifierFunc func(toolName string, arguments any) (Identity, bool)

func (f identifierFunc) Identify(toolName string, arguments any) (Identity, bool) {
	return f(toolName, arguments)
}

// IdentifierFunc wraps a function as a [CallIdentifier].
func IdentifierFunc(f func(toolName string, arguments any) (Identity, bool)) CallIdentifier {
	return identifierFunc(f)
}

// noIdentity is the fallback when no identifier was configured.
//
// It exists so the middleware needs no nil check on a per-request path, and so
// that forgetting to wire an identifier degrades to spans without an action
// attribute rather than to a panic on the first tool call.
type noIdentity struct{}

func (noIdentity) Identify(string, any) (Identity, bool) { return Identity{}, false }
