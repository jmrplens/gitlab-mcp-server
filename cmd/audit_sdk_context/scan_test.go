package main

import (
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// fixtureDir is the directory the in-memory fixture package pretends to live
// in. Nothing is written there: the package exists only in the loader overlay,
// so the walk is exercised on real type-checked source, against the real
// client-go, without generated Go files landing in the repository.
const fixtureDir = "cmd/audit_sdk_context/fixture"

// fixtureHeader opens every fixture file: the package, and client-go under
// the alias this repository imports it by. The retryablehttp and context
// imports are used by the request-builder cases and blanked for the rest, so
// one header serves every case.
const fixtureHeader = `package fixture

import (
	"context"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

var (
	_ = context.Background
	_ *retryablehttp.Request
	_ = gl.WithContext
)
`

// repoRoot walks up from the test's working directory to the module root, so
// the overlay can name absolute paths inside the module and the loader
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

// fixtureOverlay places the named files in the fixture directory.
func fixtureOverlay(root string, files map[string]string) map[string][]byte {
	overlay := map[string][]byte{}
	for name, source := range files {
		overlay[filepath.Join(root, filepath.FromSlash(fixtureDir), name)] = []byte(source)
	}
	return overlay
}

// auditFixture runs the whole audit over the named fixture files, held to the
// given declarations, and returns the report.
func auditFixture(t *testing.T, files map[string]string, declared map[string]declaration) Report {
	t.Helper()
	root := repoRoot(t)
	report, err := audit(auditConfig{
		dir:      root,
		patterns: []string{"./" + fixtureDir},
		overlay:  fixtureOverlay(root, files),
		declared: declared,
	})
	if err != nil {
		t.Fatalf("audit fixture: %v", err)
	}
	return report
}

// scanBody audits one fixture file made of the shared header and body, with
// no declarations.
func scanBody(t *testing.T, body string) Report {
	t.Helper()
	return auditFixture(t, map[string]string{"fixture.go": fixtureHeader + body}, nil)
}

// spelled renders findings as `Func: Callee reason`, sorted, which is what
// most cases hold: which declaration, which call, and why.
func spelled(findings []Finding) []string {
	out := make([]string, 0, len(findings))
	for _, finding := range findings {
		out = append(out, finding.Func+": "+finding.Callee+" "+finding.Reason)
	}
	slices.Sort(out)
	return out
}

// assertFindings compares the findings of a report with the spelled ones a
// case expects.
func assertFindings(t *testing.T, report Report, want ...string) {
	t.Helper()
	got := spelled(report.Findings)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("findings =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// TestScan_ServiceCallWithoutTheOption_IsReported is the defect the gate
// exists for: a service call handed no request option at all, which client-go
// answers by building its request from context.Background(). The record is
// held whole, since every field of it is what a reader uses to find the call.
func TestScan_ServiceCallWithoutTheOption_IsReported(t *testing.T) {
	report := scanBody(t, `
func Get(c *gl.Client) {
	_, _, _ = c.Projects.GetProject(1, nil)
}
`)
	want := []Finding{{
		Package: fixtureDir,
		File:    fixtureDir + "/fixture.go",
		Line:    17,
		Func:    "Get",
		Callee:  "c.Projects.GetProject",
		Reason:  reasonMissing,
	}}
	if !slices.Equal(report.Findings, want) {
		t.Fatalf("findings = %+v, want %+v", report.Findings, want)
	}
	if report.Summary.Calls != 1 || report.Summary.Packages != 1 {
		t.Fatalf("summary = %+v, want one call in one package", report.Summary)
	}
}

// TestScan_TheOptionUnderEitherImportName_IsClean: client-go is imported as
// `gl` in most packages and as `gitlab` in some, and the option is resolved
// through the type checker, so both spellings are the same function.
func TestScan_TheOptionUnderEitherImportName_IsClean(t *testing.T) {
	report := auditFixture(t, map[string]string{
		"gl.go": fixtureHeader + `
func Get(ctx context.Context, c *gl.Client) {
	_, _, _ = c.Projects.GetProject(1, nil, gl.WithContext(ctx))
}
`,
		"gitlab.go": `package fixture

import (
	"context"

	gitlab "gitlab.com/gitlab-org/api/client-go/v3"
)

func Version(ctx context.Context, c *gitlab.Client) {
	_, _, _ = c.Version.GetVersion(gitlab.WithContext(ctx))
}
`,
	}, nil)
	assertFindings(t, report)
	if report.Summary.Calls != 2 {
		t.Fatalf("calls = %d, want both judged", report.Summary.Calls)
	}
}

// TestScan_ForwardingClosure_IsCleanAndItsCallerIsJudged is the shape the
// list helpers here use: a closure that hands its own options straight to the
// service method, called by a helper that is where the context has to be
// added. The closure is clean by forwarding, and each call of it is judged by
// the rule the service call would have been.
func TestScan_ForwardingClosure_IsCleanAndItsCallerIsJudged(t *testing.T) {
	report := scanBody(t, `
func list(ctx context.Context, fetch func(...gl.RequestOptionFunc) error) {
	_ = fetch(gl.WithContext(ctx))
	_ = fetch()
}

func Runners(ctx context.Context, c *gl.Client) {
	list(ctx, func(opts ...gl.RequestOptionFunc) error {
		_, _, err := c.Runners.ListRunners(nil, opts...)
		return err
	})
}
`)
	assertFindings(t, report, "list: fetch "+reasonMissing)
	if report.Summary.Forwarded != 1 || report.Summary.Calls != 3 {
		t.Fatalf("summary = %+v, want three calls, one forwarded", report.Summary)
	}
}

// TestScan_ForwardingAParameterOfAnOuterFunction_IsClean: forwarding is
// decided by the parameter's identity, so a closure handing on the options of
// the function around it forwards them as much as that function would.
func TestScan_ForwardingAParameterOfAnOuterFunction_IsClean(t *testing.T) {
	report := scanBody(t, `
func Outer(c *gl.Client, opts ...gl.RequestOptionFunc) func() {
	return func() {
		_, _, _ = c.Version.GetVersion(opts...)
	}
}
`)
	assertFindings(t, report)
	if report.Summary.Forwarded != 1 {
		t.Fatalf("forwarded = %d, want 1", report.Summary.Forwarded)
	}
}

// TestScan_ForwardingASliceParameter_IsCleanAndItsCallerIsJudged: a plain
// `[]RequestOptionFunc` parameter forwards like a variadic one, and a call of
// a function taking one is judged on the slice it is handed, which is how a
// helper taking its options as a slice cannot hide a caller that passed none.
func TestScan_ForwardingASliceParameter_IsCleanAndItsCallerIsJudged(t *testing.T) {
	report := scanBody(t, `
func version(c *gl.Client, opts []gl.RequestOptionFunc) {
	_, _, _ = c.Version.GetVersion(opts...)
}

func Callers(ctx context.Context, c *gl.Client) {
	version(c, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	version(c, nil)
}
`)
	assertFindings(t, report, "Callers: version "+reasonMissing)
}

// TestScan_MethodValues_AreJudgedByTheirSignature: a service method stored in
// a variable or a struct field is called through a function value, which a
// text scan cannot tie to client-go and the type checker's signature can.
func TestScan_MethodValues_AreJudgedByTheirSignature(t *testing.T) {
	report := scanBody(t, `
type searcher struct {
	search func(string, *gl.SearchOptions, ...gl.RequestOptionFunc) ([]*gl.Project, *gl.Response, error)
}

type lister func(opts ...gl.RequestOptionFunc) error

func Values(ctx context.Context, c *gl.Client, list lister) {
	get := c.Version.GetVersion
	_, _, _ = get()
	_, _, _ = get(gl.WithContext(ctx))
	s := searcher{search: c.Search.Projects}
	_, _, _ = s.search("q", nil)
	_ = list(gl.WithContext(ctx))
}
`)
	assertFindings(t, report, "Values: get "+reasonMissing, "Values: s.search "+reasonMissing)
}

// TestScan_AnOptionSliceBeforeOtherParameters_IsJudgedOnItsOwnArgument: a
// callee may take its options as a slice that is not its last parameter, in a
// function that is variadic in something else or not at all, and the call is
// judged on that slice's argument alone. An option handed to a parameter that
// is a single RequestOptionFunc rather than a slice is not where the call's
// options are read, so it does not pass the call; the conservative answer,
// since nothing says the callee sends it.
func TestScan_AnOptionSliceBeforeOtherParameters_IsJudgedOnItsOwnArgument(t *testing.T) {
	report := scanBody(t, `
func helper(opts []gl.RequestOptionFunc, names ...string) {}

func single(opts []gl.RequestOptionFunc, extra gl.RequestOptionFunc) {}

func mixed(opts []gl.RequestOptionFunc, extra gl.RequestOptionFunc, names ...string) {}

func Caller(ctx context.Context) {
	helper(nil, "a")
	helper([]gl.RequestOptionFunc{gl.WithContext(ctx)}, "a", "b")
	single(nil, gl.WithContext(ctx))
	mixed(nil, gl.WithContext(ctx), "a")
}
`)
	assertFindings(t, report,
		"Caller: helper "+reasonMissing,
		"Caller: single "+reasonMissing,
		"Caller: mixed "+reasonMissing)
}

// TestScan_WhatOnlyLooksLikeTheRules_IsNotMistakenForThem holds the checks
// that keep a look-alike from passing or failing a call: a WithContext method
// of another type is no rebinding, a rebinding of a request reached through a
// call is not tied to the variable it came from, and a context produced by a
// function value or by a helper of this package is taken as one that can end,
// since only the direct context.Background() and context.TODO() are
// recognized. A client-go type other than the option is not an option either.
func TestScan_WhatOnlyLooksLikeTheRules_IsNotMistakenForThem(t *testing.T) {
	report := scanBody(t, `
type scoped struct{}

func (s scoped) WithContext(ctx context.Context) scoped { return s }

func passThrough(r *retryablehttp.Request) *retryablehttp.Request { return r }

func background() context.Context { return context.Background() }

func LookAlikes(ctx context.Context, c *gl.Client, pick func() context.Context) {
	s := scoped{}
	s = s.WithContext(ctx)
	_ = s

	req, _ := c.NewRequest("GET", "version", nil, nil)
	_, _ = c.Do(passThrough(req).WithContext(ctx), nil)

	_, _, _ = c.Version.GetVersion(gl.WithContext(pick()))
	_, _, _ = c.Version.GetVersion(gl.WithContext(background()))

	var kind gl.UploadType = gl.UploadFile
	_ = kind
}
`)
	assertFindings(t, report, "LookAlikes: c.NewRequest "+reasonUnbound)
}

// TestObserveFile_ADeclarationThatIsNeitherAFunctionNorAVariable_IsSkipped: a
// file the parser could not make sense of in places carries a BadDecl, which
// holds no call and names nothing, and the walk passes over it.
func TestObserveFile_ADeclarationThatIsNeitherAFunctionNorAVariable_IsSkipped(t *testing.T) {
	found := newScanner(t.TempDir(), nil)
	found.observeFile(&packages.Package{}, "p", &facts{}, &ast.File{Decls: []ast.Decl{&ast.BadDecl{}}})
	if len(found.findings) != 0 || found.calls != 0 {
		t.Fatalf("findings = %v, calls = %d, want nothing", found.findings, found.calls)
	}
}

// TestScan_GraphQLWithoutTheOption_IsReported: the GraphQL client takes its
// context the same way the REST services do.
func TestScan_GraphQLWithoutTheOption_IsReported(t *testing.T) {
	report := scanBody(t, `
func Query(ctx context.Context, c *gl.Client) {
	_, _ = c.GraphQL.Do(gl.GraphQLQuery{Query: "query { currentUser { id } }"}, nil)
	_, _ = c.GraphQL.Do(gl.GraphQLQuery{Query: "query { currentUser { id } }"}, nil, gl.WithContext(ctx))
}
`)
	assertFindings(t, report, "Query: c.GraphQL.Do "+reasonMissing)
}

// TestScan_RequestBuilders holds the rules for a request built by hand, which
// takes its options as a slice and can be given the context afterwards
// through the request's own WithContext method. The rebinding counts only
// where it reaches the send: assigned back to the request, or handed straight
// to a call.
func TestScan_RequestBuilders(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		want     []string
		rebounds int
	}{
		{name: "nil options and no rebinding", body: `
func Build(c *gl.Client) {
	req, _ := c.NewRequest("GET", "version", nil, nil)
	_, _ = c.Do(req, nil)
}
`, want: []string{"Build: c.NewRequest " + reasonUnbound}},
		{name: "the option in the literal", body: `
func Build(ctx context.Context, c *gl.Client) {
	req, _ := c.NewRequest("GET", "version", nil, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	_, _ = c.Do(req, nil)
}
`},
		{name: "rebound to itself", body: `
func Build(ctx context.Context, c *gl.Client) {
	req, _ := c.NewRequest("GET", "version", nil, nil)
	req = req.WithContext(ctx)
	_, _ = c.Do(req, nil)
}
`, rebounds: 1},
		{name: "rebound as the argument that sends it", body: `
func Build(ctx context.Context, c *gl.Client) {
	req, _ := c.NewRequest("GET", "version", nil, nil)
	_, _ = c.Do(req.WithContext(ctx), nil)
}
`, rebounds: 1},
		{name: "rebound and discarded", body: `
func Build(ctx context.Context, c *gl.Client) {
	req, _ := c.NewRequest("GET", "version", nil, nil)
	_ = req.WithContext(ctx)
	req.WithContext(ctx)
	_, _ = c.Do(req, nil)
}
`, want: []string{"Build: c.NewRequest " + reasonUnbound}},
		{name: "rebound into another variable", body: `
func Build(ctx context.Context, c *gl.Client) {
	req, _ := c.NewRequest("GET", "version", nil, nil)
	bound := req.WithContext(ctx)
	_, _ = c.Do(bound, nil)
}
`, want: []string{"Build: c.NewRequest " + reasonUnbound}},
		{name: "rebound to a context that never ends", body: `
func Build(c *gl.Client) {
	req, _ := c.NewRequest("GET", "version", nil, nil)
	req = req.WithContext(context.Background())
	_, _ = c.Do(req, nil)
}
`, want: []string{"Build: c.NewRequest " + reasonUnbound}},
		{name: "a result nobody names", body: `
func Build(c *gl.Client) {
	_, _ = c.NewRequest("GET", "version", nil, nil)
}
`, want: []string{"Build: c.NewRequest " + reasonUnbound}},
		{name: "the other two builders", body: `
func Build(c *gl.Client) {
	_, _ = c.NewRequestToURL("GET", nil, nil, nil)
	_, _ = c.UploadRequest("POST", "uploads", nil, "f", gl.UploadFile, nil, nil)
}
`, want: []string{"Build: c.NewRequestToURL " + reasonUnbound, "Build: c.UploadRequest " + reasonUnbound}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := scanBody(t, tt.body)
			assertFindings(t, report, tt.want...)
			if report.Summary.Rebound != tt.rebounds {
				t.Fatalf("rebound = %d, want %d", report.Summary.Rebound, tt.rebounds)
			}
		})
	}
}

// TestScan_AContextThatNeverEnds_CountsAsNone: the option compiles and reads
// like the fix while bounding nothing, so it is reported under a reason of its
// own. A variable holding one is followed no further, which is the documented
// limit: a command's own context is usually derived from context.Background()
// by a timeout before it is used.
func TestScan_AContextThatNeverEnds_CountsAsNone(t *testing.T) {
	report := scanBody(t, `
func Detached(c *gl.Client) {
	_, _, _ = c.Version.GetVersion(gl.WithContext(context.Background()))
	_, _, _ = c.Version.GetVersion(gl.WithContext((context.TODO())))
	ctx := context.Background()
	_, _, _ = c.Version.GetVersion(gl.WithContext(ctx))
	_, _, _ = c.Version.GetVersion(gl.WithContext(context.WithoutCancel(ctx)))
}
`)
	assertFindings(t, report,
		"Detached: c.Version.GetVersion "+reasonDetached,
		"Detached: c.Version.GetVersion "+reasonDetached)
}

// TestScan_AContextThatNeverEnds_IsReportedBesideTheCallersContext: client-go
// applies a call's options in order and a later WithContext replaces an
// earlier one, so a detached option placed after the caller's context is the
// one the request ends up with, and one placed before it does nothing while
// reading as though it did. Both are reported whatever else the call passes,
// and so is one appended to options that were forwarded, which a forwarding
// caller could not undo. The caller's context beside a forwarded slice or
// another option keeps the call clean.
func TestScan_AContextThatNeverEnds_IsReportedBesideTheCallersContext(t *testing.T) {
	report := scanBody(t, `
func After(ctx context.Context, c *gl.Client) {
	_, _, _ = c.Version.GetVersion(gl.WithContext(ctx), gl.WithContext(context.Background()))
}

func Before(ctx context.Context, c *gl.Client) {
	_, _, _ = c.Version.GetVersion(gl.WithContext(context.TODO()), gl.WithContext(ctx))
}

func InALiteral(ctx context.Context, c *gl.Client) {
	_, _, _ = c.Version.GetVersion([]gl.RequestOptionFunc{gl.WithContext(ctx), gl.WithContext(context.Background())}...)
}

func Appended(c *gl.Client, opts ...gl.RequestOptionFunc) {
	_, _, _ = c.Version.GetVersion(append(opts, gl.WithContext(context.Background()))...)
}

func Clean(ctx context.Context, c *gl.Client, opts ...gl.RequestOptionFunc) {
	_, _, _ = c.Version.GetVersion(gl.WithContext(ctx), gl.WithHeader("X", "y"))
	_, _, _ = c.Version.GetVersion([]gl.RequestOptionFunc{gl.WithContext(ctx), gl.WithHeader("X", "y")}...)
	_, _, _ = c.Version.GetVersion(append([]gl.RequestOptionFunc{gl.WithContext(ctx)}, gl.WithHeader("X", "y"))...)
	_, _, _ = c.Version.GetVersion(append(opts, gl.WithContext(ctx))...)
}
`)
	assertFindings(t, report,
		"After: c.Version.GetVersion "+reasonDetached,
		"Before: c.Version.GetVersion "+reasonDetached,
		"InALiteral: c.Version.GetVersion "+reasonDetached,
		"Appended: c.Version.GetVersion "+reasonDetached)
	if report.Summary.Forwarded != 0 || report.Summary.Calls != 8 {
		t.Fatalf("summary = %+v, want eight calls and none resting on forwarding", report.Summary)
	}
}

// TestScan_TheRulesDoNotReadControlFlow pins the limit the command states: a
// variable counts as carrying the context if any assignment of it does, a
// parameter reassigned before the call still counts as forwarded, and a
// rebinding counts wherever it sits, after the send included. Each of these
// passes a call that does not carry the context, and none exists in the tree;
// the day the rules are made to read order, this is the test that says so.
func TestScan_TheRulesDoNotReadControlFlow(t *testing.T) {
	report := scanBody(t, `
func Dropped(ctx context.Context, c *gl.Client) {
	opts := []gl.RequestOptionFunc{gl.WithContext(ctx)}
	opts = nil
	_, _, _ = c.Version.GetVersion(opts...)
}

func Reassigned(c *gl.Client, opts ...gl.RequestOptionFunc) {
	opts = []gl.RequestOptionFunc{gl.WithHeader("X", "y")}
	_, _, _ = c.Version.GetVersion(opts...)
}

func ReboundAfterTheSend(ctx context.Context, c *gl.Client) {
	req, _ := c.NewRequest("GET", "version", nil, nil)
	_, _ = c.Do(req, nil)
	req = req.WithContext(ctx)
	_ = req
}
`)
	assertFindings(t, report)
	if report.Summary.Forwarded != 1 || report.Summary.Rebound != 1 || report.Summary.Calls != 3 {
		t.Fatalf("summary = %+v, want three calls, one forwarded and one rebound", report.Summary)
	}
}

// TestScan_RetryablehttpConstructors holds the requests client-go did not
// build: go-retryablehttp's own constructors, whose request (*gl.Client).Do
// sends as it is. NewRequest builds from context.Background() and passes only
// when rebound; NewRequestWithContext passes on a context that can end and is
// held to the rebinding rule on one that never does. Its other functions, a
// method of the request and a constructor of the same name in another package
// are not judged.
func TestScan_RetryablehttpConstructors(t *testing.T) {
	report := auditFixture(t, map[string]string{"fixture.go": `package fixture

import (
	"context"
	"net/http"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

func Unbound(c *gl.Client) {
	req, _ := retryablehttp.NewRequest("GET", "https://example.com", nil)
	_, _ = c.Do(req, nil)
}

func Rebound(ctx context.Context, c *gl.Client) {
	req, _ := retryablehttp.NewRequest("GET", "https://example.com", nil)
	req = req.WithContext(ctx)
	_, _ = c.Do(req, nil)
}

func Carried(ctx context.Context, c *gl.Client) {
	req, _ := retryablehttp.NewRequestWithContext(ctx, "GET", "https://example.com", nil)
	_, _ = c.Do(req, nil)
}

func Detached(c *gl.Client) {
	req, _ := retryablehttp.NewRequestWithContext(context.Background(), "GET", "https://example.com", nil)
	_, _ = c.Do(req, nil)
}

func DetachedThenRebound(ctx context.Context, c *gl.Client) {
	req, _ := retryablehttp.NewRequestWithContext(context.TODO(), "GET", "https://example.com", nil)
	_, _ = c.Do(req.WithContext(ctx), nil)
}

func NotJudged(ctx context.Context, c *gl.Client) {
	_ = retryablehttp.NewClient()
	plain, _ := http.NewRequestWithContext(ctx, "GET", "https://example.com", nil)
	wrapped, _ := retryablehttp.FromRequest(plain)
	_, _ = wrapped.BodyBytes()
	_, _ = c.Do(wrapped, nil)
}
`}, nil)
	assertFindings(t, report,
		"Unbound: retryablehttp.NewRequest "+reasonUnbound,
		"Detached: retryablehttp.NewRequestWithContext "+reasonUnbound)
	if report.Summary.Calls != 5 || report.Summary.Rebound != 2 {
		t.Fatalf("summary = %+v, want five constructor calls, two rebound", report.Summary)
	}
}

// TestScan_OptionVariables holds the options kept in a variable: a slice
// initialized with the context and appended to on a branch, which is the
// natural way to add pagination conditionally, a slice given the context by
// a later append, and a single option held on its own. A variable that never
// gets one is reported, including one appended to from itself.
func TestScan_OptionVariables(t *testing.T) {
	report := scanBody(t, `
func Variables(ctx context.Context, c *gl.Client, cursor string) {
	opts := []gl.RequestOptionFunc{gl.WithContext(ctx)}
	if cursor != "" {
		opts = append(opts, gl.WithKeysetPaginationParameters(cursor))
	}
	_, _, _ = c.Version.GetVersion(opts...)

	var later []gl.RequestOptionFunc
	later = append(later, gl.WithContext(ctx))
	_, _, _ = c.Version.GetVersion(later...)

	single := gl.WithContext(ctx)
	_, _, _ = c.Version.GetVersion(single)

	paired, count := []gl.RequestOptionFunc{gl.WithContext(ctx)}, 1
	_, _, _ = c.Version.GetVersion(paired...)
	_ = count

	var none []gl.RequestOptionFunc
	none = append(none, gl.WithHeader("X", "y"))
	_, _, _ = c.Version.GetVersion(none...)

	var (
		detached = gl.WithContext(context.TODO())
		plain    = gl.WithHeader("X", "y")
	)
	_, _, _ = c.Version.GetVersion(detached)
	_, _, _ = c.Version.GetVersion(plain)
}
`)
	assertFindings(t, report,
		"Variables: c.Version.GetVersion "+reasonMissing,
		"Variables: c.Version.GetVersion "+reasonDetached,
		"Variables: c.Version.GetVersion "+reasonMissing)
}

// TestScan_OptionsTheWalkDoesNotFollow_AreReported holds the conservative
// half of the rules: an option the walk cannot trace to gl.WithContext is
// treated as carrying no context, so the call is reported rather than passed.
// A slice kept in a struct field, an option a method or a function of this
// package builds, one a function value in a slice builds and a slice made
// empty are all of that kind, even where they would carry it; the answer to
// one is to pass gl.WithContext(ctx) beside it.
func TestScan_OptionsTheWalkDoesNotFollow_AreReported(t *testing.T) {
	report := scanBody(t, `
type holder struct{ opts []gl.RequestOptionFunc }

func (h holder) option(ctx context.Context) gl.RequestOptionFunc { return gl.WithContext(ctx) }

func option(ctx context.Context) gl.RequestOptionFunc { return gl.WithContext(ctx) }

func Untraced(ctx context.Context, c *gl.Client, h holder, makers []func() gl.RequestOptionFunc) {
	_, _, _ = c.Version.GetVersion(h.opts...)
	_, _, _ = c.Version.GetVersion(h.option(ctx))
	_, _, _ = c.Version.GetVersion(option(ctx))
	_, _, _ = c.Version.GetVersion(makers[0]())
	_, _, _ = c.Version.GetVersion(make([]gl.RequestOptionFunc, 0)...)
}
`)
	assertFindings(t, report,
		"Untraced: c.Version.GetVersion "+reasonMissing,
		"Untraced: c.Version.GetVersion "+reasonMissing,
		"Untraced: c.Version.GetVersion "+reasonMissing,
		"Untraced: c.Version.GetVersion "+reasonMissing,
		"Untraced: c.Version.GetVersion "+reasonMissing)
}

// TestScan_WhatIsNotARequest_IsNotJudged: appending options builds a slice
// and sends nothing, a conversion to the option type builds an option, and a
// call through a type parameter has no signature the checker can name. None
// of them is counted, which the third is as a stated limit rather than a
// verdict: its constraint could hide a service method.
func TestScan_WhatIsNotARequest_IsNotJudged(t *testing.T) {
	report := scanBody(t, `
func Build(ctx context.Context, fn func(*retryablehttp.Request) error) []gl.RequestOptionFunc {
	opts := append([]gl.RequestOptionFunc{}, gl.WithHeader("X", "y"))
	return append(opts, gl.RequestOptionFunc(fn))
}

func Generic[F func(...gl.RequestOptionFunc) error](call F) error {
	return call()
}
`)
	assertFindings(t, report)
	if report.Summary.Calls != 0 {
		t.Fatalf("calls = %d, want none judged", report.Summary.Calls)
	}
}

// TestScan_ACallHandedAnotherCallsResults_IsReported: `f(g())` has no
// argument per parameter to read, so the options cannot be seen and the call
// is reported rather than passed.
func TestScan_ACallHandedAnotherCallsResults_IsReported(t *testing.T) {
	report := scanBody(t, `
func two(ctx context.Context) (gl.RequestOptionFunc, gl.RequestOptionFunc) {
	return gl.WithContext(ctx), gl.WithHeader("X", "y")
}

func Spread(ctx context.Context, c *gl.Client) {
	_, _, _ = c.Version.GetVersion(two(ctx))
}
`)
	assertFindings(t, report, "Spread: c.Version.GetVersion "+reasonMissing)
}

// TestScan_FindingsAreNamedAfterTheirDeclaration holds the names a finding
// gives the place it sits, which is what a declaration is keyed on: a method
// by its receiver type with the pointer and the type parameters taken off, a
// closure by the function around it, and a package-level function literal by
// the variable it is assigned to.
func TestScan_FindingsAreNamedAfterTheirDeclaration(t *testing.T) {
	report := scanBody(t, `
type service struct{ c *gl.Client }

func (s *service) Pointer() { _, _, _ = s.c.Version.GetVersion() }

func (s service) Value() { _, _, _ = s.c.Version.GetVersion() }

type one[K any] struct{ c *gl.Client }

func (o *one[K]) Generic() { _, _, _ = o.c.Version.GetVersion() }

type two[K, V any] struct{ c *gl.Client }

func (t two[K, V]) Generics() { _, _, _ = t.c.Version.GetVersion() }

func (service) Unnamed() { _, _, _ = (*gl.Client)(nil).Version.GetVersion() }

func (s *(service)) Paren() { _, _, _ = s.c.Version.GetVersion() }

func Closure(c *gl.Client) {
	func() { _, _, _ = c.Version.GetVersion() }()
}

var fetch, other = func(c *gl.Client) { _, _, _ = c.Version.GetVersion() }, 1
`)
	var names []string
	for _, finding := range report.Findings {
		names = append(names, finding.Func)
	}
	slices.Sort(names)
	want := []string{"Closure", "fetch", "one.Generic", "service.Paren", "service.Pointer", "service.Unnamed", "service.Value", "two.Generics"}
	if !slices.Equal(names, want) {
		t.Fatalf("finding names = %v, want %v", names, want)
	}
}

// TestFuncDeclName_AReceiverThatIsNoTypeName_KeepsTheMethodName: a receiver
// written as a qualified name does not type-check, so no load hands one over,
// and the name is the method's own rather than a guess at the type.
func TestFuncDeclName_AReceiverThatIsNoTypeName_KeepsTheMethodName(t *testing.T) {
	fn := &ast.FuncDecl{
		Name: ast.NewIdent("Method"),
		Recv: &ast.FieldList{List: []*ast.Field{{Type: &ast.SelectorExpr{X: ast.NewIdent("pkg"), Sel: ast.NewIdent("T")}}}},
	}
	if got := funcDeclName(fn); got != "Method" {
		t.Fatalf("funcDeclName = %q, want Method", got)
	}
}

// TestScan_BuildConstrainedFiles holds the one blind spot a load has that the
// gate can see from outside: a file its build constraints left out. One that
// imports client-go is reported as unjudged, since every call in it went
// unread; one that does not is not the gate's business, and neither is a test
// file, which the gate would not read under any constraint: reporting it
// would blame a build tag for the blind spot the gate states.
func TestScan_BuildConstrainedFiles(t *testing.T) {
	report := auditFixture(t, map[string]string{
		"fixture.go": "package fixture\n",
		"hidden.go": `//go:build ignore

package fixture

import gl "gitlab.com/gitlab-org/api/client-go/v3"

var _ = gl.WithContext
`,
		"hidden_test.go": `//go:build ignore

package fixture

import gl "gitlab.com/gitlab-org/api/client-go/v3"

func version(c *gl.Client) { _, _, _ = c.Version.GetVersion() }
`,
		"elsewhere.go": `//go:build ignore

package fixture

import "context"

var _ = context.Background
`,
	}, nil)
	want := []string{fixtureDir + "/hidden.go"}
	if !slices.Equal(report.Unjudged, want) {
		t.Fatalf("unjudged = %v, want %v", report.Unjudged, want)
	}
	if report.ok() {
		t.Fatal("ok() = true, want an unjudged file to fail the gate")
	}
}

// TestNoteUnjudged_AFileThatCannotBeRead_IsUnjudged: nothing can be said about
// what an unreadable file calls, so it is reported with the rest. A file the
// load left out that is not Go source is not read at all, and neither is a
// test file, which is passed over before anything tries to read it.
func TestNoteUnjudged_AFileThatCannotBeRead_IsUnjudged(t *testing.T) {
	root := t.TempDir()
	found := newScanner(root, nil)
	found.noteUnjudged(&packages.Package{IgnoredFiles: []string{
		filepath.Join(root, "gone.go"),
		filepath.Join(root, "notes.txt"),
		filepath.Join(root, "gone_test.go"),
	}})
	if want := []string{"gone.go"}; !slices.Equal(found.unjudged, want) {
		t.Fatalf("unjudged = %v, want %v", found.unjudged, want)
	}
}

// TestImportsClientGo_ReadsThePathNotTheName: the import is recognized by its
// path under any name, a raw string included, and a path that only begins
// like it is another package.
func TestImportsClientGo_ReadsThePathNotTheName(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"quoted", `"gitlab.com/gitlab-org/api/client-go/v3"`, true},
		{"raw", "`gitlab.com/gitlab-org/api/client-go/v3`", true},
		{"a subpackage", `"gitlab.com/gitlab-org/api/client-go/v3/testing"`, false},
		{"something else", `"context"`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := &ast.File{Imports: []*ast.ImportSpec{{Path: &ast.BasicLit{Kind: token.STRING, Value: tt.path}}}}
			if got := importsClientGo(file); got != tt.want {
				t.Fatalf("importsClientGo(%s) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestRelativePath_OutsideTheRoot_KeepsTheAbsolutePath: a file the root does
// not contain is named in full rather than climbed to.
func TestRelativePath_OutsideTheRoot_KeepsTheAbsolutePath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	outside := filepath.Join(filepath.Dir(root), "elsewhere", "file.go")
	if got := relativePath(outside, root); got != filepath.ToSlash(outside) {
		t.Fatalf("relativePath = %q, want the absolute path %q", got, filepath.ToSlash(outside))
	}
	inside := filepath.Join(root, "internal", "x.go")
	if got := relativePath(inside, root); got != "internal/x.go" {
		t.Fatalf("relativePath = %q, want internal/x.go", got)
	}
	// A path that cannot be made relative to the root at all, because one of
	// the two is relative and the other is not, is kept as it was given.
	if got := relativePath(filepath.Join("already", "relative.go"), root); got != "already/relative.go" {
		t.Fatalf("relativePath = %q, want already/relative.go", got)
	}
}

// TestTypePredicates_ReadTheNamedTypeNotItsShape: the option and request
// types are recognized by package and name, so a look-alike declared
// elsewhere is not either of them, and a type with no package (a basic type)
// is neither.
func TestTypePredicates_ReadTheNamedTypeNotItsShape(t *testing.T) {
	elsewhere := types.NewPackage("example.com/other", "other")
	lookalike := types.NewNamed(types.NewTypeName(token.NoPos, elsewhere, "RequestOptionFunc", nil), types.NewSignatureType(nil, nil, nil, nil, nil, false), nil)
	if isOption(lookalike) {
		t.Fatal("isOption(other.RequestOptionFunc) = true, want false")
	}
	if isOptionSlice(types.NewSlice(lookalike)) {
		t.Fatal("isOptionSlice([]other.RequestOptionFunc) = true, want false")
	}
	if isOptionSlice(types.Typ[types.String]) {
		t.Fatal("isOptionSlice(string) = true, want false")
	}
	if isRequest(lookalike) {
		t.Fatal("isRequest(a non-pointer) = true, want false")
	}
	if isNamed(types.Universe.Lookup("error").Type(), "", "error") {
		t.Fatal("isNamed(error) = true, want false for a type with no package")
	}
}
