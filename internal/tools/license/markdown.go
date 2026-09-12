package license

import (
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatLicenseMarkdown renders one installed license as the card of one
// object: the plan and the seat figures, the three dates, and the licensee and
// the add-ons as nested objects.
func FormatLicenseMarkdown(item Item) *mcp.CallToolResult {
	var b strings.Builder
	c := toolutil.NewCard(&b, "License #"+strconv.FormatInt(item.ID, 10))
	c.Int("ID", item.ID)
	c.Field("Plan", item.Plan)
	// Expiry is marked with the warning sign rather than rendered as a flag:
	// a tick beside "Expired" reads as success, which is the opposite of what
	// an expired license means.
	c.Warn("Expired", item.Expired)
	c.Int("Active Users", item.ActiveUsers)
	c.Int("User Limit", item.UserLimit)
	c.Int("Maximum User Count", item.MaximumUserCount)
	c.Int("Historical Max", item.HistoricalMax)
	c.Int("Overage", item.Overage)
	c.Time("Starts At", item.StartsAt)
	c.Time("Expires At", item.ExpiresAt)
	c.Time("Created At", item.CreatedAt)
	writeLicensee(c, item.Licensee)
	writeAddOns(c, item.AddOns)
	c.End("Check license expiry date and plan for renewal if needed")
	return toolutil.ToolResultWithMarkdown(b.String())
}

// writeLicensee writes who the license was issued to, as a nested object, and
// writes nothing when GitLab sent none of the three fields.
//
// The row it replaces was "%s (%s) - %s", which rendered " () - " for a
// licensee GitLab sent nothing about and "Acme () - ops@acme.test" whenever
// one of the three was missing.
func writeLicensee(c *toolutil.Card, l LicenseeItem) {
	if l == (LicenseeItem{}) {
		return
	}
	// All three are free text whoever bought the license typed.
	s := c.Sub("Licensee")
	s.Field("Name", l.Name)
	s.Field("Company", l.Company)
	s.Field("Email", l.Email)
}

// writeAddOns writes the add-on seat counts the license carries, which the
// card used to publish in its JSON and drop from the Markdown entirely. An
// add-on with no seats is not licensed, so its row is left out, and a license
// with no add-ons at all opens no nested object.
func writeAddOns(c *toolutil.Card, a AddOnsItem) {
	if a == (AddOnsItem{}) {
		return
	}
	s := c.Sub("Add-ons")
	s.Count("Auditor User", a.GitLabAuditorUser)
	s.Count("Deploy Board", a.GitLabDeployBoard)
	s.Count("File Locks", a.GitLabFileLocks)
	s.Count("Geo", a.GitLabGeo)
	s.Count("Service Desk", a.GitLabServiceDesk)
}

// FormatGetMarkdown formats a GetOutput.
func FormatGetMarkdown(output GetOutput) *mcp.CallToolResult {
	return FormatLicenseMarkdown(output.License)
}

// FormatAddMarkdown formats an AddOutput.
func FormatAddMarkdown(output AddOutput) *mcp.CallToolResult {
	return FormatLicenseMarkdown(output.License)
}

func init() {
	toolutil.RegisterMarkdownResult(FormatLicenseMarkdown)
	toolutil.RegisterMarkdownResult(FormatGetMarkdown)
	toolutil.RegisterMarkdownResult(FormatAddMarkdown)
}
