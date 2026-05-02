// prestart_test.go contains unit tests for the pre-start update check.
package autoupdate

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/creativeprojects/go-selfupdate"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestJustUpdated_Default verifies JustUpdated returns false when the
// environment variable is not set.
func TestJustUpdated_Default(t *testing.T) {
	os.Unsetenv(envJustUpdated)
	if JustUpdated() {
		t.Fatal("expected false when env var is not set")
	}
}

// TestJustUpdated_Set verifies the full set/check/clear cycle.
func TestJustUpdated_Set(t *testing.T) {
	t.Cleanup(func() { os.Unsetenv(envJustUpdated) })

	if err := SetJustUpdated(); err != nil {
		t.Fatalf("SetJustUpdated: %v", err)
	}
	if !JustUpdated() {
		t.Fatal("expected true after SetJustUpdated")
	}

	ClearJustUpdated()
	if JustUpdated() {
		t.Fatal("expected false after ClearJustUpdated")
	}
}

// TestCleanupOldBinary_RemovesOldFile verifies that .old files are cleaned up.
func TestCleanupOldBinary_RemovesOldFile(t *testing.T) {
	exe := stubExecutablePath(t)
	oldPath := exe + ".old"

	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("cannot create .old file: %v", err)
	}

	CleanupOldBinary()

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("expected .old file to be removed, Stat returned: %v", err)
	}
}

// TestCleanupOldBinary_NoOldFile verifies CleanupOldBinary is a no-op when
// there is no .old file.
func TestCleanupOldBinary_NoOldFile(t *testing.T) {
	// Should not panic or error when no .old exists.
	CleanupOldBinary()
}

// TestReplaceExecutable_Success verifies the rename-and-replace logic
// on a temporary directory with fake binaries.
func TestReplaceExecutable_Success(t *testing.T) {
	dir := t.TempDir()
	fakeCurrent := filepath.Join(dir, "current")
	fakeTmp := filepath.Join(dir, "current.tmp")

	if err := os.WriteFile(fakeCurrent, []byte("v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fakeTmp, []byte("v2"), 0o755); err != nil {
		t.Fatal(err)
	}

	// replaceExecutable uses os.Executable() which we can't override,
	// so we test the internal logic directly with file operations.
	oldPath := fakeCurrent + ".old"
	_ = os.Remove(oldPath)
	if err := os.Rename(fakeCurrent, oldPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(fakeTmp, fakeCurrent); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(fakeCurrent)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "v2" {
		t.Errorf("expected v2, got %q", string(data))
	}

	// .old should contain v1
	data, err = os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "v1" {
		t.Errorf("expected v1 in .old, got %q", string(data))
	}
}

// TestPreStartUpdate_JustUpdatedGuard verifies that PreStartUpdate skips
// update checks when the re-exec guard is set.
func TestPreStartUpdate_JustUpdatedGuard(t *testing.T) {
	t.Setenv(envJustUpdated, "1")

	result := PreStartUpdate(t.Context(), Config{
		Mode:           ModeAuto,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	})

	if result.Updated {
		t.Error("expected no update when just-updated guard is set")
	}
}

// TestPreStartUpdate_Disabled verifies PreStartUpdate is a no-op for
// disabled mode.
func TestPreStartUpdate_Disabled(t *testing.T) {
	result := PreStartUpdate(t.Context(), Config{
		Mode: ModeDisabled,
	})

	if result.Updated {
		t.Error("expected no update when disabled")
	}
}

// TestPreStartUpdate_CheckMode verifies PreStartUpdate reports available
// update but does not apply in check mode.
func TestPreStartUpdate_CheckMode(t *testing.T) {
	rel := newMockReleaseForPlatform("v2.0.0", "notes", "")
	src := &mockSource{releases: []selfupdate.SourceRelease{rel}}

	// We cannot easily set the source on the Updater through PreStartUpdate
	// since it creates its own Updater, so we test the component behavior
	// via CheckOnce with ModeCheck instead.
	u := NewUpdaterWithSource(Config{
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
		Mode:           ModeCheck,
	}, src)

	newVersion, updated, err := u.CheckOnce(t.Context())
	if err != nil {
		t.Fatalf("CheckOnce: %v", err)
	}
	if updated {
		t.Error("check mode should not apply update")
	}
	if newVersion != "2.0.0" {
		t.Errorf("expected version 2.0.0, got %q", newVersion)
	}
}

// TestPreStartUpdate_NewUpdaterError verifies that PreStartUpdate returns
// an empty result when NewUpdater fails (e.g. missing Repository).
func TestPreStartUpdate_NewUpdaterError(t *testing.T) {
	result := PreStartUpdate(t.Context(), Config{
		Mode:           ModeAuto,
		Repository:     "",
		CurrentVersion: "1.0.0",
	})
	if result.Updated {
		t.Error("expected no update when NewUpdater fails")
	}
	if result.NewVersion != "" {
		t.Errorf("NewVersion = %q, want empty", result.NewVersion)
	}
}

// TestPreStartUpdate_MissingCurrentVersion verifies that PreStartUpdate
// returns an empty result when CurrentVersion is invalid.
func TestPreStartUpdate_MissingCurrentVersion(t *testing.T) {
	result := PreStartUpdate(t.Context(), Config{
		Mode:       ModeAuto,
		Repository: "group/project",
		// Missing CurrentVersion → NewUpdater returns error.
	})
	if result.Updated {
		t.Error("expected no update when CurrentVersion is missing")
	}
}

// TestPreStartUpdate_DevVersion verifies that PreStartUpdate handles
// the "dev" version string, which NewUpdater rejects.
func TestPreStartUpdate_DevVersion(t *testing.T) {
	result := PreStartUpdate(t.Context(), Config{
		Mode:           ModeAuto,
		Repository:     "group/project",
		CurrentVersion: "dev",
	})
	if result.Updated {
		t.Error("expected no update for dev version")
	}
}

// ---------------------------------------------------------------------------
// replaceExecutable integration tests (C5 audit finding)
// ---------------------------------------------------------------------------

// TestReplaceExecutable_FileContentSwap verifies the full rename trick using
// replaceExecutable: the original binary becomes .old and the staged binary
// takes its place. Both file contents are verified after the swap to ensure
// data integrity.
func TestReplaceExecutable_FileContentSwap(t *testing.T) {
	exe := stubExecutablePath(t)
	tmpPath := exe + ".tmp"
	newContent := fakeBinary()

	if err := os.WriteFile(tmpPath, newContent, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := replaceExecutable(tmpPath); err != nil {
		t.Fatalf("replaceExecutable: %v", err)
	}

	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("reading swapped binary: %v", err)
	}
	if !bytes.Equal(got, newContent) {
		t.Error("swapped binary content does not match staged content")
	}

	oldData, err := os.ReadFile(exe + ".old")
	if err != nil {
		t.Fatalf("reading .old binary: %v", err)
	}
	if string(oldData) != "fake-binary" {
		t.Errorf(".old content = %q, want %q", string(oldData), "fake-binary")
	}
}

// TestReplaceExecutable_Rollback verifies that when the second os.Rename
// (tmp → exe) fails — because the staged file does not exist — the function
// rolls back by renaming .old → exe, restoring the original binary content.
func TestReplaceExecutable_Rollback(t *testing.T) {
	exe := stubExecutablePath(t)

	// tmpPath does not exist → second rename will fail.
	tmpPath := filepath.Join(t.TempDir(), "nonexistent.tmp")

	err := replaceExecutable(tmpPath)
	if err == nil {
		t.Fatal("expected error when staged file does not exist")
	}

	// The original binary should be restored via rollback.
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("original binary not restored after rollback: %v", err)
	}
	if string(data) != "fake-binary" {
		t.Errorf("rollback content = %q, want %q", string(data), "fake-binary")
	}

	// .old should not remain after successful rollback.
	if _, statErr := os.Stat(exe + ".old"); !os.IsNotExist(statErr) {
		t.Error("expected .old to be absent after rollback (renamed back to exe)")
	}
}

// TestReplaceExecutable_LeftoverOldRemoved verifies that a pre-existing .old
// file — left over from a previous update — is removed before the rename trick
// starts. After a successful swap the .old should contain the current binary,
// not the stale leftover.
func TestReplaceExecutable_LeftoverOldRemoved(t *testing.T) {
	exe := stubExecutablePath(t)
	oldPath := exe + ".old"
	tmpPath := exe + ".tmp"

	if err := os.WriteFile(oldPath, []byte("stale-leftover"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmpPath, fakeBinary(), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := replaceExecutable(tmpPath); err != nil {
		t.Fatalf("replaceExecutable: %v", err)
	}

	// .old should now contain the previous current binary, not the stale file.
	data, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatalf("reading .old: %v", err)
	}
	if string(data) != "fake-binary" {
		t.Errorf(".old = %q, want %q (previous exe, not stale leftover)", string(data), "fake-binary")
	}
}

// TestReplaceExecutable_ResolveError verifies replaceExecutable returns an
// error when resolveExecutable fails.
func TestReplaceExecutable_ResolveError(t *testing.T) {
	orig := resolveExecutable
	resolveExecutable = func() (string, error) {
		return "", errors.New("cannot resolve executable")
	}
	t.Cleanup(func() { resolveExecutable = orig })

	err := replaceExecutable("/tmp/dummy.tmp")
	if err == nil {
		t.Fatal("expected error when resolveExecutable fails")
	}
}

// ---------------------------------------------------------------------------
// PreStartUpdate ExecFailed path (C6 audit finding)
// ---------------------------------------------------------------------------

// TestPreStartUpdate_ExecSelfFails_Integration verifies the post-replace
// code path in PreStartUpdate where execSelf fails, causing ExecFailed=true.
//
// PreStartUpdate creates its own Updater internally, making end-to-end
// mocking through httptest fragile (go-selfupdate URL routing). Instead,
// this test exercises the same code path by calling downloadToStaging +
// replaceExecutable + execSelf in the same sequence PreStartUpdate uses,
// via a downloadableMockSource that bypasses HTTP entirely.
//
// On Windows, PreStartUpdate never calls execSelf (runtime.GOOS check),
// so the ExecFailed path is unreachable — test skipped.
func TestPreStartUpdate_ExecSelfFails_Integration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ExecFailed path unreachable on Windows (execSelf is not called)")
	}

	exe := stubExecSelf(t, errors.New("simulated exec failure"))

	rel := newMockReleaseForPlatform("v3.0.0", "exec-fail test", "")
	src := &downloadableMockSource{
		releases:     []selfupdate.SourceRelease{rel},
		downloadData: fakeBinary(),
	}
	u := NewUpdaterWithSource(Config{
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	// Replicate PreStartUpdate's sequence: download → replace → exec.
	newVersion, tmpPath, err := u.downloadToStaging(context.Background())
	if tmpPath != "" {
		t.Cleanup(func() { os.Remove(tmpPath) })
	}
	if err != nil {
		t.Fatalf("downloadToStaging: %v", err)
	}

	if err = replaceExecutable(tmpPath); err != nil {
		t.Fatalf("replaceExecutable: %v", err)
	}

	// Verify binary was replaced.
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("reading replaced binary: %v", err)
	}
	if !bytes.Equal(data, fakeBinary()) {
		t.Error("binary content not updated after replaceExecutable")
	}

	// Now exec should fail (stubbed).
	if execSelf() == nil {
		t.Fatal("expected error from stubbed execSelf")
	}

	// Verify the result matches what PreStartUpdate would return.
	if newVersion != "3.0.0" {
		t.Errorf("newVersion = %q, want %q", newVersion, "3.0.0")
	}
}

// TestCleanupOldBinary_ResolveError verifies CleanupOldBinary is a no-op
// when resolveExecutable returns an error (e.g. in a deleted-binary scenario).
func TestCleanupOldBinary_ResolveError(t *testing.T) {
	orig := resolveExecutable
	resolveExecutable = func() (string, error) {
		return "", errors.New("cannot resolve")
	}
	t.Cleanup(func() { resolveExecutable = orig })

	// Should not panic.
	CleanupOldBinary()
}

// TestDownloadAndReplace_FullSuccess verifies the complete DownloadAndReplace
// flow using a downloadableMockSource with valid binary data and a stubbed
// executable path to avoid touching the production binary.
func TestDownloadAndReplace_FullSuccess(t *testing.T) {
	exe := stubExecutablePath(t)

	rel := newMockReleaseForPlatform("v3.0.0", "", "")
	src := &downloadableMockSource{
		releases:     []selfupdate.SourceRelease{rel},
		downloadData: fakeBinary(),
	}
	u := NewUpdaterWithSource(Config{
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	version, err := u.DownloadAndReplace(context.Background())
	if err != nil {
		t.Fatalf("DownloadAndReplace: %v", err)
	}
	if version != "3.0.0" {
		t.Errorf("version = %q, want %q", version, "3.0.0")
	}

	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("reading replaced binary: %v", err)
	}
	if !bytes.Equal(data, fakeBinary()) {
		t.Error("binary content not updated")
	}
}

// TestPreStartUpdate_CheckForUpdateFails verifies PreStartUpdate returns an
// empty result when CheckForUpdate fails (e.g. network error). This is tested
// indirectly since PreStartUpdate creates its own Updater with a real GitHub
// source; we use a cancelled context to trigger the error path.
func TestPreStartUpdate_CheckForUpdateFails(t *testing.T) {
	ctx := testutil.CancelledCtx(t)

	result := PreStartUpdate(ctx, Config{
		Mode:           ModeAuto,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	})
	if result.Updated {
		t.Error("expected no update when context is cancelled")
	}
}

// ---------------------------------------------------------------------------
// PreStartUpdate deep-path coverage (requires stubNewGitHubSource)
// ---------------------------------------------------------------------------

// TestPreStartUpdate_UpToDate verifies PreStartUpdate returns an empty result
// when the server is already running the latest version (CheckForUpdate
// returns available=false).
func TestPreStartUpdate_UpToDate(t *testing.T) {
	rel := newMockReleaseForPlatform("v1.0.0", "", "")
	stubNewGitHubSource(t, &mockSource{releases: []selfupdate.SourceRelease{rel}})

	result := PreStartUpdate(t.Context(), Config{
		Mode:           ModeAuto,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	})
	if result.Updated {
		t.Error("expected no update when already up to date")
	}
	if result.NewVersion != "" {
		t.Errorf("NewVersion = %q, want empty", result.NewVersion)
	}
}

// TestPreStartUpdate_CheckMode_ReportsNewVersion verifies that in check-only
// mode, PreStartUpdate reports the available version without downloading or
// applying the update.
func TestPreStartUpdate_CheckMode_ReportsNewVersion(t *testing.T) {
	rel := newMockReleaseForPlatform("v2.0.0", "notes", "")
	stubNewGitHubSource(t, &mockSource{releases: []selfupdate.SourceRelease{rel}})

	result := PreStartUpdate(t.Context(), Config{
		Mode:           ModeCheck,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	})
	if result.Updated {
		t.Error("check mode should not apply update")
	}
	if result.NewVersion != "2.0.0" {
		t.Errorf("NewVersion = %q, want %q", result.NewVersion, "2.0.0")
	}
}

// TestPreStartUpdate_DownloadError verifies PreStartUpdate returns an empty
// result when downloadToStaging fails (e.g. asset download error).
func TestPreStartUpdate_DownloadError(t *testing.T) {
	stubExecutablePath(t)
	rel := newMockReleaseForPlatform("v3.0.0", "", "")
	src := &downloadableMockSource{
		releases:    []selfupdate.SourceRelease{rel},
		downloadErr: errors.New("download failed"),
	}
	stubNewGitHubSource(t, src)

	result := PreStartUpdate(t.Context(), Config{
		Mode:           ModeAuto,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	})
	if result.Updated {
		t.Error("expected no update when download fails")
	}
	if result.NewVersion != "" {
		t.Errorf("NewVersion = %q, want empty", result.NewVersion)
	}
}

// TestPreStartUpdate_ReplaceError verifies PreStartUpdate returns an empty
// result when replaceExecutable fails. This is triggered by making
// resolveExecutable return an error during the replace step (after download
// succeeds).
func TestPreStartUpdate_ReplaceError(t *testing.T) {
	rel := newMockReleaseForPlatform("v3.0.0", "", "")
	src := &downloadableMockSource{
		releases:     []selfupdate.SourceRelease{rel},
		downloadData: fakeBinary(),
	}
	stubNewGitHubSource(t, src)
	stubExecSelf(t, errors.New("should not be reached"))

	// First call (downloadToStaging) succeeds; second call
	// (replaceExecutable) fails, preventing the rename.
	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "gitlab-mcp-server")
	if err := os.WriteFile(fakeBin, []byte("fake-binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	callCount := 0
	orig := resolveExecutable
	resolveExecutable = func() (string, error) {
		callCount++
		if callCount <= 1 {
			return fakeBin, nil
		}
		return "", errors.New("cannot resolve for replace")
	}
	t.Cleanup(func() { resolveExecutable = orig })

	result := PreStartUpdate(t.Context(), Config{
		Mode:           ModeAuto,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	})
	if result.Updated {
		t.Error("expected no update when replace fails")
	}
}

// TestPreStartUpdate_ExecSelfFails_Full verifies the complete PreStartUpdate
// flow on Unix when execSelf fails: the binary is updated on disk but
// ExecFailed is set to true.
func TestPreStartUpdate_ExecSelfFails_Full(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ExecFailed path unreachable on Windows")
	}

	// Stub execSelf (fail), resolveExecutable (temp dir), and source.
	stubExecutablePath(t)
	stubExecSelf(t, errors.New("simulated exec failure"))

	rel := newMockReleaseForPlatform("v3.0.0", "", "")
	src := &downloadableMockSource{
		releases:     []selfupdate.SourceRelease{rel},
		downloadData: fakeBinary(),
	}
	stubNewGitHubSource(t, src)

	t.Cleanup(func() { os.Unsetenv(envJustUpdated) })

	result := PreStartUpdate(t.Context(), Config{
		Mode:           ModeAuto,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	})
	if !result.Updated {
		t.Error("expected Updated=true after successful replace")
	}
	if result.NewVersion != "3.0.0" {
		t.Errorf("NewVersion = %q, want %q", result.NewVersion, "3.0.0")
	}
	if !result.ExecFailed {
		t.Error("expected ExecFailed=true when execSelf returns error")
	}
	// JustUpdated should be cleared after exec failure.
	if JustUpdated() {
		t.Error("JustUpdated should be cleared after exec failure")
	}
}

// TestPreStartUpdate_UnixExecSuccess verifies the successful Unix update path
// where execSelf returns nil, simulating a process replacement that normally
// does not return to the caller.
func TestPreStartUpdate_UnixExecSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("exec path not taken on Windows")
	}

	// Stub resolveExecutable (temp dir) and execSelf (nil = simulated
	// successful exec that normally doesn't return). PreStartUpdate will
	// fall through to the Windows return path.
	stubExecutablePath(t)
	stubExecSelf(t, nil)

	rel := newMockReleaseForPlatform("v4.0.0", "", "")
	src := &downloadableMockSource{
		releases:     []selfupdate.SourceRelease{rel},
		downloadData: fakeBinary(),
	}
	stubNewGitHubSource(t, src)

	t.Cleanup(func() { os.Unsetenv(envJustUpdated) })

	result := PreStartUpdate(t.Context(), Config{
		Mode:           ModeAuto,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	})
	// When execSelf returns nil, the function falls through to the
	// "update will take effect on next restart" return.
	if !result.Updated {
		t.Error("expected Updated=true")
	}
	if result.NewVersion != "4.0.0" {
		t.Errorf("NewVersion = %q, want %q", result.NewVersion, "4.0.0")
	}
	if result.ExecFailed {
		t.Error("expected ExecFailed=false when execSelf succeeds")
	}
}

// TestPreStartUpdate_PanicRecovery verifies that a panic inside the update
// logic is caught by the deferred recover and PreStartUpdate returns an
// empty result instead of crashing.
func TestPreStartUpdate_PanicRecovery(t *testing.T) {
	// Override newGitHubSource to panic, triggering the recovery path.
	orig := newGitHubSource
	newGitHubSource = func() (selfupdate.Source, error) {
		panic("simulated panic in source creation")
	}
	t.Cleanup(func() { newGitHubSource = orig })

	result := PreStartUpdate(t.Context(), Config{
		Mode:           ModeAuto,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	})
	if result == nil {
		t.Fatal("expected non-nil result after panic recovery")
	}
	if result.Updated {
		t.Error("expected Updated=false after panic recovery")
	}
}

// TestPreStartUpdate_SourceError verifies PreStartUpdate returns an empty
// result when the source returns an error during CheckForUpdate (network
// failure, API rate limit, etc.).
func TestPreStartUpdate_SourceError(t *testing.T) {
	stubNewGitHubSource(t, &mockSource{err: errors.New("API rate limited")})

	result := PreStartUpdate(t.Context(), Config{
		Mode:           ModeAuto,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	})
	if result.Updated {
		t.Error("expected no update when source errors")
	}
}

// TestPreStartUpdate_NoReleases verifies PreStartUpdate returns an empty
// result when the source has no releases at all.
func TestPreStartUpdate_NoReleases(t *testing.T) {
	stubNewGitHubSource(t, &mockSource{releases: nil})

	result := PreStartUpdate(t.Context(), Config{
		Mode:           ModeAuto,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	})
	if result.Updated {
		t.Error("expected no update when no releases found")
	}
}

// ---------------------------------------------------------------------------
// replaceExecutable additional coverage
// ---------------------------------------------------------------------------

// TestReplaceExecutable_FirstRenameFails verifies replaceExecutable returns
// an error when the first os.Rename (exe → .old) fails because the
// executable no longer exists on disk.
func TestReplaceExecutable_FirstRenameFails(t *testing.T) {
	dir := t.TempDir()
	missingExe := filepath.Join(dir, "nonexistent-binary")

	orig := resolveExecutable
	resolveExecutable = func() (string, error) { return missingExe, nil }
	t.Cleanup(func() { resolveExecutable = orig })

	tmpPath := filepath.Join(dir, "staged.tmp")
	if err := os.WriteFile(tmpPath, fakeBinary(), 0o755); err != nil {
		t.Fatal(err)
	}

	err := replaceExecutable(tmpPath)
	if err == nil {
		t.Fatal("expected error when exe does not exist for first rename")
	}
	if !strings.Contains(err.Error(), "renaming current binary") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "renaming current binary")
	}
}
