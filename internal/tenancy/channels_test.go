package tenancy

import (
	"strings"
	"testing"
)

// TestCarriages_AreSpecSection421 pins the carried-channel matrix to spec
// section 4.2.1, row by row. An SDK upgrade that changes what is carried
// edits this table and the matrix in the same pull request.
func TestCarriages_AreSpecSection421(t *testing.T) {
	want := []struct {
		methods  string
		era      Era
		channels string
	}{
		{"tools/list", EraAny, "rpc,absent"},
		{"prompts/list,resources/list,resources/templates/list", EraAny, "rpc"},
		{"tools/call", EraAny, "rpc,tool-error,withheld,unknown,absent"},
		{"resources/read,prompts/get", EraAny, "rpc"},
		{"completion/complete", EraAny, "rpc,empty-completion"},
		{"resources/subscribe", EraLegacy, "rpc"},
		{"subscriptions/listen", EraModern, "rpc,listen-end"},
		{"initialize", EraLegacy, "rpc"},
		{"server/discover", EraModern, "rpc"},
		{"notifications/resources/updated", EraAny, "silent"},
		{"http", EraAny, "gate,silent"},
		{"eviction,expiry", EraLegacy, "session-close"},
		{"startup", EraAny, "startup"},
	}
	got := Carriages()
	if len(got) != len(want) {
		t.Fatalf("%d rows, want %d", len(got), len(want))
	}
	for i, w := range want {
		t.Run(w.methods, func(t *testing.T) {
			var channels []string
			for _, c := range got[i].Channels {
				channels = append(channels, c.String())
			}
			if m := strings.Join(got[i].Methods, ","); m != w.methods || got[i].Era != w.era ||
				strings.Join(channels, ",") != w.channels {
				t.Errorf("row %d = %s, era %d, %v; want %s, era %d, %s",
					i, m, got[i].Era, channels, w.methods, w.era, w.channels)
			}
		})
	}
}

// TestCarries_AnswersPerMethodAndEra walks the cases INV-011 names: only
// tools/call has an error flag, only completion an empty answer, only the gate
// a status, and a listen exists only in the modern era, a session-era
// subscribe only in the legacy one.
func TestCarries_AnswersPerMethodAndEra(t *testing.T) {
	for _, tc := range []struct {
		name    string
		method  string
		era     Era
		channel Channel
		want    bool
	}{
		{"a tool call refused as a result", "tools/call", EraAny, ToolError, true},
		{"a listing refused as a result", "tools/list", EraAny, ToolError, false},
		{"a read refused as a result", "resources/read", EraAny, ToolError, false},
		{"a listing refused in-band", "tools/list", EraAny, RPC, true},
		{"a completion answered empty", "completion/complete", EraAny, EmptyCompletion, true},
		{"a tool call answered empty", "tools/call", EraAny, EmptyCompletion, false},
		{"the gate's status", "http", EraAny, Gate, true},
		{"a status inside the SDK", "tools/call", EraAny, Gate, false},
		{"a listen in the modern era", "subscriptions/listen", EraModern, RPC, true},
		{"a listen in the legacy era", "subscriptions/listen", EraLegacy, RPC, false},
		{"a listen in either era", "subscriptions/listen", EraAny, ListenEnd, true},
		{"a session-era subscribe in the legacy era", "resources/subscribe", EraLegacy, RPC, true},
		{"a session-era subscribe in the modern era", "resources/subscribe", EraModern, RPC, false},
		{"an ending on a listing", "tools/list", EraAny, ListenEnd, false},
		{"a session closed on eviction", "eviction", EraLegacy, SessionClose, true},
		{"a session closed in the modern era", "eviction", EraModern, SessionClose, false},
		{"a refusal to start", "startup", EraAny, Startup, true},
		{"a method nobody carries", "no/such/method", EraAny, RPC, false},
		{"stdio carries an in-band error", "tools/list", EraStdio, RPC, true},
		{"stdio carries a tool error", "tools/call", EraStdio, ToolError, true},
		{"stdio has no status", "http", EraStdio, Gate, false},
		{"stdio has no session to close", "eviction", EraStdio, SessionClose, false},
		{"stdio speaks both eras", "subscriptions/listen", EraStdio, ListenEnd, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Carries(tc.method, tc.era, tc.channel); got != tc.want {
				t.Errorf("Carries(%q, %d, %s) = %v, want %v", tc.method, tc.era, tc.channel, got, tc.want)
			}
		})
	}
}
