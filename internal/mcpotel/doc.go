// Package mcpotel instruments MCP request handling with OpenTelemetry.
//
// It imports only the OpenTelemetry API, never the SDK. Which providers exist,
// where they export and whether they exist at all is decided once in
// internal/telemetry; this package asks the global providers for a tracer and a
// meter and gets working no-ops when nothing is installed. That is why there is
// no "telemetry enabled" flag anywhere here, and why there must never be one:
// the API is specified to work without an SDK, so a flag would only add a
// branch that can disagree with reality.
package mcpotel
