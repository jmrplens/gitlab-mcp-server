package main

import (
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/derive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sdkroutes"
)

// sdkFixtureDir is sdkroutes' own stand-in for client-go, which holds every
// shape of method this reading answers for. It is found from the module root
// rather than from this package's directory, so the tests read it from a copy
// of the package staged elsewhere in the module too, which is how a mutation
// run measures a package main.
func sdkFixtureDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(moduleRoot(t), "cmd", "internal", "sdkroutes", "testdata", "sdk")
}

// inlineDocument places a document written inline in the stand-in at the line
// holding marker, the way graphqldocs.SDKDocuments reports one, with the text
// the test gives it.
func inlineDocument(t *testing.T, file, marker, text string) graphqldocs.Document {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(sdkFixtureDir(t), file))
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	for i, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, marker) {
			return graphqldocs.Document{Position: token.Position{Filename: "/elsewhere/" + file, Line: i + 1}, Text: text}
		}
	}
	t.Fatalf("%s holds no line with %q", file, marker)
	return graphqldocs.Document{}
}

// TestSDKSource_Requests_AnswersByWhatTheMethodSends verifies the reading of
// one client-go method: its routes, a legacy request it cannot fold as an
// unresolved route, its documents with the lookups before the writes, a
// document assembled from a template or a format string as an unresolved
// request a declaration answers, and a method client-go does not declare as
// unknown.
func TestSDKSource_Requests_AnswersByWhatTheMethodSends(t *testing.T) {
	source := &sdkSource{sdk: sdkroutes.Read(sdkFixtureDir(t)), documents: []graphqldocs.Document{
		{Name: "listAchievementsQuery", Text: "query { achievements }"},
		{Name: "templateSource", Text: "mutation { item { {{ template \"fields\" }} } }"},
		inlineDocument(t, "declarations.go", "query { inlineInInitializer }", "query { inlineInInitializer }"),
		inlineDocument(t, "declarations.go", "query { chained }", "query { chained(name: %q) }"),
	}}
	cases := []struct {
		method    string
		routes    []derive.Request
		documents []derive.Request
		known     bool
	}{
		{method: "Absent.Method"},
		{
			method: "Issues.GetIssue", known: true,
			routes: []derive.Request{{Kind: derive.KindREST, Method: "GET", Path: "/projects/:/issues/:"}},
		},
		{
			method: "Repositories.Fetch", known: true,
			routes: []derive.Request{
				{Kind: derive.KindREST, Method: "DELETE", Path: "/two/args"},
				{Kind: derive.KindREST, Method: "POST", Path: "/projects/uploads"},
				{Kind: derive.KindUnresolved, Reason: "sdk-path Repositories.Fetch"},
			},
		},
		{
			method: "Achievements.ListAchievements", known: true,
			documents: []derive.Request{{Kind: derive.KindGraphQL, Document: "query { achievements }", Name: "listAchievementsQuery"}},
		},
		{
			method: "WorkItems.GetWorkItem", known: true,
			documents: []derive.Request{
				{Kind: derive.KindGraphQL, Document: "query { inlineInInitializer }"},
				{Kind: derive.KindUnresolved, Reason: "sdk-graphql-format WorkItems.GetWorkItem"},
				{Kind: derive.KindUnresolved, Reason: "sdk-graphql-template WorkItems.GetWorkItem"},
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.method, func(t *testing.T) {
			routes, documents, known := source.Requests(testCase.method)
			if known != testCase.known || !reflect.DeepEqual(routes, testCase.routes) || !reflect.DeepEqual(documents, testCase.documents) {
				t.Errorf("Requests(%q) =\n%+v\n%+v\n%t\nwant\n%+v\n%+v\n%t", testCase.method,
					routes, documents, known, testCase.routes, testCase.documents, testCase.known)
			}
		})
	}
}

// TestClientGoDir_IsTheModuleTheProgramImports verifies client-go is read
// from the directory of the package the loaded program imports, and from
// nowhere when no package imports it or the import carries no files.
func TestClientGoDir_IsTheModuleTheProgramImports(t *testing.T) {
	withFiles := &packages.Package{Imports: map[string]*packages.Package{
		actionrequests.ClientGoPath: {GoFiles: []string{filepath.Join("mod", "client-go", "gitlab.go")}},
	}}
	empty := &packages.Package{Imports: map[string]*packages.Package{actionrequests.ClientGoPath: {}}}
	cases := []struct {
		name string
		pkgs []*packages.Package
		want string
	}{
		{name: "imported", pkgs: []*packages.Package{{}, withFiles}, want: filepath.Join("mod", "client-go")},
		{name: "no files", pkgs: []*packages.Package{empty}},
		{name: "not imported", pkgs: []*packages.Package{{}}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := clientGoDir(testCase.pkgs); got != testCase.want {
				t.Errorf("clientGoDir = %q, want %q", got, testCase.want)
			}
		})
	}
}
