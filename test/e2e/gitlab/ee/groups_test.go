//go:build e2e

// groups_test.go covers two licensed reads and one licensed write of the
// group tool that stand on nothing but a fresh group: the three analytics
// counts, and the group's secret push protection switch. The group's other
// licensed families, its boards, wikis, links, certificates, credentials
// and protected refs, each have a file of their own.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupanalytics"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/securitysettings"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

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
