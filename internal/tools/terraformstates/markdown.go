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
//
// Each is one literal rather than two joined with a plus: a package-level
// declaration carries no coverage counter, so a mutation run files the
// arithmetic mutant on that plus as not covered, and no test can kill it,
// since subtracting one string from another does not compile.
const (
	hintLockNeedsCLI    = "Locking a state needs the terraform CLI against the GitLab HTTP backend: `admin.terraform_state_lock` sends no lock-info body, which GitLab refuses"
	hintUnlockStaleLock = "Use `admin.terraform_state_unlock` to clear a stale lock"
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
	sb.WriteString(toolutil.MarkdownTableHeader("Name", "Latest Serial", "Updated", "Locked Since", "Deleted"))
	for _, s := range out.States {
		sb.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(s.Name),
			serialCell(s.LatestSerial),
			toolutil.FormatTime(s.UpdatedAt),
			lockedCell(s.LockedAt),
			deletedCell(s.DeletedAt),
		))
	}
	// The table carries no link, so the footer carries no instruction to keep
	// the links of a table that has none.
	toolutil.WriteListFooter(&sb, toolutil.PaginationOutput{}, false,
		"Use `admin.terraform_state_get` to view details of a specific state")
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

// lockedCell renders when a state was locked, and says "unlocked" for a state
// GitLab sent no lock time for, since a blank cell in a column about locks
// reads as a value nobody knows.
func lockedCell(lockedAt string) string {
	if lockedAt == "" {
		return "unlocked"
	}
	return toolutil.FormatTime(lockedAt)
}

// deletedCell renders when a state was deleted, and says "no" for a state
// GitLab sent no deletion time for. GitLab deletes a state in the background
// and keeps it readable until the worker removes it, for a grace period when
// delayed deletion is on, so a list can hold a deleted state, and without this
// column it would read as live.
func deletedCell(deletedAt string) string {
	if deletedAt == "" {
		return "no"
	}
	return toolutil.FormatTime(deletedAt)
}

// FormatStateMarkdown formats a single Terraform state as the card of one
// object. A state nothing has written to yet carries neither a serial nor a
// download path, and says so once instead of showing two labels with nothing
// after them. A lock and a pending deletion are written only while they hold.
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
	c.Time("Created", s.CreatedAt)
	c.Time("Updated", s.UpdatedAt)
	c.Time("Locked", s.LockedAt)
	c.Time("Deleted", s.DeletedAt)
	if s.LatestSerial == 0 && strings.TrimSpace(s.DownloadPath) == "" {
		c.Note("GitLab has recorded no versions of this state.")
	}
	// GitLab deletes a state in the background: the delete marks it, and a
	// worker removes it later, so a read in between still answers.
	if s.DeletedAt != "" {
		c.Note("GitLab has deleted this state and removes it in the background; until then it can still be read.")
	}
	c.End(
		hintLockNeedsCLI,
		hintUnlockStaleLock,
		"Use `admin.terraform_state_delete` to remove it",
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
