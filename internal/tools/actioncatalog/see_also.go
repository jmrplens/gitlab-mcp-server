package actioncatalog

import (
	"regexp"
	"strings"
)

// SeeAlsoClause matches the cross-reference sentence an individual tool's
// Description ends with, "See also: gitlab_a, gitlab_b.", and, because the
// character class admits dots, a clause written in canonical IDs or already
// projected into a surface's entry IDs ("See also: widget.create,
// gitlab_widget.get."). The terminating literal dot still matches: the greedy
// class backtracks one character off the final name. The first submatch is
// the comma-separated list of names.
//
// It is the one definition of the clause, because two readers have to agree
// on it and disagreeing fails silently. internal/resources rewrites what it
// matches into the names of the surface a manifest serves, and
// cmd/audit_action_ids passes over what it matches when it holds a
// Description to the rule that served prose names no tool, since that rewrite
// is what makes an individual tool name right there. With a copy in each, a
// manifest pattern narrowed later would stop rewriting text the gate still
// skipped, and tool names would reach the dynamic and meta surfaces verbatim
// with the gate green.
var SeeAlsoClause = regexp.MustCompile(`See also: ([a-z0-9_.]+(?:, [a-z0-9_.]+)*)\.`)

// SeeAlsoResolver maps a name a "See also:" clause spells, an
// individual-surface tool name or a canonical action ID, to the identifier
// the active surface's entries are invoked by, reporting whether the name is
// known. A nil resolver leaves descriptions untouched (the individual
// surface, whose namespace a domain action's clause already uses, and where
// a canonical ID resolves as it does on every surface).
type SeeAlsoResolver func(name string) (string, bool)

// ServedDescription is the description gitlab://tools serves for an action
// on a surface whose "See also:" names resolve through resolve: the
// individual tool's Description with its clause rewritten by
// [RewriteSeeAlso], or, for an action with no Description, its Usage line
// verbatim.
//
// It is the one definition of that text, because two readers have to agree
// on it: internal/resources serves it, and cmd/gen_tool_reference publishes
// it on the site as the description the dynamic surface serves. A copy in the
// generator had already come to rewrite a Usage line the server serves as it
// is, with nothing to notice, since its gate compares the generator with its
// own output.
func ServedDescription(action Action, resolve SeeAlsoResolver) string {
	description := action.IndividualTool.Description
	if description == "" {
		return action.Usage
	}
	return RewriteSeeAlso(description, resolve)
}

// RewriteSeeAlso projects the "See also:" clause of an individual-surface
// description into the active surface's identifier namespace.
//
// The specs hand-write these clauses once, in individual-tool names; on the
// dynamic and meta surfaces those names are not invocable, and the manifest
// instructions tell the model to pass entry IDs, so emitting the individual
// names there contradicts the same document two lines later. The standalone
// surface tools (the guided flows and project discovery) write theirs in
// canonical IDs instead, because their description reaches every surface's
// tools/list and find results verbatim, which this projection never sees. An
// ID is rewritten here like a tool name is, which only the dynamic manifest
// does to theirs, where an ID maps to itself: the meta and individual
// surfaces register those tools beside a catalog that does not carry them, so
// their manifests list them as direct entries and serve the description, the
// clause's canonical IDs included, verbatim, and gitlab://tools/{id} resolves
// those IDs on every surface.
//
// A name the resolver does not know is dropped, not passed through: on one
// instance the catalog is tier-filtered, so a Free-tier server legitimately
// cannot resolve a reference to a Premium action, and a name that resolves to
// nothing on the whole instance is not a reference, it is noise. Stale names
// cannot hide behind this: the guard test in internal/resources checks the
// hand-written clauses against the full unfiltered catalog, where only a
// genuinely wrong name fails. A clause left empty is removed whole, and the
// spaces and line breaks it leaves at the end of the text with it.
func RewriteSeeAlso(description string, resolve SeeAlsoResolver) string {
	if resolve == nil {
		return description
	}
	rewritten := SeeAlsoClause.ReplaceAllStringFunc(description, func(clause string) string {
		names := strings.Split(strings.TrimSuffix(strings.TrimPrefix(clause, "See also: "), "."), ", ")
		kept := names[:0]
		for _, name := range names {
			if id, ok := resolve(name); ok {
				kept = append(kept, id)
			}
		}
		if len(kept) == 0 {
			return ""
		}
		return "See also: " + strings.Join(kept, ", ") + "."
	})
	return strings.TrimRight(rewritten, " \n")
}
