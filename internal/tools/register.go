// register.go wires all individual GitLab MCP tools to the MCP server.
// Each register* function groups related tools by domain (projects, branches,
// tags, releases, merge requests, etc.) and binds them to handler closures
// that capture the GitLab client.
package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"

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
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/dependencyproxy"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deploykeys"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deploymentmergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deployments"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deploytokens"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/dockerfiletemplates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/dorametrics"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/elicitationtools"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/enterpriseusers"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/environments"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/epicdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/epicissues"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/epicnotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/epics"
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
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupanalytics"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupboards"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupcredentials"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupepicboards"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupimportexport"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupiterations"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/grouplabels"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupldap"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupmarkdownuploads"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupmembers"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupmilestones"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupprotectedbranches"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupprotectedenvs"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/grouprelationsexport"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupreleases"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupsaml"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupscim"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupserviceaccounts"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupsshcerts"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupstoragemoves"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupvariables"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupwikis"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/health"
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
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/markdown"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/memberroles"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/members"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergetrains"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/metadata"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/modelregistry"
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
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectdiscovery"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectimportexport"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectiterations"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectmirrors"
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
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/runnercontrollers"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/runnercontrollerscopes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/runnercontrollertokens"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/runners"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/samplingtools"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/search"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/securefiles"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/securityfindings"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/securitysettings"
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
)

// RegisterAll wires all GitLab MCP tools to the MCP server.
// Tool closures capture the client to inject it into each handler.
// When enterprise is false, Premium/Ultimate-only tool packages are not registered.
func RegisterAll(server *mcp.Server, client *gitlabclient.Client, enterprise bool) {
	projects.RegisterTools(server, client)
	projectimportexport.RegisterTools(server, client)
	uploads.RegisterTools(server, client)
	branches.RegisterTools(server, client)
	tags.RegisterTools(server, client)
	releases.RegisterTools(server, client)
	releaselinks.RegisterTools(server, client)
	mergerequests.RegisterTools(server, client)
	mrapprovals.RegisterTools(server, client)
	mrapprovalsettings.RegisterTools(server, client)
	mrnotes.RegisterTools(server, client)
	mrdiscussions.RegisterTools(server, client)
	mrchanges.RegisterTools(server, client)
	commits.RegisterTools(server, client)
	files.RegisterTools(server, client)
	members.RegisterTools(server, client)
	groups.RegisterTools(server, client)
	groupimportexport.RegisterTools(server, client)
	issues.RegisterTools(server, client)
	issuenotes.RegisterTools(server, client)
	pipelines.RegisterTools(server, client)
	labels.RegisterTools(server, client)
	milestones.RegisterTools(server, client)
	repository.RegisterTools(server, client)
	jobs.RegisterTools(server, client)
	search.RegisterTools(server, client)
	users.RegisterTools(server, client)
	usergpgkeys.RegisterTools(server, client)
	useremails.RegisterTools(server, client)
	impersonationtokens.RegisterTools(server, client)
	health.RegisterTools(server, client)
	samplingtools.RegisterTools(server, client)
	elicitationtools.RegisterTools(server, client)
	packages.RegisterTools(server, client)
	wikis.RegisterTools(server, client)
	todos.RegisterTools(server, client)
	mrdraftnotes.RegisterTools(server, client)
	environments.RegisterTools(server, client)
	deployments.RegisterTools(server, client)
	deploymentmergerequests.RegisterTools(server, client)
	pipelineschedules.RegisterTools(server, client)
	civariables.RegisterTools(server, client)
	issuelinks.RegisterTools(server, client)
	cilint.RegisterTools(server, client)
	runners.RegisterTools(server, client)
	runnercontrollers.RegisterTools(server, client)
	runnercontrollertokens.RegisterTools(server, client)
	runnercontrollerscopes.RegisterTools(server, client)
	accesstokens.RegisterTools(server, client)
	deploykeys.RegisterTools(server, client)
	deploytokens.RegisterTools(server, client)
	accessrequests.RegisterTools(server, client)
	containerregistry.RegisterTools(server, client)
	snippets.RegisterTools(server, client)
	boards.RegisterTools(server, client)
	groupboards.RegisterTools(server, client)
	featureflags.RegisterTools(server, client)
	ffuserlists.RegisterTools(server, client)
	pipelinetriggers.RegisterTools(server, client)
	groupmembers.RegisterTools(server, client)
	grouplabels.RegisterTools(server, client)
	groupmilestones.RegisterTools(server, client)
	groupvariables.RegisterTools(server, client)
	instancevariables.RegisterTools(server, client)
	protectedenvs.RegisterTools(server, client)
	protectedpackages.RegisterTools(server, client)
	namespaces.RegisterTools(server, client)
	events.RegisterTools(server, client)
	invites.RegisterTools(server, client)
	awardemoji.RegisterTools(server, client)
	notifications.RegisterTools(server, client)
	freezeperiods.RegisterTools(server, client)
	keys.RegisterTools(server, client)
	issuediscussions.RegisterTools(server, client)
	commitdiscussions.RegisterTools(server, client)
	snippetdiscussions.RegisterTools(server, client)
	snippetnotes.RegisterTools(server, client)
	jobtokenscope.RegisterTools(server, client)
	repositorysubmodules.RegisterTools(server, client)
	mrcontextcommits.RegisterTools(server, client)
	markdown.RegisterTools(server, client)
	workitems.RegisterTools(server, client)
	integrations.RegisterTools(server, client)
	badges.RegisterTools(server, client)
	topics.RegisterTools(server, client)
	settings.RegisterTools(server, client)
	appearance.RegisterTools(server, client)
	broadcastmessages.RegisterTools(server, client)
	features.RegisterTools(server, client)
	license.RegisterTools(server, client)
	systemhooks.RegisterTools(server, client)
	sidekiq.RegisterTools(server, client)
	planlimits.RegisterTools(server, client)
	usagedata.RegisterTools(server, client)
	dbmigrations.RegisterTools(server, client)
	applications.RegisterTools(server, client)
	appstatistics.RegisterTools(server, client)
	metadata.RegisterTools(server, client)
	customattributes.RegisterTools(server, client)
	bulkimports.RegisterTools(server, client)
	ciyamltemplates.RegisterTools(server, client)
	dockerfiletemplates.RegisterTools(server, client)
	gitignoretemplates.RegisterTools(server, client)
	licensetemplates.RegisterTools(server, client)
	projecttemplates.RegisterTools(server, client)
	issuestatistics.RegisterTools(server, client)
	projectstatistics.RegisterTools(server, client)
	errortracking.RegisterTools(server, client)
	alertmanagement.RegisterTools(server, client)
	securefiles.RegisterTools(server, client)
	terraformstates.RegisterTools(server, client)
	clusteragents.RegisterTools(server, client)
	resourcegroups.RegisterTools(server, client)
	avatar.RegisterTools(server, client)
	dependencyproxy.RegisterTools(server, client)
	grouprelationsexport.RegisterTools(server, client)
	groupmarkdownuploads.RegisterTools(server, client)
	importservice.RegisterTools(server, client)
	pages.RegisterTools(server, client)
	resourceevents.RegisterTools(server, client)
	projectdiscovery.RegisterTools(server, client)

	// Free-tier tools (available on CE — GraphQL/REST based)
	cicatalog.RegisterTools(server, client)
	branchrules.RegisterTools(server, client)
	customemoji.RegisterTools(server, client)
	modelregistry.RegisterTools(server, client)
	projectstoragemoves.RegisterTools(server, client)
	snippetstoragemoves.RegisterTools(server, client)

	// Free-tier tools (previously enterprise-gated, verified Free via GitLab docs and E2E on CE)
	projectmirrors.RegisterTools(server, client)
	groupreleases.RegisterTools(server, client)

	// Enterprise tools (Premium/Ultimate — gated by GITLAB_ENTERPRISE)
	if enterprise {
		groupserviceaccounts.RegisterTools(server, client)
		projects.RegisterPushRuleTools(server, client)
		mergetrains.RegisterTools(server, client)
		auditevents.RegisterTools(server, client)
		dorametrics.RegisterTools(server, client)
		dependencies.RegisterTools(server, client)
		externalstatuschecks.RegisterTools(server, client)
		groupscim.RegisterTools(server, client)
		memberroles.RegisterTools(server, client)
		enterpriseusers.RegisterTools(server, client)
		attestations.RegisterTools(server, client)
		compliancepolicy.RegisterTools(server, client)
		projectaliases.RegisterTools(server, client)
		geo.RegisterTools(server, client)
		groupstoragemoves.RegisterTools(server, client)
		vulnerabilities.RegisterTools(server, client)
		securityfindings.RegisterTools(server, client)
		securitysettings.RegisterTools(server, client)
		groupanalytics.RegisterTools(server, client)
		groupcredentials.RegisterTools(server, client)
		groupsshcerts.RegisterTools(server, client)
		projectiterations.RegisterTools(server, client)
		groupiterations.RegisterTools(server, client)
		epics.RegisterTools(server, client)
		epicissues.RegisterTools(server, client)
		epicnotes.RegisterTools(server, client)
		epicdiscussions.RegisterTools(server, client)
		groupepicboards.RegisterTools(server, client)
		groupwikis.RegisterTools(server, client)
		groupprotectedbranches.RegisterTools(server, client)
		groupprotectedenvs.RegisterTools(server, client)
		groupldap.RegisterTools(server, client)
		groupsaml.RegisterTools(server, client)
		users.RegisterEnterpriseTools(server, client)
	}
}
