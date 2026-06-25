// shapes_test.go covers the nested-object converters that mirror client-go
// sub-objects on the project Output (namespace, owner, permissions,
// forked_from_project, _links, statistics, shared_with_groups, license,
// container_expiration_policy, custom_attributes) and the approval-rule /
// approval-config object arrays. Each converter is exercised with nil,
// fully-populated, and nil-element inputs to reach exhaustive coverage.
package projects

import (
	"reflect"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestShapeConverters_NilInputs verifies every pointer/slice converter returns
// nil (or an empty result) for nil or empty inputs.
func TestShapeConverters_NilInputs(t *testing.T) {
	if namespaceOutput(nil) != nil {
		t.Error("namespaceOutput(nil) != nil")
	}
	if forkParentOutput(nil) != nil {
		t.Error("forkParentOutput(nil) != nil")
	}
	if ownerOutput(nil) != nil {
		t.Error("ownerOutput(nil) != nil")
	}
	if permissionsOutput(nil) != nil {
		t.Error("permissionsOutput(nil) != nil")
	}
	if linksOutput(nil) != nil {
		t.Error("linksOutput(nil) != nil")
	}
	if statisticsOutput(nil) != nil {
		t.Error("statisticsOutput(nil) != nil")
	}
	if sharedWithGroupsOutput(nil) != nil {
		t.Error("sharedWithGroupsOutput(nil) != nil")
	}
	if licenseOutput(nil) != nil {
		t.Error("licenseOutput(nil) != nil")
	}
	if containerExpirationPolicyOutput(nil) != nil {
		t.Error("containerExpirationPolicyOutput(nil) != nil")
	}
	if customAttributesOutput(nil) != nil {
		t.Error("customAttributesOutput(nil) != nil")
	}
	if basicUserOutput(nil) != nil {
		t.Error("basicUserOutput(nil) != nil")
	}
	if basicUserOutputs(nil) != nil {
		t.Error("basicUserOutputs(nil) != nil")
	}
	if approvalGroupsOutput(nil) != nil {
		t.Error("approvalGroupsOutput(nil) != nil")
	}
	if protectedBranchRefsOutput(nil) != nil {
		t.Error("protectedBranchRefsOutput(nil) != nil")
	}
	if approverUsersOutput(nil) != nil {
		t.Error("approverUsersOutput(nil) != nil")
	}
	if approverGroupsOutput(nil) != nil {
		t.Error("approverGroupsOutput(nil) != nil")
	}
	if formatTimePtr(nil) != "" {
		t.Error("formatTimePtr(nil) != empty")
	}
	if formatISODatePtr(nil) != "" {
		t.Error("formatISODatePtr(nil) != empty")
	}
}

// TestShapeConverters_NilElements verifies slice converters skip nil elements
// and collapse all-nil slices to nil.
func TestShapeConverters_NilElements(t *testing.T) {
	if got := customAttributesOutput([]*gl.CustomAttribute{nil}); got != nil {
		t.Errorf("customAttributesOutput all-nil = %v, want nil", got)
	}
	if got := basicUserOutputs([]*gl.BasicUser{nil}); got != nil {
		t.Errorf("basicUserOutputs all-nil = %v, want nil", got)
	}
	if got := approvalGroupsOutput([]*gl.Group{nil}); got != nil {
		t.Errorf("approvalGroupsOutput all-nil = %v, want nil", got)
	}
	if got := protectedBranchRefsOutput([]*gl.ProtectedBranch{nil}); got != nil {
		t.Errorf("protectedBranchRefsOutput all-nil = %v, want nil", got)
	}
	if got := approverUsersOutput([]*gl.MergeRequestApproverUser{nil}); got != nil {
		t.Errorf("approverUsersOutput all-nil = %v, want nil", got)
	}
	if got := approverGroupsOutput([]*gl.MergeRequestApproverGroup{nil}); got != nil {
		t.Errorf("approverGroupsOutput all-nil = %v, want nil", got)
	}
}

// TestShapeConverters_SingleObjects verifies the single-object converters map
// the SDK fields to the local mirror with full fidelity, using DeepEqual against
// the complete expected output so every field is checked.
func TestShapeConverters_SingleObjects(t *testing.T) {
	now := time.Now()
	rfc := now.Format(time.RFC3339)

	checkEqual(t, "namespaceOutput",
		namespaceOutput(&gl.ProjectNamespace{ID: 1, Name: "n", Path: "p", Kind: "group", FullPath: "g/p", ParentID: 2, AvatarURL: "a", WebURL: "w"}),
		&NamespaceOutput{ID: 1, Name: "n", Path: "p", Kind: "group", FullPath: "g/p", ParentID: 2, AvatarURL: "a", WebURL: "w"})

	checkEqual(t, "forkParentOutput",
		forkParentOutput(&gl.ForkParent{ID: 9, Name: "up", NameWithNamespace: "u/up", Path: "up", PathWithNamespace: "u/up", HTTPURLToRepo: "h", WebURL: "w", RepositoryStorage: "s"}),
		&ForkParentOutput{ID: 9, Name: "up", NameWithNamespace: "u/up", Path: "up", PathWithNamespace: "u/up", HTTPURLToRepo: "h", WebURL: "w", RepositoryStorage: "s"})

	checkEqual(t, "ownerOutput",
		ownerOutput(&gl.User{ID: 7, Username: "alice", Name: "Alice", State: "active", Locked: true, AvatarURL: "a", WebURL: "w", PublicEmail: "e@x", CreatedAt: &now}),
		&OwnerOutput{ID: 7, Username: "alice", Name: "Alice", State: "active", Locked: true, AvatarURL: "a", WebURL: "w", PublicEmail: "e@x", CreatedAt: rfc})

	checkEqual(t, "permissionsOutput",
		permissionsOutput(&gl.Permissions{ProjectAccess: &gl.ProjectAccess{AccessLevel: 40, NotificationLevel: 3}, GroupAccess: &gl.GroupAccess{AccessLevel: 50, NotificationLevel: 4}}),
		&PermissionsOutput{ProjectAccess: &AccessOutput{AccessLevel: 40, NotificationLevel: 3}, GroupAccess: &AccessOutput{AccessLevel: 50, NotificationLevel: 4}})

	checkEqual(t, "permissionsOutput_empty",
		permissionsOutput(&gl.Permissions{}), &PermissionsOutput{})

	checkEqual(t, "linksOutput",
		linksOutput(&gl.Links{Self: "s", Issues: "i", MergeRequests: "m", RepoBranches: "rb", Labels: "l", Events: "e", Members: "mem", ClusterAgents: "ca"}),
		&LinksOutput{Self: "s", Issues: "i", MergeRequests: "m", RepoBranches: "rb", Labels: "l", Events: "e", Members: "mem", ClusterAgents: "ca"})

	checkEqual(t, "statisticsOutput",
		statisticsOutput(&gl.Statistics{CommitCount: 10, StorageSize: 20, RepositorySize: 30, WikiSize: 40, LFSObjectsSize: 50, JobArtifactsSize: 60, PipelineArtifactsSize: 70, PackagesSize: 80, SnippetsSize: 90, UploadsSize: 100, ContainerRegistrySize: 110}),
		&StatisticsOutput{CommitCount: 10, StorageSize: 20, RepositorySize: 30, WikiSize: 40, LFSObjectsSize: 50, JobArtifactsSize: 60, PipelineArtifactsSize: 70, PackagesSize: 80, SnippetsSize: 90, UploadsSize: 100, ContainerRegistrySize: 110})

	checkEqual(t, "licenseOutput",
		licenseOutput(&gl.ProjectLicense{Key: "mit", Name: "MIT", Nickname: "n", HTMLURL: "h", SourceURL: "s"}),
		&LicenseOutput{Key: "mit", Name: "MIT", Nickname: "n", HTMLURL: "h", SourceURL: "s"})

	checkEqual(t, "containerExpirationPolicyOutput",
		containerExpirationPolicyOutput(&gl.ContainerExpirationPolicy{Cadence: "1d", KeepN: 5, OlderThan: "30d", NameRegexDelete: "d", NameRegexKeep: "k", Enabled: true, NextRunAt: &now, NameRegex: "r"}),
		&ContainerExpirationPolicyOutput{Cadence: "1d", KeepN: 5, OlderThan: "30d", NameRegexDelete: "d", NameRegexKeep: "k", Enabled: true, NextRunAt: rfc, NameRegex: "r"})
}

// TestShapeConverters_Collections verifies the slice converters map elements
// and skip nil entries, using DeepEqual against the complete expected output.
func TestShapeConverters_Collections(t *testing.T) {
	now := time.Now()
	rfc := now.Format(time.RFC3339)
	iso := gl.ISOTime(now)
	isoDate := time.Time(iso).Format("2006-01-02")

	checkEqual(t, "sharedWithGroupsOutput",
		sharedWithGroupsOutput([]gl.ProjectSharedWithGroup{{GroupID: 3, GroupName: "g", GroupFullPath: "g/p", GroupAccessLevel: 30, ExpiresAt: &iso}}),
		[]SharedWithGroupOutput{{GroupID: 3, GroupName: "g", GroupFullPath: "g/p", GroupAccessLevel: 30, ExpiresAt: isoDate}})

	checkEqual(t, "customAttributesOutput",
		customAttributesOutput([]*gl.CustomAttribute{nil, {Key: "k", Value: "v"}}),
		[]CustomAttributeOutput{{Key: "k", Value: "v"}})

	checkEqual(t, "basicUserOutputs",
		basicUserOutputs([]*gl.BasicUser{nil, {ID: 1, Username: "u", Name: "U", State: "active", AvatarURL: "a", WebURL: "w", CreatedAt: &now}}),
		[]*BasicUserOutput{{ID: 1, Username: "u", Name: "U", State: "active", AvatarURL: "a", WebURL: "w", CreatedAt: rfc}})

	checkEqual(t, "approvalGroupsOutput",
		approvalGroupsOutput([]*gl.Group{nil, {ID: 2, Name: "g", Path: "p", FullName: "fn", FullPath: "fp", Visibility: gl.PrivateVisibility, AvatarURL: "a", WebURL: "w"}}),
		[]*ApprovalGroupOutput{{ID: 2, Name: "g", Path: "p", FullName: "fn", FullPath: "fp", Visibility: "private", AvatarURL: "a", WebURL: "w"}})

	checkEqual(t, "protectedBranchRefsOutput",
		protectedBranchRefsOutput([]*gl.ProtectedBranch{nil, {ID: 3, Name: "main"}}),
		[]*ProtectedBranchRefOutput{{ID: 3, Name: "main"}})

	checkEqual(t, "approverUsersOutput",
		approverUsersOutput([]*gl.MergeRequestApproverUser{nil, {User: &gl.BasicUser{Username: "a"}, ApprovedAt: &now}}),
		[]*ApproverUserOutput{{User: &BasicUserOutput{Username: "a"}, ApprovedAt: rfc}})

	checkEqual(t, "approverGroupsOutput",
		approverGroupsOutput([]*gl.MergeRequestApproverGroup{nil, {Group: gl.MergeRequestApproverNestedGroup{ID: 4, Name: "ag", Path: "p", Description: "d", Visibility: "private", AvatarURL: "a", WebURL: "w", FullName: "fn", FullPath: "fp", LFSEnabled: true, RequestAccessEnabled: true}}}),
		[]*ApproverGroupOutput{{Group: ApproverNestedGroupOutput{ID: 4, Name: "ag", Path: "p", Description: "d", Visibility: "private", AvatarURL: "a", WebURL: "w", FullName: "fn", FullPath: "fp", LFSEnabled: true, RequestAccessEnabled: true}}})
}

// checkEqual fails the test when got and want differ under reflect.DeepEqual.
func checkEqual[T any](t *testing.T, name string, got, want T) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %+v, want %+v", name, got, want)
	}
}

// TestUserGroupNames_NilElements covers the Markdown helpers that flatten the
// approval-rule object arrays to name strings, including nil-element skipping.
func TestUserGroupNames_NilElements(t *testing.T) {
	if userNames(nil) != nil {
		t.Error("userNames(nil) != nil")
	}
	if groupNames(nil) != nil {
		t.Error("groupNames(nil) != nil")
	}
	if got := userNames([]*BasicUserOutput{nil, {Username: "a"}}); len(got) != 1 || got[0] != "a" {
		t.Errorf("userNames = %v", got)
	}
	if got := groupNames([]*ApprovalGroupOutput{nil, {Name: "g"}}); len(got) != 1 || got[0] != "g" {
		t.Errorf("groupNames = %v", got)
	}
}

// TestToOutput_NestedObjects verifies ToOutput populates the full nested-object
// keys from a richly-populated gl.Project, plus the deprecated additive fields.
func TestToOutput_NestedObjects(t *testing.T) {
	now := time.Now()
	delAt := gl.ISOTime(now)
	p := &gl.Project{
		ID:                        1,
		Name:                      "proj",
		Owner:                     &gl.User{ID: 5, Username: "owner"},
		Permissions:               &gl.Permissions{ProjectAccess: &gl.ProjectAccess{AccessLevel: 50}},
		Links:                     &gl.Links{Self: "self"},
		Statistics:                &gl.Statistics{CommitCount: 3},
		SharedWithGroups:          []gl.ProjectSharedWithGroup{{GroupID: 9}},
		License:                   &gl.ProjectLicense{Key: "mit"},
		CustomAttributes:          []*gl.CustomAttribute{{Key: "k", Value: "v"}},
		MarkedForDeletionAt:       &delAt,
		ContainerExpirationPolicy: &gl.ContainerExpirationPolicy{Cadence: "1d"},
		TagList:                   []string{"t1"},
		IssuesAccessLevel:         gl.EnabledAccessControl,
		Mirror:                    true,
		PublicJobs:                true,
	}
	out := ToOutput(p)
	assertNestedObjects(t, out)
	if out.MarkedForDeletionAt == "" {
		t.Error("expected MarkedForDeletionAt populated")
	}
	if len(out.TagList) != 1 || out.TagList[0] != "t1" {
		t.Errorf("TagList = %+v", out.TagList)
	}
	if out.IssuesAccessLevel != "enabled" || !out.Mirror || !out.PublicJobs {
		t.Errorf("scalar mirrors not populated: %+v", out)
	}
}

// assertNestedObjects checks the full nested-object keys on a converted Output.
func assertNestedObjects(t *testing.T, out Output) {
	t.Helper()
	if out.Owner == nil || out.Owner.Username != "owner" {
		t.Errorf("Owner = %+v", out.Owner)
	}
	if out.Permissions == nil || out.Permissions.ProjectAccess == nil || out.Permissions.ProjectAccess.AccessLevel != 50 {
		t.Errorf("Permissions = %+v", out.Permissions)
	}
	if out.Links == nil || out.Links.Self != "self" {
		t.Errorf("Links = %+v", out.Links)
	}
	if out.Statistics == nil || out.Statistics.CommitCount != 3 {
		t.Errorf("Statistics = %+v", out.Statistics)
	}
	if len(out.SharedWithGroups) != 1 || out.SharedWithGroups[0].GroupID != 9 {
		t.Errorf("SharedWithGroups = %+v", out.SharedWithGroups)
	}
	if out.License == nil || out.License.Key != "mit" {
		t.Errorf("License = %+v", out.License)
	}
	if len(out.CustomAttributes) != 1 || out.CustomAttributes[0].Value != "v" {
		t.Errorf("CustomAttributes = %+v", out.CustomAttributes)
	}
	if out.ContainerExpirationPolicy == nil || out.ContainerExpirationPolicy.Cadence != "1d" {
		t.Errorf("ContainerExpirationPolicy = %+v", out.ContainerExpirationPolicy)
	}
}
