package structs

import (
	"go/ast"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
)

// ProjectionPairing names an output struct a handler builds field by field out
// of a client-go struct, which is the one shape of output [CollectPairs] cannot
// see: there is no converter taking the SDK struct and returning ours, only a
// composite literal whose fields are read off it.
//
// These are the compact projections of a GitLab entity: the six fields of an
// issue a milestone's issue list keeps, the four of a job a resource group's
// queue keeps. Nothing paired them, so the type grain of the shape join judged
// none of them and the only comparison that saw their gaps was the package
// grain, where they sat under two thousand findings of other kinds.
//
// It is kept apart from [OutputPairing] on purpose. The field diff reads that
// list, and a projection held against client-go's whole struct would report
// every field it leaves out as a gap of the SDK surface, which is not the
// question: whether a compact row should carry a field is a question about
// what GitLab sends on the one endpoint the row comes from, and that is what
// Methods is for.
type ProjectionPairing struct {
	// Package is the internal/tools domain name, spelled the way
	// [shared.ShortPackage] spells it.
	Package string
	// MCPType is the output struct's Go name.
	MCPType string
	// SDKType is the client-go struct its fields are read from, unqualified.
	SDKType string
	// SDKFields is what that struct deserializes, as [OutputPairing.SDKFields]
	// has it.
	SDKFields []string
	// Methods are the client-go service methods whose answer the projection is
	// built from, as "Service.Method" with the service spelled the way
	// [shared.ServiceName] spells it, sorted. They are found in the function
	// that builds the literal or, where that function is a helper, in the
	// functions of the package that call it. Empty when none of them calls a
	// method answering with SDKType, which leaves a reader of the pairing with
	// the struct alone, as a converter pairing does.
	Methods []string
}

// callerDepth bounds how far up the package's own call graph the methods a
// projection is built from are looked for. A literal sits in a handler, in a
// converter the handler calls, or in a helper that converter calls; past that
// the walk has left every function that made a request, and a method found
// further up answers some other action.
const callerDepth = 3

// inputSuffix names a struct a model sends rather than one it reads, which no
// literal of is a projection of anything GitLab answered.
const inputSuffix = "Input"

// sdkCall is one reference to a client-go service method: which method, and
// the struct it answers with.
type sdkCall struct {
	method  string
	element string
}

// projectionSite is one composite literal of a local output type whose fields
// are read off a client-go struct, and the function it sits in.
type projectionSite struct {
	mcpName string
	sdk     *types.Named
	st      *types.Struct
	in      *types.Func
}

// packageFunctions is what one walk of a package records about its functions:
// the client-go methods each one references, the package's own functions each
// one references, and the projections each one builds.
type packageFunctions struct {
	calls   map[*types.Func][]sdkCall
	callers map[*types.Func]map[*types.Func]bool
	sites   []projectionSite
}

// CollectProjections returns every projection pairing one tool package
// builds, sorted by output type and then by client-go struct.
//
// A type a converter already pairs is reported here too when a literal also
// builds it: the two lists answer different questions and a reader joins
// them, rather than this deciding for it which one wins.
func CollectProjections(pkg *packages.Package) []ProjectionPairing {
	if pkg.TypesInfo == nil {
		return nil
	}
	walked := walkFunctions(pkg)
	short := shared.ShortPackage(pkg.PkgPath)

	type key struct{ mcp, sdk string }
	byKey := map[key]*ProjectionPairing{}
	methods := map[key]map[string]bool{}
	for _, site := range walked.sites {
		k := key{site.mcpName, site.sdk.Obj().Name()}
		if byKey[k] == nil {
			byKey[k] = &ProjectionPairing{
				Package:   short,
				MCPType:   site.mcpName,
				SDKType:   k.sdk,
				SDKFields: sdkFieldNames(site.st),
			}
			methods[k] = map[string]bool{}
		}
		for _, method := range walked.methodsFor(site.in, k.sdk) {
			methods[k][method] = true
		}
	}

	out := make([]ProjectionPairing, 0, len(byKey))
	for k, pairing := range byKey {
		for method := range methods[k] {
			pairing.Methods = append(pairing.Methods, method)
		}
		sort.Strings(pairing.Methods)
		out = append(out, *pairing)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].MCPType != out[j].MCPType {
			return out[i].MCPType < out[j].MCPType
		}
		return out[i].SDKType < out[j].SDKType
	})
	return out
}

// walkFunctions reads every function declaration of the package once.
//
// A function literal is attributed to the declaration it sits in, which is
// right for both of the shapes that hold one here: a handler closure in an
// ActionSpecs table, and a page fetcher passed to a pagination helper.
func walkFunctions(pkg *packages.Package) packageFunctions {
	walked := packageFunctions{calls: map[*types.Func][]sdkCall{}, callers: map[*types.Func]map[*types.Func]bool{}}
	for _, file := range pkg.Syntax {
		for _, declaration := range file.Decls {
			function, isFunction := declaration.(*ast.FuncDecl)
			if !isFunction || function.Body == nil {
				continue
			}
			owner, isFunc := pkg.TypesInfo.Defs[function.Name].(*types.Func)
			if !isFunc {
				continue
			}
			walked.walk(pkg, owner, function.Body)
		}
	}
	return walked
}

// walk records what one function body references and builds.
func (w *packageFunctions) walk(pkg *packages.Package, owner *types.Func, body *ast.BlockStmt) {
	ast.Inspect(body, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.SelectorExpr:
			if call, ok := serviceMethod(pkg.TypesInfo, typed); ok {
				w.calls[owner] = append(w.calls[owner], call)
			}
		case *ast.Ident:
			if callee, ok := pkg.TypesInfo.Uses[typed].(*types.Func); ok && callee != owner && declaredIn(callee, pkg.PkgPath) {
				if w.callers[callee] == nil {
					w.callers[callee] = map[*types.Func]bool{}
				}
				w.callers[callee][owner] = true
			}
		case *ast.CompositeLit:
			if site, ok := projectionOf(pkg, typed); ok {
				site.in = owner
				w.sites = append(w.sites, site)
			}
		}
		return true
	})
}

// methodsFor returns the client-go methods answering with element that the
// function a projection is built in references, or, when it references none,
// the ones the package's functions calling it reference, nearest first.
//
// Each caller stops the walk on its own branch as soon as it references such a
// method, so a converter two handlers share is credited with both handlers'
// methods and never with a method a handler's own caller happens to call.
func (w *packageFunctions) methodsFor(in *types.Func, element string) []string {
	var found []string
	seen := map[*types.Func]bool{in: true}
	level := []*types.Func{in}
	for range callerDepth + 1 {
		var next []*types.Func
		for _, function := range level {
			matched := false
			for _, call := range w.calls[function] {
				if call.element == element {
					found = append(found, call.method)
					matched = true
				}
			}
			if matched {
				continue
			}
			for caller := range w.callers[function] {
				if !seen[caller] {
					seen[caller] = true
					next = append(next, caller)
				}
			}
		}
		level = next
	}
	return found
}

// declaredIn reports whether a function belongs to the package being walked.
func declaredIn(function *types.Func, pkgPath string) bool {
	return function.Pkg() != nil && function.Pkg().Path() == pkgPath
}

// serviceMethod reads a selector naming a method of a client-go service
// interface, called or passed as a value, and the struct that method answers
// with. A method answering with no client-go struct, or with the pagination
// wrapper, names nothing a projection could be read from.
func serviceMethod(info *types.Info, selector *ast.SelectorExpr) (sdkCall, bool) {
	service, isService := shared.ClientGoServiceInterface(info.TypeOf(selector.X))
	if !isService {
		return sdkCall{}, false
	}
	method, isMethod := info.Uses[selector.Sel].(*types.Func)
	if !isMethod || method.Signature().Results().Len() == 0 {
		return sdkCall{}, false
	}
	element := answerStruct(method.Signature().Results().At(0).Type())
	if element == nil || nonResultSDKStruct(element) {
		return sdkCall{}, false
	}
	return sdkCall{method: shared.ServiceName(service) + "." + method.Name(), element: element.Obj().Name()}, true
}

// answerStruct unwraps a method's first result to the client-go struct at its
// core: *T, []*T and []T all answer with T.
func answerStruct(t types.Type) *types.Named {
	for {
		switch typed := t.(type) {
		case *types.Pointer:
			t = typed.Elem()
		case *types.Slice:
			t = typed.Elem()
		default:
			named, _, isSDK := clientGoNamedStruct(t)
			if !isSDK {
				return nil
			}
			return named
		}
	}
}

// projectionOf reads one composite literal: whether it builds an exported
// struct of this package, and which client-go struct most of its fields are
// read off.
//
// A literal whose fields are read off two structs equally often is left
// unpaired rather than credited to either, since a guess here names the
// endpoints the type is judged against.
func projectionOf(pkg *packages.Package, literal *ast.CompositeLit) (projectionSite, bool) {
	// An elided literal inside a slice of the type has no type expression, and
	// is read by its type alone, which is the first thing this tries anyway.
	_, _, name, isLocal := localOrAliasNamedStruct(pkg, literal.Type, pkg.TypesInfo.TypeOf(literal))
	if !isLocal || !ast.IsExported(name) || strings.HasSuffix(name, inputSuffix) {
		return projectionSite{}, false
	}

	counts := map[*types.Named]int{}
	structsOf := map[*types.Named]*types.Struct{}
	for _, element := range literal.Elts {
		value := element
		if pair, isPair := element.(*ast.KeyValueExpr); isPair {
			value = pair.Value
		}
		for named, st := range sourcesOf(pkg, value) {
			counts[named]++
			structsOf[named] = st
		}
	}

	var best *types.Named
	bestCount, tied := 0, false
	for named, count := range counts {
		if count > bestCount {
			best, bestCount, tied = named, count, false
		} else if count == bestCount {
			tied = true
		}
	}
	if best == nil || tied {
		return projectionSite{}, false
	}
	return projectionSite{mcpName: name, sdk: best, st: structsOf[best]}, true
}

// sourcesOf names the client-go structs one field value is read off: the
// receiver of each field selection on a client-go struct that is not itself
// the operand of another such selection. `mr.Author.Username` is read off the
// author, not off the merge request, and `issue.CreatedAt.Format(...)` is read
// off the issue, the Format being a method of the time rather than a field.
//
// A literal of a type of this package inside the value, the value itself
// included, is not followed, since that literal is a projection of its own and
// says nothing about what this one is read off.
func sourcesOf(pkg *packages.Package, value ast.Expr) map[*types.Named]*types.Struct {
	selections := map[*ast.SelectorExpr]*types.Named{}
	operands := map[ast.Expr]bool{}
	structsOf := map[*types.Named]*types.Struct{}
	ast.Inspect(value, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.CompositeLit:
			if _, _, local := localNamedStruct(pkg, pkg.TypesInfo.TypeOf(typed)); local {
				return false
			}
		case *ast.SelectorExpr:
			selection, isSelection := pkg.TypesInfo.Selections[typed]
			if !isSelection || selection.Kind() != types.FieldVal {
				return true
			}
			named, st, isSDK := clientGoNamedStruct(selection.Recv())
			if !isSDK || nonResultSDKStruct(named) {
				return true
			}
			selections[typed] = named
			structsOf[named] = st
			operands[ast.Unparen(typed.X)] = true
		}
		return true
	})

	found := map[*types.Named]*types.Struct{}
	for selector, named := range selections {
		if operands[selector] {
			continue
		}
		found[named] = structsOf[named]
	}
	return found
}
