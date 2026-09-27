// env_overlay_test.go verifies the HTTP-mode configuration precedence:
// an explicitly passed flag, then the environment, then the built-in default.
package main

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
)

// newOverlayConfig returns an httpConfig carrying the flag defaults, with the
// named flags marked as explicitly passed.
func newOverlayConfig(setFlags ...string) *httpConfig {
	hcfg := &httpConfig{
		sessionTimeout:     config.DefaultSessionTimeout,
		poolIdleTimeout:    config.DefaultPoolIdleTimeout,
		revalidateInterval: config.DefaultRevalidateInterval,
		maxHTTPClients:     config.DefaultMaxHTTPClients,
		capabilitySurface:  config.DefaultCapabilitySurface,
		setFlags:           map[string]bool{},
	}
	for _, name := range setFlags {
		hcfg.setFlags[name] = true
	}
	return hcfg
}

// TestApplyHTTPEnvOverlay_FlagBeatsEnvironment verifies the top of the
// precedence order: a flag the operator actually passed is never replaced by
// the environment. This is the property that keeps a deployment's command line
// authoritative over stray variables inherited from an image or a shell.
func TestApplyHTTPEnvOverlay_FlagBeatsEnvironment(t *testing.T) {
	hcfg := newOverlayConfig("pool-idle-timeout", "session-timeout", "read-only")
	hcfg.poolIdleTimeout = 15 * time.Minute
	hcfg.sessionTimeout = 5 * time.Minute
	hcfg.readOnly = true

	envPool := 6 * time.Hour
	envSession := 2 * time.Hour
	envReadOnly := false
	applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{
		PoolIdleTimeout: &envPool,
		SessionTimeout:  &envSession,
		ReadOnly:        &envReadOnly,
	})

	if hcfg.poolIdleTimeout != 15*time.Minute {
		t.Errorf("poolIdleTimeout = %v, want the flag value 15m", hcfg.poolIdleTimeout)
	}
	if hcfg.sessionTimeout != 5*time.Minute {
		t.Errorf("sessionTimeout = %v, want the flag value 5m", hcfg.sessionTimeout)
	}
	if !hcfg.readOnly {
		t.Error("readOnly = false, want the flag value true")
	}
}

// TestApplyHTTPEnvOverlay_EnvironmentBeatsDefault verifies the middle of the
// order: when no flag was passed, the environment supplies the value instead
// of the built-in default. Before this layer existed the flag default always
// won, which made every documented HTTP environment variable inert.
func TestApplyHTTPEnvOverlay_EnvironmentBeatsDefault(t *testing.T) {
	hcfg := newOverlayConfig()

	envPool := 6 * time.Hour
	envClients := 250
	envSurface := config.ToolSurfaceMeta
	envReadOnly := true
	applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{
		PoolIdleTimeout: &envPool,
		MaxHTTPClients:  &envClients,
		ToolSurface:     &envSurface,
		ReadOnly:        &envReadOnly,
	})

	if hcfg.poolIdleTimeout != 6*time.Hour {
		t.Errorf("poolIdleTimeout = %v, want the environment value 6h", hcfg.poolIdleTimeout)
	}
	if hcfg.maxHTTPClients != 250 {
		t.Errorf("maxHTTPClients = %d, want the environment value 250", hcfg.maxHTTPClients)
	}
	if hcfg.toolSurface != config.ToolSurfaceMeta {
		t.Errorf("toolSurface = %q, want the environment value %q", hcfg.toolSurface, config.ToolSurfaceMeta)
	}
	if !hcfg.readOnly {
		t.Error("readOnly = false, want the environment value true")
	}
}

// TestApplyHTTPEnvOverlay_AbsentEnvironmentKeepsDefault verifies the bottom of
// the order: a nil overlay field means the variable was absent, so whatever the
// flag layer resolved survives untouched.
func TestApplyHTTPEnvOverlay_AbsentEnvironmentKeepsDefault(t *testing.T) {
	hcfg := newOverlayConfig()
	applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{})

	if hcfg.poolIdleTimeout != config.DefaultPoolIdleTimeout {
		t.Errorf("poolIdleTimeout = %v, want the default %v", hcfg.poolIdleTimeout, config.DefaultPoolIdleTimeout)
	}
	if hcfg.maxHTTPClients != config.DefaultMaxHTTPClients {
		t.Errorf("maxHTTPClients = %d, want the default %d", hcfg.maxHTTPClients, config.DefaultMaxHTTPClients)
	}
}

// TestApplyHTTPEnvOverlay_TierOnlyPins verifies that the environment can pin
// the tier but never un-pin it. A non-explicit tier must leave detection in
// place, because clearing tierSet would silently switch a deployment from its
// configured tier to per-instance detection.
func TestApplyHTTPEnvOverlay_TierOnlyPins(t *testing.T) {
	t.Run("explicit environment tier pins it", func(t *testing.T) {
		hcfg := newOverlayConfig()
		tier := edition.Ultimate
		applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{Tier: &tier, TierExplicit: true})

		if !hcfg.tierSet || hcfg.tier != tier.String() {
			t.Errorf("tier = %q set = %v, want %q pinned", hcfg.tier, hcfg.tierSet, tier)
		}
	})

	t.Run("non-explicit environment tier leaves detection alone", func(t *testing.T) {
		hcfg := newOverlayConfig()
		tier := edition.Free
		applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{Tier: &tier, TierExplicit: false})

		if hcfg.tierSet {
			t.Error("tierSet = true, want detection preserved")
		}
	})

	t.Run("the tier flag wins over the environment", func(t *testing.T) {
		hcfg := newOverlayConfig("tier")
		hcfg.tier, hcfg.tierSet = "premium", true
		tier := edition.Ultimate
		applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{Tier: &tier, TierExplicit: true})

		if hcfg.tier != "premium" {
			t.Errorf("tier = %q, want the flag value premium", hcfg.tier)
		}
	})
}

// TestApplyHTTPEnvOverlay_SurfaceFlagBeatsEnv verifies that an explicit
// --tool-surface is not overridden by the environment variable of the same
// meaning.
func TestApplyHTTPEnvOverlay_SurfaceFlagBeatsEnv(t *testing.T) {
	hcfg := newOverlayConfig("tool-surface")
	hcfg.toolSurface = config.ToolSurfaceDynamic

	surface := config.ToolSurfaceMeta
	applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{ToolSurface: &surface})

	if hcfg.toolSurface != config.ToolSurfaceDynamic {
		t.Errorf("toolSurface = %q, want the explicit flag value to win", hcfg.toolSurface)
	}
}

// TestApplyHTTPEnvOverlay_NilInputsAreNoOps verifies the guard clauses, so a
// caller that has no overlay cannot panic the startup path.
func TestApplyHTTPEnvOverlay_NilInputsAreNoOps(t *testing.T) {
	applyHTTPEnvOverlay(nil, &config.HTTPEnvOverlay{})

	hcfg := newOverlayConfig()
	applyHTTPEnvOverlay(hcfg, nil)
	if hcfg.poolIdleTimeout != config.DefaultPoolIdleTimeout {
		t.Errorf("poolIdleTimeout = %v, want it untouched", hcfg.poolIdleTimeout)
	}
}

// TestApplyHTTPEnvOverlay_OAuthOriginSettingsFollowPrecedence verifies both
// halves of the precedence rule for the two settings a containerised OAuth
// deployment can only supply through the environment. Before they were
// overlaid, AUTH_MODE=oauth reached the configuration but PUBLIC_URL did not,
// so such a deployment failed at startup demanding a flag it had no way to
// pass.
func TestApplyHTTPEnvOverlay_OAuthOriginSettingsFollowPrecedence(t *testing.T) {
	envPublicURL := "https://env.example.com/gitlab"
	envOrigins := "https://env-origin.example"
	// The application allow-list joined the overlay late: the variable was
	// documented for HTTP mode and read by nothing there, so only the flag
	// admitted an application.
	envClientUID := "12ab,34cd"

	t.Run("environment fills an unpassed flag", func(t *testing.T) {
		hcfg := newOverlayConfig()
		applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{
			PublicURL:      &envPublicURL,
			TrustedOrigins: &envOrigins,
			OAuthClientUID: &envClientUID,
		})
		if hcfg.publicURL != envPublicURL {
			t.Errorf("publicURL = %q, want the environment value %q", hcfg.publicURL, envPublicURL)
		}
		if hcfg.trustedOrigins != envOrigins {
			t.Errorf("trustedOrigins = %q, want the environment value %q", hcfg.trustedOrigins, envOrigins)
		}
		if hcfg.oauthClientUID != envClientUID {
			t.Errorf("oauthClientUID = %q, want the environment value %q", hcfg.oauthClientUID, envClientUID)
		}
	})

	t.Run("passed flag beats the environment", func(t *testing.T) {
		hcfg := newOverlayConfig("public-url", "trusted-origins", "oauth-client-uid")
		hcfg.publicURL = "https://flag.example.com"
		hcfg.trustedOrigins = "https://flag-origin.example"
		hcfg.oauthClientUID = "flag-uid"
		applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{
			PublicURL:      &envPublicURL,
			TrustedOrigins: &envOrigins,
			OAuthClientUID: &envClientUID,
		})
		if hcfg.publicURL != "https://flag.example.com" {
			t.Errorf("publicURL = %q, want the flag value", hcfg.publicURL)
		}
		if hcfg.trustedOrigins != "https://flag-origin.example" {
			t.Errorf("trustedOrigins = %q, want the flag value", hcfg.trustedOrigins)
		}
		if hcfg.oauthClientUID != "flag-uid" {
			t.Errorf("oauthClientUID = %q, want the flag value", hcfg.oauthClientUID)
		}
	})
}

// TestApplyHTTPEnvOverlay_SettingsWithoutTheirOwnTest covers the overlay
// entries that no other case exercises: the instance list, the deprecated
// boolean surface selector, and the two rate-limit numbers.
//
// GITLAB_URL is the one that is not a plain assignment. The environment carries
// one string where the flag can be repeated, so a comma-separated value has to
// spell the same list — and it replaces whatever the flag layer left rather
// than appending to it, or a value from a previous parse would survive
// underneath the operator's own.
func TestApplyHTTPEnvOverlay_SettingsWithoutTheirOwnTest(t *testing.T) {
	t.Parallel()

	t.Run("the instance list comes from one comma-separated value", func(t *testing.T) {
		t.Parallel()

		hcfg := newOverlayConfig()
		hcfg.gitlabURLs = repeatedFlag{"https://left-over.example.com"}
		value := "https://gitlab.com, https://gitlab.example.com"

		applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{GitLabURL: &value})

		want := repeatedFlag{"https://gitlab.com", "https://gitlab.example.com"}
		if !slices.Equal(hcfg.gitlabURLs, want) {
			t.Errorf("gitlabURLs = %v, want %v", hcfg.gitlabURLs, want)
		}
	})

	t.Run("a passed flag keeps the instance list", func(t *testing.T) {
		t.Parallel()

		hcfg := newOverlayConfig("gitlab-url")
		hcfg.gitlabURLs = repeatedFlag{"https://from-the-flag.example.com"}
		value := "https://from-the-environment.example.com"

		applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{GitLabURL: &value})

		if !slices.Equal(hcfg.gitlabURLs, repeatedFlag{"https://from-the-flag.example.com"}) {
			t.Errorf("gitlabURLs = %v, want the flag value kept", hcfg.gitlabURLs)
		}
	})

	t.Run("the rate limit comes from the environment", func(t *testing.T) {
		t.Parallel()

		hcfg := newOverlayConfig()
		rps, burst := 42.5, 99

		applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{RateLimitRPS: &rps, RateLimitBurst: &burst})

		if hcfg.rateLimitRPS != rps {
			t.Errorf("rateLimitRPS = %v, want %v", hcfg.rateLimitRPS, rps)
		}
		if hcfg.rateLimitBurst != burst {
			t.Errorf("rateLimitBurst = %d, want %d", hcfg.rateLimitBurst, burst)
		}
	})
}

// overlaySetting is one HTTP setting as the overlay sees it: the flag an
// operator types for it, the variable that supplies it, and the field it lands
// in.
type overlaySetting struct {
	flag string
	env  func(*config.HTTPEnvOverlay)
	want func(*httpConfig)
}

// overlaySettings lists every setting the overlay carries, each with a value
// its default does not hold. The flag names are spelled as main registers
// them, so an overlay consulting a misspelled or a neighboring flag is caught.
func overlaySettings() []overlaySetting {
	str := func(s string) *string { return &s }
	yes := func() *bool { b := true; return &b }
	num := func(n int) *int { return &n }
	dur := func(d time.Duration) *time.Duration { return &d }
	ultimate := edition.Ultimate
	return []overlaySetting{
		{
			"gitlab-url", func(o *config.HTTPEnvOverlay) { o.GitLabURL = str("https://env.example.com") },
			func(h *httpConfig) { h.gitlabURLs = repeatedFlag{"https://env.example.com"} },
		},
		{
			"tool-surface", func(o *config.HTTPEnvOverlay) { o.ToolSurface = str(config.ToolSurfaceMeta) },
			func(h *httpConfig) { h.toolSurface = config.ToolSurfaceMeta },
		},
		{
			"capability-surface", func(o *config.HTTPEnvOverlay) { o.CapabilitySurface = str(config.CapabilitySurfaceMinimal) },
			func(h *httpConfig) { h.capabilitySurface = config.CapabilitySurfaceMinimal },
		},
		{
			"meta-param-schema", func(o *config.HTTPEnvOverlay) { o.MetaParamSchema = str(config.MetaParamSchemaCompact) },
			func(h *httpConfig) { h.metaParamSchema = config.MetaParamSchemaCompact },
		},
		{
			"exclude-tools", func(o *config.HTTPEnvOverlay) { o.ExcludeTools = str("gitlab_admin") },
			func(h *httpConfig) { h.excludeTools = "gitlab_admin" },
		},
		{
			"auth-mode", func(o *config.HTTPEnvOverlay) { o.AuthMode = str(config.AuthModeOAuth) },
			func(h *httpConfig) { h.authMode = config.AuthModeOAuth },
		},
		{
			"public-url", func(o *config.HTTPEnvOverlay) { o.PublicURL = str("https://env.example.com/mcp") },
			func(h *httpConfig) { h.publicURL = "https://env.example.com/mcp" },
		},
		{
			"trusted-origins", func(o *config.HTTPEnvOverlay) { o.TrustedOrigins = str("https://origin.example") },
			func(h *httpConfig) { h.trustedOrigins = "https://origin.example" },
		},
		{
			"oauth-client-uid", func(o *config.HTTPEnvOverlay) { o.OAuthClientUID = str("env-uid") },
			func(h *httpConfig) { h.oauthClientUID = "env-uid" },
		},
		{
			"skip-tls-verify", func(o *config.HTTPEnvOverlay) { o.SkipTLSVerify = yes() },
			func(h *httpConfig) { h.skipTLSVerify = true },
		},
		{
			"read-only", func(o *config.HTTPEnvOverlay) { o.ReadOnly = yes() },
			func(h *httpConfig) { h.readOnly = true },
		},
		{
			"safe-mode", func(o *config.HTTPEnvOverlay) { o.SafeMode = yes() },
			func(h *httpConfig) { h.safeMode = true },
		},
		{
			"embedded-resources", func(o *config.HTTPEnvOverlay) { o.EmbeddedResources = yes() },
			func(h *httpConfig) { h.embeddedResources = true },
		},
		{
			"ignore-scopes", func(o *config.HTTPEnvOverlay) { o.IgnoreScopes = yes() },
			func(h *httpConfig) { h.ignoreScopes = true },
		},
		{
			"max-http-clients", func(o *config.HTTPEnvOverlay) { o.MaxHTTPClients = num(250) },
			func(h *httpConfig) { h.maxHTTPClients = 250 },
		},
		{
			"rate-limit-rps", func(o *config.HTTPEnvOverlay) { rps := 42.5; o.RateLimitRPS = &rps },
			func(h *httpConfig) { h.rateLimitRPS = 42.5 },
		},
		{
			"rate-limit-burst", func(o *config.HTTPEnvOverlay) { o.RateLimitBurst = num(99) },
			func(h *httpConfig) { h.rateLimitBurst = 99 },
		},
		{
			"auth-failure-limit", func(o *config.HTTPEnvOverlay) { o.AuthFailureLimit = num(7) },
			func(h *httpConfig) { h.authFailureLimit = 7 },
		},
		{
			"auth-failure-window", func(o *config.HTTPEnvOverlay) { o.AuthFailureWindow = dur(3 * time.Minute) },
			func(h *httpConfig) { h.authFailureWindow = 3 * time.Minute },
		},
		{
			"auth-distinct-token-limit", func(o *config.HTTPEnvOverlay) { o.AuthDistinctTokenLimit = num(70) },
			func(h *httpConfig) { h.authDistinctLimit = 70 },
		},
		{
			"auth-distinct-token-window", func(o *config.HTTPEnvOverlay) { o.AuthDistinctWindow = dur(17 * time.Minute) },
			func(h *httpConfig) { h.authDistinctWindow = 17 * time.Minute },
		},
		{
			"session-timeout", func(o *config.HTTPEnvOverlay) { o.SessionTimeout = dur(2 * time.Hour) },
			func(h *httpConfig) { h.sessionTimeout = 2 * time.Hour },
		},
		{
			"pool-idle-timeout", func(o *config.HTTPEnvOverlay) { o.PoolIdleTimeout = dur(6 * time.Hour) },
			func(h *httpConfig) { h.poolIdleTimeout = 6 * time.Hour },
		},
		{
			"revalidate-interval", func(o *config.HTTPEnvOverlay) { o.RevalidateInterval = dur(45 * time.Minute) },
			func(h *httpConfig) { h.revalidateInterval = 45 * time.Minute },
		},
		{
			"oauth-cache-ttl", func(o *config.HTTPEnvOverlay) { o.OAuthCacheTTL = dur(25 * time.Minute) },
			func(h *httpConfig) { h.oauthCacheTTL = 25 * time.Minute },
		},
		{
			"action-timeout", func(o *config.HTTPEnvOverlay) { o.ActionTimeout = dur(90 * time.Minute) },
			func(h *httpConfig) { h.actionTimeout = 90 * time.Minute },
		},
		{
			"drain-delay", func(o *config.HTTPEnvOverlay) { o.DrainDelay = dur(20 * time.Second) },
			func(h *httpConfig) { h.drainDelay = 20 * time.Second },
		},
		{
			"tier", func(o *config.HTTPEnvOverlay) { o.Tier, o.TierExplicit = &ultimate, true },
			func(h *httpConfig) { h.tier, h.tierSet = ultimate.String(), true },
		},
	}
}

// TestApplyHTTPEnvOverlay_EachSettingLandsInItsOwnFieldAndYieldsToItsOwnFlag
// holds every overlay entry to the one field it names and the one flag that
// outranks it. Each entry is a plain assignment, so a value written into a
// neighbour's field, or held back by a neighbour's flag, is invisible to every
// operator-flipping tool and to any test that sets two settings to one value.
// Applied alone, a variable must change its own field and nothing else, and
// its own flag, passed alone, must leave the configuration exactly as the flag
// layer resolved it.
func TestApplyHTTPEnvOverlay_EachSettingLandsInItsOwnFieldAndYieldsToItsOwnFlag(t *testing.T) {
	t.Parallel()

	for _, tc := range overlaySettings() {
		t.Run(tc.flag, func(t *testing.T) {
			t.Parallel()

			overlay := &config.HTTPEnvOverlay{}
			tc.env(overlay)

			got := newOverlayConfig()
			applyHTTPEnvOverlay(got, overlay)
			want := newOverlayConfig()
			tc.want(want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("the environment alone changed the configuration to\n%+v\nwant only this setting changed:\n%+v", got, want)
			}

			held := newOverlayConfig(tc.flag)
			applyHTTPEnvOverlay(held, overlay)
			if untouched := newOverlayConfig(tc.flag); !reflect.DeepEqual(held, untouched) {
				t.Errorf("--%s was passed, yet the environment changed the configuration to\n%+v", tc.flag, held)
			}
		})
	}
}

// TestOverlaySettings_CoverEveryOverlayField keeps the table above complete:
// a variable added to the overlay without a row there would be checked by
// nothing, so every pointer field of the overlay must be filled by exactly one
// row.
func TestOverlaySettings_CoverEveryOverlayField(t *testing.T) {
	t.Parallel()

	filledBy := map[string]string{}
	for _, tc := range overlaySettings() {
		overlay := config.HTTPEnvOverlay{}
		tc.env(&overlay)
		value := reflect.ValueOf(overlay)
		for i := range value.NumField() {
			field := value.Type().Field(i)
			if field.Type.Kind() != reflect.Pointer || value.Field(i).IsNil() {
				continue
			}
			if previous, taken := filledBy[field.Name]; taken {
				t.Errorf("%s is filled by the rows for --%s and --%s", field.Name, previous, tc.flag)
			}
			filledBy[field.Name] = tc.flag
		}
	}

	overlayType := reflect.TypeFor[config.HTTPEnvOverlay]()
	for field := range overlayType.Fields() {
		if field.Type.Kind() != reflect.Pointer {
			continue
		}
		if _, found := filledBy[field.Name]; !found {
			t.Errorf("HTTPEnvOverlay.%s has no row in overlaySettings, so nothing checks where it lands", field.Name)
		}
	}
}
