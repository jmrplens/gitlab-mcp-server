package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// TestBuildIdentifier_RendersOneComparableLabelForEveryBuildShape pins the
// label /health and the startup log report for a build.
//
// A release binary is stamped with a plain version; a build from main is
// stamped with a Go pseudo-version, which names the patch that does not exist
// yet. The label folds both into "closest release + short commit" so a
// monitor comparing instances sees the same shape whichever way they were
// built, with ".dirty" when the tree had uncommitted changes.
func TestBuildIdentifier_RendersOneComparableLabelForEveryBuildShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
		commit  string
		want    string
	}{
		{name: "a release with its commit", version: "2.7.5", commit: "404e3671a2b3c4d5", want: "2.7.5+404e367"},
		{name: "a release with no commit recorded", version: "2.7.5", commit: "none", want: "2.7.5"},
		{name: "a release with a blank commit", version: "2.7.5", commit: "  ", want: "2.7.5"},
		{name: "a pseudo-version names the previous patch", version: "2.7.6-0.20260903061404-6e6ff5beb20e", commit: "6e6ff5beb20e", want: "2.7.5+6e6ff5b"},
		{name: "a dirty pseudo-version says so", version: "2.7.6-0.20260903061404-6e6ff5beb20e+dirty", commit: "6e6ff5beb20e", want: "2.7.5+6e6ff5b.dirty"},
		{name: "a pseudo-version supplies the commit when none was stamped", version: "2.8.1-0.20260903061404-abcdef123456", commit: "none", want: "2.8.0+abcdef1"},
		{name: "a pseudo-version at patch zero keeps it", version: "3.0.0-0.20260903061404-abcdef123456", commit: "", want: "3.0.0+abcdef1"},
		{name: "a dirty release", version: "2.7.5+dirty", commit: "404e367", want: "2.7.5+404e367.dirty"},
		{name: "a short commit is kept whole", version: "dev", commit: "abc", want: "dev+abc"},
		{name: "an unstamped build", version: "dev", commit: "", want: "dev"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := buildIdentifier(tc.version, tc.commit); got != tc.want {
				t.Errorf("buildIdentifier(%q, %q) = %q, want %q", tc.version, tc.commit, got, tc.want)
			}
		})
	}
}

// TestConfigDigest_AgreesOnlyWhenTheServedSurfaceWouldAgree pins that the
// digest changes with every setting that decides what a client sees, and
// with nothing else.
//
// The point of the digest is to let a monitor compare the instances behind
// one balancer: two of them serving different catalogs must differ, and two
// configured alike, including the same exclusions in a different order, must
// match, since the order of an exclusion list is not a configuration.
func TestConfigDigest_AgreesOnlyWhenTheServedSurfaceWouldAgree(t *testing.T) {
	t.Parallel()

	base := func() *config.Config {
		return &config.Config{
			ToolSurface:       "dynamic",
			CapabilitySurface: "full",
			MetaParamSchema:   "opaque",
			ExcludeTools:      []string{"b", "a"},
		}
	}
	reference := configDigest(base())
	if len(reference) != 12 {
		t.Fatalf("digest %q is not twelve hex characters", reference)
	}

	tests := []struct {
		name   string
		mutate func(*config.Config)
		same   bool
	}{
		{name: "the same configuration", mutate: func(*config.Config) {}, same: true},
		{name: "the same exclusions in another order", mutate: func(c *config.Config) { c.ExcludeTools = []string{"a", "b"} }, same: true},
		{name: "a setting that shapes nothing a client sees", mutate: func(c *config.Config) { c.SessionTimeout = time.Hour }, same: true},
		{name: "another tool surface", mutate: func(c *config.Config) { c.ToolSurface = "meta" }, same: false},
		{name: "another capability surface", mutate: func(c *config.Config) { c.CapabilitySurface = "minimal" }, same: false},
		{name: "another parameter schema", mutate: func(c *config.Config) { c.MetaParamSchema = "full" }, same: false},
		{name: "read-only", mutate: func(c *config.Config) { c.ReadOnly = true }, same: false},
		{name: "safe mode", mutate: func(c *config.Config) { c.SafeMode = true }, same: false},
		{name: "one more exclusion", mutate: func(c *config.Config) { c.ExcludeTools = append(c.ExcludeTools, "c") }, same: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := base()
			tc.mutate(cfg)
			got := configDigest(cfg)
			if (got == reference) != tc.same {
				t.Errorf("digest %q vs reference %q: same=%v, want same=%v", got, reference, got == reference, tc.same)
			}
		})
	}

	t.Run("no configuration at all", func(t *testing.T) {
		t.Parallel()
		if got := configDigest(nil); got != "" {
			t.Errorf("configDigest(nil) = %q, want empty", got)
		}
	})
}

// TestHealthHandler_ReportsDrainingAsServiceUnavailable pins the flip a
// balancer watches for: once shutdown is requested, /health answers 503 with
// status draining and forbids caching, so the last 200 is not served across
// the flip; until then it answers 200 with the build and the digest.
//
// The flag is injected rather than the process-wide one, because every other
// test in this package expects that one to stay clear.
func TestHealthHandler_ReportsDrainingAsServiceUnavailable(t *testing.T) {
	t.Parallel()

	var drain atomic.Bool
	handler := healthHandler("abc123def456", &drain)

	probe := func() (int, http.Header, healthResponse) {
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", http.NoBody))
		var body healthResponse
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decoding the body: %v", err)
		}
		return rec.Code, rec.Header(), body
	}

	code, header, body := probe()
	if code != http.StatusOK || body.Status != healthStatusOK {
		t.Fatalf("before draining: status %d body %q, want 200 ok", code, body.Status)
	}
	if body.ConfigDigest != "abc123def456" {
		t.Errorf("config_digest = %q, want the injected digest", body.ConfigDigest)
	}
	if body.Build != buildIdentifier(version, commit) {
		t.Errorf("build = %q, want %q", body.Build, buildIdentifier(version, commit))
	}
	if header.Get("Cache-Control") != "" {
		t.Errorf("a serving instance set Cache-Control %q; the flip is what must not be cached", header.Get("Cache-Control"))
	}

	drain.Store(true)
	code, header, body = probe()
	if code != http.StatusServiceUnavailable || body.Status != healthStatusDraining {
		t.Fatalf("while draining: status %d body %q, want 503 draining", code, body.Status)
	}
	if header.Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q while draining, want no-store", header.Get("Cache-Control"))
	}
	if header.Get(hdrContentType) != mimeJSON {
		t.Errorf("Content-Type = %q while draining, want %s", header.Get(hdrContentType), mimeJSON)
	}
}

// TestNewHealthResponse_CarriesTheDrainingStatus pins that the body's status
// follows the flag it was built with, independently of the HTTP layer.
func TestNewHealthResponse_CarriesTheDrainingStatus(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	if got := newHealthResponse(now, now, "d1", false).Status; got != healthStatusOK {
		t.Errorf("status = %q, want %q", got, healthStatusOK)
	}
	got := newHealthResponse(now, now, "d1", true)
	if got.Status != healthStatusDraining {
		t.Errorf("status = %q, want %q", got.Status, healthStatusDraining)
	}
	if got.ConfigDigest != "d1" {
		t.Errorf("config_digest = %q, want d1", got.ConfigDigest)
	}
}
