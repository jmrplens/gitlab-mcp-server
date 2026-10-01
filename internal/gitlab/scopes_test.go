// scopes_test.go contains unit tests for GitLab token scope validation
// and scope-checking helpers.
package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
)

// TestDetectToken_SelfAnswers_ReadsTheTokensKindAndID verifies what the self
// endpoint's answers are read as: a classic token's scopes and id with no
// grant to read; a fine-grained token's single scope and its id with its grant
// readable; a fine-grained token with no id reported, whose grant cannot be
// asked for; GitLab's 403 refusing a fine-grained token Personal Access Token:
// Read, read as that token with no id and its grant unreadable; and every
// other failure, a 404 and a classic 403 among them, as nothing known.
func TestDetectToken_SelfAnswers_ReadsTheTokensKindAndID(t *testing.T) {
	refusal := `{"error":"insufficient_granular_scope","error_description":"Access denied: This operation requires a ` +
		`fine-grained personal access token with the following user permissions: [read_personal_access_token]"}`
	cases := []struct {
		name   string
		status int
		body   string
		want   TokenFacts
	}{
		{
			name: "classic", status: http.StatusOK, body: `{"id":7,"scopes":["api","read_user"],"active":true}`,
			want: TokenFacts{Scopes: []string{"api", "read_user"}, ID: 7},
		},
		{
			name: "fine-grained", status: http.StatusOK, body: `{"id":9,"scopes":["granular"],"active":true}`,
			want: TokenFacts{Scopes: []string{ScopeGranular}, ID: 9, FineGrained: true, GrantReadable: true},
		},
		{
			name: "fine-grained with no id", status: http.StatusOK, body: `{"scopes":["granular"],"active":true}`,
			want: TokenFacts{Scopes: []string{ScopeGranular}, FineGrained: true},
		},
		{
			name: "fine-grained refused its own description", status: http.StatusForbidden, body: refusal,
			want: TokenFacts{Scopes: []string{ScopeGranular}, FineGrained: true},
		},
		{
			name: "classic refused", status: http.StatusForbidden, body: `{"error":"insufficient_scope"}`,
			want: TokenFacts{},
		},
		{name: "not available", status: http.StatusNotFound, body: `{}`, want: TokenFacts{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/v4/personal_access_tokens/self", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			srv := httptest.NewServer(mux)
			defer srv.Close()

			client, err := NewClientWithTokenRetries(srv.URL, testValidToken, false, true)
			if err != nil {
				t.Fatalf("NewClient() error: %v", err)
			}
			got := DetectToken(context.Background(), client.GL())
			if !slices.Equal(got.Scopes, tc.want.Scopes) || got.ID != tc.want.ID ||
				got.FineGrained != tc.want.FineGrained || got.GrantReadable != tc.want.GrantReadable {
				t.Errorf("DetectToken() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestFactsFromScopes_AppliesDetectTokensRule verifies the facts built from
// scopes and an id read elsewhere follow the rule DetectToken applies: the
// kind from the list, and the grant readable only for a fine-grained token
// whose id is known.
func TestFactsFromScopes_AppliesDetectTokensRule(t *testing.T) {
	for _, tc := range []struct {
		name   string
		scopes []string
		id     int64
		want   TokenFacts
	}{
		{"classic with an id", []string{"api"}, 3, TokenFacts{Scopes: []string{"api"}, ID: 3}},
		{"fine-grained with an id", []string{ScopeGranular}, 3, TokenFacts{Scopes: []string{ScopeGranular}, ID: 3, FineGrained: true, GrantReadable: true}},
		{"fine-grained without one", []string{ScopeGranular}, 0, TokenFacts{Scopes: []string{ScopeGranular}, FineGrained: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := FactsFromScopes(tc.scopes, tc.id)
			if !slices.Equal(got.Scopes, tc.want.Scopes) || got.ID != tc.want.ID ||
				got.FineGrained != tc.want.FineGrained || got.GrantReadable != tc.want.GrantReadable {
				t.Errorf("FactsFromScopes() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestScopeSatisfied_Scenarios_CorrectResult uses table-driven subtests to verify that ScopeSatisfied correctly reports whether the token scopes cover the required scopes across nil, empty, exact, partial, and missing combinations.
func TestScopeSatisfied_Scenarios_CorrectResult(t *testing.T) {
	tests := []struct {
		name     string
		token    []string
		required []string
		want     bool
	}{
		{"nil token scopes allows all", nil, []string{"api"}, true},
		{"empty required always satisfied", []string{"api"}, nil, true},
		{"exact match", []string{"api", "read_user"}, []string{"api"}, true},
		{"multiple required all present", []string{"api", "read_user", "sudo"}, []string{"api", "sudo"}, true},
		{"missing required scope", []string{"read_user"}, []string{"api"}, false},
		{"partial match fails", []string{"api"}, []string{"api", "sudo"}, false},
		{"both empty", []string{}, []string{}, true},
		{"empty token with requirement", []string{}, []string{"api"}, false},
		{"fine-grained token is unknown authority", []string{ScopeGranular}, []string{"api", "admin_mode"}, true},
		{"granular beside classic scopes is read as the classic list", []string{ScopeGranular, "read_api"}, []string{"api"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ScopeSatisfied(tt.token, tt.required)
			if got != tt.want {
				t.Errorf("ScopeSatisfied(%v, %v) = %v, want %v", tt.token, tt.required, got, tt.want)
			}
		})
	}
}

// TestNarrowToTokenScope_NarrowsOnlyATokenThatCannotWrite verifies the one
// decision both transports share: a token without the api scope makes the
// configuration read-only and marks the narrowing as the token's, unknown
// scopes narrow nothing, and a configuration already read-only is left as the
// operator set it.
func TestNarrowToTokenScope_NarrowsOnlyATokenThatCannotWrite(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		cfg           *config.ServerConfig
		wantNarrowed  bool
		wantReadOnly  bool
		wantFromScope bool
	}{
		{name: "read_api narrows", cfg: &config.ServerConfig{TokenScopes: []string{"read_api"}}, wantNarrowed: true, wantReadOnly: true, wantFromScope: true},
		{name: "api stays writable", cfg: &config.ServerConfig{TokenScopes: []string{"api", "read_user"}}},
		{name: "unknown scopes stay writable", cfg: &config.ServerConfig{}},
		{name: "empty scopes narrow", cfg: &config.ServerConfig{TokenScopes: []string{}}, wantNarrowed: true, wantReadOnly: true, wantFromScope: true},
		{name: "a fine-grained token is not read-only", cfg: &config.ServerConfig{TokenScopes: []string{ScopeGranular}}},
		{name: "the operator's read-only is not the token's", cfg: &config.ServerConfig{ReadOnly: true, TokenScopes: []string{"read_api"}}, wantReadOnly: true},
		{name: "nil configuration", cfg: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := NarrowToTokenScope(tt.cfg); got != tt.wantNarrowed {
				t.Errorf("NarrowToTokenScope() = %v, want %v", got, tt.wantNarrowed)
			}
			if tt.cfg == nil {
				return
			}
			if tt.cfg.ReadOnly != tt.wantReadOnly || tt.cfg.ReadOnlyFromTokenScope != tt.wantFromScope {
				t.Errorf("ReadOnly = %v (from scope %v), want %v (from scope %v)", tt.cfg.ReadOnly, tt.cfg.ReadOnlyFromTokenScope, tt.wantReadOnly, tt.wantFromScope)
			}
		})
	}
}

// TestFineGrained_OnlyTheSingleGranularScopeIsTheShape verifies the predicate
// the rest of this file reads the token kind through: the one list GitLab gives
// a fine-grained token, exactly ["granular"], and no other. A list carrying the
// scope beside a classic one is read as the classic list it spells, and the
// unknown and empty lists are not fine-grained either, since they are the two
// cases the fine-grained list used to be confused with.
func TestFineGrained_OnlyTheSingleGranularScopeIsTheShape(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		scopes []string
		want   bool
	}{
		{name: "the fine-grained list", scopes: []string{"granular"}, want: true},
		{name: "unknown scopes", scopes: nil},
		{name: "no scope at all", scopes: []string{}},
		{name: "a classic write token", scopes: []string{"api"}},
		{name: "a classic read token", scopes: []string{"read_api"}},
		{name: "granular beside a classic scope", scopes: []string{"granular", "read_api"}},
		{name: "a classic scope before granular", scopes: []string{"api", "granular"}},
		{name: "granular repeated", scopes: []string{"granular", "granular"}},
		{name: "a different spelling", scopes: []string{"Granular"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := FineGrained(tt.scopes); got != tt.want {
				t.Errorf("FineGrained(%#v) = %v, want %v", tt.scopes, got, tt.want)
			}
		})
	}
}

// TestWriteCapable_FineGrainedIsUnknownAuthority verifies that a token whose
// scopes say nothing about what it may do is served as able to write, the
// unknown list and the fine-grained one alike, while a classic list is read
// literally: only the api scope writes.
func TestWriteCapable_FineGrainedIsUnknownAuthority(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		scopes []string
		want   bool
	}{
		{name: "unknown scopes", scopes: nil, want: true},
		{name: "a fine-grained token", scopes: []string{"granular"}, want: true},
		{name: "api", scopes: []string{"api"}, want: true},
		{name: "api among others", scopes: []string{"read_user", "api", "read_repository"}, want: true},
		{name: "read_api", scopes: []string{"read_api"}},
		{name: "no scope at all", scopes: []string{}},
		{name: "granular beside read_api is the read_api token it spells", scopes: []string{"granular", "read_api"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := WriteCapable(tt.scopes); got != tt.want {
				t.Errorf("WriteCapable(%#v) = %v, want %v", tt.scopes, got, tt.want)
			}
		})
	}
}

// TestCatalogScopes_FineGrainedReadsAsUnknown verifies the list the catalog's
// scope filter and its cache key read: a fine-grained token's list becomes nil,
// the list of a token whose scopes are unknown, and every other list is handed
// back as it came. Nil and empty stay apart, since the filter treats them in
// opposite ways.
func TestCatalogScopes_FineGrainedReadsAsUnknown(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		scopes []string
		want   []string
	}{
		{name: "a fine-grained token", scopes: []string{"granular"}, want: nil},
		{name: "unknown scopes", scopes: nil, want: nil},
		{name: "no scope at all", scopes: []string{}, want: []string{}},
		{name: "a classic list", scopes: []string{"read_api", "admin_mode"}, want: []string{"read_api", "admin_mode"}},
		{name: "granular beside a classic scope", scopes: []string{"granular", "admin_mode"}, want: []string{"granular", "admin_mode"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := CatalogScopes(tt.scopes)
			if (got == nil) != (tt.want == nil) || !slices.Equal(got, tt.want) {
				t.Errorf("CatalogScopes(%#v) = %#v, want %#v", tt.scopes, got, tt.want)
			}
		})
	}
}
