package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/baggage"
)

// OutboundContext strips a caller's baggage before this server makes a request
// of its own.
//
// # Why this is an action rather than a default
//
// It is tempting to assume nothing forwards baggage unless asked. That is
// false. propagation.Baggage.Inject writes baggage.FromContext(ctx).String()
// unconditionally whenever it is non-empty, and the global propagator this
// package installs contains propagation.Baggage. So the moment the GitLab
// client's transport is instrumented, which adopting traces implies, and the
// inbound request's context reaches client-go, which is the ordinary Go idiom,
// a client's baggage rides outbound. Nobody has to opt in for the leak; someone
// has to opt out for it not to happen. This function is that opt-out, and it is
// meant to be called at every point where a context that came from a caller
// becomes a request this server makes.
//
// # The shape of the exposure
//
// This server sits between a client it does not control and a GitLab instance
// it calls with a privileged credential. Baggage is a header a client fills in
// freely: "Baggage values are any valid UTF-8 strings. Language API MUST accept
// any valid UTF-8 string as baggage value in Set and return the same value from
// Get." A well-formed header of 64 keys and 8192 bytes, all attacker-chosen,
// parses cleanly and would arrive at the customer's GitLab instance carrying
// this server's name.
//
// # This declines a documented SHOULD, on purpose
//
// W3C says "A system receiving a baggage request header SHOULD send it to
// outgoing requests." We do not. The same section provides the escape hatch in
// the same breath ("Any key/value pair MAY be deleted"), and the OTel Baggage
// API requires the facility used here to exist for exactly this reason: "To
// avoid sending any name/value pairs to an untrusted process, the Baggage API
// MUST provide a way to remove all baggage entries from a context." Declining a
// SHOULD is legitimate when the reason is recorded, and the reason is that the
// process downstream of us belongs to someone else.
//
// The trace context itself is untouched. Only baggage is cleared, so a
// distributed trace still joins up across the call.
func OutboundContext(ctx context.Context) context.Context {
	return baggage.ContextWithoutBaggage(ctx)
}
