package paths

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// sdkSourceIn writes one directory of pretend client-go source and returns it.
//
// The files only have to parse, never to compile, which is what lets one
// fixture carry shapes a real SDK cannot hold at once. What the reading itself
// does with each shape is held by cmd/internal/sdkroutes' own suite; what is
// held here is the three views R-PATH cuts from it.
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
//
// The award emoji methods are the case the shared reading was moved here for:
// both reach their route through one helper handed a collection name, and the
// reading this replaced gave neither a route at all, while its helper's own
// read produced /projects/:/:/:/award_emoji/:, a path no GitLab route has.
func TestReadSDKRoutes_TheShapeEveryServiceMethodIsWrittenIn_IsRead(t *testing.T) {
	dir := sdkSourceIn(t, map[string]string{
		"routes.go": `package gitlab

import "net/http"

var (
	routeProjects      = route("projects")
	routeProjectsID    = route("projects/%s")
	routeProjectsIDMRs = route("projects/%s/merge_requests/%d/approvals")
	routeArchive       = route("projects/%s/repository/archive%s")
	routeEmoji         = route("projects/%s/%s/%d/award_emoji/%d")
)

const routeVersion = route("/version/")

const awardIssue = "issues"
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

func (s *AwardEmojiService) GetIssueAwardEmoji(pid any, iid, award int64) (*AwardEmoji, *Response, error) {
	return s.getAwardEmoji(pid, awardIssue, iid, award)
}

func (s *AwardEmojiService) getAwardEmoji(pid any, resource string, id, award int64) (*AwardEmoji, *Response, error) {
	return do[*AwardEmoji](s.client, withPath(routeEmoji, ProjectID{pid}, resource, id, award))
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
		"AwardEmoji":            {{Method: "GET", Path: "/projects/:/issues/:/award_emoji/:"}},
	}
	if !reflect.DeepEqual(routes, want) {
		t.Errorf("readSDKRoutes() = %+v, want %+v", routes, want)
	}
}

// TestReadSDKMethodRoutes_EachServiceMethod_IsKeyedByServiceAndName verifies
// the per-method view the projection pairings read. A compact row is filled
// from one method's answer, so the question is which endpoints that one method
// reaches, keyed the way a handler names the method through its interface:
// the concrete MilestonesService behind MilestonesServiceInterface is
// "Milestones". Two methods answering with one struct keep their routes apart
// here, which is the whole difference from the per-struct view, and a method
// answering with nothing but the pagination wrapper is listed, since its route
// is a fact about it whatever it returns.
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

	// getByHelper is no method a handler can call; a method of the Client, and
	// of a type called Service and nothing else, is not a service's.
	want := map[string][]sdkRoute{
		"Milestones.GetMilestoneIssues": {{Method: "GET", Path: "/projects/:/milestones/:/issues", Many: true}},
		"Issues.ListProjectIssues":      {{Method: "GET", Path: "/projects/:/issues", Many: true}},
		"Issues.UpdateIssue":            {{Method: "PUT", Path: "/projects/:/issues/:"}},
		"Issues.DeleteIssue":            {{Method: "DELETE", Path: "/projects/:/issues/:"}},
	}
	if !reflect.DeepEqual(routes, want) {
		t.Errorf("readSDKMethodRoutes() = %+v, want %+v", routes, want)
	}
}

// TestReadSDKRoutes_NothingToRead_IsNoRoutes verifies that both route views
// give nothing, rather than an empty answer a caller could mistake for a
// reading, when the module cache cannot be read or holds no route: the
// projections they would have narrowed are counted as unrouted rather than
// held against a guess.
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
			if routes := readSDKMethodRoutes(dir); routes != nil {
				t.Errorf("readSDKMethodRoutes(%q) = %+v, want nothing", dir, routes)
			}
		})
	}
}

// TestSDKRoute_Operation_NamesACollection verifies the spelling a finding
// names a route by.
func TestSDKRoute_Operation_NamesACollection(t *testing.T) {
	if got := (sdkRoute{Method: "GET", Path: "/projects", Many: true}).operation(); got != "GET /projects (collection)" {
		t.Errorf("operation() = %q, want the collection named", got)
	}
	if got := (sdkRoute{Method: "GET", Path: "/projects/:"}).operation(); got != "GET /projects/:" {
		t.Errorf("operation() = %q, want the route alone", got)
	}
}
