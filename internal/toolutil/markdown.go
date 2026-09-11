package toolutil

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DateFormatISO is the date-only layout GitLab uses for a due date or a start
// date. The literal-Z timestamp layout that sat beside it is gone: it stamped
// whatever wall clock a value carried with a zone it may not have been in,
// and [RFC3339] and [RFC3339Ptr] convert to UTC before they write.
const DateFormatISO = "2006-01-02"

// TblFieldValue is the "| Field | Value |" detail table header and separator.
//
// It is the table form of a one-object card, which [Card] supersedes: a card
// row is a list item, and a table is for a collection of objects that share
// columns. The constant stays until every formatter that opens a card as a
// table has moved; a new formatter starts a card with [NewCard].
const TblFieldValue = "| Field | Value |\n| --- | --- |\n"

// Table row format constants for common detail-table fields. Each pairs with
// TblFieldValue and is superseded by the [Card] rows of the same name:
// [Card.Int] for the ID, [Card.Field] for the status, [Card.Time] for the two
// timestamps, and [Card.Warn] for a failure flag, which marks the condition
// with the warning sign rather than rendering the boolean as "true".
const (
	TblRowID          = "| ID | %d |\n"
	TblRowStatus      = "| Status | %s |\n"
	TblRowCreatedAt   = "| Created At | %s |\n"
	TblRowUpdatedAt   = "| Updated At | %s |\n"
	TblRowHasFailures = "| Has Failures | %v |\n"
)

// Headings format string with trailing blank line.
const (
	FmtMdH1 = "# %s\n\n"
	FmtMdH2 = "## %s\n\n"
	FmtMdH3 = "### %s\n\n"
	FmtMdH4 = "#### %s\n\n"
	FmtMdH5 = "##### %s\n\n"
	FmtMdH6 = "###### %s\n\n"
)

// Markdown format constants for repeated table separators and field patterns.
//
// The FmtMd* card rows are superseded by [Card], which writes the same line
// through one writer and escapes the value at the write; they stay until the
// formatters that use them have moved.
const (
	FmtMdID          = "- **ID**: %d\n"
	FmtMdName        = "- **Name**: %s\n"
	FmtMdTitle       = "- **Title**: %s\n"
	FmtMdState       = "- **State**: %s\n"
	FmtMdStatus      = "- **Status**: %s\n"
	FmtMdDescription = "- **Description**: %s\n"
	FmtMdPath        = "- **Path**: %s\n"
	FmtMdVisibility  = "- **Visibility**: %s\n"
	FmtMdEmail       = "- **Email**: %s\n"
	FmtMdUsername    = "- **Username**: %s\n"
	FmtMdTarget      = "- **Target**: %s\n"
	FmtMdCreated     = "- **Created**: %s\n"
	FmtMdUpdated     = "- **Updated**: %s\n"
	fmtMdURLLine     = "- **URL**: %s\n"
	FmtMdAuthorAt    = "- **Author**: @%s\n"
	FmtMdAuthor      = "- **Author**: %s\n"
	FmtMdSectionText = "\n%s\n"
	FmtMdH2Count     = "## %s (%d)\n\n"
	TblSep1Col       = "| --- |\n"
	TblSep2Col       = "| --- | --- |\n"
	TblSep3Col       = "| --- | --- | --- |\n"
	TblSep4Col       = "| --- | --- | --- | --- |\n"
	TblSep5Col       = "| --- | --- | --- | --- | --- |\n"
	TblSep6Col       = "| --- | --- | --- | --- | --- | --- |\n"
	TblSep7Col       = "| --- | --- | --- | --- | --- | --- | --- |\n"
	TblSep8Col       = "| --- | --- | --- | --- | --- | --- | --- | --- |\n"
	TblSep9Col       = "| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n"
	TblSep10Col      = "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n"
	FmtRow1Str       = "| %s |\n"
	FmtRow2Str       = "| %s | %s |\n"
	FmtRow3Str       = "| %s | %s | %s |\n"
	FmtRow4Str       = "| %s | %s | %s | %s |\n"
	FmtRow5Str       = "| %s | %s | %s | %s | %s |\n"
	FmtRow6Str       = "| %s | %s | %s | %s | %s | %s |\n"
	FmtRow7Str       = "| %s | %s | %s | %s | %s | %s | %s |\n"
	FmtRow8Str       = "| %s | %s | %s | %s | %s | %s | %s | %s |\n"
	FmtRow9Str       = "| %s | %s | %s | %s | %s | %s | %s | %s | %s |\n"
	FmtRow10Str      = "| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n"
)

// Contextual emoji constants for consistent visual indicators across formatters.
//
// EmojiWarning is the negative-polarity flag [Card.Warn] writes: a condition
// such as revoked, locked, expired or has failures is marked with it and never
// with the tick [BoolEmoji] gives a true, which on such a label reads as
// success.
const (
	EmojiDraft        = "\U0001F4DD" // 📝
	EmojiWarning      = "⚠️"         // ⚠️
	EmojiConfidential = "\U0001F512" // 🔒
	EmojiArchived     = "\U0001F4E6" // 📦
	EmojiStar         = "⭐"          // ⭐
	EmojiSuccess      = "✅"          // ✅
	EmojiCross        = "❌"          // ❌
	EmojiRefresh      = "\U0001F504" // 🔄
	EmojiFile         = "\U0001F4C4" // 📄
	EmojiFolder       = "\U0001F4C1" // 📁
	EmojiCalendar     = "\U0001F4C5" // 📅
	EmojiUpArrow      = "⬆️"         // ⬆️
	EmojiDownArrow    = "⬇️"         // ⬇️
	EmojiInfo         = "ℹ️"         // ℹ️
	EmojiQuestion     = "❓"          // ❓
	EmojiLink         = "\U0001F517" // 🔗
	EmojiUser         = "\U0001F464" // 👤
	EmojiGroup        = "\U0001F465" // 👥
	EmojiPipeline     = "\U0001F6A7" // 🚧
	EmojiMergeRequest = "\U0001F5C3" // 🗃️
	EmojiIssue        = "\U0001F4A1" // 💡
	EmojiRed          = "\U0001F534" // 🔴
	EmojiOrange       = "\U0001F7E0" // 🟠
	EmojiYellow       = "\U0001F7E1" // 🟡
	EmojiGreen        = "\U0001F7E2" // 🟢
	EmojiProhibited   = "\U0001F6AB" // 🚫
	EmojiWhiteCircle  = "⚪"          // ⚪
	EmojiParty        = "\U0001F389" // 🎉
	EmojiPurple       = "\U0001F7E3" // 🟣
	EmojiBlue         = "\U0001F535" // 🔵
	EmojiStop         = "⛔"          // ⛔
	EmojiSkip         = "⏭️"         // ⏭️
	EmojiNew          = "\U0001F195" // 🆕
	EmojiHand         = "✋"          // ✋
)

// WriteMdURL appends the "- **URL**: ..." line, rendering url as a link whose
// label and destination are the same address.
//
// It replaces a pair of format constants that read "- **URL**: [%[1]s](%[1]s)"
// and were used at 31 call sites. One value filling both halves of a link is a
// shape no argument can be escaped into safely, so each of those call sites
// hand-wrote a link with nothing in front of it, and the audit flagged every
// one of them twice. Escaping belongs here, once, rather than in a decision
// each of 22 packages makes for itself.
//
// [Card.URL] is the same row on a card, and writes nothing for an empty
// address where this writes a bare label.
func WriteMdURL(b *strings.Builder, url string) {
	fmt.Fprintf(b, fmtMdURLLine, MdTitleLink(url, url))
}

// WriteMdURLNewline appends the same line as [WriteMdURL], preceded by a blank
// line, for a formatter that closes a section with it.
func WriteMdURLNewline(b *strings.Builder, url string) {
	b.WriteString("\n")
	WriteMdURL(b, url)
}

// WritePagination appends the pagination footer after a blank line, whatever
// the builder ends with, so the footer opens a paragraph of its own rather
// than continuing the last table row or list item as a lazy line. A
// pagination that says nothing (no page, no total, no page size) writes
// nothing, since "Page 0 of 0 | 0 items total" is an answer GitLab never
// gave.
func WritePagination(b *strings.Builder, p PaginationOutput) {
	line := formatPagination(p)
	if line == "" {
		return
	}
	endBlock(b)
	b.WriteString(line)
	b.WriteString("\n")
}

// formatPagination renders pagination metadata as a compact line of what is
// known: the page and the page count when GitLab sent a total, the page alone
// under keyset pagination where no total exists, and whether more pages
// follow. Nothing known renders as nothing.
func formatPagination(p PaginationOutput) string {
	var parts []string
	switch {
	case p.TotalPages > 0 && p.Page > 0:
		parts = append(parts, fmt.Sprintf("Page %d of %d", p.Page, p.TotalPages))
	case p.Page > 0:
		parts = append(parts, fmt.Sprintf("Page %d", p.Page))
	}
	if p.TotalItems > 0 {
		parts = append(parts, fmt.Sprintf("%d items total", p.TotalItems))
	}
	if p.PerPage > 0 {
		parts = append(parts, fmt.Sprintf("%d per page", p.PerPage))
	}
	if p.TotalPages == 0 && p.Page > 0 {
		if p.HasMore || p.NextPage > 0 {
			parts = append(parts, "more pages available")
		} else {
			parts = append(parts, "no more pages")
		}
	}
	return strings.Join(parts, " | ")
}

// WriteListSummary appends a brief "Showing N of M results (page X of Y)"
// line between the heading and the table body. It is a no-op when there is
// only a single page, because the heading count already conveys everything,
// and it names no total when GitLab sent none rather than saying "of 0".
func WriteListSummary(b *strings.Builder, shown int, p PaginationOutput) {
	if p.TotalPages <= 1 {
		return
	}
	if p.TotalItems > 0 {
		fmt.Fprintf(b, "Showing %d of %d results (page %d of %d)\n\n", shown, p.TotalItems, p.Page, p.TotalPages)
		return
	}
	fmt.Fprintf(b, "Showing %d results (page %d of %d)\n\n", shown, p.Page, p.TotalPages)
}

// WriteListHeading writes the H2 a list result opens with, "## Title (N)",
// and the summary line [WriteListSummary] adds for a multi-page result.
//
// N is what the response can vouch for: the total GitLab sent when it sent
// one, the count shown with "more available" when the page has a successor
// but no total (keyset pagination sends none), and the count shown otherwise.
// A heading that printed the page length under a larger total, or a zero
// total above a table of rows, was the commonest way a list misled its
// reader.
func WriteListHeading(b *strings.Builder, title string, shown int, p PaginationOutput) {
	count := strconv.Itoa(shown)
	switch {
	case p.TotalItems > 0:
		count = strconv.FormatInt(p.TotalItems, 10)
	case shown > 0 && (p.HasMore || p.NextPage > 0):
		count = fmt.Sprintf("%d shown, more available", shown)
	}
	fmt.Fprintf(b, "## %s (%s)\n\n", EscapeMdHeading(title), count)
	WriteListSummary(b, shown, p)
}

// WriteListFooter closes a list result: the pagination footer through
// [WritePagination], then the guidance section. When linked is true the
// table carried a link column and [HintPreserveLinks] leads the hints, as
// [ListHints] arranges; when it is false the hint is dropped even if the
// caller passed it, since an instruction to keep the links of a table that
// has none is noise the model has to read past.
func WriteListFooter(b *strings.Builder, p PaginationOutput, linked bool, hints ...string) {
	WritePagination(b, p)
	if linked {
		WriteHints(b, ListHints(hints...)...)
		return
	}
	WriteHints(b, withoutPreserveLinks(hints)...)
}

// withoutPreserveLinks drops [HintPreserveLinks] and empty hints from hints.
func withoutPreserveLinks(hints []string) []string {
	out := make([]string, 0, len(hints))
	for _, hint := range hints {
		if hint == "" || hint == HintPreserveLinks {
			continue
		}
		out = append(out, hint)
	}
	return out
}

// WriteGraphQLPagination writes the cursor summary of a GraphQL list after a
// blank line, the one way a cursor-paginated list ends: the line used to be
// written four ways across ten packages, one of them with no blank line in
// front of it, which glued it to the last row of the table.
func WriteGraphQLPagination(b *strings.Builder, p GraphQLPaginationOutput, shown int) {
	endBlock(b)
	b.WriteString(FormatGraphQLPagination(p, shown))
	b.WriteString("\n")
}

// EmptyMessage is the one sentence an empty list renders, "No <resource>
// found." with its newline, the whole response of a list with nothing in it.
// The resource is a plural noun the formatter names ("merge requests",
// "protected branches"). It replaces the heading rather than sitting under
// it: a heading counting zero above a sentence saying so said it twice.
func EmptyMessage(resource string) string {
	return "No " + resource + " found.\n"
}

// emptyResult renders the empty-list message a shared renderer was configured
// with as the whole response, ending in exactly one newline whichever way the
// caller spelled it.
func emptyResult(message string) string {
	return strings.TrimRight(message, "\r\n") + "\n"
}

// MdUserHandle renders a username as the "@handle" GitLab shows, escaped for
// a card row or a cell, or nothing for an empty name, so a card never shows a
// bare "@".
func MdUserHandle(username string) string {
	if blank(username) {
		return ""
	}
	return "@" + EscapeMdTableCell(username)
}

// MdUserLink renders a username as its handle linked to the user's profile,
// through [MdTitleLink], the escaped handle alone when the profile URL is
// empty, and nothing for an empty name.
func MdUserLink(username, webURL string) string {
	if blank(username) {
		return ""
	}
	return MdTitleLink("@"+username, webURL)
}

// HintAction composes a next-step hint that names an action by its canonical
// catalog ID, "Use action 'issue.update' to change this issue". The ID is the
// one form every surface accepts: the dynamic surface executes it directly,
// and the meta and individual surfaces resolve it to their own tool names, so
// a hint written this way is never a name the serving surface does not
// register.
func HintAction(actionID, purpose string) string {
	return "Use action '" + actionID + "' to " + purpose
}

// WriteHookSecretKeys writes the tables of a webhook's URL variables and
// custom headers, keys only, with every value redacted: both are secrets
// GitLab itself never sends back, and the key is what a reader needs to
// know which ones are set. Either list may be empty, in which case its table
// is not written.
func WriteHookSecretKeys(b *strings.Builder, urlVariableKeys, customHeaderKeys []string) {
	writeRedactedKeyTable(b, "URL Variables", urlVariableKeys)
	writeRedactedKeyTable(b, "Custom Headers", customHeaderKeys)
}

// writeRedactedKeyTable writes one Key/Value table under an H3, every value
// [RedactedSecretValue].
func writeRedactedKeyTable(b *strings.Builder, title string, keys []string) {
	if len(keys) == 0 {
		return
	}
	endBlock(b)
	fmt.Fprintf(b, "### %s\n\n", title)
	b.WriteString(MarkdownTableHeader("Key", "Value"))
	for _, key := range keys {
		b.WriteString(MarkdownTableRow(EscapeMdTableCell(key), RedactedSecretValue))
	}
}

// SeverityBadge renders a vulnerability or finding severity as its color
// glyph and the level in capitals, the one badge the security domains share.
// A level the table does not know is rendered escaped, since it is a value
// GitLab sent.
func SeverityBadge(severity string) string {
	switch strings.ToUpper(strings.TrimSpace(severity)) {
	case "CRITICAL":
		return EmojiRed + " CRITICAL"
	case "HIGH":
		return EmojiOrange + " HIGH"
	case "MEDIUM":
		return EmojiYellow + " MEDIUM"
	case "LOW":
		return EmojiBlue + " LOW"
	case "INFO":
		return EmojiInfo + " INFO"
	case "UNKNOWN":
		return EmojiQuestion + " UNKNOWN"
	default:
		return EscapeMdTableCell(severity)
	}
}

// mapSlice applies f to every element of in, the one mapping the shared
// renderers used to spell six times over.
func mapSlice[T, U any](in []T, f func(T) U) []U {
	out := make([]U, 0, len(in))
	for _, v := range in {
		out = append(out, f(v))
	}
	return out
}

// endBlock leaves the builder empty or ending in a blank line, whatever it
// ends with now, so what is written next opens a block of its own rather than
// continuing the last line as a lazy paragraph or a table row.
func endBlock(b *strings.Builder) {
	written := b.String()
	switch {
	case written == "" || strings.HasSuffix(written, "\n\n"):
	case strings.HasSuffix(written, "\n"):
		b.WriteString("\n")
	default:
		b.WriteString("\n\n")
	}
}

// MarkdownTableHeader returns a Markdown table header followed by a standard
// separator row for the supplied column labels.
func MarkdownTableHeader(columns ...string) string {
	if len(columns) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(markdownTableLine(columns))
	b.WriteString(markdownTableSeparator(len(columns)))
	return b.String()
}

// markdownTableSeparator returns a standard left-aligned Markdown separator
// row for the requested number of columns.
func markdownTableSeparator(columns int) string {
	if columns <= 0 {
		return ""
	}

	parts := make([]string, columns)
	for i := range parts {
		parts[i] = "---"
	}
	return markdownTableLine(parts)
}

// MarkdownTableRow returns a Markdown table row for the supplied cell values.
func MarkdownTableRow(cells ...string) string {
	if len(cells) == 0 {
		return ""
	}
	return markdownTableLine(cells)
}

// StorageMoveEntityMarkdown carries the optional GitLab resource associated
// with a repository storage move.
type StorageMoveEntityMarkdown struct {
	Label string
	Name  string
	URL   string
	ID    int64
}

// StorageMoveMarkdown carries the common fields rendered by repository storage
// move tools at group, snippet, and other resource scopes.
type StorageMoveMarkdown struct {
	ID                     int64
	State                  string
	SourceStorageName      string
	DestinationStorageName string
	CreatedAt              time.Time
	Entity                 *StorageMoveEntityMarkdown
}

// NewStorageMoveEntityMarkdown builds the optional entity view model for a
// repository storage move.
func NewStorageMoveEntityMarkdown(label, name, url string, id int64) *StorageMoveEntityMarkdown {
	return &StorageMoveEntityMarkdown{
		Label: label,
		Name:  name,
		URL:   url,
		ID:    id,
	}
}

// NewStorageMoveMarkdown builds a shared repository storage move Markdown view
// model without forcing tool packages to duplicate composite literals.
func NewStorageMoveMarkdown(id int64, state, sourceStorageName, destinationStorageName string, createdAt time.Time, entity *StorageMoveEntityMarkdown) StorageMoveMarkdown {
	return StorageMoveMarkdown{
		ID:                     id,
		State:                  state,
		SourceStorageName:      sourceStorageName,
		DestinationStorageName: destinationStorageName,
		CreatedAt:              createdAt,
		Entity:                 entity,
	}
}

// storageMoveListMarkdownOptions configures the shared storage move list
// renderer.
type storageMoveListMarkdownOptions struct {
	Title        string
	EmptyMessage string
	EntityColumn string
	Pagination   PaginationOutput
}

// FormatStorageMoveDetailMarkdown renders one repository storage move as a
// card: the identity and state rows, the two storages, the creation time in
// the display form, and the moved entity as a link with its ID.
func FormatStorageMoveDetailMarkdown(move StorageMoveMarkdown, title string, hints ...string) string {
	var b strings.Builder
	c := NewCard(&b, fmt.Sprintf("%s #%d", title, move.ID))
	c.Int("ID", move.ID)
	c.Field("State", move.State)
	c.Field("Source", move.SourceStorageName)
	c.Field("Destination", move.DestinationStorageName)
	c.Time("Created", RFC3339(move.CreatedAt))
	if move.Entity != nil {
		c.Markdown(move.Entity.Label, storageMoveEntityCell(*move.Entity, true))
	}
	c.End(hints...)
	return b.String()
}

// formatStorageMoveListMarkdown renders repository storage moves as a Markdown
// table with the domain-specific entity column supplied by the caller.
func formatStorageMoveListMarkdown(moves []StorageMoveMarkdown, opts storageMoveListMarkdownOptions) string {
	if len(moves) == 0 {
		return emptyResult(opts.EmptyMessage)
	}
	var b strings.Builder
	WriteListHeading(&b, opts.Title, len(moves), opts.Pagination)
	b.WriteString(MarkdownTableHeader("ID", "State", "Source", "Destination", opts.EntityColumn, "Created"))
	linked := false
	for _, move := range moves {
		entity := ""
		if move.Entity != nil {
			entity = storageMoveEntityCell(*move.Entity, false)
			linked = linked || move.Entity.URL != ""
		}
		b.WriteString(MarkdownTableRow(
			strconv.FormatInt(move.ID, 10),
			EscapeMdTableCell(move.State),
			EscapeMdTableCell(move.SourceStorageName),
			EscapeMdTableCell(move.DestinationStorageName),
			entity,
			FormatTimeValue(move.CreatedAt),
		))
	}
	WriteListFooter(&b, opts.Pagination, linked)
	return b.String()
}

// FormatStorageMoveCollectionMarkdown maps package-specific storage moves and
// renders them as a shared Markdown list.
func FormatStorageMoveCollectionMarkdown[T any](moves []T, pagination PaginationOutput, convert func(T) StorageMoveMarkdown, title, emptyMessage, entityColumn string) string {
	return formatStorageMoveListMarkdown(mapSlice(moves, convert), storageMoveListMarkdownOptions{
		Title:        title,
		EmptyMessage: emptyMessage,
		EntityColumn: entityColumn,
		Pagination:   pagination,
	})
}

func storageMoveEntityCell(entity StorageMoveEntityMarkdown, includeID bool) string {
	// The name is a group name or a snippet title, and the cell escaper leaves
	// the brackets alone, so a title of "Fix login](http://attacker.invalid/x)"
	// closed the label of the link written around it here. MdTitleLink escapes
	// both halves and returns the bare escaped name when there is no URL,
	// which is the shape this used to build by hand.
	name := MdTitleLink(entity.Name, entity.URL)
	if includeID {
		return fmt.Sprintf("%s (ID: %d)", name, entity.ID)
	}
	return name
}

// CICDVariableMarkdown carries the common fields rendered by GitLab CI/CD
// variable tools at project, group, and instance scopes.
type CICDVariableMarkdown struct {
	Key              string
	Value            string
	VariableType     string
	Protected        bool
	Masked           bool
	Hidden           bool
	Raw              bool
	EnvironmentScope string
	Description      string
}

// CICDVariableFlags groups boolean CI/CD variable attributes for Markdown
// view-model construction.
type CICDVariableFlags struct {
	Protected bool
	Masked    bool
	Hidden    bool
	Raw       bool
}

// NewCICDVariableMarkdown builds a shared Markdown view model for CI/CD
// variables without forcing tool packages to duplicate composite literals.
func NewCICDVariableMarkdown(key, value, variableType string, flags CICDVariableFlags, environmentScope, description string) CICDVariableMarkdown {
	return CICDVariableMarkdown{
		Key:              key,
		Value:            value,
		VariableType:     variableType,
		Protected:        flags.Protected,
		Masked:           flags.Masked,
		Hidden:           flags.Hidden,
		Raw:              flags.Raw,
		EnvironmentScope: environmentScope,
		Description:      description,
	}
}

// cicdVariableMarkdownOptions configures the shared CI/CD variable detail
// renderer.
type cicdVariableMarkdownOptions struct {
	Title                   string
	IncludeEnvironmentScope bool
	Hints                   []string
}

// maskedVariableValue stands in for the value of a masked or hidden variable,
// which GitLab shows nowhere and this server shows nowhere either.
const maskedVariableValue = "[masked]"

// formatCICDVariableMarkdown renders a single CI/CD variable as a card. A
// variable with no key renders nothing: there is no such variable.
func formatCICDVariableMarkdown(v CICDVariableMarkdown, opts cicdVariableMarkdownOptions) string {
	if v.Key == "" {
		return ""
	}
	var b strings.Builder
	c := NewCard(&b, opts.Title+": "+v.Key)
	c.Field("Type", v.VariableType)
	c.Bool("Protected", v.Protected)
	c.Bool("Masked", v.Masked)
	if v.Hidden {
		c.Bool("Hidden", true)
	}
	c.Bool("Raw", v.Raw)
	if opts.IncludeEnvironmentScope {
		c.Field("Environment Scope", v.EnvironmentScope)
	}
	c.Text("Description", v.Description)
	// Masked and hidden are separate GitLab flags and either withholds the
	// value: a hidden variable is never shown again in GitLab's own UI whether
	// or not it is also masked.
	if v.Masked || v.Hidden {
		c.Markdown("Value", maskedVariableValue)
	} else {
		c.Field("Value", v.Value)
	}
	c.End(opts.Hints...)
	return b.String()
}

// FormatCICDVariableDetailMarkdown renders a CI/CD variable with standard
// update/delete next-step hints shared by project and group variables.
func FormatCICDVariableDetailMarkdown(v CICDVariableMarkdown, title string, includeEnvironmentScope bool) string {
	return formatCICDVariableMarkdown(v, cicdVariableMarkdownOptions{
		Title:                   title,
		IncludeEnvironmentScope: includeEnvironmentScope,
		Hints: []string{
			"Use action 'update' to change this variable",
			"Use action 'delete' to remove this variable",
		},
	})
}

// cicdVariableListMarkdownOptions configures the shared CI/CD variable list
// renderer.
type cicdVariableListMarkdownOptions struct {
	Title                   string
	EmptyMessage            string
	IncludeEnvironmentScope bool
	Hints                   []string
}

// formatCICDVariableListMarkdown renders CI/CD variables as a Markdown table.
// The table carries no link, so the footer carries no instruction to keep
// them.
func formatCICDVariableListMarkdown(variables []CICDVariableMarkdown, pagination PaginationOutput, opts cicdVariableListMarkdownOptions) string {
	if len(variables) == 0 {
		return emptyResult(opts.EmptyMessage)
	}
	var b strings.Builder
	WriteListHeading(&b, opts.Title, len(variables), pagination)
	if opts.IncludeEnvironmentScope {
		b.WriteString(MarkdownTableHeader("Key", "Type", "Protected", "Masked", "Scope"))
	} else {
		b.WriteString(MarkdownTableHeader("Key", "Type", "Protected", "Masked"))
	}
	for _, v := range variables {
		cells := []string{
			EscapeMdTableCell(v.Key),
			EscapeMdTableCell(v.VariableType),
			BoolEmoji(v.Protected),
			BoolEmoji(v.Masked),
		}
		if opts.IncludeEnvironmentScope {
			cells = append(cells, EscapeMdTableCell(v.EnvironmentScope))
		}
		b.WriteString(MarkdownTableRow(cells...))
	}
	WriteListFooter(&b, pagination, false, opts.Hints...)
	return b.String()
}

// FormatCICDVariableCollectionMarkdown maps package-specific CI/CD variable
// outputs and renders them as a shared Markdown list.
func FormatCICDVariableCollectionMarkdown[T any](variables []T, pagination PaginationOutput, convert func(T) CICDVariableMarkdown, title, emptyMessage string, includeEnvironmentScope bool, hints ...string) string {
	return formatCICDVariableListMarkdown(mapSlice(variables, convert), pagination, cicdVariableListMarkdownOptions{
		Title:                   title,
		EmptyMessage:            emptyMessage,
		IncludeEnvironmentScope: includeEnvironmentScope,
		Hints:                   hints,
	})
}

// DiscussionNoteMarkdown is the note of a discussion thread, which is a
// GitLab note like any other and is rendered by the same view model: the two
// used to be separate types with renderers that disagreed on the heading, the
// author's spelling and the body's label, and the discussion one carried no
// resolution state, which kept the merge request and commit discussion
// packages from adopting it at all.
type DiscussionNoteMarkdown = NoteMarkdown

// NewDiscussionNoteMarkdown builds the note view model from the four fields a
// discussion note always carries.
func NewDiscussionNoteMarkdown(id int64, body, author, createdAt string) NoteMarkdown {
	return NoteMarkdown{ID: id, Body: body, Author: author, CreatedAt: createdAt}
}

// DiscussionMarkdown carries common discussion fields for Markdown responses.
type DiscussionMarkdown struct {
	ID    string
	Notes []NoteMarkdown
}

// DiscussionRenderer stores stable labels and hints for a discussion family so
// package formatters can avoid repeating identical rendering glue.
type DiscussionRenderer struct {
	ListTitle       string
	EmptyMessage    string
	ListHints       []string
	DiscussionHints []string
	NoteHints       []string
}

// NewDiscussionRenderer builds a renderer for discussion tool families that use
// one hint for each list, discussion, and note view.
func NewDiscussionRenderer(listTitle, emptyMessage, listHint, discussionHint, noteHint string) DiscussionRenderer {
	return DiscussionRenderer{
		ListTitle:       listTitle,
		EmptyMessage:    emptyMessage,
		ListHints:       []string{listHint},
		DiscussionHints: []string{discussionHint},
		NoteHints:       []string{noteHint},
	}
}

// FormatRESTList renders REST discussion threads with offset pagination.
func (r DiscussionRenderer) FormatRESTList(discussions []DiscussionMarkdown, pagination PaginationOutput) string {
	return formatDiscussionListMarkdown(discussions, discussionListMarkdownOptions{
		Title:        r.ListTitle,
		EmptyMessage: r.EmptyMessage,
		Pagination:   pagination,
		Hints:        r.ListHints,
	})
}

// formatGraphQLList renders GraphQL discussion threads with cursor pagination.
func (r DiscussionRenderer) formatGraphQLList(discussions []DiscussionMarkdown, pagination GraphQLPaginationOutput) string {
	return formatDiscussionListMarkdown(discussions, discussionListMarkdownOptions{
		Title:             r.ListTitle,
		EmptyMessage:      r.EmptyMessage,
		GraphQLPagination: &pagination,
		Hints:             r.ListHints,
	})
}

// FormatGraphQLForwardList renders GraphQL discussion threads from a
// connection that only pages forward, so the summary line names no previous
// page.
func (r DiscussionRenderer) FormatGraphQLForwardList(discussions []DiscussionMarkdown, pagination GraphQLForwardPaginationOutput) string {
	return r.formatGraphQLList(discussions, GraphQLPaginationOutput{
		HasNextPage: pagination.HasNextPage,
		EndCursor:   pagination.EndCursor,
	})
}

// FormatDiscussion renders a single discussion using the renderer hints.
func (r DiscussionRenderer) FormatDiscussion(discussion DiscussionMarkdown) string {
	return FormatDiscussionMarkdown(discussion, r.DiscussionHints...)
}

// FormatNote renders a single discussion note using the renderer hints, as
// the note card every note tool renders.
func (r DiscussionRenderer) FormatNote(note NoteMarkdown) string {
	return FormatNoteMarkdown(note, NoteMarkdownOptions{
		Title:             discussionNoteTitle,
		IncludeResolvable: true,
		Hints:             r.NoteHints,
	})
}

// discussionNoteTitle is the heading a discussion note card opens with, the
// same across the REST and GraphQL discussion families.
const discussionNoteTitle = "Discussion Note"

// NewDiscussionMarkdown builds a shared Markdown view model for discussion
// threads.
func NewDiscussionMarkdown(id string, notes []NoteMarkdown) DiscussionMarkdown {
	return DiscussionMarkdown{ID: id, Notes: notes}
}

// DiscussionMarkdowns maps package-specific discussion outputs to the shared
// discussion Markdown view model.
func DiscussionMarkdowns[T any](discussions []T, convert func(T) DiscussionMarkdown) []DiscussionMarkdown {
	return mapSlice(discussions, convert)
}

// discussionListMarkdownOptions configures shared discussion list rendering.
type discussionListMarkdownOptions struct {
	Title             string
	EmptyMessage      string
	Pagination        PaginationOutput
	GraphQLPagination *GraphQLPaginationOutput
	Hints             []string
}

// formatDiscussionListMarkdown renders discussion threads as Markdown: the
// list heading, one H3 per thread with its notes quoted under their authors,
// the pagination the call used, and the hints. The threads carry no link, so
// the footer carries no instruction to keep them.
func formatDiscussionListMarkdown(discussions []DiscussionMarkdown, opts discussionListMarkdownOptions) string {
	if len(discussions) == 0 {
		return emptyResult(opts.EmptyMessage)
	}
	var b strings.Builder
	if opts.GraphQLPagination == nil {
		WriteListHeading(&b, opts.Title, len(discussions), opts.Pagination)
	} else {
		// A keyset connection counts nothing it has not walked, so the heading
		// carries the count shown and the cursor line at the bottom says
		// whether more follow.
		WriteListHeading(&b, opts.Title, len(discussions), PaginationOutput{})
	}
	for _, discussion := range discussions {
		//gitlab:allow-unescaped discussion.ID: a discussion thread id, hexadecimal digits from the REST digest or from the numeric half of a GraphQL global id.
		fmt.Fprintf(&b, "### Discussion %s\n", discussion.ID)
		writeDiscussionNotes(&b, discussion.Notes)
		b.WriteString("\n")
	}
	if opts.GraphQLPagination != nil {
		WriteGraphQLPagination(&b, *opts.GraphQLPagination, len(discussions))
	} else {
		WritePagination(&b, opts.Pagination)
	}
	WriteHints(&b, withoutPreserveLinks(opts.Hints)...)
	return b.String()
}

// FormatRESTDiscussionListMarkdown maps REST discussion outputs and renders
// them with offset pagination metadata.
func FormatRESTDiscussionListMarkdown[T any](discussions []T, pagination PaginationOutput, convert func(T) DiscussionMarkdown, title, emptyMessage string, hints ...string) string {
	return formatDiscussionListMarkdown(mapSlice(discussions, convert), discussionListMarkdownOptions{
		Title:        title,
		EmptyMessage: emptyMessage,
		Pagination:   pagination,
		Hints:        hints,
	})
}

// FormatDiscussionMarkdown renders a single discussion thread as Markdown.
func FormatDiscussionMarkdown(discussion DiscussionMarkdown, hints ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Discussion %s\n\n", discussion.ID)
	writeDiscussionNotes(&b, discussion.Notes)
	WriteHints(&b, hints...)
	return b.String()
}

// writeDiscussionNotes renders each note in a thread as a list item naming
// the author, the time and the note's ID, with the body quoted underneath it.
// The ID is there because it is what every note action takes.
//
// The body used to be interpolated into the item itself, which is the one
// discussion path that did not quote. A note body is written by anybody who
// can comment on the issue or merge request, and printed raw at column 0 it
// could add list items of its own, impersonate a system note, open a heading,
// or forge the server's guidance section.
func writeDiscussionNotes(b *strings.Builder, notes []NoteMarkdown) {
	for _, note := range notes {
		fmt.Fprintf(b, "- **@%s** (%s, note %d):\n", EscapeMdTableCell(note.Author), FormatTime(note.CreatedAt), note.ID)
		quoted := WrapGFMBody(note.Body)
		if quoted == "" {
			continue
		}
		// Indented so the quote belongs to the list item rather than ending
		// the list.
		for line := range strings.SplitSeq(quoted, "\n") {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
}

// TemplateMarkdown carries common fields rendered by template list tools.
type TemplateMarkdown struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// TemplateRenderer stores the stable labels and hints for a GitLab template
// family so package formatters can avoid repeating identical rendering glue.
type TemplateRenderer struct {
	ListTitle    string
	EmptyMessage string
	ListHint     string
	DetailTitle  string
	Language     string
	DetailHint   string
}

// NewTemplateRenderer builds a renderer for a GitLab template family.
func NewTemplateRenderer(listTitle, emptyMessage, listHint, detailTitle, language, detailHint string) TemplateRenderer {
	return TemplateRenderer{
		ListTitle:    listTitle,
		EmptyMessage: emptyMessage,
		ListHint:     listHint,
		DetailTitle:  detailTitle,
		Language:     language,
		DetailHint:   detailHint,
	}
}

// FormatList renders a GitLab template list with the renderer configuration,
// as the two-column form of [FormatTemplateAttributeListMarkdown]: one table
// renderer serves the families with an attribute column and the ones
// without, so they cannot disagree on the heading, the empty form or the
// footer.
func (r TemplateRenderer) FormatList(templates []TemplateMarkdown, pagination PaginationOutput) string {
	items := mapSlice(templates, func(t TemplateMarkdown) TemplateAttributeListMarkdownItem {
		return TemplateAttributeListMarkdownItem{Key: t.Key, Name: t.Name}
	})
	return FormatTemplateAttributeListMarkdown(items, TemplateAttributeListMarkdownOptions{
		Title:        r.ListTitle,
		EmptyMessage: r.EmptyMessage,
		Pagination:   pagination,
		Hints:        []string{r.ListHint},
	})
}

// FormatContent renders a single GitLab template body with the renderer
// configuration.
func (r TemplateRenderer) FormatContent(name, content string) string {
	return FormatTemplateContentMarkdown(r.DetailTitle, name, r.Language, content, r.DetailHint)
}

// FormatTemplateContentMarkdown renders a GitLab template body as a card
// whose only content is the body inside a fenced code block, through
// [Card.Fence], which sizes the fence past the longest backtick run in the
// body. On an instance with custom file templates the set is served out of a
// designated project, so the name is a repository file name somebody chose,
// and the heading is escaped as every card heading is.
func FormatTemplateContentMarkdown(title, name, language, content string, hints ...string) string {
	var b strings.Builder
	c := NewCard(&b, title+": "+name)
	c.Fence("", language, content)
	c.End(hints...)
	return b.String()
}

// MarkdownCodeFence returns a backtick fence long enough to contain content:
// three backticks, or one more than the longest run inside it.
//
// A fixed three-backtick fence is closed by any content that contains one, and
// the content here is a repository file, a job log, a snippet or a diff,
// written by whoever can push a branch or run a pipeline. Everything after the
// run they wrote renders as live Markdown at the top level of the response:
// headings, links, and the server's own guidance section. Sizing the fence to
// the body is the rule Markdown itself uses for nesting code, and it is why
// this helper exists rather than a literal.
func MarkdownCodeFence(content string) string {
	longestRun := 0
	currentRun := 0
	for _, char := range content {
		if char == '`' {
			currentRun++
			longestRun = max(longestRun, currentRun)
			continue
		}
		currentRun = 0
	}
	fenceLength := max(3, longestRun+1)
	return strings.Repeat("`", fenceLength)
}

// WriteDescription writes a GitLab-authored description field.
//
// A description is prose somebody typed into GitLab, and the "- **Description**:
// %s" line it used to be interpolated into puts it at column zero after a list
// bullet: its second line is no longer part of the item, so an embedded heading
// is a heading of the response and an embedded bullet is an item of the
// server's own list. A one-line description keeps the compact form with the
// cell escaping applied and the guidance heading defused, the containment a
// card row has; anything longer becomes a blockquote, which is what the merge
// request, wiki and release renderers already do.
//
// [Card.Text] is the same row on a card, with the quote indented under the
// label so it stays inside the item.
func WriteDescription(b *strings.Builder, description string) {
	if description == "" {
		return
	}
	if !strings.ContainsAny(description, "\n\r") {
		fmt.Fprintf(b, FmtMdDescription, cardInline(description))
		return
	}
	b.WriteString("- **Description**:\n\n")
	b.WriteString(WrapGFMBody(description))
	b.WriteString("\n")
}

// MarkdownFencedBlock renders content as a complete fenced code block: a fence
// sized by [MarkdownCodeFence], the info string, the body, and the matching
// closing fence on its own line. Pass an empty language for a bare fence.
//
// Prefer it over writing a fence by hand wherever the body is GitLab-authored.
func MarkdownFencedBlock(language, content string) string {
	fence := MarkdownCodeFence(content)
	var b strings.Builder
	b.WriteString(fence)
	b.WriteString(sanitizeFenceInfo(language))
	b.WriteString("\n")
	b.WriteString(content)
	if content != "" && !strings.HasSuffix(content, "\n") {
		b.WriteString("\n")
	}
	b.WriteString(fence)
	b.WriteString("\n")
	return b.String()
}

// fenceInfoSanitizer strips the characters that would let an info string end
// its own fence or start a line of its own.
var fenceInfoSanitizer = strings.NewReplacer("`", "", "\r", "", "\n", "")

// sanitizeFenceInfo renders a language name as a code fence's info string.
func sanitizeFenceInfo(language string) string {
	return fenceInfoSanitizer.Replace(StripControlBytes(language))
}

// NoteMarkdown carries the fields every GitLab note renders with, whether it
// stands alone or sits in a discussion thread: issue, merge request, snippet,
// epic and commit notes alike.
type NoteMarkdown struct {
	ID         int64
	Body       string
	Author     string
	CreatedAt  string
	System     bool
	Internal   bool
	Resolvable bool
	Resolved   bool
	ResolvedBy string
}

// NoteMarkdownFlags groups boolean note attributes for Markdown view-model
// construction.
type NoteMarkdownFlags struct {
	System     bool
	Internal   bool
	Resolvable bool
	Resolved   bool
}

// NewNoteMarkdown builds a shared Markdown view model for GitLab notes.
func NewNoteMarkdown(id int64, body, author, createdAt string, flags NoteMarkdownFlags, resolvedBy string) NoteMarkdown {
	return NoteMarkdown{
		ID:         id,
		Body:       body,
		Author:     author,
		CreatedAt:  createdAt,
		System:     flags.System,
		Internal:   flags.Internal,
		Resolvable: flags.Resolvable,
		Resolved:   flags.Resolved,
		ResolvedBy: resolvedBy,
	}
}

// NoteMarkdowns maps package-specific note outputs to the shared note Markdown
// view model.
func NoteMarkdowns[T any](notes []T, convert func(T) NoteMarkdown) []NoteMarkdown {
	return mapSlice(notes, convert)
}

// NoteMarkdownOptions configures shared note detail rendering.
type NoteMarkdownOptions struct {
	Title             string
	IncludeInternal   bool
	IncludeResolvable bool
	Hints             []string
}

// FormatNoteMarkdown renders a single GitLab note as a card: the author as a
// handle, the time, the flags that hold, the resolution state when the domain
// has one, and the body as the card's long text, quoted under its label when
// it spans more than one line.
func FormatNoteMarkdown(note NoteMarkdown, opts NoteMarkdownOptions) string {
	var b strings.Builder
	c := NewCard(&b, fmt.Sprintf("%s #%d", opts.Title, note.ID))
	c.Markdown("Author", MdUserHandle(note.Author))
	c.Time("Created", note.CreatedAt)
	c.Flag("", "System note", note.System)
	c.Flag("", "Internal note", opts.IncludeInternal && note.Internal)
	if opts.IncludeResolvable && note.Resolvable {
		resolved := "unresolved"
		if note.Resolved {
			resolved = "resolved"
		}
		c.Field("Resolvable", resolved)
		c.Markdown("Resolved By", MdUserHandle(note.ResolvedBy))
	}
	c.Text("Body", note.Body)
	c.End(opts.Hints...)
	return b.String()
}

// NoteListMarkdownOptions configures shared note list rendering.
type NoteListMarkdownOptions struct {
	Title           string
	EmptyMessage    string
	IncludeInternal bool
	Hints           []string
}

// FormatNoteListMarkdown renders a list of GitLab notes as a Markdown table.
// The table carries no link, so the footer carries no instruction to keep
// them.
func FormatNoteListMarkdown(notes []NoteMarkdown, pagination PaginationOutput, opts NoteListMarkdownOptions) string {
	if len(notes) == 0 {
		return emptyResult(opts.EmptyMessage)
	}
	var b strings.Builder
	WriteListHeading(&b, opts.Title, len(notes), pagination)
	if opts.IncludeInternal {
		b.WriteString(MarkdownTableHeader("ID", "Author", "Created", "System", "Internal"))
	} else {
		b.WriteString(MarkdownTableHeader("ID", "Author", "Created", "System"))
	}
	for _, note := range notes {
		cells := []string{
			strconv.FormatInt(note.ID, 10),
			EscapeMdTableCell(note.Author),
			FormatTime(note.CreatedAt),
			BoolEmoji(note.System),
		}
		if opts.IncludeInternal {
			cells = append(cells, BoolEmoji(note.Internal))
		}
		b.WriteString(MarkdownTableRow(cells...))
	}
	WriteListFooter(&b, pagination, false, opts.Hints...)
	return b.String()
}

// markdownTableLine writes one pipe-table row, cells separated by " | ".
//
// strings.Join sizes the result from the cells themselves. The hand-rolled
// builder this replaced grew by a guess (eight bytes a cell plus four), which
// no output could tell apart from any other guess: a builder that guesses low
// simply grows again.
func markdownTableLine(cells []string) string {
	return "| " + strings.Join(cells, " | ") + " |\n"
}

// ToolResultWithMarkdown wraps a Markdown string into a CallToolResult
// with a single TextContent entry annotated for assistant-only audience.
// This prevents MCP clients (e.g. VS Code) from displaying raw Markdown
// inline: the LLM processes it and presents formatted output to the user.
//
// Control characters are dropped here as well as in the helpers that build the
// Markdown, because this is the last point every rendered response passes
// through and a formatter that writes a GitLab field straight into its builder
// would otherwise deliver an escape sequence to whatever prints the text. See
// [StripControlBytes].
func ToolResultWithMarkdown(md string) *mcp.CallToolResult {
	if md == "" {
		return nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: StripControlBytes(md), Annotations: ContentAssistant},
		},
	}
}

// ToolResultAnnotated wraps a Markdown string into a CallToolResult with
// content annotations that guide MCP clients on audience and priority.
// Pass nil annotations to get the same behavior as ToolResultWithMarkdown.
func ToolResultAnnotated(md string, ann *mcp.Annotations) *mcp.CallToolResult {
	if md == "" {
		return nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: StripControlBytes(md), Annotations: ann},
		},
	}
}

// ToolResultWithImage creates a CallToolResult containing both a text
// description (metadata) and an ImageContent block with the raw image bytes.
// Multimodal LLMs can "see" the image; text-only LLMs get the metadata.
func ToolResultWithImage(md string, ann *mcp.Annotations, imageData []byte, mimeType string) *mcp.CallToolResult {
	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: StripControlBytes(md), Annotations: ann},
			&mcp.ImageContent{Data: imageData, MIMEType: mimeType},
		},
	}
	return result
}

// MRStateEmoji returns the Markdown emoji for a merge request state.
func MRStateEmoji(state string) string {
	switch state {
	case "opened":
		return EmojiGreen
	case "merged":
		return EmojiPurple
	case "closed":
		return EmojiRed
	default:
		return EmojiQuestion
	}
}

// IssueStateEmoji returns the Markdown emoji for an issue state.
func IssueStateEmoji(state string) string {
	switch state {
	case "opened":
		return EmojiGreen
	case "closed":
		return EmojiRed
	default:
		return EmojiQuestion
	}
}

// pipelineStatusEmojis maps pipeline status strings to their Markdown emoji:
// the states GitLab documents for a pipeline and for a job, including the
// three the repository already advertised without mapping (scheduled,
// preparing, waiting_for_resource) and the two newer ones (canceling,
// waiting_for_callback).
var pipelineStatusEmojis = map[string]string{
	"success":              EmojiSuccess,
	"failed":               EmojiCross,
	"running":              EmojiBlue,
	"pending":              EmojiYellow,
	"canceled":             EmojiStop,
	"cancelled":            EmojiStop,
	"canceling":            EmojiStop,
	"skipped":              EmojiSkip,
	"created":              EmojiNew,
	"manual":               EmojiHand,
	"scheduled":            EmojiCalendar,
	"preparing":            EmojiRefresh,
	"waiting_for_resource": EmojiRefresh,
	"waiting_for_callback": EmojiRefresh,
}

// PipelineStatusEmoji returns the Markdown emoji for a pipeline status.
func PipelineStatusEmoji(status string) string {
	if e, ok := pipelineStatusEmojis[status]; ok {
		return e
	}
	return EmojiQuestion
}

// BoolEmoji returns ✅ for true and ❌ for false.
func BoolEmoji(v bool) string {
	if v {
		return EmojiSuccess
	}
	return EmojiCross
}
