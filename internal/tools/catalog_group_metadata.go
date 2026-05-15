package tools

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/samplingtools"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

//go:embed testdata/tools_meta.json
var metaToolSnapshotJSON []byte

type metaToolSnapshot struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

var catalogMetaToolDescriptions = loadCatalogMetaToolDescriptions()

func loadCatalogMetaToolDescriptions() map[string]string {
	var snapshots []metaToolSnapshot
	if err := json.Unmarshal(metaToolSnapshotJSON, &snapshots); err != nil {
		panic(fmt.Sprintf("load meta-tool descriptions: %v", err))
	}
	descriptions := make(map[string]string, len(snapshots))
	for _, snapshot := range snapshots {
		if snapshot.Name == "" || snapshot.Description == "" {
			continue
		}
		descriptions[snapshot.Name] = snapshot.Description
	}
	return descriptions
}

func catalogGroupDescription(toolName string, routes toolutil.ActionMap) string {
	fullDescription := catalogMetaToolDescriptions[toolName]
	if fullDescription != "" {
		prefix := toolutil.MetaToolDescriptionPrefix(toolName, routes)
		if baseDescription, ok := strings.CutPrefix(fullDescription, prefix); ok {
			return baseDescription
		}
	}
	return fmt.Sprintf("GitLab %s actions.", strings.ReplaceAll(strings.TrimPrefix(toolName, "gitlab_"), "_", " "))
}

func catalogGroupReadOnly(specs []toolutil.ActionSpec) bool {
	if len(specs) == 0 {
		return false
	}
	for _, spec := range specs {
		if !spec.ReadOnly {
			return false
		}
	}
	return true
}

func catalogGroupSurfaceKind(toolName string) actioncatalog.SurfaceKind {
	if toolName == "gitlab_analyze" {
		return actioncatalog.SurfaceKindSamplingUtility
	}
	return actioncatalog.SurfaceKindMetaGroup
}

func catalogGroupCapabilityRequirements(toolName string) []string {
	if toolName == "gitlab_analyze" {
		return []string{"sampling"}
	}
	return nil
}

func catalogGroupIcons(toolName string) []mcp.Icon {
	switch toolName {
	case "gitlab_access":
		return toolutil.IconToken
	case "gitlab_admin":
		return toolutil.IconConfig
	case "gitlab_analyze", "gitlab_dora_metrics", "gitlab_orbit":
		return toolutil.IconAnalytics
	case "gitlab_attestation", "gitlab_external_status_check":
		return toolutil.IconShield
	case "gitlab_audit_event":
		return toolutil.IconAudit
	case "gitlab_branch":
		return toolutil.IconBranch
	case "gitlab_ci_catalog", "gitlab_template":
		return toolutil.IconTemplate
	case "gitlab_ci_variable":
		return toolutil.IconVariable
	case "gitlab_compliance_policy":
		return toolutil.IconCompliance
	case "gitlab_custom_emoji":
		return toolutil.IconEvent
	case "gitlab_dependency", "gitlab_model_registry", "gitlab_package":
		return toolutil.IconPackage
	case "gitlab_enterprise_user", "gitlab_user":
		return toolutil.IconUser
	case "gitlab_environment":
		return toolutil.IconEnvironment
	case "gitlab_feature_flags", "gitlab_member_role":
		return toolutil.IconConfig
	case "gitlab_geo", "gitlab_storage_move":
		return toolutil.IconInfra
	case "gitlab_group", "gitlab_group_scim":
		return toolutil.IconGroup
	case "gitlab_issue":
		return toolutil.IconIssue
	case "gitlab_job":
		return toolutil.IconJob
	case "gitlab_merge_request", "gitlab_mr_review":
		return toolutil.IconMR
	case "gitlab_merge_train":
		return toolutil.IconQueue
	case "gitlab_pipeline":
		return toolutil.IconPipeline
	case "gitlab_project", "gitlab_project_alias":
		return toolutil.IconProject
	case "gitlab_release":
		return toolutil.IconRelease
	case "gitlab_repository":
		return toolutil.IconFile
	case "gitlab_runner":
		return toolutil.IconRunner
	case "gitlab_search":
		return toolutil.IconSearch
	case "gitlab_security_finding":
		return toolutil.IconSecurity
	case "gitlab_snippet":
		return toolutil.IconSnippet
	case "gitlab_tag":
		return toolutil.IconTag
	case "gitlab_vulnerability":
		return toolutil.IconVulnerability
	case "gitlab_wiki":
		return toolutil.IconWiki
	default:
		return toolutil.IconServer
	}
}

func catalogGroupFormatResult(toolName string) toolutil.FormatResultFunc {
	if toolName == "gitlab_analyze" {
		return samplingtools.MetaMarkdownForResult
	}
	return nil
}
