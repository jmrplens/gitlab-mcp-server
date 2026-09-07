package mcpotel

import (
	"context"
	"errors"
	"testing"

	apimetric "go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// samplePoolCounts is a reading in which every field differs, so a callback
// that observes the wrong counter under a reason produces a wrong number rather
// than a coincidence.
var samplePoolCounts = PoolCounts{
	Entries:           7,
	MaxSize:           100,
	SizeEvictions:     11,
	BusyEvictions:     3,
	IdleEvictions:     5,
	StaleEvictions:    13,
	RejectedEvictions: 17,
	InvalidEvictions:  19,
	RebuildEvictions:  23,
}

// collectedMetric returns one named metric from a fresh collection, or fails.
func collectedMetric(t *testing.T, reader *metric.ManualReader, name string) metricdata.Metrics {
	t.Helper()

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("collecting metrics: %v", err)
	}
	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == name {
				return m
			}
		}
	}
	t.Fatalf("no metric named %q was collected", name)
	return metricdata.Metrics{}
}

// sumPoints returns an int64 sum's data points, or fails when the instrument
// aggregated to something else.
func sumPoints(t *testing.T, m metricdata.Metrics) []metricdata.DataPoint[int64] {
	t.Helper()

	sum, ok := m.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("%s aggregated to %T, want an int64 sum", m.Name, m.Data)
	}
	return sum.DataPoints
}

// observingSamplePool registers the callback over samplePoolCounts and returns
// the reader a collection can be taken from.
//
// A real SDK reader rather than a fake observer: the shape a collector receives
// is decided inside the SDK, and an asynchronous instrument's aggregation,
// monotonicity and attribute-set handling are exactly the parts a fake would
// accept without proving anything.
func observingSamplePool(t *testing.T) *metric.ManualReader {
	t.Helper()

	reader, restore := newMetricRecorder(t)
	t.Cleanup(restore)

	registration, err := ObservePool(func() PoolCounts { return samplePoolCounts })
	if err != nil {
		t.Fatalf("ObservePool() error: %v", err)
	}
	t.Cleanup(func() { _ = registration.Unregister() })
	return reader
}

// TestObservePool_PublishesOccupancyAgainstItsCapacity covers the pair that
// answers "how close to the bound".
//
// Capacity is published beside occupancy rather than left to the operator,
// because deriving it from --max-http-clients typed into a dashboard query
// makes the answer wrong the moment the flag moves and says nothing when it
// does.
func TestObservePool_PublishesOccupancyAgainstItsCapacity(t *testing.T) {
	reader := observingSamplePool(t)

	tests := []struct {
		name string
		want int64
	}{
		{name: poolEntriesInstrument, want: samplePoolCounts.Entries},
		{name: poolCapacityInstrument, want: samplePoolCounts.MaxSize},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := collectedMetric(t, reader, tt.name)
			if m.Unit != "{entry}" {
				t.Errorf("unit = %q, want {entry}", m.Unit)
			}
			if m.Description == "" {
				t.Error("the instrument carries no description, so a collector shows a bare name")
			}
			if sum, _ := m.Data.(metricdata.Sum[int64]); sum.IsMonotonic {
				t.Error("occupancy was published as monotonic; it falls as well as rises")
			}
			points := sumPoints(t, m)
			if len(points) != 1 {
				t.Fatalf("got %d data points, want exactly one: occupancy carries no dimensions", len(points))
			}
			if points[0].Value != tt.want {
				t.Errorf("value = %d, want %d", points[0].Value, tt.want)
			}
		})
	}
}

// TestObservePool_PublishesOneSeriesPerEvictionReason pins the split that makes
// the counter worth reading.
//
// One conflated total cannot tell "the pool is too small" from "somebody's
// token was revoked", and neither from the one path that ends a subscription
// under a client that is waiting on it. Each reason is checked against a
// distinct number, so a callback that observes the wrong counter under a reason
// produces a wrong value rather than a coincidence.
func TestObservePool_PublishesOneSeriesPerEvictionReason(t *testing.T) {
	reader := observingSamplePool(t)

	m := collectedMetric(t, reader, poolEvictionsInstrument)
	if m.Unit != "{eviction}" {
		t.Errorf("unit = %q, want {eviction}", m.Unit)
	}
	sum, isSum := m.Data.(metricdata.Sum[int64])
	if !isSum {
		t.Fatalf("%s aggregated to %T, want an int64 sum", m.Name, m.Data)
	}
	if !sum.IsMonotonic {
		t.Error("the eviction counter was published as non-monotonic; it only ever rises")
	}

	want := map[string]int64{
		poolEvictionSizePressure:       samplePoolCounts.SizeEvictions,
		poolEvictionSizePressureBusy:   samplePoolCounts.BusyEvictions,
		poolEvictionIdle:               samplePoolCounts.IdleEvictions,
		poolEvictionStaleCredential:    samplePoolCounts.StaleEvictions,
		poolEvictionRejectedCredential: samplePoolCounts.RejectedEvictions,
		poolEvictionInvalidCredential:  samplePoolCounts.InvalidEvictions,
		poolEvictionRebuild:            samplePoolCounts.RebuildEvictions,
	}
	got := reasonValues(t, sum)
	if len(got) != len(want) {
		t.Errorf("got %d reason series, want %d: the vocabulary is closed", len(got), len(want))
	}
	for reason, value := range want {
		t.Run(reason, func(t *testing.T) {
			if got[reason] != value {
				t.Errorf("%s = %d, want %d", reason, got[reason], value)
			}
		})
	}
}

// reasonValues indexes an eviction sum's data points by their reason.
func reasonValues(t *testing.T, sum metricdata.Sum[int64]) map[string]int64 {
	t.Helper()

	values := map[string]int64{}
	for _, point := range sum.DataPoints {
		reason, present := point.Attributes.Value(AttrPoolEvictionReason)
		if !present {
			t.Errorf("an eviction data point carries no %s attribute", AttrPoolEvictionReason)
			continue
		}
		values[reason.AsString()] = point.Value
	}
	return values
}

// TestObservePool_ObservesEveryReasonBeforeAnythingIsEvicted covers the
// deliberate zeros.
//
// A counter that appears only after its first eviction cannot be graphed or
// alerted on until the moment it already matters, and an operator watching an
// empty panel cannot tell "nothing has been evicted" from "this build does not
// export that". The vocabulary is closed at seven values, so publishing all of
// them costs less than one dimension of the request instrument.
func TestObservePool_ObservesEveryReasonBeforeAnythingIsEvicted(t *testing.T) {
	reader, restore := newMetricRecorder(t)
	t.Cleanup(restore)

	registration, err := ObservePool(func() PoolCounts { return PoolCounts{MaxSize: 100} })
	if err != nil {
		t.Fatalf("ObservePool() error: %v", err)
	}
	t.Cleanup(func() { _ = registration.Unregister() })

	points := sumPoints(t, collectedMetric(t, reader, poolEvictionsInstrument))
	if len(points) != 7 {
		t.Fatalf("got %d reason series on a pool that has evicted nothing, want 7", len(points))
	}
	for _, point := range points {
		if point.Value != 0 {
			t.Errorf("a reason reported %d evictions on an untouched pool", point.Value)
		}
	}
}

// TestObservePool_WithoutAReadFunction_IsRefused pins the refusal rather than a
// permanent zero.
//
// A registered callback with nothing to read exports zeros forever, which is
// indistinguishable from a healthy empty pool and would hide the wiring mistake
// it comes from.
func TestObservePool_WithoutAReadFunction_IsRefused(t *testing.T) {
	registration, err := ObservePool(nil)
	if !errors.Is(err, ErrNoPoolReader) {
		t.Errorf("ObservePool(nil) error = %v, want ErrNoPoolReader", err)
	}
	if registration != nil {
		t.Error("ObservePool(nil) returned a registration, which nothing would ever unregister")
	}
}

// refusingPoolMeter is a Meter that will not create one named instrument, or
// will not register the callback, standing in for a provider that rejects a
// registration: a duplicate under a different description, or a name a future
// SDK validates more strictly.
//
// It embeds the interface rather than implementing it, so the fake stays valid
// as the Meter interface grows.
type refusingPoolMeter struct {
	apimetric.Meter
	// refuse is the instrument name to reject, or "callback" to reject the
	// registration itself.
	refuse string
	err    error
}

func (m refusingPoolMeter) Int64ObservableUpDownCounter(
	name string, opts ...apimetric.Int64ObservableUpDownCounterOption,
) (apimetric.Int64ObservableUpDownCounter, error) {
	if name == m.refuse {
		return nil, m.err
	}
	return m.Meter.Int64ObservableUpDownCounter(name, opts...)
}

func (m refusingPoolMeter) Int64ObservableCounter(
	name string, opts ...apimetric.Int64ObservableCounterOption,
) (apimetric.Int64ObservableCounter, error) {
	if name == m.refuse {
		return nil, m.err
	}
	return m.Meter.Int64ObservableCounter(name, opts...)
}

func (m refusingPoolMeter) RegisterCallback(
	callback apimetric.Callback, instruments ...apimetric.Observable,
) (apimetric.Registration, error) {
	if m.refuse == "callback" {
		return nil, m.err
	}
	return m.Meter.RegisterCallback(callback, instruments...)
}

// TestObservePool_ARefusedRegistration_IsReturnedNotSwallowed covers every
// point at which the provider can say no.
//
// Returned rather than reported through otel.Handle, unlike this package's
// synchronous instruments: those degrade to a nil instrument that records
// nothing and harms nobody, while a half-built set of asynchronous instruments
// has no callback and therefore no reason to exist. The caller logs it and
// keeps serving.
func TestObservePool_ARefusedRegistration_IsReturnedNotSwallowed(t *testing.T) {
	refused := errors.New("the provider refused this registration")

	for _, refuse := range []string{
		poolEntriesInstrument,
		poolCapacityInstrument,
		poolEvictionsInstrument,
		"callback",
	} {
		t.Run(refuse, func(t *testing.T) {
			meter := refusingPoolMeter{
				Meter:  noopmetric.NewMeterProvider().Meter("test"),
				refuse: refuse,
				err:    refused,
			}

			registration, err := observePool(meter, func() PoolCounts { return samplePoolCounts })
			if !errors.Is(err, refused) {
				t.Errorf("observePool() error = %v, want the provider's refusal", err)
			}
			if registration != nil {
				t.Error("observePool() returned a registration despite the refusal")
			}
		})
	}
}

// TestObservePool_Unregistering_StopsTheReads pins the half that matters at
// shutdown: the callback reads the pool, so leaving it registered would keep a
// closed pool reachable and being read.
func TestObservePool_Unregistering_StopsTheReads(t *testing.T) {
	reader, restore := newMetricRecorder(t)
	t.Cleanup(restore)

	reads := 0
	registration, err := ObservePool(func() PoolCounts {
		reads++
		return samplePoolCounts
	})
	if err != nil {
		t.Fatalf("ObservePool() error: %v", err)
	}

	var collected metricdata.ResourceMetrics
	if collectErr := reader.Collect(context.Background(), &collected); collectErr != nil {
		t.Fatalf("collecting metrics: %v", collectErr)
	}
	if reads != 1 {
		t.Fatalf("the callback ran %d times for one collection, want 1", reads)
	}

	if unregisterErr := registration.Unregister(); unregisterErr != nil {
		t.Fatalf("Unregister() error: %v", unregisterErr)
	}
	if collectErr := reader.Collect(context.Background(), &collected); collectErr != nil {
		t.Fatalf("collecting metrics after unregistering: %v", collectErr)
	}
	if reads != 1 {
		t.Errorf("the callback ran %d times after unregistering, want it to have stopped at 1", reads)
	}
}
