package mcpotel

import "testing"

// TestMetaCarrier_KeysNamesOnlyThePropagationKeys covers the method that decides
// what a propagator believes is present.
//
// It is asked for the keys this carrier can supply, not for everything in
// _meta, and the distinction matters: _meta carries the protocol version and
// whatever else a client put there, and reporting those as propagation keys
// would describe a carrier that cannot deliver them.
func TestMetaCarrier_KeysNamesOnlyThePropagationKeys(t *testing.T) {
	carrier := metaCarrier{meta: map[string]any{
		"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		"baggage":     "key=value",
		// Present, and not a propagation key.
		"io.modelcontextprotocol/protocolVersion": "2026-07-28",
		// Present under the right name and the wrong type, which the getter
		// already declines, so the key list must decline it too.
		"tracestate": 42,
	}}

	keys := carrier.Keys()

	want := map[string]bool{"traceparent": true, "baggage": true}
	for _, key := range keys {
		if !want[key] {
			t.Errorf("Keys reported %q, which this carrier cannot supply", key)
		}
		delete(want, key)
	}
	for missing := range want {
		t.Errorf("Keys did not report %q, which is present and readable", missing)
	}
}

// TestMetaCarrier_SetIsInert pins the deliberate emptiness, which is a decision
// rather than an omission.
//
// Writing a traceparent into a response would hand every caller the identifiers
// of this server's internal spans, which is the outward leak the W3C security
// section warns about. The propagator interface requires the method; nothing
// here may implement it.
func TestMetaCarrier_SetIsInert(t *testing.T) {
	meta := map[string]any{}
	carrier := metaCarrier{meta: meta}

	carrier.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")

	if len(meta) != 0 {
		t.Errorf("Set wrote %v; a response must not carry this server's span identifiers", meta)
	}
}
