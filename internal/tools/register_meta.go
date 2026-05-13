package tools

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/elicitationtools"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/orbit"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectdiscovery"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/runners"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/samplingtools"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/search"
)

// RegisterAllMeta wires meta-tools to the MCP server.
// Base: 32 tools = 28 meta-tools (24 inline + 3 delegated + 1 standalone) +
// 4 standalone interactive elicitation tools (gitlab_interactive_*).
// Enterprise: +15 inline meta-tools = 47 tools total; GitLab.com Enterprise also adds gitlab_orbit.
// Each meta-tool dispatches to the underlying handler based on the "action"
// parameter. This reduces token usage for LLMs while preserving full
// functionality. Interactive tools cannot be consolidated because they
// require multi-round MCP elicitation/create exchanges with the client.
// Returns an error if the action catalog cannot be built or if wiring tools
// to the MCP server fails.
func RegisterAllMeta(server *mcp.Server, client *gitlabclient.Client, enterprise bool) error {
	catalog, err := BuildActionCatalog(client, ActionCatalogOptions{Enterprise: enterprise})
	if err != nil {
		return fmt.Errorf("failed to build action catalog: %w", err)
	}
	RegisterMetaCatalog(server, catalog)
	RegisterMetaStandaloneTools(server, client)
	return nil
}

// RegisterMetaStandaloneTools wires standalone utility tools that remain visible
// alongside the catalog-backed meta-tools.
func RegisterMetaStandaloneTools(server *mcp.Server, client *gitlabclient.Client) {
	registerStandaloneUtilities(server, client)
}

func registerAllMetaGroups(server *mcp.Server, client *gitlabclient.Client, enterprise bool) {
	// Core domain meta-tools (inline handlers — enterprise routes injected when enabled)
	registerProjectMeta(server, client, enterprise)
	registerBranchMeta(server, client)
	registerTagMeta(server, client)
	registerReleaseMeta(server, client)
	registerMergeRequestMeta(server, client)
	registerMRReviewMeta(server, client)
	registerRepositoryMeta(server, client)
	registerGroupMeta(server, client, enterprise)
	registerIssueMeta(server, client, enterprise)
	registerPipelineMeta(server, client)
	registerJobMeta(server, client)
	registerUserMeta(server, client, enterprise)
	registerWikiMeta(server, client)
	registerEnvironmentMeta(server, client)
	registerCIVariableMeta(server, client)
	registerTemplateMeta(server, client)
	registerAdminMeta(server, client)

	// Consolidated domain meta-tools (inline handlers)
	registerAccessMeta(server, client)
	registerPackageMeta(server, client)
	registerSnippetMeta(server, client)
	registerFeatureFlagsMeta(server, client)

	// Free-tier meta-tools (available on CE — GraphQL/REST based)
	registerModelRegistryMeta(server, client)
	registerCICatalogMeta(server, client)
	registerCustomEmojiMeta(server, client)

	// Enterprise meta-tools (Premium/Ultimate — gated by GITLAB_ENTERPRISE)
	if enterprise {
		registerMergeTrainMeta(server, client)
		registerAuditEventMeta(server, client)
		registerDORAMetricsMeta(server, client)
		registerDependencyMeta(server, client)
		registerExternalStatusCheckMeta(server, client)
		registerGroupSCIMMeta(server, client)
		registerMemberRoleMeta(server, client)
		registerEnterpriseUserMeta(server, client)
		registerAttestationMeta(server, client)
		registerCompliancePolicyMeta(server, client)
		registerProjectAliasMeta(server, client)
		registerGeoMeta(server, client)
		registerStorageMoveMeta(server, client)
		registerVulnerabilityMeta(server, client)
		registerSecurityFindingsMeta(server, client)
		if client.IsGitLabDotCom() {
			orbit.RegisterMeta(server, client)
		}
	}

	// Delegated meta-tools (sub-package RegisterMeta)
	search.RegisterMeta(server, client)
	runners.RegisterMeta(server, client)
	samplingtools.RegisterMeta(server, client)
}

func registerStandaloneUtilities(server *mcp.Server, client *gitlabclient.Client) {
	// Standalone utility tools (not consolidated into meta-tools).
	// projectdiscovery: git-remote → project resolution helper.
	// elicitationtools: 4 gitlab_interactive_* tools that drive multi-step MCP
	// elicitation flows. They cannot be folded into an action+params meta-tool
	// because each step requires a separate elicitation/create round-trip with
	// the client. They degrade gracefully on clients without the elicitation
	// capability via UnsupportedResult (IsError: true).
	projectdiscovery.RegisterTools(server, client)
	elicitationtools.RegisterTools(server, client)
}
