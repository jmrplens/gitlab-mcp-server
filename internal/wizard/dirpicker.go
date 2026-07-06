package wizard

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var linuxDialogToolDirs = []string{"/usr/bin", "/bin"}

var findLinuxDialogToolPath = fixedLinuxDialogToolPath

// pickDirectoryFn is the function used internally to pick a directory.
// Tests can swap this to prevent real OS dialogs.
var pickDirectoryFn = pickDirectory

// pickDirectory opens a native OS directory picker dialog and returns the selected path.
// Uses PowerShell FolderBrowserDialog on Windows, osascript on macOS, zenity/kdialog on Linux.
func pickDirectory(startDir string) (string, error) {
	return pickDirectoryOn(runtime.GOOS, startDir)
}

// pickDirectoryOn is the GOOS-parameterized implementation of pickDirectory,
// extracted so the per-platform dispatch and dialog output handling are
// testable on any host.
func pickDirectoryOn(goos, startDir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd, err := directoryPickerCommand(ctx, goos, startDir)
	if err != nil {
		return "", errors.New("no dialog tool available (install zenity or kdialog)")
	}

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("dialog cancelled or failed: %w", err)
	}

	selected := strings.TrimSpace(string(out))
	if selected == "" {
		return "", errors.New("no directory selected")
	}
	return selected, nil
}

// directoryPickerCommand selects the platform-specific directory picker
// command builder for the given GOOS.
func directoryPickerCommand(ctx context.Context, goos, startDir string) (*exec.Cmd, error) {
	switch goos {
	case "windows":
		return windowsDirectoryPickerCommand(ctx, startDir), nil
	case "darwin":
		return darwinDirectoryPickerCommand(ctx, startDir), nil
	default: // Linux / FreeBSD
		return linuxDirectoryPickerCommand(ctx, startDir)
	}
}

// windowsDirectoryPickerCommand builds the PowerShell command that opens the
// native Windows FolderBrowserDialog.
func windowsDirectoryPickerCommand(ctx context.Context, startDir string) *exec.Cmd {
	// PowerShell script to open native Windows FolderBrowserDialog.
	// -STA is required for WinForms dialogs; a hidden topmost form ensures
	// the dialog appears in front of the browser window.
	escaped := strings.ReplaceAll(startDir, "'", "''")
	ps := fmt.Sprintf(
		`Add-Type -AssemblyName System.Windows.Forms; `+
			`$f = New-Object System.Windows.Forms.Form; `+
			`$f.TopMost = $true; `+
			`$f.WindowState = 'Minimized'; `+
			`$f.ShowInTaskbar = $false; `+
			`$d = New-Object System.Windows.Forms.FolderBrowserDialog; `+
			`$d.Description = 'Select installation directory'; `+
			`$d.ShowNewFolderButton = $true; `+
			`if ('%s' -ne '') { $d.SelectedPath = '%s' }; `+
			`if ($d.ShowDialog($f) -eq 'OK') { $d.SelectedPath }; `+
			`$f.Dispose()`,
		escaped, escaped,
	)
	return exec.CommandContext(ctx, "powershell", "-NoProfile", "-STA", "-Command", ps) // #nosec G204 -- trusted internal command with escaped directory path
}

// darwinDirectoryPickerCommand builds the osascript command that opens the
// native macOS folder chooser.
func darwinDirectoryPickerCommand(ctx context.Context, startDir string) *exec.Cmd {
	script := `POSIX path of (choose folder with prompt "Select installation directory")`
	if startDir != "" {
		script = fmt.Sprintf( //nolint:gocritic // AppleScript requires literal double quotes, %q would break syntax
			`POSIX path of (choose folder with prompt "Select installation directory" default location POSIX file "%s")`,
			strings.ReplaceAll(startDir, `"`, `\"`),
		)
	}
	return exec.CommandContext(ctx, "osascript", "-e", script) // #nosec G204 -- trusted internal command with escaped directory path
}

func linuxDirectoryPickerCommand(ctx context.Context, startDir string) (*exec.Cmd, error) {
	if zenityPath, ok := findLinuxDialogToolPath("zenity"); ok {
		args := []string{"--file-selection", "--directory", "--title=Select installation directory"}
		if startDir != "" {
			args = append(args, "--filename="+startDir+"/")
		}
		return exec.CommandContext(ctx, zenityPath, args...), nil // #nosec G204 -- executable resolved from fixed system directories
	}
	kdialogPath, ok := findLinuxDialogToolPath("kdialog")
	if !ok {
		return nil, errors.New("no fixed dialog tool found")
	}
	args := []string{"--getexistingdirectory"}
	if startDir != "" {
		args = append(args, startDir)
	} else {
		args = append(args, ".")
	}
	return exec.CommandContext(ctx, kdialogPath, args...), nil // #nosec G204 -- executable resolved from fixed system directories
}

func fixedLinuxDialogToolPath(name string) (string, bool) {
	for _, dir := range linuxDialogToolDirs {
		if !isFixedSystemDir(dir) {
			continue
		}
		path := filepath.Join(dir, name)
		if isExecutableFile(path) {
			return path, true
		}
	}
	return "", false
}

func isFixedSystemDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	return info.Mode().Perm()&0o022 == 0
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode().Perm()&0o111 != 0
}
