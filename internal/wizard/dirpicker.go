// dirpicker.go provides an interactive directory picker for selecting
// IDE configuration paths during wizard setup.

package wizard

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// pickDirectoryFn is the function used internally to pick a directory.
// Tests can swap this to prevent real OS dialogs.
var pickDirectoryFn = pickDirectory

// pickDirectory opens a native OS directory picker dialog and returns the selected path.
// Uses PowerShell FolderBrowserDialog on Windows, osascript on macOS, zenity/kdialog on Linux.
func pickDirectory(startDir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var cmd *exec.Cmd
	var err error

	switch runtime.GOOS {
	case "windows":
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
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-STA", "-Command", ps) // #nosec G204 -- trusted internal command with escaped directory path

	case "darwin":
		script := `POSIX path of (choose folder with prompt "Select installation directory")`
		if startDir != "" {
			script = fmt.Sprintf( //nolint:gocritic // AppleScript requires literal double quotes, %q would break syntax
				`POSIX path of (choose folder with prompt "Select installation directory" default location POSIX file "%s")`,
				strings.ReplaceAll(startDir, `"`, `\"`),
			)
		}
		cmd = exec.CommandContext(ctx, "osascript", "-e", script) // #nosec G204 -- trusted internal command with escaped directory path

	default: // Linux / FreeBSD
		// Try zenity first, fall back to kdialog
		if _, err = exec.LookPath("zenity"); err == nil {
			args := []string{"--file-selection", "--directory", "--title=Select installation directory"}
			if startDir != "" {
				args = append(args, "--filename="+startDir+"/")
			}
			cmd = exec.CommandContext(ctx, "zenity", args...) // #nosec G204 -- trusted internal command
		} else if _, err = exec.LookPath("kdialog"); err == nil {
			args := []string{"--getexistingdirectory"}
			if startDir != "" {
				args = append(args, startDir)
			} else {
				args = append(args, ".")
			}
			cmd = exec.CommandContext(ctx, "kdialog", args...) // #nosec G204 -- trusted internal command
		} else {
			return "", errors.New("no dialog tool available (install zenity or kdialog)")
		}
	}

	var out []byte
	out, err = cmd.Output()
	if err != nil {
		return "", fmt.Errorf("dialog cancelled or failed: %w", err)
	}

	selected := strings.TrimSpace(string(out))
	if selected == "" {
		return "", errors.New("no directory selected")
	}
	return selected, nil
}
