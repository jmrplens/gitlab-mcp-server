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
//
// It is an alias for [actioncatalog.CatalogGroupSpec]; the alias keeps
// the domain builders free of cross-package dependencies while preserving
// a single canonical type for catalog projection.
type ActionSpecGroup = actioncatalog.CatalogGroupSpec

// actionSpecGroupBuilder produces the [ActionSpecGroup]s contributed by one
// domain to the canonical catalog. The client is the GitLab API client
// (or nil for spec-only builders), and enterprise gates Premium/Ultimate
// domains.
type actionSpecGroupBuilder func(*gitlabclient.Client, bool) []ActionSpecGroup

//go:generate go run ../../cmd/gen_action_catalog_manifest/

// Edition tier markers for per-action gating. An empty Edition means Free.
// These are the canonical minimum-tier values mapped by
// [edition.TierFromEdition] and applied by the central catalog tier filter.
const (
	editionPremium  = "premium"
	editionUltimate = "ultimate"
)

// editionTaggedSpecs returns copies of specs with Edition set to tier for the
// whole set, so the central tier filter gates the group at that minimum tier.
// It overrides any existing Edition because these domains are uniform-tier and
// were historically self-tagged "premium" before the 3-tier model could
// distinguish Premium from Ultimate; the caller asserts the correct tier here.
func editionTaggedSpecs(specs []toolutil.ActionSpec, tier string) []toolutil.ActionSpec {
	out := toolutil.CloneActionSpecs(specs)
	for i := range out {
		out[i].Edition = tier
	}
	return out
}

// CollectActionSpecs gathers canonical specs from domain-local builders
// and returns them in deterministic, sorted order. The enterprise flag
// toggles Premium/Ultimate domains; client is forwarded to every builder
// so GitLab.com detection and edition-sensitive specs can be assembled
// correctly. The result is the input to [BuildActionCatalog].
func CollectActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	groups := make([]ActionSpecGroup, 0)
	for _, build := range actionSpecGroupBuilders() {
		groups = append(groups, build(client, enterprise)...)
	}
	return sortedActionSpecGroups(actioncompat.ApplyToGroupSpecs(groups))
}

// buildAdminActionSpecs contributes the gitlab_admin catalog group.
func buildAdminActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_admin", adminspecs.ActionSpecs(client))
}

// buildAccessActionSpecs contributes the gitlab_access catalog group by
// merging specs from access tokens, deploy tokens, deploy keys, access
// requests, and invites sub-packages.
func buildAccessActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 48)
	specs = append(specs, accesstokens.ActionSpecs(client)...)
	specs = append(specs, deploytokens.ActionSpecs(client)...)
	specs = append(specs, deploykeys.ActionSpecs(client)...)
	specs = append(specs, accessrequests.ActionSpecs(client)...)
	specs = append(specs, invites.ActionSpecs(client)...)
	return actionSpecGroup("gitlab_access", specs)
}

// buildOrbitActionSpecs contributes the gitlab_orbit catalog group only
// when the deployment is GitLab.com and Enterprise is enabled. Returns
// nil otherwise so the group is omitted from the catalog for
// self-managed instances and CE catalogs.
func buildOrbitActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	if client == nil || !client.IsGitLabDotCom() {
		return nil
	}
	// Orbit is a Premium+ GitLab.com feature; the GitLab.com check stays here and
	// the central tier filter gates it to Premium/Ultimate via the Edition tag.
	return actionSpecGroup("gitlab_orbit", editionTaggedSpecs(orbit.ActionSpecs(client), editionPremium))
}

// buildAttestationActionSpecs contributes the gitlab_attestation
// Enterprise catalog group.
func buildAttestationActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_attestation", editionTaggedSpecs(attestations.ActionSpecs(client), editionUltimate))
}

// buildAuditEventActionSpecs contributes the gitlab_audit_event
// Enterprise catalog group.
func buildAuditEventActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_audit_event", editionTaggedSpecs(auditevents.ActionSpecs(client), editionPremium))
}

// buildBranchActionSpecs contributes the gitlab_branch catalog group by
// merging branch and branch rule specs.
func buildBranchActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := append(branches.ActionSpecs(client), branchrules.ActionSpecs(client)...)
	return actionSpecGroup("gitlab_branch", specs)
}

// buildCICatalogActionSpecs contributes the gitlab_ci_catalog catalog group.
func buildCICatalogActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_ci_catalog", cicatalog.ActionSpecs(client))
}

// buildCIVariableActionSpecs contributes the gitlab_ci_variable catalog
// group by merging project, group, and instance CI variable specs.
func buildCIVariableActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 15)
	specs = append(specs, civariables.ActionSpecs(client)...)
	specs = append(specs, groupvariables.ActionSpecs(client)...)
	specs = append(specs, instancevariables.ActionSpecs(client)...)
	return actionSpecGroup("gitlab_ci_variable", specs)
}

// buildCompliancePolicyActionSpecs contributes the gitlab_compliance_policy
// Enterprise catalog group.
func buildCompliancePolicyActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_compliance_policy", editionTaggedSpecs(compliancepolicy.ActionSpecs(client), editionUltimate))
}

// buildCustomEmojiActionSpecs contributes the gitlab_custom_emoji catalog
// group.
func buildCustomEmojiActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_custom_emoji", customemoji.ActionSpecs(client))
}

// buildDependencyActionSpecs contributes the gitlab_dependency Enterprise
// catalog group.
func buildDependencyActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_dependency", editionTaggedSpecs(dependencies.ActionSpecs(client), editionUltimate))
}

// buildDORAMetricsActionSpecs contributes the gitlab_dora_metrics
// Enterprise catalog group.
func buildDORAMetricsActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_dora_metrics", editionTaggedSpecs(dorametrics.ActionSpecs(client), editionUltimate))
}

// buildEnvironmentActionSpecs contributes the gitlab_environment catalog
// group by merging environment, protected environment, freeze period,
// deployment, and deployment merge request specs.
func buildEnvironmentActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 23)
	specs = append(specs, environments.ActionSpecs(client)...)
	specs = append(specs, protectedenvs.ActionSpecs(client)...)
	specs = append(specs, freezeperiods.ActionSpecs(client)...)
	specs = append(specs, deployments.ActionSpecs(client)...)
	specs = append(specs, deploymentmergerequests.ActionSpecs(client)...)
	return actionSpecGroup("gitlab_environment", specs)
}

// buildEnterpriseUserActionSpecs contributes the gitlab_enterprise_user
// Enterprise catalog group.
func buildEnterpriseUserActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_enterprise_user", editionTaggedSpecs(enterpriseusers.ActionSpecs(client), editionPremium))
}

// buildExternalStatusCheckActionSpecs contributes the
// gitlab_external_status_check Enterprise catalog group.
func buildExternalStatusCheckActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_external_status_check", editionTaggedSpecs(externalstatuschecks.ActionSpecs(client), editionUltimate))
}

// buildFeatureFlagsActionSpecs contributes the gitlab_feature_flags
// catalog group by merging feature flag and flag user list specs.
func buildFeatureFlagsActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 10)
	specs = append(specs, featureflags.ActionSpecs(client)...)
	specs = append(specs, ffuserlists.ActionSpecs(client)...)
	return actionSpecGroup("gitlab_feature_flags", specs)
}

// buildGeoActionSpecs contributes the gitlab_geo Enterprise catalog group.
func buildGeoActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_geo", editionTaggedSpecs(geo.ActionSpecs(client), editionPremium))
}

// buildGroupActionSpecs contributes the gitlab_group catalog group. It
// always emits the base CE set and, when enterprise is true, also merges
// in Premium/Ultimate-only group sub-domains (epics, SAML, LDAP, audit
// settings, group iterations, group service accounts, group wikis, etc.).
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
	// Group service accounts are Free tier (service_accounts.md); the central
	// tier filter — not positional gating — decides their visibility.
	specs = append(specs, groupserviceaccounts.ActionSpecs(client)...)
	if !enterprise {
		return actionSpecGroup("gitlab_group", specs)
	}
	specs = append(specs, epicdiscussions.ActionSpecs(client)...)
	specs = append(specs, epics.ActionSpecs(client)...)
	specs = append(specs, resourceevents.EpicActionSpecs(client)...)
	specs = append(specs, epicissues.ActionSpecs(client)...)
	specs = append(specs, epicnotes.ActionSpecs(client)...)
	specs = append(specs, groupepicboards.ActionSpecs(client)...)
	specs = append(specs, groupwikis.ActionSpecs(client)...)
	specs = append(specs, groupprotectedbranches.ActionSpecs(client)...)
	specs = append(specs, groupprotectedenvs.ActionSpecs(client)...)
	specs = append(specs, groupldap.ActionSpecs(client)...)
	specs = append(specs, groupsaml.ActionSpecs(client)...)
	specs = append(specs, groupanalytics.ActionSpecs(client)...)
	specs = append(specs, editionTaggedSpecs(groupcredentials.ActionSpecs(client), editionUltimate)...)
	specs = append(specs, groupsshcerts.ActionSpecs(client)...)
	specs = append(specs, editionTaggedSpecs(securitysettings.GroupActionSpecs(client), editionUltimate)...)
	return actionSpecGroup("gitlab_group", specs)
}

// buildGroupSCIMActionSpecs contributes the gitlab_group_scim Enterprise
// catalog group.
func buildGroupSCIMActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_group_scim", editionTaggedSpecs(groupscim.ActionSpecs(client), editionPremium))
}

// buildIssueActionSpecs contributes the gitlab_issue catalog group by
// merging issue, issue note, issue link, issue discussion, issue
// statistics, work item, issue award emoji, and issue resource event
// specs. Project and group iteration specs are appended when enterprise
// is true.
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

// buildJobActionSpecs contributes the gitlab_job catalog group by
// merging job and job token scope specs.
func buildJobActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 25)
	specs = append(specs, jobs.ActionSpecs(client)...)
	specs = append(specs, jobtokenscope.ActionSpecs(client)...)
	return actionSpecGroup("gitlab_job", specs)
}

// buildMergeRequestActionSpecs contributes the gitlab_merge_request
// catalog group by merging MR, MR approval, MR approval settings, MR
// context commits, MR award emoji, and MR resource event specs.
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

// buildMergeTrainActionSpecs contributes the gitlab_merge_train Enterprise
// catalog group.
func buildMergeTrainActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_merge_train", editionTaggedSpecs(mergetrains.ActionSpecs(client), editionPremium))
}

// buildMemberRoleActionSpecs contributes the gitlab_member_role Enterprise
// catalog group.
func buildMemberRoleActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_member_role", editionTaggedSpecs(memberroles.ActionSpecs(client), editionUltimate))
}

// buildModelRegistryActionSpecs contributes the gitlab_model_registry
// catalog group.
func buildModelRegistryActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_model_registry", modelregistry.ActionSpecs(client))
}

// buildMRReviewActionSpecs contributes the gitlab_mr_review catalog
// group by merging MR note, MR discussion, MR change, and MR draft note
// specs.
func buildMRReviewActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 23)
	specs = append(specs, mrnotes.ActionSpecs(client)...)
	specs = append(specs, mrdiscussions.ActionSpecs(client)...)
	specs = append(specs, mrchanges.ActionSpecs(client)...)
	specs = append(specs, mrdraftnotes.ActionSpecs(client)...)
	return actionSpecGroup("gitlab_mr_review", specs)
}

// buildPackageActionSpecs contributes the gitlab_package catalog group by
// merging package, container registry, and protected package specs.
func buildPackageActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 24)
	specs = append(specs, packages.ActionSpecs(client)...)
	specs = append(specs, containerregistry.ActionSpecs(client)...)
	specs = append(specs, protectedpackages.ActionSpecs(client)...)
	return actionSpecGroup("gitlab_package", specs)
}

// buildPipelineActionSpecs contributes the gitlab_pipeline catalog group
// by merging pipeline, pipeline trigger, resource group, and pipeline
// schedule specs.
func buildPipelineActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 33)
	specs = append(specs, pipelines.ActionSpecs(client)...)
	specs = append(specs, pipelinetriggers.ActionSpecs(client)...)
	specs = append(specs, resourcegroups.ActionSpecs(client)...)
	specs = append(specs, pipelineschedules.ActionSpecs(client)...)
	return actionSpecGroup("gitlab_pipeline", specs)
}

// buildProjectAliasActionSpecs contributes the gitlab_project_alias
// Enterprise catalog group.
func buildProjectAliasActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_project_alias", editionTaggedSpecs(projectaliases.ActionSpecs(client), editionPremium))
}

// buildProjectActionSpecs contributes the gitlab_project catalog group.
// It always emits the base CE project surface and, when enterprise is
// true, also includes push rule and project service account specs.
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
	// Project service accounts are Free tier (service_accounts.md); the central
	// tier filter decides visibility, so they live in the base spec list.
	specs = append(specs, projectserviceaccounts.ActionSpecs(client)...)
	if enterprise {
		specs = append(specs, editionTaggedSpecs(securitysettings.ProjectActionSpecs(client), editionUltimate)...)
	}
	specs = append(specs, projects.ActionSpecs(client, enterprise)...)
	return actionSpecGroup("gitlab_project", specs)
}

// buildReleaseActionSpecs contributes the gitlab_release catalog group by
// merging release and release link specs.
func buildReleaseActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 12)
	specs = append(specs, releases.ActionSpecs(client)...)
	specs = append(specs, releaselinks.ActionSpecs(client)...)
	return actionSpecGroup("gitlab_release", specs)
}

// buildRepositoryActionSpecs contributes the gitlab_repository catalog
// group by merging repository tree/compare, commit, file, submodule,
// markdown, and commit discussion specs.
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

// buildRunnerActionSpecs contributes the gitlab_runner catalog group.
func buildRunnerActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_runner", runners.ActionSpecs(client))
}

// buildSearchActionSpecs contributes the gitlab_search catalog group.
func buildSearchActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_search", search.ActionSpecs(client))
}

// buildSecurityAttributeActionSpecs contributes the gitlab_security_attribute
// Enterprise catalog group. The custom group description documents the
// supported actions in human-readable form for the schema resource.
func buildSecurityAttributeActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	groups := actionSpecGroup("gitlab_security_attribute", editionTaggedSpecs(securityattributes.ActionSpecs(client), editionUltimate))
	if len(groups) > 0 {
		groups[0].Description = "Manage GitLab security attributes via GraphQL (Premium/Ultimate). Security attributes classify groups and projects under namespace-level security categories.\nReturns: JSON with created or updated attribute data, project update counts, or destructive confirmation messages. Destructive actions require confirmation.\n\nParam conventions: IDs are numeric GitLab IDs; mode is one of ADD, REMOVE, or REPLACE.\n\n- create: namespace_id*, category_id*, attributes* (array of {name, description, color})\n- update: attribute_id*, name, description, color\n- delete: attribute_id*\n- project_update: project_id*, add_attribute_ids, remove_attribute_ids\n- bulk_update: group_ids or project_ids*, attribute_ids*, mode*\n\nSee also: gitlab_security_category, gitlab_project, gitlab_group"
	}
	return groups
}

// buildSecurityCategoryActionSpecs contributes the gitlab_security_category
// Enterprise catalog group. The custom group description documents the
// supported actions in human-readable form for the schema resource.
func buildSecurityCategoryActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	groups := actionSpecGroup("gitlab_security_category", editionTaggedSpecs(securitycategories.ActionSpecs(client), editionUltimate))
	if len(groups) > 0 {
		groups[0].Description = "Manage GitLab security categories via GraphQL (Premium/Ultimate). Categories group namespace-level security attributes and control whether multiple attributes can be selected.\nReturns: JSON with category metadata and nested attribute summaries. Delete is destructive and requires confirmation because associated attributes are also deleted.\n\nParam conventions: IDs are numeric GitLab IDs.\n\n- create: namespace_id*, name*, description, multiple_selection\n- update: category_id*, namespace_id*, name, description\n- delete: category_id*\n\nSee also: gitlab_security_attribute, gitlab_group, gitlab_project"
	}
	return groups
}

// buildSecurityFindingActionSpecs contributes the gitlab_security_finding
// Enterprise catalog group.
func buildSecurityFindingActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_security_finding", editionTaggedSpecs(securityfindings.ActionSpecs(client), editionUltimate))
}

// buildSnippetActionSpecs contributes the gitlab_snippet catalog group by
// merging snippet, snippet discussion, snippet note, and snippet award
// emoji specs.
func buildSnippetActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 34)
	specs = append(specs, snippets.ActionSpecs(client)...)
	specs = append(specs, snippetdiscussions.ActionSpecs(client)...)
	specs = append(specs, snippetnotes.ActionSpecs(client)...)
	specs = append(specs, awardemoji.SnippetActionSpecs(client)...)
	return actionSpecGroup("gitlab_snippet", specs)
}

// buildStorageMoveActionSpecs contributes the gitlab_storage_move
// catalog group. The group always includes project and snippet storage
// move specs and appends group storage move specs when enterprise is
// true.
func buildStorageMoveActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 18)
	specs = append(specs, projectstoragemoves.ActionSpecs(client)...)
	specs = append(specs, snippetstoragemoves.ActionSpecs(client)...)
	if enterprise {
		specs = append(specs, groupstoragemoves.ActionSpecs(client)...)
	}
	return actionSpecGroup("gitlab_storage_move", specs)
}

// buildTagActionSpecs contributes the gitlab_tag catalog group.
func buildTagActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_tag", tags.ActionSpecs(client))
}

// buildTemplateActionSpecs contributes the gitlab_template catalog group
// by merging CI lint, CI YAML, Dockerfile, gitignore, license, and
// project template specs.
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

// buildUserActionSpecs contributes the gitlab_user catalog group. The
// user package receives the enterprise flag so it can decide whether to
// include Enterprise-only user sub-resources.
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

// buildVulnerabilityActionSpecs contributes the gitlab_vulnerability
// Enterprise catalog group.
func buildVulnerabilityActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_vulnerability", editionTaggedSpecs(vulnerabilities.ActionSpecs(client), editionUltimate))
}

// buildWikiActionSpecs contributes the gitlab_wiki catalog group.
func buildWikiActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_wiki", wikis.ActionSpecs(client))
}

// actionSpecGroup wraps specs into a single [ActionSpecGroup] with the
// catalog group metadata derived from the tool name. Returns nil when
// specs is empty so the catalog builder treats the group as absent.
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

// actionSpecGroupsByTool groups specs by tool name, validates that each
// group has a non-blank tool name, and that every spec has a non-blank
// and non-duplicate action name. Returns a copy of the specs sorted by
// action name per tool, joined with errors.Join so callers can surface
// every validation issue in a single round-trip.
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

// sortedActionSpecGroups returns a defensive copy of groups sorted by
// ToolName. Returns nil for an empty or nil input so callers can use
// the result as a drop-in replacement for the original.
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
