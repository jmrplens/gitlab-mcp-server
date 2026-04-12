// issue_report.go provides a helper that formats unrecoverable tool errors
// as pre-filled GitLab/GitHub issue Markdown. The output is copyable text
// that includes tool name, action, sanitized input, error details, HTTP status,
// server version, and timestamp — everything a maintainer needs to reproduce.

package toolutil

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// issueReportsEnabled controls whether FormatIssueReport includes the
// full pre-filled issue body. When false (default), FormatIssueReport
// falls back to ErrorResultMarkdown — the standard Markdown error output.
var issueReportsEnabled bool

// EnableIssueReports activates the automatic issue report generation.
// Call from main() after loading configuration.
func EnableIssueReports(enabled bool) {
	issueReportsEnabled = enabled
}

// IssueReportsEnabled returns whether issue report generation is active.
func IssueReportsEnabled() bool {
	return issueReportsEnabled
}

// serverVersion is read once from the VERSION file at package init.
// Falls back to "unknown" if the file is missing or unreadable.
var serverVersion string

// init reads the server version from the VERSION file at startup.
func init() {
	data, err := os.ReadFile("VERSION")
	if err == nil {
		serverVersion = strings.TrimSpace(string(data))
	}
	if serverVersion == "" {
		serverVersion = "unknown"
	}
}

// SetServerVersion overrides the auto-detected server version.
// Call from main() before registering tools when the VERSION file
// is not in the working directory.
func SetServerVersion(v string) {
	serverVersion = v
}

// IssueReport holds the context needed to generate a bug report body.
type IssueReport struct {
	Tool         string
	Action       string
	ErrorMessage string
	HTTPStatus   int
	RequestID    string
	Input        map[string]any
	Timestamp    time.Time
}

// NewIssueReport creates an IssueReport from a DetailedError plus runtime context.
func NewIssueReport(de *DetailedError, input map[string]any) *IssueReport {
	return &IssueReport{
		Tool:         de.Domain,
		Action:       de.Action,
		ErrorMessage: de.Message,
		HTTPStatus:   de.GitLabStatus,
		RequestID:    de.RequestID,
		Input:        sanitizeInput(input),
		Timestamp:    time.Now().UTC(),
	}
}

// Markdown renders the issue report as a Markdown body suitable for pasting
// into a GitLab or GitHub issue. Secrets are redacted from the input.
func (r *IssueReport) Markdown() string {
	var b strings.Builder

	b.WriteString("## Bug Report — MCP Tool Error\n\n")

	// Environment table
	b.WriteString("### Environment\n\n")
	b.WriteString("| Key | Value |\n")
	b.WriteString("| --- | --- |\n")
	fmt.Fprintf(&b, "| Server version | %s |\n", serverVersion)
	fmt.Fprintf(&b, "| Go version | %s |\n", runtime.Version())
	fmt.Fprintf(&b, "| OS / Arch | %s / %s |\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "| Timestamp (UTC) | %s |\n", r.Timestamp.Format(time.RFC3339))
	b.WriteString("\n")

	// Error details
	b.WriteString("### Error Details\n\n")
	b.WriteString("| Key | Value |\n")
	b.WriteString("| --- | --- |\n")
	fmt.Fprintf(&b, "| Tool | `%s` |\n", r.Tool)
	fmt.Fprintf(&b, "| Action | `%s` |\n", r.Action)
	fmt.Fprintf(&b, "| Error | %s |\n", r.ErrorMessage)
	if r.HTTPStatus > 0 {
		fmt.Fprintf(&b, "| HTTP Status | %d — %s |\n", r.HTTPStatus, ClassifyHTTPStatus(r.HTTPStatus))
	}
	if r.RequestID != "" {
		fmt.Fprintf(&b, "| Request ID | `%s` |\n", r.RequestID)
	}
	b.WriteString("\n")

	// Sanitized input
	if len(r.Input) > 0 {
		b.WriteString("### Input (sanitized)\n\n")
		b.WriteString("```json\n")
		for k, v := range r.Input {
			fmt.Fprintf(&b, "  %q: %v\n", k, v)
		}
		b.WriteString("```\n\n")
	}

	// Reproduction steps
	b.WriteString("### Steps to Reproduce\n\n")
	fmt.Fprintf(&b, "1. Call `%s` with action `%s`\n", r.Tool, r.Action)
	b.WriteString("2. Provide the input parameters shown above\n")
	b.WriteString("3. Observe the error\n\n")

	// Labels suggestion
	b.WriteString("### Suggested Labels\n\n")
	b.WriteString("`bug`, `mcp-tool`, `automated-report`\n")

	return b.String()
}

// Title returns a suggested issue title.
func (r *IssueReport) Title() string {
	return fmt.Sprintf("[Bug] %s/%s: %s", r.Tool, r.Action, truncate(r.ErrorMessage, 80))
}

// sanitizeInput creates a copy of the input map with sensitive fields redacted.
func sanitizeInput(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	safe := make(map[string]any, len(input))
	for k, v := range input {
		if isSensitiveKey(k) {
			safe[k] = "[REDACTED]"
		} else {
			safe[k] = v
		}
	}
	return safe
}

// sensitiveKeys lists substrings that identify fields containing secrets.
var sensitiveKeys = []string{
	"token", "password", "secret", "key", "credential",
	"auth", "cookie", "session", "private",
}

// isSensitiveKey returns true if the key name suggests it holds a secret.
func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, s := range sensitiveKeys {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

// truncate shortens s to maxLen characters, appending "…" if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

// FormatIssueReport creates an MCP tool result containing a pre-filled issue
// report for an unrecoverable error. The result includes both the error and
// the copyable issue body, and has IsError = true.
//
// When issue reports are disabled (default), this falls back to
// ErrorResultMarkdown — the standard Markdown error output without the
// issue template. Enable via ISSUE_REPORTS=true environment variable.
func FormatIssueReport(domain, action string, err error, input map[string]any) *mcp.CallToolResult {
	if !issueReportsEnabled {
		return ErrorResultMarkdown(domain, action, err)
	}

	de := NewDetailedError(domain, action, err)
	report := NewIssueReport(de, input)

	var b strings.Builder
	b.WriteString(de.Markdown())
	b.WriteString("\n---\n\n")
	b.WriteString("### 📋 Copy the following to create an issue:\n\n")
	fmt.Fprintf(&b, "**Title**: %s\n\n", report.Title())
	b.WriteString("<details><summary>Issue Body (click to expand)</summary>\n\n")
	b.WriteString(report.Markdown())
	b.WriteString("\n</details>\n")

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: b.String()},
		},
		IsError: true,
	}
}
