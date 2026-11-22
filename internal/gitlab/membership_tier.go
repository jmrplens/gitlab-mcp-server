// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package gitlab

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/hashicorp/go-retryablehttp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// MembershipTierQuery asks GitLab.com for the top-level groups the caller
// reaches and, for each, whether its plan carries a Premium and an Ultimate
// feature (issue 1224). It is exported for one reader, the test in
// internal/testutil that holds its cost to what GitLab.com measured: the
// estimate lives there, testutil imports this package so this package's tests
// cannot import it, and an external test package here would cost the package
// its condition gate (gobco cannot instrument one).
//
// **Features rather than the plan.** Group.plan is authorized for
// :admin_namespace, so a member who does not administer the group reads null,
// the same wall GET /namespaces puts up. licensedFeatureAvailability is open to
// any member, and on GitLab.com it answers from the plan of the group's root
// ancestor, trials and the open source program included, which is the check
// GitLab itself makes before serving a licensed feature. EPICS is a Premium
// feature and SECURITY_DASHBOARD an Ultimate one in GitlabSubscriptions::
// Features, and neither is global, which GitLab refuses to answer for.
//
// **Which groups.** allAvailable: false is GroupsFinder's authorized groups:
// direct memberships, the groups shared with them, and the groups of the
// projects the caller is a member of, each with its ancestors, so
// topLevelOnly: true returns the root of every group the caller works in, a
// developer of a subgroup included. GitLab.com answered this for the
// maintainer's account on 2026-10-08, a Developer of a subgroup of a licensed
// group who administers only free namespaces: gitlab-community available for
// both features, where the namespace step had answered Free. A role is not
// filtered on, because the page offers no role at a root a caller reaches only
// through a subgroup; a member of any role is served a licensed feature
// GitLab's own policy lets that role use, and the role decides the rest.
//
// **A fine-grained token gets no answer from it.** GitLab declares no
// fine-grained permission for LicensedFeatureAvailability (it is on the
// authorization to-do list of the 19.4.1 record), so it refuses that answer to
// every fine-grained token. The field and its available are both non-null, so
// the refusal nulls the whole node: such a token is answered a page of null
// groups beside an errors array, no group answers, and it keeps the tier the
// namespace step gave it.
//
// It costs 22 at a page of a hundred, measured on GitLab.com, against the 250
// GitLab refuses a query above.
const MembershipTierQuery = `query tierFromMembership($first: Int!, $after: String) {
  groups(allAvailable: false, topLevelOnly: true, first: $first, after: $after) {
    nodes {
      fullPath
      premium: licensedFeatureAvailability(feature: EPICS) { available }
      ultimate: licensedFeatureAvailability(feature: SECURITY_DASHBOARD) { available }
    }
    pageInfo { hasNextPage endCursor }
  }
}`

// MembershipGroupsQuery is [MembershipTierQuery] without topLevelOnly: the
// groups the caller reaches themselves, subgroups included, each with the same
// two features. The membership step asks it only when the first pass returned
// no node at all (issue 1491), for a caller whose only membership is a
// subgroup holding no project under a private top-level group, whose root
// GitLab does not show it. A subgroup answers the features from its root's
// plan on GitLab.com and from the instance license on a self-managed
// instance. It is exported for the same reader as [MembershipTierQuery], the
// cost test in internal/testutil.
const MembershipGroupsQuery = `query tierFromGroupMembership($first: Int!, $after: String) {
  groups(allAvailable: false, first: $first, after: $after) {
    nodes {
      fullPath
      premium: licensedFeatureAvailability(feature: EPICS) { available }
      ultimate: licensedFeatureAvailability(feature: SECURITY_DASHBOARD) { available }
    }
    pageInfo { hasNextPage endCursor }
  }
}`

// membershipPageSize and membershipMaxPages bound the top-level groups the
// tier probe reads, for the reason namespacePlanPageSize and
// namespacePlanMaxPages give: the walk stops at the first Ultimate, which
// settles the common case on the first page, and after membershipMaxPages
// either way, so a caller reaching more top-level groups than that resolves on
// those it was shown.
const (
	membershipPageSize = tenancy.TierMembershipPageSize // register row AUT-003
	membershipMaxPages = tenancy.TierMembershipMaxPages // register row AUT-003
)

// membershipTimeout bounds the membership step on a clock of its own,
// measured from the membership base [Client.ResolveTier] is given rather than
// from what the license and namespace steps left of the context they share.
// The step is asked last, so a classic token's license and namespace reads,
// retried with backoff, could spend the whole shared budget before it, and the
// one step that answers a member of a paid group on GitLab.com then went
// unread (issue 1782). It covers one page with its two retries or several
// healthy pages.
const membershipTimeout = tenancy.TierMembershipTimeout // register row AUT-003

// membershipTierApplies reports whether c's instance is GitLab.com, the one
// instance that sells its plans per top-level group, where the membership
// step is always asked. It is a variable so a test can stand an httptest
// server in for GitLab.com, whose host a test server cannot have.
// [Client.membershipStepApplies] is the whole rule.
var membershipTierApplies = (*Client).IsGitLabDotCom

// membershipSinceMajor and membershipSinceMinor are the first release whose
// GraphQL schema has Group.licensedFeatureAvailability, which the membership
// step asks; a CE build has no such field at any release.
const (
	membershipSinceMajor = 18
	membershipSinceMinor = 11
)

// membershipStepApplies reports whether the membership step is asked of c's
// instance (issue 1491). GitLab.com always. An instance whose version answer
// carried a null plan, GitLab saying subscriptions are held per namespace.
// And an enterprise build from 18.11 that reported no recognized plan, where
// licensedFeatureAvailability answers from the instance license wherever
// namespace plans are not checked (load_feature_available in
// ee/app/models/ee/namespace.rb): measured on 2026-11-01 against a licensed
// 19.5.0, where a non-administrator's groups answered true, and on 2026-11-22
// against an unlicensed 19.6.1, where they answered false, which is an
// answer. That reaches a non-administrator on a licensed instance before
// 19.6, which only the license told before. A CE build, an enterprise build
// before 18.11 and an instance whose edition is unknown are not asked, since
// their schemas may not have the field. It reads what the version read of the
// plan step already learned.
func (c *Client) membershipStepApplies(ctx context.Context, plan instancePlan) bool {
	if membershipTierApplies(c) || plan.kind == instancePlanPerNamespace {
		return true
	}
	if !c.AtLeast(ctx, membershipSinceMajor, membershipSinceMinor) {
		return false
	}
	answer, _ := c.editionAsRead()
	return answer == editionEnterprise
}

// errNoGroupsConnection is a membership page GitLab answered with no groups
// connection and no error, which is no answer.
var errNoGroupsConnection = errors.New("the answer carried no groups connection")

// membershipTierAnswer is GitLab's answer to [MembershipTierQuery].
type membershipTierAnswer struct {
	Data struct {
		Groups *membershipGroups `json:"groups"`
	} `json:"data"`
	Errors []membershipTierError `json:"errors"`
}

// membershipTierError is one top-level error GitLab answered the document
// with.
type membershipTierError struct {
	Message string `json:"message"`
}

// membershipGroups is one page of the top-level groups the caller reaches.
type membershipGroups struct {
	Nodes    []*membershipGroup `json:"nodes"`
	PageInfo struct {
		HasNextPage bool   `json:"hasNextPage"`
		EndCursor   string `json:"endCursor"`
	} `json:"pageInfo"`
}

// membershipGroup is one top-level group and whether its plan carries the two
// features the query asks about.
type membershipGroup struct {
	FullPath string           `json:"fullPath"`
	Premium  featureAvailable `json:"premium"`
	Ultimate featureAvailable `json:"ultimate"`
}

// featureAvailable is GitLab's answer about one licensed feature.
type featureAvailable struct {
	Available bool `json:"available"`
}

// tier is the tier the group's plan carries, read from the features it makes
// available.
func (g *membershipGroup) tier() edition.Tier {
	switch {
	case g.Ultimate.Available:
		return edition.Ultimate
	case g.Premium.Available:
		return edition.Premium
	default:
		return edition.Free
	}
}

// tierFromMembership reads the plans of the top-level groups the caller
// reaches, where [Client.membershipStepApplies] asks it, and answers with the
// highest, for the reason
// [Client.tierFromNamespaces] gives for taking the highest. It has answered
// nothing when no group answered, which a caller in no group is, and a read
// that fails keeps what earlier pages answered.
//
// A page GitLab answered in part keeps the groups it resolved and ends the
// walk. Every node that came back was resolved whole (see
// [MembershipTierQuery]), so a group answering Premium is evidence of Premium
// whatever GitLab refused beside it, and discarding it would resolve lower
// than the caller was shown to hold; the page after it would be refused the
// same way, which is what a fine-grained token's walk would otherwise spend
// up to membershipMaxPages requests learning. A page whose every error is
// that permission refusal is GitLab's answer, so the step it ends is
// resolved, the fine-grained token's page of null nodes included. Any other
// error leaves the step unresolved ([membershipReadUnresolved]), whatever
// the page resolved beside it: a feature that ran out of time nulls its node
// as a refused one does, so the null node may be the group that raises the
// tier, and the pages after it go unread. The groups the page resolved still
// count, as the floor the step answers, and the step is asked again.
//
// A read that did not come back whole is said at info, naming the setting
// that pins the tier, because the namespace step may already have answered
// Free: no warning fires then, and the tier a caller is served would say
// nothing about the step that could have raised it.
//
// **A second pass, when the first returned no node at all** (issue 1491). A
// caller whose only membership is a subgroup holding no project, under a
// private top-level group, is shown no root: GitLab finds it and drops the
// node, since the Group type requires read_group, which a private root grants
// a non-member only through a project under it. Asked without topLevelOnly
// ([MembershipGroupsQuery]), the subgroup answers, from its root's plan on
// GitLab.com and from the instance license on a self-managed instance
// (measured on 2026-11-01 and 2026-11-22). It is asked only after a first
// pass that GitLab answered whole and that returned no node, null nodes
// included, so no caller the first pass answered pays for the extra pages.
func (c *Client) tierFromMembership(ctx context.Context) tierStep {
	step, nodes, failed := c.membershipWalk(ctx, MembershipTierQuery)
	if nodes > 0 || failed {
		return step
	}
	slog.DebugContext(ctx, "no top-level group answered; asking the groups the token is a member of themselves")
	step, _, _ = c.membershipWalk(ctx, MembershipGroupsQuery)
	return step
}

// membershipWalk reads the pages query answers, as [Client.tierFromMembership]
// describes, and returns the step with how many nodes the pages carried, null
// ones included, and whether a page came back with an error.
func (c *Client) membershipWalk(ctx context.Context, query string) (step tierStep, nodes int, failed bool) {
	best, found, unresolved := edition.Free, false, false
	after := ""
	// The page cap ends the walk, as it does the namespace walk, and for the
	// same reason no second copy of it sits in the loop header.
	for page := 1; ; page++ {
		groups, err := c.membershipPage(ctx, query, after)
		if groups != nil {
			nodes += len(groups.Nodes)
			for _, group := range groups.Nodes {
				if group == nil {
					continue
				}
				tier := group.tier()
				best, found = max(best, tier), true
				slog.DebugContext(ctx, "a group the token reaches answers for its plan", "group", group.FullPath, "tier", tier.String())
			}
		}
		if err != nil {
			failed = true
			unresolved = membershipReadUnresolved(err)
			slog.InfoContext(ctx, "could not read every group the token is a member of; the tier is resolved without the rest, "+
				"and GITLAB_MCP_TIER (or --tier in HTTP mode) pins it", "page", page, "error", err, "unresolved", unresolved)
			break
		}
		if best == edition.Ultimate || !groups.PageInfo.HasNextPage {
			break
		}
		if page == membershipMaxPages {
			slog.DebugContext(ctx, "stopped reading groups at the page bound; a paid plan past it is not seen",
				"pages", membershipMaxPages, "per_page", membershipPageSize)
			break
		}
		after = groups.PageInfo.EndCursor
	}

	if found {
		slog.InfoContext(ctx, "detected GitLab tier from the groups the token is a member of", "tier", best.String())
	}
	return tierStep{tier: best, answered: found, unresolved: unresolved}, nodes, failed
}

// membershipPage asks query for the page of groups that follows after, or the
// first page when after is empty. query is [MembershipTierQuery] or
// [MembershipGroupsQuery], which share their variables and their answer.
//
// GitLab answers a document it refuses with HTTP 200 and a top-level errors
// array, which client-go does not turn into an error, so the answer is read
// for one. Without a groups connection the page is no answer, and the error
// is GitLab's reason where it gave one. With one, the page is returned beside
// the error naming what GitLab refused in it, so the caller keeps the groups
// it resolved.
func (c *Client) membershipPage(ctx context.Context, query, after string) (*membershipGroups, error) {
	variables := map[string]any{"first": membershipPageSize}
	if after != "" {
		variables["after"] = after
	}
	var answer membershipTierAnswer
	// The retry check goes after the context: WithContext replaces the
	// request's context, which is where WithRequestRetry keeps the check.
	if _, err := c.inner.GraphQL.Do(gl.GraphQLQuery{Query: query, Variables: variables}, &answer,
		gl.WithContext(ctx), gl.WithRequestRetry(c.membershipRetry)); err != nil {
		return nil, err
	}
	refusal := graphQLRefusal(answer.Errors)
	if refusal != nil {
		return answer.Data.Groups, refusal
	}
	if answer.Data.Groups == nil {
		return nil, errNoGroupsConnection
	}
	return answer.Data.Groups, nil
}

// membershipRetry is the retry check of the membership query, which is a POST
// that only reads: the client's policy ([retryOptions]) retries a 5xx only for
// an idempotent method, so one 502 used to leave the step unread where a GET
// would have been asked again (issue 1782). It is retryablehttp's default
// policy, which retries a 5xx other than 501, a 429 and a failure to reach
// GitLab, and never once ctx has ended, so the deadline that ends the step is
// not spent on a retry. The re-sends and the wait between them are the
// client's own (RTC-006), and a client built with retries off retries this
// request no more than any other.
func (c *Client) membershipRetry(ctx context.Context, resp *http.Response, err error) (bool, error) {
	if c.retriesDisabled {
		return false, nil
	}
	return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
}

// graphQLRefusal is the refusal a top-level errors array stands for, or nil
// when GitLab answered none.
func graphQLRefusal(refusals []membershipTierError) *documentRefusalError {
	if len(refusals) == 0 {
		return nil
	}
	messages := make([]string, 0, len(refusals))
	for _, refusal := range refusals {
		messages = append(messages, refusal.Message)
	}
	return &documentRefusalError{messages: messages}
}

// permissionRefusalMessage is the sentence GitLab answers a GraphQL field the
// caller may not read with: the resource is said not to exist or not to be the
// caller's, never which.
const permissionRefusalMessage = "The resource that you are attempting to access does not exist or you don't have permission to perform this action"

// documentRefusalError is a membership page GitLab answered with a top-level
// errors array, in place of the groups or beside them.
type documentRefusalError struct {
	// messages are GitLab's own, in the order it answered them.
	messages []string
}

// Error names GitLab's messages, as the line that reports a membership read
// has always quoted them.
func (r *documentRefusalError) Error() string {
	return "GitLab answered the document with errors: " + strings.Join(r.messages, "; ")
}

// permissionOnly reports whether every message is GitLab's permission
// refusal, which is an answer about the caller: asked again, GitLab refuses
// it the same way. Any other message may be GitLab's own moment, a field that
// ran out of time or an internal error, which it answers with HTTP 200 and
// the same shape.
func (r *documentRefusalError) permissionOnly() bool {
	for _, message := range r.messages {
		if message != permissionRefusalMessage {
			return false
		}
	}
	return true
}

// membershipReadUnresolved reports whether a membership page that came back
// with an error, whatever groups it resolved beside it, left the step unread,
// as opposed to GitLab answering it: a document refused or answered in part
// for anything but the caller's permission, an answer with
// no groups connection and no reason, or a read [tierReadUnresolved] says
// GitLab did not answer.
func membershipReadUnresolved(err error) bool {
	if refusal, ok := errors.AsType[*documentRefusalError](err); ok {
		return !refusal.permissionOnly()
	}
	if errors.Is(err, errNoGroupsConnection) {
		return true
	}
	return tierReadUnresolved(err)
}
