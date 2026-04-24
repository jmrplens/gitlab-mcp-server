// serverupdate_test.go contains unit tests for the server update MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.

package serverupdate

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/autoupdate"
	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestFormatCheckMarkdownString_UpdateAvailable verifies Markdown output
// when a newer version is available.
func TestFormatCheckMarkdownString_UpdateAvailable(t *testing.T) {
	out := CheckOutput{
		UpdateAvailable: true,
		CurrentVersion:  "1.0.0",
		LatestVersion:   "2.0.0",
		ReleaseURL:      "https://example.com/releases/v2.0.0",
		ReleaseNotes:    "Bug fixes and improvements.",
		Mode:            "true",
		Author:          "Test Author",
		Department:      "Test Dept",
		Repository:      "https://example.com/repo",
	}

	md := FormatCheckMarkdownString(out)

	if md == "" {
		t.Fatal("expected non-empty Markdown")
	}
	if !contains(md, "Update Available") {
		t.Error("expected 'Update Available' in output")
	}
	if !contains(md, "2.0.0") {
		t.Error("expected latest version in output")
	}
	if !contains(md, "Bug fixes") {
		t.Error("expected release notes in output")
	}
	if !contains(md, "Test Author") {
		t.Error("expected author in output")
	}
	if !contains(md, "Test Dept") {
		t.Error("expected department in output")
	}
	if !contains(md, "https://example.com/repo") {
		t.Error("expected repository in output")
	}
}

// TestFormatCheckMarkdownString_UpToDate verifies Markdown output when no
// update is available.
func TestFormatCheckMarkdownString_UpToDate(t *testing.T) {
	out := CheckOutput{
		UpdateAvailable: false,
		CurrentVersion:  "1.0.0",
		Mode:            "true",
		Author:          "Author",
		Department:      "Dept",
		Repository:      "https://example.com/r",
	}

	md := FormatCheckMarkdownString(out)

	if !contains(md, "Up to Date") {
		t.Error("expected 'Up to Date' in output")
	}
	if !contains(md, "Author") {
		t.Error("expected author in output")
	}
	if !contains(md, "Dept") {
		t.Error("expected department in output")
	}
	if !contains(md, "https://example.com/r") {
		t.Error("expected repository in output")
	}
}

// TestFormatCheckMarkdownString_MetadataOmittedWhenEmpty verifies that
// Author, Department, and Repository labels are omitted when fields are empty.
func TestFormatCheckMarkdownString_MetadataOmittedWhenEmpty(t *testing.T) {
	out := CheckOutput{
		UpdateAvailable: false,
		CurrentVersion:  "1.0.0",
		Mode:            "true",
	}

	md := FormatCheckMarkdownString(out)

	if contains(md, "**Author**") {
		t.Error("should not contain Author when empty")
	}
	if contains(md, "**Department**") {
		t.Error("should not contain Department when empty")
	}
	if contains(md, "**Repository**") {
		t.Error("should not contain Repository when empty")
	}
}

// TestSetServerInfo_PopulatesCheckOutput verifies that SetServerInfo causes
// Check to include author, department, and repository in the CheckOutput.
func TestSetServerInfo_PopulatesCheckOutput(t *testing.T) {
	original := serverInfo
	t.Cleanup(func() { serverInfo = original })

	SetServerInfo(ServerInfo{
		Author:     "Test Author",
		Department: "Test Dept",
		Repository: "https://example.com/repo",
	})

	updater := newTestUpdater(t)

	out, err := Check(context.Background(), updater, CheckInput{})
	if err != nil {
		t.Fatalf("Check() unexpected error: %v", err)
	}
	if out.Author != "Test Author" {
		t.Errorf("Author = %q, want %q", out.Author, "Test Author")
	}
	if out.Department != "Test Dept" {
		t.Errorf("Department = %q, want %q", out.Department, "Test Dept")
	}
	if out.Repository != "https://example.com/repo" {
		t.Errorf("Repository = %q, want %q", out.Repository, "https://example.com/repo")
	}
}

// TestSetServerInfo_DefaultsEmpty verifies that when SetServerInfo is not
// called, the metadata fields in CheckOutput remain empty.
func TestSetServerInfo_DefaultsEmpty(t *testing.T) {
	original := serverInfo
	t.Cleanup(func() { serverInfo = original })

	SetServerInfo(ServerInfo{})

	updater := newTestUpdater(t)

	out, err := Check(context.Background(), updater, CheckInput{})
	if err != nil {
		t.Fatalf("Check() unexpected error: %v", err)
	}
	if out.Author != "" {
		t.Errorf("Author = %q, want empty", out.Author)
	}
	if out.Department != "" {
		t.Errorf("Department = %q, want empty", out.Department)
	}
	if out.Repository != "" {
		t.Errorf("Repository = %q, want empty", out.Repository)
	}
}

// TestFormatApplyMarkdownString_Applied verifies Markdown for a successful update.
func TestFormatApplyMarkdownString_Applied(t *testing.T) {
	out := ApplyOutput{
		Applied:         true,
		PreviousVersion: "1.0.0",
		NewVersion:      "2.0.0",
		Message:         "Updated from 1.0.0 to 2.0.0.",
	}

	md := FormatApplyMarkdownString(out)

	if !contains(md, "Update Applied") {
		t.Error("expected 'Update Applied' in output")
	}
	if !contains(md, "2.0.0") {
		t.Error("expected new version in output")
	}
}

// TestFormatApplyMarkdownString_NotApplied verifies Markdown when no update was applied.
func TestFormatApplyMarkdownString_NotApplied(t *testing.T) {
	out := ApplyOutput{
		Applied: false,
		Message: "No update needed.",
	}

	md := FormatApplyMarkdownString(out)

	if !contains(md, "No Update Applied") {
		t.Error("expected 'No Update Applied' in output")
	}
}

// contains is a helper to check substring presence.
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && stringContains(s, substr)
}

// stringContains is an internal helper for the serverupdate package.
func stringContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestFormatCheckMarkdownString_NoReleaseURL verifies Markdown when ReleaseURL is empty.
func TestFormatCheckMarkdownString_NoReleaseURL(t *testing.T) {
	out := CheckOutput{
		UpdateAvailable: true,
		CurrentVersion:  "1.0.0",
		LatestVersion:   "2.0.0",
		ReleaseNotes:    "Fixed things",
	}
	md := FormatCheckMarkdownString(out)
	if contains(md, "Release URL") {
		t.Error("should not contain Release URL when empty")
	}
}

// TestFormatCheckMarkdownString_NoReleaseNotes verifies Markdown when ReleaseNotes is empty.
func TestFormatCheckMarkdownString_NoReleaseNotes(t *testing.T) {
	out := CheckOutput{
		UpdateAvailable: true,
		CurrentVersion:  "1.0.0",
		LatestVersion:   "2.0.0",
		ReleaseURL:      "https://example.com/v2",
	}
	md := FormatCheckMarkdownString(out)
	if contains(md, "Release Notes") {
		t.Error("should not contain Release Notes when empty")
	}
}

// TestFormatCheckMarkdown verifies the MCP CallToolResult wrapper for Check output.
func TestFormatCheckMarkdown(t *testing.T) {
	out := CheckOutput{
		UpdateAvailable: true,
		CurrentVersion:  "1.0.0",
		LatestVersion:   "2.0.0",
	}
	result := FormatCheckMarkdown(out)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}
}

// TestFormatApplyMarkdown verifies the MCP CallToolResult wrapper for Apply output.
func TestFormatApplyMarkdown(t *testing.T) {
	out := ApplyOutput{
		Applied:         true,
		PreviousVersion: "1.0.0",
		NewVersion:      "2.0.0",
		Message:         "Done",
	}
	result := FormatApplyMarkdown(out)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}
}

// newTestUpdater creates an Updater with a mock source for testing.
func newTestUpdater(t *testing.T) *autoupdate.Updater {
	t.Helper()
	return autoupdate.NewUpdaterWithSource(autoupdate.Config{
		Repository:     "test/repo",
		CurrentVersion: "1.0.0",
	}, autoupdate.EmptySource{})
}

// newUnreachableUpdater creates an Updater whose backing source always
// returns an error, causing CheckForUpdate, ApplyUpdate, and
// DownloadAndReplace to fail immediately.
func newUnreachableUpdater(t *testing.T) *autoupdate.Updater {
	t.Helper()
	return autoupdate.NewUpdaterWithSource(autoupdate.Config{
		Repository:     "test/repo",
		CurrentVersion: "1.0.0",
	}, autoupdate.ErrorSource{Err: errors.New("unreachable")})
}

// TestCheck_NoUpdate verifies Check returns no update available when versions match.
func TestCheck_NoUpdate(t *testing.T) {
	updater := newTestUpdater(t)

	out, err := Check(context.Background(), updater, CheckInput{})
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if out.CurrentVersion != "1.0.0" {
		t.Errorf("CurrentVersion = %q, want %q", out.CurrentVersion, "1.0.0")
	}
	// With empty releases, there should be no update available
	if out.UpdateAvailable {
		t.Error("expected no update available with empty releases")
	}
}

// TestCheck_CancelledContext verifies Check respects context cancellation.
func TestCheck_CancelledContext(t *testing.T) {
	updater := newTestUpdater(t)
	ctx := testutil.CancelledCtx(t)

	_, err := Check(ctx, updater, CheckInput{})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// TestApply_CancelledContext verifies Apply respects context cancellation.
func TestApply_CancelledContext(t *testing.T) {
	updater := newTestUpdater(t)
	ctx := testutil.CancelledCtx(t)

	_, err := Apply(ctx, updater, ApplyInput{})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// TestApply_NoUpdateAvailable verifies Apply behavior when there is no update
// available. With empty releases, go-selfupdate returns the current version.
func TestApply_NoUpdateAvailable(t *testing.T) {
	updater := newTestUpdater(t)

	out, err := Apply(context.Background(), updater, ApplyInput{})
	if err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	if !out.Applied {
		t.Error("expected Applied=true")
	}
	if out.PreviousVersion != "1.0.0" {
		t.Errorf("PreviousVersion = %q, want %q", out.PreviousVersion, "1.0.0")
	}
	if out.NewVersion == "" {
		t.Error("expected non-empty NewVersion")
	}
	if out.Message == "" {
		t.Error("expected non-empty Message")
	}
}

// TestRegisterTools_NilUpdater verifies RegisterTools is a no-op when updater is nil.
func TestRegisterTools_NilUpdater(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, nil)
}

// TestRegisterTools_WithUpdater verifies RegisterTools does not panic with a valid updater.
func TestRegisterTools_WithUpdater(t *testing.T) {
	updater := newTestUpdater(t)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, updater)
}

// TestCheck_APIError verifies Check returns an error when the GitLab API
// endpoint is unreachable, exercising the CheckForUpdate error return path.
func TestCheck_APIError(t *testing.T) {
	updater := newUnreachableUpdater(t)

	_, err := Check(context.Background(), updater, CheckInput{})
	if err == nil {
		t.Fatal("expected error for unreachable API")
	}
	if !contains(err.Error(), "checking for update") {
		t.Errorf("error = %q, want it to contain 'checking for update'", err.Error())
	}
}

// TestApply_APIError verifies Apply returns an error when the GitLab API
// endpoint is unreachable. On Windows this exercises the deferred fallback
// path (applyDeferredFallback); on other platforms the direct error return.
func TestApply_APIError(t *testing.T) {
	updater := newUnreachableUpdater(t)

	_, err := Apply(context.Background(), updater, ApplyInput{})
	if err == nil {
		t.Fatal("expected error for unreachable API")
	}
}

// TestFormatApplyMarkdownString_Deferred verifies the Deferred branch of
// the apply Markdown formatter, including staging path and the Windows note.
func TestFormatApplyMarkdownString_Deferred(t *testing.T) {
	out := ApplyOutput{
		Deferred:        true,
		PreviousVersion: "1.0.0",
		NewVersion:      "2.0.0",
		StagingPath:     "/tmp/gitlab-mcp-server-staging",
	}

	md := FormatApplyMarkdownString(out)

	if !contains(md, "Update Downloaded (Deferred)") {
		t.Error("expected 'Update Downloaded (Deferred)' in output")
	}
	if !contains(md, "1.0.0") {
		t.Error("expected previous version in output")
	}
	if !contains(md, "2.0.0") {
		t.Error("expected new version in output")
	}
	if !contains(md, "/tmp/gitlab-mcp-server-staging") {
		t.Error("expected staging path in output")
	}
	if contains(md, "Update Script") {
		t.Error("should not contain Update Script when ScriptPath is empty")
	}
	if !contains(md, "Windows") {
		t.Error("expected Windows note in deferred output")
	}
}

// TestFormatApplyMarkdownString_DeferredWithScript verifies the Deferred branch
// includes the update script path when ScriptPath is set.
func TestFormatApplyMarkdownString_DeferredWithScript(t *testing.T) {
	out := ApplyOutput{
		Deferred:        true,
		PreviousVersion: "1.0.0",
		NewVersion:      "2.0.0",
		StagingPath:     "/tmp/gitlab-mcp-server-staging",
		ScriptPath:      "/tmp/update.ps1",
	}

	md := FormatApplyMarkdownString(out)

	if !contains(md, "Update Script") {
		t.Error("expected 'Update Script' when ScriptPath is set")
	}
	if !contains(md, "/tmp/update.ps1") {
		t.Error("expected script path in output")
	}
}

// TestRegisterTools_CallThroughMCP verifies the registered tools can be called
// through MCP in-memory transport.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	updater := newTestUpdater(t)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, updater)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	// Check tool — should succeed (no update from empty releases)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "gitlab_server_check_update"})
	if err != nil {
		t.Fatalf("CallTool(check) error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for check")
	}

	// Apply tool — should succeed (no update, returns current version)
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "gitlab_server_apply_update"})
	if err != nil {
		t.Fatalf("CallTool(apply) error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for apply")
	}
}
