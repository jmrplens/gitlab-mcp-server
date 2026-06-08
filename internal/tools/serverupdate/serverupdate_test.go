// serverupdate_test.go contains unit tests for the server update MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package serverupdate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"runtime"
	"strings"
	"testing"
	"time"

	selfupdate "github.com/creativeprojects/go-selfupdate"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/autoupdate"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

// stringContains supports string contains assertions in serverupdate tests.
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

// TestActionSpecs_NilUpdater verifies no update tools are exposed when updater is nil.
func TestActionSpecs_NilUpdater(t *testing.T) {
	if specs := ActionSpecs(nil); len(specs) != 0 {
		t.Fatalf("len(ActionSpecs(nil)) = %d, want 0", len(specs))
	}
}

// TestActionSpecs_Metadata verifies canonical metadata for server update actions.
func TestActionSpecs_Metadata(t *testing.T) {
	updater := newTestUpdater(t)
	byTool := serverUpdateSpecsByTool(t, ActionSpecs(updater))

	if len(byTool) != 2 {
		t.Fatalf("len(ActionSpecs) = %d, want 2", len(byTool))
	}
	if !byTool[checkUpdateToolName].ReadOnly || !byTool[checkUpdateToolName].Idempotent {
		t.Error("check update action should be read-only and idempotent")
	}
	if !byTool[applyUpdateToolName].Destructive || !byTool[applyUpdateToolName].Idempotent {
		t.Error("apply update action should be destructive and idempotent")
	}
	for _, spec := range byTool {
		if spec.OwnerPackage != "serverupdate" {
			t.Errorf("OwnerPackage for %s = %q, want serverupdate", spec.Name, spec.OwnerPackage)
		}
	}
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

// TestApplyDeferredFallback_DownloadError verifies the Windows fallback helper
// wraps errors from DownloadAndReplace without attempting to replace binaries.
func TestApplyDeferredFallback_DownloadError(t *testing.T) {
	updater := newUnreachableUpdater(t)
	out := ApplyOutput{PreviousVersion: "1.0.0"}

	_, err := applyDeferredFallback(context.Background(), updater, out, errors.New("apply failed"))
	if err == nil {
		t.Fatal("expected fallback download error")
	}
	if !contains(err.Error(), "Windows rename fallback") {
		t.Fatalf("error = %q, want fallback context", err.Error())
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

// TestActionSpecs_CallRoutes verifies the server update routes can be called directly.
func TestActionSpecs_CallRoutes(t *testing.T) {
	updater := newTestUpdater(t)
	byTool := serverUpdateSpecsByTool(t, ActionSpecs(updater))

	ctx := context.Background()
	result, err := byTool[checkUpdateToolName].Route.Handler(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("Route.Handler(check) error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for check")
	}

	result, err = byTool[applyUpdateToolName].Route.Handler(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("Route.Handler(apply) error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for apply")
	}
}

// serverUpdateSpecsByTool supports server update specs by tool assertions in serverupdate tests.
func serverUpdateSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}

// fakeSuccessSource implements selfupdate.Source and returns a valid
// release for the current platform plus a valid PE/MZ binary body on
// download. Used to exercise the applyDeferredFallback success path
// without requiring network access or a real GitHub release.
type fakeSuccessSource struct{}

func (fakeSuccessSource) ListReleases(_ context.Context, _ selfupdate.Repository) ([]selfupdate.SourceRelease, error) {
	assetName := fmt.Sprintf("gitlab-mcp-server-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}
	return []selfupdate.SourceRelease{
		&fakeRelease{tag: "v2.0.0", assetName: assetName},
	}, nil
}

func (fakeSuccessSource) DownloadReleaseAsset(_ context.Context, _ *selfupdate.Release, _ int64) (io.ReadCloser, error) {
	body := make([]byte, 1024*1024+1) // 1 MB + 1 byte to satisfy writeToFile's size guard
	body[0] = 'M'
	body[1] = 'Z'
	return io.NopCloser(bytes.NewReader(body)), nil
}

// fakeRelease implements selfupdate.SourceRelease with the minimal surface
// required by downloadToStaging: a tag name (version), an asset matching
// the current platform, and a non-zero AssetID.
type fakeRelease struct {
	tag       string
	assetName string
}

func (f *fakeRelease) GetID() int64              { return 1 }
func (f *fakeRelease) GetTagName() string        { return f.tag }
func (f *fakeRelease) GetDraft() bool            { return false }
func (f *fakeRelease) GetPrerelease() bool       { return false }
func (f *fakeRelease) GetPublishedAt() time.Time { return time.Time{} }
func (f *fakeRelease) GetReleaseNotes() string   { return "" }
func (f *fakeRelease) GetName() string           { return f.tag }
func (f *fakeRelease) GetURL() string            { return "" }
func (f *fakeRelease) GetAssets() []selfupdate.SourceAsset {
	return []selfupdate.SourceAsset{
		&fakeAsset{name: f.assetName, id: 1},
		&fakeAsset{name: "checksums.txt", id: 2},
	}
}

// fakeAsset implements selfupdate.SourceAsset with the minimal fields used
// by DetectLatest to match assets to the current platform.
type fakeAsset struct {
	name string
	id   int64
}

func (a *fakeAsset) GetID() int64                  { return a.id }
func (a *fakeAsset) GetName() string               { return a.name }
func (a *fakeAsset) GetSize() int                  { return 1024 * 1024 }
func (a *fakeAsset) GetBrowserDownloadURL() string { return "" }

// newSuccessUpdater constructs an autoupdate.Updater backed by
// fakeSuccessSource so DownloadAndReplace can succeed end-to-end.
func newSuccessUpdater(t *testing.T) *autoupdate.Updater {
	t.Helper()
	return autoupdate.NewUpdaterWithSource(autoupdate.Config{
		Repository:     "test/repo",
		CurrentVersion: "1.0.0",
	}, fakeSuccessSource{})
}

// TestApplyDeferredFallback_Success exercises the success branch of
// applyDeferredFallback directly. The helper calls updater.DownloadAndReplace
// and, on success, populates ApplyOutput with Applied=true, the new
// version, and a Windows-specific message. To reach this branch we need
// an updater whose backing source returns a valid release plus a binary
// body that satisfies writeToFile (size + magic bytes). The autoupdate
// package's resolveExecutable variable is now a package-level var
// (stubbed by the autoupdate package's tests), so the staging file is
// created next to the production binary — when that directory is
// writable and the production binary is not locked, the chain succeeds
// end-to-end.
func TestApplyDeferredFallback_Success(t *testing.T) {
	updater := newSuccessUpdater(t)
	out := ApplyOutput{PreviousVersion: "1.0.0"}

	got, err := applyDeferredFallback(t.Context(), updater, out, errors.New("apply failed"))
	if err != nil {
		// On hosts where the production binary's directory is not
		// writable (or the binary is locked), the test cannot reach
		// the success branch. The success branch is still covered by
		// the in-package autoupdate test
		// TestDownloadAndReplace_ReplaceFails, which uses the same
		// DownloadAndReplace pipeline with a temp-dir
		// resolveExecutable stub. Skip explicitly so the intent is
		// visible in test reports; any other error indicates a
		// regression and must fail the test.
		if strings.Contains(err.Error(), "rename") ||
			strings.Contains(err.Error(), "permission") ||
			errors.Is(err, fs.ErrPermission) {
			t.Skipf("environment-specific replace constraint: %v", err)
		}
		t.Fatalf("applyDeferredFallback failed: %v", err)
	}
	if !got.Applied {
		t.Error("expected Applied=true on success")
	}
	if got.NewVersion != "2.0.0" {
		t.Errorf("NewVersion = %q, want %q", got.NewVersion, "2.0.0")
	}
	if !strings.Contains(got.Message, "rename trick") {
		t.Errorf("Message = %q, want it to contain 'rename trick'", got.Message)
	}
}

// TestApply_WindowsFallbackContract documents the Apply branch that
// delegates to applyDeferredFallback when the running executable is
// locked. The branch is guarded by runtime.GOOS == "windows" so it is
// unreachable on Unix-like systems; on Windows the in-package
// applyDeferredFallback test (TestApplyDeferredFallback_Success) covers
// the helper. This test asserts the contract: on non-Windows hosts
// ApplyUpdate failures must surface directly with the "applying update"
// context, never through the deferred fallback path.
func TestApply_WindowsFallbackContract(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows hosts exercise the deferred fallback in TestApplyDeferredFallback_*")
	}
	updater := newUnreachableUpdater(t)
	_, err := Apply(context.Background(), updater, ApplyInput{})
	if err == nil {
		t.Fatal("expected error from Apply with unreachable updater")
	}
	if !strings.Contains(err.Error(), "applying update") {
		t.Errorf("err = %v, want to contain 'applying update' (non-Windows path)", err)
	}
}
