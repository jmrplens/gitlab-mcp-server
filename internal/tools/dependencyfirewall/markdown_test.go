// markdown_test.go contains tests for the Dependency Firewall Markdown
// formatters: the verdict rendering and the feature-flag guidance.
package dependencyfirewall

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
func TestFormatEvaluatePackageMarkdown(t *testing.T) {
	tests := []struct {
		name        string
		out         EvaluatePackageOutput
		wantAll     []string
		wantMissing []string
	}{
		{
			name:    "allowed",
			out:     EvaluatePackageOutput{Outcome: outcomeAllowed},
			wantAll: []string{"allowed", "no policy rule matched", "not that GitLab holds vulnerability or license data"},
		},
		{
			name:    "warned",
			out:     EvaluatePackageOutput{Outcome: outcomeWarned, Reason: new("license policy 'deny-gpl'")},
			wantAll: []string{"warned", "warn mode", "license policy 'deny-gpl'", "names the policy that matched"},
		},
		{
			name:    "blocked",
			out:     EvaluatePackageOutput{Outcome: outcomeBlocked, Reason: new("Package 'lodash' violates 'deny-mit' policy")},
			wantAll: []string{"blocked", "enforce mode", "deny-mit"},
		},
		{
			name:        "empty outcome",
			out:         EvaluatePackageOutput{},
			wantAll:     []string{"not reported"},
			wantMissing: []string{"Reason:"},
		},
		{
			name:    "unknown outcome is passed through",
			out:     EvaluatePackageOutput{Outcome: "quarantined"},
			wantAll: []string{"quarantined"},
		},
		{
			name:        "blank reason is omitted",
			out:         EvaluatePackageOutput{Outcome: outcomeAllowed, Reason: new("   ")},
			wantMissing: []string{"Reason:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := FormatEvaluatePackageMarkdown(tt.out)
			if !strings.HasPrefix(md, "## Dependency Firewall Evaluation") {
				t.Errorf("markdown does not start with the heading:\n%s", md)
			}
			for _, want := range tt.wantAll {
				if !strings.Contains(md, want) {
					t.Errorf("markdown is missing %q:\n%s", want, md)
				}
			}
			for _, unwanted := range tt.wantMissing {
				if strings.Contains(md, unwanted) {
					t.Errorf("markdown unexpectedly contains %q:\n%s", unwanted, md)
				}
			}
		})
	}
}

// TestFormatEvaluatePackageMarkdown_EscapesReason verifies a reason carrying
// table syntax or newlines cannot break the rendered document.
func TestFormatEvaluatePackageMarkdown_EscapesReason(t *testing.T) {
	md := FormatEvaluatePackageMarkdown(EvaluatePackageOutput{
		Outcome: outcomeBlocked,
		Reason:  new("policy | with pipes\nand a newline"),
	})
	if strings.Contains(md, "policy | with pipes") {
		t.Errorf("pipe was not escaped:\n%s", md)
	}
	if strings.Contains(md, "pipes\nand") {
		t.Errorf("newline was not flattened:\n%s", md)
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
	if text := resultText(t, result.Content); !strings.Contains(text, "Dependency Firewall Evaluation") {
		t.Errorf("registered formatter produced %q", text)
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
