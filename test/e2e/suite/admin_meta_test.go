//go:build e2e

// admin_meta_test.go tests the GitLab admin-level MCP tools via the
// gitlab_admin meta-tool against a live GitLab instance. Covers topics,
// settings, appearance, broadcast messages, feature flags, system hooks,
// Sidekiq metrics, plan limits, metadata, applications, and custom attributes.
package suite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/appearance"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/applications"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/appstatistics"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/broadcastmessages"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/customattributes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/features"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/metadata"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/planlimits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/settings"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/sidekiq"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/systemhooks"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/topics"
)

// TestMeta_AdminTopics exercises gitlab_admin topic CRUD actions.
func TestMeta_AdminTopics(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

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
		requireTrue(t, out.Topic.ID > 0, "topic_create: expected ID > 0")
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
		requireTrue(t, topicID > 0, "topicID not set")
		out, err := callToolOn[topics.GetOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "topic_get",
			"params": map[string]any{"topic_id": topicID},
		})
		requireNoError(t, err, "topic_get")
		requireTrue(t, out.Topic.ID == topicID, "topic_get: ID mismatch")
		t.Logf("Got topic %d: %s", out.Topic.ID, out.Topic.Name)
	})

	t.Run("TopicUpdate", func(t *testing.T) {
		requireTrue(t, topicID > 0, "topicID not set")
		out, err := callToolOn[topics.UpdateOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "topic_update",
			"params": map[string]any{
				"topic_id":    topicID,
				"description": "Updated by E2E test",
			},
		})
		requireNoError(t, err, "topic_update")
		requireTrue(t, out.Topic.ID == topicID, "topic_update: ID mismatch")
	})
}

// TestMeta_AdminSettingsAppearance exercises settings and appearance actions.
func TestMeta_AdminSettingsAppearance(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

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
		requireTrue(t, len(out.Settings) > 0, "settings_update: expected settings map")
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
}

// TestMeta_AdminBroadcast exercises broadcast message CRUD.
func TestMeta_AdminBroadcast(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

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
		requireTrue(t, out.Message.ID > 0, "broadcast_message_create: expected ID > 0")
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
		requireTrue(t, msgID > 0, "msgID not set")
		out, err := callToolOn[broadcastmessages.GetOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "broadcast_message_get",
			"params": map[string]any{"id": msgID},
		})
		requireNoError(t, err, "broadcast_message_get")
		requireTrue(t, out.Message.ID == msgID, "broadcast_message_get: ID mismatch")
	})

	t.Run("BroadcastUpdate", func(t *testing.T) {
		requireTrue(t, msgID > 0, "msgID not set")
		out, err := callToolOn[broadcastmessages.UpdateOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "broadcast_message_update",
			"params": map[string]any{
				"id":      msgID,
				"message": "Updated E2E broadcast",
			},
		})
		requireNoError(t, err, "broadcast_message_update")
		requireTrue(t, out.Message.ID == msgID, "broadcast_message_update: ID mismatch")
	})
}

// TestMeta_AdminFeatures exercises feature flag actions.
func TestMeta_AdminFeatures(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

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
		requireTrue(t, out.Feature.Name == featureName, "feature name mismatch")
		t.Logf("Set feature: %s", out.Feature.Name)
	})

	t.Run("FeatureDelete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "feature_delete",
			"params": map[string]any{"name": featureName},
		})
		requireNoError(t, err, "feature_delete")
	})
}

// TestMeta_AdminSystemHooks exercises system hook CRUD.
func TestMeta_AdminSystemHooks(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

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
		requireTrue(t, out.Hook.ID > 0, "system_hook_add: expected ID > 0")
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
		requireTrue(t, hookID > 0, "hookID not set")
		out, err := callToolOn[systemhooks.GetOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "system_hook_get",
			"params": map[string]any{"id": hookID},
		})
		requireNoError(t, err, "system_hook_get")
		requireTrue(t, out.Hook.ID == hookID, "system_hook_get: ID mismatch")
	})

	t.Run("SystemHookTest", func(t *testing.T) {
		requireTrue(t, hookID > 0, "hookID not set")
		_, err := callToolOn[systemhooks.TestOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "system_hook_test",
			"params": map[string]any{"id": hookID},
		})
		requireNoError(t, err, "system_hook_test")
	})
}

// TestMeta_AdminSidekiqMetrics exercises Sidekiq metrics (read-only).
func TestMeta_AdminSidekiqMetrics(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

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
}

// TestMeta_AdminPlanLimitsMetadata exercises plan_limits, metadata, and app_statistics.
func TestMeta_AdminPlanLimitsMetadata(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	t.Run("PlanLimitsGet", func(t *testing.T) {
		_, err := callToolOn[planlimits.GetOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "plan_limits_get",
			"params": map[string]any{"plan_name": "default"},
		})
		requireNoError(t, err, "plan_limits_get")
	})

	t.Run("MetadataGet", func(t *testing.T) {
		out, err := callToolOn[metadata.GetOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "metadata_get",
			"params": map[string]any{},
		})
		requireNoError(t, err, "metadata_get")
		requireTrue(t, out.Version != "", "metadata_get: expected non-empty version")
		t.Logf("GitLab version: %s revision: %s", out.Version, out.Revision)
	})

	t.Run("AppStatisticsGet", func(t *testing.T) {
		_, err := callToolOn[appstatistics.GetOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "app_statistics_get",
			"params": map[string]any{},
		})
		requireNoError(t, err, "app_statistics_get")
	})
}

// TestMeta_AdminApplications exercises OAuth application CRUD.
func TestMeta_AdminApplications(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

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
		requireTrue(t, out.ID > 0, "application_create: expected ID > 0")
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
}

// TestMeta_AdminCustomAttributes exercises custom attribute CRUD on the current user.
func TestMeta_AdminCustomAttributes(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

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
		requireTrue(t, len(out.Attributes) > 0, "custom_attr_list: expected at least 1 attribute")
		t.Logf("Custom attributes: %d", len(out.Attributes))
	})
}
