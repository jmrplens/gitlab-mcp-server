// envfile.go decides which dotenv files are allowed to configure this process.
//
// A stdio MCP server inherits its working directory from the client, and every
// client that opens a workspace sets it to that workspace. The directory is
// therefore untrusted input: its contents can arrive with a cloned repository,
// an extracted archive or a shared network share, chosen by whoever wrote them
// rather than by the person running the server.
//
// This file used to load "./.env" first, so a two-line file in a cloned
// repository chose which host received the operator's GitLab token, disabled
// certificate verification so the redirection raised no error, turned OTLP
// telemetry on and named the collector, and rewrote the tool descriptions the
// model reads. None of that needed a tool call, a model turn or any
// interaction at all: the startup probe delivered the token. The variables
// that make it work are exactly the ones no MCP client sets, so they are
// always unset and always available to be filled in by whatever file is found
// first.
//
// The rule now is that a dotenv file configures this server only when someone
// deliberately put it where the server looks (the home file) or deliberately
// named it (GITLAB_MCP_ENV_FILE). Being in the working directory is not a
// decision anyone made. Git's safe.directory ownership check, direnv's
// per-directory "direnv allow" and VS Code Workspace Trust are three
// independent tools that reached the same conclusion.
package config

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/joho/godotenv"
)

// EnvFileName is the name of the env file this server reads credentials from,
// in the user's home directory.
const EnvFileName = ".gitlab-mcp-server.env"

// EnvFileVar names a dotenv file to load in addition to the home file. It is
// read from the process environment only, which is what makes it an opt-in: a
// file this server declines to load cannot nominate itself.
//
// It exists so the convenience the working-directory load used to provide
// survives as a deliberate act. A developer who keeps credentials in a
// repository-local file passes GITLAB_MCP_ENV_FILE=.env once, in the client
// configuration or the shell that launches the server, and says so.
const EnvFileVar = "GITLAB_MCP_ENV_FILE"

// workingDirEnvFileName is the file this server deliberately does not load. It
// is still looked for, because a developer whose repository-local .env stopped
// taking effect deserves to be told why rather than left debugging it.
const workingDirEnvFileName = ".env"

// maxDotenvBytes bounds how much of the unloaded working-directory file is
// parsed to name its keys in the warning. The file is untrusted, and nothing
// here is worth reading a multi-gigabyte one for.
const maxDotenvBytes = 64 << 10

// maxAnnouncedKeys and maxAnnouncedKeyRunes bound the warning itself: the key
// names come from an untrusted file and are logged, so their number and length
// are capped rather than trusted.
const (
	maxAnnouncedKeys     = 10
	maxAnnouncedKeyRunes = 40
)

// envFileAnnounceOnce keeps the startup announcement to one occurrence per
// process. LoadEnvFiles is called twice on the startup path (once before the
// first-run check, once from Load) and once per pooled server entry in HTTP
// mode, but what it reports is a property of the process, not of the call.
var envFileAnnounceOnce sync.Once

// EnvFileReport describes what LoadEnvFiles did, so a caller can render it
// itself instead of parsing logs. LoadEnvFiles already announces the
// security-relevant parts.
type EnvFileReport struct {
	// ExplicitPath is the absolute path of the file EnvFileVar named, whether
	// or not it could be read. Empty when the variable is unset.
	ExplicitPath string
	// ExplicitErr is why the file EnvFileVar named could not be read.
	ExplicitErr error
	// HomePath is the home file that was loaded. Empty when there is none.
	HomePath string
	// IgnoredPath is the absolute path of a working-directory .env that was
	// found and deliberately not loaded. Empty when there is none.
	IgnoredPath string
	// IgnoredKeys are the variable names that file would have set, sorted.
	IgnoredKeys []string
}

// LoadEnvFiles populates the process environment from the dotenv files this
// server reads, and is safe to call more than once.
//
// It is separate from [Load] because something has to consult these values
// before a full configuration exists. The startup path decides whether to show
// a first-run screen instead of serving, and that decision must be made against
// the same environment Load will see: a deployment that put its credentials in
// ~/.gitlab-mcp-server.env is configured, and a check reading only os.Getenv
// would conclude the opposite and refuse to start.
//
// Every load is best-effort, a missing file being the normal case, and godotenv
// never overwrites a variable that is already set. The resulting precedence,
// highest first, is:
//
//  1. the process environment, which is what the MCP client passed;
//  2. the file [EnvFileVar] names, if any;
//  3. ~/.gitlab-mcp-server.env.
//
// A .env in the working directory is not on that list. When one exists it is
// read far enough to name its keys and reported at WARN, and nothing it
// contains reaches the environment.
func LoadEnvFiles() EnvFileReport {
	var report EnvFileReport

	explicit := strings.TrimSpace(os.Getenv(EnvFileVar))
	if explicit != "" {
		report.ExplicitPath = absolutePath(explicit)
		if err := godotenv.Load(explicit); err != nil {
			report.ExplicitErr = err
		}
	}

	if home, err := os.UserHomeDir(); err == nil {
		homePath := filepath.Join(home, EnvFileName)
		if godotenv.Load(homePath) == nil {
			report.HomePath = homePath
		}
	}

	report.IgnoredPath, report.IgnoredKeys = ignoredWorkingDirEnvFile(report.ExplicitPath)
	envFileAnnounceOnce.Do(report.announce)
	return report
}

// ignoredWorkingDirEnvFile reports the working-directory .env that was not
// loaded, and the variable names it would have set.
//
// A file the operator named through EnvFileVar was loaded on purpose even when
// it is that same .env, so it is not reported as ignored.
func ignoredWorkingDirEnvFile(explicitPath string) (path string, keys []string) {
	info, err := os.Stat(workingDirEnvFileName)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return "", nil
	}
	found := absolutePath(workingDirEnvFileName)
	if explicitPath != "" && explicitPath == found {
		return "", nil
	}
	return found, dotenvKeys(workingDirEnvFileName)
}

// dotenvKeys returns the sorted variable names a dotenv file sets, reading at
// most maxDotenvBytes of it. A file that cannot be opened or parsed yields no
// names, which weakens the warning without suppressing it.
func dotenvKeys(path string) []string {
	file, err := os.Open(path) // #nosec G304 -- fixed name in the working directory, read only to name its keys
	if err != nil {
		return nil
	}
	defer func() { _ = file.Close() }()

	values, err := godotenv.Parse(io.LimitReader(file, maxDotenvBytes))
	if err != nil {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// absolutePath resolves path for display, falling back to what was given when
// the working directory cannot be determined. It is presentation only: the
// loads themselves use the path as written.
func absolutePath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// announce writes the startup record of where configuration came from.
//
// Both lines are WARN rather than INFO on purpose. They are the only local
// evidence that this process took configuration from a file outside the client
// configuration, and LOG_LEVEL is itself one of the settings such a file would
// like to set.
func (r EnvFileReport) announce() {
	if r.ExplicitPath != "" {
		if r.ExplicitErr != nil {
			slog.Warn("env file named by "+EnvFileVar+" could not be read",
				"env", EnvFileVar, "path", r.ExplicitPath, "error", r.ExplicitErr)
		} else {
			slog.Warn("loading configuration from the env file named by "+EnvFileVar,
				"env", EnvFileVar, "path", r.ExplicitPath)
		}
	}
	if r.IgnoredPath != "" {
		slog.Warn("ignoring the .env file in the working directory",
			"path", r.IgnoredPath,
			"key_count", len(r.IgnoredKeys),
			"keys", announcedKeys(r.IgnoredKeys),
			"reason", "the working directory belongs to whoever wrote it, not to this server",
			"hint", "put these settings in ~/"+EnvFileName+", or name the file with "+EnvFileVar)
	}
}

// announcedKeys bounds the untrusted key names before they are logged.
func announcedKeys(keys []string) []string {
	if len(keys) > maxAnnouncedKeys {
		keys = keys[:maxAnnouncedKeys]
	}
	out := make([]string, len(keys))
	for i, key := range keys {
		if runes := []rune(key); len(runes) > maxAnnouncedKeyRunes {
			key = string(runes[:maxAnnouncedKeyRunes]) + "..."
		}
		out[i] = key
	}
	return out
}
