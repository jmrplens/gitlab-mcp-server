package groups

import (
	"context"
	"errors"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ListInput defines parameters for listing groups.
type ListInput struct {
	Search               string            `json:"search,omitempty"                jsonschema:"Filter groups by name or path"`
	Owned                bool              `json:"owned,omitempty"                 jsonschema:"Limit to groups explicitly owned by the authenticated user"`
	TopLevelOnly         bool              `json:"top_level_only,omitempty"        jsonschema:"Limit to top-level groups (exclude subgroups)"`
	OrderBy              string            `json:"order_by,omitempty"              jsonschema:"Order groups by field (name, path, id, similarity)"`
	Sort                 string            `json:"sort,omitempty"                  jsonschema:"Sort direction (asc, desc)"`
	Visibility           string            `json:"visibility,omitempty"            jsonschema:"Filter by visibility (public, internal, private)"`
	AllAvailable         bool              `json:"all_available,omitempty"         jsonschema:"Show all groups accessible by the authenticated user"`
	Statistics           bool              `json:"statistics,omitempty"            jsonschema:"Include group statistics (storage, counts)"`
	WithCustomAttributes bool              `json:"with_custom_attributes,omitempty" jsonschema:"Include custom attributes in the response"`
	CustomAttributes     map[string]string `json:"custom_attributes,omitempty" jsonschema:"Filter groups by custom attribute key/value pairs (administrators only); distinct from with_custom_attributes, which only includes them in the response"`
	SkipGroups           []int64           `json:"skip_groups,omitempty"           jsonschema:"Group IDs to exclude from results"`
	MinAccessLevel       int               `json:"min_access_level,omitempty"      jsonschema:"Minimum access level (10=Guest,20=Reporter,30=Developer,40=Maintainer,50=Owner)"`
	RepositoryStorage    string            `json:"repository_storage,omitempty"    jsonschema:"Filter by repository storage shard (administrators only)"`
	Active               *bool             `json:"active,omitempty"                jsonschema:"Filter by active (true) or inactive/archived (false) groups"`
	Archived             *bool             `json:"archived,omitempty"              jsonschema:"Limit to archived groups (true) or non-archived (false)"`
	MarkedForDeletionOn  string            `json:"marked_for_deletion_on,omitempty" jsonschema:"Filter to groups marked for deletion on this date (YYYY-MM-DD)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// Output represents a GitLab group.
//
// Fields mirror gl.Group (1:1 audit policy: full nested objects). Nested
// sub-objects (statistics, custom_attributes, default_branch_protection_defaults,
// shared_with_groups, ldap_group_links, saml_group_links, projects,
// shared_projects) are surfaced as full local mirrors on their canonical json
// keys (C-IMPORTS: replicated here rather than imported from sibling packages to
// preserve the zero-import-cycle constraint).
type Output struct {
	toolutil.HintableOutput
	ID                    int64  `json:"id"`
	Name                  string `json:"name"`
	Path                  string `json:"path"`
	FullPath              string `json:"full_path"`
	FullName              string `json:"full_name,omitempty"`
	Description           string `json:"description,omitempty"`
	Visibility            string `json:"visibility"`
	WebURL                string `json:"web_url"`
	ParentID              int64  `json:"parent_id,omitempty"`
	OrganizationID        int64  `json:"organization_id,omitempty"`
	DefaultBranch         string `json:"default_branch,omitempty"`
	RequestAccessEnabled  bool   `json:"request_access_enabled"`
	CreatedAt             string `json:"created_at,omitempty"`
	MarkedForDeletion     string `json:"marked_for_deletion_on,omitempty"`
	AvatarURL             string `json:"avatar_url,omitempty"`
	ProjectCreationLevel  string `json:"project_creation_level,omitempty"`
	SubGroupCreationLevel string `json:"subgroup_creation_level,omitempty"`
	LFSEnabled            bool   `json:"lfs_enabled"`
	SharedRunnersSetting  string `json:"shared_runners_setting,omitempty"`
	// Fields added in client-go v2.41.0.
	Archived                             bool   `json:"archived"`
	PreventSharingGroupsOutsideHierarchy bool   `json:"prevent_sharing_groups_outside_hierarchy"`
	EnabledGitAccessProtocol             string `json:"enabled_git_access_protocol,omitempty"`
	MathRenderingLimitsEnabled           bool   `json:"math_rendering_limits_enabled"`
	LockMathRenderingLimitsEnabled       bool   `json:"lock_math_rendering_limits_enabled"`
	DuoAvailability                      string `json:"duo_availability,omitempty" tier:"premium"`
	DuoFeaturesEnabled                   bool   `json:"duo_features_enabled" tier:"premium"`
	LockDuoFeaturesEnabled               bool   `json:"lock_duo_features_enabled" tier:"premium"`
	ExperimentFeaturesEnabled            bool   `json:"experiment_features_enabled" tier:"premium"`
	// Remaining gl.Group fields (1:1 audit).
	MembershipLock                            bool                      `json:"membership_lock" tier:"premium"`
	MaxArtifactsSize                          int64                     `json:"max_artifacts_size,omitempty"`
	DefaultBranchProtectionDefaults           *BranchProtectionDefaults `json:"default_branch_protection_defaults,omitempty"`
	RepositoryStorage                         string                    `json:"repository_storage,omitempty" tier:"premium"`
	FileTemplateProjectID                     int64                     `json:"file_template_project_id,omitempty" tier:"premium"`
	Statistics                                *StatisticsOutput         `json:"statistics,omitempty"`
	CustomAttributes                          []CustomAttributeOutput   `json:"custom_attributes,omitempty"`
	ShareWithGroupLock                        bool                      `json:"share_with_group_lock"`
	RequireTwoFactorAuth                      bool                      `json:"require_two_factor_authentication"`
	TwoFactorGracePeriod                      int64                     `json:"two_factor_grace_period,omitempty"`
	AutoDevopsEnabled                         bool                      `json:"auto_devops_enabled"`
	EmailsEnabled                             bool                      `json:"emails_enabled"`
	EmailsDisabled                            bool                      `json:"emails_disabled"`
	MentionsDisabled                          bool                      `json:"mentions_disabled"`
	CRMEnabled                                bool                      `json:"crm_enabled" jsonschema:"Whether Customer Relations Management (CRM) is enabled for the group"`
	RunnersToken                              string                    `json:"runners_token,omitempty"`
	SharedWithGroups                          []SharedWithGroupOutput   `json:"shared_with_groups,omitempty"`
	LDAPCN                                    string                    `json:"ldap_cn,omitempty" tier:"premium"`
	LDAPAccess                                int                       `json:"ldap_access,omitempty" tier:"premium"`
	LDAPGroupLinks                            []LDAPGroupLinkOutput     `json:"ldap_group_links,omitempty"`
	SAMLGroupLinks                            []SAMLGroupLinkOutput     `json:"saml_group_links,omitempty"`
	SharedRunnersMinutesLimit                 int64                     `json:"shared_runners_minutes_limit,omitempty" tier:"premium"`
	ExtraSharedRunnersMinutesLimit            int64                     `json:"extra_shared_runners_minutes_limit,omitempty" tier:"premium"`
	PreventForkingOutsideGroup                bool                      `json:"prevent_forking_outside_group" tier:"premium"`
	IPRestrictionRanges                       string                    `json:"ip_restriction_ranges,omitempty" tier:"premium"`
	AllowedEmailDomainsList                   string                    `json:"allowed_email_domains_list,omitempty" tier:"premium"`
	WikiAccessLevel                           string                    `json:"wiki_access_level,omitempty" tier:"premium"`
	OnlyAllowMergeIfPipelineSucceeds          bool                      `json:"only_allow_merge_if_pipeline_succeeds" tier:"premium"`
	AllowMergeOnSkippedPipeline               bool                      `json:"allow_merge_on_skipped_pipeline" tier:"premium"`
	OnlyAllowMergeIfAllDiscussionsAreResolved bool                      `json:"only_allow_merge_if_all_discussions_are_resolved" tier:"premium"`
	DefaultBranchProtection                   int64                     `json:"default_branch_protection,omitempty"`
	Projects                                  []ProjectItem             `json:"projects,omitempty"`
	SharedProjects                            []ProjectItem             `json:"shared_projects,omitempty"`
}

// StatisticsOutput mirrors gl.Statistics (the statistics object, returned when
// statistics=true is requested by an Owner).
type StatisticsOutput struct {
	CommitCount           int64 `json:"commit_count"`
	StorageSize           int64 `json:"storage_size"`
	RepositorySize        int64 `json:"repository_size"`
	WikiSize              int64 `json:"wiki_size"`
	LFSObjectsSize        int64 `json:"lfs_objects_size"`
	JobArtifactsSize      int64 `json:"job_artifacts_size"`
	PipelineArtifactsSize int64 `json:"pipeline_artifacts_size"`
	PackagesSize          int64 `json:"packages_size"`
	SnippetsSize          int64 `json:"snippets_size"`
	UploadsSize           int64 `json:"uploads_size"`
	ContainerRegistrySize int64 `json:"container_registry_size"`
}

// CustomAttributeOutput mirrors gl.CustomAttribute (a custom_attributes entry).
type CustomAttributeOutput struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// BranchProtectionDefaults mirrors gl.BranchProtectionDefaults (the
// default_branch_protection_defaults object).
type BranchProtectionDefaults struct {
	AllowedToPush             []GroupAccessLevelOutput `json:"allowed_to_push,omitempty"`
	AllowForcePush            bool                     `json:"allow_force_push,omitempty"`
	AllowedToMerge            []GroupAccessLevelOutput `json:"allowed_to_merge,omitempty"`
	DeveloperCanInitialPush   bool                     `json:"developer_can_initial_push,omitempty"`
	CodeOwnerApprovalRequired bool                     `json:"code_owner_approval_required,omitempty"`
}

// GroupAccessLevelOutput mirrors gl.GroupAccessLevel (an access-level entry in
// allowed_to_push / allowed_to_merge).
type GroupAccessLevelOutput struct {
	AccessLevel int `json:"access_level,omitempty"`
}

// SharedWithGroupOutput mirrors gl.SharedWithGroup (a shared_with_groups entry).
type SharedWithGroupOutput struct {
	GroupID          int64  `json:"group_id"`
	GroupName        string `json:"group_name"`
	GroupFullPath    string `json:"group_full_path"`
	GroupAccessLevel int64  `json:"group_access_level"`
	ExpiresAt        string `json:"expires_at,omitempty"`
	MemberRoleID     int64  `json:"member_role_id,omitempty"`
}

// LDAPGroupLinkOutput mirrors gl.LDAPGroupLink (an ldap_group_links entry).
type LDAPGroupLinkOutput struct {
	CN           string `json:"cn"`
	Filter       string `json:"filter,omitempty"`
	GroupAccess  int    `json:"group_access"`
	Provider     string `json:"provider"`
	MemberRoleID int64  `json:"member_role_id,omitempty"`
}

// SAMLGroupLinkOutput mirrors gl.SAMLGroupLink (a saml_group_links entry).
type SAMLGroupLinkOutput struct {
	Name         string `json:"name"`
	AccessLevel  int    `json:"access_level"`
	MemberRoleID int64  `json:"member_role_id,omitempty"`
	Provider     string `json:"provider,omitempty"`
}

// ListOutput holds a paginated list of groups.
type ListOutput struct {
	toolutil.HintableOutput
	Groups     []Output                  `json:"groups"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// GetInput defines parameters for retrieving a single group.
type GetInput struct {
	GroupID              toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	WithCustomAttributes bool                 `json:"with_custom_attributes,omitempty" jsonschema:"Include custom attributes in the response"`
	WithProjects         *bool                `json:"with_projects,omitempty"          jsonschema:"Include the group's projects in the response (deprecated; prefer gitlab_group_projects)"`
	OrderBy              string               `json:"order_by,omitempty"               jsonschema:"Order embedded projects by field (only applies with with_projects)"`
	Sort                 string               `json:"sort,omitempty"                   jsonschema:"Sort direction for embedded projects (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// MembersListInput defines parameters for listing group members.
type MembersListInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	Query        string               `json:"query,omitempty"          jsonschema:"Filter members by name or username"`
	UserIDs      []int64              `json:"user_ids,omitempty"       jsonschema:"Filter the result to the given user IDs"`
	ShowSeatInfo *bool                `json:"show_seat_info,omitempty" jsonschema:"Include seat information for each member (Premium/Ultimate)"`
	OrderBy      string               `json:"order_by,omitempty"       jsonschema:"Order members by field (id, name, username, access_level, last_activity_on)"`
	Sort         string               `json:"sort,omitempty"           jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// MemberOutput represents a GitLab group member.
//
// Fields mirror gl.GroupMember (1:1 audit policy: full nested objects). The
// created_by, group_saml_identity, and member_role sub-objects are surfaced as
// full local mirrors on their canonical json keys (C-IMPORTS: replicated here
// rather than imported from sibling packages to preserve the zero-import-cycle
// constraint).
type MemberOutput struct {
	ID                int64               `json:"id"`
	Username          string              `json:"username"`
	Name              string              `json:"name"`
	State             string              `json:"state"`
	AvatarURL         string              `json:"avatar_url,omitempty"`
	AccessLevel       int                 `json:"access_level"`
	WebURL            string              `json:"web_url"`
	CreatedAt         string              `json:"created_at,omitempty"`
	CreatedBy         *MemberUserOutput   `json:"created_by,omitempty"`
	ExpiresAt         string              `json:"expires_at,omitempty"`
	Email             string              `json:"email,omitempty"`
	PublicEmail       string              `json:"public_email,omitempty"`
	GroupSAMLIdentity *SAMLIdentityOutput `json:"group_saml_identity,omitempty"`
	MemberRole        *MemberRoleOutput   `json:"member_role,omitempty"`
	IsUsingSeat       bool                `json:"is_using_seat,omitempty"`
}

// MemberUserOutput mirrors gl.MemberCreatedBy (the created_by object);
// canonical shape shared via toolutil.
type MemberUserOutput = toolutil.MemberUserOutput

// SAMLIdentityOutput mirrors gl.GroupMemberSAMLIdentity (the
// group_saml_identity object); canonical shape shared via toolutil.
type SAMLIdentityOutput = toolutil.SAMLIdentityOutput

// MemberRoleOutput mirrors gl.MemberRole (the member_role object). Custom member
// roles are an Enterprise (Premium/Ultimate) feature; the object is nil on
// instances or members without a custom role. Canonical shape shared via
// toolutil.
type MemberRoleOutput = toolutil.MemberRoleOutput

// MemberListOutput holds a paginated list of group members.
type MemberListOutput struct {
	toolutil.HintableOutput
	Members    []MemberOutput            `json:"members"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// SubgroupsListInput defines parameters for listing subgroups.
type SubgroupsListInput struct {
	GroupID              toolutil.StringOrInt `json:"group_id"                jsonschema:"Group ID or URL-encoded path,required"`
	Search               string               `json:"search,omitempty"        jsonschema:"Filter subgroups by name or path"`
	AllAvailable         bool                 `json:"all_available,omitempty" jsonschema:"Show all subgroups accessible by the authenticated user"`
	Owned                bool                 `json:"owned,omitempty"         jsonschema:"Limit to subgroups explicitly owned by the authenticated user"`
	MinAccessLevel       int                  `json:"min_access_level,omitempty" jsonschema:"Minimum access level (5=Minimal access,10=Guest,15=Planner (Premium/Ultimate),20=Reporter,25=Security Manager (Premium/Ultimate),30=Developer,40=Maintainer,50=Owner,60=Admin where supported)"`
	OrderBy              string               `json:"order_by,omitempty"      jsonschema:"Order subgroups by field (name, path, id, similarity)"`
	Sort                 string               `json:"sort,omitempty"          jsonschema:"Sort direction (asc, desc)"`
	Statistics           bool                 `json:"statistics,omitempty"    jsonschema:"Include group statistics (storage, counts)"`
	Visibility           string               `json:"visibility,omitempty"    jsonschema:"Filter by visibility (public, internal, private)"`
	TopLevelOnly         bool                 `json:"top_level_only,omitempty" jsonschema:"Limit to top-level subgroups (exclude nested descendants)"`
	WithCustomAttributes bool                 `json:"with_custom_attributes,omitempty" jsonschema:"Include custom attributes in the response"`
	SkipGroups           []int64              `json:"skip_groups,omitempty"   jsonschema:"Group IDs to exclude from results"`
	RepositoryStorage    string               `json:"repository_storage,omitempty" jsonschema:"Filter by repository storage shard (administrators only)"`
	Active               *bool                `json:"active,omitempty"        jsonschema:"Filter by active (true) or inactive/archived (false) subgroups"`
	Archived             *bool                `json:"archived,omitempty"      jsonschema:"Limit to archived subgroups (true) or non-archived (false)"`
	MarkedForDeletionOn  string               `json:"marked_for_deletion_on,omitempty" jsonschema:"Filter to subgroups marked for deletion on this date (YYYY-MM-DD)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ToOutput converts a GitLab API [gl.Group] to the MCP tool output
// format, extracting identifier, path, visibility, and parent information.
func ToOutput(g *gl.Group) Output {
	out := Output{
		ID:                    g.ID,
		Name:                  g.Name,
		Path:                  g.Path,
		FullPath:              g.FullPath,
		FullName:              g.FullName,
		Description:           g.Description,
		Visibility:            string(g.Visibility),
		WebURL:                g.WebURL,
		ParentID:              g.ParentID,
		OrganizationID:        g.OrganizationID,
		DefaultBranch:         g.DefaultBranch,
		RequestAccessEnabled:  g.RequestAccessEnabled,
		AvatarURL:             g.AvatarURL,
		ProjectCreationLevel:  string(g.ProjectCreationLevel),
		SubGroupCreationLevel: string(g.SubGroupCreationLevel),
	}
	if g.CreatedAt != nil {
		out.CreatedAt = g.CreatedAt.Format(time.RFC3339)
	}
	if g.MarkedForDeletionOn != nil {
		out.MarkedForDeletion = g.MarkedForDeletionOn.String()
	}
	out.LFSEnabled = g.LFSEnabled
	out.SharedRunnersSetting = string(g.SharedRunnersSetting)
	out.Archived = g.Archived
	out.PreventSharingGroupsOutsideHierarchy = g.PreventSharingGroupsOutsideHierarchy
	out.EnabledGitAccessProtocol = string(g.EnabledGitAccessProtocol)
	out.MathRenderingLimitsEnabled = g.MathRenderingLimitsEnabled
	out.LockMathRenderingLimitsEnabled = g.LockMathRenderingLimitsEnabled
	out.DuoAvailability = string(g.DuoAvailability)
	out.DuoFeaturesEnabled = g.DuoFeaturesEnabled
	out.LockDuoFeaturesEnabled = g.LockDuoFeaturesEnabled
	out.ExperimentFeaturesEnabled = g.ExperimentFeaturesEnabled
	out.MembershipLock = g.MembershipLock
	out.MaxArtifactsSize = g.MaxArtifactsSize
	out.RepositoryStorage = g.RepositoryStorage
	out.FileTemplateProjectID = g.FileTemplateProjectID
	out.ShareWithGroupLock = g.ShareWithGroupLock
	out.RequireTwoFactorAuth = g.RequireTwoFactorAuth
	out.TwoFactorGracePeriod = g.TwoFactorGracePeriod
	out.AutoDevopsEnabled = g.AutoDevopsEnabled
	out.EmailsEnabled = g.EmailsEnabled
	out.EmailsDisabled = g.EmailsDisabled //nolint:staticcheck // SA1019: mirror deprecated SDK field for 1:1 API coverage
	out.MentionsDisabled = g.MentionsDisabled
	out.CRMEnabled = g.CRMEnabled
	out.RunnersToken = g.RunnersToken
	out.LDAPCN = g.LDAPCN
	out.LDAPAccess = int(g.LDAPAccess)
	out.SharedRunnersMinutesLimit = g.SharedRunnersMinutesLimit
	out.ExtraSharedRunnersMinutesLimit = g.ExtraSharedRunnersMinutesLimit
	out.PreventForkingOutsideGroup = g.PreventForkingOutsideGroup
	out.IPRestrictionRanges = g.IPRestrictionRanges
	out.AllowedEmailDomainsList = g.AllowedEmailDomainsList
	out.WikiAccessLevel = string(g.WikiAccessLevel)
	out.OnlyAllowMergeIfPipelineSucceeds = g.OnlyAllowMergeIfPipelineSucceeds
	out.AllowMergeOnSkippedPipeline = g.AllowMergeOnSkippedPipeline
	out.OnlyAllowMergeIfAllDiscussionsAreResolved = g.OnlyAllowMergeIfAllDiscussionsAreResolved
	out.DefaultBranchProtection = g.DefaultBranchProtection //nolint:staticcheck // SA1019: mirror deprecated SDK field for 1:1 API coverage
	out.Statistics = statisticsOutput(g.Statistics)
	out.DefaultBranchProtectionDefaults = branchProtectionDefaultsOutput(g.DefaultBranchProtectionDefaults)
	out.CustomAttributes = customAttributesOutput(g.CustomAttributes)
	out.SharedWithGroups = sharedWithGroupsOutput(g.SharedWithGroups)
	out.LDAPGroupLinks = ldapGroupLinksOutput(g.LDAPGroupLinks)
	out.SAMLGroupLinks = samlGroupLinksOutput(g.SAMLGroupLinks)
	out.Projects = projectItemsFromGroup(g.Projects)             //nolint:staticcheck // SA1019: mirror deprecated SDK field for 1:1 API coverage
	out.SharedProjects = projectItemsFromGroup(g.SharedProjects) //nolint:staticcheck // SA1019: mirror deprecated SDK field for 1:1 API coverage
	return out
}

// statisticsOutput mirrors a gl.Statistics into the local output shape.
func statisticsOutput(s *gl.Statistics) *StatisticsOutput {
	if s == nil {
		return nil
	}
	return &StatisticsOutput{
		CommitCount:           s.CommitCount,
		StorageSize:           s.StorageSize,
		RepositorySize:        s.RepositorySize,
		WikiSize:              s.WikiSize,
		LFSObjectsSize:        s.LFSObjectsSize,
		JobArtifactsSize:      s.JobArtifactsSize,
		PipelineArtifactsSize: s.PipelineArtifactsSize,
		PackagesSize:          s.PackagesSize,
		SnippetsSize:          s.SnippetsSize,
		UploadsSize:           s.UploadsSize,
		ContainerRegistrySize: s.ContainerRegistrySize,
	}
}

// branchProtectionDefaultsOutput mirrors a gl.BranchProtectionDefaults into the
// local output shape.
func branchProtectionDefaultsOutput(d *gl.BranchProtectionDefaults) *BranchProtectionDefaults {
	if d == nil {
		return nil
	}
	return &BranchProtectionDefaults{
		AllowedToPush:             groupAccessLevelsOutput(d.AllowedToPush),
		AllowForcePush:            d.AllowForcePush,
		AllowedToMerge:            groupAccessLevelsOutput(d.AllowedToMerge),
		DeveloperCanInitialPush:   d.DeveloperCanInitialPush,
		CodeOwnerApprovalRequired: d.CodeOwnerApprovalRequired,
	}
}

// groupAccessLevelsOutput mirrors a slice of gl.GroupAccessLevel into the local
// output shape.
func groupAccessLevelsOutput(levels []*gl.GroupAccessLevel) []GroupAccessLevelOutput {
	if len(levels) == 0 {
		return nil
	}
	out := make([]GroupAccessLevelOutput, 0, len(levels))
	for _, l := range levels {
		if l == nil {
			continue
		}
		var al int
		if l.AccessLevel != nil {
			al = int(*l.AccessLevel)
		}
		out = append(out, GroupAccessLevelOutput{AccessLevel: al})
	}
	return out
}

// customAttributesOutput mirrors a slice of gl.CustomAttribute into the local
// output shape.
func customAttributesOutput(attrs []*gl.CustomAttribute) []CustomAttributeOutput {
	if len(attrs) == 0 {
		return nil
	}
	out := make([]CustomAttributeOutput, 0, len(attrs))
	for _, a := range attrs {
		if a == nil {
			continue
		}
		out = append(out, CustomAttributeOutput{Key: a.Key, Value: a.Value})
	}
	return out
}

// sharedWithGroupsOutput mirrors a slice of gl.SharedWithGroup into the local
// output shape.
func sharedWithGroupsOutput(groups []gl.SharedWithGroup) []SharedWithGroupOutput {
	if len(groups) == 0 {
		return nil
	}
	out := make([]SharedWithGroupOutput, len(groups))
	for i, g := range groups {
		out[i] = SharedWithGroupOutput{
			GroupID:          g.GroupID,
			GroupName:        g.GroupName,
			GroupFullPath:    g.GroupFullPath,
			GroupAccessLevel: g.GroupAccessLevel,
			MemberRoleID:     g.MemberRoleID,
		}
		if g.ExpiresAt != nil {
			out[i].ExpiresAt = g.ExpiresAt.String()
		}
	}
	return out
}

// ldapGroupLinksOutput mirrors a slice of gl.LDAPGroupLink into the local
// output shape.
func ldapGroupLinksOutput(links []*gl.LDAPGroupLink) []LDAPGroupLinkOutput {
	if len(links) == 0 {
		return nil
	}
	out := make([]LDAPGroupLinkOutput, 0, len(links))
	for _, l := range links {
		if l == nil {
			continue
		}
		out = append(out, LDAPGroupLinkOutput{
			CN:           l.CN,
			Filter:       l.Filter,
			GroupAccess:  int(l.GroupAccess),
			Provider:     l.Provider,
			MemberRoleID: l.MemberRoleID,
		})
	}
	return out
}

// samlGroupLinksOutput mirrors a slice of gl.SAMLGroupLink into the local
// output shape.
func samlGroupLinksOutput(links []*gl.SAMLGroupLink) []SAMLGroupLinkOutput {
	if len(links) == 0 {
		return nil
	}
	out := make([]SAMLGroupLinkOutput, 0, len(links))
	for _, l := range links {
		if l == nil {
			continue
		}
		out = append(out, SAMLGroupLinkOutput{
			Name:         l.Name,
			AccessLevel:  int(l.AccessLevel),
			MemberRoleID: l.MemberRoleID,
			Provider:     l.Provider,
		})
	}
	return out
}

// projectItemsFromGroup maps the deprecated embedded gl.Project slices
// (Group.Projects / Group.SharedProjects) into the local ProjectItem shape.
func projectItemsFromGroup(projects []*gl.Project) []ProjectItem {
	if len(projects) == 0 {
		return nil
	}
	out := make([]ProjectItem, len(projects))
	for i, p := range projects {
		out[i] = ProjectItem{
			ID:                p.ID,
			Name:              p.Name,
			PathWithNamespace: p.PathWithNamespace,
			Description:       p.Description,
			Visibility:        string(p.Visibility),
			WebURL:            p.WebURL,
			DefaultBranch:     p.DefaultBranch,
			Archived:          p.Archived,
		}
		if p.CreatedAt != nil {
			out[i].CreatedAt = p.CreatedAt.Format(time.RFC3339)
		}
	}
	return out
}

// MemberToOutput converts a GitLab API [gl.GroupMember] to the MCP tool output
// format, surfacing the full created_by, group_saml_identity, and member_role
// sub-objects (1:1 audit policy: full nested objects).
func MemberToOutput(m *gl.GroupMember) MemberOutput {
	out := MemberOutput{
		ID:                m.ID,
		Username:          m.Username,
		Name:              m.Name,
		State:             m.State,
		AvatarURL:         m.AvatarURL,
		AccessLevel:       int(m.AccessLevel),
		WebURL:            m.WebURL,
		Email:             m.Email,
		PublicEmail:       m.PublicEmail,
		IsUsingSeat:       m.IsUsingSeat,
		CreatedBy:         memberUserOutput(m.CreatedBy),
		GroupSAMLIdentity: samlIdentityOutput(m.GroupSAMLIdentity),
		MemberRole:        memberRoleOutput(m.MemberRole),
	}
	if m.CreatedAt != nil {
		out.CreatedAt = m.CreatedAt.Format(time.RFC3339)
	}
	if m.ExpiresAt != nil {
		out.ExpiresAt = m.ExpiresAt.String()
	}
	return out
}

// memberUserOutput mirrors a gl.MemberCreatedBy into the shared output shape.
func memberUserOutput(u *gl.MemberCreatedBy) *MemberUserOutput {
	return toolutil.NewMemberUserOutput(u)
}

// samlIdentityOutput mirrors a gl.GroupMemberSAMLIdentity into the shared
// output shape.
func samlIdentityOutput(s *gl.GroupMemberSAMLIdentity) *SAMLIdentityOutput {
	return toolutil.NewSAMLIdentityOutput(s)
}

// memberRoleOutput mirrors a gl.MemberRole into the shared output shape.
func memberRoleOutput(r *gl.MemberRole) *MemberRoleOutput {
	return toolutil.NewMemberRoleOutput(r)
}

// listGroupsOptions builds the SDK options for [List] from the tool input,
// applying pagination and every supported group filter.
func listGroupsOptions(input ListInput) *gl.ListGroupsOptions {
	opts := &gl.ListGroupsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.Owned {
		opts.Owned = new(true)
	}
	if input.TopLevelOnly {
		opts.TopLevelOnly = new(true)
	}
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	if input.Visibility != "" {
		opts.Visibility = new(gl.VisibilityValue(input.Visibility))
	}
	if input.AllAvailable {
		opts.AllAvailable = new(true)
	}
	if input.Statistics {
		opts.Statistics = new(true)
	}
	if input.WithCustomAttributes {
		opts.WithCustomAttributes = new(true)
	}
	if len(input.CustomAttributes) > 0 {
		opts.CustomAttributes = gl.CustomAttributesFilter(input.CustomAttributes)
	}
	if len(input.SkipGroups) > 0 {
		opts.SkipGroups = &input.SkipGroups
	}
	if input.MinAccessLevel > 0 {
		opts.MinAccessLevel = new(gl.AccessLevelValue(input.MinAccessLevel))
	}
	if input.RepositoryStorage != "" {
		opts.RepositoryStorage = new(input.RepositoryStorage)
	}
	if input.Active != nil {
		opts.Active = input.Active
	}
	if input.Archived != nil {
		opts.Archived = input.Archived
	}
	if input.MarkedForDeletionOn != "" {
		if t, perr := gl.ParseISOTime(input.MarkedForDeletionOn); perr == nil {
			opts.MarkedForDeletionOn = &t
		}
	}
	return opts
}

// List retrieves a paginated list of GitLab groups visible to the
// authenticated user. Supports filtering by search term, ownership, and
// top-level-only restriction. Returns the groups with pagination metadata.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}

	opts := listGroupsOptions(input)

	groups, resp, err := client.GL().Groups.ListGroups(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("List", err, http.StatusUnauthorized,
			"verify GITLAB_TOKEN is valid; non-authenticated requests only return public groups")
	}

	out := ListOutput{
		Groups:     make([]Output, len(groups)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for i, g := range groups {
		out.Groups[i] = ToOutput(g)
	}
	return out, nil
}

// Get retrieves a single GitLab group by its ID or URL-encoded path.
// Returns the group details or an error if the group is not found.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, errors.New("Get: group_id is required. Use gitlab_group_list to find the ID first, then pass it as group_id")
	}

	opts := &gl.GetGroupOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	if input.WithCustomAttributes {
		opts.WithCustomAttributes = new(true)
	}
	if input.WithProjects != nil {
		opts.WithProjects = input.WithProjects //nolint:staticcheck // SA1019: mirror deprecated SDK option for 1:1 API coverage
	}

	g, _, err := client.GL().Groups.GetGroup(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("Get", err, http.StatusNotFound,
			"verify group_id (numeric ID or full path like 'group/subgroup'); URL-encode '/' as '%2F' when using paths")
	}
	return ToOutput(g), nil
}

// MembersList retrieves all members of a GitLab group, including
// inherited members from parent groups. Supports filtering by name or
// username and pagination. Returns the member list with pagination metadata.
func MembersList(ctx context.Context, client *gitlabclient.Client, input MembersListInput) (MemberListOutput, error) {
	if err := ctx.Err(); err != nil {
		return MemberListOutput{}, err
	}
	if input.GroupID == "" {
		return MemberListOutput{}, errors.New("MembersList: group_id is required. Use gitlab_group_list to find the ID first, then pass it as group_id")
	}

	opts := &gl.ListGroupMembersOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	if input.Query != "" {
		opts.Query = new(input.Query)
	}
	if len(input.UserIDs) > 0 {
		opts.UserIDs = &input.UserIDs
	}
	if input.ShowSeatInfo != nil {
		opts.ShowSeatInfo = input.ShowSeatInfo
	}

	memberList, resp, err := client.GL().Groups.ListAllGroupMembers(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return MemberListOutput{}, toolutil.WrapErrWithStatusHint("MembersList", err, http.StatusNotFound,
			"verify group_id with gitlab_group_get; private group membership requires the caller to be a member")
	}

	out := MemberListOutput{
		Members:    make([]MemberOutput, len(memberList)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for i, m := range memberList {
		out.Members[i] = MemberToOutput(m)
	}
	return out, nil
}

// SubgroupsList retrieves a paginated list of descendant groups (subgroups)
// for a given parent group. Supports filtering by search term and pagination.
// Returns the subgroups with pagination metadata.
func SubgroupsList(ctx context.Context, client *gitlabclient.Client, input SubgroupsListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.GroupID == "" {
		return ListOutput{}, errors.New("SubgroupsList: group_id is required. Use gitlab_group_list to find the ID first, then pass it as group_id")
	}

	opts := subgroupsListOptions(input)

	groups, resp, err := client.GL().Groups.ListDescendantGroups(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("SubgroupsList", err, http.StatusNotFound,
			"verify group_id with gitlab_group_get; subgroup listing returns descendants at all depths")
	}

	out := ListOutput{
		Groups:     make([]Output, len(groups)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for i, g := range groups {
		out.Groups[i] = ToOutput(g)
	}
	return out, nil
}

// subgroupsListOptions builds the ListDescendantGroups options from the input,
// applying offset/keyset pagination and every supported descendant-group
// filter. Split out of SubgroupsList to keep that handler's cyclomatic
// complexity flat.
func subgroupsListOptions(input SubgroupsListInput) *gl.ListDescendantGroupsOptions {
	opts := &gl.ListDescendantGroupsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.AllAvailable {
		opts.AllAvailable = new(true)
	}
	if input.Owned {
		opts.Owned = new(true)
	}
	if input.MinAccessLevel > 0 {
		opts.MinAccessLevel = new(gl.AccessLevelValue(input.MinAccessLevel))
	}
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	if input.Statistics {
		opts.Statistics = new(true)
	}
	if input.Visibility != "" {
		opts.Visibility = new(gl.VisibilityValue(input.Visibility))
	}
	if input.TopLevelOnly {
		opts.TopLevelOnly = new(true)
	}
	if input.WithCustomAttributes {
		opts.WithCustomAttributes = new(true)
	}
	if len(input.SkipGroups) > 0 {
		opts.SkipGroups = &input.SkipGroups
	}
	if input.RepositoryStorage != "" {
		opts.RepositoryStorage = new(input.RepositoryStorage)
	}
	if input.Active != nil {
		opts.Active = input.Active
	}
	if input.Archived != nil {
		opts.Archived = input.Archived
	}
	if input.MarkedForDeletionOn != "" {
		if t, perr := gl.ParseISOTime(input.MarkedForDeletionOn); perr == nil {
			opts.MarkedForDeletionOn = &t
		}
	}
	return opts
}

// ---------------------------------------------------------------------------
// Input types for new group operations
// ---------------------------------------------------------------------------.

// BranchProtectionDefaultsInput mirrors gl.DefaultBranchProtectionDefaultsOptions
// (the default_branch_protection_defaults object on create/update). Access
// levels in allowed_to_push / allowed_to_merge are given as a list of integer
// access levels; GitLab applies the highest provided level.
type BranchProtectionDefaultsInput struct {
	AllowedToPush             []int `json:"allowed_to_push,omitempty"              jsonschema:"Access levels allowed to push (30=Developer, 40=Maintainer); GitLab applies the highest provided"`
	AllowForcePush            *bool `json:"allow_force_push,omitempty"             jsonschema:"Allow force push on the default branch"`
	AllowedToMerge            []int `json:"allowed_to_merge,omitempty"             jsonschema:"Access levels allowed to merge (30=Developer, 40=Maintainer); GitLab applies the highest provided"`
	DeveloperCanInitialPush   *bool `json:"developer_can_initial_push,omitempty"   jsonschema:"Allow developers to make the initial push to the default branch"`
	CodeOwnerApprovalRequired *bool `json:"code_owner_approval_required,omitempty" jsonschema:"Require code owner approval before merging into the default branch"`
}

// toDefaultBranchProtectionDefaultsOptions converts the input into the SDK
// options shape, returning nil when no field was supplied.
func (b *BranchProtectionDefaultsInput) toOptions() *gl.DefaultBranchProtectionDefaultsOptions {
	if b == nil {
		return nil
	}
	opts := &gl.DefaultBranchProtectionDefaultsOptions{
		AllowForcePush:            b.AllowForcePush,
		DeveloperCanInitialPush:   b.DeveloperCanInitialPush,
		CodeOwnerApprovalRequired: b.CodeOwnerApprovalRequired,
	}
	if levels := accessLevelOptions(b.AllowedToPush); levels != nil {
		opts.AllowedToPush = levels
	}
	if levels := accessLevelOptions(b.AllowedToMerge); levels != nil {
		opts.AllowedToMerge = levels
	}
	return opts
}

// accessLevelOptions converts integer access levels into the SDK
// []*gl.GroupAccessLevel pointer-slice shape.
func accessLevelOptions(levels []int) *[]*gl.GroupAccessLevel {
	if len(levels) == 0 {
		return nil
	}
	out := make([]*gl.GroupAccessLevel, len(levels))
	for i, l := range levels {
		out[i] = &gl.GroupAccessLevel{AccessLevel: new(gl.AccessLevelValue(l))}
	}
	return &out
}

// CreateInput defines parameters for creating a group.
type CreateInput struct {
	Name                         string `json:"name"                          jsonschema:"Group name,required"`
	Path                         string `json:"path,omitempty"                jsonschema:"Group URL path (defaults to kebab-case of name)"`
	Description                  string `json:"description,omitempty"         jsonschema:"Group description"`
	Visibility                   string `json:"visibility,omitempty"          jsonschema:"Visibility level (private, internal, public)"`
	ParentID                     int64  `json:"parent_id,omitempty"           jsonschema:"Parent group ID (creates a subgroup)"`
	OrganizationID               *int64 `json:"organization_id,omitempty"     jsonschema:"Organization ID to create the group in (GitLab.com multi-organization; defaults to the default organization)"`
	RequestAccessEnabled         *bool  `json:"request_access_enabled,omitempty" jsonschema:"Allow users to request access"`
	LFSEnabled                   *bool  `json:"lfs_enabled,omitempty"         jsonschema:"Enable Git LFS"`
	DefaultBranch                string `json:"default_branch,omitempty"      jsonschema:"Default branch name"`
	MathRenderingLimitsEnabled   *bool  `json:"math_rendering_limits_enabled,omitempty"   jsonschema:"Enable math rendering limits"`
	WebBasedCommitSigningEnabled *bool  `json:"web_based_commit_signing_enabled,omitempty" jsonschema:"Enable web-based commit signing for projects in this group"`
	AllowPersonalSnippets        *bool  `json:"allow_personal_snippets,omitempty"          jsonschema:"Allow members to create personal snippets"`
	CRMEnabled                   *bool  `json:"crm_enabled,omitempty"         jsonschema:"Enable Customer Relations Management (CRM) for the group"`

	AutoDevopsEnabled               *bool                          `json:"auto_devops_enabled,omitempty"                jsonschema:"Enable Auto DevOps for projects in this group"`
	DefaultBranchProtection         *int64                         `json:"default_branch_protection,omitempty"          jsonschema:"Deprecated: default branch protection level (0=none,1=partial,2=full,3=initial push,4=fully protected). Prefer default_branch_protection_defaults"`
	DefaultBranchProtectionDefaults *BranchProtectionDefaultsInput `json:"default_branch_protection_defaults,omitempty" jsonschema:"Default branch protection settings object"`
	DuoAvailability                 string                         `json:"duo_availability,omitempty"                   jsonschema:"GitLab Duo availability (default_on, default_off, never_on)"`
	EmailsEnabled                   *bool                          `json:"emails_enabled,omitempty"                     jsonschema:"Enable email notifications"`
	EmailsDisabled                  *bool                          `json:"emails_disabled,omitempty"                    jsonschema:"Deprecated: disable email notifications. Prefer emails_enabled"`
	EnabledGitAccessProtocol        string                         `json:"enabled_git_access_protocol,omitempty"        jsonschema:"Allowed Git access protocol (ssh, http, all)"`
	ExperimentFeaturesEnabled       *bool                          `json:"experiment_features_enabled,omitempty"        jsonschema:"Enable experimental features"`
	ExtraSharedRunnersMinutesLimit  *int64                         `json:"extra_shared_runners_minutes_limit,omitempty" jsonschema:"Extra shared runner compute-minutes (administrators only)"`
	MembershipLock                  *bool                          `json:"membership_lock,omitempty"                    jsonschema:"Prevent members from being added to projects in this group"`
	MentionsDisabled                *bool                          `json:"mentions_disabled,omitempty"                  jsonschema:"Disable @-mention notifications"`
	ProjectCreationLevel            string                         `json:"project_creation_level,omitempty"             jsonschema:"Who can create projects (noone, maintainer, developer)"`
	RequireTwoFactorAuth            *bool                          `json:"require_two_factor_authentication,omitempty"  jsonschema:"Require two-factor authentication for members"`
	ShareWithGroupLock              *bool                          `json:"share_with_group_lock,omitempty"              jsonschema:"Prevent sharing projects in this group with other groups"`
	SharedRunnersMinutesLimit       *int64                         `json:"shared_runners_minutes_limit,omitempty"       jsonschema:"Shared runner compute-minutes limit (administrators only)"`
	SubGroupCreationLevel           string                         `json:"subgroup_creation_level,omitempty"            jsonschema:"Who can create subgroups (owner, maintainer)"`
	TwoFactorGracePeriod            *int64                         `json:"two_factor_grace_period,omitempty"            jsonschema:"Grace period in hours before two-factor authentication is enforced"`
	WikiAccessLevel                 string                         `json:"wiki_access_level,omitempty"                  jsonschema:"Wiki access level (disabled, private, enabled)"`

	UniqueProjectDownloadLimit                  *int64   `json:"unique_project_download_limit,omitempty" tier:"ultimate"                       jsonschema:"Max number of unique projects a user can download before being banned (Ultimate)"`
	UniqueProjectDownloadLimitIntervalInSeconds *int64   `json:"unique_project_download_limit_interval_in_seconds,omitempty" tier:"ultimate"   jsonschema:"Time window in seconds for the unique project download limit (Ultimate)"`
	UniqueProjectDownloadLimitAllowlist         []string `json:"unique_project_download_limit_allowlist,omitempty" tier:"ultimate"             jsonschema:"Usernames excluded from the unique project download limit (Ultimate)"`
	UniqueProjectDownloadLimitAlertlist         []int64  `json:"unique_project_download_limit_alertlist,omitempty" tier:"ultimate"             jsonschema:"User IDs notified when the unique project download limit is exceeded (Ultimate)"`
	AutoBanUserOnExcessiveProjectsDownload      *bool    `json:"auto_ban_user_on_excessive_projects_download,omitempty" tier:"ultimate"        jsonschema:"Automatically ban users who exceed the unique project download limit (Ultimate)"`
}

// UpdateInput defines parameters for updating a group.
type UpdateInput struct {
	GroupID                      toolutil.StringOrInt `json:"group_id"                jsonschema:"Group ID or URL-encoded path,required"`
	Name                         string               `json:"name,omitempty"          jsonschema:"Group name"`
	Path                         string               `json:"path,omitempty"          jsonschema:"Group URL path"`
	Description                  string               `json:"description,omitempty"   jsonschema:"Group description"`
	Visibility                   string               `json:"visibility,omitempty"    jsonschema:"Visibility level (private, internal, public)"`
	RequestAccessEnabled         *bool                `json:"request_access_enabled,omitempty" jsonschema:"Allow users to request access"`
	LFSEnabled                   *bool                `json:"lfs_enabled,omitempty"   jsonschema:"Enable Git LFS"`
	DefaultBranch                string               `json:"default_branch,omitempty" jsonschema:"Default branch name"`
	MathRenderingLimitsEnabled   *bool                `json:"math_rendering_limits_enabled,omitempty"   jsonschema:"Enable math rendering limits"`
	WebBasedCommitSigningEnabled *bool                `json:"web_based_commit_signing_enabled,omitempty" jsonschema:"Enable web-based commit signing for projects in this group"`
	AllowPersonalSnippets        *bool                `json:"allow_personal_snippets,omitempty"          jsonschema:"Allow members to create personal snippets"`
	CRMEnabled                   *bool                `json:"crm_enabled,omitempty"         jsonschema:"Enable Customer Relations Management (CRM) for the group"`

	AutoDevopsEnabled               *bool                          `json:"auto_devops_enabled,omitempty"                jsonschema:"Enable Auto DevOps for projects in this group"`
	DefaultBranchProtection         *int64                         `json:"default_branch_protection,omitempty"          jsonschema:"Deprecated: default branch protection level (0=none,1=partial,2=full,3=initial push,4=fully protected). Prefer default_branch_protection_defaults"`
	DefaultBranchProtectionDefaults *BranchProtectionDefaultsInput `json:"default_branch_protection_defaults,omitempty" jsonschema:"Default branch protection settings object"`
	DuoAvailability                 string                         `json:"duo_availability,omitempty"                   jsonschema:"GitLab Duo availability (default_on, default_off, never_on)"`
	DuoFeaturesEnabled              *bool                          `json:"duo_features_enabled,omitempty"               jsonschema:"Enable GitLab Duo features"`
	LockDuoFeaturesEnabled          *bool                          `json:"lock_duo_features_enabled,omitempty"          jsonschema:"Prevent subgroups from changing the Duo features setting"`
	EmailsEnabled                   *bool                          `json:"emails_enabled,omitempty"                     jsonschema:"Enable email notifications"`
	EmailsDisabled                  *bool                          `json:"emails_disabled,omitempty"                    jsonschema:"Deprecated: disable email notifications. Prefer emails_enabled"`
	EnabledGitAccessProtocol        string                         `json:"enabled_git_access_protocol,omitempty"        jsonschema:"Allowed Git access protocol (ssh, http, all)"`
	ExperimentFeaturesEnabled       *bool                          `json:"experiment_features_enabled,omitempty"        jsonschema:"Enable experimental features"`
	ExtraSharedRunnersMinutesLimit  *int64                         `json:"extra_shared_runners_minutes_limit,omitempty" jsonschema:"Extra shared runner compute-minutes (administrators only)"`
	FileTemplateProjectID           *int64                         `json:"file_template_project_id,omitempty"           jsonschema:"Project ID providing file templates for this group (Premium/Ultimate)"`
	IPRestrictionRanges             string                         `json:"ip_restriction_ranges,omitempty"              jsonschema:"Comma-separated CIDR ranges allowed to access the group (Premium/Ultimate)"`
	AllowedEmailDomainsList         string                         `json:"allowed_email_domains_list,omitempty"         jsonschema:"Comma-separated list of email domains allowed for members (Premium/Ultimate)"`
	LockMathRenderingLimitsEnabled  *bool                          `json:"lock_math_rendering_limits_enabled,omitempty" jsonschema:"Prevent subgroups from changing the math rendering limits setting"`
	MaxArtifactsSize                *int64                         `json:"max_artifacts_size,omitempty"                 jsonschema:"Maximum job artifacts size in MB (administrators only)"`
	MembershipLock                  *bool                          `json:"membership_lock,omitempty"                    jsonschema:"Prevent members from being added to projects in this group"`
	MentionsDisabled                *bool                          `json:"mentions_disabled,omitempty"                  jsonschema:"Disable @-mention notifications"`
	PreventForkingOutsideGroup      *bool                          `json:"prevent_forking_outside_group,omitempty"      jsonschema:"Prevent forking projects outside the group (Premium/Ultimate)"`
	PreventSharingGroupsOutside     *bool                          `json:"prevent_sharing_groups_outside_hierarchy,omitempty" jsonschema:"Prevent inviting groups outside this group's hierarchy"`
	ProjectCreationLevel            string                         `json:"project_creation_level,omitempty"             jsonschema:"Who can create projects (noone, maintainer, developer)"`
	RequireTwoFactorAuth            *bool                          `json:"require_two_factor_authentication,omitempty"  jsonschema:"Require two-factor authentication for members"`
	ShareWithGroupLock              *bool                          `json:"share_with_group_lock,omitempty"              jsonschema:"Prevent sharing projects in this group with other groups"`
	SharedRunnersMinutesLimit       *int64                         `json:"shared_runners_minutes_limit,omitempty"       jsonschema:"Shared runner compute-minutes limit (administrators only)"`
	SharedRunnersSetting            string                         `json:"shared_runners_setting,omitempty"             jsonschema:"Shared runners setting (enabled, disabled_and_overridable, disabled_and_unoverridable)"`
	SubGroupCreationLevel           string                         `json:"subgroup_creation_level,omitempty"            jsonschema:"Who can create subgroups (owner, maintainer)"`
	StepUpAuthRequiredOAuthProvider string                         `json:"step_up_auth_required_oauth_provider,omitempty" jsonschema:"OAuth provider required for step-up authentication"`
	TwoFactorGracePeriod            *int64                         `json:"two_factor_grace_period,omitempty"            jsonschema:"Grace period in hours before two-factor authentication is enforced"`
	WikiAccessLevel                 string                         `json:"wiki_access_level,omitempty"                  jsonschema:"Wiki access level (disabled, private, enabled)"`

	OnlyAllowMergeIfPipelineSucceeds          *bool `json:"only_allow_merge_if_pipeline_succeeds,omitempty"           jsonschema:"Only allow merging when the pipeline succeeds"`
	AllowMergeOnSkippedPipeline               *bool `json:"allow_merge_on_skipped_pipeline,omitempty"                jsonschema:"Allow merging when the pipeline is skipped"`
	OnlyAllowMergeIfAllDiscussionsAreResolved *bool `json:"only_allow_merge_if_all_discussions_are_resolved,omitempty" jsonschema:"Only allow merging when all discussions are resolved"`

	UniqueProjectDownloadLimit                  *int64   `json:"unique_project_download_limit,omitempty" tier:"ultimate"                       jsonschema:"Max number of unique projects a user can download before being banned (Ultimate)"`
	UniqueProjectDownloadLimitIntervalInSeconds *int64   `json:"unique_project_download_limit_interval_in_seconds,omitempty" tier:"ultimate"   jsonschema:"Time window in seconds for the unique project download limit (Ultimate)"`
	UniqueProjectDownloadLimitAllowlist         []string `json:"unique_project_download_limit_allowlist,omitempty" tier:"ultimate"             jsonschema:"Usernames excluded from the unique project download limit (Ultimate)"`
	UniqueProjectDownloadLimitAlertlist         []int64  `json:"unique_project_download_limit_alertlist,omitempty" tier:"ultimate"             jsonschema:"User IDs notified when the unique project download limit is exceeded (Ultimate)"`
	AutoBanUserOnExcessiveProjectsDownload      *bool    `json:"auto_ban_user_on_excessive_projects_download,omitempty" tier:"ultimate"        jsonschema:"Automatically ban users who exceed the unique project download limit (Ultimate)"`
}

// DeleteInput defines parameters for deleting a group.
type DeleteInput struct {
	GroupID           toolutil.StringOrInt `json:"group_id"                    jsonschema:"Group ID or URL-encoded path,required"`
	PermanentlyRemove bool                 `json:"permanently_remove,omitempty" jsonschema:"Permanently remove instead of marking for deletion"`
	FullPath          string               `json:"full_path,omitempty"          jsonschema:"Full path (required when permanently_remove=true)"`
}

// RestoreInput defines parameters for restoring a group marked for deletion.
type RestoreInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
}

// ArchiveInput defines parameters for archiving or unarchiving a group.
type ArchiveInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
}

// SearchInput defines parameters for searching groups.
type SearchInput struct {
	Query string `json:"query" jsonschema:"Search query string,required"`
}

// TransferInput defines parameters for transferring a project to a group.
type TransferInput struct {
	GroupID   toolutil.StringOrInt `json:"group_id"    jsonschema:"Group ID or URL-encoded path,required"`
	ProjectID toolutil.StringOrInt `json:"project_id"  jsonschema:"Project ID or URL-encoded path to transfer,required"`
}

// ListProjectsInput defines parameters for listing group projects.
type ListProjectsInput struct {
	GroupID                  toolutil.StringOrInt `json:"group_id"                  jsonschema:"Group ID or URL-encoded path,required"`
	Search                   string               `json:"search,omitempty"          jsonschema:"Filter projects by name"`
	Archived                 *bool                `json:"archived,omitempty"        jsonschema:"Filter archived projects"`
	Visibility               string               `json:"visibility,omitempty"      jsonschema:"Filter by visibility (public, internal, private)"`
	OrderBy                  string               `json:"order_by,omitempty"        jsonschema:"Order by field (id, name, path, created_at, updated_at, last_activity_at, similarity)"`
	Sort                     string               `json:"sort,omitempty"            jsonschema:"Sort direction (asc, desc)"`
	Simple                   bool                 `json:"simple,omitempty"          jsonschema:"Return limited fields"`
	Owned                    bool                 `json:"owned,omitempty"           jsonschema:"Limit to projects owned by current user"`
	Starred                  bool                 `json:"starred,omitempty"         jsonschema:"Limit to starred projects"`
	IncludeSubGroups         bool                 `json:"include_subgroups,omitempty" jsonschema:"Include projects in subgroups"`
	WithShared               *bool                `json:"with_shared,omitempty"     jsonschema:"Include shared projects"`
	Active                   *bool                `json:"active,omitempty"          jsonschema:"Filter by active (true) or inactive/archived (false) projects"`
	MinAccessLevel           int                  `json:"min_access_level,omitempty" jsonschema:"Limit to projects where the caller has at least this access level (10=Guest,20=Reporter,30=Developer,40=Maintainer,50=Owner)"`
	Topic                    string               `json:"topic,omitempty"           jsonschema:"Filter projects by topic"`
	WithCustomAttributes     bool                 `json:"with_custom_attributes,omitempty"     jsonschema:"Include custom attributes in the response"`
	WithIssuesEnabled        *bool                `json:"with_issues_enabled,omitempty"        jsonschema:"Limit to projects with issues enabled"`
	WithMergeRequestsEnabled *bool                `json:"with_merge_requests_enabled,omitempty" jsonschema:"Limit to projects with merge requests enabled"`
	WithSecurityReports      *bool                `json:"with_security_reports,omitempty"      jsonschema:"Limit to projects with security reports (Ultimate)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ProjectItem is a simplified project representation for group context.
type ProjectItem struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	PathWithNamespace string `json:"path_with_namespace"`
	Description       string `json:"description,omitempty"`
	Visibility        string `json:"visibility"`
	WebURL            string `json:"web_url"`
	DefaultBranch     string `json:"default_branch,omitempty"`
	Archived          bool   `json:"archived"`
	CreatedAt         string `json:"created_at,omitempty"`
}

// ListProjectsOutput holds a paginated list of group projects.
type ListProjectsOutput struct {
	toolutil.HintableOutput
	Projects   []ProjectItem             `json:"projects"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------.

// Create creates a new GitLab group.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if input.Name == "" {
		return Output{}, errors.New("groupCreate: name is required")
	}

	opts := &gl.CreateGroupOptions{
		Name: new(input.Name),
	}
	if input.Path != "" {
		opts.Path = new(input.Path)
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.Visibility != "" {
		opts.Visibility = new(gl.VisibilityValue(input.Visibility))
	}
	if input.ParentID != 0 {
		opts.ParentID = new(input.ParentID)
	}
	if input.OrganizationID != nil {
		opts.OrganizationID = input.OrganizationID
	}
	if input.RequestAccessEnabled != nil {
		opts.RequestAccessEnabled = input.RequestAccessEnabled
	}
	if input.LFSEnabled != nil {
		opts.LFSEnabled = input.LFSEnabled
	}
	if input.DefaultBranch != "" {
		opts.DefaultBranch = new(input.DefaultBranch)
	}
	if input.MathRenderingLimitsEnabled != nil {
		opts.MathRenderingLimitsEnabled = input.MathRenderingLimitsEnabled
	}
	if input.WebBasedCommitSigningEnabled != nil {
		opts.WebBasedCommitSigningEnabled = input.WebBasedCommitSigningEnabled
	}
	if input.AllowPersonalSnippets != nil {
		opts.AllowPersonalSnippets = input.AllowPersonalSnippets
	}
	if input.UniqueProjectDownloadLimit != nil {
		opts.UniqueProjectDownloadLimit = input.UniqueProjectDownloadLimit
	}
	if input.UniqueProjectDownloadLimitIntervalInSeconds != nil {
		opts.UniqueProjectDownloadLimitIntervalInSeconds = input.UniqueProjectDownloadLimitIntervalInSeconds
	}
	if len(input.UniqueProjectDownloadLimitAllowlist) > 0 {
		opts.UniqueProjectDownloadLimitAllowlist = &input.UniqueProjectDownloadLimitAllowlist
	}
	if len(input.UniqueProjectDownloadLimitAlertlist) > 0 {
		opts.UniqueProjectDownloadLimitAlertlist = &input.UniqueProjectDownloadLimitAlertlist
	}
	if input.AutoBanUserOnExcessiveProjectsDownload != nil {
		opts.AutoBanUserOnExcessiveProjectsDownload = input.AutoBanUserOnExcessiveProjectsDownload
	}
	applyCreateGroupExtras(input, opts)

	g, _, err := client.GL().Groups.CreateGroup(opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("groupCreate", err, "creating groups requires Owner role on the parent namespace")
		}
		return Output{}, toolutil.WrapErrWithMessage("groupCreate", err)
	}
	return ToOutput(g), nil
}

// applyCreateGroupExtras copies the remaining optional CreateGroup settings
// (1:1 audit coverage of gl.CreateGroupOptions) onto the SDK options. Split out
// of Create to keep that handler's cyclomatic complexity flat.
func applyCreateGroupExtras(input CreateInput, opts *gl.CreateGroupOptions) {
	opts.AutoDevopsEnabled = input.AutoDevopsEnabled
	opts.CRMEnabled = input.CRMEnabled
	opts.DefaultBranchProtection = input.DefaultBranchProtection //nolint:staticcheck // SA1019: mirror deprecated SDK option for 1:1 API coverage
	opts.EmailsEnabled = input.EmailsEnabled
	opts.EmailsDisabled = input.EmailsDisabled //nolint:staticcheck // SA1019: mirror deprecated SDK option for 1:1 API coverage
	opts.ExperimentFeaturesEnabled = input.ExperimentFeaturesEnabled
	opts.ExtraSharedRunnersMinutesLimit = input.ExtraSharedRunnersMinutesLimit
	opts.MembershipLock = input.MembershipLock
	opts.MentionsDisabled = input.MentionsDisabled
	opts.RequireTwoFactorAuth = input.RequireTwoFactorAuth
	opts.ShareWithGroupLock = input.ShareWithGroupLock
	opts.SharedRunnersMinutesLimit = input.SharedRunnersMinutesLimit
	opts.TwoFactorGracePeriod = input.TwoFactorGracePeriod
	opts.DefaultBranchProtectionDefaults = input.DefaultBranchProtectionDefaults.toOptions()
	if input.DuoAvailability != "" {
		opts.DuoAvailability = new(gl.DuoAvailabilityValue(input.DuoAvailability))
	}
	if input.EnabledGitAccessProtocol != "" {
		opts.EnabledGitAccessProtocol = new(gl.EnabledGitAccessProtocolValue(input.EnabledGitAccessProtocol))
	}
	if input.ProjectCreationLevel != "" {
		opts.ProjectCreationLevel = new(gl.ProjectCreationLevelValue(input.ProjectCreationLevel))
	}
	if input.SubGroupCreationLevel != "" {
		opts.SubGroupCreationLevel = new(gl.SubGroupCreationLevelValue(input.SubGroupCreationLevel))
	}
	if input.WikiAccessLevel != "" {
		opts.WikiAccessLevel = new(gl.AccessControlValue(input.WikiAccessLevel))
	}
}

// Update modifies an existing GitLab group.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, errors.New("groupUpdate: group_id is required")
	}

	opts := &gl.UpdateGroupOptions{}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.Path != "" {
		opts.Path = new(input.Path)
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.Visibility != "" {
		opts.Visibility = new(gl.VisibilityValue(input.Visibility))
	}
	if input.RequestAccessEnabled != nil {
		opts.RequestAccessEnabled = input.RequestAccessEnabled
	}
	if input.LFSEnabled != nil {
		opts.LFSEnabled = input.LFSEnabled
	}
	if input.DefaultBranch != "" {
		opts.DefaultBranch = new(input.DefaultBranch)
	}
	if input.MathRenderingLimitsEnabled != nil {
		opts.MathRenderingLimitsEnabled = input.MathRenderingLimitsEnabled
	}
	if input.WebBasedCommitSigningEnabled != nil {
		opts.WebBasedCommitSigningEnabled = input.WebBasedCommitSigningEnabled
	}
	if input.AllowPersonalSnippets != nil {
		opts.AllowPersonalSnippets = input.AllowPersonalSnippets
	}
	applyUpdateGroupPointers(input, opts)
	applyUpdateGroupEnums(input, opts)

	g, _, err := client.GL().Groups.UpdateGroup(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("groupUpdate", err, "group updates require Owner role on the group")
		}
		return Output{}, toolutil.WrapErrWithMessage("groupUpdate", err)
	}
	return ToOutput(g), nil
}

// applyUpdateGroupPointers copies the pointer-valued UpdateGroup settings
// (bool/int64 and the nested object) onto the SDK options. Pointer fields are
// passed through verbatim so an explicit zero value (e.g. false) is preserved.
// Split out of Update to keep that handler's cyclomatic complexity flat.
func applyUpdateGroupPointers(input UpdateInput, opts *gl.UpdateGroupOptions) {
	opts.AutoDevopsEnabled = input.AutoDevopsEnabled
	opts.CRMEnabled = input.CRMEnabled
	opts.DefaultBranchProtection = input.DefaultBranchProtection //nolint:staticcheck // SA1019: mirror deprecated SDK option for 1:1 API coverage
	opts.DuoFeaturesEnabled = input.DuoFeaturesEnabled
	opts.LockDuoFeaturesEnabled = input.LockDuoFeaturesEnabled
	opts.EmailsEnabled = input.EmailsEnabled
	opts.EmailsDisabled = input.EmailsDisabled //nolint:staticcheck // SA1019: mirror deprecated SDK option for 1:1 API coverage
	opts.ExperimentFeaturesEnabled = input.ExperimentFeaturesEnabled
	opts.ExtraSharedRunnersMinutesLimit = input.ExtraSharedRunnersMinutesLimit
	opts.FileTemplateProjectID = input.FileTemplateProjectID
	opts.LockMathRenderingLimitsEnabled = input.LockMathRenderingLimitsEnabled
	opts.MaxArtifactsSize = input.MaxArtifactsSize
	opts.MembershipLock = input.MembershipLock
	opts.MentionsDisabled = input.MentionsDisabled
	opts.PreventForkingOutsideGroup = input.PreventForkingOutsideGroup
	opts.PreventSharingGroupsOutsideHierarchy = input.PreventSharingGroupsOutside
	opts.RequireTwoFactorAuth = input.RequireTwoFactorAuth
	opts.ShareWithGroupLock = input.ShareWithGroupLock
	opts.SharedRunnersMinutesLimit = input.SharedRunnersMinutesLimit
	opts.TwoFactorGracePeriod = input.TwoFactorGracePeriod
	opts.OnlyAllowMergeIfPipelineSucceeds = input.OnlyAllowMergeIfPipelineSucceeds
	opts.AllowMergeOnSkippedPipeline = input.AllowMergeOnSkippedPipeline
	opts.OnlyAllowMergeIfAllDiscussionsAreResolved = input.OnlyAllowMergeIfAllDiscussionsAreResolved
	opts.UniqueProjectDownloadLimit = input.UniqueProjectDownloadLimit
	opts.UniqueProjectDownloadLimitIntervalInSeconds = input.UniqueProjectDownloadLimitIntervalInSeconds
	opts.AutoBanUserOnExcessiveProjectsDownload = input.AutoBanUserOnExcessiveProjectsDownload
	opts.DefaultBranchProtectionDefaults = input.DefaultBranchProtectionDefaults.toOptions()
	if len(input.UniqueProjectDownloadLimitAllowlist) > 0 {
		opts.UniqueProjectDownloadLimitAllowlist = &input.UniqueProjectDownloadLimitAllowlist
	}
	if len(input.UniqueProjectDownloadLimitAlertlist) > 0 {
		opts.UniqueProjectDownloadLimitAlertlist = &input.UniqueProjectDownloadLimitAlertlist
	}
}

// applyUpdateGroupEnums copies the string-and-enum-valued UpdateGroup settings
// onto the SDK options, converting each non-empty string to its typed value.
// Split out of Update to keep that handler's cyclomatic complexity flat.
func applyUpdateGroupEnums(input UpdateInput, opts *gl.UpdateGroupOptions) {
	if input.IPRestrictionRanges != "" {
		opts.IPRestrictionRanges = new(input.IPRestrictionRanges)
	}
	if input.AllowedEmailDomainsList != "" {
		opts.AllowedEmailDomainsList = new(input.AllowedEmailDomainsList)
	}
	if input.StepUpAuthRequiredOAuthProvider != "" {
		opts.StepUpAuthRequiredOAuthProvider = new(input.StepUpAuthRequiredOAuthProvider)
	}
	if input.DuoAvailability != "" {
		opts.DuoAvailability = new(gl.DuoAvailabilityValue(input.DuoAvailability))
	}
	if input.EnabledGitAccessProtocol != "" {
		opts.EnabledGitAccessProtocol = new(gl.EnabledGitAccessProtocolValue(input.EnabledGitAccessProtocol))
	}
	if input.ProjectCreationLevel != "" {
		opts.ProjectCreationLevel = new(gl.ProjectCreationLevelValue(input.ProjectCreationLevel))
	}
	if input.SubGroupCreationLevel != "" {
		opts.SubGroupCreationLevel = new(gl.SubGroupCreationLevelValue(input.SubGroupCreationLevel))
	}
	if input.SharedRunnersSetting != "" {
		opts.SharedRunnersSetting = new(gl.SharedRunnersSettingValue(input.SharedRunnersSetting))
	}
	if input.WikiAccessLevel != "" {
		opts.WikiAccessLevel = new(gl.AccessControlValue(input.WikiAccessLevel))
	}
}

// Delete removes a GitLab group.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.GroupID == "" {
		return errors.New("groupDelete: group_id is required")
	}

	opts := &gl.DeleteGroupOptions{}
	if input.PermanentlyRemove {
		opts.PermanentlyRemove = new(true)
		if input.FullPath != "" {
			opts.FullPath = new(input.FullPath)
		}
	}

	_, err := client.GL().Groups.DeleteGroup(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("groupDelete", err, "only group owners can delete groups")
		}
		return toolutil.WrapErrWithMessage("groupDelete", err)
	}
	return nil
}

// Restore restores a group that was marked for deletion.
func Restore(ctx context.Context, client *gitlabclient.Client, input RestoreInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, errors.New("groupRestore: group_id is required")
	}

	g, _, err := client.GL().Groups.RestoreGroup(string(input.GroupID), gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("groupRestore", err,
				"restoring groups requires Owner role; the group must be marked for deletion (within retention window) and not yet permanently removed")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("groupRestore", err, http.StatusNotFound,
			"the group is not marked for deletion or has already been permanently removed \u2014 only soft-deleted groups can be restored")
	}
	return ToOutput(g), nil
}

// Archive archives a GitLab group. Requires Owner role or administrator.
func Archive(ctx context.Context, client *gitlabclient.Client, input ArchiveInput) error {
	if input.GroupID == "" {
		return errors.New("groupArchive: group_id is required")
	}

	_, err := client.GL().Groups.ArchiveGroup(string(input.GroupID), gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("groupArchive", err, "archiving groups requires Owner role or administrator")
		}
		return toolutil.WrapErrWithMessage("groupArchive", err)
	}
	return nil
}

// Unarchive unarchives a GitLab group. Requires Owner role or administrator.
func Unarchive(ctx context.Context, client *gitlabclient.Client, input ArchiveInput) error {
	if input.GroupID == "" {
		return errors.New("groupUnarchive: group_id is required")
	}

	_, err := client.GL().Groups.UnarchiveGroup(string(input.GroupID), gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("groupUnarchive", err, "unarchiving groups requires Owner role or administrator")
		}
		return toolutil.WrapErrWithMessage("groupUnarchive", err)
	}
	return nil
}

// Search searches for groups by query string.
func Search(ctx context.Context, client *gitlabclient.Client, input SearchInput) (ListOutput, error) {
	if input.Query == "" {
		return ListOutput{}, errors.New("groupSearch: query is required")
	}

	groups, _, err := client.GL().Groups.SearchGroup(input.Query, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("groupSearch", err, http.StatusUnauthorized,
			"search returns groups visible to the authenticated user; pass a non-empty query string")
	}

	out := ListOutput{
		Groups: make([]Output, len(groups)),
	}
	for i, g := range groups {
		out.Groups[i] = ToOutput(g)
	}
	return out, nil
}

// TransferProject transfers a project into the group namespace.
func TransferProject(ctx context.Context, client *gitlabclient.Client, input TransferInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, errors.New("groupTransferProject: group_id is required")
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("groupTransferProject: project_id is required")
	}

	g, _, err := client.GL().Groups.TransferGroup(string(input.GroupID), string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("groupTransferProject", err,
				"transferring projects requires Owner role on both source and target groups")
		}
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint("groupTransferProject", err,
				"the project may already belong to this group, or the target group is incompatible (e.g. visibility mismatch, missing CI/CD setup)")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("groupTransferProject", err, http.StatusNotFound,
			"verify both group_id and project_id with gitlab_group_get and gitlab_project_get")
	}
	return ToOutput(g), nil
}

// ListProjects retrieves projects belonging to a group.
func ListProjects(ctx context.Context, client *gitlabclient.Client, input ListProjectsInput) (ListProjectsOutput, error) {
	if input.GroupID == "" {
		return ListProjectsOutput{}, errors.New("groupListProjects: group_id is required")
	}

	opts := &gl.ListGroupProjectsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.Archived != nil {
		opts.Archived = input.Archived
	}
	if input.Visibility != "" {
		opts.Visibility = new(gl.VisibilityValue(input.Visibility))
	}
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	if input.Simple {
		opts.Simple = new(true)
	}
	if input.Owned {
		opts.Owned = new(true)
	}
	if input.Starred {
		opts.Starred = new(true)
	}
	if input.IncludeSubGroups {
		opts.IncludeSubGroups = new(true)
	}
	if input.WithShared != nil {
		opts.WithShared = input.WithShared
	}
	if input.Active != nil {
		opts.Active = input.Active
	}
	if input.MinAccessLevel > 0 {
		opts.MinAccessLevel = new(gl.AccessLevelValue(input.MinAccessLevel))
	}
	if input.Topic != "" {
		opts.Topic = new(input.Topic)
	}
	if input.WithCustomAttributes {
		opts.WithCustomAttributes = new(true)
	}
	if input.WithIssuesEnabled != nil {
		opts.WithIssuesEnabled = input.WithIssuesEnabled
	}
	if input.WithMergeRequestsEnabled != nil {
		opts.WithMergeRequestsEnabled = input.WithMergeRequestsEnabled
	}
	if input.WithSecurityReports != nil {
		opts.WithSecurityReports = input.WithSecurityReports
	}

	projects, resp, err := client.GL().Groups.ListGroupProjects(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListProjectsOutput{}, toolutil.WrapErrWithStatusHint("groupListProjects", err, http.StatusNotFound,
			"verify group_id with gitlab_group_get \u2014 use include_subgroups=true to also list projects in descendant groups")
	}

	return ListProjectsOutput{Projects: projectItemsFromGroup(projects), Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ---------------------------------------------------------------------------
// SharedWithList
// ---------------------------------------------------------------------------.

// SharedWithListInput defines parameters for listing groups shared with a group.
type SharedWithListInput struct {
	GroupID              toolutil.StringOrInt `json:"group_id"                 jsonschema:"Group ID or URL-encoded path,required"`
	Search               string               `json:"search,omitempty"         jsonschema:"Filter shared groups by name or path"`
	MinAccessLevel       int                  `json:"min_access_level,omitempty" jsonschema:"Minimum access level the share grants (10=Guest,20=Reporter,30=Developer,40=Maintainer,50=Owner)"`
	Visibility           string               `json:"visibility,omitempty"     jsonschema:"Filter by visibility (public, internal, private)"`
	OrderBy              string               `json:"order_by,omitempty"       jsonschema:"Order shared groups by field (name, path, id)"`
	Sort                 string               `json:"sort,omitempty"           jsonschema:"Sort direction (asc, desc)"`
	SkipGroups           []int64              `json:"skip_groups,omitempty"    jsonschema:"Group IDs to exclude from the results"`
	WithCustomAttributes bool                 `json:"with_custom_attributes,omitempty" jsonschema:"Include custom attributes in the response"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// SharedWithList lists the groups that have been shared with the given group.
func SharedWithList(ctx context.Context, client *gitlabclient.Client, input SharedWithListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.GroupID == "" {
		return ListOutput{}, errors.New("SharedWithList: group_id is required. Use gitlab_group_list to find the ID first, then pass it as group_id")
	}

	opts := &gl.ListGroupsSharedWithOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.MinAccessLevel > 0 {
		opts.MinAccessLevel = new(gl.AccessLevelValue(input.MinAccessLevel))
	}
	if input.Visibility != "" {
		opts.Visibility = new(gl.VisibilityValue(input.Visibility))
	}
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	if len(input.SkipGroups) > 0 {
		opts.SkipGroups = new(input.SkipGroups)
	}
	if input.WithCustomAttributes {
		opts.WithCustomAttributes = new(true)
	}

	groups, resp, err := client.GL().Groups.ListGroupsSharedWith(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("SharedWithList", err, http.StatusNotFound,
			"verify group_id with gitlab_group_get; this lists groups shared *with* the target group (group-to-group shares)")
	}

	out := ListOutput{
		Groups:     make([]Output, len(groups)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for i, g := range groups {
		out.Groups[i] = ToOutput(g)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// InvitedList
// ---------------------------------------------------------------------------.

// InvitedListInput defines parameters for listing groups invited to a group.
type InvitedListInput struct {
	GroupID              toolutil.StringOrInt `json:"group_id"                 jsonschema:"Group ID or URL-encoded path,required"`
	Search               string               `json:"search,omitempty"         jsonschema:"Filter invited groups by name or path"`
	MinAccessLevel       int                  `json:"min_access_level,omitempty" jsonschema:"Minimum access level the invitation grants (10=Guest,20=Reporter,30=Developer,40=Maintainer,50=Owner)"`
	Relation             []string             `json:"relation,omitempty"       jsonschema:"Filter by relation (direct, inherited)"`
	WithCustomAttributes bool                 `json:"with_custom_attributes,omitempty" jsonschema:"Include custom attributes in the response"`
	OrderBy              string               `json:"order_by,omitempty"       jsonschema:"Order invited groups by field (name, path, id)"`
	Sort                 string               `json:"sort,omitempty"           jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// InvitedList lists the groups invited to the given group.
func InvitedList(ctx context.Context, client *gitlabclient.Client, input InvitedListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.GroupID == "" {
		return ListOutput{}, errors.New("InvitedList: group_id is required. Use gitlab_group_list to find the ID first, then pass it as group_id")
	}

	opts := &gl.ListInvitedGroupsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.MinAccessLevel > 0 {
		opts.MinAccessLevel = new(gl.AccessLevelValue(input.MinAccessLevel))
	}
	if len(input.Relation) > 0 {
		opts.Relation = new(input.Relation)
	}
	if input.WithCustomAttributes {
		opts.WithCustomAttributes = new(true)
	}

	groups, resp, err := client.GL().Groups.ListInvitedGroups(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("InvitedList", err, http.StatusNotFound,
			"verify group_id with gitlab_group_get; this lists groups invited to the target group")
	}

	out := ListOutput{
		Groups:     make([]Output, len(groups)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for i, g := range groups {
		out.Groups[i] = ToOutput(g)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// TransferLocationsList
// ---------------------------------------------------------------------------.

// TransferLocationsListInput defines parameters for listing possible transfer locations.
type TransferLocationsListInput struct {
	GroupID toolutil.StringOrInt `json:"group_id"         jsonschema:"Group ID or URL-encoded path,required"`
	Search  string               `json:"search,omitempty" jsonschema:"Filter candidate parent groups by name or path"`
	OrderBy string               `json:"order_by,omitempty" jsonschema:"Order candidate parent groups by field (name, path, id)"`
	Sort    string               `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// TransferLocationOutput represents a candidate parent group for a transfer.
type TransferLocationOutput struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	FullName  string `json:"full_name,omitempty"`
	FullPath  string `json:"full_path,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

// TransferLocationsListOutput holds a paginated list of transfer locations.
type TransferLocationsListOutput struct {
	toolutil.HintableOutput
	Locations  []TransferLocationOutput  `json:"locations"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// TransferLocationsList lists the parent groups available for transferring the given group.
func TransferLocationsList(ctx context.Context, client *gitlabclient.Client, input TransferLocationsListInput) (TransferLocationsListOutput, error) {
	if err := ctx.Err(); err != nil {
		return TransferLocationsListOutput{}, err
	}
	if input.GroupID == "" {
		return TransferLocationsListOutput{}, errors.New("TransferLocationsList: group_id is required. Use gitlab_group_list to find the ID first, then pass it as group_id")
	}

	opts := &gl.ListTransferLocationsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}

	locations, resp, err := client.GL().Groups.ListTransferLocations(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return TransferLocationsListOutput{}, toolutil.WrapErrWithStatusHint("TransferLocationsList", err, http.StatusNotFound,
			"verify group_id with gitlab_group_get; returns groups you can transfer this group into (requires Owner role on the target)")
	}

	out := TransferLocationsListOutput{
		Locations:  make([]TransferLocationOutput, len(locations)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for i, l := range locations {
		out.Locations[i] = TransferLocationOutput{
			ID:        l.ID,
			Name:      l.Name,
			FullName:  l.FullName,
			FullPath:  l.FullPath,
			WebURL:    l.WebURL,
			AvatarURL: l.AvatarURL,
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.
