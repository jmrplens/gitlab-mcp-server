package projects

import (
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical output shapes mirrored from client-go sub-objects. Per the 1:1
// audit policy (full nested objects) these surface every field of the SDK
// struct and are replicated locally rather than imported from sibling packages
// to preserve the zero-import-cycle constraint (C-IMPORTS).
//
// This file covers the project sub-objects surfaced on their canonical json
// keys: namespace, owner, permissions, forked_from_project, _links, statistics,
// shared_with_groups, license, container_expiration_policy, custom_attributes,
// and the approval-rule eligible_approvers / users / groups / protected_branches
// object arrays.

// formatISODatePtr renders an optional ISO date (gl.ISOTime) as YYYY-MM-DD, or
// "" when nil.
func formatISODatePtr(t *gl.ISOTime) string {
	if t == nil {
		return ""
	}
	return time.Time(*t).Format("2006-01-02")
}

// NamespaceOutput mirrors gl.ProjectNamespace, the namespace object embedded in
// project payloads on the namespace key.
type NamespaceOutput struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	FullPath  string `json:"full_path"`
	ParentID  int64  `json:"parent_id"`
	AvatarURL string `json:"avatar_url"`
	WebURL    string `json:"web_url"`
}

func namespaceOutput(n *gl.ProjectNamespace) *NamespaceOutput {
	if n == nil {
		return nil
	}
	return &NamespaceOutput{
		ID: n.ID, Name: n.Name, Path: n.Path, Kind: n.Kind,
		FullPath: n.FullPath, ParentID: n.ParentID,
		AvatarURL: n.AvatarURL, WebURL: n.WebURL,
	}
}

// ForkParentOutput mirrors gl.ForkParent, the forked_from_project object.
type ForkParentOutput struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	NameWithNamespace string `json:"name_with_namespace"`
	Path              string `json:"path"`
	PathWithNamespace string `json:"path_with_namespace"`
	HTTPURLToRepo     string `json:"http_url_to_repo"`
	WebURL            string `json:"web_url"`
	RepositoryStorage string `json:"repository_storage"`
}

func forkParentOutput(f *gl.ForkParent) *ForkParentOutput {
	if f == nil {
		return nil
	}
	return &ForkParentOutput{
		ID: f.ID, Name: f.Name, NameWithNamespace: f.NameWithNamespace,
		Path: f.Path, PathWithNamespace: f.PathWithNamespace,
		HTTPURLToRepo: f.HTTPURLToRepo, WebURL: f.WebURL,
		RepositoryStorage: f.RepositoryStorage,
	}
}

// OwnerOutput is a documented reference subset of the project owner object.
//
// Documented reference subset per doc/api/projects.md (`owner.*` attribute
// table): id, username, public_email, name, state, locked, avatar_url, web_url,
// created_at. The owner key is a *gl.User in the SDK, but the project payload
// surfaces only this identity subset rather than the full user record, so the
// remaining administrative gl.User fields (bio, projects_limit, identities,
// is_admin, etc.) are intentionally not mirrored here.
type OwnerOutput struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	Name        string `json:"name"`
	State       string `json:"state"`
	Locked      bool   `json:"locked"`
	AvatarURL   string `json:"avatar_url"`
	WebURL      string `json:"web_url"`
	PublicEmail string `json:"public_email,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
}

func ownerOutput(u *gl.User) *OwnerOutput {
	if u == nil {
		return nil
	}
	return &OwnerOutput{
		ID: u.ID, Username: u.Username, Name: u.Name, State: u.State,
		Locked: u.Locked, AvatarURL: u.AvatarURL, WebURL: u.WebURL,
		PublicEmail: u.PublicEmail,
		CreatedAt:   toolutil.FormatTimePtr(u.CreatedAt),
	}
}

// AccessOutput mirrors gl.ProjectAccess / gl.GroupAccess, the access-level pair
// embedded in the permissions object.
type AccessOutput struct {
	AccessLevel       int64 `json:"access_level"`
	NotificationLevel int64 `json:"notification_level"`
}

// PermissionsOutput mirrors gl.Permissions, the permissions object.
type PermissionsOutput struct {
	ProjectAccess *AccessOutput `json:"project_access"`
	GroupAccess   *AccessOutput `json:"group_access"`
}

func permissionsOutput(p *gl.Permissions) *PermissionsOutput {
	if p == nil {
		return nil
	}
	out := &PermissionsOutput{}
	if p.ProjectAccess != nil {
		out.ProjectAccess = &AccessOutput{
			AccessLevel:       int64(p.ProjectAccess.AccessLevel),
			NotificationLevel: int64(p.ProjectAccess.NotificationLevel),
		}
	}
	if p.GroupAccess != nil {
		out.GroupAccess = &AccessOutput{
			AccessLevel:       int64(p.GroupAccess.AccessLevel),
			NotificationLevel: int64(p.GroupAccess.NotificationLevel),
		}
	}
	return out
}

// LinksOutput mirrors gl.Links, the project _links object.
type LinksOutput struct {
	Self          string `json:"self"`
	Issues        string `json:"issues"`
	MergeRequests string `json:"merge_requests"`
	RepoBranches  string `json:"repo_branches"`
	Labels        string `json:"labels"`
	Events        string `json:"events"`
	Members       string `json:"members"`
	ClusterAgents string `json:"cluster_agents"`
}

func linksOutput(l *gl.Links) *LinksOutput {
	if l == nil {
		return nil
	}
	return &LinksOutput{
		Self: l.Self, Issues: l.Issues, MergeRequests: l.MergeRequests,
		RepoBranches: l.RepoBranches, Labels: l.Labels, Events: l.Events,
		Members: l.Members, ClusterAgents: l.ClusterAgents,
	}
}

// StatisticsOutput mirrors gl.Statistics, the project statistics object.
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

func statisticsOutput(s *gl.Statistics) *StatisticsOutput {
	if s == nil {
		return nil
	}
	return &StatisticsOutput{
		CommitCount: s.CommitCount, StorageSize: s.StorageSize,
		RepositorySize: s.RepositorySize, WikiSize: s.WikiSize,
		LFSObjectsSize: s.LFSObjectsSize, JobArtifactsSize: s.JobArtifactsSize,
		PipelineArtifactsSize: s.PipelineArtifactsSize, PackagesSize: s.PackagesSize,
		SnippetsSize: s.SnippetsSize, UploadsSize: s.UploadsSize,
		ContainerRegistrySize: s.ContainerRegistrySize,
	}
}

// SharedWithGroupOutput mirrors gl.ProjectSharedWithGroup, an element of the
// shared_with_groups array.
type SharedWithGroupOutput struct {
	GroupID          int64  `json:"group_id"`
	GroupName        string `json:"group_name"`
	GroupFullPath    string `json:"group_full_path"`
	GroupAccessLevel int64  `json:"group_access_level"`
	ExpiresAt        string `json:"expires_at,omitempty"`
}

func sharedWithGroupsOutput(groups []gl.ProjectSharedWithGroup) []SharedWithGroupOutput {
	if len(groups) == 0 {
		return nil
	}
	out := make([]SharedWithGroupOutput, 0, len(groups))
	for _, g := range groups {
		out = append(out, SharedWithGroupOutput{
			GroupID: g.GroupID, GroupName: g.GroupName,
			GroupFullPath: g.GroupFullPath, GroupAccessLevel: g.GroupAccessLevel,
			ExpiresAt: formatISODatePtr(g.ExpiresAt),
		})
	}
	return out
}

// LicenseOutput mirrors gl.ProjectLicense, the license object.
type LicenseOutput struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Nickname  string `json:"nickname"`
	HTMLURL   string `json:"html_url"`
	SourceURL string `json:"source_url"`
}

func licenseOutput(l *gl.ProjectLicense) *LicenseOutput {
	if l == nil {
		return nil
	}
	return &LicenseOutput{
		Key: l.Key, Name: l.Name, Nickname: l.Nickname,
		HTMLURL: l.HTMLURL, SourceURL: l.SourceURL,
	}
}

// ContainerExpirationPolicyOutput mirrors gl.ContainerExpirationPolicy, the
// container_expiration_policy object.
type ContainerExpirationPolicyOutput struct {
	Cadence         string `json:"cadence"`
	KeepN           int64  `json:"keep_n"`
	OlderThan       string `json:"older_than"`
	NameRegexDelete string `json:"name_regex_delete"`
	NameRegexKeep   string `json:"name_regex_keep"`
	Enabled         bool   `json:"enabled"`
	NextRunAt       string `json:"next_run_at,omitempty"`
	NameRegex       string `json:"name_regex,omitempty"`
}

func containerExpirationPolicyOutput(p *gl.ContainerExpirationPolicy) *ContainerExpirationPolicyOutput {
	if p == nil {
		return nil
	}
	return &ContainerExpirationPolicyOutput{
		Cadence: p.Cadence, KeepN: p.KeepN, OlderThan: p.OlderThan,
		NameRegexDelete: p.NameRegexDelete, NameRegexKeep: p.NameRegexKeep,
		Enabled:   p.Enabled,
		NextRunAt: toolutil.FormatTimePtr(p.NextRunAt),
		NameRegex: p.NameRegex, //nolint:staticcheck // 1:1 SDK parity; deprecated field surfaced additively.
	}
}

// CustomAttributeOutput mirrors gl.CustomAttribute, an element of the
// custom_attributes array.
type CustomAttributeOutput struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

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
	if len(out) == 0 {
		return nil
	}
	return out
}

// ApprovalGroupOutput mirrors the compact identity subset of gl.Group surfaced
// in an approval rule's groups array. The full gl.Group struct carries dozens
// of administrative fields not present in the approval-rule projection; this
// curated mirror surfaces the group identity fields that the API returns there.
type ApprovalGroupOutput struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	FullName   string `json:"full_name"`
	FullPath   string `json:"full_path"`
	Visibility string `json:"visibility"`
	AvatarURL  string `json:"avatar_url"`
	WebURL     string `json:"web_url"`
}

func approvalGroupsOutput(groups []*gl.Group) []*ApprovalGroupOutput {
	if len(groups) == 0 {
		return nil
	}
	out := make([]*ApprovalGroupOutput, 0, len(groups))
	for _, g := range groups {
		if g == nil {
			continue
		}
		out = append(out, &ApprovalGroupOutput{
			ID: g.ID, Name: g.Name, Path: g.Path, FullName: g.FullName,
			FullPath: g.FullPath, Visibility: string(g.Visibility),
			AvatarURL: g.AvatarURL, WebURL: g.WebURL,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ProtectedBranchRefOutput mirrors the compact identity subset of
// gl.ProtectedBranch surfaced in an approval rule's protected_branches array.
type ProtectedBranchRefOutput struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func protectedBranchRefsOutput(branches []*gl.ProtectedBranch) []*ProtectedBranchRefOutput {
	if len(branches) == 0 {
		return nil
	}
	out := make([]*ProtectedBranchRefOutput, 0, len(branches))
	for _, b := range branches {
		if b == nil {
			continue
		}
		out = append(out, &ProtectedBranchRefOutput{ID: b.ID, Name: b.Name})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ApproverUserOutput mirrors gl.MergeRequestApproverUser, an element of the
// project approvals approvers array (a user plus its approved_at timestamp).
type ApproverUserOutput struct {
	User       *toolutil.BasicUserOutput `json:"user"`
	ApprovedAt string                    `json:"approved_at,omitempty"`
}

func approverUsersOutput(approvers []*gl.MergeRequestApproverUser) []*ApproverUserOutput {
	if len(approvers) == 0 {
		return nil
	}
	out := make([]*ApproverUserOutput, 0, len(approvers))
	for _, a := range approvers {
		if a == nil {
			continue
		}
		out = append(out, &ApproverUserOutput{
			User:       toolutil.NewBasicUserOutput(a.User),
			ApprovedAt: toolutil.FormatTimePtr(a.ApprovedAt),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ApproverGroupOutput mirrors gl.MergeRequestApproverGroup, an element of the
// project approvals approver_groups array. The SDK nests the group identity
// under a group key (gl.MergeRequestApproverNestedGroup).
type ApproverGroupOutput struct {
	Group ApproverNestedGroupOutput `json:"group"`
}

// ApproverNestedGroupOutput mirrors gl.MergeRequestApproverNestedGroup.
type ApproverNestedGroupOutput struct {
	ID                   int64  `json:"id"`
	Name                 string `json:"name"`
	Path                 string `json:"path"`
	Description          string `json:"description"`
	Visibility           string `json:"visibility"`
	AvatarURL            string `json:"avatar_url"`
	WebURL               string `json:"web_url"`
	FullName             string `json:"full_name"`
	FullPath             string `json:"full_path"`
	LFSEnabled           bool   `json:"lfs_enabled"`
	RequestAccessEnabled bool   `json:"request_access_enabled"`
}

func approverGroupsOutput(groups []*gl.MergeRequestApproverGroup) []*ApproverGroupOutput {
	if len(groups) == 0 {
		return nil
	}
	out := make([]*ApproverGroupOutput, 0, len(groups))
	for _, g := range groups {
		if g == nil {
			continue
		}
		ng := g.Group
		out = append(out, &ApproverGroupOutput{
			Group: ApproverNestedGroupOutput{
				ID: ng.ID, Name: ng.Name, Path: ng.Path, Description: ng.Description,
				Visibility: ng.Visibility, AvatarURL: ng.AvatarURL, WebURL: ng.WebURL,
				FullName: ng.FullName, FullPath: ng.FullPath,
				LFSEnabled: ng.LFSEnabled, RequestAccessEnabled: ng.RequestAccessEnabled,
			},
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
