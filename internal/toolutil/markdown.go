package toolutil

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Common date format constants used across tool sub-packages.
const (
	DateFormatISO  = "2006-01-02"
	DateTimeFormat = "2006-01-02T15:04:05Z"
)

// TblFieldValue is the standard "| Field | Value |" detail table header+separator.
const TblFieldValue = "| Field | Value |\n| --- | --- |\n"

// Table row format constants for common detail-table fields. Each pairs with
// TblFieldValue and is shared across sub-packages to avoid duplicated literals.
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
	FmtMdURL         = "- **URL**: [%[1]s](%[1]s)\n"
	FmtMdURLNewline  = "\n- **URL**: [%[1]s](%[1]s)\n"
	FmtMdAuthorAt    = "- **Author**: @%s\n"
	FmtMdAuthor      = "- **Author**: %s\n"
	FmtMdSectionText = "\n%s\n"
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
const (
	EmojiDraft        = "\U0001F4DD"   // 📝
	EmojiWarning      = "\u26A0\uFE0F" // ⚠️
	EmojiConfidential = "\U0001F512"   // 🔒
	EmojiArchived     = "\U0001F4E6"   // 📦
	EmojiStar         = "\u2B50"       // ⭐
	EmojiSuccess      = "\u2705"       // ✅
	EmojiCross        = "\u274C"       // ❌
	EmojiRefresh      = "\U0001F504"   // 🔄
	EmojiFile         = "\U0001F4C4"   // 📄
	EmojiFolder       = "\U0001F4C1"   // 📁
	EmojiCalendar     = "\U0001F4C5"   // 📅
	EmojiUpArrow      = "\u2B06\uFE0F" // ⬆️
	EmojiDownArrow    = "\u2B07\uFE0F" // ⬇️
	EmojiInfo         = "\u2139\uFE0F" // ℹ️
	EmojiQuestion     = "\u2753"       // ❓
	EmojiLink         = "\U0001F517"   // 🔗
	EmojiUser         = "\U0001F464"   // 👤
	EmojiGroup        = "\U0001F465"   // 👥
	EmojiPipeline     = "\U0001F6A7"   // 🚧
	EmojiMergeRequest = "\U0001F5C3"   // 🗃️
	EmojiIssue        = "\U0001F4A1"   // 💡
	EmojiRed          = "\U0001F534"   // 🔴
	EmojiYellow       = "\U0001F7E1"   // 🟡
	EmojiGreen        = "\U0001F7E2"   // 🟢
	EmojiProhibited   = "\U0001F6AB"   // 🚫
	EmojiWhiteCircle  = "\u26AA"       // ⚪
	EmojiParty        = "\U0001F389"   // 🎉
	EmojiPurple       = "\U0001F7E3"   // 🟣
	EmojiBlue         = "\U0001F535"   // 🔵
	EmojiStop         = "\u26D4"       // ⛔
	EmojiSkip         = "\u23ED\uFE0F" // ⏭️
	EmojiNew          = "\U0001F195"   // 🆕
	EmojiHand         = "\u270B"       // ✋
)

// WritePagination appends a newline-wrapped pagination summary to the builder.
func WritePagination(b *strings.Builder, p PaginationOutput) {
	fmt.Fprintf(b, FmtMdSectionText, FormatPagination(p))
}

// WriteListSummary appends a brief "Showing N of M results (page X of Y)"
// line between the heading and the table body. It is a no-op when there is
// only a single page, because the heading count already conveys everything.
func WriteListSummary(b *strings.Builder, shown int, p PaginationOutput) {
	if p.TotalPages <= 1 {
		return
	}
	fmt.Fprintf(b, "Showing %d of %d results (page %d of %d)\n\n", shown, p.TotalItems, p.Page, p.TotalPages)
}

// MarkdownTableHeader returns a Markdown table header followed by a standard
// separator row for the supplied column labels.
func MarkdownTableHeader(columns ...string) string {
	if len(columns) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(markdownTableLine(columns))
	b.WriteString(MarkdownTableSeparator(len(columns)))
	return b.String()
}

// MarkdownTableSeparator returns a standard left-aligned Markdown separator row
// for the requested number of columns.
func MarkdownTableSeparator(columns int) string {
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

// NewCICDVariableMarkdown builds a shared Markdown view model for CI/CD
// variables without forcing tool packages to duplicate composite literals.
func NewCICDVariableMarkdown(key, value, variableType string, protected, masked, hidden, raw bool, environmentScope, description string) CICDVariableMarkdown {
	return CICDVariableMarkdown{
		Key:              key,
		Value:            value,
		VariableType:     variableType,
		Protected:        protected,
		Masked:           masked,
		Hidden:           hidden,
		Raw:              raw,
		EnvironmentScope: environmentScope,
		Description:      description,
	}
}

// CICDVariableMarkdowns maps package-specific variable outputs to the shared
// CI/CD variable Markdown view model.
func CICDVariableMarkdowns[T any](variables []T, convert func(T) CICDVariableMarkdown) []CICDVariableMarkdown {
	out := make([]CICDVariableMarkdown, 0, len(variables))
	for _, variable := range variables {
		out = append(out, convert(variable))
	}
	return out
}

// CICDVariableMarkdownOptions configures the shared CI/CD variable detail
// renderer.
type CICDVariableMarkdownOptions struct {
	Title                   string
	IncludeEnvironmentScope bool
	Hints                   []string
}

// FormatCICDVariableMarkdown renders a single CI/CD variable as a Markdown
// detail table.
func FormatCICDVariableMarkdown(v CICDVariableMarkdown, opts CICDVariableMarkdownOptions) string {
	if v.Key == "" {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## %s: %s\n\n", opts.Title, v.Key)
	b.WriteString(TblFieldValue)
	fmt.Fprintf(&b, "| Type | %s |\n", EscapeMdTableCell(v.VariableType))
	fmt.Fprintf(&b, "| Protected | %s |\n", BoolEmoji(v.Protected))
	fmt.Fprintf(&b, "| Masked | %s |\n", BoolEmoji(v.Masked))
	if v.Hidden {
		fmt.Fprintf(&b, "| Hidden | %s |\n", BoolEmoji(true))
	}
	fmt.Fprintf(&b, "| Raw | %s |\n", BoolEmoji(v.Raw))
	if opts.IncludeEnvironmentScope {
		fmt.Fprintf(&b, "| Environment Scope | %s |\n", EscapeMdTableCell(v.EnvironmentScope))
	}
	if v.Description != "" {
		fmt.Fprintf(&b, "| Description | %s |\n", EscapeMdTableCell(v.Description))
	}
	if !v.Masked && !v.Hidden {
		fmt.Fprintf(&b, "| Value | %s |\n", EscapeMdTableCell(v.Value))
	} else {
		b.WriteString("| Value | [masked] |\n")
	}
	WriteHints(&b, opts.Hints...)
	return b.String()
}

// FormatCICDVariableDetailMarkdown renders a CI/CD variable with standard
// update/delete next-step hints shared by project and group variables.
func FormatCICDVariableDetailMarkdown(v CICDVariableMarkdown, title string, includeEnvironmentScope bool) string {
	return FormatCICDVariableMarkdown(v, CICDVariableMarkdownOptions{
		Title:                   title,
		IncludeEnvironmentScope: includeEnvironmentScope,
		Hints: []string{
			"Use action 'update' to change this variable",
			"Use action 'delete' to remove this variable",
		},
	})
}

// CICDVariableListMarkdownOptions configures the shared CI/CD variable list
// renderer.
type CICDVariableListMarkdownOptions struct {
	Title                   string
	EmptyMessage            string
	IncludeEnvironmentScope bool
	Hints                   []string
}

// FormatCICDVariableListMarkdown renders CI/CD variables as a Markdown table.
func FormatCICDVariableListMarkdown(variables []CICDVariableMarkdown, pagination PaginationOutput, opts CICDVariableListMarkdownOptions) string {
	if len(variables) == 0 {
		return opts.EmptyMessage
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## %s (%d)\n\n", opts.Title, pagination.TotalItems)
	WriteListSummary(&b, len(variables), pagination)
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
	WritePagination(&b, pagination)
	WriteHints(&b, ListHints(opts.Hints...)...)
	return b.String()
}

// FormatCICDVariableCollectionMarkdown maps package-specific CI/CD variable
// outputs and renders them as a shared Markdown list.
func FormatCICDVariableCollectionMarkdown[T any](variables []T, pagination PaginationOutput, convert func(T) CICDVariableMarkdown, title, emptyMessage string, includeEnvironmentScope bool, hints ...string) string {
	return FormatCICDVariableListMarkdown(CICDVariableMarkdowns(variables, convert), pagination, CICDVariableListMarkdownOptions{
		Title:                   title,
		EmptyMessage:            emptyMessage,
		IncludeEnvironmentScope: includeEnvironmentScope,
		Hints:                   hints,
	})
}

// DiscussionNoteMarkdown carries common note fields rendered inside discussion
// Markdown responses.
type DiscussionNoteMarkdown struct {
	ID        int64
	Body      string
	Author    string
	CreatedAt string
}

// NewDiscussionNoteMarkdown builds a shared Markdown view model for discussion
// notes.
func NewDiscussionNoteMarkdown(id int64, body, author, createdAt string) DiscussionNoteMarkdown {
	return DiscussionNoteMarkdown{ID: id, Body: body, Author: author, CreatedAt: createdAt}
}

// DiscussionNoteMarkdowns maps package-specific note outputs to the shared
// discussion note Markdown view model.
func DiscussionNoteMarkdowns[T any](notes []T, convert func(T) DiscussionNoteMarkdown) []DiscussionNoteMarkdown {
	out := make([]DiscussionNoteMarkdown, 0, len(notes))
	for _, note := range notes {
		out = append(out, convert(note))
	}
	return out
}

// DiscussionMarkdown carries common discussion fields for Markdown responses.
type DiscussionMarkdown struct {
	ID    string
	Notes []DiscussionNoteMarkdown
}

// NewDiscussionMarkdown builds a shared Markdown view model for discussion
// threads.
func NewDiscussionMarkdown(id string, notes []DiscussionNoteMarkdown) DiscussionMarkdown {
	return DiscussionMarkdown{ID: id, Notes: notes}
}

// DiscussionMarkdowns maps package-specific discussion outputs to the shared
// discussion Markdown view model.
func DiscussionMarkdowns[T any](discussions []T, convert func(T) DiscussionMarkdown) []DiscussionMarkdown {
	out := make([]DiscussionMarkdown, 0, len(discussions))
	for _, discussion := range discussions {
		out = append(out, convert(discussion))
	}
	return out
}

// DiscussionListMarkdownOptions configures shared discussion list rendering.
type DiscussionListMarkdownOptions struct {
	Title             string
	EmptyMessage      string
	Pagination        PaginationOutput
	GraphQLPagination *GraphQLPaginationOutput
	Hints             []string
}

// FormatDiscussionListMarkdown renders discussion threads as Markdown.
func FormatDiscussionListMarkdown(discussions []DiscussionMarkdown, opts DiscussionListMarkdownOptions) string {
	if len(discussions) == 0 {
		return opts.EmptyMessage
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## %s (%d)\n\n", opts.Title, len(discussions))
	if opts.GraphQLPagination == nil {
		WriteListSummary(&b, len(discussions), opts.Pagination)
	}
	for _, discussion := range discussions {
		fmt.Fprintf(&b, "### Discussion %s\n", discussion.ID)
		writeDiscussionNotes(&b, discussion.Notes)
		b.WriteString("\n")
	}
	if opts.GraphQLPagination != nil {
		b.WriteString(FormatGraphQLPagination(*opts.GraphQLPagination, len(discussions)))
		b.WriteString("\n")
	} else {
		WritePagination(&b, opts.Pagination)
	}
	WriteHints(&b, ListHints(opts.Hints...)...)
	return b.String()
}

// FormatRESTDiscussionListMarkdown maps REST discussion outputs and renders
// them with offset pagination metadata.
func FormatRESTDiscussionListMarkdown[T any](discussions []T, pagination PaginationOutput, convert func(T) DiscussionMarkdown, title, emptyMessage string, hints ...string) string {
	return FormatDiscussionListMarkdown(DiscussionMarkdowns(discussions, convert), DiscussionListMarkdownOptions{
		Title:        title,
		EmptyMessage: emptyMessage,
		Pagination:   pagination,
		Hints:        hints,
	})
}

// FormatGraphQLDiscussionListMarkdown maps GraphQL discussion outputs and
// renders them with cursor pagination metadata.
func FormatGraphQLDiscussionListMarkdown[T any](discussions []T, pagination GraphQLPaginationOutput, convert func(T) DiscussionMarkdown, title, emptyMessage string, hints ...string) string {
	return FormatDiscussionListMarkdown(DiscussionMarkdowns(discussions, convert), DiscussionListMarkdownOptions{
		Title:             title,
		EmptyMessage:      emptyMessage,
		GraphQLPagination: &pagination,
		Hints:             hints,
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

// FormatDiscussionNoteMarkdown renders a single discussion note as Markdown.
func FormatDiscussionNoteMarkdown(note DiscussionNoteMarkdown, hints ...string) string {
	var b strings.Builder
	b.WriteString("## Note\n\n")
	fmt.Fprintf(&b, FmtMdID, note.ID)
	fmt.Fprintf(&b, FmtMdAuthorAt, note.Author)
	b.WriteString("- **Body**:\n\n")
	b.WriteString(WrapGFMBody(note.Body))
	b.WriteString("\n")
	if note.CreatedAt != "" {
		fmt.Fprintf(&b, FmtMdCreated, FormatTime(note.CreatedAt))
	}
	WriteHints(&b, hints...)
	return b.String()
}

func writeDiscussionNotes(b *strings.Builder, notes []DiscussionNoteMarkdown) {
	for _, note := range notes {
		fmt.Fprintf(b, "- **@%s** (%s): %s\n", note.Author, FormatTime(note.CreatedAt), note.Body)
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

// FormatList renders a GitLab template list with the renderer configuration.
func (r TemplateRenderer) FormatList(templates []TemplateMarkdown, pagination PaginationOutput) string {
	return FormatTemplateListMarkdown(templates, pagination, TemplateListMarkdownOptions{
		Title:        r.ListTitle,
		EmptyMessage: r.EmptyMessage,
		Hints:        []string{r.ListHint},
	})
}

// FormatContent renders a single GitLab template body with the renderer
// configuration.
func (r TemplateRenderer) FormatContent(name, content string) string {
	return FormatTemplateContentMarkdown(r.DetailTitle, name, r.Language, content, r.DetailHint)
}

// NewTemplateMarkdown builds a shared Markdown view model for GitLab template
// list entries.
func NewTemplateMarkdown(key, name string) TemplateMarkdown {
	return TemplateMarkdown{Key: key, Name: name}
}

// TemplateMarkdowns maps package-specific template outputs to the shared
// template Markdown view model.
func TemplateMarkdowns[T any](templates []T, convert func(T) TemplateMarkdown) []TemplateMarkdown {
	out := make([]TemplateMarkdown, 0, len(templates))
	for _, template := range templates {
		out = append(out, convert(template))
	}
	return out
}

// FormatTemplateCollectionMarkdown maps package-specific template outputs and
// renders them as a shared Markdown list.
func FormatTemplateCollectionMarkdown[T any](templates []T, pagination PaginationOutput, convert func(T) TemplateMarkdown, title, emptyMessage string, hints ...string) string {
	return FormatTemplateListMarkdown(TemplateMarkdowns(templates, convert), pagination, TemplateListMarkdownOptions{
		Title:        title,
		EmptyMessage: emptyMessage,
		Hints:        hints,
	})
}

// TemplateListMarkdownOptions configures shared template list rendering.
type TemplateListMarkdownOptions struct {
	Title        string
	EmptyMessage string
	Hints        []string
}

// FormatTemplateListMarkdown renders GitLab template list entries as Markdown.
func FormatTemplateListMarkdown(templates []TemplateMarkdown, pagination PaginationOutput, opts TemplateListMarkdownOptions) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n", opts.Title)
	WriteListSummary(&b, len(templates), pagination)
	if len(templates) == 0 {
		b.WriteString(opts.EmptyMessage)
		return b.String()
	}
	b.WriteString(MarkdownTableHeader("Key", "Name"))
	for _, template := range templates {
		b.WriteString(MarkdownTableRow(EscapeMdTableCell(template.Key), EscapeMdTableCell(template.Name)))
	}
	WritePagination(&b, pagination)
	WriteHints(&b, ListHints(opts.Hints...)...)
	return b.String()
}

// FormatTemplateContentMarkdown renders a GitLab template body inside a fenced
// code block.
func FormatTemplateContentMarkdown(title, name, language, content string, hints ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s: %s\n\n", title, name)
	fence := "```"
	if strings.Contains(content, "```") {
		fence = "````"
	}
	safeLanguage := strings.NewReplacer("`", "", "\r", "", "\n", "").Replace(language)
	fmt.Fprintf(&b, "%s%s\n", fence, safeLanguage)
	b.WriteString(content)
	fmt.Fprintf(&b, "\n%s\n", fence)
	WriteHints(&b, hints...)
	return b.String()
}

// NoteMarkdown carries common fields rendered by issue, merge request, and
// snippet note tools.
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

// NewNoteMarkdown builds a shared Markdown view model for GitLab notes.
func NewNoteMarkdown(id int64, body, author, createdAt string, system, internal, resolvable, resolved bool, resolvedBy string) NoteMarkdown {
	return NoteMarkdown{
		ID:         id,
		Body:       body,
		Author:     author,
		CreatedAt:  createdAt,
		System:     system,
		Internal:   internal,
		Resolvable: resolvable,
		Resolved:   resolved,
		ResolvedBy: resolvedBy,
	}
}

// NoteMarkdowns maps package-specific note outputs to the shared note Markdown
// view model.
func NoteMarkdowns[T any](notes []T, convert func(T) NoteMarkdown) []NoteMarkdown {
	out := make([]NoteMarkdown, 0, len(notes))
	for _, note := range notes {
		out = append(out, convert(note))
	}
	return out
}

// NoteMarkdownOptions configures shared note detail rendering.
type NoteMarkdownOptions struct {
	Title             string
	IncludeInternal   bool
	IncludeResolvable bool
	Hints             []string
}

// FormatNoteMarkdown renders a single GitLab note as Markdown.
func FormatNoteMarkdown(note NoteMarkdown, opts NoteMarkdownOptions) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s #%d\n\n", opts.Title, note.ID)
	fmt.Fprintf(&b, FmtMdAuthor, note.Author)
	fmt.Fprintf(&b, FmtMdCreated, FormatTime(note.CreatedAt))
	if note.System {
		b.WriteString("- **System note**\n")
	}
	if opts.IncludeInternal && note.Internal {
		b.WriteString("- **Internal note**\n")
	}
	if opts.IncludeResolvable && note.Resolvable {
		resolved := "unresolved"
		if note.Resolved {
			resolved = "resolved"
		}
		fmt.Fprintf(&b, "- **Resolvable**: %s\n", resolved)
		if note.ResolvedBy != "" {
			fmt.Fprintf(&b, "- **Resolved By**: @%s\n", note.ResolvedBy)
		}
	}
	fmt.Fprintf(&b, "\n%s\n", WrapGFMBody(note.Body))
	WriteHints(&b, opts.Hints...)
	return b.String()
}

// NoteListMarkdownOptions configures shared note list rendering.
type NoteListMarkdownOptions struct {
	Title           string
	EmptyMessage    string
	IncludeInternal bool
	Hints           []string
}

// FormatNoteListMarkdown renders a list of GitLab notes as Markdown.
func FormatNoteListMarkdown(notes []NoteMarkdown, pagination PaginationOutput, opts NoteListMarkdownOptions) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s (%d)\n\n", opts.Title, pagination.TotalItems)
	WriteListSummary(&b, len(notes), pagination)
	if len(notes) == 0 {
		b.WriteString(opts.EmptyMessage)
		return b.String()
	}
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
	WritePagination(&b, pagination)
	WriteHints(&b, ListHints(opts.Hints...)...)
	return b.String()
}

func markdownTableLine(cells []string) string {
	var b strings.Builder
	b.Grow(len(cells)*8 + 4)
	b.WriteString("| ")
	for i, cell := range cells {
		if i > 0 {
			b.WriteString(" | ")
		}
		b.WriteString(cell)
	}
	b.WriteString(" |\n")
	return b.String()
}

// ToolResultWithMarkdown wraps a Markdown string into a CallToolResult
// with a single TextContent entry annotated for assistant-only audience.
// This prevents MCP clients (e.g. VS Code) from displaying raw Markdown
// inline — the LLM processes it and presents formatted output to the user.
func ToolResultWithMarkdown(md string) *mcp.CallToolResult {
	if md == "" {
		return nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: md, Annotations: ContentAssistant},
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
			&mcp.TextContent{Text: md, Annotations: ann},
		},
	}
}

// ToolResultWithImage creates a CallToolResult containing both a text
// description (metadata) and an ImageContent block with the raw image bytes.
// Multimodal LLMs can "see" the image; text-only LLMs get the metadata.
func ToolResultWithImage(md string, ann *mcp.Annotations, imageData []byte, mimeType string) *mcp.CallToolResult {
	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: md, Annotations: ann},
			&mcp.ImageContent{Data: imageData, MIMEType: mimeType},
		},
	}
	return result
}

// AppendResourceLink preserves the legacy resource-link hook as a no-op.
//
// Deprecated: AppendResourceLink is intentionally a no-op. It previously emitted
// mcp.ResourceLink content blocks with external HTTP URLs (GitLab WebURL),
// but ResourceLink is reserved for MCP-registered resources (gitlab:// URIs).
// Clients that received an https:// ResourceLink attempted to resolve it via
// resources/read, triggering JSON-RPC -32002 "Resource not found" errors.
// External web links are already included in the Markdown text output.
// Callers will be removed in a future major version.
func AppendResourceLink(_ *mcp.CallToolResult, _, _, _ string) {
	// Intentional no-op: see deprecation notice above.
}

// FormatPagination renders pagination metadata as a compact Markdown line.
func FormatPagination(p PaginationOutput) string {
	return fmt.Sprintf("Page %d of %d | %d items total | %d per page",
		p.Page, p.TotalPages, p.TotalItems, p.PerPage)
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

// WriteEmpty writes a standardized empty-result message to the builder.
// The resource parameter should be a clear, specific plural noun
// (e.g. "merge requests", "pipeline variables", "protected branches").
func WriteEmpty(b *strings.Builder, resource string) {
	fmt.Fprintf(b, "No %s found.\n", resource)
}

// pipelineStatusEmojis maps pipeline status strings to their Markdown emoji.
var pipelineStatusEmojis = map[string]string{
	"success":   EmojiSuccess,
	"failed":    EmojiCross,
	"running":   EmojiBlue,
	"pending":   EmojiYellow,
	"canceled":  EmojiStop,
	"cancelled": EmojiStop,
	"skipped":   EmojiSkip,
	"created":   EmojiNew,
	"manual":    EmojiHand,
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
