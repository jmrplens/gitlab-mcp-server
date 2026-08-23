// envoverlay.go applies environment variables underneath the HTTP-mode flags.
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

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
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
}

// overlayString assigns value to target when the flag was not passed and the
// environment supplied one.
func overlayString(hcfg *httpConfig, flagName string, target, value *string) {
	if value != nil && !hcfg.setFlags[flagName] {
		*target = *value
	}
}

func overlayBool(hcfg *httpConfig, flagName string, target, value *bool) {
	if value != nil && !hcfg.setFlags[flagName] {
		*target = *value
	}
}

func applyOverlayStrings(hcfg *httpConfig, o *config.HTTPEnvOverlay) {
	overlayString(hcfg, "gitlab-url", &hcfg.gitlabURL, o.GitLabURL)
	overlayString(hcfg, "tool-surface", &hcfg.toolSurface, o.ToolSurface)
	overlayString(hcfg, "capability-surface", &hcfg.capabilitySurface, o.CapabilitySurface)
	overlayString(hcfg, "meta-param-schema", &hcfg.metaParamSchema, o.MetaParamSchema)
	overlayString(hcfg, "exclude-tools", &hcfg.excludeTools, o.ExcludeTools)
	overlayString(hcfg, "auth-mode", &hcfg.authMode, o.AuthMode)
	overlayString(hcfg, "auto-update", &hcfg.autoUpdate, o.AutoUpdate)
	overlayString(hcfg, "auto-update-repo", &hcfg.autoUpdateRepo, o.AutoUpdateRepo)
}

func applyOverlayBools(hcfg *httpConfig, o *config.HTTPEnvOverlay) {
	overlayBool(hcfg, "skip-tls-verify", &hcfg.skipTLSVerify, o.SkipTLSVerify)
	overlayBool(hcfg, "read-only", &hcfg.readOnly, o.ReadOnly)
	overlayBool(hcfg, "safe-mode", &hcfg.safeMode, o.SafeMode)
	overlayBool(hcfg, "embedded-resources", &hcfg.embeddedResources, o.EmbeddedResources)
	overlayBool(hcfg, "ignore-scopes", &hcfg.ignoreScopes, o.IgnoreScopes)

	// META_TOOLS is the deprecated selector. It only reaches the flag layer
	// when the operator passed neither --tool-surface nor --meta-tools, so an
	// explicit surface flag always beats a stale environment variable.
	if o.MetaTools != nil && !hcfg.setFlags["meta-tools"] && !hcfg.setFlags["tool-surface"] {
		hcfg.metaTools = *o.MetaTools
		hcfg.metaToolsSet = true
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
		{"auto-update-interval", &hcfg.autoUpdateInterval, o.AutoUpdateInterval},
		{"auto-update-timeout", &hcfg.autoUpdateTimeout, o.AutoUpdateTimeout},
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
