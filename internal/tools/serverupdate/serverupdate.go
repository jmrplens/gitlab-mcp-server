// Package serverupdate implements MCP tools for on-demand server update
// checks and manual update application.
package serverupdate

import (
	"context"
	"fmt"
	"runtime"

	"github.com/jmrplens/gitlab-mcp-server/internal/autoupdate"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// CheckInput is the (empty) input for the check-update tool.
type CheckInput struct{}

// ServerInfo holds static project metadata, set via SetServerInfo.
type ServerInfo struct {
	Author     string
	Department string
	Repository string
}

var serverInfo ServerInfo

// SetServerInfo configures the static metadata included in check results.
func SetServerInfo(info ServerInfo) {
	serverInfo = info
}

// CheckOutput describes the result of an update availability check.
type CheckOutput struct {
	toolutil.HintableOutput
	UpdateAvailable bool   `json:"update_available"`
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version,omitempty"`
	ReleaseURL      string `json:"release_url,omitempty"`
	ReleaseNotes    string `json:"release_notes,omitempty"`
	Mode            string `json:"mode"`
	Author          string `json:"author,omitempty"`
	Department      string `json:"department,omitempty"`
	Repository      string `json:"repository,omitempty"`
}

// ApplyInput is the (empty) input for the apply-update tool.
type ApplyInput struct{}

// ApplyOutput describes the result of applying an update.
type ApplyOutput struct {
	toolutil.HintableOutput
	Applied         bool   `json:"applied"`
	Deferred        bool   `json:"deferred,omitempty"`
	PreviousVersion string `json:"previous_version"`
	NewVersion      string `json:"new_version,omitempty"`
	StagingPath     string `json:"staging_path,omitempty"`
	ScriptPath      string `json:"script_path,omitempty"`
	Message         string `json:"message"`
}

// Check verifies whether a newer server version is available.
func Check(ctx context.Context, updater *autoupdate.Updater, _ CheckInput) (CheckOutput, error) {
	if err := ctx.Err(); err != nil {
		return CheckOutput{}, err
	}

	cfg := updater.GetConfig()
	out := CheckOutput{
		CurrentVersion: cfg.CurrentVersion,
		Mode:           string(cfg.Mode),
		Author:         serverInfo.Author,
		Department:     serverInfo.Department,
		Repository:     serverInfo.Repository,
	}

	info, available, err := updater.CheckForUpdate(ctx)
	if err != nil {
		return CheckOutput{}, fmt.Errorf("checking for update: %w", err)
	}

	out.UpdateAvailable = available
	if info != nil {
		out.LatestVersion = info.LatestVersion
		out.ReleaseURL = info.ReleaseURL
		out.ReleaseNotes = info.ReleaseNotes
	}

	return out, nil
}

// Apply downloads and applies the latest server update.
// On Windows, if the binary cannot be replaced (running exe lock),
// it falls back to downloading to a staging path with an update script.
func Apply(ctx context.Context, updater *autoupdate.Updater, _ ApplyInput) (ApplyOutput, error) {
	if err := ctx.Err(); err != nil {
		return ApplyOutput{}, err
	}

	cfg := updater.GetConfig()
	out := ApplyOutput{
		PreviousVersion: cfg.CurrentVersion,
	}

	newVersion, err := updater.ApplyUpdate(ctx)
	if err != nil {
		if runtime.GOOS == "windows" {
			return applyDeferredFallback(ctx, updater, out, err)
		}
		return ApplyOutput{}, fmt.Errorf("applying update: %w", err)
	}

	out.Applied = true
	out.NewVersion = newVersion
	out.Message = fmt.Sprintf("Updated from %s to %s. Restart the server to use the new version.", cfg.CurrentVersion, newVersion)

	return out, nil
}

// applyDeferredFallback handles the Windows case where ApplyUpdate fails.
// Downloads and replaces the binary using the rename trick.
func applyDeferredFallback(ctx context.Context, updater *autoupdate.Updater, out ApplyOutput, _ error) (ApplyOutput, error) {
	newVersion, err := updater.DownloadAndReplace(ctx)
	if err != nil {
		return ApplyOutput{}, fmt.Errorf("downloading update (Windows rename fallback): %w", err)
	}

	out.Applied = true
	out.NewVersion = newVersion
	out.Message = fmt.Sprintf(
		"Update %s downloaded and placed at the original binary path via rename trick. "+
			"Restart the server to use the new version.",
		newVersion)

	return out, nil
}
