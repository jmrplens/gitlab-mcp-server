package tools

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/accessrequests"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/accesstokens"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncompat"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/adminspecs"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/attestations"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/auditevents"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/avatar"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/awardemoji"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/badges"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/boards"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/branches"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/branchrules"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/cicatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/cilint"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/civariables"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/ciyamltemplates"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/commitdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/compliancepolicy"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/containerregistry"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/customemoji"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dependencies"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/deploykeys"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/deploymentmergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/deployments"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/deploytokens"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dockerfiletemplates"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dorametrics"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/enterpriseusers"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/environments"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/epicdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/epicissues"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/epicnotes"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/epics"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/events"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/externalstatuschecks"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/featureflags"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/ffuserlists"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/files"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/freezeperiods"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/geo"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/gitignoretemplates"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupanalytics"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupboards"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupcredentials"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupepicboards"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupimportexport"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupiterations"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/grouplabels"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupldap"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupmarkdownuploads"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupmembers"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupmilestones"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupprotectedbranches"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupprotectedenvs"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/grouprelationsexport"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupreleases"
	grouptools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupsaml"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupscim"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupserviceaccounts"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupsshcerts"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupstoragemoves"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupvariables"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupwikis"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/impersonationtokens"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/instancevariables"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/integrations"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/invites"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/issuediscussions"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/issuelinks"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/issuenotes"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/issuestatistics"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/jobtokenscope"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/keys"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/labels"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/licensetemplates"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/markdown"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/memberroles"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/members"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mergetrains"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/modelregistry"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mrapprovals"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mrapprovalsettings"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mrchanges"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mrcontextcommits"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mrdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mrdraftnotes"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mrnotes"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/namespaces"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/notifications"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/orbit"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/packages"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/pages"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/pipelineschedules"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/pipelinetriggers"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projectaliases"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projectimportexport"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projectiterations"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projectmirrors"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projectserviceaccounts"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projectstatistics"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projectstoragemoves"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projecttemplates"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/protectedenvs"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/protectedpackages"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/releaselinks"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/releases"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/repository"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/repositorysubmodules"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/resourceevents"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/resourcegroups"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/runners"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/samplingtools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/search"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/securityattributes"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/securitycategories"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/securityfindings"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/securitysettings"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/snippetdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/snippetnotes"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/snippets"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/snippetstoragemoves"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/tags"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/todos"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/uploads"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/useremails"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/usergpgkeys"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/users"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/vulnerabilities"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/wikis"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/workitems"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecGroup contains specs owned by one catalog group.
type ActionSpecGroup = actioncatalog.CatalogGroupSpec

type actionSpecGroupBuilder func(*gitlabclient.Client, bool) []ActionSpecGroup

//go:generate go run ../../cmd/gen_action_catalog_manifest/

// CollectActionSpecs gathers canonical specs from domain-local builders.
func CollectActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	groups := make([]ActionSpecGroup, 0)
	for _, build := range actionSpecGroupBuilders() {
		groups = append(groups, build(client, enterprise)...)
	}
	return sortedActionSpecGroups(actioncompat.ApplyToGroupSpecs(groups))
}

func buildAdminActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_admin", adminspecs.ActionSpecs(client))
}

func buildAccessActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 48)
	specs = append(specs, accesstokens.ActionSpecs(client)...)
	specs = append(specs, deploytokens.ActionSpecs(client)...)
	specs = append(specs, deploykeys.ActionSpecs(client)...)
	specs = append(specs, accessrequests.ActionSpecs(client)...)
	specs = append(specs, invites.ActionSpecs(client)...)
	return actionSpecGroup("gitlab_access", specs)
}

func buildAnalyzeActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_analyze", samplingtools.ActionSpecs(client))
}

func buildOrbitActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	if !enterprise || client == nil || !client.IsGitLabDotCom() {
		return nil
	}
	return actionSpecGroup("gitlab_orbit", orbit.ActionSpecs(client))
}

func buildAttestationActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	if !enterprise {
		return nil
	}
	return actionSpecGroup("gitlab_attestation", attestations.ActionSpecs(client))
}

func buildAuditEventActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	if !enterprise {
		return nil
	}
	return actionSpecGroup("gitlab_audit_event", auditevents.ActionSpecs(client))
}

func buildBranchActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := append(branches.ActionSpecs(client), branchrules.ActionSpecs(client)...)
	return actionSpecGroup("gitlab_branch", specs)
}

func buildCICatalogActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_ci_catalog", cicatalog.ActionSpecs(client))
}

func buildCIVariableActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 15)
	specs = append(specs, civariables.ActionSpecs(client)...)
	specs = append(specs, groupvariables.ActionSpecs(client)...)
	specs = append(specs, instancevariables.ActionSpecs(client)...)
	return actionSpecGroup("gitlab_ci_variable", specs)
}

func buildCompliancePolicyActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	if !enterprise {
		return nil
	}
	return actionSpecGroup("gitlab_compliance_policy", compliancepolicy.ActionSpecs(client))
}

func buildCustomEmojiActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_custom_emoji", customemoji.ActionSpecs(client))
}

func buildDependencyActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	if !enterprise {
		return nil
	}
	return actionSpecGroup("gitlab_dependency", dependencies.ActionSpecs(client))
}

func buildDORAMetricsActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	if !enterprise {
		return nil
	}
	return actionSpecGroup("gitlab_dora_metrics", dorametrics.ActionSpecs(client))
}

func buildEnvironmentActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 23)
	specs = append(specs, environments.ActionSpecs(client)...)
	specs = append(specs, protectedenvs.ActionSpecs(client)...)
	specs = append(specs, freezeperiods.ActionSpecs(client)...)
	specs = append(specs, deployments.ActionSpecs(client)...)
	specs = append(specs, deploymentmergerequests.ActionSpecs(client)...)
	return actionSpecGroup("gitlab_environment", specs)
}

func buildEnterpriseUserActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	if !enterprise {
		return nil
	}
	return actionSpecGroup("gitlab_enterprise_user", enterpriseusers.ActionSpecs(client))
}

func buildExternalStatusCheckActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	if !enterprise {
		return nil
	}
	return actionSpecGroup("gitlab_external_status_check", externalstatuschecks.ActionSpecs(client))
}

func buildFeatureFlagsActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 10)
	specs = append(specs, featureflags.ActionSpecs(client)...)
	specs = append(specs, ffuserlists.ActionSpecs(client)...)
	return actionSpecGroup("gitlab_feature_flags", specs)
}

func buildGeoActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	if !enterprise {
		return nil
	}
	return actionSpecGroup("gitlab_geo", geo.ActionSpecs(client))
}

func buildGroupActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 96)
	specs = append(specs, grouptools.ActionSpecs(client)...)
	specs = append(specs, badges.GroupActionSpecs(client)...)
	specs = append(specs, groupmembers.ActionSpecs(client)...)
	specs = append(specs, grouplabels.ActionSpecs(client)...)
	specs = append(specs, groupmilestones.ActionSpecs(client)...)
	specs = append(specs, groupboards.ActionSpecs(client)...)
	specs = append(specs, groupmarkdownuploads.ActionSpecs(client)...)
	specs = append(specs, groupimportexport.ActionSpecs(client)...)
	specs = append(specs, grouprelationsexport.ActionSpecs(client)...)
	specs = append(specs, groupreleases.ActionSpecs(client)...)
	specs = append(specs, issues.GroupActionSpecs(client)...)
	if !enterprise {
		return actionSpecGroup("gitlab_group", specs)
	}
	specs = append(specs, groupserviceaccounts.ActionSpecs(client)...)
	specs = append(specs, epicdiscussions.ActionSpecs(client)...)
	specs = append(specs, epics.ActionSpecs(client)...)
	specs = append(specs, epicissues.ActionSpecs(client)...)
	specs = append(specs, epicnotes.ActionSpecs(client)...)
	specs = append(specs, groupepicboards.ActionSpecs(client)...)
	specs = append(specs, groupwikis.ActionSpecs(client)...)
	specs = append(specs, groupprotectedbranches.ActionSpecs(client)...)
	specs = append(specs, groupprotectedenvs.ActionSpecs(client)...)
	specs = append(specs, groupldap.ActionSpecs(client)...)
	specs = append(specs, groupsaml.ActionSpecs(client)...)
	specs = append(specs, groupanalytics.ActionSpecs(client)...)
	specs = append(specs, groupcredentials.ActionSpecs(client)...)
	specs = append(specs, groupsshcerts.ActionSpecs(client)...)
	specs = append(specs, securitysettings.GroupActionSpecs(client)...)
	return actionSpecGroup("gitlab_group", specs)
}

func buildGroupSCIMActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	if !enterprise {
		return nil
	}
	return actionSpecGroup("gitlab_group_scim", groupscim.ActionSpecs(client))
}

func buildIssueActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 63)
	specs = append(specs, issues.ActionSpecs(client)...)
	specs = append(specs, issuenotes.ActionSpecs(client)...)
	specs = append(specs, issuelinks.ActionSpecs(client)...)
	specs = append(specs, issuediscussions.ActionSpecs(client)...)
	specs = append(specs, issuestatistics.ActionSpecs(client)...)
	specs = append(specs, workitems.ActionSpecs(client)...)
	specs = append(specs, awardemoji.IssueActionSpecs(client)...)
	specs = append(specs, resourceevents.IssueActionSpecs(client)...)
	if enterprise {
		specs = append(specs, projectiterations.IssueActionSpecs(client)...)
		specs = append(specs, groupiterations.IssueActionSpecs(client)...)
	}
	return actionSpecGroup("gitlab_issue", specs)
}

func buildJobActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 25)
	specs = append(specs, jobs.ActionSpecs(client)...)
	specs = append(specs, jobtokenscope.ActionSpecs(client)...)
	return actionSpecGroup("gitlab_job", specs)
}

func buildMergeRequestActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 58)
	specs = append(specs, mergerequests.ActionSpecs(client)...)
	specs = append(specs, mrapprovals.ActionSpecs(client)...)
	specs = append(specs, mrapprovalsettings.ActionSpecs(client)...)
	specs = append(specs, mrcontextcommits.ActionSpecs(client)...)
	specs = append(specs, awardemoji.MergeRequestActionSpecs(client)...)
	specs = append(specs, resourceevents.MergeRequestActionSpecs(client)...)
	return actionSpecGroup("gitlab_merge_request", specs)
}

func buildMergeTrainActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	if !enterprise {
		return nil
	}
	return actionSpecGroup("gitlab_merge_train", mergetrains.ActionSpecs(client))
}

func buildMemberRoleActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	if !enterprise {
		return nil
	}
	return actionSpecGroup("gitlab_member_role", memberroles.ActionSpecs(client))
}

func buildModelRegistryActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_model_registry", modelregistry.ActionSpecs(client))
}

func buildMRReviewActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 23)
	specs = append(specs, mrnotes.ActionSpecs(client)...)
	specs = append(specs, mrdiscussions.ActionSpecs(client)...)
	specs = append(specs, mrchanges.ActionSpecs(client)...)
	specs = append(specs, mrdraftnotes.ActionSpecs(client)...)
	return actionSpecGroup("gitlab_mr_review", specs)
}

func buildPackageActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 24)
	specs = append(specs, packages.ActionSpecs(client)...)
	specs = append(specs, containerregistry.ActionSpecs(client)...)
	specs = append(specs, protectedpackages.ActionSpecs(client)...)
	return actionSpecGroup("gitlab_package", specs)
}

func buildPipelineActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 33)
	specs = append(specs, pipelines.ActionSpecs(client)...)
	specs = append(specs, pipelinetriggers.ActionSpecs(client)...)
	specs = append(specs, resourcegroups.ActionSpecs(client)...)
	specs = append(specs, pipelineschedules.ActionSpecs(client)...)
	return actionSpecGroup("gitlab_pipeline", specs)
}

func buildProjectAliasActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	if !enterprise {
		return nil
	}
	return actionSpecGroup("gitlab_project_alias", projectaliases.ActionSpecs(client))
}

func buildProjectActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 130)
	specs = append(specs, uploads.ActionSpecs(client)...)
	specs = append(specs, projectstatistics.ActionSpecs(client)...)
	specs = append(specs, projectimportexport.ActionSpecs(client)...)
	specs = append(specs, members.ActionSpecs(client)...)
	specs = append(specs, labels.ActionSpecs(client)...)
	specs = append(specs, milestones.ActionSpecs(client)...)
	specs = append(specs, badges.ProjectActionSpecs(client)...)
	specs = append(specs, boards.ActionSpecs(client)...)
	specs = append(specs, integrations.ActionSpecs(client)...)
	specs = append(specs, pages.ActionSpecs(client)...)
	specs = append(specs, projectmirrors.ActionSpecs(client)...)
	if enterprise {
		specs = append(specs, securitysettings.ProjectActionSpecs(client)...)
		specs = append(specs, projectserviceaccounts.ActionSpecs(client)...)
	}
	specs = append(specs, projects.ActionSpecs(client, enterprise)...)
	return actionSpecGroup("gitlab_project", specs)
}

func buildReleaseActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 12)
	specs = append(specs, releases.ActionSpecs(client)...)
	specs = append(specs, releaselinks.ActionSpecs(client)...)
	return actionSpecGroup("gitlab_release", specs)
}

func buildRepositoryActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 41)
	specs = append(specs, repository.ActionSpecs(client)...)
	specs = append(specs, commits.ActionSpecs(client)...)
	specs = append(specs, files.ActionSpecs(client)...)
	specs = append(specs, repositorysubmodules.ActionSpecs(client)...)
	specs = append(specs, markdown.ActionSpecs(client)...)
	specs = append(specs, commitdiscussions.ActionSpecs(client)...)
	return actionSpecGroup("gitlab_repository", specs)
}

func buildRunnerActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_runner", runners.ActionSpecs(client))
}

func buildSearchActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_search", search.ActionSpecs(client))
}

func buildSecurityAttributeActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	if !enterprise {
		return nil
	}
	groups := actionSpecGroup("gitlab_security_attribute", securityattributes.ActionSpecs(client))
	if len(groups) > 0 {
		groups[0].Description = "Manage GitLab security attributes via GraphQL (Premium/Ultimate). Security attributes classify groups and projects under namespace-level security categories.\nReturns: JSON with created or updated attribute data, project update counts, or destructive confirmation messages. Destructive actions require confirmation.\n\nParam conventions: IDs are numeric GitLab IDs; mode is one of ADD, REMOVE, or REPLACE.\n\n- create: namespace_id*, category_id*, attributes* (array of {name, description, color})\n- update: attribute_id*, name, description, color\n- delete: attribute_id*\n- project_update: project_id*, add_attribute_ids, remove_attribute_ids\n- bulk_update: group_ids or project_ids*, attribute_ids*, mode*\n\nSee also: gitlab_security_category, gitlab_project, gitlab_group"
	}
	return groups
}

func buildSecurityCategoryActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	if !enterprise {
		return nil
	}
	groups := actionSpecGroup("gitlab_security_category", securitycategories.ActionSpecs(client))
	if len(groups) > 0 {
		groups[0].Description = "Manage GitLab security categories via GraphQL (Premium/Ultimate). Categories group namespace-level security attributes and control whether multiple attributes can be selected.\nReturns: JSON with category metadata and nested attribute summaries. Delete is destructive and requires confirmation because associated attributes are also deleted.\n\nParam conventions: IDs are numeric GitLab IDs.\n\n- create: namespace_id*, name*, description, multiple_selection\n- update: category_id*, namespace_id*, name, description\n- delete: category_id*\n\nSee also: gitlab_security_attribute, gitlab_group, gitlab_project"
	}
	return groups
}

func buildSecurityFindingActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	if !enterprise {
		return nil
	}
	return actionSpecGroup("gitlab_security_finding", securityfindings.ActionSpecs(client))
}

func buildSnippetActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 34)
	specs = append(specs, snippets.ActionSpecs(client)...)
	specs = append(specs, snippetdiscussions.ActionSpecs(client)...)
	specs = append(specs, snippetnotes.ActionSpecs(client)...)
	specs = append(specs, awardemoji.SnippetActionSpecs(client)...)
	return actionSpecGroup("gitlab_snippet", specs)
}

func buildStorageMoveActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 18)
	specs = append(specs, projectstoragemoves.ActionSpecs(client)...)
	specs = append(specs, snippetstoragemoves.ActionSpecs(client)...)
	if enterprise {
		specs = append(specs, groupstoragemoves.ActionSpecs(client)...)
	}
	return actionSpecGroup("gitlab_storage_move", specs)
}

func buildTagActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_tag", tags.ActionSpecs(client))
}

func buildTemplateActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 12)
	specs = append(specs, cilint.ActionSpecs(client)...)
	specs = append(specs, ciyamltemplates.ActionSpecs(client)...)
	specs = append(specs, dockerfiletemplates.ActionSpecs(client)...)
	specs = append(specs, gitignoretemplates.ActionSpecs(client)...)
	specs = append(specs, licensetemplates.ActionSpecs(client)...)
	specs = append(specs, projecttemplates.ActionSpecs(client)...)
	return actionSpecGroup("gitlab_template", specs)
}

func buildUserActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 75)
	specs = append(specs, users.ActionSpecs(client, enterprise)...)
	specs = append(specs, todos.ActionSpecs(client)...)
	specs = append(specs, events.UserActionSpecs(client)...)
	specs = append(specs, notifications.ActionSpecs(client)...)
	specs = append(specs, keys.ActionSpecs(client)...)
	specs = append(specs, namespaces.ActionSpecs(client)...)
	specs = append(specs, avatar.ActionSpecs(client)...)
	specs = append(specs, usergpgkeys.ActionSpecs(client)...)
	specs = append(specs, useremails.ActionSpecs(client)...)
	specs = append(specs, impersonationtokens.ActionSpecs(client)...)
	return actionSpecGroup("gitlab_user", specs)
}

func buildVulnerabilityActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	if !enterprise {
		return nil
	}
	return actionSpecGroup("gitlab_vulnerability", vulnerabilities.ActionSpecs(client))
}

func buildWikiActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_wiki", wikis.ActionSpecs(client))
}

func actionSpecGroup(toolName string, specs []toolutil.ActionSpec) []ActionSpecGroup {
	if len(specs) == 0 {
		return nil
	}
	return []ActionSpecGroup{{
		ToolName:               toolName,
		ReadOnly:               catalogGroupReadOnly(specs),
		Icons:                  catalogGroupIcons(toolName),
		CapabilityRequirements: catalogGroupCapabilityRequirements(toolName),
		FormatResult:           catalogGroupFormatResult(toolName),
		Actions:                specs,
		OwnerPackage:           "tools",
		SurfaceKind:            catalogGroupSurfaceKind(toolName),
	}}
}

func actionSpecGroupsByTool(groups []ActionSpecGroup) (map[string][]toolutil.ActionSpec, error) {
	byTool := make(map[string][]toolutil.ActionSpec, len(groups))
	var errs []error
	for _, group := range groups {
		toolName := strings.TrimSpace(group.ToolName)
		if toolName == "" {
			errs = append(errs, errors.New("action spec group tool name is required"))
			continue
		}
		byTool[toolName] = append(byTool[toolName], toolutil.CloneActionSpecs(group.Actions)...)
	}
	for toolName, specs := range byTool {
		seen := make(map[string]struct{}, len(specs))
		for _, spec := range specs {
			name := strings.TrimSpace(spec.Name)
			if name == "" {
				errs = append(errs, fmt.Errorf("%s: action spec name is required", toolName))
				continue
			}
			if _, exists := seen[name]; exists {
				errs = append(errs, fmt.Errorf("%s: duplicate action spec %q", toolName, name))
				continue
			}
			seen[name] = struct{}{}
		}
		sort.SliceStable(specs, func(left, right int) bool {
			return specs[left].Name < specs[right].Name
		})
		byTool[toolName] = specs
	}
	return byTool, errors.Join(errs...)
}

func sortedActionSpecGroups(groups []ActionSpecGroup) []ActionSpecGroup {
	if len(groups) == 0 {
		return nil
	}
	out := append([]ActionSpecGroup(nil), groups...)
	sort.SliceStable(out, func(left, right int) bool {
		return out[left].ToolName < out[right].ToolName
	})
	return out
}
