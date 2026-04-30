// autoupdate.go provides self-update capability for the gitlab-mcp-server binary.
// It wraps the creativeprojects/go-selfupdate library with a GitHub source
// to detect, download, validate, and apply new releases.
package autoupdate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/creativeprojects/go-selfupdate"
)

// envJustUpdated is set before re-exec to prevent infinite update loops.
const envJustUpdated = "MCP_JUST_UPDATED"

// resolveExecutable returns the path of the current running binary.
// It defaults to [os.Executable] and can be overridden in tests via
// stubExecutablePath to prevent tests from touching the production binary.
var resolveExecutable = os.Executable

// Default configuration values.
const (
	DefaultEnabled    = true
	DefaultRepository = "mcp/gitlab-mcp-server"
	DefaultInterval   = 1 * time.Hour
	DefaultMode       = ModeAuto
)

// Mode controls auto-update behavior.
type Mode string

const (
	// ModeAuto downloads and applies updates automatically.
	ModeAuto Mode = "true"
	// ModeCheck only checks for updates and logs the result without applying.
	ModeCheck Mode = "check"
	// ModeDisabled disables all update checks.
	ModeDisabled Mode = "false"
)

// ParseMode parses a string into an update [Mode].
// Accepted values: "true"/"1"/"yes" (auto), "check" (check-only),
// "false"/"0"/"no" (disabled). Empty defaults to [DefaultMode].
func ParseMode(s string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "true", "1", "yes":
		return ModeAuto, nil
	case "check":
		return ModeCheck, nil
	case "false", "0", "no":
		return ModeDisabled, nil
	default:
		return "", fmt.Errorf("invalid auto-update mode %q: expected true, check, or false", s)
	}
}

// Config holds auto-update configuration.
type Config struct {
	Mode           Mode          // Update behavior (auto, check, disabled)
	Repository     string        // GitHub repository slug (e.g. "jmrplens/gitlab-mcp-server")
	Interval       time.Duration // Check interval for HTTP mode periodic checks
	Timeout        time.Duration // Timeout for individual update checks (default 30s if zero)
	CurrentVersion string        // Running binary version (semver without "v" prefix)
}

// String returns a redacted representation of Config to prevent accidental
// token leakage via fmt.Print, log, or %v formatting.
func (c Config) String() string {
	return fmt.Sprintf("Config{Mode:%s Repository:%s Interval:%s Timeout:%s CurrentVersion:%s}",
		c.Mode, c.Repository, c.Interval, c.Timeout, c.CurrentVersion)
}

// GoString implements [fmt.GoStringer] to prevent token leakage via %#v formatting.
func (c Config) GoString() string { return c.String() }

// UpdateInfo describes an available release.
type UpdateInfo struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	ReleaseURL     string `json:"release_url,omitempty"`
	ReleaseNotes   string `json:"release_notes,omitempty"`
	PublishedAt    string `json:"published_at,omitempty"`
}

// Updater manages update detection and application.
type Updater struct {
	cfg    Config
	source selfupdate.Source
}

// NewUpdater creates an [Updater] for the given configuration.
// Returns an error if required fields are missing or a GitHub source cannot
// be created.
func NewUpdater(cfg Config) (*Updater, error) {
	if cfg.Repository == "" {
		return nil, errors.New("autoupdate: repository is required")
	}
	if cfg.CurrentVersion == "" || cfg.CurrentVersion == "dev" {
		return nil, errors.New("autoupdate: current version is required (binary built without -ldflags?)")
	}
	if cfg.Mode == "" {
		cfg.Mode = DefaultMode
	}
	if cfg.Interval <= 0 {
		cfg.Interval = DefaultInterval
	}

	src, err := newGitHubSource()
	if err != nil {
		return nil, fmt.Errorf("autoupdate: creating GitHub source: %w", err)
	}

	return &Updater{cfg: cfg, source: src}, nil
}

// NewUpdaterWithSource creates an Updater with an injected source (for testing).
func NewUpdaterWithSource(cfg Config, src selfupdate.Source) *Updater {
	if cfg.Mode == "" {
		cfg.Mode = DefaultMode
	}
	if cfg.Interval <= 0 {
		cfg.Interval = DefaultInterval
	}
	return &Updater{cfg: cfg, source: src}
}

// splitRepository splits a full GitLab project path into owner and repo.
// For nested groups like "mcp/gitlab-mcp-server", owner is
// "mcp" and repo is "gitlab-mcp-server".
func splitRepository(path string) (owner, repo string, err error) {
	path = strings.Trim(path, "/")
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return "", "", fmt.Errorf("autoupdate: invalid repository path %q: expected owner/repo format", path)
	}
	return path[:idx], path[idx+1:], nil
}

// selfupdateUpdater creates a go-selfupdate Updater with checksum validation.
func selfupdateUpdater(src selfupdate.Source) (*selfupdate.Updater, error) {
	u, err := selfupdate.NewUpdater(selfupdate.Config{
		Source:    src,
		Validator: &selfupdate.ChecksumValidator{UniqueFilename: "checksums.txt"},
	})
	if err != nil {
		return nil, fmt.Errorf("autoupdate: creating updater: %w", err)
	}
	return u, nil
}

// selfupdateSlug creates a RepositorySlug from owner and repo strings.
func selfupdateSlug(owner, repo string) selfupdate.RepositorySlug {
	return selfupdate.NewRepositorySlug(owner, repo)
}

// CheckForUpdate checks the GitLab releases for a newer version.
// Returns update information and whether an update is available.
func (u *Updater) CheckForUpdate(ctx context.Context) (*UpdateInfo, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}

	owner, repo, err := splitRepository(u.cfg.Repository)
	if err != nil {
		return nil, false, err
	}

	updater, err := selfupdateUpdater(u.source)
	if err != nil {
		return nil, false, err
	}

	latest, found, err := updater.DetectLatest(ctx, selfupdateSlug(owner, repo))
	if err != nil {
		return nil, false, fmt.Errorf("autoupdate: detecting latest release: %w", err)
	}

	info := &UpdateInfo{
		CurrentVersion: u.cfg.CurrentVersion,
	}

	if !found {
		return info, false, nil
	}

	info.LatestVersion = latest.Version()
	info.ReleaseURL = latest.URL
	info.ReleaseNotes = latest.ReleaseNotes

	if !latest.GreaterThan(u.cfg.CurrentVersion) {
		return info, false, nil
	}

	return info, true, nil
}

// ApplyUpdate downloads and applies the latest release, replacing the
// running binary. Returns the new version on success.
func (u *Updater) ApplyUpdate(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	owner, repo, err := splitRepository(u.cfg.Repository)
	if err != nil {
		return "", err
	}

	updater, err := selfupdateUpdater(u.source)
	if err != nil {
		return "", err
	}

	latest, err := updater.UpdateSelf(ctx, u.cfg.CurrentVersion, selfupdateSlug(owner, repo))
	if err != nil {
		return "", fmt.Errorf("autoupdate: applying update: %w", err)
	}

	return latest.Version(), nil
}

// StartPeriodicCheck launches a background goroutine that checks for updates
// at the configured interval. Cancel the context to stop.
// In [ModeAuto], detected updates are applied automatically.
// In [ModeCheck], updates are only logged.
func (u *Updater) StartPeriodicCheck(ctx context.Context) {
	slog.Info("autoupdate: starting periodic check",
		"interval", u.cfg.Interval,
		"mode", u.cfg.Mode,
		"repository", u.cfg.Repository,
		"current_version", u.cfg.CurrentVersion,
	)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("autoupdate: periodic check goroutine panicked — update checks disabled", "panic", r)
			}
		}()

		ticker := time.NewTicker(u.cfg.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.Info("autoupdate: periodic check stopped")
				return
			case <-ticker.C:
				u.safePeriodicCheckOnce(ctx)
			}
		}
	}()
}

// safePeriodicCheckOnce wraps periodicCheckOnce with panic recovery so that
// a single failed check does not crash the background goroutine.
func (u *Updater) safePeriodicCheckOnce(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("autoupdate: periodic check panicked — skipping this cycle", "panic", r)
		}
	}()
	u.periodicCheckOnce(ctx)
}

// periodicCheckOnce performs a single update check cycle, logging the result.
func (u *Updater) periodicCheckOnce(ctx context.Context) {
	timeout := u.cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	info, available, err := u.CheckForUpdate(checkCtx)
	if err != nil {
		slog.Warn("autoupdate: periodic check failed", "error", err)
		return
	}

	if !available {
		slog.Debug("autoupdate: no update available", "current_version", u.cfg.CurrentVersion)
		return
	}

	slog.Info("autoupdate: new version available",
		"current_version", info.CurrentVersion,
		"latest_version", info.LatestVersion,
	)

	if u.cfg.Mode == ModeCheck {
		slog.Info("autoupdate: check-only mode — skipping apply",
			"latest_version", info.LatestVersion,
		)
		return
	}

	newVersion, err := u.ApplyUpdate(checkCtx)
	if err != nil {
		if runtime.GOOS == "windows" {
			u.periodicFallbackDownload(checkCtx, err)
			return
		}
		slog.Error("autoupdate: failed to apply update", "error", err)
		return
	}

	slog.Info("autoupdate: update applied — restart the server to use the new version",
		"new_version", newVersion,
	)
}

// CheckOnce runs a single update check (for stdio mode startup).
// In [ModeAuto], if an update is found it is applied and the function
// returns the new version (the caller should re-exec the process).
// In [ModeCheck], only a log message is emitted.
func (u *Updater) CheckOnce(ctx context.Context) (newVersion string, updated bool, err error) {
	info, available, err := u.CheckForUpdate(ctx)
	if err != nil {
		return "", false, err
	}

	if !available {
		slog.Info("autoupdate: server is up to date",
			"version", u.cfg.CurrentVersion,
		)
		return "", false, nil
	}

	slog.Info("autoupdate: new version available",
		"current_version", info.CurrentVersion,
		"latest_version", info.LatestVersion,
	)

	if u.cfg.Mode == ModeCheck {
		slog.Warn("autoupdate: check-only mode — update available but not applying",
			"latest_version", info.LatestVersion,
		)
		return info.LatestVersion, false, nil
	}

	v, err := u.ApplyUpdate(ctx)
	if err != nil {
		if runtime.GOOS == "windows" {
			return u.checkOnceFallbackDownload(ctx, err)
		}
		return "", false, err
	}

	slog.Info("autoupdate: update applied", "new_version", v)
	return v, true, nil
}

// IsEnabled reports whether auto-update checks are active (mode is not disabled).
func (u *Updater) IsEnabled() bool {
	return u.cfg.Mode != ModeDisabled
}

// GetConfig returns the updater configuration.
func (u *Updater) GetConfig() Config {
	return u.cfg
}

// checkOnceFallbackDownload is the Windows fallback for CheckOnce.
// When ApplyUpdate fails (binary locked), it downloads and replaces
// the binary using the rename trick.
func (u *Updater) checkOnceFallbackDownload(ctx context.Context, applyErr error) (newVersion string, applied bool, err error) {
	v, tmpPath, dlErr := u.downloadToStaging(ctx)
	if dlErr != nil {
		return "", false, fmt.Errorf("applying update: %w (download fallback: %w)", applyErr, dlErr)
	}
	if replErr := replaceExecutable(tmpPath); replErr != nil {
		_ = os.Remove(tmpPath)
		return "", false, fmt.Errorf("applying update: %w (rename fallback: %w)", applyErr, replErr)
	}
	slog.Info("autoupdate: binary updated via rename trick (will take effect on next restart)",
		"new_version", v,
	)
	return v, true, nil
}

// periodicFallbackDownload is the Windows fallback for periodicCheckOnce.
// When ApplyUpdate fails (binary locked), it downloads and replaces
// the binary using the rename trick.
func (u *Updater) periodicFallbackDownload(ctx context.Context, applyErr error) {
	v, tmpPath, dlErr := u.downloadToStaging(ctx)
	if dlErr != nil {
		slog.Error("autoupdate: failed to apply and download update",
			"apply_error", applyErr,
			"download_error", dlErr,
		)
		return
	}
	if replErr := replaceExecutable(tmpPath); replErr != nil {
		_ = os.Remove(tmpPath)
		slog.Error("autoupdate: failed to apply and rename update",
			"apply_error", applyErr,
			"rename_error", replErr,
		)
		return
	}
	slog.Info("autoupdate: binary updated via rename trick (will take effect on next restart)",
		"new_version", v,
	)
}

// CleanupOldBinary removes leftover .old files from a previous rename-based
// update. Safe to call unconditionally at startup; errors are logged and
// do not prevent the server from starting.
func CleanupOldBinary() {
	exe, err := resolveExecutable()
	if err != nil {
		return
	}
	exe, _ = filepath.EvalSymlinks(exe)
	oldPath := exe + ".old"
	if _, err = os.Stat(oldPath); err != nil {
		return
	}
	if err = os.Remove(oldPath); err != nil {
		slog.Debug("autoupdate: could not remove old binary", "path", oldPath, "error", err)
		return
	}
	slog.Info("autoupdate: removed old binary from previous update", "path", oldPath)
}

// JustUpdated reports whether the process was re-executed after an update.
func JustUpdated() bool {
	return os.Getenv(envJustUpdated) == "1"
}

// SetJustUpdated sets the environment variable that prevents re-exec loops.
func SetJustUpdated() error {
	return os.Setenv(envJustUpdated, "1")
}

// ClearJustUpdated removes the re-exec guard variable.
func ClearJustUpdated() {
	_ = os.Unsetenv(envJustUpdated)
}

// DownloadAndReplace downloads the latest release and replaces the current
// binary using the rename trick. Returns the new version. This is the
// public entry point for MCP tools that need to trigger an update.
func (u *Updater) DownloadAndReplace(ctx context.Context) (string, error) {
	v, tmpPath, err := u.downloadToStaging(ctx)
	if err != nil {
		return "", err
	}
	if err = replaceExecutable(tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	return v, nil
}
