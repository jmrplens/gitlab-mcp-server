package securitysettings

import (
	"strings"
	"testing"
)

// TestFormatProjectMarkdown_AllFields validates the Markdown renderer for
// project security settings when all fields are populated, including the
// optional UpdatedAt timestamp.
func TestFormatProjectMarkdown_AllFields(t *testing.T) {
	out := ProjectOutput{
		ProjectID:                           42,
		CreatedAt:                           "2024-01-01T00:00:00Z",
		UpdatedAt:                           "2024-01-02T00:00:00Z",
		AutoFixContainerScanning:            true,
		AutoFixDAST:                         false,
		AutoFixDependencyScanning:           true,
		AutoFixSAST:                         false,
		ContinuousVulnerabilityScansEnabled: true,
		ContainerScanningForRegistryEnabled: false,
		SecretPushProtectionEnabled:         true,
	}

	md := FormatProjectMarkdown(out)

	expectations := []string{
		"## Project Security Settings (Project 42)",
		"| Secret Push Protection | true |",
		"| Continuous Vulnerability Scans | true |",
		"| Container Scanning for Registry | false |",
		"| Auto-fix SAST | false |",
		"| Auto-fix DAST | false |",
		"| Auto-fix Dependency Scanning | true |",
		"| Auto-fix Container Scanning | true |",
		"**Updated**: 2024-01-02T00:00:00Z",
	}
	for _, exp := range expectations {
		if !strings.Contains(md, exp) {
			t.Errorf("expected markdown to contain %q, got:\n%s", exp, md)
		}
	}
}

// TestFormatProjectMarkdown_NoUpdatedAt validates the renderer omits the
// Updated line when UpdatedAt is empty.
func TestFormatProjectMarkdown_NoUpdatedAt(t *testing.T) {
	out := ProjectOutput{
		ProjectID:                   10,
		SecretPushProtectionEnabled: false,
	}

	md := FormatProjectMarkdown(out)

	if !strings.Contains(md, "## Project Security Settings (Project 10)") {
		t.Error("expected project header in markdown")
	}
	if strings.Contains(md, "**Updated**") {
		t.Error("expected no Updated line when UpdatedAt is empty")
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

// TestFormatGroupMarkdown_NoErrors validates the group Markdown renderer
// with secret push protection enabled and no errors.
func TestFormatGroupMarkdown_NoErrors(t *testing.T) {
	out := GroupOutput{
		SecretPushProtectionEnabled: true,
	}

	md := FormatGroupMarkdown(out)

	if !strings.Contains(md, "## Group Security Settings") {
		t.Error("expected group header in markdown")
	}
	if !strings.Contains(md, "**Secret Push Protection**: true") {
		t.Error("expected secret push protection true in markdown")
	}
	if strings.Contains(md, "**Errors**") {
		t.Error("expected no errors section when Errors is empty")
	}
}

// TestFormatGroupMarkdown_Disabled validates the group Markdown renderer
// when secret push protection is disabled.
func TestFormatGroupMarkdown_Disabled(t *testing.T) {
	out := GroupOutput{
		SecretPushProtectionEnabled: false,
	}

	md := FormatGroupMarkdown(out)

	if !strings.Contains(md, "**Secret Push Protection**: false") {
		t.Error("expected secret push protection false in markdown")
	}
}

// TestFormatGroupMarkdown_WithErrors validates the group Markdown renderer
// includes the errors section when the API response contains errors.
func TestFormatGroupMarkdown_WithErrors(t *testing.T) {
	out := GroupOutput{
		SecretPushProtectionEnabled: true,
		Errors:                      []string{"project 10 not found", "project 20 is archived"},
	}

	md := FormatGroupMarkdown(out)

	if !strings.Contains(md, "**Errors**") {
		t.Error("expected errors section in markdown")
	}
	if !strings.Contains(md, "- project 10 not found") {
		t.Error("expected first error in markdown")
	}
	if !strings.Contains(md, "- project 20 is archived") {
		t.Error("expected second error in markdown")
	}
}
