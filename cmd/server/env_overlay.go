// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// env_overlay.go applies environment variables underneath the HTTP-mode flags.
//
// HTTP mode built its configuration purely from flags, so every documented
// HTTP environment variable was inert: the flag default won even when the
// operator had exported a value and passed no flag. Precedence is now the one
// the reference documents — an explicitly passed flag, then the environment,
// then the built-in default.
//
// The request surface is unchanged and stays narrow: only the GitLab token
// and, when --gitlab-url was omitted, the GITLAB-URL header are
// client-controlled. Nothing here is reachable per request, so a client cannot
// influence any of it for itself or for anyone else.

package main

import (
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/i18n"
)

// applyHTTPEnvOverlay fills in every HTTP setting whose flag the operator did
// not pass explicitly, using the value found in the environment. Flags that
// were passed are left untouched, so the command line always wins.
func applyHTTPEnvOverlay(hcfg *httpConfig, overlay *config.HTTPEnvOverlay) {
	if hcfg == nil || overlay == nil {
		return
	}

	applyOverlayStrings(hcfg, overlay)
	applyOverlayBools(hcfg, overlay)
	applyOverlayNumbers(hcfg, overlay)
	applyOverlayDurations(hcfg, overlay)
	applyOverlayTier(hcfg, overlay)
	applyOverlayUploadLimit(hcfg, overlay)
	applyOverlayLanguage(hcfg, overlay)
}

// applyOverlayLanguage resolves the language the texts a person reads are
// rendered in, which has no flag of its own in httpConfig either: --language
// writes the variable (env_flags.go), so the overlay's value is the flag's
// when it was passed. With neither it is the default, set here because no
// flag declaration holds it. The overlay has already refused a language the
// server has no catalog for, before any listener opens.
func applyOverlayLanguage(hcfg *httpConfig, o *config.HTTPEnvOverlay) {
	hcfg.language = i18n.Default
	if o.Language != nil {
		hcfg.language = *o.Language
	}
}

// applyOverlayUploadLimit resolves the upload limit (register row RQB-007),
// the one HTTP setting with no flag of its own in httpConfig:
// --upload-max-file-size writes the variable (env_flags.go), so the
// overlay's value is the flag's when it was passed, and there is no passed
// flag here for it to give way to. With neither, it is the default, which is
// set here because no flag declaration holds it. The overlay has already
// refused a value that does not parse or lies above the ceiling (issue 1170).
func applyOverlayUploadLimit(hcfg *httpConfig, o *config.HTTPEnvOverlay) {
	hcfg.uploadMaxFileSize = config.DefaultMaxFileSize
	if o.UploadMaxFileSize != nil {
		hcfg.uploadMaxFileSize = *o.UploadMaxFileSize
	}
}

// overlayString assigns value to target when the flag was not passed and the
// environment supplied one.
func overlayString(hcfg *httpConfig, flagName string, target, value *string) {
	if value != nil && !hcfg.setFlags[flagName] {
		*target = *value
	}
}

// overlayGitLabURLs applies GITLAB_URL to the instance list.
//
// The environment carries one string where the flag can be repeated, so a
// comma-separated value spells the same list — the flag parser accepts that
// form too, for exactly this reason.
func overlayGitLabURLs(hcfg *httpConfig, value *string) {
	if value == nil || hcfg.setFlags["gitlab-url"] {
		return
	}
	hcfg.gitlabURLs = nil
	hcfg.gitlabURLs.add(*value)
}

func overlayBool(hcfg *httpConfig, flagName string, target, value *bool) {
	if value != nil && !hcfg.setFlags[flagName] {
		*target = *value
	}
}

func applyOverlayStrings(hcfg *httpConfig, o *config.HTTPEnvOverlay) {
	overlayGitLabURLs(hcfg, o.GitLabURL)
	overlayString(hcfg, "tool-surface", &hcfg.toolSurface, o.ToolSurface)
	overlayString(hcfg, "capability-surface", &hcfg.capabilitySurface, o.CapabilitySurface)
	overlayString(hcfg, "meta-param-schema", &hcfg.metaParamSchema, o.MetaParamSchema)
	overlayString(hcfg, "exclude-tools", &hcfg.excludeTools, o.ExcludeTools)
	overlayString(hcfg, "auth-mode", &hcfg.authMode, o.AuthMode)
	overlayString(hcfg, "public-url", &hcfg.publicURL, o.PublicURL)
	overlayString(hcfg, "trusted-origins", &hcfg.trustedOrigins, o.TrustedOrigins)
	overlayString(hcfg, "oauth-client-uid", &hcfg.oauthClientUID, o.OAuthClientUID)
}

func applyOverlayBools(hcfg *httpConfig, o *config.HTTPEnvOverlay) {
	overlayBool(hcfg, "skip-tls-verify", &hcfg.skipTLSVerify, o.SkipTLSVerify)
	overlayBool(hcfg, "read-only", &hcfg.readOnly, o.ReadOnly)
	overlayBool(hcfg, "safe-mode", &hcfg.safeMode, o.SafeMode)
	overlayBool(hcfg, "embedded-resources", &hcfg.embeddedResources, o.EmbeddedResources)
	overlayBool(hcfg, "ignore-scopes", &hcfg.ignoreScopes, o.IgnoreScopes)
	overlayPlaintextHTTP(hcfg, o.AllowPlaintextHTTP)
}

// overlayPlaintextHTTP applies GITLAB_MCP_ALLOW_PLAINTEXT_HTTP to the plain
// HTTP setting when --allow-plaintext-http was not passed. The setting tells
// a value the operator gave from the default, which is why it is not a plain
// boolean: the variable at true is a declaration, said at INFO, and unset is
// not.
func overlayPlaintextHTTP(hcfg *httpConfig, value *bool) {
	if value != nil && !hcfg.setFlags["allow-plaintext-http"] {
		hcfg.plaintextHTTP.setAllowed(*value)
	}
}

func applyOverlayNumbers(hcfg *httpConfig, o *config.HTTPEnvOverlay) {
	if o.MaxHTTPClients != nil && !hcfg.setFlags["max-http-clients"] {
		hcfg.maxHTTPClients = *o.MaxHTTPClients
	}
	if o.RateLimitRPS != nil && !hcfg.setFlags["rate-limit-rps"] {
		hcfg.rateLimitRPS = *o.RateLimitRPS
	}
	if o.RateLimitBurst != nil && !hcfg.setFlags["rate-limit-burst"] {
		hcfg.rateLimitBurst = *o.RateLimitBurst
	}
	if o.AuthFailureLimit != nil && !hcfg.setFlags["auth-failure-limit"] {
		hcfg.authFailureLimit = *o.AuthFailureLimit
	}
	if o.AuthFailureWindow != nil && !hcfg.setFlags["auth-failure-window"] {
		hcfg.authFailureWindow = *o.AuthFailureWindow
	}
	if o.AuthDistinctTokenLimit != nil && !hcfg.setFlags["auth-distinct-token-limit"] {
		hcfg.authDistinctLimit = *o.AuthDistinctTokenLimit
	}
	if o.AuthDistinctWindow != nil && !hcfg.setFlags["auth-distinct-token-window"] {
		hcfg.authDistinctWindow = *o.AuthDistinctWindow
	}
	if o.DiscoveryRateLimitRPS != nil && !hcfg.setFlags["discovery-rate-limit-rps"] {
		hcfg.discoveryRateLimitRPS = *o.DiscoveryRateLimitRPS
	}
	if o.DiscoveryRateLimitBurst != nil && !hcfg.setFlags["discovery-rate-limit-burst"] {
		hcfg.discoveryRateLimitBurst = *o.DiscoveryRateLimitBurst
	}
}

func applyOverlayDurations(hcfg *httpConfig, o *config.HTTPEnvOverlay) {
	durations := []struct {
		flagName string
		target   *time.Duration
		value    *time.Duration
	}{
		{"session-timeout", &hcfg.sessionTimeout, o.SessionTimeout},
		{"pool-idle-timeout", &hcfg.poolIdleTimeout, o.PoolIdleTimeout},
		{"revalidate-interval", &hcfg.revalidateInterval, o.RevalidateInterval},
		{"oauth-cache-ttl", &hcfg.oauthCacheTTL, o.OAuthCacheTTL},
		{"action-timeout", &hcfg.actionTimeout, o.ActionTimeout},
		{"drain-delay", &hcfg.drainDelay, o.DrainDelay},
	}
	for _, d := range durations {
		if d.value != nil && !hcfg.setFlags[d.flagName] {
			*d.target = *d.value
		}
	}
}

// applyOverlayTier mirrors the flag path's tier handling: the environment can
// only pin the tier, never un-pin it. When neither the flag nor the
// environment names one, the tier stays detected per pool entry.
func applyOverlayTier(hcfg *httpConfig, o *config.HTTPEnvOverlay) {
	if o.Tier == nil || hcfg.setFlags["tier"] {
		return
	}
	if !o.TierExplicit {
		return
	}
	hcfg.tier = o.Tier.String()
	hcfg.tierSet = true
}
