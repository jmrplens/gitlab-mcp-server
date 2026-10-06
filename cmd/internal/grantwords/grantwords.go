package grantwords

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// Words is the wording of one language: the two column headings a table of
// requirements carries, and every phrase a requirement is written in.
type Words struct {
	// Needs and ServedEmpty head the columns of what an action needs and of
	// the parts of its answer a fine-grained token is served empty.
	Needs       string
	ServedEmpty string

	// notReachable opens the entry of an action no fine-grained token runs.
	notReachable string
	// oneOf opens the entry of an action with several ways of running.
	oneOf string
	// or separates ways, and the denials of the ways no token takes.
	or string
	// deniedWays leads into the ways no fine-grained token takes, after what
	// the others need.
	deniedWays string
	// notJudged is a request GitLab leaves out of the grant check.
	notJudged string
	// noPermission is a way that needs nothing.
	noPermission string
	// at joins a need's permissions to its boundaries, and boundaryOr joins
	// the boundaries.
	at         string
	boundaryOr string
	// boundaries are the words a boundary is written in where they are not
	// GitLab's own; a boundary it does not hold is written as GitLab names it.
	boundaries map[string]string
	// always and without close a part of the answer served empty: always, or
	// unless the grant holds what follows without.
	always  string
	without string

	// The denials, one format per cause; each %s is the element GitLab
	// names, and unresolvable's second %s is what GitLab does about it.
	mutationUndeclared string
	typeUndeclared     string
	payloadUndeclared  string
	unresolvable       string
	restTodo           string
	undeclared         string
	// The effects of a boundary that never resolves.
	effectRefused   string
	effectCommitted string
	effectNull      string
}

// English is the English wording.
func English() *Words {
	return &Words{
		Needs:              "Needs",
		ServedEmpty:        "Served empty",
		notReachable:       "Not reachable at this release: ",
		oneOf:              "one of: ",
		or:                 " **or** ",
		deniedWays:         ". With an input that sends another request instead, not reachable at this release: ",
		notJudged:          "a request GitLab does not judge by the grant",
		noPermission:       "no permission",
		at:                 " at ",
		boundaryOr:         " or ",
		always:             " (always)",
		without:            " (without ",
		mutationUndeclared: "GitLab declares no fine-grained permission for the mutation %s, and refuses it",
		typeUndeclared:     "GitLab declares no fine-grained permission for %s, which the answer is made of",
		payloadUndeclared:  "GitLab declares no fine-grained permission for %s in the answer, so the write commits and its answer is lost",
		unresolvable:       "%s is declared at a boundary the object the action reaches never resolves to, %s",
		restTodo:           "GitLab marks %s as not yet supported for fine-grained tokens",
		undeclared:         "GitLab declares no fine-grained permission for %s",
		effectRefused:      "so GitLab refuses the write before it runs",
		effectCommitted:    "so the write commits and its answer is lost",
		effectNull:         "so GitLab answers it as null or leaves it out of the list",
	}
}

// Spanish is the Spanish wording. A boundary is written the way the Spanish
// fine-grained tokens guide names it.
func Spanish() *Words {
	return &Words{
		Needs:        "Necesita",
		ServedEmpty:  "Se sirve vacío",
		notReachable: "No alcanzable en esta versión: ",
		oneOf:        "una de: ",
		or:           " **o** ",
		deniedWays:   ". Con una entrada que envía otra petición en su lugar, no alcanzable en esta versión: ",
		notJudged:    "una petición que GitLab no juzga por la concesión",
		noPermission: "ningún permiso",
		at:           " en ",
		boundaryOr:   " o ",
		boundaries: map[string]string{
			"project":  "proyecto",
			"group":    "grupo",
			"user":     "usuario",
			"instance": "instancia",
		},
		always:             " (siempre)",
		without:            " (sin ",
		mutationUndeclared: "GitLab no declara ningún permiso de grano fino para la mutación %s, y la rechaza",
		typeUndeclared:     "GitLab no declara ningún permiso de grano fino para %s, del que está hecha la respuesta",
		payloadUndeclared:  "GitLab no declara ningún permiso de grano fino para %s en la respuesta, así que la escritura se confirma y su respuesta se pierde",
		unresolvable:       "%s se declara en un ámbito al que nunca se resuelve el objeto que alcanza la acción, %s",
		restTodo:           "GitLab marca %s como aún no admitido para tokens de grano fino",
		undeclared:         "GitLab no declara ningún permiso de grano fino para %s",
		effectRefused:      "así que GitLab rechaza la escritura antes de que se ejecute",
		effectCommitted:    "así que la escritura se confirma y su respuesta se pierde",
		effectNull:         "así que GitLab lo responde como null o lo deja fuera de la lista",
	}
}

// NeedsText is what the action needs, way by way, and the ways no
// fine-grained token takes when some other way runs.
func (w *Words) NeedsText(description *finegrained.Description) string {
	if description.Denied != nil {
		return w.notReachable + w.denialText(description.Denied)
	}
	ways := make([]string, len(description.AnyOf))
	for i, way := range description.AnyOf {
		ways[i] = w.wayText(way)
	}
	text := strings.Join(ways, w.or)
	if len(ways) > 1 {
		text = w.oneOf + text
	}
	if len(description.DeniedWays) == 0 {
		return text
	}
	reasons := make([]string, len(description.DeniedWays))
	for i := range description.DeniedWays {
		reasons[i] = w.denialText(&description.DeniedWays[i])
	}
	return text + w.deniedWays + strings.Join(reasons, w.or)
}

// wayText is what one way of running the action needs.
func (w *Words) wayText(way finegrained.Way) string {
	parts := make([]string, 0, len(way.Needs)+1)
	for _, need := range way.Needs {
		parts = append(parts, w.needText(need))
	}
	if way.NotJudged {
		parts = append(parts, w.notJudged)
	}
	if len(parts) == 0 {
		return w.noPermission
	}
	return strings.Join(parts, "; ")
}

// needText is one need: every permission, at one of the boundaries.
func (w *Words) needText(need finegrained.Need) string {
	boundaries := make([]string, len(need.At))
	for i, boundary := range need.At {
		boundaries[i] = w.boundary(boundary)
	}
	return strings.Join(need.Permissions, ", ") + w.at + strings.Join(boundaries, w.boundaryOr)
}

// boundary is one boundary in the language's words.
func (w *Words) boundary(name string) string {
	if word, ok := w.boundaries[name]; ok {
		return word
	}
	return name
}

// ServedEmptyText names the parts of the answer a fine-grained token may be
// served empty, or is empty when there is none.
func (w *Words) ServedEmptyText(description *finegrained.Description) string {
	var parts []string
	for _, selection := range description.AlwaysEmpty {
		parts = append(parts, "`"+selection+"`"+w.always)
	}
	for _, position := range description.EmptyWithout {
		needs := make([]string, len(position.Needs))
		for i, need := range position.Needs {
			needs[i] = w.needText(need)
		}
		parts = append(parts, "`"+position.Selection+"`"+w.without+strings.Join(needs, "; ")+")")
	}
	return strings.Join(parts, ", ")
}

// denialText says why no fine-grained token reaches an action.
func (w *Words) denialText(denial *finegrained.Denial) string {
	element := "`" + denial.Element + "`"
	switch denial.Cause {
	case finegrained.CauseMutationUndeclared:
		return fmt.Sprintf(w.mutationUndeclared, element)
	case finegrained.CauseTypeUndeclared:
		return fmt.Sprintf(w.typeUndeclared, element)
	case finegrained.CausePayloadUndeclared:
		return fmt.Sprintf(w.payloadUndeclared, element)
	case finegrained.CauseBoundaryUnresolvable:
		return fmt.Sprintf(w.unresolvable, element, w.unresolvedEffect(denial.Effect))
	case finegrained.CauseRESTTodo:
		return fmt.Sprintf(w.restTodo, element)
	default:
		return fmt.Sprintf(w.undeclared, element)
	}
}

// unresolvedEffect words what GitLab does where a boundary never resolves: a
// mutation's own check refuses it before anything runs, a write's answer is
// lost after the write commits, and a read answers null or leaves the object
// out of its list.
func (w *Words) unresolvedEffect(effect finegrained.Effect) string {
	switch effect {
	case finegrained.EffectRefused:
		return w.effectRefused
	case finegrained.EffectCommittedThenNull:
		return w.effectCommitted
	default:
		return w.effectNull
	}
}
