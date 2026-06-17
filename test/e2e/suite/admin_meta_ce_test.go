//go:build e2e && !enterprise

// admin_meta_ce_test.go tests the GitLab admin-level MCP tools via the
// gitlab_admin meta-tool against a live GitLab instance. Covers topics,
// settings, appearance, broadcast messages, feature flags, system hooks,
// Sidekiq metrics, plan limits, metadata, applications, and custom attributes.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/appearance"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/applications"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/appstatistics"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/broadcastmessages"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/customattributes"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/features"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/metadata"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/planlimits"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/settings"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/sidekiq"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/systemhooks"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/topics"
)

// TestMeta_AdminTopics exercises gitlab_admin topic CRUD actions on the
// instance-scoped topics endpoint.
//
// The test acquires the admin and instance-global capabilities (skips when
// the token is not an admin or shared state is locked), then creates a unique
// topic, fetches it, and updates its description. Cleanup deletes the topic
// via a deferred call so the instance stays clean even when subtests fail.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_AdminTopics(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin, CapabilityInstanceGlobal}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		topicName := uniqueName("topic")
		var topicID int64

		t.Run("TopicCreate", func(t *testing.T) {
			out, err := callToolOn[topics.CreateOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "topic_create",
				"params": map[string]any{
					"name":  topicName,
					"title": "E2E " + topicName,
				},
			})
			requireNoError(t, err, "topic_create")
			requireTruef(t, out.Topic.ID > 0, "topic_create: expected ID > 0")
			topicID = out.Topic.ID
			t.Logf("Created topic %d: %s", topicID, topicName)
		})
		defer func() {
			if topicID > 0 {
				_ = callToolVoidOn(ctx, sess.meta, "gitlab_admin", map[string]any{
					"action": "topic_delete",
					"params": map[string]any{"topic_id": topicID},
				})
			}
		}()

		t.Run("TopicGet", func(t *testing.T) {
			requireTruef(t, topicID > 0, "topicID not set")
			out, err := callToolOn[topics.GetOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "topic_get",
				"params": map[string]any{"topic_id": topicID},
			})
			requireNoError(t, err, "topic_get")
			requireTruef(t, out.Topic.ID == topicID, "topic_get: ID mismatch")
			t.Logf("Got topic %d: %s", out.Topic.ID, out.Topic.Name)
		})

		t.Run("TopicUpdate", func(t *testing.T) {
			requireTruef(t, topicID > 0, "topicID not set")
			out, err := callToolOn[topics.UpdateOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "topic_update",
				"params": map[string]any{
					"topic_id":    topicID,
					"description": "Updated by E2E test",
				},
			})
			requireNoError(t, err, "topic_update")
			requireTruef(t, out.Topic.ID == topicID, "topic_update: ID mismatch")
		})
	})
}

// TestMeta_AdminSettingsAppearance exercises settings and appearance
// actions on the gitlab_admin meta-tool.
//
// The test acquires the admin and instance-global capabilities, then runs
// three subtests: update settings (set default_branch_name), get appearance,
// and update appearance title. The settings update is non-destructive — it
// changes only the default branch name to "main" — and runs in isolation
// because instance-global state is touched.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_AdminSettingsAppearance(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin, CapabilityInstanceGlobal}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		t.Run("SettingsUpdate", func(t *testing.T) {
			out, err := callToolOn[settings.UpdateOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "settings_update",
				"params": map[string]any{
					"settings": map[string]any{"default_branch_name": "main"},
				},
			})
			requireNoError(t, err, "settings_update")
			requireTruef(t, len(out.Settings) > 0, "settings_update: expected settings map")
			t.Logf("Settings updated (%d keys)", len(out.Settings))
		})

		t.Run("AppearanceGet", func(t *testing.T) {
			out, err := callToolOn[appearance.GetOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "appearance_get",
				"params": map[string]any{},
			})
			requireNoError(t, err, "appearance_get")
			t.Logf("Appearance: title=%s", out.Appearance.Title)
		})

		t.Run("AppearanceUpdate", func(t *testing.T) {
			out, err := callToolOn[appearance.UpdateOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "appearance_update",
				"params": map[string]any{
					"title": "E2E GitLab",
				},
			})
			requireNoError(t, err, "appearance_update")
			t.Logf("Updated appearance: title=%s", out.Appearance.Title)
		})
	})
}

// TestMeta_AdminBroadcast exercises the broadcast message CRUD cycle on the
// gitlab_admin meta-tool.
//
// The test acquires the admin and instance-global capabilities, lists
// existing broadcast messages, creates a new one with a unique message,
// fetches it by ID, updates its message, and defers deletion so the instance
// stays clean. Each subtest asserts the expected ID round-trips and the
// operation reports no error.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_AdminBroadcast(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin, CapabilityInstanceGlobal}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		var msgID int64

		t.Run("BroadcastList", func(t *testing.T) {
			out, err := callToolOn[broadcastmessages.ListOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "broadcast_message_list",
				"params": map[string]any{},
			})
			requireNoError(t, err, "broadcast_message_list")
			t.Logf("Broadcast messages: %d", len(out.Messages))
		})

		t.Run("BroadcastCreate", func(t *testing.T) {
			out, err := callToolOn[broadcastmessages.CreateOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "broadcast_message_create",
				"params": map[string]any{
					"message": "E2E test broadcast " + uniqueName(""),
				},
			})
			requireNoError(t, err, "broadcast_message_create")
			requireTruef(t, out.Message.ID > 0, "broadcast_message_create: expected ID > 0")
			msgID = out.Message.ID
			t.Logf("Created broadcast %d", msgID)
		})
		defer func() {
			if msgID > 0 {
				_ = callToolVoidOn(ctx, sess.meta, "gitlab_admin", map[string]any{
					"action": "broadcast_message_delete",
					"params": map[string]any{"id": msgID},
				})
			}
		}()

		t.Run("BroadcastGet", func(t *testing.T) {
			requireTruef(t, msgID > 0, "msgID not set")
			out, err := callToolOn[broadcastmessages.GetOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "broadcast_message_get",
				"params": map[string]any{"id": msgID},
			})
			requireNoError(t, err, "broadcast_message_get")
			requireTruef(t, out.Message.ID == msgID, "broadcast_message_get: ID mismatch")
		})

		t.Run("BroadcastUpdate", func(t *testing.T) {
			requireTruef(t, msgID > 0, "msgID not set")
			out, err := callToolOn[broadcastmessages.UpdateOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "broadcast_message_update",
				"params": map[string]any{
					"id":      msgID,
					"message": "Updated E2E broadcast",
				},
			})
			requireNoError(t, err, "broadcast_message_update")
			requireTruef(t, out.Message.ID == msgID, "broadcast_message_update: ID mismatch")
		})
	})
}

// TestMeta_AdminFeatures exercises feature flag actions on the gitlab_admin
// meta-tool.
//
// The test acquires the admin and instance-global capabilities, lists both
// feature flags and feature flag definitions, sets a unique feature flag to
// true, and then deletes it. The feature name is suffixed with the current
// millisecond timestamp so parallel runs cannot collide on the same flag.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_AdminFeatures(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin, CapabilityInstanceGlobal}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		featureName := fmt.Sprintf("e2e_test_feature_%d", time.Now().UnixMilli())

		t.Run("FeatureList", func(t *testing.T) {
			out, err := callToolOn[features.ListOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "feature_list",
				"params": map[string]any{},
			})
			requireNoError(t, err, "feature_list")
			t.Logf("Features: %d", len(out.Features))
		})

		t.Run("FeatureListDefinitions", func(t *testing.T) {
			out, err := callToolOn[features.ListDefinitionsOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "feature_list_definitions",
				"params": map[string]any{},
			})
			requireNoError(t, err, "feature_list_definitions")
			t.Logf("Feature definitions: %d", len(out.Definitions))
		})

		t.Run("FeatureSet", func(t *testing.T) {
			out, err := callToolOn[features.SetOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "feature_set",
				"params": map[string]any{
					"name":  featureName,
					"value": true,
				},
			})
			requireNoError(t, err, "feature_set")
			requireTruef(t, out.Feature.Name == featureName, "feature name mismatch")
			t.Logf("Set feature: %s", out.Feature.Name)
		})

		t.Run("FeatureDelete", func(t *testing.T) {
			err := callToolVoidOn(ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "feature_delete",
				"params": map[string]any{"name": featureName},
			})
			requireNoError(t, err, "feature_delete")
		})
	})
}

// TestMeta_AdminSystemHooks exercises the system hook CRUD cycle on the
// gitlab_admin meta-tool.
//
// The test acquires the admin and instance-global capabilities, lists
// existing system hooks, registers a new HTTP hook against an example.com
// placeholder URL, fetches it, triggers a test event, and defers deletion.
// The test event is allowed to fail silently on isolated networks because
// the example.com URL will not deliver; the deletion still runs.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_AdminSystemHooks(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin, CapabilityInstanceGlobal}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		var hookID int64

		t.Run("SystemHookList", func(t *testing.T) {
			out, err := callToolOn[systemhooks.ListOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "system_hook_list",
				"params": map[string]any{},
			})
			requireNoError(t, err, "system_hook_list")
			t.Logf("System hooks: %d", len(out.Hooks))
		})

		t.Run("SystemHookAdd", func(t *testing.T) {
			out, err := callToolOn[systemhooks.AddOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "system_hook_add",
				"params": map[string]any{
					"url":  "https://e2e-test.example.com/hook",
					"name": "e2e-test-hook",
				},
			})
			requireNoError(t, err, "system_hook_add")
			requireTruef(t, out.Hook.ID > 0, "system_hook_add: expected ID > 0")
			hookID = out.Hook.ID
			t.Logf("Added system hook %d (name=%s)", hookID, out.Hook.Name)
		})
		defer func() {
			if hookID > 0 {
				_ = callToolVoidOn(ctx, sess.meta, "gitlab_admin", map[string]any{
					"action": "system_hook_delete",
					"params": map[string]any{"id": hookID},
				})
			}
		}()

		t.Run("SystemHookGet", func(t *testing.T) {
			requireTruef(t, hookID > 0, "hookID not set")
			out, err := callToolOn[systemhooks.GetOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "system_hook_get",
				"params": map[string]any{"id": hookID},
			})
			requireNoError(t, err, "system_hook_get")
			requireTruef(t, out.Hook.ID == hookID, "system_hook_get: ID mismatch")
		})

		t.Run("SystemHookTest", func(t *testing.T) {
			requireTruef(t, hookID > 0, "hookID not set")
			_, err := callToolOn[systemhooks.TestOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "system_hook_test",
				"params": map[string]any{"id": hookID},
			})
			requireNoError(t, err, "system_hook_test")
		})
	})
}

// TestMeta_AdminSidekiqMetrics exercises read-only Sidekiq metrics on the
// gitlab_admin meta-tool.
//
// The test acquires the admin capability and runs four subtests that fetch
// queue metrics, process metrics, job stats, and compound metrics. No state
// is mutated, so the test can run in parallel with other admin-only tests
// that touch different instance endpoints.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_AdminSidekiqMetrics(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		t.Run("QueueMetrics", func(t *testing.T) {
			out, err := callToolOn[sidekiq.GetQueueMetricsOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "sidekiq_queue_metrics",
				"params": map[string]any{},
			})
			requireNoError(t, err, "sidekiq_queue_metrics")
			t.Logf("Sidekiq queues: %d", len(out.Queues))
		})

		t.Run("ProcessMetrics", func(t *testing.T) {
			_, err := callToolOn[sidekiq.GetProcessMetricsOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "sidekiq_process_metrics",
				"params": map[string]any{},
			})
			requireNoError(t, err, "sidekiq_process_metrics")
		})

		t.Run("JobStats", func(t *testing.T) {
			_, err := callToolOn[sidekiq.GetJobStatsOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "sidekiq_job_stats",
				"params": map[string]any{},
			})
			requireNoError(t, err, "sidekiq_job_stats")
		})

		t.Run("CompoundMetrics", func(t *testing.T) {
			_, err := callToolOn[sidekiq.GetCompoundMetricsOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "sidekiq_compound_metrics",
				"params": map[string]any{},
			})
			requireNoError(t, err, "sidekiq_compound_metrics")
		})
	})
}

// TestMeta_AdminPlanLimitsMetadata exercises plan_limits, metadata, and
// app_statistics read-only admin actions on the gitlab_admin meta-tool.
//
// The test acquires the admin capability and runs three subtests. The plan
// limits call retries on transient network errors because the request
// frequently times out while sidekiq is busy at run start; the metadata
// call asserts that the GitLab version is non-empty so an empty response
// (proxy misroute) is caught early.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_AdminPlanLimitsMetadata(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		t.Run("PlanLimitsGet", func(t *testing.T) {
			const maxRetries = 3
			var lastErr error
			for attempt := range maxRetries {
				_, lastErr = callToolOn[planlimits.GetOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
					"action": "plan_limits_get",
					"params": map[string]any{"plan_name": "default"},
				})
				if lastErr == nil || !isTransientNetworkError(lastErr) {
					break
				}
				t.Logf("plan_limits_get: transient error (attempt %d/%d): %v", attempt+1, maxRetries, lastErr)
				time.Sleep(time.Duration(attempt+1) * time.Second)
			}
			requireNoError(t, lastErr, "plan_limits_get")
		})

		t.Run("MetadataGet", func(t *testing.T) {
			out, err := callToolOn[metadata.GetOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "metadata_get",
				"params": map[string]any{},
			})
			requireNoError(t, err, "metadata_get")
			requireTruef(t, out.Version != "", "metadata_get: expected non-empty version")
			t.Logf("GitLab version: %s revision: %s", out.Version, out.Revision)
		})

		t.Run("AppStatisticsGet", func(t *testing.T) {
			_, err := callToolOn[appstatistics.GetOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "app_statistics_get",
				"params": map[string]any{},
			})
			requireNoError(t, err, "app_statistics_get")
		})
	})
}

// TestMeta_AdminApplications exercises OAuth application CRUD on the
// gitlab_admin meta-tool.
//
// The test acquires the admin and instance-global capabilities, lists
// existing OAuth applications, creates a new application with a unique name
// and a placeholder redirect URI, and defers deletion. The redirect URI is
// intentionally non-deliverable so the application cannot be exercised
// against a real callback endpoint.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_AdminApplications(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin, CapabilityInstanceGlobal}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		var appID int64

		t.Run("ApplicationList", func(t *testing.T) {
			out, err := callToolOn[applications.ListOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "application_list",
				"params": map[string]any{},
			})
			requireNoError(t, err, "application_list")
			t.Logf("Applications: %d", len(out.Applications))
		})

		t.Run("ApplicationCreate", func(t *testing.T) {
			out, err := callToolOn[applications.CreateOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "application_create",
				"params": map[string]any{
					"name":         "e2e-app-" + uniqueName(""),
					"redirect_uri": "https://e2e-test.example.com/callback",
					"scopes":       "api",
				},
			})
			requireNoError(t, err, "application_create")
			requireTruef(t, out.ID > 0, "application_create: expected ID > 0")
			appID = out.ID
			t.Logf("Created application %d", appID)
		})
		defer func() {
			if appID > 0 {
				_ = callToolVoidOn(ctx, sess.meta, "gitlab_admin", map[string]any{
					"action": "application_delete",
					"params": map[string]any{"id": appID},
				})
			}
		}()
	})
}

// TestMeta_AdminCustomAttributes exercises custom attribute CRUD on the
// current user through the gitlab_admin meta-tool.
//
// The test acquires the admin and instance-global capabilities, fetches the
// authenticated user ID through gitlab_user{action=current}, then sets,
// gets, and lists a custom attribute named "e2e_test_attr" on the user.
// Cleanup deletes the attribute via a deferred call so the user is restored
// even when subtests fail.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_AdminCustomAttributes(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin, CapabilityInstanceGlobal}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		// Get current user ID for custom attributes
		usr, usrErr := callToolOn[struct{ ID int64 }](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "current",
			"params": map[string]any{},
		})
		requireNoError(t, usrErr, "get current user")
		attrKey := "e2e_test_attr"

		t.Run("CustomAttrSet", func(t *testing.T) {
			out, err := callToolOn[customattributes.SetOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "custom_attr_set",
				"params": map[string]any{
					"resource_type": "user",
					"resource_id":   usr.ID,
					"key":           attrKey,
					"value":         "test-value",
				},
			})
			requireNoError(t, err, "custom_attr_set")
			t.Logf("Set custom attr: key=%s", out.Key)
		})
		defer func() {
			_ = callToolVoidOn(ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "custom_attr_delete",
				"params": map[string]any{
					"resource_type": "user",
					"resource_id":   usr.ID,
					"key":           attrKey,
				},
			})
		}()

		t.Run("CustomAttrGet", func(t *testing.T) {
			out, err := callToolOn[customattributes.GetOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "custom_attr_get",
				"params": map[string]any{
					"resource_type": "user",
					"resource_id":   usr.ID,
					"key":           attrKey,
				},
			})
			requireNoError(t, err, "custom_attr_get")
			t.Logf("Got custom attr: key=%s value=%s", out.Key, out.Value)
		})

		t.Run("CustomAttrList", func(t *testing.T) {
			out, err := callToolOn[customattributes.ListOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "custom_attr_list",
				"params": map[string]any{
					"resource_type": "user",
					"resource_id":   usr.ID,
				},
			})
			requireNoError(t, err, "custom_attr_list")
			requireTruef(t, len(out.Attributes) > 0, "custom_attr_list: expected at least 1 attribute")
			t.Logf("Custom attributes: %d", len(out.Attributes))
		})
	})
}
