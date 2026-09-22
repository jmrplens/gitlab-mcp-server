package mcpotel

import (
	"errors"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// sampleAuthBlockCounts is one reading with every counter at a different
// figure, so a callback that published the wrong one under a reason would show
// as the wrong number rather than as a coincidence.
var sampleAuthBlockCounts = AuthBlockCounts{
	FailureLockout:  7,
	TransportSource: 3,
	DistinctTokens:  11,
}

// observingSampleAuthBlocks registers the callback against a real SDK reader
// and returns the reader, the way observingSamplePool does for the pool.
func observingSampleAuthBlocks(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()

	reader, restore := newMetricRecorder(t)
	t.Cleanup(restore)

	registration, err := ObserveAuthBlocks(func() AuthBlockCounts { return sampleAuthBlockCounts })
	if err != nil {
		t.Fatalf("ObserveAuthBlocks() error: %v", err)
	}
	t.Cleanup(func() { _ = registration.Unregister() })
	return reader
}

// TestObserveAuthBlocks_PublishesOneSeriesPerBudget covers what the reason
// dimension is for: an operator has to be able to tell which rule is doing the
// refusing, since the three answer different questions and only one of them
// escalates.
func TestObserveAuthBlocks_PublishesOneSeriesPerBudget(t *testing.T) {
	reader := observingSampleAuthBlocks(t)
	m := collectedMetric(t, reader, authBlocksInstrument)

	if m.Unit != "{request}" {
		t.Errorf("unit = %q, want {request}", m.Unit)
	}
	if m.Description == "" {
		t.Error("the instrument carries no description, so a collector shows a bare name")
	}
	if sum, ok := m.Data.(metricdata.Sum[int64]); ok && !sum.IsMonotonic {
		t.Error("refusals were published as non-monotonic; a count of what was turned away only rises")
	}

	want := map[string]int64{
		AuthBlockFailureLockout:  sampleAuthBlockCounts.FailureLockout,
		AuthBlockTransportSource: sampleAuthBlockCounts.TransportSource,
		AuthBlockDistinctTokens:  sampleAuthBlockCounts.DistinctTokens,
	}
	points := sumPoints(t, m)
	if len(points) != len(want) {
		t.Fatalf("got %d data points, want %d: one per budget", len(points), len(want))
	}
	for _, p := range points {
		reason, ok := p.Attributes.Value(AttrAuthBlockReason)
		if !ok {
			t.Errorf("a data point carries no %s attribute", AttrAuthBlockReason)
			continue
		}
		expected, known := want[reason.AsString()]
		if !known {
			t.Errorf("unexpected reason %q", reason.AsString())
			continue
		}
		if p.Value != expected {
			t.Errorf("reason %q = %d, want %d", reason.AsString(), p.Value, expected)
		}
	}
}

// TestObserveAuthBlocks_CarriesNoAddress is the privacy assertion, and it is
// the reason this instrument exists in this shape.
//
// An address is who was refused. As a metric dimension it would make the
// series identify people, and it would grow without bound under exactly the
// traffic the counter exists to measure, since a sprayer rotating addresses
// mints a new one per request.
func TestObserveAuthBlocks_CarriesNoAddress(t *testing.T) {
	reader := observingSampleAuthBlocks(t)
	m := collectedMetric(t, reader, authBlocksInstrument)

	for _, p := range sumPoints(t, m) {
		iter := p.Attributes.Iter()
		for iter.Next() {
			kv := iter.Attribute()
			if kv.Key != AttrAuthBlockReason {
				t.Errorf("a data point carries the attribute %q; the reason is the only dimension this may have", kv.Key)
			}
		}
	}
}

// TestObserveAuthBlocks_WithoutAReadFunction_IsRefused pins the refusal rather
// than a permanent zero, which would read as a deployment refusing nobody
// instead of as a wiring mistake.
func TestObserveAuthBlocks_WithoutAReadFunction_IsRefused(t *testing.T) {
	t.Parallel()
	registration, err := observeAuthBlocks(noop.NewMeterProvider().Meter("test"), nil)
	if !errors.Is(err, ErrNoAuthBlockReader) {
		t.Errorf("observeAuthBlocks(nil) error = %v, want ErrNoAuthBlockReader", err)
	}
	if registration != nil {
		t.Error("a refused registration returned something to unregister")
	}
}

// TestObserveAuthBlocks_ARefusedRegistration_IsReturnedNotSwallowed covers the
// provider that will not create the instrument or take the callback: the
// caller has to learn that the series is missing, since the alternative is a
// dashboard that is silently blank and looks like a collector problem at the
// other end.
//
// It reuses the pool test's refusing meter rather than declaring another,
// because the two questions are the same one and a second fake would drift.
func TestObserveAuthBlocks_ARefusedRegistration_IsReturnedNotSwallowed(t *testing.T) {
	refused := errors.New("the provider refused this registration")

	for _, refuse := range []string{authBlocksInstrument, "callback"} {
		t.Run(refuse, func(t *testing.T) {
			meter := refusingPoolMeter{
				Meter:  noop.NewMeterProvider().Meter("test"),
				refuse: refuse,
				err:    refused,
			}

			registration, err := observeAuthBlocks(meter, func() AuthBlockCounts { return sampleAuthBlockCounts })
			if !errors.Is(err, refused) {
				t.Errorf("observeAuthBlocks() error = %v, want the provider's refusal", err)
			}
			if registration != nil {
				t.Error("observeAuthBlocks() returned a registration despite the refusal")
			}
		})
	}
}

// TestAuthBlockReasons_AreDistinct guards the closed vocabulary: two reasons
// that collided would merge two budgets into one series, and the merge would
// look exactly like a rule firing twice as often.
func TestAuthBlockReasons_AreDistinct(t *testing.T) {
	t.Parallel()
	reasons := []string{AuthBlockFailureLockout, AuthBlockTransportSource, AuthBlockDistinctTokens}

	distinct := map[string]bool{}
	for _, r := range reasons {
		distinct[r] = true
	}
	if len(distinct) != len(reasons) {
		t.Errorf("the reasons are not distinct: %v collapses to %d values", reasons, len(distinct))
	}
	if distinct[""] {
		t.Error("a reason is the empty string, which no attribute should carry")
	}
	if AttrAuthBlockReason == attribute.Key("") {
		t.Error("the attribute key is empty")
	}
}
