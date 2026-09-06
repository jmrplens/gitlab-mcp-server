package projectmirrors

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatOutputMarkdown renders a single project mirror as Markdown.
func FormatOutputMarkdown(m Output) string {
	if m.ID == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Remote Mirror #%d\n\n", m.ID)
	// The mirror URL is whatever the maintainer configuring the mirror typed.
	fmt.Fprintf(&b, "- **URL**: `%s`\n", toolutil.EscapeMdTableCell(m.URL))
	fmt.Fprintf(&b, "- **Enabled**: %t\n", m.Enabled)
	//gitlab:allow-unescaped m.UpdateStatus: a mirror update status GitLab picks from a fixed set (none, scheduled, started, finished, failed).
	fmt.Fprintf(&b, "- **Status**: %s\n", m.UpdateStatus)
	if m.AuthMethod != "" {
		//gitlab:allow-unescaped m.AuthMethod: a mirror authentication method GitLab picks from a fixed set (password, ssh_public_key).
		fmt.Fprintf(&b, "- **Auth Method**: %s\n", m.AuthMethod)
	}
	fmt.Fprintf(&b, "- **Only Protected Branches**: %t\n", m.OnlyProtectedBranches)
	fmt.Fprintf(&b, "- **Keep Divergent Refs**: %t\n", m.KeepDivergentRefs)
	if m.MirrorBranchRegex != "" {
		// A regex a maintainer types, where '|' is ordinary alternation.
		fmt.Fprintf(&b, "- **Branch Regex**: `%s`\n", toolutil.EscapeMdTableCell(m.MirrorBranchRegex))
	}
	if len(m.HostKeys) > 0 {
		b.WriteString("- **Host Keys**:\n")
		for _, hk := range m.HostKeys {
			//gitlab:allow-unescaped hk.FingerprintSHA256: a host key fingerprint GitLab derives from the key, base64 after a "SHA256:" prefix.
			fmt.Fprintf(&b, "  - `%s`\n", hk.FingerprintSHA256)
		}
	}
	if m.LastError != "" {
		// GitLab quotes the remote's own output back in this field, so a
		// hostile remote writes it.
		fmt.Fprintf(&b, "- **Last Error**: %s\n", toolutil.EscapeMdTableCell(m.LastError))
	}
	if m.LastSuccessfulUpdateAt != "" {
		//gitlab:allow-unescaped m.LastSuccessfulUpdateAt: a timestamp this package formatted itself, with time.Time.Format.
		fmt.Fprintf(&b, "- **Last Successful Update**: %s\n", m.LastSuccessfulUpdateAt)
	}
	if m.LastUpdateAt != "" {
		//gitlab:allow-unescaped m.LastUpdateAt: a timestamp this package formatted itself, with time.Time.Format.
		fmt.Fprintf(&b, "- **Last Update**: %s\n", m.LastUpdateAt)
	}
	toolutil.WriteHints(
		&b,
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
		fmt.Fprintf(
			&b, "| %d | `%s` | %t | %s | %t |\n",
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
	toolutil.WriteHints(
		&b,
		"Use `gitlab_list_project_mirrors` to view all configured mirrors",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)    // Output
	toolutil.RegisterMarkdown(FormatListMarkdown)      // ListOutput
	toolutil.RegisterMarkdown(FormatPublicKeyMarkdown) // PublicKeyOutput
}
