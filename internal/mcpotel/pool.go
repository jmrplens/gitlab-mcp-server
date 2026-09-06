package mcpotel

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// PoolCounts is one reading of an HTTP deployment's credential pool, in the
// shape this package publishes it.
//
// It is declared here rather than imported from internal/serverpool on purpose.
// This package instruments MCP request handling and knows nothing about how a
// deployment holds credentials; importing the pool would invert a dependency
// that currently runs one way only. The caller owns both and adapts one to the
// other in a closure, which is also what keeps stdio, where there is no pool,
// from linking any of this.
type PoolCounts struct {
	// Entries is how many credentials the pool holds at this instant and
	// MaxSize is how many --max-http-clients allows. Both are published,
	// because the question an operator asks is "how close to the bound", and
	// answering it from a constant typed into a dashboard makes the answer
	// wrong the moment the flag moves.
	Entries int64
	MaxSize int64

	// The eviction counters, one per reason and disjoint by construction. Their
	// sum is every entry the pool has ever dropped except the ones taken at
	// shutdown, which are deliberately uncounted: nothing observes a metric
	// after the process ends.
	SizeEvictions     int64
	BusyEvictions     int64
	IdleEvictions     int64
	StaleEvictions    int64
	RejectedEvictions int64
	InvalidEvictions  int64
	RebuildEvictions  int64
}

// AttrPoolEvictionReason says why the pool dropped an entry.
//
// Under this server's own namespace rather than the convention's mcp.*, for the
// reason recorded on the attribute block in attributes.go: a credential pool is
// this deployment's concept and not the protocol's, so a name under mcp.* would
// claim part of a namespace the semantic convention owns and may later define
// differently. The instruments below are named the same way and for the same
// reason.
const AttrPoolEvictionReason = attribute.Key("gitlab_mcp.credential_pool.eviction.reason")

// The closed vocabulary AttrPoolEvictionReason takes, one value per removal
// path the pool has. size_pressure and size_pressure_busy are the two halves of
// one path, split because only the second ends work somebody was waiting on.
//
//nolint:gosec // G101 fires on the identifiers containing "Credential"; these are metric label values naming why an entry was dropped, and no credential is anywhere near them.
const (
	poolEvictionSizePressure       = "size_pressure"
	poolEvictionSizePressureBusy   = "size_pressure_busy"
	poolEvictionIdle               = "idle"
	poolEvictionStaleCredential    = "stale_credential"
	poolEvictionRejectedCredential = "rejected_credential"
	poolEvictionInvalidCredential  = "invalid_credential"
	poolEvictionRebuild            = "rebuild"
)

// The instrument names, written out once so the callback and its documentation
// cannot drift apart.
const (
	poolEntriesInstrument   = "gitlab_mcp.credential_pool.entries"
	poolCapacityInstrument  = "gitlab_mcp.credential_pool.capacity"
	poolEvictionsInstrument = "gitlab_mcp.credential_pool.evictions"
)

// ErrNoPoolReader is returned when ObservePool is given no way to read the
// pool. A registered callback that cannot read anything would export a
// permanent zero, which reads as a healthy empty pool rather than as a wiring
// mistake, so this refuses instead.
var ErrNoPoolReader = errors.New("mcpotel: ObservePool needs a function that reads the pool")

// ObservePool publishes the credential pool's occupancy and its evictions,
// reading them through read on every collection.
//
// Asynchronous instruments rather than counters incremented at the eviction
// site, because the pool already keeps these numbers and the increments happen
// under its write lock: a synchronous instrument there would put an exporter's
// code on a path that must not block. read is called on the SDK's collection
// goroutine, so it must be cheap and must not block either; a pool snapshot is
// a read lock and a handful of atomic loads.
//
// Register it unconditionally. With telemetry off the global meter is a no-op
// whose callback is never invoked, so this costs one registration at startup
// and nothing afterwards.
//
// The returned [metric.Registration] must be unregistered when the pool it
// reads is closed, or the callback keeps a dead pool reachable.
func ObservePool(read func() PoolCounts) (metric.Registration, error) {
	return observePool(otel.Meter(scopeName), read)
}

// observePool is [ObservePool] with the meter passed in, so a test can drive a
// real SDK reader and a provider that refuses an instrument.
func observePool(meter metric.Meter, read func() PoolCounts) (metric.Registration, error) {
	if read == nil {
		return nil, ErrNoPoolReader
	}

	entries, err := meter.Int64ObservableUpDownCounter(
		poolEntriesInstrument,
		metric.WithUnit("{entry}"),
		metric.WithDescription("Credentials the pool currently holds."),
	)
	if err != nil {
		return nil, err
	}
	capacity, err := meter.Int64ObservableUpDownCounter(
		poolCapacityInstrument,
		metric.WithUnit("{entry}"),
		metric.WithDescription("Credentials the pool may hold, which is --max-http-clients."),
	)
	if err != nil {
		return nil, err
	}
	const evictionsDescription = "Entries the pool has dropped, by reason. Entries dropped at shutdown are not " +
		"counted, since nothing observes a metric after the process ends."
	evictions, err := meter.Int64ObservableCounter(
		poolEvictionsInstrument,
		metric.WithUnit("{eviction}"),
		metric.WithDescription(evictionsDescription),
	)
	if err != nil {
		return nil, err
	}

	// Built once: the reason attribute sets are constant, and rebuilding them
	// on every collection would allocate for nothing.
	reasons := []struct {
		value  string
		amount func(PoolCounts) int64
	}{
		{poolEvictionSizePressure, func(c PoolCounts) int64 { return c.SizeEvictions }},
		{poolEvictionSizePressureBusy, func(c PoolCounts) int64 { return c.BusyEvictions }},
		{poolEvictionIdle, func(c PoolCounts) int64 { return c.IdleEvictions }},
		{poolEvictionStaleCredential, func(c PoolCounts) int64 { return c.StaleEvictions }},
		{poolEvictionRejectedCredential, func(c PoolCounts) int64 { return c.RejectedEvictions }},
		{poolEvictionInvalidCredential, func(c PoolCounts) int64 { return c.InvalidEvictions }},
		{poolEvictionRebuild, func(c PoolCounts) int64 { return c.RebuildEvictions }},
	}
	sets := make([]metric.MeasurementOption, len(reasons))
	for i, reason := range reasons {
		sets[i] = metric.WithAttributes(AttrPoolEvictionReason.String(reason.value))
	}

	return meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		counts := read()
		observer.ObserveInt64(entries, counts.Entries)
		observer.ObserveInt64(capacity, counts.MaxSize)
		// Every reason on every collection, zeros included. A counter that
		// appears only after its first eviction cannot be alerted on before it
		// matters, and the vocabulary is closed at seven values, so the whole
		// series set costs less than one method dimension of the request
		// instrument.
		for i, reason := range reasons {
			observer.ObserveInt64(evictions, reason.amount(counts), sets[i])
		}
		return nil
	}, entries, capacity, evictions)
}
