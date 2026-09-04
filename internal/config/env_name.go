package config

import (
	"os"
	"slices"
	"strings"
	"sync"
)

// EnvPrefix is what every variable this project defines is named with from
// 2.8.0.
//
// A stdio MCP server runs in whatever shell its client was started from,
// alongside every other tool that person uses. Names like RATE_LIMIT_RPS,
// AUTH_MODE and LOG_LEVEL are generic enough that another tool may already own
// them there, and a collision is silent: the server reads a value it was never
// given and behaves in a way nobody configured.
const EnvPrefix = "GITLAB_MCP_"

// prefixedNames are the variables that gained EnvPrefix in 2.8.0 and still
// answer to their old name. The unprefixed spelling is removed in v3, which is
// the release that renumbers anyway when client-go does.
//
// Two names are deliberately absent. GITLAB_URL and GITLAB_TOKEN stay bare:
// they are GitLab's own convention, every existing configuration sets them, and
// they are the two an assistant is most likely to write from memory into a
// client configuration. Prefixing them would break far more than it protects.
//
// OTEL_* is absent for a stronger reason: those names belong to the
// OpenTelemetry specification and the exporters read them directly, so a
// prefixed spelling would simply not be seen.
var prefixedNames = []string{
	"ACTION_TIMEOUT",
	"AUTH_MODE",
	"CAPABILITY_SURFACE",
	"CLIENT_COMPAT",
	"DRAIN_DELAY",
	"EMBEDDED_RESOURCES",
	"EXCLUDE_TOOLS",
	"LOG_LEVEL",
	"MAX_HTTP_CLIENTS",
	"META_PARAM_SCHEMA",
	"META_TOOLS",
	"OAUTH_CACHE_TTL",
	"OAUTH_CLIENT_UID",
	"POOL_IDLE_TIMEOUT",
	"PUBLIC_URL",
	"RATE_LIMIT_BURST",
	"RATE_LIMIT_RPS",
	"SESSION_REVALIDATE_INTERVAL",
	"SESSION_TIMEOUT",
	"TOOL_SURFACE",
	"TRUSTED_ORIGINS",
	"UPLOAD_MAX_FILE_SIZE",
}

// deprecatedEnvUses records, once per unprefixed name actually read, that the
// old spelling was the value's source. The warning is emitted once at startup
// rather than at each read, because several of these are consulted more than
// once and a per-read warning would say the same thing four times.
var deprecatedEnvUses sync.Map

// Getenv reads a setting under its prefixed name, falling back to the
// unprefixed one and recording that it did.
//
// The prefixed name wins when both are set, and that case is recorded too:
// an operator who set both has one of them doing nothing, which is worth
// saying out loud rather than resolving in silence.
//
// A name outside [prefixedNames] is read verbatim, so this is safe to use for
// GITLAB_URL, GITLAB_TOKEN and anything else that never gained a prefix.
func Getenv(name string) string {
	if !slices.Contains(prefixedNames, name) {
		return os.Getenv(name)
	}

	prefixed, prefixedSet := os.LookupEnv(EnvPrefix + name)
	legacy, legacySet := os.LookupEnv(name)

	switch {
	case prefixedSet && legacySet:
		deprecatedEnvUses.Store(name, bothSet)
		return prefixed
	case prefixedSet:
		return prefixed
	case legacySet:
		deprecatedEnvUses.Store(name, legacyOnly)
		return legacy
	default:
		return ""
	}
}

// How an unprefixed name came to be noticed, which decides what the warning
// tells the operator to do about it.
const (
	legacyOnly = iota
	bothSet
)

// DeprecatedEnvWarnings returns one line per unprefixed variable that was
// actually read, in a stable order, ready to be logged at startup.
//
// It reports what was read rather than what is set, so an operator is never
// warned about a variable this deployment ignores anyway.
func DeprecatedEnvWarnings() []string {
	var names []string
	deprecatedEnvUses.Range(func(key, _ any) bool {
		if name, ok := key.(string); ok {
			names = append(names, name)
		}
		return true
	})
	slices.Sort(names)

	warnings := make([]string, 0, len(names))
	for _, name := range names {
		kind, _ := deprecatedEnvUses.Load(name)
		if kind == bothSet {
			warnings = append(warnings, "both "+EnvPrefix+name+" and "+name+
				" are set; "+EnvPrefix+name+" is being used and "+name+" is ignored")
			continue
		}
		warnings = append(warnings, name+" is deprecated and will be removed in v3; rename it to "+EnvPrefix+name)
	}
	return warnings
}

// PrefixedEnvNames returns the variables that answer to both spellings, for
// tests and for documentation generators that must not drift from this list.
func PrefixedEnvNames() []string {
	return slices.Clone(prefixedNames)
}

// resetDeprecatedEnvUses clears what has been recorded. Tests need it because
// the record is process-wide by design: the warning belongs to the process, not
// to a call.
func resetDeprecatedEnvUses() {
	deprecatedEnvUses.Range(func(key, _ any) bool {
		deprecatedEnvUses.Delete(key)
		return true
	})
}

// TrimmedGetenv is [Getenv] with surrounding whitespace removed, which is what
// almost every caller here wants from a value a human typed into a shell.
func TrimmedGetenv(name string) string {
	return strings.TrimSpace(Getenv(name))
}
