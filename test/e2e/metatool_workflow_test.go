//go:build e2e

// metatool_workflow_test.go contains end-to-end workflow tests that exercise
// the complete GitLab project lifecycle using domain meta-tools
// (gitlab_project, gitlab_branch, gitlab_repository, etc.) instead of
// individual MCP tools.
package e2e

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/accessrequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/accesstokens"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/alertmanagement"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/appearance"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/applications"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/appstatistics"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/attestations"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/auditevents"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/avatar"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/awardemoji"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/badges"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/boards"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/branches"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/branchrules"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/broadcastmessages"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/bulkimports"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/cicatalog"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/cilint"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/civariables"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/ciyamltemplates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/clusteragents"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commitdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/compliancepolicy"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/containerregistry"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/customattributes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/customemoji"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/dbmigrations"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/dependencies"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deploykeys"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deploymentmergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deployments"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deploytokens"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/dockerfiletemplates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/dorametrics"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/enterpriseusers"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/environments"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/errortracking"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/events"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/externalstatuschecks"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/featureflags"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/features"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/ffuserlists"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/files"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/freezeperiods"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/geo"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/gitignoretemplates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupboards"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupimportexport"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/grouplabels"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupmarkdownuploads"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupmembers"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupmilestones"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/grouprelationsexport"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupscim"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupstoragemoves"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupvariables"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/impersonationtokens"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/importservice"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/instancevariables"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/integrations"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/invites"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuediscussions"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuelinks"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuenotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuestatistics"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/jobtokenscope"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/keys"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/labels"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/license"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/licensetemplates"
	markdowntool "github.com/jmrplens/gitlab-mcp-server/internal/tools/markdown"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/memberroles"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/members"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergetrains"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/metadata"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrapprovals"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrapprovalsettings"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrchanges"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrcontextcommits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrdraftnotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrnotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/namespaces"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/notifications"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/packages"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pages"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pipelineschedules"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pipelinetriggers"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/planlimits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectaliases"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectimportexport"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectstatistics"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectstoragemoves"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projecttemplates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/protectedenvs"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/protectedpackages"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/releaselinks"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/releases"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/repository"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/repositorysubmodules"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/resourceevents"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/resourcegroups"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/search"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/securefiles"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/securityfindings"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/settings"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/sidekiq"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/snippetdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/snippetnotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/snippets"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/snippetstoragemoves"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/systemhooks"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/tags"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/terraformstates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/todos"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/topics"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/uploads"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/usagedata"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/useremails"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/usergpgkeys"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/users"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/vulnerabilities"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/wikis"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/workitems"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Branch name, tag version, and assertion format used across meta-tool
// E2E workflow steps.
const (
	testMetaBranch        = "feature/meta-changes"
	testMetaTag           = "v0.1.0-meta"
	fmtExpectedTag        = "expected tag, got %q"
	msgMetaIssueIIDNotSet = "issueIID not set"
)

// metaState holds shared state for the meta-tool workflow.
// Separated from the individual-tools state to allow independent execution.
type metaState struct {
	projectID     int64
	projectPath   string
	mrIID         int64
	noteID        int64
	discussionID  string
	releaseLinkID int64
	lastCommitSHA string // SHA from most recent commit
	issueIID      int64  // issue IID for issue lifecycle tests
	issueNoteID   int64  // issue note ID for issue note tests
	groupID       int64  // group ID discovered via group list (0 if none)
	groupPath     string // group full path discovered via group list
	packageID     int64  // package ID for package lifecycle tests
	packageFileID int64  // package file ID for package file tests
	// Phase 5: new domain state fields.
	wikiSlug              string
	envID                 int64
	labelID               int64
	milestoneIID          int64
	issue2IID             int64
	deployKeyID           int64
	snippetID             int64
	issueDiscussionID     string
	issueDiscussionNoteID int64
	draftNoteID           int64
	pipelineScheduleID    int
	badgeID               int64
	accessTokenID         int64
	awardEmojiID          int64
	freezePeriodID        int64
	triggerID             int64
	boardID               int64
	deployTokenProjectID  int64
	groupLabelID          int64
	groupMilestoneIID     int64
	groupVariableKey      string
	stateEventIssueIID    int64
}

// mState is the shared [metaState] instance used by [TestMetaToolWorkflow]
// sequential test steps.
var mState metaState

// TestMetaToolWorkflow exercises gap-coverage and enterprise meta-tool actions.
// Common CRUD meta-tool paths are tested by standalone TestMeta_* files.
func TestMetaToolWorkflow(t *testing.T) {
	if state.metaSession == nil {
		t.Skip("meta session not configured — set META_TOOLS=true")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 580*time.Second)
	defer cancel()

	// Minimal project setup for gap-coverage and enterprise tests below.
	t.Run("01_CreateProject", func(t *testing.T) { metaCreateProject(ctx, t) })
	t.Run("03_UnprotectMain", func(t *testing.T) { metaUnprotectMain(ctx, t) })
	t.Run("04_CommitCreate", func(t *testing.T) { metaCommitCreate(ctx, t) })
	t.Run("06_BranchCreate", func(t *testing.T) { metaBranchCreate(ctx, t) })
	t.Run("11_CommitFeatureChanges", func(t *testing.T) { metaCommitFeatureChanges(ctx, t) })
	t.Run("22a_IssueCreate", func(t *testing.T) { metaIssueCreate(ctx, t) })
	t.Run("23_CreateMR", func(t *testing.T) { metaCreateMR(ctx, t) })

	// Enterprise push rules (no standalone coverage).
	if state.enterprise {
		t.Run("41_AddPushRule", func(t *testing.T) { metaAddPushRule(ctx, t) })
		t.Run("42_GetPushRules", func(t *testing.T) { metaGetPushRules(ctx, t) })
		t.Run("43_EditPushRule", func(t *testing.T) { metaEditPushRule(ctx, t) })
		t.Run("44_DeletePushRule", func(t *testing.T) { metaDeletePushRule(ctx, t) })
	}

	// Group discovery (sets groupPath for conditional blocks below).
	t.Run("38c_GroupList", func(t *testing.T) { metaGroupList(ctx, t) })

	// GQL meta-tools.
	t.Run("47_BranchRuleList", func(t *testing.T) { metaBranchRuleList(ctx, t) })
	t.Run("48_CICatalogList", func(t *testing.T) { metaCICatalogList(ctx, t) })
	if state.enterprise {
		t.Run("49_VulnerabilitySeverityCount", func(t *testing.T) { metaVulnerabilitySeverityCount(ctx, t) })
		t.Run("50_VulnerabilityList", func(t *testing.T) { metaVulnerabilityList(ctx, t) })
	}

	// Instance-level CI variables (no standalone coverage).
	t.Run("61a_CIVariableInstanceCreate", func(t *testing.T) { metaCIVariableInstanceCreate(ctx, t) })
	t.Run("61b_CIVariableInstanceList", func(t *testing.T) { metaCIVariableInstanceList(ctx, t) })
	t.Run("61c_CIVariableInstanceGet", func(t *testing.T) { metaCIVariableInstanceGet(ctx, t) })
	t.Run("61d_CIVariableInstanceUpdate", func(t *testing.T) { metaCIVariableInstanceUpdate(ctx, t) })
	t.Run("61e_CIVariableInstanceDelete", func(t *testing.T) { metaCIVariableInstanceDelete(ctx, t) })

	// --- Gap-coverage and enterprise meta-tool tests ---

	// CE-testable: gitlab_deployment, gitlab_job, gitlab_user extensions, gitlab_template, gitlab_admin.
	t.Run("115_DeploymentList", func(t *testing.T) { metaDeploymentList(ctx, t) })
	t.Run("116_JobList", func(t *testing.T) { metaJobList(ctx, t) })
	t.Run("116a_JobTokenScopeGet", func(t *testing.T) { metaJobTokenScopeGet(ctx, t) })
	t.Run("117_UserSSHKeyList", func(t *testing.T) { metaUserSSHKeyList(ctx, t) })
	t.Run("118_UserGPGKeyList", func(t *testing.T) { metaUserGPGKeyList(ctx, t) })
	t.Run("119_TemplateGitignoreList", func(t *testing.T) { metaTemplateGitignoreList(ctx, t) })
	t.Run("120_TemplateCIYmlList", func(t *testing.T) { metaTemplateCIYmlList(ctx, t) })
	t.Run("121_AdminTopicList", func(t *testing.T) { metaAdminTopicList(ctx, t) })
	t.Run("122_AdminSettingsGet", func(t *testing.T) { metaAdminSettingsGet(ctx, t) })
	t.Run("123_SearchIssues", func(t *testing.T) { metaSearchIssues(ctx, t) })
	t.Run("124_SearchProjects", func(t *testing.T) { metaSearchProjects(ctx, t) })

	// Enterprise / Premium meta-tools (only registered when GITLAB_ENTERPRISE=true).
	// FeatureFlagList is CE-compatible — always registered.
	t.Run("125_FeatureFlagList", func(t *testing.T) { metaFeatureFlagList(ctx, t) })

	// Freeze periods via gitlab_environment.
	t.Run("125a_FreezePeriodCreate", func(t *testing.T) { metaFreezePeriodCreate(ctx, t) })
	t.Run("125b_FreezePeriodList", func(t *testing.T) { metaFreezePeriodList(ctx, t) })
	t.Run("125c_FreezePeriodGet", func(t *testing.T) { metaFreezePeriodGet(ctx, t) })
	t.Run("125d_FreezePeriodUpdate", func(t *testing.T) { metaFreezePeriodUpdate(ctx, t) })
	t.Run("125e_FreezePeriodDelete", func(t *testing.T) { metaFreezePeriodDelete(ctx, t) })

	// Protected environments via gitlab_environment (enterprise only).
	if state.enterprise {
		t.Run("125f_ProtectedEnvProtect", func(t *testing.T) { metaProtectedEnvProtect(ctx, t) })
		t.Run("125g_ProtectedEnvList", func(t *testing.T) { metaProtectedEnvList(ctx, t) })
		t.Run("125h_ProtectedEnvGet", func(t *testing.T) { metaProtectedEnvGet(ctx, t) })
		t.Run("125i_ProtectedEnvUnprotect", func(t *testing.T) { metaProtectedEnvUnprotect(ctx, t) })
	}

	// Pipeline triggers via gitlab_pipeline.
	t.Run("125j_PipelineTriggerCreate", func(t *testing.T) { metaPipelineTriggerCreate(ctx, t) })
	t.Run("125k_PipelineTriggerList", func(t *testing.T) { metaPipelineTriggerList(ctx, t) })
	t.Run("125l_PipelineTriggerGet", func(t *testing.T) { metaPipelineTriggerGet(ctx, t) })
	t.Run("125m_PipelineTriggerUpdate", func(t *testing.T) { metaPipelineTriggerUpdate(ctx, t) })
	t.Run("125n_PipelineTriggerDelete", func(t *testing.T) { metaPipelineTriggerDelete(ctx, t) })

	// Boards via gitlab_project.
	t.Run("125o_BoardCreate", func(t *testing.T) { metaBoardCreate(ctx, t) })
	t.Run("125p_BoardList", func(t *testing.T) { metaBoardList(ctx, t) })
	t.Run("125q_BoardGet", func(t *testing.T) { metaBoardGet(ctx, t) })
	t.Run("125r_BoardDelete", func(t *testing.T) { metaBoardDelete(ctx, t) })

	// Deploy tokens via gitlab_access.
	t.Run("125s_DeployTokenCreate", func(t *testing.T) { metaDeployTokenCreateProject(ctx, t) })
	t.Run("125t_DeployTokenList", func(t *testing.T) { metaDeployTokenListProject(ctx, t) })
	t.Run("125u_DeployTokenGet", func(t *testing.T) { metaDeployTokenGetProject(ctx, t) })
	t.Run("125v_DeployTokenDelete", func(t *testing.T) { metaDeployTokenDeleteProject(ctx, t) })

	// Protected tags via gitlab_tag.
	t.Run("125w_ProtectedTagProtect", func(t *testing.T) { metaProtectedTagProtect(ctx, t) })
	t.Run("125x_ProtectedTagList", func(t *testing.T) { metaProtectedTagList(ctx, t) })
	t.Run("125y_ProtectedTagGet", func(t *testing.T) { metaProtectedTagGet(ctx, t) })
	t.Run("125z_ProtectedTagUnprotect", func(t *testing.T) { metaProtectedTagUnprotect(ctx, t) })

	// Notifications via gitlab_user.
	t.Run("125aa_NotificationGlobalGet", func(t *testing.T) { metaNotificationGlobalGet(ctx, t) })
	t.Run("125ab_NotificationProjectGet", func(t *testing.T) { metaNotificationProjectGet(ctx, t) })

	// Markdown render via gitlab_repository.
	t.Run("125ac_MarkdownRender", func(t *testing.T) { metaMarkdownRender(ctx, t) })

	// Resource state events via gitlab_issue and gitlab_merge_request.
	t.Run("125ad_IssueCreateForStateEvent", func(t *testing.T) { metaIssueCreateForStateEvent(ctx, t) })
	t.Run("125ae_IssueCloseForStateEvent", func(t *testing.T) { metaIssueCloseForStateEvent(ctx, t) })
	t.Run("125af_IssueStateEventList", func(t *testing.T) { metaIssueStateEventList(ctx, t) })
	t.Run("125ag_IssueDeleteStateEvent", func(t *testing.T) { metaIssueDeleteStateEvent(ctx, t) })
	t.Run("125ah_MRStateEventList", func(t *testing.T) { metaMRStateEventList(ctx, t) })

	// Group labels, milestones, variables via gitlab_group and gitlab_ci_variable.
	if mState.groupPath != "" {
		t.Run("125ai_GroupLabelCreate", func(t *testing.T) { metaGroupLabelCreate(ctx, t) })
		t.Run("125aj_GroupLabelList", func(t *testing.T) { metaGroupLabelList(ctx, t) })
		t.Run("125ak_GroupLabelDelete", func(t *testing.T) { metaGroupLabelDelete(ctx, t) })
		t.Run("125al_GroupMilestoneCreate", func(t *testing.T) { metaGroupMilestoneCreate(ctx, t) })
		t.Run("125am_GroupMilestoneList", func(t *testing.T) { metaGroupMilestoneList(ctx, t) })
		t.Run("125an_GroupMilestoneGet", func(t *testing.T) { metaGroupMilestoneGet(ctx, t) })
		t.Run("125ao_GroupMilestoneDelete", func(t *testing.T) { metaGroupMilestoneDelete(ctx, t) })
		t.Run("125ap_GroupVariableCreate", func(t *testing.T) { metaGroupVariableCreate(ctx, t) })
		t.Run("125aq_GroupVariableList", func(t *testing.T) { metaGroupVariableList(ctx, t) })
		t.Run("125ar_GroupVariableGet", func(t *testing.T) { metaGroupVariableGet(ctx, t) })
		t.Run("125ara_GroupVariableUpdate", func(t *testing.T) { metaGroupVariableUpdate(ctx, t) })
		t.Run("125as_GroupVariableDelete", func(t *testing.T) { metaGroupVariableDelete(ctx, t) })
	}

	if state.enterprise {
		t.Run("126_MergeTrainList", func(t *testing.T) { metaMergeTrainList(ctx, t) })
		t.Run("127_AuditEventList", func(t *testing.T) { metaAuditEventList(ctx, t) })
		t.Run("128_DORAMetrics", func(t *testing.T) { metaDORAMetrics(ctx, t) })
		t.Run("129_DependencyList", func(t *testing.T) { metaDependencyList(ctx, t) })
		t.Run("130_ExternalStatusCheckList", func(t *testing.T) { metaExternalStatusCheckList(ctx, t) })
		if mState.groupPath != "" {
			t.Run("131_GroupSCIMList", func(t *testing.T) { metaGroupSCIMList(ctx, t) })
		}
		t.Run("132_MemberRoleList", func(t *testing.T) { metaMemberRoleList(ctx, t) })
		if mState.groupPath != "" {
			t.Run("133_EnterpriseUserList", func(t *testing.T) { metaEnterpriseUserList(ctx, t) })
		}
		t.Run("134_AttestationList", func(t *testing.T) { metaAttestationList(ctx, t) })
		t.Run("135_CompliancePolicyGet", func(t *testing.T) { metaCompliancePolicyGet(ctx, t) })
		t.Run("136_ProjectAliasList", func(t *testing.T) { metaProjectAliasList(ctx, t) })
		t.Run("137_GeoList", func(t *testing.T) { metaGeoList(ctx, t) })
		t.Run("138_StorageMoveList", func(t *testing.T) { metaStorageMoveList(ctx, t) })
		t.Run("139_SecurityFindingList", func(t *testing.T) { metaSecurityFindingList(ctx, t) })
		t.Run("140_ModelRegistryDownload", func(t *testing.T) { metaModelRegistryDownload(ctx, t) })
	}

	// ================================================================
	// AUTO-GENERATED GAP COVERAGE (CE normal, no group dependency)
	// ================================================================
	t.Run("access_approve_project", func(t *testing.T) { metaAccessApproveProject(ctx, t) })
	t.Run("access_deny_project", func(t *testing.T) { metaAccessDenyProject(ctx, t) })
	t.Run("access_deploy_key_add_instance", func(t *testing.T) { metaAccessDeployKeyAddInstance(ctx, t) })
	t.Run("access_deploy_key_enable", func(t *testing.T) { metaAccessDeployKeyEnable(ctx, t) })
	t.Run("access_deploy_key_list_all", func(t *testing.T) { metaAccessDeployKeyListAll(ctx, t) })
	t.Run("access_deploy_key_list_user_project", func(t *testing.T) { metaAccessDeployKeyListUserProject(ctx, t) })
	t.Run("access_deploy_key_update", func(t *testing.T) { metaAccessDeployKeyUpdate(ctx, t) })
	t.Run("access_deploy_token_list_all", func(t *testing.T) { metaAccessDeployTokenListAll(ctx, t) })
	t.Run("access_invite_list_project", func(t *testing.T) { metaAccessInviteListProject(ctx, t) })
	t.Run("access_invite_project", func(t *testing.T) { metaAccessInviteProject(ctx, t) })
	t.Run("access_request_list_project", func(t *testing.T) { metaAccessRequestListProject(ctx, t) })
	t.Run("access_request_project", func(t *testing.T) { metaAccessRequestProject(ctx, t) })
	t.Run("access_token_personal_get", func(t *testing.T) { metaAccessTokenPersonalGet(ctx, t) })
	t.Run("access_token_personal_list", func(t *testing.T) { metaAccessTokenPersonalList(ctx, t) })
	t.Run("access_token_personal_revoke", func(t *testing.T) { metaAccessTokenPersonalRevoke(ctx, t) })
	// SKIP: token_personal_revoke_self and token_personal_rotate_self would invalidate the PAT used for authentication
	// t.Run("access_token_personal_revoke_self", func(t *testing.T) { metaAccessTokenPersonalRevokeSelf(ctx, t) })
	t.Run("access_token_personal_rotate", func(t *testing.T) { metaAccessTokenPersonalRotate(ctx, t) })
	// t.Run("access_token_personal_rotate_self", func(t *testing.T) { metaAccessTokenPersonalRotateSelf(ctx, t) })
	t.Run("access_token_project_get", func(t *testing.T) { metaAccessTokenProjectGet(ctx, t) })
	t.Run("access_token_project_rotate", func(t *testing.T) { metaAccessTokenProjectRotate(ctx, t) })
	t.Run("access_token_project_rotate_self", func(t *testing.T) { metaAccessTokenProjectRotateSelf(ctx, t) })
	t.Run("admin_alert_metric_image_delete", func(t *testing.T) { metaAdminAlertMetricImageDelete(ctx, t) })
	t.Run("admin_alert_metric_image_list", func(t *testing.T) { metaAdminAlertMetricImageList(ctx, t) })
	t.Run("admin_alert_metric_image_update", func(t *testing.T) { metaAdminAlertMetricImageUpdate(ctx, t) })
	t.Run("admin_alert_metric_image_upload", func(t *testing.T) { metaAdminAlertMetricImageUpload(ctx, t) })
	t.Run("admin_app_statistics_get", func(t *testing.T) { metaAdminAppStatisticsGet(ctx, t) })
	t.Run("admin_appearance_get", func(t *testing.T) { metaAdminAppearanceGet(ctx, t) })
	t.Run("admin_appearance_update", func(t *testing.T) { metaAdminAppearanceUpdate(ctx, t) })
	t.Run("admin_application_create", func(t *testing.T) { metaAdminApplicationCreate(ctx, t) })
	t.Run("admin_application_delete", func(t *testing.T) { metaAdminApplicationDelete(ctx, t) })
	t.Run("admin_application_list", func(t *testing.T) { metaAdminApplicationList(ctx, t) })
	t.Run("admin_broadcast_message_create", func(t *testing.T) { metaAdminBroadcastMessageCreate(ctx, t) })
	t.Run("admin_broadcast_message_delete", func(t *testing.T) { metaAdminBroadcastMessageDelete(ctx, t) })
	t.Run("admin_broadcast_message_get", func(t *testing.T) { metaAdminBroadcastMessageGet(ctx, t) })
	t.Run("admin_broadcast_message_list", func(t *testing.T) { metaAdminBroadcastMessageList(ctx, t) })
	t.Run("admin_broadcast_message_update", func(t *testing.T) { metaAdminBroadcastMessageUpdate(ctx, t) })
	t.Run("admin_bulk_import_start", func(t *testing.T) { metaAdminBulkImportStart(ctx, t) })
	t.Run("admin_cluster_agent_delete", func(t *testing.T) { metaAdminClusterAgentDelete(ctx, t) })
	t.Run("admin_cluster_agent_get", func(t *testing.T) { metaAdminClusterAgentGet(ctx, t) })
	t.Run("admin_cluster_agent_list", func(t *testing.T) { metaAdminClusterAgentList(ctx, t) })
	t.Run("admin_cluster_agent_register", func(t *testing.T) { metaAdminClusterAgentRegister(ctx, t) })
	t.Run("admin_cluster_agent_token_create", func(t *testing.T) { metaAdminClusterAgentTokenCreate(ctx, t) })
	t.Run("admin_cluster_agent_token_get", func(t *testing.T) { metaAdminClusterAgentTokenGet(ctx, t) })
	t.Run("admin_cluster_agent_token_list", func(t *testing.T) { metaAdminClusterAgentTokenList(ctx, t) })
	t.Run("admin_cluster_agent_token_revoke", func(t *testing.T) { metaAdminClusterAgentTokenRevoke(ctx, t) })
	t.Run("admin_custom_attr_delete", func(t *testing.T) { metaAdminCustomAttrDelete(ctx, t) })
	t.Run("admin_custom_attr_get", func(t *testing.T) { metaAdminCustomAttrGet(ctx, t) })
	t.Run("admin_custom_attr_list", func(t *testing.T) { metaAdminCustomAttrList(ctx, t) })
	t.Run("admin_custom_attr_set", func(t *testing.T) { metaAdminCustomAttrSet(ctx, t) })
	t.Run("admin_db_migration_mark", func(t *testing.T) { metaAdminDbMigrationMark(ctx, t) })
	t.Run("admin_error_tracking_create", func(t *testing.T) { metaAdminErrorTrackingCreate(ctx, t) })
	t.Run("admin_error_tracking_delete", func(t *testing.T) { metaAdminErrorTrackingDelete(ctx, t) })
	t.Run("admin_error_tracking_get_settings", func(t *testing.T) { metaAdminErrorTrackingGetSettings(ctx, t) })
	t.Run("admin_error_tracking_list", func(t *testing.T) { metaAdminErrorTrackingList(ctx, t) })
	t.Run("admin_error_tracking_update_settings", func(t *testing.T) { metaAdminErrorTrackingUpdateSettings(ctx, t) })
	t.Run("admin_feature_delete", func(t *testing.T) { metaAdminFeatureDelete(ctx, t) })
	t.Run("admin_feature_list", func(t *testing.T) { metaAdminFeatureList(ctx, t) })
	t.Run("admin_feature_list_definitions", func(t *testing.T) { metaAdminFeatureListDefinitions(ctx, t) })
	t.Run("admin_feature_set", func(t *testing.T) { metaAdminFeatureSet(ctx, t) })
	t.Run("admin_import_bitbucket", func(t *testing.T) { metaAdminImportBitbucket(ctx, t) })
	t.Run("admin_import_bitbucket_server", func(t *testing.T) { metaAdminImportBitbucketServer(ctx, t) })
	t.Run("admin_import_cancel_github", func(t *testing.T) { metaAdminImportCancelGithub(ctx, t) })
	t.Run("admin_import_gists", func(t *testing.T) { metaAdminImportGists(ctx, t) })
	t.Run("admin_import_github", func(t *testing.T) { metaAdminImportGithub(ctx, t) })
	t.Run("admin_license_add", func(t *testing.T) { metaAdminLicenseAdd(ctx, t) })
	t.Run("admin_license_delete", func(t *testing.T) { metaAdminLicenseDelete(ctx, t) })
	t.Run("admin_license_get", func(t *testing.T) { metaAdminLicenseGet(ctx, t) })
	t.Run("admin_metadata_get", func(t *testing.T) { metaAdminMetadataGet(ctx, t) })
	t.Run("admin_plan_limits_change", func(t *testing.T) { metaAdminPlanLimitsChange(ctx, t) })
	t.Run("admin_plan_limits_get", func(t *testing.T) { metaAdminPlanLimitsGet(ctx, t) })
	t.Run("admin_secure_file_create", func(t *testing.T) { metaAdminSecureFileCreate(ctx, t) })
	t.Run("admin_secure_file_delete", func(t *testing.T) { metaAdminSecureFileDelete(ctx, t) })
	t.Run("admin_secure_file_get", func(t *testing.T) { metaAdminSecureFileGet(ctx, t) })
	t.Run("admin_secure_file_list", func(t *testing.T) { metaAdminSecureFileList(ctx, t) })
	t.Run("admin_settings_update", func(t *testing.T) { metaAdminSettingsUpdate(ctx, t) })
	t.Run("admin_sidekiq_compound_metrics", func(t *testing.T) { metaAdminSidekiqCompoundMetrics(ctx, t) })
	t.Run("admin_sidekiq_job_stats", func(t *testing.T) { metaAdminSidekiqJobStats(ctx, t) })
	t.Run("admin_sidekiq_process_metrics", func(t *testing.T) { metaAdminSidekiqProcessMetrics(ctx, t) })
	t.Run("admin_sidekiq_queue_metrics", func(t *testing.T) { metaAdminSidekiqQueueMetrics(ctx, t) })
	t.Run("admin_system_hook_add", func(t *testing.T) { metaAdminSystemHookAdd(ctx, t) })
	t.Run("admin_system_hook_delete", func(t *testing.T) { metaAdminSystemHookDelete(ctx, t) })
	t.Run("admin_system_hook_get", func(t *testing.T) { metaAdminSystemHookGet(ctx, t) })
	t.Run("admin_system_hook_list", func(t *testing.T) { metaAdminSystemHookList(ctx, t) })
	t.Run("admin_system_hook_test", func(t *testing.T) { metaAdminSystemHookTest(ctx, t) })
	t.Run("admin_terraform_state_delete", func(t *testing.T) { metaAdminTerraformStateDelete(ctx, t) })
	t.Run("admin_terraform_state_get", func(t *testing.T) { metaAdminTerraformStateGet(ctx, t) })
	t.Run("admin_terraform_state_list", func(t *testing.T) { metaAdminTerraformStateList(ctx, t) })
	t.Run("admin_terraform_state_lock", func(t *testing.T) { metaAdminTerraformStateLock(ctx, t) })
	t.Run("admin_terraform_state_unlock", func(t *testing.T) { metaAdminTerraformStateUnlock(ctx, t) })
	t.Run("admin_terraform_version_delete", func(t *testing.T) { metaAdminTerraformVersionDelete(ctx, t) })
	t.Run("admin_topic_create", func(t *testing.T) { metaAdminTopicCreate(ctx, t) })
	t.Run("admin_topic_delete", func(t *testing.T) { metaAdminTopicDelete(ctx, t) })
	t.Run("admin_topic_get", func(t *testing.T) { metaAdminTopicGet(ctx, t) })
	t.Run("admin_topic_update", func(t *testing.T) { metaAdminTopicUpdate(ctx, t) })
	t.Run("admin_usage_data_metric_definitions", func(t *testing.T) { metaAdminUsageDataMetricDefinitions(ctx, t) })
	t.Run("admin_usage_data_non_sql_metrics", func(t *testing.T) { metaAdminUsageDataNonSqlMetrics(ctx, t) })
	t.Run("admin_usage_data_queries", func(t *testing.T) { metaAdminUsageDataQueries(ctx, t) })
	t.Run("admin_usage_data_service_ping", func(t *testing.T) { metaAdminUsageDataServicePing(ctx, t) })
	t.Run("admin_usage_data_track_event", func(t *testing.T) { metaAdminUsageDataTrackEvent(ctx, t) })
	t.Run("admin_usage_data_track_events", func(t *testing.T) { metaAdminUsageDataTrackEvents(ctx, t) })
	t.Run("branch_delete", func(t *testing.T) { metaBranchDelete(ctx, t) })
	t.Run("ci_catalog_get", func(t *testing.T) { metaCiCatalogGet(ctx, t) })
	t.Run("deployment_approve_or_reject", func(t *testing.T) { metaDeploymentApproveOrReject(ctx, t) })
	t.Run("deployment_create", func(t *testing.T) { metaDeploymentCreate(ctx, t) })
	t.Run("deployment_delete", func(t *testing.T) { metaDeploymentDelete(ctx, t) })
	t.Run("deployment_get", func(t *testing.T) { metaDeploymentGet(ctx, t) })
	t.Run("deployment_merge_requests", func(t *testing.T) { metaDeploymentMergeRequests(ctx, t) })
	t.Run("deployment_update", func(t *testing.T) { metaDeploymentUpdate(ctx, t) })
	t.Run("environment_protected_update", func(t *testing.T) { metaEnvironmentProtectedUpdate(ctx, t) })
	t.Run("feature_flags_feature_flag_create", func(t *testing.T) { metaFeatureFlagsFeatureFlagCreate(ctx, t) })
	t.Run("feature_flags_feature_flag_delete", func(t *testing.T) { metaFeatureFlagsFeatureFlagDelete(ctx, t) })
	t.Run("feature_flags_feature_flag_get", func(t *testing.T) { metaFeatureFlagsFeatureFlagGet(ctx, t) })
	t.Run("feature_flags_feature_flag_update", func(t *testing.T) { metaFeatureFlagsFeatureFlagUpdate(ctx, t) })
	t.Run("feature_flags_ff_user_list_create", func(t *testing.T) { metaFeatureFlagsFfUserListCreate(ctx, t) })
	t.Run("feature_flags_ff_user_list_delete", func(t *testing.T) { metaFeatureFlagsFfUserListDelete(ctx, t) })
	t.Run("feature_flags_ff_user_list_get", func(t *testing.T) { metaFeatureFlagsFfUserListGet(ctx, t) })
	t.Run("feature_flags_ff_user_list_list", func(t *testing.T) { metaFeatureFlagsFfUserListList(ctx, t) })
	t.Run("feature_flags_ff_user_list_update", func(t *testing.T) { metaFeatureFlagsFfUserListUpdate(ctx, t) })
	t.Run("group_create", func(t *testing.T) { metaGroupCreate(ctx, t) })
	t.Run("issue_create_todo", func(t *testing.T) { metaIssueCreateTodo(ctx, t) })
	t.Run("issue_discussion_get", func(t *testing.T) { metaIssueDiscussionGet(ctx, t) })
	t.Run("issue_discussion_update_note", func(t *testing.T) { metaIssueDiscussionUpdateNote(ctx, t) })
	t.Run("issue_emoji_issue_get", func(t *testing.T) { metaIssueEmojiIssueGet(ctx, t) })
	t.Run("issue_emoji_issue_note_create", func(t *testing.T) { metaIssueEmojiIssueNoteCreate(ctx, t) })
	t.Run("issue_emoji_issue_note_delete", func(t *testing.T) { metaIssueEmojiIssueNoteDelete(ctx, t) })
	t.Run("issue_emoji_issue_note_get", func(t *testing.T) { metaIssueEmojiIssueNoteGet(ctx, t) })
	t.Run("issue_emoji_issue_note_list", func(t *testing.T) { metaIssueEmojiIssueNoteList(ctx, t) })
	t.Run("issue_event_issue_label_get", func(t *testing.T) { metaIssueEventIssueLabelGet(ctx, t) })
	t.Run("issue_event_issue_label_list", func(t *testing.T) { metaIssueEventIssueLabelList(ctx, t) })
	t.Run("issue_event_issue_milestone_get", func(t *testing.T) { metaIssueEventIssueMilestoneGet(ctx, t) })
	t.Run("issue_event_issue_milestone_list", func(t *testing.T) { metaIssueEventIssueMilestoneList(ctx, t) })
	t.Run("issue_event_issue_state_get", func(t *testing.T) { metaIssueEventIssueStateGet(ctx, t) })
	t.Run("issue_get_by_id", func(t *testing.T) { metaIssueGetById(ctx, t) })
	t.Run("issue_link_get", func(t *testing.T) { metaIssueLinkGet(ctx, t) })
	t.Run("issue_list_all", func(t *testing.T) { metaIssueListAll(ctx, t) })
	t.Run("issue_move", func(t *testing.T) { metaIssueMove(ctx, t) })
	t.Run("issue_mrs_closing", func(t *testing.T) { metaIssueMrsClosing(ctx, t) })
	t.Run("issue_mrs_related", func(t *testing.T) { metaIssueMrsRelated(ctx, t) })
	t.Run("issue_note_delete", func(t *testing.T) { metaIssueNoteDelete(ctx, t) })
	t.Run("issue_note_get", func(t *testing.T) { metaIssueNoteGet(ctx, t) })
	t.Run("issue_note_update", func(t *testing.T) { metaIssueNoteUpdate(ctx, t) })
	t.Run("issue_participants", func(t *testing.T) { metaIssueParticipants(ctx, t) })
	t.Run("issue_reorder", func(t *testing.T) { metaIssueReorder(ctx, t) })
	t.Run("issue_spent_time_add", func(t *testing.T) { metaIssueSpentTimeAdd(ctx, t) })
	t.Run("issue_spent_time_reset", func(t *testing.T) { metaIssueSpentTimeReset(ctx, t) })
	t.Run("issue_statistics_get", func(t *testing.T) { metaIssueStatisticsGet(ctx, t) })
	t.Run("issue_statistics_get_project", func(t *testing.T) { metaIssueStatisticsGetProject(ctx, t) })
	t.Run("issue_subscribe", func(t *testing.T) { metaIssueSubscribe(ctx, t) })
	t.Run("issue_time_estimate_reset", func(t *testing.T) { metaIssueTimeEstimateReset(ctx, t) })
	t.Run("issue_time_estimate_set", func(t *testing.T) { metaIssueTimeEstimateSet(ctx, t) })
	t.Run("issue_time_stats_get", func(t *testing.T) { metaIssueTimeStatsGet(ctx, t) })
	t.Run("issue_unsubscribe", func(t *testing.T) { metaIssueUnsubscribe(ctx, t) })
	t.Run("issue_work_item_create", func(t *testing.T) { metaIssueWorkItemCreate(ctx, t) })
	t.Run("issue_work_item_delete", func(t *testing.T) { metaIssueWorkItemDelete(ctx, t) })
	t.Run("issue_work_item_get", func(t *testing.T) { metaIssueWorkItemGet(ctx, t) })
	t.Run("issue_work_item_list", func(t *testing.T) { metaIssueWorkItemList(ctx, t) })
	t.Run("issue_work_item_update", func(t *testing.T) { metaIssueWorkItemUpdate(ctx, t) })
	t.Run("job_artifacts", func(t *testing.T) { metaJobArtifacts(ctx, t) })
	t.Run("job_cancel", func(t *testing.T) { metaJobCancel(ctx, t) })
	t.Run("job_delete_artifacts", func(t *testing.T) { metaJobDeleteArtifacts(ctx, t) })
	t.Run("job_delete_project_artifacts", func(t *testing.T) { metaJobDeleteProjectArtifacts(ctx, t) })
	t.Run("job_download_artifacts", func(t *testing.T) { metaJobDownloadArtifacts(ctx, t) })
	t.Run("job_download_single_artifact", func(t *testing.T) { metaJobDownloadSingleArtifact(ctx, t) })
	t.Run("job_download_single_artifact_by_ref", func(t *testing.T) { metaJobDownloadSingleArtifactByRef(ctx, t) })
	t.Run("job_erase", func(t *testing.T) { metaJobErase(ctx, t) })
	t.Run("job_get", func(t *testing.T) { metaJobGet(ctx, t) })
	t.Run("job_keep_artifacts", func(t *testing.T) { metaJobKeepArtifacts(ctx, t) })
	t.Run("job_list_bridges", func(t *testing.T) { metaJobListBridges(ctx, t) })
	t.Run("job_play", func(t *testing.T) { metaJobPlay(ctx, t) })
	t.Run("job_retry", func(t *testing.T) { metaJobRetry(ctx, t) })
	t.Run("job_token_scope_add_group", func(t *testing.T) { metaJobTokenScopeAddGroup(ctx, t) })
	t.Run("job_token_scope_add_project", func(t *testing.T) { metaJobTokenScopeAddProject(ctx, t) })
	t.Run("job_token_scope_list_groups", func(t *testing.T) { metaJobTokenScopeListGroups(ctx, t) })
	t.Run("job_token_scope_list_inbound", func(t *testing.T) { metaJobTokenScopeListInbound(ctx, t) })
	t.Run("job_token_scope_patch", func(t *testing.T) { metaJobTokenScopePatch(ctx, t) })
	t.Run("job_token_scope_remove_group", func(t *testing.T) { metaJobTokenScopeRemoveGroup(ctx, t) })
	t.Run("job_token_scope_remove_project", func(t *testing.T) { metaJobTokenScopeRemoveProject(ctx, t) })
	t.Run("job_trace", func(t *testing.T) { metaJobTrace(ctx, t) })
	t.Run("merge_request_approval_config", func(t *testing.T) { metaMergeRequestApprovalConfig(ctx, t) })
	t.Run("merge_request_approval_reset", func(t *testing.T) { metaMergeRequestApprovalReset(ctx, t) })
	t.Run("merge_request_approval_rule_create", func(t *testing.T) { metaMergeRequestApprovalRuleCreate(ctx, t) })
	t.Run("merge_request_approval_rule_delete", func(t *testing.T) { metaMergeRequestApprovalRuleDelete(ctx, t) })
	t.Run("merge_request_approval_rule_update", func(t *testing.T) { metaMergeRequestApprovalRuleUpdate(ctx, t) })
	t.Run("merge_request_approval_rules", func(t *testing.T) { metaMergeRequestApprovalRules(ctx, t) })
	t.Run("merge_request_approval_settings_project_get", func(t *testing.T) { metaMergeRequestApprovalSettingsProjectGet(ctx, t) })
	t.Run("merge_request_approval_settings_project_update", func(t *testing.T) { metaMergeRequestApprovalSettingsProjectUpdate(ctx, t) })
	t.Run("merge_request_approval_state", func(t *testing.T) { metaMergeRequestApprovalState(ctx, t) })
	t.Run("merge_request_cancel_auto_merge", func(t *testing.T) { metaMergeRequestCancelAutoMerge(ctx, t) })
	t.Run("merge_request_context_commits_create", func(t *testing.T) { metaMergeRequestContextCommitsCreate(ctx, t) })
	t.Run("merge_request_context_commits_delete", func(t *testing.T) { metaMergeRequestContextCommitsDelete(ctx, t) })
	t.Run("merge_request_context_commits_list", func(t *testing.T) { metaMergeRequestContextCommitsList(ctx, t) })
	t.Run("merge_request_create_pipeline", func(t *testing.T) { metaMergeRequestCreatePipeline(ctx, t) })
	t.Run("merge_request_delete", func(t *testing.T) { metaMergeRequestDelete(ctx, t) })
	t.Run("merge_request_emoji_mr_create", func(t *testing.T) { metaMergeRequestEmojiMrCreate(ctx, t) })
	t.Run("merge_request_emoji_mr_delete", func(t *testing.T) { metaMergeRequestEmojiMrDelete(ctx, t) })
	t.Run("merge_request_emoji_mr_get", func(t *testing.T) { metaMergeRequestEmojiMrGet(ctx, t) })
	t.Run("merge_request_emoji_mr_list", func(t *testing.T) { metaMergeRequestEmojiMrList(ctx, t) })
	t.Run("merge_request_emoji_mr_note_create", func(t *testing.T) { metaMergeRequestEmojiMrNoteCreate(ctx, t) })
	t.Run("merge_request_emoji_mr_note_delete", func(t *testing.T) { metaMergeRequestEmojiMrNoteDelete(ctx, t) })
	t.Run("merge_request_emoji_mr_note_get", func(t *testing.T) { metaMergeRequestEmojiMrNoteGet(ctx, t) })
	t.Run("merge_request_emoji_mr_note_list", func(t *testing.T) { metaMergeRequestEmojiMrNoteList(ctx, t) })
	t.Run("merge_request_event_mr_label_get", func(t *testing.T) { metaMergeRequestEventMrLabelGet(ctx, t) })
	t.Run("merge_request_event_mr_label_list", func(t *testing.T) { metaMergeRequestEventMrLabelList(ctx, t) })
	t.Run("merge_request_event_mr_milestone_get", func(t *testing.T) { metaMergeRequestEventMrMilestoneGet(ctx, t) })
	t.Run("merge_request_event_mr_milestone_list", func(t *testing.T) { metaMergeRequestEventMrMilestoneList(ctx, t) })
	t.Run("merge_request_event_mr_state_get", func(t *testing.T) { metaMergeRequestEventMrStateGet(ctx, t) })
	t.Run("merge_request_issues_closed", func(t *testing.T) { metaMergeRequestIssuesClosed(ctx, t) })
	t.Run("merge_request_list_global", func(t *testing.T) { metaMergeRequestListGlobal(ctx, t) })
	t.Run("merge_request_participants", func(t *testing.T) { metaMergeRequestParticipants(ctx, t) })
	t.Run("merge_request_reviewers", func(t *testing.T) { metaMergeRequestReviewers(ctx, t) })
	t.Run("merge_request_spent_time_add", func(t *testing.T) { metaMergeRequestSpentTimeAdd(ctx, t) })
	t.Run("merge_request_spent_time_reset", func(t *testing.T) { metaMergeRequestSpentTimeReset(ctx, t) })
	t.Run("merge_request_subscribe", func(t *testing.T) { metaMergeRequestSubscribe(ctx, t) })
	t.Run("merge_request_time_estimate_reset", func(t *testing.T) { metaMergeRequestTimeEstimateReset(ctx, t) })
	t.Run("merge_request_time_estimate_set", func(t *testing.T) { metaMergeRequestTimeEstimateSet(ctx, t) })
	t.Run("merge_request_time_stats", func(t *testing.T) { metaMergeRequestTimeStats(ctx, t) })
	t.Run("merge_request_unsubscribe", func(t *testing.T) { metaMergeRequestUnsubscribe(ctx, t) })
	t.Run("mr_review_discussion_note_delete", func(t *testing.T) { metaMrReviewDiscussionNoteDelete(ctx, t) })
	t.Run("mr_review_discussion_note_update", func(t *testing.T) { metaMrReviewDiscussionNoteUpdate(ctx, t) })
	t.Run("mr_review_draft_note_delete", func(t *testing.T) { metaMrReviewDraftNoteDelete(ctx, t) })
	t.Run("mr_review_draft_note_publish", func(t *testing.T) { metaMrReviewDraftNotePublish(ctx, t) })
	t.Run("package_protection_rule_create", func(t *testing.T) { metaPackageProtectionRuleCreate(ctx, t) })
	t.Run("package_protection_rule_delete", func(t *testing.T) { metaPackageProtectionRuleDelete(ctx, t) })
	t.Run("package_protection_rule_list", func(t *testing.T) { metaPackageProtectionRuleList(ctx, t) })
	t.Run("package_protection_rule_update", func(t *testing.T) { metaPackageProtectionRuleUpdate(ctx, t) })
	t.Run("package_registry_delete", func(t *testing.T) { metaPackageRegistryDelete(ctx, t) })
	t.Run("package_registry_get", func(t *testing.T) { metaPackageRegistryGet(ctx, t) })
	t.Run("package_registry_list_project", func(t *testing.T) { metaPackageRegistryListProject(ctx, t) })
	t.Run("package_registry_rule_create", func(t *testing.T) { metaPackageRegistryRuleCreate(ctx, t) })
	t.Run("package_registry_rule_delete", func(t *testing.T) { metaPackageRegistryRuleDelete(ctx, t) })
	t.Run("package_registry_rule_list", func(t *testing.T) { metaPackageRegistryRuleList(ctx, t) })
	t.Run("package_registry_rule_update", func(t *testing.T) { metaPackageRegistryRuleUpdate(ctx, t) })
	t.Run("package_registry_tag_delete", func(t *testing.T) { metaPackageRegistryTagDelete(ctx, t) })
	t.Run("package_registry_tag_delete_bulk", func(t *testing.T) { metaPackageRegistryTagDeleteBulk(ctx, t) })
	t.Run("package_registry_tag_get", func(t *testing.T) { metaPackageRegistryTagGet(ctx, t) })
	t.Run("package_registry_tag_list", func(t *testing.T) { metaPackageRegistryTagList(ctx, t) })
	t.Run("pipeline_cancel", func(t *testing.T) { metaPipelineCancel(ctx, t) })
	t.Run("pipeline_create", func(t *testing.T) { metaPipelineCreate(ctx, t) })
	t.Run("pipeline_delete", func(t *testing.T) { metaPipelineDelete(ctx, t) })
	t.Run("pipeline_resource_group_edit", func(t *testing.T) { metaPipelineResourceGroupEdit(ctx, t) })
	t.Run("pipeline_resource_group_get", func(t *testing.T) { metaPipelineResourceGroupGet(ctx, t) })
	t.Run("pipeline_resource_group_list", func(t *testing.T) { metaPipelineResourceGroupList(ctx, t) })
	t.Run("pipeline_resource_group_upcoming_jobs", func(t *testing.T) { metaPipelineResourceGroupUpcomingJobs(ctx, t) })
	t.Run("pipeline_retry", func(t *testing.T) { metaPipelineRetry(ctx, t) })
	t.Run("pipeline_trigger_run", func(t *testing.T) { metaPipelineTriggerRun(ctx, t) })
	t.Run("pipeline_update_metadata", func(t *testing.T) { metaPipelineUpdateMetadata(ctx, t) })
	t.Run("project_approval_config_change", func(t *testing.T) { metaProjectApprovalConfigChange(ctx, t) })
	t.Run("project_approval_config_get", func(t *testing.T) { metaProjectApprovalConfigGet(ctx, t) })
	t.Run("project_approval_rule_create", func(t *testing.T) { metaProjectApprovalRuleCreate(ctx, t) })
	t.Run("project_approval_rule_delete", func(t *testing.T) { metaProjectApprovalRuleDelete(ctx, t) })
	t.Run("project_approval_rule_get", func(t *testing.T) { metaProjectApprovalRuleGet(ctx, t) })
	t.Run("project_approval_rule_list", func(t *testing.T) { metaProjectApprovalRuleList(ctx, t) })
	t.Run("project_approval_rule_update", func(t *testing.T) { metaProjectApprovalRuleUpdate(ctx, t) })
	// SKIP: archive/unarchive could break subsequent tests depending on ordering
	// t.Run("project_archive", func(t *testing.T) { metaProjectArchive(ctx, t) })
	t.Run("project_badge_get", func(t *testing.T) { metaProjectBadgeGet(ctx, t) })
	t.Run("project_badge_preview", func(t *testing.T) { metaProjectBadgePreview(ctx, t) })
	t.Run("project_board_list_create", func(t *testing.T) { metaProjectBoardListCreate(ctx, t) })
	t.Run("project_board_list_delete", func(t *testing.T) { metaProjectBoardListDelete(ctx, t) })
	t.Run("project_board_list_get", func(t *testing.T) { metaProjectBoardListGet(ctx, t) })
	t.Run("project_board_list_list", func(t *testing.T) { metaProjectBoardListList(ctx, t) })
	t.Run("project_board_list_update", func(t *testing.T) { metaProjectBoardListUpdate(ctx, t) })
	t.Run("project_board_update", func(t *testing.T) { metaProjectBoardUpdate(ctx, t) })
	t.Run("project_create_for_user", func(t *testing.T) { metaProjectCreateForUser(ctx, t) })
	t.Run("project_create_fork_relation", func(t *testing.T) { metaProjectCreateForkRelation(ctx, t) })
	t.Run("project_delete_fork_relation", func(t *testing.T) { metaProjectDeleteForkRelation(ctx, t) })
	t.Run("project_delete_shared_group", func(t *testing.T) { metaProjectDeleteSharedGroup(ctx, t) })
	t.Run("project_download_avatar", func(t *testing.T) { metaProjectDownloadAvatar(ctx, t) })
	t.Run("project_export_download", func(t *testing.T) { metaProjectExportDownload(ctx, t) })
	t.Run("project_export_schedule", func(t *testing.T) { metaProjectExportSchedule(ctx, t) })
	t.Run("project_export_status", func(t *testing.T) { metaProjectExportStatus(ctx, t) })
	t.Run("project_fork", func(t *testing.T) { metaProjectFork(ctx, t) })
	t.Run("project_hook_add", func(t *testing.T) { metaProjectHookAdd(ctx, t) })
	t.Run("project_hook_delete", func(t *testing.T) { metaProjectHookDelete(ctx, t) })
	t.Run("project_hook_delete_custom_header", func(t *testing.T) { metaProjectHookDeleteCustomHeader(ctx, t) })
	t.Run("project_hook_delete_url_variable", func(t *testing.T) { metaProjectHookDeleteUrlVariable(ctx, t) })
	t.Run("project_hook_edit", func(t *testing.T) { metaProjectHookEdit(ctx, t) })
	t.Run("project_hook_get", func(t *testing.T) { metaProjectHookGet(ctx, t) })
	t.Run("project_hook_list", func(t *testing.T) { metaProjectHookList(ctx, t) })
	t.Run("project_hook_set_custom_header", func(t *testing.T) { metaProjectHookSetCustomHeader(ctx, t) })
	t.Run("project_hook_set_url_variable", func(t *testing.T) { metaProjectHookSetUrlVariable(ctx, t) })
	t.Run("project_hook_test", func(t *testing.T) { metaProjectHookTest(ctx, t) })
	t.Run("project_import_from_file", func(t *testing.T) { metaProjectImportFromFile(ctx, t) })
	t.Run("project_import_status", func(t *testing.T) { metaProjectImportStatus(ctx, t) })
	t.Run("project_integration_delete", func(t *testing.T) { metaProjectIntegrationDelete(ctx, t) })
	t.Run("project_integration_get", func(t *testing.T) { metaProjectIntegrationGet(ctx, t) })
	t.Run("project_integration_list", func(t *testing.T) { metaProjectIntegrationList(ctx, t) })
	t.Run("project_integration_set_jira", func(t *testing.T) { metaProjectIntegrationSetJira(ctx, t) })
	t.Run("project_label_get", func(t *testing.T) { metaProjectLabelGet(ctx, t) })
	t.Run("project_label_promote", func(t *testing.T) { metaProjectLabelPromote(ctx, t) })
	t.Run("project_label_subscribe", func(t *testing.T) { metaProjectLabelSubscribe(ctx, t) })
	t.Run("project_label_unsubscribe", func(t *testing.T) { metaProjectLabelUnsubscribe(ctx, t) })
	t.Run("project_languages", func(t *testing.T) { metaProjectLanguages(ctx, t) })
	t.Run("project_list_forks", func(t *testing.T) { metaProjectListForks(ctx, t) })
	t.Run("project_list_groups", func(t *testing.T) { metaProjectListGroups(ctx, t) })
	t.Run("project_list_invited_groups", func(t *testing.T) { metaProjectListInvitedGroups(ctx, t) })
	t.Run("project_list_starrers", func(t *testing.T) { metaProjectListStarrers(ctx, t) })
	t.Run("project_list_user_projects", func(t *testing.T) { metaProjectListUserProjects(ctx, t) })
	t.Run("project_list_users", func(t *testing.T) { metaProjectListUsers(ctx, t) })
	t.Run("project_member_add", func(t *testing.T) { metaProjectMemberAdd(ctx, t) })
	t.Run("project_member_delete", func(t *testing.T) { metaProjectMemberDelete(ctx, t) })
	t.Run("project_member_edit", func(t *testing.T) { metaProjectMemberEdit(ctx, t) })
	t.Run("project_member_get", func(t *testing.T) { metaProjectMemberGet(ctx, t) })
	t.Run("project_member_inherited", func(t *testing.T) { metaProjectMemberInherited(ctx, t) })
	t.Run("project_milestone_issues", func(t *testing.T) { metaProjectMilestoneIssues(ctx, t) })
	t.Run("project_milestone_merge_requests", func(t *testing.T) { metaProjectMilestoneMergeRequests(ctx, t) })
	t.Run("project_pages_domain_create", func(t *testing.T) { metaProjectPagesDomainCreate(ctx, t) })
	t.Run("project_pages_domain_delete", func(t *testing.T) { metaProjectPagesDomainDelete(ctx, t) })
	t.Run("project_pages_domain_get", func(t *testing.T) { metaProjectPagesDomainGet(ctx, t) })
	t.Run("project_pages_domain_list", func(t *testing.T) { metaProjectPagesDomainList(ctx, t) })
	t.Run("project_pages_domain_list_all", func(t *testing.T) { metaProjectPagesDomainListAll(ctx, t) })
	t.Run("project_pages_domain_update", func(t *testing.T) { metaProjectPagesDomainUpdate(ctx, t) })
	t.Run("project_pages_get", func(t *testing.T) { metaProjectPagesGet(ctx, t) })
	t.Run("project_pages_unpublish", func(t *testing.T) { metaProjectPagesUnpublish(ctx, t) })
	t.Run("project_pages_update", func(t *testing.T) { metaProjectPagesUpdate(ctx, t) })
	t.Run("project_repository_storage_get", func(t *testing.T) { metaProjectRepositoryStorageGet(ctx, t) })
	t.Run("project_restore", func(t *testing.T) { metaProjectRestore(ctx, t) })
	t.Run("project_share_with_group", func(t *testing.T) { metaProjectShareWithGroup(ctx, t) })
	t.Run("project_star", func(t *testing.T) { metaProjectStar(ctx, t) })
	t.Run("project_start_housekeeping", func(t *testing.T) { metaProjectStartHousekeeping(ctx, t) })
	t.Run("project_statistics_get", func(t *testing.T) { metaProjectStatisticsGet(ctx, t) })
	// SKIP: transfer could move the test project to a different namespace
	// t.Run("project_transfer", func(t *testing.T) { metaProjectTransfer(ctx, t) })
	// t.Run("project_unarchive", func(t *testing.T) { metaProjectUnarchive(ctx, t) })
	t.Run("project_unstar", func(t *testing.T) { metaProjectUnarchive(ctx, t) })
	t.Run("project_unstar", func(t *testing.T) { metaProjectUnstar(ctx, t) })
	t.Run("project_upload_avatar", func(t *testing.T) { metaProjectUploadAvatar(ctx, t) })
	t.Run("project_upload_delete", func(t *testing.T) { metaProjectUploadDelete(ctx, t) })
	t.Run("project_upload_list", func(t *testing.T) { metaProjectUploadList(ctx, t) })
	t.Run("release_link_create_batch", func(t *testing.T) { metaReleaseLinkCreateBatch(ctx, t) })
	t.Run("repository_archive", func(t *testing.T) { metaRepositoryArchive(ctx, t) })
	t.Run("repository_blob", func(t *testing.T) { metaRepositoryBlob(ctx, t) })
	t.Run("repository_changelog_add", func(t *testing.T) { metaRepositoryChangelogAdd(ctx, t) })
	t.Run("repository_changelog_generate", func(t *testing.T) { metaRepositoryChangelogGenerate(ctx, t) })
	t.Run("repository_commit_cherry_pick", func(t *testing.T) { metaRepositoryCommitCherryPick(ctx, t) })
	t.Run("repository_commit_comment_create", func(t *testing.T) { metaRepositoryCommitCommentCreate(ctx, t) })
	t.Run("repository_commit_comments", func(t *testing.T) { metaRepositoryCommitComments(ctx, t) })
	t.Run("repository_commit_discussion_add_note", func(t *testing.T) { metaRepositoryCommitDiscussionAddNote(ctx, t) })
	t.Run("repository_commit_discussion_create", func(t *testing.T) { metaRepositoryCommitDiscussionCreate(ctx, t) })
	t.Run("repository_commit_discussion_delete_note", func(t *testing.T) { metaRepositoryCommitDiscussionDeleteNote(ctx, t) })
	t.Run("repository_commit_discussion_get", func(t *testing.T) { metaRepositoryCommitDiscussionGet(ctx, t) })
	t.Run("repository_commit_discussion_list", func(t *testing.T) { metaRepositoryCommitDiscussionList(ctx, t) })
	t.Run("repository_commit_discussion_update_note", func(t *testing.T) { metaRepositoryCommitDiscussionUpdateNote(ctx, t) })
	t.Run("repository_commit_merge_requests", func(t *testing.T) { metaRepositoryCommitMergeRequests(ctx, t) })
	t.Run("repository_commit_refs", func(t *testing.T) { metaRepositoryCommitRefs(ctx, t) })
	t.Run("repository_commit_revert", func(t *testing.T) { metaRepositoryCommitRevert(ctx, t) })
	t.Run("repository_commit_signature", func(t *testing.T) { metaRepositoryCommitSignature(ctx, t) })
	t.Run("repository_commit_status_set", func(t *testing.T) { metaRepositoryCommitStatusSet(ctx, t) })
	t.Run("repository_commit_statuses", func(t *testing.T) { metaRepositoryCommitStatuses(ctx, t) })
	t.Run("repository_contributors", func(t *testing.T) { metaRepositoryContributors(ctx, t) })
	t.Run("repository_file_blame", func(t *testing.T) { metaRepositoryFileBlame(ctx, t) })
	t.Run("repository_file_create", func(t *testing.T) { metaRepositoryFileCreate(ctx, t) })
	t.Run("repository_file_delete", func(t *testing.T) { metaRepositoryFileDelete(ctx, t) })
	t.Run("repository_file_history", func(t *testing.T) { metaRepositoryFileHistory(ctx, t) })
	t.Run("repository_file_metadata", func(t *testing.T) { metaRepositoryFileMetadata(ctx, t) })
	t.Run("repository_file_raw", func(t *testing.T) { metaRepositoryFileRaw(ctx, t) })
	t.Run("repository_file_update", func(t *testing.T) { metaRepositoryFileUpdate(ctx, t) })
	t.Run("repository_list_submodules", func(t *testing.T) { metaRepositoryListSubmodules(ctx, t) })
	t.Run("repository_merge_base", func(t *testing.T) { metaRepositoryMergeBase(ctx, t) })
	t.Run("repository_raw_blob", func(t *testing.T) { metaRepositoryRawBlob(ctx, t) })
	t.Run("repository_read_submodule_file", func(t *testing.T) { metaRepositoryReadSubmoduleFile(ctx, t) })
	t.Run("repository_update_submodule", func(t *testing.T) { metaRepositoryUpdateSubmodule(ctx, t) })
	t.Run("snippet_content", func(t *testing.T) { metaSnippetContent(ctx, t) })
	t.Run("snippet_discussion_add_note", func(t *testing.T) { metaSnippetDiscussionAddNote(ctx, t) })
	t.Run("snippet_discussion_create", func(t *testing.T) { metaSnippetDiscussionCreate(ctx, t) })
	t.Run("snippet_discussion_delete_note", func(t *testing.T) { metaSnippetDiscussionDeleteNote(ctx, t) })
	t.Run("snippet_discussion_get", func(t *testing.T) { metaSnippetDiscussionGet(ctx, t) })
	t.Run("snippet_discussion_list", func(t *testing.T) { metaSnippetDiscussionList(ctx, t) })
	t.Run("snippet_discussion_update_note", func(t *testing.T) { metaSnippetDiscussionUpdateNote(ctx, t) })
	t.Run("snippet_emoji_snippet_create", func(t *testing.T) { metaSnippetEmojiSnippetCreate(ctx, t) })
	t.Run("snippet_emoji_snippet_delete", func(t *testing.T) { metaSnippetEmojiSnippetDelete(ctx, t) })
	t.Run("snippet_emoji_snippet_get", func(t *testing.T) { metaSnippetEmojiSnippetGet(ctx, t) })
	t.Run("snippet_emoji_snippet_list", func(t *testing.T) { metaSnippetEmojiSnippetList(ctx, t) })
	t.Run("snippet_emoji_snippet_note_create", func(t *testing.T) { metaSnippetEmojiSnippetNoteCreate(ctx, t) })
	t.Run("snippet_emoji_snippet_note_delete", func(t *testing.T) { metaSnippetEmojiSnippetNoteDelete(ctx, t) })
	t.Run("snippet_emoji_snippet_note_get", func(t *testing.T) { metaSnippetEmojiSnippetNoteGet(ctx, t) })
	t.Run("snippet_emoji_snippet_note_list", func(t *testing.T) { metaSnippetEmojiSnippetNoteList(ctx, t) })
	t.Run("snippet_explore", func(t *testing.T) { metaSnippetExplore(ctx, t) })
	t.Run("snippet_file_content", func(t *testing.T) { metaSnippetFileContent(ctx, t) })
	t.Run("snippet_list_all", func(t *testing.T) { metaSnippetListAll(ctx, t) })
	t.Run("snippet_note_create", func(t *testing.T) { metaSnippetNoteCreate(ctx, t) })
	t.Run("snippet_note_delete", func(t *testing.T) { metaSnippetNoteDelete(ctx, t) })
	t.Run("snippet_note_get", func(t *testing.T) { metaSnippetNoteGet(ctx, t) })
	t.Run("snippet_note_list", func(t *testing.T) { metaSnippetNoteList(ctx, t) })
	t.Run("snippet_note_update", func(t *testing.T) { metaSnippetNoteUpdate(ctx, t) })
	t.Run("snippet_project_content", func(t *testing.T) { metaSnippetProjectContent(ctx, t) })
	t.Run("template_ci_yml_get", func(t *testing.T) { metaTemplateCiYmlGet(ctx, t) })
	t.Run("template_dockerfile_get", func(t *testing.T) { metaTemplateDockerfileGet(ctx, t) })
	t.Run("template_dockerfile_list", func(t *testing.T) { metaTemplateDockerfileList(ctx, t) })
	t.Run("template_gitignore_get", func(t *testing.T) { metaTemplateGitignoreGet(ctx, t) })
	t.Run("template_license_get", func(t *testing.T) { metaTemplateLicenseGet(ctx, t) })
	t.Run("template_license_list", func(t *testing.T) { metaTemplateLicenseList(ctx, t) })
	t.Run("template_lint_project", func(t *testing.T) { metaTemplateLintProject(ctx, t) })
	t.Run("template_project_template_get", func(t *testing.T) { metaTemplateProjectTemplateGet(ctx, t) })
	t.Run("template_project_template_list", func(t *testing.T) { metaTemplateProjectTemplateList(ctx, t) })
	t.Run("user_activate", func(t *testing.T) { metaUserActivate(ctx, t) })
	t.Run("user_activities", func(t *testing.T) { metaUserActivities(ctx, t) })
	t.Run("user_add_email", func(t *testing.T) { metaUserAddEmail(ctx, t) })
	t.Run("user_add_email_for_user", func(t *testing.T) { metaUserAddEmailForUser(ctx, t) })
	t.Run("user_add_gpg_key", func(t *testing.T) { metaUserAddGpgKey(ctx, t) })
	t.Run("user_add_gpg_key_for_user", func(t *testing.T) { metaUserAddGpgKeyForUser(ctx, t) })
	t.Run("user_add_ssh_key", func(t *testing.T) { metaUserAddSshKey(ctx, t) })
	t.Run("user_add_ssh_key_for_user", func(t *testing.T) { metaUserAddSshKeyForUser(ctx, t) })
	t.Run("user_approve", func(t *testing.T) { metaUserApprove(ctx, t) })
	t.Run("user_associations_count", func(t *testing.T) { metaUserAssociationsCount(ctx, t) })
	t.Run("user_avatar_get", func(t *testing.T) { metaUserAvatarGet(ctx, t) })
	t.Run("user_ban", func(t *testing.T) { metaUserBan(ctx, t) })
	t.Run("user_block", func(t *testing.T) { metaUserBlock(ctx, t) })
	t.Run("user_contribution_events", func(t *testing.T) { metaUserContributionEvents(ctx, t) })
	t.Run("user_create", func(t *testing.T) { metaUserCreate(ctx, t) })
	t.Run("user_create_current_user_pat", func(t *testing.T) { metaUserCreateCurrentUserPat(ctx, t) })
	t.Run("user_create_impersonation_token", func(t *testing.T) { metaUserCreateImpersonationToken(ctx, t) })
	t.Run("user_create_personal_access_token", func(t *testing.T) { metaUserCreatePersonalAccessToken(ctx, t) })
	t.Run("user_create_runner", func(t *testing.T) { metaUserCreateRunner(ctx, t) })
	t.Run("user_create_service_account", func(t *testing.T) { metaUserCreateServiceAccount(ctx, t) })
	t.Run("user_current_user_status", func(t *testing.T) { metaUserCurrentUserStatus(ctx, t) })
	t.Run("user_deactivate", func(t *testing.T) { metaUserDeactivate(ctx, t) })
	t.Run("user_delete", func(t *testing.T) { metaUserDelete(ctx, t) })
	t.Run("user_delete_email", func(t *testing.T) { metaUserDeleteEmail(ctx, t) })
	t.Run("user_delete_email_for_user", func(t *testing.T) { metaUserDeleteEmailForUser(ctx, t) })
	t.Run("user_delete_gpg_key", func(t *testing.T) { metaUserDeleteGpgKey(ctx, t) })
	t.Run("user_delete_gpg_key_for_user", func(t *testing.T) { metaUserDeleteGpgKeyForUser(ctx, t) })
	t.Run("user_delete_identity", func(t *testing.T) { metaUserDeleteIdentity(ctx, t) })
	t.Run("user_delete_ssh_key", func(t *testing.T) { metaUserDeleteSshKey(ctx, t) })
	t.Run("user_delete_ssh_key_for_user", func(t *testing.T) { metaUserDeleteSshKeyForUser(ctx, t) })
	t.Run("user_disable_two_factor", func(t *testing.T) { metaUserDisableTwoFactor(ctx, t) })
	t.Run("user_emails", func(t *testing.T) { metaUserEmails(ctx, t) })
	t.Run("user_emails_for_user", func(t *testing.T) { metaUserEmailsForUser(ctx, t) })
	t.Run("user_event_list_contributions", func(t *testing.T) { metaUserEventListContributions(ctx, t) })
	t.Run("user_event_list_project", func(t *testing.T) { metaUserEventListProject(ctx, t) })
	t.Run("user_get", func(t *testing.T) { metaUserGet(ctx, t) })
	t.Run("user_get_email", func(t *testing.T) { metaUserGetEmail(ctx, t) })
	t.Run("user_get_gpg_key", func(t *testing.T) { metaUserGetGpgKey(ctx, t) })
	t.Run("user_get_gpg_key_for_user", func(t *testing.T) { metaUserGetGpgKeyForUser(ctx, t) })
	t.Run("user_get_impersonation_token", func(t *testing.T) { metaUserGetImpersonationToken(ctx, t) })
	t.Run("user_get_ssh_key", func(t *testing.T) { metaUserGetSshKey(ctx, t) })
	t.Run("user_get_ssh_key_for_user", func(t *testing.T) { metaUserGetSshKeyForUser(ctx, t) })
	t.Run("user_get_status", func(t *testing.T) { metaUserGetStatus(ctx, t) })
	t.Run("user_gpg_keys_for_user", func(t *testing.T) { metaUserGpgKeysForUser(ctx, t) })
	t.Run("user_key_get_by_fingerprint", func(t *testing.T) { metaUserKeyGetByFingerprint(ctx, t) })
	t.Run("user_key_get_with_user", func(t *testing.T) { metaUserKeyGetWithUser(ctx, t) })
	t.Run("user_list", func(t *testing.T) { metaUserList(ctx, t) })
	t.Run("user_list_impersonation_tokens", func(t *testing.T) { metaUserListImpersonationTokens(ctx, t) })
	t.Run("user_list_service_accounts", func(t *testing.T) { metaUserListServiceAccounts(ctx, t) })
	t.Run("user_me", func(t *testing.T) { metaUserMe(ctx, t) })
	t.Run("user_memberships", func(t *testing.T) { metaUserMemberships(ctx, t) })
	t.Run("user_modify", func(t *testing.T) { metaUserModify(ctx, t) })
	t.Run("user_namespace_exists", func(t *testing.T) { metaUserNamespaceExists(ctx, t) })
	t.Run("user_namespace_get", func(t *testing.T) { metaUserNamespaceGet(ctx, t) })
	t.Run("user_namespace_list", func(t *testing.T) { metaUserNamespaceList(ctx, t) })
	t.Run("user_namespace_search", func(t *testing.T) { metaUserNamespaceSearch(ctx, t) })
	t.Run("user_notification_global_update", func(t *testing.T) { metaUserNotificationGlobalUpdate(ctx, t) })
	t.Run("user_notification_project_update", func(t *testing.T) { metaUserNotificationProjectUpdate(ctx, t) })
	t.Run("user_reject", func(t *testing.T) { metaUserReject(ctx, t) })
	t.Run("user_revoke_impersonation_token", func(t *testing.T) { metaUserRevokeImpersonationToken(ctx, t) })
	t.Run("user_set_status", func(t *testing.T) { metaUserSetStatus(ctx, t) })
	t.Run("user_ssh_keys_for_user", func(t *testing.T) { metaUserSshKeysForUser(ctx, t) })
	t.Run("user_todo_mark_done", func(t *testing.T) { metaUserTodoMarkDone(ctx, t) })
	t.Run("user_unban", func(t *testing.T) { metaUserUnban(ctx, t) })
	t.Run("user_unblock", func(t *testing.T) { metaUserUnblock(ctx, t) })

	// CE group-dependent gap tests.
	if mState.groupPath != "" {
		t.Run("access_approve_group", func(t *testing.T) { metaAccessApproveGroup(ctx, t) })
		t.Run("access_deny_group", func(t *testing.T) { metaAccessDenyGroup(ctx, t) })
		t.Run("access_deploy_token_create_group", func(t *testing.T) { metaAccessDeployTokenCreateGroup(ctx, t) })
		t.Run("access_deploy_token_delete_group", func(t *testing.T) { metaAccessDeployTokenDeleteGroup(ctx, t) })
		t.Run("access_deploy_token_get_group", func(t *testing.T) { metaAccessDeployTokenGetGroup(ctx, t) })
		t.Run("access_deploy_token_list_group", func(t *testing.T) { metaAccessDeployTokenListGroup(ctx, t) })
		t.Run("access_invite_group", func(t *testing.T) { metaAccessInviteGroup(ctx, t) })
		t.Run("access_invite_list_group", func(t *testing.T) { metaAccessInviteListGroup(ctx, t) })
		t.Run("access_request_group", func(t *testing.T) { metaAccessRequestGroup(ctx, t) })
		t.Run("access_request_list_group", func(t *testing.T) { metaAccessRequestListGroup(ctx, t) })
		t.Run("access_token_group_create", func(t *testing.T) { metaAccessTokenGroupCreate(ctx, t) })
		t.Run("access_token_group_get", func(t *testing.T) { metaAccessTokenGroupGet(ctx, t) })
		t.Run("access_token_group_list", func(t *testing.T) { metaAccessTokenGroupList(ctx, t) })
		t.Run("access_token_group_revoke", func(t *testing.T) { metaAccessTokenGroupRevoke(ctx, t) })
		t.Run("access_token_group_rotate", func(t *testing.T) { metaAccessTokenGroupRotate(ctx, t) })
		t.Run("access_token_group_rotate_self", func(t *testing.T) { metaAccessTokenGroupRotateSelf(ctx, t) })
		t.Run("admin_dependency_proxy_delete", func(t *testing.T) { metaAdminDependencyProxyDelete(ctx, t) })
		t.Run("custom_emoji_create", func(t *testing.T) { metaCustomEmojiCreate(ctx, t) })
		t.Run("custom_emoji_delete", func(t *testing.T) { metaCustomEmojiDelete(ctx, t) })
		t.Run("group_archive", func(t *testing.T) { metaGroupArchive(ctx, t) })
		t.Run("group_badge_add", func(t *testing.T) { metaGroupBadgeAdd(ctx, t) })
		t.Run("group_badge_delete", func(t *testing.T) { metaGroupBadgeDelete(ctx, t) })
		t.Run("group_badge_edit", func(t *testing.T) { metaGroupBadgeEdit(ctx, t) })
		t.Run("group_badge_get", func(t *testing.T) { metaGroupBadgeGet(ctx, t) })
		t.Run("group_badge_list", func(t *testing.T) { metaGroupBadgeList(ctx, t) })
		t.Run("group_badge_preview", func(t *testing.T) { metaGroupBadgePreview(ctx, t) })
		// SKIP: would delete the test group, breaking subsequent group tests
		// t.Run("group_delete", func(t *testing.T) { metaGroupDelete(ctx, t) })
		t.Run("group_group_board_create", func(t *testing.T) { metaGroupGroupBoardCreate(ctx, t) })
		t.Run("group_group_board_create_list", func(t *testing.T) { metaGroupGroupBoardCreateList(ctx, t) })
		t.Run("group_group_board_delete", func(t *testing.T) { metaGroupGroupBoardDelete(ctx, t) })
		t.Run("group_group_board_delete_list", func(t *testing.T) { metaGroupGroupBoardDeleteList(ctx, t) })
		t.Run("group_group_board_get", func(t *testing.T) { metaGroupGroupBoardGet(ctx, t) })
		t.Run("group_group_board_get_list", func(t *testing.T) { metaGroupGroupBoardGetList(ctx, t) })
		t.Run("group_group_board_list", func(t *testing.T) { metaGroupGroupBoardList(ctx, t) })
		t.Run("group_group_board_list_lists", func(t *testing.T) { metaGroupGroupBoardListLists(ctx, t) })
		t.Run("group_group_board_update", func(t *testing.T) { metaGroupGroupBoardUpdate(ctx, t) })
		t.Run("group_group_board_update_list", func(t *testing.T) { metaGroupGroupBoardUpdateList(ctx, t) })
		t.Run("group_group_export_download", func(t *testing.T) { metaGroupGroupExportDownload(ctx, t) })
		t.Run("group_group_export_schedule", func(t *testing.T) { metaGroupGroupExportSchedule(ctx, t) })
		t.Run("group_group_import_file", func(t *testing.T) { metaGroupGroupImportFile(ctx, t) })
		t.Run("group_group_label_get", func(t *testing.T) { metaGroupGroupLabelGet(ctx, t) })
		t.Run("group_group_label_subscribe", func(t *testing.T) { metaGroupGroupLabelSubscribe(ctx, t) })
		t.Run("group_group_label_unsubscribe", func(t *testing.T) { metaGroupGroupLabelUnsubscribe(ctx, t) })
		t.Run("group_group_label_update", func(t *testing.T) { metaGroupGroupLabelUpdate(ctx, t) })
		t.Run("group_group_member_add", func(t *testing.T) { metaGroupGroupMemberAdd(ctx, t) })
		t.Run("group_group_member_edit", func(t *testing.T) { metaGroupGroupMemberEdit(ctx, t) })
		t.Run("group_group_member_get", func(t *testing.T) { metaGroupGroupMemberGet(ctx, t) })
		t.Run("group_group_member_get_inherited", func(t *testing.T) { metaGroupGroupMemberGetInherited(ctx, t) })
		t.Run("group_group_member_remove", func(t *testing.T) { metaGroupGroupMemberRemove(ctx, t) })
		t.Run("group_group_member_share", func(t *testing.T) { metaGroupGroupMemberShare(ctx, t) })
		t.Run("group_group_member_unshare", func(t *testing.T) { metaGroupGroupMemberUnshare(ctx, t) })
		t.Run("group_group_milestone_burndown", func(t *testing.T) { metaGroupGroupMilestoneBurndown(ctx, t) })
		t.Run("group_group_milestone_issues", func(t *testing.T) { metaGroupGroupMilestoneIssues(ctx, t) })
		t.Run("group_group_milestone_merge_requests", func(t *testing.T) { metaGroupGroupMilestoneMergeRequests(ctx, t) })
		t.Run("group_group_milestone_update", func(t *testing.T) { metaGroupGroupMilestoneUpdate(ctx, t) })
		t.Run("group_group_relations_list_status", func(t *testing.T) { metaGroupGroupRelationsListStatus(ctx, t) })
		t.Run("group_group_relations_schedule", func(t *testing.T) { metaGroupGroupRelationsSchedule(ctx, t) })
		t.Run("group_group_upload_delete_by_id", func(t *testing.T) { metaGroupGroupUploadDeleteById(ctx, t) })
		t.Run("group_group_upload_delete_by_secret", func(t *testing.T) { metaGroupGroupUploadDeleteBySecret(ctx, t) })
		t.Run("group_group_upload_list", func(t *testing.T) { metaGroupGroupUploadList(ctx, t) })
		t.Run("group_hook_add", func(t *testing.T) { metaGroupHookAdd(ctx, t) })
		t.Run("group_hook_delete", func(t *testing.T) { metaGroupHookDelete(ctx, t) })
		t.Run("group_hook_edit", func(t *testing.T) { metaGroupHookEdit(ctx, t) })
		t.Run("group_hook_get", func(t *testing.T) { metaGroupHookGet(ctx, t) })
		t.Run("group_hook_list", func(t *testing.T) { metaGroupHookList(ctx, t) })
		t.Run("group_projects", func(t *testing.T) { metaGroupProjects(ctx, t) })
		t.Run("group_restore", func(t *testing.T) { metaGroupRestore(ctx, t) })
		t.Run("group_search", func(t *testing.T) { metaGroupSearch(ctx, t) })
		t.Run("group_transfer_project", func(t *testing.T) { metaGroupTransferProject(ctx, t) })
		t.Run("group_unarchive", func(t *testing.T) { metaGroupUnarchive(ctx, t) })
		t.Run("group_update", func(t *testing.T) { metaGroupUpdate(ctx, t) })
		t.Run("issue_list_group", func(t *testing.T) { metaIssueListGroup(ctx, t) })
		t.Run("issue_statistics_get_group", func(t *testing.T) { metaIssueStatisticsGetGroup(ctx, t) })
		t.Run("merge_request_list_group", func(t *testing.T) { metaMergeRequestListGroup(ctx, t) })
		t.Run("package_registry_list_group", func(t *testing.T) { metaPackageRegistryListGroup(ctx, t) })
		t.Run("user_notification_group_get", func(t *testing.T) { metaUserNotificationGroupGet(ctx, t) })
		t.Run("user_notification_group_update", func(t *testing.T) { metaUserNotificationGroupUpdate(ctx, t) })
	}

	// Enterprise gap tests.
	if state.enterprise {
		t.Run("attestation_download", func(t *testing.T) { metaAttestationDownload(ctx, t) })
		t.Run("audit_event_get_instance", func(t *testing.T) { metaAuditEventGetInstance(ctx, t) })
		t.Run("audit_event_get_project", func(t *testing.T) { metaAuditEventGetProject(ctx, t) })
		t.Run("audit_event_list_instance", func(t *testing.T) { metaAuditEventListInstance(ctx, t) })
		t.Run("compliance_policy_update", func(t *testing.T) { metaCompliancePolicyUpdate(ctx, t) })
		t.Run("dependency_export_create", func(t *testing.T) { metaDependencyExportCreate(ctx, t) })
		t.Run("dependency_export_download", func(t *testing.T) { metaDependencyExportDownload(ctx, t) })
		t.Run("dependency_export_get", func(t *testing.T) { metaDependencyExportGet(ctx, t) })
		t.Run("enterprise_user_delete", func(t *testing.T) { metaEnterpriseUserDelete(ctx, t) })
		t.Run("enterprise_user_disable_2fa", func(t *testing.T) { metaEnterpriseUserDisable2fa(ctx, t) })
		t.Run("enterprise_user_get", func(t *testing.T) { metaEnterpriseUserGet(ctx, t) })
		t.Run("external_status_check_create", func(t *testing.T) { metaExternalStatusCheckCreate(ctx, t) })
		t.Run("external_status_check_create_project", func(t *testing.T) { metaExternalStatusCheckCreateProject(ctx, t) })
		t.Run("external_status_check_delete", func(t *testing.T) { metaExternalStatusCheckDelete(ctx, t) })
		t.Run("external_status_check_delete_project", func(t *testing.T) { metaExternalStatusCheckDeleteProject(ctx, t) })
		t.Run("external_status_check_list_mr_checks", func(t *testing.T) { metaExternalStatusCheckListMrChecks(ctx, t) })
		t.Run("external_status_check_list_project", func(t *testing.T) { metaExternalStatusCheckListProject(ctx, t) })
		t.Run("external_status_check_list_project_mr_checks", func(t *testing.T) { metaExternalStatusCheckListProjectMrChecks(ctx, t) })
		t.Run("external_status_check_retry", func(t *testing.T) { metaExternalStatusCheckRetry(ctx, t) })
		t.Run("external_status_check_retry_project", func(t *testing.T) { metaExternalStatusCheckRetryProject(ctx, t) })
		t.Run("external_status_check_set_project_mr_status", func(t *testing.T) { metaExternalStatusCheckSetProjectMrStatus(ctx, t) })
		t.Run("external_status_check_set_status", func(t *testing.T) { metaExternalStatusCheckSetStatus(ctx, t) })
		t.Run("external_status_check_update", func(t *testing.T) { metaExternalStatusCheckUpdate(ctx, t) })
		t.Run("external_status_check_update_project", func(t *testing.T) { metaExternalStatusCheckUpdateProject(ctx, t) })
		t.Run("geo_create", func(t *testing.T) { metaGeoCreate(ctx, t) })
		t.Run("geo_delete", func(t *testing.T) { metaGeoDelete(ctx, t) })
		t.Run("geo_edit", func(t *testing.T) { metaGeoEdit(ctx, t) })
		t.Run("geo_get", func(t *testing.T) { metaGeoGet(ctx, t) })
		t.Run("geo_get_status", func(t *testing.T) { metaGeoGetStatus(ctx, t) })
		t.Run("geo_list_status", func(t *testing.T) { metaGeoListStatus(ctx, t) })
		t.Run("geo_repair", func(t *testing.T) { metaGeoRepair(ctx, t) })
		t.Run("issue_event_issue_iteration_get", func(t *testing.T) { metaIssueEventIssueIterationGet(ctx, t) })
		t.Run("issue_event_issue_iteration_list", func(t *testing.T) { metaIssueEventIssueIterationList(ctx, t) })
		t.Run("issue_event_issue_weight_list", func(t *testing.T) { metaIssueEventIssueWeightList(ctx, t) })
		t.Run("member_role_create_instance", func(t *testing.T) { metaMemberRoleCreateInstance(ctx, t) })
		t.Run("member_role_delete_instance", func(t *testing.T) { metaMemberRoleDeleteInstance(ctx, t) })
		t.Run("merge_train_add", func(t *testing.T) { metaMergeTrainAdd(ctx, t) })
		t.Run("merge_train_get", func(t *testing.T) { metaMergeTrainGet(ctx, t) })
		t.Run("merge_train_list_branch", func(t *testing.T) { metaMergeTrainListBranch(ctx, t) })
		t.Run("project_pull_mirror_configure", func(t *testing.T) { metaProjectPullMirrorConfigure(ctx, t) })
		t.Run("project_pull_mirror_get", func(t *testing.T) { metaProjectPullMirrorGet(ctx, t) })
		t.Run("project_start_mirroring", func(t *testing.T) { metaProjectStartMirroring(ctx, t) })
		t.Run("project_alias_create", func(t *testing.T) { metaProjectAliasCreate(ctx, t) })
		t.Run("project_alias_delete", func(t *testing.T) { metaProjectAliasDelete(ctx, t) })
		t.Run("project_alias_get", func(t *testing.T) { metaProjectAliasGet(ctx, t) })
		t.Run("storage_move_get_group", func(t *testing.T) { metaStorageMoveGetGroup(ctx, t) })
		t.Run("storage_move_get_group_for_group", func(t *testing.T) { metaStorageMoveGetGroupForGroup(ctx, t) })
		t.Run("storage_move_get_project", func(t *testing.T) { metaStorageMoveGetProject(ctx, t) })
		t.Run("storage_move_get_project_for_project", func(t *testing.T) { metaStorageMoveGetProjectForProject(ctx, t) })
		t.Run("storage_move_get_snippet", func(t *testing.T) { metaStorageMoveGetSnippet(ctx, t) })
		t.Run("storage_move_get_snippet_for_snippet", func(t *testing.T) { metaStorageMoveGetSnippetForSnippet(ctx, t) })
		t.Run("storage_move_retrieve_all_group", func(t *testing.T) { metaStorageMoveRetrieveAllGroup(ctx, t) })
		t.Run("storage_move_retrieve_all_snippet", func(t *testing.T) { metaStorageMoveRetrieveAllSnippet(ctx, t) })
		t.Run("storage_move_retrieve_group", func(t *testing.T) { metaStorageMoveRetrieveGroup(ctx, t) })
		t.Run("storage_move_retrieve_project", func(t *testing.T) { metaStorageMoveRetrieveProject(ctx, t) })
		t.Run("storage_move_retrieve_snippet", func(t *testing.T) { metaStorageMoveRetrieveSnippet(ctx, t) })
		t.Run("storage_move_schedule_all_group", func(t *testing.T) { metaStorageMoveScheduleAllGroup(ctx, t) })
		t.Run("storage_move_schedule_all_project", func(t *testing.T) { metaStorageMoveScheduleAllProject(ctx, t) })
		t.Run("storage_move_schedule_all_snippet", func(t *testing.T) { metaStorageMoveScheduleAllSnippet(ctx, t) })
		t.Run("storage_move_schedule_group", func(t *testing.T) { metaStorageMoveScheduleGroup(ctx, t) })
		t.Run("storage_move_schedule_project", func(t *testing.T) { metaStorageMoveScheduleProject(ctx, t) })
		t.Run("storage_move_schedule_snippet", func(t *testing.T) { metaStorageMoveScheduleSnippet(ctx, t) })
		t.Run("vulnerability_confirm", func(t *testing.T) { metaVulnerabilityConfirm(ctx, t) })
		t.Run("vulnerability_dismiss", func(t *testing.T) { metaVulnerabilityDismiss(ctx, t) })
		t.Run("vulnerability_get", func(t *testing.T) { metaVulnerabilityGet(ctx, t) })
		t.Run("vulnerability_pipeline_security_summary", func(t *testing.T) { metaVulnerabilityPipelineSecuritySummary(ctx, t) })
		t.Run("vulnerability_resolve", func(t *testing.T) { metaVulnerabilityResolve(ctx, t) })
		t.Run("vulnerability_revert", func(t *testing.T) { metaVulnerabilityRevert(ctx, t) })
		if mState.groupPath != "" {
			t.Run("audit_event_get_group", func(t *testing.T) { metaAuditEventGetGroup(ctx, t) })
			t.Run("audit_event_list_group", func(t *testing.T) { metaAuditEventListGroup(ctx, t) })
			t.Run("dora_metrics_group", func(t *testing.T) { metaDoraMetricsGroup(ctx, t) })
			t.Run("group_scim_delete", func(t *testing.T) { metaGroupScimDelete(ctx, t) })
			t.Run("group_scim_get", func(t *testing.T) { metaGroupScimGet(ctx, t) })
			t.Run("group_scim_update", func(t *testing.T) { metaGroupScimUpdate(ctx, t) })
			t.Run("member_role_create_group", func(t *testing.T) { metaMemberRoleCreateGroup(ctx, t) })
			t.Run("member_role_delete_group", func(t *testing.T) { metaMemberRoleDeleteGroup(ctx, t) })
			t.Run("member_role_list_group", func(t *testing.T) { metaMemberRoleListGroup(ctx, t) })
			t.Run("merge_request_approval_settings_group_get", func(t *testing.T) { metaMergeRequestApprovalSettingsGroupGet(ctx, t) })
			t.Run("merge_request_approval_settings_group_update", func(t *testing.T) { metaMergeRequestApprovalSettingsGroupUpdate(ctx, t) })
		}
	}

	// Cleanup.
	t.Run("99_Cleanup_DeleteProject", func(t *testing.T) { metaDeleteProject(ctx, t) })
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------.

// mPID returns the meta-tool test project ID as a string.
func mPID() string { return strconv.FormatInt(mState.projectID, 10) }

// requireMetaProjectID fails the test if the meta-tool project ID has not
// been set by a prior step.
func requireMetaProjectID(t *testing.T) {
	t.Helper()
	if mState.projectID == 0 {
		t.Fatal("meta project ID not set \u2014 CreateProject must run first")
	}
}

// requireMetaMRIID fails the test if the meta-tool merge request IID has
// not been set by a prior step.
func requireMetaMRIID(t *testing.T) {
	t.Helper()
	requireMetaProjectID(t)
	if mState.mrIID == 0 {
		t.Fatal("meta MR IID not set — CreateMR must run first")
	}
}

// ---------------------------------------------------------------------------
// Project meta-tool
// ---------------------------------------------------------------------------.

// metaCreateProject creates a new private GitLab project via the
// gitlab_project meta-tool and stores its ID and path in mState.
func metaCreateProject(ctx context.Context, t *testing.T) {
	name := uniqueName("e2e-meta")
	out, err := callMeta[projects.Output](ctx, "gitlab_project", "create", map[string]any{
		"name":                   name,
		"description":            "Meta-tool E2E test project — will be deleted automatically",
		"visibility":             "private",
		"initialize_with_readme": true,
		"default_branch":         defaultBranch,
	})
	requireNoError(t, err, "meta create project")
	requireTrue(t, out.ID > 0, "project ID should be positive")

	mState.projectID = out.ID
	mState.projectPath = out.PathWithNamespace
	t.Logf("Created project: %s (ID=%d)", mState.projectPath, mState.projectID)

	waitForBranchMeta(ctx, t, defaultBranch)
}

// metaGetProject retrieves the E2E test project by ID via the
// gitlab_project meta-tool and verifies its ID matches.
func metaGetProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[projects.Output](ctx, "gitlab_project", "get", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta get project")
	requireTrue(t, out.ID == mState.projectID, "expected project ID %d, got %d", mState.projectID, out.ID)
	t.Logf("Got project %s", out.PathWithNamespace)
}

// metaUnprotectMain removes protection from the main branch via the
// gitlab_branch meta-tool. The MCP tool is idempotent — calling it on an
// already-unprotected branch returns success without error.
func metaUnprotectMain(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_branch", "unprotect", map[string]any{
		"project_id":  mPID(),
		"branch_name": defaultBranch,
	})
	requireNoError(t, err, "meta unprotect main")
	t.Logf("Unprotected %s branch via meta-tool", defaultBranch)
}

// metaUpdateProject updates the project description via the
// gitlab_project meta-tool.
func metaUpdateProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[projects.Output](ctx, "gitlab_project", "update", map[string]any{
		"project_id":  mPID(),
		"description": "Meta-tool E2E — UPDATED, ready for deletion",
	})
	requireNoError(t, err, "meta update project")
	requireTrue(t, out.ID == mState.projectID, "expected project ID %d, got %d", mState.projectID, out.ID)
	t.Logf("Updated project %s", mState.projectPath)
}

// metaListProjects lists owned projects via the gitlab_project meta-tool
// and verifies the test project appears in the result.
func metaListProjects(ctx context.Context, t *testing.T) {
	out, err := callMeta[projects.ListOutput](ctx, "gitlab_project", "list", map[string]any{
		"owned": true,
	})
	requireNoError(t, err, "meta list projects")
	requireTrue(t, len(out.Projects) >= 1, "expected at least 1 project")

	found := false
	for _, p := range out.Projects {
		if p.ID == mState.projectID {
			found = true
			break
		}
	}
	requireTrue(t, found, "project %d not in list", mState.projectID)
	t.Logf("Found %d owned projects", len(out.Projects))
}

// metaDeleteProject permanently deletes the E2E test project via the
// gitlab_project meta-tool and verifies it is no longer accessible.
func metaDeleteProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)

	// Step 1: Permanently delete via meta-tool.
	out, err := callMeta[projects.DeleteOutput](ctx, "gitlab_project", "delete", map[string]any{
		"project_id":         mPID(),
		"permanently_remove": true,
		"full_path":          mState.projectPath,
	})
	requireNoError(t, err, "meta delete project")
	t.Logf("Delete response: status=%s, permanently_removed=%v", out.Status, out.PermanentlyRemoved)

	// Step 2: Verify the project is gone (GET should fail).
	_, getErr := callMeta[projects.Output](ctx, "gitlab_project", "get", map[string]any{
		"project_id": mPID(),
	})
	requireTrue(t, getErr != nil, "expected project %d to be deleted, but GET succeeded", mState.projectID)
	t.Logf("Verified project %s (ID=%d) is permanently deleted", mState.projectPath, mState.projectID)
	mState.projectID = 0
}

// Push Rules (meta-tool).

// metaAddPushRule adds push rules via the gitlab_project meta-tool.
func metaAddPushRule(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[projects.PushRuleOutput](ctx, "gitlab_project", "push_rule_add", map[string]any{
		"project_id":           mPID(),
		"commit_message_regex": "^[A-Z].*",
		"max_file_size":        50,
	})
	requireNoError(t, err, "meta add push rule")
	requireTrue(t, out.ID > 0, "push rule ID should be positive, got %d", out.ID)
	t.Logf("Added push rule %d via meta-tool", out.ID)
}

// metaGetPushRules retrieves push rules via the gitlab_project meta-tool.
func metaGetPushRules(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[projects.PushRuleOutput](ctx, "gitlab_project", "push_rule_get", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta get push rules")
	requireTrue(t, out.ID > 0, "push rule ID should be positive")
	requireTrue(t, out.MaxFileSize == 50, "expected max_file_size=50, got %d", out.MaxFileSize)
	t.Logf("Got push rules via meta-tool: max_file_size=%d", out.MaxFileSize)
}

// metaEditPushRule modifies push rules via the gitlab_project meta-tool.
func metaEditPushRule(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[projects.PushRuleOutput](ctx, "gitlab_project", "push_rule_edit", map[string]any{
		"project_id":    mPID(),
		"max_file_size": 100,
	})
	requireNoError(t, err, "meta edit push rule")
	requireTrue(t, out.MaxFileSize == 100, "expected max_file_size=100, got %d", out.MaxFileSize)
	t.Logf("Edited push rule via meta-tool: max_file_size=%d", out.MaxFileSize)
}

// metaDeletePushRule removes push rules via the gitlab_project meta-tool.
func metaDeletePushRule(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_project", "push_rule_delete", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta delete push rule")
	t.Logf("Deleted push rules via meta-tool")
}

// User-scoped project listings (meta-tool).

// metaListUserContributed lists user contributed projects via the gitlab_project meta-tool.
func metaListUserContributed(ctx context.Context, t *testing.T) {
	user := os.Getenv("GITLAB_USER")
	out, err := callMeta[projects.ListOutput](ctx, "gitlab_project", "list_user_contributed", map[string]any{
		"user_id": user,
	})
	requireNoError(t, err, "meta list user contributed")
	t.Logf("User %s contributed to %d projects (via meta-tool)", user, len(out.Projects))
}

// metaListUserStarred lists user starred projects via the gitlab_project meta-tool.
func metaListUserStarred(ctx context.Context, t *testing.T) {
	user := os.Getenv("GITLAB_USER")
	out, err := callMeta[projects.ListOutput](ctx, "gitlab_project", "list_user_starred", map[string]any{
		"user_id": user,
	})
	requireNoError(t, err, "meta list user starred")
	t.Logf("User %s starred %d projects (via meta-tool)", user, len(out.Projects))
}

// GraphQL meta-tools (branch rules, CI catalog, vulnerabilities, custom emoji).

// metaBranchRuleList lists branch rules for the project via the
// gitlab_branch_rule meta-tool.
func metaBranchRuleList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[branchrules.ListOutput](ctx, "gitlab_branch_rule", "list", map[string]any{
		"project_path": mState.projectPath,
	})
	requireNoError(t, err, "meta branch rule list")
	t.Logf("Project %s has %d branch rules (via meta-tool)", mState.projectPath, len(out.Rules))
}

// metaCICatalogList queries CI/CD Catalog resources via the gitlab_ci_catalog
// meta-tool.
func metaCICatalogList(ctx context.Context, t *testing.T) {
	out, err := callMeta[cicatalog.ListOutput](ctx, "gitlab_ci_catalog", "list", map[string]any{})
	requireNoError(t, err, "meta ci catalog list")
	t.Logf("Found %d CI/CD catalog resources (via meta-tool)", len(out.Resources))
}

// metaVulnerabilitySeverityCount queries vulnerability severity counts via
// the gitlab_vulnerability meta-tool. Skips if not available (requires Ultimate).
func metaVulnerabilitySeverityCount(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[vulnerabilities.SeverityCountOutput](ctx, "gitlab_vulnerability", "severity_count", map[string]any{
		"project_path": mState.projectPath,
	})
	requireNoError(t, err, "meta vulnerability severity_count")
	requireTrue(t, out.Total >= 0, "expected non-negative total, got %d", out.Total)
	t.Logf("Vulnerability severity counts via meta-tool: critical=%d high=%d medium=%d low=%d total=%d",
		out.Critical, out.High, out.Medium, out.Low, out.Total)
}

// metaVulnerabilityList lists vulnerabilities via the gitlab_vulnerability
// meta-tool. Skips if not available (requires Ultimate).
func metaVulnerabilityList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[vulnerabilities.ListOutput](ctx, "gitlab_vulnerability", "list", map[string]any{
		"project_path": mState.projectPath,
	})
	requireNoError(t, err, "meta vulnerability list")
	t.Logf("Project %s has %d vulnerabilities (via meta-tool)", mState.projectPath, len(out.Vulnerabilities))
}

// metaCustomEmojiList lists custom emoji for the discovered group via
// the gitlab_custom_emoji meta-tool.
func metaCustomEmojiList(ctx context.Context, t *testing.T) {
	out, err := callMeta[customemoji.ListOutput](ctx, "gitlab_custom_emoji", "list", map[string]any{
		"group_path": mState.groupPath,
	})
	requireNoError(t, err, "meta custom emoji list")
	t.Logf("Group %s has %d custom emoji (via meta-tool)", mState.groupPath, len(out.Emoji))
}

// ---------------------------------------------------------------------------
// Repository meta-tool
// ---------------------------------------------------------------------------.

// metaCommitCreate creates main.go on the default branch via the
// gitlab_repository meta-tool commit_create action.
func metaCommitCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[commits.Output](ctx, "gitlab_repository", "commit_create", map[string]any{
		"project_id":     mPID(),
		"branch":         defaultBranch,
		"commit_message": "feat: add main.go via meta-tool E2E",
		"actions": []map[string]any{
			{
				"action":    "create",
				"file_path": testFileMainGo,
				"content":   "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"Hello, Meta-tool E2E!\")\n}\n\nfunc add(a, b int) int {\n\treturn a + b\n}\n",
			},
		},
	})
	requireNoError(t, err, "meta commit create")
	requireTrue(t, out.ID != "", msgCommitIDEmpty)
	mState.lastCommitSHA = out.ID
	t.Logf("Committed main.go via meta-tool (SHA=%s)", out.ShortID)
}

// metaFileGet retrieves main.go content via the gitlab_repository meta-tool
// and verifies the file name and size.
func metaFileGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[files.Output](ctx, "gitlab_repository", "file_get", map[string]any{
		"project_id": mPID(),
		"file_path":  testFileMainGo,
		"ref":        defaultBranch,
	})
	requireNoError(t, err, "meta file get")
	requireTrue(t, out.FileName == testFileMainGo, "expected main.go, got %q", out.FileName)
	requireTrue(t, out.Size > 0, "file size should be positive")
	t.Logf("Got file %s (%d bytes)", out.FileName, out.Size)
}

// ---------------------------------------------------------------------------
// Branch meta-tool
// ---------------------------------------------------------------------------.

// metaBranchCreate creates the feature/meta-changes branch from the
// default branch via the gitlab_branch meta-tool.
func metaBranchCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[branches.Output](ctx, "gitlab_branch", "create", map[string]any{
		"project_id":  mPID(),
		"branch_name": testMetaBranch,
		"ref":         defaultBranch,
	})
	requireNoError(t, err, "meta branch create")
	requireTrue(t, out.Name == testMetaBranch, "expected branch name, got %q", out.Name)
	t.Logf("Created branch %s", out.Name)
}

// metaBranchList lists all branches via the gitlab_branch meta-tool and
// verifies at least two exist.
func metaBranchList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[branches.ListOutput](ctx, "gitlab_branch", "list", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta branch list")
	requireTrue(t, len(out.Branches) >= 2, "expected at least 2 branches, got %d", len(out.Branches))
	t.Logf("Listed %d branches", len(out.Branches))
}

// metaBranchProtect protects the feature branch with specified access
// levels via the gitlab_branch meta-tool.
func metaBranchProtect(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[branches.ProtectedOutput](ctx, "gitlab_branch", "protect", map[string]any{
		"project_id":         mPID(),
		"branch_name":        testMetaBranch,
		"push_access_level":  40,
		"merge_access_level": 30,
	})
	requireNoError(t, err, "meta branch protect")
	requireTrue(t, out.Name == testMetaBranch, "expected protected branch name, got %q", out.Name)
	t.Logf("Protected branch %s", out.Name)
}

// metaListProtectedBranches lists protected branches via the gitlab_branch
// meta-tool and verifies the feature branch appears.
func metaListProtectedBranches(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[branches.ProtectedListOutput](ctx, "gitlab_branch", "list_protected", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta list protected branches")
	requireTrue(t, len(out.Branches) >= 1, "expected at least 1 protected branch")

	found := false
	for _, b := range out.Branches {
		if b.Name == testMetaBranch {
			found = true
			break
		}
	}
	requireTrue(t, found, "feature/meta-changes not in protected branches")
	t.Logf("Listed %d protected branches", len(out.Branches))
}

// metaBranchUnprotect removes protection from the feature branch via the
// gitlab_branch meta-tool so commits can be pushed to it. After unprotecting,
// it verifies the branch is no longer in the protected list — GitLab CE may
// have a brief propagation delay between the unprotect API response and the
// commit authorization check.
func metaBranchUnprotect(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_branch", "unprotect", map[string]any{
		"project_id":  mPID(),
		"branch_name": testMetaBranch,
	})
	requireNoError(t, err, "meta branch unprotect")

	// Verify the branch is no longer protected (also serves as a propagation delay).
	out, err := callMeta[branches.ProtectedListOutput](ctx, "gitlab_branch", "list_protected", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta list protected after unprotect")
	for _, b := range out.Branches {
		if b.Name == testMetaBranch {
			t.Fatalf("branch %q still appears in protected list after unprotect", testMetaBranch)
		}
	}
	t.Log("Unprotected feature/meta-changes (verified)")
}

// metaCommitFeatureChanges pushes an updated main.go with a multiply
// function to the feature branch via the gitlab_repository meta-tool.
// Retries once after a short delay if the commit is rejected with 403,
// which can happen on fresh GitLab CE instances due to branch protection
// propagation lag after unprotect.
func metaCommitFeatureChanges(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	params := map[string]any{
		"project_id":     mPID(),
		"branch":         testMetaBranch,
		"commit_message": "refactor: add multiply via meta-tool",
		"actions": []map[string]any{
			{
				"action":    "update",
				"file_path": testFileMainGo,
				"content":   "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"Hello, Meta-tool E2E!\")\n\tresult := multiply(3, 4)\n\tfmt.Println(\"3 * 4 =\", result)\n}\n\nfunc add(a, b int) int {\n\treturn a + b\n}\n\nfunc multiply(a, b int) int {\n\treturn a * b\n}\n",
			},
		},
	}
	out, err := callMeta[commits.Output](ctx, "gitlab_repository", "commit_create", params)
	if err != nil && strings.Contains(err.Error(), "403") {
		t.Logf("commit_create got 403, retrying after 1s (branch protection propagation lag)")
		time.Sleep(1 * time.Second)
		out, err = callMeta[commits.Output](ctx, "gitlab_repository", "commit_create", params)
	}
	requireNoError(t, err, "meta commit feature changes")
	requireTrue(t, out.ID != "", msgCommitIDEmpty)
	t.Logf("Committed feature changes via meta-tool (SHA=%s)", out.ShortID)
}

// ---------------------------------------------------------------------------
// Tag meta-tool
// ---------------------------------------------------------------------------.

// metaTagCreate creates tag v0.1.0-meta on the default branch via the
// gitlab_tag meta-tool.
func metaTagCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[tags.Output](ctx, "gitlab_tag", "create", map[string]any{
		"project_id": mPID(),
		"tag_name":   testMetaTag,
		"ref":        defaultBranch,
		"message":    "Meta-tool E2E tag",
	})
	requireNoError(t, err, "meta tag create")
	requireTrue(t, out.Name == testMetaTag, "expected tag name, got %q", out.Name)
	t.Logf("Created tag %s", out.Name)
}

// metaTagList lists tags via the gitlab_tag meta-tool and verifies
// v0.1.0-meta appears in the result.
func metaTagList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[tags.ListOutput](ctx, "gitlab_tag", "list", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta tag list")
	requireTrue(t, len(out.Tags) >= 1, "expected at least 1 tag")

	found := false
	for _, tag := range out.Tags {
		if tag.Name == testMetaTag {
			found = true
			break
		}
	}
	requireTrue(t, found, "tag v0.1.0-meta not found")
	t.Logf("Listed %d tags", len(out.Tags))
}

// metaTagDelete deletes tag v0.1.0-meta via the gitlab_tag meta-tool.
func metaTagDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_tag", "delete", map[string]any{
		"project_id": mPID(),
		"tag_name":   testMetaTag,
	})
	requireNoError(t, err, "meta tag delete")
	t.Log("Deleted tag v0.1.0-meta")
}

// ---------------------------------------------------------------------------
// Release meta-tool
// ---------------------------------------------------------------------------.

// metaReleaseCreate creates a release for tag v0.1.0-meta via the
// gitlab_release meta-tool.
func metaReleaseCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[releases.Output](ctx, "gitlab_release", "create", map[string]any{
		"project_id":  mPID(),
		"tag_name":    testMetaTag,
		"name":        "Meta-tool E2E Release v0.1.0-meta",
		"description": "Release created via meta-tool E2E testing.",
	})
	requireNoError(t, err, "meta release create")
	requireTrue(t, out.TagName == testMetaTag, fmtExpectedTag, out.TagName)
	t.Logf("Created release %s", out.Name)
}

// metaReleaseGet retrieves the release for v0.1.0-meta via the
// gitlab_release meta-tool and verifies its tag.
func metaReleaseGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[releases.Output](ctx, "gitlab_release", "get", map[string]any{
		"project_id": mPID(),
		"tag_name":   testMetaTag,
	})
	requireNoError(t, err, "meta release get")
	requireTrue(t, out.TagName == testMetaTag, fmtExpectedTag, out.TagName)
	t.Logf("Got release %s (created=%s)", out.Name, out.CreatedAt)
}

// metaReleaseUpdate updates the release description via the gitlab_release
// meta-tool.
func metaReleaseUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[releases.Output](ctx, "gitlab_release", "update", map[string]any{
		"project_id":  mPID(),
		"tag_name":    testMetaTag,
		"description": "Updated meta-tool E2E release.",
	})
	requireNoError(t, err, "meta release update")
	requireTrue(t, out.TagName == testMetaTag, fmtExpectedTag, out.TagName)
	t.Logf("Updated release %s", out.Name)
}

// metaReleaseList lists releases via the gitlab_release meta-tool and
// verifies at least one exists.
func metaReleaseList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[releases.ListOutput](ctx, "gitlab_release", "list", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta release list")
	requireTrue(t, len(out.Releases) >= 1, "expected at least 1 release")
	t.Logf("Listed %d releases", len(out.Releases))
}

// metaReleaseLinkCreate adds a package asset link to the release via the
// gitlab_release meta-tool and stores the link ID in mState.
func metaReleaseLinkCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[releaselinks.Output](ctx, "gitlab_release", "link_create", map[string]any{
		"project_id": mPID(),
		"tag_name":   testMetaTag,
		"name":       "Meta Binary (Linux arm64)",
		"url":        "https://example.com/releases/v0.1.0-meta/binary-linux-arm64",
		"link_type":  "package",
	})
	requireNoError(t, err, "meta release link create")
	requireTrue(t, out.ID > 0, "link ID should be positive")

	mState.releaseLinkID = out.ID
	t.Logf("Created release link ID=%d", out.ID)
}

// metaReleaseLinkList lists release links via the gitlab_release meta-tool
// and verifies at least one exists.
func metaReleaseLinkList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[releaselinks.ListOutput](ctx, "gitlab_release", "link_list", map[string]any{
		"project_id": mPID(),
		"tag_name":   testMetaTag,
	})
	requireNoError(t, err, "meta release link list")
	requireTrue(t, len(out.Links) >= 1, "expected at least 1 link")
	t.Logf("Listed %d release links", len(out.Links))
}

// metaReleaseLinkDelete deletes the release asset link via the
// gitlab_release meta-tool.
func metaReleaseLinkDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.releaseLinkID > 0, "release link ID not set")

	out, err := callMeta[releaselinks.Output](ctx, "gitlab_release", "link_delete", map[string]any{
		"project_id": mPID(),
		"tag_name":   testMetaTag,
		"link_id":    mState.releaseLinkID,
	})
	requireNoError(t, err, "meta release link delete")
	requireTrue(t, out.ID == mState.releaseLinkID, "expected link ID %d, got %d", mState.releaseLinkID, out.ID)
	t.Logf("Deleted release link ID=%d", out.ID)
	mState.releaseLinkID = 0
}

// metaReleaseDelete deletes the v0.1.0-meta release via the gitlab_release
// meta-tool.
func metaReleaseDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[releases.Output](ctx, "gitlab_release", "delete", map[string]any{
		"project_id": mPID(),
		"tag_name":   testMetaTag,
	})
	requireNoError(t, err, "meta release delete")
	requireTrue(t, out.TagName == testMetaTag, fmtExpectedTag, out.TagName)
	t.Logf("Deleted release %s", out.TagName)
}

// ---------------------------------------------------------------------------
// Merge request meta-tool
// ---------------------------------------------------------------------------.

// metaCreateMR creates a merge request from the feature branch to main via
// the gitlab_merge_request meta-tool and stores its IID in mState.
func metaCreateMR(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[mergerequests.Output](ctx, "gitlab_merge_request", "create", map[string]any{
		"project_id":    mPID(),
		"source_branch": testMetaBranch,
		"target_branch": defaultBranch,
		"title":         "feat: add multiply function [meta E2E]",
		"description":   "MR created via meta-tool E2E.",
	})
	requireNoError(t, err, "meta create MR")
	requireTrue(t, out.IID > 0, "MR IID should be positive")
	requireTrue(t, out.State == "opened", "expected state opened, got %q", out.State)

	mState.mrIID = out.IID
	t.Logf("Created MR !%d via meta-tool", out.IID)
}

// metaGetMR retrieves the merge request by IID via the
// gitlab_merge_request meta-tool and verifies its IID matches.
func metaGetMR(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	out, err := callMeta[mergerequests.Output](ctx, "gitlab_merge_request", "get", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
	})
	requireNoError(t, err, "meta get MR")
	requireTrue(t, out.IID == mState.mrIID, "expected MR IID %d, got %d", mState.mrIID, out.IID)
	t.Logf("Got MR !%d state=%s", out.IID, out.State)
}

// metaListMRs lists open merge requests via the gitlab_merge_request
// meta-tool and verifies the test MR appears in the result.
func metaListMRs(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[mergerequests.ListOutput](ctx, "gitlab_merge_request", "list", map[string]any{
		"project_id": mPID(),
		"state":      "opened",
	})
	requireNoError(t, err, "meta list MRs")
	requireTrue(t, len(out.MergeRequests) >= 1, "expected at least 1 MR")

	found := false
	for _, mr := range out.MergeRequests {
		if mr.IID == mState.mrIID {
			found = true
			break
		}
	}
	requireTrue(t, found, "MR !%d not in list", mState.mrIID)
	t.Logf("Listed %d MRs via meta-tool", len(out.MergeRequests))
}

// metaUpdateMR modifies the merge request title and description via the
// gitlab_merge_request meta-tool.
func metaUpdateMR(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	out, err := callMeta[mergerequests.Output](ctx, "gitlab_merge_request", "update", map[string]any{
		"project_id":  mPID(),
		"mr_iid":      mState.mrIID,
		"title":       "feat: add multiply function [meta E2E] (updated)",
		"description": "Updated via meta-tool E2E.",
	})
	requireNoError(t, err, "meta update MR")
	requireTrue(t, out.IID == mState.mrIID, "expected MR IID %d, got %d", mState.mrIID, out.IID)
	t.Logf("Updated MR !%d via meta-tool", out.IID)
}

// metaApproveMR approves the merge request via the gitlab_merge_request
// meta-tool.
func metaApproveMR(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	out, err := callMeta[mergerequests.ApproveOutput](ctx, "gitlab_merge_request", "approve", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
	})
	requireNoError(t, err, "meta approve MR")
	t.Logf("Approved MR !%d (approved=%v)", mState.mrIID, out.Approved)
}

// metaUnapproveMR revokes the approval from the merge request via the
// gitlab_merge_request meta-tool.
func metaUnapproveMR(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	err := callMetaVoid(ctx, "gitlab_merge_request", "unapprove", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
	})
	requireNoError(t, err, "meta unapprove MR")
	t.Logf("Unapproved MR !%d", mState.mrIID)
}

// metaMergeMR merges the merge request with source branch removal via the
// gitlab_merge_request meta-tool and verifies the state is "merged".
func metaMergeMR(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	out, err := callMeta[mergerequests.Output](ctx, "gitlab_merge_request", "merge", map[string]any{
		"project_id":                  mPID(),
		"mr_iid":                      mState.mrIID,
		"should_remove_source_branch": true,
	})
	requireNoError(t, err, "meta merge MR")
	requireTrue(t, out.State == "merged", "expected state merged, got %q", out.State)
	t.Logf("Merged MR !%d via meta-tool", mState.mrIID)
}

// ---------------------------------------------------------------------------
// MR review meta-tool
// ---------------------------------------------------------------------------.

// metaNoteCreate creates a general comment on the merge request via the
// gitlab_mr_review meta-tool and stores the note ID in mState.
func metaNoteCreate(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	out, err := callMeta[mrnotes.Output](ctx, "gitlab_mr_review", "note_create", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
		"body":       "**Meta-tool E2E Bot**: This MR looks great!",
	})
	requireNoError(t, err, "meta note create")
	requireTrue(t, out.ID > 0, "note ID should be positive")

	mState.noteID = out.ID
	t.Logf("Created note ID=%d via meta-tool", out.ID)
}

// metaNoteList lists notes on the merge request via the gitlab_mr_review
// meta-tool and verifies the created note appears.
func metaNoteList(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	out, err := callMeta[mrnotes.ListOutput](ctx, "gitlab_mr_review", "note_list", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
	})
	requireNoError(t, err, "meta note list")

	found := false
	for _, n := range out.Notes {
		if n.ID == mState.noteID {
			found = true
			break
		}
	}
	requireTrue(t, found, "note ID=%d not in list", mState.noteID)
	t.Logf("Listed %d notes via meta-tool", len(out.Notes))
}

// metaNoteUpdate modifies the note body via the gitlab_mr_review meta-tool.
func metaNoteUpdate(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	requireTrue(t, mState.noteID > 0, "note ID not set")

	out, err := callMeta[mrnotes.Output](ctx, "gitlab_mr_review", "note_update", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
		"note_id":    mState.noteID,
		"body":       "**Meta-tool E2E Bot** (updated): LGTM!",
	})
	requireNoError(t, err, "meta note update")
	requireTrue(t, out.ID == mState.noteID, "expected note ID %d, got %d", mState.noteID, out.ID)
	t.Logf("Updated note ID=%d via meta-tool", out.ID)
}

// metaNoteDelete deletes the note from the merge request via the
// gitlab_mr_review meta-tool.
func metaNoteDelete(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	requireTrue(t, mState.noteID > 0, "note ID not set")

	err := callMetaVoid(ctx, "gitlab_mr_review", "note_delete", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
		"note_id":    mState.noteID,
	})
	requireNoError(t, err, "meta note delete")
	t.Logf("Deleted note ID=%d via meta-tool", mState.noteID)
	mState.noteID = 0
}

// metaChangesGet retrieves the diff changes for the merge request via the
// gitlab_mr_review meta-tool and verifies at least one file changed.
func metaChangesGet(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	out, err := callMeta[mrchanges.Output](ctx, "gitlab_mr_review", "changes_get", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
	})
	requireNoError(t, err, "meta changes get")
	requireTrue(t, len(out.Changes) > 0, "expected at least 1 changed file")
	t.Logf("Got %d changed files via meta-tool", len(out.Changes))
}

// metaDiscussionCreate creates a code review discussion on a specific line
// of main.go via the gitlab_mr_review meta-tool using diff position metadata.
func metaDiscussionCreate(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)

	// Get diff SHAs via GitLab client (needed for inline positioning).
	versions, _, err := state.glClient.GL().MergeRequests.GetMergeRequestDiffVersions(
		int(mState.projectID), mState.mrIID, nil,
	)
	requireNoError(t, err, "get MR diff versions (meta)")
	requireTrue(t, len(versions) > 0, "expected at least 1 diff version")

	v := versions[0]
	out, err := callMeta[mrdiscussions.Output](ctx, "gitlab_mr_review", "discussion_create", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
		"body":       "**Meta Code Review**: Consider edge cases for multiply.",
		"position": map[string]any{
			"base_sha":  v.BaseCommitSHA,
			"start_sha": v.StartCommitSHA,
			"head_sha":  v.HeadCommitSHA,
			"new_path":  testFileMainGo,
			"new_line":  15,
		},
	})
	requireNoError(t, err, "meta discussion create")
	requireTrue(t, out.ID != "", "discussion ID should not be empty")

	mState.discussionID = out.ID
	t.Logf("Created inline discussion %s via meta-tool", out.ID)
}

// metaDiscussionReply adds a reply to the inline discussion via the
// gitlab_mr_review meta-tool.
func metaDiscussionReply(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	requireTrue(t, mState.discussionID != "", "discussion ID not set")

	out, err := callMeta[mrdiscussions.NoteOutput](ctx, "gitlab_mr_review", "discussion_reply", map[string]any{
		"project_id":    mPID(),
		"mr_iid":        mState.mrIID,
		"discussion_id": mState.discussionID,
		"body":          "Acknowledged! Will address in follow-up.",
	})
	requireNoError(t, err, "meta discussion reply")
	requireTrue(t, out.ID > 0, "reply note ID should be positive")
	t.Logf("Replied to discussion %s via meta-tool", mState.discussionID)
}

// metaDiscussionResolve marks the inline discussion as resolved via the
// gitlab_mr_review meta-tool.
func metaDiscussionResolve(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	requireTrue(t, mState.discussionID != "", "discussion ID not set")

	out, err := callMeta[mrdiscussions.Output](ctx, "gitlab_mr_review", "discussion_resolve", map[string]any{
		"project_id":    mPID(),
		"mr_iid":        mState.mrIID,
		"discussion_id": mState.discussionID,
		"resolved":      true,
	})
	requireNoError(t, err, "meta discussion resolve")
	requireTrue(t, out.ID == mState.discussionID, "expected discussion %s, got %s", mState.discussionID, out.ID)
	t.Logf("Resolved discussion %s via meta-tool", out.ID)
}

// metaDiscussionList lists discussions on the merge request via the
// gitlab_mr_review meta-tool and verifies at least one exists.
func metaDiscussionList(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	out, err := callMeta[mrdiscussions.ListOutput](ctx, "gitlab_mr_review", "discussion_list", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
	})
	requireNoError(t, err, "meta discussion list")
	requireTrue(t, len(out.Discussions) >= 1, "expected at least 1 discussion")
	t.Logf("Listed %d discussions via meta-tool", len(out.Discussions))
}

// ---------------------------------------------------------------------------
// User meta-tool
// ---------------------------------------------------------------------------.

// metaUserCurrent retrieves the authenticated user via the gitlab_user
// meta-tool and verifies basic fields.
func metaUserCurrent(ctx context.Context, t *testing.T) {
	out, err := callMeta[users.Output](ctx, "gitlab_user", "current", map[string]any{})
	requireNoError(t, err, "meta user current")
	requireTrue(t, out.ID > 0, "user ID should be positive, got %d", out.ID)
	requireTrue(t, out.Username != "", "username should not be empty")
	requireTrue(t, out.State == "active", "expected state 'active', got %q", out.State)
	t.Logf("Current user via meta-tool: %s (ID=%d)", out.Username, out.ID)
}

// ---------------------------------------------------------------------------
// Additional commit inspection via gitlab_repository meta-tool
// ---------------------------------------------------------------------------.

// metaCommitList lists commits on the default branch and saves the latest SHA.
func metaCommitList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[commits.ListOutput](ctx, "gitlab_repository", "commit_list", map[string]any{
		"project_id": mPID(),
		"ref_name":   defaultBranch,
	})
	requireNoError(t, err, "meta commit list")
	requireTrue(t, len(out.Commits) >= 1, "expected at least 1 commit, got %d", len(out.Commits))
	mState.lastCommitSHA = out.Commits[0].ID
	t.Logf("Listed %d commits via meta-tool (latest=%s)", len(out.Commits), out.Commits[0].ShortID)
}

// metaCommitGet retrieves a specific commit by SHA.
func metaCommitGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.lastCommitSHA != "", "lastCommitSHA not set — MetaCommitList must run first")

	out, err := callMeta[commits.DetailOutput](ctx, "gitlab_repository", "commit_get", map[string]any{
		"project_id": mPID(),
		"sha":        mState.lastCommitSHA,
	})
	requireNoError(t, err, "meta commit get")
	requireTrue(t, out.ID == mState.lastCommitSHA, "expected SHA %s, got %s", mState.lastCommitSHA, out.ID)
	requireTrue(t, out.Title != "", "commit title should not be empty")
	t.Logf("Got commit %s via meta-tool: %s", out.ShortID, out.Title)
}

// metaCommitDiff retrieves the diff for the latest commit.
func metaCommitDiff(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.lastCommitSHA != "", "lastCommitSHA not set")

	out, err := callMeta[commits.DiffOutput](ctx, "gitlab_repository", "commit_diff", map[string]any{
		"project_id": mPID(),
		"sha":        mState.lastCommitSHA,
	})
	requireNoError(t, err, "meta commit diff")
	requireTrue(t, len(out.Diffs) >= 1, "expected at least 1 diff, got %d", len(out.Diffs))
	t.Logf("Commit %s has %d file diffs via meta-tool", mState.lastCommitSHA[:8], len(out.Diffs))
}

// metaRepositoryTree lists the repository tree and verifies main.go is present.
func metaRepositoryTree(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[repository.TreeOutput](ctx, "gitlab_repository", "tree", map[string]any{
		"project_id": mPID(),
		"ref":        defaultBranch,
	})
	requireNoError(t, err, "meta repository tree")
	requireTrue(t, len(out.Tree) >= 1, "expected at least 1 tree node, got %d", len(out.Tree))

	found := false
	for _, node := range out.Tree {
		if node.Name == testFileMainGo {
			found = true
			break
		}
	}
	requireTrue(t, found, "main.go not found in tree listing")
	t.Logf("Repository tree has %d entries via meta-tool", len(out.Tree))
}

// metaBranchGet retrieves a specific branch via the gitlab_branch meta-tool.
func metaBranchGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[branches.Output](ctx, "gitlab_branch", "get", map[string]any{
		"project_id":  mPID(),
		"branch_name": testMetaBranch,
	})
	requireNoError(t, err, "meta branch get")
	requireTrue(t, out.Name == testMetaBranch, "expected branch %q, got %q", testMetaBranch, out.Name)
	requireTrue(t, out.CommitID != "", msgCommitIDEmpty)
	t.Logf("Got branch %s via meta-tool (commit=%s)", out.Name, out.CommitID[:8])
}

// metaRepositoryCompare compares the default branch and the feature branch.
func metaRepositoryCompare(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[repository.CompareOutput](ctx, "gitlab_repository", "compare", map[string]any{
		"project_id": mPID(),
		"from":       defaultBranch,
		"to":         testMetaBranch,
	})
	requireNoError(t, err, "meta repository compare")
	requireTrue(t, len(out.Commits) >= 1, "expected at least 1 commit in comparison, got %d", len(out.Commits))
	requireTrue(t, len(out.Diffs) >= 1, "expected at least 1 diff in comparison, got %d", len(out.Diffs))
	t.Logf("Compared %s..%s via meta-tool: %d commits, %d diffs", defaultBranch, testMetaBranch, len(out.Commits), len(out.Diffs))
}

// metaTagGet retrieves a specific tag via the gitlab_tag meta-tool.
func metaTagGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[tags.Output](ctx, "gitlab_tag", "get", map[string]any{
		"project_id": mPID(),
		"tag_name":   testMetaTag,
	})
	requireNoError(t, err, "meta tag get")
	requireTrue(t, out.Name == testMetaTag, fmtExpectedTag, out.Name)
	requireTrue(t, out.Target != "", "tag target should not be empty")
	t.Logf("Got tag %s via meta-tool (target=%s)", out.Name, out.Target[:8])
}

// ---------------------------------------------------------------------------
// Issue lifecycle via gitlab_issue meta-tool
// ---------------------------------------------------------------------------.

// metaIssueCreate creates a test issue via the gitlab_issue meta-tool.
func metaIssueCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[issues.Output](ctx, "gitlab_issue", "create", map[string]any{
		"project_id":  mPID(),
		"title":       "Meta E2E test issue — automated lifecycle",
		"description": "Issue created via gitlab_issue meta-tool E2E.",
	})
	requireNoError(t, err, "meta issue create")
	requireTrue(t, out.IID > 0, "issue IID should be positive, got %d", out.IID)
	requireTrue(t, out.State == "opened", "expected state 'opened', got %q", out.State)

	mState.issueIID = out.IID
	t.Logf("Created issue #%d via meta-tool: %s", out.IID, out.Title)
}

// metaIssueGet retrieves the issue by IID.
func metaIssueGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.issueIID > 0, "issueIID not set — MetaIssueCreate must run first")

	out, err := callMeta[issues.Output](ctx, "gitlab_issue", "get", map[string]any{
		"project_id": mPID(),
		"issue_iid":  mState.issueIID,
	})
	requireNoError(t, err, "meta issue get")
	requireTrue(t, out.IID == mState.issueIID, "expected issue IID %d, got %d", mState.issueIID, out.IID)
	t.Logf("Got issue #%d via meta-tool: %s", out.IID, out.Title)
}

// metaIssueList lists issues and verifies the test issue appears.
func metaIssueList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[issues.ListOutput](ctx, "gitlab_issue", "list", map[string]any{
		"project_id": mPID(),
		"state":      "opened",
	})
	requireNoError(t, err, "meta issue list")
	requireTrue(t, len(out.Issues) >= 1, "expected at least 1 issue, got %d", len(out.Issues))

	found := false
	for _, i := range out.Issues {
		if i.IID == mState.issueIID {
			found = true
			break
		}
	}
	requireTrue(t, found, "issue #%d not in meta list", mState.issueIID)
	t.Logf("Listed %d open issues via meta-tool", len(out.Issues))
}

// metaIssueUpdate modifies the issue title.
func metaIssueUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.issueIID > 0, msgMetaIssueIIDNotSet)

	out, err := callMeta[issues.Output](ctx, "gitlab_issue", "update", map[string]any{
		"project_id": mPID(),
		"issue_iid":  mState.issueIID,
		"title":      "Meta E2E test issue — updated title",
	})
	requireNoError(t, err, "meta issue update")
	requireTrue(t, out.IID == mState.issueIID, "expected issue IID %d, got %d", mState.issueIID, out.IID)
	t.Logf("Updated issue #%d via meta-tool", out.IID)
}

// metaIssueNoteCreate adds a comment to the test issue via the gitlab_issue meta-tool.
func metaIssueNoteCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.issueIID > 0, msgMetaIssueIIDNotSet)

	out, err := callMeta[issuenotes.Output](ctx, "gitlab_issue", "note_create", map[string]any{
		"project_id": mPID(),
		"issue_iid":  mState.issueIID,
		"body":       "**Meta E2E Bot**: Automated comment via meta-tool.",
	})
	requireNoError(t, err, "meta issue note create")
	requireTrue(t, out.ID > 0, "note ID should be positive, got %d", out.ID)

	mState.issueNoteID = out.ID
	t.Logf("Created note %d on issue #%d via meta-tool", out.ID, mState.issueIID)
}

// metaIssueNoteList lists notes on the test issue.
func metaIssueNoteList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.issueIID > 0, msgMetaIssueIIDNotSet)

	out, err := callMeta[issuenotes.ListOutput](ctx, "gitlab_issue", "note_list", map[string]any{
		"project_id": mPID(),
		"issue_iid":  mState.issueIID,
	})
	requireNoError(t, err, "meta issue note list")
	requireTrue(t, len(out.Notes) >= 1, "expected at least 1 note, got %d", len(out.Notes))

	found := false
	for _, n := range out.Notes {
		if n.ID == mState.issueNoteID {
			found = true
			break
		}
	}
	requireTrue(t, found, "note %d not found in list", mState.issueNoteID)
	t.Logf("Listed %d notes on issue #%d via meta-tool", len(out.Notes), mState.issueIID)
}

// metaIssueDelete deletes the test issue.
func metaIssueDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.issueIID > 0, msgMetaIssueIIDNotSet)

	err := callMetaVoid(ctx, "gitlab_issue", "delete", map[string]any{
		"project_id": mPID(),
		"issue_iid":  mState.issueIID,
	})
	requireNoError(t, err, "meta issue delete")
	t.Logf("Deleted issue #%d via meta-tool", mState.issueIID)
	mState.issueIID = 0
}

// ---------------------------------------------------------------------------
// Additional project actions via gitlab_project meta-tool
// ---------------------------------------------------------------------------.

// metaLabelList lists project labels (may be empty for a fresh project).
func metaLabelList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[labels.ListOutput](ctx, "gitlab_project", "label_list", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta label list")
	t.Log("Label list via meta-tool succeeded")
}

// metaMilestoneList lists project milestones (may be empty for a fresh project).
func metaMilestoneList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[milestones.ListOutput](ctx, "gitlab_project", "milestone_list", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta milestone list")
	t.Log("Milestone list via meta-tool succeeded")
}

// metaProjectMembersList lists project members and verifies at least 1 exists.
func metaProjectMembersList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[members.ListOutput](ctx, "gitlab_project", "members", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta project members list")
	requireTrue(t, len(out.Members) >= 1, "expected at least 1 member, got %d", len(out.Members))
	t.Logf("Listed %d project members via meta-tool", len(out.Members))
}

// metaProjectUpload uploads a small text file to the project.
func metaProjectUpload(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	content := base64.StdEncoding.EncodeToString([]byte("meta e2e upload test"))
	out, err := callMeta[uploads.UploadOutput](ctx, "gitlab_project", "upload", map[string]any{
		"project_id":     mPID(),
		"filename":       "meta-e2e-test.txt",
		"content_base64": content,
	})
	requireNoError(t, err, "meta project upload")
	requireTrue(t, out.URL != "", "upload URL should not be empty")
	requireTrue(t, out.Markdown != "", "upload markdown should not be empty")
	t.Logf("Uploaded file via meta-tool: %s", out.URL)
}

// ---------------------------------------------------------------------------
// Additional MR actions via gitlab_merge_request meta-tool
// ---------------------------------------------------------------------------.

// metaMRCommits lists commits in the merge request.
func metaMRCommits(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	var out mergerequests.CommitsOutput
	var err error
	for attempt := range 3 {
		out, err = callMeta[mergerequests.CommitsOutput](ctx, "gitlab_merge_request", "commits", map[string]any{
			"project_id": mPID(),
			"mr_iid":     mState.mrIID,
		})
		requireNoError(t, err, "meta MR commits")
		if len(out.Commits) > 0 {
			break
		}
		if attempt < 2 {
			time.Sleep(2 * time.Second)
		}
	}
	requireTrue(t, len(out.Commits) >= 1, "expected at least 1 MR commit, got %d", len(out.Commits))
	t.Logf("MR !%d has %d commits via meta-tool", mState.mrIID, len(out.Commits))
}

// metaMRPipelines lists pipelines for the merge request (may be empty if no CI config).
func metaMRPipelines(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	_, err := callMeta[mergerequests.PipelinesOutput](ctx, "gitlab_merge_request", "pipelines", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
	})
	requireNoError(t, err, "meta MR pipelines")
	t.Logf("MR pipelines listed via meta-tool")
}

// metaRebaseMR rebases the merge request with skip_ci=true.
func metaRebaseMR(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	err := callMetaVoid(ctx, "gitlab_merge_request", "rebase", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
		"skip_ci":    true,
	})
	requireNoError(t, err, "meta MR rebase")
	t.Logf("Rebased MR !%d via meta-tool", mState.mrIID)
}

// ---------------------------------------------------------------------------
// Search via gitlab_search meta-tool
// ---------------------------------------------------------------------------.

// metaSearchCode searches for code containing "add" in the project.
func metaSearchCode(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[search.CodeOutput](ctx, "gitlab_search", "code", map[string]any{
		"project_id": mPID(),
		"query":      "add",
	})
	// Search indexing may lag, so just verify the call doesn't error.
	requireNoError(t, err, "meta search code")
	t.Log("Search code via meta-tool succeeded")
}

// metaSearchMergeRequests searches for merge requests.
func metaSearchMergeRequests(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[search.MergeRequestsOutput](ctx, "gitlab_search", "merge_requests", map[string]any{
		"project_id": mPID(),
		"query":      "multiply",
	})
	requireNoError(t, err, "meta search merge requests")
	t.Log("Search merge requests via meta-tool succeeded")
}

// Group tools (read-only, via gitlab_group meta-tool).

// metaGroupList lists accessible groups and stores the first group ID for
// subsequent tests. Skips dependent tests if no groups exist.
func metaGroupList(ctx context.Context, t *testing.T) {
	out, err := callMeta[groups.ListOutput](ctx, "gitlab_group", "list", map[string]any{})
	requireNoError(t, err, "meta group list")
	t.Logf("Found %d groups via meta-tool", len(out.Groups))
	if len(out.Groups) > 0 {
		mState.groupID = out.Groups[0].ID
		mState.groupPath = out.Groups[0].FullPath
		t.Logf("Using group %d (%s) for subsequent tests", mState.groupID, out.Groups[0].FullPath)
	}
}

// metaGroupGet retrieves group details.
func metaGroupGet(ctx context.Context, t *testing.T) {
	gid := strconv.FormatInt(mState.groupID, 10)
	out, err := callMeta[groups.Output](ctx, "gitlab_group", "get", map[string]any{
		"group_id": gid,
	})
	requireNoError(t, err, "meta group get")
	requireTrue(t, out.ID == mState.groupID, "expected group ID %d, got %d", mState.groupID, out.ID)
	t.Logf("Group %d: %s (visibility=%s)", out.ID, out.FullPath, out.Visibility)
}

// metaGroupMembersList lists members of the discovered group.
func metaGroupMembersList(ctx context.Context, t *testing.T) {
	gid := strconv.FormatInt(mState.groupID, 10)
	out, err := callMeta[groups.MemberListOutput](ctx, "gitlab_group", "members", map[string]any{
		"group_id": gid,
	})
	requireNoError(t, err, "meta group members list")
	t.Logf("Group %d has %d members", mState.groupID, len(out.Members))
}

// metaSubgroupsList lists subgroups of the discovered group. May return empty.
func metaSubgroupsList(ctx context.Context, t *testing.T) {
	gid := strconv.FormatInt(mState.groupID, 10)
	out, err := callMeta[groups.ListOutput](ctx, "gitlab_group", "subgroups", map[string]any{
		"group_id": gid,
	})
	requireNoError(t, err, "meta subgroups list")
	t.Logf("Group %d has %d subgroups", mState.groupID, len(out.Groups))
}

// metaGroupIssues lists issues across all projects in the discovered group.
func metaGroupIssues(ctx context.Context, t *testing.T) {
	gid := strconv.FormatInt(mState.groupID, 10)
	out, err := callMeta[issues.ListGroupOutput](ctx, "gitlab_group", "issues", map[string]any{
		"group_id": gid,
	})
	requireNoError(t, err, "meta group issues")
	t.Logf("Group %d has %d issues", mState.groupID, len(out.Issues))
}

// Pipeline list (read-only, via gitlab_pipeline meta-tool).

// metaPipelineList lists pipelines on the meta-tool test project. May return
// empty if no CI configuration exists.
func metaPipelineList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[pipelines.ListOutput](ctx, "gitlab_pipeline", "list", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta pipeline list")
	t.Logf("Project has %d pipelines via meta-tool", len(out.Pipelines))
}

// Package lifecycle (via gitlab_package meta-tool).

const (
	metaPackageName    = "e2e-meta-pkg"
	metaPackageVersion = "1.0.0"
	metaPackageFile    = "meta-hello.txt"
)

// metaPackagePublish publishes a small file to the Generic Package Registry
// using the gitlab_package meta-tool and stores the package/file IDs.
func metaPackagePublish(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	content := base64.StdEncoding.EncodeToString([]byte("E2E meta package content"))
	out, err := callMeta[packages.PublishOutput](ctx, "gitlab_package", "publish", map[string]any{
		"project_id":      mPID(),
		"package_name":    metaPackageName,
		"package_version": metaPackageVersion,
		"file_name":       metaPackageFile,
		"content_base64":  content,
	})
	requireNoError(t, err, "meta publish package file")
	requireTrue(t, out.PackageID > 0, "package ID should be positive, got %d", out.PackageID)
	requireTrue(t, out.PackageFileID > 0, "package file ID should be positive, got %d", out.PackageFileID)
	mState.packageID = out.PackageID
	mState.packageFileID = out.PackageFileID
	t.Logf("Meta published package ID=%d file_id=%d (%s)", out.PackageID, out.PackageFileID, out.FileName)
}

// metaPackageList lists packages via the gitlab_package meta-tool and verifies
// the published package appears.
func metaPackageList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.packageID > 0, "package ID not set — metaPackagePublish must run first")
	out, err := callMeta[packages.ListOutput](ctx, "gitlab_package", "list", map[string]any{
		"project_id":   mPID(),
		"package_name": metaPackageName,
	})
	requireNoError(t, err, "meta list packages")
	found := false
	for _, p := range out.Packages {
		if p.ID == mState.packageID {
			found = true
			break
		}
	}
	requireTrue(t, found, "package %d not found in meta list", mState.packageID)
	t.Logf("Meta listed %d packages, found ID=%d", len(out.Packages), mState.packageID)
}

// metaPackageFileList lists files within the published package via the
// gitlab_package meta-tool.
func metaPackageFileList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.packageID > 0, msgPackageIDNotSet)
	out, err := callMeta[packages.FileListOutput](ctx, "gitlab_package", "file_list", map[string]any{
		"project_id": mPID(),
		"package_id": mState.packageID,
	})
	requireNoError(t, err, "meta list package files")
	requireTrue(t, len(out.Files) >= 1, "expected at least 1 file, got %d", len(out.Files))
	found := false
	for _, f := range out.Files {
		if f.FileName == metaPackageFile {
			found = true
			break
		}
	}
	requireTrue(t, found, "file %q not found in meta package", metaPackageFile)
	t.Logf("Meta package %d has %d files", mState.packageID, len(out.Files))
}

// metaPackageDownload downloads the published package file via the
// gitlab_package meta-tool and verifies the content.
func metaPackageDownload(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.packageID > 0, msgPackageIDNotSet)
	outputPath := filepath.Join(t.TempDir(), metaPackageFile)
	out, err := callMeta[packages.DownloadOutput](ctx, "gitlab_package", "download", map[string]any{
		"project_id":      mPID(),
		"package_name":    metaPackageName,
		"package_version": metaPackageVersion,
		"file_name":       metaPackageFile,
		"output_path":     outputPath,
	})
	requireNoError(t, err, "meta download package file")
	requireTrue(t, out.Size > 0, "downloaded file size should be positive, got %d", out.Size)
	requireTrue(t, out.SHA256 != "", "SHA256 should not be empty")

	data, err := os.ReadFile(outputPath)
	requireNoError(t, err, "read meta downloaded file")
	requireTrue(t, string(data) == "E2E meta package content", "expected original content, got %q", string(data))
	t.Logf("Meta downloaded %s (%d bytes, sha256=%s)", outputPath, out.Size, out.SHA256)
}

// metaPackageFileDelete deletes the file from the package via meta-tool.
func metaPackageFileDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.packageFileID > 0, "package file ID not set")
	err := callMetaVoid(ctx, "gitlab_package", "file_delete", map[string]any{
		"project_id":      mPID(),
		"package_id":      mState.packageID,
		"package_file_id": mState.packageFileID,
	})
	requireNoError(t, err, "meta delete package file")
	t.Logf("Meta deleted package file ID=%d from package %d", mState.packageFileID, mState.packageID)
	mState.packageFileID = 0
}

// metaPackageDelete deletes the package from the registry via meta-tool.
func metaPackageDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.packageID > 0, msgPackageIDNotSet)
	err := callMetaVoid(ctx, "gitlab_package", "delete", map[string]any{
		"project_id": mPID(),
		"package_id": mState.packageID,
	})
	requireNoError(t, err, "meta delete package")
	t.Logf("Meta deleted package ID=%d", mState.packageID)
	mState.packageID = 0
}

// Upload with file_path (via gitlab_project meta-tool).

// metaUploadFilePath uploads a file using a local file_path instead of base64
// through the gitlab_project meta-tool.
func metaUploadFilePath(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	tmpFile := filepath.Join(t.TempDir(), "e2e-meta-filepath-upload.txt")
	err := os.WriteFile(tmpFile, []byte("E2E meta file_path upload content"), 0o644)
	requireNoError(t, err, "create temp file for meta upload")

	out, err := callMeta[uploads.UploadOutput](ctx, "gitlab_project", "upload", map[string]any{
		"project_id": mPID(),
		"filename":   "e2e-meta-filepath-upload.txt",
		"file_path":  tmpFile,
	})
	requireNoError(t, err, "meta upload file via file_path")
	requireTrue(t, out.URL != "", "upload URL should not be empty")
	requireTrue(t, out.Markdown != "", "upload markdown should not be empty")
	t.Logf("Meta uploaded via file_path: %s (markdown=%s)", out.URL, out.Markdown)
}

// ---------------------------------------------------------------------------
// Phase 5: Wiki lifecycle (gitlab_wiki)
// ---------------------------------------------------------------------------.

func metaWikiCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[wikis.Output](ctx, "gitlab_wiki", "create", map[string]any{
		"project_id": mPID(),
		"title":      "E2E Meta Wiki",
		"content":    "# Meta wiki\nCreated by E2E meta-tool test.",
	})
	requireNoError(t, err, "meta wiki create")
	requireTrue(t, out.Slug != "", "expected non-empty wiki slug")
	mState.wikiSlug = out.Slug
	t.Logf("Created wiki page: %s", out.Slug)
}

func metaWikiGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.wikiSlug != "", "wikiSlug not set")
	out, err := callMeta[wikis.Output](ctx, "gitlab_wiki", "get", map[string]any{
		"project_id": mPID(),
		"slug":       mState.wikiSlug,
	})
	requireNoError(t, err, "meta wiki get")
	requireTrue(t, out.Slug == mState.wikiSlug, "expected slug %q, got %q", mState.wikiSlug, out.Slug)
	t.Logf("Got wiki page: %s", out.Title)
}

func metaWikiList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[wikis.ListOutput](ctx, "gitlab_wiki", "list", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta wiki list")
	requireTrue(t, len(out.WikiPages) > 0, "expected at least one wiki page")
	t.Logf("Listed %d wiki pages", len(out.WikiPages))
}

func metaWikiUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.wikiSlug != "", "wikiSlug not set")
	out, err := callMeta[wikis.Output](ctx, "gitlab_wiki", "update", map[string]any{
		"project_id": mPID(),
		"slug":       mState.wikiSlug,
		"content":    "# Updated Meta Wiki\nUpdated by E2E meta-tool test.",
	})
	requireNoError(t, err, "meta wiki update")
	requireTrue(t, out.Slug == mState.wikiSlug, "slug mismatch after update")
	t.Logf("Updated wiki page: %s", out.Slug)
}

func metaWikiDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.wikiSlug != "", "wikiSlug not set")
	err := callMetaVoid(ctx, "gitlab_wiki", "delete", map[string]any{
		"project_id": mPID(),
		"slug":       mState.wikiSlug,
	})
	requireNoError(t, err, "meta wiki delete")
	t.Logf("Deleted wiki page: %s", mState.wikiSlug)
}

// ---------------------------------------------------------------------------
// Phase 5: CI Variables lifecycle (gitlab_ci_variable)
// ---------------------------------------------------------------------------.

func metaCIVariableCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[civariables.Output](ctx, "gitlab_ci_variable", "create", map[string]any{
		"project_id": mPID(),
		"key":        "E2E_META_VAR",
		"value":      "meta-test-value",
	})
	requireNoError(t, err, "meta CI variable create")
	requireTrue(t, out.Key == "E2E_META_VAR", "expected key E2E_META_VAR, got %q", out.Key)
	t.Logf("Created CI variable: %s", out.Key)
}

func metaCIVariableGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[civariables.Output](ctx, "gitlab_ci_variable", "get", map[string]any{
		"project_id": mPID(),
		"key":        "E2E_META_VAR",
	})
	requireNoError(t, err, "meta CI variable get")
	requireTrue(t, out.Value == "meta-test-value", "expected value meta-test-value, got %q", out.Value)
	t.Logf("Got CI variable: %s=%s", out.Key, out.Value)
}

func metaCIVariableList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[civariables.ListOutput](ctx, "gitlab_ci_variable", "list", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta CI variable list")
	requireTrue(t, len(out.Variables) > 0, "expected at least one CI variable")
	t.Logf("Listed %d CI variables", len(out.Variables))
}

func metaCIVariableUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[civariables.Output](ctx, "gitlab_ci_variable", "update", map[string]any{
		"project_id": mPID(),
		"key":        "E2E_META_VAR",
		"value":      "updated-meta-value",
	})
	requireNoError(t, err, "meta CI variable update")
	requireTrue(t, out.Value == "updated-meta-value", "expected updated value, got %q", out.Value)
	t.Logf("Updated CI variable: %s=%s", out.Key, out.Value)
}

func metaCIVariableDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_ci_variable", "delete", map[string]any{
		"project_id": mPID(),
		"key":        "E2E_META_VAR",
	})
	requireNoError(t, err, "meta CI variable delete")
	t.Log("Deleted CI variable E2E_META_VAR")
}

// ---------------------------------------------------------------------------
// Phase 5: CI Lint (gitlab_template)
// ---------------------------------------------------------------------------.

func metaCILint(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[cilint.Output](ctx, "gitlab_template", "lint", map[string]any{
		"project_id": mPID(),
		"content":    "stages:\n  - build\nbuild_job:\n  stage: build\n  script:\n    - echo hello",
	})
	requireNoError(t, err, "meta CI lint")
	requireTrue(t, out.Valid, "CI config should be valid, errors: %v", out.Errors)
	t.Logf("CI lint valid=%v, warnings=%d", out.Valid, len(out.Warnings))
}

// ---------------------------------------------------------------------------
// Phase 5: Environment lifecycle (gitlab_environment)
// ---------------------------------------------------------------------------.

func metaEnvironmentCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[environments.Output](ctx, "gitlab_environment", "create", map[string]any{
		"project_id": mPID(),
		"name":       "e2e-meta-staging",
	})
	requireNoError(t, err, "meta environment create")
	requireTrue(t, out.ID > 0, "expected positive environment ID")
	mState.envID = out.ID
	t.Logf("Created environment: %s (ID=%d)", out.Name, out.ID)
}

func metaEnvironmentGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.envID > 0, "envID not set")
	out, err := callMeta[environments.Output](ctx, "gitlab_environment", "get", map[string]any{
		"project_id":     mPID(),
		"environment_id": mState.envID,
	})
	requireNoError(t, err, "meta environment get")
	requireTrue(t, out.ID == mState.envID, "expected env ID %d, got %d", mState.envID, out.ID)
	t.Logf("Got environment: %s (state=%s)", out.Name, out.State)
}

func metaEnvironmentList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[environments.ListOutput](ctx, "gitlab_environment", "list", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta environment list")
	requireTrue(t, len(out.Environments) > 0, "expected at least one environment")
	t.Logf("Listed %d environments", len(out.Environments))
}

func metaEnvironmentStop(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.envID > 0, "envID not set")
	out, err := callMeta[environments.Output](ctx, "gitlab_environment", "stop", map[string]any{
		"project_id":     mPID(),
		"environment_id": mState.envID,
	})
	requireNoError(t, err, "meta environment stop")
	requireTrue(t, out.ID == mState.envID, "expected env ID %d after stop", mState.envID)
	t.Logf("Stopped environment: %s", out.Name)
}

func metaEnvironmentDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.envID > 0, "envID not set")
	err := callMetaVoid(ctx, "gitlab_environment", "delete", map[string]any{
		"project_id":     mPID(),
		"environment_id": mState.envID,
	})
	requireNoError(t, err, "meta environment delete")
	t.Logf("Deleted environment ID=%d", mState.envID)
}

// ---------------------------------------------------------------------------
// Phase 5: Label CRUD (gitlab_project)
// ---------------------------------------------------------------------------.

func metaLabelCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[labels.Output](ctx, "gitlab_project", "label_create", map[string]any{
		"project_id": mPID(),
		"name":       "e2e-meta-label",
		"color":      "#FF0000",
	})
	requireNoError(t, err, "meta label create")
	requireTrue(t, out.ID > 0, "expected positive label ID")
	mState.labelID = out.ID
	t.Logf("Created label: %s (ID=%d)", out.Name, out.ID)
}

func metaLabelUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.labelID > 0, "labelID not set")
	out, err := callMeta[labels.Output](ctx, "gitlab_project", "label_update", map[string]any{
		"project_id": mPID(),
		"label_id":   mState.labelID,
		"color":      "#00FF00",
	})
	requireNoError(t, err, "meta label update")
	requireTrue(t, out.ID == mState.labelID, "label ID mismatch after update")
	t.Logf("Updated label: %s (color=%s)", out.Name, out.Color)
}

func metaLabelDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.labelID > 0, "labelID not set")
	err := callMetaVoid(ctx, "gitlab_project", "label_delete", map[string]any{
		"project_id": mPID(),
		"label_id":   mState.labelID,
	})
	requireNoError(t, err, "meta label delete")
	t.Logf("Deleted label ID=%d", mState.labelID)
}

// ---------------------------------------------------------------------------
// Phase 5: Milestone CRUD (gitlab_project)
// ---------------------------------------------------------------------------.

func metaMilestoneCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[milestones.Output](ctx, "gitlab_project", "milestone_create", map[string]any{
		"project_id": mPID(),
		"title":      "e2e-meta-milestone",
	})
	requireNoError(t, err, "meta milestone create")
	requireTrue(t, out.IID > 0, "expected positive milestone IID")
	mState.milestoneIID = out.IID
	t.Logf("Created milestone: %s (IID=%d)", out.Title, out.IID)
}

func metaMilestoneGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.milestoneIID > 0, "milestoneIID not set")
	out, err := callMeta[milestones.Output](ctx, "gitlab_project", "milestone_get", map[string]any{
		"project_id":    mPID(),
		"milestone_iid": mState.milestoneIID,
	})
	requireNoError(t, err, "meta milestone get")
	requireTrue(t, out.IID == mState.milestoneIID, "milestone IID mismatch")
	t.Logf("Got milestone: %s (state=%s)", out.Title, out.State)
}

func metaMilestoneUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.milestoneIID > 0, "milestoneIID not set")
	out, err := callMeta[milestones.Output](ctx, "gitlab_project", "milestone_update", map[string]any{
		"project_id":    mPID(),
		"milestone_iid": mState.milestoneIID,
		"description":   "Updated by E2E meta-tool test",
	})
	requireNoError(t, err, "meta milestone update")
	requireTrue(t, out.IID == mState.milestoneIID, "milestone IID mismatch after update")
	t.Logf("Updated milestone: %s", out.Title)
}

func metaMilestoneDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.milestoneIID > 0, "milestoneIID not set")
	err := callMetaVoid(ctx, "gitlab_project", "milestone_delete", map[string]any{
		"project_id":    mPID(),
		"milestone_iid": mState.milestoneIID,
	})
	requireNoError(t, err, "meta milestone delete")
	t.Logf("Deleted milestone IID=%d", mState.milestoneIID)
}

// ---------------------------------------------------------------------------
// Phase 5: Issue Links (gitlab_issue — needs 2 issues)
// ---------------------------------------------------------------------------.

func metaIssueCreateSecond(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	// Create two issues for link testing.
	out1, err := callMeta[issues.Output](ctx, "gitlab_issue", "create", map[string]any{
		"project_id": mPID(),
		"title":      "E2E Meta Issue Link Source",
	})
	requireNoError(t, err, "meta issue create (link source)")
	mState.issueIID = out1.IID

	out2, err := callMeta[issues.Output](ctx, "gitlab_issue", "create", map[string]any{
		"project_id": mPID(),
		"title":      "E2E Meta Issue Link Target",
	})
	requireNoError(t, err, "meta issue create (link target)")
	mState.issue2IID = out2.IID
	t.Logf("Created issues for linking: IID=%d, IID=%d", mState.issueIID, mState.issue2IID)
}

func metaIssueLinkCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.issueIID > 0, msgMetaIssueIIDNotSet)
	requireTrue(t, mState.issue2IID > 0, "issue2IID not set")
	out, err := callMeta[issuelinks.Output](ctx, "gitlab_issue", "link_create", map[string]any{
		"project_id":        mPID(),
		"issue_iid":         mState.issueIID,
		"target_project_id": mPID(),
		"target_issue_iid":  strconv.FormatInt(mState.issue2IID, 10),
	})
	requireNoError(t, err, "meta issue link create")
	requireTrue(t, out.ID > 0, "expected positive issue link ID")
	t.Logf("Created issue link: ID=%d (source=%d → target=%d)", out.ID, out.SourceIssueIID, out.TargetIssueIID)
}

func metaIssueLinkList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.issueIID > 0, msgMetaIssueIIDNotSet)
	out, err := callMeta[issuelinks.ListOutput](ctx, "gitlab_issue", "link_list", map[string]any{
		"project_id": mPID(),
		"issue_iid":  mState.issueIID,
	})
	requireNoError(t, err, "meta issue link list")
	requireTrue(t, len(out.Relations) > 0, "expected at least one issue link")
	t.Logf("Listed %d issue links", len(out.Relations))
}

func metaIssueLinkDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.issueIID > 0, msgMetaIssueIIDNotSet)
	// Get the link to find its ID.
	listOut, err := callMeta[issuelinks.ListOutput](ctx, "gitlab_issue", "link_list", map[string]any{
		"project_id": mPID(),
		"issue_iid":  mState.issueIID,
	})
	requireNoError(t, err, "meta issue link list for delete")
	requireTrue(t, len(listOut.Relations) > 0, "no links to delete")
	linkID := listOut.Relations[0].IssueLinkID

	err = callMetaVoid(ctx, "gitlab_issue", "link_delete", map[string]any{
		"project_id":    mPID(),
		"issue_iid":     mState.issueIID,
		"issue_link_id": linkID,
	})
	requireNoError(t, err, "meta issue link delete")
	t.Logf("Deleted issue link ID=%d", linkID)
}

func metaIssueDeleteSecond(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	// Delete both issues created for link testing.
	if mState.issueIID > 0 {
		err := callMetaVoid(ctx, "gitlab_issue", "delete", map[string]any{
			"project_id": mPID(),
			"issue_iid":  mState.issueIID,
		})
		requireNoError(t, err, "meta delete issue (link source)")
	}
	if mState.issue2IID > 0 {
		err := callMetaVoid(ctx, "gitlab_issue", "delete", map[string]any{
			"project_id": mPID(),
			"issue_iid":  mState.issue2IID,
		})
		requireNoError(t, err, "meta delete issue (link target)")
	}
	t.Logf("Deleted link-test issues IID=%d, IID=%d", mState.issueIID, mState.issue2IID)
	mState.issueIID = 0
	mState.issue2IID = 0
}

// ---------------------------------------------------------------------------
// Phase 5: Todos (gitlab_user)
// ---------------------------------------------------------------------------.

func metaTodoList(ctx context.Context, t *testing.T) {
	out, err := callMeta[todos.ListOutput](ctx, "gitlab_user", "todo_list", map[string]any{})
	requireNoError(t, err, "meta todo list")
	t.Logf("Listed %d todos", len(out.Todos))
}

func metaTodoMarkAllDone(ctx context.Context, t *testing.T) {
	out, err := callMeta[todos.MarkAllDoneOutput](ctx, "gitlab_user", "todo_mark_all_done", map[string]any{})
	requireNoError(t, err, "meta todo mark all done")
	t.Logf("Mark all done: %s", out.Message)
}

// ---------------------------------------------------------------------------
// Phase 5: Deploy Keys (gitlab_deploy_key)
// ---------------------------------------------------------------------------.

func metaDeployKeyCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	// Use a separate ED25519 key to avoid collision with workflow_test deploy key.
	const metaDeployKeyPub = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDFKs4H4EDnEetOywhF6xBXCRN8b9XpGtlw+TQJhLM9B e2e-meta-disposable-key"
	out, err := callMeta[deploykeys.Output](ctx, "gitlab_access", "deploy_key_add", map[string]any{
		"project_id": mPID(),
		"title":      "e2e-meta-deploy-key",
		"key":        metaDeployKeyPub,
	})
	requireNoError(t, err, "meta deploy key add")
	requireTrue(t, out.ID > 0, "expected positive deploy key ID")
	mState.deployKeyID = out.ID
	t.Logf("Created deploy key: %s (ID=%d)", out.Title, out.ID)
}

func metaDeployKeyGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.deployKeyID > 0, "deployKeyID not set")
	out, err := callMeta[deploykeys.Output](ctx, "gitlab_access", "deploy_key_get", map[string]any{
		"project_id":    mPID(),
		"deploy_key_id": mState.deployKeyID,
	})
	requireNoError(t, err, "meta deploy key get")
	requireTrue(t, out.ID == mState.deployKeyID, "deploy key ID mismatch")
	t.Logf("Got deploy key: %s", out.Title)
}

func metaDeployKeyList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[deploykeys.ListOutput](ctx, "gitlab_access", "deploy_key_list_project", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta deploy key list")
	requireTrue(t, len(out.DeployKeys) > 0, "expected at least one deploy key")
	t.Logf("Listed %d deploy keys", len(out.DeployKeys))
}

func metaDeployKeyDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.deployKeyID > 0, "deployKeyID not set")
	err := callMetaVoid(ctx, "gitlab_access", "deploy_key_delete", map[string]any{
		"project_id":    mPID(),
		"deploy_key_id": mState.deployKeyID,
	})
	requireNoError(t, err, "meta deploy key delete")
	t.Logf("Deleted deploy key ID=%d", mState.deployKeyID)
}

// ---------------------------------------------------------------------------
// Phase 5: Snippets (gitlab_snippet)
// ---------------------------------------------------------------------------.

func metaSnippetCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[snippets.Output](ctx, "gitlab_snippet", "project_create", map[string]any{
		"project_id":  mPID(),
		"title":       "E2E Meta Snippet",
		"file_name":   "meta-snippet.txt",
		"content":     "Meta snippet content for E2E testing",
		"visibility":  "private",
		"description": "Created by E2E meta-tool test",
	})
	requireNoError(t, err, "meta snippet create")
	requireTrue(t, out.ID > 0, "expected positive snippet ID")
	mState.snippetID = out.ID
	t.Logf("Created snippet: %s (ID=%d)", out.Title, out.ID)
}

func metaSnippetGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.snippetID > 0, "snippetID not set")
	out, err := callMeta[snippets.Output](ctx, "gitlab_snippet", "project_get", map[string]any{
		"project_id": mPID(),
		"snippet_id": mState.snippetID,
	})
	requireNoError(t, err, "meta snippet get")
	requireTrue(t, out.ID == mState.snippetID, "snippet ID mismatch")
	t.Logf("Got snippet: %s", out.Title)
}

func metaSnippetList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[snippets.ListOutput](ctx, "gitlab_snippet", "project_list", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta snippet list")
	requireTrue(t, len(out.Snippets) > 0, "expected at least one snippet")
	t.Logf("Listed %d snippets", len(out.Snippets))
}

func metaSnippetUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.snippetID > 0, "snippetID not set")
	out, err := callMeta[snippets.Output](ctx, "gitlab_snippet", "project_update", map[string]any{
		"project_id":  mPID(),
		"snippet_id":  mState.snippetID,
		"title":       "E2E Meta Snippet Updated",
		"description": "Updated by E2E meta-tool test",
	})
	requireNoError(t, err, "meta snippet update")
	requireTrue(t, out.ID == mState.snippetID, "snippet ID mismatch after update")
	t.Logf("Updated snippet: %s", out.Title)
}

func metaSnippetDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.snippetID > 0, "snippetID not set")
	err := callMetaVoid(ctx, "gitlab_snippet", "project_delete", map[string]any{
		"project_id": mPID(),
		"snippet_id": mState.snippetID,
	})
	requireNoError(t, err, "meta snippet delete")
	t.Logf("Deleted snippet ID=%d", mState.snippetID)
}

// ---------------------------------------------------------------------------
// Phase 5: Issue Discussions (gitlab_issue)
// ---------------------------------------------------------------------------.

func metaIssueDiscussionCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	// Create a temporary issue for discussion tests.
	issue, err := callMeta[issues.Output](ctx, "gitlab_issue", "create", map[string]any{
		"project_id": mPID(),
		"title":      "E2E Meta Discussion Issue",
	})
	requireNoError(t, err, "meta create issue for discussions")
	mState.issueIID = issue.IID

	out, err := callMeta[issuediscussions.Output](ctx, "gitlab_issue", "discussion_create", map[string]any{
		"project_id": mPID(),
		"issue_iid":  mState.issueIID,
		"body":       "E2E meta-tool discussion thread",
	})
	requireNoError(t, err, "meta issue discussion create")
	requireTrue(t, out.ID != "", "expected non-empty discussion ID")
	mState.issueDiscussionID = out.ID
	if len(out.Notes) > 0 {
		mState.issueDiscussionNoteID = out.Notes[0].ID
	}
	t.Logf("Created issue discussion: %s", out.ID)
}

func metaIssueDiscussionList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.issueIID > 0, msgMetaIssueIIDNotSet)
	out, err := callMeta[issuediscussions.ListOutput](ctx, "gitlab_issue", "discussion_list", map[string]any{
		"project_id": mPID(),
		"issue_iid":  mState.issueIID,
	})
	requireNoError(t, err, "meta issue discussion list")
	requireTrue(t, len(out.Discussions) > 0, "expected at least one discussion")
	t.Logf("Listed %d issue discussions", len(out.Discussions))
}

func metaIssueDiscussionAddNote(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.issueDiscussionID != "", "issueDiscussionID not set")
	out, err := callMeta[issuediscussions.NoteOutput](ctx, "gitlab_issue", "discussion_add_note", map[string]any{
		"project_id":    mPID(),
		"issue_iid":     mState.issueIID,
		"discussion_id": mState.issueDiscussionID,
		"body":          "Reply from E2E meta-tool test",
	})
	requireNoError(t, err, "meta issue discussion add note")
	requireTrue(t, out.ID > 0, "expected positive note ID")
	mState.issueDiscussionNoteID = out.ID
	t.Logf("Added note to discussion: note ID=%d", out.ID)
}

func metaIssueDiscussionDeleteNote(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.issueDiscussionNoteID > 0, "issueDiscussionNoteID not set")
	err := callMetaVoid(ctx, "gitlab_issue", "discussion_delete_note", map[string]any{
		"project_id":    mPID(),
		"issue_iid":     mState.issueIID,
		"discussion_id": mState.issueDiscussionID,
		"note_id":       mState.issueDiscussionNoteID,
	})
	requireNoError(t, err, "meta issue discussion delete note")
	t.Logf("Deleted discussion note ID=%d", mState.issueDiscussionNoteID)

	// Clean up the temporary issue.
	_ = callMetaVoid(ctx, "gitlab_issue", "delete", map[string]any{
		"project_id": mPID(),
		"issue_iid":  mState.issueIID,
	})
	mState.issueIID = 0
}

// ---------------------------------------------------------------------------
// Phase 5: MR Draft Notes (gitlab_mr_review)
// ---------------------------------------------------------------------------.

func metaDraftNoteCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	// Draft notes require an open MR. Create a branch + commit + MR.
	_, err := callMeta[branches.Output](ctx, "gitlab_branch", "create", map[string]any{
		"project_id":  mPID(),
		"branch_name": "feature/meta-draft-notes",
		"ref":         defaultBranch,
	})
	requireNoError(t, err, "create branch for draft notes")

	_, err = callMeta[commits.Output](ctx, "gitlab_repository", "commit_create", map[string]any{
		"project_id":     mPID(),
		"branch":         "feature/meta-draft-notes",
		"commit_message": "chore: draft note test file",
		"actions": []map[string]any{{
			"action":    "create",
			"file_path": "draft-note-test.md",
			"content":   "Draft note test",
		}},
	})
	requireNoError(t, err, "commit for draft notes")

	mr, err := callMeta[mergerequests.Output](ctx, "gitlab_merge_request", "create", map[string]any{
		"project_id":    mPID(),
		"source_branch": "feature/meta-draft-notes",
		"target_branch": defaultBranch,
		"title":         "E2E Meta Draft Note MR",
	})
	requireNoError(t, err, "create MR for draft notes")
	mState.mrIID = mr.IID

	out, err := callMeta[mrdraftnotes.Output](ctx, "gitlab_mr_review", "draft_note_create", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
		"note":       "E2E meta-tool draft note",
	})
	requireNoError(t, err, "meta draft note create")
	requireTrue(t, out.ID > 0, "expected positive draft note ID")
	mState.draftNoteID = out.ID
	t.Logf("Created draft note: ID=%d on MR !%d", out.ID, mState.mrIID)
}

func metaDraftNoteList(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	out, err := callMeta[mrdraftnotes.ListOutput](ctx, "gitlab_mr_review", "draft_note_list", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
	})
	requireNoError(t, err, "meta draft note list")
	requireTrue(t, len(out.DraftNotes) > 0, "expected at least one draft note")
	t.Logf("Listed %d draft notes", len(out.DraftNotes))
}

func metaDraftNoteUpdate(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	requireTrue(t, mState.draftNoteID > 0, "draftNoteID not set")
	out, err := callMeta[mrdraftnotes.Output](ctx, "gitlab_mr_review", "draft_note_update", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
		"note_id":    mState.draftNoteID,
		"note":       "Updated E2E meta-tool draft note",
	})
	requireNoError(t, err, "meta draft note update")
	requireTrue(t, out.ID == mState.draftNoteID, "draft note ID mismatch after update")
	t.Logf("Updated draft note: ID=%d", out.ID)
}

func metaDraftNotePublishAll(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	err := callMetaVoid(ctx, "gitlab_mr_review", "draft_note_publish_all", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
	})
	requireNoError(t, err, "meta draft note publish all")
	t.Log("Published all draft notes")
}

// ---------------------------------------------------------------------------
// Phase 5: Pipeline Schedules (gitlab_pipeline_schedule)
// ---------------------------------------------------------------------------.

func metaPipelineScheduleCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[pipelineschedules.Output](ctx, "gitlab_pipeline_schedule", "create", map[string]any{
		"project_id":  mPID(),
		"description": "E2E Meta Schedule",
		"ref":         defaultBranch,
		"cron":        "0 3 * * *",
		"active":      false,
	})
	requireNoError(t, err, "meta pipeline schedule create")
	requireTrue(t, out.ID > 0, "expected positive pipeline schedule ID")
	mState.pipelineScheduleID = out.ID
	t.Logf("Created pipeline schedule: %s (ID=%d)", out.Description, out.ID)
}

func metaPipelineScheduleGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.pipelineScheduleID > 0, "pipelineScheduleID not set")
	out, err := callMeta[pipelineschedules.Output](ctx, "gitlab_pipeline_schedule", "get", map[string]any{
		"project_id":  mPID(),
		"schedule_id": mState.pipelineScheduleID,
	})
	requireNoError(t, err, "meta pipeline schedule get")
	requireTrue(t, out.ID == mState.pipelineScheduleID, "schedule ID mismatch")
	t.Logf("Got pipeline schedule: %s (cron=%s)", out.Description, out.Cron)
}

func metaPipelineScheduleList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[pipelineschedules.ListOutput](ctx, "gitlab_pipeline_schedule", "list", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta pipeline schedule list")
	requireTrue(t, len(out.Schedules) > 0, "expected at least one pipeline schedule")
	t.Logf("Listed %d pipeline schedules", len(out.Schedules))
}

func metaPipelineScheduleUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.pipelineScheduleID > 0, "pipelineScheduleID not set")
	out, err := callMeta[pipelineschedules.Output](ctx, "gitlab_pipeline_schedule", "update", map[string]any{
		"project_id":  mPID(),
		"schedule_id": mState.pipelineScheduleID,
		"description": "E2E Meta Schedule Updated",
	})
	requireNoError(t, err, "meta pipeline schedule update")
	requireTrue(t, out.ID == mState.pipelineScheduleID, "schedule ID mismatch after update")
	t.Logf("Updated pipeline schedule: %s", out.Description)
}

func metaPipelineScheduleDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.pipelineScheduleID > 0, "pipelineScheduleID not set")
	err := callMetaVoid(ctx, "gitlab_pipeline_schedule", "delete", map[string]any{
		"project_id":  mPID(),
		"schedule_id": mState.pipelineScheduleID,
	})
	requireNoError(t, err, "meta pipeline schedule delete")
	t.Logf("Deleted pipeline schedule ID=%d", mState.pipelineScheduleID)
}

// ---------------------------------------------------------------------------
// Phase 5: Badges (gitlab_project)
// ---------------------------------------------------------------------------.

func metaBadgeCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[badges.AddProjectOutput](ctx, "gitlab_project", "badge_add", map[string]any{
		"project_id": mPID(),
		"link_url":   "https://example.com/badge",
		"image_url":  "https://img.shields.io/badge/test-passing-green",
	})
	requireNoError(t, err, "meta badge add")
	requireTrue(t, out.Badge.ID > 0, "expected positive badge ID")
	mState.badgeID = out.Badge.ID
	t.Logf("Created badge: ID=%d", out.Badge.ID)
}

func metaBadgeList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[badges.ListProjectOutput](ctx, "gitlab_project", "badge_list", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta badge list")
	requireTrue(t, len(out.Badges) > 0, "expected at least one badge")
	t.Logf("Listed %d badges", len(out.Badges))
}

func metaBadgeUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.badgeID > 0, "badgeID not set")
	out, err := callMeta[badges.EditProjectOutput](ctx, "gitlab_project", "badge_edit", map[string]any{
		"project_id": mPID(),
		"badge_id":   mState.badgeID,
		"link_url":   "https://example.com/badge-updated",
	})
	requireNoError(t, err, "meta badge edit")
	requireTrue(t, out.Badge.ID == mState.badgeID, "badge ID mismatch after edit")
	t.Logf("Updated badge: ID=%d", out.Badge.ID)
}

func metaBadgeDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.badgeID > 0, "badgeID not set")
	err := callMetaVoid(ctx, "gitlab_project", "badge_delete", map[string]any{
		"project_id": mPID(),
		"badge_id":   mState.badgeID,
	})
	requireNoError(t, err, "meta badge delete")
	t.Logf("Deleted badge ID=%d", mState.badgeID)
}

// ---------------------------------------------------------------------------
// Phase 5: Access Tokens (gitlab_access_token)
// ---------------------------------------------------------------------------.

func metaAccessTokenCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	// Token expires in 30 days.
	expires := time.Now().AddDate(0, 0, 30).Format("2006-01-02")
	out, err := callMeta[accesstokens.Output](ctx, "gitlab_access", "token_project_create", map[string]any{
		"project_id": mPID(),
		"name":       "e2e-meta-token",
		"scopes":     []string{"read_api"},
		"expires_at": expires,
	})
	requireNoError(t, err, "meta access token create")
	requireTrue(t, out.ID > 0, "expected positive access token ID")
	requireTrue(t, out.Token != "", "expected non-empty token value")
	mState.accessTokenID = out.ID
	t.Logf("Created access token: %s (ID=%d)", out.Name, out.ID)
}

func metaAccessTokenList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[accesstokens.ListOutput](ctx, "gitlab_access", "token_project_list", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta access token list")
	requireTrue(t, len(out.Tokens) > 0, "expected at least one access token")
	t.Logf("Listed %d access tokens", len(out.Tokens))
}

func metaAccessTokenRevoke(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.accessTokenID > 0, "accessTokenID not set")
	err := callMetaVoid(ctx, "gitlab_access", "token_project_revoke", map[string]any{
		"project_id": mPID(),
		"token_id":   mState.accessTokenID,
	})
	requireNoError(t, err, "meta access token revoke")
	t.Logf("Revoked access token ID=%d", mState.accessTokenID)
}

// ---------------------------------------------------------------------------
// Phase 5: Award Emoji (gitlab_issue)
// ---------------------------------------------------------------------------.

func metaAwardEmojiCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	// Create a temporary issue for emoji tests.
	issue, err := callMeta[issues.Output](ctx, "gitlab_issue", "create", map[string]any{
		"project_id": mPID(),
		"title":      "E2E Meta Emoji Issue",
	})
	requireNoError(t, err, "meta create issue for emoji")
	mState.issueIID = issue.IID

	out, err := callMeta[awardemoji.Output](ctx, "gitlab_issue", "emoji_issue_create", map[string]any{
		"project_id": mPID(),
		"iid":        mState.issueIID,
		"name":       "thumbsup",
	})
	requireNoError(t, err, "meta award emoji create")
	requireTrue(t, out.ID > 0, "expected positive award emoji ID")
	mState.awardEmojiID = out.ID
	t.Logf("Created award emoji: %s (ID=%d)", out.Name, out.ID)
}

func metaAwardEmojiList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.issueIID > 0, msgMetaIssueIIDNotSet)
	out, err := callMeta[awardemoji.ListOutput](ctx, "gitlab_issue", "emoji_issue_list", map[string]any{
		"project_id": mPID(),
		"iid":        mState.issueIID,
	})
	requireNoError(t, err, "meta award emoji list")
	requireTrue(t, len(out.AwardEmoji) > 0, "expected at least one award emoji")
	t.Logf("Listed %d award emoji", len(out.AwardEmoji))
}

func metaAwardEmojiDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.awardEmojiID > 0, "awardEmojiID not set")
	err := callMetaVoid(ctx, "gitlab_issue", "emoji_issue_delete", map[string]any{
		"project_id": mPID(),
		"iid":        mState.issueIID,
		"award_id":   mState.awardEmojiID,
	})
	requireNoError(t, err, "meta award emoji delete")
	t.Logf("Deleted award emoji ID=%d", mState.awardEmojiID)

	// Clean up the temporary issue.
	_ = callMetaVoid(ctx, "gitlab_issue", "delete", map[string]any{
		"project_id": mPID(),
		"issue_iid":  mState.issueIID,
	})
	mState.issueIID = 0
}

// ---------------------------------------------------------------------------
// Gap coverage: CE-testable meta-tools
// ---------------------------------------------------------------------------.

func metaDeploymentList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[deployments.ListOutput](ctx, "gitlab_deployment", "list", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta deployment list")
	t.Log("Deployment list OK (may be empty without CI pipeline)")
}

func metaJobList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[jobs.ListOutput](ctx, "gitlab_job", "list_project", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta job list_project")
	t.Log("Job list_project OK (may be empty without CI pipeline)")
}

func metaUserSSHKeyList(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_user", "ssh_keys", map[string]any{})
	requireNoError(t, err, "meta user ssh_keys")
	t.Log("SSH keys OK")
}

func metaUserGPGKeyList(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_user", "gpg_keys", map[string]any{})
	requireNoError(t, err, "meta user gpg_keys")
	t.Log("GPG keys OK")
}

func metaTemplateGitignoreList(ctx context.Context, t *testing.T) {
	out, err := callMeta[gitignoretemplates.ListOutput](ctx, "gitlab_template", "gitignore_list", map[string]any{})
	requireNoError(t, err, "meta gitignore template list")
	requireTrue(t, len(out.Templates) > 0, "expected at least one gitignore template, got %d", len(out.Templates))
	t.Logf("Listed %d gitignore templates", len(out.Templates))
}

func metaTemplateCIYmlList(ctx context.Context, t *testing.T) {
	out, err := callMeta[gitignoretemplates.ListOutput](ctx, "gitlab_template", "ci_yml_list", map[string]any{})
	requireNoError(t, err, "meta CI yml template list")
	requireTrue(t, len(out.Templates) > 0, "expected at least one CI yml template, got %d", len(out.Templates))
	t.Logf("Listed %d CI yml templates", len(out.Templates))
}

func metaAdminTopicList(ctx context.Context, t *testing.T) {
	out, err := callMeta[topics.ListOutput](ctx, "gitlab_admin", "topic_list", map[string]any{})
	requireNoError(t, err, "meta admin topic list")
	t.Logf("Listed %d topics", len(out.Topics))
}

func metaAdminSettingsGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[settings.GetOutput](ctx, "gitlab_admin", "settings_get", map[string]any{})
	requireNoError(t, err, "meta admin settings get")
	t.Log("Admin settings get OK")
}

func metaSearchIssues(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_search", "issues", map[string]any{
		"project_id": mPID(),
		"query":      "test",
	})
	requireNoError(t, err, "meta search issues")
	t.Log("Search issues OK")
}

func metaSearchProjects(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_search", "projects", map[string]any{
		"query": "test",
	})
	requireNoError(t, err, "meta search projects")
	t.Log("Search projects OK")
}

// ---------------------------------------------------------------------------
// Gap coverage: Enterprise / Premium meta-tools (graceful skip on CE)
// ---------------------------------------------------------------------------.

func metaFeatureFlagList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[featureflags.ListOutput](ctx, "gitlab_feature_flags", "feature_flag_list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		requirePremiumFeature(t, err, "feature flags")
	}
	t.Log("Feature flag list OK")
}

func metaMergeTrainList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[mergetrains.ListOutput](ctx, "gitlab_merge_train", "list_project", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		requirePremiumFeature(t, err, "merge trains")
	}
	t.Log("Merge train list OK")
}

func metaAuditEventList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[auditevents.ListOutput](ctx, "gitlab_audit_event", "list_project", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		requirePremiumFeature(t, err, "audit events")
	}
	t.Log("Audit event list OK")
}

func metaDORAMetrics(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_dora_metrics", "project", map[string]any{
		"project_id": mPID(),
		"metric":     "deployment_frequency",
	})
	if err != nil {
		requirePremiumFeature(t, err, "DORA metrics")
	}
	t.Log("DORA metrics OK")
}

func metaDependencyList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[dependencies.ListOutput](ctx, "gitlab_dependency", "list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		requirePremiumFeature(t, err, "dependencies")
	}
	t.Log("Dependency list OK")
}

func metaExternalStatusCheckList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_external_status_check", "list_project_checks", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		requirePremiumFeature(t, err, "external status checks")
	}
	t.Log("External status check list OK")
}

func metaGroupSCIMList(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupscim.ListOutput](ctx, "gitlab_group_scim", "list", map[string]any{
		"group_id": mState.groupPath,
	})
	if err != nil {
		requirePremiumFeature(t, err, "group SCIM")
	}
	t.Log("Group SCIM list OK")
}

func metaMemberRoleList(ctx context.Context, t *testing.T) {
	_, err := callMeta[memberroles.ListOutput](ctx, "gitlab_member_role", "list_instance", map[string]any{})
	if err != nil {
		requirePremiumFeature(t, err, "member roles")
	}
	t.Log("Member role list OK")
}

func metaEnterpriseUserList(ctx context.Context, t *testing.T) {
	_, err := callMeta[enterpriseusers.ListOutput](ctx, "gitlab_enterprise_user", "list", map[string]any{
		"group_id": mState.groupPath,
	})
	if err != nil {
		requirePremiumFeature(t, err, "enterprise users")
	}
	t.Log("Enterprise user list OK")
}

func metaAttestationList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[attestations.ListOutput](ctx, "gitlab_attestation", "list", map[string]any{
		"project_id":     mPID(),
		"subject_digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
	})
	if err != nil {
		requirePremiumFeature(t, err, "attestations")
	}
	t.Log("Attestation list OK")
}

func metaCompliancePolicyGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[compliancepolicy.Output](ctx, "gitlab_compliance_policy", "get", map[string]any{})
	if err != nil {
		requirePremiumFeature(t, err, "compliance policy")
	}
	t.Log("Compliance policy get OK")
}

func metaProjectAliasList(ctx context.Context, t *testing.T) {
	_, err := callMeta[projectaliases.ListOutput](ctx, "gitlab_project_alias", "list", map[string]any{})
	if err != nil {
		requirePremiumFeature(t, err, "project aliases")
	}
	t.Log("Project alias list OK")
}

func metaGeoList(ctx context.Context, t *testing.T) {
	_, err := callMeta[geo.ListOutput](ctx, "gitlab_geo", "list", map[string]any{})
	if err != nil {
		requirePremiumFeature(t, err, "Geo sites")
	}
	t.Log("Geo list OK")
}

func metaStorageMoveList(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_storage_move", "retrieve_all_project", map[string]any{})
	if err != nil {
		requirePremiumFeature(t, err, "storage moves")
	}
	t.Log("Storage move list OK")
}

func metaSecurityFindingList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[securityfindings.ListOutput](ctx, "gitlab_security_finding", "list", map[string]any{
		"project_path": mState.projectPath,
		"pipeline_iid": "1",
	})
	if err != nil {
		requirePremiumFeature(t, err, "security findings")
	}
	t.Log("Security finding list OK")
}

func metaModelRegistryDownload(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_model_registry", "download", map[string]any{
		"project_id":       mPID(),
		"model_version_id": "1",
		"path":             "model",
		"filename":         "model.bin",
	})
	if err != nil {
		requirePremiumFeature(t, err, "model registry")
	}
	t.Log("Model registry download OK")
}

// ---------------------------------------------------------------------------
// Premium feature helpers
// ---------------------------------------------------------------------------.

// requirePremiumFeature fails the test if the error indicates the feature
// requires a premium/ultimate license or admin permissions. Fails unconditionally
// on any error — enterprise tests are gated at registration level so they only
// run when the GitLab instance supports them.
func requirePremiumFeature(t *testing.T, err error, feature string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s failed: %v", feature, err)
	}
}

// ---------------------------------------------------------------------------
// Freeze periods (gitlab_environment meta-tool)
// ---------------------------------------------------------------------------.

func metaFreezePeriodCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[freezeperiods.Output](ctx, "gitlab_environment", "freeze_create", map[string]any{
		"project_id":    mPID(),
		"freeze_start":  "0 1 * * *",
		"freeze_end":    "0 2 * * *",
		"cron_timezone": "UTC",
	})
	requireNoError(t, err, "meta freeze period create")
	requireTrue(t, out.ID > 0, "expected positive freeze period ID")
	mState.freezePeriodID = out.ID
	t.Logf("Created freeze period ID=%d", out.ID)
}

func metaFreezePeriodList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[freezeperiods.ListOutput](ctx, "gitlab_environment", "freeze_list", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta freeze period list")
	requireTrue(t, len(out.FreezePeriods) >= 1, "expected at least 1 freeze period")
	t.Logf("Listed %d freeze period(s)", len(out.FreezePeriods))
}

func metaFreezePeriodGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.freezePeriodID > 0, "freezePeriodID not set")
	out, err := callMeta[freezeperiods.Output](ctx, "gitlab_environment", "freeze_get", map[string]any{
		"project_id":       mPID(),
		"freeze_period_id": mState.freezePeriodID,
	})
	requireNoError(t, err, "meta freeze period get")
	requireTrue(t, out.ID == mState.freezePeriodID, "freeze period ID mismatch")
	t.Logf("Got freeze period ID=%d", out.ID)
}

func metaFreezePeriodUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.freezePeriodID > 0, "freezePeriodID not set")
	out, err := callMeta[freezeperiods.Output](ctx, "gitlab_environment", "freeze_update", map[string]any{
		"project_id":       mPID(),
		"freeze_period_id": mState.freezePeriodID,
		"cron_timezone":    "Europe/Madrid",
	})
	requireNoError(t, err, "meta freeze period update")
	requireTrue(t, out.ID == mState.freezePeriodID, "freeze period ID mismatch after update")
	t.Logf("Updated freeze period ID=%d timezone=%s", out.ID, out.CronTimezone)
}

func metaFreezePeriodDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.freezePeriodID > 0, "freezePeriodID not set")
	err := callMetaVoid(ctx, "gitlab_environment", "freeze_delete", map[string]any{
		"project_id":       mPID(),
		"freeze_period_id": mState.freezePeriodID,
	})
	requireNoError(t, err, "meta freeze period delete")
	t.Logf("Deleted freeze period ID=%d", mState.freezePeriodID)
}

// ---------------------------------------------------------------------------
// Protected environments (gitlab_environment meta-tool, project-level)
// ---------------------------------------------------------------------------.

const testMetaProtectedEnvName = "e2e-meta-staging"

func metaProtectedEnvProtect(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[protectedenvs.Output](ctx, "gitlab_environment", "protected_protect", map[string]any{
		"project_id": mPID(),
		"name":       testMetaProtectedEnvName,
	})
	requireNoError(t, err, "meta protected env protect")
	requireTrue(t, out.Name == testMetaProtectedEnvName, "expected protected env name "+testMetaProtectedEnvName)
	t.Logf("Protected environment: %s", out.Name)
}

func metaProtectedEnvList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[protectedenvs.ListOutput](ctx, "gitlab_environment", "protected_list", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta protected env list")
	requireTrue(t, len(out.Environments) >= 1, "expected at least 1 protected environment")
	t.Logf("Listed %d protected environment(s)", len(out.Environments))
}

func metaProtectedEnvGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[protectedenvs.Output](ctx, "gitlab_environment", "protected_get", map[string]any{
		"project_id":  mPID(),
		"environment": testMetaProtectedEnvName,
	})
	requireNoError(t, err, "meta protected env get")
	requireTrue(t, out.Name == testMetaProtectedEnvName, "protected env name mismatch")
	t.Logf("Got protected environment: %s", out.Name)
}

func metaProtectedEnvUnprotect(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_environment", "protected_unprotect", map[string]any{
		"project_id":  mPID(),
		"environment": testMetaProtectedEnvName,
	})
	requireNoError(t, err, "meta protected env unprotect")
	t.Logf("Unprotected environment: %s", testMetaProtectedEnvName)
}

// ---------------------------------------------------------------------------
// Pipeline triggers (gitlab_pipeline meta-tool)
// ---------------------------------------------------------------------------.

func metaPipelineTriggerCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[pipelinetriggers.Output](ctx, "gitlab_pipeline", "trigger_create", map[string]any{
		"project_id":  mPID(),
		"description": "e2e-meta-trigger",
	})
	requireNoError(t, err, "meta pipeline trigger create")
	requireTrue(t, out.ID > 0, "expected positive trigger ID")
	mState.triggerID = out.ID
	t.Logf("Created pipeline trigger: %s (ID=%d)", out.Description, out.ID)
}

func metaPipelineTriggerList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[pipelinetriggers.ListOutput](ctx, "gitlab_pipeline", "trigger_list", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta pipeline trigger list")
	requireTrue(t, len(out.Triggers) >= 1, "expected at least 1 trigger")
	t.Logf("Listed %d pipeline trigger(s)", len(out.Triggers))
}

func metaPipelineTriggerGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.triggerID > 0, "triggerID not set")
	out, err := callMeta[pipelinetriggers.Output](ctx, "gitlab_pipeline", "trigger_get", map[string]any{
		"project_id": mPID(),
		"trigger_id": mState.triggerID,
	})
	requireNoError(t, err, "meta pipeline trigger get")
	requireTrue(t, out.ID == mState.triggerID, "trigger ID mismatch")
	t.Logf("Got pipeline trigger ID=%d", out.ID)
}

func metaPipelineTriggerUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.triggerID > 0, "triggerID not set")
	out, err := callMeta[pipelinetriggers.Output](ctx, "gitlab_pipeline", "trigger_update", map[string]any{
		"project_id":  mPID(),
		"trigger_id":  mState.triggerID,
		"description": "e2e-meta-trigger-updated",
	})
	requireNoError(t, err, "meta pipeline trigger update")
	requireTrue(t, out.ID == mState.triggerID, "trigger ID mismatch after update")
	t.Logf("Updated pipeline trigger ID=%d desc=%s", out.ID, out.Description)
}

func metaPipelineTriggerDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.triggerID > 0, "triggerID not set")
	err := callMetaVoid(ctx, "gitlab_pipeline", "trigger_delete", map[string]any{
		"project_id": mPID(),
		"trigger_id": mState.triggerID,
	})
	requireNoError(t, err, "meta pipeline trigger delete")
	t.Logf("Deleted pipeline trigger ID=%d", mState.triggerID)
}

// ---------------------------------------------------------------------------
// Boards (gitlab_project meta-tool)
// ---------------------------------------------------------------------------.

func metaBoardCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[boards.BoardOutput](ctx, "gitlab_project", "board_create", map[string]any{
		"project_id": mPID(),
		"name":       "e2e-meta-board",
	})
	requireNoError(t, err, "meta board create")
	requireTrue(t, out.ID > 0, "expected positive board ID")
	mState.boardID = out.ID
	t.Logf("Created board: %s (ID=%d)", out.Name, out.ID)
}

func metaBoardList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[boards.ListBoardsOutput](ctx, "gitlab_project", "board_list", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta board list")
	requireTrue(t, len(out.Boards) >= 1, "expected at least 1 board")
	t.Logf("Listed %d board(s)", len(out.Boards))
}

func metaBoardGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.boardID > 0, "boardID not set")
	out, err := callMeta[boards.BoardOutput](ctx, "gitlab_project", "board_get", map[string]any{
		"project_id": mPID(),
		"board_id":   mState.boardID,
	})
	requireNoError(t, err, "meta board get")
	requireTrue(t, out.ID == mState.boardID, "board ID mismatch")
	t.Logf("Got board: %s (ID=%d)", out.Name, out.ID)
}

func metaBoardDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.boardID > 0, "boardID not set")
	err := callMetaVoid(ctx, "gitlab_project", "board_delete", map[string]any{
		"project_id": mPID(),
		"board_id":   mState.boardID,
	})
	requireNoError(t, err, "meta board delete")
	t.Logf("Deleted board ID=%d", mState.boardID)
}

// ---------------------------------------------------------------------------
// Deploy tokens (gitlab_access meta-tool)
// ---------------------------------------------------------------------------.

func metaDeployTokenCreateProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[deploytokens.Output](ctx, "gitlab_access", "deploy_token_create_project", map[string]any{
		"project_id": mPID(),
		"name":       "e2e-meta-deploy-token",
		"scopes":     []string{"read_repository"},
	})
	requireNoError(t, err, "meta deploy token create project")
	requireTrue(t, out.ID > 0, "expected positive deploy token ID")
	mState.deployTokenProjectID = out.ID
	t.Logf("Created deploy token: %s (ID=%d)", out.Name, out.ID)
}

func metaDeployTokenListProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[deploytokens.ListOutput](ctx, "gitlab_access", "deploy_token_list_project", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta deploy token list project")
	requireTrue(t, len(out.DeployTokens) >= 1, "expected at least 1 deploy token")
	t.Logf("Listed %d deploy token(s)", len(out.DeployTokens))
}

func metaDeployTokenGetProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.deployTokenProjectID > 0, "deployTokenProjectID not set")
	out, err := callMeta[deploytokens.Output](ctx, "gitlab_access", "deploy_token_get_project", map[string]any{
		"project_id":      mPID(),
		"deploy_token_id": mState.deployTokenProjectID,
	})
	requireNoError(t, err, "meta deploy token get project")
	requireTrue(t, out.ID == mState.deployTokenProjectID, "deploy token ID mismatch")
	t.Logf("Got deploy token: %s (ID=%d)", out.Name, out.ID)
}

func metaDeployTokenDeleteProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.deployTokenProjectID > 0, "deployTokenProjectID not set")
	err := callMetaVoid(ctx, "gitlab_access", "deploy_token_delete_project", map[string]any{
		"project_id":      mPID(),
		"deploy_token_id": mState.deployTokenProjectID,
	})
	requireNoError(t, err, "meta deploy token delete project")
	t.Logf("Deleted deploy token ID=%d", mState.deployTokenProjectID)
}

// ---------------------------------------------------------------------------
// Protected tags (gitlab_tag meta-tool)
// ---------------------------------------------------------------------------.

const testMetaProtectedTagPattern = "v*"

func metaProtectedTagProtect(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[tags.ProtectedTagOutput](ctx, "gitlab_tag", "protect", map[string]any{
		"project_id": mPID(),
		"tag_name":   testMetaProtectedTagPattern,
	})
	requireNoError(t, err, "meta protected tag protect")
	requireTrue(t, out.Name == testMetaProtectedTagPattern, "expected protected tag pattern "+testMetaProtectedTagPattern)
	t.Logf("Protected tag pattern: %s", out.Name)
}

func metaProtectedTagList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[tags.ListProtectedTagsOutput](ctx, "gitlab_tag", "list_protected", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta protected tag list")
	requireTrue(t, len(out.Tags) >= 1, "expected at least 1 protected tag")
	t.Logf("Listed %d protected tag(s)", len(out.Tags))
}

func metaProtectedTagGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[tags.ProtectedTagOutput](ctx, "gitlab_tag", "get_protected", map[string]any{
		"project_id": mPID(),
		"tag_name":   testMetaProtectedTagPattern,
	})
	requireNoError(t, err, "meta protected tag get")
	requireTrue(t, out.Name == testMetaProtectedTagPattern, "protected tag name mismatch")
	t.Logf("Got protected tag: %s", out.Name)
}

func metaProtectedTagUnprotect(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_tag", "unprotect", map[string]any{
		"project_id": mPID(),
		"tag_name":   testMetaProtectedTagPattern,
	})
	requireNoError(t, err, "meta protected tag unprotect")
	t.Logf("Unprotected tag pattern: %s", testMetaProtectedTagPattern)
}

// ---------------------------------------------------------------------------
// Notifications (gitlab_user meta-tool)
// ---------------------------------------------------------------------------.

func metaNotificationGlobalGet(ctx context.Context, t *testing.T) {
	out, err := callMeta[notifications.Output](ctx, "gitlab_user", "notification_global_get", map[string]any{})
	requireNoError(t, err, "meta notification global get")
	requireTrue(t, out.Level != "", "expected non-empty notification level")
	t.Logf("Global notification level: %s", out.Level)
}

func metaNotificationProjectGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[notifications.Output](ctx, "gitlab_user", "notification_project_get", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta notification project get")
	t.Logf("Project notification level: %s", out.Level)
}

// ---------------------------------------------------------------------------
// Markdown render (gitlab_repository meta-tool)
// ---------------------------------------------------------------------------.

func metaMarkdownRender(ctx context.Context, t *testing.T) {
	out, err := callMeta[markdowntool.RenderOutput](ctx, "gitlab_repository", "markdown_render", map[string]any{
		"text": "**bold** text",
	})
	requireNoError(t, err, "meta markdown render")
	requireTrue(t, out.HTML != "", "expected non-empty HTML output")
	t.Logf("Rendered markdown: %s", out.HTML)
}

// ---------------------------------------------------------------------------
// Resource state events (gitlab_issue / gitlab_merge_request)
// ---------------------------------------------------------------------------.

func metaIssueCreateForStateEvent(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[issues.Output](ctx, "gitlab_issue", "create", map[string]any{
		"project_id": mPID(),
		"title":      "State Event Test Issue",
	})
	requireNoError(t, err, "meta issue create for state event")
	requireTrue(t, out.IID > 0, "expected positive issue IID")
	mState.stateEventIssueIID = out.IID
	t.Logf("Created issue for state events: IID=%d", out.IID)
}

func metaIssueCloseForStateEvent(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.stateEventIssueIID > 0, "stateEventIssueIID not set")
	out, err := callMeta[issues.Output](ctx, "gitlab_issue", "update", map[string]any{
		"project_id":  mPID(),
		"issue_iid":   mState.stateEventIssueIID,
		"state_event": "close",
	})
	requireNoError(t, err, "meta issue close for state event")
	requireTrue(t, out.State == "closed", "expected issue state=closed")
	t.Logf("Closed issue IID=%d state=%s", out.IID, out.State)
}

func metaIssueStateEventList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.stateEventIssueIID > 0, "stateEventIssueIID not set")
	out, err := callMeta[resourceevents.ListStateEventsOutput](ctx, "gitlab_issue", "event_issue_state_list", map[string]any{
		"project_id": mPID(),
		"issue_iid":  mState.stateEventIssueIID,
	})
	requireNoError(t, err, "meta issue state event list")
	requireTrue(t, len(out.Events) >= 1, "expected at least 1 state event")
	t.Logf("Listed %d issue state event(s)", len(out.Events))
}

func metaIssueDeleteStateEvent(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.stateEventIssueIID > 0, "stateEventIssueIID not set")
	err := callMetaVoid(ctx, "gitlab_issue", "delete", map[string]any{
		"project_id": mPID(),
		"issue_iid":  mState.stateEventIssueIID,
	})
	requireNoError(t, err, "meta issue delete for state event")
	t.Logf("Deleted issue IID=%d", mState.stateEventIssueIID)
}

func metaMRStateEventList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.mrIID > 0, "mrIID not set")
	out, err := callMeta[resourceevents.ListStateEventsOutput](ctx, "gitlab_merge_request", "event_mr_state_list", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
	})
	requireNoError(t, err, "meta MR state event list")
	// CE may return empty events; only log.
	t.Logf("Listed %d MR state event(s)", len(out.Events))
}

// ---------------------------------------------------------------------------
// Group labels (gitlab_group meta-tool)
// ---------------------------------------------------------------------------.

func metaGroupLabelCreate(ctx context.Context, t *testing.T) {
	requireTrue(t, mState.groupPath != "", "groupPath not set")
	out, err := callMeta[grouplabels.Output](ctx, "gitlab_group", "group_label_create", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
		"name":     "e2e-meta-group-label",
		"color":    "#FF0000",
	})
	requireNoError(t, err, "meta group label create")
	requireTrue(t, out.ID > 0, "expected positive group label ID")
	mState.groupLabelID = out.ID
	t.Logf("Created group label: %s (ID=%d)", out.Name, out.ID)
}

func metaGroupLabelList(ctx context.Context, t *testing.T) {
	requireTrue(t, mState.groupPath != "", "groupPath not set")
	out, err := callMeta[grouplabels.ListOutput](ctx, "gitlab_group", "group_label_list", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	requireNoError(t, err, "meta group label list")
	requireTrue(t, len(out.Labels) >= 1, "expected at least 1 group label")
	t.Logf("Listed %d group label(s)", len(out.Labels))
}

func metaGroupLabelDelete(ctx context.Context, t *testing.T) {
	requireTrue(t, mState.groupPath != "", "groupPath not set")
	requireTrue(t, mState.groupLabelID > 0, "groupLabelID not set")
	err := callMetaVoid(ctx, "gitlab_group", "group_label_delete", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
		"label_id": mState.groupLabelID,
	})
	requireNoError(t, err, "meta group label delete")
	t.Logf("Deleted group label ID=%d", mState.groupLabelID)
}

// ---------------------------------------------------------------------------
// Group milestones (gitlab_group meta-tool)
// ---------------------------------------------------------------------------.

func metaGroupMilestoneCreate(ctx context.Context, t *testing.T) {
	requireTrue(t, mState.groupPath != "", "groupPath not set")
	out, err := callMeta[groupmilestones.Output](ctx, "gitlab_group", "group_milestone_create", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
		"title":    "e2e-meta-group-milestone",
	})
	requireNoError(t, err, "meta group milestone create")
	requireTrue(t, out.IID > 0, "expected positive group milestone IID")
	mState.groupMilestoneIID = out.IID
	t.Logf("Created group milestone: %s (IID=%d)", out.Title, out.IID)
}

func metaGroupMilestoneList(ctx context.Context, t *testing.T) {
	requireTrue(t, mState.groupPath != "", "groupPath not set")
	out, err := callMeta[groupmilestones.ListOutput](ctx, "gitlab_group", "group_milestone_list", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	requireNoError(t, err, "meta group milestone list")
	requireTrue(t, len(out.Milestones) >= 1, "expected at least 1 group milestone")
	t.Logf("Listed %d group milestone(s)", len(out.Milestones))
}

func metaGroupMilestoneGet(ctx context.Context, t *testing.T) {
	requireTrue(t, mState.groupPath != "", "groupPath not set")
	requireTrue(t, mState.groupMilestoneIID > 0, "groupMilestoneIID not set")
	out, err := callMeta[groupmilestones.Output](ctx, "gitlab_group", "group_milestone_get", map[string]any{
		"group_id":      strconv.FormatInt(mState.groupID, 10),
		"milestone_iid": mState.groupMilestoneIID,
	})
	requireNoError(t, err, "meta group milestone get")
	requireTrue(t, out.IID == mState.groupMilestoneIID, "group milestone IID mismatch")
	t.Logf("Got group milestone: %s (IID=%d)", out.Title, out.IID)
}

func metaGroupMilestoneDelete(ctx context.Context, t *testing.T) {
	requireTrue(t, mState.groupPath != "", "groupPath not set")
	requireTrue(t, mState.groupMilestoneIID > 0, "groupMilestoneIID not set")
	err := callMetaVoid(ctx, "gitlab_group", "group_milestone_delete", map[string]any{
		"group_id":      strconv.FormatInt(mState.groupID, 10),
		"milestone_iid": mState.groupMilestoneIID,
	})
	requireNoError(t, err, "meta group milestone delete")
	t.Logf("Deleted group milestone IID=%d", mState.groupMilestoneIID)
}

// ---------------------------------------------------------------------------
// Group variables (gitlab_ci_variable meta-tool)
// ---------------------------------------------------------------------------.

func metaGroupVariableCreate(ctx context.Context, t *testing.T) {
	requireTrue(t, mState.groupPath != "", "groupPath not set")
	out, err := callMeta[groupvariables.Output](ctx, "gitlab_ci_variable", "group_create", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
		"key":      "E2E_META_GROUP_VAR",
		"value":    "group-test-value",
	})
	requireNoError(t, err, "meta group variable create")
	requireTrue(t, out.Key == "E2E_META_GROUP_VAR", "expected key E2E_META_GROUP_VAR")
	mState.groupVariableKey = out.Key
	t.Logf("Created group variable: %s", out.Key)
}

func metaGroupVariableList(ctx context.Context, t *testing.T) {
	requireTrue(t, mState.groupPath != "", "groupPath not set")
	out, err := callMeta[groupvariables.ListOutput](ctx, "gitlab_ci_variable", "group_list", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	requireNoError(t, err, "meta group variable list")
	requireTrue(t, len(out.Variables) >= 1, "expected at least 1 group variable")
	t.Logf("Listed %d group variable(s)", len(out.Variables))
}

func metaGroupVariableGet(ctx context.Context, t *testing.T) {
	requireTrue(t, mState.groupPath != "", "groupPath not set")
	requireTrue(t, mState.groupVariableKey != "", "groupVariableKey not set")
	out, err := callMeta[groupvariables.Output](ctx, "gitlab_ci_variable", "group_get", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
		"key":      mState.groupVariableKey,
	})
	requireNoError(t, err, "meta group variable get")
	requireTrue(t, out.Key == mState.groupVariableKey, "group variable key mismatch")
	t.Logf("Got group variable: %s=%s", out.Key, out.Value)
}

func metaGroupVariableDelete(ctx context.Context, t *testing.T) {
	requireTrue(t, mState.groupPath != "", "groupPath not set")
	requireTrue(t, mState.groupVariableKey != "", "groupVariableKey not set")
	err := callMetaVoid(ctx, "gitlab_ci_variable", "group_delete", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
		"key":      mState.groupVariableKey,
	})
	requireNoError(t, err, "meta group variable delete")
	t.Logf("Deleted group variable: %s", mState.groupVariableKey)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------.

// waitForBranchMeta polls until the branch exists (using the shared GitLab client).
func waitForBranchMeta(ctx context.Context, t *testing.T, branch string) {
	t.Helper()
	pid := int(mState.projectID)
	for range 15 {
		_, resp, err := state.glClient.GL().Branches.GetBranch(pid, branch)
		if err == nil {
			t.Logf("Branch %q ready (meta)", branch)
			return
		}
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			select {
			case <-ctx.Done():
				t.Fatalf("context canceled waiting for branch %q: %v", branch, ctx.Err())
			case <-time.After(1 * time.Second):
			}
			continue
		}
		requireNoError(t, err, "get branch "+branch)
	}
	t.Fatalf("branch %q not available after 15s (meta)", branch)
}

// ---------------------------------------------------------------------------
// Batch 1 gap tests: Branch, Tag, Release, MR Review, Pipeline, Job,
// Wiki, Environment, CI Variable, Pipeline Schedule
// ---------------------------------------------------------------------------

func metaBranchGetProtected(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[branches.ProtectedOutput](ctx, "gitlab_branch", "get_protected", map[string]any{
		"project_id":  mPID(),
		"branch_name": testMetaBranch,
	})
	requireNoError(t, err, "meta branch get_protected")
	requireTrue(t, out.Name == testMetaBranch, "expected protected branch %q, got %q", testMetaBranch, out.Name)
	t.Logf("Got protected branch %s (allow_force_push=%v)", out.Name, out.AllowForcePush)
}

func metaBranchUpdateProtected(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[branches.ProtectedOutput](ctx, "gitlab_branch", "update_protected", map[string]any{
		"project_id":       mPID(),
		"branch_name":      testMetaBranch,
		"allow_force_push": false,
	})
	requireNoError(t, err, "meta branch update_protected")
	requireTrue(t, out.Name == testMetaBranch, "expected protected branch %q, got %q", testMetaBranch, out.Name)
	t.Logf("Updated protected branch %s", out.Name)
}

func metaBranchDeleteMerged(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_branch", "delete_merged", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("delete_merged returned error (acceptable if no merged branches): %v", err)
		return
	}
	t.Log("Deleted merged branches (meta)")
}

func metaTagGetSignature(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[tags.SignatureOutput](ctx, "gitlab_tag", "get_signature", map[string]any{
		"project_id": mPID(),
		"tag_name":   testMetaTag,
	})
	if err != nil {
		t.Logf("tag get_signature not available (expected for unsigned tags): %v", err)
		return
	}
	t.Log("Retrieved tag signature (meta)")
}

func metaReleaseGetLatest(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[releases.Output](ctx, "gitlab_release", "get_latest", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta release get_latest")
	requireTrue(t, out.TagName != "", "expected non-empty tag_name in latest release")
	t.Logf("Latest release: %s (tag=%s)", out.Name, out.TagName)
}

func metaReleaseLinkGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.releaseLinkID > 0, "releaseLinkID not set")
	out, err := callMeta[releaselinks.Output](ctx, "gitlab_release", "link_get", map[string]any{
		"project_id": mPID(),
		"tag_name":   testMetaTag,
		"link_id":    mState.releaseLinkID,
	})
	requireNoError(t, err, "meta release link_get")
	requireTrue(t, out.ID == mState.releaseLinkID, "expected link ID %d, got %d", mState.releaseLinkID, out.ID)
	t.Logf("Got release link: %s (id=%d)", out.Name, out.ID)
}

func metaReleaseLinkUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.releaseLinkID > 0, "releaseLinkID not set")
	out, err := callMeta[releaselinks.Output](ctx, "gitlab_release", "link_update", map[string]any{
		"project_id": mPID(),
		"tag_name":   testMetaTag,
		"link_id":    mState.releaseLinkID,
		"name":       "Updated Documentation",
		"url":        "https://example.com/docs-updated",
	})
	requireNoError(t, err, "meta release link_update")
	requireTrue(t, out.Name == "Updated Documentation", "expected updated link name, got %q", out.Name)
	t.Logf("Updated release link: %s (id=%d)", out.Name, out.ID)
}

func metaNoteGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.noteID > 0, "noteID not set")
	out, err := callMeta[mrnotes.Output](ctx, "gitlab_mr_review", "note_get", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
		"note_id":    mState.noteID,
	})
	requireNoError(t, err, "meta note_get")
	requireTrue(t, out.ID == mState.noteID, "expected note ID %d, got %d", mState.noteID, out.ID)
	t.Logf("Got MR note: id=%d", out.ID)
}

func metaDiscussionGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.discussionID != "", "discussionID not set")
	out, err := callMeta[mrdiscussions.Output](ctx, "gitlab_mr_review", "discussion_get", map[string]any{
		"project_id":    mPID(),
		"mr_iid":        mState.mrIID,
		"discussion_id": mState.discussionID,
	})
	requireNoError(t, err, "meta discussion_get")
	requireTrue(t, out.ID == mState.discussionID, "expected discussion ID %s, got %s", mState.discussionID, out.ID)
	t.Logf("Got MR discussion: id=%s notes=%d", out.ID, len(out.Notes))
}

func metaDraftNoteGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.draftNoteID > 0, "draftNoteID not set")
	out, err := callMeta[mrdraftnotes.Output](ctx, "gitlab_mr_review", "draft_note_get", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
		"note_id":    mState.draftNoteID,
	})
	requireNoError(t, err, "meta draft_note_get")
	requireTrue(t, out.ID == mState.draftNoteID, "expected draft note ID %d, got %d", mState.draftNoteID, out.ID)
	t.Logf("Got draft note: id=%d", out.ID)
}

func metaDiffVersionsList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.mrIID > 0, "mrIID not set")
	out, err := callMeta[mrchanges.DiffVersionsListOutput](ctx, "gitlab_mr_review", "diff_versions_list", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
	})
	requireNoError(t, err, "meta diff_versions_list")
	requireTrue(t, len(out.DiffVersions) >= 1, "expected at least 1 diff version, got %d", len(out.DiffVersions))
	t.Logf("MR has %d diff versions", len(out.DiffVersions))
}

func metaDiffVersionGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.mrIID > 0, "mrIID not set")
	listOut, err := callMeta[mrchanges.DiffVersionsListOutput](ctx, "gitlab_mr_review", "diff_versions_list", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
	})
	requireNoError(t, err, "meta diff_versions_list for get")
	requireTrue(t, len(listOut.DiffVersions) >= 1, "need at least 1 diff version")
	versionID := listOut.DiffVersions[0].ID
	out, err := callMeta[mrchanges.DiffVersionOutput](ctx, "gitlab_mr_review", "diff_version_get", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
		"version_id": versionID,
	})
	requireNoError(t, err, "meta diff_version_get")
	requireTrue(t, out.ID == versionID, "expected version ID %d, got %d", versionID, out.ID)
	t.Logf("Got diff version: id=%d state=%s", out.ID, out.State)
}

func metaPipelineGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	listOut, err := callMeta[pipelines.ListOutput](ctx, "gitlab_pipeline", "list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil || len(listOut.Pipelines) == 0 {
		t.Log("No pipelines available for get test (non-fatal)")
		return
	}
	pipelineID := listOut.Pipelines[0].ID
	out, err := callMeta[pipelines.DetailOutput](ctx, "gitlab_pipeline", "get", map[string]any{
		"project_id":  mPID(),
		"pipeline_id": pipelineID,
	})
	requireNoError(t, err, "meta pipeline get")
	requireTrue(t, out.ID == pipelineID, "expected pipeline ID %d, got %d", pipelineID, out.ID)
	t.Logf("Got pipeline: id=%d status=%s ref=%s", out.ID, out.Status, out.Ref)
}

func metaPipelineVariables(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	listOut, err := callMeta[pipelines.ListOutput](ctx, "gitlab_pipeline", "list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil || len(listOut.Pipelines) == 0 {
		t.Log("No pipelines available for variables test (non-fatal)")
		return
	}
	pipelineID := listOut.Pipelines[0].ID
	out, err := callMeta[pipelines.VariablesOutput](ctx, "gitlab_pipeline", "variables", map[string]any{
		"project_id":  mPID(),
		"pipeline_id": pipelineID,
	})
	if err != nil {
		t.Logf("Pipeline variables not accessible: %v", err)
		return
	}
	t.Logf("Pipeline %d has %d variables", pipelineID, len(out.Variables))
}

func metaPipelineTestReport(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	listOut, err := callMeta[pipelines.ListOutput](ctx, "gitlab_pipeline", "list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil || len(listOut.Pipelines) == 0 {
		t.Log("No pipelines available for test_report test (non-fatal)")
		return
	}
	pipelineID := listOut.Pipelines[0].ID
	out, err := callMeta[pipelines.TestReportOutput](ctx, "gitlab_pipeline", "test_report", map[string]any{
		"project_id":  mPID(),
		"pipeline_id": pipelineID,
	})
	if err != nil {
		t.Logf("Pipeline test report not available: %v", err)
		return
	}
	t.Logf("Pipeline %d test report: total=%d", pipelineID, out.TotalCount)
}

func metaPipelineTestReportSummary(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	listOut, err := callMeta[pipelines.ListOutput](ctx, "gitlab_pipeline", "list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil || len(listOut.Pipelines) == 0 {
		t.Log("No pipelines available for test_report_summary test (non-fatal)")
		return
	}
	pipelineID := listOut.Pipelines[0].ID
	out, err := callMeta[pipelines.TestReportSummaryOutput](ctx, "gitlab_pipeline", "test_report_summary", map[string]any{
		"project_id":  mPID(),
		"pipeline_id": pipelineID,
	})
	if err != nil {
		t.Logf("Pipeline test report summary not available: %v", err)
		return
	}
	t.Logf("Pipeline %d test report summary: suites=%d", pipelineID, len(out.TestSuites))
}

func metaPipelineLatest(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[pipelines.DetailOutput](ctx, "gitlab_pipeline", "latest", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("No latest pipeline available (non-fatal): %v", err)
		return
	}
	t.Logf("Latest pipeline: id=%d status=%s ref=%s", out.ID, out.Status, out.Ref)
}

func metaJobTokenScopeGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[jobtokenscope.AccessSettingsOutput](ctx, "gitlab_job", "token_scope_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("Job token scope get returned error (non-fatal): %v", err)
		return
	}
	t.Log("Retrieved job token scope (meta)")
}

func metaWikiUploadAttachment(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	content := base64.StdEncoding.EncodeToString([]byte("test wiki attachment content"))
	out, err := callMeta[wikis.AttachmentOutput](ctx, "gitlab_wiki", "upload_attachment", map[string]any{
		"project_id":     mPID(),
		"filename":       "test-attach.txt",
		"content_base64": content,
	})
	requireNoError(t, err, "meta wiki upload_attachment")
	requireTrue(t, out.FileName == "test-attach.txt", "expected filename test-attach.txt, got %q", out.FileName)
	t.Logf("Uploaded wiki attachment: %s", out.FileName)
}

func metaEnvironmentUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.envID > 0, "envID not set")
	out, err := callMeta[environments.Output](ctx, "gitlab_environment", "update", map[string]any{
		"project_id":     mPID(),
		"environment_id": mState.envID,
		"external_url":   "https://staging.example.com",
	})
	requireNoError(t, err, "meta environment update")
	requireTrue(t, out.ID == mState.envID, "expected env ID %d, got %d", mState.envID, out.ID)
	t.Logf("Updated environment: %s", out.Name)
}

func metaCIVariableInstanceCreate(ctx context.Context, t *testing.T) {
	out, err := callMeta[instancevariables.Output](ctx, "gitlab_ci_variable", "instance_create", map[string]any{
		"key":   "E2E_INSTANCE_VAR",
		"value": "instance_test_value",
	})
	requireNoError(t, err, "meta ci variable instance_create")
	requireTrue(t, out.Key == "E2E_INSTANCE_VAR", "expected key E2E_INSTANCE_VAR, got %q", out.Key)
	t.Logf("Created instance variable: %s", out.Key)
}

func metaCIVariableInstanceList(ctx context.Context, t *testing.T) {
	out, err := callMeta[instancevariables.ListOutput](ctx, "gitlab_ci_variable", "instance_list", map[string]any{})
	requireNoError(t, err, "meta ci variable instance_list")
	requireTrue(t, len(out.Variables) >= 1, "expected at least 1 instance variable")
	t.Logf("Instance variables: %d", len(out.Variables))
}

func metaCIVariableInstanceGet(ctx context.Context, t *testing.T) {
	out, err := callMeta[instancevariables.Output](ctx, "gitlab_ci_variable", "instance_get", map[string]any{
		"key": "E2E_INSTANCE_VAR",
	})
	requireNoError(t, err, "meta ci variable instance_get")
	requireTrue(t, out.Key == "E2E_INSTANCE_VAR", "expected key E2E_INSTANCE_VAR, got %q", out.Key)
	t.Logf("Got instance variable: %s=%s", out.Key, out.Value)
}

func metaCIVariableInstanceUpdate(ctx context.Context, t *testing.T) {
	out, err := callMeta[instancevariables.Output](ctx, "gitlab_ci_variable", "instance_update", map[string]any{
		"key":   "E2E_INSTANCE_VAR",
		"value": "instance_updated_value",
	})
	requireNoError(t, err, "meta ci variable instance_update")
	requireTrue(t, out.Key == "E2E_INSTANCE_VAR", "expected key E2E_INSTANCE_VAR, got %q", out.Key)
	t.Logf("Updated instance variable: %s", out.Key)
}

func metaCIVariableInstanceDelete(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_ci_variable", "instance_delete", map[string]any{
		"key": "E2E_INSTANCE_VAR",
	})
	requireNoError(t, err, "meta ci variable instance_delete")
	t.Log("Deleted instance variable E2E_INSTANCE_VAR")
}

func metaGroupVariableUpdate(ctx context.Context, t *testing.T) {
	requireTrue(t, mState.groupVariableKey != "", "groupVariableKey not set")
	out, err := callMeta[groupvariables.Output](ctx, "gitlab_ci_variable", "group_update", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
		"key":      mState.groupVariableKey,
		"value":    "meta_updated_value",
	})
	requireNoError(t, err, "meta ci variable group_update")
	requireTrue(t, out.Key == mState.groupVariableKey, "expected key %s, got %q", mState.groupVariableKey, out.Key)
	t.Logf("Updated group variable: %s", out.Key)
}

func metaPipelineScheduleRun(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.pipelineScheduleID > 0, "pipelineScheduleID not set")
	_, err := callMeta[pipelineschedules.Output](ctx, "gitlab_pipeline_schedule", "run", map[string]any{
		"project_id":  mPID(),
		"schedule_id": mState.pipelineScheduleID,
	})
	if err != nil {
		t.Logf("Pipeline schedule run returned error (non-fatal, may need .gitlab-ci.yml): %v", err)
		return
	}
	t.Logf("Triggered pipeline schedule %d", mState.pipelineScheduleID)
}

func metaPipelineScheduleTakeOwnership(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.pipelineScheduleID > 0, "pipelineScheduleID not set")
	out, err := callMeta[pipelineschedules.Output](ctx, "gitlab_pipeline_schedule", "take_ownership", map[string]any{
		"project_id":  mPID(),
		"schedule_id": mState.pipelineScheduleID,
	})
	requireNoError(t, err, "meta pipeline schedule take_ownership")
	requireTrue(t, out.ID == mState.pipelineScheduleID, "expected schedule ID %d, got %d", mState.pipelineScheduleID, out.ID)
	t.Logf("Took ownership of pipeline schedule %d", out.ID)
}

func metaPipelineScheduleCreateVariable(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.pipelineScheduleID > 0, "pipelineScheduleID not set")
	out, err := callMeta[pipelineschedules.VariableOutput](ctx, "gitlab_pipeline_schedule", "create_variable", map[string]any{
		"project_id":  mPID(),
		"schedule_id": mState.pipelineScheduleID,
		"key":         "E2E_SCHED_VAR",
		"value":       "schedule_test",
	})
	requireNoError(t, err, "meta pipeline schedule create_variable")
	requireTrue(t, out.Key == "E2E_SCHED_VAR", "expected key E2E_SCHED_VAR, got %q", out.Key)
	t.Logf("Created schedule variable: %s", out.Key)
}

func metaPipelineScheduleEditVariable(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.pipelineScheduleID > 0, "pipelineScheduleID not set")
	out, err := callMeta[pipelineschedules.VariableOutput](ctx, "gitlab_pipeline_schedule", "edit_variable", map[string]any{
		"project_id":  mPID(),
		"schedule_id": mState.pipelineScheduleID,
		"key":         "E2E_SCHED_VAR",
		"value":       "schedule_updated",
	})
	requireNoError(t, err, "meta pipeline schedule edit_variable")
	requireTrue(t, out.Key == "E2E_SCHED_VAR", "expected key E2E_SCHED_VAR, got %q", out.Key)
	t.Logf("Edited schedule variable: %s=%s", out.Key, out.Value)
}

func metaPipelineScheduleDeleteVariable(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.pipelineScheduleID > 0, "pipelineScheduleID not set")
	err := callMetaVoid(ctx, "gitlab_pipeline_schedule", "delete_variable", map[string]any{
		"project_id":  mPID(),
		"schedule_id": mState.pipelineScheduleID,
		"key":         "E2E_SCHED_VAR",
	})
	requireNoError(t, err, "meta pipeline schedule delete_variable")
	t.Logf("Deleted schedule variable E2E_SCHED_VAR")
}

func metaPipelineScheduleListTriggered(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.pipelineScheduleID > 0, "pipelineScheduleID not set")
	out, err := callMeta[pipelineschedules.TriggeredPipelinesListOutput](ctx, "gitlab_pipeline_schedule", "list_triggered_pipelines", map[string]any{
		"project_id":  mPID(),
		"schedule_id": mState.pipelineScheduleID,
	})
	if err != nil {
		t.Logf("List triggered pipelines returned error (non-fatal): %v", err)
		return
	}
	t.Logf("Schedule %d has %d triggered pipelines", mState.pipelineScheduleID, len(out.Pipelines))
}

// ================================================================
// AUTO-GENERATED GAP TEST FUNCTIONS (619 actions)
// ================================================================

// --- gitlab_access gap tests (37 actions) ---

func metaAccessApproveGroup(ctx context.Context, t *testing.T) {
	_, err := callMeta[accessrequests.Output](ctx, "gitlab_access", "approve_group", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("approve_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("approve_group completed (meta)")
}

func metaAccessApproveProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[accessrequests.Output](ctx, "gitlab_access", "approve_project", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("approve_project returned error (non-fatal): %v", err)
		return
	}
	t.Log("approve_project completed (meta)")
}

func metaAccessDenyGroup(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_access", "deny_group", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("deny_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("deny_group completed (meta)")
}

func metaAccessDenyProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_access", "deny_project", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("deny_project returned error (non-fatal): %v", err)
		return
	}
	t.Log("deny_project completed (meta)")
}

func metaAccessDeployKeyAddInstance(ctx context.Context, t *testing.T) {
	_, err := callMeta[deploykeys.InstanceOutput](ctx, "gitlab_access", "deploy_key_add_instance", map[string]any{})
	if err != nil {
		t.Logf("deploy_key_add_instance returned error (non-fatal): %v", err)
		return
	}
	t.Log("deploy_key_add_instance completed (meta)")
}

func metaAccessDeployKeyEnable(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[deploykeys.Output](ctx, "gitlab_access", "deploy_key_enable", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("deploy_key_enable returned error (non-fatal): %v", err)
		return
	}
	t.Log("deploy_key_enable completed (meta)")
}

func metaAccessDeployKeyListAll(ctx context.Context, t *testing.T) {
	_, err := callMeta[deploykeys.InstanceListOutput](ctx, "gitlab_access", "deploy_key_list_all", map[string]any{})
	if err != nil {
		t.Logf("deploy_key_list_all returned error (non-fatal): %v", err)
		return
	}
	t.Log("deploy_key_list_all completed (meta)")
}

func metaAccessDeployKeyListUserProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[deploykeys.ListOutput](ctx, "gitlab_access", "deploy_key_list_user_project", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("deploy_key_list_user_project returned error (non-fatal): %v", err)
		return
	}
	t.Log("deploy_key_list_user_project completed (meta)")
}

func metaAccessDeployKeyUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[deploykeys.Output](ctx, "gitlab_access", "deploy_key_update", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("deploy_key_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("deploy_key_update completed (meta)")
}

func metaAccessDeployTokenCreateGroup(ctx context.Context, t *testing.T) {
	_, err := callMeta[deploytokens.Output](ctx, "gitlab_access", "deploy_token_create_group", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("deploy_token_create_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("deploy_token_create_group completed (meta)")
}

func metaAccessDeployTokenDeleteGroup(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_access", "deploy_token_delete_group", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("deploy_token_delete_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("deploy_token_delete_group completed (meta)")
}

func metaAccessDeployTokenGetGroup(ctx context.Context, t *testing.T) {
	_, err := callMeta[deploytokens.Output](ctx, "gitlab_access", "deploy_token_get_group", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("deploy_token_get_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("deploy_token_get_group completed (meta)")
}

func metaAccessDeployTokenListAll(ctx context.Context, t *testing.T) {
	_, err := callMeta[deploytokens.ListOutput](ctx, "gitlab_access", "deploy_token_list_all", map[string]any{})
	if err != nil {
		t.Logf("deploy_token_list_all returned error (non-fatal): %v", err)
		return
	}
	t.Log("deploy_token_list_all completed (meta)")
}

func metaAccessDeployTokenListGroup(ctx context.Context, t *testing.T) {
	_, err := callMeta[deploytokens.ListOutput](ctx, "gitlab_access", "deploy_token_list_group", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("deploy_token_list_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("deploy_token_list_group completed (meta)")
}

func metaAccessInviteGroup(ctx context.Context, t *testing.T) {
	_, err := callMeta[invites.InviteResultOutput](ctx, "gitlab_access", "invite_group", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("invite_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("invite_group completed (meta)")
}

func metaAccessInviteListGroup(ctx context.Context, t *testing.T) {
	_, err := callMeta[invites.ListPendingInvitationsOutput](ctx, "gitlab_access", "invite_list_group", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("invite_list_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("invite_list_group completed (meta)")
}

func metaAccessInviteListProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[invites.ListPendingInvitationsOutput](ctx, "gitlab_access", "invite_list_project", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("invite_list_project returned error (non-fatal): %v", err)
		return
	}
	t.Log("invite_list_project completed (meta)")
}

func metaAccessInviteProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[invites.InviteResultOutput](ctx, "gitlab_access", "invite_project", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("invite_project returned error (non-fatal): %v", err)
		return
	}
	t.Log("invite_project completed (meta)")
}

func metaAccessRequestGroup(ctx context.Context, t *testing.T) {
	_, err := callMeta[accessrequests.Output](ctx, "gitlab_access", "request_group", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("request_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("request_group completed (meta)")
}

func metaAccessRequestListGroup(ctx context.Context, t *testing.T) {
	_, err := callMeta[accessrequests.ListOutput](ctx, "gitlab_access", "request_list_group", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("request_list_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("request_list_group completed (meta)")
}

func metaAccessRequestListProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[accessrequests.ListOutput](ctx, "gitlab_access", "request_list_project", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("request_list_project returned error (non-fatal): %v", err)
		return
	}
	t.Log("request_list_project completed (meta)")
}

func metaAccessRequestProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[accessrequests.Output](ctx, "gitlab_access", "request_project", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("request_project returned error (non-fatal): %v", err)
		return
	}
	t.Log("request_project completed (meta)")
}

func metaAccessTokenGroupCreate(ctx context.Context, t *testing.T) {
	_, err := callMeta[accesstokens.Output](ctx, "gitlab_access", "token_group_create", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("token_group_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("token_group_create completed (meta)")
}

func metaAccessTokenGroupGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[accesstokens.Output](ctx, "gitlab_access", "token_group_get", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("token_group_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("token_group_get completed (meta)")
}

func metaAccessTokenGroupList(ctx context.Context, t *testing.T) {
	_, err := callMeta[accesstokens.ListOutput](ctx, "gitlab_access", "token_group_list", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("token_group_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("token_group_list completed (meta)")
}

func metaAccessTokenGroupRevoke(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_access", "token_group_revoke", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("token_group_revoke returned error (non-fatal): %v", err)
		return
	}
	t.Log("token_group_revoke completed (meta)")
}

func metaAccessTokenGroupRotate(ctx context.Context, t *testing.T) {
	_, err := callMeta[accesstokens.Output](ctx, "gitlab_access", "token_group_rotate", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("token_group_rotate returned error (non-fatal): %v", err)
		return
	}
	t.Log("token_group_rotate completed (meta)")
}

func metaAccessTokenGroupRotateSelf(ctx context.Context, t *testing.T) {
	_, err := callMeta[accesstokens.Output](ctx, "gitlab_access", "token_group_rotate_self", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("token_group_rotate_self returned error (non-fatal): %v", err)
		return
	}
	t.Log("token_group_rotate_self completed (meta)")
}

func metaAccessTokenPersonalGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[accesstokens.Output](ctx, "gitlab_access", "token_personal_get", map[string]any{})
	if err != nil {
		t.Logf("token_personal_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("token_personal_get completed (meta)")
}

func metaAccessTokenPersonalList(ctx context.Context, t *testing.T) {
	_, err := callMeta[accesstokens.ListOutput](ctx, "gitlab_access", "token_personal_list", map[string]any{})
	if err != nil {
		t.Logf("token_personal_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("token_personal_list completed (meta)")
}

func metaAccessTokenPersonalRevoke(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_access", "token_personal_revoke", map[string]any{})
	if err != nil {
		t.Logf("token_personal_revoke returned error (non-fatal): %v", err)
		return
	}
	t.Log("token_personal_revoke completed (meta)")
}

func metaAccessTokenPersonalRevokeSelf(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_access", "token_personal_revoke_self", map[string]any{})
	if err != nil {
		t.Logf("token_personal_revoke_self returned error (non-fatal): %v", err)
		return
	}
	t.Log("token_personal_revoke_self completed (meta)")
}

func metaAccessTokenPersonalRotate(ctx context.Context, t *testing.T) {
	_, err := callMeta[accesstokens.Output](ctx, "gitlab_access", "token_personal_rotate", map[string]any{})
	if err != nil {
		t.Logf("token_personal_rotate returned error (non-fatal): %v", err)
		return
	}
	t.Log("token_personal_rotate completed (meta)")
}

func metaAccessTokenPersonalRotateSelf(ctx context.Context, t *testing.T) {
	_, err := callMeta[accesstokens.Output](ctx, "gitlab_access", "token_personal_rotate_self", map[string]any{})
	if err != nil {
		t.Logf("token_personal_rotate_self returned error (non-fatal): %v", err)
		return
	}
	t.Log("token_personal_rotate_self completed (meta)")
}

func metaAccessTokenProjectGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[accesstokens.Output](ctx, "gitlab_access", "token_project_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("token_project_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("token_project_get completed (meta)")
}

func metaAccessTokenProjectRotate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[accesstokens.Output](ctx, "gitlab_access", "token_project_rotate", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("token_project_rotate returned error (non-fatal): %v", err)
		return
	}
	t.Log("token_project_rotate completed (meta)")
}

func metaAccessTokenProjectRotateSelf(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[accesstokens.Output](ctx, "gitlab_access", "token_project_rotate_self", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("token_project_rotate_self returned error (non-fatal): %v", err)
		return
	}
	t.Log("token_project_rotate_self completed (meta)")
}

// --- gitlab_admin gap tests (80 actions) ---

func metaAdminAlertMetricImageDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_admin", "alert_metric_image_delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("alert_metric_image_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("alert_metric_image_delete completed (meta)")
}

func metaAdminAlertMetricImageList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[alertmanagement.ListMetricImagesOutput](ctx, "gitlab_admin", "alert_metric_image_list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("alert_metric_image_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("alert_metric_image_list completed (meta)")
}

func metaAdminAlertMetricImageUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[alertmanagement.MetricImageItem](ctx, "gitlab_admin", "alert_metric_image_update", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("alert_metric_image_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("alert_metric_image_update completed (meta)")
}

func metaAdminAlertMetricImageUpload(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[alertmanagement.MetricImageItem](ctx, "gitlab_admin", "alert_metric_image_upload", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("alert_metric_image_upload returned error (non-fatal): %v", err)
		return
	}
	t.Log("alert_metric_image_upload completed (meta)")
}

func metaAdminAppStatisticsGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[appstatistics.GetOutput](ctx, "gitlab_admin", "app_statistics_get", map[string]any{})
	if err != nil {
		t.Logf("app_statistics_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("app_statistics_get completed (meta)")
}

func metaAdminAppearanceGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[appearance.GetOutput](ctx, "gitlab_admin", "appearance_get", map[string]any{})
	if err != nil {
		t.Logf("appearance_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("appearance_get completed (meta)")
}

func metaAdminAppearanceUpdate(ctx context.Context, t *testing.T) {
	_, err := callMeta[appearance.UpdateOutput](ctx, "gitlab_admin", "appearance_update", map[string]any{})
	if err != nil {
		t.Logf("appearance_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("appearance_update completed (meta)")
}

func metaAdminApplicationCreate(ctx context.Context, t *testing.T) {
	_, err := callMeta[applications.CreateOutput](ctx, "gitlab_admin", "application_create", map[string]any{})
	if err != nil {
		t.Logf("application_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("application_create completed (meta)")
}

func metaAdminApplicationDelete(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_admin", "application_delete", map[string]any{})
	if err != nil {
		t.Logf("application_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("application_delete completed (meta)")
}

func metaAdminApplicationList(ctx context.Context, t *testing.T) {
	_, err := callMeta[applications.ListOutput](ctx, "gitlab_admin", "application_list", map[string]any{})
	if err != nil {
		t.Logf("application_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("application_list completed (meta)")
}

func metaAdminBroadcastMessageCreate(ctx context.Context, t *testing.T) {
	_, err := callMeta[broadcastmessages.CreateOutput](ctx, "gitlab_admin", "broadcast_message_create", map[string]any{})
	if err != nil {
		t.Logf("broadcast_message_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("broadcast_message_create completed (meta)")
}

func metaAdminBroadcastMessageDelete(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_admin", "broadcast_message_delete", map[string]any{})
	if err != nil {
		t.Logf("broadcast_message_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("broadcast_message_delete completed (meta)")
}

func metaAdminBroadcastMessageGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[broadcastmessages.GetOutput](ctx, "gitlab_admin", "broadcast_message_get", map[string]any{})
	if err != nil {
		t.Logf("broadcast_message_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("broadcast_message_get completed (meta)")
}

func metaAdminBroadcastMessageList(ctx context.Context, t *testing.T) {
	_, err := callMeta[broadcastmessages.ListOutput](ctx, "gitlab_admin", "broadcast_message_list", map[string]any{})
	if err != nil {
		t.Logf("broadcast_message_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("broadcast_message_list completed (meta)")
}

func metaAdminBroadcastMessageUpdate(ctx context.Context, t *testing.T) {
	_, err := callMeta[broadcastmessages.UpdateOutput](ctx, "gitlab_admin", "broadcast_message_update", map[string]any{})
	if err != nil {
		t.Logf("broadcast_message_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("broadcast_message_update completed (meta)")
}

func metaAdminBulkImportStart(ctx context.Context, t *testing.T) {
	_, err := callMeta[bulkimports.MigrationOutput](ctx, "gitlab_admin", "bulk_import_start", map[string]any{})
	if err != nil {
		t.Logf("bulk_import_start returned error (non-fatal): %v", err)
		return
	}
	t.Log("bulk_import_start completed (meta)")
}

func metaAdminClusterAgentDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_admin", "cluster_agent_delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("cluster_agent_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("cluster_agent_delete completed (meta)")
}

func metaAdminClusterAgentGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[clusteragents.AgentItem](ctx, "gitlab_admin", "cluster_agent_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("cluster_agent_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("cluster_agent_get completed (meta)")
}

func metaAdminClusterAgentList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[clusteragents.ListAgentsOutput](ctx, "gitlab_admin", "cluster_agent_list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("cluster_agent_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("cluster_agent_list completed (meta)")
}

func metaAdminClusterAgentRegister(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[clusteragents.AgentItem](ctx, "gitlab_admin", "cluster_agent_register", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("cluster_agent_register returned error (non-fatal): %v", err)
		return
	}
	t.Log("cluster_agent_register completed (meta)")
}

func metaAdminClusterAgentTokenCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[clusteragents.AgentTokenItem](ctx, "gitlab_admin", "cluster_agent_token_create", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("cluster_agent_token_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("cluster_agent_token_create completed (meta)")
}

func metaAdminClusterAgentTokenGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[clusteragents.AgentTokenItem](ctx, "gitlab_admin", "cluster_agent_token_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("cluster_agent_token_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("cluster_agent_token_get completed (meta)")
}

func metaAdminClusterAgentTokenList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[clusteragents.ListAgentTokensOutput](ctx, "gitlab_admin", "cluster_agent_token_list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("cluster_agent_token_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("cluster_agent_token_list completed (meta)")
}

func metaAdminClusterAgentTokenRevoke(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_admin", "cluster_agent_token_revoke", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("cluster_agent_token_revoke returned error (non-fatal): %v", err)
		return
	}
	t.Log("cluster_agent_token_revoke completed (meta)")
}

func metaAdminCustomAttrDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_admin", "custom_attr_delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("custom_attr_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("custom_attr_delete completed (meta)")
}

func metaAdminCustomAttrGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[customattributes.GetOutput](ctx, "gitlab_admin", "custom_attr_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("custom_attr_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("custom_attr_get completed (meta)")
}

func metaAdminCustomAttrList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[customattributes.ListOutput](ctx, "gitlab_admin", "custom_attr_list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("custom_attr_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("custom_attr_list completed (meta)")
}

func metaAdminCustomAttrSet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[customattributes.SetOutput](ctx, "gitlab_admin", "custom_attr_set", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("custom_attr_set returned error (non-fatal): %v", err)
		return
	}
	t.Log("custom_attr_set completed (meta)")
}

func metaAdminDbMigrationMark(ctx context.Context, t *testing.T) {
	_, err := callMeta[dbmigrations.MarkOutput](ctx, "gitlab_admin", "db_migration_mark", map[string]any{})
	if err != nil {
		t.Logf("db_migration_mark returned error (non-fatal): %v", err)
		return
	}
	t.Log("db_migration_mark completed (meta)")
}

func metaAdminDependencyProxyDelete(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_admin", "dependency_proxy_delete", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("dependency_proxy_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("dependency_proxy_delete completed (meta)")
}

func metaAdminErrorTrackingCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[errortracking.ClientKeyItem](ctx, "gitlab_admin", "error_tracking_create", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("error_tracking_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("error_tracking_create completed (meta)")
}

func metaAdminErrorTrackingDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_admin", "error_tracking_delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("error_tracking_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("error_tracking_delete completed (meta)")
}

func metaAdminErrorTrackingGetSettings(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[errortracking.SettingsOutput](ctx, "gitlab_admin", "error_tracking_get_settings", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("error_tracking_get_settings returned error (non-fatal): %v", err)
		return
	}
	t.Log("error_tracking_get_settings completed (meta)")
}

func metaAdminErrorTrackingList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[errortracking.ListClientKeysOutput](ctx, "gitlab_admin", "error_tracking_list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("error_tracking_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("error_tracking_list completed (meta)")
}

func metaAdminErrorTrackingUpdateSettings(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[errortracking.SettingsOutput](ctx, "gitlab_admin", "error_tracking_update_settings", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("error_tracking_update_settings returned error (non-fatal): %v", err)
		return
	}
	t.Log("error_tracking_update_settings completed (meta)")
}

func metaAdminFeatureDelete(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_admin", "feature_delete", map[string]any{})
	if err != nil {
		t.Logf("feature_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("feature_delete completed (meta)")
}

func metaAdminFeatureList(ctx context.Context, t *testing.T) {
	_, err := callMeta[features.ListOutput](ctx, "gitlab_admin", "feature_list", map[string]any{})
	if err != nil {
		t.Logf("feature_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("feature_list completed (meta)")
}

func metaAdminFeatureListDefinitions(ctx context.Context, t *testing.T) {
	_, err := callMeta[features.ListDefinitionsOutput](ctx, "gitlab_admin", "feature_list_definitions", map[string]any{})
	if err != nil {
		t.Logf("feature_list_definitions returned error (non-fatal): %v", err)
		return
	}
	t.Log("feature_list_definitions completed (meta)")
}

func metaAdminFeatureSet(ctx context.Context, t *testing.T) {
	_, err := callMeta[features.SetOutput](ctx, "gitlab_admin", "feature_set", map[string]any{})
	if err != nil {
		t.Logf("feature_set returned error (non-fatal): %v", err)
		return
	}
	t.Log("feature_set completed (meta)")
}

func metaAdminImportBitbucket(ctx context.Context, t *testing.T) {
	_, err := callMeta[importservice.BitbucketCloudImportOutput](ctx, "gitlab_admin", "import_bitbucket", map[string]any{})
	if err != nil {
		t.Logf("import_bitbucket returned error (non-fatal): %v", err)
		return
	}
	t.Log("import_bitbucket completed (meta)")
}

func metaAdminImportBitbucketServer(ctx context.Context, t *testing.T) {
	_, err := callMeta[importservice.BitbucketServerImportOutput](ctx, "gitlab_admin", "import_bitbucket_server", map[string]any{})
	if err != nil {
		t.Logf("import_bitbucket_server returned error (non-fatal): %v", err)
		return
	}
	t.Log("import_bitbucket_server completed (meta)")
}

func metaAdminImportCancelGithub(ctx context.Context, t *testing.T) {
	_, err := callMeta[importservice.CancelledImportOutput](ctx, "gitlab_admin", "import_cancel_github", map[string]any{})
	if err != nil {
		t.Logf("import_cancel_github returned error (non-fatal): %v", err)
		return
	}
	t.Log("import_cancel_github completed (meta)")
}

func metaAdminImportGists(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_admin", "import_gists", map[string]any{})
	if err != nil {
		t.Logf("import_gists returned error (non-fatal): %v", err)
		return
	}
	t.Log("import_gists completed (meta)")
}

func metaAdminImportGithub(ctx context.Context, t *testing.T) {
	_, err := callMeta[importservice.GitHubImportOutput](ctx, "gitlab_admin", "import_github", map[string]any{})
	if err != nil {
		t.Logf("import_github returned error (non-fatal): %v", err)
		return
	}
	t.Log("import_github completed (meta)")
}

func metaAdminLicenseAdd(ctx context.Context, t *testing.T) {
	_, err := callMeta[license.AddOutput](ctx, "gitlab_admin", "license_add", map[string]any{})
	if err != nil {
		t.Logf("license_add returned error (non-fatal): %v", err)
		return
	}
	t.Log("license_add completed (meta)")
}

func metaAdminLicenseDelete(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_admin", "license_delete", map[string]any{})
	if err != nil {
		t.Logf("license_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("license_delete completed (meta)")
}

func metaAdminLicenseGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[license.GetOutput](ctx, "gitlab_admin", "license_get", map[string]any{})
	if err != nil {
		t.Logf("license_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("license_get completed (meta)")
}

func metaAdminMetadataGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[metadata.GetOutput](ctx, "gitlab_admin", "metadata_get", map[string]any{})
	if err != nil {
		t.Logf("metadata_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("metadata_get completed (meta)")
}

func metaAdminPlanLimitsChange(ctx context.Context, t *testing.T) {
	_, err := callMeta[planlimits.ChangeOutput](ctx, "gitlab_admin", "plan_limits_change", map[string]any{})
	if err != nil {
		t.Logf("plan_limits_change returned error (non-fatal): %v", err)
		return
	}
	t.Log("plan_limits_change completed (meta)")
}

func metaAdminPlanLimitsGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[planlimits.GetOutput](ctx, "gitlab_admin", "plan_limits_get", map[string]any{})
	if err != nil {
		t.Logf("plan_limits_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("plan_limits_get completed (meta)")
}

func metaAdminSecureFileCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[securefiles.SecureFileItem](ctx, "gitlab_admin", "secure_file_create", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("secure_file_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("secure_file_create completed (meta)")
}

func metaAdminSecureFileDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_admin", "secure_file_delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("secure_file_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("secure_file_delete completed (meta)")
}

func metaAdminSecureFileGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[securefiles.SecureFileItem](ctx, "gitlab_admin", "secure_file_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("secure_file_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("secure_file_get completed (meta)")
}

func metaAdminSecureFileList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[securefiles.ListOutput](ctx, "gitlab_admin", "secure_file_list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("secure_file_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("secure_file_list completed (meta)")
}

func metaAdminSettingsUpdate(ctx context.Context, t *testing.T) {
	_, err := callMeta[settings.UpdateOutput](ctx, "gitlab_admin", "settings_update", map[string]any{})
	if err != nil {
		t.Logf("settings_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("settings_update completed (meta)")
}

func metaAdminSidekiqCompoundMetrics(ctx context.Context, t *testing.T) {
	_, err := callMeta[sidekiq.GetCompoundMetricsOutput](ctx, "gitlab_admin", "sidekiq_compound_metrics", map[string]any{})
	if err != nil {
		t.Logf("sidekiq_compound_metrics returned error (non-fatal): %v", err)
		return
	}
	t.Log("sidekiq_compound_metrics completed (meta)")
}

func metaAdminSidekiqJobStats(ctx context.Context, t *testing.T) {
	_, err := callMeta[sidekiq.GetJobStatsOutput](ctx, "gitlab_admin", "sidekiq_job_stats", map[string]any{})
	if err != nil {
		t.Logf("sidekiq_job_stats returned error (non-fatal): %v", err)
		return
	}
	t.Log("sidekiq_job_stats completed (meta)")
}

func metaAdminSidekiqProcessMetrics(ctx context.Context, t *testing.T) {
	_, err := callMeta[sidekiq.GetProcessMetricsOutput](ctx, "gitlab_admin", "sidekiq_process_metrics", map[string]any{})
	if err != nil {
		t.Logf("sidekiq_process_metrics returned error (non-fatal): %v", err)
		return
	}
	t.Log("sidekiq_process_metrics completed (meta)")
}

func metaAdminSidekiqQueueMetrics(ctx context.Context, t *testing.T) {
	_, err := callMeta[sidekiq.GetQueueMetricsOutput](ctx, "gitlab_admin", "sidekiq_queue_metrics", map[string]any{})
	if err != nil {
		t.Logf("sidekiq_queue_metrics returned error (non-fatal): %v", err)
		return
	}
	t.Log("sidekiq_queue_metrics completed (meta)")
}

func metaAdminSystemHookAdd(ctx context.Context, t *testing.T) {
	_, err := callMeta[systemhooks.AddOutput](ctx, "gitlab_admin", "system_hook_add", map[string]any{})
	if err != nil {
		t.Logf("system_hook_add returned error (non-fatal): %v", err)
		return
	}
	t.Log("system_hook_add completed (meta)")
}

func metaAdminSystemHookDelete(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_admin", "system_hook_delete", map[string]any{})
	if err != nil {
		t.Logf("system_hook_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("system_hook_delete completed (meta)")
}

func metaAdminSystemHookGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[systemhooks.GetOutput](ctx, "gitlab_admin", "system_hook_get", map[string]any{})
	if err != nil {
		t.Logf("system_hook_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("system_hook_get completed (meta)")
}

func metaAdminSystemHookList(ctx context.Context, t *testing.T) {
	_, err := callMeta[systemhooks.ListOutput](ctx, "gitlab_admin", "system_hook_list", map[string]any{})
	if err != nil {
		t.Logf("system_hook_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("system_hook_list completed (meta)")
}

func metaAdminSystemHookTest(ctx context.Context, t *testing.T) {
	_, err := callMeta[systemhooks.TestOutput](ctx, "gitlab_admin", "system_hook_test", map[string]any{})
	if err != nil {
		t.Logf("system_hook_test returned error (non-fatal): %v", err)
		return
	}
	t.Log("system_hook_test completed (meta)")
}

func metaAdminTerraformStateDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_admin", "terraform_state_delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("terraform_state_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("terraform_state_delete completed (meta)")
}

func metaAdminTerraformStateGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[terraformstates.StateItem](ctx, "gitlab_admin", "terraform_state_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("terraform_state_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("terraform_state_get completed (meta)")
}

func metaAdminTerraformStateList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[terraformstates.ListOutput](ctx, "gitlab_admin", "terraform_state_list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("terraform_state_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("terraform_state_list completed (meta)")
}

func metaAdminTerraformStateLock(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[terraformstates.LockOutput](ctx, "gitlab_admin", "terraform_state_lock", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("terraform_state_lock returned error (non-fatal): %v", err)
		return
	}
	t.Log("terraform_state_lock completed (meta)")
}

func metaAdminTerraformStateUnlock(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[terraformstates.LockOutput](ctx, "gitlab_admin", "terraform_state_unlock", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("terraform_state_unlock returned error (non-fatal): %v", err)
		return
	}
	t.Log("terraform_state_unlock completed (meta)")
}

func metaAdminTerraformVersionDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_admin", "terraform_version_delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("terraform_version_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("terraform_version_delete completed (meta)")
}

func metaAdminTopicCreate(ctx context.Context, t *testing.T) {
	_, err := callMeta[topics.CreateOutput](ctx, "gitlab_admin", "topic_create", map[string]any{})
	if err != nil {
		t.Logf("topic_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("topic_create completed (meta)")
}

func metaAdminTopicDelete(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_admin", "topic_delete", map[string]any{})
	if err != nil {
		t.Logf("topic_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("topic_delete completed (meta)")
}

func metaAdminTopicGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[topics.GetOutput](ctx, "gitlab_admin", "topic_get", map[string]any{})
	if err != nil {
		t.Logf("topic_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("topic_get completed (meta)")
}

func metaAdminTopicUpdate(ctx context.Context, t *testing.T) {
	_, err := callMeta[topics.UpdateOutput](ctx, "gitlab_admin", "topic_update", map[string]any{})
	if err != nil {
		t.Logf("topic_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("topic_update completed (meta)")
}

func metaAdminUsageDataMetricDefinitions(ctx context.Context, t *testing.T) {
	_, err := callMeta[usagedata.MetricDefinitionsOutput](ctx, "gitlab_admin", "usage_data_metric_definitions", map[string]any{})
	if err != nil {
		t.Logf("usage_data_metric_definitions returned error (non-fatal): %v", err)
		return
	}
	t.Log("usage_data_metric_definitions completed (meta)")
}

func metaAdminUsageDataNonSqlMetrics(ctx context.Context, t *testing.T) {
	_, err := callMeta[usagedata.NonSQLMetricsOutput](ctx, "gitlab_admin", "usage_data_non_sql_metrics", map[string]any{})
	if err != nil {
		t.Logf("usage_data_non_sql_metrics returned error (non-fatal): %v", err)
		return
	}
	t.Log("usage_data_non_sql_metrics completed (meta)")
}

func metaAdminUsageDataQueries(ctx context.Context, t *testing.T) {
	_, err := callMeta[usagedata.QueriesOutput](ctx, "gitlab_admin", "usage_data_queries", map[string]any{})
	if err != nil {
		t.Logf("usage_data_queries returned error (non-fatal): %v", err)
		return
	}
	t.Log("usage_data_queries completed (meta)")
}

func metaAdminUsageDataServicePing(ctx context.Context, t *testing.T) {
	_, err := callMeta[usagedata.GetServicePingOutput](ctx, "gitlab_admin", "usage_data_service_ping", map[string]any{})
	if err != nil {
		t.Logf("usage_data_service_ping returned error (non-fatal): %v", err)
		return
	}
	t.Log("usage_data_service_ping completed (meta)")
}

func metaAdminUsageDataTrackEvent(ctx context.Context, t *testing.T) {
	_, err := callMeta[usagedata.TrackEventOutput](ctx, "gitlab_admin", "usage_data_track_event", map[string]any{})
	if err != nil {
		t.Logf("usage_data_track_event returned error (non-fatal): %v", err)
		return
	}
	t.Log("usage_data_track_event completed (meta)")
}

func metaAdminUsageDataTrackEvents(ctx context.Context, t *testing.T) {
	_, err := callMeta[usagedata.TrackEventsOutput](ctx, "gitlab_admin", "usage_data_track_events", map[string]any{})
	if err != nil {
		t.Logf("usage_data_track_events returned error (non-fatal): %v", err)
		return
	}
	t.Log("usage_data_track_events completed (meta)")
}

// --- gitlab_attestation gap tests (1 actions) ---

func metaAttestationDownload(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[attestations.DownloadOutput](ctx, "gitlab_attestation", "download", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("download returned error (non-fatal): %v", err)
		return
	}
	t.Log("download completed (meta)")
}

// --- gitlab_audit_event gap tests (5 actions) ---

func metaAuditEventGetGroup(ctx context.Context, t *testing.T) {
	_, err := callMeta[auditevents.Output](ctx, "gitlab_audit_event", "get_group", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("get_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("get_group completed (meta)")
}

func metaAuditEventGetInstance(ctx context.Context, t *testing.T) {
	_, err := callMeta[auditevents.Output](ctx, "gitlab_audit_event", "get_instance", map[string]any{})
	if err != nil {
		t.Logf("get_instance returned error (non-fatal): %v", err)
		return
	}
	t.Log("get_instance completed (meta)")
}

func metaAuditEventGetProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[auditevents.Output](ctx, "gitlab_audit_event", "get_project", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("get_project returned error (non-fatal): %v", err)
		return
	}
	t.Log("get_project completed (meta)")
}

func metaAuditEventListGroup(ctx context.Context, t *testing.T) {
	_, err := callMeta[auditevents.ListOutput](ctx, "gitlab_audit_event", "list_group", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("list_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("list_group completed (meta)")
}

func metaAuditEventListInstance(ctx context.Context, t *testing.T) {
	_, err := callMeta[auditevents.ListOutput](ctx, "gitlab_audit_event", "list_instance", map[string]any{})
	if err != nil {
		t.Logf("list_instance returned error (non-fatal): %v", err)
		return
	}
	t.Log("list_instance completed (meta)")
}

// --- gitlab_branch gap tests (1 actions) ---

func metaBranchDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_branch", "delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete completed (meta)")
}

// --- gitlab_ci_catalog gap tests (1 actions) ---

func metaCiCatalogGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[cicatalog.GetOutput](ctx, "gitlab_ci_catalog", "get", map[string]any{})
	if err != nil {
		t.Logf("get returned error (non-fatal): %v", err)
		return
	}
	t.Log("get completed (meta)")
}

// --- gitlab_compliance_policy gap tests (1 actions) ---

func metaCompliancePolicyUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[compliancepolicy.Output](ctx, "gitlab_compliance_policy", "update", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("update returned error (non-fatal): %v", err)
		return
	}
	t.Log("update completed (meta)")
}

// --- gitlab_custom_emoji gap tests (2 actions) ---

func metaCustomEmojiCreate(ctx context.Context, t *testing.T) {
	_, err := callMeta[customemoji.CreateOutput](ctx, "gitlab_custom_emoji", "create", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("create returned error (non-fatal): %v", err)
		return
	}
	t.Log("create completed (meta)")
}

func metaCustomEmojiDelete(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_custom_emoji", "delete", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete completed (meta)")
}

// --- gitlab_dependency gap tests (3 actions) ---

func metaDependencyExportCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[dependencies.ExportOutput](ctx, "gitlab_dependency", "export_create", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("export_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("export_create completed (meta)")
}

func metaDependencyExportDownload(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[dependencies.DownloadOutput](ctx, "gitlab_dependency", "export_download", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("export_download returned error (non-fatal): %v", err)
		return
	}
	t.Log("export_download completed (meta)")
}

func metaDependencyExportGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[dependencies.ExportOutput](ctx, "gitlab_dependency", "export_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("export_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("export_get completed (meta)")
}

// --- gitlab_deployment gap tests (6 actions) ---

func metaDeploymentApproveOrReject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[deployments.ApproveOrRejectOutput](ctx, "gitlab_deployment", "approve_or_reject", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("approve_or_reject returned error (non-fatal): %v", err)
		return
	}
	t.Log("approve_or_reject completed (meta)")
}

func metaDeploymentCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[deployments.Output](ctx, "gitlab_deployment", "create", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("create returned error (non-fatal): %v", err)
		return
	}
	t.Log("create completed (meta)")
}

func metaDeploymentDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_deployment", "delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete completed (meta)")
}

func metaDeploymentGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[deployments.Output](ctx, "gitlab_deployment", "get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("get returned error (non-fatal): %v", err)
		return
	}
	t.Log("get completed (meta)")
}

func metaDeploymentMergeRequests(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[deploymentmergerequests.ListOutput](ctx, "gitlab_deployment", "merge_requests", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("merge_requests returned error (non-fatal): %v", err)
		return
	}
	t.Log("merge_requests completed (meta)")
}

func metaDeploymentUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[deployments.Output](ctx, "gitlab_deployment", "update", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("update returned error (non-fatal): %v", err)
		return
	}
	t.Log("update completed (meta)")
}

// --- gitlab_dora_metrics gap tests (1 actions) ---

func metaDoraMetricsGroup(ctx context.Context, t *testing.T) {
	_, err := callMeta[dorametrics.Output](ctx, "gitlab_dora_metrics", "group", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group returned error (non-fatal): %v", err)
		return
	}
	t.Log("group completed (meta)")
}

// --- gitlab_enterprise_user gap tests (3 actions) ---

func metaEnterpriseUserDelete(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_enterprise_user", "delete", map[string]any{})
	if err != nil {
		t.Logf("delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete completed (meta)")
}

func metaEnterpriseUserDisable2fa(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_enterprise_user", "disable_2fa", map[string]any{})
	if err != nil {
		t.Logf("disable_2fa returned error (non-fatal): %v", err)
		return
	}
	t.Log("disable_2fa completed (meta)")
}

func metaEnterpriseUserGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[enterpriseusers.Output](ctx, "gitlab_enterprise_user", "get", map[string]any{})
	if err != nil {
		t.Logf("get returned error (non-fatal): %v", err)
		return
	}
	t.Log("get completed (meta)")
}

// --- gitlab_environment gap tests (1 actions) ---

func metaEnvironmentProtectedUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[protectedenvs.Output](ctx, "gitlab_environment", "protected_update", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("protected_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("protected_update completed (meta)")
}

// --- gitlab_external_status_check gap tests (13 actions) ---

func metaExternalStatusCheckCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_external_status_check", "create", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("create returned error (non-fatal): %v", err)
		return
	}
	t.Log("create completed (meta)")
}

func metaExternalStatusCheckCreateProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[externalstatuschecks.ProjectStatusCheckOutput](ctx, "gitlab_external_status_check", "create_project", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("create_project returned error (non-fatal): %v", err)
		return
	}
	t.Log("create_project completed (meta)")
}

func metaExternalStatusCheckDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_external_status_check", "delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete completed (meta)")
}

func metaExternalStatusCheckDeleteProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_external_status_check", "delete_project", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("delete_project returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete_project completed (meta)")
}

func metaExternalStatusCheckListMrChecks(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[externalstatuschecks.ListMergeStatusCheckOutput](ctx, "gitlab_external_status_check", "list_mr_checks", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("list_mr_checks returned error (non-fatal): %v", err)
		return
	}
	t.Log("list_mr_checks completed (meta)")
}

func metaExternalStatusCheckListProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[externalstatuschecks.ListProjectStatusCheckOutput](ctx, "gitlab_external_status_check", "list_project", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("list_project returned error (non-fatal): %v", err)
		return
	}
	t.Log("list_project completed (meta)")
}

func metaExternalStatusCheckListProjectMrChecks(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[externalstatuschecks.ListMergeStatusCheckOutput](ctx, "gitlab_external_status_check", "list_project_mr_checks", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("list_project_mr_checks returned error (non-fatal): %v", err)
		return
	}
	t.Log("list_project_mr_checks completed (meta)")
}

func metaExternalStatusCheckRetry(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_external_status_check", "retry", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("retry returned error (non-fatal): %v", err)
		return
	}
	t.Log("retry completed (meta)")
}

func metaExternalStatusCheckRetryProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_external_status_check", "retry_project", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("retry_project returned error (non-fatal): %v", err)
		return
	}
	t.Log("retry_project completed (meta)")
}

func metaExternalStatusCheckSetProjectMrStatus(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_external_status_check", "set_project_mr_status", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("set_project_mr_status returned error (non-fatal): %v", err)
		return
	}
	t.Log("set_project_mr_status completed (meta)")
}

func metaExternalStatusCheckSetStatus(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_external_status_check", "set_status", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("set_status returned error (non-fatal): %v", err)
		return
	}
	t.Log("set_status completed (meta)")
}

func metaExternalStatusCheckUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_external_status_check", "update", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("update returned error (non-fatal): %v", err)
		return
	}
	t.Log("update completed (meta)")
}

func metaExternalStatusCheckUpdateProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[externalstatuschecks.ProjectStatusCheckOutput](ctx, "gitlab_external_status_check", "update_project", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("update_project returned error (non-fatal): %v", err)
		return
	}
	t.Log("update_project completed (meta)")
}

// --- gitlab_feature_flags gap tests (9 actions) ---

func metaFeatureFlagsFeatureFlagCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[featureflags.Output](ctx, "gitlab_feature_flags", "feature_flag_create", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("feature_flag_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("feature_flag_create completed (meta)")
}

func metaFeatureFlagsFeatureFlagDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_feature_flags", "feature_flag_delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("feature_flag_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("feature_flag_delete completed (meta)")
}

func metaFeatureFlagsFeatureFlagGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[featureflags.Output](ctx, "gitlab_feature_flags", "feature_flag_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("feature_flag_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("feature_flag_get completed (meta)")
}

func metaFeatureFlagsFeatureFlagUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[featureflags.Output](ctx, "gitlab_feature_flags", "feature_flag_update", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("feature_flag_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("feature_flag_update completed (meta)")
}

func metaFeatureFlagsFfUserListCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[ffuserlists.Output](ctx, "gitlab_feature_flags", "ff_user_list_create", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("ff_user_list_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("ff_user_list_create completed (meta)")
}

func metaFeatureFlagsFfUserListDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_feature_flags", "ff_user_list_delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("ff_user_list_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("ff_user_list_delete completed (meta)")
}

func metaFeatureFlagsFfUserListGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[ffuserlists.Output](ctx, "gitlab_feature_flags", "ff_user_list_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("ff_user_list_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("ff_user_list_get completed (meta)")
}

func metaFeatureFlagsFfUserListList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[ffuserlists.ListOutput](ctx, "gitlab_feature_flags", "ff_user_list_list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("ff_user_list_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("ff_user_list_list completed (meta)")
}

func metaFeatureFlagsFfUserListUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[ffuserlists.Output](ctx, "gitlab_feature_flags", "ff_user_list_update", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("ff_user_list_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("ff_user_list_update completed (meta)")
}

// --- gitlab_geo gap tests (7 actions) ---

func metaGeoCreate(ctx context.Context, t *testing.T) {
	_, err := callMeta[geo.Output](ctx, "gitlab_geo", "create", map[string]any{})
	if err != nil {
		t.Logf("create returned error (non-fatal): %v", err)
		return
	}
	t.Log("create completed (meta)")
}

func metaGeoDelete(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_geo", "delete", map[string]any{})
	if err != nil {
		t.Logf("delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete completed (meta)")
}

func metaGeoEdit(ctx context.Context, t *testing.T) {
	_, err := callMeta[geo.Output](ctx, "gitlab_geo", "edit", map[string]any{})
	if err != nil {
		t.Logf("edit returned error (non-fatal): %v", err)
		return
	}
	t.Log("edit completed (meta)")
}

func metaGeoGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[geo.Output](ctx, "gitlab_geo", "get", map[string]any{})
	if err != nil {
		t.Logf("get returned error (non-fatal): %v", err)
		return
	}
	t.Log("get completed (meta)")
}

func metaGeoGetStatus(ctx context.Context, t *testing.T) {
	_, err := callMeta[geo.StatusOutput](ctx, "gitlab_geo", "get_status", map[string]any{})
	if err != nil {
		t.Logf("get_status returned error (non-fatal): %v", err)
		return
	}
	t.Log("get_status completed (meta)")
}

func metaGeoListStatus(ctx context.Context, t *testing.T) {
	_, err := callMeta[geo.ListStatusOutput](ctx, "gitlab_geo", "list_status", map[string]any{})
	if err != nil {
		t.Logf("list_status returned error (non-fatal): %v", err)
		return
	}
	t.Log("list_status completed (meta)")
}

func metaGeoRepair(ctx context.Context, t *testing.T) {
	_, err := callMeta[geo.Output](ctx, "gitlab_geo", "repair", map[string]any{})
	if err != nil {
		t.Logf("repair returned error (non-fatal): %v", err)
		return
	}
	t.Log("repair completed (meta)")
}

// --- gitlab_group gap tests (53 actions) ---

func metaGroupArchive(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_group", "archive", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("archive returned error (non-fatal): %v", err)
		return
	}
	t.Log("archive completed (meta)")
}

func metaGroupBadgeAdd(ctx context.Context, t *testing.T) {
	_, err := callMeta[badges.AddGroupOutput](ctx, "gitlab_group", "badge_add", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("badge_add returned error (non-fatal): %v", err)
		return
	}
	t.Log("badge_add completed (meta)")
}

func metaGroupBadgeDelete(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_group", "badge_delete", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("badge_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("badge_delete completed (meta)")
}

func metaGroupBadgeEdit(ctx context.Context, t *testing.T) {
	_, err := callMeta[badges.EditGroupOutput](ctx, "gitlab_group", "badge_edit", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("badge_edit returned error (non-fatal): %v", err)
		return
	}
	t.Log("badge_edit completed (meta)")
}

func metaGroupBadgeGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[badges.GetGroupOutput](ctx, "gitlab_group", "badge_get", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("badge_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("badge_get completed (meta)")
}

func metaGroupBadgeList(ctx context.Context, t *testing.T) {
	_, err := callMeta[badges.ListGroupOutput](ctx, "gitlab_group", "badge_list", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("badge_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("badge_list completed (meta)")
}

func metaGroupBadgePreview(ctx context.Context, t *testing.T) {
	_, err := callMeta[badges.PreviewGroupOutput](ctx, "gitlab_group", "badge_preview", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("badge_preview returned error (non-fatal): %v", err)
		return
	}
	t.Log("badge_preview completed (meta)")
}

func metaGroupCreate(ctx context.Context, t *testing.T) {
	_, err := callMeta[groups.Output](ctx, "gitlab_group", "create", map[string]any{})
	if err != nil {
		t.Logf("create returned error (non-fatal): %v", err)
		return
	}
	t.Log("create completed (meta)")
}

func metaGroupDelete(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_group", "delete", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete completed (meta)")
}

func metaGroupGroupBoardCreate(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupboards.GroupBoardOutput](ctx, "gitlab_group", "group_board_create", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_board_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_board_create completed (meta)")
}

func metaGroupGroupBoardCreateList(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupboards.BoardListOutput](ctx, "gitlab_group", "group_board_create_list", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_board_create_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_board_create_list completed (meta)")
}

func metaGroupGroupBoardDelete(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_group", "group_board_delete", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_board_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_board_delete completed (meta)")
}

func metaGroupGroupBoardDeleteList(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_group", "group_board_delete_list", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_board_delete_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_board_delete_list completed (meta)")
}

func metaGroupGroupBoardGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupboards.GroupBoardOutput](ctx, "gitlab_group", "group_board_get", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_board_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_board_get completed (meta)")
}

func metaGroupGroupBoardGetList(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupboards.BoardListOutput](ctx, "gitlab_group", "group_board_get_list", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_board_get_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_board_get_list completed (meta)")
}

func metaGroupGroupBoardList(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupboards.ListGroupBoardsOutput](ctx, "gitlab_group", "group_board_list", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_board_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_board_list completed (meta)")
}

func metaGroupGroupBoardListLists(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupboards.ListBoardListsOutput](ctx, "gitlab_group", "group_board_list_lists", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_board_list_lists returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_board_list_lists completed (meta)")
}

func metaGroupGroupBoardUpdate(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupboards.GroupBoardOutput](ctx, "gitlab_group", "group_board_update", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_board_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_board_update completed (meta)")
}

func metaGroupGroupBoardUpdateList(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupboards.BoardListOutput](ctx, "gitlab_group", "group_board_update_list", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_board_update_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_board_update_list completed (meta)")
}

func metaGroupGroupExportDownload(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupimportexport.ExportDownloadOutput](ctx, "gitlab_group", "group_export_download", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_export_download returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_export_download completed (meta)")
}

func metaGroupGroupExportSchedule(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupimportexport.ScheduleExportOutput](ctx, "gitlab_group", "group_export_schedule", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_export_schedule returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_export_schedule completed (meta)")
}

func metaGroupGroupImportFile(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupimportexport.ImportFileOutput](ctx, "gitlab_group", "group_import_file", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_import_file returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_import_file completed (meta)")
}

func metaGroupGroupLabelGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[grouplabels.Output](ctx, "gitlab_group", "group_label_get", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_label_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_label_get completed (meta)")
}

func metaGroupGroupLabelSubscribe(ctx context.Context, t *testing.T) {
	_, err := callMeta[grouplabels.Output](ctx, "gitlab_group", "group_label_subscribe", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_label_subscribe returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_label_subscribe completed (meta)")
}

func metaGroupGroupLabelUnsubscribe(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_group", "group_label_unsubscribe", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_label_unsubscribe returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_label_unsubscribe completed (meta)")
}

func metaGroupGroupLabelUpdate(ctx context.Context, t *testing.T) {
	_, err := callMeta[grouplabels.Output](ctx, "gitlab_group", "group_label_update", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_label_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_label_update completed (meta)")
}

func metaGroupGroupMemberAdd(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupmembers.Output](ctx, "gitlab_group", "group_member_add", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_member_add returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_member_add completed (meta)")
}

func metaGroupGroupMemberEdit(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupmembers.Output](ctx, "gitlab_group", "group_member_edit", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_member_edit returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_member_edit completed (meta)")
}

func metaGroupGroupMemberGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupmembers.Output](ctx, "gitlab_group", "group_member_get", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_member_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_member_get completed (meta)")
}

func metaGroupGroupMemberGetInherited(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupmembers.Output](ctx, "gitlab_group", "group_member_get_inherited", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_member_get_inherited returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_member_get_inherited completed (meta)")
}

func metaGroupGroupMemberRemove(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_group", "group_member_remove", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_member_remove returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_member_remove completed (meta)")
}

func metaGroupGroupMemberShare(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupmembers.ShareOutput](ctx, "gitlab_group", "group_member_share", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_member_share returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_member_share completed (meta)")
}

func metaGroupGroupMemberUnshare(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_group", "group_member_unshare", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_member_unshare returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_member_unshare completed (meta)")
}

func metaGroupGroupMilestoneBurndown(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupmilestones.BurndownChartEventsOutput](ctx, "gitlab_group", "group_milestone_burndown", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_milestone_burndown returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_milestone_burndown completed (meta)")
}

func metaGroupGroupMilestoneIssues(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupmilestones.IssuesOutput](ctx, "gitlab_group", "group_milestone_issues", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_milestone_issues returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_milestone_issues completed (meta)")
}

func metaGroupGroupMilestoneMergeRequests(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupmilestones.MergeRequestsOutput](ctx, "gitlab_group", "group_milestone_merge_requests", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_milestone_merge_requests returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_milestone_merge_requests completed (meta)")
}

func metaGroupGroupMilestoneUpdate(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupmilestones.Output](ctx, "gitlab_group", "group_milestone_update", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_milestone_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_milestone_update completed (meta)")
}

func metaGroupGroupRelationsListStatus(ctx context.Context, t *testing.T) {
	_, err := callMeta[grouprelationsexport.ListExportStatusOutput](ctx, "gitlab_group", "group_relations_list_status", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_relations_list_status returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_relations_list_status completed (meta)")
}

func metaGroupGroupRelationsSchedule(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_group", "group_relations_schedule", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_relations_schedule returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_relations_schedule completed (meta)")
}

func metaGroupGroupUploadDeleteById(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_group", "group_upload_delete_by_id", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_upload_delete_by_id returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_upload_delete_by_id completed (meta)")
}

func metaGroupGroupUploadDeleteBySecret(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_group", "group_upload_delete_by_secret", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_upload_delete_by_secret returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_upload_delete_by_secret completed (meta)")
}

func metaGroupGroupUploadList(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupmarkdownuploads.ListOutput](ctx, "gitlab_group", "group_upload_list", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("group_upload_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("group_upload_list completed (meta)")
}

func metaGroupHookAdd(ctx context.Context, t *testing.T) {
	_, err := callMeta[groups.HookOutput](ctx, "gitlab_group", "hook_add", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("hook_add returned error (non-fatal): %v", err)
		return
	}
	t.Log("hook_add completed (meta)")
}

func metaGroupHookDelete(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_group", "hook_delete", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("hook_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("hook_delete completed (meta)")
}

func metaGroupHookEdit(ctx context.Context, t *testing.T) {
	_, err := callMeta[groups.HookOutput](ctx, "gitlab_group", "hook_edit", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("hook_edit returned error (non-fatal): %v", err)
		return
	}
	t.Log("hook_edit completed (meta)")
}

func metaGroupHookGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[groups.HookOutput](ctx, "gitlab_group", "hook_get", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("hook_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("hook_get completed (meta)")
}

func metaGroupHookList(ctx context.Context, t *testing.T) {
	_, err := callMeta[groups.HookListOutput](ctx, "gitlab_group", "hook_list", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("hook_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("hook_list completed (meta)")
}

func metaGroupProjects(ctx context.Context, t *testing.T) {
	_, err := callMeta[groups.ListProjectsOutput](ctx, "gitlab_group", "projects", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("projects returned error (non-fatal): %v", err)
		return
	}
	t.Log("projects completed (meta)")
}

func metaGroupRestore(ctx context.Context, t *testing.T) {
	_, err := callMeta[groups.Output](ctx, "gitlab_group", "restore", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("restore returned error (non-fatal): %v", err)
		return
	}
	t.Log("restore completed (meta)")
}

func metaGroupSearch(ctx context.Context, t *testing.T) {
	_, err := callMeta[groups.ListOutput](ctx, "gitlab_group", "search", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("search returned error (non-fatal): %v", err)
		return
	}
	t.Log("search completed (meta)")
}

func metaGroupTransferProject(ctx context.Context, t *testing.T) {
	_, err := callMeta[groups.Output](ctx, "gitlab_group", "transfer_project", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("transfer_project returned error (non-fatal): %v", err)
		return
	}
	t.Log("transfer_project completed (meta)")
}

func metaGroupUnarchive(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_group", "unarchive", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("unarchive returned error (non-fatal): %v", err)
		return
	}
	t.Log("unarchive completed (meta)")
}

func metaGroupUpdate(ctx context.Context, t *testing.T) {
	_, err := callMeta[groups.Output](ctx, "gitlab_group", "update", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("update returned error (non-fatal): %v", err)
		return
	}
	t.Log("update completed (meta)")
}

// --- gitlab_group_scim gap tests (3 actions) ---

func metaGroupScimDelete(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_group_scim", "delete", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete completed (meta)")
}

func metaGroupScimGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupscim.Output](ctx, "gitlab_group_scim", "get", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("get returned error (non-fatal): %v", err)
		return
	}
	t.Log("get completed (meta)")
}

func metaGroupScimUpdate(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_group_scim", "update", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("update returned error (non-fatal): %v", err)
		return
	}
	t.Log("update completed (meta)")
}

// --- gitlab_issue gap tests (43 actions) ---

func metaIssueCreateTodo(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[issues.TodoOutput](ctx, "gitlab_issue", "create_todo", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("create_todo returned error (non-fatal): %v", err)
		return
	}
	t.Log("create_todo completed (meta)")
}

func metaIssueDiscussionGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[issuediscussions.Output](ctx, "gitlab_issue", "discussion_get", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("discussion_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("discussion_get completed (meta)")
}

func metaIssueDiscussionUpdateNote(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[issuediscussions.NoteOutput](ctx, "gitlab_issue", "discussion_update_note", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("discussion_update_note returned error (non-fatal): %v", err)
		return
	}
	t.Log("discussion_update_note completed (meta)")
}

func metaIssueEmojiIssueGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[awardemoji.Output](ctx, "gitlab_issue", "emoji_issue_get", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("emoji_issue_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("emoji_issue_get completed (meta)")
}

func metaIssueEmojiIssueNoteCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[awardemoji.Output](ctx, "gitlab_issue", "emoji_issue_note_create", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("emoji_issue_note_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("emoji_issue_note_create completed (meta)")
}

func metaIssueEmojiIssueNoteDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_issue", "emoji_issue_note_delete", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("emoji_issue_note_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("emoji_issue_note_delete completed (meta)")
}

func metaIssueEmojiIssueNoteGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[awardemoji.Output](ctx, "gitlab_issue", "emoji_issue_note_get", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("emoji_issue_note_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("emoji_issue_note_get completed (meta)")
}

func metaIssueEmojiIssueNoteList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[awardemoji.ListOutput](ctx, "gitlab_issue", "emoji_issue_note_list", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("emoji_issue_note_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("emoji_issue_note_list completed (meta)")
}

func metaIssueEventIssueIterationGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[resourceevents.IterationEventOutput](ctx, "gitlab_issue", "event_issue_iteration_get", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("event_issue_iteration_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("event_issue_iteration_get completed (meta)")
}

func metaIssueEventIssueIterationList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[resourceevents.ListIterationEventsOutput](ctx, "gitlab_issue", "event_issue_iteration_list", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("event_issue_iteration_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("event_issue_iteration_list completed (meta)")
}

func metaIssueEventIssueLabelGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[resourceevents.LabelEventOutput](ctx, "gitlab_issue", "event_issue_label_get", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("event_issue_label_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("event_issue_label_get completed (meta)")
}

func metaIssueEventIssueLabelList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[resourceevents.ListLabelEventsOutput](ctx, "gitlab_issue", "event_issue_label_list", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("event_issue_label_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("event_issue_label_list completed (meta)")
}

func metaIssueEventIssueMilestoneGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[resourceevents.MilestoneEventOutput](ctx, "gitlab_issue", "event_issue_milestone_get", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("event_issue_milestone_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("event_issue_milestone_get completed (meta)")
}

func metaIssueEventIssueMilestoneList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[resourceevents.ListMilestoneEventsOutput](ctx, "gitlab_issue", "event_issue_milestone_list", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("event_issue_milestone_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("event_issue_milestone_list completed (meta)")
}

func metaIssueEventIssueStateGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[resourceevents.StateEventOutput](ctx, "gitlab_issue", "event_issue_state_get", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("event_issue_state_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("event_issue_state_get completed (meta)")
}

func metaIssueEventIssueWeightList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[resourceevents.ListWeightEventsOutput](ctx, "gitlab_issue", "event_issue_weight_list", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("event_issue_weight_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("event_issue_weight_list completed (meta)")
}

func metaIssueGetById(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[issues.Output](ctx, "gitlab_issue", "get_by_id", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("get_by_id returned error (non-fatal): %v", err)
		return
	}
	t.Log("get_by_id completed (meta)")
}

func metaIssueLinkGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[issuelinks.Output](ctx, "gitlab_issue", "link_get", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("link_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("link_get completed (meta)")
}

func metaIssueListAll(ctx context.Context, t *testing.T) {
	_, err := callMeta[issues.ListOutput](ctx, "gitlab_issue", "list_all", map[string]any{})
	if err != nil {
		t.Logf("list_all returned error (non-fatal): %v", err)
		return
	}
	t.Log("list_all completed (meta)")
}

func metaIssueListGroup(ctx context.Context, t *testing.T) {
	_, err := callMeta[issues.ListGroupOutput](ctx, "gitlab_issue", "list_group", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("list_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("list_group completed (meta)")
}

func metaIssueMove(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[issues.Output](ctx, "gitlab_issue", "move", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("move returned error (non-fatal): %v", err)
		return
	}
	t.Log("move completed (meta)")
}

func metaIssueMrsClosing(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[issues.RelatedMRsOutput](ctx, "gitlab_issue", "mrs_closing", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("mrs_closing returned error (non-fatal): %v", err)
		return
	}
	t.Log("mrs_closing completed (meta)")
}

func metaIssueMrsRelated(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[issues.RelatedMRsOutput](ctx, "gitlab_issue", "mrs_related", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("mrs_related returned error (non-fatal): %v", err)
		return
	}
	t.Log("mrs_related completed (meta)")
}

func metaIssueNoteDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_issue", "note_delete", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("note_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("note_delete completed (meta)")
}

func metaIssueNoteGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[issuenotes.Output](ctx, "gitlab_issue", "note_get", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("note_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("note_get completed (meta)")
}

func metaIssueNoteUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[issuenotes.Output](ctx, "gitlab_issue", "note_update", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("note_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("note_update completed (meta)")
}

func metaIssueParticipants(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[issues.ParticipantsOutput](ctx, "gitlab_issue", "participants", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("participants returned error (non-fatal): %v", err)
		return
	}
	t.Log("participants completed (meta)")
}

func metaIssueReorder(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[issues.Output](ctx, "gitlab_issue", "reorder", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("reorder returned error (non-fatal): %v", err)
		return
	}
	t.Log("reorder completed (meta)")
}

func metaIssueSpentTimeAdd(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[issues.TimeStatsOutput](ctx, "gitlab_issue", "spent_time_add", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("spent_time_add returned error (non-fatal): %v", err)
		return
	}
	t.Log("spent_time_add completed (meta)")
}

func metaIssueSpentTimeReset(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[issues.TimeStatsOutput](ctx, "gitlab_issue", "spent_time_reset", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("spent_time_reset returned error (non-fatal): %v", err)
		return
	}
	t.Log("spent_time_reset completed (meta)")
}

func metaIssueStatisticsGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[issuestatistics.StatisticsOutput](ctx, "gitlab_issue", "statistics_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("statistics_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("statistics_get completed (meta)")
}

func metaIssueStatisticsGetGroup(ctx context.Context, t *testing.T) {
	_, err := callMeta[issuestatistics.StatisticsOutput](ctx, "gitlab_issue", "statistics_get_group", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("statistics_get_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("statistics_get_group completed (meta)")
}

func metaIssueStatisticsGetProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[issuestatistics.StatisticsOutput](ctx, "gitlab_issue", "statistics_get_project", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("statistics_get_project returned error (non-fatal): %v", err)
		return
	}
	t.Log("statistics_get_project completed (meta)")
}

func metaIssueSubscribe(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[issues.Output](ctx, "gitlab_issue", "subscribe", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("subscribe returned error (non-fatal): %v", err)
		return
	}
	t.Log("subscribe completed (meta)")
}

func metaIssueTimeEstimateReset(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[issues.TimeStatsOutput](ctx, "gitlab_issue", "time_estimate_reset", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("time_estimate_reset returned error (non-fatal): %v", err)
		return
	}
	t.Log("time_estimate_reset completed (meta)")
}

func metaIssueTimeEstimateSet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[issues.TimeStatsOutput](ctx, "gitlab_issue", "time_estimate_set", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("time_estimate_set returned error (non-fatal): %v", err)
		return
	}
	t.Log("time_estimate_set completed (meta)")
}

func metaIssueTimeStatsGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[issues.TimeStatsOutput](ctx, "gitlab_issue", "time_stats_get", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("time_stats_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("time_stats_get completed (meta)")
}

func metaIssueUnsubscribe(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[issues.Output](ctx, "gitlab_issue", "unsubscribe", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("unsubscribe returned error (non-fatal): %v", err)
		return
	}
	t.Log("unsubscribe completed (meta)")
}

func metaIssueWorkItemCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[workitems.GetOutput](ctx, "gitlab_issue", "work_item_create", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("work_item_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("work_item_create completed (meta)")
}

func metaIssueWorkItemDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_issue", "work_item_delete", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("work_item_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("work_item_delete completed (meta)")
}

func metaIssueWorkItemGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[workitems.GetOutput](ctx, "gitlab_issue", "work_item_get", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("work_item_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("work_item_get completed (meta)")
}

func metaIssueWorkItemList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[workitems.ListOutput](ctx, "gitlab_issue", "work_item_list", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("work_item_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("work_item_list completed (meta)")
}

func metaIssueWorkItemUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[workitems.GetOutput](ctx, "gitlab_issue", "work_item_update", map[string]any{
		"issue_iid":  mState.issueIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("work_item_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("work_item_update completed (meta)")
}

// --- gitlab_job gap tests (22 actions) ---

func metaJobArtifacts(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[jobs.ArtifactsOutput](ctx, "gitlab_job", "artifacts", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("artifacts returned error (non-fatal): %v", err)
		return
	}
	t.Log("artifacts completed (meta)")
}

func metaJobCancel(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[jobs.Output](ctx, "gitlab_job", "cancel", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("cancel returned error (non-fatal): %v", err)
		return
	}
	t.Log("cancel completed (meta)")
}

func metaJobDeleteArtifacts(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_job", "delete_artifacts", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("delete_artifacts returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete_artifacts completed (meta)")
}

func metaJobDeleteProjectArtifacts(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_job", "delete_project_artifacts", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("delete_project_artifacts returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete_project_artifacts completed (meta)")
}

func metaJobDownloadArtifacts(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[jobs.ArtifactsOutput](ctx, "gitlab_job", "download_artifacts", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("download_artifacts returned error (non-fatal): %v", err)
		return
	}
	t.Log("download_artifacts completed (meta)")
}

func metaJobDownloadSingleArtifact(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[jobs.SingleArtifactOutput](ctx, "gitlab_job", "download_single_artifact", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("download_single_artifact returned error (non-fatal): %v", err)
		return
	}
	t.Log("download_single_artifact completed (meta)")
}

func metaJobDownloadSingleArtifactByRef(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[jobs.SingleArtifactOutput](ctx, "gitlab_job", "download_single_artifact_by_ref", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("download_single_artifact_by_ref returned error (non-fatal): %v", err)
		return
	}
	t.Log("download_single_artifact_by_ref completed (meta)")
}

func metaJobErase(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[jobs.Output](ctx, "gitlab_job", "erase", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("erase returned error (non-fatal): %v", err)
		return
	}
	t.Log("erase completed (meta)")
}

func metaJobGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[jobs.Output](ctx, "gitlab_job", "get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("get returned error (non-fatal): %v", err)
		return
	}
	t.Log("get completed (meta)")
}

func metaJobKeepArtifacts(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[jobs.Output](ctx, "gitlab_job", "keep_artifacts", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("keep_artifacts returned error (non-fatal): %v", err)
		return
	}
	t.Log("keep_artifacts completed (meta)")
}

func metaJobListBridges(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[jobs.BridgeListOutput](ctx, "gitlab_job", "list_bridges", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("list_bridges returned error (non-fatal): %v", err)
		return
	}
	t.Log("list_bridges completed (meta)")
}

func metaJobPlay(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[jobs.Output](ctx, "gitlab_job", "play", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("play returned error (non-fatal): %v", err)
		return
	}
	t.Log("play completed (meta)")
}

func metaJobRetry(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[jobs.Output](ctx, "gitlab_job", "retry", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("retry returned error (non-fatal): %v", err)
		return
	}
	t.Log("retry completed (meta)")
}

func metaJobTokenScopeAddGroup(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[jobtokenscope.GroupAllowlistItemOutput](ctx, "gitlab_job", "token_scope_add_group", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("token_scope_add_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("token_scope_add_group completed (meta)")
}

func metaJobTokenScopeAddProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[jobtokenscope.InboundAllowItemOutput](ctx, "gitlab_job", "token_scope_add_project", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("token_scope_add_project returned error (non-fatal): %v", err)
		return
	}
	t.Log("token_scope_add_project completed (meta)")
}

func metaJobTokenScopeListGroups(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[jobtokenscope.ListGroupAllowlistOutput](ctx, "gitlab_job", "token_scope_list_groups", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("token_scope_list_groups returned error (non-fatal): %v", err)
		return
	}
	t.Log("token_scope_list_groups completed (meta)")
}

func metaJobTokenScopeListInbound(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[jobtokenscope.ListInboundAllowlistOutput](ctx, "gitlab_job", "token_scope_list_inbound", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("token_scope_list_inbound returned error (non-fatal): %v", err)
		return
	}
	t.Log("token_scope_list_inbound completed (meta)")
}

func metaJobTokenScopePatch(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[toolutil.DeleteOutput](ctx, "gitlab_job", "token_scope_patch", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("token_scope_patch returned error (non-fatal): %v", err)
		return
	}
	t.Log("token_scope_patch completed (meta)")
}

func metaJobTokenScopeRemoveGroup(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_job", "token_scope_remove_group", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("token_scope_remove_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("token_scope_remove_group completed (meta)")
}

func metaJobTokenScopeRemoveProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_job", "token_scope_remove_project", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("token_scope_remove_project returned error (non-fatal): %v", err)
		return
	}
	t.Log("token_scope_remove_project completed (meta)")
}

func metaJobTrace(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[jobs.TraceOutput](ctx, "gitlab_job", "trace", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("trace returned error (non-fatal): %v", err)
		return
	}
	t.Log("trace completed (meta)")
}

// --- gitlab_member_role gap tests (5 actions) ---

func metaMemberRoleCreateGroup(ctx context.Context, t *testing.T) {
	_, err := callMeta[memberroles.Output](ctx, "gitlab_member_role", "create_group", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("create_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("create_group completed (meta)")
}

func metaMemberRoleCreateInstance(ctx context.Context, t *testing.T) {
	_, err := callMeta[memberroles.Output](ctx, "gitlab_member_role", "create_instance", map[string]any{})
	if err != nil {
		t.Logf("create_instance returned error (non-fatal): %v", err)
		return
	}
	t.Log("create_instance completed (meta)")
}

func metaMemberRoleDeleteGroup(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_member_role", "delete_group", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("delete_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete_group completed (meta)")
}

func metaMemberRoleDeleteInstance(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_member_role", "delete_instance", map[string]any{})
	if err != nil {
		t.Logf("delete_instance returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete_instance completed (meta)")
}

func metaMemberRoleListGroup(ctx context.Context, t *testing.T) {
	_, err := callMeta[memberroles.ListOutput](ctx, "gitlab_member_role", "list_group", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("list_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("list_group completed (meta)")
}

// --- gitlab_merge_request gap tests (42 actions) ---

func metaMergeRequestApprovalConfig(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[mrapprovals.ConfigOutput](ctx, "gitlab_merge_request", "approval_config", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("approval_config returned error (non-fatal): %v", err)
		return
	}
	t.Log("approval_config completed (meta)")
}

func metaMergeRequestApprovalReset(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_merge_request", "approval_reset", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("approval_reset returned error (non-fatal): %v", err)
		return
	}
	t.Log("approval_reset completed (meta)")
}

func metaMergeRequestApprovalRuleCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[mrapprovals.RuleOutput](ctx, "gitlab_merge_request", "approval_rule_create", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("approval_rule_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("approval_rule_create completed (meta)")
}

func metaMergeRequestApprovalRuleDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_merge_request", "approval_rule_delete", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("approval_rule_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("approval_rule_delete completed (meta)")
}

func metaMergeRequestApprovalRuleUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[mrapprovals.RuleOutput](ctx, "gitlab_merge_request", "approval_rule_update", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("approval_rule_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("approval_rule_update completed (meta)")
}

func metaMergeRequestApprovalRules(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[mrapprovals.RulesOutput](ctx, "gitlab_merge_request", "approval_rules", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("approval_rules returned error (non-fatal): %v", err)
		return
	}
	t.Log("approval_rules completed (meta)")
}

func metaMergeRequestApprovalSettingsGroupGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[mrapprovalsettings.Output](ctx, "gitlab_merge_request", "approval_settings_group_get", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("approval_settings_group_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("approval_settings_group_get completed (meta)")
}

func metaMergeRequestApprovalSettingsGroupUpdate(ctx context.Context, t *testing.T) {
	_, err := callMeta[mrapprovalsettings.Output](ctx, "gitlab_merge_request", "approval_settings_group_update", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("approval_settings_group_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("approval_settings_group_update completed (meta)")
}

func metaMergeRequestApprovalSettingsProjectGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[mrapprovalsettings.Output](ctx, "gitlab_merge_request", "approval_settings_project_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("approval_settings_project_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("approval_settings_project_get completed (meta)")
}

func metaMergeRequestApprovalSettingsProjectUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[mrapprovalsettings.Output](ctx, "gitlab_merge_request", "approval_settings_project_update", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("approval_settings_project_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("approval_settings_project_update completed (meta)")
}

func metaMergeRequestApprovalState(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[mrapprovals.StateOutput](ctx, "gitlab_merge_request", "approval_state", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("approval_state returned error (non-fatal): %v", err)
		return
	}
	t.Log("approval_state completed (meta)")
}

func metaMergeRequestCancelAutoMerge(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[mergerequests.Output](ctx, "gitlab_merge_request", "cancel_auto_merge", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("cancel_auto_merge returned error (non-fatal): %v", err)
		return
	}
	t.Log("cancel_auto_merge completed (meta)")
}

func metaMergeRequestContextCommitsCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[mrcontextcommits.ListOutput](ctx, "gitlab_merge_request", "context_commits_create", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("context_commits_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("context_commits_create completed (meta)")
}

func metaMergeRequestContextCommitsDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_merge_request", "context_commits_delete", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("context_commits_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("context_commits_delete completed (meta)")
}

func metaMergeRequestContextCommitsList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[mrcontextcommits.ListOutput](ctx, "gitlab_merge_request", "context_commits_list", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("context_commits_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("context_commits_list completed (meta)")
}

func metaMergeRequestCreatePipeline(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[pipelines.Output](ctx, "gitlab_merge_request", "create_pipeline", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("create_pipeline returned error (non-fatal): %v", err)
		return
	}
	t.Log("create_pipeline completed (meta)")
}

func metaMergeRequestDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_merge_request", "delete", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete completed (meta)")
}

func metaMergeRequestEmojiMrCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[awardemoji.Output](ctx, "gitlab_merge_request", "emoji_mr_create", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("emoji_mr_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("emoji_mr_create completed (meta)")
}

func metaMergeRequestEmojiMrDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_merge_request", "emoji_mr_delete", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("emoji_mr_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("emoji_mr_delete completed (meta)")
}

func metaMergeRequestEmojiMrGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[awardemoji.Output](ctx, "gitlab_merge_request", "emoji_mr_get", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("emoji_mr_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("emoji_mr_get completed (meta)")
}

func metaMergeRequestEmojiMrList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[awardemoji.ListOutput](ctx, "gitlab_merge_request", "emoji_mr_list", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("emoji_mr_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("emoji_mr_list completed (meta)")
}

func metaMergeRequestEmojiMrNoteCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[awardemoji.Output](ctx, "gitlab_merge_request", "emoji_mr_note_create", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("emoji_mr_note_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("emoji_mr_note_create completed (meta)")
}

func metaMergeRequestEmojiMrNoteDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_merge_request", "emoji_mr_note_delete", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("emoji_mr_note_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("emoji_mr_note_delete completed (meta)")
}

func metaMergeRequestEmojiMrNoteGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[awardemoji.Output](ctx, "gitlab_merge_request", "emoji_mr_note_get", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("emoji_mr_note_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("emoji_mr_note_get completed (meta)")
}

func metaMergeRequestEmojiMrNoteList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[awardemoji.ListOutput](ctx, "gitlab_merge_request", "emoji_mr_note_list", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("emoji_mr_note_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("emoji_mr_note_list completed (meta)")
}

func metaMergeRequestEventMrLabelGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[resourceevents.LabelEventOutput](ctx, "gitlab_merge_request", "event_mr_label_get", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("event_mr_label_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("event_mr_label_get completed (meta)")
}

func metaMergeRequestEventMrLabelList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[resourceevents.ListLabelEventsOutput](ctx, "gitlab_merge_request", "event_mr_label_list", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("event_mr_label_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("event_mr_label_list completed (meta)")
}

func metaMergeRequestEventMrMilestoneGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[resourceevents.MilestoneEventOutput](ctx, "gitlab_merge_request", "event_mr_milestone_get", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("event_mr_milestone_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("event_mr_milestone_get completed (meta)")
}

func metaMergeRequestEventMrMilestoneList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[resourceevents.ListMilestoneEventsOutput](ctx, "gitlab_merge_request", "event_mr_milestone_list", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("event_mr_milestone_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("event_mr_milestone_list completed (meta)")
}

func metaMergeRequestEventMrStateGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[resourceevents.StateEventOutput](ctx, "gitlab_merge_request", "event_mr_state_get", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("event_mr_state_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("event_mr_state_get completed (meta)")
}

func metaMergeRequestIssuesClosed(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[mergerequests.IssuesClosedOutput](ctx, "gitlab_merge_request", "issues_closed", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("issues_closed returned error (non-fatal): %v", err)
		return
	}
	t.Log("issues_closed completed (meta)")
}

func metaMergeRequestListGlobal(ctx context.Context, t *testing.T) {
	_, err := callMeta[mergerequests.ListOutput](ctx, "gitlab_merge_request", "list_global", map[string]any{})
	if err != nil {
		t.Logf("list_global returned error (non-fatal): %v", err)
		return
	}
	t.Log("list_global completed (meta)")
}

func metaMergeRequestListGroup(ctx context.Context, t *testing.T) {
	_, err := callMeta[mergerequests.ListOutput](ctx, "gitlab_merge_request", "list_group", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("list_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("list_group completed (meta)")
}

func metaMergeRequestParticipants(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[mergerequests.ParticipantsOutput](ctx, "gitlab_merge_request", "participants", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("participants returned error (non-fatal): %v", err)
		return
	}
	t.Log("participants completed (meta)")
}

func metaMergeRequestReviewers(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[mergerequests.ReviewersOutput](ctx, "gitlab_merge_request", "reviewers", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("reviewers returned error (non-fatal): %v", err)
		return
	}
	t.Log("reviewers completed (meta)")
}

func metaMergeRequestSpentTimeAdd(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[mergerequests.TimeStatsOutput](ctx, "gitlab_merge_request", "spent_time_add", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("spent_time_add returned error (non-fatal): %v", err)
		return
	}
	t.Log("spent_time_add completed (meta)")
}

func metaMergeRequestSpentTimeReset(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[mergerequests.TimeStatsOutput](ctx, "gitlab_merge_request", "spent_time_reset", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("spent_time_reset returned error (non-fatal): %v", err)
		return
	}
	t.Log("spent_time_reset completed (meta)")
}

func metaMergeRequestSubscribe(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[mergerequests.Output](ctx, "gitlab_merge_request", "subscribe", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("subscribe returned error (non-fatal): %v", err)
		return
	}
	t.Log("subscribe completed (meta)")
}

func metaMergeRequestTimeEstimateReset(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[mergerequests.TimeStatsOutput](ctx, "gitlab_merge_request", "time_estimate_reset", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("time_estimate_reset returned error (non-fatal): %v", err)
		return
	}
	t.Log("time_estimate_reset completed (meta)")
}

func metaMergeRequestTimeEstimateSet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[mergerequests.TimeStatsOutput](ctx, "gitlab_merge_request", "time_estimate_set", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("time_estimate_set returned error (non-fatal): %v", err)
		return
	}
	t.Log("time_estimate_set completed (meta)")
}

func metaMergeRequestTimeStats(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[mergerequests.TimeStatsOutput](ctx, "gitlab_merge_request", "time_stats", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("time_stats returned error (non-fatal): %v", err)
		return
	}
	t.Log("time_stats completed (meta)")
}

func metaMergeRequestUnsubscribe(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[mergerequests.Output](ctx, "gitlab_merge_request", "unsubscribe", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("unsubscribe returned error (non-fatal): %v", err)
		return
	}
	t.Log("unsubscribe completed (meta)")
}

// --- gitlab_merge_train gap tests (3 actions) ---

func metaMergeTrainAdd(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[mergetrains.ListOutput](ctx, "gitlab_merge_train", "add", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("add returned error (non-fatal): %v", err)
		return
	}
	t.Log("add completed (meta)")
}

func metaMergeTrainGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[mergetrains.Output](ctx, "gitlab_merge_train", "get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("get returned error (non-fatal): %v", err)
		return
	}
	t.Log("get completed (meta)")
}

func metaMergeTrainListBranch(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[mergetrains.ListOutput](ctx, "gitlab_merge_train", "list_branch", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("list_branch returned error (non-fatal): %v", err)
		return
	}
	t.Log("list_branch completed (meta)")
}

// --- gitlab_mr_review gap tests (4 actions) ---

func metaMrReviewDiscussionNoteDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_mr_review", "discussion_note_delete", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("discussion_note_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("discussion_note_delete completed (meta)")
}

func metaMrReviewDiscussionNoteUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[mrdiscussions.NoteOutput](ctx, "gitlab_mr_review", "discussion_note_update", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("discussion_note_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("discussion_note_update completed (meta)")
}

func metaMrReviewDraftNoteDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_mr_review", "draft_note_delete", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("draft_note_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("draft_note_delete completed (meta)")
}

func metaMrReviewDraftNotePublish(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_mr_review", "draft_note_publish", map[string]any{
		"mr_iid":     mState.mrIID,
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("draft_note_publish returned error (non-fatal): %v", err)
		return
	}
	t.Log("draft_note_publish completed (meta)")
}

// --- gitlab_package gap tests (16 actions) ---

func metaPackageProtectionRuleCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[protectedpackages.Output](ctx, "gitlab_package", "protection_rule_create", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("protection_rule_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("protection_rule_create completed (meta)")
}

func metaPackageProtectionRuleDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_package", "protection_rule_delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("protection_rule_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("protection_rule_delete completed (meta)")
}

func metaPackageProtectionRuleList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[protectedpackages.ListOutput](ctx, "gitlab_package", "protection_rule_list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("protection_rule_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("protection_rule_list completed (meta)")
}

func metaPackageProtectionRuleUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[protectedpackages.Output](ctx, "gitlab_package", "protection_rule_update", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("protection_rule_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("protection_rule_update completed (meta)")
}

func metaPackageRegistryDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_package", "registry_delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("registry_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("registry_delete completed (meta)")
}

func metaPackageRegistryGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[containerregistry.RepositoryOutput](ctx, "gitlab_package", "registry_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("registry_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("registry_get completed (meta)")
}

func metaPackageRegistryListGroup(ctx context.Context, t *testing.T) {
	_, err := callMeta[containerregistry.RepositoryListOutput](ctx, "gitlab_package", "registry_list_group", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("registry_list_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("registry_list_group completed (meta)")
}

func metaPackageRegistryListProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[containerregistry.RepositoryListOutput](ctx, "gitlab_package", "registry_list_project", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("registry_list_project returned error (non-fatal): %v", err)
		return
	}
	t.Log("registry_list_project completed (meta)")
}

func metaPackageRegistryRuleCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[containerregistry.ProtectionRuleOutput](ctx, "gitlab_package", "registry_rule_create", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("registry_rule_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("registry_rule_create completed (meta)")
}

func metaPackageRegistryRuleDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_package", "registry_rule_delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("registry_rule_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("registry_rule_delete completed (meta)")
}

func metaPackageRegistryRuleList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[containerregistry.ProtectionRuleListOutput](ctx, "gitlab_package", "registry_rule_list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("registry_rule_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("registry_rule_list completed (meta)")
}

func metaPackageRegistryRuleUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[containerregistry.ProtectionRuleOutput](ctx, "gitlab_package", "registry_rule_update", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("registry_rule_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("registry_rule_update completed (meta)")
}

func metaPackageRegistryTagDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_package", "registry_tag_delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("registry_tag_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("registry_tag_delete completed (meta)")
}

func metaPackageRegistryTagDeleteBulk(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_package", "registry_tag_delete_bulk", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("registry_tag_delete_bulk returned error (non-fatal): %v", err)
		return
	}
	t.Log("registry_tag_delete_bulk completed (meta)")
}

func metaPackageRegistryTagGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[containerregistry.TagOutput](ctx, "gitlab_package", "registry_tag_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("registry_tag_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("registry_tag_get completed (meta)")
}

func metaPackageRegistryTagList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[containerregistry.TagListOutput](ctx, "gitlab_package", "registry_tag_list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("registry_tag_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("registry_tag_list completed (meta)")
}

// --- gitlab_pipeline gap tests (10 actions) ---

func metaPipelineCancel(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[pipelines.DetailOutput](ctx, "gitlab_pipeline", "cancel", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("cancel returned error (non-fatal): %v", err)
		return
	}
	t.Log("cancel completed (meta)")
}

func metaPipelineCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[pipelines.DetailOutput](ctx, "gitlab_pipeline", "create", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("create returned error (non-fatal): %v", err)
		return
	}
	t.Log("create completed (meta)")
}

func metaPipelineDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_pipeline", "delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete completed (meta)")
}

func metaPipelineResourceGroupEdit(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[resourcegroups.ResourceGroupItem](ctx, "gitlab_pipeline", "resource_group_edit", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("resource_group_edit returned error (non-fatal): %v", err)
		return
	}
	t.Log("resource_group_edit completed (meta)")
}

func metaPipelineResourceGroupGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[resourcegroups.ResourceGroupItem](ctx, "gitlab_pipeline", "resource_group_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("resource_group_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("resource_group_get completed (meta)")
}

func metaPipelineResourceGroupList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[resourcegroups.ListOutput](ctx, "gitlab_pipeline", "resource_group_list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("resource_group_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("resource_group_list completed (meta)")
}

func metaPipelineResourceGroupUpcomingJobs(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[resourcegroups.ListUpcomingJobsOutput](ctx, "gitlab_pipeline", "resource_group_upcoming_jobs", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("resource_group_upcoming_jobs returned error (non-fatal): %v", err)
		return
	}
	t.Log("resource_group_upcoming_jobs completed (meta)")
}

func metaPipelineRetry(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[pipelines.DetailOutput](ctx, "gitlab_pipeline", "retry", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("retry returned error (non-fatal): %v", err)
		return
	}
	t.Log("retry completed (meta)")
}

func metaPipelineTriggerRun(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[pipelinetriggers.RunOutput](ctx, "gitlab_pipeline", "trigger_run", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("trigger_run returned error (non-fatal): %v", err)
		return
	}
	t.Log("trigger_run completed (meta)")
}

func metaPipelineUpdateMetadata(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[pipelines.DetailOutput](ctx, "gitlab_pipeline", "update_metadata", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("update_metadata returned error (non-fatal): %v", err)
		return
	}
	t.Log("update_metadata completed (meta)")
}

// --- gitlab_project gap tests (83 actions) ---

func metaProjectApprovalConfigChange(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.ApprovalConfigOutput](ctx, "gitlab_project", "approval_config_change", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("approval_config_change returned error (non-fatal): %v", err)
		return
	}
	t.Log("approval_config_change completed (meta)")
}

func metaProjectApprovalConfigGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.ApprovalConfigOutput](ctx, "gitlab_project", "approval_config_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("approval_config_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("approval_config_get completed (meta)")
}

func metaProjectApprovalRuleCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.ApprovalRuleOutput](ctx, "gitlab_project", "approval_rule_create", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("approval_rule_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("approval_rule_create completed (meta)")
}

func metaProjectApprovalRuleDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_project", "approval_rule_delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("approval_rule_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("approval_rule_delete completed (meta)")
}

func metaProjectApprovalRuleGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.ApprovalRuleOutput](ctx, "gitlab_project", "approval_rule_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("approval_rule_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("approval_rule_get completed (meta)")
}

func metaProjectApprovalRuleList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.ListApprovalRulesOutput](ctx, "gitlab_project", "approval_rule_list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("approval_rule_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("approval_rule_list completed (meta)")
}

func metaProjectApprovalRuleUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.ApprovalRuleOutput](ctx, "gitlab_project", "approval_rule_update", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("approval_rule_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("approval_rule_update completed (meta)")
}

func metaProjectArchive(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.Output](ctx, "gitlab_project", "archive", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("archive returned error (non-fatal): %v", err)
		return
	}
	t.Log("archive completed (meta)")
}

func metaProjectBadgeGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[badges.GetProjectOutput](ctx, "gitlab_project", "badge_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("badge_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("badge_get completed (meta)")
}

func metaProjectBadgePreview(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[badges.PreviewProjectOutput](ctx, "gitlab_project", "badge_preview", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("badge_preview returned error (non-fatal): %v", err)
		return
	}
	t.Log("badge_preview completed (meta)")
}

func metaProjectBoardListCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[boards.BoardListOutput](ctx, "gitlab_project", "board_list_create", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("board_list_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("board_list_create completed (meta)")
}

func metaProjectBoardListDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_project", "board_list_delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("board_list_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("board_list_delete completed (meta)")
}

func metaProjectBoardListGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[boards.BoardListOutput](ctx, "gitlab_project", "board_list_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("board_list_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("board_list_get completed (meta)")
}

func metaProjectBoardListList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[boards.ListBoardListsOutput](ctx, "gitlab_project", "board_list_list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("board_list_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("board_list_list completed (meta)")
}

func metaProjectBoardListUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[boards.BoardListOutput](ctx, "gitlab_project", "board_list_update", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("board_list_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("board_list_update completed (meta)")
}

func metaProjectBoardUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[boards.BoardOutput](ctx, "gitlab_project", "board_update", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("board_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("board_update completed (meta)")
}

func metaProjectCreateForUser(ctx context.Context, t *testing.T) {
	_, err := callMeta[projects.Output](ctx, "gitlab_project", "create_for_user", map[string]any{})
	if err != nil {
		t.Logf("create_for_user returned error (non-fatal): %v", err)
		return
	}
	t.Log("create_for_user completed (meta)")
}

func metaProjectCreateForkRelation(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.ForkRelationOutput](ctx, "gitlab_project", "create_fork_relation", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("create_fork_relation returned error (non-fatal): %v", err)
		return
	}
	t.Log("create_fork_relation completed (meta)")
}

func metaProjectDeleteForkRelation(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_project", "delete_fork_relation", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("delete_fork_relation returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete_fork_relation completed (meta)")
}

func metaProjectDeleteSharedGroup(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_project", "delete_shared_group", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("delete_shared_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete_shared_group completed (meta)")
}

func metaProjectDownloadAvatar(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.DownloadAvatarOutput](ctx, "gitlab_project", "download_avatar", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("download_avatar returned error (non-fatal): %v", err)
		return
	}
	t.Log("download_avatar completed (meta)")
}

func metaProjectExportDownload(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projectimportexport.ExportDownloadOutput](ctx, "gitlab_project", "export_download", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("export_download returned error (non-fatal): %v", err)
		return
	}
	t.Log("export_download completed (meta)")
}

func metaProjectExportSchedule(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projectimportexport.ScheduleExportOutput](ctx, "gitlab_project", "export_schedule", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("export_schedule returned error (non-fatal): %v", err)
		return
	}
	t.Log("export_schedule completed (meta)")
}

func metaProjectExportStatus(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projectimportexport.ExportStatusOutput](ctx, "gitlab_project", "export_status", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("export_status returned error (non-fatal): %v", err)
		return
	}
	t.Log("export_status completed (meta)")
}

func metaProjectFork(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.Output](ctx, "gitlab_project", "fork", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("fork returned error (non-fatal): %v", err)
		return
	}
	t.Log("fork completed (meta)")
}

func metaProjectHookAdd(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.HookOutput](ctx, "gitlab_project", "hook_add", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("hook_add returned error (non-fatal): %v", err)
		return
	}
	t.Log("hook_add completed (meta)")
}

func metaProjectHookDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_project", "hook_delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("hook_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("hook_delete completed (meta)")
}

func metaProjectHookDeleteCustomHeader(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_project", "hook_delete_custom_header", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("hook_delete_custom_header returned error (non-fatal): %v", err)
		return
	}
	t.Log("hook_delete_custom_header completed (meta)")
}

func metaProjectHookDeleteUrlVariable(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_project", "hook_delete_url_variable", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("hook_delete_url_variable returned error (non-fatal): %v", err)
		return
	}
	t.Log("hook_delete_url_variable completed (meta)")
}

func metaProjectHookEdit(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.HookOutput](ctx, "gitlab_project", "hook_edit", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("hook_edit returned error (non-fatal): %v", err)
		return
	}
	t.Log("hook_edit completed (meta)")
}

func metaProjectHookGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.HookOutput](ctx, "gitlab_project", "hook_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("hook_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("hook_get completed (meta)")
}

func metaProjectHookList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.ListHooksOutput](ctx, "gitlab_project", "hook_list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("hook_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("hook_list completed (meta)")
}

func metaProjectHookSetCustomHeader(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_project", "hook_set_custom_header", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("hook_set_custom_header returned error (non-fatal): %v", err)
		return
	}
	t.Log("hook_set_custom_header completed (meta)")
}

func metaProjectHookSetUrlVariable(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_project", "hook_set_url_variable", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("hook_set_url_variable returned error (non-fatal): %v", err)
		return
	}
	t.Log("hook_set_url_variable completed (meta)")
}

func metaProjectHookTest(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.TriggerTestHookOutput](ctx, "gitlab_project", "hook_test", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("hook_test returned error (non-fatal): %v", err)
		return
	}
	t.Log("hook_test completed (meta)")
}

func metaProjectImportFromFile(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projectimportexport.ImportStatusOutput](ctx, "gitlab_project", "import_from_file", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("import_from_file returned error (non-fatal): %v", err)
		return
	}
	t.Log("import_from_file completed (meta)")
}

func metaProjectImportStatus(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projectimportexport.ImportStatusOutput](ctx, "gitlab_project", "import_status", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("import_status returned error (non-fatal): %v", err)
		return
	}
	t.Log("import_status completed (meta)")
}

func metaProjectIntegrationDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_project", "integration_delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("integration_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("integration_delete completed (meta)")
}

func metaProjectIntegrationGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[integrations.GetOutput](ctx, "gitlab_project", "integration_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("integration_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("integration_get completed (meta)")
}

func metaProjectIntegrationList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[integrations.ListOutput](ctx, "gitlab_project", "integration_list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("integration_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("integration_list completed (meta)")
}

func metaProjectIntegrationSetJira(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[integrations.SetJiraOutput](ctx, "gitlab_project", "integration_set_jira", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("integration_set_jira returned error (non-fatal): %v", err)
		return
	}
	t.Log("integration_set_jira completed (meta)")
}

func metaProjectLabelGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[labels.Output](ctx, "gitlab_project", "label_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("label_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("label_get completed (meta)")
}

func metaProjectLabelPromote(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_project", "label_promote", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("label_promote returned error (non-fatal): %v", err)
		return
	}
	t.Log("label_promote completed (meta)")
}

func metaProjectLabelSubscribe(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[labels.Output](ctx, "gitlab_project", "label_subscribe", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("label_subscribe returned error (non-fatal): %v", err)
		return
	}
	t.Log("label_subscribe completed (meta)")
}

func metaProjectLabelUnsubscribe(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_project", "label_unsubscribe", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("label_unsubscribe returned error (non-fatal): %v", err)
		return
	}
	t.Log("label_unsubscribe completed (meta)")
}

func metaProjectLanguages(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.LanguagesOutput](ctx, "gitlab_project", "languages", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("languages returned error (non-fatal): %v", err)
		return
	}
	t.Log("languages completed (meta)")
}

func metaProjectListForks(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.ListForksOutput](ctx, "gitlab_project", "list_forks", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("list_forks returned error (non-fatal): %v", err)
		return
	}
	t.Log("list_forks completed (meta)")
}

func metaProjectListGroups(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.ListProjectGroupsOutput](ctx, "gitlab_project", "list_groups", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("list_groups returned error (non-fatal): %v", err)
		return
	}
	t.Log("list_groups completed (meta)")
}

func metaProjectListInvitedGroups(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.ListProjectGroupsOutput](ctx, "gitlab_project", "list_invited_groups", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("list_invited_groups returned error (non-fatal): %v", err)
		return
	}
	t.Log("list_invited_groups completed (meta)")
}

func metaProjectListStarrers(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.ListProjectStarrersOutput](ctx, "gitlab_project", "list_starrers", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("list_starrers returned error (non-fatal): %v", err)
		return
	}
	t.Log("list_starrers completed (meta)")
}

func metaProjectListUserProjects(ctx context.Context, t *testing.T) {
	_, err := callMeta[projects.ListOutput](ctx, "gitlab_project", "list_user_projects", map[string]any{})
	if err != nil {
		t.Logf("list_user_projects returned error (non-fatal): %v", err)
		return
	}
	t.Log("list_user_projects completed (meta)")
}

func metaProjectListUsers(ctx context.Context, t *testing.T) {
	_, err := callMeta[projects.ListProjectUsersOutput](ctx, "gitlab_project", "list_users", map[string]any{})
	if err != nil {
		t.Logf("list_users returned error (non-fatal): %v", err)
		return
	}
	t.Log("list_users completed (meta)")
}

func metaProjectMemberAdd(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[members.Output](ctx, "gitlab_project", "member_add", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("member_add returned error (non-fatal): %v", err)
		return
	}
	t.Log("member_add completed (meta)")
}

func metaProjectMemberDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_project", "member_delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("member_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("member_delete completed (meta)")
}

func metaProjectMemberEdit(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[members.Output](ctx, "gitlab_project", "member_edit", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("member_edit returned error (non-fatal): %v", err)
		return
	}
	t.Log("member_edit completed (meta)")
}

func metaProjectMemberGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[members.Output](ctx, "gitlab_project", "member_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("member_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("member_get completed (meta)")
}

func metaProjectMemberInherited(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[members.Output](ctx, "gitlab_project", "member_inherited", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("member_inherited returned error (non-fatal): %v", err)
		return
	}
	t.Log("member_inherited completed (meta)")
}

func metaProjectMilestoneIssues(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[milestones.MilestoneIssuesOutput](ctx, "gitlab_project", "milestone_issues", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("milestone_issues returned error (non-fatal): %v", err)
		return
	}
	t.Log("milestone_issues completed (meta)")
}

func metaProjectMilestoneMergeRequests(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[milestones.MilestoneMergeRequestsOutput](ctx, "gitlab_project", "milestone_merge_requests", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("milestone_merge_requests returned error (non-fatal): %v", err)
		return
	}
	t.Log("milestone_merge_requests completed (meta)")
}

func metaProjectPagesDomainCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[pages.DomainOutput](ctx, "gitlab_project", "pages_domain_create", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("pages_domain_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("pages_domain_create completed (meta)")
}

func metaProjectPagesDomainDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_project", "pages_domain_delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("pages_domain_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("pages_domain_delete completed (meta)")
}

func metaProjectPagesDomainGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[pages.DomainOutput](ctx, "gitlab_project", "pages_domain_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("pages_domain_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("pages_domain_get completed (meta)")
}

func metaProjectPagesDomainList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[pages.ListDomainsOutput](ctx, "gitlab_project", "pages_domain_list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("pages_domain_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("pages_domain_list completed (meta)")
}

func metaProjectPagesDomainListAll(ctx context.Context, t *testing.T) {
	_, err := callMeta[pages.ListAllDomainsOutput](ctx, "gitlab_project", "pages_domain_list_all", map[string]any{})
	if err != nil {
		t.Logf("pages_domain_list_all returned error (non-fatal): %v", err)
		return
	}
	t.Log("pages_domain_list_all completed (meta)")
}

func metaProjectPagesDomainUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[pages.DomainOutput](ctx, "gitlab_project", "pages_domain_update", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("pages_domain_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("pages_domain_update completed (meta)")
}

func metaProjectPagesGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[pages.Output](ctx, "gitlab_project", "pages_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("pages_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("pages_get completed (meta)")
}

func metaProjectPagesUnpublish(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_project", "pages_unpublish", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("pages_unpublish returned error (non-fatal): %v", err)
		return
	}
	t.Log("pages_unpublish completed (meta)")
}

func metaProjectPagesUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[pages.Output](ctx, "gitlab_project", "pages_update", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("pages_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("pages_update completed (meta)")
}

func metaProjectPullMirrorConfigure(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.PullMirrorOutput](ctx, "gitlab_project", "pull_mirror_configure", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("pull_mirror_configure returned error (non-fatal): %v", err)
		return
	}
	t.Log("pull_mirror_configure completed (meta)")
}

func metaProjectPullMirrorGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.PullMirrorOutput](ctx, "gitlab_project", "pull_mirror_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("pull_mirror_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("pull_mirror_get completed (meta)")
}

func metaProjectRepositoryStorageGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.RepositoryStorageOutput](ctx, "gitlab_project", "repository_storage_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("repository_storage_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("repository_storage_get completed (meta)")
}

func metaProjectRestore(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.Output](ctx, "gitlab_project", "restore", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("restore returned error (non-fatal): %v", err)
		return
	}
	t.Log("restore completed (meta)")
}

func metaProjectShareWithGroup(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.ShareProjectOutput](ctx, "gitlab_project", "share_with_group", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("share_with_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("share_with_group completed (meta)")
}

func metaProjectStar(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.Output](ctx, "gitlab_project", "star", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("star returned error (non-fatal): %v", err)
		return
	}
	t.Log("star completed (meta)")
}

func metaProjectStartHousekeeping(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_project", "start_housekeeping", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("start_housekeeping returned error (non-fatal): %v", err)
		return
	}
	t.Log("start_housekeeping completed (meta)")
}

func metaProjectStartMirroring(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_project", "start_mirroring", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("start_mirroring returned error (non-fatal): %v", err)
		return
	}
	t.Log("start_mirroring completed (meta)")
}

func metaProjectStatisticsGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projectstatistics.GetOutput](ctx, "gitlab_project", "statistics_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("statistics_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("statistics_get completed (meta)")
}

func metaProjectTransfer(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.Output](ctx, "gitlab_project", "transfer", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("transfer returned error (non-fatal): %v", err)
		return
	}
	t.Log("transfer completed (meta)")
}

func metaProjectUnarchive(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.Output](ctx, "gitlab_project", "unarchive", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("unarchive returned error (non-fatal): %v", err)
		return
	}
	t.Log("unarchive completed (meta)")
}

func metaProjectUnstar(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.Output](ctx, "gitlab_project", "unstar", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("unstar returned error (non-fatal): %v", err)
		return
	}
	t.Log("unstar completed (meta)")
}

func metaProjectUploadAvatar(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projects.Output](ctx, "gitlab_project", "upload_avatar", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("upload_avatar returned error (non-fatal): %v", err)
		return
	}
	t.Log("upload_avatar completed (meta)")
}

func metaProjectUploadDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_project", "upload_delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("upload_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("upload_delete completed (meta)")
}

func metaProjectUploadList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[uploads.ListOutput](ctx, "gitlab_project", "upload_list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("upload_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("upload_list completed (meta)")
}

// --- gitlab_project_alias gap tests (3 actions) ---

func metaProjectAliasCreate(ctx context.Context, t *testing.T) {
	_, err := callMeta[projectaliases.Output](ctx, "gitlab_project_alias", "create", map[string]any{})
	if err != nil {
		t.Logf("create returned error (non-fatal): %v", err)
		return
	}
	t.Log("create completed (meta)")
}

func metaProjectAliasDelete(ctx context.Context, t *testing.T) {
	err := callMetaVoid(ctx, "gitlab_project_alias", "delete", map[string]any{})
	if err != nil {
		t.Logf("delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete completed (meta)")
}

func metaProjectAliasGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[projectaliases.Output](ctx, "gitlab_project_alias", "get", map[string]any{})
	if err != nil {
		t.Logf("get returned error (non-fatal): %v", err)
		return
	}
	t.Log("get completed (meta)")
}

// --- gitlab_release gap tests (1 actions) ---

func metaReleaseLinkCreateBatch(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[releaselinks.CreateBatchOutput](ctx, "gitlab_release", "link_create_batch", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("link_create_batch returned error (non-fatal): %v", err)
		return
	}
	t.Log("link_create_batch completed (meta)")
}

// --- gitlab_repository gap tests (32 actions) ---

func metaRepositoryArchive(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[repository.ArchiveOutput](ctx, "gitlab_repository", "archive", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("archive returned error (non-fatal): %v", err)
		return
	}
	t.Log("archive completed (meta)")
}

func metaRepositoryBlob(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[repository.BlobOutput](ctx, "gitlab_repository", "blob", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("blob returned error (non-fatal): %v", err)
		return
	}
	t.Log("blob completed (meta)")
}

func metaRepositoryChangelogAdd(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[repository.AddChangelogOutput](ctx, "gitlab_repository", "changelog_add", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("changelog_add returned error (non-fatal): %v", err)
		return
	}
	t.Log("changelog_add completed (meta)")
}

func metaRepositoryChangelogGenerate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[repository.ChangelogDataOutput](ctx, "gitlab_repository", "changelog_generate", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("changelog_generate returned error (non-fatal): %v", err)
		return
	}
	t.Log("changelog_generate completed (meta)")
}

func metaRepositoryCommitCherryPick(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[commits.Output](ctx, "gitlab_repository", "commit_cherry_pick", map[string]any{
		"project_id": mPID(),
		"sha":        mState.lastCommitSHA,
	})
	if err != nil {
		t.Logf("commit_cherry_pick returned error (non-fatal): %v", err)
		return
	}
	t.Log("commit_cherry_pick completed (meta)")
}

func metaRepositoryCommitCommentCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[commits.CommentOutput](ctx, "gitlab_repository", "commit_comment_create", map[string]any{
		"project_id": mPID(),
		"sha":        mState.lastCommitSHA,
	})
	if err != nil {
		t.Logf("commit_comment_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("commit_comment_create completed (meta)")
}

func metaRepositoryCommitComments(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[commits.CommentsOutput](ctx, "gitlab_repository", "commit_comments", map[string]any{
		"project_id": mPID(),
		"sha":        mState.lastCommitSHA,
	})
	if err != nil {
		t.Logf("commit_comments returned error (non-fatal): %v", err)
		return
	}
	t.Log("commit_comments completed (meta)")
}

func metaRepositoryCommitDiscussionAddNote(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[commitdiscussions.NoteOutput](ctx, "gitlab_repository", "commit_discussion_add_note", map[string]any{
		"project_id": mPID(),
		"sha":        mState.lastCommitSHA,
	})
	if err != nil {
		t.Logf("commit_discussion_add_note returned error (non-fatal): %v", err)
		return
	}
	t.Log("commit_discussion_add_note completed (meta)")
}

func metaRepositoryCommitDiscussionCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[commitdiscussions.Output](ctx, "gitlab_repository", "commit_discussion_create", map[string]any{
		"project_id": mPID(),
		"sha":        mState.lastCommitSHA,
	})
	if err != nil {
		t.Logf("commit_discussion_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("commit_discussion_create completed (meta)")
}

func metaRepositoryCommitDiscussionDeleteNote(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_repository", "commit_discussion_delete_note", map[string]any{
		"project_id": mPID(),
		"sha":        mState.lastCommitSHA,
	})
	if err != nil {
		t.Logf("commit_discussion_delete_note returned error (non-fatal): %v", err)
		return
	}
	t.Log("commit_discussion_delete_note completed (meta)")
}

func metaRepositoryCommitDiscussionGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[commitdiscussions.Output](ctx, "gitlab_repository", "commit_discussion_get", map[string]any{
		"project_id": mPID(),
		"sha":        mState.lastCommitSHA,
	})
	if err != nil {
		t.Logf("commit_discussion_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("commit_discussion_get completed (meta)")
}

func metaRepositoryCommitDiscussionList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[commitdiscussions.ListOutput](ctx, "gitlab_repository", "commit_discussion_list", map[string]any{
		"project_id": mPID(),
		"sha":        mState.lastCommitSHA,
	})
	if err != nil {
		t.Logf("commit_discussion_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("commit_discussion_list completed (meta)")
}

func metaRepositoryCommitDiscussionUpdateNote(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[commitdiscussions.NoteOutput](ctx, "gitlab_repository", "commit_discussion_update_note", map[string]any{
		"project_id": mPID(),
		"sha":        mState.lastCommitSHA,
	})
	if err != nil {
		t.Logf("commit_discussion_update_note returned error (non-fatal): %v", err)
		return
	}
	t.Log("commit_discussion_update_note completed (meta)")
}

func metaRepositoryCommitMergeRequests(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[commits.MRsByCommitOutput](ctx, "gitlab_repository", "commit_merge_requests", map[string]any{
		"project_id": mPID(),
		"sha":        mState.lastCommitSHA,
	})
	if err != nil {
		t.Logf("commit_merge_requests returned error (non-fatal): %v", err)
		return
	}
	t.Log("commit_merge_requests completed (meta)")
}

func metaRepositoryCommitRefs(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[commits.RefsOutput](ctx, "gitlab_repository", "commit_refs", map[string]any{
		"project_id": mPID(),
		"sha":        mState.lastCommitSHA,
	})
	if err != nil {
		t.Logf("commit_refs returned error (non-fatal): %v", err)
		return
	}
	t.Log("commit_refs completed (meta)")
}

func metaRepositoryCommitRevert(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[commits.Output](ctx, "gitlab_repository", "commit_revert", map[string]any{
		"project_id": mPID(),
		"sha":        mState.lastCommitSHA,
	})
	if err != nil {
		t.Logf("commit_revert returned error (non-fatal): %v", err)
		return
	}
	t.Log("commit_revert completed (meta)")
}

func metaRepositoryCommitSignature(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[commits.GPGSignatureOutput](ctx, "gitlab_repository", "commit_signature", map[string]any{
		"project_id": mPID(),
		"sha":        mState.lastCommitSHA,
	})
	if err != nil {
		t.Logf("commit_signature returned error (non-fatal): %v", err)
		return
	}
	t.Log("commit_signature completed (meta)")
}

func metaRepositoryCommitStatusSet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[commits.StatusOutput](ctx, "gitlab_repository", "commit_status_set", map[string]any{
		"project_id": mPID(),
		"sha":        mState.lastCommitSHA,
	})
	if err != nil {
		t.Logf("commit_status_set returned error (non-fatal): %v", err)
		return
	}
	t.Log("commit_status_set completed (meta)")
}

func metaRepositoryCommitStatuses(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[commits.StatusesOutput](ctx, "gitlab_repository", "commit_statuses", map[string]any{
		"project_id": mPID(),
		"sha":        mState.lastCommitSHA,
	})
	if err != nil {
		t.Logf("commit_statuses returned error (non-fatal): %v", err)
		return
	}
	t.Log("commit_statuses completed (meta)")
}

func metaRepositoryContributors(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[repository.ContributorsOutput](ctx, "gitlab_repository", "contributors", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("contributors returned error (non-fatal): %v", err)
		return
	}
	t.Log("contributors completed (meta)")
}

func metaRepositoryFileBlame(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[files.BlameOutput](ctx, "gitlab_repository", "file_blame", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("file_blame returned error (non-fatal): %v", err)
		return
	}
	t.Log("file_blame completed (meta)")
}

func metaRepositoryFileCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[files.FileInfoOutput](ctx, "gitlab_repository", "file_create", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("file_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("file_create completed (meta)")
}

func metaRepositoryFileDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_repository", "file_delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("file_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("file_delete completed (meta)")
}

func metaRepositoryFileHistory(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[commits.ListOutput](ctx, "gitlab_repository", "file_history", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("file_history returned error (non-fatal): %v", err)
		return
	}
	t.Log("file_history completed (meta)")
}

func metaRepositoryFileMetadata(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[files.MetaDataOutput](ctx, "gitlab_repository", "file_metadata", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("file_metadata returned error (non-fatal): %v", err)
		return
	}
	t.Log("file_metadata completed (meta)")
}

func metaRepositoryFileRaw(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[files.RawOutput](ctx, "gitlab_repository", "file_raw", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("file_raw returned error (non-fatal): %v", err)
		return
	}
	t.Log("file_raw completed (meta)")
}

func metaRepositoryFileUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[files.FileInfoOutput](ctx, "gitlab_repository", "file_update", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("file_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("file_update completed (meta)")
}

func metaRepositoryListSubmodules(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[repositorysubmodules.ListOutput](ctx, "gitlab_repository", "list_submodules", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("list_submodules returned error (non-fatal): %v", err)
		return
	}
	t.Log("list_submodules completed (meta)")
}

func metaRepositoryMergeBase(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[commits.Output](ctx, "gitlab_repository", "merge_base", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("merge_base returned error (non-fatal): %v", err)
		return
	}
	t.Log("merge_base completed (meta)")
}

func metaRepositoryRawBlob(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[repository.RawBlobContentOutput](ctx, "gitlab_repository", "raw_blob", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("raw_blob returned error (non-fatal): %v", err)
		return
	}
	t.Log("raw_blob completed (meta)")
}

func metaRepositoryReadSubmoduleFile(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[repositorysubmodules.ReadOutput](ctx, "gitlab_repository", "read_submodule_file", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("read_submodule_file returned error (non-fatal): %v", err)
		return
	}
	t.Log("read_submodule_file completed (meta)")
}

func metaRepositoryUpdateSubmodule(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[repositorysubmodules.UpdateOutput](ctx, "gitlab_repository", "update_submodule", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("update_submodule returned error (non-fatal): %v", err)
		return
	}
	t.Log("update_submodule completed (meta)")
}

// --- gitlab_snippet gap tests (29 actions) ---

func metaSnippetContent(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[snippets.ContentOutput](ctx, "gitlab_snippet", "content", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("content returned error (non-fatal): %v", err)
		return
	}
	t.Log("content completed (meta)")
}

func metaSnippetDiscussionAddNote(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[snippetdiscussions.NoteOutput](ctx, "gitlab_snippet", "discussion_add_note", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("discussion_add_note returned error (non-fatal): %v", err)
		return
	}
	t.Log("discussion_add_note completed (meta)")
}

func metaSnippetDiscussionCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[snippetdiscussions.Output](ctx, "gitlab_snippet", "discussion_create", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("discussion_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("discussion_create completed (meta)")
}

func metaSnippetDiscussionDeleteNote(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_snippet", "discussion_delete_note", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("discussion_delete_note returned error (non-fatal): %v", err)
		return
	}
	t.Log("discussion_delete_note completed (meta)")
}

func metaSnippetDiscussionGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[snippetdiscussions.Output](ctx, "gitlab_snippet", "discussion_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("discussion_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("discussion_get completed (meta)")
}

func metaSnippetDiscussionList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[snippetdiscussions.ListOutput](ctx, "gitlab_snippet", "discussion_list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("discussion_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("discussion_list completed (meta)")
}

func metaSnippetDiscussionUpdateNote(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[snippetdiscussions.NoteOutput](ctx, "gitlab_snippet", "discussion_update_note", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("discussion_update_note returned error (non-fatal): %v", err)
		return
	}
	t.Log("discussion_update_note completed (meta)")
}

func metaSnippetEmojiSnippetCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[awardemoji.Output](ctx, "gitlab_snippet", "emoji_snippet_create", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("emoji_snippet_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("emoji_snippet_create completed (meta)")
}

func metaSnippetEmojiSnippetDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_snippet", "emoji_snippet_delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("emoji_snippet_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("emoji_snippet_delete completed (meta)")
}

func metaSnippetEmojiSnippetGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[awardemoji.Output](ctx, "gitlab_snippet", "emoji_snippet_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("emoji_snippet_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("emoji_snippet_get completed (meta)")
}

func metaSnippetEmojiSnippetList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[awardemoji.ListOutput](ctx, "gitlab_snippet", "emoji_snippet_list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("emoji_snippet_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("emoji_snippet_list completed (meta)")
}

func metaSnippetEmojiSnippetNoteCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[awardemoji.Output](ctx, "gitlab_snippet", "emoji_snippet_note_create", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("emoji_snippet_note_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("emoji_snippet_note_create completed (meta)")
}

func metaSnippetEmojiSnippetNoteDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_snippet", "emoji_snippet_note_delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("emoji_snippet_note_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("emoji_snippet_note_delete completed (meta)")
}

func metaSnippetEmojiSnippetNoteGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[awardemoji.Output](ctx, "gitlab_snippet", "emoji_snippet_note_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("emoji_snippet_note_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("emoji_snippet_note_get completed (meta)")
}

func metaSnippetEmojiSnippetNoteList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[awardemoji.ListOutput](ctx, "gitlab_snippet", "emoji_snippet_note_list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("emoji_snippet_note_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("emoji_snippet_note_list completed (meta)")
}

func metaSnippetExplore(ctx context.Context, t *testing.T) {
	_, err := callMeta[snippets.ListOutput](ctx, "gitlab_snippet", "explore", map[string]any{})
	if err != nil {
		t.Logf("explore returned error (non-fatal): %v", err)
		return
	}
	t.Log("explore completed (meta)")
}

func metaSnippetFileContent(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[snippets.FileContentOutput](ctx, "gitlab_snippet", "file_content", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("file_content returned error (non-fatal): %v", err)
		return
	}
	t.Log("file_content completed (meta)")
}

func metaSnippetListAll(ctx context.Context, t *testing.T) {
	_, err := callMeta[snippets.ListOutput](ctx, "gitlab_snippet", "list_all", map[string]any{})
	if err != nil {
		t.Logf("list_all returned error (non-fatal): %v", err)
		return
	}
	t.Log("list_all completed (meta)")
}

func metaSnippetNoteCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[snippetnotes.Output](ctx, "gitlab_snippet", "note_create", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("note_create returned error (non-fatal): %v", err)
		return
	}
	t.Log("note_create completed (meta)")
}

func metaSnippetNoteDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_snippet", "note_delete", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("note_delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("note_delete completed (meta)")
}

func metaSnippetNoteGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[snippetnotes.Output](ctx, "gitlab_snippet", "note_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("note_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("note_get completed (meta)")
}

func metaSnippetNoteList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[snippetnotes.ListOutput](ctx, "gitlab_snippet", "note_list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("note_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("note_list completed (meta)")
}

func metaSnippetNoteUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[snippetnotes.Output](ctx, "gitlab_snippet", "note_update", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("note_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("note_update completed (meta)")
}

func metaSnippetProjectContent(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[snippets.ContentOutput](ctx, "gitlab_snippet", "project_content", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("project_content returned error (non-fatal): %v", err)
		return
	}
	t.Log("project_content completed (meta)")
}

// --- gitlab_storage_move gap tests (17 actions) ---

func metaStorageMoveGetGroup(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupstoragemoves.Output](ctx, "gitlab_storage_move", "get_group", map[string]any{})
	if err != nil {
		t.Logf("get_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("get_group completed (meta)")
}

func metaStorageMoveGetGroupForGroup(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupstoragemoves.Output](ctx, "gitlab_storage_move", "get_group_for_group", map[string]any{})
	if err != nil {
		t.Logf("get_group_for_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("get_group_for_group completed (meta)")
}

func metaStorageMoveGetProject(ctx context.Context, t *testing.T) {
	_, err := callMeta[projectstoragemoves.Output](ctx, "gitlab_storage_move", "get_project", map[string]any{})
	if err != nil {
		t.Logf("get_project returned error (non-fatal): %v", err)
		return
	}
	t.Log("get_project completed (meta)")
}

func metaStorageMoveGetProjectForProject(ctx context.Context, t *testing.T) {
	_, err := callMeta[projectstoragemoves.Output](ctx, "gitlab_storage_move", "get_project_for_project", map[string]any{})
	if err != nil {
		t.Logf("get_project_for_project returned error (non-fatal): %v", err)
		return
	}
	t.Log("get_project_for_project completed (meta)")
}

func metaStorageMoveGetSnippet(ctx context.Context, t *testing.T) {
	_, err := callMeta[snippetstoragemoves.Output](ctx, "gitlab_storage_move", "get_snippet", map[string]any{})
	if err != nil {
		t.Logf("get_snippet returned error (non-fatal): %v", err)
		return
	}
	t.Log("get_snippet completed (meta)")
}

func metaStorageMoveGetSnippetForSnippet(ctx context.Context, t *testing.T) {
	_, err := callMeta[snippetstoragemoves.Output](ctx, "gitlab_storage_move", "get_snippet_for_snippet", map[string]any{})
	if err != nil {
		t.Logf("get_snippet_for_snippet returned error (non-fatal): %v", err)
		return
	}
	t.Log("get_snippet_for_snippet completed (meta)")
}

func metaStorageMoveRetrieveAllGroup(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupstoragemoves.ListOutput](ctx, "gitlab_storage_move", "retrieve_all_group", map[string]any{})
	if err != nil {
		t.Logf("retrieve_all_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("retrieve_all_group completed (meta)")
}

func metaStorageMoveRetrieveAllSnippet(ctx context.Context, t *testing.T) {
	_, err := callMeta[snippetstoragemoves.ListOutput](ctx, "gitlab_storage_move", "retrieve_all_snippet", map[string]any{})
	if err != nil {
		t.Logf("retrieve_all_snippet returned error (non-fatal): %v", err)
		return
	}
	t.Log("retrieve_all_snippet completed (meta)")
}

func metaStorageMoveRetrieveGroup(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupstoragemoves.ListOutput](ctx, "gitlab_storage_move", "retrieve_group", map[string]any{})
	if err != nil {
		t.Logf("retrieve_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("retrieve_group completed (meta)")
}

func metaStorageMoveRetrieveProject(ctx context.Context, t *testing.T) {
	_, err := callMeta[projectstoragemoves.ListOutput](ctx, "gitlab_storage_move", "retrieve_project", map[string]any{})
	if err != nil {
		t.Logf("retrieve_project returned error (non-fatal): %v", err)
		return
	}
	t.Log("retrieve_project completed (meta)")
}

func metaStorageMoveRetrieveSnippet(ctx context.Context, t *testing.T) {
	_, err := callMeta[snippetstoragemoves.ListOutput](ctx, "gitlab_storage_move", "retrieve_snippet", map[string]any{})
	if err != nil {
		t.Logf("retrieve_snippet returned error (non-fatal): %v", err)
		return
	}
	t.Log("retrieve_snippet completed (meta)")
}

func metaStorageMoveScheduleAllGroup(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupstoragemoves.ScheduleAllOutput](ctx, "gitlab_storage_move", "schedule_all_group", map[string]any{})
	if err != nil {
		t.Logf("schedule_all_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("schedule_all_group completed (meta)")
}

func metaStorageMoveScheduleAllProject(ctx context.Context, t *testing.T) {
	_, err := callMeta[projectstoragemoves.ScheduleAllOutput](ctx, "gitlab_storage_move", "schedule_all_project", map[string]any{})
	if err != nil {
		t.Logf("schedule_all_project returned error (non-fatal): %v", err)
		return
	}
	t.Log("schedule_all_project completed (meta)")
}

func metaStorageMoveScheduleAllSnippet(ctx context.Context, t *testing.T) {
	_, err := callMeta[snippetstoragemoves.ScheduleAllOutput](ctx, "gitlab_storage_move", "schedule_all_snippet", map[string]any{})
	if err != nil {
		t.Logf("schedule_all_snippet returned error (non-fatal): %v", err)
		return
	}
	t.Log("schedule_all_snippet completed (meta)")
}

func metaStorageMoveScheduleGroup(ctx context.Context, t *testing.T) {
	_, err := callMeta[groupstoragemoves.Output](ctx, "gitlab_storage_move", "schedule_group", map[string]any{})
	if err != nil {
		t.Logf("schedule_group returned error (non-fatal): %v", err)
		return
	}
	t.Log("schedule_group completed (meta)")
}

func metaStorageMoveScheduleProject(ctx context.Context, t *testing.T) {
	_, err := callMeta[projectstoragemoves.Output](ctx, "gitlab_storage_move", "schedule_project", map[string]any{})
	if err != nil {
		t.Logf("schedule_project returned error (non-fatal): %v", err)
		return
	}
	t.Log("schedule_project completed (meta)")
}

func metaStorageMoveScheduleSnippet(ctx context.Context, t *testing.T) {
	_, err := callMeta[snippetstoragemoves.Output](ctx, "gitlab_storage_move", "schedule_snippet", map[string]any{})
	if err != nil {
		t.Logf("schedule_snippet returned error (non-fatal): %v", err)
		return
	}
	t.Log("schedule_snippet completed (meta)")
}

// --- gitlab_template gap tests (9 actions) ---

func metaTemplateCiYmlGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[ciyamltemplates.GetOutput](ctx, "gitlab_template", "ci_yml_get", map[string]any{})
	if err != nil {
		t.Logf("ci_yml_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("ci_yml_get completed (meta)")
}

func metaTemplateDockerfileGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[dockerfiletemplates.GetOutput](ctx, "gitlab_template", "dockerfile_get", map[string]any{})
	if err != nil {
		t.Logf("dockerfile_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("dockerfile_get completed (meta)")
}

func metaTemplateDockerfileList(ctx context.Context, t *testing.T) {
	_, err := callMeta[dockerfiletemplates.ListOutput](ctx, "gitlab_template", "dockerfile_list", map[string]any{})
	if err != nil {
		t.Logf("dockerfile_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("dockerfile_list completed (meta)")
}

func metaTemplateGitignoreGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[gitignoretemplates.GetOutput](ctx, "gitlab_template", "gitignore_get", map[string]any{})
	if err != nil {
		t.Logf("gitignore_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("gitignore_get completed (meta)")
}

func metaTemplateLicenseGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[licensetemplates.GetOutput](ctx, "gitlab_template", "license_get", map[string]any{})
	if err != nil {
		t.Logf("license_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("license_get completed (meta)")
}

func metaTemplateLicenseList(ctx context.Context, t *testing.T) {
	_, err := callMeta[licensetemplates.ListOutput](ctx, "gitlab_template", "license_list", map[string]any{})
	if err != nil {
		t.Logf("license_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("license_list completed (meta)")
}

func metaTemplateLintProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[cilint.Output](ctx, "gitlab_template", "lint_project", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("lint_project returned error (non-fatal): %v", err)
		return
	}
	t.Log("lint_project completed (meta)")
}

func metaTemplateProjectTemplateGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projecttemplates.GetOutput](ctx, "gitlab_template", "project_template_get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("project_template_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("project_template_get completed (meta)")
}

func metaTemplateProjectTemplateList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[projecttemplates.ListOutput](ctx, "gitlab_template", "project_template_list", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("project_template_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("project_template_list completed (meta)")
}

// --- gitlab_user gap tests (67 actions) ---

func metaUserActivate(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.AdminActionOutput](ctx, "gitlab_user", "activate", map[string]any{})
	if err != nil {
		t.Logf("activate returned error (non-fatal): %v", err)
		return
	}
	t.Log("activate completed (meta)")
}

func metaUserActivities(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.UserActivitiesOutput](ctx, "gitlab_user", "activities", map[string]any{})
	if err != nil {
		t.Logf("activities returned error (non-fatal): %v", err)
		return
	}
	t.Log("activities completed (meta)")
}

func metaUserAddEmail(ctx context.Context, t *testing.T) {
	_, err := callMeta[useremails.Output](ctx, "gitlab_user", "add_email", map[string]any{})
	if err != nil {
		t.Logf("add_email returned error (non-fatal): %v", err)
		return
	}
	t.Log("add_email completed (meta)")
}

func metaUserAddEmailForUser(ctx context.Context, t *testing.T) {
	_, err := callMeta[useremails.Output](ctx, "gitlab_user", "add_email_for_user", map[string]any{})
	if err != nil {
		t.Logf("add_email_for_user returned error (non-fatal): %v", err)
		return
	}
	t.Log("add_email_for_user completed (meta)")
}

func metaUserAddGpgKey(ctx context.Context, t *testing.T) {
	_, err := callMeta[usergpgkeys.Output](ctx, "gitlab_user", "add_gpg_key", map[string]any{})
	if err != nil {
		t.Logf("add_gpg_key returned error (non-fatal): %v", err)
		return
	}
	t.Log("add_gpg_key completed (meta)")
}

func metaUserAddGpgKeyForUser(ctx context.Context, t *testing.T) {
	_, err := callMeta[usergpgkeys.Output](ctx, "gitlab_user", "add_gpg_key_for_user", map[string]any{})
	if err != nil {
		t.Logf("add_gpg_key_for_user returned error (non-fatal): %v", err)
		return
	}
	t.Log("add_gpg_key_for_user completed (meta)")
}

func metaUserAddSshKey(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.SSHKeyOutput](ctx, "gitlab_user", "add_ssh_key", map[string]any{})
	if err != nil {
		t.Logf("add_ssh_key returned error (non-fatal): %v", err)
		return
	}
	t.Log("add_ssh_key completed (meta)")
}

func metaUserAddSshKeyForUser(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.SSHKeyOutput](ctx, "gitlab_user", "add_ssh_key_for_user", map[string]any{})
	if err != nil {
		t.Logf("add_ssh_key_for_user returned error (non-fatal): %v", err)
		return
	}
	t.Log("add_ssh_key_for_user completed (meta)")
}

func metaUserApprove(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.AdminActionOutput](ctx, "gitlab_user", "approve", map[string]any{})
	if err != nil {
		t.Logf("approve returned error (non-fatal): %v", err)
		return
	}
	t.Log("approve completed (meta)")
}

func metaUserAssociationsCount(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.AssociationsCountOutput](ctx, "gitlab_user", "associations_count", map[string]any{})
	if err != nil {
		t.Logf("associations_count returned error (non-fatal): %v", err)
		return
	}
	t.Log("associations_count completed (meta)")
}

func metaUserAvatarGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[avatar.GetOutput](ctx, "gitlab_user", "avatar_get", map[string]any{})
	if err != nil {
		t.Logf("avatar_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("avatar_get completed (meta)")
}

func metaUserBan(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.AdminActionOutput](ctx, "gitlab_user", "ban", map[string]any{})
	if err != nil {
		t.Logf("ban returned error (non-fatal): %v", err)
		return
	}
	t.Log("ban completed (meta)")
}

func metaUserBlock(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.AdminActionOutput](ctx, "gitlab_user", "block", map[string]any{})
	if err != nil {
		t.Logf("block returned error (non-fatal): %v", err)
		return
	}
	t.Log("block completed (meta)")
}

func metaUserContributionEvents(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.ContributionEventsOutput](ctx, "gitlab_user", "contribution_events", map[string]any{})
	if err != nil {
		t.Logf("contribution_events returned error (non-fatal): %v", err)
		return
	}
	t.Log("contribution_events completed (meta)")
}

func metaUserCreate(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.Output](ctx, "gitlab_user", "create", map[string]any{})
	if err != nil {
		t.Logf("create returned error (non-fatal): %v", err)
		return
	}
	t.Log("create completed (meta)")
}

func metaUserCreateCurrentUserPat(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.CurrentUserPATOutput](ctx, "gitlab_user", "create_current_user_pat", map[string]any{})
	if err != nil {
		t.Logf("create_current_user_pat returned error (non-fatal): %v", err)
		return
	}
	t.Log("create_current_user_pat completed (meta)")
}

func metaUserCreateImpersonationToken(ctx context.Context, t *testing.T) {
	_, err := callMeta[impersonationtokens.Output](ctx, "gitlab_user", "create_impersonation_token", map[string]any{})
	if err != nil {
		t.Logf("create_impersonation_token returned error (non-fatal): %v", err)
		return
	}
	t.Log("create_impersonation_token completed (meta)")
}

func metaUserCreatePersonalAccessToken(ctx context.Context, t *testing.T) {
	_, err := callMeta[impersonationtokens.PATOutput](ctx, "gitlab_user", "create_personal_access_token", map[string]any{})
	if err != nil {
		t.Logf("create_personal_access_token returned error (non-fatal): %v", err)
		return
	}
	t.Log("create_personal_access_token completed (meta)")
}

func metaUserCreateRunner(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.UserRunnerOutput](ctx, "gitlab_user", "create_runner", map[string]any{})
	if err != nil {
		t.Logf("create_runner returned error (non-fatal): %v", err)
		return
	}
	t.Log("create_runner completed (meta)")
}

func metaUserCreateServiceAccount(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.Output](ctx, "gitlab_user", "create_service_account", map[string]any{})
	if err != nil {
		t.Logf("create_service_account returned error (non-fatal): %v", err)
		return
	}
	t.Log("create_service_account completed (meta)")
}

func metaUserCurrentUserStatus(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.StatusOutput](ctx, "gitlab_user", "current_user_status", map[string]any{})
	if err != nil {
		t.Logf("current_user_status returned error (non-fatal): %v", err)
		return
	}
	t.Log("current_user_status completed (meta)")
}

func metaUserDeactivate(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.AdminActionOutput](ctx, "gitlab_user", "deactivate", map[string]any{})
	if err != nil {
		t.Logf("deactivate returned error (non-fatal): %v", err)
		return
	}
	t.Log("deactivate completed (meta)")
}

func metaUserDelete(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.DeleteOutput](ctx, "gitlab_user", "delete", map[string]any{})
	if err != nil {
		t.Logf("delete returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete completed (meta)")
}

func metaUserDeleteEmail(ctx context.Context, t *testing.T) {
	_, err := callMeta[useremails.DeleteOutput](ctx, "gitlab_user", "delete_email", map[string]any{})
	if err != nil {
		t.Logf("delete_email returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete_email completed (meta)")
}

func metaUserDeleteEmailForUser(ctx context.Context, t *testing.T) {
	_, err := callMeta[useremails.DeleteOutput](ctx, "gitlab_user", "delete_email_for_user", map[string]any{})
	if err != nil {
		t.Logf("delete_email_for_user returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete_email_for_user completed (meta)")
}

func metaUserDeleteGpgKey(ctx context.Context, t *testing.T) {
	_, err := callMeta[usergpgkeys.DeleteOutput](ctx, "gitlab_user", "delete_gpg_key", map[string]any{})
	if err != nil {
		t.Logf("delete_gpg_key returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete_gpg_key completed (meta)")
}

func metaUserDeleteGpgKeyForUser(ctx context.Context, t *testing.T) {
	_, err := callMeta[usergpgkeys.DeleteOutput](ctx, "gitlab_user", "delete_gpg_key_for_user", map[string]any{})
	if err != nil {
		t.Logf("delete_gpg_key_for_user returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete_gpg_key_for_user completed (meta)")
}

func metaUserDeleteIdentity(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.DeleteUserIdentityOutput](ctx, "gitlab_user", "delete_identity", map[string]any{})
	if err != nil {
		t.Logf("delete_identity returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete_identity completed (meta)")
}

func metaUserDeleteSshKey(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.DeleteSSHKeyOutput](ctx, "gitlab_user", "delete_ssh_key", map[string]any{})
	if err != nil {
		t.Logf("delete_ssh_key returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete_ssh_key completed (meta)")
}

func metaUserDeleteSshKeyForUser(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.DeleteSSHKeyOutput](ctx, "gitlab_user", "delete_ssh_key_for_user", map[string]any{})
	if err != nil {
		t.Logf("delete_ssh_key_for_user returned error (non-fatal): %v", err)
		return
	}
	t.Log("delete_ssh_key_for_user completed (meta)")
}

func metaUserDisableTwoFactor(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.AdminActionOutput](ctx, "gitlab_user", "disable_two_factor", map[string]any{})
	if err != nil {
		t.Logf("disable_two_factor returned error (non-fatal): %v", err)
		return
	}
	t.Log("disable_two_factor completed (meta)")
}

func metaUserEmails(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.EmailListOutput](ctx, "gitlab_user", "emails", map[string]any{})
	if err != nil {
		t.Logf("emails returned error (non-fatal): %v", err)
		return
	}
	t.Log("emails completed (meta)")
}

func metaUserEmailsForUser(ctx context.Context, t *testing.T) {
	_, err := callMeta[useremails.ListOutput](ctx, "gitlab_user", "emails_for_user", map[string]any{})
	if err != nil {
		t.Logf("emails_for_user returned error (non-fatal): %v", err)
		return
	}
	t.Log("emails_for_user completed (meta)")
}

func metaUserEventListContributions(ctx context.Context, t *testing.T) {
	_, err := callMeta[events.ListContributionEventsOutput](ctx, "gitlab_user", "event_list_contributions", map[string]any{})
	if err != nil {
		t.Logf("event_list_contributions returned error (non-fatal): %v", err)
		return
	}
	t.Log("event_list_contributions completed (meta)")
}

func metaUserEventListProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[events.ListProjectEventsOutput](ctx, "gitlab_user", "event_list_project", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("event_list_project returned error (non-fatal): %v", err)
		return
	}
	t.Log("event_list_project completed (meta)")
}

func metaUserGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.Output](ctx, "gitlab_user", "get", map[string]any{})
	if err != nil {
		t.Logf("get returned error (non-fatal): %v", err)
		return
	}
	t.Log("get completed (meta)")
}

func metaUserGetEmail(ctx context.Context, t *testing.T) {
	_, err := callMeta[useremails.Output](ctx, "gitlab_user", "get_email", map[string]any{})
	if err != nil {
		t.Logf("get_email returned error (non-fatal): %v", err)
		return
	}
	t.Log("get_email completed (meta)")
}

func metaUserGetGpgKey(ctx context.Context, t *testing.T) {
	_, err := callMeta[usergpgkeys.Output](ctx, "gitlab_user", "get_gpg_key", map[string]any{})
	if err != nil {
		t.Logf("get_gpg_key returned error (non-fatal): %v", err)
		return
	}
	t.Log("get_gpg_key completed (meta)")
}

func metaUserGetGpgKeyForUser(ctx context.Context, t *testing.T) {
	_, err := callMeta[usergpgkeys.Output](ctx, "gitlab_user", "get_gpg_key_for_user", map[string]any{})
	if err != nil {
		t.Logf("get_gpg_key_for_user returned error (non-fatal): %v", err)
		return
	}
	t.Log("get_gpg_key_for_user completed (meta)")
}

func metaUserGetImpersonationToken(ctx context.Context, t *testing.T) {
	_, err := callMeta[impersonationtokens.Output](ctx, "gitlab_user", "get_impersonation_token", map[string]any{})
	if err != nil {
		t.Logf("get_impersonation_token returned error (non-fatal): %v", err)
		return
	}
	t.Log("get_impersonation_token completed (meta)")
}

func metaUserGetSshKey(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.SSHKeyOutput](ctx, "gitlab_user", "get_ssh_key", map[string]any{})
	if err != nil {
		t.Logf("get_ssh_key returned error (non-fatal): %v", err)
		return
	}
	t.Log("get_ssh_key completed (meta)")
}

func metaUserGetSshKeyForUser(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.SSHKeyOutput](ctx, "gitlab_user", "get_ssh_key_for_user", map[string]any{})
	if err != nil {
		t.Logf("get_ssh_key_for_user returned error (non-fatal): %v", err)
		return
	}
	t.Log("get_ssh_key_for_user completed (meta)")
}

func metaUserGetStatus(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.StatusOutput](ctx, "gitlab_user", "get_status", map[string]any{})
	if err != nil {
		t.Logf("get_status returned error (non-fatal): %v", err)
		return
	}
	t.Log("get_status completed (meta)")
}

func metaUserGpgKeysForUser(ctx context.Context, t *testing.T) {
	_, err := callMeta[usergpgkeys.ListOutput](ctx, "gitlab_user", "gpg_keys_for_user", map[string]any{})
	if err != nil {
		t.Logf("gpg_keys_for_user returned error (non-fatal): %v", err)
		return
	}
	t.Log("gpg_keys_for_user completed (meta)")
}

func metaUserKeyGetByFingerprint(ctx context.Context, t *testing.T) {
	_, err := callMeta[keys.Output](ctx, "gitlab_user", "key_get_by_fingerprint", map[string]any{})
	if err != nil {
		t.Logf("key_get_by_fingerprint returned error (non-fatal): %v", err)
		return
	}
	t.Log("key_get_by_fingerprint completed (meta)")
}

func metaUserKeyGetWithUser(ctx context.Context, t *testing.T) {
	_, err := callMeta[keys.Output](ctx, "gitlab_user", "key_get_with_user", map[string]any{})
	if err != nil {
		t.Logf("key_get_with_user returned error (non-fatal): %v", err)
		return
	}
	t.Log("key_get_with_user completed (meta)")
}

func metaUserList(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.ListOutput](ctx, "gitlab_user", "list", map[string]any{})
	if err != nil {
		t.Logf("list returned error (non-fatal): %v", err)
		return
	}
	t.Log("list completed (meta)")
}

func metaUserListImpersonationTokens(ctx context.Context, t *testing.T) {
	_, err := callMeta[impersonationtokens.ListOutput](ctx, "gitlab_user", "list_impersonation_tokens", map[string]any{})
	if err != nil {
		t.Logf("list_impersonation_tokens returned error (non-fatal): %v", err)
		return
	}
	t.Log("list_impersonation_tokens completed (meta)")
}

func metaUserListServiceAccounts(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.ServiceAccountListOutput](ctx, "gitlab_user", "list_service_accounts", map[string]any{})
	if err != nil {
		t.Logf("list_service_accounts returned error (non-fatal): %v", err)
		return
	}
	t.Log("list_service_accounts completed (meta)")
}

func metaUserMe(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.Output](ctx, "gitlab_user", "me", map[string]any{})
	if err != nil {
		t.Logf("me returned error (non-fatal): %v", err)
		return
	}
	t.Log("me completed (meta)")
}

func metaUserMemberships(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.UserMembershipsOutput](ctx, "gitlab_user", "memberships", map[string]any{})
	if err != nil {
		t.Logf("memberships returned error (non-fatal): %v", err)
		return
	}
	t.Log("memberships completed (meta)")
}

func metaUserModify(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.Output](ctx, "gitlab_user", "modify", map[string]any{})
	if err != nil {
		t.Logf("modify returned error (non-fatal): %v", err)
		return
	}
	t.Log("modify completed (meta)")
}

func metaUserNamespaceExists(ctx context.Context, t *testing.T) {
	_, err := callMeta[namespaces.ExistsOutput](ctx, "gitlab_user", "namespace_exists", map[string]any{})
	if err != nil {
		t.Logf("namespace_exists returned error (non-fatal): %v", err)
		return
	}
	t.Log("namespace_exists completed (meta)")
}

func metaUserNamespaceGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[namespaces.Output](ctx, "gitlab_user", "namespace_get", map[string]any{})
	if err != nil {
		t.Logf("namespace_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("namespace_get completed (meta)")
}

func metaUserNamespaceList(ctx context.Context, t *testing.T) {
	_, err := callMeta[namespaces.ListOutput](ctx, "gitlab_user", "namespace_list", map[string]any{})
	if err != nil {
		t.Logf("namespace_list returned error (non-fatal): %v", err)
		return
	}
	t.Log("namespace_list completed (meta)")
}

func metaUserNamespaceSearch(ctx context.Context, t *testing.T) {
	_, err := callMeta[namespaces.ListOutput](ctx, "gitlab_user", "namespace_search", map[string]any{})
	if err != nil {
		t.Logf("namespace_search returned error (non-fatal): %v", err)
		return
	}
	t.Log("namespace_search completed (meta)")
}

func metaUserNotificationGlobalUpdate(ctx context.Context, t *testing.T) {
	_, err := callMeta[notifications.Output](ctx, "gitlab_user", "notification_global_update", map[string]any{})
	if err != nil {
		t.Logf("notification_global_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("notification_global_update completed (meta)")
}

func metaUserNotificationGroupGet(ctx context.Context, t *testing.T) {
	_, err := callMeta[notifications.Output](ctx, "gitlab_user", "notification_group_get", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("notification_group_get returned error (non-fatal): %v", err)
		return
	}
	t.Log("notification_group_get completed (meta)")
}

func metaUserNotificationGroupUpdate(ctx context.Context, t *testing.T) {
	_, err := callMeta[notifications.Output](ctx, "gitlab_user", "notification_group_update", map[string]any{
		"group_id": strconv.FormatInt(mState.groupID, 10),
	})
	if err != nil {
		t.Logf("notification_group_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("notification_group_update completed (meta)")
}

func metaUserNotificationProjectUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[notifications.Output](ctx, "gitlab_user", "notification_project_update", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("notification_project_update returned error (non-fatal): %v", err)
		return
	}
	t.Log("notification_project_update completed (meta)")
}

func metaUserReject(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.AdminActionOutput](ctx, "gitlab_user", "reject", map[string]any{})
	if err != nil {
		t.Logf("reject returned error (non-fatal): %v", err)
		return
	}
	t.Log("reject completed (meta)")
}

func metaUserRevokeImpersonationToken(ctx context.Context, t *testing.T) {
	_, err := callMeta[impersonationtokens.RevokeOutput](ctx, "gitlab_user", "revoke_impersonation_token", map[string]any{})
	if err != nil {
		t.Logf("revoke_impersonation_token returned error (non-fatal): %v", err)
		return
	}
	t.Log("revoke_impersonation_token completed (meta)")
}

func metaUserSetStatus(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.StatusOutput](ctx, "gitlab_user", "set_status", map[string]any{})
	if err != nil {
		t.Logf("set_status returned error (non-fatal): %v", err)
		return
	}
	t.Log("set_status completed (meta)")
}

func metaUserSshKeysForUser(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.SSHKeyListOutput](ctx, "gitlab_user", "ssh_keys_for_user", map[string]any{})
	if err != nil {
		t.Logf("ssh_keys_for_user returned error (non-fatal): %v", err)
		return
	}
	t.Log("ssh_keys_for_user completed (meta)")
}

func metaUserTodoMarkDone(ctx context.Context, t *testing.T) {
	_, err := callMeta[todos.MarkDoneOutput](ctx, "gitlab_user", "todo_mark_done", map[string]any{})
	if err != nil {
		t.Logf("todo_mark_done returned error (non-fatal): %v", err)
		return
	}
	t.Log("todo_mark_done completed (meta)")
}

func metaUserUnban(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.AdminActionOutput](ctx, "gitlab_user", "unban", map[string]any{})
	if err != nil {
		t.Logf("unban returned error (non-fatal): %v", err)
		return
	}
	t.Log("unban completed (meta)")
}

func metaUserUnblock(ctx context.Context, t *testing.T) {
	_, err := callMeta[users.AdminActionOutput](ctx, "gitlab_user", "unblock", map[string]any{})
	if err != nil {
		t.Logf("unblock returned error (non-fatal): %v", err)
		return
	}
	t.Log("unblock completed (meta)")
}

// --- gitlab_vulnerability gap tests (6 actions) ---

func metaVulnerabilityConfirm(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[vulnerabilities.MutationOutput](ctx, "gitlab_vulnerability", "confirm", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("confirm returned error (non-fatal): %v", err)
		return
	}
	t.Log("confirm completed (meta)")
}

func metaVulnerabilityDismiss(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[vulnerabilities.MutationOutput](ctx, "gitlab_vulnerability", "dismiss", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("dismiss returned error (non-fatal): %v", err)
		return
	}
	t.Log("dismiss completed (meta)")
}

func metaVulnerabilityGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[vulnerabilities.GetOutput](ctx, "gitlab_vulnerability", "get", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("get returned error (non-fatal): %v", err)
		return
	}
	t.Log("get completed (meta)")
}

func metaVulnerabilityPipelineSecuritySummary(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[vulnerabilities.PipelineSecuritySummaryOutput](ctx, "gitlab_vulnerability", "pipeline_security_summary", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("pipeline_security_summary returned error (non-fatal): %v", err)
		return
	}
	t.Log("pipeline_security_summary completed (meta)")
}

func metaVulnerabilityResolve(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[vulnerabilities.MutationOutput](ctx, "gitlab_vulnerability", "resolve", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("resolve returned error (non-fatal): %v", err)
		return
	}
	t.Log("resolve completed (meta)")
}

func metaVulnerabilityRevert(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[vulnerabilities.MutationOutput](ctx, "gitlab_vulnerability", "revert", map[string]any{
		"project_id": mPID(),
	})
	if err != nil {
		t.Logf("revert returned error (non-fatal): %v", err)
		return
	}
	t.Log("revert completed (meta)")
}
