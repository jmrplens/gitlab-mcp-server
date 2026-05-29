package auditevents

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for audit event actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		auditEventReadSpec("list_instance", toolutil.RouteAction(client, ListInstance), "gitlab_list_instance_audit_events"),
		auditEventReadSpec("get_instance", toolutil.RouteAction(client, GetInstance), "gitlab_get_instance_audit_event"),
		auditEventReadSpec("list_group", toolutil.RouteAction(client, ListGroup), "gitlab_list_group_audit_events"),
		auditEventReadSpec("get_group", toolutil.RouteAction(client, GetGroup), "gitlab_get_group_audit_event"),
		auditEventReadSpec("list_project", toolutil.RouteAction(client, ListProject), "gitlab_list_project_audit_events"),
		auditEventReadSpec("get_project", toolutil.RouteAction(client, GetProject), "gitlab_get_project_audit_event"),
	}
}

func auditEventReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute auditevents domain action.", Tags: []string{"audit", "event"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "auditevents",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}
