package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
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
type action struct {
	ID       string
	Name     string
	Owner    string
	ReadOnly bool
}

// exception is one declared exception, kept with where it was declared.
type exception struct {
	pkgName string
	action  string
	reason  string
	pos     token.Pos
}

// finding is one reason the audit fails.
type finding struct {
	// action is the catalog action ID the finding is about.
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
func audit(prog *program, actions []action, root string) auditResult {
	res := &resolver{prog: prog}
	sites := res.collectSites()
	exceptions := collectExceptions(prog)
	used := make(map[string]bool, len(exceptions))
	result := auditResult{}

	for _, act := range actions {
		if !act.ReadOnly {
			continue
		}
		matched := matchSites(sites, act)
		if len(matched) == 0 {
			result.findings = append(result.findings, unresolvedFinding(act,
				"no ActionSpec construction resolves to this action"))
			continue
		}
		roots := handlerFuncs(prog, matched)
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
		reached := prog.reachable(roots)
		sends, mutations := classifyReached(prog, reached)
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
		result.findings = append(result.findings, mutationFinding(prog, act, matched, mutations, root))
	}

	result.findings = append(result.findings, staleExceptions(prog, exceptions, used, root)...)
	sort.Slice(result.findings, func(i, j int) bool {
		if result.findings[i].action != result.findings[j].action {
			return result.findings[i].action < result.findings[j].action
		}
		return result.findings[i].message < result.findings[j].message
	})
	return result
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

// matchSites picks the construction sites that declare one catalog action.
// The owning package decides when several packages declare the same action
// name; when none of them is the owner, every site with that name is taken,
// because a wrong guess must not be the quiet one.
func matchSites(sites map[string][]site, act action) []site {
	candidates := sites[act.Name]
	if len(candidates) == 0 {
		return nil
	}
	owned := make([]site, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.pkgName == act.Owner {
			owned = append(owned, candidate)
		}
	}
	if len(owned) > 0 {
		return owned
	}
	return candidates
}

// handlerFuncs turns resolved handler references into call-graph roots. A
// handler written as a literal has no declared function, so its body is
// indexed on the spot and its callees become the roots instead.
func handlerFuncs(prog *program, matched []site) []*types.Func {
	var roots []*types.Func
	for _, matchedSite := range matched {
		for _, handler := range matchedSite.handlers {
			if handler.fn != nil {
				roots = append(roots, handler.fn)
				continue
			}
			roots = append(roots, prog.indexLiteral(handler.pkg, handler.lit)...)
		}
	}
	return roots
}

// classifyReached reports whether the reachable set sends GraphQL at all, and
// which mutation documents it names.
func classifyReached(prog *program, reached map[*types.Func]bool) (bool, []mutationSite) {
	sends := false
	var mutations []mutationSite
	for fnObj := range reached {
		fn, ok := prog.funcs[fnObj]
		if !ok {
			continue
		}
		if fn.sendsGraphQL {
			sends = true
		}
		for _, doc := range fn.docs {
			if doc.kind == writeDocument {
				mutations = append(mutations, mutationSite{fn: fnObj, doc: doc})
			}
		}
	}
	sort.Slice(mutations, func(i, j int) bool { return mutations[i].doc.pos < mutations[j].doc.pos })
	return sends, mutations
}

// mutationSite is one mutation document named by one reachable function.
type mutationSite struct {
	fn  *types.Func
	doc docRef
}

// mutationFinding formats the failure this audit exists for.
func mutationFinding(prog *program, act action, matched []site, mutations []mutationSite, root string) finding {
	var builder strings.Builder
	fmt.Fprintf(&builder, "%s is classified ReadOnly but its handler sends a GraphQL mutation.\n", act.ID)
	for _, matchedSite := range matched {
		fmt.Fprintf(&builder, "    action declared at %s\n", relative(prog.position(matchedSite.pos), root))
	}
	for _, mutation := range mutations {
		fmt.Fprintf(&builder, "    %s sends %s at %s\n",
			mutation.fn.Name(), documentName(mutation.doc), relative(prog.position(mutation.doc.pos), root))
	}
	builder.WriteString("    A read-only action must not reach a mutation: --read-only and the surface served to a\n")
	builder.WriteString("    read_api token both keep it, so the write would run where a write is supposed to be impossible.\n")
	builder.WriteString("    Reclassify the action as mutating, or declare the exception with " + exceptionDirective + ".")
	return finding{action: act.ID, message: builder.String()}
}

// documentName names the document a finding points at.
func documentName(doc docRef) string {
	if doc.name == "" {
		return "an inline mutation document"
	}
	return doc.name
}

// collectExceptions reads every exception directive in the loaded source.
func collectExceptions(prog *program) map[string]exception {
	found := make(map[string]exception)
	for _, pkg := range prog.order {
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
func staleExceptions(prog *program, exceptions map[string]exception, used map[string]bool, root string) []finding {
	var findings []finding
	for key, declared := range exceptions {
		if used[key] {
			continue
		}
		findings = append(findings, finding{
			action: key,
			message: fmt.Sprintf("%s at %s excuses a read-only action that no longer sends a mutation. Remove it.",
				exceptionDirective, relative(prog.position(declared.pos), root)),
		})
	}
	return findings
}

// indexLiteral indexes a handler written as a function literal and returns the
// functions its body names, which stand in for it as call-graph roots.
func (p *program) indexLiteral(pkg *packages.Package, lit *ast.FuncLit) []*types.Func {
	fn := p.indexBody(pkg, nil, &ast.FuncDecl{Body: lit.Body})
	roots := make([]*types.Func, 0, len(fn.calls))
	for callee := range fn.calls {
		roots = append(roots, callee)
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].Pos() < roots[j].Pos() })
	return roots
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
