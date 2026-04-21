package groupscim

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single SCIM identity as Markdown.
func FormatOutputMarkdown(o Output) string {
	if o.UserID == 0 && o.ExternalUID == "" {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## SCIM Identity\n\n")
	fmt.Fprintf(&b, "- **External UID**: `%s`\n", o.ExternalUID)
	fmt.Fprintf(&b, "- **User ID**: %d\n", o.UserID)
	fmt.Fprintf(&b, "- **Active**: %t\n", o.Active)
	toolutil.WriteHints(&b,
		"Use `gitlab_update_group_scim_identity` to modify the external UID",
		"Use `gitlab_delete_group_scim_identity` to remove this identity",
	)
	return b.String()
}

// FormatListMarkdown renders a list of SCIM identities as Markdown.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Identities) == 0 {
		return "No SCIM identities found."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## SCIM Identities (%d)\n\n", len(out.Identities))
	b.WriteString("| External UID | User ID | Active |\n")
	b.WriteString("| ------------ | ------: | ------ |\n")
	for _, id := range out.Identities {
		fmt.Fprintf(&b, "| `%s` | %d | %t |\n", id.ExternalUID, id.UserID, id.Active)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_get_group_scim_identity` to view full identity details",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown) // Output
	toolutil.RegisterMarkdown(FormatListMarkdown)   // ListOutput
}
