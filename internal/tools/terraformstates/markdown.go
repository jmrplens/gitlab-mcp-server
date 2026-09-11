package terraformstates

import (
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// hintLockNeedsCLI is what a reader has to know about locking a state through
// this server: `gitlab_lock_terraform_state` cannot work, because client-go
// sends no Terraform lock-info body and GitLab rejects the call with 400.
// Saying so where the lock is mentioned is the point; pointing at the tool
// instead sent a reader to a call that always fails.
const (
	hintLockNeedsCLI = "Locking a state needs the terraform CLI against the GitLab HTTP backend: " +
		"`gitlab_lock_terraform_state` sends no lock-info body, which GitLab refuses"
	hintUnlockStaleLock = "Use `gitlab_unlock_terraform_state` to clear a stale lock"
)

// FormatListMarkdown formats Terraform states as markdown.
//
// The endpoint sends no pagination headers, so the heading counts what is
// shown; the empty pagination is what says so rather than a zero total.
func FormatListMarkdown(out ListOutput) string {
	if len(out.States) == 0 {
		return toolutil.EmptyMessage("Terraform states")
	}
	var sb strings.Builder
	toolutil.WriteListHeading(&sb, "Terraform States", len(out.States), toolutil.PaginationOutput{})
	sb.WriteString(toolutil.MarkdownTableHeader("Name", "Latest Serial"))
	for _, s := range out.States {
		sb.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(s.Name),
			serialCell(s.LatestSerial),
		))
	}
	// The table carries no link, so the footer carries no instruction to keep
	// the links of a table that has none.
	toolutil.WriteListFooter(&sb, toolutil.PaginationOutput{}, false,
		"Use `gitlab_get_terraform_state` to view details of a specific state")
	return sb.String()
}

// serialCell renders a state's latest serial, and says "no versions" for a
// zero: GitLab omits the serial for a state nothing has written yet, and a
// bare 0 reads as a version numbered zero.
func serialCell(serial uint64) string {
	if serial == 0 {
		return "no versions"
	}
	return strconv.FormatUint(serial, 10)
}

// FormatStateMarkdown formats a single Terraform state as the card of one
// object. A state nothing has written to yet carries neither a serial nor a
// download path, and says so once instead of showing two labels with nothing
// after them.
func FormatStateMarkdown(s StateItem) string {
	var b strings.Builder
	// The state's name is chosen by whoever ran terraform against it, and the
	// download path is built around that name.
	c := toolutil.NewCard(&b, stateHeading(s.Name))
	// The serial is a uint64, so it is written through its own formatter
	// rather than through Card.Count, which takes an int64 and would need a
	// conversion that can misrepresent a large value. Zero is skipped for the
	// reason Count skips it: GitLab omits the serial for a state nothing has
	// written yet.
	if s.LatestSerial != 0 {
		c.Field("Latest Serial", strconv.FormatUint(s.LatestSerial, 10))
	}
	c.Code("Download Path", s.DownloadPath)
	if s.LatestSerial == 0 && strings.TrimSpace(s.DownloadPath) == "" {
		c.Note("GitLab has recorded no versions of this state.")
	}
	c.End(
		hintLockNeedsCLI,
		hintUnlockStaleLock,
		"Use `gitlab_delete_terraform_state` to remove it",
	)
	return b.String()
}

// stateHeading names the card after the state when GitLab sent a name, and
// after the resource alone when it did not, so the heading never ends in a
// colon with nothing behind it.
func stateHeading(name string) string {
	if strings.TrimSpace(name) == "" {
		return "Terraform State"
	}
	return "Terraform State: " + name
}

// FormatLockMarkdown formats a lock/unlock result as the card of one object.
func FormatLockMarkdown(out LockOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Terraform State Lock")
	c.Bool("Success", out.Success)
	c.Field("Message", out.Message)
	c.End(hintLockNeedsCLI, hintUnlockStaleLock)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatStateMarkdown)
	toolutil.RegisterMarkdown(FormatLockMarkdown)
}
