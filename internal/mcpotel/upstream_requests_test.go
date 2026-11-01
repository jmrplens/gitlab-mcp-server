// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package mcpotel

import (
	"errors"
	"maps"
	"testing"

	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// observingUpstreamRequests registers the callback over reading against a real
// SDK reader, with declared as the hosts the metric may name, and returns what
// one collection published, by server address.
func observingUpstreamRequests(t *testing.T, declared []string, reading map[string]int64) (metricdata.Metrics, map[string]int64) {
	t.Helper()

	previous := metricServerAddresses.Load()
	t.Cleanup(func() { metricServerAddresses.Store(previous) })
	SetMetricServerAddresses(declared)

	reader, restore := newMetricRecorder(t)
	t.Cleanup(restore)

	registration, err := ObserveUpstreamRequests(func() map[string]int64 { return reading })
	if err != nil {
		t.Fatalf("ObserveUpstreamRequests() error: %v", err)
	}
	t.Cleanup(func() { _ = registration.Unregister() })

	m := collectedMetric(t, reader, upstreamRequestsInstrument)
	published := map[string]int64{}
	for _, p := range sumPoints(t, m) {
		if p.Attributes.Len() != 1 {
			t.Errorf("a data point carries %d attributes (%v); the server address is the only dimension this may have",
				p.Attributes.Len(), p.Attributes.ToSlice())
		}
		address, ok := p.Attributes.Value(attrServerAddress)
		if !ok {
			t.Errorf("a data point carries no %s", attrServerAddress)
			continue
		}
		if _, twice := published[address.AsString()]; twice {
			t.Errorf("%q was published twice in one collection", address.AsString())
		}
		published[address.AsString()] = p.Value
	}
	return m, published
}

// TestObserveUpstreamRequests_PublishesTheReadingByServerAddress covers the
// shape a collector receives: a monotonic count of requests, one series per
// server address, carrying the address alone.
//
// A credential, a user or a path beside it would split one budget GitLab keeps
// for the deployment's address into series nobody keeps, and would put on a
// metric the identifiers this server declines to record anywhere else.
func TestObserveUpstreamRequests_PublishesTheReadingByServerAddress(t *testing.T) {
	m, published := observingUpstreamRequests(t, []string{"gitlab.example.com"},
		map[string]int64{"gitlab.example.com": 12, OtherServerAddress: 5})

	if m.Unit != "{request}" {
		t.Errorf("unit = %q, want {request}", m.Unit)
	}
	if m.Description == "" {
		t.Error("the instrument carries no description, so a collector shows a bare name")
	}
	if sum, ok := m.Data.(metricdata.Sum[int64]); ok && !sum.IsMonotonic {
		t.Error("the count was published as non-monotonic; requests sent only add up")
	}
	if want := map[string]int64{"gitlab.example.com": 12, OtherServerAddress: 5}; !maps.Equal(published, want) {
		t.Errorf("published %v, want %v", published, want)
	}
}

// TestObserveUpstreamRequests_BoundsWhatTheReaderNames covers the label a
// reader must not choose, and the sum that keeps it from being lost.
//
// The export bounds every address itself rather than trusting the reader to
// have, so a reader keyed on whatever host a caller named cannot mint a series
// per name. Two undeclared hosts land in the one other bucket, and their
// counts are added there rather than observed twice under one attribute set,
// which the specification asks a callback not to do.
func TestObserveUpstreamRequests_BoundsWhatTheReaderNames(t *testing.T) {
	_, published := observingUpstreamRequests(t, []string{"gitlab.example.com"},
		map[string]int64{"gitlab.example.com": 2, "one.invalid": 3, "two.invalid": 4})

	if want := map[string]int64{"gitlab.example.com": 2, OtherServerAddress: 7}; !maps.Equal(published, want) {
		t.Errorf("published %v, want %v", published, want)
	}
}

// TestObserveUpstreamRequests_ADeclaredHost_IsPublishedFromTheStart covers a
// host nothing has been sent to yet.
//
// It is published at zero rather than left out, as every other counter of
// this server is, so a panel is not empty for the ambiguous reason and a rate
// taken over the first request has a point to start from. With nothing
// declared there is no host to start, and nothing is published until a
// request is counted.
func TestObserveUpstreamRequests_ADeclaredHost_IsPublishedFromTheStart(t *testing.T) {
	tests := []struct {
		name     string
		declared []string
		want     map[string]int64
	}{
		{name: "a declared host starts at zero", declared: []string{"gitlab.example.com"}, want: map[string]int64{"gitlab.example.com": 0}},
		{name: "with nothing declared nothing is published", want: map[string]int64{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, restore := newMetricRecorder(t)
			t.Cleanup(restore)
			previous := metricServerAddresses.Load()
			t.Cleanup(func() { metricServerAddresses.Store(previous) })
			if tt.declared != nil {
				SetMetricServerAddresses(tt.declared)
			} else {
				metricServerAddresses.Store(nil)
			}

			registration, err := ObserveUpstreamRequests(func() map[string]int64 { return nil })
			if err != nil {
				t.Fatalf("ObserveUpstreamRequests() error: %v", err)
			}
			t.Cleanup(func() { _ = registration.Unregister() })

			if published := publishedUpstreamRequests(t, reader); !maps.Equal(published, tt.want) {
				t.Errorf("published %v before any request, want %v", published, tt.want)
			}
		})
	}
}

// publishedUpstreamRequests is what one collection published of the upstream
// count, by server address, and nothing when the instrument published no
// point at all.
func publishedUpstreamRequests(t *testing.T, reader *sdkmetric.ManualReader) map[string]int64 {
	t.Helper()

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &collected); err != nil {
		t.Fatalf("collecting metrics: %v", err)
	}
	published := map[string]int64{}
	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != upstreamRequestsInstrument {
				continue
			}
			for _, p := range sumPoints(t, m) {
				address, _ := p.Attributes.Value(attrServerAddress)
				published[address.AsString()] = p.Value
			}
		}
	}
	return published
}

// TestObserveUpstreamRequests_WithoutAReadFunction_IsRefused pins the refusal
// rather than a permanent zero, which would read as a deployment that never
// sent GitLab anything instead of as a wiring mistake.
func TestObserveUpstreamRequests_WithoutAReadFunction_IsRefused(t *testing.T) {
	t.Parallel()
	registration, err := observeUpstreamRequests(noop.NewMeterProvider().Meter("test"), nil)
	if !errors.Is(err, ErrNoUpstreamRequestReader) {
		t.Errorf("observeUpstreamRequests(nil) error = %v, want ErrNoUpstreamRequestReader", err)
	}
	if registration != nil {
		t.Error("a refused registration returned something to unregister")
	}
}

// TestObserveUpstreamRequests_ARefusedRegistration_IsReturnedNotSwallowed
// covers the provider that will not create the instrument or take the
// callback: the caller has to learn that the series is missing, since a blank
// panel otherwise reads as a collector problem at the other end.
func TestObserveUpstreamRequests_ARefusedRegistration_IsReturnedNotSwallowed(t *testing.T) {
	refused := errors.New("the provider refused this registration")

	for _, refuse := range []string{upstreamRequestsInstrument, "callback"} {
		t.Run(refuse, func(t *testing.T) {
			meter := refusingPoolMeter{
				Meter:  noop.NewMeterProvider().Meter("test"),
				refuse: refuse,
				err:    refused,
			}

			registration, err := observeUpstreamRequests(meter, func() map[string]int64 { return nil })
			if !errors.Is(err, refused) {
				t.Errorf("observeUpstreamRequests() error = %v, want the provider's refusal", err)
			}
			if registration != nil {
				t.Error("observeUpstreamRequests() returned a registration despite the refusal")
			}
		})
	}
}
