package main

import (
	"log/slog"
	"maps"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/serverpool"
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
//
// # Which flags refuse the start instead
//
// The condition is what ignoring the value would cost, the rule
// [config.RetiredEnvUses] applies to the retired variable names. Ignoring most
// of these flags costs a setting (a tier, a surface, a limit), which is worth a
// line. Two kinds cost more, and there is no warning quiet enough to be the
// right answer to either, because the stdio servers most likely to carry one
// are the ones an MCP client started with nobody reading their stderr:
//
//   - A flag withholding part of what the server serves: --read-only,
//     --safe-mode and --exclude-tools, the three settings register row
//     AUT-004 decides together. Ignoring one serves what the operator asked
//     not to be served. RetiredEnvUses refuses a retired spelling of each of
//     the three for that reason.
//   - --gitlab-url naming instances none of which is the one a stdio run
//     connects to. Ignoring it sends GITLAB_TOKEN to an instance this command
//     line did not name, https://gitlab.com when GITLAB_URL is unset. It is
//     the refusal HTTP mode makes when it will not choose an instance on a
//     caller's behalf ([serverpool.ErrMissingGitLabURL]), made here of the
//     operator.
//
// A value that asks for nothing (--read-only=false, an empty --exclude-tools,
// a --gitlab-url naming the instance stdio connects to) costs nothing when
// ignored, so it is only reported.
//
// The variables are spelled out rather than built from [config.EnvPrefix]:
// a concatenation in a package-level initializer is folded by the compiler,
// so it is a mutant no test can reach, and
// TestHTTPOnlyFlags_NameVariablesStdioReads already holds every spelling to a
// variable stdio reads.
type httpOnlyFlag struct {
	// stdioVariable is the variable that carries the same setting on stdio,
	// or empty where stdio has no such setting at all: the listener, the pool,
	// the authentication gate and the OAuth metadata exist only in HTTP mode.
	stdioVariable string
	// refusal, where set, reads the value the flag was given and returns the
	// line a stdio run refuses to start with and the setting to write in the
	// flag's place, or an empty line when ignoring that value costs only the
	// setting. It is handed stdioVariable, which is what the setting names.
	refusal func(variable string, hcfg *httpConfig) (line, setInstead string)
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
	"max-request-body-bytes":     {stdioVariable: stdioMaxLineBytesEnv},
	"session-timeout":            {},
	"http-idle-timeout":          {},
	"drain-delay":                {},
	"gitlab-url":                 {stdioVariable: "GITLAB_URL", refusal: refuseUnnamedInstance},
	"allow-any-gitlab-url":       {},
	"skip-tls-verify":            {stdioVariable: "GITLAB_MCP_SKIP_TLS_VERIFY"},
	"tier":                       {stdioVariable: "GITLAB_MCP_TIER"},
	"ignore-scopes":              {stdioVariable: "GITLAB_MCP_IGNORE_SCOPES"},
	"tool-surface":               {stdioVariable: "GITLAB_MCP_TOOL_SURFACE"},
	"capability-surface":         {stdioVariable: "GITLAB_MCP_CAPABILITY_SURFACE"},
	"meta-param-schema":          {stdioVariable: "GITLAB_MCP_META_PARAM_SCHEMA"},
	"embedded-resources":         {stdioVariable: "GITLAB_MCP_EMBEDDED_RESOURCES"},
	"exclude-tools":              {stdioVariable: "GITLAB_MCP_EXCLUDE_TOOLS", refusal: refuseExcludedTools},
	"read-only":                  {stdioVariable: "GITLAB_MCP_READ_ONLY", refusal: refuseWhenOn(func(h *httpConfig) bool { return h.readOnly })},
	"safe-mode":                  {stdioVariable: "GITLAB_MCP_SAFE_MODE", refusal: refuseWhenOn(func(h *httpConfig) bool { return h.safeMode })},
	"action-timeout":             {stdioVariable: "GITLAB_MCP_ACTION_TIMEOUT"},
	"rate-limit-rps":             {stdioVariable: "GITLAB_MCP_RATE_LIMIT_RPS"},
	"rate-limit-burst":           {stdioVariable: "GITLAB_MCP_RATE_LIMIT_BURST"},
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

// refuseWhenOn is the refusal of a flag that switches a protection on: the
// value asking for it cannot be ignored, and false asks for nothing.
func refuseWhenOn(on func(*httpConfig) bool) func(string, *httpConfig) (string, string) {
	return func(variable string, hcfg *httpConfig) (string, string) {
		if !on(hcfg) {
			return "", ""
		}
		return stdioWithheldLine, variable + "=true"
	}
}

// refuseExcludedTools refuses an --exclude-tools that names anything, read the
// way both transports read the list ([config.ParseCSV]), so a value of blanks
// and commas names nothing and is only reported. The setting it names carries
// the list as read, which is what stdio would exclude.
func refuseExcludedTools(variable string, hcfg *httpConfig) (line, setInstead string) {
	excluded := strings.Join(config.ParseCSV(hcfg.excludeTools), ",")
	if excluded == "" {
		return "", ""
	}
	return stdioWithheldLine, variable + "=" + excluded
}

// refuseUnnamedInstance refuses a --gitlab-url whose instances do not include
// the one a stdio run connects to, since that run would send GITLAB_TOKEN to
// an instance the command line did not name. An empty --gitlab-url names no
// instance and is only reported.
func refuseUnnamedInstance(variable string, hcfg *httpConfig) (line, setInstead string) {
	if len(hcfg.gitlabURLs) == 0 || namesStdioInstance(hcfg.gitlabURLs) {
		return "", ""
	}
	return stdioInstanceLine, variable
}

// stdioInstance is the instance a stdio run connects to, resolved the way
// [config.Load] resolves it: GITLAB_URL, or [config.DefaultGitLabURL] when that
// is blank. TestStdioInstance_IsTheOneConfigLoadResolves holds the two
// together, since config keeps its own resolution unexported.
func stdioInstance() string {
	if named := strings.TrimSpace(config.Getenv("GITLAB_URL")); named != "" {
		return named
	}
	return config.DefaultGitLabURL
}

// namesStdioInstance reports whether the instances --gitlab-url names include
// the one a stdio run connects to, compared in the canonical form the HTTP
// allow-list compares in ([serverpool.NormalizeGitLabURLs]), so
// https://GitLab.example.com:443/ names https://gitlab.example.com. A
// spelling either side cannot canonicalize matches nothing: HTTP mode would
// refuse such a flag outright, and a GITLAB_URL the allow-list could not read
// cannot be shown to be the instance the flag names.
func namesStdioInstance(named []string) bool {
	connected, err := serverpool.NormalizeGitLabURLs([]string{stdioInstance()})
	if err != nil {
		return false
	}
	canonical, err := serverpool.NormalizeGitLabURLs(named)
	if err != nil {
		return false
	}
	return slices.Contains(canonical, connected[0])
}

// stdioRefusal is one flag a stdio run refuses to start with: the flag as the
// operator typed it, the line saying why, and the setting to write instead.
type stdioRefusal struct {
	flag, line, setInstead string
}

// stdioIgnoredFlags sorts the HTTP-only flags hcfg records as passed into the
// ones a stdio run must refuse and the ones it only reports, each in name
// order. A refused flag is not reported as well: its refusal already says what
// it is and what to set instead.
func stdioIgnoredFlags(hcfg *httpConfig) (refusals []stdioRefusal, ignored []string) {
	for _, name := range slices.Sorted(maps.Keys(hcfg.setFlags)) {
		entry, httpOnly := httpOnlyFlags[name]
		if !httpOnly {
			continue
		}
		if entry.refusal != nil {
			if line, setInstead := entry.refusal(entry.stdioVariable, hcfg); line != "" {
				refusals = append(refusals, stdioRefusal{flag: "--" + name, line: line, setInstead: setInstead})
				continue
			}
		}
		ignored = append(ignored, name)
	}
	return refusals, ignored
}

// describeIgnoredFlag spells one ignored flag for the startup line, with the
// variable a stdio deployment sets instead where it has one.
func describeIgnoredFlag(name string) string {
	if variable := httpOnlyFlags[name].stdioVariable; variable != "" {
		return "--" + name + " (on stdio set " + variable + ")"
	}
	return "--" + name + " (stdio has no such setting)"
}

// The four lines [reportStdioIgnoredFlags] writes. test/e2e/stdio reads them
// off the real binary's stderr, spelled out there because it cannot import
// this package. Each is one literal, for the reason [httpOnlyFlag] gives, and
// TestStdioInstanceLine_NamesTheDefaultInstance holds the instance the last
// one names to [config.DefaultGitLabURL].
const (
	stdioIgnoredFlagsLine     = "these flags are read in HTTP mode only, and a stdio server takes its configuration from the environment, so they have no effect"
	stdioAutoIgnoredFlagsLine = "--transport=auto chose stdio, which has none of the settings these flags configure, so they have no effect on this run"
	stdioWithheldLine         = "a flag withholding part of what this server serves was passed to a stdio server, which reads it in HTTP mode only, so the flag has no effect here and this deployment will not be started with it: set the variable named instead and remove the flag"
	stdioInstanceLine         = "--gitlab-url names no instance this stdio server connects to: stdio connects to the one GITLAB_URL names, https://gitlab.com when it is unset, and sends GITLAB_TOKEN there, so this deployment will not be started with the flag: set GITLAB_URL to the instance and remove the flag"
)

// reportStdioIgnoredFlags tells the operator of a stdio run which of the flags
// it was given only HTTP mode reads, and reports whether startup may continue.
//
// An HTTP run reads them all and has nothing to report. A stdio run logs each
// flag it ignores at WARN, with the variable to set instead where stdio has
// one. Under --transport=auto a flag stdio has no setting for at all (the
// listener, the pool, the authentication gate) is named at INFO instead, on a
// line of its own: that operator wrote one command line for either transport,
// which is what the container image's own command does with --http-addr, so
// such a flag there is expected rather than a mistake, while still not silent.
// A flag stdio has a variable for stays at WARN under auto, since a setting
// written once for either transport was plausibly meant for both. A refusal is
// written in either case, one ERROR line each, and startup stops.
func reportStdioIgnoredFlags(choice transportDecision, hcfg *httpConfig) bool {
	if choice.HTTP {
		return true
	}
	refusals, ignored := stdioIgnoredFlags(hcfg)
	var warned, informed []string
	for _, name := range ignored {
		if choice.Inference != "" && httpOnlyFlags[name].stdioVariable == "" {
			informed = append(informed, describeIgnoredFlag(name))
			continue
		}
		warned = append(warned, describeIgnoredFlag(name))
	}
	if len(informed) > 0 {
		slog.Info(stdioAutoIgnoredFlagsLine, "flags", informed)
	}
	if len(warned) > 0 {
		slog.Warn(stdioIgnoredFlagsLine, "flags", warned)
	}
	for _, refused := range refusals {
		slog.Error(refused.line, "flag", refused.flag, "set_instead", refused.setInstead) //#nosec G706 -- the line is one of this file's constants and the setting names a variable from the table above
	}
	return len(refusals) == 0
}
