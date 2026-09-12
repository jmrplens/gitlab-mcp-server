package compliancepolicy

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionUpdate   = "compliance_policy.update"
	actionGroupGet = "group.get"
)

// cspNotSet is what the card shows when no namespace hosts the centralized
// compliance security policy project. It is an answer rather than a missing
// value, so it is written instead of the ID and not beside it.
const cspNotSet = "not set"

// FormatOutputMarkdown renders the instance compliance policy settings as the
// card of one object: the namespace that hosts the centralized compliance
// security policy (CSP) project, or the statement that none does.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Compliance Policy Settings")
	if out.CSPNamespaceID != nil {
		c.Int("CSP Namespace ID", *out.CSPNamespaceID)
	} else {
		c.Field("CSP Namespace ID", cspNotSet)
	}
	c.End(
		toolutil.HintAction(actionUpdate, "bind the compliance security policy project to a top-level group"),
		toolutil.HintAction(actionGroupGet, "read the group this namespace ID names"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown) // Output
}
