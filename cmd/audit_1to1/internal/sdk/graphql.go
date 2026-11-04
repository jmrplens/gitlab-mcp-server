// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdk

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"maps"
	"slices"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
)

// graphqlOperation is one handler (or helper) function that reaches GitLab
// through the raw GraphQL interface instead of an SDK service wrapper.
type graphqlOperation struct {
	// Package is the short internal/tools package name.
	Package string `json:"package"`
	// Operation is the enclosing function, the unit ADR-0006 is adjudicated
	// in: a package that uses GraphQL for one operation and the SDK for the
	// rest has one entry here, not a whole-package finding.
	Operation string `json:"operation"`
	// Service is the client-go service that now covers the same domain, and
	// whose existence retires the ADR-0006 exemption.
	Service string `json:"service"`
	// ServiceMethods is how many endpoint methods that service declares.
	ServiceMethods int `json:"service_methods"`
	// Status is adjudicated or unadjudicated.
	Status string `json:"status"`
	// Decision and Reason are set for an adjudicated operation.
	Decision string `json:"decision,omitempty"`
	Reason   string `json:"reason,omitempty"`
	// Sites lists the file:line of each raw GraphQL reference in the function.
	Sites []string `json:"sites"`
}

// operationKey identifies one adjudicated operation, "<package>.<function>".
func (o graphqlOperation) operationKey() string { return o.Package + "." + o.Operation }

// graphqlSite is one raw GraphQL reference found in the tree.
type graphqlSite struct {
	pkg      string
	function string
	position string
}

// collectGraphQLSites records every place a tool package takes hold of the
// client-go GraphQL interface: a value of that type, wherever it appears.
// Matching the VALUE rather than the .Do call is what catches the delegating
// form, client.GL().GraphQL handed to a helper. internal/toolutil is outside
// the audited tree, so a handler that delegates its mutation there would
// otherwise look REST-only.
func collectGraphQLSites(pkgs []*packages.Package, graphQL *types.Named) []graphqlSite {
	var sites []graphqlSite
	for _, pkg := range pkgs {
		pkgName := shared.ShortPackage(pkg.PkgPath)
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				name := functionName(fn)
				ast.Inspect(fn.Body, func(node ast.Node) bool {
					expr, isExpr := node.(ast.Expr)
					if !isExpr || !isGraphQLInterface(pkg.TypesInfo.TypeOf(expr), graphQL) {
						return true
					}
					sites = append(sites, graphqlSite{
						pkg:      pkgName,
						function: name,
						position: relativePosition(pkg, expr.Pos()),
					})
					// Stop here: descending into a matched client.GL().GraphQL
					// would match its own GraphQL identifier again and record
					// the same reference twice.
					return false
				})
			}
		}
	}
	slices.SortFunc(sites, func(a, b graphqlSite) int {
		return cmp.Or(
			strings.Compare(a.pkg, b.pkg),
			strings.Compare(a.function, b.function),
			strings.Compare(a.position, b.position),
		)
	})
	return sites
}

// functionName renders a declaration as it should read in the adjudication
// table: bare for a function, Receiver.Method for a method.
func functionName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return receiverName(fn.Recv.List[0].Type) + "." + fn.Name.Name
}

func receiverName(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.StarExpr:
		return receiverName(typed.X)
	case *ast.IndexExpr:
		return receiverName(typed.X)
	case *ast.IndexListExpr:
		return receiverName(typed.X)
	case *ast.Ident:
		return typed.Name
	default:
		return "?"
	}
}

// isGraphQLInterface reports whether t is the client-go GraphQL interface,
// through a pointer or directly.
func isGraphQLInterface(t types.Type, graphQL *types.Named) bool {
	if t == nil {
		return false
	}
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	return ok && named.Obj() == graphQL.Obj()
}

// relativePosition renders a position as a repository-relative file:line.
func relativePosition(pkg *packages.Package, pos token.Pos) string {
	position := pkg.Fset.Position(pos)
	return fmt.Sprintf("%s:%d", repoRelativeFile(position.Filename), position.Line)
}

// repoRelativeFile trims an absolute source path down to its repository-relative
// form, and is a function of its own so the trimming can be tested without a
// loaded package.
//
// Separators are normalized first. go/token stores whatever the build system
// handed it and normalizes nothing, so on Windows the position carries
// backslashes, the search finds no match, and the site is reported with the
// developer's whole workspace path in front of it. The audit compares its output
// against a declared table, so that difference is not cosmetic.
//
// The replacement is unconditional rather than filepath.ToSlash, which is a
// no-op on Unix and would leave the Windows shape untestable from the only
// platform CI runs on. The cost is a literal backslash inside a Unix directory
// name, which would change how a path is printed and nothing else.
func repoRelativeFile(filename string) string {
	file := strings.ReplaceAll(filename, `\`, "/")
	if idx := strings.LastIndex(file, "/internal/"); idx >= 0 {
		file = file[idx+1:]
	}
	return file
}

// buildGraphQLOperations groups the raw GraphQL sites by enclosing function and
// keeps those whose package maps to a client-go service, adjudicating each
// against graphqlDecisions.
func buildGraphQLOperations(sites []graphqlSite, services map[string]sdkService, aliases map[string]string, decisions map[string]decision) []graphqlOperation {
	byOperation := map[string]*graphqlOperation{}
	var order []string
	for _, site := range sites {
		service, ok := serviceForPackage(site.pkg, services, aliases)
		if !ok {
			continue
		}
		key := site.pkg + "." + site.function
		op, seen := byOperation[key]
		if !seen {
			op = &graphqlOperation{
				Package:        site.pkg,
				Operation:      site.function,
				Service:        service.Service,
				ServiceMethods: service.APIMethods,
			}
			byOperation[key] = op
			order = append(order, key)
		}
		op.Sites = append(op.Sites, site.position)
	}
	sort.Strings(order)
	operations := make([]graphqlOperation, 0, len(order))
	for _, key := range order {
		op := byOperation[key]
		if adjudged, ok := decisions[key]; ok {
			op.Status = statusAdjudicated
			op.Decision = adjudged.Decision
			op.Reason = adjudged.Reason
		} else {
			op.Status = statusUnadjudicated
		}
		operations = append(operations, *op)
	}
	return operations
}

// serviceForPackage resolves the client-go service a tool package corresponds
// to: the service whose lower-cased name equals the package name, or the one an
// explicit alias names when the two spellings differ.
func serviceForPackage(pkgName string, services map[string]sdkService, aliases map[string]string) (sdkService, bool) {
	if alias, ok := aliases[pkgName]; ok {
		service, exists := services[alias]
		return service, exists
	}
	for name, service := range services {
		if strings.EqualFold(name, pkgName) {
			return service, true
		}
	}
	return sdkService{}, false
}

// CoveredGraphQLMethods returns the client-go methods a KEEP decision of
// graphqlDecisions reads in place of, keyed "Service.Method" as
// acceptedMissingMethods keys them, each with the operation that replaces it
// and why, which R-ACTION adjudicates as COVERED_GRAPHQL.
func CoveredGraphQLMethods() map[string]string {
	return keptReplacements(graphqlDecisions, graphqlServiceAliases)
}

// keptReplacements collects the methods the KEEP decisions of decisions
// replace, in key order, so the operation a method is attributed to does not
// depend on the order a map is read in. A method of a service other than the
// one the operation's package resolves to through aliases is not collected:
// the sdk gate refuses it, and R-ACTION goes on listing the method rather
// than taking it as answered by an operation that cannot read in its place.
func keptReplacements(decisions map[string]decision, aliases map[string]string) map[string]string {
	out := map[string]string{}
	for _, key := range slices.Sorted(maps.Keys(decisions)) {
		adjudged := decisions[key]
		if adjudged.Decision != decisionKeep {
			continue
		}
		for _, method := range adjudged.Replaces {
			if _, seen := out[method]; !seen && ofOperationService(key, method, aliases) {
				out[method] = key + ": " + adjudged.Reason
			}
		}
	}
	return out
}

// ofOperationService reports whether method, keyed "Service.Method", is a
// method of the service the package of the operation key resolves to, the
// way [serviceForPackage] resolves it: the service aliases names for the
// package, or else the one whose name is the package's in any case. An
// operation is adjudicated against that service, so it can read in place of
// that service's methods and of no other's.
func ofOperationService(key, method string, aliases map[string]string) bool {
	pkg, _, _ := strings.Cut(key, ".")
	serviceName, _, _ := strings.Cut(method, ".")
	if alias, aliased := aliases[pkg]; aliased {
		return alias == serviceName
	}
	return strings.EqualFold(serviceName, pkg)
}

// staleGraphQLReplaces lists each method a decision's Replaces names that
// answers no method R-ACTION would list: one a MIGRATE decision names, one of
// a service client-go does not declare, one of a service other than the one
// the operation's package resolves to through aliases, one of a service no
// handler calls, one the service does not declare, one a handler calls, and
// one an earlier decision in key order already names. services is keyed by
// bare service name and usage by interface name, as [serviceIndex] and
// [shared.CollectServiceUsage] key them.
func staleGraphQLReplaces(decisions map[string]decision, services map[string]sdkService, aliases map[string]string, usage map[string]*shared.ServiceUsage) []string {
	var stale []string
	claimed := map[string]string{}
	for _, key := range slices.Sorted(maps.Keys(decisions)) {
		adjudged := decisions[key]
		for _, method := range adjudged.Replaces {
			problem := replacesProblem(key, adjudged.Decision, method, services, aliases, usage)
			if first, seen := claimed[method]; problem == "" && seen {
				problem = "graphql " + first + " replaces it too"
			}
			if problem != "" {
				stale = append(stale, "graphql "+key+" (replaces "+method+": "+problem+")")
				continue
			}
			claimed[method] = key
		}
	}
	return stale
}

// replacesProblem says why a method the decision of verdict on the operation
// key names answers no method R-ACTION would list, or returns "" when it
// answers one.
func replacesProblem(key, verdict, method string, services map[string]sdkService, aliases map[string]string, usage map[string]*shared.ServiceUsage) string {
	if verdict != decisionKeep {
		return "only a KEEP decision stands in for a method"
	}
	serviceName, methodName, _ := strings.Cut(method, ".")
	service, declared := services[serviceName]
	if !declared {
		return "client-go declares no such service"
	}
	if !ofOperationService(key, method, aliases) {
		pkg, _, _ := strings.Cut(key, ".")
		return "the operation's package, " + pkg + ", does not resolve to this service"
	}
	use, called := usage[service.Interface]
	if !called {
		return "no handler calls the service, which its declaration answers"
	}
	if !slices.Contains(shared.APIMethodNames(use.Named), methodName) {
		return "the service declares no such method"
	}
	if _, reached := use.Called[methodName]; reached {
		return "a handler calls it"
	}
	return ""
}

// staleGraphQLDecisions lists adjudicated operations that no longer exist, so a
// decision cannot outlive the code it was written about.
func staleGraphQLDecisions(operations []graphqlOperation, decisions map[string]decision) []string {
	live := map[string]struct{}{}
	for _, op := range operations {
		live[op.operationKey()] = struct{}{}
	}
	var stale []string
	for key := range decisions {
		if _, ok := live[key]; !ok {
			stale = append(stale, "graphql "+key)
		}
	}
	sort.Strings(stale)
	return stale
}
