package derive

import (
	"fmt"
	"go/token"
	"go/types"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests/actionfixture"
)

// grantsSource is the fixture package the derivation is held to: one handler
// per shape a body can take, each routed through an ActionSpec the way a
// domain package writes one, so the walk meets them the way it meets the tree.
const grantsSource = `package grants

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Input is every handler's input; an ordinary comment like this one is not a
// directive.
type Input struct {
	ID     string @@json:"id"@@
	Method string @@json:"method"@@
	N      int    @@json:"n"@@
}

type Output struct {
	OK bool @@json:"ok"@@
}

const listQuery = @@
query($id: ID!) { project(fullPath: $id) { id } }
@@

func get(ctx context.Context, client *gitlabclient.Client, id string) error {
	_, _, err := client.GL().Projects.GetProject(id, nil, gl.WithContext(ctx))
	return err
}

func del(ctx context.Context, client *gitlabclient.Client, id string) error {
	_, err := client.GL().Projects.DeleteProject(id, nil, gl.WithContext(ctx))
	return err
}

func edit(ctx context.Context, client *gitlabclient.Client, id string) error {
	_, _, err := client.GL().Projects.EditProject(id, nil, gl.WithContext(ctx))
	return err
}

func both(ctx context.Context, client *gitlabclient.Client, id string) error {
	if err := get(ctx, client, id); err != nil {
		return err
	}
	return del(ctx, client, id)
}

func choose(ctx context.Context, client *gitlabclient.Client, id string, first, second func(context.Context, *gitlabclient.Client, string) error) error {
	if id == "" {
		return first(ctx, client, id)
	}
	return second(ctx, client, id)
}

func pickAny(first, second any) any {
	if first != nil {
		return first
	}
	return second
}

func generic[T any](ctx context.Context, client *gitlabclient.Client, id string, _ T) error {
	return get(ctx, client, id)
}

func generic2[T, U any](ctx context.Context, client *gitlabclient.Client, id string) error {
	return del(ctx, client, id)
}

func recurse(ctx context.Context, client *gitlabclient.Client, id string) error {
	if id == "" {
		return recurse(ctx, client, "x")
	}
	return get(ctx, client, id)
}

func relay(ctx context.Context, client *gitlabclient.Client, id string) (Output, error) {
	return Output{}, del(ctx, client, id)
}

var handlers = map[string]func(context.Context, *gitlabclient.Client, string) error{
	"get": get,
	"del": del,
}

var seam = (*gl.Client).NewRequest

var helperSeam = func(client *gitlabclient.Client, path string) error {
	_, err := client.GL().NewRequest(http.MethodGet, path, nil, nil)
	return err
}

func projectPath(id string) string {
	return "projects/" + url.PathEscape(id)
}

func loop(s string) string {
	return loop(s)
}

func named() (s string) {
	s = "named"
	return
}

func count(n int) int {
	return n
}

func withClosure(id string) string {
	inner := func() string { return "inner" }
	_ = inner
	return "outer/" + id
}

// NewRequest shares client-go's constructor name and is a function of ours.
func NewRequest(ctx context.Context, client *gitlabclient.Client, id string) error {
	return edit(ctx, client, id)
}

var qualified = fmt.Sprint

var baseURL = (*gl.Client).BaseURL

func Nothing(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_, _, _ = ctx, client, input
	//gitlab:request alternatives: neither sends anything
	_ = named()
	return Output{}, nil
}

func Refusals(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	if err := get(ctx, client, input.ID); err != nil {
		return Output{}, err
	}
	switch {
	case input.N > 0:
		_ = named()
		return Output{}, errors.New("positive")
	default:
		_ = named()
		return Output{}, errors.New("not positive")
	}
}

func Seams(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_ = qualified(input.ID)
	_ = baseURL(client.GL())
	return Output{}, NewRequest(ctx, client, input.ID)
}

func Spellings(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_ = ctx
	var base = "projects"
	p := "a"
	p = "b"
	p = "c"
	p = "d"
	p = "e"
	q := "v"
	q = "w"
	q = "x"
	q = "y"
	_, err := client.GL().NewRequest("GET", base+"/"+p+"/"+q+"/"+withClosure(input.ID), nil, nil)
	return Output{}, err
}

func Wide(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_ = ctx
	gl := client.GL()
	if input.N == 1 { _, _ = gl.NewRequest("GET", "w1", nil, nil) } else { _, _ = gl.NewRequest("GET", "x1", nil, nil) }
	if input.N == 2 { _, _ = gl.NewRequest("GET", "w2", nil, nil) } else { _, _ = gl.NewRequest("GET", "x2", nil, nil) }
	if input.N == 3 { _, _ = gl.NewRequest("GET", "w3", nil, nil) } else { _, _ = gl.NewRequest("GET", "x3", nil, nil) }
	if input.N == 4 { _, _ = gl.NewRequest("GET", "w4", nil, nil) } else { _, _ = gl.NewRequest("GET", "x4", nil, nil) }
	if input.N == 5 { _, _ = gl.NewRequest("GET", "w5", nil, nil) } else { _, _ = gl.NewRequest("GET", "x5", nil, nil) }
	return Output{}, nil
}

func Wide2(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_ = ctx
	gl := client.GL()
	if input.N == 1 { _, _ = gl.NewRequest("GET", "y1", nil, nil) } else { _, _ = gl.NewRequest("GET", "z1", nil, nil) }
	if input.N == 2 { _, _ = gl.NewRequest("GET", "y2", nil, nil) } else { _, _ = gl.NewRequest("GET", "z2", nil, nil) }
	if input.N == 3 { _, _ = gl.NewRequest("GET", "y3", nil, nil) } else { _, _ = gl.NewRequest("GET", "z3", nil, nil) }
	if input.N == 4 { _, _ = gl.NewRequest("GET", "y4", nil, nil) } else { _, _ = gl.NewRequest("GET", "z4", nil, nil) }
	return Output{}, nil
}

func Sequence(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	if err := get(ctx, client, input.ID); err != nil {
		return Output{}, err
	}
	return Output{}, del(ctx, client, input.ID)
}

func Branches(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	var err error
	if input.ID == "" {
		err = get(ctx, client, input.ID)
	} else if input.N > 0 {
		err = del(ctx, client, input.ID)
	} else {
		err = edit(ctx, client, input.ID)
	}
	return Output{}, err
}

func Optional(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	if input.N > 0 {
		_ = get(ctx, client, input.ID)
	}
	return Output{}, nil
}

func Switches(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	switch input.ID {
	case "get":
		_ = get(ctx, client, input.ID)
	case "del":
		_ = del(ctx, client, input.ID)
	default:
		_ = edit(ctx, client, input.ID)
	}
	switch {
	case input.N > 1:
		return Output{}, errors.New("refused")
	}
	return Output{}, nil
}

func Kinds(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	var value any = input
	switch typed := value.(type) {
	case Input:
		_ = get(ctx, client, typed.ID)
	case *Input:
		_ = del(ctx, client, typed.ID)
	}
	select {
	case <-ctx.Done():
		return Output{}, ctx.Err()
	default:
	}
	return Output{}, nil
}

func Loops(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	for i := 0; i < input.N; i++ {
		_ = get(ctx, client, input.ID)
	}
	for range input.N {
		_ = del(ctx, client, input.ID)
	}
	return Output{}, nil
}

func Directed(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	//gitlab:request mandatory: the caller always passes at least one
	for range input.N {
		_ = get(ctx, client, input.ID)
	}
	//gitlab:request optional: the edit only enriches the answer
	_ = edit(ctx, client, input.ID)
	//gitlab:request alternatives: exactly one of the two runs
	_, _ = get(ctx, client, input.ID), del(ctx, client, input.ID)
	//gitlab:request mandatory: the lookup runs before the rest on every call
	if err := both(ctx, client, input.ID); err != nil {
		return Output{}, err
	}
	//gitlab:request alternatives: the branch picks one
	if input.N > 2 {
		_ = get(ctx, client, input.ID)
	} else {
		_ = del(ctx, client, input.ID)
	}
	return Output{}, nil
}

func Stale(input Input) {
	//gitlab:request sometimes: not a kind
	_ = input
	//gitlab:request mandatory
	_ = input
	//gitlab:request mandatory: nothing here sends a request
	_ = input
}

func Closures(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	run := func() error { return get(ctx, client, input.ID) }
	_ = run
	func() { _ = del(ctx, client, input.ID) }()
	return Output{}, nil
}

func Tables(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	pick := []func(context.Context, *gitlabclient.Client, string) error{
		func(ctx context.Context, client *gitlabclient.Client, id string) error { return edit(ctx, client, id) },
		get,
	}
	_ = pick
	return Output{}, handlers[input.ID](ctx, client, input.ID)
}

func Choices(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_ = pickAny(client.GL().Projects.GetProject, client.GL().Projects.DeleteProject)
	_ = pickAny(fmt.Sprint, input)
	_ = pickAny(pickAny(nil, nil), nil)
	return Output{}, choose(ctx, client, input.ID, get, del)
}

func Documents(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	var out struct{}
	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{Query: listQuery}, &out, gl.WithContext(ctx))
	if err != nil {
		return Output{}, err
	}
	_, err = client.GL().GraphQL.Do(gl.GraphQLQuery{Query: @@mutation { thingCreate(input: {}) { errors } }@@}, &out, gl.WithContext(ctx))
	label := "not a document"
	_ = label
	_ = 42
	return Output{}, err
}

func Unsent(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_ = listQuery
	_, _, _ = client.GL().Projects.ListProjects(nil, gl.WithContext(ctx))
	return Output{}, get(ctx, client, input.ID)
}

func Raws(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	path := fmt.Sprintf("projects/%s/things/%d?x=1", url.PathEscape(input.ID), input.N)
	if input.N > 1 {
		path = projectPath(input.ID) + "/other"
	}
	_, err := client.GL().NewRequest(http.MethodPost, path, nil, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return Output{}, err
	}
	_, err = seam(client.GL(), http.MethodDelete, "/api/v4/projects/"+input.ID, nil, nil)
	if err != nil {
		return Output{}, err
	}
	return Output{}, helperSeam(client, "groups/"+input.ID)
}

func RawUnknowns(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_ = ctx
	method := input.Method
	_, err := client.GL().NewRequest(method, "projects", nil, nil)
	if err != nil {
		return Output{}, err
	}
	_, err = client.GL().NewRequest("GET", fmt.Sprintf("a/%%/%05d/%s", input.N), nil, nil)
	if err != nil {
		return Output{}, err
	}
	_, err = client.GL().NewRequestToURL("GET", &url.URL{}, nil, nil)
	return Output{}, err
}

func Folds(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_ = ctx
	const fixed = 5
	ids := []string{input.ID}
	ptr := &input.ID
	var value any = input.ID
	var unset string
	var number int
	name := "named"
	first, second := named(), named()
	var a, b = named(), named()
	_, _, _, _ = first, second, a, b
	_, _ = client.GL().NewRequest("GET", fmt.Sprintf("a/%d/%d/%d", fixed, input.N*2, count(input.N)), nil, nil)
	_, _ = client.GL().NewRequest("GET", "b/"+ids[0]+"/"+*ptr, nil, nil)
	_, _ = client.GL().NewRequest("GET", "c/"+value.(string), nil, nil)
	_, _ = client.GL().NewRequest("GET", "d/"+loop(input.ID), nil, nil)
	_, _ = client.GL().NewRequest("GET", "e/"+named(), nil, nil)
	_, _ = client.GL().NewRequest("GET", "f/"+string(name), nil, nil)
	_, _ = client.GL().NewRequest("GET", "g/"+func() string { return "x" }(), nil, nil)
	_, _ = client.GL().NewRequest("GET", "h/"+unset, nil, nil)
	_, _ = client.GL().NewRequest("GET", fmt.Sprintf("i/%v/%d", get, number), nil, nil)
	return Output{}, nil
}

func Overflow(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_ = ctx
	gl := client.GL()
	if input.N == 1 { _, _ = gl.NewRequest("GET", "a1", nil, nil) } else { _, _ = gl.NewRequest("GET", "b1", nil, nil) }
	if input.N == 2 { _, _ = gl.NewRequest("GET", "a2", nil, nil) } else { _, _ = gl.NewRequest("GET", "b2", nil, nil) }
	if input.N == 3 { _, _ = gl.NewRequest("GET", "a3", nil, nil) } else { _, _ = gl.NewRequest("GET", "b3", nil, nil) }
	if input.N == 4 { _, _ = gl.NewRequest("GET", "a4", nil, nil) } else { _, _ = gl.NewRequest("GET", "b4", nil, nil) }
	if input.N == 5 { _, _ = gl.NewRequest("GET", "a5", nil, nil) } else { _, _ = gl.NewRequest("GET", "b5", nil, nil) }
	if input.N == 6 { _, _ = gl.NewRequest("GET", "a6", nil, nil) } else { _, _ = gl.NewRequest("GET", "b6", nil, nil) }
	if input.N == 7 { _, _ = gl.NewRequest("GET", "a7", nil, nil) } else { _, _ = gl.NewRequest("GET", "b7", nil, nil) }
	if input.N == 8 { _, _ = gl.NewRequest("GET", "a8", nil, nil) } else { _, _ = gl.NewRequest("GET", "b8", nil, nil) }
	if input.N == 9 { _, _ = gl.NewRequest("GET", "a9", nil, nil) } else { _, _ = gl.NewRequest("GET", "b9", nil, nil) }
	return Output{}, nil
}

func Statements(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	results := make(chan error, 1)
	go func() { results <- get(ctx, client, input.ID) }()
	defer func() { _ = del(ctx, client, input.ID) }()
	results <- nil
	counter := 0
	counter++
	{
		_ = edit(ctx, client, input.ID)
	}
outer:
	for i := range 2 {
		if i > counter {
			break outer
		}
		continue
	}
labeled:
	switch input.N {
	case 1:
		break labeled
	}
	type local struct{}
	var _, _ = local{}, counter
	return Output{}, nil
}

func Expressions(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	ids := []string{input.ID}
	_ = ids[0:1]
	var value any = input
	_ = value.(Input)
	ptr := &input
	_ = *ptr
	_ = (input.N + 1) * 2
	_ = input.N > 0 && get(ctx, client, input.ID) == nil
	_ = []byte(input.ID)
	_ = Output{OK: true}
	g := generic2[int, string]
	_ = g
	_ = slices.Contains[[]string]([]string{}, "x")
	_ = generic[int](ctx, client, input.ID, 1)
	return Output{}, generic2[int, string](ctx, client, input.ID)
}

func Terminations(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	if input.N < 0 {
		panic("negative")
	}
	if input.N == 0 {
		{
			return Output{}, nil
		}
	}
	if input.N == 1 {
		if input.ID == "" {
			return Output{}, errors.New("empty")
		} else {
			return Output{}, nil
		}
	}
	if input.N == 2 {
		return relay(ctx, client, input.ID)
	}
	if input.N == 3 {
	}
	if input.N == 4 {
		<-ctx.Done()
	}
	if input.N == 5 {
		print("five")
	}
	if input.N == 6 {
		fmt.Println("six")
	}
	Bare(input)
	return Output{}, recurse(ctx, client, input.ID)
}

func Bare(input Input) {
	if input.N == 0 {
		return
	}
}

func deleteRoute(client *gitlabclient.Client, fn func(context.Context, *gitlabclient.Client, string) error) toolutil.ActionRoute {
	return toolutil.RouteAction(client, func(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
		return Output{}, fn(ctx, client, input.ID)
	})
}

func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		spec("sequence", toolutil.RouteAction(client, Sequence)),
		spec("branches", toolutil.RouteAction(client, Branches)),
		spec("optional", toolutil.RouteAction(client, Optional)),
		spec("switches", toolutil.RouteAction(client, Switches)),
		spec("kinds", toolutil.RouteAction(client, Kinds)),
		spec("loops", toolutil.RouteAction(client, Loops)),
		spec("directed", toolutil.RouteAction(client, Directed)),
		spec("closures", toolutil.RouteAction(client, Closures)),
		spec("tables", toolutil.RouteAction(client, Tables)),
		spec("choices", toolutil.RouteAction(client, Choices)),
		spec("documents", toolutil.RouteAction(client, Documents)),
		spec("unsent", toolutil.RouteAction(client, Unsent)),
		spec("raws", toolutil.RouteAction(client, Raws)),
		spec("raw_unknowns", toolutil.RouteAction(client, RawUnknowns)),
		spec("folds", toolutil.RouteAction(client, Folds)),
		spec("overflow", toolutil.RouteAction(client, Overflow)),
		spec("statements", toolutil.RouteAction(client, Statements)),
		spec("expressions", toolutil.RouteAction(client, Expressions)),
		spec("terminations", toolutil.RouteAction(client, Terminations)),
		spec("bound", deleteRoute(client, del)),
		spec("nothing", toolutil.RouteAction(client, Nothing)),
		spec("refusals", toolutil.RouteAction(client, Refusals)),
		spec("seams", toolutil.RouteAction(client, Seams)),
		spec("spellings", toolutil.RouteAction(client, Spellings)),
		spec("twice", toolutil.RouteAction(client, Sequence)),
		spec("twice", toolutil.RouteAction(client, Branches)),
	}
}

func spec(name string, route toolutil.ActionRoute) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, toolutil.ActionSpecOptions{Usage: "fixture"})
}
`

// fixtureCache memoizes the loaded fixture: loading type-checks it against
// toolutil and client-go, which takes seconds, and every test reads it
// without changing it.
var fixtureCache *actionrequests.Program

// loadGrants loads the fixture program.
func loadGrants(t *testing.T) *actionrequests.Program {
	t.Helper()
	if fixtureCache != nil {
		return fixtureCache
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	root, err := actionfixture.Root(dir)
	if err != nil {
		t.Fatalf("module root: %v", err)
	}
	prog, err := actionrequests.Load(root, []string{actionfixture.Pattern}, actionfixture.Overlay(root, map[string]string{"grants": grantsSource}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	fixtureCache = prog
	return prog
}

// stubSDK answers the derivation's client-go questions from a table.
type stubSDK map[string]stubMethod

// stubMethod is what one client-go method sends.
type stubMethod struct {
	routes    []Request
	documents []Request
}

// Requests answers from the table, and unknown for a method it does not hold.
func (s stubSDK) Requests(key string) (routes, documents []Request, known bool) {
	method, ok := s[key]
	return method.routes, method.documents, ok
}

// fixtureSDK is the client-go the fixture compiles against, as far as the
// fixture calls it: two of the methods route one way, the delete two ways,
// and GetProject is left out so the walk meets a method it cannot read.
func fixtureSDK() stubSDK {
	return stubSDK{
		"Projects.GetProject": {routes: []Request{{Kind: KindREST, Method: "GET", Path: "/projects/:"}}},
		"Projects.DeleteProject": {routes: []Request{
			{Kind: KindREST, Method: "DELETE", Path: "/projects/:"},
			{Kind: KindREST, Method: "DELETE", Path: "/projects/:/full"},
		}},
		"Projects.EditProject": {
			routes: []Request{{Kind: KindREST, Method: "PUT", Path: "/projects/:"}},
			documents: []Request{
				{Kind: KindGraphQL, Document: "query one { a }", Name: "one"},
				{Kind: KindGraphQL, Document: "query two { b }", Name: "two"},
			},
		},
	}
}

// fixtureActions are the fixture's catalog actions.
func fixtureActions() []actionrequests.Action {
	names := []string{
		"sequence", "branches", "optional", "switches", "kinds", "loops", "directed", "closures", "tables",
		"choices", "documents", "unsent", "raws", "raw_unknowns", "folds", "overflow", "statements",
		"expressions", "terminations", "bound", "nothing", "refusals", "seams", "spellings", "twice", "missing",
	}
	actions := make([]actionrequests.Action, len(names))
	for i, name := range names {
		actions[i] = actionrequests.Action{ID: "fixture." + name, Name: name, Owner: "grants"}
	}
	return actions
}

// describe renders an action's derivation the way a test reads it: each
// request with its class, then its paths, and a count where the paths are too
// many to read.
func describe(act *Action) string {
	var b strings.Builder
	if len(act.Paths) > 16 {
		fmt.Fprintf(&b, "%d requests, %d paths\n", len(act.Uses), len(act.Paths))
		return b.String()
	}
	for _, use := range act.Uses {
		fmt.Fprintf(&b, "%s [%s]\n", use.Key(), use.Class)
	}
	for _, path := range act.Paths {
		fmt.Fprintf(&b, "path %v\n", path)
	}
	return b.String()
}

// actionByID finds one action of a result.
func actionByID(t *testing.T, result Result, id string) *Action {
	t.Helper()
	for i := range result.Actions {
		if result.Actions[i].ID == id {
			return &result.Actions[i]
		}
	}
	t.Fatalf("the result holds no %s", id)
	return nil
}

// The two requests the fixture's delete routes to, its read, its edit and the
// two documents the edit posts ("query one" and "query two", in that order),
// spelled once for the expectations below.
const (
	delete1  = "DELETE /projects/: "
	delete2  = "DELETE /projects/:/full "
	getRoute = "GET /projects/: "
	putRoute = "PUT /projects/: "
	queryOne = "graphql ff5e4aaea3a2 "
	queryTwo = "graphql acc2030049ab "
)

// TestDerive_Fixture_ReadsEachShape holds the derivation of every fixture
// shape to what the shape means: a lookup the action fails on is a mandatory
// request before the write; the arms of one branch, a dispatch table and two
// functions handed to one call are alternatives; an if with no else, a loop,
// the right side of && and an early success are optional; a directive turns
// each of those around; a client-go method with two routes is one of them and
// one posting two documents posts both; a raw request's path is folded through
// its locals, helpers and formats, and one nothing static names stays
// unresolved for a declaration to answer. The requests are listed in the
// order the walk meets them, which is the order the handler makes them: the
// read before the delete in a sequence, and the edit's documents in the order
// client-go posts them.
func TestDerive_Fixture_ReadsEachShape(t *testing.T) {
	result := Derive(loadGrants(t), fixtureActions(), fixtureSDK(), nil)
	readThenDelete := getRoute + "[mandatory]\n" + delete1 + "[alternative]\n" + delete2 + "[alternative]\npath [0 1]\npath [0 2]\n"
	oneOfFour := getRoute + "[alternative]\n" + delete1 + "[alternative]\n" + delete2 + "[alternative]\n" + putRoute + "[alternative]\n" +
		queryOne + "[alternative]\n" + queryTwo + "[alternative]\npath [0]\npath [1]\npath [2]\npath [3 4 5]\n"
	maybeEach := getRoute + "[optional]\n" + delete1 + "[optional]\n" + delete2 + "[optional]\npath []\n"
	cases := map[string]string{
		"sequence": readThenDelete,
		"branches": oneOfFour,
		"optional": getRoute + "[optional]\npath []\n",
		"switches": oneOfFour,
		"kinds":    maybeEach,
		"loops":    maybeEach,
		"directed": getRoute + "[mandatory]\n" + putRoute + "[optional]\n" + queryOne + "[optional]\n" + queryTwo + "[optional]\n" +
			delete1 + "[alternative]\n" + delete2 + "[alternative]\npath [0 4]\npath [0 5]\n",
		"closures": readThenDelete,
		"tables": putRoute + "[alternative]\n" + queryOne + "[alternative]\n" + queryTwo + "[alternative]\n" + getRoute + "[alternative]\n" +
			delete1 + "[alternative]\n" + delete2 + "[alternative]\npath [3]\npath [0 1 2 4]\npath [0 1 2 5]\n",
		"choices":   getRoute + "[alternative]\n" + delete1 + "[alternative]\n" + delete2 + "[alternative]\npath [0]\npath [1]\npath [2]\n",
		"documents": "graphql 413d1ae50a40 [mandatory]\ngraphql e9681fdcc8ae [mandatory]\npath [0 1]\n",
		"unsent":    "unresolved sdk-undeclared Projects.ListProjects [mandatory]\n" + getRoute + "[mandatory]\npath [0 1]\n",
		"raws": "POST /projects/:/things/: [alternative]\nPOST /projects/:/other [alternative]\nDELETE /projects/: [mandatory]\n" +
			"GET /groups/: [mandatory]\npath [0 2 3]\npath [1 2 3]\n",
		"raw_unknowns": "unresolved raw-path grants.RawUnknowns [mandatory]\nunresolved raw-url grants.RawUnknowns [mandatory]\npath [0 1]\n",
		"folds": "GET /a/:/:/: [mandatory]\nGET /b/:/: [mandatory]\nunresolved raw-path grants.Folds [mandatory]\nGET /f/named [mandatory]\n" +
			getRoute + "[mandatory]\npath [0 1 2 3 4]\n",
		"overflow": "18 requests, 256 paths\n",
		"statements": getRoute + "[mandatory]\n" + delete1 + "[alternative]\n" + delete2 + "[alternative]\n" + putRoute + "[mandatory]\n" +
			queryOne + "[mandatory]\n" + queryTwo + "[mandatory]\npath [0 1 3 4 5]\npath [0 2 3 4 5]\n",
		"expressions":  readThenDelete,
		"terminations": delete1 + "[optional]\n" + delete2 + "[optional]\n" + getRoute + "[optional]\npath []\n",
		"bound":        delete1 + "[alternative]\n" + delete2 + "[alternative]\npath [0]\npath [1]\n",
		"nothing":      "path []\n",
		"refusals":     getRoute + "[mandatory]\npath [0]\n",
		"seams":        putRoute + "[mandatory]\n" + queryOne + "[mandatory]\n" + queryTwo + "[mandatory]\npath [0 1 2]\n",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			if got := describe(actionByID(t, result, "fixture."+name)); got != want {
				t.Errorf("fixture.%s derives\n%s\nwant\n%s", name, got, want)
			}
		})
	}
}

// TestDerive_Fixture_SpellingsStopAtTheBound verifies a path whose pieces
// multiply past the bound keeps the first sixteen spellings rather than
// growing without end, and that a local declared with a value, a string
// helper and a closure inside it fold like any other piece.
func TestDerive_Fixture_SpellingsStopAtTheBound(t *testing.T) {
	act := actionByID(t, Derive(loadGrants(t), fixtureActions(), fixtureSDK(), nil), "fixture.spellings")
	if len(act.Uses) != maxSpellings {
		t.Fatalf("fixture.spellings derives %d requests, want %d", len(act.Uses), maxSpellings)
	}
	if first := act.Uses[0].Key(); first != "GET /projects/a/v/outer/:" {
		t.Errorf("the first spelling is %q, want GET /projects/a/v/outer/:", first)
	}
}

// TestDerive_Fixture_ReportsWhatItCannotRead verifies every reason the
// derivation stops short is reported: an action meeting no ActionSpec or two,
// one expanding past the path bound, a directive naming no kind or giving no
// reason, and one qualifying nothing any action reaches.
func TestDerive_Fixture_ReportsWhatItCannotRead(t *testing.T) {
	result := Derive(loadGrants(t), fixtureActions(), fixtureSDK(), nil)
	wantAction := map[string]string{
		"fixture.twice":    "fixture.twice meets 2 ActionSpec constructions, so its handler cannot be read",
		"fixture.missing":  "fixture.missing meets 0 ActionSpec constructions, so its handler cannot be read",
		"fixture.overflow": "fixture.overflow expands into more than 256 paths; declare how its requests combine",
	}
	for id, want := range wantAction {
		t.Run(id, func(t *testing.T) {
			if got := actionByID(t, result, id).Findings; !slices.Equal(got, []string{want}) {
				t.Errorf("%s findings = %q, want %q", id, got, want)
			}
		})
	}
	wantSuffixes := []string{
		`: //gitlab:request names "sometimes", which is not optional, mandatory or alternatives`,
		": //gitlab:request mandatory gives no reason",
		": //gitlab:request qualifies no request any action reaches",
		": //gitlab:request qualifies no request any action reaches",
	}
	if len(result.Findings) != len(wantSuffixes) {
		t.Fatalf("findings = %q, want %d", result.Findings, len(wantSuffixes))
	}
	for _, want := range wantSuffixes {
		t.Run(want, func(t *testing.T) {
			if !slices.ContainsFunc(result.Findings, func(finding string) bool { return strings.HasSuffix(finding, want) }) {
				t.Errorf("no finding ends %q among %q", want, result.Findings)
			}
		})
	}
}

// TestDerive_Declarations_AnswerWhatTheWalkCannotRead verifies a declaration
// replaces the unresolved request it names with the requests it declares, all
// of them or any one, marks them qualified and names its category on each;
// that a sends-nothing declaration holds an action the walk finds silent; and
// that a declaration answering nothing is reported, whether it names a request
// no walk reads or declares silent an action that sends.
func TestDerive_Declarations_AnswerWhatTheWalkCannotRead(t *testing.T) {
	declarations := []Declaration{
		{
			Action: "fixture.raw_unknowns", Category: "path-function-value", Replaces: "raw-path grants.RawUnknowns",
			Requests: []Request{{Kind: KindREST, Method: "GET", Path: "/a"}, {Kind: KindREST, Method: "GET", Path: "/b"}},
		},
		{
			Action: "fixture.raw_unknowns", Category: "sdk-path-sprintf", Replaces: "raw-url grants.RawUnknowns", Any: true,
			Requests: []Request{{Kind: KindREST, Method: "GET", Path: "/c"}, {Kind: KindREST, Method: "GET", Path: "/d"}},
		},
		{Action: "fixture.nothing", Category: "sends-nothing"},
		{Action: "fixture.sequence", Category: "sends-nothing"},
		{Action: "fixture.raws", Category: "path-function-value", Replaces: "raw-path nowhere"},
	}
	result := Derive(loadGrants(t), fixtureActions(), fixtureSDK(), declarations)

	unknowns := actionByID(t, result, "fixture.raw_unknowns")
	want := "GET /a [mandatory]\nGET /b [mandatory]\nGET /c [alternative]\nGET /d [alternative]\npath [0 1 2]\npath [0 1 3]\n"
	if got := describe(unknowns); got != want {
		t.Errorf("fixture.raw_unknowns derives\n%s\nwant\n%s", got, want)
	}
	for _, use := range unknowns.Uses {
		if !use.Qualified || use.Declaration == "" {
			t.Errorf("%s is qualified %t by %q, want qualified by its declaration", use.Key(), use.Qualified, use.Declaration)
		}
	}
	if got := actionByID(t, result, "fixture.nothing").Declaration; got != "sends-nothing" {
		t.Errorf("fixture.nothing is declared %q, want sends-nothing", got)
	}
	if got := actionByID(t, result, "fixture.sequence").Declaration; got != "" {
		t.Errorf("fixture.sequence, which sends, is declared %q", got)
	}
	for _, want := range []string{
		"fixture.sequence: the sends-nothing declaration answers nothing the walk reaches " +
			"(the action is declared to send nothing and the walk found a request); remove it, or say what it answers now",
		"fixture.raws: the path-function-value declaration answers nothing the walk reaches " +
			"(no request reads raw-path nowhere); remove it, or say what it answers now",
	} {
		t.Run(want, func(t *testing.T) {
			if !slices.Contains(result.Findings, want) {
				t.Errorf("no finding %q among %q", want, result.Findings)
			}
		})
	}
}

// TestDeriverAction_HandlersWhoseProductPassesTheBound_AreReported verifies
// the bound holds across the handlers of one route as it does inside one: two
// handlers that each stay under it can multiply past it together.
func TestDeriverAction_HandlersWhoseProductPassesTheBound_AreReported(t *testing.T) {
	d := fixtureDeriver(t, fixtureSDK())
	sites := map[string][]actionrequests.Site{"wide": {{
		Package: "grants", Name: "wide",
		Handlers: []actionrequests.Handler{{Func: fixtureFunc(t, "Wide")}, {Func: fixtureFunc(t, "Wide2")}},
	}}}
	act := d.action(actionrequests.Action{ID: "fixture.wide", Name: "wide", Owner: "grants"}, sites, nil)
	want := "fixture.wide expands into more than 256 paths; declare how its requests combine"
	if len(act.Findings) != 1 || act.Findings[0] != want {
		t.Errorf("findings = %q, want %q", act.Findings, want)
	}
	if len(act.Handlers) != 2 {
		t.Errorf("handlers = %v, want both", act.Handlers)
	}
}

// TestDeriverAction_ASiteWithNoHandler_IsReported verifies a construction
// whose route resolves to no handler is a finding rather than an action read
// as sending nothing.
func TestDeriverAction_ASiteWithNoHandler_IsReported(t *testing.T) {
	d := fixtureDeriver(t, fixtureSDK())
	sites := map[string][]actionrequests.Site{"empty": {{Package: "grants", Name: "empty"}}}
	act := d.action(actionrequests.Action{ID: "fixture.empty", Name: "empty", Owner: "grants"}, sites, nil)
	if want := "fixture.empty: its route resolves to no handler"; len(act.Findings) != 1 || act.Findings[0] != want {
		t.Errorf("findings = %q, want %q", act.Findings, want)
	}
}

// TestSymbol_NamesAFunctionByPackageReceiverAndName verifies how the record
// names where a request is made: a method by its receiver's type, through a
// pointer or not, and a function of no package by its name alone.
func TestSymbol_NamesAFunctionByPackageReceiverAndName(t *testing.T) {
	pkg := types.NewPackage("example.com/handlers", "handlers")
	named := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Service", nil), types.NewStruct(nil, nil), nil)
	method := func(recv types.Type) *types.Func {
		sig := types.NewSignatureType(types.NewVar(token.NoPos, pkg, "s", recv), nil, nil, nil, nil, false)
		return types.NewFunc(token.NoPos, pkg, "Get", sig)
	}
	plain := types.NewSignatureType(nil, nil, nil, nil, nil, false)
	cases := []struct {
		name string
		fn   *types.Func
		want string
	}{
		{name: "pointer receiver", fn: method(types.NewPointer(named)), want: "handlers.Service.Get"},
		{name: "value receiver", fn: method(named), want: "handlers.Service.Get"},
		{name: "unnamed receiver", fn: method(types.NewStruct(nil, nil)), want: "handlers.Get"},
		{name: "function", fn: types.NewFunc(token.NoPos, pkg, "List", plain), want: "handlers.List"},
		{name: "no package", fn: types.NewFunc(token.NoPos, nil, "Loose", plain), want: "Loose"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := symbol(testCase.fn); got != testCase.want {
				t.Errorf("symbol() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestSiteName_NoFunction_IsEmpty verifies a leaf written in no function, as
// a declared request is, names no site.
func TestSiteName_NoFunction_IsEmpty(t *testing.T) {
	if got := siteName(nil); got != "" {
		t.Errorf("siteName(nil) = %q, want empty", got)
	}
}

// TestClassify_ReadsHowARequestIsNeeded verifies the three classes from the
// paths that make a request: on every path, on some, on none.
func TestClassify_ReadsHowARequestIsNeeded(t *testing.T) {
	paths := [][]int{{0, 1}, {0, 2}}
	cases := map[int]Class{0: ClassMandatory, 1: ClassAlternative, 3: ClassOptional}
	for index, want := range cases {
		t.Run(string(want), func(t *testing.T) {
			if got := classify(index, paths); got != want {
				t.Errorf("classify(%d) = %s, want %s", index, got, want)
			}
		})
	}
}
