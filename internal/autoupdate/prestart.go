package autoupdate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
)

// execSelf is the function that replaces the current process with a new
// instance. It defaults to [ExecSelf] and can be overridden in tests to
// simulate exec failures without requiring syscall.Exec.
var execSelf = ExecSelf

// PreStartResult describes what happened during the pre-start update check.
type PreStartResult struct {
	Updated    bool   // Whether a new binary was placed on disk
	NewVersion string // Version of the downloaded update (empty if none)
	ExecFailed bool   // True if exec was attempted but failed (Unix only)
}

// PreStartUpdate checks for an update and, if available, downloads the new
// binary and replaces the current executable using the rename trick.
//
// On Unix, if the replacement succeeds, it calls [ExecSelf] to re-exec the
// process with the new binary. ExecSelf does not return on success; if it
// fails, ExecFailed is set and the server continues with the old code.
//
// On Windows, the rename trick places the new binary at the original path.
// ExecSelf is not supported, so the new version takes effect on next restart.
//
// The function is guarded by [envJustUpdated] to prevent infinite re-exec loops.
func PreStartUpdate(ctx context.Context, cfg Config) (result *PreStartResult) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("autoupdate: pre-start check panicked — continuing without update", "panic", r)
			result = &PreStartResult{}
		}
	}()

	if JustUpdated() {
		ClearJustUpdated()
		slog.Info("autoupdate: skipping update check (just re-executed after update)")
		return &PreStartResult{}
	}

	if cfg.Mode == ModeDisabled {
		return &PreStartResult{}
	}

	u, err := NewUpdater(cfg)
	if err != nil {
		slog.Warn("autoupdate: could not initialize updater for pre-start check", "error", err)
		return &PreStartResult{}
	}

	info, available, err := u.CheckForUpdate(ctx)
	if err != nil {
		slog.Warn("autoupdate: pre-start check failed", "error", err)
		return &PreStartResult{}
	}

	if !available {
		slog.Info("autoupdate: server is up to date", "version", cfg.CurrentVersion)
		return &PreStartResult{}
	}

	slog.Info("autoupdate: new version available",
		"current_version", info.CurrentVersion,
		"latest_version", info.LatestVersion,
	)

	if cfg.Mode == ModeCheck {
		slog.Warn("autoupdate: check-only mode — update available but not applying",
			"latest_version", info.LatestVersion,
		)
		return &PreStartResult{NewVersion: info.LatestVersion}
	}

	// Download new binary to a temporary file.
	newVersion, tmpPath, err := u.downloadToStaging(ctx)
	if err != nil {
		slog.Warn("autoupdate: failed to download update", "error", err)
		return &PreStartResult{}
	}

	// Replace the current binary using rename trick.
	if err = replaceExecutable(tmpPath); err != nil {
		slog.Error("autoupdate: failed to replace binary", "error", err)
		_ = os.Remove(tmpPath)
		return &PreStartResult{}
	}

	slog.Info("autoupdate: binary updated on disk", "new_version", newVersion)

	// On Unix, re-exec so the new binary runs. On Windows, log that
	// the update takes effect on next restart.
	if runtime.GOOS != "windows" {
		if err = SetJustUpdated(); err != nil {
			slog.Error("autoupdate: could not set re-exec guard", "error", err)
			return &PreStartResult{Updated: true, NewVersion: newVersion}
		}
		slog.Info("autoupdate: re-executing with new binary", "new_version", newVersion)
		if err = execSelf(); err != nil {
			slog.Error("autoupdate: exec-self failed — continuing with old code", "error", err)
			ClearJustUpdated()
			return &PreStartResult{Updated: true, NewVersion: newVersion, ExecFailed: true}
		}
		// ExecSelf does not return on success.
	}

	slog.Info("autoupdate: update will take effect on next restart", "new_version", newVersion)
	return &PreStartResult{Updated: true, NewVersion: newVersion}
}

// downloadToStaging downloads the latest release asset to a temporary file
// in the same directory as the executable. Returns the new version string
// and the path to the temporary file.
func (u *Updater) downloadToStaging(ctx context.Context) (version, tmpPath string, err error) {
	owner, repo, splitErr := splitRepository(u.cfg.Repository)
	if splitErr != nil {
		return "", "", splitErr
	}

	updater, err := selfupdateUpdater(u.source)
	if err != nil {
		return "", "", err
	}

	latest, found, err := updater.DetectLatest(ctx, selfupdateSlug(owner, repo))
	if err != nil {
		return "", "", fmt.Errorf("autoupdate: detecting latest release: %w", err)
	}
	if !found || !latest.GreaterThan(u.cfg.CurrentVersion) {
		return "", "", errors.New("autoupdate: no newer version found")
	}

	body, err := u.source.DownloadReleaseAsset(ctx, latest, latest.AssetID)
	if err != nil {
		return "", "", fmt.Errorf("autoupdate: downloading release asset: %w", err)
	}
	defer body.Close()

	exe, err := resolveExecutable()
	if err != nil {
		return "", "", fmt.Errorf("autoupdate: resolving executable: %w", err)
	}
	exe, _ = filepath.EvalSymlinks(exe)

	tmpPath = exe + ".tmp"
	if err = writeToFile(tmpPath, body); err != nil {
		return "", "", err
	}

	return latest.Version(), tmpPath, nil
}

// replaceExecutable puts the staged binary at the current executable path
// using the rename trick: rename current → .old, rename tmp → current.
func replaceExecutable(tmpPath string) error {
	exe, err := resolveExecutable()
	if err != nil {
		return fmt.Errorf("autoupdate: resolving executable: %w", err)
	}
	exe, _ = filepath.EvalSymlinks(exe)

	oldPath := exe + ".old"

	// Remove a leftover .old from a previous update (best-effort).
	_ = os.Remove(oldPath)

	// Rename the running binary to .old.
	if err = os.Rename(exe, oldPath); err != nil {
		return fmt.Errorf("autoupdate: renaming current binary: %w", err)
	}

	// Move the downloaded binary to the original path.
	if err = os.Rename(tmpPath, exe); err != nil {
		// Rollback: restore the original binary.
		if rbErr := os.Rename(oldPath, exe); rbErr != nil {
			slog.Error("autoupdate: rollback failed", "rename_error", err, "rollback_error", rbErr)
		}
		return fmt.Errorf("autoupdate: placing new binary: %w", err)
	}

	return nil
}
