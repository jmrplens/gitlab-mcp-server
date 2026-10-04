// scopes_test.go contains unit tests for GitLab token scope validation
// and scope-checking helpers.
package gitlab

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
)

// TestDetectToken_SelfAnswers_ReadsTheTokensKindAndID verifies what the self
// endpoint's answers are read as: a classic token's scopes and id with no
// grant to read; a fine-grained token's single scope and its id with its grant
// readable; a fine-grained token with no id reported, whose grant cannot be
// asked for; GitLab's 403 refusing a fine-grained token Personal Access Token:
// Read, read as that token with no id and its grant unreadable; and every
// other failure, a 404, a 503 and a classic 403 among them, as nothing known,
// the token's kind included.
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
			want: TokenFacts{KindUnknown: true},
		},
		{name: "not available", status: http.StatusNotFound, body: `{}`, want: TokenFacts{KindUnknown: true}},
		{name: "unavailable", status: http.StatusServiceUnavailable, body: `{}`, want: TokenFacts{KindUnknown: true}},
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
			if !sameFacts(got, tc.want) {
				t.Errorf("DetectToken() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// sameFacts reports whether two token facts say the same thing, field by field.
func sameFacts(got, want TokenFacts) bool {
	return slices.Equal(got.Scopes, want.Scopes) && got.ID == want.ID && got.FineGrained == want.FineGrained &&
		got.GrantReadable == want.GrantReadable && got.KindUnknown == want.KindUnknown
}

// TestDetectToken_NoAnswer_IsNothingKnown verifies an instance that does not
// answer the self request at all, a connection refused rather than any status,
// is read as nothing known, the token's kind included: a failure that carries
// no response is not GitLab's refusal of a fine-grained token, and reading it
// as one would withhold actions from a classic token whose instance was
// briefly away, while reading it as a classic token for good would serve a
// fine-grained one every action for as long as nothing asked again.
func TestDetectToken_NoAnswer_IsNothingKnown(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	client, err := NewClientWithTokenRetries(url, testValidToken, false, true)
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	got := DetectToken(context.Background(), client.GL())
	if !sameFacts(got, TokenFacts{KindUnknown: true}) {
		t.Errorf("DetectToken() = %+v, want nothing known", got)
	}
}

// TestDetectToken_WarnsOnlyWhenNothingAnswered verifies the one warning a
// start or an entry build gives about the token: written when the self
// endpoint did not answer, since the whole catalog is then served, and not
// when it described the token.
func TestDetectToken_WarnsOnlyWhenNothingAnswered(t *testing.T) {
	var up atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/personal_access_tokens/self", func(w http.ResponseWriter, _ *http.Request) {
		if !up.Load() {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":7,"scopes":["api"],"active":true}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client, err := NewClientWithTokenRetries(srv.URL, testValidToken, false, true)
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}

	var logged bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	DetectToken(context.Background(), client.GL())
	if !strings.Contains(logged.String(), `"level":"WARN","msg":"failed to detect PAT scopes, all tools will be registered"`) {
		t.Errorf("an unanswered detection logged %s; want the warning", logged.String())
	}
	logged.Reset()
	up.Store(true)
	DetectToken(context.Background(), client.GL())
	if strings.Contains(logged.String(), `"level":"WARN"`) {
		t.Errorf("an answered detection logged %s; want no warning", logged.String())
	}
}

// TestRedetectToken_AsksAgainAndLogsAFailureQuietly verifies the read a caller
// repeats for a token whose kind is unknown: it reads an answer as DetectToken
// does and logs it the same way, while a failure, which can repeat on every
// round for as long as the instance does not answer, is logged at DEBUG and
// never as the warning DetectToken gives the first one, and a round the
// process cancelled is not logged at all.
func TestRedetectToken_AsksAgainAndLogsAFailureQuietly(t *testing.T) {
	var up atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/personal_access_tokens/self", func(w http.ResponseWriter, _ *http.Request) {
		if !up.Load() {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":9,"scopes":["granular"],"active":true}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client, err := NewClientWithTokenRetries(srv.URL, testValidToken, false, true)
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}

	var logged bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	if got := RedetectToken(context.Background(), client.GL()); !sameFacts(got, TokenFacts{KindUnknown: true}) {
		t.Errorf("RedetectToken() with no answer = %+v, want the kind still unknown", got)
	}
	if !strings.Contains(logged.String(), `"level":"DEBUG","msg":"the token's kind is still unknown"`) ||
		strings.Contains(logged.String(), `"level":"WARN"`) {
		t.Errorf("a failed re-read logged %s; want one DEBUG line and no warning", logged.String())
	}

	logged.Reset()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if got := RedetectToken(cancelled, client.GL()); !sameFacts(got, TokenFacts{KindUnknown: true}) {
		t.Errorf("RedetectToken() on a cancelled round = %+v, want the kind still unknown", got)
	}
	if logged.Len() != 0 {
		t.Errorf("a cancelled round logged %s; want nothing", logged.String())
	}

	up.Store(true)
	want := TokenFacts{Scopes: []string{ScopeGranular}, ID: 9, FineGrained: true, GrantReadable: true}
	if got := RedetectToken(context.Background(), client.GL()); !sameFacts(got, want) {
		t.Errorf("RedetectToken() once answered = %+v, want %+v", got, want)
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
