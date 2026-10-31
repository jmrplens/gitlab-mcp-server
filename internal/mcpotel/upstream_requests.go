// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package mcpotel

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// upstreamRequestsInstrument is the instrument name, written out once so the
// callback and its documentation cannot drift apart.
//
// Under this server's own namespace, for the reason the credential pool's
// instruments are: the http.client convention describes one request's
// duration and has no count of the requests a process sent, so a name under
// http.* would claim a metric the convention may later define differently.
const upstreamRequestsInstrument = "gitlab_mcp.upstream.requests"

// ErrNoUpstreamRequestReader is returned when ObserveUpstreamRequests is given
// no way to read the count. A registered callback that cannot read anything
// would export nothing, which reads as a deployment that never sent GitLab a
// request rather than as a wiring mistake, so this refuses instead.
var ErrNoUpstreamRequestReader = errors.New("mcpotel: ObserveUpstreamRequests needs a function that reads the counts")

// ObserveUpstreamRequests publishes how many requests the process has sent to
// GitLab, by server address, reading them through read on every collection.
//
// The count is kept by the transports in internal/gitlab, which this package
// knows nothing about, so the caller hands over the function that reads it,
// as it does for [ObservePool]. It is the process's count, and one budget is
// what it measures: GitLab.com keeps a per-IP limit on every request a
// deployment sends from its egress address, whichever tenant sent it, and
// http.client.request.duration sees only the SDK's calls, and only with
// telemetry on.
//
// read returns counts keyed by server address; an address that is not one
// [SetMetricServerAddresses] declared is published as [OtherServerAddress],
// whatever the reader named, so a reader cannot mint a series. It is called on
// the SDK's collection goroutine, so it must be cheap and must not block.
//
// Register it unconditionally. With telemetry off the global meter is a no-op
// whose callback is never invoked, so this costs one registration at startup
// and nothing afterwards.
func ObserveUpstreamRequests(read func() map[string]int64) (metric.Registration, error) {
	return observeUpstreamRequests(otel.Meter(scopeName), read)
}

// observeUpstreamRequests is [ObserveUpstreamRequests] with the meter passed
// in, so a test can drive a provider that refuses an instrument.
func observeUpstreamRequests(meter metric.Meter, read func() map[string]int64) (metric.Registration, error) {
	if read == nil {
		return nil, ErrNoUpstreamRequestReader
	}

	const description = "Requests this process sent to GitLab, retries and redirect hops included, " +
		"from the API client, the OAuth verifier and the health probes. " +
		"The server address is the only dimension: no credential, user or path."
	requests, err := meter.Int64ObservableCounter(
		upstreamRequestsInstrument,
		metric.WithUnit("{request}"),
		metric.WithDescription(description),
	)
	if err != nil {
		return nil, err
	}

	return meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		for address, count := range upstreamTotals(read()) {
			o.ObserveInt64(requests, count, metric.WithAttributes(attrServerAddress.String(address)))
		}
		return nil
	}, requests)
}

// upstreamTotals is a reading as the metric publishes it.
//
// Every address is bounded here, and the counts of addresses that bound to the
// same one are added: the specification asks a callback not to observe one
// attribute set twice in a collection and leaves to each SDK what it makes of
// a second observation, so the sum is taken where it does not depend on the
// SDK. Every declared
// host is present, at zero when nothing has been sent to it, so its series
// exists from startup, as every other counter of this server's does.
func upstreamTotals(reading map[string]int64) map[string]int64 {
	totals := map[string]int64{}
	if declared := metricServerAddresses.Load(); declared != nil {
		for host := range *declared {
			totals[host] = 0
		}
	}
	for address, count := range reading {
		totals[BoundedServerAddress(address)] += count
	}
	return totals
}
