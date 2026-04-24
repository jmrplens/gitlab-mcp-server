// markdown.go provides Markdown formatting functions for project mirror
// MCP tool output.
package projectmirrors

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single project mirror as Markdown.
func FormatOutputMarkdown(m Output) string {
	if m.ID == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Remote Mirror #%d\n\n", m.ID)
	fmt.Fprintf(&b, "- **URL**: `%s`\n", m.URL)
	fmt.Fprintf(&b, "- **Enabled**: %t\n", m.Enabled)
	fmt.Fprintf(&b, "- **Status**: %s\n", m.UpdateStatus)
	if m.AuthMethod != "" {
		fmt.Fprintf(&b, "- **Auth Method**: %s\n", m.AuthMethod)
	}
	fmt.Fprintf(&b, "- **Only Protected Branches**: %t\n", m.OnlyProtectedBranches)
	fmt.Fprintf(&b, "- **Keep Divergent Refs**: %t\n", m.KeepDivergentRefs)
	if m.MirrorBranchRegex != "" {
		fmt.Fprintf(&b, "- **Branch Regex**: `%s`\n", m.MirrorBranchRegex)
	}
	if len(m.HostKeys) > 0 {
		b.WriteString("- **Host Keys**:\n")
		for _, hk := range m.HostKeys {
			fmt.Fprintf(&b, "  - `%s`\n", hk.FingerprintSHA256)
		}
	}
	if m.LastError != "" {
		fmt.Fprintf(&b, "- **Last Error**: %s\n", m.LastError)
	}
	if m.LastSuccessfulUpdateAt != "" {
		fmt.Fprintf(&b, "- **Last Successful Update**: %s\n", m.LastSuccessfulUpdateAt)
	}
	if m.LastUpdateAt != "" {
		fmt.Fprintf(&b, "- **Last Update**: %s\n", m.LastUpdateAt)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_edit_project_mirror` to modify this mirror's settings",
		"Use `gitlab_force_push_mirror_update` to trigger an immediate sync",
		"Use `gitlab_get_project_mirror_public_key` to retrieve the SSH public key",
	)
	return b.String()
}

// FormatListMarkdown renders a paginated list of project mirrors as Markdown.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Mirrors) == 0 {
		return "No remote mirrors found."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Remote Mirrors (%d)\n\n", len(out.Mirrors))
	b.WriteString("| ID | URL | Enabled | Status | Protected Only |\n")
	b.WriteString("| --: | --- | :-----: | ------ | :------------: |\n")
	for _, m := range out.Mirrors {
		fmt.Fprintf(&b, "| %d | `%s` | %t | %s | %t |\n",
			m.ID,
			toolutil.EscapeMdTableCell(m.URL),
			m.Enabled,
			toolutil.EscapeMdTableCell(m.UpdateStatus),
			m.OnlyProtectedBranches,
		)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b, toolutil.HintPreserveLinks)
	return b.String()
}

// FormatPublicKeyMarkdown renders a mirror's SSH public key as Markdown.
func FormatPublicKeyMarkdown(pk PublicKeyOutput) string {
	if pk.PublicKey == "" {
		return "No public key available."
	}
	var b strings.Builder
	b.WriteString("## Mirror SSH Public Key\n\n")
	b.WriteString("```\n")
	b.WriteString(pk.PublicKey)
	b.WriteString("\n```\n")
	toolutil.WriteHints(&b,
		"Use `gitlab_list_project_mirrors` to view all configured mirrors",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)    // Output
	toolutil.RegisterMarkdown(FormatListMarkdown)      // ListOutput
	toolutil.RegisterMarkdown(FormatPublicKeyMarkdown) // PublicKeyOutput
}
