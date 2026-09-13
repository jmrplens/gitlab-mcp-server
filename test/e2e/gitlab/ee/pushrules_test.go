//go:build e2e

// pushrules_test.go covers a project's push rule: add, read, edit, delete,
// and the not-found a read gives once the rule is gone. A project has at
// most one rule, which is why the whole life fits one project per surface.

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

		edited := harness.Do[projects.PushRuleOutput](s, actionPushRuleEdit, map[string]any{"project_id": id, "max_file_size": pushRuleMaxFileSizeEdited})
		if edited.MaxFileSize != pushRuleMaxFileSizeEdited {
			e.T.Errorf("push_rule_edit answered max_file_size %d, want %d", edited.MaxFileSize, pushRuleMaxFileSizeEdited)
		}

		harness.DoVoid(s, actionPushRuleDelete, map[string]any{"project_id": id})
		refused := harness.Refused(s, actionPushRuleGet, map[string]any{"project_id": id}, harness.FailureNotFound)
		e.T.Logf("the read after the delete is refused: %s", firstLine(refused))
	})
}
