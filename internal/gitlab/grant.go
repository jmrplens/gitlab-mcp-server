package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

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
// client-go does not model the grant (row 32 of
// docs/development/upstream-bugs.md), and the self endpoint does not present
// it, so it is decoded from the captured response of GET
// /personal_access_tokens/:id (ADR-0021), read under [GrantMaxBytes] and
// decoded with at most [GrantMaxScopes] scopes ([finegrained.DecodeGrant]).
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
	ctx, capture := WithResponseCapture(WithResponseLimit(ctx, GrantMaxBytes))
	_, _, err := client.PersonalAccessTokens.GetSinglePersonalAccessTokenByID(id, gl.WithContext(ctx))
	// Separate ifs rather than a tagless switch, whose case expressions carry
	// no statement counter for the mutation tool to measure.
	if errors.Is(err, ErrResponseTooLarge) {
		return finegrained.Grant{}, finegrained.FallbackGrantTooLarge, nil
	}
	if isStatus(err, http.StatusForbidden) {
		return finegrained.Grant{}, finegrained.FallbackGrantUnreadable, nil
	}
	if err != nil {
		return finegrained.Grant{}, finegrained.FallbackNone, fmt.Errorf("read the token's grant: %w", err)
	}
	var body json.RawMessage
	if decodeErr := capture.Decode(&body); decodeErr != nil {
		return finegrained.Grant{}, finegrained.FallbackGrantShape, nil //nolint:nilerr // an answer that is not JSON is a reason the authority names, not a read that failed
	}
	grant, reason := finegrained.DecodeGrant(body, GrantMaxScopes)
	return grant, reason, nil
}

// ReadFineGrained reads what a fine-grained token's authority is judged from
// ([finegrained.Judge]): the version the instance reports, and the token's
// grant, for the token the client authenticates with as [DetectToken]
// described it.
//
// A token that may not read its own grant is not asked anything, since no
// answer could lift that: its reading says why and nothing else, and its
// authority is phase A. Otherwise it asks GET /api/v4/version through the
// health client, and then GET /personal_access_tokens/:id only when that left
// a version to judge the grant at, since without one no grant is evaluated.
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
		return finegrained.Reading{Fallback: finegrained.FallbackVersionUnanswered}
	}
	return c.ReadFineGrainedAt(ctx, facts, version)
}

// ReadFineGrainedAt is [Client.ReadFineGrained] with the version already
// read and answered, which is a stdio start's: [Client.Initialize] asked it
// moments ago, and asking again would cost the instance a request and a token
// without Metadata: Read a second refusal for nothing. An empty version is one
// the instance answered with none this server can read, and the grant is not
// asked for.
func (c *Client) ReadFineGrainedAt(ctx context.Context, facts TokenFacts, version string) finegrained.Reading {
	if !facts.GrantReadable {
		return finegrained.Reading{Fallback: finegrained.FallbackGrantUnreadable}
	}
	if version == "" {
		return finegrained.Reading{Fallback: finegrained.FallbackVersionUnreadable}
	}
	grant, reason, err := ReadGrant(ctx, c.GL(), facts.ID)
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
// as an entry build logs one in phase A, so an instance upgraded to a release
// no table records leaves a line saying the sessions on it fell back. A
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
