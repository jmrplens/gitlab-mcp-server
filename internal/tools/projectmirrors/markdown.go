package projectmirrors

import (
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatOutputMarkdown renders one remote mirror as a card. The mirror URL and
// the branch regex are code spans rather than escaped cells: both are values a
// maintainer typed and a reader copies, and a regex holding an alternation used
// to reach the model as `^(feat&#124;fix):`, which is not the pattern GitLab
// holds.
func FormatOutputMarkdown(m Output) string {
	if m.ID == 0 {
		return ""
	}
	var b strings.Builder
	c := toolutil.NewCard(&b, "Remote Mirror #"+strconv.FormatInt(m.ID, 10))
	// The mirror URL is whatever the maintainer configuring the mirror typed.
	c.Code("URL", m.URL)
	c.Bool("Enabled", m.Enabled)
	c.Field("Status", m.UpdateStatus)
	c.Field("Auth Method", m.AuthMethod)
	c.Bool("Only Protected Branches", m.OnlyProtectedBranches)
	c.Bool("Keep Divergent Refs", m.KeepDivergentRefs)
	c.Code("Branch Regex", m.MirrorBranchRegex)
	// GitLab quotes the remote's own output back in this field, so a hostile
	// remote writes it.
	c.Text("Last Error", m.LastError)
	c.Time("Last Successful Update", m.LastSuccessfulUpdateAt)
	c.Time("Last Update", m.LastUpdateAt)
	if len(m.HostKeys) > 0 {
		keys := c.Table("Host Keys", "Fingerprint (SHA256)")
		for _, hk := range m.HostKeys {
			keys.Row(toolutil.MdCodeSpanCell(hk.FingerprintSHA256))
		}
	}
	c.End(
		toolutil.HintAction(actionMirrorEdit, "modify this mirror's settings"),
		toolutil.HintAction(actionMirrorForcePush, "trigger an immediate sync"),
		toolutil.HintAction(actionMirrorGetPublicKey, "retrieve the SSH public key"),
	)
	return b.String()
}

// FormatListMarkdown renders a page of remote mirrors as a table. The URL cell
// is a code span and no cell carries a link, so the footer drops the
// instruction to preserve links.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Mirrors) == 0 {
		return toolutil.EmptyMessage("remote mirrors")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Remote Mirrors", len(out.Mirrors), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "URL", "Enabled", "Status", "Protected Only"))
	for _, m := range out.Mirrors {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(m.ID, 10),
			toolutil.MdCodeSpanCell(m.URL),
			toolutil.BoolEmoji(m.Enabled),
			toolutil.EscapeMdTableCell(m.UpdateStatus),
			toolutil.BoolEmoji(m.OnlyProtectedBranches),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionMirrorGet, "see one mirror in full"))
	return b.String()
}

// FormatPublicKeyMarkdown renders a mirror's SSH public key inside a fence
// sized to the key, so nothing in it can close the block early.
func FormatPublicKeyMarkdown(pk PublicKeyOutput) string {
	if pk.PublicKey == "" {
		return "No public key available.\n"
	}
	var b strings.Builder
	c := toolutil.NewCard(&b, "Mirror SSH Public Key")
	c.Fence("", "", pk.PublicKey)
	c.End(toolutil.HintAction(actionMirrorList, "view all configured mirrors"))
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)    // Output
	toolutil.RegisterMarkdown(FormatListMarkdown)      // ListOutput
	toolutil.RegisterMarkdown(FormatPublicKeyMarkdown) // PublicKeyOutput
}
