// markdown_test.go contains unit tests for security settings Markdown
// formatting functions.
package securitysettings

import (
	"strings"
	"testing"
)

// projectHints is the guidance section both project renders close with.
const projectHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'project.security_settings_update' to turn secret push protection on or off for this project\n" +
	"- Use action 'group.security_settings_update' to set it for every project in a group at once\n"

// groupHints is the guidance section every group render closes with, preceded
// by the sentence that keeps the flag from reading as a per-project outcome.
const groupHints = "\n" + groupScopeNote + "\n\n---\n💡 **Next steps:**\n" +
	"- Use action 'group.security_settings_update' to change the setting for the group again, excluding the projects that failed\n" +
	"- Use action 'project.security_settings_get' to read one project's settings to see what it carries now\n"

// TestFormatProjectMarkdown_AllFields validates the Markdown renderer for
// project security settings when all fields are populated, including the two
// timestamps. The whole render is compared: a substring assertion cannot see a
// row that landed outside the block it was meant for, which is the defect
// class this migration closes.
func TestFormatProjectMarkdown_AllFields(t *testing.T) {
	out := ProjectOutput{
		ProjectID:                           42,
		CreatedAt:                           "2026-01-01T00:00:00Z",
		UpdatedAt:                           "2026-01-02T09:30:00Z",
		AutoFixContainerScanning:            true,
		AutoFixDAST:                         false,
		AutoFixDependencyScanning:           true,
		AutoFixSAST:                         false,
		ContinuousVulnerabilityScansEnabled: true,
		ContainerScanningForRegistryEnabled: false,
		SecretPushProtectionEnabled:         true,
	}

	want := "## Project Security Settings (Project 42)\n\n" +
		"- **Project ID**: 42\n" +
		"- **Secret Push Protection**: ✅\n" +
		"- **Continuous Vulnerability Scans**: ✅\n" +
		"- **Container Scanning for Registry**: ❌\n" +
		"- **Auto-fix SAST**: ❌\n" +
		"- **Auto-fix DAST**: ❌\n" +
		"- **Auto-fix Dependency Scanning**: ✅\n" +
		"- **Auto-fix Container Scanning**: ✅\n" +
		"- **Created**: 1 Jan 2026 00:00 UTC\n" +
		"- **Updated**: 2 Jan 2026 09:30 UTC\n" +
		projectHints

	if got := FormatProjectMarkdown(out); got != want {
		t.Errorf("FormatProjectMarkdown() =\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatProjectMarkdown_NoTimestamps validates that a settings record
// GitLab sent no timestamps for writes neither time row: an absent value is
// never a label with nothing after it.
func TestFormatProjectMarkdown_NoTimestamps(t *testing.T) {
	out := ProjectOutput{
		ProjectID:                   10,
		SecretPushProtectionEnabled: false,
	}

	want := "## Project Security Settings (Project 10)\n\n" +
		"- **Project ID**: 10\n" +
		"- **Secret Push Protection**: ❌\n" +
		"- **Continuous Vulnerability Scans**: ❌\n" +
		"- **Container Scanning for Registry**: ❌\n" +
		"- **Auto-fix SAST**: ❌\n" +
		"- **Auto-fix DAST**: ❌\n" +
		"- **Auto-fix Dependency Scanning**: ❌\n" +
		"- **Auto-fix Container Scanning**: ❌\n" +
		projectHints

	got := FormatProjectMarkdown(out)
	if got != want {
		t.Errorf("FormatProjectMarkdown() =\n%s\nwant:\n%s", got, want)
	}
	if strings.Contains(got, "Created") || strings.Contains(got, "Updated") {
		t.Error("expected no time row when GitLab sent no timestamp")
	}
}

// TestFormatProjectMarkdown_ZeroProjectID validates that the renderer
// returns an empty string when ProjectID is zero (empty output).
func TestFormatProjectMarkdown_ZeroProjectID(t *testing.T) {
	out := ProjectOutput{}

	md := FormatProjectMarkdown(out)

	if md != "" {
		t.Errorf("expected empty markdown for zero ProjectID, got: %q", md)
	}
}

// TestFormatGroupMarkdown_NoErrors validates the group Markdown renderer with
// secret push protection enabled and no errors: no error table, no refusal
// count, and the scope sentence still written.
func TestFormatGroupMarkdown_NoErrors(t *testing.T) {
	out := GroupOutput{
		SecretPushProtectionEnabled: true,
	}

	want := "## Group Security Settings\n\n" +
		"- **Secret Push Protection**: ✅\n" +
		groupHints

	if got := FormatGroupMarkdown(out); got != want {
		t.Errorf("FormatGroupMarkdown() =\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatGroupMarkdown_Disabled validates the group Markdown renderer when
// secret push protection is disabled: the flag is a cross rather than the word
// "false", and the count of refusals stays absent.
func TestFormatGroupMarkdown_Disabled(t *testing.T) {
	out := GroupOutput{
		SecretPushProtectionEnabled: false,
	}

	want := "## Group Security Settings\n\n" +
		"- **Secret Push Protection**: ❌\n" +
		groupHints

	if got := FormatGroupMarkdown(out); got != want {
		t.Errorf("FormatGroupMarkdown() =\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatGroupMarkdown_WithErrors validates that the projects GitLab
// refused are counted on the card and listed as a collection under their own
// heading, so a reader can tell the group setting from its per-project result.
func TestFormatGroupMarkdown_WithErrors(t *testing.T) {
	out := GroupOutput{
		SecretPushProtectionEnabled: true,
		Errors:                      []string{"project 10 not found", "project 20 is archived"},
	}

	want := "## Group Security Settings\n\n" +
		"- **Secret Push Protection**: ✅\n" +
		"- **Projects GitLab could not update**: 2\n\n" +
		"### Errors\n\n" +
		"| Error |\n| --- |\n" +
		"| project 10 not found |\n" +
		"| project 20 is archived |\n" +
		groupHints

	if got := FormatGroupMarkdown(out); got != want {
		t.Errorf("FormatGroupMarkdown() =\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatGroupMarkdown_HostileError validates that a refusal message
// carrying Markdown cannot open a heading or a list item of its own: the cell
// escaper neutralizes the pipe, the tag and the bracket, and the card writes
// one table row whatever the message holds.
func TestFormatGroupMarkdown_HostileError(t *testing.T) {
	out := GroupOutput{
		SecretPushProtectionEnabled: true,
		Errors:                      []string{"a|b\n## injected\n- **State**: closed"},
	}

	want := "## Group Security Settings\n\n" +
		"- **Secret Push Protection**: ✅\n" +
		"- **Projects GitLab could not update**: 1\n\n" +
		"### Errors\n\n" +
		"| Error |\n| --- |\n" +
		"| a&#124;b ## injected - **State**: closed |\n" +
		groupHints

	if got := FormatGroupMarkdown(out); got != want {
		t.Errorf("FormatGroupMarkdown() =\n%s\nwant:\n%s", got, want)
	}
}
