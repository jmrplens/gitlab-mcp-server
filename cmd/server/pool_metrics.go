package main

import (
	"log/slog"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/mcpotel"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/serverpool"
)

// observePoolMetrics publishes the credential pool's occupancy and its eviction
// counters over OpenTelemetry, and returns the function that stops doing so.
//
// This is where the two packages meet, and deliberately the only place: the
// pool knows nothing about telemetry and internal/mcpotel knows nothing about
// pools, so the adaptation between [serverpool.Snapshot] and
// [mcpotel.PoolCounts] lives with the caller that owns both. It is [poolCounts]
// rather than a closure so that the copying can be asserted field by field: a
// counter added to the snapshot and forgotten there, or written into the wrong
// series, publishes a permanent zero or a wrong number that no other test can
// see.
//
// Registered whatever --telemetry says. With telemetry off the global meter is
// a no-op whose callback is never invoked, so the alternative would be a branch
// that can disagree with whether telemetry is actually running.
//
// A registration that fails is logged and nothing else: an instrument the
// provider refused costs its own measurements, and refusing to serve HTTP over
// it would be a much larger failure than the one being reported.
func observePoolMetrics(pool *serverpool.ServerPool) func() {
	registration, err := mcpotel.ObservePool(func() mcpotel.PoolCounts {
		return poolCounts(pool.Stats())
	})
	if err != nil {
		slog.Warn("credential pool telemetry could not be registered; its instruments will be absent", "error", err)
		return func() {
			// Nothing was registered, so there is nothing to unregister. The
			// caller still defers this, which is why it is a function rather
			// than a nil the caller would have to guard.
		}
	}
	return func() {
		if unregisterErr := registration.Unregister(); unregisterErr != nil {
			slog.Warn("credential pool telemetry could not be unregistered", "error", unregisterErr)
		}
	}
}

// poolCounts is the whole of the translation between the pool's snapshot and
// the numbers the instruments publish.
//
// Every eviction counter is carried across separately rather than summed,
// because each is published as its own series of the eviction reason attribute
// and the series are only meaningful while they stay disjoint. The snapshot's
// legacy Evictions total is deliberately absent: it overlaps four of the others,
// so exporting it beside them would double-count every size, revalidation and
// rebuild eviction.
func poolCounts(stats serverpool.Snapshot) mcpotel.PoolCounts {
	return mcpotel.PoolCounts{
		Entries:           int64(stats.CurrentSize),
		MaxSize:           int64(stats.MaxSize),
		SizeEvictions:     stats.SizeEvictions,
		BusyEvictions:     stats.BusyEvictions,
		IdleEvictions:     stats.IdleEvictions,
		StaleEvictions:    stats.StaleCredentialEvictions,
		RejectedEvictions: stats.RejectedCredentialEvictions,
		InvalidEvictions:  stats.InvalidEvictions,
		RebuildEvictions:  stats.RebuildEvictions,
	}
}
