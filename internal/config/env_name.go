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
// quietly reconfigured. Only the settings 3.0.0 read had one
// ([hadOldSpelling]); a setting added since carries the prefix from the start.
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
	"DISCOVERY_RATE_LIMIT_BURST",
	"DISCOVERY_RATE_LIMIT_RPS",
	"DRAIN_DELAY",
	"EMBEDDED_RESOURCES",
	"EXCLUDE_TOOLS",
	"IGNORE_SCOPES",
	"LOG_LEVEL",
	"MAX_HTTP_CLIENTS",
	"MAX_LISTEN_STREAMS",
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
	"STDIO_MAX_LINE_BYTES",
	"TIER",
	"TOOL_SURFACE",
	"TRUSTED_ORIGINS",
	"UPLOAD_MAX_FILE_SIZE",
	"YOLO_MODE",
}

// httpOnlyNames are the settings only HTTP mode has, named without their
// prefix and in name order: the listener's drain, the pool and its idle
// timeout, the stateful session's timeout, the authentication gate with its
// mode and budgets, the OAuth verifier and the metadata it publishes, the
// origins a browser may come from, and the discovery documents' bucket.
//
// A stdio server has none of these, so [Load] reads none of them, and
// [HTTPOnlyEnvUses] names the ones an environment sets so a stdio run can say
// it ignores them, as it says so of an HTTP-only flag (issue 1171). Until then
// stdio parsed most of them and refused to start on a value it would never
// have used: a GITLAB_MCP_AUTH_MODE=oauth or a GITLAB_MCP_MAX_HTTP_CLIENTS=0
// the same shell exported for an HTTP deployment beside it stopped the stdio
// process, while GITLAB_MCP_TRUSTED_ORIGINS, which it never read, did not.
//
// The revalidation interval is not one of them: stdio re-reads a fine-grained
// token at it. TestHTTPOnlyNames_AreTheSettingsOnlyTheOverlayReads holds the
// list to the two readers, so a name is here exactly when
// [LoadHTTPEnvOverlay] reads it and [Load] does not.
var httpOnlyNames = []string{
	"AUTH_DISTINCT_TOKEN_LIMIT",
	"AUTH_DISTINCT_TOKEN_WINDOW",
	"AUTH_FAILURE_LIMIT",
	"AUTH_FAILURE_WINDOW",
	"AUTH_MODE",
	"DISCOVERY_RATE_LIMIT_BURST",
	"DISCOVERY_RATE_LIMIT_RPS",
	"DRAIN_DELAY",
	"MAX_HTTP_CLIENTS",
	"OAUTH_CACHE_TTL",
	"OAUTH_CLIENT_UID",
	"POOL_IDLE_TIMEOUT",
	"PUBLIC_URL",
	"SESSION_TIMEOUT",
	"TRUSTED_ORIGINS",
}

// HTTPOnlyEnvUses returns the settings only HTTP mode has that this
// environment sets, each under the name an operator exports it by, in name
// order. A value of blanks counts as unset, as it does to the HTTP overlay
// ([envPresent]), which would not read it either.
func HTTPOnlyEnvUses() []string {
	var set []string
	for _, name := range httpOnlyNames {
		if envPresent(name) {
			set = append(set, EnvPrefix+name)
		}
	}
	return set
}

// hadOldSpelling are the settings that answered to another name as well until
// 3.1.0 removed it: the ones 3.0.0 read, and the only ones [RetiredEnvUses]
// looks for.
//
// The list is closed, and spelled out rather than derived from
// [prefixedNames] for that reason. A setting added since was read under its
// prefixed name from its first day, so its bare suffix is somebody else's
// variable if it is anybody's, and reporting it as "no longer read (removed in
// 3.1.0)" would tell an operator that a name another tool owns was once this
// server's, and send them renaming it. The four authentication budgets are the
// first such settings: they arrived in 3.1.0 after its removal of the old
// spellings had landed. A setting added after them needs nothing here.
var hadOldSpelling = []string{
	"ACTION_TIMEOUT",
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

// protectionNames are the settings an operator sets to withhold part of what a
// deployment serves, the three register row AUT-004 decides together, and the
// reason [RetiredEnvUses] splits its answer.
//
// Ignoring any retired name reconfigures a deployment that did not ask to be
// reconfigured, but ignoring one of these serves what the operator took away.
// A deployment carrying GITLAB_READ_ONLY=true and nothing else has asked to
// serve reads, one carrying EXCLUDE_TOOLS=project.delete has asked for that
// action not to exist, and a version that silently stops reading the variable
// serves the writes, or the action, instead. There is no warning quiet enough
// to be the right answer to that, because the deployments most likely to be
// running unattended are exactly the ones nobody is reading stderr for.
//
// EXCLUDE_TOOLS is the one bare name of the three, generic enough that another
// tool in the same shell could own it. It refuses all the same: the server
// cannot tell whose it is, the guess that it belongs to somebody else is the
// one that serves what an operator removed, and the refusal costs whoever set
// it one variable renamed, or unset in this server's environment.
var protectionNames = map[string]struct{}{
	"EXCLUDE_TOOLS": {},
	"READ_ONLY":     {},
	"SAFE_MODE":     {},
}

// RetiredEnvUses reports the retired spellings present in this environment,
// split by what ignoring one would cost. refuse names the three settings
// register row AUT-004 decides together, which withhold part of what the
// deployment serves ([protectionNames]); warn names the rest. A retired bound
// among the rest, UPLOAD_MAX_FILE_SIZE or RATE_LIMIT_RPS say, warns too:
// ignoring it puts the setting back to its default, which may be looser than
// the value the retired name carried.
//
// It reads the environment rather than what was consulted, which is the
// opposite of what the deprecation warning it replaces did. That warning could
// report only what had been read, because reading was still happening. Nothing
// reads these now, so the only moment they can be noticed is this one.
func RetiredEnvUses() (refuse, warn []string) {
	for _, name := range hadOldSpelling {
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
