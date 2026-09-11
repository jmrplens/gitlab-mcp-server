package dependencyfirewall

import (
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Documented outcome values, kept as constants because the markdown formatter
// and its tests both read them and a typo would silently fall through to the
// unknown-outcome branch.
const (
	outcomeAllowed = "allowed"
	outcomeWarned  = "warned"
	outcomeBlocked = "blocked"
)

// notFoundOutput is returned instead of an error when the evaluate endpoint
// answers 404, so the caller gets the feature-flag explanation rather than an
// opaque failure. ProjectID is already rendered for the message.
type notFoundOutput struct {
	ProjectID string
}

// init registers the Markdown formatters for the Dependency Firewall outputs.
func init() {
	toolutil.RegisterMarkdown[EvaluatePackageOutput](FormatEvaluatePackageMarkdown)
	toolutil.RegisterMarkdownResult(formatNotFound)
}

// FormatEvaluatePackageMarkdown renders a Dependency Firewall verdict.
//
// The outcome leads, because it is the answer to the only question the caller
// asked, and the reason follows when GitLab gave one. An allowed outcome is
// reported as "no policy rule matched" rather than "the package is safe": the
// API allows a package that is simply absent from the package metadata
// database, and a formatter that called that safe would be inventing an
// assurance nobody made.
func FormatEvaluatePackageMarkdown(out EvaluatePackageOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Dependency Firewall Evaluation")

	// The three documented outcomes are rendered as composed rows: the glyph
	// and the parenthetical are the server's own words beside a word GitLab
	// chose, and the word is one of the three constants above. An outcome the
	// documentation does not list is written through the escaping row, since
	// it is a value GitLab sent and nothing here constrains it.
	switch strings.ToLower(out.Outcome) {
	case outcomeAllowed:
		c.Markdown("Outcome", toolutil.EmojiSuccess+" **allowed** (no policy rule matched the package)")
	case outcomeWarned:
		c.Markdown("Outcome", toolutil.EmojiWarning+" **warned** (a policy rule matched and the policy is in warn mode)")
	case outcomeBlocked:
		c.Markdown("Outcome", toolutil.EmojiCross+" **blocked** (a policy rule matched and the policy is in enforce mode)")
	case "":
		c.Field("Outcome", "not reported")
	default:
		c.Field("Outcome", out.Outcome)
	}

	if out.Reason != nil {
		c.Field("Reason", *out.Reason)
	}

	switch strings.ToLower(out.Outcome) {
	case outcomeWarned, outcomeBlocked:
		c.End(
			"The reason names the policy that matched. Read it with the project's security policy configuration before overriding anything",
			"An allowed alternative version can be found by evaluating other versions of the same package",
		)
	default:
		c.End(
			"An allowed outcome means no policy rule matched, not that GitLab holds vulnerability or license data for the package",
		)
	}
	return b.String()
}

// formatNotFound explains a 404 from the evaluate endpoint.
//
// The feature flag is named first because it is the likelier cause of the two:
// while dependency_firewall_phase1 is off, every project on the instance
// answers 404, so a caller told only "not found" concludes the project
// reference is wrong and retries with another one forever.
func formatNotFound(out notFoundOutput) *mcp.CallToolResult {
	return toolutil.NotFoundResult(
		"Dependency Firewall", "on "+out.ProjectID,
		"The Dependency Firewall API is served behind the "+FeatureFlag+" feature flag, added in GitLab 19.4 and disabled by default. An instance without it answers 404 for every project, so ask an administrator whether the flag is enabled",
		"The API needs GitLab Premium or Ultimate. It is an experiment, so its shape can change between releases",
		"Verify the project with gitlab_project_get. A project the token cannot read answers 404 here too",
	)
}
