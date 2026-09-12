package appearance

import (
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatGetMarkdown renders the instance appearance as the card of one object.
func FormatGetMarkdown(out GetOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(
		appearanceCard("Application Appearance", out.Appearance,
			"Use `gitlab_update_appearance` to modify appearance settings"),
	)
}

// FormatUpdateMarkdown renders the appearance GitLab answered the update with.
//
// It used to call the read formatter, so the card said "Application
// Appearance" and a reader could not tell a read from a write; the heading
// now names what happened.
func FormatUpdateMarkdown(out UpdateOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(
		appearanceCard("Application Appearance Updated", out.Appearance,
			"Use `gitlab_get_appearance` to read the settings back"),
	)
}

// appearanceCard writes the appearance as one card: the instance identity and
// the branding assets as rows, then the five pieces of Markdown an
// administrator typed as quoted bodies, then the hints.
//
// Every field the output type carries is written. The card used to render
// eight of eighteen, so the logos, the favicon, the PWA description, the two
// message colors and all three guideline texts were fetched, published in the
// JSON and dropped from the Markdown a reader sees.
func appearanceCard(heading string, a Item, hint string) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, heading)
	// Every value below is free text an administrator typed into the appearance
	// settings; the card escapes each one at the row that writes it.
	c.Field("Site Name", a.SiteName)
	c.Field("Title", a.Title)
	c.Field("PWA Name", a.PWAName)
	c.Field("PWA Short Name", a.PWAShortName)
	c.Bool("Email Header/Footer", a.EmailHeaderAndFooterEnabled)
	// The five asset fields are instance-relative upload paths, and the two
	// colors are the hex values the appearance form takes.
	c.Field("Logo", a.Logo)
	c.Field("Header Logo", a.HeaderLogo)
	c.Field("Favicon", a.Favicon)
	c.Field("PWA Icon", a.PWAIcon)
	c.Field("Message Background Color", a.MessageBackgroundColor)
	c.Field("Message Font Color", a.MessageFontColor)
	// The rest is Markdown GitLab renders itself, so each is quoted rather than
	// written into the card's own list.
	c.Text("Description", a.Description)
	c.Text("PWA Description", a.PWADescription)
	c.Text("Header Message", a.HeaderMessage)
	c.Text("Footer Message", a.FooterMessage)
	c.Text("Member Guidelines", a.MemberGuidelines)
	c.Text("New Project Guidelines", a.NewProjectGuidelines)
	c.Text("Profile Image Guidelines", a.ProfileImageGuidelines)
	c.End(hint)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdownResult(FormatGetMarkdown)
	toolutil.RegisterMarkdownResult(FormatUpdateMarkdown)
}
