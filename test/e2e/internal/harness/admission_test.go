//go:build e2e

// admission_test.go covers the reading of an HTTP gate's answer that a refused
// admission is held to, which needs no GitLab: only a status and a body.

package harness

import (
	"net/http"
	"strings"
	"testing"
)

// TestGateRefusal_ReadsTheStatusAndTheJSONRPCError holds what a refused
// admission reports of an HTTP gate's answer. A status outside 2xx carrying a
// JSON-RPC error is the refusal, read whole, at the edges of the range as well
// as at the 403 the minimum is refused with. A 2xx is the credential admitted,
// whatever its body says, and a refusal whose body carries no error is not one
// this server's gate writes, so neither is taken for the refusal a scenario
// asserts.
func TestGateRefusal_ReadsTheStatusAndTheJSONRPCError(t *testing.T) {
	const refusalBody = `{"jsonrpc":"2.0","id":1,"error":{"code":-40300,"message":"GitLab accepted this token, which carries neither the read_api nor the api scope"}}`
	tests := []struct {
		name    string
		status  int
		body    string
		want    AdmissionRefusal
		wantErr string
	}{
		{
			name: "the minimum's 403", status: http.StatusForbidden, body: refusalBody,
			want: AdmissionRefusal{Status: http.StatusForbidden, Code: -40300, Message: "GitLab accepted this token, which carries neither the read_api nor the api scope"},
		},
		{
			name: "the first status past 2xx", status: http.StatusMultipleChoices, body: refusalBody,
			want: AdmissionRefusal{Status: http.StatusMultipleChoices, Code: -40300, Message: "GitLab accepted this token, which carries neither the read_api nor the api scope"},
		},
		{name: "a 200 admits", status: http.StatusOK, body: refusalBody, wantErr: "admitted with 200"},
		{name: "the last 2xx admits", status: 299, body: refusalBody, wantErr: "admitted with 299"},
		{name: "a body that is not JSON", status: http.StatusForbidden, body: "Forbidden", wantErr: "carries no JSON-RPC error"},
		{name: "JSON with no error", status: http.StatusForbidden, body: `{"jsonrpc":"2.0","id":1,"result":{}}`, wantErr: "carries no JSON-RPC error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gateRefusal(tt.status, []byte(tt.body))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("gateRefusal(%d) error = %v, want one saying %q", tt.status, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("gateRefusal(%d) error = %v, want the refusal", tt.status, err)
			}
			if got != tt.want {
				t.Errorf("gateRefusal(%d) = %+v, want %+v", tt.status, got, tt.want)
			}
		})
	}
}
