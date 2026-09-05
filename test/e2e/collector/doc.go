// Package collectore2e proves that a real OTLP receiver accepts what the
// server exports: traces, metrics and logs reach a collector started for the
// test, with the identity policy and the metric views applied as configured.
// It exercises the exporters the OpenTelemetry SDK provides, which the unit
// tests replace with fakes.
//
// The tests carry the collectore2e build tag and run through
// make test-e2e-collector; this file is what a plain build sees of the
// package.
package collectore2e
