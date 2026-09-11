package projectdiscovery

import (
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatMarkdown renders the resolved project as a card, closing with the two
// spellings of the project_id a following call can use. Both are code spans:
// the path is whatever the namespace holder chose, and a span is what keeps it
// from being read as Markdown of the response.
func FormatMarkdown(out ResolveOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Resolved GitLab Project")
	c.Int("ID", out.ID)
	c.Field("Name", out.Name)
	c.Field("Path", out.PathWithNamespace)
	c.URL(out.WebURL)
	c.Field("Default Branch", out.DefaultBranch)
	c.Text("Description", out.Description)
	c.Field("Visibility", out.Visibility)
	c.Note("Use " + toolutil.MdCodeSpan("project_id: "+strconv.FormatInt(out.ID, 10)) +
		" or " + toolutil.MdCodeSpan(`project_id: "`+out.PathWithNamespace+`"`) +
		" for subsequent operations.")
	c.End(linkHints(out.WebURL,
		toolutil.HintAction("project.get", "verify the project's metadata before repository operations"),
		"Use the project_id in subsequent tool calls to operate on this project",
	)...)
	return b.String()
}

// linkHints leads with the instruction to preserve links only when the card
// carries one: telling the model to keep the clickable links of a card that
// shows none is noise it has to read past.
func linkHints(webURL string, hints ...string) []string {
	if webURL == "" {
		return hints
	}
	return append([]string{toolutil.HintPreserveLinks}, hints...)
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdown)
}
