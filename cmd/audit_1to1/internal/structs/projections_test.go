package structs

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"reflect"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
)

// fixtureSDKPath is where the pretend client-go lives. It carries the real
// module path as a prefix, which is all [clientGoNamedStruct] and
// [shared.ClientGoServiceInterface] ask of a package.
const fixtureSDKPath = shared.ClientGoPkgPath + "/v3"

// fixtureSDK is the part of client-go the projection fixtures read: two
// service interfaces, the structs their methods answer with, one struct nested
// in another, and the two non-result structs a literal must never be read off.
const fixtureSDK = `package gitlab

import "time"

type RequestOptionFunc func()

type Response struct {
	TotalItems int64 ` + "`json:\"total_items\"`" + `
}

type ListProjectsOptions struct {
	Search string ` + "`json:\"search\"`" + `
}

type BasicUser struct {
	Username string ` + "`json:\"username\"`" + `
}

type Issue struct {
	ID        int64      ` + "`json:\"id\"`" + `
	IID       int64      ` + "`json:\"iid\"`" + `
	Title     string     ` + "`json:\"title\"`" + `
	Author    *BasicUser ` + "`json:\"author\"`" + `
	CreatedAt *time.Time ` + "`json:\"created_at\"`" + `
}

type Project struct {
	ID   int64  ` + "`json:\"id\"`" + `
	Name string ` + "`json:\"name\"`" + `
}

type MilestonesServiceInterface interface {
	GetMilestoneIssues(pid any, milestone int64, options ...RequestOptionFunc) ([]*Issue, *Response, error)
	DeleteMilestone(pid any, milestone int64, options ...RequestOptionFunc) (*Response, error)
	Count(pid any) int
	Ping(options ...RequestOptionFunc)
}

type GroupsServiceInterface interface {
	ListGroupProjects(gid any, options ...RequestOptionFunc) ([]*Project, *Response, error)
	ListGroupSharedProjects(gid any, options ...RequestOptionFunc) ([]*Project, *Response, error)
	GetProject(gid any, options ...RequestOptionFunc) (*Project, *Response, error)
	SearchProjects(gid any, options ...RequestOptionFunc) ([]*Project, *Response, error)
	Options(gid any) (*ListProjectsOptions, *Response, error)
}

type Client struct {
	Milestones MilestonesServiceInterface
	Groups     GroupsServiceInterface
}
`

// checkedPackage type-checks the fixture SDK and one tool package importing it,
// and returns the tool package the way go/packages hands it to the audit.
func checkedPackage(t *testing.T, path, source string) *packages.Package {
	t.Helper()
	fileSet := token.NewFileSet()
	sdkFile, err := parser.ParseFile(fileSet, "sdk.go", fixtureSDK, 0)
	if err != nil {
		t.Fatalf("parse the SDK fixture: %v", err)
	}
	sdk, err := (&types.Config{Importer: importer.Default()}).Check(fixtureSDKPath, fileSet, []*ast.File{sdkFile}, nil)
	if err != nil {
		t.Fatalf("check the SDK fixture: %v", err)
	}

	file, err := parser.ParseFile(fileSet, "tool.go", source, 0)
	if err != nil {
		t.Fatalf("parse the tool fixture: %v", err)
	}
	info := &types.Info{
		Types:      map[ast.Expr]types.TypeAndValue{},
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	config := &types.Config{Importer: importerFunc(func(importPath string) (*types.Package, error) {
		if importPath == fixtureSDKPath {
			return sdk, nil
		}
		return importer.Default().Import(importPath)
	})}
	checked, err := config.Check(path, fileSet, []*ast.File{file}, info)
	if err != nil {
		t.Fatalf("check the tool fixture: %v", err)
	}
	return &packages.Package{PkgPath: path, Types: checked, TypesInfo: info, Syntax: []*ast.File{file}}
}

// importerFunc adapts a function to types.Importer.
type importerFunc func(path string) (*types.Package, error)

func (f importerFunc) Import(path string) (*types.Package, error) { return f(path) }

// TestCollectProjections_ARowBuiltInAHandler_IsPairedWithTheMethodItIsReadFrom
// verifies the shape issue 971 was filed about: a milestone's issue list keeps
// six fields of each issue in a literal inside the handler, and no converter
// names the pairing. Read here, the row is paired with the struct its fields
// come off and with the one method whose answer the handler ranges over, so
// the type grain can hold it to that endpoint alone.
func TestCollectProjections_ARowBuiltInAHandler_IsPairedWithTheMethodItIsReadFrom(t *testing.T) {
	pkg := checkedPackage(t, "example.com/x/internal/tools/milestones", `package milestones

import (
	"time"

	gl "`+fixtureSDKPath+`"
)

type IssueItem struct {
	ID        int64  `+"`json:\"id\"`"+`
	Title     string `+"`json:\"title\"`"+`
	Author    string `+"`json:\"author\"`"+`
	CreatedAt string `+"`json:\"created_at\"`"+`
}

type IssuesOutput struct {
	Issues []IssueItem `+"`json:\"issues\"`"+`
}

func GetIssues(client *gl.Client) IssuesOutput {
	issues, _, _ := client.Milestones.GetMilestoneIssues("p", 1)
	items := make([]IssueItem, 0, len(issues))
	for _, issue := range issues {
		items = append(items, IssueItem{
			ID:        issue.ID,
			Title:     issue.Title,
			Author:    issue.Author.Username,
			CreatedAt: issue.CreatedAt.Format(time.RFC3339),
		})
	}
	return IssuesOutput{Issues: items}
}
`)

	got := CollectProjections(pkg)

	want := []ProjectionPairing{{
		Package: "milestones", MCPType: "IssueItem", SDKType: "Issue",
		SDKFields: []string{"author", "created_at", "id", "iid", "title"},
		Methods:   []string{"Milestones.GetMilestoneIssues"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CollectProjections() = %+v, want %+v", got, want)
	}
}

// TestCollectProjections_AHelperTwoHandlersShare_IsCreditedWithBothMethods
// verifies the walk up the package's own call graph. The group's project rows
// are built in one helper that makes no request; the two handlers calling it
// each call a different method, and the row is read from both. A handler that
// calls the helper through a method value, and one reached only through a
// caller too far up, show where the walk looks and where it stops.
func TestCollectProjections_AHelperTwoHandlersShare_IsCreditedWithBothMethods(t *testing.T) {
	pkg := checkedPackage(t, "example.com/x/internal/tools/groups", `package groups

import gl "`+fixtureSDKPath+`"

type ProjectItem struct {
	ID   int64  `+"`json:\"id\"`"+`
	Name string `+"`json:\"name\"`"+`
}

func rows(projects []*gl.Project) []ProjectItem {
	out := make([]ProjectItem, len(projects))
	for i, p := range projects {
		out[i] = ProjectItem{ID: p.ID, Name: p.Name}
	}
	return out
}

func ListProjects(client *gl.Client) []ProjectItem {
	projects, _, _ := client.Groups.ListGroupProjects("g")
	return rows(projects)
}

func ListShared(client *gl.Client) []ProjectItem {
	list := client.Groups.ListGroupSharedProjects
	projects, _, _ := list("g")
	convert := rows
	return convert(projects)
}

func levelOne() []ProjectItem { return rows(nil) }
func levelTwo() []ProjectItem { return levelOne() }
func levelThree() []ProjectItem { return levelTwo() }

func JustFarEnough(client *gl.Client) []ProjectItem {
	_, _, _ = client.Groups.GetProject("g")
	return levelTwo()
}

func TooFarUp(client *gl.Client) []ProjectItem {
	_, _, _ = client.Groups.SearchProjects("g")
	return levelThree()
}

func diamondLeft() []ProjectItem  { return rows(nil) }
func diamondRight() []ProjectItem { return rows(nil) }

func Diamond(client *gl.Client) []ProjectItem {
	_, _, _ = client.Groups.ListGroupProjects("g")
	return append(diamondLeft(), diamondRight()...)
}

type PairItem struct {
	ID   int64
	Name string
}

func Positional(client *gl.Client) PairItem {
	projects, _, _ := client.Groups.ListGroupProjects("g")
	return PairItem{projects[0].ID, projects[0].Name}
}

func recursive(n int) int {
	if n == 0 {
		return 0
	}
	return recursive(n - 1)
}
`)

	got := CollectProjections(pkg)

	// JustFarEnough calls a method answering with the same struct three calls
	// above the literal, which is as far as the walk climbs, and TooFarUp four,
	// which is one more. Diamond is reached twice, through both of its helpers,
	// and read once. A literal naming no keys is read by position the same way.
	want := []ProjectionPairing{
		{
			Package: "groups", MCPType: "PairItem", SDKType: "Project",
			SDKFields: []string{"id", "name"},
			Methods:   []string{"Groups.ListGroupProjects"},
		},
		{
			Package: "groups", MCPType: "ProjectItem", SDKType: "Project",
			SDKFields: []string{"id", "name"},
			Methods:   []string{"Groups.GetProject", "Groups.ListGroupProjects", "Groups.ListGroupSharedProjects"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CollectProjections() = %+v, want %+v", got, want)
	}
}

// TestCollectProjections_ACallerAboveAMatchedHandler_IsNotClimbedTo verifies
// where the walk stops on a branch. The rows are built in a helper that makes
// no request, the handler calling it references the method the rows are read
// from, and a dispatcher above that handler references another method
// answering with the same struct before calling it. The rows were never read
// from the dispatcher's answer, so once the handler matched, its branch is
// done and the dispatcher's method is not credited to them.
func TestCollectProjections_ACallerAboveAMatchedHandler_IsNotClimbedTo(t *testing.T) {
	pkg := checkedPackage(t, "example.com/x/internal/tools/groups", `package groups

import gl "`+fixtureSDKPath+`"

type ProjectItem struct {
	ID   int64  `+"`json:\"id\"`"+`
	Name string `+"`json:\"name\"`"+`
}

func rows(projects []*gl.Project) []ProjectItem {
	out := make([]ProjectItem, len(projects))
	for i, p := range projects {
		out[i] = ProjectItem{ID: p.ID, Name: p.Name}
	}
	return out
}

func Handler(client *gl.Client) []ProjectItem {
	projects, _, _ := client.Groups.ListGroupProjects("g")
	return rows(projects)
}

func Dispatch(client *gl.Client) []ProjectItem {
	_, _, _ = client.Groups.SearchProjects("g")
	return Handler(client)
}
`)

	got := CollectProjections(pkg)

	want := []ProjectionPairing{{
		Package: "groups", MCPType: "ProjectItem", SDKType: "Project",
		SDKFields: []string{"id", "name"},
		Methods:   []string{"Groups.ListGroupProjects"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CollectProjections() = %+v, want the handler's method alone, %+v", got, want)
	}
}

// TestCollectProjections_WhatIsNotAProjection_IsNotPaired verifies every shape
// a literal can take that names no endpoint's answer: a struct read off two
// structs equally, one read off nothing, one read off the pagination wrapper
// or an options struct, an input, an unexported struct, a type of another
// package, and an outer literal whose only evidence is a nested literal of the
// package's own, which is a projection of its own and says nothing about the
// outer one.
func TestCollectProjections_WhatIsNotAProjection_IsNotPaired(t *testing.T) {
	pkg := checkedPackage(t, "example.com/x/internal/tools/mixed", `package mixed

import (
	"strings"

	gl "`+fixtureSDKPath+`"
)

type TiedOutput struct {
	ID   int64  `+"`json:\"id\"`"+`
	Name string `+"`json:\"name\"`"+`
}

type ConstantOutput struct {
	Name string `+"`json:\"name\"`"+`
}

type PageOutput struct {
	Total int64 `+"`json:\"total\"`"+`
}

type SearchOutput struct {
	Search string `+"`json:\"search\"`"+`
}

type CreateInput struct {
	Name string `+"`json:\"name\"`"+`
}

type rawOutput struct {
	Name string `+"`json:\"name\"`"+`
}

type AuthorOutput struct {
	Username string `+"`json:\"username\"`"+`
}

type WrapperOutput struct {
	Author AuthorOutput `+"`json:\"author\"`"+`
}

func Build(issue *gl.Issue, project *gl.Project, resp *gl.Response, opts *gl.ListProjectsOptions, err error) {
	local := CreateInput{Name: "local"}
	_ = TiedOutput{ID: issue.ID, Name: project.Name}
	_ = ConstantOutput{Name: "fixed"}
	_ = ConstantOutput{Name: local.Name}
	_ = ConstantOutput{Name: err.Error()}
	_ = PageOutput{Total: resp.TotalItems}
	_ = SearchOutput{Search: opts.Search}
	_ = CreateInput{Name: project.Name}
	_ = rawOutput{Name: project.Name}
	_ = strings.Builder{}
	_ = WrapperOutput{Author: AuthorOutput{Username: issue.Author.Username}}
	_ = NameOutput{Name: project.Name}
	_ = NameOutput{Name: issue.Title}
}

type NameOutput struct {
	Name string `+"`json:\"name\"`"+`
}
`)

	got := CollectProjections(pkg)

	// AuthorOutput is paired, and paired with the author rather than with the
	// issue the author sits in: that is the struct its one field is read off.
	// No method answers with a user, so none is named. NameOutput is built
	// from two structs in two literals and is paired with each, in the order
	// of their names.
	want := []ProjectionPairing{
		{Package: "mixed", MCPType: "AuthorOutput", SDKType: "BasicUser", SDKFields: []string{"username"}},
		{Package: "mixed", MCPType: "NameOutput", SDKType: "Issue", SDKFields: []string{"author", "created_at", "id", "iid", "title"}},
		{Package: "mixed", MCPType: "NameOutput", SDKType: "Project", SDKFields: []string{"id", "name"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CollectProjections() = %+v, want %+v", got, want)
	}
}

// TestCollectProjections_APackageWithoutTypeInformation_GivesNothing verifies
// the guard for a package go/packages could not type-check: without the
// information a literal's type and a selector's receiver are read from, every
// answer would be a guess.
func TestCollectProjections_APackageWithoutTypeInformation_GivesNothing(t *testing.T) {
	if got := CollectProjections(&packages.Package{PkgPath: "example.com/x/internal/tools/none"}); got != nil {
		t.Errorf("CollectProjections() = %+v, want nothing", got)
	}
}

// TestWalkFunctions_ADeclarationWithNoObject_IsPassedOver verifies the branch
// a type-checked package cannot reach: a function declaration the checker
// recorded no object for. It is kept because the walk reads an AST whose
// invariants it did not build, and crediting a literal to no function would
// make every caller of nothing its caller. A declaration with no body, which
// is how a function implemented in assembly is written, has nothing to walk.
func TestWalkFunctions_ADeclarationWithNoObject_IsPassedOver(t *testing.T) {
	body := &ast.BlockStmt{}
	pkg := &packages.Package{
		PkgPath:   "example.com/x/internal/tools/p",
		TypesInfo: &types.Info{Defs: map[*ast.Ident]types.Object{}},
		Syntax: []*ast.File{{Decls: []ast.Decl{
			&ast.FuncDecl{Name: ast.NewIdent("Orphan"), Type: &ast.FuncType{}, Body: body},
			&ast.FuncDecl{Name: ast.NewIdent("External"), Type: &ast.FuncType{}},
			&ast.GenDecl{Tok: token.VAR},
		}}},
	}

	walked := walkFunctions(pkg)

	if len(walked.calls) != 0 || len(walked.callers) != 0 || len(walked.sites) != 0 {
		t.Errorf("walkFunctions() = %+v, want nothing recorded", walked)
	}
}

// TestServiceMethod_WhatIsNotAnAnswer_NamesNoMethod verifies the selectors
// that name no struct a projection could be read from: a method answering with
// the pagination wrapper alone, one answering with no client-go struct, one
// answering with an options struct, and a selector on something that is not a
// service at all.
func TestServiceMethod_WhatIsNotAnAnswer_NamesNoMethod(t *testing.T) {
	pkg := checkedPackage(t, "example.com/x/internal/tools/calls", `package calls

import gl "`+fixtureSDKPath+`"

func Call(client *gl.Client, issue *gl.Issue) {
	_, _ = client.Milestones.DeleteMilestone("p", 1)
	_ = client.Milestones.Count("p")
	client.Milestones.Ping()
	_, _, _ = client.Groups.Options("g")
	_ = issue.Title
}
`)

	walked := walkFunctions(pkg)

	if len(walked.calls) != 0 {
		t.Errorf("calls = %+v, want none", walked.calls)
	}

	// The branch a type-checked selector cannot reach: a service interface
	// whose method the checker recorded no object for.
	sdk := types.NewPackage(fixtureSDKPath, "gitlab")
	service := types.NewNamed(types.NewTypeName(token.NoPos, sdk, "IssuesServiceInterface", nil), types.NewInterfaceType(nil, nil), nil)
	receiver := ast.NewIdent("issues")
	selector := &ast.SelectorExpr{X: receiver, Sel: ast.NewIdent("ListIssues")}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{receiver: {Type: service}}, Uses: map[*ast.Ident]types.Object{}}
	if call, ok := serviceMethod(info, selector); ok {
		t.Errorf("serviceMethod() = %+v, true; want no method named", call)
	}
}

// TestAnswerStruct_EveryWrapping_ReachesTheStruct verifies the unwrapping of a
// method's first result, and that a result that is not a client-go struct at
// its core names nothing.
func TestAnswerStruct_EveryWrapping_ReachesTheStruct(t *testing.T) {
	sdk := types.NewPackage(fixtureSDKPath, "gitlab")
	issue := types.NewNamed(types.NewTypeName(token.NoPos, sdk, "Issue", nil), types.NewStruct(nil, nil), nil)
	cases := []struct {
		name  string
		typed types.Type
		want  bool
	}{
		{name: "a pointer", typed: types.NewPointer(issue), want: true},
		{name: "a slice of pointers", typed: types.NewSlice(types.NewPointer(issue)), want: true},
		{name: "a slice of values", typed: types.NewSlice(issue), want: true},
		{name: "a string", typed: types.Typ[types.String]},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := answerStruct(testCase.typed); (got != nil) != testCase.want {
				t.Errorf("answerStruct(%s) = %v, want a struct: %v", testCase.typed, got, testCase.want)
			}
		})
	}
}
