package main

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"sort"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// shapesFixture declares one action per spelling an ActionSpec is written in
// across this repository, each routed to its own handler, so the resolver is
// held to following the forwarding rather than to recognizing one shape.
//
// The closure action exists because a route may be built from a function
// literal, which has no declared function to use as a call-graph root; its body
// reaches the mutation, so a resolver that dropped literals would report
// nothing for it.
const shapesFixture = `package shapes

import (
	"context"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_readonly_graphql/fixture/other"
)

// constantName is an action name written as a constant rather than a literal.
const constantName = "constant"

const readQuery = @@
query {
  currentUser { id }
}
@@

const writeMutation = @@
mutation($id: ID!) {
  thingUpdate(input: {id: $id}) { errors }
}
@@

// Input is the shared input for every fixture handler.
type Input struct {
	ID string ` + "`json:\"id\"`" + `
}

// Output is the shared output for every fixture handler.
type Output struct {
	OK bool ` + "`json:\"ok\"`" + `
}

func Direct(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return read(ctx, client)
}

func ViaHelper(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return read(ctx, client)
}

func Decorated(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return read(ctx, client)
}

func Chained(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return read(ctx, client)
}

func Literal(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return read(ctx, client)
}

func Variable(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return read(ctx, client)
}

func Constant(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return read(ctx, client)
}

func Appended(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return read(ctx, client)
}

// Quiet touches no GraphQL at all, which is the ordinary case: most actions
// are REST and this audit has nothing to say about them.
func Quiet(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_ = ctx
	_ = client
	return Output{OK: input.ID != ""}, nil
}

// closureBody is what the function-literal route runs, and it writes.
func closureBody(ctx context.Context) error {
	_ = ctx
	_ = writeMutation
	return nil
}

// closureAudit is the second function the literal route names, so the roots a
// literal stands in for are more than one and their order has to be settled.
func closureAudit(ctx context.Context) error {
	_ = ctx
	return nil
}

// ViaExecutor sends through the shared toolutil executor rather than through
// the client-go service method. It is the shape the note domains are written
// in, and the reason the transport check has a package half: the executor's
// receiver is not a GraphQL type, so where it is declared is what says it puts
// a document on the wire.
func ViaExecutor(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_, err := toolutil.ExecGraphQLNoteMutation[Output](ctx, client.GL().GraphQL, toolutil.GraphQLNoteMutation{
		Op:         "fixtureNoteUpdate",
		PayloadKey: "updateNote",
		Query:      writeMutation,
		Variables:  map[string]any{"id": input.ID},
	})
	return Output{OK: err == nil}, err
}

func read(ctx context.Context, client *gitlabclient.Client) (Output, error) {
	var response struct {
		Data map[string]any ` + "`json:\"data\"`" + `
	}
	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{Query: readQuery}, &response, gl.WithContext(ctx))
	return Output{OK: err == nil}, err
}

// GenericHandler is a handler whose route names an explicit instantiation.
func GenericHandler[T any](ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return read(ctx, client)
}

// VoidHandler routes through the void constructor, whose output type is
// synthesized rather than declared.
func VoidHandler(ctx context.Context, client *gitlabclient.Client, input Input) error {
	_, err := read(ctx, client)
	return err
}

// routeLiteralHandler is wired through an ActionRoute struct literal.
func routeLiteralHandler(ctx context.Context, params map[string]any) (any, error) {
	_ = ctx
	_ = params
	return nil, nil
}

// namedByFunc supplies an action name from a function rather than a literal.
func namedByFunc() string {
	return "named_by_func"
}

// ActionSpecs declares one action per construction shape.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	specs := []toolutil.ActionSpec{
		toolutil.NewReadActionSpec("direct", toolutil.RouteAction(client, Direct), toolutil.ActionSpecOptions{}),
		helperSpec("helper", toolutil.RouteAction(client, ViaHelper)),
		toolutil.NewReadActionSpec("decorated", toolutil.RouteAction(client, Decorated).WithTags("fixture"), toolutil.ActionSpecOptions{}),
		helperSpec("chained", toolutil.RouteAction(client, Chained)).WithEmbeddedResource("gitlab://project/{id}"),
		toolutil.ActionSpec{Name: "literal", Route: toolutil.RouteAction(client, Literal)},
		variableSpec(client),
		toolutil.NewReadActionSpec(constantName, routeFor(client), toolutil.ActionSpecOptions{}),
		toolutil.NewReadActionSpec("closure", toolutil.Route(func(ctx context.Context, params map[string]any) (any, error) {
			_ = params
			if err := closureAudit(ctx); err != nil {
				return nil, err
			}
			return nil, closureBody(ctx)
		}), toolutil.ActionSpecOptions{}),
		toolutil.NewReadActionSpec("quiet", toolutil.RouteAction(client, Quiet), toolutil.ActionSpecOptions{}),
		toolutil.NewReadActionSpec("generic", toolutil.RouteAction[Input, Output](client, Direct), toolutil.ActionSpecOptions{}),
		toolutil.NewReadActionSpec("generic_void", toolutil.RouteVoidAction[Input](client, VoidHandler), toolutil.ActionSpecOptions{}),
		toolutil.NewReadActionSpec("instantiated", toolutil.RouteAction(client, GenericHandler[string]), toolutil.ActionSpecOptions{}),
		toolutil.ActionSpec{Name: "route_literal", Route: toolutil.ActionRoute{Handler: routeLiteralHandler}},
		handlerSpec("passed_handler", client, ViaHelper),
		toolutil.NewReadActionSpec(strings.TrimSpace("  trimmed  "), toolutil.RouteAction(client, Direct), toolutil.ActionSpecOptions{}),
		toolutil.NewReadActionSpec(namedByFunc(), toolutil.RouteAction(client, Direct), toolutil.ActionSpecOptions{}),
		toolutil.NewReadActionSpec("shared_route", other.SharedRoute, toolutil.ActionSpecOptions{}),
		toolutil.NewReadActionSpec("cross_package", toolutil.RouteAction(client, other.Handle), toolutil.ActionSpecOptions{}),
	}
	return append(specs, toolutil.NewReadActionSpec("appended", toolutil.RouteAction(client, Appended), toolutil.ActionSpecOptions{}))
}

func helperSpec(name string, route toolutil.ActionRoute) toolutil.ActionSpec {
	options := toolutil.ActionSpecOptions{Usage: "fixture"}
	return toolutil.NewReadActionSpec(name, route, options)
}

// handlerSpec takes the handler itself rather than a built route, so the
// resolver has to follow a function value through a parameter.
func handlerSpec(name string, client *gitlabclient.Client, handler func(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error)) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, toolutil.RouteAction(client, handler), toolutil.ActionSpecOptions{})
}

func variableSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	var route = toolutil.RouteAction(client, Variable)
	spec := toolutil.NewReadActionSpec("variable", route, toolutil.ActionSpecOptions{})
	return spec
}

func routeFor(client *gitlabclient.Client) toolutil.ActionRoute {
	return toolutil.RouteAction(client, Constant)
}
`

// otherFixture holds the pieces a spec can reach across a package boundary: a
// handler named through a selector, and a route held in a package-level
// variable, which the resolver deliberately does not follow.
const otherFixture = `package other

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Input is the fixture handler input.
type Input struct {
	ID string ` + "`json:\"id\"`" + `
}

// Output is the fixture handler output.
type Output struct {
	OK bool ` + "`json:\"ok\"`" + `
}

// SharedRoute is a route built once at package level.
var SharedRoute = toolutil.Route(shared)

func shared(ctx context.Context, params map[string]any) (any, error) {
	_ = ctx
	_ = params
	return nil, nil
}

// Handle is a handler another package routes to by selector.
func Handle(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_ = ctx
	_ = client
	return Output{OK: input.ID != ""}, nil
}
`

// mainSources is the fixture set every resolution and detection test loads.
func mainSources() map[string]string {
	return map[string]string{
		"vuln":   vulnFixture,
		"shapes": shapesFixture,
		"vars":   varFixture,
		"other":  otherFixture,
	}
}

// handlerNames returns the handlers resolved for one action name in one
// package, sorted, with a function literal reported as "closure".
func handlerNames(sites map[string][]site, pkgName, actionName string) []string {
	var names []string
	for _, resolved := range sites[actionName] {
		if resolved.pkgName != pkgName {
			continue
		}
		for _, handler := range resolved.handlers {
			if handler.fn == nil {
				names = append(names, "closure")
				continue
			}
			names = append(names, handler.fn.Name())
		}
	}
	sort.Strings(names)
	return names
}

// TestResolver_CollectSites_EveryConstructionShape verifies the resolver finds
// the action name and the handler for every spelling an ActionSpec is written
// in: the toolutil constructor called directly, a package-local helper that
// forwards to it, a decorated route, a struct literal, a local variable, a
// name written as a constant, a route built by a helper, an appended element,
// and a route whose handler is a function literal.
func TestResolver_CollectSites_EveryConstructionShape(t *testing.T) {
	prog := loadFixture(t, mainSources())
	sites := (&resolver{prog: prog}).collectSites()

	cases := []struct {
		action  string
		handler string
	}{
		{action: "direct", handler: "Direct"},
		{action: "helper", handler: "ViaHelper"},
		{action: "decorated", handler: "Decorated"},
		{action: "chained", handler: "Chained"},
		{action: "literal", handler: "Literal"},
		{action: "variable", handler: "Variable"},
		{action: "constant", handler: "Constant"},
		{action: "appended", handler: "Appended"},
		{action: "closure", handler: "closure"},
		{action: "generic", handler: "Direct"},
		{action: "generic_void", handler: "VoidHandler"},
		{action: "instantiated", handler: "GenericHandler"},
		{action: "route_literal", handler: "routeLiteralHandler"},
		{action: "passed_handler", handler: "ViaHelper"},
		{action: "trimmed", handler: "Direct"},
		{action: "named_by_func", handler: "Direct"},
		{action: "cross_package", handler: "Handle"},
	}
	for _, testCase := range cases {
		t.Run(testCase.action, func(t *testing.T) {
			got := handlerNames(sites, "shapes", testCase.action)
			if len(got) != 1 || got[0] != testCase.handler {
				t.Errorf("action %q resolved to %v, want [%s]", testCase.action, got, testCase.handler)
			}
		})
	}
}

// TestResolver_CollectSites_ForwardingHelper verifies the domain shape most of
// this repository uses: a package-local helper that takes the action name and
// the route and forwards both to a toolutil constructor.
func TestResolver_CollectSites_ForwardingHelper(t *testing.T) {
	prog := loadFixture(t, mainSources())
	sites := (&resolver{prog: prog}).collectSites()

	for _, actionName := range []string{"list", "read_dismiss", "read_dismiss_indirect"} {
		t.Run(actionName, func(t *testing.T) {
			if got := handlerNames(sites, "vuln", actionName); len(got) != 1 {
				t.Errorf("action %q resolved to %v, want exactly one handler", actionName, got)
			}
		})
	}
	if got := handlerNames(sites, "vuln", "read_dismiss"); got[0] != "Dismiss" {
		t.Errorf("read_dismiss resolved to %v, want [Dismiss]", got)
	}
	if got := handlerNames(sites, "vuln", "read_dismiss_indirect"); got[0] != "DismissIndirect" {
		t.Errorf("read_dismiss_indirect resolved to %v, want [DismissIndirect]", got)
	}
}

// TestResolver_CollectSites_UnknownActionHasNoSite verifies an action name the
// source never declares resolves to nothing, which is what turns a catalog
// action the audit cannot place into a reported failure rather than a silent
// pass.
func TestResolver_CollectSites_UnknownActionHasNoSite(t *testing.T) {
	prog := loadFixture(t, mainSources())
	sites := (&resolver{prog: prog}).collectSites()
	if found, ok := sites["never_declared"]; ok {
		t.Errorf("an undeclared action resolved to %d site(s)", len(found))
	}
}

// TestResolveString_NonConstantExpression_ResolvesToEmpty verifies that a name
// the source does not determine resolves to the empty string, so a site whose
// name is computed at run time is not silently attributed to some other action.
func TestResolveString_NonConstantExpression_ResolvesToEmpty(t *testing.T) {
	prog := loadFixture(t, loadedSourceWithComputedName())
	sites := (&resolver{prog: prog}).collectSites()
	for name := range sites {
		if strings.Contains(name, "computed") {
			t.Errorf("a computed action name resolved to %q", name)
		}
	}
}

// loadedSourceWithComputedName is a fixture whose action name is not knowable
// from the source.
func loadedSourceWithComputedName() map[string]string {
	return map[string]string{"computed": computedFixture}
}

// computedFixture builds an action name at run time, which no static resolver
// can attribute to a catalog action.
const computedFixture = `package computed

import (
	"context"
	"os"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Input is the fixture handler input.
type Input struct {
	ID string ` + "`json:\"id\"`" + `
}

// Output is the fixture handler output.
type Output struct {
	OK bool ` + "`json:\"ok\"`" + `
}

func Handle(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_ = ctx
	_ = client
	_ = input
	return Output{}, nil
}

func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewReadActionSpec(os.Getenv("COMPUTED_NAME"), toolutil.RouteAction(client, Handle), toolutil.ActionSpecOptions{}),
	}
}
`

// synthResolver is a resolver over an empty program, for the refusals that
// answer before any source is read.
//
// The expressions the tests below hand it are built rather than parsed,
// because they stand for shapes no source in this repository writes: a spec
// held in a node kind the resolver does not follow, a call whose callee cannot
// be named, a spec reached past the depth bound. Each of them resolves to
// nothing on purpose, which is what turns such an action into a reported
// failure instead of a silent pass, and only a built expression can reach
// them. The depth ones resolve to something at depth 0 as well, since a shape
// that resolves to nothing everywhere would not tell the bound from its
// absence.
func synthResolver() *resolver {
	return &resolver{prog: &program{funcs: map[*types.Func]*function{}}}
}

// synthInfo is type information a test fills in itself.
func synthInfo() *types.Info {
	return &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Uses:  map[*ast.Ident]types.Object{},
		Defs:  map[*ast.Ident]types.Object{},
	}
}

// synthFrame is a frame over one hand-built package.
func synthFrame(info *types.Info, decl *ast.FuncDecl) frame {
	return frame{pkg: &packages.Package{Name: "synth", TypesInfo: info}, decl: decl}
}

// synthString is a string literal carrying its constant value, the way the
// type information carries the value of a Name field written in source.
func synthString(info *types.Info, value string) *ast.BasicLit {
	lit := &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(value)}
	info.Types[lit] = types.TypeAndValue{Type: types.Typ[types.String], Value: constant.MakeString(value)}
	return lit
}

// synthRoute is an ActionRoute literal whose Handler field is a function
// literal, which the resolver names without consulting any type information.
func synthRoute() *ast.CompositeLit {
	return &ast.CompositeLit{Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent("Handler"), Value: &ast.FuncLit{Type: &ast.FuncType{}, Body: &ast.BlockStmt{}}},
	}}
}

// synthSpec is an ActionSpec literal declaring name and routing to one
// function literal.
func synthSpec(info *types.Info, name string) *ast.CompositeLit {
	return &ast.CompositeLit{Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent("Name"), Value: synthString(info, name)},
		&ast.KeyValueExpr{Key: ast.NewIdent("Route"), Value: synthRoute()},
	}}
}

// TestResolver_DepthBeyondTheBound_ResolvesToNothing verifies the four
// recursive resolvers stop at maxResolveDepth. The bound exists so a helper
// that forwards to itself cannot hang the audit, and stopping has to mean
// "resolved nothing", which the caller reports, rather than "resolved to
// something partial", which it would trust.
//
// Each expression is resolved twice, at depth 0 and past the bound, because
// only the pair discriminates: an expression that resolves to nothing at any
// depth satisfies the second assertion whether the guard fires or not, so the
// first is what makes removing the guard fail this test.
func TestResolver_DepthBeyondTheBound_ResolvesToNothing(t *testing.T) {
	info := synthInfo()
	res := synthResolver()
	at := synthFrame(info, nil)
	beyond := maxResolveDepth + 1

	t.Run("spec", func(t *testing.T) {
		expr := synthSpec(info, "deep.action")
		if name, handlers := res.resolveSpec(expr, at, 0); name != "deep.action" || len(handlers) != 1 {
			t.Fatalf("resolveSpec() at depth 0 = %q, %d handler(s), want the fixture spec and its handler", name, len(handlers))
		}
		if name, handlers := res.resolveSpec(expr, at, beyond); name != "" || handlers != nil {
			t.Errorf("resolveSpec() past the bound = %q, %v, want the empty resolution", name, handlers)
		}
	})
	t.Run("route", func(t *testing.T) {
		expr := synthRoute()
		if handlers := res.resolveRoute(expr, at, 0); len(handlers) != 1 {
			t.Fatalf("resolveRoute() at depth 0 = %d handler(s), want the fixture handler", len(handlers))
		}
		if handlers := res.resolveRoute(expr, at, beyond); handlers != nil {
			t.Errorf("resolveRoute() past the bound = %v, want the empty resolution", handlers)
		}
	})
	t.Run("handler", func(t *testing.T) {
		expr := &ast.FuncLit{Type: &ast.FuncType{}, Body: &ast.BlockStmt{}}
		if handlers := res.resolveHandler(expr, at, 0); len(handlers) != 1 {
			t.Fatalf("resolveHandler() at depth 0 = %d handler(s), want the function literal", len(handlers))
		}
		if handlers := res.resolveHandler(expr, at, beyond); handlers != nil {
			t.Errorf("resolveHandler() past the bound = %v, want the empty resolution", handlers)
		}
	})
	t.Run("string", func(t *testing.T) {
		expr := synthString(info, "deep.action")
		if name := res.resolveString(expr, at, 0); name != "deep.action" {
			t.Fatalf("resolveString() at depth 0 = %q, want the fixture name", name)
		}
		if name := res.resolveString(expr, at, beyond); name != "" {
			t.Errorf("resolveString() past the bound = %q, want the empty resolution", name)
		}
	})
}

// TestResolveElements_ElementThatIsNoSpec_IsSkipped verifies the element loop
// judges each expression by its type. The loop is entered for the arguments of
// every append in the source, not only the ones appending to a spec slice, so
// an element of some other type has to be stepped over rather than resolved
// and filed under whatever name it happens to yield.
func TestResolveElements_ElementThatIsNoSpec_IsSkipped(t *testing.T) {
	res := synthResolver()
	info := synthInfo()
	element := &ast.Ident{Name: "name"}
	info.Types[element] = types.TypeAndValue{Type: types.Typ[types.String]}
	sites := map[string][]site{}

	res.resolveElements(&packages.Package{Name: "synth", TypesInfo: info}, nil, []ast.Expr{element}, sites)

	if len(sites) != 0 {
		t.Errorf("resolveElements() recorded %d site(s) from an element that is no spec", len(sites))
	}
}

// TestResolveSpec_NodeKindThatBuildsNoSpec_ResolvesToNothing verifies a spec
// expression that is neither an identifier, a literal nor a call resolves to
// nothing rather than to a half-read name.
func TestResolveSpec_NodeKindThatBuildsNoSpec_ResolvesToNothing(t *testing.T) {
	res := synthResolver()

	name, handlers := res.resolveSpec(&ast.BasicLit{Kind: token.STRING, Value: `"not a spec"`}, synthFrame(synthInfo(), nil), 0)

	if name != "" || handlers != nil {
		t.Errorf("resolveSpec() = %q, %v, want the empty resolution", name, handlers)
	}
}

// TestResolveSpecLiteral_ElementsThatNameNoField_AreSkipped verifies a spec
// literal element that is not a keyed field, and a keyed field whose key is
// not an identifier, are both stepped over. Neither can say which field it
// sets, so reading either would attribute a value to the wrong field.
func TestResolveSpecLiteral_ElementsThatNameNoField_AreSkipped(t *testing.T) {
	res := synthResolver()
	lit := &ast.CompositeLit{Elts: []ast.Expr{
		&ast.BasicLit{Kind: token.STRING, Value: `"positional"`},
		&ast.KeyValueExpr{Key: &ast.BasicLit{Kind: token.INT, Value: "0"}, Value: &ast.Ident{Name: "value"}},
	}}

	name, handlers := res.resolveSpecLiteral(lit, synthFrame(synthInfo(), nil), 0)

	if name != "" || handlers != nil {
		t.Errorf("resolveSpecLiteral() = %q, %v, want the empty resolution", name, handlers)
	}
}

// TestResolveRouteLiteral_ElementThatNamesNoField_IsSkipped verifies the same
// for a route literal, whose only field of interest is Handler.
func TestResolveRouteLiteral_ElementThatNamesNoField_IsSkipped(t *testing.T) {
	res := synthResolver()
	lit := &ast.CompositeLit{Elts: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"positional"`}}}

	if handlers := res.resolveRouteLiteral(lit, synthFrame(synthInfo(), nil), 0); handlers != nil {
		t.Errorf("resolveRouteLiteral() = %v, want the empty resolution", handlers)
	}
}

// TestResolveCall_CalleeThatCannotBeNamed_ResolvesToNothing verifies the three
// resolvers that enter a call give up when the call names no declared
// function, which is what an immediately invoked literal and a function value
// read out of a slice both look like.
func TestResolveCall_CalleeThatCannotBeNamed_ResolvesToNothing(t *testing.T) {
	res := synthResolver()
	at := synthFrame(synthInfo(), nil)
	call := &ast.CallExpr{Fun: &ast.FuncLit{Body: &ast.BlockStmt{}}}

	t.Run("spec", func(t *testing.T) {
		if name, handlers := res.resolveSpecCall(call, at, 0); name != "" || handlers != nil {
			t.Errorf("resolveSpecCall() = %q, %v, want the empty resolution", name, handlers)
		}
	})
	t.Run("route", func(t *testing.T) {
		if handlers := res.resolveRouteCall(call, at, 0); handlers != nil {
			t.Errorf("resolveRouteCall() = %v, want the empty resolution", handlers)
		}
	})
	t.Run("string", func(t *testing.T) {
		if name := res.resolveString(call, at, 0); name != "" {
			t.Errorf("resolveString() = %q, want the empty resolution", name)
		}
	})
}

// TestResolveRouteCall_CalleeWithNoBodyAndNoReceiver_ResolvesToNothing
// verifies the last exit of the route resolver: a call that names a function
// this audit did not index, that is not a toolutil constructor and that is not
// a method, so there is nothing left to follow.
func TestResolveRouteCall_CalleeWithNoBodyAndNoReceiver_ResolvesToNothing(t *testing.T) {
	res := synthResolver()
	info := synthInfo()
	callee := &ast.Ident{Name: "buildRoute"}
	info.Uses[callee] = types.NewFunc(token.NoPos, nil, "buildRoute", nil)

	handlers := res.resolveRouteCall(&ast.CallExpr{Fun: callee}, synthFrame(info, nil), 0)

	if handlers != nil {
		t.Errorf("resolveRouteCall() = %v, want the empty resolution", handlers)
	}
}

// TestResolveHandler_ExpressionThatNamesNoFunction_ResolvesToNothing verifies
// the two remaining exits of the handler resolver: an instantiation with
// several type arguments is unwrapped to what it instantiates, and an
// expression of function type that names no declared function resolves to
// nothing rather than to a handler the audit invented.
func TestResolveHandler_ExpressionThatNamesNoFunction_ResolvesToNothing(t *testing.T) {
	res := synthResolver()
	info := synthInfo()
	inner := &ast.BasicLit{Kind: token.STRING, Value: `"not a function"`}
	info.Types[inner] = types.TypeAndValue{Type: types.Typ[types.String]}
	unnamed := &ast.CallExpr{Fun: &ast.Ident{Name: "produce"}}
	info.Types[unnamed] = types.TypeAndValue{Type: types.NewSignatureType(nil, nil, nil, nil, nil, false)}
	at := synthFrame(info, nil)

	cases := []struct {
		name string
		expr ast.Expr
	}{
		{name: "an instantiation of something that is not a function", expr: &ast.IndexListExpr{X: inner}},
		{name: "a function value no declaration names", expr: unnamed},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if handlers := res.resolveHandler(testCase.expr, at, 0); handlers != nil {
				t.Errorf("resolveHandler() = %v, want the empty resolution", handlers)
			}
		})
	}
}

// TestReturnsOf_CalleeWhoseResultsDoNotCarryTheType_YieldsNothing verifies a
// body is only entered for the result that carries what is being resolved. A
// function returning no ActionRoute has no return expression worth following,
// and following one anyway would resolve a route to whatever it does return.
func TestReturnsOf_CalleeWhoseResultsDoNotCarryTheType_YieldsNothing(t *testing.T) {
	callee := types.NewFunc(token.NoPos, nil, "name", types.NewSignatureType(nil, nil, nil, nil,
		types.NewTuple(types.NewVar(token.NoPos, nil, "", types.Typ[types.String])), false))
	body := &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.Ident{Name: "value"}}}}}
	res := &resolver{prog: &program{funcs: map[*types.Func]*function{
		callee: {decl: &ast.FuncDecl{Body: body}},
	}}}

	found := res.returnsOf(callee, &ast.CallExpr{}, synthFrame(synthInfo(), nil), routeTypeName)

	if found != nil {
		t.Errorf("returnsOf() = %v, want nothing from a callee that returns no %s", found, routeTypeName)
	}
}

// TestReturnsOf_ReturnInsideAFunctionLiteral_IsNotTheCalleesReturn verifies a
// return written inside a closure is not read as the enclosing function's.
// The closure runs when it is called, not when the function returns, so
// treating its return as the callee's would resolve a spec to something the
// call never produces.
func TestReturnsOf_ReturnInsideAFunctionLiteral_IsNotTheCalleesReturn(t *testing.T) {
	callee := types.NewFunc(token.NoPos, nil, "name", types.NewSignatureType(nil, nil, nil, nil,
		types.NewTuple(types.NewVar(token.NoPos, nil, "", types.Typ[types.String])), false))
	own := &ast.Ident{Name: "own"}
	body := &ast.BlockStmt{List: []ast.Stmt{
		&ast.ExprStmt{X: &ast.FuncLit{Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.ReturnStmt{Results: []ast.Expr{&ast.Ident{Name: "inner"}}},
		}}}},
		&ast.ReturnStmt{Results: []ast.Expr{own}},
	}}
	res := &resolver{prog: &program{funcs: map[*types.Func]*function{
		callee: {decl: &ast.FuncDecl{Body: body}},
	}}}

	found := res.returnsOf(callee, &ast.CallExpr{}, synthFrame(synthInfo(), nil), "")

	if len(found) != 1 || found[0].expr != own {
		t.Errorf("returnsOf() yielded %d expression(s), want only the callee's own return", len(found))
	}
}

// TestFollow_IdentifierThatHoldsNothing_ResolvesToNothing verifies the two
// identifiers the resolver cannot follow: one that names something other than
// a variable, and a variable that is neither a bound parameter nor local to a
// body the resolver is standing in.
func TestFollow_IdentifierThatHoldsNothing_ResolvesToNothing(t *testing.T) {
	res := synthResolver()
	info := synthInfo()
	notAVariable := &ast.Ident{Name: "Handle"}
	info.Uses[notAVariable] = types.NewFunc(token.NoPos, nil, "Handle", nil)
	unbound := &ast.Ident{Name: "route"}
	info.Uses[unbound] = types.NewVar(token.NoPos, nil, "route", types.Typ[types.String])
	at := synthFrame(info, nil)

	cases := []struct {
		name  string
		ident *ast.Ident
	}{
		{name: "an identifier that names no variable", ident: notAVariable},
		{name: "a variable with no enclosing body", ident: unbound},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if found := res.follow(testCase.ident, at); found != nil {
				t.Errorf("follow() = %v, want the empty resolution", found)
			}
		})
	}
}

// TestAssignmentsTo_AssignmentsThatBindNoSingleValue_AreSkipped verifies the
// two assignment shapes that say nothing about what a variable holds: a
// multiple assignment from one call, whose values do not line up with its
// names, and an assignment to something that is not a plain identifier.
func TestAssignmentsTo_AssignmentsThatBindNoSingleValue_AreSkipped(t *testing.T) {
	variable := types.NewVar(token.NoPos, nil, "spec", types.Typ[types.String])
	info := synthInfo()
	multiple := &ast.Ident{Name: "spec"}
	info.Uses[multiple] = variable
	indexed := &ast.Ident{Name: "specs"}
	decl := &ast.FuncDecl{Body: &ast.BlockStmt{List: []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{multiple, &ast.Ident{Name: "err"}},
			Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.Ident{Name: "build"}}},
		},
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.IndexExpr{X: indexed, Index: &ast.BasicLit{Kind: token.INT, Value: "0"}}},
			Rhs: []ast.Expr{&ast.Ident{Name: "value"}},
		},
	}}}

	found := assignmentsTo(&packages.Package{TypesInfo: info}, decl, variable)

	if found != nil {
		t.Errorf("assignmentsTo() = %v, want nothing from assignments that bind no single value", found)
	}
}

// TestBindParams_ParametersThatCarryNothing_AreLeftUnbound verifies the two
// parameters a caller's argument is not bound to: those of a callee with no
// signature to read, and a variadic one, which no spec or route travels
// through and whose arguments do not correspond one to one.
func TestBindParams_ParametersThatCarryNothing_AreLeftUnbound(t *testing.T) {
	at := synthFrame(synthInfo(), nil)
	call := &ast.CallExpr{Args: []ast.Expr{&ast.Ident{Name: "first"}, &ast.Ident{Name: "second"}}}

	t.Run("a callee with no signature", func(t *testing.T) {
		if env := bindParams(types.NewFunc(token.NoPos, nil, "opaque", nil), call, at); env != nil {
			t.Errorf("bindParams() = %v, want no bindings", env)
		}
	})
	t.Run("a variadic parameter", func(t *testing.T) {
		variadic := types.NewFunc(token.NoPos, nil, "log", types.NewSignatureType(nil, nil, nil,
			types.NewTuple(
				types.NewVar(token.NoPos, nil, "name", types.Typ[types.String]),
				types.NewVar(token.NoPos, nil, "rest", types.NewSlice(types.Typ[types.String])),
			), nil, true))

		env := bindParams(variadic, call, at)

		if len(env) != 1 {
			t.Errorf("bindParams() bound %d parameter(s), want only the one before the variadic", len(env))
		}
	})
}

// TestResultIndex_SignaturesThatCarryNoAnswer_AreRefused verifies the three
// signatures no result index can be read from: one that is not a signature at
// all, one asked for its single result while returning several, and one whose
// results hold none of the toolutil type being resolved.
func TestResultIndex_SignaturesThatCarryNoAnswer_AreRefused(t *testing.T) {
	two := types.NewTuple(
		types.NewVar(token.NoPos, nil, "", types.Typ[types.String]),
		types.NewVar(token.NoPos, nil, "", types.Typ[types.String]),
	)
	cases := []struct {
		name     string
		callee   *types.Func
		typeName string
	}{
		{name: "no signature to read", callee: types.NewFunc(token.NoPos, nil, "opaque", nil)},
		{
			name:   "several results where one was wanted",
			callee: types.NewFunc(token.NoPos, nil, "pair", types.NewSignatureType(nil, nil, nil, nil, two, false)),
		},
		{
			name:     "no result of the wanted type",
			callee:   types.NewFunc(token.NoPos, nil, "pair", types.NewSignatureType(nil, nil, nil, nil, two, false)),
			typeName: routeTypeName,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if index, ok := resultIndex(testCase.callee, testCase.typeName); ok {
				t.Errorf("resultIndex() = %d, true, want it refused", index)
			}
		})
	}
}

// TestSpecConstructorArgs_SignaturesThatAreNotTheConstructorShape_AreRefused
// verifies the shape match is on all three of its parts. Matching the shape
// rather than a list of names is what covers a constructor added later, and a
// toolutil function that merely resembles one must not be read as the base
// case, or the two arguments taken as the answer would be someone else's.
func TestSpecConstructorArgs_SignaturesThatAreNotTheConstructorShape_AreRefused(t *testing.T) {
	toolutilPkg := types.NewPackage(toolutilPath, "toolutil")
	routeType := types.NewNamed(types.NewTypeName(token.NoPos, toolutilPkg, routeTypeName, nil), types.NewStruct(nil, nil), nil)
	str := types.Typ[types.String]
	call := &ast.CallExpr{Args: []ast.Expr{&ast.Ident{Name: "name"}, &ast.Ident{Name: "route"}}}
	constructor := func(first, second types.Type, results *types.Tuple) *types.Func {
		params := types.NewTuple(
			types.NewVar(token.NoPos, toolutilPkg, "first", first),
			types.NewVar(token.NoPos, toolutilPkg, "second", second),
		)
		return types.NewFunc(token.NoPos, toolutilPkg, "New", types.NewSignatureType(nil, nil, nil, params, results, false))
	}
	routeResult := types.NewTuple(types.NewVar(token.NoPos, toolutilPkg, "", routeType))

	cases := []struct {
		name   string
		callee *types.Func
	}{
		{name: "a first parameter that is not the action name", callee: constructor(types.Typ[types.Int], routeType, routeResult)},
		{name: "a second parameter that is not a route", callee: constructor(str, types.Typ[types.Int], routeResult)},
		{name: "a result that is not a spec", callee: constructor(str, routeType, routeResult)},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, _, ok := specConstructorArgs(testCase.callee, call); ok {
				t.Error("specConstructorArgs() matched a signature that is not the constructor shape")
			}
		})
	}
}

// TestIsToolutilType_TypeThatIsNotNamed_IsNotOne verifies the type check
// starts by asking for a named type: a builtin has no package to compare.
func TestIsToolutilType_TypeThatIsNotNamed_IsNotOne(t *testing.T) {
	if isToolutilType(types.Typ[types.String], routeTypeName) {
		t.Errorf("isToolutilType(string, %q) = true, want false", routeTypeName)
	}
}
