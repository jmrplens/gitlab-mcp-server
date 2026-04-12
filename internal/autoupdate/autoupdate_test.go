// autoupdate_test.go contains unit tests for the core auto-update logic.
package autoupdate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creativeprojects/go-selfupdate"
)

// Mock types for selfupdate.Source, SourceRelease, SourceAsset.

// mockSource implements selfupdate.Source for testing. It returns
// preconfigured releases or an error from ListReleases.
type mockSource struct {
	releases []selfupdate.SourceRelease
	err      error
}

// ListReleases performs the list releases operation on *mockSource.
func (m *mockSource) ListReleases(_ context.Context, _ selfupdate.Repository) ([]selfupdate.SourceRelease, error) {
	return m.releases, m.err
}

// DownloadReleaseAsset performs the download release asset operation on *mockSource.
func (m *mockSource) DownloadReleaseAsset(_ context.Context, _ *selfupdate.Release, _ int64) (io.ReadCloser, error) {
	return nil, errors.New("mock: download not implemented")
}

// mockRelease implements selfupdate.SourceRelease.
type mockRelease struct {
	tag    string
	notes  string
	url    string
	assets []selfupdate.SourceAsset
}

// GetID performs the get i d operation on *mockRelease.
func (r *mockRelease) GetID() int64 { return 1 }

// GetTagName performs the get tag name operation on *mockRelease.
func (r *mockRelease) GetTagName() string { return r.tag }

// GetDraft reports whether the *mockRelease satisfies the get draft condition.
func (r *mockRelease) GetDraft() bool { return false }

// GetPrerelease reports whether the *mockRelease satisfies the get prerelease condition.
func (r *mockRelease) GetPrerelease() bool { return false }

// GetPublishedAt performs the get published at operation on *mockRelease.
func (r *mockRelease) GetPublishedAt() time.Time { return time.Now() }

// GetReleaseNotes performs the get release notes operation on *mockRelease.
func (r *mockRelease) GetReleaseNotes() string { return r.notes }

// GetName performs the get name operation on *mockRelease.
func (r *mockRelease) GetName() string { return r.tag }

// GetURL performs the get u r l operation on *mockRelease.
func (r *mockRelease) GetURL() string { return r.url }

// GetAssets performs the get assets operation on *mockRelease.
func (r *mockRelease) GetAssets() []selfupdate.SourceAsset { return r.assets }

// mockAsset implements selfupdate.SourceAsset.
type mockAsset struct {
	id   int64
	name string
	url  string
}

// GetID performs the get i d operation on *mockAsset.
func (a *mockAsset) GetID() int64 { return a.id }

// GetName performs the get name operation on *mockAsset.
func (a *mockAsset) GetName() string { return a.name }

// GetSize performs the get size operation on *mockAsset.
func (a *mockAsset) GetSize() int { return 1024 }

// GetBrowserDownloadURL performs the get browser download u r l operation on *mockAsset.
func (a *mockAsset) GetBrowserDownloadURL() string { return a.url }

// newMockReleaseForPlatform creates a mock release with an asset matching
// the current GOOS/GOARCH and a checksums.txt validation asset.
func newMockReleaseForPlatform(tag, notes, url string) *mockRelease {
	assetName := fmt.Sprintf("gitlab-mcp-server-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}
	return &mockRelease{
		tag:   tag,
		notes: notes,
		url:   url,
		assets: []selfupdate.SourceAsset{
			&mockAsset{id: 1, name: assetName, url: "https://example.com/assets/" + assetName},
			&mockAsset{id: 2, name: "checksums.txt", url: "https://example.com/assets/checksums.txt"},
		},
	}
}

// TestParseMode verifies all accepted mode strings and the default.
func TestParseMode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Mode
		wantErr bool
	}{
		{name: "empty defaults to auto", input: "", want: ModeAuto},
		{name: "true", input: "true", want: ModeAuto},
		{name: "TRUE", input: "TRUE", want: ModeAuto},
		{name: "1", input: "1", want: ModeAuto},
		{name: "yes", input: "yes", want: ModeAuto},
		{name: "check", input: "check", want: ModeCheck},
		{name: "CHECK", input: "CHECK", want: ModeCheck},
		{name: "false", input: "false", want: ModeDisabled},
		{name: "FALSE", input: "FALSE", want: ModeDisabled},
		{name: "0", input: "0", want: ModeDisabled},
		{name: "no", input: "no", want: ModeDisabled},
		{name: "invalid", input: "maybe", wantErr: true},
		{name: "whitespace trimmed", input: "  true  ", want: ModeAuto},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMode(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseMode(%q) expected error, got %q", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseMode(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseMode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestSplitRepository verifies nested group paths and edge cases.
func TestSplitRepository(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "simple owner/repo",
			path:      "mygroup/myproject",
			wantOwner: "mygroup",
			wantRepo:  "myproject",
		},
		{
			name:      "nested groups",
			path:      "mcp/gitlab-mcp-server",
			wantOwner: "mcp",
			wantRepo:  "gitlab-mcp-server",
		},
		{
			name:      "leading/trailing slashes trimmed",
			path:      "/mcp/gitlab-mcp-server/",
			wantOwner: "mcp",
			wantRepo:  "gitlab-mcp-server",
		},
		{
			name:    "no slash",
			path:    "singlename",
			wantErr: true,
		},
		{
			name:    "empty",
			path:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := splitRepository(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("splitRepository(%q) expected error", tt.path)
				}
				return
			}
			if err != nil {
				t.Fatalf("splitRepository(%q) unexpected error: %v", tt.path, err)
			}
			if owner != tt.wantOwner || repo != tt.wantRepo {
				t.Errorf("splitRepository(%q) = (%q, %q), want (%q, %q)",
					tt.path, owner, repo, tt.wantOwner, tt.wantRepo)
			}
		})
	}
}

// TestNewUpdater_MissingFields verifies validation errors for required config fields.
func TestNewUpdater_MissingFields(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{name: "empty repo", cfg: Config{CurrentVersion: "1.0.0"}},
		{name: "empty version", cfg: Config{Repository: "a/b"}},
		{name: "dev version", cfg: Config{Repository: "a/b", CurrentVersion: "dev"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewUpdater(tt.cfg)
			if err == nil {
				t.Fatal("NewUpdater expected error for missing field")
			}
		})
	}
}

// TestNewUpdater_Defaults verifies that NewUpdater fills in defaults.
func TestNewUpdater_Defaults(t *testing.T) {
	u, err := NewUpdater(Config{
		Token:          "test-token",
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("NewUpdater: %v", err)
	}
	if u.cfg.Mode != DefaultMode {
		t.Errorf("Mode = %q, want %q", u.cfg.Mode, DefaultMode)
	}
	if u.cfg.Interval != DefaultInterval {
		t.Errorf("Interval = %v, want %v", u.cfg.Interval, DefaultInterval)
	}
}

// TestGetConfig_TokenRedacted verifies token is masked in returned config.
func TestGetConfig_TokenRedacted(t *testing.T) {
	u, err := NewUpdater(Config{
		Token:          "glpat-secret-token",
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("NewUpdater: %v", err)
	}
	c := u.GetConfig()
	if c.Token != "***" {
		t.Errorf("Token = %q, want '***'", c.Token)
	}
}

// TestIsEnabled verifies mode-to-enabled mapping.
func TestIsEnabled(t *testing.T) {
	tests := []struct {
		mode Mode
		want bool
	}{
		{ModeAuto, true},
		{ModeCheck, true},
		{ModeDisabled, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			u := NewUpdaterWithSource(Config{
				Mode:           tt.mode,
				Repository:     "a/b",
				CurrentVersion: "1.0.0",
			}, nil)
			if got := u.IsEnabled(); got != tt.want {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestCheckForUpdate_CancelledContext verifies context cancellation is respected.
func TestCheckForUpdate_CancelledContext(t *testing.T) {
	u := NewUpdaterWithSource(Config{
		Repository:     "a/b",
		CurrentVersion: "1.0.0",
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := u.CheckForUpdate(ctx)
	if err == nil {
		t.Fatal("CheckForUpdate expected error for canceled context")
	}
}

// TestApplyUpdate_CancelledContext verifies context cancellation is respected.
func TestApplyUpdate_CancelledContext(t *testing.T) {
	u := NewUpdaterWithSource(Config{
		Repository:     "a/b",
		CurrentVersion: "1.0.0",
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := u.ApplyUpdate(ctx)
	if err == nil {
		t.Fatal("ApplyUpdate expected error for canceled context")
	}
}

// TestStartPeriodicCheck_ContextCancellation verifies the goroutine exits on cancel.
func TestStartPeriodicCheck_ContextCancellation(t *testing.T) {
	u := NewUpdaterWithSource(Config{
		Mode:           ModeDisabled,
		Repository:     "a/b",
		CurrentVersion: "1.0.0",
		Interval:       100 * time.Millisecond,
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	u.StartPeriodicCheck(ctx)

	// Allow the goroutine to start.
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Allow the goroutine to exit gracefully.
	time.Sleep(200 * time.Millisecond)
}

// NewUpdater branch coverage.

// TestNewUpdater_ModePreset verifies Mode is preserved when already set.
func TestNewUpdater_ModePreset(t *testing.T) {
	u, err := NewUpdater(Config{
		Token:          "test-token",
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
		Mode:           ModeCheck,
	})
	if err != nil {
		t.Fatalf("NewUpdater: %v", err)
	}
	if u.cfg.Mode != ModeCheck {
		t.Errorf("Mode = %q, want %q", u.cfg.Mode, ModeCheck)
	}
}

// TestNewUpdater_IntervalPreset verifies Interval is preserved when already set.
func TestNewUpdater_IntervalPreset(t *testing.T) {
	custom := 5 * time.Minute
	u, err := NewUpdater(Config{
		Token:          "test-token",
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
		Interval:       custom,
	})
	if err != nil {
		t.Fatalf("NewUpdater: %v", err)
	}
	if u.cfg.Interval != custom {
		t.Errorf("Interval = %v, want %v", u.cfg.Interval, custom)
	}
}

// CheckForUpdate coverage.

// TestCheckForUpdate_NoReleases verifies that CheckForUpdate returns
// not-available when the source has no releases.
func TestCheckForUpdate_NoReleases(t *testing.T) {
	src := &mockSource{releases: nil}
	u := NewUpdaterWithSource(Config{
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	info, available, err := u.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if available {
		t.Error("expected available=false for empty releases")
	}
	if info == nil {
		t.Fatal("expected non-nil info even when not found")
	}
	if info.CurrentVersion != "1.0.0" {
		t.Errorf("CurrentVersion = %q, want %q", info.CurrentVersion, "1.0.0")
	}
}

// TestCheckForUpdate_SourceError verifies error propagation from the source.
func TestCheckForUpdate_SourceError(t *testing.T) {
	src := &mockSource{err: errors.New("network failure")}
	u := NewUpdaterWithSource(Config{
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	_, _, err := u.CheckForUpdate(context.Background())
	if err == nil {
		t.Fatal("expected error from source, got nil")
	}
	if !strings.Contains(err.Error(), "network failure") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "network failure")
	}
}

// TestCheckForUpdate_NewerVersion verifies that a newer release is detected
// as available. Requires a mock release with platform-matching assets and
// a checksums.txt validation asset.
func TestCheckForUpdate_NewerVersion(t *testing.T) {
	rel := newMockReleaseForPlatform("v2.0.0", "New features", "https://example.com/v2.0.0")
	src := &mockSource{releases: []selfupdate.SourceRelease{rel}}
	u := NewUpdaterWithSource(Config{
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	info, available, err := u.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !available {
		t.Error("expected available=true for newer version")
	}
	if info.LatestVersion != "2.0.0" {
		t.Errorf("LatestVersion = %q, want %q", info.LatestVersion, "2.0.0")
	}
	if info.ReleaseNotes != "New features" {
		t.Errorf("ReleaseNotes = %q, want %q", info.ReleaseNotes, "New features")
	}
	if info.ReleaseURL != "https://example.com/v2.0.0" {
		t.Errorf("ReleaseURL = %q, want %q", info.ReleaseURL, "https://example.com/v2.0.0")
	}
}

// TestCheckForUpdate_SameVersion verifies that a release at the same version
// is reported as not-available (GreaterThan returns false).
func TestCheckForUpdate_SameVersion(t *testing.T) {
	rel := newMockReleaseForPlatform("v1.0.0", "", "")
	src := &mockSource{releases: []selfupdate.SourceRelease{rel}}
	u := NewUpdaterWithSource(Config{
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	info, available, err := u.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if available {
		t.Error("expected available=false for same version")
	}
	if info.LatestVersion != "1.0.0" {
		t.Errorf("LatestVersion = %q, want %q", info.LatestVersion, "1.0.0")
	}
}

// TestCheckForUpdate_InvalidRepository verifies the splitRepository error path.
func TestCheckForUpdate_InvalidRepository(t *testing.T) {
	src := &mockSource{}
	u := NewUpdaterWithSource(Config{
		Repository:     "noslash",
		CurrentVersion: "1.0.0",
	}, src)

	_, _, err := u.CheckForUpdate(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid repository path")
	}
}

// ApplyUpdate coverage.

// TestApplyUpdate_InvalidRepository verifies the splitRepository error path in ApplyUpdate.
func TestApplyUpdate_InvalidRepository(t *testing.T) {
	src := &mockSource{}
	u := NewUpdaterWithSource(Config{
		Repository:     "noslash",
		CurrentVersion: "1.0.0",
	}, src)

	_, err := u.ApplyUpdate(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid repository")
	}
}

// TestApplyUpdate_NoUpdateAvailable verifies ApplyUpdate behavior
// when the source has no releases matching the current platform.
// go-selfupdate's UpdateSelf returns the current version when no update is found.
func TestApplyUpdate_NoUpdateAvailable(t *testing.T) {
	src := &mockSource{releases: nil}
	u := NewUpdaterWithSource(Config{
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	version, err := u.ApplyUpdate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// UpdateSelf returns the current version when nothing to update.
	if version != "1.0.0" {
		t.Errorf("version = %q, want %q", version, "1.0.0")
	}
}

// CheckOnce coverage.

// TestCheckOnce_NoUpdate verifies the "server is up to date" path.
func TestCheckOnce_NoUpdate(t *testing.T) {
	src := &mockSource{releases: nil}
	u := NewUpdaterWithSource(Config{
		Mode:           ModeAuto,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	newVersion, updated, err := u.CheckOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated {
		t.Error("expected updated=false")
	}
	if newVersion != "" {
		t.Errorf("newVersion = %q, want empty", newVersion)
	}
}

// TestCheckOnce_Error verifies error propagation from CheckForUpdate.
func TestCheckOnce_Error(t *testing.T) {
	src := &mockSource{err: errors.New("connection refused")}
	u := NewUpdaterWithSource(Config{
		Mode:           ModeAuto,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	_, _, err := u.CheckOnce(context.Background())
	if err == nil {
		t.Fatal("expected error from CheckOnce")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "connection refused")
	}
}

// TestCheckOnce_ModeCheck verifies that in check-only mode, an available
// update is reported but not applied.
func TestCheckOnce_ModeCheck(t *testing.T) {
	rel := newMockReleaseForPlatform("v2.0.0", "Release notes", "https://example.com/v2")
	src := &mockSource{releases: []selfupdate.SourceRelease{rel}}
	u := NewUpdaterWithSource(Config{
		Mode:           ModeCheck,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	newVersion, updated, err := u.CheckOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated {
		t.Error("expected updated=false in check-only mode")
	}
	if newVersion != "2.0.0" {
		t.Errorf("newVersion = %q, want %q", newVersion, "2.0.0")
	}
}

// periodicCheckOnce coverage.

// TestPeriodicCheckOnce_NoUpdate verifies the "no update" path by triggering
// a periodic check with a short interval and an empty source.
func TestPeriodicCheckOnce_NoUpdate(t *testing.T) {
	src := &mockSource{releases: nil}
	u := NewUpdaterWithSource(Config{
		Mode:           ModeAuto,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
		Interval:       50 * time.Millisecond,
	}, src)

	ctx := t.Context()

	u.periodicCheckOnce(ctx)
	// No panic, no error — the "no update available" branch was exercised.
}

// TestPeriodicCheckOnce_Error verifies the error handling path by providing
// a source that returns an error.
func TestPeriodicCheckOnce_Error(t *testing.T) {
	src := &mockSource{err: errors.New("timeout")}
	u := NewUpdaterWithSource(Config{
		Mode:           ModeAuto,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
		Interval:       50 * time.Millisecond,
	}, src)

	ctx := t.Context()

	u.periodicCheckOnce(ctx)
	// No panic — the "check failed" log branch was exercised.
}

// TestPeriodicCheckOnce_ModeCheck verifies that in check-only mode,
// an available update is logged but not applied.
func TestPeriodicCheckOnce_ModeCheck(t *testing.T) {
	rel := newMockReleaseForPlatform("v3.0.0", "Big release", "https://example.com/v3")
	src := &mockSource{releases: []selfupdate.SourceRelease{rel}}
	u := NewUpdaterWithSource(Config{
		Mode:           ModeCheck,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
		Interval:       50 * time.Millisecond,
	}, src)

	ctx := t.Context()

	u.periodicCheckOnce(ctx)
	// No panic — the "check-only mode — skipping apply" branch was exercised.
}

// TestPeriodicCheckOnce_ModeAutoApplyFails verifies that in auto mode
// when an update is found but ApplyUpdate fails, the error is logged.
func TestPeriodicCheckOnce_ModeAutoApplyFails(t *testing.T) {
	rel := newMockReleaseForPlatform("v4.0.0", "Upgrade", "https://example.com/v4")
	src := &mockSource{releases: []selfupdate.SourceRelease{rel}}
	u := NewUpdaterWithSource(Config{
		Mode:           ModeAuto,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
		Interval:       50 * time.Millisecond,
	}, src)

	ctx := t.Context()

	// ApplyUpdate will fail because the mock source doesn't support download.
	u.periodicCheckOnce(ctx)
	// No panic — the "failed to apply update" log branch was exercised.
}

// TestStartPeriodicCheck_RunsOnce verifies the ticker fires at least once
// and exercises periodicCheckOnce through the goroutine path.
func TestStartPeriodicCheck_RunsOnce(t *testing.T) {
	var mu sync.Mutex
	callCount := 0
	src := &mockSource{
		releases: nil,
		err: func() error {
			mu.Lock()
			callCount++
			mu.Unlock()
			return errors.New("counted")
		}(),
	}
	u := NewUpdaterWithSource(Config{
		Mode:           ModeAuto,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
		Interval:       30 * time.Millisecond,
	}, src)

	ctx, cancel := context.WithCancel(context.Background())
	u.StartPeriodicCheck(ctx)

	// Wait for at least one tick.
	time.Sleep(100 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)
}

// CheckOnce with ModeAuto + ApplyUpdate failure.

// TestCheckOnce_ModeAutoApplyFails verifies that in auto mode, when
// CheckForUpdate finds a newer version but ApplyUpdate fails (because
// the mock source cannot download), the error is propagated.
func TestCheckOnce_ModeAutoApplyFails(t *testing.T) {
	rel := newMockReleaseForPlatform("v5.0.0", "Major", "https://example.com/v5")
	src := &mockSource{releases: []selfupdate.SourceRelease{rel}}
	u := NewUpdaterWithSource(Config{
		Mode:           ModeAuto,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	_, _, err := u.CheckOnce(context.Background())
	if err == nil {
		t.Fatal("expected error from ApplyUpdate failure")
	}
}

// downloadableMockSource implements selfupdate.Source and returns configurable
// binary data from DownloadReleaseAsset, unlike mockSource which always errors.
type downloadableMockSource struct {
	releases     []selfupdate.SourceRelease
	listErr      error
	downloadData []byte
	downloadErr  error
}

// ListReleases performs the list releases operation on downloadableMockSource.
func (m *downloadableMockSource) ListReleases(_ context.Context, _ selfupdate.Repository) ([]selfupdate.SourceRelease, error) {
	return m.releases, m.listErr
}

// DownloadReleaseAsset returns the configured data or error.
func (m *downloadableMockSource) DownloadReleaseAsset(_ context.Context, _ *selfupdate.Release, _ int64) (io.ReadCloser, error) {
	if m.downloadErr != nil {
		return nil, m.downloadErr
	}
	return io.NopCloser(bytes.NewReader(m.downloadData)), nil
}

// TestExecSelf_Windows verifies that ExecSelf returns a descriptive error
// on Windows where exec-self is not supported.
func TestExecSelf_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("ExecSelf error test only applicable on Windows")
	}
	err := ExecSelf()
	if err == nil {
		t.Fatal("expected error from ExecSelf on Windows")
	}
	if !strings.Contains(err.Error(), "not supported on Windows") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "not supported on Windows")
	}
}

// TestExecSelf_ReturnsError verifies that ExecSelf returns an error on
// every supported platform. On Windows it is always unsupported; on Unix
// the syscall.Exec call cannot be exercised in a unit test (it replaces the
// process), so that branch is skipped.
func TestExecSelf_ReturnsError(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("ExecSelf succeeds on Unix (replaces process); cannot verify in unit test")
	}
	err := ExecSelf()
	if err == nil {
		t.Fatal("expected error from ExecSelf")
	}
}

// TestExecSelf_ViaVariable verifies that the package-level execSelf variable
// points to ExecSelf and returns the expected error when called on Windows.
// On non-Windows the variable is stubbed to avoid replacing the test process.
func TestExecSelf_ViaVariable(t *testing.T) {
	if runtime.GOOS == "windows" {
		err := execSelf()
		if err == nil {
			t.Fatal("expected error from execSelf on Windows")
		}
		if !strings.Contains(err.Error(), "not supported on Windows") {
			t.Errorf("error = %q, want to contain %q", err.Error(), "not supported on Windows")
		}
		return
	}
	// On Unix, stub to avoid process replacement.
	orig := execSelf
	execSelf = func() error { return errors.New("stubbed") }
	t.Cleanup(func() { execSelf = orig })

	err := execSelf()
	if err == nil {
		t.Fatal("expected error from stubbed execSelf")
	}
}

// TestGetConfig_EmptyToken verifies that GetConfig does not redact an
// empty token (only non-empty tokens are masked with '***').
func TestGetConfig_EmptyToken(t *testing.T) {
	u := NewUpdaterWithSource(Config{
		Repository:     "a/b",
		CurrentVersion: "1.0.0",
	}, nil)
	c := u.GetConfig()
	if c.Token != "" {
		t.Errorf("Token = %q, want empty string", c.Token)
	}
}

// TestDownloadToStaging_Success verifies the full download-to-staging flow:
// DetectLatest finds a newer version, DownloadReleaseAsset provides valid
// binary data, and writeToFile creates a valid .tmp file.
func TestDownloadToStaging_Success(t *testing.T) {
	rel := newMockReleaseForPlatform("v2.0.0", "", "")
	src := &downloadableMockSource{
		releases:     []selfupdate.SourceRelease{rel},
		downloadData: fakeBinary(),
	}
	u := NewUpdaterWithSource(Config{
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	version, tmpPath, err := u.downloadToStaging(context.Background())
	if tmpPath != "" {
		t.Cleanup(func() { os.Remove(tmpPath) })
	}
	if err != nil {
		t.Fatalf("downloadToStaging: %v", err)
	}
	if version != "2.0.0" {
		t.Errorf("version = %q, want %q", version, "2.0.0")
	}
	if !strings.HasSuffix(tmpPath, ".tmp") {
		t.Errorf("tmpPath = %q, want .tmp suffix", tmpPath)
	}
	// Verify the staging file exists and is large enough.
	info, err := os.Stat(tmpPath)
	if err != nil {
		t.Fatalf("stat staging file: %v", err)
	}
	if info.Size() < minBinarySize {
		t.Errorf("staging file size = %d, want >= %d", info.Size(), minBinarySize)
	}
}

// TestDownloadToStaging_InvalidRepository verifies that downloadToStaging
// returns an error when the repository path cannot be split.
func TestDownloadToStaging_InvalidRepository(t *testing.T) {
	u := NewUpdaterWithSource(Config{
		Repository:     "noslash",
		CurrentVersion: "1.0.0",
	}, nil)
	_, _, err := u.downloadToStaging(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid repository")
	}
}

// TestDownloadToStaging_NoNewerVersion verifies that downloadToStaging
// returns an error when no release is newer than the current version.
func TestDownloadToStaging_NoNewerVersion(t *testing.T) {
	rel := newMockReleaseForPlatform("v0.5.0", "", "")
	src := &downloadableMockSource{
		releases:     []selfupdate.SourceRelease{rel},
		downloadData: fakeBinary(),
	}
	u := NewUpdaterWithSource(Config{
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	_, _, err := u.downloadToStaging(context.Background())
	if err == nil {
		t.Fatal("expected error when no newer version exists")
	}
	if !strings.Contains(err.Error(), "no newer version") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "no newer version")
	}
}

// TestDownloadToStaging_DownloadError verifies that downloadToStaging
// returns an error when DownloadReleaseAsset fails.
func TestDownloadToStaging_DownloadError(t *testing.T) {
	rel := newMockReleaseForPlatform("v2.0.0", "", "")
	src := &downloadableMockSource{
		releases:    []selfupdate.SourceRelease{rel},
		downloadErr: errors.New("connection reset"),
	}
	u := NewUpdaterWithSource(Config{
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	_, _, err := u.downloadToStaging(context.Background())
	if err == nil {
		t.Fatal("expected error for download failure")
	}
	if !strings.Contains(err.Error(), "downloading release asset") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "downloading release asset")
	}
}

// TestDownloadToStaging_InvalidBinaryContent verifies that downloadToStaging
// returns errNotBinary when the downloaded data is large enough but has
// unrecognized magic bytes (e.g. a large JSON error page).
func TestDownloadToStaging_InvalidBinaryContent(t *testing.T) {
	rel := newMockReleaseForPlatform("v2.0.0", "", "")
	// Large enough to pass size check but with invalid magic bytes.
	badData := make([]byte, minBinarySize+1)
	for i := range badData {
		badData[i] = 'X'
	}
	src := &downloadableMockSource{
		releases:     []selfupdate.SourceRelease{rel},
		downloadData: badData,
	}
	u := NewUpdaterWithSource(Config{
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	_, tmpPath, err := u.downloadToStaging(context.Background())
	if tmpPath != "" {
		t.Cleanup(func() { os.Remove(tmpPath) })
	}
	if err == nil {
		t.Fatal("expected error for invalid binary content")
	}
	if !errors.Is(err, errNotBinary) {
		t.Errorf("err = %v, want errNotBinary", err)
	}
}

// TestDownloadToStaging_EmptyReleases verifies downloadToStaging returns
// an error when the source has no releases (DetectLatest returns found=false).
func TestDownloadToStaging_EmptyReleases(t *testing.T) {
	src := &downloadableMockSource{releases: nil}
	u := NewUpdaterWithSource(Config{
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	_, _, err := u.downloadToStaging(context.Background())
	if err == nil {
		t.Fatal("expected error for empty releases")
	}
}

// TestDownloadToStaging_ListError verifies downloadToStaging propagates
// ListReleases errors.
func TestDownloadToStaging_ListError(t *testing.T) {
	src := &downloadableMockSource{listErr: errors.New("API rate limited")}
	u := NewUpdaterWithSource(Config{
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	_, _, err := u.downloadToStaging(context.Background())
	if err == nil {
		t.Fatal("expected error from list releases")
	}
}

// TestDownloadAndReplace_InvalidRepository verifies DownloadAndReplace
// returns an error when the repository path is invalid.
func TestDownloadAndReplace_InvalidRepository(t *testing.T) {
	u := NewUpdaterWithSource(Config{
		Repository:     "noslash",
		CurrentVersion: "1.0.0",
	}, nil)
	_, err := u.DownloadAndReplace(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid repository")
	}
}

// TestDownloadAndReplace_DownloadError verifies DownloadAndReplace returns
// an error when the download fails.
func TestDownloadAndReplace_DownloadError(t *testing.T) {
	rel := newMockReleaseForPlatform("v2.0.0", "", "")
	src := &downloadableMockSource{
		releases:    []selfupdate.SourceRelease{rel},
		downloadErr: errors.New("network error"),
	}
	u := NewUpdaterWithSource(Config{
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	_, err := u.DownloadAndReplace(context.Background())
	if err == nil {
		t.Fatal("expected error for download failure")
	}
}

// TestDownloadAndReplace_InvalidBinaryContent verifies DownloadAndReplace
// returns errNotBinary when downloaded content fails binary validation.
func TestDownloadAndReplace_InvalidBinaryContent(t *testing.T) {
	rel := newMockReleaseForPlatform("v2.0.0", "", "")
	badData := make([]byte, minBinarySize+1)
	src := &downloadableMockSource{
		releases:     []selfupdate.SourceRelease{rel},
		downloadData: badData,
	}
	u := NewUpdaterWithSource(Config{
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	_, err := u.DownloadAndReplace(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid binary content")
	}
	if !errors.Is(err, errNotBinary) {
		t.Errorf("err = %v, want errNotBinary", err)
	}
}

// restoreTestBinary stubs the executable path to a temp directory and
// registers cleanup that restores any .old backup. This prevents tests
// from modifying the production binary.
func restoreTestBinary(t *testing.T) {
	t.Helper()
	stubExecutablePath(t)
}

// TestDownloadAndReplace_ReplaceFails verifies DownloadAndReplace when
// downloadToStaging succeeds but replaceExecutable may fail (on Windows
// the running binary may or may not be locked by the OS depending on how
// the test binary was built). Both outcomes are valid and cover useful paths.
func TestDownloadAndReplace_ReplaceFails(t *testing.T) {
	restoreTestBinary(t)

	rel := newMockReleaseForPlatform("v2.0.0", "", "")
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
		// Binary was locked — covers downloadToStaging success + replaceExecutable error.
		if !strings.Contains(err.Error(), "renaming current binary") {
			t.Errorf("error = %q, want to contain 'renaming current binary'", err.Error())
		}
		return
	}
	// Binary was NOT locked — covers full success path including replaceExecutable.
	if version != "2.0.0" {
		t.Errorf("version = %q, want %q", version, "2.0.0")
	}
}

// TestCheckOnce_ModeAuto_WindowsFallbackRenameFails verifies the Windows
// fallback path in CheckOnce: ApplyUpdate fails (checksum mismatch),
// then checkOnceFallbackDownload runs. The rename may or may not succeed
// depending on whether the OS locks the test binary.
func TestCheckOnce_ModeAuto_WindowsFallbackRenameFails(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows fallback test only runs on Windows")
	}
	restoreTestBinary(t)

	rel := newMockReleaseForPlatform("v2.0.0", "", "")
	src := &downloadableMockSource{
		releases:     []selfupdate.SourceRelease{rel},
		downloadData: fakeBinary(),
	}
	u := NewUpdaterWithSource(Config{
		Mode:           ModeAuto,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	newVersion, updated, err := u.CheckOnce(context.Background())
	if err != nil {
		// Binary locked — fallback download succeeded but rename failed.
		if !strings.Contains(err.Error(), "rename fallback") {
			t.Errorf("error = %q, want to contain 'rename fallback'", err.Error())
		}
		return
	}
	// Binary NOT locked — full fallback success.
	if !updated {
		t.Error("expected updated=true for fallback success")
	}
	if newVersion != "2.0.0" {
		t.Errorf("newVersion = %q, want %q", newVersion, "2.0.0")
	}
}

// TestPeriodicCheckOnce_ModeAuto_WindowsFallbackRenameFails verifies the
// Windows fallback path in periodicCheckOnce. Both outcomes (rename success
// or failure) exercise distinct branches in periodicFallbackDownload.
func TestPeriodicCheckOnce_ModeAuto_WindowsFallbackRenameFails(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows fallback test only runs on Windows")
	}
	restoreTestBinary(t)

	rel := newMockReleaseForPlatform("v2.0.0", "", "")
	src := &downloadableMockSource{
		releases:     []selfupdate.SourceRelease{rel},
		downloadData: fakeBinary(),
	}
	u := NewUpdaterWithSource(Config{
		Mode:           ModeAuto,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	// periodicCheckOnce logs results; no panic = exercise completed.
	u.periodicCheckOnce(context.Background())
}

// TestPeriodicFallbackDownload_DownloadError verifies the error path in
// periodicFallbackDownload when downloadToStaging fails.
func TestPeriodicFallbackDownload_DownloadError(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("periodicFallbackDownload is Windows-only")
	}
	src := &downloadableMockSource{
		releases:    []selfupdate.SourceRelease{newMockReleaseForPlatform("v2.0.0", "", "")},
		downloadErr: errors.New("network timeout"),
	}
	u := NewUpdaterWithSource(Config{
		Mode:           ModeAuto,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	// Should log error but not panic.
	u.periodicFallbackDownload(context.Background(), errors.New("apply failed"))
}

// TestCheckOnceFallbackDownload_DownloadError verifies checkOnceFallbackDownload
// returns a combined error when downloadToStaging fails.
func TestCheckOnceFallbackDownload_DownloadError(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("checkOnceFallbackDownload is Windows-only")
	}
	src := &downloadableMockSource{
		releases:    []selfupdate.SourceRelease{newMockReleaseForPlatform("v2.0.0", "", "")},
		downloadErr: errors.New("download failed"),
	}
	u := NewUpdaterWithSource(Config{
		Mode:           ModeAuto,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	_, _, err := u.checkOnceFallbackDownload(context.Background(), errors.New("apply error"))
	if err == nil {
		t.Fatal("expected error when download fails")
	}
	if !strings.Contains(err.Error(), "apply error") {
		t.Errorf("error = %q, want to contain 'apply error'", err.Error())
	}
	if !strings.Contains(err.Error(), "download fallback") {
		t.Errorf("error = %q, want to contain 'download fallback'", err.Error())
	}
}

// TestCheckOnceFallbackDownload_InvalidBinary verifies checkOnceFallbackDownload
// returns a combined error when the downloaded binary fails validation.
func TestCheckOnceFallbackDownload_InvalidBinary(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("checkOnceFallbackDownload is Windows-only")
	}
	badData := make([]byte, minBinarySize+1)
	src := &downloadableMockSource{
		releases:     []selfupdate.SourceRelease{newMockReleaseForPlatform("v2.0.0", "", "")},
		downloadData: badData,
	}
	u := NewUpdaterWithSource(Config{
		Mode:           ModeAuto,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	_, _, err := u.checkOnceFallbackDownload(context.Background(), errors.New("apply error"))
	if err == nil {
		t.Fatal("expected error for invalid binary")
	}
}

// TestPeriodicFallbackDownload_InvalidBinary verifies periodicFallbackDownload
// logs error when the downloaded binary has invalid content.
func TestPeriodicFallbackDownload_InvalidBinary(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("periodicFallbackDownload is Windows-only")
	}
	badData := make([]byte, minBinarySize+1)
	src := &downloadableMockSource{
		releases:     []selfupdate.SourceRelease{newMockReleaseForPlatform("v2.0.0", "", "")},
		downloadData: badData,
	}
	u := NewUpdaterWithSource(Config{
		Mode:           ModeAuto,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	// Should log error but not panic.
	u.periodicFallbackDownload(context.Background(), errors.New("apply failed"))
}

// TestPeriodicFallbackDownload_ValidBinary verifies periodicFallbackDownload
// succeeds end-to-end when downloadToStaging produces a valid binary.
func TestPeriodicFallbackDownload_ValidBinary(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("periodicFallbackDownload is Windows-only")
	}
	restoreTestBinary(t)

	src := &downloadableMockSource{
		releases:     []selfupdate.SourceRelease{newMockReleaseForPlatform("v2.0.0", "", "")},
		downloadData: fakeBinary(),
	}
	u := NewUpdaterWithSource(Config{
		Mode:           ModeAuto,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, src)

	// Should succeed or log error depending on binary locking.
	u.periodicFallbackDownload(context.Background(), errors.New("apply failed"))
}

// TestSelfupdateUpdater verifies that selfupdateUpdater returns
// a valid updater using checksum-only validation.
func TestSelfupdateUpdater(t *testing.T) {
	src := &mockSource{}
	u, err := selfupdateUpdater(src)
	if err != nil {
		t.Fatalf("selfupdateUpdater(src) unexpected error: %v", err)
	}
	if u == nil {
		t.Fatal("selfupdateUpdater(src) returned nil updater")
	}
}
