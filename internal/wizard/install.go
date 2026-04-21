package wizard

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// installBinaryFn is the function used to install the binary. Defaults to
// installBinaryImpl. Tests override it via stubInstallBinary to avoid
// writing to the real install directory.
var installBinaryFn = installBinaryImpl

// InstallBinary copies the currently running binary to destDir.
// Returns the full path of the installed binary. Skips copy if
// the source and destination resolve to the same path.
func InstallBinary(destDir string) (string, error) {
	return installBinaryFn(destDir)
}

func installBinaryImpl(destDir string) (string, error) {
	srcPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("getting executable path: %w", err)
	}
	srcPath, err = filepath.EvalSymlinks(srcPath)
	if err != nil {
		return "", fmt.Errorf("resolving executable path: %w", err)
	}

	destPath := filepath.Join(destDir, DefaultBinaryName())
	destResolved, _ := filepath.EvalSymlinks(destPath)
	if destResolved == "" {
		destResolved = destPath
	}

	if srcPath == destResolved {
		return destPath, nil
	}

	if err = os.MkdirAll(destDir, 0o755); err != nil { // #nosec G301 -- install dir needs execute permission
		return "", fmt.Errorf("creating directory %s: %w", destDir, err)
	}

	if err = copyFile(srcPath, destPath); err != nil {
		return "", fmt.Errorf("copying binary: %w", err)
	}

	if runtime.GOOS != "windows" {
		if err = os.Chmod(destPath, 0o700); err != nil { // #nosec G302 -- binary needs owner-only execute permission
			return "", fmt.Errorf("setting permissions: %w", err)
		}
	}

	return destPath, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src) // #nosec G304 -- src is the running binary path, not user input
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst) // #nosec G304 -- dst is a constructed path within the install directory
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
