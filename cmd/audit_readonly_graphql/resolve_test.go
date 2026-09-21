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

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_readonly_graphql/fixture/other"
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

// closureCleanup and closureAudit are the other functions the literal route
// names, so the roots a literal stands in for are three, declared in an order
// the literal calls them in no part of, and their order has to be settled.
func closureCleanup(ctx context.Context) error {
	_ = ctx
	return nil
}

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
			if err := closureBody(ctx); err != nil {
				return nil, err
			}
			return nil, closureCleanup(ctx)
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
//
// It also declares an action called "quiet", which is the name the shapes
// fixture gives its REST-only action, routed here to a handler that sends a
// mutation. Two packages declaring one action name is what makes the owning
// package's part in resolution observable: the catalog says which of them
// declares the action it is asking about, and taking the other one would report
// a mutation against an action whose handler touches no GraphQL at all.
const otherFixture = `package other

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const quietMutation = @@
mutation($id: ID!) {
	otherQuietUpdate(input: {id: $id}) { errors }
}
@@

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

// Quiet shares its action name with the REST-only action of the shapes
// fixture, and writes.
func Quiet(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	var response struct {
		Data map[string]any ` + "`json:\"data\"`" + `
	}
	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     quietMutation,
		Variables: map[string]any{"id": input.ID},
	}, &response, gl.WithContext(ctx))
	return Output{OK: err == nil}, err
}

// ActionSpecs declares this package's own "quiet".
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewReadActionSpec("quiet", toolutil.RouteAction(client, Quiet), toolutil.ActionSpecOptions{}),
	}
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

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
	return frame{pkg: synthPkg(info), decl: decl}
}

// synthPkg is the hand-built package the frames above stand in.
func synthPkg(info *types.Info) *packages.Package {
	return &packages.Package{Name: "synth", TypesInfo: info}
}

// synthToolutilTypes builds the two toolutil types the resolver follows, in a
// package carrying toolutil's own import path, which is how it recognizes them.
func synthToolutilTypes() (pkg *types.Package, spec, route types.Type) {
	pkg = types.NewPackage(toolutilPath, "toolutil")
	spec = types.NewNamed(types.NewTypeName(token.NoPos, pkg, specTypeName, nil), types.NewStruct(nil, nil), nil)
	route = types.NewNamed(types.NewTypeName(token.NoPos, pkg, routeTypeName, nil), types.NewStruct(nil, nil), nil)

	return pkg, spec, route
}

// synthFunc builds a function object with the given parameter and result
// types, which is all the resolver reads of a callee it has not indexed.
func synthFunc(pkg *types.Package, name string, params, results []types.Type) *types.Func {
	tuple := func(list []types.Type) *types.Tuple {
		vars := make([]*types.Var, 0, len(list))
		for _, typ := range list {
			vars = append(vars, types.NewVar(token.NoPos, pkg, "", typ))
		}

		return types.NewTuple(vars...)
	}

	return types.NewFunc(token.NoPos, pkg, name, types.NewSignatureType(nil, nil, nil, tuple(params), tuple(results), false))
}

// synthCall builds a call to a function named by an identifier.
func synthCall(info *types.Info, callee *types.Func, args ...ast.Expr) *ast.CallExpr {
	fun := ast.NewIdent(callee.Name())
	info.Uses[fun] = callee

	return &ast.CallExpr{Fun: fun, Args: args}
}

// synthMethodCall builds a call to a method on the given receiver expression,
// which is the shape a decorating method such as WithTags is written in.
func synthMethodCall(info *types.Info, callee *types.Func, receiver ast.Expr) *ast.CallExpr {
	sel := ast.NewIdent(callee.Name())
	info.Uses[sel] = callee

	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: receiver, Sel: sel}, Args: nil}
}

// synthBound builds an identifier bound to one expression the way a parameter
// is bound to the argument a caller passed, and the frame that binding lives
// in.
func synthBound(info *types.Info, name string, typ types.Type, held ast.Expr) (*ast.Ident, frame) {
	variable := types.NewVar(token.NoPos, nil, name, typ)
	ident := ast.NewIdent(name)
	info.Uses[ident] = variable
	at := synthFrame(info, nil)
	at.env = map[*types.Var]binding{variable: {expr: held, frame: synthFrame(info, nil)}}

	return ident, at
}

// synthIndexed builds a resolver whose program holds one indexed function, so
// the resolvers that enter a callee's body have one to enter.
func synthIndexed(info *types.Info, callee *types.Func, body *ast.BlockStmt) *resolver {
	return &resolver{prog: &program{funcs: map[*types.Func]*function{
		callee: {pkg: synthPkg(info), decl: &ast.FuncDecl{Body: body}},
	}}}
}

// resolution is what one resolver call came back with, reduced to the two
// things every one of them can be asked: the action name, and how many
// handlers. The empty resolution is what the depth bound produces, and what
// the caller reports rather than trusts.
type resolution struct {
	name     string
	handlers int
}

func (r resolution) empty() bool {
	return r.name == "" && r.handlers == 0
}

// depthProbe is one recursive step of the resolver: a call that takes exactly
// one step past the expression it is given, so resolving it at a depth is the
// same as asking whether that step was counted.
type depthProbe struct {
	name    string
	resolve func(depth int) resolution
}

// recursiveSteps builds one probe per place a resolver calls another with a
// deeper depth. Each is built once and resolved twice, since what discriminates
// is the pair: an expression that resolves to nothing at every depth would
// satisfy the bound assertion whether the step counted or not.
func recursiveSteps() []depthProbe {
	toolutilPkg, specType, routeType := synthToolutilTypes()
	str := types.Typ[types.String]
	handlerType := types.NewSignatureType(nil, nil, nil, nil, nil, false)

	return []depthProbe{
		specIdentProbe(specType),
		specLiteralProbe(),
		specConstructorProbe(toolutilPkg, specType, routeType, str),
		specReturnProbe(specType),
		specReceiverProbe(toolutilPkg, specType),
		routeIdentProbe(routeType),
		routeLiteralProbe(),
		routeConstructorProbe(toolutilPkg, routeType, handlerType),
		routeReturnProbe(routeType),
		routeReceiverProbe(routeType),
		handlerIndexProbe(),
		handlerIndexListProbe(),
		handlerIdentProbe(handlerType),
		stringIdentProbe(str),
		stringPassThroughProbe(str),
		stringReturnProbe(str),
	}
}

// specIdentProbe resolves a spec held in a bound identifier.
func specIdentProbe(specType types.Type) depthProbe {
	info := synthInfo()
	ident, at := synthBound(info, "spec", specType, synthSpec(info, "deep.action"))

	return depthProbe{name: "a spec held in an identifier", resolve: func(depth int) resolution {
		name, handlers := synthResolver().resolveSpec(ident, at, depth)

		return resolution{name: name, handlers: len(handlers)}
	}}
}

// specLiteralProbe resolves the Name and Route fields of a spec literal, which
// are two steps taken from one expression.
func specLiteralProbe() depthProbe {
	info := synthInfo()
	lit := synthSpec(info, "deep.action")
	at := synthFrame(info, nil)

	return depthProbe{name: "the fields of a spec literal", resolve: func(depth int) resolution {
		name, handlers := synthResolver().resolveSpec(lit, at, depth)

		return resolution{name: name, handlers: len(handlers)}
	}}
}

// specConstructorProbe resolves the two arguments a toolutil constructor is
// given.
func specConstructorProbe(toolutilPkg *types.Package, specType, routeType, str types.Type) depthProbe {
	info := synthInfo()
	callee := synthFunc(toolutilPkg, "NewActionSpec", []types.Type{str, routeType}, []types.Type{specType})
	call := synthCall(info, callee, synthString(info, "deep.action"), synthRoute())
	at := synthFrame(info, nil)

	return depthProbe{name: "the arguments of a toolutil constructor", resolve: func(depth int) resolution {
		name, handlers := synthResolver().resolveSpecCall(call, at, depth)

		return resolution{name: name, handlers: len(handlers)}
	}}
}

// specReturnProbe resolves the spec a helper returns.
func specReturnProbe(specType types.Type) depthProbe {
	info := synthInfo()
	callee := synthFunc(types.NewPackage("example.com/domain", "domain"), "buildSpec", nil, []types.Type{specType})
	body := &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{synthSpec(info, "deep.action")}}}}
	res := synthIndexed(info, callee, body)
	call := synthCall(info, callee)
	at := synthFrame(info, nil)

	return depthProbe{name: "the spec a helper returns", resolve: func(depth int) resolution {
		name, handlers := res.resolveSpecCall(call, at, depth)

		return resolution{name: name, handlers: len(handlers)}
	}}
}

// specReceiverProbe resolves the spec a decorating method was called on.
func specReceiverProbe(toolutilPkg *types.Package, specType types.Type) depthProbe {
	info := synthInfo()
	callee := synthFunc(toolutilPkg, "WithTags", nil, []types.Type{specType})
	call := synthMethodCall(info, callee, synthSpec(info, "deep.action"))
	at := synthFrame(info, nil)

	return depthProbe{name: "the receiver of a decorating method", resolve: func(depth int) resolution {
		name, handlers := synthResolver().resolveSpecCall(call, at, depth)

		return resolution{name: name, handlers: len(handlers)}
	}}
}

// routeIdentProbe resolves a route held in a bound identifier.
func routeIdentProbe(routeType types.Type) depthProbe {
	info := synthInfo()
	ident, at := synthBound(info, "route", routeType, synthRoute())

	return depthProbe{name: "a route held in an identifier", resolve: func(depth int) resolution {
		return resolution{handlers: len(synthResolver().resolveRoute(ident, at, depth))}
	}}
}

// routeLiteralProbe resolves the Handler field of a route literal.
func routeLiteralProbe() depthProbe {
	info := synthInfo()
	lit := synthRoute()
	at := synthFrame(info, nil)

	return depthProbe{name: "the handler of a route literal", resolve: func(depth int) resolution {
		return resolution{handlers: len(synthResolver().resolveRoute(lit, at, depth))}
	}}
}

// routeConstructorProbe resolves the handler a toolutil route constructor is
// given.
func routeConstructorProbe(toolutilPkg *types.Package, routeType, handlerType types.Type) depthProbe {
	info := synthInfo()
	callee := synthFunc(toolutilPkg, "Route", []types.Type{handlerType}, []types.Type{routeType})
	call := synthCall(info, callee, &ast.FuncLit{Type: &ast.FuncType{}, Body: &ast.BlockStmt{}})
	at := synthFrame(info, nil)

	return depthProbe{name: "the handler a toolutil route constructor takes", resolve: func(depth int) resolution {
		return resolution{handlers: len(synthResolver().resolveRouteCall(call, at, depth))}
	}}
}

// routeReturnProbe resolves the route a helper returns.
func routeReturnProbe(routeType types.Type) depthProbe {
	info := synthInfo()
	callee := synthFunc(types.NewPackage("example.com/domain", "domain"), "routeFor", nil, []types.Type{routeType})
	body := &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{synthRoute()}}}}
	res := synthIndexed(info, callee, body)
	call := synthCall(info, callee)
	at := synthFrame(info, nil)

	return depthProbe{name: "the route a helper returns", resolve: func(depth int) resolution {
		return resolution{handlers: len(res.resolveRouteCall(call, at, depth))}
	}}
}

// routeReceiverProbe resolves the route a decorating method was called on.
func routeReceiverProbe(routeType types.Type) depthProbe {
	info := synthInfo()
	callee := synthFunc(types.NewPackage("example.com/domain", "domain"), "WithTags", nil, []types.Type{routeType})
	call := synthMethodCall(info, callee, synthRoute())
	at := synthFrame(info, nil)

	return depthProbe{name: "the receiver of a decorated route", resolve: func(depth int) resolution {
		return resolution{handlers: len(synthResolver().resolveRouteCall(call, at, depth))}
	}}
}

// handlerIndexProbe resolves the function an instantiation instantiates.
func handlerIndexProbe() depthProbe {
	info := synthInfo()
	expr := &ast.IndexExpr{X: &ast.FuncLit{Type: &ast.FuncType{}, Body: &ast.BlockStmt{}}}
	at := synthFrame(info, nil)

	return depthProbe{name: "the function an instantiation names", resolve: func(depth int) resolution {
		return resolution{handlers: len(synthResolver().resolveHandler(expr, at, depth))}
	}}
}

// handlerIndexListProbe is the same for an instantiation with several type
// arguments.
func handlerIndexListProbe() depthProbe {
	info := synthInfo()
	expr := &ast.IndexListExpr{X: &ast.FuncLit{Type: &ast.FuncType{}, Body: &ast.BlockStmt{}}}
	at := synthFrame(info, nil)

	return depthProbe{name: "the function a multi-argument instantiation names", resolve: func(depth int) resolution {
		return resolution{handlers: len(synthResolver().resolveHandler(expr, at, depth))}
	}}
}

// handlerIdentProbe resolves a handler held in a bound identifier.
func handlerIdentProbe(handlerType types.Type) depthProbe {
	info := synthInfo()
	ident, at := synthBound(info, "handler", handlerType, &ast.FuncLit{Type: &ast.FuncType{}, Body: &ast.BlockStmt{}})

	return depthProbe{name: "a handler held in an identifier", resolve: func(depth int) resolution {
		return resolution{handlers: len(synthResolver().resolveHandler(ident, at, depth))}
	}}
}

// stringIdentProbe resolves a name held in a bound identifier.
func stringIdentProbe(str types.Type) depthProbe {
	info := synthInfo()
	ident, at := synthBound(info, "name", str, synthString(info, "deep.action"))

	return depthProbe{name: "a name held in an identifier", resolve: func(depth int) resolution {
		return resolution{name: synthResolver().resolveString(ident, at, depth)}
	}}
}

// stringPassThroughProbe resolves the argument of the one call that returns its
// own string argument.
func stringPassThroughProbe(str types.Type) depthProbe {
	info := synthInfo()
	callee := synthFunc(types.NewPackage("strings", "strings"), "TrimSpace", []types.Type{str}, []types.Type{str})
	call := synthCall(info, callee, synthString(info, "  deep.action  "))
	at := synthFrame(info, nil)

	return depthProbe{name: "the argument of the string pass-through", resolve: func(depth int) resolution {
		return resolution{name: synthResolver().resolveString(call, at, depth)}
	}}
}

// stringReturnProbe resolves the name a helper returns.
func stringReturnProbe(str types.Type) depthProbe {
	info := synthInfo()
	callee := synthFunc(types.NewPackage("example.com/domain", "domain"), "nameFor", nil, []types.Type{str})
	body := &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{synthString(info, "deep.action")}}}}
	res := synthIndexed(info, callee, body)
	call := synthCall(info, callee)
	at := synthFrame(info, nil)

	return depthProbe{name: "the name a helper returns", resolve: func(depth int) resolution {
		return resolution{name: res.resolveString(call, at, depth)}
	}}
}

// TestResolver_EveryRecursiveStep_CountsAgainstTheBound verifies the bound is
// enforced at every place one resolver calls another, not only at the entry
// points.
//
// The bound exists so a helper that forwards to itself cannot hang the audit,
// and it only does that if each step is taken one deeper than the last: a step
// that passed its own depth along, or counted down, would let a cycle run
// forever while every test that resolves an ordinary three-deep chain stayed
// green. Each probe is one such step, resolved at depth zero, where it has to
// produce something, and at the bound, where the step it takes is past the
// bound and has to produce nothing.
func TestResolver_EveryRecursiveStep_CountsAgainstTheBound(t *testing.T) {
	for _, probe := range recursiveSteps() {
		t.Run(probe.name, func(t *testing.T) {
			if got := probe.resolve(0); got.empty() {
				t.Fatalf("resolving at depth 0 produced nothing, so the bound assertion below would prove nothing")
			}
			if got := probe.resolve(maxResolveDepth); !got.empty() {
				t.Errorf("resolving at the bound produced %+v, want nothing: the step it takes is past the bound", got)
			}
		})
	}
}

// TestResolver_TheBound_IsTheLastDepthThatResolves verifies the guard is
// written as "past the bound" rather than "at it". The two resolvers that can
// answer without taking another step are the only ones that can tell the
// difference, and the difference is one whole level of nesting: a bound that
// fired one step early would drop the deepest handler of every chain that
// reached it, and drop it silently, since an action that resolves to nothing is
// reported as unresolvable rather than as truncated.
func TestResolver_TheBound_IsTheLastDepthThatResolves(t *testing.T) {
	info := synthInfo()
	at := synthFrame(info, nil)
	res := synthResolver()
	literal := &ast.FuncLit{Type: &ast.FuncType{}, Body: &ast.BlockStmt{}}
	name := synthString(info, "deep.action")

	t.Run("handler", func(t *testing.T) {
		if handlers := res.resolveHandler(literal, at, maxResolveDepth); len(handlers) != 1 {
			t.Errorf("resolveHandler() at the bound = %d handler(s), want the function literal", len(handlers))
		}
		if handlers := res.resolveHandler(literal, at, maxResolveDepth+1); handlers != nil {
			t.Errorf("resolveHandler() past the bound = %v, want the empty resolution", handlers)
		}
	})
	t.Run("string", func(t *testing.T) {
		if got := res.resolveString(name, at, maxResolveDepth); got != "deep.action" {
			t.Errorf("resolveString() at the bound = %q, want the fixture name", got)
		}
		if got := res.resolveString(name, at, maxResolveDepth+1); got != "" {
			t.Errorf("resolveString() past the bound = %q, want the empty resolution", got)
		}
	})
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

// TestIsToolutilType_TypesFromSomewhereElse_AreNotOne verifies all three
// answers the type check gives before it compares a name: a builtin is not a
// named type at all, a named type may belong to no package, and a named type of
// the right name may belong to another one. The last is what the path is for:
// every domain package here is free to declare its own ActionSpec, and reading
// one as toolutil's would take a handler from whatever it happens to hold.
func TestIsToolutilType_TypesFromSomewhereElse_AreNotOne(t *testing.T) {
	elsewhere := types.NewPackage("example.com/domain", "domain")
	cases := []struct {
		name string
		typ  types.Type
	}{
		{name: "a builtin", typ: types.Typ[types.String]},
		{
			name: "a named type belonging to no package",
			typ:  types.NewNamed(types.NewTypeName(token.NoPos, nil, routeTypeName, nil), types.NewStruct(nil, nil), nil),
		},
		{
			name: "a named type of another package",
			typ:  types.NewNamed(types.NewTypeName(token.NoPos, elsewhere, routeTypeName, nil), types.NewStruct(nil, nil), nil),
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if isToolutilType(testCase.typ, routeTypeName) {
				t.Errorf("isToolutilType(%s, %q) = true, want false", testCase.typ, routeTypeName)
			}
		})
	}
}

// TestIsSpecType_ValuesThatAreNoSpec_AreRefused verifies the two answers the
// element loop depends on: an expression the type checker typed as something
// else is not a spec, and neither is one it could not type at all. The loop
// runs over the arguments of every append in the tree, so admitting either
// would resolve expressions that declare no action and file them under
// whatever name they happened to yield.
func TestIsSpecType_ValuesThatAreNoSpec_AreRefused(t *testing.T) {
	cases := []struct {
		name string
		typ  types.Type
	}{
		{name: "a type that is not a spec", typ: types.Typ[types.String]},
		{name: "no type at all", typ: nil},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if isSpecType(testCase.typ) {
				t.Error("isSpecType() = true, want false")
			}
		})
	}
}

// TestIsSpecSliceType_SlicesOfSomethingElse_AreRefused verifies the slice check
// asks what the slice holds. Every file here has string slices and option
// slices in it, and reading one as a slice of specs would walk its elements
// looking for action names.
func TestIsSpecSliceType_SlicesOfSomethingElse_AreRefused(t *testing.T) {
	cases := []struct {
		name string
		typ  types.Type
	}{
		{name: "a slice of something else", typ: types.NewSlice(types.Typ[types.String])},
		{name: "not a slice", typ: types.Typ[types.String]},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if isSpecSliceType(testCase.typ) {
				t.Error("isSpecSliceType() = true, want false")
			}
		})
	}
}

// TestMerge_ASecondResolution_KeepsTheFirstNameAndEveryHandler verifies what
// merging two partial resolutions of one spec means: the first name wins, and
// no handler is dropped. A variable assigned in two branches routes to both,
// and keeping only one of them would leave a mutation unclassified; taking the
// second name would attribute the spec to whichever branch was resolved last.
func TestMerge_ASecondResolution_KeepsTheFirstNameAndEveryHandler(t *testing.T) {
	first := []handlerRef{{}}
	second := []handlerRef{{}, {}}

	name, handlers := merge("first", first, "second", second)

	if name != "first" {
		t.Errorf("merge() name = %q, want the first resolution's", name)
	}
	if len(handlers) != 3 {
		t.Errorf("merge() kept %d handler(s), want all three", len(handlers))
	}
}

// TestResolveSpecLiteral_FieldThatIsNeitherNameNorRoute_IsIgnored verifies a
// spec literal is read field by field. An ActionSpec carries usage text, tags
// and options beside the two fields this audit reads, and taking a value from
// one of those would answer with something that is not an action name.
func TestResolveSpecLiteral_FieldThatIsNeitherNameNorRoute_IsIgnored(t *testing.T) {
	info := synthInfo()
	lit := &ast.CompositeLit{Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent("Usage"), Value: synthString(info, "not an action name")},
		&ast.KeyValueExpr{Key: ast.NewIdent("Name"), Value: synthString(info, "the.action")},
		&ast.KeyValueExpr{Key: ast.NewIdent("Route"), Value: synthRoute()},
	}}

	name, handlers := synthResolver().resolveSpecLiteral(lit, synthFrame(info, nil), 0)

	if name != "the.action" {
		t.Errorf("resolveSpecLiteral() name = %q, want the Name field", name)
	}
	if len(handlers) != 1 {
		t.Errorf("resolveSpecLiteral() resolved %d handler(s), want the one the Route field names", len(handlers))
	}
}

// TestResolveRouteLiteral_KeysThatAreNotTheHandlerField_AreSteppedOver
// verifies the route literal is read the same way, and that the key is asked
// what it is before it is asked what it says. A composite literal may carry a
// key that is not an identifier at all, and reading a name off one would end
// the run on a panic rather than on a finding.
func TestResolveRouteLiteral_KeysThatAreNotTheHandlerField_AreSteppedOver(t *testing.T) {
	info := synthInfo()
	lit := &ast.CompositeLit{Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: &ast.BasicLit{Kind: token.INT, Value: "0"}, Value: &ast.FuncLit{Type: &ast.FuncType{}, Body: &ast.BlockStmt{}}},
		&ast.KeyValueExpr{Key: ast.NewIdent("Tags"), Value: synthString(info, "fixture")},
	}}

	if handlers := synthResolver().resolveRouteLiteral(lit, synthFrame(info, nil), 0); handlers != nil {
		t.Errorf("resolveRouteLiteral() = %v, want nothing: neither element names the Handler field", handlers)
	}
}

// TestResolveHandler_SelectorThatNamesNoFunction_ResolvesToNothing verifies a
// handler reached through a selector is only taken when the selector names a
// declared function. A package-level variable of function type is written the
// same way and holds whatever was assigned to it, which this resolver does not
// follow across a package boundary.
func TestResolveHandler_SelectorThatNamesNoFunction_ResolvesToNothing(t *testing.T) {
	info := synthInfo()
	signature := types.NewSignatureType(nil, nil, nil, nil, nil, false)
	sel := ast.NewIdent("SharedRoute")
	info.Uses[sel] = types.NewVar(token.NoPos, nil, "SharedRoute", signature)
	selector := &ast.SelectorExpr{X: ast.NewIdent("other"), Sel: sel}
	info.Types[selector] = types.TypeAndValue{Type: signature}

	if handlers := synthResolver().resolveHandler(selector, synthFrame(info, nil), 0); handlers != nil {
		t.Errorf("resolveHandler() = %v, want nothing from a selector that names a variable", handlers)
	}
}

// TestResolveString_ValuesThatAreNoName_ResolveToEmpty verifies the two ways an
// expression says nothing about an action name: it carries a constant that is
// not a string, or it is a kind of node this resolver does not follow. Both
// have to come back empty, because a site whose name cannot be read is reported
// as unresolvable, and a name read off something else would be filed under an
// action nobody declared.
func TestResolveString_ValuesThatAreNoName_ResolveToEmpty(t *testing.T) {
	info := synthInfo()
	number := &ast.BasicLit{Kind: token.INT, Value: "42"}
	info.Types[number] = types.TypeAndValue{Value: constant.MakeInt64(42)}
	joined := &ast.BinaryExpr{X: ast.NewIdent("prefix"), Op: token.ADD, Y: ast.NewIdent("suffix")}

	cases := []struct {
		name string
		expr ast.Expr
	}{
		{name: "a constant that is not a string", expr: number},
		{name: "a node kind the resolver does not follow", expr: joined},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := synthResolver().resolveString(testCase.expr, synthFrame(info, nil), 0); got != "" {
				t.Errorf("resolveString() = %q, want the empty resolution", got)
			}
		})
	}
}

// TestResolveString_TheFirstExpressionThatNamesNothing_IsPassedOver verifies a
// name is looked for in every expression a variable can hold and in every
// expression a helper can return, rather than in the first one alone. A
// variable declared empty and assigned later, and a helper that returns early,
// both put an expression that names nothing in front of the one that does.
func TestResolveString_TheFirstExpressionThatNamesNothing_IsPassedOver(t *testing.T) {
	t.Run("a variable assigned twice", func(t *testing.T) {
		info := synthInfo()
		variable := types.NewVar(token.NoPos, nil, "name", types.Typ[types.String])
		declared, assigned, used := ast.NewIdent("name"), ast.NewIdent("name"), ast.NewIdent("name")
		info.Defs[declared] = variable
		info.Uses[assigned] = variable
		info.Uses[used] = variable
		decl := &ast.FuncDecl{Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{declared}, Rhs: []ast.Expr{ast.NewIdent("unresolvable")}},
			&ast.AssignStmt{Lhs: []ast.Expr{assigned}, Rhs: []ast.Expr{synthString(info, "the.action")}},
		}}}

		if got := synthResolver().resolveString(used, synthFrame(info, decl), 0); got != "the.action" {
			t.Errorf("resolveString() = %q, want the name the second assignment gives it", got)
		}
	})
	t.Run("a helper that returns twice", func(t *testing.T) {
		info := synthInfo()
		callee := synthFunc(types.NewPackage("example.com/domain", "domain"), "nameFor", nil, []types.Type{types.Typ[types.String]})
		res := synthIndexed(info, callee, &ast.BlockStmt{List: []ast.Stmt{
			&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent("unresolvable")}},
			&ast.ReturnStmt{Results: []ast.Expr{synthString(info, "the.action")}},
		}})

		if got := res.resolveString(synthCall(info, callee), synthFrame(info, nil), 0); got != "the.action" {
			t.Errorf("resolveString() = %q, want the name the second return gives it", got)
		}
	})
}

// TestResolveString_CallThatIsNotThePassThrough_IsEnteredRatherThanUnwrapped
// verifies the pass-through is recognized by which function it is and by how
// many arguments it was given, both. A call of one argument that is not
// strings.TrimSpace declares its name in what it returns, and reading the
// argument instead would name the action after whatever that helper was handed;
// a call of two arguments is not the one-argument function this unwraps
// whatever it is named, and there would be no saying which argument to read.
func TestResolveString_CallThatIsNotThePassThrough_IsEnteredRatherThanUnwrapped(t *testing.T) {
	info := synthInfo()
	str := types.Typ[types.String]
	domain := types.NewPackage("example.com/domain", "domain")
	callee := synthFunc(domain, "nameFor", []types.Type{str}, []types.Type{str})
	res := synthIndexed(info, callee, &ast.BlockStmt{List: []ast.Stmt{
		&ast.ReturnStmt{Results: []ast.Expr{synthString(info, "the.action")}},
	}})

	t.Run("a helper of one argument", func(t *testing.T) {
		call := synthCall(info, callee, synthString(info, "an argument"))

		if got := res.resolveString(call, synthFrame(info, nil), 0); got != "the.action" {
			t.Errorf("resolveString() = %q, want what the helper returns rather than what it was passed", got)
		}
	})
	t.Run("the pass-through given more than one argument", func(t *testing.T) {
		trim := synthFunc(types.NewPackage("strings", "strings"), "TrimSpace", []types.Type{str, str}, []types.Type{str})
		call := synthCall(info, trim, synthString(info, "first"), synthString(info, "second"))

		if got := res.resolveString(call, synthFrame(info, nil), 0); got != "" {
			t.Errorf("resolveString() = %q, want the empty resolution: this is not the call being unwrapped", got)
		}
	})
}

// TestAssignmentsTo_AssignmentsToAnotherVariable_AreNotCollected verifies the
// walk asks which variable each assignment binds. A function body assigns to
// several variables, and collecting another one's right-hand side would resolve
// a spec to whatever sat beside it.
func TestAssignmentsTo_AssignmentsToAnotherVariable_AreNotCollected(t *testing.T) {
	info := synthInfo()
	wanted := types.NewVar(token.NoPos, nil, "spec", types.Typ[types.String])
	other := types.NewVar(token.NoPos, nil, "options", types.Typ[types.String])
	ours, theirs := ast.NewIdent("spec"), ast.NewIdent("options")
	info.Defs[ours] = wanted
	info.Defs[theirs] = other
	ourValue, theirValue := ast.NewIdent("ourValue"), ast.NewIdent("theirValue")
	decl := &ast.FuncDecl{Body: &ast.BlockStmt{List: []ast.Stmt{
		&ast.AssignStmt{Lhs: []ast.Expr{theirs}, Rhs: []ast.Expr{theirValue}},
		&ast.AssignStmt{Lhs: []ast.Expr{ours}, Rhs: []ast.Expr{ourValue}},
	}}}

	found := assignmentsTo(synthPkg(info), decl, wanted)

	if len(found) != 1 || found[0] != ourValue {
		t.Errorf("assignmentsTo() collected %d expression(s), want only the one assigned to the variable asked about", len(found))
	}
}

// TestIsStringPassThrough_CallsThatReturnSomethingOfTheirOwn_AreRefused
// verifies all three parts of the one pass-through this resolver applies. A
// function belonging to no package has no path to compare, a function of
// another package named TrimSpace is not the one the constructors apply, and
// neither is another function of the strings package.
func TestIsStringPassThrough_CallsThatReturnSomethingOfTheirOwn_AreRefused(t *testing.T) {
	strings := types.NewPackage("strings", "strings")
	elsewhere := types.NewPackage("example.com/text", "text")
	cases := []struct {
		name   string
		callee *types.Func
	}{
		{name: "a function belonging to no package", callee: types.NewFunc(token.NoPos, nil, "TrimSpace", nil)},
		{name: "another package's TrimSpace", callee: types.NewFunc(token.NoPos, elsewhere, "TrimSpace", nil)},
		{name: "another function of the strings package", callee: types.NewFunc(token.NoPos, strings, "ToUpper", nil)},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if isStringPassThrough(testCase.callee) {
				t.Error("isStringPassThrough() = true, want false")
			}
		})
	}
	if !isStringPassThrough(types.NewFunc(token.NoPos, strings, "TrimSpace", nil)) {
		t.Error("isStringPassThrough(strings.TrimSpace) = false, so the refusals above prove nothing")
	}
}

// TestReturnsOf_ReturnsThatCarryNoExpression_AreSkipped verifies the body walk
// reads the result it is after only where the return statement has one. A
// helper that returns early writes a bare return, and a guard against that
// which stopped one short of the results would read past the end of the
// statement rather than step over it.
func TestReturnsOf_ReturnsThatCarryNoExpression_AreSkipped(t *testing.T) {
	info := synthInfo()
	str := types.Typ[types.String]
	callee := synthFunc(types.NewPackage("example.com/domain", "domain"), "nameFor", nil, []types.Type{str})
	own := synthString(info, "the.action")
	res := synthIndexed(info, callee, &ast.BlockStmt{List: []ast.Stmt{
		&ast.ReturnStmt{},
		&ast.ReturnStmt{Results: []ast.Expr{own}},
	}})

	found := res.returnsOf(callee, synthCall(info, callee), synthFrame(info, nil), "")

	if len(found) != 1 || found[0].expr != own {
		t.Errorf("returnsOf() yielded %d expression(s), want only the return that carries one", len(found))
	}
}

// TestReturnsOf_CalleeWithNoBody_YieldsNothing verifies a callee the audit
// indexed without a body is stepped over rather than walked. A declaration
// whose body is elsewhere has no return statement to read, and walking a nil
// body would end the run on a panic.
func TestReturnsOf_CalleeWithNoBody_YieldsNothing(t *testing.T) {
	info := synthInfo()
	callee := synthFunc(types.NewPackage("example.com/domain", "domain"), "nameFor", nil, []types.Type{types.Typ[types.String]})
	res := &resolver{prog: &program{funcs: map[*types.Func]*function{callee: {decl: &ast.FuncDecl{}}}}}

	if found := res.returnsOf(callee, synthCall(info, callee), synthFrame(info, nil), ""); found != nil {
		t.Errorf("returnsOf() = %v, want nothing from a callee with no body", found)
	}
}

// TestBindParams_ArgumentsAndParametersOfDifferentLengths_StopAtTheShorter
// verifies the binding walks both lists at once. A call may carry fewer
// arguments than the signature declares, or more, and reading past the end of
// either would end the run on a panic instead of on a finding.
func TestBindParams_ArgumentsAndParametersOfDifferentLengths_StopAtTheShorter(t *testing.T) {
	str := types.Typ[types.String]
	pkg := types.NewPackage("example.com/domain", "domain")
	first, second := ast.NewIdent("first"), ast.NewIdent("second")

	t.Run("more arguments than parameters", func(t *testing.T) {
		callee := synthFunc(pkg, "one", []types.Type{str}, nil)

		env := bindParams(callee, &ast.CallExpr{Args: []ast.Expr{first, second}}, frame{})

		if len(env) != 1 {
			t.Errorf("bindParams() bound %d parameter(s), want the one the signature declares", len(env))
		}
	})
	t.Run("fewer arguments than parameters", func(t *testing.T) {
		callee := synthFunc(pkg, "two", []types.Type{str, str}, nil)

		env := bindParams(callee, &ast.CallExpr{Args: []ast.Expr{first}}, frame{})

		if len(env) != 1 {
			t.Errorf("bindParams() bound %d parameter(s), want the one the call supplies", len(env))
		}
	})
}

// TestSpecConstructorArgs_TheConstructorShape_IsTheBaseCase verifies the shape
// match accepts the smallest constructor it is written for: two parameters,
// two arguments, a string first and a route second, returning a spec. The
// refusals beside it are what keep a toolutil function that merely resembles
// one from being read as the base case, and without a case that is accepted
// they would be satisfied by a match that accepts nothing at all.
func TestSpecConstructorArgs_TheConstructorShape_IsTheBaseCase(t *testing.T) {
	toolutilPkg, specType, routeType := synthToolutilTypes()
	str := types.Typ[types.String]
	name, route := ast.NewIdent("name"), ast.NewIdent("route")
	call := &ast.CallExpr{Args: []ast.Expr{name, route}}

	t.Run("the shape is accepted", func(t *testing.T) {
		callee := synthFunc(toolutilPkg, "NewActionSpec", []types.Type{str, routeType}, []types.Type{specType})

		nameArg, routeArg, ok := specConstructorArgs(callee, call)

		if !ok {
			t.Fatal("specConstructorArgs() refused the constructor shape")
		}
		if nameArg != name || routeArg != route {
			t.Errorf("specConstructorArgs() = %v, %v, want the call's own two arguments", nameArg, routeArg)
		}
	})
	t.Run("a callee with no signature to read", func(t *testing.T) {
		if _, _, ok := specConstructorArgs(types.NewFunc(token.NoPos, toolutilPkg, "New", nil), call); ok {
			t.Error("specConstructorArgs() matched a callee with no signature")
		}
	})
	t.Run("fewer parameters than the shape", func(t *testing.T) {
		callee := synthFunc(toolutilPkg, "New", []types.Type{str}, []types.Type{specType})

		if _, _, ok := specConstructorArgs(callee, call); ok {
			t.Error("specConstructorArgs() matched a callee of one parameter")
		}
	})
	t.Run("fewer arguments than the shape", func(t *testing.T) {
		callee := synthFunc(toolutilPkg, "NewActionSpec", []types.Type{str, routeType}, []types.Type{specType})

		if _, _, ok := specConstructorArgs(callee, &ast.CallExpr{Args: []ast.Expr{name}}); ok {
			t.Error("specConstructorArgs() matched a call of one argument")
		}
	})
}

// TestIsAppendCall_CallsThatAppendNothingResolvable_AreRefused verifies what
// the element loop is entered for. The loop reads every argument after the
// first as a spec, so a call that is not the builtin, one whose elements are
// spread from a slice, and one with no element at all each have to be refused:
// the first would resolve someone else's arguments, and the others have no
// element the loop could read.
func TestIsAppendCall_CallsThatAppendNothingResolvable_AreRefused(t *testing.T) {
	appendBuiltin, _ := types.Universe.Lookup("append").(*types.Builtin)
	copyBuiltin, _ := types.Universe.Lookup("copy").(*types.Builtin)
	if appendBuiltin == nil || copyBuiltin == nil {
		t.Fatal("the universe scope no longer declares append and copy as builtins")
	}
	slice, element := ast.NewIdent("specs"), ast.NewIdent("spec")

	cases := []struct {
		name string
		call func(info *types.Info) *ast.CallExpr
		want bool
	}{
		{
			name: "the builtin with an element",
			want: true,
			call: func(info *types.Info) *ast.CallExpr {
				return appendCall(info, appendBuiltin, token.NoPos, slice, element)
			},
		},
		{
			name: "a function of another name",
			call: func(info *types.Info) *ast.CallExpr {
				fun := ast.NewIdent("extend")
				info.Uses[fun] = appendBuiltin

				return &ast.CallExpr{Fun: fun, Args: []ast.Expr{slice, element}}
			},
		},
		{
			name: "an append that spreads a slice",
			call: func(info *types.Info) *ast.CallExpr {
				return appendCall(info, appendBuiltin, token.Pos(1), slice, element)
			},
		},
		{
			name: "an append with no element",
			call: func(info *types.Info) *ast.CallExpr {
				return appendCall(info, appendBuiltin, token.NoPos, slice)
			},
		},
		{
			name: "a name shadowing the builtin",
			call: func(info *types.Info) *ast.CallExpr {
				fun := ast.NewIdent("append")
				info.Uses[fun] = types.NewFunc(token.NoPos, types.NewPackage("example.com/domain", "domain"), "append", nil)

				return &ast.CallExpr{Fun: fun, Args: []ast.Expr{slice, element}}
			},
		},
		{
			name: "another builtin under that name",
			call: func(info *types.Info) *ast.CallExpr {
				return appendCall(info, copyBuiltin, token.NoPos, slice, element)
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			info := synthInfo()

			if got := isAppendCall(synthPkg(info), testCase.call(info)); got != testCase.want {
				t.Errorf("isAppendCall() = %t, want %t", got, testCase.want)
			}
		})
	}
}

// appendCall builds a call written as append, bound to the given object.
func appendCall(info *types.Info, obj types.Object, ellipsis token.Pos, args ...ast.Expr) *ast.CallExpr {
	fun := ast.NewIdent("append")
	info.Uses[fun] = obj

	return &ast.CallExpr{Fun: fun, Args: args, Ellipsis: ellipsis}
}
