package events

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

func init() {
	toolutil.RegisterMarkdown(FormatContributionListMarkdownString)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
}

// formatTarget builds the target description with a clickable link.
//
// The label is [toolutil.FormatTarget]'s: the target title when GitLab sent
// one, and "<type> #<iid>" when it did not, and nothing at all when it sent
// neither, so an event about no object no longer renders a link to "Issue #0".
// The reference follows the title in parentheses when the title took the
// label, since the type is what says whether the event is about an issue or a
// merge request.
func formatTarget(targetType string, targetIID int64, targetTitle, targetURL string) string {
	label := toolutil.FormatTarget(targetType, targetIID, targetTitle, targetURL)
	if label == "" {
		return ""
	}
	out := " " + label
	if toolutil.EscapeMdTableCell(targetTitle) == "" || targetType == "" {
		return out
	}
	// The target type is a class name GitLab writes and the IID an integer, so
	// the reference carries nothing a list item reacts to; it is escaped all
	// the same, since the type reaches here as a published output field.
	ref := toolutil.EscapeMdTableCell(targetType)
	if targetIID > 0 {
		ref = fmt.Sprintf("%s #%d", ref, targetIID)
	}
	return out + " (" + ref + ")"
}

// formatPushData names what a push event pushed: the ref by its kind, the
// number of commits, and the title of the newest one. All three are what a
// reader of "pushed to" wants and none of them was rendered, so every push
// event read as the bare word "pushed to" whichever branch it touched.
func formatPushData(p *pushData) string {
	if p == nil {
		return ""
	}
	parts := make([]string, 0, 2)
	if p.Ref != "" {
		kind := p.RefType
		if kind == "" {
			kind = "ref"
		}
		// A git ref is not an identifier: git check-ref-format permits '|',
		// '<' and '>', and a reader copies a branch name verbatim.
		parts = append(parts, toolutil.EscapeMdTableCell(kind)+" "+toolutil.MdCodeSpan(p.Ref))
	}
	detail := make([]string, 0, 2)
	if p.CommitCount > 0 {
		detail = append(detail, fmt.Sprintf("%d commit%s", p.CommitCount, plural(p.CommitCount)))
	}
	if p.CommitTitle != "" {
		detail = append(detail, `latest "`+toolutil.EscapeMdTableCell(p.CommitTitle)+`"`)
	}
	if len(detail) > 0 {
		parts = append(parts, "("+strings.Join(detail, ", ")+")")
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

// plural returns the plural suffix for n.
func plural(n int64) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// formatNoteTarget names the issue or merge request a comment event's note
// hangs on. The event's own target is the note, whose title is a slice of the
// comment body, so without this the reader is never told what was commented on.
func formatNoteTarget(noteableType string, noteableIID int64) string {
	if noteableType == "" {
		return ""
	}
	if noteableIID <= 0 {
		return " on " + toolutil.EscapeMdTableCell(noteableType)
	}
	return fmt.Sprintf(" on %s #%d", toolutil.EscapeMdTableCell(noteableType), noteableIID)
}

// formatAttribution says who did the thing and when, naming only the halves
// GitLab sent: an event with neither used to end "by , ", two separators
// around nothing.
func formatAttribution(author, when string) string {
	parts := make([]string, 0, 2)
	if author != "" {
		parts = append(parts, "by "+toolutil.EscapeMdTableCell(author))
	}
	if when != "" {
		parts = append(parts, when)
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, ", ")
}

// formatWikiPage names the wiki page an event happened to, and nothing for an
// event about anything else.
func formatWikiPage(page *WikiPageOutput) string {
	if page == nil || page.Title == "" {
		return ""
	}
	return ` wiki page "` + toolutil.EscapeMdTableCell(page.Title) + `"`
}

// formatOrigin says which platform an event was imported from, and nothing for
// an event that happened on this instance.
func formatOrigin(imported bool, importedFrom string) string {
	if !imported {
		return ""
	}
	if importedFrom == "" {
		return " (imported)"
	}
	return fmt.Sprintf(" (imported from %s)", toolutil.EscapeMdTableCell(importedFrom))
}

// pushData is the push payload of an event, the same shape at contribution and
// project scope, carried here so one renderer serves both.
type pushData struct {
	CommitCount int64
	RefType     string
	Ref         string
	CommitTitle string
}

type markdownEvent struct {
	ActionName     string
	TargetType     string
	TargetIID      int64
	TargetTitle    string
	TargetURL      string
	AuthorUsername string
	CreatedAt      string
	WikiPage       *WikiPageOutput
	Imported       bool
	ImportedFrom   string
	Push           *pushData
	NoteableType   string
	NoteableIID    int64
}

// FormatContributionListMarkdown formats contribution events as a Markdown CallToolResult.
func FormatContributionListMarkdown(out ListContributionEventsOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatContributionListMarkdownString(out))
}

// FormatContributionListMarkdownString renders contribution events as a Markdown string.
func FormatContributionListMarkdownString(out ListContributionEventsOutput) string {
	return formatEventListMarkdown("Contribution Events", "contribution events", contributionMarkdownEvents(out.Events), out.Pagination)
}

// FormatListMarkdown formats project events as a Markdown CallToolResult.
func FormatListMarkdown(out ListProjectEventsOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out))
}

// FormatListMarkdownString renders project events as a Markdown string.
func FormatListMarkdownString(out ListProjectEventsOutput) string {
	return formatEventListMarkdown("Project Events", "project events", projectMarkdownEvents(out.Events), out.Pagination)
}

// formatEventListMarkdown renders a page of events as one list item each: an
// event is a sentence about what somebody did, not a row of shared columns.
//
// The heading is [toolutil.WriteListHeading]'s, which counts what the response
// can vouch for: the count used to be the length of the page, so a page of two
// under a total of forty-five announced "(2)".
func formatEventListMarkdown(title, emptyResource string, events []markdownEvent, pagination toolutil.PaginationOutput) string {
	if len(events) == 0 {
		return toolutil.EmptyMessage(emptyResource)
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, title, len(events), pagination)
	linked := false
	for _, e := range events {
		target := formatTarget(e.TargetType, e.TargetIID, e.TargetTitle, e.TargetURL)
		if strings.Contains(target, "](") {
			linked = true
		}
		//gitlab:allow-unescaped e.ActionName: a contribution-event action GitLab writes from its own vocabulary (opened, closed, pushed to and the rest).
		fmt.Fprintf(&b, "- **%s**%s%s%s%s%s%s\n", e.ActionName, target,
			formatPushData(e.Push), formatNoteTarget(e.NoteableType, e.NoteableIID), formatWikiPage(e.WikiPage),
			formatAttribution(formatAuthor(e.AuthorUsername), toolutil.FormatTime(e.CreatedAt)),
			formatOrigin(e.Imported, e.ImportedFrom))
	}
	toolutil.WriteListFooter(&b, pagination, linked,
		toolutil.HintPreserveLinks,
		"Filter events using action and target_type parameters",
	)
	return b.String()
}

//nolint:dupl // ContributionEventOutput and ProjectEventOutput are 1:1 mirrors of two distinct GitLab entities with no shared interface, and each carries its own push-data and note types; the parallel shape is the API's.
func contributionMarkdownEvents(events []ContributionEventOutput) []markdownEvent {
	items := make([]markdownEvent, len(events))
	for i, event := range events {
		items[i] = markdownEvent{
			ActionName:     event.ActionName,
			TargetType:     event.TargetType,
			TargetIID:      event.TargetIID,
			TargetTitle:    event.TargetTitle,
			TargetURL:      event.TargetURL,
			AuthorUsername: event.AuthorUsername,
			CreatedAt:      event.CreatedAt,
			WikiPage:       event.WikiPage,
			Imported:       event.Imported,
			ImportedFrom:   event.ImportedFrom,
		}
		if p := event.PushData; p != nil {
			items[i].Push = &pushData{CommitCount: p.CommitCount, RefType: p.RefType, Ref: p.Ref, CommitTitle: p.CommitTitle}
		}
		if n := event.Note; n != nil {
			items[i].NoteableType, items[i].NoteableIID = n.NoteableType, n.NoteableIID
		}
	}
	return items
}

//nolint:dupl // ContributionEventOutput and ProjectEventOutput are 1:1 mirrors of two distinct GitLab entities with no shared interface, and each carries its own push-data and note types; the parallel shape is the API's.
func projectMarkdownEvents(events []ProjectEventOutput) []markdownEvent {
	items := make([]markdownEvent, len(events))
	for i, event := range events {
		items[i] = markdownEvent{
			ActionName:     event.ActionName,
			TargetType:     event.TargetType,
			TargetIID:      event.TargetIID,
			TargetTitle:    event.TargetTitle,
			TargetURL:      event.TargetURL,
			AuthorUsername: event.AuthorUsername,
			CreatedAt:      event.CreatedAt,
			WikiPage:       event.WikiPage,
			Imported:       event.Imported,
			ImportedFrom:   event.ImportedFrom,
		}
		if p := event.PushData; p != nil {
			items[i].Push = &pushData{CommitCount: p.CommitCount, RefType: p.RefType, Ref: p.Ref, CommitTitle: p.CommitTitle}
		}
		if n := event.Note; n != nil {
			items[i].NoteableType, items[i].NoteableIID = n.NoteableType, n.NoteableIID
		}
	}
	return items
}
