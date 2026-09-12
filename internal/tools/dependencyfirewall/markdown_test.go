// markdown_test.go contains tests for the Dependency Firewall Markdown
// formatters: the verdict rendering and the feature-flag guidance.
package dependencyfirewall

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// hintsAllowed is the guidance an allowed, empty or unrecognized outcome
// closes with, and hintsMatched the guidance a warned or blocked one does.
const (
	hintsAllowed = "\n---\n💡 **Next steps:**\n" +
		"- An allowed outcome means no policy rule matched, not that GitLab holds vulnerability or license data for the package\n"
	hintsMatched = "\n---\n💡 **Next steps:**\n" +
		"- The reason names the policy that matched. Read it with the project's security policy configuration before overriding anything\n" +
		"- An allowed alternative version can be found by evaluating other versions of the same package\n"
)

// resultText concatenates the text content of a tool result.
func resultText(t *testing.T, content []mcp.Content) string {
	t.Helper()
	var b strings.Builder
	for _, item := range content {
		if text, ok := item.(*mcp.TextContent); ok {
			b.WriteString(text.Text)
		}
	}
	return b.String()
}

// TestFormatEvaluatePackageMarkdown verifies each documented outcome renders
// its own wording, and that an allowed verdict is never described as a
// safety assurance the API did not give.
//
// Every case pins the whole card: the heading, the rows, the blank line and
// the guidance section are one document, and a substring assertion cannot see
// a row that landed somewhere it does not render.
func TestFormatEvaluatePackageMarkdown(t *testing.T) {
	const heading = "## Dependency Firewall Evaluation\n\n"
	tests := []struct {
		name string
		out  EvaluatePackageOutput
		want string
	}{
		{
			name: "allowed",
			out:  EvaluatePackageOutput{Outcome: outcomeAllowed},
			want: heading +
				"- **Outcome**: " + toolutil.EmojiSuccess + " **allowed** (no policy rule matched the package)\n" +
				hintsAllowed,
		},
		{
			name: "warned",
			out:  EvaluatePackageOutput{Outcome: outcomeWarned, Reason: new("license policy 'deny-gpl'")},
			want: heading +
				"- **Outcome**: " + toolutil.EmojiWarning + " **warned** (a policy rule matched and the policy is in warn mode)\n" +
				"- **Reason**: license policy 'deny-gpl'\n" +
				hintsMatched,
		},
		{
			name: "blocked",
			out:  EvaluatePackageOutput{Outcome: outcomeBlocked, Reason: new("Package 'lodash' violates 'deny-mit' policy")},
			want: heading +
				"- **Outcome**: " + toolutil.EmojiCross + " **blocked** (a policy rule matched and the policy is in enforce mode)\n" +
				"- **Reason**: Package 'lodash' violates 'deny-mit' policy\n" +
				hintsMatched,
		},
		{
			name: "empty outcome",
			out:  EvaluatePackageOutput{},
			want: heading + "- **Outcome**: not reported\n" + hintsAllowed,
		},
		{
			name: "unknown outcome is passed through",
			out:  EvaluatePackageOutput{Outcome: "quarantined"},
			want: heading + "- **Outcome**: quarantined\n" + hintsAllowed,
		},
		{
			name: "blank reason is omitted",
			out:  EvaluatePackageOutput{Outcome: outcomeAllowed, Reason: new("   ")},
			want: heading +
				"- **Outcome**: " + toolutil.EmojiSuccess + " **allowed** (no policy rule matched the package)\n" +
				hintsAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatEvaluatePackageMarkdown(tt.out); got != tt.want {
				t.Errorf("FormatEvaluatePackageMarkdown() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

// TestFormatEvaluatePackageMarkdown_EscapesReason verifies a reason carrying
// table syntax or newlines cannot break the rendered document: the pipe
// becomes its entity and the line break collapses to a space, so the reason
// stays one list item.
func TestFormatEvaluatePackageMarkdown_EscapesReason(t *testing.T) {
	got := FormatEvaluatePackageMarkdown(EvaluatePackageOutput{
		Outcome: outcomeBlocked,
		Reason:  new("policy | with pipes\nand a newline"),
	})
	want := "## Dependency Firewall Evaluation\n\n" +
		"- **Outcome**: " + toolutil.EmojiCross + " **blocked** (a policy rule matched and the policy is in enforce mode)\n" +
		"- **Reason**: policy &#124; with pipes and a newline\n" +
		hintsMatched
	if got != want {
		t.Errorf("FormatEvaluatePackageMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatEvaluatePackageMarkdown_RegisteredForOutput verifies the formatter
// is reachable through the type registry, which is how every surface renders
// the result.
func TestFormatEvaluatePackageMarkdown_RegisteredForOutput(t *testing.T) {
	result := toolutil.MarkdownForResult(EvaluatePackageOutput{Outcome: outcomeAllowed})
	if result == nil {
		t.Fatal("MarkdownForResult() = nil, want the registered formatter")
	}
	if got, want := resultText(t, result.Content), FormatEvaluatePackageMarkdown(EvaluatePackageOutput{Outcome: outcomeAllowed}); got != want {
		t.Errorf("registered formatter produced %q, want %q", got, want)
	}
}

// TestFormatNotFound verifies the 404 guidance names the feature flag, the
// tier, the project, and the tool that checks the project reference.
func TestFormatNotFound(t *testing.T) {
	result := formatNotFound(notFoundOutput{ProjectID: "project group/app"})
	if result == nil || !result.IsError {
		t.Fatalf("formatNotFound() = %#v, want an informational error result", result)
	}
	text := resultText(t, result.Content)
	for _, want := range []string{
		"Dependency Firewall",
		"project group/app",
		FeatureFlag,
		"19.4",
		"Premium or Ultimate",
		"gitlab_project_get",
	} {
		t.Run("mentions/"+want, func(t *testing.T) {
			if !strings.Contains(text, want) {
				t.Errorf("guidance is missing %q:\n%s", want, text)
			}
		})
	}
}
