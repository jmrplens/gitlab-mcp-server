package accessrequests

import (
	"fmt"
	"strconv"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// accessLevelLabel renders a membership access level as the role GitLab means
// by it with the number beside it, "Developer (30)". The number alone was what
// the card printed, and it is the one part of the pair a reader cannot act on.
func accessLevelLabel(level int) string {
	return fmt.Sprintf("%s (%d)", toolutil.AccessLevelDescription(gl.AccessLevelValue(level)), level)
}

// FormatOutputMarkdown renders one pending access request as a card. It
// carries no access level, no membership state and no expiry, because the
// entity behind it sends none of them: what a pending request has is a person
// and the moment they asked.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Access Request #%d", out.ID))
	c.Int("ID", out.ID)
	c.Field("Username", toolutil.MdUserHandle(out.Username))
	c.Field("Name", out.Name)
	c.Field("State", out.State)
	c.Warn("Locked", out.Locked)
	if out.PublicEmail != "" {
		c.Field("Public Email", out.PublicEmail)
	}
	c.Time("Requested At", out.RequestedAt)
	c.URL(out.WebURL)
	c.End(
		toolutil.HintAction(actionAccessApproveProject, "approve this request at project scope"),
		toolutil.HintAction(actionAccessApproveGroup, "approve this request at group scope"),
		toolutil.HintAction(actionAccessDenyProject, "deny this request at project scope"),
		toolutil.HintAction(actionAccessDenyGroup, "deny this request at group scope"),
	)
	return b.String()
}

// FormatMemberOutputMarkdown renders the membership an approved request became.
// It is the one card of this package with an access level on it, and the guard
// is what keeps a level GitLab did not send from reading as the role called
// "No access": the same reason the list table no longer has that column at all.
func FormatMemberOutputMarkdown(out MemberOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Access Request #%d Approved", out.ID))
	c.Int("ID", out.ID)
	c.Field("Username", toolutil.MdUserHandle(out.Username))
	c.Field("Name", out.Name)
	c.Field("State", out.State)
	if out.AccessLevel != 0 {
		c.Field("Access Level", accessLevelLabel(out.AccessLevel))
	}
	c.Warn("Locked", out.Locked)
	if out.Email != "" {
		c.Field("Email", out.Email)
	}
	if out.PublicEmail != "" {
		c.Field("Public Email", out.PublicEmail)
	}
	if out.MemberRole != nil {
		c.Field("Member Role", out.MemberRole.Name)
	}
	if out.MembershipState != "" {
		c.Field("Membership State", out.MembershipState)
	}
	if by := out.CreatedBy; by != nil {
		sub := c.Sub("Created By")
		sub.Field("Name", by.Name)
		sub.Link("Username", toolutil.MdUserHandle(by.Username), by.WebURL)
	}
	c.Time("Created At", out.CreatedAt)
	c.Time("Expires At", out.ExpiresAt)
	c.URL(out.WebURL)
	c.End(
		toolutil.HintAction(actionAccessRequestListProject, "list the requests still pending at project scope"),
		toolutil.HintAction(actionAccessRequestListGroup, "list the requests still pending at group scope"),
	)
	return b.String()
}

// FormatListMarkdown renders a list of access requests as a table.
//
// There is no access-level column. GitLab answers these two routes with the
// access requester entity, which sends no such key, so every row used to read
// "No access (0)": a state none of those requests was in, rendered from a zero
// that came from the decode rather than from GitLab.
func FormatListMarkdown(out ListOutput) string {
	if len(out.AccessRequests) == 0 {
		return toolutil.EmptyMessage("access requests")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Access Requests", len(out.AccessRequests), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Username", "Name", "State", "Requested At"))
	for _, ar := range out.AccessRequests {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(ar.ID, 10),
			toolutil.MdUserLink(ar.Username, ar.WebURL),
			toolutil.EscapeMdTableCell(ar.Name),
			toolutil.EscapeMdTableCell(ar.State),
			toolutil.EscapeMdTableCell(toolutil.FormatTime(ar.RequestedAt)),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(actionAccessApproveProject, "approve one of these requests at project scope"),
		toolutil.HintAction(actionAccessApproveGroup, "approve one of these requests at group scope"),
		toolutil.HintAction(actionAccessDenyProject, "deny one of these requests at project scope"),
		toolutil.HintAction(actionAccessDenyGroup, "deny one of these requests at group scope"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatMemberOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
