package avatar

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatMarkdown formats the avatar output as markdown.
func FormatMarkdown(out GetOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Avatar\n\n- **URL**: %s\n", out.AvatarURL)
	toolutil.WriteHints(&b, "Use the avatar URL directly in your application")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdown)
}
