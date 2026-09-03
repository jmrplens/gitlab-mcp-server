// Unit tests for the raw-GraphQL rule: how a declaration is named, how a
// package resolves to a service, and how sites become adjudicated operations.

package sdk

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"reflect"
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/shared"
)

// declFrom parses a snippet and returns its last function declaration, so the
// naming cases read as the Go they are about even when a receiver type has to
// be declared first.
func declFrom(t *testing.T, src string) *ast.FuncDecl {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "x.go", "package p\n\n"+src, 0)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	for _, decl := range slices.Backward(file.Decls) {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			return fn
		}
	}
	t.Fatalf("%q declares no function", src)
	return nil
}

// TestFunctionName_Declarations_NameFunctionsAndMethods verifies the operation
// key each declaration form produces, including the generic receivers whose
// type expression is an index rather than a name.
func TestFunctionName_Declarations_NameFunctionsAndMethods(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{name: "function", src: "func List() {}", want: "List"},
		{name: "value_receiver", src: "type S struct{}\n\nfunc (s S) List() {}", want: "S.List"},
		{name: "pointer_receiver", src: "func (s *Service) List() {}", want: "Service.List"},
		{name: "generic_receiver", src: "func (s *Store[T]) List() {}", want: "Store.List"},
		{name: "generic_receiver_two_params", src: "func (s Store[K, V]) List() {}", want: "Store.List"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := functionName(declFrom(t, tc.src)); got != tc.want {
				t.Errorf("functionName = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestReceiverName_Unexpected_FallsBackRatherThanPanics verifies an unforeseen
// receiver expression yields a placeholder, so a future Go form cannot crash
// the audit.
func TestReceiverName_Unexpected_FallsBackRatherThanPanics(t *testing.T) {
	if got := receiverName(&ast.BasicLit{Kind: token.INT, Value: "1"}); got != "?" {
		t.Errorf("receiverName of an unexpected expression = %q, want ?", got)
	}
}

// TestRepoRelativeFile_Separators_TrimTheWorkspacePrefixOnEveryPlatform
// verifies the trimming that turns a loader's absolute path into the
// repository-relative form the declared table is written in. The Windows case
// is the one that matters: go/token normalizes nothing, so a backslash path
// reaches this function verbatim, and without normalization the search finds no
// separator and the developer's whole workspace path is reported as the site.
func TestRepoRelativeFile_Separators_TrimTheWorkspacePrefixOnEveryPlatform(t *testing.T) {
	cases := []struct {
		name string
		file string
		want string
	}{
		{
			name: "unix absolute path",
			file: "/home/dev/gitlab-mcp-server/internal/tools/epics/epics.go",
			want: "internal/tools/epics/epics.go",
		},
		{
			name: "windows absolute path",
			file: `C:\Users\dev\gitlab-mcp-server\internal\tools\epics\epics.go`,
			want: "internal/tools/epics/epics.go",
		},
		{
			name: "already relative",
			file: "internal/tools/epics/epics.go",
			want: "internal/tools/epics/epics.go",
		},
		{
			name: "no internal segment is left alone",
			file: "/home/dev/gitlab-mcp-server/main.go",
			want: "/home/dev/gitlab-mcp-server/main.go",
		},
		{
			name: "the last internal segment wins",
			file: "/home/dev/internal/checkouts/repo/internal/tools/epics/epics.go",
			want: "internal/tools/epics/epics.go",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := repoRelativeFile(tc.file); got != tc.want {
				t.Errorf("repoRelativeFile(%q) = %q, want %q", tc.file, got, tc.want)
			}
		})
	}
}

// TestIsGraphQLInterface_Types_MatchOnlyThatInterface verifies the type test
// behind every recorded site: the interface itself, through a pointer, and
// nothing else.
func TestIsGraphQLInterface_Types_MatchOnlyThatInterface(t *testing.T) {
	pkg := types.NewPackage(shared.ClientGoPkgPath+"/v2", "gitlab")
	named := func(name string) *types.Named {
		iface := types.NewInterfaceType(nil, nil)
		iface.Complete()
		return types.NewNamed(types.NewTypeName(token.NoPos, pkg, name, nil), iface, nil)
	}
	graphQL := named("GraphQLInterface")
	cases := []struct {
		name string
		typ  types.Type
		want bool
	}{
		{name: "nil", typ: nil, want: false},
		{name: "basic", typ: types.Typ[types.String], want: false},
		{name: "other_interface", typ: named("BranchesServiceInterface"), want: false},
		{name: "the_interface", typ: graphQL, want: true},
		{name: "pointer_to_it", typ: types.NewPointer(graphQL), want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isGraphQLInterface(tc.typ, graphQL); got != tc.want {
				t.Errorf("isGraphQLInterface = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestServiceForPackage_Names_MatchCaseInsensitivelyOrByAlias verifies the
// package-to-service resolution: a case-insensitive name match, an explicit
// alias for the spellings that differ, an alias naming a service the SDK does
// not declare (which resolves to nothing rather than inventing one), and a
// package with no counterpart.
func TestServiceForPackage_Names_MatchCaseInsensitivelyOrByAlias(t *testing.T) {
	services := map[string]sdkService{
		"WorkItems":              {Service: "WorkItems", APIMethods: 6},
		"ProjectVulnerabilities": {Service: "ProjectVulnerabilities", APIMethods: 2},
	}
	aliases := map[string]string{
		"vulnerabilities": "ProjectVulnerabilities",
		"phantom":         "NoSuchService",
	}
	cases := []struct {
		name    string
		pkg     string
		want    string
		wantOK  bool
		methods int
	}{
		{name: "case_insensitive_name_match", pkg: "workitems", want: "WorkItems", wantOK: true, methods: 6},
		{name: "alias", pkg: "vulnerabilities", want: "ProjectVulnerabilities", wantOK: true, methods: 2},
		{name: "alias_to_an_absent_service", pkg: "phantom", wantOK: false},
		{name: "no_counterpart", pkg: "cicatalog", wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, ok := serviceForPackage(tc.pkg, services, aliases)
			if ok != tc.wantOK {
				t.Fatalf("serviceForPackage(%q) ok = %v, want %v", tc.pkg, ok, tc.wantOK)
			}
			if ok && (service.Service != tc.want || service.APIMethods != tc.methods) {
				t.Errorf("serviceForPackage(%q) = %+v, want %s/%d", tc.pkg, service, tc.want, tc.methods)
			}
		})
	}
}

// TestBuildGraphQLOperations_Sites_GroupPerFunctionAndAdjudicate verifies the
// grouping and the verdict: sites in the same function collapse into one
// operation carrying every position, a package with no client-go counterpart
// is dropped rather than reported, and the table's decision decides the status.
func TestBuildGraphQLOperations_Sites_GroupPerFunctionAndAdjudicate(t *testing.T) {
	sites := []graphqlSite{
		{pkg: "workitems", function: "List", position: "internal/tools/workitems/workitems.go:10"},
		{pkg: "workitems", function: "List", position: "internal/tools/workitems/workitems.go:20"},
		{pkg: "workitems", function: "Get", position: "internal/tools/workitems/workitems.go:40"},
		{pkg: "cicatalog", function: "List", position: "internal/tools/cicatalog/cicatalog.go:10"},
	}
	services := map[string]sdkService{"WorkItems": {Service: "WorkItems", APIMethods: 6}}
	decisions := map[string]decision{"workitems.List": {decisionMigrate, "the wrapper covers it"}}

	got := buildGraphQLOperations(sites, services, nil, decisions)
	want := []graphqlOperation{
		{
			Package: "workitems", Operation: "Get", Service: "WorkItems", ServiceMethods: 6,
			Status: statusUnadjudicated,
			Sites:  []string{"internal/tools/workitems/workitems.go:40"},
		},
		{
			Package: "workitems", Operation: "List", Service: "WorkItems", ServiceMethods: 6,
			Status: statusAdjudicated, Decision: decisionMigrate, Reason: "the wrapper covers it",
			Sites: []string{"internal/tools/workitems/workitems.go:10", "internal/tools/workitems/workitems.go:20"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildGraphQLOperations = %+v, want %+v", got, want)
	}
}

// TestStaleGraphQLDecisions_Keys_ReportDecisionsWithoutAnOperation verifies a
// decision whose operation is gone is reported, and one still matching an
// operation is not.
func TestStaleGraphQLDecisions_Keys_ReportDecisionsWithoutAnOperation(t *testing.T) {
	operations := []graphqlOperation{{Package: "workitems", Operation: "List"}}
	decisions := map[string]decision{
		"workitems.List":   {decisionMigrate, "still there"},
		"workitems.Gone":   {decisionKeep, "the function was removed"},
		"removed.Anything": {decisionKeep, "the package was removed"},
	}
	want := []string{"graphql removed.Anything", "graphql workitems.Gone"}
	if got := staleGraphQLDecisions(operations, decisions); !reflect.DeepEqual(got, want) {
		t.Errorf("staleGraphQLDecisions = %v, want %v", got, want)
	}
}
