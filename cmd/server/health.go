// health.go answers /health: whether this process is serving or draining,
// which build it is, and a fingerprint of the configuration that shapes what
// it serves.
//
// The endpoint needs no credential on purpose. A balancer, an orchestrator
// and a person with curl all ask it the same two questions, "is it up" and
// "which one is it", and none of them holds a GitLab token to ask with. What
// it answers is therefore chosen to give away nothing a caller could use: no
// counters, no configuration, no GitLab round-trip.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// Health statuses.
const (
	healthStatusOK       = "ok"
	healthStatusDraining = "draining"
)

// announceDraining flips /health to draining and, when a delay is
// configured, holds the listener open that long before the caller closes it.
//
// Without the delay the close is what a balancer notices, one probe later,
// and every request it sent in that window failed. With it set to at least
// one probe interval the balancer sees the 503 first and stops sending work
// before the listener goes.
//
// The flag belongs to one listener rather than to the process, and is never
// cleared: a server that has started to drain does not come back to serving,
// but the next server built in the same process (a test, or a restart of the
// listener) starts out serving.
func announceDraining(ctx context.Context, draining *atomic.Bool, delay time.Duration) {
	draining.Store(true)
	if delay <= 0 {
		slog.InfoContext(ctx, "HTTP server shutdown requested")
		return
	}
	slog.InfoContext(ctx, "HTTP server shutdown requested, announcing draining before closing the listener",
		"drain_delay", delay)
	time.Sleep(delay)
}

// healthResponse is the /health body. StartedAt is the stable fact: it is
// byte-identical across probes, so a monitor can cache it and detect a
// restart by noticing it moved, the same reason Prometheus exposes
// process_start_time_seconds rather than an uptime counter. UptimeSeconds is
// the derived convenience value, in the unit the IETF health check draft uses
// for it ("observedUnit": "s").
type healthResponse struct {
	// Status is ok while the process is serving and draining once shutdown
	// was requested; the HTTP status carries the same verdict, 200 or 503.
	Status  string `json:"status"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
	// Build is the one string a display wants: the release this build is
	// closest to plus the short commit it was built from, "2.7.5+404e367".
	// A tag build and a build from main report comparable shapes, where
	// Version alone gives one a plain number and the other a Go
	// pseudo-version with a timestamp in it.
	Build string `json:"build"`
	// ConfigDigest fingerprints the settings that decide what a client
	// sees, so a monitor can tell whether the instances behind one balancer
	// are configured alike without any of them publishing its configuration.
	ConfigDigest string `json:"config_digest,omitempty"`
	// StartedAt is the process start instant in RFC 3339, matching how this
	// project renders timestamps everywhere else.
	StartedAt string `json:"started_at"`
	// UptimeSeconds is whole seconds since StartedAt. Sub-second precision
	// would be noise on an endpoint polled at probe intervals.
	UptimeSeconds int64 `json:"uptime_seconds"`
}

// healthHandler returns the /health handler for a listener whose
// configuration digest is digest and whose draining flag is draining. It
// requires no authentication.
func healthHandler(digest string, draining *atomic.Bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		body := newHealthResponse(processStartTime, time.Now(), digest, draining.Load())
		w.Header().Set(hdrContentType, mimeJSON)
		if body.Status == healthStatusDraining {
			// A balancer must not cache the last 200 across the flip.
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		// A client that went away mid-write is not something the handler can
		// act on.
		_ = json.NewEncoder(w).Encode(body)
	}
}

// newHealthResponse builds the /health body for a start instant observed at
// now. Both instants are parameters so the uptime arithmetic can be tested
// without mutating a package-level clock from concurrent tests.
func newHealthResponse(startedAt, now time.Time, digest string, isDraining bool) healthResponse {
	// Truncating instead of rounding keeps uptime from reporting a second that
	// has not fully elapsed. The clamp guards a caller that observes an instant
	// before the start; time.Now within one process cannot, because its
	// monotonic reading never goes backwards.
	uptime := int64(now.Sub(startedAt).Seconds())
	uptime = max(uptime, 0)
	status := healthStatusOK
	if isDraining {
		status = healthStatusDraining
	}
	return healthResponse{
		Status:        status,
		Version:       version,
		Commit:        commit,
		Build:         buildIdentifier(version, commit),
		ConfigDigest:  digest,
		StartedAt:     startedAt.UTC().Format(time.RFC3339),
		UptimeSeconds: uptime,
	}
}

// pseudoVersion matches what the Go toolchain records for a build from a
// working tree that is not a tag: the next patch version, a timestamp and a
// twelve-character commit, with +dirty when the tree had changes.
var pseudoVersion = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)-0\.\d{14}-([0-9a-f]{12})(\+dirty)?$`)

// buildIdentifier renders a build as one displayable string: the release it
// is closest to, plus the short commit it was built from, plus ".dirty" when
// the tree had uncommitted changes.
//
// A release binary is stamped with its version, so it reads "2.7.5+404e367".
// A build from main carries a pseudo-version such as
// "2.7.6-0.20260903061404-6e6ff5beb20e+dirty", correct provenance and
// unusable as a label: it encodes the patch that does not exist yet, so the
// release it is closest to is the one before, "2.7.5+6e6ff5b.dirty". With no
// commit recorded at all, the version is all there is.
func buildIdentifier(version, commit string) string {
	base := strings.TrimSuffix(version, "+dirty")
	dirty := strings.HasSuffix(version, "+dirty")
	short := shortCommit(commit)
	if m := pseudoVersion.FindStringSubmatch(version); m != nil {
		if patch, err := strconv.Atoi(m[3]); err == nil && patch > 0 {
			base = m[1] + "." + m[2] + "." + strconv.Itoa(patch-1)
		} else {
			base = m[1] + "." + m[2] + "." + m[3]
		}
		if short == "" {
			short = m[4][:7]
		}
	}
	out := base
	if short != "" {
		out += "+" + short
	}
	if dirty {
		out += ".dirty"
	}
	return out
}

// shortCommit is the seven-character form of a commit hash, or empty when
// none was recorded.
func shortCommit(commit string) string {
	commit = strings.TrimSpace(commit)
	if commit == "" || commit == "none" {
		return ""
	}
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

// configDigest fingerprints the settings that decide the tool list a client
// sees. Several instances behind one balancer must agree on every one of
// them, or one misconfigured node serves a different catalog to whichever
// clients reach it, and nothing else detects that.
//
// The digest is of the values, not a hash of anything secret: none of these
// is one, and none is recoverable from twelve hex characters. Order-free
// where the setting is a set.
func configDigest(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	excluded := slices.Clone(cfg.ExcludeTools)
	slices.Sort(excluded)
	fields := []string{
		"tool_surface=" + cfg.ToolSurface,
		"capability_surface=" + cfg.CapabilitySurface,
		"meta_param_schema=" + cfg.MetaParamSchema,
		"tier=" + cfg.Tier.String(),
		"read_only=" + strconv.FormatBool(cfg.ReadOnly),
		"safe_mode=" + strconv.FormatBool(cfg.SafeMode),
		"exclude_tools=" + strings.Join(excluded, ","),
	}
	sum := sha256.Sum256([]byte(strings.Join(fields, "\n")))
	return hex.EncodeToString(sum[:])[:12]
}
