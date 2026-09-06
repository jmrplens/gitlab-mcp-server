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
// [mcpotel.PoolCounts] lives with the caller that owns both. A field added to
// the snapshot and not copied here is a field that is not exported, which is
// the failure this shape makes visible rather than silent.
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
		stats := pool.Stats()
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
	})
	if err != nil {
		slog.Warn("credential pool telemetry could not be registered; its instruments will be absent", "error", err)
		return func() {}
	}
	return func() {
		if unregisterErr := registration.Unregister(); unregisterErr != nil {
			slog.Warn("credential pool telemetry could not be unregistered", "error", unregisterErr)
		}
	}
}
