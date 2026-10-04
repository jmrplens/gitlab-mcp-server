package actionrequests

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
)

// toolutilPath is the gates' shared spelling of the package that owns
// ActionSpec, the route constructors and the shared GraphQL executors, kept
// under the name this package reads it by: the path is written once, in
// [goprogram.ToolutilPath], so a module move lands in every gate at once.
const toolutilPath = goprogram.ToolutilPath

// clientGoPath is the import path of client-go's root package, whose service
// methods are the requests a handler hands to the SDK.
const clientGoPath = "gitlab.com/gitlab-org/api/client-go/v3"

// serviceSuffixes are the endings a client-go type that groups requests is
// named with: the interface a handler holds (IssuesServiceInterface) and the
// struct behind it (IssuesService). The first that matches is cut, so the key
// is the one [sdkroutes] gives the same method.
var serviceSuffixes = []string{"ServiceInterface", "Service"}

// Program is the loaded, indexed source the resolution and the walk reason
// over.
//
// It carries what a reader of it consults and nothing else. Two fields used to
// sit beside these and be read by nothing: a map of the loaded packages by
// import path, filled at load, and on each [Function] the object it was
// declared as, which funcs is already keyed by. A field no reader consults is
// one no test can hold, so both are gone.
type Program struct {
	fset *token.FileSet
	// funcs maps a declared function, or the stand-in for a package-level
	// variable's initializer, to its indexed body.
	funcs map[*types.Func]*Function
	// documents maps the constant or variable a GraphQL document is declared
	// as to its text.
	documents map[types.Object]string
	// initializers maps a package-level variable to the stand-in function its
	// initializer was indexed as, which is what a body naming the variable
	// reaches.
	initializers map[*types.Var]*types.Func
	// unattributed is every document in the shared inventory that neither the
	// object index nor the body walk could place: a .graphql file, or a
	// document written inline in a shape the body walk does not fold. Nothing
	// here can be tied to the handler that sends it.
	unattributed []graphqldocs.Document
	// order is the loaded packages in the loader's order.
	order []*packages.Package
}

// DocumentUse is one GraphQL document named inside a body.
type DocumentUse struct {
	// Text is the document as GitLab receives it, fragments spliced in.
	Text string
	// Name is the constant or variable the document was declared as, or ""
	// when it is written inline at the point of use.
	Name string
	// Pos is where the body names it, which is the line inside the handler a
	// reader has to change, and so the line a finding points at.
	Pos token.Pos
}

// Function is one indexed body with everything a reader of the walk asks of
// it. The exported fields are what it found; the rest is how it is walked.
type Function struct {
	// Documents is every GraphQL document this body names.
	Documents []DocumentUse
	// SendsGraphQL reports whether this body reaches the GraphQL transport,
	// which is what makes a document it names a request rather than a string.
	SendsGraphQL bool
	// SDKMethods is every client-go service method this body names, as
	// "Service.Method", sorted. A method value handed to a helper counts, for
	// the reason a function reference counts as a call.
	SDKMethods []string

	pkg  *packages.Package
	decl *ast.FuncDecl
	// calls is every function this body names. A reference counts as a call:
	// a function value handed to a helper is called by that helper, and the
	// walk would rather over-approximate the reachable set than miss a request
	// reached through a callback.
	calls map[*types.Func]bool
	// variables is every package-level variable this body names, resolved to
	// the stand-ins of their initializers once every package is indexed.
	variables []*types.Var
	sdk       map[string]bool
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

// Load loads and indexes the packages named by patterns, rooted at dir.
//
// The load itself, including the refusal of a package that did not type-check,
// belongs to [goprogram.Load]; what is here is the indexing the walk needs.
// The overlay is passed straight through: it is how a test supplies source
// that is not on disk, so a fixture package written in a test file
// type-checks against the real toolutil and the resolver is exercised on the
// shapes it has to handle rather than on a mock of them. Production passes nil.
func Load(dir string, patterns []string, overlay map[string][]byte) (*Program, error) {
	// The documents that live in .graphql files are read from the same trees
	// the patterns name, so the inventory indexed here is the inventory the
	// schema gate judges rather than a second, narrower reading of it. They
	// are read first for the reason graphqldocs.Collect reads them first: it
	// costs milliseconds and type-checking the tree costs seconds, so a run
	// that cannot read one of its own documents says so before paying the rest.
	standalone, err := graphqldocs.Standalone(dir, patterns)
	if err != nil {
		return nil, err
	}
	loaded, err := goprogram.Load(dir, patterns, overlay)
	if err != nil {
		return nil, err
	}
	prog := &Program{
		fset:         loaded[0].Fset,
		funcs:        make(map[*types.Func]*Function),
		documents:    make(map[types.Object]string),
		initializers: make(map[*types.Var]*types.Func),
		order:        loaded,
	}
	inventory := append(graphqldocs.FromPackages(loaded), standalone...)
	prog.indexDocuments(inventory)
	for _, pkg := range prog.order {
		prog.indexFunctions(pkg)
		prog.indexInitializers(pkg)
	}
	prog.linkVariables()
	// The bodies have to be indexed first: an inline document is placed by the
	// body that writes it, and until the walk has run nothing has placed one.
	prog.unattributed = prog.unattributedDocuments(inventory)
	return prog, nil
}

// indexDocuments records the text of every named document in the shared
// inventory, keyed by the object that declares it.
//
// The inventory comes from cmd/internal/graphqldocs, the same reading the
// schema gate judges, so a document moved into a .graphql file or assembled
// from a shared fragment is seen here too. Constants are folded by the type
// checker, so a document assembled from a shared fragment constant is indexed
// with the fragment already spliced in, which is how the vulnerability state
// mutations are written. What a document asks GitLab to do is each reader's
// own question, so nothing is classified here.
func (p *Program) indexDocuments(inventory []graphqldocs.Document) {
	for _, document := range inventory {
		if document.Object == nil {
			continue
		}
		p.documents[document.Object] = document.Text
	}
}

// unattributedDocuments returns the inventory entries nothing walked here can
// tie to a body.
//
// A document is placed either by the object that declares it, which the call
// graph resolves at every use, or by the body that writes it inline, which
// [Program.indexBody] records at the position of the literal. A document with
// neither is one the reachability walk can never reach: a .graphql file belongs
// to no function, and an inline document written in a shape the body walk does
// not fold, such as a concatenation or a literal outside every function body,
// is a string nothing here classifies.
func (p *Program) unattributedDocuments(inventory []graphqldocs.Document) []graphqldocs.Document {
	inline := make(map[token.Position]bool)
	for _, fn := range p.funcs {
		for _, doc := range fn.Documents {
			if doc.Name == "" {
				inline[p.Position(doc.Pos)] = true
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
// TypesInfo is read unchecked here, for the reason [Program.indexDocuments]
// gives: [goprogram.Load] refuses a package that did not type-check.
func (p *Program) indexFunctions(pkg *packages.Package) {
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
			fn := p.indexBody(pkg, funcDecl.Body)
			fn.decl = funcDecl
			p.funcs[obj] = fn
		}
	}
}

// indexInitializers indexes the initializer of every package-level variable as
// a function of its own, under a stand-in object named after the variable.
//
// A handler reaches a request through a package-level variable in three shapes
// this tree writes: a test seam holding a function literal
// (`var newRawRequest = func(...)` in the group boards), a dispatch table of
// literals selected by a key (`integrationGetters`), and a method expression
// (`var newRequest = (*gl.Client).NewRequest` in commits). A body that names
// the variable reaches whatever its initializer names, which is what the
// stand-in records; a table is reached whole, because which entry runs is
// decided by the caller's input. The stand-in has no body of its own to
// resolve a return in, so its declaration carries none.
func (p *Program) indexInitializers(pkg *packages.Package) {
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				if valueSpec, isValue := spec.(*ast.ValueSpec); isValue {
					p.indexValueSpec(pkg, valueSpec)
				}
			}
		}
	}
}

// indexValueSpec indexes the initializers of one package-level var spec, each
// name against the value it is initialized from. A spec whose one value is a
// call returning several results initializes every name from that call, so
// each name is indexed against it; a spec with no values initializes nothing
// and has nothing to index.
func (p *Program) indexValueSpec(pkg *packages.Package, spec *ast.ValueSpec) {
	if len(spec.Values) == 0 {
		return
	}
	for i, name := range spec.Names {
		value := spec.Values[min(i, len(spec.Values)-1)]
		variable, defined := pkg.TypesInfo.Defs[name].(*types.Var)
		if !defined {
			continue
		}
		stand := types.NewFunc(name.Pos(), pkg.Types, name.Name, types.NewSignatureType(nil, nil, nil, nil, nil, false))
		fn := p.indexBody(pkg, value)
		fn.decl = &ast.FuncDecl{Name: name}
		p.funcs[stand] = fn
		p.initializers[variable] = stand
	}
}

// linkVariables links every indexed body. It runs once every package is
// indexed, because a body may name a variable of a package indexed after it.
func (p *Program) linkVariables() {
	for _, fn := range p.funcs {
		p.link(fn)
	}
}

// link turns every package-level variable a body names into a call of its
// initializer's stand-in. A variable of a package outside the load has no
// stand-in, and contributes nothing, as a function outside it does.
func (p *Program) link(fn *Function) {
	for _, variable := range fn.variables {
		if stand, ok := p.initializers[variable]; ok {
			fn.calls[stand] = true
		}
	}
	fn.variables = nil
}

// indexBody walks one body. Function literals inside it are walked as part of
// it: a closure a handler defines runs when that handler runs, so folding it
// into the enclosing function keeps the reachable set honest without a
// separate node per literal.
func (p *Program) indexBody(pkg *packages.Package, root ast.Node) *Function {
	fn := &Function{pkg: pkg, calls: make(map[*types.Func]bool), sdk: make(map[string]bool)}
	ast.Inspect(root, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.Ident:
			p.recordUse(fn, pkg, typed)
		case *ast.BasicLit:
			if typed.Kind == token.STRING {
				p.recordLiteral(fn, pkg, typed)
			}
		}
		return true
	})
	for method := range fn.sdk {
		fn.SDKMethods = append(fn.SDKMethods, method)
	}
	sort.Strings(fn.SDKMethods)
	return fn
}

// recordUse records what one identifier in a body names.
//
// Both recorders used to share a set of the positions they had already
// recorded, which nothing could ever observe: [ast.Inspect] visits each node of
// a body once, and two nodes of these two kinds cannot share a position, so the
// set was written 102 times in the suite and read back false every one of them.
func (p *Program) recordUse(fn *Function, pkg *packages.Package, ident *ast.Ident) {
	obj := pkg.TypesInfo.Uses[ident]
	if obj == nil {
		return
	}
	if callee, ok := obj.(*types.Func); ok {
		fn.calls[callee] = true
		if graphQLSenders[callee.Name()] && isGraphQLSender(callee) {
			fn.SendsGraphQL = true
		}
		if method, isService := serviceMethod(callee); isService {
			fn.sdk[method] = true
		}
		return
	}
	if text, ok := p.documents[obj]; ok {
		fn.Documents = append(fn.Documents, DocumentUse{Text: text, Name: obj.Name(), Pos: ident.Pos()})
		return
	}
	if variable, ok := obj.(*types.Var); ok && isPackageLevel(variable) {
		fn.variables = append(fn.variables, variable)
	}
}

// isPackageLevel reports whether a variable is declared at package scope, the
// one place an initializer runs once for every body that names it. A field, a
// parameter and a local all have a scope of their own or none.
func isPackageLevel(variable *types.Var) bool {
	scope := variable.Parent()
	return scope != nil && scope.Parent() == types.Universe
}

// recordLiteral records a GraphQL document written inline rather than as a
// named constant.
//
// What the type checker knows about the literal is read straight into
// [constantString], because the two guards that used to stand in front of it
// answered nothing it does not: a literal the type information has no entry for
// yields the zero [types.TypeAndValue], whose value is nil, and a nil value is
// not a string constant. Whether the string is a document is the inventory's
// rule, [graphqldocs.LooksLikeDocument], so a literal is placed here exactly
// when the inventory holds it.
func (p *Program) recordLiteral(fn *Function, pkg *packages.Package, lit *ast.BasicLit) {
	value, ok := constantString(pkg.TypesInfo.Types[lit].Value)
	if !ok || !graphqldocs.LooksLikeDocument(value) {
		return
	}
	fn.Documents = append(fn.Documents, DocumentUse{Text: value, Pos: lit.Pos()})
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

// serviceMethod names a client-go service method the way [sdkroutes] keys it,
// "Issues.GetIssue", and reports whether callee is one: a method of a named
// client-go type whose name ends in one of [serviceSuffixes].
func serviceMethod(callee *types.Func) (string, bool) {
	recv := callee.Signature().Recv()
	if recv == nil {
		return "", false
	}
	typ := recv.Type()
	if pointer, ok := typ.(*types.Pointer); ok {
		typ = pointer.Elem()
	}
	named, ok := typ.(*types.Named)
	if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != clientGoPath {
		return "", false
	}
	for _, suffix := range serviceSuffixes {
		if service, cut := strings.CutSuffix(named.Obj().Name(), suffix); cut && service != "" {
			return service + "." + callee.Name(), true
		}
	}
	return "", false
}

// Reachable returns the transitive closure of functions a handler can run,
// including the handlers themselves. It is unbounded, with a cycle guard: the
// requests the deeper calls make are the second and third requests of the
// actions that make several, so a depth bound loses them.
func (p *Program) Reachable(roots []*types.Func) map[*types.Func]bool {
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

// Function returns the indexed body of a function, or false for one this
// program holds no body for: a function of a package outside the load, or a
// method of an interface.
func (p *Program) Function(fn *types.Func) (*Function, bool) {
	found, ok := p.funcs[fn]
	return found, ok
}

// Packages returns the loaded packages in the loader's order, for a reader
// that reads something of its own out of the syntax, such as a directive.
func (p *Program) Packages() []*packages.Package {
	return p.order
}

// Unattributed returns every GraphQL document the inventory holds that nothing
// here can tie to a body. A reader that answers for what its handlers send has
// to report these rather than pass them, since one could be a mutation a
// handler sends.
func (p *Program) Unattributed() []graphqldocs.Document {
	return p.unattributed
}

// Position renders a source position.
func (p *Program) Position(pos token.Pos) token.Position {
	return p.fset.Position(pos)
}
