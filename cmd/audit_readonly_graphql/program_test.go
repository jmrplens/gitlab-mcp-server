package main

import (
	"errors"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// fixtureDir is the directory the in-memory fixture packages pretend to live
// in. Nothing is written there: the packages exist only in the loader overlay,
// which keeps generated Go source out of the repository while still
// type-checking it against the real toolutil.
const fixtureDir = "cmd/audit_readonly_graphql/fixture"

// fixturePattern matches every fixture package at once.
const fixturePattern = "./" + fixtureDir + "/..."

// backtickPlaceholder stands in for a backtick inside a fixture source, which
// is itself written as a raw string literal and so cannot contain one.
const backtickPlaceholder = "@@"

// repoRoot walks up from the test's working directory to the module root, so
// the fixture overlay can name absolute paths inside the module and the loader
// resolves the module's own import paths.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s", dir)
		}
		dir = parent
	}
}

// fixtureOverlay turns a map of package name to source into a loader overlay
// rooted at the module. Each entry becomes one file in its own package
// directory under [fixtureDir].
func fixtureOverlay(t *testing.T, sources map[string]string) map[string][]byte {
	t.Helper()
	root := repoRoot(t)
	overlay := make(map[string][]byte, len(sources))
	for name, source := range sources {
		path := filepath.Join(root, filepath.FromSlash(fixtureDir), name, name+".go")
		overlay[path] = []byte(strings.ReplaceAll(source, backtickPlaceholder, "`"))
	}
	return overlay
}

// fixtureCache memoizes one loaded program per fixture source set. Loading is
// a full type-check of the fixture against toolutil and takes seconds, the
// result is read-only for everything the tests ask of it, and the tests run in
// one goroutine, so paying for it once per source set keeps the package's
// tests from spending a minute re-parsing the same few packages.
var fixtureCache = map[string]*program{}

// loadFixture loads the fixture packages described by sources.
func loadFixture(t *testing.T, sources map[string]string) *program {
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
	prog, err := loadProgram(repoRoot(t), []string{fixturePattern}, fixtureOverlay(t, sources))
	if err != nil {
		t.Fatalf("loadProgram: %v", err)
	}
	fixtureCache[key] = prog
	return prog
}

// vulnFixture is the fixture the audit tests share: one package holding a read
// document and a mutation document, handlers for each, a handler that reaches
// the mutation only through a callee, and action specs that classify some of
// them honestly and one of them wrongly.
//
// It is written the way a real domain is written, including the
// package-local spec helper that forwards to toolutil, because the resolver's
// whole job is to follow that forwarding.
const vulnFixture = `package vuln

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	ID string ` + "`json:\"id\"`" + `
}

// Output is the shared output for every fixture handler.
type Output struct {
	OK bool ` + "`json:\"ok\"`" + `
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
		Data map[string]any ` + "`json:\"data\"`" + `
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

// vulnSources is the fixture package set the audit tests load.
func vulnSources() map[string]string {
	return map[string]string{"vuln": vulnFixture}
}

// vulnActions is the catalog the audit tests hand to [audit] for the vuln
// fixture: the same names the fixture declares, classified the same way.
func vulnActions() []action {
	return []action{
		{ID: "vuln.list", Name: "list", Owner: "vuln", ReadOnly: true},
		{ID: "vuln.dismiss", Name: "dismiss", Owner: "vuln", ReadOnly: false},
		{ID: "vuln.read_dismiss", Name: "read_dismiss", Owner: "vuln", ReadOnly: true},
		{ID: "vuln.read_dismiss_indirect", Name: "read_dismiss_indirect", Owner: "vuln", ReadOnly: true},
		{ID: "vuln.read_inline", Name: "read_inline", Owner: "vuln", ReadOnly: true},
	}
}

// lookupFunc finds a declared function in the loaded program by package name
// and function name.
func lookupFunc(t *testing.T, prog *program, pkgName, funcName string) *types.Func {
	t.Helper()
	for fn := range prog.funcs {
		if fn.Pkg() != nil && fn.Pkg().Name() == pkgName && fn.Name() == funcName {
			return fn
		}
	}
	t.Fatalf("function %s.%s not found in loaded program", pkgName, funcName)
	return nil
}

// TestLoadProgram_FixturePackages_IndexesDocumentsAndFunctions verifies the
// loader indexes an overlay-only package: its functions get bodies, its
// GraphQL constants get classified, and the function that calls the GraphQL
// transport is marked as sending.
func TestLoadProgram_FixturePackages_IndexesDocumentsAndFunctions(t *testing.T) {
	prog := loadFixture(t, vulnSources())

	send := lookupFunc(t, prog, "vuln", "send")
	if !prog.funcs[send].sendsGraphQL {
		t.Error("send() calls GraphQL.Do but was not marked as sending GraphQL")
	}

	kinds := map[string]documentKind{}
	for obj, kind := range prog.documents {
		if obj.Pkg() != nil && obj.Pkg().Name() == "vuln" {
			kinds[obj.Name()] = kind
		}
	}
	if got := kinds["listQuery"]; got != readDocument {
		t.Errorf("listQuery classified %v, want %v", got, readDocument)
	}
	if got := kinds["dismissMutation"]; got != writeDocument {
		t.Errorf("dismissMutation classified %v, want %v", got, writeDocument)
	}
}

// TestLoadProgram_NoMatchingPattern_ReturnsError verifies that a pattern
// matching nothing is an error rather than an empty, silently passing audit.
// A testdata directory is the pattern that matches nothing without the
// toolchain calling it an error: the go command skips testdata, so the
// directory exists and the pattern still resolves to no packages.
func TestLoadProgram_NoMatchingPattern_ReturnsError(t *testing.T) {
	_, err := loadProgram(repoRoot(t), []string{"./internal/tools/testdata/..."}, nil)
	if err == nil {
		t.Fatal("loading a pattern that matches nothing must fail")
	}
	if !strings.Contains(err.Error(), "no packages matched") {
		t.Errorf("error %q does not say the pattern matched nothing", err)
	}
}

// TestLoadProgram_BrokenFixture_ReturnsLoadError verifies a package that does
// not compile is reported rather than treated as a package with no handlers,
// which would make every action in it silently unclassifiable.
func TestLoadProgram_BrokenFixture_ReturnsLoadError(t *testing.T) {
	_, err := loadProgram(repoRoot(t), []string{fixturePattern}, fixtureOverlay(t, map[string]string{
		"broken": "package broken\n\nfunc Broken() int { return \"not an int\" }\n",
	}))
	if err == nil {
		t.Fatal("a fixture that does not type-check must fail the load")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("error %q does not name the package that failed", err)
	}
}

// TestPackageLoadError_NoErrors_ReturnsNil verifies the happy path of the
// per-package error check, which the loader relies on to pass clean packages
// through untouched.
func TestPackageLoadError_NoErrors_ReturnsNil(t *testing.T) {
	prog := loadFixture(t, vulnSources())
	for _, pkg := range prog.order {
		if err := packageLoadError(pkg); err != nil {
			t.Errorf("packageLoadError(%s) = %v, want nil", pkg.PkgPath, err)
		}
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

	reached := prog.reachable([]*types.Func{indirect})
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
	if reached := prog.reachable(nil); len(reached) != 0 {
		t.Errorf("reachable(nil) returned %d functions, want 0", len(reached))
	}
}

// TestIsGraphQLSender_Classification verifies the transport check accepts the
// client-go GraphQL service method and the shared toolutil executors, and
// rejects a method named Do on something else.
func TestIsGraphQLSender_Classification(t *testing.T) {
	prog := loadFixture(t, vulnSources())
	send := prog.funcs[lookupFunc(t, prog, "vuln", "send")]

	var graphQLDo, other *types.Func
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
	other = lookupFunc(t, prog, "vuln", "List")
	if isGraphQLSender(other) {
		t.Error("an ordinary handler was wrongly recognized as a GraphQL sender")
	}
}

// TestLoadProgram_PackageLevelVariables_IndexesOnlyConstantDocuments verifies
// the two shapes a document can be written in as a variable: one initialized
// from a constant string, which is indexed, and one initialized from a call,
// whose value is not knowable from the source and so is not.
func TestLoadProgram_PackageLevelVariables_IndexesOnlyConstantDocuments(t *testing.T) {
	prog := loadFixture(t, map[string]string{"vars": varFixture})

	indexed := map[string]documentKind{}
	for obj, kind := range prog.documents {
		if obj.Pkg() != nil && obj.Pkg().Name() == "vars" {
			indexed[obj.Name()] = kind
		}
	}
	if got, ok := indexed["declaredMutation"]; !ok || got != writeDocument {
		t.Errorf("declaredMutation indexed as %v (present=%t), want %v", got, ok, writeDocument)
	}
	if _, ok := indexed["computedMutation"]; ok {
		t.Error("a variable initialized from a call must not be indexed as a document")
	}
	if _, ok := indexed["undeclared"]; ok {
		t.Error("a variable with no initializer must not be indexed as a document")
	}
}

// varFixture declares GraphQL documents as package-level variables rather than
// constants, plus the two variable shapes that carry no knowable value.
const varFixture = `package vars

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

// TestConstantString_NonString_ReturnsFalse verifies the constant unwrapper
// refuses values that are not strings, which is what keeps numeric and boolean
// constants out of the document index.
func TestConstantString_NonString_ReturnsFalse(t *testing.T) {
	if _, ok := constantString(nil); ok {
		t.Error("a nil constant value must not unwrap to a string")
	}
}

// errFixture is returned by the failing action source in the run tests.
var errFixture = errors.New("fixture catalog failure")
