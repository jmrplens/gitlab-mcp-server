// Package modelregistry markdown.go provides human-readable Markdown formatters for model registry tools.
package modelregistry

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatDownloadMarkdown formats a downloaded ML model package file as Markdown.
func FormatDownloadMarkdown(o DownloadOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## ML Model Package: %s\n\n", o.Filename)
	sb.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| Project | %s |\n", o.ProjectID)
	fmt.Fprintf(&sb, "| Model Version | %s |\n", o.ModelVersionID)
	fmt.Fprintf(&sb, "| Path | %s |\n", o.Path)
	fmt.Fprintf(&sb, "| Filename | %s |\n", o.Filename)
	fmt.Fprintf(&sb, "| Size | %d bytes |\n", o.SizeBytes)
	sb.WriteString("\n_Content is base64-encoded in the structured JSON output._\n")
	toolutil.WriteHints(&sb,
		"Use `gitlab_package_list` to browse available model packages",
	)
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatDownloadMarkdown) // DownloadOutput
}
