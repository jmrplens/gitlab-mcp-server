// markdown.go provides generic Markdown formatting utilities for MCP tool responses.
// Domain-specific format functions live in their respective domain sub-packages.

package toolutil

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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

// Deprecated: AppendResourceLink is intentionally a no-op. It previously emitted
// mcp.ResourceLink content blocks with external HTTP URLs (GitLab WebURL),
// but ResourceLink is reserved for MCP-registered resources (gitlab:// URIs).
// Clients that received an https:// ResourceLink attempted to resolve it via
// resources/read, triggering JSON-RPC -32002 "Resource not found" errors.
// External web links are already included in the Markdown text output.
// Callers will be removed in a future major version.
func AppendResourceLink(_ *mcp.CallToolResult, _, _, _ string) {}

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
