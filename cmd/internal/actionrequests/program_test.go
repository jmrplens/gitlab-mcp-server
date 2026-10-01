package actionrequests

import (
	"errors"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests/actionfixture"
)

// repoRoot is the module root the fixture overlay is written under.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	root, err := actionfixture.Root(dir)
	if err != nil {
		t.Fatalf("module root: %v", err)
	}
	return root
}

// fixtureCache memoizes one loaded program per fixture source set. Loading is
// a full type-check of the fixture against toolutil and takes seconds, the
// result is read-only for everything the tests ask of it but the stand-ins
// [Program.Roots] adds, which no other test reads, and the tests run in one
// goroutine, so paying for it once per source set keeps the package's tests
// from spending a minute re-parsing the same few packages.
var fixtureCache = map[string]*Program{}

// loadFixture loads the fixture packages described by sources.
func loadFixture(t *testing.T, sources map[string]string) *Program {
	t.Helper()
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)
	key := strings.Join(names, "|")
	if cached, ok := fixtureCache[key]; ok {
		return cached
	}
	root := repoRoot(t)
	prog, err := Load(root, []string{actionfixture.Pattern}, actionfixture.Overlay(root, sources))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	fixtureCache[key] = prog
	return prog
}

// vulnSources is the fixture set holding only the vulnerability handlers.
func vulnSources() map[string]string {
	return map[string]string{"vuln": actionfixture.Vuln}
}

// requestSources is the fixture set holding the request shapes.
func requestSources() map[string]string {
	return map[string]string{"requests": actionfixture.Requests}
}

// lookupFunc finds a declared function in the loaded program by package name
// and function name. A stand-in for an initializer or a literal carries no
// body of its own and is passed over, since it may share a declared
// function's name.
func lookupFunc(t *testing.T, prog *Program, pkgName, funcName string) *types.Func {
	t.Helper()
	for fn, body := range prog.funcs {
		if body.decl.Body == nil {
			continue
		}
		if fn.Pkg() != nil && fn.Pkg().Name() == pkgName && fn.Name() == funcName {
			return fn
		}
	}
	t.Fatalf("function %s.%s not found in loaded program", pkgName, funcName)
	return nil
}

// lookupInitializer finds the stand-in a package-level variable's initializer
// was indexed as.
func lookupInitializer(t *testing.T, prog *Program, pkgName, varName string) *types.Func {
	t.Helper()
	for variable, stand := range prog.initializers {
		if variable.Pkg() != nil && variable.Pkg().Name() == pkgName && variable.Name() == varName {
			return stand
		}
	}
	t.Fatalf("variable %s.%s has no indexed initializer", pkgName, varName)
	return nil
}

// documentTexts returns the text of every document a package's objects
// declare, keyed by the object's name.
func documentTexts(prog *Program, pkgName string) map[string]string {
	texts := map[string]string{}
	for obj, text := range prog.documents {
		if obj.Pkg() != nil && obj.Pkg().Name() == pkgName {
			texts[obj.Name()] = text
		}
	}
	return texts
}

// TestLoad_FixturePackages_IndexesDocumentsAndFunctions verifies the loader
// indexes an overlay-only package: its functions get bodies, its GraphQL
// constants get their text, and the function that calls the GraphQL transport
// is marked as sending.
func TestLoad_FixturePackages_IndexesDocumentsAndFunctions(t *testing.T) {
	prog := loadFixture(t, vulnSources())

	send := lookupFunc(t, prog, "vuln", "send")
	if !prog.funcs[send].SendsGraphQL {
		t.Error("send() calls GraphQL.Do but was not marked as sending GraphQL")
	}

	texts := documentTexts(prog, "vuln")
	if !strings.Contains(texts["listQuery"], "vulnerabilities { nodes { id } }") {
		t.Errorf("listQuery indexed as %q, want its query", texts["listQuery"])
	}
	if !strings.Contains(texts["dismissMutation"], "vulnerabilityDismiss") {
		t.Errorf("dismissMutation indexed as %q, want its mutation", texts["dismissMutation"])
	}
}

// TestLoad_NoMatchingPattern_ReturnsError verifies that a pattern matching
// nothing is an error rather than an empty, silently passing walk. A testdata
// directory is the pattern that matches nothing without the toolchain calling
// it an error: the go command skips testdata, so the directory exists and the
// pattern still resolves to no packages.
func TestLoad_NoMatchingPattern_ReturnsError(t *testing.T) {
	_, err := Load(repoRoot(t), []string{"./internal/tools/testdata/..."}, nil)
	if err == nil {
		t.Fatal("loading a pattern that matches nothing must fail")
	}
	if !strings.Contains(err.Error(), "no packages matched") {
		t.Errorf("error %q does not say the pattern matched nothing", err)
	}
}

// TestLoad_BrokenFixture_ReturnsLoadError verifies a package that does not
// compile is reported rather than treated as a package with no handlers,
// which would make every action in it silently unclassifiable.
func TestLoad_BrokenFixture_ReturnsLoadError(t *testing.T) {
	root := repoRoot(t)
	_, err := Load(root, []string{actionfixture.Pattern}, actionfixture.Overlay(root, map[string]string{
		"broken": "package broken\n\nfunc Broken() int { return \"not an int\" }\n",
	}))
	if err == nil {
		t.Fatal("a fixture that does not type-check must fail the load")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("error %q does not name the package that failed", err)
	}
}

// TestProgram_Reachable_FollowsCallees verifies the call graph reaches a
// function only named through an intermediate callee, and stops at functions
// nothing names.
func TestProgram_Reachable_FollowsCallees(t *testing.T) {
	prog := loadFixture(t, vulnSources())
	indirect := lookupFunc(t, prog, "vuln", "DismissIndirect")
	dismissAll := lookupFunc(t, prog, "vuln", "dismissAll")
	send := lookupFunc(t, prog, "vuln", "send")
	list := lookupFunc(t, prog, "vuln", "List")

	reached := prog.Reachable([]*types.Func{indirect})
	for _, want := range []*types.Func{indirect, dismissAll, send} {
		t.Run(want.Name(), func(t *testing.T) {
			if !reached[want] {
				t.Errorf("reachable from DismissIndirect does not contain %s", want.Name())
			}
		})
	}
	if reached[list] {
		t.Error("reachable from DismissIndirect wrongly contains List")
	}
}

// TestProgram_Reachable_NoRoots_IsEmpty verifies the closure of nothing is
// nothing, which is the shape an unresolved handler would produce.
func TestProgram_Reachable_NoRoots_IsEmpty(t *testing.T) {
	prog := loadFixture(t, vulnSources())
	if reached := prog.Reachable(nil); len(reached) != 0 {
		t.Errorf("Reachable(nil) returned %d functions, want 0", len(reached))
	}
}

// TestProgram_Reachable_RepeatedRoot_IsVisitedOnce verifies the closure stops
// at a function it has already reached. Nothing deduplicates the roots it is
// given, and a handler that calls a helper the closure also reaches puts the
// same function on the queue twice, so without this the walk would re-expand
// it and a cycle would never end.
func TestProgram_Reachable_RepeatedRoot_IsVisitedOnce(t *testing.T) {
	prog := loadFixture(t, vulnSources())
	send := lookupFunc(t, prog, "vuln", "send")
	dismissAll := lookupFunc(t, prog, "vuln", "dismissAll")

	reached := prog.Reachable([]*types.Func{dismissAll, dismissAll, send})

	for _, want := range []*types.Func{dismissAll, send} {
		t.Run(want.Name(), func(t *testing.T) {
			if !reached[want] {
				t.Errorf("reachable from a repeated root does not contain %s", want.Name())
			}
		})
	}
}

// TestProgram_Function_AnswersForIndexedBodiesOnly verifies the lookup a
// reader of the walk uses: an indexed function is returned, and a function
// the program holds no body for (one of a package outside the load) is not.
func TestProgram_Function_AnswersForIndexedBodiesOnly(t *testing.T) {
	prog := loadFixture(t, vulnSources())
	send := lookupFunc(t, prog, "vuln", "send")

	if fn, ok := prog.Function(send); !ok || fn != prog.funcs[send] {
		t.Errorf("Function(send) = %v, %t, want its indexed body", fn, ok)
	}
	if fn, ok := prog.Function(types.NewFunc(token.NoPos, nil, "Elsewhere", nil)); ok || fn != nil {
		t.Errorf("Function() of an unloaded function = %v, %t, want nothing", fn, ok)
	}
}

// TestProgram_Initializer_AnswersForIndexedVariablesOnly verifies the stand-in
// a package-level variable's initializer was indexed as is handed to a reader
// that walks it, and that a variable with no indexed initializer gets nothing.
func TestProgram_Initializer_AnswersForIndexedVariablesOnly(t *testing.T) {
	prog := loadFixture(t, requestSources())
	want := lookupInitializer(t, prog, "requests", "first")
	var first *types.Var
	for variable, stand := range prog.initializers {
		if stand == want {
			first = variable
		}
	}

	if stand, ok := prog.Initializer(first); !ok || stand != want {
		t.Errorf("Initializer(first) = %v, %t, want its stand-in", stand, ok)
	}
	if stand, ok := prog.Initializer(types.NewVar(token.NoPos, nil, "elsewhere", types.Typ[types.Int])); ok || stand != nil {
		t.Errorf("Initializer() of an unindexed variable = %v, %t, want nothing", stand, ok)
	}
}

// TestProgram_Document_AnswersForDeclaringObjectsOnly verifies the text of a
// named document is handed to a reader that meets the name, and that an
// object declaring no document gets nothing.
func TestProgram_Document_AnswersForDeclaringObjectsOnly(t *testing.T) {
	prog := loadFixture(t, vulnSources())
	var listQuery types.Object
	for obj := range prog.documents {
		if obj.Pkg() != nil && obj.Pkg().Name() == "vuln" && obj.Name() == "listQuery" {
			listQuery = obj
		}
	}

	if text, ok := prog.Document(listQuery); !ok || !strings.Contains(text, "vulnerabilities") {
		t.Errorf("Document(listQuery) = %q, %t, want the list query", text, ok)
	}
	if text, ok := prog.Document(lookupFunc(t, prog, "vuln", "send")); ok || text != "" {
		t.Errorf("Document() of a function = %q, %t, want nothing", text, ok)
	}
}

// TestFunction_Accessors_HandTheIndexedBodyToAWalker verifies what a walker
// of a body reads off it: the package it resolves names through, the node it
// was indexed from, and the declaration its parameters bind to, which a
// stand-in has none of.
func TestFunction_Accessors_HandTheIndexedBodyToAWalker(t *testing.T) {
	prog := loadFixture(t, vulnSources())
	send, _ := prog.Function(lookupFunc(t, prog, "vuln", "send"))

	if send.Package() == nil || send.Package().Name != "vuln" {
		t.Errorf("Package() = %v, want vuln", send.Package())
	}
	if send.Decl() == nil || send.Root() != send.Decl().Body {
		t.Errorf("Root() = %v, Decl() = %v, want the declaration's body and the declaration", send.Root(), send.Decl())
	}
	for _, testCase := range []struct {
		name string
		fn   *Function
	}{
		{name: "stand-in", fn: &Function{}},
		{name: "declaration without a body", fn: &Function{decl: &ast.FuncDecl{}}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if decl := testCase.fn.Decl(); decl != nil {
				t.Errorf("Decl() = %v, want nil", decl)
			}
		})
	}
}

// TestFunction_Bound_NamesWhatTheHelperWasHandedForOneVariable verifies a
// handler literal answers, per variable it names, the functions its route
// helper's caller bound there, and nothing for a variable nothing bound.
func TestFunction_Bound_NamesWhatTheHelperWasHandedForOneVariable(t *testing.T) {
	handler := types.NewVar(token.NoPos, nil, "fn", types.NewSignatureType(nil, nil, nil, nil, nil, false))
	deleteFn := types.NewFunc(token.NoPos, nil, "Delete", nil)
	lit := &Function{bound: map[*types.Var][]*types.Func{handler: {deleteFn}}}

	if got := lit.Bound(handler); !slices.Equal(got, []*types.Func{deleteFn}) {
		t.Errorf("Bound(fn) = %v, want [Delete]", got)
	}
	if got := lit.Bound(types.NewVar(token.NoPos, nil, "other", types.Typ[types.Int])); got != nil {
		t.Errorf("Bound(other) = %v, want nothing", got)
	}
}

// TestProgram_Packages_AreTheLoadedOnesInOrder verifies a reader that reads
// its own directives out of the syntax is handed every loaded package.
func TestProgram_Packages_AreTheLoadedOnesInOrder(t *testing.T) {
	prog := loadFixture(t, actionfixture.Main())

	var names []string
	for _, pkg := range prog.Packages() {
		names = append(names, pkg.Name)
	}
	sort.Strings(names)
	if want := []string{"other", "shapes", "vars", "vuln"}; !slices.Equal(names, want) {
		t.Errorf("Packages() = %v, want %v", names, want)
	}
}

// TestIsGraphQLSender_SharedToolutilExecutor_IsATransport verifies the package
// half of the transport check. The shared executors send the document their
// caller supplied, and they are ordinary functions rather than methods on a
// GraphQL service, so a check that only read receivers would miss every note
// domain and report the actions that reach them as touching no GraphQL at all.
func TestIsGraphQLSender_SharedToolutilExecutor_IsATransport(t *testing.T) {
	prog := loadFixture(t, actionfixture.Main())
	viaExecutor := prog.funcs[lookupFunc(t, prog, "shapes", "ViaExecutor")]

	if !viaExecutor.SendsGraphQL {
		t.Error("a handler calling the shared toolutil executor was not marked as sending GraphQL")
	}
}

// TestIsGraphQLSender_Classification verifies the transport check accepts the
// client-go GraphQL service method and the shared toolutil executors, and
// rejects a method named Do on something else.
func TestIsGraphQLSender_Classification(t *testing.T) {
	prog := loadFixture(t, vulnSources())
	send := prog.funcs[lookupFunc(t, prog, "vuln", "send")]

	var graphQLDo *types.Func
	for callee := range send.calls {
		if callee.Name() == "Do" {
			graphQLDo = callee
		}
	}
	if graphQLDo == nil {
		t.Fatal("send() does not name a Do method, so the fixture no longer exercises the transport check")
	}
	if !isGraphQLSender(graphQLDo) {
		t.Error("the client-go GraphQL Do method was not recognized as a sender")
	}
	// A plain function with no receiver, from a package that is not toolutil,
	// must not count as a transport.
	if isGraphQLSender(lookupFunc(t, prog, "vuln", "List")) {
		t.Error("an ordinary handler was wrongly recognized as a GraphQL sender")
	}
}

// nonTransportDo is a method named Do on a type that has nothing to do with
// GraphQL, which is what the name half of the transport check cannot tell apart
// on its own: client-go's GraphQL service is reached as Do, and so is every
// http.Client in the module.
func nonTransportDo() *types.Func {
	pkg := types.NewPackage("example.com/transport", "transport")
	client := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Client", nil), types.NewStruct(nil, nil), nil)
	recv := types.NewVar(token.NoPos, pkg, "c", client)
	return types.NewFunc(token.NoPos, pkg, "Do", types.NewSignatureType(recv, nil, nil, nil, nil, false))
}

// TestIsGraphQLSender_CalleesThatPutNoDocumentOnTheWire_AreRefused verifies the
// three ways the transport check answers no. A function with no package cannot
// be one of toolutil's executors, a function the type checker gave no signature
// has no receiver to read, and a method named Do on an ordinary type is the
// case the name alone cannot decide: every http.Client in the module is reached
// through one.
func TestIsGraphQLSender_CalleesThatPutNoDocumentOnTheWire_AreRefused(t *testing.T) {
	cases := []struct {
		name   string
		callee *types.Func
	}{
		{name: "a function belonging to no package", callee: types.NewFunc(token.NoPos, nil, "Do", nil)},
		{
			name:   "a function with no signature to read",
			callee: types.NewFunc(token.NoPos, types.NewPackage("example.com/other", "other"), "Do", nil),
		},
		{name: "a method named Do on an ordinary type", callee: nonTransportDo()},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if isGraphQLSender(testCase.callee) {
				t.Error("isGraphQLSender() = true, want false")
			}
		})
	}
}

// TestRecordUse_CallToAMethodNamedDoOnSomethingElse_IsNoTransport verifies the
// body walk asks both halves of the transport question. The name is checked
// first because it is cheap, but a body that calls some other Do has not sent a
// GraphQL document, and marking it as sending would put every document it names
// on the list of requests a reader answers for.
func TestRecordUse_CallToAMethodNamedDoOnSomethingElse_IsNoTransport(t *testing.T) {
	info := synthInfo()
	ident := ast.NewIdent("Do")
	callee := nonTransportDo()
	info.Uses[ident] = callee
	body := &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: ident}}}

	fn := (&Program{}).indexBody(&packages.Package{TypesInfo: info}, body)

	if fn.SendsGraphQL {
		t.Error("indexBody() marked a body calling an unrelated Do as sending GraphQL")
	}
	if !fn.calls[callee] {
		t.Error("indexBody() did not record the call at all, so the assertion above proves nothing")
	}
}

// TestLoad_PackageLevelVariables_IndexesOnlyConstantDocuments verifies the two
// shapes a document can be written in as a variable: one initialized from a
// constant string, which is indexed, and one initialized from a call, whose
// value is not knowable from the source and so is not.
func TestLoad_PackageLevelVariables_IndexesOnlyConstantDocuments(t *testing.T) {
	prog := loadFixture(t, map[string]string{"vars": actionfixture.Vars})

	texts := documentTexts(prog, "vars")
	if !strings.Contains(texts["declaredMutation"], "thing(input: {id: $id})") {
		t.Errorf("declaredMutation indexed as %q, want its mutation", texts["declaredMutation"])
	}
	if text, ok := texts["computedMutation"]; ok {
		t.Errorf("a variable initialized from a call was indexed as the document %q", text)
	}
	if text, ok := texts["undeclared"]; ok {
		t.Errorf("a variable with no initializer was indexed as the document %q", text)
	}
}

// TestLoad_ADocumentNoDeclarationNames_IsLeftUnattributed verifies the
// tripwire the shared inventory made possible.
//
// The index resolves a document through the object that declares it, so a
// document assembled in a package-level initializer belongs to no object and
// sits in no function body: nothing in the reachability walk can ever reach
// it, and a mutation written that way would leave a reader reporting a clean
// run. It is recorded as unattributed instead, which is what a reader reports.
func TestLoad_ADocumentNoDeclarationNames_IsLeftUnattributed(t *testing.T) {
	prog := loadFixture(t, map[string]string{"unplaced": actionfixture.Unplaced})

	unattributed := prog.Unattributed()
	if len(unattributed) != 1 {
		t.Fatalf("Unattributed() = %d document(s), want 1: %+v", len(unattributed), unattributed)
	}
	document := unattributed[0]
	if got := document.Label(); got != "an inline document" {
		t.Errorf("the unattributed document is labeled %q, want an inline one", got)
	}
	if !strings.Contains(document.Text, "thing { errors }") {
		t.Errorf("the unattributed document reads %q, want the assembled mutation", document.Text)
	}
	if !strings.HasSuffix(filepath.ToSlash(document.Position.Filename), "/unplaced/unplaced.go") {
		t.Errorf("the unattributed document is positioned at %q, want the fixture file", document.Position.Filename)
	}
}

// TestLoad_AnInlineDocumentInAFunctionBody_IsAttributed verifies the other
// half of that rule, which is what keeps the tripwire from firing on a shape
// the walk does place. An inline document written in a body is recorded by
// the body walk at the position of its literal, so the inventory entry at
// that position is placed and reported by nobody.
func TestLoad_AnInlineDocumentInAFunctionBody_IsAttributed(t *testing.T) {
	prog := loadFixture(t, vulnSources())

	if unattributed := prog.Unattributed(); len(unattributed) != 0 {
		t.Errorf("Unattributed() = %+v, want nothing: the fixture's inline document is in a body", unattributed)
	}
}

// TestLoad_AStandaloneTreeItCannotRead_Fails verifies the load stops when the
// .graphql half of the inventory cannot be read. Continuing would index the
// documents in Go source and silently none of the others, which is the
// silence reading them was added to remove.
func TestLoad_AStandaloneTreeItCannotRead_Fails(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "internal"), []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}

	_, err := Load(dir, []string{"./internal/..."}, nil)

	if err == nil {
		t.Fatal("Load() error = nil, want the unreadable tree")
	}
	if !strings.Contains(err.Error(), "open ") {
		t.Errorf("Load() error = %q, want it to name the tree it could not open", err)
	}
}

// standaloneModule writes a throwaway module holding one Go package and one
// standalone GraphQL document beside it, and returns its root.
//
// A real directory is needed rather than a loader overlay: the .graphql half
// of the inventory is read off disk, which no overlay reaches. The module is
// minimal and imports nothing, so type-checking it costs no network and no
// dependency tree.
func standaloneModule(t *testing.T, document string) string {
	t.Helper()
	dir := t.TempDir()
	pkg := filepath.Join(dir, "internal", "probe")
	if err := os.MkdirAll(pkg, 0o750); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
	files := map[string]string{
		filepath.Join(dir, "go.mod"):        "module standalone.example\n\ngo 1.24\n",
		filepath.Join(pkg, "probe.go"):      "package probe\n",
		filepath.Join(pkg, "probe.graphql"): document,
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("prepare the fixture: %v", err)
		}
	}
	return dir
}

// TestLoad_ADocumentInAFileOfItsOwn_IsReadAndLeftUnattributed verifies the
// .graphql half of the tripwire end to end.
//
// This is the shape the shared inventory was adopted for: a document that
// folds to nothing for the type checker, is bound to no object, and sits in no
// function body. Read but not indexed, it has to reach a reader as an
// unattributed document; dropped from the inventory instead, a mutation in a
// file of its own would be judged by nothing.
func TestLoad_ADocumentInAFileOfItsOwn_IsReadAndLeftUnattributed(t *testing.T) {
	dir := standaloneModule(t, "mutation($id: ID!) {\n  thing(input: {id: $id}) { errors }\n}\n")

	prog, err := Load(dir, []string{"./internal/..."}, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	unattributed := prog.Unattributed()
	if len(unattributed) != 1 {
		t.Fatalf("Unattributed() = %d document(s), want the .graphql file: %+v", len(unattributed), unattributed)
	}
	document := unattributed[0]
	if document.Name != "probe.graphql" {
		t.Errorf("the unattributed document is named %q, want probe.graphql", document.Name)
	}
	if document.Object != nil {
		t.Errorf("the unattributed document carries object %v, want none: no declaration names it", document.Object)
	}
	if !strings.HasSuffix(filepath.ToSlash(document.Position.Filename), "/internal/probe/probe.graphql") {
		t.Errorf("the unattributed document is positioned at %q, want the fixture file", document.Position.Filename)
	}
}

// TestLoad_TheShapesTheTwoRulesDisagreedAbout_AreIndexed holds the convergence
// the two rules were reduced to, for the half that lives here: a mutation
// written under a header line, and a one-field selection set written without a
// space, are both indexed as documents, and a document whose selection set
// opens on the line below its keyword is placed by the body that writes it.
// What each asks for is the reader's question.
func TestLoad_TheShapesTheTwoRulesDisagreedAbout_AreIndexed(t *testing.T) {
	prog := loadFixture(t, map[string]string{"edges": actionfixture.Edge})

	texts := documentTexts(prog, "edges")
	for _, name := range []string{"headedMutation", "spacelessRead"} {
		t.Run(name, func(t *testing.T) {
			if _, ok := texts[name]; !ok {
				t.Errorf("%s was not indexed as a document", name)
			}
		})
	}
	if unattributed := prog.Unattributed(); len(unattributed) != 0 {
		t.Errorf("Unattributed() = %+v, want nothing: the inline document is in a body", unattributed)
	}
}

// TestConstantString_NonString_ReturnsFalse verifies the constant unwrapper
// refuses values that are not strings, which is what keeps numeric and boolean
// constants out of the document index.
func TestConstantString_NonString_ReturnsFalse(t *testing.T) {
	if _, ok := constantString(nil); ok {
		t.Error("a nil constant value must not unwrap to a string")
	}
}

// TestIndexFunctions_DeclarationBoundToNoFunction_IsSkipped verifies the
// function index only records declarations the type checker gave an object
// for. A declaration named with the blank identifier is bound to nothing, and
// indexing it under a nil object would put every such declaration in the same
// bucket of the call graph.
func TestIndexFunctions_DeclarationBoundToNoFunction_IsSkipped(t *testing.T) {
	prog := &Program{funcs: map[*types.Func]*Function{}}
	pkg := &packages.Package{
		Name:      "synth",
		Syntax:    []*ast.File{{Decls: []ast.Decl{&ast.FuncDecl{Name: ast.NewIdent("_"), Body: &ast.BlockStmt{}}}}},
		TypesInfo: synthInfo(),
	}

	prog.indexFunctions(pkg)

	if len(prog.funcs) != 0 {
		t.Errorf("indexFunctions() recorded %d function(s) for a declaration bound to none", len(prog.funcs))
	}
}

// TestRecordLiteral_ConstantsThatCarryNoDocument_AreNotRecorded verifies the
// literal recorder judges the constant it was handed rather than the kind of
// node it arrived in. Only a document can be a document: a value that is not a
// string cannot be one, a literal the type checker recorded nothing for cannot
// be one, and a string the inventory's rule refuses is not one either.
func TestRecordLiteral_ConstantsThatCarryNoDocument_AreNotRecorded(t *testing.T) {
	number := &ast.BasicLit{Kind: token.INT, Value: "42"}
	untyped := &ast.BasicLit{Kind: token.STRING, Value: `"mutation { thing { errors } }"`}
	prose := &ast.BasicLit{Kind: token.STRING, Value: `"not a document"`}
	info := synthInfo()
	info.Types[number] = types.TypeAndValue{Value: constant.MakeInt64(42)}
	info.Types[prose] = types.TypeAndValue{Value: constant.MakeString("not a document")}

	cases := []struct {
		name string
		lit  *ast.BasicLit
	}{
		{name: "a constant that is not a string", lit: number},
		{name: "a literal the type checker has no value for", lit: untyped},
		{name: "a string that is no document", lit: prose},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fn := &Function{}

			(&Program{}).recordLiteral(fn, &packages.Package{TypesInfo: info}, testCase.lit)

			if len(fn.Documents) != 0 {
				t.Errorf("recordLiteral() recorded %d document(s): %+v", len(fn.Documents), fn.Documents)
			}
		})
	}
}

// TestRecordUse_IdentifiersThatNameNoDocument_AreNotRecorded verifies the body
// walk records a document for the identifiers that name one and for nothing
// else. A body names its parameters, its locals and its types too, and
// recording those as documents would fill every function's list with entries
// a reader then has to judge.
func TestRecordUse_IdentifiersThatNameNoDocument_AreNotRecorded(t *testing.T) {
	prog := loadFixture(t, vulnSources())

	cases := []struct {
		fn    string
		named []string
	}{
		{fn: "List", named: []string{"listQuery"}},
		{fn: "Dismiss", named: []string{"dismissMutation"}},
		{fn: "send", named: nil},
	}
	for _, testCase := range cases {
		t.Run(testCase.fn, func(t *testing.T) {
			var named []string
			for _, doc := range prog.funcs[lookupFunc(t, prog, "vuln", testCase.fn)].Documents {
				named = append(named, doc.Name)
			}
			if !slices.Equal(named, testCase.named) {
				t.Errorf("%s() names documents %v, want %v", testCase.fn, named, testCase.named)
			}
		})
	}
}

// TestIndexFunctions_DeclarationWithNoBody_IsSkipped verifies a function
// declared without a body is not indexed. Go allows one for a function
// implemented elsewhere, and walking a nil body would end the run on a panic
// rather than on a finding.
func TestIndexFunctions_DeclarationWithNoBody_IsSkipped(t *testing.T) {
	prog := &Program{funcs: map[*types.Func]*Function{}}
	pkg := &packages.Package{
		Name:      "synth",
		Syntax:    []*ast.File{{Decls: []ast.Decl{&ast.FuncDecl{Name: ast.NewIdent("Assembly")}}}},
		TypesInfo: synthInfo(),
	}

	prog.indexFunctions(pkg)

	if len(prog.funcs) != 0 {
		t.Errorf("indexFunctions() recorded %d function(s) for a declaration with no body", len(prog.funcs))
	}
}

// TestIndexBody_LiteralThatIsNotAString_IsNotOfferedAsADocument verifies the
// body walk decides on the literal's kind before it reads a value. A numeric
// literal carries a constant value like a string one does, so handing it to the
// recorder would put the question to the inventory's rule instead of to the
// node's own kind.
func TestIndexBody_LiteralThatIsNotAString_IsNotOfferedAsADocument(t *testing.T) {
	number := &ast.BasicLit{Kind: token.INT, Value: "42"}
	info := synthInfo()
	info.Types[number] = types.TypeAndValue{Value: constant.MakeString("mutation { thing { errors } }")}
	body := &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: number}}}

	fn := (&Program{}).indexBody(&packages.Package{TypesInfo: info}, body)

	if len(fn.Documents) != 0 {
		t.Errorf("indexBody() recorded %d document(s) from a literal that is not a string", len(fn.Documents))
	}
}

// reachedFrom returns the names of the indexed functions a handler of the
// requests fixture reaches, and the service methods they name.
func reachedFrom(t *testing.T, prog *Program, handler string) (names, methods []string) {
	t.Helper()
	for fn := range prog.Reachable([]*types.Func{lookupFunc(t, prog, "requests", handler)}) {
		body, ok := prog.Function(fn)
		if !ok {
			names = append(names, fn.FullName())
			continue
		}
		names = append(names, fn.Name())
		methods = append(methods, body.SDKMethods...)
	}
	sort.Strings(names)
	sort.Strings(methods)
	return names, methods
}

// TestLoad_PackageLevelVariables_AreReachedByTheBodiesThatNameThem verifies the
// three shapes a handler reaches a request through a package-level variable
// in: a seam holding a function literal, a seam holding a method expression,
// and a dispatch table of literals, which is reached whole since the caller's
// input picks the entry.
func TestLoad_PackageLevelVariables_AreReachedByTheBodiesThatNameThem(t *testing.T) {
	prog := loadFixture(t, requestSources())

	cases := []struct {
		handler     string
		wantReached string
		wantMethods []string
	}{
		{handler: "Raw", wantReached: "(*gitlab.com/gitlab-org/api/client-go/v3.Client).NewRequest"},
		{handler: "Expression", wantReached: "(*gitlab.com/gitlab-org/api/client-go/v3.Client).NewRequest"},
		{handler: "Table", wantReached: "getters", wantMethods: []string{"Projects.DeleteProject", "Projects.GetProject"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.handler, func(t *testing.T) {
			names, methods := reachedFrom(t, prog, testCase.handler)
			if !slices.Contains(names, testCase.wantReached) {
				t.Errorf("%s reaches %v, want %s among them", testCase.handler, names, testCase.wantReached)
			}
			if !slices.Equal(methods, testCase.wantMethods) {
				t.Errorf("%s reaches service methods %v, want %v", testCase.handler, methods, testCase.wantMethods)
			}
		})
	}
}

// TestIndexValueSpec_EveryNameIsIndexedAgainstItsValue verifies the
// initializer index: each name of a spec whose one value is a call is indexed
// against that call, and a name with no value is not indexed at all.
func TestIndexValueSpec_EveryNameIsIndexedAgainstItsValue(t *testing.T) {
	prog := loadFixture(t, requestSources())
	pair := lookupFunc(t, prog, "requests", "pair")

	for _, name := range []string{"first", "second"} {
		t.Run(name, func(t *testing.T) {
			if stand := lookupInitializer(t, prog, "requests", name); !prog.funcs[stand].calls[pair] {
				t.Errorf("the initializer of %s does not call pair()", name)
			}
		})
	}
	for variable := range prog.initializers {
		if variable.Name() == "unset" {
			t.Error("a variable with no initializer was indexed as one")
		}
	}
}

// TestIndexInitializers_DeclarationsThatInitializeNoVariable_AreSkipped
// verifies the initializer index reads only a var declaration's value specs,
// and only names the type checker defined a variable for: a declaration of
// another kind, a spec of another kind and a name with no definition add no
// stand-in, where indexing one would hand a body a call to nothing.
func TestIndexInitializers_DeclarationsThatInitializeNoVariable_AreSkipped(t *testing.T) {
	info := synthInfo()
	value := &ast.BasicLit{Kind: token.STRING, Value: `"value"`}
	defined := ast.NewIdent("defined")
	variable := types.NewVar(token.NoPos, nil, "defined", types.Typ[types.String])
	info.Defs[defined] = variable
	pkg := &packages.Package{TypesInfo: info, Syntax: []*ast.File{{Decls: []ast.Decl{
		&ast.GenDecl{Tok: token.CONST, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent("constant")}, Values: []ast.Expr{value}}}},
		&ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{
			&ast.TypeSpec{Name: ast.NewIdent("Odd")},
			&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent("undefined"), defined}, Values: []ast.Expr{value, value}},
		}},
	}}}}
	prog := &Program{funcs: map[*types.Func]*Function{}, initializers: map[*types.Var]*types.Func{}}

	prog.indexInitializers(pkg)

	if len(prog.initializers) != 1 || prog.initializers[variable] == nil {
		t.Errorf("indexInitializers() indexed %d initializer(s), want only the defined variable's", len(prog.initializers))
	}
}

// TestLink_AVariableOfAnUnloadedPackage_AddsNoCall verifies a variable the
// load holds no initializer for, errors.ErrUnsupported here, contributes
// nothing, as a function outside the load does, while a loaded one becomes a
// call of its stand-in.
func TestLink_AVariableOfAnUnloadedPackage_AddsNoCall(t *testing.T) {
	prog := loadFixture(t, requestSources())
	body := prog.funcs[lookupFunc(t, prog, "requests", "Pair")]

	second := lookupInitializer(t, prog, "requests", "second")
	if !body.calls[second] {
		t.Error("Pair names second, but its initializer is not among Pair's calls")
	}
	for callee := range body.calls {
		if callee.Name() == "ErrUnsupported" {
			t.Errorf("Pair calls %s, a variable of a package outside the load", callee.FullName())
		}
	}
	if body.variables != nil {
		t.Errorf("Pair still holds %d variable(s) once linked", len(body.variables))
	}
}

// TestRecordUse_ServiceMethods_AreRecordedCalledOrHandedOn verifies the
// client-go service methods a body names are recorded the way sdkroutes keys
// them, whether called or taken as a method value.
func TestRecordUse_ServiceMethods_AreRecordedCalledOrHandedOn(t *testing.T) {
	prog := loadFixture(t, requestSources())

	got := prog.funcs[lookupFunc(t, prog, "requests", "Direct")].SDKMethods
	if want := []string{"Projects.GetProject", "Projects.ListProjects"}; !slices.Equal(got, want) {
		t.Errorf("Direct names service methods %v, want %v", got, want)
	}
}

// serviceType builds a named type in a package, for the receiver of a method
// the service-method rule is asked about.
func serviceType(path, name string) *types.Named {
	pkg := types.NewPackage(path, "pkg")
	return types.NewNamed(types.NewTypeName(token.NoPos, pkg, name, nil), types.NewStruct(nil, nil), nil)
}

// methodOn builds a method named Get on a receiver of the given type.
func methodOn(recv types.Type) *types.Func {
	return types.NewFunc(token.NoPos, nil, "Get", types.NewSignatureType(types.NewVar(token.NoPos, nil, "s", recv), nil, nil, nil, nil, false))
}

// TestServiceMethod_TheMethodsThatSendARequest verifies what counts as a
// client-go service method: a method of a client-go type named for a service,
// by value or through a pointer, and nothing else.
func TestServiceMethod_TheMethodsThatSendARequest(t *testing.T) {
	cases := []struct {
		name   string
		callee *types.Func
		want   string
	}{
		{name: "an interface a handler holds", callee: methodOn(serviceType(ClientGoPath, "IssuesServiceInterface")), want: "Issues.Get"},
		{name: "a struct through a pointer", callee: methodOn(types.NewPointer(serviceType(ClientGoPath, "IssuesService"))), want: "Issues.Get"},
		{name: "a function with no receiver", callee: types.NewFunc(token.NoPos, nil, "Get", types.NewSignatureType(nil, nil, nil, nil, nil, false))},
		{name: "a method of an unnamed type", callee: methodOn(types.NewStruct(nil, nil))},
		{name: "a service of another package", callee: methodOn(serviceType("example.com/other", "IssuesService"))},
		{name: "a client-go type no service is named for", callee: methodOn(serviceType(ClientGoPath, "Client"))},
		{name: "a type named for no service at all", callee: methodOn(serviceType(ClientGoPath, "Service"))},
		{name: "a type of no package", callee: methodOn(types.NewNamed(types.NewTypeName(token.NoPos, nil, "IssuesService", nil), types.NewStruct(nil, nil), nil))},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, ok := serviceMethod(testCase.callee)
			if got != testCase.want || ok != (testCase.want != "") {
				t.Errorf("serviceMethod() = %q, %t; want %q, %t", got, ok, testCase.want, testCase.want != "")
			}
		})
	}
}

// TestIsPackageLevel_OnlyAPackageScopeVariable verifies the rule a body's
// variables are linked by: a variable declared at package scope, and not a
// field, a parameter or a local.
func TestIsPackageLevel_OnlyAPackageScopeVariable(t *testing.T) {
	pkg := types.NewPackage("example.com/scope", "scope")
	local := types.NewScope(pkg.Scope(), token.NoPos, token.NoPos, "func")
	packageVar := types.NewVar(token.NoPos, pkg, "table", types.Typ[types.Int])
	localVar := types.NewVar(token.NoPos, pkg, "table", types.Typ[types.Int])
	pkg.Scope().Insert(packageVar)
	local.Insert(localVar)

	cases := []struct {
		name     string
		variable *types.Var
		want     bool
	}{
		{name: "a package-level variable", variable: packageVar, want: true},
		{name: "a local", variable: localVar},
		{name: "a field", variable: types.NewField(token.NoPos, pkg, "ID", types.Typ[types.String], false)},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := isPackageLevel(testCase.variable); got != testCase.want {
				t.Errorf("isPackageLevel() = %t, want %t", got, testCase.want)
			}
		})
	}
}

// errFixture is returned by the failing catalog builder in the catalog tests.
var errFixture = errors.New("fixture catalog failure")
