package tools

import (
	_ "embed" // required by go:embed directives for tool snapshot JSON files
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

//go:embed testdata/tools_meta.json
var metaToolSnapshotJSON []byte

//go:embed testdata/tools_individual.json
var individualToolSnapshotJSON []byte

// metaToolSnapshot is a single row of the embedded description snapshot
// files. Each row maps a tool name to the curated description used by
// the schema resource.
type metaToolSnapshot struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// catalogMetaToolDescriptions is the in-memory map keyed by meta-tool
// name that the catalog builder consults before falling back to the
// auto-derived description. Populated from metaToolSnapshotJSON at
// package init.
var catalogMetaToolDescriptions = loadCatalogMetaToolDescriptions()

// catalogIndividualToolDescriptions is the in-memory map keyed by
// individual tool name that the catalog builder consults for the
// individual-tool surface. Populated from individualToolSnapshotJSON at
// package init.
var catalogIndividualToolDescriptions = loadCatalogIndividualToolDescriptions()

// loadCatalogMetaToolDescriptions parses metaToolSnapshotJSON and
// returns the meta-tool name to curated description map. Snapshot rows
// missing a name or description are skipped so generated-data corruption
// does not silently strip descriptions. Panics on invalid JSON to
// surface broken embeds at startup.
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

// loadCatalogIndividualToolDescriptions parses
// individualToolSnapshotJSON and returns the individual tool name to
// curated description map. Behaves like
// [loadCatalogMetaToolDescriptions]: skips incomplete rows and panics
// on invalid JSON so the server fails fast on broken embeds.
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

// catalogGroupDescription returns the curated base description for a
// meta-tool group, stripping the runtime-added "Action params schema"
// prefix when present. Falls back to a deterministic "<domain> actions"
// sentence when the curated snapshot is missing or its stripped form
// is empty.
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

// catalogGroupReadOnly reports whether every spec in the slice is
// read-only. Returns false for an empty input because a group with no
// actions cannot be classified as read-only.
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

// catalogGroupSurfaceKind returns the catalog [actioncatalog.SurfaceKind]
// for a meta-tool group. Every meta-tool group is a regular meta group.
func catalogGroupSurfaceKind(_ string) actioncatalog.SurfaceKind {
	return actioncatalog.SurfaceKindMetaGroup
}

// catalogGroupCapabilityRequirements returns the MCP capability
// requirements for a meta-tool group. No group has capability
// requirements (the sampling-backed group was removed).
func catalogGroupCapabilityRequirements(_ string) []string {
	return nil
}

// catalogGroupIconsByToolName is the canonical icon mapping used by
// [catalogGroupIcons]. Each entry picks a domain-appropriate icon from
// [toolutil] so the same icon is reused across the meta and individual
// surfaces for the same domain.
var catalogGroupIconsByToolName = map[string][]mcp.Icon{
	"gitlab_access":                toolutil.IconToken,
	"gitlab_admin":                 toolutil.IconConfig,
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

// catalogGroupIcons returns the MCP icons for a meta-tool group. Falls
// back to [toolutil.IconServer] when the group is not in
// [catalogGroupIconsByToolName] so unknown groups still expose a
// visible icon on registration.
func catalogGroupIcons(toolName string) []mcp.Icon {
	if icons, ok := catalogGroupIconsByToolName[toolName]; ok {
		return icons
	}
	return toolutil.IconServer
}

// catalogGroupFormatResult returns the [toolutil.FormatResultFunc] used
// by a meta-tool group. No group overrides the default dispatcher.
func catalogGroupFormatResult(_ string) toolutil.FormatResultFunc {
	return nil
}
