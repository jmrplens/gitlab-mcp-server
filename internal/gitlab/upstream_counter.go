// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package gitlab

import (
	"errors"
	"maps"
	"net/http"
	"sync"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/mcpotel"
)

// upstreamCounter counts the requests this process sends upstream, by the
// server address the metric may name.
//
// It observes and never limits. GitLab keeps its own budgets on what it
// receives: per user, which every credential of that user spends, and on
// GitLab.com per IP address, which every tenant of a deployment spends through
// the one egress address. No allowance here is keyed on either, so what the
// process sends against them was counted by nothing until this (finding F-18
// of the tenant policy register, issue 955): the duration histogram sees the
// SDK's calls only, and only with telemetry on, while the OAuth verifier and
// the health probes spend the same budgets. The count is the process's
// (INV-018) and carries no credential, user or path, so it bounds nothing and
// no register row decides it.
//
// The key is [mcpotel.BoundedServerAddress] rather than the host. In the
// free-selection mode a caller names the instance, and a key per host would be
// a table that caller grows by one entry for every name it invents.
type upstreamCounter struct {
	mu     sync.Mutex
	counts map[string]int64
}

// newUpstreamCounter returns a counter that has counted nothing.
func newUpstreamCounter() *upstreamCounter {
	return &upstreamCounter{counts: map[string]int64{}}
}

// add counts one request sent to host.
func (c *upstreamCounter) add(host string) {
	address := mcpotel.BoundedServerAddress(host)
	c.mu.Lock()
	c.counts[address]++
	c.mu.Unlock()
}

// reading returns a copy of the counts, so the caller cannot rewrite them.
func (c *upstreamCounter) reading() map[string]int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return maps.Clone(c.counts)
}

// processUpstream is the one counter every transport of this package counts
// into: the API client, the health probes and the transport [HTTPTransport]
// lends. One per process because the budget it measures is the deployment's,
// whichever client spent it.
var processUpstream = newUpstreamCounter()

// UpstreamRequests returns how many requests this process has sent upstream
// since it started, by the server address the metric may name: a host
// [mcpotel.SetMetricServerAddresses] declared, or [mcpotel.OtherServerAddress]
// for any other, a redirect hop off the instance included.
//
// Every attempt is counted, so a request client-go re-sends is counted once
// per attempt, and a redirect hop is counted as the request of its own that
// net/http makes it. A request is counted once the transport that dials
// returns, answered or not: one that reached no instance because the
// connection failed is counted, since nothing on this side can tell it from
// one GitLab received and failed to answer. One the destination guard refused
// is not, because the guard refuses before a connection exists and the
// request never left the process. It counts with telemetry off as with it on;
// telemetry only publishes it ([mcpotel.ObserveUpstreamRequests]).
func UpstreamRequests() map[string]int64 {
	return processUpstream.reading()
}

// sendUpstream hands req to base, the transport that dials, and counts it as
// one upstream request unless the destination guard refused it.
//
// It wraps the dialing transport and nothing above it, so every layer that
// re-sends (client-go's retries, net/http's redirects) passes through it once
// per request it sends, and a layer that refuses unsent returns before it.
func sendUpstream(base http.RoundTripper, req *http.Request) (*http.Response, error) {
	resp, err := base.RoundTrip(req)
	if !errors.Is(err, ErrDestinationRefused) {
		processUpstream.add(req.URL.Hostname())
	}
	return resp, err
}

// upstreamCountedTransport is the transport [HTTPTransport] lends: the base
// transport, with every request it sends counted.
type upstreamCountedTransport struct {
	base http.RoundTripper
}

// RoundTrip sends req through the base transport and counts it.
func (t upstreamCountedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return sendUpstream(t.base, req)
}
