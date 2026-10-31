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
//
// It also holds the two lists GitLab can never page by keyset to the offset
// pagination they offer instead: the personal access tokens of a group's and
// of a project's service account, which GitLab answers 405 for every keyset
// request, refuse the keyset arguments before sending anything and page by
// page number, in the named order their routes document.
//
// And it holds the lists GitLab pages by keyset in one order alone to that
// order: the four project lists refuse a keyset request in any order but id,
// which GitLab answers 405, before sending it, and the two job lists, which
// GitLab orders by id whatever is sent, refuse order_by and sort as arguments
// they do not take, as the project's job list does include_retried, which its
// route never reads.

package common

import (
	"slices"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupserviceaccounts"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projectserviceaccounts"
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

// listArgument is one argument a list no longer takes, with a value it could
// be sent with.
type listArgument struct {
	name  string
	value any
}

// tokenListKeysetArguments are the three fields of client-go's list options
// the service account token lists no longer take. GitLab pages those tokens
// with paginate_with_strategies over PersonalAccessToken, which has no keyset
// ordering, so it answers pagination=keyset with 405, pages by offset
// otherwise, which reads no page_token, and declares no order_by on either
// route (issue 1165).
var tokenListKeysetArguments = []listArgument{
	{name: "pagination", value: "keyset"},
	{name: "page_token", value: "1"},
	{name: "order_by", value: "created_at"},
}

// tokenNameSuffixes are what the two tokens minted beside the fixture's add to
// its name, which is the account's username, so the three sort by name in the
// order they were made.
var tokenNameSuffixes = []string{"-b", "-c"}

// serviceAccountTokens is a service account holding three tokens, as its
// group's or its project's token list is asked about them.
type serviceAccountTokens struct {
	// params names the account the way its token list takes it.
	params map[string]any
	// names are the three tokens' names in ascending order.
	names []string
}

// withTwoMoreTokens mints the two tokens [tokenNameSuffixes] names beside the
// one the fixture gave account, through mint, and returns the three with the
// parameters scope names the account's owner by.
func withTwoMoreTokens(e *harness.Env, scope map[string]any, account fixture.ServiceAccount, mint func(name string, expiry *gl.ISOTime) error) serviceAccountTokens {
	e.T.Helper()
	names := []string{account.Username}
	for _, suffix := range tokenNameSuffixes {
		name := account.Username + suffix
		expiry := gl.ISOTime(time.Now().Add(serviceAccountTokenLifetime))
		if err := mint(name, &expiry); err != nil {
			e.T.Fatalf("minting token %q for service account %d: %v", name, account.ID, err)
		}
		names = append(names, name)
	}
	return serviceAccountTokens{params: withParams(scope, map[string]any{"service_account_id": account.ID}), names: names}
}

// newGroupServiceAccountTokens creates a group and a service account in it
// holding three tokens.
func newGroupServiceAccountTokens(e *harness.Env) serviceAccountTokens {
	e.T.Helper()
	group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("keysetsa"))
	account := fixture.NewGroupServiceAccount(e, group)
	return withTwoMoreTokens(e, map[string]any{"group_id": group.IDParam()}, account, func(name string, expiry *gl.ISOTime) error {
		_, _, err := e.Client().GL().Groups.CreateServiceAccountPersonalAccessToken(group.ID, account.ID,
			&gl.CreateServiceAccountPersonalAccessTokenOptions{Name: new(name), Scopes: new([]string{"read_api"}), ExpiresAt: expiry},
			gl.WithContext(e.Ctx))
		return err
	})
}

// newProjectServiceAccountTokens creates a project and a service account in
// it holding three tokens.
func newProjectServiceAccountTokens(e *harness.Env) serviceAccountTokens {
	e.T.Helper()
	project := fixture.NewProject(e, fixture.WithNamePrefix("keysetsa"))
	account := fixture.NewProjectServiceAccount(e, project)
	return withTwoMoreTokens(e, map[string]any{"project_id": project.IDParam()}, account, func(name string, expiry *gl.ISOTime) error {
		_, _, err := e.Client().GL().Projects.CreateProjectServiceAccountPersonalAccessToken(project.ID, account.ID,
			&gl.CreateProjectServiceAccountPersonalAccessTokenOptions{Name: new(name), Scopes: new([]string{"read_api"}), ExpiresAt: expiry},
			gl.WithContext(e.Ctx))
		return err
	})
}

// refuseTokenListKeysetArguments sends each of [tokenListKeysetArguments] to a
// token list and holds the answer to a refusal naming the argument, made
// before any request (see [refuseListArguments]). Sent to GitLab, the first
// would be answered 405 and the other two would be ignored.
func refuseTokenListKeysetArguments(e *harness.Env, s *harness.Session, surface harness.Surface, id harness.ActionID, params map[string]any) {
	e.T.Helper()
	refuseListArguments(e, s, surface, id, params, tokenListKeysetArguments)
}

// refuseListArguments sends each of arguments to a list beside params and
// holds the answer to a refusal naming the argument, made before any request.
// Who refuses depends on the surface: the individual tool's schema admits no
// property it does not list, so the SDK refuses the argument before the
// handler runs; execute holds the params to the action's schema and lists the
// ones it does not know; and the meta dispatcher decodes the action's input
// strictly and names the field it does not know.
func refuseListArguments(e *harness.Env, s *harness.Session, surface harness.Surface, id harness.ActionID, params map[string]any, arguments []listArgument) {
	e.T.Helper()
	for _, argument := range arguments {
		call := withParams(params, map[string]any{argument.name: argument.value})
		switch surface {
		case harness.SurfaceIndividual:
			refusal := harness.Refused(s, id, call, harness.FailureInvalidParams)
			assertMentions(e, "the refusal of "+argument.name, refusal, argument.name)
		case harness.SurfaceDynamic:
			harness.ExpectToolError(s, id, call, "Unknown params: "+argument.name+".")
		default:
			harness.ExpectToolError(s, id, call, `unknown field "`+argument.name+`"`)
		}
	}
}

// followOffset reads a listing to its end by page number, one row a page,
// passing next_page back as page while has_more holds. It returns every row in
// the order the pages gave them, and fails the test when a page publishes a
// next_page_token, which an offset page never carries, when a page saying more
// follow names any page but the next, or when a three-row listing is still
// naming a next page after [keysetPageLimit] pages.
func followOffset[O any](e *harness.Env, s *harness.Session, id harness.ActionID, params map[string]any, rows func(O) ([]string, toolutil.PaginationOutput)) []string {
	e.T.Helper()
	var read []string
	page := int64(1)
	for range keysetPageLimit {
		got, pagination := rows(harness.Do[O](s, id, withParams(params, map[string]any{"page": page, "per_page": 1})))
		read = append(read, got...)
		if pagination.NextPageToken != "" {
			e.T.Errorf("%s page %d publishes next_page_token %q, which an offset page never carries", id, page, pagination.NextPageToken)
		}
		if !pagination.HasMore {
			return read
		}
		if pagination.NextPage != page+1 {
			e.T.Fatalf("%s page %d says more follow and names page %d next, want %d", id, page, pagination.NextPage, page+1)
		}
		page = pagination.NextPage
	}
	e.T.Fatalf("%s was still naming a next page after %d pages of a three-row listing: read %q", id, keysetPageLimit, read)
	return nil
}

// assertTokensPageByName follows a token list by page in name_asc order and
// reads it whole in name_desc order, two of the eight named orders the routes
// document, and holds both to the tokens' names: the first in their order and
// the second in reverse, so an order GitLab ignored, which leaves the tokens
// unsorted, reads as a failure rather than as a listing.
func assertTokensPageByName[O any](e *harness.Env, s *harness.Session, id harness.ActionID, tokens serviceAccountTokens, rows func(O) ([]string, toolutil.PaginationOutput)) {
	e.T.Helper()
	ascending := followOffset(e, s, id, withParams(tokens.params, map[string]any{"sort": "name_asc"}), rows)
	if !slices.Equal(ascending, tokens.names) {
		e.T.Errorf("following %s by page in name_asc order read %q, want %q in that order", id, ascending, tokens.names)
	}
	descending, _ := rows(harness.Do[O](s, id, withParams(tokens.params, map[string]any{"sort": "name_desc", "per_page": 100})))
	want := slices.Clone(tokens.names)
	slices.Reverse(want)
	if !slices.Equal(descending, want) {
		e.T.Errorf("%s in name_desc order read %q, want %q in that order", id, descending, want)
	}
}

// TestKeysetPagination_GroupServiceAccountTokens_PageByOffsetAlone holds the
// token list of a group's service account to the pagination GitLab gives it:
// the keyset arguments are refused before any request, and three tokens are
// read one a page by page number, in the name order asked for.
func TestKeysetPagination_GroupServiceAccountTokens_PageByOffsetAlone(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, newGroupServiceAccountTokens, func(e *harness.Env, surface harness.Surface, tokens serviceAccountTokens) {
		s := e.On(surface)
		refuseTokenListKeysetArguments(e, s, surface, actionGroupServiceAccountPATList, tokens.params)
		assertTokensPageByName(e, s, actionGroupServiceAccountPATList, tokens,
			func(out groupserviceaccounts.ListPATOutput) ([]string, toolutil.PaginationOutput) {
				names := make([]string, 0, len(out.Tokens))
				for _, token := range out.Tokens {
					names = append(names, token.Name)
				}
				return names, out.Pagination
			})
	})
}

// TestKeysetPagination_ProjectServiceAccountTokens_PageByOffsetAlone is the
// same for the token list of a project's service account.
func TestKeysetPagination_ProjectServiceAccountTokens_PageByOffsetAlone(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, newProjectServiceAccountTokens, func(e *harness.Env, surface harness.Surface, tokens serviceAccountTokens) {
		s := e.On(surface)
		refuseTokenListKeysetArguments(e, s, surface, actionProjectServiceAccountPATList, tokens.params)
		assertTokensPageByName(e, s, actionProjectServiceAccountPATList, tokens,
			func(out projectserviceaccounts.ListPATOutput) ([]string, toolutil.PaginationOutput) {
				names := make([]string, 0, len(out.Tokens))
				for _, token := range out.Tokens {
					names = append(names, token.Name)
				}
				return names, out.Pagination
			})
	})
}

// keysetOrderRefusal is what the refusal of a project list's keyset request
// in an order other than id names: the one order GitLab pages projects by
// keyset in.
const keysetOrderRefusal = "order_by=id"

// TestKeysetPagination_ProjectLists_RefuseKeysetOutsideIDOrder holds the four
// project lists GitLab pages by keyset only in id order to that order. Asked
// for a keyset page with no order_by, whose default on these routes is
// created_at, or ordered by name, each list refuses before sending anything
// and names order_by=id, where GitLab answered 405 "Keyset pagination is not
// yet available for this type of request"; asked in id order, each is served
// its keyset page. The forks are those of a project created for the scenario,
// and the user lists are the run user's.
func TestKeysetPagination_ProjectLists_RefuseKeysetOutsideIDOrder(t *testing.T) {
	e := harness.New(t)
	rt := e.Runtime()

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("keysetorder"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		lists := []struct {
			id     harness.ActionID
			params map[string]any
		}{
			{id: actionProjectList, params: map[string]any{"owned": true}},
			{id: actionProjectListForks, params: map[string]any{"project_id": project.IDParam()}},
			{id: actionProjectListUserProjects, params: map[string]any{"user_id": rt.Username}},
			{id: actionProjectListUserStarred, params: map[string]any{"user_id": rt.Username}},
		}
		for _, list := range lists {
			keyset := withParams(list.params, map[string]any{"pagination": "keyset", "per_page": 1})
			harness.ExpectToolError(s, list.id, keyset, keysetOrderRefusal)
			harness.ExpectToolError(s, list.id, withParams(keyset, map[string]any{"order_by": "name"}), keysetOrderRefusal)
			harness.DoVoid(s, list.id, withParams(keyset, map[string]any{"order_by": "id", "sort": "desc"}))
		}
	})
}

// TestKeysetPagination_JobLists_RefuseTheOrderingGitLabIgnores holds the two
// job lists to the arguments their routes read. GitLab orders a project's
// jobs and a pipeline's jobs by id, newest first, whatever is sent, and
// declares no order_by or sort on either route, and a project's job list
// declares no include_retried and lists retried jobs whatever it says; so
// each list refuses order_by and sort, and the project's list
// include_retried, before sending anything, and both still list the job of a
// pipeline created for the scenario, the pipeline's list with include_retried
// as well.
func TestKeysetPagination_JobLists_RefuseTheOrderingGitLabIgnores(t *testing.T) {
	e := harness.New(t)

	type world struct {
		project  fixture.Project
		pipeline fixture.Pipeline
	}
	harness.SurfacesWith(e, func(e *harness.Env) world {
		project := fixture.NewProject(e, fixture.WithNamePrefix("keysetjobs"))
		fixture.CommitFile(e, project, project.DefaultBranch, fixture.CIFilePath, neverRunCIYAML, "ci: add a manual job nobody plays "+fixture.SkipCIMarker)
		return world{project: project, pipeline: fixture.NewPipelineNoWait(e, project, project.DefaultBranch)}
	}, func(e *harness.Env, surface harness.Surface, w world) {
		s := e.On(surface)
		byProject := map[string]any{"project_id": w.project.IDParam()}
		byPipeline := withParams(byProject, map[string]any{"pipeline_id": w.pipeline.ID})

		refuseListArguments(e, s, surface, actionJobListProject, byProject, []listArgument{
			{name: "order_by", value: "id"},
			{name: "sort", value: "asc"},
			{name: "include_retried", value: true},
		})
		refuseListArguments(e, s, surface, actionJobList, byPipeline, []listArgument{
			{name: "order_by", value: "id"},
			{name: "sort", value: "asc"},
		})

		jobNamed(e, harness.Do[jobs.ListOutput](s, actionJobListProject, byProject), "held")
		jobNamed(e, harness.Do[jobs.ListOutput](s, actionJobList, withParams(byPipeline, map[string]any{"include_retried": true})), "held")
	})
}
