// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// keyset_pagination_test.go follows a list GitLab pages by keyset to its end
// through the next_page_token each page before the last publishes, once for
// each of the three parameters GitLab resumes a keyset listing by: id_after on
// the project list sorted ascending, cursor on a project's issues, and
// Gitaly's page_token on the repository tree. Until each page published its
// next link, a caller who asked for keyset pagination got the first page and
// no way to ask for the second (issue 1165).

package common

import (
	"slices"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/repository"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// keysetPageLimit bounds how many pages a scenario follows. Each listing holds
// three rows read one a page, and Gitaly may answer one empty page after the
// last, so a fifth page means the listing is going round rather than on.
const keysetPageLimit = 5

// followKeyset reads a listing to its end by keyset, one row a page: the
// first call asks for pagination=keyset and every later one passes back the
// next_page_token the page before published. rows names what a page carried
// and returns its pagination block. It returns every row in the order the
// pages gave them, and fails the test unless a page before the last
// published a token, every page that did said more follow, and the last
// published none and said nothing does, within [keysetPageLimit] pages.
func followKeyset[O any](e *harness.Env, s *harness.Session, id harness.ActionID, params map[string]any, rows func(O) ([]string, toolutil.PaginationOutput)) []string {
	e.T.Helper()
	var read []string
	token := ""
	for page := 1; page <= keysetPageLimit; page++ {
		call := withParams(params, map[string]any{"pagination": "keyset", "per_page": 1})
		if token != "" {
			call["page_token"] = token
		}
		got, pagination := rows(harness.Do[O](s, id, call))
		read = append(read, got...)
		if pagination.NextPageToken == "" {
			if pagination.HasMore {
				e.T.Errorf("%s page %d says more follow and publishes no next_page_token to read them by", id, page)
			}
			if page == 1 {
				e.T.Errorf("%s answered its keyset listing in one page of %d rows at per_page 1, want it to name the next page", id, len(got))
			}
			return read
		}
		if !pagination.HasMore {
			e.T.Errorf("%s page %d publishes next_page_token %q and says nothing follows", id, page, pagination.NextPageToken)
		}
		if pagination.NextPageToken == token {
			e.T.Fatalf("%s page %d publishes the token it was asked with, %q, so the listing does not advance", id, page, token)
		}
		token = pagination.NextPageToken
	}
	e.T.Fatalf("%s was still naming a next page after %d pages of a three-row listing: read %q", id, keysetPageLimit, read)
	return nil
}

// labelIssue sets the issue's labels to exactly labels.
func labelIssue(e *harness.Env, project fixture.Project, issue fixture.Issue, labels []string) {
	e.T.Helper()
	options := gl.LabelOptions(labels)
	if _, _, err := e.Client().GL().Issues.UpdateIssue(project.ID, issue.IID, &gl.UpdateIssueOptions{Labels: &options}, gl.WithContext(e.Ctx)); err != nil {
		e.T.Fatalf("labeling issue #%d of project %d with %q: %v", issue.IID, project.ID, labels, err)
	}
}

// assertSameRows fails the test unless got holds exactly the rows of want,
// each once, whatever their order.
func assertSameRows(e *harness.Env, what string, got, want []string) {
	e.T.Helper()
	got, want = slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want))
	if !slices.Equal(got, want) {
		e.T.Errorf("following %s by keyset read %q, want each of %q once", what, got, want)
	}
}

// TestKeysetPagination_ProjectList_FollowsIDAfterToTheEnd creates three
// projects whose names share a stem and follows the project list, filtered to
// that stem and ordered by id as GitLab's keyset on projects requires, one
// project a page. GitLab resumes this listing by id_after, which only the
// next link names.
func TestKeysetPagination_ProjectList_FollowsIDAfterToTheEnd(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) []fixture.Project {
		return []fixture.Project{
			fixture.NewProject(e, fixture.WithNamePrefix("keyset")),
			fixture.NewProject(e, fixture.WithNamePrefix("keyset")),
			fixture.NewProject(e, fixture.WithNamePrefix("keyset")),
		}
	}, func(e *harness.Env, surface harness.Surface, created []fixture.Project) {
		s := e.On(surface)
		want := make([]string, 0, len(created))
		for _, project := range created {
			want = append(want, project.Name)
		}
		// The names differ only in the counter after their last hyphen, so
		// what comes before it is a search only these three match.
		stem := want[0][:strings.LastIndex(want[0], "-")+1]

		got := followKeyset(e, s, actionProjectList, map[string]any{"search": stem, "order_by": "id", "sort": "asc"},
			func(out projects.ListOutput) ([]string, toolutil.PaginationOutput) {
				names := make([]string, 0, len(out.Projects))
				for _, project := range out.Projects {
					names = append(names, project.Name)
				}
				return names, out.Pagination
			})
		assertSameRows(e, "the project list", got, want)
	})
}

// TestKeysetPagination_IssueList_FollowsTheCursorToTheEnd opens three issues
// carrying two labels in a project, beside a fourth carrying one of them, and
// follows its issue list filtered by both labels one issue a page. GitLab
// resumes this listing by an opaque cursor, which only the next link names.
// The label filter goes out again on every page beside the token, as a caller
// sends it: GitLab writes it into its link as labels[] while client-go sends
// it comma-separated as labels, and a request carrying both forms is one Rack
// refuses with 400, so the token has to be the whole query of the call.
func TestKeysetPagination_IssueList_FollowsTheCursorToTheEnd(t *testing.T) {
	e := harness.New(t)

	type world struct {
		project fixture.Project
		labels  []string
		titles  []string
	}
	harness.SurfacesWith(e, func(e *harness.Env) world {
		project := fixture.NewProject(e, fixture.WithNamePrefix("keysetissues"))
		first, second := fixture.NewLabel(e, project, "keyset-a"), fixture.NewLabel(e, project, "keyset-b")
		labels := []string{first.Name, second.Name}
		titles := make([]string, 0, 3)
		for range 3 {
			issue := fixture.NewIssue(e, project, e.Name("keyset issue"))
			labelIssue(e, project, issue, labels)
			titles = append(titles, issue.Title)
		}
		// One label alone does not match the filter, so a listing that lost
		// it on a later page would read this issue too.
		labelIssue(e, project, fixture.NewIssue(e, project, e.Name("keyset issue with one label")), labels[:1])
		return world{project: project, labels: labels, titles: titles}
	}, func(e *harness.Env, surface harness.Surface, w world) {
		s := e.On(surface)
		got := followKeyset(e, s, actionIssueList, map[string]any{"project_id": w.project.IDParam(), "labels": w.labels, "order_by": "created_at", "sort": "asc"},
			func(out issues.ListOutput) ([]string, toolutil.PaginationOutput) {
				titles := make([]string, 0, len(out.Issues))
				for _, issue := range out.Issues {
					titles = append(titles, issue.Title)
				}
				return titles, out.Pagination
			})
		assertSameRows(e, "the issue list", got, w.titles)
	})
}

// TestKeysetPagination_RepositoryTree_FollowsThePageTokenToTheEnd commits two
// files beside the README a project starts with and follows the root of its
// tree one entry a page. GitLab resumes this listing by a Gitaly page_token,
// which only the next link names; the rows it must read are the ones an
// offset listing of the same tree returns in one page.
func TestKeysetPagination_RepositoryTree_FollowsThePageTokenToTheEnd(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		project := fixture.NewProject(e, fixture.WithNamePrefix("keysettree"))
		fixture.CommitFile(e, project, project.DefaultBranch, "alpha.txt", "alpha\n", "chore: add alpha")
		fixture.CommitFile(e, project, project.DefaultBranch, "beta.txt", "beta\n", "chore: add beta")
		return project
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam(), "ref": project.DefaultBranch}
		names := func(out repository.TreeOutput) ([]string, toolutil.PaginationOutput) {
			read := make([]string, 0, len(out.Tree))
			for _, node := range out.Tree {
				read = append(read, node.Name)
			}
			return read, out.Pagination
		}

		whole, _ := names(harness.Do[repository.TreeOutput](s, actionRepositoryTree, withParams(params, map[string]any{"per_page": 100})))
		if len(whole) != 3 {
			e.T.Fatalf("the tree's root lists %q, want the README and the two files committed", whole)
		}
		got := followKeyset(e, s, actionRepositoryTree, params, names)
		assertSameRows(e, "the repository tree", got, whole)
	})
}
