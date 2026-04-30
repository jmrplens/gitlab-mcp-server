// markdown.go provides Markdown formatting functions for CI lint MCP tool output.
package cilint

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a CI lint result as Markdown.
func FormatOutputMarkdown(v Output) string {
	if v.Valid && len(v.Errors) == 0 && len(v.Warnings) == 0 && len(v.Includes) == 0 && v.MergedYaml == "" {
		return fmt.Sprintf("## CI Lint: %s Valid\n\nConfiguration is valid with no errors, warnings, or includes.\n", toolutil.BoolEmoji(true))
	}
	var b strings.Builder
	if v.Valid {
		fmt.Fprintf(&b, "## CI Lint: %s Valid\n\n", toolutil.BoolEmoji(true))
	} else {
		fmt.Fprintf(&b, "## CI Lint: %s Invalid\n\n", toolutil.BoolEmoji(false))
	}
	if len(v.Errors) > 0 {
		b.WriteString("### Errors\n\n")
		for _, e := range v.Errors {
			fmt.Fprintf(&b, "- %s\n", e)
		}
		b.WriteString("\n")
	}
	if len(v.Warnings) > 0 {
		b.WriteString("### Warnings\n\n")
		for _, w := range v.Warnings {
			fmt.Fprintf(&b, "- %s\n", w)
		}
		b.WriteString("\n")
	}
	if len(v.Includes) > 0 {
		b.WriteString("### Includes\n\n")
		b.WriteString("| Type | Location | Context Project |\n")
		b.WriteString("| --- | --- | --- |\n")
		for _, inc := range v.Includes {
			fmt.Fprintf(&b, "| %s | %s | %s |\n",
				inc.Type, toolutil.EscapeMdTableCell(inc.Location), toolutil.EscapeMdTableCell(inc.ContextProject))
		}
		b.WriteString("\n")
	}
	if v.MergedYaml != "" {
		b.WriteString("### Merged YAML\n\n```yaml\n")
		b.WriteString(v.MergedYaml)
		if !strings.HasSuffix(v.MergedYaml, "\n") {
			b.WriteByte('\n')
		}
		b.WriteString("```\n")
	}
	toolutil.WriteHints(&b, "Fix reported errors and warnings before committing your CI configuration")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
}
