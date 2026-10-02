package gitlab

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
)

// TokenFacts is what the personal access token self endpoint says about the
// token a client authenticates with.
type TokenFacts struct {
	// Scopes are the token's scopes, nil when they are not known: the endpoint
	// failed, or does not answer for this kind of token (an OAuth access token,
	// an instance older than 16.0). A fine-grained token's list is the single
	// value [ScopeGranular].
	Scopes []string
	// ID is the token's own id, 0 when it is not known. It is held to read the
	// token's grant ([ReadGrant]) and is never logged.
	ID int64
	// FineGrained is set for a fine-grained personal access token.
	FineGrained bool
	// GrantReadable is set when the token may read its own description, which
	// is the permission (Personal Access Token: Read) its grant is read with
	// too, and its id is known.
	GrantReadable bool
	// KindUnknown is set when nothing has said whether the token is a
	// fine-grained one: the self endpoint gave neither a description nor
	// GitLab's refusal of Personal Access Token: Read ([DetectToken]), it was
	// not asked, or the OAuth verifier's scopes are its own assumption rather
	// than an answer. Such a token is served as a classic one whose scopes are
	// what the rest of these facts say, and its kind is asked again
	// ([RedetectToken]) until GitLab answers, because a fine-grained token
	// served as a classic one is served every action, the ones no grant reaches
	// among them.
	KindUnknown bool
}

// DetectToken asks the GitLab personal access token self endpoint what the
// token the client authenticates with is: its scopes, its id, and whether it
// is a fine-grained token whose grant it may read.
//
// It is one request, the one scope detection has always made. A 403 carrying
// insufficient_granular_scope is an answer and not a failure: the route's
// boundary is the user and names no root namespace, so no enforcement of
// fine-grained tokens applies to it, and only a fine-grained token is refused a
// grant there. Such a token lacks Personal Access Token: Read, so its grant
// cannot be read, and it is reported with its one scope and no id. Any other
// failure is reported as nothing known, its kind included
// ([TokenFacts.KindUnknown]), which serves every tool, as a failed scope
// detection always has (ADR-0018), until the kind is asked again.
//
// The log line names the scopes and whether the token is fine-grained, never
// its id.
func DetectToken(ctx context.Context, client *gl.Client) TokenFacts {
	facts, err := detectToken(ctx, client)
	if err != nil {
		slog.WarnContext(ctx, "failed to detect PAT scopes, all tools will be registered", "error", err)
	}
	return facts
}

// RedetectToken is [DetectToken] for a token whose kind an earlier read left
// unknown, which a caller asks again on every revalidation round, on the stdio
// timer and once a degraded stdio start recovers, until GitLab answers. Its
// failure is logged at DEBUG rather than WARN, since the first one was already
// a warning and the rounds can repeat for as long as the instance does not
// answer. A round whose context was cancelled is not logged at all: the
// process ended it, which says nothing about whether GitLab answers, and a line
// written then would outlive whatever ended it.
func RedetectToken(ctx context.Context, client *gl.Client) TokenFacts {
	facts, err := detectToken(ctx, client)
	if err != nil && !errors.Is(ctx.Err(), context.Canceled) {
		slog.DebugContext(ctx, "the token's kind is still unknown", "error", err)
	}
	return facts
}

// detectToken is the one request of [DetectToken] and [RedetectToken], with
// the error that left the token's kind unknown returned for the caller to log
// at its own level.
func detectToken(ctx context.Context, client *gl.Client) (TokenFacts, error) {
	token, _, err := client.PersonalAccessTokens.GetSinglePersonalAccessToken(gl.WithContext(ctx))
	if err != nil {
		if refusedAGrant(err) {
			slog.InfoContext(ctx, "the token is a fine-grained personal access token that may not read its own grant",
				"scopes", []string{ScopeGranular})
			return TokenFacts{Scopes: []string{ScopeGranular}, FineGrained: true}, nil
		}
		return TokenFacts{KindUnknown: true}, err
	}
	facts := FactsFromScopes(token.Scopes, token.ID)
	slog.InfoContext(ctx, "detected PAT scopes", "scopes", token.Scopes, "fine_grained", facts.FineGrained)
	return facts, nil
}

// FactsFromScopes builds the token facts of a token whose scopes and id were
// read elsewhere, by the OAuth verifier's introspection of the same token on
// the same instance, with the one rule [DetectToken] applies: the kind from
// the scope list, and the grant readable when the token is fine-grained and
// its id is known.
func FactsFromScopes(scopes []string, id int64) TokenFacts {
	facts := TokenFacts{Scopes: scopes, ID: id, FineGrained: FineGrained(scopes)}
	facts.GrantReadable = facts.FineGrained && facts.ID != 0
	return facts
}

// refusedAGrant reports whether err is GitLab's 403 refusing a fine-grained
// token a permission its grant lacks ([PermissionRefusal]).
func refusedAGrant(err error) bool {
	refusal, isResponse := errors.AsType[*gl.ErrorResponse](err)
	if !isResponse || refusal.StatusCode != http.StatusForbidden {
		return false
	}
	_, missing := PermissionRefusal(refusal.Body)
	return missing
}

// ScopeAPI is the GitLab scope that permits writes.
//
// Only the write scope is named here, and deliberately: this package answers
// one question — can this token mutate GitLab — and the read scope is not
// part of that answer. internal/oauth owns the full scope vocabulary for the
// authorization layer; duplicating it here would be a second place to keep
// in step with GitLab.
const ScopeAPI = "api"

// ScopeGranular is the one legacy scope GitLab gives a fine-grained personal
// access token. Its authority is not a scope at all but a grant of named
// permissions per namespace, which the scope list does not carry.
//
// It is named here, beside the write scope, for the same reason: it changes the
// answer to whether a token can write, and to which scopes the catalog filter
// may read.
const ScopeGranular = "granular"

// FineGrained reports whether a scope list is the one a fine-grained personal
// access token presents: exactly ScopeGranular and nothing else.
//
// GitLab creates such a token with that single legacy scope
// (app/services/authn/personal_access_tokens/create_granular_service.rb at
// v19.4.1-ee, the one service both the REST and the GraphQL creation paths
// call), and the list is the only place the kind shows in what client-go
// decodes of the PAT self endpoint this server already asks. GitLab sends the
// kind there too, as granular: true on every token
// (lib/api/entities/personal_access_token.rb), but client-go's
// PersonalAccessToken does not model that field (row 32 of
// docs/development/upstream-bugs.md); a reader that needs it takes it from the
// captured response (ADR-0021). For every token GitLab can mint the two agree,
// since the service that sets granular also sets this list. The RFC 6750 code
// insufficient_granular_scope does not prove the kind: GitLab answers a classic
// token with the same code under a root namespace that enforces fine-grained
// tokens. A list that carries the scope beside others is not this shape, and is
// read as the classic scopes it spells.
//
// The list reaches this predicate from the self endpoint only when the token
// may read itself: that endpoint requires Personal Access Token: Read of a
// fine-grained token, and without it GitLab answers 403
// insufficient_granular_scope, which [DetectToken] reads as a fine-grained
// token with no id whose grant cannot be read, reporting this list for it
// without asking this predicate.
func FineGrained(scopes []string) bool {
	return len(scopes) == 1 && scopes[0] == ScopeGranular
}

// WriteCapable reports whether a token's scopes permit mutating GitLab.
//
// Unknown scopes (nil: detection failed, was disabled, or the instance is
// too old to answer) count as write-capable. Assuming otherwise would
// silently strip every mutating tool from a deployment whose token is
// perfectly able to use them, and a wrong "no" is invisible — the tools are
// simply not there — while a wrong "yes" surfaces as GitLab's own 403 on the
// call that actually tried to write.
//
// A fine-grained token counts as write-capable for the same reason: its scope
// list is the single value ScopeGranular, which says nothing about what it may
// do, so it is unknown authority and not a read-only token. GitLab judges each
// call against the permissions the token was granted, and a write outside them
// is refused there, with GitLab's own 403 naming the missing permission.
func WriteCapable(scopes []string) bool {
	if scopes == nil || FineGrained(scopes) {
		return true
	}
	return slices.Contains(scopes, ScopeAPI)
}

// CatalogScopes returns the scope list the catalog's scope filter and its cache
// key read for a token: the token's own list, or nil when that list says
// nothing about what the token may reach.
//
// A fine-grained token is the one case that maps to nil. Its list carries no
// legacy scope a group requirement can name, so reading it literally would
// remove every group that requires one (the admin_mode groups) on the strength
// of a scope the token cannot carry, and would key its catalog exactly as an
// empty list keys one while it must be filtered as unknown. Mapped to nil, it
// shares the filter and the key of a classic token whose scopes are unknown,
// which is what both are: served the unfiltered catalog, with GitLab deciding
// each call.
//
// It is one function because the filter and the key must read the same list:
// a key that told the two cases apart while the filter did not, or the reverse,
// would serve one of them a catalog it did not earn.
func CatalogScopes(scopes []string) []string {
	if FineGrained(scopes) {
		return nil
	}
	return scopes
}

// NarrowToTokenScope marks a server configuration read-only when the token
// it was built for cannot write, and reports whether it did. Both transports
// call it once the scopes are known: the HTTP pool per entry, since an entry
// is per token, and stdio once at startup for its single token. The catalog
// built from the configuration then withholds every write action and reports
// it as withheld by the token scope, rather than listing actions GitLab would
// refuse one by one with its own 403 (ADR-0018).
//
// A configuration already read-only is left alone, so the operator's setting
// and the token's limit never contradict each other in the log, and unknown
// scopes narrow nothing, for the reason WriteCapable gives.
func NarrowToTokenScope(cfg *config.ServerConfig) bool {
	if cfg == nil || cfg.ReadOnly || WriteCapable(cfg.TokenScopes) {
		return false
	}
	cfg.ReadOnly = true
	cfg.ReadOnlyFromTokenScope = true
	slog.Info("token cannot write; serving a read-only tool surface for it",
		"scopes", cfg.TokenScopes)
	return true
}

// ScopeSatisfied checks whether requiredScopes are all present in the
// detected tokenScopes. If tokenScopes is nil (detection failed or disabled),
// returns true (allow all). If requiredScopes is empty, returns true (no
// requirement). A fine-grained token's list is read through [CatalogScopes],
// so it is unknown authority here too and satisfies every requirement.
func ScopeSatisfied(tokenScopes, requiredScopes []string) bool {
	tokenScopes = CatalogScopes(tokenScopes)
	if tokenScopes == nil || len(requiredScopes) == 0 {
		return true
	}
	scopeSet := make(map[string]struct{}, len(tokenScopes))
	for _, s := range tokenScopes {
		scopeSet[s] = struct{}{}
	}
	for _, req := range requiredScopes {
		if _, ok := scopeSet[req]; !ok {
			return false
		}
	}
	return true
}
