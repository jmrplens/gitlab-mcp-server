// deferred.go provides file-level helpers for the autoupdate package:
// writing binary data to files and detecting pending staged updates.

package autoupdate

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

const minBinarySize = 1024 * 1024 // 1 MB — any legitimate binary is larger

// errNotBinary is returned when the downloaded content does not look like
// an executable binary (e.g. a JSON error page was served instead).
var errNotBinary = errors.New("autoupdate: downloaded file is not a valid executable")

// writeToFile creates a file at path and copies all data from r into it.
// After writing, it validates that the file looks like an executable binary.
// On Unix, the file is made executable (0o755).
func writeToFile(path string, r io.Reader) error {
	f, err := os.Create(path) //#nosec G304 -- path derived from os.Executable
	if err != nil {
		return fmt.Errorf("autoupdate: creating staging file %s: %w", path, err)
	}
	n, err := io.Copy(f, r)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return fmt.Errorf("autoupdate: writing staging file: %w", err)
	}
	if err = f.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("autoupdate: closing staging file: %w", err)
	}
	if n < minBinarySize {
		_ = os.Remove(path)
		return fmt.Errorf("%w: size %d bytes (minimum %d)", errNotBinary, n, minBinarySize)
	}
	if err = validateBinaryMagic(path); err != nil {
		_ = os.Remove(path)
		return err
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(path, 0o755) //#nosec G302 -- executable binary needs 0755
	}
	return nil
}

// validateBinaryMagic reads the first bytes of path and checks for known
// executable magic numbers (ELF, Mach-O, PE/MZ).
func validateBinaryMagic(path string) error {
	f, err := os.Open(path) //#nosec G304 -- path derived from os.Executable
	if err != nil {
		return fmt.Errorf("autoupdate: opening staged file for validation: %w", err)
	}
	defer f.Close()

	header := make([]byte, 4)
	if _, err = io.ReadFull(f, header); err != nil {
		return fmt.Errorf("%w: cannot read header", errNotBinary)
	}

	switch {
	case header[0] == 0x7f && header[1] == 'E' && header[2] == 'L' && header[3] == 'F': // ELF
		return nil
	case header[0] == 0xCF && header[1] == 0xFA && header[2] == 0xED && header[3] == 0xFE: // Mach-O 64-bit
		return nil
	case header[0] == 0xFE && header[1] == 0xED && header[2] == 0xFA && header[3] == 0xCF: // Mach-O 64-bit (big-endian)
		return nil
	case header[0] == 0xCA && header[1] == 0xFE && header[2] == 0xBA && header[3] == 0xBE: // Mach-O universal
		return nil
	case header[0] == 'M' && header[1] == 'Z': // PE (Windows)
		return nil
	default:
		return fmt.Errorf("%w: unrecognized magic bytes %x", errNotBinary, header)
	}
}

// HasPendingUpdate reports whether a previous update left a staged
// binary (.tmp or .new) waiting to be applied.
func HasPendingUpdate() (stagingPath string, ok bool) {
	exePath, err := resolveExecutable()
	if err != nil {
		return "", false
	}
	exePath, _ = filepath.EvalSymlinks(exePath)

	for _, suffix := range []string{".tmp", ".new"} {
		staging := exePath + suffix
		if _, err = os.Stat(staging); err == nil {
			return staging, true
		}
	}
	return "", false
}
