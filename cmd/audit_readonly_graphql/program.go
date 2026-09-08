package main

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/goprogram"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/graphqldocs"
)

// toolutilPath is the import path of the package that owns ActionSpec, the
// route constructors, and the shared GraphQL executors. Resolution keys on the
// path rather than on the package name so an import alias cannot fool it.
const toolutilPath = "github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"

// program is the loaded, indexed source the audit reasons over.
type program struct {
	fset *token.FileSet
	// pkgs is every loaded package keyed by import path.
	pkgs map[string]*packages.Package
	// funcs maps a declared function to its indexed body.
	funcs map[*types.Func]*function
	// documents maps the constant or variable a GraphQL document is declared
	// as to what its value asks GitLab to do.
	documents map[types.Object]documentKind
	// unattributed is every document in the shared inventory that neither the
	// object index nor the body walk could place: a .graphql file, or a
	// document written inline in a shape the body walk does not fold. Nothing
	// here can be tied to the handler that sends it, so the audit reports them
	// instead of passing them.
	unattributed []graphqldocs.Document
	// order is the loaded packages in the loader's order.
	order []*packages.Package
}

// docRef is one GraphQL document named inside a function body.
type docRef struct {
	kind documentKind
	// name is the constant this document was declared as, or "" when the
	// document is written inline at the point of use.
	name string
	// pos is where the body names it, which is the line a finding points at.
	pos token.Pos
	// declared is where the document itself is written.
	declared token.Pos
}

// function is one declared function with everything the audit reads from it.
type function struct {
	obj  *types.Func
	pkg  *packages.Package
	decl *ast.FuncDecl
	// calls is every function this body names. A reference counts as a call:
	// a function value handed to a helper is called by that helper, and the
	// audit would rather over-approximate the reachable set than miss a
	// mutation reached through a callback.
	calls map[*types.Func]bool
	// docs is every GraphQL document this body names.
	docs []docRef
	// sendsGraphQL reports whether this body reaches the GraphQL transport,
	// which is what makes a document it names a request rather than a string.
	sendsGraphQL bool
}

// graphQLSenders are the entry points that put a GraphQL document on the wire:
// the client-go GraphQL service method every call here ultimately reaches, and
// the shared toolutil executors that call it with a document their caller
// supplied.
var graphQLSenders = map[string]bool{
	"Do":                      true,
	"ExecGraphQLNoteMutation": true,
	"ExecGraphQLDestroyNote":  true,
}

// loadProgram loads and indexes the packages named by patterns, rooted at dir.
//
// The load itself, including the refusal of a package that did not type-check,
// belongs to [goprogram.Load]; what is here is the indexing this audit needs.
// The overlay is passed straight through: it is how a test supplies source
// that is not on disk, so a fixture package written in the test file itself
// type-checks against the real toolutil and the resolver is exercised on the
// shapes it has to handle rather than on a mock of them. Production passes nil.
func loadProgram(dir string, patterns []string, overlay map[string][]byte) (*program, error) {
	// The documents that live in .graphql files are read from the same trees
	// the patterns name, so the inventory this audit indexes is the inventory
	// the schema gate judges rather than a second, narrower reading of it.
	// They are read first for the reason graphqldocs.Collect reads them first:
	// it costs milliseconds and type-checking the tree costs seconds, so a run
	// that cannot read one of its own documents says so before paying the rest.
	standalone, err := graphqldocs.Standalone(dir, patterns)
	if err != nil {
		return nil, err
	}
	loaded, err := goprogram.Load(dir, patterns, overlay)
	if err != nil {
		return nil, err
	}
	prog := &program{
		fset:      loaded[0].Fset,
		pkgs:      make(map[string]*packages.Package, len(loaded)),
		funcs:     make(map[*types.Func]*function),
		documents: make(map[types.Object]documentKind),
		order:     loaded,
	}
	for _, pkg := range prog.order {
		prog.pkgs[pkg.PkgPath] = pkg
	}
	inventory := append(graphqldocs.FromPackages(loaded), standalone...)
	prog.indexDocuments(inventory)
	for _, pkg := range prog.order {
		prog.indexFunctions(pkg)
	}
	// The bodies have to be indexed first: an inline document is placed by the
	// body that writes it, and until the walk has run nothing has placed one.
	prog.unattributed = prog.unattributedDocuments(inventory)
	return prog, nil
}

// indexDocuments records what every named document in the shared inventory
// asks GitLab to do, keyed by the object that declares it.
//
// The inventory comes from cmd/internal/graphqldocs, the same reading the
// schema gate judges, so a document moved into a .graphql file or assembled
// from a shared fragment is seen here too; what stays this audit's own is the
// rule that says what a document asks for, because the schema gate has no
// opinion about read against write.
//
// Constants are folded by the type checker, so a document assembled from a
// shared fragment constant is indexed with the fragment already spliced in,
// which is how the vulnerability state mutations are written.
func (p *program) indexDocuments(inventory []graphqldocs.Document) {
	for _, document := range inventory {
		if document.Object == nil {
			continue
		}
		if kind := classifyDocument(document.Text); kind != notADocument {
			p.documents[document.Object] = kind
		}
	}
}

// unattributedDocuments returns the inventory entries this audit cannot tie to
// anything it walks.
//
// A document is placed either by the object that declares it, which the call
// graph resolves at every use, or by the body that writes it inline, which
// [program.indexBody] records at the position of the literal. A document with
// neither is one the reachability walk can never reach: a .graphql file belongs
// to no function, and an inline document written in a shape the body walk does
// not fold, such as a concatenation or a literal outside every function body,
// is a string this audit never classifies. Both are reported, because the
// alternative is a gate that answers "no read-only action reaches a mutation"
// when what it means is "none of the ones I could see".
func (p *program) unattributedDocuments(inventory []graphqldocs.Document) []graphqldocs.Document {
	inline := make(map[token.Position]bool)
	for _, fn := range p.funcs {
		for _, doc := range fn.docs {
			if doc.name == "" {
				inline[p.position(doc.pos)] = true
			}
		}
	}
	var found []graphqldocs.Document
	for _, document := range inventory {
		if document.Object != nil || inline[document.Position] {
			continue
		}
		found = append(found, document)
	}
	return found
}

// constantString unwraps a string constant value.
func constantString(value constant.Value) (string, bool) {
	if value == nil || value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(value), true
}

// indexFunctions records every declared function's body.
//
// TypesInfo is read unchecked here too, for the reason [program.indexDocuments]
// gives: [goprogram.Load] refuses a package that did not type-check.
func (p *program) indexFunctions(pkg *packages.Package) {
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Body == nil {
				continue
			}
			obj, ok := pkg.TypesInfo.Defs[funcDecl.Name].(*types.Func)
			if !ok {
				continue
			}
			p.funcs[obj] = p.indexBody(pkg, obj, funcDecl)
		}
	}
}

// indexBody walks one function body. Function literals inside it are walked as
// part of it: a closure a handler defines runs when that handler runs, so
// folding it into the enclosing function keeps the reachable set honest
// without a separate node per literal.
func (p *program) indexBody(pkg *packages.Package, obj *types.Func, decl *ast.FuncDecl) *function {
	fn := &function{obj: obj, pkg: pkg, decl: decl, calls: make(map[*types.Func]bool)}
	seen := make(map[token.Pos]bool)
	ast.Inspect(decl.Body, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.Ident:
			p.recordUse(fn, pkg, typed, seen)
		case *ast.BasicLit:
			if typed.Kind == token.STRING {
				p.recordLiteral(fn, pkg, typed, seen)
			}
		}
		return true
	})
	return fn
}

// recordUse records what one identifier in a body names.
func (p *program) recordUse(fn *function, pkg *packages.Package, ident *ast.Ident, seen map[token.Pos]bool) {
	obj := pkg.TypesInfo.Uses[ident]
	if obj == nil {
		return
	}
	if callee, ok := obj.(*types.Func); ok {
		fn.calls[callee] = true
		if graphQLSenders[callee.Name()] && isGraphQLSender(callee) {
			fn.sendsGraphQL = true
		}
		return
	}
	kind, ok := p.documents[obj]
	if !ok || seen[ident.Pos()] {
		return
	}
	seen[ident.Pos()] = true
	fn.docs = append(fn.docs, docRef{kind: kind, name: obj.Name(), pos: ident.Pos(), declared: obj.Pos()})
}

// recordLiteral records a GraphQL document written inline rather than as a
// named constant.
func (p *program) recordLiteral(fn *function, pkg *packages.Package, lit *ast.BasicLit, seen map[token.Pos]bool) {
	tv, ok := pkg.TypesInfo.Types[lit]
	if !ok || tv.Value == nil || seen[lit.Pos()] {
		return
	}
	value, ok := constantString(tv.Value)
	if !ok {
		return
	}
	kind := classifyDocument(value)
	if kind == notADocument {
		return
	}
	seen[lit.Pos()] = true
	fn.docs = append(fn.docs, docRef{kind: kind, pos: lit.Pos(), declared: lit.Pos()})
}

// isGraphQLSender reports whether a method named Do (or one of the shared
// executors) belongs to the GraphQL transport rather than to some unrelated
// type that happens to have a method with the same name.
func isGraphQLSender(callee *types.Func) bool {
	if callee.Pkg() != nil && callee.Pkg().Path() == toolutilPath {
		return true
	}
	sig, ok := callee.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return false
	}
	return strings.Contains(sig.Recv().Type().String(), "GraphQL")
}

// reachable returns the transitive closure of functions a handler can run,
// including the handlers themselves.
func (p *program) reachable(roots []*types.Func) map[*types.Func]bool {
	seen := make(map[*types.Func]bool, len(roots))
	queue := append([]*types.Func(nil), roots...)
	for len(queue) > 0 {
		current := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if seen[current] {
			continue
		}
		seen[current] = true
		fn, ok := p.funcs[current]
		if !ok {
			continue
		}
		for callee := range fn.calls {
			if !seen[callee] {
				queue = append(queue, callee)
			}
		}
	}
	return seen
}

// position renders a source position.
func (p *program) position(pos token.Pos) token.Position {
	return p.fset.Position(pos)
}
