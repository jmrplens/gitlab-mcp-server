// audit_1to1_test.go contains unit tests for the 1:1 client-go fidelity audit:
// full nested output objects on gl.Group / gl.GroupMember / gl.GroupHook, the
// additional create/update input coverage of gl.CreateGroupOptions and
// gl.UpdateGroupOptions, the expanded list-filter inputs (including keyset
// pagination and order_by/sort), and the custom-headers / branch-protection
// nested input shapes. Tests use httptest to mock the GitLab API.
package groups

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// fullGroupJSON is a single-group detail fixture exercising every additive
// gl.Group field surfaced by the 1:1 audit, including nested sub-objects.
const fullGroupJSON = `{
	"id":99,"name":"infra","path":"infra","full_path":"org/infra","visibility":"private",
	"web_url":"https://gitlab.example.com/groups/org/infra",
	"membership_lock":true,"max_artifacts_size":500,"repository_storage":"default",
	"file_template_project_id":7,"share_with_group_lock":true,
	"require_two_factor_authentication":true,"two_factor_grace_period":48,
	"auto_devops_enabled":true,"emails_enabled":true,"emails_disabled":false,
	"mentions_disabled":true,"runners_token":"glrt-xxx","ldap_cn":"infra-cn","ldap_access":30,
	"shared_runners_minutes_limit":1000,"extra_shared_runners_minutes_limit":200,
	"prevent_forking_outside_group":true,"ip_restriction_ranges":"10.0.0.0/8",
	"allowed_email_domains_list":"example.com","wiki_access_level":"enabled",
	"only_allow_merge_if_pipeline_succeeds":true,"allow_merge_on_skipped_pipeline":true,
	"only_allow_merge_if_all_discussions_are_resolved":true,"default_branch_protection":2,
	"statistics":{"commit_count":12,"storage_size":34,"repository_size":5,"wiki_size":1,
		"lfs_objects_size":2,"job_artifacts_size":3,"pipeline_artifacts_size":4,
		"packages_size":6,"snippets_size":7,"uploads_size":8,"container_registry_size":9},
	"custom_attributes":[{"key":"team","value":"platform"}],
	"default_branch_protection_defaults":{"allow_force_push":true,
		"developer_can_initial_push":true,"code_owner_approval_required":true,
		"allowed_to_push":[{"access_level":40}],"allowed_to_merge":[{"access_level":30}]},
	"shared_with_groups":[{"group_id":5,"group_name":"sec","group_full_path":"org/sec",
		"group_access_level":30,"expires_at":"2026-12-31","member_role_id":2}],
	"ldap_group_links":[{"cn":"link-cn","filter":"(uid=*)","group_access":30,
		"provider":"ldapmain","member_role_id":3}],
	"saml_group_links":[{"name":"saml-link","access_level":40,"member_role_id":4,"provider":"okta"}],
	"projects":[{"id":1,"name":"p1","path_with_namespace":"org/infra/p1","visibility":"private",
		"web_url":"https://gitlab.example.com/org/infra/p1","created_at":"2026-01-15T10:00:00Z"}],
	"shared_projects":[{"id":2,"name":"p2","path_with_namespace":"org/other/p2","visibility":"private",
		"web_url":"https://gitlab.example.com/org/other/p2"}]
}`

// TestToOutput_FullNestedObjects verifies Get surfaces every additive gl.Group
// field and nested sub-object on its canonical json key (1:1 audit).
func TestToOutput_FullNestedObjects(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroup99 {
			testutil.RespondJSON(w, http.StatusOK, fullGroupJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{GroupID: "99"})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	assertGroupScalarFields(t, out)
	assertGroupNestedObjects(t, out)
	assertGroupNestedLists(t, out)
}

// assertGroupScalarFields checks the additive scalar gl.Group fields surfaced by
// ToOutput, comparing each against the fullGroupJSON fixture via a table.
func assertGroupScalarFields(t *testing.T, out Output) {
	t.Helper()
	cases := []struct {
		name string
		got  any
		want any
	}{
		{"MembershipLock", out.MembershipLock, true},
		{"MaxArtifactsSize", out.MaxArtifactsSize, int64(500)},
		{"RepositoryStorage", out.RepositoryStorage, "default"},
		{"FileTemplateProjectID", out.FileTemplateProjectID, int64(7)},
		{"ShareWithGroupLock", out.ShareWithGroupLock, true},
		{"RequireTwoFactorAuth", out.RequireTwoFactorAuth, true},
		{"TwoFactorGracePeriod", out.TwoFactorGracePeriod, int64(48)},
		{"AutoDevopsEnabled", out.AutoDevopsEnabled, true},
		{"EmailsEnabled", out.EmailsEnabled, true},
		{"MentionsDisabled", out.MentionsDisabled, true},
		{"RunnersToken", out.RunnersToken, "glrt-xxx"},
		{"LDAPCN", out.LDAPCN, "infra-cn"},
		{"LDAPAccess", out.LDAPAccess, 30},
		{"SharedRunnersMinutesLimit", out.SharedRunnersMinutesLimit, int64(1000)},
		{"ExtraSharedRunnersMinutesLimit", out.ExtraSharedRunnersMinutesLimit, int64(200)},
		{"PreventForkingOutsideGroup", out.PreventForkingOutsideGroup, true},
		{"IPRestrictionRanges", out.IPRestrictionRanges, "10.0.0.0/8"},
		{"AllowedEmailDomainsList", out.AllowedEmailDomainsList, "example.com"},
		{"WikiAccessLevel", out.WikiAccessLevel, "enabled"},
		{"OnlyAllowMergeIfPipelineSucceeds", out.OnlyAllowMergeIfPipelineSucceeds, true},
		{"AllowMergeOnSkippedPipeline", out.AllowMergeOnSkippedPipeline, true},
		{"OnlyAllowMergeIfAllDiscussionsAreResolved", out.OnlyAllowMergeIfAllDiscussionsAreResolved, true},
		{"DefaultBranchProtection", out.DefaultBranchProtection, int64(2)},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

// assertGroupNestedObjects checks the singular nested sub-objects (statistics
// and default_branch_protection_defaults) surfaced by ToOutput.
func assertGroupNestedObjects(t *testing.T, out Output) {
	t.Helper()
	if out.Statistics == nil || out.Statistics.CommitCount != 12 || out.Statistics.ContainerRegistrySize != 9 {
		t.Errorf("Statistics not mapped: %+v", out.Statistics)
	}
	d := out.DefaultBranchProtectionDefaults
	if d == nil || !d.AllowForcePush || !d.DeveloperCanInitialPush || !d.CodeOwnerApprovalRequired {
		t.Fatalf("DefaultBranchProtectionDefaults not mapped: %+v", d)
	}
	if len(d.AllowedToPush) != 1 || d.AllowedToPush[0].AccessLevel != 40 {
		t.Errorf("AllowedToPush not mapped: %+v", d.AllowedToPush)
	}
	if len(d.AllowedToMerge) != 1 || d.AllowedToMerge[0].AccessLevel != 30 {
		t.Errorf("AllowedToMerge not mapped: %+v", d.AllowedToMerge)
	}
}

// assertGroupNestedLists checks the link/attribute nested-slice sub-objects
// surfaced by ToOutput (custom attributes, shares, LDAP/SAML links).
func assertGroupNestedLists(t *testing.T, out Output) {
	t.Helper()
	if len(out.CustomAttributes) != 1 || out.CustomAttributes[0].Key != "team" || out.CustomAttributes[0].Value != "platform" {
		t.Errorf("CustomAttributes not mapped: %+v", out.CustomAttributes)
	}
	if len(out.SharedWithGroups) != 1 || out.SharedWithGroups[0].GroupID != 5 || out.SharedWithGroups[0].ExpiresAt == "" || out.SharedWithGroups[0].MemberRoleID != 2 {
		t.Errorf("SharedWithGroups not mapped: %+v", out.SharedWithGroups)
	}
	if len(out.LDAPGroupLinks) != 1 || out.LDAPGroupLinks[0].CN != "link-cn" || out.LDAPGroupLinks[0].GroupAccess != 30 || out.LDAPGroupLinks[0].MemberRoleID != 3 {
		t.Errorf("LDAPGroupLinks not mapped: %+v", out.LDAPGroupLinks)
	}
	if len(out.SAMLGroupLinks) != 1 || out.SAMLGroupLinks[0].Name != "saml-link" || out.SAMLGroupLinks[0].AccessLevel != 40 || out.SAMLGroupLinks[0].MemberRoleID != 4 {
		t.Errorf("SAMLGroupLinks not mapped: %+v", out.SAMLGroupLinks)
	}
	assertGroupEmbeddedProjects(t, out)
}

// assertGroupEmbeddedProjects checks the deprecated embedded projects and
// shared_projects slices surfaced by ToOutput.
func assertGroupEmbeddedProjects(t *testing.T, out Output) {
	t.Helper()
	if len(out.Projects) != 1 || out.Projects[0].ID != 1 || out.Projects[0].CreatedAt == "" {
		t.Errorf("Projects not mapped: %+v", out.Projects)
	}
	if len(out.SharedProjects) != 1 || out.SharedProjects[0].ID != 2 {
		t.Errorf("SharedProjects not mapped: %+v", out.SharedProjects)
	}
}

// TestToOutput_NilNestedObjects verifies the nested-object converters return nil
// for absent sub-objects, so omitempty drops them from the output.
func TestToOutput_NilNestedObjects(t *testing.T) {
	out := ToOutput(&gl.Group{ID: 1, Name: "x"})
	if out.Statistics != nil || out.DefaultBranchProtectionDefaults != nil {
		t.Errorf("expected nil nested objects, got %+v", out)
	}
	if out.CustomAttributes != nil || out.SharedWithGroups != nil || out.LDAPGroupLinks != nil {
		t.Errorf("expected nil slices, got %+v", out)
	}
	if out.SAMLGroupLinks != nil || out.Projects != nil || out.SharedProjects != nil {
		t.Errorf("expected nil slices, got %+v", out)
	}
}

// TestToOutput_NilSliceElements verifies the nested-slice converters skip nil
// pointer elements without panicking.
func TestToOutput_NilSliceElements(t *testing.T) {
	g := &gl.Group{
		ID:               1,
		CustomAttributes: []*gl.CustomAttribute{nil},
		LDAPGroupLinks:   []*gl.LDAPGroupLink{nil},
		SAMLGroupLinks:   []*gl.SAMLGroupLink{nil},
		DefaultBranchProtectionDefaults: &gl.BranchProtectionDefaults{
			AllowedToPush: []*gl.GroupAccessLevel{nil, {AccessLevel: nil}},
		},
	}
	out := ToOutput(g)
	if len(out.CustomAttributes) != 0 || len(out.LDAPGroupLinks) != 0 || len(out.SAMLGroupLinks) != 0 {
		t.Errorf("nil elements should be skipped: %+v", out)
	}
	if d := out.DefaultBranchProtectionDefaults; d == nil || len(d.AllowedToPush) != 1 || d.AllowedToPush[0].AccessLevel != 0 {
		t.Errorf("nil/zero access level handling wrong: %+v", out.DefaultBranchProtectionDefaults)
	}
}

// TestMemberToOutput_FullObjects verifies MembersList surfaces the full
// created_by, group_saml_identity, and member_role objects plus public_email
// and is_using_seat (1:1 audit; pruned scalars removed).
func TestMemberToOutput_FullObjects(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroupMembers {
			testutil.RespondJSON(w, http.StatusOK, `[{
				"id":10,"username":"u","name":"U","state":"active","access_level":40,
				"web_url":"https://gitlab.example.com/u","public_email":"u@example.com","is_using_seat":true,
				"created_by":{"id":1,"username":"admin","name":"Admin","state":"active","web_url":"https://gitlab.example.com/admin"},
				"group_saml_identity":{"extern_uid":"x","provider":"okta","saml_provider_id":3},
				"member_role":{"id":2,"name":"Role","group_id":99,"base_access_level":30,"read_code":true}
			}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := MembersList(context.Background(), client, MembersListInput{GroupID: "99"})
	if err != nil {
		t.Fatalf(fmtGroupMembersListErr, err)
	}
	m := out.Members[0]
	if m.PublicEmail != "u@example.com" || !m.IsUsingSeat {
		t.Errorf("public_email/is_using_seat not mapped: %+v", m)
	}
	if m.CreatedBy == nil || m.CreatedBy.Username != "admin" {
		t.Errorf("created_by not mapped: %+v", m.CreatedBy)
	}
	if m.GroupSAMLIdentity == nil || m.GroupSAMLIdentity.ExternUID != "x" || m.GroupSAMLIdentity.SAMLProviderID != 3 {
		t.Errorf("group_saml_identity not mapped: %+v", m.GroupSAMLIdentity)
	}
	if m.MemberRole == nil || m.MemberRole.ID != 2 || m.MemberRole.BaseAccessLevel != 30 || !m.MemberRole.ReadCode {
		t.Errorf("member_role not mapped: %+v", m.MemberRole)
	}
}

// TestMemberToOutput_NilObjects verifies the member sub-object converters return
// nil for absent objects.
func TestMemberToOutput_NilObjects(t *testing.T) {
	out := MemberToOutput(&gl.GroupMember{ID: 1})
	if out.CreatedBy != nil || out.GroupSAMLIdentity != nil || out.MemberRole != nil {
		t.Errorf("expected nil member sub-objects, got %+v", out)
	}
}

// TestCreate_AuditFields verifies Create forwards the additive
// gl.CreateGroupOptions fields, including the nested
// default_branch_protection_defaults object and the enum string values.
func TestCreate_AuditFields(t *testing.T) {
	var body string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathGroups {
			bufBytes, _ := io.ReadAll(r.Body)
			body = string(bufBytes)
			testutil.RespondJSON(w, http.StatusCreated, `{"id":99,"name":"infra","path":"infra","full_path":"infra","visibility":"private","web_url":"https://gitlab.example.com/groups/infra"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		Name:                           "infra",
		AutoDevopsEnabled:              new(true),
		DefaultBranchProtection:        new(int64(2)),
		DuoAvailability:                "default_on",
		EmailsEnabled:                  new(true),
		EmailsDisabled:                 new(false),
		EnabledGitAccessProtocol:       "ssh",
		ExperimentFeaturesEnabled:      new(true),
		ExtraSharedRunnersMinutesLimit: new(int64(100)),
		MembershipLock:                 new(true),
		MentionsDisabled:               new(true),
		ProjectCreationLevel:           "maintainer",
		RequireTwoFactorAuth:           new(true),
		ShareWithGroupLock:             new(true),
		SharedRunnersMinutesLimit:      new(int64(500)),
		SubGroupCreationLevel:          "owner",
		TwoFactorGracePeriod:           new(int64(48)),
		WikiAccessLevel:                "enabled",
		DefaultBranchProtectionDefaults: &BranchProtectionDefaultsInput{
			AllowForcePush: new(true),
			AllowedToPush:  []int{40},
			AllowedToMerge: []int{30},
		},
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	for _, want := range []string{
		`"auto_devops_enabled":true`, `"default_branch_protection":2`, `"duo_availability":"default_on"`,
		`"emails_enabled":true`, `"emails_disabled":false`, `"enabled_git_access_protocol":"ssh"`,
		`"experiment_features_enabled":true`, `"extra_shared_runners_minutes_limit":100`,
		`"membership_lock":true`, `"mentions_disabled":true`, `"project_creation_level":"maintainer"`,
		`"require_two_factor_authentication":true`, `"share_with_group_lock":true`,
		`"shared_runners_minutes_limit":500`, `"subgroup_creation_level":"owner"`,
		`"two_factor_grace_period":48`, `"wiki_access_level":"enabled"`,
		`"default_branch_protection_defaults"`, `"allow_force_push":true`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("create request body missing %q:\n%s", want, body)
		}
	}
}

// TestUpdate_AuditFields verifies Update forwards the additive
// gl.UpdateGroupOptions fields covered by the 1:1 audit.
func TestUpdate_AuditFields(t *testing.T) {
	var body string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathGroup99 {
			bufBytes, _ := io.ReadAll(r.Body)
			body = string(bufBytes)
			testutil.RespondJSON(w, http.StatusOK, `{"id":99,"name":"infra","path":"infra","full_path":"infra","visibility":"private","web_url":"https://gitlab.example.com/groups/infra"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Update(context.Background(), client, UpdateInput{
		GroupID:                                     "99",
		AllowMergeOnSkippedPipeline:                 new(true),
		AllowedEmailDomainsList:                     "example.com",
		AutoBanUserOnExcessiveProjectsDownload:      new(true),
		AutoDevopsEnabled:                           new(true),
		DefaultBranchProtection:                     new(int64(2)),
		DuoAvailability:                             "default_off",
		DuoFeaturesEnabled:                          new(true),
		EmailsDisabled:                              new(false),
		EmailsEnabled:                               new(true),
		EnabledGitAccessProtocol:                    "all",
		ExperimentFeaturesEnabled:                   new(true),
		ExtraSharedRunnersMinutesLimit:              new(int64(50)),
		FileTemplateProjectID:                       new(int64(7)),
		IPRestrictionRanges:                         "10.0.0.0/8",
		LockDuoFeaturesEnabled:                      new(true),
		LockMathRenderingLimitsEnabled:              new(true),
		MaxArtifactsSize:                            new(int64(500)),
		MembershipLock:                              new(true),
		MentionsDisabled:                            new(true),
		OnlyAllowMergeIfAllDiscussionsAreResolved:   new(true),
		OnlyAllowMergeIfPipelineSucceeds:            new(true),
		PreventForkingOutsideGroup:                  new(true),
		PreventSharingGroupsOutside:                 new(true),
		ProjectCreationLevel:                        "developer",
		RequireTwoFactorAuth:                        new(true),
		ShareWithGroupLock:                          new(true),
		SharedRunnersMinutesLimit:                   new(int64(500)),
		SharedRunnersSetting:                        "enabled",
		StepUpAuthRequiredOAuthProvider:             "okta",
		SubGroupCreationLevel:                       "maintainer",
		TwoFactorGracePeriod:                        new(int64(24)),
		UniqueProjectDownloadLimit:                  new(int64(5)),
		UniqueProjectDownloadLimitIntervalInSeconds: new(int64(60)),
		UniqueProjectDownloadLimitAllowlist:         []string{"safe"},
		UniqueProjectDownloadLimitAlertlist:         []int64{1},
		WikiAccessLevel:                             "private",
		DefaultBranchProtectionDefaults:             &BranchProtectionDefaultsInput{CodeOwnerApprovalRequired: new(true)},
	})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	for _, want := range []string{
		`"allow_merge_on_skipped_pipeline":true`, `"allowed_email_domains_list":"example.com"`,
		`"auto_ban_user_on_excessive_projects_download":true`, `"auto_devops_enabled":true`,
		`"default_branch_protection":2`, `"duo_availability":"default_off"`, `"duo_features_enabled":true`,
		`"emails_disabled":false`, `"emails_enabled":true`, `"enabled_git_access_protocol":"all"`,
		`"experiment_features_enabled":true`, `"extra_shared_runners_minutes_limit":50`,
		`"file_template_project_id":7`, `"ip_restriction_ranges":"10.0.0.0/8"`,
		`"lock_duo_features_enabled":true`, `"lock_math_rendering_limits_enabled":true`,
		`"max_artifacts_size":500`, `"membership_lock":true`, `"mentions_disabled":true`,
		`"only_allow_merge_if_all_discussions_are_resolved":true`, `"only_allow_merge_if_pipeline_succeeds":true`,
		`"prevent_forking_outside_group":true`, `"prevent_sharing_groups_outside_hierarchy":true`,
		`"project_creation_level":"developer"`, `"require_two_factor_authentication":true`,
		`"share_with_group_lock":true`, `"shared_runners_minutes_limit":500`,
		`"shared_runners_setting":"enabled"`, `"step_up_auth_required_oauth_provider":"okta"`,
		`"subgroup_creation_level":"maintainer"`, `"two_factor_grace_period":24`,
		`"unique_project_download_limit":5`, `"unique_project_download_limit_interval_in_seconds":60`,
		`"unique_project_download_limit_allowlist":["safe"]`, `"unique_project_download_limit_alertlist":[1]`,
		`"wiki_access_level":"private"`, `"code_owner_approval_required":true`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("update request body missing %q:\n%s", want, body)
		}
	}
}

// TestList_AuditFiltersAndKeyset verifies List forwards the new filters and
// keyset-pagination parameters.
func TestList_AuditFiltersAndKeyset(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathGroups {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		checks := map[string]string{
			"min_access_level":       "30",
			"repository_storage":     "nfs-01",
			"active":                 "true",
			"archived":               "false",
			"marked_for_deletion_on": "2026-06-01",
			"pagination":             "keyset",
			"page_token":             "abc",
		}
		for k, want := range checks {
			if got := q.Get(k); got != want {
				t.Errorf("query %s = %q, want %q", k, got, want)
			}
		}
		testutil.RespondJSON(w, http.StatusOK, groupListJSON)
	}))

	active, archived := true, false
	_, err := List(context.Background(), client, ListInput{
		MinAccessLevel:        30,
		RepositoryStorage:     "nfs-01",
		Active:                &active,
		Archived:              &archived,
		MarkedForDeletionOn:   "2026-06-01",
		KeysetPaginationInput: keysetKeyset(),
	})
	if err != nil {
		t.Fatalf(fmtGroupListErr, err)
	}
}

// TestList_InvalidMarkedForDeletionOn verifies an unparseable date is silently
// dropped rather than causing an error.
func TestList_InvalidMarkedForDeletionOn(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroups {
			if r.URL.Query().Get("marked_for_deletion_on") != "" {
				t.Error("unparseable marked_for_deletion_on should be dropped")
			}
			testutil.RespondJSON(w, http.StatusOK, groupListJSON)
			return
		}
		http.NotFound(w, r)
	}))
	if _, err := List(context.Background(), client, ListInput{MarkedForDeletionOn: "not-a-date"}); err != nil {
		t.Fatalf(fmtGroupListErr, err)
	}
}

// TestSubgroupsList_AuditFilters verifies the additive subgroup filters and
// keyset parameters reach the request.
func TestSubgroupsList_AuditFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathGroupSubgroups {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		checks := map[string]string{
			"visibility":             "private",
			"top_level_only":         "true",
			"with_custom_attributes": "true",
			"repository_storage":     "nfs-02",
			"active":                 "true",
			"archived":               "false",
			"marked_for_deletion_on": "2026-06-02",
			"pagination":             "keyset",
		}
		for k, want := range checks {
			if got := q.Get(k); got != want {
				t.Errorf("query %s = %q, want %q", k, got, want)
			}
		}
		if !strings.Contains(r.URL.RawQuery, "skip_groups") {
			t.Error("skip_groups missing")
		}
		testutil.RespondJSON(w, http.StatusOK, subgroupsJSON)
	}))

	active, archived := true, false
	_, err := SubgroupsList(context.Background(), client, SubgroupsListInput{
		GroupID:               "99",
		Visibility:            "private",
		TopLevelOnly:          true,
		WithCustomAttributes:  true,
		SkipGroups:            []int64{7},
		RepositoryStorage:     "nfs-02",
		Active:                &active,
		Archived:              &archived,
		MarkedForDeletionOn:   "2026-06-02",
		KeysetPaginationInput: keysetKeyset(),
	})
	if err != nil {
		t.Fatalf(fmtSubgroupsListErr, err)
	}
}

// TestSubgroupsList_InvalidMarkedForDeletionOn covers the unparseable-date
// branch in SubgroupsList.
func TestSubgroupsList_InvalidMarkedForDeletionOn(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroupSubgroups {
			if r.URL.Query().Get("marked_for_deletion_on") != "" {
				t.Error("unparseable marked_for_deletion_on should be dropped")
			}
			testutil.RespondJSON(w, http.StatusOK, subgroupsJSON)
			return
		}
		http.NotFound(w, r)
	}))
	if _, err := SubgroupsList(context.Background(), client, SubgroupsListInput{GroupID: "99", MarkedForDeletionOn: "bad"}); err != nil {
		t.Fatalf(fmtSubgroupsListErr, err)
	}
}

// TestListProjects_AuditFilters verifies the additive group-project filters.
func TestListProjects_AuditFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/groups/99/projects" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		checks := map[string]string{
			"active":                      "true",
			"min_access_level":            "30",
			"topic":                       "go",
			"with_custom_attributes":      "true",
			"with_issues_enabled":         "true",
			"with_merge_requests_enabled": "true",
			"with_security_reports":       "true",
			"pagination":                  "keyset",
		}
		for k, want := range checks {
			if got := q.Get(k); got != want {
				t.Errorf("query %s = %q, want %q", k, got, want)
			}
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"p","path_with_namespace":"org/p","visibility":"private","web_url":"https://gitlab.example.com/org/p"}]`)
	}))

	active, t1, t2, t3 := true, true, true, true
	out, err := ListProjects(context.Background(), client, ListProjectsInput{
		GroupID:                  "99",
		Active:                   &active,
		MinAccessLevel:           30,
		Topic:                    "go",
		WithCustomAttributes:     true,
		WithIssuesEnabled:        &t1,
		WithMergeRequestsEnabled: &t2,
		WithSecurityReports:      &t3,
		KeysetPaginationInput:    keysetKeyset(),
	})
	if err != nil {
		t.Fatalf("ListProjects() unexpected error: %v", err)
	}
	if len(out.Projects) != 1 {
		t.Fatalf("len(out.Projects) = %d, want 1", len(out.Projects))
	}
}

// TestMembersList_AuditFilters verifies query/user_ids/show_seat_info/order_by/
// sort/keyset reach the members request.
func TestMembersList_AuditFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathGroupMembers {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if q.Get("order_by") != "access_level" || q.Get("sort") != "desc" || q.Get("show_seat_info") != "true" {
			t.Errorf("order_by/sort/show_seat_info missing: %s", r.URL.RawQuery)
		}
		if q.Get("pagination") != "keyset" {
			t.Errorf("pagination missing: %s", r.URL.RawQuery)
		}
		if !strings.Contains(r.URL.RawQuery, "user_ids") {
			t.Errorf("user_ids missing: %s", r.URL.RawQuery)
		}
		testutil.RespondJSON(w, http.StatusOK, groupMembersJSON)
	}))

	seat := true
	_, err := MembersList(context.Background(), client, MembersListInput{
		GroupID:               "99",
		OrderBy:               "access_level",
		Sort:                  "desc",
		ShowSeatInfo:          &seat,
		UserIDs:               []int64{10, 11},
		KeysetPaginationInput: keysetKeyset(),
	})
	if err != nil {
		t.Fatalf(fmtGroupMembersListErr, err)
	}
}

// TestGet_AuditParams verifies with_custom_attributes/with_projects/order_by/
// sort/keyset reach the single-group request.
func TestGet_AuditParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathGroup99 {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if q.Get("with_custom_attributes") != "true" || q.Get("with_projects") != "true" {
			t.Errorf("with_* params missing: %s", r.URL.RawQuery)
		}
		if q.Get("order_by") != "name" || q.Get("sort") != "asc" || q.Get("pagination") != "keyset" {
			t.Errorf("order_by/sort/pagination missing: %s", r.URL.RawQuery)
		}
		testutil.RespondJSON(w, http.StatusOK, groupDetailJSON)
	}))

	_, err := Get(context.Background(), client, GetInput{
		GroupID:               "99",
		WithCustomAttributes:  true,
		WithProjects:          new(true),
		OrderBy:               "name",
		Sort:                  "asc",
		KeysetPaginationInput: keysetKeyset(),
	})
	if err != nil {
		t.Fatalf(fmtGroupGetErr, err)
	}
}

// TestInvitedAndTransferLocations_OrderSort verifies order_by/sort reach the
// invited-groups and transfer-locations requests.
func TestInvitedAndTransferLocations_OrderSort(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/groups/99/invited_groups", "/api/v4/groups/99/transfer_locations":
			q := r.URL.Query()
			if q.Get("order_by") != "name" || q.Get("sort") != "desc" {
				t.Errorf("order_by/sort missing on %s: %s", r.URL.Path, r.URL.RawQuery)
			}
			testutil.RespondJSON(w, http.StatusOK, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))

	if _, err := InvitedList(context.Background(), client, InvitedListInput{GroupID: "99", OrderBy: "name", Sort: "desc"}); err != nil {
		t.Fatalf("InvitedList() unexpected error: %v", err)
	}
	if _, err := TransferLocationsList(context.Background(), client, TransferLocationsListInput{GroupID: "99", OrderBy: "name", Sort: "desc"}); err != nil {
		t.Fatalf("TransferLocationsList() unexpected error: %v", err)
	}
}

// TestHook_AuditCustomFields verifies AddHook forwards custom_webhook_template
// and custom_headers, and that GetHook surfaces them as redacted []objects.
func TestHook_AuditCustomFields(t *testing.T) {
	var body string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/groups/99/hooks":
			bufBytes, _ := io.ReadAll(r.Body)
			body = string(bufBytes)
			testutil.RespondJSON(w, http.StatusCreated, `{"id":10,"url":"https://example.com/hook","group_id":99,
				"custom_webhook_template":"{\"a\":1}","custom_headers":[{"key":"X-Token","value":"secret"}],
				"url_variables":[{"key":"env","value":"prod"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))

	out, err := AddHook(context.Background(), client, AddHookInput{
		GroupID: "99",
		HookInput: HookInput{
			URL:                   "https://example.com/hook",
			CustomWebhookTemplate: `{"a":1}`,
			CustomHeaders:         []HookCustomHeaderInput{{Key: "X-Token", Value: "secret"}},
		},
	})
	if err != nil {
		t.Fatalf("AddHook() unexpected error: %v", err)
	}
	if !strings.Contains(body, `"custom_webhook_template":"{\"a\":1}"`) {
		t.Errorf("custom_webhook_template missing from request: %s", body)
	}
	if !strings.Contains(body, `"custom_headers"`) || !strings.Contains(body, `"X-Token"`) || !strings.Contains(body, `"secret"`) {
		t.Errorf("custom_headers missing from request: %s", body)
	}
	if out.CustomWebhookTemplate != `{"a":1}` {
		t.Errorf("custom_webhook_template = %q, want object template", out.CustomWebhookTemplate)
	}
	if len(out.CustomHeaders) != 1 || out.CustomHeaders[0].Key != "X-Token" || out.CustomHeaders[0].Value != "" {
		t.Errorf("custom_headers should expose key only (value redacted): %+v", out.CustomHeaders)
	}
	if len(out.URLVariables) != 1 || out.URLVariables[0].Key != "env" || out.URLVariables[0].Value != "" {
		t.Errorf("url_variables should expose key only (value redacted): %+v", out.URLVariables)
	}
}

// TestHookToOutput_NilCustomHeaderElement verifies hookToOutput skips nil
// custom-header pointers without panicking.
func TestHookToOutput_NilCustomHeaderElement(t *testing.T) {
	out := hookToOutput(&gl.GroupHook{
		ID:            1,
		CustomHeaders: []*gl.HookCustomHeader{nil, {Key: "X-Real", Value: "secret"}},
	})
	if len(out.CustomHeaders) != 1 || out.CustomHeaders[0].Key != "X-Real" || out.CustomHeaders[0].Value != "" {
		t.Errorf("nil custom header not skipped / value not redacted: %+v", out.CustomHeaders)
	}
}

// TestListHooks_AuditParams verifies order_by/sort/keyset reach the hooks list
// request.
func TestListHooks_AuditParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/groups/99/hooks" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if q.Get("order_by") != "id" || q.Get("sort") != "asc" || q.Get("pagination") != "keyset" {
			t.Errorf("order_by/sort/pagination missing: %s", r.URL.RawQuery)
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	_, err := ListHooks(context.Background(), client, ListHooksInput{
		GroupID:               "99",
		OrderBy:               "id",
		Sort:                  "asc",
		KeysetPaginationInput: keysetKeyset(),
	})
	if err != nil {
		t.Fatalf("ListHooks() unexpected error: %v", err)
	}
}

// keysetKeyset returns a populated keyset-pagination input for tests.
func keysetKeyset() toolutil.KeysetPaginationInput {
	return toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "abc"}
}
