package main

import (
	"sort"
	"strings"
	"testing"
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
