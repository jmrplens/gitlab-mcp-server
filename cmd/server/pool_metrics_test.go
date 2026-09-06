package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	apimetric "go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/mcpotel"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/serverpool"
)

// TestObservePoolMetrics_ExportsWhatTheSnapshotHolds pins the adaptation that
// only exists here.
//
// The pool knows nothing about telemetry and internal/mcpotel knows nothing
// about pools, so nothing but this function copies one into the other, and a
// counter added to the snapshot and forgotten here is a counter that is
// published as a permanent zero. The eviction driven below is the one that
// matters most: with a pool of one, size pressure has nothing quiet to take.
func TestObservePoolMetrics_ExportsWhatTheSnapshotHolds(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() {
		otel.SetMeterProvider(previous)
		_ = provider.Shutdown(context.Background())
	})

	gitlab := gateStubGitLab(t, false)
	pool := serverpool.New(&config.Config{
		GitLabURL:    gitlab,
		Tier:         edition.Free,
		TierExplicit: true,
		IgnoreScopes: true,
	}, okFactory, serverpool.WithMaxSize(1),
		serverpool.WithInUse(func(*serverpool.Entry) bool { return true }))
	t.Cleanup(pool.Close)

	stop := observePoolMetrics(pool)
	t.Cleanup(stop)

	// Two credentials into a pool of one, every entry reported busy: the
	// arriving one is admitted and the incumbent goes, which is the only path
	// that ends a subscription somebody is waiting on.
	for _, token := range []string{"glpat-the-subscriber", "glpat-the-newcomer"} {
		if _, err := pool.GetOrCreate(token, gitlab); err != nil {
			t.Fatalf("GetOrCreate(%s) error: %v", token, err)
		}
	}

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &collected); err != nil {
		t.Fatalf("collecting metrics: %v", err)
	}

	entries, capacity, busy := int64(-1), int64(-1), int64(-1)
	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}
			for _, point := range sum.DataPoints {
				reason, hasReason := point.Attributes.Value(mcpotel.AttrPoolEvictionReason)
				switch {
				case m.Name == "gitlab_mcp.credential_pool.entries":
					entries = point.Value
				case m.Name == "gitlab_mcp.credential_pool.capacity":
					capacity = point.Value
				case hasReason && reason.AsString() == "size_pressure_busy":
					busy = point.Value
				}
			}
		}
	}

	if entries != 1 {
		t.Errorf("credential_pool.entries = %d, want 1", entries)
	}
	if capacity != 1 {
		t.Errorf("credential_pool.capacity = %d, want the pool maximum of 1", capacity)
	}
	if busy != 1 {
		t.Errorf("the size_pressure_busy series = %d, want 1 after a busy eviction", busy)
	}
}

// refusingMeterProvider hands out a meter that will not create an instrument,
// standing in for a provider that rejects one.
type refusingMeterProvider struct {
	apimetric.MeterProvider
	err error
}

func (p refusingMeterProvider) Meter(string, ...apimetric.MeterOption) apimetric.Meter {
	return refusingMeter{Meter: noopmetric.Meter{}, err: p.err}
}

type refusingMeter struct {
	apimetric.Meter
	err error
}

func (m refusingMeter) Int64ObservableUpDownCounter(
	string, ...apimetric.Int64ObservableUpDownCounterOption,
) (apimetric.Int64ObservableUpDownCounter, error) {
	return nil, m.err
}

// TestObservePoolMetrics_ARefusedRegistration_LeavesTheServerServing covers the
// failure this function deliberately does not propagate.
//
// An instrument the provider refused costs its own measurements. Refusing to
// serve HTTP over it would be a far larger failure than the one being reported,
// so it is logged and the returned stop is a working no-op rather than a nil
// nobody may call.
func TestObservePoolMetrics_ARefusedRegistration_LeavesTheServerServing(t *testing.T) {
	refused := errors.New("the provider refused this instrument")
	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(refusingMeterProvider{MeterProvider: noopmetric.NewMeterProvider(), err: refused})
	t.Cleanup(func() { otel.SetMeterProvider(previous) })

	var logged bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	pool := serverpool.New(&config.Config{
		GitLabURL:    "https://gitlab.example.com",
		Tier:         edition.Free,
		TierExplicit: true,
		IgnoreScopes: true,
	}, okFactory)
	t.Cleanup(pool.Close)

	stop := observePoolMetrics(pool)
	if stop == nil {
		t.Fatal("observePoolMetrics returned no stop function, so the caller's defer would panic")
	}
	stop()

	if !strings.Contains(logged.String(), "credential pool telemetry could not be registered") {
		t.Errorf("the refusal was swallowed; an operator would see a missing instrument and nothing saying why.\nlogged: %s",
			logged.String())
	}
}
