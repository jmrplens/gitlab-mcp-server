package projectmirrors

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for project remote mirror actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		mirrorReadSpec("mirror_list", toolutil.RouteAction(client, List), "gitlab_list_project_mirrors"),
		mirrorReadSpec("mirror_get", toolutil.RouteAction(client, Get), "gitlab_get_project_mirror"),
		mirrorReadSpec("mirror_get_public_key", toolutil.RouteAction(client, GetPublicKey), "gitlab_get_project_mirror_public_key"),
		mirrorCreateSpec("mirror_add", toolutil.RouteAction(client, Add), "gitlab_add_project_mirror"),
		mirrorUpdateSpec("mirror_edit", toolutil.RouteAction(client, Edit), "gitlab_edit_project_mirror"),
		mirrorDeleteSpec("mirror_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_delete_project_mirror"),
		mirrorForcePushSpec(client),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("mirror %d from project %s", input.MirrorID, input.ProjectID))
	return out, nil
}

func forcePushOutput(ctx context.Context, client *gitlabclient.Client, input ForcePushInput) (toolutil.DeleteOutput, error) {
	if err := ForcePushUpdate(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Message: fmt.Sprintf("Force push update triggered for mirror %d in project %s", input.MirrorID, input.ProjectID)}, nil
}

// Canonical action IDs for project remote mirror cross-references. Mirror
// actions are projected under the gitlab_project meta-tool, so their domain
// prefix is "project" (see actioncatalog DomainFromToolName).
const (
	actionMirrorList         = "project.mirror_list"
	actionMirrorGet          = "project.mirror_get"
	actionMirrorGetPublicKey = "project.mirror_get_public_key"
	actionMirrorAdd          = "project.mirror_add"
	actionMirrorEdit         = "project.mirror_edit"
	actionMirrorDelete       = "project.mirror_delete"
	actionMirrorForcePush    = "project.mirror_force_push"
	actionPullMirrorGet      = "project.pull_mirror_get"
)

func mirrorReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, mirrorOptions(individualTool))
}

func mirrorCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, mirrorOptions(individualTool))
}

func mirrorUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, mirrorOptions(individualTool))
}

func mirrorDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, mirrorOptions(individualTool))
}

func mirrorForcePushSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	individualDestructive := false
	options := mirrorOptions("gitlab_force_push_mirror_update")
	options.IndividualTool.AnnotationOverrides.Destructive = &individualDestructive
	return toolutil.NewDeleteActionSpec("mirror_force_push", toolutil.DestructiveAction(client, forcePushOutput), options)
}

// projectIDGuidance is the shared parameter guidance for the project_id input,
// which scopes every mirror action to its owning project.
var projectIDGuidance = toolutil.ParameterGuidance{
	SemanticRole:     "scope_project",
	ValueSource:      "Project ID or full namespace path that owns the remote mirror.",
	ExampleBinding:   `params.project_id:"group/project"`,
	CommonConfusions: []string{"Use the project that owns the mirror, not a group path or the remote repository's project."},
}

// mirrorIDGuidance is the shared parameter guidance for the mirror_id input,
// the numeric remote-mirror identifier returned by gitlab_list_project_mirrors.
var mirrorIDGuidance = toolutil.ParameterGuidance{
	SemanticRole:     "mirror_id",
	ValueSource:      "Numeric remote-mirror id from a prior gitlab_list_project_mirrors response.",
	ExampleBinding:   "params.mirror_id:42",
	CommonConfusions: []string{"mirror_id is the remote-mirror's own id, not the project id and not the mirror URL."},
}

// mirrorActionMeta maps each individual mirror tool to its discovery metadata.
var mirrorActionMeta = map[string]toolutil.ActionMetaEntry{
	"gitlab_list_project_mirrors": {
		Usage:   "List every push (remote) mirror configured on a project, with their enabled state, URL, and last-update status. Use to discover mirror_id values before getting, editing, deleting, or force-pushing a specific mirror.",
		Aliases: []string{"list project mirrors", "show remote mirrors", "list push mirrors"},
		Related: []string{actionMirrorGet, actionMirrorAdd, actionPullMirrorGet},
		Guidance: map[string]toolutil.ParameterGuidance{
			"project_id": projectIDGuidance,
			"order_by": {
				ValueSource:      "Column to order keyset-paginated results by, such as id.",
				ExampleBinding:   `params.order_by:"id"`,
				CommonConfusions: []string{"order_by only applies when pagination='keyset'. Combine it with sort."},
			},
		},
		Description: "List a project's push (remote) mirrors. Returns: each mirror with id, enabled state, redacted URL, last-update status, and pagination metadata. See also: gitlab_get_project_mirror, gitlab_add_project_mirror, gitlab_project_pull_mirror_get.",
	},
	"gitlab_get_project_mirror": {
		Usage:   "Get one push mirror by project_id plus mirror_id, including its enabled state, auth method, branch filtering, and last-update status and error. Use after listing mirrors when the target mirror_id is already known.",
		Aliases: []string{"get project mirror", "show remote mirror", "fetch push mirror"},
		Related: []string{actionMirrorList, actionMirrorEdit, actionMirrorGetPublicKey, actionMirrorForcePush},
		Guidance: map[string]toolutil.ParameterGuidance{
			"project_id": projectIDGuidance,
			"mirror_id":  mirrorIDGuidance,
		},
		Description: "Get a single push mirror of a project by mirror id. Returns: the mirror with enabled state, redacted URL, auth method, branch filtering, and last-update status/error. See also: gitlab_list_project_mirrors, gitlab_edit_project_mirror, gitlab_force_push_mirror_update.",
	},
	"gitlab_get_project_mirror_public_key": {
		Usage:   "Get the SSH public key GitLab uses to authenticate an SSH-based push mirror, so it can be added as a deploy key on the remote. Only mirrors with ssh_public_key auth expose a public key.",
		Aliases: []string{"get mirror public key", "show mirror ssh key", "fetch remote mirror public key"},
		Related: []string{actionMirrorGet, actionMirrorList, actionMirrorEdit},
		Guidance: map[string]toolutil.ParameterGuidance{
			"project_id": projectIDGuidance,
			"mirror_id":  mirrorIDGuidance,
		},
		Description: "Get the SSH public key for an SSH-authenticated push mirror. Returns: the public key to register on the remote. See also: gitlab_get_project_mirror, gitlab_edit_project_mirror.",
	},
	"gitlab_add_project_mirror": {
		Usage:   "Create a new push (remote) mirror on a project so commits are mirrored to an external Git URL. Requires GitLab Premium/Ultimate and Maintainer+ role. Supply credentials inline in the URL or via SSH auth.",
		Aliases: []string{"add project mirror", "create push mirror", "set up remote mirror"},
		Related: []string{actionMirrorList, actionMirrorEdit, actionMirrorGetPublicKey},
		Guidance: map[string]toolutil.ParameterGuidance{
			"project_id": projectIDGuidance,
			"url": {
				SemanticRole:     "mirror_url",
				ValueSource:      "Remote Git URL to push to, optionally with inline credentials in the userinfo component.",
				ExampleBinding:   `params.url:"https://example.com/repo.git"`,
				CommonConfusions: []string{"Inline credentials are secrets. Do not mirror a project to itself."},
			},
		},
		Description: "Create a push (remote) mirror on a project. Returns: the created mirror with id, enabled state, redacted URL, and update status. See also: gitlab_list_project_mirrors, gitlab_edit_project_mirror, gitlab_get_project_mirror_public_key.",
	},
	"gitlab_edit_project_mirror": {
		Usage:   "Update an existing push mirror's attributes: enable/disable it, toggle keep_divergent_refs or only_protected_branches, change the branch regex, or switch auth method. Identify the mirror with project_id plus mirror_id.",
		Aliases: []string{"edit project mirror", "update push mirror", "enable or disable remote mirror"},
		Related: []string{actionMirrorGet, actionMirrorList, actionMirrorForcePush},
		Guidance: map[string]toolutil.ParameterGuidance{
			"project_id": projectIDGuidance,
			"mirror_id":  mirrorIDGuidance,
		},
		Description: "Update a push mirror's attributes (enabled, branch filtering, divergent refs, auth method). Returns: the updated mirror. See also: gitlab_get_project_mirror, gitlab_force_push_mirror_update.",
	},
	"gitlab_delete_project_mirror": {
		Usage:   "Permanently delete a push mirror from a project. Destructive and irreversible. Confirm project_id and mirror_id before calling.",
		Aliases: []string{"delete project mirror", "remove push mirror", "delete remote mirror"},
		Related: []string{actionMirrorGet, actionMirrorList, actionMirrorEdit},
		Guidance: map[string]toolutil.ParameterGuidance{
			"project_id": projectIDGuidance,
			"mirror_id":  mirrorIDGuidance,
		},
		Description: "Delete a push mirror from a project permanently. Returns: a success confirmation naming the mirror and project. See also: gitlab_get_project_mirror, gitlab_edit_project_mirror.",
	},
	"gitlab_force_push_mirror_update": {
		Usage:   "Trigger an immediate update of a push mirror instead of waiting for the next scheduled sync. The mirror must be enabled and not in a failed-auth state.",
		Aliases: []string{"force push mirror update", "sync remote mirror now", "trigger mirror update"},
		Related: []string{actionMirrorGet, actionMirrorList, actionMirrorEdit},
		Guidance: map[string]toolutil.ParameterGuidance{
			"project_id": projectIDGuidance,
			"mirror_id":  mirrorIDGuidance,
		},
		Description: "Trigger an immediate update of a push mirror. Returns: a confirmation that the update was triggered. See also: gitlab_get_project_mirror, gitlab_edit_project_mirror.",
	},
}

func mirrorOptions(individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute projectmirrors domain action.", Tags: []string{"project", "mirror"},
		RelatedActions: []string{actionPullMirrorGet, "repository.commit_list"},
		OpenWorld:      true,
		OwnerPackage:   "projectmirrors",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	if meta, ok := mirrorActionMeta[individualTool]; ok {
		toolutil.ApplyActionMeta(&options, meta)
	}
	switch individualTool {
	case "gitlab_add_project_mirror", "gitlab_edit_project_mirror":
		// https://docs.gitlab.com/api/remote_mirrors/ accepts exactly these two.
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaEnumOverride("auth_method", "ssh_public_key", "password"),
		}
	}
	return options
}
