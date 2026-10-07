package main

import (
	"cmp"
	"fmt"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
)

// exceptionDirective is how a deliberate exception is declared: in the source
// next to the action it excuses, never in this audit.
//
//	//gitlab:allow-readonly-graphql-mutation <action_name>: <reason>
//
// The directive has to sit in the package that owns the action and name that
// action, so a reader of the handler sees the exception beside the handler and
// a reader of this file sees no list of blessed actions at all. An exception
// that stops matching anything is reported, so one left behind by a later
// change does not quietly widen the gate.
const exceptionDirective = "//gitlab:allow-readonly-graphql-mutation"

// action is one catalog action the audit has to answer for.
type action = actionrequests.Action

// exception is one declared exception, kept with where it was declared.
type exception struct {
	pkgName string
	action  string
	reason  string
	pos     token.Pos
}

// finding is one reason the audit fails.
type finding struct {
	// action is the catalog action ID the finding is about, or, for a finding
	// about a document no action can be tied to, the package or directory it
	// lives in. It is never printed: it is the first sort key, so the findings
	// of one action stay together and a CI log diff shows a changed finding
	// rather than a reshuffled report.
	action string
	// message is the whole explanation, already formatted.
	message string
}

// auditResult is everything one run learned, so a caller can report it.
type auditResult struct {
	findings []finding
	// checked is how many read-only actions were resolved and classified.
	checked int
	// graphQL is the read-only actions that reach the GraphQL transport at
	// all, by catalog ID. They are the ones this gate is really about, and
	// the verbose report lists them so the set is reviewable rather than a
	// number.
	graphQL []string
	// exceptions is how many declared exceptions were used.
	exceptions int
}

// audit resolves every read-only action to its handler, classifies the GraphQL
// documents that handler can send, and reports the ones that are mutations.
func audit(prog *actionrequests.Program, actions []action, root string) auditResult {
	sites := prog.Sites()
	exceptions := collectExceptions(prog.Packages())
	used := make(map[string]bool, len(exceptions))
	result := auditResult{}

	for _, act := range actions {
		if !act.ReadOnly {
			continue
		}
		matched := actionrequests.Match(sites, act)
		if len(matched) == 0 {
			result.findings = append(result.findings, unresolvedFinding(act,
				"no ActionSpec construction resolves to this action"))
			continue
		}
		// Two construction sites for one action are two actions' handlers
		// classified as one: the answer would be about their union, and a
		// mutation one of them sends would be charged to both or hidden by
		// the exception of the other.
		if len(matched) > 1 {
			result.findings = append(result.findings, ambiguousFinding(prog, act, matched, root))
			continue
		}
		roots := prog.Roots(matched)
		// A spec whose route resolves to no handler at all is the dangerous
		// shape, not a harmless one: the reachable set is empty, so every
		// classification below would come back clean whatever the handler
		// actually does. Report it for the same reason an unplaceable action
		// is reported.
		if len(roots) == 0 {
			result.findings = append(result.findings, unresolvedFinding(act,
				"the action's route resolves to no handler"))
			continue
		}
		result.checked++
		reached := prog.Reachable(roots)
		sends, mutations := classifyReached(prog.Function, reached)
		if sends {
			result.graphQL = append(result.graphQL, act.ID)
		}
		// Naming a mutation document is the finding, whether or not the send
		// was recognized. Requiring both would make every failure depend on
		// recognizing the transport too, and a chain this audit could not
		// follow to GraphQL.Do would then excuse the mutation rather than
		// report it, which is the one outcome a gate must not have.
		if len(mutations) == 0 {
			continue
		}
		if key, ok := exceptionFor(exceptions, act); ok {
			used[key] = true
			result.exceptions++
			continue
		}
		result.findings = append(result.findings, mutationFinding(prog.Position, act, matched, mutations, root))
	}

	result.findings = append(result.findings, staleExceptions(prog.Position, exceptions, used, root)...)
	result.findings = append(result.findings, unattributedFindings(prog.Unattributed(), root)...)
	sortFindings(result.findings)
	return result
}

// sortFindings orders a report by the action a finding is about, and by the
// message within one action, whatever order the findings were made in.
func sortFindings(findings []finding) {
	slices.SortFunc(findings, func(a, b finding) int {
		return cmp.Or(strings.Compare(a.action, b.action), strings.Compare(a.message, b.message))
	})
}

// ambiguousFinding reports a read-only action more than one construction site
// was joined to, naming each.
func ambiguousFinding(prog *actionrequests.Program, act action, matched []actionrequests.Site, root string) finding {
	var builder strings.Builder
	fmt.Fprintf(&builder, "%s resolves to %d ActionSpec constructions, so its handler cannot be told from another action's.\n",
		act.ID, len(matched))
	for _, site := range matched {
		fmt.Fprintf(&builder, "    declared at %s\n", relative(prog.Position(site.Pos), root))
	}
	builder.WriteString("    Give each construction the individual tool name the catalog holds for its action, so the join reads one site.")
	return finding{action: act.ID, message: builder.String()}
}

// unresolvedFinding reports a read-only action this audit cannot answer for.
// It is a failure rather than a skip because a gate that passes what it cannot
// classify stops holding the moment a domain is written in a shape the
// resolver does not follow, and says nothing while it does.
func unresolvedFinding(act action, reason string) finding {
	return finding{
		action: act.ID,
		message: fmt.Sprintf("%s: %s, so its handler cannot be classified.\n"+
			"    Declare the action through a toolutil action-spec constructor in package %q, or the gate cannot vouch for it.",
			act.ID, reason, act.Owner),
	}
}

// functionIndex is how the audit reads the body of a reached function: the
// program's own index in a run, and a table of hand-built bodies in a test of
// the classification alone.
type functionIndex func(*types.Func) (*actionrequests.Function, bool)

// positioner renders a source position for a finding.
type positioner func(token.Pos) token.Position

// classifyReached reports whether the reachable set sends GraphQL at all, and
// which mutation documents it names.
func classifyReached(functions functionIndex, reached map[*types.Func]bool) (bool, []mutationSite) {
	sends := false
	var mutations []mutationSite
	for fnObj := range reached {
		fn, ok := functions(fnObj)
		if !ok {
			continue
		}
		if fn.SendsGraphQL {
			sends = true
		}
		for _, doc := range fn.Documents {
			if classifyDocument(doc.Text) == writeDocument {
				mutations = append(mutations, mutationSite{fn: fnObj, doc: doc})
			}
		}
	}
	slices.SortFunc(mutations, func(a, b mutationSite) int { return cmp.Compare(a.doc.Pos, b.doc.Pos) })
	return sends, mutations
}

// mutationSite is one mutation document named by one reachable function.
type mutationSite struct {
	fn  *types.Func
	doc actionrequests.DocumentUse
}

// mutationFinding formats the failure this audit exists for.
func mutationFinding(position positioner, act action, matched []actionrequests.Site, mutations []mutationSite, root string) finding {
	var builder strings.Builder
	fmt.Fprintf(&builder, "%s is classified ReadOnly but its handler sends a GraphQL mutation.\n", act.ID)
	for _, matchedSite := range matched {
		fmt.Fprintf(&builder, "    action declared at %s\n", relative(position(matchedSite.Pos), root))
	}
	for _, mutation := range mutations {
		fmt.Fprintf(&builder, "    %s sends %s at %s\n",
			mutation.fn.Name(), documentName(mutation.doc), relative(position(mutation.doc.Pos), root))
	}
	builder.WriteString("    A read-only action must not reach a mutation: --read-only keeps it, so the write would run\n")
	builder.WriteString("    on a deployment whose operator asked for none.\n")
	builder.WriteString("    Reclassify the action as mutating, or declare the exception with " + exceptionDirective + ".")
	return finding{action: act.ID, message: builder.String()}
}

// documentName names the document a finding points at.
func documentName(doc actionrequests.DocumentUse) string {
	if doc.Name == "" {
		return "an inline mutation document"
	}
	return doc.Name
}

// collectExceptions reads every exception directive in the loaded source.
func collectExceptions(loaded []*packages.Package) map[string]exception {
	found := make(map[string]exception)
	for _, pkg := range loaded {
		for _, file := range pkg.Syntax {
			for _, group := range file.Comments {
				for _, comment := range group.List {
					parsed, ok := parseException(pkg.Name, comment.Text, comment.Pos())
					if !ok {
						continue
					}
					found[exceptionKey(parsed.pkgName, parsed.action)] = parsed
				}
			}
		}
	}
	return found
}

// parseException reads one directive comment.
func parseException(pkgName, text string, pos token.Pos) (exception, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(text), exceptionDirective)
	if !ok {
		return exception{}, false
	}
	name, reason, hasReason := strings.Cut(strings.TrimSpace(rest), ":")
	name = strings.TrimSpace(name)
	reason = strings.TrimSpace(reason)
	if name == "" || !hasReason || reason == "" {
		return exception{}, false
	}
	return exception{pkgName: pkgName, action: name, reason: reason, pos: pos}, true
}

// exceptionKey identifies one exception by the package and action it names.
func exceptionKey(pkgName, actionName string) string {
	return pkgName + "." + actionName
}

// exceptionFor finds the exception excusing one action, if it was declared in
// the package that owns the action.
func exceptionFor(exceptions map[string]exception, act action) (string, bool) {
	key := exceptionKey(act.Owner, act.Name)
	_, ok := exceptions[key]
	return key, ok
}

// staleExceptions reports directives that no longer excuse anything, so an
// exception cannot outlive the reason it was written for.
func staleExceptions(position positioner, exceptions map[string]exception, used map[string]bool, root string) []finding {
	var findings []finding
	for key, declared := range exceptions {
		if used[key] {
			continue
		}
		findings = append(findings, finding{
			action: key,
			message: fmt.Sprintf("%s at %s excuses a read-only action that no longer sends a mutation. Remove it.",
				exceptionDirective, relative(position(declared.pos), root)),
		})
	}
	return findings
}

// unattributedFindings reports every GraphQL document in the shared inventory
// that this audit cannot tie to a handler.
//
// It is the gate's own tripwire. The reachability walk resolves a document
// through the object that declares it, so a document written in a .graphql
// file of its own, or built inline in a shape the body walk does not fold, is
// judged by nothing here: the run would still end with "no read-only action
// reaches a mutation" while a mutation sat in a file the walk never opened.
// The repository writes every document as a named constant today, so this is
// silent, and the day one moves it says so instead of going quiet.
//
// These findings belong to no action, by construction: not being able to name
// the handler is what they report. The package the document lives in stands in
// as their sort key, which is all the field is used for.
func unattributedFindings(unattributed []graphqldocs.Document, root string) []finding {
	findings := make([]finding, 0, len(unattributed))
	for _, document := range unattributed {
		findings = append(findings, finding{
			action: document.Package,
			message: fmt.Sprintf("%s at %s is a GraphQL document no handler can be held responsible for.\n"+
				"    This gate resolves a document through the constant that declares it, so one written in a file\n"+
				"    of its own or assembled where it is used is classified by nothing and could be a mutation a\n"+
				"    read-only action sends. Declare it as a constant in the package that sends it.",
				document.Label(), relative(document.Position, root)),
		})
	}
	return findings
}

// relative trims a position to the repository root so a finding reads as a
// path a person can open.
func relative(pos token.Position, root string) string {
	path := pos.Filename
	if root != "" {
		if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
			path = filepath.ToSlash(rel)
		}
	}
	return fmt.Sprintf("%s:%d", path, pos.Line)
}
