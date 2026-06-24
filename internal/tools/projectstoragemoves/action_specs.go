package projectstoragemoves

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical action IDs for cross-linking project storage move actions.
const (
	actionRetrieveAll   = "projectstoragemoves.retrieve_all_project"
	actionRetrieveOne   = "projectstoragemoves.retrieve_project"
	actionGet           = "projectstoragemoves.get_project"
	actionGetForProject = "projectstoragemoves.get_project_for_project"
	actionSchedule      = "projectstoragemoves.schedule_project"
	actionScheduleAll   = "projectstoragemoves.schedule_all_project"
)

// guidanceProjectID describes the project_id parameter shared by the
// project-scoped storage move actions.
func guidanceProjectID() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "scope_project",
		ValueSource:      "Numeric project ID whose repository storage move(s) you want to inspect or schedule.",
		ExampleBinding:   `params.project_id:42`,
		CommonConfusions: []string{"This endpoint takes the numeric project ID only, not a namespace path or the storage move ID."},
	}
}

// guidanceMoveID describes the storage move id parameter.
func guidanceMoveID() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "target_resource",
		ValueSource:      "Numeric ID of the repository storage move, taken from a retrieve_all/retrieve_project listing.",
		ExampleBinding:   `params.id:1`,
		CommonConfusions: []string{"This is the storage move record ID, not the project ID."},
	}
}

// guidanceDestination describes the destination_storage_name parameter.
func guidanceDestination() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "config_value",
		ValueSource:      "Name of a Gitaly storage shard already configured on the instance to move the repository to.",
		ExampleBinding:   `params.destination_storage_name:"storage2"`,
		CommonConfusions: []string{"Must reference an existing storage shard; cannot equal the shard the project currently lives on. Omit to let GitLab pick a shard by weight."},
	}
}

// guidanceSource describes the source_storage_name parameter.
func guidanceSource() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "config_value",
		ValueSource:      "Name of the Gitaly storage shard to drain; every project on this shard is scheduled to move.",
		ExampleBinding:   `params.source_storage_name:"default"`,
		CommonConfusions: []string{"This is the shard to move projects away from, not the destination."},
	}
}

// ActionSpecs returns canonical specs for project repository storage move actions.
// Project repository storage moves are an admin-only, self-managed feature that
// migrates project repositories between Gitaly storage shards.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		retrieveAllSpec(client),
		retrieveForProjectSpec(client),
		getSpec(client),
		getForProjectSpec(client),
		scheduleSpec(client),
		scheduleAllSpec(client),
	}
}

func retrieveAllSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	opts := baseOptions("gitlab_retrieve_all_project_storage_moves")
	opts.Usage = "List every project repository storage move on the instance (admin-only, self-managed). Use to audit in-flight or completed Gitaly shard migrations across all projects before scheduling new ones."
	opts.Aliases = []string{"gitlab_retrieve_all_project_storage_moves", "list all storage moves", "all repository storage moves", "list gitaly shard migrations"}
	opts.RelatedActions = []string{actionGet, actionRetrieveOne, actionScheduleAll}
	opts.IndividualTool.Description = "List all project repository storage moves on the instance. Returns each move's id, state, source and destination storage shards, and project. Admin-only and self-managed only. See also: gitlab_get_project_storage_move, gitlab_schedule_all_project_storage_moves."
	return toolutil.NewReadActionSpec("retrieve_all_project", toolutil.RouteAction(client, RetrieveAll), opts)
}

func retrieveForProjectSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	opts := baseOptions("gitlab_retrieve_project_storage_moves")
	opts.Usage = "List repository storage moves for one project (admin-only, self-managed). Use to track that project's Gitaly shard migration history or in-flight move state."
	opts.Aliases = []string{"gitlab_retrieve_project_storage_moves", "list storage moves for project", "project repository storage moves", "project shard migration history"}
	opts.RelatedActions = []string{actionGetForProject, actionSchedule, actionRetrieveAll}
	opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{"project_id": guidanceProjectID()}
	opts.IndividualTool.Description = "List repository storage moves for a single project. Returns each move's id, state, source and destination storage shards, and project. Admin-only and self-managed only. See also: gitlab_get_project_storage_move_for_project, gitlab_schedule_project_storage_move."
	return toolutil.NewReadActionSpec("retrieve_project", toolutil.RouteAction(client, RetrieveForProject), opts)
}

func getSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	opts := baseOptions("gitlab_get_project_storage_move")
	opts.Usage = "Get one repository storage move by its global move id (admin-only, self-managed). Use after a retrieve_all listing to inspect a specific move's state and shards."
	opts.Aliases = []string{"gitlab_get_project_storage_move", "get storage move", "show repository storage move", "storage move status"}
	opts.RelatedActions = []string{actionRetrieveAll, actionGetForProject}
	opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{"id": guidanceMoveID()}
	opts.IndividualTool.Description = "Get a single project repository storage move by its global move id. Returns the move's id, state, source and destination storage shards, and project. Admin-only and self-managed only. See also: gitlab_retrieve_all_project_storage_moves."
	return toolutil.NewReadActionSpec("get_project", toolutil.RouteAction(client, Get), opts)
}

func getForProjectSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	opts := baseOptions("gitlab_get_project_storage_move_for_project")
	opts.Usage = "Get one repository storage move scoped to a specific project by project_id plus move id (admin-only, self-managed). Use to inspect a known move on a known project."
	opts.Aliases = []string{"gitlab_get_project_storage_move_for_project", "get project storage move", "show project repository storage move"}
	opts.RelatedActions = []string{actionRetrieveOne, actionGet, actionSchedule}
	opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{"project_id": guidanceProjectID(), "id": guidanceMoveID()}
	opts.IndividualTool.Description = "Get a single repository storage move for a specific project by project_id and move id. Returns the move's id, state, source and destination storage shards, and project. Admin-only and self-managed only. See also: gitlab_retrieve_project_storage_moves."
	return toolutil.NewReadActionSpec("get_project_for_project", toolutil.RouteAction(client, GetForProject), opts)
}

func scheduleSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	opts := baseOptions("gitlab_schedule_project_storage_move")
	opts.Usage = "Schedule a single project's repository to migrate to another Gitaly storage shard (admin-only, self-managed). Omit destination_storage_name to let GitLab pick a shard by configured weight."
	opts.Aliases = []string{"gitlab_schedule_project_storage_move", "move project repository storage", "migrate project to storage shard", "schedule storage move"}
	opts.RelatedActions = []string{actionRetrieveOne, actionGetForProject, actionScheduleAll}
	opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{"project_id": guidanceProjectID(), "destination_storage_name": guidanceDestination()}
	opts.IndividualTool.Description = "Schedule a repository storage move for a single project to another Gitaly storage shard. Returns the created move's id, state, source and destination shards, and project. Admin-only and self-managed only. See also: gitlab_retrieve_project_storage_moves, gitlab_schedule_all_project_storage_moves."
	return toolutil.NewCreateActionSpec("schedule_project", toolutil.RouteAction(client, Schedule), opts)
}

func scheduleAllSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	opts := baseOptions("gitlab_schedule_all_project_storage_moves")
	opts.Usage = "Schedule every project on one Gitaly storage shard to migrate to another (admin-only, self-managed). Bulk operation used to drain a shard; may enqueue many concurrent moves."
	opts.Aliases = []string{"gitlab_schedule_all_project_storage_moves", "drain storage shard", "migrate all projects off shard", "bulk schedule storage moves"}
	opts.RelatedActions = []string{actionRetrieveAll, actionSchedule}
	opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{"source_storage_name": guidanceSource(), "destination_storage_name": guidanceDestination()}
	opts.IndividualTool.Description = "Schedule repository storage moves for all projects on a Gitaly storage shard. Drains source_storage_name onto destination_storage_name. Returns a confirmation message. Admin-only and self-managed only. See also: gitlab_retrieve_all_project_storage_moves, gitlab_schedule_project_storage_move."
	return toolutil.NewCreateActionSpec("schedule_all_project", toolutil.RouteAction(client, ScheduleAll), opts)
}

// baseOptions returns the shared ActionSpecOptions for project storage move
// actions, leaving action-specific Usage/Aliases/RelatedActions to callers.
func baseOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"storage_move", "project"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "projectstoragemoves",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
