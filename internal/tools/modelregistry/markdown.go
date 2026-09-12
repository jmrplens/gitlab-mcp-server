package modelregistry

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatDownloadMarkdown renders a downloaded ML model package file as the
// card of one object: where it came from, what it is called and how large it
// is, with the note that the bytes themselves travel in the structured
// output.
func FormatDownloadMarkdown(o DownloadOutput) string {
	var sb strings.Builder
	// Every one of these is echoed from the caller's own arguments: the
	// project, the model version, the package path and the file name inside it.
	c := toolutil.NewCard(&sb, downloadHeading(o.Filename))
	c.Field("Project", o.ProjectID)
	c.Field("Model Version", o.ModelVersionID)
	c.Field("Path", o.Path)
	c.Field("Filename", o.Filename)
	c.Field("Size", fmt.Sprintf("%d bytes", o.SizeBytes))
	c.Note("_Content is base64-encoded in the structured JSON output._")
	c.End("Use `gitlab_package_list` to browse available model packages")
	return sb.String()
}

// downloadHeading names the card after the file when GitLab sent a name, and
// after the resource alone when it did not, so the heading never ends in a
// colon with nothing behind it.
func downloadHeading(filename string) string {
	if strings.TrimSpace(filename) == "" {
		return "ML Model Package"
	}
	return "ML Model Package: " + filename
}

func init() {
	toolutil.RegisterMarkdown(FormatDownloadMarkdown) // DownloadOutput
}
