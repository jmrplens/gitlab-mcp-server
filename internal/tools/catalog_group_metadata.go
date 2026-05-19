package tools

import (
	_ "embed" // required by go:embed directives for tool snapshot JSON files
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/samplingtools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

//go:embed testdata/tools_meta.json
var metaToolSnapshotJSON []byte

//go:embed testdata/tools_individual.json
var individualToolSnapshotJSON []byte

type metaToolSnapshot struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

var catalogMetaToolDescriptions = loadCatalogMetaToolDescriptions()

var catalogIndividualToolDescriptions = loadCatalogIndividualToolDescriptions()

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

func loadCatalogIndividualToolDescriptions() map[string]string {
	var snapshots []metaToolSnapshot
	if err := json.Unmarshal(individualToolSnapshotJSON, &snapshots); err != nil {
		panic(fmt.Sprintf("load individual tool descriptions: %v", err))
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

func catalogGroupDescription(toolName string, _ toolutil.ActionMap) string {
	fullDescription := catalogMetaToolDescriptions[toolName]
	if fullDescription != "" {
		baseDescription := toolutil.StripMetaToolDescriptionPrefix(fullDescription)
		if strings.TrimSpace(baseDescription) != "" && baseDescription != fullDescription {
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

var catalogGroupIconsByToolName = map[string][]mcp.Icon{
	"gitlab_access":                toolutil.IconToken,
	"gitlab_admin":                 toolutil.IconConfig,
	"gitlab_analyze":               toolutil.IconAnalytics,
	"gitlab_attestation":           toolutil.IconShield,
	"gitlab_audit_event":           toolutil.IconAudit,
	"gitlab_branch":                toolutil.IconBranch,
	"gitlab_ci_catalog":            toolutil.IconTemplate,
	"gitlab_ci_variable":           toolutil.IconVariable,
	"gitlab_compliance_policy":     toolutil.IconCompliance,
	"gitlab_custom_emoji":          toolutil.IconEvent,
	"gitlab_dependency":            toolutil.IconPackage,
	"gitlab_dora_metrics":          toolutil.IconAnalytics,
	"gitlab_enterprise_user":       toolutil.IconUser,
	"gitlab_environment":           toolutil.IconEnvironment,
	"gitlab_external_status_check": toolutil.IconShield,
	"gitlab_feature_flags":         toolutil.IconConfig,
	"gitlab_geo":                   toolutil.IconInfra,
	"gitlab_group":                 toolutil.IconGroup,
	"gitlab_group_scim":            toolutil.IconGroup,
	"gitlab_issue":                 toolutil.IconIssue,
	"gitlab_job":                   toolutil.IconJob,
	"gitlab_member_role":           toolutil.IconConfig,
	"gitlab_merge_request":         toolutil.IconMR,
	"gitlab_merge_train":           toolutil.IconQueue,
	"gitlab_model_registry":        toolutil.IconPackage,
	"gitlab_mr_review":             toolutil.IconMR,
	"gitlab_orbit":                 toolutil.IconAnalytics,
	"gitlab_package":               toolutil.IconPackage,
	"gitlab_pipeline":              toolutil.IconPipeline,
	"gitlab_project":               toolutil.IconProject,
	"gitlab_project_alias":         toolutil.IconProject,
	"gitlab_release":               toolutil.IconRelease,
	"gitlab_repository":            toolutil.IconFile,
	"gitlab_runner":                toolutil.IconRunner,
	"gitlab_search":                toolutil.IconSearch,
	"gitlab_security_attribute":    toolutil.IconSecurity,
	"gitlab_security_category":     toolutil.IconSecurity,
	"gitlab_security_finding":      toolutil.IconSecurity,
	"gitlab_snippet":               toolutil.IconSnippet,
	"gitlab_storage_move":          toolutil.IconInfra,
	"gitlab_tag":                   toolutil.IconTag,
	"gitlab_template":              toolutil.IconTemplate,
	"gitlab_user":                  toolutil.IconUser,
	"gitlab_vulnerability":         toolutil.IconVulnerability,
	"gitlab_wiki":                  toolutil.IconWiki,
}

func catalogGroupIcons(toolName string) []mcp.Icon {
	if icons, ok := catalogGroupIconsByToolName[toolName]; ok {
		return icons
	}
	return toolutil.IconServer
}

func catalogGroupFormatResult(toolName string) toolutil.FormatResultFunc {
	if toolName == "gitlab_analyze" {
		return samplingtools.MetaMarkdownForResult
	}
	return nil
}
