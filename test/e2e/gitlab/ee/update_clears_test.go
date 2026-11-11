// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// update_clears_test.go holds the licensed update actions to sending the
// value that empties a field, as common/update_clears_test.go does for the
// rest. Each scenario sets a field through the server, sends it empty, and
// reads the object back, once per surface. Every one of these fields used to
// be dropped for being empty, so the update reported success with the old
// value (issue 1306); what GitLab does with each empty value was measured on
// 19.5.0-ee before the handlers changed.

package ee

import (
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupwikis"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/integrations"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestUpdateClears_GroupWiki_Content writes a group wiki page, then sends its
// content empty and holds the answer and a fresh read to an empty page.
func TestUpdateClears_GroupWiki_Content(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("gwikiclear"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)
		page := fixture.NewGroupWikiPage(e, group)
		params := map[string]any{"group_id": group.IDParam(), "slug": page.Slug}

		before := harness.Do[groupwikis.Output](s, actionGroupWikiGet, params)
		if before.Content == "" {
			e.T.Fatalf("the group wiki page fixture answered %+v, want content to remove", before)
		}
		cleared := harness.Do[groupwikis.Output](s, actionGroupWikiEdit, withParams(params, map[string]any{"content": ""}))
		read := harness.Do[groupwikis.Output](s, actionGroupWikiGet, params)
		if cleared.Content != "" || read.Content != "" {
			e.T.Errorf("after the removal the group wiki edit answered content %q and the read %q, want none", cleared.Content, read.Content)
		}
	})
}

// TestUpdateClears_Project_DescriptionTemplates gives a project an issue and
// a merge request description template, then sends both empty and holds the
// answer and a fresh read to a project with neither. It also sends
// mirror_branch_regex alone, which GitLab refuses with a 400 whatever its
// value because the route's at-least-one-of list leaves it out, and holds the
// server to refusing it before any request, naming another setting to send
// beside it.
func TestUpdateClears_Project_DescriptionTemplates(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("tmplclear"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}

		set := harness.Do[projects.Output](s, actionProjectUpdate, withParams(params, map[string]any{
			"issues_template": "to be removed", "merge_requests_template": "to be removed",
		}))
		if set.IssuesTemplate == "" || set.MergeRequestsTemplate == "" {
			e.T.Fatalf("project update answered templates %q and %q, want both to remove", set.IssuesTemplate, set.MergeRequestsTemplate)
		}
		cleared := harness.Do[projects.Output](s, actionProjectUpdate, withParams(params, map[string]any{
			"issues_template": "", "merge_requests_template": "",
		}))
		read := harness.Do[projects.Output](s, actionProjectGet, params)
		assertCleared := func(call string, out projects.Output) {
			if out.IssuesTemplate != "" || out.MergeRequestsTemplate != "" {
				e.T.Errorf("project %s after the removal answered templates %q and %q, want neither", call, out.IssuesTemplate, out.MergeRequestsTemplate)
			}
		}
		assertCleared("update", cleared)
		assertCleared("get", read)

		refused := harness.ExpectToolError(s, actionProjectUpdate, withParams(params, map[string]any{"mirror_branch_regex": ""}), "mirror_branch_regex")
		e.T.Logf("mirror_branch_regex alone is refused before any request: %s", firstLine(refused))
	})
}

// TestUpdateClears_PullMirror_BranchRegex configures a pull mirror that
// pulls only the branches matching a regex, then sends the regex empty and
// holds the answer and a fresh read to a mirror of every branch.
func TestUpdateClears_PullMirror_BranchRegex(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("regex-src"), fixture.WithVisibility(gl.PublicVisibility))
	}, func(e *harness.Env, surface harness.Surface, upstream fixture.Project) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("regexclear"))
		params := map[string]any{"project_id": project.IDParam()}

		set := harness.Do[projects.PullMirrorOutput](s, actionPullMirrorConfigure, withParams(params, map[string]any{
			"enabled": true, "url": fixture.RepositoryURL(e, upstream), "mirror_branch_regex": "^main$",
		}))
		if set.MirrorBranchRegex != "^main$" {
			e.T.Fatalf("pull_mirror_configure answered regex %q, want ^main$ before the removal", set.MirrorBranchRegex)
		}
		cleared := harness.Do[projects.PullMirrorOutput](s, actionPullMirrorConfigure, withParams(params, map[string]any{"mirror_branch_regex": ""}))
		read := harness.Do[projects.PullMirrorOutput](s, actionPullMirrorGet, params)
		if cleared.MirrorBranchRegex != "" || read.MirrorBranchRegex != "" {
			e.T.Errorf("after the removal the configure answered regex %q and the read %q, want none", cleared.MirrorBranchRegex, read.MirrorBranchRegex)
		}
		harness.DoVoid(s, actionPullMirrorConfigure, withParams(params, map[string]any{"enabled": false}))
	})
}

// TestUpdateClears_GroupDatadog_Tags sets a group's Datadog integration with
// custom tags, then sends the tags empty and holds the answer and a fresh
// read to an integration with none.
func TestUpdateClears_GroupDatadog_Tags(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("ddtagclear"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)
		params := map[string]any{"group_id": group.IDParam(), "api_key": placeholderDatadogKey, "datadog_site": "datadoghq.com"}

		set := harness.Do[integrations.SetGroupDatadogOutput](s, actionGroupDatadogSet, withParams(params, map[string]any{"datadog_tags": "team:e2e"}))
		if datadogTags(set.Integration) == "" {
			e.T.Fatalf("set answered %+v, want tags to remove", set.Integration)
		}
		cleared := harness.Do[integrations.SetGroupDatadogOutput](s, actionGroupDatadogSet, withParams(params, map[string]any{"datadog_tags": ""}))
		read := harness.Do[integrations.GetGroupDatadogOutput](s, actionGroupDatadogGet, map[string]any{"group_id": group.IDParam()})
		if datadogTags(cleared.Integration) != "" || datadogTags(read.Integration) != "" {
			e.T.Errorf("after the removal the set answered tags %q and the read %q, want none",
				datadogTags(cleared.Integration), datadogTags(read.Integration))
		}
		harness.DoVoid(s, actionGroupDatadogDelete, map[string]any{"group_id": group.IDParam()})
	})
}

// datadogTags reads the custom tags of a published Datadog integration, an
// integration published without its properties holding none.
func datadogTags(item integrations.GroupDatadogItem) string {
	if item.Properties == nil {
		return ""
	}
	return item.Properties.DatadogTags
}
