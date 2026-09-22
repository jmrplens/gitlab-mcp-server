package mcpotel

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// AuthBlockCounts is one reading of how many requests an HTTP deployment has
// refused before reading their credential, by the budget that refused them.
//
// Like [PoolCounts], it is declared here rather than imported from the guards
// that keep these numbers: this package instruments MCP request handling and
// knows nothing about how a deployment budgets authentication, and the caller
// adapts one to the other in a closure. That is also what keeps stdio, which
// has no such budgets, from linking any of it.
//
// What it deliberately does not carry is who was refused. An address is the
// one fact that would make this series identify people, a counter keyed by it
// grows without bound under exactly the traffic it exists to measure, and the
// question an operator asks is how much is being refused and by which rule.
type AuthBlockCounts struct {
	// One counter per budget, disjoint by construction because the guard
	// consults them in order and answers with the first that refuses.
	FailureLockout  int64
	TransportSource int64
	DistinctTokens  int64
}

// AttrAuthBlockReason says which budget refused the request.
//
// Under this server's own namespace rather than the convention's mcp.*, for the
// reason recorded on the attribute block in attributes.go: an authentication
// budget is this deployment's concept and not the protocol's.
const AttrAuthBlockReason = attribute.Key("gitlab_mcp.auth.block.reason")

// The closed vocabulary AttrAuthBlockReason takes, one value per budget.
//
// They are exported because the guards that answer with them live in package
// main, and a value spelled twice is a value that can be spelled differently.
const (
	// AuthBlockFailureLockout is the fast per-address budget: a number of
	// failed authentications inside a short window.
	AuthBlockFailureLockout = "failure_lockout"
	// AuthBlockTransportSource is the coarse budget charged to the address the
	// connection came from, rather than to whatever a trusted proxy header
	// claims.
	AuthBlockTransportSource = "transport_source"
	// AuthBlockDistinctTokens is the slow budget: distinct credentials refused
	// for one address over a longer window, answered with a block that
	// lengthens each time it is reached.
	AuthBlockDistinctTokens = "distinct_tokens"
)

// authBlocksInstrument is the instrument name, written out once so the callback
// and its documentation cannot drift apart.
const authBlocksInstrument = "gitlab_mcp.auth.blocks"

// ErrNoAuthBlockReader is returned when ObserveAuthBlocks is given no way to
// read the counters. A registered callback that cannot read anything would
// export a permanent zero, which reads as a deployment refusing nobody rather
// than as a wiring mistake, so this refuses instead.
var ErrNoAuthBlockReader = errors.New("mcpotel: ObserveAuthBlocks needs a function that reads the budgets")

// ObserveAuthBlocks publishes how many requests each authentication budget has
// refused, reading them through read on every collection.
//
// Asynchronous for the same reason [ObservePool] is: the guards already keep
// these numbers, and the increments happen on the request path, where an
// exporter's code does not belong. read is called on the SDK's collection
// goroutine, so it must be cheap and must not block; three atomic loads are.
//
// Register it unconditionally. With telemetry off the global meter is a no-op
// whose callback is never invoked, so this costs one registration at startup
// and nothing afterwards.
func ObserveAuthBlocks(read func() AuthBlockCounts) (metric.Registration, error) {
	return observeAuthBlocks(otel.Meter(scopeName), read)
}

// observeAuthBlocks is [ObserveAuthBlocks] with the meter passed in, so a test
// can drive a real SDK reader and a provider that refuses an instrument.
func observeAuthBlocks(meter metric.Meter, read func() AuthBlockCounts) (metric.Registration, error) {
	if read == nil {
		return nil, ErrNoAuthBlockReader
	}

	const description = "Requests refused before their credential was read, by the budget that refused them. " +
		"The address refused is deliberately not a dimension."
	blocks, err := meter.Int64ObservableCounter(
		authBlocksInstrument,
		metric.WithUnit("{request}"),
		metric.WithDescription(description),
	)
	if err != nil {
		return nil, err
	}

	// Built once: the reason attribute sets are constant, and rebuilding them
	// on every collection would allocate for nothing.
	reasons := []struct {
		value  string
		amount func(AuthBlockCounts) int64
	}{
		{AuthBlockFailureLockout, func(c AuthBlockCounts) int64 { return c.FailureLockout }},
		{AuthBlockTransportSource, func(c AuthBlockCounts) int64 { return c.TransportSource }},
		{AuthBlockDistinctTokens, func(c AuthBlockCounts) int64 { return c.DistinctTokens }},
	}
	sets := make([]metric.MeasurementOption, len(reasons))
	for i, r := range reasons {
		sets[i] = metric.WithAttributes(AttrAuthBlockReason.String(r.value))
	}

	return meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		counts := read()
		for i, r := range reasons {
			o.ObserveInt64(blocks, r.amount(counts), sets[i])
		}
		return nil
	}, blocks)
}
