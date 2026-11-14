// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// pushrules_test.go covers a project's push rule: add, read, edit, delete,
// and the not-found a read gives once the rule is gone. A project has at
// most one rule, which is why the whole life fits one project per surface.
// The edit is made three ways: one that tightens the rule and asks nothing,
// one that relaxes it and is refused unconfirmed, and that one confirmed.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The file size limits the rule is created with and edited to, in MB.
const (
	pushRuleMaxFileSize       = int64(50)
	pushRuleMaxFileSizeEdited = int64(100)
)

// TestPushRules_Lifecycle_AddsEditsAndDeletes walks the rule of a project
// of the surface's own through its life.
//
// Replaces: TestIndividual_PushRules, TestMeta_PushRules
func TestPushRules_Lifecycle_AddsEditsAndDeletes(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("pushrule"))
		id := project.IDParam()

		added := harness.Do[projects.PushRuleOutput](s, actionPushRuleAdd, map[string]any{
			"project_id": id, "commit_message_regex": "^[A-Z].*", "max_file_size": pushRuleMaxFileSize,
		})
		if added.ID == 0 {
			e.T.Fatalf("push_rule_add answered %+v, want a rule with an ID", added)
		}

		got := harness.Do[projects.PushRuleOutput](s, actionPushRuleGet, map[string]any{"project_id": id})
		if got.ID != added.ID || got.MaxFileSize != pushRuleMaxFileSize {
			e.T.Errorf("push_rule_get answered %+v, want rule %d with max_file_size %d", got, added.ID, pushRuleMaxFileSize)
		}

		// An edit that adds a restriction asks nothing: the guard in
		// internal/tools/projects reads the rule only for an edit that can
		// relax one, and a switch turned on cannot.
		tightened := harness.Do[projects.PushRuleOutput](s, actionPushRuleEdit, map[string]any{"project_id": id, "prevent_secrets": true})
		if !tightened.PreventSecrets || tightened.MaxFileSize != pushRuleMaxFileSize {
			e.T.Errorf("push_rule_edit answered %+v, want prevent_secrets on and max_file_size still %d", tightened, pushRuleMaxFileSize)
		}

		// Raising the file size limit relaxes a rule in force, so the edit is
		// refused unconfirmed on every surface, before anything is written,
		// and the limit stays where it was.
		unconfirmed := harness.ExpectToolError(s, actionPushRuleEdit, map[string]any{
			"project_id": id, "max_file_size": pushRuleMaxFileSizeEdited,
		}, "confirm=true")
		assertMentions(e, "the unconfirmed raising of the file size limit", unconfirmed, "raises max_file_size from 50 MB to 100 MB")
		kept := harness.Do[projects.PushRuleOutput](s, actionPushRuleGet, map[string]any{"project_id": id})
		if kept.MaxFileSize != pushRuleMaxFileSize {
			e.T.Errorf("push_rule_get answered max_file_size %d after the refused edit, want %d kept", kept.MaxFileSize, pushRuleMaxFileSize)
		}

		// The same edit goes through with confirm, which is what a caller
		// sends once the user has approved it.
		edited := harness.Do[projects.PushRuleOutput](s, actionPushRuleEdit, map[string]any{
			"project_id": id, "max_file_size": pushRuleMaxFileSizeEdited, "confirm": true,
		})
		if edited.MaxFileSize != pushRuleMaxFileSizeEdited {
			e.T.Errorf("push_rule_edit answered max_file_size %d, want %d", edited.MaxFileSize, pushRuleMaxFileSizeEdited)
		}

		harness.DoVoid(s, actionPushRuleDelete, map[string]any{"project_id": id})
		refused := harness.Refused(s, actionPushRuleGet, map[string]any{"project_id": id}, harness.FailureNotFound)
		e.T.Logf("the read after the delete is refused: %s", firstLine(refused))
	})
}
