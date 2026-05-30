package mrchanges

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for merge request changes and diff version actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		mrChangeReadSpec("changes_get", toolutil.RouteAction(client, Get), "gitlab_mr_changes_get"),
		mrChangeReadSpec("raw_diffs", toolutil.RouteAction(client, RawDiffs), "gitlab_mr_raw_diffs"),
		mrChangeReadSpec("diff_versions_list", toolutil.RouteAction(client, ListDiffVersions), "gitlab_mr_diff_versions_list"),
		mrChangeReadSpec("diff_version_get", toolutil.RouteAction(client, GetDiffVersion), "gitlab_mr_diff_version_get"),
	}
}

func mrChangeReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, mrChangeOptions(individualTool))
}

func mrChangeOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Usage:          "Use to execute mrchanges domain action.",
		Tags:           []string{"merge_request", "review", "diff", "changes", "inspect"},
		OpenWorld:      true,
		OwnerPackage:   "mrchanges",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
