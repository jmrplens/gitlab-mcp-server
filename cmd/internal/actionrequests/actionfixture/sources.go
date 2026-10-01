package actionfixture

// Vuln is one package holding a read document and a mutation document,
// handlers for each, a handler that reaches the mutation only through a
// callee, and action specs that classify some of them honestly and one of them
// wrongly.
//
// It is written the way a real domain is written, including the package-local
// spec helper that forwards to toolutil, because the resolver's whole job is
// to follow that forwarding.
const Vuln = `package vuln

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const listQuery = @@
query($fullPath: ID!) {
  group(fullPath: $fullPath) {
    vulnerabilities { nodes { id } }
  }
}
@@

const dismissMutation = @@
mutation($id: VulnerabilityID!) {
  vulnerabilityDismiss(input: {id: $id}) {
    errors
  }
}
@@

// Input is the shared input for every fixture handler.
type Input struct {
	ID string @@json:"id"@@
}

// Output is the shared output for every fixture handler.
type Output struct {
	OK bool @@json:"ok"@@
}

// List sends the read document.
func List(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return send(ctx, client, listQuery, input)
}

// Dismiss sends the mutation document itself.
func Dismiss(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return send(ctx, client, dismissMutation, input)
}

// DismissIndirect names no document: it reaches the mutation through a callee,
// which is what the call graph has to see.
func DismissIndirect(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return dismissAll(ctx, client, input)
}

func dismissAll(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return send(ctx, client, dismissMutation, input)
}

// InlineWrite writes with a document that is never given a name.
func InlineWrite(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return send(ctx, client, @@
mutation($id: VulnerabilityID!) {
  vulnerabilityConfirm(input: {id: $id}) {
    errors
  }
}
@@, input)
}

func send(ctx context.Context, client *gitlabclient.Client, query string, input Input) (Output, error) {
	var response struct {
		Data map[string]any @@json:"data"@@
	}
	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     query,
		Variables: map[string]any{"id": input.ID},
	}, &response, gl.WithContext(ctx))
	return Output{OK: err == nil}, err
}

// ActionSpecs declares the fixture's actions. "list" is honest, "dismiss" is
// classified as a write and so is not a finding, "read_dismiss" and
// "read_dismiss_indirect" are the constructed violations.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		readSpec("list", toolutil.RouteAction(client, List)),
		toolutil.NewCreateActionSpec("dismiss", toolutil.RouteAction(client, Dismiss), toolutil.ActionSpecOptions{}),
		readSpec("read_dismiss", toolutil.RouteAction(client, Dismiss)),
		readSpec("read_dismiss_indirect", toolutil.RouteAction(client, DismissIndirect)),
		readSpec("read_inline", toolutil.RouteAction(client, InlineWrite)),
	}
}

func readSpec(name string, route toolutil.ActionRoute) toolutil.ActionSpec {
	options := toolutil.ActionSpecOptions{Usage: "fixture"}
	return toolutil.NewReadActionSpec(name, route, options)
}
`

// Shapes declares one action per spelling an ActionSpec is written in across
// this repository, each routed to its own handler, so the resolver is held to
// following the forwarding rather than to recognizing one shape.
//
// The closure action exists because a route may be built from a function
// literal, which has no declared function to use as a call-graph root; its body
// reaches the mutation, so a resolver that dropped literals would report
// nothing for it.
const Shapes = `package shapes

import (
	"context"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests/fixture/other"
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
	ID string @@json:"id"@@
}

// Output is the shared output for every fixture handler.
type Output struct {
	OK bool @@json:"ok"@@
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

// Wrapped is the handler behind a route whose 404 WrapNotFound answers.
func Wrapped(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
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
		Data map[string]any @@json:"data"@@
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
		toolutil.NewReadActionSpec("wrapped", toolutil.RouteAction(client, Wrapped).WrapNotFound(func(map[string]any) any { return nil }), toolutil.ActionSpecOptions{}),
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

// Other holds the pieces a spec can reach across a package boundary: a
// handler named through a selector, and a route held in a package-level
// variable, which the resolver deliberately does not follow.
//
// It also declares an action called "quiet", which is the name [Shapes] gives
// its REST-only action, routed here to a handler that sends a mutation. Two
// packages declaring one action name is what makes the owning package's part
// in resolution observable: the catalog says which of them declares the action
// it is asking about, and taking the other one would report a mutation against
// an action whose handler touches no GraphQL at all.
const Other = `package other

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
	ID string @@json:"id"@@
}

// Output is the fixture handler output.
type Output struct {
	OK bool @@json:"ok"@@
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
		Data map[string]any @@json:"data"@@
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

// Vars declares GraphQL documents as package-level variables rather than
// constants, plus the two variable shapes that carry no knowable value.
const Vars = `package vars

var declaredMutation = @@
mutation($id: ID!) {
  thing(input: {id: $id}) { errors }
}
@@

var computedMutation = build()

var undeclared string

func build() string {
	return "mutation { thing { errors } }"
}
`

// Edge writes the shapes the inventory's rule and the readonly audit's own
// rule used to judge differently, in both directions, with the handlers and
// specs that make the gate answer for them end to end.
//
// headedMutation carries its operation under a header line, so the document
// does not open the string it is written in. spacelessRead is a one-field
// selection set written without a space, which is also how a UCUM unit
// annotation is written. The inventory refused both, the audit's own rule read
// both, and a mutation written either way was therefore judged by nothing
// while the gate went on printing a clean run. secondLine is the other
// direction: a document the inventory read and the audit's rule did not, which
// cost a finding rather than a silence.
const Edge = `package edges

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const headedMutation = @@
Sent to GitLab:
mutation($id: ID!) {
  thing(input: {id: $id}) { errors }
}
@@

const spacelessRead = @@{__typename}@@

// Input is the shared input for every fixture handler.
type Input struct {
	ID string @@json:"id"@@
}

// Output is the shared output for every fixture handler.
type Output struct {
	OK bool @@json:"ok"@@
}

// Touch sends the mutation written under a header line.
func Touch(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return send(ctx, client, headedMutation, input)
}

// Typename sends the document written without spaces.
func Typename(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return send(ctx, client, spacelessRead, input)
}

// secondLine writes a document inline whose selection set opens on the line
// below its keyword.
func secondLine() string {
	return @@query
{ thing { id } }@@
}

func send(ctx context.Context, client *gitlabclient.Client, query string, input Input) (Output, error) {
	var response struct {
		Data map[string]any @@json:"data"@@
	}
	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     query,
		Variables: map[string]any{"id": input.ID},
	}, &response, gl.WithContext(ctx))
	return Output{OK: err == nil}, err
}

// ActionSpecs declares one read action per document: "read_touch" is the
// constructed violation and "typename" is honest.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		readSpec("read_touch", toolutil.RouteAction(client, Touch)),
		readSpec("typename", toolutil.RouteAction(client, Typename)),
	}
}

func readSpec(name string, route toolutil.ActionRoute) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, toolutil.ActionSpecOptions{Usage: "fixture"})
}
`

// Unplaced writes a document in the one shape no walk can place: assembled
// where it is used, in a package-level initializer, so it is bound to no
// object and sits in no function body. It is a package of its own because it
// fails every readonly audit run that loads it, which is the point.
const Unplaced = `package unplaced

var sent = pass("mutation {" + " thing { errors } }")

func pass(document string) string {
	return document
}
`

// Requests writes the shapes the walk from a handler to what it reaches had to
// learn, each the way a domain of this tree writes it:
//
//   - a test seam holding a function literal (the group boards'
//     newRawRequest), a method expression (commits' newRequest) and a
//     dispatch table of literals (the integrations' getters), all
//     package-level variables a handler reaches only by naming them;
//   - a route helper that wraps the delete it is handed in a literal of its
//     own (awardEmojiDeleteRoute), called twice with two deletes;
//   - two lists declaring one set of action names, told apart only by the
//     individual tool name each one's options declare (the badge twins),
//     through a helper that amends the options, a field assignment of the
//     whole tool spec, and a field assignment of its name over a literal that
//     wrote another.
const Requests = `package requests

import (
	"context"
	"errors"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Input is the fixture handler input.
type Input struct {
	ID string @@json:"id"@@
}

// Output is the fixture handler output.
type Output struct {
	OK bool @@json:"ok"@@
}

// newRawRequest is a test seam holding a function literal.
var newRawRequest = func(client *gitlabclient.Client, path string) error {
	_, err := client.GL().NewRequest("GET", path, nil, nil)
	return err
}

// newRequest is a test seam holding a method expression.
var newRequest = (*gl.Client).NewRequest

// getters is a dispatch table selected by the caller's input.
var getters = map[string]func(context.Context, gl.ProjectsServiceInterface, string) error{
	"get": func(ctx context.Context, projects gl.ProjectsServiceInterface, id string) error {
		_, _, err := projects.GetProject(id, nil, gl.WithContext(ctx))
		return err
	},
	"delete": func(ctx context.Context, projects gl.ProjectsServiceInterface, id string) error {
		_, err := projects.DeleteProject(id, nil, gl.WithContext(ctx))
		return err
	},
}

// first and second are initialized from one call.
var first, second = pair()

// unset has no initializer to index.
var unset func()

func pair() (string, string) {
	return "first", "second"
}

// Raw reaches a request only through the literal seam.
func Raw(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_ = ctx
	return Output{}, newRawRequest(client, input.ID)
}

// Expression reaches NewRequest only through the method expression seam.
func Expression(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_, err := newRequest(client.GL(), "GET", input.ID, nil, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	return Output{}, err
}

// Table reaches every entry of the dispatch table.
func Table(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, getters[input.ID](ctx, client.GL().Projects, input.ID)
}

// Pair names the second of two variables one call initializes, an unset one,
// and a variable of a package outside the load.
func Pair(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_, _ = ctx, client
	if unset != nil {
		return Output{}, errors.ErrUnsupported
	}
	return Output{OK: second == input.ID}, nil
}

// Direct names a service method in a call and another as a method value.
func Direct(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_, _, err := client.GL().Projects.GetProject(input.ID, nil, gl.WithContext(ctx))
	list := client.GL().Projects.ListProjects
	_ = list
	return Output{}, err
}

// DeleteProject is the first delete handed to the route helper.
func DeleteProject(ctx context.Context, client *gitlabclient.Client, input Input) error {
	_, err := client.GL().Projects.DeleteProject(input.ID, nil, gl.WithContext(ctx))
	return err
}

// DeleteIssue is the second.
func DeleteIssue(ctx context.Context, client *gitlabclient.Client, input Input) error {
	_, err := client.GL().Issues.DeleteIssue(input.ID, 1, gl.WithContext(ctx))
	return err
}

// deleteRoute wraps the delete it is handed in a literal of its own, which is
// one literal bound to a different delete at each call.
func deleteRoute(client *gitlabclient.Client, fn func(context.Context, *gitlabclient.Client, Input) error) toolutil.ActionRoute {
	return toolutil.DestructiveAction(client, func(ctx context.Context, client *gitlabclient.Client, input Input) (toolutil.DeleteOutput, error) {
		if err := fn(ctx, client, input); err != nil {
			return toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteOutput{Status: "success"}, nil
	})
}

// ProjectSpecs declares the project half of the twins and the seams.
func ProjectSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		twinSpec("twin_get", toolutil.RouteAction(client, Direct), "gitlab_project_twin_get"),
		toolutil.NewReadActionSpec("twin_field", toolutil.RouteAction(client, Raw), fieldOptions("gitlab_project_twin_field")),
		toolutil.NewReadActionSpec("expression", toolutil.RouteAction(client, Expression), toolutil.ActionSpecOptions{}),
		toolutil.NewDeleteActionSpec("delete_project", deleteRoute(client, DeleteProject), toolutil.ActionSpecOptions{}),
		toolutil.NewDeleteActionSpec("delete_issue", deleteRoute(client, DeleteIssue), toolutil.ActionSpecOptions{}),
		toolutil.ActionSpec{
			Name:           "literal_tool",
			Route:          toolutil.RouteAction(client, Pair),
			IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_literal_tool"},
		},
	}
}

// GroupSpecs declares the group half of the twins, under the same names.
func GroupSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		twinSpec("twin_get", toolutil.RouteAction(client, Pair), "gitlab_group_twin_get"),
		toolutil.NewReadActionSpec("twin_field", toolutil.RouteAction(client, Table), nameOptions("gitlab_group_twin_field")),
	}
}

func twinSpec(name string, route toolutil.ActionRoute, tool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, twinOptions(name, tool))
}

// twinOptions builds the options in a literal and hands them through a helper
// that amends them, as the badge helpers do.
func twinOptions(name, tool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{Usage: name, IndividualTool: toolutil.IndividualToolSpec{Name: tool}}
	return guidance(options)
}

func guidance(options toolutil.ActionSpecOptions) toolutil.ActionSpecOptions {
	options.Tags = []string{"twin"}
	return options
}

// fieldOptions writes the whole tool spec by a field assignment.
func fieldOptions(tool string) toolutil.ActionSpecOptions {
	var options toolutil.ActionSpecOptions
	options.IndividualTool = toolSpec(tool)
	return options
}

// nameOptions writes the tool's name by a field assignment, over a literal
// that wrote another.
func nameOptions(tool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{IndividualTool: toolutil.IndividualToolSpec{Name: "overwritten"}}
	options.IndividualTool.Name = tool
	return options
}

func toolSpec(tool string) toolutil.IndividualToolSpec {
	return toolutil.IndividualToolSpec{Name: tool}
}
`
