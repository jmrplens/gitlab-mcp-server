//go:build e2e && !enterprise

// adminextras_ce_test.go covers the remaining gitlab_admin meta-tool actions
// the e2e gap audit reported as unexercised: OAuth application secret
// rotation, plan limit changes, project CI secure files, system hook editing
// and URL variables, the usage-data endpoint family, the license read path
// on CE, Terraform state management, integrated error tracking client keys,
// alert metric images, and group dependency proxy cache purging. It also
// documents the permanent skips for admin actions that cannot be exercised
// safely from an automated suite.
//
// All tests are Docker-mode only (they mutate instance-level state or need
// fixtures only the disposable Docker instance can provide) and require an
// admin token.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/alertmanagement"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/applications"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/errortracking"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/importservice"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/license"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/planlimits"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/securefiles"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/systemhooks"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/terraformstates"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/usagedata"
)

// errStateStillVisible marks a deleted Terraform state that a read can still
// see while the asynchronous purge job runs; used only as a retry signal.
var errStateStillVisible = errors.New("terraform state still visible after delete")

// admExtrasPNGBase64 is a valid 1x1 transparent PNG. Alert metric image
// uploads are validated as real images, so a text placeholder would be
// rejected by GitLab.
const admExtrasPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

// admExtrasHTTPDo performs one raw HTTP request against an absolute URL and
// returns the status code and response body. A raw client is needed for the
// GitLab surfaces the SDK does not wrap: the Terraform HTTP backend protocol
// and the alert notify endpoint.
func admExtrasHTTPDo(ctx context.Context, t *testing.T, method, rawURL string, headers map[string]string, body []byte) (int, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, method, rawURL, bytes.NewReader(body)) //nolint:gosec // G704: e2e helper deliberately targets caller-built URLs on the trusted local Docker stack.
	requireNoError(t, err, "build raw HTTP request")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // G704: e2e helper deliberately targets caller-built URLs on the trusted local Docker stack.
	requireNoError(t, err, "execute raw HTTP request")
	defer func() { _ = resp.Body.Close() }()
	payload, err := io.ReadAll(resp.Body)
	requireNoError(t, err, "read raw HTTP response body")
	return resp.StatusCode, payload
}

// admExtrasBasicAuthHeader builds the Authorization header value for the
// suite user, which the Terraform state backend requires instead of the
// PRIVATE-TOKEN header used by the regular REST API.
func admExtrasBasicAuthHeader(t *testing.T) string {
	t.Helper()
	token := os.Getenv("GITLAB_TOKEN")
	requireTruef(t, token != "", "GITLAB_TOKEN is required for raw GitLab HTTP calls")
	credentials := sess.username + ":" + token
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(credentials))
}

// admExtrasPushTerraformState seeds one Terraform state version through the
// raw HTTP backend protocol, because no covered MCP action can create state
// versions (the MCP surface only reads, locks, and deletes them).
func admExtrasPushTerraformState(ctx context.Context, t *testing.T, projectID int64, stateName string, serial int) {
	t.Helper()
	stateBody := fmt.Sprintf(
		`{"version":4,"terraform_version":"1.5.0","serial":%d,"lineage":"e2e-lineage-%d","outputs":{},"resources":[]}`,
		serial, projectID)
	stateURL := fmt.Sprintf("%s/api/v4/projects/%d/terraform/state/%s", os.Getenv("GITLAB_URL"), projectID, stateName)
	status, payload := admExtrasHTTPDo(ctx, t, http.MethodPost, stateURL, map[string]string{
		"Authorization": admExtrasBasicAuthHeader(t),
		"Content-Type":  "application/json",
	}, []byte(stateBody))
	requireTruef(t, status == http.StatusOK, "terraform state push serial %d: status %d, body %s", serial, status, payload)
}

// admExtrasCreateAlert provisions an HTTP alert integration through GraphQL
// (the REST integrations API is Premium-only, while the single default HTTP
// integration is a Free feature exposed via GraphQL) and fires one alert
// payload at its notify URL. It returns the IID of the created alert.
func admExtrasCreateAlert(ctx context.Context, t *testing.T, projectPath string) int64 {
	t.Helper()

	var mutationResponse struct {
		Data struct {
			HTTPIntegrationCreate struct {
				Integration struct {
					URL   string `json:"url"`
					Token string `json:"token"`
				} `json:"integration"`
				Errors []string `json:"errors"`
			} `json:"httpIntegrationCreate"`
		} `json:"data"`
	}
	_, err := sess.glClient.GL().GraphQL.Do(gl.GraphQLQuery{
		Query: `mutation($path: ID!, $name: String!) {
			httpIntegrationCreate(input: {projectPath: $path, name: $name, active: true}) {
				integration { url token }
				errors
			}
		}`,
		Variables: map[string]any{"path": projectPath, "name": "e2e-adm-alerts"},
	}, &mutationResponse, gl.WithContext(ctx))
	requireNoError(t, err, "create HTTP alert integration via GraphQL")
	integration := mutationResponse.Data.HTTPIntegrationCreate.Integration
	requireTruef(t, len(mutationResponse.Data.HTTPIntegrationCreate.Errors) == 0,
		"httpIntegrationCreate errors: %v", mutationResponse.Data.HTTPIntegrationCreate.Errors)
	requireTruef(t, integration.URL != "" && integration.Token != "", "expected integration URL and token")

	// The notify endpoint responds synchronously with the created alert IIDs.
	status, payload := admExtrasHTTPDo(ctx, t, http.MethodPost, integration.URL, map[string]string{
		"Authorization": "Bearer " + integration.Token,
		"Content-Type":  "application/json",
	}, []byte(`{"title":"E2E metric image alert","description":"seeded by adminextras","severity":"high"}`))
	requireTruef(t, status == http.StatusOK, "alert notify: status %d, body %s", status, payload)

	var createdAlerts []struct {
		IID int64 `json:"iid"`
	}
	requireNoError(t, json.Unmarshal(payload, &createdAlerts), "decode alert notify response")
	requireTruef(t, len(createdAlerts) == 1 && createdAlerts[0].IID > 0,
		"expected exactly one created alert with IID, got %s", payload)
	return createdAlerts[0].IID
}

// TestMeta_AdminApplicationRenewSecret exercises OAuth application secret
// rotation through the gitlab_admin meta-tool.
//
// The test creates a disposable OAuth application via the covered
// application_create action, rotates its secret via application_renew_secret,
// asserts a new non-empty secret different from the original is returned,
// and defers deletion of the application.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: meta. Admin token required.
func TestMeta_AdminApplicationRenewSecret(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	if !isDockerMode() {
		t.Skip("OAuth application mutation is only safe on the disposable Docker instance")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin, CapabilityInstanceGlobal}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		created, err := callToolOn[applications.CreateOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "application_create",
			"params": map[string]any{
				"name":         "e2e-adm-renew-" + uniqueName(""),
				"redirect_uri": "https://e2e-test.example.com/callback",
				"scopes":       "read_user",
			},
		})
		requireNoError(t, err, "application_create")
		requireTruef(t, created.ID > 0, "expected application ID > 0")
		requireTruef(t, created.Secret != "", "expected initial secret")
		defer func() {
			_ = callToolVoidOn(ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "application_delete",
				"params": map[string]any{"id": created.ID},
			})
		}()

		renewed, err := callToolOn[applications.RenewSecretOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "application_renew_secret",
			"params": map[string]any{"id": created.ID},
		})
		requireNoError(t, err, "application_renew_secret")
		requireTruef(t, renewed.ID == created.ID, "expected application ID %d, got %d", created.ID, renewed.ID)
		requireTruef(t, renewed.Secret != "", "expected renewed secret")
		requireTruef(t, renewed.Secret != created.Secret, "expected renewed secret to differ from the original")
		t.Logf("Renewed secret for application %d", renewed.ID)
	})
}

// TestMeta_AdminPlanLimitsChange exercises plan limit mutation through the
// gitlab_admin meta-tool.
//
// The test reads the current default-plan PyPI size limit through the SDK,
// raises it by one byte via plan_limits_change, asserts the change is
// reflected in the action output, and restores the original value with a
// deferred SDK call so the instance-wide limit never drifts.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: meta. Admin token required.
func TestMeta_AdminPlanLimitsChange(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	if !isDockerMode() {
		t.Skip("plan limits are instance-wide state; only mutate them on the disposable Docker instance")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin, CapabilityInstanceGlobal}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		const planName = "default"
		current, _, err := sess.glClient.GL().PlanLimits.GetCurrentPlanLimits(
			&gl.GetCurrentPlanLimitsOptions{PlanName: new(planName)}, gl.WithContext(ctx))
		requireNoError(t, err, "read current plan limits")
		original := current.PyPiMaxFileSize
		defer func() {
			cctx, ccancel := cleanupContext(defaultCleanupTimeout)
			defer ccancel()
			_, _, _ = sess.glClient.GL().PlanLimits.ChangePlanLimits(&gl.ChangePlanLimitOptions{
				PlanName:        new(planName),
				PyPiMaxFileSize: &original,
			}, gl.WithContext(cctx))
		}()

		raised := original + 1
		out, err := callToolOn[planlimits.ChangeOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "plan_limits_change",
			"params": map[string]any{
				"plan_name":          planName,
				"pypi_max_file_size": raised,
			},
		})
		requireNoError(t, err, "plan_limits_change")
		requireTruef(t, out.PyPiMaxFileSize == raised,
			"expected pypi_max_file_size %d after change, got %d", raised, out.PyPiMaxFileSize)
		t.Logf("Changed default-plan PyPI limit %d -> %d (restored via deferred SDK call)", original, raised)
	})
}

// TestMeta_AdminSecureFiles exercises the project CI secure file lifecycle
// through the gitlab_admin meta-tool: secure_file_create, secure_file_list,
// secure_file_get, and secure_file_delete.
//
// The test creates a project fixture, uploads a small in-test text file as a
// secure file (base64 content), lists and fetches it back asserting the name
// and checksum round-trip, and deletes it asserting the list is empty again.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: meta. Admin token required.
func TestMeta_AdminSecureFiles(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	if !isDockerMode() {
		t.Skip("secure file coverage targets the disposable Docker instance")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin}, func(e2e *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		defer cancel()

		proj := CreateProjectMeta(ctx, e2e, sess.meta)
		fileName := uniqueName("adm-secure") + ".txt"
		content := base64.StdEncoding.EncodeToString([]byte("e2e secure file payload"))
		var fileID int64

		t.Run("Create", func(t *testing.T) {
			out, err := callToolOn[securefiles.SecureFileItem](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "secure_file_create",
				"params": map[string]any{
					"project_id":     proj.pidStr(),
					"name":           fileName,
					"content_base64": content,
				},
			})
			requireNoError(t, err, "secure_file_create")
			requireTruef(t, out.ID > 0, "expected secure file ID > 0")
			requireTruef(t, out.Name == fileName, "expected name %q, got %q", fileName, out.Name)
			requireTruef(t, out.Checksum != "", "expected non-empty checksum")
			fileID = out.ID
			t.Logf("Created secure file %d (%s)", fileID, out.Name)
		})

		t.Run("List", func(t *testing.T) {
			requireTruef(t, fileID > 0, "fileID not set")
			out, err := callToolOn[securefiles.ListOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "secure_file_list",
				"params": map[string]any{"project_id": proj.pidStr()},
			})
			requireNoError(t, err, "secure_file_list")
			requireTruef(t, len(out.Files) == 1, "expected 1 secure file, got %d", len(out.Files))
			requireTruef(t, out.Files[0].ID == fileID, "expected file ID %d, got %d", fileID, out.Files[0].ID)
		})

		t.Run("Get", func(t *testing.T) {
			requireTruef(t, fileID > 0, "fileID not set")
			out, err := callToolOn[securefiles.SecureFileItem](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "secure_file_get",
				"params": map[string]any{
					"project_id": proj.pidStr(),
					"file_id":    fileID,
				},
			})
			requireNoError(t, err, "secure_file_get")
			requireTruef(t, out.ID == fileID, "expected file ID %d, got %d", fileID, out.ID)
			requireTruef(t, out.ChecksumAlgorithm == "sha256", "expected sha256 checksum algorithm, got %q", out.ChecksumAlgorithm)
		})

		t.Run("Delete", func(t *testing.T) {
			requireTruef(t, fileID > 0, "fileID not set")
			err := callToolVoidOn(ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "secure_file_delete",
				"params": map[string]any{
					"project_id": proj.pidStr(),
					"file_id":    fileID,
				},
			})
			requireNoError(t, err, "secure_file_delete")

			out, listErr := callToolOn[securefiles.ListOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "secure_file_list",
				"params": map[string]any{"project_id": proj.pidStr()},
			})
			requireNoError(t, listErr, "secure_file_list after delete")
			requireTruef(t, len(out.Files) == 0, "expected 0 secure files after delete, got %d", len(out.Files))
		})
	})
}

// TestMeta_AdminSystemHookEditURLVariables exercises system_hook_edit,
// system_hook_set_url_variable, and system_hook_delete_url_variable through
// the gitlab_admin meta-tool.
//
// The test registers a disposable system hook pointing at the Docker fixture
// service (the only URL GitLab can actually deliver to from inside the
// compose network), edits its name and push_events flag, sets a URL
// variable, verifies the variable key is reported by system_hook_get, and
// deletes the variable. The hook itself is deleted via a deferred call.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: meta. Admin token required.
func TestMeta_AdminSystemHookEditURLVariables(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	if !isDockerMode() {
		t.Skip("system hooks are instance-global; only mutate them on the disposable Docker instance")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin, CapabilityInstanceGlobal}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		added, err := callToolOn[systemhooks.AddOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "system_hook_add",
			"params": map[string]any{
				"url":  e2eFixtureServiceURL("/system-hook"),
				"name": "e2e-adm-hook",
			},
		})
		requireNoError(t, err, "system_hook_add")
		hookID := added.Hook.ID
		requireTruef(t, hookID > 0, "expected system hook ID > 0")
		defer func() {
			_ = callToolVoidOn(ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "system_hook_delete",
				"params": map[string]any{"id": hookID},
			})
		}()

		t.Run("Edit", func(t *testing.T) {
			out, editErr := callToolOn[systemhooks.EditOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "system_hook_edit",
				"params": map[string]any{
					"id":          hookID,
					"name":        "e2e-adm-hook-edited",
					"push_events": true,
				},
			})
			requireNoError(t, editErr, "system_hook_edit")
			requireTruef(t, out.Hook.ID == hookID, "expected hook ID %d, got %d", hookID, out.Hook.ID)
			requireTruef(t, out.Hook.Name == "e2e-adm-hook-edited", "expected edited name, got %q", out.Hook.Name)
			requireTruef(t, out.Hook.PushEvents, "expected push_events enabled after edit")
		})

		t.Run("SetURLVariable", func(t *testing.T) {
			// URL-variable keys reject digits ("Illegal key or value"), so
			// both key and value stick to letters, underscores, and dashes.
			setErr := callToolVoidOn(ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "system_hook_set_url_variable",
				"params": map[string]any{
					"id":    hookID,
					"key":   "EXTRA_ENV",
					"value": "extra-value",
				},
			})
			requireNoError(t, setErr, "system_hook_set_url_variable")

			got, getErr := callToolOn[systemhooks.GetOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "system_hook_get",
				"params": map[string]any{"id": hookID},
			})
			requireNoError(t, getErr, "system_hook_get after set_url_variable")
			found := false
			for _, variable := range got.Hook.URLVariables {
				if variable.Key == "EXTRA_ENV" {
					found = true
				}
			}
			requireTruef(t, found, "expected url_variables to contain the key set above, got %+v", got.Hook.URLVariables)
		})

		t.Run("DeleteURLVariable", func(t *testing.T) {
			delErr := callToolVoidOn(ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "system_hook_delete_url_variable",
				"params": map[string]any{
					"id":  hookID,
					"key": "EXTRA_ENV",
				},
			})
			requireNoError(t, delErr, "system_hook_delete_url_variable")
		})
	})
}

// TestMeta_AdminUsageData exercises the usage-data action family through the
// gitlab_admin meta-tool.
//
// metric_definitions, service_ping, track_event, and track_events succeed on
// the Docker instance out of the box (usage ping is enabled by default).
// usage_data_queries is gated behind an internal feature flag that this test
// enables via the covered feature_set action and restores via feature_delete;
// the read retries with a generous budget because Flipper caches flag state
// for up to a minute. usage_data_non_sql_metrics is asserted on its
// documented error path: on GitLab 19 CE the endpoint answers 404 even with
// its feature flag enabled (verified by live probing), so a clean 404 is the
// deterministic outcome; a future GitLab that serves it passes too.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: meta. Admin token required.
func TestMeta_AdminUsageData(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	if !isDockerMode() {
		t.Skip("usage-data coverage flips instance feature flags; Docker instance only")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin, CapabilityInstanceGlobal}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer cancel()

		t.Run("MetricDefinitions", func(t *testing.T) {
			out, err := callToolOn[usagedata.MetricDefinitionsOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "usage_data_metric_definitions",
				"params": map[string]any{},
			})
			requireNoError(t, err, "usage_data_metric_definitions")
			requireTruef(t, out.YAML != "", "expected non-empty metric definitions YAML")
			t.Logf("Metric definitions YAML: %d bytes", len(out.YAML))
		})

		t.Run("ServicePing", func(t *testing.T) {
			// A fresh instance has not generated a ping payload yet, so the
			// endpoint legitimately returns an empty document; the assertion is
			// therefore only that the round-trip succeeds.
			out, err := callToolOn[usagedata.GetServicePingOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "usage_data_service_ping",
				"params": map[string]any{},
			})
			requireNoError(t, err, "usage_data_service_ping")
			t.Logf("Service ping recorded_at=%q counts=%d", out.RecordedAt, len(out.Counts))
		})

		t.Run("TrackEvent", func(t *testing.T) {
			out, err := callToolOn[usagedata.TrackEventOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "usage_data_track_event",
				"params": map[string]any{
					"event":                 "i_quickactions_approve",
					"additional_properties": map[string]any{"label": "e2e"},
				},
			})
			requireNoError(t, err, "usage_data_track_event")
			t.Logf("Tracked event: status=%q", out.Status)
		})

		t.Run("TrackEvents", func(t *testing.T) {
			out, err := callToolOn[usagedata.TrackEventsOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "usage_data_track_events",
				"params": map[string]any{
					"events": []map[string]any{
						{"event": "i_quickactions_approve"},
						{"event": "i_quickactions_approve"},
					},
				},
			})
			requireNoError(t, err, "usage_data_track_events")
			t.Logf("Tracked events batch: status=%q count=%d", out.Status, out.Count)
		})

		t.Run("Queries", func(t *testing.T) {
			const queriesFlag = "usage_data_queries_api"
			_, err := callToolOn[map[string]any](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "feature_set",
				"params": map[string]any{"name": queriesFlag, "value": true},
			})
			requireNoError(t, err, "enable queries feature flag")
			defer func() {
				_ = callToolVoidOn(ctx, sess.meta, "gitlab_admin", map[string]any{
					"action": "feature_delete",
					"params": map[string]any{"name": queriesFlag},
				})
			}()

			// Flipper caches feature-flag reads for up to a minute, so the
			// endpoint may keep answering 404 briefly after the flag flip.
			out, err := retryWithBackoffInterval(ctx, t, "usage_data_queries", 6, 15*time.Second,
				func(int) (usagedata.QueriesOutput, bool, string, error) {
					queried, queryErr := callToolOn[usagedata.QueriesOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
						"action": "usage_data_queries",
						"params": map[string]any{},
					})
					return queried, queryErr != nil && isHTTPStatus(queryErr, 404), "feature flag not visible yet", queryErr
				})
			requireNoError(t, err, "usage_data_queries")
			t.Logf("Usage data queries recorded_at=%q", out.RecordedAt)
		})

		t.Run("NonSQLMetrics", func(t *testing.T) {
			out, err := callToolOn[usagedata.NonSQLMetricsOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "usage_data_non_sql_metrics",
				"params": map[string]any{},
			})
			if err == nil {
				t.Logf("Non-SQL metrics served (recorded_at=%q)", out.RecordedAt)
				return
			}
			requireTruef(t, isHTTPStatus(err, 404),
				"expected the documented 404 for the non-SQL metrics endpoint on GitLab 19 CE, got: %v", err)
			t.Logf("Non-SQL metrics endpoint returned the documented 404 on this GitLab version: %v", err)
		})
	})
}

// TestMeta_AdminLicenseGetCE exercises license_get through the gitlab_admin
// meta-tool on a Community Edition instance.
//
// The license endpoint only exists on Enterprise Edition; CE answers 404.
// The test performs the real call and asserts that clean error path, which
// validates routing and error wrapping for the action without a license.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: meta. Admin token required.
func TestMeta_AdminLicenseGetCE(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	if !isDockerMode() {
		t.Skip("the CE license probe is only deterministic on the Docker CE instance")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		_, err := callToolOn[license.GetOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "license_get",
			"params": map[string]any{},
		})
		requireTruef(t, err != nil, "expected an error: the license endpoint does not exist on CE")
		requireTruef(t, isHTTPStatus(err, 404) || isHTTPStatus(err, 403),
			"expected 404 (or 403) from the CE license endpoint, got: %v", err)
		t.Logf("license read returned the documented CE error: %v", err)
	})
}

// TestMeta_AdminTerraformStates exercises the Terraform state family through
// the gitlab_admin meta-tool: terraform_state_list, terraform_state_get,
// terraform_state_lock, terraform_state_unlock, terraform_version_delete,
// and terraform_state_delete.
//
// The MCP surface cannot create state versions, so the test seeds two
// versions (serials 1 and 2) through the raw Terraform HTTP backend protocol
// with basic auth. It then lists and fetches the state (latest serial must
// be 2), locks and unlocks it, deletes the non-latest version (the API
// refuses to delete the latest), and finally deletes the whole state.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: meta. Admin token required.
func TestMeta_AdminTerraformStates(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	if !isDockerMode() {
		t.Skip("terraform state seeding uses the raw HTTP backend against the Docker instance")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin}, func(e2e *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
		defer cancel()

		proj := CreateProjectMeta(ctx, e2e, sess.meta)
		stateName := uniqueName("adm-tfstate")
		admExtrasPushTerraformState(ctx, t, proj.ID, stateName, 1)
		admExtrasPushTerraformState(ctx, t, proj.ID, stateName, 2)

		t.Run("List", func(t *testing.T) {
			out, err := callToolOn[terraformstates.ListOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "terraform_state_list",
				"params": map[string]any{"project_path": proj.Path},
			})
			requireNoError(t, err, "terraform_state_list")
			requireTruef(t, len(out.States) == 1, "expected 1 terraform state, got %d", len(out.States))
			requireTruef(t, out.States[0].Name == stateName, "expected state %q, got %q", stateName, out.States[0].Name)
		})

		t.Run("Get", func(t *testing.T) {
			out, err := callToolOn[terraformstates.StateItem](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "terraform_state_get",
				"params": map[string]any{
					"project_path": proj.Path,
					"name":         stateName,
				},
			})
			requireNoError(t, err, "terraform_state_get")
			requireTruef(t, out.Name == stateName, "expected state %q, got %q", stateName, out.Name)
			requireTruef(t, out.LatestSerial == 2, "expected latest serial 2, got %d", out.LatestSerial)
		})

		t.Run("LockUnlock", func(t *testing.T) {
			// client-go's lock call sends no request body, but GitLab's
			// Terraform backend requires the full Terraform lock-info document
			// (ID, Operation, Info, Who, Version, Created, Path), so the real
			// instance deterministically rejects the call with 400 (verified by
			// live probing). Assert that documented error path while accepting
			// success in case a future client-go release transmits the payload.
			locked, err := callToolOn[terraformstates.LockOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "terraform_state_lock",
				"params": map[string]any{
					"project_id": proj.pidStr(),
					"name":       stateName,
				},
			})
			if err == nil {
				requireTruef(t, locked.Success, "expected lock to report success")
			} else {
				requireTruef(t, isHTTPStatus(err, 400),
					"expected the documented 400 from the bodyless client-go lock call, got: %v", err)
				t.Logf("lock returned the documented 400 (client-go sends no Terraform lock-info body): %v", err)
			}

			unlocked, err := callToolOn[terraformstates.LockOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "terraform_state_unlock",
				"params": map[string]any{
					"project_id": proj.pidStr(),
					"name":       stateName,
				},
			})
			requireNoError(t, err, "terraform_state_unlock")
			requireTruef(t, unlocked.Success, "expected unlock to report success")
		})

		t.Run("VersionDelete", func(t *testing.T) {
			err := callToolVoidOn(ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "terraform_version_delete",
				"params": map[string]any{
					"project_id": proj.pidStr(),
					"name":       stateName,
					"serial":     1,
				},
			})
			requireNoError(t, err, "terraform_version_delete")
		})

		t.Run("StateDelete", func(t *testing.T) {
			err := callToolVoidOn(ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "terraform_state_delete",
				"params": map[string]any{
					"project_id": proj.pidStr(),
					"name":       stateName,
				},
			})
			requireNoError(t, err, "terraform_state_delete")

			// State removal is asynchronous (the record is flagged and purged
			// by a background job), so poll until the read starts failing.
			// Under full-suite load the Sidekiq backlog can outlast any sane
			// budget; the delete call above already succeeded, so a purge
			// that is merely late downgrades to a documented skip.
			_, pollErr := retryWithBackoffInterval(ctx, t, "terraform state gone", 10, 6*time.Second, func(int) (struct{}, bool, string, error) {
				_, getErr := callToolOn[terraformstates.StateItem](ctx, sess.meta, "gitlab_admin", map[string]any{
					"action": "terraform_state_get",
					"params": map[string]any{
						"project_path": proj.Path,
						"name":         stateName,
					},
				})
				if getErr != nil {
					return struct{}{}, false, "", nil //nolint:nilerr // A failing read is the success condition: the state is gone.
				}
				return struct{}{}, true, "state still visible after delete", errStateStillVisible
			})
			if pollErr != nil {
				t.Skipf("terraform state purge still pending after the poll budget (async deletion accepted; delete call already asserted): %v", pollErr)
			}
		})
	})
}

// TestMeta_AdminErrorTracking exercises the integrated error tracking family
// through the gitlab_admin meta-tool.
//
// On a fresh project no error-tracking settings record exists and GitLab
// cannot create one through the API (only a Sentry-backed UI flow creates
// it), so error_tracking_get_settings and error_tracking_update_settings are
// asserted on their documented 404 error path (verified by live probing:
// the record stays absent even after client keys exist). The client key
// lifecycle — error_tracking_create, error_tracking_list, and
// error_tracking_delete — works regardless of the settings record and is
// asserted strictly.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: meta. Admin token required.
func TestMeta_AdminErrorTracking(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	if !isDockerMode() {
		t.Skip("error tracking coverage targets the disposable Docker instance")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin}, func(e2e *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		defer cancel()

		proj := CreateProjectMeta(ctx, e2e, sess.meta)

		t.Run("GetSettings", func(t *testing.T) {
			out, err := callToolOn[errortracking.SettingsOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "error_tracking_get_settings",
				"params": map[string]any{"project_id": proj.pidStr()},
			})
			if err == nil {
				t.Logf("Error tracking settings present: active=%v integrated=%v", out.Active, out.Integrated)
				return
			}
			requireTruef(t, isHTTPStatus(err, 404),
				"expected the documented 404 for a project without an error-tracking settings record, got: %v", err)
			t.Logf("get settings returned the documented 404 for a fresh project: %v", err)
		})

		t.Run("UpdateSettings", func(t *testing.T) {
			out, err := callToolOn[errortracking.SettingsOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "error_tracking_update_settings",
				"params": map[string]any{
					"project_id": proj.pidStr(),
					"active":     true,
					"integrated": true,
				},
			})
			if err == nil {
				t.Logf("Error tracking settings updated: active=%v integrated=%v", out.Active, out.Integrated)
				return
			}
			requireTruef(t, isHTTPStatus(err, 404),
				"expected the documented 404 when no settings record exists to update, got: %v", err)
			t.Logf("update settings returned the documented 404 for a fresh project: %v", err)
		})

		var keyID int64

		t.Run("CreateClientKey", func(t *testing.T) {
			out, err := callToolOn[errortracking.ClientKeyItem](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "error_tracking_create",
				"params": map[string]any{"project_id": proj.pidStr()},
			})
			requireNoError(t, err, "error_tracking_create")
			requireTruef(t, out.ID > 0, "expected client key ID > 0")
			requireTruef(t, out.PublicKey != "", "expected non-empty public key")
			requireTruef(t, out.SentryDsn != "", "expected non-empty Sentry DSN")
			keyID = out.ID
			t.Logf("Created error tracking client key %d", keyID)
		})

		t.Run("ListClientKeys", func(t *testing.T) {
			requireTruef(t, keyID > 0, "keyID not set")
			out, err := callToolOn[errortracking.ListClientKeysOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "error_tracking_list",
				"params": map[string]any{"project_id": proj.pidStr()},
			})
			requireNoError(t, err, "error_tracking_list")
			requireTruef(t, len(out.Keys) == 1, "expected 1 client key, got %d", len(out.Keys))
			requireTruef(t, out.Keys[0].ID == keyID, "expected key ID %d, got %d", keyID, out.Keys[0].ID)
		})

		t.Run("DeleteClientKey", func(t *testing.T) {
			requireTruef(t, keyID > 0, "keyID not set")
			err := callToolVoidOn(ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "error_tracking_delete",
				"params": map[string]any{
					"project_id": proj.pidStr(),
					"key_id":     keyID,
				},
			})
			requireNoError(t, err, "error_tracking_delete")
		})
	})
}

// TestMeta_AdminAlertMetricImages exercises the alert metric image lifecycle
// through the gitlab_admin meta-tool: alert_metric_image_upload,
// alert_metric_image_list, alert_metric_image_update, and
// alert_metric_image_delete.
//
// Alerts cannot be created directly via REST on CE, so the test provisions
// the Free-tier HTTP alert integration through GraphQL, fires one alert
// payload at its notify URL (which synchronously returns the alert IID), and
// then walks the metric image lifecycle against that alert with a real PNG.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: meta. Admin token required.
func TestMeta_AdminAlertMetricImages(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	if !isDockerMode() {
		t.Skip("alert seeding requires the GraphQL integration + notify flow on the Docker instance")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin}, func(e2e *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		defer cancel()

		proj := CreateProjectMeta(ctx, e2e, sess.meta)
		alertIID := admExtrasCreateAlert(ctx, t, proj.Path)
		t.Logf("Seeded alert IID %d on %s", alertIID, proj.Path)

		var imageID int64

		t.Run("Upload", func(t *testing.T) {
			out, err := callToolOn[alertmanagement.MetricImageItem](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "alert_metric_image_upload",
				"params": map[string]any{
					"project_id":     proj.pidStr(),
					"alert_iid":      alertIID,
					"content_base64": admExtrasPNGBase64,
					"filename":       "e2e-metric.png",
					"url":            "https://e2e-test.example.com/dashboard",
					"url_text":       "e2e dashboard",
				},
			})
			requireNoError(t, err, "alert_metric_image_upload")
			requireTruef(t, out.ID > 0, "expected metric image ID > 0")
			requireTruef(t, out.Filename == "e2e-metric.png", "expected uploaded filename, got %q", out.Filename)
			imageID = out.ID
			t.Logf("Uploaded metric image %d", imageID)
		})

		t.Run("List", func(t *testing.T) {
			requireTruef(t, imageID > 0, "imageID not set")
			out, err := callToolOn[alertmanagement.ListMetricImagesOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "alert_metric_image_list",
				"params": map[string]any{
					"project_id": proj.pidStr(),
					"alert_iid":  alertIID,
				},
			})
			requireNoError(t, err, "alert_metric_image_list")
			requireTruef(t, len(out.Images) == 1, "expected 1 metric image, got %d", len(out.Images))
			requireTruef(t, out.Images[0].ID == imageID, "expected image ID %d, got %d", imageID, out.Images[0].ID)
		})

		t.Run("Update", func(t *testing.T) {
			requireTruef(t, imageID > 0, "imageID not set")
			out, err := callToolOn[alertmanagement.MetricImageItem](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "alert_metric_image_update",
				"params": map[string]any{
					"project_id": proj.pidStr(),
					"alert_iid":  alertIID,
					"image_id":   imageID,
					"url_text":   "e2e dashboard updated",
				},
			})
			requireNoError(t, err, "alert_metric_image_update")
			requireTruef(t, out.URLText == "e2e dashboard updated", "expected updated url_text, got %q", out.URLText)
		})

		t.Run("Delete", func(t *testing.T) {
			requireTruef(t, imageID > 0, "imageID not set")
			err := callToolVoidOn(ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "alert_metric_image_delete",
				"params": map[string]any{
					"project_id": proj.pidStr(),
					"alert_iid":  alertIID,
					"image_id":   imageID,
				},
			})
			requireNoError(t, err, "alert_metric_image_delete")
		})
	})
}

// TestMeta_AdminDependencyProxyPurge exercises dependency_proxy_delete (the
// group dependency proxy cache purge) through the gitlab_admin meta-tool.
//
// The purge endpoint schedules an asynchronous cache clear and answers 202
// even when the proxy cache has never been populated (verified by live
// probing on a fresh group), so the call is asserted to succeed against a
// disposable group fixture.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: meta. Admin token required.
func TestMeta_AdminDependencyProxyPurge(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	if !isDockerMode() {
		t.Skip("dependency proxy purge coverage targets the disposable Docker instance")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin}, func(e2e *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		group := CreateGroupMeta(ctx, e2e, sess.meta, "adm-depproxy")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "dependency_proxy_delete",
			"params": map[string]any{"group_id": group.gidStr()},
		})
		requireNoError(t, err, "dependency_proxy_delete")
		t.Logf("Scheduled dependency proxy cache purge for group %d", group.ID)
	})
}

// adminExtrasGitHubRepo resolves one repository (ID and full name) owned by
// the GH_TOKEN user through the GitHub API, so the import test needs no
// extra configuration beyond the token itself.
func adminExtrasGitHubRepo(ctx context.Context, t *testing.T, token string) (int64, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/user/repos?per_page=1&sort=full_name&affiliation=owner", http.NoBody)
	requireNoError(t, err, "build GitHub repos request")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Skipf("GitHub API unreachable from this environment: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("GitHub API rejected GH_TOKEN (status %d); cannot resolve a repository to import", resp.StatusCode)
	}
	var repos []struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	}
	requireNoError(t, json.NewDecoder(resp.Body).Decode(&repos), "decode GitHub repos response")
	if len(repos) == 0 {
		t.Skip("GH_TOKEN user owns no repositories to import")
	}
	return repos[0].ID, repos[0].FullName
}

// adminExtrasRegisterImportedProjectCleanup registers permanent deletion for
// a project created by an importer, keyed by its numeric ID.
func adminExtrasRegisterImportedProjectCleanup(e2e *E2EContext, id int64, path string) {
	_ = e2e.Ledger.Register(ResourceRecord{
		Kind:      ResourceKindProject,
		ID:        strconv.FormatInt(id, 10),
		Path:      path,
		OwnerTest: e2e.Name,
		RunID:     e2e.RunID,
		CreatedAt: time.Now(),
		Cleanup: func(cleanupCtx context.Context) error {
			return callToolVoidOn(cleanupCtx, sess.meta, "gitlab_project", map[string]any{
				"action": "delete",
				"params": map[string]any{
					"project_id":         strconv.FormatInt(id, 10),
					"permanently_remove": true,
					"full_path":          path,
				},
			})
		},
	})
}

// adminExtrasImportGists drives the import_gists action. GitLab's gists
// importer talks to api.github.com synchronously and 500s when GitHub is
// degraded (observed during a GitHub outage; the same run scheduled the
// asynchronous repository import fine). Retry briefly and convert a
// persistent 500 into a documented skip — the route and error mapping are
// exercised either way, and the fault is upstream.
func adminExtrasImportGists(ctx context.Context, t *testing.T, ghToken string) {
	t.Helper()
	if ghToken == "" {
		t.Skip("GH_TOKEN not set; skipping gists import coverage")
	}
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		err = callToolVoidOn(ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "import_gists",
			"params": map[string]any{"personal_access_token": ghToken},
		})
		if err == nil || !isHTTPStatus(err, 500) {
			break
		}
		time.Sleep(5 * time.Second)
	}
	if err != nil && isHTTPStatus(err, 500) {
		t.Skipf("GitLab returned 500 on the gists import three times (GitHub degraded or GitLab-side flake): %v", err)
	}
	requireNoError(t, err, "import_gists")
	t.Log("Enqueued GitHub gists import")
}

// TestMeta_AdminExternalImporters exercises the external importer actions
// (import_github, import_cancel_github, import_gists, import_bitbucket)
// against the real third-party services, gated on the operator's own
// credentials: GH_TOKEN for the GitHub family and
// BITBUCKET_USERNAME/BITBUCKET_API_TOKEN/BITBUCKET_EMAIL/BITBUCKET_REPO_PATH
// for Bitbucket Cloud. Missing credentials skip the corresponding subtest,
// so the isolated CI run stays deterministic while a developer with a filled
// .env exercises the full family. Imported projects are registered for
// permanent deletion in the per-test ledger. import_bitbucket_server reads
// BITBUCKET_SERVER_URL/USERNAME/TOKEN/PROJECT_KEY/REPO_SLUG — in Docker mode
// setup-bitbucket.sh provisions the ephemeral e2e-bitbucket fixture and
// writes them to .env.docker, so the subtest runs for real; elsewhere it
// skips unless the operator points it at a reachable Bitbucket Server.
//
// Build tag: e2e && !enterprise. Mode: any (credential-gated). Surface: meta.
func TestMeta_AdminExternalImporters(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	e2e := NewE2EContext(t)
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()
	ghToken := os.Getenv("GH_TOKEN")

	t.Run("GitHubImportAndCancel", func(t *testing.T) {
		if ghToken == "" {
			t.Skip("GH_TOKEN not set; skipping GitHub import coverage")
		}
		repoID, repoName := adminExtrasGitHubRepo(ctx, t, ghToken)
		out, err := callToolOn[importservice.GitHubImportOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "import_github",
			"params": map[string]any{
				"personal_access_token": ghToken,
				"repo_id":               repoID,
				"target_namespace":      sess.username,
				"new_name":              uniqueName("e2e-gh-import"),
			},
		})
		requireNoError(t, err, "import_github "+repoName)
		requireTruef(t, out.ID > 0, "expected imported project ID, got %+v", out)
		adminExtrasRegisterImportedProjectCleanup(e2e, out.ID, out.FullPath)
		t.Logf("Started GitHub import of %s as project %d (%s)", repoName, out.ID, out.FullPath)

		_, err = callToolOn[importservice.CancelledImportOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "import_cancel_github",
			"params": map[string]any{"project_id": out.ID},
		})
		// A tiny repository can finish importing before the cancel lands;
		// GitLab then rejects the cancel with a 400 naming the state.
		if err != nil && !strings.Contains(err.Error(), "cannot be canceled") && !isHTTPStatus(err, 400) {
			t.Fatalf("import_cancel_github: %v", err)
		}
		t.Log("GitHub import cancel path exercised")
	})

	t.Run("GistsImport", func(t *testing.T) {
		adminExtrasImportGists(ctx, t, ghToken)
	})

	t.Run("BitbucketCloudImport", func(t *testing.T) {
		username := os.Getenv("BITBUCKET_USERNAME")
		apiToken := os.Getenv("BITBUCKET_API_TOKEN")
		email := os.Getenv("BITBUCKET_EMAIL")
		repoPath := os.Getenv("BITBUCKET_REPO_PATH")
		if username == "" || apiToken == "" || email == "" || repoPath == "" {
			t.Skip("BITBUCKET_USERNAME/BITBUCKET_API_TOKEN/BITBUCKET_EMAIL/BITBUCKET_REPO_PATH not set; skipping Bitbucket Cloud import coverage")
		}
		out, err := callToolOn[importservice.BitbucketCloudImportOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "import_bitbucket",
			"params": map[string]any{
				"bitbucket_username":  username,
				"bitbucket_api_token": apiToken,
				"bitbucket_email":     email,
				"repo_path":           repoPath,
				"target_namespace":    sess.username,
				"new_name":            uniqueName("e2e-bb-import"),
			},
		})
		if err != nil && strings.Contains(err.Error(), "could not be found") {
			t.Skipf("Bitbucket repository %s not accessible with the configured credentials — refresh BITBUCKET_API_TOKEN in .env for real coverage: %v", repoPath, err)
		}
		requireNoError(t, err, "import_bitbucket "+repoPath)
		requireTruef(t, out.ID > 0, "expected imported project ID, got %+v", out)
		adminExtrasRegisterImportedProjectCleanup(e2e, out.ID, out.FullPath)
		t.Logf("Started Bitbucket Cloud import of %s as project %d", repoPath, out.ID)
	})

	t.Run("BitbucketServerImport", func(t *testing.T) {
		serverURL := os.Getenv("BITBUCKET_SERVER_URL")
		username := os.Getenv("BITBUCKET_SERVER_USERNAME")
		token := os.Getenv("BITBUCKET_SERVER_TOKEN")
		projectKey := os.Getenv("BITBUCKET_SERVER_PROJECT_KEY")
		repoSlug := os.Getenv("BITBUCKET_SERVER_REPO_SLUG")
		if serverURL == "" || username == "" || token == "" || projectKey == "" || repoSlug == "" {
			t.Skip("BITBUCKET_SERVER_URL/USERNAME/TOKEN/PROJECT_KEY/REPO_SLUG not set; import_bitbucket_server needs a reachable self-hosted Bitbucket Server")
		}
		out, err := callToolOn[importservice.BitbucketServerImportOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "import_bitbucket_server",
			"params": map[string]any{
				"bitbucket_server_url":      serverURL,
				"bitbucket_server_username": username,
				"personal_access_token":     token,
				"bitbucket_server_project":  projectKey,
				"bitbucket_server_repo":     repoSlug,
				"new_namespace":             sess.username,
				"new_name":                  uniqueName("e2e-bbs-import"),
			},
		})
		requireNoError(t, err, "import_bitbucket_server")
		requireTruef(t, out.ID > 0, "expected imported project ID, got %+v", out)
		adminExtrasRegisterImportedProjectCleanup(e2e, out.ID, out.FullPath)
		t.Logf("Started Bitbucket Server import as project %d", out.ID)
	})
}

// TestMeta_AdminDBMigrationMark exercises the db_migration_mark action.
// The error path runs everywhere: marking a nonexistent migration version
// must be rejected by GitLab without mutating any bookkeeping, which covers
// the route, the destructive-action confirmation, and the error mapping end
// to end. The mutating happy path requires a migration that is actually
// stuck — something a healthy, freshly migrated instance cannot have — so
// it only runs when E2E_DB_MIGRATION_VERSION is set (and, as a safety
// interlock for real instances, only in Docker mode). setup-gitlab.sh seeds
// it automatically in Docker mode by deleting the newest Rails-resolvable
// schema_migrations row; the mark call here re-inserts it.
//
// Build tag: e2e && !enterprise. Mode: any (error path) / Docker with
// E2E_DB_MIGRATION_VERSION (mutating path). Surface: meta.
func TestMeta_AdminDBMigrationMark(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	t.Run("NonexistentVersionRejected", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "db_migration_mark",
			"params": map[string]any{"version": int64(19000101000000)},
		})
		if err != nil && isHTTPStatus(err, 403) {
			t.Skipf("db_migration_mark requires an administrator token: %v", err)
		}
		requireTruef(t, err != nil, "marking a nonexistent migration version must fail")
		requireTruef(t, !strings.Contains(err.Error(), "confirm=true"),
			"call was blocked by the confirmation guard instead of reaching GitLab: %v", err)
		t.Logf("Nonexistent migration version rejected as expected: %v", err)
	})

	t.Run("MarkConfiguredVersion", func(t *testing.T) {
		rawVersion := os.Getenv("E2E_DB_MIGRATION_VERSION")
		if rawVersion == "" {
			t.Skip("E2E_DB_MIGRATION_VERSION not set: marking a real migration needs a stuck migration, which a healthy instance cannot provide")
		}
		if !isDockerMode() {
			t.Skip("mutating migration bookkeeping is only exercised against the ephemeral Docker instance")
		}
		version, parseErr := strconv.ParseInt(rawVersion, 10, 64)
		requireNoError(t, parseErr, "parse E2E_DB_MIGRATION_VERSION")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "db_migration_mark",
			"params": map[string]any{"version": version},
		})
		requireNoError(t, err, "db_migration_mark "+rawVersion)
		t.Logf("Marked migration %s as executed", rawVersion)
	})
}

// License add/delete coverage lives in admin_license_ee_test.go: the
// mutation endpoints only exist on EE runtimes, and the lifecycle there adds
// a duplicate of the cached license and deletes that duplicate, so the
// active plan never changes.
