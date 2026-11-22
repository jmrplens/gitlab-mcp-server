// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// GrantMaxBytes is the most of one grant read this server takes in, and
// GrantMaxScopes the most scopes it evaluates (register row RQB-011).
//
// A grant's size is its minter's choice. Each scope may carry every one of the
// 857 assignable permissions GitLab 19.4 names, up to 64 KB of them, and every
// project or group id a creation request lists becomes a scope of its own,
// needing only membership, so a user in a few thousand namespaces can mint a
// grant of tens of MiB. The read is bounded where the body is read
// ([WithResponseLimit]), under the response capture and client-go's decoder
// alike, rather than after either has taken the whole of it in, and the client
// keeps its own ceiling for every other call. A mebibyte holds a thousand
// scopes of a few dozen permissions each with room to spare, which is past any
// grant a person writes by hand; a grant past either bound is read as too
// large and leaves the session in phase A, saying so.
const (
	GrantMaxBytes  int64 = 1 << 20
	GrantMaxScopes       = 1000
)

// ReadGrant reads the grant of the fine-grained personal access token with
// the given id, which must be the token the client authenticates with
// ([DetectToken] read it from the self endpoint).
//
// It is the read for an instance whose self endpoint does not present the
// grant, which GitLab did before 19.5; from 19.5 on the self answer carries
// it and [DetectToken] keeps it, so this is not asked. It reads GET
// /personal_access_tokens/:id under [GrantMaxBytes]: the granular scopes from
// what client-go decodes, and the granular flag, which client-go does not
// model, from the captured response (ADR-0021), and evaluated with at most
// [GrantMaxScopes] scopes ([finegrained.GrantFromScopes]).
//
// What comes back says which of three things happened. A grant and
// [finegrained.FallbackNone]: it was read and can be evaluated. A reason and
// no error: GitLab answered, and the answer cannot be evaluated: refused (the
// token may not read its grant), too large, or holding something the decoder
// would have to guess at. An error: GitLab did not answer the question (a
// transport failure, a timeout, a status that is no verdict on the read), which
// a caller holding an evaluated grant reads as no news rather than as phase A,
// since the grant cannot have changed (nothing edits one after creation).
//
// Nothing of the grant is logged here, and the body is dropped once decoded.
func ReadGrant(ctx context.Context, client *gl.Client, id int64) (finegrained.Grant, finegrained.FallbackReason, error) {
	return readGrant(ctx, func(ctx context.Context) (*gl.PersonalAccessToken, error) {
		token, _, err := client.PersonalAccessTokens.GetSinglePersonalAccessTokenByID(id, gl.WithContext(ctx))
		return token, err
	})
}

// readSelfGrant is [ReadGrant] through the self endpoint, for a token whose
// self answer carried its grant: what a revalidation reads again, with the
// same bounds and the same reading of each answer.
func readSelfGrant(ctx context.Context, client *gl.Client) (finegrained.Grant, finegrained.FallbackReason, error) {
	return readGrant(ctx, func(ctx context.Context) (*gl.PersonalAccessToken, error) {
		token, _, err := client.PersonalAccessTokens.GetSinglePersonalAccessToken(gl.WithContext(ctx))
		return token, err
	})
}

// readGrant makes the one request read under [GrantMaxBytes] with its
// response captured, and reads the grant out of the answer as [ReadGrant]
// says.
func readGrant(ctx context.Context, read func(context.Context) (*gl.PersonalAccessToken, error)) (finegrained.Grant, finegrained.FallbackReason, error) {
	ctx, capture := WithResponseCapture(WithResponseLimit(ctx, GrantMaxBytes))
	token, err := read(ctx)
	// Separate ifs rather than a tagless switch, whose case expressions carry
	// no statement counter for the mutation tool to measure.
	if errors.Is(err, ErrResponseTooLarge) {
		return finegrained.Grant{}, finegrained.FallbackGrantTooLarge, nil
	}
	if isStatus(err, http.StatusForbidden) {
		return finegrained.Grant{}, finegrained.FallbackGrantUnreadable, nil
	}
	// client-go decodes only a 2xx answer, so a value of the wrong type in the
	// scope list is GitLab answering with a grant this server does not know:
	// a shape the authority names, not a read that failed. Any other decode
	// failure (no JSON at all, or a wrong type in a field the token read
	// already modeled) stays an error, as it was before client-go decoded the
	// scopes.
	if isGrantShapeError(err) {
		return finegrained.Grant{}, finegrained.FallbackGrantShape, nil
	}
	if err != nil {
		return finegrained.Grant{}, finegrained.FallbackNone, fmt.Errorf("read the token's grant: %w", err)
	}
	answer := grantFromAnswer(capture, token)
	return answer.grant, answer.reason, nil
}

// grantFromAnswer reads the grant out of an answer client-go decoded into
// token, taking the granular flag from the captured copy of the same body.
func grantFromAnswer(capture *ResponseCapture, token *gl.PersonalAccessToken) grantAnswer {
	var extra grantExtra
	// An answer that is not JSON as a whole (client-go's decoder stops at the
	// first value and ignores what trails it) is a reason the authority names,
	// not a read that failed, so it is no error.
	// A body of null decodes without an error on both reads and leaves client-go
	// no token at all, which is a shape the authority names, as above.
	decoded := capture.Decode(&extra) == nil
	if !decoded || token == nil {
		return grantAnswer{reason: finegrained.FallbackGrantShape}
	}
	grant, reason := finegrained.GrantFromScopes(extra.Granular, grantScopes(token.GranularScopes), GrantMaxScopes)
	return grantAnswer{grant: grant, reason: reason}
}

// grantAnswer is what one answer said of a token's grant: the grant, or why
// it cannot be evaluated.
type grantAnswer struct {
	grant  finegrained.Grant
	reason finegrained.FallbackReason
}

// selfGrant holds the grant a self answer carried until one reading takes it
// or stops before it for want of a version ([Client.ReadFineGrained]), and
// says, for as long as the facts live, that the self route is the one the
// grant is read from. The grant is handed out once and let go: the facts an
// HTTP pool entry keeps for its life would otherwise hold a grant of up to
// [GrantMaxBytes], whose size is its minter's to choose, beside an authority
// sized by the table alone. A reading after that, a revalidation's, asks the
// self route again.
type selfGrant struct {
	answer atomic.Pointer[grantAnswer]
}

// release lets go of the carried grant when a reading stops before it: a
// version that was not answered, or one this server cannot read, which for a
// token refused Metadata: Read is every version for the token's whole life.
// Nil, facts whose self answer carried no grant, has nothing to let go.
func (s *selfGrant) release() {
	if s != nil {
		s.answer.Store(nil)
	}
}

// carry records on the facts the grant their self answer carried, which
// makes it readable whether or not the answer named the token's id.
func (f *TokenFacts) carry(answer grantAnswer) {
	f.selfGrant = &selfGrant{}
	f.selfGrant.answer.Store(&answer)
	f.GrantReadable = true
}

// grant returns the token's grant: the one its self answer carried when no
// reading has taken it yet, the self route's answer again when one has, and
// GET /personal_access_tokens/:id's when the self answer did not carry it.
func (c *Client) grant(ctx context.Context, facts TokenFacts) (finegrained.Grant, finegrained.FallbackReason, error) {
	if facts.selfGrant == nil {
		return ReadGrant(ctx, c.GL(), facts.ID)
	}
	if answer := facts.selfGrant.answer.Swap(nil); answer != nil {
		return answer.grant, answer.reason, nil
	}
	return readSelfGrant(ctx, c.GL())
}

// isGrantShapeError reports whether err is client-go's JSON decoder refusing
// a value of the wrong type in granular_scopes, the list itself or a field of
// one of its entries (the decoder names those granular_scopes.0.access and
// the like).
func isGrantShapeError(err error) bool {
	var typeErr *json.UnmarshalTypeError
	if !errors.As(err, &typeErr) {
		return false
	}
	return typeErr.Field == grantScopesField || strings.HasPrefix(typeErr.Field, grantScopesField+".")
}

// grantScopesField is the JSON name of the granular scope list.
const grantScopesField = "granular_scopes"

// grantExtra is what the self or by-id answer about a token
// (GET /personal_access_tokens/self or /personal_access_tokens/:id) sends that
// client-go's PersonalAccessToken does not model: the granular flag, a
// pointer so that an answer without it is told from one that says false.
type grantExtra struct {
	Granular *bool `json:"granular"`
}

// grantScopes hands the granular scopes client-go decoded to
// [finegrained.GrantFromScopes] in its own type, keeping nil (the answer
// carried no list, or null) apart from empty (it carried an empty one). A
// null entry becomes a scope with no access level, which the evaluation reads
// as a shape it does not know.
func grantScopes(scopes []*gl.GranularScope) []finegrained.GrantScope {
	if scopes == nil {
		return nil
	}
	out := make([]finegrained.GrantScope, 0, len(scopes))
	for _, scope := range scopes {
		if scope == nil {
			out = append(out, finegrained.GrantScope{})
			continue
		}
		out = append(out, finegrained.GrantScope{
			Access:      string(scope.Access),
			Permissions: scope.Permissions,
			ProjectID:   scope.ProjectID,
			GroupID:     scope.GroupID,
		})
	}
	return out
}

// ReadFineGrained reads what a fine-grained token's authority is judged from
// ([finegrained.Judge]): the version the instance reports, and the token's
// grant, for the token the client authenticates with as [DetectToken]
// described it.
//
// A token that may not read its own grant is not asked anything, since no
// answer could lift that: its reading says why and nothing else, and its
// authority is phase A. Otherwise it asks GET /api/v4/version through the
// health client, and then reads the grant only when that left a version to
// judge it at, since without one no grant is evaluated: from the self answer
// [DetectToken] kept when it carried the grant, with no request, and from GET
// /personal_access_tokens/:id when it did not. A carried grant is let go by
// the first reading, which takes it when it gets that far and drops it when
// the version stops it, and a later one asks the self route again.
// A version read the instance did not answer is
// [finegrained.FallbackVersionUnanswered], and one it answered with no
// version this server can read (a refusal of Metadata: Read, or a string that
// does not validate) [finegrained.FallbackVersionUnreadable]; a grant read it
// did not answer is [finegrained.FallbackGrantUnanswered]. The two unanswered
// reasons are the ones a later read can lift without anything about the token
// changing. Nothing is logged here: the error of an unanswered read names the
// route, and the route names the token's id.
func (c *Client) ReadFineGrained(ctx context.Context, facts TokenFacts) finegrained.Reading {
	if !facts.GrantReadable {
		return finegrained.Reading{Fallback: finegrained.FallbackGrantUnreadable}
	}
	version, answered := c.ReadVersion(ctx)
	if !answered {
		facts.selfGrant.release()
		return finegrained.Reading{Fallback: finegrained.FallbackVersionUnanswered}
	}
	return c.ReadFineGrainedAt(ctx, facts, version)
}

// ReadFineGrainedOnce is [Client.ReadFineGrained] for a build that may already
// have read the version: it reads the grant at the version the last answered
// read reported, and asks /version only when no read has answered. From
// GitLab 19.6 the tier probe reads the version first, for the instance plan
// (issue 1491), so an HTTP pool entry and a stdio start ask it once whatever
// their token. A refusal of the version already seen is an answer with no
// version to judge at, as [Client.ReadVersion] reads it. A revalidation keeps
// [Client.ReadFineGrained], since it is there to learn whether the release
// moved.
func (c *Client) ReadFineGrainedOnce(ctx context.Context, facts TokenFacts) finegrained.Reading {
	if _, refused := c.VersionRefusal(); refused {
		return c.ReadFineGrainedAt(ctx, facts, "")
	}
	if version := c.version.Load(); version != nil {
		return c.ReadFineGrainedAt(ctx, facts, *version)
	}
	return c.ReadFineGrained(ctx, facts)
}

// ReadFineGrainedAt is [Client.ReadFineGrained] with the version already
// read and answered, which is a stdio start's: [Client.Initialize] asked it
// moments ago, and asking again would cost the instance a request and a token
// without Metadata: Read a second refusal for nothing. An empty version is one
// the instance answered with none this server can read: the grant is not
// asked for, and one the self answer carried is let go.
func (c *Client) ReadFineGrainedAt(ctx context.Context, facts TokenFacts, version string) finegrained.Reading {
	if !facts.GrantReadable {
		return finegrained.Reading{Fallback: finegrained.FallbackGrantUnreadable}
	}
	if version == "" {
		facts.selfGrant.release()
		return finegrained.Reading{Fallback: finegrained.FallbackVersionUnreadable}
	}
	grant, reason, err := c.grant(ctx, facts)
	if err != nil {
		reason = finegrained.FallbackGrantUnanswered
	}
	return finegrained.Reading{Grant: grant, Fallback: reason, Version: version}
}

// RefreshAuthority re-reads a fine-grained token's grant and the instance
// version and replaces the authority this client carries when the reads
// answered, judging them against table ([finegrained.Rejudge]).
//
// It returns the new authority when the replacement moved the token to
// another verdict ([finegrained.Moved]: its phase, its reason or the release it
// was judged at), which the caller logs with its [finegrained.Authority.LogArgs]
// as an entry build logs the one it builds, so an instance upgraded to a
// release no table records leaves a line saying the sessions on it fell back. A
// replacement that moved nothing returns neither, since a re-read that answered
// replaces the authority on every round. When it kept the authority it returns
// the reason, which the caller logs once rather than on every round: a version
// that was not answered or not readable, or why the grant was not usable. A
// token that may not read its grant is not asked anything, and keeps its
// authority.
//
// It is what an accepted revalidation of an HTTP pool entry does, and what the
// stdio timer does for the process, so an instance upgraded under a running
// session moves it to that release's verdict without its credential being
// rebuilt.
//
// Each round reads the grant from the one route the build read it from: the
// self route when the build's self answer carried it, whose grant the build
// took, and GET /personal_access_tokens/:id when it did not. It does not ask
// both, nor move from one to the other on an upgrade: the grant is the same on
// either, since nothing edits it after the token is created, the by-id read
// keeps answering after an upgrade to 19.5, and a second request a round would
// cost every pooled fine-grained token for nothing it could learn.
func (c *Client) RefreshAuthority(ctx context.Context, facts TokenFacts, table *finegrained.Table) (moved *finegrained.Authority, kept string) {
	if !facts.GrantReadable {
		return nil, ""
	}
	reading := c.ReadFineGrained(ctx, facts)
	current := c.Authority()
	next, replaced := finegrained.Rejudge(table, current, reading)
	if !replaced {
		return nil, string(reading.Fallback)
	}
	c.SetAuthority(next)
	if finegrained.Moved(current, next) {
		return next, ""
	}
	return nil, ""
}

// isStatus reports whether err is GitLab answering with the given status.
func isStatus(err error, status int) bool {
	refusal, isResponse := errors.AsType[*gl.ErrorResponse](err)
	return isResponse && refusal.StatusCode == status
}
