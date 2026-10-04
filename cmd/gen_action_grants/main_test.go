package main

import (
	"bytes"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	gqlast "github.com/vektah/gqlparser/v2/ast"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/derive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests/actionfixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
)

// mainSource is the program a run is held to: one action that reads a
// project, and one for each gate the derivation refuses on (two lookups
// nothing qualifies, a request that may not run, and a mutation the record
// does not hold).
const mainSource = `package grantsmain

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

type Input struct {
	ID string @@json:"id"@@
}

type Output struct {
	OK bool @@json:"ok"@@
}

const thingCreate = @@mutation { thingCreate(title: "t") { errors } }@@

func get(ctx context.Context, client *gitlabclient.Client, id string) error {
	_, _, err := client.GL().Projects.GetProject(id, nil, gl.WithContext(ctx))
	return err
}

func Get(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, get(ctx, client, input.ID)
}

func Pair(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	if err := get(ctx, client, input.ID); err != nil {
		return Output{}, err
	}
	_, err := client.GL().Projects.DeleteProject(input.ID, nil, gl.WithContext(ctx))
	return Output{}, err
}

func Maybe(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	if input.ID != "" {
		return Output{}, get(ctx, client, input.ID)
	}
	return Output{}, nil
}

func Mutate(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{Query: thingCreate}, nil, gl.WithContext(ctx))
	return Output{}, err
}

func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		spec("get", toolutil.RouteAction(client, Get)),
		spec("pair", toolutil.RouteAction(client, Pair)),
		spec("maybe", toolutil.RouteAction(client, Maybe)),
		spec("mutate", toolutil.RouteAction(client, Mutate)),
	}
}

func spec(name string, route toolutil.ActionRoute) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, toolutil.ActionSpecOptions{Usage: "fixture"})
}
`

// mainSDL is the schema the fixture's mutation validates against.
const mainSDL = `
schema { query: Query mutation: Mutation }
type Query { version: String }
type Mutation { thingCreate(title: String): ThingPayload }
type ThingPayload { errors: [String!]! }
`

var (
	fixtureOnce sync.Once
	fixtureProg *actionrequests.Program
	fixtureRoot string
	errFixture  error
)

// moduleRoot is the repository the fixture program is loaded in.
func moduleRoot(t *testing.T) string {
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

// loadMain loads the fixture program once for the package.
func loadMain(t *testing.T) *actionrequests.Program {
	t.Helper()
	root := moduleRoot(t)
	fixtureOnce.Do(func() {
		fixtureRoot = root
		fixtureProg, errFixture = actionrequests.Load(root, []string{actionfixture.Pattern},
			actionfixture.Overlay(root, map[string]string{"grantsmain": mainSource}))
	})
	if errFixture != nil {
		t.Fatalf("load the fixture program: %v", errFixture)
	}
	return fixtureProg
}

// mainSDK answers the fixture's two client-go methods.
type mainSDK struct{}

// Requests answers the project read and delete, and nothing else.
func (mainSDK) Requests(method string) (routes, documents []derive.Request, known bool) {
	switch method {
	case "Projects.GetProject":
		return []derive.Request{{Kind: derive.KindREST, Method: "GET", Path: "/projects/:"}}, nil, true
	case "Projects.DeleteProject":
		return []derive.Request{{Kind: derive.KindREST, Method: "DELETE", Path: "/projects/:"}}, nil, true
	}
	return nil, nil, false
}

// mainRecord is the live record the fixture is joined to: the project read
// and delete, and no GraphQL mutation.
func mainRecord() *apilive.Document {
	assignable := func(name, display string) apilive.Assignable {
		return apilive.Assignable{
			Name: name, Display: display, Boundaries: []string{"project"}, Permissions: []string{name},
			AvailableFor: []string{"granular_access_token"},
		}
	}
	route := func(method, permission string) apilive.Route {
		return apilive.Route{Method: method, Path: "/api/:version/projects/:id", Authorization: &apilive.RouteAuthorization{
			Permissions: []string{permission}, BoundaryType: "project",
		}}
	}
	return &apilive.Document{
		Source: apilive.Source{Version: "19.4.1-ee"},
		Routes: []apilive.Route{route("GET", "read_project"), route("DELETE", "delete_project")},
		Granular: &apilive.Granular{
			Assignable: []apilive.Assignable{assignable("read_project", "Project: Read"), assignable("delete_project", "Project: Delete")},
			RawToAssignable: map[string]apilive.AssignableMatch{
				"read_project":   {First: "read_project", FirstAvailable: "read_project"},
				"delete_project": {First: "delete_project", FirstAvailable: "delete_project"},
			},
		},
		GraphQLAuthz: &apilive.GraphQLAuthz{Types: map[string]apilive.GraphQLType{}},
	}
}

// fixtureSources hands a run the fixture program, its client-go answers, the
// small record and schema, and no declarations, for the named actions.
func fixtureSources(t *testing.T, names ...string) sources {
	t.Helper()
	prog := loadMain(t)
	actions := make([]actionrequests.Action, len(names))
	for i, name := range names {
		actions[i] = actionrequests.Action{ID: "fixture." + name, Name: name, Owner: "grantsmain"}
	}
	return sources{
		catalog: func() ([]actionrequests.Action, error) { return actions, nil },
		load: func(string, map[string][]byte) (*actionrequests.Program, error) {
			return prog, nil
		},
		sdk:    func(*actionrequests.Program) (derive.Requester, error) { return mainSDK{}, nil },
		record: func(string) (*apilive.Document, error) { return mainRecord(), nil },
		schema: func() (*gqlast.Schema, error) { return graphqlschema.Load([]byte(mainSDL)) },
	}
}

// artifactPaths are the three files a run writes, under a root.
func artifactPaths(root string) []string {
	return []string{
		filepath.Join(root, requestsPath), filepath.Join(root, filepath.FromSlash(tablePath)), filepath.Join(root, referencePath),
	}
}

// TestRun_WritesTheArtifactsThenHoldsThemCurrent verifies a clean run writes
// the three artifacts, a check of them passes, and a check after one of them
// changed names it stale and the command that refreshes it.
func TestRun_WritesTheArtifactsThenHoldsThemCurrent(t *testing.T) {
	root := t.TempDir()
	in := fixtureSources(t, "get")
	var progress bytes.Buffer
	if err := run(&progress, root, options{}, in); err != nil {
		t.Fatalf("run = %v\n%s", err, progress.String())
	}
	if want := "1 actions derived, 1 rows at GitLab 19.4.1-ee: 0 denied to every fine-grained token, 0 served with parts always empty; " +
		"1 operations, 1 groups, 0 GraphQL elements, 0 element signatures read from the pinned schema\n"; progress.String() != want {
		t.Errorf("progress = %q, want %q", progress.String(), want)
	}
	for path, want := range map[string]string{
		artifactPaths(root)[0]: `"route": "GET /projects/:id"`,
		artifactPaths(root)[1]: `{ID: "fixture.get", Paths: [][]uint32{{0}}}`,
		artifactPaths(root)[2]: "| `fixture.get` | Project: Read at project |",
	} {
		t.Run(path, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil || !strings.Contains(string(data), want) {
				t.Errorf("%s holds no %q (%v):\n%s", path, want, err, data)
			}
		})
	}
	if err := run(&progress, root, options{check: true}, in); err != nil {
		t.Errorf("a check of what the run wrote = %v", err)
	}
	if err := os.WriteFile(artifactPaths(root)[2], []byte("edited\n"), 0o600); err != nil {
		t.Fatalf("edit the reference page: %v", err)
	}
	err := run(&progress, root, options{check: true}, in)
	if err == nil || !strings.Contains(err.Error(), "is stale; run "+regenerate) {
		t.Errorf("a check of an edited page = %v, want it named stale", err)
	}
}

// TestRun_CheckDerivation_WritesNothing verifies the derivation check derives
// and joins and leaves the tree as it was.
func TestRun_CheckDerivation_WritesNothing(t *testing.T) {
	root := t.TempDir()
	if err := run(&bytes.Buffer{}, root, options{checkDerivation: true}, fixtureSources(t, "get")); err != nil {
		t.Fatalf("run = %v", err)
	}
	for _, path := range artifactPaths(root) {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the derivation check left %s (%v)", path, err)
		}
	}
}

// TestRun_EachGate_RefusesTheArtifacts verifies the three gates, each on the
// shape it exists for, and that a run with a finding writes and compares
// nothing, in either mode: an artifact joined from an incomplete derivation
// would answer wrongly for the actions the findings name.
func TestRun_EachGate_RefusesTheArtifacts(t *testing.T) {
	in := fixtureSources(t, "get", "pair", "maybe", "mutate")
	cases := []struct {
		name string
		opts options
	}{
		{name: "write", opts: options{}},
		{name: "check", opts: options{check: true}},
		{name: "check derivation", opts: options{checkDerivation: true}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			var progress bytes.Buffer
			err := run(&progress, root, testCase.opts, in)
			if err == nil || !strings.Contains(err.Error(), "the derivation has 3 finding(s)") {
				t.Fatalf("run = %v, want three findings refused", err)
			}
			for _, want := range []string{
				"gate 1: fixture.pair sends GET /projects/: (from grantsmain.get) and DELETE /projects/: (from grantsmain.Pair) on one path",
				"gate 2: fixture.mutate is denied by thingCreate, which the live record does not hold as a graphql-mutation-undeclared",
				"gate 3: fixture.maybe can run sending none of its requests (GET /projects/: (from grantsmain.get) is never mandatory)",
			} {
				if !strings.Contains(progress.String(), want) {
					t.Errorf("progress holds no %q:\n%s", want, progress.String())
				}
			}
			for _, path := range artifactPaths(root) {
				if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
					t.Errorf("a refused run left %s", path)
				}
			}
		})
	}
}

// TestDeriveAndJoin_LoadsWithTheTableStandIn verifies the program is loaded
// with the generated table replaced by its stand-in, at the path the table is
// written to, so a table that no longer compiles cannot stop the run that
// would regenerate it.
func TestDeriveAndJoin_LoadsWithTheTableStandIn(t *testing.T) {
	in := fixtureSources(t, "get")
	load := in.load
	var overlay map[string][]byte
	in.load = func(root string, given map[string][]byte) (*actionrequests.Program, error) {
		overlay = given
		return load(root, given)
	}
	if _, err := deriveAndJoin("/repo", in); err != nil {
		t.Fatalf("deriveAndJoin = %v", err)
	}
	want := filepath.Join("/repo", filepath.FromSlash(tablePath))
	if len(overlay) != 1 || !bytes.Equal(overlay[want], tableStub) {
		t.Errorf("the overlay is %v, want the stand-in at %s", overlay, want)
	}
}

// TestTableStub_CompilesWhereABrokenTableDoesNot verifies the reason the
// stand-in exists: the table package with a table that does not compile stops
// a load, and the same package with the stand-in in its place loads.
func TestTableStub_CompilesWhereABrokenTableDoesNot(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, filepath.FromSlash(tablePath))
	patterns := []string{"./" + filepath.ToSlash(filepath.Dir(tablePath))}
	broken := []byte("package actiongrants\n\nvar table = noSuchValue\n")
	if _, err := actionrequests.Load(root, patterns, map[string][]byte{path: broken}); err == nil {
		t.Error("a table that does not compile loaded")
	}
	if _, err := actionrequests.Load(root, patterns, map[string][]byte{path: tableStub}); err != nil {
		t.Errorf("the stand-in does not load: %v", err)
	}
}

// TestDeriveAndJoin_NamesTheStageThatFailed verifies each input's failure is
// returned with the stage that read it.
func TestDeriveAndJoin_NamesTheStageThatFailed(t *testing.T) {
	failure := errors.New("refused")
	cases := []struct {
		name  string
		spoil func(*sources)
		want  string
	}{
		{name: "catalog", spoil: func(in *sources) {
			in.catalog = func() ([]actionrequests.Action, error) { return nil, failure }
		}, want: "build the catalog: refused"},
		{name: "program", spoil: func(in *sources) {
			in.load = func(string, map[string][]byte) (*actionrequests.Program, error) { return nil, failure }
		}, want: "load the program: refused"},
		{name: "client-go", spoil: func(in *sources) {
			in.sdk = func(*actionrequests.Program) (derive.Requester, error) { return nil, failure }
		}, want: "refused"},
		{name: "record", spoil: func(in *sources) {
			in.record = func(string) (*apilive.Document, error) { return nil, failure }
		}, want: "read the live record: refused"},
		{name: "schema", spoil: func(in *sources) {
			in.schema = func() (*gqlast.Schema, error) { return nil, failure }
		}, want: "load the pinned GraphQL schema: refused"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			in := fixtureSources(t, "get")
			testCase.spoil(&in)
			if _, err := deriveAndJoin(t.TempDir(), in); err == nil || err.Error() != testCase.want || !errors.Is(err, failure) {
				t.Errorf("deriveAndJoin = %v, want %q", err, testCase.want)
			}
			if err := run(&bytes.Buffer{}, t.TempDir(), options{}, in); !errors.Is(err, failure) {
				t.Errorf("run = %v, want the stage's failure", err)
			}
		})
	}
}

// TestLiveSources_ReadTheTreeWithThisCommandsDeclarations verifies the inputs
// a real run reads carry this command's declaration tables, and that the
// committed record is the one they read.
func TestLiveSources_ReadTheTreeWithThisCommandsDeclarations(t *testing.T) {
	in := liveSources()
	if len(in.requests) != len(requestDeclarations) || len(in.grants.Routes) != len(grantDeclarations.Routes) ||
		len(in.grants.Unresolvable) != len(grantDeclarations.Unresolvable) {
		t.Error("the live sources do not carry this command's declarations")
	}
	record, recordErr := in.record(moduleRoot(t))
	if recordErr != nil || record.Source.Version == "" {
		t.Errorf("the committed record reads as %v, %v", record, recordErr)
	}
	if _, missingErr := in.record(t.TempDir()); missingErr == nil {
		t.Error("a root with no record read one")
	}
	if schema, schemaErr := in.schema(); schemaErr != nil || schema == nil {
		t.Errorf("the pinned schema = %v, %v", schema, schemaErr)
	}
	// Loading the tree is the real run's business; here the loader only has
	// to read this command's patterns where it is pointed, and a directory
	// holding no module has nothing to read.
	if _, loadErr := in.load(t.TempDir(), nil); loadErr == nil {
		t.Error("a directory holding no module loaded")
	}
}

// TestReadSDK_ReadsTheClientGoTheProgramCompilesAgainst verifies client-go is
// read from the module the loaded program imports, and that a program
// importing no client-go is refused rather than read as one sending nothing.
func TestReadSDK_ReadsTheClientGoTheProgramCompilesAgainst(t *testing.T) {
	sdk, err := readSDK(loadMain(t))
	if err != nil {
		t.Fatalf("readSDK = %v", err)
	}
	routes, _, known := sdk.Requests("Projects.GetProject")
	if !known || !slices.ContainsFunc(routes, func(r derive.Request) bool { return r.Method == http.MethodGet && r.Path == "/projects/:" }) {
		t.Errorf("Projects.GetProject sends %v (known %t)", routes, known)
	}
	root := moduleRoot(t)
	empty, err := actionrequests.Load(root, []string{actionfixture.Pattern}, actionfixture.Overlay(root, map[string]string{"nosdk": "package nosdk\n"}))
	if err != nil {
		t.Fatalf("load a program with no client-go: %v", err)
	}
	if _, sdkErr := readSDK(empty); sdkErr == nil || !strings.Contains(sdkErr.Error(), "read client-go documents") {
		t.Errorf("readSDK on a program with no client-go = %v", sdkErr)
	}
}

// TestRunMain_Flags verifies the exit codes: 0 for help and for a run that
// passes, 2 for a flag it does not know, and 1 for a run that fails or a
// working directory outside any module.
func TestRunMain_Flags(t *testing.T) {
	clean := fixtureSources(t, "get")
	gated := fixtureSources(t, "pair")
	cases := []struct {
		name string
		args []string
		in   sources
		want int
	}{
		{name: "help", args: []string{"-h"}, in: clean, want: 0},
		{name: "unknown flag", args: []string{"-nope"}, in: clean, want: 2},
		{name: "derivation passes", args: []string{"-check-derivation"}, in: clean, want: 0},
		{name: "derivation fails", args: []string{"-check-derivation"}, in: gated, want: 1},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			restore := newSources
			newSources = func() sources { return testCase.in }
			t.Cleanup(func() { newSources = restore })
			if got := runMain(testCase.args, &bytes.Buffer{}); got != testCase.want {
				t.Errorf("runMain(%q) = %d, want %d", testCase.args, got, testCase.want)
			}
		})
	}
	t.Run("outside a module", func(t *testing.T) {
		t.Chdir(t.TempDir())
		var stderr bytes.Buffer
		if got := runMain(nil, &stderr); got != 1 || !strings.Contains(stderr.String(), "find repository root") {
			t.Errorf("runMain outside a module = %d, %q", got, stderr.String())
		}
	})
}

// TestMain_ExitsWithWhatRunMainReturns verifies the entry point hands the
// exit code to the process.
func TestMain_ExitsWithWhatRunMainReturns(t *testing.T) {
	restoreExit, restoreArgs := osExit, os.Args
	t.Cleanup(func() { osExit, os.Args = restoreExit, restoreArgs })
	code := -1
	osExit = func(c int) { code = c }
	os.Args = []string{toolName, "-h"}
	main()
	if code != 0 {
		t.Errorf("main exited %d, want 0", code)
	}
}
