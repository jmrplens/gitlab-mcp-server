package toolutil

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/progress"
)

// DefaultMaxFileSize re-exports the upload size limit from config as the single
// source of truth for tool utilities.
const (
	DefaultMaxFileSize = config.DefaultMaxFileSize
	// ImportArchiveAllowlistEnv names extra directories allowed for local
	// GitLab project/group import archives, separated by the OS path-list separator.
	ImportArchiveAllowlistEnv = "GITLAB_MCP_ALLOWED_IMPORT_DIRS"
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

// CanonicalImportArchivePath validates a local GitLab export archive path and
// returns the canonical path resolved through symlinks. Archives must be regular
// .tar.gz files under the current working directory, the OS temporary directory,
// or a directory listed in GITLAB_MCP_ALLOWED_IMPORT_DIRS.
func CanonicalImportArchivePath(path string) (string, error) {
	if path == "" {
		return "", errors.New("archive path is required")
	}

	absolutePath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve archive path: %w", err)
	}
	canonicalPath, err := filepath.EvalSymlinks(absolutePath)
	if err != nil {
		return "", fmt.Errorf("resolve archive symlinks: %w", err)
	}
	if !strings.HasSuffix(strings.ToLower(canonicalPath), ".tar.gz") {
		return "", fmt.Errorf("archive %s must use .tar.gz extension", canonicalPath)
	}

	info, err := os.Stat(canonicalPath)
	if err != nil {
		return "", fmt.Errorf("stat archive %s: %w", canonicalPath, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("archive %s is not a regular file", canonicalPath)
	}
	if archiveHasUnsafePermissions(info) {
		return "", fmt.Errorf("archive %s must not be group/world-writable", canonicalPath)
	}

	if !pathWithinAllowedImportDirs(canonicalPath) {
		return "", fmt.Errorf("archive %s is outside allowed import directories; use the current working directory, the OS temp directory, or set %s", canonicalPath, ImportArchiveAllowlistEnv)
	}
	return canonicalPath, nil
}

func archiveHasUnsafePermissions(info os.FileInfo) bool {
	return runtime.GOOS != "windows" && info.Mode().Perm()&0o022 != 0
}

func pathWithinAllowedImportDirs(canonicalPath string) bool {
	for _, base := range allowedImportArchiveDirs() {
		if pathWithinBase(canonicalPath, base) {
			return true
		}
	}
	return false
}

func allowedImportArchiveDirs() []string {
	type importDirEntry struct {
		path       string
		configured bool
	}
	dirs := []importDirEntry{}
	if cwd, err := os.Getwd(); err == nil {
		dirs = append(dirs, importDirEntry{path: cwd})
	}
	dirs = append(dirs, importDirEntry{path: os.TempDir()})
	if configured := os.Getenv(ImportArchiveAllowlistEnv); configured != "" {
		for _, dir := range filepath.SplitList(configured) {
			dirs = append(dirs, importDirEntry{path: dir, configured: true})
		}
	}

	allowed := make([]string, 0, len(dirs))
	seen := make(map[string]struct{}, len(dirs))
	for _, dir := range dirs {
		canonicalDir, err := canonicalDirPath(dir.path)
		if err != nil {
			if dir.configured {
				slog.Warn("skipping invalid import archive allowlist directory", "env", ImportArchiveAllowlistEnv, "path", dir.path, "error", err)
			}
			continue
		}
		if _, ok := seen[canonicalDir]; ok {
			continue
		}
		seen[canonicalDir] = struct{}{}
		allowed = append(allowed, canonicalDir)
	}
	return allowed
}

func canonicalDirPath(dir string) (string, error) {
	if dir == "" {
		return "", errors.New("empty directory")
	}
	absoluteDir, err := filepath.Abs(filepath.Clean(dir))
	if err != nil {
		return "", err
	}
	canonicalDir, err := filepath.EvalSymlinks(absoluteDir)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(canonicalDir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", canonicalDir)
	}
	return canonicalDir, nil
}

func pathWithinBase(path, base string) bool {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
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
