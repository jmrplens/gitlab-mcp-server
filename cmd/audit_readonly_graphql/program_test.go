package main

import (
	"errors"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
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

// TestProgram_Reachable_RepeatedRoot_IsVisitedOnce verifies the closure stops
// at a function it has already reached. Nothing in the audit deduplicates the
// roots it is given, and a handler that calls a helper the closure also reaches
// puts the same function on the queue twice, so without this the walk would
// re-expand it and a cycle would never end.
func TestProgram_Reachable_RepeatedRoot_IsVisitedOnce(t *testing.T) {
	prog := loadFixture(t, vulnSources())
	send := lookupFunc(t, prog, "vuln", "send")
	dismissAll := lookupFunc(t, prog, "vuln", "dismissAll")

	reached := prog.reachable([]*types.Func{dismissAll, dismissAll, send})

	for _, want := range []*types.Func{dismissAll, send} {
		t.Run(want.Name(), func(t *testing.T) {
			if !reached[want] {
				t.Errorf("reachable from a repeated root does not contain %s", want.Name())
			}
		})
	}
}

// TestIsGraphQLSender_SharedToolutilExecutor_IsATransport verifies the package
// half of the transport check. The shared executors send the document their
// caller supplied, and they are ordinary functions rather than methods on a
// GraphQL service, so a check that only read receivers would miss every note
// domain and report the actions that reach them as touching no GraphQL at all.
func TestIsGraphQLSender_SharedToolutilExecutor_IsATransport(t *testing.T) {
	prog := loadFixture(t, mainSources())
	viaExecutor := prog.funcs[lookupFunc(t, prog, "shapes", "ViaExecutor")]

	if !viaExecutor.sendsGraphQL {
		t.Error("a handler calling the shared toolutil executor was not marked as sending GraphQL")
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

// TestLoadProgram_ADocumentNoDeclarationNames_IsLeftUnattributed verifies the
// tripwire the shared inventory made possible.
//
// The index resolves a document through the object that declares it, so a
// document assembled in a package-level initializer belongs to no object and
// sits in no function body: nothing in the reachability walk can ever reach it,
// and a mutation written that way would leave the gate reporting a clean run.
// It is recorded as unattributed instead, which is what the audit reports.
func TestLoadProgram_ADocumentNoDeclarationNames_IsLeftUnattributed(t *testing.T) {
	prog := loadFixture(t, map[string]string{"unplaced": unplacedFixture})

	if len(prog.unattributed) != 1 {
		t.Fatalf("loadProgram() left %d document(s) unattributed, want 1: %+v", len(prog.unattributed), prog.unattributed)
	}
	document := prog.unattributed[0]
	t.Run("it is the assembled document", func(t *testing.T) {
		if got := document.Label(); got != "an inline document" {
			t.Errorf("the unattributed document is labeled %q, want an inline one", got)
		}
		if !strings.Contains(document.Text, "thing { errors }") {
			t.Errorf("the unattributed document reads %q, want the assembled mutation", document.Text)
		}
		if !strings.HasSuffix(filepath.ToSlash(document.Position.Filename), "/unplaced/unplaced.go") {
			t.Errorf("the unattributed document is positioned at %q, want the fixture file", document.Position.Filename)
		}
	})
	t.Run("the audit reports it", func(t *testing.T) {
		result := audit(prog, nil, repoRoot(t))

		if len(result.findings) != 1 {
			t.Fatalf("audit() reported %d finding(s), want the unattributed document: %+v", len(result.findings), result.findings)
		}
		if !strings.Contains(result.findings[0].message, "unplaced/unplaced.go") {
			t.Errorf("the finding does not name the file:\n%s", result.findings[0].message)
		}
	})
}

// TestLoadProgram_AnInlineDocumentInAFunctionBody_IsAttributed verifies the
// other half of that rule, which is what keeps the tripwire from firing on a
// shape this audit does classify. An inline document written in a body is
// recorded by the body walk at the position of its literal, so the inventory
// entry at that position is placed and reported by nobody.
func TestLoadProgram_AnInlineDocumentInAFunctionBody_IsAttributed(t *testing.T) {
	prog := loadFixture(t, vulnSources())

	if len(prog.unattributed) != 0 {
		t.Errorf("loadProgram() left %+v unattributed, want nothing: the fixture's inline document is in a body",
			prog.unattributed)
	}
}

// TestLoadProgram_AStandaloneTreeItCannotRead_Fails verifies the run stops when
// the .graphql half of the inventory cannot be read. Continuing would audit the
// documents in Go source and silently none of the others, which is the silence
// reading them was added to remove.
func TestLoadProgram_AStandaloneTreeItCannotRead_Fails(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "internal"), []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}

	_, err := loadProgram(dir, []string{"./internal/..."}, nil)

	if err == nil {
		t.Fatal("loadProgram() error = nil, want the unreadable tree")
	}
	if !strings.Contains(err.Error(), "open ") {
		t.Errorf("loadProgram() error = %q, want it to name the tree it could not open", err)
	}
}

// standaloneModule writes a throwaway module holding one Go package and one
// standalone GraphQL document beside it, and returns its root.
//
// A real directory is needed rather than a loader overlay: the .graphql half of
// the inventory is read off disk by [graphqldocs.Standalone], which no overlay
// reaches. The module is minimal and imports nothing, so type-checking it costs
// no network and no dependency tree.
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

// TestLoadProgram_ADocumentInAFileOfItsOwn_IsReadAndLeftUnattributed verifies
// the .graphql half of the tripwire end to end.
//
// This is the shape the shared inventory was adopted for: a document that folds
// to nothing for the type checker, is bound to no object, and sits in no
// function body. Read but not indexed, it has to reach the report as an
// unattributed document; dropped from the inventory instead, a mutation in a
// file of its own would leave the gate printing a clean run. Nothing else in
// this package can see that, because an overlay never reaches the disk the
// standalone half reads.
func TestLoadProgram_ADocumentInAFileOfItsOwn_IsReadAndLeftUnattributed(t *testing.T) {
	dir := standaloneModule(t, "mutation($id: ID!) {\n  thing(input: {id: $id}) { errors }\n}\n")

	prog, err := loadProgram(dir, []string{"./internal/..."}, nil)
	if err != nil {
		t.Fatalf("loadProgram: %v", err)
	}

	if len(prog.unattributed) != 1 {
		t.Fatalf("loadProgram() left %d document(s) unattributed, want the .graphql file: %+v",
			len(prog.unattributed), prog.unattributed)
	}
	document := prog.unattributed[0]
	t.Run("it is the standalone document", func(t *testing.T) {
		if document.Name != "probe.graphql" {
			t.Errorf("the unattributed document is named %q, want probe.graphql", document.Name)
		}
		if document.Object != nil {
			t.Errorf("the unattributed document carries object %v, want none: no declaration names it", document.Object)
		}
		if !strings.HasSuffix(filepath.ToSlash(document.Position.Filename), "/internal/probe/probe.graphql") {
			t.Errorf("the unattributed document is positioned at %q, want the fixture file", document.Position.Filename)
		}
	})
	t.Run("the audit reports it", func(t *testing.T) {
		result := audit(prog, nil, dir)

		if len(result.findings) != 1 {
			t.Fatalf("audit() reported %d finding(s), want the standalone document: %+v", len(result.findings), result.findings)
		}
		if !strings.Contains(result.findings[0].message, "probe.graphql") {
			t.Errorf("the finding does not name the document:\n%s", result.findings[0].message)
		}
	})
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

// disagreeFixture writes the two shapes the shared pre-filter and this audit's
// own operation-type rule judge differently, one in each direction.
//
// prose carries a mutation on a line of its own but opens with something that
// is not an operation keyword, so the inventory does not consider it a document
// at all while classifyDocument's per-line regex would call it a mutation.
// send returns a literal the inventory does consider a document, the keyword
// being the first token, while classifyDocument refuses it because the keyword
// is not followed on its own line by a name, a brace or a paren.
const disagreeFixture = `package disagree

const prose = @@
Sent to GitLab:
mutation { thing { errors } }
@@

func send() string {
	return @@query
{ thing { id } }@@
}
`

// TestLoadProgram_ADocumentThePreFilterRefuses_IsNotIndexed pins the narrowing
// the shared inventory brings with it.
//
// The inventory asks for the operation keyword at the very start of the
// comment-stripped text, where this audit's own rule accepts it at the start of
// any line. A string that only satisfies the looser rule therefore leaves the
// inventory and is neither indexed nor reported. Every document this repository
// sends opens with its keyword, so nothing is lost today; the test is here so
// that the day the difference matters, it is a failing assertion rather than a
// gate that quietly stopped looking.
func TestLoadProgram_ADocumentThePreFilterRefuses_IsNotIndexed(t *testing.T) {
	prog := loadFixture(t, map[string]string{"disagree": disagreeFixture})

	if got := classifyDocument("\nSent to GitLab:\nmutation { thing { errors } }\n"); got != writeDocument {
		t.Fatalf("classifyDocument() = %v for the fixture's prose, want %v: the test would not be about the "+
			"narrowing if this audit's own rule refused it too", got, writeDocument)
	}
	for obj := range prog.documents {
		if obj.Pkg() != nil && obj.Pkg().Name() == "disagree" && obj.Name() == "prose" {
			t.Errorf("prose is indexed as a document, want it left out: the inventory does not read it as one")
		}
	}
	for _, document := range prog.unattributed {
		if strings.Contains(document.Text, "Sent to GitLab") {
			t.Errorf("prose is reported as unattributed, want it left out entirely: %+v", document)
		}
	}
}

// TestLoadProgram_ALiteralOnlyTheInventoryReadsAsADocument_IsReported pins the
// other direction, which is a false alarm rather than a silence.
//
// The inventory reads such a literal as a document; the body walk classifies it
// as none and so places nothing at its position, which leaves it unattributed
// and fails the run. That is the trade this gate makes everywhere else too: a
// reviewable finding over a string it never classified, since the alternative is
// a clean report over a document nobody judged.
func TestLoadProgram_ALiteralOnlyTheInventoryReadsAsADocument_IsReported(t *testing.T) {
	prog := loadFixture(t, map[string]string{"disagree": disagreeFixture})

	if len(prog.unattributed) != 1 {
		t.Fatalf("loadProgram() left %d document(s) unattributed, want the literal the two rules disagree about: %+v",
			len(prog.unattributed), prog.unattributed)
	}
	document := prog.unattributed[0]
	if !strings.Contains(document.Text, "thing { id }") {
		t.Errorf("the unattributed document reads %q, want the literal in send()", document.Text)
	}
	if got := classifyDocument(document.Text); got != notADocument {
		t.Errorf("classifyDocument() = %v for the literal, want %v: it is unattributed precisely because this "+
			"audit's own rule reads it as no document", got, notADocument)
	}
	if !strings.HasSuffix(filepath.ToSlash(document.Position.Filename), "/disagree/disagree.go") {
		t.Errorf("the unattributed document is positioned at %q, want the fixture file", document.Position.Filename)
	}
}

// unplacedFixture writes a document in the one shape no walk in this audit can
// place: assembled where it is used, in a package-level initializer, so it is
// bound to no object and sits in no function body. It is a package of its own
// because it fails every audit run that loads it, which is the point.
const unplacedFixture = `package unplaced

var sent = pass("mutation {" + " thing { errors } }")

func pass(document string) string {
	return document
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

// TestIndexFunctions_DeclarationBoundToNoFunction_IsSkipped verifies the
// function index only records declarations the type checker gave an object
// for. A declaration named with the blank identifier is bound to nothing, and
// indexing it under a nil object would put every such declaration in the same
// bucket of the call graph.
func TestIndexFunctions_DeclarationBoundToNoFunction_IsSkipped(t *testing.T) {
	prog := &program{funcs: map[*types.Func]*function{}}
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

// TestRecordLiteral_ConstantThatIsNotAString_IsNoDocument verifies the literal
// recorder judges the constant it was handed rather than the kind of node it
// arrived in. Only a document can be a document, and a value that is not a
// string cannot be one.
func TestRecordLiteral_ConstantThatIsNotAString_IsNoDocument(t *testing.T) {
	lit := &ast.BasicLit{Kind: token.INT, Value: "42"}
	info := synthInfo()
	info.Types[lit] = types.TypeAndValue{Value: constant.MakeInt64(42)}
	fn := &function{}

	(&program{}).recordLiteral(fn, &packages.Package{TypesInfo: info}, lit, map[token.Pos]bool{})

	if len(fn.docs) != 0 {
		t.Errorf("recordLiteral() recorded %d document(s) for a constant that is not a string", len(fn.docs))
	}
}

// errFixture is returned by the failing action source in the run tests.
var errFixture = errors.New("fixture catalog failure")
