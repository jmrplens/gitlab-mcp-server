//go:build e2e

// groups_test.go covers two licensed reads and one licensed write of the
// group tool that stand on nothing but a fresh group: the three analytics
// counts, and the group's secret push protection switch; and the one
// Ultimate scenario of the Free group create, update and read, the
// download-limit fields only an Ultimate schema carries. The group's other
// licensed families, its boards, wikis, links, certificates, credentials
// and protected refs, each have a file of their own.

package ee

import (
	"context"
	"strconv"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupanalytics"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/securitysettings"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The download limit the Ultimate group is created with: ten unique
// projects in five minutes, with the ban on exceeding it.
const (
	downloadLimit         = int64(10)
	downloadLimitInterval = int64(300)
)

// downloadLimitFields is the download limit as the group actions take it:
// the limit, its interval, the two empty lists and the ban.
var downloadLimitFields = map[string]any{
	"unique_project_download_limit":                     downloadLimit,
	"unique_project_download_limit_interval_in_seconds": downloadLimitInterval,
	"unique_project_download_limit_allowlist":           []string{},
	"unique_project_download_limit_alertlist":           []int64{},
	"auto_ban_user_on_excessive_projects_download":      true,
}

// assertDownloadLimit checks the three settings an answer carries about the
// download limit, which GitLab exposes to the group's owner on Ultimate.
func assertDownloadLimit(e *harness.Env, what string, group groups.DetailOutput) {
	e.T.Helper()
	if group.UniqueProjectDownloadLimit == nil || *group.UniqueProjectDownloadLimit != downloadLimit {
		e.T.Errorf("%s answers the download limit %s, want %d", what, int64Word(group.UniqueProjectDownloadLimit), downloadLimit)
	}
	if group.UniqueProjectDownloadLimitIntervalSecs == nil || *group.UniqueProjectDownloadLimitIntervalSecs != downloadLimitInterval {
		e.T.Errorf("%s answers the limit interval %s, want %d", what, int64Word(group.UniqueProjectDownloadLimitIntervalSecs), downloadLimitInterval)
	}
	if group.AutoBanUserOnExcessiveProjectsDownload == nil || !*group.AutoBanUserOnExcessiveProjectsDownload {
		e.T.Errorf("%s answers the auto-ban %v, want it on", what, group.AutoBanUserOnExcessiveProjectsDownload)
	}
}

// int64Word spells an optional number for a message: the number, or absent.
func int64Word(value *int64) string {
	if value == nil {
		return "absent"
	}
	return strconv.FormatInt(*value, 10)
}

// TestGroupUpdate_UltimateDownloadLimit_FieldsRoundTrip creates a group on
// every surface with the download-limit fields, then sets them through the
// update and reads them back off the update's answer and off a read.
//
// The create is sent the fields and is asserted only to be accepted: GitLab's
// create route drops them, as the live API record says by declaring the five
// on PUT /groups/:id and none of them on POST /groups, while the create
// input still publishes them because client-go's CreateGroupOptions carries
// them. The update route is where the settings land.
//
// Replaces: TestIndividual_GroupCreateUltimateFields
func TestGroupUpdate_UltimateDownloadLimit_FieldsRoundTrip(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.Tier(edition.Ultimate)))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		name := e.Name("ult")

		created := harness.Do[groups.DetailOutput](s, actionGroupCreate, withParams(downloadLimitFields, map[string]any{
			"name": name, "path": name, "visibility": "private",
		}))
		if created.ID == 0 || created.Path != name {
			e.T.Fatalf("group create answered %+v, want a group at %q with an ID", created, name)
		}
		e.Defer("group "+created.FullPath, func(ctx context.Context) error {
			return fixture.DeleteGroup(ctx, e.Client(), created.ID, created.FullPath)
		})
		params := map[string]any{"group_id": strconv.FormatInt(created.ID, 10)}

		updated := harness.Do[groups.DetailOutput](s, actionGroupUpdate, withParams(downloadLimitFields, params))
		assertDownloadLimit(e, "group update", updated)
		reread := harness.Do[groups.DetailOutput](s, actionGroupGet, params)
		assertDownloadLimit(e, "group get after the update", reread)
	})
}

// TestGroupAnalytics_FreshGroup_CountsAnswerForThePath asks a fresh group
// for its recently created issues, merge requests and members on every
// surface, and checks each answer names the group it was asked about.
//
// Replaces: TestEE_MetaGroupEnterpriseOperations
func TestGroupAnalytics_FreshGroup_CountsAnswerForThePath(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("analytics"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)
		params := map[string]any{"group_path": group.Path}

		issues := harness.Do[groupanalytics.IssuesCountOutput](s, actionGroupAnalyticsIssuesCount, params)
		if issues.GroupPath != group.Path {
			e.T.Errorf("analytics_issues_count answered for %q, want %q", issues.GroupPath, group.Path)
		}
		mrs := harness.Do[groupanalytics.MRCountOutput](s, actionGroupAnalyticsMRCount, params)
		if mrs.GroupPath != group.Path {
			e.T.Errorf("analytics_mr_count answered for %q, want %q", mrs.GroupPath, group.Path)
		}
		members := harness.Do[groupanalytics.MembersCountOutput](s, actionGroupAnalyticsMembersCount, params)
		if members.GroupPath != group.Path {
			e.T.Errorf("analytics_members_count answered for %q, want %q", members.GroupPath, group.Path)
		}
	})
}

// TestGroupSecuritySettings_Update_TurnsSecretPushProtectionOn turns secret
// push protection on for a fresh group on every surface and reads the
// switch off the answer.
//
// Replaces: TestEE_MetaGroupEnterpriseOperations
func TestGroupSecuritySettings_Update_TurnsSecretPushProtectionOn(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.Tier(edition.Ultimate)))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("secset"))

		updated := harness.Do[securitysettings.GroupOutput](s, actionGroupSecuritySettingsUpdate, map[string]any{
			"group_id": group.IDParam(), "secret_push_protection_enabled": true,
		})
		if !updated.SecretPushProtectionEnabled {
			e.T.Errorf("security_settings_update answered %+v, want secret push protection on", updated)
		}
	})
}
