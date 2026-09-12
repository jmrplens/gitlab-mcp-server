package metadata

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatGetMarkdown renders the instance metadata as the card of one object,
// with the Kubernetes Agent Server as a nested object of its own: its version
// and its two addresses belong to KAS rather than to GitLab, and reading
// "Version" twice in one flat list said otherwise.
func FormatGetMarkdown(out GetOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "GitLab Metadata")
	c.Field("Version", out.Version)
	c.Field("Revision", out.Revision)
	c.Bool("Enterprise", out.Enterprise)
	kas := c.Sub("KAS")
	kas.Bool("Enabled", out.KAS.Enabled)
	kas.Field("Version", out.KAS.Version)
	kas.Link("External URL", out.KAS.ExternalURL, out.KAS.ExternalURL)
	// The proxy address is what a kubectl configuration points at, and the card
	// used to drop it although the endpoint sends it and this server publishes
	// it.
	kas.Link("Kubernetes Proxy URL", out.KAS.ExternalK8SProxyURL, out.KAS.ExternalK8SProxyURL)
	c.End("Use version information to verify API compatibility")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatGetMarkdown)
}
