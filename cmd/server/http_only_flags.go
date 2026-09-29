package main

import (
	"log/slog"
	"maps"
	"slices"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
)

// httpOnlyFlag describes a flag whose value lands in [httpConfig], which a
// stdio run never reads: main hands [runWithContext] a nil one unless the
// transport is HTTP, and [runStdio] builds its configuration from
// [config.Load] and the environment alone.
//
// # Why a stdio run says so
//
// The flags are registered for the whole process, so a stdio run accepts and
// parses every one of them and, until issue 1045, went on without a word. A
// stdio user who passed --rate-limit-rps=5 to switch the limiter on got no
// limiter and no message, and one who passed --read-only got a server that
// writes. The fix is to say so at startup, once, naming each flag and the
// variable that carries the same setting on stdio where there is one.
type httpOnlyFlag struct {
	// stdioVariable is the variable that carries the same setting on stdio,
	// or empty where stdio has no such setting at all: the listener, the pool,
	// the authentication gate and the OAuth metadata exist only in HTTP mode.
	stdioVariable string
	// withholds reports whether the value the flag was given asks the server
	// to hold back writes. Set only for --read-only and --safe-mode, the two
	// settings [config.RetiredEnvUses] refuses a retired spelling of, for the
	// same reason: ignoring either serves writes on a deployment that asked
	// for reads, and there is no warning quiet enough to be the right answer
	// to that, because the stdio servers most likely to carry one are the ones
	// an MCP client started with nobody reading their stderr. A value that
	// asks for nothing to be held back (--read-only=false) costs nothing when
	// ignored, so it is only reported.
	withholds func(*httpConfig) bool
}

// httpOnlyFlags names every flag main registers into [httpConfig], keyed by
// the name the operator types. TestHTTPOnlyFlags_AreTheFlagsBoundToHTTPConfig
// holds the key set to the registrations in main.go in both directions, so a
// flag added to the HTTP configuration cannot be left out of the stdio
// warning, and a flag that stops being HTTP-only cannot stay in it.
//
// The flags a stdio run does read are not here: --transport, --http,
// --env-file, the telemetry flags and the ones env_flags.go registers, which
// write their variable before anything reads configuration. --tool-search and
// --probe read --tool-surface, --tier and --tls-cert, but both exit before a
// transport is chosen, so those three are HTTP-only for a run that serves.
var httpOnlyFlags = map[string]httpOnlyFlag{
	"http-addr":                  {},
	"http-socket-mode":           {},
	"tls-cert":                   {},
	"tls-key":                    {},
	"stateless":                  {},
	"json-response":              {},
	"max-request-body-bytes":     {},
	"session-timeout":            {},
	"http-idle-timeout":          {},
	"drain-delay":                {},
	"gitlab-url":                 {stdioVariable: "GITLAB_URL"},
	"allow-any-gitlab-url":       {},
	"skip-tls-verify":            {stdioVariable: config.EnvPrefix + "SKIP_TLS_VERIFY"},
	"tier":                       {stdioVariable: config.EnvPrefix + "TIER"},
	"ignore-scopes":              {stdioVariable: config.EnvPrefix + "IGNORE_SCOPES"},
	"tool-surface":               {stdioVariable: config.EnvPrefix + "TOOL_SURFACE"},
	"capability-surface":         {stdioVariable: config.EnvPrefix + "CAPABILITY_SURFACE"},
	"meta-param-schema":          {stdioVariable: config.EnvPrefix + "META_PARAM_SCHEMA"},
	"embedded-resources":         {stdioVariable: config.EnvPrefix + "EMBEDDED_RESOURCES"},
	"exclude-tools":              {stdioVariable: config.EnvPrefix + "EXCLUDE_TOOLS"},
	"read-only":                  {stdioVariable: config.EnvPrefix + "READ_ONLY", withholds: func(h *httpConfig) bool { return h.readOnly }},
	"safe-mode":                  {stdioVariable: config.EnvPrefix + "SAFE_MODE", withholds: func(h *httpConfig) bool { return h.safeMode }},
	"action-timeout":             {stdioVariable: config.EnvPrefix + "ACTION_TIMEOUT"},
	"rate-limit-rps":             {stdioVariable: config.EnvPrefix + "RATE_LIMIT_RPS"},
	"rate-limit-burst":           {stdioVariable: config.EnvPrefix + "RATE_LIMIT_BURST"},
	"max-http-clients":           {},
	"revalidate-interval":        {},
	"pool-idle-timeout":          {},
	"auth-mode":                  {},
	"public-url":                 {},
	"resource-documentation":     {},
	"resource-policy-uri":        {},
	"resource-tos-uri":           {},
	"oauth-cache-ttl":            {},
	"oauth-client-uid":           {},
	"trusted-origins":            {},
	"trusted-proxies":            {},
	"trusted-proxy-header":       {},
	"auth-failure-limit":         {},
	"auth-failure-window":        {},
	"auth-distinct-token-limit":  {},
	"auth-distinct-token-window": {},
}

// stdioIgnoredFlags sorts the HTTP-only flags hcfg records as passed into the
// ones a stdio run must refuse and the ones it only reports, each in name
// order. A refused flag is not reported as well: its refusal already says what
// it is and what to set instead.
func stdioIgnoredFlags(hcfg *httpConfig) (refuse, ignored []string) {
	for _, name := range slices.Sorted(maps.Keys(hcfg.setFlags)) {
		entry, httpOnly := httpOnlyFlags[name]
		if !httpOnly {
			continue
		}
		if entry.withholds != nil && entry.withholds(hcfg) {
			refuse = append(refuse, name)
			continue
		}
		ignored = append(ignored, name)
	}
	return refuse, ignored
}

// describeIgnoredFlag spells one ignored flag for the startup line, with the
// variable a stdio deployment sets instead where it has one.
func describeIgnoredFlag(name string) string {
	if variable := httpOnlyFlags[name].stdioVariable; variable != "" {
		return "--" + name + " (on stdio set " + variable + ")"
	}
	return "--" + name + " (stdio has no such setting)"
}

// The three lines [reportStdioIgnoredFlags] writes. test/e2e/stdio reads them
// off the real binary's stderr, spelled out there because it cannot import
// this package.
const (
	stdioIgnoredFlagsLine = "these flags are read in HTTP mode only, and a stdio server takes its configuration " +
		"from the environment, so they have no effect"
	stdioAutoIgnoredFlagsLine = "--transport=auto chose stdio, so the flags given for HTTP mode have no effect on this run"
	stdioWithheldWritesLine   = "a flag asking to hold back writes was passed to a stdio server, which reads it in HTTP mode " +
		"only and would serve the writes, so this deployment will not be started under a capability it did not ask for"
)

// reportStdioIgnoredFlags tells the operator of a stdio run which of the flags
// it was given only HTTP mode reads, and reports whether startup may continue.
//
// An HTTP run reads them all and has nothing to report. A stdio run logs one
// line naming each flag it ignores, at WARN when the operator chose stdio,
// explicitly or by default, and at INFO when --transport=auto chose it: that
// operator wrote one command line for either transport, which is what the
// container image's own command does with --http-addr, so an HTTP flag there
// is expected rather than a mistake, while still not silent. A flag that asks
// the server to hold back writes is refused in either case, one ERROR line
// each, and startup stops.
func reportStdioIgnoredFlags(choice transportDecision, hcfg *httpConfig) bool {
	if choice.HTTP {
		return true
	}
	refuse, ignored := stdioIgnoredFlags(hcfg)
	if len(ignored) > 0 {
		described := make([]string, 0, len(ignored))
		for _, name := range ignored {
			described = append(described, describeIgnoredFlag(name))
		}
		if choice.Inference != "" {
			slog.Info(stdioAutoIgnoredFlagsLine, "flags", described)
		} else {
			slog.Warn(stdioIgnoredFlagsLine, "flags", described)
		}
	}
	for _, name := range refuse {
		slog.Error(stdioWithheldWritesLine, "flag", "--"+name, "set_instead", httpOnlyFlags[name].stdioVariable+"=true")
	}
	return len(refuse) == 0
}
