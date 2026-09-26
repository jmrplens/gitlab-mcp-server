package tenancy

import (
	"net/http"
	"testing"
)

// TestCodes_AreTheirLiterals pins every code the register declares to the
// number a client matches on. A refusal's code is part of its wire shape
// (spec: Refusal channels), so a change here is a change clients see.
func TestCodes_AreTheirLiterals(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  int
		want int
	}{
		{"CodeUnauthorized", CodeUnauthorized, -40100},
		{"CodeForbidden", CodeForbidden, -40300},
		{"CodeTooManyRequests", CodeTooManyRequests, -42900},
		{"CodeUnavailable", CodeUnavailable, -50300},
		{"CodeServerBusyLegacy", CodeServerBusyLegacy, -32000},
		{"codeParseError", codeParseError, -32700},
		{"codeInvalidRequest", codeInvalidRequest, -32600},
		{"codeMethodNotFound", codeMethodNotFound, -32601},
		{"codeInvalidParams", codeInvalidParams, -32602},
		{"codeInternalError", codeInternalError, -32603},
		{"codeHeaderMismatch", codeHeaderMismatch, -32020},
		{"codeMissingRequiredClientCapability", codeMissingRequiredClientCapability, -32021},
		{"codeUnsupportedProtocolVersion", codeUnsupportedProtocolVersion, -32022},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s = %d, want %d", tc.name, tc.got, tc.want)
			}
		})
	}
}

// TestCodes_GateCodesMirrorTheirStatus holds each gate code to its HTTP status
// times -100, which is what keeps them outside the JSON-RPC reserved range
// and tells a client the status from the body alone.
func TestCodes_GateCodesMirrorTheirStatus(t *testing.T) {
	for _, tc := range []struct {
		name   string
		code   int
		status int
	}{
		{"unauthorized", CodeUnauthorized, http.StatusUnauthorized},
		{"forbidden", CodeForbidden, http.StatusForbidden},
		{"too-many-requests", CodeTooManyRequests, http.StatusTooManyRequests},
		{"unavailable", CodeUnavailable, http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.code != tc.status*-100 {
				t.Errorf("code %d does not mirror status %d", tc.code, tc.status)
			}
			if tc.code >= -32768 && tc.code <= -32000 {
				t.Errorf("code %d is inside the JSON-RPC reserved range", tc.code)
			}
		})
	}
}

// TestCodes_ProtocolClassification separates the codes MCP defines in its own
// reserved sub-range, and JSON-RPC's own codes, from every other.
func TestCodes_ProtocolClassification(t *testing.T) {
	for _, tc := range []struct {
		name     string
		code     int
		defined  bool
		standard bool
	}{
		{"header-mismatch", -32020, true, false},
		{"missing-capability", -32021, true, false},
		{"unsupported-version", -32022, true, false},
		{"url-elicitation-2025", -32042, false, false},
		{"parse-error", -32700, false, true},
		{"invalid-request", -32600, false, true},
		{"method-not-found", -32601, false, true},
		{"invalid-params", -32602, false, true},
		{"internal-error", -32603, false, true},
		{"server-busy", -32000, false, false},
		{"too-many-requests", -42900, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := mcpDefinesCode(tc.code); got != tc.defined {
				t.Errorf("mcpDefinesCode(%d) = %v, want %v", tc.code, got, tc.defined)
			}
			if got := standardJSONRPCCode(tc.code); got != tc.standard {
				t.Errorf("standardJSONRPCCode(%d) = %v, want %v", tc.code, got, tc.standard)
			}
		})
	}
}
