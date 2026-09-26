package paths

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// sdkSourceIn writes one directory of pretend client-go source and returns it.
//
// The files only have to parse, never to compile, which is what lets one
// fixture carry shapes a real SDK cannot hold at once: a call with no
// arguments, a verb constant from the wrong package, a route declared from
// something other than a literal.
func sdkSourceIn(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

// TestReadSDKRoutes_TheShapeEveryServiceMethodIsWrittenIn_IsRead verifies the
// second link of the type-grain join. Every endpoint client-go reaches is named
// by a route template and, when it is not a GET, by a withMethod option, so a
// method's result type plus those two calls are what say which operations may
// answer with a struct. Getting this wrong loses a type's routes silently,
// which reads as a type nothing routes to rather than as a parse that failed.
func TestReadSDKRoutes_TheShapeEveryServiceMethodIsWrittenIn_IsRead(t *testing.T) {
	dir := sdkSourceIn(t, map[string]string{
		"routes.go": `package gitlab

import "net/http"

var (
	routeProjects      = route("projects")
	routeProjectsID    = route("projects/%s")
	routeProjectsIDMRs = route("projects/%s/merge_requests/%d/approvals")
	routeArchive       = route("projects/%s/repository/archive%s")
)

const routeVersion = route("/version/")
`,
		"service.go": `package gitlab

func (s *ProjectsService) ListProjects(opt *Options) ([]*Project, *Response, error) {
	return do[[]*Project](s.client, withPath(routeProjects))
}

func (s *ProjectsService) GetProject(pid any) (*Project, *Response, error) {
	return do[*Project](s.client, withPath(routeProjectsID, ProjectID{pid}))
}

func (s *ProjectsService) ArchiveProject(pid any) (*Project, *Response, error) {
	return do[*Project](s.client, withPath(routeProjectsID, ProjectID{pid}), withMethod(http.MethodPost))
}

func (s *ProjectsService) DeleteProject(pid any) (*Response, error) {
	return do[none](s.client, withPath(routeProjectsID, ProjectID{pid}), withMethod(http.MethodDelete))
}

func (s *MergeRequestApprovalsService) GetConfiguration(pid any, mr int64) (*MergeRequestApprovals, *Response, error) {
	return do[*MergeRequestApprovals](s.client, withPath(routeProjectsIDMRs, ProjectID{pid}, mr))
}

func (s *RepositoriesService) ArchiveBytes(pid any) (*bytes.Buffer, *Response, error) {
	return do[bytes.Buffer](s.client, withPath(routeArchive, ProjectID{pid}, format))
}

func (s *RepositoriesService) ArchiveMeta(pid any) (*Archive, *Response, error) {
	return do[*Archive](s.client, withPath(routeArchive, ProjectID{pid}, format))
}

func (s *VersionService) GetVersion() (*Version, *Response, error) {
	return do[*Version](s.client, withPath(routeVersion))
}

func (s *GroupImportExportService) ImportFile() (*ImportStatus, *Response, error) {
	req, err := s.client.NewRequest(http.MethodPost, "groups/import", nil, options)
	return nil, nil, err
}

func (s *ProjectsService) NoResults() {}

func NotAMethod() (*Project, *Response, error) {
	return do[*Project](nil, withPath(routeProjects))
}
`,
	})

	routes := readSDKRoutes(dir)

	want := map[string][]sdkRoute{
		"Project": {
			{Method: "GET", Path: "/projects", Many: true},
			{Method: "GET", Path: "/projects/:"},
			{Method: "POST", Path: "/projects/:"},
		},
		"MergeRequestApprovals": {{Method: "GET", Path: "/projects/:/merge_requests/:/approvals"}},
		"Archive":               {{Method: "GET", Path: "/projects/:/repository/archive"}},
		"Version":               {{Method: "GET", Path: "/version"}},
		"ImportStatus":          {{Method: "POST", Path: "/groups/import"}},
	}
	if !reflect.DeepEqual(routes, want) {
		t.Errorf("readSDKRoutes() = %+v, want %+v", routes, want)
	}
}

// TestReadSDKRoutes_WhatItRefuses_ContributesNoRoute verifies that every shape
// this cannot resolve leaves its type with one route fewer rather than with a
// wrong one. A route guessed from a call it did not understand would be
// compared against somebody else's response, and a phantom reported from that
// is worse than a phantom missed.
func TestReadSDKRoutes_WhatItRefuses_ContributesNoRoute(t *testing.T) {
	dir := sdkSourceIn(t, map[string]string{
		"declarations.go": `package gitlab

import "net/http"

var (
	notACall     = 5
	notRouteFunc = other("projects")
	qualified    = pkg.route("projects")
	noArguments  = route()
	notALiteral  = route(pathConstant)
	notAString   = route(5)
	first, second = route("first")
)
`,
		"methods.go": `package gitlab

func (s *Service) NoPathNamed() (*Thing, *Response, error) {
	return do[*Thing](s.client, withPath())
}

func (s *Service) PathFromExpression() (*Thing, *Response, error) {
	return do[*Thing](s.client, withPath(buildRoute()))
}

func (s *Service) PathFromUnknownRoute() (*Thing, *Response, error) {
	return do[*Thing](s.client, withPath(routeNobodyDeclared))
}

func (s *Service) VerbWithTwoArguments() (*Thing, *Response, error) {
	return do[*Thing](s.client, withPath(first), withMethod(http.MethodPut, extra))
}

func (s *Service) VerbThatIsNotASelector() (*Verb, *Response, error) {
	return do[*Verb](s.client, withPath(first), withMethod(MethodPut))
}

func (s *Service) VerbFromAnotherPackage() (*Foreign, *Response, error) {
	return do[*Foreign](s.client, withPath(first), withMethod(rest.MethodPut))
}

func (s *Service) VerbThatIsNotOne() (*Constant, *Response, error) {
	return do[*Constant](s.client, withPath(first), withMethod(http.StatusOK))
}

func (s *Service) LegacyWithOneArgument() (*Thing, *Response, error) {
	req, err := s.client.NewRequest(http.MethodPost)
	return nil, nil, err
}

func (s *Service) LegacyWithoutAVerb() (*Thing, *Response, error) {
	req, err := s.client.NewRequest(verb, "groups/import", nil)
	return nil, nil, err
}

func (s *Service) LegacyWithAComputedPath() (*Thing, *Response, error) {
	req, err := s.client.NewRequest(http.MethodGet, u, opt)
	return nil, nil, err
}

func (s *Service) LegacyWithAnEmptyPath() (*Thing, *Response, error) {
	req, err := s.client.NewRequest(http.MethodPost, "", query)
	return nil, nil, err
}

func (s *Service) SomeOtherCall() (*Thing, *Response, error) {
	s.client.Do(request)
	plain()
	return nil, nil, nil
}
`,
	})

	routes := readSDKRoutes(dir)

	// Only the four types whose method named a declared route are here, each on
	// that route and each sending the GET a method that names no verb sends:
	// every withMethod in the fixture is one this refuses to read.
	want := map[string][]sdkRoute{
		"Verb":     {{Method: "GET", Path: "/first"}},
		"Foreign":  {{Method: "GET", Path: "/first"}},
		"Constant": {{Method: "GET", Path: "/first"}},
		"Thing":    {{Method: "GET", Path: "/first"}},
	}
	if !reflect.DeepEqual(routes, want) {
		t.Errorf("readSDKRoutes() = %+v, want %+v", routes, want)
	}
}

// TestReadSDKRoutes_WhatItReads_IsTheSourceAndNothingBeside verifies the file
// selection. Tests, subdirectories and a file that does not parse are all
// passed over: this reads a module cache it does not own the state of, and a
// scope that failed over somebody else's half-written file would report nothing
// at all.
func TestReadSDKRoutes_WhatItReads_IsTheSourceAndNothingBeside(t *testing.T) {
	dir := sdkSourceIn(t, map[string]string{
		"good.go": `package gitlab

var routeGood = route("good")

func (s *Service) Get() (*Good, *Response, error) {
	return do[*Good](s.client, withPath(routeGood))
}
`,
		"good_test.go": `package gitlab

var routeFromATest = route("from_a_test")

func (s *Service) FromATest() (*FromATest, *Response, error) {
	return do[*FromATest](s.client, withPath(routeFromATest))
}
`,
		"broken.go": `package gitlab

func (s *Service) Unclosed() (*Broken, *Response, error {
`,
		// Go source under another extension is a template or a backup, and it
		// parses exactly as well as the source does: the extension is the only
		// thing saying the compiler never reads it.
		"generated.go.tmpl": `package gitlab

var routeFromATemplate = route("from_a_template")

func (s *Service) FromATemplate() (*FromATemplate, *Response, error) {
	return do[*FromATemplate](s.client, withPath(routeFromATemplate))
}
`,
		"notes.md": "not Go at all",
	})
	if err := os.Mkdir(filepath.Join(dir, "testing"), 0o750); err != nil {
		t.Fatalf("create the subdirectory: %v", err)
	}

	routes := readSDKRoutes(dir)

	want := map[string][]sdkRoute{"Good": {{Method: "GET", Path: "/good"}}}
	if !reflect.DeepEqual(routes, want) {
		t.Errorf("readSDKRoutes() = %+v, want only the one route the source declares", routes)
	}
}

// TestReadSDKRoutes_OneEndpointNamedTwice_ContributesOneRoute verifies the
// shapes a branching service method puts in front of this reader.
//
// A method that chooses between two requests names its route twice and may
// name its verb in a call this cannot read, and the shortest legacy call in
// client-go passes the verb and the path and nothing else. Each of those has
// to leave the type with exactly the routes it really reaches: a duplicate
// would double a finding's evidence, and a two-argument call refused would
// lose an endpoint silently.
func TestReadSDKRoutes_OneEndpointNamedTwice_ContributesOneRoute(t *testing.T) {
	dir := sdkSourceIn(t, map[string]string{
		"routes.go": `package gitlab

var routeTwice = route("groups/%s/twice")
`,
		"methods.go": `package gitlab

import "net/http"

func (s *Service) OptionRouteNamedTwice(cond bool) (*Optioned, *Response, error) {
	if cond {
		return do[*Optioned](s.client, withPath(routeTwice, GroupID{gid}))
	}
	return do[*Optioned](s.client, withPath(routeTwice, GroupID{gid}))
}

func (s *Service) LegacyPathNamedTwice(cond bool) (*Legacy, *Response, error) {
	if cond {
		req, err := s.client.NewRequest(http.MethodPost, "groups/legacy", nil, options)
		return nil, nil, err
	}
	req, err := s.client.NewRequest(http.MethodPost, "groups/legacy", nil, options)
	return nil, nil, err
}

func (s *Service) LegacyWithTheVerbAndThePathAlone() (*Minimal, *Response, error) {
	req, err := s.client.NewRequest(http.MethodDelete, "groups/minimal")
	return nil, nil, err
}

func (s *Service) VerbQualifiedByMoreThanAPackage() (*Qualified, *Response, error) {
	return do[*Qualified](s.client, withPath(routeTwice, GroupID{gid}), withMethod(rest.http.MethodPut))
}
`,
	})

	routes := readSDKRoutes(dir)

	want := map[string][]sdkRoute{
		"Optioned": {{Method: "GET", Path: "/groups/:/twice"}},
		"Legacy":   {{Method: "POST", Path: "/groups/legacy"}},
		"Minimal":  {{Method: "DELETE", Path: "/groups/minimal"}},
		// The verb is qualified by a selector rather than by a package name,
		// so it names no net/http constant and the method sends the GET a
		// method naming no verb sends.
		"Qualified": {{Method: "GET", Path: "/groups/:/twice"}},
	}
	if !reflect.DeepEqual(routes, want) {
		t.Errorf("readSDKRoutes() = %+v, want %+v", routes, want)
	}
}

// TestReadSDKRoutes_AMethodWithAnEmptyResultList_NamesNoType verifies the
// third shape a result list can take.
//
// A method with no results at all has none to read, and one whose results are
// written as an empty pair has a list with nothing in it: reading the first
// element of that would be reading past the end of the slice, so the length is
// what decides rather than the presence of the list.
func TestReadSDKRoutes_AMethodWithAnEmptyResultList_NamesNoType(t *testing.T) {
	dir := sdkSourceIn(t, map[string]string{
		"routes.go": `package gitlab

var routeThing = route("things")
`,
		"methods.go": `package gitlab

func (s *Service) EmptyResultList() () {
	_ = do[*Thing](s.client, withPath(routeThing))
}

func (s *Service) NoResultsAtAll() {
	_ = do[*Thing](s.client, withPath(routeThing))
}

func (s *Service) Named() (*Thing, *Response, error) {
	return do[*Thing](s.client, withPath(routeThing))
}
`,
	})

	routes := readSDKRoutes(dir)

	want := map[string][]sdkRoute{"Thing": {{Method: "GET", Path: "/things"}}}
	if !reflect.DeepEqual(routes, want) {
		t.Errorf("readSDKRoutes() = %+v, want only the route the method naming a type reaches", routes)
	}
}

// TestReadSDKRoutes_NothingToRead_IsNoRoutes verifies that an absent directory
// is answered with nothing rather than with an error the scope would have to
// carry. The join above treats an empty result as a type nothing routes to,
// which is the same conclusion and needs no second way of saying it.
func TestReadSDKRoutes_NothingToRead_IsNoRoutes(t *testing.T) {
	cases := map[string]string{
		"no directory named":            "",
		"a directory that is not there": filepath.Join(t.TempDir(), "absent"),
		"a directory with no Go in it":  t.TempDir(),
	}
	for name, dir := range cases {
		t.Run(name, func(t *testing.T) {
			if routes := readSDKRoutes(dir); routes != nil {
				t.Errorf("readSDKRoutes(%q) = %+v, want nothing", dir, routes)
			}
		})
	}
}

// TestRouteShape_ATemplateWithPlaceholders_IsSpelledAsClientGoSpellsIt verifies the spelling both sides
// of the join have to meet in. client-go registers a template by replacing a
// segment that is nothing but a format verb, and by dropping a verb embedded in
// a longer segment; reproducing that rather than inventing one is what keeps a
// template such as "archive%s" meeting the path GitLab documents.
func TestRouteShape_ATemplateWithPlaceholders_IsSpelledAsClientGoSpellsIt(t *testing.T) {
	cases := []struct {
		name     string
		template string
		want     string
	}{
		{name: "a segment that is nothing but a verb", template: "projects/%s/issues", want: "/projects/:/issues"},
		{name: "the numeric verb", template: "projects/%s/issues/%d", want: "/projects/:/issues/:"},
		{name: "a verb embedded in a segment", template: "projects/%s/repository/archive%s", want: "/projects/:/repository/archive"},
		{name: "no verb at all", template: "version", want: "/version"},
		{name: "slashes at both ends", template: "/version/", want: "/version"},
		{name: "nothing", template: "", want: "/"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := routeShape(testCase.template); got != testCase.want {
				t.Errorf("routeShape(%q) = %q, want %q", testCase.template, got, testCase.want)
			}
		})
	}
}

// TestStringLiteral_ALiteralTheParserWouldRefuse_IsNotRead verifies the one
// branch a parsed fixture cannot reach: go/parser rejects a file holding a
// string it cannot unquote, so the guard is only reachable from a node built by
// hand. It is kept because this walks an AST it did not build the invariants
// of, and reading a broken literal as a route would name an endpoint nothing
// serves.
func TestStringLiteral_ALiteralTheParserWouldRefuse_IsNotRead(t *testing.T) {
	cases := []struct {
		name string
		expr ast.Expr
	}{
		{name: "not a literal at all", expr: ast.NewIdent("path")},
		{name: "a literal that is not a string", expr: &ast.BasicLit{Kind: token.INT, Value: "5"}},
		{name: "a string that does not unquote", expr: &ast.BasicLit{Kind: token.STRING, Value: `"unterminated`}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if value, ok := stringLiteral(testCase.expr); ok {
				t.Errorf("stringLiteral() = %q, true; want it refused", value)
			}
		})
	}
}

// TestReadSDKMethodRoutes_EachServiceMethod_IsKeyedByServiceAndName verifies
// the per-method view the projection pairings read. A compact row is filled
// from one method's answer, so the question is which endpoints that one method
// reaches, keyed the way a handler names the method through its interface:
// the concrete MilestonesService behind MilestonesServiceInterface is
// "Milestones". Two methods answering with one struct keep their routes apart
// here, which is the whole difference from the per-struct view.
func TestReadSDKMethodRoutes_EachServiceMethod_IsKeyedByServiceAndName(t *testing.T) {
	dir := sdkSourceIn(t, map[string]string{
		"milestones.go": `package gitlab

import "net/http"

var (
	routeProjectsIDIssues           = route("projects/%s/issues")
	routeProjectsIDMilestonesIssues = route("projects/%s/milestones/%d/issues")
	routeProjectsIDIssue            = route("projects/%s/issues/%d")
)

func (s *MilestonesService) GetMilestoneIssues(pid any, milestone int64) ([]*Issue, *Response, error) {
	return do[[]*Issue](s.client, withPath(routeProjectsIDMilestonesIssues, ProjectID{pid}, milestone))
}

func (s *IssuesService) ListProjectIssues(pid any) ([]*Issue, *Response, error) {
	return do[[]*Issue](s.client, withPath(routeProjectsIDIssues, ProjectID{pid}))
}

func (s IssuesService) UpdateIssue(pid any, issue int64) (*Issue, *Response, error) {
	return do[*Issue](s.client, withPath(routeProjectsIDIssue, ProjectID{pid}, issue), withMethod(http.MethodPut))
}

func (s *IssuesService) DeleteIssue(pid any, issue int64) (*Response, error) {
	return do[none](s.client, withPath(routeProjectsIDIssue, ProjectID{pid}, issue), withMethod(http.MethodDelete))
}

func (s *IssuesService) getByHelper(pid any) (*Issue, *Response, error) {
	return s.fetch(pid)
}

func (c *Client) Issue(pid any) (*Issue, *Response, error) {
	return do[*Issue](c, withPath(routeProjectsIDIssue, ProjectID{pid}, 1))
}

func (s *Service) Anything() (*Issue, *Response, error) {
	return do[*Issue](s.client, withPath(routeProjectsIDIssue, ProjectID{pid}, 1))
}
`,
	})

	routes := readSDKMethodRoutes(dir)

	// DeleteIssue answers with the pagination wrapper alone and reaches nothing
	// a caller reads; getByHelper names no route of its own; a method of the
	// Client, and of a type called Service and nothing else, is not a
	// service's.
	want := map[string][]sdkRoute{
		"Milestones.GetMilestoneIssues": {{Method: "GET", Path: "/projects/:/milestones/:/issues", Many: true}},
		"Issues.ListProjectIssues":      {{Method: "GET", Path: "/projects/:/issues", Many: true}},
		"Issues.UpdateIssue":            {{Method: "PUT", Path: "/projects/:/issues/:"}},
	}
	if !reflect.DeepEqual(routes, want) {
		t.Errorf("readSDKMethodRoutes() = %+v, want %+v", routes, want)
	}
}

// TestReadSDKMethodRoutes_ASourceThatCannotBeRead_GivesNothing verifies that
// the per-method view fails the way the per-struct one does: a module cache it
// cannot read contributes no route, so the projections it would have narrowed
// are counted as unrouted rather than held against a guess.
func TestReadSDKMethodRoutes_ASourceThatCannotBeRead_GivesNothing(t *testing.T) {
	for name, dir := range map[string]string{
		"no directory named": "",
		"a directory gone":   filepath.Join(t.TempDir(), "gone"),
	} {
		t.Run(name, func(t *testing.T) {
			if routes := readSDKMethodRoutes(dir); routes != nil {
				t.Errorf("readSDKMethodRoutes(%q) = %+v, want nothing", dir, routes)
			}
		})
	}
}

// TestServiceReceiver_OnlyAServiceNamesOne verifies the one branch a parsed
// method cannot reach, a receiver list with nothing in it, beside the shapes
// it can: a receiver that is not a service, and one called Service alone,
// which names no service a handler could call through an interface.
func TestServiceReceiver_OnlyAServiceNamesOne(t *testing.T) {
	cases := []struct {
		name     string
		receiver *ast.FieldList
		want     string
	}{
		{name: "an empty receiver list", receiver: &ast.FieldList{}},
		{
			name:     "a pointer to a service",
			receiver: &ast.FieldList{List: []*ast.Field{{Type: &ast.StarExpr{X: ast.NewIdent("JobsService")}}}},
			want:     "Jobs",
		},
		{
			name:     "a service by value",
			receiver: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("JobsService")}}},
			want:     "Jobs",
		},
		{name: "the client", receiver: &ast.FieldList{List: []*ast.Field{{Type: &ast.StarExpr{X: ast.NewIdent("Client")}}}}},
		{name: "a type called Service", receiver: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("Service")}}}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := serviceReceiver(testCase.receiver); got != testCase.want {
				t.Errorf("serviceReceiver() = %q, want %q", got, testCase.want)
			}
		})
	}
}
