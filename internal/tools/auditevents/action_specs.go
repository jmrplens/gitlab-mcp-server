package auditevents

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical action IDs for audit event routes. The catalog projects these
// specs under the gitlab_audit_event group, so cross-references in
// RelatedActions use the audit_event.<action> form.
const (
	actionListInstance = "audit_event.list_instance"
	actionGetInstance  = "audit_event.get_instance"
	actionListGroup    = "audit_event.list_group"
	actionGetGroup     = "audit_event.get_group"
	actionListProject  = "audit_event.list_project"
	actionGetProject   = "audit_event.get_project"
)

// ActionSpecs returns canonical specs for audit event actions. The
// instance/group/project list and get routes are projected into the
// dynamic, meta, individual, and audit surfaces by the action catalog
// (ADR-0004). Each spec carries action-specific usage guidance,
// natural-language aliases, canonical related actions, and a
// "Returns: … See also: …" individual-tool description so the 1:1
// discovery metadata mirrors the GitLab audit_events API surface.
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

// auditEventReadSpec builds a read-only [toolutil.ActionSpec] for an audit
// event action and fills in action-specific usage, natural-language
// aliases, canonical related actions, and the "Returns: … See also: …"
// individual-tool description. All audit event tools are read-only,
// open-world, and gated to Premium/Ultimate (premium edition).
func auditEventReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Usage:          "Use to execute auditevents domain action.",
		Tags:           []string{"audit", "event"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "auditevents",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	decorateAuditEventMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

// decorateAuditEventMeta overlays action-specific discovery metadata
// (usage, aliases, related actions, and the individual-tool description)
// onto the shared read-only options for the named audit event tool.
func decorateAuditEventMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	switch individualTool {
	case "gitlab_list_instance_audit_events":
		options.Usage = "List instance-level audit events across the whole GitLab instance. Use when an administrator asks for a security or compliance trail spanning all groups and projects. Filter with created_after and created_before (ISO 8601 dates) and paginate with page and per_page. Requires administrator access on self-managed Premium/Ultimate."
		options.Aliases = []string{"list instance audit events", "instance audit log", "show admin audit trail", "list all audit events"}
		options.RelatedActions = []string{actionGetInstance, actionListGroup, actionListProject}
		options.IndividualTool.Description = "List instance-level audit events (administrator only). Returns: audit events with author, entity, event name/type, details, timestamps, and pagination metadata. See also: gitlab_get_instance_audit_event, gitlab_list_group_audit_events, gitlab_list_project_audit_events."
	case "gitlab_get_instance_audit_event":
		options.Usage = "Get one exact instance-level audit event by event_id. Use after a list result or when the prompt already names a concrete instance audit event ID. Requires administrator access on self-managed Premium/Ultimate."
		options.Aliases = []string{"get instance audit event", "show instance audit event", "fetch admin audit event"}
		options.RelatedActions = []string{actionListInstance, actionGetGroup, actionGetProject}
		options.IndividualTool.Description = "Get a single instance-level audit event by ID (administrator only). Returns: the audit event with author, entity, event name/type, target details, IP address, and timestamp. See also: gitlab_list_instance_audit_events, gitlab_get_group_audit_event, gitlab_get_project_audit_event."
	case "gitlab_list_group_audit_events":
		options.Usage = "List audit events for one group and its subgroups/projects. Use when the prompt asks for a group's security or compliance trail. Identify the group with group_id (ID or URL-encoded path), filter with created_after and created_before (ISO 8601 dates), and paginate with page and per_page. Requires the Owner role plus Premium/Ultimate."
		options.Aliases = []string{"list group audit events", "group audit log", "show group audit trail"}
		options.RelatedActions = []string{actionGetGroup, actionListProject, actionListInstance}
		options.IndividualTool.Description = "List group-level audit events for a group. Returns: audit events with author, entity, event name/type, details, timestamps, and pagination metadata. See also: gitlab_get_group_audit_event, gitlab_list_project_audit_events, gitlab_list_instance_audit_events."
	case "gitlab_get_group_audit_event":
		options.Usage = "Get one exact group-level audit event by group_id plus event_id. Use after a group audit list result or when the prompt already names a concrete group audit event ID. Requires the Owner role plus Premium/Ultimate."
		options.Aliases = []string{"get group audit event", "show group audit event", "fetch group audit event"}
		options.RelatedActions = []string{actionListGroup, actionGetProject, actionGetInstance}
		options.IndividualTool.Description = "Get a single group-level audit event by group_id and ID. Returns: the audit event with author, entity, event name/type, target details, IP address, and timestamp. See also: gitlab_list_group_audit_events, gitlab_get_project_audit_event, gitlab_get_instance_audit_event."
	case "gitlab_list_project_audit_events":
		options.Usage = "List audit events for one project. Use when the prompt asks for a project's security or compliance trail. Identify the project with project_id (ID or URL-encoded path), filter with created_after and created_before (ISO 8601 dates), and paginate with page and per_page. Requires the Maintainer role plus Premium/Ultimate."
		options.Aliases = []string{"list project audit events", "project audit log", "show project audit trail"}
		options.RelatedActions = []string{actionGetProject, actionListGroup, actionListInstance}
		options.IndividualTool.Description = "List project-level audit events for a project. Returns: audit events with author, entity, event name/type, details, timestamps, and pagination metadata. See also: gitlab_get_project_audit_event, gitlab_list_group_audit_events, gitlab_list_instance_audit_events."
	case "gitlab_get_project_audit_event":
		options.Usage = "Get one exact project-level audit event by project_id plus event_id. Use after a project audit list result or when the prompt already names a concrete project audit event ID. Requires the Maintainer role plus Premium/Ultimate."
		options.Aliases = []string{"get project audit event", "show project audit event", "fetch project audit event"}
		options.RelatedActions = []string{actionListProject, actionGetGroup, actionGetInstance}
		options.IndividualTool.Description = "Get a single project-level audit event by project_id and ID. Returns: the audit event with author, entity, event name/type, target details, IP address, and timestamp. See also: gitlab_list_project_audit_events, gitlab_get_group_audit_event, gitlab_get_instance_audit_event."
	}
}
