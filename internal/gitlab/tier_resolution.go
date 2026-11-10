// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package gitlab

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
)

// The steps of the tier probe, as [TierResolution.Unresolved] names them.
const (
	tierStepLicense    = "license"
	tierStepNamespaces = "namespaces"
	tierStepMembership = "membership"
)

// TierResolution is what one run of the tier probe concluded.
//
// A step of the probe either answered, which GitLab did whenever it said
// anything about the question, a refusal or an empty page included, or it was
// not read: GitLab did not answer it ([tierReadUnresolved]). The two must not
// be told apart by the tier alone, because both leave it Free, and only the
// first is evidence that Free is the caller's tier (issue 1782).
type TierResolution struct {
	// Tier is the highest tier a step answered for, or Free when none did.
	Tier edition.Tier
	// Answered reports whether any step answered for a tier: the license's
	// plan, a namespace's plan or a top-level group's. False is the Free
	// fallback an enterprise build warns about.
	Answered bool
	// Provisional reports that a step which could have raised Tier was not
	// read. Tier is then the best known so far, a floor rather than GitLab's
	// answer, and asking again may raise it. Ultimate is never provisional,
	// since nothing can raise it.
	Provisional bool
	// Unresolved names the steps that were not read, in the order the probe
	// asks them, for the line that reports the result: "license",
	// "namespaces" and "membership".
	Unresolved []string
}

// tierStep is one step's outcome.
type tierStep struct {
	// tier is the highest tier the step answered for, Free when none.
	tier edition.Tier
	// answered reports whether the step answered for a tier.
	answered bool
	// unresolved reports that the step, or a page of it, was not read.
	unresolved bool
}

// merge folds one step's outcome into the resolution: the highest tier
// answered wins, on ADR-0018's asymmetry, and a step not read is named.
func (r *TierResolution) merge(name string, step tierStep) {
	if step.answered {
		r.Tier, r.Answered = max(r.Tier, step.tier), true
	}
	if step.unresolved {
		r.Unresolved = append(r.Unresolved, name)
	}
}

// ResolveTier asks the tier probe's questions and returns what they
// concluded. It stores nothing on the client and warns about nothing, so it
// can be asked of a client whose catalog is already registered without moving
// what that client's handlers read; [Client.DetectTier] is the call that
// adopts the answer.
//
// It asks up to three questions, because no one of them answers for every
// deployment:
//
//   - GET /license, which carries the instance's own plan and is **admin-only
//     on self-managed**. It is the authoritative answer where it is available,
//     and nothing is asked after a plan it names.
//   - GET /namespaces, which carries a plan per namespace and is readable by
//     any token, for the namespaces the caller administers. On GitLab.com that
//     plan is the subscription; on self-managed it is "default" whatever the
//     instance is licensed for, since a subscription is a GitLab.com concept.
//   - On GitLab.com alone, the top-level groups the caller is a member of, each
//     with whether its plan carries a Premium and an Ultimate feature
//     ([Client.tierFromMembership]), asked unless the namespaces already
//     answered Ultimate.
//
// Measured on 2026-09-22 against a licensed EE 19.3.1 and against GitLab.com:
// a non-admin token is refused /license with 403 on both, /namespaces reports
// "default" for admin and non-admin alike on self-managed, and reports the real
// plan on GitLab.com for each namespace the caller administers. So the second
// question rescues the GitLab.com caller who owns a paid group, and rescues
// nobody on self-managed. It does not rescue a member: GitLab.com shows a
// namespace's plan only to a caller who may administer it, while it serves a
// licensed feature to every member of a group whose top-level group's plan
// carries it, which is most of the people working in a paid group (issue
// 1224). The third question is that check, and GitLab.com answered it for the
// maintainer's account, a Developer of a subgroup of a licensed group, on
// 2026-10-08.
//
// The highest answer of the second and third wins, on ADR-0018's asymmetry,
// and a step that was not read lowers nothing: it is no evidence that the
// caller holds less than it was already shown to hold. It makes the answer
// provisional instead, unless the answer is already Ultimate.
func (c *Client) ResolveTier(ctx context.Context) TierResolution {
	license := c.tierFromLicense(ctx)
	if license.answered {
		return TierResolution{Tier: license.tier, Answered: true}
	}
	var resolution TierResolution
	resolution.merge(tierStepLicense, license)
	resolution.merge(tierStepNamespaces, c.tierFromNamespaces(ctx))
	if resolution.Tier < edition.Ultimate && membershipTierApplies(c) {
		resolution.merge(tierStepMembership, c.tierFromMembership(ctx))
	}
	resolution.Provisional = len(resolution.Unresolved) > 0 && resolution.Tier < edition.Ultimate
	return resolution
}

// DetectTier is [Client.ResolveTier] and what a build does with its answer:
// it stores the tier on the client, warns when the answer is provisional, and
// warns as it always has when no step answered.
//
// Where none answers, the tier is [edition.Free] as before. That is right on a
// CE instance and wrong on a licensed one the caller cannot read the license
// of, and the two are told apart by the edition rather than guessed at: an
// enterprise build that could not be resolved gets a warning naming the flag
// that settles it, and a CE build stays silent because Free is the truth there.
func (c *Client) DetectTier(ctx context.Context) TierResolution {
	resolution := c.ResolveTier(ctx)
	if resolution.Provisional {
		slog.WarnContext(ctx, "could not finish detecting the licensing tier: a step GitLab did not answer could raise it, "+
			"so the tier detected so far is served; GITLAB_MCP_TIER (or --tier in HTTP mode) pins it",
			"tier", resolution.Tier.String(), "unresolved", resolution.Unresolved)
	}
	if !resolution.Answered {
		c.warnUnresolvedTier(ctx)
	}
	c.SetTier(resolution.Tier)
	return resolution
}

// tierReadUnresolved reports whether err leaves a step of the tier probe
// unread, as opposed to GitLab answering it.
//
// GitLab answered when it sent a status that says something about the
// question: 401 and 403 refuse the caller, 404 says there is no such thing
// (a CE instance's license), and every other 4xx refuses the request, which
// asking again does not change. It did not answer when no status came back,
// for a deadline, a cancellation, a network or TLS failure or a body that did
// not decode, nor when the status is its own moment: 408, 429, a redirect the
// client did not follow, or a 5xx, which reaches here after the retries.
func tierReadUnresolved(err error) bool {
	if errors.Is(err, gl.ErrNotFound) {
		return false
	}
	status, answered := gitLabStatus(err)
	if !answered {
		return true
	}
	return status >= http.StatusInternalServerError ||
		status == http.StatusTooManyRequests ||
		status == http.StatusRequestTimeout ||
		(status >= http.StatusMultipleChoices && status < http.StatusBadRequest)
}

// gitLabStatus is the HTTP status GitLab answered err with, and false when no
// status came back. GraphQL.Do wraps the *gl.ErrorResponse of a JSON body in a
// *gl.GraphQLResponseError, which has no Unwrap, so that one is opened by
// hand.
func gitLabStatus(err error) (int, bool) {
	if refusal, ok := errors.AsType[*gl.GraphQLResponseError](err); ok {
		err = refusal.Err
	}
	if answer, ok := errors.AsType[*gl.ErrorResponse](err); ok {
		return answer.StatusCode, true
	}
	return 0, false
}
