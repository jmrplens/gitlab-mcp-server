//go:build e2e

// admin_settings_test.go ports the instance-settings half of the old admin
// meta suite: topics, application and appearance settings, instance metadata
// and plan limits, broadcast messages, feature flags, system hooks, Sidekiq
// metrics, OAuth applications, custom attributes and the dependency proxy
// purge.
//
// Every scenario here mutates instance-global state and runs through the
// gitlab_admin meta tool on the meta surface, which is the surface the old
// suite used and the one the run's administrator token serves the group on.
// The dynamic and individual surfaces are exercised for the admin reads by
// the reads-and-previews sweeps, so a meta-only write scenario loses no
// coverage the sweeps do not already give. Each declares the instance-global
// lock so two admin scenarios never race for the same instance state, and the
// admin need so a non-administrator run is skipped with a reason rather than
// buried under a wall of 403s.

package common

import (
	"context"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/appearance"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/applications"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/appstatistics"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/broadcastmessages"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/customattributes"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/features"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/metadata"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/planlimits"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/settings"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/sidekiq"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/systemhooks"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/topics"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/users"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// adminDeferDelete registers a best-effort cleanup that removes a resource
// through the server after the test, so a failure before the body reaches its
// own delete does not leave instance-global state behind. A resource the body
// already deleted answers not found, which is logged rather than reported as a
// cleanup failure: the delete the test asserts on is the one in the body.
func adminDeferDelete(e *harness.Env, s *harness.Session, label string, action harness.ActionID, params map[string]any) {
	e.Defer(label, func(context.Context) error {
		if _, err := harness.Try[toolutil.DeleteOutput](s, action, params, harness.For(harness.PurposeCleanup)); err != nil {
			e.T.Logf("best-effort cleanup of %s answered: %v", label, err)
		}
		return nil
	})
}

// TestAdmin_Topics_Lifecycle lists the instance topics, creates one, reads it
// back, renames it and deletes it, asserting the id round-trips through each
// call.
//
// Replaces: TestMeta_AdminTopics, TestMeta_Admin
func TestAdmin_Topics_Lifecycle(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin), harness.Locks(harness.LockInstanceGlobal))
	s := e.On(harness.SurfaceMeta)

	before := harness.Do[topics.ListOutput](s, actionAdminTopicList, nil)
	e.T.Logf("%d topic(s) before the create", len(before.Topics))

	name := e.Name("topic")
	created := harness.Do[topics.CreateOutput](s, actionAdminTopicCreate, map[string]any{"name": name, "title": "E2E " + name})
	if created.Topic.ID == 0 {
		e.T.Fatalf("topic_create answered %+v, want a topic with an ID", created.Topic)
	}
	adminDeferDelete(e, s, "topic "+name, actionAdminTopicDelete, map[string]any{"topic_id": created.Topic.ID})

	got := harness.Do[topics.GetOutput](s, actionAdminTopicGet, map[string]any{"topic_id": created.Topic.ID})
	if got.Topic.ID != created.Topic.ID {
		e.T.Errorf("topic_get answered %d, want the created %d", got.Topic.ID, created.Topic.ID)
	}

	updated := harness.Do[topics.UpdateOutput](s, actionAdminTopicUpdate, map[string]any{
		"topic_id": created.Topic.ID, "description": "updated by the e2e suite",
	})
	if updated.Topic.ID != created.Topic.ID {
		e.T.Errorf("topic_update answered %d, want %d", updated.Topic.ID, created.Topic.ID)
	}

	harness.DoVoid(s, actionAdminTopicDelete, map[string]any{"topic_id": created.Topic.ID})
}

// TestAdmin_SettingsAndAppearance reads the application settings, sets the
// default branch name, reads the appearance and changes its title.
//
// Replaces: TestMeta_AdminSettingsAppearance, TestMeta_Admin
func TestAdmin_SettingsAndAppearance(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin), harness.Locks(harness.LockInstanceGlobal))
	s := e.On(harness.SurfaceMeta)

	got := harness.Do[settings.GetOutput](s, actionAdminSettingsGet, nil)
	if len(got.Settings) == 0 {
		e.T.Fatalf("settings_get answered an empty settings map")
	}

	updated := harness.Do[settings.UpdateOutput](s, actionAdminSettingsUpdate, map[string]any{
		"settings": map[string]any{"default_branch_name": "main"},
	})
	if len(updated.Settings) == 0 {
		e.T.Errorf("settings_update answered an empty settings map")
	}

	appearanceGet := harness.Do[appearance.GetOutput](s, actionAdminAppearanceGet, nil)
	e.T.Logf("appearance title before: %q", appearanceGet.Appearance.Title)

	const wantTitle = "E2E GitLab"
	appearanceUpd := harness.Do[appearance.UpdateOutput](s, actionAdminAppearanceUpd, map[string]any{"title": wantTitle})
	if appearanceUpd.Appearance.Title != wantTitle {
		e.T.Errorf("appearance_update answered title %q, want %q", appearanceUpd.Appearance.Title, wantTitle)
	}
}

// TestAdmin_InstanceMetadata reads the instance metadata, application
// statistics and plan limits, and raises then restores the default plan's
// PyPI file-size limit.
//
// Replaces: TestMeta_AdminPlanLimitsMetadata, TestMeta_AdminPlanLimitsChange
func TestAdmin_InstanceMetadata(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin), harness.Locks(harness.LockInstanceGlobal))
	s := e.On(harness.SurfaceMeta)

	meta := harness.Do[metadata.GetOutput](s, actionAdminMetadataGet, nil)
	if meta.Version == "" {
		e.T.Errorf("metadata_get answered an empty version")
	}

	stats := harness.Do[appstatistics.GetOutput](s, actionAdminAppStatsGet, nil)
	e.T.Logf("application statistics: %d users, %d projects", stats.Users, stats.Projects)

	const planName = "default"
	current := harness.Do[planlimits.GetOutput](s, actionAdminPlanLimitsGet, map[string]any{"plan_name": planName})
	original := current.PyPiMaxFileSize
	raised := original + 1
	e.Defer("restore plan limit", func(context.Context) error {
		if _, err := harness.Try[planlimits.ChangeOutput](s, actionAdminPlanLimitsChg, map[string]any{
			"plan_name": planName, "pypi_max_file_size": original,
		}, harness.For(harness.PurposeCleanup)); err != nil {
			e.T.Logf("restoring the default plan's PyPI limit answered: %v", err)
		}
		return nil
	})

	changed := harness.Do[planlimits.ChangeOutput](s, actionAdminPlanLimitsChg, map[string]any{
		"plan_name": planName, "pypi_max_file_size": raised,
	})
	if changed.PyPiMaxFileSize != raised {
		e.T.Errorf("plan_limits_change answered pypi_max_file_size %d, want %d", changed.PyPiMaxFileSize, raised)
	}
}

// TestAdmin_BroadcastMessages walks the broadcast message CRUD cycle: list,
// create, get, update and delete, asserting the id round-trips.
//
// Replaces: TestMeta_AdminBroadcast
func TestAdmin_BroadcastMessages(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin), harness.Locks(harness.LockInstanceGlobal))
	s := e.On(harness.SurfaceMeta)

	before := harness.Do[broadcastmessages.ListOutput](s, actionAdminBroadcastList, nil)
	e.T.Logf("%d broadcast message(s) before the create", len(before.Messages))

	created := harness.Do[broadcastmessages.CreateOutput](s, actionAdminBroadcastCreate, map[string]any{
		"message": "E2E broadcast " + e.Name("msg"),
	})
	if created.Message.ID == 0 {
		e.T.Fatalf("broadcast_message_create answered %+v, want a message with an ID", created.Message)
	}
	adminDeferDelete(e, s, "broadcast message", actionAdminBroadcastDelete, map[string]any{"id": created.Message.ID})

	got := harness.Do[broadcastmessages.GetOutput](s, actionAdminBroadcastGet, map[string]any{"id": created.Message.ID})
	if got.Message.ID != created.Message.ID {
		e.T.Errorf("broadcast_message_get answered %d, want %d", got.Message.ID, created.Message.ID)
	}

	updated := harness.Do[broadcastmessages.UpdateOutput](s, actionAdminBroadcastUpdate, map[string]any{
		"id": created.Message.ID, "message": "Updated E2E broadcast",
	})
	if updated.Message.ID != created.Message.ID {
		e.T.Errorf("broadcast_message_update answered %d, want %d", updated.Message.ID, created.Message.ID)
	}

	harness.DoVoid(s, actionAdminBroadcastDelete, map[string]any{"id": created.Message.ID})
}

// TestAdmin_FeatureFlags lists the instance feature flags and their
// definitions, sets a unique flag and deletes it.
//
// Replaces: TestMeta_AdminFeatures
func TestAdmin_FeatureFlags(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin), harness.Locks(harness.LockInstanceGlobal))
	s := e.On(harness.SurfaceMeta)

	flags := harness.Do[features.ListOutput](s, actionAdminFeatureList, nil)
	e.T.Logf("%d feature flag(s)", len(flags.Features))

	defs := harness.Do[features.ListDefinitionsOutput](s, actionAdminFeatureListDefs, nil)
	if len(defs.Definitions) == 0 {
		e.T.Errorf("feature_list_definitions answered no definitions; GitLab ships with a set of them")
	}

	name := adminFeatureName(e)
	set := harness.Do[features.SetOutput](s, actionAdminFeatureSet, map[string]any{"name": name, "value": true})
	if set.Feature.Name != name {
		e.T.Errorf("feature_set answered %q, want %q", set.Feature.Name, name)
	}
	adminDeferDelete(e, s, "feature flag "+name, actionAdminFeatureDelete, map[string]any{"name": name})

	harness.DoVoid(s, actionAdminFeatureDelete, map[string]any{"name": name})
}

// adminFeatureName is a feature-flag name unique to this run, so two admin
// scenarios never set the same flag. GitLab feature flag names carry only
// lowercase letters, digits and underscores, so the run ID is folded to that
// alphabet.
func adminFeatureName(e *harness.Env) string {
	var b strings.Builder
	b.WriteString("e2e_test_feature_")
	for _, r := range strings.ToLower(e.RunID()) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// TestAdmin_SystemHooks registers an instance system hook, edits it, sets and
// removes a URL variable, fires a test event and deletes it.
//
// Replaces: TestMeta_AdminSystemHooks, TestMeta_AdminSystemHookEditURLVariables
func TestAdmin_SystemHooks(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin), harness.Locks(harness.LockInstanceGlobal))
	s := e.On(harness.SurfaceMeta)

	before := harness.Do[systemhooks.ListOutput](s, actionAdminSystemHookList, nil)
	e.T.Logf("%d system hook(s) before the add", len(before.Hooks))

	added := harness.Do[systemhooks.AddOutput](s, actionAdminSystemHookAdd, map[string]any{
		"url": "https://e2e-test.example.com/hook", "name": "e2e-adm-hook",
	})
	if added.Hook.ID == 0 {
		e.T.Fatalf("system_hook_add answered %+v, want a hook with an ID", added.Hook)
	}
	hookID := added.Hook.ID
	adminDeferDelete(e, s, "system hook", actionAdminSystemHookDelete, map[string]any{"id": hookID})

	edited := harness.Do[systemhooks.EditOutput](s, actionAdminSystemHookEdit, map[string]any{
		"id": hookID, "name": "e2e-adm-hook-edited", "push_events": true,
	})
	if edited.Hook.Name != "e2e-adm-hook-edited" || !edited.Hook.PushEvents {
		e.T.Errorf("system_hook_edit answered %+v, want the edited name with push_events on", edited.Hook)
	}

	// URL-variable keys reject digits, so the key uses only letters and an
	// underscore.
	harness.DoVoid(s, actionAdminSystemHookSetVar, map[string]any{"id": hookID, "key": "EXTRA_ENV", "value": "extra-value"})
	withVar := harness.Do[systemhooks.GetOutput](s, actionAdminSystemHookGet, map[string]any{"id": hookID})
	if !systemHookHasVariable(withVar.Hook, "EXTRA_ENV") {
		e.T.Errorf("the hook does not carry the URL variable EXTRA_ENV after the set: %+v", withVar.Hook.URLVariables)
	}
	harness.DoVoid(s, actionAdminSystemHookDeleteVar, map[string]any{"id": hookID, "key": "EXTRA_ENV"})

	// The test event delivers to the example.com placeholder, which never
	// answers on an isolated network; a tool error is the expected outcome
	// there and a success the expected one where the URL is reachable.
	if _, err := harness.Try[systemhooks.TestOutput](s, actionAdminSystemHookTest, map[string]any{"id": hookID}); err != nil {
		e.T.Logf("system_hook_test against the example.com placeholder answered: %v", err)
	}

	harness.DoVoid(s, actionAdminSystemHookDelete, map[string]any{"id": hookID})
}

// systemHookHasVariable reports whether a hook carries a URL variable key.
func systemHookHasVariable(hook systemhooks.HookItem, key string) bool {
	for _, variable := range hook.URLVariables {
		if variable.Key == key {
			return true
		}
	}
	return false
}

// TestAdmin_SidekiqMetrics reads the four Sidekiq metric endpoints.
//
// Replaces: TestMeta_AdminSidekiqMetrics
func TestAdmin_SidekiqMetrics(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))
	s := e.On(harness.SurfaceMeta)

	queues := harness.Do[sidekiq.GetQueueMetricsOutput](s, actionAdminSidekiqQueue, nil)
	e.T.Logf("Sidekiq queues: %d", len(queues.Queues))
	processes := harness.Do[sidekiq.GetProcessMetricsOutput](s, actionAdminSidekiqProcess, nil)
	e.T.Logf("Sidekiq processes: %d", len(processes.Processes))
	jobStats := harness.Do[sidekiq.GetJobStatsOutput](s, actionAdminSidekiqJobStats, nil)
	e.T.Logf("Sidekiq processed jobs: %d", jobStats.Jobs.Processed)
	compound := harness.Do[sidekiq.GetCompoundMetricsOutput](s, actionAdminSidekiqCompound, nil)
	e.T.Logf("Sidekiq compound: %d queues, %d processes", len(compound.Queues), len(compound.Processes))
}

// TestAdmin_OAuthApplications lists the OAuth applications, creates one,
// rotates its secret and deletes it.
//
// Replaces: TestMeta_AdminApplications, TestMeta_AdminApplicationRenewSecret
func TestAdmin_OAuthApplications(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin), harness.Locks(harness.LockInstanceGlobal))
	s := e.On(harness.SurfaceMeta)

	before := harness.Do[applications.ListOutput](s, actionAdminApplicationList, nil)
	e.T.Logf("%d OAuth application(s) before the create", len(before.Applications))

	created := harness.Do[applications.CreateOutput](s, actionAdminApplicationCreate, map[string]any{
		"name": "e2e-adm-" + e.Name("app"), "redirect_uri": "https://e2e-test.example.com/callback", "scopes": "read_user",
	})
	if created.ID == 0 || created.Secret == "" {
		e.T.Fatalf("application_create answered %+v, want an application with an ID and a secret", created.ApplicationItem)
	}
	adminDeferDelete(e, s, "oauth application", actionAdminApplicationDelete, map[string]any{"id": created.ID})

	renewed := harness.Do[applications.RenewSecretOutput](s, actionAdminApplicationRenew, map[string]any{"id": created.ID})
	if renewed.ID != created.ID || renewed.Secret == "" || renewed.Secret == created.Secret {
		e.T.Errorf("application_renew_secret answered %+v, want application %d with a fresh secret", renewed.ApplicationItem, created.ID)
	}

	harness.DoVoid(s, actionAdminApplicationDelete, map[string]any{"id": created.ID})
}

// TestAdmin_CustomAttributes sets, reads and lists a custom attribute on the
// authenticated user, then deletes it.
//
// Replaces: TestMeta_AdminCustomAttributes
func TestAdmin_CustomAttributes(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin), harness.Locks(harness.LockCurrentUserState))
	s := e.On(harness.SurfaceMeta)

	me := harness.Do[users.Output](s, actionUserCurrent, nil)
	if me.ID == 0 {
		e.T.Fatalf("user.current answered %+v, want the authenticated user with an ID", me)
	}
	const key = "e2e_test_attr"
	base := map[string]any{"resource_type": "user", "resource_id": me.ID, "key": key}

	set := harness.Do[customattributes.SetOutput](s, actionAdminCustomAttrSet, withParams(base, map[string]any{"value": "test-value"}))
	if set.Key != key || set.Value != "test-value" {
		e.T.Fatalf("custom_attr_set answered key=%q value=%q, want %q test-value", set.Key, set.Value, key)
	}
	adminDeferDelete(e, s, "custom attribute", actionAdminCustomAttrDelete, base)

	got := harness.Do[customattributes.GetOutput](s, actionAdminCustomAttrGet, base)
	if got.Key != key || got.Value != "test-value" {
		e.T.Errorf("custom_attr_get answered key=%q value=%q, want %q test-value", got.Key, got.Value, key)
	}

	list := harness.Do[customattributes.ListOutput](s, actionAdminCustomAttrList, map[string]any{"resource_type": "user", "resource_id": me.ID})
	if len(list.Attributes) == 0 {
		e.T.Errorf("custom_attr_list answered no attributes after the set")
	}

	harness.DoVoid(s, actionAdminCustomAttrDelete, base)
}

// TestAdmin_DependencyProxyPurge schedules the dependency proxy cache purge
// for a disposable group, which GitLab accepts even on a group whose proxy
// cache was never populated.
//
// Replaces: TestMeta_AdminDependencyProxyPurge
func TestAdmin_DependencyProxyPurge(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))
	s := e.On(harness.SurfaceMeta)

	group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("adm-depproxy"))
	harness.DoVoid(s, actionAdminDependencyProxyDelete, map[string]any{"group_id": group.IDParam()})
	e.T.Logf("scheduled the dependency proxy cache purge for group %d", group.ID)
}
