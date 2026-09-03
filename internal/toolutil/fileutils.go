package toolutil

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"

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
	// UploadDirAllowlistEnv names extra directories a tool may READ a local
	// file from (every file_path input), separated by the OS path-list
	// separator. The working directory and the OS temp directory are always
	// allowed.
	UploadDirAllowlistEnv = "GITLAB_MCP_ALLOWED_UPLOAD_DIRS"
	// DownloadDirAllowlistEnv names extra directories a tool may WRITE a
	// downloaded file into (output_path), separated by the OS path-list
	// separator. The working directory and the OS temp directory are always
	// allowed.
	DownloadDirAllowlistEnv = "GITLAB_MCP_ALLOWED_DOWNLOAD_DIRS"
)

// localFilesystemAllowed reports whether tool handlers may name paths on the
// machine the server runs on. It is the transport distinction, not a knob: a
// stdio server runs on the same machine as the person driving it, and naming a
// file by path is the whole point of file_path; a server reached over HTTP is
// talking to somebody who has no files here, so every path they can name
// belongs to someone else. See [SetLocalFilesystemAccess].
var localFilesystemAllowed atomic.Bool

func init() {
	localFilesystemAllowed.Store(!httpTransportConfigured(os.Args))
}

// SetLocalFilesystemAccess overrides the transport inference for this process:
// pass false to refuse every caller-supplied local path, true to allow the
// allow-listed roots. Call it before any tool handler runs.
//
// The default is inferred from the process arguments rather than configured,
// so a deployment that never heard of this policy still gets the right answer:
// an operator who forgets a flag would otherwise be the one running the
// exposed server. An explicit call always wins over the inference.
func SetLocalFilesystemAccess(allowed bool) { localFilesystemAllowed.Store(allowed) }

// LocalFilesystemAccessAllowed reports whether caller-supplied local paths are
// honored in this process.
func LocalFilesystemAccessAllowed() bool { return localFilesystemAllowed.Load() }

// httpTransportConfigured reports whether args start this binary in HTTP mode,
// recognizing every spelling of the boolean --http flag that Go's flag package
// accepts and stopping at the -- terminator, as flag parsing does.
func httpTransportConfigured(args []string) bool {
	if len(args) == 0 {
		return false
	}
	for _, arg := range args[1:] {
		if arg == "--" {
			return false
		}
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-"), "=")
		if name != "http" {
			continue
		}
		if !hasValue {
			return true
		}
		enabled, err := strconv.ParseBool(value)
		return err == nil && enabled
	}
	return false
}

// requireLocalFilesystemAccess refuses a caller-supplied local path when this
// process is serving a remote transport. alternative names the input a remote
// caller should use instead, or is empty when the action has none.
func requireLocalFilesystemAccess(what, alternative string) error {
	if localFilesystemAllowed.Load() {
		return nil
	}
	if alternative != "" {
		return fmt.Errorf("%s is disabled when the server is reached over HTTP: the file would be read from the server's own disk, not yours; send the bytes with %s instead", what, alternative)
	}
	return fmt.Errorf("%s is disabled when the server is reached over HTTP: it names a path on the server's own disk, not yours", what)
}

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

// OpenAndValidateFile opens a caller-supplied local file for reading after
// confining it to the allowed upload directories, resolving it through every
// symlink on the way, and validating that what the resolved path names is a
// regular file of at most maxSize bytes. Returns the open file handle and its
// FileInfo.
//
// The containment is the point: file_path names a path on the machine the
// server runs on, and the caller — a model following an instruction that may
// have come from an issue description or a job log — is not the person whose
// files these are. See [CanonicalLocalFilePath] for the roots.
func OpenAndValidateFile(path string, maxSize int64) (*os.File, os.FileInfo, error) {
	canonicalPath, err := CanonicalLocalFilePath(path)
	if err != nil {
		return nil, nil, err
	}

	// Lstat, not Stat: canonicalPath is already symlink-free, so the two agree
	// unless the leaf became a symlink between resolution and here, and in
	// that race Lstat is the answer that refuses.
	info, err := os.Lstat(canonicalPath)
	if err != nil {
		return nil, nil, fmt.Errorf("stat %s: %w", canonicalPath, err)
	}

	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%s is not a regular file", canonicalPath)
	}

	if maxSize > 0 && info.Size() > maxSize {
		return nil, nil, fmt.Errorf("file %s is %d bytes, exceeds maximum allowed size of %d bytes",
			canonicalPath, info.Size(), maxSize)
	}

	f, err := openLeafNoFollow(canonicalPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", canonicalPath, err)
	}

	// The checks above ran on a path; these run on the descriptor, which is
	// the only thing that can still be read from. Between the Lstat and the
	// open the leaf may have been replaced, and a swap the open itself could
	// not refuse is caught here instead.
	opened, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, fmt.Errorf("stat %s: %w", canonicalPath, err)
	}
	if !opened.Mode().IsRegular() {
		_ = f.Close()
		return nil, nil, fmt.Errorf("%s is not a regular file", canonicalPath)
	}
	if maxSize > 0 && opened.Size() > maxSize {
		_ = f.Close()
		return nil, nil, fmt.Errorf("file %s is %d bytes, exceeds maximum allowed size of %d bytes",
			canonicalPath, opened.Size(), maxSize)
	}

	return f, opened, nil
}

// CreateDownloadOutputFile creates the destination a download writes to,
// refusing a symlink at the leaf where the platform can.
//
// [CanonicalDownloadOutputPath] refuses a destination that is already a
// symlink, but it refuses a path, and the file is created by a later syscall:
// a local principal who can write in an allowed root can put a symlink there
// in between and redirect the write to whatever the server may overwrite. The
// creation is the only place that race can be closed, so it happens here
// rather than at the call site.
func CreateDownloadOutputFile(path string) (*os.File, error) {
	return createLeafNoFollow(path)
}

// CanonicalLocalFilePath resolves a caller-supplied path to an existing local
// file and returns it canonicalized, provided the resolved path lies under the
// working directory, the OS temporary directory, or a directory listed in
// GITLAB_MCP_ALLOWED_UPLOAD_DIRS. It refuses every path when the server is
// reached over HTTP.
func CanonicalLocalFilePath(path string) (string, error) {
	if path == "" {
		return "", errors.New("file path is required")
	}
	if err := requireLocalFilesystemAccess("file_path", "content_base64"); err != nil {
		return "", err
	}
	absolutePath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve file path: %w", err)
	}
	canonicalPath, err := filepath.EvalSymlinks(absolutePath)
	if err != nil {
		return "", fmt.Errorf("resolve file path %s: %w", absolutePath, err)
	}
	if !pathWithinAllowedDirs(canonicalPath, allowedLocalDirs(UploadDirAllowlistEnv)) {
		return "", outsideAllowedDirsError("file", canonicalPath, UploadDirAllowlistEnv)
	}
	return canonicalPath, nil
}

// CanonicalLocalDirPath resolves a caller-supplied path to an existing local
// directory and returns it canonicalized, subject to the same roots and the
// same HTTP refusal as [CanonicalLocalFilePath].
func CanonicalLocalDirPath(path string) (string, error) {
	if path == "" {
		return "", errors.New("directory path is required")
	}
	// No alternative to name here: a directory of files has no inline form.
	// Publishing them one at a time with content_base64 is the remote path,
	// and that is the publish action's own error to give.
	if err := requireLocalFilesystemAccess("directory_path", ""); err != nil {
		return "", err
	}
	canonicalPath, err := canonicalDirPath(path)
	if err != nil {
		return "", fmt.Errorf("resolve directory path: %w", err)
	}
	if !pathWithinAllowedDirs(canonicalPath, allowedLocalDirs(UploadDirAllowlistEnv)) {
		return "", outsideAllowedDirsError("directory", canonicalPath, UploadDirAllowlistEnv)
	}
	return canonicalPath, nil
}

// CanonicalDownloadOutputPath resolves a caller-supplied destination for a
// file the server is about to write and returns it canonicalized, provided it
// lies under the working directory, the OS temporary directory, or a directory
// listed in GITLAB_MCP_ALLOWED_DOWNLOAD_DIRS. It refuses every path when the
// server is reached over HTTP.
//
// The destination does not exist yet and neither may its parents, so the
// deepest existing ancestor is what gets resolved through symlinks; the
// segments below it cannot be symlinks because they do not exist. A leaf that
// does exist must be a regular file: a symlink there would redirect the write
// to whatever it names, which is how an "output path" becomes a way to
// overwrite an SSH key.
//
// Call it again after creating the parent directories. The second call
// resolves a parent that now exists, which is what turns the check from a
// promise about the path into a check on the directory being written to.
//
// An existing regular file is overwritten, deliberately, and there is no
// caller opt-in to refuse it. The audit that produced the symlink check asked
// for one, and the trade is not worth taking: an opt-in is a new field on
// DownloadInput, which is a served input schema, so it lands in the tool
// snapshots, all three llms artifacts, the token-footprint tables and the
// per-domain docs. What it buys is bounded by two things that are already
// true. The destination can only be inside the working directory, the OS
// temporary directory or a directory the operator allow-listed, so the file at
// risk is one in the workspace rather than one belonging to the system; and
// under stdio, which is the only transport where a caller-supplied path is
// honored at all, whatever is driving this server holds its own filesystem
// write and can overwrite that same file directly. The opt-in would be one
// bool plus a regeneration pass if the calculus ever changes, but the residual
// risk it removes is smaller than the surface it adds.
func CanonicalDownloadOutputPath(path string) (string, error) {
	if path == "" {
		return "", errors.New("output path is required")
	}
	if err := requireLocalFilesystemAccess("output_path", ""); err != nil {
		return "", err
	}
	absolutePath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve output path: %w", err)
	}
	canonicalPath, err := canonicalizeThroughExistingAncestor(absolutePath)
	if err != nil {
		return "", fmt.Errorf("resolve output path %s: %w", absolutePath, err)
	}
	if info, statErr := os.Lstat(canonicalPath); statErr == nil && !info.Mode().IsRegular() {
		return "", fmt.Errorf("output path %s already exists and is not a regular file", canonicalPath)
	}
	if !pathWithinAllowedDirs(canonicalPath, allowedLocalDirs(DownloadDirAllowlistEnv)) {
		return "", outsideAllowedDirsError("output path", canonicalPath, DownloadDirAllowlistEnv)
	}
	return canonicalPath, nil
}

// canonicalizeThroughExistingAncestor resolves the longest existing prefix of
// an absolute path through symlinks and rejoins the not-yet-existing tail.
func canonicalizeThroughExistingAncestor(absolutePath string) (string, error) {
	dir := absolutePath
	tail := ""
	for {
		resolved, err := filepath.EvalSymlinks(dir)
		if err == nil {
			if tail == "" {
				return resolved, nil
			}
			return filepath.Join(resolved, tail), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no existing ancestor directory for %s", absolutePath)
		}
		tail = filepath.Join(filepath.Base(dir), tail)
		dir = parent
	}
}

// outsideAllowedDirsError explains a containment refusal in the terms the
// caller can act on: what was refused, and which variable widens the roots.
func outsideAllowedDirsError(kind, canonicalPath, envName string) error {
	return fmt.Errorf("%s %s is outside allowed directories; use the current working directory, the OS temp directory, or set %s",
		kind, canonicalPath, envName)
}

// CanonicalImportArchivePath validates a local GitLab export archive path and
// returns the canonical path resolved through symlinks. Archives must be regular
// .tar.gz files under the current working directory, the OS temporary directory,
// or a directory listed in GITLAB_MCP_ALLOWED_IMPORT_DIRS.
func CanonicalImportArchivePath(path string) (string, error) {
	if path == "" {
		return "", errors.New("archive path is required")
	}
	// An archive path is the same primitive as file_path, so it answers to the
	// same transport rule: a remote caller never placed an archive here.
	if err := requireLocalFilesystemAccess("a local archive path", ""); err != nil {
		return "", err
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
	return pathWithinAllowedDirs(canonicalPath, allowedImportArchiveDirs())
}

func pathWithinAllowedDirs(canonicalPath string, dirs []string) bool {
	for _, base := range dirs {
		if pathWithinBase(canonicalPath, base) {
			return true
		}
	}
	return false
}

func allowedImportArchiveDirs() []string {
	return allowedLocalDirs(ImportArchiveAllowlistEnv)
}

// allowedLocalDirs returns the canonical directories a caller-supplied local
// path may resolve into: the working directory, the OS temporary directory,
// and whatever envName lists.
//
// The working directory is skipped when it is a filesystem root or the user's
// home directory. A stdio server usually starts in the workspace the user just
// opened, which is what makes it a sensible root; Claude Desktop starts its
// servers in "/", where keeping it would allow-list the entire disk and quietly
// undo the whole containment. The home directory is the same argument one level
// down, and it is not hypothetical: it holds ~/.ssh, ~/.aws, the browser
// profiles, and this server's own ~/.gitlab-mcp-server.env, so implicitly
// allow-listing it means a file_path naming that file exfiltrates the very
// GITLAB_TOKEN the containment exists to protect.
//
// Skipped as an implicit root, not forbidden: an operator whose workspace
// really is their home directory names it in envName and gets it back. That is
// the difference between a default that is safe and a policy that decides for
// them, and the warning logged the first time a caller-supplied path is
// resolved there says so rather than leaving a working setup to fail as a
// puzzling "outside the allowed directories".
func allowedLocalDirs(envName string) []string {
	type dirEntry struct {
		path       string
		configured bool
	}
	dirs := []dirEntry{}
	if cwd, err := os.Getwd(); err == nil && filepath.Dir(cwd) != cwd && !skipHomeAsImplicitRoot(cwd) {
		dirs = append(dirs, dirEntry{path: cwd})
	}
	dirs = append(dirs, dirEntry{path: os.TempDir()})
	if configured := os.Getenv(envName); configured != "" {
		for _, dir := range filepath.SplitList(configured) {
			dirs = append(dirs, dirEntry{path: dir, configured: true})
		}
	}

	allowed := make([]string, 0, len(dirs))
	seen := make(map[string]struct{}, len(dirs))
	for _, dir := range dirs {
		canonicalDir, err := canonicalDirPath(dir.path)
		if err != nil {
			if dir.configured {
				slog.Warn("skipping invalid allowlist directory", "env", envName, "path", dir.path, "error", err)
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

// homeDirWarned keeps the dropped-home-root warning to one line per process.
// It is emitted from a path a tool call reaches rather than from startup, so
// without this a session doing many uploads would repeat it on every one.
var homeDirWarned atomic.Bool

// userHomeDir is [os.UserHomeDir], replaceable in tests.
var userHomeDir = os.UserHomeDir

// skipHomeAsImplicitRoot reports whether cwd is the user's home directory, and
// says so once when it is.
//
// A false answer whenever the home directory cannot be determined is the safe
// direction here: it keeps the working directory as a root, which is the
// behavior every deployment already has, rather than silently narrowing the
// allow-list on a platform where os.UserHomeDir happens to fail.
func skipHomeAsImplicitRoot(cwd string) bool {
	home, err := userHomeDir()
	if err != nil || home == "" {
		return false
	}
	canonicalHome, err := canonicalDirPath(home)
	if err != nil {
		return false
	}
	canonicalCWD, err := canonicalDirPath(cwd)
	if err != nil {
		canonicalCWD = filepath.Clean(cwd)
	}
	if canonicalCWD != canonicalHome {
		return false
	}
	if homeDirWarned.CompareAndSwap(false, true) {
		slog.Warn("working directory is the home directory, so it is not an implicit allowlist root for caller-supplied paths",
			"home", canonicalHome,
			"remedy", "start the server from a project directory, or name the directories you want reachable in "+
				UploadDirAllowlistEnv+", "+DownloadDirAllowlistEnv+" or "+ImportArchiveAllowlistEnv,
		)
	}
	return true
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
	f, err := os.Open(cleanPath) //#nosec G304 -- path is cleaned via filepath.Clean; callers are internal, not user-controlled
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
