package config

import (
	"os"
	"slices"
	"strings"
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

// prefixedNames are the variables this project defines, named without their
// prefix. Each is read as EnvPrefix + the name and under no other spelling.
//
// They gained the prefix in 2.8.0 and answered to their old name as well until
// 3.1.0, which removes it. The removal was held back from 3.0.0 on purpose:
// 2.7.5 still carries a self-updater and 3.0.0 does not, so a 2.7.5 deployment
// updates itself into 3.0.0 without anyone reading a release note, and taking
// the old spellings away in the release that arrives unannounced would have
// broken those deployments in silence. 3.1.0 is the first version nobody is
// carried into.
//
// What is left of the old spellings is [RetiredEnvUses], which finds them in
// the environment so that a deployment still setting one is told rather than
// quietly reconfigured.
//
// Every variable this server defines is on this list; the rule has no
// exception for a name that already began with GITLAB_. Two names stay bare
// on purpose: GITLAB_URL and GITLAB_TOKEN are GitLab's own convention, every
// existing configuration sets them, and a user who already has them in the
// environment must not have to spell them twice. OTEL_* stays bare for a
// stronger reason: those names belong to the OpenTelemetry specification and
// the exporters read them directly, so a prefixed spelling would not be seen.
// AUTOPILOT is not ours either; it is a convention other agent tooling sets,
// consulted as an alias of YOLO_MODE and never warned about.
var prefixedNames = []string{
	"ACTION_TIMEOUT",
	"AUTH_DISTINCT_TOKEN_LIMIT",
	"AUTH_DISTINCT_TOKEN_WINDOW",
	"AUTH_FAILURE_LIMIT",
	"AUTH_FAILURE_WINDOW",
	"AUTH_MODE",
	"CAPABILITY_SURFACE",
	"CLIENT_COMPAT",
	"DRAIN_DELAY",
	"EMBEDDED_RESOURCES",
	"EXCLUDE_TOOLS",
	"IGNORE_SCOPES",
	"LOG_LEVEL",
	"MAX_HTTP_CLIENTS",
	"META_PARAM_SCHEMA",
	"OAUTH_CACHE_TTL",
	"OAUTH_CLIENT_UID",
	"POOL_IDLE_TIMEOUT",
	"PPROF_ADDR",
	"PUBLIC_URL",
	"RATE_LIMIT_BURST",
	"RATE_LIMIT_RPS",
	"READ_ONLY",
	"SAFE_MODE",
	"SESSION_REVALIDATE_INTERVAL",
	"SESSION_TIMEOUT",
	"SKIP_TLS_VERIFY",
	"TIER",
	"TOOL_SURFACE",
	"TRUSTED_ORIGINS",
	"UPLOAD_MAX_FILE_SIZE",
	"YOLO_MODE",
}

// retiredNames spells the removed name of the settings whose old name was not
// the bare suffix. The first batch dropped a generic name to a prefixed one
// (TOOL_SURFACE to GITLAB_MCP_TOOL_SURFACE); these carried a GITLAB_ of their
// own, so the name to look for is that spelling rather than TIER.
var retiredNames = map[string]string{
	"IGNORE_SCOPES":   "GITLAB_IGNORE_SCOPES",
	"READ_ONLY":       "GITLAB_READ_ONLY",
	"SAFE_MODE":       "GITLAB_SAFE_MODE",
	"SKIP_TLS_VERIFY": "GITLAB_SKIP_TLS_VERIFY",
	"TIER":            "GITLAB_TIER",
}

// RetiredEnvName returns the spelling a setting answered to before it gained
// EnvPrefix: the bare suffix for most, a GITLAB_-prefixed name for the ones
// that already had one. Nothing reads a setting under it any more.
func RetiredEnvName(name string) string {
	if retired, ok := retiredNames[name]; ok {
		return retired
	}
	return name
}

// protectionNames are the settings an operator sets to take capability away
// from a deployment, and the reason [RetiredEnvUses] splits its answer.
//
// Ignoring any retired name reconfigures a deployment that did not ask to be
// reconfigured, but these two decide whether a tool call may write. A
// deployment carrying GITLAB_READ_ONLY=true and nothing else has asked to serve
// reads, and a version that silently stops reading that variable serves writes
// instead. There is no warning quiet enough to be the right answer to that,
// because the deployments most likely to be running unattended are exactly the
// ones nobody is reading stderr for.
var protectionNames = map[string]struct{}{
	"READ_ONLY": {},
	"SAFE_MODE": {},
}

// RetiredEnvUses reports the retired spellings present in this environment,
// split by what ignoring one would cost: refuse names a setting whose absence
// would leave the deployment able to do more than it was configured for, and
// warn names the rest.
//
// It reads the environment rather than what was consulted, which is the
// opposite of what the deprecation warning it replaces did. That warning could
// report only what had been read, because reading was still happening. Nothing
// reads these now, so the only moment they can be noticed is this one.
func RetiredEnvUses() (refuse, warn []string) {
	for _, name := range prefixedNames {
		retired := RetiredEnvName(name)
		if _, set := os.LookupEnv(retired); !set {
			continue
		}
		line := retired + " is no longer read (removed in 3.1.0): rename it to " + EnvPrefix + name
		if _, protects := protectionNames[name]; protects {
			refuse = append(refuse, line)
			continue
		}
		warn = append(warn, line)
	}
	return refuse, warn
}

// Getenv reads a setting under its prefixed name, and under that name alone.
//
// A name outside [prefixedNames] is read verbatim, so this is safe to use for
// GITLAB_URL, GITLAB_TOKEN and anything else that never gained a prefix. That
// is the whole of what the list decides now: whether a name is one this
// project defines, and therefore carries EnvPrefix, or somebody else's.
func Getenv(name string) string {
	if !slices.Contains(prefixedNames, name) {
		return os.Getenv(name)
	}
	return os.Getenv(EnvPrefix + name)
}

// PrefixedEnvNames returns the variables this project defines, for tests and
// for documentation generators that must not drift from this list.
func PrefixedEnvNames() []string {
	return slices.Clone(prefixedNames)
}

// TrimmedGetenv is [Getenv] with surrounding whitespace removed, which is what
// almost every caller here wants from a value a human typed into a shell.
func TrimmedGetenv(name string) string {
	return strings.TrimSpace(Getenv(name))
}
