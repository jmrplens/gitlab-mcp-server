package runnercontrollerscopes

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The canonical catalog IDs the hints name. A scope action is projected under
// the runner domain, so the ID every surface resolves carries that prefix,
// which the bare action names in action_specs.go do not.
const (
	hintActionScopeList           = "runner." + actionScopeList
	hintActionScopeAddInstance    = "runner." + actionScopeAddInstance
	hintActionScopeAddRunner      = "runner." + actionScopeAddRunner
	hintActionScopeRemoveInstance = "runner." + actionScopeRemoveInstance
	hintActionScopeRemoveRunner   = "runner." + actionScopeRemoveRunner
)

// FormatScopesMarkdown renders every scope a runner controller holds: the card
// of one controller's scoping, with each kind of scope as a nested collection
// under a heading of its own.
func FormatScopesMarkdown(out ScopesOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Runner Controller Scopes")
	if len(out.InstanceLevelScopings) == 0 {
		c.Section(fmt.Sprintf("Instance-Level Scopes (%d)", 0)).
			Note("No instance-level scopes configured.")
	} else {
		t := c.Table(fmt.Sprintf("Instance-Level Scopes (%d)", len(out.InstanceLevelScopings)), "Created", "Updated")
		for _, is := range out.InstanceLevelScopings {
			t.Row(toolutil.FormatTime(is.CreatedAt), toolutil.FormatTime(is.UpdatedAt))
		}
	}
	if len(out.RunnerLevelScopings) == 0 {
		c.Section(fmt.Sprintf("Runner-Level Scopes (%d)", 0)).
			Note("No runner-level scopes configured.")
	} else {
		t := c.Table(fmt.Sprintf("Runner-Level Scopes (%d)", len(out.RunnerLevelScopings)), "Runner ID", "Created", "Updated")
		for _, rs := range out.RunnerLevelScopings {
			t.Row(strconv.FormatInt(rs.RunnerID, 10), toolutil.FormatTime(rs.CreatedAt), toolutil.FormatTime(rs.UpdatedAt))
		}
	}
	c.End(
		toolutil.HintAction(hintActionScopeAddInstance, "grant this controller the instance-level scope"),
		toolutil.HintAction(hintActionScopeAddRunner, "scope this controller to one more runner"),
	)
	return b.String()
}

// FormatInstanceScopeMarkdown renders the instance-level scope a controller was
// granted as the card of one object.
func FormatInstanceScopeMarkdown(out InstanceScopeOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Instance-Level Scope")
	c.Time("Created", out.CreatedAt)
	c.Time("Updated", out.UpdatedAt)
	c.End(
		toolutil.HintAction(hintActionScopeList, "see every scope this controller holds"),
		toolutil.HintAction(hintActionScopeRemoveInstance, "revoke the instance-level scope"),
	)
	return b.String()
}

// FormatRunnerScopeMarkdown renders the scope tying a controller to one runner
// as the card of one object.
func FormatRunnerScopeMarkdown(out RunnerScopeOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Runner Scope (Runner #%d)", out.RunnerID))
	c.Int("Runner ID", out.RunnerID)
	c.Time("Created", out.CreatedAt)
	c.Time("Updated", out.UpdatedAt)
	c.End(
		toolutil.HintAction(hintActionScopeList, "see every scope this controller holds"),
		toolutil.HintAction(hintActionScopeRemoveRunner, "remove this runner from the controller's scope"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatScopesMarkdown)
	toolutil.RegisterMarkdown(FormatInstanceScopeMarkdown)
	toolutil.RegisterMarkdown(FormatRunnerScopeMarkdown)
}
