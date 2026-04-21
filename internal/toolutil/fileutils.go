// fileutils.go provides shared file utilities for upload and download
// operations: file validation, SHA-256 checksum computation, progress-reporting
// io.Reader/io.Writer wrappers, and GitLab package name validation.

package toolutil

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/internal/progress"
)

// Upload size defaults re-exported from config (single source of truth).
const (
	DefaultMaxFileSize = config.DefaultMaxFileSize
)

// UploadConfig holds runtime-configurable upload parameters. Initialized with
// package defaults; use SetUploadConfig to override from environment config.
type UploadConfig struct {
	MaxFileSize int64
}

// uploadCfg holds the active upload configuration. Package-level so handler
// closures can reference it without changing RegisterAll signatures.
// NOT safe for concurrent writes — must be set during init before any tool
// handlers run (i.e., before RegisterAll). Tests may call SetUploadConfig
// but must restore original values via defer.
var uploadCfg = UploadConfig{
	MaxFileSize: DefaultMaxFileSize,
}

// SetUploadConfig overrides the default upload thresholds. Call before
// RegisterAll to propagate values into tool handler closures.
func SetUploadConfig(maxFileSize int64) {
	uploadCfg = UploadConfig{
		MaxFileSize: maxFileSize,
	}
}

// GetUploadConfig returns the current upload configuration (for testing).
func GetUploadConfig() UploadConfig {
	return uploadCfg
}

// OpenAndValidateFile opens a local file for reading after validating it
// exists, is a regular file (not a directory, symlink, device or pipe), and
// does not exceed maxSize bytes. Returns the open file handle and its FileInfo.
func OpenAndValidateFile(path string, maxSize int64) (*os.File, os.FileInfo, error) {
	if path == "" {
		return nil, nil, errors.New("file path is required")
	}

	cleanPath := filepath.Clean(path)

	info, err := os.Stat(cleanPath)
	if err != nil {
		return nil, nil, fmt.Errorf("stat %s: %w", cleanPath, err)
	}

	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%s is not a regular file", cleanPath)
	}

	if maxSize > 0 && info.Size() > maxSize {
		return nil, nil, fmt.Errorf("file %s is %d bytes, exceeds maximum allowed size of %d bytes",
			cleanPath, info.Size(), maxSize)
	}

	f, err := os.Open(cleanPath) //#nosec G304 -- path is cleaned via filepath.Clean, validated as regular file with Stat, and size-checked before open
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", cleanPath, err)
	}

	return f, info, nil
}

// ComputeSHA256 computes the SHA-256 checksum of a file at the given path
// and returns the lowercase hex-encoded hash string.
func ComputeSHA256(path string) (string, error) {
	cleanPath := filepath.Clean(path)
	f, err := os.Open(cleanPath) //#nosec G304 -- path is cleaned via filepath.Clean; callers are internal (auto-update binary path from os.Executable), not user-controlled
	if err != nil {
		return "", fmt.Errorf("open for checksum %s: %w", cleanPath, err)
	}
	defer f.Close()

	return ComputeSHA256Reader(f)
}

// ComputeSHA256Reader computes the SHA-256 checksum from an arbitrary io.Reader
// and returns the lowercase hex-encoded hash string.
func ComputeSHA256Reader(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", fmt.Errorf("computing SHA-256: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ProgressReportInterval returns the byte interval between progress reports.
// It is the smaller of 1 MB or 5% of total, with a minimum of 64 KB.
func ProgressReportInterval(total int64) int64 {
	const oneMB = 1024 * 1024
	const minInterval = 64 * 1024

	fivePercent := total / 20
	interval := min(fivePercent, int64(oneMB))
	interval = max(interval, minInterval)
	return interval
}

// ProgressReader wraps an io.Reader and reports progress to an MCP progress
// tracker as bytes are read. Safe to use with a zero-value/inactive tracker.
type ProgressReader struct {
	inner      io.Reader
	onProgress func(read, total int64)
	read       int64
	total      int64
	lastReport int64
	interval   int64
}

// NewProgressReader creates a ProgressReader that reports upload progress.
// If the tracker is inactive, the wrapper still works but skips notifications.
func NewProgressReader(ctx context.Context, r io.Reader, total int64, tracker progress.Tracker) *ProgressReader {
	return &ProgressReader{
		inner: r,
		onProgress: func(read, total int64) {
			if !tracker.IsActive() {
				return
			}
			tracker.Update(ctx, float64(read), float64(total),
				fmt.Sprintf("Uploaded %d / %d bytes", read, total))
		},
		total:    total,
		interval: ProgressReportInterval(total),
	}
}

// BytesRead returns the total number of bytes read so far.
func (pr *ProgressReader) BytesRead() int64 { return pr.read }

// Read implements io.Reader. It reads from the inner reader and periodically
// sends progress notifications via the MCP tracker.
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.inner.Read(p)
	pr.read += int64(n)

	if pr.onProgress != nil && (pr.read-pr.lastReport >= pr.interval || err == io.EOF) {
		pr.onProgress(pr.read, pr.total)
		pr.lastReport = pr.read
	}

	return n, err
}

// ProgressWriter wraps an io.Writer and reports progress to an MCP progress
// tracker as bytes are written (used for downloads to disk).
type ProgressWriter struct {
	inner      io.Writer
	onProgress func(written, total int64)
	written    int64
	total      int64
	lastReport int64
	interval   int64
}

// NewProgressWriter creates a ProgressWriter that reports download progress.
func NewProgressWriter(ctx context.Context, w io.Writer, total int64, tracker progress.Tracker) *ProgressWriter {
	return &ProgressWriter{
		inner: w,
		onProgress: func(written, total int64) {
			if !tracker.IsActive() {
				return
			}
			tracker.Update(ctx, float64(written), float64(total),
				fmt.Sprintf("Downloaded %d / %d bytes", written, total))
		},
		total:    total,
		interval: ProgressReportInterval(total),
	}
}

// BytesWritten returns the total number of bytes written so far.
func (pw *ProgressWriter) BytesWritten() int64 { return pw.written }

// Write implements io.Writer. It writes to the inner writer and periodically
// sends progress notifications via the MCP tracker.
func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n, err := pw.inner.Write(p)
	pw.written += int64(n)

	if pw.onProgress != nil && (pw.written-pw.lastReport >= pw.interval || err != nil) {
		pw.onProgress(pw.written, pw.total)
		pw.lastReport = pw.written
	}

	return n, err
}

// packageNameRegex matches valid GitLab generic package names (letters, digits,
// dots, dashes, underscores, plus signs, tildes, slashes).
var packageNameRegex = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._\-+~/@]*$`)

// ValidatePackageName validates a GitLab generic package name against allowed
// characters. Names must start with a letter or digit and may contain
// A-Z a-z 0-9 . _ - + ~ / @.
func ValidatePackageName(name string) error {
	if name == "" {
		return errors.New("package name is required")
	}
	if !packageNameRegex.MatchString(name) {
		return fmt.Errorf("invalid package name %q: must start with a letter or digit and contain only A-Za-z0-9._-+~/@", name)
	}
	return nil
}

// ValidatePackageFileName validates a filename for GitLab generic package upload.
// Filenames must not be empty, must not contain spaces, and must not start
// with a tilde or at-sign.
func ValidatePackageFileName(filename string) error {
	if filename == "" {
		return errors.New("package file name is required")
	}
	if strings.Contains(filename, " ") {
		return fmt.Errorf("package file name %q must not contain spaces", filename)
	}
	if strings.HasPrefix(filename, "~") || strings.HasPrefix(filename, "@") {
		return fmt.Errorf("package file name %q must not start with ~ or @", filename)
	}
	return nil
}
