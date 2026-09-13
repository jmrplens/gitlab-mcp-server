//go:build e2e

// grouppushrules_test.go covers a group's push rules, which are one record
// per group: add it, read it back, edit it and check the edit persisted,
// delete it and check the read is then refused.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The two file size limits the rule is written with, so the edit is told
// apart from the add by what a read answers.
const (
	pushRuleInitialMaxFileSize = int64(25)
	pushRuleEditedMaxFileSize  = int64(75)
)

// TestGroupPushRules_Lifecycle_AddGetEditDelete walks the push rules of a
// group of the surface's own through their whole life, since a group holds
// one rule record and three surfaces cannot share it.
//
// Replaces: TestMeta_GroupPushRules
func TestGroupPushRules_Lifecycle_AddGetEditDelete(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("pushrule"))
		params := map[string]any{"group_id": group.IDParam()}

		added := harness.Do[groups.PushRuleOutput](s, actionGroupPushRuleAdd, withParams(params, map[string]any{
			"commit_message_regex": "^[A-Z].*", "max_file_size": pushRuleInitialMaxFileSize,
		}))
		if added.ID == 0 || added.MaxFileSize != pushRuleInitialMaxFileSize {
			e.T.Fatalf("push_rule_add answered %+v, want a rule with an ID limiting files to %d MB", added, pushRuleInitialMaxFileSize)
		}

		got := harness.Do[groups.PushRuleOutput](s, actionGroupPushRuleGet, params)
		if got.ID != added.ID || got.MaxFileSize != pushRuleInitialMaxFileSize || got.CommitMessageRegex != "^[A-Z].*" {
			e.T.Errorf("push_rule_get answered %+v, want rule %d as it was added", got, added.ID)
		}

		edited := harness.Do[groups.PushRuleOutput](s, actionGroupPushRuleEdit, withParams(params, map[string]any{"max_file_size": pushRuleEditedMaxFileSize}))
		if edited.MaxFileSize != pushRuleEditedMaxFileSize {
			e.T.Errorf("push_rule_edit answered a limit of %d MB, want %d", edited.MaxFileSize, pushRuleEditedMaxFileSize)
		}
		reread := harness.Do[groups.PushRuleOutput](s, actionGroupPushRuleGet, params)
		if reread.MaxFileSize != pushRuleEditedMaxFileSize {
			e.T.Errorf("the rule reads a limit of %d MB after the edit, want %d", reread.MaxFileSize, pushRuleEditedMaxFileSize)
		}

		harness.DoVoid(s, actionGroupPushRuleDelete, params)
		refused := harness.Refused(s, actionGroupPushRuleGet, params, harness.FailureNotFound)
		e.T.Logf("the read of the deleted rule was refused: %s", firstLine(refused))
	})
}
