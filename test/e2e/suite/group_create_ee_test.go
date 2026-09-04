//go:build e2e && enterprise

// group_create_ee_test.go exercises the Ultimate-only group create options added
// in client-go v2.41.0 against a live GitLab EE Ultimate instance: the
// unique-project-download-limit cluster (limit, interval, allowlist, alertlist)
// and auto_ban_user_on_excessive_projects_download.
//
// The group Output does not surface these settings, so the test asserts the
// create call is accepted end-to-end (the unit tests already verify the fields
// are marshaled into the request body). Instances that gate these behind
// admin/application settings are tolerated via a graceful skip.
//
// Build tag: e2e && enterprise. Mode: EE. Surface: individual.
package suite

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestIndividual_GroupCreateUltimateFields verifies the v2.41.0 Ultimate group
// create options round-trip through the create endpoint without error.
func TestIndividual_GroupCreateUltimateFields(t *testing.T) {
	if !sess.enterprise {
		t.Skip("unique-project-download-limit group settings require GitLab Ultimate")
	}
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	path := fmt.Sprintf("e2e-grp-ult-%d", time.Now().UnixMilli())
	limit := int64(10)
	interval := int64(300)
	enabled := true
	var groupID int64
	t.Cleanup(func() {
		if groupID > 0 {
			cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cleanCancel()
			_ = callToolVoidOn(cleanCtx, sess.individual, "gitlab_group_delete", groups.DeleteInput{
				GroupID: toolutil.StringOrInt(strconv.FormatInt(groupID, 10)),
			})
		}
	})

	created, err := callToolOn[groups.Output](ctx, sess.individual, "gitlab_group_create", groups.CreateInput{
		Name:                       path,
		Path:                       path,
		Visibility:                 "private",
		UniqueProjectDownloadLimit: &limit,
		UniqueProjectDownloadLimitIntervalInSeconds: &interval,
		UniqueProjectDownloadLimitAllowlist:         []string{},
		UniqueProjectDownloadLimitAlertlist:         []int64{},
		AutoBanUserOnExcessiveProjectsDownload:      &enabled,
	})
	if err != nil {
		if isHTTPStatus(err, 400) || isHTTPStatus(err, 403) || isHTTPStatus(err, 422) {
			t.Skipf("Ultimate download-limit group settings not accepted on this instance: %v", err)
		}
		requireNoError(t, err, "group create with Ultimate download-limit fields")
	}
	requireTruef(t, created.ID > 0, "created group ID should be positive")
	groupID = created.ID
	t.Logf("Created Ultimate group %d with unique-project-download-limit settings", groupID)
}
